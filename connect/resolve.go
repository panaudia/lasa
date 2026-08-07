package connect

import (
	"github.com/panaudia/lasa/wire"
)

// ResolvedEntity is one entity admitted on a connection. Signed reports
// whether the definition came from inside the ticket (a setup) or ad-hoc
// from the config. For a signed entity, SetupAttrs carries the setup's
// unsigned runtime attrs separately from the definition's signed ones,
// and the setup's initial pose (if any) has already overridden the
// definition's default.
type ResolvedEntity struct {
	Entity
	Signed     bool
	SetupAttrs map[string]any
}

// Resolved is the outcome of admission resolution: the connection's
// identity, its admitted entities, and the ticket-derived access inputs.
// Roles are carried opaquely as the ticket presented them; defaulting an
// absent/empty roles claim to "audience" is base-profile behaviour and
// happens in the profile layer.
type Resolved struct {
	ClientID    string
	Ticketed    bool
	Roles       []string
	Permissions Permissions
	Entities    []ResolvedEntity
	// Transport is the connection's transport kind, stamped by the
	// server shell after resolution: "quic" (native clients) or
	// "webtransport" (browsers can use nothing else). Backends may use
	// it as a route signal — e.g. sizing ingest jitter bounds to the
	// sender class. Empty in unit contexts that never touch a
	// transport.
	Transport string
}

// Resolve applies the entity/setup rules of lasa-core.md §4.2 to a parsed
// config and its verified ticket (nil for an unticketed connection).
// adHocPermitted reports whether this connection may carry ad-hoc entity
// definitions despite being ticketed — profile policy (base profile:
// admin and presenter), decided by the caller; it is ignored for
// unticketed connections, which always may.
func Resolve(cfg *ConnectionConfig, ticket *Ticket, adHocPermitted bool) (*Resolved, *Rejection) {
	if !cfg.HasEntities() && !cfg.HasSetups() {
		return nil, rejectMalformed("config must carry at least one of entities/setups")
	}
	if cfg.HasSetups() && ticket == nil {
		return nil, rejectMalformed("setups require a ticket")
	}
	if ticket != nil && cfg.ClientID != ticket.ClientID {
		return nil, rejectf(wire.ErrCodeClientIDMismatch,
			"client_id %q does not match ticket claim %q", cfg.ClientID, ticket.ClientID)
	}

	r := &Resolved{ClientID: cfg.ClientID, Ticketed: ticket != nil}
	if ticket != nil {
		r.Roles = ticket.Roles
		r.Permissions = ticket.Permissions
	}

	// Ticket-defined entities, by id.
	signed := map[string]*Entity{}
	if ticket != nil {
		for i := range ticket.Entities {
			e := &ticket.Entities[i]
			if signed[e.ID] != nil {
				return nil, rejectTicket("duplicate entity id %q in ticket", e.ID)
			}
			signed[e.ID] = e
		}
	}

	// Setups instantiate ticket-defined entities; instantiating a subset
	// is permitted; unknown or duplicate ids reject.
	seen := map[string]bool{}
	for _, s := range cfg.Setups {
		if seen[s.ID] {
			return nil, rejectMalformed("duplicate setup id %q", s.ID)
		}
		seen[s.ID] = true
		def := signed[s.ID]
		if def == nil {
			return nil, rejectMalformed("setup id %q references no entity in the ticket", s.ID)
		}
		e := *def
		if s.InitialPose != nil {
			e.InitialPose = s.InitialPose
		}
		r.Entities = append(r.Entities, ResolvedEntity{Entity: e, Signed: true, SetupAttrs: s.Attrs})
	}

	// Ad-hoc entities: admitted when the connection is unticketed, or when
	// the ticket authorizes it. Ad-hoc ids cannot shadow signed
	// definitions, whether instantiated or not.
	if len(cfg.Entities) > 0 && ticket != nil && !adHocPermitted {
		return nil, rejectf(wire.ErrCodeAdHocNotPermitted,
			"ad-hoc entities not permitted for this ticket")
	}
	for i := range cfg.Entities {
		e := &cfg.Entities[i]
		if seen[e.ID] {
			return nil, rejectMalformed("duplicate entity id %q", e.ID)
		}
		if signed[e.ID] != nil {
			return nil, rejectMalformed("ad-hoc entity id %q shadows a ticket definition", e.ID)
		}
		seen[e.ID] = true
		r.Entities = append(r.Entities, ResolvedEntity{Entity: *e})
	}

	return r, nil
}
