[@panaudia/lasa-client](../index.md) / LasaEntity

# Interface: LasaEntity

Defined in: [src/lasa/setup.ts:15](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L15)

An entity definition (schemata/entity.schema.json subset).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="attrs"></a> `attrs?` | `Record`\<`string`, `unknown`\> | Free-form application attributes, published in state. | [src/lasa/setup.ts:27](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L27) |
| <a id="dof"></a> `dof?` | `0` \| `6` \| `3` | Degrees of freedom the entity's pose carries (default 6). | [src/lasa/setup.ts:31](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L31) |
| <a id="frame"></a> `frame?` | `"world"` \| `"head"` | Pose frame: `world` (default) or `head` (`lasa-core.md` §3). | [src/lasa/setup.ts:29](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L29) |
| <a id="heard-in"></a> `heard-in?` | `string`[] | Channels in which this entity's source is heard (§4; omitted → ["main"]). | [src/lasa/setup.ts:23](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L23) |
| <a id="hears"></a> `hears?` | `string`[] | Channels this entity's sink listens to (§4; omitted → ["main"]). | [src/lasa/setup.ts:25](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L25) |
| <a id="id"></a> `id` | `string` | Entity id, unique within the space. An identifier per `lasa-core.md` §2. | [src/lasa/setup.ts:17](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L17) |
| <a id="initial-pose"></a> `initial-pose?` | `object` | - | [src/lasa/setup.ts:45](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L45) |
| `initial-pose.attitude?` | `object` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.attitude.pitch` | `number` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.attitude.roll` | `number` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.attitude.yaw` | `number` | - | [src/lasa/setup.ts:47](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L47) |
| `initial-pose.position?` | `object` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| `initial-pose.position.x` | `number` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| `initial-pose.position.y` | `number` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| `initial-pose.position.z` | `number` | - | [src/lasa/setup.ts:46](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L46) |
| <a id="name"></a> `name` | `string` | Display name. | [src/lasa/setup.ts:19](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L19) |
| <a id="priority"></a> `priority?` | `boolean` | Priority entity (profile semantics, `lasa-base-profile.md`). | [src/lasa/setup.ts:21](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L21) |
| <a id="quality"></a> `quality?` | `0` \| `1` \| `2` | Latency-tolerance intent for this entity's audio path, 0–2 (lasa-core.md §4.2; path-scoped: the space applies it to its ingest buffer, the client applies it to its own playout buffer). 0 = interactive (default), 1 = resilient, 2 = playback/broadcast. | [src/lasa/setup.ts:38](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L38) |
| <a id="redundancy"></a> `redundancy?` | `number` | Maximum redundancy offset 0–7 for this entity's path (§4.2/§5.1); bounds the entity's uplink emission and requests sink-track redundancy from the space. 0 (default) = none. | [src/lasa/setup.ts:44](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L44) |
| <a id="render"></a> `render?` | `object` | Rendering parameters: gain multiplier, distance attenuation, source size, directivity. | [src/lasa/setup.ts:50](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L50) |
| `render.attenuation?` | `number` | - | [src/lasa/setup.ts:50](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L50) |
| `render.directivity?` | `number` | - | [src/lasa/setup.ts:50](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L50) |
| `render.gain?` | `number` | - | [src/lasa/setup.ts:50](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L50) |
| `render.size?` | `number` | - | [src/lasa/setup.ts:50](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L50) |
