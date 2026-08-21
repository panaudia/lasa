/**
 * Step-5 browser latency harness — the TS-client rows for
 * lasa/plan/latency-baseline.md. Two measurements in one run, both
 * detected in the playout worklet's RENDERED output via fine-grained
 * stats windows (statsEvery=6 ≈ 16 ms — the measurement resolution),
 * so results are to the render point and exclude only the device DAC
 * stage (reported separately from the graph):
 *
 * 1. MOUTH-TO-EAR (onset): speaker entity A runs the full capture
 *    chain with its tone gated silent; at t0 the gate opens; latency =
 *    first energetic window at listener B. Crosses: capture worklet +
 *    SAB ring + worker Opus encode + wire + server (depacketize,
 *    jitter, render, sink encode) + wire + worker decode + SAB jitter
 *    ring + worklet render.
 *
 * 2. SINK-POSE FLIP (head rotation): B also runs a (silent) capture
 *    pipeline, so its pose rides the SAB pose cell exactly like a real
 *    head-tracked client; at t1 B's yaw flips by π. A 2 kHz lateral
 *    tone (the Go pose harness's technique — ILD needs frequency)
 *    makes L/R dominance swap; latency = first persistently flipped
 *    window. Crosses: pose cell + worker framing + wire + server
 *    latest-wins sink-pose slot + render + downstream + jitter ring +
 *    worklet.
 *
 * Driven by Playwright via __latencyRun.
 */

import { LasaClient } from '../src/lasa/client';
import type { StereoMeterReport } from '../src/audio/stereo-meter-core';

export interface LatencyRunResult {
  ok: boolean;
  steps: string[];
  /** Onset latency, ms (t0 → first energetic window receipt). */
  mouthToEarMs?: number;
  /** Pose-flip latency, ms (t1 → first persistently flipped window). */
  poseFlipMs?: number;
  /** Window length (measurement resolution), ms. */
  windowMs?: number;
  /** L/R dominance before/after the flip, (rmsL−rmsR)/(rmsL+rmsR). */
  domBefore?: number;
  domAfter?: number;
  /** Graph-reported device-side latencies (excluded from the numbers above). */
  baseLatencyMs?: number | null;
  outputLatencyMs?: number | null;
  error?: string;
}

const SR = 48000;
const STATS_EVERY = 6; // reads → 6*128/48000 ≈ 16 ms windows

interface Win {
  t: number;
  tapB: StereoMeterReport;
}

function dominance(tapB: StereoMeterReport): number {
  const sum = tapB.rmsL + tapB.rmsR;
  return sum > 1e-9 ? (tapB.rmsL - tapB.rmsR) / sum : 0;
}

