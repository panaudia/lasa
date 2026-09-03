package conformance

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestTicketReuseAndSubset covers C-4.2-2 (a setup may instantiate a
// subset of the ticket's entities) and C-4.5-1 (the same ticket admits
// a reconnection until exp). Ticketed only.
func TestTicketReuseAndSubset(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		if !f.Ticketed {
			NotExercised(t, "C-4.2-2", "unticketed fixture")
			NotExercised(t, "C-4.5-1", "unticketed fixture")
			return
		}
		two := []connect.Entity{{ID: "sub-e1", Name: "A"}, {ID: "sub-e2", Name: "B"}}
		tok := mint(t, f, f.IssuerKey, connect.MintClaims{Issuer: f.Issuer, SpaceID: f.SpaceID, ClientID: "sub-c", Entities: two, IssuedAt: time.Now()})
		obs, _ := observer(t, f, "sub-obs", []string{"lasa.entity.sub-e"}, connect.Permissions{})
		cfg := configFor(t, f, party{clientID: "sub-c", entities: two, ticket: tok})
		cfg.Setups = []connect.Setup{{ID: "sub-e1"}}
		c, err := dial(t, f, cfg)
		if err != nil {
			t.Fatalf("subset dial: %v", err)
		}
		waitCond(t, "e1 marker", func() bool { _, ok := storeGet(obs, "lasa.entity.sub-e1"); return ok })
		_, e2 := storeGet(obs, "lasa.entity.sub-e2")
		Check(t, "C-4.2-2", !e2, "an entity the setups did not name was instantiated")
		_ = c.Close()
		waitCond(t, "e1 swept", func() bool { _, ok := storeGet(obs, "lasa.entity.sub-e1"); return !ok })
		c2, err := dial(t, f, cfg)
		Check(t, "C-4.5-1", err == nil, "the same ticket was refused on reconnection: %v", err)
		if c2 != nil {
			_ = c2.Close()
		}
	})
}

// TestRoles covers P-2-5 (presenter carries ad-hoc entities), P-2-8
// (producer subscribes any sink) and P-2-9 (admin is the union).
func TestRoles(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		if !f.Ticketed {
			for _, id := range []string{"P-2-5", "P-2-8", "P-2-9"} {
				NotExercised(t, id, "unticketed fixture")
			}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		adhoc := []connect.Entity{{ID: "roles-adhoc", Name: "P"}}

		cfg := configFor(t, f, party{clientID: "roles-presenter", entities: []connect.Entity{}, roles: []string{"presenter"}})
		cfg.Entities, cfg.Setups = adhoc, nil
		p, err := dial(t, f, cfg)
		Check(t, "P-2-5", err == nil, "presenter's ad-hoc entities refused: %v", err)
		if p != nil {
			_ = p.Close()
		}

		_ = mustDial(t, f, party{clientID: "roles-owner", entities: []connect.Entity{{ID: "roles-e", Name: "O"}}})
		producer := mustDial(t, f, party{clientID: "roles-producer", entities: []connect.Entity{}, roles: []string{"producer"}})
		rt, err := producer.SubscribeSink(ctx, "roles-e", "binaural", func(uint64, []byte) {})
		Check(t, "P-2-8", err == nil, "producer refused another entity's sink: %v", err)
		if rt != nil {
			_ = rt.Close()
		}

		cfgA := configFor(t, f, party{clientID: "roles-admin", entities: []connect.Entity{}, roles: []string{"admin"}})
		cfgA.Entities, cfgA.Setups = []connect.Entity{{ID: "roles-admin-adhoc", Name: "A"}}, nil
		admin := syncingClientCfg(t, f, cfgA, []string{"lasa.entity.roles-e"})
		rtA, errSink := admin.SubscribeSink(ctx, "roles-e", "binaural", func(uint64, []byte) {})
		if rtA != nil {
			defer rtA.Close()
		}
		write(t, admin,
			wire.Set{Key: "lasa.entity.roles-e.render.gain", Value: []byte("0.5")}, // producer grant
			wire.Set{Key: "lasa.entity.roles-e.dof", Value: []byte("3")})           // moderator grant
		t.Cleanup(func() {
			write(t, admin, wire.Clear{Key: "lasa.entity.roles-e.render.gain"}, wire.Clear{Key: "lasa.entity.roles-e.dof"})
		})
		both := waitCond(t, "admin's producer and moderator writes echoed", func() bool {
			_, a := storeGet(admin, "lasa.entity.roles-e.render.gain")
			_, b := storeGet(admin, "lasa.entity.roles-e.dof")
			return a && b
		})
		Check(t, "P-2-9", errSink == nil && both, "admin union: sink %v, writes %v", errSink, both)
	})
}

// TestBlockReasonExpiry covers C-4.6-6: the blocked rejection's reason
// SHOULD carry the block's expiry timestamp.
func TestBlockReasonExpiry(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		if !f.Ticketed {
			NotExercised(t, "C-4.6-6", "unticketed fixture")
			return
		}
		admin := mustDial(t, f, party{clientID: "expiry-admin", entities: []connect.Entity{}, roles: []string{"admin"}})
		until := time.Now().Add(time.Hour).Unix()
		key := "lasa.space.client.blocked.expiry-victim"
		write(t, admin, wire.Set{Key: key, Value: []byte(strconv.FormatInt(until, 10))})
		t.Cleanup(func() { write(t, admin, wire.Clear{Key: key}); time.Sleep(100 * time.Millisecond) })
		time.Sleep(100 * time.Millisecond)
		code, reason := rejectionReason(t, f, configFor(t, f, party{clientID: "expiry-victim", entities: []connect.Entity{{ID: "expiry-e", Name: "V"}}}))
		if code != wire.ErrCodeBlocked {
			t.Fatalf("code %#x, want blocked", code)
		}
		if containsInt(reason, until) {
			Pass(t, "C-4.6-6", "reason %q carries the expiry", reason)
		} else {
			Advisory(t, "C-4.6-6", "reason %q does not carry the expiry %d", reason, until)
		}
	})
}

func containsInt(s string, n int64) bool {
	d := strconv.FormatInt(n, 10)
	return len(s) >= len(d) && (indexOf(s, d) >= 0 || indexOf(s, time.Unix(n, 0).UTC().Format(time.RFC3339)) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
