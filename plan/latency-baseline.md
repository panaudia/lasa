# Latency baseline

*Started 2026-08-04 (Phase B milestone). The audio-latency companion
to `state-perf-baseline.md`: the mouth-to-ear model, the measured
baselines, how to re-measure, and the client-debugging recipe. This
is a **living results document** — append a dated row whenever a
harness is re-run on a new machine, commit, or configuration; never
overwrite history. The harnesses themselves are tripwires with
generous bounds; the precise numbers live here.*

## 1. The mouth-to-ear model

One frame of audio, from a speaking client's microphone to a
listening client's ear, crosses these stages. Legend: **[M]**
measured by a harness below; **[D]** fixed by design; **[C]**
client- or deployment-owned (outside our baselines).

```
 speaker device                     server                                listener device
┌───────────────┐   ┌─────────────────────────────────────────────┐   ┌────────────────┐
│ capture [C]   │   │ depacketizer      [D] 0 in order, hold-on-  │   │ opus decode    │
│ opus encode   │   │                       gap ≤ 8 frames when   │   │  [M, in-loop]  │
│  [M, in-loop] │ → │                       recovering a loss     │ → │ playback       │
│ network up [C]│   │ jitter buffer     [M] adaptive fill; live   │   │  buffer [C]    │
└───────────────┘   │                       via LatencySamples    │   └────────────────┘
                    │ tick alignment    [D] 0–5 ms quantization   │
                    │ render DSP        [M, residual] HRTF/delay  │
                    │                       group delay           │
                    │ sink opus encode  [M, in-loop]              │
                    └─────────────────────────────────────────────┘
```

- **Framing is 5 ms end to end** (240 samples at 48 kHz): the wire
  frame (`lasa-core.md` §5.1), the render tick, the sink frame, and
  — since 2026-08-04 — the engine's default jitter writer geometry
  are the same unit, so no stage adds re-framing latency. (The
  engine default was 20 ms — the incumbent's packetization — until
  Paul had it changed to match the LASA framing; the first engine
  rows below ran on the old default and are kept as history. The
  server has always set 5 ms explicitly at `EntityJoined`.)
