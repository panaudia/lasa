package base

import (
	"strings"
	"testing"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

func opsAsMap(t *testing.T, g wire.Group) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, op := range g.Ops {
		s, ok := op.(wire.Set)
		if !ok {
			t.Fatalf("admission group must contain only sets, got %T", op)
		}
		out[s.Key] = string(s.Value)
	}
	return out
}

func TestAdmissionGroupAdHoc(t *testing.T) {
	g, err := AdmissionGroup("c1", &connect.ResolvedEntity{
		Entity: connect.Entity{
			ID:   "e1",
			Name: "Alice",
			Attrs: map[string]any{
				"mood": "calm",
				"team": map[string]any{"name": "red", "rank": float64(3)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := opsAsMap(t, g)
	want := map[string]string{
		"lasa.entity.e1":                 `"c1"`, // marker value = owning client id (§6)
		"lasa.entity.e1.name":            `"Alice"`,
		"lasa.entity.e1.heard-in.main":   "true", // omitted list → effective default (§4)
		"lasa.entity.e1.hears.main":      "true",
		"lasa.entity.e1.attrs.mood":      `"calm"`,
		"lasa.entity.e1.attrs.team.name": `"red"`,
		"lasa.entity.e1.attrs.team.rank": "3",
	}
	if len(m) != len(want) {
		t.Fatalf("ops = %v", m)
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	if first, ok := g.Ops[0].(wire.Set); !ok || first.Key != "lasa.entity.e1" {
		t.Error("the existence marker must be the group's first op")
	}
}

func TestAdmissionGroupSigned(t *testing.T) {
	g, err := AdmissionGroup("c1", &connect.ResolvedEntity{
		Entity: connect.Entity{
			ID:    "e1",
			Name:  "Alice",
			Attrs: map[string]any{"verified": true},
		},
		Signed:     true,
		SetupAttrs: map[string]any{"mood": "up"},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := opsAsMap(t, g)
	if m["lasa.entity.e1.signed-attrs.verified"] != "true" {
		t.Error("ticket-definition attrs must land under signed-attrs")
	}
	if m["lasa.entity.e1.attrs.mood"] != `"up"` {
		t.Error("setup attrs must land under attrs (unsigned)")
	}
}

// TestAdmissionGroupMaterializesConfig: the config-derived consumer
// vocabulary lands in the admission group — effective memberships
// (an empty list defaults, like an omitted one), set render fields,
// priority, dof.
func TestAdmissionGroupMaterializesConfig(t *testing.T) {
	gain, size, dof := 2.5, 0.4, 3
	g, err := AdmissionGroup("c1", &connect.ResolvedEntity{
		Entity: connect.Entity{
			ID:       "e1",
			Name:     "Desk",
			Priority: true,
			HeardIn:  []string{"stage", "aux"},
			Hears:    []string{}, // empty is not "none" — it defaults
			Render:   &connect.Render{Gain: &gain, Size: &size},
			Dof:      &dof,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := opsAsMap(t, g)
	want := map[string]string{
		"lasa.entity.e1":                `"c1"`,
		"lasa.entity.e1.name":           `"Desk"`,
		"lasa.entity.e1.heard-in.stage": "true",
		"lasa.entity.e1.heard-in.aux":   "true",
		"lasa.entity.e1.hears.main":     "true",
		"lasa.entity.e1.priority":       "true",
		"lasa.entity.e1.render.gain":    "2.5",
		"lasa.entity.e1.render.size":    "0.4",
		"lasa.entity.e1.dof":            "3",
	}
	if len(m) != len(want) {
		t.Fatalf("ops = %v", m)
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	// The materialized values must round-trip through the consumer parse.
	for k, v := range m {
		if _, err := ParseOp(k, []byte(v), false); err != nil {
			t.Errorf("ParseOp(%s, %s): %v", k, v, err)
		}
	}
}

func TestAttrArraysRejected(t *testing.T) {
	_, err := AdmissionGroup("c1", &connect.ResolvedEntity{
		Entity: connect.Entity{ID: "e1", Name: "A", Attrs: map[string]any{"tags": []any{"x"}}},
	})
	if err == nil {
		t.Fatal("arrays in attrs must be rejected")
	}
}

func TestScopeCoversSubtree(t *testing.T) {
	// The sweep scope must cover the marker and every dotted descendant.
	if SweepScope("e1") != "lasa.entity.e1" {
		t.Fatal("sweep scope must be the marker key")
	}
	if ClientSweepScope("c1") != "lasa.client.c1" {
		t.Fatal("client sweep scope must be the claim key")
	}
}

// TestClientClaimGroup: the connection-level claim record — one Set,
// lasa.client.{id} = true, its own group (identity-implementation.md
// batch A: never shares a group with an attr-heavy entity).
func TestClientClaimGroup(t *testing.T) {
	g := ClientClaimGroup("c1")
	if len(g.Ops) != 1 {
		t.Fatalf("claim group must hold exactly one op, got %d", len(g.Ops))
	}
	s, ok := g.Ops[0].(wire.Set)
	if !ok || s.Key != "lasa.client.c1" || string(s.Value) != "true" {
		t.Fatalf("claim op = %+v", g.Ops[0])
	}
	// The claim key must be invisible to the audience read scope (§2):
	// no participant-facing prefix may match it.
	for _, prefix := range []string{"lasa.entity.", "lasa.space."} {
		if len(s.Key) >= len(prefix) && s.Key[:len(prefix)] == prefix {
			t.Fatalf("claim key %s under participant-facing prefix %s", s.Key, prefix)
		}
	}
}

// E1 (spec-audit 2026-08-21): the store's value cap applies to
// admission writes — an oversized config attr rejects at group build.
func TestAdmissionGroupValueCap(t *testing.T) {
	big := strings.Repeat("x", wire.MaxValueLen)
	_, err := AdmissionGroup("c1", &connect.ResolvedEntity{
		Entity: connect.Entity{ID: "e1", Name: "n", Attrs: map[string]any{"blob": big}},
	})
	if err == nil {
		t.Error("oversized attr value must reject the admission group")
	}
}
