package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrUnknownFlags reports flag bits a format does not define. Per the
// discard rule of lasa-core.md §5.1 the packet is not this format and MUST
// be discarded silently.
var ErrUnknownFlags = errors.New("wire: unknown flag bits set (not this format)")

// ErrMalformed reports a packet or message whose body is inconsistent with
// its claimed format.
var ErrMalformed = errors.New("wire: malformed")

// The shared flags byte of lasa-core.md §5.1. Bit positions are shared
// across formats; not every bit is meaningful in every format.
const (
	flagPose       = 1 << 0 // pose present (mono-object)
	flagAudio      = 1 << 1 // audio present (mono-object)
	flagRedundancy = 1 << 2 // redundancy present (all)
	offsetShift    = 3      // bits 3-5: redundancy offset 1-7
	offsetBits     = 0x7 << offsetShift
	flagTS         = 1 << 6 // ts present (sink formats)
	flagReserved   = 1 << 7 // reserved, MBZ
)

// Redundancy is a full copy of the audio frame Offset packets ago
// (offset 1-7; lasa-core.md §5.1). The referenced packet MUST be
// audio-bearing.
type Redundancy struct {
	Offset uint8
	Audio  []byte
}

// MonoObjectPacket is the mono-object uplink packet (lasa-core.md §5.3):
// one 5 ms mono Opus frame and/or a pose. A nil Pose or nil Audio marks
// the section absent; pose-only packets are silence, audio-only packets
// are pose decimation.
type MonoObjectPacket struct {
	Pose       *Pose
	Audio      []byte
	Redundancy *Redundancy
}

// SinkPacket is the shared layout of the binaural and ambi2/ambi3 sink
// formats (lasa-core.md §5.3): an optional u64 unix-epoch-microseconds
// timestamp naming the first sample of the frame, and exactly one audio
// payload (an Opus packet for binaural, an Opus multistream packet for
// ambi), which is always present — sink tracks are continuous.
type SinkPacket struct {
	Timestamp  *uint64
	Audio      []byte
	Redundancy *Redundancy
}

func validateRedundancy(r *Redundancy) error {
	if r.Offset < 1 || r.Offset > 7 {
		return fmt.Errorf("%w: redundancy offset %d outside 1-7", ErrMalformed, r.Offset)
	}
	if len(r.Audio) == 0 {
		return fmt.Errorf("%w: empty redundancy payload", ErrMalformed)
	}
	return nil
}

// AppendMonoObject appends the encoding of p to buf.
func AppendMonoObject(buf []byte, p *MonoObjectPacket) ([]byte, error) {
	var flags byte
	if p.Pose != nil {
		flags |= flagPose
	}
	if p.Audio != nil {
		if len(p.Audio) == 0 {
			return nil, fmt.Errorf("%w: empty audio payload", ErrMalformed)
		}
		flags |= flagAudio
	}
	if p.Redundancy != nil {
		if p.Audio == nil {
			return nil, fmt.Errorf("%w: redundancy requires an audio-bearing packet", ErrMalformed)
		}
		if err := validateRedundancy(p.Redundancy); err != nil {
			return nil, err
		}
		flags |= flagRedundancy | p.Redundancy.Offset<<offsetShift
	}
	buf = append(buf, flags)
	if p.Pose != nil {
		buf = AppendPose(buf, *p.Pose)
	}
	return appendAudioSection(buf, p.Audio, p.Redundancy), nil
}

// ParseMonoObject decodes a mono-object packet. A packet with unknown flag
// bits set returns ErrUnknownFlags and MUST be discarded silently.
func ParseMonoObject(data []byte) (*MonoObjectPacket, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty packet", ErrMalformed)
	}
	flags := data[0]
	if flags&(flagTS|flagReserved) != 0 {
		return nil, ErrUnknownFlags
	}
	if flags&flagRedundancy == 0 && flags&offsetBits != 0 {
		return nil, ErrUnknownFlags
	}
	rest := data[1:]

	p := &MonoObjectPacket{}
	if flags&flagPose != 0 {
		pose, err := ParsePose(rest)
		if err != nil {
			return nil, err
		}
		p.Pose = &pose
		rest = rest[PoseSize:]
	}
	hasAudio := flags&flagAudio != 0
	if flags&flagRedundancy != 0 && !hasAudio {
		return nil, fmt.Errorf("%w: redundancy on a packet with no audio", ErrMalformed)
	}
	audio, red, err := parseAudioSection(rest, hasAudio, flags)
	if err != nil {
		return nil, err
	}
	p.Audio, p.Redundancy = audio, red
	return p, nil
}

