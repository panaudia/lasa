package server

import (
	"sync"
	"testing"

	"github.com/panaudia/moqtransport"
)

// fakePub records CloseWithError; the send methods are never used here.
type fakePub struct {
	mu     sync.Mutex
	closed bool
	code   uint64
}

func (p *fakePub) SendDatagram(moqtransport.Object) error { return nil }
func (p *fakePub) OpenSubgroup(uint64, uint64, uint8, ...moqtransport.SubgroupOption) (*moqtransport.Subgroup, error) {
	return nil, nil
}
func (p *fakePub) CloseWithError(code uint64, reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed, p.code = true, code
	return nil
}

type countingSession struct{ stops *int }

func (s countingSession) Stop() { *s.stops++ }

type countingBackend struct {
	stubBackend
	starts int
	stops  int
}

func (b *countingBackend) StartSink(entityID, format string, w SinkWriter) (SinkSession, error) {
	b.starts++
	return countingSession{stops: &b.stops}, nil
}

// TestDropEntityEndsTracks: entity departure stops the entity's renders,
// ends subscriber tracks with SUBSCRIBE_DONE track-ended, and leaves the
// manager ready to start a FRESH render if the entity returns (option A,
// 2026-08-01 — no stale entry survives departure).
func TestDropEntityEndsTracks(t *testing.T) {
	b := &countingBackend{}
	m := newSinkManager(b, nil)

	r, err := m.ensure("e1", "binaural")
	if err != nil {
		t.Fatal(err)
	}
	pub := &fakePub{}
	m.addPublisher(r, pub, nil)
	if b.starts != 1 {
		t.Fatalf("starts = %d", b.starts)
	}

	m.dropEntity("e1")
	if b.stops != 1 {
		t.Fatalf("render not stopped: stops = %d", b.stops)
	}
	pub.mu.Lock()
	closed, code := pub.closed, pub.code
	pub.mu.Unlock()
	if !closed || code != uint64(moqtransport.ErrorCodeSubscribeDoneTrackEnded) {
		t.Fatalf("subscriber track not ended: closed=%v code=%#x", closed, code)
	}

	// Unrelated entities are untouched; the departed one restarts fresh.
	if _, err := m.ensure("e2", "binaural"); err != nil {
		t.Fatal(err)
	}
	m.dropEntity("e1") // idempotent, no renders match
	if b.stops != 1 {
		t.Fatalf("dropEntity not idempotent: stops = %d", b.stops)
	}
	if _, err := m.ensure("e1", "binaural"); err != nil {
		t.Fatal(err)
	}
	if b.starts != 3 {
		t.Fatalf("returning entity did not get a fresh render: starts = %d", b.starts)
	}
}
