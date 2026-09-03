# Conformance matrix

*First matrix, built by hand on 2026-09-03 as step 2 of
`lasa-planning/plan/conformance-suite.md` §2.6. Every row is an
assertion id carried in `lasa-core.md`, `lasa-base-profile.md` or
`lasa-diagnostics.md`,
and `matrix_test.go` holds this file and the spec text to the same
id set. The test column is a mapping by test name, unverified until
the e2e corpus is extracted (step 3). Rows the uplink observer
(step 4, `observer.go`) closes name `TestUplinkObserver`. The spec text is the source
of truth for ids. Add or retire an assertion in the spec and in this
file together, and the test refuses the pair when they differ.*

## Conventions

- An id follows the sentence, list item or table cell that states
  the obligation, as `[C-4.6-3]` after the terminating punctuation.
  Where one sentence carries obligations on two parties it carries
  two ids.
- Ids are section-scoped and never renumbered. A retired assertion
  keeps its id with a retired note. Prefixes: `C` core, `P` base
  profile, `D` diagnostics.
- **Party**: S server, C client, B both as codec peers, I issuer.
- **Obs**: W wire-observable, F needs a fixture item, X by
  inspection only.
- **Debug**: the §8 event or sample that corroborates the assertion.
  An assertion passes on its wire effect and never on a debug
  message alone (core §8.5).
- **Tests**: `conformance:` names a test in this package that exercises the row from the wire. A SHOULD the implementation does not follow reports as `advisory`, not `fail`. The rest are names in the repo's unit and e2e corpus, unverified.
- **Triage**: the step 3 sort, see the table at the end.



### Core §2

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-2-1 | One identifier grammar `[a-z0-9-]{1,128}` covers every embedded id. A violation in a config rejects with `0x1A5A00` | S | W |  | TestValidIdentifier, TestParseConnectionConfigRejects | hostile client later |
| C-2-2 | Issuers SHOULD mint client and entity ids as lowercase UUIDs | I | X |  |  | inspection |
| C-2-3 | Implementations MUST NOT assume structure beyond the grammar. A non-UUID id is admitted | S, C | W |  |  | hostile client later |
| C-2-4 | State keys are `[a-z0-9.-]`, at most 512 bytes | S | W |  | conformance: TestStateKeyRules; TestValidKey, TestSilentDrops | closed |
| C-2-5 | A ticket's `aud` MUST equal the space id | S | W |  | conformance: TestRejectionCodes; TestTicketVerifyRejects | closed |

### Core §3

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-3-1 | The three namespace shapes and the track-name table are exactly as given | S | W |  | conformance: TestSinkOffering; TestNamespaceTupleRoundTrip, TestE2E* | closed |
| C-3-2 | A Space MUST offer `binaural` and `ambi2` for every entity that has a sink | S | W | subscribe.rejected(no-such-track) | conformance: TestSinkOffering; TestE2ESinkFormatOffering | closed |
| C-3-3 | A Space SHOULD offer `ambi3` when its render order is 3 or higher | S | F |  | conformance: TestSinkOffering, TestSinkOfferingAtOrder3; TestE2EAmbi3AtOrder3 | closed |
| C-3-4 | Formats carry no authorization dimension. A connection permitted a sink may take any offered format | S | W |  | conformance: TestSinkOffering | closed |
| C-3-5 | A subscription to a sink that does not exist is refused | S | W | subscribe.rejected(no-such-track) | conformance: TestSinkOffering | closed |

### Core §3.1

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-3.1-1 | The subscribe authorization table: owner (plus profile grants) for entity sinks, own client only for state and debug sinks, anyone for presence, the space only for sources | S | W | subscribe.rejected(unauthorized) | conformance: TestSinkOffering; TestAuthzMatrix, TestE2EUnauthorizedSink | closed |
| C-3.1-2 | A client MAY announce only its own `source` namespaces. An out-of-binding subscribe or announce MUST be rejected | S | W | subscribe.rejected(unauthorized) |  | hostile client later |

### Core §3.2

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-3.2-1 | The server renders a sink only while at least one subscription is active | S | X | sample entity.render | TestReleaseStopsUnpublishedRender | inspection |
| C-3.2-2 | A new sink subscription joins at live with no history | S | W |  | conformance: TestSinkIdsAcrossChurn | closed |
| C-3.2-3 | Concurrent formats on one sink are permitted, each flowing during overlap | S | W |  | conformance: TestSinkConcurrentFormats | closed |
| C-3.2-4 | On entity departure its sink tracks end and live subscriptions are closed with SUBSCRIBE_DONE | S | W |  | conformance: TestSinkEndsOnDeparture; TestDropEntityEndsTracks | closed |

### Core §4.1

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-4.1-1 | The config is presented once, as CLIENT_SETUP parameter `0x1A5A1`, UTF-8 JSON | C | W |  | TestE2E* | client harness later |
| C-4.1-2 | The config is validated against `connection.schema.json`. A violation rejects with `0x1A5A00` | S | W |  | conformance: TestRejectionCodes; TestParseConnectionConfigRejects | closed |
| C-4.1-3 | At least one of `entities` and `setups` MUST be present, and both MAY be | S | W |  | conformance: TestRejectionCodes | closed |
| C-4.1-4 | On failure the server terminates without completing SERVER_SETUP | S | W |  | conformance: TestRejectionCodes; TestWTRejectionCarriesCode | closed |
| C-4.1-5 | Behaviour is identical over WebTransport and raw QUIC | S | W |  | conformance: TestTransportParity; wt_test.go | closed |
| C-4.1-6 | A missing setup parameter rejects with `0x1A5A00` | S | W |  |  | hostile client later |

