import { test, expect } from '@playwright/test';
import type { StateRunResult } from '../spike/state';

// D3 state client: subgroup lanes → store, own-echo of a client write,
// admission keys visible, and a second client observing the write.
test('state sync round-trip over WebTransport', async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.text().startsWith('[state]')) console.log(msg.text());
  });
  await page.goto('/spike/');
  await expect(page.locator('#log')).toContainText('spike ready');

  const url = process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa';
  const certHash = process.env['LASA_SPIKE_CERT_HASH'];
  const result = (await page.evaluate(
    ({ u, h }) => window.__stateRun(u, h),
    { u: url, h: certHash }
  )) as StateRunResult;

  console.log('state result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);
  // Admission wrote the marker and name for A's entity.
  expect(result.aKeys).toContain('lasa.entity.e-state-a');
  expect(result.aKeys).toContain('lasa.entity.e-state-a.name');
  // A's write echoed back with a server seq, and B saw it in catch-up.
  expect(result.aAttr).toBe('"teal"');
  expect(result.bAttr).toBe('"teal"');
});
