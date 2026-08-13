package server

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/panaudia/moqtransport"

	"github.com/panaudia/lasa/wire"
)

// capturePub records every datagram (payload copied — the writer reuses
// its scratch buffer across frames).
type capturePub struct {
	fakePub
	mu   sync.Mutex
	seqs []uint64
	pkts [][]byte
}

func (p *capturePub) SendDatagram(o moqtransport.Object) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seqs = append(p.seqs, o.GroupID)
	p.pkts = append(p.pkts, append([]byte(nil), o.Payload...))
	return nil
}

func frameBytes(i int) []byte { return []byte(fmt.Sprintf("opus-frame-%03d", i)) }

// TestSinkRedundancyEmission: with a declared offset registered, the
// render's packets carry the §5.1 redundancy framing — a full copy of
// the frame `offset` packets ago — from the first packet whose
// reference exists; without a declaration nothing changes on the wire.
func TestSinkRedundancyEmission(t *testing.T) {
	m := newSinkManager(&countingBackend{}, nil)
	m.setDecl("e1", 4)
	r, err := m.ensure("e1", "binaural")
	if err != nil {
		t.Fatal(err)
	}
	pub := &capturePub{}
	m.addPublisher(r, pub, nil)

	for i := 0; i < 10; i++ {
		if err := r.writer.WriteFrame(frameBytes(i), uint64(i*240)); err != nil {
			t.Fatal(err)
		}
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.pkts) != 10 {
		t.Fatalf("sent %d packets, want 10", len(pub.pkts))
	}
	for i, raw := range pub.pkts {
		if pub.seqs[i] != uint64(i) {
			t.Fatalf("packet %d: group id %d", i, pub.seqs[i])
		}
		p, err := wire.ParseSink(raw)
		if err != nil {
			t.Fatalf("packet %d: %v", i, err)
		}
		if !bytes.Equal(p.Audio, frameBytes(i)) {
			t.Fatalf("packet %d: wrong primary payload %q", i, p.Audio)
		}
		if i < 4 {
			if p.Redundancy != nil {
				t.Fatalf("packet %d: redundancy before the reference exists", i)
			}
			continue
		}
		if p.Redundancy == nil {
			t.Fatalf("packet %d: no redundancy", i)
		}
		if p.Redundancy.Offset != 4 {
			t.Fatalf("packet %d: offset %d, want 4", i, p.Redundancy.Offset)
		}
		if !bytes.Equal(p.Redundancy.Audio, frameBytes(i-4)) {
			t.Fatalf("packet %d: repeat payload %q, want frame %d", i, p.Redundancy.Audio, i-4)
		}
	}
}

// TestSinkRedundancyOffDefault: no declaration → plain packets (the
// zero-cost path), and a cleared declaration means an entity's NEXT
// render session starts plain (fresh render per §5.2 instance scoping).
func TestSinkRedundancyOffDefault(t *testing.T) {
	m := newSinkManager(&countingBackend{}, nil)
	r, err := m.ensure("e1", "binaural")
	if err != nil {
		t.Fatal(err)
	}
	pub := &capturePub{}
	m.addPublisher(r, pub, nil)
	for i := 0; i < 6; i++ {
		if err := r.writer.WriteFrame(frameBytes(i), 0); err != nil {
			t.Fatal(err)
		}
	}
	pub.mu.Lock()
	for i, raw := range pub.pkts {
		p, err := wire.ParseSink(raw)
		if err != nil {
			t.Fatalf("packet %d: %v", i, err)
		}
		if p.Redundancy != nil {
			t.Fatalf("packet %d: unexpected redundancy", i)
		}
	}
	pub.mu.Unlock()

	// Declared then cleared (departure): the replacement render is plain.
	m.setDecl("e2", 3)
	m.clearDecl("e2")
	r2, err := m.ensure("e2", "binaural")
	if err != nil {
		t.Fatal(err)
	}
	if r2.writer.red != 0 {
		t.Fatalf("cleared declaration leaked into a new render: red=%d", r2.writer.red)
	}
}

// TestSinkRedundancyHistoryWithoutSubscribers: the id space and the
// repeat history advance while no subscriber is attached, so a
// mid-render join immediately receives correctly referenced repeats.
func TestSinkRedundancyHistoryWithoutSubscribers(t *testing.T) {
	m := newSinkManager(&countingBackend{}, nil)
	m.setDecl("e1", 4)
	r, err := m.ensure("e1", "binaural")
	if err != nil {
		t.Fatal(err)
	}
	// Frames 0..4 with nobody listening.
	for i := 0; i < 5; i++ {
		if err := r.writer.WriteFrame(frameBytes(i), 0); err != nil {
			t.Fatal(err)
		}
	}
	pub := &capturePub{}
	m.addPublisher(r, pub, nil)
	if err := r.writer.WriteFrame(frameBytes(5), 0); err != nil {
		t.Fatal(err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.pkts) != 1 || pub.seqs[0] != 5 {
		t.Fatalf("got %d packets, first seq %v", len(pub.pkts), pub.seqs)
	}
	p, err := wire.ParseSink(pub.pkts[0])
	if err != nil {
		t.Fatal(err)
	}
	if p.Redundancy == nil || !bytes.Equal(p.Redundancy.Audio, frameBytes(1)) {
		t.Fatalf("mid-render join: repeat not referenced from pre-join history: %+v", p.Redundancy)
	}
}
