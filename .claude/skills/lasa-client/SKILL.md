---
name: lasa-client
description: Build a browser app on @panaudia/lasa-client (LASA live spatial audio over MoQ/WebTransport) — connecting, capture, binaural playout, presence, state and the base profile. Use when writing or debugging code that uses the TypeScript client, or when asked how the LASA protocol behaves from a client's point of view.
---

Help the user build against `@panaudia/lasa-client`, the TypeScript
client in `typescript/`. Use the verified sources in this repository as
the primary reference and adapt from them rather than generating from
memory: the API is small and specific, and most mistakes are one of the
"must get right" items below.

## Key files to consult

- `typescript/README.md` — install, requirements, the first-session
  example, guide index.
- `typescript/docs/` — one guide per subsystem: `connecting.md`,
  `audio.md`, `presence.md`, `state.md`, `profile.md`,
  `coordinates.md`. Defer to these for detail.
- `typescript/src/index.ts` — the exported surface, with TSDoc on every
  declaration. If a symbol is not exported there, it is not public.
- `typescript/examples/minimal/src/main.ts` — the smallest complete
  client (connect, capture, playout, roster with names, teardown), ~120
  lines. Start new code from this shape.
- `typescript/examples/test-page/src/app.ts` — every subsystem on one
  page, including multi-entity, pose dragging, state writes and
  diagnostics. Consult for anything the minimal example lacks.
- `lasa-core.md` (protocol) and `lasa-base-profile.md` (the `lasa.`
  vocabulary, roles, channels, moderation) — for "why" questions and
  anything the client docs leave to the spec.

## Essential details code MUST get right

- **Cross-origin isolation.** Capture and playout move audio between
  threads through `SharedArrayBuffer`, so the page needs
  `Cross-Origin-Opener-Policy: same-origin` and
  `Cross-Origin-Embedder-Policy: require-corp`. Without them
  `startCapture`/`playSink` throw `NOT_ISOLATED`. Vite dev servers set
  them in `server.headers`; production hosts must send them too.
- **48 kHz capture context.** The wire format is Opus at 48 kHz in 5 ms
  frames. Build the capture context as
  `new AudioContext({ sampleRate: 48000 })`; any other rate is refused
  with a `SAMPLE_RATE` error. Frame duration is not an option.
  `playSink` creates its own 48 kHz context.
- **User gesture.** Browsers gate audio output behind a gesture: call
  `playSink` (and create the capture context) from a click or key
  handler, never on page load.
- **Entities are declared at connect.** The entity set is fixed per
  connection (`entities` for ad-hoc definitions, `setups` to
  instantiate ticket-defined ones). To add an entity, reconnect.
- **Identifiers** (space, client, entity, channel, role, attr names) are
  `[a-z0-9-]{1,128}` — lowercase, no dots or underscores. UUIDs are fine
  but not required.
- **Ticketed XOR open space.** A ticketed space requires a ticket whose
  `client_id` equals `clientId` and whose `aud` equals `spaceId`;
  presenting a ticket to an open space is refused (`MODE_MISMATCH`).
  Ad-hoc entities in a ticketed space need the `presenter` or `admin`
  role. Ticket minting is out of the client (see `tickets/`).
- **Dev certificates.** A development server prints
  `serverCertificateHashes (sha-256, base64): …` at boot; pass it as
  `serverCertificateHashBase64`. Omit it against a real certificate.
- **Coordinates.** Positions are metres, right-handed, **x forward, y
  left, z up**; rotations are radians (yaw anticlockwise from +x,
  pitch up, roll anticlockwise around the line of sight). Do not feed
  Y-up graphics coordinates in directly: use the converters
  (`threejsToLasa`, `babylonToLasa`, `unityToLasa`, `unrealToLasa`,
  `aframeToLasa`, `playcanvasToLasa`, `pixiToLasa` and their inverses).
- **Poses ride on audio.** `capture.setPose(pose)` is wait-free and may
  be called at any rate; the freshest pose is stamped on the next
  frame. Presence is how everyone else's poses arrive.
