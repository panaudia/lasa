[@panaudia/lasa-client](../index.md) / EntityView

# Interface: EntityView

Defined in: [src/lasa/base.ts:387](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L387)

One live entity as the store describes it.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="attrs"></a> `attrs` | `Record`\<`string`, [`BaseValue`](../type-aliases/BaseValue.md)\> | Runtime attributes, flat by dotted path. | [src/lasa/base.ts:393](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L393) |
| <a id="dof"></a> `dof` | `0` \| `3` \| `6` | - | [src/lasa/base.ts:404](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L404) |
| <a id="frame"></a> `frame` | `"world"` \| `"head"` | - | [src/lasa/base.ts:403](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L403) |
| <a id="heardin"></a> `heardIn` | `string`[] | Channels in which the source is heard. | [src/lasa/base.ts:397](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L397) |
| <a id="hears"></a> `hears` | `string`[] | Channels the sink listens to. | [src/lasa/base.ts:399](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L399) |
| <a id="id"></a> `id` | `string` | - | [src/lasa/base.ts:388](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L388) |
| <a id="mutes"></a> `mutes` | `string`[] | Entities this entity has personally muted / solo'd (readable by every role, §2: a muted user can see it). | [src/lasa/base.ts:406](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L406) |
| <a id="name"></a> `name` | `string` | - | [src/lasa/base.ts:391](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L391) |
| <a id="owner"></a> `owner` | `string` | The owning client id (the existence marker's value). | [src/lasa/base.ts:390](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L390) |
| <a id="priority"></a> `priority` | `boolean` | - | [src/lasa/base.ts:400](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L400) |
| <a id="render"></a> `render` | [`RenderParams`](RenderParams.md) | Set render parameters only — absent fields take the §6 defaults. | [src/lasa/base.ts:402](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L402) |
| <a id="signedattrs"></a> `signedAttrs` | `Record`\<`string`, [`BaseValue`](../type-aliases/BaseValue.md)\> | Attributes from inside the signed ticket, immutable. | [src/lasa/base.ts:395](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L395) |
| <a id="solos"></a> `solos` | `string`[] | - | [src/lasa/base.ts:407](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L407) |
