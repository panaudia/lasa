# WebTransport close codes never reach the client

*Recorded 2026-08-13, found by the identity batch E browser rig
(`identity-implementation.md`). OUTSTANDING — parked with
reproductions in place; fix is its own session.*

## Symptom

A LASA session rejection or sever over **WebTransport** loses its
§4.6 code: the server calls `CloseWithError(0x1A5Axx, reason)`, but
Chrome reports only "WebTransportError: Connection lost." and the Go
webtransport client gets bare `EOF` — no `webtransport.SessionError`
ever materializes client-side (probe: session cause is plain "context
canceled" 500 ms after the close). 100% reproducible, not a race we
sometimes win. Consequence: a browser user refused admission cannot be
told *why* (invalid ticket vs blocked vs at capacity vs mode
mismatch), and a post-connect sever (moderation, supersession) is
equally anonymous. The spec's §4.6 property — "correctness MUST ride
the code alone" — is broken for one transport.

**Raw QUIC is unaffected**: the code rides CONNECTION_CLOSE natively
and is asserted end-to-end (`TestRawQUICRejectionCarriesCode`,
passing). The bridge's transport is clean; browsers are the victims.

## Suspected mechanism (webtransport-go v0.12.0, read not yet proven)

`Session.CloseWithError` does not write the close capsule — it queues
it and signals `capsuleQueueUpdated`; a separate goroutine
(`writeToConnectStream`) owns the CONNECT stream and does the write.
Three weak links in a row:

1. That goroutine's loop selects `ctx.Done()` against the queue
   signal — teardown winning drops the capsule silently (the likely
   cause of our 100% loss).
2. The capsule write carries a **10 ms** deadline
   (`closeSessionTimeout`); missing it turns the close into a stream
   reset — code destroyed.
3. Even on success, `CancelRead` + FIN + stream-map resets follow
   immediately and can share a flight with the capsule.

No newer upstream version exists (v0.12.0 is latest).

## Fix ladder

1. **Pin it (~30 min):** vendor webtransport-go via a `replace`
   directive, log capsule queued / written / writer-exited-via-ctx,
   run the skipped reproduction. The patch differs by link.
2. **Patch webtransport-go** (new fork or vendored copy — the Eyevinn
   moqtransport pattern): make close-capsule delivery unconditional —
   write it synchronously in `CloseWithError`, or make the writer
   drain a pending close capsule even when ctx is done; raise the
   write deadline to ~250 ms. Fixes rejection AND sever codes in one
   place.
3. **Companion line in the moqtransport fork** only if step 1
   implicates the MoQ layer: on the rejection path, hold the
   goroutine briefly after `conn.CloseWithError` so errgroup teardown
   (control-stream FINs) doesn't race the capsule flight.
4. **Upstream** a quic-go/webtransport-go issue+PR once confirmed —
   without waiting on it.

Rejected: moving rejections to an in-band MoQ message (spec surface
added to dodge one library bug in one transport; §4.6's
transport-close design is proven sound by raw QUIC).

## Acceptance criteria, sitting ready

- Un-skip `TestWTRejectionCarriesCode` (panaudia-server) — the Go WT
  client sees `SessionError{ErrorCode: 0x1A5A01}`.
- Un-skip `TestWTRejectionProbe` or delete it (diagnostic).
- Tighten `tests/ticketed.spec.ts` (typescript-lasa) back to
  `.toBe(0x1a5a01)` / `.toBe(0x1a5a08)` — real Chrome then proves the
  full chain.
- No client work remains: the TS SDK's typed surfacing
  (`LasaRejectionError`, `onClosed`, worker plumbing) shipped with
  batch E and is unit-tested; it lights up when the transport
  delivers.

Landing is a version bump through Paul's hands (fork tags, go.mod).
