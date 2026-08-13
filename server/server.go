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
	"sync"
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

	// TicketKeys verifies tickets; AllowUnticketed runs the space
	// unticketed (dev/demo/closed-network mode). A space is ticketed
	// XOR unticketed (lasa-core.md §4.4): exactly one must be set —
	// New rejects both and neither, so no embedder can build a mixed
	// mode, and an unticketed space rejects any presented ticket.
	TicketKeys      []ed25519.PublicKey
	AllowUnticketed bool

	// Capacity caps live admitted entities space-wide; 0 = unlimited.
	// Adjudicated with the admission at the state engine, so every
	// admission refusal issues from one place (id-capabilities.md).
	Capacity int

	// AdHocPermitted decides whether a ticketed connection may carry
	// ad-hoc entity definitions — profile policy over the ticket's roles
	// (base profile: admin and presenter). nil means never.
	AdHocPermitted func(roles []string) bool

	// SinkAll decides whether a connection may subscribe to any entity's
	// sink tracks, beyond its own — profile policy over the ticket's
	// roles (base profile: admin and producer). nil means never;
	// unticketed connections carry no roles and never qualify.
	SinkAll func(roles []string) bool

	// AmbiFormats decides whether a connection may subscribe the
	// ambisonic sink formats — profile policy over the ticket's roles
	// (base profile: admin and producer). nil means never.
	AmbiFormats func(roles []string) bool

	// SinkRedundancy decides whether an entity's declared sink
	// redundancy (lasa-core.md §4.2: the client's request that the
	// space emit repeats on its sink track, which the space SHOULD
	// honour within deployment policy) is honoured — the §4.2 policy
	// hook for e.g. ad-hoc entities claiming egress bandwidth.
	// Refusing = simply not emitting; the declaration is a request,
	// not a MUST. nil honours every declaration.
	SinkRedundancy func(e *connect.ResolvedEntity) bool

	// RulesFor computes a connection's merged state-access rules — the
	// union of its roles' expanded rules and its ticket permissions.
	// Required.
	RulesFor func(res *connect.Resolved) state.Rules

	// AmbiOrder is the highest ambisonic sink order the space offers
	// (2026-08-04, superseding deviation 1's exactly-one-format rule):
	// 0 offers binaural only; N ≥ 2 offers every ambi format UP TO N
	// alongside binaural (ambi2 at N=2; ambi2+ambi3 at N=3). Embedders
	// derive it from their render bus order.
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
	cfg       Config
	engine    *state.Engine
	sinks     *sinkManager
	presence  *presence.Author
	collector *presence.Collector
	gate      *moderationGate
	closed    chan struct{}

	// Live-session registry: the moderation gate's sever surface.
	liveMu sync.Mutex
	live   map[*connSession]liveInfo
}

type liveInfo struct {
	clientID string
	roles    []string
}

// registerLive / unregisterLive bracket a session's post-handshake
// life; severMatching cancels every session the predicate selects
// (teardown then runs on each session's own goroutine — the normal
// funnel; the embedder sees only EntityLeft).
func (s *Server) registerLive(cs *connSession) {
	s.liveMu.Lock()
	s.live[cs] = liveInfo{clientID: cs.resolved.ClientID, roles: cs.resolved.Roles}
	s.liveMu.Unlock()
	// Close the admission→registration window: a block that landed
	// between the gate's connect check and this registration would see
	// an empty registry and sever nothing — so re-check now that we are
	// visible. Either ordering catches it; no window remains.
	if s.gate.check(cs.resolved) != nil {
		cs.cancel(errBlockedLive)
	}
}

func (s *Server) unregisterLive(cs *connSession) {
	s.liveMu.Lock()
	delete(s.live, cs)
	s.liveMu.Unlock()
}

func (s *Server) severMatching(cause error, match func(clientID string, roles []string) bool) {
	s.liveMu.Lock()
	var hit []*connSession
	for cs, info := range s.live {
		if match(info.clientID, info.roles) {
			hit = append(hit, cs)
		}
	}
	s.liveMu.Unlock()
	for _, cs := range hit {
		cs.cancel(cause)
	}
}

