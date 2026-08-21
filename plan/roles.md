# Roles

*Rename note (2026-08-09): `sources.{channel-name}` /
`sinks.{channel-name}` below are now `heard-in.{channel-name}` /
`hears.{channel-name}` (membership-vs-track-direction collision; see
the note in `lasa-state-keys.md`). Current names in
`lasa-base-profile.md` §2/§6.*

- admin
- presenter 
- moderator
- producer
- audience

Ticketed connections carry their roles in the ticket
(`panaudia.roles`). **Unticketed connections are granted exactly
`audience`** (decided 2026-07-27) — as is a ticket with an absent or
empty `roles` claim.

Roles are **compact aliases** over prefix-rule machinery: a ticket may
also carry explicit `permissions` (`set`/`clear`/`read` prefix lists)
and the connection's effective access is the union
(`review-resolutions.md` §4). Role scopes below match **exact key or
dot-extended prefix** — `lasa.entity.{e}.` covers the marker key
`lasa.entity.{e}` too, without leaking sibling ids
(`review-resolutions.md` §2).


### admin 

All of audience, presenter, moderator and producer

Plus (extra grant, not part of the union — decided 2026-07-27): may
subscribe to **any** entity's sink tracks ("listening through e's
ears"). No other role has this; every connection may subscribe to its
own entities' sinks regardless of role.

*(Superseded: producer holds the same any-sink grant as part of the
mixing desk, and admin's copy derives from the union — see
`lasa-base-profile.md` §2. The 2026-08-04 ambi-format role gate that
briefly accompanied it was removed 2026-08-21: sink formats carry no
authorization dimension, core §3.)*

### audience

**write**

When entity-id is one of the client's own

- lasa.entity.{entity-id}.attrs.{attr-name} set clear
- lasa.entity.{entity-id}.mute.{other-entity-id} set clear
- lasa.entity.{entity-id}.solo.{other-entity-id} set clear

**read**

Prefixes (widened 2026-07-27 — was own-entities-only, which
contradicted attrs-are-forwarded, presence's pointer to the existence
marker, and you-are-muted UX; see `review-resolutions.md` §5):

- lasa.entity.
- lasa.space.


### presenter

`audience` and can add any number of Entities to a connection —
ad-hoc entity definitions carried directly in the Connection Config,
a connect-time admission privilege (`connection-and-auth.md` §setups
and entities)

### moderator

`audience` and

**write**

- lasa.entity.{entity-id}.dof set
- lasa.space.entity.muted.{entity-id} set clear
- lasa.space.client.blocked.{client-id} set clear

**read**

- lasa.entity.
- lasa.space.


### producer

`audience` and

- lasa.entity.{entity-id}.sources.{channel-name} set clear
- lasa.entity.{entity-id}.sinks.{channel-name} set clear
- lasa.entity.{entity-id}.priority set clear
- lasa.entity.{entity-id}.render.gain set clear
- lasa.entity.{entity-id}.render.attenuation set clear
- lasa.entity.{entity-id}.render.size set clear
- lasa.entity.{entity-id}.render.directivity set clear
- lasa.space.role.blocked.{role-name} set clear
- lasa.space.channel.muted.{channel-name} set clear
- lasa.space.channel.gain.{channel-name} set clear
- lasa.space.channel.attenuation.{channel-name} set clear

(render.* and channel gain/attenuation: clear reverts to the
documented default / removes the override — absence-means-default,
decided 2026-07-27, see `lasa-state-keys.md`)

**read**

- lasa.entity.
- lasa.space.


