/**
 * Golden-vector tests for the state-plane codecs: byte-exact against
 * the Go `lasa/wire` (vectors generated from it — see
 * tests/vectors/lasa-wire.json).
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  encodeStateMessage,
  encodeSubscribeParams,
  parseStateMessage,
  setHash,
  type StateMessage,
} from '../src/lasa/statewire';

const here = dirname(fileURLToPath(import.meta.url));
interface Vec {
  name: string;
  hex: string;
  meta: unknown;
}
const vectors: Vec[] = JSON.parse(readFileSync(join(here, 'vectors/lasa-wire.json'), 'utf8'));
const byName = new Map(vectors.map((v) => [v.name, v]));
function bytes(name: string): Uint8Array {
  const v = byName.get(name);
  if (!v) throw new Error(`missing vector ${name}`);
  const out = new Uint8Array(v.hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(v.hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}
function hexOf(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
}

const KEY = 'lasa.entity.e1.mute';
const VALUE = new TextEncoder().encode('{"muted":true}');
const EPOCH = new Uint8Array([0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04]);

describe('state messages', () => {
  const cases: [string, StateMessage, 'upstream' | 'downstream'][] = [
    ['state-set-up', { kind: 'set', key: KEY, value: VALUE, seq: 0n }, 'upstream'],
    ['state-set-down', { kind: 'set', key: KEY, value: VALUE, seq: 4242n }, 'downstream'],
    ['state-clear-up', { kind: 'clear', key: KEY, seq: 0n }, 'upstream'],
    ['state-clear-down', { kind: 'clear', key: KEY, seq: 4243n }, 'downstream'],
    [
      'state-group-down',
      {
        kind: 'group',
        ops: [
          { kind: 'set', key: KEY, value: VALUE, seq: 4242n },
          { kind: 'clear', key: KEY, seq: 4243n },
        ],
      },
      'downstream',
    ],
    ['state-frontier', { kind: 'frontier', epoch: EPOCH, seq: 99999n }, 'downstream'],
    ['state-begin-delta', { kind: 'begin', begin: 'delta' }, 'downstream'],
    ['state-begin-snapshot', { kind: 'begin', begin: 'snapshot' }, 'downstream'],
  ];

  it.each(cases)('%s encodes byte-exactly', (name, msg, dir) => {
    expect(hexOf(encodeStateMessage(msg, dir))).toBe(byName.get(name)!.hex);
  });

  it.each(cases)('%s round-trips through parse', (name, msg, dir) => {
    expect(parseStateMessage(bytes(name), dir)).toEqual(msg);
  });

  it('rejects trailing bytes (one message per object)', () => {
    const b = bytes('state-begin-delta');
    const padded = new Uint8Array(b.length + 1);
    padded.set(b);
    expect(() => parseStateMessage(padded, 'downstream')).toThrowError(/trailing/);
  });

  it('rejects downstream-only messages upstream', () => {
    expect(() => parseStateMessage(bytes('state-frontier'), 'upstream')).toThrowError(/downstream-only/);
    expect(() => parseStateMessage(bytes('state-begin-delta'), 'upstream')).toThrowError(/downstream-only/);
  });

  it('parses per direction: the same set bytes differ', () => {
    // Downstream bytes carry a trailing seq the upstream layout lacks.
    expect(byName.get('state-set-down')!.hex).not.toBe(byName.get('state-set-up')!.hex);
  });
});

describe('subscribe params', () => {
  // Same deliberately unsorted, duplicated input as the generator.
  const prefixes = ['lasa.space.', 'lasa.entity.', 'lasa.space.'];

  it('encodes the fresh (cursorless) form canonically', () => {
    expect(hexOf(encodeSubscribeParams(prefixes))).toBe(byName.get('subparams-fresh')!.hex);
  });

  it('encodes the cursor form, and setHash matches Go', async () => {
    const meta = byName.get('subparams-cursor')!.meta as { sethash: string };
    const hash = await setHash(prefixes);
    expect(hexOf(hash)).toBe(meta.sethash);
    expect(hexOf(encodeSubscribeParams(prefixes, { epoch: EPOCH, seq: 4242n, setHash: hash }))).toBe(
      byName.get('subparams-cursor')!.hex
    );
  });

  it('rejects an empty prefix set', () => {
    expect(() => encodeSubscribeParams([])).toThrowError(/empty prefix set/);
  });
});

// E2 (spec-audit 2026-08-21): the TS codec holds keys to the §2
// grammar and 512-byte cap on both encode and parse, errors on
// truncation instead of clamping, and validates prefixes so the
// canonical sort is bytewise by construction.
import { canonicalPrefixes, MAX_KEY_LEN } from '../src/lasa/statewire';
import { MalformedError } from '../src/lasa/wire';

describe('key and prefix validation', () => {
  const enc = (m: StateMessage) => encodeStateMessage(m, 'upstream');

  it('rejects invalid keys on encode', () => {
    expect(() => enc({ kind: 'set', key: 'Bad Key', value: new Uint8Array(), seq: 0n })).toThrow();
    expect(() => enc({ kind: 'clear', key: '', seq: 0n })).toThrow();
    expect(() => enc({ kind: 'set', key: 'k'.repeat(MAX_KEY_LEN + 1), value: new Uint8Array(), seq: 0n })).toThrow();
  });

  it('rejects over-cap and truncated keys on parse', () => {
    // set, key-len varint claiming 513 bytes
    const long = new Uint8Array([1, 0x42, 0x01, ...new Array(513).fill(0x61), 0]);
    expect(() => parseStateMessage(long, 'upstream')).toThrow(/key too long/);
    // clear, key-len 5 but only 2 key bytes present
    const truncated = new Uint8Array([2, 5, 0x61, 0x62]);
    expect(() => parseStateMessage(truncated, 'upstream')).toThrow(/truncated key/);
  });

  it('rejects keys outside the grammar on parse', () => {
    // clear with key "AB"
    const bad = new Uint8Array([2, 2, 0x41, 0x42]);
    expect(() => parseStateMessage(bad, 'upstream')).toThrow(/invalid key/);
  });

  it('validates prefixes so sort() is bytewise', () => {
    expect(() => canonicalPrefixes(['lasa.', 'é'])).toThrow(/invalid prefix/);
    expect(canonicalPrefixes(['lasa.space.', 'lasa.entity.', 'lasa.entity.'])).toEqual([
      'lasa.entity.',
      'lasa.space.',
    ]);
    expect(canonicalPrefixes([''])).toEqual(['']); // empty prefix stays legal
  });
});

// B10: the upstream layout never carries seq, so a client write must not
// have to invent one; downstream encoding without it is a caller error.
describe('upstream ops without seq', () => {
  it('encodes a seq-less set identically to one with a dummy seq', () => {
    const key = 'lasa.x';
    const value = new Uint8Array([1, 2, 3]);
    const a = encodeStateMessage({ kind: 'set', key, value }, 'upstream');
    const b = encodeStateMessage({ kind: 'set', key, value, seq: 0n }, 'upstream');
    expect(a).toEqual(b);
    const c = encodeStateMessage({ kind: 'group', ops: [{ kind: 'clear', key }] }, 'upstream');
    expect(c[0]).toBe(3);
  });

  it('refuses to encode a seq-less op downstream', () => {
    expect(() => encodeStateMessage({ kind: 'clear', key: 'lasa.x' }, 'downstream')).toThrow(MalformedError);
  });
});
