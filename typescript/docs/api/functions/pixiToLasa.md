[@panaudia/lasa-client](../index.md) / pixiToLasa

# Function: pixiToLasa()

```ts
function pixiToLasa(position, rotation): Pose;
```

Defined in: [src/lasa/coordinates.ts:746](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L746)

Convert a PixiJS position and rotation to a LASA pose.

Rotation 0 faces screen-up; see the section note for sprites whose
art faces screen-right. Accumulated rotations wrap into (-π, π].

## Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `position` | [`Vec2`](../interfaces/Vec2.md) | PixiJS position {x, y} (screen coordinates) |
| `rotation` | `number` | PixiJS rotation in radians (clockwise positive) |

## Returns

[`Pose`](../interfaces/Pose.md)
