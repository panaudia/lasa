# FOA rotation-correction sidecar — DEFERRED

*Opened 2026-07-25. **Deferred 2026-07-27** along with the parent
head-rotation question (`head-rotation-and-sink-formats.md`): rotation
latency is postponed as a future piece of work; v1 ships plain
`binaural`/`ambi2`/`ambi3`. The listening rig planned for this week is
postponed with it. Everything below — the two architectures, the
literature survey, the planned evaluations — is preserved for that
future phase. Nothing here is spec.*

## The idea

Keep the full server-side quality path — bilateral-3 render, HRTF decode
to binaural on the server — and make its rotation lag patchable:
alongside the binaural, stream **low-bitrate first-order components**
and **the rotation the server rendered at**. The client compares the
server's (older) rotation with its current one and uses the first-order
material to correct the rotation locally.

## Why the mechanism is sound

Binaural rendering is linear in the scene, and rotation does not mix
ambisonic orders — order ℓ components rotate only among themselves. So
the wanted-minus-sent difference decomposes order by order:

```
B(θ_now) − B(θ_server) = Σ_ℓ  D_ℓ · [R_ℓ(θ_now) − R_ℓ(θ_server)] · A_ℓ
```

With the first-order scene A₁ and a first-order binaural decode D₁, the
client computes the ℓ=1 term exactly and adds it to the server's
binaural. Orders ≥2 keep the stale rotation — audio timewarp: correct
the dominant part, let the detail lag.

Two consequences fall straight out of the math:

- **W drops out.** Order 0 is rotation-invariant, so the ℓ=0 term is
  identically zero. The head-centered sidecar is **3 dipole channels**,
  not 4 — W is never sent.
- **First order is where the dynamic cues live.** Low-frequency ITD and
  broad ILD — the cues that make rotation lag audible — are dominated
  by the first-order field. Orders ≥2 mostly carry image sharpness and
  timbre, the part the ear tracks least urgently during motion.

## Two architectures

### A. Differential correction (the proposal)

Server sends its normal full-quality binaural + lossy FOA dipoles + the
render rotation. Client adds `D₁ · [R(θ_now) − R(θ_server)] · FOA`.

The killer property: **at rest the correction is identically zero** —
including all of the sidecar's coding noise. Artifacts scale with the
rotation delta, so the sidecar can be crushed hard: coding noise
appears only during motion, exactly when the ear is least analytical
and when the alternative (lag) is worse. Steady-state output is the
server's binaural, bit-exact.

Implementation note: the correction factors into *one* small FOA
binauralisation — apply the 3×3 matrix difference to the dipoles, then
a single 3→2 decode convolution. Cheap on any client.

### B. Layer split (the fallback shape)

Server renders binaural from orders ≥2 only; streams that plus a
full-quality FOA base (W included, 4ch, world/head-static frame). The
client rotates and binauralises the base itself and sums. Roughly the
YouTube-VR "ambisonic bed" shape.

Genuine attractions, recorded as the steelman:

- No subtraction → the ghosting failure mode structurally cannot occur.
- **Tiered for free**: the base is decodable alone — a thin client
  takes FOA-only and gets a complete soft-focus rotatable mix; the
  residual is an enhancement layer. (The differential's sidecar is
  parasitic — dipoles without W are not listenable content.)
- Each bit sent once (the differential ships first-order information
  twice: inside the binaural and again in the sidecar).
- No render-pose metadata needed.
- Closer to deployed art — some validation is borrowed.

Why it was judged the weaker fit for LASA's stated goals:

- **The seam is permanent, not motion-gated.** Every source's energy is
  split across both layers (ℓ≤1 client-rendered, ℓ≥2 server-rendered),
  so decoder mismatch produces an inconsistent composite HRTF for every
  source, *all the time*. The differential has the same matching
  requirement multiplied by the rotation delta — zero at rest. Known-art
  caveat: deployed bed-plus-stereo schemes put *different content* in
  each layer (ambience vs dialog); splitting one scene's order
  structure across two renderers is not the validated configuration.
- **Concedes render quality on the dominant components.** W and the
  dipoles — most of the energy, and the low-frequency ILD where
  near-field proximity cues live — become client-rendered and
  head-centered. Near-field whispers are a requirement
  (`head-rotation-and-sink-formats.md`); the split compromises them
  permanently, the differential only transiently during motion.
- **Loss behaviour inverts.** Differential: lose the sidecar → complete
  full-quality binaural, merely laggy. Split: lose the base → a hole
  where most of the mix's energy was; the load-bearing layer is the one
  that is not the server's render.
- **Bandwidth runs the wrong way.** The base must sound good in
  isolation (~4ch at full rate, ~130–190 kbps, plus the residual
  binaural ≈ 200–280 kbps total) vs the differential's
  96 + ~48 + ~10 ≈ 155 kbps.

