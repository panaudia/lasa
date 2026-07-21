# State reconnect, catch-up & tombstone GC — design

Design notes for how the LASA state store handles client reconnection,
incremental catch-up, and garbage collection of tombstones. Settles the
questions left open by the `State` section of `lasa2.md`. This document is
about the **protocol**: what LASA promises clients and what any conforming
server must therefore maintain. How a server implements it (data structures,
locking) is deliberately out of scope — see the end.

*Updated 2026-07-20: the three formerly-open questions (prefix subscriptions
vs replace-all, TTL sizing, visibility changes vs the watermark) are
resolved, and the design gains epochs, the frontier message, and the
live-first delivery model. Rejected alternatives are recorded inline and in
"Alternatives considered".*

*Updated 2026-07-21: the parked question (per-subscription cursors &
subscription-set changes) is resolved. The state subscription is a single
track whose prefix-set filter is fixed at subscribe time; the persisted
cursor gains a set-hash; dynamic subscription changes are deferred with a
recorded reopen trigger. See "The subscription set is fixed at subscribe
time" and the updated "Alternatives considered". Later the same day the
three remaining parameter choices were resolved — TTL default 6 hours,
TTL/watermark not advertised, set-hash is SHA-256 — leaving no open
items.*

*Later still, a final sanity review tightened the spec rather than the
design: cursor–store coherence, the resurrection trap generalized to
deltas, ahead-of-head cursors, plain string-prefix matching, canonical
dedupe, absent-field semantics, the epoch now riding the frontier message
(renamed from "boundary"), epoch bumps terminating subscriptions, and the
write-echo caveat added to `lasa2.md`.*

## The problem

The state store is a distributed KV with two ops, `set` and `tomb`, each
given a monotonic server-assigned sequence id. Clients may disconnect and
reconnect, and should be able to resume without replaying the world. But
tombstones cannot be kept forever: a long-lived space accumulates deletions
without bound. GC of tombstones and resumability are in tension — a client
that reconnects from before a discarded tombstone would never learn of the
deletion and would hold a stale key forever.

A further requirement shapes the whole design: **this mechanism will be used
across widely differing data sizes, value sizes, and op frequencies.** The
protocol must remain correct and performant across those regimes, not just
in the current deployment's traffic shape. This is what rules out designs
whose costs are denominated in op count (see *Alternatives considered*).

## The design: compacted deltas

### The client keeps one cursor: (epoch, seq)

The client persists a single **cursor**: an `(epoch, seq)` pair — the
server's epoch and the highest sequence id it has applied. Nothing else —
no timestamp. On reconnect it presents the cursor and the server decides
what happens next. An earlier draft had the client save a (seq id,
timestamp) pair and judge its own staleness; that duplicates the decision
in two places with two clocks, and the server has to enforce the check
anyway. Client clocks are unreliable; sequence ids are exact.

**Epoch.** The seq space is only meaningful while the server can prove its
continuity. The **epoch** is an opaque value, carried on every frontier
message (see *The frontier message*) and compared **for equality only** —
never ordered, never interpreted. The
client's sole rule: presented epoch ≠ server's epoch → the cursor is void →
full resync. The server mints a **new epoch whenever seq-space continuity is
unprovable**:

- crash/restart of an in-memory store;
- restore from persistence to an earlier point — a rewound seq space with
  the old epoch would leave clients holding cursors *ahead* of the server's
  head, which is worse than any stale-tombstone bug;
- migration/failover of a space between instances in the decomposed
  deployment.

Equality-only semantics is what makes the epoch free: a random value or a
startup timestamp both work, and neither requires the server to persist
anything — a monotonic epoch counter would need persisted state, quietly
reintroducing the dependency the epoch exists to remove. The client must
persist the pair **atomically**; a seq without its epoch is meaningless.

**Cursor–store coherence.** The cursor attests *"I hold the effects of
every op ≤ seq."* A client that persists state must therefore never let
the persisted cursor get **ahead of** the persisted store: values,
per-key seq ids and cursor must be written so the cursor is at or behind
the store it describes. Cursor-behind is safe — over-delivery, absorbed
by dedup. Cursor-ahead is silent, permanent data loss: every reconnect
gate passes and the missing keys are never detected. A client that does
not persist its store must not persist a cursor; a fresh process presents
cursor 0.

