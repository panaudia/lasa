[@panaudia/lasa-client](../index.md) / PresenceKeyframe

# Interface: PresenceKeyframe

Defined in: [src/lasa/wire.ts:245](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L245)

A presence keyframe: the full record set from index `first`, for generation `gen`.

## Properties

| Property | Type | Defined in |
| ------ | ------ | ------ |
| <a id="first"></a> `first` | `number` | [src/lasa/wire.ts:248](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L248) |
| <a id="gen"></a> `gen` | `number` | [src/lasa/wire.ts:247](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L247) |
| <a id="kind"></a> `kind` | `"keyframe"` | [src/lasa/wire.ts:246](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L246) |
| <a id="records"></a> `records` | [`KeyframeRecord`](KeyframeRecord.md)[] | [src/lasa/wire.ts:249](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L249) |
