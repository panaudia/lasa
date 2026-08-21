/**
 * jitter-buffer-core.ts — the v4 jitter buffer (envelope sensing + placement
 * servo), ported from the Go `panaudia-engine/buffers/jitter_buffer.go`
 * (design: `panaudia-engine/plan/jitter-v4-design.md`; port plan:
 * `panaudia-engine/plan/jitter-v4-ts-port.md`). Supersedes the v3
 * correction-counting core that previously lived in this file.
 *
 * Architecture (design §2–§7): a sensing window feeds two fill views — the
 * VIRTUAL signal (wp − R·tick, actuation-invariant: the clean view of the
 * input process, measured for per-side widths and the feed-forward drift
 * slope) and RAW fill (wp − rp, the physical quantity the servo controls,
 * measured for the displacement error and the macro-trim trigger). A slow
 * drop-only P anchor plus a soft-clamped feed-forward term holds the raw-fill
 * window median at a derived setpoint by trickling rate-limited ±1 splices
 * through a debt bucket; a confirmed whole-window displacement beyond the
 * servo's reach goes in one macro-trim. There is no operating band, no drop
 * line, and no correction counting — the only discrete events are the two
 * physical safety valves (underrun, capacity) and the bounded trim.
 *
 * **Single-threaded controller, SAB-aware ring.** The Go original is
 * lock-free SPSC across threads; in the browser the reader (worklet
 * `process`) owns ALL controller state as plain fields. In cross-thread SAB
 * mode (worker decodes and writes, worklet reads) only the ring storage and
 * `writePos` cross the boundary: `writePos` lives in a shared
 * `BigInt64Array` cell and the `Atomics.store/load` pair is the
 * release/acquire fence ordering ring writes against ring reads — exactly
 * the Go SPSC contract, reduced to the one value that needs it.
 *
 * **One interleaved buffer, frame-based corrections.** The atomic unit is a
 * frame of `nc` interleaved samples; a ±1 correction moves the whole frame,
 * so for binaural (`nc=2`) L/R stay sample-locked and the ITD is preserved
 * structurally.
 *
 * **Deterministic lockstep with Go.** Positions and ticks are exact in JS
 * numbers to 2^53; the controller float arithmetic (no transcendentals) is
 * IEEE-754 double in both implementations; the splice placement scan does
 * its float32 comparisons through `Math.fround` to mirror Go's `float32`
 * ops. This is what makes the fixture replay (`tests/`) assert LEDGER
 * EQUALITY — the same decision at every read of every scenario — rather
 * than approximate agreement.
 *
 * This module is pure logic — no Web Audio, no worklet, no DOM. The class is
 * serialized into the AudioWorklet via `.toString()` (see
 * playout-worklet.ts), so it must be SELF-CONTAINED: every constant is
 * inlined as a literal (mirroring the Go v4 tuning block; the comments name
 * the Go constants) and every helper is a private method — no free
 * references to module-scope identifiers.
 */

/**
 * Configuration for {@link JitterBufferCore}. All frame/latency fields are
 * in **samples** (frames) — the browser knows these concretely: R = the
 * 128-sample worklet quantum, W = the server's 240-sample Opus frame. This
 * is the Go config's exact-count form (`WriterFrames`/`ReaderFrames`, added
 * in TS-port phase 0 precisely because durations cannot express the
 * 128-sample quantum); the duration-based fields do not exist here.
 *
 * The per-buffer surface is {floor, robustness, capacity} plus geometry
 * (design §8) — no per-route presets. `qualityLevel` maps the LASA stream
 * quality intent (0–2) onto those three, exactly Go's `QualityLevel`;
 * explicit fields override the level's values.
 */
export interface JitterBufferCoreConfig {
  /** Sample rate (Hz). Used for ms-derived tuning constants and stats. Default 48000. */
  sampleRate?: number;
  /** Channels per frame (interleaved). Default 1. Binaural playout is 2. */
  numChannels?: number;
  /** R — reader frame, samples. Required: the caller declares its callback size (browser worklet: 128). */
  readerFrame?: number;
  /** W — writer frame, samples. Required: the caller declares its wire framing (server Opus: 240). */
  writerFrame?: number;
  /** S — static pad above the underrun edge, samples. Default = 1 ms. */
  safety?: number;
  /**
   * LASA quality level 0–2 (an INTENT — "how latency-tolerant is this
   * stream"). Maps to {floor, robustness, capacity} defaults:
   * 0 = interactive (0, 1, derived), 1 = resilient (50 ms, 4, 500 ms),
   * 2 = playback/broadcast (150 ms, 8, 400 ms). Default 0.
   */
  qualityLevel?: number;
  /**
   * Declared minimum operating latency, samples — e.g. the FEC floor
   * ({@link fecFloor}). Composes with the quality level's floor and the
   * width-derived setpoint by MAX (floors compose; they never stack —
   * mirrors Go `engine.AddSource`).
   */
  floor?: number;
  /**
   * The latency↔robustness knob κ ≥ 1 (values < 1 clamp to 1, the
   * low-latency end). Scales responses to MEASURED spread, so it costs
   * nothing on clean networks. Overrides the level's κ.
   */
  robustness?: number;
  /** Ring/latency budget, frames; 0/omitted derives the worst case. Overrides the level's capacity. */
  capacity?: number;
  /**
   * Cross-thread mode. When the writer (decode Worker) and the reader
   * (worklet) are different threads, the ring storage and writePos must
   * live in shared memory. Provide a `SharedArrayBuffer`-backed interleaved
   * `Float32Array` of exactly `capacity*nc` and a length-1 `BigInt64Array`
   * for the (atomic) writePos; `readPos` stays reader-owned (plain). Size
   * them with {@link computeJitterCapacity}. Omit both for single-thread mode.
   */
  sharedStorage?: Float32Array;
  /** SAB-backed length-1 BigInt64Array holding the atomic cumulative writePos (frames). */
  sharedWritePos?: BigInt64Array;
}

