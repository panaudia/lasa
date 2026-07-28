# State engine design

*Decided 2026-07-28; revised the same day after an adversarial review
pass (spec-consistency + edge-case hunt; findings folded in below).
Companion to `implementation-plan.md` (work-order context; carries the
consolidated spec-errata list) and to `lasa-core.md` §6, which this
implements. Records the concurrency/fan-out decision with rejected
alternatives, the write path, the delivery interface, the reconnect
gate, the invariants, and the smaller defaults.*

The engine is the transport-neutral implementation of core §6: the
store, seq/epoch machinery, the reconnect gate, two-lane delivery,
tombstone GC, and the disconnect sweep. It speaks structs, never wire
bytes. **One engine instance per space**; a process hosts many; seq
counter, epoch, GC, and the broadcast log are all per-instance, no
globals.

## 1. Concurrency and fan-out: actor + broadcast log

Two orthogonal choices, decided together:

- **Serialization: a single-goroutine actor** owns the store. All
  mutations — connection register/deregister, writes, subscription
  attach/detach, sweeps, GC ticks, rule-set changes — are messages
  consumed in order by one goroutine owning the map and the seq
  counter.
- **Fan-out: a bounded in-memory broadcast log.** On acceptance the
  actor assigns the seq, updates the compacted map, appends to the
  log, publishes the new head. Subscribers hold cursors into the log
  and consume at their own pace in their own goroutines.

Why the actor: total order falls out of message order, so seq
assignment is trivially correct; the sweep-before-replacement-admission
ordering rule (core §4.4) is just two messages in order; the
subscription-attach cut (§4) is a single event rather than a locking
protocol; and the loop can be driven deterministically in simulation
tests, which is where this component's hard bugs will be flushed out.
The actor never blocks on delivery — appending to the log and
publishing the head is the whole write-side fan-out cost, O(1) in
subscriber count.

Why the broadcast log (over per-subscriber queues): one append instead
of N enqueues; one shared window of ops instead of N duplicated
queues; per-subscriber filtering and wire encoding move into subscriber
reader goroutines, off the write path (safe because a subscription's
read rules are fixed at attach — mid-session visibility changes work
by terminating the subscription, core §6.6, so rules are captured once
and never consulted under contention). Log entries share the map's
immutable value bytes — the log is a window of small structs, not a
second copy of the store. It is a *broadcast log*, not a WAL: it exists
for live fan-out only and plays no durability role.

**Log sizing.** The log is bounded in **entries and bytes**. Shared
value bytes mean an overwritten map value stays pinned until its entry
leaves the window — worst case ≈ window × 64 KiB per space, which the
byte bound caps. Sizing invariant, tied to resync (below): **the log
window must cover max sustained write rate × worst-case catch-up
transfer time**, else an evicted client can never complete a resync.

**Overflow policy: eviction-on-lag.** A reader whose cursor falls off
the tail gets a distinct error (`ErrLagged`) and the transport
terminates *that state subscription only* (invariant L3, §6). This is
not an error hack — it is exactly the protocol's designed resync path
(the client re-subscribes, hits the gate, receives a delta or
snapshot), the same mechanism the spec uses for visibility changes.
Two guards keep the loop convergent rather than a livelock:

