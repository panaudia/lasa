// Package clientstore implements the client half of the LASA state
// machinery (lasa-core.md §6.5–6.6): the local store, the catch-up window
// with the reference apply-live-with-transient-tombstones strategy, and
// cursor persistence honouring "never ahead of the persisted store"
// (invariant F7 of plan/state-engine-design.md: no cursor until the
// subscription's first catch-up frontier is applied).
package clientstore

import (
	"fmt"
	"time"

	"github.com/panaudia/lasa/wire"
)

// KV is one stored key: the latest value and its seq.
type KV struct {
	Value []byte
	Seq   uint64
}

// Snapshot is the persistable store state.
type Snapshot struct {
	Entries map[string]KV
	Cursor  *wire.Cursor
}

// Persistence stores the state and cursor ATOMICALLY — persisting a
// cursor ahead of the store it describes silently loses ops forever. Keyed
// by space id: a cursor earned in one space must never be presented to
// another. A nil Persistence makes the store ephemeral (a client that does
// not persist its store MUST NOT persist a cursor — and then presents
// nothing on reconnect).
type Persistence interface {
	Load(spaceID string) (*Snapshot, error) // nil, nil = nothing stored
	Save(spaceID string, s *Snapshot) error
}

// Store is a client state store for one subscription's scope.
type Store struct {
	spaceID  string
	prefixes []string // canonical
	persist  Persistence
	clock    func() time.Time

	entries map[string]KV
	cursor  *wire.Cursor
	// eligible: the first catch-up frontier of the current subscription
	// instance has been applied (invariant F7); until then no cursor may
	// be advanced or persisted.
	eligible bool

	// Catch-up window state (transient-tombstone strategy).
	inWindow    bool
	windowKind  wire.BeginKind
	windowStart time.Time
	transient   map[string]uint64 // key → live clear seq, incl. absent keys
	snapshotIn  map[string]bool   // keys delivered by the current snapshot
	liveMaxSeq  uint64            // max live frontier seen this instance
	epoch       [wire.EpochSize]byte
	haveEpoch   bool
}

// Option configures a Store.
type Option func(*Store)

// WithClock injects the clock used for the catch-up deadline.
func WithClock(clock func() time.Time) Option {
	return func(s *Store) { s.clock = clock }
}

// New creates a store for one space and prefix set, loading any persisted
// state. The prefix set is canonicalized.
func New(spaceID string, prefixes []string, p Persistence, opts ...Option) (*Store, error) {
	s := &Store{
		spaceID:  spaceID,
		prefixes: wire.CanonicalPrefixes(prefixes),
		persist:  p,
		clock:    time.Now,
		entries:  map[string]KV{},
	}
	for _, o := range opts {
		o(s)
	}
	if p != nil {
		snap, err := p.Load(spaceID)
		if err != nil {
			return nil, err
		}
		if snap != nil {
			if snap.Entries != nil {
				s.entries = snap.Entries
			}
			// Present the persisted cursor only if it was earned under the
			// same prefix set.
			if snap.Cursor != nil && snap.Cursor.SetHash == wire.SetHash(s.prefixes) {
				s.cursor = snap.Cursor
			}
		}
	}
	return s, nil
}

// SubscribeParams builds the subscribe message for a (re)connection: the
// prefix set plus the persisted cursor, if any.
func (s *Store) SubscribeParams() *wire.SubscribeParams {
	return &wire.SubscribeParams{Prefixes: s.prefixes, Cursor: s.cursor}
}

// Get returns the value for key.
func (s *Store) Get(key string) ([]byte, bool) {
	kv, ok := s.entries[key]
	return kv.Value, ok
}

// Len returns the number of live keys.
func (s *Store) Len() int { return len(s.entries) }

// Range calls fn for every live key.
func (s *Store) Range(fn func(key string, value []byte, seq uint64) bool) {
	for k, kv := range s.entries {
		if !fn(k, kv.Value, kv.Seq) {
			return
		}
	}
}

// ApplyLive applies one live-lane message. Live ops apply immediately
// under per-key dedup; during the catch-up window every live clear —
// including for keys not held — is recorded as a transient tombstone so a
// late catch-up chunk cannot resurrect it (lasa-core.md §6.5 note).
func (s *Store) ApplyLive(m wire.StateMessage) error {
	switch v := m.(type) {
	case wire.Set:
		s.liveSet(v)
	case wire.Clear:
		s.liveClear(v)
	case wire.Group:
		for _, op := range v.Ops {
			if err := s.ApplyLive(op); err != nil {
				return err
			}
		}
	case wire.Frontier:
		s.setEpoch(v.Epoch)
		if v.Seq > s.liveMaxSeq {
			s.liveMaxSeq = v.Seq
		}
		s.advanceCursor()
	default:
		return fmt.Errorf("clientstore: unexpected live message %T", m)
	}
	return nil
}

