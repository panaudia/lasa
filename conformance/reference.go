package conformance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	webtransport "github.com/quic-go/webtransport-go"

	"github.com/panaudia/moqtransport/webtransportmoq"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/presence"
	"github.com/panaudia/lasa/profile/base"
	"github.com/panaudia/lasa/server"
	"github.com/panaudia/lasa/wire"
)

// The in-repo reference: the lasa server shell over a stub backend,
// wired with the base profile exactly as panaudia-lasa wires it. Used
// as the implementation under test when no LASA_CONFORM_URL is set,
// and as the host of the uplink observer (observer.go).

func startReference(tr Transport, ticketed bool, mod func(*server.Config)) (*Fixture, func(), error) {
	return startShell(tr, ticketed, newStubBackend(), mod)
}

// backendWithCollector is what startShell needs from a backend beyond
// the SPI: the presence collector is only known after server.New.
type backendWithCollector interface {
	server.Backend
	attach(*presence.Collector)
}

func startShell(tr Transport, ticketed bool, backend backendWithCollector, mod func(*server.Config)) (*Fixture, func(), error) {
	f := &Fixture{
		Name:        fmt.Sprintf("%s-%s", tr, map[bool]string{false: "unticketed", true: "ticketed"}[ticketed]),
		Transport:   tr,
		SpaceID:     "conform",
		Ticketed:    ticketed,
		Issuer:      "lasa-conform",
		RenderOrder: 2,
		TLS:         &tls.Config{InsecureSkipVerify: true},
	}
	cfg := server.Config{
		SpaceID:          f.SpaceID,
		Backend:          backend,
		SampleEpoch:      func(s uint64) uint64 { return 1_700_000_000_000_000 + s*1_000_000/48000 },
		PresenceInterval: 20 * time.Millisecond,
		KeyframeInterval: 200 * time.Millisecond,
		AmbiOrder:        2,
		AdHocPermitted:   base.AdHocPermitted,
		SinkAll:          base.SinkAll,
		ValidRoles:       base.ValidRoles,
		RulesFor:         base.RulesFor,
	}
	if ticketed {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		f.IssuerKey = priv
		cfg.TicketKeys = []ed25519.PublicKey{pub}
	} else {
		cfg.AllowUnticketed = true
	}
	if mod != nil {
		mod(&cfg)
	}
	f.RenderOrder = cfg.AmbiOrder
	srv, err := server.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	backend.attach(srv.Presence())
	f.Shutdown = srv.Shutdown
	tlsCfg, err := selfSignedTLS()
	if err != nil {
		srv.Close()
		return nil, nil, err
	}
	switch tr {
	case TransportQUIC:
		tlsCfg.NextProtos = []string{server.ALPN}
		l, err := quic.ListenAddr("localhost:0", tlsCfg, &quic.Config{EnableDatagrams: true})
		if err != nil {
			srv.Close()
			return nil, nil, err
		}
		go func() { _ = srv.ServeQUIC(l) }()
		f.Addr = l.Addr().String()
		return f, func() { _ = l.Close(); srv.Close() }, nil
	case TransportWebTransport:
		tlsCfg.NextProtos = []string{http3.NextProtoH3}
		l, err := quic.ListenAddr("localhost:0", tlsCfg, &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		})
		if err != nil {
			srv.Close()
			return nil, nil, err
		}
		mux := http.NewServeMux()
		wt := &webtransport.Server{
			H3:                   &http3.Server{TLSConfig: tlsCfg, Handler: mux, EnableDatagrams: true},
			ApplicationProtocols: []string{server.ALPN},
			CheckOrigin:          func(*http.Request) bool { return true },
		}
		mux.HandleFunc("/lasa", func(w http.ResponseWriter, r *http.Request) {
			sess, err := wt.Upgrade(w, r)
			if err != nil {
				http.Error(w, "upgrade failed", http.StatusBadRequest)
				return
			}
			go func() { _ = srv.ServeConn(sess.Context(), webtransportmoq.NewServer(sess)) }()
		})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for {
				conn, err := l.Accept(ctx)
				if err != nil {
					return
				}
				go func() { _ = wt.ServeQUICConn(conn) }()
			}
		}()
		f.Addr = fmt.Sprintf("https://%s/lasa", l.Addr())
		return f, func() { cancel(); _ = wt.Close(); _ = l.Close(); srv.Close() }, nil
	}
	srv.Close()
	return nil, nil, fmt.Errorf("unknown transport %q", tr)
}

