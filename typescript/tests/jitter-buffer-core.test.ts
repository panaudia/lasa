/**
 * White-box tests for the v4 jitter-buffer core (port of
 * panaudia-engine/buffers/jitter_buffer.go; plan/jitter-v4-ts-port.md
 * phase 3).
 *
 * The heavy behavioral load — the controller's every decision across the
 * golden scenarios — is carried by jitter-v4-replay.test.ts (ledger
 * equality against the Go reference). This file keeps the small
 * mechanism tests worth having close to the code: ring mechanics, the
 * grade-1 splice shapes, valves, the SAB cross-thread mode, capacity
 * derivation, and the quality-level mapping.
 *
 * Test seams: `debt`, `pendingTrim`, `readPos`/`writePos` are public
 * mutable fields (v3 convention) — a window close takes windowReads
 * (= 2 s) reads, so unit tests seed the actuator state directly instead
 * of simulating minutes of stream.
 */

import { describe, it, expect } from 'vitest';
import { JitterBufferCore, computeJitterCapacity, fecFloor, type JitterBufferCoreConfig } from '../src/audio/jitter-buffer-core.js';

const F = 48; // frames per ms at 48 kHz

/**
 * Small explicit geometry for branch/splice tests: W = R = 10, S = 10 →
 * spBase = setpoint = 20; capacity pinned to 200 → near-capacity valve at
 * fill > 180, lap at ≥ 200; Θ = 50. windowReads is derived (9600 at
 * R = 10), so no window ever closes in these tests — the controller stays
 * quiescent and splices fire only from seeded debt.
 */
function branchBuf(nc: number): JitterBufferCore {
  return new JitterBufferCore({
    sampleRate: 48000,
    numChannels: nc,
    readerFrame: 10,
    writerFrame: 10,
    safety: 10,
    capacity: 200,
  });
}

/** Fill the ring so ring[i] = i (mono). */
function indexRing(core: JitterBufferCore): void {
  for (let i = 0; i < core.data.length; i++) core.data[i] = i;
}

function arr(a: Float32Array): number[] {
  return Array.from(a);
}

// ---- construction / derived geometry -------------------------------------

