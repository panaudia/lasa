/**
 * Framework pose conversion.
 *
 * Converts positions and rotations between common 3D framework
 * conventions and the LASA pose (lasa-core.md §5.1): metres and
 * radians, x forward, y left, z up, positive yaw anticlockwise about
 * z, positive pitch upward, positive roll anticlockwise about the
 * line of sight.
 *
 * LASA's axes are the ambisonic convention, fixed in the early 1970s
 * and inherited from navigation: x is the direction of travel. The
 * Y-up convention of most graphics systems descends from IRIS GL,
 * which post-dates ambisonics; these converters bridge the two
 * lineages.
 *
 * Supported frameworks (Euler orders are intrinsic, in the Three.js
 * labelling used throughout this file):
 *   Three.js:    RH, Y-up, -Z forward, XYZ Euler (per Euler.order), radians
 *   Babylon.js:  LH, Y-up, +Z forward, YXZ Euler, radians
 *   A-Frame:     RH, Y-up, -Z forward, YXZ Euler, degrees
 *   PlayCanvas:  RH, Y-up, -Z forward, ZYX Euler (extrinsic XYZ), degrees
 *   Unity:       LH, Y-up, +Z forward, YXZ Euler (docs: applied z, x, y), degrees
 *   Unreal:      LH, Z-up, +X forward, FRotator (pitch/yaw/roll), degrees
 *   PixiJS:      2D, Y-down, +X right, single rotation, radians
 *
 * Positions pass through with axis mapping only; there is no scale
 * normalisation, so one framework unit becomes one metre. Callers
 * working in other units (Unreal centimetres, PixiJS pixels) scale
 * before or after conversion.
 *
 * Rotations go through a quaternion intermediary to correctly handle
 * the differing Euler orders and left/right handedness (A-Frame and
 * PixiJS map directly). Cost per conversion is a handful of
 * transcendental calls and one small array allocation, well under a
 * microsecond, not a meaningful bottleneck at pose rates.
 *
 * The Euler math is ported from Three.js (MIT license).
 * Copyright 2010-2024 Three.js authors. https://threejs.org/
 */

import type { Pose } from './wire.js';

// ============================================================================
// Framework-facing types
// ============================================================================

/** 3D vector (used by Three.js, Babylon, A-Frame, PlayCanvas, Unity) */
export interface Vec3 {
  x: number;
  y: number;
  z: number;
}

/** 2D vector (used by PixiJS) */
export interface Vec2 {
  x: number;
  y: number;
}

/** Unreal Engine FRotator (pitch/yaw/roll in degrees) */
export interface FRotator {
  pitch: number;
  yaw: number;
  roll: number;
}

/** Three.js / Babylon / PlayCanvas / Unity / A-Frame pose (position + Euler rotation as Vec3) */
export interface Vec3Pose {
  position: Vec3;
  rotation: Vec3;
}

/** Unreal Engine pose (position + FRotator) */
export interface UnrealPose {
  position: Vec3;
  rotation: FRotator;
}

/** PixiJS pose (2D position + single rotation angle) */
export interface PixiPose {
  position: Vec2;
  rotation: number;
}

// ============================================================================
// Euler / Quaternion math
// ============================================================================

/**
 * Intrinsic Euler application order, in the Three.js labelling. All
 * six THREE.Euler orders are supported, so a THREE.Euler's own
 * `.order` can always be passed through.
 */
export type EulerOrder = 'XYZ' | 'YXZ' | 'ZXY' | 'ZYX' | 'YZX' | 'XZY';

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

interface Quat {
  x: number;
  y: number;
  z: number;
  w: number;
}

function radiansToDegrees(rad: number): number {
  return (rad / Math.PI) * 180;
}

function degreesToRadians(deg: number): number {
  return (deg / 180.0) * Math.PI;
}

/**
 * Convert Euler angles (radians) to quaternion.
 * Supports XYZ, YXZ, ZXY, ZYX intrinsic orders.
 * Formulas from Three.js Quaternion.setFromEuler().
 */
