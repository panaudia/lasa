// SUBSCRIBE_ERROR codes are a separate table from the session close codes
// (panaudia/moqtransport errors.go: ErrorCodeSubscribe* vs ErrorCode). The
// client once labelled subscribe refusals with the session table, so an
// Unauthorized (0x01) read as "Internal error" and a Timeout (0x02) was
// thrown as an AuthenticationError. Pinned against the fork's values.
import { describe, it, expect } from 'vitest';
import { getMoqErrorMessage, getSubscribeErrorMessage, isSubscribeAuthError } from '../src/transport/errors.js';

describe('MOQ error code tables', () => {
  it('labels SUBSCRIBE_ERROR codes with the subscribe table', () => {
    expect(getSubscribeErrorMessage(0x00)).toBe('Internal error');
    expect(getSubscribeErrorMessage(0x01)).toBe('Unauthorized');
    expect(getSubscribeErrorMessage(0x02)).toBe('Timeout');
    expect(getSubscribeErrorMessage(0x03)).toBe('Not supported');
    expect(getSubscribeErrorMessage(0x04)).toBe('Track does not exist');
    expect(getSubscribeErrorMessage(0x10)).toBe('Malformed auth token');
    expect(getSubscribeErrorMessage(0x12)).toBe('Expired auth token');
    expect(getSubscribeErrorMessage(0x99)).toMatch(/Unknown subscribe error/);
  });

  it('classifies only the auth-flavoured subscribe codes as authentication failures', () => {
    for (const c of [0x01, 0x10, 0x12, 0x403]) expect(isSubscribeAuthError(c)).toBe(true);
    for (const c of [0x00, 0x02, 0x03, 0x04, 0x05]) expect(isSubscribeAuthError(c)).toBe(false);
  });

  it('labels session close codes with the session table', () => {
    expect(getMoqErrorMessage(0x02)).toBe('Unauthorized');
    expect(getMoqErrorMessage(0x04)).toBe('Invalid request ID');
    expect(getMoqErrorMessage(0x05)).toBe('Duplicate track alias');
    expect(getMoqErrorMessage(0x15)).toBe('Version negotiation failed');
  });
});