/**
 * The jitter-buffer floor (samples) a declared LASA redundancy intent
 * requires (lasa-core.md §4.2/§5.1): a redundant copy repairs a loss only
 * if the buffer already spans the offset, and the floor must cover loss
 * CLUSTERS — two overlapping repair holds — not one: 2 × (offset +
 * give-up margin of 2 frames) × the wire frame size (A9 bring-up, design
 * doc §14). Zero offset means no FEC, no floor. Port of Go
 * `buffers.FECFloor` (sample-native: writerFrame in samples, not a
 * duration). Feed the result into {@link JitterBufferCoreConfig.floor};
 * it composes with the quality level's floor by max.
 */
export function fecFloor(maxOffset: number, writerFrame: number): number {
  if (maxOffset <= 0) return 0;
  return 2 * (maxOffset + 2) * writerFrame;
}

/**
 * Pure geometry: the ring capacity (frames) and channel count for a given
 * config — the single source of truth shared by callers that must allocate
 * the SharedArrayBuffer storage (`capacity*nc` floats) before constructing
 * the cross-thread cores.
 *
 * MUST match the constructor's derivation exactly (the constructor cannot
 * call this function — it is serialized standalone into the worklet — so
 * the formula is duplicated; jitter-buffer-core.test cross-checks they
 * agree). Mirrors Go `NewJitterBuffer`'s capacity path.
 */
export function computeJitterCapacity(cfg: JitterBufferCoreConfig = {}): { capacity: number; nc: number } {
  const sr = cfg.sampleRate ?? 48000;
  const nc = cfg.numChannels ?? 1;
  const f = (ms: number) => Math.trunc((ms * sr) / 1000);
  const W = cfg.writerFrame ?? 0;
  const R = cfg.readerFrame ?? 0;
  if (W <= 0 || R <= 0) {
    throw new Error('computeJitterCapacity: readerFrame and writerFrame must be > 0');
  }
  const S = cfg.safety || f(1);
  // Quality level → {floor, robustness, capacity} defaults (Go QualityLevel).
  const level = cfg.qualityLevel ?? 0;
  let qFloor = 0;
  let qRob = 1;
  let qCap = 0;
  if (level === 1) {
    qFloor = f(50);
    qRob = 4;
    qCap = f(500);
  } else if (level >= 2) {
    qFloor = f(150);
    qRob = 8;
    qCap = f(400);
  }
  const flDecl = Math.max(cfg.floor ?? 0, qFloor);
  let kappa = cfg.robustness ?? qRob;
  if (kappa < 1) kappa = 1;
  const kLow = 1.2 * kappa;
  const kHigh = 1.5 * kappa;
  const wlCap = Math.trunc(f(30) * kappa);
  const whCap = Math.trunc(f(60) * kappa);
  const spBase = Math.max(R + S, R + W);
  const spMax = Math.max(spBase + Math.trunc(kLow * wlCap), flDecl);
  let capacity = cfg.capacity ?? qCap;
  if (!capacity) {
    capacity = 2 * (spMax + Math.trunc(kHigh * whCap)) + 2 * Math.max(W, R);
  }
  return { capacity, nc };
}

/** Rich snapshot of the buffer for tuning/observability (port of Go `JitterBufferStats`). */
export interface JitterBufferSnapshot {
  fillFrames: number;
  fillMs: number;
  /** Target pre-read fill the servo holds (F + cushion, floored, capped). */
  setpointFrames: number;
  setpointMs: number;
  /** Held effective late-side width (latency currency). */
  widthLowFrames: number;
  widthLowMs: number;
  /** Held effective bunch/reader-stall-side width (capacity currency). */
  widthHighFrames: number;
  widthHighMs: number;
  /** Current servo rate, frames/s of read time; + = drop trickle, − = insert. */
  ratePerSec: number;
  /** Sensing + servo frozen (sustained delivery absence — outage/loss burst). */
  frozen: boolean;
  started: boolean;
  underruns: number;
  /** Near-capacity snaps (the capacity valve short of a full lap). */
  overruns: number;
  /** Writer lapped a stalled reader. */
  laps: number;
  /** Macro-trims: confirmed-displacement single discards (one position jump each). */
  trims: number;
  samplesDropped: number;
  samplesInserted: number;
}

