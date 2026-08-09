package base

// The consumer vocabulary: typed parsing of base-profile state ops
// (design F: vocabulary in the profile, wiring in the server,
// mechanism in the engine). A profile consumer subscribes to `lasa.`
// prefixes (state.Engine.Observe) and feeds each op through ParseOp;
// the returned event carries final values with the documented
// absence-means-default semantics already applied — a cleared
// render.gain arrives as gain 1.0, a cleared channel attenuation as
// "no override".
//
// The defaults here are the profile's documented values as code; the
// embedder's engine owns its own defaults, and the two are held equal
// by a parity test in the server that imports both (the libraries
// never import each other).

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Documented defaults (§6 semantics; absence means these).
const (
	DefaultGain        = 1.0
	DefaultAttenuation = 2.0
	DefaultSize        = 0.0
	DefaultDirectivity = 0.0
	DefaultDof         = 6
	DefaultHeadFrame   = false // frame: "world"
	DefaultChannelGain = 1.0   // unset channel gain is the identity (§4)
)

// DefaultChannels is the membership an entity whose config omits
// heard-in (resp. hears) receives (§4). An explicit empty list means
// deliberately none.
func DefaultChannels() []string { return []string{"main"} }

// ErrInvalidValue marks a recognised key carrying a value outside the
// profile's documented range or type. Consumers drop such ops (the
// profile's "dropped like any other invalid value" rule).
var ErrInvalidValue = errors.New("base: invalid value for profile key")

// Event is a typed profile op. A nil Event (with nil error) means the
// key is not part of the consumer vocabulary (markers, names, attrs,
// application keys) — not an error.
type Event interface{ profileEvent() }

type RenderField int

const (
	FieldGain RenderField = iota
	FieldAttenuation
	FieldSize
	FieldDirectivity
)

// EntityRender: a render.* change; Value has the default applied on
// clear.
type EntityRender struct {
	EntityID string
	Field    RenderField
	Value    float64
}

// ChannelMembership: entity joins/leaves a channel on the heard-in
// (being heard) or hears (listening) side.
type ChannelMembership struct {
	EntityID string
	Channel  string
	Hears    bool // false: heard-in.*, true: hears.*
	Member   bool
}

// EntityMuted: the space-wide moderation mute (§5 — rendering only).
type EntityMuted struct {
	EntityID string
	Muted    bool
}

// PersonalMute / PersonalSolo: owner-scoped listener preferences (§6).
type PersonalMute struct {
	OwnerID, OtherID string
	On               bool
}
type PersonalSolo struct {
	OwnerID, OtherID string
	On               bool
}

// Dof: degrees of freedom, enforced at ingest (§6).
type Dof struct {
	EntityID string
	Value    int
}

// Frame: the entity's reference frame (§6).
type Frame struct {
	EntityID  string
	HeadFrame bool
}

// ChannelMuted / ChannelGain / ChannelAttenuation: the space-scoped
// channel table (§4). Attenuation nil = no override (unset is not a
// number).
type ChannelMuted struct {
	Channel string
	Muted   bool
}
type ChannelGain struct {
	Channel string
	Gain    float64
}
type ChannelAttenuation struct {
	Channel     string
	Attenuation *float64
}

// ClientBlocked / RoleBlocked: §5 moderation (0 = forever, otherwise
// blocked until the unix-seconds timestamp; Blocked false = cleared).
type ClientBlocked struct {
	ClientID string
	Blocked  bool
	Until    int64
}
type RoleBlocked struct {
	RoleName string
	Blocked  bool
	Until    int64
}

func (EntityRender) profileEvent()       {}
func (ChannelMembership) profileEvent()  {}
func (EntityMuted) profileEvent()        {}
func (PersonalMute) profileEvent()       {}
func (PersonalSolo) profileEvent()       {}
func (Dof) profileEvent()                {}
func (Frame) profileEvent()              {}
func (ChannelMuted) profileEvent()       {}
func (ChannelGain) profileEvent()        {}
func (ChannelAttenuation) profileEvent() {}
func (ClientBlocked) profileEvent()      {}
func (RoleBlocked) profileEvent()        {}

// ParseOp classifies one state op. cleared ops resolve to the
// documented default; recognised keys with invalid values return
// ErrInvalidValue (drop and count); keys outside the consumer
// vocabulary return (nil, nil).
func ParseOp(key string, value []byte, cleared bool) (Event, error) {
	switch {
	case strings.HasPrefix(key, "lasa.entity."):
		return parseEntityOp(key[len("lasa.entity."):], value, cleared)
	case strings.HasPrefix(key, "lasa.space."):
		return parseSpaceOp(key[len("lasa.space."):], value, cleared)
	}
	return nil, nil
}

