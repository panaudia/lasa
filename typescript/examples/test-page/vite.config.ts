import { defineConfig } from 'vite';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { existsSync, readFileSync } from 'node:fs';

const here = dirname(fileURLToPath(import.meta.url));

// HTTPS for the dev.panaudia.com route: a non-localhost HTTP origin is
// not a secure context (no WebTransport, no cross-origin isolation), so
// browsing via the /etc/hosts alias needs the page served over TLS.
// Uses the same wildcard cert the server uses; auto-enabled when found.
// 127.0.0.1 over plain HTTP keeps working either way (localhost is a
// secure context by exemption) — but with https on, use the hostname.
const keysDir = process.env['PANAUDIA_DEV_KEYS_DIR'] ?? resolve(here, '../../../../keys');
const crt = resolve(keysDir, 'server.crt');
const key = resolve(keysDir, 'server.key');
const https = existsSync(crt) && existsSync(key)
  ? { cert: readFileSync(crt), key: readFileSync(key) }
  : undefined;

// The SDK is consumed straight from source: this page is the SDK's
// own instrument and tracks the tree it sits in. The alias keeps app
// imports on the package name (examples/minimal shows the dependency
// route a real consumer takes).
const sdkSrc = resolve(here, '../../src');

// Capture/playout ride SharedArrayBuffer rings, which require
// cross-origin isolation. The SDK gates on the effect
// (crossOriginIsolated), COOP/COEP is the mechanism.
const isolationHeaders = {
  'Cross-Origin-Opener-Policy': 'same-origin',
  'Cross-Origin-Embedder-Policy': 'require-corp',
};

export default defineConfig({
  server: {
    host: '127.0.0.1',
    port: 5300,
    strictPort: true,
    headers: isolationHeaders,
    // The SDK source (incl. its module worker) lives outside this
    // package root; allow Vite dev to serve it.
    fs: { allow: [here, sdkSrc] },
    // Browsing via dev.panaudia.com (→127.0.0.1 in /etc/hosts — the
    // wildcard-cert route in the README) needs the host allowlisted.
    allowedHosts: ['dev.panaudia.com'],
    https,
  },
  preview: { headers: isolationHeaders },
  resolve: {
    alias: {
      '@panaudia/lasa-client': resolve(sdkSrc, 'index.ts'),
    },
  },
});
