/**
 * ramp-ledger.ts — TS port of the jitterlab black-box measurement chain
 * (panaudia-engine/jitterlab: signal.go + ledger.go + the runner's
 * gap/conceal attribution), for replaying exported trace fixtures against
 * the ported v4 core (plan/jitter-v4-ts-port.md phase 3).
 *
 * The buffer under test is never asked what it did — everything is
 * measured from its output via the self-describing ramp signal: stream
 * frame position n carries the value 1 + (n mod 2^20) (integers exact in
 * float32), 0.0 is reserved for cleared output, and the single-tap
 * averaging splice produces exact half-integer marker values that
 * classify by value, on sight.
 *
 * Test-only: never imported by production code.
 */

import { JitterBufferCore } from '../src/audio/jitter-buffer-core.js';

// ---- ramp codec (signal.go) ----------------------------------------------

/** Ramp modulus: wrap period 2^20 frames ≈ 21.8 s at 48 kHz. */
export const RAMP_WRAP = 1 << 20;

/** Write nFrames frames of ramp signal starting at stream position pos. */
export function fillRamp(dst: Float32Array, pos: number, nFrames: number, nc: number): void {
  for (let f = 0; f < nFrames; f++) {
    const v = 1 + ((pos + f) % RAMP_WRAP);
    const base = f * nc;
    for (let ch = 0; ch < nc; ch++) dst[base + ch] = v;
  }
}

/**
 * The exact splice-marker sample the single-tap average produces for
 * adjacent stream positions p, p+1. All quantities are exactly
 * representable in float32, so observed markers compare bit-exactly.
 */
function markerValue(p: number): number {
  const a = 1 + (p % RAMP_WRAP);
  const b = 1 + ((p + 1) % RAMP_WRAP);
  return (a + b) * 0.5;
}

/**
 * Map a wrapped position w to the absolute position congruent to w
 * (mod RAMP_WRAP) nearest to lastAbs+1 (the expected next frame).
 */
function unwrapPos(lastAbs: number, w: number): number {
  const next = lastAbs + 1;
  let d = (((w - (next % RAMP_WRAP)) % RAMP_WRAP) + RAMP_WRAP) % RAMP_WRAP;
  if (d > RAMP_WRAP / 2) d -= RAMP_WRAP;
  return next + d;
}

// ---- artifact ledger (ledger.go) -----------------------------------------

/** ArtifactKind values — must match the Go ArtifactKind iota order. */
export const enum ArtifactKind {
  Drop = 0,
  Insert = 1,
  Snap = 2,
  Silence = 3,
  Alien = 4,
  LossGap = 5,
}

/** One ledger entry: [kind, time, pos, delta] — the fixture's row shape. */
export type LedgerRow = [kind: number, time: number, pos: number, delta: number];

/**
 * The black-box decoder: a small state machine fed each read's output
 * block; reconstructs played stream positions and emits typed artifacts.
 * Marker classification is by value, on sight — it survives a
 * splice-read followed immediately by a snap-read (which destroys span
 * evidence). Silence before the first decoded position is join/startup
 * latency, not an artifact.
 */
export class RampDecoder {
  private lastPos = -1;
  private silenceOpen = false;
  private silenceTime = 0;
  private silenceFrames = 0;

  joinSilenceFrames = 0;
  alienSamples = 0;
  readonly artifacts: LedgerRow[] = [];

  constructor(private readonly nc: number) {}

  /** Decode one read block (interleaved; channel 0 is decoded — splices move whole frames). */
  feed(time: number, block: Float32Array): void {
    const nFrames = Math.floor(block.length / this.nc);
    for (let f = 0; f < nFrames; f++) {
      const v = block[f * this.nc]!;
      if (v === 0) {
        if (this.lastPos < 0) {
          this.joinSilenceFrames++;
          continue;
        }
        if (!this.silenceOpen) {
          this.silenceOpen = true;
          this.silenceTime = time;
          this.silenceFrames = 0;
        }
        this.silenceFrames++;
        continue;
      }
      const i = Math.trunc(v);
      const frac = v - i;
      if (frac === 0) {
        // Plain sample: a stream position (1..RAMP_WRAP), else alien.
        if (i >= 1 && i <= RAMP_WRAP) {
          this.closeSilence();
          this.plainSample(time, i - 1);
        } else {
          this.alienSamples++;
        }
      } else if (frac === 0.5) {
        this.closeSilence();
        this.markerSample(time, v);
      } else {
        this.alienSamples++;
      }
    }
  }

  private plainSample(time: number, w: number): void {
    if (this.lastPos < 0) {
      this.lastPos = w; // first audio ever: baseline, not an artifact
      return;
    }
    const pos = unwrapPos(this.lastPos, w);
    if (pos !== this.lastPos + 1) {
      this.artifacts.push([ArtifactKind.Snap, time, pos, pos - (this.lastPos + 1)]);
    }
    this.lastPos = pos;
  }

  /**
   * A splice marker, classified by value against the expected insert/drop
   * encodings for the current position: an insert plays a synthetic
   * preview of lastPos+1 without consuming it (lastPos unchanged); a drop
   * plays through lastPos+2 (lastPos advances), so the snap rule keeps
   * working across the marker in both cases.
   */
  private markerSample(time: number, v: number): void {
    if (this.lastPos < 0) {
      this.alienSamples++; // marker before any position: not producible
      return;
    }
    if (v === markerValue(this.lastPos)) {
      this.artifacts.push([ArtifactKind.Insert, time, this.lastPos + 1, 0]);
    } else if (v === markerValue(this.lastPos + 1)) {
      this.artifacts.push([ArtifactKind.Drop, time, this.lastPos + 2, 0]);
      this.lastPos += 2;
    } else {
      this.alienSamples++;
    }
  }

