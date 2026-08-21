/**
 * Main-side worker proxy: request/reply correlation, event fan-out,
 * one-shot PUBLISH_DONE dispatch, and the StateSession adapter —
 * driven through a fake worker port (no real Worker).
 */

import { describe, it, expect } from 'vitest';
import { WorkerTransport, WorkerSessionProxy, type WorkerPortLike } from '../src/worker/worker-transport.js';
import type { MainToWorker, WorkerToMain } from '../src/worker/protocol.js';

class FakePort implements WorkerPortLike {
  sent: MainToWorker[] = [];
  onmessage: ((e: MessageEvent) => void) | null = null;
  terminated = false;

  postMessage(m: unknown): void {
    this.sent.push(m as MainToWorker);
  }
  terminate(): void {
    this.terminated = true;
  }
  /** Deliver a worker→main message. */
  emit(m: WorkerToMain): void {
    this.onmessage!({ data: m } as MessageEvent);
  }
}

function rig(): { port: FakePort; transport: WorkerTransport } {
  const port = new FakePort();
  return { port, transport: new WorkerTransport(port) };
}

describe('WorkerTransport', () => {
  it('correlates subscribe request and reply by id', async () => {
    const { port, transport } = rig();
    const p = transport.subscribe(['main', 'entity', 'e1', 'sink'], 'binaural');
    const cmd = port.sent[0]!;
    expect(cmd.type).toBe('subscribe');
    const id = (cmd as Extract<MainToWorker, { type: 'subscribe' }>).id;
    port.emit({ type: 'subscribed', id, subscribeId: 7, trackAlias: 3 });
    await expect(p).resolves.toEqual({ subscribeId: 7, trackAlias: 3 });
  });

  it('rejects a request on fail reply, leaving other requests pending', async () => {
    const { port, transport } = rig();
    const a = transport.subscribe(['ns'], 't1');
    const b = transport.subscribe(['ns'], 't2');
    const idA = (port.sent[0] as Extract<MainToWorker, { type: 'subscribe' }>).id;
    const idB = (port.sent[1] as Extract<MainToWorker, { type: 'subscribe' }>).id;
    port.emit({ type: 'fail', id: idA, message: 'boom' });
    await expect(a).rejects.toThrow('boom');
    port.emit({ type: 'subscribed', id: idB, subscribeId: 2, trackAlias: 9 });
    await expect(b).resolves.toEqual({ subscribeId: 2, trackAlias: 9 });
  });

  it('fans events out and dispatches publishDone one-shot by subscribe id', () => {
    const { port, transport } = rig();
    const seen: string[] = [];
    transport.setEvents({
      onIncomingSubscribe: (e) => seen.push(`incoming:${e.trackAlias}`),
      onDatagram: (e) => seen.push(`dg:${e.trackAlias}`),
      onSubgroupObject: (e) => seen.push(`sg:${e.trackAlias}:${e.subgroupId}`),
      onNotice: (e) => seen.push(`notice:${e.event}`),
    });
    let done = 0;
    transport.onPublishDone(5, (status) => {
      done = status;
    });

    port.emit({ type: 'incomingSubscribe', namespace: ['s', 'entity', 'e', 'source'], trackAlias: 1 });
    port.emit({ type: 'datagram', trackAlias: 2, groupId: 10n, objectId: 0n, payload: new Uint8Array(3) });
    port.emit({ type: 'subgroupObject', trackAlias: 4, groupId: 0n, subgroupId: 1n, objectId: 0n, payload: new Uint8Array(1) });
    port.emit({ type: 'notice', event: 'datagram-reader-done' });
    port.emit({ type: 'publishDone', subscribeId: 5, statusCode: 6, reason: '' });
    // One-shot: a second publishDone for the same id is dropped.
    port.emit({ type: 'publishDone', subscribeId: 5, statusCode: 99, reason: '' });

    expect(seen).toEqual(['incoming:1', 'dg:2', 'sg:4:1', 'notice:datagram-reader-done']);
    expect(done).toBe(6);
  });

  it('sendDatagram is fire-and-forget', () => {
    const { port, transport } = rig();
    transport.sendDatagram(new Uint8Array([1, 2, 3]));
    expect(port.sent).toHaveLength(1);
    expect(port.sent[0]!.type).toBe('sendDatagram');
  });

  it('close resolves the reply, rejects stragglers, and terminates the worker', async () => {
    const { port, transport } = rig();
    const straggler = transport.subscribe(['ns'], 't');
    const closing = transport.close();
    const closeCmd = port.sent.find((m) => m.type === 'close') as Extract<MainToWorker, { type: 'close' }>;
    port.emit({ type: 'closed', id: closeCmd.id });
    await closing;
    await expect(straggler).rejects.toThrow('worker closed');
    expect(port.terminated).toBe(true);
  });
});

describe('WorkerSessionProxy', () => {
  it('presents the StateSession surface over the RPC', async () => {
    const { port, transport } = rig();
    const session = new WorkerSessionProxy(transport);
    const p = session.subscribe(['s', 'client', 'c', 'sink'], 'state', undefined, undefined, [
      { type: 0x1a5a3, value: new Uint8Array([1]) },
    ]);
    const cmd = port.sent[0] as Extract<MainToWorker, { type: 'subscribe' }>;
    expect(cmd.extraParams).toHaveLength(1);
    port.emit({ type: 'subscribed', id: cmd.id, subscribeId: 11, trackAlias: 8 });
    await expect(p).resolves.toBe(11);
    expect(session.getTrackAlias(11)).toBe(8);

    let status = 0;
    session.onPublishDone(11, (s) => {
      status = s;
    });
    port.emit({ type: 'publishDone', subscribeId: 11, statusCode: 2, reason: '' });
    expect(status).toBe(2);
  });
});

describe('LASA rejection surfacing', () => {
  it('rejects connect with a typed LasaRejectionError when the fail carries a §4.6 close code', async () => {
    const { port, transport } = rig();
    const p = transport.connect('https://x/lasa', { clientId: 'c1' }, undefined, false);
    const id = (port.sent[0] as Extract<MainToWorker, { type: 'connect' }>).id;
    port.emit({
      type: 'fail',
      id,
      message: 'stream closed',
      closeCode: 0x1a5a08,
      closeReason: 'this space is unticketed; do not present a ticket',
    });
    await expect(p).rejects.toMatchObject({
      name: 'LasaRejectionError',
      code: 0x1a5a08,
      reason: 'this space is unticketed; do not present a ticket',
    });
  });

  it('falls back to a plain Error when the close code is not a LASA code', async () => {
    const { port, transport } = rig();
    const p = transport.connect('https://x/lasa', { clientId: 'c1' }, undefined, false);
    const id = (port.sent[0] as Extract<MainToWorker, { type: 'connect' }>).id;
    port.emit({ type: 'fail', id, message: 'network sadness', closeCode: 0 });
    await expect(p).rejects.toThrow('network sadness');
  });

  it('fans transportClosed out to the event handler', () => {
    const { port, transport } = rig();
    const seen: Array<{ closeCode?: number; reason?: string }> = [];
    transport.setEvents({ onTransportClosed: (e) => seen.push({ closeCode: e.closeCode, reason: e.reason }) });
    port.emit({ type: 'transportClosed', closeCode: 0x1a5a06, reason: 'blocked until 1799999999' });
    expect(seen).toEqual([{ closeCode: 0x1a5a06, reason: 'blocked until 1799999999' }]);
  });
});
