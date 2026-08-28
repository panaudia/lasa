[@panaudia/lasa-client](../index.md) / lasaToPixi

# Function: lasaToPixi()

```ts
function lasaToPixi(pose): PixiPose;
```

Defined in: [src/lasa/coordinates.ts:759](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L759)

Convert a LASA pose to a PixiJS position and rotation.
Height (z) is discarded. Only yaw maps to 2D rotation; pitch and roll are ignored.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

## Returns

[`PixiPose`](../interfaces/PixiPose.md)

PixiJS position {x, y} and rotation in radians
