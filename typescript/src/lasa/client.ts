/**
 * The LASA client: one connection owning N entities (multi-entity-first
 * — build-plan D3 requirement). Each entity gets its own mono-object
 * publisher; sinks and presence are subscriptions; authentication is
 * the Connection Config at CLIENT_SETUP and nothing else.
 *
 * Architecture C (step 2 of the audio-machinery port): the transport —
 * WebTransport, MoqSession, datagram read loop, subgroup accept loop —
 * lives in the MOQ worker (src/worker/moq-worker.ts); this class is the
 * main-side facade over the RPC proxy. The state store + StateSync stay
 * HERE, fed by forwarded subgroup objects; publishers frame datagram
 * bytes here and the worker writes them. Main-thread jank therefore
 * cannot gate delivery, and (steps 3/4) the audio paths never touch
 * this thread at all.
 */

import { encodeObjectDatagramInto } from '../transport/moq-transport.js';
import { SubgroupRouter } from '../transport/subgroups.js';
import { DatagramRouter } from '../transport/datagram-router.js';
import {
  entityNamespace,
  presenceNamespace,
  TRACK_MONO_OBJECT,
  type LasaConnectionConfig,
  type LasaEntity,
  type LasaSetup,
} from './setup.js';
import {
  encodeMonoObjectInto,
  parsePresence,
  parseSink,
  type MonoObjectPacket,
  type PresenceDelta,
  type PresenceKeyframe,
  type SinkPacket,
} from './wire.js';
import { encodeStateMessage, type UpstreamStateMessage } from './statewire.js';
import { EntityControls, SpaceControls, type BaseValue } from './base.js';
import { StateSync, type StateSyncOptions } from './state.js';
import { PresenceRoster } from './roster.js';
import type { Store } from './store.js';
import { WorkerTransport, WorkerSessionProxy } from '../worker/worker-transport.js';
import { createMoqWorker } from '../worker/moq-worker-loader.js';
import { SinkPlayer, type SinkPlayerOptions } from '../audio/player.js';
import { CapturePipeline } from '../audio/capture.js';
import type { Pose } from './wire.js';

/** A sink track format: `binaural` (2 channels), `ambi2` (9) or `ambi3` (16). */
export type SinkFormat = 'binaural' | 'ambi2' | 'ambi3';
/** Receives each presence packet with its datagram group id (see {@link LasaClient.subscribePresence}). */
export type PresenceHandler = (groupId: bigint, msg: PresenceKeyframe | PresenceDelta) => void;
/** Receives each sink packet with its sequence number (see {@link LasaClient.subscribeSink}). */
export type SinkHandler = (seq: bigint, packet: SinkPacket) => void;

/** Options for {@link LasaClient.startCapture}. */
export interface CaptureOptions {
  /** The audio source feeding this entity (mic node, oscillator, DAW channel…). */
  source: AudioNode;
  /** Which channel of the source (default 0) — stereo capture = two entities, indices 0/1. */
  channelIndex?: number;
  /**
   * Opus target bitrate in bits per second (default 64000, i.e.
   * 64 kb/s). An encoder setting, not a wire-format one: voice is
   * comfortable at 32–48 kb/s, a music or DAW source wants 96–128.
   * Unrelated to the sample rate, which is always 48 kHz.
   */
  bitrate?: number;
  /** Initial pose (default: home). Update live via the handle. */
  pose?: Pose;
}

/** A running capture, returned by {@link LasaClient.startCapture}. */
export interface CaptureHandle {
  /** Publish the entity's freshest pose — wait-free, call at any rate. */
  setPose(pose: Pose): void;
  /** Stops this entity's capture (same as {@link LasaClient.stopCapture}). */
  stop(): Promise<void>;
}

/** Ingress health of a decoded sink track (see worker protocol). */
export interface SinkIngress {
  /** Sink packets received. */
  received: number;
  /** Runs of one or more missing sequence numbers. */
  gapEvents: number;
  /** Frames never received (after any redundancy repair). */
  lostFrames: number;
  /** Packets that arrived out of sequence order. */
  reordered: number;
  /** Frames the decoder rejected. */
  decodeErrors: number;
}

