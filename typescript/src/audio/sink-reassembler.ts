/**
 * The sink-track FEC reassembler (panaudia-engine plan/sink-redundancy.md,
 * piece 2) — TS port of the Go reference (panaudia-bridge
 * bridge/reassembler.go). Sits between MoQ receive and the playout
 * jitter buffer when the entity declared redundancy > 0; the redundant
 * payload is split from the packet BEFORE WebCodecs decode (the u16
 * prefix is on the encoded packet — parseSink does the split).
 *
 * Contract (rulings 2026-08-13, recorded in the plan doc):
 *
 * - Pass-through while the stream is in order — no copies, no added
 *   latency on the happy path.
 * - On a group-id gap: hold successors; a later packet whose redundant
 *   copy reaches back into the gap repairs it, and repaired frames
 *   flush in order.
 * - Give-up is id-denominated (arrival-driven, wall-clock-free): frame
 *   g is dead once a packet with group-id ≥ g + lag + 2 has arrived
 *   while g is unresolved. At give-up the frame is CONCEALED — released
 *   with null audio, which the caller must turn into a zero-content
 *   frame at the frame's position (finding F5, jitter-v4 design §14: a
 *   skipped frame is indistinguishable from a drift-down quantum inside
 *   the buffer and would be wrongly repaid with synthetic inserts).
 * - Concealment is bounded by the buffer's sensing freeze threshold:
 *   when the currently-dead run at the head of the queue exceeds
 *   8 packets (40 ms — freezeReads), the run is SKIPPED, not concealed.
 *   A hole that size is the absence shape the buffer's freeze/re-anchor
 *   machinery already handles; concealing an outage at resume would
 *   burst seconds of zeros into the ring.
 *
 * Lockstep: tests/sink-reassembler.test.ts replays the fixtures the Go
 * reference exported (tests/fixtures/reassembler) and asserts event
 * equality. State is scoped to one (MoQ session, track) instance
 * (lasa-core.md §5.2) — create a fresh instance per subscription.
 */

/** Dead-run length (packets) above which a hole is skipped instead of
 * concealed: the jitter buffer's freezeReads threshold (40 ms) in 5 ms
 * wire frames. */
export const REASM_SKIP_THRESHOLD = 8;

/** One in-order frame released by the reassembler: an audio payload
 * (primary or repaired copy — identical content), or null audio for a
 * concealment frame given up on. */
export interface ReassemblerEvent {
  seq: bigint;
  audio: Uint8Array | null;
}

interface Held {
  audio: Uint8Array;
  repaired: boolean;
}

export class SinkReassembler {
  private readonly lag: bigint;
  private readonly skip: bigint;

  private started = false;
  private next = 0n; // next seq to release
  private high = 0n; // highest seq seen
  private readonly held = new Map<bigint, Held>();

  /** Counters, cumulative for the instance (stats surface). */
  delivered = 0; // frames released with audio (repaired included)
  repaired = 0; // released from a redundant copy
  concealed = 0; // null-audio frames released at give-up
  skipped = 0; // dead frames skipped (freeze-sized holes)
  stale = 0; // packets dropped for arriving behind the release point

  private readonly out: ReassemblerEvent[] = [];

  constructor(lag: number) {
    this.lag = BigInt(lag);
    this.skip = BigInt(REASM_SKIP_THRESHOLD);
  }

  /**
   * Accepts the packet with MoQ group-id seq, its primary audio payload,
   * and its redundant copy (redOffset 0 / redAudio null when absent).
   * Returns the frames released in order; the array is reused and valid
   * until the next push. Held payloads are copied (the caller's buffer
   * is not retained); the in-order fast path aliases the input.
   */
  push(seq: bigint, audio: Uint8Array, redOffset: number, redAudio: Uint8Array | null): ReassemblerEvent[] {
    this.out.length = 0;
    if (!this.started) {
      // The stream starts where it starts: predecessors of the first
      // packet seen are pre-session (warm start swallows them).
      this.started = true;
      this.next = seq;
      this.high = seq;
    }
    if (seq < this.next) {
      // Late arrival behind the release point — already released or
      // concealed; its redundant copy reaches even further back.
      this.stale++;
      return this.out;
    }
    if (seq > this.high) this.high = seq;

    // Fast path: the expected packet with nothing pending. Its redundant
    // copy can only reference already-released frames.
    if (seq === this.next && this.held.size === 0) {
      this.next++;
      this.delivered++;
      this.out.push({ seq, audio });
      return this.out;
    }

    // Slow path: buffer the primary, repair from the redundant copy,
    // then drain whatever became releasable.
    if (!this.held.has(seq)) {
      this.held.set(seq, { audio: audio.slice(), repaired: false });
    }
    if (redAudio !== null && redOffset > 0 && seq >= BigInt(redOffset)) {
      const r = seq - BigInt(redOffset);
      if (r >= this.next && !this.held.has(r)) {
        this.held.set(r, { audio: redAudio.slice(), repaired: true });
      }
    }
    this.drain();
    return this.out;
  }

  /**
   * Releases from the head of the queue: held frames flush in order; a
   * missing head frame past its give-up deadline is concealed, or the
   * whole currently-dead run is skipped when it exceeds the freeze
   * threshold.
   */
  private drain(): void {
    for (;;) {
      const h = this.held.get(this.next);
      if (h !== undefined) {
        this.held.delete(this.next);
        this.out.push({ seq: this.next, audio: h.audio });
        this.delivered++;
        if (h.repaired) this.repaired++;
        this.next++;
        continue;
      }
      if (this.high < this.next + this.lag + 2n) {
        return; // head frame's repeat may still arrive
      }
      // The head frame is dead. Measure the currently-dead run: the
      // consecutive missing frames whose deadlines have all passed.
      let run = 1n;
      for (;;) {
        const s = this.next + run;
        if (this.held.has(s)) break;
        if (this.high < s + this.lag + 2n) break;
        run++;
      }
      if (run > this.skip) {
        this.next += run;
        this.skipped += Number(run);
        continue;
      }
      this.out.push({ seq: this.next, audio: null });
      this.concealed++;
      this.next++;
    }
  }
}
