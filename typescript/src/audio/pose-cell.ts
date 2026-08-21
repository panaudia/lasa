/**
 * PoseCell — the per-entity shared pose slot (head-rotation-latency
 * rule, 2026-08-05): main writes the freshest pose, the MOQ worker
 * reads it at datagram-framing time so every mono-object packet carries
 * the newest completed write. A seqlock over a tiny SAB: no queue, no
 * postMessage latency, no torn reads (a read that races a write retries
 * until it sees one consistent snapshot).
 *
 * Layout: one 56-byte SAB — Int32 sequence at offset 0 (odd = write in
 * progress), 6 float64 values (x, y, z, yaw, pitch, roll) at offset 8.
 * Freshness beats pose/audio alignment by design: the pose on a packet
 * is NEWER than the audio in it (decision recorded in the build ledger).
 */

import type { Pose } from '../lasa/wire.js';

export interface PoseCellViews {
  /** Length-1 Int32Array over the SAB — the seqlock counter. */
  seq: Int32Array;
  /** Length-6 Float64Array over the SAB — x, y, z, yaw, pitch, roll. */
  values: Float64Array;
}

const POSE_CELL_BYTES = 8 + 6 * 8; // aligned seq word + 6 float64

/** Allocate the shared cell (main side). Zero-initialised = home pose, stable. */
export function allocatePoseCell(): PoseCellViews {
  const sab = new SharedArrayBuffer(POSE_CELL_BYTES);
  return {
    seq: new Int32Array(sab, 0, 1),
    values: new Float64Array(sab, 8, 6),
  };
}

export class PoseCell {
  constructor(private readonly views: PoseCellViews) {
    if (views.seq.length < 1 || views.values.length < 6) {
      throw new Error('PoseCell: seq must be length-1 Int32Array, values length-6 Float64Array');
    }
  }

  /**
   * WRITER (main). Publish a pose: odd the counter, write, even it.
   * Single-writer — concurrent writers would corrupt the seqlock.
   */
  write(p: Pose): void {
    const { seq, values } = this.views;
    Atomics.add(seq, 0, 1); // odd: write in progress
    values[0] = p.x;
    values[1] = p.y;
    values[2] = p.z;
    values[3] = p.yaw;
    values[4] = p.pitch;
    values[5] = p.roll;
    Atomics.add(seq, 0, 1); // even: stable
  }

  /**
   * READER (worker). Copy the latest consistent snapshot into `out`.
   * BOUNDED retry: the writer is the main thread, and if it is
   * preempted mid-write the counter stays odd for the whole preemption
   * — an unbounded retry loop would busy-spin the worker (stalling
   * capture, encode, sends AND sink decode) until main resumes. That
   * exact stall was observed live 2026-08-06 as accumulate-then-burst
   * jitter churn while dragging poses. On retry exhaustion `out` is
   * left UNTOUCHED: the caller reuses its previous snapshot, so the
   * packet carries a pose one write staler — harmless under the
   * freshest-wins design, and the next read catches up.
   */
  read(out: Pose): void {
    const { seq, values } = this.views;
    for (let attempt = 0; attempt < 3; attempt++) {
      const s1 = Atomics.load(seq, 0);
      if ((s1 & 1) === 1) continue; // mid-write
      const x = values[0]!;
      const y = values[1]!;
      const z = values[2]!;
      const yaw = values[3]!;
      const pitch = values[4]!;
      const roll = values[5]!;
      if (Atomics.load(seq, 0) !== s1) continue; // a write raced us
      out.x = x;
      out.y = y;
      out.z = z;
      out.yaw = yaw;
      out.pitch = pitch;
      out.roll = roll;
      return;
    }
    // Retries exhausted (writer mid-write, likely preempted): keep the
    // caller's previous snapshot rather than stalling the worker.
  }
}
