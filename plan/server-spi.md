# Server SPI

*Decided 2026-07-28. The interface between the `server` package (the
transport shell) and its embedder (the panaudia engine, the in-repo test
stub, and eventually the cloud-mixer gateway). Companion to
`implementation-plan.md` (which schedules the shell as step 5) and
`state-engine-design.md` (the engine the shell wires in).*

## Division of labour

**The shell owns:** MoQ session establishment over the forked
moqtransport (CLIENT_SETUP auth via `connect`, rejection codes as
`SetupRejection`); namespace/track routing and the §3.1 authorization
matrix via `ident`; the state engine instance and all state-track
traffic; the presence *track machinery* (generations, implicit indexes,
keyframe chunking, delta cadence); sink track framing (seq, wire ts
cadence, future redundancy policy); admission-time base-profile writes
(existence marker, name, signed-attrs) and the departure sweep, via
`profile/base` helpers, so no embedder can forget them.

**The embedder owns:** everything audio (decode, mixing, rendering,
jitter, redundancy recovery); entity lifecycle in the space; admission
policy (collisions, capacity, blocking); pose state — including
answering the presence poll; profile-consumer behaviour against the
engine (reading `lasa.*`, enforcement writes).

## The interface

```go
// One Space = one space id = one engine. The embedder implements Backend.
type Backend interface {
    // Admit applies admission policy to a parsed, verified, resolved
    // connection. It MAY block until an evicted predecessor's teardown
    // has reached the engine — the sweep-before-replacement-writes
    // ordering is this contract (the SPI's one ordering obligation).
    Admit(res *connect.Resolved) *connect.Rejection

    // Entity lifecycle. The returned handler receives the entity's
    // ingress for the life of the connection.
    EntityJoined(clientID string, e connect.ResolvedEntity) (EntityHandler, error)
    EntityLeft(clientID, entityID string)

    // Sink lifecycle: subscription is the only signal. The first live
    // subscription to any format of an entity's sink starts a render;
    // the embedder pushes frames into w until Stop. An error rejects
    // the subscribe (e.g. the wrong ambi order under the global-order
    // deviation).
    StartSink(entityID, format string, w SinkWriter) (SinkSession, error)

    // Presence: polled by the shell at its delta cadence. The embedder
    // answers from the pose state it applied from ingress.
    Presence() []PresenceEntry
}

type EntityHandler interface {
    // WritePacket delivers one mono-object datagram payload RAW,
    // synchronously, on the receive path, in arrival order per session;
    // seq is the MoQ group id. See "The ingress contract".
    WritePacket(seq uint64, payload []byte)
}

type SinkWriter interface {
    // WriteFrame pushes one rendered frame; sampleTS names the frame's
    // first sample on the embedder's sample clock (samples — the
    // embedder is wall-clock-free; Config.SampleEpoch maps samples to
    // epoch µs when the shell emits the informational anchors). The
    // shell owns seq, the on-wire ts cadence (anchor + ~1/s), and any
    // future sink-redundancy policy — the embedder never sees framing.
    WriteFrame(frame []byte, sampleTS uint64) error
}

type SinkSession interface{ Stop() }

type PresenceEntry struct {
    EntityID  string
    Pose      wire.Pose
    Loudness  wire.Loudness
    HeadFrame bool
}
```

At startup the embedder also receives the shell's `*state.Engine` — it
registers its own observer subscription for profile keys and uses
`Engine.Write` for enforcement writes.

## The hot-path contract (load-bearing)

**The LASA layer must not add latency to audio.** On the audio paths —
ingress datagram → `WritePacket`, and `WriteFrame` → egress datagram —
the shell introduces **no buffering, no queuing, no goroutine or
channel hops, no object allocation, and no copying beyond the single
unavoidable framing copy on egress**. Audio is never held waiting to be
asked for. Concretely:

*Ingress.* On datagram arrival the shell routes to the entity's handler
and calls `WritePacket` synchronously on the receive goroutine — raw
payload, no decode, no latest-wins, no receiver-state of any kind.
**Ownership of the payload bytes transfers to the handler**: the slice
is the received datagram's own buffer (already allocated by the QUIC
stack), the shell neither copies nor retains it, and no shell-side
buffer pooling may force the embedder to copy. The embedder's decode is
itself zero-copy — `wire.ParseMonoObject`'s audio and redundancy slices
alias the payload — so a jitter buffer can retain them directly. All
§5.1/§5.2 receiver behaviour (flag validation and discard, latest-wins
pose, the pose fast path, jitter buffering, redundancy recovery)
belongs to the embedder. Symmetrically, the handler must hand off and
return — never process in place on the receive goroutine.

