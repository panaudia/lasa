// Package client is the reference LASA client: it dials a space, presents
// the Connection Config in CLIENT_SETUP, publishes entity sources when the
// space subscribes to them, and subscribes sinks, state and presence. It
// doubles as the e2e test harness for the server package.
package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panaudia/moqtransport"
	"github.com/panaudia/moqtransport/quicmoq"
	"github.com/panaudia/moqtransport/webtransportmoq"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	webtransport "github.com/quic-go/webtransport-go"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/ident"
	"github.com/panaudia/lasa/state/clientstore"
	"github.com/panaudia/lasa/wire"
)

// ALPN is the MoQ version this client speaks.
const ALPN = "moqt-16"

// Config describes the connection.
type Config struct {
	SpaceID  string
	ClientID string
	Ticket   string           // optional
	Entities []connect.Entity // nil = absent; empty non-nil = observer
	Setups   []connect.Setup  // requires Ticket
	// Lockstep declares one frame counter across every entity source
	// track (lasa-core.md §4.2). The counter is the caller's: every
	// EntityPublisher must be written with the same seq per frame,
	// and a member with nothing to send writes a flags 0x00 packet.
	Lockstep bool

	TLS *tls.Config
	// Store, when set, backs the state subscription (SyncState).
	Store *clientstore.Store
}

// Client is one LASA connection.
type Client struct {
	cfg     Config
	session *moqtransport.Session
	ctx     context.Context
	cancel  context.CancelCauseFunc

	mu         sync.Mutex
	entityPubs map[string]*EntityPublisher
	entityCond *sync.Cond
	stateSG    *moqtransport.Subgroup
	stateObjID uint64

	// routes maps track alias → datagram consumer; copy-on-write for the
	// receive hot path.
	routes atomic.Pointer[map[uint64]func(seq uint64, payload []byte)]
}

// Dial connects over raw QUIC and completes the LASA handshake. A
// server rejection surfaces as a *quic.ApplicationError carrying the
// §4.6 code.
func Dial(ctx context.Context, addr string, cfg Config) (*Client, error) {
	tlsConf := cfg.TLS.Clone()
	if tlsConf == nil {
		tlsConf = &tls.Config{}
	}
	tlsConf.NextProtos = []string{ALPN}
	conn, err := quic.DialAddr(ctx, addr, tlsConf, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return nil, err
	}
	return start(ctx, cfg, quicmoq.NewClient(conn), nil)
}

// DialWebTransport connects over WebTransport — the browser transport;
// this Go side exists for tests and tooling ("identical over
// WebTransport and raw QUIC", lasa-core §4.1). urlStr is the upgrade
// endpoint, e.g. https://host:4443/lasa.
func DialWebTransport(ctx context.Context, urlStr string, cfg Config) (*Client, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	tlsConf := cfg.TLS.Clone()
	if tlsConf == nil {
		tlsConf = &tls.Config{}
	}
	tlsConf.NextProtos = []string{http3.NextProtoH3}
	qconn, err := quic.DialAddr(ctx, host, tlsConf, &quic.Config{
		EnableDatagrams:                  true,
		EnableStreamResetPartialDelivery: true, // WebTransport requirement
	})
	if err != nil {
		return nil, err
	}
	t := &webtransport.Transport{ApplicationProtocols: []string{ALPN}}
	cc, err := t.NewClientConn(qconn)
	if err != nil {
		_ = qconn.CloseWithError(0, "webtransport setup failed")
		return nil, err
	}
	_, sess, err := cc.Dial(ctx, urlStr, nil)
	if err != nil {
		_ = qconn.CloseWithError(0, "webtransport dial failed")
		return nil, err
	}
	closeReason := wtCloseReason(sess)
	c, err := start(ctx, cfg, webtransportmoq.NewClient(sess), closeReason)
	if err != nil {
		// A server rejection races its own teardown: the control-stream
		// FIN can reach the blocked MoQ read before the WT_CLOSE_SESSION
		// capsule is processed, so RunContext fails with a bare EOF. The
		// session's recorded close error is the authoritative §4.6
		// reason, so wait a grace window for the session to end and
		// prefer that error when it carries the coded SessionError
		// (correctness rides the code alone).
		select {
		case <-sess.Context().Done():
			var sessErr *webtransport.SessionError
			if errors.As(closeReason(), &sessErr) {
				return nil, fmt.Errorf("lasa: session closed by server: %w", sessErr)
			}
		case <-time.After(250 * time.Millisecond):
		}
		return nil, err
	}
	return c, nil
}

