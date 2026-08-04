# LASA Core


// TODO

*Draft 2 — 2026-07-27. Supersedes `lasa2.md`. Companion document:
`lasa-base-profile.md`. Design rationale for every decision here is
recorded in `plan/`.*

## 1. Introduction

LASA is a protocol for live spatial audio over Media over QUIC (MoQ).
It is written in the context of the Open Media Alliance's
Eclipsa/IETF work and MPEG-I, and is different from both in having a
deliberately narrower scope: live, low-latency, head-tracked spatial
audio for shared virtual spaces.

A LASA **Space** is a server-hosted virtual acoustic environment.
**Clients** connect to a Space, publish audio and pose for one or more
**Entities**, and subscribe to rendered output. LASA supports three
audio format families:

- **`mono-object`** — multi-track mono object-based input: one mono
  Opus stream plus pose per entity.
- **`ambi2` / `ambi3`** — 2nd/3rd order ambisonic rendered output
  (9 / 16 channels, ACN order, SN3D normalisation).
- **`binaural`** — binaural stereo rendered output.

There are no speaker-layout formats other than binaural. LASA is
opinionated about codec: **Opus at 48 kHz in 5 ms frames**, and
nothing else. (At 5 ms Opus is CELT-only; the trade was priced and
accepted — see `plan/opus-frame-size.md`.)// TODO

LASA also provides game-engine-style semantics for synchronising
distributed data: a small key/value **state store** with server-
assigned sequence ids, resumable subscriptions, and role/permission
filtering.

**LASA commits to MoQ** rather than defining an agnostic packet
format. MoQ is used as a **session, subscription and identity
layer** — track identity carries what would otherwise be per-packet
header fields, and subscription is format negotiation. LASA does
*not* use MoQ as a caching/relay fabric: apart from `presence` (and,
in a possible future relay topology, the uplinks), every track is
per-subscriber and not meaningfully cacheable.

### 1.1 Layering: core and profiles

This document specifies the **core**: media tracks and the state
*machinery*. The core is content-blind — it attaches no meaning to
any state key and functions with an empty vocabulary. The single key
prefix **`lasa.`** (with the trailing dot) is reserved for the
first-party **base profile**, a normative companion specification
(`lasa-base-profile.md`) defining the standard key vocabulary, role
definitions, and key lifetime semantics. All other keys are
application space. There is no profile discovery mechanism and no
registry: clients know what they are connecting to out-of-band.

### 1.2 Conformance language

MUST, MUST NOT, SHOULD, MAY are used per RFC 2119. Non-normative
implementation notes appear as indented *Note* blocks.

## 2. Identifiers

One grammar covers every identifier that can be embedded in a state
key or namespace tuple — entity ids, client ids, space ids, channel
names, role names, attribute names:

```
identifier = [a-z0-9-]{1,128}     (lowercase alphanumeric + hyphen, 1–128 bytes)
```

Dots are excluded so that identifiers embedded in dotted keys remain
unambiguous. Lowercase UUIDs fit the grammar and are the conventional
choice for entity, client and space ids.

State keys are sequences of `[a-z0-9.-]`, at most **512 bytes**.

The **space id** is a deployment-assigned identifier. It is the root
of every namespace tuple, and the value a Ticket's `aud` claim MUST
equal (§4.3).

## 3. Tracks and namespaces

Full track identity is a MoQ *(namespace tuple, track name)* pair.
The namespace shapes are:

- `(space, "entity", entity-id, direction)` — per-entity media
- `(space, "client", client-id, direction)` — per-client state
- `(space, "presence")` — the one space-wide broadcast

`direction` ∈ {`source`, `sink`} and is always **entity/client-
centric**: `source` is what the entity or client emits, `sink` is
what it receives.

**Track names are format names.** A track name pins a complete wire
format; a changed layout is a new track name, never a version byte.
**Subscription is the format negotiation**: a client subscribes to
the format it wants; there is no catalog and no in-band negotiation.
New formats are additive — new names that break nothing.

