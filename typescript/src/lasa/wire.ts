/**
 * LASA audio-plane wire codecs (lasa-core.md §5.1/§5.3): the 18-byte
 * pose, mono-object uplink packets, sink packets, and presence
 * datagrams. Byte-exact against the Go implementation — verified by
 * golden vectors generated from `lasa/wire` (tests/vectors/).
 *
 * Everything here is fixed-width little-endian; QUIC varints do not
 * appear in these payloads. Encoders write into caller scratch where it
 * matters (the capture hot path); parsers return views into the input
 * where aliasing is safe for synchronous consumption.
 */

/** A packet or message that violates its format. Receivers discard it. */
export class MalformedError extends Error {}
/** Unknown flag bits: not this format — callers MUST discard silently. */
export class UnknownFlagsError extends Error {}

// The shared flags byte (§5.1).
const FLAG_POSE = 1 << 0;
const FLAG_AUDIO = 1 << 1;
const FLAG_REDUNDANCY = 1 << 2;
const OFFSET_SHIFT = 3;
const OFFSET_BITS = 0x7 << OFFSET_SHIFT;
const FLAG_TS = 1 << 6;
const FLAG_RESERVED = 1 << 7;

// ---------------------------------------------------------------------------
// Pose
// ---------------------------------------------------------------------------

/** Engineering units: metres, radians; x forward, y left, z up. */
export interface Pose {
  x: number;
  y: number;
  z: number;
  yaw: number;
  pitch: number;
  roll: number;
}

export const POSE_SIZE = 18;
export const ANGLE_QUANTUM = Math.PI / 32767;

/** Round half away from zero (Go math.Round; JS Math.round differs at −.5). */
function roundHalfAway(v: number): number {
  return v < 0 ? -Math.round(-v) : Math.round(v);
}

function quantizeAngle(a: number): number {
  let q = roundHalfAway((a * 32767) / Math.PI);
  if (q > 32767) q = 32767;
  if (q < -32767) q = -32767;
  return q;
}

/** Writes the 18-byte pose at `offset`; returns the new offset. */
export function encodePoseInto(view: DataView, offset: number, p: Pose): number {
  view.setFloat32(offset, p.x, true);
  view.setFloat32(offset + 4, p.y, true);
  view.setFloat32(offset + 8, p.z, true);
  view.setInt16(offset + 12, quantizeAngle(p.yaw), true);
  view.setInt16(offset + 14, quantizeAngle(p.pitch), true);
  view.setInt16(offset + 16, quantizeAngle(p.roll), true);
  return offset + POSE_SIZE;
}

export function parsePose(view: DataView, offset: number): Pose {
  if (view.byteLength - offset < POSE_SIZE) {
    throw new MalformedError('truncated pose');
  }
  return {
    x: view.getFloat32(offset, true),
    y: view.getFloat32(offset + 4, true),
    z: view.getFloat32(offset + 8, true),
    yaw: view.getInt16(offset + 12, true) * ANGLE_QUANTUM,
    pitch: view.getInt16(offset + 14, true) * ANGLE_QUANTUM,
    roll: view.getInt16(offset + 16, true) * ANGLE_QUANTUM,
  };
}

// ---------------------------------------------------------------------------
// Mono-object (uplink)
// ---------------------------------------------------------------------------

/** A repeat of an earlier packet's audio, for loss repair (`lasa-core.md` §5.1). */
export interface Redundancy {
  /** 1–7 packets back; the referenced packet MUST be audio-bearing. */
  offset: number;
  audio: Uint8Array;
}

/** One mono-object datagram: any of a pose, an audio frame, and a redundancy record. */
export interface MonoObjectPacket {
  pose?: Pose;
  audio?: Uint8Array;
  redundancy?: Redundancy;
}

/**
 * Encodes a mono-object packet into `out`, returning the byte length.
 * Zero-allocation: the capture hot path reuses `out` across frames.
 */
