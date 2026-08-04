# panaudia-server: build plan

*Drafted 2026-08-01. The execution companion to
`panaudia-server-design.md` (the why — all §-references below point
there). Each phase is sized for one focused session, lists exactly
the context a fresh session needs, and ends with the repo(s) green
under `-race`. Paul commits between phases. Check boxes as phases
land; this file is the cross-session progress ledger.*

**Standing rules:** hot-path contract (no alloc/lock/block on audio
paths); one clock (§1); stop-and-discuss on surprises; PROVENANCE.md
for any engine Layer-1 divergence; Paul's git.

## Preconditions

- [x] Paul: commit outstanding engine work (latency/pose harnesses,
      `LatencySamples`, vestige removal — PROVENANCE #5)
- [x] Paul: create the `panaudia-server` repo (needed from S1)

## Batch L — lasa (SPI-breaking window: the stub is the only embedder)

### L1 — the clock changes (§1) — DONE 2026-08-01
- [x] SPI: `SinkWriter.WriteFrame(frame []byte, sampleTS uint64)`
- [x] `server.Config` gains `SampleEpoch func(sampleTS uint64) uint64`
      (µs); shell egress computes §5.1 anchors from it
      (`epoch₀µs + sampleTS·10⁶/48000`); no other wall-clock use
- [x] C3 invariant sentence into `server-spi.md`; §5.1 note queued
      for next spec touch
- [x] stub backend + e2e + hotpath allocation tests updated
- Context: design §1; `server/spi.go`, `server/sink.go`,
  `plan/server-spi.md`
- Accept: lasa `-race` green; hotpath alloc checks unchanged

### L2 — the Depacketizer (§3) — DONE 2026-08-01
- [x] `server` package utility (not an SPI change): seq tracking,
      flag validation/discard, `ParseMonoObject`, reorder,
      hold-on-gap, redundancy substitution, arrival-driven release
      (packet ≥ n+8 without covering copy ⇒ `Lost(n)`), stall/resume
      flush; emits `Frame(seq, pose, opus)` / `Lost(seq)` in stream
      order
- [x] gap-length counters (diagnostic surface; future k evidence)
- [x] table-driven tests: in-order fast path (zero held state,
      zero alloc), single loss + recovery at k=1..7, burst ≤ k fully
      recovered, burst > k partial, frame+carrier both lost,
      plain reorder, duplicates, stall then seq-jump resume
- Context: design §3; `wire/audio.go` (§5.1 layouts), `server/conn.go`
- Accept: tests green; clean-path `AllocsPerRun` = 0

### L3 — the presence aggregator (§6) — DONE 2026-08-01
- [x] grow `presence`: collector (per-entity latest-wins slots,
      `Update/Remove`, loudness getter registration), cadence
      sampler (pacer — schedules, mints no time values),
      delta/keyframe builder (reuse existing wire), pubsub
      (`Subscribe/Unsubscribe`, aggregated cadence-rate stream)
- [x] shell's presence-track machinery becomes subscriber #1
- [x] SPI: remove `Backend.Presence()` + `PresenceEntry`; stub +
      e2e presence tests move to feeding the collector
- Context: design §6; `presence/`, `server/server.go` presence loop
- Accept: e2e presence green via the new path; collector write path
  alloc-free

### L4 — the moderation gate (§7) — DONE 2026-08-01
- [x] gate in `server`: derived blocked sets (client-ids, roles) via
      its own state-engine observer; auth-time refusal ordered
      BEFORE `Admit`; live sever through the normal shell funnel
      (embedder sees only `EntityLeft`)
- [x] tests: blocked-at-connect; blocked-while-live severed;
      block-then-reconnect cannot evict a live session
- Context: design §7 + §5; `state/` observer API, `server/server.go`
  admission path
- Accept: tests green; unticketed id-weakness note recorded beside
  the evict-old deviation

### L5 — `profile/base` consumer vocabulary (F) — DONE 2026-08-01
- [x] typed parse/build + validation + defaults-as-code for:
      `render.gain/attenuation/size/directivity`, `sources.*`/
      `sinks.*`, `channel.muted/gain/attenuation`,
      `space.entity.muted`, `client.blocked`/`role.blocked`,
      `mute.*`/`solo.*`, `dof`, `frame`
- [x] channel composition semantics as code (gain multiplies;
      attenuation overrides-when-set — profile §4)
- Note: compose.go pins one under-specified §4 reading (mixed
  set/unset shared gains: unset contributes the identity 1.0 as a
  candidate, highest wins — most-audible-wins); raised with Paul
  2026-08-01
- Context: design F + erratum-11 resolution; `lasa-base-profile.md`
  §§4–6, `profile/base/base.go`
- Accept: table-driven tests against the profile text

## Batch E — engine (independent of L; interleave freely)

### E1 — clock, loudness, concealment (§1, §2, §3) — DONE 2026-08-01
- [x] `Sink` stamps global sample time ((tick−1)×240), not per-sink
      counts (§1 C4)
- [x] loudness moves to the render pipeline: ingest EMA removed;
      `phaseIn` computes RMS(frame)×gain → EMA (named time-constant)
      → atomic; `Source.Loudness()` signature unchanged (§2)
- [x] `Source.Conceal(seq)`: decoder PLC into the jitter stream,
      pose-ring sample accounting preserved (§3)
- [x] tests: post-gain loudness; Conceal emits a frame's samples and
      keeps pose/audio coherence; harnesses re-checked (global
      stamps stay contiguous; chirp/pose numbers re-logged)
- Context: design §§1–3 work items; `engine/sink.go`, `source.go`,
  `render.go`; hraban opus PLC API
- Accept: full engine suite + harnesses + alloc guard `-race` green

## Batch S — panaudia-server (composes L + E)

### S1 — scaffold & clock wiring — DONE 2026-08-01
- [x] repo, module `github.com/panaudia/panaudia-server`, deps
      (lasa, panaudia-engine — both replace-free), config/env
      surface, `main` skeleton
- [x] ticker owned here; epoch₀ read at first tick; `SampleEpoch`
      closure handed to the shell (§1 C5)
- Note: go.mod carries TEMPORARY `replace` directives to the sibling
  checkouts — lasa/engine local commits are ahead of origin; flip to
  pinned pseudo-versions once Paul pushes both
- Accept: starts, ticks, serves an empty space — boot smoke test
  (`main_test.go`) boots the composed server, waits for render
  ticks, connects a real lasa client, and sees the probe entity's
  admission marker in the synced client store
### S2 — the lifecycle funnel (§5) — DONE 2026-08-01
- [x] one serialized funnel: `Admit` (evict-old wait), `EntityJoined`
      → engine adds, `EntityLeft` → engine removes; conn-map mutated
      only here; engine errors fatal to the connection
- [x] e2e with the lasa reference client: connect / disconnect /
      evict-old sequences; conn-map ≡ engine-entity-set invariant
- Enablers added: lasa `Server.SeverClient` (the evict trigger half
  of the Admit-may-block contract; severMatching gained a cause
  param); engine `Mixer.Entities()` (invariant assertion surface)
- Composition findings fixed in engine (found by -race here, both
  Layer 2): `Close` now waits for an active `Run` to return before
  draining audio-thread state (the documented contract was
  undischargeable by a Run-driven host); the default HRTF delay-model
  install in `New` is once-guarded (process-global params vs a
  rendering mixer's audio thread)
- Known gap (recorded in Admit's comment): zero-entity sessions hold
  no conn-map records, so they are neither tracked nor evictable —
  nothing engine/entity-key-side to order against
- Accept: lifecycle e2e green (connect/disconnect, entity-id
  collision handoff, same-client reconnect displacement, capacity
  refusal — each settling through the two-sided invariant)
### S3 — the audio path — DONE 2026-08-01
- [x] ingress: Depacketizer-wrapped handler → pose fan-out (source
      ring via `WriteOpus`, sink slot via `SetPose`, dof enforcement,
      presence collector) → `WriteOpus`/`Conceal`
- [x] `StartSink` → `AddSink` with the `FrameWriter`→`SinkWriter`
      bridge; jitter geometry W=5 ms; head-frame reject at admission
      (§4); format negotiation (binaural v0)
- [x] e2e: two clients hear each other (tone detection);
      `AllocsPerRun` = 0 on both audio paths
- Details: entityRec IS the Receiver (dof enforced at ingest per
  profile §6 — dof 3 pins position to home, dof 0 discards pose;
  the enforced pose feeds ring + sink slot + presence slot alike);
  Lost conceals only when audioLikely; head-frame refusal uses
  ErrCodeMalformedConfig (no dedicated code exists — revisit with a
  spec erratum if head-frame support ever negotiates); sinks are
  each entity's OWN rendered perspective (binding: own-or-SinkAll)
- Gap RESOLVED (Paul chose option A, 2026-08-01): lasa's
  sinkManager gained `dropEntity` — entity departure stops the
  entity's renders and ends subscribers' tracks with SUBSCRIBE_DONE
  track-ended; re-subscribe is the recovery (subscription stays the
  only lifecycle signal). Called from teardown AFTER EntityLeft so
  no interleaving leaves a stale entry (post-EntityLeft subscribes
  refuse; pre-delete attachees still get their DONE). Unit-tested
  with a fake publisher (stop, DONE code, idempotence, fresh render
  on return). The funnel's idempotent RemoveSink at EntityLeft
  stays as the engine-side backstop.
- Accept: audio e2e green (440 Hz Opus through speaker →
  depacketizer → jitter → spatial render → binaural encode →
  listener's own sink → decode, energy detected; ingress and egress
  bridge both 0 allocs/op)
### S3.5 — tickets (pulled forward from deferred, 2026-08-01) — DONE 2026-08-04
- Design settled with Paul 2026-08-04:
  1. Issuance is out of scope by deliberate separation of concerns.
     The server is verification-only: its config carries a SINGLE
     Ed25519 PUBLIC key; the private key never touches the server.
     Mint tooling (CLI, client libraries in other languages) comes
     later — tests mint in-process via `connect.Mint`.
  2. `exp` gates admission only — a live session outlives its
     ticket's validity (matches core §4.3/§4.5: exp bounds reuse,
     not tenure). No mid-session sever timer.
  3. No refresh story for these tokens.
  4. `SinkAll` is granted by the `admin` and `producer` roles.
  5. No key rotation.
- [x] roles→rules expansion in `profile/base` (`roles.go`): the §2
      tables as code; `RulesFor`, `AdHocPermitted` (admin,
      presenter), `SinkAll` (admin, producer)
- [x] server: `PANAUDIA_TICKET_KEY` env surface (one public key);
      `RulesFor`/`AdHocPermitted`/`SinkAll` wired to `profile/base`;
      `AllowUnticketed` explicit (`PANAUDIA_ALLOW_UNTICKETED`,
      default true iff no ticket key)
- Enablers (each a spec edit folded in place + code):
  - core §6.4 gained `*`-segment scopes (one wildcard segment = one
    identifier segment) — moderator's any-entity dof and producer's
    any-entity keys are inexpressible as literal prefixes since
    rules are fixed at Register but entities keep arriving. Chosen
    over predicate-rules with Paul (option A, 2026-08-04). Legal in
    role expansion only; ticket `permissions` stay literal
    (schema unchanged). `ident.ScopeMatches` fast-path preserved.
  - base profile §2: producer writes pinned as any-entity (the
    mixing-desk role) and producer gained the subscribe-to-any-sink
    grant; admin's "two grants outside the roles" reduced to one
    (unblockable), sink-any now arriving via the union.
  - `state.Rules` gained `App` (application-space grant — every key
    outside `lasa.`): §2 "unticketed spaces are unrestricted over
    application space" is otherwise inexpressible (positive prefixes
    cannot say "everything but lasa."). Unticketed → App=true;
    ticketed reach app space via permissions only. Read-visibility
    hash carries a `\x00` sentinel (illegal in keys) when App set.
  - `server.Config` gained the `SinkAll func(roles) bool` hook
    (mirror of `AdHocPermitted`); the admission binding now carries
    it.
- Why now: evict-old and block-by-client-id trust a self-asserted
  id until tickets sign it (the recorded id-weakness notes); S5's
  block/kick e2e only tests real authority if blocked-key writes
  need a producer/admin role
- Accept MET: e2e with `AllowUnticketed: false` (`ticket_test.go`) —
  unticketed refused; ticketed setups admit; ad-hoc refused for
  audience, admitted for presenter; audience `lasa.space.*` write
  silently filtered while producer's lands (same-connection ordering
  proves the drop); audience refused another entity's sink while
  producer subscribes cross-connection (the first real SinkAll use).
  Both repos `-race` green; profile role tables unit-tested against
  the §2 text.