| Tuple | Track name | Publisher | Delivery |
|---|---|---|---|
| `(space, "entity", e, "source")` | `mono-object` | client | datagram |
| `(space, "entity", e, "sink")` | `binaural`, `ambi2`, `ambi3` | space | datagram |
| `(space, "client", c, "source")` | `state` | client | stream |
| `(space, "client", c, "sink")` | `state` | space | stream |
| `(space, "presence")` | `presence` | space | datagram |

A conforming Space MUST offer all three sink formats — `binaural`,
`ambi2`, `ambi3` — for every entity.// TODO

### 3.1 Subscribe and announce authorization

Enforced at one point, against the binding established at connection
(§4):

| Track | May subscribe |
|---|---|
| `(space, "entity", e, "sink")` (any format) | the connection owning e; further grants are profile policy |
| `(space, "client", c, "sink")` / `state` | the connection with client id c only |
| `(space, "presence")` | any connected client |
| `(space, "entity", e, "source")` | the space only |
| `(space, "client", c, "source")` | the space only |

Announce is the mirror: a client MAY announce only its own `source`
namespaces. A subscribe or announce whose namespace tuple falls
outside the connection's binding MUST be rejected.

> *Note (privacy): subscribing to an entity's sink is hearing the
> space through that entity's ears. The base profile grants this
> beyond the owner to the `admin` role; deployments should surface
> this fact to users.*

### 3.2 Sink lifecycle

Sink formats are independent tracks and **subscription is the only
lifecycle signal**: the server renders and encodes an entity's sink
only while at least one subscription to any of its formats is active;
a new subscription joins at live (no history); concurrent formats are
permitted (each pays its own encode); switching formats is
subscribe-new-then-unsubscribe-old, with both flowing during the
overlap. Any limits on concurrency are deployment policy.

## 4. Connection

### 4.1 The Connection Config rides CLIENT_SETUP

A client presents a **Connection Config** exactly once, at MoQ
session establishment, as a setup parameter on CLIENT_SETUP:

- **Parameter key `0x1A5A1`** (odd — length-prefixed bytes).
- Value: the UTF-8 JSON of the Connection Config
  (`schemata/connection.schema.json`):

```
{ "client_id": <identifier>,
  "ticket":    <JWT string>,        (optional)
  "entities":  [Entity, ...],       (see entity.schema.json)
  "setups":    [Setup, ...] }       (see setup.schema.json; requires ticket)
```

At least one of `entities` / `setups` MUST be present; both MAY be.
The server authenticates the session from this one message, derives
every legal track tuple from the config's client and entity ids, and
enforces §3.1 against that binding thereafter. On success the server
completes SERVER_SETUP; on failure it terminates the session (§4.6)
without completing the handshake. This is identical over WebTransport
and raw QUIC.

### 4.2 Entities and Setups

An entity comes to exist on a connection in one of two ways:

