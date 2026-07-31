package base

import "testing"

func fp(v float64) *float64 { return &v }

// The §4 rules, line by line.
func TestComposeAgainstProfileText(t *testing.T) {
	cases := []struct {
		name       string
		entityGain float64
		entityAtt  float64
		shared     []ChannelState
		wantGain   float64
		wantAtt    float64
		audible    bool
	}{
		{"no shared channel: inaudible",
			1, 2, nil, 0, 2, false},
		{"all shared muted: inaudible (muted contributes nothing)",
			1, 2, []ChannelState{{Muted: true, Gain: fp(3)}}, 0, 2, false},
		{"default is a no-op: unset channel, entity values pass through",
			0.8, 2.3, []ChannelState{{}}, 0.8, 2.3, true},
		{"gain composes multiplicatively",
			0.5, 2, []ChannelState{{Gain: fp(2.0)}}, 1.0, 2, true},
		{"composed value may exceed the per-key range",
			3, 2, []ChannelState{{Gain: fp(3)}}, 9, 2, true},
		{"highest gain wins across shared channels",
			1, 2, []ChannelState{{Gain: fp(0.5)}, {Gain: fp(1.5)}}, 1.5, 2, true},
		{"an unrestricted channel cannot be attenuated by another's fader",
			1, 2, []ChannelState{{Gain: fp(0.5)}, {}}, 1.0, 2, true},
		{"a muted channel's gain never competes",
			1, 2, []ChannelState{{Muted: true, Gain: fp(3)}, {Gain: fp(0.5)}}, 0.5, 2, true},
		{"attenuation overrides when set",
			1, 2, []ChannelState{{Attenuation: fp(0.5)}}, 1, 0.5, true},
		{"lowest set attenuation wins",
			1, 2, []ChannelState{{Attenuation: fp(1.5)}, {Attenuation: fp(0.5)}}, 1, 0.5, true},
		{"unset attenuation imposes no policy: entity value stands",
			1, 2.7, []ChannelState{{Gain: fp(2)}}, 2, 2.7, true},
		{"whisper via channel: attenuation 3 overrides entity 2",
			1, 2, []ChannelState{{Attenuation: fp(3)}}, 1, 3, true},
	}
	for _, c := range cases {
		gain, att, audible := EffectiveRender(c.entityGain, c.entityAtt, c.shared)
		if gain != c.wantGain || att != c.wantAtt || audible != c.audible {
			t.Errorf("%s: got (%v, %v, %v) want (%v, %v, %v)",
				c.name, gain, att, audible, c.wantGain, c.wantAtt, c.audible)
		}
	}
}
