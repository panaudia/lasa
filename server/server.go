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
	"sync/atomic"
	"time"

	"github.com/panaudia/moqtransport"
	"github.com/panaudia/moqtransport/quicmoq"
	"github.com/quic-go/quic-go"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/ident"
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

	// ValidRoles decides whether a ticket's role names are acceptable —
	// profile vocabulary over an opaque core (base profile §2: a role
	// name outside the vocabulary rejects the connection as an invalid
	// ticket, 0x1A5A01). nil accepts any role name.
	ValidRoles func(roles []string) bool

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

	// AmbiOrder is the ambisonic order the space renders internally —
	// its render bus order, range 2–5; 0 defaults to 2. The sink
	// offering derives from it (lasa-core.md §3): binaural and ambi2
	// always, ambi3 when the order is ≥ 3 (the wire formats top out at
	// third order). Embedders set it from their render bus.
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
	closeOnce sync.Once
	stopping  atomic.Bool // Shutdown began: new registrations are severed too

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
	// Likewise a session that completed its handshake after Shutdown
	// began: sever it with the shutdown code rather than let it live
	// on into a closed engine.
	if s.stopping.Load() {
		cs.cancel(errShuttingDown)
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
	if !ident.ValidIdentifier(cfg.SpaceID) {
		// Fail at config time: a space id outside the §2 grammar could
		// never match a schema-valid ticket aud, silently making every
		// ticket unusable at runtime.
		return nil, fmt.Errorf("server: SpaceID %q is not a valid identifier (lasa-core.md §2: [a-z0-9-]{1,128})", cfg.SpaceID)
	}
	if len(cfg.TicketKeys) == 0 && !cfg.AllowUnticketed {
		return nil, errors.New("server: no ticket keys and unticketed connections not allowed")
	}
	if len(cfg.TicketKeys) > 0 && cfg.AllowUnticketed {
		return nil, errors.New("server: a space is ticketed XOR unticketed — ticket keys with AllowUnticketed is mixed mode, which does not exist (lasa-core.md §4.4)")
	}
	if cfg.AmbiOrder == 0 {
		cfg.AmbiOrder = 2
	}
	if cfg.AmbiOrder < 2 || cfg.AmbiOrder > 5 {
		return nil, fmt.Errorf("server: invalid AmbiOrder %d (the space's internal render order; valid range 2–5)", cfg.AmbiOrder)
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

// Close stops the space. Live sessions are not told; use Shutdown for
// an orderly stop. Idempotent.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.engine.Close()
	})
}

// Shutdown stops the space deliberately: every live session is closed
// with ErrCodeShuttingDown (so clients can tell a stop from a dead
// network), their teardowns are awaited — each runs the normal
// departure funnel, EntityLeft included — until ctx expires, and then
// the space is closed. The caller stops accepting new transports
// before or alongside this; a handshake that completes meanwhile is
// severed the same way.
func (s *Server) Shutdown(ctx context.Context) error {
	s.stopping.Store(true)
	s.liveMu.Lock()
	live := make([]*connSession, 0, len(s.live))
	for cs := range s.live {
		live = append(live, cs)
	}
	s.liveMu.Unlock()
	for _, cs := range live {
		cs.cancel(errShuttingDown)
	}
	var err error
	for _, cs := range live {
		select {
		case <-cs.torndown:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			break
		}
	}
	s.Close()
	return err
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
	cs.raw = conn
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

// sinkFormatOffered: binaural and ambi2 always (lasa-core.md §3);
// ambi3 when the space renders at third order or higher.
func (s *Server) sinkFormatOffered(format string) bool {
	switch format {
	case "binaural", "ambi2":
		return true
	case "ambi3":
		return s.cfg.AmbiOrder >= 3
	}
	return false
}
