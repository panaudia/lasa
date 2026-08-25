import { defineConfig } from 'vite';

// Capture and playout hand audio between threads through
// SharedArrayBuffer rings, which the browser allows only on a
// cross-origin-isolated page. These two headers are the whole
// requirement; your production server must send them too.
const isolationHeaders = {
  'Cross-Origin-Opener-Policy': 'same-origin',
  'Cross-Origin-Embedder-Policy': 'require-corp',
};

export default defineConfig({
  server: { host: '127.0.0.1', port: 5301, strictPort: true, headers: isolationHeaders },
  preview: { headers: isolationHeaders },
});
