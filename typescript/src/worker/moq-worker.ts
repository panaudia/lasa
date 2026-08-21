/**
 * The MOQ worker entry: owns, OFF THE MAIN THREAD, the WebTransport,
 * the MoqSession (control stream + message loop), the datagram read
 * loop, and the subgroup-stream accept loop — so main-thread jank can
 * never gate delivery (architecture C; the old SDK's moq-worker shape,
 * rebuilt for LASA's subgroup state plane).
 *
 * Step 2 scope: everything received is forwarded to main (datagrams and
 * subgroup objects alike). Step 3 diverts the sink-audio alias to
 * WebCodecs decode + the SAB playout ring; step 4 adds the per-entity
 * capture encoders. The client state-source writer lives HERE (the
 * stream belongs to this transport): main sends upstream-encoded state
 * ops, the worker waits for the server's subscribe of the state source,
 * opens the stream lazily, and owns the object-id sequence.
 *
 * Loaded as a Vite module worker:
 *   new Worker(new URL('./moq-worker.ts', import.meta.url), { type: 'module' })
 */

import { MoqConnection } from '../transport/connection.js';
import { MoqSession } from '../transport/session.js';
import { ConnectionState, MoqRole } from '../transport/types.js';
import { parseObjectDatagram } from '../transport/moq-transport.js';
import { readSubgroupStream, SubgroupWriter } from '../transport/subgroups.js';
import { connectionConfigParam } from '../lasa/setup.js';
import { parseSink } from '../lasa/wire.js';
import { SinkReassembler } from '../audio/sink-reassembler.js';
import { JitterBufferCore } from '../audio/jitter-buffer-core.js';
import { CaptureRing } from '../audio/capture-ring.js';
import { CaptureEncoder, type EncodedChunkLike, type FrameEncoder } from '../audio/capture-encoder.js';
import { PoseCell } from '../audio/pose-cell.js';
import type { MainToWorker, SetCaptureTrackCommand, SetSinkTrackCommand, WorkerToMain } from './protocol.js';

// tsconfig lib is DOM (not WebWorker); type the worker globals locally.
interface WorkerCtx {
  onmessage: ((e: MessageEvent) => void) | null;
  postMessage(m: unknown, transfer?: Transferable[]): void;
}
const ctx = self as unknown as WorkerCtx;

function post(m: WorkerToMain, transfer?: Transferable[]): void {
  ctx.postMessage(m, transfer);
}

let conn: MoqConnection | null = null;
let session: MoqSession | null = null;
let debug = false;
// The last transport close info (LASA §4.6 code + reason on a rejected
// CLIENT_SETUP) — used to enrich a connect failure, since the close and
// the failed SERVER_SETUP read race.
let lastCloseInfo: { closeCode?: number; reason?: string } | null = null;
// One transportClosed post per connection: a graceful close (capsule,
// carries the code) and an abrupt one (closed promise rejects — how a
// server-side sever surfaces in browsers) must not double-report.
let closePosted = false;

// Client state-source upstream (moved here from the main-thread client:
// the subgroup stream belongs to this transport).
let stateSourceAlias: number | null = null;
let stateAliasWaiters: (() => void)[] = [];
let stateWriter: SubgroupWriter | null = null;
let stateObjId = 0n;

// Entity source aliases (the server's subscribe-backs), for capture
// pipelines: entityId → trackAlias, with waiters for captures started
// before the subscribe-back lands.
const entityAliases = new Map<string, number>();
const entityAliasWaiters = new Map<string, (() => void)[]>();

function waitForEntityAlias(entityId: string, timeoutMs = 5000): Promise<number> {
  const existing = entityAliases.get(entityId);
  if (existing !== undefined) return Promise.resolve(existing);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`server did not subscribe entity ${entityId} source in ${timeoutMs}ms`)),
      timeoutMs
    );
    const list = entityAliasWaiters.get(entityId) ?? [];
    list.push(() => {
      clearTimeout(timer);
      resolve(entityAliases.get(entityId)!);
    });
    entityAliasWaiters.set(entityId, list);
  });
}

