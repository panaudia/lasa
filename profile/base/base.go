// Package base implements the LASA base profile (lasa-base-profile.md):
// the lasa. key vocabulary as typed builders and parsers (keys.go,
// consume.go), the channel composition semantics (compose.go), the
// admission/sweep helpers the server shell applies so no embedder can
// forget them, and the §2 roles expansion (roles.go) that turns a
// connection's roles into its effective state-access rules.
package base

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// Prefix is the reserved base-profile key prefix.
const Prefix = "lasa."

// EntityMarkerKey is the existence marker lasa.entity.{id}: set and
// cleared by the server only, valued with the owning client id (§6).
// All other keys under the entity's prefix MUST be ignored while it
// is absent; existence-only consumers treat the value as opaque.
func EntityMarkerKey(id string) string { return "lasa.entity." + id }

// ClientKey is the client claim record lasa.client.{id}: present iff
// a connection is currently exercising the client id (core §4.4).
// Server-written, value `true` (deliberately minimal), swept
// immediately at disconnect, and in no role's read scope — clients
// never receive it.
func ClientKey(id string) string { return "lasa.client." + id }

// EntityKey builds lasa.entity.{id}.{suffix}.
func EntityKey(id, suffix string) string { return "lasa.entity." + id + "." + suffix }

// NameKey is lasa.entity.{id}.name.
func NameKey(id string) string { return EntityKey(id, "name") }

// AttrKey is lasa.entity.{id}.attrs.{path}.
func AttrKey(id, path string) string { return EntityKey(id, "attrs."+path) }

// SignedAttrKey is lasa.entity.{id}.signed-attrs.{path}.
func SignedAttrKey(id, path string) string { return EntityKey(id, "signed-attrs."+path) }

// SweepScope is the access-rule scope covering everything the server
// sweeps for an entity on disconnect: the marker key, which
// dot-extends over the whole subtree. The engine's marker-first sweep
// ordering (shortest key first) then satisfies the profile's SHOULD.
func SweepScope(entityID string) string { return EntityMarkerKey(entityID) }

// ClientSweepScope is the sweep scope for the connection's client
// claim record. Passed alongside the per-entity SweepScopes so one
// Close sweeps the whole claim set; within the sweep group the engine
// orders shortest-first/bytewise, and any transient mid-group state a
// replica observes is benign — replicas are advisory (core §4.4) and
// consume the sweep as one group.
func ClientSweepScope(clientID string) string { return ClientKey(clientID) }

// ClientClaimGroup is the connection-level claim write: exactly one
// per connection, written BEFORE any entity's AdmissionGroup (its own
// group — the claim never shares a group with an attr-heavy entity,
// which could overflow the core §6.1 op cap). Zero-entity observer
// connections write this and nothing else.
func ClientClaimGroup(clientID string) wire.Group {
	return wire.Group{Ops: []wire.StateMessage{
		wire.Set{Key: ClientKey(clientID), Value: []byte("true")},
	}}
}

