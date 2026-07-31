package server

// The moderation gate (design: plan/panaudia-server-design.md §7): the
// base profile's §5 block/kick enforcement, inside the shell — the one
// component that holds both the session table (kill capability) and
// the state engine (subscription source), keeping the single departure
// authority intact. Blocked keys do double duty (there is no separate
// kick): setting one severs matching live sessions AND gates admission
// until expiry; a kick is a block with a short expiry.
//
// Derived sets, not store reads: an engine observer maintains the
// blocked maps; the connect path checks memory. The admission check
// runs after ticket verification and BEFORE Backend.Admit — the
// incumbent KickGate lesson, kept: a blocked id's reconnect attempt
// can never reach the eviction logic and displace a live session.
//
// Wall-clock note (design §1): block expiry is unix-seconds by
// profile definition — a control-plane comparison on the admission
// and moderation paths, quarantined like the state engine's tombstone
// TTL; nothing here touches an audio path.

import (
	"strconv"
	"sync"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

const (
	blockedClientPrefix = "lasa.space.client.blocked."
	blockedRolePrefix   = "lasa.space.role.blocked."
	adminRole           = "admin"
)

type moderationGate struct {
	srv *Server

	mu      sync.Mutex
	clients map[string]int64 // blocked-until unix-seconds; 0 = forever
	roles   map[string]int64
}

func newModerationGate(srv *Server) (*moderationGate, error) {
	g := &moderationGate{
		srv:     srv,
		clients: map[string]int64{},
		roles:   map[string]int64{},
	}
	err := srv.engine.Observe(
		[]string{blockedClientPrefix, blockedRolePrefix}, g.onOp)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// onOp runs on the engine actor: update the derived sets, then sever
// matching live sessions. Severing is context cancellation only —
// teardown runs on the sessions' own goroutines, so the actor is never
// re-entered.
func (g *moderationGate) onOp(key string, value []byte, cleared bool) {
	var m map[string]int64
	var id string
	var byClient bool
	switch {
	case len(key) > len(blockedClientPrefix) && key[:len(blockedClientPrefix)] == blockedClientPrefix:
		m, id, byClient = g.clients, key[len(blockedClientPrefix):], true
	case len(key) > len(blockedRolePrefix) && key[:len(blockedRolePrefix)] == blockedRolePrefix:
		m, id = g.roles, key[len(blockedRolePrefix):]
		if id == adminRole {
			return // profile §5: admin is unblockable — invalid write, dropped
		}
	default:
		return
	}

	if cleared {
		g.mu.Lock()
		delete(m, id)
		g.mu.Unlock()
		return
	}
	until, ok := parseBlockUntil(value)
	if !ok {
		return // invalid value: dropped like any other (profile consumer rule)
	}
	g.mu.Lock()
	m[id] = until
	g.mu.Unlock()

	if effective(until, time.Now().Unix()) {
		g.srv.severMatching(func(clientID string, roles []string) bool {
			if hasRole(roles, adminRole) {
				return false // §5: never terminate an admin connection
			}
			if byClient {
				return clientID == id
			}
			return hasRole(roles, id)
		})
	}
}

// check gates admission. Runs on the connect path; memory only.
func (g *moderationGate) check(res *connect.Resolved) *connect.Rejection {
	if hasRole(res.Roles, adminRole) {
		return nil // §5: client.blocked entries are ignored for admin
	}
	now := time.Now().Unix()
	g.mu.Lock()
	defer g.mu.Unlock()
	if until, ok := g.clients[res.ClientID]; ok && effective(until, now) {
		return &connect.Rejection{Code: wire.ErrCodeBlocked, Reason: "client blocked"}
	}
	for _, r := range res.Roles {
		if until, ok := g.roles[r]; ok && effective(until, now) {
			return &connect.Rejection{Code: wire.ErrCodeBlocked, Reason: "role blocked"}
		}
	}
	return nil
}

// effective: 0 = forever; otherwise blocked until the timestamp.
func effective(until, now int64) bool { return until == 0 || now < until }

// parseBlockUntil reads the profile's Number ≥ 0 (JSON) value.
func parseBlockUntil(value []byte) (int64, bool) {
	f, err := strconv.ParseFloat(string(value), 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int64(f), true
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}
