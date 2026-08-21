# LASA Base Profile

*Draft 1, 2026-07-27. Companion to `lasa-core.md`. This profile
claims the reserved `lasa.` key prefix and defines the standard key
vocabulary, the role definitions, and key lifetime semantics. The
three are one unit: a vocabulary without who-may-write is unusable,
and roles without a vocabulary have nothing to protect.*

This profile is not optional. Every conforming Space implements it
(`lasa-core.md` §1.1), because the split between core and profile is
machinery versus vocabulary, not a feature boundary. The sync
machinery itself stays content-blind. Nothing below is required for
it to operate, and the reference server's mixer reading these keys
is a profile consumer, not core machinery. Entity definitions carry
this profile's fields directly in the core's entity schema
(`schemata/entity.schema.json`: `name`, `priority`,
`heard-in`/`hears`, `frame`, `dof`, `render`, core §4.2).

## 1. Value conventions

All values under `lasa.` are UTF-8 and one of three types, Boolean,
String or Number, encoded per JSON conventions. (The core's 64 KiB
cap is far above anything here.)

Attrs flatten to dotted paths, objects only. A nested attrs object
in a Connection Config or Ticket flattens to one key per leaf
(`attrs.a.b = v`). Leaves are Boolean, String or Number. Arrays are
invalid anywhere in attrs, because index keys would import ordering
semantics and resize bugs the flat store cannot honour. An
application needing a list encodes a JSON array inside a single
String value. Attribute names follow the core identifier grammar.

## 2. Roles

There are five roles: `admin`, `presenter`, `moderator`, `producer`
and `audience`. Ticketed connections carry roles in the ticket
(`panaudia.roles`). A ticket with an absent or empty roles claim,
and every unticketed connection, is granted exactly `audience`. The
core carries role names opaquely (`lasa-core.md` §4.3), and this
profile is where they gain meaning and where unknown ones are
refused. A ticket bearing a role name outside this vocabulary
rejects the connection at admission as an invalid ticket
(`lasa-core.md` §4.6, `0x1A5A01`).