- **The jitter buffer is the dominant adaptive term.** Everything
  else is (near-)constant; this one grows with network jitter and
  loss. It is readable live per source (`Source.LatencySamples`,
  surfaced per entity in the server's stats snapshot), which is what
  makes the subtraction recipe in §4 work.
- **The buffer's latency is T = R + S + L, and W is not in it.**
  The v3 window is asymmetric by design (`buffers/jitter_buffer.go`):
  only the late allowance L costs latency; the writer frame W is the
  sawtooth amplitude and capacity-side geometry (drop/overrun
  lines), and cancels out of the stream delay. The measured 11.0 ms
  fill on a clean feed is exactly 5 ms (reader frame) + 1 ms
  (safety) + 5 ms (L warm start) = 528 samples — which is why the
  20 ms→5 ms writer-geometry change moved nothing (see the chirp
  table). L adapts within [2 ms, 30 ms], so live fills span
  ~8–36 ms with network jitter. Where frame size DOES cost latency
  is client-side packetization — a 20 ms sender holds samples an
  average of 10 ms before sending — which is a [C] stage above and
  the real-world win of the 5 ms wire framing.
- **Two pose pipelines, two different latencies — by design**
  (`panaudia-server-design.md` §1, engine pose harness):
  - *sink pose* (your own head turn) takes the latest-wins fast
    path: it must never wait behind the jitter depth;
  - *source pose* (someone else moving) rides the jitter buffer
    with its audio — its latency **equals the audio latency**,
    deliberately, so movement and sound stay coherent. A client
    that perceives "slow movement" at correct audio latency is not
    seeing a pose bug; it is seeing the audio latency itself.

## 2. Baselines

All rows: dev Mac (Apple M4 Pro, macOS), loopback QUIC, reverb none,
defaults elsewhere. Bracketed terms are the measured decomposition.
Commits name the repo HEADs the measurement ran on.

### Mouth-to-ear (chirp cross-correlation)

| Date | Loop | Total | = jitter fill | + residual | Commits | Harness |
|---|---|---|---|---|---|---|
| 2026-07-31 | engine only (PCM in → opus out; **20 ms ingest packets**, then-default W=20 ms) | **18.7 ms** | 11.0 ms | 7.7 ms | engine pre-`ac7d9d3` | `engine/latency_test.go` `TestAudioLatencyChirp` |
| 2026-08-04 | full server (client opus → QUIC → server → QUIC → client opus; **5 ms frames**, W=5 ms) | **30.2 ms** (1451 smp) | 11.0 ms (528 smp) | 19.2 ms (923 smp) | lasa `b800929`, engine `ac7d9d3`, server `1def4fa` + S6 working tree | `panaudia-server/harness_test.go` `TestServerAudioLatencyChirp` |
| 2026-08-04 | engine only (**5 ms packets**, new default W=5 ms) | **18.7 ms** (899 smp) | 11.0 ms (528 smp) | 7.7 ms (371 smp) | engine `ac7d9d3` + default-change working tree | `TestAudioLatencyChirp` |
| 2026-08-05 | full server, **after the browser-sender ingest retune** (per-source LowInit 10/LowMin 7 ms + deep H, panaudia-server EntityJoined; jitter-debug session — see the engine plan's burst-aware-widening note) | **35.2 ms** (1691 smp) | 16.0 ms (768 smp, warm-start L=10; settles to 13 ms as L→7) | 19.2 ms (923 smp, unchanged) | engine/server working trees post-retune | `TestServerAudioLatencyChirp` |

Reading the rows against each other: the **server loop adds
~11.5 ms over the engine loop**, which is the speaking client's own
opus encode/decode pass, transport, and real-clock (vs deterministic)
tick alignment — an exact comparison, since the 2026-08-04 engine
row runs the same 5 ms geometry as the server row. The engine
residual (7.7 ms) is one opus pass + tick quantization + DSP group
delay. Loopback transport itself is negligible (µs); ingress was
lossless everywhere (server run: 400/400 delivered, 0 gap events).
The 20 ms-vs-5 ms engine rows also establish a useful fact: on a
clean feed the adaptive steady-state fill is **geometry-independent**
(528 samples both ways) — packet size buys coherence granularity
(see the pose table), not fill depth.

### Browser client (TS SDK, Phase D — full loop to the RENDER point)

Measured in the playout worklet's rendered output via ~16 ms stats
windows (resolution ± one window); **excludes the device DAC stage**
(headless Chromium reported baseLatency 5.3 ms + outputLatency 24 ms
— virtual; measure on real devices separately). Loop: capture worklet
→ SAB ring → worker Opus encode (5 ms) → loopback WT/QUIC → server →
WT/QUIC → worker decode → SAB jitter ring → worklet render. Both
browser jitter terms included (server ingest ~11 ms + browser playout
~13 ms operating fill).

| Date | Path | Result (3 runs) | Commits | Harness |
|---|---|---|---|---|
| 2026-08-05 | mouth-to-ear onset, browser full loop | **46–62 ms** (61.8 / 45.7 / 61.6) | lasa `720c0c1`, engine `88f7008`, server `09770ac`, panaudia-client `824872c` + D3 working tree | `typescript-lasa/tests/latency.spec.ts` (`__latencyRun`) |
| 2026-08-05 | sink pose flip (head turn, SAB pose cell path), browser full loop | **50–53 ms** (53.0 / 49.8 / 51.8) | as above | same |
| 2026-08-05 | mouth-to-ear **with real devices** (acoustic loop: click → built-in mic → full loop → speakers → recorded echo), dev Mac, Chrome, lasa-test-page | **~120 ms** (single measurement, Paul) | as above + D4 working tree | manual — the §4 clap-and-listen recipe |

The real-device row closes the [C] stages on this machine: the page's
graph report gave the output side directly (baseLatency 5.3 ms +
outputLatency 27.0 ms ≈ 32 ms), the harness rows give the loop to the
render point (46–62 ms), so the implied INPUT side (mic → ADC → OS →
getUserMedia delivery) is **~25–40 ms** — the one stage no browser API
reports. Device tax on this machine ≈ 60 ms of the 120 ms total; a
USB interface would reclaim ~20–30 ms of it.

Cross-check against the Go rows: browser mouth-to-ear ≈ Go server
30.2 ms + browser playout jitter ~13 ms + capture-worklet/quantum
~5 ms + detection bias ≤ 1 window — the stages account for the
number. The pose flip ≈ pose→packet ≤ 5 ms + server 15 ms + playout
jitter 13 ms + ≤ 2 windows detection — the sink-pose fast path stays
well clear of tracking the full audio latency. Note the pose result
rides the freshest-pose SAB seqlock (framing-time stamping): the
listener entity ran a silent capture pipeline precisely so its pose
took the real head-tracked-client path.

### Pose

| Date | Path | Result | Commits | Harness |
|---|---|---|---|---|
| 2026-07-31 | sink pose (head turn), engine | **2 frames = 10 ms** | engine pre-`ac7d9d3` | `engine/pose_latency_test.go` `TestSinkPoseLatency` |
| 2026-08-04 | sink pose (head turn), full server | **3 frames = 15 ms** send→flip | as chirp row | `TestServerSinkPoseLatency` |
| 2026-07-31 | source pose/audio coherence, engine (20 ms packets) | skew **+5 ms** (+240 smp) | engine pre-`ac7d9d3` | `TestSourcePoseAudioCoherence` |
| 2026-08-04 | source pose/audio coherence, full server (5 ms frames) | skew **−2.5 ms** (−120 smp) | as chirp row | `TestServerSourcePoseCoherence` |
| 2026-08-04 | source pose/audio coherence, engine (5 ms packets, new default) | skew **+2.5 ms** (+120 smp) | engine, default-change working tree | `TestSourcePoseAudioCoherence` |

Coherence skew is *not* a latency: it is the error in the
stream-speed identity (heard onset→flip distance vs sent distance).
Anything inside a frame or two is alignment working; a skew near the
jitter depth would mean the source-pose ring has broken alignment.

### Capacity (context for the numbers above)

| Date | Load | Render per tick | Max jitter fill | Commits |
|---|---|---|---|---|
| 2026-08-04 | 50 sources × 50 sinks, real cadence, all-audible | **1.08 ms** of the 5 ms budget | 768 smp (16 ms) | as chirp row |

The render holding its budget is what keeps every latency above
valid under load: a tick overrun would show up directly as added
mouth-to-ear latency for every listener.

## 3. Re-measuring

Neither suite runs the harnesses under `-short`; timing gates
self-skip under `-race`.

```sh
# engine loop (deterministic, no network):
cd panaudia-engine && go test -run 'TestAudioLatencyChirp|TestSinkPoseLatency|TestSourcePoseAudioCoherence' -v ./engine

# full server loop (real clients over loopback QUIC):
cd panaudia-server && go test -run 'TestServerAudioLatencyChirp|TestServerSinkPoseLatency|TestServerSourcePoseCoherence|TestCapacitySmoke50x50' -v .
```

Append a dated row per run worth keeping (new machine, new commit of
any of the three repos, changed jitter geometry or codec settings —
say what changed). The production x86 box needs its own first rows
when it exists.

## 4. Debugging a client's latency

The point of the baselines: the server's contribution is a **known,
subtractable constant**, and its one adaptive term is **readable
live**. Given a complaint "audio feels late" on a real deployment:

1. **Measure end-to-end** at the client (their capture → their
   playback; a clap-and-listen loop against their own sink works —
   an entity always may subscribe its own sink).
2. **Subtract the server baseline** (30.2 ms on the dev box — the
   deployment box's row once measured). What remains is: real
   network (up + down), client capture buffering, client playback
   buffering, and any extra client-side queuing. The baseline
   already includes both codec passes, so do not subtract codec
   estimates separately.
3. **Split the adaptive term live**: read the entity's
   `latency_samples` from the server stats snapshot. If it is far
   above the ~11 ms loopback fill, the network's jitter (not the
   server, not the client's buffers) is buying latency — and the
   depacketizer counters alongside it (`lost`, `gap-events`,
   `GapHist`) say whether loss-driven recovery is contributing.
4. **Distinguish the pose complaints**: "my head turns feel slow" →
   sink-pose path, compare against the 15 ms baseline — it should
   never track jitter depth. "Others move late" at correct audio
   sync → that is the audio latency by design (§1); fixing it means
   fixing the audio path, not the pose path.

The TS client's harness (browser rows above) now closes most of the
[C] gaps: its loop includes real capture buffering (worklet + SAB
ring) and real playback buffering (the browser's own v3 jitter ring).
Re-run it with `npx playwright test tests/latency.spec.ts` in
`panaudia-client/sdks/typescript-lasa` (numbers print in the spec
output; append rows per machine/commit). Still outside the rows: the
device DAC stage (`outputLatency` — read it from the harness output's
graph report on real hardware) and real network.