// Per-entity capture pipelines (step 4): consumer ring + Opus encoder
// pumped on the worklet's Atomics wake, framing mono-object datagrams
// with the SAB-fresh pose.
interface Capture {
  encoder: CaptureEncoder;
  signal: Int32Array;
  running: boolean;
}
const captures = new Map<string, Capture>();

// Sink playout (step 3): per-alias decode straight into the SAB jitter
// ring the playout worklet reads. Multiple sinks (multi-entity) each
// get their own decoder + writer-view core.
interface SinkTrack {
  decoder: AudioDecoder;
  jbuf: JitterBufferCore;
  frameDurationUs: number;
  formatKey: string;
  // Sink-FEC reassembler (null unless the entity declared redundancy):
  // sits in front of the decoder, per (MoQ session, track) instance.
  reasm: SinkReassembler | null;
  // Concealment frames awaiting their slot in the ring. Decode is
  // async, so a give-up's zero frame cannot be written directly — it
  // would land ahead of decoder outputs already in flight. It queues
  // here and flushes in the output callback, ordered by timestamp.
  pendingConceal: bigint[];
  // Ingress health (posted periodically as 'sinkIngress').
  lastSeq: bigint | null;
  received: number;
  gapEvents: number;
  lostFrames: number;
  reordered: number;
  decodeErrors: number;
  ingressTimer: ReturnType<typeof setInterval>;
}
const sinkTracks = new Map<number, SinkTrack>();

// AudioData.copyTo path: the spec only mandates 'f32-planar'; the
// interleaved 'f32' copy is optional and Firefox rejects it. Probe once
// per worker lifetime (an engine property), then stick to what works.
let copyPath: 'f32' | 'f32-planar' | null = null;
let decodePcm = new Float32Array(0); // grow-on-demand decode scratch
let planarScratch = new Float32Array(0);
let zeroPcm = new Float32Array(0); // grow-on-demand all-zero concealment scratch (never written)

/**
 * Copy one decoded AudioData into `pcm` as interleaved f32, preferring
 * the single interleaved copy and falling back to per-plane planar.
 */
function copyDecoded(audioData: AudioData, pcm: Float32Array, frames: number, channels: number): void {
  if (copyPath !== 'f32-planar') {
    try {
      audioData.copyTo(pcm, { planeIndex: 0, format: 'f32' });
      copyPath = 'f32';
      return;
    } catch {
      copyPath = 'f32-planar'; // engine refuses interleaved copies
    }
  }
  if (planarScratch.length < frames) planarScratch = new Float32Array(frames);
  const plane = planarScratch.subarray(0, frames);
  for (let ch = 0; ch < channels; ch++) {
    audioData.copyTo(plane, { planeIndex: ch, format: 'f32-planar' });
    for (let i = 0; i < frames; i++) pcm[i * channels + ch] = plane[i]!;
  }
}

function log(...args: unknown[]): void {
  if (debug) console.log('[moq-worker]', ...args);
}

function certHashOption(base64?: string): { algorithm: string; value: ArrayBuffer }[] | undefined {
  if (!base64) return undefined;
  const raw = atob(base64);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return [{ algorithm: 'sha-256', value: bytes.buffer }];
}