describe('construction', () => {
  it('requires explicit geometry', () => {
    expect(() => new JitterBufferCore({})).toThrow(/readerFrame and writerFrame/);
    expect(() => new JitterBufferCore({ writerFrame: 240 })).toThrow();
    expect(() => new JitterBufferCore({ writerFrame: 240, readerFrame: -1 })).toThrow();
  });

  it('derives the browser-geometry tuning (240/128)', () => {
    const j = new JitterBufferCore({ writerFrame: 240, readerFrame: 128 });
    expect(j.w).toBe(240);
    expect(j.r).toBe(128);
    expect(j.s).toBe(48); // 1 ms default
    expect(j.spBase).toBe(368); // max(R+S, R+W) — the F1 lattice floor
    expect(j.setpoint).toBe(368); // warm start: zero measured width
    expect(j.windowReads).toBe(750); // 2 s at the 2.667 ms worklet cadence
    expect(j.freezeReads).toBe(15); // 40 ms at R=128
    expect(j.ffDeadZone).toBe(8); // gcd(240,128)/2 — the phase-0 W≠R finding
    expect(j.theta).toBe(280); // W + rateCap·windowSec
  });

  it('derives the server-geometry tuning (W = R = 240)', () => {
    const j = new JitterBufferCore({ writerFrame: 240, readerFrame: 240 });
    expect(j.spBase).toBe(480); // max(R+S, R+W) = R + W in square geometry
    expect(j.windowReads).toBe(400);
    expect(j.freezeReads).toBe(8);
    expect(j.ffDeadZone).toBe(120); // gcd/2 = W/2 in square geometry
  });

  it('computeJitterCapacity matches the constructor for every config shape', () => {
    const cases: JitterBufferCoreConfig[] = [
      { writerFrame: 240, readerFrame: 128 },
      { writerFrame: 240, readerFrame: 128, numChannels: 2 },
      { writerFrame: 240, readerFrame: 240 },
      { writerFrame: 240, readerFrame: 128, qualityLevel: 1 },
      { writerFrame: 240, readerFrame: 128, qualityLevel: 2 },
      { writerFrame: 240, readerFrame: 128, floor: 2976 },
      { writerFrame: 240, readerFrame: 128, robustness: 4 },
      { writerFrame: 240, readerFrame: 128, capacity: 9600 },
      { writerFrame: 960, readerFrame: 240, floor: 2400, robustness: 8, capacity: 19200 },
    ];
    for (const cfg of cases) {
      const { capacity, nc } = computeJitterCapacity(cfg);
      const j = new JitterBufferCore(cfg);
      expect(j.capacity, JSON.stringify(cfg)).toBe(capacity);
      expect(j.nc, JSON.stringify(cfg)).toBe(nc);
    }
  });

  it('quality levels map to {floor, robustness, capacity} (Go QualityLevel)', () => {
    const g = { writerFrame: 240, readerFrame: 128 };
    const l0 = new JitterBufferCore({ ...g });
    expect([l0.flDecl, l0.setpoint]).toEqual([0, 368]); // interactive: zero-value config

    const l1 = new JitterBufferCore({ ...g, qualityLevel: 1 });
    expect(l1.flDecl).toBe(2400); // 50 ms
    expect(l1.capacity).toBe(24000); // 500 ms
    expect(l1.setpoint).toBe(2400); // joins at the declared floor

    const l2 = new JitterBufferCore({ ...g, qualityLevel: 2 });
    expect(l2.flDecl).toBe(7200); // 150 ms
    expect(l2.capacity).toBe(19200); // 400 ms
    expect(l2.setpoint).toBe(7200);

    // Explicit robustness/capacity override the level's values; the floor
    // does NOT override — floors compose by max (next test).
    const ov = new JitterBufferCore({ ...g, qualityLevel: 2, floor: 2976, robustness: 1 });
    expect(ov.flDecl).toBe(7200); // max(2976, level-2 floor 7200)
    expect(ov.capacity).toBe(19200); // level capacity kept where not overridden
  });

  it('floors compose by max, never override or stack (Go engine.AddSource)', () => {
    const g = { writerFrame: 240, readerFrame: 128 };
    // Explicit floor above the level's: explicit wins.
    const hi = new JitterBufferCore({ ...g, qualityLevel: 1, floor: 4800 });
    expect(hi.flDecl).toBe(4800); // max(4800, 50 ms = 2400)
    // Explicit floor below the level's: the level's wins (no override).
    const lo = new JitterBufferCore({ ...g, qualityLevel: 1, floor: 960 });
    expect(lo.flDecl).toBe(2400);
    // computeJitterCapacity uses the same composition (ctor-match guard).
    for (const cfg of [
      { ...g, qualityLevel: 1, floor: 4800 },
      { ...g, qualityLevel: 1, floor: 960 },
      { ...g, qualityLevel: 2, floor: fecFloor(7, 240) },
    ]) {
      const { capacity } = computeJitterCapacity(cfg);
      expect(new JitterBufferCore(cfg).capacity, JSON.stringify(cfg)).toBe(capacity);
    }
  });

  it('fecFloor covers loss clusters: 2×(offset+2)×W; zero offset = no floor (Go buffers.FECFloor)', () => {
    expect(fecFloor(0, 240)).toBe(0);
    expect(fecFloor(-1, 240)).toBe(0);
    expect(fecFloor(4, 240)).toBe(2880); // 2×(4+2)×240 = 60 ms at 48 kHz
    expect(fecFloor(7, 240)).toBe(4320); // the declared-max bound: 90 ms
    // Composes with a quality level by max through the config floor.
    const g = { writerFrame: 240, readerFrame: 128 };
    const j = new JitterBufferCore({ ...g, qualityLevel: 1, floor: fecFloor(7, 240) });
    expect(j.flDecl).toBe(4320); // max(FEC 4320, level-1 2400)
  });

  it('an explicit small capacity clamps the setpoint cap (valve headroom reserve)', () => {
    const j = branchBuf(1);
    expect(j.capacity).toBe(200);
    // reserve = 4·max(W,R) = 40 ⇒ spMax = capacity − reserve = 160.
    expect(j.spMax).toBe(160);
    expect(j.setpoint).toBe(20);
  });
});

