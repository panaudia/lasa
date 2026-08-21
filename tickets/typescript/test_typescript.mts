/**
 * Verify TypeScript/Node.js LASA ticket creation and signing.
 * Run with: npx tsx test_typescript.mts
 */

import * as crypto from 'node:crypto';
import { SignJWT, importPKCS8, importSPKI, jwtVerify, decodeProtectedHeader, decodeJwt } from 'jose';

// --- Step 1: Generate keys ---

const { privateKey, publicKey } = crypto.generateKeyPairSync('ed25519');

const privatePem = privateKey.export({ type: 'pkcs8', format: 'pem' }) as string;
const publicPem = publicKey.export({ type: 'spki', format: 'pem' }) as string;

console.log('Keys generated OK');

// --- Step 2: Create and sign ticket ---

const josePrivateKey = await importPKCS8(privatePem, 'EdDSA');

const spaceId = 'df8c7a85-0702-45e9-a626-a1c147eafce9';
const clientId = '6f1c2d3e-8a4b-4c5d-9e0f-1a2b3c4d5e6f';
const entityId = 'a1b2c3d4-0000-0000-0000-000000000001';
const now = Math.floor(Date.now() / 1000);

const ticket = await new SignJWT({
  iss: 'test-verify',
  aud: spaceId,
  panaudia: {
    client_id: clientId,
    roles: ['presenter'],
    permissions: {
      set: ['myapp.'],
      clear: ['myapp.'],
      read: ['myapp.'],
    },
    entities: [
      {
        id: entityId,
        name: 'TestUser',
        priority: true,
        'heard-in': ['main'],
        hears: ['main'],
        attrs: { colour: '00aaff' },
        render: { gain: 1.5, attenuation: 2.0 },
      },
    ],
  },
})
  .setProtectedHeader({ typ: 'JWT', alg: 'EdDSA', crv: 'Ed25519' })
  .setIssuedAt(now)
  .setExpirationTime(now + 3600)
  .sign(josePrivateKey);

console.log(`Ticket created OK (${ticket.length} chars)`);

// --- Step 3: Verify ---

const parts = ticket.split('.');
if (parts.length !== 3) throw new Error(`Expected 3 parts, got ${parts.length}`);

// Decode header
const header = decodeProtectedHeader(ticket);
if (header.typ !== 'JWT') throw new Error(`typ: ${header.typ}`);
if (header.alg !== 'EdDSA') throw new Error(`alg: ${header.alg}`);
if (header.crv !== 'Ed25519') throw new Error(`crv: ${header.crv}`);
console.log(`Header OK: ${JSON.stringify(header)}`);

// Decode payload
const payload = decodeJwt(ticket);
if (payload.iss !== 'test-verify') throw new Error(`iss: ${payload.iss}`);
if (payload.aud !== spaceId) throw new Error(`aud must be a plain string equal to the space id: ${payload.aud}`);
if (payload.iat !== now) throw new Error(`iat: ${payload.iat}`);
if (payload.exp !== now + 3600) throw new Error(`exp: ${payload.exp}`);

const panaudia = (payload as any).panaudia;
if (panaudia.client_id !== clientId) throw new Error(`client_id: ${panaudia.client_id}`);
if (panaudia.roles[0] !== 'presenter') throw new Error(`roles: ${panaudia.roles}`);
if (panaudia.permissions.set[0] !== 'myapp.') throw new Error(`permissions.set: ${panaudia.permissions.set}`);
if (panaudia.permissions.clear[0] !== 'myapp.') throw new Error(`permissions.clear: ${panaudia.permissions.clear}`);
if (panaudia.permissions.read[0] !== 'myapp.') throw new Error(`permissions.read: ${panaudia.permissions.read}`);
const entity = panaudia.entities[0];
if (entity.id !== entityId) throw new Error(`entity.id: ${entity.id}`);
if (entity.name !== 'TestUser') throw new Error(`entity.name: ${entity.name}`);
if (entity.priority !== true) throw new Error(`entity.priority: ${entity.priority}`);
if (entity['heard-in'][0] !== 'main') throw new Error(`entity.heard-in: ${entity['heard-in']}`);
if (entity.hears[0] !== 'main') throw new Error(`entity.hears: ${entity.hears}`);
if (entity.attrs.colour !== '00aaff') throw new Error(`entity.attrs.colour: ${entity.attrs.colour}`);
if (entity.render.gain !== 1.5) throw new Error(`entity.render.gain: ${entity.render.gain}`);
if (entity.render.attenuation !== 2.0) throw new Error(`entity.render.attenuation: ${entity.render.attenuation}`);
console.log('Payload OK: all claims verified');

// Verify signature with public key
const josePublicKey = await importSPKI(publicPem, 'EdDSA');
await jwtVerify(ticket, josePublicKey, { audience: spaceId });
console.log('Signature verified OK with public key');

console.log('\n=== TYPESCRIPT: ALL TESTS PASSED ===');
