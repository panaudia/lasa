package conformance

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/presence"
	"github.com/panaudia/lasa/server"
	"github.com/panaudia/lasa/wire"
)

// Observer is the permissive server's receiving half
// (plan/conformance-suite.md §2.4, step 4): a space that admits a
// client under test, subscribes its source tracks, and records what
// the client sent against the §5.1 sender rules. It sits behind the
// reference shell's SPI, so admission, subscription and datagram
// routing are the shell's, and the observer sees each packet as
// (group id, payload) per entity, per connection instance.
//
// What the SPI does not carry, and the observer therefore cannot
// check: the MoQ object id (C-5.1-1's "object-id is 0"), and the
// subgroup layout of the state source track (C-6.7-11). Both wait for
// the raw layer.
type Observer struct {
	Fixture *Fixture
	backend *observingBackend
	closeFn func()
}

// StartObserver boots an observer space on tr. Unticketed, so a client
// under test needs no ticket. mod may tune the shell.
func StartObserver(tr Transport, mod func(*server.Config)) (*Observer, error) {
	b := newObservingBackend()
	f, closeFn, err := startShell(tr, false, b, mod)
	if err != nil {
		return nil, err
	}
	f.Name = "observer-" + string(tr)
	return &Observer{Fixture: f, backend: b, closeFn: closeFn}, nil
}

// Close stops the observer space.
func (o *Observer) Close() { o.closeFn() }

// Reports returns one report per connection instance that carried
// entityID, oldest first. Copies, safe to inspect while packets flow.
func (o *Observer) Reports(entityID string) []TrackReport {
	return o.backend.reports(entityID)
}

// TrackReport is the evidence for one source track instance.
type TrackReport struct {
	ClientID string
	EntityID string
	Instance int // 1 for the first connection carrying the entity
	Declared int // the entity's declared redundancy (§4.2)

	// Lockstep is set when the connection declared lockstep with at
	// least two entities: this track's index and the set's size, and
	// the widest spread ever seen between the members' newest ids
	// while this instance was live. In step means a spread that stays
	// within the receiver's tolerance (C-5.2-4).
	Lockstep *LockstepEvidence

	Received  int
	First     uint64
	Last      uint64
	Gaps      int // ids skipped between consecutive packets
	Dups      int // an id seen twice
	Backwards int // an id below its predecessor

	Audio    int // audio-bearing packets
	PoseOnly int
	Empty    int // flags 0x00
	// Sender rule violations.
	UnknownFlags          int // reserved or ts bits set (C-5.1-6, C-5.1-14)
	Malformed             int // did not parse to the layout
	OffsetOverDeclaration int // redundancy offset above Declared (C-4.2-10)
	Redundant             int // packets carrying a copy
	RefNotAudio           int // the copy references a packet that carried no audio (C-5.1-13)
	RefMismatch           int // the copy differs from the referenced packet's audio (C-5.1-12)
	RefUnseen             int // the referenced packet never arrived, so the copy could not be compared
}

// LockstepEvidence is the cross-track evidence for one lockstep set.
type LockstepEvidence struct {
	Index     int
	Count     int
	MaxSpread uint64
}

const observerHistory = 64

// lockGroup is one connection instance's lockstep set: the newest id
// seen on any member, and the most an arriving packet has trailed it.
// The same definition as the server's gather stage reports, so the
// two agree by construction: 0 in step, d for a member d frames
// behind, and the release rule fires at 8.
type lockGroup struct {
	mu      sync.Mutex
	started bool
	newest  uint64
	spread  uint64
}

func (g *lockGroup) note(entity string, seq uint64) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.started || seq > g.newest {
		g.newest, g.started = seq, true
	}
	if s := g.newest - seq; s > g.spread {
		g.spread = s
	}
	return g.spread
}

type observingBackend struct {
	mu        sync.Mutex
	collector *presence.Collector
	byEntity  map[string][]*observedTrack
	groups    map[string]*lockGroup // by client id, the live connection's set
	stub      *stubBackend          // sinks and presence, so a client under test may also listen
}

func newObservingBackend() *observingBackend {
	return &observingBackend{byEntity: map[string][]*observedTrack{}, groups: map[string]*lockGroup{}, stub: newStubBackend()}
}

func (b *observingBackend) attach(c *presence.Collector) {
	b.mu.Lock()
	b.collector = c
	b.mu.Unlock()
	b.stub.attach(c)
}

type observedTrack struct {
	mu      sync.Mutex
	rep     TrackReport
	started bool
	// history holds the audio of the last observerHistory seqs (nil
	// for a packet without audio) so a redundant copy can be checked
	// against what it claims to repeat.
	history [observerHistory][]byte
	seen    [observerHistory]uint64
	valid   [observerHistory]bool
	slot    *presence.Slot
	group   *lockGroup
}

