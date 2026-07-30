# Panaudia integration plan (step 6) — NOT TAKEN

*Drafted 2026-07-29 for review; **superseded the same day** by the
greenfield rebuild decided in `greenfield-standalone.md` (new repos
`panaudia-engine` + `panaudia-server`; existing panaudia left frozen,
no integration, no bridging). Kept as the priced record of the
alternative: the survey findings below (funnel contract, incumbent
front end, cloud-mixer constraints) remain accurate and reusable, and
each decision D1–D10's fate under the chosen path is tabulated in
`greenfield-standalone.md`.*

*Original framing: the phased plan for implementation-plan.md step 6 —
a new panaudia front end implementing the LASA server SPI
(`server-spi.md`), running beside `core/moq`. Organised around the
awkward choices, each written as an open decision (D1–D10) with its
options priced. Grounded in a survey of `panaudia`
(`core/panaudia_server`, `direct/`, `core/moq`, `core/sessions`,
`main.go`) and `../cloud-mixer` (architecture docs, doc-check script,
gateway imports) done the same day.*

## Ground truth the plan builds on

Facts established by the survey; file references are into the panaudia
repo unless said otherwise.

- **The embedder contract.** A front end obtains one
  `panaudia_server.ConnectionHandler` per node via
  `NewConnectionHandlerE(backend, nodeConfig, outputTrack, live,
  transport)` (`core/panaudia_server/backend.go:98`). Admission runs
  `sessions.Registry.Admit` (evict-old, may block ~2–4 s), then builds
  jitter buffer + codecs + `BouncerClient` and registers the node.
  Distinct failures: `SERVER_ERROR_DUPLICATE` / `SERVER_ERROR_FULL`.
- **Teardown is the funnel, only the funnel.** Every removal cause
  severs transport via `LiveSession.Kill`; the owner goroutine observes
  the close and calls `connectionHandler.Stop()` →
  `DepartNode` (`direct/depart.go:47`). No ad-hoc cleanup, no
  `stopOnce`, no direct node removal
  (`../cloud-mixer/architecture/components/session-lifecycle.md`).
  Encoder cgo free happens later on the mixer goroutine
  (`pendingFree`/`FreeSource`) — never on the transport goroutine.
- **Ingress.** `ConnectionHandler.WriteOpus(src []byte)` takes one raw
  Opus packet and decodes it *on the calling goroutine* (cgo), feeds
  loudness (`SetVolume`) and the per-connection jitter buffer. One
  writer goroutine per connection; the decoder is not concurrency-safe.
  Existing MoQ jitter geometry: 20 ms writer / 5 ms reader frames.
- **Pose.** `SetPosition` takes **normalized 0..1** cube coordinates
  (scaled by `PANAUDIA_SPACE_SIZE`, default 40 m, centre `{0.5,0.5,0.5}`);
  `SetRotation` takes **degrees** (SAF `Rx(roll)·Ry(pitch)·Rz(yaw)`).
  `SetPosition` only updates BouncerClient state (broadcast on its
  100 ms tick); **`SetRotation` forwards a bouncer message immediately
  per call** (`core/space/bouncer_client.go:236`).
- **Egress.** The mixer's 5 ms loop calls
  `WriteAmbisonic` → binaural convolve + Opus encode →
  `outputTrack.WriteSample(media.Sample{Data, Duration: 5ms})` on a
  mixer worker goroutine. The returned Opus slice aliases a reused
  buffer — copy if deferring. The `TrackWriter` must be handed over at
  admission time, before any subscriber exists.
- **State/presence plane.** Mute/solo/gain/kick are statecache **ops on
  topics** (`entity`/`attributes`/`space`/`state`), authored via
  `BouncerClient`/commands, fanned out to every connection's
  `IMessageSender` writer (`MoqDataWriter` for MoQ). Load-bearing:
  entity-before-attributes ordering; pre-attach buffering so backfill
  isn't lost; connection-scoped keys swept on departure. Presence =
  entity/attrs ops + each node's 100 ms `NodeInfo3` self-publish.
- **Listener arrangement.** `core/moq/server.go` owns UDP
  `PANAUDIA_PORT` and demuxes by ALPN (`h3` → WebTransport,
  `moqt-16`/`moq-00` → MoQ); unknown ALPNs are closed. **LASA's ALPN is
  also `moqt-16`** (`lasa/server/server.go`), so ALPN cannot
  distinguish the two servers on a shared listener.
