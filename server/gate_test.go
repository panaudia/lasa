package server

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// Unit level: expiry semantics and the admin exemptions.
func TestGateSemantics(t *testing.T) {
	if v, ok := parseBlockUntil([]byte("0")); !ok || v != 0 {
		t.Fatal("0 must parse as forever")
	}
	if v, ok := parseBlockUntil([]byte("1800000000")); !ok || v != 1800000000 {
		t.Fatal("timestamp must parse")
	}
	if _, ok := parseBlockUntil([]byte(`"soon"`)); ok {
		t.Fatal("non-number must be dropped")
	}
	if !effective(0, 999) || !effective(1000, 999) || effective(1000, 1000) {
		t.Fatal("expiry semantics wrong")
	}

	g := &moderationGate{clients: map[string]int64{"c1": 0}, roles: map[string]int64{"noisy": 0}}
	if rej := g.check(&connect.Resolved{ClientID: "c1"}); rej == nil || rej.Code != wire.ErrCodeBlocked {
		t.Fatal("blocked client must be rejected")
	}
	if rej := g.check(&connect.Resolved{ClientID: "c1", Roles: []string{"admin"}}); rej != nil {
		t.Fatal("admin must bypass client blocks (§5)")
	}
	if rej := g.check(&connect.Resolved{ClientID: "c2", Roles: []string{"noisy"}}); rej == nil {
		t.Fatal("blocked role must be rejected")
	}
	// Expired block admits.
	g.clients["c3"] = time.Now().Unix() - 10
	if rej := g.check(&connect.Resolved{ClientID: "c3"}); rej != nil {
		t.Fatal("expired block must admit")
	}
	// role.blocked.admin is an invalid write: never enters the set.
	g2 := &moderationGate{clients: map[string]int64{}, roles: map[string]int64{}}
	g2.onOp("lasa.space.role.blocked.admin", []byte("0"), false)
	if len(g2.roles) != 0 {
		t.Fatal("admin role block must be dropped")
	}
}

// dialErr dials expecting the connection to be refused.
func dialErr(t *testing.T, sp *testSpace, clientID string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := client.Dial(ctx, sp.addr, client.Config{
		SpaceID:  "space1",
		ClientID: clientID,
		Entities: []connect.Entity{{ID: "e-" + clientID, Name: "x"}},
		TLS:      &tls.Config{InsecureSkipVerify: true},
	})
	if err == nil {
		_ = c.Close()
	}
	return err
}

func block(t *testing.T, sp *testSpace, key, value string) {
	t.Helper()
	if err := sp.srv.Engine().Write(wire.Set{Key: key, Value: []byte(value)}); err != nil {
		t.Fatal(err)
	}
}

// Blocked at connect: the gate refuses before Admit — the backend never
// even sees the attempt.
func TestE2EBlockedAtConnect(t *testing.T) {
	sp := startSpace(t, nil)
	block(t, sp, "lasa.space.client.blocked.c1", "0")
	if err := dialErr(t, sp, "c1"); err == nil {
		t.Fatal("blocked client connected")
	}
	// The gate refuses BEFORE adjudication: a refused connection never
	// reaches Engine.Admit, so its claim key is never written.
	if _, ok := engineGet(sp.srv, "lasa.client.c1"); ok {
		t.Fatal("gate must refuse BEFORE Admit; claim key was written")
	}
	// Expired blocks admit.
	block(t, sp, "lasa.space.client.blocked.c3", "1") // 1970: long expired
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := client.Dial(ctx, sp.addr, client.Config{
		SpaceID: "space1", ClientID: "c3",
		Entities: []connect.Entity{{ID: "e-c3", Name: "x"}},
		TLS:      &tls.Config{InsecureSkipVerify: true},
	})
	if err != nil {
		t.Fatalf("expired block must admit: %v", err)
	}
	_ = c.Close()
}

// Blocked while live: the block severs the session through the normal
// funnel — the embedder sees EntityLeft.
func TestE2EBlockedWhileLive(t *testing.T) {
	sp := startSpace(t, nil)
	_ = dial(t, sp, nil) // ClientID c1, entity e1
	waitCond(t, "admission", func() bool {
		_, ok := engineGet(sp.srv, "lasa.client.c1")
		return ok
	})
	block(t, sp, "lasa.space.client.blocked.c1", "0")
	waitCond(t, "EntityLeft after live block", func() bool {
		sp.backend.mu.Lock()
		defer sp.backend.mu.Unlock()
		for _, id := range sp.backend.left {
			if id == "e1" {
				return true
			}
		}
		return false
	})
}

// The KickGate lesson: a blocked id's reconnect attempts never reach
// Admit, so they can never displace anything.
func TestE2EBlockedReconnectCannotEvict(t *testing.T) {
	sp := startSpace(t, nil)
	_ = dial(t, sp, nil) // c1 live
	waitCond(t, "admission", func() bool {
		_, ok := engineGet(sp.srv, "lasa.client.c1")
		return ok
	})
	block(t, sp, "lasa.space.client.blocked.c1", "0")
	// The live session is severed (claim swept); the still-blocked id
	// retries and must never reach adjudication again.
	waitCond(t, "sever sweep", func() bool {
		_, ok := engineGet(sp.srv, "lasa.client.c1")
		return !ok
	})
	for i := 0; i < 3; i++ {
		if err := dialErr(t, sp, "c1"); err == nil {
			t.Fatal("blocked reconnect admitted")
		}
	}
	if _, ok := engineGet(sp.srv, "lasa.client.c1"); ok {
		t.Fatal("blocked retries reached Admit: claim key rewritten")
	}
}