function eulerToQuaternion(
  ex: number,
  ey: number,
  ez: number,
  order: EulerOrder,
): Quat {
  const c1 = Math.cos(ex / 2);
  const c2 = Math.cos(ey / 2);
  const c3 = Math.cos(ez / 2);
  const s1 = Math.sin(ex / 2);
  const s2 = Math.sin(ey / 2);
  const s3 = Math.sin(ez / 2);

  switch (order) {
    case 'XYZ':
      return {
        x: s1 * c2 * c3 + c1 * s2 * s3,
        y: c1 * s2 * c3 - s1 * c2 * s3,
        z: c1 * c2 * s3 + s1 * s2 * c3,
        w: c1 * c2 * c3 - s1 * s2 * s3,
      };
    case 'YXZ':
      return {
        x: s1 * c2 * c3 + c1 * s2 * s3,
        y: c1 * s2 * c3 - s1 * c2 * s3,
        z: c1 * c2 * s3 - s1 * s2 * c3,
        w: c1 * c2 * c3 + s1 * s2 * s3,
      };
    case 'ZXY':
      return {
        x: s1 * c2 * c3 - c1 * s2 * s3,
        y: c1 * s2 * c3 + s1 * c2 * s3,
        z: c1 * c2 * s3 + s1 * s2 * c3,
        w: c1 * c2 * c3 - s1 * s2 * s3,
      };
    case 'ZYX':
      return {
        x: s1 * c2 * c3 - c1 * s2 * s3,
        y: c1 * s2 * c3 + s1 * c2 * s3,
        z: c1 * c2 * s3 - s1 * s2 * c3,
        w: c1 * c2 * c3 + s1 * s2 * s3,
      };
    case 'YZX':
      return {
        x: s1 * c2 * c3 + c1 * s2 * s3,
        y: c1 * s2 * c3 + s1 * c2 * s3,
        z: c1 * c2 * s3 - s1 * s2 * c3,
        w: c1 * c2 * c3 - s1 * s2 * s3,
      };
    case 'XZY':
      return {
        x: s1 * c2 * c3 - c1 * s2 * s3,
        y: c1 * s2 * c3 - s1 * c2 * s3,
        z: c1 * c2 * s3 + s1 * s2 * c3,
        w: c1 * c2 * c3 + s1 * s2 * s3,
      };
    default:
      // Guards plain-JS callers passing a typo; without it the switch
      // would fall through to undefined and fail obscurely downstream.
      throw new Error(`Unknown Euler order: ${order as string}`);
  }
}

/**
 * Convert quaternion to rotation matrix (column-major 4x4 elements).
 */
function quaternionToMatrix(q: Quat): number[] {
  const { x, y, z, w } = q;
  const x2 = x + x,
    y2 = y + y,
    z2 = z + z;
  const xx = x * x2,
    xy = x * y2,
    xz = x * z2;
  const yy = y * y2,
    yz = y * z2,
    zz = z * z2;
  const wx = w * x2,
    wy = w * y2,
    wz = w * z2;

  return [
    1 - (yy + zz), xy + wz, xz - wy, 0,
    xy - wz, 1 - (xx + zz), yz + wx, 0,
    xz + wy, yz - wx, 1 - (xx + yy), 0,
    0, 0, 0, 1,
  ];
}

/**
 * Extract Euler angles (radians) from rotation matrix in the given order.
 * Formulas from Three.js Euler.setFromRotationMatrix().
 */
