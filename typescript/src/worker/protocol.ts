/**
 * The main ↔ worker RPC protocol for the MOQ worker (build-plan D3,
 * architecture C — the old SDK's proven worker-transport shape adapted
 * for LASA; design lineage: cloud-mixer/plan/history/browser-audio/
 * worker-transport-design.md).
 *
 * The worker owns the WebTransport, the MoqSession (control stream),
 * the datagram read loop, and the subgroup-stream accept loop; main
 * owns the app-facing client, the state store/sync, and the audio
 * worklets. Everything here is low-rate and jank-tolerant — the
 * real-time audio paths (steps 3/4) ride SharedArrayBuffer rings, never
 * this channel.
 *
 * Commands carry a request `id`; the worker answers each with exactly
 * one reply (`fail` on error). Events are unsolicited. BigInt fields
 * ride structured clone natively.
 */

import type { LasaConnectionConfig } from '../lasa/setup.js';
import type { MonoObjectPacket } from '../lasa/wire.js';
import type { JitterBufferCoreConfig } from '../audio/jitter-buffer-core.js';

/** A subscribe/setup parameter in transferable form. */
export interface WireKvp {
  type: number;
  value: bigint | Uint8Array;
}

/** WebCodecs decoder config for a sink track + timestamp synthesis. */
export interface SinkAudioConfig {
  codec: string;
  sampleRate: number;
  numberOfChannels: number;
  /** Microseconds per sink frame (timestamps = seq × this). */
  frameDurationUs: number;
}

// ---- main → worker -------------------------------------------------------

export interface ConnectCommand {
  type: 'connect';
  id: number;
  url: string;
  config: LasaConnectionConfig;
  serverCertificateHashBase64?: string;
  debug: boolean;
}

export interface SubscribeCommand {
  type: 'subscribe';
  id: number;
  namespace: string[];
  trackName: string;
  extraParams?: WireKvp[];
}

/** Fire-and-forget: a fully framed OBJECT_DATAGRAM to write. */
export interface SendDatagramCommand {
  type: 'sendDatagram';
  bytes: Uint8Array;
}

/**
 * Fire-and-forget: one mono-object packet on an entity's source track.
 * The worker owns the entity's seq counter and redundancy ring (shared
 * with a running capture on the same entity — lasa-core.md §5.1 allows
 * exactly one gapless counter per track), frames it, and sends it.
 * Dropped with a notice if the server has not subscribed the source yet.
 */
export interface PublishCommand {
  type: 'publish';
  entityId: string;
  packet: MonoObjectPacket;
}

/**
 * Write upstream-encoded state messages on the client state-source
 * subgroup stream. The worker waits for the server's subscribe of the
 * state source (it assigns the alias), opens the stream lazily, and
 * owns the object-id sequence.
 */
export interface WriteStateCommand {
  type: 'writeState';
  id: number;
  payloads: Uint8Array[];
}

/**
 * Divert a subscribed sink-track alias to in-worker decode: datagrams
 * for it are parsed as SinkPackets, Opus-decoded, and written into the
 * SAB jitter ring the playout worklet reads — they stop being forwarded
 * to main. The SAB-backed views are shared (structured clone shares the
 * underlying SharedArrayBuffer); `jbufConfig` must be the SAME config
 * the worklet's reader core was built with.
 */
export interface SetSinkTrackCommand {
  type: 'setSinkTrack';
  id: number;
  trackAlias: number;
  decoderConfig: SinkAudioConfig;
  jbufConfig: JitterBufferCoreConfig;
  sharedStorage: Float32Array;
  sharedWritePos: BigInt64Array;
  /**
   * The entity's declared redundancy (max offset 0–7, lasa-core.md
   * §4.2). > 0 puts the FEC reassembler in front of the decoder:
   * lost frames repair from redundant copies, unrepairable ones become
   * zero-content concealment frames (plan/sink-redundancy.md piece 2).
   * The caller composes the matching FEC floor into `jbufConfig.floor`.
   */
  redundancy?: number;
}

/** Stop decoding a sink alias (its datagrams forward to main again). */
export interface ClearSinkTrackCommand {
  type: 'clearSinkTrack';
  id: number;
  trackAlias: number;
}

/** WebCodecs Opus encoder config for a capture pipeline. */
export interface CaptureEncoderConfigWire {
  codec: string;
  sampleRate: number;
  numberOfChannels: number;
  bitrate: number;
  /** Opus frame duration in microseconds (5000 = the LASA default). */
  frameDurationUs: number;
  /**
   * Uplink redundancy offset override for this entity (0–7); absent =
   * the entity's declaration. Applied to the entity's sequencer, so it
   * also governs pose-only publishes on the same entity.
   */
  redundancy?: number;
}

/**
 * Start the worker-side half of one entity's capture pipeline: a
 * consumer CaptureRing over the shared cells (the capture worklet is
 * the producer), a WebCodecs Opus encoder pumped on the Atomics wake
 * signal, LASA mono-object framing with the pose read from the shared
 * pose cell at datagram-framing time (the freshness rule), sent on the
 * entity's source-track alias (the worker waits for the server's
 * subscribe-back if it hasn't landed). All views are SAB-backed and
 * ride structured clone by reference.
 */
export interface SetCaptureTrackCommand {
  type: 'setCaptureTrack';
  id: number;
  entityId: string;
  numChannels: number;
  capacityFrames: number;
  sharedStorage: Float32Array;
  sharedWritePos: BigInt64Array;
  sharedReadPos: BigInt64Array;
  /** The wake cell: the capture worklet notifies, the worker waits. */
  sharedSignal: Int32Array;
  /** The entity's pose seqlock cell (see src/audio/pose-cell.ts). */
  poseSeq: Int32Array;
  poseValues: Float64Array;
  encoderConfig: CaptureEncoderConfigWire;
}

