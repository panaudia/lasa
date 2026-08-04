# State-path performance baseline

*Measured 2026-07-31 on an M4 Pro (14 cores), loopback. Findings note,
not a decision doc. Benchmarks live in `state/perf_test.go`
(engine-only) and `client/perf_test.go` (end-to-end through real
QUIC/MoQ: `WriteState` → server → engine → `deliverLive` → client
`ReadObject`; env knobs `LASA_PERF_SUBS`, `LASA_PERF_INTERVAL_MS`).
Run: `go test ./state -run '^$' -bench BenchmarkState -benchtime 2s`
and `go test ./client -run '^$' -bench BenchmarkE2EState -benchtime
1000x` (fixed `-benchtime Nx` for the e2e ones — each `b.N` ramp
redials every connection).*

## Engine alone

Write 562 ns/op single caller (~1.8M writes/s); **~470k/s aggregate
with many concurrent one-op callers** (the unbuffered actor-channel
round-trip dominates under contention); batched via variadic
`Conn.Write` 184–221 ns/op (~5M/s). Delivery holds **~4.5–5M
deliveries/s** roughly flat from 1 to 128 subscribers (each drain
includes the `wire.AppendStateMessage` encode). Write→deliver latency
is single-digit µs at 1 sub, ~126 µs p50 at 128 subs under paced 1 kHz
load.

## Through the transport (loopback QUIC, one process for both sides)

Ceiling **~1.8M deliveries/s**, flat 8→256 subscriber connections.
Per-write cost through one QUIC stream ~2.5 µs (~400k writes/s).
Latency p50/p99 under paced load: 196 µs/870 µs at 1 sub, 540 µs/2.5 ms
at 128, ~3 ms/9 ms at 256 (at 20 writes/s) — the binder is the fan-out
sweep, **~10–20 µs of server work per subscriber per write**, so the
last subscriber waits for the whole sweep. Loopback numbers: the bench
process runs all clients too, so a dedicated server has roughly 2× this
CPU; real RTT adds a constant on top.

## What it means

- **The engine is not the bottleneck; the transport is**, by ~an order
  of magnitude. Direct-serving capacity is `subs × space-wide write
  rate ≤ ~1.8M/s` with the latency sweep binding first: mid-hundreds of
  state subscribers are comfortable at design-intent (low-rate) state
  usage; the pathological everyone-writes-20Hz-through-state mesh caps
  near ~300 clients — which is why pose does not ride the state track.
- **Singleton-store topology** (future cloud shape: edge nodes carry
  client fan-out, one state service per space holding one subscription
  per edge node): sustainable rate ≈ `min(~5M/s batched ingest,
  ~4.5M/s ÷ edge-count delivery)` — ~500k writes/s at 8 edges. The N²
  client explosion moves to the edges; the engine copes by a wide
  margin and the RPC layer in front of it binds first.
- **Batch into the engine.** The one load-bearing service-design
  finding: per-op calls from many goroutines get ~470k/s aggregate —
  worse than one caller — while batching recovers 10×. A service
  frontend should coalesce ops per connection/tick before applying.
- Ingest is one core by design (the actor): a per-space ceiling that
  does not grow with machine size. Fine at this headroom.

## Reopen triggers

Re-measure before trusting these if: state grows to millions of keys
(attach-time catch-up builds a sorted snapshot inside the actor —
O(store) ingest stall, unmeasured); the engine's `do()` path or
broadcast log changes; a real state-service RPC frontend exists
(measure it, not the engine); or state acquires any high-rate
per-client writer.