/** Options for {@link LasaClient.connect}. See `docs/connecting.md`. */
export interface LasaClientOptions {
  /** WebTransport endpoint, e.g. https://host:4443/lasa */
  url: string;
  /** The space to join. In a ticketed space this MUST equal the ticket's `aud` claim. */
  spaceId: string;
  /** This client's public id within the space. With a ticket it MUST match the `client_id` claim. */
  clientId: string;
  /** Signed JWT ticket. Required by a ticketed space, refused by an unticketed one. */
  ticket?: string;
  /** Ad-hoc entity definitions (unticketed spaces, or presenter/admin tickets). */
  entities?: LasaEntity[];
  /** Instantiations of ticket-defined entities, by id. */
  setups?: LasaSetup[];
  /** Dev-cert pinning (base64 SHA-256 from the server boot log). */
  serverCertificateHashBase64?: string;
  /** Console logging from the client, worker and audio graph. */
  debug?: boolean;
  /**
   * The session closed after a completed handshake — a moderation
   * sever, a supersession by a newer connection exercising this client
   * id (lasa-core.md §4.4), or a network/server failure. closeCode
   * carries a LASA §4.6 code when the close was a deliberate LASA
   * action; a rejected handshake surfaces as a LasaRejectionError from
   * connect() instead, never here.
   */
  onClosed?: (info: { closeCode?: number; reason?: string }) => void;
}

/**
 * Publishes one entity's mono-object source track, and carries the
 * owner-scope base-profile operations for it (personal mutes and
 * solos, runtime attrs — see {@link EntityControls}).
 */
export class EntityPublisher {
  private scratch = new Uint8Array(2048);
  private frame = new Uint8Array(1500);
  private readonly controls: EntityControls;

  /** @internal Built by LasaClient. */
  constructor(
    private readonly transport: WorkerTransport,
    private readonly alias: number,
    /** The entity this publisher owns. */
    readonly entityId: string,
    client: LasaClient
  ) {
    this.controls = new EntityControls(client, entityId);
  }

  /** Silences the pair (this entity, `other`) in both directions (profile §6). */
  mute(other: string): Promise<void> {
    return this.controls.mute(other);
  }
  unmute(other: string): Promise<void> {
    return this.controls.unmute(other);
  }
  /** Hear `other` (and the other solo'd entities) only; wins over mutes. */
  solo(other: string): Promise<void> {
    return this.controls.solo(other);
  }
  unsolo(other: string): Promise<void> {
    return this.controls.unsolo(other);
  }
  /** Sets one runtime attribute at a dotted path. */
  setAttr(path: string, value: BaseValue): Promise<void> {
    return this.controls.setAttr(path, value);
  }
  clearAttr(path: string): Promise<void> {
    return this.controls.clearAttr(path);
  }
  /** Sets several attributes as one group. */
  setAttrs(attrs: Record<string, BaseValue>): Promise<void> {
    return this.controls.setAttrs(attrs);
  }

  /**
   * Sends one mono-object packet with the gapless sequence number as
   * the datagram group id (§5.2). The framed bytes are structured-cloned
   * at postMessage, so the scratch is immediately reusable.
   */
  async publish(seq: bigint, packet: MonoObjectPacket): Promise<void> {
    const payloadLen = encodeMonoObjectInto(this.frame, packet);
    const n = encodeObjectDatagramInto(
      this.scratch,
      this.alias,
      seq,
      0n,
      0,
      this.frame.subarray(0, payloadLen)
    );
    this.transport.sendDatagram(this.scratch.subarray(0, n));
  }
}

/**
 * A connection to one LASA space: capture, playout, presence and
 * state over a single WebTransport session. Create one with
 * {@link LasaClient.connect}; end it with {@link LasaClient.close}.
 */
