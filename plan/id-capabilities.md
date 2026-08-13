# Id capabilities

*Decided 2026-08-13. Supersedes the collision rules of
`connection-and-auth.md` (2026-07-27) and closes deliberate deviation 2
of `implementation-plan.md`. Feeds spec edits to `lasa-core.md` §4.4–§4.5
and implementation work in `state/`, `server/`, and panaudia-server's
backend. Reached by stepping back from "which collision cases evict"
to "what an id *is*."*

## Decided 2026-08-13: ids are capabilities — title and exercise

An id is a **capability**. Two distinct facts about it were previously
conflated under the one word "claim":

- **Title** — the right to be client `c` and to instantiate entities
  `e1…en`. In a ticketed space, title is granted by the issuer inside
  the signed ticket and holds from `nbf` to `exp`, entirely independent
  of any connection. Nobody acquires an id at admission; the issuer
  already assigned it. Identity is the signed `client_id` claim, not
  the JWT bytes — a renewed ticket carrying the same `client_id` is the
  same capability with fresh title. Entity capabilities are subordinate
  to the client capability that carries them.
- **Exercise** — a connection currently acting under a capability.
  Exclusive: at most one exercise per capability at a time. A
  connection exercises the client capability by connecting (zero-entity
  observers included) and entity capabilities by instantiating them.

The spec sentence "the claim lasts exactly as long as the connection"
is true of *exercise* only. It was written for unticketed mode, where
it remains the whole story (below); generalising it to ticketed mode
was the error that made the collision rules look like arbitrary policy.

## The collision rules, derived

One principle: **contests over the same capability resolve to the
newest exercise; collisions between different capabilities refuse the
newcomer.** Supersession severs the incumbent connection wholesale and
its sweep completes before the replacement's admission writes.

| Collision | Rule | Why |
|---|---|---|
| Ticketed, same `client_id` | newest wins (supersede) | same capability, same principal — a principal replacing its own stale exercise; not "eviction" in any hostile sense |
| Ticketed, entity id collides across different `client_id`s | refuse newcomer | two distinct capabilities naming one id — an issuer coordination error; neither outranks the other; you cannot evict someone else on entity grounds |
| Unticketed, any collision | newest wins (supersede) | possession only — same principal and hijacker are indistinguishable by construction, and dev mode (below) optimises for the restart loop |

Signed-beats-unsigned is **deleted**, not decided: mixed mode no longer
exists (below), so titled and untitled exercises can never meet.

The ghost problem — a crashed client's half-open session blocking its
own reconnect — is dissolved rather than solved: supersession makes
death-detection latency irrelevant. The ghost may linger undetected
indefinitely; it holds nothing the returning principal cannot take.
Reconnect latency is bounded by teardown work (milliseconds of sweep),
never by a timeout. Detection of ungraceful death is fundamentally a
*chosen* timeout, not a discovery — uplink silence is a near-instant
signal but cannot distinguish death from a recoverable blip, and QUIC
deliberately ignores ICMP — so no design may depend on fast death
detection. Transport idle timeout remains only as garbage collection
of uncontested dead sessions; clean closes (the engineered common
path — clients SHOULD close on quit/SIGINT; browsers already do on tab
lifecycle) make ghosts rare to begin with.

## Decided 2026-08-13: a space is ticketed XOR unticketed

A Space is **either ticketed or unticketed**, fixed at configuration.
In a ticketed space every connection needs a ticket (already so). In an
unticketed space a config presenting a ticket is **rejected** — a
client presenting a ticket believes it is getting signed identity;
silently admitting it as possession-based identity is a quiet
degradation that must fail loudly instead. There is no mixed mode:
little benefit, many issues (the signed-beats-unsigned adjudication and
the incumbent-ticketedness bookkeeping were the visible costs; the
rest were latent). The XOR moves from panaudia-server's env validation
down into `server.New` itself, so no embedder can recreate mixed mode.

