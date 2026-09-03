package conformance

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Transport selects the LASA transport under test. The spec makes the
// two identical at the LASA layer (core §4.1), so every test runs under
// both when both are available.
type Transport string

const (
	TransportQUIC         Transport = "quic"
	TransportWebTransport Transport = "webtransport"
)

// Fixture is the fixture contract (plan/conformance-suite.md §2.3): what
// the harness must know about an implementation under test beyond its
// URL. Every field past Addr is a declared fact about the server, not a
// control over it. See fixture.md for the items and their evidence.
type Fixture struct {
	Name      string
	Transport Transport
	// Addr is host:port for raw QUIC, or the session URL for
	// WebTransport.
	Addr    string
	SpaceID string
	// Ticketed says which mode the space runs in. When true the harness
	// mints its own tickets with IssuerKey, whose public half the server
	// must be configured to trust.
	Ticketed  bool
	Issuer    string
	IssuerKey ed25519.PrivateKey
	// RenderOrder is the space's declared internal render order (core
	// §3: ambi3 SHOULD be offered at 3 or higher). Zero means unknown,
	// and the rows that depend on it are reported as not exercised.
	RenderOrder int
	TLS         *tls.Config
	// Shutdown stops the space deliberately, when the fixture can. nil
	// means the shutdown rows are not exercised.
	Shutdown func(ctx context.Context) error
}

// FixtureFromEnv builds a fixture for an external implementation from
// the LASA_CONFORM_* variables. ok is false when LASA_CONFORM_URL is
// unset, in which case the tests run against the in-repo reference.
//
//	LASA_CONFORM_URL         host:port (raw QUIC) or https://host/path (WebTransport)
//	LASA_CONFORM_SPACE       space id (required)
//	LASA_CONFORM_ISSUER_KEY  hex Ed25519 private key (64 bytes) or seed (32); presence means ticketed
//	LASA_CONFORM_ISSUER      iss claim for minted tickets (default lasa-conform)
//	LASA_CONFORM_ORDER       declared internal render order
//	LASA_CONFORM_INSECURE    1 to skip TLS verification
func FixtureFromEnv() (f *Fixture, ok bool, err error) {
	url := os.Getenv("LASA_CONFORM_URL")
	if url == "" {
		return nil, false, nil
	}
	f = &Fixture{Name: "env", Addr: url, Transport: TransportQUIC, Issuer: "lasa-conform"}
	if strings.HasPrefix(url, "https://") {
		f.Transport = TransportWebTransport
	}
	if f.SpaceID = os.Getenv("LASA_CONFORM_SPACE"); f.SpaceID == "" {
		return nil, true, errors.New("LASA_CONFORM_SPACE is required")
	}
	if k := os.Getenv("LASA_CONFORM_ISSUER_KEY"); k != "" {
		b, err := hex.DecodeString(k)
		if err != nil {
			return nil, true, fmt.Errorf("LASA_CONFORM_ISSUER_KEY: %w", err)
		}
		switch len(b) {
		case ed25519.SeedSize:
			f.IssuerKey = ed25519.NewKeyFromSeed(b)
		case ed25519.PrivateKeySize:
			f.IssuerKey = ed25519.PrivateKey(b)
		default:
			return nil, true, fmt.Errorf("LASA_CONFORM_ISSUER_KEY: %d bytes, want 32 or 64", len(b))
		}
		f.Ticketed = true
	}
	if iss := os.Getenv("LASA_CONFORM_ISSUER"); iss != "" {
		f.Issuer = iss
	}
	if o := os.Getenv("LASA_CONFORM_ORDER"); o != "" {
		if f.RenderOrder, err = strconv.Atoi(o); err != nil {
			return nil, true, fmt.Errorf("LASA_CONFORM_ORDER: %w", err)
		}
	}
	f.TLS = &tls.Config{InsecureSkipVerify: os.Getenv("LASA_CONFORM_INSECURE") == "1"}
	return f, true, nil
}