**Per-regime summary.** Split wins under: heterogeneous client tiers
wanting a one-track scalable story, congestion adaptation by
layer-dropping, or listening results showing motion-gated ghosting is
worse than a permanent seam. Differential wins under LASA's stated
regime: maximum server render quality at rest, near-field as a
requirement, rotation lag as the thing being patched.

## The sidecar variants: 3ch vs 6ch

- **3ch head-centered** (dipoles of the head-centered field): captures
  rotational cue changes; cannot capture how near-field per-ear gains
  shift under rotation. Cheapest.
- **6ch bilateral** (3 dipoles per ear-field, rotated each about its own
  center — the same "rotation is exact per ear-field" property recorded
  in `head-rotation-and-sink-formats.md`): tracks per-ear cue changes
  including most near-field behaviour, leaving only the ~2–3cm
  ear-translation transient uncorrected. The two ear-field dipole sets
  are correlated pairs — the natural Opus coupling fit again — so
  3 coupled stereo streams, plausibly ~70 kbps rather than 2×.

## Costs (house denominator: 1 byte = 1.6 kbps at 200 pkt/s)

- Sidecar at ~16 kbps/ch: ~48 kbps (3ch) or ~70–96 kbps (6ch coupled)
  on top of 96 kbps binaural.
- Render-rotation metadata: 3 × int16 angles = 6 bytes = 9.6 kbps,
  per-packet (the server's render pose changes every frame). Reuses the
  angle encoding of `audio-packet-structure.md` §4.
- **Server CPU**: keeps the most expensive *render* (bilateral-3 + HRTF
  decode) but the cheapest *encode* of any quality path — 1 stereo +
  3–6 low-rate channels ≈ 3–4 encode-stream-equivalents vs
  bilateral-2's 9 coupled streams. Since encode dominates per-listener
  cost at 200 pkt/s, this may undercut bilateral-2 on CPU while beating
  it on bandwidth. To be benchmarked, not assumed.
- Downstream bandwidth ~155 kbps vs bilateral-2 streaming (18ch) —
  dramatically cheaper.

## Failure modes, priced

1. **Decode-mismatch ghosting** (the serious one, differential). The
   correction implicitly subtracts `D₁·R(θ_s)·A₁`, assuming the
   client's D₁ matches the effective first-order transfer inside the
   server's bilateral render. Mismatch (HRTF family, group delay) →
   phasey doubled image — but only during motion, scaled by delta.
   Mitigation: make D₁ part of the format contract — the spec pins (or
   the server ships) the exact correction filters, delay-matched to the
   render. The 6ch variant helps: per-ear fields make "what the server
   embedded" better-posed.
2. **Split-image blur**: ℓ=1 tracks, ℓ≥2 lags — during a fast turn the
   sharp image momentarily smears around a correctly-placed broad
   image, healing at loop latency. Predicted better than whole-image
   lag; only ears can confirm. (Both architectures share this
   behaviour identically.)
3. **Alignment**: the correction must be sample-aligned with the
   binaural. Same datagram solves it structurally — one packet per
   frame, both payloads through the same jitter buffer; correction
   applied at render-callback rate with per-block interpolated rotation
   matrices (zipper noise otherwise).
4. **Low-rate CELT roughness** at 5ms/16 kbps is real in isolation but
   delta-gated — unusually favorable masking geometry.

## Fit with existing decisions

Fully additive under format-in-name: a new sink format name (binaural
plus sidecar *is* a different format; subscription is the negotiation),
a new layout row:

```
binaural+corr:  [flags][ts?][render-rot(6)][len(2)][opus-stereo][len(2)?][opus-foa]
```

(Exact layout to be pinned in `audio-packet-structure.md` if this
lands; redundancy interplay with the dup scheme to be worked through
then.) No change to MoQ-id sequencing or the flags machinery.

**Knock-on for the sink-format menu**: if this works, plain-binaural
clients get a rotation-corrected quality path at ~5 channels, which
*weakens* the case for making `bilateral2` streaming mandatory —
bilateral-2 becomes the "full local rotation, no correction machinery"
alternative rather than the only quality path. The
mandatory-to-implement sketch in `head-rotation-and-sink-formats.md`
should be revisited against this.

## What the literature says (surveyed 2026-07-25)

Motivating question: is there an auditory analogue of *saccadic
suppression* — are we untroubled by spatial "blur" during head rotation
the way we are during fast eye movements? Four sweeps. Short answer:
**no gating, but real blur, arriving by a different mechanism — and
critically, the tolerance does not grow with rotation speed.** Full
reference list at the end.

### There is no auditory saccadic suppression

Direct tests come out null. Harris & Lieberman (1996) found auditory
detection, pitch discrimination and localization all unaffected by
saccade timing; a well-powered 2023 replication confirmed it with
Bayes factors favouring the null, and specifically ruled out
eye-movement-related eardrum oscillations as a modulator despite those
being present and saccade-locked. **There is no shutter.**

What exists instead is blur from **integration**. The binaural temporal
window has an equivalent rectangular duration of about **140 ms**
(Akeroyd & Summerfield 1999) — roughly an order of magnitude longer
than the monaural window — and modern snapshot models put spatial-
percept integration near **300 ms** (Roggerone et al. 2019). Grantham &
Wightman (1979) showed a ±1.0 interaural correlation swing at 4 Hz is
smeared away entirely.

*Framing worth keeping*: LASA's packets are 5 ms; the binaural
integration window is ~140 ms. The protocol operates ~28× faster than
the perceptual mechanism it feeds.

### The acuity loss during rotation is large and well measured

| Comparison | Result | Source |
|---|---|---|
| Moving vs static MAA, matched conditions | 2–3× worse | Carlile & Leung 2016 (review) |
| Same, single study | 8.8° vs 3.4° | Chandler & Grantham 1992 |
| **Detecting source movement: still head vs rotating head (63.5°/s)** | **3.6° → 17.7° (~5×)** | Honda et al. 2016 |
| Just-audible BRIR grid coarseness, head moving | 4–18° (vs ~1° static MAA) | Lindau et al. 2009 |
| Passive whole-body rotation, straight-ahead JND | 1.0° → 1.8° | Kanno et al. 2022 |

Honda et al. (2016) is the closest paper to the question — it measures
exactly "how far can a sound move during a head turn before you notice."

### ★ The tolerance is velocity-independent — the finding that changes design thinking

- Honda et al. (2016): 30°/s and 60°/s **indistinguishable** (p = .28).
- Lindau (2009): latency detection threshold vs head velocity across a
  **mean of 190°/s, peaks of 942°/s — no relation at all**; the best
  subject was a slow mover; the best linear fit was a constant.
- Rappin et al. (2023): imposed 60/180/360°/s → significant but tiny,
  **~12 ms of threshold across a 6× speed range**, saturating above
  180°/s.

This is where the eyesight analogy breaks, and it matters here.
Saccadic suppression *scales with the saccade* — faster eye movement
hides more. Auditory motion tolerance is a **constant offset, not a
function of speed**. A scheme whose artifacts scale with rotation delta
therefore gets **no matching increase in perceptual cover** during the
turn. Delta-gating still buys silence *at rest*, which is real and is
the sidecar's central argument — but it does not buy a growing masking
allowance while moving.

### Three findings that cut against the hope

1. **Self-motion-correlated error is detected *better*, not worse.**
   Brimijoin & Akeroyd (2014): identical relative motion gives a moving
   MAA of **5.4° when the listener moves vs 6.6° when the source
   moves** — the system partially corrects for self-generated motion,
   so errors correlated with one's own head turn are the class it is
   best equipped to catch. The correction residual is exactly that
   class. (Both remain worse than the ~3–4° static baseline, so motion
   is still net cover — just less than the raw 5× suggests.)
2. **Listeners are temporally forgiving and geometrically strict.**
   Grimaldi et al. (2023) separated the artefact types: **400 ms of
   pure tracking latency did not degrade azimuth localization, while
   estimation mismatch — the rotation being the wrong *amount* — did.**
   A partial correction is a gain error, i.e. the failure mode they
   found people sensitive to. Caveats: the tested magnitudes are
   paywalled (direction only), and localization accuracy is not the
   real DV here — believability is.
3. **No documented masking of spectral or coding artifacts during
   motion.** Searched specifically, twice. Everything supporting
   "artifacts are masked during rotation" is for **positional** error.
   **No published work on codec artifact audibility under head motion
   exists at all** — that search space is empty. Nearest real evidence:
   Hoffmann & Møller found broadband, modulated content masks HRTF
   switching artifacts *within-signal*; Lindau & Roos found MTB
   switching artifacts "always clearly audible" with noise but often
   inaudible with speech and music. **The masking this scheme counts on
   is content-dependent and, for this artifact class, an extrapolation
   the literature does not license.** This is the single largest
   recorded risk to the design.

### Findings that support the premise

- **Getting rotation right may buy more than getting the render
  right.** Brimijoin, Boyd & Akeroyd (2013): with a badly degraded
  BRIR, static externalization was **10%**, but world-stabilized head
  motion raised it to **50%** — 5× from correct rotation *despite* the
  spectral degradation. And a head-locked scene under head motion
  scored **38% vs 89%** for a static head: **untracked motion is
  actively worse than no motion.** Strongest evidence found for the
  "rotation lag breaks believability" premise, and it tilts the
  server-quality-vs-local-rotation trade toward rotation.
- **Perceptual compensation for head rotation is ~85%, not 100%.**
  Freeman et al. (2017) measured the point of subjective stationarity
  at movement gain **+0.15**, invariant across 20/60/100°/s and across
  azimuth — an auditory Filehne illusion in which a genuinely
  world-stationary source is heard drifting slightly against the turn.
  **There is no perceptually neutral rotation gain**: exactness is not
  the target, and a partial correction may sit closer to the perceptual
  null than its numbers suggest.
- **Latency thresholds bracket LASA's loop** — median listeners
  60–110 ms, critical listeners 44–52 ms. Detailed below.

### The acceptable-latency budget

The literature's answer is a **range spanning 30 ms to over 200 ms**,
and almost all of that spread is methodology rather than perceptual
disagreement. All figures are *total* motion-to-sound latency including
each study's own baseline — the currency LASA cares about.

| Study | Paradigm | Mean threshold | Best listener |
|---|---|---|---|
| Brungart et al. 2005/2006 | yes/no vs 11.7 ms baseline, free head movement | 82 ms | 60 ms |
| — same, + co-located zero-latency reference tone | | 61 ms | 36–38 ms |
| Yairi et al. 2006 | absolute judgement (DT) | 94 ms (42–132) | 42 ms |
| — same, paired comparison (DL) | | 70 ms (30–118) | 30 ms |
| Lindau 2009 | adaptive 3AFC, Best-PEST | 107.6 ms (σ 30.4) | 52 ms |
| Rappin et al. 2023 | yes/no "did you feel latency" | 195–207 ms | 44 ms |

**Four drivers of the spread.** Knowing which apply to LASA is what
turns this into a budget:

1. **Whether a low-latency reference exists** — the biggest lever.
   Brungart's listeners improved **23–25 ms** with a zero-latency tone
   present, and that benefit **vanished with only 5° of angular
   separation** between reference and target. The reference must be
   essentially co-located to help. → **For headphone-delivered virtual
   scenes with no co-located real sound, the no-reference (higher)
   numbers apply.** This is also why Brungart recommends <30 ms for AR
   and <50 ms for VR: in AR the real world *is* the co-located
   reference. *(Open for LASA: a visual avatar may act as a
   near-zero-latency cross-modal reference and pull thresholds down.
   Sankaran et al. 2016 (Oculus) tested exactly this with a
   zero-latency visual stimulus, but the numbers are unobtainable —
   meeting abstract only. Relevant only if panaudia is not audio-only.)*
2. **Paradigm sensitivity.** Rappin's 195–207 ms medians do not
   contradict Lindau's 107.6 ms — they used non-expert listeners
   answering "did you feel latency" rather than adaptive forced choice.
   Their *critical* subjects reached 44 ms, against Lindau's 52 ms.
   **Look at the sensitive tail and the studies converge tightly**; the
   medians are where the paradigm shows.
3. **Individual differences dominate every dataset.** Lindau 52–187 ms,
   Yairi 30–118 ms, Brungart 60–120 ms; Lindau's Cronbach α = 0.7 says
   this is real between-subject variance, not noise. There is no single
   population threshold — there is a distribution with a long sensitive
   tail, and that tail is what a quality-first protocol designs for.
4. **Scene complexity *raises* the threshold.** +10 ms for a five-source
   scene (Stitt et al. 2016), independently replicated in direction by
   Rappin (+63 ms, pink noise → busy café). Mechanism appears to be
   source **continuity**, not source count. **Convenient for LASA: a
   busy space is more forgiving than a single talker — the opposite of
   how bandwidth pressure runs.**

**Independent theoretical convergence.** Lindau derived a budget from a
completely different direction — Perrott & Musicant's minimum audible
movement angles, asking how long you can lag before accumulated angular
error exceeds the MAMA at a given head speed: **65.6 / 45.6 / 26.7 ms
at 90 / 180 / 360°/s**. The 180°/s figure lands almost exactly on the
empirical critical-listener numbers (44 and 52 ms). Psychophysical
measurement and first-principles derivation agree.

**Standards are thin.** No numeric motion-to-sound requirement found
from **MPEG-I, ITU or EBU** — MPEG-I says only "below the perceivable
threshold". The only hard institutional number is **Android's 150 ms
maximum** head-tracking latency (pose reported ~every 20 ms, 40 ms max
gap), which is a don't-be-absurd bound, not a quality target. A claimed
<20 ms for 3GPP ISAR traces to a glossary site, not the spec. The most
useful calibration is a measurement rather than a standard: HEAD
acoustics instrumented a real-time **3GPP IVAS binaural renderer** with
a motorised HATS and measured **93.4 ms mean motion-to-sound**
(Kentgens et al. 2025) — a shipping-grade split-rendering system
sitting right at the median detection threshold.

**Where LASA's 40–80 ms loop sits:**

- **Below median detection** (60–110 ms) — most listeners will not
  reliably notice it.
- **Inside the critical-listener band** (44–52 ms) — expert listeners
  will, at the upper half of the range.
- **Above every engineering recommendation** — <30 ms (Brungart, AR),
  <40 ms (Rappin), <50–60 ms (Brungart, VR).

For a quality-first protocol whose author is himself a critical
listener, the sensitive-tail numbers are the ones to design against.
That makes the **top** of the range genuinely problematic and the
bottom acceptable — which is an argument for treating the pose fast
path's 15–25 ms saving (`audio-packet-structure.md` §8) as
**load-bearing rather than a nice-to-have**: it is roughly the
difference between the two ends of that judgement.

**★ The caveat that matters most.** Every number above is a
**detection** threshold — "can you tell this from a lower-latency
version in a controlled comparison". **None of them measure when
rotation lag breaks believability**, which is the actual design
concern. The two come apart, with strong evidence in both directions:
Grimaldi et al. found **400 ms of latency did not degrade azimuth
localization at all**, vastly above any detection threshold; while
Brimijoin, Boyd & Akeroyd found a **head-locked** scene (effectively
infinite latency) collapses externalization to 38% against 89% static.
So somewhere between "undetectable" and "head-locked", latency stops
being a curiosity and starts destroying the illusion — **and nobody has
mapped that curve.** The detection literature is well-populated; the
acceptability literature, in this sense, essentially does not exist.

Consequence for the rig: **detection thresholds are the wrong dependent
variable.** Score **externalization or plausibility** against a latency
sweep instead. That would be a genuinely novel measurement and sits
naturally alongside the conditions already planned.

### Three gaps that sit precisely under this scheme

1. **Nobody has studied different parts of one scene, or different
   frequency bands, rotating by different amounts.** That is exactly
   the ℓ=1-tracks-while-ℓ≥2-lags condition (§Failure modes 2), and it
   is unmeasured. Nearest evidence: Brimijoin & Akeroyd (2012)
   inverted the dynamic cue entirely and found the **dynamic cue wins
   outright over true position** for lowpass sounds, while **full-band
   signals produced flickering, bistable front/back percepts**, with
   spectral energy **above 4 kHz protecting** against capture by the
   false cue. → **Bistability, not graceful blur, is the failure mode
   to listen for; HF content is what anchors true position.**
2. **No academic treatment of audio reprojection exists.** The prior
   art is industrial: **3GPP ISAR (TS 26.249)** split rendering — server
   renders, UE applies pose correction for last-moment head movement —
   and **Meta patent US11102602B1** (Brimijoin, Hassager, Amengual
   Garí — the same psychoacousticians), which corrects rendered audio
   using **ITD and ILD adjustments rather than a sound-field
   rotation**. That divergence is worth understanding before the tests:
   they chose a cue-domain correction where this design chooses an
   SH-domain one.
3. **The latency *acceptability* curve is unmapped** — see §The
   acceptable-latency budget. Detection thresholds are thoroughly
   measured; the point at which lag stops being detectable-but-fine and
   starts destroying externalization is not. This determines which
   dependent variable the rig should score.

### Net read

The literature supports the **premise** — rotation lag genuinely costs
believability, and spatial acuity really is degraded during motion —
but does not yet support the **mechanism being relied on**: that
timbral artifacts are masked during rotation, or that partial-order
correction reads as better than uniform lag. The listening test is
doing real work, not confirming known art. This strengthens rather than
weakens the case for running it, but it lowers the prior on the scheme
working first time.

## Current lean (recorded as lean, not decision)

**Differential over split**, because the split's apparent safety is
partly illusory — the matching problem doesn't disappear, it becomes
always-on — and because the differential preserves the server render
bit-exactly at rest, confining every artifact to moments of motion.
Held as a lean pending the listening rig.

Honest unknown: the differential form (delta-gated correction against a
shipped server pose) is not known deployed art — head-locked-plus-bed
splits are; the differential is at least uncommon. No borrowed
validation; the listening test is doing real work.

## The discriminating evaluations (planned week of 2026-07-27)

Unusually cheap: **no network needed**, all offline. One rig tests
both architectures:

1. **Differential ghosting**: render `B(θ_s)`, build the correction
   from bitrate-crushed FOA, compare against true `B(θ_now)` sweeping
   the delta over realistic head-turn profiles. **Use measured
   velocities, not worst-case guesses**: natural listening is
   ~34–45°/s (Brimijoin 2013; Brimijoin & Akeroyd 2014; McLachlan et
   al. 2023) giving a 2–3° residual at a 60 ms loop; ~190°/s mean is
   what subjects produce when *actively hunting for latency* (Lindau
   2009), giving ~11°. Include reversals — prediction fails there.
2. **Split seam**: steady-state A/B of base+residual against the
   unsplit render — can you hear the seam at rest, with delay-matched
   filters?
3. **3ch vs 6ch** variants, especially on a near-field whisper source.
4. **Deliberately mismatched D₁** — how much decoder mismatch is
   tolerable before ghosting registers (calibrates how tightly the
   format contract must pin the correction filters).

Four conditions the literature says to add:

5. **Listen for bistability, not blur.** Brimijoin & Akeroyd (2012)
   found inconsistent dynamic cues produce *flickering, bistable*
   front/back percepts rather than graceful degradation. Score for
   percept stability explicitly, and include a **lowpass-heavy source**
   (most vulnerable — HF above 4 kHz anchors true position).
6. **Vary content, not just delta.** Artifact masking is strongly
   content-dependent (Hoffmann & Møller; Lindau & Roos; and Lindau's
   grid thresholds are 5° for noise vs 12.3° for guitar). Test noise,
   speech, and music — the sidecar's coding noise is exactly the class
   that noise-like material fails to mask.
7. **Separate gain error from latency.** Grimaldi et al. (2023) found
   these dissociate — latency was forgiven, angular mismatch was not.
   Run a condition with correct-but-late rotation against one with
   right-timed-but-partial rotation; the differential scheme is the
   second kind, and this is the comparison that prices it.
8. **A/B against deliberate under-rotation.** Freeman et al. (2017) put
   the perceptual null at gain +0.15 (~85% compensation). Worth testing
   whether a correction scaled to ~0.85 beats an exact one — free if
   the rig already sweeps gain.
9. **Score externalization/plausibility, not detection.** The whole
   detection-threshold literature measures the wrong dependent variable
   for "does lag break believability" (§The acceptable-latency budget).
   Rating externalization against a latency sweep would map the curve
   nobody has mapped, and gives the sidecar a meaningful yardstick —
   "how much latency does the correction buy back" in the units that
   actually matter. **Run this condition even if the sidecar fails**:
   it is the more generally useful result.

## Reopen triggers

- If the split's seam proves inaudible with delay-matched filters, the
  tiered/scalable story (FOA-only floor, layer-dropping under
  congestion) starts to matter and the split deserves reconsideration.
- If differential ghosting is audible even with matched filters, the
  scheme fails at its riskiest link; the sink-format menu question
  reverts to streaming bilateral-2 vs plain binaural as recorded in
  `head-rotation-and-sink-formats.md`.
- If the loop latency can be driven below ~45 ms end to end, the whole
  question softens: that is the critical-listener detection threshold
  (Rappin 44 ms, Lindau 52 ms) and Lindau's MAMA-derived budget at
  180°/s (45.6 ms). Correction machinery earns much less below it.
- Uplink-style regime shifts don't touch this — it is downlink-only.
- If a published perceptual evaluation of audio reprojection appears
  (none exists as of 2026-07-25), re-read it against §Failure modes —
  particularly 3GPP ISAR (TS 26.249) pose correction, whose approach is
  cue-domain (ITD/ILD) rather than SH-domain.

## References

Ordered by relevance to this document. `[FT]` full text read, `[AB]`
abstract only, `[SEC]` secondary source — treat `[SEC]` figures as
needing verification before they are quoted anywhere normative.

**Directly on the question — acuity during head rotation**

- Honda, A., Ohba, K., Iwaya, Y. & Suzuki, Y. (2016). "Detection of
  sound image movement during horizontal head rotation." *i-Perception*
  7(5). — 3.6° static → 17.7° rotating at 63.5°/s; 30°/s vs 60°/s
  n.s. (p=.28). `[FT]`
- Brimijoin, W.O. & Akeroyd, M.A. (2014). "The moving minimum audible
  angle is smaller during self motion than during source motion."
  *Front. Neurosci.* 8:273. — 5.4° self vs 6.6° source vs 3–4° static.
  `[FT]` *(reported ANOVA df/p are internally inconsistent — re-check
  against the PDF before quoting the statistics)*
- Chandler, D.W. & Grantham, D.W. (1992). *JASA* 91(3):1624. — MAMA
  8.8°→20.2° (10→180°/s); static MAA 3.4°. `[AB]`
- Carlile, S. & Leung, J. (2016). "The perception of auditory motion."
  *Trends in Hearing* 20:1–19. — review; "MAMA 2–3× larger than MAA
  under matched conditions." `[FT]`

**The saccadic-suppression analogue**

- Harris, L.R. & Lieberman, L. (1996). "Auditory stimulus detection is
  not suppressed during saccadic eye movements." *Perception*
  25(8):999–1004. `[AB]`
- (2023). "Detection of spatially localized sounds is robust to
  saccades and concurrent eye movement-related eardrum oscillations."
  *J. Neurosci.* 43(45):7668. — null replicated, Bayes factors favour
  the null. `[FT]`
- Klingenhoefer, S. & Bremmer, F. (2009). "Perisaccadic localization of
  auditory stimuli." *Exp. Brain Res.* 198:411–423. — auditory
  perisaccadic mislocalization exists but with a different
  spatiotemporal signature from vision. `[AB]`
- Leung, J., Alais, D. & Carlile, S. (2008). "Compression of auditory
  space during rapid head turns." *PNAS* 105(17). — up to ~50%
  perceptual compression. `[SEC]`

**Binaural sluggishness / integration**

- Akeroyd, M.A. & Summerfield, A.Q. (1999). "A binaural analog of gap
  detection." *JASA* 105(5):2807. — binaural window ERD ~140 ms. `[SEC]`
- Grantham, D.W. & Wightman, F.L. (1979). *JASA* 65(6):1509. —
  integration 44–243 ms; ±1.0 correlation swing at 4 Hz fully smeared.
  `[SEC]`
- Roggerone, V. et al. (2019). "Auditory motion perception emerges from
  successive sound localizations integrated over time." *Sci. Rep.*
  9:16437. — snapshot model, integration ≈300 ms. `[FT]`

**Latency thresholds**

- Lindau, A. (2009). "The perception of system latency in dynamic
  binaural synthesis." *NAG/DAGA 2009*, 1063–1066 (= ch. 9 of
  *Binaural Resynthesis of Acoustical Environments*, TU Berlin diss.,
  2014). — mean 107.6 ms, lowest 52 ms, mTSL 43 ms; **no relation to
  head velocity** (mean 190°/s, max 942°/s); MAMA budget 65.6/45.6/
  26.7 ms at 90/180/360°/s. `[FT]`
