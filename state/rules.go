package state

import (
	"github.com/panaudia/lasa/ident"
	"github.com/panaudia/lasa/wire"
)

// Rules are a connection's effective state-access rules: the merged union
// of its roles' expanded rules and its ticket's permissions, computed by
// the caller (profile layer) before registration. Scopes match
// exact-or-dot-extended (ident.ScopeMatches), never plain-prefix.
type Rules struct {
	Set   []string
	Clear []string
	Read  []string
}

func (r Rules) MaySet(key string) bool   { return anyScope(r.Set, key) }
func (r Rules) MayClear(key string) bool { return anyScope(r.Clear, key) }
func (r Rules) MayRead(key string) bool  { return anyScope(r.Read, key) }

func anyScope(scopes []string, key string) bool {
	for _, s := range scopes {
		if ident.ScopeMatches(s, key) {
			return true
		}
	}
	return false
}

// readHash is the gate-4 effective-visibility hash: a canonical hash of
// the fully-expanded read-rule set. The encoding is engine-internal — it
// never crosses the wire — but reuses the set-hash construction for
// convenience.
func (r Rules) readHash() [32]byte {
	return wire.SetHash(wire.CanonicalPrefixes(r.Read))
}

// subscriptionMatch is the §6.3 subscription filter: PLAIN string prefix —
// "foo" matches "foobar" as well as "foo.bar"; keys are strings and the
// dotted structure is convention. Not to be confused with access-rule
// matching.
func subscriptionMatch(prefixes []string, key string) bool {
	for _, p := range prefixes {
		if len(key) >= len(p) && key[:len(p)] == p {
			return true
		}
	}
	return false
}
