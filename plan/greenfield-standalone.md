# Greenfield rebuild: panaudia-engine + panaudia-server

*Drafted 2026-07-29 as an alternative to `panaudia-integration.md`;
**decided the same day** — this is now the plan, and
`panaudia-integration.md` is the priced alternative not taken.
Grounded in a same-day survey of the panaudia render core's dependency
graph, entanglements, vestigial code, cgo boundaries, and the NFC
bilateral plan status.*

## The decision

Current panaudia works and keeps serving current users, **untouched
and unmigrated** — it enters maintenance mode when its successor
reaches parity. The cloud server is heading for a significantly
different architecture anyway, so tight standalone/cloud alignment
stops being a goal; the two future servers share a few modules, not an
architecture. The new work is organised as three artifacts:

| Artifact | What it is |
|---|---|
| **`lasa`** (existing repo) | The protocol only: spec + the wire/state/connect/shell/client packages. Deliberately **not** rebranded as "the reference implementation" of which the server is the embodiment — implementations stay at arm's length so partial conformance (no directivity yet, the recorded deviations) reads as a fact about a server, not a defect in the spec's own repo. |
| **`panaudia-engine`** (new repo) | The protocol- and transport-blind audio library: jitter buffering, decode, spatial render, policy, binaural encode — Layers 1+2 below, plus the cgo. **Copied out of panaudia with its tests**; imports nothing from `lasa`. |
| **`panaudia-server`** (new repo) | The new standalone server: `lasa` + `panaudia-engine` + a thin Backend adapter + the pragmatic layer (config, stats, parity items). The panaudia *product* lineage continues under its own name; the protocol got the new name. |

The future cloud implementation becomes a later, third composition
over the same two libraries, coupled to `panaudia-server` only by
choice.

**Naming (decided, alternatives recorded):** `lasa-render`/
`lasa-server` were considered and rejected — the `lasa-` prefix on the
server implies a conformance strictness not intended (and dilutes
"lasa" as unambiguously meaning the protocol as implementations
multiply). `panaudia-core` was considered and rejected — "core" is the
most overloaded word in the project (the old repo's `core/` package
tree, `lasa-core.md`), whereas "engine" collides only with lasa's
`state.Engine`, which is habitually qualified as "the state engine".
`panaudia-mixer`/`panaudia-audio` were noted as content-accurate
alternatives; cosmetic either way.

**Succession, not fork:** `panaudia-engine` is a copy, not a shared
extraction — old panaudia is not refactored to import it. The
double-maintenance price of copying evaporates because the old repo
stops receiving DSP work; queued DSP work (loudness/headroom, post-M9
fixes) lands only in the new library. Residual liability: a DSP bug
found in old panaudia during the transition window is backported by
hand or accepted. The copy **takes the tests with it** — the M1–M8
bilateral anchors and the budget/soak tests are most of the value of
copying over regenerating — and carries a provenance note recording
what was copied at which panaudia commit.

## What the survey established (ground truth)

- **The mix already runs network-free.** `direct/budget_m8_test.go`
  builds a 50-person full mix with tone sources and null listeners and
  no sockets; `fixtures/` and the DSP-level suites (`core/ambisonic`,
  `core/binaural` M-chain tests) do the same lower down. The seam
  exists de facto; it had never been named.
- **The DSP stack is clean and copyable as-is** (~6.5 kLOC non-test):
  `core/ambisonic` (← `core/gemm`), `core/binaural`
  (← `core/convolver`, `core/hrtf`), `core/buffers` (JitterBuffer v3),
  `core/inout` minus `encoding.go`, `core/timing`, and the
  mixer-relevant subset of `core/common`. None of it imports
  statecache, commands, sessions, or any network package. cgo is
  fenced in four leaf packages with stable Go facades (convolver C,
  gemm/Accelerate/xsmm, CBuffer alloc, libopus); everything builds
  with plain `go build` (xsmm stays an opt-in tag).
- **The entanglement hub is `core/space`** (~2.9 kLOC non-test); most
  of its awkwardness is state-plane (`BouncerClient` 739 LOC,
  `KickGate` 278, stats — none referenced by the render path) or
  vestigial (clustering residue: `PeerEncoders` allocated but unused,
  `Worker` constructed nowhere, a globals path nothing enables,
  `OpenToChange` lock-ghost). The orchestration worth keeping is
  ~1.2 kLOC: the In → barrier → Across → Out phase structure (the
  input-ring correctness depends on the barrier), the slot allocator,
  the changes-queue "mutate only on the audio thread" discipline, and
  the ops→encoder mutator logic.