- **`setups` instantiate ticket-defined entities.** A Setup
  `{id, attrs, initial-pose}` references, by id, an entity definition
  inside the signed ticket. The ticket carries the
  authorship-guaranteed definition; the setup supplies the
  runtime-only parts (initial pose, overriding the ticket
  definition's default; unsigned attrs). Every `setup.id` MUST
  reference an entity in the ticket; unknown or duplicate ids reject
  the connection. Instantiating a subset is permitted.
- **`entities` are ad-hoc definitions** carried directly in the
  config. Admitted when the Space is unticketed, or when the ticket
  authorizes it (base profile: the `admin` and `presenter` roles).

Duplicate-id rules — noting that a setup's id necessarily also
appears in the ticket's entities block; that reference is the
instantiation mechanism, not a collision: no id may appear twice
within the setups list; no id may appear twice within the entities
list; and no ad-hoc entity id may equal any ticket-defined id,
instantiated or not — ad-hoc entities cannot shadow signed
definitions. Any violation rejects the connection.

**Zero-entity connections are legal** (`entities: []`) — the
observer/console pattern: such a connection may subscribe `presence`,
its own state sink, and whatever sinks its grants allow.

### 4.3 Tickets

A Ticket is a signed JWT (`schemata/ticket.schema.json`). EdDSA
(Ed25519) is RECOMMENDED. The server MUST verify:

- the signature, against the Space's configured issuer key(s);
- **`aud` equals the space id** — what stops a ticket minted for one
  space being replayed into another;
- `exp`/`nbf` when present. A ticket without `exp` never expires;
  issuers should set it deliberately, as it is the only bound on
  reuse (§4.5).

The `panaudia` claim is required and carries `client_id` (MUST equal
the config's `client_id`), optional `roles`, optional `permissions`
(three arrays of key prefixes: `set`, `clear`, `read`), and optional
`entities`. Role *names* are profile vocabulary; the core carries
them opaquely. The connection's effective state access is the union
of its roles' rules and its ticket's permissions (§6.4).

### 4.4 Unticketed spaces and admission

A Space MAY be configured to accept unticketed connections. This mode
is deliberately fragile — identity is claim-based with no authorship
guarantee — and is intended for development, demos and closed
networks. An unticketed config carries `entities` directly (no
`setups`, which require a ticket).

Admission collision rules:

- **Unticketed:** if the presented client id or any entity id is
  currently connected, the connection MUST be rejected. First claim
  wins; the claim lasts exactly as long as the connection. The new
  connection fails; the live one is never evicted (without a
  signature, evict-on-collision would be an id-hijack primitive).
- **Ticketed, same client id live:** the old connection is
  **evicted** wholesale — the ticket is the proof of principal that
  unticketed mode lacks, and this serves the half-open-session
  reconnect case. (Two devices sharing one ticket therefore do not
  get two sessions; the multi-device story is more tickets.)
- **Ticketed, entity id collides with a different client's live
  entity:** reject — unless the incumbent is unticketed, in which
  case the incumbent is evicted (signed beats unsigned).

Any state cleanup owed by an evicted connection MUST be completed or
cancelled before the replacement connection's admission writes are
applied.

### 4.5 Reconnect

**"Reconnect" is not a protocol concept.** Every connection is a
fresh connection presenting a full Connection Config. Continuity
exists in exactly one place, where it is explicitly designed: the
state cursor (§6.6). Media tracks have none — all per-track ids and
receiver state are scoped to the (MoQ session, track) instance
(§5.2). A ticket remains valid for reconnection until `exp`.

### 4.6 Rejection and error codes

Rejection is a deliberate close of the established session (QUIC
CONNECTION_CLOSE application error / WebTransport session close), so
the client always receives a code and a reason string on both
transports. Codes fit in 32 bits (the WebTransport bound) and sit in
a LASA range distinct from MoQ's own session-terminate codes.
Correctness MUST ride the code alone; reason phrases are advisory,
may be truncated or dropped, and MUST NOT require parsing.

| Code | Meaning |
|---|---|
| `0x1A5A00` | malformed / schema-invalid Connection Config |
| `0x1A5A01` | invalid ticket (signature, expired, not yet valid, wrong audience) |
| `0x1A5A02` | `client_id` does not match the ticket claim |
| `0x1A5A03` | id collision (unticketed live client or entity id) |
| `0x1A5A04` | entity id collides with another client's live entity |
| `0x1A5A05` | ad-hoc entities not permitted for this ticket |
| `0x1A5A06` | blocked |
| `0x1A5A07` | space at capacity |

The blocked rejection's reason string SHOULD carry the block expiry
timestamp.

## 5. Audio formats

### 5.1 Common structure

**Sequence ids are MoQ ids: one group per frame.** There is no seq
field in any payload. The group-id increments by exactly one per
packet, the object-id is always 0; the group-id *is* the frame
sequence number. **No gaps**: pose-only uplink packets increment too,
and sink tracks are continuous — 200 packets/s regardless of audio
content. This is load-bearing: the redundancy offset (below) is
denominated in these ids, so `offset = 3` MUST mean exactly 15 ms.

**Byte order.** Every fixed-width field in this specification —
length prefixes, presence indexes, timestamps, and the pose
components below — is **little-endian**. QUIC varints keep their
RFC 9000 encoding.

**The flags byte.** Every audio packet begins with one flags byte.
Bit positions are shared across formats; not every bit is meaningful
in every format:

| Bit | Meaning | Applies to |
|---|---|---|
| 0 | pose present | `mono-object` |
| 1 | audio present | `mono-object` |
| 2 | redundancy present | all |
| 3–5 | redundancy offset (1–7) | all |
| 6 | ts present | sink formats |
| 7 | reserved, MBZ | — |

Reserved bits MUST be zero; a receiver MUST discard packets with
unknown bits set (they are not this format). The redundancy-offset
bits 3–5 are meaningful only when bit 2 is set and are MBZ when it
is clear; a packet with bit 2 clear and a nonzero offset falls under
the same discard rule.

**Pose encoding — 18 bytes**, byte-for-byte identical wherever a pose
appears on the wire: `x, y, z` as IEEE-754 f32 (metres; x forward, y
left, z up), then `yaw, pitch, roll` as int16 scaled to ±π/32767
(positive yaw anticlockwise about z; positive pitch upward; positive
roll anticlockwise about the line of sight).

The pose in a `mono-object` packet is **the freshest pose at packet
assembly time** — sampled after the audio is encoded, deliberately
not synchronised with the audio samples in the same packet. This is a
stated property: it minimises pose staleness for the rotation path,
at the price of the source being rendered at a position roughly one
packetisation interval fresher than its audio — imperceptible at
human movement speeds.

**Redundancy framing — one rule for all formats.** When the
redundancy bit is set the packet carries two audio payloads: the
current frame and a full copy of the frame `offset` packets ago
(offset 1–7 from the flag bits; burst cover up to 35 ms). Neither an
elementary Opus packet nor a multistream packet is self-delimiting,
so a **u16 length prefix on the primary payload** separates the two.
When the redundancy bit is clear there is no length prefix. The
referenced packet MUST be audio-bearing. The format carries the
capability only: whether redundancy is on is the sender's per-packet
choice, and any adaptation policy is profile vocabulary over state,
not format surface.

> *Note: the offset converts loss into latency at `offset × 5 ms` and
> must be tuned jointly with jitter-buffer depth — an offset larger
> than the buffer depth is unusable.*

**Timestamps.** Uplink: none, deliberately — the server re-times
everything at the mix, and a source timestamp would wrongly suggest
participation in presentation timing. Sink formats: occasional, not
per-packet. Frame spacing is exactly 5 ms on the server's clock, so a
single (seq, ts) anchor extends to every frame:
`ts(seq) = anchor_ts + (seq − anchor_seq) × 5 ms`. When the
ts-present bit is set the packet carries a **u64 unix-epoch
microseconds** timestamp naming **the first sample of the
accompanying frame**; emission cadence is on stream start and on the
order of once per second. The anchor is per track instance and
re-established after reconnect. These timestamps are informational;
presentation timing does not depend on them.

### 5.2 Id continuity: session-scoped, reconnect restarts at zero

Entity and client ids come from the Connection Config, so a
reconnecting client republishes the same track tuple — but the id
space does not carry over. A reconnect is a **new track instance**
and group-ids restart at 0. (Id arithmetic is time arithmetic only
within a contiguous run; stitching across a wall-clock hole would
silently break the redundancy offset and freshness logic.) All
per-track receiver state — latest-wins ids, jitter buffers,
redundancy history, ts anchors — is scoped to the (MoQ session,
track) instance and resets with it. Nothing in the packet format
signals a reconnect; the new MoQ session is the signal.

### 5.3 Layouts

```
mono-object:  [flags][pose(18)?][len(2)?][opus][opus-dup?]
binaural:     [flags][ts(8)?]   [len(2)?][opus][opus-dup?]
ambi2/ambi3:  [flags][ts(8)?]   [len(2)?][opus-ms][opus-ms-dup?]
```

Everything after the flags byte is present iff its flag bit (or the
redundancy bit, for `len`) says so. Frame identity rides the MoQ
group-id; there is no other header.

- **`mono-object`**: one 5 ms mono Opus frame (when audio present) at
  48 kHz, plus optional pose. Pose-only packets are silence; audio-
  only packets are pose decimation — both sender choices with zero
  format cost.
- **`binaural`**: one 5 ms stereo Opus frame — the binaural rendering
  of the space as heard by the entity.
- **`ambi2` / `ambi3`**: the audio section is exactly one Opus
  multistream packet (RFC 7845 framing: per-stream packets
  concatenated, all but the last self-delimited per RFC 6716 App. B),
  produced and consumed whole by the standard multistream API.
  Format-in-name is the channel-configuration negotiation: `ambi2` =
  9 channels as 9 uncoupled mono streams, `ambi3` = 16 channels as 16
  uncoupled mono streams (`coupled = 0`, identity mapping), ACN
  order, SN3D normalisation.

### 5.4 The pose fast path (non-normative)

// TODO

> *Note: pose SHOULD bypass the audio jitter buffer. Audio is a
> stream where late packets are worth waiting for; pose is a
> latest-wins sampled signal where a late sample is worthless. The
> two payloads of one datagram take different paths: audio → jitter
> buffer → mixer tick; pose → applied on arrival, guarded only by the
> group-id latest-wins rule. Combined with assembly-time pose
> sampling this removes an estimated 15–25 ms from the rotation loop
> with no wire change. See `plan/audio-packet-structure.md` §8 for
> the full three-stage rotation-latency picture and its measurement
> method.*

## 6. State

### 6.1 The store

The state store is a distributed key/value store with two operations:
`set` assigns a value to a key; `clear` removes it. Keys follow the
grammar of §2. Values are opaque binary, at most **64 KiB** — this is
a coordination store, not a blob store. The core attaches no meaning
to any key; `lasa.`-prefixed keys belong to the base profile.

Every operation is assigned a **sequence id** by the store —
monotonic incrementing integers. Operations MAY be grouped; a
**group** is a hint that its operations should be applied together
and provides **no consistency guarantee**. A group's op count is
capped by a server constant, well below any delivery window —
without a cap, one over-large group is a forced-resync primitive
against every subscriber. An over-cap group is silently dropped
whole, never partially applied. Dotted keys suggest
nesting but the store is flat: removing a parent removes nothing, and
a leaf can share a name with another key's parent.

Clients track the sequence id per key and MUST ignore any operation
whose id is older than or equal to the current one for that key.

### 6.2 Writes and the echo

A client writes by publishing ops on its state **source** track. Ops
carry no seq upstream; the server assigns one on acceptance. **The
only acknowledgement is the echo**: an accepted op comes back, with
its seq, on the client's own state sink subscription. All rejections
are silent — no echo means it didn't happen. The silent-drop causes:
the write is denied by role/permission rules; the key matches none
of the connection's write grants; the value exceeds 64 KiB; the
group exceeds the op-count cap (§6.1).

Two shapes are *accepted yet never echoed* — delivery filtering, not
rejection: a write under a `set`/`clear` grant without the matching
`read` grant is applied but its echo is dropped by the read filter
(an ack-less grant shape); and a write to a key outside the
connection's subscribed prefixes produces no echo on that
connection. Clients SHOULD subscribe to what they write.
Acknowledgement is **per-key state confirmation, not per-op**: a
write made before subscribing is confirmed by the compacted
catch-up — or, if superseded, never.

### 6.3 Subscription

A client holds exactly **one** state subscription: its per-client
sink track, filtered by a **set of key prefixes fixed at subscribe
time**. Scope is the union of the prefixes (overlap is benign; a key
matching several prefixes is delivered once). The set containing the
empty prefix (`[""]`) subscribes to everything; an empty *set* is a
protocol error. Changing the set means unsubscribing and
re-subscribing. **Subscription filtering is plain string prefix** —
`foo` matches `foobar` as well as `foo.bar`; keys are strings and the
dotted structure is convention.

### 6.4 Access rules

The server filters both directions per connection through one merged
rule set: the union of the connection's role-derived rules and its
ticket's `permissions` grants (roles are compact aliases over the
same machinery; role definitions live in profiles). Unlike
subscription filters, **access rules match exact-key-or-dot-extended
prefix**: a scope `p` matches key `k` iff `k = p′` or `k` begins with
`p′ + "."`, where `p′` is `p` without any trailing dot. (Plain-prefix
matching here would either hide a bare key from its own dotted scope
or leak sibling ids.)

A scope may contain **`*` segments**, each matching exactly one
identifier segment — `lasa.entity.*.dof` matches
`lasa.entity.a1.dof` (and its dot-extensions) but not
`lasa.entity.a1.render.dof`. Since identifiers never contain dots
(§2), segment matching is unambiguous. Wildcards exist for role
expansion — profile role tables grant key suffixes across all
entities, present and future (base profile §2) — and are legal only
in server-side expanded rules; ticket `permissions` prefixes remain
literal (`ticket.schema.json` is unchanged).

### 6.5 Delivery: delta, snapshot, frontier

On subscribe the server answers with exactly one of:

- **delta** — for every key in scope whose latest op has seq > the
  presented cursor, the latest op only (a `set` if the key lives, a
  `clear` if deletion was most recent). Compaction means intermediate
  history is never delivered and groups are not preserved.
- **snapshot** — replace-all: the client discards its local store
  across its subscribed prefixes and adopts the snapshot (per-key
  seqs included; snapshots never contain tombstones).

The server MUST snapshot when it cannot prove a delta complete and
MAY snapshot whenever else (e.g. when the delta would be larger).
Every response closes with a **frontier** message — "you will receive
nothing further **at or below** X on this path" (inclusive, so a
cursor equal to the cut is fully caught up) — carrying the explicit cut
`(epoch, seq)`, never derived from content. Frontiers are also
emitted periodically during live operation and are how the client
learns the epoch. Frontiers require ordered reliable transport: state
can never migrate to datagrams without revisiting this design.

