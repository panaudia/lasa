package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/state"
	"github.com/panaudia/lasa/state/clientstore"
	"github.com/panaudia/lasa/wire"
)

func testTLS(t *testing.T) *tls.Config {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(1)}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{ALPN}}
}

type testSpace struct {
	srv     *Server
	backend *stubBackend
	addr    string
}

func startSpace(t *testing.T, mod func(*Config)) *testSpace {
	t.Helper()
	backend := newStubBackend()
	cfg := Config{
		SpaceID:         "space1",
		Backend:         backend,
		AllowUnticketed: true,
		// The one place epoch time exists (design §1): sample → µs.
		SampleEpoch:      func(s uint64) uint64 { return 1_000_000_000 + s*1_000_000/48000 },
		PresenceInterval: 20 * time.Millisecond,
		KeyframeInterval: 200 * time.Millisecond,
		RulesFor: func(*connect.Resolved) state.Rules {
			return state.Rules{Set: []string{""}, Clear: []string{""}, Read: []string{""}}
		},
	}
	if mod != nil {
		mod(&cfg)
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	backend.attach(srv.Presence())
	l, err := quic.ListenAddr("localhost:0", testTLS(t), &quic.Config{EnableDatagrams: true})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.ServeQUIC(l) }()
	t.Cleanup(func() { _ = l.Close(); srv.Close() })
	return &testSpace{srv: srv, backend: backend, addr: l.Addr().String()}
}

