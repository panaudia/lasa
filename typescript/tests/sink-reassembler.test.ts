// Lockstep replay of the sink-reassembler fixtures exported by the Go
// reference (panaudia-bridge bridge/reassembler_test.go →
// fixtures/reassembler/*.json): the same packets in, the same released
// events out — event-for-event, payload-for-payload — plus counter
// equality. Regenerate the fixtures after any reassembler change:
//
//	REASM_EXPORT_DIR=$(pwd)/../lasa/typescript/tests/fixtures/reassembler \
//	  go test ./bridge/ -run TestExportReassemblerFixtures
//
// (run from the panaudia-bridge repo root).

import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { SinkReassembler, REASM_SKIP_THRESHOLD } from '../src/audio/sink-reassembler.js';

interface ReasmFixture {
  name: string;
  lag: number;
  skipThreshold: number;
  packets: [number, number][]; // [seq, redOffset] in arrival order
  events: [number, number][]; // [seq, kind]: 0 = audio, 1 = conceal
  counters: {
    delivered: number;
    repaired: number;
    concealed: number;
    skipped: number;
    stale: number;
  };
}

const FIXTURE_DIR = join(__dirname, 'fixtures', 'reassembler');

/** The synthetic payload rule shared with the Go generator: frame seq's
 * payload is its seq as u32 little-endian. */
function canonPayload(seq: number): Uint8Array {
  const b = new Uint8Array(4);
  new DataView(b.buffer).setUint32(0, seq >>> 0, true);
  return b;
}

describe('sink-reassembler fixture replay (lockstep with Go)', () => {
  const files = readdirSync(FIXTURE_DIR).filter((f) => f.endsWith('.json'));
  it('has the fixture set', () => {
    expect(files.length).toBeGreaterThanOrEqual(8);
  });

  for (const file of files) {
    it(file.replace(/\.json$/, ''), () => {
      const fx = JSON.parse(readFileSync(join(FIXTURE_DIR, file), 'utf8')) as ReasmFixture;
      expect(fx.skipThreshold).toBe(REASM_SKIP_THRESHOLD);

      const ra = new SinkReassembler(fx.lag);
      const got: [number, number][] = [];
      for (const [seq, redOffset] of fx.packets) {
        const events = ra.push(
          BigInt(seq),
          canonPayload(seq),
          redOffset,
          redOffset > 0 ? canonPayload(seq - redOffset) : null
        );
        for (const e of events) {
          const s = Number(e.seq);
          if (e.audio === null) {
            got.push([s, 1]);
          } else {
            // Released content must be the frame's own payload whether
            // it came from the primary or a repaired copy.
            expect(e.audio, `payload of seq ${s}`).toEqual(canonPayload(s));
            got.push([s, 0]);
          }
        }
      }
      expect(got).toEqual(fx.events);
      expect({
        delivered: ra.delivered,
        repaired: ra.repaired,
        concealed: ra.concealed,
        skipped: ra.skipped,
        stale: ra.stale,
      }).toEqual(fx.counters);
    });
  }
});
