package base

import (
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
	g, err := AdmissionGroup(&connect.ResolvedEntity{
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
		"lasa.entity.e1":                 "true",
		"lasa.entity.e1.name":            `"Alice"`,
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
	g, err := AdmissionGroup(&connect.ResolvedEntity{
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

func TestAttrArraysRejected(t *testing.T) {
	_, err := AdmissionGroup(&connect.ResolvedEntity{
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
}