**Unticketed mode is dev mode.** It is not a second identity system to
make coherent — it is the absence of one, tolerated for development,
demos and closed networks. Its one rule (newest wins) is chosen purely
for the crash-restart iteration loop: zero wait, zero tuning, and the
same observable behaviour as ticketed mode ("reconnecting with your id
always works immediately") via possession instead of title. Its
fragility is documented, accepted, and out of design scope: a blocked
client can hop ids, a prankster can boot the presenter. Anything real
uses tickets. The 2026-07-27 reject-newcomer rule and its id-hijack
rationale are hereby reversed — the threat model that motivated it is
declared empty for this mode.

## Decided 2026-08-13: the ledger lives at the state engine

*Amended later the same day (next section): the ledger as a distinct
component dissolves — claims become state **keys**, adjudicated at
the same actor. The serialization-point reasoning below stands; the
"extend `Register`" mechanism does not.*

Collision detection is a linearization problem: two connections
claiming one id at two entry points need one decider. A Space already
has exactly one serialization point — the state engine's actor, which
assigns the monotonic seqs — so the claims ledger goes there.
`engine.Register` becomes the claim point: it atomically claims the
client id and entity ids (today it ignores duplicates — the observer
blind spot), adjudicates by the rules above, and releases on
generation close. Not the KV vocabulary: no `lasa.client.{id}` key is
invented, core admission does not read profile keys, and the
`lasa.entity.{id}` marker remains what §7 says it is — the
consumer-facing *announcement* of existence, a consequence of
admission, never its gate.

What this buys:

- **The §4.4 ordering rule comes free.** Claim, sweep, and admission
  writes all serialize through one actor; "old sweep completes before
  replacement writes" is not a cross-component obligation.
- **Entry points never need global knowledge.** In today's single
  process the shell forwards claims in-process. In a future
  multi-entry-point topology, each entry point holds its own session
  to the Space core and every exercise it forwards is scoped to that
  session — entry point dies, its session drops, its exercises sweep,
  exactly as a connection's death sweeps its keys today. The hierarchy
  composes to any depth.
- **Release is structural, never temporal.** Exercises are
  session-scoped resources, the protocol's one release idiom (MoQ
  subscriptions die with the session; generations close on
  disconnect; connection-scoped keys sweep). No leases, no renewal
  protocol, no expiry races, no fencing tokens, no clocks anywhere in
  the ledger. Time appears only inside transports (idle timeouts),
  where it already lived and carries no new correctness burden.

One seam to cut now because it is cheap now and annoying later: claim
release and key sweep happen together in `Conn.Close` today. A future
sweep *grace period* (base profile §3 permits one) is about key
hygiene, not id ownership — claims must release at disconnect even if
keys sweep later. The engine should keep the two separable.

## Decided 2026-08-13 (later): claims are state keys — the ledger dissolves

Reached by holding the design against one architectural constraint:
**share-nothing entry points coordinate only through the Space-wide
planes that already exist** — audio through the engine, the KV state
machinery, presence messaging. Do not invent places to hold state
when we have them already. Under that constraint the ledger as a
component was a stealth fourth coordination surface; and the store is
*already* the authority for an entity's presence in the Space (the
existence marker). The ledger's contents decompose entirely into the
store:

- **`lasa.client.{id}`** — new connection-scoped key: the client
  claim record. Value **`true`**, minimal by choice, extensible
  later. Written in admission, swept at disconnect.
- **`lasa.entity.{id}`** — the marker's value becomes the **owning
  client id** (was `true`): the entity claim record and the ownership
  binding in one key, living exactly where the entity is.
- **Visibility:** client ids are names, not secrets. Entity ownership
  is readable by every participant (the marker rides the existing
  `lasa.entity.` read grant — including the linkage fact that several
  entities share one owner, accepted). `lasa.client.` is deliberately
  NOT in the audience read scope: permissible is not the same as a
  feature, and a connected-clients roster (observers included) is new
  observable surface deferred until wanted.
- **Adjudication is an admission-group primitive at the actor**, not
  a component API: apply a connection's claim + admission writes
  atomically, where supersession = close-the-incumbent-generation +
  sweep + apply, in one actor step (the §4.4 ordering rule and the
  review-resolutions §7 seq hazard both demand exactly this order).
  Supersession signalling is the closed generation itself — the
  incumbent's own state lifecycle, no new channel, the displacing-
  attach precedent generalized. The only *refuse* preconditions:
  a ticketed entity claim whose live marker names a different owner,
  and **capacity** — moved here too, so every admission refusal is
  issued from one place.
- **Claim keys sweep immediately at disconnect, and the sweep grace
  period is deleted** from profile §3 rather than exempted: once key
  presence means "id exercised," a grace period makes every entry
  point's replica authoritatively wrong for its duration, and with
  the marker-absent rule already hiding lingering subtrees the
  allowance had no remaining use case. Simpler to lose it.
