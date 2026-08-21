/**
 * playout-worklet.ts — the AudioWorklet wrapper around {@link JitterBufferCore}
 * for LASA playout (design lineage: cloud-mixer/plan/history/browser-audio/
 * playout-v3-design.md §5.1 + worker-transport-design.md).
 *
 * SAB-ONLY (LASA divergence from the old-SDK copy): the ring is a
 * SharedArrayBuffer written by the MOQ worker (decode thread) and read here by
 * `process()` at the render quantum — cross-origin isolation is a hard
 * requirement of this SDK, so the old postMessage-PCM and transferred-pcmPort
 * fallbacks are gone. The worklet's `port` is outbound-only (stats). The buffer
 * holds interleaved frames; `process()` deinterleaves the 128-frame block into
 * the per-channel `outputs[0][ch]` (Web Audio is always planar), keeping the
 * binaural L/R sample-locked.
 *
 * **Build-free loading.** A worklet loaded via Blob URL cannot `import` app
 * modules at runtime, so we serialize the (self-contained) core class via
 * `.toString()` and concatenate it with the processor source into one Blob — the
 * same pattern as the capture worklet in `opus-encoder.ts`, but with the core's
 * single source of truth preserved (no duplicated logic). The core's constructor
 * inlines its default literals precisely so this serialization stays valid under
 * a minified build (no free module-scope references).
 */

import { JitterBufferCore, type JitterBufferCoreConfig, type JitterBufferSnapshot } from './jitter-buffer-core.js';
import { StereoMeterCore, type StereoMeterReport } from './stereo-meter-core.js';

/** The name the processor registers under / that `AudioWorkletNode` references. */
export const PLAYOUT_PROCESSOR_NAME = 'playout-processor';

/**
 * `processorOptions` passed to the worklet via `new AudioWorkletNode(...)`.
 * `config` MUST carry `sharedStorage` + `sharedWritePos` (the SAB ring the
 * worker writes) — the processor throws otherwise rather than silently playing
 * nothing.
 */
export interface PlayoutProcessorOptions {
  /** Buffer geometry/tuning. The worklet builds a {@link JitterBufferCore} from it. */
  config: JitterBufferCoreConfig;
  /** Emit a stats snapshot every N reads (~250 ms at 48 kHz). Default 94. */
  statsEvery?: number;
}

/**
 * Discontinuity event, posted the moment it happens (not window-sampled)
 * — each of these kinds is an audible artefact candidate: underrun =
 * silence gap, overrun/lap = snap (hard waveform jump), insert/drop =
 * ±1-frame splice (soft, but audible as low-level grit when frequent),
 * trim = the v4 macro-trim (one confirmed-displacement discard of stale
 * audio — a single position jump replacing v3's minutes of drip).
 */
export interface PlayoutEventMessage {
  type: 'event';
  kind: 'underrun' | 'overrun' | 'lap' | 'insert' | 'drop' | 'trim';
  /** Fill AFTER the read that triggered the event, frames. */
  fillFrames: number;
  /** Reads since the processor started (the worklet's clock). */
  reads: number;
}

/** Message posted from the worklet to the main thread every `statsEvery` reads. */
export interface PlayoutStatsMessage {
  type: 'stats';
  snapshot: JitterBufferSnapshot;
  /** True fill min/max (frames) across the window — every read, not point-sampled. */
  fillMin?: number;
  fillMax?: number;
  /**
   * Tap B (plan/stereo-diagnostics): stereo-ness of the RENDERED output over
   * the same window — measured on `outputs[0]` after deinterleave, i.e. what
   * the graph delivers toward the destination.
   */
  stereo?: StereoMeterReport;
}

/**
 * The processor source, as plain JS text (it runs in AudioWorkletGlobalScope and
 * references `AudioWorkletProcessor` / `registerProcessor`, so it cannot be a
 * normal imported module). It depends only on `JitterBufferCore`, which is
 * prepended by {@link buildPlayoutWorkletCode}. ~250 ms ≈ 94 reads at the
 * 128-sample / 48 kHz quantum.
 */
