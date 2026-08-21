# Feature-parity inventory — old panaudia vs panaudia-server (I-A)

*Written 2026-08-05 (Phase D / D4). The greenfield plan's investigation
item I-A: what the frozen `panaudia` repo's PRODUCT surface is, what
`panaudia-server` already covers, what was dropped deliberately, and —
the point of the exercise — what must exist before putting old panaudia
into maintenance mode is honest.*

*Sources: full surface survey of `panaudia` @ `fd76cc8` (v0.3.0,
banner/env/endpoints/commands verified against code, not docs) and
`panaudia-server` @ `09770ac`. Operator-visible surface only; DSP
internals live in the engine PROVENANCE trail.*

**Status legend** — `CARRIED`: equivalent exists in panaudia-server.
`SUPERSEDED`: replaced by a different-by-design mechanism. `DROPPED`:
deliberately not carried (reason given). `GAP`: must be built before
maintenance mode. `OPEN`: needs a product ruling.

## 1. Env / config surface

Old panaudia's whole config is env vars echoed in a startup banner.
Notable survey findings: `PANAUDIA_TLS_CTR_PATH` is **misspelled in the
product** (CTR, not CRT); `PANAUDIA_TLS_AUTOGEN` and
`PANAUDIA_MIXER_GONUM` are live but undocumented; struct defaults say
`../keys/…` while the docs say `keys/…`; the documented MOQ client cap
(`MaxClients`) is **not actually enforced** — the real cap was the
fixed pool sizes.

