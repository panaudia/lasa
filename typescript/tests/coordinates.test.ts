import { describe, it, expect } from 'vitest';
import {
  threejsToLasa,
  lasaToThreejs,
  babylonToLasa,
  lasaToBabylon,
  aframeToLasa,
  lasaToAframe,
  playcanvasToLasa,
  lasaToPlaycanvas,
  unityToLasa,
  lasaToUnity,
  unrealToLasa,
  lasaToUnreal,
  pixiToLasa,
  lasaToPixi,
} from '../src/lasa/coordinates.js';

import type { Pose } from '../src/lasa/wire.js';

// ============================================================================
// Test helpers
// ============================================================================

/** Approximate equality for floats (6 decimal places) */
function expectClose(actual: number, expected: number, label?: string) {
  expect(actual, label).toBeCloseTo(expected, 6);
}

/** Check a full LASA pose */
function expectPose(actual: Pose, expected: Pose) {
  expectClose(actual.x, expected.x, 'x');
  expectClose(actual.y, expected.y, 'y');
  expectClose(actual.z, expected.z, 'z');
  expectClose(actual.yaw, expected.yaw, 'yaw');
  expectClose(actual.pitch, expected.pitch, 'pitch');
  expectClose(actual.roll, expected.roll, 'roll');
}

const DEG_TO_RAD = Math.PI / 180;

/** Degrees to radians, for readable expected values */
function deg(d: number): number {
  return d * DEG_TO_RAD;
}

const IDENTITY: Pose = { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 };

// ============================================================================
// LASA pose reference (lasa-core.md §5.1):
//   Position: metres, x forward, y left, z up (converters do NOT
//   normalise scale)
//   Rotation: yaw/pitch/roll in radians; positive yaw anticlockwise
//   about z, positive pitch upward, positive roll anticlockwise about
//   the line of sight
//
//   WebGL converter mapping:
//     WebGL X  → LASA -y
//     WebGL Y  → LASA  z
//     WebGL Z  → LASA -x
// ============================================================================

// ============================================================================
// Three.js — right-handed, Y-up, -Z forward, XYZ Euler order, radians
// ============================================================================

