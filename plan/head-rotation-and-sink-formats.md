# Head rotation & the sink format menu — DEFERRED

*Opened 2026-07-24. **Deferred 2026-07-27**: the whole head-rotation
question is postponed as a future piece of work so a full lasa2 draft
can be pulled together. v1 ships the initially planned sink menu —
**`binaural`, `ambi2`, `ambi3`, all mandatory to implement** — with no
bilateral formats and no correction sidecar. Everything below is
preserved as the trade space for that future phase; the listening
tests and the rotation-loop profiling (`next-steps.md` item 1) are the
re-entry path. Adding bilateral formats or a sidecar later is additive
under format-in-name, which is what makes this deferral cheap. Nothing
here is spec.*

*2026-07-25: a new candidate entered the space —
`rotation-correction-sidecar.md` (server binaural + low-rate FOA
sidecar + differential client correction). If it survives its listening
tests it weakens the case for mandatory `bilateral2` streaming and adds
a ~5-channel quality path; the mandatory-set sketch below should be
revisited against it.*

## The dilemma

Rotation latency is very perceptible (first-hand experience; literature
detection thresholds sit *inside* realistic network loop latencies of
40–80ms). But server-side binauralisation gives much better rendering —
the server could do third order with **bilateral ambisonics** (one SH
expansion per ear) for proper near-field compensation and ITD accuracy.
Streaming bilateral-3 is 32 channels.

**Literature backing, added 2026-07-25** (full survey and references in
`rotation-correction-sidecar.md`):

- **Latency thresholds bracket the loop.** Median listeners 60–110 ms
  (Brungart et al. 2006; Yairi et al. 2006; Lindau 2009: mean 107.6 ms);
  **critical listeners 44–52 ms**; engineering targets <40 ms (Rappin
  et al. 2023) / <30 ms (Brungart). Lindau's MAMA-derived budget:
  65.6 / 45.6 / 26.7 ms at 90 / 180 / 360°/s. A 40–80 ms loop is inside
  the critical-listener band and outside the median one. Full budget
  with the four methodology drivers, the standards position, and the
  detection-vs-acceptability caveat: `rotation-correction-sidecar.md`
  §The acceptable-latency budget.
- **Getting rotation right may buy more than getting the render
  right** — the finding most directly on this dilemma. Brimijoin, Boyd
  & Akeroyd (2013, *PLOS ONE* 8:e83068): with a badly degraded BRIR,
  static externalization was 10% but world-stabilized head motion
  raised it to **50%** — 5× from correct rotation *despite* spectral
  degradation. And a head-locked scene under head motion scored
  **38% vs 89%** for a static head: **untracked motion is actively
  worse than not moving at all.** This tilts the trade toward local
  rotation and away from "maximum server render quality, lag accepted".
- **Rotation tolerance does not scale with speed.** Honda et al. (2016)
  found 30°/s and 60°/s indistinguishable; Lindau found no relation to
  head velocity at all. Unlike vision's saccadic suppression, faster
  turns do not hide more — so "the artifact only appears during fast
  motion" is not by itself a defence.

Near-field is a **requirement, not a nicety**: the state vocabulary's
attenuation up to 3.0 exists "for whispers and private communication" —
whisper-range sources are exactly where head-centered ambisonics fails
and bilateral pays.

## The reframe that partially dissolves it: split the rotation

Borrowed from VR video reprojection/timewarp: **the server renders
against the freshest pose it has; the client applies only the residual
rotation** (delta between rendered pose and current pose) locally in the
render callback. The residual is set by head *velocity* × loop latency,
and small rotations are cheap and exact in the SH domain.

**Velocity figures corrected 2026-07-25** against measured data (see
`rotation-correction-sidecar.md` §What the literature says). This
section originally assumed "~18° worst case at 60ms and a fast 300°/s
turn". Measured head velocities in listening tasks:

