/**
 * session.ts — the MOQ control-stream session: CLIENT_SETUP/SERVER_SETUP handshake,
 * SUBSCRIBE/ANNOUNCE, incoming-SUBSCRIBE handling, and the background message loop.
 *
 * Extracted verbatim from client.ts (Phase 2.0 of worker-transport-plan.md) so the
 * session can move into the worker that owns the WebTransport (Phase 2.1+).
 * Behavior unchanged; it depends only on MoqConnection + the MOQ wire codec.
 */
import { MoqConnection } from './connection.js';
import { MoqRole, MoqFilterType, MoqMessageType } from './types.js';
import {
  buildClientSetup,
  buildSubscribe,
  buildAnnounce,
  parseServerSetup,
  parseSubscribeOk,
  parseSubscribeError,
  parseAnnounceOk,
  parseAnnounceError,
  MOQ_TRANSPORT_VERSION,
  MessageBuilder,
  wrapWithLengthFrame,
  decodeVarint,
  type Kvp,
} from './moq-transport.js';
import {
  AuthenticationError,
  ProtocolError,
  SubscriptionError,
  AnnouncementError,
  getMoqErrorMessage,
} from './errors.js';

export class MoqSession {
  private controlStream: WebTransportBidirectionalStream | null = null;
  private writer: WritableStreamDefaultWriter<Uint8Array> | null = null;
  private reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
  private readBuffer: Uint8Array = new Uint8Array(0);

  private nextSubscribeId = 1;
  private nextTrackAlias = 1;
  private nextAnnounceRequestId = 2; // Client uses even IDs for announces (to avoid collisions with server)

  // Track state
  private subscriptions: Map<number, { namespace: string[]; trackName: string; alias: number }> = new Map();
  private announcements: Map<string, { namespace: string[] }> = new Map();
  private incomingSubscriptions: Map<number, { trackAlias: number; namespace: string[] }> = new Map();

  // Callbacks for when server subscribes to our tracks
  private onIncomingSubscribeCallback: ((namespace: string[], trackAlias: number) => void) | null = null;

  // PUBLISH_DONE (0x0B) dispatch, keyed by the subscribe request id
  // (divergence 4, PROVENANCE.md): the server ends a subscription
  // instance natively — LASA's state plane uses too-far-behind (0x06)
  // as the re-subscribe cue.
  private publishDoneHandlers: Map<number, (statusCode: number, reason: string) => void> = new Map();

  private readonly debug: boolean;

