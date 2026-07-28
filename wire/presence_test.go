package wire

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestLoudness(t *testing.T) {
	cases := []struct {
		db   float64
		want Loudness
	}{
		{0, 0}, {3, 0}, {-0.5, 1}, {-0.6, 1}, {-10, 20},
		{-127, 254}, {-127.5, 255}, {-200, 255},
	}
	for _, c := range cases {
		if got := LoudnessFromDBFS(c.db); got != c.want {
			t.Errorf("LoudnessFromDBFS(%v) = %d, want %d", c.db, got, c.want)
		}
	}
	if Loudness(1).DBFS() != -0.5 {
		t.Errorf("Loudness(1).DBFS() = %v, want -0.5", Loudness(1).DBFS())
	}
	if !math.IsInf(LoudnessSilent.DBFS(), -1) {
		t.Error("silent loudness must report -inf dBFS")
	}
}

func TestPresenceDeltaGolden(t *testing.T) {
	d := &PresenceDelta{
		Gen:     7,
		Records: []DeltaRecord{{Index: 2, Pose: goldenPose(), Loudness: 10}},
	}
	// flags 0x00, gen, then records: u16 LE index, pose, loudness.
	wantHex := "00" + "07" + "0200" + poseHex + "0a"
	got, err := AppendPresenceDelta(nil, d)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, wantHex); !bytes.Equal(got, want) {
		t.Fatalf("encode = %x, want %s", got, wantHex)
	}
	back, err := ParsePresence(got)
	if err != nil {
		t.Fatal(err)
	}
	bd, ok := back.(*PresenceDelta)
	if !ok || bd.Gen != 7 || len(bd.Records) != 1 || bd.Records[0].Index != 2 || bd.Records[0].Loudness != 10 {
		t.Fatalf("decode = %#v", back)
	}
}

func TestPresenceKeyframeGolden(t *testing.T) {
	k := &PresenceKeyframe{
		Gen:   7,
		First: 0,
		Records: []KeyframeRecord{
			{HeadFrame: true, ID: "e1", Pose: goldenPose(), Loudness: LoudnessSilent},
		},
	}
	// flags 0x01 (keyframe), gen, u16 LE first, then krecords:
	// kflags (bit0 head-frame), u8 id-len, id, pose, loudness.
	wantHex := "01" + "07" + "0000" + "01" + "02" + "6531" + poseHex + "ff"
	got, err := AppendPresenceKeyframe(nil, k)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, wantHex); !bytes.Equal(got, want) {
		t.Fatalf("encode = %x, want %s", got, wantHex)
	}
	back, err := ParsePresence(got)
	if err != nil {
		t.Fatal(err)
	}
	bk, ok := back.(*PresenceKeyframe)
	if !ok || bk.Gen != 7 || bk.First != 0 || len(bk.Records) != 1 {
		t.Fatalf("decode = %#v", back)
	}
	r := bk.Records[0]
	if !r.HeadFrame || r.ID != "e1" || r.Loudness != LoudnessSilent {
		t.Fatalf("record = %+v", r)
	}
}

func TestPresenceMultiRecordRoundTrip(t *testing.T) {
	k := &PresenceKeyframe{
		Gen:   1,
		First: 256,
		Records: []KeyframeRecord{
			{ID: "aa", Pose: Pose{X: 1}, Loudness: 3},
			{HeadFrame: true, ID: "hud-1", Pose: Pose{Z: 2}, Loudness: 0},
			{ID: "550e8400-e29b-41d4-a716-446655440000", Loudness: 200},
		},
	}
	buf, err := AppendPresenceKeyframe(nil, k)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParsePresence(buf)
	if err != nil {
		t.Fatal(err)
	}
	bk := back.(*PresenceKeyframe)
	if bk.First != 256 || len(bk.Records) != 3 {
		t.Fatalf("decode = %#v", bk)
	}
	ids := []string{bk.Records[0].ID, bk.Records[1].ID, bk.Records[2].ID}
	if !reflect.DeepEqual(ids, []string{"aa", "hud-1", "550e8400-e29b-41d4-a716-446655440000"}) {
		t.Fatalf("ids = %v", ids)
	}

	// Empty delta and empty keyframe chunk are wire-legal.
	if _, err := ParsePresence(mustHex(t, "0007")); err != nil {
		t.Errorf("empty delta: %v", err)
	}
	if _, err := ParsePresence(mustHex(t, "01070000")); err != nil {
		t.Errorf("empty keyframe chunk: %v", err)
	}
}

func TestPresenceRejects(t *testing.T) {
	if _, err := ParsePresence(mustHex(t, "02"+"07")); !errors.Is(err, ErrUnknownFlags) {
		t.Error("unknown presence flag bits must return ErrUnknownFlags")
	}
	// krecord with unknown kflags bit.
	buf, err := AppendPresenceKeyframe(nil, &PresenceKeyframe{Records: []KeyframeRecord{{ID: "e1"}}})
	if err != nil {
		t.Fatal(err)
	}
	buf[4] |= 0x02 // kflags of the first record
	if _, err := ParsePresence(buf); !errors.Is(err, ErrUnknownFlags) {
		t.Error("unknown krecord flag bits must return ErrUnknownFlags")
	}

	malformed := []string{
		"00",                         // too short
		"0007" + "0200",              // partial delta record
		"0107",                       // keyframe without first index
		"01070000" + "01",            // truncated krecord header
		"01070000" + "0102" + "6531", // truncated krecord body
		"01070000" + "0102" + "4531" + poseHex + "ff", // id "E1" invalid
	}
	for _, s := range malformed {
		if _, err := ParsePresence(mustHex(t, s)); !errors.Is(err, ErrMalformed) {
			t.Errorf("%q: got %v, want ErrMalformed", s, err)
		}
	}

	if _, err := AppendPresenceKeyframe(nil, &PresenceKeyframe{Records: []KeyframeRecord{{ID: "E1"}}}); err == nil {
		t.Error("invalid id must fail to encode")
	}
}