export class JitterBufferCore {
  // ---- immutable geometry (frames) ----
  readonly capacity: number;
  readonly w: number;
  readonly r: number;
  readonly s: number;
  readonly nc: number;
  readonly sampleRate: number;

  // ---- immutable derived tuning (Go: the ctor-derived block) ----
  readonly windowReads: number;
  readonly freezeReads: number;
  readonly narrowStep: number; // frames per window (float; ÷κ)
  readonly spBase: number; // max(R+S, R+W): the structural lattice floor
  readonly flDecl: number; // declared floor; composes by MAX
  readonly kLow: number;
  readonly kHigh: number;
  readonly wlCap: number;
  readonly deadband: number;
  readonly gain: number;
  readonly rateCap: number;
  readonly ffClamp: number;
  readonly ffDeadZone: number;
  readonly theta: number;
  readonly spMax: number;

  // ---- storage: capacity * nc interleaved floats ----
  readonly data: Float32Array;

  // ---- SPSC heads — cumulative (never wrap). Index via (pos % capacity) * nc. ----
  // writePos crosses the writer→reader thread boundary in SAB mode, so it is
  // backed by an atomic cell when `sharedWritePos` is given; otherwise a plain
  // number. The Atomics.store/load act as the release/acquire fence pairing
  // the ring writes (writer) with the ring reads (reader) — the Go SPSC
  // contract. readPos is reader-owned (the writer never touches it): plain.
  private _writePos = 0;
  private wpCell: BigInt64Array | null = null;
  get writePos(): number {
    return this.wpCell ? Number(Atomics.load(this.wpCell, 0)) : this._writePos;
  }
  set writePos(v: number) {
    if (this.wpCell) Atomics.store(this.wpCell, 0, BigInt(v));
    else this._writePos = v;
  }
  readPos = 0;

  // ---- reader-owned sensing/controller state ----
  started = false;
  tick = 0; // read count since start — the sensing clock
  private lastWp = 0;
  private gapRun = 0;
  frozen = false;

  // Current sensing window: pre-read raw fill + virtual signal, appended
  // together (one shared count). Preallocated — no steady-state allocation
  // on the audio thread.
  private rawBuf: Float64Array;
  private virtBuf: Float64Array;
  private sortBuf: Float64Array;
  private winCount = 0;

  // Last K windows' measured widths (rings) for the rank filter.
  private wlHist: Float64Array;
  private whHist: Float64Array;
  private histLen = 0;
  private histPos = 0;

  // Feed-forward drift estimator: (tick, window-median virtual) pairs across
  // the last K window closes. Reset on freeze-resume so an outage's virtual
  // step never reads as drift.
  private ffTicks: Float64Array;
  private ffVals: Float64Array;
  private ffLen = 0;

  // Held effective widths (frames, float), servo rate, debt, pending trim.
  // Public for white-box tests and observability (v3 convention: outside
  // write/read they are read-only; mutating by hand is a test seam only).
  wl = 0;
  wh = 0;
  rate = 0; // frames/s of read time; + = drop
  debt = 0;
  pendingTrim = 0;

  /** Live setpoint (pre-read fill target, frames). Reader-written; observers read. */
  setpoint: number;

  // ---- cumulative stats (reader-owned plain numbers) ----
  underruns = 0;
  laps = 0;
  overruns = 0;
  trims = 0;
  samplesDropped = 0;
  samplesInserted = 0;

