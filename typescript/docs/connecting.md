# Connecting

`LasaClient.connect` opens one WebTransport session to a space,
performs the LASA handshake inside the MoQ setup, and returns a
client whose publishers are wired as the server subscribes them.
Everything the handshake needs travels in `LasaClientOptions`.

## The options

```ts
const client = await LasaClient.connect({
  url: 'https://lasa.example.com:4443/lasa',
  spaceId: 'main',
  clientId: 'alice',
  ticket: jwt,                       // omit against an unticketed space
  setups: [{ id: 'alice-voice' }],   // ticket-defined entities
  entities: [...],                   // ad-hoc entities, where permitted
  serverCertificateHashBase64: hash, // development servers only
  onClosed: (info) => { ... },
  debug: false,
});
```

`url` is the WebTransport endpoint. The path is the server's, and
`/lasa` is the conventional one.

`spaceId` names the space. In a ticketed space it MUST equal the
ticket's `aud` claim, because the audience is the space. In an
unticketed space it is whatever the endpoint serves.

`clientId` is the client's public identity within the space. The
base profile records it under `lasa.client.`, and it is the name a
ticket grants. When a ticket is presented, `clientId` MUST
match the ticket's `client_id` claim, or the server refuses with
`CLIENT_ID_MISMATCH`.

## Tickets, entities and setups

A space is either ticketed or unticketed, never both.

A **ticket** is a signed JWT granting a client id, roles, and a set
of entity definitions (`lasa-core.md` §4.3). Against a ticketed
space the ticket is mandatory, and the client instantiates the
entities the ticket defines by listing their ids in `setups`. A
setup MAY add `attrs`, which are merged over the ticket's.

An **entity** is a source of audio with a position, a name, and
rendering parameters (`LasaEntity`, mirroring
`schemata/entity.schema.json`). Ad-hoc entities are listed in
`entities`. An unticketed space accepts them from anyone. A ticketed
space accepts them only from tickets carrying the presenter or admin
role, and refuses others with `AD_HOC_NOT_PERMITTED`.

Two fields on `LasaEntity` shape the audio path rather than the
scene, and both default to the least demanding setting:

- `quality` (0 to 2) states how latency-tolerant this entity's
  audio is. The space sizes its ingest buffer from it, and the
  client sizes its own playout buffer from it. 0 is interactive.
- `redundancy` (0 to 7) is the maximum redundancy offset, and it
  works in both directions. Above 0 the space repeats earlier audio
  in later packets on the sink track and the playout buffer grows a
  floor to make use of the repeats; and this client repeats its own
  audio at the same offset on the entity's uplink, where the space's
  ingest buffer (provisioned from the declaration) repairs a burst of
  up to `redundancy` lost packets exactly. The price is double the
  audio bitrate each way and a buffer floor of 2 × (offset + 2) × 5 ms
  at each end. `startCapture` may lower the uplink offset below the
  declaration, never raise it.

Unticketed mode is a development mode with an empty threat model.
Presenting a ticket to an unticketed space is refused with
`MODE_MISMATCH`.

## Development servers and certificate pinning

A development server running on a self-signed certificate prints its
certificate hash at boot:

```
serverCertificateHashes (sha-256, base64): <hash>
```

Pass that string as `serverCertificateHashBase64` and WebTransport
accepts the certificate. A server on a real certificate needs no
hash, and the option MUST be omitted in production.

## Rejection

A handshake the server refuses surfaces as a `LasaRejectionError`
thrown from `connect`. It carries the `code` (one of `LASA_ERR`) and
the server's advisory `reason` string.

| Name | Code | Meaning |
|---|---|---|
| `MALFORMED_CONFIG` | `0x1A5A00` | The Connection Config failed schema validation. |
| `INVALID_TICKET` | `0x1A5A01` | Bad signature, expired, not yet valid, wrong audience, or no ticket to a ticketed space. |
| `CLIENT_ID_MISMATCH` | `0x1A5A02` | `clientId` differs from the ticket's `client_id`. |
| `ENTITY_COLLISION` | `0x1A5A04` | An entity id is live under another client (ticketed spaces). |
| `AD_HOC_NOT_PERMITTED` | `0x1A5A05` | Ad-hoc entities from a ticket without the role for them. |
| `BLOCKED` | `0x1A5A06` | The client is blocked. The reason SHOULD carry the expiry. |
| `AT_CAPACITY` | `0x1A5A07` | The space is full. |
| `MODE_MISMATCH` | `0x1A5A08` | A ticket was presented to an unticketed space. |

`0x1A5A03` is retired and never appears on the wire.
`isLasaRejectionCode` and `lasaErrorName` classify and name codes
for logging.

```ts
try {
  client = await LasaClient.connect(opts);
} catch (e) {
  if (e instanceof LasaRejectionError && e.code === LASA_ERR.INVALID_TICKET) {
    // fetch a fresh ticket and retry
  }
  throw e;
}
```

## Ids are capabilities

An id (client or entity) is a capability, exercised by one connection
at a time. If a second connection exercises the same client id, the
newer one wins and the older session is closed. In a ticketed space
that applies within the ticket's grant. In an unticketed space it
applies to any colliding id. Either way the displaced client learns
of it through `onClosed`.

## The session lifecycle

`onClosed` fires when a session that completed its handshake ends:
a moderation sever, a supersession as above, a server shutdown, or a
transport failure. `closeCode` carries a LASA code when the close was
a deliberate LASA action: `LASA_ERR.BLOCKED`, `LASA_ERR.SUPERSEDED`,
or `LASA_ERR.SHUTTING_DOWN`, the last of which means the space is
stopping and reconnecting later is reasonable. A transport failure
carries no LASA code. A rejected handshake never reaches `onClosed`.
It is `connect`'s to report.

`close()` stops every capture and sink, ends the state sync, and
tears down the worker and the transport. It is idempotent: a second
call returns the first call's promise. Any `entity()` call still
waiting at close rejects with `client closed`.

Reconnection is the application's decision. A new `connect` starts
a fresh session with fresh subscriptions. The state store carries
its cursor across sessions so the second session resumes where the
first left off (see [State](state.md)).
