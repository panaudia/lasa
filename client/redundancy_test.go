package client

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/panaudia/lasa/wire"
)

func audioOf(seq uint64) []byte { return []byte(fmt.Sprintf("frame-%03d", seq)) }

// roundTrip encodes what Attach produced and parses it back — the wire
// is the contract, and it rejects a redundancy record on an audio-less
// packet, so any attempt to attach one surfaces here as an error.
func roundTrip(t *testing.T, pkt *wire.MonoObjectPacket) *wire.MonoObjectPacket {
	t.Helper()
	raw, err := wire.AppendMonoObject(nil, pkt)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := wire.ParseMonoObject(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return out
}

// TestUplinkRedundancyContinuousAudio: on an all-audio stream the copy
// attaches from the first seq whose reference exists, at the offset,
// carrying exactly the referenced frame.
func TestUplinkRedundancyContinuousAudio(t *testing.T) {
	r := NewUplinkRedundancy(3)
	buf := make([]byte, 0, 32) // reused like a real encoder buffer
	for seq := uint64(0); seq < 20; seq++ {
		buf = append(buf[:0], audioOf(seq)...)
		pkt := wire.MonoObjectPacket{Audio: buf}
		r.Attach(seq, &pkt)
		out := roundTrip(t, &pkt)
		if seq < 3 {
			if out.Redundancy != nil {
				t.Fatalf("seq %d: redundancy attached below the offset", seq)
			}
			continue
		}
		if out.Redundancy == nil {
			t.Fatalf("seq %d: no redundancy", seq)
		}
		if out.Redundancy.Offset != 3 {
			t.Fatalf("seq %d: offset %d, want 3", seq, out.Redundancy.Offset)
		}
		if !bytes.Equal(out.Redundancy.Audio, audioOf(seq-3)) {
			t.Fatalf("seq %d: copy %q, want %q (the caller's buffer was reused — ring must copy)", seq, out.Redundancy.Audio, audioOf(seq-3))
		}
	}
}

// TestUplinkRedundancyPoseOnlyReference: a pose-only packet advances seq
// but carries no audio, so the packet offset later omits the record
// rather than referencing it (§5.1: the referenced packet MUST be
// audio-bearing) — and never substitutes another offset.
func TestUplinkRedundancyPoseOnlyReference(t *testing.T) {
	r := NewUplinkRedundancy(2)
	pose := wire.Pose{X: 1}
	send := func(seq uint64, audio bool) *wire.MonoObjectPacket {
		pkt := wire.MonoObjectPacket{Pose: &pose}
		if audio {
			pkt.Audio = audioOf(seq)
		}
		r.Attach(seq, &pkt)
		return roundTrip(t, &pkt)
	}
	send(0, true)
	send(1, true)
	if out := send(2, false); out.Redundancy != nil || out.Audio != nil {
		t.Fatal("pose-only packet must carry neither audio nor redundancy")
	}
	if out := send(3, true); out.Redundancy == nil || !bytes.Equal(out.Redundancy.Audio, audioOf(1)) {
		t.Fatalf("seq 3 should repeat seq 1: %+v", out.Redundancy)
	}
	if out := send(4, true); out.Redundancy != nil {
		t.Fatalf("seq 4 references pose-only seq 2: must omit, got offset %d", out.Redundancy.Offset)
	}
	if out := send(5, true); out.Redundancy == nil || !bytes.Equal(out.Redundancy.Audio, audioOf(3)) {
		t.Fatalf("seq 5 should repeat seq 3: %+v", out.Redundancy)
	}
}

// TestUplinkRedundancyOffsetZeroPassesThrough: at offset 0 (and on a
// nil emitter) the packet is untouched, including a caller-supplied
// redundancy record.
func TestUplinkRedundancyOffsetZeroPassesThrough(t *testing.T) {
	own := &wire.Redundancy{Offset: 1, Audio: []byte("mine")}
	for _, r := range []*UplinkRedundancy{nil, NewUplinkRedundancy(0)} {
		pkt := wire.MonoObjectPacket{Audio: []byte("a"), Redundancy: own}
		r.Attach(5, &pkt)
		if pkt.Redundancy != own {
			t.Fatal("offset 0 must not touch the packet")
		}
		if r.Offset() != 0 {
			t.Fatal("offset should read 0")
		}
	}
}

// TestUplinkRedundancyStaleSlot: a seq jump (a caller restarting its
// counter, say) must not pair a packet with whatever an old lap left
// in the ring — the slot is keyed by seq, not by position.
func TestUplinkRedundancyStaleSlot(t *testing.T) {
	r := NewUplinkRedundancy(1)
	for seq := uint64(0); seq < 8; seq++ {
		pkt := wire.MonoObjectPacket{Audio: audioOf(seq)}
		r.Attach(seq, &pkt)
	}
	pkt := wire.MonoObjectPacket{Audio: audioOf(100)}
	r.Attach(100, &pkt) // slot 4 holds seq 4; 99 was never seen
	if pkt.Redundancy != nil {
		t.Fatalf("attached a copy of seq %q for a reference that never existed", pkt.Redundancy.Audio)
	}
	pkt = wire.MonoObjectPacket{Audio: audioOf(101)}
	r.Attach(101, &pkt)
	if pkt.Redundancy == nil || !bytes.Equal(pkt.Redundancy.Audio, audioOf(100)) {
		t.Fatal("seq 101 should repeat seq 100")
	}
}

func TestUplinkRedundancyClamp(t *testing.T) {
	if got := NewUplinkRedundancy(9).Offset(); got != 7 {
		t.Fatalf("offset 9 clamps to 7, got %d", got)
	}
	if got := NewUplinkRedundancy(-1).Offset(); got != 0 {
		t.Fatalf("offset -1 clamps to 0, got %d", got)
	}
}