async function handleConnect(cmd: Extract<MainToWorker, { type: 'connect' }>): Promise<void> {
  debug = cmd.debug;
  lastCloseInfo = null;
  closePosted = false;
  conn = new MoqConnection(cmd.url, debug);
  // Capture the session-close info: a rejected CLIENT_SETUP is a
  // deliberate close carrying the LASA §4.6 code + reason (lasa-core.md
  // §4.6 — the browser reliably learns WHY because rejection happens at
  // the MoQ layer inside an established session). Post-handshake closes
  // (moderation sever, supersession) surface through the same event —
  // graceful closes via onClose with the code, abrupt ones (the closed
  // promise rejects, how a server-side sever typically lands) via the
  // ERROR state change, code-less.
  conn.setHandlers({
    onClose: (info) => {
      lastCloseInfo = { closeCode: info.closeCode, reason: info.reason };
      if (!closePosted) {
        closePosted = true;
        post({ type: 'transportClosed', closeCode: info.closeCode, reason: info.reason });
      }
    },
    onStateChange: (state, error) => {
      if (state === ConnectionState.ERROR && !closePosted) {
        closePosted = true;
        lastCloseInfo ??= { reason: String(error ?? 'transport error') };
        post({ type: 'transportClosed', reason: String(error ?? 'transport error') });
      }
    },
  });
  await conn.connect({
    protocols: ['moqt-16'],
    serverCertificateHashes: certHashOption(cmd.serverCertificateHashBase64),
  });
  session = new MoqSession(conn, debug);

  session.onIncomingSubscribe((namespace, trackAlias) => {
    if (namespace.length === 4 && namespace[1] === 'client' && namespace[3] === 'source') {
      stateSourceAlias = trackAlias;
      for (const w of stateAliasWaiters.splice(0)) w();
    }
    if (namespace.length === 4 && namespace[1] === 'entity' && namespace[3] === 'source') {
      const entityId = namespace[2]!;
      entityAliases.set(entityId, trackAlias);
      const waiters = entityAliasWaiters.get(entityId);
      if (waiters) {
        entityAliasWaiters.delete(entityId);
        for (const w of waiters) w();
      }
    }
    post({ type: 'incomingSubscribe', namespace, trackAlias });
  });

  await session.initialize(MoqRole.PUBSUB, undefined, 100, [connectionConfigParam(cmd.config)]);
  session.startMessageLoop();
  startDatagramLoop();
  startSubgroupAcceptLoop();
}

/** Read datagrams and forward each (step 2: no audio divert yet). */
function startDatagramLoop(): void {
  const readable = conn!.takeDatagramReadableForWorker();
  if (!readable) throw new Error('no datagram readable (not connected)');
  const reader = readable.getReader();
  void (async () => {
    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) {
          post({ type: 'notice', event: 'datagram-reader-done' });
          return;
        }
        if (!value) continue;
        let parsed;
        try {
          parsed = parseObjectDatagram(value);
        } catch {
          continue; // malformed — skip
        }
        // Diverted sink alias: decode here, straight into the SAB ring —
        // this datagram never touches the main thread.
        const sink = sinkTracks.get(parsed.trackAlias);
        if (sink) {
          decodeSinkDatagram(parsed.trackAlias, sink, parsed.groupId, parsed.payload);
          continue;
        }
        // The payload aliases the received datagram's buffer; no other
        // consumer exists, so transfer the whole buffer with it.
        post(
          {
            type: 'datagram',
            trackAlias: parsed.trackAlias,
            groupId: parsed.groupId,
            objectId: parsed.objectId,
            payload: parsed.payload,
          },
          [value.buffer]
        );
      }
    } catch (e) {
      post({ type: 'notice', event: 'datagram-reader-error', detail: String(e) });
    }
  })();
}

/** Accept incoming uni streams; forward every subgroup object to main. */
function startSubgroupAcceptLoop(): void {
  const transport = conn!.getTransport()!;
  void (async () => {
    const streams = transport.incomingUnidirectionalStreams.getReader();
    for (;;) {
      const r = await streams.read().catch(() => ({ done: true as const, value: undefined }));
      if (r.done) {
        post({ type: 'notice', event: 'subgroup-accept-done' });
        return;
      }
      void readSubgroupStream(
        r.value as ReadableStream<Uint8Array>,
        () => {},
        (h, o) => {
          // readSubgroupStream hands us a fresh copy per object — transferable.
          post(
            {
              type: 'subgroupObject',
              trackAlias: h.trackAlias,
              groupId: h.groupId,
              subgroupId: h.subgroupId,
              objectId: o.objectId,
              payload: o.payload,
            },
            [o.payload.buffer]
          );
        }
      ).catch((e) => {
        post({ type: 'notice', event: 'subgroup-stream-error', detail: String(e) });
      });
    }
  })();
}