### Core §4.2

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-4.2-1 | Every `setup.id` MUST reference an entity in the ticket. Unknown or duplicate ids reject | S | W |  | conformance: TestRejectionCodes; TestResolveRejections | closed |
| C-4.2-2 | Instantiating a subset of ticket entities is permitted | S | W |  | conformance: TestTicketReuseAndSubset; TestResolveSetupsInstantiateSubset | closed |
| C-4.2-3 | Ad-hoc entities are admitted when unticketed or when the ticket authorises them, else `0x1A5A05` | S | W |  | conformance: TestRejectionCodes; TestResolveAdHocPermittedWithTicket, TestAdHocPermitted | closed |
| C-4.2-4 | No duplicate id within `setups`, within `entities`, or between an ad-hoc entity and any ticket-defined id | S | W |  | TestResolveRejections | hostile client later |
| C-4.2-5 | A duplicate id inside the ticket's own entities block rejects the ticket with `0x1A5A01` | S | W |  |  | hostile client later |
| C-4.2-6 | Zero-entity connections are legal and may subscribe presence, own state sink and granted sinks | S | W |  | conformance: TestObserver; TestResolveZeroEntityObserver, TestZeroEntityBinding | closed |
| C-4.2-7 | An entity definition requires `id` and `name` | S | W |  |  | hostile client later |
| C-4.2-8 | `quality` is 0 to 2 default 0, `redundancy` is 0 to 7 default 0, both fixed for the connection | S | W |  |  | hostile client later |
| C-4.2-9 | The space SHOULD honour the sink redundancy request within policy | S | W |  | conformance: TestSinkRedundancy; TestSinkRedundancyEmission | closed |
| C-4.2-10 | A sender MUST NOT emit an offset exceeding the declaration (uplink) | C | W | sample entity.ingest discarded(offset-over-declaration) | conformance: TestUplinkObserver; TestUplinkRedundancyClamp | closed |
| C-4.2-11 | A sender MUST NOT emit an offset exceeding the declaration (sink) | S | W |  | conformance: TestSinkRedundancy; TestSinkRedundancyOffDefault | closed |
| C-4.2-12 | `lockstep` on the config declares one shared frame counter across every entity source track the connection instantiates | C | W |  | conformance: TestUplinkLockstep | closed |
| C-4.2-13 | The space buffers a lockstep connection's sources as one stream provisioned at the members' maximum `quality` and `redundancy` | S | X |  |  | with the lockstep implementation |
| C-4.2-14 | A client SHOULD NOT declare lockstep for sources captured on separate clocks | C | X |  |  | inspection |

### Core §4.3

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-4.3-1 | The signature MUST verify against the configured issuer keys, any of several | S | F |  | conformance: TestRejectionCodes; TestTicketVerifyRejects, TestTicketMultipleIssuerKeys | closed |
| C-4.3-2 | `aud` MUST be the single-string form, never the array form | S | W |  |  | hostile client later |
| C-4.3-3 | `exp`, `nbf` and `iat` are enforced when present. Absent `exp` never expires | S | W |  | conformance: TestRejectionCodes; TestTicketVerifyRejects | closed |
| C-4.3-4 | Claims are held to the schema exactly. Any other member, `sub`, `azp`, `jti` included, rejects | S | W |  | TestTicketVerifySchemaEnforced | hostile client later |
| C-4.3-5 | `iss`, `aud` and `panaudia` are required | S | W |  | TestTicketVerifySchemaEnforced | hostile client later |
| C-4.3-6 | `panaudia.client_id` MUST equal the config's `client_id`, else `0x1A5A02` | S | W |  | conformance: TestRejectionCodes; TestParseClaimsRejects | closed |
| C-4.3-7 | Role names are held to the identifier grammar. Unknown names are refused per profile with `0x1A5A01` | S | W |  | conformance: TestRejectionCodes; TestValidRoles | closed |
| C-4.3-8 | Effective access is the union of role rules and ticket permissions | S | W |  | conformance: TestRulesUnion; TestRulesFor | closed |
| C-4.3-9 | Only configured signature algorithms are accepted, never `none` | S | W |  | TestTicketAlgorithmAllowlist | hostile client later |

### Core §4.4

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-4.4-1 | In a ticketed Space every connection MUST present a ticket. None rejects with `0x1A5A01` | S | W |  | conformance: TestRejectionCodes; TestE2ETicketRequired, ticketed.spec | closed |
| C-4.4-2 | In an unticketed Space a config presenting a ticket MUST be rejected with `0x1A5A08` | S | W |  | conformance: TestRejectionCodes; ticketed.spec | closed |
| C-4.4-3 | Ticketed, same `client_id` live: newest wins, incumbent severed with `0x1A5A0A` | S | W |  | conformance: TestSupersession; TestAdmitSupersedesSameClient, TestE2ESupersededCarriesCode | closed |
| C-4.4-4 | Ticketed, entity id collides with a live entity not owned: reject with `0x1A5A04`, bystander never severed | S | W |  | conformance: TestRejectionCodes; TestAdmitTicketedRefusesCrossOwner | closed |
| C-4.4-5 | Unticketed, any collision: newest wins, incumbent superseded wholesale | S | W |  | conformance: TestSupersession; TestAdmitUnticketedSupersedesCrossOwner | closed |
| C-4.4-6 | An unticketed config carries `entities`, not `setups` | S | W |  |  | hostile client later |
| C-4.4-7 | Cleanup owed by a superseded connection MUST complete or cancel before the replacement's admission writes | S | W |  | conformance: TestSupersessionSweepOrder; TestSweepBeforeReplacementWrites | closed |
| C-4.4-8 | Collision and capacity refusals issue from one atomic point. Capacity rejects with `0x1A5A07` | S | F |  | TestAdmitCapacity, TestAdmitConcurrentCollisions | fixture item later |
| C-4.4-9 | The block refusal runs after ticket verification and before adjudication, so a blocked id never reaches supersession | S | W |  | conformance: TestBlocks; TestE2EBlockedReconnectCannotEvict | closed |
| C-4.4-10 | A zero-entity connection exercises its client capability like any other (collides on client id) | S | W |  | conformance: TestSupersession; TestAdmitObserverCollision | closed |

