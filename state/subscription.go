package state

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/panaudia/lasa/wire"
)

// Subscription is one client's state subscription: a gate verdict, a
// catch-up lane (iterator over the attach-time copy, closed by the
// frontier at the cut H), and a live lane (a pull cursor over the
// broadcast log). The lanes share exactly one fact — H — and never
// coordinate afterwards (invariants F1–F5).
type Subscription struct {
	e        *Engine
	clientID string
	prefixes []string
	rules    Rules

	verdict wire.BeginKind
	catchup []wire.StateMessage // Begin, entries…, Frontier{epoch, H}
	cutH    uint64

	mu             sync.Mutex
	catchupPos     int
	cursorNext     uint64 // next log seq to consume
	position       uint64 // last log seq processed (frontier basis, F3)
	pending        []wire.StateMessage
	lastFrontierAt time.Time
	termErr        error
	term           chan struct{}
}

// Attach establishes conn's state subscription (lasa-core.md §6.3: one per
// client — a live predecessor for the same client id is displaced
// atomically, invariant L4). The six-step gate runs inside the attach
// event, atomic with the cut (invariant F1). prefixes must be non-empty;
// cursor is nil for a fresh client.
func (e *Engine) Attach(c *Conn, prefixes []string, cursor *wire.Cursor) (*Subscription, error) {
	canonical := wire.CanonicalPrefixes(prefixes)
	if len(canonical) == 0 {
		return nil, fmt.Errorf("state: empty prefix set is a protocol error")
	}
	s := &Subscription{
		e:        e,
		clientID: c.clientID,
		prefixes: canonical,
		rules:    c.rules,
		term:     make(chan struct{}),
	}
	err := e.do(func() {
		if old := e.subs[c.clientID]; old != nil {
			old.terminate(ErrTerminated)
		}
		e.subs[c.clientID] = s

		// The six-step gate (lasa-core.md §6.6; gate 4 per
		// state-engine-design.md §5), then the atomic cut.
		verdict := e.gate(c, canonical, cursor)
		e.rulesSeen[c.clientID] = c.rules.readHash()

		s.verdict = verdict
		s.cutH = e.seq
		s.cursorNext = e.seq + 1
		s.position = e.seq
		s.lastFrontierAt = e.cfg.Clock()
		s.catchup = e.buildCatchUp(s, verdict, cursor)
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// gate returns the delivery verdict for a presented cursor.
func (e *Engine) gate(c *Conn, canonical []string, cursor *wire.Cursor) wire.BeginKind {
	if cursor == nil {
		return wire.BeginSnapshot // fresh client: no special case
	}
	switch {
	case cursor.Epoch != e.epoch: // 1: epoch mismatch
		return wire.BeginSnapshot
	case cursor.SetHash != wire.SetHash(canonical): // 2: set-hash mismatch
		return wire.BeginSnapshot
	case cursor.Seq < e.watermark: // 3: below the tombstone watermark
		return wire.BeginSnapshot
	case e.visibilityChanged(c): // 4: effective visibility unprovable
		return wire.BeginSnapshot
	case cursor.Seq > e.seq: // 5: ahead of head (corrupt client state)
		return wire.BeginSnapshot
	default: // 6: delta
		return wire.BeginDelta
	}
}

// visibilityChanged implements gate 4: snapshot unless the remembered
// read-rules hash for this identity equals the current one. Forgetting is
// always safe — an absent entry means "cannot prove", which is a snapshot,
// never a wrong delta.
func (e *Engine) visibilityChanged(c *Conn) bool {
	seen, ok := e.rulesSeen[c.clientID]
	return !ok || seen != c.rules.readHash()
}

// buildCatchUp computes the catch-up lane from the attach-time state
// (invariant F4: never the live map after the cut — this runs inside the
// attach event, so the copy IS the cut-time state). Deterministic key
// order for testability.
func (e *Engine) buildCatchUp(s *Subscription, verdict wire.BeginKind, cursor *wire.Cursor) []wire.StateMessage {
	msgs := []wire.StateMessage{wire.Begin{Kind: verdict}}
	var keys []string
	for k := range e.store {
		if subscriptionMatch(s.prefixes, k) && s.rules.MayRead(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		en := e.store[k]
		switch verdict {
		case wire.BeginSnapshot:
			// Snapshots never contain tombstones.
			if !en.tombstone {
				msgs = append(msgs, wire.Set{Key: k, Value: en.value, Seq: en.seq})
			}
		case wire.BeginDelta:
			// Latest op only, for keys past the cursor; compaction means
			// intermediate history is never delivered.
			if en.seq > cursor.Seq {
				if en.tombstone {
					msgs = append(msgs, wire.Clear{Key: k, Seq: en.seq})
				} else {
					msgs = append(msgs, wire.Set{Key: k, Value: en.value, Seq: en.seq})
				}
			}
		}
	}
	// Invariant F2: the frontier carries the fixed cut H — emitted even
	// when the delta is empty ("you were up to date").
	return append(msgs, wire.Frontier{Epoch: e.epoch, Seq: s.cutH})
}

// Detach ends the subscription. Idempotent, and addressed by instance: a
// late Detach of a displaced subscription cannot kill its successor
// (invariant L4). The transport MUST guarantee Detach on session close
// (invariant L5).
func (e *Engine) Detach(s *Subscription) {
	_ = e.do(func() {
		if e.subs[s.clientID] == s {
			delete(e.subs, s.clientID)
		}
		s.terminate(ErrTerminated)
	})
}

// Verdict reports the gate outcome: BeginSnapshot or BeginDelta.
func (s *Subscription) Verdict() wire.BeginKind { return s.verdict }

// CutSeq returns the cut H shared by both lanes.
func (s *Subscription) CutSeq() uint64 { return s.cutH }

// CatchUpNext returns the next catch-up message: Begin, then the entries,
// then the closing Frontier, then io.EOF.
func (s *Subscription) CatchUpNext(ctx context.Context) (wire.StateMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catchupPos >= len(s.catchup) {
		return nil, io.EOF
	}
	m := s.catchup[s.catchupPos]
	s.catchupPos++
	return m, nil
}

// LiveNext returns the next live-lane message: ops (with group boundaries
// preserved when all is well) and periodic frontiers. Backpressure is
// cursor lag; overrun returns ErrLagged, which terminates the
// subscription object only (invariant L3). Blocks until a message, ctx
// cancellation, or termination.
func (s *Subscription) LiveNext(ctx context.Context) (wire.StateMessage, error) {
	for {
		s.mu.Lock()
		if s.termErr != nil {
			err := s.termErr
			s.mu.Unlock()
			return nil, err
		}
		if len(s.pending) > 0 {
			m := s.pending[0]
			s.pending = s.pending[1:]
			s.mu.Unlock()
			return m, nil
		}
		next := s.cursorNext
		s.mu.Unlock()

		ch := s.e.log.waitCh()
		entries, lagged := s.e.log.read(next)
		if lagged {
			s.mu.Lock()
			if s.termErr == nil {
				s.termErr = ErrLagged
				close(s.term)
			}
			err := s.termErr
			s.mu.Unlock()
			return nil, err
		}
		if len(entries) > 0 {
			s.ingest(entries)
			continue
		}
		if f, ok := s.frontierDue(); ok {
			return f, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.term:
			continue // loop reports termErr
		case <-ch:
		}
	}
}

// ingest filters a batch of log entries through the subscription's scope
// and assembles pending messages, preserving group boundaries among the
// members that survive filtering.
func (s *Subscription) ingest(entries []logEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var group []wire.StateMessage
	inGroup := false
	for _, le := range entries {
		if le.groupFirst {
			inGroup = true
			group = group[:0]
		}
		if subscriptionMatch(s.prefixes, le.key) && s.rules.MayRead(le.key) {
			var m wire.StateMessage
			if le.tombstone {
				m = wire.Clear{Key: le.key, Seq: le.seq}
			} else {
				m = wire.Set{Key: le.key, Value: le.value, Seq: le.seq}
			}
			group = append(group, m)
		}
		if le.groupLast {
			inGroup = false
			switch len(group) {
			case 0: // fully filtered
			case 1:
				s.pending = append(s.pending, group[0])
			default:
				s.pending = append(s.pending, wire.Group{Ops: append([]wire.StateMessage(nil), group...)})
			}
			group = group[:0]
		}
		s.cursorNext = le.seq + 1
		s.position = le.seq
	}
	_ = inGroup // a log append is always whole groups; a batch cannot end mid-group
}

// frontierDue emits the periodic live-lane frontier: (epoch, cursor
// position) — position, never delivered content (invariant F3).
func (s *Subscription) frontierDue() (wire.StateMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.e.cfg.Clock()
	if now.Sub(s.lastFrontierAt) < s.e.cfg.FrontierInterval {
		return nil, false
	}
	s.lastFrontierAt = now
	return wire.Frontier{Epoch: s.e.epoch, Seq: s.position}, true
}

// terminate is called on the actor (or under the lagged path's own lock
// discipline) to end the subscription.
func (s *Subscription) terminate(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.termErr == nil {
		s.termErr = err
		close(s.term)
	}
}

// Err returns the terminal error, if the subscription has ended.
func (s *Subscription) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.termErr
}
