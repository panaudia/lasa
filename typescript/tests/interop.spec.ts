import { test, expect } from '@playwright/test';
import type { SpikeResult } from '../spike/main';

// D2: the copied draft-16 codec completes the LASA handshake against
// the panaudia/moqtransport fork over WebTransport and receives
// presence datagrams.
test('LASA handshake + presence over WebTransport', async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.text().startsWith('[spike]')) console.log(msg.text());
  });
  await page.goto('/spike/');
  await expect(page.locator('#log')).toContainText('spike ready');

  const url = process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa';
  const certHash = process.env['LASA_SPIKE_CERT_HASH'];
  console.log('cert hash from env:', certHash ?? '(missing!)');
  const result = (await page.evaluate(
    ({ u, h }) => window.__spikeRun(u, h),
    { u: url, h: certHash }
  )) as SpikeResult;

  console.log('spike result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);
  expect(result.steps.join(' ')).toContain('setup-complete');
  expect(result.steps.join(' ')).toContain('presence-datagram');
  expect(result.presenceBytes ?? 0).toBeGreaterThan(0);
});