export async function latencyRun(url: string, certHashBase64?: string): Promise<LatencyRunResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    console.log('[latency]', s);
  };
  try {
    if (globalThis.crossOriginIsolated !== true) throw new Error('page is not cross-origin isolated');

    const client = await LasaClient.connect({
      url,
      spaceId: 'main',
      clientId: 'ts-latency',
      entities: [
        { id: 'e-lat-a', name: 'Latency Speaker' },
        { id: 'e-lat-b', name: 'Latency Listener' },
      ],
      serverCertificateHashBase64: certHashBase64,
    });
    step('connected');
    await client.entity('e-lat-a');
    await client.entity('e-lat-b');

    const player = await client.playSink('e-lat-b', 'binaural', { statsEvery: STATS_EVERY });
    const windows: Win[] = [];
    player.setStatsListener((_snap, tapB, atMs) => {
      if (tapB) windows.push({ t: atMs, tapB });
      if (windows.length > 4000) windows.shift();
    });
    step('playout-started');

    const captureCtx = new AudioContext({ sampleRate: SR });
    await captureCtx.resume();

    // Speaker A: 2 kHz tone, gated silent until t0, LATERAL of B's home
    // pose so dominance is decisive after the flip test's yaw change.
    const osc = new OscillatorNode(captureCtx, { frequency: 2000 });
    const gate = new GainNode(captureCtx, { gain: 0 });
    osc.connect(gate);
    osc.start();
    await client.startCapture('e-lat-a', {
      source: gate,
      pose: { x: 0, y: 1, z: 0, yaw: 0, pitch: 0, roll: 0 },
    });

    // Listener B: silent capture so its pose rides the SAB pose cell —
    // the exact head-tracked-client path. ConstantSource(0) produces
    // real (silent) samples, keeping packets flowing.
    const silence = new ConstantSourceNode(captureCtx, { offset: 0 });
    silence.start();
    const captureB = await client.startCapture('e-lat-b', {
      source: silence,
      pose: { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 },
    });
    step('captures-running');

    // Let both pipelines + the sink settle while silent.
    await new Promise((r) => setTimeout(r, 1500));

    // ---- Measurement 1: mouth-to-ear onset ----
    const RMS_ON = 0.01;
    windows.length = 0;
    const t0 = performance.now();
    gate.gain.value = 0.4;
    const onsetWin = await waitFor(
      windows,
      (w) => w.tapB.rmsL + w.tapB.rmsR > RMS_ON,
      t0,
      10000,
      'energetic onset'
    );
    const mouthToEarMs = onsetWin.t - t0;
    step(`onset-detected ${mouthToEarMs.toFixed(1)}ms`);

    // Let dominance stabilise at yaw 0, then record it.
    await new Promise((r) => setTimeout(r, 1000));
    const before = windows[windows.length - 1]!;
    const domBefore = dominance(before.tapB);
    if (Math.abs(domBefore) < 0.05) {
      throw new Error(`source not lateral enough for a dominance flip (dom=${domBefore.toFixed(3)})`);
    }
    step(`dominance-settled ${domBefore.toFixed(3)}`);

    // ---- Measurement 2: sink-pose flip (head rotation) ----
    windows.length = 0;
    const t1 = performance.now();
    captureB.setPose({ x: 0, y: 0, z: 0, yaw: Math.PI, pitch: 0, roll: 0 });
    const flipWin = await waitFor(
      windows,
      (w, i, all) => {
        const d = dominance(w.tapB);
        if (Math.sign(d) === Math.sign(domBefore) || Math.abs(d) < 0.05) return false;
        // Persistent: the next window (if present) must agree.
        const next = all[i + 1];
        return next ? Math.sign(dominance(next.tapB)) === Math.sign(d) : false;
      },
      t1,
      10000,
      'dominance flip'
    );
    const poseFlipMs = flipWin.t - t1;
    const domAfter = dominance(flipWin.tapB);
    step(`pose-flip-detected ${poseFlipMs.toFixed(1)}ms (dom ${domBefore.toFixed(2)} → ${domAfter.toFixed(2)})`);

    const graph = player.getAudioGraphReport();
    await client.close();
    await captureCtx.close();
    return {
      ok: true,
      steps,
      mouthToEarMs,
      poseFlipMs,
      windowMs: (STATS_EVERY * 128 * 1000) / SR,
      domBefore,
      domAfter,
      baseLatencyMs: graph.context.baseLatencyMs,
      outputLatencyMs: graph.context.outputLatencyMs,
    };
  } catch (e) {
    return { ok: false, steps, error: String(e) };
  }
}

/** Poll the windows array until one (arrived after tStart) matches. */
async function waitFor(
  windows: Win[],
  pred: (w: Win, i: number, all: Win[]) => boolean,
  tStart: number,
  timeoutMs: number,
  what: string
): Promise<Win> {
  const deadline = tStart + timeoutMs;
  let scanned = 0;
  for (;;) {
    for (; scanned < windows.length; scanned++) {
      const w = windows[scanned]!;
      if (w.t <= tStart) continue;
      if (pred(w, scanned, windows)) return w;
    }
    // Re-scan the last window next round too: persistence checks need
    // its successor, which may not have arrived yet.
    if (scanned > 0) scanned--;
    if (performance.now() > deadline) throw new Error(`timeout waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 4));
  }
}
