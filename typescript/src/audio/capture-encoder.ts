/**
 * capture-encoder.ts — the worker-side per-entity SEND pipeline.
 * Descended from the old SDK's capture-encoder.ts (worker-capture-design
 * §6) with LASA framing: each Opus packet is framed as a MonoObjectPacket
 * (POSE + audio) and sent as an OBJECT_DATAGRAM whose groupId is the
 * gapless sequence number (lasa-core §5.2) — not the old
 * timestamp-ms/objectId scheme.
 *
 * Pose freshness (Paul's head-rotation-latency rule): the pose is read
 * from the shared {@link PoseCell} in `handleChunk` — datagram-framing
 * time, the last possible moment before send — so every packet carries
 * the newest completed write, ~one encoder frame FRESHER than the audio
 * it accompanies.
 *
 * The WebCodecs globals are INJECTED via `makeEncoder` so the whole
 * drain → encode → frame → send path is unit-testable off-thread with a
 * fake encoder and a fake send sink. Zero per-frame allocation: drained
 * PCM, Opus bytes, mono-object payload, and framed datagrams all live
 * in reused scratch (datagrams cycle a small pool because the
 * transport's write may drain asynchronously).
 */

import type { CaptureRing } from './capture-ring.js';
import { encodeObjectDatagramInto, maxObjectDatagramSize } from '../transport/moq-transport.js';
import { encodeMonoObjectInto, type Pose } from '../lasa/wire.js';
import { PoseCell } from './pose-cell.js';

/** Minimal view of an encoded Opus packet (WebCodecs `EncodedAudioChunk` duck type). */
export interface EncodedChunkLike {
  readonly byteLength: number;
  /** Presentation timestamp in microseconds (propagated from the input AudioData). */
  readonly timestamp: number;
  copyTo(dest: Uint8Array): void;
}

/**
 * Frame-oriented encoder seam. The worker implements this over a
 * WebCodecs `AudioEncoder`; tests implement a fake that mimics Opus's
 * accumulate-to-frameDuration behaviour.
 */
export interface FrameEncoder {
  /** Encode `frames` interleaved frames from `samples` (length `frames*nc`) at `timestampUs`. */
  encode(samples: Float32Array, frames: number, timestampUs: number): void;
  flush(): Promise<void>;
  close(): void;
}

export interface CaptureEncoderConfig {
  ring: CaptureRing;
  /** The entity's source-track alias (the server's subscribe-back). */
  trackAlias: number;
  sampleRate: number;
  numChannels: number;
  /** The entity's shared pose cell (read at framing time). */
  poseCell: PoseCell;
  /** Publisher priority byte (0 = highest). Default 0. */
  publisherPriority?: number;
  /** Upper bound on an encoded Opus packet (scratch sizing). Default 4000. */
  maxPayloadBytes?: number;
  /** Round-robin datagram buffer count. Default 8. */
  datagramPoolSize?: number;
  /** Construct the frame encoder, wiring its output to `onChunk`. */
  makeEncoder: (onChunk: (chunk: EncodedChunkLike) => void) => FrameEncoder;
  /** Send a framed datagram (the worker writes it to the transport). */
  send: (bytes: Uint8Array) => void;
}

/**
 * Drains a CaptureRing and publishes Opus frames as LASA mono-object
 * datagrams. Call {@link pump} on each wake; {@link stop} flushes +
 * closes on teardown.
 */
export class CaptureEncoder {
  private readonly ring: CaptureRing;
  private readonly trackAlias: number;
  private readonly sampleRate: number;
  private readonly nc: number;
  private readonly priority: number;
  private readonly poseCell: PoseCell;
  private readonly encoder: FrameEncoder;
  private readonly send: (bytes: Uint8Array) => void;

