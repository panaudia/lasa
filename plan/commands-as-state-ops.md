# Commands become state operations — decision

*Decided 2026-07-21. LASA v2 carries **no command track**. Everything a
command did is expressed as a direct `set`/`clear` on a reserved key of the
state store.*

## The decision

The previous design had a separate client→server track for explicit
commands. Every command on it had an exact single `set` or `clear`
equivalent. That track is dropped: clients act by writing state, and the
reserved-key namespace (already introduced in `lasa2.md`) is the command
vocabulary.

## Why

The decisive argument is not "fewer message types" — it is that **every
piece of machinery a command track needs already exists for state, or
would have to be duplicated**. A shared spec must say, for commands: how
they order relative to state, what happens to one in flight at disconnect,
how they are authorized, how acceptance is confirmed, and what conformance
looks like. The state track answers all five today:

- **Ordering** — seq assignment gives a total order, so command effects
  serialize with the state they affect (a separate track never had this).
- **Reconnect** — commands-as-writes have no existence apart from their
  effect, so there is no "was my command lost in the gap" ambiguity; the
  catch-up machinery delivers the result.
- **Authorization** — role rules on key writes; one permission system.
- **Acknowledgement** — the echo (see the write-echo note in `lasa2.md`).
- **Conformance** — the state test surface already exists
  (`state-reconnect-and-gc.md`).

Secondary arguments: when a command is exactly one state write, the
command layer is indirection, not abstraction — dropping it makes the wire
fact and the truth of the system the same thing (the declarative,
Kubernetes-style model, familiar to implementers). And the door is
asymmetric, as with the fixed subscription set and the deferred
prefix-clear: **adding** a command track later is additive and breaks
nothing; **removing** one later breaks deployed clients. Minimal-first is
the reversible position.

The documentation burden does not shrink — it moves. The spec documents a
vocabulary of reserved keys instead of a vocabulary of commands. That
namespace is now the protocol's control surface, and deserves the design
care that command messages would have received: paths, payload types, and
which roles may write each key. (Future work; not yet designed.)

## Prices paid (recorded, accepted)

- **No error channel.** A command could be rejected with a reason; a state
  write is silently filtered — "no echo = didn't happen", per `lasa2.md`.
  Accepted because roles are known ahead of time (a client should not
  offer UI for actions its role forbids, so silent failure is confined to
  races), and because a nack message can be added additively later if it
  proves needed.
- **The 1:1 mapping is a load-bearing premise.** The decision was taken on
  the stated fact that every current command is exactly one `set` or one
  `clear`. See the reopen trigger.

## Reopen trigger

**The first command that is not exactly one `set` or one `clear`.** The two
shapes to watch for:

- **Allocation-shaped** — the server must choose the value ("assign me a
  slot"). A client write cannot express it.
- **Multi-key atomic** — state ops cannot express it: groups are
  explicitly hints with no consistency guarantee.

Forcing either through the KV means request-key/response-key conventions —
RPC reinvented over the store, worse than the command track this decision
removed. If such a command becomes real, reintroduce a command track
(additive) rather than bending the state model.

*(2026-07-27: the working notes that stood here — the scope-mapping
scribbles and the old command-name inventory — are superseded by
`lasa-state-keys.md`, which now carries the full vocabulary with
scopes and per-role write access.)*