// ---- write ----------------------------------------------------------------

describe('write', () => {
  it('fill accounting', () => {
    const j = new JitterBufferCore({ writerFrame: 20 * F, readerFrame: 5 * F });
    j.write(new Float32Array(100));
    expect(j.fillFrames()).toBe(100);
    j.write(new Float32Array(50));
    expect(j.fillFrames()).toBe(150);
  });

  it('wraps transparently', () => {
    const j = new JitterBufferCore({ writerFrame: 20 * F, readerFrame: 5 * F });
    j.writePos = j.capacity - 2; // two frames before wrap
    j.write(Float32Array.from([1, 2, 3, 4, 5]));
    expect(j.data[j.capacity - 2]).toBe(1);
    expect(j.data[j.capacity - 1]).toBe(2);
    expect(j.data[0]).toBe(3);
    expect(j.data[1]).toBe(4);
    expect(j.data[2]).toBe(5);
  });

  it('clips over-capacity writes to the last capacity frames', () => {
    const j = new JitterBufferCore({ writerFrame: 20 * F, readerFrame: 5 * F });
    const cap = j.capacity;
    const src = new Float32Array(cap + 10);
    for (let i = 0; i < src.length; i++) src[i] = i;
    j.write(src);
    expect(j.fillFrames()).toBe(cap);
    expect(j.data[0]).toBe(10);
    expect(j.data[cap - 1]).toBe(cap + 9);
  });
});

// ---- read path: startup, valves, trim ------------------------------------

describe('read path', () => {
  it('startup silence when fill < setpoint', () => {
    const j = branchBuf(1); // setpoint 20
    j.write(new Float32Array(15));
    const dst = Float32Array.from([9, 9, 9, 9]);
    expect(j.read(dst)).toBe(false);
    expect(j.readPos).toBe(0);
    expect(arr(dst)).toEqual([0, 0, 0, 0]);
  });

  it('startup places exactly at the setpoint and plays', () => {
    const j = branchBuf(1);
    indexRing(j);
    j.writePos = 100; // fill 100 >= sp 20
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    // place rp = 100 − 20 = 80; plain read ring[80..83]; rp 84
    expect(j.readPos).toBe(84);
    expect(arr(dst)).toEqual([80, 81, 82, 83]);
  });

  it('lap snaps to the setpoint and plays', () => {
    const j = branchBuf(1);
    indexRing(j);
    j.writePos = 300;
    j.readPos = 50; // fill 250 >= capacity 200
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    expect(j.laps).toBe(1);
    // snap rp = 300 − 20 = 280; play ring[280 % 200 = 80..]; rp 284
    expect(j.readPos).toBe(284);
    expect(arr(dst)).toEqual([80, 81, 82, 83]);
  });

  it('near-capacity valve snaps to the setpoint and plays', () => {
    const j = branchBuf(1); // valve above 200 − (10+10) = 180
    indexRing(j);
    j.writePos = 300;
    j.readPos = 110; // fill 190 > 180, < capacity
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    expect(j.overruns).toBe(1);
    expect(j.readPos).toBe(284);
    expect(arr(dst)).toEqual([80, 81, 82, 83]);
  });

  it('underrun silences without advancing rp (the free time-slip)', () => {
    const j = branchBuf(1);
    j.writePos = 102;
    j.readPos = 100; // fill 2 < nFrames 4
    const dst = Float32Array.from([9, 9, 9, 9]);
    expect(j.read(dst)).toBe(false);
    expect(j.underruns).toBe(1);
    expect(j.readPos).toBe(100);
    expect(arr(dst)).toEqual([0, 0, 0, 0]);
  });

  it('plays plain with no correction when debt is quiet', () => {
    const j = branchBuf(1);
    indexRing(j);
    j.writePos = 130;
    j.readPos = 100; // fill 30
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    expect(j.readPos).toBe(104);
    expect(j.samplesInserted).toBe(0);
    expect(j.samplesDropped).toBe(0);
    expect(arr(dst)).toEqual([100, 101, 102, 103]);
  });

  it('pending macro-trim discards in one event, clamped to land at the setpoint', () => {
    const j = branchBuf(1);
    indexRing(j);
    j.writePos = 200;
    j.readPos = 100; // fill 100
    j.pendingTrim = 30;
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    expect(j.trims).toBe(1);
    // rp += 30 → 130; play ring[130..133]; rp 134
    expect(j.readPos).toBe(134);
    expect(arr(dst)).toEqual([130, 131, 132, 133]);

    // Clamp: a trim larger than fill − sp lands exactly at the setpoint.
    j.pendingTrim = 1000; // fill now 200 − 134 = 66; max discard 66 − 20 = 46
    expect(j.read(dst)).toBe(true);
    expect(j.trims).toBe(2);
    expect(j.writePos - j.readPos).toBe(20 - 4); // post-read fill = sp − nFrames
  });
});

