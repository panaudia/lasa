/**
 * The client half of the LASA state machinery (lasa-core.md §6.5–§6.6):
 * the local store, the catch-up window with the reference
 * apply-live-with-transient-tombstones strategy, and the in-memory
 * cursor honouring invariant F7 (no cursor until the subscription's
 * first catch-up frontier is applied).
 *
 * The cursor lives with the store, in memory, so a re-subscription
 * within the store's lifetime — StateSync's own retries, or a new
 * LasaClient handed the same Store — resumes with a delta. Nothing is
 * persisted across page loads: a fresh page presents no cursor and
 * takes a snapshot (§6.6 permits exactly this — a client that does not
 * persist its store MUST NOT persist a cursor). Cross-load persistence
 * is implemented in the Go reference (`clientstore.Persistence`) and
 * deliberately not offered here yet (decision 2026-08-25: the
 * atomicity contract is a footgun and the saving is one small
 * snapshot; reopen when a real app's snapshot is big enough to notice).
 *
 * Port of Go `lasa/state/clientstore` minus persistence — behavioural
 * changes belong there first.
 */

import { MalformedError } from './wire.js';
import {
  canonicalPrefixes,
  setHash,
  type Cursor,
  type StateMessage,
  type SetOp,
  type ClearOp,
} from './statewire.js';

/** One stored key: the latest value and its seq. */
export interface KV {
  value: Uint8Array;
  seq: bigint;
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

export interface StoreOptions {
  /** Clock override for the catch-up deadline (ms since epoch). */
  clock?: () => number;
}

/** A client state store for one subscription's scope. */
export class Store {
  private entries = new Map<string, KV>();
  private cursor: Cursor | null = null;
  // eligible: the first catch-up frontier of the current subscription
  // instance has been applied (invariant F7); until then no cursor may
  // be advanced.
  private eligible = false;

  // Catch-up window state (transient-tombstone strategy).
  private inWindow = false;
  private windowKind: 'delta' | 'snapshot' = 'delta';
  private windowStart = 0;
  private transient: Map<string, bigint> | null = null; // key → live clear seq
  private snapshotIn: Set<string> | null = null; // keys delivered by the current snapshot
  private liveMaxSeq = 0n;
  private epoch: Uint8Array | null = null;

  private constructor(
    private readonly prefixes: string[],
    private readonly prefixSetHash: Uint8Array,
    private readonly clock: () => number
  ) {}

  /**
   * Creates an empty store for one prefix set. The set is
   * canonicalized. Async: the set-hash needs WebCrypto. A store belongs
   * to one space — its cursor is meaningless anywhere else.
   */
  static async create(prefixes: string[], opts: StoreOptions = {}): Promise<Store> {
    const canon = canonicalPrefixes(prefixes);
    return new Store(canon, await setHash(canon), opts.clock ?? (() => Date.now()));
  }

  /**
   * The subscribe inputs for a (re)connection: the prefix set plus the
   * cursor earned so far, if any.
   */
  subscribeParams(): { prefixes: string[]; cursor?: Cursor } {
    return { prefixes: this.prefixes, ...(this.cursor ? { cursor: this.cursor } : {}) };
  }

  /** The latest value for `key`, or undefined. */
  get(key: string): Uint8Array | undefined {
    return this.entries.get(key)?.value;
  }

  /** Number of keys held. */
  get size(): number {
    return this.entries.size;
  }

  /** Iterates every live key. */
  *[Symbol.iterator](): IterableIterator<[string, KV]> {
    yield* this.entries;
  }

  /**
   * Applies one live-lane message. Live ops apply immediately under
   * per-key dedup; during the catch-up window every live clear —
   * including for keys not held — is recorded as a transient tombstone
   * so a late catch-up chunk cannot resurrect it (lasa-core.md §6.5).
   */
  async applyLive(m: StateMessage): Promise<void> {
    switch (m.kind) {
      case 'set':
        this.liveSet(m);
        return;
      case 'clear':
        this.liveClear(m);
        return;
      case 'group':
        for (const op of m.ops) await this.applyLive(op);
        return;
      case 'frontier':
        this.setEpoch(m.epoch);
        if (m.seq > this.liveMaxSeq) this.liveMaxSeq = m.seq;
        this.advanceCursor();
        return;
      default:
        throw new MalformedError(`unexpected live message ${(m as StateMessage).kind}`);
    }
  }