function matrixToEuler(
  te: number[],
  order: EulerOrder,
): { x: number; y: number; z: number } {
  const m11 = te[0]!,
    m12 = te[4]!,
    m13 = te[8]!;
  const m21 = te[1]!,
    m22 = te[5]!,
    m23 = te[9]!;
  const m31 = te[2]!,
    m32 = te[6]!,
    m33 = te[10]!;

  switch (order) {
    case 'XYZ': {
      const sy = clamp(m13, -1, 1);
      const ey = Math.asin(sy);
      if (Math.abs(sy) < 0.9999999) {
        return { x: Math.atan2(-m23, m33), y: ey, z: Math.atan2(-m12, m11) };
      } else {
        return { x: Math.atan2(m32, m22), y: ey, z: 0 };
      }
    }
    case 'YXZ': {
      const sx = clamp(m23, -1, 1);
      const ex = Math.asin(-sx);
      if (Math.abs(sx) < 0.9999999) {
        return { x: ex, y: Math.atan2(m13, m33), z: Math.atan2(m21, m22) };
      } else {
        return { x: ex, y: Math.atan2(-m31, m11), z: 0 };
      }
    }
    case 'ZXY': {
      const sx = clamp(m32, -1, 1);
      const ex = Math.asin(sx);
      if (Math.abs(sx) < 0.9999999) {
        return { x: ex, y: Math.atan2(-m31, m33), z: Math.atan2(-m12, m22) };
      } else {
        return { x: ex, y: 0, z: Math.atan2(m21, m11) };
      }
    }
    case 'ZYX': {
      const sy = clamp(m31, -1, 1);
      const ey = Math.asin(-sy);
      if (Math.abs(sy) < 0.9999999) {
        return { x: Math.atan2(m32, m33), y: ey, z: Math.atan2(m21, m11) };
      } else {
        return { x: 0, y: ey, z: Math.atan2(-m12, m22) };
      }
    }
    case 'YZX': {
      const sz = clamp(m21, -1, 1);
      const ez = Math.asin(sz);
      if (Math.abs(sz) < 0.9999999) {
        return { x: Math.atan2(-m23, m22), y: Math.atan2(-m31, m11), z: ez };
      } else {
        return { x: 0, y: Math.atan2(m13, m33), z: ez };
      }
    }
    case 'XZY': {
      const sz = clamp(m12, -1, 1);
      const ez = Math.asin(-sz);
      if (Math.abs(sz) < 0.9999999) {
        return { x: Math.atan2(m32, m22), y: Math.atan2(m13, m11), z: ez };
      } else {
        return { x: Math.atan2(-m23, m33), y: 0, z: ez };
      }
    }
    default:
      throw new Error(`Unknown Euler order: ${order as string}`);
  }
}

/**
 * Convert a quaternion to Euler angles in the given order.
 */
function quaternionToEuler(q: Quat, order: EulerOrder): { x: number; y: number; z: number } {
  const m = quaternionToMatrix(q);
  return matrixToEuler(m, order);
}

// ============================================================================
// Internal: WebGL ↔ LASA core transforms
//
// WebGL (Three.js) axes: +X=right, +Y=up, +Z=backward
// LASA axes:             +X=forward, +Y=left, +Z=up
//
// Position mapping (no scale normalisation):
//   LASA.x = -WebGL.z   (forward = -backward)
//   LASA.y = -WebGL.x   (left = -right)
//   LASA.z =  WebGL.y   (up = up)
//
// Rotation: the LASA yaw/pitch/roll triple is the YXZ Euler
// decomposition of the WebGL-space quaternion:
//   yaw   = Y component (rotation about WebGL Y, the up axis)
//   pitch = X component (rotation about WebGL X, the right axis)
//   roll  = Z component (rotation about WebGL Z, the backward axis)
// The RH signs about those axes match §5.1: positive yaw
// anticlockwise about up, positive pitch upward, positive roll
// anticlockwise about the line of sight.
// ============================================================================

interface LasaPosition {
  x: number;
  y: number;
  z: number;
}

interface LasaRotation {
  yaw: number;
  pitch: number;
  roll: number;
}

const TWO_PI = 2 * Math.PI;

/**
 * Wrap an angle into (-π, π]. Applied to every pose this module
 * constructs (see makePose): the quaternion paths are already
 * range-bounded by atan2/asin, but the direct-mapping paths (A-Frame,
 * PixiJS) can receive 0..360° or accumulated angles, and the wire
 * encoder clamps out-of-range values rather than wrapping them.
 */
function wrapAngle(a: number): number {
  a = a % TWO_PI;
  if (a > Math.PI) a -= TWO_PI;
  else if (a <= -Math.PI) a += TWO_PI;
  return a;
}

// Every converter builds its result here, so wrapped angles are an
// invariant of Pose construction, not a per-path convention.
function makePose(pos: LasaPosition, rot: LasaRotation): Pose {
  return {
    x: pos.x,
    y: pos.y,
    z: pos.z,
    yaw: wrapAngle(rot.yaw),
    pitch: wrapAngle(rot.pitch),
    roll: wrapAngle(rot.roll),
  };
}