- Brungart, D.S., Kordik, A.J. & Simpson, B.D. (2006). "Effects of
  headtracker latency in virtual audio displays." *JAES* 54(1/2):32–44
  (and ICAD 2005, 37–42). — best listeners 60–70 ms; −25 ms with a
  low-latency reference; >73 ms degrades localization; recommends
  <60 ms, <30 ms demanding. `[FT]` for ICAD 2005, `[AB]` for JAES.
- Yairi, S., Iwaya, Y. & Suzuki, Y. (2006). *ICAD 2006*, 217–222. — DT
  94 ms (42–132), DL 70 ms (30–118). `[FT]` *(a widely-circulated
  "DT 45 / DL 59 ms" figure contradicts the primary — do not cite it)*
- Rappin, C. et al. (2023). "Latency detection threshold of
  head-tracking for different head rotation speeds in binaural
  rendering." *Forum Acusticum 2023*, 303–309. — 60/180/360°/s →
  206.7/200.1/195.0 ms medians (protocol-inflated); MM vs FM n.s.;
  most critical subjects 44 ms; recommends TSL <40 ms. `[FT]`
- Stitt, P., Hendrickx, E., Messonnier, J.-C. & Katz, B.F.G. (2016).
  "The influence of head tracking latency on binaural rendering in
  simple and complex sound scenes." *AES 140th*, paper 9591. —
  complex scene threshold **+10 ms**, head speeds +6°/s. `[AB]`
  *(absolute values not obtainable; any quoted absolute is unsourced)*
