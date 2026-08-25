# LASA Core

The LASA Core defines the audio and data tracks LASA uses and how
they are carried in MoQ, together with the low-level machinery of
LASA's distributed state system. The meaning of the state is not
defined here. It lives in the companion document
`lasa-base-profile.md`.

## 1. Introduction

A LASA **Space** is a server-hosted virtual acoustic environment.
**Clients** connect to a Space, publish audio and pose for one or more
**Entities**, and subscribe to rendered output. LASA supports three
audio format families.

- **`mono-object`** is the multi-track mono object input, one mono
  Opus stream plus a pose per entity.
- **`ambi2`** and **`ambi3`** are 2nd and 3rd order ambisonic
  rendered output (9 and 16 channels, ACN order, SN3D normalisation).
- **`binaural`** is binaural stereo rendered output.

LASA also provides game-engine-style semantics for synchronising
distributed data: a small key/value **state store** with
server-assigned sequence ids, resumable subscriptions, and role and
permission filtering.

LASA commits to MoQ rather than defining an agnostic packet format.
MoQ serves as the session, subscription and identity layer. Track
identity carries what would otherwise be per-packet header fields,
and subscription is the format negotiation.

### 1.1 Core and profiles

This document specifies the **core**: media tracks and the state
machinery, but not state parameters.

The state namespace starting with key prefix **`lasa.`** is used by the
**base profile**, which is described in a sibling document (`lasa-base-profile.md`).
This defines the standard key vocabulary used by LASA,
the role definitions, rendering options and permissions, etc. 
The base profile is not optional. Every conforming Space implements it. 

All other key values are available for use by applications, which can define their own profiles.
Extending values can be layered on top of the base profile, rather than replacing it.

There is no profile discovery mechanism and no
registry. Clients know what application vocabulary they are
connecting to out-of-band.

### 1.2 Conformance language

MUST, MUST NOT, SHOULD, MAY are used per RFC 2119. Non-normative
implementation notes appear as indented *Note* blocks.

## 2. Identifiers

One grammar covers every identifier that can be embedded in a state
key or namespace tuple: entity ids, client ids, space ids, channel
names, role names, and attribute names.

```
identifier = [a-z0-9-]{1,128}     (lowercase alphanumeric + hyphen, 1–128 bytes)
```

Dots are excluded so that identifiers embedded in dotted keys remain
unambiguous. Lowercase UUIDs fit the grammar. Issuers that cannot
otherwise guarantee uniqueness within a space SHOULD mint client and
entity ids as lowercase RFC 9562 UUIDs; implementations MUST NOT
assume any structure beyond the grammar.

State keys are sequences of `[a-z0-9.-]`, at most 512 bytes.

The **space id** is a deployment-assigned identifier. It is the root
of every namespace tuple, and it is the value a Ticket's `aud` claim
MUST equal (§4.3).

## 3. Tracks and namespaces

Full track identity is a MoQ *(namespace tuple, track name)* pair.
There are three namespace shapes.

- `(space, "entity", entity-id, direction)` carries per-entity media.
- `(space, "client", client-id, direction)` carries per-client state.
- `(space, "presence")` is the one space-wide broadcast.

`direction` is `source` or `sink`, and it is always entity- or
client-centric. `source` is what the entity or client emits, and
`sink` is what it receives.

Track names are format names. A track name pins a complete wire
format, and a changed layout is a new track name, never a version
byte. Subscription is the format negotiation. A client subscribes to
the format it wants, and there is no catalog and no in-band
negotiation. New formats are additive, new names that break nothing.

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
format). Formats carry no authorization dimension. A connection that
may subscribe an entity's sink may take it in any offered format.
There is one carve-out. A profile MAY define entities with no sink at
all, as the base profile does for its source-only head-frame entities
(base profile §6, `frame`). A subscription to a sink that does not
exist is refused, and the offering obligation applies only to
entities that have sinks.

### 3.1 Subscribe and announce authorization

Authorization is enforced at one point, against the binding
established at connection (§4).

| Track | May subscribe |
|---|---|
| `(space, "entity", e, "sink")` (any format) | the connection owning e, with further grants as profile policy |
| `(space, "client", c, "sink")` / `state` | the connection with client id c only |
| `(space, "presence")` | any connected client |
| `(space, "entity", e, "source")` | the space only |
| `(space, "client", c, "source")` | the space only |

Announce mirrors subscribe. A client MAY announce only its own
`source` namespaces. A subscribe or announce whose namespace tuple
falls outside the connection's binding MUST be rejected.

> *Note (privacy): subscribing to an entity's sink is hearing the
> space through that entity's ears. The base profile grants this
> beyond the owner to the `admin` and `producer` roles. Deployments
> should surface this fact to users.*

### 3.2 Sink lifecycle