Delivery is **live-first**: live ops flow immediately; the catch-up
payload streams lazily alongside; the server never coordinates the
two. Client ordering rules: (1) apply live ops under per-key dedup;
(2) apply a catch-up entry only if its seq > the local per-key seq;
(3) when a **snapshot** completes, delete every in-scope local key
absent from it whose local seq ≤ the cut H — rule 3 is
snapshot-only; absence from a delta means unchanged.

> *Note: during the catch-up window a live `clear` can be followed by
> a late catch-up chunk carrying the same key at an older seq; naive
> application resurrects the deleted key. Two conforming strategies:
> **buffer-and-replay** (freeze live application until the frontier,
> then replay under dedup — hazard-free, view frozen for the window)
> and **apply-live with transient tombstones** (record every live
> clear during the window, including for absent keys, and drop the
> records at the frontier — fully live, subtler). The wire exchange
> is identical; the choice is out of protocol scope.*

### 6.6 Cursor, epoch, and the reconnect gate

The client persists one cursor: **(epoch, seq, set-hash)**,
atomically, and never ahead of its persisted store (a client that
does not persist its store MUST NOT persist a cursor). No cursor may
be persisted until the subscription's first catch-up frontier has
been applied — before that point the local store is not at any
provable cut, whatever the delivery strategy. The **epoch**
is 8 opaque bytes, compared for equality only; the server mints a new
epoch whenever seq-space continuity is unprovable (crash, restore,
migration). The **set-hash** is SHA-256 over the canonical prefix-set
encoding (deduplicated, sorted bytewise, each length-prefixed with a
**minimal** varint — §6.7), stamped when
the cursor is earned — a bug-catcher, not a security gate. When
frontiers arrive on both delivery paths, persist the **maximum** seq
per epoch.

