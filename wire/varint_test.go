package wire

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// RFC 9000 §A.1 example values plus boundary cases.
var varintVectors = []struct {
	v   uint64
	hex string
}{
	{0, "00"},
	{37, "25"},
	{63, "3f"},
	{64, "4040"},
	{15293, "7bbd"},
	{16383, "7fff"},
	{16384, "80004000"},
	{494878333, "9d7f3e7d"},
	{1073741823, "bfffffff"},
	{1073741824, "c000000040000000"},
	{151288809941952652, "c2197c5eff14e88c"},
	{MaxVarint, "ffffffffffffffff"},
}

func TestVarintVectors(t *testing.T) {
	for _, c := range varintVectors {
		want := mustHex(t, c.hex)
		if got := AppendVarint(nil, c.v); !bytes.Equal(got, want) {
			t.Errorf("AppendVarint(%d) = %x, want %s", c.v, got, c.hex)
		}
		v, n, err := ParseVarint(want)
		if err != nil || v != c.v || n != len(want) {
			t.Errorf("ParseVarint(%s) = %d,%d,%v, want %d,%d,nil", c.hex, v, n, err, c.v, len(want))
		}
		if VarintLen(c.v) != len(want) {
			t.Errorf("VarintLen(%d) = %d, want %d", c.v, VarintLen(c.v), len(want))
		}
	}
}

func TestParseVarintNonMinimalAccepted(t *testing.T) {
	v, n, err := ParseVarint(mustHex(t, "4025"))
	if err != nil || v != 37 || n != 2 {
		t.Errorf("ParseVarint(4025) = %d,%d,%v, want 37,2,nil", v, n, err)
	}
}

func TestParseVarintTruncated(t *testing.T) {
	for _, s := range []string{"", "40", "80ff", "c000112233"} {
		if _, _, err := ParseVarint(mustHex(t, s)); err == nil {
			t.Errorf("ParseVarint(%q): want error", s)
		}
	}
}
