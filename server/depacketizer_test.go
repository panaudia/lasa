package server

import (
	"fmt"
	"testing"

	"github.com/panaudia/lasa/wire"
)

// recorder captures the Depacketizer's ordered output.
type recEvent struct {
	lost        bool
	seq         uint64
	audio       []byte
	hasPose     bool
	poseX       float32
	audioLikely bool
}

type recorder struct{ events []recEvent }

func (r *recorder) Frame(seq uint64, pose *wire.Pose, audio []byte) {
	e := recEvent{seq: seq, audio: append([]byte(nil), audio...)}
	if pose != nil {
		e.hasPose = true
		e.poseX = pose.X
	}
	r.events = append(r.events, e)
}

func (r *recorder) Lost(seq uint64, audioLikely bool) {
	r.events = append(r.events, recEvent{lost: true, seq: seq, audioLikely: audioLikely})
}

// audioFor gives each seq distinct recognisable audio bytes.
func audioFor(seq uint64) []byte {
	return []byte{0xA0, byte(seq), byte(seq >> 8), 0x0F}
}

// mk builds a mono-object payload: pose optional, audio optional,
// redundancy = copy of frame (seq−offset) when offset > 0.
func mk(t *testing.T, seq uint64, withPose, withAudio bool, offset uint8) []byte {
	t.Helper()
	p := wire.MonoObjectPacket{}
	if withPose {
		p.Pose = &wire.Pose{X: float32(seq)}
	}
	if withAudio {
		p.Audio = audioFor(seq)
	}
	if offset > 0 {
		p.Redundancy = &wire.Redundancy{Offset: offset, Audio: audioFor(seq - uint64(offset))}
	}
	buf, err := wire.AppendMonoObject(nil, &p)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

// expect asserts the exact ordered event stream.
func expect(t *testing.T, r *recorder, want []recEvent) {
	t.Helper()
	if len(r.events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(r.events), len(want), r.events)
	}
	for i, w := range want {
		g := r.events[i]
		if g.lost != w.lost || g.seq != w.seq {
			t.Fatalf("event %d: got %+v want %+v", i, g, w)
		}
		if !w.lost && w.audio != nil && string(g.audio) != string(w.audio) {
			t.Fatalf("event %d audio: got %x want %x", i, g.audio, w.audio)
		}
		if w.lost && g.audioLikely != w.audioLikely {
			t.Fatalf("event %d audioLikely: got %v want %v", i, g.audioLikely, w.audioLikely)
		}
	}
}

func frame(seq uint64) recEvent { return recEvent{seq: seq, audio: audioFor(seq)} }
func lost(seq uint64) recEvent  { return recEvent{lost: true, seq: seq, audioLikely: true} }

func TestDepacketizerInOrder(t *testing.T) {
	r := &recorder{}
	d := NewDepacketizer(r)
	var want []recEvent
	for s := uint64(0); s < 10; s++ {
		d.WritePacket(s, mk(t, s, true, true, 0))
		want = append(want, frame(s))
	}
	expect(t, r, want)
	st := d.Stats()
	if st.Delivered != 10 || st.Lost != 0 || st.GapEvents != 0 || d.pendingCount != 0 {
		t.Fatalf("stats: %+v pending %d", st, d.pendingCount)
	}
	// Pose came through the fast path.
	if !r.events[3].hasPose || r.events[3].poseX != 3 {
		t.Fatalf("pose lost on fast path: %+v", r.events[3])
	}
}

func TestDepacketizerFastPathAllocs(t *testing.T) {
	r := &nullReceiver{}
	d := NewDepacketizer(r)
	payload := mk(t, 0, true, true, 3) // pose + audio + redundancy: the fat clean-path packet
	seq := uint64(0)
	d.WritePacket(seq, payload)
	allocs := testing.AllocsPerRun(1000, func() {
		seq++
		d.WritePacket(seq, payload)
	})
	if allocs > 0 {
		t.Errorf("clean path allocates %.2f per packet", allocs)
	}
}

type nullReceiver struct{}

