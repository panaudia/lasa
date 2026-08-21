/**
 * jitter-v4-replay.test.ts — fixture-equality replay of the ported v4 core
 * (plan/jitter-v4-ts-port.md phase 3).
 *
 * The Go jitterlab exported canonical traces (panaudia-engine
 * jitterlab/export.go, `TestExportTraces`; regenerate with
 * JITTERLAB_EXPORT_DIR pointing at tests/fixtures/jitter-v4/): each carries
 * a scenario's full event schedule plus the artifact ledger + summary the
 * Go reference produced. This suite replays the events against the TS core
 * and asserts LEDGER EQUALITY — same artifacts, same kinds, same positions,
 * same counts — which proves the two implementations make the same decision
 * at every read of every scenario. On divergence, the first differing
 * ledger entry names the read where behaviour split.
 *
 * This replaces the suspended ports-in-lockstep obligation: change the Go
 * core → regenerate fixtures → this suite tells you whether the port needs
 * the same change.
 */

import { readFileSync, readdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it, expect } from 'vitest';
import { replayTrace, type TraceFixture, type LedgerRow } from './ramp-ledger.js';

const FIXTURE_DIR = join(dirname(fileURLToPath(import.meta.url)), 'fixtures', 'jitter-v4');

const KIND_NAMES = ['drop', 'insert', 'snap', 'silence', 'alien', 'lossgap'] as const;

function rowStr(r: LedgerRow): string {
  return `${KIND_NAMES[r[0]] ?? '?'} t=${r[1]} pos=${r[2]} delta=${r[3]}`;
}

describe('v4 trace replay — ledger equality with the Go reference', () => {
  const files = readdirSync(FIXTURE_DIR).filter((f) => f.endsWith('.json'));
  expect(files.length).toBeGreaterThan(0);

  for (const file of files) {
    it(file.replace(/\.json$/, ''), () => {
      const fx = JSON.parse(readFileSync(join(FIXTURE_DIR, file), 'utf8')) as TraceFixture;
      const want = fx.ledger ?? []; // Go marshals an empty ledger as null
      const got = replayTrace(fx);

      // First divergence, named precisely — far more useful than a giant
      // array diff when 800 entries follow a single split decision.
      const n = Math.min(want.length, got.ledger.length);
      for (let i = 0; i < n; i++) {
        const w = want[i]!;
        const g = got.ledger[i]!;
        if (w[0] !== g[0] || w[1] !== g[1] || w[2] !== g[2] || w[3] !== g[3]) {
          expect.fail(
            `ledger diverges at entry ${i}:\n  go: ${rowStr(w)}\n  ts: ${rowStr(g)}\n` +
              `  (go next: ${want[i + 1] ? rowStr(want[i + 1]!) : '—'}; ts next: ${
                got.ledger[i + 1] ? rowStr(got.ledger[i + 1]!) : '—'
              })`
          );
        }
      }
      if (want.length !== got.ledger.length) {
        const extra = want.length > got.ledger.length ? want[n]! : got.ledger[n]!;
        expect.fail(
          `ledger length differs: go ${want.length} vs ts ${got.ledger.length}; ` +
            `first unmatched (${want.length > got.ledger.length ? 'go' : 'ts'}): ${rowStr(extra)}`
        );
      }

      expect(got.summary).toEqual({
        drops: fx.summary.drops,
        inserts: fx.summary.inserts,
        snaps: fx.summary.snaps,
        lossGaps: fx.summary.lossGaps,
        silenceRuns: fx.summary.silenceRuns,
        silenceFrames: fx.summary.silenceFrames,
        concealedFrames: fx.summary.concealedFrames,
        alienSamples: fx.summary.alienSamples,
        joinTime: fx.summary.joinTime,
      });
    });
  }
});