- **State is not persisted across page loads.** `Store.create(prefixes)`
  starts empty; a fresh page takes a snapshot. Reuse the same `Store`
  across reconnects within a page to resume on a delta.
- **Read after `await sync.settled()`.** Store application is
  asynchronous; a write is not applied locally, it is echoed by the
  server.
- **Use the base-profile helpers, not raw keys.** Personal mutes and
  solos are `me.mute(other)` / `me.solo(other)` on the entity's
  publisher; moderation, channels and rendering are `client.space.*`;
  names and memberships are read through `baseView(store)`. Raw
  `writeState` is for an application's own prefixes.
- **Permission is the server's.** A write the caller's role does not
  allow is silently dropped; do not offer UI for actions the role
  forbids.
- **Teardown.** `client.close()` stops every capture and sink, ends
  state sync and tears down the worker; close the capture
  `AudioContext` yourself. `onClosed` fires for a session that ends
  after a completed handshake, with `closeCode` `BLOCKED`,
  `SUPERSEDED` or `SHUTTING_DOWN` when the close was deliberate; a
  rejected handshake throws `LasaRejectionError` from `connect`
  instead.

## Tasks, and where the verified code is

| Task | Read |
|---|---|
| Join a space, talk, listen, list who is there | `examples/minimal/src/main.ts` |
| Tickets, roles, rejection handling | `docs/connecting.md`; rejection table with `LASA_ERR` |
| Microphone selection, stereo/DAW capture (one entity per channel), quality/redundancy | `docs/audio.md`; `selectBestMicrophone` |
| Ambisonic sinks (`ambi2`/`ambi3`) and the raw packet API | `docs/audio.md`, "The raw packet API" |
| Presence roster, loudness, head-frame entities | `docs/presence.md`; `RosterEntry` |
| Application state, groups, cursors | `docs/state.md` |
| Mute/solo, blocks/kicks, channels, gain/attenuation/size/directivity, dof | `docs/profile.md`; `EntityControls`, `SpaceControls`, `BaseView` |
| Converting poses from a game engine or 2D canvas | `docs/coordinates.md` |
| Multi-entity, pose dragging, diagnostics panels | `examples/test-page/src/app.ts` |
| Running a space locally to test against | `../panaudia-lasa/README.md` (sibling checkout): `PANAUDIA_ALLOW_UNTICKETED=true go run .` in `panaudia-lasa/server`, or the Docker image |

## Diagnosing problems

- `NOT_ISOLATED` → the page is not cross-origin isolated (headers, or
  a plain-HTTP non-localhost origin, which is not a secure context).
- `SAMPLE_RATE` → the capture `AudioContext` is not 48 kHz.
- `LasaRejectionError` from `connect` → read `code` against
  `LASA_ERR`; `INVALID_TICKET` and `CLIENT_ID_MISMATCH` are ticket
  problems, `MODE_MISMATCH` is ticketed-vs-open.
- `entity()` times out → the server never subscribed that entity's
  source: the id was not in the Connection Config, or admission
  failed for it.
- Silence on a sink → check `client.getSinkIngress(id)` (network side)
  and `player.getJitterStats()` (buffer side); a gap event is the
  network's, an underrun is the buffer's. Check channel membership:
  a source and a sink hear each other only if they share an unmuted
  channel (`baseView(store).entity(id).heardIn/hears`).
- Wrong direction / mirrored scene → a coordinate-system mismatch;
  use the converters, and remember y is left, not right.

## Working in this repository

- `typescript/`: `npm test` (vitest), `npm run typecheck`, `npm run
  build` (needed before `examples/minimal` can resolve the package),
  `npm run docs:api` after changing the exported surface.
- Wire codecs are ports of the Go reference (`wire/`, `state/`); a
  behavioural change lands in Go first and the TS test vectors are
  regenerated from it (`typescript/tests/vectors/`).
- The Go client (`client/`) is the reference for the protocol side;
  the browser client mirrors it.
