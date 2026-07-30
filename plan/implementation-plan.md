# Implementation plan

*Drafted 2026-07-28, from the panaudia refactor-structure design session.
Companion: `state-engine-design.md` (the state engine's internal design,
decided the same session). This is the outline plan for implementing
LASA in Go and integrating it into panaudia; it records structure,
order, and deliberate deviations — detailed designs stay in their own
docs.*

## Shape of the work

The implementation decomposes into three self-contained, well-tested
components, all living **in this repo** as a Go module — which thereby
becomes the reference implementation the README promises:

1. **The pure wire layer** — every byte layout in the spec, as pure
   functions. No transport, no engine, stdlib only.
2. **The state engine** — the §6 machinery (store, seq/epoch/gate,
   two-lane delivery, GC, sweep), transport-neutral. See
   `state-engine-design.md`.
3. **The transport shell** — the actual MoQ server (and reference
   client) over the Eyevinn fork, driving 1 and 2.

Panaudia consumes the module and implements a small SPI (below). The
existing panaudia front ends (`core/moq`, WebRTC, ROC) are left in
place untouched, with the ambition to delete `core/moq` once LASA/MoQ
is stable. Both MoQ servers run side by side during migration.

## Module layout

Single Go module **`github.com/panaudia/lasa`**, `go.mod` at the repo
root, no root-level `.go` files — the spec stays the first thing you
see. Packages, in dependency order (each depends only on earlier ones):

| Package | Contents |
|---|---|
| `ident` | identifier & key grammars, namespace tuple types, track-name constants, subscribe/announce authorization matrix |
| `wire` | §5/§7/§6.7 byte layouts: flags, 18-byte pose, mono-object / binaural / ambi packets, redundancy framing, sink ts anchoring, state messages (direction-aware parsers), subscribe-KVP + set-hash, presence delta/keyframe |
| `connect` | Connection Config parse/validate against `schemata/`, ticket JWT verification (Ed25519, `aud`/`exp`/`nbf`), entities/setups resolution, permission-set derivation, error codes `0x1A5A00–07` |
| `state` | the engine: server store + `clientstore` (client half). Design in `state-engine-design.md` |
| `profile/base` | base-profile vocabulary as code: typed key builders/parsers, role→rule expansion, value validation, defaults |
| `server` | transport shell: CLIENT_SETUP auth, track routing, subscription-driven sink lifecycle, state subgroup mapping, presence loop. Self-contained behind an SPI |
| `client` | reference Go client: dial, present config, publish entities, subscribe sinks/state/presence, run `clientstore`. Doubles as the e2e test harness |

`ident`/`wire`/`state`/`profile` are stdlib-only; `connect` adds a JWT
dependency; `server`/`client` add the Eyevinn fork + quic-go.

**The server is self-contained** (decided over the thin-library
alternative): `server` defines its own SPI — designed 2026-07-28 in
**`server-spi.md`**: a `Backend` with per-entity handler objects, raw
synchronous ingress (`WritePacket(seq, payload)` — the shell never
buffers or decodes ingress), push sinks with per-frame sample
timestamps, polled presence, Admit-may-block for eviction ordering,
and no space id anywhere on the SPI surface (multi-space/SFU-gateway
seam recorded). A stub Backend ships in-repo for e2e tests, on the
pattern of panaudia's `moq_test_stub`. Panaudia implements the SPI by
mapping entity-id → `panaudia_server.ConnectionHandler` (one handler
per entity, through the existing admission funnel).

## Deliberate deviations from the spec (v0)

Recorded so they cannot drift silently; each has a reopen trigger.

1. **Server-global ambisonic order.** The spec (core §3) requires every
   Space to offer all three sink formats. v0 instead makes the ambi
   order a global server configuration; subscribing to the wrong order
   is an error. Binaural is always offered. *Why:* per-entity
   dual-order ambi encode is an awkward engine refactor that adds
   nothing to proving the protocol. *Reopen:* when a real deployment
   needs mixed-order subscribers.
2. **Evict-old on unticketed collision.** The spec (core §4.4) rejects
   a new unticketed connection whose client/entity id is live. v0
   matches current panaudia behaviour: the old session is evicted.
   *Why:* one admission rule across old and new front ends during
   migration. *Price:* evict-on-collision is an id-hijack primitive on
   open networks — acceptable only because unticketed mode is dev-only.
   *Reopen:* before any unticketed deployment beyond closed networks;
   revisit alongside ticketed-mode hardening.
