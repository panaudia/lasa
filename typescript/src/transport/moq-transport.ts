/**
 * MOQ Transport Protocol Implementation
 *
 * Implements the IETF MOQ Transport protocol (draft-ietf-moq-transport-16)
 * for use with the Panaudia spatial audio mixer.
 *
 * Targets the wire format emitted by github.com/Eyevinn/moqtransport (our
 * server). See spatial-mixer/plan/moq-draft14/wire-delta-16.md for the
 * draft-11 -> draft-16 delta and golden/draft16-vectors.json for byte-exact
 * fixtures. Key draft-16 changes vs draft-11:
 *   - Version 0xff000010, ALPN "moqt-16" (negotiated out-of-band, not in SETUP).
 *   - KVP parameters are delta-encoded (sorted ascending; see encodeParams).
 *   - CLIENT_SETUP drops the version list and the ROLE param.
 *   - SUBSCRIBE/SUBSCRIBE_OK move priority/order/forward/filter/expires/largest
 *     into parameters.
 * Unchanged: OBJECT_DATAGRAM (audio path), ANNOUNCE/SUBSCRIBE_ANNOUNCES, the
 * AuthorizationToken value (raw JWT bytes, not the spec Token struct).
 *
 * This is a minimal implementation focused on the messages needed for
 * audio streaming and state updates.
 */

import {
  MoqMessageType,
  MoqRole,
  MoqSetupParameter,
  MoqGroupOrder,
  MoqSubscription,
  MoqAnnouncement,
} from './types.js';

// ============================================================================
// VARINT Encoding/Decoding (RFC 9000 style)
// ============================================================================

/**
 * Encode a number as a QUIC-style variable-length integer
 * Returns the encoded bytes
 */
export function encodeVarint(value: number | bigint): Uint8Array {
  const n = BigInt(value);

  if (n < 64n) {
    // 1 byte: 00xxxxxx
    return new Uint8Array([Number(n)]);
  } else if (n < 16384n) {
    // 2 bytes: 01xxxxxx xxxxxxxx
    return new Uint8Array([Number((n >> 8n) | 0x40n), Number(n & 0xffn)]);
  } else if (n < 1073741824n) {
    // 4 bytes: 10xxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
    return new Uint8Array([
      Number((n >> 24n) | 0x80n),
      Number((n >> 16n) & 0xffn),
      Number((n >> 8n) & 0xffn),
      Number(n & 0xffn),
    ]);
  } else {
    // 8 bytes: 11xxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
    return new Uint8Array([
      Number((n >> 56n) | 0xc0n),
      Number((n >> 48n) & 0xffn),
      Number((n >> 40n) & 0xffn),
      Number((n >> 32n) & 0xffn),
      Number((n >> 24n) & 0xffn),
      Number((n >> 16n) & 0xffn),
      Number((n >> 8n) & 0xffn),
      Number(n & 0xffn),
    ]);
  }
}

/**
 * Decode a QUIC-style variable-length integer from a byte array
 * Returns the decoded value and the number of bytes consumed
 */
export function decodeVarint(data: Uint8Array, offset: number = 0): { value: bigint; bytesRead: number } {
  if (offset >= data.length) {
    throw new Error('Not enough data to decode varint');
  }

  const firstByte = data[offset]!;
  const prefix = firstByte >> 6;

  switch (prefix) {
    case 0: {
      // 1 byte
      return { value: BigInt(firstByte), bytesRead: 1 };
    }
    case 1: {
      // 2 bytes
      if (offset + 2 > data.length) {
        throw new Error('Not enough data for 2-byte varint');
      }
      const value = BigInt((firstByte & 0x3f) << 8) | BigInt(data[offset + 1]!);
      return { value, bytesRead: 2 };
    }
    case 2: {
      // 4 bytes
      if (offset + 4 > data.length) {
        throw new Error('Not enough data for 4-byte varint');
      }
      const value =
        (BigInt(firstByte & 0x3f) << 24n) |
        (BigInt(data[offset + 1]!) << 16n) |
        (BigInt(data[offset + 2]!) << 8n) |
        BigInt(data[offset + 3]!);
      return { value, bytesRead: 4 };
    }
    case 3: {
      // 8 bytes
      if (offset + 8 > data.length) {
        throw new Error('Not enough data for 8-byte varint');
      }
      const value =
        (BigInt(firstByte & 0x3f) << 56n) |
        (BigInt(data[offset + 1]!) << 48n) |
        (BigInt(data[offset + 2]!) << 40n) |
        (BigInt(data[offset + 3]!) << 32n) |
        (BigInt(data[offset + 4]!) << 24n) |
        (BigInt(data[offset + 5]!) << 16n) |
        (BigInt(data[offset + 6]!) << 8n) |
        BigInt(data[offset + 7]!);
      return { value, bytesRead: 8 };
    }
    default:
      throw new Error('Invalid varint prefix');
  }
}

