[@panaudia/lasa-client](../index.md) / RenderParams

# Interface: RenderParams

Defined in: [src/lasa/base.ts:184](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L184)

Render parameters (profile §6 `render.*`). Undefined fields are left untouched.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="attenuation"></a> `attenuation?` | `number` | Distance-attenuation exponent 0.0–3.0; absent = 2.0 (inverse-square). | [src/lasa/base.ts:188](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L188) |
| <a id="directivity"></a> `directivity?` | `number` | 0.0 omnidirectional – 1.0 maximally directional. | [src/lasa/base.ts:192](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L192) |
| <a id="gain"></a> `gain?` | `number` | Amplitude multiplier 0.0–3.0; absent = 1.0. | [src/lasa/base.ts:186](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L186) |
| <a id="size"></a> `size?` | `number` | Source size in metres, ≥ 0; absent = point source. | [src/lasa/base.ts:190](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L190) |
