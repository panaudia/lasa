package ident

import "fmt"

// Direction distinguishes the two per-entity / per-client namespace
// directions. Directions are always entity/client-centric: source is what
// the entity or client emits, sink is what it receives (lasa-core.md §3).
type Direction uint8

const (
	DirectionSource Direction = iota
	DirectionSink
)

func (d Direction) String() string {
	switch d {
	case DirectionSource:
		return "source"
	case DirectionSink:
		return "sink"
	}
	return fmt.Sprintf("Direction(%d)", uint8(d))
}

// Kind is the shape of a namespace tuple (lasa-core.md §3).
type Kind uint8

const (
	// KindEntity is (space, "entity", entity-id, direction): per-entity media.
	KindEntity Kind = iota
	// KindClient is (space, "client", client-id, direction): per-client state.
	KindClient
	// KindPresence is (space, "presence"): the one space-wide broadcast.
	KindPresence
)

func (k Kind) String() string {
	switch k {
	case KindEntity:
		return "entity"
	case KindClient:
		return "client"
	case KindPresence:
		return "presence"
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Track names. A track name pins a complete wire format; a changed layout is
// a new track name, never a version byte (lasa-core.md §3).
const (
	TrackMonoObject = "mono-object"
	TrackBinaural   = "binaural"
	TrackAmbi2      = "ambi2"
	TrackAmbi3      = "ambi3"
	TrackState      = "state"
	TrackPresence   = "presence"
)

// Namespace is a parsed LASA namespace tuple. For KindPresence, ID and
// Direction are unused.
type Namespace struct {
	Space     string
	Kind      Kind
	ID        string
	Direction Direction
}

// EntityNamespace builds a per-entity media namespace.
func EntityNamespace(space, entity string, dir Direction) Namespace {
	return Namespace{Space: space, Kind: KindEntity, ID: entity, Direction: dir}
}

// ClientNamespace builds a per-client state namespace.
func ClientNamespace(space, client string, dir Direction) Namespace {
	return Namespace{Space: space, Kind: KindClient, ID: client, Direction: dir}
}

// PresenceNamespace builds the space-wide presence namespace.
func PresenceNamespace(space string) Namespace {
	return Namespace{Space: space, Kind: KindPresence}
}

// Tuple renders the namespace as a MoQ namespace tuple.
func (n Namespace) Tuple() []string {
	switch n.Kind {
	case KindPresence:
		return []string{n.Space, "presence"}
	default:
		return []string{n.Space, n.Kind.String(), n.ID, n.Direction.String()}
	}
}

// ParseNamespace parses a MoQ namespace tuple into a Namespace. Every
// element is validated against the identifier grammar.
func ParseNamespace(tuple []string) (Namespace, error) {
	switch len(tuple) {
	case 2:
		if err := CheckIdentifier(tuple[0]); err != nil {
			return Namespace{}, fmt.Errorf("space: %w", err)
		}
		if tuple[1] != "presence" {
			return Namespace{}, fmt.Errorf("unknown 2-element namespace kind %q", tuple[1])
		}
		return PresenceNamespace(tuple[0]), nil
	case 4:
		if err := CheckIdentifier(tuple[0]); err != nil {
			return Namespace{}, fmt.Errorf("space: %w", err)
		}
		var kind Kind
		switch tuple[1] {
		case "entity":
			kind = KindEntity
		case "client":
			kind = KindClient
		default:
			return Namespace{}, fmt.Errorf("unknown namespace kind %q", tuple[1])
		}
		if err := CheckIdentifier(tuple[2]); err != nil {
			return Namespace{}, fmt.Errorf("id: %w", err)
		}
		var dir Direction
		switch tuple[3] {
		case "source":
			dir = DirectionSource
		case "sink":
			dir = DirectionSink
		default:
			return Namespace{}, fmt.Errorf("unknown direction %q", tuple[3])
		}
		return Namespace{Space: tuple[0], Kind: kind, ID: tuple[2], Direction: dir}, nil
	default:
		return Namespace{}, fmt.Errorf("namespace tuple must have 2 or 4 elements, got %d", len(tuple))
	}
}

// ValidTrack reports whether name is a defined track name on namespace n,
// per the track table of lasa-core.md §3.
func ValidTrack(n Namespace, name string) bool {
	switch n.Kind {
	case KindEntity:
		if n.Direction == DirectionSource {
			return name == TrackMonoObject
		}
		return name == TrackBinaural || name == TrackAmbi2 || name == TrackAmbi3
	case KindClient:
		return name == TrackState
	case KindPresence:
		return name == TrackPresence
	}
	return false
}
