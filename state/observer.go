package state

// Internal observers — the in-process consumption surface for profile
// enforcement (the server's moderation gate, an embedder's render-key
// wiring). Deliberately lighter than a Subscription: no gate, no
// cursors, no catch-up lanes — an observer sees applied ops as they
// land, plus a replay of the live store at registration so late
// registration misses nothing.

import "sort"

// Observer receives applied ops matching its prefixes, and the
// registration-time replay. It runs ON THE ENGINE ACTOR: keep it fast,
// never call back into the Engine from inside (use a goroutine for
// anything heavier than map updates and context cancels).
type Observer func(key string, value []byte, cleared bool)

type observerReg struct {
	prefixes []string
	fn       Observer
}

// Observe registers fn for ops whose key starts with any of prefixes
// (plain string prefixes, §6.3 semantics). Before returning it replays
// every live matching key in sorted order, so derived state can be
// rebuilt at any time. Observers cannot be removed — they are
// process-lifetime wiring, registered at construction.
func (e *Engine) Observe(prefixes []string, fn Observer) error {
	return e.do(func() {
		e.observers = append(e.observers, observerReg{prefixes: prefixes, fn: fn})
		var keys []string
		for k, en := range e.store {
			if !en.tombstone && prefixMatch(prefixes, k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			fn(k, e.store[k].value, false)
		}
	})
}

func prefixMatch(prefixes []string, key string) bool {
	for _, p := range prefixes {
		if len(key) >= len(p) && key[:len(p)] == p {
			return true
		}
	}
	return false
}

// notifyObservers runs on the actor, after an op is applied.
func (e *Engine) notifyObservers(key string, value []byte, cleared bool) {
	for _, o := range e.observers {
		if prefixMatch(o.prefixes, key) {
			o.fn(key, value, cleared)
		}
	}
}
