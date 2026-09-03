# LASA Diagnostics

*Draft 1, 2026-09-03. Companion to `lasa-core.md` and
`lasa-base-profile.md`. This document defines two optional
diagnostic tracks, one published by a Space and one by a client,
each narrating its publisher's own decisions and state as
structured messages. Neither is part of the core. A conforming
Space or client need not offer them, and nothing in the core
depends on them. Where one is offered, this document is normative
for it, so that two implementations that offer it agree and a test
harness can read either. The relation to the core is the one qlog
has to QUIC: a diagnostic surface beside the protocol, never inside
it.*

## 1. Conventions

Both tracks carry one message per MoQ object, a UTF-8 JSON
document of the shape below, and share the reading rules, the
cadence and the cost discipline of this section.

### 1.1 Message shape

```
{ "kind":   string, a kind from the publisher's vocabulary, or "sample"
  "t":      number, unix microseconds on the publisher's clock
  ...       members specific to the kind or scope
  "detail": string, advisory, optional }
```

Readers MUST ignore a message whose `kind` they do not know, and MUST
ignore members they do not know. [D-1-1] Kinds, causes, scopes and
members are additive. A later revision may add them and never changes
the meaning of an existing one. The `detail` member is for humans. It
MAY be absent or truncated, and it MUST NOT be parsed for program logic,
exactly as core §4.6 treats reason phrases. [D-1-2] Everything a program
may act on is in a named member.

There are two families of message. An **event** narrates one input
and the decision its publisher took on it, and it carries the
input's identity. A **sample** carries the whole current value of
one named scope of the publisher's own state, emitted on subscribe
and again when the value changes. Events say what happened to an
input. Samples say how things stand.

Each track defines its own kinds and scopes (§2, §3). The
two-family split is common to both.

### 1.2 Cadence, cost and trust

A sample is emitted once for every scope that exists when a subscription
begins, and again when a scope's value changes, at most once per second
per scope, so a counter that moves every packet is reported about once a
second. A sample always carries the whole value of its scope, never a
difference from the previous sample. [D-1-3]

A debug track MUST NOT alter its publisher's protocol behaviour. A Space
that offers one behaves identically toward a client that subscribes and
one that does not, and a client that offers one behaves identically
whether or not the Space subscribes. [D-1-5] A publisher SHOULD bound
the track's buffering and drop debug messages rather than stall any
other track, reporting each run of lost messages as `debug.dropped` on
the next message it does deliver. A lagging subscriber costs itself
narration and costs the publisher nothing. [D-1-4]

> *Note (conformance): a debug message names a cause or turns an
> absence into a positive observation. It never satisfies an
> obligation of the core by itself. A Space that reports a write
> dropped and then applies it has failed the write rule, and a
> client that reports a packet discarded and then plays it has
> failed the discard rule, whatever either narrated.*

## 2. The Space's debug track

The **debug track** is a per-client stream of structured
diagnostics published by the space. It narrates what the space did
with the connection's own inputs and how the connection's own
machinery stands, so that a developer whose write vanished, whose
subscribe was refused, or whose audio sounds late can read the
cause on the protocol rather than in a server log. Core §6.2 makes
rejected writes silent by design, and this track is the diagnostic
complement to that silence.

Offering the track is a MAY. A conforming Space need not offer it. A
client MUST NOT depend on it, and no rule in this specification requires
it for correctness. [D-2-1] Where a Space offers the track, its name,
authorization, MoQ mapping, and the meaning of every kind, cause and
scope below are normative, so that two Spaces that offer it agree and a
test harness can read either.

### 2.1 Track and authorization

| Tuple | Track name | Publisher | Delivery |
|---|---|---|---|
| `(space, "client", c, "sink")` | `debug` | space | stream |

The track is subscribable by the connection with client id `c` only
(core §3.1). [D-2-2] The subscribe carries no LASA SUBSCRIBE parameter.
[D-2-3] There is no upstream half. The track is read-only narration and
carries no commands. [D-2-4]

**MoQ mapping.** Each subscription instance is a fresh MoQ group with
one subgroup, subgroup 0. Object-ids increment by one per object, and
each object is one message, a UTF-8 JSON document. [D-2-5] A
subscription joins at live and carries no history, beyond the opening
samples of §2.3. [D-2-6] Delivery is by stream rather than datagram
because the messages describe drops and losses, and losing the narration
of a loss would defeat the purpose.