### Core §4.5

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-4.5-1 | Every connection presents a full config. A ticket remains valid for reconnection until `exp` | S | W |  | conformance: TestTicketReuseAndSubset | closed |
| C-4.5-2 | Continuity exists only in the state cursor. Media tracks carry none | S, C | W |  | conformance: TestSinkIdsAcrossChurn, TestUplinkObserver; covered by C-5.2 and C-6.6 | closed |

### Core §4.6

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-4.6-1 | Rejection is a session close carrying a code and reason on both transports | S | W |  | conformance: TestRejectionCodes; TestWTRejectionCarriesCode | closed |
| C-4.6-2 | Codes fit 32 bits and sit in the LASA range | B | W |  | conformance: TestBlocks, TestRejectionCodes, TestSupersession; 'MOQ error code tables' (TS) | closed |
| C-4.6-3 | Correctness MUST ride the code alone. A client MUST NOT require parsing the reason | C | W |  |  | client harness later |
| C-4.6-4 | Each code is emitted for its tabled cause and no other | S | W |  | TestE2E* (partial) | hostile client later |
| C-4.6-5 | `0x1A5A03` MUST NOT be reused | S | X |  |  | inspection |
| C-4.6-6 | The blocked reason SHOULD carry the block expiry | S | W |  | conformance: TestBlockReasonExpiry | closed |
| C-4.6-7 | An established session is closed with `0x1A5A06`, `0x1A5A0A` or `0x1A5A09` on live block, supersession or shutdown | S | W, F |  | conformance: TestBlocks, TestShutdownCode, TestSupersession; TestE2EBlockedWhileLiveCarriesCode, TestE2EShutdownSeversWithCode | closed |
| C-4.6-8 | A session ending for any other reason carries no LASA code | S | X |  |  | inspection |
| C-4.6-9 | After `0x1A5A09` the client MAY reconnect later | C | X |  |  | inspection |

### Core §5.1

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-5.1-1 | Group-id increments by exactly one per packet, object-id is 0, no gaps. Pose-only packets increment too | C, S | W |  | conformance: TestSinkContinuity, TestUplinkObserver | closed |
| C-5.1-2 | Sink tracks are continuous at 200 packets per second regardless of content | S | W |  | conformance: TestSinkContinuity | closed |
| C-5.1-3 | `offset = 3` MUST mean exactly 15 ms | B | X |  | follows from C-5.1-1 | inspection |
| C-5.1-4 | Every fixed-width field is little-endian. Varints are RFC 9000 | B | W |  | golden vectors | golden vectors |
| C-5.1-5 | Flags bit assignments are as tabled | B | W |  | golden vectors | golden vectors |
| C-5.1-6 | Reserved bits MUST be zero on emission | C, S | W |  | conformance: TestUplinkObserver | closed |
| C-5.1-7 | A receiver MUST discard packets with unknown bits set | C, S | W | sample entity.ingest discarded(reserved-bits) | TestAudioDiscardRule, TestDepacketizerMalformedDiscard | hostile client later |
| C-5.1-8 | Bit 2 clear with a nonzero offset is discarded | C, S | W | sample entity.ingest discarded(offset-without-flag) | TestAudioDiscardRule | hostile client later |
| C-5.1-9 | Bit 2 set with a zero offset is discarded | C, S | W | sample entity.ingest discarded(zero-offset) | TestAudioDiscardRule | hostile client later |
| C-5.1-10 | Pose is 18 bytes, f32 xyz then int16 yaw pitch roll at ±π/32767 | B | W |  | TestPoseGoldenEncode, TestPoseGoldenDecode, TestPoseAngleClamp | golden vectors |
| C-5.1-11 | The mono-object pose is the freshest at assembly time | C | X |  |  | inspection |
| C-5.1-12 | With the redundancy bit set, a u16 length prefix precedes the primary and a full copy of frame `offset` ago follows. Without it, no prefix | B | W |  | conformance: TestUplinkObserver; TestMonoObjectGolden, TestUplinkRedundancyContinuousAudio | closed |
| C-5.1-13 | The referenced packet MUST be audio-bearing | C, S | W |  | conformance: TestUplinkObserver; TestUplinkRedundancyPoseOnlyReference | closed |
| C-5.1-14 | The uplink carries no timestamps | C | W |  | conformance: TestUplinkObserver | closed |
| C-5.1-15 | Sink ts is u64 unix microseconds of the first sample when bit 6 is set, emitted on start and about once per second, and `ts(seq)` is linear at 5 ms | S | W |  | conformance: TestSinkContinuity; TestSinkGolden | closed |
| C-5.1-16 | The ts anchor is per track instance and re-established after reconnect | S | W |  | conformance: TestSinkIdsAcrossChurn | closed |

### Core §5.2

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-5.2-1 | An uplink reconnect is a new track instance and group-ids restart at 0 | C | W |  | conformance: TestUplinkObserver | closed |
| C-5.2-2 | Sink ids start at 0 when rendering starts and run across subscriber churn. A joining subscription attaches at the current id | S | W |  | conformance: TestSinkIdsAcrossChurn | closed |
| C-5.2-3 | All per-track receiver state is scoped to the (session, track) instance and resets with it | C, S | W |  |  | client harness later |
| C-5.2-4 | On a lockstep connection every member emits at every frame (flags `0x00` when it has nothing), the counters never diverge, and all restart at 0 together | C | W |  | conformance: TestUplinkLockstep | closed |

