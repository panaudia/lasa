/**
 * datagram-router.ts — the main-thread datagram dispatch: a trackAlias→handler
 * map plus the SUBSCRIBE_OK/first-datagram pre-handler race buffer.
 *
 * Extracted verbatim from `MoqConnection` (Phase 1 of worker-transport-plan.md) so
 * the *routing* is independent of the *transport*. Today `MoqConnection` composes
 * it and feeds it from its own read loop; once the transport moves into the worker
 * (Phase 2), this same router runs on the main thread fed by datagrams the worker
 * forwards — the subscribers register on it exactly as they register on the
 * connection today, so their code is unchanged.
 *
 * The race it guards: the server writes SUBSCRIBE_OK on the bidi control stream
 * and then immediately SendDatagram's backfilled objects on the unreliable QUIC
 * datagram channel; a datagram for a freshly-assigned trackAlias can arrive before
 * `await session.subscribe()` resolves and the handler is registered. Such
 * datagrams are buffered in arrival order and drained when the handler registers.
 */

/** Handler for datagrams dispatched by track alias. */
export type DatagramHandler = (payload: Uint8Array, trackAlias: number, groupId: bigint, objectId: bigint) => void;

/** A parsed OBJECT_DATAGRAM, reduced to what routing needs (publisher priority is irrelevant here). */
export interface ParsedDatagram {
  trackAlias: number;
  payload: Uint8Array;
  groupId: bigint;
  objectId: bigint;
}

/**
 * The minimal receive surface a subscriber needs — register/unregister a handler
 * by track alias. Satisfied by both `MoqConnection` (main-thread transport) and
 * `DatagramRouter` (the main-side router fed by the worker's forwarded datagrams),
 * so subscribers work either way without knowing which.
 */
export interface DatagramReceiver {
  registerDatagramHandler(trackAlias: number, handler: DatagramHandler): void;
  unregisterDatagramHandler(trackAlias: number): void;
}

// Max bytes the pre-handler buffer holds across all unknown aliases. 1 MiB is the
// same envelope budget used elsewhere and far more than any real SUBSCRIBE_OK race
// needs (a handful of envelopes a few hundred bytes each).
const PENDING_DATAGRAM_MAX_BYTES = 1 * 1024 * 1024;

export class DatagramRouter {
  private handlers: Map<number, DatagramHandler> = new Map();

  // Pre-handler buffer, FIFO across all aliases; oldest dropped when the byte cap
  // is exceeded. Cleared on clear().
  private pending: ParsedDatagram[] = [];
  private pendingBytes = 0;

  /**
   * Register a handler for a track alias and drain any datagrams that arrived for
   * it before registration (the SUBSCRIBE_OK race), in arrival order.
   */
  register(trackAlias: number, handler: DatagramHandler): void {
    this.handlers.set(trackAlias, handler);
    if (this.pending.length > 0) this.drainForAlias(trackAlias, handler);
  }

  /** Unregister a handler and discard any still-buffered datagrams for its alias. */
  unregister(trackAlias: number): void {
    this.handlers.delete(trackAlias);
    if (this.pending.length > 0) this.discardForAlias(trackAlias);
  }

  /** Route a parsed datagram to its handler, or buffer it if none is registered yet. */
  ingest(d: ParsedDatagram): void {
    const handler = this.handlers.get(d.trackAlias);
    if (handler) {
      handler(d.payload, d.trackAlias, d.groupId, d.objectId);
    } else {
      this.bufferUnknown(d);
    }
  }

  // DatagramReceiver surface (same names as MoqConnection) so subscribers can take
  // either. These are the public aliases of register()/unregister().
  registerDatagramHandler(trackAlias: number, handler: DatagramHandler): void {
    this.register(trackAlias, handler);
  }
  unregisterDatagramHandler(trackAlias: number): void {
    this.unregister(trackAlias);
  }

  /** Number of buffered pre-handler datagrams (tests/diagnostics). */
  pendingCount(): number {
    return this.pending.length;
  }

  /** Drop all handlers + buffered datagrams (connection close). */
  clear(): void {
    this.handlers.clear();
    this.pending = [];
    this.pendingBytes = 0;
  }

  private drainForAlias(trackAlias: number, handler: DatagramHandler): void {
    const remaining: ParsedDatagram[] = [];
    let drainedBytes = 0;
    for (const d of this.pending) {
      if (d.trackAlias === trackAlias) {
        try {
          handler(d.payload, d.trackAlias, d.groupId, d.objectId);
        } catch {
          // Ignore handler errors; drain proceeds.
        }
        drainedBytes += d.payload.length;
      } else {
        remaining.push(d);
      }
    }
    this.pending = remaining;
    this.pendingBytes -= drainedBytes;
  }

  private discardForAlias(trackAlias: number): void {
    const remaining: ParsedDatagram[] = [];
    let discardedBytes = 0;
    for (const d of this.pending) {
      if (d.trackAlias === trackAlias) {
        discardedBytes += d.payload.length;
      } else {
        remaining.push(d);
      }
    }
    this.pending = remaining;
    this.pendingBytes -= discardedBytes;
  }

  private bufferUnknown(d: ParsedDatagram): void {
    this.pending.push(d);
    this.pendingBytes += d.payload.length;
    while (this.pendingBytes > PENDING_DATAGRAM_MAX_BYTES && this.pending.length > 0) {
      const dropped = this.pending.shift()!;
      this.pendingBytes -= dropped.payload.length;
    }
  }
}
