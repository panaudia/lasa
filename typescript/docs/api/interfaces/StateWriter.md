[@panaudia/lasa-client](../index.md) / StateWriter

# Interface: StateWriter

Defined in: [src/lasa/base.ts:176](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L176)

Anything that can publish upstream state ops — a [LasaClient](../classes/LasaClient.md), or a test double.

## Methods

### writeState()

```ts
writeState(...msgs): Promise<void>;
```

Defined in: [src/lasa/base.ts:177](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L177)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| ...`msgs` | [`UpstreamStateMessage`](../type-aliases/UpstreamStateMessage.md)[] |

#### Returns

`Promise`\<`void`\>
