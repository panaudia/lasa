/**
 * uplink.ts — the per-entity source-track sequencer.
 *
 * Two things every packet an entity sends must agree on, so they live
 * in one object per entity, in the worker, and every send path goes
 * through it:
 *
 * 1. ONE gapless seq counter (lasa-core.md §5.1: the datagram group id
 *    increments by exactly one per packet, pose-only packets included).
 *    Capture-encoder audio and main-thread pose-only publishes on the
 *    same entity used to keep separate counters; the server saw the
 *    second counter's packets as late replays of the first's and
 *    dropped them.
 *
 * 2. The §5.1 uplink repeat emitter at the entity's declared offset
 *    (§4.2): with offset n, every audio-bearing packet also carries a
 *    full copy of the audio n packets earlier, so a burst of up to n
 *    lost packets is repaired exactly at the server's ingest. Policy is
 *    always-on at the offset — the sink writer's mirror image; the
 *    declaration is both the request and the bound.
 *
 * Unlike a sink track, an uplink track carries pose-only packets, which
 * advance seq without audio. The referenced packet MUST be
 * audio-bearing, so the ring remembers which seqs carried audio and
 * omits the record when seq − offset was pose-only — never substituting
 * a different offset, since the offset is what locates the copy.
 */

import type { MonoObjectPacket, Redundancy } from './wire.js';

interface HistSlot {
  seq: bigint;
  valid: boolean;
  /** Byte length of the recorded audio; 0 = the packet at seq was pose-only. */
  len: number;
  buf: Uint8Array;
}

const HISTORY = 8; // slot (seq−7)%8 is never the slot being written at seq

function clampOffset(n: number): number {
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.min(7, Math.floor(n));
}

export class UplinkSequencer {
  private seq = 0n;
  private offset: number;
  private readonly hist: HistSlot[] = [];
  /** Reused record handed out on the packet (aliases the ring). */
  private readonly red: Redundancy;

  /**
   * @param redundancy the offset (0 = off; above 7, the wire maximum,
   *   clamps to 7).
   * @param maxPayloadBytes initial ring slot size (grown on demand).
   */
  constructor(redundancy = 0, maxPayloadBytes = 4000) {
    this.offset = clampOffset(redundancy);
    for (let i = 0; i < HISTORY; i++) {
      this.hist.push({ seq: 0n, valid: false, len: 0, buf: new Uint8Array(maxPayloadBytes) });
    }
    this.red = { offset: this.offset, audio: new Uint8Array(0) };
  }

  /** The uplink redundancy offset in force (0 = none). */
  get redundancy(): number {
    return this.offset;
  }

  /** Changes the offset. Bounding it to the declaration is the caller's job. */
  setRedundancy(offset: number): void {
    this.offset = clampOffset(offset);
  }

  /** The seq the next packet will get (diagnostics). */
  get nextSeq(): bigint {
    return this.seq;
  }

  /**
   * Stamps `pkt` with the next seq: records its audio (a copy — the
   * caller's buffer is reused) and, when the packet `offset` ago was
   * audio-bearing and this one is too, attaches a repeat of it. Any
   * `redundancy` the caller set is replaced when the offset is above
   * 0; at offset 0 the packet passes through as given. The attached
   * record aliases the ring and is valid until the next call: encode
   * before calling again. Returns the seq (the datagram group id).
   */
  next(pkt: MonoObjectPacket): bigint {
    const seq = this.seq++;
    this.attach(seq, pkt);
    return seq;
  }

  private attach(seq: bigint, pkt: MonoObjectPacket): void {
    if (this.offset === 0) return;
    delete pkt.redundancy;
    const slot = this.hist[Number(seq % BigInt(HISTORY))]!;
    slot.seq = seq;
    slot.valid = true;
    const audio = pkt.audio;
    if (!audio || audio.length === 0) {
      slot.len = 0;
      return;
    }
    if (audio.length > slot.buf.length) slot.buf = new Uint8Array(audio.length);
    slot.buf.set(audio);
    slot.len = audio.length;
    const offset = BigInt(this.offset);
    if (seq < offset) return;
    const refSeq = seq - offset;
    const ref = this.hist[Number(refSeq % BigInt(HISTORY))]!;
    if (!ref.valid || ref.seq !== refSeq || ref.len === 0) return;
    this.red.offset = this.offset;
    this.red.audio = ref.buf.subarray(0, ref.len);
    pkt.redundancy = this.red;
  }
}