Sink formats are independent tracks, and while the entity lives
subscription is the only lifecycle signal. The server renders and
encodes an entity's sink only while at least one subscription to any
of its formats is active. A new subscription joins at live, with no
history. Concurrent formats are permitted, and each pays its own
encode. Switching formats is subscribe-new-then-unsubscribe-old,
with both flowing during the overlap. Any limits on concurrency are
deployment policy. When an entity departs, its sink tracks end with
it, and the server closes any live subscription to them (MoQ
SUBSCRIBE_DONE, track ended).

> *Note (capability is exercised, not declared): an entity never
> declares whether it acts as a source or a sink. The Connection
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
session establishment, as a setup parameter on CLIENT_SETUP.

- The parameter key is `0x1A5A1` (odd, so the value is
  length-prefixed bytes).
- The value is the UTF-8 JSON of the Connection Config
  (`schemata/connection.schema.json`):

```
{ "client_id": <identifier>,
  "ticket":    <JWT string>,        (optional)
  "entities":  [Entity, ...],       (see entity.schema.json)
  "setups":    [Setup, ...] }       (see setup.schema.json; requires ticket)
```

At least one of `entities` and `setups` MUST be present, and both MAY
be. The server authenticates the session from this one message,
derives every legal track tuple from the config's client and entity
ids, and enforces §3.1 against that binding thereafter. On success
the server completes SERVER_SETUP. On failure it terminates the
session (§4.6) without completing the handshake. This is identical
over WebTransport and raw QUIC.

### 4.2 Entities and Setups

An entity comes to exist on a connection in one of two ways.

- **`setups` instantiate ticket-defined entities.** A Setup
  `{id, attrs, initial-pose}` references, by id, an entity definition
  inside the signed ticket. The ticket carries the
  authorship-guaranteed definition, and the setup supplies the
  runtime-only parts: an initial pose that overrides the ticket
  definition's default, and unsigned attrs. Every `setup.id` MUST
  reference an entity in the ticket. Unknown or duplicate ids reject
  the connection. Instantiating a subset is permitted.
- **`entities` are ad-hoc definitions** carried directly in the
  config. They are admitted when the Space is unticketed, or when the
  ticket authorizes them (base profile: the `admin` and `presenter`
  roles).

The duplicate-id rules are as follows, noting that a setup's id
necessarily also appears in the ticket's entities block, where the
reference is the instantiation mechanism, not a collision. No id may
appear twice within the setups list. No id may appear twice within
the entities list. No ad-hoc entity id may equal any ticket-defined
id, instantiated or not, because ad-hoc entities cannot shadow
signed definitions. Any violation rejects the connection. A
duplicate id within the ticket's own entities block is an issuer
error and rejects the ticket itself (`0x1A5A01`).

Zero-entity connections are legal (`entities: []`). This is the
observer and console pattern. Such a connection may subscribe
`presence`, its own state sink, and whatever sinks its grants allow.

An entity definition (`schemata/entity.schema.json`) carries a
required `id` and a required human-readable `name` (non-unique,
surfaced to other participants, base profile §6). Its remaining
fields are optional. The schema deliberately spans both layers.
`id`, `attrs`, `initial-pose` and the two declarations below are
core. `name`, `priority`, `heard-in`/`hears`, `frame`, `dof` and
`render` are base-profile vocabulary whose semantics live in the
profile (§6). One schema carries both because the base profile is
not optional (§1.1).

**Quality and redundancy declarations.** An entity definition MAY
carry two declarations about its audio path, meaning the stream it
publishes and the sink track rendered for it. Each direction has a
jitter buffer at its receiving end, and one declaration configures
both. Both fields are fixed for the life of the connection. They
exist so each receiving side can provision buffering before the
first packet rather than learn it from the first audible failure.

- **`quality`** is an integer 0–2, default 0, stating the path's
  latency-tolerance intent. Level 0 is interactive (minimal
  latency). Level 1 is resilient (moderate added latency for
  materially fewer artifacts). Level 2 is playback and broadcast
  (latency is unimportant, uninterrupted artifact-free delivery is).
  The level is intent, not parameters. Each receiver maps it onto
  its own buffering policy, and that mapping may be re-tuned without
  protocol change. The space applies it to its ingest buffer, and
  the client applies its own declaration to its playout buffer. The
  sink half never touches the wire, because the client authored the
  declaration and is the sink's receiver. A client MAY therefore
  depart from it locally, and an entity whose two directions
  genuinely differ needs no protocol support.
- **`redundancy`** is an integer 0–7, default 0, giving the maximum
  redundancy offset (§5.1) on this path. Zero means none. It bounds
  the sender in both directions, the entity's uplink emission and
  the space's emission on that entity's sink track. For the sink
  direction the declaration doubles as the client's request, which
  the space SHOULD honour within deployment policy. A redundant copy
  repairs a loss only if the receiver's buffer already spans the
  offset, so each receiving side provisions buffer depth from the
  declaration, the space for its ingest buffer and the client for
  its playout buffer. A sender MUST NOT emit an offset exceeding the
  declaration. Within it, redundancy remains a per-packet choice
  (§5.1).

