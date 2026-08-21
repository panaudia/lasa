/**
 * Ticketed-mode interop: the browser exercises the identity rules of
 * lasa-core.md §4.4 end-to-end against two real servers (one ticketed,
 * one open):
 *
 *   1. ticket + setups → admitted (the signed path)
 *   2. no ticket → ticketed space refuses (0x1A5A01), surfaced as a
 *      typed LasaRejectionError — correctness rides the code
 *   3. ticket → OPEN space refuses (0x1A5A08 mode mismatch)
 *   4. same ticket again → newest exercise wins: the second connection
 *      supersedes the first, whose onClosed fires
 *
 * Driven by Playwright via __ticketedRun.
 */

import { LasaClient } from '../src/lasa/client';
import { LasaRejectionError } from '../src/lasa/errors';

export interface TicketedRunArgs {
  ticketedUrl: string;
  openUrl: string;
  ticketedCertHash?: string;
  openCertHash?: string;
  /** Ticket for client ts-tick-a with entity e-tick-a. */
  ticket: string;
}

export interface TicketedRunResult {
  ok: boolean;
  steps: string[];
  /** §4.6 code from connecting the ticketed space without a ticket. */
  noTicketCode?: number;
  /** §4.6 code from presenting the ticket to the open space. */
  modeMismatchCode?: number;
  /** The superseded first connection's close info (step 4). */
  supersededClose?: { closeCode?: number; reason?: string };
  error?: string;
}

async function rejectionCode(p: Promise<unknown>): Promise<number | undefined> {
  try {
    await p;
    return undefined;
  } catch (e) {
    return e instanceof LasaRejectionError ? e.code : undefined;
  }
}

export async function ticketedRun(args: TicketedRunArgs): Promise<TicketedRunResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    console.log('[ticketed]', s);
  };
  const result: TicketedRunResult = { ok: false, steps };
  try {
    // 1 — the signed path: instantiate the ticket-defined entity.
    let firstClosed: { closeCode?: number; reason?: string } | null = null;
    const a1 = await LasaClient.connect({
      url: args.ticketedUrl,
      spaceId: 'main',
      clientId: 'ts-tick-a',
      ticket: args.ticket,
      setups: [{ id: 'e-tick-a' }],
      serverCertificateHashBase64: args.ticketedCertHash,
      onClosed: (info) => {
        firstClosed = info;
      },
    });
    step('ticketed-admitted');

    // 2 — no ticket into the ticketed space: refused with a §4.6 code.
    result.noTicketCode = await rejectionCode(
      LasaClient.connect({
        url: args.ticketedUrl,
        spaceId: 'main',
        clientId: 'ts-tick-b',
        entities: [{ id: 'e-tick-b', name: 'No Ticket' }],
        serverCertificateHashBase64: args.ticketedCertHash,
      })
    );
    step(`no-ticket-refused:0x${result.noTicketCode?.toString(16)}`);

    // 3 — the ticket into the OPEN space: mode mismatch.
    result.modeMismatchCode = await rejectionCode(
      LasaClient.connect({
        url: args.openUrl,
        spaceId: 'main',
        clientId: 'ts-tick-a',
        ticket: args.ticket,
        entities: [{ id: 'e-tick-a', name: 'Tick A' }],
        serverCertificateHashBase64: args.openCertHash,
      })
    );
    step(`mode-mismatch:0x${result.modeMismatchCode?.toString(16)}`);

    // 4 — re-exercise the same capability: newest wins (§4.4), the
    // first connection is superseded and its onClosed fires.
    const a2 = await LasaClient.connect({
      url: args.ticketedUrl,
      spaceId: 'main',
      clientId: 'ts-tick-a',
      ticket: args.ticket,
      setups: [{ id: 'e-tick-a' }],
      serverCertificateHashBase64: args.ticketedCertHash,
    });
    step('re-exercise-admitted');
    const t0 = performance.now();
    while (firstClosed === null && performance.now() - t0 < 8000) {
      await new Promise((r) => setTimeout(r, 50));
    }
    if (firstClosed === null) throw new Error('superseded connection never observed its close');
    result.supersededClose = firstClosed;
    step('predecessor-superseded');

    await a2.close();
    await a1.close().catch(() => {});
    result.ok = true;
  } catch (e) {
    result.error = String(e);
  }
  return result;
}
