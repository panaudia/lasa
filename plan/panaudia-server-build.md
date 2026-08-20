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
      — *revised 2026-08-09 (Paul's ruling: a space is either
      ticketed or open, never open by accident): key + allow=true is
      now a startup error, and an open space requires the explicit
      `PANAUDIA_ALLOW_UNTICKETED=true` (no key present no longer
      defaults open); matrix in `config_test.go`*
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

### P5 — head-frame entities — DONE 2026-08-05
- The Phase B §4 sketch implemented, with Paul's three rulings:
  EXCLUDED from ambi/classic renders (a per-listener source is
  unrepresentable in a world-frame field), SOURCE-ONLY (no world
  position to hear from — StartSink refuses all formats), and
  JITTER-ALIGNED pose pipeline (the sketch's fresh-slot lean not
  taken: zero new machinery, one source-pose story, coherence free).
- [x] engine: `SourceConfig.HeadFrame` → encoder `headFrameSource`
      (PROVENANCE #13): bilateral geometry takes the pose verbatim
      (no translation, no head rotation — ITD/parallax/NFC/extent
      unchanged downstream; directivity/size stay coherent since
      forward and rel share the listener frame);
      `shouldIncludePeer` excludes head-frame from classic renders
- [x] lasa: `AdmissionGroup` materialises `frame:"head"`; §6
      erratum pins source-only + absent-from-ambi
- [x] server: Admit's rejection removed (the S3
      ErrCodeMalformedConfig note is moot); EntityJoined passes the
      flag to engine AND presence Register (krecord flag was
      already on the wire); StartSink refuses head-frame entities
- Accept MET: engine `TestHeadFrameInvariance` (angel pinned to the
  left ear at constant level across teleports and about-faces —
  the defining property) + `TestHeadFrameExcludedFromAmbi`;
  composed `TestHeadFrameE2E` (admitted, head-invariant across a
  20 m teleport + yaw via real pose packets, presence keyframe
  flag set, own sink refused) + `TestHeadFrameAbsentFromAmbiE2E`
  (presenter-carried head-frame entity audible in binaural, W
  power ~0 in the tapped ambi2 field). All three repos `-race`
  green.

### P6 — Phase D: WebTransport + TypeScript client (design agreed 2026-08-05)
- Decisions (Paul): adapt the EXISTING in-house draft-16 TS codec
  (tested, golden-fixtured, worker-integrated; its Eyevinn deltas
  are documented) — moqtail-ts (also draft-16, in-tree) serves as
  interop reference, not dependency. New SDK = SIBLING package
  beside `panaudia-client/sdks/typescript` importing nothing from
  the old one (portable core COPIED, provenance-style: jitter
  buffer, rings, worklets, datagram router, coordinates,
  topic-tree views, verify-* release gates; the audio hot path
  ports by swapping the two codec seam calls). Old SDK keeps
  serving old panaudia. Sibling demo page in
  `panaudia-client/examples` when D4 arrives.