describe('Three.js ↔ LASA', () => {
  describe('threejsToLasa', () => {
    it('converts origin (identity pose)', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectPose(result, IDENTITY);
    });

    it('converts positive X position (Three.js right → LASA)', () => {
      const result = threejsToLasa(
        { x: 1, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      // Three.js +X = right. LASA +y = left, so X → -y
      expectClose(result.y, -1, 'x→-y');
      expectClose(result.x, 0);
      expectClose(result.z, 0);
    });

    it('converts positive Y position (Three.js up → LASA)', () => {
      const result = threejsToLasa(
        { x: 0, y: 1, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      // Three.js +Y = up → LASA +z
      expectClose(result.z, 1, 'y→z');
      expectClose(result.x, 0);
      expectClose(result.y, 0);
    });

    it('converts positive Z position (Three.js backward → LASA)', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 1 },
        { x: 0, y: 0, z: 0 },
      );
      // Three.js +Z = backward (−forward). LASA +x = forward, so Z → -x
      expectClose(result.x, -1, 'z→-x');
      expectClose(result.y, 0);
      expectClose(result.z, 0);
    });

    it('converts yaw left 90° (positive Y rotation in RH = counter-clockwise from above)', () => {
      // Three.js: +Y rotation = yaw LEFT (right-hand rule around Y-up)
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: Math.PI / 2, z: 0 },
      );
      // Should produce non-zero yaw, zero pitch and roll
      expectClose(Math.abs(result.yaw), Math.PI / 2, 'yaw magnitude');
      expectClose(result.pitch, 0);
      expectClose(result.roll, 0);
    });

    it('converts pitch up 90° (positive X rotation in RH = nose up)', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: Math.PI / 2, y: 0, z: 0 },
      );
      // Should produce non-zero pitch, zero yaw and roll
      expectClose(Math.abs(result.pitch), Math.PI / 2, 'pitch magnitude');
      expectClose(result.yaw, 0);
    });

    it('converts roll 90° (positive Z rotation in RH)', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: Math.PI / 2 },
      );
      expectClose(Math.abs(result.roll), Math.PI / 2, 'roll magnitude');
      expectClose(result.yaw, 0);
    });

    it('converts combined rotation (45° yaw + 30° pitch)', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: deg(30), y: deg(45), z: 0 },
      );
      // Combined rotation should produce non-zero yaw and pitch
      expect(Math.abs(result.yaw)).toBeGreaterThan(0);
      expect(Math.abs(result.pitch)).toBeGreaterThan(0);
    });

    it('multi-axis golden: (30,45,60)° intrinsic XYZ', () => {
      // Independently derived: q = qx(30°)·qy(45°)·qz(60°), geometric
      // YXZ extraction (scratch derivation, not this module).
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: deg(30), y: deg(45), z: deg(60) },
      );
      expectClose(result.yaw, 0.85707194785, 'yaw');
      expectClose(result.pitch, 0.361367123907, 'pitch');
      expectClose(result.roll, 1.434794237852, 'roll');
    });

    it('supports all six THREE.Euler orders (roundtrip)', () => {
      // 'YZX' and 'XZY' are valid THREE.Euler orders too; the
      // documented camera.rotation.order pass-through must not throw
      // on any of the six, and each must roundtrip.
      const orders = ['XYZ', 'YXZ', 'ZXY', 'ZYX', 'YZX', 'XZY'] as const;
      const rot = { x: 0.3, y: -0.7, z: 1.1 };
      for (const order of orders) {
        const pose = threejsToLasa({ x: 1, y: 2, z: 3 }, rot, order);
        const back = lasaToThreejs(pose, order);
        expectClose(back.rotation.x, rot.x, `${order} rotation.x`);
        expectClose(back.rotation.y, rot.y, `${order} rotation.y`);
        expectClose(back.rotation.z, rot.z, `${order} rotation.z`);
      }
    });

    it('rejects an unknown Euler order with a clear error', () => {
      expect(() =>
        threejsToLasa(
          { x: 0, y: 0, z: 0 },
          { x: 0.1, y: 0.2, z: 0.3 },
          'XYX' as never,
        ),
      ).toThrow(/Euler order/);
    });

    it('honours a non-default Euler order (YXZ camera rigs)', () => {
      // With order 'YXZ' the components ARE the LASA decomposition,
      // so they pass through; interpreting the same numbers as the
      // default XYZ gives a different orientation.
      const rot = { x: deg(30), y: deg(45), z: deg(60) };
      const yxz = threejsToLasa({ x: 0, y: 0, z: 0 }, rot, 'YXZ');
      expectClose(yxz.yaw, deg(45), 'yaw');
      expectClose(yxz.pitch, deg(30), 'pitch');
      expectClose(yxz.roll, deg(60), 'roll');
      const xyz = threejsToLasa({ x: 0, y: 0, z: 0 }, rot);
      expect(Math.abs(xyz.pitch - yxz.pitch)).toBeGreaterThan(0.01);
    });

    it('converts negative positions', () => {
      const result = threejsToLasa(
        { x: -2, y: -3, z: -4 },
        { x: 0, y: 0, z: 0 },
      );
      expectClose(result.y, 2);   // -X → +y (double negation)
      expectClose(result.z, -3);  // -Y → -z
      expectClose(result.x, 4);   // -Z → +x (double negation)
    });

    it('preserves large positions without clamping', () => {
      const result = threejsToLasa(
        { x: 100, y: 200, z: 300 },
        { x: 0, y: 0, z: 0 },
      );
      // No normalisation — just axis swap
      expectClose(result.x, -300);
      expectClose(result.y, -100);
      expectClose(result.z, 200);
    });
  });

  describe('lasaToThreejs', () => {
    it('converts origin', () => {
      const result = lasaToThreejs(IDENTITY);
      expectClose(result.position.x, 0);
      expectClose(result.position.y, 0);
      expectClose(result.position.z, 0);
      expectClose(result.rotation.x, 0);
      expectClose(result.rotation.y, 0);
      expectClose(result.rotation.z, 0);
    });

    it('converts 90-degree yaw back to Three.js', () => {
      const result = lasaToThreejs(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 2, pitch: 0, roll: 0 },
      );
      expectClose(result.rotation.y, Math.PI / 2);
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 1, y: 2, z: 3 }, rot: { x: 0.1, y: 0.2, z: 0.3 } },
      { pos: { x: -5, y: 10, z: -15 }, rot: { x: 0, y: Math.PI / 4, z: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { x: 0, y: 0, z: 0 } },
      { pos: { x: 0.5, y: -0.5, z: 0.5 }, rot: { x: -0.3, y: 0.6, z: -0.1 } },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: threejs → lasa → threejs`, () => {
        const pose = threejsToLasa(pos, rot);
        const back = lasaToThreejs(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.position.z, pos.z, 'position.z');
        expectClose(back.rotation.x, rot.x, 'rotation.x');
        expectClose(back.rotation.y, rot.y, 'rotation.y');
        expectClose(back.rotation.z, rot.z, 'rotation.z');
      });
    });
  });

  describe('edge cases', () => {
    it('handles gimbal lock (pitch = 90°)', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: Math.PI / 2, y: 0, z: 0 },
      );
      // Should not produce NaN or Infinity
      expect(Number.isFinite(result.yaw)).toBe(true);
      expect(Number.isFinite(result.pitch)).toBe(true);
      expect(Number.isFinite(result.roll)).toBe(true);
    });

    it('handles full 360° rotation', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 2 * Math.PI, y: 0, z: 0 },
      );
      // 360° should be equivalent to 0° (or ±360°)
      expect(Number.isFinite(result.pitch)).toBe(true);
    });

    it('handles negative rotation', () => {
      const result = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: -Math.PI / 2, z: 0 },
      );
      expectClose(result.yaw, -Math.PI / 2);
    });

    it('handles very small values (near-zero)', () => {
      const result = threejsToLasa(
        { x: 1e-10, y: 1e-10, z: 1e-10 },
        { x: 1e-10, y: 1e-10, z: 1e-10 },
      );
      expectClose(result.x, 0, 'near-zero position');
      expectClose(result.yaw, 0, 'near-zero rotation');
    });
  });
});

// ============================================================================
// Babylon.js — left-handed, Y-up, +Z forward, YXZ Euler order, radians
//
// Axis differences from Three.js:
//   Babylon +Z = forward  (Three.js -Z = forward) → negate Z
//   Babylon +Y = up       (same as Three.js)
// ============================================================================

describe('Babylon.js ↔ LASA', () => {
  describe('babylonToLasa', () => {
    it('converts origin', () => {
      const result = babylonToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectPose(result, IDENTITY);
    });

    it('converts +Z position (Babylon forward → LASA +x)', () => {
      const result = babylonToLasa(
        { x: 0, y: 0, z: 1 },
        { x: 0, y: 0, z: 0 },
      );
      // Babylon +Z = forward → LASA +x
      expectClose(result.x, 1);
      expectClose(result.y, 0);
      expectClose(result.z, 0);
    });

    it('converts +Y position (Babylon up → LASA +z)', () => {
      const result = babylonToLasa(
        { x: 0, y: 1, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectClose(result.z, 1, 'y→z');
    });

    it('converts +X position (Babylon right → LASA -y)', () => {
      const result = babylonToLasa(
        { x: 1, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      // Babylon +X = right (same as Three.js), maps to LASA -y
      expectClose(result.y, -1, 'bab +x right → lasa -y');
      expectClose(result.x, 0);
      expectClose(result.z, 0);
    });

    it('converts yaw right 90° (positive Y in LH = clockwise from above)', () => {
      // Babylon.js LH: +Y rotation = yaw RIGHT (left-hand rule around Y-up)
      const result = babylonToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: Math.PI / 2, z: 0 },
      );
      // Should match Three.js yaw RIGHT (which is -π/2 in Three.js)
      const threeYawRight = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: -Math.PI / 2, z: 0 },
      );
      expectClose(result.yaw, threeYawRight.yaw, 'yaw should match');
      expectClose(result.pitch, 0);
      expectClose(result.roll, 0);
    });

    it('converts pitch via rotation around X (Babylon negated vs Three.js)', () => {
      // Babylon -rot.x should match Three.js +rot.x (same physical pitch)
      const result = babylonToLasa(
        { x: 0, y: 0, z: 0 },
        { x: -Math.PI / 4, y: 0, z: 0 },
      );
      const threePitchUp = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: Math.PI / 4, y: 0, z: 0 },
      );
      expectClose(result.pitch, threePitchUp.pitch, 'pitch should match');
    });

    it('multi-axis golden: (π/6, π/4, π/3) intrinsic YXZ', () => {
      // Independently derived: q = qy·qx·qz in Babylon's LH basis,
      // LH→RH transform, geometric YXZ extraction (scratch derivation,
      // not this module). Exact values: the same-axes YXZ
      // decomposition means the LH→RH transform reduces to negating
      // the x and y Euler components.
      const result = babylonToLasa(
        { x: 0, y: 0, z: 0 },
        { x: Math.PI / 6, y: Math.PI / 4, z: Math.PI / 3 },
      );
      expectClose(result.yaw, -Math.PI / 4, 'yaw');
      expectClose(result.pitch, -Math.PI / 6, 'pitch');
      expectClose(result.roll, Math.PI / 3, 'roll');
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 1, y: 2, z: 3 }, rot: { x: 0.1, y: 0.2, z: 0.3 } },
      { pos: { x: -5, y: 10, z: -15 }, rot: { x: 0, y: Math.PI / 4, z: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { x: 0, y: 0, z: 0 } },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: babylon → lasa → babylon`, () => {
        const pose = babylonToLasa(pos, rot);
        const back = lasaToBabylon(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.position.z, pos.z, 'position.z');
        expectClose(back.rotation.x, rot.x, 'rotation.x');
        expectClose(back.rotation.y, rot.y, 'rotation.y');
        expectClose(back.rotation.z, rot.z, 'rotation.z');
      });
    });
  });

  describe('edge cases', () => {
    it('handles gimbal lock', () => {
      const result = babylonToLasa(
        { x: 0, y: 0, z: 0 },
        { x: Math.PI / 2, y: 0, z: 0 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
      expect(Number.isFinite(result.pitch)).toBe(true);
      expect(Number.isFinite(result.roll)).toBe(true);
    });

    it('handles negative rotation', () => {
      const result = babylonToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: -Math.PI / 2, z: 0 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
    });
  });
});

