/**
 * Tests for CaptureEncoder — the worker-side per-entity send pipeline,
 * adapted from the old SDK's test with LASA framing assertions: each
 * sent datagram is an OBJECT_DATAGRAM whose groupId is the gapless
 * packet sequence and whose payload is a MonoObjectPacket carrying the
 * pose that was in the shared PoseCell AT FRAMING TIME (the freshness
 * rule) plus the Opus bytes. A fake FrameEncoder mimics Opus's
 * accumulate-to-frameDuration behaviour.
 */

import { describe, it, expect } from 'vitest';
import { CaptureRing } from '../src/audio/capture-ring.js';
import { CaptureEncoder, type EncodedChunkLike, type FrameEncoder } from '../src/audio/capture-encoder.js';
import { PoseCell, allocatePoseCell, type PoseCellViews } from '../src/audio/pose-cell.js';
import { parseObjectDatagram } from '../src/transport/moq-transport.js';
import { parseMonoObject, type Pose } from '../src/lasa/wire.js';

const SR = 48000;
const FRAME = 240; // 5 ms @ 48 k — Opus packet boundary the fake emits on

/** Non-shared stand-in for the SAB pose cell (Atomics work on plain buffers). */
function plainPoseCell(): PoseCellViews {
  const buf = new ArrayBuffer(8 + 6 * 8);
  return { seq: new Int32Array(buf, 0, 1), values: new Float64Array(buf, 8, 6) };
}

/**
 * Fake Opus encoder: buffers input frames and emits one packet every
 * `FRAME` frames, like WebCodecs AudioEncoder packetizing to its
 * frameDuration.
 */
function makeFakeEncoder(payloadLen = 40) {
  return (onChunk: (c: EncodedChunkLike) => void): FrameEncoder => {
    let buffered = 0;
    let emittedFrames = 0;
    const emit = () => {
      const timestamp = Math.round((emittedFrames / SR) * 1e6);
      onChunk({
        byteLength: payloadLen,
        timestamp,
        copyTo(dest: Uint8Array) {
          for (let i = 0; i < payloadLen; i++) dest[i] = (i + emittedFrames) & 0xff;
        },
      });
      emittedFrames += FRAME;
    };
    return {
      encode(_samples, frames) {
        buffered += frames;
        while (buffered >= FRAME) {
          emit();
          buffered -= FRAME;
        }
      },
      async flush() {
        /* drop sub-frame remainder, like a real flushless stop */
      },
      close() {},
    };
  };
}

function harness(nc: number, capacityFrames: number, trackAlias: number, payloadLen = 40) {
  const sharedStorage = new Float32Array(capacityFrames * nc);
  const sharedWritePos = new BigInt64Array(1);
  const sharedReadPos = new BigInt64Array(1);
  const producer = new CaptureRing({ numChannels: nc, capacityFrames, sharedStorage, sharedWritePos, sharedReadPos });
  const consumer = new CaptureRing({ numChannels: nc, capacityFrames, sharedStorage, sharedWritePos, sharedReadPos });
  const views = plainPoseCell();
  const poseWriter = new PoseCell(views);
  const sent: Uint8Array[] = [];
  const enc = new CaptureEncoder({
    ring: consumer,
    trackAlias,
    sampleRate: SR,
    numChannels: nc,
    poseCell: new PoseCell(views),
    publisherPriority: 7,
    makeEncoder: makeFakeEncoder(payloadLen),
    send: (b) => sent.push(b.slice()), // copy: b is a reused pool buffer view
  });
  return { producer, enc, sent, poseWriter };
}

/** A planar quantum of `nFrames` (values are irrelevant to framing). */
function quantum(nFrames: number, nc: number): Float32Array[] {
  const planar: Float32Array[] = [];
  for (let ch = 0; ch < nc; ch++) planar.push(new Float32Array(nFrames));
  return planar;
}

