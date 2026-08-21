/**
 * Tests the playout AudioWorklet WITHOUT a browser by evaluating the generated
 * worklet source in a stubbed AudioWorkletGlobalScope (exactly what the browser
 * does with the Blob). This validates both the `.toString()` serialization
 * (the core must be self-contained) and the processor's read/deinterleave/
 * stats logic.
 *
 * LASA divergence from the old-SDK test: the worklet is SAB-only — PCM is fed
 * through a writer-view JitterBufferCore over the shared ring (the worker's
 * real topology), not via port messages. Off-thread, plain ArrayBuffer-backed
 * typed arrays stand in for the SAB (Atomics work on non-shared arrays too).
 */

import { describe, it, expect } from 'vitest';
import {
  buildPlayoutWorkletCode,
  PLAYOUT_PROCESSOR_NAME,
  type PlayoutProcessorOptions,
  type PlayoutStatsMessage,
} from '../src/audio/playout-worklet.js';
import {
  JitterBufferCore,
  computeJitterCapacity,
  type JitterBufferCoreConfig,
} from '../src/audio/jitter-buffer-core.js';

interface FakePort {
  posted: unknown[];
  postMessage(m: unknown): void;
}

interface WorkletProcessor {
  port: FakePort;
  process(inputs: Float32Array[][], outputs: Float32Array[][]): boolean;
}

/** Eval the worklet source in a stubbed scope and instantiate the processor. */
function instantiate(options: { processorOptions: PlayoutProcessorOptions }): WorkletProcessor {
  let RegisteredClass: (new (o: unknown) => WorkletProcessor) | null = null;
  let registeredName = '';

  class FakeAudioWorkletProcessor {
    port: FakePort;
    constructor() {
      this.port = {
        posted: [],
        postMessage(m: unknown) {
          this.posted.push(m);
        },
      };
    }
  }
  const registerProcessor = (name: string, cls: new (o: unknown) => WorkletProcessor) => {
    registeredName = name;
    RegisteredClass = cls;
  };

  const code = buildPlayoutWorkletCode();
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  new Function('AudioWorkletProcessor', 'registerProcessor', code)(FakeAudioWorkletProcessor, registerProcessor);

  expect(registeredName).toBe(PLAYOUT_PROCESSOR_NAME);
  expect(RegisteredClass).not.toBeNull();
  return new RegisteredClass!(options);
}

// v4 browser geometry: setpoint = spBase = max(R+S, R+W) = 368 at zero
// measured width — the startup placement these wiring tests key on.
const BASE_CFG: JitterBufferCoreConfig = { numChannels: 2, readerFrame: 128, writerFrame: 240 };

/**
 * Build a shared ring for `base`, returning the worklet's processorOptions and
 * a writer-view core over the same storage — the worker's side of the SAB.
 */
function sabRig(base: JitterBufferCoreConfig, statsEvery = 5): {
  options: { processorOptions: PlayoutProcessorOptions };
  writer: JitterBufferCore;
} {
  const { capacity, nc } = computeJitterCapacity(base);
  const config: JitterBufferCoreConfig = {
    ...base,
    sharedStorage: new Float32Array(capacity * nc),
    sharedWritePos: new BigInt64Array(1),
  };
  return {
    options: { processorOptions: { config, statsEvery } },
    writer: new JitterBufferCore(config),
  };
}

/** Fresh stereo output frame: one output, 2 channels of 128 samples. */
function stereoOut(): Float32Array[][] {
  return [[new Float32Array(128), new Float32Array(128)]];
}

/** Interleaved stereo PCM ramp: frame i → L=i, R=i+0.5 (exact in f32). */
function rampPcm(frames: number): Float32Array {
  const pcm = new Float32Array(frames * 2);
  for (let i = 0; i < frames; i++) {
    pcm[i * 2] = i;
    pcm[i * 2 + 1] = i + 0.5;
  }
  return pcm;
}