export class LasaClient {
  private publishers = new Map<string, EntityPublisher>();
  private publisherWaiters = new Map<string, { resolve: (p: EntityPublisher) => void; reject: (e: Error) => void }[]>();
  private readonly subgroups = new SubgroupRouter();
  private readonly datagrams = new DatagramRouter();
  private readonly session: WorkerSessionProxy;
  private stateSync: StateSync | null = null;
  private sinkPlayers = new Map<string, { player: SinkPlayer; trackAlias: number }>();
  private capturePipelines = new Map<string, CapturePipeline>();
  /** Latest ingress-health report per sink alias (worker posts ~2 s). */
  private sinkIngress = new Map<number, SinkIngress>();
  /**
   * The connection's own entity definitions, by id. The declarations
   * are path-scoped (lasa-core.md §4.2): playSink defaults the playout
   * buffer's quality level from the entity's declared quality. Only
   * ad-hoc entities are known here — a ticket-defined entity's
   * definition rides in the ticket, so its sinks take the interactive
   * default unless the caller passes qualityLevel explicitly.
   */
  private entityDefs = new Map<string, LasaEntity>();
  private closing: Promise<void> | null = null;
  // Slots reserved synchronously by playSink/startCapture so two
  // overlapping calls for one entity cannot both pass the "already
  // running" guard while the first is still awaiting (B1).
  private sinkStarting = new Set<string>();
  private captureStarting = new Set<string>();
  /**
   * Space-wide base-profile operations — moderation, channel policy,
   * rendering (see {@link SpaceControls}). Role-gated by the server.
   */
  readonly space: SpaceControls;

  private constructor(
    private readonly transport: WorkerTransport,
    private readonly spaceId: string,
    private readonly clientId: string,
    private readonly debug: boolean
  ) {
    this.session = new WorkerSessionProxy(transport);
    this.space = new SpaceControls(this);
  }

  /**
   * Spawns the MOQ worker, connects, authenticates via the Connection
   * Config, and wires the per-entity publishers as the server
   * subscribes them back.
   */
  static async connect(opts: LasaClientOptions): Promise<LasaClient> {
    const transport = WorkerTransport.spawn(createMoqWorker());
    const client = new LasaClient(transport, opts.spaceId, opts.clientId, opts.debug ?? false);
    for (const e of opts.entities ?? []) client.entityDefs.set(e.id, e);

    // The worker reports every transport close, including one that ends
    // a rejected handshake. That case is connect()'s to surface (as a
    // LasaRejectionError), so onClosed is gated on a completed connect.
    let connected = false;
    transport.setEvents({
      // The server subscribes our source tracks after SETUP; the worker
      // auto-OKs each and forwards the alias it assigned — that alias IS
      // the entity's publisher handle. (The client state source is
      // handled inside the worker, where its stream lives.)
      onIncomingSubscribe: ({ namespace, trackAlias }) => {
        if (namespace.length === 4 && namespace[1] === 'entity' && namespace[3] === 'source') {
          client.resolvePublisher(namespace[2]!, new EntityPublisher(transport, trackAlias, namespace[2]!, client));
        }
      },
      onDatagram: ({ trackAlias, groupId, objectId, payload }) => {
        client.datagrams.ingest({ trackAlias, groupId, objectId, payload });
      },
      onSubgroupObject: ({ trackAlias, groupId, subgroupId, objectId, payload }) => {
        client.subgroups.ingest(
          { trackAlias, groupId, subgroupId, publisherPriority: 0, endOfGroup: false },
          { objectId, payload }
        );
      },
      onNotice: (n) => {
        if (opts.debug) console.log('[lasa] worker notice:', n.event, n.detail ?? '');
      },
      onSinkIngress: (e) => {
        client.sinkIngress.set(e.trackAlias, {
          received: e.received,
          gapEvents: e.gapEvents,
          lostFrames: e.lostFrames,
          reordered: e.reordered,
          decodeErrors: e.decodeErrors,
        });
      },
      onTransportClosed: (e) => {
        if (!connected) return;
        opts.onClosed?.({ closeCode: e.closeCode, reason: e.reason });
      },
    });

    const config: LasaConnectionConfig = {
      clientId: opts.clientId,
      ticket: opts.ticket,
      entities: opts.entities,
      setups: opts.setups,
    };
    try {
      await transport.connect(opts.url, config, opts.serverCertificateHashBase64, opts.debug ?? false);
    } catch (e) {
      await transport.close().catch(() => {});
      throw e;
    }
    connected = true;
    return client;
  }

