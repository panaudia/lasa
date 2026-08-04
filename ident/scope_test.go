package ident

import "testing"

// The spec's §6.4 example hazards: plain-prefix would either hide a bare
// key from its own dotted scope or leak sibling ids.
func TestScopeMatches(t *testing.T) {
	cases := []struct {
		scope, key string
		want       bool
	}{
		{"foo", "foo", true},
		{"foo", "foo.bar", true},
		{"foo", "foo.bar.baz", true},
		{"foo", "foobar", false}, // the sibling-id leak plain-prefix would allow
		{"foo.", "foo", true},    // trailing dot: p′ = "foo", bare key matches its own scope
		{"foo.", "foo.bar", true},
		{"foo.", "foobar", false},
		{"", "anything", true}, // empty scope matches every key
		{"", "", true},
		{"lasa.entity", "lasa.entity.e1.name", true},
		{"lasa.entity.e1", "lasa.entity.e10", false},
		{"a.b", "a.b", true},
		{"a.b", "a.bc", false},
		{"a.b", "a", false},

		// Wildcard segments: * matches exactly one non-empty segment
		// (role expansion's any-entity grants, lasa-core.md §6.4).
		{"lasa.entity.*.dof", "lasa.entity.a1.dof", true},
		{"lasa.entity.*.dof", "lasa.entity.a1.dof.x", true},       // dot-extension still applies
		{"lasa.entity.*.dof", "lasa.entity.a1.render.dof", false}, // * spans exactly one segment
		{"lasa.entity.*.dof", "lasa.entity.a1", false},            // key exhausted mid-scope
		{"lasa.entity.*.dof", "lasa.entity.a1.dofx", false},       // sibling-id leak check holds
		{"lasa.entity.*.render.gain", "lasa.entity.e1.render.gain", true},
		{"lasa.entity.*.render.gain", "lasa.entity.e1.render", false},
		{"lasa.entity.*", "lasa.entity.a1", true},       // trailing wildcard, exact
		{"lasa.entity.*", "lasa.entity.a1.name", true},  // ...and its subtree
		{"lasa.entity.*", "lasa.entity", false},         // nothing for * to consume
		{"lasa.entity.*.", "lasa.entity.a1.name", true}, // trailing dot trims as usual
		{"*", "anything", true},                         // one segment
		{"*", "any.thing", true},                        // ...plus dot-extension
		{"*", "", false},                                // empty key gives * nothing to consume
		{"*.dof", "a1.dof", true},
		{"*.dof", "dof", false},
	}
	for _, c := range cases {
		if got := ScopeMatches(c.scope, c.key); got != c.want {
			t.Errorf("ScopeMatches(%q, %q) = %v, want %v", c.scope, c.key, got, c.want)
		}
	}
}