- **Entry-point replicas are advisory.** Every entry point subscribes
  (server-side observer, read-rules bypassed) and holds a current
  collection of client ids, entity ids, and ownership — for routing,
  rendering, diagnostics. Never for adjudication: replicas lag;
  adjudication happens only where writes serialize.

Prices paid, this section: plane 2 grows one adjudicated-write
primitive (the store is no longer pure last-write-wins at admission —
contained, but real); core admission correctness rides
profile-materialized keys through profile-supplied builders (the
`AdmissionGroup`/`RulesFor` pattern extended — the layering softening
is recorded, chosen over a core-owned key namespace); entity→client
linkage is public forever (ids declared names); the grace-period
allowance is gone (restorable — reopen below).

Rejected in this pass, with prices: **the ledger as a component /
`Register` as claim point** (a fourth coordination plane in all but
name — the exact creeping-state failure the constraint exists to
catch); **a core-owned key namespace** (two vocabularies to document,
content-blindness amended for one feature the profile hooks already
cover); **ownership as log-attribution metadata** (invisible to
clients, but grows store internals with a who-wrote-this concept —
quieter creep, same disease); **ownership under the client subtree
with markers staying `true`** (kept clients blind to ownership — moot
once ids were declared public, and it puts the binding a hop from the
entity); **`lasa.client.` in the audience read scope** (roster
deferred, not refused); **grace-exemption instead of deletion** (an
exception rule guarding a feature nothing uses).

## Prices paid (recorded, accepted)

- **Dev mode is hijackable by design.** Newest-possession-wins is an
  id-takeover primitive on any network where an attacker can reach the
  space. Accepted: the mode's threat model is declared empty, the
  fragility was already recorded ("do not run on open networks"), and
  the alternative (refuse + tuned idle timeout) taxes the restart loop
  the mode exists to serve.
- **Two devices sharing one ticket fight** — newest wins, ping-pong.
  Reaffirmed (the 2026-07-27 decision already accepted this); the
  multi-device story remains more tickets.
- **Issuer entity-id coordination errors refuse rather than
  self-heal.** A ticket whose entity id collides with another client's
  live entity is turned away with a valid ticket in hand. Id
  coordination is the issuer's job; the protocol will not boot a
  bystander to paper over it.
- **Mode mismatch is a hard failure in both directions.** A
  ticket-bearing client cannot fall back to open admission; fail-loud
  chosen over convenience.
- **The state engine takes on an admission responsibility.** Claims
  are not state, yet the ledger lives in the state engine — accepted
  because the engine is the Space's serialization point and the
  ordering guarantee is free there; the alternative homes (below) all
  re-create that guarantee by hand.

## Rejected alternatives, with their prices

- **Refuse on every collision (uniform).** Ticket-pinned ids make
  reconnect-collision the *normal* path in ticketed mode (signed ids
  cannot rotate — that is what makes moderation launder-proof), so
  refuse-all taxes every ungraceful reconnect with the
  death-detection latency (transport idle timeout, ~30 s default,
  tunable only by trading false session deaths) — and discards unused
  the one proof the ticket provides. Its uniformity buys the deletion
  of the supersession path, which ticketed mode wants anyway.
- **The 2026-07-27 asymmetry (refuse unticketed, evict ticketed).**
  Principled when written; its dev-mode half defends against a threat
  model dev mode is now declared not to have, and pays for it in the
  iterate loop. Superseded rather than wrong.
- **Evict on every collision (deviation 2 as implemented).** Right
  about newest-wins, wrong at the one cell where standing is unequal:
  it boots another client's live entity on an issuer error. Also
  keyed on the entity conn-map only, leaving zero-entity sessions
  invisible — two live sessions could share a client id.
- **Leases / renewal on claims.** Time-based release: clocks,
  renewal cadence, expiry races, fencing tokens — a second
  adjudication axis reintroduced immediately after title had removed
  time from the story. Structural scoping does the whole job.
- **A separate shared registry service.** Second system, second
  failure domain, and the sweep-before-replacement-writes rule
  becomes a cross-system ordering obligation — the exact bug class
  the single actor eliminates.
- **State markers as the authority.** No client key exists and
  inventing one inverts the core/profile layering; sweep grace would
  couple key hygiene to id release; markers are written after Admit
  and swept in teardown, so check-then-claim races.
- **The shell's live-session registry as the authority.** Correct
  process-locally, and where the ledger's job sits today — but it is
  transport-plane state in one process; the multi-entry-point
  topology breaks it. The ledger belongs one layer down, at the
  serialization point that survives the topology change.