func selfSignedTLS() (*tls.Config, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(1)}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}

// stubBackend is the reference embedder for conformance runs: it feeds
// presence from arriving poses, renders 5 ms counter frames on every
// sink, and refuses sinks for head-frame entities (base profile §6).
// It mirrors the server package's own stub without sharing it, because
// that one exists to test the SPI and this one exists to stand behind
// a URL.
type stubBackend struct {
	mu        sync.Mutex
	collector *presence.Collector
	headFrame map[string]bool
}

func newStubBackend() *stubBackend { return &stubBackend{headFrame: map[string]bool{}} }

func (b *stubBackend) attach(c *presence.Collector) {
	b.mu.Lock()
	b.collector = c
	b.mu.Unlock()
}

type stubEntity struct {
	slot    *presence.Slot
	mu      sync.Mutex
	poseSeq uint64
	hasPose bool
	dof     int
	initial wire.Pose
}

// poseFromSpec converts a config pose to the wire pose (base profile
// §6, initial-pose).
func poseFromSpec(s *connect.PoseSpec) wire.Pose {
	var p wire.Pose
	if s == nil {
		return p
	}
	if s.Position != nil {
		p.X, p.Y, p.Z = float32(s.Position.X), float32(s.Position.Y), float32(s.Position.Z)
	}
	if s.Attitude != nil {
		p.Yaw, p.Pitch, p.Roll = float32(s.Attitude.Yaw), float32(s.Attitude.Pitch), float32(s.Attitude.Roll)
	}
	return p
}

func (b *stubBackend) EntityJoined(clientID string, e connect.ResolvedEntity) (server.EntityHandler, error) {
	ent := &stubEntity{dof: 6, initial: poseFromSpec(e.InitialPose)}
	if e.Dof != nil {
		ent.dof = *e.Dof
	}
	b.mu.Lock()
	head := e.Frame == "head"
	b.headFrame[e.ID] = head
	if b.collector != nil {
		ent.slot = b.collector.Register(e.ID, head, func() wire.Loudness { return 10 })
		ent.slot.UpdatePose(ent.initial)
	}
	b.mu.Unlock()
	return ent, nil
}

func (b *stubBackend) EntityLeft(clientID, entityID string) {
	b.mu.Lock()
	delete(b.headFrame, entityID)
	b.mu.Unlock()
}

func (e *stubEntity) WritePacket(seq uint64, payload []byte) {
	pkt, err := wire.ParseMonoObject(payload)
	if err != nil {
		return
	}
	e.mu.Lock()
	if pkt.Pose != nil && (!e.hasPose || seq > e.poseSeq) && e.dof != 0 {
		e.poseSeq, e.hasPose = seq, true
		pose := *pkt.Pose
		if e.dof == 3 { // turns in place: position stays the initial one
			pose.X, pose.Y, pose.Z = e.initial.X, e.initial.Y, e.initial.Z
		}
		if e.slot != nil {
			e.slot.UpdatePose(pose)
		}
	}
	e.mu.Unlock()
}

func (b *stubBackend) StartSink(entityID, format string, w server.SinkWriter) (server.SinkSession, error) {
	b.mu.Lock()
	head := b.headFrame[entityID]
	b.mu.Unlock()
	if head {
		return nil, fmt.Errorf("head-frame entity %s has no sink", entityID)
	}
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		var i byte
		var sampleTS uint64
		frame := make([]byte, 60)
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				frame[0] = i
				_ = w.WriteFrame(frame, sampleTS)
				i++
				sampleTS += wire.FrameSamples
			}
		}
	}()
	return stubSession{stop: stop}, nil
}

type stubSession struct{ stop chan struct{} }

func (s stubSession) Stop() { close(s.stop) }
