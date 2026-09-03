package server

// The GroupDepacketizer — the lockstep receiver (lasa-core.md §4.2,
// §5.2; plan/lockstep.md §4.4). A lockstep connection's N entity
// source tracks share one frame counter, so the receiver gathers frame
// k across all N tracks and releases it as one N-channel event. It is
// the single-track Depacketizer's ring with N slots per seq: the same
// no-clock release rule, the same redundancy substitution per channel,
// the same conceal cap per channel, and one addition, any-track
// evidence: frame k releases when every member has delivered it, or
// when the set's highest seen seq reaches k+8, whichever comes first.
// A dead member therefore costs per-channel concealment and then
// silence, and never stalls its siblings.
//
// Concurrency: every member handler is driven by the one receive
// goroutine of the member's connection, and members are on one
// connection by rule, so the ring is single-writer and takes no lock.
// Stats may be read from any goroutine.

import (
	"sync/atomic"

	"github.com/panaudia/lasa/wire"
)

// GroupReceiver consumes a GroupDepacketizer's ordered output: one call
// per released frame, chans indexed by member. Pose and Audio alias
// slot storage the next release reuses: consume synchronously.
type GroupReceiver interface {
	Frame(seq uint64, chans []GroupChannel)
}

// GroupChannel is one member's share of one released frame.
type GroupChannel struct {
	// Present is true when the member's packet (or a redundant copy
	// of it) arrived. Pose and Audio may each still be absent.
	Present   bool
	Recovered bool
	Pose      *wire.Pose
	Audio     []byte
	// When Present is false the member's frame is provably lost.
	// AudioLikely is the single-track hint (the member's last
	// delivered packet carried audio). Silence says the member's
	// conceal run has passed the cap: fill with silence, not PLC.
	AudioLikely bool
	Silence     bool
}

type groupHeld struct {
	valid     bool
	recovered bool
	hasPose   bool
	pose      wire.Pose
	audio     []byte // slot-owned storage, reused across laps
}

type groupSlot struct {
	seq     uint64
	present int // valid channels
	ch      []groupHeld
}

// GroupDepacketizer gathers one lockstep set.
type GroupDepacketizer struct {
	r       GroupReceiver
	n       int
	scratch wire.MonoObjectScratch
	pkt     wire.MonoObjectPacket

	started      bool
	nextSeq      uint64
	highest      uint64
	pending      [depacketizerWindow]groupSlot
	pendingCount int // slots holding at least one channel
	groupRun     int // whole frames lost in a row, for the skip rule
	gapRun       int
	concealRun   []int
	lastHadAudio []bool
	chans        []GroupChannel
	members      []groupMember

	// maxSpread is the widest arrival spread seen: how many frames an
	// arriving packet trailed the set's newest seq at that moment. In
	// step it reads 0, and a member d frames behind reads d. It is the
	// evidence behind the any-track release rule, which conceals the
	// trailing member once the spread reaches 8 (plan/lockstep.md §4.4).
	maxSpread atomic.Uint64

	c  depacketizerCounters   // frame level
	mc []depacketizerCounters // per member
}

// NewGroupDepacketizer gathers n member tracks into r.
func NewGroupDepacketizer(n int, r GroupReceiver) *GroupDepacketizer {
	g := &GroupDepacketizer{
		r: r, n: n,
		concealRun:   make([]int, n),
		lastHadAudio: make([]bool, n),
		chans:        make([]GroupChannel, n),
		members:      make([]groupMember, n),
		mc:           make([]depacketizerCounters, n),
	}
	for i := range g.pending {
		g.pending[i].ch = make([]groupHeld, n)
	}
	for i := range g.members {
		g.members[i] = groupMember{g: g, i: i}
	}
	return g
}

// Member returns member i's EntityHandler: the thin alias the shell
// hands each entity's ingress to.
func (g *GroupDepacketizer) Member(i int) EntityHandler { return &g.members[i] }

type groupMember struct {
	g *GroupDepacketizer
	i int
}

func (m *groupMember) WritePacket(seq uint64, payload []byte) { m.g.write(m.i, seq, payload) }

// Stats is the frame-level view: Delivered counts released frames with
// at least one member present, Lost frames released with no member
// present (concealed on every channel), Skipped frames dropped past
// the conceal cap, plus the gap histogram over whole-frame losses.
func (g *GroupDepacketizer) Stats() DepacketizerStats { return statsOf(&g.c) }

// MemberStats is member i's view in the single-track vocabulary:
// Delivered, Recovered, Lost (concealed), Skipped (silenced past the
// cap), Duplicates, Late and Malformed.
func (g *GroupDepacketizer) MemberStats(i int) DepacketizerStats { return statsOf(&g.mc[i]) }

// MaxSpread is the widest arrival spread seen, in frames: the most an
// arriving packet has trailed the set's newest seq. 0 means every
// member's packet for a frame arrived before any member's next frame.
// The release rule fires at 8. Safe from any goroutine.
func (g *GroupDepacketizer) MaxSpread() uint64 { return g.maxSpread.Load() }

func (g *GroupDepacketizer) noteSpread(seq uint64) {
	if s := g.highest - seq; s > g.maxSpread.Load() {
		g.maxSpread.Store(s)
	}
}