*Rejected: wrap-inference instead of an explicit epoch* ("seq jumped
backwards by more than a max offset ⇒ assume restart"). A heuristic that
fails silently under exactly the conditions it exists for — long partitions,
delayed replays, restores. And with a 64-bit seq the wrap it guards against
is a non-event (~585,000 years at 1M ops/sec). The explicit field costs a
few bytes per *connection*, not per op.

### One cursor, one subscription; resync is all-or-nothing

A client holds exactly **one** state subscription: a single track whose
filter is a **set of key prefixes**, fixed at subscribe time (next
section). Scope is the union of the prefixes — a key matching more than
one prefix is delivered once — so overlapping prefixes are permitted and
benign: under one cursor and one cut there is no pairwise reasoning
surface for an overlap to create.

The client keeps **one** cursor for the track, and the reconnect gate
applies to the client as a whole. A failed gate forces a resync of
**everything the client subscribes to, answered as one snapshot taken at
one cut**.

That last clause is the load-bearing invariant. Multiple snapshots taken at
different cuts, interleaved with live delta streams, create a pairwise
reasoning surface (which snapshot wins for a key in an overlap? which cut
guards which deletion?) that is hard to hold in the head and harder to
test. One snapshot at one cut collapses it: one authoritative key set, one
head cursor, one deletion guard. A server that implements resync-all as
per-subscription snapshots at differing cuts silently reintroduces the
problem.

*(Historical note: earlier drafts had clients holding several
independently prefix-filtered subscriptions sharing a min-cursor; the
arguments that made one cursor cheap in that world — over-delivery
absorbed by per-key dedup, staleness correlated across subscriptions by
the common-cause disconnection gap — are preserved with the rejection of
per-subscription cursors under* Alternatives considered*. The one-track
decision dissolves the question: there is nothing to take a minimum
over.)*

### The subscription set is fixed at subscribe time

*Resolved 2026-07-21 (this section settles the TO REVISIT block that
previously closed this file).*

The prefix set is declared once, in the subscribe message, and is
**immutable for the life of the subscription**. There is no mid-session
add or remove of prefixes in v1. A client that wants a different set
terminates its state subscription and re-subscribes — the same path as a
mid-session role change — presenting whatever cursor it holds.

**The set-hash check.** The subscribe message carries the requested prefix
set (the server needs it to filter regardless), the cursor `(epoch, seq)`,
and a **hash of the set the cursor was earned under**. The server hashes
the requested set and compares: mismatch → the cursor is void → snapshot.
The client's persisted unit therefore grows to **(epoch, seq, set-hash)**,
atomic. The hash is stamped when the cursor is earned and must not be
recomputed at reconnect — recomputing it from the current set would make
the check pass vacuously. It is a **bug-catcher, not a security gate**: a
client that misstates its earned set corrupts only its own store; role
rules still bound what it can see. It is deliberately server-enforced
rather than left as a client-side MUST — a few bytes per connect buys a
conformance-testable rule instead of trusted client hygiene.

The blunt rule — *any* set change voids the cursor — deliberately
collapses widening, narrowing and reshuffling into one case:

- **Widening is the correctness case.** The cursor attests only to what
  the old filter let through; keys under a new prefix that changed before
  the cursor were never delivered, so no delta over the new scope is
  provable — the same shape as a role widening.
- **Narrowing needs no drop-local-keys rule.** Replace-all against the new
  set's snapshot removes out-of-scope keys for free.

Three matching/encoding decisions are pinned now because divergence here
is silent:

- **Prefix matching is plain string prefix.** `foo` matches `foobar` as
  well as `foo/bar`. Keys are strings and the path structure is
  convention (`lasa2.md`), so matching does not special-case `/` —
  segment-aligned matching would import path semantics the store
  deliberately does not have. A subscriber that wants segment behaviour
  subscribes to `foo/`.
- **Canonical set encoding + named hash.** The hash is **SHA-256** over a
  canonical form: prefixes **deduplicated**, sorted, each length-prefixed
  (so `["a","bc"]` and `["ab","c"]` cannot collide, and `["a","a"]` ≡
  `["a"]`). SHA-256 is chosen for ubiquity alone — collision resistance
  is beside the point for a bug-catcher. The same canonical list rides
  the subscribe message.
- **The empty cases.** `[""]` — the set containing the empty prefix — is
  the subscribe-to-everything idiom. An empty *set* is a protocol error,
  not a subscription to nothing.

**Why fixed rather than dynamic: the doors are asymmetric.** A fixed-set
v1 is additively upgradeable — dynamic changes can be layered on later as
new subscribe-time rules, and the standing "server MAY always answer
snapshot" rule keeps any such addition backward-compatible with deployed
clients. Per-subscription cursors — the alternative that handles set
churn natively — put subscription identity and N cursors on the wire and
into persisted client state, and removing them later breaks deployed
clients. Deferring is the reversible position.

**Reopen trigger (recorded).** Dynamic subscription changes become worth
designing if a churning-subscription regime becomes real — region-keyed
spatial interest management (clients subscribing to area-scoped prefixes
that change as they move) is the canonical case. As of 2026-07-21 this is
out of scope: dynamic hierarchical clustering of entities lives in the
sibling `clustard` project, and nothing from it beyond entity location —
already carried by the presence track — is expected to reach this
protocol. If that changes, the priced menu is under *Alternatives
considered*: the filter-widening gate (cursor valid for filter ⊆ earned
filter), the split response, and per-subscription cursors with a
disjointness rule.

### Catch-up serves a compacted delta

A reconnect within the window is answered with a **compacted delta**: for
every key whose latest op has seq id > cursor, the latest op only — a `set`
if the key lives, a `tomb` if deletion was the most recent operation. The
client does not see intermediate history.

This is the load-bearing choice. Delta cost is proportional to **changed
keys** (bounded by state size), not to **op count** since the cursor. A key
written 50 times during a disconnection costs one op to catch up, and a
high-frequency writer cannot inflate reconnect cost, server memory, or
anyone else's catch-up.

Consequences:

- The store must retain, per key, the latest op and its seq id. (Per-key
  seq ids are needed anyway for the client's dedup rule.)
- **Tombstones are real state.** Unlike `set` ops — whose latest value is
  retained indefinitely, so no information is ever lost — a tombstone is
  the only record that a key was deleted. It must be retained long enough
  for reconnecting clients to learn of the deletion. Hence TTL, flush, and
  watermark below.
- **Groups may arrive partial.** Compaction keeps only each key's latest
  op, so a delta may contain some members of a `group` and not others.
  `lasa2.md` already defines groups as a hint with no consistency
  guarantee; the spec should state explicitly that deltas do not preserve
  them.

### Delivery: live-first, ordered by the client

On any response — delta or snapshot — the server starts delivering **live
ops immediately** and streams the catch-up payload lazily alongside. The
server **never coordinates the two paths.** This is a deliberate transfer
of responsibility: the server saves per-client buffering, cut-and-hold, and
any coupling between snapshot generation and the live write path (snapshots
can be built from immutable segments and delivered at whatever pace
backpressure allows); the client pays with a small set of ordering rules.

The client rules:

1. Apply live ops immediately, under the usual per-key seq dedup.
2. Apply a catch-up entry — delta and snapshot alike — only if its seq >
   the local per-key seq.
3. When a snapshot completes (see *frontier message*), delete every local
   key in scope that is absent from the snapshot **and whose local seq ≤ H**,
   where H is the snapshot's cut cursor. Rule 3 is **snapshot-only**:
   absence from a *delta* means unchanged, never deleted.

Rule 3's guard exists because a snapshot carries no seq for *absent* keys:
without it, a key created live at seq 1005 against a snapshot cut at 1000
would be wrongly deleted when the snapshot finishes. The guard is needed in
any design where live ops and a snapshot share a wire — it is not an
artifact of multiple subscriptions.

**The resurrection trap.** During the catch-up window a live `tomb` can be
followed by a late catch-up chunk — **snapshot or delta alike** —
containing the same key at an older seq; naive application resurrects the
deleted key. (The delta case is easy to miss: the cut H precedes every
live op, so a delta may legitimately carry an older `set` for a key
tombed live after the cut.) Two conforming client strategies handle this:

- **Buffer-and-replay** — buffer live ops during catch-up delivery, apply
  the catch-up payload, replay the buffer under dedup, run rule 3
  (snapshots only), go live. No
  resurrection hazard exists at all; the rules are hard to implement wrong.
  Cost: the client's view of live activity is frozen for the window.
- **Apply-live with transient tombstones** — apply live ops immediately;
  record `(key, tomb-seq)` for **every** live `tomb` during the window —
  including tombs for keys the client does not currently hold (the created-
  at-980, tombed-at-1005, chunk-arrives-late case) — and drop the records
  at the frontier. Keeps the client fully live during a possibly
  seconds-long window; carries the subtle rule.

The wire exchange is identical, so the choice is out of protocol scope. The
spec should carry a **non-normative note describing both** (the transient-
tombstone trap will otherwise be rediscovered the hard way by every
implementer). The first-party client prefers transient tombstones: in a
live spatial-audio context, applying a mute or kick seconds late is a worse
artifact than the client-side complexity.

### The frontier message

*(Renamed 2026-07-21 — previously the "boundary" message. Frontier is the
dataflow term of art for "you will see nothing below X", and avoids
overloading both "watermark" — already the tombstone gate here, and the
term Flink/Beam use for this very concept — and the generic "boundary".)*

Every catch-up response — delta or snapshot — **closes with an explicit
frontier message**: "you will receive nothing further below X" for the
subscription. X is the **cut cursor**, transmitted explicitly, **never
derived from content**. The distinction matters under filtering: role or
prefix rules can hide the key holding the true head op, so max-seq-present
understates the cut. Derived-H fails in both directions — a stale key
spared forever by an understated deletion guard, or a current client
force-resynced later because its persisted cursor understated its real
position.

The frontier carries the pair **(epoch, seq)** — it is where the client
learns and refreshes the server's epoch, so the client's persisted unit is
exactly *last frontier + set-hash*, with no second channel for the epoch
to disagree with.

The frontier serves four roles:

1. the explicit **H** of a snapshot;
2. the explicit cut of a delta;
3. the end-of-window marker that lets clients drop transient tombstones or
   replay buffers;
4. emitted **periodically during live operation**, it keeps every connected
   client's cursor at head regardless of what it can see. Without this,
   cursor staleness accrues *during* a connection whenever a client's view
   is filtered, and a quiet or heavily-filtered client eats a false resync
   on its next reconnect.

A frontier is a **promise**, with two soundness conditions:

- **Ordered, reliable transport only.** State rides MoQ streams today, so
  the promise is enforceable per-stream. Recorded constraint: state ops can
  never migrate to datagrams without revisiting this mechanism.
- **Server discipline.** The frontier must be computed against the same cut
  that governs what was actually sent; emitting X while a straggler below X
  is queued on another internal path silently corrupts clients. This is a
  prime conformance-test target (see below).

### Tombstone TTL, flush & watermark

Three protocol-level concepts, one per role:

- **TTL** — how long a tombstone remains replayable after it is written.
  **Decided: a fixed wall-clock duration, configured on the server** — so
  the reconnect window is time-denominated and independent of traffic rate.
  A server parameter, not a protocol constant; clients must not assume any
  particular value. **Default: 6 hours** (set 2026-07-21 — comfortably
  above the mobile-backgrounding floor, well under the wasted-beyond-a-day
  ceiling; revisit against measured gap distributions per *Sizing* below).
- **Flush** — the periodic pass that discards tombstones older than the
  TTL. How and when a server runs it is an implementation concern; the
  protocol only requires that discards are reflected in the watermark.
- **Watermark** — the highest seq id of any tombstone ever discarded. This
  is the *correctness gate*: it marks the oldest cursor for which a delta
  is still complete.

The watermark is tombstone-specific by construction: `set` ops never age
out (the latest value per key is always in the store), so tombstones are
the only ops whose disposal loses information — and therefore the only ops
whose disposal must advance the gate.

**Sizing.** Err long: a compacted tombstone is key + seq + map overhead, so
memory is deletion-rate × TTL and cheap — and in LASA the state store's
deletion churn is modest anyway (presence/pose ride their own datagram
tracks, not state; deletions are mostly departure sweeps of
connection-scoped keys plus explicit app deletes). The floor is set by the
gap regime where deltas are genuinely valuable and common — mobile
backgrounding, minutes to hours. The ceiling is the space's state
half-life: as the changed fraction approaches 1, a delta approaches
snapshot-plus-tombstones and resumability buys nothing. The default should
comfortably cover hours; beyond a day it is wasted, though cheaply.

**Economic snapshots.** Long TTLs introduce one non-memory cost: a delta
carries a tombstone for every key deleted since the cursor, while a
snapshot carries none, so for a long-gap client in a churny space the delta
can *exceed* the snapshot. The server **may answer snapshot on purely
economic grounds** (clients already must handle either response), so the
TTL is an upper bound on resumability, not a commitment to serve expensive
deltas.

**Per-class TTL trap (recorded so nobody adds it later).** Under the
single global watermark, the effective reconnect window is the *minimum*
over all key classes — flushing one short-TTL class's tombstone advances
the one watermark and kills everyone's resumability. Per-class TTLs only
pay alongside prefix-bucketed watermarks (server QoI, below), and given the
low deletion churn, likely never.

### The reconnect rule

Exact, server-enforced, checked in order:

1. **epoch mismatch** → full resync (the cursor is void).
2. **set-hash mismatch** → full resync (the cursor was earned under a
   different filter; see *The subscription set is fixed at subscribe
   time*).
3. **seq < watermark** → the client may have missed a discarded deletion;
   full resync.
4. **seq < the identity's visibility-change seq** (see *Visibility
   changes*) → full resync.
5. **seq > the server's head (same epoch)** → the cursor claims ops the
   server never issued — corrupted or incoherently-persisted client
   state, and a delta cannot be answered (its frontier would sit below
   the presented cursor) → full resync.
6. otherwise → compacted delta — though the server **may** still answer
   snapshot (economic grounds, or any completeness it cannot prove).

A fresh client presents nothing: absent epoch, seq and set-hash fail
rules 1–2 by construction (an absent value never equals a server value),
so a fresh connect lands on snapshot with no special case.

The response is explicitly one of **delta** or **snapshot**; the client
never guesses which it will get, and must handle both. Stated as a minimum
guarantee: the server MUST snapshot when it cannot prove the delta
complete, and MAY use any finer-grained bookkeeping to avoid unnecessary
snapshots — such bookkeeping is invisible to the protocol.

### Full resync means replace-all

On full resync the client **discards its local store entirely** — across
all its subscribed prefixes — and adopts the snapshot wholesale. It does
not merge. Therefore:

- **Snapshots never contain tombstones.** A deletion only has to be
  communicated to clients who might still hold the key; a replace-all
  client holds nothing.
- Snapshots must carry **per-key seq ids** and close with the frontier
  message carrying the cut cursor **H**, so the dedup rule and the rule-3
  deletion guard keep working across a resync.
- Full resync is the same exchange as a fresh connect: a new client is
  simply a client with cursor 0.

Replace-all is a *semantic* contract, not a rendering instruction — a
client may implement it as a diff against its local store and only touch
keys that changed, avoiding UI flash. Under the live-first delivery model,
"wholesale" is realized by the delivery rules above (dedup + the ≤ H
guard), which make replace-all correct even with live ops interleaved.

### Visibility changes (roles)

Server-side role rules filter which ops a client sees, so a change in a
client's effective visibility invalidates delta completeness the same way a
flushed tombstone does — a newly visible key won't be in any delta. The
problem has a half that no mid-session trigger can reach: **roles can
change while the client is disconnected**, and the returning client
presents a perfectly valid cursor. Hence:

- **Subscribe-time gate (required in every variant).** The server tracks,
  per identity, the seq at which its effective visibility last changed;
  reconnect rule 4 forces a snapshot when the cursor predates it. Same
  shape as the watermark — a per-identity watermark. A server that loses
  this bookkeeping just answers snapshot.
- **Mid-session changes: terminate and re-subscribe.** The server
  terminates the affected client's state subscription(s) using native MoQ
  subscription termination; the client re-subscribes as after any transport
  event and hits the gate. Scoped to the state subscription only — the MoQ
  session and all audio tracks stay up; the user-visible effect is a brief
  state refresh. Net new LASA protocol surface: zero.
- **Global rule-set changes** (as opposed to one identity's role) may be
  implemented as an **epoch bump** — everyone resyncs through the existing
  mismatch path, sparing per-identity bookkeeping for a change that
  affected all identities anyway. An epoch bump alone is invisible to
  connected clients — the mismatch check runs at subscribe time — so it
  is accompanied by terminating all state subscriptions, the same
  mechanism as a per-identity change.
- **Narrowing (demotion) is handled free by replace-all**: the snapshot
  omits newly-hidden keys and the ≤ H guard removes them. This is
  compliant-client hygiene, not security — a client that saw a key cannot
  be made to unsee it.

*Rejected: an in-protocol mid-session resync message* (server pushes an
unsolicited snapshot on the live subscription). Buys a zero-gap refresh,
but costs a new message type and makes catch-up-window re-entry — the
subtlest part of the client — an any-time event rather than a connect-time
one. Role changes are rare, human-paced events; a sub-second state refresh
with audio uninterrupted is invisible in practice. Revisit only if some
other feature independently needs server-initiated resync (nothing in the
current design does — economic snapshots and epoch changes both occur at
subscribe time).

### Lifecycle of a deletion

1. `set foo=1` arrives as op 100 → store holds `foo → (value, 100)`.
2. `tomb foo` arrives as op 200 → store holds `foo → (tombstone, 200)`.
   The value is gone; the tombstone is retained state.
3. Client reconnects with cursor 150 → delta includes the tomb (200) →
   client deletes `foo`. Correct.
4. TTL elapses; a flush discards the tombstone; the watermark advances to
   at least 200.
5. The same cursor-150 client reconnecting after the flush fails the gate
   (150 < watermark) → full resync → snapshot contains no `foo` →
   replace-all. Still correct.

## Alternatives considered

### Log-window design (rejected)

Serve catch-up by replaying a raw op log from the cursor; tombstones live
only in the log and are discarded by ordinary log trimming, so no
tombstone-specific machinery exists at all — the log horizon is the gate.
Maximally simple, and correct.

Rejected because its costs are denominated in **op count**: delta size,
server memory, and per-reconnect work all scale with ops-in-window, and the
retention window is denominated in ops/bytes, so a chatty space silently
shortens the reconnect window in wall-clock terms. Serve-time deduplication
of the log fixes delta *size* but not memory or scan cost. For a mechanism
that must serve widely differing rates and sizes, costs should be
denominated in state size, which is what the compacted-delta design buys —
at the price of the tombstone TTL/flush/watermark machinery.

### Snapshot-only (rejected)

Drop the cursor and catch-up entirely; every reconnect is a full snapshot.
The smallest possible protocol surface — no cursor, no delta/snapshot
branching, no retention concepts. Rejected because reconnect cost becomes
proportional to space size rather than to what was missed, which fails the
differing-sizes requirement in the other direction (large state, brief
blip), and a mass-reconnect event becomes a herd of full snapshots.

### Per-subscription cursors (rejected)

Each prefix subscription is its own resumable session: (prefix, cursor)
pairs, per-subscription delta/snapshot decisions, blast radius of a failed
gate scoped to one subscription. The cleanest *semantics* — subscription =
(filter, cursor) — and it would have unified prefix filtering and role
filtering under one completeness rule.

Rejected on two grounds. First, multiple snapshots at multiple cuts
interleaved with live streams create the pairwise overlap-reasoning surface
described above; the per-rule fixes are cheap but the state space of
interleavings is expensive to reason about and test. Second, the blast-
radius benefit is small because staleness is correlated (common-cause
disconnection gap), while the cost — subscription-identity machinery in the
spec, N persisted cursors, N independent catch-ups — is paid always. The
variant with one cursor but per-prefix resync scoping was rejected for the
same multi-cut reason; its client-simplicity advantage is partly illusory
since per-stream progress must be tracked in memory regardless.

*Re-examined 2026-07-21.* The rejection was found to be conditional on an
assumption never made explicit: that a client's prefix set is small and
essentially static. Under churning subscription sets (spatial interest
management), per-subscription cursors handle prefix add/remove natively
while resync-all handles it badly. Resolution: make the assumption
explicit and true — the set is fixed per subscription (see *The
subscription set is fixed at subscribe time*), churn regimes are out of
scope, and this section is the recorded reopening menu if that changes.
Also recorded from the re-examination: with **disjoint** prefixes the
multi-cut objection largely dissolves — snapshots over scopes that share
no keys cannot contradict each other — so a reopened design would likely
pair per-subscription cursors with a per-client prefix-disjointness rule.
The second objection (N persisted cursors, subscription identity,
per-subscription gates — paid always, by every client) stands regardless.

**Watermark granularity, relegated to server QoI.** The residual cost of
the single global watermark is cross-prefix false resyncs. An honest
prefix-aware watermark needs the *keys* of discarded tombstones — but a
compacted tombstone is already just (key, seq), so keeping skeletons ≈ not
flushing, with memory landing exactly in the regime GC targets. Coarse
bucketed watermarks approximate it. Because the server may always answer
snapshot, all such bookkeeping is invisible to the protocol and can be
added later, within B's all-or-delta response shape, without any wire
change.

### Split response (recorded, not taken)

The middle design between "any set change voids the cursor" and
per-subscription cursors: keep the single cursor, and answer a widened
reconnect with a **delta for the retained scope plus a snapshot for the
added scope, both computed at the same cut**, closed by one frontier.
Notably this does *not* violate one-snapshot-at-one-cut — there is exactly
one cut — so it is cheaper than it first looks. Its real prices: a mixed
response shape in the spec; the rule-3 deletion guard becomes **scoped**
(absence deletes only within the snapshot's scope); and the server must
learn which filter the cursor was earned under — the set-hash carries that
only as a digest, so the client would have to declare the earned set
explicitly. First candidate if dynamic subscription changes are added
later and full-resync-per-change proves too expensive.

### Single-subscription-only (not taken)

Dissolve the question: one state subscription per session, one prefix
(possibly empty), multiple interests served by subscribing broader and
filtering client-side. Costs over-subscription bandwidth (state size ×
change rate of the broader prefix) and forfeits server-side filtering as a
bandwidth tool. Not taken, but recorded: if multi-prefix subscription turns
out to be rare in practice, this remains the cheapest position the design
could retreat to. *2026-07-21: partially adopted — v1 is
single-subscription, but with a multi-prefix union filter rather than a
single prefix; the full retreat (one prefix only) remains available if
multi-prefix filters prove unearned.*

## Spec implications for LASA v2

1. A client holds **one** `state` subscription: a single track whose
   filter is a **set of key prefixes** — plain string-prefix matching (no
   path semantics), union semantics, overlap permitted, `[""]` (the empty
   prefix) subscribes to everything, an empty list is a protocol error —
   fixed for the life of the subscription. The subscribe message carries
   the requested set, the cursor **(epoch, seq)** (absent for a fresh
   client; absent fields always fail the reconnect gate), and the **hash
   of the set the cursor was earned under**, computed over the canonical
   encoding (prefixes deduplicated, sorted, length-prefixed, SHA-256).
   The epoch is opaque and compared for equality only; the client persists
   **(epoch, seq, set-hash)** atomically — and never ahead of its
   persisted store (**cursor–store coherence**): a client that does not
   persist its store must not persist a cursor.
2. The response is explicitly **delta** or **snapshot**. A snapshot means
   *replace all local state across the client's subscribed prefixes*, and a
   resync is answered as **one snapshot at one cut**. The server MUST
   snapshot when it cannot prove a delta complete (epoch mismatch,
   watermark, visibility change) and MAY snapshot whenever else (e.g. when
   the delta would be larger).
3. Deltas are compacted: latest op per key, tombstones included where
   deletion is the latest op. Intermediate history is not delivered, and
   groups are not preserved in deltas.
4. Snapshots contain no tombstones and carry per-key seq ids.
5. Every response closes with a **frontier message** carrying **(epoch,
   seq)** — the explicit cut cursor and the epoch it belongs to; frontiers
   are also emitted periodically during live operation and are how the
   client learns the epoch. Clients never derive their cursor from
   content. Frontiers require ordered reliable transport — state ops
   cannot move to datagrams without revisiting this.
6. Delivery is live-first: the server does not sequence live ops against
   the catch-up payload. Clients order via per-key dedup, catch-up-entry-
   applies-only-if-newer (delta and snapshot alike), and the snapshot-only
   absent-key deletion guard (local seq ≤ cut cursor). A non-normative note describes the two conforming catch-up
   strategies (buffer-and-replay; apply-live with transient tombstones —
   recording **all** tombs during the window, including for absent keys).
7. The tombstone TTL is a fixed wall-clock server parameter, not a protocol
   constant (default 6 hours). The TTL and watermark are **not advertised**
   to clients; clients must not assume any particular reconnect window —
   they receive delta or snapshot and must handle either.
8. Visibility changes gate at subscribe time (per-identity visibility-
   change seq); mid-session changes are applied by terminating the state
   subscription via native MoQ termination. There is no LASA-level resync
   message. Global rule-set changes may bump the epoch.
9. The prefix set is immutable per subscription; changing it means
   re-subscribing, and any set change voids the cursor via the hash check
   → snapshot. There is no mid-session prefix add or remove in v1.

## Conformance test surface

The places where a wrong implementation stays quiet until production:

- catch-up chunks — delta and snapshot alike — interleaved with live
  `tomb`s (the resurrection trap, both client strategies);
- the absent-key deletion guard at the ≤ H frontier (keys created live
  during the window must survive replace-all);
- frontier promises under concurrent writes (no straggler below an emitted
  frontier, cut computed against what was actually sent);
- epoch mismatch handling, including a rewound-seq restore;
- a cursor ahead of the server's head with a matching epoch (must answer
  snapshot, never emit a frontier below the presented cursor);
- a role change applied while the subject is disconnected;
- a reconnect presenting a changed prefix set (hash mismatch must force
  snapshot; an equal set re-ordered or re-encoded must hash identically
  and must not).

## Out of scope (for now): server implementation

How the server realises this — the compacted store, flush scheduling,
watermark maintenance, per-identity visibility-change seqs, any prefix-
bucketed watermark QoI — is deliberately deferred. One constraint worth
recording so it isn't lost: the existing `core/statecache` write path is
**lock-free** (atomic slot claim + pointer store, mutex only on segment
advance), and it is desirable to keep writes lock-free under the
compacted-delta design. Whether and how that carries over is a refactor
question for another day.

## Remaining open items

None. The three parameter choices left open by earlier drafts were
resolved 2026-07-21:

- **TTL default: 6 hours.** Comfortably above the mobile-backgrounding
  floor, well under the wasted-beyond-a-day ceiling. A config default, not
  a commitment — revisit against measured gap-length distributions and
  changed-fraction-vs-gap curves once there is traffic, per the sizing
  logic above.
- **TTL/watermark are not advertised — at all.** Not even as a
  non-normative "may": clients get delta or snapshot, must handle either,
  and have nothing to plan around. The one imagined consumer (a mobile
  client deciding whether persisting its store to disk is worthwhile) can
  simply persist unconditionally — cheap and never wrong *provided
  cursor–store coherence is kept* (values + per-key seqs + cursor, cursor
  never ahead of the store), since a cursor past the window just resyncs.
- **Set-hash function: SHA-256.** Chosen for ubiquity alone; collision
  resistance is irrelevant to a bug-catcher, so being available everywhere
  is the only criterion that matters.

---

> [!NOTE]
> ## ✅ RESOLVED 2026-07-21: per-subscription cursors & subscription-set changes
>
> The TO REVISIT block that stood here is settled. Decisions:
>
> - The subscription set is **fixed at subscribe time** — single track,
>   prefix-set filter with union semantics, any set change voids the
>   cursor via a server-enforced **set-hash** check (see *The subscription
>   set is fixed at subscribe time*).
> - **Dynamic subscription changes are deferred**, with the reopen trigger
>   recorded in that section: churning-subscription regimes (spatial
>   interest management) becoming real. Currently out of scope —
>   clustering lives in the sibling `clustard` project and is expected to
>   reach the protocol only via entity location, already on the presence
>   track.
> - The reopening menu — filter-widening gate, split response,
>   per-subscription cursors with a disjointness rule — is priced under
>   *Alternatives considered*.
