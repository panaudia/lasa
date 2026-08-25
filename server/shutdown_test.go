package server

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/panaudia/lasa/wire"
)

// Shutdown closes every live session with ErrCodeShuttingDown — the
// client can tell a deliberate stop from a dead network — and runs each
// departure through the normal funnel before the space closes.
func TestE2EShutdownSeversWithCode(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil) // ClientID c1, entity e1
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
	var appErr *quic.ApplicationError
	if err := c.Err(); !errors.As(err, &appErr) || appErr.ErrorCode != quic.ApplicationErrorCode(wire.ErrCodeShuttingDown) {
		t.Fatalf("client close = %v, want ApplicationError %#x", err, wire.ErrCodeShuttingDown)
	}
	sp.backend.mu.Lock()
	left := slices.Contains(sp.backend.left, "e1")
	sp.backend.mu.Unlock()
	if !left {
		t.Fatal("EntityLeft not delivered before Shutdown returned")
	}
}

// A live moderation block closes with ErrCodeBlocked, not a bare close.
func TestE2EBlockedWhileLiveCarriesCode(t *testing.T) {
	sp := startSpace(t, nil)
	c := dial(t, sp, nil)
	waitCond(t, "admission", func() bool {
		_, ok := engineGet(sp.srv, "lasa.client.c1")
		return ok
	})
	block(t, sp, "lasa.space.client.blocked.c1", "0")
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("client not severed by the block")
	}
	var appErr *quic.ApplicationError
	if err := c.Err(); !errors.As(err, &appErr) || appErr.ErrorCode != quic.ApplicationErrorCode(wire.ErrCodeBlocked) {
		t.Fatalf("client close = %v, want ApplicationError %#x", err, wire.ErrCodeBlocked)
	}
}

// A same-client supersession (§4.4, newest exercise wins) closes the
// displaced session with ErrCodeSuperseded.
func TestE2ESupersededCarriesCode(t *testing.T) {
	sp := startSpace(t, nil)
	first := dial(t, sp, nil) // ClientID c1, entity e1
	waitCond(t, "admission", func() bool {
		_, ok := engineGet(sp.srv, "lasa.client.c1")
		return ok
	})
	_ = dial(t, sp, nil) // the same client id again: supersedes
	select {
	case <-first.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("displaced session not closed")
	}
	var appErr *quic.ApplicationError
	if err := first.Err(); !errors.As(err, &appErr) || appErr.ErrorCode != quic.ApplicationErrorCode(wire.ErrCodeSuperseded) {
		t.Fatalf("displaced close = %v, want ApplicationError %#x", err, wire.ErrCodeSuperseded)
	}
}
