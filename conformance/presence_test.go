package conformance

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestPresence covers the presence rows a polite client can see:
// group ids per datagram (C-7-1), keyframe and delta layouts parse
// (C-7-3), the roster carries every entity with its pose and the
// head-frame flag (C-7-6, C-7-15, P-7-1), and departure is keyframe
// absence (C-7-12).
func TestPresence(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "pres-c", entities: []connect.Entity{
			{ID: "pres-e", Name: "A"},
			{ID: "pres-hud", Name: "HUD", Frame: "head"},
		}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		pub, err := c.Entity(ctx, "pres-e")
		if err != nil {
			t.Fatal(err)
		}
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			// The pose wobbles so deltas flow (changed entities only).
			tk := time.NewTicker(10 * time.Millisecond)
			defer tk.Stop()
			for seq := uint64(0); ; seq++ {
				select {
				case <-stop:
					return
				case <-tk.C:
					pose := wire.Pose{X: 7, Y: float32(seq%100) * 0.001}
					_ = pub.WriteMonoObject(seq, &wire.MonoObjectPacket{Pose: &pose})
				}
			}
		}()

		// A second connection watches presence.
		w := mustDial(t, f, party{clientID: "pres-w", entities: []connect.Entity{}})
		var mu sync.Mutex
		var last uint64
		var n, gaps, keyframes, deltas int
		roster := client.NewRoster()
		rt, err := w.SubscribePresence(ctx, func(groupID uint64, msg any) {
			mu.Lock()
			defer mu.Unlock()
			if n > 0 {
				if groupID <= last {
					gaps = -1_000_000 // backwards or duplicate, counted as failure below
				} else {
					gaps += int(groupID - last - 1)
				}
			}
			last = groupID
			n++
			switch msg.(type) {
			case *wire.PresenceKeyframe:
				keyframes++
			case *wire.PresenceDelta:
				deltas++
			}
			roster.Apply(groupID, msg)
		})
		if err != nil {
			t.Fatal(err)
		}
		defer rt.Close()

		seen := waitCond(t, "roster shows pres-e at X=7 and the HUD flagged", func() bool {
			var e, hud bool
			for _, r := range roster.Snapshot() {
				if r.ID == "pres-e" && math.Abs(float64(r.Pose.X-7)) < 0.01 {
					e = true
				}
				if r.ID == "pres-hud" && r.HeadFrame {
					hud = true
				}
			}
			return e && hud
		})
		Check(t, "C-7-6", seen, "keyframes did not define the roster")
		Check(t, "P-7-1", seen, "not every entity appeared, or the head-frame flag was missing")
		Check(t, "C-7-15", seen, "head-frame entity not flagged on presence")
		waitCond(t, "40 presence packets", func() bool { mu.Lock(); defer mu.Unlock(); return n >= 40 })
		mu.Lock()
		Check(t, "C-7-1", n > 5 && gaps >= 0 && gaps*20 <= n, "presence ids: %d packets, gap count %d", n, gaps)
		Check(t, "C-7-3", keyframes > 0 && deltas > 0, "layouts: %d keyframes, %d deltas parsed", keyframes, deltas)
		mu.Unlock()

		_ = c.Close()
		gone := waitCond(t, "pres-e absent from the roster", func() bool {
			for _, r := range roster.Snapshot() {
				if r.ID == "pres-e" {
					return false
				}
			}
			return true
		})
		Check(t, "C-7-12", gone, "departed entity still on the roster")
	})
}
