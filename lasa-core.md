# LASA Core

The LASA Core contains definition of the audio and data tracks used by LASA 
and how they are carried in MoQ.

This includes the definition the low-level functioning of LASA's distributed state system, 
but not the semantics of any of the state, that can be found in the companion document `lasa-base-profile.md`

## 1. Introduction

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
accepted — see `plan/opus-frame-size.md`.)

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
any state key; the split between the two documents is machinery
versus vocabulary, not optionality. The single key prefix **`lasa.`**
(with the trailing dot) belongs to the first-party **base profile**,
a normative companion specification (`lasa-base-profile.md`)
defining the standard key vocabulary, role definitions, and key
lifetime semantics. **The base profile is not optional**: every
conforming Space implements it, which is why the entity schema the
core references (§4.2) carries base-profile fields. All other keys
are application space, layered on top of the base profile, never
instead of it. There is no profile discovery mechanism and no
registry: clients know what application vocabulary they are
connecting to out-of-band.

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

A conforming Space MUST offer `binaural` and `ambi2` for every
entity, and SHOULD offer `ambi3` too when its internal render order
is 3 or higher (the internal order may exceed the highest wire
format). Formats carry no authorization dimension: a connection that
may subscribe an entity's sink may take it in any offered format. A
profile MAY define entities that have no sink at all — the base
profile's head-frame entities are source-only (base profile §6,
`frame`) — and a subscription to a sink that does not exist is
refused; the offering obligation applies to entities that have
sinks.

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
> beyond the owner to the `admin` and `producer` roles; deployments
> should surface this fact to users.*

### 3.2 Sink lifecycle

Sink formats are independent tracks and, while the entity lives,
**subscription is the only lifecycle signal**: the server renders and
encodes an entity's sink only while at least one subscription to any
of its formats is active; a new subscription joins at live (no
history); concurrent formats are permitted (each pays its own
encode); switching formats is subscribe-new-then-unsubscribe-old,
with both flowing during the overlap. Any limits on concurrency are
deployment policy. When an entity departs, its sink tracks end with
it: the server closes any live subscription to them (MoQ
SUBSCRIBE_DONE / track ended).

> *Note (capability is exercised, not declared): an entity never
> declares whether it acts as a source or a sink — the Connection
> Config deliberately carries no such field. An entity is a source by
> publishing on its source track and a listener by holding a sink
> subscription, and either may begin or end at any moment in the
> session. A declaration could only restate observable behaviour or
> contradict it, and every contradiction would need an adjudication
> rule. Entities that stay silent, never subscribe, or publish pose
> without audio are ordinary cases, not special ones.*

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
definitions. Any violation rejects the connection. A duplicate id
within the ticket's own entities block is an issuer error and rejects
the ticket itself (`0x1A5A01`).

**Zero-entity connections are legal** (`entities: []`) — the
observer/console pattern: such a connection may subscribe `presence`,
its own state sink, and whatever sinks its grants allow.

An entity definition (`schemata/entity.schema.json`) carries a
required `id` and a required human-readable `name` (non-unique,
surfaced to other participants — base profile §6); its remaining
fields are optional. The schema deliberately spans both layers:
`id`, `attrs`, `initial-pose` and the two declarations below are
core; `name`, `priority`, `heard-in`/`hears`, `frame`, `dof` and
`render` are base-profile vocabulary whose semantics live in the
profile (§6) — carried in the one schema because the base profile is
not optional (§1.1).

**Quality and redundancy declarations.** An entity definition MAY
carry two declarations about its **audio path** — the stream it
publishes *and* the sink track rendered for it. Each direction has a
jitter buffer at its receiving end; one declaration configures both.
Both fields are fixed for the life of the connection and exist so
each receiving side can provision buffering *before the first packet*
rather than learn it from the first audible failure:

- **`quality`** — integer 0–2, default 0: the path's
  latency-tolerance **intent**. 0 = interactive (minimal latency);
  1 = resilient (moderate added latency for materially fewer
  artifacts); 2 = playback/broadcast (latency is unimportant;
  uninterrupted artifact-free delivery is). The level is intent, not
  parameters: each receiver maps it onto its own buffering policy,
  and that mapping may be re-tuned without protocol change. The space
  applies it to its ingest buffer; the client applies its own
  declaration to its playout buffer. The sink half never touches the
  wire — the client authored the declaration and is the sink's
  receiver — so a client MAY depart from it locally, and an entity
  whose two directions genuinely differ needs no protocol support.
