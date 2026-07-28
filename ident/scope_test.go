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
	}
	for _, c := range cases {
		if got := ScopeMatches(c.scope, c.key); got != c.want {
			t.Errorf("ScopeMatches(%q, %q) = %v, want %v", c.scope, c.key, got, c.want)
		}
	}
}
