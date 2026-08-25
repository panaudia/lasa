/**
 * LASA state-plane wire codecs (lasa-core.md §6.5–§6.7): state
 * messages in both directions, the state subscribe parameter, cursors
 * and the set-hash. Byte-exact against the Go `lasa/wire` — golden
 * vectors in tests/vectors/.
 */

import { MalformedError } from './wire.js';

export const SUBSCRIBE_PARAM_STATE = 0x1a5a3;
export const EPOCH_SIZE = 8;
export const SET_HASH_SIZE = 32;
export const MAX_VALUE_LEN = 65536;
export const MAX_KEY_LEN = 512;

// Key grammar (lasa-core.md §2): [a-z0-9.-], 1–512 bytes. Prefixes
// share the grammar but may be empty (the subscribe-everything set,
// §6.3). ASCII-only by construction, so string length = byte length
// and code-unit comparison = bytewise comparison.
const VALID_KEY_RE = /^[a-z0-9.-]{1,512}$/;
const VALID_PREFIX_RE = /^[a-z0-9.-]{0,512}$/;

export type StateDirection = 'upstream' | 'downstream';

export interface SetOp {
  kind: 'set';
  key: string;
  value: Uint8Array;
  /** Server-assigned; meaningful downstream only. */
  seq: bigint;
}
export interface ClearOp {
  kind: 'clear';
  key: string;
  seq: bigint;
}
export interface GroupOp {
  kind: 'group';
  ops: (SetOp | ClearOp)[];
}
export interface FrontierMsg {
  kind: 'frontier';
  epoch: Uint8Array; // EPOCH_SIZE opaque bytes
  seq: bigint;
}
export interface BeginMsg {
  kind: 'begin';
  begin: 'delta' | 'snapshot';
}
export type StateMessage = SetOp | ClearOp | GroupOp | FrontierMsg | BeginMsg;

/**
 * What a client writes: the same ops without the server-assigned seq
 * (the upstream layout never encodes it). A downstream StateMessage is
 * assignable here, so decoded messages can be re-published unchanged.
 */
export type UpstreamSetOp = Omit<SetOp, 'seq'> & { seq?: bigint };
export type UpstreamClearOp = Omit<ClearOp, 'seq'> & { seq?: bigint };
export interface UpstreamGroupOp {
  kind: 'group';
  ops: (UpstreamSetOp | UpstreamClearOp)[];
}
export type UpstreamStateMessage = UpstreamSetOp | UpstreamClearOp | UpstreamGroupOp | FrontierMsg | BeginMsg;

const MSG_SET = 1;
const MSG_CLEAR = 2;
const MSG_GROUP = 3;
const MSG_FRONTIER = 4;
const MSG_BEGIN = 5;
const BEGIN_DELTA = 1;
const BEGIN_SNAPSHOT = 2;

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

// --- varints (RFC 9000, canonical) ----------------------------------------

function appendVarint(parts: number[], v: bigint | number): void {
  const n = BigInt(v);
  if (n < 64n) {
    parts.push(Number(n));
  } else if (n < 16384n) {
    parts.push(Number(n >> 8n) | 0x40, Number(n & 0xffn));
  } else if (n < 1073741824n) {
    parts.push(Number(n >> 24n) | 0x80, Number((n >> 16n) & 0xffn), Number((n >> 8n) & 0xffn), Number(n & 0xffn));
  } else {
    parts.push(Number(n >> 56n) | 0xc0);
    for (let s = 48n; s >= 0n; s -= 8n) parts.push(Number((n >> s) & 0xffn));
  }
}

function readVarint(data: Uint8Array, o: number): { value: bigint; next: number } {
  if (o >= data.length) throw new MalformedError('truncated varint');
  const width = 1 << (data[o]! >> 6);
  if (o + width > data.length) throw new MalformedError('truncated varint');
  let v = BigInt(data[o]! & 0x3f);
  for (let i = 1; i < width; i++) v = (v << 8n) | BigInt(data[o + i]!);
  return { value: v, next: o + width };
}

// --- state messages --------------------------------------------------------

export function encodeStateMessage(m: UpstreamStateMessage, dir: StateDirection): Uint8Array {
  const parts: number[] = [];
  appendStateMessage(parts, m, dir, true);
  return new Uint8Array(parts);
}