- **`redundancy`** — integer 0–7, default 0: the **maximum**
  redundancy offset (§5.1) on this path; 0 means none. It bounds the
  sender in both directions: the entity's uplink emission, and the
  space's emission on that entity's sink track — for the sink
  direction the declaration doubles as the client's *request*, which
  the space SHOULD honour within deployment policy. A redundant copy
  repairs a loss only if the receiver's buffer already spans the
  offset, so each receiving side provisions buffer depth from the
  declaration: the space for its ingest buffer, the client for its
  playout buffer. A sender MUST NOT emit an offset exceeding the
  declaration; within it, redundancy remains a per-packet choice
  (§5.1).

The two compose (a redundancy-derived buffering floor and a
quality-derived one combine as their maximum) but are independent
inputs. Because these fields are part of the entity definition,
ticket-defined entities carry them authorship-guaranteed; whether
ad-hoc entities may claim elevated quality or sink redundancy — which
cost the space per-entity buffer memory and egress bandwidth
respectively — is deployment policy.

### 4.3 Tickets

A Ticket is a signed JWT (`schemata/ticket.schema.json`). EdDSA
(Ed25519) is RECOMMENDED. The server MUST verify:

- the signature, against the Space's configured issuer key(s);
- **`aud` equals the space id** — what stops a ticket minted for one
  space being replayed into another;
- `exp`/`nbf`/`iat` when present. A ticket without `exp` never
  expires; issuers should set it deliberately, as it is the only
  bound on reuse (§4.5).

**The claims object is a deliberately limited, constrained profile
of JWT**, held to the normative schema exactly: `iss`, `aud` and
`panaudia` are required; `exp`, `nbf` and `iat` are optional but
enforced when present; `aud` MUST be the single-string form, never
the JWT array form. Any other member — registered JWT claims
included (`sub`, `azp`, `jti`) — rejects the ticket: tickets are
grants of capability, not descriptions of identity, and admitting a
claim the server does not consume would imply it has some function.
`jti` in particular is deliberately absent — session exclusivity
rides the `client_id` capability (§4.4), so a per-token id would
have nothing to govern.

The `panaudia` claim is required and carries `client_id` (MUST equal
the config's `client_id`), optional `roles`, optional `permissions`
(three arrays of key prefixes: `set`, `clear`, `read`), and optional
`entities`. Role *names* are profile vocabulary; the core carries
them opaquely — the ticket schema holds them to the identifier
grammar (§2) only, and whether an unknown name is refused is profile
policy, applied at admission (base profile §2). The connection's
effective state access is the union of its roles' rules and its
ticket's permissions (§6.4).

### 4.4 Space mode, capabilities, and admission

A Space is **ticketed or unticketed, exclusively**, fixed at
configuration. In a ticketed Space every connection MUST present a
ticket. In an unticketed Space a config presenting a ticket MUST be
rejected (`0x1A5A08`): a client presenting a ticket believes it is
getting signed identity, and silently admitting it with claim-based
identity would be a quiet degradation. There is no mixed mode.

**Ids are capabilities**, and two facts about a capability are
distinct. **Title** is the right to be a client id and to instantiate
its entity ids: in a ticketed Space, title is granted by the signed
ticket from `nbf` to `exp`, independent of any connection. Identity
is the signed `client_id` claim, not the JWT — a renewed ticket
bearing the same `client_id` is the same capability; entity
capabilities are subordinate to the client capability that carries
them. **Exercise** is a connection currently acting under a
capability; at most one exercise per capability at a time. A
zero-entity connection exercises its client capability like any
other.

Admission collision rules follow from one principle: **contests over
the same capability resolve to the newest exercise; collisions
between different capabilities refuse the newcomer.**

- **Ticketed, same `client_id` live:** the newest exercise wins — the
  old connection is **superseded**: severed wholesale and replaced.
  The returning principal never waits on the server detecting the old
  session's death. (Two devices sharing one ticket therefore do not
  get two sessions; the multi-device story is more tickets.)
