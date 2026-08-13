# panaudia-server: composition design

*Started 2026-08-01, opening Phase B. The working document for the
boundary and composition decisions between `lasa` (protocol),
`panaudia-engine` (audio), and the new `panaudia-server`. Companions:
`greenfield-standalone.md` (the plan this executes),
`server-spi.md` (the lasa shell's embedder contract).*

## Agenda (from the 2026-08-01 boundary survey)

Findings from reading both libraries against each other, worked
through one at a time below; unresolved items stay listed here.

- **A. Clocks** — **resolved, §1**: one authoritative clock (the
  render tick); epoch₀ read once at first tick; SPI in sample time;
  anchors derived; global sink stamps; closure plumbing.
- **B. Loudness** — **resolved, §2**: computed per frame in the
  render pipeline, post-gain, published atomically; presence
  assembly is a server-side object answering the shell's polls.
- **C + E. Redundancy recovery / ingress receiver** — **resolved,
  §3**: merged; a Depacketizer utility in lasa owns the §5.1
  receiver state machine (parse, discard, reorder, redundancy
  substitution, loss declaration); decode and concealment stay in
  the engine.
- **D. Head-frame entities** — **resolved, §4**: a certainly-wanted
  feature, deferred; v0 rejects `frame: "head"` at admission
  (recorded deviation with reopen note and an implementation
  sketch).
- **F. `profile/base` consumer vocabulary** — **resolved
  (2026-08-01, confirmed)**: typed parsers/defaults for
  `render.*`/`channel.*`/moderation/`mute`/`solo` grow in lasa's
  `profile/base`; the server wires (observer → parse → engine
  mutators); the engine stays key-blind; the defaults parity test
  lives in the server, asserting profile defaults ≡
  `engine.DefaultRenderParams()`. Vocabulary in the profile, wiring
  in the server, mechanism in the engine.
- **G. Presence aggregation & publication** — **resolved, §6**: a
  third singleton (sibling to state engine and mixer), internal to
  lasa; latest-wins collector slots as the rate reducer; cadence
  sampler + pubsub; collector/aggregation as the external-service
  seam; presence leaves the SPI.
- **H. Block/kick** — **resolved, §7**: a moderation gate inside
  lasa's server package (state subscription + auth-time refusal +
  live sever through the funnel); no SPI surface.
- **Entity bookkeeping / departure authority** — **resolved, §5**:
  transport session is the single authority; engine originates no
  departures; staleness vestiges removed from the engine.
- Minor — **closed as config-notes**: adapter sets LASA jitter
  geometry (W = 5 ms one-frame packets); sink-format negotiation and
  the global-ambi-order deviation live in `StartSink` against server
  config.

## 1. Clocks

**The principle (Paul, 2026-08-01, decided):** the server has exactly
**one authoritative clock: the render tick**. There is no other
source of timing information. Milan/Dante/AVB-style shared clocks,
NTP, presentation timestamps and system-time reads are rejected —
each added time constraint either degrades performance or propagates
inaccurate information. Time constraints are added only where they
belong, minimally.

**What this means structurally.** A networked audio server unavoidably
*touches* many clocks — every sender's sound card ticks at its own
~48 kHz. The architecture's answer is that those clocks are never
*read*: each one is embodied in its packet stream (seq, gapless), and
is reconciled against the render tick at exactly one place — the
jitter buffer — purely in the sample domain (fill adaptation and
±sample drift correction), never by timestamp comparison. Downstream
of the buffer there is only the tick: sample time is `ticks × 240`,
and every engine surface (pose ring, sink frames, latency
instrumentation) is denominated in seq and samples. The engine's
no-wall-clock rule (greenfield plan, protected) is this principle
applied to the library.

**Clock vs pacer.** The rule governs *time values in the data plane*.
Mechanisms that merely schedule work (the OS sleep inside
`timing.Ticker` that realises the 5 ms cadence, the shell's presence
delta cadence, state GC sweeps) are pacers, not clocks: they produce
no time values, attach nothing to data, and their jitter cannot
propagate. Diagnostics (`ProcessStats` phase timing) measure host CPU
cost and stay outside the data plane by construction — the recorded
exemption stands.

