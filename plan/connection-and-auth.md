# Connection and auth

*2026-08-13: the admission-collision and space-mode rules in this doc
are superseded by **`id-capabilities.md`** (ids as capabilities:
title/exercise, newest-exercise-wins on same-capability contests in
both modes, strict ticketed-XOR-unticketed with tickets rejected in
open spaces, claims ledger at the state engine). The affected
sections are annotated in place; everything else here stands, and the
superseded reasoning is retained as history.*

When connecting a client must present a Connection Config, which contains configuration and authentication information.

A Space inself may be pre-configured to allow unauthorised connections or it may reqire the Connection Config to include a signed Ticket

A client can connect as a single Entity which is an object-based audio source and/or sink. 
It can also connect with many Entities, sending and recieving to/from any of them.

## Decided 2026-07-27: the Connection Config rides CLIENT_SETUP

The Connection Config (`schemata/connection.schema.json` —
`{ticket, entities[] | setups[]}`, the ticket embedded as a JWT string)
is presented **once, at MoQ session establishment, as a setup
parameter on CLIENT_SETUP**: an odd-numbered (length-prefixed-bytes)
parameter key from the unregistered space, value = the UTF-8 JSON of
the connection object. Exact key value to be pinned in the wire
section of the spec (panaudia precedent for private keys: `0xFF01`).

Why here and not anywhere else:

- **Connection-scoped by definition.** Auth and capabilities are
  session-wide facts; SETUP is the one message that is about the
  session itself.
- **Transport-uniform.** Identical over WebTransport and raw QUIC —
  dissolving the current panaudia asymmetry where the WT URL query is
  the only config channel and raw-QUIC clients have none.
- **Auth completes before any track machinery exists.** The server
  authenticates the session, then derives every legal track tuple from
  the config's entity/client ids — giving **one enforcement point**:
  any later announce or subscribe whose namespace tuple falls outside
  the binding is rejected. (Fixes the current implementation's
  fall-through, where the namespace/token cross-check ends in a
  blanket accept.)
- **Clean rejection semantics.** Auth failure = no SERVER_SETUP; the
  session is terminated with an error code. No half-admitted state.
- **MoQ-native.** Unknown setup parameters are ignorable by spec, so
  this is MoQ's own extension point; the drafts have themselves been
  moving authorization tokens into setup parameters.

*Rejected:*

- **Auth on first SUBSCRIBE** (the current panaudia mechanism —
  connection config as an auth parameter on whichever track the client
  happens to subscribe to first). Codifies a mismatch: order
  dependence becomes normative, the announce path needs a duplicate
  rule, enforcement is per-track. The observed implementation
  corollaries (unauthenticated announces, identity stated twice and
  never cross-checked, capabilities expressed as track existence) are
  the price made visible.
- **URL query / PATH parameter.** Two transport-specific homes for one
  thing; tokens leak into request logs; size bounded by HTTP header
  limits. (Was panaudia's original design, deliberately abandoned —
  see `cloud-mixer/plan/history/ARCHITECTURE_DECISION_PURE_QUIC.md`.)
- **Out-of-band HTTPS pre-exchange** (POST config, get session token,
  connect with token). A second protocol surface, a token store, an
  extra round trip — machinery without a current need. Recorded as the
  natural fallback if the config ever outgrows a control message or an
  allocation-shaped connect response becomes real.
- **A hello/config track.** Chicken-and-egg (the server must subscribe
  before knowing who is asking) and puts connection facts above the
  layer that needs them.

**Implementation note (Eyevinn moqtransport v0.8.2, checked
2026-07-27).** The wire layer already supports this fully: KVP parsing
is generic, any key round-trips, odd keys carry bytes, no unknown-key
rejection. The gap is API-only, three small changes (fork or PR):
`sendClientSetup` hardcodes its parameter list (needs a
`WithSetupParameters` option — Go clients only; the TS client
hand-rolls its encoder); `onClientSetup` discards the received
parameters (needs an accessor or an `OnClientSetup(params) error`
callback); and the handshake completes unconditionally (the callback
must be able to reject *before* SERVER_SETUP is written). Draft-16
delta-encodes parameter keys ascending, so custom keys append in key
order.

## Decided 2026-07-27: unticketed spaces