### Core §5.3

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-5.3-1 | The three layouts are as given, and every section after flags is present iff its bit says so | B | W |  | golden vectors | golden vectors |
| C-5.3-2 | A mono-object packet carries one 5 ms mono Opus frame at 48 kHz. Flags `0x00` is legal and advances the sequence | B | W |  | conformance: TestUplinkObserver | closed |
| C-5.3-3 | A binaural packet carries one 5 ms stereo Opus frame | S | W |  |  | acoustic harness later |
| C-5.3-4 | `ambi2` is 9 and `ambi3` is 16 uncoupled mono streams in one RFC 7845 multistream packet, ACN order, SN3D | S | W |  | conformance: TestSinkOfferingAtOrder3; TestE2EAmbi3AtOrder3 | closed |

### Core §6.1

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.1-1 | Two operations, `set` and `clear`. Values are opaque, at most 64 KiB | S | W |  | TestSilentDrops | hostile client later |
| C-6.1-2 | Every op gets a seq from one monotonic incrementing run | S | W |  | conformance: TestSeqAndFlatStore | closed |
| C-6.1-3 | Group op count is capped. An over-cap group is dropped whole, never partially applied | S | W | write.dropped(group-cap) | TestServerWriteCaps, TestGroupDeliveredWhole | hostile client later |
| C-6.1-4 | The store is flat. Clearing a parent removes nothing | S | W |  | conformance: TestSeqAndFlatStore | closed |
| C-6.1-5 | Clients MUST ignore an op whose seq is at or below the current one for that key | C | W |  | TestResurrectionGuard, store.test.ts | client harness later |

### Core §6.2

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.2-1 | Upstream ops carry no seq. The server assigns on acceptance | B | W |  | conformance: TestSeqAndFlatStore; 'upstream ops without seq' (TS) | closed |
| C-6.2-2 | An accepted op is echoed with its seq on the writer's own state sink | S | W |  | conformance: TestStateEchoAndReconnect; TestEchoCorners | closed |
| C-6.2-3 | Silent drop on parse failure | S | W | write.dropped(parse) | TestSilentDrops | hostile client later |
| C-6.2-4 | Silent drop on key grammar or 512-byte violation | S | W | write.dropped(key) | TestSilentDrops | hostile client later |
| C-6.2-5 | Silent drop on value over 64 KiB | S | W | write.dropped(value) | TestSilentDrops | hostile client later |
| C-6.2-6 | Silent drop on group over cap | S | W | write.dropped(group-cap) | TestServerWriteCaps | hostile client later |
| C-6.2-7 | Silent drop when denied by role or permission rules | S | W | write.dropped(denied) | TestSilentDrops | hostile client later |
| C-6.2-8 | Silent drop when the op arrives after the disconnect sweep has begun. A superseded connection's in-flight writes are never applied for the successor | S | W | write.dropped(swept) | TestSweptGenerationWriteDropped | hostile client later |
| C-6.2-9 | A write under a `set` or `clear` grant without `read` is applied and its echo dropped | S | W | write.unechoed(no-read) | TestEchoCorners | hostile client later |
| C-6.2-10 | A write outside the subscribed prefixes produces no echo on that connection | S | W | write.unechoed(out-of-scope) | TestOutOfScopeAdvancesLiveFrontier | hostile client later |
| C-6.2-11 | Clients SHOULD subscribe to what they write | C | X |  |  | inspection |
| C-6.2-12 | Silent drop when the key matches no write grant | S | W | write.dropped(no-grant) | TestSilentDrops | hostile client later |

### Core §6.3

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.3-1 | A client holds exactly one state subscription with prefixes fixed at subscribe time | S | W |  |  | raw layer later |
| C-6.3-2 | A key matching several prefixes is delivered once | S | W |  |  | raw layer later |
| C-6.3-3 | `[""]` subscribes to everything. An empty set is a protocol error | S | W | subscribe.rejected(empty-set) | TestSubscribeParamsRejects | hostile client later |
| C-6.3-4 | Filtering is plain string prefix, so `foo` matches `foobar` | S | W |  |  | hostile client later |

### Core §6.4

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.4-1 | Both directions are filtered through one merged rule set | S | W |  | conformance: TestRulesUnion | closed |
| C-6.4-2 | Access scopes match exact key or dot-extended prefix, with a trailing dot stripped | S | W |  | TestScopeMatches | hostile client later |
| C-6.4-3 | The empty scope is the universal grant | S | W |  | TestScopeMatches | hostile client later |
| C-6.4-4 | A `*` segment matches exactly one identifier segment | S | W |  | TestScopeMatches | hostile client later |
| C-6.4-5 | Wildcards are legal only in server-expanded rules. Ticket permissions remain literal | S | W |  |  | hostile client later |

### Core §6.5

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.5-1 | On subscribe the server answers with exactly one of delta or snapshot | S | W | sample subscription.state |  | raw layer later |
| C-6.5-2 | A delta carries only the latest op per in-scope key with seq above the cursor, compacted, groups not preserved | S | W |  | conformance: TestStateEchoAndReconnect; TestSimReconnectDelta | closed |
| C-6.5-3 | A snapshot carries per-key seqs and never contains tombstones | S | W |  |  | raw layer later |
| C-6.5-4 | On snapshot the client discards its local store across subscribed prefixes and adopts it | C | W |  | TestSnapshotDeletionRule | client harness later |
| C-6.5-5 | The server MUST snapshot when it cannot prove a delta complete, and MAY otherwise | S | W | sample subscription.state | TestCatchUpNeverLeaksPastH | raw layer later |
| C-6.5-6 | Every catch-up closes with a frontier carrying an explicit `(epoch, seq)`, inclusive | S | W |  | TestEmptyDeltaYieldsFrontier | raw layer later |
| C-6.5-7 | Frontiers are emitted periodically during live operation | S | W |  |  | raw layer later |
| C-6.5-8 | Delivery is live-first. Live ops flow immediately and catch-up streams alongside, uncoordinated | S | W |  |  | raw layer later |
| C-6.5-9 | Client rule 1: apply live ops under per-key dedup | C | W |  | store.test.ts | client harness later |
| C-6.5-10 | Client rule 2: apply a catch-up entry only if its seq exceeds the local per-key seq | C | W |  | store.test.ts | client harness later |
| C-6.5-11 | Client rule 3: when a snapshot completes, delete in-scope local keys absent from it with seq at or below the cut. Absence from a delta means unchanged | C | W |  | TestSnapshotDeletionRule | client harness later |

