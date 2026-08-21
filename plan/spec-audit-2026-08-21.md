# Spec audit 2026-08-21 — remaining findings ledger

A five-way audit of `lasa-core.md` + `lasa-base-profile.md` against
`plan/` and both codebases (lasa, panaudia-lasa). Findings came in
five sections. **A (spec lagged implementation, 17 items) and B
(smaller spec gaps, 8 items) were fully resolved 2026-08-21** —
docs, schemas, code and tests all updated; both repos green. The
decisions taken that day are recorded in the docs themselves (core
§1.1 base-profile-not-optional; §3 sink offering; §4.3 constrained
JWT profile, jti dropped; base profile §2 audience default
materialized, §5 no-effect wording + blanket admin immunity, dof
clear granted, initial-pose de-vocab'd).

This file holds the **unresolved** sections C, D, E. Work through
and mark items as they close.

## C — spec claims with no implementation counterpart

Decide each: spec-ahead-of-code (fine, maybe annotate), ticket it,
or implement.

- [x] **C1 `quality` server half** — CLOSED (verified 2026-08-21):
  the obligation lives in the engine and is implemented. panaudia-lasa
  backend passes `Quality`/`Redundancy` into `engine.SourceConfig`;
  `buffers.QualityLevel` (jitter_buffer.go:182) maps the level to
  the ingest buffer's {floor, robustness, capacity} (0=interactive/0,
  1=resilient/50ms, 2=playback/150ms) and `buffers.FECFloor`
  provisions depth from the declared redundancy offset, composing
  via max (mixer.go:273-274) — exactly §4.2's composition rule. The
  audit's verdict was scoped to the lasa repo only, where the field
  rightly just passes through to the backend.
