/**
 * Node-side LASA ticket minting for the interop rig (no dependencies —
 * Ed25519 JWS via node:crypto). Mirrors tickets/typescript but
 * inlined so the test rig stays self-contained.
 */

import { createPrivateKey, sign, type KeyObject } from 'node:crypto';

function b64url(buf: Buffer): string {
  return buf.toString('base64url');
}

export interface MintOptions {
  spaceId: string;
  clientId: string;
  roles?: string[];
  entities?: Array<Record<string, unknown>>;
  /** Seconds from now; default 3600. */
  ttlSec?: number;
}

/** Mints a signed LASA ticket (lasa-core.md §4.3) with an Ed25519 key. */
export function mintTicket(privateKeyPem: string, opts: MintOptions): string {
  const key: KeyObject = createPrivateKey(privateKeyPem);
  const now = Math.floor(Date.now() / 1000);
  const header = { typ: 'JWT', alg: 'EdDSA', crv: 'Ed25519' };
  const panaudia: Record<string, unknown> = { client_id: opts.clientId };
  if (opts.roles !== undefined) panaudia['roles'] = opts.roles;
  if (opts.entities !== undefined) panaudia['entities'] = opts.entities;
  const payload = {
    iss: 'interop-rig',
    iat: now,
    exp: now + (opts.ttlSec ?? 3600),
    aud: opts.spaceId,
    panaudia,
  };
  const signingInput = `${b64url(Buffer.from(JSON.stringify(header)))}.${b64url(Buffer.from(JSON.stringify(payload)))}`;
  const sig = sign(null, Buffer.from(signingInput), key);
  return `${signingInput}.${b64url(sig)}`;
}