// ============================================================================
// String/Bytes Encoding
// ============================================================================

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

/**
 * Encode a string with length prefix (varint length + UTF-8 bytes)
 */
export function encodeString(str: string): Uint8Array {
  const bytes = textEncoder.encode(str);
  const lengthBytes = encodeVarint(bytes.length);
  const result = new Uint8Array(lengthBytes.length + bytes.length);
  result.set(lengthBytes, 0);
  result.set(bytes, lengthBytes.length);
  return result;
}

/**
 * Decode a length-prefixed string
 */
export function decodeString(data: Uint8Array, offset: number = 0): { value: string; bytesRead: number } {
  const { value: length, bytesRead: lengthBytes } = decodeVarint(data, offset);
  const stringLength = Number(length);
  const stringStart = offset + lengthBytes;
  const stringEnd = stringStart + stringLength;

  if (stringEnd > data.length) {
    throw new Error('Not enough data for string');
  }

  const value = textDecoder.decode(data.subarray(stringStart, stringEnd));
  return { value, bytesRead: lengthBytes + stringLength };
}

/**
 * Encode bytes with length prefix (varint length + raw bytes)
 */
export function encodeBytes(bytes: Uint8Array): Uint8Array {
  const lengthBytes = encodeVarint(bytes.length);
  const result = new Uint8Array(lengthBytes.length + bytes.length);
  result.set(lengthBytes, 0);
  result.set(bytes, lengthBytes.length);
  return result;
}

/**
 * Decode length-prefixed bytes
 */
export function decodeBytes(data: Uint8Array, offset: number = 0): { value: Uint8Array; bytesRead: number } {
  const { value: length, bytesRead: lengthBytes } = decodeVarint(data, offset);
  const bytesLength = Number(length);
  const bytesStart = offset + lengthBytes;
  const bytesEnd = bytesStart + bytesLength;

  if (bytesEnd > data.length) {
    throw new Error('Not enough data for bytes');
  }

  const value = data.subarray(bytesStart, bytesEnd);
  return { value, bytesRead: lengthBytes + bytesLength };
}

// ============================================================================
// Message Building Helpers
// ============================================================================

/**
 * Buffer builder for constructing MOQ messages
 */
export class MessageBuilder {
  private chunks: Uint8Array[] = [];
  private totalLength = 0;

  /**
   * Append a varint to the message
   */
  writeVarint(value: number | bigint): this {
    const bytes = encodeVarint(value);
    this.chunks.push(bytes);
    this.totalLength += bytes.length;
    return this;
  }

  /**
   * Append a length-prefixed string to the message
   */
  writeString(str: string): this {
    const bytes = encodeString(str);
    this.chunks.push(bytes);
    this.totalLength += bytes.length;
    return this;
  }

  /**
   * Append length-prefixed bytes to the message
   */
  writeBytes(data: Uint8Array): this {
    const bytes = encodeBytes(data);
    this.chunks.push(bytes);
    this.totalLength += bytes.length;
    return this;
  }

  /**
   * Append raw bytes (no length prefix) to the message
   */
  writeRaw(data: Uint8Array): this {
    this.chunks.push(data);
    this.totalLength += data.length;
    return this;
  }

  /**
   * Build the final message
   */
  build(): Uint8Array {
    const result = new Uint8Array(this.totalLength);
    let offset = 0;
    for (const chunk of this.chunks) {
      result.set(chunk, offset);
      offset += chunk.length;
    }
    return result;
  }
}

// ============================================================================
// MOQ Message Encoding
// ============================================================================

/**
 * Wrap a message with length framing as expected by moqtransport library.
 * Format: [Message Type varint] [Length: 2 bytes big-endian] [Content]
 */
