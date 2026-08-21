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
  const result = (await page.evaluate(
    ({ u, h }) => window.__audioRun(u, h),
    { u: url, h: certHash }
  )) as AudioRunResult;

  console.log('audio result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);
  expect(result.energeticFrames).toBeGreaterThanOrEqual(50);
});