- **Policy meets DSP inside `ambisonic.Encoder`**, already reduced to
  per-node scalars and sets (gain, attenuation, subspaces,
  mutes/solos, roles, role composites) applied via a complete typed
  mutator surface. The only state-plane binding is the statecache-key
  parsing in `base_space_ops.go`, isolated. Kick never reaches DSP
  (it severs connections) — stays true.
- **48 kHz / 240-sample frames is a hard invariant** across reverb,
  NFC tables, the Woodworth delay ring, opus, and jitter defaults.
  The engine inherits it; parameterising sample rate is out of scope.
- **The NFC bilateral redesign is not mid-flight in code.** M1–M9
  implemented; bilateral always-on, per-output render mode;
  validation/rollout remains. The copy does not cut through active
  DSP work — and the M-series anchors are the argument for copying
  `core/ambisonic`/`core/binaural` as-is rather than rewriting.
- Decode currently happens **before** the jitter buffer (PCM in the
  buffer, decoded on the network goroutine at packet cadence); the
  mixer tick is the only clock, owned by `main.go`'s 5 ms loop.

## What this path dissolves (vs the integration plan)

Fate of `panaudia-integration.md`'s decisions:

| | Fate here |
|---|---|
| D3 port sharing | **Dissolved** — `panaudia-server` owns its own port; no incumbent listener. |
| D5 id mapping | **Dissolved** — LASA string ids native throughout; no UUID plane. |
| D9 state bridging (all three parts) | **Dissolved** — one state plane: the LASA engine. No statecache, bouncer, commands, or mirroring. |
| D4 entities per connection | **Transformed** — a free design choice in the engine's entity model (entity = optional source + optional sink). |
| D7 pose cadence | **Transformed** — no BouncerClient to flood; per-packet rotation reaches the encoder directly. Unit/axis/sign mapping still needed once, at the engine boundary (I-D). |
| D8 sink gating | **Transformed** — subscription-driven render is natural when the engine is designed around `StartSink`. |
| D1 fork module arithmetic | **Done 2026-07-30** — fork branch `lasa-setup-params` carries the module-path rewrite as its last commit (upstream PRs cherry-pick from below it), tagged `v0.8.4-lasa.1`; lasa imports rewritten and the `replace` dropped — plain `require github.com/panaudia/moqtransport v0.8.4-lasa.1`, all green under `-race`. |
| D6 jitter geometry, hot-path contract | **Unchanged** — physics doesn't care which plan. |
| D10 auth phasing | **Roughly unchanged** — unticketed first, LASA tickets after; no old JWT format to coexist with. |

Not dissolved but *moved*: base-profile policy
(implementation-plan.md step 7) stops being deferrable. The LASA
engine is the only state authority, so the minimal policy subset
(mute, solo, gain, block/kick keys → engine mutators, enforced from
`lasa.*` state) is on the critical path of the first usable server —
step 7 pulled forward in place of the bridging code.

## panaudia-engine

Two layers, one module:

**Layer 1 — the DSP packages, copied as-is** (with tests), with two
splits: `core/inout` sheds `encoding.go` (NodeInfo3 — state plane),
`core/common` sheds control-messages/server-error (server plane).
Legacy `CircularBuffer`/`CircularBufferA` (~550 LOC, directroc-only)
stay behind.

**Layer 2 — a fresh orchestration package** (the `Space` rewrite,
~1.2 kLOC of keepable logic re-homed, vestiges dropped). Keeps the
four load-bearing ideas — phase barriers, slot allocator,
audio-thread-confined mutation, the encoder mutator surface as the
policy input — and replaces the node in/out duality with the entity
model (optional source, optional sink, string ids; sinks start and
stop on subscription).

**Hard boundary:** `panaudia-engine` imports nothing from `lasa`. Its
value to the future cloud rests on being protocol- and
transport-blind — string ids, its own pose types, audio in, frames
out. The `panaudia-` name is lineage, not dependency.

### Sketch of the library surface

Deliberately close to the LASA server SPI, because the SPI is the
network-facing half of this seam — `panaudia-server`'s Backend is a
thin adapter between two interfaces designed for each other:

