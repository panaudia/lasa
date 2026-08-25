/**
 * CapturePipeline — the main-thread half of one entity's send path
 * (per-entity MONO pipelines, rulings 2026-08-05: a DAW's multi-track
 * output is N entities, stereo playback capture is 2; any source node
 * feeds a pipeline through a ChannelSplitter, mic being the N=1 case).
 *
 * Owns the Web Audio side only: source → splitter[channelIndex] →
 * capture AudioWorklet, which interleaves each 128-sample quantum into
 * the SAB ring and wakes the MOQ worker (Atomics.notify). Encode +
 * framing + send live in the worker (CaptureEncoder); the pose rides
 * the shared PoseCell, written here via {@link setPose} and read by the
 * worker at datagram-framing time (the freshness rule). Main never
 * touches mic PCM.
 *
 * The caller owns the AudioContext (one context can feed N pipelines);
 * dispose() removes this pipeline's nodes only.
 */

import { MoqClientError } from '../transport/errors.js';
import { createCaptureWorkletUrl, CAPTURE_PROCESSOR_NAME, type CaptureProcessorOptions } from './capture-worklet.js';
import { captureCapacityFrames } from './capture-ring.js';
import { allocatePoseCell, PoseCell, type PoseCellViews } from './pose-cell.js';
import type { Pose } from '../lasa/wire.js';

/** Everything the worker needs to run this pipeline's encode half. */
export interface CaptureHandoff {
  numChannels: number;
  capacityFrames: number;
  sharedStorage: Float32Array;
  sharedWritePos: BigInt64Array;
  sharedReadPos: BigInt64Array;
  sharedSignal: Int32Array;
  poseSeq: Int32Array;
  poseValues: Float64Array;
}

export interface CapturePipelineOptions {
  /**
   * Which channel of the source feeds this (mono) pipeline. Default 0.
   * A stereo source becomes two pipelines with indices 0 and 1; a DAW
   * multi-channel source one pipeline per track.
   */
  channelIndex?: number;
}

// addModule() registers the processor name context-wide; a second call
// for the same context must be skipped (re-registration throws).
/**
 * One addModule per context, keyed by the in-flight promise rather than
 * a done flag: two concurrent create() calls on one context (the
 * documented stereo-source usage) must share the load, or the second
 * registerProcessor throws on the duplicate name.
 */
const workletLoading = new WeakMap<BaseAudioContext, Promise<void>>();

function ensureCaptureWorklet(ctx: BaseAudioContext): Promise<void> {
  let p = workletLoading.get(ctx);
  if (!p) {
    const url = createCaptureWorkletUrl();
    p = ctx.audioWorklet.addModule(url).finally(() => URL.revokeObjectURL(url));
    // A failed load must not poison the context for later attempts.
    p.catch(() => workletLoading.delete(ctx));
    workletLoading.set(ctx, p);
  }
  return p;
}

/**
 * The page half of an entity's capture: a worklet writing a shared
 * ring the transport worker encodes from, plus the shared pose cell.
 * {@link LasaClient.startCapture} builds one; build directly for a
 * custom graph.
 */
export class CapturePipeline {
  /** Quanta dropped by the capture ring (worker stalled) — upstream gaps. */
  private overflowCount = 0;

  private constructor(
    private readonly source: AudioNode,
    private readonly splitter: ChannelSplitterNode,
    private readonly workletNode: AudioWorkletNode,
    private readonly ringViews: {
      sharedStorage: Float32Array;
      sharedWritePos: BigInt64Array;
      sharedReadPos: BigInt64Array;
      sharedSignal: Int32Array;
    },
    private readonly poseViews: PoseCellViews,
    private readonly poseWriter: PoseCell,
    readonly capacityFrames: number
  ) {}

