/**
 * D3 audio round-trip: a MULTI-ENTITY LasaClient (two entities, one
 * connection) where entity A publishes a WebCodecs-encoded 440 Hz tone
 * as 5 ms mono-object packets and entity B hears it through its own
 * binaural sink, decoded with WebCodecs. No worklets, no mic — the
 * protocol path in isolation, driven by Playwright via __audioRun.
 */

import { LasaClient } from '../src/lasa/client';

export interface AudioRunResult {
  ok: boolean;
  steps: string[];
  sinkFrames: number;
  energeticFrames: number;
  error?: string;
}

const SAMPLE_RATE = 48000;
const FRAME_SAMPLES = 240; // 5 ms

export async function audioRun(url: string, certHashBase64?: string): Promise<AudioRunResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    console.log('[audio]', s);
  };
  let sinkFrames = 0;
  let energeticFrames = 0;
  try {
    if (typeof AudioEncoder === 'undefined' || typeof AudioDecoder === 'undefined') {
      throw new Error('WebCodecs unavailable in this browser');
    }

    const client = await LasaClient.connect({
      url,
      spaceId: 'main',
      clientId: 'ts-audio',
      entities: [
        // Declared redundancy: the browser emits §5.1 repeats on its
        // uplink, and the server must accept them (malformed packets
        // are discarded, which would silence the round-trip).
        { id: 'e-ts-a', name: 'TS Speaker', redundancy: 2 },
        { id: 'e-ts-b', name: 'TS Listener' },
      ],
      serverCertificateHashBase64: certHashBase64,
    });
    step('connected-two-entities');

    // Both publishers must materialise — the multi-entity contract.
    const pubA = await client.entity('e-ts-a');
    await client.entity('e-ts-b');
    step('both-publishers-ready');

    // Decoder side first: e-ts-b's own binaural sink.
    const decoder = new AudioDecoder({
      output: (data: AudioData) => {
        const n = data.numberOfFrames * 2;
        const pcm = new Float32Array(n);
        data.copyTo(pcm, { planeIndex: 0, format: 'f32' });
        data.close();
        let sum = 0;
        for (let i = 0; i < n; i++) sum += pcm[i]! * pcm[i]!;
        if (Math.sqrt(sum / n) > 0.005) energeticFrames++;
      },
      error: (e: DOMException) => console.log('[audio] decoder error', e.message),
    });
    decoder.configure({ codec: 'opus', sampleRate: SAMPLE_RATE, numberOfChannels: 2 });
    await client.subscribeSink('e-ts-b', 'binaural', (_seq, pkt) => {
      sinkFrames++;
      decoder.decode(
        new EncodedAudioChunk({
          type: 'key',
          timestamp: sinkFrames * 5000,
          data: pkt.audio,
        })
      );
    });
    step('sink-subscribed');

    // Encoder: 440 Hz mono at 5 ms frames, published at ~real cadence.
    const pose = { x: 0, y: 1, z: 0, yaw: 0, pitch: 0, roll: 0 };
    const encoder = new AudioEncoder({
      output: (chunk: EncodedAudioChunk) => {
        const bytes = new Uint8Array(chunk.byteLength);
        chunk.copyTo(bytes);
        void pubA.publish({ pose, audio: bytes }); // the worker sequences the track
      },
      error: (e: DOMException) => console.log('[audio] encoder error', e.message),
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

    // Wait for a healthy run of energetic sink frames.
    await new Promise<void>((resolve, reject) => {
      const deadline = setTimeout(
        () => reject(new Error(`timeout: ${sinkFrames} sink frames, ${energeticFrames} energetic`)),
        15000
      );
      const poll = setInterval(() => {
        if (energeticFrames >= 50) {
          clearTimeout(deadline);
          clearInterval(poll);
          resolve();
        }
      }, 100);
    }).finally(() => clearInterval(feeder));
    step('heard');

    await client.close();
    return { ok: true, steps, sinkFrames, energeticFrames };
  } catch (e) {
    return { ok: false, steps, sinkFrames, energeticFrames, error: String(e) };
  }
}
