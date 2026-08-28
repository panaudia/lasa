[@panaudia/lasa-client](../index.md) / playcanvasToLasa

# Function: playcanvasToLasa()

```ts
function playcanvasToLasa(position, rotation): Pose;
```

Defined in: [src/lasa/coordinates.ts:584](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L584)

Convert a PlayCanvas position and rotation to a LASA pose.

## Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `position` | [`Vec3`](../interfaces/Vec3.md) | PlayCanvas position {x, y, z} |
| `rotation` | [`Vec3`](../interfaces/Vec3.md) | PlayCanvas Euler rotation {x, y, z} in DEGREES (extrinsic XYZ) |

## Returns

[`Pose`](../interfaces/Pose.md)
