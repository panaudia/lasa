[@panaudia/lasa-client](../index.md) / babylonToLasa

# Function: babylonToLasa()

```ts
function babylonToLasa(position, rotation): Pose;
```

Defined in: src/lasa/coordinates.ts:494

Convert a Babylon.js position and rotation to a LASA pose.

## Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `position` | [`Vec3`](../interfaces/Vec3.md) | Babylon.js position {x, y, z} |
| `rotation` | [`Vec3`](../interfaces/Vec3.md) | Babylon.js Euler rotation {x, y, z} in radians, YXZ order |

## Returns

[`Pose`](../interfaces/Pose.md)
