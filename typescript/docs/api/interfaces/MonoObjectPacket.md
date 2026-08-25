[@panaudia/lasa-client](../index.md) / MonoObjectPacket

# Interface: MonoObjectPacket

Defined in: [src/lasa/wire.ts:93](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L93)

One mono-object datagram: any of a pose, an audio frame, and a redundancy record.

## Properties

| Property | Type | Defined in |
| ------ | ------ | ------ |
| <a id="audio"></a> `audio?` | `Uint8Array`\<`ArrayBufferLike`\> | [src/lasa/wire.ts:95](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L95) |
| <a id="pose"></a> `pose?` | [`Pose`](Pose.md) | [src/lasa/wire.ts:94](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L94) |
| <a id="redundancy"></a> `redundancy?` | [`Redundancy`](Redundancy.md) | [src/lasa/wire.ts:96](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L96) |
