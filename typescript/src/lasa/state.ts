/**
 * The LASA state sync client (lasa-core.md §6.5–§6.7): subscribes the
 * client sink 'state' track with the 0x1A5A3 subscribe parameter, feeds
 * the two subgroup lanes into the Store (subgroup 0 live, subgroup 1
 * catch-up), and re-subscribes with backoff when the server ends the
 * instance (PUBLISH_DONE — too-far-behind is the native resync path).
 *
 * Mirrors Go `lasa/client.SyncState`; the store rules live in store.ts.
 */

import type { StateSession } from '../worker/worker-transport.js';
import type { SubgroupHeader, SubgroupObject, SubgroupRouter } from '../transport/subgroups.js';
import { clientNamespace, TRACK_STATE } from './setup.js';
import { encodeSubscribeParams, parseStateMessage, SUBSCRIBE_PARAM_STATE } from './statewire.js';
import type { Store } from './store.js';

const CATCH_UP_LANE = 1n;

export interface StateSyncOptions {
  /** Initial re-subscribe backoff in ms (doubles to a 5 s cap). */
  initialBackoffMs?: number;
  /** Console logging of subscribe, instance and re-subscribe events. */
  debug?: boolean;
}

/**
 * Runs the state subscription loop until stop() is called. All store
 * mutation happens on a single promise chain, preserving per-lane
 * arrival order.
 */
export class StateSync {
  private stopped = false;
  private currentAlias: number | null = null;
  private wakeup: (() => void) | null = null;
  private endInstance: (() => void) | null = null;
  // Serializes store application: lane handlers are synchronous
  // callbacks but the store's apply methods are async.
  private chain: Promise<void> = Promise.resolve();

  /** @internal Built by LasaClient. */
  constructor(
    private readonly session: StateSession,
    private readonly router: SubgroupRouter,
    private readonly spaceId: string,
    private readonly clientId: string,
    private readonly store: Store,
    private readonly opts: StateSyncOptions = {}
  ) {}

  private log(...args: unknown[]): void {
    if (this.opts.debug) console.log('[state]', ...args);
  }

  /** Starts the subscribe/apply/re-subscribe loop; resolves on stop(). */
  async run(): Promise<void> {
    let backoff = this.opts.initialBackoffMs ?? 100;
    while (!this.stopped) {
      try {
        await this.syncOnce();
        backoff = this.opts.initialBackoffMs ?? 100; // instance ended after a healthy run
      } catch (e) {
        this.log('sync error:', e);
      }
      if (this.stopped) return;
      await new Promise<void>((resolve) => {
        const t = setTimeout(resolve, backoff);
        this.wakeup = () => {
          clearTimeout(t);
          resolve();
        };
      });
      this.wakeup = null;
      if (backoff < 5000) backoff *= 2;
    }
  }

  /** Ends the loop: unregisters the lane, ends the current instance, and wakes any backoff wait. */
  stop(): void {
    this.stopped = true;
    if (this.currentAlias !== null) {
      this.router.unregister(this.currentAlias);
      this.currentAlias = null;
    }
    this.endInstance?.();
    this.wakeup?.();
  }

  /** Resolves after every message applied so far has been processed. */
  async settled(): Promise<void> {
    await this.chain;
  }

  private async syncOnce(): Promise<void> {
    const { prefixes, cursor } = this.store.subscribeParams();
    const raw = encodeSubscribeParams(prefixes, cursor);
    const subId = await this.session.subscribe(
      clientNamespace(this.spaceId, this.clientId, 'sink'),
      TRACK_STATE,
      undefined,
      undefined,
      [{ type: SUBSCRIBE_PARAM_STATE, value: raw }]
    );
    if (this.stopped) {
      // stop() ran while SUBSCRIBE was in flight: nothing is registered
      // yet, so register nothing; the instance lapses with the session.
      return;
    }
    const alias = this.session.getTrackAlias(subId);
    if (alias === undefined) throw new Error('no track alias after SUBSCRIBE_OK');
    this.log(`subscribed, id=${subId} alias=${alias}`);

    // The instance ends when the server says so (PUBLISH_DONE) or we stop.
    const done = new Promise<void>((resolve) => {
      this.endInstance = resolve;
      this.session.onPublishDone(subId, (status) => {
        this.log(`instance ended, status=${status}`);
        resolve();
      });
    });

    this.currentAlias = alias;
    this.router.register(alias, (h, o) => this.onObject(h, o));
    try {
      await done;
    } finally {
      this.router.unregister(alias);
      this.currentAlias = null;
      this.endInstance = null;
      this.session.removePublishDoneHandler(subId);
      // Applies in flight belong to the dead instance — flush, then reset.
      await this.chain;
      await this.applySerialized(async () => this.store.terminated());
    }
  }

  private onObject(h: SubgroupHeader, o: SubgroupObject): void {
    if (o.payload.length === 0) return; // status-only object
    this.applySerialized(async () => {
      const msg = parseStateMessage(o.payload, 'downstream');
      if (h.subgroupId === CATCH_UP_LANE) {
        await this.store.applyCatchUp(msg);
      } else {
        await this.store.applyLive(msg);
      }
    });
  }

  private applySerialized(fn: () => Promise<void>): Promise<void> {
    this.chain = this.chain.then(fn).catch((e) => this.log('apply error:', e));
    return this.chain;
  }
}
