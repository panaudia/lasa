[@panaudia/lasa-client](../index.md) / threejsToLasa

# Function: threejsToLasa()

```ts
function threejsToLasa(
   position, 
   rotation, 
   order?): Pose;
```

Defined in: src/lasa/coordinates.ts:461

Convert a Three.js position and rotation to a LASA pose.

A THREE.Euler carries its own `.order`, which this function cannot
see; pass it explicitly when it is not the default. Camera rigs and
A-Frame commonly set 'YXZ'.

## Parameters

| Parameter | Type | Default value | Description |
| ------ | ------ | ------ | ------ |
| `position` | [`Vec3`](../interfaces/Vec3.md) | `undefined` | Three.js position {x, y, z} |
| `rotation` | [`Vec3`](../interfaces/Vec3.md) | `undefined` | Three.js Euler rotation {x, y, z} in radians |
| `order` | [`EulerOrder`](../type-aliases/EulerOrder.md) | `'XYZ'` | the Euler's order, default 'XYZ' (the THREE.Euler default) |

## Returns

[`Pose`](../interfaces/Pose.md)