func statsOf(c *depacketizerCounters) DepacketizerStats {
	s := DepacketizerStats{
		Delivered:  c.delivered.Load(),
		Recovered:  c.recovered.Load(),
		Lost:       c.lost.Load(),
		Skipped:    c.skipped.Load(),
		Duplicates: c.duplicates.Load(),
		Late:       c.late.Load(),
		Malformed:  c.malformed.Load(),
		GapEvents:  c.gaps.Load(),
	}
	for i := range s.GapHist {
		s.GapHist[i] = c.gapHist[i].Load()
	}
	return s
}

func (g *GroupDepacketizer) write(i int, seq uint64, payload []byte) {
	if err := wire.ParseMonoObjectInto(payload, &g.scratch, &g.pkt); err != nil {
		g.mc[i].malformed.Add(1)
		return
	}
	if !g.started {
		g.started = true
		g.nextSeq = seq
		g.highest = seq
	}
	if seq > g.highest {
		g.highest = seq
	}
	g.noteSpread(seq)
	if g.pendingCount > 0 || g.highest >= g.nextSeq+8 {
		g.release()
	}
	if seq < g.nextSeq {
		g.mc[i].late.Add(1)
		return
	}
	if g.pendingCount == 0 && g.gapRun == 0 && seq > g.nextSeq {
		g.c.gaps.Add(1)
	}
	// Redundancy substitution, per channel: this packet may carry the
	// copy of member i's missing frame (the §5.1 sliding scheme).
	if r := g.pkt.Redundancy; r != nil {
		m := seq - uint64(r.Offset)
		if m >= g.nextSeq {
			if s := g.slotFor(m); !s.ch[i].valid {
				g.store(s, i, groupHeld{valid: true, recovered: true, audio: r.Audio}, nil)
			}
		}
	}
	s := g.slotFor(seq)
	if s.ch[i].valid {
		g.mc[i].duplicates.Add(1)
	} else {
		g.store(s, i, groupHeld{valid: true, audio: g.pkt.Audio}, g.pkt.Pose)
	}
	g.release()
}

// slotFor returns seq's slot, claiming it when it holds an older seq.
// release keeps the live window at most 8 wide, so a claimed slot is
// always resolved.
func (g *GroupDepacketizer) slotFor(seq uint64) *groupSlot {
	s := &g.pending[seq%depacketizerWindow]
	if s.present == 0 {
		s.seq = seq
	}
	return s
}

func (g *GroupDepacketizer) store(s *groupSlot, i int, h groupHeld, pose *wire.Pose) {
	if s.present == 0 {
		g.pendingCount++
	}
	dst := &s.ch[i]
	if !dst.valid {
		s.present++
	}
	buf := dst.audio[:0]
	dst.valid, dst.recovered = h.valid, h.recovered
	dst.audio = append(buf, h.audio...)
	if pose != nil {
		dst.hasPose, dst.pose = true, *pose
	} else {
		dst.hasPose = false
	}
}

// release advances nextSeq as far as evidence allows: a frame every
// member delivered, or, once the set's highest seq is 8 past it, a
// frame delivered with its absent members declared lost.
func (g *GroupDepacketizer) release() {
	for {
		s := &g.pending[g.nextSeq%depacketizerWindow]
		full := s.present > 0 && s.seq == g.nextSeq
		switch {
		case full && s.present == g.n:
			g.deliver(s)
		case g.highest >= g.nextSeq+8:
			if full {
				g.deliver(s)
				continue
			}
			// Nobody delivered this frame and no copy can arrive.
			if g.groupRun < maxConcealRun {
				g.deliverAbsent()
			} else {
				g.c.skipped.Add(1)
				g.gapRun++
				g.nextSeq++
			}
		default:
			return
		}
	}
}

func (g *GroupDepacketizer) deliver(s *groupSlot) {
	g.closeGapRun()
	for i := range g.chans {
		c := &g.chans[i]
		h := &s.ch[i]
		if h.valid {
			c.Present, c.Recovered = true, h.recovered
			c.Pose = nil
			if h.hasPose {
				c.Pose = &h.pose
			}
			c.Audio = h.audio
			c.AudioLikely, c.Silence = false, false
			g.lastHadAudio[i] = len(h.audio) > 0
			g.concealRun[i] = 0
			g.mc[i].delivered.Add(1)
			if h.recovered {
				g.mc[i].recovered.Add(1)
			}
		} else {
			g.absent(i, c)
		}
	}
	g.c.delivered.Add(1)
	g.groupRun = 0
	g.r.Frame(g.nextSeq, g.chans)
	for i := range s.ch {
		s.ch[i].valid = false
	}
	s.present = 0
	g.pendingCount--
	g.nextSeq++
}

// deliverAbsent releases a frame no member delivered: every channel
// lost, concealed or silenced by its own run.
func (g *GroupDepacketizer) deliverAbsent() {
	for i := range g.chans {
		g.absent(i, &g.chans[i])
	}
	g.c.lost.Add(1)
	g.groupRun++
	g.gapRun++
	g.r.Frame(g.nextSeq, g.chans)
	g.nextSeq++
}

func (g *GroupDepacketizer) absent(i int, c *GroupChannel) {
	*c = GroupChannel{AudioLikely: g.lastHadAudio[i]}
	if g.concealRun[i] < maxConcealRun {
		g.mc[i].lost.Add(1)
	} else {
		c.Silence = true
		g.mc[i].skipped.Add(1)
	}
	g.concealRun[i]++
}

func (g *GroupDepacketizer) closeGapRun() {
	if g.gapRun == 0 {
		return
	}
	b := g.gapRun - 1
	if b > 7 {
		b = 7
	}
	g.c.gapHist[b].Add(1)
	g.gapRun = 0
}