describe('CaptureEncoder (LASA framing)', () => {
  it('frames one drained Opus packet as a mono-object datagram', () => {
    const { producer, enc, sent, poseWriter } = harness(1, 1024, 3);
    poseWriter.write({ x: 1.5, y: -2.25, z: 0.125, yaw: 1, pitch: -0.5, roll: 0 });
    producer.write(quantum(FRAME, 1));
    expect(enc.pump()).toBe(FRAME);
    expect(sent.length).toBe(1);

    const d = parseObjectDatagram(sent[0]!);
    expect(d.trackAlias).toBe(3);
    expect(d.groupId).toBe(0n); // gapless seq, not timestamp-ms
    expect(d.objectId).toBe(0n);
    expect(d.publisherPriority).toBe(7);

    const pkt = parseMonoObject(d.payload);
    expect(pkt.audio!.length).toBe(40);
    expect(pkt.pose!.x).toBeCloseTo(1.5, 3);
    expect(pkt.pose!.y).toBeCloseTo(-2.25, 3);
    expect(pkt.pose!.yaw).toBeCloseTo(1, 3);
  });

  it('groupId is the gapless packet sequence across frames', () => {
    const { producer, enc, sent } = harness(2, 2048, 1);
    producer.write(quantum(FRAME * 3, 2));
    enc.pump();
    expect(sent.length).toBe(3);

    const parsed = sent.map((b) => parseObjectDatagram(b));
    expect(parsed.map((p) => p.groupId)).toEqual([0n, 1n, 2n]); // §5.2 gapless
    expect(parsed.every((p) => p.objectId === 0n)).toBe(true);
    expect(parsed.every((p) => p.trackAlias === 1)).toBe(true);
  });

  it('stamps the pose read at FRAMING time — a fresher pose than the audio', () => {
    const { producer, enc, sent, poseWriter } = harness(1, 4096, 2);
    // Audio for packets 0..2 enters the ring while pose = A...
    poseWriter.write({ x: 0, y: 0, z: 0, yaw: 0.1, pitch: 0, roll: 0 });
    producer.write(quantum(FRAME * 2, 1));
    enc.pump(); // packets 0,1 framed under pose A
    // ...then the pose moves before the next packet's framing.
    poseWriter.write({ x: 0, y: 0, z: 0, yaw: 0.9, pitch: 0, roll: 0 });
    producer.write(quantum(FRAME, 1));
    enc.pump(); // packet 2 framed under pose B

    const yaws = sent.map((b) => parseMonoObject(parseObjectDatagram(b).payload).pose!.yaw);
    expect(yaws[0]).toBeCloseTo(0.1, 3);
    expect(yaws[1]).toBeCloseTo(0.1, 3);
    expect(yaws[2]).toBeCloseTo(0.9, 3); // freshest-wins
  });

  it('handles sub-frame quanta — output only once a frame has accumulated', () => {
    const { producer, enc, sent } = harness(1, 1024, 5);
    producer.write(quantum(128, 1));
    expect(enc.pump()).toBe(128);
    expect(sent.length).toBe(0);
    producer.write(quantum(128, 1));
    expect(enc.pump()).toBe(128);
    expect(sent.length).toBe(1);
    expect(parseObjectDatagram(sent[0]!).groupId).toBe(0n);
  });

  it('pump on an empty ring sends nothing', () => {
    const { enc, sent } = harness(1, 256, 0);
    expect(enc.pump()).toBe(0);
    expect(sent.length).toBe(0);
  });

  it('drains several accumulated quanta in one pump', () => {
    const { producer, enc, sent } = harness(1, 2048, 9);
    producer.write(quantum(FRAME, 1));
    producer.write(quantum(FRAME, 1));
    producer.write(quantum(FRAME, 1));
    expect(enc.pump()).toBe(FRAME * 3);
    expect(sent.length).toBe(3);
    expect(enc.encodedBatches).toBe(1);
  });

  it('frames distinct contents correctly across more packets than one pool buffer', () => {
    const { producer, enc, sent } = harness(1, 4096, 2);
    producer.write(quantum(FRAME * 10, 1));
    enc.pump();
    expect(sent.length).toBe(10);
    const seqs = sent.map((b) => parseObjectDatagram(b).groupId);
    expect(seqs).toEqual([0n, 1n, 2n, 3n, 4n, 5n, 6n, 7n, 8n, 9n]);
  });

  it('stop() resolves and closes the encoder', async () => {
    const { enc } = harness(1, 256, 0);
    await expect(enc.stop()).resolves.toBeUndefined();
  });
});

describe('PoseCell', () => {
  it('round-trips a pose and tolerates repeated writes', () => {
    const views = plainPoseCell();
    const writer = new PoseCell(views);
    const reader = new PoseCell(views);
    const out: Pose = { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 };
    reader.read(out);
    expect(out).toEqual({ x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 }); // home pose
    for (let i = 1; i <= 5; i++) {
      writer.write({ x: i, y: -i, z: i / 2, yaw: i / 10, pitch: 0, roll: -i });
      reader.read(out);
      expect(out.x).toBe(i);
      expect(out.roll).toBe(-i);
    }
  });

  it('a stalled mid-write leaves the previous snapshot instead of spinning', () => {
    // The writer is the main thread; if it is preempted between the two
    // seqlock increments the counter stays odd indefinitely. read() must
    // give up after bounded retries and keep the caller's last snapshot
    // (2026-08-06 worker-stall incident).
    const views = plainPoseCell();
    const writer = new PoseCell(views);
    const reader = new PoseCell(views);
    const out: Pose = { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 };
    writer.write({ x: 1, y: 2, z: 3, yaw: 0.4, pitch: 0.5, roll: 0.6 });
    reader.read(out);
    expect(out.x).toBe(1);
    // Simulate a writer preempted mid-write: odd counter, torn values.
    Atomics.add(views.seq, 0, 1);
    views.values[0] = 99;
    reader.read(out); // must return promptly...
    expect(out).toEqual({ x: 1, y: 2, z: 3, yaw: 0.4, pitch: 0.5, roll: 0.6 }); // ...keeping the old snapshot
    // Writer completes; the next read picks up the new pose.
    views.values[1] = 98;
    views.values[2] = 97;
    views.values[3] = 0;
    views.values[4] = 0;
    views.values[5] = 0;
    Atomics.add(views.seq, 0, 1);
    reader.read(out);
    expect(out.x).toBe(99);
    expect(out.y).toBe(98);
  });

  it('allocatePoseCell views are SAB-backed and correctly shaped', () => {
    const views = allocatePoseCell();
    expect(views.seq.length).toBe(1);
    expect(views.values.length).toBe(6);
    expect(views.seq.buffer).toBe(views.values.buffer);
    expect(views.seq.buffer.constructor.name).toBe('SharedArrayBuffer');
  });
});