- Kentgens, M., Bárány, T., Reimes, J. & Steingrobe, C. (2025).
  "Measuring motion-to-sound latency with motorized HATS." *DAGA 2025*,
  1533–1536. — real 3GPP IVAS binaural renderer demo measured at
  **93.4 ms mean / 94.6 ms median** M2S; renderer updated direction
  every 20 ms; tracker error <2°. `[FT]`
- Sankaran, N., Hillis, J., Zannoli, M. & Mehra, R. (2016). "Perceptual
  thresholds of spatial audio update latency in virtual auditory and
  audiovisual environments." *JASA* 140(4_Suppl):3008 (Oculus
  Research). — tested three head velocities **and a zero-latency visual
  stimulus** (the cross-modal reference question). **No numbers
  obtainable** — meeting abstract, paywalled; flagged as a known
  existing but unretrieved data point.
- Android platform documentation, spatial audio implementation. —
  head-tracking latency **must not exceed 150 ms**; pose reported
  ~every 20 ms, 40 ms maximum gap before stale-input logic trips. The
  only hard institutional number found. `[FT]`
- **Negative finding:** no numeric motion-to-sound latency requirement
  found from **MPEG-I (ISO/IEC 23090-4:2025), ITU, or EBU**. MPEG-I
  states only "below the perceivable threshold". Commonly quoted
  "15–20 ms" figures are motion-to-**photon**, not motion-to-sound. A
  claimed "<20 ms" for 3GPP ISAR traces to a glossary site, not the
  spec text — **do not cite it.**

