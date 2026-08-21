/**
 * D2 interop scenario: the copied draft-16 codec against the LASA server
 * (panaudia/moqtransport fork) over WebTransport. Proves, in order:
 * WT session up → CLIENT_SETUP carrying the 0x1A5A1 Connection Config
 * → SERVER_SETUP → SUBSCRIBE presence → SUBSCRIBE_OK → presence
 * datagrams flowing (which also means the server admitted our entity
 * and is rendering the space). Driven by Playwright via __interopRun.
 */

import { MoqConnection } from '../src/transport/connection';
import { MoqSession } from '../src/transport/session';
import { MoqRole } from '../src/transport/types';
import { connectionConfigParam, presenceNamespace, TRACK_PRESENCE } from '../src/lasa/setup';

export interface InteropResult {
  ok: boolean;
  steps: string[];
  presenceBytes?: number;
  error?: string;
}

const logEl = document.getElementById('log')!;
function log(line: string): void {
  logEl.textContent += line + '\n';
  console.log('[interop]', line);
}

function hashOption(certHashBase64?: string) {
  if (!certHashBase64) return undefined;
  const raw = atob(certHashBase64);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return [{ algorithm: 'sha-256', value: bytes.buffer }];
}

async function run(url: string, certHashBase64?: string): Promise<InteropResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    log(s);
  };
  try {
    const conn = new MoqConnection(url, true);
    // Subprotocol negotiation is under investigation (D2): try with,
    // fall back to without, so the failing layer is unambiguous.
    try {
      await conn.connect({
        protocols: ['moqt-16'],
        serverCertificateHashes: hashOption(certHashBase64),
      });
      step('webtransport-connected-with-subprotocol');
    } catch (e1) {
      log('connect with subprotocol failed: ' + String(e1));
      const conn2 = new MoqConnection(url, true);
      await conn2.connect({ serverCertificateHashes: hashOption(certHashBase64) });
      step('webtransport-connected-no-subprotocol');
      return await runSession(conn2, steps, step);
    }
    return await runSession(conn, steps, step);
  } catch (e) {
    log('ERROR: ' + String(e));
    return { ok: false, steps, error: String(e) };
  }
}

async function runSession(
  conn: MoqConnection,
  steps: string[],
  step: (s: string) => void
): Promise<InteropResult> {
  try {
    const session = new MoqSession(conn, true);
    await session.initialize(MoqRole.PUBSUB, undefined, 100, [
      connectionConfigParam({
        clientId: 'interop',
        entities: [{ id: 'e-interop', name: 'Interop' }],
      }),
    ]);
    step('setup-complete');

    const subId = await session.subscribe(presenceNamespace('main'), TRACK_PRESENCE);
    const alias = session.getTrackAlias(subId);
    if (alias === undefined) {
      throw new Error('no track alias after SUBSCRIBE_OK');
    }
    step('subscribed-presence alias=' + alias);

    let presenceBytes = 0;
    const gotDatagram = new Promise<void>((resolve) => {
      conn.registerDatagramHandler(alias, (payload) => {
        presenceBytes = payload.length;
        resolve();
      });
    });
    session.startMessageLoop();

    await Promise.race([
      gotDatagram,
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('no presence datagram within 8s')), 8000)),
    ]);
    step('presence-datagram bytes=' + presenceBytes);

    return { ok: true, steps, presenceBytes };
  } catch (e) {
    log('ERROR: ' + String(e));
    return { ok: false, steps, error: String(e) };
  }
}

import { audioRun, type AudioRunResult } from './audio';
import { stateRun, type StateRunResult } from './state';
import { playoutRun, type PlayoutRunResult } from './playout';
import { captureRun, type CaptureRunResult } from './capture';
import { latencyRun, type LatencyRunResult } from './latency';
import { ticketedRun, type TicketedRunArgs, type TicketedRunResult } from './ticketed';

declare global {
  interface Window {
    __interopRun: (url: string, certHashBase64?: string) => Promise<InteropResult>;
    __audioRun: (url: string, certHashBase64?: string) => Promise<AudioRunResult>;
    __stateRun: (url: string, certHashBase64?: string) => Promise<StateRunResult>;
    __playoutRun: (url: string, certHashBase64?: string) => Promise<PlayoutRunResult>;
    __captureRun: (url: string, certHashBase64?: string) => Promise<CaptureRunResult>;
    __latencyRun: (url: string, certHashBase64?: string) => Promise<LatencyRunResult>;
    __ticketedRun: (args: TicketedRunArgs) => Promise<TicketedRunResult>;
  }
}
window.__interopRun = run;
window.__audioRun = audioRun;
window.__stateRun = stateRun;
window.__playoutRun = playoutRun;
window.__captureRun = captureRun;
window.__latencyRun = latencyRun;
window.__ticketedRun = ticketedRun;
// The capture/playout SAB rings need cross-origin isolation; surface the
// state on load so a missing COOP/COEP header is obvious immediately.
log(`interop ready (crossOriginIsolated=${String(globalThis.crossOriginIsolated)})`);