The two compose (a redundancy-derived buffering floor and a
quality-derived one combine as their maximum) but are independent
inputs. Because these fields are part of the entity definition,
ticket-defined entities carry them authorship-guaranteed. Whether
ad-hoc entities may claim elevated quality or sink redundancy, which
cost the space per-entity buffer memory and egress bandwidth
respectively, is deployment policy.

### 4.3 Tickets

A **Ticket** is a signed JWT (`schemata/ticket.schema.json`). EdDSA
(Ed25519) is RECOMMENDED. The server MUST verify the following.

- The signature is verified against the Space's configured issuer
  key(s).
- The `aud` claim MUST equal the space id. This is what stops a
  ticket minted for one space being replayed into another.
- `exp`, `nbf` and `iat` are enforced when present. A ticket without
  `exp` never expires, and issuers should set it deliberately, as it
  is the only bound on reuse (§4.5).

The claims object is a deliberately limited, constrained profile of
JWT, held to the normative schema exactly. `iss`, `aud` and
`panaudia` are required. `exp`, `nbf` and `iat` are optional but
enforced when present. `aud` MUST be the single-string form, never
the JWT array form. Any other member, registered JWT claims such as
`sub`, `azp` and `jti` included, rejects the ticket. Tickets are
grants of capability, not descriptions of identity, and admitting a
claim the server does not consume would imply it has some function.
`jti` in particular is deliberately absent. Session exclusivity
rides the `client_id` capability (§4.4), so a per-token id would
have nothing to govern.

The `panaudia` claim is required. It carries `client_id` (which MUST
equal the config's `client_id`), optional `roles`, optional
`permissions` (three arrays of key prefixes: `set`, `clear`,
`read`), and optional `entities`. Role names are profile vocabulary,
and the core carries them opaquely. The ticket schema holds them to
the identifier grammar (§2) and nothing more, and refusing unknown
names is profile policy, applied at admission (base profile §2). The
connection's effective state access is the union of its roles' rules
and its ticket's permissions (§6.4).

### 4.4 Space mode, capabilities, and admission

A Space is ticketed or unticketed, exclusively, fixed at
configuration. In a ticketed Space every connection MUST present a
ticket. In an unticketed Space a config presenting a ticket MUST be
rejected (`0x1A5A08`). A client presenting a ticket believes it is
getting signed identity, and silently admitting it with claim-based
identity would be a quiet degradation. There is no mixed mode.

Ids are capabilities, and two facts about a capability are distinct.
**Title** is the right to be a client id and to instantiate its
entity ids. In a ticketed Space, title is granted by the signed
ticket from `nbf` to `exp`, independent of any connection. Identity
is the signed `client_id` claim, not the JWT. A renewed ticket
bearing the same `client_id` is the same capability, and entity
capabilities are subordinate to the client capability that carries
them. **Exercise** is a connection currently acting under a
capability, and there is at most one exercise per capability at a
time. A zero-entity connection exercises its client capability like
any other.

The admission collision rules follow from one principle. Contests
over the same capability resolve to the newest exercise, and
collisions between different capabilities refuse the newcomer.

- **Ticketed, same `client_id` live.** The newest exercise wins, and
  the old connection is superseded: severed wholesale and replaced.
  The returning principal never waits on the server detecting the
  old session's death. (Two devices sharing one ticket therefore do
  not get two sessions. The multi-device story is more tickets.)
- **Ticketed, entity id collides with a live entity the connection
  does not own**, whether another client's or one no live connection
  accounts for. The connection is rejected (`0x1A5A04`). Two
  distinct capabilities naming one id is an issuer coordination
  error. Neither outranks the other, and a bystander is never
  severed on entity grounds. An unattributable live marker is an
  embedder or issuer anomaly, and refusal surfaces it rather than
  masking it by silent claim.
- **Unticketed.** Any collision, client id or entity id, resolves
  newest-wins, and the incumbent connection is superseded wholesale.
  Without title, reclaim and hijack are indistinguishable by
  construction. The rule is chosen for the development restart loop,
  and the mode's fragility is accepted (§8).

Unticketed mode is a development mode. Identity is possession-based
with no authorship guarantee, intended for development, demos and
closed networks. An unticketed config carries `entities` directly
(no `setups`, which require a ticket).

Any state cleanup owed by a superseded connection MUST be completed
or cancelled before the replacement connection's admission writes
are applied.

