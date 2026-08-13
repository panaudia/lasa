# Identity implementation plan

*Written 2026-08-13. Implements `id-capabilities.md` (both same-day
decisions: title/exercise + newest-wins, and claims-as-state-keys).
Spec and profile text already updated (`lasa-core.md` §§4.4–4.6, 8, 9;
`lasa-base-profile.md` §§2, 3, 6). This plan is the code side. Batches
are ordered by dependency; A and B are independent of each other.*

Target behaviour, one paragraph: a Space is ticketed XOR unticketed;
admission writes `lasa.client.{id} = true` and entity markers valued
with the owning client id, adjudicated atomically on the state actor —
same-client collision supersedes (close incumbent generation + sweep +
apply, one actor step), ticketed cross-owner entity claims and
over-capacity refuse, unticketed collisions always supersede; claim
keys sweep immediately at disconnect; entry points hold advisory
replicas via observers and never adjudicate locally.

## Batch A — `profile/base`: builders — DONE 2026-08-13

- [x] `AdmissionGroup` takes the owning client id; marker value
      becomes the client id (JSON string), not `true`.
- [x] `base.ClientClaimGroup` (`lasa.client.{id}` = `true`, one Set,
      its own group) + `base.ClientKey`. Wired in `conn.go start()`:
      written **before** any entity group — one client key per
      connection, and the 64-op group cap (core §6.1) means the claim
      never shares a group with an attr-heavy entity.
- [x] Sweep scopes: `base.ClientSweepScope` passed with the
      per-entity `SweepScope`s in `teardown()`. *Ordering resolution
      (amends this bullet as first written):* the engine sorts one
      sweep group shortest-first/bytewise and `Conn.Close` is a
      single atomic call, so among equal-length roots the client key
      clears bytewise-first — "client key last" is not achievable
      without touching engine sort invariants, and is not needed: the
      transient mid-group state is benign because replicas are
      advisory (id-capabilities.md) and consume the sweep as one
      group. Documented at `ClientSweepScope`.
- [x] `consume.go`: verified tolerant as-is — bare markers return
      `(nil, nil)` before value inspection; `lasa.client.` falls
      through `ParseOp`'s default. No change needed.
- [x] Stale `AdmissionGroup` doc comment fixed (empty list defaults
      to `main` per §4).
- [x] Tests: builder units (claim group shape, scope coverage,
      participant-prefix exclusion), marker-value expectations
      updated, e2e departure test now asserts claim write + sweep
      and owner-valued marker. Full lasa + panaudia-server suites
      green.

## Batch B — `state`: the adjudicated admission primitive — DONE 2026-08-13

- [x] `Engine.Admit(AdmitRequest)` in `state/admit.go`: client id,
      rules, ticketed flag, capacity, profile-built claim/marker keys,
      admission groups. *Ordering amendment (improves on this bullet
      as first written):* adjudication is **validate-first** — the
      cross-owner refuse and the capacity refuse (checked net of
      everything supersession would free) run before any mutation, so
      a refused admission mutates nothing (no half-supersessions);
      only then supersede (own predecessor both modes; owning
      connections too when unticketed) → apply → register, all in one
      actor step. Sweep-before-apply per §4.4 / review-resolutions §7.
- [x] Ownership resolved via each admitted connection's recorded
      sweep scopes (`connOwning`) — the engine never parses values or
      invents key shapes; profile-blindness held.
- [x] Claim identity: admitted connections registered in an
      actor-owned client-id-keyed `conns` map beside `subs` /
      `rulesSeen`. (Full unification of attach onto the conn registry
      not needed yet — noted, not done.)
- [x] `Conn.Close` refactored onto the single close path
      (`closeConn`, shared with supersession); bare `Close()` sweeps
      the recorded claim set; no grace seam.
- [x] Supersession signal: `Conn.Done()` — closed with the
      generation, observed by whoever holds the conn; no actor→session
      callback. `Admit` also returns the superseded client ids for
      the shell's transport severs (batch C).
- [x] Unit tests (`state/admit_test.go`): fresh admit, supersede
      same-client (incl. L1 no-write from the superseded generation),
      observer collision, ticketed cross-owner refuse (side-effect-
      free, seq unchanged), unticketed cross-owner supersede (whole
      claim set), capacity (refuse / net-of-supersession / freed by
      bare Close), concurrent colliding admissions under `-race`.
      Full lasa + panaudia-server suites green.

## Batch C — `server` shell — DONE 2026-08-13

- [x] `New`: rejects `TicketKeys` + `AllowUnticketed` together
      (`TestNewRejectsMixedMode`, all four mode combinations).
