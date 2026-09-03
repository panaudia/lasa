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

// TestPresenceGeneration covers C-7-7 (the generation moves only on
// membership change) and C-7-9 (a keyframe follows a membership change
// promptly, measured).
func TestPresenceGeneration(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		_ = mustDial(t, f, party{clientID: "gen-a", entities: []connect.Entity{{ID: "gen-ea", Name: "A"}}})
		w := mustDial(t, f, party{clientID: "gen-w", entities: []connect.Entity{}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		var mu sync.Mutex
		type kf struct {
			gen uint8
			at  time.Time
			ids map[string]bool
		}
		var kfs []kf
		rt, err := w.SubscribePresence(ctx, func(_ uint64, msg any) {
			k, ok := msg.(*wire.PresenceKeyframe)
			if !ok {
				return
			}
			ids := map[string]bool{}
			for _, r := range k.Records {
				ids[r.ID] = true
			}
			mu.Lock()
			kfs = append(kfs, kf{gen: k.Gen, at: time.Now(), ids: ids})
			mu.Unlock()
		})
		if err != nil {
			t.Fatal(err)
		}
		defer rt.Close()
		waitCond(t, "a keyframe", func() bool { mu.Lock(); defer mu.Unlock(); return len(kfs) > 0 })
		time.Sleep(1 * time.Second)
		mu.Lock()
		first := kfs[0].gen
		steady := true
		for _, k := range kfs {
			if k.gen != first {
				steady = false
			}
		}
		n0 := len(kfs)
		mu.Unlock()
		Check(t, "C-7-7", steady, "generation moved without a membership change")

		t0 := time.Now()
		_ = mustDial(t, f, party{clientID: "gen-b", entities: []connect.Entity{{ID: "gen-eb", Name: "B"}}})
		var joined kf
		found := waitCond(t, "keyframe listing gen-eb", func() bool {
			mu.Lock()
			defer mu.Unlock()
			for _, k := range kfs[n0:] {
				if k.ids["gen-eb"] {
					joined = k
					return true
				}
			}
			return false
		})
		if !found {
			return
		}
		Check(t, "C-7-7", joined.gen == first+1, "generation %d after a join, want %d", joined.gen, first+1)
		lag := joined.at.Sub(t0)
		Check(t, "C-7-9", lag < 500*time.Millisecond, "keyframe listing the newcomer arrived %v after admission", lag)
		report.record("C-7-9", OutcomePass, "keyframe "+lag.Round(time.Millisecond).String()+" after admission on "+f.Name)
	})
}

// TestPoseRules covers P-6-13 (initial-pose is the pose until the first
// accepted packet pose, and is never state) and P-6-12 (dof is enforced
// at ingest and presence shows the enforced pose).
func TestPoseRules(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		zero, three := 0, 3
		at := func(x float64) *connect.PoseSpec { return &connect.PoseSpec{Position: &connect.Position{X: x}} }
		ents := []connect.Entity{
			{ID: "pose-free", Name: "F", InitialPose: at(3)},
			{ID: "pose-fixed", Name: "Z", InitialPose: at(4), Dof: &zero},
			{ID: "pose-turn", Name: "T", InitialPose: at(5), Dof: &three},
		}
		obs, _ := observer(t, f, "pose-obs", []string{"lasa.entity.pose-"}, connect.Permissions{})
		c := mustDial(t, f, party{clientID: "pose-c", entities: ents})
		w := mustDial(t, f, party{clientID: "pose-w", entities: []connect.Entity{}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		roster, rt, err := w.SubscribeRoster(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rt.Close()
		pose := func(id string) (wire.Pose, bool) {
			for _, e := range roster.Snapshot() {
				if e.ID == id {
					return e.Pose, true
				}
			}
			return wire.Pose{}, false
		}
		near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

		// Before any packet: the initial poses.
		initial := waitCond(t, "initial poses on presence", func() bool {
			a, ok1 := pose("pose-free")
			b, ok2 := pose("pose-fixed")
			cc, ok3 := pose("pose-turn")
			return ok1 && ok2 && ok3 && near(a.X, 3) && near(b.X, 4) && near(cc.X, 5)
		})
		waitCond(t, "entity markers", func() bool { _, ok := storeGet(obs, "lasa.entity.pose-turn"); return ok })
		inState := false
		obs.View(func(s *clientstoreStore) {
			s.Range(func(k string, _ []byte, _ uint64) bool {
				if indexOf(k, "initial") >= 0 {
					inState = true
				}
				return true
			})
		})
		Check(t, "P-6-13", initial && !inState, "initial pose shown=%v, materialised in state=%v", initial, inState)

		// Publish X=7, yaw=0.5 on all three for a while.
		var pubs []*client.EntityPublisher
		for _, e := range ents {
			p, err := c.Entity(ctx, e.ID)
			if err != nil {
				t.Fatal(err)
			}
			pubs = append(pubs, p)
		}
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			tk := time.NewTicker(10 * time.Millisecond)
			defer tk.Stop()
			for seq := uint64(0); ; seq++ {
				select {
				case <-stop:
					return
				case <-tk.C:
					for _, p := range pubs {
						_ = p.WriteMonoObject(seq, &wire.MonoObjectPacket{Pose: &wire.Pose{X: 7, Yaw: 0.5}})
					}
				}
			}
		}()
		moved := waitCond(t, "the free entity at X=7", func() bool { p, ok := pose("pose-free"); return ok && near(p.X, 7) })
		time.Sleep(300 * time.Millisecond)
		fixed, _ := pose("pose-fixed")
		turn, _ := pose("pose-turn")
		Check(t, "P-6-13", moved, "the first accepted packet pose did not supersede the initial pose")
		Check(t, "P-6-12", near(fixed.X, 4) && near(turn.X, 5) && near(turn.Yaw, 0.5),
			"dof 0 pose X=%.2f (want 4), dof 3 pose X=%.2f yaw=%.2f (want 5, 0.5)", fixed.X, turn.X, turn.Yaw)
	})
}