The authority for exercised ids is the Space's state serialization
point, and the core defines no wire surface for the records. The
base profile materializes them as state keys, `lasa.client.{id}` and
the entity existence markers, whose values name their owning client
(base profile §§3, 6). They are adjudicated atomically with the
admission writes, capacity included, so collision and capacity
refusals issue from that one point. The moderation-block refusal
(`0x1A5A06`) is the exception. It runs earlier, after ticket
verification and before adjudication (base profile §5), so a blocked
id's reconnect can never reach supersession.

### 4.5 Reconnect

"Reconnect" is not a protocol concept. Every connection is a fresh
connection presenting a full Connection Config. In capability terms
(§4.4), a reconnect is a re-exercise of the same capabilities,
superseding any half-open predecessor. Continuity exists in exactly
one place, where it is explicitly designed: the state cursor (§6.6).
Media tracks have none, because all per-track ids and receiver state
are scoped to the (MoQ session, track) instance (§5.2). A ticket
remains valid for reconnection until `exp`. Title outlives every
connection that exercises it.

### 4.6 Rejection and error codes

Rejection is a deliberate close of the established session (a QUIC
CONNECTION_CLOSE application error, or a WebTransport session
close), so the client always receives a code and a reason string on
both transports. Codes fit in 32 bits (the WebTransport bound) and
sit in a LASA range distinct from MoQ's own session-terminate codes.
Correctness MUST ride the code alone. Reason phrases are advisory,
may be truncated or dropped, and MUST NOT require parsing.

| Code | Meaning |
|---|---|
| `0x1A5A00` | invalid Connection Config: a schema violation, a §4.2 rule violation, or the setup parameter missing entirely. Also the fallback when admission fails with no more specific code |
| `0x1A5A01` | invalid ticket (signature, expired, not yet valid, wrong audience, duplicate entity definitions, unknown role name where the profile refuses them), or no ticket presented to a ticketed space |
| `0x1A5A02` | `client_id` does not match the ticket claim |
| `0x1A5A03` | *retired 2026-08-13*, MUST NOT be reused (formerly the unticketed id collision, which now resolves by supersession, §4.4) |
| `0x1A5A04` | entity id collides with a live entity the connection does not own (ticketed, §4.4) |
| `0x1A5A05` | ad-hoc entities not permitted for this ticket |
| `0x1A5A06` | blocked |
| `0x1A5A07` | space at capacity |
| `0x1A5A08` | ticket presented to an unticketed space (§4.4) |
| `0x1A5A09` | space shutting down: the server is stopping deliberately; the client MAY reconnect later |
| `0x1A5A0A` | superseded by a newer connection exercising the same client id (§4.4) |

The blocked rejection's reason string SHOULD carry the block expiry
timestamp.

The same codes close an **established** session when the server ends
it deliberately: a live block (`0x1A5A06`), a supersession
(`0x1A5A0A`), a shutdown (`0x1A5A09`). A session that ends for any
other reason (transport failure, protocol error) carries no LASA code.

## 5. Audio formats

### 5.1 Common structure

**Sequence ids.** Sequence ids are MoQ ids, one group per frame.
There is no seq field in any payload. The group-id increments by
exactly one per packet and the object-id is always 0, so the
group-id is the frame sequence number. There are no gaps. Pose-only
uplink packets increment too, and sink tracks are continuous, 200
packets per second regardless of audio content. This is
load-bearing. The redundancy offset (below) is denominated in these
ids, so `offset = 3` MUST mean exactly 15 ms.

**Byte order.** Every fixed-width field in this specification
(length prefixes, presence indexes, timestamps, and the pose
components below) is little-endian. QUIC varints keep their RFC 9000
encoding.

**The flags byte.** Every audio packet begins with one flags byte.
Bit positions are shared across formats, and not every bit is
meaningful in every format.

| Bit | Meaning | Applies to |
|---|---|---|
| 0 | pose present | `mono-object` |
| 1 | audio present | `mono-object` |
| 2 | redundancy present | all |
| 3–5 | redundancy offset (1–7) | all |
| 6 | ts present | sink formats |
| 7 | reserved, MBZ | — |

Reserved bits MUST be zero, and a receiver MUST discard packets with
unknown bits set (they are not this format). The redundancy-offset
bits 3–5 are meaningful only when bit 2 is set and are MBZ when it
is clear. A packet with bit 2 clear and a nonzero offset falls under
the same discard rule. The inverse, bit 2 set with a zero offset, is
malformed rather than unknown: it is this format but
self-contradictory, since offsets are 1–7 and the redundant copy
would be unlocatable. A receiver discards the packet.

**Pose encoding.** A pose is 18 bytes, byte-for-byte identical
wherever it appears on the wire: `x, y, z` as IEEE-754 f32 (metres,
with x forward, y left, z up), then `yaw, pitch, roll` as int16
scaled to ±π/32767 (positive yaw anticlockwise about z, positive
pitch upward, positive roll anticlockwise about the line of sight).

