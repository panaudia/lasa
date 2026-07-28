package ident

// Binding is the set of identities a connection established at admission
// (lasa-core.md §4.1): the space it connected to, its client id, and the
// entity ids it owns. Subscribe and announce authorization is enforced at
// one point, against this binding (lasa-core.md §3.1).
type Binding struct {
	Space    string
	Client   string
	Entities []string

	// SinkAll grants subscription to any entity's sink tracks — profile
	// policy beyond the owner (the base profile grants it to the admin
	// role, "listening through e's ears").
	SinkAll bool
}

func (b *Binding) ownsEntity(id string) bool {
	for _, e := range b.Entities {
		if e == id {
			return true
		}
	}
	return false
}

// MaySubscribe reports whether the connection holding this binding may
// subscribe to the given namespace, per the authorization table of
// lasa-core.md §3.1. The track name is checked for validity on the
// namespace; source namespaces are subscribable by the space only, so
// MaySubscribe always denies them for a client binding.
func (b *Binding) MaySubscribe(n Namespace, track string) bool {
	if n.Space != b.Space || !ValidTrack(n, track) {
		return false
	}
	switch n.Kind {
	case KindPresence:
		return true
	case KindEntity:
		if n.Direction != DirectionSink {
			return false
		}
		return b.SinkAll || b.ownsEntity(n.ID)
	case KindClient:
		return n.Direction == DirectionSink && n.ID == b.Client
	}
	return false
}

// MayAnnounce reports whether the connection holding this binding may
// announce the given namespace: a client may announce only its own source
// namespaces (lasa-core.md §3.1).
func (b *Binding) MayAnnounce(n Namespace) bool {
	if n.Space != b.Space {
		return false
	}
	switch n.Kind {
	case KindEntity:
		return n.Direction == DirectionSource && b.ownsEntity(n.ID)
	case KindClient:
		return n.Direction == DirectionSource && n.ID == b.Client
	}
	return false
}
