[@panaudia/lasa-client](../index.md) / EulerOrder

# Type Alias: EulerOrder

```ts
type EulerOrder = "XYZ" | "YXZ" | "ZXY" | "ZYX" | "YZX" | "XZY";
```

Defined in: src/lasa/coordinates.ts:94

Intrinsic Euler application order, in the Three.js labelling. All
six THREE.Euler orders are supported, so a THREE.Euler's own
`.order` can always be passed through.
