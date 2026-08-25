# State

Everything in a space that is not audio, pose or presence lives in
the state store: a centralised key-value store with sequential
consistency, replicated to clients over a reliable MoQ track
(`lasa-core.md` §6). Keys are strings, values are bytes. The base
profile uses the `lasa.` prefix for the space's own vocabulary
(roles, channels, moderation, rendering parameters), and
applications add their own prefixes beside it.

## Reading

```ts
const store = await Store.create(['lasa.', 'app.']);
const sync = client.syncState(store);

await sync.settled();
const raw = store.get('app.scene.name');
```

`Store.create` takes the space id and the key prefixes the client
wants. The server delivers only keys under those prefixes that the
client's roles allow it to read (§6.4). The set is canonicalised, so
order and duplicates do not matter.

`client.syncState` subscribes the state track with the store's
cursor and feeds it, re-subscribing with backoff whenever the server
ends an instance. It returns the running `StateSync`. Because
messages apply asynchronously, `await sync.settled()` before reading
so every message delivered so far has landed. `sync.stop()` ends it,
and `client.close()` stops it too.

`store.get(key)` returns the value bytes or undefined. `store.size`
is the key count. Text values decode with `TextDecoder`.

The store is iterable: `for (const [key, { value, seq }] of store)`
walks every live key. That is what the base-profile view uses to
list entities. For the `lasa.` vocabulary, prefer that view over raw
keys: see [Base profile](profile.md).

## Writing

```ts
await client.writeState(
  { kind: 'set', key: 'app.scene.name', value: new TextEncoder().encode('Lobby') },
  { kind: 'clear', key: 'app.scene.previous' },
);
```

`writeState` publishes the ops on the client's state source track.
Each call is one message. A `group` op asks the server to apply its
members together. It is a hint, not a transaction (§6 gives it no
consistency guarantee), and a group over the server's op cap is
dropped whole:

```ts
await client.writeState({
  kind: 'group',
  ops: [
    { kind: 'set', key: 'app.a', value: a },
    { kind: 'set', key: 'app.b', value: b },
  ],
});
```

Keys match `[a-z0-9.-]{1,512}`. Values are at most 65536 bytes.
Groups do not nest. Violations throw `MalformedError` before
anything is sent.

A write is not applied locally. It goes to the server, which orders
it, assigns its sequence number, and echoes it back through the
subscription like any other change. The store sees the write when
the echo arrives, so `await sync.settled()` after a write does not
guarantee the write is visible yet, only that everything already
delivered is.

The server refuses writes the client's roles do not permit, and
silently drops them (§6.2). A client that needs confirmation reads
its key back.

## Reconnects and cursors

The store earns a **cursor** once a catch-up has completed: a
position in the space's history from which a later subscription can
resume, receiving only what changed since. The cursor lives with the
store, in memory. `StateSync` presents it on its own re-subscribes,
and a new `LasaClient` handed the same `Store` presents it too, so
within a page's lifetime a reconnect lands on a delta whenever the
server can prove one complete (§6.6 lists the cases it cannot:
epoch change, a changed prefix set, a cursor older than the
server's tombstone window, changed visibility). The store then
resynchronises from a snapshot without the application doing
anything.

A fresh page load starts with no cursor and takes a snapshot. The
client does not persist state or cursor across loads. `lasa-core.md`
§6.6 allows this outright (a client that does not persist its store
must not persist a cursor), and for the sizes involved, a few hundred
keys of small values over a reliable stream, a snapshot is cheap.
Cross-load persistence is implemented in the Go reference
(`clientstore.Persistence`) and was left out of this client on
purpose: its contract, entries and cursor saved atomically with the
cursor never ahead of the entries, is easy to get subtly wrong in an
IndexedDB adapter, and getting it wrong resurrects deleted keys
silently. It comes back when an application's snapshot is large
enough to notice.

When the server ends a subscription instance, because the client
fell too far behind or was displaced, `StateSync` re-subscribes and
the store discards the half-open catch-up before starting again.

## The messages

`StateMessage` is the decoded downstream form, with server-assigned
sequence numbers. `UpstreamStateMessage` is what a client writes,
the same ops without them. A decoded message is assignable to the
upstream type, so a client can republish a message it received
unchanged. `Store.applyLive` and `Store.applyCatchUp` are the two
lanes `StateSync` feeds, exported for applications driving a store
from another source.
