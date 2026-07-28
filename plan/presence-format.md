# Presence packet format — decision

*Decided 2026-07-27. Pins the shape sketched in
`audio-packet-structure.md` §Deferred: compact index + in-band roster,
shared pose encoding, log-u8 loudness, changed-only + periodic
refresh. The track is `(space, "presence")` / `presence`
(`track-naming.md`): datagrams, published by the space, the one
shared, cacheable broadcast.*

The format is deliberately slight: machine-readable, bandwidth-first,
a lossy visualisation feed. Authoritative entity existence lives in
state (`lasa.entity.{e}`); presence is self-contained per the
media-without-state rule and never consults it.

## Layouts

```
delta:     [flags][gen u8] [record]*
           record  = [index u16][pose 18][loudness u8]        (21 B)

keyframe:  [flags][gen u8] [first u16] [krecord]*
           krecord = [kflags u8][id-len u8][id utf8][pose 18][loudness u8]
```

`kflags` bit 0 = **head-frame** (the entity's pose is interpreted
relative to each listener's head, not the shared space — the base
profile's `frame` key); bits 1–7 reserved, MBZ, discard-on-unknown.

- **Flags**: bit 0 = keyframe. All other bits reserved, MBZ; a
  receiver MUST discard packets with unknown bits set (the audio-format
  rule, `audio-packet-structure.md` §3).
- **Pose**: byte-for-byte the §4 encoding (f32 x,y,z; int16 angles).
- Records parse to the end of the datagram — no counts, no lengths
  beyond the id prefix.
- **MoQ ids**: group-per-packet, group-id +1 per datagram, object-id
  0 — the standard mapping (§1).

## The decisions

### 1. Indexes are defined by keyframes, guarded by a generation byte

A keyframe lists entities in index order, so the index→id mapping is
implicit and free: position is the assignment. The `gen` byte
increments **only when membership changes** (join/leave), and every
delta carries it. A delta whose gen does not match the client's last
keyframe is discarded — the state store's epoch trick, one byte wide.
Without it, datagram loss across a membership change silently applies
poses to the wrong entities. The server SHOULD emit a keyframe
immediately on membership change, making the discard window typically
zero. u8 wrap is harmless at membership-change rates.

### 2. Keyframes chunk cleanly across MTU

A roster of UUID-ish ids outgrows one datagram beyond ~30 entities, so
a keyframe is one-or-more **chunks**, each self-contained: `first` is
the index of the chunk's first record; subsequent records increment.
No reassembly, no ordering requirement; a lost chunk costs only those
indexes until the next keyframe round.

### 3. Per-entity latest-wins by group-id

The audio pose rule (§8): track the last group-id applied per index,
apply only newer. Reordering handled; a lost delta needs no recovery —
the next supersedes it. All per-index tracking state resets on gen
change.

### 3b. Frame-ness rides the roster, not the deltas

*Revised 2026-07-27 (was: head-frame entities omitted entirely).*
Head-frame entities **are** included, marked by the krecord `kflags`
head-frame bit. The flag lives in keyframes only because it can:
`frame` is connection-time-immutable, and membership changes bump the
generation anyway — so the roster always carries current frame-ness
and the hot-path delta record stays 21 bytes. Clients hold the
per-index frame bit alongside the id mapping.

### 4. Departure is keyframe absence

Deltas never signal departure; an entity missing from the new
generation's keyframe is gone. A frozen avatar for at most one
keyframe interval is the accepted cost of keeping deltas pure;
applications wanting instant authoritative departure have it in state
(the existence marker is cleared by the departure sweep).

### 5. Loudness: log u8

0 = digital full scale, each step −0.5 dB, 255 = ≤ −127 dBFS /
silence. One byte covers meters and proximity cues.

### 6. Rates are server policy (non-normative)

Deltas at ~10–30 Hz, **changed entities only** — a speaking or moving
entity costs 21 B per tick, an idle one costs nothing. Keyframes on
the order of once per second, plus immediately on membership change.

## Identity and aliases — the scoping rule

Entity ids are never server-assigned: they come from the Connection
Config (ticket-authored or client-claimed, policed by the
live-collision rules in `connection-and-auth.md`), and the string id
is the one true identity — namespaces, state keys, moderation keys.

The presence **index is a server-assigned alias, scoped to this track
alone**:

- assignment is implicit in the keyframe (no allocation message, no
  registration);
- validity is one generation — after a gen bump an entity may get a
  different index and freed indexes may be reused; clients treat
  (gen, index) as the lookup key. The server MAY keep indexes stable
  across generations as a courtesy; clients MUST NOT rely on it;
- the alias appears nowhere else. A compact alias is only worth its
  bookkeeping where an id would repeat at rate, and presence is the
  only such place: audio tracks carry no per-packet entity id at all —
  there identity rides the track, compressed by MoQ's own track-alias
  mechanism at the protocol layer. The presence index is the same move
  at the payload layer, once, where MoQ cannot reach. Neither alias is
  ever authoritative.

## Costs (house denominator: 1 B = 1.6 kbps at 200 pkt/s; presence runs far below that)

30 active entities at 20 Hz ≈ 100 kbps for the whole space's motion —
about one binaural stream. The u16 index caps a space at 65k entities,
comfortably inside the differential-presence reopen trigger
(`track-naming.md` §4: large-N spaces where presence bandwidth is a
measurable fraction of audio).

## Rejected

- **Loudness-only records** (a smaller record kind for
  speaking-but-still entities) — a second record shape to save ~18 B
  on a feed this small; not worth the surface. Reopen with the
  differential-presence work if large-N makes it real.
- **Timestamps** — nothing correlates by them in v1; a timestamped
  variant later is a new track name, additive (reserved flag bits are
  *not* an additive path — unknown-bit packets are discarded by rule).
- **Sticky global indexes** (server assigns each entity a stable
  number for the life of the space) — index reuse after departure
  recreates exactly the misattribution hazard the gen byte exists to
  kill, and stability invites the alias to leak beyond the track.

## Open items

None.