function webglPositionToLasa(pos: Vec3): LasaPosition {
  return { x: -pos.z, y: -pos.x, z: pos.y };
}

function lasaPositionToWebgl(pose: Pose): Vec3 {
  return { x: -pose.y, y: pose.z, z: -pose.x };
}

/**
 * Convert a WebGL-space quaternion to LASA rotation (radians).
 */
function webglQuatToLasaRotation(q: Quat): LasaRotation {
  const euler = quaternionToEuler(q, 'YXZ');
  return { yaw: euler.y, pitch: euler.x, roll: euler.z };
}

/**
 * Convert LASA rotation (radians) to a WebGL-space quaternion.
 */
function lasaRotationToWebglQuat(pose: Pose): Quat {
  return eulerToQuaternion(pose.pitch, pose.yaw, pose.roll, 'YXZ');
}

// ============================================================================
// Internal: framework → WebGL quaternion transforms
//
// For left-handed frameworks the quaternion is transformed from the
// framework's coordinate system to WebGL's right-handed system:
//   1. Transform the vector part (x,y,z) by the coordinate mapping matrix
//   2. Negate w if the mapping has det=-1 (handedness change)
// ============================================================================

/**
 * Transform a quaternion from Babylon/Unity space (LH, Y-up, +Z fwd) to WebGL space.
 *
 * Coordinate mapping: WebGL = (Bab.x, Bab.y, -Bab.z), det=-1
 * Quaternion: negate z and w, then normalize sign.
 */
function lhYupQuatToWebgl(q: Quat): Quat {
  // T = diag(1,1,-1), det=-1: transform vector by T, negate w
  // q_webgl = {x: q.x, y: q.y, z: -q.z, w: -q.w}
  // Normalize sign (negate all = same rotation): {x: -q.x, y: -q.y, z: q.z, w: q.w}
  return { x: -q.x, y: -q.y, z: q.z, w: q.w };
}

/**
 * Transform a quaternion from WebGL space to Babylon/Unity space (LH, Y-up, +Z fwd).
 * This is the inverse of lhYupQuatToWebgl.
 */
function webglQuatToLhYup(q: Quat): Quat {
  // Inverse of diag(1,1,-1) is itself (involutory), and det is still -1
  return { x: -q.x, y: -q.y, z: q.z, w: q.w };
}

/**
 * Transform a quaternion from Unreal space (LH, Z-up, +X fwd) to WebGL space.
 *
 * Coordinate mapping:
 *   WebGL.x = UE.y  (right)
 *   WebGL.y = UE.z  (up)
 *   WebGL.z = -UE.x (backward = -forward)
 *   det = -1
 *
 * Quaternion: transform vector part, negate w.
 */
function unrealQuatToWebgl(q: Quat): Quat {
  // T * (qx, qy, qz) = (qy, qz, -qx), then negate w
  // = {x: q.y, y: q.z, z: -q.x, w: -q.w}
  // Normalize sign: {x: -q.y, y: -q.z, z: q.x, w: q.w}
  return { x: -q.y, y: -q.z, z: q.x, w: q.w };
}

/**
 * Transform a quaternion from WebGL space to Unreal space.
 * Inverse of unrealQuatToWebgl.
 *
 * Inverse mapping:
 *   UE.x = -WebGL.z (forward = -backward)
 *   UE.y = WebGL.x  (right)
 *   UE.z = WebGL.y  (up)
 *   det = -1
 */
function webglQuatToUnreal(q: Quat): Quat {
  // T_inv * (qx, qy, qz) = (-qz, qx, qy), then negate w
  // = {x: -q.z, y: q.x, z: q.y, w: -q.w}
  // Normalize sign: {x: q.z, y: -q.x, z: -q.y, w: q.w}
  return { x: q.z, y: -q.x, z: -q.y, w: q.w };
}

// ============================================================================
// Public API: Three.js (RH, Y-up, -Z fwd, XYZ Euler, radians)
// ============================================================================

