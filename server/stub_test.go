package server

import (
	"sync"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// stubBackend is the in-repo test embedder (plan/server-spi.md): it
// decodes ingress into per-entity pose/audio state, renders counter
// frames on sinks, answers presence from applied poses, and admits
// everything.
type stubBackend struct {
	mu       sync.Mutex
	entities map[string]*stubEntity
	admitted []string
	left     []string
}

type stubEntity struct {
	mu      sync.Mutex
	packets []stubPacket
	pose    wire.Pose
	poseSeq uint64
	hasPose bool          // group ids legitimately start at 0: "no pose yet" is its own state
	arrived chan struct{} // closed-ish signal: buffered notifications
}

type stubPacket struct {
	seq   uint64
	audio []byte
}

func newStubBackend() *stubBackend {
	return &stubBackend{entities: map[string]*stubEntity{}}
}

func (b *stubBackend) Admit(res *connect.Resolved) *connect.Rejection {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.admitted = append(b.admitted, res.ClientID)
	return nil
}

func (b *stubBackend) EntityJoined(clientID string, e connect.ResolvedEntity) (EntityHandler, error) {
	ent := &stubEntity{arrived: make(chan struct{}, 64)}
	b.mu.Lock()
	b.entities[e.ID] = ent
	b.mu.Unlock()
	return ent, nil
}

func (b *stubBackend) EntityLeft(clientID, entityID string) {
	b.mu.Lock()
	b.left = append(b.left, entityID)
	b.mu.Unlock()
}

func (b *stubBackend) entity(id string) *stubEntity {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.entities[id]
}

// WritePacket is the embedder side of the ingress contract: decode (the
// slices alias the payload we now own), apply latest-wins pose, hand
// audio off — here into a slice under a short lock, standing in for a
// jitter buffer.
func (e *stubEntity) WritePacket(seq uint64, payload []byte) {
	pkt, err := wire.ParseMonoObject(payload)
	if err != nil {
		return // discard rule
	}
	e.mu.Lock()
	if pkt.Pose != nil && (!e.hasPose || seq > e.poseSeq) {
		e.pose, e.poseSeq, e.hasPose = *pkt.Pose, seq, true
	}
	if pkt.Audio != nil {
		e.packets = append(e.packets, stubPacket{seq: seq, audio: pkt.Audio})
	}
	e.mu.Unlock()
	select {
	case e.arrived <- struct{}{}:
	default:
	}
}

// StartSink renders 5 ms counter frames: frame i is [i, entity-agnostic
// pattern...], pushed with a synthetic sample clock.
func (b *stubBackend) StartSink(entityID, format string, w SinkWriter) (SinkSession, error) {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		var i byte
		ts := uint64(1_000_000_000)
		frame := make([]byte, 60)
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				frame[0] = i
				_ = w.WriteFrame(frame, ts)
				i++
				ts += wire.FrameMicroseconds
			}
		}
	}()
	return stubSession{stop: stop}, nil
}

type stubSession struct{ stop chan struct{} }

func (s stubSession) Stop() { close(s.stop) }

func (b *stubBackend) Presence() []PresenceEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]PresenceEntry, 0, len(b.entities))
	for id, e := range b.entities {
		e.mu.Lock()
		out = append(out, PresenceEntry{EntityID: id, Pose: e.pose, Loudness: 10})
		e.mu.Unlock()
	}
	return out
}
