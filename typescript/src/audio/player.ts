/**
 * SinkPlayer — the main-thread half of the playout path: AudioContext +
 * playout worklet + gain, over a SharedArrayBuffer jitter ring the MOQ
 * worker's decoder writes. Descended from the old SDK's AudioPlayer
 * (audio-player.ts) but SAB-only and decode-free: WebCodecs decode
 * lives in the worker (architecture C), so this class owns no decoder,
 * no postMessage-PCM path, and no worker-handoff switch — it IS the
 * worker path.
 *
 * One SinkPlayer per playing sink (multi-entity: create several). Main
 * only receives the worklet's ~4/s stats snapshots — nothing here is on
 * the real-time audio path.
 */

import { MoqClientError } from '../transport/errors.js';
import {
  createPlayoutWorkletUrl,
  PLAYOUT_PROCESSOR_NAME,
  type PlayoutEventMessage,
  type PlayoutProcessorOptions,
  type PlayoutStatsMessage,
} from './playout-worklet.js';
import {
  computeJitterCapacity,
  fecFloor,
  type JitterBufferCoreConfig,
  type JitterBufferSnapshot,
} from './jitter-buffer-core.js';
import type { StereoMeterReport } from './stereo-meter-core.js';
import type { SinkAudioConfig } from '../worker/protocol.js';

/** The AudioWorklet render quantum (frames per process() call) — fixed by the spec. */
const RENDER_QUANTUM = 128;
/** Default writer frame for geometry: the server's Opus output frame (5 ms @ 48 kHz). */
const DEFAULT_WRITER_FRAME = 240;

export interface SinkPlayerOptions {
  /** Sample rate (default 48000). */
  sampleRate?: number;
  /** Channels (default 2 — binaural). */
  channelCount?: number;
  /** Latency hint for the AudioContext (default 'interactive'). */
  latencyHint?: AudioContextLatencyCategory;
  /** Writer frame (server's Opus output) in samples. Default 240 (5 ms @ 48 kHz). */
  writerFrameSamples?: number;
  /**
   * LASA quality level 0–2 for this sink's playout buffer — an intent
   * ("how latency-tolerant is this stream"), mapped to {floor,
   * robustness, capacity} inside the core. Client-local for the sink
   * direction (no protocol involvement); default 0 (interactive).
   */
  qualityLevel?: number;
  /**
   * The entity's declared maximum redundancy offset 0–7 (lasa-core.md
   * §4.2) — the request that the space emit repeats on this sink
   * track. > 0 composes the FEC floor into the playout buffer (floors
   * compose by max) and puts the worker's reassembler in front of the
   * decoder. Default 0 (no FEC).
   */
  redundancy?: number;
  /** Advanced: override v4 jitter-buffer tuning, merged over the derived geometry. */
  jitterConfig?: Partial<JitterBufferCoreConfig>;
  /**
   * Worklet stats cadence in reads (128-sample quanta). Default 94
   * (~250 ms at 48 kHz). Latency harnesses shrink it (e.g. 6 ≈ 16 ms
   * windows) to timestamp changes in the rendered output — the finer
   * cadence costs one small postMessage per window, measurement only.
   */
  statsEvery?: number;
  debug?: boolean;
}

/** What the worker needs to write this player's ring. */
export interface SinkRingHandoff {
  jbufConfig: JitterBufferCoreConfig;
  sharedStorage: Float32Array;
  sharedWritePos: BigInt64Array;
  /** Declared max redundancy offset — the worker's reassembler switch. */
  redundancy: number;
}

/**
 * Snapshot of the playout graph's channel-count-relevant state (the old
 * SDK's stereo-diagnostics report, trimmed to this topology).
 */
export interface AudioGraphReport {
  context: {
    sampleRate: number;
    state: string;
    baseLatencyMs: number | null;
    outputLatencyMs: number | null;
  };
  destination: {
    channelCount: number;
    maxChannelCount: number;
    channelCountMode: string;
    channelInterpretation: string;
  };
  worklet: {
    outputChannelCount: number;
    channelCount: number;
    channelCountMode: string;
    channelInterpretation: string;
  };
  ring: {
    writerFrameSamples: number;
    numChannels: number;
    capacityFrames: number;
  };
}