- **Ticketed, entity id collides with a live entity the connection
  does not own** — another client's, or one no live connection
  accounts for: reject (`0x1A5A04`). Two distinct capabilities naming
  one id is an issuer coordination error; neither outranks the other,
  and a bystander is never severed on entity grounds. An
  unattributable live marker is an embedder/issuer anomaly — refusal
  surfaces it rather than masking it by silent claim.
- **Unticketed:** any collision — client id or entity id — resolves
  newest-wins: the incumbent connection is superseded wholesale.
  Without title, reclaim and hijack are indistinguishable by
  construction; the rule is chosen for the development restart loop,
  and the mode's fragility is accepted (§8).

Unticketed mode is a **development mode**: identity is
possession-based with no authorship guarantee, intended for
development, demos and closed networks. An unticketed config carries
`entities` directly (no `setups`, which require a ticket).

Any state cleanup owed by a superseded connection MUST be completed
or cancelled before the replacement connection's admission writes are
applied.

The authority for exercised ids is the Space's state serialization
point; the core defines no wire surface for the records. The base
profile materializes them as state keys — `lasa.client.{id}` and the
entity existence markers, whose values name their owning client
(base profile §§3, 6) — adjudicated atomically with the admission
writes, capacity included: collision and capacity refusals issue
from that one point. The moderation-block refusal (`0x1A5A06`) is
the exception — it runs earlier, after ticket verification and
before adjudication (base profile §5), so a blocked id's reconnect
can never reach supersession.

### 4.5 Reconnect

**"Reconnect" is not a protocol concept.** Every connection is a
fresh connection presenting a full Connection Config — in capability
terms (§4.4), a reconnect is a re-exercise of the same capabilities,
superseding any half-open predecessor. Continuity exists in exactly
one place, where it is explicitly designed: the state cursor (§6.6).
Media tracks have none — all per-track ids and receiver state are
scoped to the (MoQ session, track) instance (§5.2). A ticket remains
valid for reconnection until `exp` — title outlives every connection
that exercises it.

### 4.6 Rejection and error codes

Rejection is a deliberate close of the established session (QUIC
CONNECTION_CLOSE application error / WebTransport session close), so
the client always receives a code and a reason string on both
transports. Codes fit in 32 bits (the WebTransport bound) and sit in
a LASA range distinct from MoQ's own session-terminate codes.
Correctness MUST ride the code alone; reason phrases are advisory,
may be truncated or dropped, and MUST NOT require parsing.

> *Note (known defect, temporary): the reference WebTransport stack
> currently fails to surface the close code and reason to browser
> clients — the session ends as a generic connection loss
> (`plan/wt-close-code-defect.md`). Raw QUIC delivers both
> correctly. The normative requirement stands; this note goes when
> the defect does.*

| Code | Meaning |
|---|---|
| `0x1A5A00` | invalid Connection Config: schema violation, a §4.2 rule violation, or the setup parameter missing entirely; also the fallback when admission fails with no more specific code |
| `0x1A5A01` | invalid ticket (signature, expired, not yet valid, wrong audience, duplicate entity definitions, unknown role name where the profile refuses them) — or no ticket presented to a ticketed space |
| `0x1A5A02` | `client_id` does not match the ticket claim |
| `0x1A5A03` | *retired 2026-08-13* (was: unticketed id collision — unticketed collisions now supersede, §4.4); MUST NOT be reused |
| `0x1A5A04` | entity id collides with a live entity the connection does not own (ticketed; §4.4) |
| `0x1A5A05` | ad-hoc entities not permitted for this ticket |
| `0x1A5A06` | blocked |
| `0x1A5A07` | space at capacity |
| `0x1A5A08` | ticket presented to an unticketed space (§4.4) |

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
the same discard rule. The inverse — bit 2 set with a zero offset —
is **malformed** rather than unknown: it is this format but
self-contradictory (offsets are 1–7, and the redundant copy would be
unlocatable); a receiver discards the packet.

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
choice, bounded by the entity's declared maximum offset (§4.2), and
any adaptation policy within that bound is profile vocabulary over
state, not format surface. On sink tracks the space is the sender:
the entity's declaration is both the request for sink redundancy and
the bound on it (§4.2).

