package client

import "github.com/panaudia/lasa/wire"

// UplinkRedundancy is the §5.1 repeat emitter for a mono-object source
// track (lasa-core.md §5.1): with offset n, every audio-bearing packet
// also carries a full copy of the audio n packets earlier, so a burst
// of up to n lost packets is repaired exactly at the server's ingest.
// EntityPublisher runs one per entity at the entity's declared offset
// (§4.2); it is exported for Go clients that frame packets themselves
// and send with WritePacket.
//
// Policy is always-on at the offset (the sink writer's mirror image):
// the declaration is both the request and the bound, and any
// adaptation within it is profile vocabulary, not this type.
//
// Unlike a sink track, an uplink track carries pose-only packets,
// which advance seq without audio. The referenced packet MUST be
// audio-bearing, so the ring remembers which seqs carried audio and
// omits the record when seq − offset was pose-only — never
// substituting a different offset, since the offset is what locates
// the copy.
type UplinkRedundancy struct {
	offset uint8
	// hist holds the previous packets' audio keyed by seq — 8 slots so
	// slot (seq−7)%8 is never the slot being written at seq. Copies,
	// because callers reuse their Opus buffer across frames; the slices
	// are reused across laps (amortized zero allocation).
	hist [8]histSlot
	red  wire.Redundancy
}

type histSlot struct {
	seq   uint64
	valid bool
	audio []byte // empty = the packet at seq was pose-only
}

// NewUplinkRedundancy returns an emitter at offset (0 = off). Offsets
// above 7, the wire maximum, are clamped to 7; negative offsets are 0.
func NewUplinkRedundancy(offset int) *UplinkRedundancy {
	switch {
	case offset < 0:
		offset = 0
	case offset > 7:
		offset = 7
	}
	return &UplinkRedundancy{offset: uint8(offset)}
}

// Offset is the emitter's redundancy offset (0 = off).
func (r *UplinkRedundancy) Offset() uint8 {
	if r == nil {
		return 0
	}
	return r.offset
}

// Attach records pkt as the packet at seq and, when the packet seq −
// Offset ago was audio-bearing and this one is too, sets pkt.Redundancy
// to a copy of that packet's audio at this offset. Any Redundancy the
// caller set is replaced. With Offset 0 (or a nil receiver) pkt is left
// untouched. The attached copy aliases the ring and is valid until the
// next Attach; encode before calling again. Not safe for concurrent
// use — EntityPublisher serialises its calls.
func (r *UplinkRedundancy) Attach(seq uint64, pkt *wire.MonoObjectPacket) {
	if r == nil || r.offset == 0 {
		return
	}
	pkt.Redundancy = nil
	slot := &r.hist[seq%uint64(len(r.hist))]
	slot.seq, slot.valid = seq, true
	slot.audio = append(slot.audio[:0], pkt.Audio...)
	if len(pkt.Audio) == 0 || seq < uint64(r.offset) {
		return
	}
	refSeq := seq - uint64(r.offset)
	ref := &r.hist[refSeq%uint64(len(r.hist))]
	if !ref.valid || ref.seq != refSeq || len(ref.audio) == 0 {
		return
	}
	r.red = wire.Redundancy{Offset: r.offset, Audio: ref.audio}
	pkt.Redundancy = &r.red
}
