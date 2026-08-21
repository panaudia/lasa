# Provenance

`@panaudia/lasa-client` — the LASA TypeScript SDK. A SIBLING of
`sdks/typescript` (the old-panaudia SDK), importing nothing from it:
the portable core is **copied** here, succession-style, exactly as
`panaudia-engine` copied the DSP Layer 1 (decision 2026-08-05; plan:
`lasa/plan/panaudia-server-build.md` P6).

- **Source:** `sdks/typescript` at repo commit
  `78ac685e703b4997f0c80695d7131a4abad6eed8` (2026-06-17).
- **Copied 2026-08-05 → `src/transport/`** (from `src/moq/`, verbatim
  at copy time): `moq-transport.ts`, `connection.ts`, `session.ts`,
  `datagram-router.ts`, `types.ts`, `errors.ts`, `webtransport.d.ts`.
  Rationale: the codec targets the wire format of
  `github.com/Eyevinn/moqtransport` — the direct ancestor of the
  `github.com/panaudia/moqtransport` fork the LASA server embeds — so
  it is the shortest interop path (decision: adapt this codec; the
  in-tree `moqtail` draft-16 TS stack is the interop cross-check
  reference, not a dependency).
- **Copied 2026-08-05 → `src/audio/`** (from `src/moq/` at repo commit
  `824872c64ceeef76ad1a5ee2a3bc49b77e652b34`, with their unit tests →
  `tests/`): `jitter-buffer-core.ts`, `capture-ring.ts`,
  `stereo-meter-core.ts`, `capture-worklet.ts` (all verbatim) and
  `playout-worklet.ts` (SAB-only — divergence 5).
- **Copied 2026-08-05 → `src/audio/microphone-selection.ts`** (from
  `src/shared/microphone-selection.ts` at the same commit, verbatim):
  the Bluetooth-avoiding automatic mic selection — label heuristics +
  brand lists, ≤16 kHz sample-rate probe (HFP tell), USB > builtin >
  unknown > bluetooth ranking, and the permission-aware flow that
  never opens the default mic just to read labels (opening a
  Bluetooth mic flips the whole device into mono HFP). Known
  accepted trade carried unchanged at copy time: probing an
  unlabelled unknown briefly opens it, which on a mislabelled
  Bluetooth device would itself trigger the flip.
  **Diverged 2026-08-06 (HFP fixes):** the module
  now NEVER opens a device — the permission-priming open and the
  unknown-device sample-rate probe are both removed. Without
  permission it returns `permissionPending` and the caller opens its
  production stream on the default, then re-runs selection and
  switches away if that default proved Bluetooth (first-grant
  recovery, implemented in the test page). Post-hoc HFP detection
  restored via `probeOutputDeviceSampleRate` (ported into
  `player.ts` from the old `audio-player.ts`) + the page banner —
  matching the old client's 2026-06-11 doctrine that selection
  flows must not open devices and detection is post-hoc.
  The audio-machinery
  port (D3); review verdict: the TS jitter core is the same v3
  algorithm as `panaudia-engine/buffers/jitter_buffer.go`, and its
  browser-specific tuning (deepened L/H minimums, windowReads 750) is
  the deliberate 2026-06-10 reader-burst retune, kept as-is.
- Further portable-core copies (coordinates, verify gates, …) will be
  recorded here as they arrive.

## Divergences from the copy

1. (D2, 2026-08-05) `moq-transport.ts`: `buildClientSetup` accepts
   arbitrary extra setup KVPs — carries LASA's `0x1A5A1` Connection
   Config parameter (`lasa-core.md` §4.1); `session.ts`:
   `initialize` forwards them.
2. (D3, 2026-08-05) `session.ts`: single-reader dispatch. The old
   client did all its subscribes before starting the message loop, so
   `waitForMessage` could read the control stream inline. The LASA
   client keeps the loop running for the whole session (the server
   may subscribe our entity tracks at any time — multi-entity
   contract), so two concurrent readers would steal each other's
   messages; once the loop runs, response waiters register with a
   pending-dispatch table the loop fulfils, and the loop rejects all
   waiters when the stream ends.
3. (D3 state client, 2026-08-06) `moq-transport.ts` /
   `session.ts` / `types.ts`: `buildSubscribe` accepts arbitrary
   extra subscribe KVPs (`MoqSubscription.extraParams`,
   `session.subscribe(..., extraParams)`) — carries LASA's `0x1A5A3`
   state subscribe message (`lasa-core.md` §6.7). The subscribe twin
   of divergence 1.
4. (D3 state client, 2026-08-06) `session.ts`: PUBLISH_DONE (0x0B)
   dispatch. The old client never handled a server ending a
   subscription instance; LASA's state plane terminates natively with
   PUBLISH_DONE too-far-behind (0x06) as the re-subscribe cue.
   `onPublishDone(subscribeId, handler)` registers a one-shot handler
   the message loop fulfils.
