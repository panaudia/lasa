[@panaudia/lasa-client](../index.md) / loudnessToDBFS

# Function: loudnessToDBFS()

```ts
function loudnessToDBFS(l): number;
```

Defined in: [src/lasa/wire.ts:233](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L233)

Converts an 8-bit presence loudness to dBFS (`-Infinity` for `LOUDNESS_SILENT`).

## Parameters

| Parameter | Type |
| ------ | ------ |
| `l` | `number` |

## Returns

`number`