func parseEntityOp(rest string, value []byte, cleared bool) (Event, error) {
	id, sub, ok := strings.Cut(rest, ".")
	if !ok {
		return nil, nil // the bare existence marker
	}
	switch {
	case strings.HasPrefix(sub, "render."):
		return parseRender(id, sub[len("render."):], value, cleared)
	case strings.HasPrefix(sub, "heard-in."):
		return parseMembership(id, sub[len("heard-in."):], false, value, cleared)
	case strings.HasPrefix(sub, "hears."):
		return parseMembership(id, sub[len("hears."):], true, value, cleared)
	case strings.HasPrefix(sub, "mute."):
		on, err := parseFlag(value, cleared)
		return PersonalMute{OwnerID: id, OtherID: sub[len("mute."):], On: on}, err
	case strings.HasPrefix(sub, "solo."):
		on, err := parseFlag(value, cleared)
		return PersonalSolo{OwnerID: id, OtherID: sub[len("solo."):], On: on}, err
	case sub == "dof":
		if cleared {
			return Dof{EntityID: id, Value: DefaultDof}, nil
		}
		n, err := parseNumber(value)
		if err != nil || (n != 0 && n != 3 && n != 6) {
			return nil, fmt.Errorf("%w: dof %q", ErrInvalidValue, value)
		}
		return Dof{EntityID: id, Value: int(n)}, nil
	case sub == "frame":
		if cleared {
			return Frame{EntityID: id, HeadFrame: DefaultHeadFrame}, nil
		}
		switch string(value) {
		case `"world"`:
			return Frame{EntityID: id, HeadFrame: false}, nil
		case `"head"`:
			return Frame{EntityID: id, HeadFrame: true}, nil
		}
		return nil, fmt.Errorf("%w: frame %q", ErrInvalidValue, value)
	}
	return nil, nil // name, attrs, signed-attrs, priority, initial-pose…
}

func parseRender(id, field string, value []byte, cleared bool) (Event, error) {
	var f RenderField
	var def, lo, hi float64
	switch field {
	case "gain":
		f, def, lo, hi = FieldGain, DefaultGain, 0, 3
	case "attenuation":
		f, def, lo, hi = FieldAttenuation, DefaultAttenuation, 0, 3
	case "size":
		f, def, lo, hi = FieldSize, DefaultSize, 0, 1e9
	case "directivity":
		f, def, lo, hi = FieldDirectivity, DefaultDirectivity, 0, 1
	default:
		return nil, nil
	}
	if cleared {
		return EntityRender{EntityID: id, Field: f, Value: def}, nil
	}
	n, err := parseNumber(value)
	if err != nil || n < lo || n > hi {
		return nil, fmt.Errorf("%w: render.%s %q", ErrInvalidValue, field, value)
	}
	return EntityRender{EntityID: id, Field: f, Value: n}, nil
}

func parseMembership(id, channel string, hears bool, value []byte, cleared bool) (Event, error) {
	member, err := parseFlag(value, cleared)
	return ChannelMembership{EntityID: id, Channel: channel, Hears: hears, Member: member}, err
}

func parseSpaceOp(rest string, value []byte, cleared bool) (Event, error) {
	switch {
	case strings.HasPrefix(rest, "entity.muted."):
		muted, err := parseFlag(value, cleared)
		return EntityMuted{EntityID: rest[len("entity.muted."):], Muted: muted}, err
	case strings.HasPrefix(rest, "client.blocked."):
		until, blocked, err := parseBlocked(value, cleared)
		return ClientBlocked{ClientID: rest[len("client.blocked."):], Blocked: blocked, Until: until}, err
	case strings.HasPrefix(rest, "role.blocked."):
		role := rest[len("role.blocked."):]
		if role == "admin" && !cleared {
			return nil, fmt.Errorf("%w: admin is unblockable (§5)", ErrInvalidValue)
		}
		until, blocked, err := parseBlocked(value, cleared)
		return RoleBlocked{RoleName: role, Blocked: blocked, Until: until}, err
	case strings.HasPrefix(rest, "channel.muted."):
		muted, err := parseFlag(value, cleared)
		return ChannelMuted{Channel: rest[len("channel.muted."):], Muted: muted}, err
	case strings.HasPrefix(rest, "channel.gain."):
		ch := rest[len("channel.gain."):]
		if cleared {
			return ChannelGain{Channel: ch, Gain: DefaultChannelGain}, nil
		}
		n, err := parseNumber(value)
		if err != nil || n < 0 || n > 3 {
			return nil, fmt.Errorf("%w: channel.gain %q", ErrInvalidValue, value)
		}
		return ChannelGain{Channel: ch, Gain: n}, nil
	case strings.HasPrefix(rest, "channel.attenuation."):
		ch := rest[len("channel.attenuation."):]
		if cleared {
			return ChannelAttenuation{Channel: ch, Attenuation: nil}, nil
		}
		n, err := parseNumber(value)
		if err != nil || n < 0 || n > 3 {
			return nil, fmt.Errorf("%w: channel.attenuation %q", ErrInvalidValue, value)
		}
		return ChannelAttenuation{Channel: ch, Attenuation: &n}, nil
	}
	return nil, nil
}

// parseFlag: presence-style keys carry literally `true` (§6 tables);
// cleared means absent/off.
func parseFlag(value []byte, cleared bool) (bool, error) {
	if cleared {
		return false, nil
	}
	if string(value) != "true" {
		return false, fmt.Errorf("%w: want true, got %q", ErrInvalidValue, value)
	}
	return true, nil
}

func parseNumber(value []byte) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(string(value)), 64)
}

// parseBlocked: Number ≥ 0 (0 = forever, else until unix-seconds).
func parseBlocked(value []byte, cleared bool) (until int64, blocked bool, err error) {
	if cleared {
		return 0, false, nil
	}
	n, perr := parseNumber(value)
	if perr != nil || n < 0 {
		return 0, false, fmt.Errorf("%w: blocked %q", ErrInvalidValue, value)
	}
	return int64(n), true, nil
}
