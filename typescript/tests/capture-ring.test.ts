/**
 * Tests for CaptureRing — the mic send ring (worker-capture-design.md §5,
 * worker-capture-plan.md P2). Off-thread: plain ArrayBuffer-backed typed arrays stand
 * in for the SAB (Atomics.load/store work on non-shared arrays), so one CaptureRing
 * drives both ends.
 *
 * The bar: a correct, wrap-safe, drain-to-empty SPSC pipe with NO jitter-buffer
 * behaviour. The last test asserts the file carries none of that machinery.
 */

import { describe, it, expect } from 'vitest';
import { CaptureRing, captureCapacityFrames } from '../src/audio/capture-ring.js';

function makeRing(nc: number, capacityFrames: number): CaptureRing {
  return new CaptureRing({
    numChannels: nc,
    capacityFrames,
    sharedStorage: new Float32Array(capacityFrames * nc),
    sharedWritePos: new BigInt64Array(1),
    sharedReadPos: new BigInt64Array(1),
  });
}

/** A planar quantum where channel `ch` frame `i` = base + i + ch*0.25. */
function quantum(nFrames: number, nc: number, base: number): Float32Array[] {
  const planar: Float32Array[] = [];
  for (let ch = 0; ch < nc; ch++) {
    const a = new Float32Array(nFrames);
    for (let i = 0; i < nFrames; i++) a[i] = base + i + ch * 0.25;
    planar.push(a);
  }
  return planar;
}

describe('captureCapacityFrames', () => {
  it('is a flat positive constant (no geometry formula)', () => {
    expect(captureCapacityFrames()).toBe(2048);
    expect(Number.isInteger(captureCapacityFrames())).toBe(true);
  });
});

describe('CaptureRing — interleave + drain', () => {
  it('round-trips one stereo quantum to interleaved output', () => {
    const ring = makeRing(2, 64);
    ring.write(quantum(4, 2, 10)); // L:[10,11,12,13] R:[10.25,11.25,12.25,13.25]
    const dst = new Float32Array(8);
    const n = ring.drain(dst);
    expect(n).toBe(8); // 4 frames * 2ch
    expect(Array.from(dst)).toEqual([10, 10.25, 11, 11.25, 12, 12.25, 13, 13.25]);
    expect(ring.fillFrames()).toBe(0);
  });

  it('accumulates multiple quanta then drains them all in order (mono)', () => {
    const ring = makeRing(1, 1024);
    ring.write(quantum(128, 1, 0));
    ring.write(quantum(128, 1, 128));
    ring.write(quantum(128, 1, 256));
    expect(ring.fillFrames()).toBe(384);
    const dst = new Float32Array(512);
    const n = ring.drain(dst);
    expect(n).toBe(384);
    for (let i = 0; i < 384; i++) expect(dst[i]).toBe(i);
    expect(ring.fillFrames()).toBe(0);
  });

  it('handles wrap-around across the ring boundary', () => {
    const ring = makeRing(1, 10);
    // Fill+drain to advance the positions near the end of the ring.
    ring.write(quantum(8, 1, 0));
    expect(ring.drain(new Float32Array(8))).toBe(8); // rp = wp = 8
    // Next write of 6 frames wraps: ring slots 8,9,0,1,2,3.
    ring.write(quantum(6, 1, 100)); // values 100..105
    const dst = new Float32Array(6);
    expect(ring.drain(dst)).toBe(6);
    expect(Array.from(dst)).toEqual([100, 101, 102, 103, 104, 105]);
    expect(ring.fillFrames()).toBe(0);
  });

  it('drains only whole frames that fit, leaving the remainder (stereo)', () => {
    const ring = makeRing(2, 64);
    ring.write(quantum(5, 2, 0)); // 5 frames available
    const dst = new Float32Array(6); // room for 3 frames
    expect(ring.drain(dst)).toBe(6); // 3 frames
    expect(ring.fillFrames()).toBe(2); // 2 frames remain
    const dst2 = new Float32Array(6);
    expect(ring.drain(dst2)).toBe(4); // remaining 2 frames
    expect(Array.from(dst2.subarray(0, 4))).toEqual([3, 3.25, 4, 4.25]);
    expect(ring.fillFrames()).toBe(0);
  });

  it('drain on an empty ring returns 0', () => {
    const ring = makeRing(1, 16);
    expect(ring.drain(new Float32Array(8))).toBe(0);
    ring.write(quantum(4, 1, 0));
    expect(ring.drain(new Float32Array(8))).toBe(4);
    expect(ring.drain(new Float32Array(8))).toBe(0);
  });
});

