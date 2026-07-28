package state

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/panaudia/lasa/wire"
)

type mclock struct {
	mu sync.Mutex
	t  time.Time
}

func newMclock() *mclock { return &mclock{t: time.Unix(1_800_000_000, 0)} }

func (c *mclock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *mclock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

var testEpoch = [8]byte{1, 2, 3, 4, 5, 6, 7, 8}

func newTestEngine(t *testing.T, mod func(*Config)) (*Engine, *mclock) {
	t.Helper()
	clk := newMclock()
	cfg := Config{Clock: clk.now, Epoch: &testEpoch}
	if mod != nil {
		mod(&cfg)
	}
	e := New(cfg)
	t.Cleanup(e.Close)
	return e, clk
}

// allAccess matches every key in every direction (the empty scope).
var allAccess = Rules{Set: []string{""}, Clear: []string{""}, Read: []string{""}}

func drainCatchUp(t *testing.T, s *Subscription) []wire.StateMessage {
	t.Helper()
	var out []wire.StateMessage
	for {
		m, err := s.CatchUpNext(context.Background())
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("catch-up: %v", err)
		}
		out = append(out, m)
	}
}

func liveNext(t *testing.T, s *Subscription) wire.StateMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, err := s.LiveNext(ctx)
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	return m
}

func cursorFor(e *Engine, prefixes []string, seq uint64) *wire.Cursor {
	return &wire.Cursor{Epoch: e.Epoch(), Seq: seq, SetHash: wire.SetHash(wire.CanonicalPrefixes(prefixes))}
}

func set(k, v string) wire.Set { return wire.Set{Key: k, Value: []byte(v)} }

// F2: an empty delta still closes with its frontier — the normal "you
// were up to date" signal.
func TestEmptyDeltaYieldsFrontier(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	_ = c.Write(set("a.x", "1"))
	prefixes := []string{""}

	s1, err := e.Attach(c, prefixes, nil)
	if err != nil {
		t.Fatal(err)
	}
	drainCatchUp(t, s1)
	h := s1.CutSeq()
	e.Detach(s1)

	s2, err := e.Attach(c, prefixes, cursorFor(e, prefixes, h))
	if err != nil {
		t.Fatal(err)
	}
	if s2.Verdict() != wire.BeginDelta {
		t.Fatalf("verdict = %v, want delta", s2.Verdict())
	}
	msgs := drainCatchUp(t, s2)
	if len(msgs) != 2 {
		t.Fatalf("empty delta = %#v, want [Begin, Frontier]", msgs)
	}
	f, ok := msgs[1].(wire.Frontier)
	if !ok || f.Seq != h || f.Epoch != e.Epoch() {
		t.Fatalf("frontier = %#v, want seq %d", msgs[1], h)
	}
}