function appendSeq(parts: number[], seq: bigint | undefined, dir: StateDirection): void {
  if (dir !== 'downstream') return;
  if (seq === undefined) throw new MalformedError('seq is required downstream');
  appendVarint(parts, seq);
}

function appendStateMessage(parts: number[], m: UpstreamStateMessage, dir: StateDirection, allowGroup: boolean): void {
  switch (m.kind) {
    case 'set': {
      if (!VALID_KEY_RE.test(m.key)) throw new MalformedError('invalid key');
      if (m.value.length > MAX_VALUE_LEN) throw new MalformedError('value too long');
      parts.push(MSG_SET);
      const key = textEncoder.encode(m.key);
      appendVarint(parts, key.length);
      for (const b of key) parts.push(b);
      appendVarint(parts, m.value.length);
      for (const b of m.value) parts.push(b);
      appendSeq(parts, m.seq, dir);
      return;
    }
    case 'clear': {
      if (!VALID_KEY_RE.test(m.key)) throw new MalformedError('invalid key');
      parts.push(MSG_CLEAR);
      const key = textEncoder.encode(m.key);
      appendVarint(parts, key.length);
      for (const b of key) parts.push(b);
      appendSeq(parts, m.seq, dir);
      return;
    }
    case 'group': {
      if (!allowGroup) throw new MalformedError('groups do not nest');
      parts.push(MSG_GROUP);
      appendVarint(parts, m.ops.length);
      for (const op of m.ops) appendStateMessage(parts, op, dir, false);
      return;
    }
    case 'frontier': {
      if (dir !== 'downstream') throw new MalformedError('frontier is downstream-only');
      parts.push(MSG_FRONTIER);
      for (const b of m.epoch) parts.push(b);
      appendVarint(parts, m.seq);
      return;
    }
    case 'begin': {
      if (dir !== 'downstream') throw new MalformedError('begin is downstream-only');
      parts.push(MSG_BEGIN, m.begin === 'delta' ? BEGIN_DELTA : BEGIN_SNAPSHOT);
      return;
    }
  }
}

export function parseStateMessage(data: Uint8Array, dir: StateDirection): StateMessage {
  const { msg, next } = parseOne(data, 0, dir, true);
  if (next !== data.length) throw new MalformedError('trailing bytes after state message');
  return msg;
}

// readKey parses a length-prefixed key, holding it to the §2 grammar
// and the 512-byte cap, and erroring on truncation rather than letting
// subarray silently clamp to a short key.
function readKey(data: Uint8Array, o: number): { key: string; next: number } {
  const r = readVarint(data, o);
  const klen = Number(r.value);
  if (klen > MAX_KEY_LEN) throw new MalformedError('key too long');
  o = r.next;
  if (o + klen > data.length) throw new MalformedError('truncated key');
  const key = textDecoder.decode(data.subarray(o, o + klen));
  if (!VALID_KEY_RE.test(key)) throw new MalformedError('invalid key');
  return { key, next: o + klen };
}