describe('CaptureRing — overflow (the only policy)', () => {
  it('drops the whole quantum and counts it when the consumer stalls', () => {
    const ring = makeRing(1, 4);
    expect(ring.write(quantum(4, 1, 0))).toBe(true); // fills to capacity
    expect(ring.fillFrames()).toBe(4);
    expect(ring.write(quantum(1, 1, 99))).toBe(false); // would exceed cap → dropped
    expect(ring.overflows).toBe(1);
    expect(ring.fillFrames()).toBe(4); // unread data untouched
    // Drain frees space; the next write succeeds.
    expect(ring.drain(new Float32Array(4))).toBe(4);
    expect(ring.write(quantum(1, 1, 99))).toBe(true);
    expect(ring.overflows).toBe(1);
  });

  it('allows filling exactly to capacity (full ring is usable)', () => {
    const ring = makeRing(2, 8);
    expect(ring.write(quantum(8, 2, 0))).toBe(true);
    expect(ring.fillFrames()).toBe(8);
    expect(ring.overflows).toBe(0);
  });
});

describe('CaptureRing — drain-to-empty steady state', () => {
  it('returns to ~0 fill after each write/drain cycle (no accumulation)', () => {
    const ring = makeRing(2, 1024);
    const dst = new Float32Array(1024); // big enough to drain everything
    let base = 0;
    for (let cycle = 0; cycle < 50; cycle++) {
      ring.write(quantum(128, 2, base));
      base += 128;
      const n = ring.drain(dst);
      expect(n).toBe(256); // 128 frames * 2ch — exactly one quantum, nothing held
      expect(ring.fillFrames()).toBe(0);
    }
    expect(ring.overflows).toBe(0);
  });
});

describe('CaptureRing — config validation', () => {
  it('rejects mismatched storage length', () => {
    expect(
      () =>
        new CaptureRing({
          numChannels: 2,
          capacityFrames: 64,
          sharedStorage: new Float32Array(64), // should be 64*2
          sharedWritePos: new BigInt64Array(1),
          sharedReadPos: new BigInt64Array(1),
        })
    ).toThrow(/sharedStorage length/);
  });

  it('rejects empty position cells', () => {
    expect(
      () =>
        new CaptureRing({
          numChannels: 1,
          capacityFrames: 16,
          sharedStorage: new Float32Array(16),
          sharedWritePos: new BigInt64Array(0),
          sharedReadPos: new BigInt64Array(1),
        })
    ).toThrow(/length-1 BigInt64Array/);
  });
});

describe('CaptureRing — serialization self-containment (worklet guard)', () => {
  // The capture worklet (P3) serializes this class via .toString() into a Blob. The
  // authoritative dist-level guard ships with the builder in P3; this source-level
  // check catches the obvious breakages early.
  const src = CaptureRing.toString();

  it('is a class declaration', () => {
    expect(src.startsWith('class')).toBe(true);
  });

  it('has no transpiler/bundler helper (native class fields)', () => {
    expect(/\b__(publicField|privateField|decorateClass|decorateParam|esDecorate)\b/.test(src)).toBe(false);
  });

  it('does not reference the module-scope captureCapacityFrames (constant is inlined)', () => {
    expect(src.includes('captureCapacityFrames')).toBe(false);
  });

  it('carries NO jitter-buffer machinery (send ring stays a dumb pipe)', () => {
    // Guard against the design's core risk: complexity creeping back onto the send side.
    for (const banned of ['currentL', 'currentH', 'widen', 'narrow', 'snapTarget', 'dropLine', 'overrunAt', 'underrun', 'splice', 'windowReads']) {
      expect(src.includes(banned)).toBe(false);
    }
  });
});
