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
 * Error thrown when WebTransport is not supported
 */
export class WebTransportNotSupportedError extends MoqClientError {
  constructor() {
    super(
      'WebTransport is not supported in this browser. Try Chrome 97+, Edge 97+, or Safari 16.4+.',
      'WEBTRANSPORT_NOT_SUPPORTED'
    );
    this.name = 'WebTransportNotSupportedError';
  }
}

/**
 * Error thrown when connection fails
 */
export class ConnectionError extends MoqClientError {
  constructor(message: string, details?: unknown) {
    super(message, 'CONNECTION_FAILED', details);
    this.name = 'ConnectionError';
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
    return this.moqErrorCode === 0x02 || this.moqErrorCode === 0x403;
  }

  /**
   * Check if this is an expired token error
   */
  isExpiredToken(): boolean {
    return this.message.toLowerCase().includes('expired');
  }
}

/**
 * Error thrown when JWT parsing fails
 */
export class JwtParseError extends MoqClientError {
  constructor(message: string, details?: unknown) {
    super(message, 'JWT_PARSE_FAILED', details);
    this.name = 'JwtParseError';
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
 * Error thrown when the client is in an invalid state for an operation
 */
export class InvalidStateError extends MoqClientError {
  constructor(expectedState: string, actualState: string) {
    super(
      `Invalid state: expected ${expectedState}, but was ${actualState}`,
      'INVALID_STATE',
      { expectedState, actualState }
    );
    this.name = 'InvalidStateError';
  }
}

/**
 * Error thrown when a timeout occurs
 */
export class TimeoutError extends MoqClientError {
  constructor(operation: string, timeoutMs: number) {
    super(
      `Operation '${operation}' timed out after ${timeoutMs}ms`,
      'TIMEOUT',
      { operation, timeoutMs }
    );
    this.name = 'TimeoutError';
  }
}

/**
 * Maps MOQ error codes to human-readable messages
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
      return 'Duplicate track alias';
    case 0x05:
      return 'Parameter length mismatch';
    case 0x06:
      return 'Too many subscribes';
    case 0x10:
      return 'GOAWAY timeout';
    case 0x403:
      return 'Invalid token (custom)';
    default:
      return `Unknown error (0x${code.toString(16)})`;
  }
}

/**
 * Wraps an unknown error into a MoqClientError
 */
export function wrapError(error: unknown, defaultCode: string = 'UNKNOWN'): MoqClientError {
  if (error instanceof MoqClientError) {
    return error;
  }

  if (error instanceof Error) {
    return new MoqClientError(error.message, defaultCode, error);
  }

  return new MoqClientError(String(error), defaultCode);
}