async function handleSubscribe(cmd: Extract<MainToWorker, { type: 'subscribe' }>): Promise<void> {
  if (!session) throw new Error('not connected');
  const subscribeId = await session.subscribe(
    cmd.namespace,
    cmd.trackName,
    undefined,
    undefined,
    cmd.extraParams
  );
  const trackAlias = session.getTrackAlias(subscribeId);
  if (trackAlias === undefined) throw new Error('no track alias after SUBSCRIBE_OK');
  // Forward this instance's end; the session's handler is one-shot, and
  // main's proxy keys its own dispatch by subscribeId.
  session.onPublishDone(subscribeId, (statusCode, reason) => {
    post({ type: 'publishDone', subscribeId, statusCode, reason });
  });
  post({ type: 'subscribed', id: cmd.id, subscribeId, trackAlias });
}

/** Feed one released frame to the decoder at its positional timestamp. */
function feedDecoder(trackAlias: number, sink: SinkTrack, seq: bigint, data: Uint8Array): void {
  try {
    sink.decoder.decode(
      new EncodedAudioChunk({
        type: 'key', // Opus frames are always key frames
        timestamp: Number(seq) * sink.frameDurationUs,
        data,
      })
    );
  } catch (e) {
    sink.decodeErrors++;
    post({ type: 'notice', event: 'decode-error', detail: `alias ${trackAlias}: ${String(e)}` });
  }
}

/** Parse one sink datagram and feed its Opus payload to the decoder —
 * through the FEC reassembler when redundancy is declared. */
function decodeSinkDatagram(trackAlias: number, sink: SinkTrack, seq: bigint, payload: Uint8Array): void {
  let pkt;
  try {
    pkt = parseSink(payload);
  } catch {
    return; // discard rule: malformed/unknown-flag packets are dropped
  }
  if (pkt.audio.length === 0) return;
  // Ingress health: the sink stream is gapless-seq by contract, so any
  // forward jump is lost audio and any backward arrival is
  // reorder/lateness. Counted pre-repair — this is network truth; the
  // reassembler's counters report the repair outcome.
  sink.received++;
  if (sink.lastSeq !== null) {
    if (seq > sink.lastSeq + 1n) {
      sink.gapEvents++;
      sink.lostFrames += Number(seq - sink.lastSeq - 1n);
    } else if (seq <= sink.lastSeq) {
      sink.reordered++;
    }
  }
  if (sink.lastSeq === null || seq > sink.lastSeq) sink.lastSeq = seq;
  if (sink.reasm) {
    const red = pkt.redundancy;
    for (const e of sink.reasm.push(seq, pkt.audio, red?.offset ?? 0, red?.audio ?? null)) {
      if (e.audio === null) {
        sink.pendingConceal.push(e.seq); // written in the output callback, in order
      } else {
        feedDecoder(trackAlias, sink, e.seq, e.audio);
      }
    }
    return;
  }
  feedDecoder(trackAlias, sink, seq, pkt.audio);
}