// AdmissionGroup builds the server-authored state group announcing an
// admitted entity: the existence marker (valued with the owning client
// id, §6), the name, the config-derived consumer vocabulary (channel
// memberships, render overrides, priority, dof), and the signed attrs
// (from the ticket definition) plus unsigned attrs (from the config or
// setup), flattened to dotted leaf keys with JSON-encoded values per
// lasa-base-profile.md §1. The connection's ClientClaimGroup MUST have
// been written before any AdmissionGroup.
//
// Channel memberships are materialized as the EFFECTIVE sets (§6): an
// omitted config list and an explicit empty list both write `main`
// (§4) — under a live marker the present key set IS the membership.
// Render fields, priority and dof are written only when the config sets
// them (absence means the documented default).
func AdmissionGroup(ownerClientID string, e *connect.ResolvedEntity) (wire.Group, error) {
	owner, err := json.Marshal(ownerClientID)
	if err != nil {
		return wire.Group{}, err
	}
	ops := []wire.StateMessage{
		wire.Set{Key: EntityMarkerKey(e.ID), Value: owner},
	}
	name, err := json.Marshal(e.Name)
	if err != nil {
		return wire.Group{}, err
	}
	ops = append(ops, wire.Set{Key: NameKey(e.ID), Value: name})

	trueLit := []byte("true")
	for _, ch := range effectiveChannels(e.HeardIn) {
		ops = append(ops, wire.Set{Key: HeardInKey(e.ID, ch), Value: trueLit})
	}
	for _, ch := range effectiveChannels(e.Hears) {
		ops = append(ops, wire.Set{Key: HearsKey(e.ID, ch), Value: trueLit})
	}
	if e.Priority {
		ops = append(ops, wire.Set{Key: EntityKey(e.ID, "priority"), Value: trueLit})
	}
	if r := e.Render; r != nil {
		for _, f := range []struct {
			key string
			val *float64
		}{
			{RenderGainKey(e.ID), r.Gain},
			{RenderAttenuationKey(e.ID), r.Attenuation},
			{RenderSizeKey(e.ID), r.Size},
			{RenderDirectivityKey(e.ID), r.Directivity},
		} {
			if f.val != nil {
				ops = append(ops, wire.Set{Key: f.key, Value: strconv.AppendFloat(nil, *f.val, 'g', -1, 64)})
			}
		}
	}
	if e.Dof != nil {
		ops = append(ops, wire.Set{Key: DofKey(e.ID), Value: strconv.AppendInt(nil, int64(*e.Dof), 10)})
	}
	if e.Frame == "head" {
		// Materialized so every client can render the entity's presence
		// poses as head-relative ("world" is the absent default).
		ops = append(ops, wire.Set{Key: FrameKey(e.ID), Value: []byte(`"head"`)})
	}

	// For a signed entity the definition's attrs came from inside the
	// ticket (signed-attrs) and the setup's attrs are the unsigned ones;
	// for an ad-hoc entity the definition's attrs are unsigned.
	defKey := AttrKey
	if e.Signed {
		defKey = SignedAttrKey
	}
	defOps, err := attrOps(e.ID, defKey, e.Attrs)
	if err != nil {
		return wire.Group{}, err
	}
	ops = append(ops, defOps...)
	if e.Signed {
		setupOps, err := attrOps(e.ID, AttrKey, e.SetupAttrs)
		if err != nil {
			return wire.Group{}, err
		}
		ops = append(ops, setupOps...)
	}
	// The store's value cap applies to admission writes like any other
	// (core §9); an oversized config attr rejects the connection here,
	// cleanly, rather than reaching the engine.
	for _, op := range ops {
		if s, ok := op.(wire.Set); ok && len(s.Value) > wire.MaxValueLen {
			return wire.Group{}, fmt.Errorf("value for %s exceeds %d bytes", s.Key, wire.MaxValueLen)
		}
	}
	return wire.Group{Ops: ops}, nil
}

// effectiveChannels resolves a config membership list to the effective
// one (§4): an omitted OR empty list defaults. This is the gate — the
// only place config becomes membership state — and it guarantees every
// admitted entity holds at least one channel, so no consumer has to
// reason about an empty membership set or re-apply the default itself.
func effectiveChannels(list []string) []string {
	if len(list) == 0 {
		return DefaultChannels()
	}
	return list
}

// attrOps flattens a (possibly nested) attrs object into one Set per leaf,
// deterministically ordered. Arrays are invalid anywhere in attrs.
func attrOps(entityID string, keyFn func(id, path string) string, attrs map[string]any) ([]wire.StateMessage, error) {
	if len(attrs) == 0 {
		return nil, nil
	}
	var ops []wire.StateMessage
	var walk func(path string, v any) error
	walk = func(path string, v any) error {
		switch val := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(val))
			for k := range val {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if err := walk(path+"."+k, val[k]); err != nil {
					return err
				}
			}
			return nil
		case []any:
			return fmt.Errorf("base: arrays are invalid in attrs (at %s)", path)
		default:
			leaf, err := json.Marshal(val)
			if err != nil {
				return err
			}
			ops = append(ops, wire.Set{Key: keyFn(entityID, path[1:]), Value: leaf})
			return nil
		}
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := walk("."+k, attrs[k]); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