  private closeSilence(): void {
    if (!this.silenceOpen) return;
    this.silenceOpen = false;
    this.artifacts.push([ArtifactKind.Silence, this.silenceTime, this.lastPos, this.silenceFrames]);
  }

  /** Close any open state at end of run. */
  finish(): void {
    this.closeSilence();
  }
}

// ---- trace fixture replay (export.go schema + runner.go attribution) ------

/** The on-disk fixture schema (Go TraceFixture; nil slices marshal as null). */
export interface TraceFixture {
  name: string;
  geometry: { sampleRate: number; numChannels: number; writerFrames: number; readerFrames: number };
  config: { floorFrames: number; robustness: number; capacityFrames: number };
  duration: number;
  /** [time, kind, frames, pos]; kinds: 0 = write, 1 = read, 2 = lost, 3 = conceal-write. */
  events: Array<[number, number, number, number]>;
  ledger: LedgerRow[] | null;
  summary: TraceSummary;
}

export interface TraceSummary {
  drops: number;
  inserts: number;
  snaps: number;
  lossGaps: number;
  silenceRuns: number;
  silenceFrames: number;
  concealedFrames: number;
  alienSamples: number;
  joinTime: number;
}

export interface ReplayResult {
  ledger: LedgerRow[];
  summary: TraceSummary;
}

/**
 * Replay a fixture's event schedule against a fresh ported core and
 * measure the output exactly as the Go runner does: decode every read
 * block, then reclassify decoder artifacts that upstream loss or
 * concealment fully explains (snaps spanning unwritten/concealed ranges;
 * silence runs that are exactly delivered concealment zeros).
 */
export function replayTrace(fx: TraceFixture): ReplayResult {
  const g = fx.geometry;
  const nc = g.numChannels;
  const core = new JitterBufferCore({
    sampleRate: g.sampleRate,
    numChannels: nc,
    writerFrame: g.writerFrames,
    readerFrame: g.readerFrames,
    floor: fx.config.floorFrames,
    robustness: fx.config.robustness,
    capacity: fx.config.capacityFrames,
  });
  const dec = new RampDecoder(nc);

  let maxW = 0;
  let maxR = 0;
  for (const [, kind, frames] of fx.events) {
    if ((kind === 0 || kind === 3) && frames > maxW) maxW = frames;
    if (kind === 1 && frames > maxR) maxR = frames;
  }
  const wbuf = new Float32Array(maxW * nc);
  const rbuf = new Float32Array(maxR * nc);

  type Gap = { pos: number; frames: number };
  const gaps: Gap[] = []; // KindLost ranges: stream positions never written
  const concealed: Gap[] = []; // conceal-write ranges: positions delivered as zeros
  let concealedFrames = 0;
  let joinTime = -1;

  for (const [time, kind, frames, pos] of fx.events) {
    switch (kind) {
      case 0: // write
      case 3: {
        // conceal-write
        const src = wbuf.subarray(0, frames * nc);
        if (kind === 3) {
          src.fill(0);
          concealed.push({ pos, frames });
          concealedFrames += frames;
        } else {
          fillRamp(src, pos, frames, nc);
        }
        core.write(src);
        break;
      }
      case 2: // lost: position accounting only
        gaps.push({ pos, frames });
        break;
      case 1: {
        // read
        const dst = rbuf.subarray(0, frames * nc);
        const produced = core.read(dst);
        dec.feed(time, dst);
        if (produced && joinTime < 0) joinTime = time;
        break;
      }
    }
  }
  dec.finish();

  // Reclassify: concealed entries can arrive out of position order (they
  // land at their give-up times), so sort first; loss gaps are naturally
  // position-ordered (scheduled at send time).
  concealed.sort((a, b) => a.pos - b.pos);
  const rangeIn = (list: Gap[], lo: number, hi: number): number => {
    let n = 0;
    for (const gp of list) {
      if (gp.pos >= hi) break;
      if (gp.pos >= lo) n += gp.frames;
    }
    return n;
  };
  const ledger = dec.artifacts;
  for (const a of ledger) {
    if (a[0] === ArtifactKind.Snap && a[3] > 0 && rangeIn(gaps, a[2] - a[3], a[2]) + rangeIn(concealed, a[2] - a[3], a[2]) === a[3]) {
      a[0] = ArtifactKind.LossGap;
    }
    // A silence run that is exactly a concealed range (its zeros played
    // back) is upstream concealment, not buffer starvation. The decoder
    // reports the run at the last position BEFORE it.
    if (a[0] === ArtifactKind.Silence && rangeIn(concealed, a[2] + 1, a[2] + 1 + a[3]) === a[3]) {
      a[0] = ArtifactKind.LossGap;
    }
  }

  const summary: TraceSummary = {
    drops: 0,
    inserts: 0,
    snaps: 0,
    lossGaps: 0,
    silenceRuns: 0,
    silenceFrames: 0,
    concealedFrames,
    alienSamples: dec.alienSamples,
    joinTime,
  };
  for (const a of ledger) {
    switch (a[0]) {
      case ArtifactKind.Drop:
        summary.drops++;
        break;
      case ArtifactKind.Insert:
        summary.inserts++;
        break;
      case ArtifactKind.Snap:
        summary.snaps++;
        break;
      case ArtifactKind.LossGap:
        summary.lossGaps++;
        break;
      case ArtifactKind.Silence:
        summary.silenceRuns++;
        summary.silenceFrames += a[3];
        break;
    }
  }
  return { ledger, summary };
}