func (nullReceiver) Frame(uint64, *wire.Pose, []byte) {}
func (nullReceiver) Lost(uint64, bool)                {}

func TestDepacketizerSingleLossRecovery(t *testing.T) {
	for k := uint8(1); k <= 7; k++ {
		t.Run(fmt.Sprintf("k=%d", k), func(t *testing.T) {
			r := &recorder{}
			d := NewDepacketizer(r)
			const lostSeq = 10
			var want []recEvent
			for s := uint64(0); s < 25; s++ {
				if s == lostSeq {
					continue // lost on the network
				}
				off := k
				if s < uint64(k) {
					off = 0 // stream start: nothing to carry yet
				}
				d.WritePacket(s, mk(t, s, false, true, off))
			}
			for s := uint64(0); s < 25; s++ {
				want = append(want, frame(s))
			}
			expect(t, r, want)
			st := d.Stats()
			if st.Recovered != 1 || st.Lost != 0 {
				t.Fatalf("recovered %d lost %d, want 1/0", st.Recovered, st.Lost)
			}
		})
	}
}

func TestDepacketizerBurstFullyRecovered(t *testing.T) {
	// k=3 covers a 3-burst completely (the sliding scheme).
	r := &recorder{}
	d := NewDepacketizer(r)
	for s := uint64(0); s < 30; s++ {
		if s >= 10 && s <= 12 {
			continue
		}
		off := uint8(3)
		if s < 3 {
			off = 0
		}
		d.WritePacket(s, mk(t, s, false, true, off))
	}
	var want []recEvent
	for s := uint64(0); s < 30; s++ {
		want = append(want, frame(s))
	}
	expect(t, r, want)
	if st := d.Stats(); st.Recovered != 3 || st.Lost != 0 {
		t.Fatalf("stats %+v, want 3 recovered 0 lost", st)
	}
}

func TestDepacketizerBurstBeyondK(t *testing.T) {
	// k=2, burst of 4 (10..13): copies of 10,11 ride in 12,13 (also
	// lost) — unrecoverable; 12,13 recover from 14,15. Exact order:
	// Lost(10) Lost(11) then recovered 12,13, then the held tail.
	r := &recorder{}
	d := NewDepacketizer(r)
	for s := uint64(0); s < 30; s++ {
		if s >= 10 && s <= 13 {
			continue
		}
		off := uint8(2)
		if s < 2 {
			off = 0
		}
		d.WritePacket(s, mk(t, s, false, true, off))
	}
	var want []recEvent
	for s := uint64(0); s < 10; s++ {
		want = append(want, frame(s))
	}
	want = append(want, lost(10), lost(11), frame(12), frame(13))
	for s := uint64(14); s < 30; s++ {
		want = append(want, frame(s))
	}
	expect(t, r, want)
	st := d.Stats()
	if st.Recovered != 2 || st.Lost != 2 || st.GapEvents != 1 {
		t.Fatalf("stats %+v", st)
	}
	if st.GapHist[1] != 1 { // run of 2 lost
		t.Fatalf("gap hist %+v", st.GapHist)
	}
}

func TestDepacketizerCarrierAlsoLost(t *testing.T) {
	// k=1: lose 10 and 11 — 10's copy rode in 11 (lost) → Lost(10);
	// 11's copy rides in 12 → recovered.
	r := &recorder{}
	d := NewDepacketizer(r)
	for s := uint64(0); s < 25; s++ {
		if s == 10 || s == 11 {
			continue
		}
		off := uint8(1)
		if s < 1 {
			off = 0
		}
		d.WritePacket(s, mk(t, s, false, true, off))
	}
	var want []recEvent
	for s := uint64(0); s < 10; s++ {
		want = append(want, frame(s))
	}
	want = append(want, lost(10), frame(11))
	for s := uint64(12); s < 25; s++ {
		want = append(want, frame(s))
	}
	expect(t, r, want)
	if st := d.Stats(); st.Recovered != 1 || st.Lost != 1 {
		t.Fatalf("stats %+v", st)
	}
}

