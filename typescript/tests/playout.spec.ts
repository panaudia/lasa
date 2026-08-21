import { test, expect } from '@playwright/test';
import type { PlayoutRunResult } from '../interop/playout';

// D3 step 3: the 3-thread playout chain — worker decode → SAB jitter
// ring → playout worklet. Assertions come from the worklet's own stats
// (v3 snapshot + Tap B rendered-output meter), so a pass means audio
// was actually rendered at the render quantum, not merely decoded.
test('worklet playout through the SAB ring', async ({ page }) => {
  page.on('console', (msg) => {
    const t = msg.text();
    if (t.startsWith('[playout]') || t.startsWith('[JBUF]')) console.log(t);
  });
  await page.goto('/interop/');
  await expect(page.locator('#log')).toContainText('interop ready');
  await expect(page.locator('#log')).toContainText('crossOriginIsolated=true');

  const url = process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa';
  const certHash = process.env['LASA_SPIKE_CERT_HASH'];
  const result = (await page.evaluate(
    ({ u, h }) => window.__playoutRun(u, h),
    { u: url, h: certHash }
  )) as PlayoutRunResult;

  console.log('playout result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);

  // The worklet produced audio (startup snap taken) and rendered
  // energetic output — the tone reached the destination-facing tap.
  expect(result.final!.started).toBe(true);
  expect(result.tapB!.frames).toBeGreaterThan(0);
  expect(result.tapB!.rmsL + result.tapB!.rmsR).toBeGreaterThan(0.02);

  // Steady state: the 2 s run after settling added at most a small
  // number of underruns (warm-up may legitimately contain some).
  const underrunGrowth = result.final!.underruns - result.settle!.underruns;
  expect(underrunGrowth).toBeLessThanOrEqual(5);
  // Fill sits inside the ring (not pinned at zero or capacity).
  expect(result.final!.fillFrames).toBeGreaterThan(0);
});