  /**
   * Publishes client-authored state ops on the state source subgroup
   * (upstream layout — no seqs). The worker waits for the server's
   * subscribe of the state source if it has not landed yet.
   */
  async writeState(...msgs: UpstreamStateMessage[]): Promise<void> {
    await this.transport.writeState(msgs.map((m) => encodeStateMessage(m, 'upstream')));
  }

  /**
   * Runs the state subscription against the store: subscribe with the
   * store's cursor, feed both lanes, re-subscribe with backoff on
   * termination. Returns the running StateSync; runs until stop() or
   * close(). All store access from the caller should await
   * sync.settled() first.
   */
  syncState(store: Store, opts: StateSyncOptions = {}): StateSync {
    if (this.stateSync) return this.stateSync;
    const sync = new StateSync(this.session, this.subgroups, this.spaceId, this.clientId, store, {
      debug: this.debug,
      ...opts,
    });
    this.stateSync = sync;
    void sync.run();
    return sync;
  }

  private resolvePublisher(entityId: string, pub: EntityPublisher): void {
    this.publishers.set(entityId, pub);
    const waiters = this.publisherWaiters.get(entityId);
    if (waiters) {
      this.publisherWaiters.delete(entityId);
      for (const w of waiters) w.resolve(pub);
    }
  }

  /**
   * Owner-scope base-profile controls for one of this connection's
   * entities, without waiting for its publisher. The same operations
   * are on {@link EntityPublisher}.
   */
  controls(entityId: string): EntityControls {
    return new EntityControls(this, entityId);
  }

  /**
   * The entity's publisher; resolves when the server has subscribed
   * the entity's source track (normally within the first RTTs).
   */
  entity(entityId: string, timeoutMs = 5000): Promise<EntityPublisher> {
    const existing = this.publishers.get(entityId);
    if (existing) return Promise.resolve(existing);
    return new Promise((resolve, reject) => {
      const waiter = {
        resolve: (p: EntityPublisher) => {
          clearTimeout(timer);
          resolve(p);
        },
        reject: (e: Error) => {
          clearTimeout(timer);
          reject(e);
        },
      };
      const timer = setTimeout(() => {
        // Leave the list too, or a polled entity the server never
        // subscribes grows the waiter list without bound.
        const list = this.publisherWaiters.get(entityId);
        if (list) {
          const i = list.indexOf(waiter);
          if (i >= 0) list.splice(i, 1);
          if (list.length === 0) this.publisherWaiters.delete(entityId);
        }
        reject(new Error(`server did not subscribe entity ${entityId} source in ${timeoutMs}ms`));
      }, timeoutMs);
      const list = this.publisherWaiters.get(entityId) ?? [];
      list.push(waiter);
      this.publisherWaiters.set(entityId, list);
    });
  }

  /**
   * Subscribes an entity's sink track and plays it through the 3-thread
   * playout path: worker decode → SAB jitter ring → playout worklet.
   * Main never touches the audio. Returns the SinkPlayer (volume,
   * jitter stats, Tap B); call {@link stopSink} or close() to end it.
   * One player per entity sink (multi-entity: call per entity).
   */
  async playSink(entityId: string, format: SinkFormat, opts: SinkPlayerOptions = {}): Promise<SinkPlayer> {
    if (this.sinkPlayers.has(entityId) || this.sinkStarting.has(entityId)) {
      throw new Error(`sink for entity ${entityId} is already playing`);
    }
    this.sinkStarting.add(entityId);
    try {
      return await this.playSinkInner(entityId, format, opts);
    } finally {
      this.sinkStarting.delete(entityId);
    }
  }

