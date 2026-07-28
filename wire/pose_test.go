package wire

import (
	"bytes"
	"math"
	"testing"
)

// Golden vector: x=1, y=0, z=-1 metres; yaw=π/2, pitch=0, roll=-π.
// f32 little-endian, int16 little-endian at π/32767 per step.
const poseHex = "0000803f00000000000080bf004000000180"

func goldenPose() Pose {
	return Pose{X: 1, Y: 0, Z: -1, Yaw: math.Pi / 2, Pitch: 0, Roll: -math.Pi}
}

func TestPoseGoldenEncode(t *testing.T) {
	got := AppendPose(nil, goldenPose())
	if want := mustHex(t, poseHex); !bytes.Equal(got, want) {
		t.Errorf("AppendPose = %x, want %s", got, poseHex)
	}
}

func TestPoseGoldenDecode(t *testing.T) {
	p, err := ParsePose(mustHex(t, poseHex))
	if err != nil {
		t.Fatal(err)
	}
	if p.X != 1 || p.Y != 0 || p.Z != -1 {
		t.Errorf("position = (%v,%v,%v), want (1,0,-1)", p.X, p.Y, p.Z)
	}
	// Quantized expectations: yaw rounds to 16384 steps, roll to -32767.
	if want := float32(16384 * math.Pi / 32767); p.Yaw != want {
		t.Errorf("yaw = %v, want %v", p.Yaw, want)
	}
	if p.Pitch != 0 {
		t.Errorf("pitch = %v, want 0", p.Pitch)
	}
	if want := float32(-math.Pi); p.Roll != want {
		t.Errorf("roll = %v, want %v", p.Roll, want)
	}
}

func TestPoseRoundTripWithinQuantum(t *testing.T) {
	orig := Pose{X: -12.5, Y: 3.25, Z: 0.001, Yaw: -1.234, Pitch: 0.5, Roll: 3.0}
	p, err := ParsePose(AppendPose(nil, orig))
	if err != nil {
		t.Fatal(err)
	}
	if p.X != orig.X || p.Y != orig.Y || p.Z != orig.Z {
		t.Errorf("position not exact: %+v vs %+v", p, orig)
	}
	for _, d := range []float64{
		float64(p.Yaw - orig.Yaw), float64(p.Pitch - orig.Pitch), float64(p.Roll - orig.Roll),
	} {
		if math.Abs(d) > AngleQuantum/2*1.001 {
			t.Errorf("angle error %v exceeds half quantum", d)
		}
	}
}

func TestPoseAngleClamp(t *testing.T) {
	p, err := ParsePose(AppendPose(nil, Pose{Yaw: 100, Roll: -100}))
	if err != nil {
		t.Fatal(err)
	}
	if want := float32(math.Pi); p.Yaw != want {
		t.Errorf("out-of-range yaw must clamp to π, got %v", p.Yaw)
	}
	if want := float32(-math.Pi); p.Roll != want {
		t.Errorf("out-of-range roll must clamp to -π, got %v", p.Roll)
	}
}

func TestParsePoseTruncated(t *testing.T) {
	if _, err := ParsePose(make([]byte, PoseSize-1)); err == nil {
		t.Error("want error for 17-byte pose")
	}
}
