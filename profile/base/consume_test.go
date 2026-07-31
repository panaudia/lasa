package base

import (
	"errors"
	"testing"
)

func mustEvent(t *testing.T, key, value string, cleared bool) Event {
	t.Helper()
	ev, err := ParseOp(key, []byte(value), cleared)
	if err != nil {
		t.Fatalf("ParseOp(%s, %s): %v", key, value, err)
	}
	return ev
}

func TestParseOpTable(t *testing.T) {
	cases := []struct {
		key, value string
		cleared    bool
		want       Event
	}{
		{RenderGainKey("e1"), "2.5", false, EntityRender{"e1", FieldGain, 2.5}},
		{RenderGainKey("e1"), "", true, EntityRender{"e1", FieldGain, 1.0}},
		{RenderAttenuationKey("e1"), "0", false, EntityRender{"e1", FieldAttenuation, 0}},
		{RenderAttenuationKey("e1"), "", true, EntityRender{"e1", FieldAttenuation, 2.0}},
		{RenderSizeKey("e1"), "1.5", false, EntityRender{"e1", FieldSize, 1.5}},
		{RenderDirectivityKey("e1"), "", true, EntityRender{"e1", FieldDirectivity, 0}},
		{SourceChannelKey("e1", "stage"), "true", false, ChannelMembership{"e1", "stage", false, true}},
		{SinkChannelKey("e1", "stage"), "", true, ChannelMembership{"e1", "stage", true, false}},
		{PersonalMuteKey("e1", "e2"), "true", false, PersonalMute{"e1", "e2", true}},
		{PersonalSoloKey("e1", "e2"), "", true, PersonalSolo{"e1", "e2", false}},
		{DofKey("e1"), "3", false, Dof{"e1", 3}},
		{DofKey("e1"), "", true, Dof{"e1", 6}},
		{FrameKey("e1"), `"head"`, false, Frame{"e1", true}},
		{FrameKey("e1"), "", true, Frame{"e1", false}},
		{SpaceEntityMutedKey("e1"), "true", false, EntityMuted{"e1", true}},
		{ChannelMutedKey("stage"), "true", false, ChannelMuted{"stage", true}},
		{ChannelGainKey("stage"), "1.2", false, ChannelGain{"stage", 1.2}},
		{ChannelGainKey("stage"), "", true, ChannelGain{"stage", 1.0}},
		{ClientBlockedKey("c1"), "0", false, ClientBlocked{"c1", true, 0}},
		{ClientBlockedKey("c1"), "1800000000", false, ClientBlocked{"c1", true, 1800000000}},
		{ClientBlockedKey("c1"), "", true, ClientBlocked{"c1", false, 0}},
		{RoleBlockedKey("noisy"), "0", false, RoleBlocked{"noisy", true, 0}},
	}
	for _, c := range cases {
		if got := mustEvent(t, c.key, c.value, c.cleared); got != c.want {
			t.Errorf("%s (%q cleared=%v): got %#v want %#v", c.key, c.value, c.cleared, got, c.want)
		}
	}

	// Channel attenuation: pointer semantics need their own checks.
	ev := mustEvent(t, ChannelAttenuationKey("stage"), "0.5", false).(ChannelAttenuation)
	if ev.Attenuation == nil || *ev.Attenuation != 0.5 {
		t.Fatalf("set attenuation: %#v", ev)
	}
	ev = mustEvent(t, ChannelAttenuationKey("stage"), "", true).(ChannelAttenuation)
	if ev.Attenuation != nil {
		t.Fatal("cleared attenuation must be no-override")
	}
}

func TestParseOpInvalid(t *testing.T) {
	bad := []struct{ key, value string }{
		{RenderGainKey("e1"), "3.5"}, // above range
		{RenderGainKey("e1"), "-1"},  // below range
		{RenderDirectivityKey("e1"), "2"},
		{RenderGainKey("e1"), `"loud"`},        // wrong type
		{SourceChannelKey("e1", "x"), "false"}, // flags are literally true
		{DofKey("e1"), "4"},
		{FrameKey("e1"), `"screen"`},
		{ClientBlockedKey("c1"), "-5"},
		{RoleBlockedKey("admin"), "0"}, // §5: admin unblockable
		{ChannelGainKey("x"), "9"},
	}
	for _, c := range bad {
		if _, err := ParseOp(c.key, []byte(c.value), false); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%s=%q: want ErrInvalidValue, got %v", c.key, c.value, err)
		}
	}
}

func TestParseOpOutsideVocabulary(t *testing.T) {
	for _, key := range []string{
		"lasa.entity.e1",             // marker
		"lasa.entity.e1.name",        // shell-authored
		"lasa.entity.e1.attrs.color", // application
		"lasa.entity.e1.priority",    // not consumed here
		"app.something",              // application space
	} {
		ev, err := ParseOp(key, []byte(`"x"`), false)
		if ev != nil || err != nil {
			t.Errorf("%s: want (nil, nil), got (%#v, %v)", key, ev, err)
		}
	}
}