Tombstones are retained for a server-configured wall-clock TTL
(default 6 hours) and then flushed; the **watermark** is the highest
seq of any discarded tombstone. Neither TTL nor watermark is
advertised; clients plan around nothing and must handle either
response.

The gate, server-enforced, in order: (1) epoch mismatch → snapshot;
(2) set-hash mismatch → snapshot; (3) seq < watermark → snapshot;
(4) the server cannot prove the connection's effective visibility
unchanged since the cursor was earned → snapshot; (5) seq >
the server's head at the same epoch → snapshot (corrupt client
state); (6) otherwise → delta (or snapshot at the server's
discretion). A fresh client presents nothing and lands on snapshot
with no special case.

**Visibility changes**: gate 4 states an obligation, not a
mechanism — it must cover changes made while the identity was
disconnected, role-definition changes affecting disconnected
identities, and reconnects bearing a different ticket. Tracking a
per-identity visibility-change seq is one implementation; comparing
a hash of the connection's effective visibility rules is another.
Mid-session changes are applied by
terminating the state subscription via native MoQ termination — the
session and all audio tracks stay up; the client re-subscribes and
hits the gate. Global rule-set changes MAY be an epoch bump plus
termination of all state subscriptions. Under this specification the
only mid-session trigger is a server-side rule/configuration change:
a connection's own roles and permissions ride its ticket, presented
once.

