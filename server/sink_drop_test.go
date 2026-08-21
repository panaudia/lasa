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

// E4 (spec-audit 2026-08-21): a failed Accept releases ensure's hold —
// a render with no subscriber is stopped, not leaked (§3.2). A
// concurrent pending hold or a live publisher keeps it alive.
func TestReleaseStopsUnpublishedRender(t *testing.T) {
	b := &countingBackend{}
	m := newSinkManager(b, nil)

	// Lone subscriber, Accept fails: render stops and the slot is free.
	r, err := m.ensure("e1", "binaural")
	if err != nil {
		t.Fatal(err)
	}
	m.release(r)
	if b.stops != 1 {
		t.Fatalf("unpublished render not stopped: stops = %d", b.stops)
	}
	if _, ok := m.renders[sinkKey{"e1", "binaural"}]; ok {
		t.Fatal("released render still registered")
	}

	// Two holds on one render: one release must not kill the other's.
	r1, _ := m.ensure("e2", "binaural")
	r2, _ := m.ensure("e2", "binaural")
	if r1 != r2 {
		t.Fatal("same (entity, format) must share a render")
	}
	m.release(r1)
	if b.stops != 1 {
		t.Fatal("release killed a render another subscriber holds")
	}
	m.addPublisher(r2, &fakePub{}, nil)

	// A live publisher keeps the render across a later failed Accept.
	r3, _ := m.ensure("e2", "binaural")
	m.release(r3)
	if b.stops != 1 {
		t.Fatal("release killed a render with a live publisher")
	}

	// dropConn respects a pending hold: publisher leaves while another
	// subscriber is mid-Accept — the render survives until that hold
	// resolves.
	cs := &connSession{}
	r4, _ := m.ensure("e3", "binaural")
	m.addPublisher(r4, &fakePub{}, cs)
	r5, _ := m.ensure("e3", "binaural")
	m.dropConn(cs)
	if b.stops != 1 {
		t.Fatal("dropConn killed a render with a pending hold")
	}
	m.release(r5)
	if b.stops != 2 {
		t.Fatalf("last hold released: render must stop, stops = %d", b.stops)
	}
}