// ============================================================================
// A-Frame — right-handed, Y-up, -Z forward, YXZ Euler order, DEGREES
//
// Same axes as Three.js, but rotation in degrees and YXZ Euler order
// ============================================================================

describe('A-Frame ↔ LASA', () => {
  describe('aframeToLasa', () => {
    it('converts origin', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectPose(result, IDENTITY);
    });

    it('converts positions same as Three.js (same axes)', () => {
      const result = aframeToLasa(
        { x: 1, y: 2, z: 3 },
        { x: 0, y: 0, z: 0 },
      );
      const threeResult = threejsToLasa(
        { x: 1, y: 2, z: 3 },
        { x: 0, y: 0, z: 0 },
      );
      expectClose(result.x, threeResult.x);
      expectClose(result.y, threeResult.y);
      expectClose(result.z, threeResult.z);
    });

    it('accepts degrees (90° yaw → π/2 rad)', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 90, z: 0 },
      );
      expectClose(result.yaw, Math.PI / 2);
    });

    it('converts 45° pitch (degrees → radians)', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 45, y: 0, z: 0 },
      );
      expectClose(result.pitch, Math.PI / 4);
    });

    it('converts 30° roll (degrees → radians)', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: 30 },
      );
      expectClose(result.roll, Math.PI / 6);
    });

    it('multi-axis: matches the Three.js quaternion path with order YXZ', () => {
      // A-Frame's direct mapping is a shortcut for the same YXZ
      // decomposition the quaternion path produces; this pins the two
      // together so a change to either is caught.
      const af = aframeToLasa({ x: 0, y: 0, z: 0 }, { x: 30, y: 45, z: 60 });
      const three = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: deg(30), y: deg(45), z: deg(60) },
        'YXZ',
      );
      expectClose(af.yaw, three.yaw, 'yaw');
      expectClose(af.pitch, three.pitch, 'pitch');
      expectClose(af.roll, three.roll, 'roll');
    });

    it('wraps 0..360° authoring into (-π, π] (270° yaw → -π/2)', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 270, z: 0 },
      );
      expectClose(result.yaw, -Math.PI / 2);
    });
  });

  describe('lasaToAframe', () => {
    it('outputs rotation in degrees', () => {
      const result = lasaToAframe(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 2, pitch: 0, roll: 0 },
      );
      // A-Frame uses degrees, so output should be in degrees
      expectClose(result.rotation.y, 90);
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 1, y: 2, z: 3 }, rot: { x: 10, y: 20, z: 30 } },
      { pos: { x: -5, y: 10, z: -15 }, rot: { x: 0, y: 45, z: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { x: 0, y: 0, z: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { x: -30, y: 60, z: -15 } },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: aframe → lasa → aframe`, () => {
        const pose = aframeToLasa(pos, rot);
        const back = lasaToAframe(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.position.z, pos.z, 'position.z');
        expectClose(back.rotation.x, rot.x, 'rotation.x');
        expectClose(back.rotation.y, rot.y, 'rotation.y');
        expectClose(back.rotation.z, rot.z, 'rotation.z');
      });
    });
  });

  describe('edge cases', () => {
    it('wraps 360° rotation to 0', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 360, y: 0, z: 0 },
      );
      expectClose(result.pitch, 0);
    });

    it('handles negative degrees', () => {
      const result = aframeToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: -90, z: 0 },
      );
      expectClose(result.yaw, -Math.PI / 2);
    });
  });
});

// ============================================================================
// PlayCanvas — right-handed, Y-up, -Z forward, ZYX intrinsic Euler, DEGREES
//
// Same axes as Three.js, but degrees, and setFromEulerAngles composes
// extrinsic X-then-Y-then-Z (= intrinsic ZYX). NOT Three.js's XYZ:
// single-axis rotations match Three.js, multi-axis do not.
// ============================================================================

describe('PlayCanvas ↔ LASA', () => {
  describe('playcanvasToLasa', () => {
    it('converts origin', () => {
      const result = playcanvasToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectPose(result, IDENTITY);
    });

    it('converts positions same as Three.js (same axes)', () => {
      const result = playcanvasToLasa(
        { x: 1, y: 2, z: 3 },
        { x: 0, y: 0, z: 0 },
      );
      const threeResult = threejsToLasa(
        { x: 1, y: 2, z: 3 },
        { x: 0, y: 0, z: 0 },
      );
      expectClose(result.x, threeResult.x);
      expectClose(result.y, threeResult.y);
      expectClose(result.z, threeResult.z);
    });

    it('accepts degrees (90° yaw → π/2 rad)', () => {
      const result = playcanvasToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 90, z: 0 },
      );
      expectClose(result.yaw, Math.PI / 2);
    });

    it('matches Three.js for single-axis rotations (order-invariant)', () => {
      const pcResult = playcanvasToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 45, z: 0 },
      );
      const threeResult = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: deg(45), z: 0 },
      );
      expectClose(pcResult.yaw, threeResult.yaw);
    });

    it('multi-axis golden: (30,45,60)° composes extrinsic XYZ (intrinsic ZYX)', () => {
      // Independently derived: q = qz(60°)·qy(45°)·qx(30°) per
      // pc.Quat.setFromEulerAngles, then geometric YXZ extraction via
      // rotated basis vectors (scratch derivation, not this module).
      // NOT equal to Three.js intrinsic XYZ of the same angles.
      const result = playcanvasToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 30, y: 45, z: 60 },
      );
      expectClose(result.yaw, 0.878960513152, 'yaw');
      expectClose(result.pitch, -0.284137965927, 'pitch');
      expectClose(result.roll, 0.691835813643, 'roll');
    });
  });

  describe('lasaToPlaycanvas', () => {
    it('outputs rotation in degrees', () => {
      const result = lasaToPlaycanvas(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 2, pitch: 0, roll: 0 },
      );
      // PlayCanvas uses degrees
      expectClose(result.rotation.y, 90);
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 1, y: 2, z: 3 }, rot: { x: 10, y: 20, z: 30 } },
      { pos: { x: -5, y: 10, z: -15 }, rot: { x: 0, y: 45, z: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { x: 0, y: 0, z: 0 } },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: playcanvas → lasa → playcanvas`, () => {
        const pose = playcanvasToLasa(pos, rot);
        const back = lasaToPlaycanvas(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.position.z, pos.z, 'position.z');
        expectClose(back.rotation.x, rot.x, 'rotation.x');
        expectClose(back.rotation.y, rot.y, 'rotation.y');
        expectClose(back.rotation.z, rot.z, 'rotation.z');
      });
    });
  });
});

