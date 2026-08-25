import { defineConfig } from 'vite';
import dts from 'vite-plugin-dts';
import { resolve } from 'node:path';

// One config serves three jobs: the library build (`vite build`), the dev
// server behind Playwright (`vite`, serving interop/), and vitest.
// *.spec.ts files are Playwright's, *.test.ts vitest's.

// Cross-origin isolation: the capture/playout SAB rings require
// crossOriginIsolated === true. COOP/COEP is the mechanism every target
// engine supports (Document-Isolation-Policy is Chrome-137-desktop-only
// as of 2026-08); the SDK itself gates on the effect, not the mechanism.
const isolationHeaders = {
  'Cross-Origin-Opener-Policy': 'same-origin',
  'Cross-Origin-Embedder-Policy': 'require-corp',
};

export default defineConfig({
  plugins: [
    // Per-module .d.ts under dist/, mirroring src/; package.json points
    // `types` at dist/index.d.ts. Only src/ is declared: interop/ and
    // tests/ are in tsconfig for typechecking, not for shipping.
    dts({ include: ['src'], exclude: ['src/**/*.test.ts'] }),
  ],
  build: {
    // es2022 keeps native class fields (no `__publicField` helper). The
    // worklets are shipped as source text (`.toString()` of real classes
    // built into a Blob module), so that text must be self-contained.
    // Safe: the library already requires WebCodecs, WebTransport and
    // AudioWorklet, all newer than es2022.
    target: 'es2022',
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: () => 'index.js',
    },
    rollupOptions: {
      output: {
        // Stable chunk names (no hashes) so import maps can reference them.
        chunkFileNames: '[name].js',
      },
    },
    // No source maps: the `?worker&inline` Blob worker bakes a relative
    // `//# sourceMappingURL=` that resolves to `blob://null` and Safari
    // blocks it (Vite drives the worker map from build.sourcemap, with no
    // per-worker override). minify is off, so the bundle reads without maps.
    sourcemap: false,
    minify: false,
  },
  // The inlined module worker must be emitted as ES: Vite's default worker
  // format is 'iife', incompatible with `type: 'module'`.
  worker: { format: 'es' },
  // host pinned to 127.0.0.1: plain `localhost` resolves to ::1 on macOS,
  // and Playwright's webServer probe (baseURL) targets 127.0.0.1.
  server: { host: '127.0.0.1', port: 5199, strictPort: true, headers: isolationHeaders },
  preview: { headers: isolationHeaders },
  test: {
    include: ['tests/**/*.test.ts'],
  },
} as Parameters<typeof defineConfig>[0]);