**Rotation gain, externalization, and what motion buys**

- Freeman, T.C.A., Culling, J.F., Akeroyd, M.A. & Brimijoin, W.O.
  (2017). "Auditory compensation for head rotation is incomplete."
  *J. Exp. Psychol. HPP* 43(2):371–380. — PSE at movement gain +0.15
  (~85% compensation), invariant across 20/60/100°/s and azimuth;
  auditory Filehne illusion. `[FT]`
- Brimijoin, W.O., Boyd, A.W. & Akeroyd, M.A. (2013). "The contribution
  of head movement to the externalization and internalization of
  sounds." *PLOS ONE* 8(12):e83068. — world-stabilized 65% vs
  head-locked 20%; head-locked-while-moving **38% vs 89%** static;
  degraded BRIR 10%→50% with world-stabilized motion. `[FT]`
- Grimaldi, V., Simon, L.S.R., Courtois, G. & Lissek, H. (2023).
  "Effects of head-tracking artefacts on externalization and
  localization in azimuth with binaural wearable devices." *JAES*
  71(10):650–663. — **400 ms latency did not degrade azimuth
  localization; estimation mismatch did.** `[AB]` *(magnitudes
  paywalled — direction only)*

**Inconsistent / partial dynamic cues**

- Brimijoin, W.O. & Akeroyd, M.A. (2012). "The role of head movements
  and signal spectrum in an auditory front/back illusion."
  *i-Perception* 3(3):179–182. — inverted dynamic cue relocates the
  source; **full-band signals give bistable, flickering percepts**;
  illusion strongest at 500 Hz, at chance ≥4 kHz. `[FT]`
