# Audio track packet structure — decisions

*Decided 2026-07-24, in the session that produced `track-naming.md`. Covers
the four audio formats — `mono-object`, `binaural`, `ambi2`, `ambi3`.
The `presence` and `state` wire formats are deferred (see end).*

The cost denominator used throughout: at 200 packets/sec, **one payload
byte = 1.6 kbps per track**.

## 1. Sequence ids are MoQ ids: group-per-frame

There is **no seq field in the payload**. LASA committed to MoQ to hand
off identity work to the protocol layer (`lasa2.md`), and every MoQ
datagram already carries (track-alias, group-id, object-id). The mapping:

- **One group per frame**: group-id increments by exactly one per packet,
  object-id is always 0. The group-id *is* the frame sequence number.
- **No gaps**: pose-only packets increment too. This is load-bearing —
  the redundancy offset (below) is denominated in these ids, so
  `offset = 3` must mean exactly 15ms.

Group-per-frame is the mapping under which MoQ's own semantics stay
meaningful: every audio frame is independently decodable, so
join-at-latest lands on the current frame and cache/retention granularity
is per-frame.

*Rejected: a payload seq field* — 4–8 bytes (6.4–12.8 kbps) duplicating
transport identity. Its one real benefit, payload self-containedness for
recordings and tooling, is had by recording the MoQ ids alongside
payloads. *Rejected: one never-ending group* (byte-identical, but
"latest group" degenerates and group-aware delivery order means nothing)
and *group-per-second* (join/fetch granularity nothing in live audio
uses, for a two-part counter).

## 2. The version field is the track name

Formats are track names (`track-naming.md`), so **a changed layout is a
new track name** — `mono-object2`, not a version byte on every packet.
This licenses fixed layouts throughout. *Rejected: TLV/extensible
encodings* — ~2 bytes per field per packet buying flexibility the naming
scheme already provides additively and for free.

## 3. The flags byte

Every audio packet begins with one flags byte. Bit positions are shared
across formats; not every bit is meaningful in every format:

| Bit | Meaning | Applies to |
|---|---|---|
| 0 | pose present | `mono-object` |
| 1 | audio present | `mono-object` |
| 2 | redundancy present | all |
| 3–5 | redundancy offset (1–7) | all |
| 6 | ts present | sink formats |
| 7 | reserved, must be zero | — |

- **pose/audio independently optional** is what makes the source/pose
  merge work (`track-naming.md`): pose-only (silence), audio-only (pose
  decimation), both (normal) are one packet format. Pose decimation —
  sending pose every Nth packet — is a sender choice with zero format
  cost.
- Reserved bits are MBZ; a receiver MUST discard packets with unknown
  bits set (they are not this format).

## 4. Pose encoding: f32 position + int16 angles — 18 bytes

`x, y, z` as IEEE-754 f32; `yaw, pitch, roll` as int16 scaled to
±π/32767 (≈ 0.0001 rad steps — far below perception). Byte-for-byte the
same encoding wherever a pose appears on the wire (`mono-object`,
`presence`, and any future carrier).

**The pose is the freshest pose at packet assembly time** — sampled
after the audio is encoded, deliberately **not** synchronized with the
audio samples in the same packet. This is a stated property, not an
accident: it minimises pose staleness for the rotation path (see §8).
The price falls on the pose's other consumer — positioning the *source*
in the mix — where audio is rendered at a position ~one packetisation
interval fresher than its capture. Imperceptible at human movement
speeds; a fast synthetic entity (a fly-by effect) is where an
implementer might notice.

*Rejected:*

- **6 × f32 (24B)** — 9.6 kbps of angle precision nothing can hear.
- **int16 fixed-point position (12B)** — a protocol-level space bound
  (±327 m at 1 cm steps), or a per-space scale factor that would have to
  live in state — and the media plane working without state is a pinned
  decision (`track-naming.md`). Not worth 6 bytes.
- **f16 position (12B)** — error grows with distance from *origin* while
  audibility depends on distance from *listener*; two far-out entities
  near each other wobble relative to one another.

## 5. Timestamps

**Uplink (`mono-object`): none, deliberately.** The server re-times
everything at the mix; client clocks are untrusted. The stronger reason
is communicative: a source timestamp field would give the wrong
impression — that source timestamps participate in presentation timing —
when they never will. The field's absence is documentation. Recorded
consequences: client-side end-to-end latency measurement is impossible
(if ever wanted, a client-ts flag bit is an additive change), and the
server cannot know a pose's age directly — pose-velocity estimation for
prediction (§8) must work from successive poses and arrival times,
which network jitter contaminates. Filtering territory; the one place
the absent timestamp costs something.

