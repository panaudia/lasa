package wire

import (
	"encoding/binary"
	"math"
)

// PoseSize is the encoded size of a pose in bytes.
const PoseSize = 18

// Pose is the 18-byte pose of lasa-core.md §5.1, in engineering units:
// position in metres (x forward, y left, z up) and attitude in radians
// (positive yaw anticlockwise about z, positive pitch upward, positive
// roll anticlockwise about the line of sight).
//
// On the wire, angles are quantized to int16 at π/32767 per step; callers
// should normalise angles into [-π, π] before encoding — values outside
// that range are clamped, not wrapped.
type Pose struct {
	X, Y, Z          float32
	Yaw, Pitch, Roll float32
}

// AngleQuantum is the wire resolution of pose angles in radians.
const AngleQuantum = math.Pi / 32767

// AppendPose appends the 18-byte encoding of p to buf.
func AppendPose(buf []byte, p Pose) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.X))
	buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.Y))
	buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.Z))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(quantizeAngle(p.Yaw)))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(quantizeAngle(p.Pitch)))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(quantizeAngle(p.Roll)))
	return buf
}

// ParsePose reads an 18-byte pose from the front of data.
func ParsePose(data []byte) (Pose, error) {
	if len(data) < PoseSize {
		return Pose{}, ErrMalformed
	}
	return Pose{
		X:     math.Float32frombits(binary.LittleEndian.Uint32(data[0:])),
		Y:     math.Float32frombits(binary.LittleEndian.Uint32(data[4:])),
		Z:     math.Float32frombits(binary.LittleEndian.Uint32(data[8:])),
		Yaw:   angleFromWire(int16(binary.LittleEndian.Uint16(data[12:]))),
		Pitch: angleFromWire(int16(binary.LittleEndian.Uint16(data[14:]))),
		Roll:  angleFromWire(int16(binary.LittleEndian.Uint16(data[16:]))),
	}, nil
}

func quantizeAngle(a float32) int16 {
	q := math.Round(float64(a) * 32767 / math.Pi)
	if q > 32767 {
		q = 32767
	}
	if q < -32767 {
		q = -32767
	}
	return int16(q)
}

func angleFromWire(q int16) float32 {
	return float32(float64(q) * math.Pi / 32767)
}