```go
// engine package — no network, no state plane, no lasa imports.
// No wall-clock anywhere: the engine speaks seq and sample indices
// only (protected decision — see "Engine design decisions" below).
type Mixer struct{ ... }              // one space; ticked or self-running (SD3)

type Pose struct{ ... }               // metres, radians — native units

type SourceConfig struct{ ... }       // initial pose, RenderParams…
func (m *Mixer) AddSource(id string, cfg SourceConfig) (*Source, error)
func (m *Mixer) RemoveSource(id string)

type Source struct{ ... }
// Pose and audio arrive in the same call, mirroring the wire: nil
// pose = decimated packet; empty audio = pose-only packet. seq is
// the gapless packet sequence (core §5.1); no wall-clock.
func (s *Source) WriteOpus(seq uint64, pose *Pose, pkt []byte)   // decode + jitter, caller's goroutine
func (s *Source) WritePCM(seq uint64, pose *Pose, pcm []float32) // float32; decoded-elsewhere leg (cloud gateway)
func (s *Source) Loudness() float32      // smoothed RMS, for presence (SD4)

func (m *Mixer) AddSink(entityID string, w FrameWriter) (*Sink, error)  // binaural v0; ambi later
func (k *Sink) SetPose(p Pose)           // latest-wins slot, read once per tick
type FrameWriter interface{ WriteFrame(opus []byte, sampleTS uint64) }  // samples, never seconds

// Policy surface — SD2 settled 2026-07-30, shape (C), three tiers:
func (m *Mixer) SetRenderParams(id string, p RenderParams) // per-entity, set wholesale; DefaultRenderParams() exported
func (m *Mixer) SetChannel(name string, c ChannelParams)   // space-scoped channel table: muted / gain / attenuation
func (m *Mixer) SetEntityMuted(id string, muted bool)      // space-scoped moderation mute
// tier three — the derived per-pair record matrix — is internal

func (m *Mixer) Process()                  // host-clocked tick — SD3 settled: primary
func (m *Mixer) Run(t Ticker)              // standalone convenience over Process
```

The source/sink pose split is deliberately visible in the surface —
sources take pose with audio in one call, sinks have a bare
`SetPose` — rather than hidden behind a uniform pose setter; the two
pipelines have different semantics (below) and the API should say so.

### Seam decisions

Settled by the repo split:

