# LASA — Live Atomic Spatial Audio

**Status:** draft v1
**Scope:** per-packet binary wire format for live, lossy-tolerant spatial audio streaming.
**Licence:** This specification text is licensed under [Creative Commons Attribution 4.0 International (CC-BY 4.0)](https://creativecommons.org/licenses/by/4.0/). The repository as a whole is MIT-licensed; see [`LICENSE`](LICENSE).

---

## 1. Purpose

LASA carries per-source audio frames and their intrinsic, frame-rate-varying source state (pose, gain, directivity, etc.) from sources to mixers, and from mixers to renderers, over UDP and MOQ datagrams.

Each packet is **self-describing and stateless from the receiver's perspective**: any single packet, parsed in isolation, conveys the full current truth for one source at one moment in time. Losing a packet has no cumulative effect — the next packet from the same source restores full state. This atomicity is the central design property and the source of the spec's name.

LASA is the fast plane of a two-plane state distribution architecture:

- **Fast plane (this document).** Lossy, atomic, state-replicated. ~200 Hz per source.
- **Slow plane.** Reliable, monotonically-ordered, JSON-encoded, backfillable. Names, capabilities, persistent attributes. Specified separately.

A small shared conventions document (`docs/conventions.md`, planned) defines UUIDs, coordinate frame, and units used by both planes. Those conventions are reproduced inline in §2 for self-containment.

## 2. Conventions

### 2.1 Endianness

All multi-byte integer fields are **little-endian**. IEEE 754 `f32` values are stored in little-endian byte order.

UUIDs are an exception: they are stored as 16 raw bytes in **RFC 4122 byte order** (matching what Rust's `uuid::Uuid::as_bytes()` returns). UUIDs are not subject to integer endianness rules — they are a 16-byte opaque identifier on the wire.

### 2.2 FourCC

4 ASCII bytes laid out left-to-right at increasing offsets. Short codes are space-padded. Example: `b"f32 "` is the byte sequence `66 33 32 20`.

### 2.3 Coordinate frame

Right-handed:

- `+X` = forward
- `+Y` = left
- `+Z` = up

This is the AES / academic ambisonic convention (matches AmbiX, SN3D/N3D, ACN ordering).

### 2.4 Rotation

**All angles on the wire are in radians.** This is a deliberate choice: every LASA consumer computes trigonometry on these values (rotation matrices, spherical harmonics, pose interpolation), and radians is the unit native to every standard math library. Authoring tools and UIs that surface rotations to humans typically use degrees; that conversion lives at the tool / data boundary, **not on the wire**. Implementations MUST NOT transmit degrees in any LASA field.

Rotation order is **intrinsic Tait-Bryan, applied yaw → pitch → roll**:

- `yaw` around local `+Z` (up): positive = turning left (counterclockwise viewed from above)
- `pitch` around local `+Y` (left): positive = nose up
- `roll` around local `+X` (forward): positive = banking left (left wing up)

Each rotation is applied in the local frame produced by the previous rotation. Angle values are **unbounded** on the wire — receivers wrap to a canonical range (typically `[-π, π]` or `[0, 2π]`) if they need to. Senders SHOULD NOT pre-wrap, so that interpolators and unwrap-tracking receivers can preserve continuous trajectories across the ±π discontinuity.

### 2.5 Units

| Quantity | Unit |
|---|---|
| `position.{x,y,z}` | metres |
| `attitude.{yaw,pitch,roll}` | radians (intrinsic, see §2.4) |
| `gain` | linear amplitude (not dB) |
| `attenuation` | rolloff exponent, dimensionless (1 = linear, 2 = inverse-square, 3 = inverse-cube) |
| `diffusion` | dimensionless, range `[0, 1]` |
| `directivity` | dimensionless, range `[0, 1]` |
| `sample_ts` | samples, sender-local monotonic clock at 48 000 Hz |
| `seq` | dimensionless counter |

### 2.6 Sample clock

`sample_ts` is the sender's local monotonic sample counter at 48 kHz. Receivers MUST NOT assume cross-source synchronization at the wire level — the mixer is responsible for reconciling drift between sources.

## 3. Packet layout

Fixed header is **64 bytes**. NodeState (if present) is exactly **40 bytes**. TLV extensions and payload follow.

```
Off  Size  Field           Notes
─────────────────────────────────────────────────────────────────── header (4 B)
 0   1     magic           0xA5
 1   1     version         1
 2   1     flags           bit 0 = has_state
                           bit 1 = has_ext_tlv
                           bits 2–7 reserved (MUST be 0)
 3   1     packet_type     see §7
─────────────────────────────────────────────────────────────────── format (6 B)
 4   4     codec_fourcc    FourCC; see §8
 8   1     format          format enum; see §9
 9   1     channels        explicit channel count; sole source of truth
─────────────────────────────────────────────────────────────────── timing (14 B)
10   1     fragment_idx    0-based fragment index
11   1     fragment_count  total fragments for this logical packet
                           (1 if not fragmented); see §6
12   4     seq             u32 LE, per-(space_id, node_id), wraps
16   8     sample_ts       u64 LE, sender sample clock @ 48 kHz
─────────────────────────────────────────────────────────────────── identity (32 B)
24  16     space_id        UUID, RFC 4122 byte order
40  16     node_id         UUID, RFC 4122 byte order
─────────────────────────────────────────────────────────────────── layout (8 B)
56   4     payload_len     u32 LE; length of the payload section
60   2     header_len      u16 LE; total header bytes (incl. NodeState and TLVs)
62   2     reserved        MUST be 0; align to 8
─────────────────────────────────────────────────────────────────── optional
64  40     NodeState       present iff flags.has_state; see §4
[var]      ext TLVs        present iff flags.has_ext_tlv; see §5
[var]      payload         length = payload_len; interpretation per
                           codec_fourcc and format
```

**Total packet length** = `header_len + payload_len`.

**`header_len`** counts every byte from offset 0 up to (but not including) the start of the payload. It MUST be at least 64 (fixed header alone) and equals 64 + 40×has_state + Σ(TLV sizes).

## 4. NodeState

Present iff `flags.has_state` is set. Fixed-length, fixed-order, **40 bytes** (10 × f32 LE):

| Off (within state) | Size | Field | Range / notes |
|---|---|---|---|
| 0 | 4 | `position.x` | f32 LE, metres |
| 4 | 4 | `position.y` | f32 LE, metres |
| 8 | 4 | `position.z` | f32 LE, metres |
| 12 | 4 | `attitude.yaw` | f32 LE, radians |
| 16 | 4 | `attitude.pitch` | f32 LE, radians |
| 20 | 4 | `attitude.roll` | f32 LE, radians |
| 24 | 4 | `gain` | f32 LE, linear; typically around 1.0 |
| 28 | 4 | `attenuation` | f32 LE, rolloff exponent; typically 1–3 |
| 32 | 4 | `diffusion` | f32 LE, `[0, 1]` |
| 36 | 4 | `directivity` | f32 LE, `[0, 1]`, applied along source's local `+X` |
| 40 | – | (end of struct) | |

**Total NodeState size: 40 bytes.** The fixed header is 64 bytes; with NodeState the header section is 104 bytes.

### Semantics

- `position` — source's location in the space's coordinate frame.
- `attitude` — source's orientation. Local `+X` of the rotated frame is the **facing direction**, also used as the directivity axis.
- `gain` — linear emission level. Multiplied into the rendered signal.
- `attenuation` — exponent in the distance rolloff law applied by the mixer. 1.0 = linear, 2.0 = natural inverse-square (acoustic free field), 3.0 = inverse-cube. The mixer evaluates `1 / max(d, d_ref)^attenuation` (or equivalent) given the actual source-to-listener distance.
- `diffusion` — angular spread of the source. `0.0` = perfect point source; `1.0` = fully diffuse (no localizable direction). Spec for how the mixer realizes this is implementation-defined.
- `directivity` — emission pattern factor applied along the source's local `+X`. `0.0` = omnidirectional; `1.0` = full cardioid. Receivers SHOULD use the mic-pattern formula `gain(θ) = (1 − k) + k·cos(θ)` where `k = directivity` and `θ` is the angle from local `+X` to the source-to-listener vector. Other values in `[0, 1]` interpolate between omni and cardioid (sub-cardioid for `k < 0.5`, hypercardioid behaviour is not produced — values above 1.0 are reserved).

If `has_state` is unset, the receiver MUST treat all source state as unchanged from the most recently received state for that `(space_id, node_id)`. If no prior state is known, the receiver SHOULD NOT spatialize the audio (treat it as muted, or as a position-at-origin omni source, depending on policy).

## 5. TLV extensions

Present iff `flags.has_ext_tlv` is set. The TLV block starts immediately after NodeState (if present) or immediately after the fixed header (if not).

### 5.1 Each TLV

```
Off  Size  Field
 0   1     type        u8
 1   2     len         u16 LE; length of `bytes` in octets
 3   len   bytes       opaque per type
```

Max single TLV payload size: 65 535 bytes.

### 5.2 Block

TLVs are laid out contiguously. The block ends when the running offset reaches `header_len`. Receivers iterate `(type, len, bytes)` until the block is exhausted.

### 5.3 Registry policy

- **`0x00`** — reserved (MUST NOT be used).
- **`0x01`–`0x7F`** — standardized; allocated by this specification.
- **`0x80`–`0xFF`** — experimental, vendor, private use; not portable.

No standardized TLV types are defined in v1. Future revisions allocate from `0x01`–`0x7F` without bumping `version`.

### 5.4 Receiver rules

- Unknown TLV types MUST be skipped (advance by `3 + len` and continue). This is the forward-compatibility primitive.
- Duplicate types in one packet: **first occurrence wins**; subsequent ones MUST be ignored. This lets relays prepend overrides without rewriting.
- TLV order is not significant. Receivers MUST NOT seek by position.
- TLVs that overrun `header_len` make the packet malformed; receivers MUST drop such packets.

## 6. Fragmentation

A logical packet whose total size exceeds MTU is split into N fragments, each transmitted as an independent UDP datagram (or MOQ object).

### 6.1 Sender rules

- All fragments of a logical packet share **identical** values for: `space_id`, `node_id`, `seq`, `sample_ts`, `codec_fourcc`, `format`, `channels`, and `packet_type`.
- `fragment_count` is the total number of fragments and is identical across all of them.
- `fragment_idx` runs `0 .. fragment_count - 1`.
- `header_len` and the `flags.has_state` / `flags.has_ext_tlv` bits — and the corresponding NodeState and TLV sections — are present **only in fragment 0**. Fragments `1 .. fragment_count - 1` carry the bare 64-byte fixed header followed by their slice of the payload.
- `payload_len` on each fragment is the length of that fragment's payload slice (not the total).
- The logical payload is the byte-wise concatenation of fragment payloads in index order.

### 6.2 Receiver rules

- Reassembly key: `(node_id, seq)`.
- A reassembly buffer SHOULD timeout fragments after a small bounded interval (a few frame durations; implementation-defined).
- A reassembled packet is treated identically to an unfragmented packet.
- Fragments that arrive after timeout, or duplicates of an index already received, are dropped silently.

### 6.3 When not to fragment

Fragmentation increases loss probability (one missing fragment loses the whole logical packet). Senders SHOULD avoid fragmentation for the hot audio path:

- Mono / stereo Opus packets fit comfortably in any MTU.
- f32 PCM at FOA (4 ch × 240 samples × 4 B = 3840 B) requires fragmentation on standard 1500-byte MTU.
- Internal hops over networks supporting larger MTUs (e.g., 8896-byte frames in GCP) should be preferred for HOA streams.

## 7. `packet_type`

| Value | Name | Notes |
|---|---|---|
| 0 | RESERVED | MUST NOT be sent |
| 1 | AUDIO | normal audio frame; payload interpreted per `codec_fourcc` + `format` |
| 2 | CONTROL | payload is a control message (format defined per use case) |
| 3 | PING | keepalive / liveness probe; `payload_len` typically 0 |
| 4 | BUNDLE (reserved) | reserved for future multi-source bundling; MUST NOT be sent in v1 |
| 5–127 | reserved | standardization range |
| 128–255 | experimental / vendor | not portable |

For CONTROL and PING packets:

- `flags.has_state` SHOULD be 0.
- `fragment_count` MUST be 1.
- `codec_fourcc` SHOULD be `b"none"` (the four bytes `6E 6F 6E 65`).
- `format` SHOULD be 0 (UNDEFINED).
- `channels` SHOULD be 0.

## 8. `codec_fourcc`

Initial registry. Codes aligned with IAMF where the mapping is direct.

| FourCC | Hex bytes | Codec |
|---|---|---|
| `b"none"` | `6E 6F 6E 65` | no payload (control / ping) |
| `b"Opus"` | `4F 70 75 73` | RFC 6716 Opus, one Opus packet per audio frame |
| `b"ipcm"` | `69 70 63 6D` | Linear PCM, signed 16-bit LE per sample (IAMF-aligned) |
| `b"fLaC"` | `66 4C 61 43` | FLAC (frame-aligned) |
| `b"mp4a"` | `6D 70 34 61` | AAC-LC |
| `b"f32 "` | `66 33 32 20` | raw 32-bit IEEE 754 float PCM, little-endian |
| `b"i24 "` | `69 32 34 20` | raw 24-bit signed PCM, little-endian |

FourCCs are case-sensitive. The mixed casing of `Opus`, `fLaC`, `mp4a` matches established conventions and is preserved.

## 9. `format`

Describes the channel layout / spatial interpretation of the payload.

| Value | Name | Channel count | Notes |
|---|---|---|---|
| 0 | UNDEFINED | per `channels` field | use for non-audio packet types |
| 1 | MONO | 1 | single channel, treated as a point source at NodeState pose |
| 2 | STEREO | 2 | L, R; positional interpretation is implementation-defined |
| 3 | BINAURAL | 2 | L, R, pre-rendered for headphone playback (no further spatialization) |
| 4 | AMBISONIC | (N+1)² for order N | AmbiX channel ordering, SN3D normalization |
| 5–127 | reserved | | |
| 128–255 | experimental / vendor | | |

For AMBISONIC, the ambisonic order is `isqrt(channels) - 1`. Senders MUST set `channels` to exactly `(order + 1)²`; receivers MAY reject mismatched values.

## 10. Validation rules

A receiver MUST drop a packet if any of the following hold:

1. Packet length < 64.
2. `magic` ≠ `0xA5`.
3. `version` is not a version this implementation supports.
4. `flags` has any reserved bit (bits 2–7) set to a non-zero value (forward-compat trap; SHOULD be relaxed in future versions if reserved bits are allocated).
5. `header_len` < 64.
6. `header_len` < required size for the indicated `flags` (`64 + 40·has_state + sum of TLV sizes`).
7. `header_len + payload_len` > received packet length.
8. `fragment_count` = 0.
9. `fragment_idx` ≥ `fragment_count`.
10. For AMBISONIC `format`, `channels` is not a positive perfect square.

A receiver MAY (and SHOULD, in production) additionally:

- Reject packets with `version` ≠ 1 (this version) unless explicitly forward-compatible.
- Treat `sample_ts` going backwards by more than a tolerance as a sender restart and reset its jitter buffer for that `(space_id, node_id)`.
- Track `seq` for loss / reorder telemetry but never block on it.

## 11. Worked example

Minimal PING packet from `node_id = 0000…0001` in `space_id = 0000…00FF`, `seq = 42`, `sample_ts = 0`:

```
Field            Hex
─────────────────────────────────────────
magic            A5
version          01
flags            00          (no state, no TLV)
packet_type      03          (PING)
codec_fourcc     6E 6F 6E 65 ("none")
format           00
channels         00
fragment_idx     00
fragment_count   01
seq              2A 00 00 00 (42 LE)
sample_ts        00 00 00 00 00 00 00 00
space_id         00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 FF
node_id          00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 01
payload_len      00 00 00 00
header_len       40 00       (64 LE)
reserved         00 00
─────────────────────────────────────────
total            64 bytes
```

Worked encoders and round-trip test vectors will be generated by the reference Rust implementation (`src/raw_pose.rs`); a JSON file of canonical packets in `tests/wire_vectors.json` will be the cross-language source of truth.

## 12. Compliance checklist

A conforming v1 sender:

- [ ] Writes `magic = 0xA5`, `version = 1`.
- [ ] Writes `header_len` correctly, including NodeState and TLVs.
- [ ] Writes `payload_len` as the actual payload byte count.
- [ ] Sets `fragment_count = 1` when not fragmenting.
- [ ] Uses a monotonic `sample_ts` clock at 48 kHz.
- [ ] Uses RFC 4122 byte order for UUIDs.
- [ ] Uses little-endian for all integer and f32 fields.
- [ ] Uses the ambisonic coordinate frame and units defined in §2.
- [ ] Does not set reserved bits in `flags`.
- [ ] Does not write `packet_type = 0`, `packet_type = 4`, or `packet_type ≥ 128` for portable interchange.

A conforming v1 receiver:

- [ ] Validates per §10.
- [ ] Skips unknown TLV types.
- [ ] Honors `header_len` rather than seeking by fixed offsets past the 64-byte fixed header.
- [ ] Reassembles fragments by `(node_id, seq)` with a bounded timeout.
- [ ] Does not require cross-source clock synchronization at the wire level.
- [ ] Reports validation failures via metrics, does not crash on malformed input.

## 13. Versioning

- `version` is bumped only for genuinely incompatible binary changes (re-ordered fields, removed fields, changed semantics of existing fields).
- Additive changes (new TLV types, new `codec_fourcc` values, new `format` values, new `packet_type` values, new `flags` bits) MUST NOT bump `version`. Implementations are required to forward-skip unknown values.
- A v2 NodeState shape, if ever needed, would be selected by a new `flags` bit (e.g., `bit 2 = has_state_v2`) rather than by re-interpreting the existing state slot.