### 6.7 Wire encoding

**The subscribe message is MoQ SUBSCRIBE parameters**: one KVP,
**key `0x1A5A3`** (odd — length-prefixed bytes), on the SUBSCRIBE to
the state sink track:

```
[flags u8]                bit 0: cursor present; others MBZ
[prefix-count varint]
per prefix, in canonical order (deduplicated, sorted bytewise):
    [len varint][prefix bytes]
if cursor present:
    [epoch 8][seq varint][set-hash 32]
```

The set-hash input is exactly the concatenated
`[len varint][prefix bytes]` entries. Varints in the canonical form
MUST be **minimal** — a non-minimal length encoding would hash
differently on each side — and a server MUST reject a subscribe
whose prefix set is not canonical (duplicated, unsorted, or
non-minimal). A fresh client sends flags = 0
and omits the cursor block.

**Messages**, one per MoQ object, `[type u8][body]`, QUIC varints
throughout:

```
set       [1][key-len][key][value-len][value]([seq])
clear     [2][key-len][key]([seq])
group     [3][count][ops...]        each op a full [type][body]; groups do not nest; count capped (§6.1)
frontier  [4][epoch 8][seq]
begin     [5][kind u8]              1 = delta, 2 = snapshot
```

`seq` is downstream-only; upstream ops carry none. The same type
bytes therefore parse differently per direction — implementations
MUST select the layout by track direction and not share a
direction-blind parser. `begin` opens a catch-up payload (ordinary
`set`/`clear` messages; snapshot entries are `set`s with per-key
seqs) closed by a `frontier` carrying the cut. The server SHOULD
preserve a written group's boundaries in live delivery and never
preserves them in catch-up; clients MUST NOT depend on group
boundaries.

