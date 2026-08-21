/**
 * WebTransport API type definitions
 *
 * These supplement the browser's WebTransport API types.
 * Based on the W3C WebTransport specification.
 */

declare global {
  interface WebTransportOptions {
    allowPooling?: boolean;
    requireUnreliable?: boolean;
    serverCertificateHashes?: WebTransportHash[];
    congestionControl?: 'default' | 'throughput' | 'low-latency';
    /** Subprotocols offered via WT-Available-Protocols (e.g. ["moqt-16"]). */
    protocols?: string[];
  }

  interface WebTransportHash {
    algorithm: string;
    value: BufferSource;
  }

  interface WebTransportCloseInfo {
    closeCode?: number;
    reason?: string;
  }

  interface WebTransportDatagramDuplexStream {
    readable: ReadableStream<Uint8Array>;
    /** Legacy spec API (Chrome/Firefox). Absent in Safari — use createWritable there. */
    writable?: WritableStream<Uint8Array>;
    /** Current spec API (Safari; Chrome behind flag). */
    createWritable?: () => WritableStream<Uint8Array>;
    maxDatagramSize: number;
    incomingMaxAge: number | null;
    outgoingMaxAge: number | null;
    incomingHighWaterMark: number;
    outgoingHighWaterMark: number;
  }

  interface WebTransportBidirectionalStream {
    readable: ReadableStream<Uint8Array>;
    writable: WritableStream<Uint8Array>;
  }

  type WebTransportReceiveStream = ReadableStream<Uint8Array>;
  type WebTransportSendStream = WritableStream<Uint8Array>;

  class WebTransport {
    constructor(url: string, options?: WebTransportOptions);

    readonly ready: Promise<void>;
    readonly closed: Promise<WebTransportCloseInfo>;
    readonly draining: Promise<undefined>;

    readonly datagrams: WebTransportDatagramDuplexStream;
    readonly incomingBidirectionalStreams: ReadableStream<WebTransportBidirectionalStream>;
    readonly incomingUnidirectionalStreams: ReadableStream<WebTransportReceiveStream>;

    close(closeInfo?: WebTransportCloseInfo): void;
    createBidirectionalStream(): Promise<WebTransportBidirectionalStream>;
    createUnidirectionalStream(): Promise<WebTransportSendStream>;
  }
}

export {};
