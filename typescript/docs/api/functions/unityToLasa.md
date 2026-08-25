[@panaudia/lasa-client](../index.md) / unityToLasa

# Function: unityToLasa()

```ts
function unityToLasa(position, rotation): Pose;
```

Defined in: src/lasa/coordinates.ts:628

Convert a Unity position and rotation to a LASA pose.

## Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `position` | [`Vec3`](../interfaces/Vec3.md) | Unity position {x, y, z} |
| `rotation` | [`Vec3`](../interfaces/Vec3.md) | Unity Euler rotation {x, y, z} in DEGREES (Quaternion.Euler convention) |

## Returns

[`Pose`](../interfaces/Pose.md)