**MoQ mapping.** On the sink track, each subscription instance is a
fresh MoQ group. Within it, **subgroup 0 carries live ops** (and the
periodic frontiers) and **subgroup 1 carries the catch-up payload**
(`begin` … `frontier`). Each subgroup is its own QUIC stream:
ordering is guaranteed within each path — all the per-path frontier
promise needs — and backpressure on a large snapshot cannot stall
live ops. Object-ids increment monotonically within each subgroup.
The source track is one group, one subgroup, ordered ops. If a MoQ
stack's subgroup support is awkward, a single ordered stream carrying
the same messages interleaved is conformant-equivalent; the client
rules are unchanged.

## 7. Presence

`(space, "presence")` is the one shared, cacheable broadcast:
datagrams published by the space, subscribable by any connected
client. It is a lossy visualisation feed and is self-contained —
authoritative entity existence lives in state, and presence never
depends on it. MoQ ids: group-per-packet, object 0, +1 per datagram.

Two packet kinds; flags bit 0 = keyframe, other bits reserved-MBZ
with the discard rule of §5.1:

```
delta:     [flags][gen u8] [record]*
           record  = [index u16][pose 18][loudness u8]

keyframe:  [flags][gen u8] [first u16] [krecord]*
           krecord = [kflags u8][id-len u8][id utf8][pose 18][loudness u8]
```