The pose in a `mono-object` packet is the freshest pose at packet
assembly time, sampled after the audio is encoded and deliberately
not synchronised with the audio samples in the same packet. This is
a stated property. It minimises pose staleness for the rotation
path, at the price of the source being rendered at a position
roughly one packetisation interval fresher than its audio, which is
imperceptible at human movement speeds.

**Redundancy framing.** One rule covers all formats. When the
redundancy bit is set the packet carries two audio payloads, the
current frame and a full copy of the frame `offset` packets ago
(offset 1–7 from the flag bits, burst cover up to 35 ms). Neither an
elementary Opus packet nor a multistream packet is self-delimiting,
so a u16 length prefix on the primary payload separates the two.
When the redundancy bit is clear there is no length prefix. The
referenced packet MUST be audio-bearing. The format carries the
capability only. Whether redundancy is on is the sender's per-packet
choice, bounded by the entity's declared maximum offset (§4.2), and
any adaptation policy within that bound is profile vocabulary over
state, not format surface. On sink tracks the space is the sender,
and the entity's declaration is both the request for sink redundancy
and the bound on it (§4.2).

> *Note: the offset converts loss into latency at `offset × 5 ms`
> and is only usable if the receiver's jitter buffer spans it. That
> is why the intended maximum is declared up front (§4.2): the
> receiver provisions buffer depth from the declaration instead of
> discovering the need at the first unrepaired loss. An offset
> beyond the declared maximum is a sender violation. The packet's
> primary payload remains valid, but the redundant copy cannot be
> assumed usable.*

**Timestamps.** The uplink carries none, deliberately. The server
re-times everything at the mix, and a source timestamp would wrongly
suggest participation in presentation timing. Sink formats carry
occasional timestamps, not per-packet ones. Frame spacing is exactly
5 ms on the server's clock, so a single (seq, ts) anchor extends to
every frame: `ts(seq) = anchor_ts + (seq − anchor_seq) × 5 ms`. When
the ts-present bit is set the packet carries a u64 unix-epoch
microseconds timestamp naming the first sample of the accompanying
frame. The emission cadence is on stream start and on the order of
once per second. The anchor is per track instance and re-established
after reconnect. These timestamps are informational, and
presentation timing does not depend on them.

### 5.2 Id continuity: session-scoped, reconnect restarts at zero

Entity and client ids come from the Connection Config, so a
reconnecting client republishes the same track tuple, but the id
space does not carry over. A reconnect is a new track instance and
group-ids restart at 0. (Id arithmetic is time arithmetic only
within a contiguous run, and stitching across a wall-clock hole
would silently break the redundancy offset and freshness logic.) The
restarting instance is the publisher's. For uplinks that is the
client's connection, so a reconnect means a new instance and ids
from 0. For sink tracks it is the render itself (§3.2), whose ids
start at 0 when rendering starts and run continuously across
subscriber churn, so a subscription joining a live render attaches
at the current id, not 0. All per-track receiver state (latest-wins
ids, jitter buffers, redundancy history, ts anchors) is scoped to
the (MoQ session, track) instance and resets with it either way.
Nothing in the packet format signals a reconnect. The new MoQ
session is the signal.

### 5.3 Layouts

```
mono-object:  [flags][pose(18)?][len(2)?][opus][opus-dup?]
binaural:     [flags][ts(8)?]   [len(2)?][opus][opus-dup?]
ambi2/ambi3:  [flags][ts(8)?]   [len(2)?][opus-ms][opus-ms-dup?]
```

Everything after the flags byte is present iff its flag bit (or the
redundancy bit, for `len`) says so. Frame identity rides the MoQ
group-id, and there is no other header.

- A `mono-object` packet carries one 5 ms mono Opus frame (when
  audio is present) at 48 kHz, plus an optional pose. Pose-only
  packets are silence, and audio-only packets are pose decimation,
  both sender choices with zero format cost. A packet carrying
  neither (flags `0x00`) is likewise legal. It advances the sequence
  and says nothing else, keeping the no-gaps rule of §5.1
  unconditional.
- A `binaural` packet carries one 5 ms stereo Opus frame, the
  binaural rendering of the space as heard by the entity.
- In `ambi2` and `ambi3` the audio section is exactly one Opus
  multistream packet (RFC 7845 framing: per-stream packets
  concatenated, all but the last self-delimited per RFC 6716
  App. B), produced and consumed whole by the standard multistream
  API. Format-in-name is the channel-configuration negotiation.
  `ambi2` is 9 channels as 9 uncoupled mono streams, and `ambi3` is
  16 channels as 16 uncoupled mono streams (`coupled = 0`, identity
  mapping), ACN order, SN3D normalisation.

## 6. State

### 6.1 The store

The state store is a distributed key/value store with two
operations. `set` assigns a value to a key, and `clear` removes it.
Keys follow the grammar of §2. Values are opaque binary, at most
64 KiB. This is a coordination store, not a blob store. The core
attaches no meaning to any key, and `lasa.`-prefixed keys belong to
the base profile.