// ---- the grade-1 splice ---------------------------------------------------

describe('grade-1 splice (debt-seeded)', () => {
  it('drop on a uniform ramp cuts at block end (later ties win — v3-shaped tail)', () => {
    const j = branchBuf(1);
    indexRing(j); // all adjacent diffs equal
    j.writePos = 160;
    j.readPos = 100;
    j.debt = 1;
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    // consume 5 (ring[100..104]); cut at last pair: tail = avg(103, 104) = 103.5
    expect(arr(dst)).toEqual([100, 101, 102, 103.5]);
    expect(j.readPos).toBe(105);
    expect(j.samplesDropped).toBe(1);
    expect(j.debt).toBe(0);
  });

  it('drop cuts at the minimum-discontinuity pair inside the block', () => {
    const j = branchBuf(1);
    j.data.fill(1000); // make out-of-scan pairs expensive
    j.data[100] = 10;
    j.data[101] = 20;
    j.data[102] = 20; // the flat pair — the cut lands here
    j.data[103] = 30;
    j.data[104] = 40;
    j.writePos = 160;
    j.readPos = 100;
    j.debt = 1;
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    // cut = 1: out = [ring100, avg(101,102), ring103, ring104] — no step at all
    expect(arr(dst)).toEqual([10, 20, 30, 40]);
    expect(j.readPos).toBe(105);
  });

  it('insert on a uniform ramp synthesizes the v3-shaped tail', () => {
    const j = branchBuf(1);
    indexRing(j);
    j.writePos = 130;
    j.readPos = 100;
    j.debt = -1;
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    // consume 3 (ring[100..102]); tail = avg(102, peek 103) = 102.5
    expect(arr(dst)).toEqual([100, 101, 102, 102.5]);
    expect(j.readPos).toBe(103);
    expect(j.samplesInserted).toBe(1);
    expect(j.debt).toBe(0);
  });

  it('insert duplicates at a flat pair inside the block', () => {
    const j = branchBuf(1);
    j.data.fill(1000);
    j.data[100] = 10;
    j.data[101] = 20;
    j.data[102] = 20; // flat pair
    j.data[103] = 30;
    j.writePos = 130;
    j.readPos = 100;
    j.debt = -1;
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    // cut = 2: out = [ring100, ring101, avg(101,102)=20, ring102] = seamless repeat
    expect(arr(dst)).toEqual([10, 20, 20, 20]);
    expect(j.readPos).toBe(103);
  });

  it('drop across the ring wrap', () => {
    const j = branchBuf(1);
    indexRing(j); // ring[i] = i: the wrap [199]=199 → [0]=0 is the BIGGEST step,
    // so the scan avoids it and cuts at the last small pair — placement working as designed.
    j.writePos = 260;
    j.readPos = 198;
    j.debt = 1;
    const dst = new Float32Array(4);
    expect(j.read(dst)).toBe(true);
    // pairs: (198,199)=1, (199,0)=199, (0,1)=1, (1,2)=1 → later tie wins: cut=3
    // out = [198, 199, 0, avg(1,2)=1.5]
    expect(arr(dst)).toEqual([198, 199, 0, 1.5]);
    expect(j.readPos).toBe(203);
  });

  it('nFrames=1 precludes insert', () => {
    const j = branchBuf(1);
    j.data[100] = 7;
    j.writePos = 110;
    j.readPos = 100;
    j.debt = -1;
    const dst = new Float32Array(1);
    expect(j.read(dst)).toBe(true); // plain read
    expect(j.samplesInserted).toBe(0);
    expect(arr(dst)).toEqual([7]);
    expect(j.readPos).toBe(101);
  });
});