export function wrapWithLengthFrame(messageType: number, content: Uint8Array): Uint8Array {
  const typeBytes = encodeVarint(messageType);
  const length = content.length;

  // 2-byte big-endian length
  const lengthBytes = new Uint8Array(2);
  lengthBytes[0] = (length >> 8) & 0xff;
  lengthBytes[1] = length & 0xff;

  // Combine: type + length + content
  const result = new Uint8Array(typeBytes.length + 2 + content.length);
  result.set(typeBytes, 0);
  result.set(lengthBytes, typeBytes.length);
  result.set(content, typeBytes.length + 2);

  return result;
}

// ============================================================================
// Key-Value-Pair parameters (draft-16 delta encoding)
// ============================================================================

/**
 * A MOQ key-value parameter. By convention (RFC/draft-16):
 * - even `type` -> value is a varint (`bigint`)
 * - odd  `type` -> value is a length-prefixed byte blob (`Uint8Array`)
 */
export interface Kvp {
  type: number;
  value: bigint | Uint8Array;
}

/**
 * Append a parameter list using draft-16 delta encoding:
 *   [count][ (deltaType, value) ... ]
 * where deltaType = type - previousType (previousType starts at 0), the list is
 * sorted ascending by type first, and the value is a bare varint for even types
 * or length-prefixed bytes for odd types. Matches Eyevinn's KVPList.AppendNumVersioned.
 */
export function encodeParams(builder: MessageBuilder, params: Kvp[]): void {
  const sorted = [...params].sort((a, b) => a.type - b.type);
  builder.writeVarint(sorted.length);
  let prev = 0;
  for (const p of sorted) {
    builder.writeVarint(p.type - prev);
    prev = p.type;
    if (p.type % 2 === 1) {
      const bytes = p.value as Uint8Array;
      builder.writeVarint(bytes.length);
      builder.writeRaw(bytes);
    } else {
      builder.writeVarint(p.value as bigint);
    }
  }
}

/**
 * Decode a delta-encoded parameter list into a Map keyed by absolute type.
 * Even types map to a `bigint` (the varint value); odd types map to a
 * `Uint8Array` (the raw blob). Mirrors Eyevinn's KVPList.ParseNumVersioned.
 */
export function decodeParams(
  data: Uint8Array,
  offset: number = 0
): { params: Map<number, bigint | Uint8Array>; bytesRead: number } {
  let pos = offset;
  const { value: count, bytesRead: countBytes } = decodeVarint(data, pos);
  pos += countBytes;

  const params = new Map<number, bigint | Uint8Array>();
  let prev = 0;
  for (let i = 0; i < Number(count); i++) {
    const { value: delta, bytesRead: deltaBytes } = decodeVarint(data, pos);
    pos += deltaBytes;
    const type = prev + Number(delta);
    prev = type;

    if (type % 2 === 1) {
      const { value: blob, bytesRead: blobBytes } = decodeBytes(data, pos);
      pos += blobBytes;
      params.set(type, blob);
    } else {
      const { value: v, bytesRead: vBytes } = decodeVarint(data, pos);
      pos += vBytes;
      params.set(type, v);
    }
  }
  return { params, bytesRead: pos - offset };
}

/**
 * Build CLIENT_SETUP message (draft-16).
 *
 * draft-16 differences from draft-11:
 * - NO supported-versions list (version is negotiated out-of-band via ALPN).
 * - NO ROLE parameter (removed from the spec).
 * - Setup parameters are delta-encoded (see encodeParams).
 *
 * `supportedVersions` and `role` are accepted for signature compatibility but
 * are not written to the wire in draft-16.
 */
