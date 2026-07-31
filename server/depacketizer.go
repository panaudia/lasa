package server

// The Depacketizer — the §5.1/§5.2 receiver state machine as a shared
// utility (design: plan/panaudia-server-design.md §3; it is the
// "decoded-handler convenience wrapper" the SPI doc anticipated, grown
// to its full responsibilities). It wraps a Receiver and implements
// EntityHandler: raw datagram payloads in; parsed frames and loss
// declarations out, in strict stream order — the order a stateful
// decoder requires.
//
// Responsibilities: parse + flag validation/discard, seq tracking,
// reorder, hold-on-gap, redundancy substitution (the sliding one-copy-
// at-lag-k scheme covers contiguous bursts ≤ k), and provable-loss
// declaration. NO CLOCK anywhere (design §1): release is arrival-
// driven — frame n is recoverable only from packets n+1…n+7 (offset
// ≤ 7), so the arrival of packet n+8 without a covering copy is proof
// of unrecoverability. A total stall needs no timeout (nothing is
// playing anyway); a resume flushes.
//
// The clean-network path holds zero state and allocates nothing: an
// in-order packet is parsed into reused scratch and delivered in the
// same call. Gap-path state lives in a fixed 16-slot ring (the release
// rule bounds the live window to 8 seqs).

import (
	"sync/atomic"

	"github.com/panaudia/lasa/wire"
)

// Receiver consumes the Depacketizer's ordered output.
//
// Frame delivers one packet's content in stream order; pose and audio
// may each be absent (nil/empty — pose-only and audio-only packets).
// Both alias transient storage (the datagram buffer / the reused
// scratch): consume synchronously, before returning.
//
// Lost declares seq provably undeliverable, in stream order — the
// receiver typically conceals (decoder PLC) to keep decoder state and
// sample accounting aligned. audioLikely is a heuristic hint (the
// nearest delivered packet before the gap carried audio): a receiver
// may skip concealment for likely pose-only losses, since concealing
// those would inject samples the sender never produced.
type Receiver interface {
	Frame(seq uint64, pose *wire.Pose, audio []byte)
	Lost(seq uint64, audioLikely bool)
}

// maxConcealRun caps consecutive Lost declarations per gap event
// (~120 ms — libopus PLC's useful horizon). Beyond it the stream skips
// silently instead: after a long stall, pumping seconds of concealment
// into the jitter buffer would rebuild it as delay; a time-slip at
// discontinuity scale is the lesser harm (counted in Skipped).
const maxConcealRun = 24

// depacketizerWindow is the pending ring size. The release rule keeps
// the live seq window ≤ 8 wide; 16 gives slack and a power-of-two
// index.
const depacketizerWindow = 16

type held struct {
	valid     bool
	recovered bool
	seq       uint64
	hasPose   bool
	pose      wire.Pose
	audio     []byte
}

// DepacketizerStats is the diagnostic surface — the evidence base for
// any future redundancy-offset tuning. Snapshot with Stats().
type DepacketizerStats struct {
	Delivered  uint64 // frames delivered (including recovered)
	Recovered  uint64 // of Delivered: from a redundant copy
	Lost       uint64 // declared lost (concealable)
	Skipped    uint64 // lost beyond the conceal cap (time-slip)
	Duplicates uint64 // same seq seen again while pending/fresh
	Late       uint64 // arrived after their slot was resolved
	Malformed  uint64 // discarded at parse (§5.1 rules)
	GapEvents  uint64 // in-order → gap transitions
	// GapHist buckets contiguous loss-run lengths 1..7 and 8+ (index
	// 0..7) at gap close.
	GapHist [8]uint64
}

type depacketizerCounters struct {
	delivered, recovered, lost, skipped atomic.Uint64
	duplicates, late, malformed, gaps   atomic.Uint64
	gapHist                             [8]atomic.Uint64
}

// Depacketizer wraps one entity's ingress. Driven by the transport's
// receive goroutine (WritePacket, per the EntityHandler contract);
// Stats may be read from any goroutine.
type Depacketizer struct {
	r       Receiver
	scratch wire.MonoObjectScratch
	pkt     wire.MonoObjectPacket

	started      bool
	nextSeq      uint64 // first undelivered seq
	highest      uint64 // highest seq ever seen
	pending      [depacketizerWindow]held
	pendingCount int
	concealRun   int
	gapRun       int // contiguous Lost+Skipped, for GapHist
	lastHadAudio bool

	c depacketizerCounters
}

// NewDepacketizer wraps r. The returned value implements EntityHandler.
func NewDepacketizer(r Receiver) *Depacketizer {
	return &Depacketizer{r: r}
}

var _ EntityHandler = (*Depacketizer)(nil)