- McLachlan, G., Majdak, P., Reijniers, J., Mihocic, M. & Peremans, H.
  (2023). "Dynamic spectral cues do not affect human sound localization
  during small head movements." *Front. Neurosci.* 17:1027827. —
  freezing the spectral cue produced no localization penalty; the
  benefit of yaw comes from dynamic ITD/ILD. Measured yaw
  36.4 ± 23.0°/s. `[FT]`

**Artifact audibility with head motion (the nearest available proxies)**

- Lindau, A., Maempel, H.-J. & Weinzierl, S. (2009). "On the spatial
  resolution of virtual acoustic environments for head movements in
  horizontal, vertical and lateral direction." *EAA Symposium on
  Auralization*, Espoo. — just-audible BRIR grid 4–18°; pooled 5°
  (noise) vs 12.3° (guitar); no effect of room acoustics. `[FT]`
- Hoffmann, P.F. & Møller, H. — *Characteristics of Head-Related
  Transfer Functions* (Aalborg diss.; AES preprints 6326/6537/6652;
  *JAES* 56(11):972). — MATS 5–9.4 µs, MASS 4.1–16° azimuth;
  audibility stems from the spectral **difference**, not switching
  artifacts; broadband/random content masks switching. `[FT]`
  *(caveat: source moved, listener static — not a head-motion study)*