export function encodeMonoObjectInto(out: Uint8Array, p: MonoObjectPacket): number {
  let flags = 0;
  if (p.pose) flags |= FLAG_POSE;
  if (p.audio) {
    if (p.audio.length === 0) throw new MalformedError('empty audio payload');
    flags |= FLAG_AUDIO;
  }
  if (p.redundancy) {
    if (!p.audio) throw new MalformedError('redundancy requires an audio-bearing packet');
    if (p.redundancy.offset < 1 || p.redundancy.offset > 7) {
      throw new MalformedError(`redundancy offset ${p.redundancy.offset} outside 1-7`);
    }
    if (p.redundancy.audio.length === 0) throw new MalformedError('empty redundancy payload');
    flags |= FLAG_REDUNDANCY | (p.redundancy.offset << OFFSET_SHIFT);
  }
  const view = new DataView(out.buffer, out.byteOffset, out.byteLength);
  let o = 0;
  out[o++] = flags;
  if (p.pose) {
    o = encodePoseInto(view, o, p.pose);
  }
  if (p.audio) {
    if (p.redundancy) {
      view.setUint16(o, p.audio.length, true);
      o += 2;
    }
    out.set(p.audio, o);
    o += p.audio.length;
    if (p.redundancy) {
      out.set(p.redundancy.audio, o);
      o += p.redundancy.audio.length;
    }
  }
  return o;
}

export function parseMonoObject(data: Uint8Array): MonoObjectPacket {
  if (data.length === 0) throw new MalformedError('empty packet');
  const flags = data[0]!;
  if ((flags & (FLAG_TS | FLAG_RESERVED)) !== 0) throw new UnknownFlagsError('mono-object');
  if ((flags & FLAG_REDUNDANCY) === 0 && (flags & OFFSET_BITS) !== 0) {
    throw new UnknownFlagsError('mono-object offset bits');
  }
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  let o = 1;
  const p: MonoObjectPacket = {};
  if ((flags & FLAG_POSE) !== 0) {
    p.pose = parsePose(view, o);
    o += POSE_SIZE;
  }
  const hasAudio = (flags & FLAG_AUDIO) !== 0;
  if ((flags & FLAG_REDUNDANCY) !== 0 && !hasAudio) {
    throw new MalformedError('redundancy on a packet with no audio');
  }
  if (hasAudio) {
    if ((flags & FLAG_REDUNDANCY) !== 0) {
      if (data.length - o < 2) throw new MalformedError('truncated audio length');
      const alen = view.getUint16(o, true);
      o += 2;
      if (data.length - o < alen) throw new MalformedError('truncated audio');
      p.audio = data.subarray(o, o + alen);
      o += alen;
      const red = data.subarray(o);
      if (red.length === 0) throw new MalformedError('empty redundancy payload');
      p.redundancy = { offset: (flags & OFFSET_BITS) >> OFFSET_SHIFT, audio: red };
    } else {
      if (data.length === o) throw new MalformedError('empty audio payload');
      p.audio = data.subarray(o);
    }
  } else if (data.length !== o) {
    throw new MalformedError('trailing bytes on a no-audio packet');
  }
  return p;
}

// ---------------------------------------------------------------------------
// Sink (downlink: binaural / ambi2 / ambi3 — same layout)
// ---------------------------------------------------------------------------

/** One sink datagram: a rendered audio frame with optional redundancy. */
export interface SinkPacket {
  /** Unix-epoch microseconds of the frame's first sample (informational). */
  timestampMicros?: bigint;
  audio: Uint8Array;
  redundancy?: Redundancy;
}

export function parseSink(data: Uint8Array): SinkPacket {
  if (data.length === 0) throw new MalformedError('empty packet');
  const flags = data[0]!;
  if ((flags & (FLAG_POSE | FLAG_AUDIO | FLAG_RESERVED)) !== 0) throw new UnknownFlagsError('sink');
  if ((flags & FLAG_REDUNDANCY) === 0 && (flags & OFFSET_BITS) !== 0) {
    throw new UnknownFlagsError('sink offset bits');
  }
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  let o = 1;
  let ts: bigint | undefined;
  if ((flags & FLAG_TS) !== 0) {
    if (data.length - o < 8) throw new MalformedError('truncated timestamp');
    ts = view.getBigUint64(o, true);
    o += 8;
  }
  if ((flags & FLAG_REDUNDANCY) !== 0) {
    if (data.length - o < 2) throw new MalformedError('truncated audio length');
    const alen = view.getUint16(o, true);
    o += 2;
    if (data.length - o < alen) throw new MalformedError('truncated audio');
    const audio = data.subarray(o, o + alen);
    if (audio.length === 0) throw new MalformedError('sink packets always carry audio');
    o += alen;
    const red = data.subarray(o);
    if (red.length === 0) throw new MalformedError('empty redundancy payload');
    return {
      timestampMicros: ts,
      audio,
      redundancy: { offset: (flags & OFFSET_BITS) >> OFFSET_SHIFT, audio: red },
    };
  }
  const audio = data.subarray(o);
  if (audio.length === 0) throw new MalformedError('sink packets always carry audio');
  return { timestampMicros: ts, audio };
}

