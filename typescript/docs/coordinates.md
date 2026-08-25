# Coordinate systems

There are many 3D coordinate systems. They differ because of their
varied etymologies in mathematics, navigation, computer graphics and
elsewhere. LASA uses the convention most common in ambisonics, which
Michael Gerzon and others developed at Oxford in the early 1970s. The
convention takes its inheritance from navigation, and is very similar
to the conventions used in avionics: the x axis is the direction of
travel. The Y-up convention of most graphics systems descends from
IRIS GL, which post-dates ambisonics. The converters in this SDK
bridge the two lineages.

## The LASA pose

A LASA pose holds a position and a rotation in one flat structure,
`{x, y, z, yaw, pitch, roll}` (`lasa-core.md` §5.1).

Positions are metres in a right-handed, three-dimensional cartesian
space with these axes:

- **x** points forwards.
- **y** points to the left.
- **z** points upwards.

Positions are unbounded. One unit is one metre, and there is no
normalisation. (The old Panaudia system scaled positions into a unit
cube. LASA does not.)

Rotations are radians:

- **yaw** is the direction in the horizontal plane, measured
  anticlockwise from the positive x axis.
- **pitch** is the elevation above or below the horizon, positive
  upwards.
- **roll** is the tilt to one side or the other, measured
  anticlockwise around the line of sight as seen looking forwards.

This looks something like this:

<img src="images/coordinates_ambisonic.png" alt="Ambisonic coordinate system" width="400">

The system can be summarised as:

- Right-handed cartesian.
- z up and x forward.
- Positions in metres, unbounded.
- Angles in radians.
- Intrinsic Tait-Bryan rotations in the order yaw, pitch, roll.

On the wire each angle is an int16 scaled to ±π, giving a resolution
of π/32767. The wire encoder clamps out-of-range angles rather than
wrapping them, so angles handed to the SDK should already sit in
(−π, π]. The converters below take care of this where a framework
convention makes overshoot likely.

A detailed explanation of Euler and Tait-Bryan conventions, and of
the difference between intrinsic and extrinsic rotations, is
[here](https://en.wikipedia.org/wiki/Euler_angles).

## Graphics coordinate systems

Another common coordinate system is the one used in computer
graphics. Probably originating in IRIS GL at Silicon Graphics in the
early 1980s, this system placed the two principal axes, x and y, on
an upright plane to match the computer screen rather than on the
ground. This standard is now the default for 3D on the web, built on
WebGL and OpenGL.

OpenGL itself uses extrinsic rotations, but most JavaScript libraries
such as Three.js wrap these in more intuitive intrinsic rotations,
resulting in a coordinate system that looks like this:

<img src="images/coordinates_threejs.png" alt="Web 3D coordinate system" width="400">

- Right-handed cartesian, the same as ambisonics.
- y is up instead of z, and −z is forward.
- Angles in radians.
- Intrinsic Tait-Bryan rotations, by default in the order pitch,
  yaw, roll (a Three.js `Euler` calls this order `'XYZ'`, and can
  carry a different order per object).

## Converters in this SDK

The SDK includes conversion functions for these frameworks:

| Framework | Handedness | Up | Forward | Euler composition | Units |
|---|---|---|---|---|---|
| Three.js / React Three Fiber | RH | +Y | −Z | intrinsic XYZ (or the `Euler`'s own order) | radians |
| Babylon.js | LH | +Y | +Z | intrinsic YXZ | radians |
| A-Frame | RH | +Y | −Z | intrinsic YXZ | degrees |
| PlayCanvas | RH | +Y | −Z | intrinsic ZYX (extrinsic XYZ) | degrees |
| Unity | LH | +Y | +Z | intrinsic YXZ | degrees |
| Unreal Engine | LH | +Z | +X | FRotator (yaw, pitch, roll) | degrees |
| PixiJS | 2D, y down | | +x is screen right | scalar rotation | radians |

Two of these rows differ from what the engines' own documentation
labels suggest, because the labels describe application order rather
than the intrinsic labelling used here. Unity's `Quaternion.Euler`
documents itself as applying z, then x, then y, which is intrinsic
YXZ. PlayCanvas's `setFromEulerAngles` composes extrinsic X, then Y,
then Z about fixed axes, which is intrinsic ZYX.

Each `<framework>ToLasa` function takes `(position, rotation)` in the
framework's native shapes and returns a LASA `Pose`. Each
`lasaTo<Framework>` function takes a `Pose` and returns the
framework's native shapes.

```ts
import { threejsToLasa, lasaToThreejs } from '@panaudia/lasa-client';

// A THREE.Euler carries its own order; pass it through.
const pose = threejsToLasa(
  camera.position,
  camera.rotation,
  camera.rotation.order,
);
```

Some things to note:

- Positions pass through with axis mapping only. There is no scale
  normalisation, so one framework unit becomes one metre. Unreal
  conventionally works in centimetres and PixiJS in pixels, so scale
  before or after conversion as required.
- A-Frame rotations are conventionally authored 0 to 360 degrees,
  and an accumulating PixiJS `sprite.rotation` grows without bound.
  Both converters wrap angles into (−π, π] for the wire.
- The PixiJS mapping takes rotation zero to face screen-up. Sprite
  art that faces screen-right at rotation zero carries a constant
  quarter-turn, so add π/2 to the sprite rotation before converting.

## A spatial mnemonic for ambisonic coordinates

Ambisonic coordinates, WebGL coordinates and geospatial coordinates
are all right-handed cartesian coordinate systems. This is a spatial
mnemonic to help quickly remember their relationships to each other.

Imagine you are sitting looking East with your phone flat on the
floor in front of you, with its screen facing up and the top of the
phone pointing to your left.

Now the x, y and z axes for your head's ambisonic sound field, your
geospatial pose and your phone's WebGL renderer are all aligned.

## Further reading

- [Ambisonics](https://en.wikipedia.org/wiki/Ambisonics)
- [Euler angles](https://en.wikipedia.org/wiki/Euler_angles)
- [OpenGL coordinates](https://learnopengl.com/Getting-started/Coordinate-Systems)
