package wire

import (
	"encoding/binary"
	"fmt"
)

// MonoObjectScratch is caller-owned storage for ParseMonoObjectInto:
// reusing one scratch across calls makes the parse allocation-free (the
// receiver hot path — server/depacketizer.go). The parsed packet's
// Pose/Redundancy pointers alias the scratch; its Audio slices alias
// the input. Contents are valid until the next call with the same
// scratch.
type MonoObjectScratch struct {
	Pose Pose
	Red  Redundancy
}

// ParseMonoObjectInto is ParseMonoObject without allocation: identical
// validation and layout rules, output written into p using sc for the
// variable-presence sections.
func ParseMonoObjectInto(data []byte, sc *MonoObjectScratch, p *MonoObjectPacket) error {
	p.Pose, p.Audio, p.Redundancy = nil, nil, nil
	if len(data) == 0 {
		return fmt.Errorf("%w: empty packet", ErrMalformed)
	}
	flags := data[0]
	if flags&(flagTS|flagReserved) != 0 {
		return ErrUnknownFlags
	}
	if flags&flagRedundancy == 0 && flags&offsetBits != 0 {
		return ErrUnknownFlags
	}
	rest := data[1:]

	if flags&flagPose != 0 {
		pose, err := ParsePose(rest)
		if err != nil {
			return err
		}
		sc.Pose = pose
		p.Pose = &sc.Pose
		rest = rest[PoseSize:]
	}
	hasAudio := flags&flagAudio != 0
	if flags&flagRedundancy != 0 && !hasAudio {
		return fmt.Errorf("%w: redundancy on a packet with no audio", ErrMalformed)
	}
	audio, hasRed, err := parseAudioSectionInto(rest, hasAudio, flags, &sc.Red)
	if err != nil {
		return err
	}
	p.Audio = audio
	if hasRed {
		p.Redundancy = &sc.Red
	}
	return nil
}

// parseAudioSectionInto is the allocation-free core of
// parseAudioSection (audio.go's variant delegates here).
func parseAudioSectionInto(rest []byte, hasAudio bool, flags byte, red *Redundancy) ([]byte, bool, error) {
	if flags&flagRedundancy != 0 {
		offset := flags & offsetBits >> offsetShift
		if offset == 0 {
			return nil, false, fmt.Errorf("%w: redundancy offset 0 outside 1-7", ErrMalformed)
		}
		if len(rest) < 2 {
			return nil, false, fmt.Errorf("%w: truncated length prefix", ErrMalformed)
		}
		n := int(binary.LittleEndian.Uint16(rest))
		rest = rest[2:]
		if n == 0 || len(rest) <= n {
			return nil, false, fmt.Errorf("%w: bad primary payload length %d of %d", ErrMalformed, n, len(rest))
		}
		red.Offset = offset
		red.Audio = rest[n:]
		return rest[:n], true, nil
	}
	if !hasAudio {
		if len(rest) != 0 {
			return nil, false, fmt.Errorf("%w: %d trailing bytes", ErrMalformed, len(rest))
		}
		return nil, false, nil
	}
	if len(rest) == 0 {
		return nil, false, fmt.Errorf("%w: empty audio payload", ErrMalformed)
	}
	return rest, false, nil
}
