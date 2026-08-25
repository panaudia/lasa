[@panaudia/lasa-client](../index.md) / encodeValue

# Function: encodeValue()

```ts
function encodeValue(v): Uint8Array;
```

Defined in: src/lasa/base.ts:44

Encodes a §1 value per JSON conventions, byte-for-byte as the Go
profile writes them: `true`/`false`, a JSON string, or a shortest
round-trip number.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `v` | [`BaseValue`](../type-aliases/BaseValue.md) |

## Returns

`Uint8Array`