- [x] D1 — WebTransport in the server — DONE 2026-08-05: ALPN-routed
      shared UDP 4443 (`h3` → webtransport-go v0.12 upgrade at
      `/lasa` (`wt.go`) → `webtransportmoq` → the existing
      `ServeConn`; `moqt-16` → the raw-QUIC path; the serve loop
      moved from lasa's ServeQUIC into panaudia-server); dev cert
      now ECDSA P-256/13-day with serverCertificateHashes logged at
      boot (real certs unchanged, both ALPNs always offered);
      listener + both dialers set EnableStreamResetPartialDelivery
      (WT requirement). lasa `client.DialWebTransport` added (Dial
      refactored to a shared `start`; lasa's webtransport-go pinned
      v0.12 to match quic-go v0.61). `TestWebTransportE2E`: WT and
      raw-QUIC clients in one space, both speaking, each hearing
      the other — every path crosses the WT session. Both repos
      `-race` green.
- [x] D2 — interop spike — DONE 2026-08-05: PASSED with ZERO codec
      changes (the Eyevinn-lineage bet: our fork descends from the
      exact implementation the SDK codec was golden-fixtured
      against). New package `panaudia-client/sdks/typescript-lasa`
      (@panaudia/lasa-client, PROVENANCE.md provenance-style):
      copied transport files + shared-types; divergence 1 =
      additive extraParams on buildClientSetup/initialize carrying
      the 0x1A5A1 Connection Config (`src/lasa/setup.ts`: config
      builder, namespace tuples, track names). Playwright rig
      (vite spike page + globalSetup building/booting
      panaudia-server). TLS: the rig uses the real *.panaudia.com
      wildcard (code/panaudia/keys/server.{crt,key},
      dev.panaudia.com → 127.0.0.1 in /etc/hosts — the old SDK
      harness's pattern, Paul's pointer) = production TLS shape,
      no browser flags; falls back to the ECDSA dev-cert
      serverCertificateHashes path (also proven end to end) when
      the keys are absent. Full chain
      green: WT with moqt-16 subprotocol negotiated → SETUP+config
      → SUBSCRIBE_OK → presence keyframe (entity admitted; the
      session auto-OK'd the server's subscribes back). Server
      fixes that fell out: `CheckOrigin` allow-all on the WT
      server (deliberate policy — tickets are the auth, never
      browser-ambient credentials; library default is same-origin)
      + upgrade/session logging in wt.go.
- [~] D3 — LASA protocol layer in TS — CORE SLICE DONE 2026-08-05:
      `src/lasa/wire.ts` (pose 18B, mono-object incl. redundancy
      layout, sink, presence keyframe/delta — byte-exact via golden
      vectors GENERATED FROM `lasa/wire` Go, tests/vectors/;
      11 vitest cases) + `src/lasa/client.ts` (multi-entity-first
      LasaClient: connect = Connection Config at SETUP; per-entity
      EntityPublisher materialised from the server's
      subscribe-back alias; subscribeSink/subscribePresence;
      zero-alloc publish path). Session divergence 2:
      single-reader dispatch (two concurrent control-stream
      readers stole messages — found by the audio spec hanging).
      Acceptance: `tests/audio.spec.ts` — one connection, TWO
      entities; A publishes WebCodecs opus 5 ms mono-object
      packets at real cadence, B hears itself back through its
      binaural sink (WebCodecs decode, ≥50 energetic frames) —
      the protocol audio path proven in-browser with no worklets.
      STATE CLIENT DONE 2026-08-06:
      `src/transport/subgroups.ts` (subgroup stream reader/writer +
      per-alias router, type family 0x10–0x1D; buffers objects for
      unclaimed aliases — the server writes the catch-up right
      after Accept, racing SUBSCRIBE_OK processing; found as a
      no-debug-only flake), `src/lasa/statewire.ts` (state messages
      both directions + subscribe params + WebCrypto set-hash,
      golden vectors extended to 21 incl. canonicalization +
      cursor), `src/lasa/store.ts` (clientstore port —
      resurrection guard, snapshot deletion rule, invariant F7,
      epoch change, MemPersistence; Go store_test.go ported
      test-for-test), `src/lasa/state.ts` (StateSync:
      subscribe with 0x1A5A3 via session extraParams
      [divergence 3], lanes 0/1 → applyLive/applyCatchUp on a
      serialized chain, PUBLISH_DONE 0x0B dispatch
      [divergence 4] as the re-subscribe cue with backoff),
      LasaClient.writeState (upstream ops on a lazily-opened
      0x15 subgroup stream over the server's state-source
      subscribe-back alias) + syncState. Acceptance:
      `tests/state.spec.ts` — A syncs, sees its admission keys
      (marker + name) through catch-up, earns a cursor, writes an
      own-entity attr, observes the server echo with a server
      seq; B connects fresh and finds A's write in its catch-up.
      41 vitest + 3 Playwright green, 3× stable.
      AUDIO MACHINERY (architecture C, rulings 2026-08-05): full
      worker transport — the old SDK's proven 3-thread shape
      (worker owns WebTransport+session+read loops+codecs; main
      owns facade/state/worklets; SAB rings between) — chosen over
      the transferred-readable intermediate, which the old SDK
      tried and found still main-gated (worker-transport-design.md
      preamble). Capture: per-entity MONO pipelines (DAW multi-track
      = N entities, stereo playback = 2), source acquisition beyond
      the mic deferred. COOP/COEP hard requirement (SDK gates on
      crossOriginIsolated — mechanism-agnostic; DIP noted as
      Chrome-137-desktop-only, not a replacement). Meters kept.
      POSE FRESHNESS (Paul): head-rotation latency minimised —
      pose rides a per-entity SAB seqlock cell written by main,
      read by the worker at datagram-framing time (freshest-wins
      over capture-aligned; ~one 5 ms packet max staleness).
      Steps 0–2 DONE 2026-08-05:
      (0) COOP/COEP on the vite dev/preview server, verified
      crossOriginIsolated=true + SAB in-page; host pinned to
      127.0.0.1 (vite bound ::1-only, Playwright probes v4).
      (1) copies at panaudia-client@824872c → `src/audio/`:
      jitter-buffer-core, capture-ring, stereo-meter-core,
      capture-worklet (verbatim + their test suites, 87 tests);
      playout-worklet SAB-ONLY (divergence 5 — postMessage-PCM +
      pcmPort fallbacks removed, throws without the shared ring).
      Jitter review verdict: same v3 algorithm as
      panaudia-engine/buffers; TS browser tuning (L/H minimums,
      windowReads 750) is the deliberate 2026-06-10 reader-burst
      retune — kept.
      (2) worker transport live: `src/worker/protocol.ts` (typed
      RPC: connect/subscribe+extraParams/sendDatagram/writeState/
      close; forwarded datagram + subgroupObject + incomingSubscribe
      + publishDone + notice events), `src/worker/moq-worker.ts`
      (Vite module worker owning conn/session/both read loops and
      the state-source SubgroupWriter), `src/worker/worker-transport.ts`
      (main-side RPC + WorkerSessionProxy implementing the
      StateSession surface StateSync now takes). LasaClient is the
      main-side facade: store/StateSync fed via SubgroupRouter.ingest
      from forwarded objects, DatagramRouter for sinks/presence,
      publishers frame on main → worker writes. All three specs
      (interop/audio/state) pass through the worker unchanged,
      3× stable; 134 vitest green.
      Step 3 DONE 2026-08-05 (playout path): protocol gained
      setSinkTrack/clearSinkTrack (+ decode-error notice +
      decodedFormat event); worker diverts sink aliases — parseSink
      → AudioDecoder (optimizeForLatency, timestamps = seq×5000µs)
      → writer-view JitterBufferCore over the shared ring, with the
      old worker's f32/f32-planar copyTo probe (Firefox) + format
      watch ported (drift-era gap/clock probes deliberately left
      behind — PROVENANCE note); `src/audio/player.ts` SinkPlayer
      (AudioPlayer descendant, SAB-only decode-free: graph + SAB
      alloc + stats/TapB/graph-report + [JBUF] log);
      LasaClient.playSink/stopSink (one player per entity sink,
      multi-entity). Acceptance `tests/playout.spec.ts` (autoplay
      flag added): tone A → B's binaural through worker-decode →
      SAB → worklet; asserts from the WORKLET's own stats — Tap B
      energetic and strongly stereo (corr −0.91, sideRms 0.13 =
      real HRTF decorrelation), fill in-band 13–18 ms, ZERO
      underruns/overruns/corrections in steady state, controller
      observed narrowing L/H. 4 specs green 3× stable; 134 vitest.
      Step 4 DONE 2026-08-05 (capture path): `src/audio/pose-cell.ts`
      (per-entity 56-byte SAB seqlock, main writes / worker reads —
      the pose-freshness rule); `src/audio/capture-encoder.ts` (old
      capture-encoder with LASA framing: Opus packet →
      MonoObjectPacket(pose,audio) → datagram groupId = gapless seq
      §5.2, pose read at FRAMING time; unit tests incl. a
      pose-freshness case); `src/audio/capture.ts` CapturePipeline
      (per-entity mono, any AudioNode via ChannelSplitter, caller
      owns the context, addModule once-per-context guard); worker:
      entity-alias map + waiters, per-entity Atomics.waitAsync pump
      (the old §6.1 loop), real AudioEncoder injected into the
      unit-tested CaptureEncoder; protocol setCaptureTrack/
      stopCapture (+encode-error notice);
      LasaClient.startCapture/stopCapture (CaptureHandle.setPose).
      Acceptance `tests/capture.spec.ts` — THE MILESTONE: oscillator
      → capture worklet → SAB → worker encode → server render →
      worker decode → SAB → playout worklet, no main-thread audio
      either direction; fill in-band 13 ms, ZERO underruns/
      corrections; AND the live mid-stream setPose changed the
      rendered stereo image (corr −0.91 → +0.998, asserted shift
      >0.5) — the SAB-fresh pose path proven end-to-end through the
      server's HRTF. 5 specs green 3× stable; 144 vitest.
      Step 5 DONE 2026-08-05 — D3 AUDIO MACHINERY COMPLETE.
      Browser latency harness (`spike/latency.ts` +
      `tests/latency.spec.ts`): SinkPlayer gained statsEvery +
      setStatsListener (fine ~16 ms Tap B windows = measurement
      resolution); mouth-to-ear onset (gated 2 kHz tone through the
      full capture→server→playout loop) **46–62 ms**; sink-pose
      flip (listener runs a silent capture pipeline so its pose
      rides the SAB seqlock — the real head-tracked path; yaw π →
      L/R dominance flip, the Go harness's technique) **50–53 ms**;
      both to the RENDER point (DAC excluded; headless reported
      base 5.3 + output 24 ms virtual). Numbers cross-check against
      the Go rows (browser ≈ Go server + browser playout jitter +
      capture chain + ≤2 windows). Rows + re-run recipe appended to
      latency-baseline.md (the [C] gaps closed). Teardown verified
      by the suite's clean closes across all six specs. Final: 6
      Playwright specs green, 144 vitest, typecheck clean.
      NEXT: D4 (sibling demo page in panaudia-client/examples +
      feature-parity inventory), D5 (browser ambi receiver +
      multi-track object sender).
      Original scope note follows. REQUIREMENT (Paul,
      2026-08-05): the SDK is MULTI-ENTITY-FIRST — a connection
      owns N entities (N publishers, per-entity pose, N
      subscribable sinks) from the first commit; the old SDK's
      one-entity-one-capture shape is not carried over. ALSO:
      review the copied TS jitter buffer against the current Go v3
      (constants/defaults — e.g. the 5 ms writer-frame default,
      PLAYOUT_TUNING drift) and confirm it is channel-count-generic
      for the ambi playout path.
- [x] D4 — sibling demo page in panaudia-client/examples; the
      greenfield plan's feature-parity inventory (I-A).
      PAGE HALF DONE 2026-08-05 (rulings: purpose-built LASA page,
      not a port; SDK consumed from source via Vite alias until the
      packaging pass; spike TLS story, mkcert not carried):
      `examples/lasa-test-page/` — connect card (config/ticket/cert
      hash), pre-connect entity list (the Connection Config is
      setup-time — set fixed per connection), per-entity capture
      (tone or mic, EC/NS/AGC off) + binaural playout, shared 2D
      space map (presence roster + own entities draggable; capturing
      entities send pose via the SAB cell, others as pose-only
      mono-object packets; yaw slider), state panel (live store
      table + set/clear), diagnostics strip (jitter fill/L/H/
      underruns, Tap B, device latencies). SDK gained
      `src/index.ts` (public surface — the packaging seam).
      Smoke-tested end-to-end via scripted Chromium + fake mic
      against a real server: connect → tone capture → listen (fill
      14.3 ms, 0 underruns, TapB energetic) → SECOND simultaneous
      capture (fake mic) → pose drag → state table shows admission
      keys → clean disconnect. Vite fs.allow needed for the
      out-of-root SDK worker file.
      INVENTORY HALF DONE 2026-08-05 — D4 COMPLETE:
      `lasa/plan/feature-parity-inventory.md` (I-A), built from a
      full code-verified survey of panaudia@fd76cc8 (banner env
      block, endpoints, 20-command catalog, hardcoded role table,
      Link/ROC surface, deploy artifacts) vs panaudia-server@
      09770ac. Survey's sharpest finds: PANAUDIA_TLS_CTR_PATH typo
      is product surface; documented MOQ MaxClients was never
      enforced; the MixerStats JSON schema is dormant (zero call
      sites) — the old server's real observability was the banner;
      Link is unauthenticated by design. Doc classifies every item
      CARRIED/SUPERSEDED/DROPPED/GAP/OPEN, ends with the §10
      maintenance-mode bar: health+stats HTTP, LASA ticket
      ecosystem in panaudia-client, the Link decision (after D5),
      Safari/Firefox WT validation, prod packaging, Unreal ruling,
      small sweeps. OPEN rulings queued for Paul: Link/D5,
      role-targeted moderation, mac-app distribution, egress
      bitrate pin, older-Safari (no-WT) posture.
      Post-D4 additions 2026-08-05 (Paul trying the page live):
      dev.panaudia.com route (vite allowedHosts + page served
      HTTPS with the wildcard keys — non-localhost HTTP is not a
      secure context); first real-device latency row appended to
      latency-baseline.md (~120 ms acoustic loop, output side
      measured 32 ms, implied input ~25–40 ms); mic-settings
      read-back logged on capture; microphone selection —
      `microphone-selection.ts` copied verbatim into the SDK
      (Bluetooth-avoiding auto-pick, the HFP lesson; PROVENANCE
      entry incl. the accepted probe trade) + page dropdown with
      Auto default, manual override tagged [usb]/[builtin]/
      [bluetooth], devicechange refresh. Smoke suite re-run green
      through the auto path. Latency-reduction notes discussed:
      OS input at 48 kHz (Paul's already is);
      MediaStreamTrackProcessor-in-worker recorded as a future
      Chromium fast path (~10–20 ms input saving, worklet
      fallback retained) — not built.
      JITTER DEBUG SESSION 2026-08-05 (Paul heard clicks + low
      grit on the page): instrumentation added both sides —
      browser: worklet discontinuity EVENTS (posted the moment a
      counter increments, PROVENANCE divergence 6), worker sink
      INGRESS health (seq gaps/lost/reorder/decode-err, ~2 s),
      capture-ring overflow watch, all surfaced on the page;
      server: engine Source.JitterStats() (v3 snapshot),
      PANAUDIA_STATS_SEC (≤15 s ⇒ per-entity jbuf+dep detail
      lines). DIAGNOSIS from Paul's live data: uplink clean
      (dep lost=0); server ingest CHURN — browser worker-paced
      send bursts (~10–15 ms) clipping the native-tuned band
      (H=W=5 ms): ~30 drop-splices/2 s window (the grit) whose
      deficit starved the floor (underrun clicks ~1/2 s); v3
      controller correctly refused to widen (one-sided drops =
      its drift classifier — same June-2026 reader-burst
      confusion, writer side). Initial clock-drift theory
      FALSIFIED by the clean browser-side event log. FIX: engine
      SourceConfig gained JitterLowMin/HighInit/HighMin/HighMax;
      panaudia-server EntityJoined sets the browser-sender
      profile 10/7/16/14/30 ms (June values mirrored; H is
      capacity-only, LowMin is the latency knob). Cost measured
      + appended to latency-baseline.md: server chirp 30.2 →
      35.2 ms warm-start (fill 16 ms settling to 13; residual
      unchanged). Controller idea RECORDED (Paul's ask):
      panaudia-engine/plan/jitter-v3-burst-aware-widening.md
      (underruns-as-late-side-signal + window fill-swing
      discriminators; re-entry trigger = a third hand-tuned
      environment). All suites green post-change (engine,
      server -short + chirp, 6 Playwright, 144 vitest).
      CONFIRMED by Paul's re-test 2026-08-06: drop=0 und=0
      win=0/0 across every window, dep lost=0; fill oscillates
      15–20 ms inside the band (≈5 ms burst swing, no clipping);
      no drift residue at all — the churn was pure burst-clipping,
      no measurable clock mismatch. Sounds much better by ear.
- [ ] D5 (recorded 2026-08-05, Paul's browser-replaces-native
      insight) — two capabilities the protocol already carries:
      (a) BROWSER AS AMBI RECEIVER, replacing the old native macOS
      app: producer-ticket ambi2/ambi3 subscribe over WT → TS
      multistream splitter (coupled=0 identity mapping ⇒ the packet
      is N self-delimited standard opus packets; WebCodecs can't
      eat multistream directly, so ~100-line framing parser + N
      AudioDecoders → interleaved N-ch ring) → multichannel output
      via a virtual device (BlackHole etc.; setSinkId +
      destination.channelCount; docs/support story, no native
      code — panaudia-driver-macos exists if a first-party device
      is ever wanted); (b) BROWSER AS MULTI-TRACK OBJECT SENDER: a
      presenter/producer connection with N entities = a stem feed —
      multichannel virtual input device → channel splitter → N
      capture/encode pipelines → N mono-object tracks; server side
      needs nothing (entities are entities; the S4 state plane
      gives per-stem control). Both hinge on D3's
      multi-entity-first shape.
- [x] Pose establishment (2026-08-06, closing the spatial-rendering
      session's OPEN item): entities now exist where the map draws
      them from the first frame. The protocol always supported it —
      entity.schema.json `initial-pose`, resolved to home + engine
      InitialPose + presence seed; the page just never sent it. FIX:
      (a) test page sends each entity's pose as `initial-pose` at
      CLIENT_SETUP (SDK passed it through verbatim already); (b) new
      presence.Slot.Pose() getter (lasa); (c) addPoseSink seeds a
      late-attached sink with the entity's latest pose before it
      joins the fan-out list (single-writer rule preserved — seed
      happens before Frame() can see the sink), so a never-dragged
      listener renders with its declared attitude. Test:
      TestAddPoseSinkSeedsLatestPose. Verified: server -short,
      6 Playwright e2e, page tsc.
- [x] Per-route ingest profiles (2026-08-06, shipped as part of the
      panaudia-bridge tuning pass — the jitter TODO's "route
      profiles" interim, server side): the transport is the
      sender-class signal (browsers can only be WebTransport; raw
      QUIC = native). Plumbing: moqtransport conn.Protocol() →
      connect.Resolved.Transport (new field, stamped by the shell's
      ServeConn/onClientSetup) → backend records per-client at
      Admit → EntityJoined gives WT senders the browser burst band
      (10/7/16/14/30) and raw-QUIC senders the engine's native
      defaults; record cleared with the client's last entity.
      Verified: lasa server+connect tests, panaudia-server -short,
      chirp harness (raw QUIC) recovered 35.2 → 30.2 ms while
      browser protection is unchanged.
- [ ] Jitter-buffer burst behaviour redesign (TODO 2026-08-06,
      Paul's call after the day's investigation) — v3's re-entry
      trigger fired: four burst environments now known (browser
      reader, browser writer, worker GC stalls, Bluetooth output
      clock), and bursts ≈ band height pin the playout buffer at its
      drop line (continuous splices). Design session covers:
      burst-aware widening, fast re-centre after large offsets,
      per-route profiles as interim. Full evidence + scope:
      panaudia-engine/plan/jitter-v3-burst-aware-widening.md
      (2026-08-06 addendum). Fixed en route: PoseCell unbounded
      seqlock spin (bounded-retry read + test), instrumentation
      feedback loop (summarised logging).
- [x] Bluetooth HFP collapse + mic selection (parked then FIXED
      2026-08-06) — LASA page's mic-auto had reintroduced the
      device-opening selection flow the old client removed
      2026-06-11. All three fixes landed: selection never opens a
      device (priming + unknown-probe removed, `permissionPending`
      contract); first-grant Bluetooth recovery in the test page
      (open default once, re-select and switch when it proves BT);
      post-hoc HFP detection restored (probeOutputDeviceSampleRate
      ported to the LASA SDK + page banner with recovery advice).
      Divergence recorded in SDK PROVENANCE.md; evidence retained
      in bluetooth-hfp-todo.md (now marked DONE).
- [ ] WT close codes never reach the client (found 2026-08-13 by the
      identity browser rig; full record + fix ladder:
      `wt-close-code-defect.md`). Rejections/severs over WebTransport
      lose their §4.6 code; raw QUIC unaffected. Reproductions parked
      skipped here (`TestWTRejectionCarriesCode`,
      `TestWTRejectionProbe`); fix = webtransport-go fork/vendor
      patch (+ possibly moqtransport fork), version bump through
      Paul.
- [ ] identity changes (decided 2026-08-13, `id-capabilities.md` —
      both same-day decisions; spec + profile text updated; full
      sequenced plan: **`identity-implementation.md`**): claims as
      state keys (`lasa.client.{id}` = true; entity marker value =
      owning client id), adjudicated admission primitive on the
      state actor (supersede same-client atomically, refuse ticketed
      cross-owner + capacity — all refusals in one place), mode XOR
      in `server.New` + `0x1A5A08`, `Backend.Admit` deleted from the
      SPI, shell-side supersession teardown wait, grace period
      removed from profile §3, test flips per plan.

## Deferred beyond this plan
WebTransport;
TS client (Phase D); the SIMD-P7/P3(b) capacity program; occlusion
(cheap-render recipe recorded in size-and-directivity.md); spectral
directivity.
