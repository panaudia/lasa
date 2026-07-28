package ident

import "strings"

// ScopeMatches implements access-rule matching (lasa-core.md §6.4): a
// scope p matches key k iff k = p′ or k begins with p′ + ".", where p′ is
// p without any trailing dot. This is deliberately NOT the plain string
// prefix used for subscription filtering (§6.3) — plain-prefix matching
// here would either hide a bare key from its own dotted scope or leak
// sibling ids ("foo" plain-prefix-matches "foobar").
//
// The empty scope matches every key: p′ is empty and every key begins
// with "" — and the dot-extension rule degenerates benignly since keys
// never start with a dot in practice; an explicit check keeps it exact.
func ScopeMatches(scope, key string) bool {
	p := strings.TrimSuffix(scope, ".")
	if p == "" {
		return true
	}
	return key == p || (strings.HasPrefix(key, p) && len(key) > len(p) && key[len(p)] == '.')
}