export type SinkStatsListener = (
  snapshot: JitterBufferSnapshot,
  tapB: StereoMeterReport | null,
  /** performance.now() at receipt of this stats window. */
  atMs: number
) => void;

/** One buffer discontinuity, timestamped at receipt on main. */
export interface PlayoutEvent {
  kind: PlayoutEventMessage['kind'];
  fillFrames: number;
  /** Worklet read count at the event (2.667 ms per read at 48 kHz). */
  reads: number;
  /** performance.now() at receipt. */
  atMs: number;
}

const EVENT_RING_CAP = 500;

export class SinkPlayer {
  private lastSnapshot: JitterBufferSnapshot | null = null;
  private lastTapB: StereoMeterReport | null = null;
  private jbufLogCount = 0;
  private statsListener: SinkStatsListener | null = null;
  private events: PlayoutEvent[] = [];

  private constructor(
    private readonly audioContext: AudioContext,
    private readonly workletNode: AudioWorkletNode,
    private readonly gainNode: GainNode,
    private readonly jbConfigBase: JitterBufferCoreConfig,
    private readonly sharedStorage: Float32Array,
    private readonly sharedWritePos: BigInt64Array,
    private readonly opts: Required<
      Pick<SinkPlayerOptions, 'sampleRate' | 'channelCount' | 'writerFrameSamples' | 'redundancy' | 'debug'>
    >
  ) {
    workletNode.port.onmessage = (e: MessageEvent) => {
      const msg = e.data as PlayoutStatsMessage | PlayoutEventMessage;
      if (msg && msg.type === 'stats') {
        this.lastSnapshot = msg.snapshot;
        if (msg.stereo) this.lastTapB = msg.stereo;
        this.statsListener?.(msg.snapshot, msg.stereo ?? null, performance.now());
        this.logJitter(msg.snapshot, msg.fillMin, msg.fillMax);
      } else if (msg && msg.type === 'event') {
        const ev: PlayoutEvent = {
          kind: msg.kind,
          fillFrames: msg.fillFrames,
          reads: msg.reads,
          atMs: performance.now(),
        };
        this.events.push(ev);
        if (this.events.length > EVENT_RING_CAP) this.events.shift();
        if (this.opts.debug) {
          const srMs = this.opts.sampleRate / 1000;
          console.log(
            `[JBUF-EV] ${ev.kind} fill=${(ev.fillFrames / srMs).toFixed(1)}ms reads=${ev.reads} t=${ev.atMs.toFixed(0)}`
          );
        }
      }
    };
  }

  /**
   * Buffer discontinuities since start (bounded ring, newest last).
   * Each is an audible-artefact candidate: underrun = silence gap,
   * overrun/lap = snap, insert/drop = ±1 splice.
   */
  getEvents(): PlayoutEvent[] {
    return [...this.events];
  }

  clearEvents(): void {
    this.events = [];
  }

  /**
   * Per-window stats callback (latency harnesses; null to remove).
   * Fires once per worklet stats window with its receipt timestamp.
   */
  setStatsListener(listener: SinkStatsListener | null): void {
    this.statsListener = listener;
  }