| Regime | Velocity | Residual at 60ms |
|---|---|---|
| Natural listening | ~34–45°/s | **~2–3°** |
| Deliberate latency-hunting | ~190°/s mean | ~11° |
| Observed peak (Lindau) | 942°/s | ~57° |

Sources: Brimijoin, Boyd & Akeroyd 2013 (34°/s, 29°±12° excursion);
Brimijoin & Akeroyd 2014 (~45°/s peak); McLachlan et al. 2023
(36.4 ± 23.0°/s); Lindau 2009 (190°/s mean, 942°/s max, but subjects
were actively searching for latency). **This cuts both ways**: the
typical residual is far smaller than assumed — well under Honda et
al.'s 17.7° detection threshold during rotation — which lowers both the
stakes and the benefit of any correction scheme. Reprojection earns its
keep in the fast-turn tail, not in conversational listening.

Under this scheme:

- **Plain binaural stereo cannot be reprojected** — rotation is baked
  into the rendered pair. It keeps the full network loop in the rotation
  path, and is therefore the thin-client fallback, not the quality path.
  (Server-side pose prediction softens it but fails on direction
  reversals, when the ear is most alert.)
- **Head-centered ambisonics reprojects exactly** (rotation commutes
  with SH) but forfeits near-field — and is still per-listener (the
  expansion point translates with the listener), so it pays per-listener
  render+encode without the per-listener quality.
- **Bilateral reprojects almost-exactly**: rotating each ear-field about
  its own center captures the rotational component exactly; the
  uncorrected part is only the *translational* component of ear movement
  under the residual angle (~2–3cm transient at 18°, refreshed at server
  update rate). Errors scale with head velocity, not loop latency.

Translation (walking) stays server-side in all variants; translation
latency tolerance is loose.

## The candidates

| Format | Ch | Streams | Rotation path | Near-field |
|---|---|---|---|---|
| `binaural` | 2 | 1 stereo | full network loop | yes (server render) |
| `ambi2`/`ambi3` (head-centered) | 9/16 | uncoupled | local, exact | no |
| `bilateral2` | 18 | 9 coupled | local residual, ~exact | yes |
| `bilateral3` | 32 | 16 coupled | local residual, ~exact | yes |

**The coupling observation.** The two ear fields are the same scene seen
from points 17cm apart — massively correlated. Pairing corresponding ACN
channels of the left and right fields into coupled stereo streams is
**the one place in this design where Opus stream coupling naturally fits
the signal** (raw ambisonic channels are not stereo-like; L-field-ch-i /
R-field-ch-i genuinely are). No projection matrix needed. Expected cost
of bilateral over head-centered: plausibly ~1.3–1.6×, not 2× — to be
measured.

**Rotation stability sets an order floor — added 2026-07-25.** The
candidates table's "local, exact" entry for head-centered ambisonics is
mathematically true but perceptually incomplete. Ben-Hur, Alon, Rafaely
& Mehra (2019, *EURASIP J. Audio Speech Music Process.* 2019:5) used
**scene stability under head rotation** as the dependent variable and
found SH order truncation makes the HRTF magnitude develop azimuthal
ripples, so level and timbre swing as a function of head angle.
Loudness-stability MUSHRA medians against a 100-point reference:

| SH order | N=2 | N=4 | N=10 | ref (N=27) |
|---|---|---|---|---|
| Median | **14** | **27** | 68 | 100 |

F(3,198) = 100.79, p < 0.001; N=4 vs N=2 not significant (p = 0.85).
Counter-intuitively, *adding* aliasing improved low-order stability
(N=4: 26.5 → 53.5, p = 0.003) because aliasing energy partly replaces
the HF energy truncation removed.