| Old var | New var | Status |
|---|---|---|
| `PANAUDIA_HOST` / `PANAUDIA_PORT` | same names | CARRIED |
| `PANAUDIA_TLS_CTR_PATH` / `_TLS_KEY_PATH` | `PANAUDIA_CERT` / `PANAUDIA_KEY` | CARRIED (renamed — migration table §9 must map the old **typo'd** name) |
| `PANAUDIA_TLS_AUTOGEN` | implicit: unset cert ⇒ ephemeral self-signed + logged cert hash | CARRIED (new one is in-memory, never written to disk — better for dev, but operators who relied on the written files should use real certs) |
| `PANAUDIA_TICKET_KEY_PATH` (Ed25519 PEM file) | `PANAUDIA_TICKET_KEY` (base64 raw 32 B value) | CARRIED (renamed + format change; §9) |
| `PANAUDIA_UNTICKETED` | `PANAUDIA_ALLOW_UNTICKETED` (explicit opt-in since 2026-08-09; exactly one of key / opt-in) | CARRIED |
| `PANAUDIA_SPACE_ORDER` | `PANAUDIA_ORDER` (2–5; ambi offering derives) | CARRIED |
| `PANAUDIA_SPACE_MAX_SOURCES` (documented cap unenforced) | `PANAUDIA_MAX_ENTITIES` (enforced at admission, 0x1A5A07 at capacity) | CARRIED (and now real) |
| `PANAUDIA_REVERB_PRESET` (int 0–5, undocumented beyond 0) | `PANAUDIA_REVERB` (named: none/tight-room/small-room/medium-room/large-hall/cathedral) | CARRIED (named presets; **default differs**: old default 0=none, new default medium-room — §9 migration note) |
| `PANAUDIA_SPACE_SIZE` (cubic world scaling) | — | DROPPED: engine is metres-native (greenfield I-D); no world cube, distance model is absolute |
| `PANAUDIA_GOMAXPROCS` | — (`PANAUDIA_WORKERS` sizes the engine pool) | DROPPED: Go defaults + explicit worker knob |
| `PANAUDIA_ICE_HOST` / `_ICE_PORT` | — | DROPPED with WebRTC (§2) |
| `PANAUDIA_ENABLE_LINK_IN/_OUT` / `_LINK_PORT` | — | OPEN — Link/ROC, §8 |
| `PANAUDIA_LOG_MS` | minute stats log line + `stats()` snapshot | SUPERSEDED |
| `PANAUDIA_LOG_LEVEL` (5-level stdout logger) | plain `log` | GAP (small): pick a logging discipline before production ops |
| `PANAUDIA_TEST_TONE/_TEST_PEOPLE/_TEST_VOICES/_STEREO_TEST` | test/bench harnesses (capacity smoke 50×50, chirp/pose) | SUPERSEDED: synthetic load lives in the repos' harnesses, not the binary. Revisit only if ops want in-binary smoke sources |
| `PANAUDIA_CACHE_RING_SIZE/_SEGMENT_CAPACITY/_TOMBSTONE_TTL_SEC` | — | SUPERSEDED: the LASA state plane replaced the topic caches (tombstone TTL exists in lasa state GC design) |
| `PANAUDIA_MIXER_GONUM` (runtime GEMM override) | engine defaults (darwin=gonum after the Accelerate/AMX signal-corruption finding) + `-tags xsmm` | CARRIED in effect (compile-time rather than env; fine) |
| `PANAUDIA_ENV_FILE` + `.env` loading (+ macOS bundle path rebasing) | plain env only | DROPPED: 12-factor env; `.env` convenience can return with packaging if wanted |
| `PANAUDIA_BILATERAL` (deprecated tripwire) | — | n/a (already dead) |
| `QLOGDIR` (per-conn QUIC qlogs) | not wired | GAP (small, diagnostic): wire qlog through quic-go config when debugging needs it |

## 2. Transports & endpoints

| Old | New | Status |
|---|---|---|
| MOQ over raw QUIC (`moqt-16`/`moq-00`) + WebTransport (`h3`) on one UDP port, ALPN-routed | same shape (D1): `moqt-16` raw QUIC + `h3`→WT upgrade at `/lasa`, one UDP port | CARRIED (draft-16 fork; old `moq-00` era dropped with the old protocol) |
| WebRTC (TCP-only ICE, `/join` WS signalling, hardcoded Google STUN, server-offer SDP, 3 data channels) | — | DROPPED: LASA is WT/QUIC-native. WebRTC existed as the no-WebTransport fallback; WT now ships in all four target engines (Safari 26+). Consequence: **no fallback for older Safari/iOS** — accepted; revisit only if real users surface there. Also removes the hardcoded-STUN external dependency |
| TCP byte-peek demux (TLS vs STUN) + Alt-Svc header | — | DROPPED with WebRTC (no TCP listener at all today) |
| `GET /probe`, `GET /hello` (health, plain HTTPS) | — | GAP: deployments need a health endpoint; trivial to add alongside a stats endpoint (§5) |
| H3 SETTINGS shims for Safari/newer drafts | webtransport-go v0.12 handles current drafts; Chromium proven in CI | OPEN (I-B remnant): validate Safari + Firefox WT against panaudia-server before maintenance mode |
| Old MOQ track namespace (`in/audio/opus-mono/{uuid}` …) | LASA namespaces (entity/client/presence, format-in-name tracks) | SUPERSEDED by the protocol itself |
| Join query params as the de-facto API (uuid/name/ticket/pose/attrs/subspaces…) | Connection Config (0x1A5A1) at CLIENT_SETUP: entities, setups, ticket, initial-pose, attrs, render fields | SUPERSEDED — one authenticated config instead of query strings |
| Subspaces (UUID list per node) | channels (state-plane vocabulary, per-pair matrix) | SUPERSEDED |
| QUIC tuning constants (idle 5 s, keepalive 2 s, windows) | fork/server defaults | CARRIED in spirit; compare under load when the prod box exists |

## 3. Identity & tickets

| Old | New | Status |
|---|---|---|
| Ed25519-signed JWT, signature-only verification, custom `panaudia` claim (name/uuid/gain/attenuation/priority/roles/attrs) | LASA ticket (schema'd), verification-only server, one Ed25519 public key, exp gates admission only | CARRIED (S3.5) — stricter and schema'd |
| Unticketed mode (query-string identity) | `PANAUDIA_ALLOW_UNTICKETED` + App-space-unrestricted rules | CARRIED |
| Single-session-per-identity eviction at admission | S2 funnel: evict-old via SeverClient, waited + re-checked | CARRIED |
| Ticket minting: out of repo (panaudia-client `tickets/` examples + schema target the OLD claim shape) | out of repo; `connect.Mint` in-process for tests; mint CLI deferred | GAP (ecosystem): panaudia-client's ticket schema/examples/docs must gain LASA versions before external developers can mint for the new server |

## 4. Moderation & roles

| Old | New | Status |
|---|---|---|
| 20-command catalog on the control channel (one state op per command) | state ops on reserved keys directly (commands-as-state-ops decision) | SUPERSEDED |
| `space.entity.mute/unmute`, `set_gain`, `set_attenuation` | `lasa.entity.<id>.mute` / render keys via profile consumer (S4) | CARRIED |
| `space.entity.kick/unkick` (mins, 0=forever) | block key with unix-seconds expiry, 0=forever; severs live + refuses readmission (L4/S5) | CARRIED |
| `personal.entity.mute/unmute/solo/unsolo` | pair record (P1): personal mute BIDIRECTIONAL (deliberate change), solo beats personal mutes, space moderation above all | CARRIED (semantics deliberately improved; old solo-bypasses-space-mute retired) |
| `space.role.*` / `personal.role.*` (role-TARGETED moderation: mute/kick/gain a whole role at once) | `lasa.space.role.blocked.{role-name}` (block/kick a whole role, unix-seconds expiry — profile §5); no role-wide mute/gain | CARRIED (block/kick); role-wide mute/gain dropped — add to the vocabulary if ever missed |
| Hardcoded role table (everyone/admin/moderator/performer/audience, recompile to change) | profile/base §2 tables as code (audience/presenter/producer/moderator/admin), wildcard scopes, ticket-carried | CARRIED (still code-defined, but the ticket carries literal grants too — strictly more expressive) |
| Read scopes (`entity.all`, `space.read`) | prefix subscriptions + RulesFor read hash | SUPERSEDED |

## 5. Stats & observability

Survey finding worth keeping: old panaudia's rich `MixerStats` JSON
schema is **dormant** — zero call sites in the standalone binary. The
old server's real observability was the startup banner + stdout lines.

| Old | New | Status |
|---|---|---|
| Startup banner (version, full config echo, transport table, licence paths, HRIR set name) | boot logs (cert hash, WT address) but no banner | GAP (small): a config-echo banner is cheap and operators lean on it |
| Log lines: departures w/ reasons, stale-reconciler kills, shutdown drain, per-conn connect/disconnect | funnel logs exist; conventions differ | CARRIED in substance |
| `ms:` render-time line + over-budget WARN | minute stats line (per-tick breakdown, max jitter fill, ingress loss) + `stats()` snapshot incl. per-entity LatencySamples/DepacketizerStats | CARRIED (richer) |
| No HTTP stats/health/metrics endpoint (despite dormant schema) | none yet; `stats.go` notes "an HTTP surface can wrap the same snapshot" | GAP: add `/probe` + `/stats` (JSON of the existing snapshot). This EXCEEDS old parity but is the honest bar for replacing it in deployments |
| Version embed (`version` file → banner + image tag) | none | GAP (small): version stamp in binary + boot log |

## 6. Audio features (operator-visible)

| Old | New | Status |
|---|---|---|
| Order 2–5 (forced 2/3 with Link-out) | Order 2–5, ambi offering derives (ambi2 always, ambi3 at ≥3) | CARRIED |
| Reverb presets 0–5 (only 0 documented) | named presets, all documented | CARRIED |
| HRTF: single embedded set (thk-ku100-bimagls-v1), no runtime selection | same posture (engine-embedded set) | CARRIED (SOFA loading was future work in both worlds) |
| Per-node gain/attenuation `[0,3]` via query/ticket + commands | per-entity render fields via config/ticket/state; gain now amplitude-domain (deliberate fix — old √gain was internally inconsistent) | CARRIED (semantics deliberately corrected) |
| Opus egress fixed 96 kbps stereo (SDP advertised 48k — old mismatch) | sink opus encode in engine/server | OPEN (small): confirm and pin the new egress bitrate; expose if product wants it |
| Raw ambi out via ROC only (N3D/SN3D per connection) | `ambi2`/`ambi3` sink TRACKS over MoQ (SN3D), role-gated | SUPERSEDED (better reach; D5 browser receiver is the delivery story) |
| VAD / noise suppression: none | none | parity |
| New-only: extent/size/directivity, head-frame entities, bilateral render, presence track, state plane | — | (not parity items; the new server exceeds here) |

## 7. Deployment & distribution

| Old | New | Status |
|---|---|---|
| amd64 Dockerfile (multi-stage, xsmm static, non-root, CAP_NET_BIND_SERVICE, licences in image; **no ENTRYPOINT**; clones repo in-build; no arm64 despite README claim) | none yet | GAP: prod packaging (the deferred prod-box program). Fix the old quirks rather than inheriting them: add ENTRYPOINT, build from local source, decide arm64 honestly |
| macOS `.app` bundle (+ sign/notarize scripts, checked-in DMGs) | none | OPEN: was the mac-app distribution load-bearing, or does the browser client (D4 page / D5 capabilities) replace local-server use cases? |
| Licence files shipped + banner paths | LICENSE in repos | GAP (small): third-party licence aggregation at packaging time |
| Graceful shutdown (SIGINT/SIGTERM drain, 5 s budget) | server closes on signal via lasa teardown | CARRIED (verify drain-vs-abrupt behaviour when packaging) |
| `/probe` healthcheck for orchestrators | — | GAP (same as §5) |

## 8. Panaudia Link / ROC and the client ecosystem

| Old | New | Status |
|---|---|---|
| Panaudia Link: plain-HTTP WS signalling (`/roc-input`, `/roc-output`, no auth by design), RTP+FEC media on UDP 20000+, order forced ≤3 for out, per-connection SN3D/N3D | — | OPEN (the biggest single item): the D5 direction — browser as ambi receiver + browser as multi-track object sender over WT — is the planned replacement for Link's use cases (native macOS app, DAW stems). Decide after D5 validates: (a) D5 covers it → record Link as superseded; (b) pro/native paths still need it → port Link (or a successor) to panaudia-server. Note old Link's unauthenticated-by-design posture should NOT be carried as-is into a ticketed world |
| TS SDK (old `@panaudia/client`) | `@panaudia/lasa-client` (D2–D4: transport, state, full 3-thread audio, demo page) | CARRIED for browsers (packaging pass pending) |
| Unreal plugin (panaudia-client/sdks/unreal) | — | GAP (ecosystem): port or explicitly scope out before maintenance mode; native clients pin old-server lifetime |
| Legacy WebRTC JS SDK (deprecated already) | — | DROPPED (was already deprecated) |
| panaudia-client docs/schemata/tickets (public developer surface, old protocol) | — | GAP (ecosystem): LASA versions of the ticket schema, docs, examples (overlaps §3) |

## 9. Operator migration notes (to publish with the swap)

- Env renames: `PANAUDIA_TLS_CTR_PATH`→`PANAUDIA_CERT` (note the old
  name's CTR typo — copy exactly when migrating away from it),
  `PANAUDIA_TLS_KEY_PATH`→`PANAUDIA_KEY`,
  `PANAUDIA_TICKET_KEY_PATH` (PEM file)→`PANAUDIA_TICKET_KEY`
  (base64 raw key value), `PANAUDIA_UNTICKETED`→`PANAUDIA_ALLOW_UNTICKETED`,
  `PANAUDIA_SPACE_ORDER`→`PANAUDIA_ORDER`,
  `PANAUDIA_SPACE_MAX_SOURCES`→`PANAUDIA_MAX_ENTITIES`,
  `PANAUDIA_REVERB_PRESET` (int)→`PANAUDIA_REVERB` (name).
- Behaviour changes: reverb default none→medium-room; `.env` files no
  longer read; no TCP listener (WebRTC/HTTPS gone); max-entities now
  actually enforced; personal mute now bidirectional; gain now an
  amplitude multiplier.
- Dropped without replacement (deliberate): SPACE_SIZE world cube,
  WebRTC + ICE/STUN surface, `.env`/bundle path magic, in-binary test
  tone/load flags, GOMAXPROCS knob.

## 10. The maintenance-mode bar

Old panaudia can honestly go maintenance-mode when:

1. **Health + stats HTTP surface** exists (`/probe`, `/stats` wrapping
   the existing snapshot) — §5.
2. **Ticket ecosystem** speaks LASA: schema + docs + mint examples in
   panaudia-client (mint CLI per the deferred list) — §3/§8.
3. **The Link decision is made** — D5 validation first; supersede or
   port — §8.
4. **Browser matrix validated**: Safari + Firefox WT against
   panaudia-server (I-B remnant) — §2.
5. **Prod packaging** exists (Docker incl. licences + version stamp +
   ENTRYPOINT; the prod-box program) — §7.
6. **Unreal (and any other native) client story** decided: ported or
   explicitly out of scope — §8.
7. Small items swept: logging discipline, config-echo banner, egress
   bitrate pinned, role-targeted-moderation ruling, mac-app ruling —
   §1/§4/§5/§6/§7.

Everything else in this inventory is either already carried or
deliberately, documentedly dropped.