### The clock, defined (decided 2026-08-01)

The load-bearing ticker fact: `timing.Ticker` (tight mode) targets
**accumulated** time — `targetTime = previous + 5 ms`, with
`previous = targetTime` even on overrun — so the schedule is anchored
to start + N×5 ms exactly; scheduling jitter never accumulates, and
Go's monotonic time arithmetic makes the discipline immune to wall
steps and NTP slews. **Tick count × 5 ms IS elapsed monotonic time**,
quantized to frames.

**The server's clock is therefore: epoch₀ = the wall time of the
first tick, read once; thereafter
`time = epoch₀ + ticks × 5 ms ≡ epoch₀ + samples / 48 000`.**
One wall-clock read in the server's lifetime; every time value
anywhere (wire ts anchors included) derives from it by sample
arithmetic. Recorded as features: post-start NTP adjustments cannot
move this clock (no inaccurate information propagates mid-session),
and long-session divergence from true wall time equals the crystal
error — the honest rate at which audio is actually produced.

**Resolved consequences:**

- **C1 (decided): the SPI carries sample time.**
  `SinkWriter.WriteFrame(frame []byte, sampleTS uint64)` — the
  embedder is wall-clock-free end to end; the stub is the only
  embedder, so the change is cheap now.
- **C2 (decided): the tick-summed clock mints the anchors.** The
  shell's informational §5.1 sink anchors are computed as
  `epoch₀µs + sampleTS × 10⁶ / 48000` — no per-anchor wall reads,
  anchors perfectly coherent with the stream.
- **C3 stands as the invariant**: uplink carries no timestamps; the
  jitter buffer is the sole clock-domain coupling (fill adaptation
  in the sample domain, never timestamp comparison); no component
  compares stream time with system time on any audio path.

- **C4 (decided): sink frames stamp the global sample clock.** The
  engine's `Sink.sampleTS` becomes the mixer's global sample time
  (frame's first sample = (tick−1) × 240) instead of a per-sink
  counter: one epoch₀ mapping serves every track and any two frames'
  timestamps are directly comparable. Small engine change.
- **C5 (decided): epoch₀ reaches the shell as a converter closure.**
  lasa `server.Config` gains `SampleEpoch func(sampleTS uint64)
  uint64` (µs), called lazily at anchor emission — no startup-order
  constraint between mixer and shell.

**§1 outcome — work items** (queued for the Phase B build):
1. lasa: SPI `WriteFrame` → `sampleTS`; `Config.SampleEpoch`
   closure; shell anchor arithmetic `epoch₀µs + sampleTS·10⁶/48000`;
   stub + e2e updated; the C3 invariant sentence added to
   `server-spi.md` (and the §5.1 note when the spec is next touched).
2. engine: `Sink` stamps global sample time.
3. server: owns the ticker, reads epoch₀ at first tick, hands the
   closure to the shell.

**§1 CLOSED.**

## 2. Loudness (decided 2026-08-01)

**Principle (Paul):** loudness is a property of the entity, only
knowable in the engine once gain has been applied — shortcutting it
with a convenience calculation in the adapter would be wrong.

**Decision:** loudness moves from the ingest path to the **render
pipeline**: computed every frame in the In phase, on the audio
thread, from the frame just read out of the jitter buffer, multiplied
by the entity's current `render.gain` (audio-thread state — no
cross-thread mirroring), EMA-smoothed at the regular 5 ms cadence,
and published to an atomic read by `Source.Loudness()` whenever
wanted. This changes what is measured, deliberately: **what is being
rendered**, not what arrived — a starving source's presence loudness
now decays to silence instead of freezing at its last packet; and the
ingest hot path sheds its per-packet RMS entirely.

Matches the profile (post-`render.gain`, pre-attenuation,
entity-gain-only per the erratum-11 clarification). The EMA
time-constant becomes a small named tunable; the loudness-byte wire
encoding stays in lasa (`wire`/`presence`).

**Server side:** a dedicated presence-assembly object owns answering
the shell's `Presence()` polls at the shell's cadence (never at
ingress rate): it holds the latest-pose cache (fed from ingress,
after dof enforcement), the head-frame flags, and reads each source's
loudness atomic. The shell keeps the track machinery (generations,
indexes, chunking); the engine keeps the value; the assembler is the
snapshot in between.