  /**
   * Builds the capture graph for one entity off `source` (any AudioNode
   * — MediaStreamAudioSourceNode for a mic, OscillatorNode in tests,
   * a DAW input channel…). Requires cross-origin isolation.
   */
  static async create(source: AudioNode, options: CapturePipelineOptions = {}): Promise<CapturePipeline> {
    if (typeof SharedArrayBuffer === 'undefined' || globalThis.crossOriginIsolated !== true) {
      throw new MoqClientError(
        'Capture requires cross-origin isolation (crossOriginIsolated) for the SharedArrayBuffer ring',
        'NOT_ISOLATED'
      );
    }
    const ctx = source.context;
    // The wire format is Opus at 48 kHz (lasa-core.md §5). The encoder
    // is configured at the context's rate, so anything else would ship
    // non-conformant audio (or rely on a browser's silent resampling).
    if (ctx.sampleRate !== 48000) {
      throw new MoqClientError(
        `Capture requires a 48 kHz AudioContext (got ${ctx.sampleRate} Hz): construct it with new AudioContext({ sampleRate: 48000 })`,
        'SAMPLE_RATE'
      );
    }
    const channelIndex = options.channelIndex ?? 0;

    await ensureCaptureWorklet(ctx);

    const capacityFrames = captureCapacityFrames();
    const ringViews = {
      sharedStorage: new Float32Array(new SharedArrayBuffer(capacityFrames * 4)), // nc = 1
      sharedWritePos: new BigInt64Array(new SharedArrayBuffer(8)),
      sharedReadPos: new BigInt64Array(new SharedArrayBuffer(8)),
      sharedSignal: new Int32Array(new SharedArrayBuffer(4)),
    };
    const poseViews = allocatePoseCell(); // zero = home pose, already stable

    const workletNode = new AudioWorkletNode(ctx as AudioContext, CAPTURE_PROCESSOR_NAME, {
      numberOfInputs: 1,
      numberOfOutputs: 0,
      processorOptions: {
        numChannels: 1,
        capacityFrames,
        sharedStorage: ringViews.sharedStorage,
        sharedWritePos: ringViews.sharedWritePos,
        sharedReadPos: ringViews.sharedReadPos,
        signal: ringViews.sharedSignal,
      } satisfies CaptureProcessorOptions,
    });

    // source → splitter → [channelIndex] → worklet (a sink node).
    const splitter = ctx.createChannelSplitter(Math.max(1, channelIndex + 1));
    source.connect(splitter);
    splitter.connect(workletNode, channelIndex, 0);

    const pipeline = new CapturePipeline(
      source,
      splitter,
      workletNode,
      ringViews,
      poseViews,
      new PoseCell(poseViews),
      capacityFrames
    );
    // The worklet's port carries only the overflow watch (rare).
    workletNode.port.onmessage = (e: MessageEvent) => {
      const d = e.data as { type?: string; overflows?: number };
      if (d?.type === 'overflow' && typeof d.overflows === 'number') {
        pipeline.overflowCount = d.overflows;
      }
    };
    return pipeline;
  }

  /** Capture-ring overflows so far (each = one dropped quantum = an upstream gap). */
  getOverflows(): number {
    return this.overflowCount;
  }

  /**
   * Publish the entity's freshest pose (seqlock write — wait-free, safe
   * at any rate: sensor callbacks, rAF, XR frames). The next framed
   * packet carries it.
   */
  setPose(pose: Pose): void {
    this.poseWriter.write(pose);
  }

  /** The shared cells for the worker's `setCaptureTrack`. */
  getHandoff(): CaptureHandoff {
    return {
      numChannels: 1,
      capacityFrames: this.capacityFrames,
      ...this.ringViews,
      poseSeq: this.poseViews.seq,
      poseValues: this.poseViews.values,
    };
  }

  /** Sample rate of the owning context (the encoder must match it). */
  get sampleRate(): number {
    return this.source.context.sampleRate;
  }

  /** Remove this pipeline's nodes (the caller's source/context survive). */
  dispose(): void {
    // Tell the processor to return false from its next process() call;
    // disconnecting alone leaves it running for the context's lifetime.
    try {
      this.workletNode.port.postMessage({ type: 'stop' });
    } catch {
      // port already closed
    }
    try {
      this.source.disconnect(this.splitter);
    } catch {
      // already disconnected
    }
    try {
      this.splitter.disconnect();
    } catch {
      // already disconnected
    }
    try {
      this.workletNode.disconnect();
    } catch {
      // sink node — nothing to disconnect, but be thorough
    }
  }
}