// ---- stereo (nc=2): frame-based corrections keep L/R sample-locked --------

describe('stereo (nc=2)', () => {
  it('drop splice averages per channel, keeps L/R locked', () => {
    const j = branchBuf(2);
    for (let f = 0; f < j.capacity; f++) {
      j.data[f * 2] = 10 * f;
      j.data[f * 2 + 1] = 10 * f + 1;
    }
    j.writePos = 160;
    j.readPos = 100;
    j.debt = 1;
    const dst = new Float32Array(8); // 4 stereo frames
    expect(j.read(dst)).toBe(true);
    // uniform diffs → cut at last pair: tail = avg(frame103, frame104) per ch = (1035, 1036)
    expect(arr(dst)).toEqual([1000, 1001, 1010, 1011, 1020, 1021, 1035, 1036]);
    expect(j.readPos).toBe(105);
    expect(j.samplesDropped).toBe(1);
  });

  it('steady browser-geometry stereo stream stays clean and sample-locked', () => {
    const j = new JitterBufferCore({ writerFrame: 240, readerFrame: 128, numChannels: 2 });
    const W = 240;
    const R = 128;
    let frame = 0;
    const writeBlock = () => {
      const buf = new Float32Array(W * 2);
      for (let i = 0; i < W; i++) {
        buf[i * 2] = frame % 1000;
        buf[i * 2 + 1] = (frame % 1000) + 0.5;
        frame++;
      }
      j.write(buf);
    };
    // Master-clock pacing (the Calm scenario's shape): writer due every W
    // frames, reader every R; writes due at-or-before a read land first.
    const read = new Float32Array(R * 2);
    let nextWrite = 0;
    let locked = true;
    for (let t = 0; t < 6000; t++) {
      const now = t * R;
      while (nextWrite * W <= now) {
        writeBlock();
        nextWrite++;
      }
      if (j.read(read)) {
        for (let k = 0; k < R; k++) {
          if (read[k * 2 + 1]! - read[k * 2]! !== 0.5) locked = false;
        }
      }
    }
    expect(locked).toBe(true); // channels never decorrelate
    expect(j.underruns).toBe(0);
    expect(j.laps).toBe(0);
    expect(j.overruns).toBe(0);
    expect(j.trims).toBe(0);
    expect(j.samplesDropped).toBe(0);
    expect(j.samplesInserted).toBe(0);
  });
});

// ---- freeze rule ----------------------------------------------------------

describe('freeze on absence', () => {
  it('sustained no-delivery freezes; a delivery unfreezes', () => {
    const j = branchBuf(1); // freezeReads = max(8, 1920/10) = 192
    indexRing(j);
    j.writePos = 60; // starts (>= sp 20), then no more deliveries
    const dst = new Float32Array(4);
    j.read(dst); // startup read
    expect(j.frozen).toBe(false);
    for (let i = 0; i < 200; i++) j.read(dst); // drains, then underruns; gapRun grows
    expect(j.frozen).toBe(true);
    expect(j.rate).toBe(0);
    j.write(new Float32Array(40)); // delivery resumes
    j.read(dst);
    expect(j.frozen).toBe(false);
  });
});

// ---- snapshot / stats ------------------------------------------------------