  private liveSet(v: SetOp): void {
    const kv = this.entries.get(v.key);
    if (kv && v.seq <= kv.seq) return;
    if (this.inWindow && (this.transient!.get(v.key) ?? -1n) >= v.seq) return;
    this.entries.set(v.key, { value: v.value, seq: v.seq });
  }

  private liveClear(v: ClearOp): void {
    const kv = this.entries.get(v.key);
    if (kv && v.seq <= kv.seq) return;
    this.entries.delete(v.key);
    if (this.inWindow) this.transient!.set(v.key, v.seq);
  }

  /**
   * Applies one catch-up-lane message: Begin opens the window, entries
   * stream lazily, and the closing Frontier finalizes — running the
   * snapshot deletion rule, dropping the transient records, and earning
   * the cursor (invariant F7).
   */
  async applyCatchUp(m: StateMessage): Promise<void> {
    switch (m.kind) {
      case 'begin':
        this.inWindow = true;
        this.windowKind = m.begin;
        this.windowStart = this.clock();
        this.transient = new Map();
        this.snapshotIn = new Set();
        return;
      case 'set': {
        if (!this.inWindow) throw new MalformedError('catch-up entry before begin');
        if (this.windowKind === 'snapshot') this.snapshotIn!.add(m.key);
        // Resurrection guard: cleared live at or after this seq.
        if ((this.transient!.get(m.key) ?? -1n) >= m.seq) return;
        const kv = this.entries.get(m.key);
        if (kv && m.seq <= kv.seq) return;
        this.entries.set(m.key, { value: m.value, seq: m.seq });
        return;
      }
      case 'clear': {
        if (!this.inWindow) throw new MalformedError('catch-up entry before begin');
        const kv = this.entries.get(m.key);
        if (kv && m.seq > kv.seq) this.entries.delete(m.key);
        return;
      }
      case 'frontier': {
        if (!this.inWindow) throw new MalformedError('catch-up frontier before begin');
        this.setEpoch(m.epoch);
        if (this.windowKind === 'snapshot') {
          // Rule 3, snapshot only: delete every in-scope local key
          // absent from the snapshot whose seq ≤ the cut H.
          for (const [k, kv] of this.entries) {
            if (!this.snapshotIn!.has(k) && kv.seq <= m.seq) this.entries.delete(k);
          }
        }
        this.inWindow = false;
        this.transient = null;
        this.snapshotIn = null;
        this.eligible = true;
        if (m.seq > this.liveMaxSeq) this.liveMaxSeq = m.seq;
        this.advanceCursor();
        return;
      }
      default:
        throw new MalformedError(`unexpected catch-up message ${(m as StateMessage).kind}`);
    }
  }

  private setEpoch(e: Uint8Array): void {
    if (!this.epoch || !bytesEqual(this.epoch, e)) {
      // New epoch: any previously earned cursor is for a dead seq space.
      if (this.epoch && this.cursor && !bytesEqual(this.cursor.epoch, e)) {
        this.cursor = null;
      }
      this.epoch = e.slice();
    }
  }

  // Moves the cursor to the max seq seen across both paths — only once
  // eligible (invariant F7), and only within the current epoch.
  private advanceCursor(): void {
    if (!this.eligible || !this.epoch) return;
    if (!this.cursor || !bytesEqual(this.cursor.epoch, this.epoch) || this.liveMaxSeq > this.cursor.seq) {
      this.cursor = { epoch: this.epoch.slice(), seq: this.liveMaxSeq, setHash: this.prefixSetHash };
    }
  }

  /**
   * Informs the store its subscription instance ended (too-far-behind,
   * displacement, transport close). The catch-up window state —
   * including the transient records, which belong to the dead instance
   * — is discarded; eligibility resets so the next instance must earn
   * its own frontier before any cursor advances again.
   */
  terminated(): void {
    this.inWindow = false;
    this.transient = null;
    this.snapshotIn = null;
    this.eligible = false;
    this.liveMaxSeq = 0n;
  }

  /**
   * Whether a catch-up window has been open longer than deadlineMs —
   * the cue to unsubscribe and re-subscribe fresh rather than hold a
   * transient-record window forever.
   */
  catchUpStalled(deadlineMs: number): boolean {
    return this.inWindow && this.clock() - this.windowStart > deadlineMs;
  }
}