  constructor(cfg: JitterBufferCoreConfig = {}) {
    // NOTE: every default below is an inlined literal (never read from a
    // module-scope constant) because this class is serialized into the
    // AudioWorklet via `.toString()` — it must be self-contained. The
    // literals mirror the Go v4 tuning block (buffers/jitter_buffer.go);
    // each is annotated with its Go name.
    const sr = cfg.sampleRate ?? 48000;
    const nc = cfg.numChannels ?? 1;
    // Truncating ms→frames, matching Go's msToFrames (int64 truncation).
    const f = (ms: number) => Math.trunc((ms * sr) / 1000);

    const W = cfg.writerFrame ?? 0;
    const R = cfg.readerFrame ?? 0;
    if (W <= 0 || R <= 0) {
      throw new Error('JitterBufferCore: readerFrame and writerFrame must be > 0 (the caller declares its geometry)');
    }
    const S = cfg.safety || f(1); // Go: Safety default 1 ms

    // Quality level → {floor, robustness, capacity} defaults (Go
    // QualityLevel — the one place the LASA intent maps to buffer params;
    // levels outside 0–2 clamp). Explicit config fields override.
    const level = cfg.qualityLevel ?? 0;
    let qFloor = 0;
    let qRob = 1;
    let qCap = 0;
    if (level === 1) {
      qFloor = f(50); // resilient
      qRob = 4;
      qCap = f(500);
    } else if (level >= 2) {
      qFloor = f(150); // playback/broadcast
      qRob = 8;
      qCap = f(400);
    }
    // Floors compose by MAX, never override or stack (lasa-core.md §4.2,
    // Go engine.AddSource): an explicit floor (e.g. the FEC floor) and the
    // quality level's floor are independent lower bounds on the same
    // quantity.
    const flDecl = Math.max(cfg.floor ?? 0, qFloor);
    let kappa = cfg.robustness ?? qRob;
    if (kappa < 1) kappa = 1;

    // The setpoint base includes one whole writer frame of headroom above
    // the read quantum: in W = R geometry drift-down arrives as a
    // single-read whole-packet MISS (the fill lattice), so a click-free
    // buffer must survive fill dropping by W in one read. R + W is a hard
    // lattice floor, not a tunable cushion. The DECLARED floor composes
    // with the width-derived setpoint via MAX, not addition — the floor
    // pre-provisions the very lateness the width estimator will later
    // measure, so stacking them would double-provision.
    const deadband = f(0.25); // Go: v4DeadbandMs
    const spBase = Math.max(R + S, R + W);

    const kLow = 1.2 * kappa; // Go: v4KLow · κ
    const kHigh = 1.5 * kappa; // Go: v4KHigh · κ
    const wlCap = Math.trunc(f(30) * kappa); // Go: v4WidthLowMax · κ
    const whCap = Math.trunc(f(60) * kappa); // Go: v4WidthHighMax · κ
    let spMax = Math.max(spBase + Math.trunc(kLow * wlCap), flDecl);

    // The FF dead zone is denominated in the geometry's median quantum: in
    // W = R geometry the fill lattice accumulates drift silently and
    // releases it as whole-W medV steps, so sub-W/2 deltas are noise; in
    // W ≠ R geometry there is no lattice — the sawtooth phase glides and
    // the median moves in gcd(W, R) grains (browser 240/128: 16 frames),
    // so a W/2 zone would eat ±21 ppm drift entirely and dump it on the
    // slow anchor (TS-port phase 0 finding). gcd/2 is exact in both.
    let ffDeadZone = Math.trunc(this.gcd(W, R) / 2);
    if (ffDeadZone < 1) ffDeadZone = 1;

    // Θ is κ-INVARIANT (bring-up finding): scaling it with the knob opens a
    // drip zone between the anchor deadband and the trim threshold where
    // the slow P anchor grinds splices for minutes — the pure-slew failure
    // the trim exists to prevent. A trim removes only stale audio, so it is
    // safe at every robustness level.
    const theta = W + Math.trunc(20 * 2.0); // Go: w + v4RateCapPerSec·v4WindowSec

    let capacity = cfg.capacity ?? qCap;
    if (!capacity) {
      capacity = 2 * (spMax + Math.trunc(kHigh * whCap)) + 2 * Math.max(W, R);
    }
    // With an explicit (smaller) capacity the setpoint cap must respect it:
    // leave at least a few frames of clump headroom so a corrupted width
    // estimate can never park fill against the valves.
    const reserve = 4 * Math.max(W, R);
    if (spMax > capacity - reserve) {
      spMax = Math.max(spBase, capacity - reserve);
    }

    this.capacity = capacity;
    this.w = W;
    this.r = R;
    this.s = S;
    this.nc = nc;
    this.sampleRate = sr;
    this.windowReads = Math.trunc((2.0 * sr) / R); // Go: v4WindowSec — wall-time-derived (750 at R=128, 400 at R=240)
    this.freezeReads = Math.max(8, Math.trunc(f(40) / R)); // Go: v4FreezeMs
    this.narrowStep = f(0.5) / kappa; // Go: v4NarrowStepMs ÷ κ
    this.spBase = spBase;
    this.flDecl = flDecl;
    this.kLow = kLow;
    this.kHigh = kHigh;
    this.wlCap = wlCap;
    this.deadband = deadband;
    this.gain = 0.01; // Go: v4GainPerSec — slow drop-only P anchor (≈ 0.5 splices/s per ms)
    this.rateCap = 20; // Go: v4RateCapPerSec (≈ 420 ppm; provisional, revisit on audibility)
    this.ffClamp = 22; // Go: v4FFClampPerSec — above per-quantum flicker, far below loss-scale slopes
    this.ffDeadZone = ffDeadZone;
    this.theta = theta;
    this.spMax = spMax;

    // Cross-thread (SAB) mode: adopt the caller's shared storage + atomic
    // writePos cell instead of allocating a private ring. The SAB is zero-
    // initialised, so writePos starts at 0 with no explicit store (and we
    // must NOT re-zero it — the other thread may already be using it).
    if (cfg.sharedStorage) {
      if (cfg.sharedStorage.length !== capacity * nc) {
        throw new Error(
          `JitterBufferCore: sharedStorage length ${cfg.sharedStorage.length} != capacity*nc ${capacity * nc} ` +
            '(size it with computeJitterCapacity using the same config)'
        );
      }
      this.data = cfg.sharedStorage;
    } else {
      this.data = new Float32Array(capacity * nc);
    }
    if (cfg.sharedWritePos) {
      if (cfg.sharedWritePos.length < 1) {
        throw new Error('JitterBufferCore: sharedWritePos must be a length-1 BigInt64Array');
      }
      this.wpCell = cfg.sharedWritePos;
    }

    this.rawBuf = new Float64Array(this.windowReads);
    this.virtBuf = new Float64Array(this.windowReads);
    this.sortBuf = new Float64Array(this.windowReads);
    this.wlHist = new Float64Array(10); // Go: v4WidthHistory
    this.whHist = new Float64Array(10);
    this.ffTicks = new Float64Array(10);
    this.ffVals = new Float64Array(10);

    // Warm start: zero measured width.
    this.setpoint = Math.min(Math.max(spBase, flDecl), spMax);
  }

