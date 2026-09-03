package conformance

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestStateKeyRules covers C-2-4: the key grammar and the 512-byte cap
// are enforced on writes, observed as echo or silence.
func TestStateKeyRules(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := syncingClient(t, f, party{clientID: "key-c", entities: []connect.Entity{}, permissions: appPerms}, []string{""})
		ok := "app." + strings.Repeat("k", 512-4)
		long := ok + "k"
		write(t, c, wire.Set{Key: ok, Value: []byte("1")})
		echoed := waitCond(t, "echo of the 512-byte key", func() bool { _, got := storeGet(c, ok); return got })
		Check(t, "C-2-4", echoed, "a 512-byte key within the grammar was not echoed")
		// The polite client refuses to encode a key outside the grammar
		// or over the cap, so the refusal half waits for the raw layer.
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		refused := 0
		for _, k := range []string{"app.Bad", long} {
			if err := c.WriteState(ctx, wire.Set{Key: k, Value: []byte("1")}); err != nil {
				refused++
			}
		}
		if refused == 2 {
			NotExercised(t, "C-2-4", "server-side refusal of a bad key: the polite client refuses to encode one, hostile client later")
		} else {
			time.Sleep(300 * time.Millisecond)
			_, bad := storeGet(c, "app.Bad")
			_, tooLong := storeGet(c, long)
			Check(t, "C-2-4", !bad && !tooLong, "uppercase key echoed=%v, 513-byte key echoed=%v", bad, tooLong)
		}
	})
}

// TestSeqAndFlatStore covers C-6.1-2 (one monotonic seq run), C-6.2-1
// (the server assigns seqs) and C-6.1-4 (the store is flat).
func TestSeqAndFlatStore(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := syncingClient(t, f, party{clientID: "seq-c", entities: []connect.Entity{}, permissions: appPerms}, []string{""})
		for _, k := range []string{"app.a", "app.b", "app.c"} {
			write(t, c, wire.Set{Key: k, Value: []byte("1")})
		}
		waitCond(t, "three echoes", func() bool { _, got := storeGet(c, "app.c"); return got })
		seqs := map[string]uint64{}
		c.View(func(s *clientstoreStore) {
			s.Range(func(k string, _ []byte, seq uint64) bool { seqs[k] = seq; return true })
		})
		Check(t, "C-6.1-2", seqs["app.a"] < seqs["app.b"] && seqs["app.b"] < seqs["app.c"], "seqs a=%d b=%d c=%d", seqs["app.a"], seqs["app.b"], seqs["app.c"])
		Check(t, "C-6.2-1", seqs["app.a"] > 0, "echoed op carries no seq")

		write(t, c, wire.Set{Key: "app.p", Value: []byte("1")}, wire.Set{Key: "app.p.child", Value: []byte("2")})
		waitCond(t, "parent and child", func() bool { _, got := storeGet(c, "app.p.child"); return got })
		write(t, c, wire.Clear{Key: "app.p"})
		waitCond(t, "parent cleared", func() bool { _, got := storeGet(c, "app.p"); return !got })
		_, child := storeGet(c, "app.p.child")
		Check(t, "C-6.1-4", child, "clearing app.p removed app.p.child")
	})
}

// TestRulesUnion covers C-4.3-8 (role rules and ticket permissions
// compose by union) and C-6.4-1 (the same merged rules filter reads),
// on ticketed fixtures.
func TestRulesUnion(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		if !f.Ticketed {
			NotExercised(t, "C-4.3-8", "unticketed fixture")
			NotExercised(t, "C-6.4-1", "unticketed fixture")
			return
		}
		own := []connect.Entity{{ID: "union-e", Name: "U"}}
		c := syncingClient(t, f, party{clientID: "union-c", entities: own, permissions: appPerms}, []string{""})
		write(t, c, wire.Set{Key: "app.k", Value: []byte("1")})                        // ticket permission
		write(t, c, wire.Set{Key: "lasa.entity.union-e.attrs.x", Value: []byte("1")})  // audience role rule
		write(t, c, wire.Set{Key: "lasa.space.channel.gain.main", Value: []byte("1")}) // neither
		waitCond(t, "both grants echoed", func() bool {
			_, a := storeGet(c, "app.k")
			_, b := storeGet(c, "lasa.entity.union-e.attrs.x")
			return a && b
		})
		_, denied := storeGet(c, "lasa.space.channel.gain.main")
		Check(t, "C-4.3-8", !denied, "a write outside both the role rules and the permissions was applied")

		// Read direction: an observer without the app permission never
		// sees app.k, one with it does.
		noPerm, _ := observer(t, f, "union-obs-none", []string{""}, connect.Permissions{})
		withPerm, _ := observer(t, f, "union-obs-app", []string{""}, connect.Permissions{Read: []string{"app"}})
		sees := waitCond(t, "permitted observer sees app.k", func() bool { _, got := storeGet(withPerm, "app.k"); return got })
		hidden := absentFor(300*time.Millisecond, func() bool { _, got := storeGet(noPerm, "app.k"); return got })
		Check(t, "C-6.4-1", sees && hidden, "read filter: permitted=%v hidden=%v", sees, hidden)
	})
}

