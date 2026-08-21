import { test, expect } from '@playwright/test';
import type { CaptureRunResult } from '../spike/capture';

// D3 step 4: the complete 3-thread round trip — oscillator → capture
// worklet → SAB ring → worker Opus encode (mono-object, SAB-fresh pose)
// → server render → worker decode → SAB jitter ring → playout worklet.
// No main-thread audio in either direction; assertions from the playout
// worklet's own stats.
test('full capture-to-playout round trip through both SAB rings', async ({ page }) => {
  page.on('console', (msg) => {
    const t = msg.text();
    if (t.startsWith('[capture]') || t.startsWith('[JBUF]')) console.log(t);
  });
  await page.goto('/spike/');
  await expect(page.locator('#log')).toContainText('crossOriginIsolated=true');

  const url = process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa';
  const certHash = process.env['LASA_SPIKE_CERT_HASH'];
  const result = (await page.evaluate(
    ({ u, h }) => window.__captureRun(u, h),
    { u: url, h: certHash }
  )) as CaptureRunResult;

  console.log('capture result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);

  // The listener's worklet rendered energetic audio that originated in
  // the capture worklet on the other side of the server.
  expect(result.final!.started).toBe(true);
  expect(result.tapB!.frames).toBeGreaterThan(0);
  expect(result.tapB!.rmsL + result.tapB!.rmsR).toBeGreaterThan(0.02);

  // Steady state stays healthy across the 2 s run (incl. the live pose
  // update mid-stream).
  const underrunGrowth = result.final!.underruns - result.settle!.underruns;
  expect(underrunGrowth).toBeLessThanOrEqual(5);
  expect(result.final!.fillFrames).toBeGreaterThan(0);

  // The live setPose changed the rendered stereo image: the SAB-fresh
  // pose reached the worker's framing, the wire, and the server's HRTF
  // render. The two test poses produce images far apart in L/R
  // correlation (measured ≈ −0.9 vs ≈ +1.0), so 0.5 is a wide margin.
  const corrShift = Math.abs(result.tapB!.correlation - result.settleTapB!.correlation);
  expect(corrShift).toBeGreaterThan(0.5);
});
