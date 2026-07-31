package base

// The §4 channel-composition semantics as code (erratum 11's
// resolution): gain composes multiplicatively, attenuation overrides
// when set. This is the single executable statement of the rules the
// server's pair logic and the engine-parity tests build on.
//
// One reading pinned here (the §4 text's most-audible-wins principle
// applied to the mixed set/unset case): each shared unmuted channel
// contributes a gain candidate — its set value, or the identity 1.0
// when unset — and the highest candidate wins. So a pair sharing a
// turned-down channel AND an unrestricted one hears at unity: another
// channel's fader cannot attenuate a path that a policy-free channel
// grants. Attenuation is asymmetric by nature (an override, not a
// level): only set values compete (lowest wins); no set value means
// no override.

// ChannelState is one channel's table row as the consumer accumulated
// it (ChannelMuted / ChannelGain / ChannelAttenuation events).
type ChannelState struct {
	Muted       bool
	Gain        *float64 // nil = unset (identity)
	Attenuation *float64 // nil = unset (no override)
}

// ResolveShared resolves a (source, sink) pair's shared channels:
// audible iff at least one is unmuted (§4); gain = highest candidate
// among unmuted (set value or identity); attenuation = lowest set
// value among unmuted, nil if none sets one. Muted channels contribute
// nothing to any resolution.
func ResolveShared(shared []ChannelState) (gain float64, attOverride *float64, audible bool) {
	gain = 0
	for _, c := range shared {
		if c.Muted {
			continue
		}
		audible = true
		g := DefaultChannelGain
		if c.Gain != nil {
			g = *c.Gain
		}
		if g > gain {
			gain = g
		}
		if c.Attenuation != nil && (attOverride == nil || *c.Attenuation < *attOverride) {
			v := *c.Attenuation
			attOverride = &v
		}
	}
	if !audible {
		return 0, nil, false
	}
	return gain, attOverride, true
}

// EffectiveRender composes the winners with the entity's own render
// values (§4): gain multiplies; attenuation is replaced only when some
// shared unmuted channel set one.
func EffectiveRender(entityGain, entityAttenuation float64, shared []ChannelState) (gain, attenuation float64, audible bool) {
	chGain, attOverride, audible := ResolveShared(shared)
	if !audible {
		return 0, entityAttenuation, false
	}
	gain = entityGain * chGain
	attenuation = entityAttenuation
	if attOverride != nil {
		attenuation = *attOverride
	}
	return gain, attenuation, true
}
