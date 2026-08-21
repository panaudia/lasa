/**
 * Builds and starts panaudia-server (the sibling Go repo) for the
 * interop rig — TWO instances, one per space mode (ticketed XOR
 * unticketed, lasa-core.md §4.4):
 *
 *   44443 — unticketed dev mode (the original interop rig)
 *   44444 — ticketed, verifying against a keypair generated here; the
 *            private half rides process.env so specs can mint tickets
 *            (tests/mint-ticket.ts)
 *
 * Machine-local sibling-path coupling, as go.mod replace directives
 * are elsewhere in this workspace — acceptable for the interop/test rig.
 */
import { spawn, execSync, type ChildProcess } from 'node:child_process';
import { generateKeyPairSync } from 'node:crypto';
import { existsSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const serverRepo = resolve(here, '../../../panaudia-lasa/server');
// The real *.panaudia.com wildcard cert (dev.panaudia.com → 127.0.0.1
// in /etc/hosts — the old SDK harness's pattern). When present, the rig
// runs the exact production TLS shape: no flags, no cert hashes.
const keysDir = resolve(here, '../../../keys');

interface BootedServer {
  child: ChildProcess;
  certHash?: string;
}

function bootServer(bin: string, env: NodeJS.ProcessEnv, label: string): Promise<BootedServer> {
  const child = spawn(bin, [], { env, stdio: ['ignore', 'pipe', 'pipe'] });
  return new Promise<BootedServer>((resolveReady, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`panaudia-server (${label}) did not boot in time`)),
      15_000
    );
    let seen = '';
    let certHash: string | undefined;
    child.stderr!.on('data', (buf: Buffer) => {
      process.stderr.write(buf);
      seen += buf.toString();
      const m = seen.match(/serverCertificateHashes \(sha-256, base64\): (\S+)/);
      if (m) {
        certHash = m[1];
      }
      if (seen.includes('panaudia-server: WebTransport at')) {
        clearTimeout(timer);
        resolveReady({ child, certHash });
      }
    });
    child.stdout!.on('data', (buf: Buffer) => process.stdout.write(buf));
    child.on('exit', (code) => reject(new Error(`server (${label}) exited early: ${code}`)));
  });
}

export default async function globalSetup(): Promise<void> {
  const dir = mkdtempSync(join(tmpdir(), 'lasa-interop-'));
  const bin = join(dir, 'panaudia-server');
  execSync(`go build -o ${JSON.stringify(bin)} .`, { cwd: serverRepo, stdio: 'inherit' });

  // The ticketed space's issuer keypair: the server verifies with the
  // raw public key; specs mint with the private PEM.
  const { publicKey, privateKey } = generateKeyPairSync('ed25519');
  const rawPub = Buffer.from(publicKey.export({ format: 'jwk' }).x!, 'base64url');
  process.env['LASA_TICKET_PRIVKEY_PEM'] = privateKey
    .export({ type: 'pkcs8', format: 'pem' })
    .toString();

  const baseEnv: NodeJS.ProcessEnv = {
    ...process.env,
    PANAUDIA_HOST: '127.0.0.1',
    PANAUDIA_REVERB: 'none',
  };
  const crt = join(keysDir, 'server.crt');
  const key = join(keysDir, 'server.key');
  const realCert = existsSync(crt) && existsSync(key);
  if (realCert) {
    baseEnv['PANAUDIA_CERT'] = crt;
    baseEnv['PANAUDIA_KEY'] = key;
    process.env['LASA_SPIKE_URL'] = 'https://dev.panaudia.com:44443/lasa';
    process.env['LASA_TICKETED_URL'] = 'https://dev.panaudia.com:44444/lasa';
  }

  const open = await bootServer(
    bin,
    {
      ...baseEnv,
      PANAUDIA_PORT: '44443',
      PANAUDIA_ALLOW_UNTICKETED: 'true', // open space is an explicit opt-in
    },
    'open'
  );
  const ticketed = await bootServer(
    bin,
    {
      ...baseEnv,
      PANAUDIA_PORT: '44444',
      PANAUDIA_TICKET_KEY: rawPub.toString('base64'), // ticketed space: XOR of open
    },
    'ticketed'
  );

  writeFileSync(join(dir, 'server.pid'), String(open.child.pid));
  writeFileSync(join(dir, 'server2.pid'), String(ticketed.child.pid));
  process.env['LASA_SPIKE_TMP'] = dir;
  process.env['LASA_SPIKE_URL'] ??= 'https://127.0.0.1:44443/lasa';
  process.env['LASA_TICKETED_URL'] ??= 'https://127.0.0.1:44444/lasa';
  if (open.certHash) process.env['LASA_SPIKE_CERT_HASH'] = open.certHash;
  if (ticketed.certHash) process.env['LASA_TICKETED_CERT_HASH'] = ticketed.certHash;

  open.child.unref();
  ticketed.child.unref();
}