*Egress.* `WriteFrame` frames and sends the datagram synchronously on
the caller's (render loop's) goroutine: one append of header + audio
into a per-sink scratch buffer reused across frames — the one framing
copy, amortized zero allocation — then straight into the session's
datagram send. Fan-out to multiple subscribers of one sink track is a
loop of sends, never per-subscriber queues. Whatever queueing the QUIC
stack does internally is below this layer; the LASA layer itself holds
nothing.

*Scope.* This contract covers the audio paths only. State and presence
are not hot paths; normal Go style applies there.

*Clocks (2026-08-01, panaudia-server-design.md §1).* Uplink carries no
timestamps; the embedder's jitter buffer is the sole clock-domain
coupling (fill adaptation in the sample domain, never timestamp
comparison); no component compares stream time with system time on any
audio path. The one authoritative clock is the embedder's render tick;
the shell's only contact with epoch time is `Config.SampleEpoch`, whose
value derives from that tick (epoch₀ read once at the first tick), used
solely for the informational §5.1 sink anchors.

Step 5 acceptance: the e2e suite includes allocation checks
(`testing.AllocsPerRun`-style) on both paths so the contract is
enforced by CI, not by review.

Why raw ingress: it keeps the shell protocol-thin, gives the mixer
embedder full control of its pipeline, and makes a future SFU-style
gateway trivial — a forwarding backend relays `(seq, payload)`
unchanged; the raw write IS the SPI, not a special mode. A
decoded-handler convenience wrapper (one shared implementation of
flag-discard + latest-wins for mixer-type embedders) can be added to
the `server` package later as a utility, not an SPI change.

## Decisions and rationale

- **(a) Per-entity handler objects** over flat callbacks: maps onto
  panaudia's entity → ConnectionHandler table, no per-packet lookup in
  the embedder, natural teardown point.
- **(b) Sinks are push**, embedder-clocked, with per-frame `tsMicros`:
  the render loop is the clock; the ts names sample time only the
  embedder knows. The shell decides when a ts goes on the wire.
- **(c) Presence is polled** at the shell's delta cadence; the shell
  does change detection, generations, indexes, chunking. One trivial
  method on the embedder.
- **(d) One `server.Server` = one space** in v0. All space-scoped
  machinery (engine, presence state, backend binding) lives in one
  internal `space` struct so a future listener can host many. **The SPI
  surface carries no space id anywhere** — it appears only where the
  protocol forces it (ticket `aud` verification, namespace tuples),
  both shell-internal. A future multi-space gateway instantiates one
  Backend per space (or a later `ResolveSpace(spaceID) → Backend`
  provider); its backends know where to forward because they were
  created for that space.
- **(e) Raw synchronous ingress** (see above) — revised in review from
  a decoded-handler design; decided 2026-07-28.
- **(f) Admit may block** for predecessor eviction; the alternative
  (shell-side retry/pending machinery) buys nothing panaudia's
  departure funnel doesn't already provide.
  *Superseded 2026-08-13 (`id-capabilities.md`; plan:
  `identity-implementation.md`), implementation pending: claims
  become state keys adjudicated atomically on the state actor
  (capacity included), supersession's teardown wait becomes shell
  machinery on the live registry, and `Admit` is deleted from the
  SPI outright — collision and capacity both leave the embedder.*

## Recorded seams (not built)

- **Decomposition seams** — the state-service seam (message-shaped
  engine API, lease-sweep, ordered-stream constraint, shapes A/B) and
  the presence composer pattern (single-author constraint, transport-
  free `presence` package) are analysed in **`decomposition-seams.md`**.

- **quic-go zero-copy receive.** Below this layer, quic-go itself
  (verified in v0.60 source) copies every received datagram twice —
  once out of the pooled packet buffer at frame parse, once into its
  receive queue — and its bounded receive queue drops silently under a
  slow reader. The LASA layer and the moqtransport fork add zero
  allocations on either side of that, so quic-go is now the whole
  per-datagram allocation budget (~2 allocs + 2 copies receive, 1 copy
  send — the send copy is unavoidable by API). If end-to-end latency
  budgets tighten, the lever is an upstream quic-go contribution:
  zero-copy datagram receive has prior discussion in their tracker.

- **Multi-space / gateway split** per (d). The SFU-gateway future
  (mixer-blind forwarding by space id) is served on the audio path by
  raw ingress; its open questions — whose engine, whose presence
  roster, sink-subscribe auth in a relay — belong to the relay topology
  the spec parks, and nothing in this SPI forecloses them.
- **Sink redundancy policy**: the shell owns framing and can add
  transparent per-sink redundancy (it can retain the frame window)
  behind a policy knob without touching `SinkWriter`.
- **Decoded-handler utility wrapper** for mixer-type embedders.

## Stub backend

The `server` package ships a stub Backend for in-repo e2e tests (the
`moq_test_stub` pattern): loopback audio (an entity's sink renders its
own source), scripted presence, permissive admission. The reference
`client` package drives it over real QUIC in CI.
