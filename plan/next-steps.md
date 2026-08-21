# Next steps

*As of the 2026-07-24 session. Re-entry map, not a decision doc.*

## Update 2026-07-30 — Phase A built; optimization program open

A very full day; `plan/greenfield-standalone.md` carries the decisions,
the engine repo's `PROVENANCE.md` carries the six Layer-1 divergences.
In order: **ten spec errata folded** into `lasa-core.md` (list in
`implementation-plan.md` is now a fold note). **Engine design decided**
(recorded in greenfield-standalone.md §"Engine design decisions"):
metres/radians native, no-wall-clock (seq + sample index only —
protected), pose = source-side SPSC ring aligned to the jitter stream +
sink latest-wins slot, listener-position split in the encoder,
shape-(C) render-state boundary, channels via per-listener peer lists
(option D). **panaudia-engine built**: Layer 1 copied (commit fd76cc8)
+ fresh Layer 2 (`engine` package), all green under `-race`, budget
gate ~0.58 ms median on dev. **lasa D1 done**: fork tagged
`v0.8.4-lasa.1`, lasa requires it with no replace. **Erratum 11
resolved** (channel gain multiplies, attenuation overrides-when-set;
profile §4 rewritten; presence loudness pinned entity-gain-only).
**Accelerate AMX signal-corruption saga**: signal delivery mid-sgemm
corrupts results (OS bug, reported to Apple, standalone repro in
`gemm/accelerate-bug-repro/`); fixed by Paul's signal-shield idea
(pthread_sigmask around the call); Accelerate stays darwin default;
xsmm now also builds on darwin. **Optimization program (P0′–P3a)**:
capacity sweep + per-phase stats + zero-alloc guard landed; found and
fixed inherited hot-path allocations (variadic Contains) and lock
traffic (thread-unsafe sets); P1 chunked dispatch tried, measured
slower, rejected (comment in render.go); P3(a) shared-input reverb
with zero-weight dense masking, parity-exact. 200²-with-motion:
4.20 → 3.85 ms median, 0 allocs.

**Tomorrow (finishing the optimizations):** re-profile at 200² (the
denominator shrank — P7's true share); then decide/execute **P7**
(scalar Go pack loops → C/SIMD, the capacity lever, guarded by the
M-anchors) and **P3(b)** (batched reverb GEMM across listeners —
respect brgemm-scattered.md's reopen bar; the x86 re-ask harness
exists); opportunistic P4 (exact-zero silence gating) and P6
(clear-elimination, needs a staleness mask). Paul's prod-box runs
(budget + sweep, `-tags xsmm`) inform priorities. Beyond the program:
backport decision for the incumbent (the signal shield), then Phase B
(`panaudia-server`).

*P7 outcome (2026-07-31): tried and **reverted** — a valuable
negative result.* The 200² profile confirmed `AddInputDelayed` as the
largest working cost (~27% of CPU), and a full stage-1 was built
(row-indexed mixer writes, deferred per-bus batch, one-crossing C
pack kernel, bit-exact parity — including the discovery that Go
fuses the lerp into an FMA on arm64, so the C side must contract
identically). But the isolated microbenchmark refuted the premise:
**the scalar Go loop is ~8% faster than the same loop in C**
(31.9 µs vs 34.5 µs per 200 rows) — the pack is intrinsically
memory/dependency-bound, not a Go-codegen problem, and the sweep
moved only ~60 µs. Reverted to keep the copy clean. **The real P7 is
NEON/AVX SIMD**: the read index advances by 1−step per sample, so
runs between rare index slips vectorize as two overlapping loads +
ramping-frac FMLA — estimated 2–4× on the pack, ~0.4–0.5 ms at 200²,
at the price of losing bit-exactness (M-anchor tolerances become the
guard). Attempt only after the x86 prod-box sweep says the shape is
the same there; the batch/deferral scaffold can be recovered from
this session's history if wanted.

*Decision (Paul, 2026-07-31): the per-(pair, ear) fractional-delay
cost — the pipeline's largest single term (~1.6 ms of the 3.85 ms
frame at 200²) — is **accepted as the price of the acoustic model**:
near-field localisation under the bilateral renderer is vastly better
than the previous renderer, and that quality is the product.
Optimization of this term is welcome only in model-preserving forms
(SIMD, fewer pairs via channels/gating/clustering); model-cheapening
levers (shared per-pair delay, decode-side ITD) are off the table.*

