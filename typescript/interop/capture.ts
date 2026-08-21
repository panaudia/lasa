/**
 * Step-4 full round trip: entity A publishes through the COMPLETE
 * capture chain (OscillatorNode → capture worklet → SAB ring → worker
 * Opus encode → mono-object datagrams stamped with the SAB-fresh pose)
 * and entity B hears it through the complete playout chain (worker
 * decode → SAB jitter ring → playout worklet). No main-thread audio in
 * either direction — the whole 3-thread architecture under test at
 * once. Driven by Playwright via __captureRun.
 */

import { LasaClient } from '../src/lasa/client';
import type { JitterBufferSnapshot } from '../src/audio/jitter-buffer-core';
import type { StereoMeterReport } from '../src/audio/stereo-meter-core';

export interface CaptureRunResult {
  ok: boolean;
  steps: string[];
  settle?: JitterBufferSnapshot;
  final?: JitterBufferSnapshot;
  /** Tap B window while at the INITIAL pose (before the live update). */
  settleTapB?: StereoMeterReport;
  /** Tap B window at the UPDATED pose — a different stereo image proves
   * the SAB-fresh pose path end to end. */
  tapB?: StereoMeterReport;
  error?: string;
}

export async function captureRun(url: string, certHashBase64?: string): Promise<CaptureRunResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    console.log('[capture]', s);
  };
  try {
    if (globalThis.crossOriginIsolated !== true) throw new Error('page is not cross-origin isolated');

    const client = await LasaClient.connect({
      url,
      spaceId: 'main',
      clientId: 'ts-capture',
      entities: [
        { id: 'e-cap-a', name: 'Capture Speaker' },
        { id: 'e-cap-b', name: 'Capture Listener' },
      ],
      serverCertificateHashBase64: certHashBase64,
    });
    step('connected');
    await client.entity('e-cap-a');
    await client.entity('e-cap-b');
    step('publishers-ready');

    // Listener first (playout warms while the capture spins up).
    const player = await client.playSink('e-cap-b', 'binaural', { debug: true });
    step('playout-started');

    // Speaker: a 440 Hz oscillator through the real capture pipeline.
    const captureCtx = new AudioContext({ sampleRate: 48000 });
    await captureCtx.resume();
    const osc = new OscillatorNode(captureCtx, { frequency: 440 });
    const oscGain = new GainNode(captureCtx, { gain: 0.4 });
    osc.connect(oscGain);
    const capture = await client.startCapture('e-cap-a', {
      source: oscGain,
      pose: { x: 0, y: 1, z: 0, yaw: 0, pitch: 0, roll: 0 },
    });
    osc.start();
    step('capturing');

    try {
      // Wait for rendered, energetic output at B's worklet.
      const t0 = performance.now();
      let settle: JitterBufferSnapshot | null = null;
      for (;;) {
        if (performance.now() - t0 > 15000) {
          throw new Error(
            `timeout; stats=${JSON.stringify(player.getJitterStats())} tapB=${JSON.stringify(player.getTapB())}`
          );
        }
        const snap = player.getJitterStats();
        const tapB = player.getTapB();
        if (snap?.started && tapB && tapB.frames > 0 && tapB.rmsL + tapB.rmsR > 0.02) {
          settle = snap;
          break;
        }
        await new Promise((r) => setTimeout(r, 100));
      }
      step('heard-through-worklet');

      // Let a clean Tap B window accumulate at the initial pose, then
      // sample it as the before-image.
      await new Promise((r) => setTimeout(r, 700));
      const settleTapB = player.getTapB()!;

      // Live pose update mid-stream (the freshness path, end to end):
      // moving the source must change the rendered stereo image.
      capture.setPose({ x: 1, y: 0, z: 0, yaw: 0.5, pitch: 0, roll: 0 });
      step('pose-updated');

      await new Promise((r) => setTimeout(r, 2000));
      const final = player.getJitterStats()!;
      const tapB = player.getTapB()!;
      step('steady-run-complete');

      await capture.stop();
      await client.close();
      await captureCtx.close();
      return { ok: true, steps, settle, final, settleTapB, tapB };
    } finally {
      try {
        osc.stop();
      } catch {
        // already stopped
      }
    }
  } catch (e) {
    return { ok: false, steps, error: String(e) };
  }
}
