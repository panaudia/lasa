package server

import (
	"testing"

	"github.com/panaudia/lasa/wire"
)

// groupRecorder captures released frames as (seq, per-member state).
type groupFrame struct {
	seq   uint64
	chans []GroupChannel
	audio [][]byte
}

type groupRecorder struct{ frames []groupFrame }

func (r *groupRecorder) Frame(seq uint64, chans []GroupChannel) {
	f := groupFrame{seq: seq, chans: append([]GroupChannel(nil), chans...)}
	for _, c := range chans {
		f.audio = append(f.audio, append([]byte(nil), c.Audio...))
	}
	r.frames = append(r.frames, f)
}

// memberAudio gives each (member, seq) distinct bytes.
func memberAudio(i int, seq uint64) []byte {
	return []byte{0xB0 + byte(i), byte(seq), byte(seq >> 8), 0x0F}
}

func mkm(t *testing.T, i int, seq uint64, withAudio bool, offset uint8) []byte {
	t.Helper()
	p := wire.MonoObjectPacket{Pose: &wire.Pose{X: float32(seq), Y: float32(i)}}
	if withAudio {
		p.Audio = memberAudio(i, seq)
	}
	if offset > 0 {
		p.Redundancy = &wire.Redundancy{Offset: offset, Audio: memberAudio(i, seq-uint64(offset))}
	}
	buf, err := wire.AppendMonoObject(nil, &p)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func seqs(frames []groupFrame) []uint64 {
	out := make([]uint64, len(frames))
	for i, f := range frames {
		out[i] = f.seq
	}
	return out
}

func wantSeqs(t *testing.T, frames []groupFrame, want ...uint64) {
	t.Helper()
	got := seqs(frames)
	if len(got) != len(want) {
		t.Fatalf("released %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("released %v, want %v", got, want)
		}
	}
}

// In step: both members in order releases one frame per seq with both
// present, each carrying its own audio and pose.
func TestGroupInStep(t *testing.T) {
	r := &groupRecorder{}
	g := NewGroupDepacketizer(2, r)
	a, b := g.Member(0), g.Member(1)
	for seq := uint64(0); seq < 5; seq++ {
		a.WritePacket(seq, mkm(t, 0, seq, true, 0))
		if len(r.frames) != int(seq) {
			t.Fatalf("seq %d released before member 1 arrived", seq)
		}
		b.WritePacket(seq, mkm(t, 1, seq, true, 0))
	}
	wantSeqs(t, r.frames, 0, 1, 2, 3, 4)
	for _, f := range r.frames {
		for i := 0; i < 2; i++ {
			if !f.chans[i].Present || string(f.audio[i]) != string(memberAudio(i, f.seq)) || f.chans[i].Pose == nil || f.chans[i].Pose.Y != float32(i) {
				t.Fatalf("seq %d member %d: %+v audio %x", f.seq, i, f.chans[i], f.audio[i])
			}
		}
	}
	if s := g.Stats(); s.Delivered != 5 || s.Lost != 0 || g.MemberStats(1).Delivered != 5 {
		t.Fatalf("stats %+v member1 %+v", s, g.MemberStats(1))
	}
}

// One member's packet lost, recovered from its own next packet's copy:
// the frame waits for the copy, then releases with the member present
// and recovered.
func TestGroupPerChannelRecovery(t *testing.T) {
	r := &groupRecorder{}
	g := NewGroupDepacketizer(2, r)
	a, b := g.Member(0), g.Member(1)
	a.WritePacket(0, mkm(t, 0, 0, true, 0))
	b.WritePacket(0, mkm(t, 1, 0, true, 0))
	a.WritePacket(1, mkm(t, 0, 1, true, 0))
	// member 1 loses seq 1; its seq 2 carries the copy at offset 1
	b.WritePacket(2, mkm(t, 1, 2, true, 1))
	a.WritePacket(2, mkm(t, 0, 2, true, 0))
	wantSeqs(t, r.frames, 0, 1, 2)
	f := r.frames[1]
	if !f.chans[1].Present || !f.chans[1].Recovered || string(f.audio[1]) != string(memberAudio(1, 1)) {
		t.Fatalf("seq 1 member 1 not recovered: %+v %x", f.chans[1], f.audio[1])
	}
	if f.chans[1].Pose != nil {
		t.Fatal("a recovered copy carries no pose")
	}
	if g.MemberStats(1).Recovered != 1 {
		t.Fatalf("member 1 recovered = %d", g.MemberStats(1).Recovered)
	}
}

// Any-track evidence: a member that stops sending does not stall the
// set. Once the set's highest seq is 8 past a frame, the frame
// releases with that member lost and the live member present.
func TestGroupDeadMemberReleases(t *testing.T) {
	r := &groupRecorder{}
	g := NewGroupDepacketizer(2, r)
	a, b := g.Member(0), g.Member(1)
	a.WritePacket(0, mkm(t, 0, 0, true, 0))
	b.WritePacket(0, mkm(t, 1, 0, true, 0))
	for seq := uint64(1); seq <= 8; seq++ {
		a.WritePacket(seq, mkm(t, 0, seq, true, 0))
	}
	// seq 1..8 pending on member 1; at highest 9, seq 1 is provably
	// unrecoverable for member 1 and releases.
	wantSeqs(t, r.frames, 0)
	a.WritePacket(9, mkm(t, 0, 9, true, 0))
	wantSeqs(t, r.frames, 0, 1)
	f := r.frames[1]
	if !f.chans[0].Present || f.chans[1].Present || !f.chans[1].AudioLikely || f.chans[1].Silence {
		t.Fatalf("seq 1: %+v", f.chans)
	}
	// Past the conceal cap the dead member is silenced, not concealed.
	for seq := uint64(10); seq < 40; seq++ {
		a.WritePacket(seq, mkm(t, 0, seq, true, 0))
	}
	last := r.frames[len(r.frames)-1]
	if !last.chans[1].Silence {
		t.Fatalf("dead member still concealed at seq %d: %+v", last.seq, last.chans[1])
	}
	ms := g.MemberStats(1)
	if ms.Lost != maxConcealRun || ms.Skipped == 0 {
		t.Fatalf("member 1 lost %d skipped %d, want lost %d then silence", ms.Lost, ms.Skipped, maxConcealRun)
	}
	if s := g.Stats(); s.Lost != 0 || s.Skipped != 0 {
		t.Fatalf("frames with a live member must not count as whole-frame losses: %+v", s)
	}
}

// A whole frame lost on every member is released as an all-lost frame
// until the cap, then skipped.
func TestGroupWholeFrameLoss(t *testing.T) {
	r := &groupRecorder{}
	g := NewGroupDepacketizer(2, r)
	a, b := g.Member(0), g.Member(1)
	a.WritePacket(0, mkm(t, 0, 0, true, 0))
	b.WritePacket(0, mkm(t, 1, 0, true, 0))
	// both members skip seq 1 and continue from 2
	for seq := uint64(2); seq <= 9; seq++ {
		a.WritePacket(seq, mkm(t, 0, seq, true, 0))
		b.WritePacket(seq, mkm(t, 1, seq, true, 0))
	}
	wantSeqs(t, r.frames, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	f := r.frames[1]
	if f.chans[0].Present || f.chans[1].Present || !f.chans[0].AudioLikely {
		t.Fatalf("seq 1: %+v", f.chans)
	}
	if s := g.Stats(); s.Lost != 1 || s.Delivered != 9 || s.GapEvents != 1 {
		t.Fatalf("stats %+v", s)
	}
}

// Reorder within a member, late packets, duplicates and a malformed
// packet are counted per member and never disturb the release order.
func TestGroupReorderLateDuplicateMalformed(t *testing.T) {
	r := &groupRecorder{}
	g := NewGroupDepacketizer(2, r)
	a, b := g.Member(0), g.Member(1)
	b.WritePacket(0, mkm(t, 1, 0, true, 0))
	a.WritePacket(1, mkm(t, 0, 1, true, 0))
	a.WritePacket(0, mkm(t, 0, 0, true, 0)) // reordered within member 0
	b.WritePacket(1, mkm(t, 1, 1, true, 0))
	wantSeqs(t, r.frames, 0, 1)
	a.WritePacket(0, mkm(t, 0, 0, true, 0)) // late
	b.WritePacket(2, mkm(t, 1, 2, true, 0))
	b.WritePacket(2, mkm(t, 1, 2, true, 0)) // duplicate while pending
	a.WritePacket(2, []byte{0xFF, 1, 2})    // malformed
	a.WritePacket(2, mkm(t, 0, 2, true, 0))
	wantSeqs(t, r.frames, 0, 1, 2)
	if s := g.MemberStats(0); s.Late != 1 || s.Malformed != 1 {
		t.Fatalf("member 0 %+v", s)
	}
	if s := g.MemberStats(1); s.Duplicates != 1 {
		t.Fatalf("member 1 %+v", s)
	}
}

// A member that joins late (its first packets arrive after its
// siblings') is lost for the frames it missed and present after.
func TestGroupLateJoiner(t *testing.T) {
	r := &groupRecorder{}
	g := NewGroupDepacketizer(3, r)
	for seq := uint64(0); seq < 12; seq++ {
		g.Member(0).WritePacket(seq, mkm(t, 0, seq, true, 0))
		g.Member(1).WritePacket(seq, mkm(t, 1, seq, true, 0))
		if seq >= 3 {
			g.Member(2).WritePacket(seq, mkm(t, 2, seq, true, 0))
		}
	}
	if len(r.frames) < 4 {
		t.Fatalf("released only %v", seqs(r.frames))
	}
	if r.frames[0].chans[2].Present || r.frames[0].chans[2].AudioLikely {
		t.Fatalf("seq 0 member 2: %+v, want lost with no audio hint", r.frames[0].chans[2])
	}
	if !r.frames[3].chans[2].Present {
		t.Fatalf("seq 3 member 2 should be present: %+v", r.frames[3].chans[2])
	}
}

// TestGroupReleaseRuleAtEverySpread pins the any-track release rule's
// boundary: member B runs d frames behind member A for every d from 0
// to 12. Below 8 the frame waits for B and releases complete, with no
// conceal and no late packet, so a healthy but slower channel costs
// nothing. From 8 up the rule fires: B is concealed on every frame
// and its real packets arrive late. The spread statistic reports d.
func TestGroupReleaseRuleAtEverySpread(t *testing.T) {
	const frames = 60
	for d := uint64(0); d <= 12; d++ {
		r := &groupRecorder{}
		g := NewGroupDepacketizer(2, r)
		a, b := g.Member(0), g.Member(1)
		for s := uint64(0); s < frames; s++ {
			a.WritePacket(s, mkm(t, 0, s, true, 0))
			if s >= d {
				b.WritePacket(s-d, mkm(t, 1, s-d, true, 0))
			}
		}
		mb := g.MemberStats(1)
		bothPresent := 0
		for _, f := range r.frames {
			if f.chans[0].Present && f.chans[1].Present {
				bothPresent++
			}
		}
		if g.MaxSpread() != d {
			t.Fatalf("d=%d: MaxSpread %d", d, g.MaxSpread())
		}
		if d < 8 {
			if mb.Lost != 0 || mb.Late != 0 || bothPresent != len(r.frames) || len(r.frames) < frames-int(d)-1 {
				t.Fatalf("d=%d: a healthy lag inside the window was penalised: lost %d late %d, %d of %d frames complete",
					d, mb.Lost, mb.Late, bothPresent, len(r.frames))
			}
			continue
		}
		if mb.Lost == 0 || mb.Late == 0 || bothPresent != 0 {
			t.Fatalf("d=%d: the rule did not fire: lost %d late %d, %d frames complete", d, mb.Lost, mb.Late, bothPresent)
		}
		if ma := g.MemberStats(0); ma.Lost != 0 || ma.Late != 0 {
			t.Fatalf("d=%d: the leading member was penalised: %+v", d, ma)
		}
	}
}
