package server

// The WebTransport transport, end to end, on the upstream
// webtransport-go the module pins: "identical over WebTransport and
// raw QUIC" (lasa-core §4.1) includes the §4.6 close codes, which the
// client reads from the session's recorded close error. The
// panaudia-lasa server has its own copy of these tests against its
// patched webtransport-go; this one proves the reference client on
// what a `go get` delivers.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	webtransport "github.com/quic-go/webtransport-go"

	"github.com/panaudia/moqtransport/webtransportmoq"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

const wtTestPath = "/lasa"

// startWebTransportSpace is startSpace over HTTP/3 + WebTransport: the
// upgrade handler hands each session to the same ServeConn the raw-QUIC
// path uses. addr is the WebTransport URL.
func startWebTransportSpace(t *testing.T, mod func(*Config)) *testSpace {
	t.Helper()
	sp := startSpace(t, mod) // the raw-QUIC listener is unused here
	tlsCfg := testTLS(t)
	tlsCfg.NextProtos = []string{http3.NextProtoH3}
	l, err := quic.ListenAddr("localhost:0", tlsCfg, &quic.Config{
		EnableDatagrams:                  true,
		EnableStreamResetPartialDelivery: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	wt := &webtransport.Server{
		H3:                   &http3.Server{TLSConfig: tlsCfg, Handler: mux, EnableDatagrams: true},
		ApplicationProtocols: []string{ALPN},
		CheckOrigin:          func(*http.Request) bool { return true },
	}
	mux.HandleFunc(wtTestPath, func(w http.ResponseWriter, r *http.Request) {
		sess, err := wt.Upgrade(w, r)
		if err != nil {
			http.Error(w, "upgrade failed", http.StatusBadRequest)
			return
		}
		go func() { _ = sp.srv.ServeConn(sess.Context(), webtransportmoq.NewServer(sess)) }()
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
	t.Cleanup(func() { cancel(); _ = wt.Close(); _ = l.Close() })
	sp.addr = fmt.Sprintf("https://%s%s", l.Addr(), wtTestPath)
	return sp
}

func dialWT(t *testing.T, sp *testSpace, mod func(*client.Config)) (*client.Client, error) {
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
	return client.DialWebTransport(ctx, sp.addr, cfg)
}

// A rejection at admission reaches the client as the coded
// SessionError, not a bare EOF.
func TestWTRejectionCarriesCode(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp := startWebTransportSpace(t, func(cfg *Config) {
		cfg.AllowUnticketed = false
		cfg.TicketKeys = []ed25519.PublicKey{pub}
	})
	_, err = dialWT(t, sp, nil)
	if err == nil {
		t.Fatal("unticketed dial into a ticketed space must fail")
	}
	var sessErr *webtransport.SessionError
	if !errors.As(err, &sessErr) {
		t.Fatalf("rejection must surface as a WT session error carrying the code; got %T: %v", err, err)
	}
	if uint32(sessErr.ErrorCode) != wire.ErrCodeInvalidTicket {
		t.Fatalf("close code = %#x, want %#x (reason %q)", uint32(sessErr.ErrorCode), wire.ErrCodeInvalidTicket, sessErr.Message)
	}
}

// A close while live (Shutdown) reaches the client through Err as the
// coded SessionError, the WebTransport twin of
// TestE2EShutdownSeversWithCode.
func TestWTShutdownSeversWithCode(t *testing.T) {
	sp := startWebTransportSpace(t, nil)
	c, err := dialWT(t, sp, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	waitCond(t, "admission", func() bool {
		_, ok := engineGet(sp.srv, "lasa.client.c1")
		return ok
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sp.srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("client not closed by Shutdown")
	}
	var sessErr *webtransport.SessionError
	if err := c.Err(); !errors.As(err, &sessErr) || uint32(sessErr.ErrorCode) != wire.ErrCodeShuttingDown {
		t.Fatalf("client close = %v, want SessionError %#x", err, wire.ErrCodeShuttingDown)
	}
}