// wtCloseReason reports a WebTransport session's recorded close error:
// once the session is closed, webtransport-go returns it from every
// session operation, and when the peer closed with a code it is the
// *webtransport.SessionError carrying that code. This is the library's
// own way of surfacing the reason (the session context is merely
// cancelled). OpenStream is the cheapest such operation; the returned
// func is only ever called after the session context has ended, when
// the session is closed by construction, so it opens nothing.
func wtCloseReason(sess *webtransport.Session) func() error {
	return func() error {
		_, err := sess.OpenStream()
		return err
	}
}

// start runs the MoQ session over an established transport connection —
// the shared tail of both Dial variants. closeReason, when not nil,
// reports the transport's recorded close error after its context ends;
// it takes precedence over the context cause, which some transports
// (WebTransport) leave as a bare cancellation.
func start(ctx context.Context, cfg Config, mconn moqtransport.Connection, closeReason func() error) (*Client, error) {
	raw, err := configJSON(cfg)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithCancelCause(context.Background())
	c := &Client{
		cfg:        cfg,
		ctx:        cctx,
		cancel:     cancel,
		entityPubs: map[string]*EntityPublisher{},
	}
	c.entityCond = sync.NewCond(&c.mu)
	// The transport's context is the guaranteed death signal: when the
	// underlying connection dies (server gone, network cut), the
	// client's own context ends with the cause, observable via Done/Err.
	go func() {
		tctx := mconn.Context()
		select {
		case <-tctx.Done():
			cause := context.Cause(tctx)
			if closeReason != nil {
				if err := closeReason(); err != nil {
					cause = err
				}
			}
			cancel(cause)
		case <-cctx.Done():
		}
	}()
	c.session = &moqtransport.Session{
		InitialMaxRequestID: 1024,
		SetupParameters: moqtransport.KVPList{
			{Type: wire.SetupParamConnectionConfig, ValueBytes: raw},
		},
		SubscribeHandler:       moqtransport.SubscribeHandlerFunc(c.handleSubscribe),
		DatagramReceiveHandler: c.onDatagram,
		Handler: moqtransport.HandlerFunc(func(rw moqtransport.ResponseWriter, _ *moqtransport.Message) {
			_ = rw.Accept()
		}),
	}
	if err := c.session.RunContext(ctx, mconn); err != nil {
		cancel(err)
		return nil, err
	}
	return c, nil
}

// Done is closed when the connection ends — whether by Close or by the
// transport dying underneath (server gone, network cut). Err then
// reports the cause.
func (c *Client) Done() <-chan struct{} { return c.ctx.Done() }

// Err returns the reason the connection ended (nil while alive;
// context.Canceled after a local Close).
func (c *Client) Err() error {
	if c.ctx.Err() == nil {
		return nil
	}
	return context.Cause(c.ctx)
}

func configJSON(cfg Config) ([]byte, error) {
	m := map[string]any{"client_id": cfg.ClientID}
	if cfg.Ticket != "" {
		m["ticket"] = cfg.Ticket
	}
	if cfg.Entities != nil {
		m["entities"] = cfg.Entities
	}
	if cfg.Setups != nil {
		m["setups"] = cfg.Setups
	}
	if cfg.Lockstep {
		m["lockstep"] = true
	}
	return json.Marshal(m)
}

// Close ends the connection.
func (c *Client) Close() error {
	c.cancel(context.Canceled)
	return c.session.Close()
}

// handleSubscribe accepts the space's subscriptions to this client's own
// source tracks and wires up the publishers.
func (c *Client) handleSubscribe(w *moqtransport.SubscribeResponseWriter, m *moqtransport.SubscribeMessage) {
	ns, err := ident.ParseNamespace(m.Namespace)
	if err != nil || ns.Space != c.cfg.SpaceID || ns.Direction != ident.DirectionSource {
		_ = w.Reject(moqtransport.ErrorCodeSubscribeUnauthorized, "not a source of this client")
		return
	}
	switch {
	case ns.Kind == ident.KindEntity && m.Track == ident.TrackMonoObject:
		if err := w.Accept(); err != nil {
			return
		}
		// The entity's own declaration (§4.2) sets the uplink redundancy
		// offset — for ad-hoc entities it is right here in the config;
		// a ticket-defined entity's declaration is in the ticket, which
		// the client does not open, so it starts at 0 and the embedder
		// may SetRedundancy to what the ticket declares.
		pub := &EntityPublisher{w: w, declared: -1}
		for _, e := range c.cfg.Entities {
			if e.ID == ns.ID {
				pub.declared = e.Redundancy
				pub.red = NewUplinkRedundancy(e.Redundancy)
				break
			}
		}
		c.mu.Lock()
		c.entityPubs[ns.ID] = pub
		c.entityCond.Broadcast()
		c.mu.Unlock()
	case ns.Kind == ident.KindClient && ns.ID == c.cfg.ClientID && m.Track == ident.TrackState:
		if err := w.Accept(); err != nil {
			return
		}
		sg, err := w.OpenSubgroup(0, 0, 0)
		if err != nil {
			return
		}
		c.mu.Lock()
		c.stateSG = sg
		c.entityCond.Broadcast()
		c.mu.Unlock()
	default:
		_ = w.Reject(moqtransport.ErrorCodeSubscribeTrackDoesNotExist, "unknown source track")
	}
}