// TestSupersessionSweepOrder covers C-4.4-7: the superseded connection's
// sweep completes before the replacement's admission writes, so the
// replacement's keys are what survives.
func TestSupersessionSweepOrder(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		obs, _ := observer(t, f, "order-obs", []string{"lasa.entity.order-e"}, connect.Permissions{})
		_ = mustDial(t, f, party{clientID: "order-c", entities: []connect.Entity{{ID: "order-e", Name: "A"}}})
		waitCond(t, "name A", func() bool { v, _ := storeGet(obs, "lasa.entity.order-e.name"); return v == `"A"` })
		_ = mustDial(t, f, party{clientID: "order-c", entities: []connect.Entity{{ID: "order-e", Name: "B"}}})
		waitCond(t, "name B", func() bool { v, _ := storeGet(obs, "lasa.entity.order-e.name"); return v == `"B"` })
		stable := absentFor(300*time.Millisecond, func() bool {
			v, ok := storeGet(obs, "lasa.entity.order-e.name")
			_, marker := storeGet(obs, "lasa.entity.order-e")
			return !ok || v != `"B"` || !marker
		})
		Check(t, "C-4.4-7", stable, "the replacement's admission writes were disturbed by the predecessor's sweep")
	})
}

// TestAttrs covers P-6-7: attrs are owner-writable at runtime, and in a
// ticketed space signed-attrs come from the ticket and are immutable.
func TestAttrs(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		ent := []connect.Entity{{ID: "attrs-e", Name: "A", Attrs: map[string]any{"colour": "red"}}}
		obs, _ := observer(t, f, "attrs-obs", []string{"lasa.entity.attrs-e"}, connect.Permissions{})
		c := syncingClient(t, f, party{clientID: "attrs-c", entities: ent}, []string{"lasa.entity.attrs-e"})
		key := "lasa.entity.attrs-e.attrs.colour"
		if f.Ticketed {
			key = "lasa.entity.attrs-e.signed-attrs.colour"
		}
		seeded := waitCond(t, "the definition's attr", func() bool { v, _ := storeGet(obs, key); return v == `"red"` })
		write(t, c, wire.Set{Key: "lasa.entity.attrs-e.attrs.mood", Value: []byte(`"calm"`)})
		runtime := waitCond(t, "runtime attr echoed", func() bool { v, _ := storeGet(obs, "lasa.entity.attrs-e.attrs.mood"); return v == `"calm"` })
		if !f.Ticketed {
			Check(t, "P-6-7", seeded && runtime, "attrs: seeded=%v runtime=%v", seeded, runtime)
			return
		}
		write(t, c, wire.Set{Key: key, Value: []byte(`"blue"`)})
		immutable := absentFor(300*time.Millisecond, func() bool { v, _ := storeGet(obs, key); return v != `"red"` })
		Check(t, "P-6-7", seeded && runtime && immutable, "attrs: seeded=%v runtime=%v signed immutable=%v", seeded, runtime, immutable)
	})
}

// TestMuteResetOnReconnect covers P-6-15: personal mute and solo are
// connection-scoped and vanish when the owner reconnects.
func TestMuteResetOnReconnect(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		ent := []connect.Entity{{ID: "mute-e", Name: "M"}}
		obs, _ := observer(t, f, "mute-obs", []string{"lasa.entity.mute-e"}, connect.Permissions{})
		c := syncingClient(t, f, party{clientID: "mute-c", entities: ent}, []string{"lasa.entity.mute-e"})
		write(t, c, wire.Set{Key: "lasa.entity.mute-e.mute.other", Value: []byte("true")}, wire.Set{Key: "lasa.entity.mute-e.solo.other", Value: []byte("true")})
		waitCond(t, "mute and solo visible", func() bool {
			_, a := storeGet(obs, "lasa.entity.mute-e.mute.other")
			_, b := storeGet(obs, "lasa.entity.mute-e.solo.other")
			return a && b
		})
		_ = c.Close()
		_ = mustDial(t, f, party{clientID: "mute-c", entities: ent})
		waitCond(t, "marker back after reconnect", func() bool { _, ok := storeGet(obs, "lasa.entity.mute-e"); return ok })
		time.Sleep(200 * time.Millisecond)
		_, mute := storeGet(obs, "lasa.entity.mute-e.mute.other")
		_, solo := storeGet(obs, "lasa.entity.mute-e.solo.other")
		Check(t, "P-6-15", !mute && !solo, "mute=%v solo=%v survived the reconnect", mute, solo)
	})
}

// TestSpaceScopedSurvives covers P-3-4: a space-scoped key outlives the
// connection that wrote it. Ticketed only, since writing lasa.space.*
// needs a role.
func TestSpaceScopedSurvives(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		if !f.Ticketed {
			NotExercised(t, "P-3-4", "unticketed fixture")
			return
		}
		obs, _ := observer(t, f, "space-obs", []string{"lasa.space"}, connect.Permissions{})
		admin := mustDial(t, f, party{clientID: "space-admin", entities: []connect.Entity{}, roles: []string{"admin"}})
		key := "lasa.space.channel.gain.conform-scope"
		write(t, admin, wire.Set{Key: key, Value: []byte("1.5")})
		t.Cleanup(func() {
			a := mustDial(t, f, party{clientID: "space-admin-clean", entities: []connect.Entity{}, roles: []string{"admin"}})
			write(t, a, wire.Clear{Key: key})
		})
		waitCond(t, "space key visible", func() bool { _, ok := storeGet(obs, key); return ok })
		_ = admin.Close()
		time.Sleep(300 * time.Millisecond)
		_, ok := storeGet(obs, key)
		Check(t, "P-3-4", ok, "space-scoped key vanished with its writer's connection")
	})
}