export function buildClientSetup(
  _supportedVersions: number[],
  _role: MoqRole,
  path?: string,
  maxSubscribeId?: number,
  extraParams?: Kvp[]
): Uint8Array {
  const contentBuilder = new MessageBuilder();

  // Setup parameters (delta-encoded). PATH is QUIC-only (odd 0x01 -> bytes);
  // MAX_REQUEST_ID is even 0x02 -> varint. Note: this client runs over
  // WebTransport, so `path` is normally undefined.
  // extraParams carries application setup parameters — LASA's 0x1A5A1
  // Connection Config (lasa-core.md §4.1); encodeParams sorts, so large
  // keys land in the right delta position. (Divergence 1, PROVENANCE.md.)
  const params: Kvp[] = [];
  if (path !== undefined) {
    params.push({ type: MoqSetupParameter.PATH, value: textEncoder.encode(path) });
  }
  if (maxSubscribeId !== undefined) {
    params.push({ type: MoqSetupParameter.MAX_SUBSCRIBE_ID, value: BigInt(maxSubscribeId) });
  }
  if (extraParams) {
    params.push(...extraParams);
  }
  encodeParams(contentBuilder, params);

  return wrapWithLengthFrame(MoqMessageType.CLIENT_SETUP, contentBuilder.build());
}

// draft-16 SUBSCRIBE/SUBSCRIBE_OK parameter keys (fields moved out of the body).
const SUB_PARAM_FORWARD = 0x10;       // even -> varint (bool)
const SUB_PARAM_PRIORITY = 0x20;      // even -> varint
const SUB_PARAM_FILTER = 0x21;        // odd  -> bytes [filterType][start?][endGroup?]
const SUB_PARAM_GROUP_ORDER = 0x22;   // even -> varint
const SUB_OK_PARAM_EXPIRES = 0x08;    // even -> varint (ms)
const SUB_OK_PARAM_LARGEST = 0x09;    // odd  -> bytes (Location: group, object)
const PARAM_AUTHORIZATION = 0x03;     // odd  -> bytes (raw JWT)
const PARAM_RESUME_HLC = 0xff01;      // odd  -> bytes (8-byte BE uint64), Panaudia custom

/**
 * Build SUBSCRIBE message (draft-16).
 *
 * Body: RequestID, TrackNamespace (tuple), TrackName (len-prefixed), then a
 * delta-encoded parameter list. SubscriberPriority/GroupOrder/Forward and the
 * Subscription Filter are now PARAMETERS (not inline fields). The custom
 * AuthorizationToken (0x03, raw JWT) and ResumeHLC (0xFF01) ride in the same
 * sorted, delta-encoded list.
 *
 * TrackAlias is NOT in SUBSCRIBE; the publisher assigns it in SUBSCRIBE_OK.
 */
export function buildSubscribe(subscription: MoqSubscription): Uint8Array {
  const contentBuilder = new MessageBuilder();

  // Request ID (called subscribeId in our interface)
  contentBuilder.writeVarint(subscription.subscribeId);

  // Namespace (tuple of strings)
  contentBuilder.writeVarint(subscription.namespace.length);
  for (const part of subscription.namespace) {
    contentBuilder.writeString(part);
  }

  // Track name (varint-prefixed bytes)
  contentBuilder.writeString(subscription.trackName);

  // Parameters. Defaults match Eyevinn's own SUBSCRIBE defaults so a subscriber
  // actually receives objects: priority 128, group order ascending, forward 1.
  const params: Kvp[] = [];
  params.push({ type: SUB_PARAM_PRIORITY, value: BigInt(subscription.subscriberPriority ?? 128) });
  params.push({ type: SUB_PARAM_GROUP_ORDER, value: BigInt(subscription.groupOrder ?? MoqGroupOrder.ASCENDING) });
  params.push({ type: SUB_PARAM_FORWARD, value: BigInt(subscription.forward ?? 1) });

  // Subscription Filter (odd 0x21 -> bytes): [filterType][start if absolute][endGroup if range].
  // LATEST_GROUP / LATEST_OBJECT carry no location.
  const filterBuilder = new MessageBuilder();
  filterBuilder.writeVarint(subscription.filterType);
  params.push({ type: SUB_PARAM_FILTER, value: filterBuilder.build() });

  if (subscription.authorization) {
    // Raw JWT bytes (Eyevinn does NOT wrap in the spec Token struct).
    params.push({ type: PARAM_AUTHORIZATION, value: textEncoder.encode(subscription.authorization) });
  }

  if (subscription.resumeOpId !== undefined && subscription.resumeOpId > 0n) {
    // ResumeHLC: 8 bytes big-endian uint64.
    const opIdBuf = new Uint8Array(8);
    new DataView(opIdBuf.buffer).setBigUint64(0, subscription.resumeOpId, false);
    params.push({ type: PARAM_RESUME_HLC, value: opIdBuf });
  }

  // Application subscribe parameters — LASA's 0x1A5A3 state subscribe
  // message rides here (divergence 3, PROVENANCE.md).
  if (subscription.extraParams) {
    params.push(...subscription.extraParams);
  }

  encodeParams(contentBuilder, params);

  return wrapWithLengthFrame(MoqMessageType.SUBSCRIBE, contentBuilder.build());
}

