/**
 * Main-side face of the MOQ worker: the RPC client (request/reply
 * correlation + event fan-out) and the session proxy StateSync speaks
 * to. Constructed over any MessagePort-shaped endpoint so it is
 * unit-testable with a fake port; `WorkerTransport.spawn()` builds the
 * real module worker.
 */

import type {
  CaptureEncoderConfigWire,
  DatagramEvent,
  DecodedFormatEvent,
  IncomingSubscribeEvent,
  MainToWorker,
  NoticeEvent,
  SinkAudioConfig,
  SinkIngressEvent,
  SubgroupObjectEvent,
  TransportClosedEvent,
  WireKvp,
  WorkerToMain,
} from './protocol.js';
import { LasaRejectionError, isLasaRejectionCode } from '../lasa/errors.js';
import type { CaptureHandoff } from '../audio/capture.js';
import type { LasaConnectionConfig } from '../lasa/setup.js';
import type { JitterBufferCoreConfig } from '../audio/jitter-buffer-core.js';

/** The minimal Worker surface we use (real Worker, or a test fake). */
export interface WorkerPortLike {
  postMessage(m: unknown, transfer?: Transferable[]): void;
  onmessage: ((e: MessageEvent) => void) | null;
  terminate?(): void;
}

interface PendingRequest {
  resolve: (reply: WorkerToMain) => void;
  reject: (e: Error) => void;
}

export interface WorkerTransportEvents {
  onIncomingSubscribe?: (e: IncomingSubscribeEvent) => void;
  onDatagram?: (e: DatagramEvent) => void;
  onSubgroupObject?: (e: SubgroupObjectEvent) => void;
  onNotice?: (e: NoticeEvent) => void;
  onDecodedFormat?: (e: DecodedFormatEvent) => void;
  onSinkIngress?: (e: SinkIngressEvent) => void;
  /**
   * The WebTransport session closed. After a completed handshake this
   * is how a server-side sever surfaces (moderation block, or
   * supersession by a newer connection exercising this client id —
   * lasa-core.md §4.4).
   */
  onTransportClosed?: (e: TransportClosedEvent) => void;
}

export class WorkerTransport {
  private nextRequestId = 1;
  private pending = new Map<number, PendingRequest>();
  private publishDoneHandlers = new Map<number, (statusCode: number, reason: string) => void>();
  private events: WorkerTransportEvents = {};

  constructor(private readonly port: WorkerPortLike) {
    port.onmessage = (e: MessageEvent) => this.dispatch(e.data as WorkerToMain);
  }

  /** Builds the real module worker (Vite worker pattern). */
  static spawn(): WorkerTransport {
    const worker = new Worker(new URL('./moq-worker.ts', import.meta.url), { type: 'module' });
    return new WorkerTransport(worker as unknown as WorkerPortLike);
  }

  setEvents(events: WorkerTransportEvents): void {
    this.events = { ...this.events, ...events };
  }

  /**
   * PUBLISH_DONE dispatch, keyed by subscribe id — the main-side mirror
   * of the session's one-shot handler table (the worker forwards every
   * instance end; only registered ids are delivered).
   */
  onPublishDone(subscribeId: number, handler: (statusCode: number, reason: string) => void): void {
    this.publishDoneHandlers.set(subscribeId, handler);
  }

  removePublishDoneHandler(subscribeId: number): void {
    this.publishDoneHandlers.delete(subscribeId);
  }

  private dispatch(msg: WorkerToMain): void {
    switch (msg.type) {
      case 'connected':
      case 'subscribed':
      case 'stateWritten':
      case 'sinkTrackSet':
      case 'sinkTrackCleared':
      case 'captureTrackSet':
      case 'captureStopped':
      case 'closed': {
        const p = this.pending.get(msg.id);
        if (p) {
          this.pending.delete(msg.id);
          p.resolve(msg);
        }
        break;
      }
      case 'fail': {
        const p = this.pending.get(msg.id);
        if (p) {
          this.pending.delete(msg.id);
          // A session close carrying a LASA §4.6 code is a deliberate
          // rejection — surface it typed, code first (the reason string
          // is advisory).
          if (msg.closeCode !== undefined && isLasaRejectionCode(msg.closeCode)) {
            p.reject(new LasaRejectionError(msg.closeCode, msg.closeReason ?? msg.message));
          } else {
            p.reject(new Error(msg.message));
          }
        }
        break;
      }
      case 'publishDone': {
        const handler = this.publishDoneHandlers.get(msg.subscribeId);
        if (handler) {
          this.publishDoneHandlers.delete(msg.subscribeId);
          handler(msg.statusCode, msg.reason);
        }
        break;
      }
      case 'incomingSubscribe':
        this.events.onIncomingSubscribe?.(msg);
        break;
      case 'datagram':
        this.events.onDatagram?.(msg);
        break;
      case 'subgroupObject':
        this.events.onSubgroupObject?.(msg);
        break;
      case 'notice':
        this.events.onNotice?.(msg);
        break;
      case 'decodedFormat':
        this.events.onDecodedFormat?.(msg);
        break;
      case 'sinkIngress':
        this.events.onSinkIngress?.(msg);
        break;
      case 'transportClosed':
        this.events.onTransportClosed?.(msg);
        break;
    }
  }

