/**
 * The lockstep guard (plan/lockstep.md, TypeScript deferral): the
 * client carries the declaration but cannot honour it for more than
 * one entity, so connect refuses that shape before a worker exists.
 */
import { describe, expect, it } from 'vitest';
import { assertLockstepSupported, connectionConfigParam } from '../src/lasa/setup.js';

const one = [{ id: 'a', name: 'A' }];
const two = [{ id: 'a', name: 'A' }, { id: 'b', name: 'B' }];

describe('lockstep guard', () => {
  it('passes without the flag, and with the flag on zero or one entity', () => {
    expect(() => assertLockstepSupported({ clientId: 'c', entities: two })).not.toThrow();
    expect(() => assertLockstepSupported({ clientId: 'c', entities: one, lockstep: true })).not.toThrow();
    expect(() => assertLockstepSupported({ clientId: 'c', entities: [], lockstep: true })).not.toThrow();
  });

  it('refuses the flag with more than one entity, by entities or setups', () => {
    expect(() => assertLockstepSupported({ clientId: 'c', entities: two, lockstep: true })).toThrow(/lockstep with 2 entities/);
    expect(() =>
      assertLockstepSupported({ clientId: 'c', ticket: 't', setups: [{ id: 'a' }, { id: 'b' }], lockstep: true })
    ).toThrow(/not supported/);
  });

  it('serialises the flag only when set', () => {
    const on = new TextDecoder().decode(connectionConfigParam({ clientId: 'c', entities: one, lockstep: true }).value as Uint8Array);
    const off = new TextDecoder().decode(connectionConfigParam({ clientId: 'c', entities: one }).value as Uint8Array);
    expect(JSON.parse(on).lockstep).toBe(true);
    expect('lockstep' in JSON.parse(off)).toBe(false);
  });
});
