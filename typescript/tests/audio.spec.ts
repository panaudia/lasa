import { test, expect } from '@playwright/test';
import type { AudioRunResult } from '../interop/audio';

// D3: the full audio path in the browser — a multi-entity connection
// publishes WebCodecs opus mono-object packets and hears itself back
// through its second entity's binaural sink.
test('multi-entity audio round-trip over WebTransport', async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.text().startsWith('[audio]')) console.log(msg.text());
  });
  await page.goto('/interop/');
  await expect(page.locator('#log')).toContainText('interop ready');

  const url = process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa';
  const certHash = process.env['LASA_SPIKE_CERT_HASH'];
  const run = page.evaluate(({ u, h }) => window.__audioRun(u, h), { u: url, h: certHash }) as Promise<AudioRunResult>;

  // The speaker declared redundancy: 2, so the browser emits §5.1
  // repeats on its uplink. The server's ingress counters say whether
  // it accepts them — a framing disagreement would count as malformed
  // (and silence the round-trip). The page closes its client before
  // returning, which drops the entity from /stats, so sample the
  // counters while the run is in flight.
  // The run is short (~1 s of tone), so poll fast until it resolves,
  // keeping the latest sample in which the speaker was present.
  type Ingress = { Delivered: number; Malformed: number; Recovered: number; Lost: number };
  let speaker: Ingress | undefined;
  let floorSamples = 0;
  let running = true;
  void run.finally(() => (running = false));
  while (running) {
    await new Promise((r) => setTimeout(r, 50));
    const stats = (await (await fetch('http://127.0.0.1:44480/stats')).json()) as {
      entities: { id: string; latency_samples: number; depacketizer: Ingress }[];
    };
    const e = stats.entities.find((x) => x.id === 'e-ts-a');
    if (e && e.depacketizer.Delivered > (speaker?.Delivered ?? 0)) {
      speaker = e.depacketizer;
      floorSamples = e.latency_samples;
    }
  }
  const result = await run;

  console.log('audio result:', JSON.stringify(result, null, 2));
  console.log('speaker ingress:', JSON.stringify(speaker), 'ingest floor samples:', floorSamples);
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);
  expect(result.energeticFrames).toBeGreaterThanOrEqual(50);
  expect(speaker, 'speaker never appeared in /stats').toBeDefined();
  expect(speaker!.Delivered).toBeGreaterThanOrEqual(20);
  expect(speaker!.Malformed).toBe(0);
  // The server provisioned its ingest floor from the declaration:
  // 2 × (2 + 2) × 5 ms = 40 ms = 1920 samples, which the buffer
  // approaches over the (short) run — so assert well clear of the
  // undeclared ~5 ms (240-sample) fill rather than the settled floor.
  expect(floorSamples).toBeGreaterThanOrEqual(1200);
});