  /**
   * Copy `src` (interleaved, length a multiple of `nc`) into the ring. Never
   * blocks; writes larger than capacity are clipped to the most-recent
   * frames. Identical to v3 (and to Go `Write`).
   */
  write(src: Float32Array): void {
    let nFrames = Math.floor(src.length / this.nc);
    if (nFrames === 0) return;
    if (nFrames > this.capacity) {
      const skip = nFrames - this.capacity;
      src = src.subarray(skip * this.nc);
      nFrames = this.capacity;
    }
    const wp = this.writePos;
    this.writeToRing(src, wp, nFrames);
    this.writePos = wp + nFrames; // release: data before position (SAB mode)
  }

  /**
   * Copy up to `dst.length` interleaved samples from the ring, returning
   * true when audio was produced. Order per read (Go `Read`): startup gate →
   * pending macro-trim → capacity valves → sense → underrun valve → play
   * with debt-bucket splice.
   */
  read(dst: Float32Array): boolean {
    const nc = this.nc;
    const nFrames = Math.floor(dst.length / nc);
    if (nFrames === 0) return true;
    const wp = this.writePos; // acquire (SAB mode)
    let rp = this.readPos;
    const sp = this.setpoint;

    // 1. STARTUP — wait for a full operating point, then place exactly at
    // the setpoint (pre-read fill = sp).
    if (rp === 0) {
      if (wp < sp) {
        dst.fill(0);
        return false;
      }
      rp = wp - sp;
      this.readPos = rp;
      this.started = true;
      this.lastWp = wp; // deliveries before start are not a resume event
    }

    // 2. Pending macro-trim (confirmed last window close): one discard,
    // clamped so post-trim fill lands at the setpoint, never below.
    if (this.pendingTrim > 0) {
      const t = Math.min(this.pendingTrim, wp - rp - sp);
      if (t > 0) {
        rp += t;
        this.readPos = rp;
        this.trims++;
      }
      this.pendingTrim = 0;
      this.resetWindow();
    }

    let fill = wp - rp;

    // 3. Capacity valves: lap (writer wrapped a stalled reader) and the
    // near-capacity guard. Gross events only; snap to the setpoint.
    if (fill >= this.capacity) {
      rp = wp - sp;
      this.readPos = rp;
      fill = sp;
      this.laps++;
      this.resetWindow();
    } else if (fill > this.capacity - (this.w + this.r)) {
      rp = wp - sp;
      this.readPos = rp;
      fill = sp;
      this.overruns++;
      this.resetWindow();
    }

    // 4. Sense at read entry, before the buffer acts (underrun dips are
    // lateness evidence and must be seen; the freeze rule excises
    // sustained absence).
    this.sense(wp, fill);

    // 5. UNDERRUN — physical valve: silence, rp frozen (the free
    // whole-packet time-slip).
    if (fill < nFrames) {
      dst.fill(0);
      this.underruns++;
      return false;
    }

    // 6. PLAY, paying splice debt at most one per read.
    if (!this.frozen) {
      this.debt += (this.rate * this.r) / this.sampleRate;
      if (this.debt > 1.5) this.debt = 1.5; // Go: v4MaxDebt
      else if (this.debt < -1.5) this.debt = -1.5;
    }
    if (this.debt >= 1 && fill >= nFrames + 1) {
      this.debt--;
      this.spliceDrop(dst, rp, nFrames);
      this.readPos = rp + nFrames + 1;
      this.samplesDropped++;
    } else if (this.debt <= -1 && nFrames >= 2) {
      this.debt++;
      this.spliceInsert(dst, rp, nFrames);
      this.readPos = rp + nFrames - 1;
      this.samplesInserted++;
    } else {
      this.readFromRing(dst, rp, nFrames);
      this.readPos = rp + nFrames;
    }
    return true;
  }

