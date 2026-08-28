/**
 * Tests for UplinkSequencer — one gapless seq per entity source track
 * and the §5.1 uplink repeat emitter. Every case round-trips the
 * stamped packet through the wire encoder/parser, so an attempt to
 * attach a record to an audio-less packet (which the wire rejects)
 * would surface as a throw, and the offset/payload identity is checked
 * on the decoded bytes, not on the object we mutated.
 */

import { describe, it, expect } from 'vitest';
import { UplinkSequencer } from '../src/lasa/uplink.js';
import { encodeMonoObjectInto, parseMonoObject, type MonoObjectPacket } from '../src/lasa/wire.js';

const audioOf = (seq: number) => Uint8Array.from([0xa0, seq & 0xff, 0xc0, (seq >> 8) & 0xff]);
const pose = { x: 1, y: 2, z: 3, yaw: 0.5, pitch: 0, roll: 0 };

function roundTrip(pkt: MonoObjectPacket): MonoObjectPacket {
  const out = new Uint8Array(256);
  const n = encodeMonoObjectInto(out, pkt);
  return parseMonoObject(out.slice(0, n));
}

describe('UplinkSequencer', () => {
  it('hands out a gapless seq from 0 and leaves packets alone at offset 0', () => {
    const s = new UplinkSequencer(0);
    const own = { offset: 1, audio: Uint8Array.from([9]) };
    for (let i = 0; i < 5; i++) {
      const pkt: MonoObjectPacket = { pose, audio: audioOf(i), redundancy: own };
      expect(s.next(pkt)).toBe(BigInt(i));
      expect(pkt.redundancy).toBe(own); // pass-through: the caller's record survives
    }
    expect(s.nextSeq).toBe(5n);
    expect(s.redundancy).toBe(0);
  });

  it('attaches a copy of the frame `offset` packets back, from the first seq whose reference exists', () => {
    const s = new UplinkSequencer(3);
    const scratch = new Uint8Array(4); // reused like a real encoder buffer
    for (let i = 0; i < 20; i++) {
      scratch.set(audioOf(i));
      const pkt: MonoObjectPacket = { pose, audio: scratch };
      const seq = s.next(pkt);
      expect(seq).toBe(BigInt(i));
      const out = roundTrip(pkt);
      if (i < 3) {
        expect(out.redundancy).toBeUndefined();
        continue;
      }
      expect(out.redundancy!.offset).toBe(3);
      expect(Array.from(out.redundancy!.audio)).toEqual(Array.from(audioOf(i - 3))); // a copy, not the reused scratch
    }
  });

  it('omits the record when the referenced packet was pose-only, never substituting another offset', () => {
    const s = new UplinkSequencer(2);
    const send = (i: number, audio: boolean) => {
      const pkt: MonoObjectPacket = audio ? { pose, audio: audioOf(i) } : { pose };
      expect(s.next(pkt)).toBe(BigInt(i));
      return roundTrip(pkt);
    };
    send(0, true);
    send(1, true);
    const poseOnly = send(2, false);
    expect(poseOnly.audio).toBeUndefined();
    expect(poseOnly.redundancy).toBeUndefined();
    expect(Array.from(send(3, true).redundancy!.audio)).toEqual(Array.from(audioOf(1)));
    expect(send(4, true).redundancy).toBeUndefined(); // references pose-only seq 2
    expect(Array.from(send(5, true).redundancy!.audio)).toEqual(Array.from(audioOf(3)));
  });

  it('replaces a caller-supplied record when an offset is in force', () => {
    const s = new UplinkSequencer(1);
    s.next({ pose, audio: audioOf(0) });
    const pkt: MonoObjectPacket = { pose, audio: audioOf(1), redundancy: { offset: 5, audio: Uint8Array.from([1]) } };
    s.next(pkt);
    const out = roundTrip(pkt);
    expect(out.redundancy!.offset).toBe(1);
    expect(Array.from(out.redundancy!.audio)).toEqual(Array.from(audioOf(0)));
  });

  it('setRedundancy lowers and raises the offset live; offsets clamp to 0–7', () => {
    const s = new UplinkSequencer(9);
    expect(s.redundancy).toBe(7);
    s.setRedundancy(-2);
    expect(s.redundancy).toBe(0);
    s.setRedundancy(2.9);
    expect(s.redundancy).toBe(2);
    for (let i = 0; i < 3; i++) s.next({ pose, audio: audioOf(i) });
    s.setRedundancy(0);
    const pkt: MonoObjectPacket = { pose, audio: audioOf(3) };
    s.next(pkt);
    expect(pkt.redundancy).toBeUndefined();
  });

  it('grows a ring slot for a payload larger than the initial size', () => {
    const s = new UplinkSequencer(1, 8);
    const big = new Uint8Array(500).fill(7);
    s.next({ pose, audio: big });
    const pkt: MonoObjectPacket = { pose, audio: audioOf(1) };
    s.next(pkt);
    expect(pkt.redundancy!.audio.length).toBe(500);
    expect(pkt.redundancy!.audio.every((b) => b === 7)).toBe(true);
  });
});
