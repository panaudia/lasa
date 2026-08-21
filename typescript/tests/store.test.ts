/**
 * Store rule tests: ports of the Go clientstore suite (store_test.go)
 * — the resurrection guard, the snapshot deletion rule, invariant F7,
 * termination, stall detection, persistence, and epoch change.
 */
import { describe, expect, it } from 'vitest';
import { MemPersistence, Store, type Persistence } from '../src/lasa/store';
import type { StateMessage } from '../src/lasa/statewire';

const epochA = new Uint8Array([1, 1, 1, 1, 1, 1, 1, 1]);
const epochB = new Uint8Array([2, 2, 2, 2, 2, 2, 2, 2]);

const set = (key: string, value: string, seq: bigint): StateMessage => ({
  kind: 'set',
  key,
  value: new TextEncoder().encode(value),
  seq,
});
const clear = (key: string, seq: bigint): StateMessage => ({ kind: 'clear', key, seq });
const begin = (kind: 'delta' | 'snapshot'): StateMessage => ({ kind: 'begin', begin: kind });
const frontier = (epoch: Uint8Array, seq: bigint): StateMessage => ({ kind: 'frontier', epoch, seq });

function text(v: Uint8Array | undefined): string | undefined {
  return v ? new TextDecoder().decode(v) : undefined;
}

interface Clock {
  now: () => number;
  advance: (ms: number) => void;
}
function testClock(): Clock {
  let t = 1_800_000_000_000;
  return { now: () => t, advance: (ms) => (t += ms) };
}

async function newStore(p: Persistence | null = null, clock?: Clock): Promise<Store> {
  return Store.create('space1', [''], p, clock ? { clock: clock.now } : {});
}

// The resurrection trap (lasa-core.md §6.5 note): a live clear during
// the window, then a late catch-up chunk carrying the same key at an
// older seq. The transient record must block it — including for keys
// the client never held.
it('resurrection guard', async () => {
  const s = await newStore();
  await s.applyCatchUp(begin('delta'));

  // Live: key created at 980 elsewhere, cleared at 1005 — never held.
  await s.applyLive(clear('app.k', 1005n));
  // Late catch-up chunk carries the stale set at 980.
  await s.applyCatchUp(set('app.k', 'stale', 980n));
  expect(s.get('app.k')).toBeUndefined();
  // A genuinely newer live set still wins.
  await s.applyLive(set('app.k', 'new', 1010n));
  expect(text(s.get('app.k'))).toBe('new');
  await s.applyCatchUp(frontier(epochA, 1100n));
});

// Snapshot rule 3: on completion, delete every in-scope local key
// absent from the snapshot whose seq ≤ the cut H; keys set live during
// the window (seq > H) survive.
it('snapshot deletion rule', async () => {
  const p = new MemPersistence();
  // Stale local state from a previous epoch/instance, via persistence.
  await p.save('space1', {
    entries: new Map([
      ['old.a', { value: new TextEncoder().encode('x'), seq: 5n }],
      ['old.b', { value: new TextEncoder().encode('y'), seq: 8n }],
    ]),
    cursor: null,
  });
  const s = await newStore(p);

  await s.applyCatchUp(begin('snapshot'));
  await s.applyCatchUp(set('old.a', 'fresh', 20n));
  // Live op lands mid-window for a key the snapshot does not carry.
  await s.applyLive(set('live.c', 'live', 40n));
  await s.applyCatchUp(frontier(epochA, 30n));

  expect(s.get('old.b')).toBeUndefined();
  expect(text(s.get('old.a'))).toBe('fresh');
  expect(s.get('live.c')).toBeDefined();
});

// Invariant F7: live frontiers arriving before the catch-up frontier
// must not earn a cursor; crashing in the window loses nothing because
// nothing was persisted.
it('no cursor before the first catch-up frontier', async () => {
  const p = new MemPersistence();
  const s = await newStore(p);
  await s.applyCatchUp(begin('snapshot'));
  await s.applyLive(frontier(epochA, 500n)); // live frontier first

  expect(s.subscribeParams().cursor).toBeUndefined();
  expect(await p.load('space1')).toBeNull();

  await s.applyCatchUp(frontier(epochA, 300n));
  const cur = s.subscribeParams().cursor;
  expect(cur?.seq).toBe(500n); // max across both paths
  const snap = await p.load('space1');
  expect(snap?.cursor).toBeTruthy();
});

// Subscription termination mid-window: transient records belong to the
// dead instance and are discarded; eligibility resets.
it('terminated discards the window', async () => {
  const s = await newStore();
  await s.applyCatchUp(begin('delta'));
  await s.applyLive(clear('app.k', 100n));
  s.terminated();

  // New instance: fresh window; the old transient must not block a
  // legitimate snapshot entry.
  await s.applyCatchUp(begin('snapshot'));
  await s.applyCatchUp(set('app.k', 'v', 90n));
  expect(text(s.get('app.k'))).toBe('v');
});

it('catch-up stall detection', async () => {
  const clk = testClock();
  const s = await newStore(null, clk);
  expect(s.catchUpStalled(1000)).toBe(false);
  await s.applyCatchUp(begin('delta'));
  clk.advance(2000);
  expect(s.catchUpStalled(1000)).toBe(true);
});

// Persistence round trip, including the set-hash guard: a cursor earned
// under a different prefix set must not be presented.
it('persistence round trip with the set-hash guard', async () => {
  const p = new MemPersistence();
  const s = await newStore(p);
  await s.applyCatchUp(begin('snapshot'));
  await s.applyCatchUp(set('app.k', 'v', 7n));
  await s.applyCatchUp(frontier(epochA, 10n));

  const s2 = await newStore(p);
  expect(text(s2.get('app.k'))).toBe('v');
  const c = s2.subscribeParams().cursor;
  expect(c?.seq).toBe(10n);
  expect(Array.from(c!.epoch)).toEqual(Array.from(epochA));

  // Different prefix set: the persisted cursor is not presentable.
  const s3 = await Store.create('space1', ['app.'], p);
  expect(s3.subscribeParams().cursor).toBeUndefined();
});

// An epoch change invalidates the cursor; the new epoch's frontier
// earns a fresh one.
it('epoch change invalidates the cursor', async () => {
  const s = await newStore();
  await s.applyCatchUp(begin('snapshot'));
  await s.applyCatchUp(frontier(epochA, 10n));
  expect(s.subscribeParams().cursor).toBeTruthy();

  s.terminated(); // reconnect
  await s.applyCatchUp(begin('snapshot'));
  await s.applyCatchUp(frontier(epochB, 3n));
  const c = s.subscribeParams().cursor;
  expect(Array.from(c!.epoch)).toEqual(Array.from(epochB));
  expect(c!.seq).toBe(3n);
});

describe('catch-up ordering errors', () => {
  it('rejects entries and frontiers before begin', async () => {
    const s = await newStore();
    await expect(s.applyCatchUp(set('k', 'v', 1n))).rejects.toThrow(/before begin/);
    await expect(s.applyCatchUp(frontier(epochA, 1n))).rejects.toThrow(/before begin/);
  });
});