function handleSetSinkTrack(cmd: SetSinkTrackCommand): void {
  clearSinkTrack(cmd.trackAlias);
  // Writer-view core over the SAME shared ring the worklet reads (its
  // capacity is derived from the identical config, so geometry matches).
  const jbuf = new JitterBufferCore({
    ...cmd.jbufConfig,
    sharedStorage: cmd.sharedStorage,
    sharedWritePos: cmd.sharedWritePos,
  });
  const track: SinkTrack = {
    jbuf,
    frameDurationUs: cmd.decoderConfig.frameDurationUs,
    formatKey: '',
    reasm: (cmd.redundancy ?? 0) > 0 ? new SinkReassembler(cmd.redundancy!) : null,
    pendingConceal: [],
    lastSeq: null,
    received: 0,
    gapEvents: 0,
    lostFrames: 0,
    reordered: 0,
    decodeErrors: 0,
    ingressTimer: setInterval(() => {
      const t = sinkTracks.get(cmd.trackAlias);
      if (!t) return;
      post({
        type: 'sinkIngress',
        trackAlias: cmd.trackAlias,
        received: t.received,
        gapEvents: t.gapEvents,
        lostFrames: t.lostFrames,
        reordered: t.reordered,
        decodeErrors: t.decodeErrors,
        fecRepaired: t.reasm?.repaired ?? 0,
        fecConcealed: t.reasm?.concealed ?? 0,
        fecSkipped: t.reasm?.skipped ?? 0,
      });
    }, 2000),
    decoder: new AudioDecoder({
      output: (audioData: AudioData) => {
        try {
          const frames = audioData.numberOfFrames;
          const channels = audioData.numberOfChannels;
          const need = frames * channels;
          // Concealment frames due before this output flush first:
          // zero-content frames for positions the reassembler gave up
          // on (F5). Sequenced here — decode is async, so writing them
          // at push time would land them ahead of outputs in flight.
          if (track.pendingConceal.length > 0) {
            const outSeq = BigInt(Math.round(audioData.timestamp / track.frameDurationUs));
            while (track.pendingConceal.length > 0 && track.pendingConceal[0]! < outSeq) {
              track.pendingConceal.shift();
              if (zeroPcm.length < need) zeroPcm = new Float32Array(need);
              jbuf.write(zeroPcm.subarray(0, need));
            }
          }
          if (decodePcm.length < need) decodePcm = new Float32Array(need); // grow only
          const pcm = decodePcm.subarray(0, need);
          copyDecoded(audioData, pcm, frames, channels);
          jbuf.write(pcm); // straight into the SAB ring — no postMessage, no per-frame alloc

          // Format watch (emits on change only): decode-layer mono
          // collapse / sample-rate surprises show up right here.
          const fmtKey = `${channels}|${audioData.sampleRate}|${audioData.format ?? '?'}|${copyPath}`;
          if (fmtKey !== track.formatKey) {
            track.formatKey = fmtKey;
            post({
              type: 'decodedFormat',
              trackAlias: cmd.trackAlias,
              numberOfChannels: channels,
              sampleRate: audioData.sampleRate,
              nativeFormat: audioData.format ?? null,
              copyPath: copyPath!,
            });
          }
        } catch (e) {
          post({ type: 'notice', event: 'decode-error', detail: String(e) });
        } finally {
          audioData.close();
        }
      },
      error: (e: DOMException) => post({ type: 'notice', event: 'decode-error', detail: String(e) }),
    }),
  };
  track.decoder.configure({
    codec: cmd.decoderConfig.codec,
    sampleRate: cmd.decoderConfig.sampleRate,
    numberOfChannels: cmd.decoderConfig.numberOfChannels,
    // Real-time hint: don't batch input chunks before emitting output.
    optimizeForLatency: true,
  } as AudioDecoderConfig & { optimizeForLatency: boolean });
  sinkTracks.set(cmd.trackAlias, track);
}

function clearSinkTrack(trackAlias: number): void {
  const track = sinkTracks.get(trackAlias);
  if (!track) return;
  sinkTracks.delete(trackAlias);
  clearInterval(track.ingressTimer);
  try {
    track.decoder.close();
  } catch {
    // Already closed/errored.
  }
}

/**
 * Start one entity's capture: consumer ring over the shared cells, real
 * WebCodecs Opus encoder (injected into the unit-tested CaptureEncoder),
 * clock-free pump on the worklet's Atomics wake signal (the old SDK's
 * proven §6.1 loop — falls back to a ~2 ms poll if waitAsync is absent).
 */