- **Cross-repo surface.** cloud-mixer imports `core/moq`
  (`apps/unified/main.go`) and `core/panaudia_server`; the draft-16
  wire output of the existing server is a pinned authority
  (`../cloud-mixer/plan/history/moq-draft14/compatibility-reference.md`),
  and both repos pin **upstream** `Eyevinn/moqtransport v0.8.2` with no
  replace. The doc-check script warns on any new panaudia package not
  mentioned in the cloud-mixer architecture docs.
- **LASA pose wire.** Metres, x forward / y left / z up; angles int16
  scaled ±π (radians), yaw anticlockwise about z (`lasa-core.md` §5).

## Decision index

| # | Phase | Question |
|---|---|---|
| D1 | 0 | How the moqtransport fork reaches panaudia's build (the `replace` does not propagate) |
| D2 | 0 | Where the front-end package lives |
| D3 | 0 | Own UDP port vs shared listener |
| D4 | 1 | Entities per connection: one (v0 deviation) or many |
| D5 | 1 | LASA string ids ↔ panaudia UUIDs |
| D6 | 1 | Ingress jitter geometry for 5 ms packets |
| D7 | 1 | Pose unit/axis mapping and rotation-forwarding cadence |
| D8 | 2 | Sink lifecycle vs always-on render |
| D9 | 2 | State-plane bridging (three sub-decisions) |
| D10 | 3 | Auth: unticketed v0, LASA tickets, kick gate |

---

## Phase 0 — module and placement prerequisites

Nothing runs until the build works. This phase has no protocol content.

### D1 — getting the fork into panaudia's build

`lasa/go.mod` currently reaches the fork via
`replace github.com/Eyevinn/moqtransport => github.com/panaudia/moqtransport …`.
**Replace directives apply only in the main module**: the moment
panaudia imports `github.com/panaudia/lasa`, the replace is ignored,
the Eyevinn pseudo-version doesn't resolve, and the build breaks.
Options:

- **(A) Rewrite the lasa module's imports to
  `github.com/panaudia/moqtransport` and drop the replace.** The fork
  becomes a real module dependency; upstream Eyevinn v0.8.2 stays
  untouched under `core/moq`; the two moqtransport copies coexist as
  distinct module paths (already proven per-instance-safe — `core/moq`
  was audited for package-global state when multi-instance tests were
  added). *Costs:* a mechanical import sweep across the lasa repo; the
  fork's commits need path-rewriting when the day comes to open
  upstream PRs (a rebase chore, denominated in one-off maintainer
  time). The fork also needs a `go.mod` module-path change + tag on its
  branch.
- **(B) Add the same replace to panaudia's (and cloud-mixer's)
  go.mod.** No lasa-repo change. *Costs:* the fork is silently swapped
  under **`core/moq` as well** — and the fork is not behaviour-neutral
  (stream objects block instead of drop, datagram buffer pooling,
  handshake changes), so the incumbent draft-16 server's runtime
  behaviour changes under the golden-vector authority; cloud-mixer must
  carry the identical replace or its unified app builds against a
  different transport than the library it wraps. Denominated in
  correctness risk to the working system and a standing two-repo
  synchronisation obligation.
- **(C) Vendor the fork into the lasa repo.** Hermetic. *Costs:*
  divergence management by hand, no `go get` upgrade path, repo bloat.

### D2 — package placement

- **(A) `core/lasa` in panaudia**, mirroring `core/moq`. Follows the
  incumbent convention; doc-check wants a `containers.md` row +
  `covers:` entry either way (warn-only until `--strict`).
- **(B) A top-level `lasafront/`** signalling its
  intended-to-outlive-`core/moq` status. Same doc obligations;
  slightly odd next to `direct/`/`unified/`.

Either way the package is panaudia-side glue only: LASA protocol code
stays in the lasa module; this package implements `server.Backend` and
wires config/env.

### D3 — listener arrangement

