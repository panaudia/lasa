package server

import (
	"github.com/panaudia/lasa/connect"
)

// Backend is the SPI the embedder implements (plan/server-spi.md). One
// Space = one space id = one engine. No method carries a space id: a
// multi-space embedder instantiates one Backend per space.
type Backend interface {
	// Admit applies admission policy (collisions, capacity, blocking) to
	// a parsed, verified, resolved connection. It MAY block until an
	// evicted predecessor's teardown has reached the engine — the
	// sweep-before-replacement-writes ordering is this contract.
	Admit(res *connect.Resolved) *connect.Rejection

	// EntityJoined is called once per admitted entity; the returned
	// handler receives that entity's ingress for the life of the
	// connection. An error tears the connection down.
	EntityJoined(clientID string, e connect.ResolvedEntity) (EntityHandler, error)

	// EntityLeft is called at departure, after the engine sweep was
	// enqueued, once per entity.
	EntityLeft(clientID, entityID string)

	// StartSink starts rendering an entity's sink in the given format
	// (subscription is the only lifecycle signal). The embedder pushes
	// frames into w until Stop. An error rejects the subscribe.
	StartSink(entityID, format string, w SinkWriter) (SinkSession, error)
}

// EntityHandler receives one entity's ingress. WritePacket delivers one
// mono-object datagram payload RAW, synchronously, on the transport's
// receive goroutine, in arrival order per session; seq is the MoQ group
// id. Ownership of payload passes to the handler (it aliases the received
// datagram's buffer). The handler must hand off and return — the shell
// never buffers ingress, and a slow handler backs up into the QUIC
// stack's receive queue, which drops.
type EntityHandler interface {
	WritePacket(seq uint64, payload []byte)
}

// SinkWriter is the push surface for one (entity, format) render.
// WriteFrame frames and sends synchronously on the caller's goroutine —
// one frame per call at the 5 ms cadence, sampleTS naming the frame's
// first sample on the embedder's sample clock (samples; the embedder is
// wall-clock-free — the shell alone maps samples to epoch time via
// Config.SampleEpoch when emitting the informational §5.1 anchors).
// WriteFrame is not safe for concurrent use; one render loop drives one
// sink.
type SinkWriter interface {
	WriteFrame(frame []byte, sampleTS uint64) error
}

// SinkSession is an active render, stopped when the last subscriber
// unsubscribes or the serving connection ends.
type SinkSession interface{ Stop() }
