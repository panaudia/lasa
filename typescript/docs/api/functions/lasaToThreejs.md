[@panaudia/lasa-client](../index.md) / lasaToThreejs

# Function: lasaToThreejs()

```ts
function lasaToThreejs(pose, order?): Vec3Pose;
```

Defined in: [src/lasa/coordinates.ts:472](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/coordinates.ts#L472)

Convert a LASA pose to a Three.js position and rotation.

## Parameters

| Parameter | Type | Default value | Description |
| ------ | ------ | ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) | `undefined` | - |
| `order` | [`EulerOrder`](../type-aliases/EulerOrder.md) | `'XYZ'` | Euler order for the result, default 'XYZ' |

## Returns

[`Vec3Pose`](../interfaces/Vec3Pose.md)

Three.js position and Euler rotation {x, y, z} in radians
