# Decomposition seams

*Recorded 2026-07-28, from the structural review after step 5. This doc
answers two future-facing questions for a non-monolithic deployment (the
cloud-mixer shape: gateway / mixer / state split): is the state engine
modular enough to become a network service, and what do routing,
aggregation and rate limiting of presence require? Companions:
`server-spi.md` (whose seams section points here),
`state-engine-design.md`.*

Nothing here is scheduled work. These are load-bearing observations to
protect while the codebase evolves.

## 1. The state engine as a network service

**Verdict: the engine is already modular enough — and its API is
protocol-shaped.** The properties that make it so (protect these):

- **Message-shaped API, no inversion of control.** The engine never
  calls its embedder: every interaction is a request (`Register`,
  `Attach`, `Detach`, `Close`+sweep), a fire-and-forget (`Write`), or a
  pulled stream (`CatchUpNext`/`LiveNext`). Each maps 1:1 onto a
  network message or stream.
- **Payloads already have a wire format.** The engine speaks
  `wire.Set/Clear/Group/Frontier/Begin` — §6.7 bytes. A gateway ↔
  state-service protocol is those encodings plus a small control
  envelope (register+rules, attach+cursor→verdict, detach, sweep).
- **The write path is zero-round-trip by spec design**: rejections are
  silent and the only ack is the echo on the subscription stream —
  latency-tolerant across a process boundary for free.
- **Backpressure semantics survive remoting unchanged**: the
  broadcast-log-with-cursors model maps onto network streams directly —
  remote consumer backpressure IS cursor lag, and eviction-on-lag with
  gate resync is already the overflow policy.
- **Per-instance everything** (epoch, seq, GC, clock): a state daemon
  hosting N spaces is N engines.

Two cross-process obligations for the future service wrapper (engine
changes: none):

1. **Distributed lifetime.** A dead gateway must synthesize
   `Conn.Close(sweep)` for every connection it held. Standard shape:
   the service leases each gateway session and sweeps on lease expiry.
   The engine's one-message sweep makes the wrapper trivial.
2. **Sweep-before-replacement ordering** (the L-invariant) rides the
   actor mailbox FIFO today. Remoted, it rides stream ordering:
   `Deregister(old)` and `Register(new)` MUST travel one ordered
   stream (at minimum, ordered per client id). Do not load-balance
   these messages onto independent connections.

**Two decomposition shapes, both open** (record, don't choose):

- **(A) Bouncer-style**: the gateway terminates LASA state and speaks a
  private protocol (the envelope above) to the state service. Matches
  the current cloud-mixer bouncer + backfill model.
- **(B) State service speaks MoQ/LASA natively**: because the per-client
  state tracks are end-to-end and the service's external protocol can
  literally BE the LASA state protocol, the gateway could relay state
  subscriptions (subgroups and all) without terminating them. The
  mixer-blind SFU-gateway future composes naturally with this.

The mixer-as-profile-consumer (observer subscription + `Engine.Write`)
remotes as just another state client under either shape.

## 2. Presence: routing, aggregation, rate limiting

- **Routing needs nothing.** Presence is the one shared, cacheable
  track and its datagrams are self-contained (gen-guarded,
  keyframe-refreshed, latest-wins by group id): any relay — including a
  mixer-blind SFU gateway — replicates them verbatim.
- **Aggregation has a named constraint: the presence wire format is
  single-author.** The generation byte and implicit index assignment
  only work with one roster owner. Shards must never co-author one
  presence track; the decomposed answer is a single **presence
  composer** that merges shard samples and authors the track. The code
  is shaped for exactly this (2026-07-28 refactor): package `presence`
  is transport-free — `presence.Entry` is the aggregation currency
  (merge []Entry from N sources), `Author.Tick(now)` lets a composer
  drive the machinery on its own scheduler, and `Sender` is one
  function. The delicate assets (chunk boundaries, gen bumps,
  within-gen roster stability) live in one reusable, unit-tested place.
- **Rate limiting stays at track granularity.** Per-subscriber rates or
  spatial interest filtering would break the shared-cacheable-track
  property — that is a track-design boundary, not an implementation
  gap. The LASA-native mechanism for tiers is **additive track names**
  (a future `presence-4hz`, per-region presence tracks): new names
  break nothing, and one Author instance per track name serves each.
  Within one track, cadence remains server policy (config today).

## Related, deliberately unresolved

The mixer-blind gateway's remaining questions — whose engine serves
state, whose composer authors presence, sink-subscribe authorization in
a relay — belong to the relay topology the spec parks. Nothing above
forecloses them; shapes (A)/(B) and the composer pattern are the
building blocks they would pick from.