**§2 outcome — work items:**
1. engine: remove `updateLoudness` from `Write*`; compute in
   `phaseIn` (RMS of `enc.Input` × gain → EMA → atomic);
   `Source.Loudness()` unchanged in signature; time-constant named.
2. server: the presence assembler object (pose cache + dof + flags +
   loudness reads).

**§2 CLOSED.**

## 3. Ingress receiver: the Depacketizer (decided 2026-08-01)

**The reframe that shaped this:** the binding constraint on
redundancy recovery is not the append-only jitter buffer but the
**stateful Opus decoder** — packets must be decoded in stream order,
so any recovery scheme must hold *decode* back across a gap. The
natural shape is a small per-source pre-decode stage: the
Depacketizer.

**The offset stays (k=1 simplification considered and rejected).**
The sliding structure means one copy per packet covers a contiguous
burst of up to k packets — every packet carries the copy at fixed
lag k, so consecutive packets carry copies of consecutive frames,
and a burst's copies all ride in the k packets after it. What a
single copy can never cover (frame and its carrier both lost) is
equally uncoverable at k=1. At 5 ms cadence, real loss events (WiFi
fades, bufferbloat, handovers: 10–100 ms) span multiple packets, so
burst cover is the common case, not the exotic one — and the
receiver's state machine is k-agnostic, so generality costs nothing.
Bandwidth is 2× regardless of k; larger k costs only recovery
latency (k×5 ms must fit inside jitter depth).

**Decision (Paul): the Depacketizer lives in lasa**, as part of the
transport mechanism — the `server` package's anticipated
"decoded-handler convenience wrapper" grown to its full §5.1/§5.2
receiver responsibilities, shared by every embedder (the cloud
gateway identically). Boundary:

- **lasa (Depacketizer)**: flag validation and discard,
  `ParseMonoObject`, seq tracking, reorder, hold-on-gap, redundancy
  substitution, loss declaration. Emits to the embedder, in stream
  order: `Frame(seq, pose, opus)` and `Lost(seq)`.
- **engine**: decode (`Source.WriteOpus`) and concealment — a new
  `Source.Conceal(seq)` entry mapping `Lost` to the decoder's PLC
  (`DecodePLC` into the jitter stream — better than silence, and it
  keeps the pose ring's sample accounting exactly aligned with the
  sender's timeline under unrecovered loss, fixing today's
  time-slip).
- **panaudia-server (adapter)**: wraps its `EntityHandler` with the
  Depacketizer and maps Frame/Lost onto the Source — near-trivial.

**No clock in the state machine** (§1-consistent): release is
**arrival-driven**. Frame n is recoverable only from packets
n+1…n+7 (offset ≤ 7), so the moment packet ≥ n+8 arrives without a
covering copy, n is provably unrecoverable → declare `Lost(n)` and
flush. A total stream stall needs no timeout: nothing plays anyway
(the jitter buffer drains at the read side), and a resume with a
seq jump flushes. Clean-network fast path: no gap → zero held
state, straight-through call.

**§3 outcome — work items:**
1. lasa: the Depacketizer in `server` (utility, not an SPI change),
   with table-driven tests for burst/reorder/carrier-loss/stall
   patterns; gap-length counters as its diagnostic surface (the
   evidence base for any future k tuning).
2. engine: `Source.Conceal(seq)` (PLC into the stream).
3. server: adapter composes it; v0 receiver-complete even though
   senders may not emit redundancy yet.

**§3 CLOSED.**

## 4. Head-frame entities (decided 2026-08-01: deferred feature)