func (s *Store) liveSet(v wire.Set) {
	if kv, ok := s.entries[v.Key]; ok && v.Seq <= kv.Seq {
		return
	}
	if s.inWindow && s.transient[v.Key] >= v.Seq {
		return
	}
	s.entries[v.Key] = KV{Value: v.Value, Seq: v.Seq}
}

func (s *Store) liveClear(v wire.Clear) {
	if kv, ok := s.entries[v.Key]; ok && v.Seq <= kv.Seq {
		return
	}
	delete(s.entries, v.Key)
	if s.inWindow {
		s.transient[v.Key] = v.Seq
	}
}

// ApplyCatchUp applies one catch-up-lane message: Begin opens the window,
// entries stream lazily, and the closing Frontier finalizes — running the
// snapshot deletion rule, dropping the transient records, and earning the
// cursor (invariant F7).
func (s *Store) ApplyCatchUp(m wire.StateMessage) error {
	switch v := m.(type) {
	case wire.Begin:
		s.inWindow = true
		s.windowKind = v.Kind
		s.windowStart = s.clock()
		s.transient = map[string]uint64{}
		s.snapshotIn = map[string]bool{}
		return nil
	case wire.Set:
		if !s.inWindow {
			return fmt.Errorf("clientstore: catch-up entry before begin")
		}
		if s.windowKind == wire.BeginSnapshot {
			s.snapshotIn[v.Key] = true
		}
		if s.transient[v.Key] >= v.Seq {
			return nil // resurrection guard: cleared live at or after this seq
		}
		if kv, ok := s.entries[v.Key]; ok && v.Seq <= kv.Seq {
			return nil
		}
		s.entries[v.Key] = KV{Value: v.Value, Seq: v.Seq}
		return nil
	case wire.Clear:
		if !s.inWindow {
			return fmt.Errorf("clientstore: catch-up entry before begin")
		}
		if kv, ok := s.entries[v.Key]; ok && v.Seq > kv.Seq {
			delete(s.entries, v.Key)
		}
		return nil
	case wire.Frontier:
		if !s.inWindow {
			return fmt.Errorf("clientstore: catch-up frontier before begin")
		}
		s.setEpoch(v.Epoch)
		if s.windowKind == wire.BeginSnapshot {
			// Rule 3, snapshot only: delete every in-scope local key absent
			// from the snapshot whose seq ≤ the cut H.
			for k, kv := range s.entries {
				if !s.snapshotIn[k] && kv.Seq <= v.Seq {
					delete(s.entries, k)
				}
			}
		}
		s.inWindow = false
		s.transient = nil
		s.snapshotIn = nil
		s.eligible = true
		if v.Seq > s.liveMaxSeq {
			s.liveMaxSeq = v.Seq
		}
		s.advanceCursor()
		return s.save()
	default:
		return fmt.Errorf("clientstore: unexpected catch-up message %T", m)
	}
}

func (s *Store) setEpoch(e [wire.EpochSize]byte) {
	if !s.haveEpoch || s.epoch != e {
		// New epoch: any previously earned cursor is for a dead seq space.
		if s.haveEpoch && s.cursor != nil && s.cursor.Epoch != e {
			s.cursor = nil
		}
		s.epoch = e
		s.haveEpoch = true
	}
}

// advanceCursor moves the cursor to the max seq seen across both paths —
// only once eligible (invariant F7), and only within the current epoch.
func (s *Store) advanceCursor() {
	if !s.eligible || !s.haveEpoch {
		return
	}
	if s.cursor == nil || s.cursor.Epoch != s.epoch || s.liveMaxSeq > s.cursor.Seq {
		s.cursor = &wire.Cursor{Epoch: s.epoch, Seq: s.liveMaxSeq, SetHash: wire.SetHash(s.prefixes)}
	}
}

// Persist saves the store and cursor atomically, if a Persistence is
// configured and a cursor has been earned. Safe to call at any cadence.
func (s *Store) Persist() error { return s.save() }

func (s *Store) save() error {
	if s.persist == nil || !s.eligible {
		return nil
	}
	entries := make(map[string]KV, len(s.entries))
	for k, v := range s.entries {
		entries[k] = v
	}
	var cur *wire.Cursor
	if s.cursor != nil {
		c := *s.cursor
		cur = &c
	}
	return s.persist.Save(s.spaceID, &Snapshot{Entries: entries, Cursor: cur})
}

// Terminated informs the store its subscription instance ended (ErrLagged,
// displacement, transport close). The catch-up window state — including
// the transient records, which belong to the dead instance — is
// discarded; eligibility resets so the next instance must earn its own
// frontier before any cursor advances again.
func (s *Store) Terminated() {
	s.inWindow = false
	s.transient = nil
	s.snapshotIn = nil
	s.eligible = false
	s.liveMaxSeq = 0
}

// CatchUpStalled reports whether a catch-up window has been open longer
// than deadline — the cue to unsubscribe and re-subscribe fresh rather
// than hold a transient-record window forever.
func (s *Store) CatchUpStalled(deadline time.Duration) bool {
	return s.inWindow && s.clock().Sub(s.windowStart) > deadline
}
