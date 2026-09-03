package conformance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	webtransport "github.com/quic-go/webtransport-go"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/state/clientstore"
	"github.com/panaudia/lasa/wire"
)

const dialTimeout = 5 * time.Second

// party describes one connection the harness wants: its client id, its
// entities and, for ticketed fixtures, the roles and permissions its
// ticket should carry. The harness mints the ticket.
type party struct {
	clientID    string
	entities    []connect.Entity // nil = absent, empty = observer
	roles       []string
	permissions connect.Permissions
	// ticket overrides minting when set (rejection tests).
	ticket string
	// noSetups presents a ticket without instantiating its entities.
	noSetups bool
}

// configFor builds the client config for p on f: ad-hoc entities in an
// unticketed space, a minted ticket plus setups in a ticketed one.
func configFor(t testing.TB, f *Fixture, p party) client.Config {
	t.Helper()
	cfg := client.Config{SpaceID: f.SpaceID, ClientID: p.clientID, TLS: f.TLS}
	if !f.Ticketed {
		cfg.Entities = p.entities
		if p.ticket != "" {
			cfg.Ticket = p.ticket
		}
		return cfg
	}
	if p.ticket != "" {
		cfg.Ticket = p.ticket
	} else {
		cfg.Ticket = mint(t, f, f.IssuerKey, connect.MintClaims{
			Issuer: f.Issuer, SpaceID: f.SpaceID, ClientID: p.clientID,
			Roles: p.roles, Permissions: p.permissions, Entities: p.entities,
			IssuedAt: time.Now(),
		})
	}
	if p.entities != nil && !p.noSetups {
		setups := make([]connect.Setup, 0, len(p.entities))
		for _, e := range p.entities {
			setups = append(setups, connect.Setup{ID: e.ID})
		}
		cfg.Setups = setups
		if len(p.entities) == 0 {
			cfg.Entities = []connect.Entity{} // observer: explicit empty entities
			cfg.Setups = nil
		}
	}
	return cfg
}

func mint(t testing.TB, f *Fixture, key ed25519.PrivateKey, c connect.MintClaims) string {
	t.Helper()
	tok, err := connect.Mint(key, c)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func otherKey(t testing.TB) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

// dial connects on the fixture's transport. The error carries the §4.6
// code when the server rejected the connection (see closeCode).
func dial(t testing.TB, f *Fixture, cfg client.Config) (*client.Client, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	var c *client.Client
	var err error
	switch f.Transport {
	case TransportWebTransport:
		c, err = client.DialWebTransport(ctx, f.Addr, cfg)
	default:
		c, err = client.Dial(ctx, f.Addr, cfg)
	}
	if err == nil {
		t.Cleanup(func() { _ = c.Close() })
	}
	return c, err
}

func mustDial(t testing.TB, f *Fixture, p party) *client.Client {
	t.Helper()
	c, err := dial(t, f, configFor(t, f, p))
	if err != nil {
		t.Fatalf("dial %s: %v", p.clientID, err)
	}
	return c
}

// closeCode extracts the LASA close code from a dial error or a
// client's Err on either transport.
func closeCode(err error) (uint32, string, bool) {
	var appErr *quic.ApplicationError
	if errors.As(err, &appErr) {
		return uint32(appErr.ErrorCode), appErr.ErrorMessage, true
	}
	var sessErr *webtransport.SessionError
	if errors.As(err, &sessErr) {
		return uint32(sessErr.ErrorCode), sessErr.Message, true
	}
	return 0, "", false
}

// expectRejection dials expecting the §4.6 code and records id.
func expectRejection(t testing.TB, id string, f *Fixture, cfg client.Config, want uint32) {
	t.Helper()
	c, err := dial(t, f, cfg)
	if err == nil {
		_ = c.Close()
		Fail(t, id, "dial admitted, want rejection %#x", want)
		return
	}
	code, reason, ok := closeCode(err)
	if !ok {
		Fail(t, id, "dial failed without a LASA code: %v", err)
		return
	}
	if code != want {
		Fail(t, id, "close code %#x (%q), want %#x", code, reason, want)
		return
	}
	Pass(t, id, "%#x %q on %s", code, reason, f.Transport)
	Check(t, "C-4.6-2", code>>8 == 0x1A5A, "code %#x outside the LASA range", code)
}

// rejectionReason dials expecting a rejection and returns its reason.
func rejectionReason(t testing.TB, f *Fixture, cfg client.Config) (uint32, string) {
	t.Helper()
	c, err := dial(t, f, cfg)
	if err == nil {
		_ = c.Close()
		t.Fatal("dial admitted, want rejection")
	}
	code, reason, ok := closeCode(err)
	if !ok {
		t.Fatalf("dial failed without a LASA code: %v", err)
	}
	return code, reason
}

// expectClosed waits for the client to be closed by the server with the
// given code.
func expectClosed(t testing.TB, id string, c *client.Client, want uint32) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(dialTimeout):
		Fail(t, id, "session not closed by the server")
		return
	}
	code, reason, ok := closeCode(c.Err())
	if !ok || code != want {
		Fail(t, id, "close = %v, want code %#x", c.Err(), want)
		return
	}
	Pass(t, id, "%#x %q", code, reason)
	Check(t, "C-4.6-2", code>>8 == 0x1A5A, "code %#x outside the LASA range", code)
}

