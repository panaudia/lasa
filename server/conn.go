package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/panaudia/moqtransport"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/ident"
	"github.com/panaudia/lasa/profile/base"
	"github.com/panaudia/lasa/state"
	"github.com/panaudia/lasa/wire"
)

var (
	errSessionEnded = errors.New("server: session ended")
	errBlockedLive  = errors.New("server: blocked by moderation")
	errEvicted      = errors.New("server: evicted by a colliding admission")
)

// connSession is one client connection: admission state, ingress routing,
// and per-connection track plumbing.
type connSession struct {
	srv    *Server
	moq    *moqtransport.Session
	ctx    context.Context
	cancel context.CancelCauseFunc

	// Written during onClientSetup (before any concurrent access).
	resolved *connect.Resolved
	binding  *ident.Binding
	econn    *state.Conn

	// transport is the connection's transport kind ("quic" |
	// "webtransport"), set by ServeConn before the session runs and
	// stamped onto Resolved for the backend (a route signal — browsers
	// can only be webtransport).
	transport string

	// handlers maps track alias → EntityHandler; copy-on-write so the
	// datagram hot path is a single atomic load + map read.
	handlers atomic.Pointer[map[uint64]EntityHandler]

	mu          sync.Mutex
	entityByID  map[string]EntityHandler
	stateSubSeq uint64
	torn        bool
}

func newConnSession(s *Server) *connSession {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &connSession{
		srv:        s,
		ctx:        ctx,
		cancel:     cancel,
		entityByID: map[string]EntityHandler{},
	}
}

// onClientSetup authenticates the connection from its one Connection
// Config, before SERVER_SETUP (lasa-core.md §4.1). Any rejection closes
// the session with the §4.6 code via SetupRejection.
func (cs *connSession) onClientSetup(params moqtransport.KVPList) error {
	var raw []byte
	for _, p := range params {
		if p.Type == wire.SetupParamConnectionConfig {
			raw = p.ValueBytes
			break
		}
	}
	if raw == nil {
		return moqtransport.SetupRejection{Code: uint64(wire.ErrCodeMalformedConfig), Reason: "missing connection config"}
	}
	cfg, rej := connect.ParseConnectionConfig(raw)
	if rej != nil {
		return setupRejection(rej)
	}
	ticket, rej := cs.srv.verifyTicket(cfg)
	if rej != nil {
		return setupRejection(rej)
	}
	adHoc := false
	if ticket != nil && cs.srv.cfg.AdHocPermitted != nil {
		adHoc = cs.srv.cfg.AdHocPermitted(ticket.Roles)
	}
	res, rej := connect.Resolve(cfg, ticket, adHoc)
	if rej != nil {
		return setupRejection(rej)
	}
	res.Transport = cs.transport
	// Moderation gate BEFORE Admit (design §7, the KickGate lesson): a
	// blocked id's reconnect can never reach eviction logic.
	if rej := cs.srv.gate.check(res); rej != nil {
		return setupRejection(rej)
	}
	if rej := cs.srv.cfg.Backend.Admit(res); rej != nil {
		return setupRejection(rej)
	}
	econn, err := cs.srv.engine.Register(res.ClientID, cs.srv.cfg.RulesFor(res))
	if err != nil {
		return moqtransport.SetupRejection{Code: uint64(wire.ErrCodeMalformedConfig), Reason: "engine unavailable"}
	}
	entities := make([]string, len(res.Entities))
	for i, e := range res.Entities {
		entities[i] = e.ID
	}
	cs.resolved = res
	cs.econn = econn
	cs.binding = &ident.Binding{
		Space:       cs.srv.cfg.SpaceID,
		Client:      res.ClientID,
		Entities:    entities,
		SinkAll:     cs.srv.cfg.SinkAll != nil && cs.srv.cfg.SinkAll(res.Roles),
		AmbiFormats: cs.srv.cfg.AmbiFormats != nil && cs.srv.cfg.AmbiFormats(res.Roles),
	}
	return nil
}

func setupRejection(r *connect.Rejection) error {
	return moqtransport.SetupRejection{Code: uint64(r.Code), Reason: r.Reason}
}

// abortAdmission cleans up when the handshake fails after admission.
func (cs *connSession) abortAdmission() {
	if cs.econn != nil {
		_ = cs.econn.Close()
	}
}

// start runs post-handshake admission actions: base-profile writes,
// EntityJoined, and the subscriptions to the client's source tracks.
func (cs *connSession) start() error {
	for i := range cs.resolved.Entities {
		e := &cs.resolved.Entities[i]
		g, err := base.AdmissionGroup(e)
		if err != nil {
			return fmt.Errorf("admission ops for %s: %w", e.ID, err)
		}
		if err := cs.srv.engine.Write(g); err != nil {
			return err
		}
		h, err := cs.srv.cfg.Backend.EntityJoined(cs.resolved.ClientID, *e)
		if err != nil {
			return fmt.Errorf("EntityJoined(%s): %w", e.ID, err)
		}
		// The entity's declared sink redundancy (§4.2: the client's
		// request that the space emit repeats on its sink track),
		// filtered through deployment policy. Static for the session;
		// the entity's renders read it at start.
		if e.Redundancy > 0 && (cs.srv.cfg.SinkRedundancy == nil || cs.srv.cfg.SinkRedundancy(e)) {
			cs.srv.sinks.setDecl(e.ID, uint8(e.Redundancy))
		}
		cs.mu.Lock()
		cs.entityByID[e.ID] = h
		cs.mu.Unlock()
		go cs.subscribeEntitySource(e.ID)
	}
	go cs.subscribeStateSource()
	cs.srv.registerLive(cs) // the moderation gate's sever surface (§7)
	return nil
}

