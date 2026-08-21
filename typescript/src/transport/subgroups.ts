/**
 * Subgroup streams (draft-16 §9): the stream-per-subgroup data path the
 * LASA state plane rides — live and catch-up lanes arrive as incoming
 * unidirectional streams, and the client's own state ops leave on one
 * it opens. NEW code beside the copied codec (not a divergence): the
 * old SDK never consumed subgroup streams.
 *
 * Stream layout: [type varint][track alias][group id][subgroup id?]
 * [priority u8?] then objects: [object id delta][extension headers
 * length (+bytes)][payload length][payload | status varint when empty].
 * Object ids are delta-encoded: first delta = id, then id − prev − 1.
 */

import { decodeVarint, encodeVarint } from './moq-transport.js';

// Stream type family 0x10–0x1D (+0x20 = default-priority variants).
const SUBGROUP_TYPE_LOW = 0x10;
const SUBGROUP_TYPE_HIGH = 0x1d;
const DEFAULT_PRIORITY_BIT = 0x20;
/** The variant the panaudia fork writes: explicit SID + extensions. */
const STREAM_TYPE_SUBGROUP_SID_EXT = 0x15;

export interface SubgroupHeader {
  trackAlias: number;
  groupId: bigint;
  subgroupId: bigint;
  publisherPriority: number;
  endOfGroup: boolean;
}

export interface SubgroupObject {
  objectId: bigint;
  payload: Uint8Array;
}

function typeInfo(low: number): { explicitSid: boolean; sidIsFirstObject: boolean; ext: boolean } {
  return {
    explicitSid: low === 0x14 || low === 0x15 || low === 0x1c || low === 0x1d,
    sidIsFirstObject: low === 0x12 || low === 0x13 || low === 0x1a || low === 0x1b,
    ext: (low & 0x01) !== 0,
  };
}

/**
 * Reads one incoming subgroup stream to completion, calling onObject
 * per object in order. Resolves when the stream ends. Non-subgroup
 * stream types reject (the caller decides whether that matters).
 */
export async function readSubgroupStream(
  readable: ReadableStream<Uint8Array>,
  onHeader: (h: SubgroupHeader) => void,
  onObject: (h: SubgroupHeader, o: SubgroupObject) => void
): Promise<void> {
  const reader = readable.getReader();
  let buf = new Uint8Array(0);
  let done = false;

  const fill = async (need: number): Promise<boolean> => {
    while (buf.length < need && !done) {
      const r = await reader.read();
      if (r.done) {
        done = true;
        break;
      }
      const next = new Uint8Array(buf.length + r.value.length);
      next.set(buf, 0);
      next.set(r.value, buf.length);
      buf = next;
    }
    return buf.length >= need;
  };

  // A varint needs 1–8 bytes; peek the first to learn the width.
  const readVarint = async (): Promise<bigint | null> => {
    if (!(await fill(1))) return null;
    const width = 1 << (buf[0]! >> 6);
    if (!(await fill(width))) return null;
    const { value, bytesRead } = decodeVarint(buf, 0);
    buf = buf.subarray(bytesRead);
    return value;
  };
  const readByte = async (): Promise<number | null> => {
    if (!(await fill(1))) return null;
    const b = buf[0]!;
    buf = buf.subarray(1);
    return b;
  };
  const readBytes = async (n: number): Promise<Uint8Array | null> => {
    if (!(await fill(n))) return null;
    const out = buf.subarray(0, n);
    buf = buf.subarray(n);
    return out;
  };

  try {
    const typeV = await readVarint();
    if (typeV === null) return; // empty stream
    const type = Number(typeV);
    const low = type & ~DEFAULT_PRIORITY_BIT;
    if (low < SUBGROUP_TYPE_LOW || low > SUBGROUP_TYPE_HIGH || (low & 0x06) === 0x06) {
      throw new Error(`not a subgroup stream (type 0x${type.toString(16)})`);
    }
    const info = typeInfo(low);
    const trackAlias = Number(await readVarint());
    const groupId = (await readVarint())!;
    let subgroupId = 0n;
    if (info.explicitSid) {
      subgroupId = (await readVarint())!;
    }
    let priority = 0;
    if ((type & DEFAULT_PRIORITY_BIT) === 0) {
      priority = (await readByte())!;
    }
    const header: SubgroupHeader = {
      trackAlias,
      groupId,
      subgroupId,
      publisherPriority: priority,
      endOfGroup: low >= 0x18,
    };
    // (sidIsFirstObject: filled in from the first object below.)
    let first = true;
    let prevId = 0n;
    onHeader(header);

    for (;;) {
      const delta = await readVarint();
      if (delta === null) return; // clean end of stream
      const objectId = first ? delta : prevId + delta + 1n;
      if (first && info.sidIsFirstObject) {
        header.subgroupId = objectId;
      }
      first = false;
      prevId = objectId;

      if (info.ext) {
        const extLen = (await readVarint())!;
        if (extLen > 0n && (await readBytes(Number(extLen))) === null) {
          throw new Error('truncated extension headers');
        }
      }
      const plen = Number((await readVarint())!);
      if (plen === 0) {
        await readVarint(); // object status — uninteresting to the state plane
        onObject(header, { objectId, payload: new Uint8Array(0) });
        continue;
      }
      const payload = await readBytes(plen);
      if (payload === null) throw new Error('truncated object payload');
      // Copy: the payload aliases the rolling buffer.
      onObject(header, { objectId, payload: new Uint8Array(payload) });
    }
  } finally {
    reader.releaseLock();
  }
}