- **Liveness probes on collision** (evict only the provably dead).
  Async admission machinery to prove what title already proves in
  ticketed mode and what dev mode does not need.
- **Mixed mode.** Signed-beats-unsigned, incumbent-ticketedness
  bookkeeping, and a client base that cannot tell which identity
  regime it is in. Deleted.

## Induced edits

Spec (`lasa-core.md`):

- **§4.4 rewritten**: space mode XOR (ticket required / ticket
  rejected per mode); collision rules restated as the table above;
  signed-beats-unsigned removed; the sweep-ordering sentence stays.
- **§4.5**: reconnect restated as re-exercise — "every connection is a
  fresh connection" stands, and gets its reason.
- **Error codes**: `0x1A5A03` (unticketed live collision) retires —
  no unticketed collision rejects any more. `0x1A5A04` (entity
  collides with another client's live entity) remains, now
  ticketed-only. New code for ticket-presented-to-unticketed-space —
  lean: a fresh code (mode mismatch), not `0x1A5A01`, because the
  ticket may be perfectly valid.
- §3.1's binding derivation and the base profile are untouched;
  `ticket.schema.json` is untouched.

Base profile (`lasa-base-profile.md`), per the claims-are-keys
amendment:

- **§6**: `lasa.entity.{id}` value becomes the owning client id; new
  `lasa.client.{id}` row (`true`, server only, connection-scoped) with
  claim-record semantics; audience read scope unchanged
  (`lasa.client.` deliberately excluded).
- **§3**: sweep grace period deleted; claim keys stated as sweeping
  immediately at disconnect.
- Core §8 gains the ids-are-names-not-secrets statement.

Implementation — sequenced in **`identity-implementation.md`**:

- **`state`**: an adjudicated admission primitive on the actor
  (supersede same-client atomically as close+sweep+apply; refuse
  cross-owner entity claims in ticketed mode; refuse over capacity),
  unified with the client-id-keyed displacing-attach identity;
  claim keys swept at disconnect (no grace seam).
- **`server`**: mode XOR enforced in `New`; `0x1A5A08` on
  ticket-in-open-space; supersession's transport-side teardown wait
  in the shell (mixer ordering), off the SPI.
- **panaudia-server**: `backend.Admit`'s collision scan, done-channel
  waits and capacity check all deleted (capacity now adjudicated at
  the actor); conn-map remains as pure mixer bookkeeping.
- **Tests**: `TestEvictOldSameClientReconnect` stays valid
  (newest-wins); `TestEvictOldEntityCollision` flips to expect
  refusal in ticketed mode and supersession in dev mode; new
  coverage: observer client-id collision (both modes), mode-mismatch
  rejections both directions, marker-value ownership, client-key
  lifecycle and invisibility to audience subscribers, capacity at
  the actor, and sweep-before-replacement-writes under supersession.
- **`implementation-plan.md` deviation 2**: closed — its behaviour is
  now decided (and corrected at the cross-client entity cell); its
  related note on the moderation gate's dev-mode id-weakness stands
  as recorded fragility.

## Reopen triggers

- **A multi-entry-point / relay topology becomes real.** Build the
  session hierarchy at the engine seam (entry-point sessions scope
  their forwarded exercises). No policy change expected — this
  decision was shaped so that only the transport of claims changes.
- **Anyone wants unticketed beyond a closed network.** Not a reopen:
  the answer is tickets. Recorded so the pressure has somewhere to
  land.
- **A real multi-device-per-ticket need appears.** Revisit the
  more-tickets story (issuer-side), not exclusivity.
- **A sweep grace period is implemented.** Verify claims release at
  disconnect while keys wait — the seam this doc asks the engine to
  keep.
- **Federated or multiple issuers per space.** Entity-id coordination
  across issuers may make refuse-on-issuer-error bite; the fix would
  be namespacing entity ids under the client capability in the spec,
  not adjudication changes here.
- **Something wants a connected-clients roster.** Add `lasa.client.`
  to a read scope (a role grant, not a mechanism change) — the keys
  already exist.
- **`lasa.client.{id}` wants a richer value.** Extend from `true`
  deliberately (each field added is published vocabulary); the key's
  claim-record role must not depend on the value.
- **Something wants lingering keys after disconnect** (UI smoothing,
  reconnect diffing). Grace was deleted for lack of a use case, not
  on principle — restoring it requires re-solving claim-key
  exemption, which is why the use case must come first.
