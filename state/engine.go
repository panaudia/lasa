package state

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/panaudia/lasa/ident"
	"github.com/panaudia/lasa/wire"
)

var (
	// ErrLagged: the subscription's log cursor fell off the broadcast
	// window. Terminates the subscription object ONLY (invariant L3) —
	// the client resyncs through the gate. Never feed this into any
	// connection-lifecycle path.
	ErrLagged = errors.New("state: subscription lagged past the broadcast window")

	// ErrTerminated: the subscription was ended by Detach, displacement
	// (a newer Attach for the same client, invariant L4), or engine close.
	ErrTerminated = errors.New("state: subscription terminated")

	// ErrClosed: the engine has been closed.
	ErrClosed = errors.New("state: engine closed")
)

// Config configures an Engine. Zero values take the documented defaults.
type Config struct {
	LogEntries       int           // broadcast window, entries (default 4096)
	LogBytes         int           // broadcast window, bytes (default 16 MiB)
	GroupOpCap       int           // max ops per client-written group (default 64)
	TombstoneTTL     time.Duration // wall-clock tombstone retention (default 6h)
	FrontierInterval time.Duration // live-lane frontier cadence (default 1s)
	Clock            func() time.Time
	Epoch            *[wire.EpochSize]byte // test override; nil = crypto/rand
}