  /**
   * Feed one pre-read observation into the sensing window and run the
   * freeze/excise rule (design §3): a no-delivery run longer than
   * `freezeReads` freezes sensing and the servo, and retroactively rewinds
   * the run's own dip samples out of the window — outage, loss bursts and
   * device stalls teach the estimator nothing. Short dips (a late packet, a
   * lost packet) stay in: they are lateness evidence.
   */
  private sense(wp: number, fill: number): void {
    if (!this.started) return;
    const tick = this.tick;
    this.tick++;

    const delta = wp - this.lastWp;
    this.lastWp = wp;
    if (delta > 0) {
      if (this.frozen) {
        this.frozen = false;
        this.resetWindow(); // fresh, contiguous-delivery window
        // The frozen span moved the virtual signal by the absence; a slope
        // across it would read as huge drift. Excise.
        this.ffLen = 0;
      }
      this.gapRun = 0;
    } else {
      this.gapRun++;
      if (!this.frozen && this.gapRun > this.freezeReads) {
        this.frozen = true;
        this.rate = 0;
        this.debt = 0;
        const rw = Math.min(this.gapRun - 1, this.winCount);
        this.winCount -= rw;
      }
    }
    if (this.frozen) return;

    this.rawBuf[this.winCount] = fill;
    this.virtBuf[this.winCount] = wp - this.r * tick;
    this.winCount++;
    if (this.winCount >= this.windowReads) {
      this.closeWindow();
    }
  }

  /**
   * Compute the window estimates, update the width hold, the FF drift
   * estimate and the setpoint, then run the shipped control law (Go
   * `closeWindow` + `PServo.Decide`, inlined — the TS build is monomorphic;
   * the Go Controller seam is test scaffolding the port does not need).
   */
  private closeWindow(): void {
    const n = this.winCount;
    // winStats(rawBuf)
    let s = this.sortBuf.subarray(0, n);
    s.set(this.rawBuf.subarray(0, n));
    s.sort();
    const medRaw = s[n >> 1]!;
    const minRaw = s[0]!;
    // winStats(virtBuf)
    s = this.sortBuf.subarray(0, n);
    s.set(this.virtBuf.subarray(0, n));
    s.sort();
    const medV = s[n >> 1]!;
    const minV = s[0]!;
    const maxV = s[n - 1]!;

    // Per-side widths from the virtual (actuation-free) view, rank-filtered
    // over K windows (rank 3 of 10: a one-off event pollutes at most TWO
    // windows — it can straddle a boundary — so rank 3 excludes any single
    // event while genuine recurrence is learned at full amplitude), then
    // held eager-up / reluctant-down.
    const wlMeas = Math.min(medV - minV, this.wlCap);
    const whMeas = maxV - medV;
    this.wlHist[this.histPos] = wlMeas;
    this.whHist[this.histPos] = whMeas;
    this.histPos = (this.histPos + 1) % 10; // Go: v4WidthHistory
    if (this.histLen < 10) this.histLen++;
    const wlEff = this.rankNth(this.wlHist, this.histLen, 3); // Go: v4WidthRank
    const whEff = this.rankNth(this.whHist, this.histLen, 3);
    this.wl = Math.max(wlEff, this.wl - this.narrowStep);
    this.wh = Math.max(whEff, this.wh - this.narrowStep);

    // Feed-forward drift estimate: slope of the window-median virtual
    // signal across the K-window horizon, in frames per second of read
    // time. Median-of-triplet endpoints + a half-quantum dead zone: raw
    // endpoints chase jitter noise (phantom-drift trickle), a full
    // Theil–Sen breaks staircase repayment; the triplet median outvotes any
    // single swung window while a step between the triplets registers
    // fully, and the repay integral stays exact — one whole quantum per
    // event.
    if (this.ffLen >= 10) {
      this.ffTicks.copyWithin(0, 1);
      this.ffVals.copyWithin(0, 1);
      this.ffLen = 9;
    }
    this.ffTicks[this.ffLen] = this.tick;
    this.ffVals[this.ffLen] = medV;
    this.ffLen++;
    let slope = 0;
    const fn = this.ffLen;
    if (fn >= 2 && this.ffTicks[fn - 1]! > this.ffTicks[0]!) {
      let lo = this.ffVals[0]!;
      let hi = this.ffVals[fn - 1]!;
      let ta = this.ffTicks[0]!;
      let tb = this.ffTicks[fn - 1]!;
      if (fn >= 6) {
        lo = this.median3(this.ffVals[0]!, this.ffVals[1]!, this.ffVals[2]!);
        hi = this.median3(this.ffVals[fn - 3]!, this.ffVals[fn - 2]!, this.ffVals[fn - 1]!);
        ta = this.ffTicks[1]!;
        tb = this.ffTicks[fn - 2]!;
      }
      const span = tb - ta;
      if (span > 0) {
        const perRead = (hi - lo) / span;
        const d = perRead * span;
        if (d >= this.ffDeadZone || d <= -this.ffDeadZone) {
          slope = (perRead * this.sampleRate) / this.r;
        }
      }
    }

    // Deadband spans the FULL measured late-side envelope: displacement
    // smaller than the jitter envelope is noise, and chasing it would
    // re-commit the v2 sin. Under clean networks it stays at the small base
    // so inherited whole-packet offsets are drained promptly; in bursty
    // regimes the anchor cedes entirely to the macro-trim, whose min-window
    // confirmation already outranks the envelope.
    const dbEff = Math.max(this.deadband, Math.trunc(this.wl));
    const sp = Math.min(Math.max(this.spBase + Math.trunc(this.kLow * this.wl + 0.5), this.flDecl), this.spMax);
    this.setpoint = sp;

    // The shipped law (Go PServo.Decide): macro-trim on confirmed
    // whole-window displacement, else slow drop-only P anchor + soft-
    // clamped FF, capped.
    let rate = 0;
    let trim = 0;
    if (minRaw - sp > this.theta) {
      // Confirmed offset: even the window MIN stayed displaced. Sized by
      // the median so the trim lands at the setpoint, residue-free.
      trim = medRaw - sp;
    } else {
      // Soft clamp: pass through inside ±C, roll off as C²/|s| beyond —
      // continuous at ±C, → 0 as |s| → ∞. Sustained loss reads as a slow
      // writer in the fill domain; the roll-off sends the FF to ~zero there
      // instead of grinding inserts at the cap (v3's never-spend-
      // corrections-on-loss doctrine, kept continuously).
      let ff = slope;
      if (ff > this.ffClamp || ff < -this.ffClamp) {
        ff = (this.ffClamp * this.ffClamp) / ff;
      }
      let p = 0;
      const e = medRaw - sp;
      if (e > dbEff) {
        p = this.gain * (e - dbEff);
      }
      rate = p + ff;
      if (rate > this.rateCap) rate = this.rateCap;
      else if (rate < -this.rateCap) rate = -this.rateCap;
    }
    this.rate = rate;
    if (trim > 0) this.pendingTrim = trim;
    this.resetWindow();
  }

