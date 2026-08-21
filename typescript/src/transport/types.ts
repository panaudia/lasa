/**
 * MOQ-specific type definitions for the Panaudia MOQ client
 *
 * These types align with the MOQ Transport specification (draft-ietf-moq-transport-11)
 * and the Panaudia spatial mixer data structures.
 */

import type { Position, Rotation } from '../shared-types.js';

// Re-export shared types so existing MOQ code can keep importing from './types'
export type {
  Vec3,
  Position,
  Rotation,
  EntityInfo3,
  ClientEventType,
  ClientEventHandler,
  ErrorEvent,
  StateChangeEvent,
} from '../shared-types.js';

export { ConnectionState } from '../shared-types.js';

// ============================================================================
// MOQ Transport Types
// ============================================================================

/**
 * MOQ message types (matching moqtransport v0.5.0)
 * Note: These differ from the latest draft spec
 */
export enum MoqMessageType {
  // Session Messages
  CLIENT_SETUP = 0x20,
  SERVER_SETUP = 0x21,

  // Announcement Messages
  ANNOUNCE = 0x06,
  ANNOUNCE_OK = 0x07,
  ANNOUNCE_ERROR = 0x08,
  UNANNOUNCE = 0x09,

  // Subscription Messages
  SUBSCRIBE = 0x03,
  SUBSCRIBE_OK = 0x04,
  SUBSCRIBE_ERROR = 0x05,
  UNSUBSCRIBE = 0x0a,
  SUBSCRIBE_DONE = 0x0b,

  // Object Messages
  OBJECT_STREAM = 0x00,
  OBJECT_DATAGRAM = 0x01,

  // Other
  GOAWAY = 0x10,
}

/**
 * MOQ Setup parameters
 */
export enum MoqSetupParameter {
  ROLE = 0x00,
  PATH = 0x01,
  MAX_SUBSCRIBE_ID = 0x02,
}

/**
 * MOQ role values
 */
export enum MoqRole {
  PUBLISHER = 0x00,
  SUBSCRIBER = 0x01,
  PUBSUB = 0x02,
}

/**
 * MOQ filter type for subscriptions
 */
export enum MoqFilterType {
  LATEST_GROUP = 0x01,
  LATEST_OBJECT = 0x02,
  ABSOLUTE_START = 0x03,
  ABSOLUTE_RANGE = 0x04,
}

/**
 * MOQ object forwarding preference
 */
export enum MoqForwardingPreference {
  DATAGRAM = 0x00,
  STREAM_TRACK = 0x01,
  STREAM_GROUP = 0x02,
  STREAM_OBJECT = 0x03,
}

/**
 * MOQ error codes
 */
export enum MoqErrorCode {
  NO_ERROR = 0x00,
  INTERNAL_ERROR = 0x01,
  UNAUTHORIZED = 0x02,
  PROTOCOL_VIOLATION = 0x03,
  DUPLICATE_TRACK_ALIAS = 0x04,
  PARAMETER_LENGTH_MISMATCH = 0x05,
  TOO_MANY_SUBSCRIBES = 0x06,
  GOAWAY_TIMEOUT = 0x10,

  // Custom error codes
  INVALID_TOKEN = 0x403,
}

/**
 * MOQ Track representation
 */
export interface MoqTrack {
  namespace: string[];
  name: string;
  alias?: number;
}

/**
 * MOQ Object representation
 */
export interface MoqObject {
  trackAlias: number;
  groupId: bigint;
  objectId: bigint;
  publisherPriority: number;
  payload: Uint8Array;
}

/**
 * MOQ Group ordering preference
 */
export enum MoqGroupOrder {
  NONE = 0x00,
  ASCENDING = 0x01,
  DESCENDING = 0x02,
}

/**
 * MOQ Subscription request
 */
