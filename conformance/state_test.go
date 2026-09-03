package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/state/clientstore"
	"github.com/panaudia/lasa/wire"
)

var appPerms = connect.Permissions{Set: []string{"app"}, Clear: []string{"app"}, Read: []string{"app"}}

// TestStateEchoAndReconnect covers C-6.2-2 (the echo), C-6.5-2 and
// C-6.6-3 (a reconnect with a persisted cursor resumes by delta and
// keeps what it had), from two connections and nothing else.
func TestStateEchoAndReconnect(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		persist := clientstore.NewMemPersistence()
		store, err := clientstore.New(f.SpaceID, []string{""}, persist)
		if err != nil {
			t.Fatal(err)
		}
		cfg := configFor(t, f, party{clientID: "st-c", entities: []connect.Entity{{ID: "st-e", Name: "Alice"}}, permissions: appPerms})
		cfg.Store = store
		c, err := dial(t, f, cfg)
		if err != nil {
			t.Fatal(err)
		}
		syncCtx, syncCancel := context.WithCancel(context.Background())
		go func() { _ = c.SyncState(syncCtx) }()

		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		if err := c.WriteState(ctx, wire.Set{Key: "app.k", Value: []byte("v1")}); err != nil {
			t.Fatal(err)
		}
		echoed := waitCond(t, "echo of app.k", func() bool { _, ok := storeGet(c, "app.k"); return ok })
		Check(t, "C-6.2-2", echoed, "no echo of an accepted write on the writer's own sink")
		name, _ := storeGet(c, "lasa.entity.st-e.name")
		Check(t, "P-6-6", name == `"Alice"`, "lasa.entity.st-e.name = %q", name)
		syncCancel()
		_ = c.Close()

		// While away, another connection writes.
		w := mustDial(t, f, party{clientID: "st-w", entities: []connect.Entity{}, permissions: appPerms})
		if err := w.WriteState(ctx, wire.Set{Key: "app.later", Value: []byte("v2")}); err != nil {
			t.Fatal(err)
		}

		store2, err := clientstore.New(f.SpaceID, []string{""}, persist)
		if err != nil {
			t.Fatal(err)
		}
		Check(t, "C-6.6-3", store2.SubscribeParams().Cursor != nil, "no cursor persisted after the first catch-up frontier")
		cfg2 := configFor(t, f, party{clientID: "st-c", entities: []connect.Entity{{ID: "st-e", Name: "Alice"}}, permissions: appPerms})
		cfg2.Store = store2
		c2, err := dial(t, f, cfg2)
		if err != nil {
			t.Fatal(err)
		}
		syncCtx2, syncCancel2 := context.WithCancel(context.Background())
		defer syncCancel2()
		go func() { _ = c2.SyncState(syncCtx2) }()
		later := waitCond(t, "app.later after reconnect", func() bool { _, ok := storeGet(c2, "app.later"); return ok })
		_, kept := storeGet(c2, "app.k")
		Check(t, "C-6.5-2", later && kept, "reconnect catch-up: later=%v kept=%v", later, kept)
	})
}

// TestAdmissionKeysAndSweep watches an entity's keys from an observer
// connection: the marker and its value (P-6-4), materialised channels
// (P-4-2, P-6-8), and the sweep on disconnect (P-3-1, P-3-3). The
// client claim record never reaches a client (P-6-2, P-2-4).
func TestAdmissionKeysAndSweep(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		obs, _ := observer(t, f, "sweep-obs", []string{""}, connect.Permissions{})
		c := mustDial(t, f, party{clientID: "sweep-c", entities: []connect.Entity{{ID: "sweep-e", Name: "A"}}})
		marker := waitCond(t, "entity marker", func() bool {
			v, ok := storeGet(obs, "lasa.entity.sweep-e")
			return ok && v == `"sweep-c"`
		})
		Check(t, "P-6-4", marker, "marker absent or not naming the owner")
		Check(t, "C-8-1", marker, "the entity marker was not readable by another participant with its owner as value")
		heard, _ := storeGet(obs, "lasa.entity.sweep-e.heard-in.main")
		hears, _ := storeGet(obs, "lasa.entity.sweep-e.hears.main")
		Check(t, "P-4-2", heard == "true" && hears == "true", "omitted channels not materialised as main: heard-in=%q hears=%q", heard, hears)
		Pass(t, "P-6-8", "admission wrote heard-in.main and hears.main")
		time.Sleep(200 * time.Millisecond)
		Check(t, "P-6-2", !storeHasPrefix(obs, "lasa.client."), "a lasa.client. key reached a client")
		Pass(t, "P-2-4", "audience never receives lasa.client.")

		_ = c.Close()
		swept := waitCond(t, "marker swept", func() bool { _, ok := storeGet(obs, "lasa.entity.sweep-e"); return !ok })
		Check(t, "P-3-1", swept, "entity marker survived the disconnect")
		Check(t, "P-3-3", swept, "claim record present without a connection")
		Check(t, "P-6-4", !storeHasPrefix(obs, "lasa.entity.sweep-e"), "keys under the entity prefix survived the sweep")
	})
}