  /**
   * Builds the audio graph and the shared ring. Requires cross-origin
   * isolation (the SDK's hard requirement) — throws otherwise. Must be
   * called from a user-gesture context on browsers that require one for
   * AudioContext.
   */
  static async create(options: SinkPlayerOptions = {}): Promise<SinkPlayer> {
    if (typeof SharedArrayBuffer === 'undefined' || globalThis.crossOriginIsolated !== true) {
      throw new MoqClientError(
        'Sink playout requires cross-origin isolation (crossOriginIsolated) for the SharedArrayBuffer ring',
        'NOT_ISOLATED'
      );
    }
    const opts = {
      sampleRate: options.sampleRate ?? 48000,
      channelCount: options.channelCount ?? 2,
      writerFrameSamples: options.writerFrameSamples ?? DEFAULT_WRITER_FRAME,
      redundancy: options.redundancy ?? 0,
      debug: options.debug ?? false,
    };

    const audioContext = new AudioContext({
      sampleRate: opts.sampleRate,
      latencyHint: options.latencyHint ?? 'interactive',
    });
    try {
      const url = createPlayoutWorkletUrl();
      try {
        await audioContext.audioWorklet.addModule(url);
      } finally {
        URL.revokeObjectURL(url);
      }

      // v4 geometry: R = render quantum, W = server Opus frame. The SAME
      // base config goes to the worker so its writer view computes an
      // identical capacity.
      const jbConfigBase: JitterBufferCoreConfig = {
        sampleRate: audioContext.sampleRate,
        numChannels: opts.channelCount,
        readerFrame: RENDER_QUANTUM,
        writerFrame: opts.writerFrameSamples,
        ...(options.qualityLevel !== undefined ? { qualityLevel: options.qualityLevel } : {}),
        ...options.jitterConfig,
      };
      // The declared redundancy's FEC floor composes by max with any
      // explicit floor here, and with the quality level's floor inside
      // the core (floors compose, never stack — Go engine.AddSource).
      if (opts.redundancy > 0) {
        jbConfigBase.floor = Math.max(jbConfigBase.floor ?? 0, fecFloor(opts.redundancy, opts.writerFrameSamples));
      }
      const { capacity, nc } = computeJitterCapacity(jbConfigBase);
      const sharedStorage = new Float32Array(new SharedArrayBuffer(capacity * nc * 4));
      const sharedWritePos = new BigInt64Array(new SharedArrayBuffer(8));

      const workletNode = new AudioWorkletNode(audioContext, PLAYOUT_PROCESSOR_NAME, {
        numberOfInputs: 0,
        numberOfOutputs: 1,
        outputChannelCount: [opts.channelCount],
        processorOptions: {
          config: { ...jbConfigBase, sharedStorage, sharedWritePos },
          ...(options.statsEvery !== undefined ? { statsEvery: options.statsEvery } : {}),
        } satisfies PlayoutProcessorOptions,
      });
      const gainNode = audioContext.createGain();
      gainNode.connect(audioContext.destination);
      workletNode.connect(gainNode);

      return new SinkPlayer(audioContext, workletNode, gainNode, jbConfigBase, sharedStorage, sharedWritePos, opts);
    } catch (e) {
      await audioContext.close().catch(() => {});
      throw e;
    }
  }

  /** The shared ring + geometry for the worker's `setSinkTrack`. */
  getRingHandoff(): SinkRingHandoff {
    return {
      jbufConfig: this.jbConfigBase,
      sharedStorage: this.sharedStorage,
      sharedWritePos: this.sharedWritePos,
      redundancy: this.opts.redundancy,
    };
  }

  /** The decoder config the worker should use for this player's sink. */
  getDecoderConfig(frameDurationUs = 5000): SinkAudioConfig {
    return {
      codec: 'opus',
      sampleRate: this.opts.sampleRate,
      numberOfChannels: this.opts.channelCount,
      frameDurationUs,
    };
  }

  /** Resume the AudioContext (autoplay policies suspend it pre-gesture). */
  async resume(): Promise<void> {
    if (this.audioContext.state === 'suspended') {
      await this.audioContext.resume();
    }
  }

  setVolume(volume: number): void {
    this.gainNode.gain.value = Math.max(0, Math.min(1, volume));
  }

  getVolume(): number {
    return this.gainNode.gain.value;
  }

  /** Latest v4 jitter snapshot from the worklet, or null before the first. */
  getJitterStats(): JitterBufferSnapshot | null {
    return this.lastSnapshot ? { ...this.lastSnapshot } : null;
  }

  /** Latest Tap B window (stereo-ness of the rendered output), or null. */
  getTapB(): StereoMeterReport | null {
    return this.lastTapB ? { ...this.lastTapB } : null;
  }

