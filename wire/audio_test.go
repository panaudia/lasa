package wire

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func u64ptr(v uint64) *uint64 { return &v }

// Golden vectors for the audio packet layouts of lasa-core.md §5.3.
func TestMonoObjectGolden(t *testing.T) {
	pose := goldenPose()
	cases := []struct {
		name string
		pkt  MonoObjectPacket
		hex  string
	}{
		{
			// flags 0x03: pose + audio, no redundancy.
			"pose_and_audio",
			MonoObjectPacket{Pose: &pose, Audio: []byte("abc")},
			"03" + poseHex + "616263",
		},
		{
			// flags 0x1e: audio + redundancy offset 3; u16 LE len prefix.
			"audio_with_redundancy",
			MonoObjectPacket{Audio: []byte("abc"), Redundancy: &Redundancy{Offset: 3, Audio: []byte("de")}},
			"1e" + "0300" + "616263" + "6465",
		},
		{
			// flags 0x01: pose-only (silence).
			"pose_only",
			MonoObjectPacket{Pose: &pose},
			"01" + poseHex,
		},
		{
			// flags 0x00: empty packet — wire-legal, nothing present.
			"empty",
			MonoObjectPacket{},
			"00",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := mustHex(t, c.hex)
			got, err := AppendMonoObject(nil, &c.pkt)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("encode = %x, want %s", got, c.hex)
			}
			back, err := ParseMonoObject(want)
			if err != nil {
				t.Fatal(err)
			}
			// Compare via re-encode to sidestep pose quantization.
			re, err := AppendMonoObject(nil, back)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(re, want) {
				t.Fatalf("re-encode = %x, want %s", re, c.hex)
			}
		})
	}
}

func TestSinkGolden(t *testing.T) {
	cases := []struct {
		name string
		pkt  SinkPacket
		hex  string
	}{
		{
			// flags 0x40: u64 LE timestamp, then audio.
			"with_timestamp",
			SinkPacket{Timestamp: u64ptr(0x0102030405060708), Audio: []byte{0xAA, 0xBB}},
			"40" + "0807060504030201" + "aabb",
		},
		{
			"plain",
			SinkPacket{Audio: []byte{0xAA, 0xBB}},
			"00aabb",
		},
		{
			// flags 0x0c: redundancy offset 1.
			"with_redundancy",
			SinkPacket{Audio: []byte{0xFF}, Redundancy: &Redundancy{Offset: 1, Audio: []byte{0xEE}}},
			"0c" + "0100" + "ff" + "ee",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := mustHex(t, c.hex)
			got, err := AppendSink(nil, &c.pkt)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("encode = %x, want %s", got, c.hex)
			}
			back, err := ParseSink(want)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, &c.pkt) {
				t.Fatalf("decode = %+v, want %+v", back, &c.pkt)
			}
		})
	}
}

func TestAudioDiscardRule(t *testing.T) {
	// Unknown flag bits: not this format, MUST discard (ErrUnknownFlags).
	discard := []struct {
		name  string
		parse func([]byte) error
		hex   string
	}{
		{"mono reserved bit", parseMonoErr, "80"},
		{"mono ts bit", parseMonoErr, "40"},
		{"mono offset without redundancy", parseMonoErr, "0a" + "616263"},
		{"sink reserved bit", parseSinkErr, "80aabb"},
		{"sink pose bit", parseSinkErr, "01aabb"},
		{"sink audio bit", parseSinkErr, "02aabb"},
		{"sink offset without redundancy", parseSinkErr, "08aabb"},
	}
	for _, c := range discard {
		if err := c.parse(mustHex(t, c.hex)); !errors.Is(err, ErrUnknownFlags) {
			t.Errorf("%s: got %v, want ErrUnknownFlags", c.name, err)
		}
	}

	malformed := []struct {
		name  string
		parse func([]byte) error
		hex   string
	}{
		{"mono redundancy without audio", parseMonoErr, "05" + poseHex + "0100" + "ff" + "ee"},
		{"mono redundancy offset zero", parseMonoErr, "06" + "0100" + "ff" + "ee"},
		{"mono audio bit with empty payload", parseMonoErr, "02"},
		{"mono trailing bytes on pose-only", parseMonoErr, "01" + poseHex + "ff"},
		{"mono truncated pose", parseMonoErr, "010000"},
		{"sink no audio", parseSinkErr, "00"},
		{"sink truncated ts", parseSinkErr, "40aabb"},
		{"sink dup missing", parseSinkErr, "0c" + "0200" + "aabb"},
		{"redundancy primary len zero", parseSinkErr, "0c" + "0000" + "ee"},
		{"empty", parseSinkErr, ""},
	}
	for _, c := range malformed {
		if err := c.parse(mustHex(t, c.hex)); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: got %v, want ErrMalformed", c.name, err)
		}
	}
}

func parseMonoErr(b []byte) error { _, err := ParseMonoObject(b); return err }
func parseSinkErr(b []byte) error { _, err := ParseSink(b); return err }

func TestAudioEncodeValidation(t *testing.T) {
	if _, err := AppendMonoObject(nil, &MonoObjectPacket{Redundancy: &Redundancy{Offset: 1, Audio: []byte{1}}}); err == nil {
		t.Error("redundancy without audio must fail to encode")
	}
	if _, err := AppendMonoObject(nil, &MonoObjectPacket{Audio: []byte{1}, Redundancy: &Redundancy{Offset: 8, Audio: []byte{1}}}); err == nil {
		t.Error("offset 8 must fail to encode")
	}
	if _, err := AppendMonoObject(nil, &MonoObjectPacket{Audio: []byte{}}); err == nil {
		t.Error("empty (non-nil) audio must fail to encode")
	}
	if _, err := AppendSink(nil, &SinkPacket{}); err == nil {
		t.Error("sink without audio must fail to encode")
	}
}