// subscribeEntitySource subscribes the entity's mono-object source and
// registers its track alias for the datagram fast path.
func (cs *connSession) subscribeEntitySource(entityID string) {
	ns := ident.EntityNamespace(cs.srv.cfg.SpaceID, entityID, ident.DirectionSource)
	rt, err := cs.moq.Subscribe(cs.ctx, ns.Tuple(), ident.TrackMonoObject)
	if err != nil {
		cs.cancel(fmt.Errorf("subscribe %s source: %w", entityID, err))
		return
	}
	alias, ok := rt.TrackAlias()
	if !ok {
		cs.cancel(fmt.Errorf("subscribe %s source: no track alias", entityID))
		return
	}
	cs.mu.Lock()
	h := cs.entityByID[entityID]
	old := cs.handlers.Load()
	next := make(map[uint64]EntityHandler, 8)
	if old != nil {
		for k, v := range *old {
			next[k] = v
		}
	}
	next[alias] = h
	cs.handlers.Store(&next)
	cs.mu.Unlock()
}

// subscribeStateSource subscribes the client's state source track and
// pumps its ops into the engine. This subscription lives for the whole
// session; its end is the session-teardown signal.
func (cs *connSession) subscribeStateSource() {
	ns := ident.ClientNamespace(cs.srv.cfg.SpaceID, cs.resolved.ClientID, ident.DirectionSource)
	rt, err := cs.moq.Subscribe(cs.ctx, ns.Tuple(), ident.TrackState)
	if err != nil {
		cs.cancel(fmt.Errorf("subscribe state source: %w", err))
		return
	}
	for {
		obj, err := rt.ReadObject(cs.ctx)
		if err != nil {
			cs.cancel(err)
			return
		}
		msg, err := wire.ParseStateMessage(obj.Payload, wire.Upstream)
		if err != nil {
			continue // malformed op: drop, per the silent-failure posture
		}
		if err := cs.econn.Write(msg); err != nil {
			cs.cancel(err)
			return
		}
	}
}

// onDatagram is the ingress hot path: one atomic load, one map read, one
// synchronous WritePacket. The server consumes every datagram — the only
// datagrams a client sends are entity-source audio.
func (cs *connSession) onDatagram(trackAlias, groupID, _ uint64, payload []byte) bool {
	if m := cs.handlers.Load(); m != nil {
		if h, ok := (*m)[trackAlias]; ok {
			h.WritePacket(groupID, payload)
		}
	}
	return true
}

// handleMessage handles announces (mirror of the subscribe matrix: a
// client may announce only its own source namespaces) and accepts
// everything that needs no policy.
func (cs *connSession) handleMessage(rw moqtransport.ResponseWriter, msg *moqtransport.Message) {
	switch msg.Method {
	case moqtransport.MessageAnnounce:
		ns, err := ident.ParseNamespace(msg.Namespace)
		if err != nil || cs.binding == nil || !cs.binding.MayAnnounce(ns) {
			_ = rw.Reject(uint64(moqtransport.ErrorCodeUnauthorized), "announce outside binding")
			return
		}
		_ = rw.Accept()
	default:
		_ = rw.Accept()
	}
}

// handleSubscribe enforces §3.1 at one point, against the binding, then
// routes to the state, sink, or presence machinery.
func (cs *connSession) handleSubscribe(w *moqtransport.SubscribeResponseWriter, m *moqtransport.SubscribeMessage) {
	ns, err := ident.ParseNamespace(m.Namespace)
	if err != nil {
		_ = w.Reject(moqtransport.ErrorCodeSubscribeTrackDoesNotExist, "unknown namespace")
		return
	}
	if cs.binding == nil || !cs.binding.MaySubscribe(ns, m.Track) {
		_ = w.Reject(moqtransport.ErrorCodeSubscribeUnauthorized, "outside binding")
		return
	}
	switch ns.Kind {
	case ident.KindClient:
		cs.handleStateSubscribe(w, m)
	case ident.KindEntity:
		if !cs.srv.sinkFormatOffered(m.Track) {
			_ = w.Reject(moqtransport.ErrorCodeSubscribeTrackDoesNotExist, "format not offered by this space")
			return
		}
		r, err := cs.srv.sinks.ensure(ns.ID, m.Track)
		if err != nil {
			_ = w.Reject(moqtransport.ErrorCodeSubscribeInternal, "render unavailable")
			return
		}
		if err := w.Accept(); err != nil {
			return
		}
		cs.srv.sinks.addPublisher(r, w, cs)
	case ident.KindPresence:
		if err := w.Accept(); err != nil {
			return
		}
		cs.srv.presence.AddSender(cs, func(group uint64, payload []byte) {
			_ = w.SendDatagram(moqtransport.Object{
				GroupID:              group,
				ObjectID:             0,
				ForwardingPreference: moqtransport.ObjectForwardingPreferenceDatagram,
				Payload:              payload,
			})
		})
	}
}