> *Note: the offset converts loss into latency at `offset × 5 ms` and
> is only usable if the receiver's jitter buffer spans it — which is
> why the intended maximum is declared up front (§4.2): the receiver
> provisions buffer depth from the declaration instead of discovering
> the need at the first unrepaired loss. An offset beyond the declared
> maximum is a sender violation; the packet's primary payload remains
> valid, but the redundant copy cannot be assumed usable.*

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
silently break the redundancy offset and freshness logic.) The
restarting instance is the **publisher's**: for uplinks that is the
client's connection — reconnect means a new instance and ids from 0
— while for sink tracks it is the render itself (§3.2), whose ids
start at 0 when rendering starts and run continuously across
subscriber churn, so a subscription joining a live render attaches
at the current id, not 0. All per-track receiver state —
latest-wins ids, jitter buffers, redundancy history, ts anchors — is
scoped to the (MoQ session, track) instance and resets with it
either way. Nothing in the packet format signals a reconnect; the
new MoQ session is the signal.

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
  format cost. A packet carrying neither (flags `0x00`) is likewise
  legal: it advances the sequence and says nothing else, keeping the
  no-gaps rule of §5.1 unconditional.
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
the op fails to parse; the write is denied by role/permission rules;
the key matches none of the connection's write grants; the key is
malformed (the §2 grammar or the 512-byte cap); the value exceeds
64 KiB; the group exceeds the op-count cap (§6.1); or the op arrives
after the connection's disconnect sweep has begun — a superseded
connection's in-flight writes are dropped, never applied on the
successor's behalf.

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
or leak sibling ids.) The **empty scope is the universal grant**: it
matches every key — mirroring the subscription side, where the empty
prefix subscribes to everything (§6.3).

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
`[len varint][prefix bytes]` entries. Every varint in this parameter
MUST be **minimal**: in the canonical form because a non-minimal
length encoding would hash differently on each side, and in the
cursor block for uniformity — one rule covers the whole KVP. A
server MUST reject a subscribe whose prefix set is not canonical
(duplicated, unsorted, or non-minimal) or whose cursor seq varint is
non-minimal. A fresh client sends flags = 0 and omits the cursor
block.

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
The source track is one group, one subgroup, ordered ops. This
mapping is **normative**: the lane is identified by its subgroup id
and clients MAY depend on that (the reference clients do). Subgroups
are core MoQ; LASA defines no alternative mapping.

## 7. Presence

`(space, "presence")` is the one shared, cacheable broadcast:
datagrams published by the space, subscribable by any connected
client. It is a lossy visualisation feed and is self-contained —
authoritative entity existence lives in state, and presence never
depends on it. MoQ ids: group-per-packet, object 0, +1 per datagram.

Two packet kinds; flags bit 0 = keyframe, other bits reserved-MBZ
with the discard rule of §5.1:

```
delta:     [flags u8][gen u8] [record]*
           record  = [index u16][pose 18][loudness u8]

keyframe:  [flags u8][gen u8] [first u16] [krecord]*
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
**Loudness**: u8; 0 = digital full scale, each step −0.5 dB down to
254 = −127 dBFS; 255 = silence (below −127 dBFS); it is the entity's
source loudness post-gain, pre-attenuation (attenuation is
per-listener and cannot appear in a shared feed).

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
- Unticketed mode is fragile by design: identity is possession-based
  and collisions resolve newest-wins (§4.4), so id takeover is
  possible by construction; nothing prevents impersonation across
  connect boundaries, and ad-hoc entities are admitted from anyone.
  It is a development mode; do not run it on open networks.
- In ticketed mode a stolen ticket can supersede the legitimate
  holder's session — but a stolen ticket already grants full
  impersonation; supersession adds no new capability.
- Client ids and entity ownership are deliberately visible: the
  entity marker's value names its owning client (base profile §6),
  readable by every participant — including the linkage that several
  entities share one owner. Client ids are names, never secrets and
  never capabilities; the ticket is the capability.
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
| Session error codes | `0x1A5A00`–`0x1A5A08` (§4.6; `0x1A5A03` retired, never reused) |
| Max state key length | 512 bytes |
| Max state value length | 65 536 bytes |
| Max ops per group | server constant (§6.1); reference implementation default 64 |
| Fixed-width byte order | little-endian (§5.1); varints per RFC 9000 |
| State epoch width | 8 bytes, opaque |
| Audio frame | Opus, 48 kHz, 5 ms, one frame per packet |