`kflags` bit 0 = **head-frame**: the record's pose (and subsequent
delta poses for this index) is interpreted relative to each
listener's head rather than the shared space — see the base profile's
`frame` key. Bits 1–7 are reserved, MBZ, with the discard rule of
§5.1. The flag appears in keyframes only: frame-ness is fixed at
connection time and membership changes bump the generation, so the
roster always carries it and delta records stay unchanged.

Records parse to the end of the datagram. Pose is the §5.1 encoding.
**Loudness**: u8; 0 = digital full scale, each step −0.5 dB, 255 =
≤ −127 dBFS / silence; it is the entity's source loudness post-gain,
pre-attenuation (attenuation is per-listener and cannot appear in a
shared feed).

- **Indexes are defined by keyframes**: entities listed in index
  order, so the mapping is implicit — position is the assignment. A
  keyframe is one or more self-contained **chunks**; `first` is the
  index of the chunk's first record, subsequent records increment. A
  lost chunk costs only those indexes until the next keyframe round.
- **The generation byte** increments only when membership changes.
  Every delta carries it; a delta whose gen does not match the
  client's last keyframe MUST be discarded. The server SHOULD emit a
  keyframe immediately on membership change. All per-index tracking
  state resets on gen change; the pair (gen, index) is the lookup
  key.
- **Per-entity latest-wins by group-id**: apply a record only if its
  packet's group-id exceeds the last applied for that index.
- **Departure is keyframe absence.** Deltas never signal departure.
- The **index is a server-assigned alias scoped to this track
  alone** — it appears in no key, no namespace, no other track, may
  change across generations, and is never authoritative. Entity
  identity is the id string, everywhere.

Non-normative rates: deltas at ~10–30 Hz, changed entities only;
keyframes on the order of once per second plus on membership change.

Which entities appear is space policy; head-frame entities are
included, distinguished by the `kflags` bit above.

## 8. Security considerations

- Ticket verification requirements are in §4.3; the `aud` check is
  the replay boundary between spaces.
- Unticketed mode is fragile by design: identity is first-claim,
  nothing prevents impersonation across connect boundaries, and
  ad-hoc entities are admitted from anyone. Do not run it on open
  networks.
- A client that saw a key cannot be made to unsee it; visibility
  narrowing is hygiene, not security.
- The sink-subscription privacy property is stated in §3.1.
- Rejected state writes are silent (§6.2); clients should not offer
  UI for actions their grants forbid, confining silent failure to
  races.

## 9. Wire constants

| Constant | Value |
|---|---|
| Connection Config SETUP parameter key | `0x1A5A1` |
| State subscribe SUBSCRIBE parameter key | `0x1A5A3` |
| Session error codes | `0x1A5A00`–`0x1A5A07` (§4.6) |
| Max state key length | 512 bytes |
| Max state value length | 65 536 bytes |
| Max ops per group | server constant (§6.1); reference implementation default 64 |
| Fixed-width byte order | little-endian (§5.1); varints per RFC 9000 |
| State epoch width | 8 bytes, opaque |
| Audio frame | Opus, 48 kHz, 5 ms, one frame per packet |