- **(A) Own UDP port** (`PANAUDIA_LASA_PORT`, precedent:
  `PANAUDIA_LINK_PORT`). Zero contact with the incumbent accept path,
  zero risk to the draft-16 authority or cloud-mixer's `NewMoqServer`
  use. *Costs:* deployment surface (a second UDP port through
  firewalls/load balancers; no Alt-Svc story), and the cloud-mixer doc
  rule "adds a transport → reconcile context/containers". Almost
  certainly temporary scaffolding: step 8's endgame presumably hands
  `moqt-16` on 4443 to LASA when `core/moq` is deleted.
- **(B) Share `PANAUDIA_PORT` via the existing ALPN switch.** Blocked
  in its obvious form: both servers negotiate `moqt-16`. The only
  workable variant is **a distinct LASA ALPN** (e.g. `lasa-16`) added
  to `core/moq/transport.go` NextProtos and `acceptLoop`'s switch,
  handing the accepted `quic.Conn` to `lasa/server.ServeConn`. *Costs:*
  a wire deviation from the LASA spec's own ALPN (interop with any
  non-panaudia LASA client until reverted), and edits to the
  incumbent's cross-repo accept path (`MoqServerConfig` grows a hook —
  additive, but it is the surface cloud-mixer builds against).
  Denominated in wire purity + incumbent risk, versus (A)'s ops cost.
- Demuxing by CLIENT_SETUP param (same ALPN, sniff `0x1A5A1`) is not
  practical with two separate moqtransport stacks — the handshake is
  consumed by whichever stack reads it. Recorded, not offered.

---

## Phase 1 — the audio skeleton

Goal: a LASA client joins the one `DirectSpace`, is heard by old-plane
clients and hears them, and every lifecycle path (disconnect, kick,
evict, reconciler, shutdown drain) runs through the departure funnel.
Unticketed only; state/presence integration minimal (LASA presence
covers LASA entities only; no state bridging yet).

Shape: the front-end package implements `server.Backend`. Per admitted
entity it calls `NewConnectionHandlerE(..., live, "lasa")` where `live`
is a `sessions.FuncSession` bound to the LASA connection's transport
context; the owner goroutine parks on that context and calls
`handler.Stop()` on close — nothing else. `Admit` maps onto
`sessions.Registry.Admit`'s evict-old (deviation 2 holds across front
ends: a LASA connection evicts a live WebRTC/MoQ session with the same
identity, and vice versa — see D5). `EntityHandler.WritePacket` parses
the mono-object packet zero-copy (`wire.ParseMonoObject` aliases the
payload), applies the pose fast path, and hands the Opus bytes to
`WriteOpus` — decode lands on the QUIC receive goroutine exactly as the
incumbent does it. The hot-path contract of `server-spi.md` is then
honoured end-to-end: the only additions on the ingress path are one
parse (no copy) and the decode the engine does anyway.

### D4 — entities per connection

LASA connection configs may define several entities; panaudia's whole
stack assumes one node per connection (one handler, one jitter buffer,
one output, one registry entry).

- **(A) v0 restriction: exactly one entity per connection**; configs
  with more are rejected at Admit with a clear error. Recorded as
  deviation 4 in `implementation-plan.md` with a reopen trigger (first
  real multi-entity client). *Costs:* a visible capability cut in the
  reference implementation of a spec that explicitly allows lists —
  and the moq-draft14 decision doc's one standing instruction was
  "don't pour concrete around the single-source assumption LASA will
  break". A v0 deviation is not concrete, but the reopen trigger
  should be honest that the cost of B doesn't shrink by waiting.
- **(B) N handlers per connection**, one registry entry per entity id,
  all sharing one transport `Kill`. *Costs:* shared-fate semantics —
  evicting entity A (its id claimed elsewhere) severs the transport
  and takes entity B with it, which the spec does not obviously
  intend; N× per-connection resources; the departure funnel runs N
  times per disconnect (it is per-entry, so this is legal, but the
  owner-goroutine pattern needs care). Denominated in lifecycle
  complexity now, versus (A)'s deferred rework.

Either way the **client id** has no panaudia counterpart — the
registry identity is the *entity* id. The client id matters only to
the LASA layer (state-engine cursors, one-subscription-per-client).
Recorded so nobody later "helpfully" maps client id onto the registry.

### D5 — id mapping

LASA ids are `[a-z0-9-]{1,128}` strings; panaudia identity is
`uuid.UUID` everywhere (registry, `NodeInfo3`, statecache keys).

