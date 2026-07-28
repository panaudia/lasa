# State engine design

*Decided 2026-07-28. Companion to `implementation-plan.md` (work-order
context) and to `lasa-core.md` §6, which this implements. Records the
concurrency/fan-out decision with rejected alternatives, the delivery
interface, the frontier invariants, and the smaller defaults.*

The engine is the transport-neutral implementation of core §6: the
store, seq/epoch machinery, the reconnect gate, two-lane delivery,
tombstone GC, and the disconnect sweep. It speaks structs, never wire
bytes. **One engine instance per space**; a process hosts many; seq
counter, epoch, GC, and the broadcast log are all per-instance, no
globals.

## 1. Concurrency and fan-out: actor + broadcast log

Two orthogonal choices, decided together:

- **Serialization: a single-goroutine actor** owns the store. All
  mutations — writes, subscription attach/detach, sweeps, GC ticks —
  are messages consumed in order by one goroutine owning the map and
  the seq counter.
- **Fan-out: a bounded in-memory broadcast log.** On acceptance the
  actor assigns the seq, updates the compacted map, appends to the
  log, publishes the new head. Subscribers hold cursors into the log
  and consume at their own pace in their own goroutines.

Why the actor: total order falls out of message order, so seq
assignment is trivially correct; the sweep-before-replacement-admission
ordering rule (core §4.4) is just two messages in order; the
subscription-attach cut (below) is a single event rather than a locking
protocol; and the loop can be driven deterministically in simulation
tests, which is where this component's hard bugs will be flushed out.
The actor never blocks on delivery — appending to the log and
publishing the head is the whole write-side fan-out cost, O(1) in
subscriber count.

Why the broadcast log (over per-subscriber queues): one append instead
of N enqueues; one shared window of ops instead of N duplicated
queues; per-subscriber filtering and wire encoding move into subscriber
reader goroutines, off the write path (safe because a subscription's
access rules are fixed at attach — mid-session visibility changes work
by terminating the subscription, core §6.6, so rules are captured once
and never consulted under contention). Log entries share the map's
immutable value bytes — the log is a window of small structs, not a
second copy of the store. It is a *broadcast log*, not a WAL: it exists
for live fan-out only and plays no durability role.

**Overflow policy: eviction-on-lag.** The log is bounded; a reader
whose cursor falls off the tail gets a distinct error and the transport
terminates that state subscription via native MoQ termination. This is
not an error hack — it is exactly the protocol's designed resync path
(the client re-subscribes, hits the gate, receives a delta or
snapshot), the same mechanism the spec uses for visibility changes.

Rejected alternatives, with reasons:

- **Mutex-guarded map**: seq assignment and delivery ordering force
  holding the lock across the same critical section the actor owns —
  the same serialization with invariants by convention instead of by
  structure, and concurrent rather than deterministic tests.
- **Lock-free (statecache-style ring)**: unjustified. The ring earns
  its complexity for append/read-since semantics; LASA needs global
  order, per-key compaction, prefix scans, consistent cuts, and the
  gate. The workload is a coordination store at human rates, and the
  spec bars state from ever migrating to datagrams — there is no
  future hot path to build for.
- **Per-subscriber queues**: O(N) write path, N× op duplication, and
  an invented overflow policy where the log gets a protocol-native one.

## 2. Store representation

Plain flat map, per-key `{value, seq, tombstone}`. Prefix scans (the
attach-time catch-up computation is the only scan consumer) by full
scan. At coordination-store scale (≤512 B keys, ≤64 KiB values, human
write rates) this is adequate; revisit only if profiling says
otherwise. No retained op history beyond the broadcast log's window —
the spec guarantees compaction, so catch-up never replays intermediate
ops (§4, invariant on delta generation).

## 3. Delivery interface

The engine's API to the transport shell — the surface the `server`
SPI builds on:

- **Attach** carries the subscription's prefix set, its captured
  access rules, and the presented cursor `(epoch, seq, set-hash)`.
  The actor runs the six-step gate (core §6.6) *at the attach event*,
  so the verdict is atomic with the cut, and returns a `Subscription`:
  the gate verdict, a catch-up lane, and a live lane.
- **Catch-up lane**: an iterator over the delta or snapshot entries,
  computed from a copy of the in-scope map entries taken inside the
  attach event, terminating with the frontier cut. Snapshots carry no
  tombstones (spec); an empty delta still ends with its frontier.
