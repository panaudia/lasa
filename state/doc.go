// Package state implements the LASA state engine (lasa-core.md §6): the
// server-side store with actor-owned writes, a bounded broadcast log for
// live fan-out, two-lane delivery (catch-up + live) per subscription, the
// six-step reconnect gate, tombstone GC, and the disconnect sweep.
//
// Design and invariants: plan/state-engine-design.md. One Engine instance
// serves one space. The engine speaks wire structs (wire.Set, wire.Clear,
// wire.Group, wire.Frontier, wire.Begin) and never encodes bytes — the
// transport shell applies package wire at the edge.
//
// Two matchers, deliberately distinct (see plan/state-engine-design.md §5
// and lasa-core.md §6.3/§6.4): subscription scope uses plain string
// prefixes; access rules use ident.ScopeMatches (exact-or-dot-extended).
package state
