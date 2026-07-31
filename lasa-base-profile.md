# LASA Base Profile

*Draft 1 — 2026-07-27. Companion to `lasa-core.md`. This profile
claims the reserved `lasa.` key prefix and defines the standard key
vocabulary, the role definitions, and key lifetime semantics — the
three are one unit: a vocabulary without who-may-write is unusable,
and roles without a vocabulary have nothing to protect.*

A deployment claims this profile by name, out-of-band; there is no
wire discovery. The core functions with an empty vocabulary — nothing
below is required for the sync machinery to operate, and the
reference server's mixer reading these keys is a profile consumer,
not core machinery.

## 1. Value conventions

All values under `lasa.` are UTF-8 and one of three types — Boolean,
String, Number — encoded per JSON conventions. (The core's 64 KiB cap
is far above anything here.)

**Attrs flatten to dotted paths, objects only.** A nested attrs
object in a Connection Config or Ticket flattens to one key per leaf
(`attrs.a.b = v`). Leaves are Boolean/String/Number; **arrays are
invalid anywhere in attrs** — index keys would import ordering
semantics and resize bugs the flat store cannot honour. An
application needing a list encodes a JSON array inside a single
String value. Attribute names follow the core identifier grammar.

## 2. Roles

Five roles: `admin`, `presenter`, `moderator`, `producer`,
`audience`. Ticketed connections carry roles in the ticket
(`panaudia.roles`); a ticket with an absent or empty roles claim, and
every unticketed connection, is granted exactly **`audience`**.

