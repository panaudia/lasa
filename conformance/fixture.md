# Fixture contract

*Written 2026-09-03 from the evidence of extracting the e2e corpus
into this package (plan/conformance-suite.md §2.3 and §2.6 step 3).
Every item is a declared fact about the implementation under test,
never a control over it. An implementation that cannot supply an
item is reported "not exercised" for the rows that need it, not
failed.*

The harness reaches an implementation through `Fixture` (fixture.go),
filled from the `LASA_CONFORM_*` environment for an external server
or built in-process for the reference. The items below are the ones
the extracted corpus turned out to need, sorted by how many rows
depend on them.

| Item | Rows | How the reference supplies it | Evidence |
|---|---|---|---|
| The URL and transport | every row | one listener per transport | every test dials |
| The space id | every row | `Config.SpaceID` | the config's `client_id` binding and every namespace tuple carry it |
| The mode, and for a ticketed space the issuer signing key | ticket refusals (C-4.3-*, C-4.4-1, C-4.4-4, P-2-2), moderation (P-5-*, C-4.4-9), roles (P-2-1) | the reference boots one space per mode and hands the harness the private key it generated | the harness mints tickets, so it holds the key rather than a set of pre-minted tokens. Rows of the other mode report not exercised |
| The declared internal render order | C-3-3, C-5.3-4 | `Config.AmbiOrder`, and a private reference at order 3 | `ambi3` is a SHOULD at order 3 or higher, so the harness needs the number to know which outcome is right |
| A way to stop the space deliberately | C-4.6-7 shutdown half | `Fixture.Shutdown` on a private reference | an external fixture has none, so its row reports not exercised |

Items the plan anticipated that no extracted row needed yet, and
which therefore stay out of the contract until a row asks:

- A declared capacity (C-4.4-8). The unit test in package `state`
  covers it and no e2e test reaches it from the wire.
- A tombstone TTL, or a way to shorten it (C-6.6-7, C-6.6-10).
- A restart that keeps or discards the store (C-6.6-4, C-6.6-8,
  P-3-5).

Two facts about the reference that are not fixture items but that
an implementer should know: the base profile is the one in
`profile/base`, wired exactly as `panaudia-lasa` wires it, and the
stub backend behind the reference renders counter frames rather than
audio, so no row here decodes Opus.

## Environment for an external server

```
LASA_CONFORM_URL=host:port          raw QUIC, or https://host/path for WebTransport
LASA_CONFORM_SPACE=<space id>
LASA_CONFORM_ISSUER_KEY=<hex>       Ed25519 private key or seed; presence means ticketed
LASA_CONFORM_ISSUER=<iss>           default lasa-conform
LASA_CONFORM_ORDER=<n>              declared internal render order
LASA_CONFORM_INSECURE=1             skip TLS verification
LASA_CONFORM_REPORT=<path>          write the per-assertion JSON report
go test ./conformance/
```

Without `LASA_CONFORM_URL` the suite runs against the in-repo
reference in both modes on both transports.