### S4 — state wiring (Phase C pulled in) — DONE 2026-08-04
- [x] observer subscription → `profile/base` parse → engine mutators
      (render params, channels, entity mute); defaults parity test
      (profile defaults ≡ `engine.DefaultRenderParams()`)
- Built: `wiring.go` (stateWiring — the profile consumer). Derived
  state is the heart: the engine refuses params for entities it
  doesn't hold and forgets them at departure, while ops arrive
  before EntityJoined and space-scoped keys outlive entities — so
  every mutator call is "apply if live, re-apply at join". Entry
  lifecycle is marker-driven (§6 ignore-while-absent; marker-first
  sweep drops the entry before its subtree clears arrive); the
  funnel's EntityJoined flushes derived params + surviving space
  mute after AddSource. Channel table composes the three per-channel
  keys into engine.ChannelParams (unset attenuation → profile
  default: the engine cannot represent no-override until Phase C's
  per-pair record). Dof events → rec.dof (live moderator changes).
  PersonalMute/Solo parsed but unwired — no engine mechanism until
  the Phase C pair record; Frame config-only in v0; blocked keys are
  the gate's. Invalid values drop-and-count (S6 stats surface).
- Enabler (lasa): `base.AdmissionGroup` now materializes the
  config-derived vocabulary — EFFECTIVE channel memberships
  (omitted → main, explicit empty → no keys; §6 sentence added:
  under a live marker the key set IS the membership), set render
  fields, priority, dof. Entity config reaches the engine through
  the ONE state path; no dual application.