/**
 * Build ANNOUNCE message
 * Format: [RequestID varint][Namespace Tuple][Parameters count + list]
 */
export function buildAnnounce(announcement: MoqAnnouncement): Uint8Array {
  const contentBuilder = new MessageBuilder();

  // Request ID (required by moqtransport v0.5.0)
  contentBuilder.writeVarint(announcement.requestId);

  // Namespace (tuple of strings)
  contentBuilder.writeVarint(announcement.namespace.length);
  for (const part of announcement.namespace) {
    contentBuilder.writeString(part);
  }

  // Parameters (delta-encoded). Our client sends none today, so this is an
  // empty list (count 0); the map values are byte blobs (odd keys, e.g. auth).
  const params: Kvp[] = [];
  if (announcement.parameters) {
    for (const [key, value] of announcement.parameters) {
      params.push({ type: key, value });
    }
  }
  encodeParams(contentBuilder, params);

  return wrapWithLengthFrame(MoqMessageType.ANNOUNCE, contentBuilder.build());
}

/**
 * Build UNSUBSCRIBE message
 */
export function buildUnsubscribe(subscribeId: number): Uint8Array {
  const contentBuilder = new MessageBuilder();
  contentBuilder.writeVarint(subscribeId);
  return wrapWithLengthFrame(MoqMessageType.UNSUBSCRIBE, contentBuilder.build());
}

/**
 * Build UNANNOUNCE message
 */
export function buildUnannounce(namespace: string[]): Uint8Array {
  const contentBuilder = new MessageBuilder();

  // Namespace tuple
  contentBuilder.writeVarint(namespace.length);
  for (const part of namespace) {
    contentBuilder.writeString(part);
  }

  return wrapWithLengthFrame(MoqMessageType.UNANNOUNCE, contentBuilder.build());
}

/**
 * Build OBJECT_DATAGRAM message (for sending audio/state data)
 */
export function buildObjectDatagram(
  trackAlias: number,
  groupId: bigint,
  objectId: bigint,
  publisherPriority: number,
  payload: Uint8Array
): Uint8Array {
  const builder = new MessageBuilder();

  // Object datagram type: 0x00 = datagram without extension headers
  // (0x01 = with extensions, 0x02 = status, 0x03 = status with extensions)
  builder.writeVarint(0x00);

  // Track alias
  builder.writeVarint(trackAlias);

  // Group ID
  builder.writeVarint(groupId);

  // Object ID
  builder.writeVarint(objectId);

  // Publisher priority (single byte, NOT varint)
  builder.writeRaw(new Uint8Array([publisherPriority & 0xff]));

  // Payload (rest of datagram, no length prefix)
  builder.writeRaw(payload);

  return builder.build();
}

// ============================================================================
// Zero-allocation framing (audio send hot path — worker-capture-plan.md P1)
// ============================================================================
//
// `buildObjectDatagram` above allocates ~6 Uint8Arrays per call (one per varint,
// one for the priority byte, the builder's chunk array, and the final buffer). On
// the mic send path that runs per Opus frame (~200/s). These helpers instead write
// into a caller-owned, reused scratch buffer at a running offset — no allocation —
// so the worker can frame every frame into one preallocated buffer (design §6). They
// are byte-for-byte identical to `encodeVarint` / `buildObjectDatagram`; the
// allocating versions stay for the low-rate publishers.

/**
 * Number of bytes the QUIC varint encoding of `value` occupies (1, 2, 4, or 8) —
 * matching the thresholds in {@link encodeVarint}. Used to size scratch buffers.
 */
export function varintSize(value: number | bigint): number {
  if (typeof value === 'number') {
    if (value < 0x40) return 1;
    if (value < 0x4000) return 2;
    if (value < 0x40000000) return 4;
    return 8;
  }
  if (value < 0x40n) return 1;
  if (value < 0x4000n) return 2;
  if (value < 0x40000000n) return 4;
  return 8;
}

