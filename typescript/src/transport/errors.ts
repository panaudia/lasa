/**
 * Custom error types for the Panaudia MOQ client
 */

/**
 * Base error class for all MOQ client errors
 */
export class MoqClientError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly details?: unknown
  ) {
    super(message);
    this.name = 'MoqClientError';
  }
}

/**
 * Error thrown when authentication fails
 */
export class AuthenticationError extends MoqClientError {
  constructor(
    message: string,
    public readonly moqErrorCode?: number,
    details?: unknown
  ) {
    super(message, 'AUTHENTICATION_FAILED', details);
    this.name = 'AuthenticationError';
  }

  /**
   * Check if this is an invalid token error
   */
  isInvalidToken(): boolean {
    return this.moqErrorCode !== undefined && isSubscribeAuthError(this.moqErrorCode);
  }

  /**
   * Check if this is an expired token error
   */
  isExpiredToken(): boolean {
    return this.message.toLowerCase().includes('expired');
  }
}

/**
 * Error thrown when MOQ protocol error occurs
 */
export class ProtocolError extends MoqClientError {
  constructor(
    message: string,
    public readonly moqErrorCode?: number,
    details?: unknown
  ) {
    super(message, 'PROTOCOL_ERROR', details);
    this.name = 'ProtocolError';
  }
}

/**
 * Error thrown when subscription fails
 */
export class SubscriptionError extends MoqClientError {
  constructor(
    message: string,
    public readonly moqErrorCode?: number,
    public readonly trackNamespace?: string[],
    details?: unknown
  ) {
    super(message, 'SUBSCRIPTION_FAILED', details);
    this.name = 'SubscriptionError';
  }
}

/**
 * Error thrown when announcement fails
 */
export class AnnouncementError extends MoqClientError {
  constructor(
    message: string,
    public readonly moqErrorCode?: number,
    public readonly namespace?: string[],
    details?: unknown
  ) {
    super(message, 'ANNOUNCEMENT_FAILED', details);
    this.name = 'AnnouncementError';
  }
}

/**
 * Session-level (connection close) error codes, as emitted by the
 * moqtransport fork the LASA server runs (`errors.go`, `ErrorCode`).
 */
export function getMoqErrorMessage(code: number): string {
  switch (code) {
    case 0x00:
      return 'No error';
    case 0x01:
      return 'Internal error';
    case 0x02:
      return 'Unauthorized';
    case 0x03:
      return 'Protocol violation';
    case 0x04:
      return 'Invalid request ID';
    case 0x05:
      return 'Duplicate track alias';
    case 0x06:
      return 'Key-value formatting error';
    case 0x07:
      return 'Too many requests';
    case 0x08:
      return 'Invalid path';
    case 0x09:
      return 'Malformed path';
    case 0x10:
      return 'GOAWAY timeout';
    case 0x11:
      return 'Control message timeout';
    case 0x12:
      return 'Data stream timeout';
    case 0x13:
      return 'Auth token cache overflow';
    case 0x14:
      return 'Duplicate auth token alias';
    case 0x15:
      return 'Version negotiation failed';
    case 0x16:
      return 'Malformed auth token';
    case 0x17:
      return 'Unknown auth token alias';
    default:
      return `Unknown error (0x${code.toString(16)})`;
  }
}

/**
 * SUBSCRIBE_ERROR codes. A separate table from the session codes above:
 * the fork's `ErrorCodeSubscribe*` set (`errors.go`), which is what the
 * LASA server sends when it refuses a subscribe.
 */
export function getSubscribeErrorMessage(code: number): string {
  switch (code) {
    case 0x00:
      return 'Internal error';
    case 0x01:
      return 'Unauthorized';
    case 0x02:
      return 'Timeout';
    case 0x03:
      return 'Not supported';
    case 0x04:
      return 'Track does not exist';
    case 0x05:
      return 'Invalid range';
    case 0x10:
      return 'Malformed auth token';
    case 0x12:
      return 'Expired auth token';
    case 0x403:
      return 'Invalid token (custom)';
    default:
      return `Unknown subscribe error (0x${code.toString(16)})`;
  }
}

/** SUBSCRIBE_ERROR codes that mean the subscribe was refused on auth grounds. */
export function isSubscribeAuthError(code: number): boolean {
  return code === 0x01 || code === 0x10 || code === 0x12 || code === 0x403;
}

