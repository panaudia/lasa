import { test, expect } from '@playwright/test';
import type { TicketedRunResult } from '../interop/ticketed';
import { mintTicket } from './mint-ticket';

// Identity through the TS client, end-to-end in a real browser
// (lasa-core.md §4.4; identity-implementation.md batch E): the signed
// path admits, a missing ticket refuses with its §4.6 code surfaced
// typed, mode mismatch refuses (a space is ticketed XOR unticketed),
// and re-exercising the capability supersedes the predecessor, whose
// close surfaces through onClosed.
test('ticketed identity: admission, refusals, supersession', async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.text().startsWith('[ticketed]')) console.log(msg.text());
  });
  await page.goto('/interop/');
  await expect(page.locator('#log')).toContainText('interop ready');

  const privPem = process.env['LASA_TICKET_PRIVKEY_PEM'];
  expect(privPem, 'global-setup exports the issuer private key').toBeTruthy();
  const ticket = mintTicket(privPem!, {
    spaceId: 'main',
    clientId: 'ts-tick-a',
    entities: [{ id: 'e-tick-a', name: 'Tick A' }],
  });

  const result = (await page.evaluate(
    (args) => window.__ticketedRun(args),
    {
      ticketedUrl: process.env['LASA_TICKETED_URL'] ?? 'https://127.0.0.1:44444/lasa',
      openUrl: process.env['LASA_SPIKE_URL'] ?? 'https://127.0.0.1:44443/lasa',
      ticketedCertHash: process.env['LASA_TICKETED_CERT_HASH'],
      openCertHash: process.env['LASA_SPIKE_CERT_HASH'],
      ticket,
    }
  )) as TicketedRunResult;

  console.log('ticketed result:', JSON.stringify(result, null, 2));
  expect(result.error).toBeUndefined();
  expect(result.ok).toBe(true);
  // The §4.6 codes ride the WT close capsule into Chrome (defect fixed
  // 2026-08-21, wt-close-code-defect.md: the vendored webtransport-go
  // no longer sends STOP_SENDING in the same flight as the capsule).
  // Correctness rides the code alone, so these are hard assertions.
  expect(result.noTicketCode).toBe(0x1a5a01);
  expect(result.modeMismatchCode).toBe(0x1a5a08);
  // The superseded predecessor observed its own close.
  expect(result.supersededClose).toBeDefined();
});
