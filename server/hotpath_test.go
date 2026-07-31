package server

import (
	"testing"

	"github.com/panaudia/moqtransport"

	"github.com/panaudia/lasa/wire"
)

// fakePublisher is a no-op Publisher for allocation measurement.
type fakePublisher struct{ moqtransport.Publisher }

func (fakePublisher) SendDatagram(moqtransport.Object) error { return nil }

// The step-5 acceptance check from plan/server-spi.md: WriteFrame — the
// egress hot path — must not allocate at steady state. (The ingress path
// is the fork's DatagramReceiveHandler plus one atomic map read in
// onDatagram; its allocation freedom is asserted in the fork's own
// tests.)
func TestWriteFrameDoesNotAllocate(t *testing.T) {
	epoch := func(s uint64) uint64 { return 1_000_000_000 + s*1_000_000/48000 }
	w := &sinkWriter{mgr: newSinkManager(nil, epoch), sampleEpoch: epoch}
	w.r = &render{writer: w}
	pubs := []pubEntry{{pub: fakePublisher{}}, {pub: fakePublisher{}}}
	w.pubs.Store(&pubs)

	frame := make([]byte, 60)
	var sampleTS uint64
	// Warm up: first frame emits the ts anchor and grows the scratch.
	if err := w.WriteFrame(frame, sampleTS); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		sampleTS += wire.FrameSamples
		if err := w.WriteFrame(frame, sampleTS); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 0 {
		t.Errorf("WriteFrame allocates %.1f per frame on the hot path", allocs)
	}
}