### Core §6.6

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.6-1 | The cursor `(epoch, seq, set-hash)` is persisted atomically and never ahead of the persisted store | C | X |  | TestPersistRoundTrip | inspection |
| C-6.6-2 | A client that does not persist its store MUST NOT persist a cursor | C | W |  |  | client harness later |
| C-6.6-3 | No cursor is persisted before the first catch-up frontier has been applied | C | W |  | conformance: TestStateEchoAndReconnect; TestCursorNotBeforeFirstCatchUpFrontier | closed |
| C-6.6-4 | Epoch is 8 opaque bytes compared for equality. The server mints a new one whenever continuity is unprovable | S | F |  | TestEpochChange, TestSimCrashMidCatchUp | fixture item later |
| C-6.6-5 | Set-hash is SHA-256 over the canonical prefix encoding | B | W |  | TestSetHashDefinition | hostile client later |
| C-6.6-6 | With frontiers on both paths the client persists the maximum seq per epoch | C | W |  |  | client harness later |
| C-6.6-7 | Tombstones are flushed after a TTL. Neither TTL nor watermark is advertised, and clients handle either response | S, C | F |  | TestGCFlushMovesWatermark | fixture item later |
| C-6.6-8 | Gate 1: epoch mismatch snapshots | S | F | sample subscription.state gate=1 | TestEpochAndSetHashGates | fixture item later |
| C-6.6-9 | Gate 2: set-hash mismatch snapshots | S | W | sample subscription.state gate=2 | TestEpochAndSetHashGates | hostile client later |
| C-6.6-10 | Gate 3: seq below the watermark snapshots | S | F | sample subscription.state gate=3 | TestGCFlushMovesWatermark | fixture item later |
| C-6.6-11 | Gate 4: unprovable visibility since the cursor snapshots | S | W | sample subscription.state gate=4 | TestGate4RulesHash | hostile client later |
| C-6.6-12 | Gate 5: seq ahead of head at the same epoch snapshots | S | W | sample subscription.state gate=5 | TestGateSemantics | hostile client later |
| C-6.6-13 | Gate 6: otherwise delta, or snapshot at discretion | S | W | sample subscription.state gate=6 | TestGateSemantics | hostile client later |
| C-6.6-14 | A fresh client presents nothing and lands on snapshot | S | W | sample subscription.state gate=0 | TestSimFreshSyncAndLive | raw layer later |
| C-6.6-15 | A mid-session visibility change terminates the state subscription via MoQ termination, keeping session and audio up | S | F | state.terminated(visibility-change) | TestLaggedTerminatesSubscriptionOnly (related) | fixture item later |
| C-6.6-16 | After such termination the client re-subscribes and hits the gate | C | W |  |  | client harness later |
| C-6.6-17 | Global rule-set changes MAY be an epoch bump plus termination of all state subscriptions | S | X |  |  | inspection |

### Core §6.7

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-6.7-1 | The subscribe parameter is KVP `0x1A5A3` with the given layout | B | W |  | TestSubscribeParamsGolden | hostile client later |
| C-6.7-2 | Every varint in the parameter MUST be minimal | C | W |  | TestSubscribeParamsRejects | hostile client later |
| C-6.7-3 | A server MUST reject a non-canonical prefix set (duplicated, unsorted, non-minimal) or a non-minimal cursor seq | S | W | subscribe.rejected(non-canonical) | TestSubscribeParamsRejects, TestCanonicalPrefixes | hostile client later |
| C-6.7-4 | A fresh client sends flags 0 and omits the cursor block | C | W |  |  | client harness later |
| C-6.7-5 | Message encodings for `set`, `clear`, `group`, `frontier`, `begin` are as given, and groups do not nest | B | W |  | TestStateGolden, TestStateParseRejects | hostile client later |
| C-6.7-6 | Implementations MUST select the layout by track direction | B | W |  | TestStateDirectionMatters | hostile client later |
| C-6.7-7 | `begin` opens a catch-up closed by a `frontier`, and snapshot entries are `set`s with seqs | S | W |  |  | hostile client later |
| C-6.7-8 | The server SHOULD preserve a written group's boundaries in live delivery and never in catch-up | S | W |  | TestGroupDeliveredWhole | raw layer later |
| C-6.7-9 | Clients MUST NOT depend on group boundaries | C | W |  |  | client harness later |
| C-6.7-10 | Each subscription instance is a fresh MoQ group. Subgroup 0 carries live ops and periodic frontiers, subgroup 1 the catch-up. Object-ids increment within each subgroup | S | W |  | 'catch-up ordering errors' (TS) | hostile client later |
| C-6.7-11 | The source track is one group, one subgroup, ordered ops | C | W |  |  | client harness later |
| C-6.7-12 | Clients MAY identify the lane by subgroup id | C | X |  |  | inspection |