  private resetWindow(): void {
    this.winCount = 0;
  }

  /**
   * The n-th highest of the first `len` values — the width recurrence
   * filter (Go `rankNth`). Short histories use their lowest value (a width
   * is only believed once seen n times).
   */
  private rankNth(h: Float64Array, len: number, n: number): number {
    if (len < n) n = len;
    const top = [-Infinity, -Infinity, -Infinity, -Infinity, -Infinity, -Infinity, -Infinity, -Infinity]; // n ≤ 8; descending
    for (let k = 0; k < len; k++) {
      const v = h[k]!;
      for (let i = 0; i < n; i++) {
        if (v > top[i]!) {
          for (let j = n - 1; j > i; j--) top[j] = top[j - 1]!;
          top[i] = v;
          break;
        }
      }
    }
    return top[n - 1]!;
  }

  /** Median of three values (Go `median3`). */
  private median3(a: number, b: number, c: number): number {
    if (a > b) {
      const t = a;
      a = b;
      b = t;
    }
    if (b > c) b = c;
    return a > b ? a : b;
  }

  /** Greatest common divisor of two positive values (Go `gcd`). */
  private gcd(a: number, b: number): number {
    while (b !== 0) {
      const t = a % b;
      a = b;
      b = t;
    }
    return a;
  }

  /**
   * Consume nFrames+1 ring frames into an nFrames output with one frame
   * removed at the grade-1 cut: the adjacent consumed pair with the minimum
   * summed per-channel discontinuity, averaged into a single boundary frame
   * (the decoder's drop marker, at any in-block position). Go `spliceDrop`.
   */
  private spliceDrop(dst: Float32Array, rp: number, nFrames: number): void {
    const nc = this.nc;
    const cap = this.capacity;
    let cut = 0;
    let best = -1;
    for (let i = 0; i < nFrames; i++) {
      const d = this.frameDiff(rp + i, rp + i + 1);
      if (best < 0 || d <= best) {
        // <=: later cut wins ties (v3-shaped tail)
        best = d;
        cut = i;
      }
    }
    for (let j = 0; j < nFrames; j++) {
      let src = rp + j;
      if (j > cut) src++;
      const sb = (src % cap) * nc;
      const db = j * nc;
      if (j === cut) {
        const nb = ((src + 1) % cap) * nc;
        for (let ch = 0; ch < nc; ch++) {
          dst[db + ch] = Math.fround(this.data[sb + ch]! + this.data[nb + ch]!) * 0.5;
        }
        continue;
      }
      for (let ch = 0; ch < nc; ch++) dst[db + ch] = this.data[sb + ch]!;
    }
  }