Every operation is assigned a **sequence id** by the store, from a
run of monotonic incrementing integers. Operations MAY be grouped. A
**group** is a hint that its operations should be applied together,
and it provides no consistency guarantee. A group's op count is
capped by a server constant, well below any delivery window. Without
a cap, one over-large group is a forced-resync primitive against
every subscriber. An over-cap group is silently dropped whole, never
partially applied. Dotted keys suggest nesting but the store is
flat. Removing a parent removes nothing, and a leaf can share a name
with another key's parent.

Clients track the sequence id per key and MUST ignore any operation
whose id is older than or equal to the current one for that key.

### 6.2 Writes and the echo

A client writes by publishing ops on its state `source` track. Ops
carry no seq upstream, and the server assigns one on acceptance. The
only acknowledgement is the echo. An accepted op comes back, with
its seq, on the client's own state sink subscription. All rejections
are silent, and no echo means it didn't happen. The silent-drop
causes are these.

- The op fails to parse.
- The key violates the §2 grammar or the 512-byte cap.
- The value exceeds 64 KiB.
- The group exceeds the op-count cap (§6.1).
- The write is denied by role or permission rules.
- The key matches none of the connection's write grants.
- The op arrives after the connection's disconnect sweep has begun.
  A superseded connection's in-flight writes are dropped, never
  applied on the successor's behalf.

Two shapes are accepted yet never echoed. This is delivery
filtering, not rejection. A write under a `set` or `clear` grant
without the matching `read` grant is applied, but its echo is
dropped by the read filter (an ack-less grant shape). A write to a
key outside the connection's subscribed prefixes produces no echo on
that connection. Clients SHOULD subscribe to what they write.
Acknowledgement is per-key state confirmation, not per-op. A write
made before subscribing is confirmed by the compacted catch-up, or,
if superseded, never.

### 6.3 Subscription

A client holds exactly one state subscription: its per-client sink
track, filtered by a set of key prefixes fixed at subscribe time.
Scope is the union of the prefixes (overlap is benign, and a key
matching several prefixes is delivered once). The set containing the
empty prefix (`[""]`) subscribes to everything. An empty set is a
protocol error. Changing the set means unsubscribing and
re-subscribing. Subscription filtering is plain string prefix. `foo`
matches `foobar` as well as `foo.bar`, because keys are strings and
the dotted structure is convention.

### 6.4 Access rules

The server filters both directions per connection through one merged
rule set: the union of the connection's role-derived rules and its
ticket's `permissions` grants. (Roles are compact aliases over the
same machinery, and role definitions live in profiles.) Unlike
subscription filters, access rules match exact key or dot-extended
prefix. A scope `p` matches key `k` iff `k = p′` or `k` begins with
`p′ + "."`, where `p′` is `p` without any trailing dot.
(Plain-prefix matching here would either hide a bare key from its
own dotted scope or leak sibling ids.) The empty scope is the
universal grant. It matches every key, mirroring the subscription
side, where the empty prefix subscribes to everything (§6.3).

A scope may contain `*` segments, each matching exactly one
identifier segment. `lasa.entity.*.dof` matches `lasa.entity.a1.dof`
(and its dot-extensions) but not `lasa.entity.a1.render.dof`. Since
identifiers never contain dots (§2), segment matching is
unambiguous. Wildcards exist for role expansion, where profile role
tables grant key suffixes across all entities, present and future
(base profile §2). They are legal only in server-side expanded
rules, and ticket `permissions` prefixes remain literal
(`ticket.schema.json` is unchanged).

### 6.5 Delivery: delta, snapshot, frontier

On subscribe the server answers with exactly one of two catch-up
shapes.

- A **delta** carries, for every key in scope whose latest op has a
  seq greater than the presented cursor, the latest op only (a `set`
  if the key lives, a `clear` if deletion was most recent).
  Compaction means intermediate history is never delivered and
  groups are not preserved.
- A **snapshot** replaces everything. The client discards its local
  store across its subscribed prefixes and adopts the snapshot
  (per-key seqs included, and snapshots never contain tombstones).

The server MUST snapshot when it cannot prove a delta complete and
MAY snapshot whenever else (for example when the delta would be
larger). Every response closes with a **frontier** message, which
promises "you will receive nothing further at or below X on this
path". The promise is inclusive, so a cursor equal to the cut is
fully caught up. The frontier carries the explicit cut
`(epoch, seq)`, never derived from content. Frontiers are also
emitted periodically during live operation and are how the client
learns the epoch. Frontiers require ordered reliable transport, so
state can never migrate to datagrams without revisiting this design.

Delivery is live-first. Live ops flow immediately, the catch-up
payload streams lazily alongside, and the server never coordinates
the two. The client ordering rules are these.

