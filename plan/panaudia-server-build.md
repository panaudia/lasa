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

- [ ] Paul: commit outstanding engine work (latency/pose harnesses,
      `LatencySamples`, vestige removal — PROVENANCE #5)
- [ ] Paul: create the `panaudia-server` repo (needed from S1)

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

### S1 — scaffold & clock wiring
- [ ] repo, module `github.com/panaudia/panaudia-server`, deps
      (lasa, panaudia-engine — both replace-free), config/env
      surface, `main` skeleton
- [ ] ticker owned here; epoch₀ read at first tick; `SampleEpoch`
      closure handed to the shell (§1 C5)
- Accept: starts, ticks, serves an empty space
### S2 — the lifecycle funnel (§5)
- [ ] one serialized funnel: `Admit` (evict-old wait), `EntityJoined`
      → engine adds, `EntityLeft` → engine removes; conn-map mutated
      only here; engine errors fatal to the connection
- [ ] e2e with the lasa reference client: connect / disconnect /
      evict-old sequences; conn-map ≡ engine-entity-set invariant
- Accept: lifecycle e2e green
### S3 — the audio path
- [ ] ingress: Depacketizer-wrapped handler → pose fan-out (source
      ring via `WriteOpus`, sink slot via `SetPose`, dof enforcement,
      presence collector) → `WriteOpus`/`Conceal`
- [ ] `StartSink` → `AddSink` with the `FrameWriter`→`SinkWriter`
      bridge; jitter geometry W=5 ms; head-frame reject at admission
      (§4); format negotiation (binaural v0)
- [ ] e2e: two clients hear each other (tone detection);
      `AllocsPerRun` = 0 on both audio paths
- Accept: audio e2e green
### S4 — state wiring (Phase C pulled in)
- [ ] observer subscription → `profile/base` parse → engine mutators
      (render params, channels, entity mute); defaults parity test
      (profile defaults ≡ `engine.DefaultRenderParams()`)
- Accept: state-driven render changes e2e (mute silences, gain
  changes level, channel exclusion works)
### S5 — presence + moderation e2e
- [ ] presence feed live (collector + loudness getters); client
      receives presence with correct poses/loudness
- [ ] block/kick e2e via state writes through the gate
- Accept: both observable from the reference client
### S6 — acceptance instruments
- [ ] chirp + pose harnesses ported to run through server + client
      loop (mouth-to-ear minus network); numbers logged as the
      server baseline
- [ ] capacity smoke at 50×50 through the full server
- [ ] minimal stats/health surface (engine Stats, LatencySamples,
      depacketizer gap counters)
- Accept: **Phase B milestone — a native LASA client round-trip on a
  clean server binary, instrumented**

## Deferred beyond this plan
Ambi sink formats; head-frame rendering (§4 sketch); per-pair
channel gain/attenuation (pair record); tickets; WebTransport; TS
client (Phase D); the SIMD-P7/P3(b) capacity program.
