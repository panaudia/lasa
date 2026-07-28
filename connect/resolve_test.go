package connect

import (
	"testing"

	"github.com/panaudia/lasa/wire"
)

func mustParse(t *testing.T, s string) *ConnectionConfig {
	t.Helper()
	cfg, rej := ParseConnectionConfig([]byte(s))
	if rej != nil {
		t.Fatalf("parse: %v", rej)
	}
	return cfg
}

func ticketWith(entities ...Entity) *Ticket {
	return &Ticket{
		ClientID:    "c1",
		Roles:       []string{"presenter"},
		Permissions: Permissions{Read: []string{"app."}},
		Entities:    entities,
	}
}

func TestResolveUnticketedAdHoc(t *testing.T) {
	cfg := mustParse(t, `{"client_id":"c1","entities":[{"id":"e1","name":"Alice"},{"id":"e2","name":"Bob"}]}`)
	r, rej := Resolve(cfg, nil, false)
	if rej != nil {
		t.Fatal(rej)
	}
	if r.Ticketed || len(r.Entities) != 2 || r.Entities[0].Signed {
		t.Fatalf("resolved = %+v", r)
	}
	if r.Roles != nil {
		t.Error("unticketed connections carry no roles at this layer")
	}
}

func TestResolveZeroEntityObserver(t *testing.T) {
	cfg := mustParse(t, `{"client_id":"c1","entities":[]}`)
	r, rej := Resolve(cfg, nil, false)
	if rej != nil {
		t.Fatal(rej)
	}
	if len(r.Entities) != 0 {
		t.Fatalf("resolved = %+v", r)
	}
}

func TestResolveSetupsInstantiateSubset(t *testing.T) {
	cfg := mustParse(t, `{"client_id":"c1","ticket":"x.y.z",
		"setups":[{"id":"e1","attrs":{"mood":"calm"},
		           "initial-pose":{"position":{"x":9,"y":0,"z":0}}}]}`)
	tk := ticketWith(
		Entity{ID: "e1", Name: "Alice", InitialPose: &PoseSpec{Position: &Position{X: 1}}},
		Entity{ID: "e2", Name: "Bob"}, // not instantiated — subset is permitted
	)
	r, rej := Resolve(cfg, tk, false)
	if rej != nil {
		t.Fatal(rej)
	}
	if len(r.Entities) != 1 {
		t.Fatalf("entities = %+v", r.Entities)
	}
	e := r.Entities[0]
	if !e.Signed || e.Name != "Alice" {
		t.Errorf("entity = %+v", e)
	}
	if e.InitialPose == nil || e.InitialPose.Position.X != 9 {
		t.Error("setup initial-pose must override the ticket definition's default")
	}
	if e.SetupAttrs["mood"] != "calm" {
		t.Error("setup attrs must be carried unsigned")
	}
	if r.Roles[0] != "presenter" || !r.Permissions.MayRead("app.x") {
		t.Error("roles/permissions must pass through")
	}
}

func TestResolveRejections(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		ticket   *Ticket
		adHoc    bool
		wantCode uint32
	}{
		{
			"client_id mismatch",
			`{"client_id":"c2","ticket":"x.y.z","setups":[{"id":"e1"}]}`,
			ticketWith(Entity{ID: "e1", Name: "A"}), false,
			wire.ErrCodeClientIDMismatch,
		},
		{
			"unknown setup id",
			`{"client_id":"c1","ticket":"x.y.z","setups":[{"id":"ghost"}]}`,
			ticketWith(Entity{ID: "e1", Name: "A"}), false,
			wire.ErrCodeMalformedConfig,
		},
		{
			"duplicate setup id",
			`{"client_id":"c1","ticket":"x.y.z","setups":[{"id":"e1"},{"id":"e1"}]}`,
			ticketWith(Entity{ID: "e1", Name: "A"}), false,
			wire.ErrCodeMalformedConfig,
		},
		{
			"ad-hoc not permitted for ticket",
			`{"client_id":"c1","ticket":"x.y.z","entities":[{"id":"x1","name":"X"}]}`,
			ticketWith(Entity{ID: "e1", Name: "A"}), false,
			wire.ErrCodeAdHocNotPermitted,
		},
		{
			"ad-hoc shadows signed definition",
			`{"client_id":"c1","ticket":"x.y.z","setups":[{"id":"e1"}],"entities":[{"id":"e2","name":"X"}]}`,
			ticketWith(Entity{ID: "e1", Name: "A"}, Entity{ID: "e2", Name: "B"}), true,
			wire.ErrCodeMalformedConfig,
		},
		{
			"duplicate ad-hoc id",
			`{"client_id":"c1","entities":[{"id":"x1","name":"X"},{"id":"x1","name":"Y"}]}`,
			nil, false,
			wire.ErrCodeMalformedConfig,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rej := Resolve(mustParse(t, c.json), c.ticket, c.adHoc)
			if rej == nil {
				t.Fatal("want rejection")
			}
			if rej.Code != c.wantCode {
				t.Errorf("code = %#x, want %#x (%s)", rej.Code, c.wantCode, rej.Reason)
			}
		})
	}
}

func TestResolveAdHocPermittedWithTicket(t *testing.T) {
	cfg := mustParse(t, `{"client_id":"c1","ticket":"x.y.z",
		"setups":[{"id":"e1"}],"entities":[{"id":"x1","name":"X"}]}`)
	r, rej := Resolve(cfg, ticketWith(Entity{ID: "e1", Name: "A"}), true)
	if rej != nil {
		t.Fatal(rej)
	}
	if len(r.Entities) != 2 || !r.Entities[0].Signed || r.Entities[1].Signed {
		t.Fatalf("entities = %+v", r.Entities)
	}
}
