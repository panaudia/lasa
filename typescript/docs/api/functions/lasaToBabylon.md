[@panaudia/lasa-client](../index.md) / lasaToBabylon

# Function: lasaToBabylon()

```ts
function lasaToBabylon(pose): Vec3Pose;
```

Defined in: src/lasa/coordinates.ts:510

Convert a LASA pose to a Babylon.js position and rotation.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

## Returns

[`Vec3Pose`](../interfaces/Vec3Pose.md)

Babylon.js position and Euler rotation {x, y, z} in radians, YXZ order