*Superseded 2026-08-13 (`id-capabilities.md`): unticketed is formally
a dev mode with a declared-empty threat model; collisions now resolve
newest-wins (the reject-newcomer rule below and its id-hijack
rationale are reversed), and a config presenting a ticket to an
unticketed space is rejected — no mixed mode.*

A Space may be configured to accept unticketed connections. This is a
deliberate, uncommon operating mode — dev, demos, closed networks —
and is **essentially fragile**: identity is claim-based with no
authorship guarantee. The spec should say so plainly rather than dress
it up.

- The Connection Config carries `entities` directly, no ticket (the
  schema's `dependentRequired` already makes `setups` unavailable
  without one).
- **Uniqueness against live connections.** If the presented client id,
  or any presented entity id, is currently connected to the Space, the
  connection MUST be rejected. First claim wins; the claim lasts
  exactly as long as the connection.

Recorded consequences:

- **The rejection direction is deliberate: the new connection fails,
  the live one is never evicted.** Without a signature there is no way
  to prove the newcomer is the same principal, so evict-on-collision
  would be an id-hijack primitive. (Current panaudia does same-jti
  eviction — a ticketed-mode behaviour, where the ticket is the proof;
  whether LASA's ticketed mode evicts or rejects lands with the
  reconnect lifecycle below.)
- **Price: reclaim-after-death waits on death detection.** After an
  unnoticed network drop, the legitimate client cannot reclaim its ids
  until the server declares the old connection dead. Liveness tuning
  is server behaviour; the protocol inherits it. Id release and the
  departure sweep of connection-scoped keys
  (`core-profile-split.md`) should happen together at that same
  moment.

Both gaps this rule surfaced are resolved (2026-07-27):

- **`client_id` replaces jti-as-identity.** The Connection Config
  gains a required `client_id` property, present in both modes. The
  ticket gains a required `panaudia.client_id` claim; `jti` is no
  longer required and no longer the identity (it remains available as
  a standard replay id). In ticketed mode the config's `client_id`
  MUST equal the ticket's claim — explicit and trivially checkable
  rather than derived by JWT parsing. **Why the id must be inside the
  signed ticket when tickets are used:** moderation keys
  (`lasa.space.client.blocked.{client-id}`) are keyed by client id; an
  unsigned id would let a blocked client evade the block by re-picking
  an id while keeping a valid ticket. Unticketed spaces cannot have
  that protection — part of the recorded fragility. Schemas updated.
- **Unticketed connections are granted exactly `audience`** — enough
  to write their own mute/solo/attrs keys, nothing more. Recorded in
  `roles.md`.

## Decided 2026-07-27: setups and entities in the Connection Config

An entity comes to exist on a connection in one of two ways:

**`setups` — instantiating ticket-defined entities.** A Setup
(`setup.schema.json`: `{id, attrs, initial-pose}`) references, by id,
an entity definition inside the signed ticket (`panaudia.entities[]`).
The ticket carries the authorship-guaranteed definition — name,
sources/sinks, render params, priority, and attrs, which land in
`lasa.entity.{id}.signed-attrs.*`. The setup supplies the runtime-only
parts: the initial pose (overriding any `initial-pose` in the ticket
definition, which serves as a default) and unsigned attrs, which land
in `lasa.entity.{id}.attrs.*` — the signed/unsigned split the state
keys already define.

- Every `setup.id` MUST reference an entity in the ticket's entities
  block; an unknown or duplicated id rejects the connection.
- A subset is fine: ticket entities without a setup are simply not
  instantiated on this connection.

**`entities` — ad-hoc definitions carried directly in the
connection.** Admitted when the space is unticketed (anyone — the
fragile mode working as designed), or when the ticket bears **admin or
presenter** (decided 2026-07-27; this is the presenter role's defining
power, matching `roles.md`). Full `entity.schema.json` definitions;
their attrs are unsigned (`attrs.*` only — no `signed-attrs`).

**Both may appear together** — the natural presenter case:
instantiate the signed entity via a setup, add ad-hoc sources via
entities. The schema's `oneOf` is relaxed to `anyOf` (at least one of
the two, both permitted; `setups` still requires `ticket`). Id
collisions disambiguate: an id appearing more than once across the
setups list, the entities list, and the ticket's entities block
rejects the connection — ad-hoc entities cannot shadow ticket
definitions.