/**
 * Convert a Three.js position and rotation to a LASA pose.
 *
 * A THREE.Euler carries its own `.order`, which this function cannot
 * see; pass it explicitly when it is not the default. Camera rigs and
 * A-Frame commonly set 'YXZ'.
 *
 * @param position - Three.js position {x, y, z}
 * @param rotation - Three.js Euler rotation {x, y, z} in radians
 * @param order - the Euler's order, default 'XYZ' (the THREE.Euler default)
 */
export function threejsToLasa(position: Vec3, rotation: Vec3, order: EulerOrder = 'XYZ'): Pose {
  const q = eulerToQuaternion(rotation.x, rotation.y, rotation.z, order);
  return makePose(webglPositionToLasa(position), webglQuatToLasaRotation(q));
}

/**
 * Convert a LASA pose to a Three.js position and rotation.
 *
 * @param order - Euler order for the result, default 'XYZ'
 * @returns Three.js position and Euler rotation {x, y, z} in radians
 */
export function lasaToThreejs(pose: Pose, order: EulerOrder = 'XYZ'): Vec3Pose {
  const q = lasaRotationToWebglQuat(pose);
  const euler = quaternionToEuler(q, order);
  return {
    position: lasaPositionToWebgl(pose),
    rotation: { x: euler.x, y: euler.y, z: euler.z },
  };
}

// ============================================================================
// Public API: Babylon.js (LH, Y-up, +Z fwd, YXZ Euler, radians)
//
// Position: same right (+X) and up (+Y) as Three.js, but +Z = forward (LH)
// Rotation: YXZ Euler order, radians, left-handed
// ============================================================================

/**
 * Convert a Babylon.js position and rotation to a LASA pose.
 *
 * @param position - Babylon.js position {x, y, z}
 * @param rotation - Babylon.js Euler rotation {x, y, z} in radians, YXZ order
 */
export function babylonToLasa(position: Vec3, rotation: Vec3): Pose {
  // Position: Babylon to WebGL is just negate Z
  const webglPos: Vec3 = { x: position.x, y: position.y, z: -position.z };

  // Rotation: build quaternion in Babylon space, transform to WebGL
  const qBab = eulerToQuaternion(rotation.x, rotation.y, rotation.z, 'YXZ');
  const qWebgl = lhYupQuatToWebgl(qBab);

  return makePose(webglPositionToLasa(webglPos), webglQuatToLasaRotation(qWebgl));
}

/**
 * Convert a LASA pose to a Babylon.js position and rotation.
 *
 * @returns Babylon.js position and Euler rotation {x, y, z} in radians, YXZ order
 */
export function lasaToBabylon(pose: Pose): Vec3Pose {
  const webglPos = lasaPositionToWebgl(pose);
  const qWebgl = lasaRotationToWebglQuat(pose);

  // WebGL to Babylon: negate Z for position, transform quaternion
  const qBab = webglQuatToLhYup(qWebgl);
  const euler = quaternionToEuler(qBab, 'YXZ');

  return {
    position: { x: webglPos.x, y: webglPos.y, z: -webglPos.z },
    rotation: { x: euler.x, y: euler.y, z: euler.z },
  };
}

// ============================================================================
// Public API: A-Frame (RH, Y-up, -Z fwd, YXZ Euler, DEGREES)
//
// Same axes as Three.js (built on Three.js), but:
//   - Rotation in DEGREES
//   - YXZ Euler order (not XYZ)
// ============================================================================

/**
 * Convert an A-Frame position and rotation to a LASA pose.
 *
 * @param position - A-Frame position {x, y, z}
 * @param rotation - A-Frame rotation {x, y, z} in DEGREES, YXZ order
 */
export function aframeToLasa(position: Vec3, rotation: Vec3): Pose {
  // A-Frame uses the same axes and the same YXZ Euler order as the
  // LASA rotation's WebGL decomposition, so components map directly;
  // degrees convert to radians (and makePose wraps into (-π, π],
  // which matters here: A-Frame rotations are conventionally
  // authored 0..360):
  //   LASA yaw = A-Frame rotation.y (rotation about Y, the up axis)
  //   LASA pitch = A-Frame rotation.x (rotation about X, the right axis)
  //   LASA roll = A-Frame rotation.z (rotation about Z, the backward axis)
  return makePose(webglPositionToLasa(position), {
    yaw: degreesToRadians(rotation.y),
    pitch: degreesToRadians(rotation.x),
    roll: degreesToRadians(rotation.z),
  });
}