The rotation is exact; the *truncated HRTF* is not uniform over azimuth
and frequency, so the error becomes head-angle-dependent. Implication
for this decision: **`ambi2` rotated locally may be timbrally unstable
under rotation in a way the candidates table does not capture** — the
very thing local rotation is supposed to fix. Caveat before over-reading
it: that study truncated the *HRTF* representation, whereas `ambi2`
truncates the *scene* and the client may decode with a full-resolution
HRTF, which is not the same experiment. Whether the ripple mechanism
transfers is unverified. Worth adding as a listening condition —
head-rotation timbral stability of `ambi2`, separate from static
quality. It is also a point in the sidecar's favour, since that scheme
rotates only a small additive correction rather than the whole signal.

**The lower-order claim.** The bilateral literature's selling point is
that ear-centered expansions get ITD/ILD right at orders where a
head-centered expansion doesn't. So the live matchup is **bilateral-2
(18ch) vs head-centered ambi3 (16ch) at ~equal channel count**, and
bilateral-2 may win on the things that matter here.

## Ambisonic order vs binaural perception — added 2026-08-04

Research pass (Paul's question: is 5th order much better than 3rd,
and does the bilateral approach change the answer?), folded in here
because it reprices several rows above. Companion engine work landed
the same day: bus orders 2–5 are all verified end-to-end
(`TestBinauralAllOrders`; order 1 dropped) and priced
(`TestOrderBudgetSweep`, dev Mac, 50 people: 505/541/614/723 µs
median for orders 2/3/4/5 — sublinear in channels; CPU is not the
binding constraint).

**Head-centred: 3→5 is real but past the knee.** Order-N
reconstruction is accurate at the ear positions only to ≈ N × 600 Hz
(ears sit ~8.7 cm off-centre): order 3 covers the ITD band
(< 1.5 kHz), order 5 reaches ~3.1 kHz, and *neither* approaches the
pinna-cue band — perceptual transparency needs on the order of
**N ≈ 20** (arXiv 2412.05015). The perceptual studies match:
localisation improves only "minor[ly]" from 3rd to 5th (both vastly
better than 1st; RG 347336527, RG 253644795); source sharpness keeps
improving with diminishing returns; and with **MagLS-family decoding
the timbral MUSHRA differences across orders 1–5 were minimal**
(Huddersfield, pure.hud.ac.uk) — magnitude fitting buys back most of
what truncation costs. The genuinely order-sensitive head-centred
weakness remains rotation stability (the Ben-Hur 2019 ripple data
above).

**Bilateral changes the answer.** The HRTF's high effective SH order
is mostly its ITD phase; bilateral + ear-aligned HRTFs remove exactly
that from the SH machinery's job (Ben-Hur, Alon, Mehra & Rafaely
2021, IEEE/ACM TASLP — "substantial improvement … even with SH as
low as first order"). The engine implements the full recipe — two
ear-centred expansions, explicit Woodworth ITD, per-ear parallax,
per-ear NFC, **BiMagLS** filters (`thk-ku100-bimagls-v1`; Engel et
al. 2022 identify BiMagLS as the right preprocessing for bilateral).
Consequences: (1) much of what head-centred 5 buys over 3 — ITD-band
accuracy at the off-centre ears — bilateral gets exactly at any
order, so the remaining 3→5 delta is finer magnitude-HRTF detail
(sharpness, dense-scene separation): real, second-tier, settle by
ear (`PANAUDIA_ORDER=5` is a cheap A/B now); (2) the rotation-ripple
concern is blunted — ITD is recomputed per frame from geometry,
never reconstructed from a truncated field, and BiMagLS smooths the
magnitude ripple; (3) near-field quality is order-independent for
us — explicit models, not field reconstruction.

**A product note beside the psychoacoustics (Paul, 2026-08-04):**
there is an advantage to *offering* 5th order for the binaural path
even though 3rd is probably more than good enough — "5th-order"
sounds more attractive. Perceived quality in the market sense is a
real axis the studies above don't measure, and the measured cost is
modest (+34% render median, order 3→5). The order knob already
allows it; this note records that running spaces at 5 may be worth
it for positioning alone.

