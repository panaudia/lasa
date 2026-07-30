// Package base implements the LASA base profile (lasa-base-profile.md):
// the lasa. key vocabulary as typed builders, and the admission/sweep
// helpers the server shell applies so no embedder can forget them.
//
// v0 scope (implementation-plan.md step 6): the existence marker, name,
// attrs and signed-attrs. Roles expansion, channels, moderation
// enforcement, dof and render vocabulary land with step 7.
package base

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// Prefix is the reserved base-profile key prefix.
const Prefix = "lasa."

// EntityMarkerKey is the existence marker lasa.entity.{id}: set and
// cleared by the server only. All other keys under the entity's prefix
// MUST be ignored while it is absent.
func EntityMarkerKey(id string) string { return "lasa.entity." + id }

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

// AdmissionGroup builds the server-authored state group announcing an
// admitted entity: the existence marker, the name, the signed attrs (from
// the ticket definition) and unsigned attrs (from the config or setup),
// flattened to dotted leaf keys with JSON-encoded values per
// lasa-base-profile.md §1.
func AdmissionGroup(e *connect.ResolvedEntity) (wire.Group, error) {
	ops := []wire.StateMessage{
		wire.Set{Key: EntityMarkerKey(e.ID), Value: []byte("true")},
	}
	name, err := json.Marshal(e.Name)
	if err != nil {
		return wire.Group{}, err
	}
	ops = append(ops, wire.Set{Key: NameKey(e.ID), Value: name})

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
	return wire.Group{Ops: ops}, nil
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
