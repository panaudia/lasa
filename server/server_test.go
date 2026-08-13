package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/state"
)

// A space is ticketed XOR unticketed (lasa-core.md §4.4): New rejects
// both-set and neither-set, so no embedder can construct a mixed mode.
func TestNewRejectsMixedMode(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(keys []ed25519.PublicKey, unticketed bool) Config {
		return Config{
			SpaceID:         "s",
			Backend:         newStubBackend(),
			RulesFor:        func(*connect.Resolved) state.Rules { return state.Rules{} },
			TicketKeys:      keys,
			AllowUnticketed: unticketed,
		}
	}

	if _, err := New(mk([]ed25519.PublicKey{pub}, true)); err == nil || !strings.Contains(err.Error(), "XOR") {
		t.Fatalf("mixed mode must be a construction error, got %v", err)
	}
	if _, err := New(mk(nil, false)); err == nil {
		t.Fatal("neither mode must be a construction error")
	}
	s, err := New(mk([]ed25519.PublicKey{pub}, false))
	if err != nil {
		t.Fatalf("ticketed-only: %v", err)
	}
	s.Close()
	s, err = New(mk(nil, true))
	if err != nil {
		t.Fatalf("unticketed-only: %v", err)
	}
	s.Close()
}
