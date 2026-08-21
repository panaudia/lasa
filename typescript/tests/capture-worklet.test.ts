/**
 * Tests the capture AudioWorklet WITHOUT a browser by evaluating the generated worklet
 * source in a stubbed AudioWorkletGlobalScope (exactly what the browser does with the
 * Blob). Validates the `.toString()` serialization of CaptureRing and the processor's
 * write + signal logic. SAB-backed views let `Atomics.add`/`notify` run natively; a
 * second (consumer) CaptureRing over the same memory reads back what the worklet wrote.
 * The live in-browser smoke test comes with the test page (Phase 7).
 */

import { describe, it, expect } from 'vitest';
import {
  buildCaptureWorkletCode,
  CAPTURE_PROCESSOR_NAME,
  type CaptureProcessorOptions,
} from '../src/audio/capture-worklet.js';
import { CaptureRing } from '../src/audio/capture-ring.js';

interface WorkletProcessor {
  process(inputs: Float32Array[][]): boolean;
}

/** Shared SAB views + a consumer ring over the same memory, for round-trip checks. */
function makeShared(nc: number, capacityFrames: number) {
  const sharedStorage = new Float32Array(new SharedArrayBuffer(4 * capacityFrames * nc));
  const sharedWritePos = new BigInt64Array(new SharedArrayBuffer(8));
  const sharedReadPos = new BigInt64Array(new SharedArrayBuffer(8));
  const signal = new Int32Array(new SharedArrayBuffer(4));
  const options: { processorOptions: CaptureProcessorOptions } = {
    processorOptions: { numChannels: nc, capacityFrames, sharedStorage, sharedWritePos, sharedReadPos, signal },
  };
  const consumer = new CaptureRing({ numChannels: nc, capacityFrames, sharedStorage, sharedWritePos, sharedReadPos });
  return { options, signal, consumer };
}

/** Eval the worklet source in a stubbed scope and instantiate the processor. */
function instantiate(options: { processorOptions: CaptureProcessorOptions }): WorkletProcessor {
  let RegisteredClass: (new (o: unknown) => WorkletProcessor) | null = null;
  let registeredName = '';

  class FakeAudioWorkletProcessor {
    port = { onmessage: null, postMessage() {} };
  }
  const registerProcessor = (name: string, cls: new (o: unknown) => WorkletProcessor) => {
    registeredName = name;
    RegisteredClass = cls;
  };

  const code = buildCaptureWorkletCode();
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  new Function('AudioWorkletProcessor', 'registerProcessor', code)(FakeAudioWorkletProcessor, registerProcessor);

  expect(registeredName).toBe(CAPTURE_PROCESSOR_NAME);
  expect(RegisteredClass).not.toBeNull();
  return new RegisteredClass!(options);
}

/** Planar quantum: channel `ch` frame `i` = base + i + ch*0.25. */
function quantum(nFrames: number, nc: number, base: number): Float32Array[] {
  const planar: Float32Array[] = [];
  for (let ch = 0; ch < nc; ch++) {
    const a = new Float32Array(nFrames);
    for (let i = 0; i < nFrames; i++) a[i] = base + i + ch * 0.25;
    planar.push(a);
  }
  return planar;
}

describe('capture worklet', () => {
  it('the generated source is self-contained and registers the processor', () => {
    const code = buildCaptureWorkletCode();
    // Bound to a const (robust to anonymous- or named-class emit).
    expect(code).toContain('const CaptureRing = class');
    expect(code).toContain('class CaptureRingProcessor extends AudioWorkletProcessor');
    expect(code).toContain(`registerProcessor("${CAPTURE_PROCESSOR_NAME}"`);
    expect(code).not.toContain('__publicField');
    expect(code).not.toContain('import ');
    expect(code).not.toContain('export ');
    const { options } = makeShared(1, 256);
    expect(() => instantiate(options)).not.toThrow();
  });

  it('interleaves an input quantum into the ring and signals the worker', () => {
    const { options, signal, consumer } = makeShared(2, 64);
    const p = instantiate(options);

    // process(inputs): inputs[0] is the planar input.
    expect(p.process([quantum(4, 2, 10)])).toBe(true);

    // The worker would have been woken exactly once.
    expect(Atomics.load(signal, 0)).toBe(1);

    // And the data is in the ring, interleaved, readable by the consumer.
    const dst = new Float32Array(8);
    expect(consumer.drain(dst)).toBe(8);
    expect(Array.from(dst)).toEqual([10, 10.25, 11, 11.25, 12, 12.25, 13, 13.25]);
  });

  it('signals once per written quantum across several quanta (mono)', () => {
    const { options, signal, consumer } = makeShared(1, 1024);
    const p = instantiate(options);
    p.process([quantum(128, 1, 0)]);
    p.process([quantum(128, 1, 128)]);
    p.process([quantum(128, 1, 256)]);
    expect(Atomics.load(signal, 0)).toBe(3);

    const dst = new Float32Array(512);
    expect(consumer.drain(dst)).toBe(384);
    for (let i = 0; i < 384; i++) expect(dst[i]).toBe(i);
  });

  it('does not write or signal when no input is connected', () => {
    const { options, signal, consumer } = makeShared(1, 64);
    const p = instantiate(options);
    expect(p.process([[]])).toBe(true); // inputs[0] = [] (no channels)
    expect(p.process([])).toBe(true); // inputs = [] (no input)
    expect(Atomics.load(signal, 0)).toBe(0);
    expect(consumer.fillFrames()).toBe(0);
  });

  it('does not signal when the write overflows (consumer stalled)', () => {
    // cap = 256 frames holds exactly two 128-frame quanta; the third overflows.
    const { options, signal } = makeShared(1, 256);
    const p = instantiate(options);
    p.process([quantum(128, 1, 0)]); // ok → signal 1
    p.process([quantum(128, 1, 128)]); // ok (fills to cap) → signal 2
    p.process([quantum(128, 1, 256)]); // overflow → dropped, no signal
    expect(Atomics.load(signal, 0)).toBe(2);
  });
});