/**
 * Write the QUIC varint encoding of `value` into `buf` starting at `offset`,
 * returning the offset just past the last byte written. No allocation. Produces
 * bytes identical to {@link encodeVarint}.
 *
 * A `number` ≤ the 4-byte range (< 2^30) takes a BigInt-free fast path (the common
 * case: type, trackAlias, and realistic group/object IDs); larger values and the
 * 8-byte case use BigInt arithmetic, exactly as `encodeVarint`.
 */
export function writeVarintInto(buf: Uint8Array, offset: number, value: number | bigint): number {
  if (typeof value === 'number') {
    if (!Number.isInteger(value) || value < 0) {
      throw new RangeError(`writeVarintInto: value must be a non-negative integer, got ${value}`);
    }
    if (value < 0x40) {
      buf[offset] = value;
      return offset + 1;
    }
    if (value < 0x4000) {
      buf[offset] = (value >> 8) | 0x40;
      buf[offset + 1] = value & 0xff;
      return offset + 2;
    }
    if (value < 0x40000000) {
      buf[offset] = (value >>> 24) | 0x80;
      buf[offset + 1] = (value >>> 16) & 0xff;
      buf[offset + 2] = (value >>> 8) & 0xff;
      buf[offset + 3] = value & 0xff;
      return offset + 4;
    }
    // >= 2^30 — fall through to the BigInt 8-byte path.
    value = BigInt(value);
  }

  const n = value;
  if (n < 0n) {
    throw new RangeError(`writeVarintInto: value must be non-negative, got ${n}`);
  }
  if (n < 0x40n) {
    buf[offset] = Number(n);
    return offset + 1;
  }
  if (n < 0x4000n) {
    buf[offset] = Number((n >> 8n) | 0x40n);
    buf[offset + 1] = Number(n & 0xffn);
    return offset + 2;
  }
  if (n < 0x40000000n) {
    buf[offset] = Number((n >> 24n) | 0x80n);
    buf[offset + 1] = Number((n >> 16n) & 0xffn);
    buf[offset + 2] = Number((n >> 8n) & 0xffn);
    buf[offset + 3] = Number(n & 0xffn);
    return offset + 4;
  }
  buf[offset] = Number((n >> 56n) | 0xc0n);
  buf[offset + 1] = Number((n >> 48n) & 0xffn);
  buf[offset + 2] = Number((n >> 40n) & 0xffn);
  buf[offset + 3] = Number((n >> 32n) & 0xffn);
  buf[offset + 4] = Number((n >> 24n) & 0xffn);
  buf[offset + 5] = Number((n >> 16n) & 0xffn);
  buf[offset + 6] = Number((n >> 8n) & 0xffn);
  buf[offset + 7] = Number(n & 0xffn);
  return offset + 8;
}

/**
 * Upper bound on the encoded size of an OBJECT_DATAGRAM with a payload of at most
 * `maxPayload` bytes: type(1) + trackAlias(≤8) + groupId(≤8) + objectId(≤8) +
 * priority(1) + payload. Size one reused scratch buffer with this (design §6).
 */
export function maxObjectDatagramSize(maxPayload: number): number {
  return 1 + 8 + 8 + 8 + 1 + maxPayload;
}

/**
 * Write a full OBJECT_DATAGRAM into `buf` (caller-owned, reused) and return its
 * total length. Allocation-free counterpart of {@link buildObjectDatagram} —
 * byte-for-byte identical output. `buf` must be at least
 * {@link maxObjectDatagramSize}(payload.length); send `buf.subarray(0, len)`.
 */
export function encodeObjectDatagramInto(
  buf: Uint8Array,
  trackAlias: number,
  groupId: bigint,
  objectId: bigint,
  publisherPriority: number,
  payload: Uint8Array
): number {
  let pos = 0;
  pos = writeVarintInto(buf, pos, 0x00); // datagram type: no extension headers
  pos = writeVarintInto(buf, pos, trackAlias);
  pos = writeVarintInto(buf, pos, groupId);
  pos = writeVarintInto(buf, pos, objectId);
  buf[pos++] = publisherPriority & 0xff; // single byte, NOT varint
  buf.set(payload, pos);
  return pos + payload.length;
}

// ============================================================================
// MOQ Message Parsing
// ============================================================================