**Decision (Paul):** head-frame entities (`frame: "head"` — a source
pinned relative to every listener's head; the aural HUD) are a
feature we will certainly want, deferred to a future date.

**v0 behaviour:** the server **rejects** `frame: "head"` at admission
with a clear rejection — a loud, honest deviation (in the style of
the implementation-plan's deviations list) rather than silently
rendering an "angel on the shoulder" as a fixed point in the room.
*(Default chosen for least surprise; flag if you'd rather
accept-and-render-world-frame.)* Reopen: when the feature is
scheduled.

**Implementation sketch for then (cheaper than first feared):** the
bilateral encode already computes, per pair,
`rel = headFrame(listener) · (source − listener)`. A head-frame
source's pose is *already* listener-relative — so per pair the
transform is simply skipped: `rel = source position, verbatim`. A
per-source flag in the encoder and one branch in the geometry pass;
the pose pipelines, presence krecord flag, and dof machinery already
carry everything else. The main design question left for then is
which pose pipeline head-frame sources ride (fresh vs jitter-aligned
— an HUD's "position" is authored, not physical, so fresh-slot
semantics may fit better than the source ring).

**§4 CLOSED.**

## 5. Entity bookkeeping: one departure authority (decided 2026-08-01)

**Finding:** the incumbent's activity-timeout (`checkForStaleNodes`,
drop-on-silence) was never copied — Layer 2 was written without it —
but its dead vocabulary was, and has now been **removed from the
engine** (pool.go; `TickSource`/`GetTick`; the
`BufferStatsDelegate`/`statsDelegate` plumbing — PROVENANCE #5).

**The authority is the transport session, voiced through the shell.**
One-directional chain of command: MoQ session truth (connect, death,
eviction, kick — all transport-plane) → the shell's
`Admit`/`EntityJoined`/`EntityLeft` → **one serialized lifecycle
funnel in the adapter** — the only code that mutates the conn-map and
the only code that calls the engine's `Add*/Remove*` → the engine,
which follows. Three rules make disagreement structurally impossible:

1. **Departure causes are exclusively transport-plane.** Audio
   silence is never a departure cause — a silent-but-connected source
   (DTX, muted mic) is a present entity; LASA's session liveness
   makes the old activity-timeout workaround obsolete. The engine
   originates no departures.
2. **Engine errors are fatal, never absorbed.** An unexpected
   `ErrDuplicate`/`ErrFull` outside the admission-reject flow tears
   the connection down — no half-states survive.
3. **The engine's registry checks are backstop assertions, not
   policy.** Firing unexpectedly means the funnel is broken (loud
   log); a test-time invariant (engine entity set ≡ conn-map set
   after any lifecycle sequence) rides the e2e suite.

The incumbent's departure-funnel lesson at a fraction of the weight:
one funnel, idempotent removes (already built), same process, one
code path — nothing to reconcile, ever.

**§5 outcome — work items:** the vestige removal is done (engine,
PROVENANCE #5); the funnel, the fatal-error rule and the e2e
invariant are Phase B server code.

**§5 CLOSED.**

## 6. Presence: the third singleton (decided 2026-08-01)

All state flows through one singleton (the state engine); all audio
through one (the mixer). **Presence becomes the third — internal to
lasa** (Paul), grown from the existing `presence` package (which
already owns the wire encoding) into the full aggregator:

**collector → sampler → builder → pubsub.**

- **Routing & rate reduction:** the adapter writes every pose
  (post-depacketizer, post-dof) into a **latest-wins slot per
  entity** owned by the collector — the slot *is* the rate reducer
  (absorbs 200 Hz; no queue, no decimation logic, no timestamps;
  the engine's sink-pose-slot pattern). Loudness is read at sampling
  time from the engine's atomics via a getter registered per entity
  at join. Reduction happens before aggregation by construction:
  the sampler never sees more than the latest value.
- **Sampler & builder:** a cadence pacer (§1-compliant: schedules,
  mints no time values) samples the slots and builds the
  delta/keyframe stream.
- **Pubsub:** subscribers receive the aggregated cadence-rate
  stream. Subscriber #1 is the MoQ shell's presence-track publisher
  (the generations/indexes/chunking machinery becomes a subscriber
  adapter). Future subscribers on the same surface: recording,
  analytics, clustard (entity locations), the cloud presence tier.
- **The external-service seam** (mirroring the state store's, per
  `decomposition-seams.md`): the split is **collector vs
  aggregation**. The collector stays process-local beside ingress —
  200 Hz × N never crosses a network; everything downstream of the
  sampler is location-independent (in-process standalone; per-node
  collectors feeding a central presence service in the cloud — the
  two-level shape clustering wants).
- **SPI change:** `Backend.Presence()` and `PresenceEntry` leave
  the SPI — presence is no longer the embedder's job; the adapter
  feeds the collector directly. The stub simplifies; a cloud
  embedder gets presence for free.
- *Supersession note:* this replaces §2's server-side
  presence-assembler object; §2's engine-side loudness decision
  (per-frame, post-gain, atomic) stands unchanged and is what the
  loudness getters read.

**§6 outcome — work items:**
1. lasa: grow `presence` into collector/sampler/builder/pubsub;
   shell presence machinery becomes a subscriber; drop
   `Presence()`/`PresenceEntry` from the SPI; stub + e2e updated.
2. server: adapter feeds poses to the collector; registers loudness
   getters at entity join.

**§6 CLOSED.**

## 7. Block/kick: the moderation gate (decided 2026-08-01)

**Home: inside lasa's `server` package** — §5 coherence: every
departure cause is transport-plane and voiced through the shell, and
the shell holds both the session table (kill capability) and the
state engine (subscription source). Exporting kill authority across
a boundary would break the single-authority chain.

- **Derived sets, not store reads:** the gate maintains in-memory
  blocked sets (client-ids, roles) via its own observer subscription
  to `lasa.space.client.blocked.*` / `lasa.space.role.blocked.*`;
  the connect path checks memory, never the store.
- **Enforcement point 1 — admission:** refusal at authentication,
  **before** `Admit` and its eviction logic — the incumbent KickGate
  lesson kept: a blocked id's reconnect can never evict a live
  session.
- **Enforcement point 2 — live sever:** a state change blocking a
  connected client severs its sessions shell-side, flowing through
  the normal funnel; the embedder sees only `EntityLeft`. No SPI
  surface.
- **Recorded honesty note:** under unticketed v0, client ids are
  self-asserted, so block-by-id shares evict-old's id-weakness
  (documented together); the mechanism gains real teeth with
  tickets. *(2026-08-13: the collision rules around `Admit` are
  superseded by `id-capabilities.md` — newest-wins on
  same-capability contests in both modes, refuse on cross-client
  entity collision in ticketed mode, claims at the engine;
  implementation pending. The gate-before-Admit ordering and this
  honesty note stand.)*
- Key vocabulary from `profile/base` (F).

**§7 outcome — work items:** the gate in lasa's `server` (sets,
auth-time check, sever path) + tests (blocked-at-connect,
blocked-while-live, block-then-reconnect-cannot-evict).

**§7 CLOSED.**

## Build order (the agenda resolved — Phase B execution)

*Expanded into phased, sessionable todos with per-phase context
pointers and acceptance criteria in **`panaudia-server-build.md`** —
that file is the progress ledger; this section is the summary.*

All sections closed 2026-08-01. Work items gathered, dependency
order:

**Batch 1 — lasa** (SPI-breaking changes while the stub is the only
embedder): SPI `WriteFrame(sampleTS)` + `Config.SampleEpoch` +
anchor arithmetic + C3 invariant sentence (§1); the Depacketizer +
gap counters (§3); presence aggregator + presence out of the SPI
(§6); the moderation gate (§7); `profile/base` consumer vocabulary
(F). Stub and e2e updated alongside; all green under `-race`.

**Batch 2 — engine** (independent of batch 1, can interleave):
global-sample sink stamps (§1); loudness to the render pipeline,
post-gain (§2); `Source.Conceal` (§3). Suites + harnesses stay
green; PROVENANCE updated.

**Batch 3 — panaudia-server** (composes both): repo + module + main;
the lifecycle funnel with fatal-error rule (§5); the Backend adapter
(depacketizer-wrapped ingress, pose fan-out, `StartSink`, jitter
geometry config, head-frame rejection §4); presence feed (§6);
state observer → `profile/base` → engine mutators; defaults parity
test (F); e2e against the lasa reference client (two clients hear
each other, evict-old, teardown, conn-map ≡ engine-set invariant
§5); `AllocsPerRun` on both audio paths; chirp + pose harnesses
re-run through the full server as acceptance.
