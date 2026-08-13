package state

// Adjudicated admission (id-capabilities.md; identity-implementation.md
// batch B). The claims "ledger" is not a component: claims are ordinary
// state keys, and this file is only the adjudication that applies a
// connection's admission writes atomically on the actor. The engine
// stays profile-blind — every key shape here (claim key, entity marker
// keys) is built by the caller from profile builders, and ownership is
// resolved through each admitted connection's recorded sweep scopes,
// never by parsing values.

import (
	"errors"
	"fmt"

	"github.com/panaudia/lasa/wire"
)

// ErrAtCapacity: the admission would exceed the space's entity capacity.
// Adjudicated here so every admission refusal issues from one place
// (maps to lasa-core.md §4.6 code 0x1A5A07).
var ErrAtCapacity = errors.New("state: space at capacity")

// EntityOwnedError: a claimed entity's record is live under a different
// client — refused in ticketed mode (core §4.4; maps to 0x1A5A04).
type EntityOwnedError struct{ Key string }

func (e *EntityOwnedError) Error() string {
	return fmt.Sprintf("state: entity record %s is live under another client", e.Key)
}

// AdmitRequest describes one connection's admission.
type AdmitRequest struct {
	ClientID string
	Rules    Rules

	// Ticketed selects the cross-owner collision rule (core §4.4):
	// refuse when true; supersede when false (unticketed newest-wins
	// is total — dev mode optimises for the restart loop).
	Ticketed bool

	// Capacity caps live admitted entities space-wide; 0 = unlimited.
	Capacity int

	// ClaimKey is the connection's claim record key and EntityKeys the
	// marker key per admitted entity — profile-built (base.ClientKey,
	// base.EntityMarkerKey); together they are the connection's claim
	// set and become its recorded sweep scopes.
	ClaimKey   string
	EntityKeys []string

	// Groups are the admission writes — the claim group first, then
	// one group per entity (base.ClientClaimGroup, base.AdmissionGroup)
	// — applied only if adjudication passes.
	Groups []wire.StateMessage
}

// Admit adjudicates and applies one connection's admission atomically on
// the actor:
//
//  1. Validate, side-effect-free: in ticketed mode a colliding live
//     entity record not covered by this client's own predecessor
//     refuses (EntityOwnedError); capacity is checked net of everything
//     supersession would free (ErrAtCapacity). A refused admission
//     mutates nothing.
//  2. Supersede: the client's own predecessor (same client id — the
//     newest exercise wins, both modes), and, unticketed only, every
//     connection owning a colliding entity record. Each goes through
//     the single close path: generation closed, claim set swept.
//  3. Apply the admission groups and register the connection.
//
// Sweep-before-apply inside one actor step is the core §4.4 ordering
// rule (and closes the review-resolutions §7 hazard: a sweep landing
// after the successor's writes would out-seq and delete them).
//
// The superseded list names the client ids whose connections were
// closed; the caller severs their transports. Each closed Conn's Done
// channel is the in-band signal to whatever holds it.
func (e *Engine) Admit(req AdmitRequest) (conn *Conn, superseded []string, err error) {
	scopes := make([]string, 0, len(req.EntityKeys)+1)
	scopes = append(scopes, req.EntityKeys...)
	scopes = append(scopes, req.ClaimKey)
	c := &Conn{
		e:           e,
		clientID:    req.ClientID,
		rules:       req.Rules,
		done:        make(chan struct{}),
		sweepScopes: scopes,
		entityCount: len(req.EntityKeys),
	}
	var admitErr error
	doErr := e.do(func() {
		pred := e.conns[req.ClientID]

		// Validate — no mutation on any refusal path.
		var evict []*Conn
		for _, k := range req.EntityKeys {
			en, ok := e.store[k]
			if !ok || en.tombstone {
				continue
			}
			if pred != nil && anyScope(pred.sweepScopes, k) {
				continue // freed when the own predecessor is superseded
			}
			if req.Ticketed {
				admitErr = &EntityOwnedError{Key: k}
				return
			}
			if owner := e.connOwning(k); owner != nil && owner != pred && !containsConn(evict, owner) {
				evict = append(evict, owner)
			}
			// A live record with no registered owner (legacy write) is
			// simply overwritten by the admission writes below.
		}
		if req.Capacity > 0 {
			freed := 0
			if pred != nil {
				freed += pred.entityCount
			}
			for _, o := range evict {
				freed += o.entityCount
			}
			if e.totalEntities-freed+len(req.EntityKeys) > req.Capacity {
				admitErr = ErrAtCapacity
				return
			}
		}

		// Mutate: supersede, then apply, then register.
		if pred != nil {
			e.closeConn(pred, pred.sweepScopes)
			superseded = append(superseded, pred.clientID)
		}
		for _, o := range evict {
			e.closeConn(o, o.sweepScopes)
			superseded = append(superseded, o.clientID)
		}
		for _, m := range req.Groups {
			switch v := m.(type) {
			case wire.Group:
				e.applyGroup(v.Ops)
			case wire.Set, wire.Clear:
				e.applyGroup([]wire.StateMessage{m})
			}
		}
		e.conns[req.ClientID] = c
		e.totalEntities += c.entityCount
	})
	if doErr != nil {
		return nil, nil, doErr
	}
	if admitErr != nil {
		return nil, nil, admitErr
	}
	return c, superseded, nil
}

// connOwning finds the admitted connection whose recorded sweep scopes
// cover key — ownership without value parsing. Actor-owned.
func (e *Engine) connOwning(key string) *Conn {
	for _, c := range e.conns {
		if anyScope(c.sweepScopes, key) {
			return c
		}
	}
	return nil
}

func containsConn(list []*Conn, c *Conn) bool {
	for _, v := range list {
		if v == c {
			return true
		}
	}
	return false
}