- the sizing invariant above — a resyncing client's fresh cursor is
  positioned at its cut H and ages while the catch-up streams; the
  window must outlast the transfer (the unguarded moment between the
  attach event and the reader's first pull is the same case at t = 0);
- client re-subscribe backoff in `clientstore`, so a client that
  cannot keep up degrades gracefully instead of hammering
  snapshot-sized attaches.

Rejected alternatives, with reasons:

- **Mutex-guarded map**: seq assignment and delivery ordering force
  holding the lock across the same critical section the actor owns —
  the same serialization with invariants by convention instead of by
  structure, and concurrent rather than deterministic tests.
- **Lock-free (statecache-style ring)**: unjustified. The ring earns
  its complexity for append/read-since semantics; LASA needs global
  order, per-key compaction, prefix scans, consistent cuts, and the
  gate. The workload is a coordination store at human rates, and the
  spec records that state can never migrate to datagrams without
  revisiting the frontier design (core §6.5) — so no hot path exists
  under the current design, and arriving at one reopens more than this
  choice.
- **Per-subscriber queues**: O(N) write path, N× op duplication, and
  an invented overflow policy where the log gets a protocol-native one.

## 2. Store representation

Plain flat map, per-key `{value, seq, tombstone}`. Prefix scans (the
attach-time catch-up computation is the only scan consumer) by full
scan. At coordination-store scale (≤512 B keys, ≤64 KiB values, human
write rates) this is adequate; revisit only if profiling says
otherwise. No retained op history beyond the broadcast log's window —
the spec guarantees compaction (core §6.5: intermediate history is
never delivered), so catch-up never replays ops; see also invariant
F4 in §6 below. Seq is uint64 — no overflow handling (centuries at any
plausible rate); epoch bump restarts it anyway.

## 3. Connections and the write path

*(Added in the review revision — the write direction was previously
undesigned, and its rules cannot ride subscription attach: write
grants are per-connection, and writes may begin before any state
subscription exists.)*

**Connection registration.** At admission the shell registers the
connection with the engine — one actor event carrying the connection's
identity (client id), its **write grants** (the merged
set/clear/value rules), and a fresh **generation id** for the
connection instance. Deregistration (departure) enqueues the sweep.
Write-direction enforcement therefore lives in the actor against the
registered grants, never in the transport:

- The silent-drop causes (core §6.2) are checked on every op:
  denied by rules; no matching write grant; value > 64 KiB. Plus one
  engine-added cause: an over-cap group (below). Dropped ops produce
  nothing — no seq, no log entry, no echo.
- **Generation gate (invariant L1, §6).** Every write message carries
  its connection's generation; the sweep event atomically closes that
  generation. Late ops from a departed connection — still sitting in
  shell channels or goroutines when the sweep ran — are silently
  dropped, never applied. Without this, a straggler write re-creates a
  connection-scoped key that no sweep will ever clear again, and it
  surfaces under any replacement connection's entity.

**Groups.** A group is applied as **one actor event** (invariant F6) —
its ops get consecutive seqs and no attach cut can land inside it. The
disconnect sweep is itself a group, so sweep atomicity follows. Group
size is capped at a server constant **well below the log window** —
the spec caps key and value sizes but not group op-count (errata
candidate), and an over-window group would evict every cursor in the
space at once, including freshly attached ones: a one-writer universal
forced-resync. Over-cap groups are silently dropped whole, like any
other invalid write.

## 4. Delivery interface

The engine's API to the transport shell — the surface the `server`
SPI builds on:

- **Attach** carries the subscription's prefix set, its captured read
  rules, the owning connection (client id + generation), and the
  presented cursor `(epoch, seq, set-hash)`. The actor runs the
  six-step gate (§5) *at the attach event*, so the verdict is atomic
  with the cut, and returns a `Subscription`: the gate verdict, a
  catch-up lane, and a live lane. Layer ownership for the inputs:
  `wire` parses the subscribe-KVP, rejects a non-canonical prefix
  encoding and the empty-set protocol error, and computes the
  canonical set-hash; the engine only compares.
- **One subscription per client (invariant L4).** Subscriptions are
  keyed by client id in the engine; Attach atomically displaces a
  live predecessor for the same client — which also resolves the
  re-subscribe race (old subscription dying of `ErrLagged` in its
  reader while the new Attach arrives; either order converges).
  Detach is idempotent and addressed by **subscription instance id**,
  never client id, so a late Detach cannot hit a successor.
- **Teardown (invariant L5).** Every subscription's lifetime is tied
  to a transport-close hook that guarantees Detach — the reader
  goroutine and registration must not outlive the QUIC session on any
  path, including crash/panic paths.
- **Catch-up lane**: an iterator over the delta or snapshot entries,
  computed from a copy of the in-scope map entries taken inside the
  attach event, terminating with the frontier cut. Snapshots carry no
  tombstones (spec); an empty delta still ends with its frontier.
- **Live lane**: pull-based consumption of the subscriber's log cursor,
  filtered through the captured read rules. Backpressure is cursor
  lag; overrun surfaces as `ErrLagged` — which terminates **the
  subscription object only** (invariant L3): it must never feed any
  connection-lifecycle path. In panaudia terms: `ErrLagged` never
  reaches the departure funnel; the sweep is reachable solely from
  connection departure. The two evictions in this design — admission-
  driven *connection* eviction (core §4.4, triggers the sweep) and
  lag-driven *subscription* termination (triggers only resync) — share
  a word and nothing else; the API keeps them as distinct entry
  points.
- **Structs, not bytes**: ops surface as (key, value, seq, tombstone,
  group boundary) values; all encoding lives in `wire`, applied by the
  shell. Group boundaries are recorded in the log so live delivery can
  honour the preserve-groups SHOULD.
- **Periodic frontiers** are injected into the live lane on a tick
  from an **injectable clock** (also required for tombstone TTL, and
  by the no-wall-clock-in-tests constraint).

*Note (attach cost): attach copies its in-scope entries inside the
actor, so N clients reconnecting at once — the correlated case being
restart → new epoch → full snapshot herd — serialize N copies on the
actor. Accepted at coordination-store scale; if it ever measures as a
stall, the remedy is sharing one immutable copy per head-seq among
concurrently attaching subscribers, without changing the interface.*

## 5. The reconnect gate and visibility

The six-step gate (core §6.6) runs at the attach event. Steps 1–3 and
5–6 read state the engine already owns (epoch, set-hash, watermark,
head). Step 4 — visibility change — gets a concrete mechanism here,
because the spec's phrasing ("the server tracks, per identity, the seq
at which its effective visibility last changed") does not implement
cleanly: role definitions can change while an identity is disconnected
(the server doesn't know absent identities' roles — they ride the
ticket), and a client can return with a *different ticket*, a
visibility change no server-side event ever saw.

**Mechanism: effective-rules-hash comparison, evaluated lazily at
attach.** At attach the engine canonicalizes and hashes the
connection's fully-expanded effective **read**-rule set (roles
expanded through current definitions, union ticket grants; the
canonical encoding is engine-internal — it never crosses the wire).
Per client id it remembers the hash in force at that client's last
attach. Gate 4 is then: **remembered hash absent or ≠ current hash →
snapshot.** This covers both change directions with one comparison:

