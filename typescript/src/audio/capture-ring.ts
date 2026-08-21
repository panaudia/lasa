/**
 * capture-ring.ts — the mic SEND ring (worker-capture-design.md §5).
 *
 * A minimal lock-free SPSC ring over a SharedArrayBuffer. The capture AudioWorklet is
 * the PRODUCER (interleaves each 128-sample render quantum into the ring); the MOQ
 * worker is the CONSUMER (drains whole frames out to feed the Opus encoder). It is the
 * send-side twin of the playout ring — but deliberately carries NONE of the
 * jitter-buffer machinery: no adaptive window, no ±1 splice, no target fill, no
 * warm-start, no underrun/overrun handling. Its only job is to pass every write
 * through to the encoder as fast as possible — drain to empty, hold nothing (design
 * §5). If you find yourself reaching for any jitter-buffer concept here, it is the bug
 * this file exists to prevent.
 *
 * Cross-thread SPSC. Producer and consumer run on different threads, so BOTH
 * cumulative positions live in shared atomic cells: the worklet stores `writePos`
 * (the release fence for its ring writes) and the worker stores `readPos` (the release
 * fence for the space it freed). Each side `Atomics.load`s the other's position with
 * acquire semantics; bulk PCM is plain reads/writes — the position load/store pair
 * establishes the happens-before. Positions are cumulative (never wrap), so the fill
 * `writePos - readPos` is unambiguous in [0, capacity]; index via `(pos % cap) * nc`.
 *
 * NOTE: each THREAD constructs its own `CaptureRing` over the same shared cells — the
 * object is not shared. (Off-thread tests construct one and drive both ends, which is
 * valid because `Atomics.load`/`store` work on non-shared typed arrays too.)
 *
 * Self-contained for `.toString()` serialization into the capture worklet (Blob URL,
 * cannot import): no module-scope free references, no imported constants (the ctor
 * takes everything), and — unlike JitterBufferCore — fields are assigned in the
 * constructor rather than via class-field initializers, so NO transpile target can
 * lower them to a `__publicField` helper that would be undefined in the worklet.
 */

/**
 * Fixed ring capacity in frames — a small stall backstop, NOT a tuned buffer (the
 * operating fill is ~0; the worker drains to empty every wake). 2048 frames ≈ 42 ms at
 * 48 kHz: comfortably larger than any normal worker-drain gap, so a lap is a logged
 * glitch, not a routine event. Single source of truth for SAB sizing — main allocates
 * `sharedStorage` of `captureCapacityFrames() * numChannels`. Deliberately a flat
 * constant: there is no geometry formula on the send side.
 */
export function captureCapacityFrames(): number {
  return 2048;
}

export interface CaptureRingConfig {
  /** Channels per frame (interleaved). Default 1 (mono mic). */
  numChannels?: number;
  /** Ring capacity in frames. Default {@link captureCapacityFrames}. */
  capacityFrames?: number;
  /** SAB-backed interleaved float storage of exactly `capacityFrames * numChannels`. */
  sharedStorage: Float32Array;
  /** SAB-backed length-1 BigInt64Array — cumulative producer position (frames). */
  sharedWritePos: BigInt64Array;
  /** SAB-backed length-1 BigInt64Array — cumulative consumer position (frames). */
  sharedReadPos: BigInt64Array;
}

export class CaptureRing {
  readonly nc: number;
  readonly capacity: number;
  readonly data: Float32Array;
  private readonly wpCell: BigInt64Array;
  private readonly rpCell: BigInt64Array;
  /** Count of quanta dropped because the consumer stalled past capacity (§5.1). */
  overflows: number;

