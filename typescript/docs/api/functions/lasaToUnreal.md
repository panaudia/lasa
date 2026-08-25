[@panaudia/lasa-client](../index.md) / lasaToUnreal

# Function: lasaToUnreal()

```ts
function lasaToUnreal(pose): UnrealPose;
```

Defined in: src/lasa/coordinates.ts:703

Convert a LASA pose to an Unreal Engine position and FRotator.

## Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

## Returns

[`UnrealPose`](../interfaces/UnrealPose.md)

Unreal position {x, y, z} and FRotator {pitch, yaw, roll} in degrees
