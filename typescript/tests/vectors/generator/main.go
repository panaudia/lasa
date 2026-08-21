// Golden-vector generator: emits tests/vectors/lasa-wire.json from the
// authoritative Go codecs. Run from the lasa repo root (it needs that
// module context):
//
//	go run ./typescript/tests/vectors/generator \
//	  > typescript/tests/vectors/lasa-wire.json
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/panaudia/lasa/wire"
)

type vec struct {
	Name string          `json:"name"`
	Hex  string          `json:"hex"`
	Meta json.RawMessage `json:"meta"`
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func j(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func main() {
	var out []vec
	add := func(name string, b []byte, meta any) {
		out = append(out, vec{Name: name, Hex: hex.EncodeToString(b), Meta: j(meta)})
	}

	p1 := wire.Pose{X: 1.5, Y: -2.25, Z: 0.125, Yaw: 1.0, Pitch: -0.5, Roll: 3.2}
	add("pose-basic", wire.AppendPose(nil, p1), p1)
	p2 := wire.Pose{Yaw: -4.0} // clamps
	add("pose-clamp", wire.AppendPose(nil, p2), p2)

	audio := []byte{0xf8, 0x01, 0x02, 0x03, 0x04}
	red := []byte{0xf8, 0x09, 0x08}

	add("mono-pose-only", must(wire.AppendMonoObject(nil, &wire.MonoObjectPacket{Pose: &p1})), nil)
	add("mono-audio-only", must(wire.AppendMonoObject(nil, &wire.MonoObjectPacket{Audio: audio})), nil)
	add("mono-both", must(wire.AppendMonoObject(nil, &wire.MonoObjectPacket{Pose: &p1, Audio: audio})), nil)
	add("mono-redundancy", must(wire.AppendMonoObject(nil, &wire.MonoObjectPacket{
		Pose: &p1, Audio: audio, Redundancy: &wire.Redundancy{Offset: 3, Audio: red},
	})), map[string]any{"offset": 3})

	ts := uint64(1_722_900_000_123_456)
	add("sink-plain", must(wire.AppendSink(nil, &wire.SinkPacket{Audio: audio})), nil)
	add("sink-ts", must(wire.AppendSink(nil, &wire.SinkPacket{Timestamp: &ts, Audio: audio})), map[string]any{"ts": fmt.Sprint(ts)})
	add("sink-ts-redundancy", must(wire.AppendSink(nil, &wire.SinkPacket{
		Timestamp: &ts, Audio: audio, Redundancy: &wire.Redundancy{Offset: 1, Audio: red},
	})), map[string]any{"ts": fmt.Sprint(ts), "offset": 1})

	kf := &wire.PresenceKeyframe{Gen: 7, First: 2, Records: []wire.KeyframeRecord{
		{ID: "alice", Pose: p1, Loudness: 46},
		{HeadFrame: true, ID: "angel-1", Pose: wire.Pose{Y: 0.5}, Loudness: wire.LoudnessSilent},
	}}
	add("presence-keyframe", must(wire.AppendPresenceKeyframe(nil, kf)), map[string]any{
		"gen": 7, "first": 2, "ids": []string{"alice", "angel-1"},
	})
	d := &wire.PresenceDelta{Gen: 7, Records: []wire.DeltaRecord{
		{Index: 2, Pose: p1, Loudness: 46},
		{Index: 3, Pose: wire.Pose{Y: 0.5}, Loudness: 255},
	}}
	add("presence-delta", must(wire.AppendPresenceDelta(nil, d)), map[string]any{"gen": 7})

	// State messages, both directions (seq present downstream only).
	set := wire.Set{Key: "lasa.entity.e1.mute", Value: []byte(`{"muted":true}`), Seq: 4242}
	clear := wire.Clear{Key: "lasa.entity.e1.mute", Seq: 4243}
	add("state-set-up", must(wire.AppendStateMessage(nil, set, wire.Upstream)), map[string]any{"key": set.Key})
	add("state-set-down", must(wire.AppendStateMessage(nil, set, wire.Downstream)), map[string]any{"key": set.Key, "seq": set.Seq})
	add("state-clear-up", must(wire.AppendStateMessage(nil, clear, wire.Upstream)), map[string]any{"key": clear.Key})
	add("state-clear-down", must(wire.AppendStateMessage(nil, clear, wire.Downstream)), map[string]any{"key": clear.Key, "seq": clear.Seq})
	group := wire.Group{Ops: []wire.StateMessage{set, clear}}
	add("state-group-down", must(wire.AppendStateMessage(nil, group, wire.Downstream)), map[string]any{"count": 2})
	epoch := [wire.EpochSize]byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04}
	add("state-frontier", must(wire.AppendStateMessage(nil, wire.Frontier{Epoch: epoch, Seq: 99999}, wire.Downstream)),
		map[string]any{"epoch": hex.EncodeToString(epoch[:]), "seq": 99999})
	add("state-begin-delta", must(wire.AppendStateMessage(nil, wire.Begin{Kind: wire.BeginDelta}, wire.Downstream)), nil)
	add("state-begin-snapshot", must(wire.AppendStateMessage(nil, wire.Begin{Kind: wire.BeginSnapshot}, wire.Downstream)), nil)

	// Subscribe params: fresh (no cursor), and resuming with a cursor.
	// Prefixes deliberately unsorted with a duplicate — the encoder
	// canonicalizes.
	prefixes := []string{"lasa.space.", "lasa.entity.", "lasa.space."}
	add("subparams-fresh", must(wire.AppendSubscribeParams(nil, &wire.SubscribeParams{Prefixes: prefixes})),
		map[string]any{"prefixes": wire.CanonicalPrefixes(prefixes)})
	cur := &wire.Cursor{Epoch: epoch, Seq: 4242, SetHash: wire.SetHash(wire.CanonicalPrefixes(prefixes))}
	add("subparams-cursor", must(wire.AppendSubscribeParams(nil, &wire.SubscribeParams{Prefixes: prefixes, Cursor: cur})),
		map[string]any{"seq": cur.Seq, "sethash": hex.EncodeToString(cur.SetHash[:])})

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}