- [x] `verifyTicket`: ticket presented to an unticketed space →
      `0x1A5A08` mode mismatch, replacing the accidental
      empty-key-set failure. Checked before verification — validity
      is irrelevant to a mode error.
- [x] `wire`: `ErrCodeModeMismatch = 0x1A5A08`; `0x1A5A03` marked
      retired-never-reuse.
- [x] `onClientSetup`: gate check first (unchanged), then the shell
      builds the profile writes (claim group + admission groups) and
      calls `Engine.Admit`, replacing `Backend.Admit` +
      `engine.Register`; `EntityOwnedError` → `0x1A5A04`,
      `ErrAtCapacity` → `0x1A5A07`.
- [x] `start()` reordered: registerLive FIRST, then the
      mid-handshake-window check (own generation already closed →
      abort — the ghost-invisible-to-sever race), then
      `severSuperseded` + teardown wait (bounded by `SetupTimeout`)
      before any `EntityJoined`; admission writes no longer happen
      here (they landed in Admit). *Found by test:* severSuperseded
      excludes the calling session — on same-client supersession the
      successor is already registered under the same id and severed
      itself (caught by `TestEvictOldSameClientReconnect`).
- [x] `SeverClient` deleted (replaced by internal `severSuperseded`;
      the gate's `severMatching` unchanged). Teardown closes a
      `torndown` channel last — the successor's wait signal; bare
      `econn.Close()` sweeps the recorded claim set, which also
      makes `abortAdmission` clean up Admit's writes correctly.

## Batch D — SPI + panaudia-server — DONE 2026-08-13

- [x] SPI: `Admit` deleted from `Backend` with a pointer comment; the
      embedder first hears of a connection at `EntityJoined`. A
      policy veto returns as a non-blocking hook if ever wanted —
      not carried speculatively.
- [x] `backend.Admit` deleted (collision scan, done-channel waits,
      re-check loop, `maxEntities`); `entityRec.done` deleted too —
      the shell waits on its own `torndown`, not backend state. The
      conn-map remains as pure mixer bookkeeping.
- [x] Capacity: `MaxEntities` → `server.Config.Capacity` →
      `Engine.Admit`. `TestAdmitAtCapacity` passes unchanged (same
      code `0x1A5A07`, now issued by the actor).
- [x] `transports` map + `transportOf` **deleted, not moved** —
      amends this bullet as first written: no callers existed (the
      per-route ingest profiles were retired by the v4 jitter work);
      `Resolved.Transport` still flows to any future consumer.
- [x] Tests: `TestEvictOldSameClientReconnect` and
      `TestEvictOldEntityCollision` pass as the unticketed
      newest-wins cases (comments updated); new
      `TestTicketedSameClientSupersedes`,
      `TestTicketedEntityCollisionRefused` (incumbent untouched),
      `TestObserverSupersession`, `TestModeMismatchTicketToOpenSpace`.
      Gate tests re-anchored from the stub's admitted-list to claim-
      key presence — a stronger observable of gate-before-Admit.
- [x] e2e assertions: owner-valued marker + claim-key lifecycle
      (batch A's departure test); engine-level read-scope exclusion
      is covered by the §2 rules (audience read grants don't match
      `lasa.client.`) — a dedicated delivery-path assertion rides
      with batch E's client work.
      Full suites green in both repos, `-race` clean on server+state.

## Batch E — ripple

- [ ] TS SDK / bridge: anywhere a marker is read, existence-only
      semantics must hold (value no longer `true`); ownership becomes
      available to server-side consumers. Thread when tickets reach
      those clients (next phase of the ticket work).
- [ ] `plan/` annotations referencing "Register becomes the claim
      point" (server-spi.md, state-engine-design.md, build ledger)
      point here — updated with this plan.
- [ ] Memory + `lasa-protocol-design.md` sibling note at completion.

## Recorded subtleties (do not lose)

- **Group cap**: the claim op precedes entity groups; never fold the
  claim into an entity's group (cap 64, attr-heavy entities).
- **Sweep order**: entity markers before client key on the way out;
  client key before entity groups on the way in.
- **The actor never re-enters session code** — supersession signals
  through the closed generation, moderation through observers; keep
  it that way.
- **Replicas never adjudicate.** Entry-point collections from
  observers are routing/diagnostic state. The only admission
  decision point is the actor.
- **Death detection is unchanged** — transport idle timeout remains
  the GC for uncontested ghosts; nothing here depends on its speed
  (supersession cuts through).