  private async playSinkInner(entityId: string, format: SinkFormat, opts: SinkPlayerOptions): Promise<SinkPlayer> {
    // The entity's declared quality and redundancy default the playout
    // buffer's level and FEC floor (path-scoped declarations,
    // lasa-core.md §4.2): the declaration asked the space to emit
    // repeats on this sink track, so the reassembler and the floor
    // provision for them. The sink half never touches the wire, so
    // explicit opts are a legitimate local departure and win.
    const def = this.entityDefs.get(entityId);
    const player = await SinkPlayer.create({
      debug: this.debug,
      ...(def?.quality !== undefined ? { qualityLevel: def.quality } : {}),
      ...(def?.redundancy !== undefined ? { redundancy: def.redundancy } : {}),
      ...opts,
    });
    try {
      const { trackAlias } = await this.transport.subscribe(
        entityNamespace(this.spaceId, entityId, 'sink'),
        format
      );
      const ring = player.getRingHandoff();
      await this.transport.setSinkTrack(
        trackAlias,
        player.getDecoderConfig(),
        ring.jbufConfig,
        ring.sharedStorage,
        ring.sharedWritePos,
        ring.redundancy
      );
      this.sinkPlayers.set(entityId, { player, trackAlias });
      await player.resume();
      return player;
    } catch (e) {
      await player.dispose().catch(() => {});
      throw e;
    }
  }

  /**
   * Starts an entity's capture: any AudioNode (mic source, oscillator,
   * DAW channel) feeds a mono pipeline — worklet → SAB ring → worker
   * Opus encode → mono-object datagrams stamped with the FRESHEST pose
   * from the shared cell (update it via {@link CaptureHandle.setPose}).
   * Per-entity: call once per publishing entity, sharing one context.
   *
   * The source's AudioContext MUST run at 48 kHz (lasa-core.md §5: Opus,
   * 48 kHz, 5 ms) — construct it with `new AudioContext({ sampleRate:
   * 48000 })`; any other rate is refused. Frames are always 5 ms.
   */
  async startCapture(entityId: string, opts: CaptureOptions): Promise<CaptureHandle> {
    if (this.capturePipelines.has(entityId) || this.captureStarting.has(entityId)) {
      throw new Error(`capture for entity ${entityId} is already running`);
    }
    this.captureStarting.add(entityId);
    try {
      return await this.startCaptureInner(entityId, opts);
    } finally {
      this.captureStarting.delete(entityId);
    }
  }

  private async startCaptureInner(entityId: string, opts: CaptureOptions): Promise<CaptureHandle> {
    const pipeline = await CapturePipeline.create(opts.source, { channelIndex: opts.channelIndex });
    try {
      await this.transport.setCaptureTrack(entityId, pipeline.getHandoff(), {
        codec: 'opus',
        sampleRate: pipeline.sampleRate,
        numberOfChannels: 1,
        bitrate: opts.bitrate ?? 64000,
        // The LASA frame is 5 ms, always (lasa-core.md §5): the server
        // extends timestamps by seq × 5 ms and redundancy offsets are
        // multiples of it. Not a caller option.
        frameDurationUs: 5000,
      });
    } catch (e) {
      pipeline.dispose();
      throw e;
    }
    this.capturePipelines.set(entityId, pipeline);
    if (opts.pose) pipeline.setPose(opts.pose);
    return {
      setPose: (pose) => pipeline.setPose(pose),
      stop: () => this.stopCapture(entityId),
    };
  }

  /** Capture-ring overflows for a capturing entity (each = a dropped quantum = an upstream gap), or null. */
  getCaptureOverflows(entityId: string): number | null {
    return this.capturePipelines.get(entityId)?.getOverflows() ?? null;
  }

  /** Stops an entity's capture: flushes the worker encoder, removes the graph nodes. */
  async stopCapture(entityId: string): Promise<void> {
    const pipeline = this.capturePipelines.get(entityId);
    if (!pipeline) return;
    this.capturePipelines.delete(entityId);
    await this.transport.stopCapture(entityId).catch(() => {});
    pipeline.dispose();
  }

