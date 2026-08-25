[@panaudia/lasa-client](../index.md) / unrealToLasa

# Function: unrealToLasa()

```ts
function unrealToLasa(position, rotation): Pose;
```

Defined in: src/lasa/coordinates.ts:676

Convert an Unreal Engine position and FRotator to a LASA pose.

## Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `position` | [`Vec3`](../interfaces/Vec3.md) | Unreal position {x, y, z} (X=forward, Y=right, Z=up) |
| `rotation` | [`FRotator`](../interfaces/FRotator.md) | Unreal FRotator {pitch, yaw, roll} in degrees |

## Returns

[`Pose`](../interfaces/Pose.md)