func (d *Depacketizer) Stats() DepacketizerStats {
	s := DepacketizerStats{
		Delivered:  d.c.delivered.Load(),
		Recovered:  d.c.recovered.Load(),
		Lost:       d.c.lost.Load(),
		Skipped:    d.c.skipped.Load(),
		Duplicates: d.c.duplicates.Load(),
		Late:       d.c.late.Load(),
		Malformed:  d.c.malformed.Load(),
		GapEvents:  d.c.gaps.Load(),
	}
	for i := range s.GapHist {
		s.GapHist[i] = d.c.gapHist[i].Load()
	}
	return s
}

// WritePacket implements EntityHandler: raw payload in, ordered
// Frame/Lost out, synchronously on this goroutine.
func (d *Depacketizer) WritePacket(seq uint64, payload []byte) {
	if err := wire.ParseMonoObjectInto(payload, &d.scratch, &d.pkt); err != nil {
		d.c.malformed.Add(1)
		return
	}

	if !d.started {
		d.started = true
		d.nextSeq = seq
		d.highest = seq
	}
	if seq > d.highest {
		d.highest = seq
	}

	// Normalize before storing: this arrival's seq is itself evidence
	// that may resolve old slots (the jump case), and running the
	// release first guarantees the live window is ≤ 8 seqs wide when
	// the ring is written — no cross-seq slot collisions, ever.
	if d.pendingCount > 0 || d.highest >= d.nextSeq+8 {
		d.release()
	}

	switch {
	case seq < d.nextSeq:
		d.c.late.Add(1)
		return
	case seq == d.nextSeq && d.pendingCount == 0:
		// Clean-network fast path: zero held state, straight through.
		d.deliverParsed(seq)
		return
	}

	if seq == d.nextSeq {
		d.deliverParsed(seq)
	} else {
		if d.pendingCount == 0 && d.gapRun == 0 {
			d.c.gaps.Add(1)
		}
		// Redundancy substitution: this packet may carry the copy of a
		// missing frame (the §5.1 sliding scheme).
		if r := d.pkt.Redundancy; r != nil {
			m := seq - uint64(r.Offset)
			if m >= d.nextSeq && !d.slotValid(m) {
				d.store(m, held{
					valid: true, recovered: true, seq: m, audio: r.Audio,
				})
			}
		}
		if d.slotValid(seq) {
			d.c.duplicates.Add(1)
		} else {
			h := held{valid: true, seq: seq, audio: d.pkt.Audio}
			if d.pkt.Pose != nil {
				h.hasPose = true
				h.pose = *d.pkt.Pose
			}
			d.store(seq, h)
		}
	}
	d.release()
}

// deliverParsed emits the just-parsed packet without copying it into
// the ring (fast path and the in-order head of a drain).
func (d *Depacketizer) deliverParsed(seq uint64) {
	d.closeGapRun()
	d.r.Frame(seq, d.pkt.Pose, d.pkt.Audio)
	d.lastHadAudio = len(d.pkt.Audio) > 0
	d.concealRun = 0
	d.nextSeq = seq + 1
	d.c.delivered.Add(1)
}

func (d *Depacketizer) deliverHeld(h *held) {
	d.closeGapRun()
	var pose *wire.Pose
	if h.hasPose {
		pose = &h.pose
	}
	d.r.Frame(h.seq, pose, h.audio)
	d.lastHadAudio = len(h.audio) > 0
	d.concealRun = 0
	d.nextSeq = h.seq + 1
	d.c.delivered.Add(1)
	if h.recovered {
		d.c.recovered.Add(1)
	}
	h.valid = false
	d.pendingCount--
}

// release advances nextSeq as far as evidence allows: deliver pending
// frames in order; declare provably-unrecoverable seqs Lost (or skip
// past them beyond the conceal cap).
func (d *Depacketizer) release() {
	for {
		if h := d.slot(d.nextSeq); h.valid && h.seq == d.nextSeq {
			d.deliverHeld(h)
			continue
		}
		if d.highest >= d.nextSeq+8 {
			// No copy of nextSeq can ever arrive (offset ≤ 7).
			if d.concealRun < maxConcealRun {
				d.r.Lost(d.nextSeq, d.lastHadAudio)
				d.c.lost.Add(1)
				d.concealRun++
			} else {
				d.c.skipped.Add(1)
			}
			d.gapRun++
			d.nextSeq++
			continue
		}
		return
	}
}

func (d *Depacketizer) closeGapRun() {
	if d.gapRun == 0 {
		return
	}
	b := d.gapRun - 1
	if b > 7 {
		b = 7
	}
	d.c.gapHist[b].Add(1)
	d.gapRun = 0
}

func (d *Depacketizer) slot(seq uint64) *held { return &d.pending[seq%depacketizerWindow] }

func (d *Depacketizer) slotValid(seq uint64) bool {
	h := d.slot(seq)
	return h.valid && h.seq == seq
}

func (d *Depacketizer) store(seq uint64, h held) {
	s := d.slot(seq)
	if !s.valid {
		d.pendingCount++
	}
	*s = h
}
