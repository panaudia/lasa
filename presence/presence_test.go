package presence

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/panaudia/lasa/wire"
)

type capture struct {
	groups  []uint64
	packets []any
}

func (c *capture) sender() Sender {
	return func(group uint64, payload []byte) {
		msg, err := wire.ParsePresence(payload)
		if err != nil {
			panic(err)
		}
		c.groups = append(c.groups, group)
		c.packets = append(c.packets, msg)
	}
}

func (c *capture) keyframes() []*wire.PresenceKeyframe {
	var out []*wire.PresenceKeyframe
	for _, p := range c.packets {
		if kf, ok := p.(*wire.PresenceKeyframe); ok {
			out = append(out, kf)
		}
	}
	return out
}

func (c *capture) deltas() []*wire.PresenceDelta {
	var out []*wire.PresenceDelta
	for _, p := range c.packets {
		if d, ok := p.(*wire.PresenceDelta); ok {
			out = append(out, d)
		}
	}
	return out
}

func TestAuthorLifecycle(t *testing.T) {
	entries := []Entry{{EntityID: "e1", Pose: wire.Pose{X: 1}, Loudness: 5}}
	a := NewAuthor(func() []Entry { return entries }, time.Minute)
	cap := &capture{}
	a.AddSender("sub", cap.sender())
	now := time.Unix(1_800_000_000, 0)

	// First tick: membership change (empty → e1) → keyframe, gen 0.
	a.Tick(now)
	kfs := cap.keyframes()
	if len(kfs) != 1 || kfs[0].Gen != 0 || len(kfs[0].Records) != 1 || kfs[0].Records[0].ID != "e1" {
		t.Fatalf("first tick: %+v", cap.packets)
	}

	// Unchanged entries: nothing at all.
	n := len(cap.packets)
	a.Tick(now.Add(20 * time.Millisecond))
	if len(cap.packets) != n {
		t.Fatalf("unchanged tick emitted %d packets", len(cap.packets)-n)
	}

	// Pose change: a delta addressing index 0, same generation.
	entries[0].Pose.X = 2
	a.Tick(now.Add(40 * time.Millisecond))
	ds := cap.deltas()
	if len(ds) != 1 || ds[0].Gen != 0 || ds[0].Records[0].Index != 0 {
		t.Fatalf("delta: %+v", cap.packets)
	}

	// Membership change: gen bumps, keyframe re-lists.
	entries = append(entries, Entry{EntityID: "e2", HeadFrame: true, Loudness: 9})
	a.Tick(now.Add(60 * time.Millisecond))
	kfs = cap.keyframes()
	last := kfs[len(kfs)-1]
	if last.Gen != 1 || len(last.Records) != 2 {
		t.Fatalf("post-join keyframe: %+v", last)
	}
	if !last.Records[1].HeadFrame {
		t.Error("head-frame flag must ride the krecord")
	}

	// Keyframe cadence: past kfInterval a keyframe repeats, same gen.
	nKF := len(cap.keyframes())
	a.Tick(now.Add(2 * time.Minute))
	if len(cap.keyframes()) != nKF+1 || cap.keyframes()[nKF].Gen != 1 {
		t.Fatal("cadence keyframe missing or wrong gen")
	}

	// Departure: gen bumps, roster shrinks (departure is keyframe absence).
	entries = entries[1:]
	a.Tick(now.Add(3 * time.Minute))
	last = cap.keyframes()[len(cap.keyframes())-1]
	if last.Gen != 2 || len(last.Records) != 1 || last.Records[0].ID != "e2" {
		t.Fatalf("post-departure keyframe: %+v", last)
	}

	// Group ids are strictly monotonic across all datagrams.
	for i := 1; i < len(cap.groups); i++ {
		if cap.groups[i] != cap.groups[i-1]+1 {
			t.Fatalf("group ids not contiguous: %v", cap.groups)
		}
	}
}

func TestAuthorKeyframeChunking(t *testing.T) {
	var entries []Entry
	for i := 0; i < 120; i++ {
		entries = append(entries, Entry{EntityID: fmt.Sprintf("entity-%03d", i)})
	}
	a := NewAuthor(func() []Entry { return entries }, time.Minute)
	cap := &capture{}
	a.AddSender("sub", cap.sender())
	a.Tick(time.Unix(1_800_000_000, 0))

	kfs := cap.keyframes()
	if len(kfs) < 2 {
		t.Fatalf("120 records must chunk into several keyframes, got %d", len(kfs))
	}
	// Chunks are self-contained: First + position reconstructs every index.
	seen := map[int]string{}
	for _, kf := range kfs {
		for i, r := range kf.Records {
			seen[int(kf.First)+i] = r.ID
		}
	}
	if len(seen) != 120 {
		t.Fatalf("reconstructed %d indexes, want 120", len(seen))
	}
	for idx, id := range seen {
		if !strings.HasSuffix(id, fmt.Sprintf("%03d", idx)) {
			t.Fatalf("index %d maps to %s", idx, id)
		}
	}
}

func TestAuthorEmptyRosterKeyframe(t *testing.T) {
	a := NewAuthor(func() []Entry { return nil }, time.Minute)
	cap := &capture{}
	a.AddSender("sub", cap.sender())
	a.Tick(time.Unix(1_800_000_000, 0))
	// forced flag: an empty space still answers a new subscriber with an
	// (empty) keyframe on the cadence path.
	kfs := cap.keyframes()
	if len(kfs) != 1 || len(kfs[0].Records) != 0 {
		t.Fatalf("empty-space keyframe: %+v", cap.packets)
	}
}

func TestAuthorNoSendersNoWork(t *testing.T) {
	polled := 0
	a := NewAuthor(func() []Entry { polled++; return nil }, time.Minute)
	a.Tick(time.Unix(1_800_000_000, 0))
	if polled != 0 {
		t.Fatal("must not poll with no subscribers")
	}
}
