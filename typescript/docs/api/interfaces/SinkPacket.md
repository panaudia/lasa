[@panaudia/lasa-client](../index.md) / SinkPacket

# Interface: SinkPacket

Defined in: [src/lasa/wire.ts:183](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L183)

One sink datagram: a rendered audio frame with optional redundancy.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="audio"></a> `audio` | `Uint8Array` | - | [src/lasa/wire.ts:186](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L186) |
| <a id="redundancy"></a> `redundancy?` | [`Redundancy`](Redundancy.md) | - | [src/lasa/wire.ts:187](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L187) |
| <a id="timestampmicros"></a> `timestampMicros?` | `bigint` | Unix-epoch microseconds of the frame's first sample (informational). | [src/lasa/wire.ts:185](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L185) |