- **(A) v0 constraint: entity ids must parse as RFC-4122 UUIDs**
  (the LASA grammar admits lowercase-hyphenated UUIDs). Identity is
  then *the same value* on both planes: cross-front-end evict-old
  works (a user reconnecting over LASA displaces their WebRTC
  session), old-plane clients see the real id, statecache keys line
  up. *Costs:* constrains config/ticket authors during migration;
  human-readable entity ids ("paul-desk") are deferred.
- **(B) Derive a UUIDv5 from the id** (namespace hash). Any id works.
  *Costs:* identity equivalence across planes breaks — the same
  logical user over WebRTC (uuid) and LASA ("paul-desk" → hash) are
  different nodes and will not evict each other; old-plane presence
  shows opaque hashes; debugging correlates via a lookup table only.
- **(C) B plus a bijective map** maintained by the front end. Only
  buys reverse translation (old-plane → LASA naming); same
  cross-plane identity break as B.

The regimes differ in when they hurt: A hurts at provisioning time
during migration, B/C hurt at runtime forever-until-step-8 in the
exact scenarios (reconnect, moderation) the funnel design cares about.

### D6 — ingress jitter geometry

LASA packets are 5 ms mono Opus; the existing MoQ branch of the jitter
config assumes 20 ms writer frames. Needed: a new `jcfg` branch
(WriterFrameSize 5 ms / ReaderFrameSize 5 ms) with a **LowInit** depth
to pick — the latency-vs-dropout knob. Not really an either/or: start
at LowInit 5 ms (one packet, matching the MoQ branch's spirit) and
treat the depth as measured tuning, not design. Redundancy (the wire
format's FEC-shift) is **ignored on receive in v0** — recovery is
embedder business per the SPI, and the jitter buffer + Opus FEC
already cover the incumbent's loss model. Recorded as future work, not
silently dropped: worth an explicit line in the front-end package doc.

### D7 — pose mapping and cadence

Three independent bites:

- **Units/axes:** LASA is metres, x-forward/y-left/z-up,
  origin-at-centre implied; panaudia is a normalized 0..1 cube scaled
  by `PANAUDIA_SPACE_SIZE` with centre `{0.5,0.5,0.5}`. Proposed
  mapping to verify, not decide here: `n = 0.5 + m/Size` per axis with
  the axis permutation checked against what the TS/WebRTC clients
  actually send (the survey did not establish panaudia's world-frame
  axis convention — investigation item I-4).
- **Angles:** LASA int16 radians (±π), yaw anticlockwise about z;
  panaudia degrees under SAF `Rx(roll)·Ry(pitch)·Rz(yaw)`. Conversion
  is trivial; **sign conventions must be verified empirically** (a
  wrong yaw sign is the classic silent spatial bug) — I-4 again.
- **Cadence:** LASA delivers a pose in *every* audio packet (200 Hz).
  `SetPosition` per packet is cheap (state update; bouncer broadcasts
  on its own 100 ms tick). **`SetRotation` per packet is not** — it
  forwards a bouncer message immediately per call, so naive wiring
  floods every connection's writer at 200 Hz per node. Options:
  - **(A) Change-detect + floor** in the front end: apply
    `SetRotation` only when the quantized angles change and not more
    often than ~50–100 ms. *Costs:* rotation used by the *encoder*
    (the thing that actually spins the source's radiation pattern)
    is also throttled, since the front end has no separate handle —
    the encoder path and the bouncer path share the one method.
  - **(B) Split the paths**: give the front end (or the handler) a
    rotation-to-encoder call distinct from rotation-to-bouncer, so the
    mix tracks 200 Hz rotation while presence stays at 100 ms.
    *Costs:* a change to `direct.ConnectionHandler`'s surface — the
    shared-interface ripple rule (any `ConnectionHandler` change is a
    cross-repo surface for cloud-mixer) applies if it touches the
    interface rather than the concrete type.
  Worth noting the incumbent's ceiling: WebRTC/MoQ clients send pose
  at ~10 Hz over the state track, so (A) loses nothing relative to
  today; (B) is what makes LASA's per-packet pose actually pay off.

### Egress wiring (no open decision)

The front end's `TrackWriter` adapter receives 5 ms encoded frames on
the mixer goroutine and calls the LASA `SinkWriter.WriteFrame(frame,
tsMicros)` synchronously — `tsMicros` synthesized from a running
sample count anchored at the first frame (media.Sample carries no
timestamp; the anchor choice only affects the wire ts cadence the LASA
shell already owns). The frame slice must be treated as
copy-on-defer (it aliases the encoder's reused buffer) — the LASA
shell's one framing copy happens synchronously, so no copy is needed
beyond it. Allocation checks on the panaudia side of both paths join
CI, mirroring the lasa repo's own.

---

## Phase 2 — state and presence integration

Goal: the mixer honours LASA clients' minimal base-profile subset
(existence, name, attrs, mute/solo), and the two populations see each
other to the degree decided below.

### D8 — sink lifecycle vs upfront TrackWriter

`NewConnectionHandlerE` wants the output `TrackWriter` at admission;
LASA's `StartSink` fires on first subscription.

- **(A) Always-render:** hand the adapter over at admission; it drops
  (or pending-buffers) frames until a LASA subscriber appears. Exactly
  the incumbent's behaviour (`MoqTrackAdapter` pending buffer).
  *Costs:* per-entity binaural render + Opus encode CPU burns whether
  or not anyone listens — denominated in mixer CPU per headless
  source.
- **(B) Subscription-gated render:** call `EnableOut` only when the
  first sink subscribes, disable when the last detaches. *Costs:* the
  disable path's existence and safety is unverified (I-1); render
  spin-up adds latency to first-listen; more lifecycle states to test.

### D9 — state-plane bridging (the big one)

Until step 8 there are two state planes that do not interoperate: the
statecache/bouncer plane (authoritative for the mixer) and the LASA
engine (authoritative for LASA clients' view). Three sub-decisions,
separable:

**D9a — LASA writes → mixer.** A LASA client's `lasa.*` mute/solo must
reach `BaseSpace`.

- **(A) Engine-as-source:** the front end registers an observer
  subscription on the LASA engine (the SPI already grants the embedder
  the `*state.Engine`) and translates the accepted-and-echoed subset
  into statecache ops via the node's bouncer path. Single authority:
  what the engine echoes is what the mixer applies. *Costs:*
  translation table + one goroutine hop (engine actor → bouncer
  dispatch); ordering care so the translated ops respect
  entity-before-attributes.
- **(B) Command-tee at ingress:** when a LASA state write maps onto a
  `core/commands` command, dispatch a `ControlMessage` alongside the
  engine write. *Costs:* two authorities that can diverge — the engine
  may accept what the command layer rejects (or vice versa), and the
  client's echo then lies about the mix. Denominated in correctness
  risk rather than code.

**D9b — old plane → LASA engine.** Do old-plane participants get
`lasa.*` entries (existence marker, name, mute state) so LASA clients
see a full roster in *state*?

- **(A) Mirror:** a front-end component subscribes to the bouncer
  fan-out (as a passive `IMessageSender`) and issues server-authored
  `Engine.Write`s for old-plane nodes, sweeping them on departure
  (tombstone `NodeInfo3{Gone:1}` / entity tombstones as the trigger).
  *Costs:* a lifecycle mirror with its own sweep correctness burden —
  the exact class of code the funnel design exists to centralise; the
  entity-before-attributes and pre-attach-buffering invariants both
  apply to the mirror's reader side.
- **(B) Don't mirror in v0:** LASA clients see LASA-only state but
  hear everyone; presence can still cover everyone (D9c). *Costs:* UX
  asymmetry during migration (an old-plane admin's mute of a LASA
  user works via D9a's reverse? — no: old-plane *writes about* LASA
  nodes travel the statecache plane, which the mixer already applies,
  so moderation works; what's missing is only LASA clients *seeing*
  old-plane roster/state). The asymmetry is cosmetic-ish but real for
  any LASA UI during migration.

**D9c — presence roster scope.** `Backend.Presence()` is polled for
the LASA presence track.

- **(A) Everyone:** the backend answers with all nodes, reading
  old-plane poses from the bouncer `state` fan-out (same passive
  subscriber as D9b-A would use) or from space node state (mechanism
  is I-2). LASA clients get a complete spatial roster from day one.
- **(B) LASA entities only in v0:** the backend answers from the pose
  state it applied itself. No new machinery; old-plane users are
  audible ghosts to LASA UIs until step 8.

Note the coupling: D9b-A and D9c-A share the passive bouncer
subscriber; choosing either pays most of the cost of both. Loudness
for `PresenceEntry` needs `BouncerClient`'s smoothed volume or a
front-end RMS of its own (I-3).

---

## Phase 3 — auth, hardening, docs

### D10 — auth

- **(A) v0 unticketed only** (`AllowUnticketed`, matching
  `PANAUDIA_UNTICKETED` default), with the **kick gate consulted in
  `Admit`** so a kicked id cannot evict a live session (the incumbent
  enforces this at auth time; the LASA front end must reach the
  `KickGate` — plumbing item). Closed-network/dev parity with today.
- **(B) LASA tickets, shared key:** verify LASA's EdDSA tickets with
  the existing `PANAUDIA_TICKET_KEY_PATH` Ed25519 key — one key, two
  claim schemas (panaudia JWTs and LASA tickets are different formats;
  no translation attempted). Needed before any non-closed deployment;
  scheduled here rather than debated — the only open question is
  whether key *reuse* vs a second key env var matters operationally.

Also in this phase: config surface (`PANAUDIA_LASA_*` env vars incl.
space id, default `"default"`; `AmbiOrder` bound to the space's
existing order 2/3 config); panaudia-side e2e tests (a
`moq_test_stub`-style harness driving the lasa reference client
against the real `DirectBackend`); `AllocsPerRun` checks in panaudia
CI; the cloud-mixer doc obligations (a `containers.md` package-map row
+ `covers:` path, `context.md` if D3-A adds a transport/port, and a new
L3 component doc only if we judge the front end carries a load-bearing
invariant of its own — the funnel and hot-path contracts arguably
qualify, but they're documented in `session-lifecycle.md` and
`server-spi.md` respectively, so a pointer row may suffice).

## Recorded limitations (v0, no decision needed now)

- **Subspaces:** LASA has no subspace concept; LASA nodes join with
  none, so under the existing visibility rule they see (and are seen
  by) only other no-subspace nodes. Fine for flat spaces; a subspaced
  deployment would silo LASA clients. Reopen with the first subspaced
  LASA deployment.
- **Redundancy:** neither sent on sinks nor recovered on ingress (D6).
- **Ambi sinks:** global-order deviation 1 stands; `StartSink` rejects
  the wrong order. Raw-ambi output via the ROC path is untouched.
- **UUID mismatch on pose:** the incumbent only warns when a state
  packet's uuid mismatches the session; LASA's per-entity tracks make
  the equivalent impossible by construction — nothing to do, noted for
  symmetry.

## Investigation items (facts to establish, not choices)

- **I-1:** does a safe `EnableOut`-disable path exist for D8-B, or
  does gating render require engine work?
- **I-2:** cleanest passive read of the bouncer fan-out for D9b/D9c —
  can a non-connection `IMessageSender` subscriber be registered
  without a fake node?
- **I-3:** is `BouncerClient`'s smoothed volume readable by the front
  end for `PresenceEntry.Loudness`, or do we RMS in the front end?
- **I-4:** panaudia's world-frame axis convention and rotation signs,
  verified against a live client, before the D7 mapping is coded.
- **I-5:** confirm `Connect()`'s blocking behaviour in the direct
  backend (both incumbents call it in a goroutine and `Kill` on error
  — replicate) and that LASA's default 10 s `SetupTimeout`
  comfortably covers `Admit`'s worst-case ~4 s eviction wait.

## Phase gates

Each phase ends with the full panaudia + lasa test suites green under
`-race`, and:

- **P0 gate:** `go build ./...` in panaudia with the lasa module
  imported; cloud-mixer still builds untouched.
- **P1 gate:** e2e — LASA client audible to an old-plane client and
  vice versa on one space; kill/evict/reconciler/drain all observed
  running the funnel exactly once per session generation; allocation
  checks pass on both audio paths.
- **P2 gate:** LASA client's mute/solo affects the mix; roster
  visibility matches whatever D9 chose, documented.
- **P3 gate:** ticketed LASA session admitted and kick-gated;
  cloud-mixer `check_architecture_docs.sh` clean; deviations/reopen
  triggers recorded in `implementation-plan.md`.