// ============================================================================
// Unity — left-handed, Y-up, +Z forward, YXZ intrinsic Euler, DEGREES
//
//   Unity +X = right       (Three.js +X = right) → same
//   Unity +Y = up          (Three.js +Y = up)    → same
//   Unity +Z = forward     (Three.js -Z = forward) → negate Z
//   Left-handed: negate Z to convert to right-handed
//   Quaternion.Euler applies z, then x, then y = intrinsic YXZ
//   (the same composition as Babylon, in degrees). NOT 'ZXY'.
// ============================================================================

describe('Unity ↔ LASA', () => {
  describe('unityToLasa', () => {
    it('converts origin', () => {
      const result = unityToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectPose(result, IDENTITY);
    });

    it('converts +Z position (Unity forward → LASA +x)', () => {
      const result = unityToLasa(
        { x: 0, y: 0, z: 1 },
        { x: 0, y: 0, z: 0 },
      );
      // Unity +Z = forward. Left-to-right conversion negates Z:
      // Unity(0,0,1) → RH(0,0,-1) → Three.js -Z = forward → LASA +x
      expectClose(result.x, 1, 'unity +z forward → lasa +x');
    });

    it('converts +Y position (Unity up → LASA +z)', () => {
      const result = unityToLasa(
        { x: 0, y: 1, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      expectClose(result.z, 1, 'y→z');
    });

    it('converts +X position (Unity right)', () => {
      const result = unityToLasa(
        { x: 1, y: 0, z: 0 },
        { x: 0, y: 0, z: 0 },
      );
      // Unity +X = right, same as Three.js +X
      expectClose(result.y, -1, 'x→-y');
    });

    it('converts yaw right 90° (Unity LH: +90° Y = clockwise from above)', () => {
      const result = unityToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: 90, z: 0 },
      );
      // Should match Three.js yaw RIGHT (-π/2 around Y)
      const threeYawRight = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: -Math.PI / 2, z: 0 },
      );
      expectClose(result.yaw, threeYawRight.yaw, 'yaw right');
      expectClose(result.pitch, 0);
      expectClose(result.roll, 0);
    });

    it('multi-axis golden: (30,45,60)° composes as Quaternion.Euler (intrinsic YXZ)', () => {
      // Independently derived: q = qy(45°)·qx(30°)·qz(60°) per Unity's
      // documented z-then-x-then-y application order, LH→RH transform,
      // then geometric YXZ extraction (scratch derivation, not this
      // module). The exact values are (-π/4, -π/6, π/3): for a
      // same-axes YXZ decomposition the LH→RH transform reduces to
      // negating the x and y Euler components.
      const result = unityToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 30, y: 45, z: 60 },
      );
      expectClose(result.yaw, -Math.PI / 4, 'yaw');
      expectClose(result.pitch, -Math.PI / 6, 'pitch');
      expectClose(result.roll, Math.PI / 3, 'roll');
    });

    it('multi-axis: matches Babylon given the same angles in radians', () => {
      // Unity and Babylon share axes, handedness, and YXZ intrinsic
      // composition; only the units differ.
      const uni = unityToLasa({ x: 0, y: 0, z: 0 }, { x: 30, y: 45, z: 60 });
      const bab = babylonToLasa({ x: 0, y: 0, z: 0 }, { x: deg(30), y: deg(45), z: deg(60) });
      expectClose(uni.yaw, bab.yaw, 'yaw');
      expectClose(uni.pitch, bab.pitch, 'pitch');
      expectClose(uni.roll, bab.roll, 'roll');
    });
  });

  describe('lasaToUnity', () => {
    it('outputs rotation in degrees', () => {
      const result = lasaToUnity(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 2, pitch: 0, roll: 0 },
      );
      expect(Number.isFinite(result.rotation.y)).toBe(true);
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 1, y: 2, z: 3 }, rot: { x: 10, y: 20, z: 30 } },
      { pos: { x: -5, y: 10, z: -15 }, rot: { x: 0, y: 45, z: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { x: 0, y: 0, z: 0 } },
      { pos: { x: 100, y: 50, z: -200 }, rot: { x: -30, y: 60, z: 15 } },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: unity → lasa → unity`, () => {
        const pose = unityToLasa(pos, rot);
        const back = lasaToUnity(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.position.z, pos.z, 'position.z');
        expectClose(back.rotation.x, rot.x, 'rotation.x');
        expectClose(back.rotation.y, rot.y, 'rotation.y');
        expectClose(back.rotation.z, rot.z, 'rotation.z');
      });
    });
  });

  describe('edge cases', () => {
    it('handles gimbal lock (pitch = 90°)', () => {
      const result = unityToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 90, y: 0, z: 0 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
      expect(Number.isFinite(result.pitch)).toBe(true);
      expect(Number.isFinite(result.roll)).toBe(true);
    });
  });
});

// ============================================================================
// Unreal Engine — left-handed, Z-up, X-forward, FRotator degrees
//
// Uses FRotator: { pitch, yaw, roll } in degrees
// FVector: { x, y, z } where X=forward, Y=right, Z=up
//
//   Unreal +X = forward    → LASA +x
//   Unreal +Y = right      → LASA -y (LASA +y = left)
//   Unreal +Z = up         → LASA +z
//   Left-handed → right-handed: negate Y
// ============================================================================

describe('Unreal Engine ↔ LASA', () => {
  describe('unrealToLasa', () => {
    it('converts origin', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: 0, yaw: 0, roll: 0 },
      );
      expectPose(result, IDENTITY);
    });

    it('converts +X position (Unreal forward → LASA +x)', () => {
      const result = unrealToLasa(
        { x: 1, y: 0, z: 0 },
        { pitch: 0, yaw: 0, roll: 0 },
      );
      expectClose(result.x, 1, 'unreal +x forward → lasa +x');
    });

    it('converts +Z position (Unreal up → LASA +z)', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 1 },
        { pitch: 0, yaw: 0, roll: 0 },
      );
      expectClose(result.z, 1, 'unreal +z up → lasa +z');
    });

    it('converts +Y position (Unreal right)', () => {
      const result = unrealToLasa(
        { x: 0, y: 1, z: 0 },
        { pitch: 0, yaw: 0, roll: 0 },
      );
      // Unreal +Y = right, left-handed → negate for right-handed → LASA -y
      expectClose(result.y, -1, 'unreal +y right → lasa -y');
    });

    it('converts yaw right 90° (Unreal LH: +90° yaw = clockwise from above)', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: 0, yaw: 90, roll: 0 },
      );
      // Should match Three.js yaw RIGHT
      const threeYawRight = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: 0, y: -Math.PI / 2, z: 0 },
      );
      expectClose(result.yaw, threeYawRight.yaw, 'yaw right');
      expectClose(result.pitch, 0);
      expectClose(result.roll, 0);
    });

    it('converts pitch up 45° (Unreal: positive pitch = look up)', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: 45, yaw: 0, roll: 0 },
      );
      // Should match Three.js pitch up (+π/4 around X)
      const threePitchUp = threejsToLasa(
        { x: 0, y: 0, z: 0 },
        { x: Math.PI / 4, y: 0, z: 0 },
      );
      expectClose(result.pitch, threePitchUp.pitch, 'pitch up');
      expectClose(result.yaw, 0);
    });

    it('handles combined FRotator', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: 30, yaw: 45, roll: 60 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
      expect(Number.isFinite(result.pitch)).toBe(true);
      expect(Number.isFinite(result.roll)).toBe(true);
    });

    it('handles large positions (Unreal uses centimetre-scale)', () => {
      const result = unrealToLasa(
        { x: 1000, y: -500, z: 200 },
        { pitch: 0, yaw: 0, roll: 0 },
      );
      // No normalisation — values pass through with axis swap only
      expectClose(result.x, 1000);
      expectClose(result.y, 500); // negated
      expectClose(result.z, 200);
    });
  });

  describe('lasaToUnreal', () => {
    it('outputs FRotator-shaped rotation (pitch, yaw, roll)', () => {
      const result = lasaToUnreal(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 2, pitch: 0, roll: 0 },
      );
      expect(result.rotation).toHaveProperty('pitch');
      expect(result.rotation).toHaveProperty('yaw');
      expect(result.rotation).toHaveProperty('roll');
    });

    it('outputs degrees', () => {
      const result = lasaToUnreal(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 4, pitch: 0, roll: 0 },
      );
      expectClose(Math.abs(result.rotation.yaw), 45);
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 100, y: 200, z: 300 }, rot: { pitch: 10, yaw: 20, roll: 30 } },
      { pos: { x: -500, y: 100, z: -150 }, rot: { pitch: 0, yaw: 45, roll: 0 } },
      { pos: { x: 0, y: 0, z: 0 }, rot: { pitch: 0, yaw: 0, roll: 0 } },
      { pos: { x: 1000, y: -1000, z: 50 }, rot: { pitch: -30, yaw: 60, roll: -15 } },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: unreal → lasa → unreal`, () => {
        const pose = unrealToLasa(pos, rot);
        const back = lasaToUnreal(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.position.z, pos.z, 'position.z');
        expectClose(back.rotation.pitch, rot.pitch, 'rotation.pitch');
        expectClose(back.rotation.yaw, rot.yaw, 'rotation.yaw');
        expectClose(back.rotation.roll, rot.roll, 'rotation.roll');
      });
    });
  });

  describe('edge cases', () => {
    it('handles gimbal lock (pitch = 90°)', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: 90, yaw: 0, roll: 0 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
      expect(Number.isFinite(result.pitch)).toBe(true);
      expect(Number.isFinite(result.roll)).toBe(true);
    });

    it('handles 360° yaw', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: 0, yaw: 360, roll: 0 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
    });

    it('handles negative rotation', () => {
      const result = unrealToLasa(
        { x: 0, y: 0, z: 0 },
        { pitch: -45, yaw: -90, roll: -30 },
      );
      expect(Number.isFinite(result.yaw)).toBe(true);
      expect(Number.isFinite(result.pitch)).toBe(true);
      expect(Number.isFinite(result.roll)).toBe(true);
    });
  });
});

