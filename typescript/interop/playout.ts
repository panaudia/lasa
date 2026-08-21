/**
 * Step-3 playout round-trip: one multi-entity client where entity A
 * publishes a WebCodecs 440 Hz tone (main-framed mono-object datagrams
 * — the protocol publish path) and entity B plays its own binaural sink
 * through the FULL 3-thread playout chain: worker decode → SAB jitter
 * ring → playout worklet → destination. Success is asserted from the
 * worklet's own stats (v3 snapshot + Tap B rendered-output meter), not
 * from a main-thread decoder. Driven by Playwright via __playoutRun.
 */

import { LasaClient } from '../src/lasa/client';
import type { JitterBufferSnapshot } from '../src/audio/jitter-buffer-core';
import type { StereoMeterReport } from '../src/audio/stereo-meter-core';

export interface PlayoutRunResult {
  ok: boolean;
  steps: string[];
  /** Snapshot at the settle point and at the end (underrun delta matters). */
  settle?: JitterBufferSnapshot;
  final?: JitterBufferSnapshot;
  tapB?: StereoMeterReport;
  error?: string;
}

const SAMPLE_RATE = 48000;
const FRAME_SAMPLES = 240; // 5 ms

export async function playoutRun(url: string, certHashBase64?: string): Promise<PlayoutRunResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    console.log('[playout]', s);
  };
  try {
    if (typeof AudioEncoder === 'undefined') throw new Error('WebCodecs unavailable');
    if (globalThis.crossOriginIsolated !== true) throw new Error('page is not cross-origin isolated');

    const client = await LasaClient.connect({
      url,
      spaceId: 'main',
      clientId: 'ts-playout',
      entities: [
        { id: 'e-po-a', name: 'Playout Speaker' },
        { id: 'e-po-b', name: 'Playout Listener' },
      ],
      serverCertificateHashBase64: certHashBase64,
    });
    step('connected');
    const pubA = await client.entity('e-po-a');
    await client.entity('e-po-b');
    step('publishers-ready');

    // The full playout chain for B's own binaural sink.
    const player = await client.playSink('e-po-b', 'binaural', { debug: true });
    step('playout-started');

    // Tone publisher: 440 Hz mono at 5 ms frames, real cadence.
    let seq = 0n;
    const pose = { x: 0, y: 1, z: 0, yaw: 0, pitch: 0, roll: 0 };
    const encoder = new AudioEncoder({
      output: (chunk: EncodedAudioChunk) => {
        const bytes = new Uint8Array(chunk.byteLength);
        chunk.copyTo(bytes);
        void pubA.publish(seq++, { pose, audio: bytes });
      },
      error: (e: DOMException) => console.log('[playout] encoder error', e.message),
    });
    encoder.configure({
      codec: 'opus',
      sampleRate: SAMPLE_RATE,
      numberOfChannels: 1,
      bitrate: 64000,
      opus: { frameDuration: 5000 },
    } as AudioEncoderConfig);

    let phase = 0;
    const pcm = new Float32Array(FRAME_SAMPLES);
    let ts = 0;
    const feed = () => {
      for (let i = 0; i < FRAME_SAMPLES; i++) {
        pcm[i] = 0.4 * Math.sin(phase);
        phase += (2 * Math.PI * 440) / SAMPLE_RATE;
      }
      encoder.encode(
        new AudioData({
          format: 'f32-planar',
          sampleRate: SAMPLE_RATE,
          numberOfFrames: FRAME_SAMPLES,
          numberOfChannels: 1,
          timestamp: ts,
          data: pcm,
        })
      );
      ts += 5000;
    };
    const feeder = setInterval(feed, 5);
    step('publishing');

    try {
      // Wait for the worklet to actually be playing rendered audio:
      // snapshot started + an energetic Tap B window.
      const t0 = performance.now();
      let settle: JitterBufferSnapshot | null = null;
      for (;;) {
        if (performance.now() - t0 > 15000) {
          throw new Error(
            `timeout waiting for energetic playout; stats=${JSON.stringify(player.getJitterStats())} tapB=${JSON.stringify(player.getTapB())}`
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
      step('worklet-playing');

      // Steady run: a further 2 s of playout should be healthy (the
      // underrun count may include warm-up; only its growth matters).
      await new Promise((r) => setTimeout(r, 2000));
      const final = player.getJitterStats()!;
      const tapB = player.getTapB()!;
      step('steady-run-complete');

      await client.close();
      return { ok: true, steps, settle, final, tapB };
    } finally {
      clearInterval(feeder);
    }
  } catch (e) {
    return { ok: false, steps, error: String(e) };
  }
}