  /** Channel-count-relevant graph state (stereo diagnostics). */
  getAudioGraphReport(): AudioGraphReport {
    const ctx = this.audioContext;
    const dest = ctx.destination;
    const extCtx = ctx as AudioContext & { outputLatency?: number };
    return {
      context: {
        sampleRate: ctx.sampleRate,
        state: ctx.state,
        baseLatencyMs: typeof ctx.baseLatency === 'number' ? ctx.baseLatency * 1000 : null,
        outputLatencyMs: typeof extCtx.outputLatency === 'number' ? extCtx.outputLatency * 1000 : null,
      },
      destination: {
        channelCount: dest.channelCount,
        maxChannelCount: dest.maxChannelCount,
        channelCountMode: dest.channelCountMode,
        channelInterpretation: dest.channelInterpretation,
      },
      worklet: {
        outputChannelCount: this.opts.channelCount,
        channelCount: this.workletNode.channelCount,
        channelCountMode: this.workletNode.channelCountMode,
        channelInterpretation: this.workletNode.channelInterpretation,
      },
      ring: {
        writerFrameSamples: this.opts.writerFrameSamples,
        numChannels: this.opts.channelCount,
        capacityFrames: computeJitterCapacity(this.jbConfigBase).capacity,
      },
    };
  }

  async dispose(): Promise<void> {
    this.workletNode.port.onmessage = null;
    try {
      this.workletNode.disconnect();
    } catch {
      // already disconnected
    }
    this.gainNode.disconnect();
    await this.audioContext.close().catch(() => {});
    this.lastSnapshot = null;
    this.lastTapB = null;
  }

  /**
   * One-line [JBUF] observation log — the browser analog of the Go
   * server's [JBUF] tuning line. Gated by `debug`; throttled to ~1/s
   * (the worklet posts stats ~4/s). Filter devtools by "JBUF".
   */
  private logJitter(s: JitterBufferSnapshot, fillMin?: number, fillMax?: number): void {
    if (!this.opts.debug) return;
    if (this.jbufLogCount++ % 4 !== 0) return;
    const srMs = this.opts.sampleRate / 1000;
    const swing =
      fillMin !== undefined && fillMax !== undefined
        ? ` swing=${(fillMin / srMs).toFixed(1)}-${(fillMax / srMs).toFixed(1)}ms`
        : '';
    console.log(
      `[JBUF] fill=${s.fillMs.toFixed(1)}ms${swing} sp=${s.setpointMs.toFixed(1)} ` +
        `wl=${s.widthLowMs.toFixed(1)} wh=${s.widthHighMs.toFixed(1)} rate=${s.ratePerSec.toFixed(2)}/s${s.frozen ? ' FROZEN' : ''} ` +
        `und=${s.underruns} ovr=${s.overruns} lap=${s.laps} trim=${s.trims} ins=${s.samplesInserted} drop=${s.samplesDropped}`
    );
  }
}

/**
 * Probe the OUTPUT device's native sample rate (ported from the old
 * SDK's stereo-diagnostics).
 *
 * The playout AudioContext requests 48 kHz explicitly, so its
 * `sampleRate` never reflects the device. A throwaway default-rate
 * context inherits the current output device's rate — a Bluetooth
 * headset stuck in HFP typically reports 16–24 kHz instead of
 * 44.1/48 k. This is the only in-browser signal for the "everything
 * says stereo but it SOUNDS mono" state: HFP collapse happens in the
 * OS below every Web Audio observable. Heuristic — a low rate strongly
 * suggests HFP; a normal rate does not fully rule it out.
 *
 * Returns null where AudioContext is unavailable or construction fails.
 */
export async function probeOutputDeviceSampleRate(): Promise<number | null> {
  if (typeof AudioContext === 'undefined') return null;
  try {
    const ctx = new AudioContext(); // no sampleRate option → device rate
    const rate = ctx.sampleRate;
    await ctx.close();
    return rate;
  } catch {
    return null;
  }
}