/**
 * Parse the message type from the beginning of a message
 */
export function parseMessageType(data: Uint8Array): { type: MoqMessageType; bytesRead: number } {
  const { value, bytesRead } = decodeVarint(data, 0);
  return { type: Number(value) as MoqMessageType, bytesRead };
}

/**
 * Parsed SERVER_SETUP message
 */
export interface ParsedServerSetup {
  selectedVersion: number;
  parameters: Map<number, Uint8Array>;
}

/**
 * Parse SERVER_SETUP message (draft-16).
 *
 * draft-16 SERVER_SETUP has NO selected-version field (the version is fixed by
 * the ALPN negotiated out-of-band); the body is just a delta-encoded parameter
 * list. We report `selectedVersion` as the version we speak for logging.
 */
export function parseServerSetup(data: Uint8Array, offset: number = 0): ParsedServerSetup {
  const { params } = decodeParams(data, offset);

  // Normalize to the legacy Map<number, Uint8Array> shape callers expect:
  // even (varint) values are re-encoded to their varint bytes.
  const parameters = new Map<number, Uint8Array>();
  for (const [type, value] of params) {
    parameters.set(type, value instanceof Uint8Array ? value : encodeVarint(value));
  }

  return {
    selectedVersion: MOQ_TRANSPORT_VERSION,
    parameters,
  };
}

/**
 * Parsed SUBSCRIBE_OK message
 */
export interface ParsedSubscribeOk {
  subscribeId: number;
  trackAlias: number;
  expires: bigint;
  groupOrder: number;
  contentExists: boolean;
  largestGroupId?: bigint;
  largestObjectId?: bigint;
}

/**
 * Parse SUBSCRIBE_OK message (draft-16).
 *
 * Body: RequestID, TrackAlias, then a delta-encoded parameter list. Expires
 * (0x08), GroupOrder (0x22) and Largest Object (0x09 -> ContentExists) are now
 * PARAMETERS, not inline fields. A trailing Track Extensions block may follow
 * but Eyevinn emits none today, so we ignore anything after the params.
 */
export function parseSubscribeOk(data: Uint8Array, offset: number = 0): ParsedSubscribeOk {
  let pos = offset;

  // Request ID
  const { value: subscribeId, bytesRead: subIdBytes } = decodeVarint(data, pos);
  pos += subIdBytes;

  // Track Alias (assigned by publisher, used in datagrams)
  const { value: trackAlias, bytesRead: aliasBytes } = decodeVarint(data, pos);
  pos += aliasBytes;

  // Parameters
  const { params } = decodeParams(data, pos);

  const result: ParsedSubscribeOk = {
    subscribeId: Number(subscribeId),
    trackAlias: Number(trackAlias),
    expires: 0n,
    groupOrder: 0,
    contentExists: false,
  };

  const expires = params.get(SUB_OK_PARAM_EXPIRES);
  if (typeof expires === 'bigint') result.expires = expires;

  const groupOrder = params.get(SUB_PARAM_GROUP_ORDER);
  if (typeof groupOrder === 'bigint') result.groupOrder = Number(groupOrder);

  const largest = params.get(SUB_OK_PARAM_LARGEST);
  if (largest instanceof Uint8Array) {
    result.contentExists = true;
    const g = decodeVarint(largest, 0);
    const o = decodeVarint(largest, g.bytesRead);
    result.largestGroupId = g.value;
    result.largestObjectId = o.value;
  }

  return result;
}

/**
 * Parsed SUBSCRIBE_ERROR message
 */
export interface ParsedSubscribeError {
  subscribeId: number;
  errorCode: number;
  reasonPhrase: string;
  trackAlias: number;
}

/**
 * Parse SUBSCRIBE_ERROR message (draft-16).
 *
 * Body: RequestID, ErrorCode, RetryInterval (new in draft-16), ReasonPhrase.
 * There is NO trailing TrackAlias (that was draft-11); it is reported as 0.
 */
