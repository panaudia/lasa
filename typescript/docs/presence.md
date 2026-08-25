# Presence

A space publishes one presence track carrying the pose and loudness
of every live entity, at a modest rate, as a game engine would. The
client applies the receive rules and maintains a roster.

## The roster

```ts
const roster = await client.subscribeRoster(() => {
  render(roster.snapshot());
});
```

`subscribeRoster` subscribes the presence track and returns a
`PresenceRoster`. The callback runs after every packet that changed
it. `snapshot()` returns copies of the entries in unspecified order.

A `RosterEntry` holds:

- `id`, the entity id.
- `pose`, the entity's last reported pose in LASA coordinates.
- `loudness`, an 8-bit level. `loudnessToDBFS` converts it to dBFS,
  and `LOUDNESS_SILENT` (255) converts to negative infinity.
- `headFrame`, true when the entity is head-framed rather than
  world-framed (`lasa-core.md` §3).

The roster includes the client's own entities. Applications that
draw other participants filter by id.

## The receive rules

Presence packets are datagrams, so they can be lost or reordered.
`lasa-core.md` §7 sets the rules a receiver applies, and
`PresenceRoster` applies them once so applications need not:

- A keyframe carries a generation number and the full record set
  from a starting index. Deltas carry the generation and the records
  that changed, by index.
- A packet whose generation differs from the current one is
  discarded, unless it is a keyframe, which resets the roster to the
  new generation.
- Within a generation, each index keeps the record from the latest
  datagram group id it has seen. An older group arriving late is
  ignored.

## The raw feed

`subscribePresence` delivers each packet as parsed, with its
datagram group id, and applies no rules:

```ts
await client.subscribePresence((groupId, msg) => {
  if (msg.kind === 'keyframe') { ... msg.gen, msg.first, msg.records }
  else { ... msg.gen, msg.records }
});
```

Applications using it take on the §7 rules themselves. Most want the
roster.
