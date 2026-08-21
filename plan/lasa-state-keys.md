# LASA base profile — state keys

*Reconciled 2026-07-27 against `roles.md`, `connection-and-auth.md` and
`core-profile-split.md`. This is base-profile prose: the core reserves
the `lasa.` prefix and attaches no meaning to any key; everything here
is profile vocabulary. Connection-scoped lifetime is likewise profile
prose — the server sweeps these keys with ordinary `clear` ops on
disconnect (`core-profile-split.md` §Resolved).*

*Rename note (2026-08-09): the channel-membership keys written below as
`sources.{channel-name}` / `sinks.{channel-name}` were renamed
`heard-in.{channel-name}` / `hears.{channel-name}` — the old names
collided with the core's source/sink track-direction vocabulary and
were misread (twice, by the author included) as track lists. The spec
(`lasa-base-profile.md` §6) carries the current names; this doc is
left as the dated record.*

All `lasa.entity.` and `lasa.space.` prefixed state is structured in a particular way. 

All values are utf-8 encoded and are one of three types:

- Boolean
- String
- Number

They follow the same encoding conventions as the corresponding types in JSON.
(The core's 64 KiB value cap — `state-wire-encoding.md` — is far above
anything in this vocabulary.)

Write access below is stated per role as defined in `roles.md`;
`admin` always has the union of all roles' access. Where a key says
"absence means the default", `clear` is the way to revert it.


### These are values scoped to the connection

- lasa.entity.{entity-id} 
- lasa.entity.{entity-id}.name 
- lasa.entity.{entity-id}.attrs.{attr-name} 
- lasa.entity.{entity-id}.signed-attrs.{attr-name} 
- lasa.entity.{entity-id}.sources.{channel-name}
- lasa.entity.{entity-id}.sinks.{channel-name}
- lasa.entity.{entity-id}.priority
- lasa.entity.{entity-id}.render.gain
- lasa.entity.{entity-id}.render.attenuation
- lasa.entity.{entity-id}.render.size
- lasa.entity.{entity-id}.render.directivity
- lasa.entity.{entity-id}.frame
- lasa.entity.{entity-id}.dof
- lasa.entity.{entity-id}.initial-pose.position.x
- lasa.entity.{entity-id}.initial-pose.position.y
- lasa.entity.{entity-id}.initial-pose.position.z
- lasa.entity.{entity-id}.initial-pose.attitude.yaw
- lasa.entity.{entity-id}.initial-pose.attitude.pitch
- lasa.entity.{entity-id}.initial-pose.attitude.roll
- lasa.entity.{entity-id}.mute.{other-entity-id}
- lasa.entity.{entity-id}.solo.{other-entity-id}

### These are values scoped to the lifetime of the space

- lasa.space.entity.muted.{entity-id}
- lasa.space.client.blocked.{client-id}
- lasa.space.role.blocked.{role-name}
- lasa.space.channel.muted.{channel-name}
- lasa.space.channel.gain.{channel-name}
- lasa.space.channel.attenuation.{channel-name}


**lasa.entity.{entity-id}**

Allowed values: `true` 

*(Superseded 2026-08-13, id-capabilities: the marker's value is the
OWNING CLIENT ID, not `true` — ownership readable wherever the entity
is; `lasa-base-profile.md` §6.)*

set and clear by the server only in response to connection 

Signifies the presence of this Entity in the Space.

All other values for an Entity prefixed with key should be ignored when this is missing or removed

In the departure sweep the server SHOULD clear this marker first, or
in the same group as the rest of the sweep — the ignore-when-absent
rule then covers readers mid-sweep (`core-profile-split.md`).

**lasa.entity.{entity-id}.name**

Allowed values: String

Immutable set at connection time from the Connection object

Gives a non-unique human-readable label for the Entity.

**lasa.entity.{entity-id}.attrs.{attr-name}**

Allowed values: Any

may be set from the Connection object or set/clear by the user at run time

An arbitrary set of values attached to the Entity that are not functionally used by LASA, 
but are potentially forwarded to other clients.

Feel free to extend

**lasa.entity.{entity-id}.signed-attrs.{attr-name}**

Allowed values: Any

An immutable set of values set from the Ticket in a Connection object

Like `attrs` above but these come from inside a signed ticket and so have a different life-cycle and authorship guarantee.

**lasa.entity.{entity-id}.sources.{channel-name}**

Allowed values: `true`

Set from the Connection object and set/clear at runtime by `producer`.

One of a set of channels in which this Entity will be heard

**lasa.entity.{entity-id}.sinks.{channel-name}**

Allowed values: `true`

Set from the Connection object and set/clear at runtime by `producer`.

One of a set of channels which this Entity will hear.

**lasa.entity.{entity-id}.priority**

Allowed values: `true`

Set from the Connection object and set/clear at runtime by `producer`
(decided 2026-07-27 — producer wins over the earlier connection-only
rule).

Defines this Entity's source as being important such that when rendering, 
every effort should be made to preserve its accuracy and quality.

Don't set this on everything this would be self-defeating.

**lasa.entity.{entity-id}.render.gain**

Allowed values: Number - min: 0.0 max: 3.0

Set from the Connection object and set/clear at runtime by `producer`.

This turns up or down the gain for this Entity's source. Absence means
the default: 1.0.

**lasa.entity.{entity-id}.render.attenuation**

Allowed values: Number - min: 0.0 max: 3.0

Set from the Connection object and set/clear at runtime by `producer`.
Absence means the default: 2.0.

This defines the way this Entity's source attenuates with distance.