- **Live lane**: pull-based consumption of the subscriber's log cursor,
  filtered through the captured rules. Backpressure is cursor lag;
  overrun surfaces as `ErrLagged`, the transport's signal to terminate
  the subscription. Pull keeps the engine ignorant of transport pacing.
- **Structs, not bytes**: ops surface as (key, value, seq, tombstone,
  group boundary) values; all encoding lives in `wire`, applied by the
  shell. Group boundaries are recorded in the log so live delivery can
  honour the preserve-groups SHOULD.
- **Periodic frontiers** are injected into the live lane on a tick
  from an **injectable clock** (also required for tombstone TTL, and
  by the no-wall-clock-in-tests constraint).

## 4. Frontier correctness: five invariants

The lanes share exactly one fact — the cut H, established atomically
at attach — and never coordinate afterwards. Each invariant is a
named simulation test:

1. **Atomic cut at attach.** One actor event reads head seq H,
   copies the in-scope entries at H, and positions the log cursor at
   H. Everything ≤ H is in the catch-up's view; everything > H flows
   past the cursor. No gap, no overlap, by construction.
2. **Catch-up frontier is the fixed H**, emitted after the copied
   entries — even when the delta is empty (that is the normal "you
   were up to date" signal). Never recomputed, never derived from
   delivered content.
3. **Live frontier = log-cursor position, never content.** When ops
   in the log fall outside the subscription's scope the cursor may sit
   at seq 100 with seq 90 last delivered; the frontier honestly says
   100 — "nothing below 100 further *on this path*" covers the
   filtered ops the subscriber will never see.
4. **Catch-up iterates the attach-time copy, never the live map.**
   Lazy iteration of the mutating map would leak post-H entries onto
   the catch-up path and corrupt snapshot absence semantics (the
   delete-if-absent-and-≤H rule, core §6.5 rule 3).
5. **Client persists no cursor until the subscription's first
   catch-up frontier is applied; thereafter the max across both
   paths.** Live frontiers > H can legitimately arrive before the
   catch-up frontier; persisting one and crashing mid-catch-up would
   reconnect past ops never received — silent permanent loss. Implied
   by the spec's "never ahead of its persisted store" (§6.6); an
   explicit invariant of `clientstore` here, and an errata candidate
   for the spec text.

## 5. Remaining decisions (defaults, decided 2026-07-28)

- **Access rules live inside the engine**, captured at attach — a
  forgotten filter must not be a possible transport bug. The two
  matchers get distinct types with pointed names: plain-prefix
  (subscription scope, §6.3) vs exact-or-dot-extended (access rules,
  §6.4); they must not be swappable silently.
- **Persistence: memory-only v0** (implementation-plan deviation 3).
  Restart mints a new epoch; everyone snapshots; the protocol absorbs
  it by design. The seam for later durability: a durable store is just
  another cursor-holding consumer of the broadcast log (plus a map
  snapshot for bootstrap). Nothing built behind the seam yet.
- **Client half: `state/clientstore`**, next to the engine it mirrors
  (`client` stays purely transport). Reference catch-up strategy:
  **buffer-and-replay** (freeze live application until the frontier,
  replay under dedup) — the hazard-free option; live-with-transient-
  tombstones remains a conforming alternative for other implementers.
  Store + cursor persistence is behind an interface from day one,
  honouring atomic-persist and invariant 5.
- **Echo corner (spec erratum, behaviour as written):** a write outside
  the writer's own subscribed prefixes gets no echo —
  indistinguishable from a silent drop for that client. Resolution:
  clients subscribe to what they write; note to add at core §6.2. No
  engine workaround.

## 6. Test strategy

Deterministic simulation against the actor loop (injected clock,
scripted event interleavings), both halves talking through a pipe.
Named scenarios, at minimum:

- crash mid-catch-up after live frontiers arrived; reconnect; verify
  no op lost (invariant 5);
- empty delta still yields its frontier (invariant 2);
- out-of-scope ops advance the live frontier (invariant 3);
- writes racing a lazy catch-up never leak past H (invariants 1, 4);
- log overrun evicts the laggard and the resync round-trips through
  the gate cleanly;
- sweep-vs-eviction ordering: an evicted connection's sweep completes
  or is cancelled before the replacement's admission writes;
- visibility change while disconnected trips gate 4 (snapshot);
- set-hash mismatch and epoch mismatch trip their gates;
- GC: tombstone TTL flush moves the watermark; a cursor below the
  watermark gets a snapshot.