function parseOne(
  data: Uint8Array,
  o: number,
  dir: StateDirection,
  allowGroup: boolean
): { msg: StateMessage; next: number } {
  if (o >= data.length) throw new MalformedError('empty state message');
  const t = data[o]!;
  o += 1;
  switch (t) {
    case MSG_SET: {
      const k = readKey(data, o);
      const key = k.key;
      o = k.next;
      let r = readVarint(data, o);
      const vlen = Number(r.value);
      o = r.next;
      if (vlen > MAX_VALUE_LEN) throw new MalformedError('value too long');
      if (o + vlen > data.length) throw new MalformedError('truncated value');
      const value = data.slice(o, o + vlen);
      o += vlen;
      let seq = 0n;
      if (dir === 'downstream') {
        r = readVarint(data, o);
        seq = r.value;
        o = r.next;
      }
      return { msg: { kind: 'set', key, value, seq }, next: o };
    }
    case MSG_CLEAR: {
      const k = readKey(data, o);
      const key = k.key;
      o = k.next;
      let seq = 0n;
      if (dir === 'downstream') {
        const r = readVarint(data, o);
        seq = r.value;
        o = r.next;
      }
      return { msg: { kind: 'clear', key, seq }, next: o };
    }
    case MSG_GROUP: {
      if (!allowGroup) throw new MalformedError('groups do not nest');
      const r = readVarint(data, o);
      o = r.next;
      const ops: (SetOp | ClearOp)[] = [];
      for (let i = 0n; i < r.value; i++) {
        const inner = parseOne(data, o, dir, false);
        if (inner.msg.kind !== 'set' && inner.msg.kind !== 'clear') {
          throw new MalformedError('groups may contain only set and clear ops');
        }
        ops.push(inner.msg);
        o = inner.next;
      }
      return { msg: { kind: 'group', ops }, next: o };
    }
    case MSG_FRONTIER: {
      if (dir !== 'downstream') throw new MalformedError('frontier is downstream-only');
      if (o + EPOCH_SIZE > data.length) throw new MalformedError('truncated epoch');
      const epoch = data.slice(o, o + EPOCH_SIZE);
      o += EPOCH_SIZE;
      const r = readVarint(data, o);
      return { msg: { kind: 'frontier', epoch, seq: r.value }, next: r.next };
    }
    case MSG_BEGIN: {
      if (dir !== 'downstream') throw new MalformedError('begin is downstream-only');
      if (o >= data.length) throw new MalformedError('truncated begin');
      const kind = data[o]!;
      if (kind !== BEGIN_DELTA && kind !== BEGIN_SNAPSHOT) throw new MalformedError('unknown begin kind');
      return { msg: { kind: 'begin', begin: kind === BEGIN_DELTA ? 'delta' : 'snapshot' }, next: o + 1 };
    }
    default:
      throw new MalformedError(`unknown state message type ${t}`);
  }
}

// --- subscribe params ------------------------------------------------------

/** A resume position in the space's state history (`lasa-core.md` §6.6), earned by a completed catch-up. */
export interface Cursor {
  epoch: Uint8Array; // EPOCH_SIZE
  seq: bigint;
  setHash: Uint8Array; // SET_HASH_SIZE
}

/**
 * Canonical prefix set: validated against the §2 grammar (empty
 * allowed), deduplicated, sorted bytewise. Validation is what makes
 * the plain sort() bytewise — a valid prefix is ASCII, where code-unit
 * order and byte order coincide; an unvalidated non-ASCII prefix would
 * sort differently here than in the Go server and yield a set-hash the
 * server rejects as non-canonical.
 */
export function canonicalPrefixes(prefixes: string[]): string[] {
  for (const p of prefixes) {
    if (!VALID_PREFIX_RE.test(p)) throw new MalformedError(`invalid prefix ${JSON.stringify(p)}`);
  }
  return [...new Set(prefixes)].sort();
}

const SUBSCRIBE_FLAG_CURSOR = 1 << 0;

export function encodeSubscribeParams(prefixes: string[], cursor?: Cursor): Uint8Array {
  const canon = canonicalPrefixes(prefixes);
  if (canon.length === 0) throw new MalformedError('empty prefix set is a protocol error');
  const parts: number[] = [];
  parts.push(cursor ? SUBSCRIBE_FLAG_CURSOR : 0);
  appendVarint(parts, canon.length);
  for (const p of canon) {
    const b = textEncoder.encode(p);
    appendVarint(parts, b.length);
    for (const x of b) parts.push(x);
  }
  if (cursor) {
    for (const b of cursor.epoch) parts.push(b);
    appendVarint(parts, cursor.seq);
    for (const b of cursor.setHash) parts.push(b);
  }
  return new Uint8Array(parts);
}

/**
 * The cursor set-hash (lasa-core.md §6.6): SHA-256 over the canonical
 * encoding [len varint][prefix bytes]… . Async (WebCrypto).
 */
export async function setHash(prefixes: string[]): Promise<Uint8Array> {
  const parts: number[] = [];
  for (const p of canonicalPrefixes(prefixes)) {
    const b = textEncoder.encode(p);
    appendVarint(parts, b.length);
    for (const x of b) parts.push(x);
  }
  const digest = await crypto.subtle.digest('SHA-256', new Uint8Array(parts));
  return new Uint8Array(digest);
}