- Finding (engine, by level-measurement — engine is CORRECT, the
  first test's expectations weren't): render.gain is POWER-domain
  (ambisonic gainFactor = √gain) and every sink output passes a
  compressor/limiter (−18 dB threshold, 2.5:1, 6 dB knee —
  inout.StereoCompressorLimiter, Layer 1). Measured ratios matched
  √gain × compressor slope to three digits once identified. The e2e
  drives a −25 dBFS tone to sit below the knee and asserts the
  linear regime. Gain-domain RESOLVED 2026-08-04 (Paul chose
  amplitude): the incumbent's √gain also disagreed with the
  render-pipeline loudness meter (RMS × gain), so power-domain was
  internally inconsistent, not just unconventional. Engine encoder
  now applies gain linearly (PROVENANCE #9); profile §6 pins
  "amplitude multiplier (0.5 = −6 dB)" with a §4 echo; the e2e
  asserts gain 0.25 → ×0.25 level. Default gain 1.0 bit-identical,
  so latency baselines are untouched.
- Accept MET: `TestStateDrivenRender` (ticketed producer+moderator
  console, zero-entity) — gain 0.25 halves the level and clear
  restores; space mute silences and clear restores; channel
  exclusion (speaker main→stage) silences and the listener's sink
  joining stage restores. `TestDefaultsParity` pins profile ≡
  engine ≡ wiring defaults. Both repos `-race` green.
### S5 — presence + moderation e2e — DONE 2026-08-04
- [x] presence feed live (collector + loudness getters); client
      receives presence with correct poses/loudness
- [x] block/kick e2e via state writes through the gate
- No production code needed — S2/S3 wired the collector (Register +
  loudness getter at join, UpdatePose from ingress, Remove in
  teardown) and L4 the gate; S5 is the composed proof. Tests:
  `TestPresenceFeed` — keyframes on the reference client carry both
  entities; the speaker's record shows its published pose (Y=1) and
  the ENGINE's post-gain loudness (0.1-amp sine ≈ −23 dBFS, asserted
  in (−45, −8) dB — the composition lasa's stub-fed e2e cannot
  check); the silent listener reads home pose + LoudnessSilent.
  `TestBlockKickE2E` — a moderator console's
  `lasa.space.client.blocked` write severs the live target through
  the normal funnel (both invariant halves drain), the blocked
  reconnect is refused with §4.6 code 0x1A5A06, and clearing the key
  readmits. Suite `-race` green.