func (b *observingBackend) EntityJoined(clientID string, e connect.ResolvedEntity) (server.EntityHandler, error) {
	tr := &observedTrack{rep: TrackReport{ClientID: clientID, EntityID: e.ID, Declared: e.Redundancy}}
	b.mu.Lock()
	b.byEntity[e.ID] = append(b.byEntity[e.ID], tr)
	tr.rep.Instance = len(b.byEntity[e.ID])
	if e.Lockstep != nil {
		tr.rep.Lockstep = &LockstepEvidence{Index: e.Lockstep.Index, Count: e.Lockstep.Count}
		g, ok := b.groups[clientID]
		if !ok || e.Lockstep.Index == 0 {
			// The first member of a new instance opens a new set.
			g = &lockGroup{}
			b.groups[clientID] = g
		}
		tr.group = g
	}
	if b.collector != nil {
		tr.slot = b.collector.Register(e.ID, e.Frame == "head", func() wire.Loudness { return 255 })
	}
	b.mu.Unlock()
	return tr, nil
}

func (b *observingBackend) EntityLeft(clientID, entityID string) {}

func (b *observingBackend) StartSink(entityID, format string, w server.SinkWriter) (server.SinkSession, error) {
	return b.stub.StartSink(entityID, format, w)
}

func (b *observingBackend) reports(entityID string) []TrackReport {
	b.mu.Lock()
	tracks := append([]*observedTrack(nil), b.byEntity[entityID]...)
	b.mu.Unlock()
	out := make([]TrackReport, 0, len(tracks))
	for _, tr := range tracks {
		tr.mu.Lock()
		out = append(out, tr.rep)
		tr.mu.Unlock()
	}
	return out
}

// WritePacket is the SPI ingress: one datagram, raw, with its group id.
func (t *observedTrack) WritePacket(seq uint64, payload []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := &t.rep
	r.Received++
	if t.group != nil {
		r.Lockstep.MaxSpread = t.group.note(r.EntityID, seq)
	}
	if !t.started {
		t.started = true
		r.First, r.Last = seq, seq
	} else {
		switch {
		case seq == r.Last+1:
		case seq == r.Last:
			r.Dups++
		case seq < r.Last:
			r.Backwards++
		default:
			r.Gaps += int(seq - r.Last - 1)
		}
		if seq > r.Last {
			r.Last = seq
		}
	}

	pkt, err := wire.ParseMonoObject(payload)
	if err != nil {
		if errors.Is(err, wire.ErrUnknownFlags) {
			r.UnknownFlags++
		} else {
			r.Malformed++
		}
		t.remember(seq, nil)
		return
	}
	switch {
	case pkt.Audio != nil:
		r.Audio++
	case pkt.Pose != nil:
		r.PoseOnly++
	default:
		r.Empty++
	}
	if pkt.Pose != nil && t.slot != nil {
		t.slot.UpdatePose(*pkt.Pose)
	}
	if pkt.Redundancy != nil {
		r.Redundant++
		if int(pkt.Redundancy.Offset) > r.Declared {
			r.OffsetOverDeclaration++
		}
		ref := seq - uint64(pkt.Redundancy.Offset)
		if audio, ok := t.lookup(ref); !ok {
			r.RefUnseen++
		} else if audio == nil {
			r.RefNotAudio++
		} else if !bytes.Equal(audio, pkt.Redundancy.Audio) {
			r.RefMismatch++
		}
	}
	var audio []byte
	if pkt.Audio != nil {
		audio = append([]byte(nil), pkt.Audio...)
	}
	t.remember(seq, audio)
}

func (t *observedTrack) remember(seq uint64, audio []byte) {
	i := seq % observerHistory
	t.history[i], t.seen[i], t.valid[i] = audio, seq, true
}

func (t *observedTrack) lookup(seq uint64) ([]byte, bool) {
	i := seq % observerHistory
	if !t.valid[i] || t.seen[i] != seq {
		return nil, false
	}
	return t.history[i], true
}

// Summary is one line per report for test output.
func (r TrackReport) Summary() string {
	return fmt.Sprintf("%s/%s#%d: %d received, ids %d..%d, gaps %d dups %d back %d; audio %d pose-only %d empty %d; flags %d malformed %d; redundant %d over-decl %d ref-not-audio %d ref-mismatch %d ref-unseen %d",
		r.ClientID, r.EntityID, r.Instance, r.Received, r.First, r.Last, r.Gaps, r.Dups, r.Backwards,
		r.Audio, r.PoseOnly, r.Empty, r.UnknownFlags, r.Malformed,
		r.Redundant, r.OffsetOverDeclaration, r.RefNotAudio, r.RefMismatch, r.RefUnseen)
}
