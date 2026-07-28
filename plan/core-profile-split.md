# Core / profile split — decision

*Decided 2026-07-21. LASA is layered: a meaning-agnostic **core** and
prose-specified **profiles**. The core reserves exactly one key prefix —
`lasa.` — and attaches no meaning to any key.*

## The decision

- **Core LASA**: the media tracks and the state *machinery* — ops, seqs,
  cursors, catch-up, frontiers, prefix subscriptions, and the
  role-filtering *mechanism* (that ops are filtered; the visibility gate).
  The core is content-blind: it must function with an **empty
  vocabulary**. The moment the sync machinery requires a profile key to
  operate, the layering is fiction — this is a stated conformance
  discipline, one sentence in the core spec.
- **Profile layer**: the key vocabulary (the operation set that replaced
  the command track — see `commands-as-state-ops.md`), the role
  *definitions*, and key lifetime semantics, bundled as one unit. These
  are mutually meaningless apart: a vocabulary without who-may-write is
  unusable; roles without a vocabulary have nothing to protect. Profiles
  are **prose companion specifications**, not wire artifacts.
- **Namespacing is by key prefix** — the core's native scoping primitive
  (subscriptions, role rules, and watermark QoI are already
  prefix-shaped), so a profile is, almost literally, a claim on a prefix
  plus the rules inside it. Nothing new is added to the core to support
  it.
- The core reserves **`lasa.` — with the trailing dot** — for the
  first-party **base profile**. All other keys are application space.
  The dot matters under plain string-prefix matching (pinned in
  `state-reconnect-and-gc.md`): reserving bare `lasa` would swallow an
  application key like `lasafoo`.
- The **base profile ships as a normative companion spec**. This is the
  fragmentation mitigation: a generic core with a fully optional
  vocabulary invites every adopter to reinvent mute and kick; a
  batteries-included default that deployments claim by name prevents
  accidental fragmentation while leaving replacement open.
- **No registry, no third-party convention.** Other adopters use their own
  prefixes; collision within a deployment is that deployment's
  coordination problem, as it always was.

## Rejected: self-description and wire discovery

The protocol is **not self-describing** and has **no profile discovery
mechanism**. Machine-readable self-description serves machine-mediated
late binding — a client learning at runtime how to talk to a service it
has never seen — and that consumer essentially does not exist (the WSDL
lesson: real clients are written by humans reading prose specs, and the
self-description layer becomes a second source of truth that drifts from
the first). Clients know what they are connecting to out-of-band, as with
HTTP APIs, Matrix, and MoQ streaming formats.

If the base profile ever wants to advertise a version, it puts a version
key **inside its own namespace** — self-hosted, zero core surface, and
entirely the profile's business.

## Why the split (evidence, not taste)

The state-sync design (`state-reconnect-and-gc.md`) was designed,
reviewed, and closed without a single rule referencing what any key
means — cursors, deltas, frontiers, GC are all content-blind. When a
whole layer of machinery completes without needing the vocabulary above
it, the dependency structure has located the seam; the spec structure
should mirror it. Secondary: the vocabulary will iterate much faster than
the sync machinery, and separate documents let the core freeze while
profiles churn. The door asymmetry also favours starting layered:
extracting a vocabulary from a monolithic spec after adoption is a
breaking reorganisation; merging an unearned layer later costs almost
nothing.

## Prices paid (recorded, accepted)

- **Profile agreement is out-of-band.** Two deployments can be
  wire-compatible at the core and mutually useless at the vocabulary
  level, and nothing on the wire says so. Accepted: this is true of every
  layered protocol cited above, and the base profile's existence keeps
  the common case common.
- **The reference server is a profile consumer.** Its mixer reading a
  mute key is the reference deployment implementing the base profile —
  permitted; core machinery depending on that key is not. The line must
  be policed in the codebase, not just the spec.

## Resolved 2026-07-27: connection-scoped keys are profile/server behaviour

The core gets **no lifetime mechanism**. "Connection-scoped" is profile
prose: the base profile documents which of its prefixes are swept on
disconnect, and the sweep itself is the server issuing **ordinary
`clear` ops** — seq-assigned, groupable, compacted, tombstoned,
watermarked and echoed exactly like any other write. On the wire a
departure sweep is indistinguishable from any other batch of clears,
and that is the point: every piece of machinery the sweep needs already
exists, so a core mechanism would duplicate rather than enable — the
same argument-shape that removed the command track
(`commands-as-state-ops.md`).

The observation that settled it: lifetime attaches to key *prefixes*,
but binding a prefix to a particular connection (this entity's keys ↔
that connection) is profile knowledge — the core has no concept of
ownership. A core mechanism would have to add one: ownership/claim
registration at connect, contention rules for competing claims,
reconnect and grace semantics. Real surface, no current consumer.

What stays freely tunable because it is server behaviour, not protocol:
sweep timing (immediate vs a grace period across brief network blips),
batching, and ordering. The profile already owns the atomicity
primitive: the `lasa.entity.{id}` existence marker's
ignore-when-absent rule means readers behave correctly even mid-sweep,
provided the marker is cleared first or in the same group — a base-
profile SHOULD, to be written there.

**Prices paid (recorded, accepted):**

- **Application-space ephemeral keys are not self-service.** An
  application wanting its own prefixes swept needs server
  configuration — deployment coordination. Accepted: profile agreement
  is already out-of-band (see above), and no third-party consumer
  exists.
- **Nothing on the wire marks a clear as a sweep.** Subscribers cannot
  distinguish "departed" from "deleted". The existence-marker pattern
  carries that meaning where it matters.

**Reopen trigger:** real third-party demand for client-declared
ephemeral keys. The additive path is recorded: an ephemeral-prefix
declaration in the connect/subscribe message, claimable only within
what role rules let the client write. Adding it later breaks nothing;
a core mechanism added now would be surface maintained forever — the
doors are asymmetric in the usual direction.
