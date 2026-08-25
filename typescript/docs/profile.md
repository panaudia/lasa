# Base profile

The base profile (`lasa-base-profile.md`) is the `lasa.` key
vocabulary every space implements: who owns which entity, what it is
called, which channels it is heard in and hears, how it renders, who
has muted whom, and who is blocked. LASA has no command channel. A
"command" is a `set` or `clear` on one of these keys, ordered and
echoed by the server like any other write (`lasa-core.md` §6). The
helpers here build the keys and encode the values so an application
never spells out `lasa.entity.${me}.mute.${other}` by hand.

Two concerns are kept apart, and the helpers are grouped the same
way. **Channels own hearing**: who is audible to whom, muted, at what
gain and under what distance law is all expressed as channel
membership and channel policy. **Roles own permission**: a role says
which keys a connection may write, nothing more. There is
deliberately no role-wide mute or gain. To silence or rebalance a
group of people, put them in a channel.

## Personal: mutes, solos, attrs

Owner-scope operations act on an entity this connection owns. They
are methods on the entity's publisher, or on `client.controls(id)`
when the publisher is not needed yet:

```ts
const me = await client.entity('paul-desk');

await me.mute('heckler');      // silences the pair both ways
await me.unmute('heckler');
await me.solo('presenter');    // hear the solo'd set only; wins over mutes
await me.unsolo('presenter');

await me.setAttr('colour', '00aaff');
await me.setAttrs({ 'avatar.hat': true, score: 3 });
await me.clearAttr('colour');
```

Mute is bidirectional (§6): muting someone stops you hearing them
*and* stops them hearing you. Solo silences everyone but the solo'd
set for this listener only. Both are connection-scoped and reset on
reconnect, so a client that wants them to persist rewrites what it
knows after `connect`.

Attrs are free-form values for other clients (`BaseValue`: boolean,
string or number). Paths are dotted identifiers and each leaf is one
key; arrays are not allowed (§1), so encode a list as a JSON string.

## Space: moderation, channels, rendering

Space-wide operations live on `client.space`. Each is gated by role on
the server (§2: `moderator` for entity mutes, client blocks and
`dof`; `producer` for channels, rendering and role blocks) and a
write the caller's roles do not allow is dropped silently. A client
should not offer UI for actions its role forbids.

### Moderation (§5)

```ts
await client.space.muteEntity('heckler');           // rendering only; connection stays
await client.space.unmuteEntity('heckler');

await client.space.blockClient('c-heckler');        // forever
await client.space.blockClient('c-heckler', until); // until a Date
await client.space.kickClient('c-heckler', 5 * 60_000); // block expiring in 5 min
await client.space.unblockClient('c-heckler');

await client.space.blockRole('audience', until);
await client.space.unblockRole('audience');
```

A block terminates a live connection and refuses readmission until
it expires; "a kick is a block with a short expiry" (§5), so
`kickClient` is sugar over `blockClient`. `admin` is unblockable.

### Channels (§4)

A source is audible to a sink iff they share at least one unmuted
channel. Membership is per entity and directional: `heard-in` is
where the source is heard, `hears` is what the sink listens to.
Channels carry optional gain and attenuation overrides that resolve
per (source, sink) pair.

```ts
await client.space.addHeardIn('presenter', 'stage');
await client.space.removeHears('presenter', 'main');

// Replace both sides in one group, diffed against the current view.
const cur = view.entity('presenter')!;
await client.space.setChannels('presenter', cur, { heardIn: ['stage'], hears: ['stage', 'main'] });

await client.space.muteChannel('backstage');
await client.space.setChannelGain('stage', 1.5);         // amplitude, 0–3
await client.space.setChannelAttenuation('whisper', 3);  // distance exponent, 0–3
await client.space.setChannelGain('stage', undefined);   // clear
```

Emptying an entity's `heard-in` leaves it inaudible; the config path
never does that (admission always writes at least one channel), but a
`producer` may at runtime.

### Rendering (§6)

```ts
await client.space.setRender('presenter', { gain: 1.5, attenuation: 2.3 });
await client.space.setRender('presenter', { gain: null });  // back to the default
await client.space.setPriority('presenter', true);
await client.space.setDof('kiosk', 0);                       // fixed in place
```

`undefined` fields are left untouched and `null` clears a key to its
documented default (gain 1.0, attenuation 2.0, size 0, directivity 0,
dof 6). Ranges are checked before anything is sent.

## Reading: the entity view

`baseView(store)` is a typed read over a `Store` whose prefixes
include `lasa.` (see [State](state.md)). Read after
`await sync.settled()`.

```ts
const store = await Store.create(['lasa.']);
const sync = client.syncState(store);
const view = baseView(store);

await sync.settled();
for (const e of view.entities()) {
  console.log(e.id, e.name, e.owner, e.heardIn, e.render.gain ?? 1);
}
view.entity('presenter');          // one EntityView, or undefined
view.isEntityMuted('heckler');
view.clientBlock('c-heckler');     // { id, until: Date | null } or undefined
view.blockedClients(); view.blockedRoles();
view.channel('stage');             // { muted, gain?, attenuation? }
view.channels();
```

The view honours the profile's rules: keys under an entity whose
existence marker is absent are ignored (a departing entity's subtree
can be mid-sweep, §3/§6), and a value that is not a valid boolean,
string or number is treated as absent (§2). Each call scans the
store, which is the right shape for a few hundred keys.

The view reads what the client is allowed to see, which for every
role is all of `lasa.entity.` and `lasa.space.` (§2). Personal mutes
are included deliberately: nothing there is secret, and a muted user
should be able to see that they are muted.

## Underneath

`baseKeys` exposes the key builders (`baseKeys.personalMute(me, other)`
→ `lasa.entity.{me}.mute.{other}`), each validating its identifiers
against `lasa-core.md` §2 and throwing `MalformedError` on a
violation. `encodeValue` / `decodeValue` are the §1 codec, byte-exact
with what the Go server writes (`true`, `"head"`, `1.5`). An
application writing its own profile beside `lasa.` can reuse both.

Every helper resolves to `client.writeState(...)`; there is nothing
they can do that a raw write cannot. What they add is the vocabulary
as a typed surface, and range and identifier checks at the call
rather than a silent drop at the server.
