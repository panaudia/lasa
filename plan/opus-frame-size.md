# Opus frame size — reference

Reference notes for the LASA decision to fix the codec at Opus @ 48kHz in 5ms
frames. Collects the trade-offs between frame sizes so the choice is priced,
not assumed. Numbers are rules of thumb, not measurements — they vary with
material and bitrate.

## The options

Opus supports frame sizes of 2.5, 5, 10, 20, 40 and 60ms. The choice is not
just a latency knob — it selects which of Opus's two internal codecs are
available:

| Frame size | Modes available        | In-band FEC (LBRR) | Notes                          |
|------------|------------------------|--------------------|--------------------------------|
| 2.5ms      | CELT only              | no                 |                                |
| 5ms        | CELT only              | no                 | ← LASA's current choice        |
| 10ms       | SILK, hybrid, CELT     | yes                | smallest frame with full Opus  |
| 20ms       | SILK, hybrid, CELT     | yes                | the default / sweet spot       |
| 40/60ms    | SILK, hybrid, CELT     | yes                | streaming, not live            |

- **SILK** — linear-prediction layer, speech-optimised, best quality-per-bit
  for speech at low/mid bitrates (~16–40kbps mono). Requires ≥10ms frames.
- **CELT** — MDCT transform layer, music/full-band, low lookahead. Works at
  all frame sizes; the only option below 10ms.
- **Hybrid** — SILK low band + CELT high band. Requires ≥10ms frames.

So at 5ms, LASA is a CELT-only protocol. Every consequence below follows from
that plus the packet rate.

## Cost 1 — coding efficiency (quality per bit)

Three effects, all against short frames:

1. **No SILK.** For speech at low/mid bitrates SILK beats CELT meaningfully.
   Locking to 5ms forfeits it entirely.
2. **CELT itself degrades at short frames.** Half the MDCT frequency
   resolution, window overlap is a larger fraction of the frame, less entropy
   coder context, per-frame signalling amortised over half as many samples.
3. **Rule of thumb:** CELT at 5ms needs roughly **20–40% more bitrate** than
   Opus at 20ms for equivalent speech quality. The gap narrows a lot for
   music/full-band material at generous bitrates (≥64kbps) — CELT is a very
   good music codec and short frames hurt it less there.

## Cost 2 — packetisation overhead (usually the dominant cost)

Packet rate: 5ms → 200 pkt/s per track; 10ms → 100; 20ms → 50.

Every datagram carries IP + UDP + QUIC + MoQ object headers — roughly
**60–90 bytes** per packet. Per track that is ~100–145kbps of pure overhead at
5ms, half at 10ms, a quarter at 20ms.

Worked example: a 32kbps mono speech stream at 5ms carries **~20 bytes of Opus
payload** under ~70+ bytes of headers — overhead exceeds payload ~3:1. At 20ms
the same stream is 80-byte payloads and the ratio roughly inverts.

Mitigations already latent in the LASA design:

- **The 16-channel ambisonic track amortises well** — one packet carries all
  16 channels, so the header tax is shared across a much larger payload. The
  overhead cost falls mostly on the *mono source* tracks (smallest payloads).
- **Bundling pose into `source` amortises too** — the headers are paid anyway,
  so the ~28 bytes of pose ride nearly free. Short frames make the bundling
  *more* valuable (fresher pose per audio frame), not less.

## Cost 3 — no in-band FEC below 10ms

Opus's in-band FEC (**LBRR**) is a SILK feature: it embeds a *low-bitrate*
re-encoding of frame N−1 inside packet N, at ~20–30% bitrate overhead.
**Unavailable below 10ms frames.**

Consequence: at 5ms the only redundancy option is application-level
duplication of a previous packet's audio, which costs **100% of the audio
bitrate** instead of ~25%. The `fec?` duplication in the LASA track payloads
is therefore not a free-standing choice — it is *forced* by the 5ms decision.

### Latency of redundancy — LBRR vs DIY duplication are equivalent

Neither scheme adds encoder delay (the redundant copy is of an
already-encoded past frame). Both impose the same receiver-side requirement:
the redundant copy of a lost frame arrives in a *later* packet, so the playout
point must sit at least one packet interval deep for recovery to be possible.

This is a **floor on the jitter buffer, not an addition to it** — in both
schemes. libopus has no jitter buffer of its own and does nothing
autonomously; with LBRR it is still the application's jitter buffer holding
packet N and the application calling `opus_decode(..., decode_fec=1)` at frame
N−1's deadline. A jitter buffer already running a frame or two deep for
network jitter absorbs the recovery depth in either scheme.

### Where the schemes actually differ

| | LBRR (in-band) | DIY duplication |
|---|---|---|
| Bitrate overhead | ~20–30% | 100% (full copy) |
| Recovered frame quality | slightly degraded | perfect |
| Offset | fixed: previous packet only | **free choice** |
| Frame sizes | ≥10ms only | any |

The offset freedom is the significant one. A duplicate of frame N−1 only
covers isolated single-packet loss — at 200 pkt/s a 10ms network burst kills
two consecutive packets *and* their redundancy. Offsetting the duplicate
(carry frame N−2 or N−3 instead) covers bursts, and if the offset is aligned
with jitter-buffer depth already being paid for, burst coverage is free of
additional latency.

**Spec implication:** make the duplication *offset* a parameter (or at least
negotiable), not hard-coded to "previous packet" — the previous-packet
configuration is the one arrangement that only covers isolated loss.

(Narrowing the 100% overhead by duplicating a lower-bitrate re-encode is
possible but means a second encode per frame — rebuilding LBRR by hand.)

## What 5ms buys

The LASA pipeline traverses the codec **twice** (client encode → server decode
→ mix/render → encode → client decode). Frame buffering is paid per
traversal, plus lookahead (CELT-only ~2.5ms vs SILK/hybrid ~6.5ms):

- **5ms frames:** ~15–20ms codec-chain latency end-to-end
- **10ms frames:** roughly +10ms over that (and +5ms recovery depth vs 5ms)
- **20ms frames:** ~55–60ms

Frame size also sets the mixer tick, jitter-buffer granularity, and how fast
head-tracked binaural responds to listener motion. Head-motion-to-sound
latency has tight perceptual budgets (same spirit as VR motion-to-photon), and
this is the strongest argument for 5ms in a head-tracked spatial protocol.

## The trade, summarised

**5ms costs:** ~2× packet rate vs 10ms (4× vs 20ms), ~100% redundancy
overhead instead of ~25% (no LBRR), and roughly 20–40% worse speech coding
efficiency (no SILK, weaker CELT).

**5ms buys:** ~40ms lower end-to-end latency vs 20ms frames, and tighter
head-tracking response.

**The alternative to take seriously before committing: 10ms.** It is the
threshold at which SILK, hybrid mode and in-band FEC all become available, and
it halves the packet rate — bought for ~10ms of latency. Note the redundancy
recovery depth partially offsets the latency argument for 5ms: the honest
comparison is frame buffering + lookahead + recovery depth summed, and on that
sum the 5ms-vs-10ms gap is a little narrower than frame size alone suggests.

For a protocol whose reason to exist is *live* head-tracked spatial audio,
5ms is a defensible purchase — but the spec is stronger for showing the bill
was priced.
