# LASA — Live Atomic Spatial Audio

A binary wire format for **live, lossy-tolerant, source-authored object-based and ambisonic audio streaming** over UDP and MOQ.

LASA is a work in progress by the [Panaudia](https://github.com/panaudia) team. 
It represents our ongoing exploration of a pragmatic wire format for live spatial audio and is the format 
we're using to build the next version of Panaudia.

> **⚠ Status: work in progress.** LASA v1 is a single-author draft. No reference implementation has been published yet, no implementations have been field-tested, and the spec is expected to change as it gets exercised. Do not build production systems against it. Breaking changes are still fair game before v1 is declared stable.

---

## What is LASA?

Each LASA packet carries one source's audio frame plus the intrinsic state of that source at that moment — position, orientation, gain, directivity, diffusion, distance rolloff. Every packet is **atomic and self-describing**: a receiver can parse any single packet in isolation and have the full current truth for one source.

That atomicity is the central design property and the source of the spec's name. It means:

- **Packet loss has no cumulative effect.** The next packet from the same source restores full state.
- **There's no session state to recover** after a disconnect, a late join, or a relay restart. Every packet is a complete unit.
- **Receivers stay simple.** No event playback, no delta reconstruction, no anchor-and-replay.
- **Many independent sources can publish in parallel** without coordinating their clocks or sequencing through a central authority.

The full spec lives in [`lasa.md`](lasa.md).

## Why LASA exists

The spatial audio standards landscape today is rich for **file-based mastering** and for **delivery of finished mixes** to consumers, but there is no open, lean, lossy-tolerant format for the in-between case: the **live, distributed, many-source-to-mixer hop** that cloud spatial-audio systems need.

When building a cloud ambisonic mixer (panaudia, in active development) the gap became unavoidable:

- Existing standards assume **single-author timelines** (one creator authors the spatial scene over time, then ships it). LASA targets **many sources, each authoring its own piece in parallel, fanning into a mixer**.
- Existing standards assume **reliable, ordered transport** (file container, broadcast contribution link, fragmented MP4). LASA targets **UDP and MOQ datagrams**, where loss and reorder are normal.
- Existing standards encode **time implicitly** by accumulating frame durations from a stream start. LASA carries an **absolute sender-side sample timestamp on every packet**, so a packet is parseable atomically.

LASA is not a replacement for any of the standards below. It fills a niche they were not designed for.

## How LASA fits in the landscape

| Format | Role | Authoring model | Transport assumed | Open? |
|---|---|---|---|---|
| **BW64 + ADM** (ITU-R BS.2076) | Mastering file | Single creator, offline | File | Yes |
| **S-ADM** (ITU-R BS.2125) | Broadcast contribution | Single creator, live | Reliable, ordered (SDI, ST 2110, AES67) | Yes |
| **IAMF / Eclipsa** (AOMedia) | Consumer delivery | Renderer-authored from Mix Presentation | Container (fMP4) | Yes |
| **Dolby Atmos** | Mastering + delivery | Single creator, offline | Licensed pipelines | No |
| **MPEG-H 3D Audio** | Broadcast bitstream | Single producer, live | Broadcast multiplex | Patent-encumbered |
| **LASA** | **Live source-to-mixer hop** | **Many sources, each authoring its own piece, in parallel** | **Lossy (UDP / MOQ datagrams)** | **Yes** |

A few quick notes on the alignment:

- **ADM provides the vocabulary.** LASA borrows ADM's concepts directly — `position`, `attitude`, `gain`, `diffusion`, `directivity` — flattened from ADM's nested authoring hierarchy (which is useful at file time but is overhead at frame time) into a per-source struct on the wire.
- **S-ADM is the closest existing answer to "ADM but live"**, but it's event-stream-shaped (here is what changed at time T) which assumes ordered, reliable delivery. LASA is state-replication-shaped (here is what is true right now), which is loss-tolerant.
- **IAMF/Eclipsa is complementary, not competing.** A cloud mixer using LASA internally would naturally emit IAMF/Eclipsa **at egress** when delivering finished mixes to playback clients. LASA is the "ingress and internal hop" format; IAMF is the "egress to consumer" format. The mixer is the IAMF author at that boundary.

## Non-goals

LASA explicitly does **not**:

- Replace IAMF or Atmos for consumer delivery — different problem.
- Define a transport. LASA bytes ride inside UDP datagrams, MOQ objects, or any other datagram-shaped envelope. Transport-layer concerns (congestion, encryption, NAT) are out of scope.
- Carry slow / persistent / structured state. Names, capabilities, attributes, room geometry, permissions — these belong on a separate reliable plane.
- Define session setup or capability negotiation. No SDP-equivalent.
- Define mixing or rendering algorithms. LASA describes only the wire bytes; how a mixer interprets diffusion, directivity, attenuation, etc., is its own concern.
- Provide ordering or reliability guarantees. Receivers must tolerate loss, reorder, and duplication by design.

## Documents

- [`lasa.md`](lasa.md) — the spec itself.
- [`plan/todo.md`](plan/todo.md) — what's done, what's needed for a real v1, and open design questions.

## How to engage

This is early. The most useful contributions right now are:

1. Reading the spec critically and reporting under-specified or contradictory corners.
2. Sketching how LASA would or wouldn't fit a real use case you have — gaps in the design are easier to see from outside.
3. Once a reference implementation exists, porting it to another language and reporting where the spec wasn't sufficient.

Nothing about LASA is set in stone before v1 is declared stable.