func TestDepacketizerReorder(t *testing.T) {
	// Plain swap, no redundancy: held and delivered in order, no Lost.
	r := &recorder{}
	d := NewDepacketizer(r)
	order := []uint64{0, 1, 3, 2, 4, 5, 7, 6, 8}
	for _, s := range order {
		d.WritePacket(s, mk(t, s, false, true, 0))
	}
	var want []recEvent
	for s := uint64(0); s < 9; s++ {
		want = append(want, frame(s))
	}
	expect(t, r, want)
	st := d.Stats()
	if st.Lost != 0 || st.Duplicates != 0 || st.Late != 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestDepacketizerDuplicatesAndLate(t *testing.T) {
	r := &recorder{}
	d := NewDepacketizer(r)
	d.WritePacket(0, mk(t, 0, false, true, 0))
	d.WritePacket(0, mk(t, 0, false, true, 0)) // late (already delivered)
	d.WritePacket(2, mk(t, 2, false, true, 0)) // gap → held
	d.WritePacket(2, mk(t, 2, false, true, 0)) // duplicate while pending
	d.WritePacket(1, mk(t, 1, false, true, 0)) // fills the gap
	expect(t, r, []recEvent{frame(0), frame(1), frame(2)})
	st := d.Stats()
	if st.Late != 1 || st.Duplicates != 1 {
		t.Fatalf("late %d dup %d", st.Late, st.Duplicates)
	}
}

func TestDepacketizerStallResumeJump(t *testing.T) {
	// 0..5, then a stall and resume at 1000: the conceal cap bounds
	// Lost at maxConcealRun; the rest of the hole is skipped silently;
	// the resume packet itself waits out the 7-packet horizon.
	r := &recorder{}
	d := NewDepacketizer(r)
	for s := uint64(0); s < 6; s++ {
		d.WritePacket(s, mk(t, s, false, true, 0))
	}
	for s := uint64(1000); s < 1012; s++ {
		d.WritePacket(s, mk(t, s, false, true, 0))
	}
	st := d.Stats()
	if st.Lost != maxConcealRun {
		t.Fatalf("lost %d, want the conceal cap %d", st.Lost, maxConcealRun)
	}
	// The whole hole resolves as the resume packets push the horizon:
	// 994 missing = the conceal cap Lost + the rest skipped.
	wantSkipped := uint64(1000-6) - maxConcealRun
	if st.Skipped != wantSkipped {
		t.Fatalf("skipped %d, want %d", st.Skipped, wantSkipped)
	}
	// Every resume packet was delivered, in order, and nothing is held.
	tail := r.events[len(r.events)-12:]
	for i, e := range tail {
		if e.lost || e.seq != uint64(1000+i) {
			t.Fatalf("tail event %d: %+v", i, e)
		}
	}
	if d.pendingCount != 0 {
		t.Fatalf("pending %d after full drain", d.pendingCount)
	}
}

func TestDepacketizerPoseOnlyLostHint(t *testing.T) {
	// Losses inside a pose-only stretch are hinted audioLikely=false.
	r := &recorder{}
	d := NewDepacketizer(r)
	for s := uint64(0); s < 20; s++ {
		if s == 10 {
			continue // lost pose-only packet
		}
		d.WritePacket(s, mk(t, s, true, false, 0)) // pose-only stream
	}
	for _, e := range r.events {
		if e.lost && e.audioLikely {
			t.Fatalf("pose-only loss hinted as audio: %+v", e)
		}
	}
	if st := d.Stats(); st.Lost != 1 {
		t.Fatalf("lost %d, want 1", st.Lost)
	}
}

func TestDepacketizerMalformedDiscard(t *testing.T) {
	r := &recorder{}
	d := NewDepacketizer(r)
	d.WritePacket(0, []byte{0x80, 1, 2}) // reserved flag set → discard
	d.WritePacket(0, []byte{})           // empty → discard
	if len(r.events) != 0 {
		t.Fatalf("malformed packets reached the receiver: %+v", r.events)
	}
	if st := d.Stats(); st.Malformed != 2 {
		t.Fatalf("malformed %d, want 2", st.Malformed)
	}
}
