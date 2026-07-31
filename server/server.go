// Package server is the LASA transport shell: a self-contained MoQ server
// (over the forked moqtransport) that authenticates Connection Configs,
// routes tracks, runs the state engine and presence machinery, and drives
// an embedder through the Backend SPI (plan/server-spi.md).
package server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/panaudia/moqtransport"
	"github.com/panaudia/moqtransport/quicmoq"
	"github.com/quic-go/quic-go"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/presence"
	"github.com/panaudia/lasa/state"
	"github.com/panaudia/lasa/wire"
)

// ALPN is the MoQ version this server speaks.
const ALPN = "moqt-16"

// Config configures a Server (one space).
type Config struct {
	SpaceID string
	Backend Backend

	// SampleEpoch maps a sink frame's sample timestamp to unix-epoch
	// microseconds for the informational §5.1 wire anchors. The server's
	// one wall-clock read lives behind this closure (the embedder and
	// the audio paths are wall-clock-free; see
	// plan/panaudia-server-design.md §1). nil disables anchor emission.
	SampleEpoch func(sampleTS uint64) uint64

	// TicketKeys verifies tickets; AllowUnticketed admits configs without
	// one (dev/demo/closed-network mode). At least one must be usable.
	TicketKeys      []ed25519.PublicKey
	AllowUnticketed bool

	// AdHocPermitted decides whether a ticketed connection may carry
	// ad-hoc entity definitions — profile policy over the ticket's roles
	// (base profile: admin and presenter). nil means never.
	AdHocPermitted func(roles []string) bool

	// RulesFor computes a connection's merged state-access rules — the
	// union of its roles' expanded rules and its ticket permissions.
	// Required.
	RulesFor func(res *connect.Resolved) state.Rules

	// AmbiOrder is the space's global ambisonic order (deviation 1 of
	// implementation-plan.md): 0 offers binaural only; 2 or 3 offer
	// exactly that ambi format alongside binaural.
	AmbiOrder int

	// Engine configures the state engine.
	Engine state.Config

	// PresenceInterval is the delta poll cadence (default 50 ms);
	// KeyframeInterval the keyframe cadence (default 1 s).
	PresenceInterval time.Duration
	KeyframeInterval time.Duration

	// SetupTimeout bounds the MoQ handshake (default 10 s).
	SetupTimeout time.Duration
}

// Server is one LASA space. All space-scoped machinery lives here — a
// future multi-space listener would hold several.
type Server struct {
	cfg      Config
	engine   *state.Engine
	sinks    *sinkManager
	presence *presence.Author
	closed   chan struct{}
}

// New validates cfg, starts the engine and the presence loop.
func New(cfg Config) (*Server, error) {
	if cfg.SpaceID == "" || cfg.Backend == nil || cfg.RulesFor == nil {
		return nil, errors.New("server: SpaceID, Backend and RulesFor are required")
	}
	if len(cfg.TicketKeys) == 0 && !cfg.AllowUnticketed {
		return nil, errors.New("server: no ticket keys and unticketed connections not allowed")
	}
	if cfg.AmbiOrder != 0 && cfg.AmbiOrder != 2 && cfg.AmbiOrder != 3 {
		return nil, fmt.Errorf("server: invalid AmbiOrder %d", cfg.AmbiOrder)
	}
	if cfg.PresenceInterval == 0 {
		cfg.PresenceInterval = 50 * time.Millisecond
	}
	if cfg.KeyframeInterval == 0 {
		cfg.KeyframeInterval = time.Second
	}
	if cfg.SetupTimeout == 0 {
		cfg.SetupTimeout = 10 * time.Second
	}
	s := &Server{
		cfg:    cfg,
		engine: state.New(cfg.Engine),
		sinks:  newSinkManager(cfg.Backend, cfg.SampleEpoch),
		closed: make(chan struct{}),
	}
	s.presence = presence.NewAuthor(cfg.Backend.Presence, cfg.KeyframeInterval)
	go s.presence.Run(cfg.PresenceInterval, s.closed)
	go s.tickEngine()
	return s, nil
}

// Engine exposes the space's state engine to the embedder (profile
// consumption: observer subscriptions and server-authored writes).
func (s *Server) Engine() *state.Engine { return s.engine }

// Close stops the space.
func (s *Server) Close() {
	close(s.closed)
	s.engine.Close()
}

func (s *Server) tickEngine() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = s.engine.Tick()
		case <-s.closed:
			return
		}
	}
}

// ServeQUIC accepts raw-QUIC MoQ connections until the listener closes.
func (s *Server) ServeQUIC(l *quic.Listener) error {
	for {
		conn, err := l.Accept(context.Background())
		if err != nil {
			return err
		}
		go func() { _ = s.ServeConn(conn.Context(), quicmoq.NewServer(conn)) }()
	}
}

// ServeConn runs one MoQ connection to completion. The conn is any
// moqtransport.Connection (quicmoq for raw QUIC, webtransportmoq for WT);
// transportCtx must end when the underlying transport dies — it is the
// guaranteed teardown signal (invariant L5).
func (s *Server) ServeConn(transportCtx context.Context, conn moqtransport.Connection) error {
	cs := newConnSession(s)
	go func() {
		select {
		case <-transportCtx.Done():
			cs.cancel(context.Cause(transportCtx))
		case <-cs.ctx.Done():
		}
	}()
	moqSession := &moqtransport.Session{
		InitialMaxRequestID:    1024,
		ClientSetupHandler:     cs.onClientSetup,
		DatagramReceiveHandler: cs.onDatagram,
		Handler:                moqtransport.HandlerFunc(cs.handleMessage),
		SubscribeHandler:       moqtransport.SubscribeHandlerFunc(cs.handleSubscribe),
	}
	cs.moq = moqSession

	setupCtx, cancel := context.WithTimeout(context.Background(), s.cfg.SetupTimeout)
	err := moqSession.RunContext(setupCtx, conn)
	cancel()
	if err != nil {
		cs.abortAdmission()
		return err
	}
	defer cs.teardown()
	if err := cs.start(); err != nil {
		return err
	}
	<-cs.ctx.Done()
	return context.Cause(cs.ctx)
}

// verifyTicket runs ticket verification for a config, honouring the
// unticketed mode.
func (s *Server) verifyTicket(cfg *connect.ConnectionConfig) (*connect.Ticket, *connect.Rejection) {
	if cfg.Ticket == "" {
		if !s.cfg.AllowUnticketed {
			return nil, &connect.Rejection{Code: wire.ErrCodeInvalidTicket, Reason: "ticket required"}
		}
		return nil, nil
	}
	v := &connect.Verifier{SpaceID: s.cfg.SpaceID, Keys: s.cfg.TicketKeys}
	return v.Verify(cfg.Ticket)
}

// sinkFormatOffered applies deviation 1: binaural always; the configured
// ambi order when set.
func (s *Server) sinkFormatOffered(format string) bool {
	switch format {
	case "binaural":
		return true
	case "ambi2":
		return s.cfg.AmbiOrder == 2
	case "ambi3":
		return s.cfg.AmbiOrder == 3
	}
	return false
}
