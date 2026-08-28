[@panaudia/lasa-client](../index.md) / baseKeys

# Variable: baseKeys

```ts
const baseKeys: object;
```

Defined in: [src/lasa/base.ts:108](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L108)

Key builders for the base-profile vocabulary (`lasa-base-profile.md`
§6). Each validates its identifier arguments and throws
[MalformedError](../classes/MalformedError.md) on a violation, so a bad id fails at the call
rather than as a silently-dropped write.

## Type Declaration

| Name | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="property-attr"></a> `attr()` | (`id`, `path`) => `string` | - | [src/lasa/base.ts:114](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L114) |
| <a id="property-channelattenuation"></a> `channelAttenuation()` | (`ch`) => `string` | - | [src/lasa/base.ts:133](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L133) |
| <a id="property-channelgain"></a> `channelGain()` | (`ch`) => `string` | - | [src/lasa/base.ts:132](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L132) |
| <a id="property-channelmuted"></a> `channelMuted()` | (`ch`) => `string` | - | [src/lasa/base.ts:131](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L131) |
| <a id="property-clientblocked"></a> `clientBlocked()` | (`id`) => `string` | - | [src/lasa/base.ts:129](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L129) |
| <a id="property-dof"></a> `dof()` | (`id`) => `string` | - | [src/lasa/base.ts:124](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L124) |
| <a id="property-entity"></a> `entity()` | (`id`, `suffix`) => `string` | `lasa.entity.{id}.{suffix}`. The suffix is not validated; prefer a specific builder. | [src/lasa/base.ts:112](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L112) |
| <a id="property-entitymarker"></a> `entityMarker()` | (`id`) => `string` | `lasa.entity.{id}` — the existence marker, valued with the owning client id. | [src/lasa/base.ts:110](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L110) |
| <a id="property-frame"></a> `frame()` | (`id`) => `string` | - | [src/lasa/base.ts:123](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L123) |
| <a id="property-heardin"></a> `heardIn()` | (`id`, `channel`) => `string` | - | [src/lasa/base.ts:116](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L116) |
| <a id="property-hears"></a> `hears()` | (`id`, `channel`) => `string` | - | [src/lasa/base.ts:117](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L117) |
| <a id="property-name"></a> `name()` | (`id`) => `string` | - | [src/lasa/base.ts:113](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L113) |
| <a id="property-personalmute"></a> `personalMute()` | (`owner`, `other`) => `string` | - | [src/lasa/base.ts:125](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L125) |
| <a id="property-personalsolo"></a> `personalSolo()` | (`owner`, `other`) => `string` | - | [src/lasa/base.ts:126](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L126) |
| <a id="property-priority"></a> `priority()` | (`id`) => `string` | - | [src/lasa/base.ts:118](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L118) |
| <a id="property-renderattenuation"></a> `renderAttenuation()` | (`id`) => `string` | - | [src/lasa/base.ts:120](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L120) |
| <a id="property-renderdirectivity"></a> `renderDirectivity()` | (`id`) => `string` | - | [src/lasa/base.ts:122](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L122) |
| <a id="property-rendergain"></a> `renderGain()` | (`id`) => `string` | - | [src/lasa/base.ts:119](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L119) |
| <a id="property-rendersize"></a> `renderSize()` | (`id`) => `string` | - | [src/lasa/base.ts:121](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L121) |
| <a id="property-roleblocked"></a> `roleBlocked()` | (`role`) => `string` | - | [src/lasa/base.ts:130](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L130) |
| <a id="property-signedattr"></a> `signedAttr()` | (`id`, `path`) => `string` | - | [src/lasa/base.ts:115](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L115) |
| <a id="property-spaceentitymuted"></a> `spaceEntityMuted()` | (`id`) => `string` | - | [src/lasa/base.ts:128](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L128) |