**Sink formats: occasional, not per-packet.** Frame spacing is exactly
5ms on the server's own clock, so a single (seq, ts) anchor extends to
every frame: `ts(seq) = anchor_ts + (seq − anchor_seq) × 5ms`. Packets
with the ts-present bit carry a full timestamp; the rest carry none.
These timestamps are informational (recording alignment, diagnostics,
future multi-device sync) — nothing currently correlates audio across
tracks by them, and presentation timing does not depend on them.

*Rejected: per-packet u64* (8 bytes = 12.8 kbps of mostly-redundant
clock) *and per-packet u32* (wrap handling for a field that needn't be
per-packet at all).

**Pinned 2026-07-27:** width/epoch is **u64 unix-epoch microseconds**;
the instant named is **the first sample of the accompanying frame**;
cadence is **on stream start and periodically thereafter, on the order
of once per second**. The (seq, ts) anchor is per track instance — a
reconnect starts a new instance (§9), so the new stream re-anchors on
its first packets.

## 6. Redundancy framing — one rule for all four formats

When the redundancy bit is set, the packet carries two audio payloads:
the current frame and a **full copy of the frame `offset` packets ago**
(offset 1–7 from the flag bits, i.e. burst cover up to 35ms — the offset
is expressible per `opus-frame-size.md`, not hardcoded to
previous-packet). Neither an elementary Opus packet nor a multistream
packet is self-delimiting, so **a length prefix on the primary payload**
separates the two. When the redundancy bit is clear there is no length
prefix — the payload runs to the end of the datagram.

**The referenced packet must be audio-bearing.** The duplicate is "the
audio of packet `seq − offset`", so the sender must choose an offset
pointing at a packet that carried audio. The edge is real at silence
boundaries: the first packet of a talk burst cannot duplicate into the
pose-only silence behind it — the sender omits redundancy or picks a
smaller offset until enough audio-bearing history exists.

The format carries the **capability** only. Whether redundancy is on is
the sender's per-packet choice (the bit is self-describing); any
adaptation policy — e.g. a server hinting "loss is high, enable dup" —
is profile vocabulary over state, not format surface. The economic
asymmetry (duplicating mono is cheap; duplicating 16-channel ambi
doubles a large payload) is likewise policy, needing no format
difference.

The offset is not only a loss-robustness knob — it converts loss into
latency at `offset × 5ms`, so it must be tuned jointly with jitter
buffer depth. See §8 ("Coupling to the redundancy offset").

## 7. Ambisonic formats are Opus multistream packets

The audio section of `ambi2`/`ambi3` is **exactly one Opus multistream
packet** (RFC 7845 framing: per-stream packets concatenated, all but the
last self-delimited per RFC 6716 App. B — produced and consumed whole by
the standard multistream API, never hand-assembled).

Multistream requires the channel configuration — (streams,
coupled-streams, mapping) — to be negotiated out of band. **Format-in-name
is that negotiation**: `ambi2` and `ambi3` each *mean* a pinned
configuration, written in the spec. Channel counts and conventions:
`ambi2` = 9 channels, `ambi3` = 16 channels, ACN order, SN3D
normalisation. (This fixes `lasa2.md`'s "16-channel 2nd/3rd order"
slip in passing.)

**The arrangement — pinned provisionally 2026-07-27 (the empirical
evaluation remains the reopen condition):**

- **All-uncoupled** (`streams = 9/16, coupled = 0`, identity mapping) —
  the smallest possible spec; forfeits inter-channel redundancy.
  Multistream coupling is real joint coding only *within* a coupled
  stereo pair, and raw ambisonic channels are not stereo-like.
- **Projection (RFC 8486 family 3)** — premixes channels through a
  matrix into couplable pairs; meaningfully better quality-per-bit. The
  demixing matrix is decoder-required state that Ogg ships in a header;
  headerless datagrams mean pinning the exact matrix in the spec as a
  normative constant per track name.

Both are native libopus paths (`opus_multistream_*`,
`opus_projection_*`). Rules of thumb cannot settle this: coupling gains
on ambisonic material at target bitrates are empirical, and a
listening/bitrate evaluation is the discriminator.