5. (D3 audio machinery, 2026-08-05) `src/audio/playout-worklet.ts`:
   SAB-only. This SDK hard-requires cross-origin isolation, so the old
   postMessage-PCM and transferred-pcmPort fallbacks are removed: the
   processor throws unless `config` carries `sharedStorage` +
   `sharedWritePos`, and its `port` is outbound-only (stats). The test
   feeds PCM through a writer-view `JitterBufferCore` over the shared
   ring — the MOQ worker's real topology.
6. (jitter debug, 2026-08-05) `src/audio/playout-worklet.ts` +
   `src/audio/capture-worklet.ts`: discontinuity instrumentation. The
   playout processor posts an `event` message the moment a counter
   increments (underrun/overrun/lap/insert/drop, with fill) — the old
   SDK only window-sampled cumulative counters; the capture processor
   posts ring-overflow changes on its (previously unused) port. Both
   post only on change — silent on a healthy stream.

## New files beside the copy (not divergences)

- `src/transport/subgroups.ts` (D3 state client, 2026-08-06): subgroup
  stream reader/writer + per-alias router for the draft-16
  stream-per-subgroup data path (type family 0x10–0x1D, +0x20
  default-priority bit). The old SDK was datagram-only. Buffers
  objects for unclaimed aliases: the server opens state lanes and
  writes the catch-up immediately after accepting the subscribe,
  racing the client's processing of SUBSCRIBE_OK (the subgroup twin
  of `datagram-router.ts`'s race buffer).
- `src/lasa/statewire.ts`, `src/lasa/store.ts`, `src/lasa/state.ts`
  (D3 state client, 2026-08-06): ports of Go `lasa/wire`
  (state messages + subscribe params, golden-vector verified) and
  `lasa/state/clientstore` (store rules ported test-for-test), plus
  the StateSync subscribe/apply/re-subscribe loop mirroring Go
  `lasa/client.SyncState`.
- `src/worker/protocol.ts`, `src/worker/moq-worker.ts`,
  `src/worker/worker-transport.ts` (D3 audio machinery step 2,
  2026-08-05): the worker transport — the old SDK's proven
  `moq-worker` 3-thread architecture (design:
  `cloud-mixer/plan/history/browser-audio/worker-transport-design.md`)
  rebuilt for LASA rather than copied. The worker owns WebTransport +
  MoqSession + the datagram read loop + the subgroup accept loop and
  the client state-source SubgroupWriter; main keeps the facade, the
  state store/StateSync (fed by forwarded subgroup objects through
  `SubgroupRouter.ingest`), the `DatagramRouter`, and the publishers
  (main frames, worker writes). Rationale: transferring only
  `datagrams.readable` to a worker was tried in the old SDK and found
  insufficient — the stream's source stays in the main realm, so
  delivery remains main-gated (worker-transport-design.md preamble).
- `src/audio/capture-encoder.ts`, `src/audio/capture.ts`,
  `src/audio/pose-cell.ts` (D3 audio machinery step 4, 2026-08-05):
  the per-entity send path. `capture-encoder.ts` descends from the old
  SDK's `capture-encoder.ts` with LASA framing — each Opus packet
  becomes a `MonoObjectPacket` (pose + audio) sent as an
  OBJECT_DATAGRAM whose groupId is the gapless sequence (§5.2), not
  the old timestamp-ms/objectId scheme; the pose is read from the
  shared `PoseCell` at datagram-framing time (the head-rotation
  freshness rule — the pose on a packet is deliberately NEWER than
  the audio in it). `pose-cell.ts` is new: a seqlock over a 56-byte
  SAB, main writes / worker reads, no torn reads. `capture.ts`
  (CapturePipeline) replaces the old singleton `AudioPublisher` with
  per-entity mono pipelines fed from any AudioNode via a
  ChannelSplitter (mic = N=1, stereo playback = 2, DAW = N); the
  caller owns the AudioContext. The worker's capture pump is the old
  `moq-worker.ts` §6.1 `Atomics.waitAsync` loop, per entity.
  `SinkPlayer`, descended from the old SDK's `audio-player.ts` but
  SAB-only and decode-free — WebCodecs decode lives in the worker
  (`setSinkTrack` diverts the sink alias; parses `SinkPacket`, feeds
  Opus to an `AudioDecoder`, writes a writer-view `JitterBufferCore`
  over the shared ring). Ported from the old worker: the
  `AudioData.copyTo` f32/f32-planar engine probe (Firefox rejects
  interleaved) and the decoded-format watch (emits on change; decode-
  layer mono collapse shows there). Deliberately NOT ported: the
  playout-drift investigation probes (gap-stat buckets, one-shot
  60 s clock probe) — tools of a solved investigation; re-port from
  `sdks/typescript/src/moq/moq-worker.ts` if drift ever resurfaces.
