[@panaudia/lasa-client](../index.md) / isLasaRejectionCode

# Function: isLasaRejectionCode()

```ts
function isLasaRejectionCode(code): boolean;
```

Defined in: [src/lasa/errors.ts:48](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L48)

True iff code is a LASA session-rejection code a server may issue —
the §4.6 range minus the retired 0x1A5A03, which MUST never appear
on the wire.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `code` | `number` |

## Returns

`boolean`
