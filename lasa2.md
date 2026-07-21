

# LASA over Media over QUIC

LASA is written in the context of both OpenMedia's allience's Eclipsa/IETF and Dolby/Freunhoffer MPEG-I. 
It is different from both in that it has a narrower scope of live spatial audio. 

It supports three audio formats, one input and two output:

- Multi-track mono object-based input
- 2nd/3rd order Ambisonic output
- Binaural stereo output

There are no speaker layout formats other than binaural.

LASA over Media over QUIC is quite opinionated about codec, it only supports Opus @ 48kHz in 5ms frames.

It also provides game engine type semantics for synchronisation of distributed data.

This design is not an agnostic packet format only, but has opted to commit to the MoQ protocol, 
which allows it to hand off some identity work to the protocol layer.


## Client published tracks


| Name   | Type     | Namespace          | Payload                        |
|--------|----------|--------------------|--------------------------------|
| source | datagram | source/<entity-id> | seq, pose, 5ms mono opus, fec? |
| pose   | datagram | pose/<entity-id>   | seq, pose                      |
| state  | stream   | state              | state change ops               |


## Source Track

A connected client can send one or more `source` tracks.

Each packet contains a sequence id, the pose (x, y, z, yaw, pitch, roll) 
and 5ms of opus encoded audio for a particular entity.

Optionally, it may also include a duplicate of the previous packet's audio.

## Pose Track

A connected client can send one or more `pose` tracks.

Each packet contains a sequence id and an entities pose. This is like `source` but without the audio.


## Space published tracks


| Name     | Type     | Namespace            | Payload                               |
|----------|----------|----------------------|---------------------------------------|
| binaural | datagram | binaural/<entity-id> | ts, seq, 5ms stereo opus, fec?        |
| ambi2    | datagram | ambi2/<entity-id>    | ts, seq, 5ms multi-channel opus, fec? |
| ambi3    | datagram | ambi3/<entity-id>    | ts, seq, 5ms multi-channel opus, fec? |
| presence | datagram | presence             | entity-id, pose, loudness             |
| state    | stream   | state                | state change ops                      |



## Binaural Track

This contains an Opus encoded stereo binaural rendering of the audio heard in the virtual space by a particular entity.

It also carries a timestamp of the space's time in utc and a space sequence id.

It may optionally also include a duplicate of the previous packet's audio

## Ambisonic Track

This contains an interleaved, 16-channel, Opus encoded, ANC/SN3D, 2nd/3rd order, ambisonic rendering of the audio heard in the 
virtual space by a particular entity.

It also carries a timestamp of the space's time in utc and a space sequence id.

It may optionally also include a duplicate of the previous packet's audio

## Presence Track

The `presence` track carries small binary packets containing high frequency data about all entities in a Space:

- pose
- loudness


## State

The `state` tracks carry streams of operations for a simple distributed key/value data store.

There are just two operations `set` and `tomb` (short for tombstone). `set` sets the value for a given key in the store 
and `tomb` removes the value from the store.

Every operation is given a sequence id by the store. These are monotonic incrementing ints.

Operations come in collections called `groups`. A `group` doesn't provide consistency guarentees;
it's a hint that the operations should all be applied together.

Clients must keep track of the sequence id for each value. Operations will not necessarily be received in order, and if 
it receives an operation with an id older, or equal to, the current id, it should ignore it.

Keys used in the operations are strings that must be sequences of lowercase alphanumeric latin characters or front slashes.

As keys are like paths, they can be used to construct hierarchically structured data objects, 
but to this protocol they are just a flat key/values. This means that you cannot remove leaf values by removing a parent, 
and that a leaf can be created with the same path and anothers parent.

Values are binary data. It's up to the applications to determine/know how to interpret the data. 

Certain keys are reserved by LASA and will always have specific data types.

A client holds a single state subscription, declared with a set of key prefixes that is fixed for
the life of the subscription. Prefixes match as plain string prefixes — `foo` matches `foobar` as
well as `foo/bar`; keys are strings and the path structure is convention. The subscription's scope
is the union of its prefixes (a key matching more than one prefix is delivered once); the set
containing the empty prefix subscribes to everything. Changing the set means terminating the
subscription and re-subscribing.

Reconnect, catch-up, cursors and tombstone GC are specified in `plan/state-reconnect-and-gc.md`.

Sending and receiving operations are filtered by sets of role-based rules on the server.

A client learns that a write was accepted only by seeing the operation echoed back, with its
server-assigned sequence id, on its own state subscription. There is no other acknowledgement: an
operation rejected by the role rules is dropped silently, and a client whose role allows writing
keys it cannot read will never see confirmation of those writes.






    

    