- Accept MET: both observable from the reference client
### S6 — acceptance instruments — DONE 2026-08-04
- [x] chirp + pose harnesses ported to run through server + client
      loop (mouth-to-ear minus network); numbers logged as the
      server baseline
- [x] capacity smoke at 50×50 through the full server
- [x] minimal stats/health surface (engine Stats, LatencySamples,
      depacketizer gap counters)
- Stats surface (`stats.go`): `app.stats()` snapshot — engine
  ProcessStats + per-entity {LatencySamples, DepacketizerStats} —
  consumed by the harnesses and logged by the binary every minute
  while entities are live (`statsLoop`); an HTTP wrapper can reuse
  the same snapshot when a deployment wants one.
- Baselines now live in **`latency-baseline.md`** (the living
  results doc: model, dated rows, re-run + client-debugging recipe;
  added 2026-08-04 at Paul's request). Snapshot at S6 close
  (dev Mac, loopback, `harness_test.go`, skip under -short):
  - mouth-to-ear chirp: **30.2 ms** = 11.0 ms jitter fill + 19.2 ms
    residual (engine-only was 18.7 = 11.0 + 7.7; the delta is the
    client-side opus pass, transport, real-clock tick alignment).
    400/400 frames delivered, zero loss.
  - sink-pose (head-turn) latency: **3 frames = 15 ms** send-to-flip
    through datagram → latest-wins slot → render → sink flight.
  - source pose/audio coherence: skew **−2.5 ms** (engine tripwire
    ±15 ms; server allows ±25 for real-clock feed jitter) — the
    stream-speed identity survives the composed path.
- Capacity smoke (`capacity_test.go`): 50 clients × own sinks, every
  pair audible, real cadence — 400 ticks in 2.000 s (real time
  held), per-tick render 1.08 ms of the 5 ms budget, 20 215 frames
  ingested zero-loss, all 50 torn down through the funnel to empty.
  Timing gates skip under -race (engine's raceEnabled convention,
  mirrored as _test files); the 50-client concurrency still runs.
- Accept MET: **Phase B milestone — a native LASA client round-trip
  on a clean server binary, instrumented.** Suite green plain and
  `-race`.

## Post-milestone

### P1 — the pair record (Phase C completion) — DONE 2026-08-04
- [x] engine Layer 2 (`channels.go`): the audibility matrix grown to
      a full pair record — `pairGain`/`pairAttExp` slot matrices
      resolved per (source, listener) in `recomputeAudible` (§4:
      highest channel gain wins, unset = identity; lowest SET
      attenuation replaces the source's exponent, nil = no override —
      `ChannelParams.Attenuation` is now `*float64`)
- [x] personal mute/solo in the same resolution, id-keyed
      (`SetPersonalMute`/`SetPersonalSolo`): semantics decided with
      Paul 2026-08-04 — mute is BIDIRECTIONAL (either side muting
      silences the pair both ways; the incumbent's behaviour, kept as
      the desirable one), solo restricts the listener to its set and
      wins over personal mutes, space moderation mute above all (the
      incumbent's solo-bypasses-space-mute ordering is retired — its
      encoder `Mutes`/`Solos` sets stay empty). Owner's entries die
      at removeEntity (required: the marker-first sweep means the
      wiring never relays swept mute clears); entries targeting a
      departed id persist and re-bite on rejoin. §6 erratum pins all
      of this.
- [x] Layer 1 (PROVENANCE #10): listener encoders carry slot-indexed
      pair rows (`SetPairScalars`); the three weight sites read them —
      one indexed load per pair, nil = classic per-source values,
      copied suites unchanged
- [x] wiring: PersonalMute/PersonalSolo events → mixer mutators;
      ChannelAttenuation nil passes through end to end; parity test
      updated (unset = nil)
- Accept MET: engine unit tests (winner resolution incl. mixed
  set/unset and muted-channel exclusion; bidirectional mute; solo
  precedence; space-mute-beats-solo; per-pair audible ratio ~4 through
  the real render) + `TestPairSemanticsE2E` (channel gain ×2, flat
  distance law at 2 m, both mute directions, solo-beats-mute — all
  via real clients' state writes). All three repos `-race` green.

### P2 — size/directivity rendered (engine) — DONE 2026-08-04
- [x] all four items of `panaudia-engine/plan/size-and-directivity.md`
      (scope confirmed by Paul; occlusion out, spectral directivity
      deferred): broadband cardioid directivity (source forward from
      pose attitude, world-frame law), per-order spherical-cap spread
      weighting (energy renorm capped +6 dB — the tuning knob),
      surface-distance clamp (size = diffuseness; clamped d also
      gates NFC), size term in the wet/dry split
- No lasa or server code changes — S4's wiring already forwarded
  Size/Directivity; the server e2e (`TestPairSemanticsE2E`) now
  asserts the cardioid null at 90°, the half-directivity band, and
  the enveloping interior via producer state writes. Engine:
  PROVENANCE #11; alloc guard green; budget gate unchanged (547 µs
  median at 50×50). Addendum (same day, Paul's asks): neutral
  size/directivity are structurally GATED no-ops (per-pair extent
  gate in both render paths), and `TestExtentBudgetAtScale` prices
  the all-on 50×50 scene: +3.5% median (~19 µs) vs the unchanged
  neutral baseline, 15% tripwire. Open: the listening/tuning pass
  (Paul's ears).

### P3 — bus-order range opened to 2–5, order 1 dropped — DONE 2026-08-04
- Groundwork for the ambi-formats decision (Paul: "look at the order
  question first"). Audit found every stage below the validation line
  already order-generic (SH evaluators carry dedicated order-4/5
  paths; pack/GEMM sized by ChannelCount; convolver derives its bank
  from channel count; default HRTF set carries banks [2 3 4 5];
  extent spread table reaches 5; reverb/NFC order-independent). The
  only changes: `engine.New` validates 2–5 (order 1 deliberately
  unsupported — never wanted, and no order-1 HRTF bank exists),
  comments/README updated.
- `TestBinauralAllOrders`: full binaural path (SH encode → pack →
  GEMM → order's HRTF bank → convolution → opus) renders a
  hard-left tone with correct lateralisation at orders 2–5;
  `TestOrderOneRejected` pins the drop.
- `TestOrderBudgetSweep` (50-person budget scene, dev Mac):
  order 2 = 505 µs, 3 = 541 µs, 4 = 614 µs, 5 = 723 µs median —
  channels ×2.25 from order 3→5 costs only ×1.34 (GEMM efficiency;
  the non-order stages dominate). All far inside the 5 ms budget.
- Not a Layer-1 change (validation + docs only; no PROVENANCE entry).

### P4 — ambisonic sink formats — DONE 2026-08-05
- Design (settled with Paul across the order/roles/normalisation
  discussions; supersedes deviation 1):
  1. ONE order knob — `PANAUDIA_ORDER` (2–5). Offering DERIVED:
     `binaural` always, `ambi2` always, `ambi3` when order ≥ 3.
     lasa `Config.AmbiOrder` semantics become "ambi formats up to
     this order"; panaudia-server passes min(Order, 3).
  2. Ambi subscription is ROLE-GATED: admin + producer (the
     recording/broadcast/console constituency), a third binding
     grant mirroring SinkAll (`base.AmbiPermitted`; §2 sentence;
     `Config.AmbiFormats` hook; `Binding.AmbiFormats` checked for
     ambi tracks in MaySubscribe). Audience gets binaural only —
     the client-side-rotation path for listeners belongs to the
     deferred bilateral2/3 work. Consequence noted: manual ambi
     testing in a dev space needs a producer ticket → mint CLI
     priority nudge.
  3. Engine renders ONE classic (head-centred, WORLD-frame — no
     head rotation, decision 10; no NFC/ITD) render per
     ambi-subscribed entity at bus order; per-format TRUNCATION
     (exact for point sources — SH channels are independent; noted
     caveat: spread-weight energy normalisation is computed at bus
     order, so sized sources sit a hair quieter in truncated
     outputs) + one opus multistream encode per format (9/16
     uncoupled mono streams, ACN/SN3D per the pinned packet doc).
  4. Normalisation: N3D internal (verified already true by
     construction — orthonormal SH constants × SQRT_4_PI ≡ N3D;
     channel-0 weight 1.0, order-1 √3), SN3D at the sink boundary
     via the constant per-order divide √(2l+1), folded into the
     encode copy.
  5. Binaural and ambi renders may run SIMULTANEOUSLY per entity
     (engine grows a classic second encoder per ambi-tapped
     entity; the shell's (entity, format) render model already
     assumes this).
  6. `ambi4`/`ambi5` deferred-not-offered (bandwidth 25/36
     streams; wire consumers are head-centred so order genuinely
     pays there, but the listening path belongs to bilateral2/3 —
     additive later under format-in-name).
  7. Product note (Paul): offering order-5 *binaural* is worth it
     for positioning alone (+34% render cost) — recorded in
     head-rotation-and-sink-formats.md.
- [x] lasa: `Binding.AmbiFormats` checked in MaySubscribe for ambi
      tracks (grant does not widen entity reach); `Config.AmbiFormats`
      hook; `AmbiOrder` re-specced as "formats up to" with ≥
      offering; `base.AmbiPermitted` (admin/producer, travels with
      SinkAll); §2 producer-paragraph sentence; authz/e2e tests
      updated to the new policy
- [x] engine: `inout/multistream.go` (PROVENANCE #12 — cgo opus
      multistream, alloc-free encode) + `engine/ambisink.go`:
      AddAmbiSink/RemoveAmbiSink, one shared `NewClassicEncoder`
      per ambi-tapped entity (same uid: self-exclusion preserved;
      pair record + extent shared; world-frame — listener rotation
      never applied, position honoured), per-order truncation +
      N3D→SN3D (`1/√(2l+1)`) + interleave + multistream encode in
      the Out phase; full lifecycle (registry accounting, Close,
      removeEntity conditions). Tests: SN3D discriminator (X/W ≈ 1
      dead-ahead — √3 would mean N3D leaked), world-frame rule
      (yaw π must not rotate the field), binaural+ambi
      simultaneity, lifecycle, zero-alloc Process with ambi active
- [x] server: backend StartSink routes binaural/ambi2/ambi3 (the
      COW pose fan-out list replaced the single sink pointer —
      every attached render half gets ingress poses); EntityLeft
      removes ambi sinks as backstop; `PANAUDIA_AMBI_ORDER` env
      REMOVED (offering derived: AmbiOrder = min(Order, 3));
      `AmbiFormats: base.AmbiPermitted` wired
- Accept MET: `TestAmbiE2E` — producer taps e-listen's ambi3 while
  e-listen's own binaural keeps flowing; audience refused even its
  own ambi; decoded field carries the scene (W energetic, X/W ≈ 1
  SN3D, Y ≈ 0 for the dead-ahead source). `TestAmbiOfferingByOrder`
  — order-2 space offers ambi2, refuses ambi3. All three repos
  `-race` green.

## Deferred beyond this plan
Ambi sink formats; head-frame rendering (§4 sketch); WebTransport;
TS client (Phase D); the SIMD-P7/P3(b) capacity program; occlusion
(cheap-render recipe recorded in size-and-directivity.md); spectral
directivity.