### Core §7

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-7-1 | Presence datagrams are group-per-packet, object 0, incrementing by one | S | W |  | conformance: TestPresence; TestE2EPresence | closed |
| C-7-2 | Flags bit 0 marks a keyframe. Other bits are MBZ with the discard rule | S, C | W |  | TestPresenceRejects | hostile client later |
| C-7-3 | Delta and keyframe layouts are as given | B | W |  | conformance: TestPresence; TestPresenceDeltaGolden, TestPresenceKeyframeGolden | closed |
| C-7-4 | `kflags` bit 0 is head-frame, bits 1 to 7 MBZ, keyframes only | B | W |  | TestRosterHeadFrameCarried | hostile client later |
| C-7-5 | Records parse to the end of the datagram. Loudness is u8 at −0.5 dB per step, 255 silence | B | W |  | TestLoudness, TestPresenceMultiRecordRoundTrip | golden vectors |
| C-7-6 | Indexes are defined by keyframes. A keyframe is one or more chunks, each with `first` | S, C | W |  | conformance: TestPresence; TestAuthorKeyframeChunking, TestRosterChunkedKeyframe | closed |
| C-7-7 | Generation increments only on membership change | S | W |  | conformance: TestPresenceGeneration | closed |
| C-7-8 | A delta whose gen does not match the last keyframe MUST be discarded | C | W |  | TestRosterStaleGenerationDiscarded | client harness later |
| C-7-9 | The server SHOULD emit a keyframe immediately on membership change | S | W |  | conformance: TestPresenceGeneration | closed |
| C-7-10 | All per-index state resets on gen change | C | W |  | TestRosterGenerationChangeResets | client harness later |
| C-7-11 | Records are latest-wins by group-id per index | C | W |  | TestRosterLatestWinsByGroupID, TestRosterLateKeyframeChunkDoesNotRewind | client harness later |
| C-7-12 | Departure is keyframe absence. Deltas never signal departure | S, C | W |  | conformance: TestPresence | closed |
| C-7-13 | An unknown index in a delta is ignored | C | W |  | TestRosterUnknownIndexIgnored | client harness later |
| C-7-14 | The index is never authoritative and may change across generations | C | X |  |  | inspection |
| C-7-15 | Head-frame entities appear on presence with the flag set | S | W |  | conformance: TestPresence | closed |

### Core §8

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-8-1 | The entity marker's value names its owning client and is readable by every participant | S | W |  | conformance: TestAdmissionKeysAndSweep | closed |

### Core §9

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| C-9-1 | Wire constants are as tabled (parameter keys, code range, 512, 65536, group cap, epoch width, frame) | B | W |  | covered piecewise above | golden vectors |

### Base profile §1

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-1-1 | Values under `lasa.` are UTF-8 JSON Boolean, String or Number | S | W |  |  | hostile client later |
| P-1-2 | Attrs flatten to dotted leaf paths. Arrays anywhere in attrs are invalid | S | W |  | TestAttrArraysRejected | hostile client later |

### Base profile §2

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-2-1 | Five roles exist. Absent or empty roles, and every unticketed connection, grant exactly `audience` | S | W | sample session roles | conformance: TestBlocks; TestDefaultRoles | closed |
| P-2-2 | A role name outside the vocabulary rejects with `0x1A5A01` | S | W |  | conformance: TestRejectionCodes; TestValidRoles | closed |
| P-2-3 | `audience` writes `attrs.*`, `mute.*`, `solo.*` on its own entities only | S | W | write.dropped(denied) | TestRulesFor, TestComposeAgainstProfileText | hostile client later |
| P-2-4 | `audience` reads `lasa.entity.` and `lasa.space.`, never `lasa.client.` | S | W |  | conformance: TestAdmissionKeysAndSweep; TestParseOpTable | closed |
| P-2-5 | `presenter` may carry ad-hoc entities in the config | S | W |  | conformance: TestRoles; TestAdHocPermitted | closed |
| P-2-6 | `moderator` writes `dof` on any entity, `space.entity.muted.*`, `space.client.blocked.*` | S | W |  | TestComposeAgainstProfileText | hostile client later |
| P-2-7 | `producer` writes the tabled `heard-in`, `hears`, `priority`, `render.*` and `space.*` keys on any entity | S | W |  | TestComposeAgainstProfileText | hostile client later |
| P-2-8 | `producer` may subscribe any entity's sink | S | W |  | conformance: TestRoles; TestSinkAllGrant, TestSinkAll | closed |
| P-2-9 | `admin` is the union of all four | S | W |  | conformance: TestRoles; TestComposeAgainstProfileText | closed |
| P-2-10 | An out-of-range value is accepted, echoed and delivered, then ignored by consumers | S | W, X | value.ignored(out-of-range) |  | hostile client later |
| P-2-11 | Ticketed connections have no application-space access without `permissions`. Unticketed spaces are unrestricted there | S | W |  |  | hostile client later |
| P-2-12 | Ticket `permissions` may target `lasa.` prefixes as a fine-grained grant | S | W |  |  | hostile client later |

### Base profile §3

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-3-1 | Connection-scoped keys are swept at disconnect with ordinary `clear` ops, immediately, no grace | S | W |  | conformance: TestAdmissionKeysAndSweep; TestE2EDepartureSweep | closed |
| P-3-2 | The server SHOULD clear the existence marker first or in the same group | S | W |  | TestSweepMarkerFirstGroup | raw layer later |
| P-3-3 | Claim record presence equals connectedness | S | W |  | conformance: TestAdmissionKeysAndSweep | closed |
| P-3-4 | Space-scoped keys survive reconnects | S | W |  | conformance: TestSpaceScopedSurvives | closed |
| P-3-5 | Servers SHOULD durably persist `lasa.space.*` across restarts | S | F |  |  | fixture item later |

### Base profile §4

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-4-1 | Source is audible to sink iff they share at least one unmuted channel | S | W |  | server repo engine tests (acoustic harness later) | acoustic harness later |
| P-4-2 | Omitted or empty `heard-in` or `hears` becomes `["main"]`, materialised at admission | S | W | sample session entities | conformance: TestAdmissionKeysAndSweep | closed |
| P-4-3 | Gain composes multiplicatively with the highest channel gain, unset counting as identity | S | W |  | server repo engine tests (acoustic harness later) | acoustic harness later |
| P-4-4 | The lowest set channel attenuation replaces the source's, otherwise the source's applies | S | W |  | server repo engine tests (acoustic harness later) | acoustic harness later |