  // Reused scratch — the zero-alloc hot path.
  private readonly pcmScratch: Float32Array; // drained interleaved PCM
  private readonly bytesScratch: Uint8Array; // Opus bytes (encoder output copyTo)
  private readonly payloadScratch: Uint8Array; // framed MonoObjectPacket
  private readonly dgPool: Uint8Array[]; // framed OBJECT_DATAGRAMs, round-robin
  private dgPoolIdx: number;
  private readonly pose: Pose = { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 };

  // Sequencing: input timestamp is a running sample count; the datagram
  // groupId is the gapless packet sequence (§5.2), objectId always 0.
  private samplesSent = 0;
  private seq = 0n;

  // Observability.
  encodedBatches = 0;
  sentDatagrams = 0;
  droppedOversize = 0;

  constructor(cfg: CaptureEncoderConfig) {
    this.ring = cfg.ring;
    this.trackAlias = cfg.trackAlias;
    this.sampleRate = cfg.sampleRate;
    this.nc = cfg.numChannels;
    this.priority = cfg.publisherPriority ?? 0;
    this.poseCell = cfg.poseCell;
    this.send = cfg.send;

    const maxPayload = cfg.maxPayloadBytes ?? 4000;
    this.pcmScratch = new Float32Array(cfg.ring.capacity * this.nc); // holds a full drain
    this.bytesScratch = new Uint8Array(maxPayload);
    this.payloadScratch = new Uint8Array(maxPayload + 64); // + pose/flags/lengths headroom
    const poolSize = cfg.datagramPoolSize ?? 8;
    this.dgPool = [];
    for (let i = 0; i < poolSize; i++) {
      this.dgPool.push(new Uint8Array(maxObjectDatagramSize(this.payloadScratch.length)));
    }
    this.dgPoolIdx = 0;

    this.encoder = cfg.makeEncoder((chunk) => this.handleChunk(chunk));
  }

  /**
   * Drain all PCM currently in the ring and feed it to the encoder as
   * one batch. Returns the interleaved sample count encoded (0 if the
   * ring was empty). Opus does the packetization internally.
   */
  pump(): number {
    const n = this.ring.drain(this.pcmScratch);
    if (n <= 0) {
      return 0;
    }
    const frames = n / this.nc;
    // Running-sample-count timestamp (µs), monotonic and contiguous across pumps.
    const timestampUs = Math.round((this.samplesSent / this.sampleRate) * 1e6);
    this.encoder.encode(this.pcmScratch.subarray(0, n), frames, timestampUs);
    this.samplesSent += frames;
    this.encodedBatches++;
    return n;
  }

  /**
   * Encoder output: stamp the FRESHEST pose, frame the mono-object
   * packet, frame the datagram, send. No alloc.
   */
  private handleChunk(chunk: EncodedChunkLike): void {
    const size = chunk.byteLength;
    if (size === 0) {
      return; // never publish an audio-less packet from the encoder path
    }
    if (size > this.bytesScratch.length) {
      this.droppedOversize++; // pathological packet larger than scratch
      return;
    }
    chunk.copyTo(this.bytesScratch);
    // Pose read at framing time — the freshness rule.
    this.poseCell.read(this.pose);
    const payloadLen = encodeMonoObjectInto(this.payloadScratch, {
      pose: this.pose,
      audio: this.bytesScratch.subarray(0, size),
    });
    const dg = this.dgPool[this.dgPoolIdx]!;
    this.dgPoolIdx = (this.dgPoolIdx + 1) % this.dgPool.length;
    const len = encodeObjectDatagramInto(
      dg,
      this.trackAlias,
      this.seq++, // groupId = gapless packet sequence (§5.2)
      0n,
      this.priority,
      this.payloadScratch.subarray(0, payloadLen)
    );
    this.send(dg.subarray(0, len));
    this.sentDatagrams++;
  }

  /** Flush any buffered Opus packet (fires `handleChunk`) and close the encoder. */
  async stop(): Promise<void> {
    try {
      await this.encoder.flush();
    } catch {
      /* encoder may already be erroring/closed */
    }
    this.encoder.close();
  }
}