  constructor(private readonly connection: MoqConnection, debug: boolean = false) {
    this.debug = debug;
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private log(...args: any[]): void {
    if (this.debug) {
      console.log('[MOQ]', ...args);
    }
  }

  /**
   * Set callback for when server subscribes to one of our announced tracks
   */
  onIncomingSubscribe(callback: (namespace: string[], trackAlias: number) => void): void {
    this.onIncomingSubscribeCallback = callback;
  }

  /**
   * Register a handler for PUBLISH_DONE on a subscription (by the
   * subscribe id returned from subscribe()). One-shot per instance.
   */
  onPublishDone(subscribeId: number, handler: (statusCode: number, reason: string) => void): void {
    this.publishDoneHandlers.set(subscribeId, handler);
  }

  removePublishDoneHandler(subscribeId: number): void {
    this.publishDoneHandlers.delete(subscribeId);
  }

  /**
   * Initialize the MOQ session over the control stream
   * @param role - The MOQ role (publisher, subscriber, or pubsub)
   * @param path - Optional path parameter
   * @param maxSubscribeId - Max number of requests server can send to client (default: 100)
   */
  async initialize(
    role: MoqRole,
    path?: string,
    maxSubscribeId: number = 100,
    extraSetupParams?: Kvp[]
  ): Promise<void> {
    this.log('Creating control stream...');
    // Create bidirectional control stream
    this.controlStream = await this.connection.createControlStream();
    this.writer = this.controlStream.writable.getWriter();
    this.reader = this.controlStream.readable.getReader();
    this.log('Control stream created, sending CLIENT_SETUP...');

    // Send CLIENT_SETUP with maxSubscribeId to allow server to send requests to us
    const setupMsg = buildClientSetup([MOQ_TRANSPORT_VERSION], role, path, maxSubscribeId, extraSetupParams);
    this.log('CLIENT_SETUP message size:', setupMsg.length, 'bytes');
    this.log('CLIENT_SETUP hex:', Array.from(setupMsg).map(b => b.toString(16).padStart(2, '0')).join(' '));
    await this.writer.write(setupMsg);
    this.log('CLIENT_SETUP sent, waiting for SERVER_SETUP...');

    // Wait for SERVER_SETUP response (with proper length framing)
    const { type, content } = await this.readFramedMessage();
    this.log('Received response type: 0x' + type.toString(16) + ', content size:', content.length, 'bytes');

    if (type !== MoqMessageType.SERVER_SETUP) {
      throw new ProtocolError(
        `Expected SERVER_SETUP (0x41), got message type 0x${type.toString(16)}`,
        type
      );
    }

    // Parse content directly (type and length already stripped)
    const serverSetup = parseServerSetup(content, 0);
    this.log('Session established, server version:', serverSetup.selectedVersion.toString(16));
  }

  /**
   * Subscribe to a track with JWT authorization
   */
  async subscribe(
    namespace: string[],
    trackName: string,
    authorization?: string,
    resumeOpId?: bigint,
    extraParams?: { type: number; value: bigint | Uint8Array }[]
  ): Promise<number> {
    const subscribeId = this.nextSubscribeId++;

    const subscribeMsg = buildSubscribe({
      subscribeId,
      namespace,
      trackName,
      filterType: MoqFilterType.LATEST_GROUP,
      authorization,
      resumeOpId,
      extraParams,
    });

    this.log('SUBSCRIBE message size:', subscribeMsg.length, 'bytes');
    await this.writer!.write(subscribeMsg);

    // Wait for SUBSCRIBE_OK or SUBSCRIBE_ERROR
    // The server may send other messages (like ANNOUNCE) before the response,
    // so we need to handle/skip those while waiting
    const { type, content } = await this.waitForMessage([
      MoqMessageType.SUBSCRIBE_OK,
      MoqMessageType.SUBSCRIBE_ERROR,
    ]);

    if (type === MoqMessageType.SUBSCRIBE_OK) {
      const ok = parseSubscribeOk(content, 0);
      // Use the server-assigned trackAlias from SUBSCRIBE_OK, not our local one.
      // The server uses this alias when sending datagrams for this subscription.
      this.log('Subscribed successfully, subscribeId:', ok.subscribeId, 'trackAlias:', ok.trackAlias);
      this.subscriptions.set(subscribeId, { namespace, trackName, alias: ok.trackAlias });
      return subscribeId;
    } else if (type === MoqMessageType.SUBSCRIBE_ERROR) {
      const error = parseSubscribeError(content, 0);
      const errorMessage = `${error.reasonPhrase} (${getMoqErrorMessage(error.errorCode)})`;

      // Check if this is an authentication error
      if (error.errorCode === 0x02 || error.errorCode === 0x403) {
        throw new AuthenticationError(errorMessage, error.errorCode, { namespace, trackName });
      }

      throw new SubscriptionError(errorMessage, error.errorCode, namespace);
    } else {
      throw new ProtocolError(
        `Expected SUBSCRIBE_OK or SUBSCRIBE_ERROR, got message type 0x${type.toString(16)}`,
        type
      );
    }
  }

  // Single-reader dispatch (divergence 2, PROVENANCE.md): once the
  // background message loop is running it is the ONLY reader of the
  // control stream; waiters for a response register here and the loop
  // fulfils them. Two concurrent readers steal each other's messages —
  // the LASA client keeps the loop running for the whole session (the
  // server may subscribe our tracks at any time), so subscribe/announce
  // must not read inline after start.
  private pendingResponses: {
    types: number[];
    resolve: (m: { type: number; content: Uint8Array }) => void;
    reject: (e: Error) => void;
  }[] = [];
  private messageLoopRunning = false;

  /**
   * Wait for a specific message type. Before the message loop starts,
   * reads inline (handling unrelated messages that arrive first); once
   * the loop runs, registers with the single-reader dispatch instead.
   */
  private async waitForMessage(expectedTypes: number[]): Promise<{ type: number; content: Uint8Array }> {
    if (this.messageLoopRunning) {
      return new Promise((resolve, reject) => {
        const timer = setTimeout(
          () =>
            reject(
              new ProtocolError(
                `Timeout waiting for message types: ${expectedTypes.map(t => '0x' + t.toString(16)).join(', ')}`,
                0
              )
            ),
          10_000
        );
        this.pendingResponses.push({
          types: expectedTypes,
          resolve: (m) => {
            clearTimeout(timer);
            resolve(m);
          },
          reject: (e) => {
            clearTimeout(timer);
            reject(e);
          },
        });
      });
    }
    const maxAttempts = 20; // Prevent infinite loops
    for (let i = 0; i < maxAttempts; i++) {
      const { type, content } = await this.readFramedMessage();

      if (expectedTypes.includes(type)) {
        return { type, content };
      }

      // Handle other message types that may arrive
      this.log(`Received unexpected message type 0x${type.toString(16)} while waiting, handling it`);
      await this.handleUnexpectedMessage(type, content);
    }

    throw new ProtocolError(
      `Timeout waiting for message types: ${expectedTypes.map(t => '0x' + t.toString(16)).join(', ')}`,
      0
    );
  }

  /**
   * Handle messages that arrive when we're waiting for something else
   */
  private async handleUnexpectedMessage(type: number, content: Uint8Array): Promise<void> {
    switch (type) {
      case MoqMessageType.ANNOUNCE:
        // Server is announcing a track to us - respond with ANNOUNCE_OK
        this.log('Received ANNOUNCE from server, sending ANNOUNCE_OK');
        await this.sendAnnounceOk(content);
        break;

      case 0x11: // SUBSCRIBE_ANNOUNCES
        // Server wants to subscribe to our announcements - respond with OK
        this.log('Received SUBSCRIBE_ANNOUNCES from server, sending OK');
        await this.sendSubscribeAnnouncesOk(content);
        break;

      case MoqMessageType.SUBSCRIBE:
        // Server wants to subscribe to one of our tracks - accept it
        this.log('Received SUBSCRIBE from server, sending SUBSCRIBE_OK');
        await this.handleIncomingSubscribe(content);
        break;

      case 0x0b: {
        // PUBLISH_DONE: RequestID, StatusCode, StreamCount, Reason.
        // The request id is the subscribe id we sent (divergence 4).
        const rid = decodeVarint(content, 0);
        const status = decodeVarint(content, rid.bytesRead);
        this.log(`PUBLISH_DONE requestId=${rid.value} status=${status.value}`);
        const handler = this.publishDoneHandlers.get(Number(rid.value));
        if (handler) {
          this.publishDoneHandlers.delete(Number(rid.value));
          handler(Number(status.value), '');
        }
        break;
      }

      default:
        // Log and skip unknown messages
        this.log(`Skipping unhandled message type 0x${type.toString(16)}`);
    }
  }

  /**
   * Handle incoming SUBSCRIBE from server and respond with SUBSCRIBE_OK
   *
   * Per moqtransport v0.5.1 / draft-ietf-moq-transport-11, the SUBSCRIBE
   * wire format does NOT include TrackAlias. The publisher (us) assigns a
   * TrackAlias and returns it in SUBSCRIBE_OK.
   *
   * SUBSCRIBE wire format: RequestID, Namespace, TrackName, Priority,
   *   GroupOrder, Forward, FilterType, Parameters
   *
   * SUBSCRIBE_OK wire format: RequestID, TrackAlias, Expires, GroupOrder,
   *   ContentExists, [LargestLocation], Parameters
   */
  private async handleIncomingSubscribe(content: Uint8Array): Promise<void> {
    let pos = 0;

    // Request ID (varint)
    const requestIdByte = content[pos]!;
    let requestId: number;
    if (requestIdByte < 64) {
      requestId = requestIdByte;
      pos += 1;
    } else if ((requestIdByte & 0xc0) === 0x40) {
      requestId = ((requestIdByte & 0x3f) << 8) | content[pos + 1]!;
      pos += 2;
    } else {
      requestId = requestIdByte & 0x3f;
      pos += 1;
    }

    // Parse namespace (directly after RequestID — no TrackAlias in SUBSCRIBE)
    const namespace = this.parseNamespaceFromContent(content, pos);

    // Assign a local TrackAlias for this subscription. The server will use
    // this alias to identify our datagrams for this track.
    const trackAlias = this.nextTrackAlias++;
    this.log(`Server subscribing to: ${namespace.join('/')}, assigning trackAlias=${trackAlias}`);

    // Send SUBSCRIBE_OK with TrackAlias
    // Format: RequestID, TrackAlias, Expires, GroupOrder(1 byte), ContentExists(1 byte), Parameters
    const builder = new MessageBuilder();
    builder.writeVarint(requestId);    // Request ID
    builder.writeVarint(trackAlias);   // Track Alias (assigned by us)
    builder.writeVarint(0);            // Expires (0 = no expiry)
    builder.writeRaw(new Uint8Array([0x01])); // Group order (1 byte: ascending)
    builder.writeRaw(new Uint8Array([0x00])); // Content exists (1 byte: false)
    builder.writeVarint(0);            // Number of parameters

    const msg = wrapWithLengthFrame(MoqMessageType.SUBSCRIBE_OK, builder.build());
    await this.writer!.write(msg);
    this.log('Sent SUBSCRIBE_OK for requestId:', requestId, 'trackAlias:', trackAlias);

    // Store the subscription so we know the track alias to use when publishing
    this.incomingSubscriptions.set(requestId, { trackAlias, namespace });

    // Notify callback if registered
    if (this.onIncomingSubscribeCallback) {
      this.onIncomingSubscribeCallback(namespace, trackAlias);
    }
  }

  /**
   * Get track alias for an incoming subscription by namespace
   */
  getIncomingTrackAlias(namespacePrefix: string): number | undefined {
    for (const [, sub] of this.incomingSubscriptions) {
      if (sub.namespace.join('/').startsWith(namespacePrefix)) {
        return sub.trackAlias;
      }
    }
    return undefined;
  }

  /**
   * Parse namespace from content starting at given position
   */
  private parseNamespaceFromContent(content: Uint8Array, startPos: number): string[] {
    let pos = startPos;
    const namespace: string[] = [];

    if (pos >= content.length) return namespace;

    // Read namespace tuple length
    const firstByte = content[pos]!;
    let nsLength: number;

    if (firstByte < 64) {
      nsLength = firstByte;
      pos += 1;
    } else if ((firstByte & 0xc0) === 0x40) {
      nsLength = ((firstByte & 0x3f) << 8) | content[pos + 1]!;
      pos += 2;
    } else {
      nsLength = firstByte & 0x3f;
      pos += 1;
    }

    // Read each namespace part
    for (let i = 0; i < nsLength && pos < content.length; i++) {
      const partLenByte = content[pos]!;
      let partLen: number;

      if (partLenByte < 64) {
        partLen = partLenByte;
        pos += 1;
      } else if ((partLenByte & 0xc0) === 0x40) {
        partLen = ((partLenByte & 0x3f) << 8) | content[pos + 1]!;
        pos += 2;
      } else {
        partLen = partLenByte & 0x3f;
        pos += 1;
      }

      if (pos + partLen <= content.length) {
        const part = new TextDecoder().decode(content.slice(pos, pos + partLen));
        namespace.push(part);
        pos += partLen;
      }
    }

    return namespace;
  }

  /**
   * Send ANNOUNCE_OK response
   */
  private async sendAnnounceOk(announceContent: Uint8Array): Promise<void> {
    // Parse the RequestID from the ANNOUNCE message (first varint)
    const requestId = this.parseRequestId(announceContent);
    this.log('Sending ANNOUNCE_OK for requestId:', requestId);

    // Build ANNOUNCE_OK: just the RequestID (per moqtransport v0.5.0)
    const builder = new MessageBuilder();
    builder.writeVarint(requestId);

    const msg = wrapWithLengthFrame(MoqMessageType.ANNOUNCE_OK, builder.build());
    this.log('ANNOUNCE_OK message size:', msg.length, 'bytes');
    await this.writer!.write(msg);
  }

  /**
   * Send SUBSCRIBE_ANNOUNCES_OK response
   */
  private async sendSubscribeAnnouncesOk(subscribeAnnouncesContent: Uint8Array): Promise<void> {
    // Parse the RequestID from the message (first varint)
    const requestId = this.parseRequestId(subscribeAnnouncesContent);
    this.log('Sending SUBSCRIBE_ANNOUNCES_OK for requestId:', requestId);

    // Build SUBSCRIBE_ANNOUNCES_OK (0x12): just the RequestID (per moqtransport v0.5.0)
    const builder = new MessageBuilder();
    builder.writeVarint(requestId);

    const msg = wrapWithLengthFrame(0x12, builder.build()); // 0x12 = SUBSCRIBE_ANNOUNCES_OK
    await this.writer!.write(msg);
  }

  /**
   * Parse RequestID (first varint) from message content
   */
  private parseRequestId(content: Uint8Array): number {
    const firstByte = content[0]!;
    if (firstByte < 64) {
      return firstByte;
    } else if ((firstByte & 0xc0) === 0x40) {
      return ((firstByte & 0x3f) << 8) | content[1]!;
    } else if ((firstByte & 0xc0) === 0x80) {
      return ((firstByte & 0x3f) << 24) | (content[1]! << 16) | (content[2]! << 8) | content[3]!;
    } else {
      // 8-byte varint - for simplicity, just return the lower 32 bits
      return (content[4]! << 24) | (content[5]! << 16) | (content[6]! << 8) | content[7]!;
    }
  }

  /**
   * Announce a track namespace
   */
  async announce(namespace: string[], authorization?: string): Promise<void> {
    const requestId = this.nextAnnounceRequestId;
    this.nextAnnounceRequestId += 2; // Increment by 2 to stay in client's ID space

    const parameters = new Map<number, Uint8Array>();
    if (authorization) {
      // Add authorization as parameter 0x03 (AuthorizationTokenParameterKey per moqtransport v0.5.1)
      const encoder = new TextEncoder();
      parameters.set(0x03, encoder.encode(authorization));
    }
    const announceMsg = buildAnnounce({ requestId, namespace, parameters: parameters.size > 0 ? parameters : undefined });
    this.log('ANNOUNCE message size:', announceMsg.length, 'bytes, requestId:', requestId);
    await this.writer!.write(announceMsg);

    // Wait for ANNOUNCE_OK or ANNOUNCE_ERROR
    // Server may send other messages (like SUBSCRIBE) before the response
    const { type, content } = await this.waitForMessage([
      MoqMessageType.ANNOUNCE_OK,
      MoqMessageType.ANNOUNCE_ERROR,
    ]);

    if (type === MoqMessageType.ANNOUNCE_OK) {
      const ok = parseAnnounceOk(content, 0);
      // moqtransport v0.5.0: ANNOUNCE_OK only contains RequestID, not namespace
      // We use the namespace from our local state
      const nsKey = namespace.join('/');
      this.announcements.set(nsKey, { namespace });
      this.log('Announced successfully:', nsKey, 'requestId:', ok.requestId);
    } else if (type === MoqMessageType.ANNOUNCE_ERROR) {
      const error = parseAnnounceError(content, 0);
      const errorMessage = `${error.reasonPhrase} (${getMoqErrorMessage(error.errorCode)})`;
      throw new AnnouncementError(errorMessage, error.errorCode, namespace);
    } else {
      throw new ProtocolError(
        `Expected ANNOUNCE_OK or ANNOUNCE_ERROR, got message type 0x${type.toString(16)}`,
        type
      );
    }
  }

  /**
   * Get track alias for a subscription
   */
  getTrackAlias(subscribeId: number): number | undefined {
    return this.subscriptions.get(subscribeId)?.alias;
  }

  /**
   * Start background message processing loop
   * This handles messages that arrive after initial connection setup
   */
  startMessageLoop(): void {
    this.messageLoopRunning = true;
    this.processMessages().catch((error) => {
      this.log('Message loop ended:', error.message);
    });
  }

  /**
   * Background message processing
   */
  private async processMessages(): Promise<void> {
    this.log('Starting background message processing loop');
    while (this.reader) {
      try {
        const { type, content } = await this.readFramedMessage();
        this.log(`Background received message type 0x${type.toString(16)}`);
        // Single-reader dispatch: a registered waiter takes precedence.
        const idx = this.pendingResponses.findIndex((p) => p.types.includes(type));
        if (idx >= 0) {
          const [pending] = this.pendingResponses.splice(idx, 1);
          pending!.resolve({ type, content });
          continue;
        }
        await this.handleUnexpectedMessage(type, content);
      } catch (error) {
        // Stream closed or error - exit loop
        this.log('Message processing stopped:', (error as Error).message);
        for (const p of this.pendingResponses.splice(0)) {
          p.reject(error as Error);
        }
        break;
      }
    }
  }

  /**
   * Close the session
   */
  async close(): Promise<void> {
    if (this.writer) {
      try {
        await this.writer.close();
      } catch {
        // Ignore close errors
      }
      this.writer = null;
    }
    if (this.reader) {
      try {
        await this.reader.cancel();
      } catch {
        // Ignore cancel errors
      }
      this.reader = null;
    }
    this.controlStream = null;
  }

  /**
   * Read a complete message from the control stream with proper length framing
   * Format: [Type varint] [Length: 2 bytes big-endian] [Content: length bytes]
   * Returns: { type, content } where content is the message body without type/length
   */
  private async readFramedMessage(): Promise<{ type: number; content: Uint8Array }> {
    // Ensure we have enough bytes for the header
    // Minimum: 1 byte type + 2 bytes length = 3 bytes
    while (this.readBuffer.length < 3) {
      const { value, done } = await this.reader!.read();
      if (done) {
        throw new Error('Control stream closed unexpectedly');
      }
      const newBuffer = new Uint8Array(this.readBuffer.length + value.length);
      newBuffer.set(this.readBuffer);
      newBuffer.set(value, this.readBuffer.length);
      this.readBuffer = newBuffer;
    }

    // Parse message type (QUIC varint)
    let typeLength = 1;
    const firstByte = this.readBuffer[0]!;
    const prefix = firstByte >> 6;
    if (prefix === 1) typeLength = 2;
    else if (prefix === 2) typeLength = 4;
    else if (prefix === 3) typeLength = 8;

    // Ensure we have enough bytes for type + 2 byte length
    const headerSize = typeLength + 2;
    while (this.readBuffer.length < headerSize) {
      const { value, done } = await this.reader!.read();
      if (done) {
        throw new Error('Control stream closed unexpectedly');
      }
      const newBuffer = new Uint8Array(this.readBuffer.length + value.length);
      newBuffer.set(this.readBuffer);
      newBuffer.set(value, this.readBuffer.length);
      this.readBuffer = newBuffer;
    }

    // Extract message type
    let type: number;
    if (typeLength === 1) {
      type = firstByte;
    } else if (typeLength === 2) {
      type = ((firstByte & 0x3f) << 8) | this.readBuffer[1]!;
    } else {
      // For simplicity, only handle 1-2 byte varints (covers all MOQ message types)
      throw new Error(`Unsupported varint length: ${typeLength}`);
    }

    // Extract length (2 bytes big-endian)
    const lengthOffset = typeLength;
    const contentLength = (this.readBuffer[lengthOffset]! << 8) | this.readBuffer[lengthOffset + 1]!;
    this.log('readFramedMessage: type=0x' + type.toString(16) + ', contentLength=' + contentLength);

    // Read until we have the full message
    const totalSize = headerSize + contentLength;
    while (this.readBuffer.length < totalSize) {
      const { value, done } = await this.reader!.read();
      if (done) {
        throw new Error('Control stream closed unexpectedly');
      }
      const newBuffer = new Uint8Array(this.readBuffer.length + value.length);
      newBuffer.set(this.readBuffer);
      newBuffer.set(value, this.readBuffer.length);
      this.readBuffer = newBuffer;
    }

    // Extract content
    const content = this.readBuffer.slice(headerSize, totalSize);

    // Remove consumed bytes from buffer
    this.readBuffer = this.readBuffer.slice(totalSize);

    this.log('readFramedMessage: returning type=0x' + type.toString(16) + ', content.length=' + content.length);
    return { type, content };
  }

}