func (c *Config) defaults() {
	if c.LogEntries == 0 {
		c.LogEntries = 4096
	}
	if c.LogBytes == 0 {
		c.LogBytes = 16 << 20
	}
	if c.GroupOpCap == 0 {
		c.GroupOpCap = 64
	}
	if c.TombstoneTTL == 0 {
		c.TombstoneTTL = 6 * time.Hour
	}
	if c.FrontierInterval == 0 {
		c.FrontierInterval = time.Second
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
}

// entry is one key's compacted state: the latest value, or a tombstone
// awaiting GC.
type entry struct {
	value     []byte
	seq       uint64
	tombstone bool
	deadAt    time.Time // tombstone creation time, for TTL
}

// Engine is the per-space state engine. All mutations execute serially on
// the actor goroutine; exported methods are synchronous.
type Engine struct {
	cfg   Config
	epoch [wire.EpochSize]byte
	log   *broadcastLog

	msgs chan func()
	stop chan struct{}

	// Actor-owned state — touched only from run().
	store     map[string]*entry
	seq       uint64
	watermark uint64
	subs      map[string]*Subscription // one per client id (invariant L4)
	rulesSeen map[string][32]byte      // gate 4: last-attach read-rules hash per client id
	observers []observerReg            // internal profile consumers (observer.go)

	// Admission registry (admit.go): one admitted connection per client
	// id, carrying its recorded sweep scopes — how supersession finds
	// what an incumbent holds without the engine knowing key shapes.
	conns         map[string]*Conn
	totalEntities int // live admitted entities, for the capacity check
}

// New creates an engine and starts its actor.
func New(cfg Config) *Engine {
	cfg.defaults()
	e := &Engine{
		cfg:       cfg,
		log:       newBroadcastLog(cfg.LogEntries, cfg.LogBytes),
		msgs:      make(chan func()),
		stop:      make(chan struct{}),
		store:     map[string]*entry{},
		subs:      map[string]*Subscription{},
		rulesSeen: map[string][32]byte{},
		conns:     map[string]*Conn{},
	}
	if cfg.Epoch != nil {
		e.epoch = *cfg.Epoch
	} else if _, err := rand.Read(e.epoch[:]); err != nil {
		panic(fmt.Sprintf("state: epoch entropy unavailable: %v", err))
	}
	go e.run()
	return e
}

func (e *Engine) run() {
	for {
		select {
		case fn := <-e.msgs:
			fn()
		case <-e.stop:
			return
		}
	}
}

// do runs fn on the actor and waits for it. Returns ErrClosed after Close.
func (e *Engine) do(fn func()) error {
	done := make(chan struct{})
	select {
	case e.msgs <- func() { fn(); close(done) }:
		<-done
		return nil
	case <-e.stop:
		return ErrClosed
	}
}

// Epoch returns the engine's epoch (8 opaque bytes, crypto-random per
// instance start).
func (e *Engine) Epoch() [wire.EpochSize]byte { return e.epoch }

// Head returns the current head seq.
func (e *Engine) Head() uint64 {
	var h uint64
	_ = e.do(func() { h = e.seq })
	return h
}

// Close stops the engine, terminating every live subscription.
func (e *Engine) Close() {
	_ = e.do(func() {
		for _, s := range e.subs {
			s.terminate(ErrClosed)
		}
		e.subs = map[string]*Subscription{}
		close(e.stop)
	})
}

// Conn is a registered connection: the write-side identity (invariants
// L1, L2). One generation; closing it (the sweep) is final.
type Conn struct {
	e        *Engine
	clientID string
	rules    Rules
	closed   bool // actor-owned

	// done closes with the generation — the supersession signal
	// (id-capabilities.md): a session holding this Conn learns it was
	// displaced by observing its own generation end, never by callback.
	done chan struct{}

	// Recorded by Admit: the connection's full claim set as sweep
	// scopes (claim key + entity markers) and its entity count (the
	// capacity unit). Nil/zero for legacy Register connections.
	sweepScopes []string
	entityCount int
}

// Register registers a connection at admission, carrying its client id
// and its merged access rules (state-engine-design.md §3).
//
// Register performs no adjudication and records no claim — it is the
// pre-identity-work path, kept for tests and transitional callers; the
// admission path is Engine.Admit (admit.go).
func (e *Engine) Register(clientID string, rules Rules) (*Conn, error) {
	c := &Conn{e: e, clientID: clientID, rules: rules, done: make(chan struct{})}
	if err := e.do(func() {}); err != nil {
		return nil, err
	}
	return c, nil
}

// ClientID returns the connection's client id.
func (c *Conn) ClientID() string { return c.clientID }

// Done closes when the connection's generation ends — by its own Close
// or by supersession (a newer exercise of the same capability,
// lasa-core.md §4.4).
func (c *Conn) Done() <-chan struct{} { return c.done }

// Write applies client-authored ops (Set, Clear, or one Group per
// message). All rejections are silent, per lasa-core.md §6.2: denied by
// rules, no matching grant, value over 64 KiB, over-cap group, or a
// closed (swept) generation. It returns only engine-lifecycle errors.
func (c *Conn) Write(msgs ...wire.StateMessage) error {
	return c.e.do(func() {
		if c.closed {
			return // invariant L1: no writes from a swept generation
		}
		for _, m := range msgs {
			c.e.applyClient(c, m)
		}
	})
}

// Close closes the connection's generation and performs the disconnect
// sweep: one server-authored group clearing every live key matching the
// given scopes (exact-or-dot-extended), shortest key first so a scope's
// root marker clears before its subtree (the base profile's marker-first
// SHOULD). Atomic with closing the generation: no later write from this
// connection can land after the sweep. Called with no scopes, an
// Admit-created connection sweeps its recorded claim set; a legacy
// Register connection sweeps nothing.
func (c *Conn) Close(sweepScopes ...string) error {
	return c.e.do(func() {
		scopes := sweepScopes
		if len(scopes) == 0 {
			scopes = c.sweepScopes
		}
		c.e.closeConn(c, scopes)
	})
}

// closeConn is the single close path — Conn.Close and Admit's
// supersession both land here. Actor-owned.
func (e *Engine) closeConn(c *Conn, scopes []string) {
	if c.closed {
		return
	}
	c.closed = true
	close(c.done)
	if e.conns[c.clientID] == c {
		delete(e.conns, c.clientID)
		e.totalEntities -= c.entityCount
	}
	var keys []string
	for k, en := range e.store {
		if !en.tombstone && anyScope(scopes, k) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	if len(keys) == 0 {
		return
	}
	ops := make([]wire.StateMessage, len(keys))
	for i, k := range keys {
		ops[i] = wire.Clear{Key: k}
	}
	// Chunked: a large sweep must not become an over-cap group on the
	// wire. Shortest-key-first ordering puts the markers in the first
	// chunk, so the ignore-when-absent rule covers readers mid-sweep.
	e.applyChunked(ops)
}

// Write applies server-authored ops (profile consumer writes: existence
// markers, moderation enforcement). No grant checks, but the
// log-soundness caps are enforced: an invalid key or over-cap value is
// an error — embedder misuse should be loud, not a silent client-style
// drop — and a group larger than the op cap is applied as consecutive
// chunks within one actor event, so delivery never sees an over-cap
// group from any author.
func (e *Engine) Write(msgs ...wire.StateMessage) error {
	for _, m := range msgs {
		if err := validateServerOps(m); err != nil {
			return err
		}
	}
	return e.do(func() {
		for _, m := range msgs {
			switch v := m.(type) {
			case wire.Group:
				e.applyChunked(v.Ops)
			case wire.Set, wire.Clear:
				e.applyGroup([]wire.StateMessage{m})
			default:
				_ = v // frontier/begin are not ops; drop
			}
		}
	})
}

// validateServerOps holds server-authored ops to the same key and value
// bounds as client writes (spec §2/§9).
func validateServerOps(m wire.StateMessage) error {
	check := func(op wire.StateMessage) error {
		switch v := op.(type) {
		case wire.Set:
			if !ident.ValidKey(v.Key) {
				return fmt.Errorf("state: invalid key %q", v.Key)
			}
			if len(v.Value) > wire.MaxValueLen {
				return fmt.Errorf("state: value for %q exceeds %d bytes", v.Key, wire.MaxValueLen)
			}
		case wire.Clear:
			if !ident.ValidKey(v.Key) {
				return fmt.Errorf("state: invalid key %q", v.Key)
			}
		}
		return nil
	}
	if g, ok := m.(wire.Group); ok {
		for _, op := range g.Ops {
			if err := check(op); err != nil {
				return err
			}
		}
		return nil
	}
	return check(m)
}

// applyChunked applies ops as consecutive groups of at most GroupOpCap.
// The group cap exists because an over-large group is a forced-resync
// primitive against every subscriber's delivery window — which is true
// regardless of author, so server-side groups (sweeps, admissions)
// chunk rather than exempt themselves. Atomicity is the caller's actor
// event, not the group: consecutive seqs and no attach cut inside are
// preserved across the chunks. Actor-owned.
func (e *Engine) applyChunked(ops []wire.StateMessage) {
	for len(ops) > e.cfg.GroupOpCap {
		e.applyGroup(ops[:e.cfg.GroupOpCap])
		ops = ops[e.cfg.GroupOpCap:]
	}
	e.applyGroup(ops)
}

// applyClient enforces the silent-drop causes for one client message.
// A group is validated whole and dropped whole (state-engine-design.md
// §3): partial application would betray the applied-together hint.
func (e *Engine) applyClient(c *Conn, m wire.StateMessage) {
	switch v := m.(type) {
	case wire.Group:
		if len(v.Ops) > e.cfg.GroupOpCap {
			return
		}
		for _, op := range v.Ops {
			if !e.clientOpAllowed(c, op) {
				return
			}
		}
		e.applyGroup(v.Ops)
	case wire.Set, wire.Clear:
		if !e.clientOpAllowed(c, m) {
			return
		}
		e.applyGroup([]wire.StateMessage{m})
	default:
		// frontier/begin have no upstream meaning; drop.
	}
}

func (e *Engine) clientOpAllowed(c *Conn, m wire.StateMessage) bool {
	switch v := m.(type) {
	case wire.Set:
		return ident.ValidKey(v.Key) && len(v.Value) <= wire.MaxValueLen && c.rules.MaySet(v.Key)
	case wire.Clear:
		return ident.ValidKey(v.Key) && c.rules.MayClear(v.Key)
	default:
		return false
	}
}

// applyGroup assigns seqs, updates the compacted map, and appends one
// atomic run to the broadcast log (invariant F6: a cut never lands inside
// it, since attach and this both execute on the actor).
func (e *Engine) applyGroup(ops []wire.StateMessage) {
	if len(ops) == 0 {
		return
	}
	now := e.cfg.Clock()
	entries := make([]logEntry, 0, len(ops))
	for i, op := range ops {
		e.seq++
		le := logEntry{
			seq:        e.seq,
			groupFirst: i == 0,
			groupLast:  i == len(ops)-1,
		}
		switch v := op.(type) {
		case wire.Set:
			e.store[v.Key] = &entry{value: v.Value, seq: e.seq}
			le.key, le.value = v.Key, v.Value
			e.notifyObservers(v.Key, v.Value, false)
		case wire.Clear:
			e.store[v.Key] = &entry{seq: e.seq, tombstone: true, deadAt: now}
			le.key, le.tombstone = v.Key, true
			e.notifyObservers(v.Key, nil, true)
		}
		entries = append(entries, le)
	}
	e.log.append(entries...)
}

// Tick runs housekeeping against the injected clock: tombstone GC (the
// watermark advances over every flushed tombstone) and a reader wake so
// blocked live lanes re-check their frontier deadlines.
func (e *Engine) Tick() error {
	err := e.do(func() {
		now := e.cfg.Clock()
		for k, en := range e.store {
			if en.tombstone && now.Sub(en.deadAt) >= e.cfg.TombstoneTTL {
				if en.seq > e.watermark {
					e.watermark = en.seq
				}
				delete(e.store, k)
			}
		}
	})
	e.log.wake()
	return err
}