- [x] **C2 uplink redundancy bound unenforced** — CLOSED as
  no-action (2026-08-21): the MUST binds the *sender*; §5.1's note
  already defines receiver behaviour under violation ("the redundant
  copy cannot be assumed usable" — not "must be ignored"). The
  depacketizer using a usable over-bound copy repairs a loss the
  listener would otherwise hear, and the wire's offset cap (7) means
  nothing can overflow. Ingest policing would discard repairs to
  punish an infraction the listener pays for.
- [x] **C3 presence receive-side rules missing from both SDKs** —
  CLOSED (implemented 2026-08-21): roster layer built in both SDKs.
  Go: `client/roster.go` (`Roster`, `RosterEntry`,
  `Client.SubscribeRoster`) + `roster_test.go` (8 behaviours); TS:
  `src/lasa/roster.ts` (`PresenceRoster`, `subscribeRoster`,
  optional `onUpdate`) + `tests/roster.test.ts` (mirrored). Both
  raw presence handlers now forward the datagram group-id
  (SIGNATURE CHANGE: Go `func(groupID uint64, msg any)`, TS
  `(groupId: bigint, msg)`). The bridge migrated onto the SDK
  roster (hand-rolled `onPresence`/gen tracking deleted, gaining
  latest-wins it previously couldn't implement). e2e presence test
  now exercises SubscribeRoster. All suites green: lasa Go, TS
  vitest 158, bridge.
- [x] **C4 mid-session visibility change** — CLOSED as
  spec-ahead-of-code (2026-08-21): the reference implementation has
  no dynamic rule surface — role tables are compiled in, ticket
  permissions fixed at presentation, and rule changes happen by
  restart, which mints a new epoch and snapshots everyone. The
  termination path is unreachable until a server grows
  runtime-mutable rules. *Reopen when a server gains dynamic rule
  configuration* (same trigger for the epoch-bump MAY).
- [x] **C5 discretionary snapshot never taken** — CLOSED as
  no-action (2026-08-21): both are MAY/non-normative by design. The
  discretion clause exists so a server can add the size heuristic
  later without a conformance question; *reopen if a real workload
  shows catch-up deltas rivalling snapshot size*. Buffer-and-replay
  is offered so third-party clients may pick the simpler strategy —
  one reference implementation of one strategy suffices.
- [x] **C6 presence registration is an unenforced embedder duty** —
  CLOSED (2026-08-21): design kept, obligation made normative in the
  SPI. Register cannot be a shell duty (it needs the backend-owned
  loudness callback and pose feed), and the failure modes justify
  the split: forgotten Register is immediately visible, forgotten
  Remove is a lingering ghost — so the shell enforces the invisible
  half. `server/spi.go` EntityJoined docs now state the MUST with
  the §7 citation.
- [x] **C7 single-ordered-stream fallback** — CLOSED by REMOVAL
  (2026-08-21): the allowance was deleted from §6.7 and the
  two-subgroup mapping made normative (lane = subgroup id; clients
  may depend on it). Rationale: every targeted MoQ version has
  subgroups, and the allowance was never sound anyway — the TS
  client already routes by subgroup id (state.ts CATCH_UP_LANE), so
  a single-stream server would have misapplied catch-up entries
  under the live rules.
- [x] **C8 loudness = post-render.gain** — CLOSED (verified
  2026-08-21): engine `Source.updateRenderLoudness`
  (engine/source.go:104-115, fed at render.go:125 with
  `e.params.Gain`) is rendered-audio RMS × entity render gain, EMA
  ≈100 ms, channel gains excluded, zero RMS → silence sentinel — the
  comment cites the profile rule verbatim.

## D — stale plan docs

Historical ledgers; patch or leave-with-supersession-notes.
**All ten patched 2026-08-21**: outright factual errors fixed in
place (D5 capacity code, D7 error range, D9 chunk estimate, D10 code
comment); superseded decisions got supersession/errata notes (D1–D4,
D6, D8), history preserved.

- [x] **D1 `plan/roles.md:33-36`** — sink-all grant described as
  admin-only and "not part of the union"; both stale (producer has
  it; admin's copy derives from the union). Also predates the
  2026-08-21 removal of the ambi format gate.
- [x] **D2 `plan/state-reconnect-and-gc.md:288,293`** — exclusive
  frontier wording ("nothing further **below** X"); spec+code are
  inclusive ("at or below"). state-engine-design.md invariant F5
  already flags this.
- [x] **D3 `plan/state-wire-encoding.md`** — `:64-65` "no separate
  key cap is pinned" (superseded: 512 bytes); `:14-22` subscribe-KVP
  layout missing the flags byte.
- [x] **D4 `plan/state-engine-design.md:137-139`** — "the spec caps
  key and value sizes but not group op-count (errata candidate)";
  the spec now caps it (§6.1, §9).
- [x] **D5 `plan/feature-parity-inventory.md`** — `:37` says
  capacity rejects with 0x1A5A05 (it is 0x1A5A07); `:83` claims "no
  role-targeted moderation keys" while `lasa.space.role.blocked.*`
  is exactly that.
- [x] **D6 `plan/lasa-state-keys.md`** — `:69-75` entity marker
  value `true` (superseded: owning client id, no supersession note);
  `:312-317` channel.gain "absence means no override" (that's the
  attenuation rule; gain absence = identity 1.0); initial-pose
  per-key entries now de-vocab'd (2026-08-21).
- [x] **D7 `plan/implementation-plan.md:39`,
  `plan/next-steps.md:292-293`** — error-code range still 00–07; now
  00–08.
- [x] **D8 `plan/audio-packet-structure.md:353-355`,
  `plan/head-rotation-and-sink-formats.md:6`** — "all three
  mandatory" superseded twice (server-build derived offering, then
  2026-08-21: binaural+ambi2 MUST / ambi3 SHOULD at order ≥3, no
  format role gate). `plan/panaudia-server-build.md:413-419`'s
  pinned ambi role gate is reversed — needs a supersession note.
- [x] **D9 `plan/presence-format.md:54`** — "~30 entities" per
  datagram; with UUID ids and chunkBytes=1150 it is ~20.
- [x] **D10 `connect/reject.go:26`** — code comment still references
  retired 0x1A5A03 as an issuable collision code.

## E — code defects found in passing

- [x] **E1 server-authored writes bypass the caps** — FIXED
  (2026-08-21): `Engine.Write` now validates keys and value sizes
  (loud errors — embedder misuse); over-cap groups from any server
  path (Write, sweep, admission) are applied via `applyChunked` —
  consecutive ≤cap groups within one actor event, so atomicity holds
  and delivery never sees an over-cap group from any author.
  `base.AdmissionGroup` rejects oversized config attrs at build
  (connection refused cleanly). Tests: TestServerWriteCaps,
  TestSweepChunked, TestAdmissionGroupValueCap.
- [x] **E2 TS state wire laxity** — FIXED (2026-08-21): `MAX_KEY_LEN`
  + §2 grammar regexes added; keys validated on encode and parse
  (shared `readKey` with explicit truncation error — no more subarray
  clamping); `canonicalPrefixes` validates the grammar, which makes
  the plain sort() bytewise by construction (valid prefixes are
  ASCII). Four new tests in statewire.test.ts; vitest 162 green.
- [x] **E3 presence delta chunk overflow** — FIXED (2026-08-21): both
  paths now check before appending with correct sizes (krecord
  21+len, delta record 21) and count their headers (keyframe 4B,
  delta 2B), so chunkBytes is a true upper bound on the emitted
  datagram. Pinned by TestChunkBytesIsUpperBound (120 UUID-id roster,
  keyframe + delta chunking).
- [x] **E4 sink render leak on Accept failure** — FIXED
  (2026-08-21): renders carry a pending count — ensure takes a hold,
  addPublisher converts it, new `release` undoes it on Accept
  failure (stopping the render only when no publisher AND no other
  pending hold remains), dropConn respects pending holds. Race-free
  against concurrent subscribes to the same (entity, format). Pinned
  by TestReleaseStopsUnpublishedRender.
- [x] **E5 ticketed admission rejects ownerless live records** —
  CLOSED, code-is-right (2026-08-21): the §4.4 principle (different
  capability → refuse the newcomer) covers the unattributable-marker
  case, and refusal surfaces the anomaly instead of masking it by
  silent claim. Spec wording widened to match: §4.4 bullet now reads
  "a live entity the connection does not own", §4.6's 0x1A5A04 row
  likewise. Code unchanged.
- [x] **E6 presence encode failures silently swallowed** — FIXED
  (2026-08-21): `Author.DroppedChunks()` counter, bumped in both
  flush paths — nonzero is an invariant violation (ids are
  admission-validated), surfaced through embedder stats; the library
  stays log-free by design. Test: TestDroppedChunksCounter.
- [x] **E7 u16 index truncation unchecked** — FIXED (2026-08-21):
  `maxRoster = 1<<16` cap applied in Author.Tick — excess entities
  are absent from presence (spec-legal space policy) instead of
  wrapping indexes onto other entities' records; capped roster is
  tick-stable. Test: TestRosterCappedAtU16.
- [x] **E8 Go/TS presence parser strictness divergence** — FIXED
  (2026-08-21): TS `parsePresence` now validates krecord ids against
  the §2 identifier grammar, throwing MalformedError so the discard
  rule drops the packet — same granularity as Go. Consistent with
  the E2 stance (TS tightened to Go's strictness).
- [x] **E9 retired code exposed in TS API** — FIXED (2026-08-21):
  `isLasaRejectionCode` excludes the retired 0x1A5A03 (MUST never
  appear on the wire); the constant and its name stay for
  documentation.
- [x] **E10 space id never grammar-validated** — FIXED (2026-08-21):
  `server.New` validates `SpaceID` against `ident.ValidIdentifier`
  and fails at construction with the grammar in the error. Test:
  TestNewRejectsInvalidSpaceID.

## Stray

- [x] `lasa-core.md:25` — leftover `// TODO` removed 2026-08-21 (the
  aside it marked was already complete).

---

**AUDIT COMPLETE 2026-08-21.** All findings across sections A–E
resolved: A (17, spec-lag) and B (8, smaller gaps) as decisions +
doc/schema/code edits; C (8, unimplemented claims) as 2
verified-implemented, 3 no-action with reopen triggers, 1 SPI
normative doc, 1 closed-by-removal (§6.7 fallback), 1 built (the
SDK presence roster layer); D (10, stale plan docs) patched with
supersession notes; E (10, code defects) all fixed with tests.
Suites green in lasa (Go + TS 163), panaudia-lasa, and
panaudia-bridge. Nothing committed — Paul does git.