// ---------------------------------------------------------------------------
// Presence (lasa-core.md §7)
// ---------------------------------------------------------------------------

/** 0 = full scale, −0.5 dB per step down to 254 = −127 dBFS; 255 = silence (below −127 dBFS). */
export const LOUDNESS_SILENT = 255;
/** Converts an 8-bit presence loudness to dBFS (`-Infinity` for `LOUDNESS_SILENT`). */
export function loudnessToDBFS(l: number): number {
  return l === LOUDNESS_SILENT ? -Infinity : l * -0.5;
}

/** One entity in a presence keyframe. */
export interface KeyframeRecord {
  headFrame: boolean;
  id: string;
  pose: Pose;
  loudness: number;
}
/** A presence keyframe: the full record set from index `first`, for generation `gen`. */
export interface PresenceKeyframe {
  kind: 'keyframe';
  gen: number;
  first: number;
  records: KeyframeRecord[];
}
/** One changed entity in a presence delta, by keyframe index. */
export interface DeltaRecord {
  index: number;
  pose: Pose;
  loudness: number;
}
/** A presence delta: changed records for generation `gen`. */
export interface PresenceDelta {
  kind: 'delta';
  gen: number;
  records: DeltaRecord[];
}

const PRESENCE_FLAG_KEYFRAME = 1 << 0;
const KRECORD_FLAG_HEAD_FRAME = 1 << 0;
const textDecoder = new TextDecoder();

// Identifier grammar (lasa-core.md §2) — krecord ids are validated on
// parse, matching the Go parser's strictness: a violating id rejects
// the whole packet under the discard rule.
const VALID_IDENTIFIER_RE = /^[a-z0-9-]{1,128}$/;

export function parsePresence(data: Uint8Array): PresenceKeyframe | PresenceDelta {
  if (data.length < 2) throw new MalformedError('truncated presence packet');
  const flags = data[0]!;
  if ((flags & ~PRESENCE_FLAG_KEYFRAME) !== 0) throw new UnknownFlagsError('presence');
  const gen = data[1]!;
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);

  if ((flags & PRESENCE_FLAG_KEYFRAME) !== 0) {
    if (data.length < 4) throw new MalformedError('truncated keyframe header');
    const first = view.getUint16(2, true);
    let o = 4;
    const records: KeyframeRecord[] = [];
    while (o < data.length) {
      if (data.length - o < 2) throw new MalformedError('truncated krecord header');
      const kflags = data[o]!;
      if ((kflags & ~KRECORD_FLAG_HEAD_FRAME) !== 0) throw new UnknownFlagsError('krecord');
      const idLen = data[o + 1]!;
      o += 2;
      if (data.length - o < idLen + POSE_SIZE + 1) throw new MalformedError('truncated krecord');
      const id = textDecoder.decode(data.subarray(o, o + idLen));
      if (!VALID_IDENTIFIER_RE.test(id)) throw new MalformedError('invalid krecord id');
      o += idLen;
      const pose = parsePose(view, o);
      o += POSE_SIZE;
      const loudness = data[o]!;
      o += 1;
      records.push({ headFrame: (kflags & KRECORD_FLAG_HEAD_FRAME) !== 0, id, pose, loudness });
    }
    return { kind: 'keyframe', gen, first, records };
  }

  let o = 2;
  const records: DeltaRecord[] = [];
  while (o < data.length) {
    if (data.length - o < 2 + POSE_SIZE + 1) throw new MalformedError('truncated delta record');
    const index = view.getUint16(o, true);
    o += 2;
    const pose = parsePose(view, o);
    o += POSE_SIZE;
    const loudness = data[o]!;
    o += 1;
    records.push({ index, pose, loudness });
  }
  return { kind: 'delta', gen, records };
}
