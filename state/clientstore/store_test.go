package clientstore

import (
	"sync"
	"testing"
	"time"

	"github.com/panaudia/lasa/wire"
)

var epochA = [8]byte{1, 1, 1, 1, 1, 1, 1, 1}
var epochB = [8]byte{2, 2, 2, 2, 2, 2, 2, 2}

type tclock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tclock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *tclock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newStore(t *testing.T, p Persistence) (*Store, *tclock) {
	t.Helper()
	clk := &tclock{t: time.Unix(1_800_000_000, 0)}
	s, err := New("space1", []string{""}, p, WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	return s, clk
}

func mustLive(t *testing.T, s *Store, m wire.StateMessage) {
	t.Helper()
	if err := s.ApplyLive(m); err != nil {
		t.Fatal(err)
	}
}

func mustCatchUp(t *testing.T, s *Store, m wire.StateMessage) {
	t.Helper()
	if err := s.ApplyCatchUp(m); err != nil {
		t.Fatal(err)
	}
}

// The resurrection trap (lasa-core.md §6.5 note): a live clear during the
// window, then a late catch-up chunk carrying the same key at an older
// seq. The transient record must block it — including for keys the client
// never held.
func TestResurrectionGuard(t *testing.T) {
	s, _ := newStore(t, nil)
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginDelta})

	// Live: key created at 980 elsewhere, cleared at 1005 — the client
	// never held it.
	mustLive(t, s, wire.Clear{Key: "app.k", Seq: 1005})
	// Late catch-up chunk carries the stale set at 980.
	mustCatchUp(t, s, wire.Set{Key: "app.k", Value: []byte("stale"), Seq: 980})
	if _, ok := s.Get("app.k"); ok {
		t.Fatal("resurrected: transient tombstone must block the stale set")
	}
	// A genuinely newer live set still wins.
	mustLive(t, s, wire.Set{Key: "app.k", Value: []byte("new"), Seq: 1010})
	if v, ok := s.Get("app.k"); !ok || string(v) != "new" {
		t.Fatalf("newer live set must apply, got %q,%v", v, ok)
	}
	// The records drop at the frontier: a post-window catch-up would be a
	// protocol error anyway, but the window state must be gone.
	mustCatchUp(t, s, wire.Frontier{Epoch: epochA, Seq: 1100})
	if s.inWindow || s.transient != nil {
		t.Fatal("window state must drop at the frontier")
	}
}

// Snapshot rule 3: on completion, delete every in-scope local key absent
// from the snapshot whose seq ≤ the cut — but a key set live during the
// window (seq > H is impossible for absence… seq > H means live, which
// survives).
func TestSnapshotDeletionRule(t *testing.T) {
	s, _ := newStore(t, nil)
	// Stale local state from a previous epoch/instance.
	s.entries["old.a"] = KV{Value: []byte("x"), Seq: 5}
	s.entries["old.b"] = KV{Value: []byte("y"), Seq: 8}

	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginSnapshot})
	mustCatchUp(t, s, wire.Set{Key: "old.a", Value: []byte("fresh"), Seq: 20})
	// Live op lands mid-window for a key the snapshot does not carry.
	mustLive(t, s, wire.Set{Key: "live.c", Seq: 40, Value: []byte("live")})
	mustCatchUp(t, s, wire.Frontier{Epoch: epochA, Seq: 30})

	if _, ok := s.Get("old.b"); ok {
		t.Fatal("old.b absent from snapshot with seq ≤ H must be deleted")
	}
	if v, ok := s.Get("old.a"); !ok || string(v) != "fresh" {
		t.Fatalf("old.a = %q,%v", v, ok)
	}
	if _, ok := s.Get("live.c"); !ok {
		t.Fatal("live.c (seq > H) must survive the deletion rule")
	}
}

