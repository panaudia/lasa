package client

// The §7 receive rules, pinned against the Roster state machine:
// generation discard, per-index latest-wins by group-id, tracking
// reset on generation change, chunked-keyframe assembly, and the
// no-rewind property for late keyframe chunks.

import (
	"testing"

	"github.com/panaudia/lasa/wire"
)

func kf(gen uint8, first uint16, groupID uint64, recs ...wire.KeyframeRecord) (uint64, *wire.PresenceKeyframe) {
	return groupID, &wire.PresenceKeyframe{Gen: gen, First: first, Records: recs}
}

func krec(id string, x float32) wire.KeyframeRecord {
	return wire.KeyframeRecord{ID: id, Pose: wire.Pose{X: x}}
}

func delta(gen uint8, groupID uint64, recs ...wire.DeltaRecord) (uint64, *wire.PresenceDelta) {
	return groupID, &wire.PresenceDelta{Gen: gen, Records: recs}
}

func drec(index uint16, x float32) wire.DeltaRecord {
	return wire.DeltaRecord{Index: index, Pose: wire.Pose{X: x}}
}

func find(t *testing.T, r *Roster, id string) RosterEntry {
	t.Helper()
	for _, e := range r.Snapshot() {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("entity %q not in roster", id)
	return RosterEntry{}
}

func TestRosterKeyframeAndDelta(t *testing.T) {
	r := NewRoster()
	r.Apply(kf(1, 0, 10, krec("a", 1), krec("b", 2)))
	if len(r.Snapshot()) != 2 {
		t.Fatalf("roster size = %d, want 2", len(r.Snapshot()))
	}
	r.Apply(delta(1, 11, drec(0, 5)))
	if got := find(t, r, "a").Pose.X; got != 5 {
		t.Errorf("a.X = %v, want 5 (delta applied)", got)
	}
	if got := find(t, r, "b").Pose.X; got != 2 {
		t.Errorf("b.X = %v, want 2 (untouched)", got)
	}
}

func TestRosterStaleGenerationDiscarded(t *testing.T) {
	r := NewRoster()
	// Delta before any keyframe: no established generation — discard.
	r.Apply(delta(1, 5, drec(0, 9)))
	if len(r.Snapshot()) != 0 {
		t.Fatal("pre-keyframe delta must be discarded")
	}
	r.Apply(kf(2, 0, 10, krec("a", 1)))
	// Old-generation delta after the keyframe: discard whole packet.
	r.Apply(delta(1, 11, drec(0, 9)))
	if got := find(t, r, "a").Pose.X; got != 1 {
		t.Errorf("a.X = %v, want 1 (stale-gen delta discarded)", got)
	}
}

func TestRosterLatestWinsByGroupID(t *testing.T) {
	r := NewRoster()
	r.Apply(kf(1, 0, 10, krec("a", 1)))
	r.Apply(delta(1, 12, drec(0, 12)))
	// A reordered older delta must not rewind the pose.
	r.Apply(delta(1, 11, drec(0, 11)))
	if got := find(t, r, "a").Pose.X; got != 12 {
		t.Errorf("a.X = %v, want 12 (older group-id ignored)", got)
	}
	// Equal group-id does not re-apply either (strictly-exceeds rule).
	r.Apply(delta(1, 12, drec(0, 99)))
	if got := find(t, r, "a").Pose.X; got != 12 {
		t.Errorf("a.X = %v, want 12 (equal group-id ignored)", got)
	}
}

func TestRosterGenerationChangeResets(t *testing.T) {
	r := NewRoster()
	r.Apply(kf(1, 0, 10, krec("a", 1), krec("b", 2)))
	// Membership change: b departs, c arrives; indexes reassign.
	r.Apply(kf(2, 0, 20, krec("a", 3), krec("c", 4)))
	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("roster size = %d, want 2", len(snap))
	}
	for _, e := range snap {
		if e.ID == "b" {
			t.Error("departed entity b still in roster")
		}
	}
	// Per-index tracking reset: group-ids from the old generation do
	// not gate the new one (20 < old ids would, if state leaked).
	r.Apply(delta(2, 21, drec(1, 7)))
	if got := find(t, r, "c").Pose.X; got != 7 {
		t.Errorf("c.X = %v, want 7 (fresh tracking after gen change)", got)
	}
}

func TestRosterChunkedKeyframe(t *testing.T) {
	r := NewRoster()
	r.Apply(kf(1, 0, 10, krec("a", 1), krec("b", 2)))
	r.Apply(kf(1, 2, 11, krec("c", 3))) // second chunk, same gen: no reset
	if len(r.Snapshot()) != 3 {
		t.Fatalf("roster size = %d, want 3 (chunks assemble)", len(r.Snapshot()))
	}
	r.Apply(delta(1, 12, drec(2, 9)))
	if got := find(t, r, "c").Pose.X; got != 9 {
		t.Errorf("c.X = %v, want 9 (chunk index addressable)", got)
	}
}

func TestRosterUnknownIndexIgnored(t *testing.T) {
	r := NewRoster()
	r.Apply(kf(1, 0, 10, krec("a", 1)))
	r.Apply(delta(1, 11, drec(5, 9))) // index 5's chunk was lost
	if len(r.Snapshot()) != 1 {
		t.Fatal("delta for unknown index must not create entries")
	}
}

func TestRosterLateKeyframeChunkDoesNotRewind(t *testing.T) {
	r := NewRoster()
	r.Apply(kf(1, 0, 10, krec("a", 1)))
	r.Apply(delta(1, 12, drec(0, 12)))
	// A same-gen keyframe (periodic re-list) reordered behind the
	// delta: mapping stands, pose must not rewind.
	r.Apply(kf(1, 0, 11, krec("a", 1)))
	if got := find(t, r, "a").Pose.X; got != 12 {
		t.Errorf("a.X = %v, want 12 (late keyframe chunk ignored for pose)", got)
	}
	// A newer keyframe's pose does apply.
	r.Apply(kf(1, 0, 13, krec("a", 13)))
	if got := find(t, r, "a").Pose.X; got != 13 {
		t.Errorf("a.X = %v, want 13 (newer keyframe pose applied)", got)
	}
}

func TestRosterHeadFrameCarried(t *testing.T) {
	r := NewRoster()
	rec := krec("hud", 1)
	rec.HeadFrame = true
	r.Apply(kf(1, 0, 10, rec))
	if !find(t, r, "hud").HeadFrame {
		t.Error("head-frame flag must carry into the roster")
	}
}