// severSuperseded cancels every live session carrying one of the
// superseded client ids (the engine's Admit verdict) and returns them
// so the successor can wait on their teardown — the mixer's
// Remove-before-Add ordering, shell-internal since the SPI's
// Admit-may-block contract was deleted (identity-implementation.md
// batch C). self is excluded: on a same-client supersession the
// successor is already registered under the same id. Sessions still
// mid-handshake are not here; they find their closed generation at
// registration (conn.go start).
func (s *Server) severSuperseded(self *connSession, clientIDs []string) []*connSession {
	if len(clientIDs) == 0 {
		return nil
	}
	ids := map[string]bool{}
	for _, id := range clientIDs {
		ids[id] = true
	}
	s.liveMu.Lock()
	var hit []*connSession
	for cs, info := range s.live {
		if cs != self && ids[info.clientID] {
			hit = append(hit, cs)
		}
	}
	s.liveMu.Unlock()
	for _, cs := range hit {
		cs.cancel(errEvicted)
	}
	return hit
}

// New validates cfg, starts the engine and the presence loop.
func New(cfg Config) (*Server, error) {
	if cfg.SpaceID == "" || cfg.Backend == nil || cfg.RulesFor == nil {
		return nil, errors.New("server: SpaceID, Backend and RulesFor are required")
	}
	if len(cfg.TicketKeys) == 0 && !cfg.AllowUnticketed {
		return nil, errors.New("server: no ticket keys and unticketed connections not allowed")
	}
	if len(cfg.TicketKeys) > 0 && cfg.AllowUnticketed {
		return nil, errors.New("server: a space is ticketed XOR unticketed — ticket keys with AllowUnticketed is mixed mode, which does not exist (lasa-core.md §4.4)")
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
		live:   map[*connSession]liveInfo{},
	}
	gate, err := newModerationGate(s)
	if err != nil {
		return nil, err
	}
	s.gate = gate
	// Presence, the third singleton (design §6): the embedder feeds the
	// Collector (Register at entity join, UpdatePose from ingress); the
	// track Author is entry-subscriber #1; the Collector's pacer drives
	// sampling at PresenceInterval.
	s.collector = presence.NewCollector()
	s.presence = presence.NewAuthor(cfg.KeyframeInterval)
	s.collector.Subscribe(s.presence, s.presence.Tick)
	go s.collector.Run(cfg.PresenceInterval, s.closed)
	go s.tickEngine()
	return s, nil
}

// Engine exposes the space's state engine to the embedder (profile
// consumption: observer subscriptions and server-authored writes).
func (s *Server) Engine() *state.Engine { return s.engine }

// Presence exposes the space's presence collector to the embedder: the
// embedder Registers each entity (with its loudness getter) and pushes
// poses from its ingress path; further entry-level subscribers
// (recording, analytics, a cloud presence tier) attach here too. The
// shell removes entities from the roster at departure.
func (s *Server) Presence() *presence.Collector { return s.collector }

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
	cs.transport = conn.Protocol().String()
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

// verifyTicket enforces the space mode (ticketed XOR unticketed,
// lasa-core.md §4.4): a ticketed space requires a ticket, an unticketed
// space rejects one — a client presenting a ticket believes it is
// getting signed identity, and silently admitting it with claim-based
// identity would be a quiet degradation. Mode mismatch fails loudly.
func (s *Server) verifyTicket(cfg *connect.ConnectionConfig) (*connect.Ticket, *connect.Rejection) {
	if cfg.Ticket == "" {
		if !s.cfg.AllowUnticketed {
			return nil, &connect.Rejection{Code: wire.ErrCodeInvalidTicket, Reason: "ticket required"}
		}
		return nil, nil
	}
	if s.cfg.AllowUnticketed {
		return nil, &connect.Rejection{Code: wire.ErrCodeModeMismatch, Reason: "this space is unticketed; do not present a ticket"}
	}
	v := &connect.Verifier{SpaceID: s.cfg.SpaceID, Keys: s.cfg.TicketKeys}
	return v.Verify(cfg.Ticket)
}

// sinkFormatOffered: binaural always; ambi formats up to the configured
// AmbiOrder (the derived-offering rule that superseded deviation 1).
func (s *Server) sinkFormatOffered(format string) bool {
	switch format {
	case "binaural":
		return true
	case "ambi2":
		return s.cfg.AmbiOrder >= 2
	case "ambi3":
		return s.cfg.AmbiOrder >= 3
	}
	return false
}