/** Stop one entity's capture (flushes + closes its encoder). */
export interface StopCaptureCommand {
  type: 'stopCapture';
  id: number;
  entityId: string;
}

export interface CloseCommand {
  type: 'close';
  id: number;
}

export type MainToWorker =
  | ConnectCommand
  | SubscribeCommand
  | SendDatagramCommand
  | PublishCommand
  | WriteStateCommand
  | SetSinkTrackCommand
  | ClearSinkTrackCommand
  | SetCaptureTrackCommand
  | StopCaptureCommand
  | CloseCommand;

// ---- worker → main -------------------------------------------------------

export interface ConnectedReply {
  type: 'connected';
  id: number;
}

export interface SubscribedReply {
  type: 'subscribed';
  id: number;
  subscribeId: number;
  trackAlias: number;
}

export interface StateWrittenReply {
  type: 'stateWritten';
  id: number;
}

export interface SinkTrackSetReply {
  type: 'sinkTrackSet';
  id: number;
}

export interface SinkTrackClearedReply {
  type: 'sinkTrackCleared';
  id: number;
}

export interface CaptureTrackSetReply {
  type: 'captureTrackSet';
  id: number;
}

export interface CaptureStoppedReply {
  type: 'captureStopped';
  id: number;
}

export interface ClosedReply {
  type: 'closed';
  id: number;
}

/** Any command's error reply. */
export interface FailReply {
  type: 'fail';
  id: number;
  message: string;
  /**
   * The transport's close code/reason when the failure was a session
   * close — for a rejected CLIENT_SETUP this is the LASA §4.6 code
   * (lasa-core.md: correctness rides the code alone). Absent when the
   * failure was not a close.
   */
  closeCode?: number;
  closeReason?: string;
}

/**
 * The WebTransport session closed (server-initiated or network). After
 * a completed handshake this is how a LASA sever surfaces — moderation
 * block (0x1A5A06 was at admission; live blocks just close) or
 * supersession by a newer exercise of this client's capability
 * (lasa-core.md §4.4).
 */
export interface TransportClosedEvent {
  type: 'transportClosed';
  closeCode?: number;
  reason?: string;
}

/** The server subscribed one of our source tracks (alias assigned by us). */
export interface IncomingSubscribeEvent {
  type: 'incomingSubscribe';
  namespace: string[];
  trackAlias: number;
}

/** PUBLISH_DONE for a subscription instance we opened. */
export interface PublishDoneEvent {
  type: 'publishDone';
  subscribeId: number;
  statusCode: number;
  reason: string;
}

/** A received datagram, forwarded (step 2: all; step 3 diverts sink audio). */
export interface DatagramEvent {
  type: 'datagram';
  trackAlias: number;
  groupId: bigint;
  objectId: bigint;
  payload: Uint8Array;
}

/** One object from an incoming subgroup stream, forwarded. */
export interface SubgroupObjectEvent {
  type: 'subgroupObject';
  trackAlias: number;
  groupId: bigint;
  subgroupId: bigint;
  objectId: bigint;
  payload: Uint8Array;
}

/** Diagnostics: loop endings, decode and encode errors. */
export interface NoticeEvent {
  type: 'notice';
  event:
    | 'datagram-reader-done'
    | 'datagram-reader-error'
    | 'subgroup-accept-done'
    | 'subgroup-stream-error'
    | 'decode-error'
    | 'encode-error'
    | 'publish-dropped';
  detail?: string;
}

/**
 * Format watch (emitted on change only): what a sink decoder is ACTUALLY
 * producing — a configured-stereo decoder emitting mono, or a
 * sample-rate surprise, is mono collapse at the decode layer and shows
 * up right here.
 */
export interface DecodedFormatEvent {
  type: 'decodedFormat';
  trackAlias: number;
  numberOfChannels: number;
  sampleRate: number;
  nativeFormat: string | null;
  /** Which AudioData.copyTo path this engine supports. */
  copyPath: 'f32' | 'f32-planar';
}

/**
 * Periodic ingress-health report for a decoded sink track (every ~2 s
 * while decoding). Distinguishes network damage from buffer behaviour:
 * a seq gap means ~5 ms of audio never arrived — the decoder output
 * jumps hard across it (a click that is NOT the jitter buffer's fault).
 */
export interface SinkIngressEvent {
  type: 'sinkIngress';
  trackAlias: number;
  /** Datagrams decoded. */
  received: number;
  /** Distinct gap occurrences (seq jumped forward past a hole). */
  gapEvents: number;
  /** Total frames missing across those gaps. */
  lostFrames: number;
  /** Datagrams that arrived with seq below the highest seen (late/reordered — fed to the decoder anyway). */
  reordered: number;
  decodeErrors: number;
  /** Sink-FEC reassembler outcomes (0 unless redundancy was declared):
   * frames recovered from redundant copies, zero-content concealments
   * at give-up, and freeze-sized holes skipped to the buffer. */
  fecRepaired: number;
  fecConcealed: number;
  fecSkipped: number;
}

export type WorkerToMain =
  | ConnectedReply
  | SubscribedReply
  | StateWrittenReply
  | SinkTrackSetReply
  | SinkTrackClearedReply
  | CaptureTrackSetReply
  | CaptureStoppedReply
  | ClosedReply
  | SinkIngressEvent
  | FailReply
  | IncomingSubscribeEvent
  | PublishDoneEvent
  | DatagramEvent
  | SubgroupObjectEvent
  | NoticeEvent
  | DecodedFormatEvent
  | TransportClosedEvent;
