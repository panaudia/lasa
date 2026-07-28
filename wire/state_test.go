package wire

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

var stateEpoch = [8]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}

// Golden vectors for the state messages of lasa-core.md §6.7, in both
// directions. Seq is downstream-only: the same message encodes differently
// per direction.
func TestStateGolden(t *testing.T) {
	cases := []struct {
		name string
		msg  StateMessage
		dir  StateDirection
		hex  string
	}{
		{"set upstream", Set{Key: "a.b", Value: []byte("hi")}, Upstream, "0103612e62026869"},
		{"set downstream", Set{Key: "a.b", Value: []byte("hi"), Seq: 5}, Downstream, "0103612e6202686905"},
		{"set empty value upstream", Set{Key: "k", Value: []byte{}}, Upstream, "01016b00"},
		{"clear upstream", Clear{Key: "k"}, Upstream, "02016b"},
		{"clear downstream", Clear{Key: "k", Seq: 15293}, Downstream, "02016b7bbd"},
		{
			"group upstream",
			Group{Ops: []StateMessage{Set{Key: "a", Value: []byte("1")}, Clear{Key: "b"}}},
			Upstream,
			"03020101610131020162",
		},
		{
			"group downstream",
			Group{Ops: []StateMessage{Set{Key: "a", Value: []byte("1"), Seq: 9}, Clear{Key: "b", Seq: 10}}},
			Downstream,
			"0302010161013109020162 0a",
		},
		{"frontier", Frontier{Epoch: stateEpoch, Seq: 37}, Downstream, "04001122334455667725"},
		{"begin delta", Begin{Kind: BeginDelta}, Downstream, "0501"},
		{"begin snapshot", Begin{Kind: BeginSnapshot}, Downstream, "0502"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := mustHex(t, strings.ReplaceAll(c.hex, " ", ""))
			got, err := AppendStateMessage(nil, c.msg, c.dir)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("encode = %x, want %x", got, want)
			}
			back, err := ParseStateMessage(want, c.dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, c.msg) {
				t.Fatalf("decode = %#v, want %#v", back, c.msg)
			}
		})
	}
}

// The same bytes parse differently per direction — the spec's warning that
// implementations must select the layout by track direction.
func TestStateDirectionMatters(t *testing.T) {
	down, err := AppendStateMessage(nil, Set{Key: "k", Value: []byte("v"), Seq: 3}, Downstream)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseStateMessage(down, Upstream); err == nil {
		t.Error("downstream set bytes must not parse as one upstream message (trailing seq)")
	}
}

func TestStateParseRejects(t *testing.T) {
	cases := []struct {
		name string
		dir  StateDirection
		hex  string
	}{
		{"empty", Upstream, ""},
		{"unknown type", Upstream, "06"},
		{"frontier upstream", Upstream, "04001122334455667725"},
		{"begin upstream", Upstream, "0501"},
		{"begin unknown kind", Downstream, "0503"},
		{"truncated key", Upstream, "01056162"},
		{"truncated value", Upstream, "0101610461"},
		{"invalid key char", Upstream, "0101410131"}, // key "A"
		{"nested group", Upstream, "0301030101610131"},
		{"group with frontier", Downstream, "030104001122334455667725"},
		{"trailing bytes", Upstream, "02016bff"},
		{"truncated epoch", Downstream, "04001122"},
	}
	for _, c := range cases {
		if _, err := ParseStateMessage(mustHex(t, c.hex), c.dir); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: got %v, want ErrMalformed", c.name, err)
		}
	}
}

func TestStateEncodeRejects(t *testing.T) {
	if _, err := AppendStateMessage(nil, Frontier{}, Upstream); err == nil {
		t.Error("frontier upstream must fail to encode")
	}
	if _, err := AppendStateMessage(nil, Set{Key: "Bad"}, Upstream); err == nil {
		t.Error("invalid key must fail to encode")
	}
	if _, err := AppendStateMessage(nil, Set{Key: "k", Value: make([]byte, MaxValueLen+1)}, Upstream); err == nil {
		t.Error("oversize value must fail to encode")
	}
	if _, err := AppendStateMessage(nil, Group{Ops: []StateMessage{Begin{Kind: BeginDelta}}}, Downstream); err == nil {
		t.Error("group containing begin must fail to encode")
	}
	if _, err := AppendStateMessage(nil, Group{Ops: []StateMessage{Group{}}}, Upstream); err == nil {
		t.Error("nested group must fail to encode")
	}
}

func TestStateMaxValueRoundTrip(t *testing.T) {
	v := make([]byte, MaxValueLen)
	v[0], v[MaxValueLen-1] = 0xAB, 0xCD
	buf, err := AppendStateMessage(nil, Set{Key: "big", Value: v, Seq: 1}, Downstream)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseStateMessage(buf, Downstream)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back.(Set).Value, v) {
		t.Error("64 KiB value did not round-trip")
	}
}