  /** Stops a playing sink: worker decode ends, then the graph tears down. */
  async stopSink(entityId: string): Promise<void> {
    const entry = this.sinkPlayers.get(entityId);
    if (!entry) return;
    this.sinkPlayers.delete(entityId);
    this.sinkIngress.delete(entry.trackAlias);
    await this.transport.clearSinkTrack(entry.trackAlias).catch(() => {});
    await entry.player.dispose();
  }

  /**
   * Latest ingress health for a playing sink (updated ~2 s by the
   * worker), or null before the first report. Distinguishes network
   * damage (gaps/loss/reorder — clicks that are NOT the jitter
   * buffer's) from buffer behaviour (the player's getEvents()).
   */
  getSinkIngress(entityId: string): SinkIngress | null {
    const entry = this.sinkPlayers.get(entityId);
    if (!entry) return null;
    return this.sinkIngress.get(entry.trackAlias) ?? null;
  }

  /** Subscribes an entity's sink track (own-or-granted per §3.1/§2). */
  async subscribeSink(entityId: string, format: SinkFormat, handler: SinkHandler): Promise<void> {
    const { trackAlias } = await this.transport.subscribe(
      entityNamespace(this.spaceId, entityId, 'sink'),
      format
    );
    this.datagrams.register(trackAlias, (payload, _alias, groupId) => {
      let pkt: SinkPacket;
      try {
        pkt = parseSink(payload);
      } catch {
        return; // Discard rule: unknown flags / malformed packets are dropped.
      }
      handler(groupId, pkt); // the consumer's own faults are theirs to see
    });
  }

  /**
   * Subscribes the space presence track. The handler receives each
   * packet's datagram group-id — part of the §7 receive contract
   * (per-index latest-wins). Most consumers want subscribeRoster,
   * which applies the §7 rules once and maintains the roster.
   */
  async subscribePresence(handler: PresenceHandler): Promise<void> {
    const { trackAlias } = await this.transport.subscribe(presenceNamespace(this.spaceId), 'presence');
    this.datagrams.register(trackAlias, (payload, _alias, groupId) => {
      let msg: PresenceKeyframe | PresenceDelta;
      try {
        msg = parsePresence(payload);
      } catch {
        return; // Discard rule.
      }
      handler(groupId, msg);
    });
  }

  /**
   * Subscribes presence and maintains a PresenceRoster from it — the
   * §7 client rules (generation discard, per-index latest-wins, reset
   * on generation change) applied in the SDK.
   */
  async subscribeRoster(onUpdate?: () => void): Promise<PresenceRoster> {
    const roster = new PresenceRoster();
    if (onUpdate) roster.onUpdate = onUpdate;
    await this.subscribePresence((groupId, msg) => roster.apply(groupId, msg));
    return roster;
  }

  /**
   * Ends the session: stops every capture and sink, stops the state
   * sync, and tears down the worker and transport. Idempotent.
   */
  async close(): Promise<void> {
    // Idempotent: the first call terminates the worker, so a second
    // request to it would never be answered. Repeat calls share the
    // first's promise.
    if (this.closing) return this.closing;
    this.closing = this.closeOnce();
    return this.closing;
  }

  private async closeOnce(): Promise<void> {
    this.stateSync?.stop();
    // Main-side graphs first; the worker close flushes encoders and
    // tears down decoders with the transport.
    for (const [, pipeline] of this.capturePipelines) {
      pipeline.dispose();
    }
    this.capturePipelines.clear();
    for (const [, { player }] of this.sinkPlayers) {
      await player.dispose().catch(() => {});
    }
    this.sinkPlayers.clear();
    this.sinkIngress.clear();
    this.datagrams.clear();
    // Pending entity() calls fail now with the real reason, not after
    // their timeout with "server did not subscribe".
    const waiters = [...this.publisherWaiters.values()].flat();
    this.publisherWaiters.clear();
    for (const w of waiters) w.reject(new Error('client closed'));
    await this.transport.close();
  }
}

export { TRACK_MONO_OBJECT };