Roles are compact aliases over the core's prefix-rule machinery
(`lasa-core.md` §6.4). A connection's effective access is the union
of its roles' rules below and its ticket's explicit `permissions`
grants. Permissions may target any prefix, including `lasa.`, which
gives a fine-grained grant without the full role. Value-level rules
(allowed values, §5's admin protection) apply regardless of how
write access was obtained, and they are consumer-side. The store is
profile-blind (`lasa-core.md` §6.1), so a write outside a key's
documented values is accepted, echoed and delivered like any other,
and it simply has no effect. Consumers ignore it, and whatever value
last took effect stands. Scopes below match exact key or
dot-extended prefix, per the core, and the any-entity grants expand
to `*`-segment scopes (`lasa-core.md` §6.4).

### audience

An `audience` connection may write the following keys when
`{entity-id}` is one of its own entities.

- `lasa.entity.{entity-id}.attrs.{attr-name}` (set and clear)
- `lasa.entity.{entity-id}.mute.{other-entity-id}` (set and clear)
- `lasa.entity.{entity-id}.solo.{other-entity-id}` (set and clear)

It reads `lasa.entity.` and `lasa.space.`, the participant-facing
vocabulary. `lasa.client.` is deliberately excluded. The claim
records (§6) are server-consumed, and clients learn of participants
through entity keys and presence. (Nothing here is secret, client
ids included, core §8. The entity marker's value names its owner,
names and attrs exist to be shown to other participants, and a
muted user should be able to see that they are muted.)

### presenter

A `presenter` has everything `audience` has, plus the connect-time
admission privilege: a presenter ticket may carry ad-hoc entity
definitions directly in the Connection Config (`lasa-core.md` §4.2).

### moderator

A `moderator` has everything `audience` has, plus write access to
the following.

- `lasa.entity.{entity-id}.dof` (set and clear, any entity)
- `lasa.space.entity.muted.{entity-id}` (set and clear)
- `lasa.space.client.blocked.{client-id}` (set and clear)

### producer

A `producer` has everything `audience` has, plus write access to
the following.

- `lasa.entity.{entity-id}.heard-in.{channel-name}` (set and clear)
- `lasa.entity.{entity-id}.hears.{channel-name}` (set and clear)
- `lasa.entity.{entity-id}.priority` (set and clear)
- `lasa.entity.{entity-id}.render.gain` (set and clear)
- `lasa.entity.{entity-id}.render.attenuation` (set and clear)
- `lasa.entity.{entity-id}.render.size` (set and clear)
- `lasa.entity.{entity-id}.render.directivity` (set and clear)
- `lasa.space.role.blocked.{role-name}` (set and clear)
- `lasa.space.channel.muted.{channel-name}` (set and clear)
- `lasa.space.channel.gain.{channel-name}` (set and clear)
- `lasa.space.channel.attenuation.{channel-name}` (set and clear)

Producer writes target any entity. This is the mixing-desk role, and
unlike `audience` its entity grants are not limited to the
connection's own entities. For the `render.*` keys and the channel
overrides, `clear` reverts to the documented default (absence means
the default).

A producer may also subscribe to any entity's sink tracks
("listening through e's ears", the privacy statement in
`lasa-core.md` §3.1), because monitoring what listeners hear is part
of the desk. Sink formats carry no role gate (`lasa-core.md` §3).
Any connection that may subscribe a sink may take it in any format
the space offers.

### admin

An `admin` has the union of all four roles (which carries producer's
subscribe-to-any-sink grant), plus one grant that belongs to no
component role: admin is unblockable (§5).

### Application space

For non-`lasa.` keys, ticketed connections have no
application-space access unless granted via ticket `permissions`.
Unticketed spaces are unrestricted over application space,
consistent with the fragile mode.

## 3. Key lifetime

Keys are either **connection-scoped** or **space-scoped**, as marked
in §6. Connection-scoped keys are swept by the server on disconnect
with ordinary `clear` ops, seq-assigned, grouped, compacted and
tombstoned like any other write, and nothing on the wire marks a
sweep. The server SHOULD clear the entity existence marker first, or
in the same group as the rest of the sweep, so that the
ignore-when-absent rule covers readers mid-sweep. Sweeps run at
disconnect, immediately, and there is no grace period. The claim
records, meaning the client key and the entity existence markers
(§6), are the authority for exercised ids (core §4.4), so their
presence must equal connectedness. On supersession (core §4.4) the
core's ordering rule applies: the superseded connection's sweep
completes or is cancelled before the replacement connection's
admission writes.

Space-scoped keys survive reconnects, which is what makes moderation
launder-proof. Personal mutes reset with the connection, but a
space-level mute or block cannot be escaped by reconnecting.

A known weakness is recorded here. Space-scoped keys promise
"forever" from a store whose loss is an anticipated event (the
core's epochs exist for it), so a restart or a restore-to-earlier
can silently un-kick. Servers SHOULD durably persist `lasa.space.*`
across restarts.

## 4. Channels

Sources and sinks are joined by named channels. Source S is audible
to sink K iff they share at least one channel that is not muted. An
entity's memberships are directional. `heard-in` lists the channels
in which its source is heard, and `hears` lists the channels its
sink listens to. An entity whose config omits `heard-in` (or
`hears`), or supplies an empty array, gets `["main"]`. Membership is
resolved once, at admission, and every admitted entity holds at
least one channel. A config cannot declare an entity into silence,
because a source that emits signal is audible somewhere.

Channels carry optional render overrides, resolved per
(source, sink) pair. Muted channels contribute nothing to
resolution. Across the pair's shared unmuted channels, the highest
channel gain wins (unset counting as the identity) and the lowest
set channel attenuation wins. The winners then meet the source's own
`render.*` values as follows.

- Gain composes multiplicatively. The effective gain is the source's
  `render.gain` times the winning channel gain (gains are amplitude
  multipliers throughout, §6). An unset channel gain is the identity
  (1.0), so the default is a no-op and entity balance within a
  channel survives a channel-wide boost or cut. When set and unset
  gains mix across the pair's shared unmuted channels, the unset
  channels compete with their identity value, so a policy-free
  shared channel keeps the pair at unity and cannot be attenuated by
  another channel's fader (most audible wins). The composed value
  may exceed the 0–3.0 range of either key, and headroom policy is
  the renderer's.
- Attenuation overrides when set. If at least one shared unmuted
  channel sets `channel.attenuation`, the lowest such value replaces
  the source's `render.attenuation`. Otherwise the source's own
  value applies unchanged. Unset is the default and is not a number.
  A channel with no attenuation set imposes no distance-law policy.

Because membership is pairwise, effective gain and attenuation are
properties of the (source, sink) pair. The same source may
legitimately render louder, or under a different distance law, to
listeners in one channel than to listeners in another.

## 5. Moderation

`lasa.space.entity.muted.{entity-id}` silences an entity's source in
every render. It affects rendering only, never the connection.

The blocked keys do double duty, and there is no separate kick.
Setting `lasa.space.client.blocked.{client-id}` or
`lasa.space.role.blocked.{role-name}` (a value of 0 means forever,
and any other value means blocked until that unix-seconds timestamp)
terminates any live matching connection and gates admission until
expiry. A kick is a block with a short expiry. Enforcement is the
server acting as profile consumer. At admission the check runs after
ticket verification and before entity admission (`lasa-core.md`
§4.6, code `0x1A5A06`).

Admin is unblockable. A connection whose ticket bears `admin` is
immune to every block, `client.blocked` entries and role blocks
alike, including blocks naming its other roles. None refuses it at
admission or terminates it live. Setting
`lasa.space.role.blocked.admin` additionally has no effect at all.
Like any out-of-range value it is stored and delivered but ignored
by every consumer (§2). This guarantees in-protocol recovery from
any block state: an admin can always connect and clear.

## 6. Key vocabulary

### Connection-scoped

| Key | Values | Written by |
|---|---|---|
| `lasa.client.{id}` | `true` | server only |
| `lasa.entity.{id}` | String: owning client id | server only |
| `lasa.entity.{id}.name` | String | server, from config, immutable |
| `lasa.entity.{id}.attrs.{attr}` | any §1 type | config, then owner at runtime |
| `lasa.entity.{id}.signed-attrs.{attr}` | any §1 type | server, from ticket, immutable |
| `lasa.entity.{id}.heard-in.{channel}` | `true` | config, then `producer` |
| `lasa.entity.{id}.hears.{channel}` | `true` | config, then `producer` |
| `lasa.entity.{id}.priority` | `true` | config, then `producer` |
| `lasa.entity.{id}.render.gain` | Number 0.0–3.0 | config, then `producer` |
| `lasa.entity.{id}.render.attenuation` | Number 0.0–3.0 | config, then `producer` |
| `lasa.entity.{id}.render.size` | Number ≥ 0.0 | config, then `producer` |
| `lasa.entity.{id}.render.directivity` | Number 0.0–1.0 | config, then `producer` |
| `lasa.entity.{id}.frame` | `"world"` \| `"head"` | config only |
| `lasa.entity.{id}.dof` | 0 \| 3 \| 6 | config, then `moderator` |
| `lasa.entity.{id}.mute.{other-id}` | `true` | owner |
| `lasa.entity.{id}.solo.{other-id}` | `true` | owner |

### Space-scoped

| Key | Values | Written by |
|---|---|---|
| `lasa.space.entity.muted.{entity-id}` | `true` | `moderator` |
| `lasa.space.client.blocked.{client-id}` | Number ≥ 0 | `moderator` |
| `lasa.space.role.blocked.{role-name}` | Number ≥ 0 | `producer` |
| `lasa.space.channel.muted.{channel-name}` | `true` | `producer` |
| `lasa.space.channel.gain.{channel-name}` | Number 0–3.0, absent = 1.0 (identity) | `producer` |
| `lasa.space.channel.attenuation.{channel-name}` | Number 0–3.0, absent = no override | `producer` |

### Semantics

**`lasa.client.{id}`** is the client claim record, present iff a
connection is currently exercising this client id (core §4.4). The
server sets it at admission, before any entity group, and sweeps it
immediately at disconnect (§3). The value is `true`, deliberately
minimal. Consumers MUST NOT depend on it beyond presence, so it can
be extended later. It is in no role's read scope (§2). Server-side
consumers (entry points, the moderation gate) read it through
observers, and clients never receive it.

**`lasa.entity.{id}`** is the existence marker and entity claim
record, set and cleared by the server only, in response to
connection and departure. Its value is the owning client id, the
client whose connection instantiated the entity, which makes
ownership readable wherever the entity is (a deliberate visibility
choice, core §8, and consumers needing only existence treat the
value as opaque). All other keys under the entity's prefix MUST be
ignored while this is absent, which is what makes the departure
sweep safe mid-flight (§3).

**`name`** is a non-unique human-readable label, from the config or
ticket definition.

**`attrs.{attr}`** and **`signed-attrs.{attr}`** are arbitrary
values attached to the entity, not functionally used by LASA,
forwarded to other clients. `attrs` come from the config (or the
Setup) and are owner-writable at runtime. `signed-attrs` come from
inside the signed ticket, a different authorship guarantee, and are
immutable. Flattening is per §1.

**`heard-in.{channel}`** and **`hears.{channel}`** are the channels
in which this entity is heard and hears. Defaults are per §4,
applied at admission. The server materializes the effective
membership into these keys, writing an omitted or empty config list
as `main`, so under a live existence marker the present key set is
exactly the membership, and state carries no defaulting rule of its
own. A consumer reads these keys literally and never re-applies the
default. Admission always writes at least one. A set emptied
afterwards by a `producer` clearing its last key is a runtime
moderation act, and it leaves that entity inaudible.

**`priority`** marks this entity's source as important. Rendering
should make every effort to preserve its accuracy and quality. Do
not set it on everything, because that is self-defeating.

**`render.gain`** is the source gain multiplier. Absence means 1.0.
Gain multiplies amplitude (linear voltage gain, where 0.5 is −6 dB
and 2.0 is +6 dB, the conventional audio reading, pinned
2026-08-04). It composes multiplicatively with channel gain, per
pair (§4).

**`render.attenuation`** is the distance-attenuation exponent.
Absence means 2.0. A value of 0.0 keeps the source at the same level
everywhere, 1.0 attenuates linearly, 2.0 is natural inverse-square,
and 3.0 falls off faster, which is useful for whispers and private
communication. Fractional values are meaningful (2.3 can feel more
natural than 2.0 in virtual space). It may be replaced per pair by a
set channel attenuation (§4).

**`render.size`** is the source size in metres, and 0.0 (the
default) is a point source. How renderers represent size is up to
them.

**`render.directivity`** runs from 0.0 (the default,
omnidirectional) to 1.0 (maximally directional), and interpretation
is up to renderers.

**`frame`** is the reference frame of the entity's pose. `"world"`
(the default) is the shared space. `"head"` pins the source relative
to every listener's head, an aural HUD, an angel on the shoulder.
Head-frame entities appear on the presence track with the krecord
head-frame flag set (`lasa-core.md` §7), and their presence poses
are head-relative, not world positions. Two consequences were pinned
on 2026-08-05. Head-frame entities are source-only: they have no
world position, so their own sink tracks are refused (nothing
defines what they would hear). They are also absent from the
ambisonic sink formats, whose world-frame fields cannot carry a
per-listener source, and they render in `binaural` sinks only.

**`dof`** is the entity's degrees of freedom. Six (the default)
moves and turns freely, 3 turns in place, and 0 is fixed. `clear`
reverts to the default, 6. The space enforces dof at ingest. For
dof 3 the position components of arriving poses are ignored, and for
dof 0 the whole pose is ignored and the entity's pose is its
initial-pose. Presence reflects the enforced pose, never the sent
one.

**Initial pose** (config, not a state key). An entity definition's
`initial-pose` (`lasa-core.md` §4.2, coordinate conventions per
§5.1) positions the entity at join. Its pose is the initial-pose
until the first accepted packet pose supersedes it, and it remains
so for `dof` 0. The value is consumed at admission and never
materialized into state. Live position is presence's to carry, and a
state copy would be stale the moment the entity moved.

**`mute.{other}`** and **`solo.{other}`** are personal,
connection-scoped preferences of the owning entity. Mute is
bidirectional (pinned 2026-08-04). Muting an entity silences the
pair both ways: the muter stops hearing the muted and stops being
heard by them ("mute" understates this slightly, but it is what a
user muting someone wants). Solo silences all but the solo'd set,
for the soloing listener only, and it wins over personal mutes in
both directions, so a solo'd entity is audible to the soloing
listener even across a mute. The space moderation mute (§5) sits
above both, and no solo or mute state makes a space-muted source
audible. Both reset on reconnect (the client rewrites what it
knows).

**`lasa.space.entity.muted`**, **`client.blocked`**,
**`role.blocked`** and **`channel.*`** are per §§4 and 5.

## 7. Presence participation

All entities appear on the presence track. Head-frame entities carry
the krecord head-frame flag, and their poses are head-relative (§6,
`frame`). The loudness byte is source loudness post-`render.gain`,
pre-attenuation. Channel gains, being pair-dependent (§4), are
excluded, so presence carries one loudness per source, not one per
pair.
