/**
 * D3 state round-trip: two clients on one server. Client A syncs the
 * state store (seeing its own admission keys arrive through the
 * catch-up lane), writes an own-entity attr upstream, and observes the
 * server's echo landing back in its store with a server seq; client B
 * connects afterwards and finds A's write in its catch-up. Driven by
 * Playwright via __stateRun.
 */

import { LasaClient } from '../src/lasa/client';
import { Store } from '../src/lasa/store';

export interface StateRunResult {
  ok: boolean;
  steps: string[];
  /** Keys visible in A's store after admission + own write. */
  aKeys: string[];
  /** A's echoed attr value, decoded. */
  aAttr?: string;
  /** B's view of A's attr, decoded. */
  bAttr?: string;
  error?: string;
}

const ATTR_KEY = 'lasa.entity.e-state-a.attrs.color';
const ATTR_VALUE = '"teal"';

async function until(pred: () => boolean, what: string, timeoutMs = 8000): Promise<void> {
  const t0 = performance.now();
  while (!pred()) {
    if (performance.now() - t0 > timeoutMs) throw new Error(`timeout waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 50));
  }
}

export async function stateRun(url: string, certHashBase64?: string): Promise<StateRunResult> {
  const steps: string[] = [];
  const step = (s: string) => {
    steps.push(s);
    console.log('[state]', s);
  };
  const aKeys: string[] = [];
  try {
    const a = await LasaClient.connect({
      url,
      spaceId: 'main',
      clientId: 'ts-state-a',
      entities: [{ id: 'e-state-a', name: 'State A' }],
      serverCertificateHashBase64: certHashBase64,
    });
    step('a-connected');

    const storeA = await Store.create('main', ['lasa.', 'app.']);
    const syncA = a.syncState(storeA);
    // Admission writes the entity marker + name; they arrive through
    // the first catch-up window (or live, either lane is legal).
    await until(() => storeA.get('lasa.entity.e-state-a') !== undefined, "A's admission keys");
    await until(() => storeA.subscribeParams().cursor !== undefined, "A's first frontier (cursor earned)");
    step('a-admission-visible');

    await a.writeState({ kind: 'set', key: ATTR_KEY, value: new TextEncoder().encode(ATTR_VALUE), seq: 0n });
    step('a-wrote-attr');

    // The write is authoritative only once the server echoes it back.
    await until(() => storeA.get(ATTR_KEY) !== undefined, "A's own-echo");
    await syncA.settled();
    const aAttr = new TextDecoder().decode(storeA.get(ATTR_KEY)!);
    step('a-own-echo');
    for (const [k] of storeA) aKeys.push(k);
    aKeys.sort();

    // Second client: A's write must be in B's catch-up.
    const b = await LasaClient.connect({
      url,
      spaceId: 'main',
      clientId: 'ts-state-b',
      entities: [{ id: 'e-state-b', name: 'State B' }],
      serverCertificateHashBase64: certHashBase64,
    });
    step('b-connected');
    const storeB = await Store.create('main', ['lasa.']);
    b.syncState(storeB);
    await until(() => storeB.get(ATTR_KEY) !== undefined, "B seeing A's attr");
    const bAttr = new TextDecoder().decode(storeB.get(ATTR_KEY)!);
    step('b-sees-a-attr');

    await b.close();
    await a.close();
    return { ok: true, steps, aKeys, aAttr, bAttr };
  } catch (e) {
    return { ok: false, steps, aKeys, error: String(e) };
  }
}