export function parseSubscribeError(data: Uint8Array, offset: number = 0): ParsedSubscribeError {
  let pos = offset;

  // Request ID
  const { value: subscribeId, bytesRead: subIdBytes } = decodeVarint(data, pos);
  pos += subIdBytes;

  // Error code
  const { value: errorCode, bytesRead: errorCodeBytes } = decodeVarint(data, pos);
  pos += errorCodeBytes;

  // Retry interval (draft-16+): minimum ms before retry (0 = don't retry)
  const { bytesRead: retryBytes } = decodeVarint(data, pos);
  pos += retryBytes;

  // Reason phrase
  const { value: reasonPhrase } = decodeString(data, pos);

  return {
    subscribeId: Number(subscribeId),
    errorCode: Number(errorCode),
    reasonPhrase,
    trackAlias: 0,
  };
}

/**
 * Parsed ANNOUNCE_OK message
 * Note: moqtransport v0.5.0 returns just RequestID, not namespace
 */
export interface ParsedAnnounceOk {
  requestId: number;
}

/**
 * Parse ANNOUNCE_OK message
 */
export function parseAnnounceOk(data: Uint8Array, offset: number = 0): ParsedAnnounceOk {
  // moqtransport v0.5.0: ANNOUNCE_OK contains just RequestID
  const { value: requestId } = decodeVarint(data, offset);

  return { requestId: Number(requestId) };
}

/**
 * Parsed ANNOUNCE_ERROR message
 */
export interface ParsedAnnounceError {
  namespace: string[];
  errorCode: number;
  reasonPhrase: string;
}

/**
 * Parse ANNOUNCE_ERROR message
 */
export function parseAnnounceError(data: Uint8Array, offset: number = 0): ParsedAnnounceError {
  let pos = offset;

  // Namespace tuple length
  const { value: nsLength, bytesRead: nsLengthBytes } = decodeVarint(data, pos);
  pos += nsLengthBytes;

  // Namespace parts
  const namespace: string[] = [];
  for (let i = 0; i < Number(nsLength); i++) {
    const { value: part, bytesRead: partBytes } = decodeString(data, pos);
    pos += partBytes;
    namespace.push(part);
  }

  // Error code
  const { value: errorCode, bytesRead: errorCodeBytes } = decodeVarint(data, pos);
  pos += errorCodeBytes;

  // Reason phrase
  const { value: reasonPhrase, bytesRead: reasonBytes } = decodeString(data, pos);
  pos += reasonBytes;

  return {
    namespace,
    errorCode: Number(errorCode),
    reasonPhrase,
  };
}

/**
 * Parsed OBJECT_DATAGRAM
 */
export interface ParsedObjectDatagram {
  trackAlias: number;
  groupId: bigint;
  objectId: bigint;
  publisherPriority: number;
  payload: Uint8Array;
}

/**
 * Parse OBJECT_DATAGRAM message
 * Wire format: [Type varint][TrackAlias varint][GroupID varint][ObjectID varint][Priority 1 byte][Payload...]
 */
export function parseObjectDatagram(data: Uint8Array, offset: number = 0): ParsedObjectDatagram {
  let pos = offset;

  // Type (0x00=datagram, 0x01=datagram+extensions, 0x02=status, 0x03=status+extensions)
  const { value: _type, bytesRead: typeBytes } = decodeVarint(data, pos);
  pos += typeBytes;

  // Track alias
  const { value: trackAlias, bytesRead: aliasBytes } = decodeVarint(data, pos);
  pos += aliasBytes;

  // Group ID
  const { value: groupId, bytesRead: groupIdBytes } = decodeVarint(data, pos);
  pos += groupIdBytes;

  // Object ID
  const { value: objectId, bytesRead: objectIdBytes } = decodeVarint(data, pos);
  pos += objectIdBytes;

  // Publisher priority (single byte, NOT varint)
  if (pos >= data.length) {
    throw new Error('Not enough data for publisher priority');
  }
  const publisherPriority = data[pos]!;
  pos += 1;

  // Payload (rest of message)
  const payload = data.subarray(pos);

  return {
    trackAlias: Number(trackAlias),
    groupId,
    objectId,
    publisherPriority,
    payload,
  };
}

// ============================================================================
// MOQ Protocol Version
// ============================================================================

/**
 * MOQ Transport version we support (draft-ietf-moq-transport-16).
 * Not sent in-band (draft-16 negotiates via the "moqt-16" ALPN); kept for
 * logging and to report a selected version from SERVER_SETUP.
 */
export const MOQ_TRANSPORT_VERSION = 0xff000000 + 0x10; // 0xff000010