## Update 2026-07-29 — direction change: greenfield rebuild decided

Step 6 (panaudia integration) was planned in full and then **not
taken**. The session drafted two competing plans and Paul chose the
greenfield path:

- **`panaudia-integration.md`** (NOT TAKEN, kept priced) — LASA front
  end inside existing panaudia: ten decisions D1–D10 covering the
  fork's replace-doesn't-propagate problem, the shared-port/ALPN clash
  (LASA's ALPN is also `moqt-16`), string-id↔UUID mapping, dual
  state-plane bridging, and the SetRotation-per-packet bouncer flood.
  Its two survey reports (panaudia backend contract + `core/moq`
  incumbent + cloud-mixer constraints) remain accurate references.
- **`greenfield-standalone.md`** (DECIDED) — three artifacts: `lasa`
  stays the protocol only (deliberately *not* "the reference
  implementation" — arm's length so partial conformance reads as a
  server fact, not a spec-repo defect); new repo **`panaudia-engine`**
  = the protocol-blind audio library (DSP packages **copied with
  tests** out of panaudia — succession, not fork: old panaudia is
  frozen, never migrated — plus a fresh ~1.2 kLOC orchestration
  keeping phase barriers / slot allocator / audio-thread-confined
  mutation / the encoder mutator surface, dropping the clustering
  vestiges); new repo **`panaudia-server`** = lasa + panaudia-engine +
  thin Backend adapter. The future cloud implementation is a later
  third composition over the same two libraries.
- Naming decided against `lasa-render`/`lasa-server` (conformance
  implication; keeps "lasa" meaning the protocol) and against
  `panaudia-core` ("core" overloaded by old repo's `core/` tree and
  `lasa-core.md`).
- Key survey facts: the mix already runs network-free
  (`budget_m8_test.go`); the DSP stack (~6.5 kLOC) imports no
  state/network packages, cgo fenced in four leaves; NFC bilateral is
  post-M9, not mid-flight; 48 kHz/240 frames is a hard invariant;
  `lasa/server` is raw-QUIC-only today (WebTransport is the gap on
  the path to browsers).
- Settled seam choices: ingest both `WriteOpus`+`WritePCM`; clock
  `Process()` primary with `Run()` convenience; D1 resolved by
  rewriting lasa's moqtransport imports to
  `github.com/panaudia/moqtransport` and dropping the `replace`.
  Open: SD2 policy surface, SD4 metering, SD5 subspaces, WT
  scheduling, parity inventory (I-A…I-E).

Next: Phase A — create `panaudia-engine` (copy Layer 1 with tests +
provenance note, write Layer 2), plus the lasa-repo import rewrite
(D1); gate = ported budget/soak tests green on dev + x86 prod box.
Then Phase B `panaudia-server` assembly, Phase C base-profile policy
depth (old step 7, now critical path), Phase D reach & parity
(WebTransport, TS client, tickets, parity inventory).

## Update 2026-07-28 — implementation planned, state engine designed

Refactor-structure session with the panaudia codebase in hand. Two new
docs carry the decisions:

- **`implementation-plan.md`** — LASA implemented in this repo as Go
  module `github.com/panaudia/lasa` (spec stays top-level, packages
  `ident`/`wire`/`connect`/`state`/`profile/base`/`server`/`client`);
  self-contained server behind an SPI, panaudia maps entity →
  `ConnectionHandler`; old front ends left in place, `core/moq`
  deleted when stable; work order 1–8; three recorded v0 deviations
  (global ambi order, evict-old unticketed admission — closed
  2026-08-13, newest-wins decided in `id-capabilities.md` —
  memory-only state) each with reopen triggers; two spec errata candidates
  (§6.2 echo-outside-subscription, §6.6 cursor-not-before-first-
  catch-up-frontier).
- **`state-engine-design.md`** — actor-owned writer + bounded
  broadcast log + compacted map; eviction-on-lag as the
  protocol-native overflow policy; two-lane delivery interface
  (attach-time gate + cut, pull live lane, `ErrLagged`); five frontier
  invariants as named simulation tests; smaller defaults (rules inside
  engine, two matcher types, buffer-and-replay `clientstore`,
  persistence seam only).

Same day, later: **adversarial review pass on the state engine design**
(two independent reviews: spec-consistency + edge-case hunt; all
surviving findings folded into the docs). Headline outcomes:
`state-engine-design.md` revised — connection registration + actor-
enforced write path, generation-gated writes (no writes from a swept
generation), group-as-one-actor-event + group size cap, log sizing
invariant + resync backoff (anti-thrash), lag-eviction firewalled from
connection lifecycle, subscription keyed by client id with displacing
attach, crypto-random epochs + space-id-keyed cursors, **gate 4 via
effective-rules-hash comparison** (lazy, forgetting-is-safe cache;
covers role-def changes while disconnected and different-ticket
reconnects), inclusive frontier semantics pinned, invariants grown
5 → 12 (F1–F7, L1–L5), test scenarios expanded to match. Reference
client strategy corrected to **transient tombstones** per the settled
preference in `state-reconnect-and-gc.md` (draft had silently reversed
it). Errata list in `implementation-plan.md` grown to six, plus a
recorded future feature: per-client `debug` diagnostics track.

Later the same session, implementation began (steps 1–4 done, all
green under `-race`): go.mod + CI; the Eyevinn fork pushed as
`github.com/panaudia/moqtransport` branch `lasa-setup-params` (three
CLIENT_SETUP API changes + deterministic rejection, unit + real-QUIC
e2e tests); `ident` + `wire` with golden vectors (errata 7–9 found:
endianness gap — little-endian confirmed by Paul, set-hash minimal
varints, offset-bits MBZ); `connect` (embedded schemas, EdDSA-only
tickets, §4.2 resolution; erratum 10: duplicate-id wording); `state` +
`state/clientstore` per `state-engine-design.md` — every F/L invariant
has a named test, transient-tombstone reference client strategy,
sim tests run both halves through the wire codec. The **server SPI is
designed**: `server-spi.md` (raw synchronous ingress, push sinks,
polled presence, no space id on the SPI surface).

Step 5 is **done** (same session): `profile/base` (v0 subset: marker,
name, attrs/signed-attrs, sweep scope), the `server` shell (CLIENT_SETUP
auth → Resolve → Admit → engine; base-profile admission writes + sweep;
raw synchronous datagram ingress via the fork's fast path; sink render
lifecycle with alloc-free WriteFrame; presence gen/index/keyframe
machinery; state two-lane delivery; transport-ctx teardown for L5), the
reference `client` (config-in-setup dial, entity publishers, SyncState
against clientstore with backoff resync, fast-path sink/presence
routing), the stub backend, and nine e2e tests over real QUIC including
the §4.6 rejection code at the client, reconnect-with-delta, departure
sweep, and the hot-path allocation check. Fork grew four more commits,
all upstreamable: datagram receive fast path + pooled send buffer,
stream-object backpressure (drop → block; correctness for state),
RemoteTrack.TrackAlias(), atomic subgroupCount (data race).

Post-step-5 structural review (same session): decomposition seams for
the future gateway/mixer/state split analysed and recorded in
**`decomposition-seams.md`** — the state engine is already
network-service-shaped (no code changes; two wrapper obligations noted:
lease-sweep, ordered-stream for sweep-before-replacement; shapes A/B
kept open), and the presence machinery was **extracted into the
transport-free `presence` package** (Author/Entry/Sender, Tick-driven,
unit-tested) so a future composer reuses it — the single-author
constraint of the presence wire format is the recorded reason.

Next: step 6 — panaudia integration (new front end implementing the
SPI, entity → ConnectionHandler through the departure funnel, both MoQ
servers side by side); step 7 base-profile depth; step 8 migration.
*(Superseded 2026-07-29 — see the update above: steps 6–8 replaced by
the greenfield rebuild.)*

## Update 2026-07-27 — rotation deferred, cheap pins landed

Session goal: pull together a new lasa2 draft. Decisions taken:

- **Head rotation deferred as future work.** Both
  `head-rotation-and-sink-formats.md` and
  `rotation-correction-sidecar.md` are now DEFERRED, contents preserved
  as the future phase's trade space. v1 sink menu: **`binaural`,
  `ambi2`, `ambi3`, all mandatory**. Items 1 and 2 below (loop
  profiling, listening tests) move to the future rotation phase.
- **Item 3 cleared** — the packet opens are pinned in
  `audio-packet-structure.md`: sink ts = u64 unix-epoch µs, first
  sample of frame, ~1/s cadence (§5); ambi arrangement = all-uncoupled
  provisionally, listening evaluation as reopen condition (§7);
  group-ids session-scoped, reconnect restarts at 0 (new §9).

- **Item 8 cleared — connection-scoped key lifetime resolved**: no core
  mechanism; "connection-scoped" is profile prose and the sweep is the
  server issuing ordinary clears. Recorded with prices and reopen
  trigger in `core-profile-split.md` §Resolved 2026-07-27. The base
  profile owes a SHOULD on marker-first sweep ordering.

- **`connection-and-auth.md` is complete** (item 4 cleared, all
  decided 2026-07-27): Connection Config rides CLIENT_SETUP as a
  custom setup parameter (rejected alternatives + Eyevinn v0.8.2 fork
  notes recorded); unticketed = fragile mode, live-collision rejects
  (superseded 2026-08-13: newest-wins in both modes, dev-mode
  framing, ticketed-XOR-unticketed — `id-capabilities.md`),
  `audience` only; `client_id` replaces jti-as-identity (schemas
  updated & validated); setups instantiate ticket entities, ad-hoc
  entities need admin/presenter, both may coexist (`oneOf`→`anyOf`);
  sink-subscribe = own entities + admin-any (matrix recorded, admin
  extra grant in `roles.md`); sink lifecycle = subscription is the
  only signal; reconnect is not a protocol concept, ticketed collision
  evicts, moderation survives via scope split, ticket reuse until
  `exp`; kick/block = the block keys do double duty; rejection error
  cases listed, surfacing via session-close code+reason on both
  transports (codes u32, app range).

- **Item 5 cleared — `presence` format pinned** in
  `presence-format.md`: delta/keyframe on the one shared track,
  generation-guarded implicit indexes (server alias scoped to the
  track alone), MTU-chunked rosters, §4 pose, log-u8 loudness
  (−0.5 dB steps), departure = keyframe absence, rates as server
  policy.

- **Item 6 cleared — `state` wire encoding pinned** in
  `state-wire-encoding.md`: subscribe rides MoQ SUBSCRIBE KVP
  (canonical set + cursor + set-hash), typed messages one per MoQ
  object (`set`/`clear`/`group`/`frontier`/`begin`), seq
  downstream-only, epoch 8 opaque bytes, **64 KiB value cap**
  (oversize = silent drop), live/catch-up as subgroups 0/1 of a fresh
  group per subscription (single-stream fallback recorded).

- **Roles/state-keys reconciliation done.** `lasa-state-keys.md` is
  now titled base-profile prose: every "certain roles" replaced with
  explicit roles, `priority` producer-adjustable (producer wins),
  render.\* and channel overrides set/clear with absence-means-default
  (gain 1.0, attenuation 2.0, size 0.0, directivity 0.0), blocked
  values "≥ 0" with dual-duty cross-ref, marker-first sweep SHOULD
  added, `{other-entity-id}` normalised, typos fixed. `roles.md`:
  producer render keys gain `clear`, presenter cross-ref. The
  working-notes tail of `commands-as-state-ops.md` replaced with a
  supersession note.

- **Final pre-rewrite review done** (self + independent adversarial
  pass): 16 findings, all resolved and recorded in
  `review-resolutions.md` — headline items: identifier grammar
  `[a-z0-9-]{1,128}` / keys ≤512 B, role scopes match
  exact-or-dot-extended, admin unblockable, ticket `permissions`
  claim (general prefix grants; roles are compact aliases), audience
  reads all `lasa.`, space id = aud (verified), default channel
  `main`, dof enforced at ingest, head-frame entities omitted from
  presence, attrs flatten objects-only-no-lists. Schemas updated and
  validated (21 cases).

- **Item 7 done — the spec is written**, as two documents per the
  structure decision (core + base profile companion, schemas as
  files): **`lasa-core.md`** (identifiers, tracks, connection/auth,
  audio formats, presence, state + wire, security, wire constants)
  and **`lasa-base-profile.md`** (values/attrs, roles + permissions,
  lifetimes, channels, moderation, full key vocabulary). `lasa2.md`
  is now a pointer stub. Wire constants pinned in the writing:
  Connection Config SETUP param key `0x1A5A1`, state-subscribe
  SUBSCRIBE param key `0x1A5A3`, error codes `0x1A5A00`–`08`,
  subscribe-KVP layout.

## What's next (post-spec)

1. Paul reviews both drafts end to end.
2. Commit the working tree (nothing is in git yet — the whole tree is
   this design cycle's work).
3. Implementation: the Eyevinn v0.8.2 fork (three API changes:
   client `WithSetupParameters`, server setup-params
   accessor/callback, reject-before-SERVER_SETUP), then the panaudia
   auth refactor from first-SUBSCRIBE to CLIENT_SETUP.
4. The deferred rotation phase re-enters via
   `head-rotation-and-sink-formats.md` /
   `rotation-correction-sidecar.md` when the listening rig runs
   (profile the loop first — `audio-packet-structure.md` §8).

## Where things stand

The audio-track phase is complete and recorded:

- `track-naming.md` — merged source/pose track, typed tuple namespaces
  with source/sink direction, per-client state tracks, format-in-name,
  `mono-object`, shared presence. **Closed.**
- `audio-packet-structure.md` — MoQ ids as seq (group-per-frame), flags
  byte, 18-byte pose, occasional sink timestamps, uniform redundancy
  framing, multistream ambi, pose fast path & three-stage rotation
  latency picture. **Closed except listed open items.**
- `opus-frame-size.md` — review outcome appended: uniform 5ms confirmed,
  asymmetric 10ms-uplink rejected on deployment-envelope grounds.
  **Closed** (reopen trigger recorded).
- `head-rotation-and-sink-formats.md` — **OPEN**, the main pending
  decision. Lean: bilateral-2 as workhorse sink format. Gated on
  listening tests.
- **Psychoacoustics literature survey done 2026-07-25** — recorded in
  `rotation-correction-sidecar.md` §What the literature says, with a
  full reference list at the end of that doc. Headline: no auditory
  saccadic suppression, but a real ~5× acuity loss during rotation
  (Honda et al. 2016) that **does not scale with rotation speed**; no
  published evidence that spectral or codec artifacts are masked during
  motion (a genuine gap, and the scheme's largest recorded risk).
  Knock-on corrections landed in `head-rotation-and-sink-formats.md`
  (velocity assumptions; `ambi2` rotation-stability caveat; literature
  backing for the dilemma). Also carries **§The acceptable-latency
  budget** — where the 40–80 ms loop sits against measured thresholds,
  and the finding that **detection thresholds are the wrong dependent
  variable**: score externalization/plausibility against a latency
  sweep instead. That condition is worth running **even if the sidecar
  fails**, as the acceptability curve is unmapped in the literature.
- `rotation-correction-sidecar.md` — **OPEN** (added 2026-07-25): server
  binaural + low-rate FOA dipole sidecar + differential client rotation
  correction. Lean: differential over layer-split. Offline listening rig
  planned for the week of 2026-07-27; four evaluations listed in the
  doc. If it lands, it reshapes the sink-format menu (weakens mandatory
  `bilateral2`).

Earlier the same day (pre-design-session): key separators dots-not-
slashes, ops renamed `set`/`clear`, role names aligned in
`ticket.schema.json`, `connection.schema.json` oneOf fixed with
entities/setups mutually exclusive (validated against all eight field
combinations).

Nothing is committed to git yet — the whole working tree is today's
work.

## Next, roughly in order

1. **Profile the rotation loop** — promoted to first 2026-07-25. Get a
   per-segment latency budget (pose sample → uplink → render →
   downlink → client buffer → playout). Cheap, needs no listening rig,
   and **no listening result can invalidate it**. It settles by
   arithmetic which rotation-latency lever is worth building: the pose
   fast path is free and universal but its ceiling sits on the uplink
   half of the loop, so if the client buffer dominates the leverage is
   all downstream. Method and caveats: `audio-packet-structure.md` §8
   ("The fast path's ceiling"). Kentgens et al. 2025 give a reusable
   black-box motion-to-sound method. Independent of item 2 — they can
   run in parallel. Note the deployment regime (LAN / moderate WAN /
   high-jitter) is not pinned in any doc and largely decides this.
2. **Listening tests** — the gate for the sink-format menu.
   `head-rotation-and-sink-formats.md` lists the four evaluations in
   leverage order (bilateral-2 vs head-centered first). Its "If
   bilateral lands" section is the follow-up work list. The
   sidecar rig (`rotation-correction-sidecar.md`) can share these
   sessions — it is offline, no network needed.
3. **Pin the remaining packet opens** (`audio-packet-structure.md`
   §Open items): sink ts width/epoch/instant/cadence; ambi arrangement
   (uncoupled vs projection — loses urgency if bilateral lands);
   id continuity across reconnect.
4. **Sink-track policy items** (parked in the naming phase): render
   lifecycle on subscribe/unsubscribe, flavour-switch overlap,
   concurrent-flavours policy, sink-subscribe authorization ("listening
   through e's ears" — a real privacy/moderation question). Most belong
   in the `connection-and-auth.md` expansion.
5. **`presence` packet format** — shape sketched in
   `audio-packet-structure.md` §Deferred (compact index + in-band
   roster, changed-only + periodic refresh).
6. **`state` wire encoding** — ops, subscribe message, frontier;
   functionally specified by `state-reconnect-and-gc.md`, wire shape
   open.
7. **Rewrite `lasa2.md`** against `track-naming.md` +
   `audio-packet-structure.md` (both list the stale bits). Include
   while there: "the sink is the subscription" wording sharpening
   (recorded in track-naming), and an explicit statement that LASA uses
   MoQ as a session/subscription/identity layer rather than a caching
   fabric (only `presence` — and uplinks, in a future relay topology —
   are genuinely shared cacheable tracks).
8. **Parked from before this session**: `connection-and-auth.md` is
   still a stub; and the core/profile split's open boundary question —
   whether connection-scoped key lifetime is a core mechanism or
   profile prose — must be resolved before the base profile is written
   (`core-profile-split.md` §Parked).