3. **Memory-only state store.** The base profile SHOULDs durable
   `lasa.space.*`. v0 restarts mint a new epoch and drop everything —
   a restart can un-kick (the profile's own recorded weakness, §3).
   The engine leaves a persistence seam (see `state-engine-design.md`).
   *Reopen:* first deployment where moderation must survive restarts.

**Spec errata candidates — all ten folded into the spec text
2026-07-30** (none won't-fixed; every recorded resolution was already
implemented in code, so each fix was spec-text only). The edits, all
in `lasa-core.md`: §6.2 out-of-scope writes + the accepted-yet-
unechoed shapes and per-key-not-per-op acknowledgement (errata 1–2);
§6.5 frontier made explicitly inclusive, "at or below" (3); §6.6
gate 4 restated as the visibility obligation rather than a
per-identity-seq mechanism (4); §6.6 no-cursor-before-first-frontier
made explicit (5); §6.1/§6.7/§9 group op-count cap — server
constant, over-cap dropped whole, reference default 64 (6); §5.1/§9
endianness sentence — fixed-width little-endian, varints RFC 9000
(7); §6.6/§6.7 set-hash canonical form pinned to minimal varints,
non-canonical subscribes rejected (8); §5.1 redundancy-offset bits
MBZ when bit 2 clear, discard rule otherwise (9); §4.2 duplicate-id
rule restated with the setup→ticket reference exempted (10). The
pre-fold wording of each erratum is in this file's git history.

**Open erratum candidate 11** (2026-07-30, found designing the engine's
channel enforcement): base profile §4 resolves gain/attenuation *among*
shared channels ("highest channel gain wins; lowest attenuation wins")
but never says how the winning values **compose with** the entity's own
`render.gain`/`render.attenuation` — multiply, override, or min. The
incumbent's role-composites precedent (gain multiplies, attenuation
overrides) is one answer, not a decision. Pin in the profile text
before Phase C builds the per-pair record that bakes it in.

**Future features (recorded, not scheduled):** a per-client `debug`
track — a diagnostics channel on which the server MAY narrate rejected
state writes, silent drops, and gate decisions in human-readable form.
Non-normative, never for program logic; motivated by the deliberate
silent-failure surface above.

## Work order

*Status 2026-07-28: steps 1–5 complete, all green under `-race` (see
`next-steps.md` for the session log — the fork grew beyond step 1's
three API changes to seven upstreamable commits, and a `presence`
package was extracted per `decomposition-seams.md`).*

*Status 2026-07-29: **direction change — steps 6–8 superseded** by the
greenfield rebuild decided in `plan/greenfield-standalone.md`: instead
of integrating into existing panaudia, the render core is copied into
a new `panaudia-engine` repo and a new `panaudia-server` repo composes
it with this module; existing panaudia is left frozen (maintenance
mode at parity, no migration). The integration path is kept priced in
`plan/panaudia-integration.md` (not taken). Steps 1–5 and everything
above the work order stand unchanged; step 7's base-profile depth is
pulled onto the new server's critical path (its Phase C).*

1. **Foundations.** `go.mod` + CI in this repo. The Eyevinn v0.8.2
   fork with the three API changes (client `WithSetupParameters`,
   server setup-params accessor/callback, reject-before-SERVER_SETUP).
   The fork gates only step 5 — steps 2–4 can run before or alongside.
2. **`ident` + `wire`**, test-first against the spec: table-driven
   tests plus golden byte vectors. The vectors double as conformance
   fixtures for other implementations (TS client). Expected side
   benefit: surfacing spec ambiguities while the design is fresh.
3. **`connect`** — schemas, tickets, permission derivation.
4. **`state`** — the big one. Server + client halves, deterministic
   simulation tests, per `state-engine-design.md`.
5. **SPI design pass**, then **`server` + `client` + stub backend**;
   e2e tests entirely in-repo (reference client ↔ stub server over
   real QUIC).
6. **Panaudia integration.** New front-end package in panaudia
   implementing the SPI: entity → `ConnectionHandler` mapping via the
   existing departure funnel (evict-old rule, deviation 2); global
   ambi-order config (deviation 1); mono-object pose fast path into
   `SetPosition`/`SetRotation`; binaural sink wiring; presence fed
   from the engine; mixer reading a **minimal base-profile subset**
   (existence marker, `name`, `attrs`, `mute`/`solo`). Old `core/moq`
   untouched; both servers side by side.
7. **Base-profile depth** (engine work, not protocol work — deferred
   from 6 to keep the structural work simple; wanted soon after):
   channel routing (source audible to sink iff shared unmuted
   channel), dof enforcement at ingest, moderation/block enforcement
   (dual-duty block keys), presence loudness measurement, render.*
   keys.
8. **Migration & cleanup.** WebRTC front end adopts the `state` engine
   (retiring `statecache`/bouncer and the duplicated
   `moq_data_writer`/`data_writer` visibility filters — the old and
   new state planes do not interoperate, so WebRTC must move rather
   than bridge); TS client speaks LASA; delete `core/moq`; update
   `cloud-mixer/architecture/containers.md` (its `covers:` includes
   `core/moq/`) and add a component doc for the LASA front end; keep
   `check_architecture_docs.sh` green. **Consciously out of scope
   until LASA is proven** — nothing in 1–7 depends on it.

Notes on the critical path: with deviation 1, ambi2/ambi3 multistream
sinks drop off the critical path entirely — binaural-only is a working
end-to-end system, ambi output joins when the engine side is ready.
Sink-format-in-track-name means ambi arrives later as purely additive
track names.

## Out of scope for this plan

- Head rotation / bilateral sink formats — deferred phase, re-enters
  via `head-rotation-and-sink-formats.md`.
- Relay/cache topologies.
- Any change to the panaudia render core beyond what step 7 lists.
