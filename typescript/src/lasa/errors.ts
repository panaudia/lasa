/**
 * LASA session rejection codes (lasa-core.md §4.6) and the typed error
 * that surfaces them. A rejection is a deliberate close of the
 * established session: the code rides the WebTransport closeCode and
 * correctness rides the code alone — the reason string is advisory and
 * may be truncated or dropped.
 */

/** Session error codes, lasa-core.md §4.6 / §9. */
export const LASA_ERR = {
  /** Malformed / schema-invalid Connection Config. */
  MALFORMED_CONFIG: 0x1a5a00,
  /** Invalid ticket (signature, expired, not yet valid, wrong audience) — or no ticket presented to a ticketed space. */
  INVALID_TICKET: 0x1a5a01,
  /** client_id does not match the ticket claim. */
  CLIENT_ID_MISMATCH: 0x1a5a02,
  /** RETIRED 2026-08-13 (unticketed collisions now supersede); never reused. */
  ID_COLLISION_RETIRED: 0x1a5a03,
  /** Entity id collides with another client's live entity (ticketed). */
  ENTITY_COLLISION: 0x1a5a04,
  /** Ad-hoc entities not permitted for this ticket. */
  AD_HOC_NOT_PERMITTED: 0x1a5a05,
  /** Blocked (reason string SHOULD carry the block expiry timestamp). */
  BLOCKED: 0x1a5a06,
  /** Space at capacity. */
  AT_CAPACITY: 0x1a5a07,
  /** Ticket presented to an unticketed space (mode mismatch). */
  MODE_MISMATCH: 0x1a5a08,
} as const;

const NAMES: Record<number, string> = {
  [LASA_ERR.MALFORMED_CONFIG]: 'malformed connection config',
  [LASA_ERR.INVALID_TICKET]: 'invalid or missing ticket',
  [LASA_ERR.CLIENT_ID_MISMATCH]: 'client_id does not match the ticket claim',
  [LASA_ERR.ID_COLLISION_RETIRED]: 'id collision (retired code)',
  [LASA_ERR.ENTITY_COLLISION]: "entity id collides with another client's live entity",
  [LASA_ERR.AD_HOC_NOT_PERMITTED]: 'ad-hoc entities not permitted for this ticket',
  [LASA_ERR.BLOCKED]: 'blocked',
  [LASA_ERR.AT_CAPACITY]: 'space at capacity',
  [LASA_ERR.MODE_MISMATCH]: 'ticket presented to an unticketed space',
};

/**
 * True iff code is a LASA session-rejection code a server may issue —
 * the §4.6 range minus the retired 0x1A5A03, which MUST never appear
 * on the wire.
 */
export function isLasaRejectionCode(code: number): boolean {
  return (
    code >= LASA_ERR.MALFORMED_CONFIG &&
    code <= LASA_ERR.MODE_MISMATCH &&
    code !== LASA_ERR.ID_COLLISION_RETIRED
  );
}

/** Human-readable name for a §4.6 code (null outside the range). */
export function lasaErrorName(code: number): string | null {
  return NAMES[code] ?? null;
}

/**
 * A LASA session rejection: the server deliberately closed the session
 * with a §4.6 code. `code` is normative; `reason` is advisory.
 */
export class LasaRejectionError extends Error {
  constructor(
    public readonly code: number,
    public readonly reason: string
  ) {
    super(
      `LASA rejection 0x${code.toString(16)} (${lasaErrorName(code) ?? 'unknown'})${reason ? `: ${reason}` : ''}`
    );
    this.name = 'LasaRejectionError';
  }
}