// ============================================================================
// PixiJS — 2D, Y-down, X-right, single rotation (radians, clockwise)
//
// Position: { x, y } screen coordinates (Y-down)
// Rotation: single number in radians, clockwise positive (due to Y-down)
//
// For spatial audio mapping:
//   Pixi X → LASA -y (right)
//   Pixi Y → LASA -x (screen south = backward in top-down view)
//   Height (LASA z) is fixed at 0
//   Rotation → yaw only (rotation in 2D = orientation on horizontal plane)
// ============================================================================

describe('PixiJS ↔ LASA', () => {
  describe('pixiToLasa', () => {
    it('converts origin', () => {
      const result = pixiToLasa({ x: 0, y: 0 }, 0);
      expectPose(result, IDENTITY);
    });

    it('converts +X position (Pixi right)', () => {
      const result = pixiToLasa({ x: 1, y: 0 }, 0);
      // Pixi +X should map to some horizontal axis in LASA
      expect(Number.isFinite(result.x)).toBe(true);
      expect(Number.isFinite(result.y)).toBe(true);
      expectClose(result.z, 0, 'z should be 0 for 2D');
    });

    it('converts +Y position (Pixi down → negated)', () => {
      const result = pixiToLasa({ x: 0, y: 1 }, 0);
      // Pixi Y is down, should be negated for forward direction
      expect(Number.isFinite(result.x)).toBe(true);
      expectClose(result.z, 0, 'z should be 0 for 2D');
    });

    it('sets z to 0 (2D has no height)', () => {
      const result = pixiToLasa({ x: 100, y: 200 }, 0);
      expectClose(result.z, 0);
    });

    it('converts rotation (radians → yaw only)', () => {
      const result = pixiToLasa({ x: 0, y: 0 }, Math.PI / 2);
      // 90° clockwise in Pixi (Y-down) → should map to yaw
      expect(Math.abs(result.yaw)).toBeGreaterThan(0);
      expectClose(result.pitch, 0);
      expectClose(result.roll, 0);
    });

    it('converts rotation in radians', () => {
      const result = pixiToLasa({ x: 0, y: 0 }, Math.PI);
      // 180° rotation
      expectClose(Math.abs(result.yaw), Math.PI);
    });

    it('handles negative rotation', () => {
      const result = pixiToLasa({ x: 0, y: 0 }, -Math.PI / 4);
      expect(Number.isFinite(result.yaw)).toBe(true);
    });
  });

  describe('lasaToPixi', () => {
    it('outputs 2D position { x, y }', () => {
      const result = lasaToPixi(
        { x: 1, y: 2, z: 3, yaw: 0, pitch: 0, roll: 0 },
      );
      expect(result.position).toHaveProperty('x');
      expect(result.position).toHaveProperty('y');
      expect(result.position).not.toHaveProperty('z');
    });

    it('outputs single rotation number (radians)', () => {
      const result = lasaToPixi(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 2, pitch: 0, roll: 0 },
      );
      expect(typeof result.rotation).toBe('number');
    });

    it('discards height (z) — projects to 2D', () => {
      const result1 = lasaToPixi(
        { x: 1, y: 2, z: 0, yaw: 0, pitch: 0, roll: 0 },
      );
      const result2 = lasaToPixi(
        { x: 1, y: 2, z: 100, yaw: 0, pitch: 0, roll: 0 },
      );
      expectClose(result1.position.x, result2.position.x);
      expectClose(result1.position.y, result2.position.y);
    });

    it('discards pitch and roll — only yaw maps to 2D', () => {
      const result1 = lasaToPixi(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 4, pitch: 0, roll: 0 },
      );
      const result2 = lasaToPixi(
        { x: 0, y: 0, z: 0, yaw: Math.PI / 4, pitch: Math.PI / 6, roll: Math.PI / 3 },
      );
      expectClose(result1.rotation, result2.rotation);
    });
  });

  describe('roundtrip', () => {
    const testCases = [
      { pos: { x: 100, y: 200 }, rot: 0 },
      { pos: { x: -50, y: 50 }, rot: Math.PI / 4 },
      { pos: { x: 0, y: 0 }, rot: -Math.PI / 2 },
      { pos: { x: 500, y: -300 }, rot: (3 * Math.PI) / 4 },
    ];

    testCases.forEach(({ pos, rot }, i) => {
      it(`roundtrip case ${i}: pixi → lasa → pixi`, () => {
        const pose = pixiToLasa(pos, rot);
        const back = lasaToPixi(pose);
        expectClose(back.position.x, pos.x, 'position.x');
        expectClose(back.position.y, pos.y, 'position.y');
        expectClose(back.rotation, rot, 'rotation');
      });
    });
  });

  describe('edge cases', () => {
    it('handles zero rotation', () => {
      const result = pixiToLasa({ x: 0, y: 0 }, 0);
      expectClose(result.yaw, 0);
    });

    it('wraps full 2π rotation to 0', () => {
      const result = pixiToLasa({ x: 0, y: 0 }, 2 * Math.PI);
      expectClose(result.yaw, 0);
    });

    it('wraps accumulated rotation into (-π, π]', () => {
      // e.g. a sprite whose rotation only ever increments
      const result = pixiToLasa({ x: 0, y: 0 }, 5 * Math.PI + Math.PI / 4);
      expectClose(result.yaw, Math.PI - Math.PI / 4);
    });

    it('handles very large positions (pixel coordinates)', () => {
      const result = pixiToLasa({ x: 1920, y: 1080 }, 0);
      expect(Number.isFinite(result.x)).toBe(true);
      expect(Number.isFinite(result.y)).toBe(true);
    });
  });
});

