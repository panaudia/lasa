/**
 * stereo-meter-core.ts — shared stereo-ness metering for the mono-collapse
 * diagnostics (spatial-mixer/plan/stereo-diagnostics/plan.md).
 *
 * One accumulator class used by both measurement taps:
 *   • Tap A — decoded PCM in the MOQ worker (is the incoming *stream content* stereo?)
 *   • Tap B — rendered output in the playout worklet (is the *graph* preserving it?)
 *
 * Reading both localizes a collapse: A stereo + B mono → graph; A mono → server
 * content or decode; both stereo but it *sounds* mono → OS/device/routing.
 *
 * Everything is derived from three running sums (Σl², Σr², Σl·r), so a window
 * costs three multiply-adds per frame and the report is computed only at
 * snapshot time. `sideRms` (energy of (L−R)/2) is the unambiguous collapse
 * signal — exactly 0 for any mono-derived signal, and well-defined near silence
 * where correlation is not.
 *
 * IMPORTANT: like JitterBufferCore, this class is serialized into the playout
 * worklet's Blob via `.toString()` — it must stay fully self-contained: no
 * imports used by the class body, no external constants, no decorators.
 */

/** One analysis window's result. Linear amplitudes (not dB); convert UI-side. */
export interface StereoMeterReport {
  /** Frames (sample pairs) in the window. 0 ⇒ all other fields are 0. */
  frames: number;
  rmsL: number;
  rmsR: number;
  /** RMS of (L+R)/2 — the mono ("mid") content. */
  midRms: number;
  /** RMS of (L−R)/2 — the stereo difference ("side"). ~0 ⇒ mono collapse. */
  sideRms: number;
  /** Normalized L/R correlation at lag 0, −1…+1. 0 when either channel is ~silent. */
  correlation: number;
}

export class StereoMeterCore {
  private sumLL = 0;
  private sumRR = 0;
  private sumLR = 0;
  private frames = 0;

  /** Frames accumulated since the last snapshot (drives window emission). */
  get frameCount(): number {
    return this.frames;
  }

  /**
   * Accumulate interleaved PCM (LRLR… for stereo). `channels` is the interleave
   * stride; only the first two channels are measured. Mono input (channels=1)
   * is treated as L=R — it reports correlation 1 / sideRms 0, which is the
   * correct verdict for it.
   */
  writeInterleaved(pcm: Float32Array, channels: number): void {
    if (channels < 1) return;
    const n = Math.floor(pcm.length / channels);
    for (let i = 0; i < n; i++) {
      const l = pcm[i * channels]!;
      const r = channels > 1 ? pcm[i * channels + 1]! : l;
      this.sumLL += l * l;
      this.sumRR += r * r;
      this.sumLR += l * r;
    }
    this.frames += n;
  }

  /** Accumulate planar channels (the worklet's output layout). `right` null ⇒ mono. */
  writePlanar(left: Float32Array, right: Float32Array | null, count: number): void {
    for (let i = 0; i < count; i++) {
      const l = left[i]!;
      const r = right ? right[i]! : l;
      this.sumLL += l * l;
      this.sumRR += r * r;
      this.sumLR += l * r;
    }
    this.frames += count;
  }

  /** Produce the window report and reset the accumulators. */
  snapshotAndReset(): StereoMeterReport {
    const f = this.frames;
    const ll = this.sumLL;
    const rr = this.sumRR;
    const lr = this.sumLR;
    this.sumLL = 0;
    this.sumRR = 0;
    this.sumLR = 0;
    this.frames = 0;
    if (f === 0) {
      return { frames: 0, rmsL: 0, rmsR: 0, midRms: 0, sideRms: 0, correlation: 0 };
    }
    // mid=(L+R)/2 and side=(L−R)/2 expand to (Σl²+Σr²±2Σlr)/4 — same accumulators.
    // Math.max(0,…) guards tiny negative residue from float cancellation.
    const rmsL = Math.sqrt(ll / f);
    const rmsR = Math.sqrt(rr / f);
    const midRms = Math.sqrt(Math.max(0, ll + rr + 2 * lr) / (4 * f));
    const sideRms = Math.sqrt(Math.max(0, ll + rr - 2 * lr) / (4 * f));
    const denom = Math.sqrt(ll * rr);
    const correlation = denom > 1e-20 ? Math.max(-1, Math.min(1, lr / denom)) : 0;
    return { frames: f, rmsL, rmsR, midRms, sideRms, correlation };
  }
}