  /**
   * Consume nFrames−1 ring frames into an nFrames output with one synthetic
   * frame added at the grade-1 cut: the average of the adjacent pair around
   * it (the decoder's insert marker). Go `spliceInsert`.
   */
  private spliceInsert(dst: Float32Array, rp: number, nFrames: number): void {
    const nc = this.nc;
    const cap = this.capacity;
    let cut = 1;
    let best = -1;
    for (let i = 1; i < nFrames; i++) {
      const d = this.frameDiff(rp + i - 1, rp + i);
      if (best < 0 || d <= best) {
        best = d;
        cut = i;
      }
    }
    for (let j = 0; j < nFrames; j++) {
      let src = rp + j;
      if (j > cut) src--;
      const db = j * nc;
      if (j === cut) {
        const ab = ((rp + cut - 1) % cap) * nc;
        const bb = ((rp + cut) % cap) * nc;
        for (let ch = 0; ch < nc; ch++) {
          dst[db + ch] = Math.fround(this.data[ab + ch]! + this.data[bb + ch]!) * 0.5;
        }
        continue;
      }
      const sb = (src % cap) * nc;
      for (let ch = 0; ch < nc; ch++) dst[db + ch] = this.data[sb + ch]!;
    }
  }

  /**
   * Summed per-channel discontinuity between two frames, in float32
   * arithmetic (`Math.fround` mirrors Go's float32 ops so the placement
   * scan picks the identical cut — the fixture replay depends on it).
   */
  private frameDiff(p: number, q: number): number {
    const nc = this.nc;
    const cap = this.capacity;
    const pb = (p % cap) * nc;
    const qb = (q % cap) * nc;
    let d = 0;
    for (let ch = 0; ch < nc; ch++) {
      let x = Math.fround(this.data[pb + ch]! - this.data[qb + ch]!);
      if (x < 0) x = -x;
      d = Math.fround(d + x);
    }
    return d;
  }

  /** Current fill in frames. */
  fillFrames(): number {
    return this.writePos - this.readPos;
  }

  /** Fill in interleaved floats (matching the Go ICircularBuffer convention). */
  getBehind(): number {
    return this.fillFrames() * this.nc;
  }

  /** Rich snapshot for tuning/observability (Go `Snapshot`). */
  snapshot(): JitterBufferSnapshot {
    const srMs = this.sampleRate / 1000;
    const fill = this.fillFrames();
    const wl = Math.trunc(this.wl);
    const wh = Math.trunc(this.wh);
    return {
      fillFrames: fill,
      fillMs: fill / srMs,
      setpointFrames: this.setpoint,
      setpointMs: this.setpoint / srMs,
      widthLowFrames: wl,
      widthLowMs: wl / srMs,
      widthHighFrames: wh,
      widthHighMs: wh / srMs,
      ratePerSec: this.rate,
      frozen: this.frozen,
      started: this.readPos > 0,
      underruns: this.underruns,
      overruns: this.overruns,
      laps: this.laps,
      trims: this.trims,
      samplesDropped: this.samplesDropped,
      samplesInserted: this.samplesInserted,
    };
  }

  /**
   * Copy `nFrames` frames from `src` into the ring at frame position `wp`,
   * handling wraparound. Caller guarantees `nFrames <= capacity`.
   */
  private writeToRing(src: Float32Array, wp: number, nFrames: number): void {
    const cap = this.capacity;
    const nc = this.nc;
    const startFrame = wp % cap;
    if (startFrame + nFrames <= cap) {
      this.data.set(src.subarray(0, nFrames * nc), startFrame * nc);
      return;
    }
    const first = cap - startFrame;
    this.data.set(src.subarray(0, first * nc), startFrame * nc);
    this.data.set(src.subarray(first * nc, nFrames * nc), 0);
  }

  /**
   * Copy `nFrames` frames from the ring at frame position `rp` into `dst`,
   * handling wraparound. Caller guarantees `nFrames <= capacity`.
   */
  private readFromRing(dst: Float32Array, rp: number, nFrames: number): void {
    const cap = this.capacity;
    const nc = this.nc;
    const startFrame = rp % cap;
    if (startFrame + nFrames <= cap) {
      dst.set(this.data.subarray(startFrame * nc, (startFrame + nFrames) * nc));
      return;
    }
    const first = cap - startFrame;
    dst.set(this.data.subarray(startFrame * nc, cap * nc), 0);
    dst.set(this.data.subarray(0, (nFrames - first) * nc), first * nc);
  }
}
