package server

import (
	"sync"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/presence"
	"github.com/panaudia/lasa/wire"
)

// stubBackend is the in-repo test embedder (plan/server-spi.md): it
// decodes ingress into per-entity pose/audio state, feeds the presence
// collector, renders counter frames on sinks, and admits everything.
// attach() wires the server's collector after construction.
type stubBackend struct {
	mu        sync.Mutex
	entities  map[string]*stubEntity
	left      []string
	collector *presence.Collector
}

func (b *stubBackend) attach(c *presence.Collector) {
	b.mu.Lock()
	b.collector = c
	b.mu.Unlock()
}

type stubEntity struct {
	slot    *presence.Slot
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

func (b *stubBackend) EntityJoined(clientID string, e connect.ResolvedEntity) (EntityHandler, error) {
	ent := &stubEntity{arrived: make(chan struct{}, 64)}
	b.mu.Lock()
	if b.collector != nil {
		ent.slot = b.collector.Register(e.ID, false, func() wire.Loudness { return 10 })
	}
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
		if e.slot != nil {
			e.slot.UpdatePose(e.pose)
		}
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
		var sampleTS uint64 // the embedder's sample clock (design §1)
		frame := make([]byte, 60)
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				frame[0] = i
				_ = w.WriteFrame(frame, sampleTS)
				i++
				sampleTS += wire.FrameSamples
			}
		}
	}()
	return stubSession{stop: stop}, nil
}

type stubSession struct{ stop chan struct{} }

func (s stubSession) Stop() { close(s.stop) }