- a role definition or space rule change alters the attach-time
  expansion → mismatch → snapshot; and precisely — identities whose
  roles didn't change keep their deltas;
- a new ticket alters the expansion → mismatch → snapshot, with no
  server-side event required;
- mid-session changes need nothing new: the spec already terminates
  the state subscription; the re-attach hits the same comparison.

**Forgetting is always safe.** The per-identity hash map is a pure
optimization cache: no remembered hash → cannot prove visibility
unchanged → snapshot. Wrongness is impossible; memory only ever
*enables* deltas. The map is therefore TTL-able as aggressively as
convenient, bounded per epoch, and dropped wholesale on epoch bump —
the same "clients plan around nothing" philosophy as the watermark.
Write-grant changes need no gating: they never affect store
correctness.

Spec errata candidate (recorded in `implementation-plan.md`): restate
gate 4 as the obligation — "snapshot when the server cannot prove the
connection's effective visibility unchanged since the cursor was
earned" — rather than mandating per-identity seq tracking; this
mechanism, per-rule-source seq tracking, and always-snapshot all
conform.

**Epoch minting: 8 crypto-random bytes** per instance start. A
counter- or timestamp-derived mint can collide across space instances
or restarts and sail through gate 1 into a delta over a foreign seq
history. `clientstore` correspondingly persists cursors **keyed by
space id**.

## 6. Invariants

The frontier invariants (F) share one fact — the cut H, established
atomically at attach — after which the lanes never coordinate. The
lifecycle invariants (L) came out of the review pass. Every invariant
is a named simulation test (§8).

