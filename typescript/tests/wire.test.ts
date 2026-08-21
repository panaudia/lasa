/**
 * Golden-vector tests: the TS codecs are byte-exact against the Go
 * `lasa/wire` implementation (vectors generated from it — see
 * tests/vectors/lasa-wire.json).
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  encodeMonoObjectInto,
  encodePoseInto,
  parseMonoObject,
  parsePose,
  parsePresence,
  parseSink,
  LOUDNESS_SILENT,
  POSE_SIZE,
  type Pose,
} from '../src/lasa/wire';

const here = dirname(fileURLToPath(import.meta.url));
interface Vec {
  name: string;
  hex: string;
  meta: unknown;
}
const vectors: Vec[] = JSON.parse(readFileSync(join(here, 'vectors/lasa-wire.json'), 'utf8'));
const byName = new Map(vectors.map((v) => [v.name, v]));
function bytes(name: string): Uint8Array {
  const v = byName.get(name);
  if (!v) throw new Error(`missing vector ${name}`);
  const hex = v.hex;
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}
function hexOf(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
}

describe('pose', () => {
  it('encodes byte-exactly (basic + clamped)', () => {
    for (const [name, pose] of [
      ['pose-basic', { x: 1.5, y: -2.25, z: 0.125, yaw: 1.0, pitch: -0.5, roll: 3.2 }],
      ['pose-clamp', { x: 0, y: 0, z: 0, yaw: -4.0, pitch: 0, roll: 0 }],
    ] as [string, Pose][]) {
      const out = new Uint8Array(POSE_SIZE);
      encodePoseInto(new DataView(out.buffer), 0, pose);
      expect(hexOf(out), name).toBe(byName.get(name)!.hex);
    }
  });
  it('round-trips through parse', () => {
    const p = parsePose(new DataView(bytes('pose-basic').buffer), 0);
    expect(p.x).toBeCloseTo(1.5, 6);
    expect(p.yaw).toBeCloseTo(1.0, 4);
    expect(p.roll).toBeCloseTo(Math.PI, 4); // clamped at +π
  });
});

describe('mono-object', () => {
  const audio = new Uint8Array([0xf8, 1, 2, 3, 4]);
  const red = new Uint8Array([0xf8, 9, 8]);
  const pose: Pose = { x: 1.5, y: -2.25, z: 0.125, yaw: 1.0, pitch: -0.5, roll: 3.2 };
  const scratch = new Uint8Array(1500);

  it.each([
    ['mono-pose-only', { pose }],
    ['mono-audio-only', { audio }],
    ['mono-both', { pose, audio }],
    ['mono-redundancy', { pose, audio, redundancy: { offset: 3, audio: red } }],
  ])('%s encodes byte-exactly', (name, pkt) => {
    const n = encodeMonoObjectInto(scratch, pkt);
    expect(hexOf(scratch.subarray(0, n))).toBe(byName.get(name)!.hex);
  });

  it('parses the redundancy layout', () => {
    const p = parseMonoObject(bytes('mono-redundancy'));
    expect(p.pose?.y).toBeCloseTo(-2.25, 6);
    expect(Array.from(p.audio!)).toEqual(Array.from(audio));
    expect(p.redundancy?.offset).toBe(3);
    expect(Array.from(p.redundancy!.audio)).toEqual(Array.from(red));
  });

  it('rejects sink flags (discard rule)', () => {
    expect(() => parseMonoObject(bytes('sink-ts'))).toThrowError(/mono-object/);
  });
});

describe('sink', () => {
  it('parses plain, ts, and redundancy layouts', () => {
    expect(parseSink(bytes('sink-plain')).timestampMicros).toBeUndefined();
    const ts = parseSink(bytes('sink-ts'));
    expect(ts.timestampMicros).toBe(1_722_900_000_123_456n);
    expect(ts.audio.length).toBe(5);
    const red = parseSink(bytes('sink-ts-redundancy'));
    expect(red.redundancy?.offset).toBe(1);
    expect(red.audio.length).toBe(5);
  });
});

describe('presence', () => {
  it('parses the keyframe with the head-frame flag', () => {
    const kf = parsePresence(bytes('presence-keyframe'));
    if (kf.kind !== 'keyframe') throw new Error('expected keyframe');
    expect(kf.gen).toBe(7);
    expect(kf.first).toBe(2);
    expect(kf.records.map((r) => r.id)).toEqual(['alice', 'angel-1']);
    expect(kf.records[0]!.headFrame).toBe(false);
    expect(kf.records[0]!.loudness).toBe(46);
    expect(kf.records[1]!.headFrame).toBe(true);
    expect(kf.records[1]!.loudness).toBe(LOUDNESS_SILENT);
    expect(kf.records[1]!.pose.y).toBeCloseTo(0.5, 6);
  });
  it('parses deltas', () => {
    const d = parsePresence(bytes('presence-delta'));
    if (d.kind !== 'delta') throw new Error('expected delta');
    expect(d.records.map((r) => r.index)).toEqual([2, 3]);
  });
});

// E8 (spec-audit 2026-08-21): the TS presence parser matches Go's
// strictness — a krecord id outside the §2 identifier grammar rejects
// the whole packet.
describe('presence krecord id validation', () => {
  it('rejects a keyframe carrying an invalid id', () => {
    // flags=keyframe, gen=0, first=0, then one krecord: kflags=0,
    // id-len=2, id="AB", pose(18 zero bytes), loudness=0.
    const bad = new Uint8Array([1, 0, 0, 0, 0, 2, 0x41, 0x42, ...new Array(19).fill(0)]);
    expect(() => parsePresence(bad)).toThrow(/invalid krecord id/);
    const good = new Uint8Array([1, 0, 0, 0, 0, 2, 0x61, 0x62, ...new Array(19).fill(0)]);
    expect(() => parsePresence(good)).not.toThrow();
  });
});