1. Apply live ops under per-key dedup.
2. Apply a catch-up entry only if its seq exceeds the local per-key
   seq.
3. When a snapshot completes, delete every in-scope local key absent
   from it whose local seq is at or below the cut H. This rule is
   snapshot-only. Absence from a delta means unchanged.

> *Note: during the catch-up window a live `clear` can be followed
> by a late catch-up chunk carrying the same key at an older seq,
> and naive application resurrects the deleted key. Two strategies
> conform. Buffer-and-replay freezes live application until the
> frontier, then replays under dedup. It is hazard-free, with the
> view frozen for the window. Apply-live with transient tombstones
> records every live clear during the window, including for absent
> keys, and drops the records at the frontier. It is fully live, and
> subtler. The wire exchange is identical, and the choice is out of
> protocol scope.*

### 6.6 Cursor, epoch, and the reconnect gate

The client persists one cursor, **(epoch, seq, set-hash)**,
atomically, and never ahead of its persisted store (a client that
does not persist its store MUST NOT persist a cursor). No cursor may
be persisted until the subscription's first catch-up frontier has
been applied. Before that point the local store is not at any
provable cut, whatever the delivery strategy. The **epoch** is 8
opaque bytes, compared for equality only. The server mints a new
epoch whenever seq-space continuity is unprovable (crash, restore,
migration). The **set-hash** is SHA-256 over the canonical
prefix-set encoding (deduplicated, sorted bytewise, each
length-prefixed with a minimal varint, §6.7), stamped when the
cursor is earned. It is a bug-catcher, not a security gate. When
frontiers arrive on both delivery paths, persist the maximum seq per
epoch.

Tombstones are retained for a server-configured wall-clock TTL
(default 6 hours) and then flushed. The **watermark** is the highest
seq of any discarded tombstone. Neither TTL nor watermark is
advertised. Clients plan around nothing and must handle either
response.

The gate is server-enforced, in this order.

1. Epoch mismatch: snapshot.
2. Set-hash mismatch: snapshot.
3. Seq below the watermark: snapshot.
4. The server cannot prove the connection's effective visibility
   unchanged since the cursor was earned: snapshot.
5. Seq ahead of the server's head at the same epoch: snapshot
   (corrupt client state).
6. Otherwise: delta, or snapshot at the server's discretion.

A fresh client presents nothing and lands on snapshot with no
special case.

**Visibility changes.** Gate 4 states an obligation, not a
mechanism. It must cover changes made while the identity was
disconnected, role-definition changes affecting disconnected
identities, and reconnects bearing a different ticket. Tracking a
per-identity visibility-change seq is one implementation, and
comparing a hash of the connection's effective visibility rules is
another. Mid-session changes are applied by terminating the state
subscription via native MoQ termination. The session and all audio
tracks stay up, and the client re-subscribes and hits the gate.
Global rule-set changes MAY be an epoch bump plus termination of all
state subscriptions. Under this specification the only mid-session
trigger is a server-side rule or configuration change, because a
connection's own roles and permissions ride its ticket, presented
once.

### 6.7 Wire encoding

The subscribe message is MoQ SUBSCRIBE parameters: one KVP with key
`0x1A5A3` (odd, so length-prefixed bytes), on the SUBSCRIBE to the
state sink track.

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
MUST be minimal, in the canonical form because a non-minimal length
encoding would hash differently on each side, and in the cursor
block for uniformity, so that one rule covers the whole KVP. A
server MUST reject a subscribe whose prefix set is not canonical
(duplicated, unsorted, or non-minimal) or whose cursor seq varint is
non-minimal. A fresh client sends flags = 0 and omits the cursor
block.

Messages ride one per MoQ object as `[type u8][body]`, with QUIC
varints throughout.

```
set       [1][key-len][key][value-len][value]([seq])
clear     [2][key-len][key]([seq])
group     [3][count][ops...]        each op a full [type][body]; groups do not nest; count capped (§6.1)
frontier  [4][epoch 8][seq]
begin     [5][kind u8]              1 = delta, 2 = snapshot
```

`seq` is downstream-only, and upstream ops carry none. The same type
bytes therefore parse differently per direction, so implementations
MUST select the layout by track direction and not share a
direction-blind parser. `begin` opens a catch-up payload (ordinary
`set` and `clear` messages, with snapshot entries being `set`s that
carry per-key seqs) closed by a `frontier` carrying the cut. The
server SHOULD preserve a written group's boundaries in live delivery
and never preserves them in catch-up. Clients MUST NOT depend on
group boundaries.