// handleStateSubscribe attaches the state subscription through the gate
// and starts the two delivery lanes (subgroup 0 live, subgroup 1
// catch-up) on a fresh MoQ group per subscription instance.
func (cs *connSession) handleStateSubscribe(w *moqtransport.SubscribeResponseWriter, m *moqtransport.SubscribeMessage) {
	var raw []byte
	for _, p := range m.Parameters {
		if p.Type == wire.SubscribeParamState {
			raw = p.ValueBytes
			break
		}
	}
	if raw == nil {
		_ = w.Reject(moqtransport.ErrorCodeSubscribeNotSupported, "missing state subscribe parameter")
		return
	}
	params, err := wire.ParseSubscribeParams(raw)
	if err != nil {
		_ = w.Reject(moqtransport.ErrorCodeSubscribeNotSupported, "malformed state subscribe parameter")
		return
	}
	sub, err := cs.srv.engine.Attach(cs.econn, params.Prefixes, params.Cursor)
	if err != nil {
		_ = w.Reject(moqtransport.ErrorCodeSubscribeInternal, "attach failed")
		return
	}
	cs.mu.Lock()
	group := cs.stateSubSeq
	cs.stateSubSeq++
	cs.mu.Unlock()
	if err := w.Accept(); err != nil {
		cs.srv.engine.Detach(sub)
		return
	}
	// Open both lanes here, serially: subgroup 0 live, subgroup 1
	// catch-up, each its own QUIC stream (lasa-core.md §6.7).
	liveSG, err := w.OpenSubgroup(group, 0, 0)
	if err != nil {
		cs.srv.engine.Detach(sub)
		return
	}
	catchSG, err := w.OpenSubgroup(group, 1, 0)
	if err != nil {
		cs.srv.engine.Detach(sub)
		return
	}
	go cs.deliverCatchUp(catchSG, sub)
	go cs.deliverLive(w, liveSG, sub)
}

func (cs *connSession) deliverCatchUp(sg *moqtransport.Subgroup, sub *state.Subscription) {
	defer sg.Close()
	var buf []byte
	var objectID uint64
	for {
		m, err := sub.CatchUpNext(cs.ctx)
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		if buf, err = wire.AppendStateMessage(buf[:0], m, wire.Downstream); err != nil {
			return
		}
		if _, err := sg.WriteObject(objectID, buf); err != nil {
			return
		}
		objectID++
	}
}

func (cs *connSession) deliverLive(w *moqtransport.SubscribeResponseWriter, sg *moqtransport.Subgroup, sub *state.Subscription) {
	defer sg.Close()
	var buf []byte
	var objectID uint64
	for {
		m, err := sub.LiveNext(cs.ctx)
		if errors.Is(err, state.ErrLagged) {
			// Native MoQ termination: the client re-subscribes through the
			// gate. Subscription-object only — never connection lifecycle.
			_ = w.CloseWithError(uint64(moqtransport.ErrorCodeSubscribeDoneTooFarBehind), "state subscription lagged")
			cs.srv.engine.Detach(sub)
			return
		}
		if err != nil {
			return
		}
		if buf, err = wire.AppendStateMessage(buf[:0], m, wire.Downstream); err != nil {
			return
		}
		if _, err := sg.WriteObject(objectID, buf); err != nil {
			return
		}
		objectID++
	}
}

// teardown is the connection's single departure funnel: engine sweep,
// backend notification, machinery cleanup. Idempotent.
func (cs *connSession) teardown() {
	cs.mu.Lock()
	if cs.torn {
		cs.mu.Unlock()
		return
	}
	cs.torn = true
	cs.mu.Unlock()

	cs.srv.unregisterLive(cs)
	cs.cancel(errSessionEnded)
	if cs.resolved != nil {
		scopes := make([]string, len(cs.resolved.Entities))
		for i, e := range cs.resolved.Entities {
			scopes[i] = base.SweepScope(e.ID)
		}
		_ = cs.econn.Close(scopes...) // generation close + marker-first sweep (L1)
		for _, e := range cs.resolved.Entities {
			cs.srv.cfg.Backend.EntityLeft(cs.resolved.ClientID, e.ID)
			// Roster hygiene the embedder cannot forget (design §6).
			cs.srv.collector.Remove(e.ID)
			// After EntityLeft (see dropEntity): end the departed
			// entity's sink tracks.
			cs.srv.sinks.clearDecl(e.ID)
			cs.srv.sinks.dropEntity(e.ID)
		}
	}
	cs.srv.sinks.dropConn(cs)
	cs.srv.presence.RemoveSender(cs)
	_ = cs.moq.Close()
}
