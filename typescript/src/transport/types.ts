/**
 * MoQ transport type definitions.
 *
 * These align with the MoQ Transport wire format as spoken by the
 * server's moqtransport fork; message type values differ from the
 * latest IETF draft. WebTransport browser-API supplements live in
 * webtransport.d.ts as global declarations.
 */

// ============================================================================
// Connection state
// ============================================================================

/**
 * Transport connection state
 */
export enum ConnectionState {
  DISCONNECTED = 'disconnected',
  CONNECTING = 'connecting',
  CONNECTED = 'connected',
  ERROR = 'error',
}

// ============================================================================
// MoQ Transport Types
// ============================================================================

/**
 * MoQ message types (matching the server's moqtransport version)
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
  PUBLISH_DONE = 0x0b,
  SUBSCRIBE_ANNOUNCES = 0x11,
  SUBSCRIBE_ANNOUNCES_OK = 0x12,

  // Object Messages
  OBJECT_STREAM = 0x00,
  OBJECT_DATAGRAM = 0x01,

  // Other
  GOAWAY = 0x10,
}

/**
 * MoQ Setup parameters
 */
export enum MoqSetupParameter {
  ROLE = 0x00,
  PATH = 0x01,
  MAX_SUBSCRIBE_ID = 0x02,
}

/**
 * MoQ role values
 */
export enum MoqRole {
  PUBLISHER = 0x00,
  SUBSCRIBER = 0x01,
  PUBSUB = 0x02,
}

/**
 * MoQ filter type for subscriptions
 */
export enum MoqFilterType {
  LATEST_GROUP = 0x01,
  LATEST_OBJECT = 0x02,
  ABSOLUTE_START = 0x03,
  ABSOLUTE_RANGE = 0x04,
}

/**
 * MoQ Group ordering preference
 */
export enum MoqGroupOrder {
  NONE = 0x00,
  ASCENDING = 0x01,
  DESCENDING = 0x02,
}

/**
 * MoQ Subscription request
 */
export interface MoqSubscription {
  subscribeId: number;
  namespace: string[];
  trackName: string;
  subscriberPriority?: number; // 0-255, default 128
  groupOrder?: MoqGroupOrder; // default ASCENDING (what buildSubscribe sends)
  forward?: number; // 0 or 1, default 1 (objects flow without a further request)
  filterType: MoqFilterType;
  authorization?: string;
  resumeOpId?: bigint;
  /** Application subscribe parameters (LASA state subscribe, divergence 3). */
  extraParams?: { type: number; value: bigint | Uint8Array }[];
}

/**
 * MoQ Announcement
 */
export interface MoqAnnouncement {
  requestId: number;
  namespace: string[];
  parameters?: Map<number, Uint8Array>;
}