**MoQ mapping.** On the sink track, each subscription instance is a
fresh MoQ group. Within it, subgroup 0 carries live ops (and the
periodic frontiers), and subgroup 1 carries the catch-up payload
(`begin` … `frontier`). Each subgroup is its own QUIC stream.
Ordering is guaranteed within each path, which is all the per-path
frontier promise needs, and backpressure on a large snapshot cannot
stall live ops. Object-ids increment monotonically within each
subgroup. The source track is one group, one subgroup, ordered ops.
This mapping is normative. The lane is identified by its subgroup
id, and clients MAY depend on that (the reference clients do).
Subgroups are core MoQ, and LASA defines no alternative mapping.

## 7. Presence

`(space, "presence")` is the one shared, cacheable broadcast:
datagrams published by the space, subscribable by any connected
client. It is a lossy visualisation feed and is self-contained.
Authoritative entity existence lives in state, and presence never
depends on it. MoQ ids are group-per-packet, object 0, incrementing
by one per datagram.

There are two packet kinds. Flags bit 0 marks a keyframe, and the
other bits are reserved-MBZ with the discard rule of §5.1.

```
delta:     [flags u8][gen u8] [record]*
           record  = [index u16][pose 18][loudness u8]

keyframe:  [flags u8][gen u8] [first u16] [krecord]*
           krecord = [kflags u8][id-len u8][id utf8][pose 18][loudness u8]
```

`kflags` bit 0 is the **head-frame** flag. It marks a record whose
pose (and subsequent delta poses for this index) is interpreted
relative to each listener's head rather than the shared space (see
the base profile's `frame` key). Bits 1–7 are reserved, MBZ, with
the discard rule of §5.1. The flag appears in keyframes only.
Frame-ness is fixed at connection time and membership changes bump
the generation, so the roster always carries it and delta records
stay unchanged.

Records parse to the end of the datagram. Pose is the §5.1 encoding.
**Loudness** is a u8. Zero is digital full scale, each step is
−0.5 dB down to 254 at −127 dBFS, and 255 is silence (below
−127 dBFS). It is the entity's source loudness post-gain,
pre-attenuation (attenuation is per-listener and cannot appear in a
shared feed).

- Indexes are defined by keyframes. Entities are listed in index
  order, so the mapping is implicit and position is the assignment.
  A keyframe is one or more self-contained **chunks**. `first` is
  the index of the chunk's first record, and subsequent records
  increment. A lost chunk costs only those indexes until the next
  keyframe round.
- The generation byte increments only when membership changes. Every
  delta carries it, and a delta whose gen does not match the
  client's last keyframe MUST be discarded. The server SHOULD emit a
  keyframe immediately on membership change. All per-index tracking
  state resets on gen change, and the pair (gen, index) is the
  lookup key.
- Per-entity records are latest-wins by group-id. Apply a record
  only if its packet's group-id exceeds the last applied for that
  index.
- Departure is keyframe absence. Deltas never signal departure.
- The index is a server-assigned alias scoped to this track alone.
  It appears in no key, no namespace, and no other track, may change
  across generations, and is never authoritative. Entity identity is
  the id string, everywhere.

Non-normative rates: deltas at roughly 10–30 Hz, changed entities
only, and keyframes on the order of once per second plus on
membership change.

Which entities appear is space policy. Head-frame entities are
included, distinguished by the `kflags` bit above.

## 8. Security considerations

- Ticket verification requirements are in §4.3. The `aud` check is
  the replay boundary between spaces.
- Unticketed mode is fragile by design. Identity is possession-based
  and collisions resolve newest-wins (§4.4), so id takeover is
  possible by construction. Nothing prevents impersonation across
  connect boundaries, and ad-hoc entities are admitted from anyone.
  It is a development mode. Do not run it on open networks.
- In ticketed mode a stolen ticket can supersede the legitimate
  holder's session, but a stolen ticket already grants full
  impersonation, so supersession adds no new capability.
- Client ids and entity ownership are deliberately visible. The
  entity marker's value names its owning client (base profile §6),
  readable by every participant, including the linkage that several
  entities share one owner. Client ids are names, never secrets and
  never capabilities. The ticket is the capability.
- A client that saw a key cannot be made to unsee it. Visibility
  narrowing is hygiene, not security.
- The sink-subscription privacy property is stated in §3.1.
- Rejected state writes are silent (§6.2). Clients should not offer
  UI for actions their grants forbid, confining silent failure to
  races.

## 9. Wire constants

| Constant | Value |
|---|---|
| Connection Config SETUP parameter key | `0x1A5A1` |
| State subscribe SUBSCRIBE parameter key | `0x1A5A3` |
| Session error codes | `0x1A5A00`–`0x1A5A08` (§4.6, with `0x1A5A03` retired and never reused) |
| Max state key length | 512 bytes |
| Max state value length | 65 536 bytes |
| Max ops per group | server constant (§6.1), reference implementation default 64 |
| Fixed-width byte order | little-endian (§5.1), varints per RFC 9000 |
| State epoch width | 8 bytes, opaque |
| Audio frame | Opus, 48 kHz, 5 ms, one frame per packet |
