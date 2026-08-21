import { defineConfig } from 'vite';

// Spike/dev server + vitest config; library build arrives with the
// packaging pass. *.spec.ts files are Playwright's, *.test.ts vitest's.
// Cross-origin isolation: the capture/playout SAB rings require
// crossOriginIsolated === true. COOP/COEP is the mechanism every target
// engine supports (Document-Isolation-Policy is Chrome-137-desktop-only
// as of 2026-08); the SDK itself gates on the effect, not the mechanism.
const isolationHeaders = {
  'Cross-Origin-Opener-Policy': 'same-origin',
  'Cross-Origin-Embedder-Policy': 'require-corp',
};

export default defineConfig({
  // host pinned to 127.0.0.1: plain `localhost` resolves to ::1 on macOS,
  // and Playwright's webServer probe (baseURL) targets 127.0.0.1.
  server: { host: '127.0.0.1', port: 5199, strictPort: true, headers: isolationHeaders },
  preview: { headers: isolationHeaders },
  test: {
    include: ['tests/**/*.test.ts'],
  },
} as Parameters<typeof defineConfig>[0]);