const PLAYOUT_PROCESSOR_SOURCE = `
class PlayoutRingProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const opts = (options && options.processorOptions) || {};
    const config = opts.config || {};
    // SAB-only: the ring is written by the MOQ worker through shared memory.
    // Fail loudly at construction rather than render silence forever.
    if (!config.sharedStorage || !config.sharedWritePos) {
      throw new Error('PlayoutRingProcessor: config.sharedStorage + sharedWritePos are required (SAB-only playout)');
    }
    this.core = new JitterBufferCore(config);
    this.nc = this.core.nc;
    this.statsEvery = opts.statsEvery || 94;
    this.readsSinceStats = 0;
    // CLOCKTEST: true fill min/max across the stats window (every read, not the
    // 250ms point-sample) — reveals the real sawtooth amplitude from reader-burst.
    this.winFillMin = 1e9;
    this.winFillMax = 0;
    this.scratch = new Float32Array(128 * this.nc);
    // Tap B stereo meter over the rendered output (3 multiply-adds per frame —
    // negligible, so it is unconditionally on; the main thread decides usage).
    this.meter = new StereoMeterCore();
    // Discontinuity-event detection: previous counter values, diffed
    // after every read so events post the moment they happen.
    this.totalReads = 0;
    this.prevUnderruns = 0;
    this.prevOverruns = 0;
    this.prevLaps = 0;
    this.prevTrims = 0;
    this.prevInserted = 0;
    this.prevDropped = 0;
    // this.port is OUTBOUND only (stats); no PCM ever arrives via messages.
  }

  // Post any counter increments since the last read as events (rare —
  // a healthy stream posts none; this is not a per-read allocation path
  // in the steady state).
  postEvents() {
    const c = this.core;
    const fill = c.fillFrames();
    const emit = (kind, n) => {
      for (let i = 0; i < n; i++) {
        this.port.postMessage({ type: 'event', kind, fillFrames: fill, reads: this.totalReads });
      }
    };
    if (c.underruns !== this.prevUnderruns) emit('underrun', c.underruns - this.prevUnderruns);
    if (c.overruns !== this.prevOverruns) emit('overrun', c.overruns - this.prevOverruns);
    if (c.laps !== this.prevLaps) emit('lap', c.laps - this.prevLaps);
    if (c.trims !== this.prevTrims) emit('trim', c.trims - this.prevTrims);
    if (c.samplesInserted !== this.prevInserted) emit('insert', c.samplesInserted - this.prevInserted);
    if (c.samplesDropped !== this.prevDropped) emit('drop', c.samplesDropped - this.prevDropped);
    this.prevUnderruns = c.underruns;
    this.prevOverruns = c.overruns;
    this.prevLaps = c.laps;
    this.prevTrims = c.trims;
    this.prevInserted = c.samplesInserted;
    this.prevDropped = c.samplesDropped;
  }

  // READER: pull one render quantum from the ring, deinterleave to outputs.
  process(_inputs, outputs) {
    const out = outputs[0];
    if (!out || out.length === 0 || !out[0]) return true;
    const nFrames = out[0].length;
    const nc = this.nc;
    const need = nFrames * nc;
    if (this.scratch.length < need) this.scratch = new Float32Array(need);
    const block = this.scratch.subarray(0, need);
    // core.read zeroes the block on startup/underrun, so silence falls through.
    this.core.read(block);
    this.totalReads++;
    this.postEvents();
    const fill = this.core.fillFrames();
    if (fill < this.winFillMin) this.winFillMin = fill;
    if (fill > this.winFillMax) this.winFillMax = fill;
    for (let ch = 0; ch < out.length; ch++) {
      const dst = out[ch];
      const srcCh = ch < nc ? ch : nc - 1;
      for (let i = 0; i < nFrames; i++) dst[i] = block[i * nc + srcCh];
    }
    // Tap B: meter the planar output as rendered (mono if the node only has one
    // output channel — that itself is a finding).
    this.meter.writePlanar(out[0], out.length > 1 ? out[1] : null, nFrames);
    if (++this.readsSinceStats >= this.statsEvery) {
      this.readsSinceStats = 0;
      this.port.postMessage({
        type: 'stats',
        snapshot: this.core.snapshot(),
        fillMin: this.winFillMin,
        fillMax: this.winFillMax,
        stereo: this.meter.snapshotAndReset(),
      });
      this.winFillMin = 1e9;
      this.winFillMax = 0;
    }
    return true;
  }
}
registerProcessor(${JSON.stringify(PLAYOUT_PROCESSOR_NAME)}, PlayoutRingProcessor);
`;

/**
 * Build the full worklet module source: the serialized {@link JitterBufferCore}
 * class followed by the processor. Self-contained — no imports, no free
 * references — so it loads from a Blob URL in any browser.
 */
export function buildPlayoutWorkletCode(): string {
  const coreSource = JitterBufferCore.toString();
  const meterSource = StereoMeterCore.toString();
  if (!coreSource.startsWith('class')) {
    // Guards against a bundler wrapping the class such that .toString() is not
    // self-contained source (would break the worklet). Surfaces early/loudly.
    throw new Error('playout-worklet: JitterBufferCore.toString() is not a class declaration');
  }
  if (!meterSource.startsWith('class')) {
    throw new Error('playout-worklet: StereoMeterCore.toString() is not a class declaration');
  }
  // The serialized class must not reference transpiler/bundler helpers that live
  // in module scope (e.g. esbuild's `__publicField` for class-field lowering, or
  // `__decorateClass`) — they'd be undefined in the worklet. The build targets
  // es2022 (vite.config) specifically to keep class fields native. If a future
  // build setting reintroduces a helper, fail loudly here rather than ship a
  // worklet that throws at runtime.
  const helper = /\b__(publicField|privateField|decorateClass|decorateParam|name|esDecorate)\b/.exec(
    coreSource + meterSource
  );
  if (helper) {
    throw new Error(
      `playout-worklet: serialized source references the bundler helper "${helper[0]}" — ` +
        'it would be undefined in the worklet. Ensure the build target keeps native class fields (es2022+).'
    );
  }
  // BIND to a const rather than emit the bare source: a consumer's bundler may
  // emit the class as an ANONYMOUS expression (`var X = class {…}`), so
  // `.toString()` returns `class {…}` — a syntax error as a statement. Wrapping
  // it (`const JitterBufferCore = class {…};`) is valid for both the anonymous
  // and the named-declaration forms.
  return `const JitterBufferCore = ${coreSource};\nconst StereoMeterCore = ${meterSource};\n${PLAYOUT_PROCESSOR_SOURCE}`;
}

/**
 * Create a Blob URL for the playout worklet module. Caller passes it to
 * `audioContext.audioWorklet.addModule(url)` and should `URL.revokeObjectURL`
 * afterwards.
 */
export function createPlayoutWorkletUrl(): string {
  const blob = new Blob([buildPlayoutWorkletCode()], { type: 'application/javascript' });
  return URL.createObjectURL(blob);
}
