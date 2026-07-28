package state

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/panaudia/lasa/state/clientstore"
	"github.com/panaudia/lasa/wire"
)

// The simulation harness runs the engine and clientstore against each
// other THROUGH the wire codec — every message is encoded downstream and
// re-parsed, so the sim also exercises the §6.7 layouts end to end.

func pipeDown(t *testing.T, m wire.StateMessage) wire.StateMessage {
	t.Helper()
	buf, err := wire.AppendStateMessage(nil, m, wire.Downstream)
	if err != nil {
		t.Fatalf("encode downstream: %v", err)
	}
	back, err := wire.ParseStateMessage(buf, wire.Downstream)
	if err != nil {
		t.Fatalf("parse downstream: %v", err)
	}
	return back
}

func pipeUp(t *testing.T, m wire.StateMessage) wire.StateMessage {
	t.Helper()
	buf, err := wire.AppendStateMessage(nil, m, wire.Upstream)
	if err != nil {
		t.Fatalf("encode upstream: %v", err)
	}
	back, err := wire.ParseStateMessage(buf, wire.Upstream)
	if err != nil {
		t.Fatalf("parse upstream: %v", err)
	}
	return back
}

// syncCatchUp drains the catch-up lane into the client store via the wire.
func syncCatchUp(t *testing.T, s *Subscription, cs *clientstore.Store) {
	t.Helper()
	for {
		m, err := s.CatchUpNext(context.Background())
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.ApplyCatchUp(pipeDown(t, m)); err != nil {
			t.Fatal(err)
		}
	}
}

// syncLive pumps n live messages into the client store via the wire.
func syncLive(t *testing.T, s *Subscription, cs *clientstore.Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		m, err := s.LiveNext(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.ApplyLive(pipeDown(t, m)); err != nil {
			t.Fatal(err)
		}
	}
}

// assertConverged compares the client store against the engine's live map
// within the client's scope.
func assertConverged(t *testing.T, e *Engine, cs *clientstore.Store) {
	t.Helper()
	server := map[string]string{}
	_ = e.do(func() {
		for k, en := range e.store {
			if !en.tombstone {
				server[k] = string(en.value)
			}
		}
	})
	if cs.Len() != len(server) {
		t.Errorf("client has %d keys, server %d", cs.Len(), len(server))
	}
	cs.Range(func(k string, v []byte, _ uint64) bool {
		if server[k] != string(v) {
			t.Errorf("key %q: client %q, server %q", k, v, server[k])
		}
		return true
	})
}

func TestSimFreshSyncAndLive(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	writer, _ := e.Register("w", allAccess)
	_ = writer.Write(set("app.a", "1"), set("app.b", "2"))
	_ = writer.Write(wire.Clear{Key: "app.b"})

	reader, _ := e.Register("r", allAccess)
	cs, _ := clientstore.New("space1", []string{""}, nil)
	sub, err := e.Attach(reader, cs.SubscribeParams().Prefixes, cs.SubscribeParams().Cursor)
	if err != nil {
		t.Fatal(err)
	}
	syncCatchUp(t, sub, cs)
	assertConverged(t, e, cs)

	// Client-authored writes go up through the wire and come back live.
	_ = writer.Write(pipeUp(t, set("app.c", "3")).(wire.Set))
	_ = writer.Write(pipeUp(t, wire.Group{Ops: []wire.StateMessage{set("app.d", "4"), wire.Clear{Key: "app.a"}}}).(wire.Group))
	syncLive(t, sub, cs, 2)
	assertConverged(t, e, cs)
}

// The F7 crash scenario end to end: live frontiers arrive beyond H while
// the catch-up lane is still streaming; the client crashes before the
// catch-up frontier. Because nothing was persisted, the reconnect is
// fresh — a snapshot — and no op is lost.
func TestSimCrashMidCatchUp(t *testing.T) {
	e, clk := newTestEngine(t, nil)
	writer, _ := e.Register("w", allAccess)
	_ = writer.Write(set("app.a", "1"), set("app.b", "2"))

	p := clientstore.NewMemPersistence()
	reader, _ := e.Register("r", allAccess)
	cs, _ := clientstore.New("space1", []string{""}, p)
	sub, err := e.Attach(reader, cs.SubscribeParams().Prefixes, cs.SubscribeParams().Cursor)
	if err != nil {
		t.Fatal(err)
	}

	// Begin arrives; entries are still lazily streaming...
	m, _ := sub.CatchUpNext(context.Background())
	if err := cs.ApplyCatchUp(pipeDown(t, m)); err != nil {
		t.Fatal(err)
	}
	// ...while live ops and a live frontier land beyond H.
	_ = writer.Write(set("app.live", "x"))
	syncLive(t, sub, cs, 1) // the op
	clk.advance(2 * time.Second)
	_ = e.Tick()
	syncLive(t, sub, cs, 1) // a live frontier past H

	if cs.SubscribeParams().Cursor != nil {
		t.Fatal("client must not hold a cursor before the catch-up frontier")
	}

	// Crash: new store from persistence. Nothing was persisted, so the
	// reconnect presents no cursor and lands on a snapshot.
	cs2, _ := clientstore.New("space1", []string{""}, p)
	if cs2.SubscribeParams().Cursor != nil {
		t.Fatal("persistence must be empty after a mid-catch-up crash")
	}
	reader2, _ := e.Register("r", allAccess)
	sub2, err := e.Attach(reader2, cs2.SubscribeParams().Prefixes, cs2.SubscribeParams().Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if sub2.Verdict() != wire.BeginSnapshot {
		t.Fatalf("verdict = %v, want snapshot", sub2.Verdict())
	}
	syncCatchUp(t, sub2, cs2)
	assertConverged(t, e, cs2)
}

// Reconnect with a persisted cursor: the delta path, including a clear
// that happened while away, through the full wire round trip.
func TestSimReconnectDelta(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	writer, _ := e.Register("w", allAccess)
	_ = writer.Write(set("app.a", "1"), set("app.b", "2"))

	p := clientstore.NewMemPersistence()
	reader, _ := e.Register("r", allAccess)
	cs, _ := clientstore.New("space1", []string{""}, p)
	sub, _ := e.Attach(reader, cs.SubscribeParams().Prefixes, cs.SubscribeParams().Cursor)
	syncCatchUp(t, sub, cs)
	e.Detach(sub)

	// While away: an overwrite, a new key, a deletion.
	_ = writer.Write(set("app.a", "updated"), set("app.new", "3"))
	_ = writer.Write(wire.Clear{Key: "app.b"})

	cs2, _ := clientstore.New("space1", []string{""}, p)
	params := cs2.SubscribeParams()
	if params.Cursor == nil {
		t.Fatal("cursor must have been persisted")
	}
	reader2, _ := e.Register("r", allAccess)
	sub2, _ := e.Attach(reader2, params.Prefixes, params.Cursor)
	if sub2.Verdict() != wire.BeginDelta {
		t.Fatalf("verdict = %v, want delta", sub2.Verdict())
	}
	syncCatchUp(t, sub2, cs2)
	assertConverged(t, e, cs2)
	if _, ok := cs2.Get("app.b"); ok {
		t.Fatal("the while-away clear must reach the client via the delta")
	}
}
