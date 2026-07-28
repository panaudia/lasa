package ident

import (
	"reflect"
	"testing"
)

func TestNamespaceTupleRoundTrip(t *testing.T) {
	cases := []Namespace{
		EntityNamespace("space1", "e1", DirectionSource),
		EntityNamespace("space1", "e1", DirectionSink),
		ClientNamespace("space1", "c1", DirectionSource),
		ClientNamespace("space1", "c1", DirectionSink),
		PresenceNamespace("space1"),
	}
	for _, want := range cases {
		got, err := ParseNamespace(want.Tuple())
		if err != nil {
			t.Fatalf("ParseNamespace(%v): %v", want.Tuple(), err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip %v: got %+v, want %+v", want.Tuple(), got, want)
		}
	}
}

func TestParseNamespaceRejects(t *testing.T) {
	bad := [][]string{
		nil,
		{},
		{"space1"},
		{"space1", "presence", "extra"},
		{"space1", "entity", "e1"},
		{"space1", "entity", "e1", "source", "extra"},
		{"space1", "unknown", "e1", "source"},
		{"space1", "entity", "e1", "up"},
		{"space1", "entity", "E1", "source"}, // invalid entity id
		{"Space1", "entity", "e1", "source"}, // invalid space id
		{"space1", "Presence"},               // wrong case
		{"space1", "entity", "", "source"},   // empty id
	}
	for _, tuple := range bad {
		if _, err := ParseNamespace(tuple); err == nil {
			t.Errorf("ParseNamespace(%v): want error, got nil", tuple)
		}
	}
}

func TestValidTrack(t *testing.T) {
	cases := []struct {
		ns    Namespace
		track string
		want  bool
	}{
		{EntityNamespace("s", "e", DirectionSource), TrackMonoObject, true},
		{EntityNamespace("s", "e", DirectionSource), TrackBinaural, false},
		{EntityNamespace("s", "e", DirectionSink), TrackBinaural, true},
		{EntityNamespace("s", "e", DirectionSink), TrackAmbi2, true},
		{EntityNamespace("s", "e", DirectionSink), TrackAmbi3, true},
		{EntityNamespace("s", "e", DirectionSink), TrackMonoObject, false},
		{ClientNamespace("s", "c", DirectionSource), TrackState, true},
		{ClientNamespace("s", "c", DirectionSink), TrackState, true},
		{ClientNamespace("s", "c", DirectionSink), TrackPresence, false},
		{PresenceNamespace("s"), TrackPresence, true},
		{PresenceNamespace("s"), TrackState, false},
	}
	for _, c := range cases {
		if got := ValidTrack(c.ns, c.track); got != c.want {
			t.Errorf("ValidTrack(%v, %q) = %v, want %v", c.ns.Tuple(), c.track, got, c.want)
		}
	}
}
