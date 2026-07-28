package connect

import (
	"testing"

	"github.com/panaudia/lasa/wire"
)

func TestParseConnectionConfigValid(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"entities only", `{"client_id":"c1","entities":[{"id":"e1","name":"Alice"}]}`},
		{"zero-entity observer", `{"client_id":"c1","entities":[]}`},
		{"setups with ticket", `{"client_id":"c1","ticket":"x.y.z","setups":[{"id":"e1"}]}`},
		{
			"both, full entity",
			`{"client_id":"c1","ticket":"x.y.z",
			  "setups":[{"id":"e1","attrs":{"mood":"calm"},
			             "initial-pose":{"position":{"x":1,"y":2,"z":3}}}],
			  "entities":[{"id":"e2","name":"Bob","priority":true,
			               "sources":["main","stage"],"sinks":["main"],
			               "attrs":{"team":{"name":"red","rank":3}},
			               "frame":"head","dof":3,
			               "initial-pose":{"attitude":{"yaw":0,"pitch":0,"roll":0}},
			               "render":{"gain":1.5,"attenuation":2.0,"size":0.5,"directivity":0.8}}]}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, rej := ParseConnectionConfig([]byte(c.json))
			if rej != nil {
				t.Fatalf("rejected: %v", rej)
			}
			if cfg.ClientID != "c1" {
				t.Errorf("client_id = %q", cfg.ClientID)
			}
		})
	}

	cfg, rej := ParseConnectionConfig([]byte(`{"client_id":"c1","entities":[]}`))
	if rej != nil {
		t.Fatal(rej)
	}
	if !cfg.HasEntities() || cfg.HasSetups() {
		t.Error("presence flags: want HasEntities && !HasSetups")
	}
}

func TestParseConnectionConfigRejects(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"not json", `{`},
		{"missing client_id", `{"entities":[]}`},
		{"neither entities nor setups", `{"client_id":"c1"}`},
		{"setups without ticket", `{"client_id":"c1","setups":[{"id":"e1"}]}`},
		{"unknown top-level field", `{"client_id":"c1","entities":[],"extra":1}`},
		{"bad client_id grammar", `{"client_id":"C1","entities":[]}`},
		{"entity missing name", `{"client_id":"c1","entities":[{"id":"e1"}]}`},
		{"entity bad dof", `{"client_id":"c1","entities":[{"id":"e1","name":"a","dof":4}]}`},
		{"entity bad frame", `{"client_id":"c1","entities":[{"id":"e1","name":"a","frame":"screen"}]}`},
		{"render gain out of range", `{"client_id":"c1","entities":[{"id":"e1","name":"a","render":{"gain":3.5}}]}`},
		{"attrs array invalid", `{"client_id":"c1","entities":[{"id":"e1","name":"a","attrs":{"tags":["x"]}}]}`},
		{"attr name grammar", `{"client_id":"c1","entities":[{"id":"e1","name":"a","attrs":{"Bad":"x"}}]}`},
		{"pose missing z", `{"client_id":"c1","entities":[{"id":"e1","name":"a","initial-pose":{"position":{"x":1,"y":2}}}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rej := ParseConnectionConfig([]byte(c.json))
			if rej == nil {
				t.Fatal("want rejection, got nil")
			}
			if rej.Code != wire.ErrCodeMalformedConfig {
				t.Errorf("code = %#x, want ErrCodeMalformedConfig", rej.Code)
			}
		})
	}
}
