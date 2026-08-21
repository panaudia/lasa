/**
 * capture-worklet.ts — the AudioWorklet wrapper around {@link CaptureRing} for the
 * mic SEND path (design: worker-capture-design.md §6/§6.1, plan Phase 3).
 *
 * This is the PRODUCER side, and it is deliberately trivial and real-time-safe: each
 * 128-sample render quantum is interleaved straight into the SAB ring and — if data
 * was written — the worker is woken via the clock-free signal (`Atomics.add` +
 * `Atomics.notify` on a shared Int32 cell, design §6.1). It does **no** Opus encode,
 * **no** frame-boundary logic, and **no** `postMessage` of PCM. Encode + framing +
 * send all live in the MOQ worker, which drains the ring (CaptureRing.drain).
 *
 * **Build-free loading.** A worklet loaded via Blob URL cannot `import` app modules at
 * runtime, so the (self-contained) {@link CaptureRing} class is serialized via
 * `.toString()` and concatenated with the processor source into one Blob — the same
 * pattern as the playout worklet. CaptureRing assigns its fields in the constructor
 * (no class-field initializers), so this serialization stays valid under any build
 * target (no `__publicField` helper to leak module scope).
 */

import { CaptureRing } from './capture-ring.js';

/** The name the processor registers under / that `AudioWorkletNode` references. */
export const CAPTURE_PROCESSOR_NAME = 'capture-processor';

/**
 * `processorOptions` passed to the worklet via `new AudioWorkletNode(...)`. The SAB
 * views are created on main (sized via `captureCapacityFrames()`) and handed to both
 * this worklet and the MOQ worker so they share one ring. The `signal` cell is the
 * §6.1 wake channel: this worklet notifies it, the worker `Atomics.waitAsync`es on it.
 */
export interface CaptureProcessorOptions {
  /** Channels per frame (interleaved). Matches the mic capture channel count. */
  numChannels: number;
  /** Ring capacity in frames (from `captureCapacityFrames()`). */
  capacityFrames: number;
  /** SAB-backed interleaved float ring storage (`capacityFrames * numChannels`). */
  sharedStorage: Float32Array;
  /** SAB-backed length-1 BigInt64Array — cumulative producer position (frames). */
  sharedWritePos: BigInt64Array;
  /** SAB-backed length-1 BigInt64Array — cumulative consumer position (frames). */
  sharedReadPos: BigInt64Array;
  /** SAB-backed length-1 Int32Array — the wake signal the worker waits on. */
  signal: Int32Array;
}

/**
 * The processor source, as plain JS text (it runs in AudioWorkletGlobalScope and
 * references `AudioWorkletProcessor` / `registerProcessor` / `Atomics`, so it cannot
 * be a normal imported module). It depends only on `CaptureRing`, which is prepended
 * by {@link buildCaptureWorkletCode}.
 */
const CAPTURE_PROCESSOR_SOURCE = `
class CaptureRingProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const opts = (options && options.processorOptions) || {};
    this.signal = opts.signal;
    this.ring = new CaptureRing({
      numChannels: opts.numChannels,
      capacityFrames: opts.capacityFrames,
      sharedStorage: opts.sharedStorage,
      sharedWritePos: opts.sharedWritePos,
      sharedReadPos: opts.sharedReadPos,
    });
  }

  // PRODUCER: interleave the input quantum into the ring; wake the worker if we wrote.
  // inputs[0] is the planar input (array of per-channel Float32Arrays); empty when no
  // source is connected. CaptureRing.write guards the empty/overflow cases.
  process(inputs) {
    const input = inputs[0];
    if (input && this.ring.write(input)) {
      // Clock-free wake (design §6.1): bump + notify the signal cell. One bounded
      // futex wake, one waiter (the MOQ worker's Atomics.waitAsync). Real-time-safe:
      // no allocation, no lock, the caller never blocks.
      Atomics.add(this.signal, 0, 1);
      Atomics.notify(this.signal, 0, 1);
    }
    // Overflow watch (debug surface): an overflow means the consumer
    // stalled and a whole quantum was dropped — an upstream audio gap.
    // Posts only on change; a healthy pipeline posts never.
    if (this.ring.overflows !== (this.prevOverflows || 0)) {
      this.prevOverflows = this.ring.overflows;
      this.port.postMessage({ type: 'overflow', overflows: this.ring.overflows });
    }
    return true; // keep the processor alive
  }
}
registerProcessor(${JSON.stringify(CAPTURE_PROCESSOR_NAME)}, CaptureRingProcessor);
`;

/**
 * Build the full worklet module source: the serialized {@link CaptureRing} class
 * followed by the processor. Self-contained — no imports, no free references — so it
 * loads from a Blob URL in any browser. Mirrors {@link buildPlayoutWorkletCode}.
 */
export function buildCaptureWorkletCode(): string {
  const coreSource = CaptureRing.toString();
  if (!coreSource.startsWith('class')) {
    // Guards against a bundler wrapping the class such that .toString() is not
    // self-contained source (would break the worklet). Surfaces early/loudly.
    throw new Error('capture-worklet: CaptureRing.toString() is not a class declaration');
  }
  // The serialized class must not reference transpiler/bundler helpers that live in
  // module scope (esbuild's `__publicField` for class-field lowering, etc.) — they'd
  // be undefined in the worklet. CaptureRing assigns fields in its constructor to avoid
  // this, but fail loudly if a future build setting reintroduces a helper.
  const helper = /\b__(publicField|privateField|decorateClass|decorateParam|name|esDecorate)\b/.exec(coreSource);
  if (helper) {
    throw new Error(
      `capture-worklet: serialized CaptureRing references the bundler helper "${helper[0]}" — ` +
        'it would be undefined in the worklet. Ensure the build keeps native class output.'
    );
  }
  // BIND to a const rather than emit the bare source: a consumer's bundler may emit the
  // class as an ANONYMOUS expression (`var X = class {…}`), so `.toString()` returns
  // `class {…}` — a syntax error as a statement. Wrapping it is valid for both forms.
  return `const CaptureRing = ${coreSource};\n${CAPTURE_PROCESSOR_SOURCE}`;
}

/**
 * Create a Blob URL for the capture worklet module. Caller passes it to
 * `audioContext.audioWorklet.addModule(url)` and should `URL.revokeObjectURL`
 * afterwards.
 */
export function createCaptureWorkletUrl(): string {
  const blob = new Blob([buildCaptureWorkletCode()], { type: 'application/javascript' });
  return URL.createObjectURL(blob);
}
