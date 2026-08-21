/**
 * Generate Ed25519 key pair for signing LASA tickets.
 * Run with: npx tsx generate_keys.mts
 */

import * as crypto from 'node:crypto';
import * as fs from 'node:fs';

const { privateKey, publicKey } = crypto.generateKeyPairSync('ed25519');

fs.writeFileSync('private.pem', privateKey.export({ type: 'pkcs8', format: 'pem' }));
fs.writeFileSync('public.pem', publicKey.export({ type: 'spki', format: 'pem' }));

console.log('Keys written to private.pem and public.pem');
