package wire

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"
)

func TestCanonicalPrefixes(t *testing.T) {
	got := CanonicalPrefixes([]string{"lasa.", "app.", "lasa.", ""})
	want := []string{"", "app.", "lasa."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CanonicalPrefixes = %v, want %v", got, want)
	}
}

func TestSetHashDefinition(t *testing.T) {
	// The set-hash input is exactly the concatenated [len varint][bytes]
	// entries of the canonical encoding.
	canonical := []string{"app.", "lasa."}
	var preimage []byte
	for _, p := range canonical {
		preimage = AppendVarint(preimage, uint64(len(p)))
		preimage = append(preimage, p...)
	}
	want := sha256.Sum256(preimage)
	if got := SetHash(canonical); got != want {
		t.Errorf("SetHash = %x, want %x", got, want)
	}
}

func TestSubscribeParamsGolden(t *testing.T) {
	cases := []struct {
		name string
		p    SubscribeParams
		hex  string
	}{
		{
			// Canonicalized to ["app.", "lasa."]; flags 0, no cursor.
			"fresh two prefixes",
			SubscribeParams{Prefixes: []string{"lasa.", "app."}},
			"00" + "02" + "04" + "6170702e" + "05" + "6c6173612e",
		},
		{
			// The set containing the empty prefix subscribes to everything.
			"fresh empty prefix",
			SubscribeParams{Prefixes: []string{""}},
			"000100",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := mustHex(t, c.hex)
			got, err := AppendSubscribeParams(nil, &c.p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("encode = %x, want %s", got, c.hex)
			}
			back, err := ParseSubscribeParams(want)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back.Prefixes, CanonicalPrefixes(c.p.Prefixes)) || back.Cursor != nil {
				t.Fatalf("decode = %+v", back)
			}
		})
	}
}

func TestSubscribeParamsCursorRoundTrip(t *testing.T) {
	prefixes := []string{"lasa.entity.", "app."}
	cursor := &Cursor{Epoch: stateEpoch, Seq: 15293, SetHash: SetHash(CanonicalPrefixes(prefixes))}
	buf, err := AppendSubscribeParams(nil, &SubscribeParams{Prefixes: prefixes, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseSubscribeParams(buf)
	if err != nil {
		t.Fatal(err)
	}
	if back.Cursor == nil || *back.Cursor != *cursor {
		t.Fatalf("cursor did not round-trip: %+v", back.Cursor)
	}
	if !reflect.DeepEqual(back.Prefixes, []string{"app.", "lasa.entity."}) {
		t.Fatalf("prefixes = %v", back.Prefixes)
	}
}

func TestSubscribeParamsRejects(t *testing.T) {
	cases := []struct {
		name string
		hex  string
	}{
		{"empty", ""},
		{"unknown flag bits", "02" + "0100"},
		{"empty prefix set", "0000"},
		{"non-canonical order", "0002" + "05" + "6c6173612e" + "04" + "6170702e"},
		{"duplicate prefix", "0002" + "04" + "6170702e" + "04" + "6170702e"},
		{"non-minimal count varint", "00" + "4002" + "04" + "6170702e" + "05" + "6c6173612e"},
		{"non-minimal length varint", "0001" + "4004" + "6170702e"},
		{"invalid prefix char", "000101" + "41"}, // "A"
		{"truncated prefix", "000105" + "6170"},
		{"cursor missing", "010100"},
		{"trailing bytes", "000100" + "ff"},
	}
	for _, c := range cases {
		if _, err := ParseSubscribeParams(mustHex(t, c.hex)); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: got %v, want ErrMalformed", c.name, err)
		}
	}
	if _, err := AppendSubscribeParams(nil, &SubscribeParams{}); err == nil {
		t.Error("encoding an empty prefix set must fail")
	}
}