async function handleSetCaptureTrack(cmd: SetCaptureTrackCommand): Promise<void> {
  if (!conn) throw new Error('not connected');
  await stopCapture(cmd.entityId); // replace any prior pipeline for this entity
  const trackAlias = await waitForEntityAlias(cmd.entityId);
  const sender = conn;
  const ring = new CaptureRing({
    numChannels: cmd.numChannels,
    capacityFrames: cmd.capacityFrames,
    sharedStorage: cmd.sharedStorage,
    sharedWritePos: cmd.sharedWritePos,
    sharedReadPos: cmd.sharedReadPos,
  });
  const ec = cmd.encoderConfig;
  const encoder = new CaptureEncoder({
    ring,
    trackAlias,
    sampleRate: ec.sampleRate,
    numChannels: ec.numberOfChannels,
    poseCell: new PoseCell({ seq: cmd.poseSeq, values: cmd.poseValues }),
    // Inject the real WebCodecs encoder + AudioData (kept out of
    // CaptureEncoder so its logic stays unit-testable).
    makeEncoder: (onChunk: (chunk: EncodedChunkLike) => void): FrameEncoder => {
      const audioEncoder = new AudioEncoder({
        output: (chunk) => onChunk(chunk),
        error: (e) => post({ type: 'notice', event: 'encode-error', detail: String(e) }),
      });
      audioEncoder.configure({
        codec: ec.codec,
        sampleRate: ec.sampleRate,
        numberOfChannels: ec.numberOfChannels,
        bitrate: ec.bitrate,
        opus: { frameDuration: ec.frameDurationUs },
      } as AudioEncoderConfig);
      return {
        encode: (samples, frames, timestampUs) => {
          const audioData = new AudioData({
            format: 'f32',
            sampleRate: ec.sampleRate,
            numberOfFrames: frames,
            numberOfChannels: ec.numberOfChannels,
            timestamp: timestampUs,
            // AudioData copies synchronously, so the scratch is
            // reusable right after (cast past ArrayBufferLike strictness).
            data: samples as unknown as BufferSource,
          });
          try {
            audioEncoder.encode(audioData);
          } finally {
            audioData.close();
          }
        },
        flush: () => audioEncoder.flush(),
        close: () => {
          if (audioEncoder.state !== 'closed') audioEncoder.close();
        },
      };
    },
    send: (bytes) => {
      void sender.sendDatagram(bytes).catch(() => {});
    },
  });

  const capture: Capture = { encoder, signal: cmd.sharedSignal, running: true };
  captures.set(cmd.entityId, capture);
  startCaptureLoop(capture);
}

/**
 * Drain + encode on each wake. The encode work is sub-ms and the loop
 * spends its time awaiting, so the other async loops on this event loop
 * are serviced freely between wakes.
 */
function startCaptureLoop(capture: Capture): void {
  const signal = capture.signal;
  const enc = capture.encoder;
  const waitAsync = (Atomics as unknown as {
    waitAsync?: (ta: Int32Array, i: number, v: number) => { async: boolean; value: Promise<string> };
  }).waitAsync;
  void (async () => {
    let seen = Atomics.load(signal, 0);
    while (capture.running) {
      enc.pump(); // drain everything available → encode (Opus does the framing)
      if (waitAsync) {
        const r = waitAsync(signal, 0, seen);
        if (r.async) await r.value;
      } else {
        await new Promise((res) => setTimeout(res, 2)); // fallback poll (~2 ms)
      }
      seen = Atomics.load(signal, 0);
    }
  })();
}

/** Stop one entity's capture loop and flush+close its encoder. Idempotent. */
async function stopCapture(entityId: string): Promise<void> {
  const capture = captures.get(entityId);
  if (!capture) return;
  captures.delete(entityId);
  capture.running = false;
  // Wake the loop so it observes running=false and exits promptly.
  Atomics.add(capture.signal, 0, 1);
  Atomics.notify(capture.signal, 0, 1);
  await capture.encoder.stop();
}

