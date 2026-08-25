[@panaudia/lasa-client](../index.md) / PresenceDelta

# Interface: PresenceDelta

Defined in: [src/lasa/wire.ts:258](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L258)

A presence delta: changed records for generation `gen`.

## Properties

| Property | Type | Defined in |
| ------ | ------ | ------ |
| <a id="gen"></a> `gen` | `number` | [src/lasa/wire.ts:260](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L260) |
| <a id="kind"></a> `kind` | `"delta"` | [src/lasa/wire.ts:259](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L259) |
| <a id="records"></a> `records` | [`DeltaRecord`](DeltaRecord.md)[] | [src/lasa/wire.ts:261](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L261) |