- Lindau, A. & Roos, S. (2010). "Perceptual evaluation of discretization
  and interpolation for motion-tracked binaural (MTB) recordings."
  *26. Tonmeistertagung*, 680–701. — switching artifacts "always
  clearly audible" with noise, often inaudible with speech and music.
  `[FT]` *(qualitative — ratings and subject comments, not thresholds)*
- Ben-Hur, Z., Alon, D.L., Rafaely, B. & Mehra, R. (2019). "Loudness
  stability of binaural sound with spherical harmonic representation of
  sparse HRTFs." *EURASIP J. Audio Speech Music Process.* 2019:5. —
  rotation-stability MUSHRA medians N=2:14, N=4:27, N=10:68, ref 100.
  `[FT]` — see `head-rotation-and-sink-formats.md`.

**Prior art for reprojection (industrial only)**

- 3GPP TS 26.249 — IVAS split rendering (ISAR): server renders, UE
  applies pose correction for last-moment head movement.
- Meta/Facebook patent US11102602B1 (Brimijoin, Hassager, Amengual
  Garí, 2021), "Systems and methods for spatial update latency
  compensation for head-tracked audio" — corrects rendered audio with
  **ITD/ILD adjustments, not a sound-field rotation**. Continuation
  US11943602.

**Bonus finding — not this scheme, but useful (see knock-on below)**

