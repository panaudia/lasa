package base

// The §2 roles expansion: roles are compact aliases over the core's
// prefix-rule machinery — this file turns a resolved connection's roles
// into its effective state-access rules, plus the two grants that live
// outside the rules machinery (ad-hoc admission, sink subscription).

import (
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/state"
)

// The five base-profile roles (§2). A ticket with an absent or empty
// roles claim, and every unticketed connection, is granted exactly
// audience.
const (
	RoleAdmin     = "admin"
	RolePresenter = "presenter"
	RoleModerator = "moderator"
	RoleProducer  = "producer"
	RoleAudience  = "audience"
)

// AdHocPermitted reports the §2 connect-time admission privilege: an
// admin or presenter ticket may carry ad-hoc entity definitions directly
// in the Connection Config (lasa-core.md §4.2).
func AdHocPermitted(roles []string) bool {
	return hasRole(roles, RoleAdmin) || hasRole(roles, RolePresenter)
}

// SinkAll reports the §2 subscribe-to-any-sink grant ("listening
// through e's ears"): admin, and producer as part of the mixing desk.
func SinkAll(roles []string) bool {
	return hasRole(roles, RoleAdmin) || hasRole(roles, RoleProducer)
}

// RulesFor expands a resolved connection's roles into its effective
// state-access rules: the union of its roles' §2 rules and its ticket's
// explicit permissions grants. Every connection carries audience;
// presenter adds no state grants (its privilege is admission-time);
// admin is the union of all roles. The any-entity grants of moderator
// and producer expand to *-segment scopes (lasa-core.md §6.4);
// audience's own-entity grants expand per admitted entity, which is
// sound because a connection's entity set is fixed at admission.
// Application space follows §2: unrestricted for unticketed
// connections, permissions-only for ticketed ones.
func RulesFor(res *connect.Resolved) state.Rules {
	r := state.Rules{App: !res.Ticketed}

	// audience — every connection: write own attrs/mute/solo, read the
	// whole vocabulary (nothing in it is secret).
	for _, e := range res.Entities {
		own := []string{
			EntityKey(e.ID, "attrs"),
			EntityKey(e.ID, "mute"),
			EntityKey(e.ID, "solo"),
		}
		r.Set = append(r.Set, own...)
		r.Clear = append(r.Clear, own...)
	}
	r.Read = append(r.Read, "lasa.entity", "lasa.space")

	if hasRole(res.Roles, RoleModerator) || hasRole(res.Roles, RoleAdmin) {
		r.Set = append(r.Set, DofKey("*")) // set only: dof has no clear in §2
		mod := []string{"lasa.space.entity.muted", "lasa.space.client.blocked"}
		r.Set = append(r.Set, mod...)
		r.Clear = append(r.Clear, mod...)
	}

	if hasRole(res.Roles, RoleProducer) || hasRole(res.Roles, RoleAdmin) {
		prod := []string{
			EntityKey("*", "sources"),
			EntityKey("*", "sinks"),
			EntityKey("*", "priority"),
			RenderGainKey("*"),
			RenderAttenuationKey("*"),
			RenderSizeKey("*"),
			RenderDirectivityKey("*"),
			"lasa.space.role.blocked",
			"lasa.space.channel.muted",
			"lasa.space.channel.gain",
			"lasa.space.channel.attenuation",
		}
		r.Set = append(r.Set, prod...)
		r.Clear = append(r.Clear, prod...)
	}

	// Ticket permissions: fine-grained grants without the full role,
	// any prefix including lasa. (§2).
	r.Set = append(r.Set, res.Permissions.Set...)
	r.Clear = append(r.Clear, res.Permissions.Clear...)
	r.Read = append(r.Read, res.Permissions.Read...)
	return r
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