// AppendSink appends the encoding of p to buf. The same layout serves
// binaural and ambi2/ambi3; the payload is opaque here.
func AppendSink(buf []byte, p *SinkPacket) ([]byte, error) {
	if len(p.Audio) == 0 {
		return nil, fmt.Errorf("%w: sink packets always carry audio", ErrMalformed)
	}
	var flags byte
	if p.Timestamp != nil {
		flags |= flagTS
	}
	if p.Redundancy != nil {
		if err := validateRedundancy(p.Redundancy); err != nil {
			return nil, err
		}
		flags |= flagRedundancy | p.Redundancy.Offset<<offsetShift
	}
	buf = append(buf, flags)
	if p.Timestamp != nil {
		buf = binary.LittleEndian.AppendUint64(buf, *p.Timestamp)
	}
	return appendAudioSection(buf, p.Audio, p.Redundancy), nil
}

// ParseSink decodes a sink packet (binaural or ambi2/ambi3 — the caller
// knows which from the track name). A packet with unknown flag bits set
// returns ErrUnknownFlags and MUST be discarded silently.
func ParseSink(data []byte) (*SinkPacket, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty packet", ErrMalformed)
	}
	flags := data[0]
	if flags&(flagPose|flagAudio|flagReserved) != 0 {
		return nil, ErrUnknownFlags
	}
	if flags&flagRedundancy == 0 && flags&offsetBits != 0 {
		return nil, ErrUnknownFlags
	}
	rest := data[1:]

	p := &SinkPacket{}
	if flags&flagTS != 0 {
		if len(rest) < 8 {
			return nil, fmt.Errorf("%w: truncated timestamp", ErrMalformed)
		}
		ts := binary.LittleEndian.Uint64(rest)
		p.Timestamp = &ts
		rest = rest[8:]
	}
	audio, red, err := parseAudioSection(rest, true, flags)
	if err != nil {
		return nil, err
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("%w: sink packets always carry audio", ErrMalformed)
	}
	p.Audio, p.Redundancy = audio, red
	return p, nil
}

// appendAudioSection appends [len(2)?][audio][audio-dup?]: the u16
// little-endian length prefix on the primary payload is present iff the
// redundancy bit says so (lasa-core.md §5.1).
func appendAudioSection(buf, audio []byte, red *Redundancy) []byte {
	if red != nil {
		buf = binary.LittleEndian.AppendUint16(buf, uint16(len(audio)))
		buf = append(buf, audio...)
		return append(buf, red.Audio...)
	}
	return append(buf, audio...)
}

func parseAudioSection(rest []byte, hasAudio bool, flags byte) ([]byte, *Redundancy, error) {
	if flags&flagRedundancy != 0 {
		offset := flags & offsetBits >> offsetShift
		if offset == 0 {
			return nil, nil, fmt.Errorf("%w: redundancy offset 0 outside 1-7", ErrMalformed)
		}
		if len(rest) < 2 {
			return nil, nil, fmt.Errorf("%w: truncated length prefix", ErrMalformed)
		}
		n := int(binary.LittleEndian.Uint16(rest))
		rest = rest[2:]
		if n == 0 || len(rest) <= n {
			return nil, nil, fmt.Errorf("%w: bad primary payload length %d of %d", ErrMalformed, n, len(rest))
		}
		return rest[:n], &Redundancy{Offset: offset, Audio: rest[n:]}, nil
	}
	if !hasAudio {
		if len(rest) != 0 {
			return nil, nil, fmt.Errorf("%w: %d trailing bytes", ErrMalformed, len(rest))
		}
		return nil, nil, nil
	}
	if len(rest) == 0 {
		return nil, nil, fmt.Errorf("%w: empty audio payload", ErrMalformed)
	}
	return rest, nil, nil
}
