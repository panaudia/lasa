[@panaudia/lasa-client](../index.md) / lasaToPlaycanvas

# Function: lasaToPlaycanvas()

```ts
function lasaToPlaycanvas(pose): Vec3Pose;
```

Defined in: [src/lasa/coordinates.ts:599](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L599)

Convert a LASA pose to a PlayCanvas position and rotation.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

## Returns

[`Vec3Pose`](../interfaces/Vec3Pose.md)

PlayCanvas position and Euler rotation {x, y, z} in DEGREES (extrinsic XYZ)