/**
 * Convert a LASA pose to an A-Frame position and rotation.
 *
 * @returns A-Frame position and rotation {x, y, z} in DEGREES, YXZ order
 */
export function lasaToAframe(pose: Pose): Vec3Pose {
  return {
    position: lasaPositionToWebgl(pose),
    rotation: {
      x: radiansToDegrees(pose.pitch),
      y: radiansToDegrees(pose.yaw),
      z: radiansToDegrees(pose.roll),
    },
  };
}

// ============================================================================
// Public API: PlayCanvas (RH, Y-up, -Z fwd, ZYX intrinsic Euler, DEGREES)
//
// Same axes as Three.js, but rotation in degrees, and
// pc.Quat.setFromEulerAngles composes extrinsic X-then-Y-then-Z about
// fixed axes, which equals intrinsic ZYX in this file's labelling.
// ============================================================================

/**
 * Convert a PlayCanvas position and rotation to a LASA pose.
 *
 * @param position - PlayCanvas position {x, y, z}
 * @param rotation - PlayCanvas Euler rotation {x, y, z} in DEGREES (extrinsic XYZ)
 */
export function playcanvasToLasa(position: Vec3, rotation: Vec3): Pose {
  const q = eulerToQuaternion(
    degreesToRadians(rotation.x),
    degreesToRadians(rotation.y),
    degreesToRadians(rotation.z),
    'ZYX',
  );
  return makePose(webglPositionToLasa(position), webglQuatToLasaRotation(q));
}

/**
 * Convert a LASA pose to a PlayCanvas position and rotation.
 *
 * @returns PlayCanvas position and Euler rotation {x, y, z} in DEGREES (extrinsic XYZ)
 */
export function lasaToPlaycanvas(pose: Pose): Vec3Pose {
  const q = lasaRotationToWebglQuat(pose);
  const euler = quaternionToEuler(q, 'ZYX');
  return {
    position: lasaPositionToWebgl(pose),
    rotation: {
      x: radiansToDegrees(euler.x),
      y: radiansToDegrees(euler.y),
      z: radiansToDegrees(euler.z),
    },
  };
}

// ============================================================================
// Public API: Unity (LH, Y-up, +Z fwd, YXZ intrinsic Euler, DEGREES)
//
// Same physical directions and Euler composition as Babylon
// (+X=right, +Y=up, +Z=forward, LH, YXZ intrinsic), but degrees.
// Unity's docs describe Quaternion.Euler as applying z, x, then y;
// that is application order, which equals intrinsic YXZ in this
// file's labelling.
// ============================================================================

/**
 * Convert a Unity position and rotation to a LASA pose.
 *
 * @param position - Unity position {x, y, z}
 * @param rotation - Unity Euler rotation {x, y, z} in DEGREES (Quaternion.Euler convention)
 */
export function unityToLasa(position: Vec3, rotation: Vec3): Pose {
  // Unity is Babylon's coordinate system in degrees, so delegate:
  // one copy of the LH-Y-up pipeline to maintain.
  return babylonToLasa(position, {
    x: degreesToRadians(rotation.x),
    y: degreesToRadians(rotation.y),
    z: degreesToRadians(rotation.z),
  });
}

/**
 * Convert a LASA pose to a Unity position and rotation.
 *
 * @returns Unity position and Euler rotation {x, y, z} in DEGREES (Quaternion.Euler convention)
 */
export function lasaToUnity(pose: Pose): Vec3Pose {
  const bab = lasaToBabylon(pose);
  return {
    position: bab.position,
    rotation: {
      x: radiansToDegrees(bab.rotation.x),
      y: radiansToDegrees(bab.rotation.y),
      z: radiansToDegrees(bab.rotation.z),
    },
  };
}

// ============================================================================
// Public API: Unreal Engine (LH, Z-up, +X fwd, FRotator degrees)
//
// Position: +X=forward, +Y=right, +Z=up (left-handed)
// Rotation: FRotator { pitch, yaw, roll } in degrees
//   yaw  = rotation around Z (up axis), positive = turn right
//   pitch = rotation around Y (right axis), positive = look up
//   roll  = rotation around X (forward axis), positive = roll CW from pilot view
//
// Note: Unreal's FRotator convention negates pitch and roll relative to the
// natural LH rotation direction. The conversion accounts for this.
// Unreal positions are conventionally centimetres; no scaling is
// applied here, so scale to metres before conversion if required.
// ============================================================================