  private request(build: (id: number) => MainToWorker): Promise<WorkerToMain> {
    const id = this.nextRequestId++;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.port.postMessage(build(id));
    });
  }

  async connect(
    url: string,
    config: LasaConnectionConfig,
    serverCertificateHashBase64: string | undefined,
    debug: boolean
  ): Promise<void> {
    await this.request((id) => ({ type: 'connect', id, url, config, serverCertificateHashBase64, debug }));
  }

  async subscribe(
    namespace: string[],
    trackName: string,
    extraParams?: WireKvp[]
  ): Promise<{ subscribeId: number; trackAlias: number }> {
    const reply = await this.request((id) => ({ type: 'subscribe', id, namespace, trackName, extraParams }));
    if (reply.type !== 'subscribed') throw new Error(`unexpected reply ${reply.type}`);
    return { subscribeId: reply.subscribeId, trackAlias: reply.trackAlias };
  }

  /**
   * Fire-and-forget datagram send. postMessage structured-clones the
   * bytes synchronously, so callers may reuse their scratch buffer
   * immediately (no transfer on purpose).
   */
  sendDatagram(bytes: Uint8Array): void {
    this.port.postMessage({ type: 'sendDatagram', bytes } satisfies MainToWorker);
  }

  async writeState(payloads: Uint8Array[]): Promise<void> {
    await this.request((id) => ({ type: 'writeState', id, payloads }));
  }

  /**
   * Divert a sink alias to in-worker decode into the shared jitter ring.
   * The SAB-backed views ride structured clone by reference.
   */
  async setSinkTrack(
    trackAlias: number,
    decoderConfig: SinkAudioConfig,
    jbufConfig: JitterBufferCoreConfig,
    sharedStorage: Float32Array,
    sharedWritePos: BigInt64Array,
    redundancy = 0
  ): Promise<void> {
    await this.request((id) => ({
      type: 'setSinkTrack',
      id,
      trackAlias,
      decoderConfig,
      jbufConfig,
      sharedStorage,
      sharedWritePos,
      redundancy,
    }));
  }

  async clearSinkTrack(trackAlias: number): Promise<void> {
    await this.request((id) => ({ type: 'clearSinkTrack', id, trackAlias }));
  }

  /** Start the worker half of an entity's capture pipeline. */
  async setCaptureTrack(
    entityId: string,
    handoff: CaptureHandoff,
    encoderConfig: CaptureEncoderConfigWire
  ): Promise<void> {
    await this.request((id) => ({
      type: 'setCaptureTrack',
      id,
      entityId,
      ...handoff,
      encoderConfig,
    }));
  }

  async stopCapture(entityId: string): Promise<void> {
    await this.request((id) => ({ type: 'stopCapture', id, entityId }));
  }

  async close(): Promise<void> {
    try {
      await this.request((id) => ({ type: 'close', id }));
    } finally {
      for (const p of this.pending.values()) p.reject(new Error('worker closed'));
      this.pending.clear();
      this.port.terminate?.();
    }
  }
}

/**
 * The session surface StateSync needs, satisfied both by the in-worker
 * MoqSession (structurally) and by {@link WorkerSessionProxy} on main.
 */
export interface StateSession {
  subscribe(
    namespace: string[],
    trackName: string,
    authorization?: string,
    resumeOpId?: bigint,
    extraParams?: WireKvp[]
  ): Promise<number>;
  getTrackAlias(subscribeId: number): number | undefined;
  onPublishDone(subscribeId: number, handler: (statusCode: number, reason: string) => void): void;
  removePublishDoneHandler(subscribeId: number): void;
}

/** Adapts WorkerTransport to the {@link StateSession} surface. */
export class WorkerSessionProxy implements StateSession {
  private aliases = new Map<number, number>();

  constructor(private readonly transport: WorkerTransport) {}

  async subscribe(
    namespace: string[],
    trackName: string,
    _authorization?: string,
    _resumeOpId?: bigint,
    extraParams?: WireKvp[]
  ): Promise<number> {
    const { subscribeId, trackAlias } = await this.transport.subscribe(namespace, trackName, extraParams);
    this.aliases.set(subscribeId, trackAlias);
    return subscribeId;
  }

  getTrackAlias(subscribeId: number): number | undefined {
    return this.aliases.get(subscribeId);
  }

  onPublishDone(subscribeId: number, handler: (statusCode: number, reason: string) => void): void {
    this.transport.onPublishDone(subscribeId, handler);
  }

  removePublishDoneHandler(subscribeId: number): void {
    this.transport.removePublishDoneHandler(subscribeId);
  }
}