// Entity returns the entity's publisher, waiting until the space has
// subscribed the source track.
func (c *Client) Entity(ctx context.Context, id string) (*EntityPublisher, error) {
	return waitFor(ctx, c, func() *EntityPublisher { return c.entityPubs[id] })
}

func waitFor[T any](ctx context.Context, c *Client, get func() *T) (*T, error) {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.entityCond.Broadcast()
		case <-done:
		}
	}()
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if v := get(); v != nil {
			return v, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c.entityCond.Wait()
	}
}

// EntityPublisher publishes one entity's mono-object source. It emits
// uplink redundancy at the entity's declared offset (lasa-core.md
// §4.2/§5.1, see UplinkRedundancy); the caller keeps one gapless seq
// counter per entity across pose-only and audio packets so the ring
// sees every seq.
type EntityPublisher struct {
	w *moqtransport.SubscribeResponseWriter

	mu      sync.Mutex // serialises writes: the ring and scratch are shared state
	scratch []byte
	// declared is the entity's §4.2 redundancy declaration as known to
	// this client (−1 = not in cfg.Entities, i.e. ticket-defined).
	declared int
	red      *UplinkRedundancy
}

// ErrRedundancyBound reports a SetRedundancy above the entity's
// declaration (an offset beyond it would be a §5.1 sender violation).
var ErrRedundancyBound = errors.New("client: redundancy offset above the entity's declaration")

// Redundancy is the uplink redundancy offset in force (0 = none).
func (p *EntityPublisher) Redundancy() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return int(p.red.Offset())
}

// SetRedundancy changes the uplink redundancy offset. It may be lowered
// freely and raised only to the entity's declaration: the declaration
// is the bound the server provisioned for. For a ticket-defined entity
// the client cannot see the declaration, so any offset 0–7 is
// accepted and the embedder is trusted to stay within the ticket.
func (p *EntityPublisher) SetRedundancy(offset int) error {
	if offset < 0 || offset > 7 {
		return fmt.Errorf("client: redundancy offset %d outside 0-7", offset)
	}
	if p.declared >= 0 && offset > p.declared {
		return fmt.Errorf("%w: %d > %d", ErrRedundancyBound, offset, p.declared)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.red = NewUplinkRedundancy(offset)
	return nil
}

// WritePacket sends one already-encoded mono-object payload; seq is the
// MoQ group id (the frame sequence, +1 per packet, no gaps). This is
// the raw path: no redundancy is attached (frame with UplinkRedundancy
// yourself if you want it).
func (p *EntityPublisher) WritePacket(seq uint64, payload []byte) error {
	return p.w.SendDatagram(moqtransport.Object{
		GroupID:              seq,
		ObjectID:             0,
		ForwardingPreference: moqtransport.ObjectForwardingPreferenceDatagram,
		Payload:              payload,
	})
}

// WriteMonoObject encodes and sends a mono-object packet, reusing an
// internal scratch buffer. When the publisher's redundancy offset is
// above 0 it attaches the §5.1 repeat itself and replaces any
// pkt.Redundancy the caller set; at offset 0 pkt goes out as given.
// Safe for concurrent use; callers still own one gapless seq counter.
func (p *EntityPublisher) WriteMonoObject(seq uint64, pkt *wire.MonoObjectPacket) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.red.Attach(seq, pkt)
	var err error
	if p.scratch, err = wire.AppendMonoObject(p.scratch[:0], pkt); err != nil {
		return err
	}
	return p.WritePacket(seq, p.scratch)
}

// WriteState publishes client-authored state ops on the state source
// track, waiting for the space's subscription if necessary.
func (c *Client) WriteState(ctx context.Context, msgs ...wire.StateMessage) error {
	if _, err := waitFor(ctx, c, func() *moqtransport.Subgroup { return c.stateSG }); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var buf []byte
	for _, m := range msgs {
		var err error
		if buf, err = wire.AppendStateMessage(buf[:0], m, wire.Upstream); err != nil {
			return err
		}
		if _, err := c.stateSG.WriteObject(c.stateObjID, buf); err != nil {
			return err
		}
		c.stateObjID++
	}
	return nil
}