  constructor(cfg: CaptureRingConfig) {
    const nc = cfg.numChannels ?? 1;
    const capacity = cfg.capacityFrames ?? 2048; // flat constant (no module ref)
    if (nc < 1) {
      throw new Error('CaptureRing: numChannels must be >= 1');
    }
    if (capacity < 1) {
      throw new Error('CaptureRing: capacityFrames must be >= 1');
    }
    if (cfg.sharedStorage.length !== capacity * nc) {
      throw new Error(
        `CaptureRing: sharedStorage length ${cfg.sharedStorage.length} != capacity*nc ${capacity * nc} ` +
          '(allocate capacityFrames * numChannels floats)'
      );
    }
    if (cfg.sharedWritePos.length < 1 || cfg.sharedReadPos.length < 1) {
      throw new Error('CaptureRing: sharedWritePos/sharedReadPos must be length-1 BigInt64Arrays');
    }
    this.nc = nc;
    this.capacity = capacity;
    this.data = cfg.sharedStorage;
    this.wpCell = cfg.sharedWritePos;
    this.rpCell = cfg.sharedReadPos;
    this.overflows = 0;
  }

  /** Cumulative producer position (frames), acquire-loaded. */
  get writePos(): number {
    return Number(Atomics.load(this.wpCell, 0));
  }
  /** Cumulative consumer position (frames), acquire-loaded. */
  get readPos(): number {
    return Number(Atomics.load(this.rpCell, 0));
  }
  /** Current fill in frames (unambiguous: positions are cumulative). */
  fillFrames(): number {
    return this.writePos - this.readPos;
  }

  /**
   * PRODUCER (capture worklet). Interleave one render quantum of planar channels into
   * the ring. `planar[ch]` is a Float32Array of `nFrames` samples (Web Audio is
   * planar; all channels equal length). Channels beyond `planar.length` reuse the last
   * (mono→stereo dup); channels beyond `nc` are ignored. If the consumer has stalled
   * and the quantum would not fit, the WHOLE quantum is dropped and `overflows` is
   * bumped — never blocks, never overwrites unread data (§5.1). Returns true if written.
   */
  write(planar: Float32Array[]): boolean {
    if (!planar || planar.length === 0 || !planar[0]) {
      return false;
    }
    const nc = this.nc;
    const cap = this.capacity;
    const nFrames = planar[0].length;
    if (nFrames === 0) {
      return false;
    }

    const wp = this.writePos;
    const rp = this.readPos;
    if (wp - rp + nFrames > cap) {
      this.overflows++;
      return false; // consumer stalled — drop this quantum (§5.1)
    }

    const data = this.data;
    const startFrame = wp % cap;
    for (let i = 0; i < nFrames; i++) {
      const ringBase = ((startFrame + i) % cap) * nc;
      for (let ch = 0; ch < nc; ch++) {
        const src = planar[ch < planar.length ? ch : planar.length - 1]!;
        data[ringBase + ch] = src[i]!;
      }
    }
    // release: publish the ring writes to the consumer.
    Atomics.store(this.wpCell, 0, BigInt(wp + nFrames));
    return true;
  }

  /**
   * CONSUMER (MOQ worker). Copy all whole frames currently available into `dst`
   * (interleaved), up to `dst`'s capacity, then free that space. Returns the number of
   * interleaved SAMPLES written (`frames * nc`), or 0 if nothing was ready. Drain to
   * empty: leaves only what the producer hasn't yet published.
   */
  drain(dst: Float32Array): number {
    const nc = this.nc;
    const cap = this.capacity;
    const wp = this.writePos;
    const rp = this.readPos;
    const avail = wp - rp;
    const room = Math.floor(dst.length / nc);
    const nFrames = avail < room ? avail : room;
    if (nFrames <= 0) {
      return 0;
    }

    const data = this.data;
    const startFrame = rp % cap;
    if (startFrame + nFrames <= cap) {
      dst.set(data.subarray(startFrame * nc, (startFrame + nFrames) * nc));
    } else {
      const first = cap - startFrame;
      dst.set(data.subarray(startFrame * nc, cap * nc), 0);
      dst.set(data.subarray(0, (nFrames - first) * nc), first * nc);
    }
    // release: free the consumed space for the producer.
    Atomics.store(this.rpCell, 0, BigInt(rp + nFrames));
    return nFrames * nc;
  }
}
