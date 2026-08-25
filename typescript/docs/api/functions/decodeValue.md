[@panaudia/lasa-client](../index.md) / decodeValue

# Function: decodeValue()

```ts
function decodeValue(bytes): BaseValue | undefined;
```

Defined in: src/lasa/base.ts:57

Decodes a §1 value. Returns undefined for absent bytes, and for bytes
that are not one of the three types — the profile says an invalid
value is stored and delivered but ignored by every consumer (§2), so
a reader treats it as absent.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `bytes` | `Uint8Array`\<`ArrayBufferLike`\> \| `undefined` |

## Returns

[`BaseValue`](../type-aliases/BaseValue.md) \| `undefined`