- 0.0 Keeps this source at the same level everywhere
- 1.0 Causes this source to attenuate linearly with distance
- 2.0 The natural inverse square behaviour we have in the real world. 
- 3.0 Inverse cubed, like magnetism, the volume drops off faster with distance, useful for whispers and private communication.

You can set fractional values. I find 2.3 can feel more natural than 2.0 in virtual space.

**lasa.entity.{entity-id}.render.size**

Allowed values: Number - min: 0.0 

Set from the Connection object and set/clear at runtime by `producer`.
Absence means the default: 0.0 (a point source).

This is a size in meters of the source. 0.0 is a point source. 
Exactly how renders choose to represent `size` is up to them.

**lasa.entity.{entity-id}.render.directivity**

Allowed values: Number - min: 0.0 max: 1.0

Set from the Connection object and set/clear at runtime by `producer`.
Absence means the default: 0.0 (omnidirectional). Exactly how renders
interpret intermediate values is up to them, as with `size`.

**lasa.entity.{entity-id}.frame**

Allowed values: String - "world" or "head"

Set from the Connection object, defaults to "world" if not given.

Allows you to pin this Entity's source to a position relative to everyone’s heads, like a HUD but aural. 
This is useful if you want to place an angel on someone's shoulder.

**lasa.entity.{entity-id}.dof**

Allowed values: Number - 0 or 3 or 6

Set from the Connection object, defaults to 6. Can be set at runtime by `moderator`.

This is the degrees of freedom of movement an Entity has. 

- 6 is the default and allows the Entity to move freely withing the Space and turns its head.
- 3 still allows head turning but fixes this Entity in one position. Useful if you want to place people in fixed seats.
- 0 no movement is allowed. Audio is still rendered spatially but from a fixed listening position.

**lasa.entity.{entity-id}.initial-pose.position.x**

Allowed values: Number

Set from the connection, gives the initial x coordinate in meters of this Entity's position in the Space when it joins.

In this coordinate system x is forward.

**lasa.entity.{entity-id}.initial-pose.position.y**

Allowed values: Number

Set from the connection, gives the initial y coordinate in meters of this Entity's position in the Space when it joins.

In this coordinate system y is to the left.

**lasa.entity.{entity-id}.initial-pose.position.z**

Allowed values: Number

Set from the connection, gives the initial z coordinate in meters of this Entity's position in the Space when it joins.

In this coordinate system z is up.

**lasa.entity.{entity-id}.initial-pose.attitude.yaw**

Allowed values: Number -PI to +PI

Set from the connection, gives the initial yaw rotation in radians of this Entity's attitude in the Space when it joins.

In this coordinate system positive yaw is rotation anti-clockwise about the z-axis. ie to the left in the horizontal plane.


**lasa.entity.{entity-id}.initial-pose.attitude.pitch**

Allowed values: Number -PI to +PI

Set from the connection, gives the initial pitch rotation in radians of this Entity's attitude in the Space when it joins.

In this coordinate system positive pitch is rotation upwards away from the horizontal plane.


**lasa.entity.{entity-id}.initial-pose.attitude.roll**

Allowed values: Number -PI to +PI

Set from the connection, gives the initial roll rotation in radians of this Entity's attitude in the Space when it joins.

In this coordinate system positive roll is rotation anticlockwise around the line of sight.


**lasa.entity.{entity-id}.mute.{other-entity-id}**

Allowed values: `true`

Set/clear at run-time by this Entity to mute another Entity


**lasa.entity.{entity-id}.solo.{other-entity-id}**

Allowed values: `true`

Set/clear at run-time by this Entity to mute all other Entities except another Entity (and any other solo'd Entities)


**lasa.space.entity.muted.{entity-id}**

Allowed values: `true`

Set/clear by `moderator` to mute/unmute any entity at run time.
Affects rendering only, never the connection — the contrast case to
the blocked keys below.


**lasa.space.client.blocked.{client-id}**

Allowed values: number ≥ 0 

Set/clear by `moderator` to kick/block another connection. The key
does double duty (`connection-and-auth.md`): setting it terminates any
live matching connection and gates admission until it expires — a
"kick" is a block with a short expiry.

- 0 will block them from this Space forever
- Any other value is treated as a unix timestamp, and they will be blocked until that time


**lasa.space.role.blocked.{role-name}**

Allowed values: number ≥ 0 

Set/clear by `producer` to kick/block all connections bearing a role.
Double duty as above: terminates live matching connections and gates
admission until expiry.

- 0 will block them from this Space forever
- Any other value is treated as a unix timestamp, and they will be blocked until that time



**lasa.space.channel.muted.{channel-name}**

Allowed values: `true`

Set/clear by `producer` to mute/unmute a channel

If a source and sink have more than one channel between them and if any is not muted, the source will be heard.


**lasa.space.channel.gain.{channel-name}**

Allowed values: Number min: 0 max: 3.0

Set/clear by `producer` to adjust the gain of a whole channel.
Absence means no override. *(Superseded: absence = 1.0, the identity
— profile §§4, 6; "no override" is the attenuation rule.)*

If a source and sink have more than one channel between them, the highest gain value is used.

**lasa.space.channel.attenuation.{channel-name}**

Allowed values: Number min: 0 max: 3.0

Set/clear by `producer` to adjust the attenuation of a whole channel.
Absence means no override.

- 0.0 Keeps this source at the same level everywhere
- 1.0 Causes this source to attenuate linearly with distance
- 2.0 The natural inverse square behaviour we have in the real world. 
- 3.0 Inverse cubed, like magnetism, the volume drops off faster with distance, useful for whispers and private communication.

If a source and sink have more than one channel between them, the lowest attenuation value is used.