- Lindau, A., Kosanke, L. & Weinzierl, S. (2012). "Perceptual
  evaluation of model- and signal-based predictors of the mixing time
  in binaural room impulse responses." *JAES* 60(11):887–898. —
  perceptual mixing time t_mp50 = 20·(V/S) + 12 ms, t_mp95 =
  0.0117·V + 50.1 ms; RT is **not** a useful predictor. `[FT]`
- Lindau, A. (2014), diss. §5.3.2.2 (reporting AES 122nd preprint
  7032). — in a 10,000 m³ hall, a late tail frozen with a **75°
  head-orientation error was undetectable past ~35 ms**; some subjects
  detected differences out to 140 ms; tail from a *different source
  position/distance* did matter (50 ms). `[FT]`

**Knock-on from the bonus finding (not yet acted on):** the late
diffuse tail does not need to rotate. Past the perceptual mixing time
(~30–60 ms small rooms, up to ~150 ms large halls) a tail frozen at one
head orientation is indistinguishable from full tracking, even with a
75° orientation error — bounded by the tail matching the source's
position and distance, and retaining 12–24 directions of spatial
resolution. This is a server-side render saving independent of the
sidecar, and it means reverb never needs to be in the corrected path.
Belongs in whatever doc eventually covers server render architecture.
