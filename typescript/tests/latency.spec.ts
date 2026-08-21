import { test, expect } from '@playwright/test';
import type { LatencyRunResult } from '../interop/latency';

// D3 step 5: the browser latency harness — mouth-to-ear onset and
// sink-pose flip, both detected in the playout worklet's rendered
// output (±one ~16 ms stats window; excludes the device DAC stage).
// The bounds here are deliberately generous tripwires; the precise
// numbers are appended to lasa/plan/latency-baseline.md per run.
test('browser mouth-to-ear and sink-pose-flip latency', async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.text().startsWith('[latency]')) console.log(msg.text());
  });
  await page.goto('/interop/');
  await expect(page.locator('#log')).toContainText('crossOriginIsolated=true');

  const url = process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa';
  const certHash = process.env['LASA_SPIKE_CERT_HASH'];
  const result = (await page.evaluate(
    ({ u, h }) => window.__latencyRun(u, h),
    { u: url, h: certHash }
  )) as LatencyRunResult;

  console.log('latency result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);

  // Tripwire bounds (loopback): generous, catching regressions of the
  // shape "something now waits on a queue", not tuning drift.
  expect(result.mouthToEarMs!).toBeGreaterThan(0);
  expect(result.mouthToEarMs!).toBeLessThan(250);
  expect(result.poseFlipMs!).toBeGreaterThan(0);
  expect(result.poseFlipMs!).toBeLessThan(250);

  // The flip really flipped.
  expect(Math.sign(result.domAfter!)).not.toBe(Math.sign(result.domBefore!));
});
