[@panaudia/lasa-client](../index.md) / lasaToUnity

# Function: lasaToUnity()

```ts
function lasaToUnity(pose): Vec3Pose;
```

Defined in: [src/lasa/coordinates.ts:643](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L643)

Convert a LASA pose to a Unity position and rotation.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

## Returns

[`Vec3Pose`](../interfaces/Vec3Pose.md)

Unity position and Euler rotation {x, y, z} in DEGREES (Quaternion.Euler convention)