// F1/F4: writes racing a lazy catch-up never leak past H; everything > H
// arrives on the live lane.
func TestCatchUpNeverLeaksPastH(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	_ = c.Write(set("a.x", "1"), set("a.y", "2"))

	s, err := e.Attach(c, []string{""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := s.CutSeq()
	_ = c.Write(set("a.z", "3"), set("a.x", "overwrite"))

	for _, m := range drainCatchUp(t, s) {
		if st, ok := m.(wire.Set); ok {
			if st.Seq > h {
				t.Errorf("catch-up leaked op past H: %+v", st)
			}
			if st.Key == "a.x" && string(st.Value) != "1" {
				t.Errorf("catch-up must reflect the cut-time value, got %q", st.Value)
			}
		}
	}
	if got := liveNext(t, s).(wire.Set); got.Key != "a.z" || got.Seq != h+1 {
		t.Fatalf("live = %+v, want a.z at seq %d", got, h+1)
	}
	if got := liveNext(t, s).(wire.Set); got.Key != "a.x" || string(got.Value) != "overwrite" {
		t.Fatalf("live = %+v, want the overwrite", got)
	}
}

// F3: ops outside the subscription's scope still advance its live
// frontier — the promise covers ops the subscriber will never see.
func TestOutOfScopeAdvancesLiveFrontier(t *testing.T) {
	e, clk := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	s, err := e.Attach(c, []string{"a."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	drainCatchUp(t, s)

	_ = c.Write(set("b.x", "hidden"))
	seq := e.Head()
	clk.advance(2 * time.Second)
	_ = e.Tick()
	f, ok := liveNext(t, s).(wire.Frontier)
	if !ok || f.Seq != seq {
		t.Fatalf("live = %#v, want frontier at %d (position, not content)", f, seq)
	}
}

// F5: boundary values at the gate — cursor exactly at the watermark or at
// head gets a delta; below the watermark gets a snapshot.
func TestGateBoundaries(t *testing.T) {
	e, clk := newTestEngine(t, func(c *Config) { c.TombstoneTTL = time.Minute })
	c, _ := e.Register("c1", allAccess)
	prefixes := []string{""}

	_ = c.Write(set("a.x", "1"))
	_ = c.Write(wire.Clear{Key: "a.x"})
	clearSeq := e.Head()
	_ = c.Write(set("a.y", "2"))

	// Earn gate-4 standing, then flush the tombstone.
	s0, _ := e.Attach(c, prefixes, nil)
	drainCatchUp(t, s0)
	e.Detach(s0)
	clk.advance(2 * time.Minute)
	_ = e.Tick() // watermark = clearSeq

	verdictAt := func(seq uint64) wire.BeginKind {
		s, err := e.Attach(c, prefixes, cursorFor(e, prefixes, seq))
		if err != nil {
			t.Fatal(err)
		}
		defer e.Detach(s)
		drainCatchUp(t, s)
		return s.Verdict()
	}
	if v := verdictAt(clearSeq - 1); v != wire.BeginSnapshot {
		t.Errorf("cursor below watermark: %v, want snapshot", v)
	}
	if v := verdictAt(clearSeq); v != wire.BeginDelta {
		t.Errorf("cursor == watermark: %v, want delta (inclusive frontier semantics)", v)
	}
	if v := verdictAt(e.Head()); v != wire.BeginDelta {
		t.Errorf("cursor == head: %v, want delta", v)
	}
	if v := verdictAt(e.Head() + 10); v != wire.BeginSnapshot {
		t.Errorf("cursor ahead of head: %v, want snapshot (corrupt client)", v)
	}
}

// F6 + group preservation: groups are applied as one actor event, a cut
// lands only between groups, and live delivery preserves boundaries.
func TestGroupDeliveredWhole(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	s, _ := e.Attach(c, []string{""}, nil)
	drainCatchUp(t, s)

	_ = c.Write(wire.Group{Ops: []wire.StateMessage{set("a.1", "x"), set("a.2", "y"), wire.Clear{Key: "a.1"}}})
	g, ok := liveNext(t, s).(wire.Group)
	if !ok || len(g.Ops) != 3 {
		t.Fatalf("live = %#v, want the whole group", g)
	}
	for i, op := range g.Ops {
		switch v := op.(type) {
		case wire.Set:
			if v.Seq == 0 {
				t.Errorf("op %d missing seq", i)
			}
		case wire.Clear:
			if v.Seq == 0 {
				t.Errorf("op %d missing seq", i)
			}
		}
	}
}

// L1: a straggler write from a swept generation is silently dropped — no
// orphan key survives the sweep.
func TestSweptGenerationWriteDropped(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	_ = c.Write(set("lasa.entity.e1", "true"), set("lasa.entity.e1.name", "Alice"))
	head := e.Head()

	_ = c.Close("lasa.entity.e1")
	sweepHead := e.Head()
	if sweepHead != head+2 {
		t.Fatalf("sweep should have cleared 2 keys, head %d → %d", head, sweepHead)
	}
	_ = c.Write(set("lasa.entity.e1.attrs.mood", "ghost"))
	if e.Head() != sweepHead {
		t.Fatal("write from swept generation must not land")
	}
}

// L2: the silent-drop causes produce no seq, no log entry, no delivery.
func TestSilentDrops(t *testing.T) {
	e, _ := newTestEngine(t, func(c *Config) { c.GroupOpCap = 2 })
	scoped, _ := e.Register("c1", Rules{Set: []string{"app.mine"}, Read: []string{""}})
	head := e.Head()

	_ = scoped.Write(set("app.other.k", "denied"))  // no grant
	_ = scoped.Write(set("App.Bad", "invalid key")) // invalid key
	_ = scoped.Write(wire.Set{Key: "app.mine.big", Value: make([]byte, wire.MaxValueLen+1)})
	_ = scoped.Write(wire.Group{Ops: []wire.StateMessage{ // over-cap group
		set("app.mine.a", "1"), set("app.mine.b", "2"), set("app.mine.c", "3"),
	}})
	_ = scoped.Write(wire.Group{Ops: []wire.StateMessage{ // one denied op drops the group whole
		set("app.mine.a", "1"), set("app.other.b", "2"),
	}})
	if e.Head() != head {
		t.Fatalf("head advanced %d → %d; some drop was not silent", head, e.Head())
	}
	_ = scoped.Write(set("app.mine.ok", "1"))
	if e.Head() != head+1 {
		t.Fatal("granted write must land")
	}
}

// L3: ErrLagged terminates the subscription object only — the connection,
// its keys, and the store are untouched, and resync works through the gate.
func TestLaggedTerminatesSubscriptionOnly(t *testing.T) {
	e, _ := newTestEngine(t, func(c *Config) { c.LogEntries = 4 })
	c, _ := e.Register("c1", allAccess)
	s, _ := e.Attach(c, []string{""}, nil)
	drainCatchUp(t, s)

	for i := 0; i < 10; i++ {
		_ = c.Write(set("a.k", "v"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.LiveNext(ctx); !errors.Is(err, ErrLagged) {
		t.Fatalf("err = %v, want ErrLagged", err)
	}

	// Connection unaffected: writes still land, store intact.
	_ = c.Write(set("a.after", "1"))
	if _, ok := storeGet(e, "a.k"); !ok {
		t.Fatal("store must be untouched by subscription lag")
	}
	// Resync through the gate: the delta comes from the map, not the log.
	s2, err := e.Attach(c, []string{""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(drainCatchUp(t, s2)); got < 3 {
		t.Fatalf("resync catch-up too small: %d messages", got)
	}
}

func storeGet(e *Engine, key string) (val []byte, ok bool) {
	_ = e.do(func() {
		if en, exists := e.store[key]; exists && !en.tombstone {
			val, ok = en.value, true
		}
	})
	return
}

// L4: Attach displaces a live predecessor for the same client; a late
// Detach of the displaced instance cannot kill the successor.
func TestDisplacement(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	s1, _ := e.Attach(c, []string{""}, nil)
	s2, _ := e.Attach(c, []string{""}, nil)

	if err := s1.Err(); !errors.Is(err, ErrTerminated) {
		t.Fatalf("displaced sub err = %v, want ErrTerminated", err)
	}
	e.Detach(s1) // late detach of the dead instance
	if s2.Err() != nil {
		t.Fatal("successor must survive the late Detach")
	}
	e.Detach(s2)
	e.Detach(s2) // idempotent
	if err := s2.Err(); !errors.Is(err, ErrTerminated) {
		t.Fatalf("detached sub err = %v", err)
	}
}

// Sweep-vs-replacement ordering: FIFO through the actor guarantees the
// evicted connection's sweep lands before the replacement's admission
// writes.
func TestSweepBeforeReplacementWrites(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	old, _ := e.Register("c1", allAccess)
	_ = old.Write(set("lasa.entity.e1", "true"), set("lasa.entity.e1.name", "Old"))

	_ = old.Close("lasa.entity.e1")
	_ = e.Write(set("lasa.entity.e1", "true"), set("lasa.entity.e1.name", "New"))

	v, ok := storeGet(e, "lasa.entity.e1.name")
	if !ok || string(v) != "New" {
		t.Fatalf("name = %q,%v — replacement writes must win", v, ok)
	}
}

// The sweep is one marker-first group: shortest keys clear first.
func TestSweepMarkerFirstGroup(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	obs, _ := e.Register("obs", allAccess)
	s, _ := e.Attach(obs, []string{""}, nil)
	drainCatchUp(t, s)

	c, _ := e.Register("c1", allAccess)
	_ = c.Write(set("lasa.entity.e1.attrs.mood", "up"), set("lasa.entity.e1", "true"), set("lasa.entity.e1.name", "A"))
	for i := 0; i < 3; i++ {
		liveNext(t, s)
	}
	_ = c.Close("lasa.entity.e1")

	g, ok := liveNext(t, s).(wire.Group)
	if !ok || len(g.Ops) != 3 {
		t.Fatalf("sweep = %#v, want one group of 3 clears", g)
	}
	first, ok := g.Ops[0].(wire.Clear)
	if !ok || first.Key != "lasa.entity.e1" {
		t.Fatalf("first sweep op = %#v, want the marker lasa.entity.e1", g.Ops[0])
	}
}

// Gate 4 via the rules-hash cache: a visibility change while disconnected
// forces a snapshot; unchanged rules keep the delta; an absent cache entry
// (forgetting) is always safe — snapshot.
func TestGate4RulesHash(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	prefixes := []string{""}
	narrow := Rules{Set: []string{""}, Clear: []string{""}, Read: []string{"app.public"}}
	wide := Rules{Set: []string{""}, Clear: []string{""}, Read: []string{""}}

	c1, _ := e.Register("c1", narrow)
	s, _ := e.Attach(c1, prefixes, nil)
	drainCatchUp(t, s)
	h := s.CutSeq()
	e.Detach(s)

	// Same rules on reconnect → delta.
	c1b, _ := e.Register("c1", narrow)
	s2, _ := e.Attach(c1b, prefixes, cursorFor(e, prefixes, h))
	if s2.Verdict() != wire.BeginDelta {
		t.Fatalf("unchanged rules: %v, want delta", s2.Verdict())
	}
	e.Detach(s2)

	// Widened rules (role definition changed while away) → snapshot.
	c1c, _ := e.Register("c1", wide)
	s3, _ := e.Attach(c1c, prefixes, cursorFor(e, prefixes, h))
	if s3.Verdict() != wire.BeginSnapshot {
		t.Fatalf("changed rules: %v, want snapshot", s3.Verdict())
	}
	e.Detach(s3)

	// A different identity with no cache entry presenting a plausible
	// cursor: cannot prove → snapshot.
	c2, _ := e.Register("c2", wide)
	s4, _ := e.Attach(c2, prefixes, cursorFor(e, prefixes, h))
	if s4.Verdict() != wire.BeginSnapshot {
		t.Fatalf("absent cache entry: %v, want snapshot", s4.Verdict())
	}
}

// Gates 1 and 2, and cross-space cursors: epoch or set-hash mismatch
// always snapshots.
func TestEpochAndSetHashGates(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, _ := e.Register("c1", allAccess)
	prefixes := []string{""}
	s, _ := e.Attach(c, prefixes, nil)
	drainCatchUp(t, s)
	h := s.CutSeq()
	e.Detach(s)

	wrongEpoch := cursorFor(e, prefixes, h)
	wrongEpoch.Epoch = [8]byte{9, 9, 9, 9, 9, 9, 9, 9}
	s2, _ := e.Attach(c, prefixes, wrongEpoch)
	if s2.Verdict() != wire.BeginSnapshot {
		t.Fatalf("wrong epoch: %v, want snapshot", s2.Verdict())
	}
	e.Detach(s2)

	wrongSet := cursorFor(e, []string{"other."}, h)
	s3, _ := e.Attach(c, prefixes, wrongSet)
	if s3.Verdict() != wire.BeginSnapshot {
		t.Fatalf("wrong set-hash: %v, want snapshot", s3.Verdict())
	}

	otherEpoch := [8]byte{7, 7, 7, 7, 7, 7, 7, 7}
	e2, _ := newTestEngine(t, func(cfg *Config) { cfg.Epoch = &otherEpoch })
	c2, _ := e2.Register("c1", allAccess)
	s4, _ := e2.Attach(c2, prefixes, cursorFor(e, prefixes, h)) // cursor from e presented to e2
	if s4.Verdict() != wire.BeginSnapshot {
		t.Fatalf("cross-space cursor: %v, want snapshot", s4.Verdict())
	}
}

// The echo corners: an op inside the writer's read scope echoes on its own
// sink; a write-without-read grant is accepted, visible to others, and
// never echoed (the documented ack-less grant shape).
func TestEchoCorners(t *testing.T) {
	e, clk := newTestEngine(t, nil)
	writer, _ := e.Register("w", Rules{Set: []string{""}, Read: []string{"app.mine"}})
	reader, _ := e.Register("r", allAccess)

	ws, _ := e.Attach(writer, []string{""}, nil)
	rs, _ := e.Attach(reader, []string{""}, nil)
	drainCatchUp(t, ws)
	drainCatchUp(t, rs)

	_ = writer.Write(set("app.mine.k", "echoed"))
	if got := liveNext(t, ws).(wire.Set); got.Key != "app.mine.k" {
		t.Fatalf("echo = %+v", got)
	}

	_ = writer.Write(set("app.hidden.k", "no echo"))
	if got := liveNext(t, rs); got.(wire.Set).Key != "app.mine.k" {
		t.Fatalf("reader first = %+v", got)
	}
	if got := liveNext(t, rs); got.(wire.Set).Key != "app.hidden.k" {
		t.Fatal("other subscribers must see the write-without-read op")
	}
	// The writer's next live message is a frontier (heartbeat), never the
	// hidden op — but the frontier's position covers it.
	hidden := e.Head()
	clk.advance(2 * time.Second)
	_ = e.Tick()
	f, ok := liveNext(t, ws).(wire.Frontier)
	if !ok || f.Seq != hidden {
		t.Fatalf("writer live = %#v, want frontier at %d", f, hidden)
	}
}

// GC: the TTL flush moves the watermark and a below-watermark cursor gets
// a snapshot (also covered at the boundary in TestGateBoundaries).
func TestGCFlushMovesWatermark(t *testing.T) {
	e, clk := newTestEngine(t, func(c *Config) { c.TombstoneTTL = time.Minute })
	c, _ := e.Register("c1", allAccess)
	_ = c.Write(set("a.k", "1"))
	_ = c.Write(wire.Clear{Key: "a.k"})
	clearSeq := e.Head()

	clk.advance(30 * time.Second)
	_ = e.Tick()
	var wm uint64
	_ = e.do(func() { wm = e.watermark })
	if wm != 0 {
		t.Fatalf("watermark moved before TTL: %d", wm)
	}
	clk.advance(31 * time.Second)
	_ = e.Tick()
	_ = e.do(func() { wm = e.watermark })
	if wm != clearSeq {
		t.Fatalf("watermark = %d, want %d", wm, clearSeq)
	}
}
