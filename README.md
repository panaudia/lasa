# LASA — Live Atomic Spatial Audio

A protocol for live, object-based, spatial audio streaming.

### Features:

- Multiple clients, can contribute audio and listen simultaneously 
- Each clients can contribute multiple object-based sources
- Binaural and ambisonic sinks
- Audio and pose data in the same packet
- Real-time DB for state
- Built on Media over QUIC
- MIT licenced

### Design

Architectually, LASA is split between two types of data:

- Atomic: Fast, regular, stateless (audio and pose)
- Stateful: Uneven in regularity and size, extensible, cachable (everything else)

They are handled in quite different ways due to these characteristics.

**Media over QUIC**

LASA should really be called LASAoMoQ as we have taken the decision to commit to Media over QUIC as the transport. 
Doing so allows LASA to take advantage of several features inherent in the protocol:

- MoQ provides a single resilient connection with TLS that can carry both low-latency, multi-track audio via datagrams,
and multiple reliable streams of state data. 

- Agnosticism over codec allows us to use binary packets holding both compressed audio and pose data together.

- MoQ's own identifier structure overlaps to some degree with LASA's allowing us to push some of LASA's own 
structure up into the transport layer, removing redundency and increasing efficiency.

**Atomic Audio**

LASA is opinionated about audio format. Object-based audio at 48kHz is sent in Opus encoded 5ms frames. 
These are `atomic` in that each 5ms frame can also carry full `x, y, z, yaw, pitch, roll` pose data. 
Each audio frame can thus carry with it the time-varying data required by a renderer to spatialise it correctly. 
This removes issues of synchronisation and extra positional latency due to interpolation offsets.

Audio is sent in MoQ datagram tracks whose characteristics are lossy and low latency (a bit UDP like).

To add greater resilience, LASA allows for error correction with the addition of full bandwith redundant audio data.
The offset, and from that the size of loss this can mask, is adjustable at the cost of additional latency. 
ie we can mask a 20ms outage if you will accept 20ms of additional latency.

**Presence**

Like a game engine, LASA, optionally, provides a presence feed containing the pose of all other clients connected to a Space. 

**Distributed State**

All other state in LASA, apart from the audio and pose data, is managed through a distributed key-value store.
This is a real-time, pub-sub database with sequential consistancy.

State change operations are carried in a MoQ reliable stream track allowing the state store to provide good consistency guarantees.
Keys are utf-8 strings, values are binary data.

LASA has a `base profile` which uses the `lasa.` key prefix namspace to distribute necessay parameters and attributes through the system, 
but the state store can be extended carry any desired data.

LASA's `base profile` synchronises things like rendering chacteristics, such as gain or directivity, per user muting and permissions information, 
and display data about users.

## Spec

LASA is defined in two spec documents in this repo:

- [**`lasa-core.md`**](lasa-core.md)  Identifiers, tracks
  and namespaces, connection and auth, audio packet formats,
  presence, and the state machinery with its wire encoding.
- [**`lasa-base-profile.md`**](lasa-base-profile.md) — The base lasa state namespace covering roles,
  permissions, channels, moderation, and key lifetimes.

There are also JSON Schema documents in `./schemata` defining the data objects LASA uses:

- [connection](schemata/connection.schema.json)
- [entity](schemata/entity.schema.json)
- [ticket](schemata/ticket.schema.json)
- [pose](schemata/pose.schema.json)
- [setup](schemata/setup.schema.json)


## Go reference implementation

This repo also contains a Go reference implementation of the LASA protocol.

The sibling repo https://github.com/panaudia/panaudia-lasa contains a full, LASA media server, Panaudia LASA, also MIT licensed.

## Tickets

`lasa-ticket` (`cmd/lasa-ticket`, `go install github.com/panaudia/lasa/cmd/lasa-ticket@latest`) creates keys, mints and checks the JWT based tickets that LASA uses, on the same verifier the server runs. `tickets/` has the guide and sample issuer code in Go, Python and TypeScript.

## TypeScript Client

There is a TypeScript, WebTransport client in `typescript`