// absentFor reports whether cond stays false for d.
func absentFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

// syncingClient dials p with a fresh in-memory store over prefixes and
// starts SyncState on it.
func syncingClient(t testing.TB, f *Fixture, p party, prefixes []string) *client.Client {
	t.Helper()
	store, err := clientstore.New(f.SpaceID, prefixes, clientstore.NewMemPersistence())
	if err != nil {
		t.Fatal(err)
	}
	cfg := configFor(t, f, p)
	cfg.Store = store
	c, err := dial(t, f, cfg)
	if err != nil {
		t.Fatalf("dial %s: %v", p.clientID, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = c.SyncState(ctx) }()
	return c
}

func write(t testing.TB, c *client.Client, msgs ...wire.StateMessage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	if err := c.WriteState(ctx, msgs...); err != nil {
		t.Fatal(err)
	}
}

func waitCond(t testing.TB, what string, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("timeout waiting for %s", what)
	return false
}

// observer opens a zero-entity connection syncing state under prefixes
// into a fresh in-memory store, and returns the client and the store.
func observer(t testing.TB, f *Fixture, clientID string, prefixes []string, perms connect.Permissions) (*client.Client, *clientstore.Store) {
	t.Helper()
	store, err := clientstore.New(f.SpaceID, prefixes, clientstore.NewMemPersistence())
	if err != nil {
		t.Fatal(err)
	}
	cfg := configFor(t, f, party{clientID: clientID, entities: []connect.Entity{}, permissions: perms})
	cfg.Store = store
	c, err := dial(t, f, cfg)
	if err != nil {
		t.Fatalf("observer dial: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = c.SyncState(ctx) }()
	return c, store
}

// clientstoreStore is the store type for View callbacks.
type clientstoreStore = clientstore.Store

// syncingClientCfg is syncingClient over a prepared config.
func syncingClientCfg(t testing.TB, f *Fixture, cfg client.Config, prefixes []string) *client.Client {
	t.Helper()
	store, err := clientstore.New(f.SpaceID, prefixes, clientstore.NewMemPersistence())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Store = store
	c, err := dial(t, f, cfg)
	if err != nil {
		t.Fatalf("dial %s: %v", cfg.ClientID, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = c.SyncState(ctx) }()
	return c
}

func storeGet(c *client.Client, key string) (string, bool) {
	var v []byte
	var ok bool
	c.View(func(s *clientstore.Store) { v, ok = s.Get(key) })
	return string(v), ok
}

func storeHasPrefix(c *client.Client, prefix string) bool {
	found := false
	c.View(func(s *clientstore.Store) {
		s.Range(func(k string, _ []byte, _ uint64) bool {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				found = true
				return false
			}
			return true
		})
	})
	return found
}
