import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: 'tests',
  testMatch: '**/*.spec.ts', // *.test.ts files are vitest's
  workers: 1, // the specs share one panaudia-server instance
  timeout: 60_000,
  globalSetup: './tests/global-setup.ts',
  globalTeardown: './tests/global-teardown.ts',
  use: {
    baseURL: 'http://127.0.0.1:5199',
    launchOptions: {
      // With the real wildcard cert (dev.panaudia.com in /etc/hosts) no
      // flags are needed — the rig runs the production TLS shape. The
      // dev-mode flag remains only for the no-keys fallback, where the
      // spike passes serverCertificateHashes instead.
      args: [
        '--webtransport-developer-mode',
        // The playout spec renders real audio through an AudioContext;
        // headless has no user gesture, so lift the autoplay gate.
        '--autoplay-policy=no-user-gesture-required',
      ],
    },
  },
  webServer: {
    command: 'npm run dev',
    url: 'http://127.0.0.1:5199/spike/',
    reuseExistingServer: true,
    timeout: 30_000,
  },
});
