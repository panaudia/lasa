/**
 * Tests for StereoMeterCore — the shared stereo-ness accumulator behind the
 * mono-collapse diagnostics taps (plan/stereo-diagnostics Phase 2).
 */

import { describe, it, expect } from 'vitest';
import { StereoMeterCore } from '../src/audio/stereo-meter-core.js';

const RATE = 48000;

/** Interleaved stereo buffer from per-channel sample functions. */
function genInterleaved(frames: number, fnL: (t: number) => number, fnR: (t: number) => number): Float32Array {
  const pcm = new Float32Array(frames * 2);
  for (let i = 0; i < frames; i++) {
    const t = i / RATE;
    pcm[i * 2] = fnL(t);
    pcm[i * 2 + 1] = fnR(t);
  }
  return pcm;
}

const sine = (hz: number, amp: number) => (t: number) => amp * Math.sin(2 * Math.PI * hz * t);
const silence = () => 0;

describe('StereoMeterCore', () => {
  it('reports the server test-tone signal (440 L / 880 R) as strongly stereo', () => {
    const meter = new StereoMeterCore();
    meter.writeInterleaved(genInterleaved(RATE / 4, sine(440, 0.2), sine(880, 0.2)), 2);
    const r = meter.snapshotAndReset();

    expect(r.frames).toBe(RATE / 4);
    // Sine RMS = amp/√2 ≈ 0.1414
    expect(r.rmsL).toBeCloseTo(0.2 / Math.SQRT2, 3);
    expect(r.rmsR).toBeCloseTo(0.2 / Math.SQRT2, 3);
    // Different frequencies are (near-)orthogonal → correlation ~0, side ≈ mid
    expect(Math.abs(r.correlation)).toBeLessThan(0.05);
    expect(r.sideRms).toBeGreaterThan(r.midRms * 0.8);
  });

  it('reports identical L/R (mono collapse) as correlation 1 with zero side energy', () => {
    const meter = new StereoMeterCore();
    const mono = sine(440, 0.3);
    meter.writeInterleaved(genInterleaved(RATE / 4, mono, mono), 2);
    const r = meter.snapshotAndReset();

    expect(r.correlation).toBeCloseTo(1, 6);
    expect(r.sideRms).toBeLessThan(1e-6);
    expect(r.midRms).toBeCloseTo(0.3 / Math.SQRT2, 3);
  });

  it('reports anti-phase L/R as correlation −1 with zero mid energy', () => {
    const meter = new StereoMeterCore();
    const s = sine(440, 0.3);
    meter.writeInterleaved(genInterleaved(RATE / 4, s, (t) => -s(t)), 2);
    const r = meter.snapshotAndReset();

    expect(r.correlation).toBeCloseTo(-1, 6);
    expect(r.midRms).toBeLessThan(1e-6);
    expect(r.sideRms).toBeGreaterThan(0.2);
  });

  it('reports silence with zero RMS and a defined (0) correlation', () => {
    const meter = new StereoMeterCore();
    meter.writeInterleaved(genInterleaved(1024, silence, silence), 2);
    const r = meter.snapshotAndReset();

    expect(r.rmsL).toBe(0);
    expect(r.rmsR).toBe(0);
    expect(r.correlation).toBe(0);
    expect(r.sideRms).toBe(0);
  });

  it('treats mono input (channels=1) as a mono verdict, not an error', () => {
    const meter = new StereoMeterCore();
    const pcm = new Float32Array(1024);
    for (let i = 0; i < pcm.length; i++) pcm[i] = 0.5 * Math.sin((2 * Math.PI * 440 * i) / RATE);
    meter.writeInterleaved(pcm, 1);
    const r = meter.snapshotAndReset();

    expect(r.frames).toBe(1024);
    expect(r.correlation).toBeCloseTo(1, 6);
    expect(r.sideRms).toBe(0);
    expect(r.rmsL).toBeCloseTo(r.rmsR, 6);
  });

  it('only measures the first two channels of wider interleaves', () => {
    const frames = 512;
    const pcm = new Float32Array(frames * 4);
    for (let i = 0; i < frames; i++) {
      pcm[i * 4] = 0.25; // L
      pcm[i * 4 + 1] = -0.25; // R
      pcm[i * 4 + 2] = 99; // would wreck the sums if included
      pcm[i * 4 + 3] = 99;
    }
    const meter = new StereoMeterCore();
    meter.writeInterleaved(pcm, 4);
    const r = meter.snapshotAndReset();

    expect(r.frames).toBe(frames);
    expect(r.rmsL).toBeCloseTo(0.25, 6);
    expect(r.rmsR).toBeCloseTo(0.25, 6);
    expect(r.correlation).toBeCloseTo(-1, 6);
  });

  it('accumulates across writes and resets on snapshot', () => {
    const meter = new StereoMeterCore();
    const chunk = genInterleaved(100, sine(440, 0.2), sine(880, 0.2));
    meter.writeInterleaved(chunk, 2);
    meter.writeInterleaved(chunk, 2);
    expect(meter.frameCount).toBe(200);

    const r = meter.snapshotAndReset();
    expect(r.frames).toBe(200);
    expect(meter.frameCount).toBe(0);

    const empty = meter.snapshotAndReset();
    expect(empty.frames).toBe(0);
    expect(empty.correlation).toBe(0);
  });

  it('writePlanar matches writeInterleaved on the same signal', () => {
    const frames = RATE / 8;
    const left = new Float32Array(frames);
    const right = new Float32Array(frames);
    const interleaved = new Float32Array(frames * 2);
    for (let i = 0; i < frames; i++) {
      const t = i / RATE;
      left[i] = sine(440, 0.2)(t);
      right[i] = sine(880, 0.1)(t);
      interleaved[i * 2] = left[i]!;
      interleaved[i * 2 + 1] = right[i]!;
    }

    const a = new StereoMeterCore();
    a.writePlanar(left, right, frames);
    const b = new StereoMeterCore();
    b.writeInterleaved(interleaved, 2);

    const ra = a.snapshotAndReset();
    const rb = b.snapshotAndReset();
    expect(ra.rmsL).toBeCloseTo(rb.rmsL, 10);
    expect(ra.rmsR).toBeCloseTo(rb.rmsR, 10);
    expect(ra.correlation).toBeCloseTo(rb.correlation, 10);
    expect(ra.sideRms).toBeCloseTo(rb.sideRms, 10);
  });

  it('is .toString()-serializable for the worklet blob (no imports, no helpers)', () => {
    const src = StereoMeterCore.toString();
    expect(src.startsWith('class')).toBe(true);
    expect(/\b__(publicField|privateField|decorateClass|decorateParam|name|esDecorate)\b/.test(src)).toBe(false);
  });
});
