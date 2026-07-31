// Package presence implements the LASA presence-track machinery
// (lasa-core.md §7): generations, implicit index assignment, keyframe
// chunking, and delta emission. It is deliberately transport-free — a
// Sender is one function — so the same machinery serves the in-process
// server shell today and a decomposed presence composer later
// (plan/decomposition-seams.md).
//
// The presence wire format has a single author by design (the generation
// byte and implicit indexes only work with one roster owner); one Author
// instance is that author for one space.
package presence

import (
	"sync"
	"time"

	"github.com/panaudia/lasa/wire"
)

// Entry is one entity's presence sample — the aggregation currency: a
// future composer merges []Entry from many sources and feeds one Author.
type Entry struct {
	EntityID  string
	Pose      wire.Pose
	Loudness  wire.Loudness
	HeadFrame bool
}

// Sender delivers one presence datagram to one subscriber. group is the
// MoQ group id (+1 per datagram, object 0); payload is the encoded
// packet. Senders must not block.
type Sender func(group uint64, payload []byte)

// chunkBytes bounds one presence datagram's payload.
const chunkBytes = 1150

// Author consumes the Collector's sampled roster and authors the
// presence track — subscriber #1 of the entry-level pubsub (design §6:
// the track machinery as a subscriber adapter; Author.Tick satisfies
// Subscriber).
type Author struct {
	kfInterval time.Duration

	mu      sync.Mutex
	senders map[any]Sender
	forced  bool

	// Tick-owned state.
	group   uint64
	gen     uint8
	roster  []string // index order, fixed within a generation
	last    map[string]Entry
	lastKF  time.Time
	started bool
}

// NewAuthor creates an author emitting keyframes at least every
// kfInterval (plus immediately on membership change and new
// subscribers). Feed it by subscribing a.Tick to a Collector.
func NewAuthor(kfInterval time.Duration) *Author {
	return &Author{
		kfInterval: kfInterval,
		senders:    map[any]Sender{},
		last:       map[string]Entry{},
	}
}

// AddSender registers a subscriber under key and forces a keyframe on
// the next tick so the newcomer gets the roster promptly.
func (a *Author) AddSender(key any, s Sender) {
	a.mu.Lock()
	a.senders[key] = s
	a.forced = true
	a.mu.Unlock()
}

// RemoveSender removes the subscriber registered under key.
func (a *Author) RemoveSender(key any) {
	a.mu.Lock()
	delete(a.senders, key)
	a.mu.Unlock()
}

// Tick consumes one sampled roster and emits at most one round of
// datagrams: a keyframe round on membership change, forced request, or
// keyframe cadence; otherwise a delta of changed entries. Its signature
// is the Collector's Subscriber; one goroutine (the Collector's pacer)
// drives an Author.
func (a *Author) Tick(now time.Time, entries []Entry) {
	a.mu.Lock()
	nSenders := len(a.senders)
	forced := a.forced
	a.forced = false
	a.mu.Unlock()
	if nSenders == 0 {
		return
	}

	byID := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byID[e.EntityID] = e
	}

	changed := !a.started || len(byID) != len(a.roster)
	if !changed {
		for _, id := range a.roster {
			if _, ok := byID[id]; !ok {
				changed = true
				break
			}
		}
	}

	switch {
	case changed:
		if a.started {
			a.gen++ // the first roster is generation 0
		}
		a.started = true
		a.roster = a.roster[:0]
		for _, e := range entries {
			a.roster = append(a.roster, e.EntityID)
		}
		a.sendKeyframes(byID)
		a.lastKF = now
	case forced || now.Sub(a.lastKF) >= a.kfInterval:
		a.sendKeyframes(byID)
		a.lastKF = now
	default:
		a.sendDeltas(byID)
	}
	a.last = byID
}

func (a *Author) sendKeyframes(byID map[string]Entry) {
	kf := wire.PresenceKeyframe{Gen: a.gen}
	first := 0
	var size int
	flush := func(next int) {
		kf.First = uint16(first)
		if buf, err := wire.AppendPresenceKeyframe(nil, &kf); err == nil {
			a.broadcast(buf)
		}
		kf.Records = kf.Records[:0]
		first = next
		size = 0
	}
	for i, id := range a.roster {
		e := byID[id]
		recSize := 2 + 1 + len(id) + wire.PoseSize + 1
		if size+recSize > chunkBytes && len(kf.Records) > 0 {
			flush(i)
		}
		kf.Records = append(kf.Records, wire.KeyframeRecord{
			HeadFrame: e.HeadFrame, ID: id, Pose: e.Pose, Loudness: e.Loudness,
		})
		size += recSize
	}
	// The final chunk — or, for an empty roster, the empty keyframe: an
	// empty space must be representable (departure is keyframe absence).
	flush(len(a.roster))
}

func (a *Author) sendDeltas(byID map[string]Entry) {
	d := wire.PresenceDelta{Gen: a.gen}
	flush := func() {
		if len(d.Records) == 0 {
			return
		}
		if buf, err := wire.AppendPresenceDelta(nil, &d); err == nil {
			a.broadcast(buf)
		}
		d.Records = d.Records[:0]
	}
	for i, id := range a.roster {
		e := byID[id]
		if e == a.last[id] {
			continue
		}
		d.Records = append(d.Records, wire.DeltaRecord{
			Index: uint16(i), Pose: e.Pose, Loudness: e.Loudness,
		})
		if 2+len(d.Records)*(2+wire.PoseSize+1) > chunkBytes {
			flush()
		}
	}
	flush()
}

// broadcast sends one datagram to every sender: group id +1 per
// datagram, shared by all subscribers — presence is the one shared,
// cacheable track.
func (a *Author) broadcast(payload []byte) {
	a.mu.Lock()
	senders := make([]Sender, 0, len(a.senders))
	for _, s := range a.senders {
		senders = append(senders, s)
	}
	a.mu.Unlock()
	for _, s := range senders {
		s(a.group, payload)
	}
	a.group++
}
