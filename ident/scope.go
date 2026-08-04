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
//
// A scope may contain `*` segments, each matching exactly one
// non-empty key segment (identifiers never contain dots, so segment
// matching is unambiguous): lasa.entity.*.dof matches
// lasa.entity.a1.dof and its dot-extensions, never
// lasa.entity.a1.render.dof. Wildcards serve role expansion — ticket
// permission prefixes remain literal (lasa-core.md §6.4).
func ScopeMatches(scope, key string) bool {
	p := strings.TrimSuffix(scope, ".")
	if p == "" {
		return true
	}
	if !strings.Contains(p, "*") {
		return key == p || (strings.HasPrefix(key, p) && len(key) > len(p) && key[len(p)] == '.')
	}
	pi, ki := 0, 0
	for {
		pe, ke := segmentEnd(p, pi), segmentEnd(key, ki)
		if seg := p[pi:pe]; seg == "*" {
			if ke == ki {
				return false // a wildcard needs a segment to consume
			}
		} else if key[ki:ke] != seg {
			return false
		}
		pi, ki = pe, ke
		if pi == len(p) { // scope exhausted: exact or dot-extension
			return ki == len(key) || key[ki] == '.'
		}
		if ki == len(key) {
			return false // key exhausted with scope segments remaining
		}
		pi, ki = pi+1, ki+1 // step over both dots
	}
}

func segmentEnd(s string, i int) int {
	for i < len(s) && s[i] != '.' {
		i++
	}
	return i
}
