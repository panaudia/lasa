[@panaudia/lasa-client](../index.md) / Redundancy

# Interface: Redundancy

Defined in: [src/lasa/wire.ts:86](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L86)

A repeat of an earlier packet's audio, for loss repair (`lasa-core.md` §5.1).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="audio"></a> `audio` | `Uint8Array` | - | [src/lasa/wire.ts:89](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L89) |
| <a id="offset"></a> `offset` | `number` | 1–7 packets back; the referenced packet MUST be audio-bearing. | [src/lasa/wire.ts:88](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L88) |