**The asymmetry that matters for this doc's decision:** the
bilateral advantage lives ONLY in the server's binaural path. Raw
`ambi2`/`ambi3` sink formats are head-centred — their consumers face
the classic order-limited story above (unless the client itself
decodes MagLS-style). So "our binaural at order 3 ≈ head-centred
order 5" does NOT transfer to the wire formats, and the order
question must be asked separately per row of the candidates table.
It also sharpens the earlier lean: the formats that carry the
bilateral advantage to clients are the deferred `bilateral2/3`, not
higher-order head-centred ambi.

References for this section: Ben-Hur et al. 2021
(doi 10.1109/TASLP.2021.3055038); Engel, Goodman & Picinali 2022
(Acta Acustica, aacus210029); Armstrong et al., Huddersfield MUSHRA
(orders 1–5, magLS); arXiv 2412.05015 (transparency ≈ order 20);
RG 347336527 (VR localisation by order); RG 253644795 (FOA/HOA
localisation); JASA 143:3616 (time-aligned HRIR rendering).

## Server cost (may bind before bandwidth)

Per-listener work:

| Format | Render | Encode streams |
|---|---|---|
| `binaural` | ambi + HRTF decode | 1 stereo |
| `ambi2` | ambi | ≤9 mono |
| `bilateral2` | 2× ambi | 9 coupled stereo |
| `bilateral3` | 2× ambi | 16 coupled stereo |

Encode dominates at 200 pkt/s. Bilateral-2 is a clean ~44% stream cut vs
bilateral-3 — but likely **more** server CPU than plain `binaural` (sheds
the HRTF decode stage, takes on 9 encodes vs 1). Any "bilateral saves
CPU" claim is true against bilateral-3, false against binaural, until
measured.

## Current lean (recorded as lean, not decision)

**Bilateral-2 as the workhorse sink format.** Ground truth in its
favour: head-centered 2nd order is already "very usable" for binaural
output in current first-party experience, and bilateral-2 should be ≥
that at the same order while adding near-field and local rotation.
Honest caveat: what has been validated by ear is *head-centered* 2nd
order; bilateral's superiority at order 2 is the literature's claim, not
yet a listening result.

The decision this actually is, under subscription-as-negotiation (no
protocol "default" exists): the **mandatory-to-implement set** — which
sink formats a conforming space MUST offer. Sketched shape, unmade:
mandatory `bilateral2` + `binaural` (quality path + thin-client floor),
optional `ambi2`/`ambi3` (recording, broadcast, external processing) and
`bilateral3` (premium tier). Every mandatory format is server surface
forever.

Everything here is additive under format-in-name: new track names, no
architecture change. The client-side residual-rotation rule would ship
as a non-normative note (same spirit as the catch-up strategies note in
`state-reconnect-and-gc.md`).

## Server-side pose prediction (dead reckoning) — idea added 2026-08-05

*Context for this entry: the TS client's browser latency harness
(latency-baseline.md, 2026-08-05 rows) measured the sink-pose loop at
50–53 ms to the render point on loopback; with realistic network
(~40 ms RTT) and device output latency the estimated real-world
head-rotation loop is ~120 ms — usable for audio-only listening, above
the critical-listener thresholds in §"The dilemma". The rendering
stays server-side, so the whole bilateral/near-field advantage is
kept; this idea attacks the residual latency without moving any
rendering to the client.*

**The idea (Paul).** Game netcode routinely dead-reckons *other*
players from previous positions, timings, velocities, and expected
behaviour patterns. Invert it: the **server predicts each listener's
own head pose ~one loop-latency ahead** (per-client horizon ≈ uplink
+ render + downlink + playout fill + device output, i.e. the ~100 ms
the listener will wait to hear this frame) and renders the sink with
the predicted pose. If prediction is good, perceived rotation latency
collapses toward zero while the architecture stays exactly as built.