// SubscribeSink subscribes an entity sink; h is called from the datagram
// receive path with each packet (seq = frame sequence, payload raw —
// parse with wire.ParseSink). Close the returned track to unsubscribe.
func (c *Client) SubscribeSink(ctx context.Context, entityID, format string, h func(seq uint64, payload []byte)) (*moqtransport.RemoteTrack, error) {
	ns := ident.EntityNamespace(c.cfg.SpaceID, entityID, ident.DirectionSink)
	rt, err := c.session.Subscribe(ctx, ns.Tuple(), format)
	if err != nil {
		return nil, err
	}
	if alias, ok := rt.TrackAlias(); ok {
		c.addRoute(alias, h)
	}
	return rt, nil
}

// SubscribePresence subscribes the space's presence track; h receives
// each packet's datagram group-id and the parsed *wire.PresenceDelta /
// *wire.PresenceKeyframe. The group-id is part of the §7 receive
// contract (per-index latest-wins); most consumers want
// SubscribeRoster, which applies the §7 rules once and maintains the
// roster.
func (c *Client) SubscribePresence(ctx context.Context, h func(groupID uint64, msg any)) (*moqtransport.RemoteTrack, error) {
	ns := ident.PresenceNamespace(c.cfg.SpaceID)
	rt, err := c.session.Subscribe(ctx, ns.Tuple(), ident.TrackPresence)
	if err != nil {
		return nil, err
	}
	if alias, ok := rt.TrackAlias(); ok {
		c.addRoute(alias, func(groupID uint64, payload []byte) {
			if msg, err := wire.ParsePresence(payload); err == nil {
				h(groupID, msg)
			}
		})
	}
	return rt, nil
}

// SubscribeRoster subscribes presence and maintains a Roster from it —
// the §7 client rules (generation discard, per-index latest-wins,
// reset on generation change) applied in the SDK. Close the returned
// track to unsubscribe; the Roster then simply stops updating.
func (c *Client) SubscribeRoster(ctx context.Context) (*Roster, *moqtransport.RemoteTrack, error) {
	r := NewRoster()
	rt, err := c.SubscribePresence(ctx, r.Apply)
	if err != nil {
		return nil, nil, err
	}
	return r, rt, nil
}

func (c *Client) addRoute(alias uint64, h func(seq uint64, payload []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.routes.Load()
	next := make(map[uint64]func(uint64, []byte), 4)
	if old != nil {
		for k, v := range *old {
			next[k] = v
		}
	}
	next[alias] = h
	c.routes.Store(&next)
}

// onDatagram is the client's receive hot path.
func (c *Client) onDatagram(trackAlias, groupID, _ uint64, payload []byte) bool {
	if m := c.routes.Load(); m != nil {
		if h, ok := (*m)[trackAlias]; ok {
			h(groupID, payload)
			return true
		}
	}
	return false
}

// SyncState runs the state subscription against cfg.Store: subscribe with
// the store's cursor, feed both lanes, re-subscribe with backoff on
// termination (the protocol's native resync path). Runs until ctx ends.
// All store access from the caller must go through View.
func (c *Client) SyncState(ctx context.Context) error {
	if c.cfg.Store == nil {
		return errors.New("client: no store configured")
	}
	backoff := 100 * time.Millisecond
	for {
		err := c.syncStateOnce(ctx)
		if ctx.Err() != nil || c.ctx.Err() != nil {
			return ctx.Err()
		}
		_ = err
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}

// View runs fn with exclusive access to the state store.
func (c *Client) View(fn func(*clientstore.Store)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(c.cfg.Store)
}

func (c *Client) syncStateOnce(ctx context.Context) error {
	c.mu.Lock()
	params := c.cfg.Store.SubscribeParams()
	c.mu.Unlock()
	raw, err := wire.AppendSubscribeParams(nil, params)
	if err != nil {
		return err
	}
	ns := ident.ClientNamespace(c.cfg.SpaceID, c.cfg.ClientID, ident.DirectionSink)
	rt, err := c.session.Subscribe(ctx, ns.Tuple(), ident.TrackState,
		moqtransport.WithSubscribeParameters(moqtransport.KVPList{
			{Type: wire.SubscribeParamState, ValueBytes: raw},
		}))
	if err != nil {
		return err
	}
	defer rt.Close()
	for {
		obj, err := rt.ReadObject(ctx)
		if err != nil {
			c.mu.Lock()
			c.cfg.Store.Terminated()
			c.mu.Unlock()
			return err
		}
		msg, err := wire.ParseStateMessage(obj.Payload, wire.Downstream)
		if err != nil {
			return fmt.Errorf("client: malformed state message: %w", err)
		}
		c.mu.Lock()
		if obj.SubGroupID == 1 {
			err = c.cfg.Store.ApplyCatchUp(msg)
		} else {
			err = c.cfg.Store.ApplyLive(msg)
		}
		c.mu.Unlock()
		if err != nil {
			return err
		}
	}
}
