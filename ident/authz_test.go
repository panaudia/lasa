package ident

import "testing"

// TestAuthzMatrix exercises the full subscribe/announce authorization table
// of lasa-core.md §3.1 against a binding owning client c1 and entities
// e1, e2 in space s.
func TestAuthzMatrix(t *testing.T) {
	b := &Binding{Space: "s", Client: "c1", Entities: []string{"e1", "e2"}}

	subCases := []struct {
		name  string
		ns    Namespace
		track string
		want  bool
	}{
		{"own entity sink binaural", EntityNamespace("s", "e1", DirectionSink), TrackBinaural, true},
		// Ambi formats need their own grant (2026-08-04) — refused even
		// for the owner on an ungranted binding; TestAmbiFormatsGrant
		// covers the granted side.
		{"own entity sink ambi2 ungranted", EntityNamespace("s", "e2", DirectionSink), TrackAmbi2, false},
		{"own entity sink ambi3 ungranted", EntityNamespace("s", "e1", DirectionSink), TrackAmbi3, false},
		{"other entity sink", EntityNamespace("s", "e3", DirectionSink), TrackBinaural, false},
		{"own entity source", EntityNamespace("s", "e1", DirectionSource), TrackMonoObject, false},
		{"own client state sink", ClientNamespace("s", "c1", DirectionSink), TrackState, true},
		{"other client state sink", ClientNamespace("s", "c2", DirectionSink), TrackState, false},
		{"own client state source", ClientNamespace("s", "c1", DirectionSource), TrackState, false},
		{"presence", PresenceNamespace("s"), TrackPresence, true},
		{"wrong space presence", PresenceNamespace("other"), TrackPresence, false},
		{"wrong space own sink", EntityNamespace("other", "e1", DirectionSink), TrackBinaural, false},
		{"wrong track on sink", EntityNamespace("s", "e1", DirectionSink), TrackMonoObject, false},
		{"wrong track on presence", PresenceNamespace("s"), TrackState, false},
	}
	for _, c := range subCases {
		if got := b.MaySubscribe(c.ns, c.track); got != c.want {
			t.Errorf("MaySubscribe %s: got %v, want %v", c.name, got, c.want)
		}
	}

	annCases := []struct {
		name string
		ns   Namespace
		want bool
	}{
		{"own entity source", EntityNamespace("s", "e1", DirectionSource), true},
		{"own client source", ClientNamespace("s", "c1", DirectionSource), true},
		{"other entity source", EntityNamespace("s", "e3", DirectionSource), false},
		{"other client source", ClientNamespace("s", "c2", DirectionSource), false},
		{"own entity sink", EntityNamespace("s", "e1", DirectionSink), false},
		{"own client sink", ClientNamespace("s", "c1", DirectionSink), false},
		{"presence", PresenceNamespace("s"), false},
		{"wrong space", EntityNamespace("other", "e1", DirectionSource), false},
	}
	for _, c := range annCases {
		if got := b.MayAnnounce(c.ns); got != c.want {
			t.Errorf("MayAnnounce %s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestSinkAllGrant covers the admin "listening through e's ears" extra grant.
func TestSinkAllGrant(t *testing.T) {
	b := &Binding{Space: "s", Client: "c1", Entities: nil, SinkAll: true}
	if !b.MaySubscribe(EntityNamespace("s", "e9", DirectionSink), TrackBinaural) {
		t.Error("SinkAll binding must subscribe any entity sink")
	}
	if b.MaySubscribe(EntityNamespace("s", "e9", DirectionSource), TrackMonoObject) {
		t.Error("SinkAll must not grant source subscription")
	}
	if b.MaySubscribe(ClientNamespace("s", "c2", DirectionSink), TrackState) {
		t.Error("SinkAll must not grant other clients' state sinks")
	}
}

// TestAmbiFormatsGrant: the ambisonic sink formats need their own grant
// (base profile: admin/producer, 2026-08-04) on top of the ownership
// rule — and the grant does not widen which entities may be subscribed.
func TestAmbiFormatsGrant(t *testing.T) {
	own := &Binding{Space: "s", Client: "c1", Entities: []string{"e1"}}
	if own.MaySubscribe(EntityNamespace("s", "e1", DirectionSink), TrackAmbi2) {
		t.Error("ambi2 without the grant must be refused, even for the owner")
	}
	if own.MaySubscribe(EntityNamespace("s", "e1", DirectionSink), TrackAmbi3) {
		t.Error("ambi3 without the grant must be refused, even for the owner")
	}
	if !own.MaySubscribe(EntityNamespace("s", "e1", DirectionSink), TrackBinaural) {
		t.Error("binaural must not need the ambi grant")
	}

	granted := &Binding{Space: "s", Client: "c1", Entities: []string{"e1"}, AmbiFormats: true}
	if !granted.MaySubscribe(EntityNamespace("s", "e1", DirectionSink), TrackAmbi3) {
		t.Error("granted owner must subscribe own ambi3")
	}
	if granted.MaySubscribe(EntityNamespace("s", "e9", DirectionSink), TrackAmbi3) {
		t.Error("the ambi grant alone must not widen entity reach (no SinkAll)")
	}

	full := &Binding{Space: "s", Client: "c1", SinkAll: true, AmbiFormats: true}
	if !full.MaySubscribe(EntityNamespace("s", "e9", DirectionSink), TrackAmbi2) {
		t.Error("SinkAll+AmbiFormats must subscribe any entity's ambi")
	}
}

// TestZeroEntityBinding covers the observer/console pattern: a zero-entity
// connection may subscribe presence, its own state sink, and nothing else
// by default (lasa-core.md §4.2).
func TestZeroEntityBinding(t *testing.T) {
	b := &Binding{Space: "s", Client: "c1"}
	if !b.MaySubscribe(PresenceNamespace("s"), TrackPresence) {
		t.Error("observer must subscribe presence")
	}
	if !b.MaySubscribe(ClientNamespace("s", "c1", DirectionSink), TrackState) {
		t.Error("observer must subscribe own state sink")
	}
	if b.MaySubscribe(EntityNamespace("s", "e1", DirectionSink), TrackBinaural) {
		t.Error("observer must not subscribe entity sinks without a grant")
	}
	if b.MayAnnounce(ClientNamespace("s", "c1", DirectionSource)) != true {
		t.Error("observer may announce its own state source")
	}
}
