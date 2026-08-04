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

	// App grants unrestricted access to application space — every key
	// outside the reserved "lasa." profile prefix (lasa-core.md §1.1).
	// The base profile grants it to unticketed connections; ticketed
	// connections reach application space only via explicit permission
	// scopes.
	App bool
}

func (r Rules) MaySet(key string) bool   { return r.appSpace(key) || anyScope(r.Set, key) }
func (r Rules) MayClear(key string) bool { return r.appSpace(key) || anyScope(r.Clear, key) }
func (r Rules) MayRead(key string) bool  { return r.appSpace(key) || anyScope(r.Read, key) }

func (r Rules) appSpace(key string) bool {
	return r.App && !ident.ScopeMatches("lasa", key)
}

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
// convenience. The App grant changes visibility, so it must land in the
// hash; "\x00" cannot appear in a key, making the sentinel collision-free.
func (r Rules) readHash() [32]byte {
	read := r.Read
	if r.App {
		read = append(append([]string(nil), read...), "\x00app")
	}
	return wire.SetHash(wire.CanonicalPrefixes(read))
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
