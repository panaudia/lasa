[@panaudia/lasa-client](../index.md) / aframeToLasa

# Function: aframeToLasa()

```ts
function aframeToLasa(position, rotation): Pose;
```

Defined in: [src/lasa/coordinates.ts:538](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L538)

Convert an A-Frame position and rotation to a LASA pose.

## Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `position` | [`Vec3`](../interfaces/Vec3.md) | A-Frame position {x, y, z} |
| `rotation` | [`Vec3`](../interfaces/Vec3.md) | A-Frame rotation {x, y, z} in DEGREES, YXZ order |

## Returns

[`Pose`](../interfaces/Pose.md)