### Base profile §5

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-5-1 | `space.entity.muted` silences a source in every render and never the connection | S | W |  | acoustic harness later | acoustic harness later |
| P-5-2 | A `client.blocked` or `role.blocked` value of 0 is forever, otherwise until that unix-seconds time | S | W |  | conformance: TestBlocks; TestE2EBlockedAtConnect | closed |
| P-5-3 | Setting a block terminates any live matching connection with `0x1A5A06` | S | W |  | conformance: TestBlocks; TestE2EBlockedWhileLive | closed |
| P-5-4 | A block gates admission until expiry | S | W |  | conformance: TestBlocks; TestE2EBlockedAtConnect, TestE2ERoleBlockedAudienceDefault | closed |
| P-5-5 | Admin is unblockable, and `role.blocked.admin` has no effect | S | W |  | conformance: TestBlocks | closed |

### Base profile §6

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-6-1 | `lasa.client.{id}` is set at admission before any entity group and swept at disconnect, value `true` | S | W |  |  | inspection |
| P-6-2 | `lasa.client.{id}` is in no role's read scope and clients never receive it | S | W |  | conformance: TestAdmissionKeysAndSweep | closed |
| P-6-3 | Consumers MUST NOT depend on the client record beyond presence | C | X |  |  | inspection |
| P-6-4 | `lasa.entity.{id}` is server-written only, value the owning client id | S | W |  | conformance: TestAdmissionKeysAndSweep | closed |
| P-6-5 | All other keys under an entity prefix MUST be ignored while its marker is absent | C | W |  |  | client harness later |
| P-6-6 | `name` is server-written from config and immutable | S | W |  | conformance: TestStateEchoAndReconnect | closed |
| P-6-7 | `attrs.*` are owner-writable at runtime. `signed-attrs.*` come from the ticket and are immutable | S | W |  | conformance: TestAttrs | closed |
| P-6-8 | Admission always writes at least one `heard-in` and one `hears` key | S | W | sample session entities | conformance: TestAdmissionKeysAndSweep | closed |
| P-6-9 | `render.*` defaults and ranges are as tabled | S | X |  |  | inspection |
| P-6-10 | A head-frame entity's own sink subscription is refused | S | W | subscribe.rejected(no-such-track) | conformance: TestHeadFrameSinkRefused | closed |
| P-6-11 | Head-frame entities are absent from ambisonic sinks and render in binaural only | S | W |  | acoustic harness later | acoustic harness later |
| P-6-12 | `dof` is enforced at ingest, and presence reflects the enforced pose | S | W | sample entity.ingest pose-ignored | conformance: TestPoseRules; head_test.go (server repo) | closed |
| P-6-13 | `initial-pose` is the entity's pose until the first accepted packet pose, and is never materialised into state | S | W |  | conformance: TestPoseRules | closed |
| P-6-14 | Mute is bidirectional, solo wins over mute, space mute sits above both | S | W |  | server repo engine tests (acoustic harness later) | acoustic harness later |
| P-6-15 | Mute and solo reset on reconnect | S | W |  | conformance: TestMuteResetOnReconnect; follows from P-3-1 | closed |

### Base profile §7

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| P-7-1 | All entities appear on presence, head-frame ones flagged | S | W |  | conformance: TestPresence | closed |
| P-7-2 | Loudness is post-`render.gain`, pre-attenuation, one per source | S | W |  | acoustic harness later | acoustic harness later |


### Diagnostics §1 Conventions

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| D-1-1 | Readers MUST ignore unknown kinds and unknown members | S, C | X |  |  | with the diagnostics implementation |
| D-1-2 | `detail` MUST NOT be parsed for program logic | S, C | X |  |  | with the diagnostics implementation |
| D-1-3 | Every scope is sampled on subscribe, then on change at most once per second, whole value each time | S, C | W |  |  | with the diagnostics implementation |
| D-1-4 | A publisher SHOULD bound debug buffering, drop rather than stall, and report losses as `debug.dropped` | S, C | W |  |  | with the diagnostics implementation |
| D-1-5 | A debug track MUST NOT alter its publisher's protocol behaviour | S, C | X |  |  | with the diagnostics implementation |

### Diagnostics §2 The Space's debug track

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| D-2-1 | A client MUST NOT depend on the debug track | C | X |  |  | with the diagnostics implementation |
| D-2-2 | The `debug` track is subscribable by the owning connection only | S | W | subscribe.rejected(unauthorized) |  | with the diagnostics implementation |
| D-2-3 | The debug subscribe carries no LASA SUBSCRIBE parameter | C | W |  |  | with the diagnostics implementation |
| D-2-4 | The track has no upstream half and carries no commands | S | X |  |  | with the diagnostics implementation |
| D-2-5 | One group per subscription instance, subgroup 0, object-ids incrementing by one, one JSON document per object | S | W |  |  | with the diagnostics implementation |
| D-2-6 | A debug subscription joins at live with no history beyond the opening samples | S | W |  |  | with the diagnostics implementation |
| D-2-7 | Event kinds, members and cause vocabularies are as tabled | S | W |  |  | with the diagnostics implementation |
| D-2-8 | One `write.dropped` per dropped message, `key` present whenever known | S | W |  |  | with the diagnostics implementation |
| D-2-9 | Sample scopes and members are as tabled, and a sample carries the whole value | S | W |  |  | with the diagnostics implementation |
| D-2-10 | On subscribe every existing scope is sampled, `session` first, before any event | S | W |  |  | with the diagnostics implementation |
| D-2-11 | Discard cause names are the §5.1 rules and the cursor epoch is sixteen lowercase hex characters | S | W |  |  | with the diagnostics implementation |
| D-2-12 | The track names only the connection's own inputs and entities | S | W |  |  | with the diagnostics implementation |
| D-2-13 | A Space offering the debug track reports resolved `heard-in` and `hears` in the `session` sample | S | W | sample session entities |  | with the diagnostics implementation |
| D-2-14 | A Space offering the debug track counts dof-ignored poses in the ingest sample | S | W | sample entity.ingest pose-ignored |  | with the diagnostics implementation |