/**
 * Accepts incoming unidirectional streams and fans each subgroup stream
 * out to the handler registered for its track alias. Objects for
 * unclaimed aliases are buffered (bounded) and flushed on register —
 * the server opens lanes and writes immediately after accepting a
 * subscribe, racing our processing of SUBSCRIBE_OK (same race the
 * datagram router buffers for).
 */
const PENDING_OBJECT_CAP = 512;

export class SubgroupRouter {
  private handlers = new Map<number, (h: SubgroupHeader, o: SubgroupObject) => void>();
  private pending = new Map<number, { h: SubgroupHeader; o: SubgroupObject }[]>();
  private running = false;

  register(trackAlias: number, handler: (h: SubgroupHeader, o: SubgroupObject) => void): void {
    this.handlers.set(trackAlias, handler);
    const buffered = this.pending.get(trackAlias);
    if (buffered) {
      this.pending.delete(trackAlias);
      for (const { h, o } of buffered) handler(h, o);
    }
  }

  unregister(trackAlias: number): void {
    this.handlers.delete(trackAlias);
    this.pending.delete(trackAlias);
  }

  /**
   * Route one object: to its handler, or into the bounded pending buffer
   * until a handler registers. The accept loop feeds this directly; in
   * worker mode, main feeds it with objects forwarded from the worker's
   * accept loop (same race, same buffer).
   */
  ingest(h: SubgroupHeader, o: SubgroupObject): void {
    const handler = this.handlers.get(h.trackAlias);
    if (handler) {
      handler(h, o);
      return;
    }
    const buf = this.pending.get(h.trackAlias) ?? [];
    if (buf.length < PENDING_OBJECT_CAP) buf.push({ h, o });
    this.pending.set(h.trackAlias, buf);
  }

  /** Starts the accept loop; safe to call once per transport. */
  start(transport: WebTransport): void {
    if (this.running) return;
    this.running = true;
    void (async () => {
      const streams = transport.incomingUnidirectionalStreams.getReader();
      for (;;) {
        const r = await streams.read().catch(() => ({ done: true as const, value: undefined }));
        if (r.done) return;
        void readSubgroupStream(
          r.value as ReadableStream<Uint8Array>,
          () => {},
          (h, o) => this.ingest(h, o)
        ).catch(() => {
          // Malformed or non-subgroup stream: drop it.
        });
      }
    })();
  }
}

/**
 * An outgoing subgroup stream: the client's state-source lane. Writes
 * the fork's SID+Ext header (0x15) then delta-encoded objects.
 */
export class SubgroupWriter {
  private writer: WritableStreamDefaultWriter<Uint8Array>;
  private headerSent = false;
  private objectCount = 0n;
  private prevObjectId = 0n;

  constructor(
    writable: WritableStream<Uint8Array>,
    private readonly trackAlias: number,
    private readonly groupId: bigint = 0n,
    private readonly subgroupId: bigint = 0n,
    private readonly priority: number = 0
  ) {
    this.writer = writable.getWriter();
  }

  private concat(parts: Uint8Array[]): Uint8Array {
    let n = 0;
    for (const p of parts) n += p.length;
    const out = new Uint8Array(n);
    let o = 0;
    for (const p of parts) {
      out.set(p, o);
      o += p.length;
    }
    return out;
  }

  async writeObject(objectId: bigint, payload: Uint8Array): Promise<void> {
    const parts: Uint8Array[] = [];
    if (!this.headerSent) {
      parts.push(
        encodeVarint(STREAM_TYPE_SUBGROUP_SID_EXT),
        encodeVarint(this.trackAlias),
        encodeVarint(this.groupId),
        encodeVarint(this.subgroupId),
        new Uint8Array([this.priority])
      );
      this.headerSent = true;
    }
    const delta = this.objectCount === 0n ? objectId : objectId - this.prevObjectId - 1n;
    this.prevObjectId = objectId;
    this.objectCount++;
    parts.push(
      encodeVarint(delta),
      encodeVarint(0), // no extension headers
      encodeVarint(payload.length),
      payload
    );
    await this.writer.write(this.concat(parts));
  }

  async close(): Promise<void> {
    await this.writer.close();
  }
}
