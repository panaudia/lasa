/**
 * The §7 receive rules, pinned against the PresenceRoster state
 * machine — mirrors client/roster_test.go: generation discard,
 * per-index latest-wins by group-id, tracking reset on generation
 * change, chunked-keyframe assembly, and the no-rewind property for
 * late keyframe chunks.
 */
import { describe, expect, it } from 'vitest';
import { PresenceRoster } from '../src/lasa/roster.js';
import type { Pose, PresenceKeyframe, PresenceDelta, KeyframeRecord, DeltaRecord } from '../src/lasa/wire.js';

const pose = (x: number): Pose => ({ x, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 });

const krec = (id: string, x: number, headFrame = false): KeyframeRecord => ({
  headFrame,
  id,
  pose: pose(x),
  loudness: 0,
});

const kf = (gen: number, first: number, records: KeyframeRecord[]): PresenceKeyframe => ({
  kind: 'keyframe',
  gen,
  first,
  records,
});

const drec = (index: number, x: number): DeltaRecord => ({ index, pose: pose(x), loudness: 0 });

const delta = (gen: number, records: DeltaRecord[]): PresenceDelta => ({ kind: 'delta', gen, records });

function find(r: PresenceRoster, id: string) {
  const e = r.snapshot().find((e) => e.id === id);
  expect(e, `entity ${id} in roster`).toBeDefined();
  return e!;
}

describe('PresenceRoster', () => {
  it('builds from a keyframe and applies same-gen deltas', () => {
    const r = new PresenceRoster();
    r.apply(10n, kf(1, 0, [krec('a', 1), krec('b', 2)]));
    expect(r.snapshot()).toHaveLength(2);
    r.apply(11n, delta(1, [drec(0, 5)]));
    expect(find(r, 'a').pose.x).toBe(5);
    expect(find(r, 'b').pose.x).toBe(2);
  });

  it('discards deltas with a stale or pre-keyframe generation', () => {
    const r = new PresenceRoster();
    r.apply(5n, delta(1, [drec(0, 9)]));
    expect(r.snapshot()).toHaveLength(0);
    r.apply(10n, kf(2, 0, [krec('a', 1)]));
    r.apply(11n, delta(1, [drec(0, 9)]));
    expect(find(r, 'a').pose.x).toBe(1);
  });

  it('applies records latest-wins by group-id (strictly exceeds)', () => {
    const r = new PresenceRoster();
    r.apply(10n, kf(1, 0, [krec('a', 1)]));
    r.apply(12n, delta(1, [drec(0, 12)]));
    r.apply(11n, delta(1, [drec(0, 11)])); // reordered older delta
    expect(find(r, 'a').pose.x).toBe(12);
    r.apply(12n, delta(1, [drec(0, 99)])); // equal group-id
    expect(find(r, 'a').pose.x).toBe(12);
  });

  it('resets all tracking on generation change (departure included)', () => {
    const r = new PresenceRoster();
    r.apply(10n, kf(1, 0, [krec('a', 1), krec('b', 2)]));
    r.apply(20n, kf(2, 0, [krec('a', 3), krec('c', 4)]));
    const snap = r.snapshot();
    expect(snap).toHaveLength(2);
    expect(snap.find((e) => e.id === 'b')).toBeUndefined();
    r.apply(21n, delta(2, [drec(1, 7)]));
    expect(find(r, 'c').pose.x).toBe(7);
  });

  it('assembles chunked keyframes without resetting within a gen', () => {
    const r = new PresenceRoster();
    r.apply(10n, kf(1, 0, [krec('a', 1), krec('b', 2)]));
    r.apply(11n, kf(1, 2, [krec('c', 3)]));
    expect(r.snapshot()).toHaveLength(3);
    r.apply(12n, delta(1, [drec(2, 9)]));
    expect(find(r, 'c').pose.x).toBe(9);
  });

  it('ignores deltas for unknown indexes (lost chunk)', () => {
    const r = new PresenceRoster();
    r.apply(10n, kf(1, 0, [krec('a', 1)]));
    r.apply(11n, delta(1, [drec(5, 9)]));
    expect(r.snapshot()).toHaveLength(1);
  });

  it('does not let a late keyframe chunk rewind a newer delta pose', () => {
    const r = new PresenceRoster();
    r.apply(10n, kf(1, 0, [krec('a', 1)]));
    r.apply(12n, delta(1, [drec(0, 12)]));
    r.apply(11n, kf(1, 0, [krec('a', 1)])); // periodic re-list, reordered
    expect(find(r, 'a').pose.x).toBe(12);
    r.apply(13n, kf(1, 0, [krec('a', 13)])); // newer keyframe applies
    expect(find(r, 'a').pose.x).toBe(13);
  });

  it('carries the head-frame flag and fires onUpdate on change only', () => {
    const r = new PresenceRoster();
    let updates = 0;
    r.onUpdate = () => updates++;
    r.apply(10n, kf(1, 0, [krec('hud', 1, true)]));
    expect(find(r, 'hud').headFrame).toBe(true);
    expect(updates).toBe(1);
    r.apply(9n, delta(1, [drec(0, 5)])); // older: no change, no update
    expect(updates).toBe(1);
  });
});
