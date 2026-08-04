package base

import (
	"testing"

	"github.com/panaudia/lasa/connect"
)

func resolved(ticketed bool, roles []string, entityIDs ...string) *connect.Resolved {
	res := &connect.Resolved{ClientID: "c1", Ticketed: ticketed, Roles: roles}
	for _, id := range entityIDs {
		res.Entities = append(res.Entities, connect.ResolvedEntity{Entity: connect.Entity{ID: id}})
	}
	return res
}

// TestRulesFor holds the expansion to the §2 role tables.
func TestRulesFor(t *testing.T) {
	type check struct {
		key              string
		set, clear, read bool
	}
	cases := []struct {
		name   string
		res    *connect.Resolved
		checks []check
	}{
		{
			name: "audience owns its attrs, mute, solo — and nothing else",
			res:  resolved(true, nil, "e1"),
			checks: []check{
				{"lasa.entity.e1.attrs.color", true, true, true},
				{"lasa.entity.e1.mute.e2", true, true, true},
				{"lasa.entity.e1.solo.e2", true, true, true},
				{"lasa.entity.e2.attrs.color", false, false, true}, // not its entity
				{"lasa.entity.e1.name", false, false, true},        // server-only key
				{"lasa.entity.e1.dof", false, false, true},
				{"lasa.entity.e1.render.gain", false, false, true},
				{"lasa.space.entity.muted.e2", false, false, true},
				{"lasa.space.channel.gain.main", false, false, true},
			},
		},
		{
			name: "explicit audience role equals the default",
			res:  resolved(true, []string{RoleAudience}, "e1"),
			checks: []check{
				{"lasa.entity.e1.attrs.color", true, true, true},
				{"lasa.space.entity.muted.e2", false, false, true},
			},
		},
		{
			name: "presenter adds no state grants",
			res:  resolved(true, []string{RolePresenter}, "e1"),
			checks: []check{
				{"lasa.entity.e1.attrs.color", true, true, true},
				{"lasa.entity.e2.dof", false, false, true},
				{"lasa.space.channel.gain.main", false, false, true},
			},
		},
		{
			name: "moderator: any entity's dof (set only) and the moderation keys",
			res:  resolved(true, []string{RoleModerator}, "e1"),
			checks: []check{
				{"lasa.entity.e2.dof", true, false, true}, // any entity; §2 grants no clear
				{"lasa.space.entity.muted.e2", true, true, true},
				{"lasa.space.client.blocked.c9", true, true, true},
				{"lasa.entity.e2.render.gain", false, false, true}, // producer's, not moderator's
				{"lasa.space.channel.gain.main", false, false, true},
			},
		},
		{
			name: "producer: any entity's routing and render keys, the space channel keys",
			res:  resolved(true, []string{RoleProducer}, "e1"),
			checks: []check{
				{"lasa.entity.e2.sources.main", true, true, true},
				{"lasa.entity.e2.sinks.aux", true, true, true},
				{"lasa.entity.e2.priority", true, true, true},
				{"lasa.entity.e2.render.gain", true, true, true},
				{"lasa.entity.e2.render.attenuation", true, true, true},
				{"lasa.entity.e2.render.size", true, true, true},
				{"lasa.entity.e2.render.directivity", true, true, true},
				{"lasa.space.role.blocked.audience", true, true, true},
				{"lasa.space.channel.muted.main", true, true, true},
				{"lasa.space.channel.gain.main", true, true, true},
				{"lasa.space.channel.attenuation.main", true, true, true},
				{"lasa.entity.e2.dof", false, false, true},         // moderator's, not producer's
				{"lasa.entity.e2.attrs.color", false, false, true}, // audience is still own-entity
				{"lasa.space.entity.muted.e2", false, false, true},
			},
		},
		{
			name: "admin is the union of all roles",
			res:  resolved(true, []string{RoleAdmin}, "e1"),
			checks: []check{
				{"lasa.entity.e1.attrs.color", true, true, true},
				{"lasa.entity.e2.dof", true, false, true},
				{"lasa.space.entity.muted.e2", true, true, true},
				{"lasa.entity.e2.render.gain", true, true, true},
				{"lasa.space.channel.gain.main", true, true, true},
			},
		},
		{
			name: "ticket permissions union: a fine-grained grant without the role",
			res: func() *connect.Resolved {
				res := resolved(true, nil, "e1")
				res.Permissions = connect.Permissions{
					Set:  []string{"myapp.controls", "lasa.space.channel.gain"},
					Read: []string{"myapp"},
				}
				return res
			}(),
			checks: []check{
				{"myapp.controls.fader", true, false, true},
				{"myapp.other", false, false, true},                 // read via myapp, no set grant
				{"lasa.space.channel.gain.main", true, false, true}, // permissions may target lasa.
				{"lasa.space.channel.muted.main", false, false, true},
			},
		},
		{
			name: "unticketed: audience over lasa., unrestricted application space",
			res:  resolved(false, nil, "e1"),
			checks: []check{
				{"lasa.entity.e1.attrs.color", true, true, true},
				{"lasa.space.channel.gain.main", false, false, true}, // still just audience for lasa.
				{"myapp.anything.at.all", true, true, true},
				{"lasa", false, false, false}, // the bare reserved key is not app space
			},
		},
		{
			name: "ticketed: no application space without a grant",
			res:  resolved(true, nil, "e1"),
			checks: []check{
				{"myapp.anything", false, false, false},
			},
		},
		{
			name: "zero-entity observer reads everything, writes nothing",
			res:  resolved(true, nil),
			checks: []check{
				{"lasa.entity.e1.name", false, false, true},
				{"lasa.space.channel.gain.main", false, false, true},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := RulesFor(c.res)
			for _, ch := range c.checks {
				if got := r.MaySet(ch.key); got != ch.set {
					t.Errorf("MaySet(%q) = %v, want %v", ch.key, got, ch.set)
				}
				if got := r.MayClear(ch.key); got != ch.clear {
					t.Errorf("MayClear(%q) = %v, want %v", ch.key, got, ch.clear)
				}
				if got := r.MayRead(ch.key); got != ch.read {
					t.Errorf("MayRead(%q) = %v, want %v", ch.key, got, ch.read)
				}
			}
		})
	}
}

func TestAdHocPermitted(t *testing.T) {
	cases := []struct {
		roles []string
		want  bool
	}{
		{[]string{RoleAdmin}, true},
		{[]string{RolePresenter}, true},
		{[]string{RoleModerator, RolePresenter}, true},
		{[]string{RoleProducer}, false},
		{[]string{RoleModerator}, false},
		{[]string{RoleAudience}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := AdHocPermitted(c.roles); got != c.want {
			t.Errorf("AdHocPermitted(%v) = %v, want %v", c.roles, got, c.want)
		}
	}
}

func TestSinkAll(t *testing.T) {
	cases := []struct {
		roles []string
		want  bool
	}{
		{[]string{RoleAdmin}, true},
		{[]string{RoleProducer}, true},
		{[]string{RoleAudience, RoleProducer}, true},
		{[]string{RolePresenter}, false},
		{[]string{RoleModerator}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := SinkAll(c.roles); got != c.want {
			t.Errorf("SinkAll(%v) = %v, want %v", c.roles, got, c.want)
		}
		// AmbiPermitted travels with SinkAll in the base profile (§2):
		// same admin/producer constituency.
		if got := AmbiPermitted(c.roles); got != c.want {
			t.Errorf("AmbiPermitted(%v) = %v, want %v", c.roles, got, c.want)
		}
	}
}