// Invariant F7: live frontiers arriving before the catch-up frontier must
// not earn a cursor; crashing in the window loses nothing because nothing
// was persisted.
func TestCursorNotBeforeFirstCatchUpFrontier(t *testing.T) {
	p := NewMemPersistence()
	s, _ := newStore(t, p)
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginSnapshot})
	mustLive(t, s, wire.Frontier{Epoch: epochA, Seq: 500}) // live frontier first

	if s.SubscribeParams().Cursor != nil {
		t.Fatal("cursor earned before the catch-up frontier")
	}
	if snap, _ := p.Load("space1"); snap != nil {
		t.Fatal("nothing may be persisted before the catch-up frontier")
	}

	mustCatchUp(t, s, wire.Frontier{Epoch: epochA, Seq: 300})
	cur := s.SubscribeParams().Cursor
	if cur == nil || cur.Seq != 500 {
		t.Fatalf("cursor = %+v, want max across both paths (500)", cur)
	}
	if snap, _ := p.Load("space1"); snap == nil || snap.Cursor == nil {
		t.Fatal("frontier must persist store+cursor atomically")
	}
}

// Subscription termination mid-window: transient records belong to the
// dead instance and are discarded; eligibility resets.
func TestTerminatedDiscardsWindow(t *testing.T) {
	s, _ := newStore(t, nil)
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginDelta})
	mustLive(t, s, wire.Clear{Key: "app.k", Seq: 100})
	s.Terminated()

	if s.transient != nil || s.inWindow {
		t.Fatal("window state must be discarded on termination")
	}
	// New instance: fresh window; the old transient must not block a
	// legitimate snapshot entry.
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginSnapshot})
	mustCatchUp(t, s, wire.Set{Key: "app.k", Value: []byte("v"), Seq: 90})
	if _, ok := s.Get("app.k"); !ok {
		t.Fatal("stale transient from a dead instance blocked a snapshot entry")
	}
}

func TestCatchUpStalled(t *testing.T) {
	s, clk := newStore(t, nil)
	if s.CatchUpStalled(time.Second) {
		t.Fatal("no window open")
	}
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginDelta})
	clk.advance(2 * time.Second)
	if !s.CatchUpStalled(time.Second) {
		t.Fatal("stall must be reported")
	}
}

// Persistence round trip, including the set-hash guard: a cursor earned
// under a different prefix set must not be presented.
func TestPersistRoundTrip(t *testing.T) {
	p := NewMemPersistence()
	s, _ := newStore(t, p)
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginSnapshot})
	mustCatchUp(t, s, wire.Set{Key: "app.k", Value: []byte("v"), Seq: 7})
	mustCatchUp(t, s, wire.Frontier{Epoch: epochA, Seq: 10})

	s2, err := New("space1", []string{""}, p)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := s2.Get("app.k"); !ok || string(v) != "v" {
		t.Fatalf("restored = %q,%v", v, ok)
	}
	if c := s2.SubscribeParams().Cursor; c == nil || c.Seq != 10 || c.Epoch != epochA {
		t.Fatalf("restored cursor = %+v", c)
	}

	// Different prefix set: the persisted cursor is not presentable.
	s3, err := New("space1", []string{"app."}, p)
	if err != nil {
		t.Fatal(err)
	}
	if s3.SubscribeParams().Cursor != nil {
		t.Fatal("cursor earned under a different prefix set must be dropped")
	}
}

// An epoch change invalidates the cursor; the new epoch's frontier earns a
// fresh one.
func TestEpochChange(t *testing.T) {
	s, _ := newStore(t, nil)
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginSnapshot})
	mustCatchUp(t, s, wire.Frontier{Epoch: epochA, Seq: 10})
	if s.SubscribeParams().Cursor == nil {
		t.Fatal("cursor not earned")
	}

	s.Terminated() // reconnect
	mustCatchUp(t, s, wire.Begin{Kind: wire.BeginSnapshot})
	mustCatchUp(t, s, wire.Frontier{Epoch: epochB, Seq: 3})
	c := s.SubscribeParams().Cursor
	if c == nil || c.Epoch != epochB || c.Seq != 3 {
		t.Fatalf("cursor = %+v, want fresh epoch B cursor", c)
	}
}