describe('playout worklet', () => {
  it('the generated source is self-contained and registers the processor', () => {
    const code = buildPlayoutWorkletCode();
    expect(code).toContain('class JitterBufferCore');
    expect(code).toContain('class StereoMeterCore');
    expect(code).toContain('class PlayoutRingProcessor extends AudioWorkletProcessor');
    expect(code).toContain(`registerProcessor("${PLAYOUT_PROCESSOR_NAME}"`);
    // No leftover module references that would be undefined in the worklet.
    expect(code).not.toContain('PLAYOUT_TUNING');
    expect(code).not.toContain('import ');
    expect(code).not.toContain('export ');
    // It must actually evaluate in a bare scope.
    expect(() => instantiate(sabRig(BASE_CFG).options)).not.toThrow();
  });

  it('refuses to construct without the shared ring (SAB-only)', () => {
    expect(() => instantiate({ processorOptions: { config: { ...BASE_CFG } } })).toThrow(/sharedStorage/);
  });

  it('outputs silence before enough audio is buffered (startup)', () => {
    const { options } = sabRig(BASE_CFG);
    const p = instantiate(options);
    const out = stereoOut();
    expect(p.process([], out)).toBe(true);
    expect(Array.from(out[0]![0]!).every((v) => v === 0)).toBe(true);
    expect(Array.from(out[0]![1]!).every((v) => v === 0)).toBe(true);
  });

  it('plays worker-written audio and keeps L/R sample-locked after deinterleave', () => {
    const { options, writer } = sabRig(BASE_CFG);
    const p = instantiate(options);
    // Prime past the setpoint (368 frames) via the writer view — the
    // worker's real write path.
    writer.write(rampPcm(1000));

    const out = stereoOut();
    p.process([], out);
    const left = out[0]![0]!;
    const right = out[0]![1]!;

    // startup placement rp = 1000 - 368 = 632 ⇒ first output frame is ring frame 632.
    expect(left[0]).toBe(632);
    // Every frame: R = L + 0.5 — the deinterleave never decorrelates the channels.
    for (let i = 0; i < 128; i++) {
      expect(right[i]! - left[i]!).toBeCloseTo(0.5, 5);
    }
    // And it actually advanced through the ramp (not all one value).
    expect(left[127]).toBe(632 + 127);
  });

  it('emits a stats snapshot every statsEvery reads', () => {
    const { options, writer } = sabRig(BASE_CFG);
    const p = instantiate(options);
    writer.write(rampPcm(2000));
    for (let i = 0; i < 5; i++) p.process([], stereoOut());

    const stats = p.port.posted.filter((m): m is PlayoutStatsMessage => (m as PlayoutStatsMessage).type === 'stats');
    expect(stats.length).toBe(1); // statsEvery = 5 ⇒ exactly one after 5 reads
    const snap = stats[0]!.snapshot;
    expect(snap.started).toBe(true);
    expect(snap.fillFrames).toBeGreaterThan(0);
    expect(snap.setpointFrames).toBe(368); // warm start: spBase, zero measured width
    expect(snap.widthLowFrames).toBe(0); // no window has closed yet
    expect(snap.trims).toBe(0);
  });

  it('includes a Tap B stereo report measuring the rendered output', () => {
    const { options, writer } = sabRig(BASE_CFG);
    const p = instantiate(options);
    // Hard anti-phase content: L=+0.5, R=−0.5 every frame ⇒ corr −1, mid 0.
    // v4's startup placement keeps only setpoint (368) frames of preroll, so
    // the writer must keep feeding across the window (the worker's real
    // topology) for all 5 reads to render audio.
    const pcm = new Float32Array(2000 * 2);
    for (let i = 0; i < 2000; i++) {
      pcm[i * 2] = 0.5;
      pcm[i * 2 + 1] = -0.5;
    }
    writer.write(pcm);
    for (let i = 0; i < 5; i++) {
      p.process([], stereoOut());
      writer.write(pcm.subarray(0, 128 * 2)); // refill what the read consumed
    }

    const stats = p.port.posted.filter((m): m is PlayoutStatsMessage => (m as PlayoutStatsMessage).type === 'stats');
    expect(stats.length).toBe(1);
    const stereo = stats[0]!.stereo!;
    expect(stereo.frames).toBe(5 * 128); // every rendered frame in the window
    expect(stereo.rmsL).toBeCloseTo(0.5, 6);
    expect(stereo.rmsR).toBeCloseTo(0.5, 6);
    expect(stereo.correlation).toBeCloseTo(-1, 6);
    expect(stereo.midRms).toBeLessThan(1e-6);
    expect(stereo.sideRms).toBeCloseTo(0.5, 6);
  });

  it('Tap B reports mono collapse when the node renders a single channel', () => {
    const { options, writer } = sabRig(BASE_CFG);
    const p = instantiate(options);
    writer.write(rampPcm(2000));
    // One output channel only (e.g. a mono destination chain).
    for (let i = 0; i < 5; i++) p.process([], [[new Float32Array(128)]]);

    const stats = p.port.posted.filter((m): m is PlayoutStatsMessage => (m as PlayoutStatsMessage).type === 'stats');
    const stereo = stats[0]!.stereo!;
    expect(stereo.frames).toBe(5 * 128);
    expect(stereo.correlation).toBeCloseTo(1, 6); // L treated as both channels
    expect(stereo.sideRms).toBe(0);
  });

  it('handles mono config too', () => {
    const { options, writer } = sabRig({ ...BASE_CFG, numChannels: 1 });
    const p = instantiate(options);
    const mono = new Float32Array(800);
    for (let i = 0; i < mono.length; i++) mono[i] = i;
    writer.write(mono);
    const out: Float32Array[][] = [[new Float32Array(128)]];
    p.process([], out);
    // setpoint 368 ⇒ rp = 800 - 368 = 432; first sample = ring[432] = 432.
    expect(out[0]![0]![0]).toBe(432);
    expect(out[0]![0]![127]).toBe(432 + 127);
  });
});