The track narrates the subscribing connection's own inputs, entities and
subscription only. Nothing on it names another connection's keys or
entities beyond what the connection's own read grants already expose.
[D-2-12]

> *Note (connect-time rejections): a core §4.6 rejection happens before
> SERVER_SETUP, when no track exists. The reason phrase remains the
> only diagnostic there.*

### 2.2 Events

| Kind | Members | Meaning |
|---|---|---|
| `write.dropped` | `cause`, `key` | an upstream state message was dropped under core §6.2 |
| `write.unechoed` | `cause`, `key`, `seq` | an accepted op was applied but its echo was withheld (core §6.2) |
| `subscribe.rejected` | `track`, `cause` | a SUBSCRIBE from the connection was refused |
| `state.terminated` | `cause` | the space terminated the connection's state subscription (core §6.6) |
| `value.ignored` | `key`, `cause` | a value the connection wrote was accepted and delivered, and a profile consumer ignored it |
| `debug.dropped` | `count` | debug messages lost since the previous message on this track (§1.2) |

Each kind's `cause` takes one value from a fixed vocabulary. [D-2-7]

| Kind | `cause` values |
|---|---|
| `write.dropped` | `parse`, `key`, `value`, `group-cap`, `denied`, `no-grant`, `swept` |
| `write.unechoed` | `no-read`, `out-of-scope` |
| `subscribe.rejected` | `non-canonical`, `empty-set`, `unauthorized`, `no-such-track` |
| `state.terminated` | `visibility-change`, `lagged` |
| `value.ignored` | `out-of-range`, `wrong-type` |

The `write.dropped` causes are the core §6.2 silent-drop causes in
order. One event narrates one dropped upstream message. A group dropped
whole is one event, and `key` names the first op that failed. `key` is
present whenever the space knows it, and is absent when the message did
not parse. [D-2-8] The `write.unechoed` causes are the two
accepted-yet-unechoed shapes of core §6.2. `subscribe.rejected` names
the track by its track name, and `non-canonical` and `empty-set` are the
core §6.7 and core §6.3 subscribe-parameter refusals. `state.terminated`
with `visibility-change` is the mid-session termination of core §6.6,
and `lagged` means the client fell behind live delivery and the space
ended the subscription rather than stall the log. In both cases the
client re-subscribes and hits the gate. `value.ignored` is emitted by a
profile consumer on the writer's own track, and the meaning of its
causes is the profile's (base profile §2).

### 2.3 Samples

```
{ "kind": "sample", "t": number, "scope": string, "value": object }
```

| Scope | `value` members | Meaning |
|---|---|---|
| `session` | `mode`, `roles`, `entities` | the admission outcome. `mode` is `ticketed` or `unticketed`, `roles` is the effective role names, and `entities` holds one object per instantiated entity carrying at least `id` |
| `subscription.state` | `attached`, `gate`, `verdict`, `cursor` | the state subscription. `attached` is a boolean, `gate` is the core §6.6 rule that decided the most recent attach (1 to 6, or 0 for a fresh client), `verdict` is `delta` or `snapshot`, and `cursor` is the presented cursor as `{ "epoch": string, "seq": number }` when one was presented |
| `entity.{id}.ingest` | `discarded` | the entity's source ingest. `discarded` is an object of counts by discard cause since the track instance began |
| `entity.{id}.render` | `formats` | the sink formats currently rendering for the entity (core §3.2), an array of track names, possibly empty |

The scopes and members above are normative. [D-2-9] The `discarded`
causes are the core §5.1 receiver rules: `reserved-bits` for unknown
flag bits, `offset-without-flag` for a nonzero offset with bit 2 clear,
`zero-offset` for bit 2 set with a zero offset,
`offset-over-declaration` for an offset exceeding the entity's declared
maximum (core §4.2), and `malformed` for a packet that does not parse to
its layout. The `cursor` epoch is the eight opaque bytes of core §6.6 as
sixteen lowercase hexadecimal characters. [D-2-11]

**Cadence.** On subscribe the space emits one sample for every scope
that exists, `session` first, before any event. [D-2-10] Thereafter the
cadence is §1.2's.

A profile MAY add members to a scope's value, and the base profile
does (base profile §4, core §6). An implementation MAY add members of
its own. Readers ignore members they do not know, per §1.1.

> *Note: the reference implementation adds its jitter-buffer fill
> and depacketizer loss counters to the ingest scope, which is what
> the latency-debugging recipe in `plan/latency-baseline.md` reads.
> Those members are implementation-defined and carry no meaning
> under this specification.*

