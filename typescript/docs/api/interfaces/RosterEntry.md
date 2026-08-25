[@panaudia/lasa-client](../index.md) / RosterEntry

# Interface: RosterEntry

Defined in: [src/lasa/roster.ts:13](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L13)

One live entity as presence last reported it.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="headframe"></a> `headFrame` | `boolean` | True when the entity is head-framed (`lasa-core.md` §3). | [src/lasa/roster.ts:17](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L17) |
| <a id="id"></a> `id` | `string` | Entity id. | [src/lasa/roster.ts:15](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L15) |
| <a id="loudness"></a> `loudness` | `number` | 8-bit loudness; convert with `loudnessToDBFS`. | [src/lasa/roster.ts:21](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L21) |
| <a id="pose"></a> `pose` | [`Pose`](Pose.md) | Last reported pose, LASA coordinates. | [src/lasa/roster.ts:19](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L19) |
