package presence

import (
	"sync"
	"testing"
	"time"

	"github.com/panaudia/lasa/wire"
)

func TestCollectorSampleAndFan(t *testing.T) {
	c := NewCollector()
	s1 := c.Register("e1", false, func() wire.Loudness { return 7 })
	c.Register("e2", true, nil)

	s1.UpdatePose(wire.Pose{X: 1})
	s1.UpdatePose(wire.Pose{X: 2}) // latest wins

	var mu sync.Mutex
	var rounds [][]Entry
	c.Subscribe("t", func(_ time.Time, entries []Entry) {
		mu.Lock()
		rounds = append(rounds, append([]Entry(nil), entries...))
		mu.Unlock()
	})
	snapshot := func() [][]Entry {
		mu.Lock()
		defer mu.Unlock()
		return append([][]Entry(nil), rounds...)
	}

	c.Tick(time.Unix(1_800_000_000, 0))
	r := snapshot()
	if len(r) != 1 || len(r[0]) != 2 {
		t.Fatalf("first round: %+v", r)
	}
	byID := map[string]Entry{}
	for _, e := range r[0] {
		byID[e.EntityID] = e
	}
	if e := byID["e1"]; e.Pose.X != 2 || e.Loudness != 7 || e.HeadFrame {
		t.Fatalf("e1: %+v", e)
	}
	if e := byID["e2"]; !e.HeadFrame || e.Loudness != 0 {
		t.Fatalf("e2: %+v", e)
	}

	// Remove shrinks the roster; unsubscribe stops the fan.
	c.Remove("e2")
	c.Tick(time.Unix(1_800_000_001, 0))
	r = snapshot()
	if len(r) != 2 || len(r[1]) != 1 || r[1][0].EntityID != "e1" {
		t.Fatalf("after remove: %+v", r)
	}
	c.Unsubscribe("t")
	c.Tick(time.Unix(1_800_000_002, 0))
	if len(snapshot()) != 2 {
		t.Fatal("unsubscribed subscriber still fed")
	}
}

// The composed pipeline: slots → sampler → Author → wire datagrams.
func TestCollectorFeedsAuthor(t *testing.T) {
	c := NewCollector()
	a := NewAuthor(time.Minute)
	c.Subscribe(a, a.Tick)
	cap := &capture{}
	a.AddSender("sub", cap.sender())

	s := c.Register("e1", false, func() wire.Loudness { return 3 })
	s.UpdatePose(wire.Pose{X: 5})
	c.Tick(time.Unix(1_800_000_000, 0))

	kfs := cap.keyframes()
	if len(kfs) != 1 || kfs[0].Records[0].ID != "e1" ||
		kfs[0].Records[0].Pose.X != 5 || kfs[0].Records[0].Loudness != 3 {
		t.Fatalf("keyframe: %+v", cap.packets)
	}

	// Movement at packet rate, sampled at cadence: one delta.
	for i := 0; i < 40; i++ {
		s.UpdatePose(wire.Pose{X: float32(10 + i)})
	}
	c.Tick(time.Unix(1_800_000_000, int64(50*time.Millisecond)))
	ds := cap.deltas()
	if len(ds) != 1 || ds[0].Records[0].Pose.X != 49 {
		t.Fatalf("delta after burst: %+v", cap.packets)
	}
}