- **F1 — Atomic cut at attach.** One actor event reads head seq H,
  copies the in-scope entries at H, and positions the log cursor at
  H. Everything ≤ H is in the catch-up's view; everything > H flows
  past the cursor. No gap, no overlap, by construction.
- **F2 — Catch-up frontier is the fixed H**, emitted after the copied
  entries — even when the delta is empty (that is the normal "you
  were up to date" signal). Never recomputed, never derived from
  delivered content.
- **F3 — Live frontier = log-cursor position, never content.** When
  ops in the log fall outside the subscription's scope the cursor may
  sit at seq 100 with seq 90 last delivered; the frontier honestly
  says 100 — "nothing at or below 100 further *on this path*" covers
  the filtered ops the subscriber will never see.
- **F4 — Catch-up iterates the attach-time copy, never the live
  map.** Lazy iteration of the mutating map would leak post-H entries
  onto the catch-up path and corrupt snapshot absence semantics (the
  delete-if-absent-and-≤H rule, core §6.5 rule 3).
- **F5 — Frontiers are inclusive.** Frontier X promises the client
  holds every in-scope op with seq ≤ X, and every consumer of a
  cursor treats it so — the delta rule (`seq > cursor`), gate 3
  (`cursor < watermark` fails), gate 5. The spec's "nothing further
  below X" reads exclusively (errata candidate: "at or below"); read
  that way, a cursor exactly at the watermark may never have seen the
  flushed tombstone *at* that seq and gate 3 would wrongly grant a
  delta missing a clear. Boundary tests at cursor == watermark and
  cursor == H.
- **F6 — A cut never splits a group.** A group (including the sweep)
  is one actor event with consecutive seqs; an attach cut lands
  between groups, never inside one — else the live lane would emit an
  unopenable group fragment (the wire format carries the op count up
  front).
- **F7 — Client persists no cursor until the subscription's first
  catch-up frontier is applied; thereafter the max across both
  paths.** Live frontiers > H can legitimately arrive before the
  catch-up frontier; persisting one and crashing mid-catch-up would
  reconnect past ops never received — silent permanent loss. This
  invariant is catch-up-strategy-independent: under transient
  tombstones live ops apply immediately, but the store still lacks
  catch-up entries ≤ H until the frontier, so the cursor must wait
  exactly the same. Implied by the spec's "never ahead of its
  persisted store" (§6.6); explicit in `clientstore`; errata
  candidate for the spec text.
- **L1 — No writes from a swept generation.** Every write carries its
  connection instance's generation; the sweep closes the generation
  atomically; later ops from it are silently dropped (§3).
- **L2 — Write rules are enforced in the actor** against
  connection-registration grants — write-direction filtering can no
  more be a forgotten transport concern than read-direction (§3).
- **L3 — Lag-eviction terminates the subscription object only.**
  `ErrLagged` must not feed any connection-lifecycle path; the sweep
  is reachable solely from connection departure (§4).
- **L4 — One live subscription per client**, engine-enforced: Attach
  displaces the predecessor atomically; Detach addresses a
  subscription instance, never a client (§4).
- **L5 — Detach is guaranteed on transport close**, on every path;
  reader goroutines and registrations never outlive the session (§4).

## 7. Remaining decisions (defaults; decided 2026-07-28, revised same day)

- **Access rules live inside the engine** — read rules captured at
  subscription attach; write grants registered per connection (§3). A
  forgotten filter must not be a possible transport bug in either
  direction. The two read-side matchers get distinct types with
  pointed names: plain-prefix (subscription scope, §6.3) vs
  exact-or-dot-extended (access rules, §6.4); they must not be
  swappable silently.
- **Persistence: memory-only v0** (implementation-plan deviation 3).
  Restart mints a new epoch; everyone snapshots; the protocol absorbs
  it by design. The seam for later durability: a durable store is just
  another cursor-holding consumer of the broadcast log (plus a map
  snapshot for bootstrap). Nothing built behind the seam yet.
- **Client half: `state/clientstore`**, next to the engine it mirrors
  (`client` stays purely transport). Reference catch-up strategy:
  **apply-live with transient tombstones** — record `(key, clear-seq)`
  for every live `clear` during the catch-up window, including for
  absent keys; drop the records at the frontier. This follows the
  settled first-party preference in `state-reconnect-and-gc.md`
  ("applying a mute or kick seconds late is a worse artifact than the
  client-side complexity"); an earlier draft of this doc picked
  buffer-and-replay, reversed on review against the history.
  Buffer-and-replay remains the conforming, simpler alternative for
  other implementers, per the spec's note. Store + cursor persistence
  is behind an interface from day one, honouring atomic-persist,
  invariant F7, and space-id-keyed cursors (§5).
- **`clientstore` hardening:** a catch-up deadline (no frontier within
  T → unsubscribe, re-subscribe fresh — a stalled catch-up lane must
  not freeze the strategy's transient-record window forever);
  transient-clear records are discarded on subscription termination
  (they belong to the dead instance); re-subscribe backoff (§1).
- **Echo corners (behaviour as written; no engine workarounds):**
  - a write outside the writer's own subscribed prefixes gets no
    echo — clients subscribe to what they write (§6.2 errata note);
  - a write under a grant the writer cannot *read* is accepted but
    never echoed — the read filter drops it, so §6.2's
    "exhaustively" is wrong as stated (errata: document
    write-without-read as an ack-less grant shape);
  - acknowledgement is per-key state confirmation, not per-op: a
    write made before the subscription attached is confirmed by a
    compacted catch-up entry — or, if superseded, not at all. Client
    guidance: subscribe before writing, or watch keys not ops.
  - **Future feature (recorded, not scheduled): a per-client `debug`
    track** — a diagnostics channel on which the server MAY narrate
    rejected writes, drops, and gate decisions in human-readable
    form. Non-normative, never for program logic; well-behaved
    clients never need it; developers will love it.

## 8. Test strategy

Deterministic simulation against the actor loop (injected clock,
scripted event interleavings), both halves talking through a pipe.
Named scenarios, at minimum — invariant tests first:

- crash mid-catch-up after live frontiers arrived; reconnect; verify
  no op lost (F7);
- empty delta still yields its frontier (F2);
- out-of-scope ops advance the live frontier (F3);
- writes racing a lazy catch-up never leak past H (F1, F4);
- boundary values: cursor == watermark and cursor == H produce
  correct gate verdicts and complete deltas (F5);
- an attach cut arriving mid-group lands outside it; the live lane
  never emits a partial group (F6);
- a departed connection's straggler write is dropped by the
  generation gate; no orphan key survives the sweep (L1);
- a write denied by grants / over 64 KiB / in an over-cap group
  produces no seq, no log entry, no echo (L2, §3);
- `ErrLagged` on one subscription leaves the connection, its audio,
  and its swept keys untouched (L3);
- unsubscribe/re-subscribe racing reader-side `ErrLagged` converges
  to exactly one live subscription either way; a late Detach cannot
  kill the successor (L4);
- transport death without explicit Detach leaks no goroutine or
  registration (L5);
- log overrun evicts the laggard and the resync round-trips through
  the gate cleanly; sustained-load boundary: transfer time just
  inside vs just outside the sizing invariant (§1) — converges vs
  detected-thrash-with-backoff;
- sweep-vs-eviction ordering: an evicted connection's sweep completes
  or is cancelled before the replacement's admission writes;
- gate 4 via rules-hash (§5): role definition changes while the
  identity is disconnected → snapshot; unrelated role changes →
  delta preserved; reconnect with a different ticket → snapshot;
  cache TTL'd/absent → snapshot (forgetting is safe);
- set-hash mismatch and epoch mismatch trip their gates; two spaces
  in one process never accept each other's cursors (crypto-random
  epochs + space-id-keyed client cursors, §5);
- GC: tombstone TTL flush moves the watermark; a cursor below the
  watermark gets a snapshot;
- transient-tombstone client: the late-chunk resurrection case (clear
  live at seq 1005, catch-up chunk carries set at 980) never
  resurrects; records dropped at frontier; records discarded on
  mid-window termination; catch-up deadline fires on a stalled lane.