**Admission vs runtime.** "Admin/presenter can add entities" is a
connect-time **admission** rule gated on the ticket's roles. Role
rules proper govern runtime state writes. The two gates are
independent — which is why an unticketed space can admit entities from
anyone while still granting only `audience` at runtime.

## Decided 2026-07-27: subscribe authorization

The sink rule: **a client may subscribe to the sink tracks of its own
entities** — those instantiated on its connection, known to the server
from the Connection Config — **and a connection whose ticket bears
`admin` may subscribe to any entity's sink.**

- Admin-to-any-sink is an **extra grant on `admin`**, not a
  consequence of its role-union definition — none of the four
  component roles carries it (recorded in `roles.md`). Deliberate
  asymmetry: moderators can mute and kick but cannot eavesdrop.
- **Privacy statement for the spec, plainly:** subscribing to an
  entity's sink is hearing the space through that entity's ears. The
  spec should say in so many words that admins can do this, so
  deployments can surface it to users.
- Not granted to `producer` in v1 (monitoring mixes is a plausible
  want); adding it later is an additive role-rule change.

With that, the whole subscribe/announce authorization matrix closes,
enforced at the one point established by the CLIENT_SETUP binding:

| Track | May subscribe |
|---|---|
| `(space, "entity", e, "sink")` / formats | connection owning e; any `admin` connection |
| `(space, "client", c, "sink")` / `state` | the connection with `client_id` c only |
| `(space, "presence")` | any connected client |
| `(space, "entity", e, "source")` / `mono-object` | the space only (single-consumer uplink premise, `track-naming.md`) |
| `(space, "client", c, "source")` / `state` | the space only |

Announce is the mirror: a client may announce only its own `source`
namespaces — `(space, "entity", e, "source")` for its entities,
`(space, "client", c, "source")` for its client id.

## Decided 2026-07-27: sink lifecycle — subscription is the only signal

One principle closes the three parked policy items: **sink formats are
independent tracks, and subscription is the only lifecycle signal.**

1. **Render on demand.** The server renders and encodes an entity's
   sink only while at least one active subscription to any of its
   formats exists. A new subscription joins at live — no history; the
   last unsubscribe stops the render. Economically forced: per-listener
   encode dominates server cost (`head-rotation-and-sink-formats.md`
   §Server cost).
2. **Concurrent flavours allowed.** Each format subscription is
   independent and pays its own encode (e.g. `binaural` to the ears
   while an admin pulls `ambi3` for recording). Any limits are
   deployment policy, not protocol surface.
3. **Flavour switch is no machinery.** Subscribe the new format,
   unsubscribe the old; both flow during the overlap because they are
   independent tracks; any crossfade is client-local. Zero protocol
   surface.

## Decided 2026-07-27: reconnect lifecycle

*Amended 2026-08-13 (`id-capabilities.md`): rule 2's outcomes stand —
same-client evict is now "newest exercise wins" and cross-client
entity collision still rejects — but they are derived from the
title/exercise framing rather than argued case-by-case, and the
mixed-mode signed-beats-unsigned rule is deleted with mixed mode
itself.*

**1. "Reconnect" is not a protocol concept.** Every connection is a
fresh connection presenting a full Connection Config; there is no
resume token, no connection-id continuity, no half-alive state.
Continuity exists in exactly one place, where it was explicitly
designed: the **state cursor**, presented at state-subscribe
(`state-reconnect-and-gc.md`) and entirely independent of connection
identity. Audio has none by design — session-scoped group-ids, fresh
track instances (`audio-packet-structure.md` §9).

**2. Ticketed id collision: evict the old connection.** The opposite
of the unticketed rule, and the asymmetry is principled: the ticket is
the proof of principal that unticketed mode lacks. The served case is
the common one — a network drop leaves a half-open session; the client
reconnects immediately with the same ticket; waiting for server
death-detection would lock the legitimate user out. A stolen ticket
does let an attacker boot the real user, but a stolen ticket already
grants full impersonation — eviction adds no new capability.
Precisely: **same `client_id` on a valid ticket → the old connection
is evicted wholesale.** An entity-id collision with a *different*
client's live entity (an issuer coordination error) → **reject**; you
cannot evict someone else on entity grounds.

