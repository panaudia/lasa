package wire

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/panaudia/lasa/ident"
)

// Loudness is the presence loudness byte (lasa-core.md §7): 0 is digital
// full scale, each step -0.5 dB down to 254 = -127 dBFS; 255 is the
// silence sentinel (below -127 dBFS). It is the entity's source loudness
// post-gain, pre-attenuation.
type Loudness uint8

// LoudnessSilent is the silence sentinel (below -127 dBFS).
const LoudnessSilent Loudness = 255

// LoudnessFromDBFS quantizes a dBFS value. Values above 0 dBFS clamp to
// 0; values that quantize past 254 (below -127 dBFS) become
// LoudnessSilent.
func LoudnessFromDBFS(db float64) Loudness {
	q := math.Round(db * -2)
	if q < 0 {
		q = 0
	}
	if q > 255 {
		q = 255
	}
	return Loudness(q)
}

// DBFS returns the loudness in dBFS. For LoudnessSilent the value is
// -inf (silence), matching its below--127-dBFS meaning.
func (l Loudness) DBFS() float64 {
	if l == LoudnessSilent {
		return math.Inf(-1)
	}
	return float64(l) * -0.5
}

const (
	presenceFlagKeyframe = 1 << 0
	krecordFlagHeadFrame = 1 << 0
)

// DeltaRecord updates one entity by its keyframe-assigned index.
type DeltaRecord struct {
	Index    uint16
	Pose     Pose
	Loudness Loudness
}

// PresenceDelta is the presence delta packet: changed entities only,
// addressed by index against the last keyframe of generation Gen.
type PresenceDelta struct {
	Gen     uint8
	Records []DeltaRecord
}

// KeyframeRecord is one roster entry. HeadFrame marks the record's pose
// (and subsequent delta poses for this index) as listener-head-relative
// rather than world-frame (lasa-core.md §7).
type KeyframeRecord struct {
	HeadFrame bool
	ID        string
	Pose      Pose
	Loudness  Loudness
}

// PresenceKeyframe is one self-contained roster chunk: First is the index
// of the chunk's first record, subsequent records increment. Indexes are
// defined by position — the mapping is implicit.
type PresenceKeyframe struct {
	Gen     uint8
	First   uint16
	Records []KeyframeRecord
}

// AppendPresenceDelta appends the encoding of d.
func AppendPresenceDelta(buf []byte, d *PresenceDelta) ([]byte, error) {
	buf = append(buf, 0, d.Gen)
	for _, r := range d.Records {
		buf = binary.LittleEndian.AppendUint16(buf, r.Index)
		buf = AppendPose(buf, r.Pose)
		buf = append(buf, byte(r.Loudness))
	}
	return buf, nil
}

// AppendPresenceKeyframe appends the encoding of k. Every record id must
// be a valid identifier.
func AppendPresenceKeyframe(buf []byte, k *PresenceKeyframe) ([]byte, error) {
	buf = append(buf, presenceFlagKeyframe, k.Gen)
	buf = binary.LittleEndian.AppendUint16(buf, k.First)
	for _, r := range k.Records {
		if !ident.ValidIdentifier(r.ID) {
			return nil, fmt.Errorf("%w: invalid entity id %q", ErrMalformed, r.ID)
		}
		var kflags byte
		if r.HeadFrame {
			kflags |= krecordFlagHeadFrame
		}
		buf = append(buf, kflags, byte(len(r.ID)))
		buf = append(buf, r.ID...)
		buf = AppendPose(buf, r.Pose)
		buf = append(buf, byte(r.Loudness))
	}
	return buf, nil
}

// ParsePresence decodes a presence datagram, returning *PresenceDelta or
// *PresenceKeyframe. A packet with unknown flag bits set returns
// ErrUnknownFlags and MUST be discarded silently; records parse to the end
// of the datagram.
func ParsePresence(data []byte) (any, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("%w: presence packet too short", ErrMalformed)
	}
	flags := data[0]
	if flags&^byte(presenceFlagKeyframe) != 0 {
		return nil, ErrUnknownFlags
	}
	gen := data[1]
	rest := data[2:]

	if flags&presenceFlagKeyframe == 0 {
		const recordSize = 2 + PoseSize + 1
		if len(rest)%recordSize != 0 {
			return nil, fmt.Errorf("%w: presence delta length %d not a whole number of records", ErrMalformed, len(rest))
		}
		d := &PresenceDelta{Gen: gen}
		for len(rest) > 0 {
			pose, err := ParsePose(rest[2:])
			if err != nil {
				return nil, err
			}
			d.Records = append(d.Records, DeltaRecord{
				Index:    binary.LittleEndian.Uint16(rest),
				Pose:     pose,
				Loudness: Loudness(rest[2+PoseSize]),
			})
			rest = rest[recordSize:]
		}
		return d, nil
	}

	if len(rest) < 2 {
		return nil, fmt.Errorf("%w: truncated keyframe first index", ErrMalformed)
	}
	k := &PresenceKeyframe{Gen: gen, First: binary.LittleEndian.Uint16(rest)}
	rest = rest[2:]
	for len(rest) > 0 {
		if len(rest) < 2 {
			return nil, fmt.Errorf("%w: truncated krecord header", ErrMalformed)
		}
		kflags := rest[0]
		if kflags&^byte(krecordFlagHeadFrame) != 0 {
			return nil, ErrUnknownFlags
		}
		idLen := int(rest[1])
		rest = rest[2:]
		if len(rest) < idLen+PoseSize+1 {
			return nil, fmt.Errorf("%w: truncated krecord", ErrMalformed)
		}
		id := string(rest[:idLen])
		if !ident.ValidIdentifier(id) {
			return nil, fmt.Errorf("%w: invalid entity id %q", ErrMalformed, id)
		}
		pose, err := ParsePose(rest[idLen:])
		if err != nil {
			return nil, err
		}
		k.Records = append(k.Records, KeyframeRecord{
			HeadFrame: kflags&krecordFlagHeadFrame != 0,
			ID:        id,
			Pose:      pose,
			Loudness:  Loudness(rest[idLen+PoseSize]),
		})
		rest = rest[idLen+PoseSize+1:]
	}
	return k, nil
}
