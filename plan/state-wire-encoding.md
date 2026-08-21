# State wire encoding — decision

*Decided 2026-07-27. The byte-level shape of the state machinery
functionally specified by `state-reconnect-and-gc.md`. Tracks
(`track-naming.md`): ops upstream on `(space, "client", c, "source")` /
`state`, the personalised feed downstream on
`(space, "client", c, "sink")` / `state` — MoQ streams, never
datagrams (the frontier promise requires ordered reliable transport).*

## The subscribe message is MoQ SUBSCRIBE parameters

There is no LASA subscribe message. The MoQ SUBSCRIBE to the sink
track carries one KVP (unregistered key space, as for the Connection
Config parameter) containing:

- the requested prefix set, in its already-pinned canonical encoding
  (deduplicated, sorted, length-prefixed — the same bytes the SHA-256
  is computed over);
- the cursor `(epoch, seq)` — absent for a fresh client;
- the 32-byte set-hash of the earned set — absent for a fresh client.

*(As pinned in `lasa-core.md` §6.7, the KVP begins with a flags byte
— bit 0 = cursor present — ahead of these fields, and every varint in
the parameter must be minimal.)*

Changing the set is MoQ unsubscribe + re-subscribe, natively. The
delta/snapshot answer is **in-band** (the `begin` message below), not
a SUBSCRIBE_OK parameter — response extension params are poorly
surfaced by implementations, and the response is the head of a data
stream anyway.

## Messages

One message per MoQ object: `[type u8][body]`. QUIC varints for all
lengths, counts and seqs. Epoch is 8 opaque bytes (equality-only).

```
set       [1][key-len][key][value-len][value]([seq])
clear     [2][key-len][key]([seq])
group     [3][count][ops...]                 the consistency hint, ops inline
frontier  [4][epoch 8][seq]
begin     [5][kind u8]                       kind: 1 = delta, 2 = snapshot
```

- **seq is downstream-only.** Upstream ops carry none (the server
  assigns; the echo is the acknowledgement). The same type bytes
  therefore parse differently per direction — the tracks are
  directional by name, but a shared direction-blind parser is a known
  trap; implementations must select the layout by track direction.
- **`begin`** opens a catch-up payload and makes the delta/snapshot
  distinction explicit (the absent-key deletion guard is
  snapshot-only). The payload is ordinary `set`/`clear` messages —
  snapshot entries are `set`s with per-key seqs — closed by a
  `frontier` carrying the cut.
- **Group framing**: the server SHOULD preserve a written group's
  boundaries in live delivery (its ops get contiguous seqs anyway) and
  never preserves them in catch-up (compaction shreds them, as already
  specified). Clients MUST NOT depend on group boundaries — they are
  hints.

## Value size cap: 64 KiB

**A value is at most 65,536 bytes.** This is a coordination KV, not a
blob store; the cap keeps any single op small against the catch-up
subgroup and bounds what one writer can wedge into every subscriber's
feed. An oversized write is dropped silently — the same no-echo
signal as a role-rejected op (`lasa2.md`'s no-error-channel rule).
Keys are already bounded in practice by the key grammar; no separate
key cap is pinned. *(Superseded: keys are capped at 512 bytes —
review resolutions 2026-07-27, spec §2/§9.)*

## MoQ mapping: live and catch-up are subgroups

**Sink track.** Each subscription instance is a fresh MoQ group
(the stream analogue of session-scoped group-ids,
`audio-packet-structure.md` §9). Within it:

- **subgroup 0 — live ops**: begins at subscribe, runs for the life of
  the subscription; carries live `set`/`clear`/`group` messages and
  the periodic frontiers.
- **subgroup 1 — catch-up**: `begin`, the payload, the closing
  `frontier`; then ends.

Each subgroup is its own QUIC stream: ordering is guaranteed within
each path — all the frontier promise needs, since it is a per-path
promise (the catch-up frontier promises the catch-up payload complete;
the live path only moves forward) — and backpressure on a large
snapshot cannot stall live ops. This is the wire realisation of
live-first delivery: the server never coordinates the paths, the
client's already-specified rules (per-key dedup, apply-if-newer, the
≤ H guard) do the merge. Object-ids increment monotonically within
each subgroup.

**Source track.** One group, one subgroup, ordered ops,
fire-and-observe-echo. No cursor, no frontier, no catch-up — the sink
is the subscription (`track-naming.md` §Knock-ons).

**Fallback recorded**: if subgroup support proves awkward in a given
MoQ stack, a single ordered stream carrying the same messages
interleaved is conformant-equivalent — the client rules are unchanged.
(Eyevinn v0.8.2 has subgroup support; checked 2026-07-27.)

## Open items

None.