export interface MoqSubscription {
  subscribeId: number;
  namespace: string[];
  trackName: string;
  subscriberPriority?: number; // 0-255, default 128
  groupOrder?: MoqGroupOrder; // default NONE
  forward?: number; // 0 or 1, default 0
  filterType: MoqFilterType;
  authorization?: string;
  resumeOpId?: bigint;
  /** Application subscribe parameters (LASA state subscribe, divergence 3). */
  extraParams?: { type: number; value: bigint | Uint8Array }[];
}

/**
 * MOQ Announcement
 */
export interface MoqAnnouncement {
  requestId: number;
  namespace: string[];
  parameters?: Map<number, Uint8Array>;
}

// ============================================================================
// Client Configuration Types
// ============================================================================

/**
 * Configuration for the Panaudia MOQ client
 */
export interface PanaudiaConfig {
  /** Server URL (e.g., "https://server.example.com:4433") */
  serverUrl: string;

  /** JWT authentication token. Omit for tokenless mode (requires server-side
   *  PANAUDIA_UNTICKETED=1 and an entityId to be provided explicitly). */
  ticket?: string;

  /** Entity ID (UUID) — typically extracted from the JWT ticket, but must be
   *  provided explicitly when `ticket` is omitted. */
  entityId?: string;

  /** Initial position in Panaudia coordinates (0-1 range) */
  initialPosition?: Position;

  /** Initial rotation in degrees */
  initialRotation?: Rotation;

  /** Enable debug logging. Default: false. */
  debug?: boolean;
}

// ============================================================================
// Track Types (Panaudia-specific)
// ============================================================================

/**
 * Track types used by Panaudia
 */
export enum PanaudiaTrackType {
  AUDIO_INPUT = 'in/audio/opus-mono',
  AUDIO_OUTPUT = 'out/audio/opus-stereo',
  STATE = 'state',
  STATE_OUTPUT = 'out/state',
  ATTRIBUTES_OUTPUT = 'out/attributes',
  ENTITY_OUTPUT = 'out/entity',
  SPACE_OUTPUT = 'out/space',
  CONTROL_INPUT = 'in/control',
}

/**
 * Generate track namespace for Panaudia
 */
export function generateTrackNamespace(trackType: PanaudiaTrackType, entityId: string): string[] {
  switch (trackType) {
    case PanaudiaTrackType.AUDIO_INPUT:
      return ['in', 'audio', 'opus-mono', entityId];
    case PanaudiaTrackType.AUDIO_OUTPUT:
      return ['out', 'audio', 'opus-stereo', entityId];
    case PanaudiaTrackType.STATE:
      return ['state', entityId];
    case PanaudiaTrackType.STATE_OUTPUT:
      return ['out', 'state', entityId];
    case PanaudiaTrackType.ATTRIBUTES_OUTPUT:
      return ['out', 'attributes', entityId];
    case PanaudiaTrackType.ENTITY_OUTPUT:
      return ['out', 'entity', entityId];
    case PanaudiaTrackType.SPACE_OUTPUT:
      return ['out', 'space', entityId];
    case PanaudiaTrackType.CONTROL_INPUT:
      return ['in', 'control', entityId];
    default:
      throw new Error(`Unknown track type: ${trackType}`);
  }
}

// ============================================================================
// WebTransport Types (browser API supplements)
// ============================================================================

/**
 * WebTransport hash for server certificate verification
 */
export interface WebTransportHash {
  algorithm: string;
  value: BufferSource;
}

/**
 * WebTransport connection options
 */
export interface WebTransportOptions {
  /** Server certificate hashes for self-signed certs (development) */
  serverCertificateHashes?: WebTransportHash[];

  /** Allow pooling with other connections */
  allowPooling?: boolean;

  /** Require unreliable transport (datagrams) */
  requireUnreliable?: boolean;

  /** Congestion control algorithm */
  congestionControl?: 'default' | 'throughput' | 'low-latency';

  /**
   * Application (sub)protocols to offer via WT-Available-Protocols. Used to
   * negotiate the MOQ draft over WebTransport — we offer ["moqt-16"] so the
   * server selects draft-16. Browsers without subprotocol support ignore this.
   */
  protocols?: string[];
}
