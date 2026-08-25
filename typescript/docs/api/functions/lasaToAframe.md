[@panaudia/lasa-client](../index.md) / lasaToAframe

# Function: lasaToAframe()

```ts
function lasaToAframe(pose): Vec3Pose;
```

Defined in: src/lasa/coordinates.ts:559

Convert a LASA pose to an A-Frame position and rotation.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

## Returns

[`Vec3Pose`](../interfaces/Vec3Pose.md)

A-Frame position and rotation {x, y, z} in DEGREES, YXZ order