describe('snapshot & stats', () => {
  it('exposes the v4 shape', () => {
    const j = new JitterBufferCore({ writerFrame: 240, readerFrame: 128 });
    const s = j.snapshot();
    expect(s.setpointFrames).toBe(368);
    expect(s.setpointMs).toBeCloseTo(7.667, 2);
    expect(s.widthLowFrames).toBe(0);
    expect(s.widthHighFrames).toBe(0);
    expect(s.ratePerSec).toBe(0);
    expect(s.frozen).toBe(false);
    expect(s.started).toBe(false);
    expect(s.fillFrames).toBe(0);
    expect(s.trims).toBe(0);
  });

  it('started transitions on first successful read', () => {
    const j = new JitterBufferCore({ writerFrame: 240, readerFrame: 128 });
    expect(j.snapshot().started).toBe(false);
    for (let i = 0; i < 3; i++) j.write(new Float32Array(240));
    j.read(new Float32Array(128));
    expect(j.snapshot().started).toBe(true);
  });

  it('getBehind returns fill in interleaved floats', () => {
    const j = branchBuf(2);
    j.readPos = 100;
    j.writePos = 110; // 10 frames
    expect(j.getBehind()).toBe(20); // 10 frames × 2 channels
  });
});

// ---- SAB cross-thread mode -------------------------------------------------

describe('SAB cross-thread mode', () => {
  const cfg: JitterBufferCoreConfig = { sampleRate: 48000, numChannels: 2, readerFrame: 128, writerFrame: 240 };

  /** Allocate the shared ring + writePos and return writer and reader views of it. */
  function sharedPair() {
    const { capacity, nc } = computeJitterCapacity(cfg);
    const storage = new Float32Array(new SharedArrayBuffer(capacity * nc * 4));
    const writePos = new BigInt64Array(new SharedArrayBuffer(8));
    const writer = new JitterBufferCore({ ...cfg, sharedStorage: storage, sharedWritePos: writePos });
    const reader = new JitterBufferCore({ ...cfg, sharedStorage: storage, sharedWritePos: writePos });
    return { writer, reader, storage, writePos, capacity };
  }

  /** Interleaved stereo ramp: frame i → L=i, R=i+0.5. */
  function ramp(frames: number): Float32Array {
    const pcm = new Float32Array(frames * 2);
    for (let i = 0; i < frames; i++) {
      pcm[i * 2] = i;
      pcm[i * 2 + 1] = i + 0.5;
    }
    return pcm;
  }

  it('rejects sharedStorage of the wrong length', () => {
    const { capacity, nc } = computeJitterCapacity(cfg);
    const wrong = new Float32Array(capacity * nc - 2);
    expect(() => new JitterBufferCore({ ...cfg, sharedStorage: wrong })).toThrow(/sharedStorage length/);
  });

  it("the writer's writePos is visible to the reader through the atomic cell", () => {
    const { writer, reader, writePos } = sharedPair();
    expect(reader.writePos).toBe(0);
    writer.write(ramp(1000));
    expect(Number(Atomics.load(writePos, 0))).toBe(1000); // released by the writer
    expect(reader.writePos).toBe(1000); // acquired by the reader, separate instance
    expect(writer.readPos).toBe(0); // writer never advances readPos
  });

  it('reader pulls writer-produced audio from the shared ring, L/R sample-locked', () => {
    const { writer, reader } = sharedPair();
    writer.write(ramp(1000));
    const out = new Float32Array(128 * 2);
    expect(reader.read(out)).toBe(true);
    // startup placement: rp = 1000 − setpoint(368) = 632 ⇒ first frame is ring frame 632.
    expect(out[0]).toBe(632);
    for (let i = 0; i < 128; i++) {
      expect(out[i * 2 + 1]! - out[i * 2]!).toBeCloseTo(0.5, 5);
    }
    expect(reader.writePos).toBe(1000); // reads never touch the shared writePos
  });

  it('interleaves writer writes and reader reads across the boundary', () => {
    const { writer, reader } = sharedPair();
    writer.write(ramp(700)); // > setpoint so the reader can start
    const out = new Float32Array(128 * 2);
    reader.read(out);
    expect(reader.readPos).toBeGreaterThan(0);
    writer.write(ramp(240));
    expect(reader.writePos).toBe(940); // reader sees the new total
    expect(reader.read(out)).toBe(true);
  });
});