/**
 * Convert an Unreal Engine position and FRotator to a LASA pose.
 *
 * @param position - Unreal position {x, y, z} (X=forward, Y=right, Z=up)
 * @param rotation - Unreal FRotator {pitch, yaw, roll} in degrees
 */
export function unrealToLasa(position: Vec3, rotation: FRotator): Pose {
  // Position: Unreal to LASA direct mapping
  //   LASA.x = UE.x  (forward = forward)
  //   LASA.y = -UE.y (left = -right, accounts for LH to RH)
  //   LASA.z = UE.z  (up = up)
  const pos: LasaPosition = { x: position.x, y: -position.y, z: position.z };

  // Rotation: FRotator to quaternion in Unreal space, then transform to WebGL
  // FRotator convention: positive pitch=up, positive roll=CW from pilot view
  // These are negated relative to natural LH rotation direction:
  //   Euler angles for ZYX: ex=-roll, ey=-pitch, ez=yaw
  const qUe = eulerToQuaternion(
    degreesToRadians(-rotation.roll),
    degreesToRadians(-rotation.pitch),
    degreesToRadians(rotation.yaw),
    'ZYX',
  );
  const qWebgl = unrealQuatToWebgl(qUe);

  return makePose(pos, webglQuatToLasaRotation(qWebgl));
}

/**
 * Convert a LASA pose to an Unreal Engine position and FRotator.
 *
 * @returns Unreal position {x, y, z} and FRotator {pitch, yaw, roll} in degrees
 */
export function lasaToUnreal(pose: Pose): UnrealPose {
  const qWebgl = lasaRotationToWebglQuat(pose);
  const qUe = webglQuatToUnreal(qWebgl);

  // Extract ZYX Euler from Unreal-space quaternion
  const euler = quaternionToEuler(qUe, 'ZYX');

  return {
    position: { x: pose.x, y: -pose.y, z: pose.z },
    rotation: {
      roll: radiansToDegrees(-euler.x),
      pitch: radiansToDegrees(-euler.y),
      yaw: radiansToDegrees(euler.z),
    },
  };
}

// ============================================================================
// Public API: PixiJS (2D, Y-down, +X right, single rotation, radians)
//
// Maps 2D screen coordinates to LASA's horizontal plane:
//   Pixi +X (right)  → LASA -Y (right direction)
//   Pixi +Y (down/screen south) → LASA -X (backward, in top-down view)
//   LASA z = 0 (no height in 2D)
//
// Rotation: Pixi positive rotation is clockwise (due to Y-down)
//   Pixi CW rotation → yaw right → negative LASA yaw
//
// Rotation zero is taken to face screen-up (LASA +x, forward in the
// top-down view). Sprite art that faces screen-right at rotation 0
// carries a constant quarter-turn: add π/2 to the sprite rotation
// before converting.
// ============================================================================

/**
 * Convert a PixiJS position and rotation to a LASA pose.
 *
 * Rotation 0 faces screen-up; see the section note for sprites whose
 * art faces screen-right. Accumulated rotations wrap into (-π, π].
 *
 * @param position - PixiJS position {x, y} (screen coordinates)
 * @param rotation - PixiJS rotation in radians (clockwise positive)
 */
export function pixiToLasa(position: Vec2, rotation: number): Pose {
  return makePose(
    { x: -position.y, y: -position.x, z: 0 },
    { yaw: -rotation, pitch: 0, roll: 0 },
  );
}

/**
 * Convert a LASA pose to a PixiJS position and rotation.
 * Height (z) is discarded. Only yaw maps to 2D rotation; pitch and roll are ignored.
 *
 * @returns PixiJS position {x, y} and rotation in radians
 */
export function lasaToPixi(pose: Pose): PixiPose {
  return {
    position: { x: -pose.y, y: -pose.x },
    rotation: -pose.yaw,
  };
}
