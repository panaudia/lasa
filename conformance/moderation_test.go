package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestBlocks covers the base profile's moderation rows from an admin
// connection: a block severs a live session with 0x1A5A06 (P-5-3,
// C-4.6-7), gates admission (P-5-4, C-4.4-9), an expired value admits
// (P-5-2), and the role block hits the audience default (P-2-1).
// Ticketed fixtures only, since writing lasa.space.* needs a role.
func TestBlocks(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		rows := []string{"P-5-3", "P-5-4", "P-5-2", "C-4.4-9", "P-2-1"}
		if !f.Ticketed {
			for _, id := range rows {
				NotExercised(t, id, "unticketed fixture: no role may write blocks")
			}
			return
		}
		admin := mustDial(t, f, party{clientID: "mod-admin", entities: []connect.Entity{}, roles: []string{"admin"}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		write := func(key, value string) {
			t.Helper()
			if err := admin.WriteState(ctx, wire.Set{Key: key, Value: []byte(value)}); err != nil {
				t.Fatal(err)
			}
		}
		clear := func(key string) { _ = admin.WriteState(context.Background(), wire.Clear{Key: key}) }
		t.Cleanup(func() {
			clear("lasa.space.client.blocked.mod-victim")
			clear("lasa.space.role.blocked.audience")
			time.Sleep(100 * time.Millisecond)
		})

		victim := mustDial(t, f, party{clientID: "mod-victim", entities: []connect.Entity{{ID: "mod-victim-e", Name: "V"}}})
		write("lasa.space.client.blocked.mod-victim", "0")
		expectClosed(t, "P-5-3", victim, wire.ErrCodeBlocked)
		Pass(t, "C-4.6-7", "live block closed the session with 0x1A5A06")
		for i := 0; i < 3; i++ {
			expectRejection(t, "P-5-4", f, configFor(t, f, party{clientID: "mod-victim", entities: []connect.Entity{{ID: "mod-victim-e", Name: "V"}}}), wire.ErrCodeBlocked)
		}
		Pass(t, "C-4.4-9", "a blocked id's reconnects are refused with the block code, never adjudicated")

		// An expired block admits.
		write("lasa.space.client.blocked.mod-victim", "1")
		time.Sleep(100 * time.Millisecond)
		back, err := dial(t, f, configFor(t, f, party{clientID: "mod-victim", entities: []connect.Entity{{ID: "mod-victim-e", Name: "V"}}}))
		Check(t, "P-5-2", err == nil, "an expired block still refused: %v", err)
		if back != nil {
			_ = back.Close()
		}

		// A roleless ticket is exactly audience: the audience role block
		// severs it and refuses its return.
		aud := mustDial(t, f, party{clientID: "mod-aud", entities: []connect.Entity{{ID: "mod-aud-e", Name: "A"}}})
		write("lasa.space.role.blocked.audience", "0")
		expectClosed(t, "P-2-1", aud, wire.ErrCodeBlocked)
		expectRejection(t, "P-2-1", f, configFor(t, f, party{clientID: "mod-aud2", entities: []connect.Entity{{ID: "mod-aud2-e", Name: "A"}}}), wire.ErrCodeBlocked)
		select {
		case <-admin.Done():
			Fail(t, "P-5-5", "admin was severed by a role block: %v", admin.Err())
		case <-time.After(200 * time.Millisecond):
			Pass(t, "P-5-5", "admin survived the audience role block")
		}
	})
}