Roles are **compact aliases** over the core's prefix-rule machinery
(`lasa-core.md` §6.4): a connection's effective access is the union
of its roles' rules below and its ticket's explicit `permissions`
grants. Permissions may target any prefix, including `lasa.` — a
fine-grained grant without the full role. Value-level rules (allowed
values, §5's admin protection) apply regardless of how write access
was obtained. Scopes below match exact-key-or-dot-extended-prefix,
per the core.

### audience

Write, when `{entity-id}` is one of the connection's own entities:

- `lasa.entity.{entity-id}.attrs.{attr-name}` — set, clear
- `lasa.entity.{entity-id}.mute.{other-entity-id}` — set, clear
- `lasa.entity.{entity-id}.solo.{other-entity-id}` — set, clear

Read: `lasa.entity.` and `lasa.space.` — everything in the
vocabulary. (Nothing here is secret; names and attrs exist to be
shown to other participants, and a muted user should be able to see
that they are muted.)

### presenter

`audience`, plus the connect-time admission privilege: a presenter
ticket may carry ad-hoc entity definitions directly in the Connection
Config (`lasa-core.md` §4.2).

### moderator

`audience`, plus write:

- `lasa.entity.{entity-id}.dof` — set (any entity)
- `lasa.space.entity.muted.{entity-id}` — set, clear
- `lasa.space.client.blocked.{client-id}` — set, clear

### producer

`audience`, plus write:

- `lasa.entity.{entity-id}.sources.{channel-name}` — set, clear
- `lasa.entity.{entity-id}.sinks.{channel-name}` — set, clear
- `lasa.entity.{entity-id}.priority` — set, clear
- `lasa.entity.{entity-id}.render.gain` — set, clear
- `lasa.entity.{entity-id}.render.attenuation` — set, clear
- `lasa.entity.{entity-id}.render.size` — set, clear
- `lasa.entity.{entity-id}.render.directivity` — set, clear
- `lasa.space.role.blocked.{role-name}` — set, clear
- `lasa.space.channel.muted.{channel-name}` — set, clear
- `lasa.space.channel.gain.{channel-name}` — set, clear
- `lasa.space.channel.attenuation.{channel-name}` — set, clear

For the `render.*` keys and the channel overrides, `clear` reverts to
the documented default (absence-means-default).

### admin

The union of all four roles, plus two grants that belong to no
component role:

- may subscribe to **any** entity's sink tracks ("listening through
  e's ears" — the privacy statement in `lasa-core.md` §3.1);
- is **unblockable** (§5).

### Application space

Non-`lasa.` keys: ticketed connections have **no application-space
access unless granted** via ticket `permissions`; **unticketed spaces
are unrestricted** over application space, consistent with the
fragile mode.

## 3. Key lifetime

Keys are either **connection-scoped** or **space-scoped**, as marked
in §6. Connection-scoped keys are swept by the server on disconnect
with ordinary `clear` ops — seq-assigned, grouped, compacted,
tombstoned like any other write; nothing on the wire marks a sweep.
The server SHOULD clear the entity existence marker first, or in the
same group as the rest of the sweep — the ignore-when-absent rule
then covers readers mid-sweep. Sweep timing (immediate vs a grace
period) is server behaviour; on eviction the core's ordering rule
applies (sweep completes or is cancelled before the replacement
connection's admission writes).

Space-scoped keys survive reconnects — which is what makes
moderation launder-proof: personal mutes reset with the connection,
but a space-level mute or block cannot be escaped by reconnecting.

**Known weakness, recorded:** space-scoped keys promise "forever"
from a store whose loss is an anticipated event (the core's epochs
exist for it) — a restart or restore-to-earlier can silently un-kick.
Servers SHOULD durably persist `lasa.space.*` across restarts.

## 4. Channels

Sources and sinks are joined by named channels: **source S is audible
to sink K iff they share at least one channel that is not muted.**
An entity whose config omits `sources` (resp. `sinks`) gets
**`["main"]`**; an explicit empty array means deliberately none.

Channels carry optional render overrides, resolved per (source, sink)
pair. Muted channels contribute nothing to resolution. Across the
pair's shared unmuted channels, the **highest** set channel gain wins
and the **lowest** set channel attenuation wins; the winners then
meet the source's own `render.*` values as follows:

- **Gain composes multiplicatively**: effective gain = the source's
  `render.gain` × the winning channel gain. An unset channel gain is
  the identity (1.0), so the default is a no-op and entity balance
  within a channel survives a channel-wide boost or cut. The composed
  value may exceed the 0–3.0 range of either key; headroom policy is
  the renderer's.
- **Attenuation overrides when set**: if at least one shared unmuted
  channel sets `channel.attenuation`, the lowest such value
  **replaces** the source's `render.attenuation`; otherwise the
  source's own value applies unchanged. Unset is the default and is
  not a number — a channel with no attenuation set imposes no
  distance-law policy.

Because membership is pairwise, effective gain and attenuation are
properties of the (source, sink) pair: the same source may
legitimately render louder, or under a different distance law, to
listeners in one channel than to listeners in another.

## 5. Moderation

`lasa.space.entity.muted.{entity-id}` silences an entity's source in
every render; it affects rendering only, never the connection.

The **blocked keys do double duty** — there is no separate kick:
setting `lasa.space.client.blocked.{client-id}` or
`lasa.space.role.blocked.{role-name}` (value: 0 = forever, otherwise
blocked until that unix-seconds timestamp) terminates any live
matching connection *and* gates admission until expiry. A kick is a
block with a short expiry. Enforcement is the server acting as
profile consumer; at admission the check runs after ticket
verification and before entity admission
(`lasa-core.md` §4.6, code `0x1A5A06`).

**Admin is unblockable:** `lasa.space.role.blocked.admin` is an
invalid write (dropped like any other invalid value), and
`client.blocked` entries are ignored for — and never terminate — a
connection whose ticket bears `admin`. This guarantees in-protocol
recovery from any block state: an admin can always connect and clear.

## 6. Key vocabulary

### Connection-scoped

| Key | Values | Written by |
|---|---|---|
| `lasa.entity.{id}` | `true` | server only |
| `lasa.entity.{id}.name` | String | server, from config; immutable |
| `lasa.entity.{id}.attrs.{attr}` | any §1 type | config; owner at runtime |
| `lasa.entity.{id}.signed-attrs.{attr}` | any §1 type | server, from ticket; immutable |
| `lasa.entity.{id}.sources.{channel}` | `true` | config; `producer` |
| `lasa.entity.{id}.sinks.{channel}` | `true` | config; `producer` |
| `lasa.entity.{id}.priority` | `true` | config; `producer` |
| `lasa.entity.{id}.render.gain` | Number 0.0–3.0 | config; `producer` |
| `lasa.entity.{id}.render.attenuation` | Number 0.0–3.0 | config; `producer` |
| `lasa.entity.{id}.render.size` | Number ≥ 0.0 | config; `producer` |
| `lasa.entity.{id}.render.directivity` | Number 0.0–1.0 | config; `producer` |
| `lasa.entity.{id}.frame` | `"world"` \| `"head"` | config only |
| `lasa.entity.{id}.dof` | 0 \| 3 \| 6 | config; `moderator` |
| `lasa.entity.{id}.initial-pose.position.{x,y,z}` | Number | config only |
| `lasa.entity.{id}.initial-pose.attitude.{yaw,pitch,roll}` | Number, ±π | config only |
| `lasa.entity.{id}.mute.{other-id}` | `true` | owner |
| `lasa.entity.{id}.solo.{other-id}` | `true` | owner |

### Space-scoped

| Key | Values | Written by |
|---|---|---|
| `lasa.space.entity.muted.{entity-id}` | `true` | `moderator` |
| `lasa.space.client.blocked.{client-id}` | Number ≥ 0 | `moderator` |
| `lasa.space.role.blocked.{role-name}` | Number ≥ 0 | `producer` |
| `lasa.space.channel.muted.{channel-name}` | `true` | `producer` |
| `lasa.space.channel.gain.{channel-name}` | Number 0–3.0; absent = 1.0 (identity) | `producer` |
| `lasa.space.channel.attenuation.{channel-name}` | Number 0–3.0; absent = no override | `producer` |

### Semantics

**`lasa.entity.{id}`** — the existence marker. Set and cleared by the
server only, in response to connection and departure. All other keys
under the entity's prefix MUST be ignored while this is absent
(which is what makes the departure sweep safe mid-flight, §3).

**`name`** — non-unique human-readable label, from the config or
ticket definition.

**`attrs.{attr}` / `signed-attrs.{attr}`** — arbitrary values
attached to the entity, not functionally used by LASA, forwarded to
other clients. `attrs` come from the config (or the Setup) and are
owner-writable at runtime; `signed-attrs` come from inside the signed
ticket — different authorship guarantee, immutable. Flattening per
§1.

**`sources.{channel}` / `sinks.{channel}`** — the channels in which
this entity is heard / hears. Defaults per §4.

**`priority`** — marks this entity's source as important: rendering
should make every effort to preserve its accuracy and quality. Do not
set it on everything; that is self-defeating.

**`render.gain`** — source gain multiplier. Absence = 1.0. Composes
multiplicatively with channel gain, per pair (§4).

**`render.attenuation`** — distance-attenuation exponent. Absence =
2.0. 0.0 keeps the source at the same level everywhere; 1.0
attenuates linearly; 2.0 is natural inverse-square; 3.0 falls off
faster — useful for whispers and private communication. Fractional
values are meaningful (2.3 can feel more natural than 2.0 in virtual
space). May be replaced per pair by a set channel attenuation (§4).

**`render.size`** — source size in metres; 0.0 (the default) is a
point source. How renderers represent size is up to them.

**`render.directivity`** — 0.0 (default, omnidirectional) to 1.0
(maximally directional); interpretation is up to renderers.

**`frame`** — the reference frame of the entity's pose. `"world"`
(default) is the shared space. `"head"` pins the source relative to
every listener's head — an aural HUD, an angel on the shoulder.
Head-frame entities appear on the presence track with the krecord
head-frame flag set (`lasa-core.md` §7); their presence poses are
head-relative, not world positions.

**`dof`** — degrees of freedom: 6 (default) moves and turns freely;
3 turns in place; 0 is fixed. **The space enforces dof at ingest**:
for dof 3 the position components of arriving poses are ignored; for
dof 0 the whole pose is ignored and the entity's pose is its
initial-pose. Presence reflects the enforced pose, never the sent
one.

**`initial-pose.*`** — the entity's pose at join (coordinate
conventions per `lasa-core.md` §5.1). An entity's pose is its
initial-pose until the first accepted packet pose supersedes it.

**`mute.{other}` / `solo.{other}`** — personal, connection-scoped
preferences of the owning entity: mute silences one entity for this
listener; solo silences all but the solo'd set. Reset on reconnect
(the client rewrites what it knows).

**`lasa.space.entity.muted` / `client.blocked` / `role.blocked` /
`channel.*`** — per §§4–5.

## 7. Presence participation

All entities appear on the presence track; head-frame entities carry
the krecord head-frame flag and their poses are head-relative (§6
`frame`). The loudness byte is source loudness post-`render.gain`,
pre-attenuation; channel gains, being pair-dependent (§4), are
excluded — presence carries one loudness per source, not one per
pair.