### Diagnostics §3 The client's debug track

| Id | Assertion | Party | Obs | Debug | Tests | Triage |
|---|---|---|---|---|---|---|
| D-3-1 | A client MAY publish the client debug track. No Space behaviour depends on it and no client obligation is satisfied by it | C | X |  |  | with the diagnostics implementation |
| D-3-2 | The client track is subscribable by the Space only | C | W |  |  | with the diagnostics implementation |
| D-3-3 | A client publishes nothing on the track until the Space subscribes | C | W |  |  | with the diagnostics implementation |
| D-3-4 | One group per subscription instance, subgroup 0, object-ids incrementing by one, opening samples then live | C | W |  |  | with the diagnostics implementation |
| D-3-5 | Client sample scopes and members are as tabled | C | W |  |  | with the diagnostics implementation |
| D-3-6 | The store digest is SHA-256 over live entries sorted by key in the given encoding | C | W |  |  | with the diagnostics implementation |
| D-3-7 | Client discard cause names and the cursor epoch encoding are the server track's | C | W |  |  | with the diagnostics implementation |
| D-3-8 | `close.received` is the first event of the next subscription instance | C | W |  |  | with the diagnostics implementation |
| D-3-9 | A client reports the close code and never acts on the reason phrase | C | W, X |  |  | with the diagnostics implementation |
| D-3-10 | The client track narrates the publishing client's own state only | C | W |  |  | with the diagnostics implementation |

## Totals

| Class | Count |
|---|---|
| Assertions | 247 |
| Server party (S alone or with another) | 184 |
| Client party (C alone or with another) | 63 |
| Wire-observable (W) | 216 |
| Needs fixture (F) | 10 |
| By inspection (X) | 24 |
| Debug-corroborated | 41 |
| With a named test today | 172 |
| With no test found by name | 75 |

## Triage after the polite-client pass (2026-09-03)

| Triage | Count |
|---|---|
| closed | 94 |
| hostile client later | 56 |
| with the diagnostics implementation | 29 |
| client harness later | 19 |
| inspection | 16 |
| raw layer later | 11 |
| acoustic harness later | 8 |
| fixture item later | 7 |
| golden vectors | 6 |
| with the lockstep implementation | 1 |

"Closed" means a test in this package exercises the row from the
wire on the reference, both transports. "Raw layer later" rows need
the two state lanes (begin, frontier, subgroup ids), which the polite
client hides but which are not refusals. "Golden vectors" rows are
codec layouts held byte-identical across Go and TypeScript by
`typescript/tests/vectors/lasa-wire.json`. "Hostile client later" rows
are refusals and codec rules that only a raw MoQ client can provoke.
"Client harness later" rows are client obligations the permissive
server tests. "Polite client, open" rows are server obligations the
existing client could reach and nobody has written the test for yet.

## Changes from Appendix A

Made while moving the inventory into the spec, 2026-09-03.

- Core §8 and §9 of Appendix A are core §9 and §10 since the debug
  track landed. `C-8-1` became `C-9-1` and `C-9-1` became `C-10-1`.
  The debug track carries `C-8-1` to `C-8-17`, all new.
- `C-6.2-12` is new. Appendix A folded "denied by rules" and
  "matches no write grant" into `C-6.2-7`, and the spec lists them as
  two causes with two debug causes, so they are two ids.
- `P-4-5` and `P-6-16` are new, the base profile's two debug-track
  sentences.
- Five sentences were added to the spec where Appendix A asserted
  something the text only implied. Each is one clause and matches
  the reference implementation: schema validation and the missing
  parameter reject with `0x1A5A00` (`C-4.1-2`, `C-4.1-6`), the
  signature-algorithm allowlist (`C-4.3-9`), each code for its
  tabled cause (`C-4.6-4`), an unknown presence index is ignored
  (`C-7-13`), and the wire constants are normative (`C-10-1`).
- Seven sentences were split so that one id follows one sentence:
  the UUID SHOULD and the no-structure MUST NOT, the MUST and SHOULD
  sink formats, the no-gaps and 200 pps rules, the reserved-bits
  emit and discard rules, the cursor atomicity and the no-store
  rule, the visibility termination and the re-subscribe, and the
  block's effect and its value encoding.
- Reclassified X to W, per `plan/debug-track.md` §3: `P-4-1`,
  `P-4-3`, `P-4-4`, `P-5-1`, `P-6-11`, `P-6-14`, `P-7-2`. They are
  observable by an acoustic harness (publish a known tone, subscribe
  another sink, measure) and stay mapped to the server repo's engine
  tests until one exists.
- Nothing was struck.
- Lockstep (2026-09-03, `plan/lockstep.md`): `C-4.2-12` to `C-4.2-14`
  and `C-5.2-4` arrived with the spec text, the observer's in-step
  check closing the two sender rows from day one.
- The polite-client pass (2026-09-03, later): 28 more rows closed
  with the existing client. `C-2-4` is closed on its positive half
  only, since the polite client refuses to encode a key outside the
  grammar, and its refusal half waits for the raw layer. Eleven rows
  that need the state lanes moved to "raw layer later", six codec
  rows to "golden vectors". The reference server's blocked reason
  now carries the expiry (`C-4.6-6`, a SHOULD it did not follow).
- Later on 2026-09-03 both debug tracks moved out of the core into
  `lasa-diagnostics.md`. The core's §8 and §9 are Security and Wire
  constants again, so `C-9-1` and `C-10-1` went back to `C-8-1` and
  `C-9-1`, and the seventeen `C-8-*` rows plus `P-4-5` and `P-6-16`
  were re-keyed as `D-1-*` and `D-2-*`. The client track's rows
  `D-3-*` are new. This was the last free renumbering: nothing had
  been committed.