**Pinned 2026-07-27: all-uncoupled, provisionally.** `ambi2` = 9 mono
streams, `ambi3` = 16 mono streams, `coupled = 0`, identity mapping,
ACN order, SN3D normalisation. Chosen because it is the smallest
possible spec and moving later is a new track name — additive, breaks
nothing. **Reopen condition:** the coupled/projection listening-bitrate
evaluation, whenever a listening rig runs. (With the head-rotation
question deferred and no bilateral formats in v1, the ambi formats
remain an ear path, so the evaluation keeps its urgency — noted against
the earlier remark that it would fade if bilateral landed.)

## 8. The pose fast path (non-normative)

Wire-invisible server/client behaviour that every implementer should
hear about before discovering their jitter buffer is delaying rotation.

**Pose bypasses the audio jitter buffer.** The buffer serves a
continuity requirement pose does not have: audio is a stream where late
packets are worth waiting for; pose is a latest-wins sampled signal
where a late sample is worthless. Two payloads in one datagram take
different paths: audio → jitter buffer → mixer tick; pose → applied on
arrival, guarded only by the group-id latest-wins rule (which already
handles reordering; a lost packet's pose needs no recovery — the next
supersedes it).

Combined with §4's assembly-time sampling, this removes ~15–25ms from a
40–80ms rotation loop with no wire change: assembly-time sampling
removes frame-accumulation staleness client-side; the bypass removes
jitter-buffer depth server-side.

**The three-stage rotation-latency picture.** The loop decomposes into
segments with one lever each; the levers compose and none replaces
another:

1. **Uplink staleness** — the fast path above.
2. **Server-to-playout staleness** (encode + downlink + client jitter
   buffer + playout) — server-side pose *prediction* (extrapolating by
   estimated remaining latency; see §5 for the velocity-estimation
   caveat) partially covers it for all formats.
3. **Residual** — client-side reprojection covers it, for the sink
   formats that support it (`head-rotation-and-sink-formats.md`).

Note where the fast path matters most: plain `binaural` cannot be
reprojected, so stages 1–2 are its *only* levers. And for bilateral
formats, a smaller residual shrinks the one uncorrectable reprojection
error (ear translation, which scales with residual size).

### The fast path's ceiling, and why to profile before choosing levers

*Added 2026-07-25.* The fast path is the **cheapest** rotation-latency
lever — free of bandwidth, format surface and negotiation, and the only
one that benefits every sink format including `binaural`. It is also
**prior**: it changes the answer to the sink-format question, since
below ~45ms the correction machinery earns much less
(`rotation-correction-sidecar.md` §Reopen triggers). That makes "fast
path first" a sound sequencing claim. Three caveats keep it from being
the whole answer.

**It has a hard ceiling, and it sits on the uplink half of the loop.**
Decomposed: pose sample → uplink → server render → downlink → client
jitter buffer → playout. The fast path attacks assembly-time staleness
and the **server's** jitter buffer. The **client's downlink jitter
buffer is untouched by it**, and on a lossy WAN that buffer can be the
dominant term. The fast path cannot take the loop below the
downlink + client-buffer + playout floor — which is precisely the
segment stages 2 and 3 exist to cover. **"Fast path instead of
reprojection" is a false choice; "fast path first" is not.**

**The ~15–25ms figure is an estimate, not a measurement.** It was
reasoned out before any LASA loop existed to profile. The ordering
could invert: if the client buffer is 40ms and the server buffer is
8ms, the fast path is a rounding error and all the leverage is
downstream.

**It dissolves only half the sink-format dilemma.** That question has
two axes — rotation latency *and* render quality. Even at zero latency,
plain binaural from a head-centered expansion still fails at whisper
range, and near-field is a recorded requirement, not a nicety
(`head-rotation-and-sink-formats.md`). A perfect fast path retires the
rotation axis and leaves bilateral-vs-head-centered exactly where it
was.

**Regime dependence** (the deployment target is not pinned in any doc,
and it decides this):

| Regime | Where the leverage is |
|---|---|
| LAN / low RTT | Fast path alone plausibly clears ~45ms; correction machinery hard to justify against its bandwidth and format contract |
| Moderate WAN | Both levers wanted; they compose |
| High jitter / lossy | Client buffer dominates; the fast path's uplink saving is a small fraction and reprojection carries the load |

**The discriminating measurement — cheap, and needs no listening rig:**
**instrument a loop and get the per-segment budget.** Kentgens et al.
(2025) give a directly reusable black-box method for total
motion-to-sound — drive a motorised HATS, watch the ILD zero-crossing
(they measured a shipping 3GPP IVAS renderer at 93.4ms this way; cited
in `rotation-correction-sidecar.md` §References). The per-segment split
needs internal timestamping, which is wanted anyway. One measurement
tells you whether the fast path is 25ms of an 80ms loop or 8ms of one,
and settles the sequencing by arithmetic rather than argument. **It is
also the only piece of this work that no listening result can
invalidate.**

