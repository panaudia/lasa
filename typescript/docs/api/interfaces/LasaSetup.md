[@panaudia/lasa-client](../index.md) / LasaSetup

# Interface: LasaSetup

Defined in: [src/lasa/setup.ts:54](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L54)

A setup instantiating a ticket-defined entity by id.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="attrs"></a> `attrs?` | `Record`\<`string`, `unknown`\> | Attributes merged over the ticket's definition. | [src/lasa/setup.ts:58](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L58) |
| <a id="id"></a> `id` | `string` | Id of an entity the ticket defines. | [src/lasa/setup.ts:56](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L56) |
| <a id="initial-pose"></a> `initial-pose?` | `object` | - | [src/lasa/setup.ts:59](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L59) |
| `initial-pose.attitude?` | `object` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.attitude.pitch` | `number` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.attitude.roll` | `number` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.attitude.yaw` | `number` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.position?` | `object` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| `initial-pose.position.x` | `number` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| `initial-pose.position.y` | `number` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| `initial-pose.position.z` | `number` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
