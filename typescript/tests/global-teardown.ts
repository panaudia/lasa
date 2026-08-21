import { readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';

export default async function globalTeardown(): Promise<void> {
  const dir = process.env['LASA_SPIKE_TMP'];
  if (!dir) return;
  for (const pidFile of ['server.pid', 'server2.pid']) {
    try {
      const pid = Number(readFileSync(join(dir, pidFile), 'utf8'));
      process.kill(pid, 'SIGTERM');
    } catch {
      // already gone
    }
  }
  rmSync(dir, { recursive: true, force: true });
}