func dial(t *testing.T, sp *testSpace, mod func(*client.Config)) *client.Client {
	t.Helper()
	cfg := client.Config{
		SpaceID:  "space1",
		ClientID: "c1",
		Entities: []connect.Entity{{ID: "e1", Name: "Alice"}},
		TLS:      &tls.Config{InsecureSkipVerify: true},
	}
	if mod != nil {
		mod(&cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := client.Dial(ctx, sp.addr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func waitCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// The full ingress path: client publishes mono-object datagrams; the stub
// receives them raw via WritePacket, decodes, applies latest-wins pose.
func TestE2EPublishAudioAndPose(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pub, err := c.Entity(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	// Publish continuously, like a real client: datagrams are lossy and
	// the leading packets can race the server's subscription plumbing.
	ent := sp.backend.entity("e1")
	seq := uint64(0)
	waitCond(t, "5 packets at the stub", func() bool {
		p := wire.Pose{X: float32(seq), Y: 2, Z: 3, Yaw: 0.5}
		if err := pub.WriteMonoObject(seq, &wire.MonoObjectPacket{Pose: &p, Audio: []byte{0xAB, byte(seq), 0xCD}}); err != nil {
			t.Fatal(err)
		}
		seq++
		ent.mu.Lock()
		defer ent.mu.Unlock()
		return len(ent.packets) >= 5
	})
	ent.mu.Lock()
	defer ent.mu.Unlock()
	for _, pkt := range ent.packets {
		if pkt.audio[1] != byte(pkt.seq) {
			t.Fatalf("packet %d carries audio tag %d", pkt.seq, pkt.audio[1])
		}
	}
	last := ent.packets[len(ent.packets)-1]
	if !ent.hasPose || ent.poseSeq < last.seq {
		t.Fatalf("pose seq %d (hasPose=%v), want latest-wins ≥ %d", ent.poseSeq, ent.hasPose, last.seq)
	}
}

// Sink egress: subscription starts the render, frames arrive parseable
// with continuous seqs and a ts anchor on the first packet.
func TestE2ESinkSubscription(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil)

	type rec struct {
		seq uint64
		pkt *wire.SinkPacket
	}
	var mu sync.Mutex
	var got []rec
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.SubscribeSink(ctx, "e1", "binaural", func(seq uint64, payload []byte) {
		pkt, err := wire.ParseSink(payload)
		if err != nil {
			return
		}
		mu.Lock()
		got = append(got, rec{seq, pkt})
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	waitCond(t, "10 sink frames", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 10
	})
	mu.Lock()
	defer mu.Unlock()
	if got[0].pkt.Timestamp == nil {
		t.Error("first sink packet must carry the ts anchor")
	}
	for i := 1; i < len(got); i++ {
		if got[i].seq != got[i-1].seq+1 {
			t.Fatalf("seq gap: %d then %d", got[i-1].seq, got[i].seq)
		}
	}
}

// Wrong ambi order is rejected at subscribe (deviation 1).
func TestE2ESinkFormatPolicy(t *testing.T) {
	sp := startSpace(t, func(cfg *Config) { cfg.AmbiOrder = 2 })
	c := dial(t, sp, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.SubscribeSink(ctx, "e1", "ambi2", func(uint64, []byte) {}); err != nil {
		t.Fatalf("configured ambi order must be offered: %v", err)
	}
	if _, err := c.SubscribeSink(ctx, "e1", "ambi3", func(uint64, []byte) {}); err == nil {
		t.Fatal("ambi3 must be rejected when the global order is 2")
	}
}

// Authorization: another client's entity sink is outside the binding.
func TestE2EUnauthorizedSink(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.SubscribeSink(ctx, "someone-else", "binaural", func(uint64, []byte) {}); err == nil {
		t.Fatal("subscribing another entity's sink must be rejected")
	}
}

// State: write → echo on own sink via the store; reconnect resumes with
// a delta through the persisted cursor.
func TestE2EStateSyncAndReconnect(t *testing.T) {
	sp := startSpace(t, nil)
	persist := clientstore.NewMemPersistence()

	store, err := clientstore.New("space1", []string{""}, persist)
	if err != nil {
		t.Fatal(err)
	}
	c := dial(t, sp, func(cfg *client.Config) { cfg.Store = store })
	syncCtx, syncCancel := context.WithCancel(context.Background())
	go func() { _ = c.SyncState(syncCtx) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.WriteState(ctx, wire.Set{Key: "app.k", Value: []byte("v1")}); err != nil {
		t.Fatal(err)
	}
	waitCond(t, "echo of app.k", func() bool {
		var ok bool
		c.View(func(s *clientstore.Store) { _, ok = s.Get("app.k") })
		return ok
	})
	// The admission writes are visible too (base profile marker + name).
	c.View(func(s *clientstore.Store) {
		if v, ok := s.Get("lasa.entity.e1.name"); !ok || string(v) != `"Alice"` {
			t.Errorf("lasa.entity.e1.name = %q, %v", v, ok)
		}
	})
	syncCancel()
	_ = c.Close()

	// While away, the server writes more state; the entity sweep also ran.
	if err := sp.srv.Engine().Write(wire.Set{Key: "app.later", Value: []byte("v2")}); err != nil {
		t.Fatal(err)
	}

	store2, err := clientstore.New("space1", []string{""}, persist)
	if err != nil {
		t.Fatal(err)
	}
	if store2.SubscribeParams().Cursor == nil {
		t.Fatal("cursor must have been persisted")
	}
	c2 := dial(t, sp, func(cfg *client.Config) { cfg.Store = store2 })
	syncCtx2, syncCancel2 := context.WithCancel(context.Background())
	defer syncCancel2()
	go func() { _ = c2.SyncState(syncCtx2) }()

	waitCond(t, "delta bringing app.later", func() bool {
		var ok bool
		c2.View(func(s *clientstore.Store) { _, ok = s.Get("app.later") })
		return ok
	})
	c2.View(func(s *clientstore.Store) {
		if _, ok := s.Get("app.k"); !ok {
			t.Error("app.k must survive the reconnect")
		}
	})
}

// Presence: keyframes carry the roster with the published pose.
func TestE2EPresence(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pub, err := c.Entity(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	// Publish the pose continuously, like a real client.
	poseStop := make(chan struct{})
	defer close(poseStop)
	go func() {
		pose := wire.Pose{X: 7}
		t := time.NewTicker(10 * time.Millisecond)
		defer t.Stop()
		for seq := uint64(0); ; seq++ {
			select {
			case <-poseStop:
				return
			case <-t.C:
				_ = pub.WriteMonoObject(seq, &wire.MonoObjectPacket{Pose: &pose})
			}
		}
	}()

	var mu sync.Mutex
	var seen *wire.KeyframeRecord
	if _, err := c.SubscribePresence(ctx, func(msg any) {
		if kf, ok := msg.(*wire.PresenceKeyframe); ok {
			for i := range kf.Records {
				if kf.Records[i].ID == "e1" {
					mu.Lock()
					r := kf.Records[i]
					seen = &r
					mu.Unlock()
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	waitCond(t, "keyframe with e1 at X=7", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return seen != nil && math.Abs(float64(seen.Pose.X-7)) < 0.01
	})
}

// A ticketed-only space rejects an unticketed client with the §4.6 code,
// delivered as the QUIC application error.
func TestE2ETicketRequired(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp := startSpace(t, func(cfg *Config) {
		cfg.AllowUnticketed = false
		cfg.TicketKeys = []ed25519.PublicKey{pub}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = client.Dial(ctx, sp.addr, client.Config{
		SpaceID:  "space1",
		ClientID: "c1",
		Entities: []connect.Entity{{ID: "e1", Name: "A"}},
		TLS:      &tls.Config{InsecureSkipVerify: true},
	})
	if err == nil {
		t.Fatal("unticketed dial must fail")
	}
	var appErr *quic.ApplicationError
	if !errors.As(err, &appErr) || appErr.ErrorCode != quic.ApplicationErrorCode(wire.ErrCodeInvalidTicket) {
		t.Fatalf("err = %v, want application error 0x1A5A01", err)
	}
}

// Departure: closing the client sweeps the entity's keys (marker first)
// and notifies the backend.
func TestE2EDepartureSweep(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil)
	waitCond(t, "marker written", func() bool {
		v, ok := engineGet(sp.srv, "lasa.entity.e1")
		return ok && string(v) == "true"
	})
	_ = c.Close()
	waitCond(t, "marker swept", func() bool {
		_, ok := engineGet(sp.srv, "lasa.entity.e1")
		return !ok
	})
	waitCond(t, "EntityLeft", func() bool {
		sp.backend.mu.Lock()
		defer sp.backend.mu.Unlock()
		return len(sp.backend.left) == 1 && sp.backend.left[0] == "e1"
	})
}

func engineGet(s *Server, key string) ([]byte, bool) {
	e := s.Engine()
	c, err := e.Register("__probe", state.Rules{Read: []string{""}})
	if err != nil {
		return nil, false
	}
	sub, err := e.Attach(c, []string{key}, nil)
	if err != nil {
		return nil, false
	}
	defer e.Detach(sub)
	for {
		m, err := sub.CatchUpNext(context.Background())
		if err != nil {
			return nil, false
		}
		if s, ok := m.(wire.Set); ok && s.Key == key {
			return s.Value, true
		}
		if _, ok := m.(wire.Frontier); ok {
			return nil, false
		}
	}
}