**Coupling to the redundancy offset (§6).** The offset sits on the same
axis. Redundancy converts *loss* into *latency* at an exchange rate the
sender picks per packet: recovering a lost frame costs `offset × 5ms`
of delay. This does **not** reduce jitter-driven buffer depth — depth
is set by how late packets arrive, not by loss — so the offset is not a
latency lever on its own. But it means **offset choice and buffer depth
must be tuned as one thing**, not independently: an offset larger than
the buffer depth is unusable, and a buffer sized without reference to
the offset in use will either waste depth or strand the redundancy.
§6 currently frames the offset purely as a loss-robustness knob; that
is incomplete.

## 9. Group-ids are session-scoped: a reconnect restarts at zero

*Pinned 2026-07-27.* Entity and client ids come from the Connection
Config, so a reconnecting client republishes the **same track tuple**.
The id space does not carry over: a reconnect is a **new track
instance**, and group-ids restart at 0.

Continuation across reconnect would be wrong, not merely harder. The
no-gaps rule (§1) makes id arithmetic *time* arithmetic — `offset = 3`
means exactly 15ms — and that equivalence only holds within a
contiguous run of frames. A disconnect is a hole of wall-clock time;
stitching the id sequence across it would put seconds between adjacent
ids, silently breaking the redundancy offset and any id-denominated
freshness logic. It would also require the client to persist its last
id, for no benefit any receiver can use.

Receiver statement: all per-track receiver state — the pose
latest-wins id, jitter buffer, redundancy history, the ts anchor
(§5) — is scoped to the **(MoQ session, track) instance** and resets
with it. The server learns of the reconnect from the new MoQ session
itself; nothing in the packet format signals it. The same rule applies
symmetrically to the space's sink tracks: the server's published
instance is per-session too. Remaining connection-lifecycle detail
(auth, what survives a resumption) lands in `connection-and-auth.md`.

## Resulting layouts

```
mono-object:  [flags][pose(18)?][len(2)?][opus][opus-dup?]
binaural:     [flags][ts?]      [len(2)?][opus][opus-dup?]
ambi2/ambi3:  [flags][ts?]      [len(2)?][opus-ms][opus-ms-dup?]
```

Everything after the flags byte is present iff its flag bit (or the
redundancy bit, for `len`) says so. Frame identity rides the MoQ
group-id; there is no other header.

## Open items

*Updated 2026-07-27: the ts details (§5), the ambi arrangement (§7,
provisional) and id continuity (§9) are pinned; the sink-format menu is
closed for v1 by deferral of the head-rotation question.*

- Sink-track behaviour already parked in the naming phase: render
  lifecycle on subscribe/unsubscribe, flavour-switch overlap, concurrent
  flavours policy, sink-subscribe authorization.

**Closed 2026-07-27 — the sink format menu.** The head-rotation
question is **deferred as future work** (see the deferral note in
`head-rotation-and-sink-formats.md`). v1 ships the initially planned
menu: **`binaural`, `ambi2`, `ambi3`, all three mandatory to
implement**. *(Superseded 2026-08-21: the offering is now
`binaural`+`ambi2` MUST, `ambi3` SHOULD at internal render order ≥ 3
— core §3.)* Bilateral formats and the correction sidecar remain
recorded candidates for a future rotation-latency phase; adding them
later is additive under format-in-name.

## Deferred (own phases)

- **`presence` packet format.** ~~Sketched here~~ **Pinned 2026-07-27
  in `presence-format.md`** — delta/keyframe packets, gen-guarded
  implicit indexes, chunked rosters, §4 pose, log-u8 loudness.
- **`state` wire encoding.** ~~Open~~ **Pinned 2026-07-27 in
  `state-wire-encoding.md`** — subscribe as MoQ SUBSCRIBE KVP, typed
  messages one-per-object, live/catch-up as subgroups, 64 KiB value
  cap.

## Knock-ons

- `lasa2.md`'s track tables and payload descriptions remain **stale**
  (also noted in `track-naming.md`): to be rewritten once against both
  decision docs — merged uplink track, tuple namespaces, MoQ-id
  sequencing (drop `seq` from payload columns), occasional-ts (drop
  per-packet `ts`), `fec?` becomes the redundancy scheme above, ambi
  channel counts corrected.