async function handleWriteState(cmd: Extract<MainToWorker, { type: 'writeState' }>): Promise<void> {
  if (!conn) throw new Error('not connected');
  if (stateSourceAlias === null) {
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(
        () => reject(new Error('server did not subscribe the client state source in 5000ms')),
        5000
      );
      stateAliasWaiters.push(() => {
        clearTimeout(timer);
        resolve();
      });
    });
  }
  if (!stateWriter) {
    stateWriter = new SubgroupWriter(await conn.createSendStream(), stateSourceAlias!, 0n, 0n, 0);
  }
  for (const payload of cmd.payloads) {
    await stateWriter.writeObject(stateObjId++, payload);
  }
}

async function handleClose(): Promise<void> {
  for (const entityId of [...captures.keys()]) await stopCapture(entityId);
  for (const alias of [...sinkTracks.keys()]) clearSinkTrack(alias);
  if (stateWriter) {
    try {
      await stateWriter.close();
    } catch {
      // Transport may already be gone.
    }
    stateWriter = null;
  }
  if (session) {
    await session.close();
    session = null;
  }
  if (conn) {
    conn.close();
    conn = null;
  }
  stateSourceAlias = null;
  stateAliasWaiters = [];
}

ctx.onmessage = (e: MessageEvent) => {
  const cmd = e.data as MainToWorker;
  switch (cmd.type) {
    case 'connect':
      handleConnect(cmd).then(
        () => post({ type: 'connected', id: cmd.id }),
        async (err) => {
          // A rejected CLIENT_SETUP surfaces as a session close whose
          // code/reason may land a beat after the SERVER_SETUP read
          // fails — give the closed promise a moment, then attach what
          // we have (lasa-core.md §4.6: correctness rides the code).
          for (let i = 0; i < 10 && lastCloseInfo === null; i++) {
            await new Promise((r) => setTimeout(r, 50));
          }
          post({
            type: 'fail',
            id: cmd.id,
            message: String(err),
            closeCode: lastCloseInfo?.closeCode,
            closeReason: lastCloseInfo?.reason,
          });
        }
      );
      break;
    case 'subscribe':
      handleSubscribe(cmd).catch((err) => post({ type: 'fail', id: cmd.id, message: String(err) }));
      break;
    case 'sendDatagram':
      if (conn) {
        conn.sendDatagram(cmd.bytes).catch((err) => log('sendDatagram failed:', err));
      }
      break;
    case 'writeState':
      handleWriteState(cmd).then(
        () => post({ type: 'stateWritten', id: cmd.id }),
        (err) => post({ type: 'fail', id: cmd.id, message: String(err) })
      );
      break;
    case 'setSinkTrack':
      try {
        handleSetSinkTrack(cmd);
        post({ type: 'sinkTrackSet', id: cmd.id });
      } catch (err) {
        post({ type: 'fail', id: cmd.id, message: String(err) });
      }
      break;
    case 'clearSinkTrack':
      clearSinkTrack(cmd.trackAlias);
      post({ type: 'sinkTrackCleared', id: cmd.id });
      break;
    case 'setCaptureTrack':
      handleSetCaptureTrack(cmd).then(
        () => post({ type: 'captureTrackSet', id: cmd.id }),
        (err) => post({ type: 'fail', id: cmd.id, message: String(err) })
      );
      break;
    case 'stopCapture':
      stopCapture(cmd.entityId).then(
        () => post({ type: 'captureStopped', id: cmd.id }),
        (err) => post({ type: 'fail', id: cmd.id, message: String(err) })
      );
      break;
    case 'close':
      handleClose().then(
        () => post({ type: 'closed', id: cmd.id }),
        (err) => post({ type: 'fail', id: cmd.id, message: String(err) })
      );
      break;
  }
};
