# Track naming & structure — decision

*Decided 2026-07-24. The source and pose tracks merge into one per-entity
media track; namespaces are typed tuples with an entity-centric direction
segment; track names are format names; presence stays one shared track.*

## The decisions

### 1. Source/pose merge

The separate `pose` track is dropped. Each entity publishes **one** uplink
media track whose packets carry pose always and audio optionally, gated by
a flags byte. A silent entity sends audio-absent packets; a listener-only
entity sends them forever. The client is the authority on whether it is
sending audio.

The old pose track was already redundant whenever audio flowed (source
packets carry pose); it existed only for the silent case. Merging makes
silence a degenerate media packet instead of a second track, which buys:

- **One seq space per entity** — no freshest-pose rule across two tracks
  with independent seqs at the silence transition.
- **The silence transition is a flag flip**, not a track stop/start — no
  subscription churn, no gap, no startup race.
- **The flags byte generalises**: pose-present and audio-present are
  independent bits, so pose-only (silence), audio-only (pose decimation)
  and both (normal) share one packet format.

**Load-bearing premise:** the uplink is **single-consumer** — only the
space subscribes to client-published tracks, and the space always wants
everything, so MoQ's track-granular filtering buys nothing on the uplink.
Pose-for-visualisation is served downlink by `presence`.

**Reopen trigger:** a real second subscriber to client uplink tracks —
relays, selective-forwarding topologies, or services (recording,
analytics, spatial clustering) subscribing directly to clients. Note the
door swings both ways here: neither splitting a merged track nor merging
split tracks is additive for deployed publishers, which is why this was
decided on architecture intent rather than deferred.

### 2. Namespace tuples: typed segments + direction

Full track identity is a MoQ *(namespace tuple, track name)* pair. The
namespace shapes are:

- `(space, "entity", entity-id, direction)` — per-entity media
- `(space, "client", client-id, direction)` — per-client state
- `(space, "presence")` — the one space-wide broadcast

with `direction` ∈ {`source`, `sink`}, always **entity/client-centric**:
`source` is what the entity/client emits, `sink` is what it receives.

Why each piece:

- **Space in the tuple.** The namespace root; ticket authorization is
  namespace-prefix-shaped, so everything authorizable hangs off it.
- **Typed segments** (`"entity"`, `"client"`). The id spaces stop needing
  to be disjoint — the tuple is self-describing, and class-level prefixes
  fall out free: `(space, "entity")` is all media, `(space, "client",
  c)` is one client's everything. Future first-class things (channels,
  subspaces) get an obvious home without disturbing deployed names.
- **Id-rooted, not kind-rooted.** `(space, "entity", e, "source")` keeps
  everything about an entity under one prefix — what ticket auth wants.
  Kind-rooted wildcards ("all sources") were rejected as unneeded: the
  server sees announces anyway and the uplink is single-consumer.
- **Direction gives client and space disjoint announce prefixes.** The
  same id is published *about* by two parties (client publishes an
  entity's media; space publishes its binaural). Without the direction
  segment both would announce the same prefix — ambiguous for any relay.
  `source`/`sink` was chosen over `in`/`out`, which flips meaning between
  server-centric and client-centric readings; source/sink is anchored to
  the entity and cannot flip.

### 3. State becomes per-client source/sink tracks

The client's state ops ride `(space, "client", c, "source")` / `state`;
its filtered state feed rides `(space, "client", c, "sink")` / `state`.

This is not just naming tidiness. The state design filters ops per client
(role rules, prefix subscription), so the delivered feed is
**personalised per subscriber** — a shared `state` track whose content
differs per subscriber fights the MoQ model (relays assume a track's
objects are subscriber-independent and cacheable). Per-client sink tracks
make the personalisation structural. It also gives client identity a home
in the naming scheme, which it previously lacked (entities had ids in
namespaces; clients did not).

### 4. Presence stays one shared track

`(space, "presence")`, published by the space, consumed by everyone — the
one track that is a clean, cacheable MoQ broadcast. It deliberately breaks
the typed-segment pattern rather than force a silly tuple like
`(space, "space", "presence")`.

**Deferred: differential (per-client) presence** as an interest-management
optimisation. Costs of the shared track: each client pays O(N) inbound and
the space O(N²) total egress in entity count. **Reopen trigger:** large-N
spaces where presence bandwidth is a measurable fraction of audio.
Recorded complexity for the reopened design: when one client publishes
many entities, whose interest set governs what that client receives?

### 5. Track names are format names

| Tuple | Track name | Publisher | Type |
|---|---|---|---|
| `(space, "entity", e, "source")` | `mono-object` | client | datagram |
| `(space, "entity", e, "sink")` | `binaural`, `ambi2`, `ambi3` | space | datagram |
| `(space, "client", c, "source")` | `state` | client | stream |
| `(space, "client", c, "sink")` | `state` | space | stream |
| `(space, "presence")` | `presence` | space | datagram |

**Subscription is the format negotiation.** A client subscribes to the
format it wants; there is no catalog and no in-band negotiation. A future
format (a different order, a different frame size if the 5ms review
reopens) is a new track name — additive, breaks nothing.

`mono-object` names the uplink format: a mono, object-based source with
pose. Chosen over bare `mono` (undersells the object/pose semantics to a
reader skimming the table; "object-based" is the recognised vocabulary of
the adjacent specs this protocol positions against) and over
`object-based` (names the whole family, not the format — a later stereo
object format could not be distinguished; `mono-object`/`stereo-object`
can). Price accepted: "object" is overloaded against MoQ's own delivery
unit; the compound softens it.

## Rejected alternatives (recorded, with prices)

- **Catalog-in-state** — media-track discovery via `lasa.` keys. Rejected
  for layering inversion: the core media plane would depend on the profile
  vocabulary, undoing the core/profile split. Media tracks must work
  without dipping into state.
- **A catalog track** (the usual MoQ streaming-format move) — machinery
  without a consumer here; format-in-name does the job with zero surface.
- **`frame` as the merged track name** — collides with
  `lasa.entity.{id}.frame` (coordinate reference frame, world/head) inside
  the same spec, plus Opus-frame/QUIC-frame overload.
- **`stream`** — actively misleading: the track is datagrams, and MoQ
  uses "stream" for the other delivery mode.
- **`feed` / `signal` / `capture`** — viable friendly names, dissolved by
  the format-naming decision rather than beaten.
- **`in`/`out` direction segment** — perspective-dependent; flips meaning
  between server-centric and client-centric reading.

## Knock-ons

- `lasa2.md`'s track tables (client-published and space-published) are
  **stale**: no pose track, tuples not path strings, per-client state,
  new track names. To be rewritten together with the packet-structure
  results rather than twice.
- The packet format for `mono-object` needs the flags byte (audio-present,
  pose-present, redundancy) — taken up in the packet-structure phase.
- Consistency with `state-reconnect-and-gc.md` checked 2026-07-24: the
  per-client state split is **compatible; no rule changes**. The design
  already assumes one subscription per client ("a single track whose
  filter is a set of key prefixes"), per-client filtering, per-client
  MoQ subscription termination for role changes, and stream (not
  datagram) transport for the frontier promise — all satisfied by the
  sink track. The write-echo works unchanged: the write goes out on the
  client's source track, the echo returns on its sink subscription,
  which was always the substance ("echoed back on its own state
  subscription"). One wording sharpening for the lasa2 rewrite: "a
  client holds a single state subscription" should say the *sink* is
  the subscription — the client's op-publishing source track sits
  outside the cursor/frontier machinery entirely (no cursor, no
  catch-up; ops are fire-and-observe-echo).