- **SD1 — ingest form: both.** `WriteOpus` (library owns decode +
  jitter, today's order — FEC/concealment at network cadence) and
  `WritePCM` (a cloud gateway may decode elsewhere). The decoder is a
  ~60 LOC front over cgo; offering both is cheap and forecloses
  nothing. *2026-07-30:* PCM is float32 and takes the same jitter
  path by default (a network-fed gateway has the same reordering
  problem; "bypass jitter" is a possible later `SourceConfig` knob,
  not a day-one branch). The PCM frame-granularity contract
  (240-sample multiples vs reframing) is pinned at implementation.
- **SD3 — clock: both, `Process()` primary.** The standalone server
  uses `Run`; a cloud mixer with an external clock uses `Process`.
- **SD6 — home: copy into `panaudia-engine`** (see "Succession, not
  fork" above). Alternatives priced and rejected: extract-a-shared-
  module (churn in the repo this path leaves alone + versioning
  obligation), subdirectory module in panaudia (two worlds in one
  repo, awkward tags), new-server-imports-panaudia-directly (release
  coupling, declutter never happens). The copy's usual price —
  divergence — is voided by freezing the source.

Settled 2026-07-30 (pre-build design session; details in "Engine
design decisions" below):

- **SD2 — policy input: shape (C), three tiers.** Per-entity
  `RenderParams` set wholesale, space-scoped mutators (channel
  table, entity mute), and an internal derived per-pair record
  matrix. The injected-predicate alternative (B) stays the recorded
  refactor if cloud clustering wants it.
- **SD5 — subspaces: replaced by channels.** Resolved in a third
  direction neither option anticipated: the base profile's channel
  model (audible iff a shared unmuted channel; highest channel gain
  wins, lowest attenuation wins; per-channel overrides) is the
  engine's native visibility vocabulary. Subspace sets do not
  survive as API.

- **SD4 — metering: settled by the build (2026-07-30).**
  `Source.Loudness()` lives in the library — a smoothed RMS computed
  on the ingest path (v1: per-packet EMA; the algorithm gets its
  presence-driven tuning pass in Phase C). Host-side metering was the
  alternative; it bought nothing and spread DSP outside the seam.

### Engine design decisions (2026-07-30)

Settled in the pre-build design session. Each shapes the Layer 2
API; none disturbs the Layer 1 copy.

**Units: metres and radians, natively.** The old 0–1 normalisation
existed for the state plane; the physics (NFC tables, Woodworth
delay, distance attenuation) is metre-denominated anyway. The engine
surface and Layer 2 internals speak metres/radians throughout, and
**space-size is removed** — no scale factor survives. I-D changes
shape accordingly (see the item): find every point where 0–1 /
`PANAUDIA_SPACE_SIZE` / degrees enter Layer 1 and move the
conversion to a single point inside the copy; pin the origin
convention; check the attenuation curve's distance denomination.
Axes and rotation conventions are unchanged from the incumbent.

**No wall-clock — a protected decision.** Incoming audio has seq,
not timestamps. The engine's only clocks are the gapless packet
sequence and the monotonic **sample index** derived from decoded
packet lengths; nothing in the engine is denominated in seconds,
micro- or nanoseconds. The sink-side wire timestamp (the core §5.1
(seq, ts) anchor) is the server's concern, outside the seam —
`FrameWriter` carries a sample timestamp, not micros.

**Channels and the per-pair record.** Channel resolution is
**precomputed on change**, not evaluated per frame: the slot
allocator's dense indices give a flat `[maxSlots][maxSlots]` matrix,
mutated only on the audio thread when memberships, the channel
table, or render params change. *Decided 2026-07-30 (built):*
**include/exclude needs no DSP-layer change at all** — the peers
slice was always the engine's input, so each listener is handed its
own channel-filtered source list, assembled per tick from the
boolean matrix ("option D"; the encoder still applies
self-exclusion and moderation mute to what it receives).
Alternatives priced and rejected: an `Includes` set inside
`shouldIncludePeer` (grows the copied encoder's vocabulary for
something the caller can express by construction — though it would
have kept all policy in one gate); expressing channels via the
existing sets (impossible: Mutes are bidirectional, Solos bypass
moderation, SubSpaces are symmetric — directional include is
outside their vocabulary). **Per-channel gain/attenuation overrides
are per-pair scalars and wait for Phase C's pair-record work** —
the matrix grows from booleans to {include, gain, attExp} records
then, which is also occlusion's future home. The spec gap that
blocked them is closed (erratum 11, resolved 2026-07-30: gain
composes multiplicatively, unset = identity; attenuation overrides
when set, unset = no override — profile §4 rewritten). Open
implementation detail for Phase C: whether entity `render.gain`
folds into the pair record at recompute or stays entity-scoped and
multiplies at encode; and `ChannelParams` needs a real "unset"
representation for attenuation (a plain float can't say
no-override).

**Render state and defaults.** The render-relevant state subset
(base profile: `render.gain/attenuation/size/directivity`, channel
memberships, channel table, entity mute) crosses the boundary as
the shape-(C) surface above. The engine owns its defaults
(`DefaultRenderParams()` exported) — an entity added before its
state arrives must render sensibly; the old entity-before-attributes
lesson solved structurally. The engine never sees a `lasa.*` key
name — key parsing stays in the server — and a **parity test in
panaudia-server** asserts engine defaults == the profile's
documented defaults, so they cannot drift without a test failure.
One default that matters more than it looks: channel membership for
an entity whose config omits channels decides whether a fresh
source is audible to anyone before state lands.

**Pose: two pipelines by role.** The spec's §5.4 note (pose bypasses
the jitter buffer, latest-wins) is right for the rotation path but
conflates it with the source path; the engine splits them:

- *Sink pose — latest-wins slot.* No buffering, no interpolation;
  read **once per tick** into a per-tick snapshot consumed by both
  the bilateral ambisonic mix and the binaural render — the
  same-rotation requirement solved structurally, never two reads.
- *Source pose — a parallel pose store beside the jitter buffer*
  (chosen over tagging jitter entries inline). Per source, an SPSC
  ring of `(seq, endSampleIndex, pose)`: single writer (network
  goroutine, on any pose-bearing packet including pose-only), single
  reader (mixer tick), atomic indices, fixed capacity (32 covers any
  sane decimation), zero allocation. Write guard: drop seq ≤ last
  written (reordering and redundant duplicates). The pose pins to
  the sample position of the **end** of its packet's audio, matching
  the wire's "freshest at assembly time"; a pose-only packet lands
  at the current end-of-stream position with a higher seq
  (latest-wins resolves the zero-width interval). At dequeue the
  mixer computes the frame's last-sample index L, finds the
  bracketing entries, and lerps (position linear, angles
  shortest-arc) — one evaluation per source per frame, held across
  the frame's 240 samples: the rendered position is where the source
  was at the newest audio sample being rendered, regardless of pose
  decimation rate. **Interpolation inside the jitter window is
  free** — audio already paid the delay — so this adds no latency;
  no bracket yet → hold the last pose. Two properties inline tagging
  could not have: the copied jitter buffer stays untouched (the
  Phase A gate validates a faithful copy), and pose continuity
  survives audio concealment (a concealed frame has no packet and
  hence no inline pose; the parallel timelines fail independently).
- A both-roles entity's one pose write fans into both structures —
  consumed fresh for its ears, jitter-aligned for its mouth. Two
  deliberate delays, not a bug.
- Pins at build time: (i) what seq/sample bookkeeping the copied
  JitterBuffer v3 exposes at dequeue (it tracks placement internally
  for loss/concealment; the question is whether the copy surfaces it
  or grows a getter); (ii) ring-scan cost, measured in the ported
  budget test; (iii) source attitude interpolates identically —
  plumbed now, consumed later by directivity.

**Size and directivity: plumb now, render later.** `render.size`
(metres, 0 = point) and `render.directivity` (0–1) ride in
`RenderParams` from day one, defaulted and ignored by the DSP; pose
plumbing carries full 6-DoF from the start. Directivity later is a
DSP change, not a re-plumbing.

## panaudia-server

Composition: **`lasa/server` shell (built, tested) + Backend adapter +
`panaudia-engine` + a small `main`**. The adapter is thin by
construction: `WritePacket` → `wire.ParseMonoObject` (zero-copy) →
pose fast path → `Source.WriteOpus`, all on the receive goroutine;
`StartSink` → `AddSink` with the shell's `SinkWriter` as the
`FrameWriter`; `Presence()` → iterate sources' pose + `Loudness()`.
The hot-path contract of `server-spi.md` holds with no translation
layers — no uuid mapping, no NodeConfig, no funnel hand-off.

**Lifecycle:** the old departure funnel coordinated cleanup across
bouncer/statecache/session planes; none exist here. Teardown is the
LASA shell's own (invariant-tested: guaranteed detach, departure
sweep, displacing attach) plus `RemoveSource`/sink stops. The
funnel's *lessons* carry as design rules, not code: single owner
goroutine observes transport death; removal is idempotent and
generation-guarded against id reuse (evict-old admission recreates
the reuse hazard); encoder cgo free only on the mixer goroutine after
rendering stops; eviction waits for the predecessor's teardown before
the successor's admission writes (the SPI's Admit-may-block contract).