### 2.4 Base profile members

The base profile adds two members to the samples above, and a
Space that offers both the base profile and this track carries
them.

- The `session` sample's entity objects carry the resolved `heard-in`
  and `hears` channel lists (base profile §4), so a client can read the
  membership its config was materialised into. [D-2-13]
- The `entity.{id}.ingest` sample carries a `pose-ignored` object of
  counts by cause, `dof-3` and `dof-0`, for the poses the space ignored
  under the base profile's `dof` rule (base profile §6). [D-2-14]

## 3. The client's debug track

A client MAY publish a **client debug track** narrating what it holds:
its store, its roster, its receive-side discards and the closes it
received. It is the mirror of §2. It exists so that a client can be
observed from the Space side the way a Space is observed from the client
side, through MoQ, from a browser or from a phone on a real network
alike. No Space behaviour may depend on it, and no obligation on a
client in the core is satisfied by a message on it. [D-3-1]

### 3.1 Track and authorization

| Tuple | Track name | Publisher | Delivery |
|---|---|---|---|
| `(space, "client", c, "source")` | `debug` | client | stream |

The track is subscribable by the Space only, as every source track is
(core §3.1). [D-3-2] A client publishes nothing on it until the Space
subscribes, so an unsubscribed track costs the client nothing. [D-3-3]

**MoQ mapping.** The mapping is §2.1's, read from the publisher's side:
one MoQ group per subscription instance, subgroup 0, object-ids
incrementing by one, one message per object. A subscription receives the
opening samples of §1.2 and then joins at live. [D-3-4]

### 3.2 Samples

```
{ "kind": "sample", "t": number, "scope": string, "value": object }
```

| Scope | `value` members | Meaning |
|---|---|---|
| `session` | `client_id`, `entities`, `transport`, `connected` | what the client believes it is: its client id, the entity ids it instantiated, `quic` or `webtransport`, and whether it holds a session |
| `store` | `prefixes`, `count`, `digest`, `entries` | the state store under the subscribed prefixes. `count` is the number of live keys, `digest` is defined below, and `entries` is the list of `{ "key", "seq", "value" }` objects, present when the client chooses to include it |
| `subscription.state` | `cursor`, `verdict`, `terminations` | the state subscription as the client saw it: the cursor it last presented as `{ "epoch": string, "seq": number }` or absent, the `begin` kind it last received as `delta` or `snapshot`, and how many times the Space terminated the subscription |
| `roster` | `gen`, `entities` | the presence roster: the generation of the last keyframe and one object per known entity carrying `id`, `index` and `head` |
| `sink.{entity}.{format}` | `received`, `discarded` | one live sink subscription: packets received, and an object of counts by the core §5.1 discard cause since the subscription began |

The scopes and members above are normative. [D-3-5] The `digest` member
is the lowercase hexadecimal SHA-256 over the store's live entries
sorted bytewise by key, each encoded as `[key-len varint][key][seq u64
little-endian][value-len varint][value]`, so that a harness can compare
a large store without reading it. [D-3-6] The `discarded` cause names
are those of §2.3, and the `cursor` epoch is encoded as there. [D-3-7] A
profile MAY add members, and an implementation MAY add members of its
own, under the reading rule of §1.1.

### 3.3 Events

| Kind | Members | Meaning |
|---|---|---|
| `close.received` | `code`, `reason` | the Space closed the session, and the client saw this code and reason phrase (core §4.6) |
| `debug.dropped` | `count` | debug messages lost since the previous message on this track (§1.2) |

`close.received` is emitted on the next subscription instance, since the
session that carried the close is gone, and it is the first event of
that instance. [D-3-8] Its `reason` member is the phrase as received and
MAY be truncated. A client reports the code and never acts on the
phrase, which is the core's own rule (core §4.6). [D-3-9]

### 3.4 What it narrates

The track narrates the publishing client's own state only. A client
never reports another client's keys beyond those its own store holds
under its read grants, and never reports another client's entities
beyond what presence told it. [D-3-10]

## 4. Security considerations

- The Space's track narrates a connection's own inputs and entities
  to that connection only. It exposes nothing the connection's read
  grants do not, and it is read-only, so it adds no control surface.
- The client's track is readable by the Space only. A deployment
  that lets its own tooling subscribe it is showing that tooling
  what the client already sent or received, and nothing else. It
  MUST NOT be granted to other clients.
- Neither track carries commands. A diagnostic surface with a
  control verb would be a control surface, and this document
  defines none.