**Why it might work.** The server already has the ideal inputs: a
5 ms-cadence pose stream per entity (the SAB-fresh client stamping
means samples are near-sensor-fresh), so angular velocity/acceleration
are directly estimable. VR compositors do precisely this at 20–30 ms
horizons as standard practice; 100 ms is more ambitious but head
motion is favourable material — mostly stillness (prediction trivially
exact), and turns are ballistic with stereotyped bell-shaped velocity
profiles rather than white noise. Literature on head-motion prediction
(Kalman / polynomial extrapolation) reports usable errors at ~100 ms
horizons during motion, near-zero at rest.

**Expected failure mode.** Overshoot at turn onset/stop — naive
constant-velocity extrapolation at 100 ms with peak head velocities of
300–800°/s can err by tens of degrees for a few frames. Whether
overshoot-then-settle *sounds worse than* honest lag is a perceptual
question, not an engineering one (plausibly: overshoot during fast
motion is masked by the motion itself — same regime where the
detection thresholds are most forgiving per Lindau's velocity-scaled
budget; but it must not breathe at rest, so any predictor needs a
stillness gate / velocity-scaled blend to zero horizon).

**Cost.** Server compute is negligible: per sink per tick, a velocity
estimate + one quaternion (or yaw-dominant) extrapolation — a handful
of scalar ops against a measured render budget of 1.08 ms/tick at
50×50. The real costs are (a) tuning/validation effort, (b) a
per-client horizon estimate — RTT is observable at the transport, but
playout fill + device output latency live client-side, so the client
should report its render-point + device latency (a state key or config
field; the TS client can already measure both), and (c) the perceptual
evaluation itself.

**Cheap first evaluation (no server changes).** Record real
head-tracking traces (or reuse published datasets), run offline
extrapolation at 50/100/150 ms horizons, and plot angular-error
distributions during still/slow/fast segments. That prices the idea
before any engine work. Second step if it survives: a client-side
mirror experiment — apply the same predictor to `setPose` in the TS
client (prediction lever 1 from the latency discussion) — which needs
no server change at all and A/B's by ear with the artificial-delay
knob. Server-side then only wins over client-side prediction by using
fresher samples (it predicts from poses ~half a loop newer) and by
centralising the tuning.

**Interaction with this doc's trade space.** If prediction holds up
perceptually at ~100 ms horizons, it further weakens the case for
mandatory client-side rotation formats (bilateral streaming / the
sidecar) as *latency* solutions — they would remain quality/bandwidth
plays only. It composes with, rather than competes against, the
existing candidates.

## What would discriminate — the pending evaluations

In leverage order:

1. **Bilateral-2 vs head-centered listening test** (vs both ambi2 and
   ambi3) — validates the whole bet; the planned listening sessions.
   Add a **rotation-stability** condition per the Ben-Hur finding
   above: timbral/loudness stability while rotating, scored separately
   from static quality.
2. **Coupled-pair coding efficiency** on real bilateral material —
   prices the bandwidth premium (the 1.3–1.6× guess).
3. **Per-listener encode benchmark** at target listener counts — decides
   whether the quality path scales or stays a premium tier, and whether
   `binaural` remains the mass-audience format.
4. **Residual-error check** — does the ~2–3cm transient ear-translation
   error under fast rotation register audibly? It is the one
   approximation the reprojection scheme cannot remove.

## If bilateral lands, the packet doc needs

- `bilateral2`: 18 channels as 9 coupled stereo streams pairing
  corresponding ACN channels of the two ear fields, SN3D per field;
  `bilateral3` analogous at 16 coupled streams. (Pinned per track name —
  format-in-name is the negotiation, as for `ambi2`/`ambi3`.)
- The non-normative residual-rotation client note.
- A revisit of the head-centered ambi arrangement question
  (`audio-packet-structure.md` §7): if bilateral becomes the headphone
  path, the uncoupled-vs-projection evaluation for head-centered formats
  loses urgency — their remaining consumers are pipelines, not ears.