**Transport reach — an honest gap:** `lasa/server` today serves raw
QUIC only (`quicmoq`). Browsers cannot open raw QUIC, so either
WebTransport support is added to the shell (Eyevinn ships
`webtransportmoq`; the work is plumbing plus the Safari quirks the
old front end already solved) or `panaudia-server` is
native-clients-only until it lands. A scheduling decision on the
critical path to any browser user.

**Reference-vs-product, resolved structurally:** the spec-shaped
behaviour lives in the `lasa` module the server imports; the
pragmatic accretions (stats, config surface, parity items) are
visibly `panaudia-server`'s own. The server is *an* implementation of
LASA — a full one in ambition — not "the reference implementation."

## Phasing

**Phase A — panaudia-engine.** Create the repo; copy Layer 1 with
tests + provenance note; write Layer 2 fresh (entity model,
phase-barrier loop, policy mutators keyed by string id). Also the
lasa-repo prerequisite: rewrite moqtransport imports to the fork's
real path, drop the `replace` (D1). Acceptance: the budget/soak and
M-series tests **ported, not rewritten** — the 50-person budget test
running against the new library on dev and the x86 production box is
the gate that the copy preserved the render's performance envelope.
No network anywhere in this phase. The 2026-07-30 engine decisions
leave the copy untouched — the pose store and pair matrix are new
Layer 2 components. *Amended 2026-07-30 (Paul's ruling, during the
build):* small Layer 1 changes are permitted before the gate when
Layer 2 needs them — neater than contortions to avoid them — with
every one surfaced explicitly and recorded in the engine repo's
PROVENANCE.md, which is the ledger of divergence from the copy.
*Status 2026-07-30:* Layer 1 copy and Layer 2 build complete, all
green under `-race`; budget gate green on dev (~0.55 ms median —
an Apple Accelerate signal-delivery AMX corruption was diagnosed,
reported to Apple, and fixed in place with a per-call signal
shield, keeping Accelerate as the darwin default; `-tags xsmm` now
also builds on darwin; PROVENANCE divergence #3); prod-box run
outstanding.

**Phase B — server assembly.** `panaudia-server` repo: Backend
adapter + main; unticketed auth; e2e with the lasa reference Go
client (two clients hear each other, evict-old, teardown paths);
`AllocsPerRun` checks on both audio paths of the adapter. Milestone:
a native LASA client round-trip on a clean server binary.

**Phase C — policy depth (step 7, pulled forward).** Base-profile
enforcement against the state engine: existence/name/attrs read;
mute, solo, gain, block/kick keys → engine mutators via an observer
subscription; kick severs the connection (shell), never touches DSP;
loudness into presence; departure sweep symmetry. Milestone: a
moderated space with only the LASA state plane.

**Phase D — reach and parity.** WebTransport in the shell (if
browsers are in scope); TS client speaks LASA; LASA tickets (shared
or new Ed25519 key); a **feature-parity inventory** against old
panaudia (I-A); A/B listening against the incumbent; old panaudia
enters maintenance mode.

The cloud rewrite is explicitly not a phase of this plan. What this
plan owes it: the shared-modules list (`panaudia-engine` + `lasa` +
possibly the state engine as a service per `decomposition-seams.md`)
and the seam choices already made in the accommodating direction
(SD1-both, SD3-both).

## Prices accepted (recorded, not hidden)

- **Migration continuity.** Nobody uses the new server until the TS
  client + parity land (Phase D); real-user feedback on the new code
  starts late. Old panaudia keeps working meanwhile — untouched is
  also unrisked.
- **Hardening is re-earned.** The reconciler, stale-read timeouts,
  the eviction wait, Safari quirks each encode an incident.
  Mitigations — carried test anchors, LASA's invariant suite, the
  lessons list — are real but not years of production soak.
- **Two servers operated** during the transition; old-server bugs get
  fixed in a codebase no longer invested in (or accepted).
- **If the new cloud architecture lands far from this standalone
  shape**, Layer 2 is standalone-only code and only Layer 1 + lasa
  were truly shared. The bet: Layer 2 is small enough (~1–2 kLOC)
  for that to be acceptable.

## Alternative not taken

`panaudia-integration.md` — integrate a LASA front end into existing
panaudia (entity → ConnectionHandler through the departure funnel,
dual state planes bridged, both MoQ servers side by side). Kept as
the priced record; its D-decisions map to this plan per the table
above. The axis that separated them: integration spends on bridging
two planes that migration later deletes and buys early coexistence
with real users; this path spends on the copy and re-earned hardening
and buys a clean codebase optimised for LASA plus a deliberate,
module-sized sharing story with the future cloud.

## Investigation items

- **I-A: feature-parity inventory** of old panaudia (env surface,
  stats, ROC/Link, recording, admin, health endpoints) — what must
  exist before "maintenance mode" is honest.
- **I-B: WebTransport-in-shell effort** — what of the incumbent's WT
  path (upgrade handling, Safari SETTINGS workarounds, empty-ALPN
  shim) applies to the fork, and what can be lifted.
- **I-C: `common` split inventory** — *done 2026-07-30, during the
  copy:* only `control_messages.go` and `server_error.go` were
  server-plane; everything else (constants, trig, rotation,
  bilateral, types, logging) is consumed by the DSP layer and copied.
  The NodeInfo codecs travelled in `inout/encoding.go` and stayed
  behind with it (two zero-copy f32 views extracted verbatim — see
  the engine repo's PROVENANCE.md).
- **I-D: units inside the copy** — *resolved 2026-07-30, better than
  hoped:* `Encoder.SetPosition` turned out to be the **only** place
  the 0–1/space-size representation entered Layer 1, so the engine
  passes metres with `MixerConfig.Size = 1.0` and the copied
  conversion is the identity — the single conversion point is the
  engine's `Pose.position()`/`rotation()` (radians→degrees). The
  origin question dissolved: the geometry pass is entirely relative
  (listener−source differences), so LASA's origin passes through
  untouched. The attenuation curve is metre-denominated with its
  reference at 1 m (`NearFieldGainCap` anchors there) — no
  space-size dependence existed to remove. Axis/sign conventions
  unchanged per the incumbent (same SAF frame).
- **I-E: prod-box validation debt** — NFC M8/M9.6 rollout items are
  still open on old panaudia; decide whether the Phase-A budget-test
  gate subsumes them or they run first on the old code.
