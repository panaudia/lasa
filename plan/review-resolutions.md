# Final pre-rewrite review — resolutions

*2026-07-27. A consistency review (one pass with full session context,
one independent adversarial pass) ran over the whole plan corpus
before the lasa2 rewrite. Findings and their decided resolutions.
Where a resolution patches an earlier doc, this doc wins; the rewrite
consumes it last. Items the review confirmed clean: audio layouts vs
frame-size consequences, group-id scoping vs redundancy arithmetic,
presence generations, delta/snapshot/frontier/watermark rules, track
table vs subscribe-auth matrix, core/profile layering discipline.*

## 1. Identifier grammar (was: ids collide with the key grammar)

Entity ids, client ids, space ids, channel names, role names and
attr names share one grammar: **`[a-z0-9-]`, 1–128 bytes** — lowercase
alphanumeric plus hyphen, no dots (a dot in an embedded id would make
keys unparseable and let one id sit inside another's prefix). The
**key charset gains `-`** accordingly: keys are `[a-z0-9.-]`, and with
ids bounded, **keys are capped at 512 bytes** (formalising the bound
the 64 KiB value-cap reasoning assumed). UUIDs (lowercase) now fit.
Presence's `id-len u8` is comfortably consistent (128 < 255). Schemas
carry matching `pattern`/`maxLength`.

## 2. Role-scope matching (was: the existence marker is invisible)

Under plain string-prefix matching, audience's dot-terminated read
scope `lasa.entity.{e}.` cannot match the marker key
`lasa.entity.{e}`; dropping the dot leaks sibling ids. **Resolution:
role rules (only) match *exact key or dot-extended prefix*** — a scope
`p` matches key `k` iff `k == p'` or `k` starts with `p' + "."`
(where `p'` is `p` without any trailing dot). **Subscription
filtering stays plain string-prefix as pinned** in
`state-reconnect-and-gc.md` — only the role layer is segment-aware.

## 3. Admin is unblockable (was: privilege inversion / permanent lockout)

`lasa.space.role.blocked.admin` is an invalid write (dropped like any
other invalid value), and `lasa.space.client.blocked.*` is ignored at
admission and never terminates a connection whose ticket bears
`admin`. This guarantees in-protocol recovery from any block state:
an admin can always connect and clear. Finer hierarchy rules (can
moderator block producer?) are deliberately not added now.

## 4. Ticket `permissions` (was: application-space keys had no policy)

The ticket gains a **`permissions`** claim, sibling of `roles`
(`panaudia.permissions`): three arrays of key **prefixes** — `set`,
`clear`, `read` — explicitly granting state access to this ticket.
(`read` governs what the filtered state subscription may deliver —
the "subscribe" side; named `read` to match `roles.md`'s vocabulary.)

- **Effective access = union of the roles' rules and the ticket's
  permissions.** Roles are compact aliases over the same prefix-rule
  machinery; permissions are the general form, and may target any
  prefix **including `lasa.*`** (a fine-grained grant without the full
  role).
- **Application space defaults**: ticketed connections have **no
  application-space access unless granted** via permissions;
  **unticketed spaces are unrestricted** over application space
  (consistent with the fragile mode).
- Value-level rules (allowed values, admin-unblockable) apply
  regardless of how the write access was obtained.
- Zero new core surface: the server enforces one merged prefix-rule
  set per connection, exactly as before.

## 5. Audience read scope widened

Audience reads all of `lasa.entity.` and `lasa.space.` (was: own
entities only, which contradicted attrs-are-forwarded, presence's
pointer to the existence marker, and you-are-muted UX). `roles.md`
updated.

## 6. Space identifier pinned

A space id follows the identifier grammar (§1) and is conventionally
a UUID, like entity and client ids. It is deployment-assigned, is the
namespace-tuple root, and **the server MUST verify the ticket's `aud`
equals its space id** — what stops a ticket minted for one space being
replayed into another.

## 7. Evict/sweep ordering

On same-`client_id` evict-and-reconnect, the old connection's
departure sweep MUST complete or be cancelled **before** the new
connection's admission writes are applied. (Otherwise a grace-period
sweep firing after re-admission deletes the fresh entity keys — its
clears carry later seqs, so dedup cannot save it.)

## 8. Channel routing rule and the default channel

Stated as a rule (was only inferable): **source S is audible to sink
K iff they share at least one channel that is not muted**; with
multiple shared channels, highest gain wins and lowest attenuation
wins (as the key docs already said). **Default channel:** an entity
whose config omits `sources` (resp. `sinks`) gets `["main"]`; an
explicit empty array means deliberately none. Bare configs just work.
*(Superseded detail: the spec as written — base profile §4, and the
implementation — defaults an explicit empty list to `["main"]` too;
"empty means none" did not survive into the normative text.)*

## 9. Mixed-mode entity collision: signed beats unsigned

*Deleted 2026-08-13 (`id-capabilities.md`): mixed mode no longer
exists — an unticketed space now rejects any presented ticket (mode
mismatch, `0x1A5A08`), so this collision can never occur and the rule
below is void.*

Spaces are expected to run ticketed or unticketed, but an unticketed
space still verifies tickets that are presented, so the mix can
occur: if a ticketed connection's entity id collides with a live
**unticketed** claimant, the unticketed incumbent is evicted (signed
claim beats unsigned squat). Between two ticketed clients the earlier
rule stands: reject the newcomer (issuer coordination error). Price
recorded: an unticketed client can lose its connection to a ticket
holder — inherent to the fragile mode.

## 10. dof/frame enforcement and presence

- The **space enforces `dof` at ingest**: dof 3 → position components
  of arriving poses are ignored; dof 0 → the whole pose is ignored and
  the entity's pose is its initial-pose. Presence reflects the
  enforced pose, never the sent one.
- **Pose handoff**: an entity's pose is its initial-pose until the
  first accepted packet pose supersedes it.
- ~~Head-frame entities are omitted from presence~~ *Superseded later
  the same day*: head-frame entities are **included**, marked by a
  per-krecord flags byte (bit 0 = head-frame, pose listener-head-
  relative). The flag rides keyframes only — `frame` is
  connection-immutable and membership changes bump the generation, so
  deltas stay 21 bytes. See `presence-format.md` §3b.

## 11. Space-key durability — known weakness, recorded

`lasa.space.*` moderation keys promise "forever" from a store whose
loss is an anticipated event (epochs exist for it): a restart or
restore can silently un-kick. Server QoI: space-scoped moderation
keys SHOULD be durably persisted across restarts. The weakness is
stated plainly in the spec rather than hidden.

## 12. Batch of small pins

- `ticket.schema.json` now **requires `panaudia`** (a ticket without
  it was schema-valid but unusable).
- A ticket with absent or empty `roles` ⇒ **`audience`** (same default
  as unticketed).
- Blocked-key timestamps are **unix seconds**.
- `priority: false` in a config ⇒ the key is simply never written.
- **Attrs flatten to dotted paths; objects only, no lists** — leaves
  are Boolean/String/Number; nested objects flatten
  (`attrs.a.b = v`); arrays are invalid everywhere in attrs (index
  keys import ordering semantics and stale-tail resize bugs; an app
  needing a list encodes a JSON array inside a single String value).
  Attr names follow the identifier grammar (§1).
- The state-subscribe KVP's internal layout (field order,
  absence-encoding for the fresh-client case) is specified in the
  rewrite's wire section, alongside the Connection Config parameter
  key.
- Cursor persistence rule: persist the **max** frontier seq per epoch
  (frontiers arrive on two independent subgroup streams and may
  interleave; "last received" could rewind — harmless but the max is
  the intended rule).
- The rewrite enumerates **all silent-drop causes in one place**:
  role/permission-rejected write, write outside any grant, oversize
  value.
- Mid-session role change: the only trigger is a global rule-set /
  server-config change (the terminate-and-resubscribe machinery in
  `state-reconnect-and-gc.md` is future-proofing; roles ride the
  ticket, presented once at CLIENT_SETUP).
- **Zero-entity connections are legal** (`entities: []` satisfies the
  schema): the observer/console pattern — may subscribe presence and
  its own state sink, plus whatever sinks its role allows (admin).