// ============================================================================
// Cross-framework consistency tests
//
// These tests define physical directions and rotations in plain English
// (forward, back, up, down, left, right, yaw-right, pitch-up, roll-clockwise)
// and verify that every framework's representation of that same physical
// motion produces identical LASA output.
//
// This catches bugs where an implementation just swaps axis names without
// accounting for handedness or axis permutation in rotations.
// ============================================================================

describe('Cross-framework consistency — positions', () => {
  it('all frameworks at origin produce the same LASA pose', () => {
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: 0 }, { pitch: 0, yaw: 0, roll: 0 }),
      pixiToLasa({ x: 0, y: 0 }, 0),
    ];

    for (const result of results) {
      expectPose(result, IDENTITY);
    }
  });

  it('1 unit FORWARD in each framework → same LASA position', () => {
    // "Forward" in each framework (the direction the default camera faces):
    //   Three.js:   -Z    Babylon.js: +Z    A-Frame:    -Z
    //   PlayCanvas: -Z    Unity:      +Z    Unreal:     +X
    const results = [
      threejsToLasa({ x: 0, y: 0, z: -1 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: 1 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: -1 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: -1 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: 1 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: 1, y: 0, z: 0 }, { pitch: 0, yaw: 0, roll: 0 }),
    ];
    for (const result of results) {
      expectClose(result.x, 1, 'forward → lasa +x');
      expectClose(result.y, 0, 'no lateral');
      expectClose(result.z, 0, 'no vertical');
    }
  });

  it('1 unit BACK in each framework → same LASA position', () => {
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 1 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: -1 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: 1 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: 1 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: -1 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: -1, y: 0, z: 0 }, { pitch: 0, yaw: 0, roll: 0 }),
    ];
    for (const result of results) {
      expectClose(result.x, -1, 'back → lasa -x');
      expectClose(result.y, 0, 'no lateral');
      expectClose(result.z, 0, 'no vertical');
    }
  });

  it('1 unit UP in each framework → same LASA position', () => {
    //   Three.js/Babylon/A-Frame/PlayCanvas/Unity: +Y = up
    //   Unreal: +Z = up
    const results = [
      threejsToLasa({ x: 0, y: 1, z: 0 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: 1, z: 0 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: 1, z: 0 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: 1, z: 0 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: 1, z: 0 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: 1 }, { pitch: 0, yaw: 0, roll: 0 }),
    ];
    for (const result of results) {
      expectClose(result.x, 0, 'no forward');
      expectClose(result.y, 0, 'no lateral');
      expectClose(result.z, 1, 'up → lasa +z');
    }
  });

  it('1 unit DOWN in each framework → same LASA position', () => {
    const results = [
      threejsToLasa({ x: 0, y: -1, z: 0 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: -1, z: 0 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: -1, z: 0 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: -1, z: 0 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: -1, z: 0 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: -1 }, { pitch: 0, yaw: 0, roll: 0 }),
    ];
    for (const result of results) {
      expectClose(result.x, 0, 'no forward');
      expectClose(result.y, 0, 'no lateral');
      expectClose(result.z, -1, 'down → lasa -z');
    }
  });

  it('1 unit RIGHT in each framework → same LASA position', () => {
    //   Three.js: +X    Babylon.js: +X    A-Frame: +X
    //   PlayCanvas: +X  Unity: +X         Unreal: +Y
    const results = [
      threejsToLasa({ x: 1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: 1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: 1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: 1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: 1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: 1, z: 0 }, { pitch: 0, yaw: 0, roll: 0 }),
    ];
    const expected = results[0]!;
    for (const result of results) {
      expectClose(result.x, 0, 'no forward');
      expectClose(result.z, 0, 'no vertical');
      expectClose(result.y, expected.y, 'right → same lasa y');
    }
    // Right is LASA -y (y is left)
    expectClose(expected.y, -1, 'right → lasa -y');
  });

  it('1 unit LEFT in each framework → same LASA position', () => {
    const results = [
      threejsToLasa({ x: -1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      babylonToLasa({ x: -1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      aframeToLasa({ x: -1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      playcanvasToLasa({ x: -1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      unityToLasa({ x: -1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: -1, z: 0 }, { pitch: 0, yaw: 0, roll: 0 }),
    ];
    const expected = results[0]!;
    for (const result of results) {
      expectClose(result.x, 0, 'no forward');
      expectClose(result.z, 0, 'no vertical');
      expectClose(result.y, expected.y, 'left → same lasa y');
    }
    // Left should be opposite to right, and LASA +y is left
    const rightResult = threejsToLasa({ x: 1, y: 0, z: 0 }, { x: 0, y: 0, z: 0 });
    expectClose(expected.y, -rightResult.y, 'left is opposite of right');
    expectClose(expected.y, 1, 'left → lasa +y');
  });
});

describe('Cross-framework consistency — rotations', () => {
  // These tests verify that the SAME PHYSICAL ROTATION expressed in each
  // framework's native convention produces the same LASA output.
  //
  // This is the critical test that catches naive axis-name-swapping bugs.
  // A rotation "turn 90° to the right" (yaw right) has DIFFERENT signs
  // in left-handed vs right-handed systems, and maps to different axis
  // components in Z-up vs Y-up systems.

  it('YAW RIGHT 90° (turn right, viewed from above) → same LASA yaw', () => {
    // "Yaw right" = clockwise viewed from above:
    //   Three.js/A-Frame/PlayCanvas (RH, Y-up): NEGATIVE rotation.y
    //   Babylon/Unity (LH, Y-up): POSITIVE rotation.y
    //   Unreal (LH, Z-up): POSITIVE FRotator.yaw
    // LASA §5.1: positive yaw anticlockwise about z, so yaw right is
    // NEGATIVE LASA yaw.
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: -Math.PI / 2, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: Math.PI / 2, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: -90, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: -90, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 90, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: 0 }, { pitch: 0, yaw: 90, roll: 0 }),
    ];
    const expected = results[0]!;
    for (let i = 0; i < results.length; i++) {
      expectClose(results[i]!.yaw, expected.yaw, `framework ${i}: yaw`);
      expectClose(results[i]!.pitch, 0, `framework ${i}: pitch should be 0`);
      expectClose(results[i]!.roll, 0, `framework ${i}: roll should be 0`);
    }
    // Yaw right = negative LASA yaw (positive yaw is anticlockwise about z)
    expectClose(expected.yaw, -Math.PI / 2, 'yaw right → lasa -π/2');
  });

  it('YAW LEFT 90° (turn left, viewed from above) → same LASA yaw', () => {
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: Math.PI / 2, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: -Math.PI / 2, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 90, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 90, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: -90, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: 0 }, { pitch: 0, yaw: -90, roll: 0 }),
    ];
    const expected = results[0]!;
    for (let i = 0; i < results.length; i++) {
      expectClose(results[i]!.yaw, expected.yaw, `framework ${i}: yaw`);
      expectClose(results[i]!.pitch, 0, `framework ${i}: pitch`);
      expectClose(results[i]!.roll, 0, `framework ${i}: roll`);
    }
    // Yaw left should be opposite of yaw right, positive in LASA
    const yawRight = threejsToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: -Math.PI / 2, z: 0 });
    expectClose(expected.yaw, -yawRight.yaw, 'left is opposite of right');
    expectClose(expected.yaw, Math.PI / 2, 'yaw left → lasa +π/2');
  });

  it('PITCH UP 45° (look up, tilt nose toward sky) → same LASA pitch', () => {
    // "Pitch up" per framework:
    //   Three.js/A-Frame/PlayCanvas (RH, Y-up): POSITIVE rotation.x
    //   Babylon/Unity (LH, Y-up): NEGATIVE rotation.x
    //   Unreal: POSITIVE FRotator.pitch
    // LASA §5.1: positive pitch upward.
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 0 }, { x: Math.PI / 4, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: 0 }, { x: -Math.PI / 4, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: 0 }, { x: 45, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: 0 }, { x: 45, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: 0 }, { x: -45, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: 0 }, { pitch: 45, yaw: 0, roll: 0 }),
    ];
    const expected = results[0]!;
    for (let i = 0; i < results.length; i++) {
      expectClose(results[i]!.yaw, 0, `framework ${i}: yaw should be 0`);
      expectClose(results[i]!.pitch, expected.pitch, `framework ${i}: pitch`);
      expectClose(results[i]!.roll, 0, `framework ${i}: roll should be 0`);
    }
    // Pitch up = positive LASA pitch
    expectClose(expected.pitch, Math.PI / 4, 'pitch up → lasa +π/4');
  });

  it('PITCH DOWN 45° → same LASA pitch (opposite of pitch up)', () => {
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 0 }, { x: -Math.PI / 4, y: 0, z: 0 }),
      babylonToLasa({ x: 0, y: 0, z: 0 }, { x: Math.PI / 4, y: 0, z: 0 }),
      aframeToLasa({ x: 0, y: 0, z: 0 }, { x: -45, y: 0, z: 0 }),
      playcanvasToLasa({ x: 0, y: 0, z: 0 }, { x: -45, y: 0, z: 0 }),
      unityToLasa({ x: 0, y: 0, z: 0 }, { x: 45, y: 0, z: 0 }),
      unrealToLasa({ x: 0, y: 0, z: 0 }, { pitch: -45, yaw: 0, roll: 0 }),
    ];
    const expected = results[0]!;
    for (let i = 0; i < results.length; i++) {
      expectClose(results[i]!.pitch, expected.pitch, `framework ${i}: pitch`);
    }
    // Down should be opposite of up
    const pitchUp = threejsToLasa({ x: 0, y: 0, z: 0 }, { x: Math.PI / 4, y: 0, z: 0 });
    expectClose(expected.pitch, -pitchUp.pitch, 'down is opposite of up');
  });

  it('ROLL CLOCKWISE 30° (tilt head right) → same LASA roll', () => {
    // "Roll clockwise" = looking forward, the top of the head tilts to
    // the right. Per framework:
    //   Three.js/A-Frame/PlayCanvas: NEGATIVE rotation.z
    //   Babylon/Unity: NEGATIVE rotation.z
    //   Unreal: POSITIVE FRotator.roll
    // LASA §5.1: positive roll anticlockwise about the line of sight,
    // so clockwise roll is NEGATIVE LASA roll.
    const results = [
      threejsToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -Math.PI / 6 }),
      babylonToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -Math.PI / 6 }),
      aframeToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -30 }),
      playcanvasToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -30 }),
      unityToLasa({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -30 }),
      unrealToLasa({ x: 0, y: 0, z: 0 }, { pitch: 0, yaw: 0, roll: 30 }),
    ];
    const expected = results[0]!;
    for (let i = 0; i < results.length; i++) {
      expectClose(results[i]!.yaw, 0, `framework ${i}: yaw should be 0`);
      expectClose(results[i]!.pitch, 0, `framework ${i}: pitch should be 0`);
      expectClose(results[i]!.roll, expected.roll, `framework ${i}: roll`);
    }
    expectClose(expected.roll, -Math.PI / 6, 'roll clockwise → lasa -π/6');
  });

  it('combined yaw+pitch in every framework → same LASA output', () => {
    // Verify that a specific LASA rotation converts to each framework
    // and back consistently.
    const pose: Pose = { x: 0, y: 0, z: 0, yaw: deg(45), pitch: deg(30), roll: 0 };

    const backs = [
      (() => { const r = lasaToThreejs(pose); return threejsToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToBabylon(pose); return babylonToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToAframe(pose); return aframeToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToPlaycanvas(pose); return playcanvasToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToUnity(pose); return unityToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToUnreal(pose); return unrealToLasa(r.position, r.rotation); })(),
    ];

    for (const back of backs) {
      expectClose(back.yaw, pose.yaw, 'yaw');
      expectClose(back.pitch, pose.pitch, 'pitch');
      expectClose(back.roll, pose.roll, 'roll');
    }
  });

  it('combined yaw+pitch+roll in every framework → same LASA output', () => {
    const pose: Pose = { x: 5, y: -3, z: 7, yaw: deg(30), pitch: deg(-20), roll: deg(15) };

    const backs = [
      (() => { const r = lasaToThreejs(pose); return threejsToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToBabylon(pose); return babylonToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToUnity(pose); return unityToLasa(r.position, r.rotation); })(),
      (() => { const r = lasaToUnreal(pose); return unrealToLasa(r.position, r.rotation); })(),
    ];

    for (const back of backs) {
      expectPose(back, pose);
    }
  });
});