Recorded consequence, accepted: two devices sharing one ticket do not
get two sessions — newest wins, the first goes dark (matches current
panaudia same-jti behaviour). The multi-device story, if ever wanted,
is more tickets.

**3. The scope split gives the right moderation semantics for free.**
On evict/reconnect the departure sweep clears the old connection's
keys and the new admission re-writes from the config — all ordinary
seq'd ops, no special rules (`core-profile-split.md`).
Connection-scoped state (personal mutes/solos) resets; the client
rewrites what it knows. **Space-scoped moderation keys survive**
(`lasa.space.entity.muted.*`, `lasa.space.client.blocked.*`,
`lasa.space.role.blocked.*`) — reconnecting cannot launder a mute or a
kick. Deserves a sentence in the spec.

**4. Ticket reuse across reconnects is a feature.** With
jti-as-identity gone, the same ticket is valid for reconnection until
`exp` — which makes `exp` the real bound on ticket lifetime. Issuers
should set it deliberately; a ticket without `exp` never expires.

## Decided 2026-07-27: kick/block — the block keys do double duty

In keeping with `commands-as-state-ops.md`, there is no separate kick
mechanism. Setting `lasa.space.client.blocked.{client-id}` or
`lasa.space.role.blocked.{role-name}` (value: 0 = forever, else
blocked until that unix timestamp) does both halves:

- **Runtime**: the server — as profile consumer; the core stays
  content-blind — terminates any live matching connection when the key
  is set.
- **Admission**: the block is checked at connect, after ticket
  verification and before entity admission, until it expires.

"Kick" is simply a block with a short expiry — which is why the
vocabulary needs no kick key. (`lasa.space.entity.muted.*` is the
contrast case: it affects rendering only, never the connection.)

## Decided 2026-07-27: rejection error codes and how they surface

**The rejection cases** (wire values to be assigned in the lasa2
rewrite): malformed / schema-invalid config; invalid ticket
(signature, expired, wrong audience); `client_id` mismatch with the
ticket claim; id collision, unticketed (live client or entity id);
entity collision with another client's live entity; entities without
privilege (ad-hoc entities, ticket lacks admin/presenter); blocked;
space at capacity.

**How they surface.** By the time the Connection Config is evaluated,
the QUIC connection (raw) or WebTransport session (browser) is already
established — CLIENT_SETUP flows inside it. Rejection is therefore not
a failed connection attempt but a deliberate close of a live session,
and both transports carry a code and a reason string on close:

- **Raw QUIC**: CONNECTION_CLOSE with an application error code
  (varint) plus a reason phrase (quic-go: `ApplicationError`).
- **WebTransport**: the session-close capsule carries a 32-bit
  `closeCode` and a reason string, surfacing in the browser on the
  `transport.closed` promise as `{closeCode, reason}` — a normal,
  well-supported API path.

Rules that follow:

- **LASA codes must fit in 32 bits** (the WebTransport bound, the
  narrower of the two transports).
- **Correctness rides the code alone.** Reason phrases are advisory
  and may be truncated or dropped. Parameterised rejections put their
  data in the reason string (blocked-until carries the expiry
  timestamp) but a client must not need to parse it.
- **Distinct code range.** MoQ defines its own session-terminate code
  table (protocol violation, unauthorized, …); LASA codes sit in an
  application range that cannot collide with it — the same discipline
  as the unregistered SETUP parameter key.

**Recorded side-benefit of the CLIENT_SETUP decision:** had auth run
at the HTTP CONNECT (URL query), a rejection would be a failed CONNECT
— which browsers surface as an opaque failed `ready` promise, no code,
no reason. Because rejection happens at the MoQ layer inside an
established session, the browser client reliably learns *why*.

## Still open in this doc

None — all sections above are decided as of 2026-07-27. The doc is
ready to feed the lasa2 rewrite.

*Post-review patches (2026-07-27, `review-resolutions.md`): identifier
grammar `[a-z0-9-]{1,128}` incl. space ids (aud MUST equal the space
id); admin unblockable; ticket `permissions` claim (set/clear/read
prefixes, union with roles; app-space deny-by-default when ticketed,
unrestricted when unticketed); evict-sweep ordering; signed-beats-
unsigned on mixed-mode entity collision (deleted 2026-08-13 with
mixed mode itself — `id-capabilities.md`); zero-entity connections
legal; ticket with no roles ⇒ audience.*

