package client

// Roster is the client half of the presence contract (lasa-core.md §7)
// applied once, in the SDK: the index→entity mapping keyframes define,
// the generation discard rule for deltas, per-index latest-wins by
// datagram group-id, and the full tracking reset on generation change.
// Feed every presence packet through Apply (SubscribeRoster wires this
// up); read the roster with Snapshot.

import (
	"sync"

	"github.com/panaudia/lasa/wire"
)

// RosterEntry is one live entity as presence last reported it.
type RosterEntry struct {
	ID        string
	HeadFrame bool
	Pose      wire.Pose
	Loudness  wire.Loudness
}

type rosterSlot struct {
	entry     RosterEntry
	lastGroup uint64 // group-id of the last applied record (latest-wins)
}

// Roster is safe for concurrent use.
type Roster struct {
	mu    sync.Mutex
	gen   uint8
	seen  bool // a keyframe has established a generation
	slots map[uint16]*rosterSlot
}

func NewRoster() *Roster {
	return &Roster{slots: map[uint16]*rosterSlot{}}
}

// Apply feeds one presence packet — a *wire.PresenceKeyframe or
// *wire.PresenceDelta, as wire.ParsePresence returns — together with
// its datagram group-id. Anything else is ignored.
func (r *Roster) Apply(groupID uint64, msg any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch m := msg.(type) {
	case *wire.PresenceKeyframe:
		if !r.seen || m.Gen != r.gen {
			// New generation: membership changed — every per-index
			// tracking state resets with it (§7), which is also how
			// departure lands (absence from the new roster).
			r.seen, r.gen = true, m.Gen
			r.slots = map[uint16]*rosterSlot{}
		}
		for i, rec := range m.Records {
			idx := m.First + uint16(i)
			s, ok := r.slots[idx]
			if !ok {
				r.slots[idx] = &rosterSlot{
					entry: RosterEntry{
						ID: rec.ID, HeadFrame: rec.HeadFrame,
						Pose: rec.Pose, Loudness: rec.Loudness,
					},
					lastGroup: groupID,
				}
				continue
			}
			// The mapping is constant within a generation; only the
			// sampled pose/loudness are gated by latest-wins, so a
			// keyframe chunk arriving behind a newer delta cannot
			// rewind the pose.
			if groupID > s.lastGroup {
				s.entry.Pose, s.entry.Loudness = rec.Pose, rec.Loudness
				s.lastGroup = groupID
			}
		}
	case *wire.PresenceDelta:
		if !r.seen || m.Gen != r.gen {
			return // stale or pre-keyframe generation — discard (§7)
		}
		for _, rec := range m.Records {
			s, ok := r.slots[rec.Index]
			if !ok {
				continue // index's keyframe chunk lost; next round covers it
			}
			if groupID > s.lastGroup {
				s.entry.Pose, s.entry.Loudness = rec.Pose, rec.Loudness
				s.lastGroup = groupID
			}
		}
	}
}

// Snapshot returns the current roster in unspecified order.
func (r *Roster) Snapshot() []RosterEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RosterEntry, 0, len(r.slots))
	for _, s := range r.slots {
		out = append(out, s.entry)
	}
	return out
}
