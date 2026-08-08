package currentformmodel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Relation is one cross-resource reference a Form declares, derived from the
// Form's desired schema rather than declared a second time.
//
// Relations are DERIVED, never authored. Every reference already encodes its
// target group and kind as schema constants, so a separate `resourceRelations`
// member on the Form Definition would be a second source of truth for facts the
// desired schema already states — and the published Form Definition schema
// forbids unknown members anyway
// (spec/decisions/0014-published-schemas-are-structural-minima.md). Deriving
// also means a host needs nothing beyond the `desiredSchema` it already serves:
// the same walk that produced this list on the authoring side reproduces it on
// the host side.
type Relation struct {
	// Pointer is the JSON Pointer of the reference node inside a desired spec,
	// with "*" standing for "every element" of an array. Examples:
	// "/worker", "/versions/*/workerVersion", "/kvBindings/*/resource".
	Pointer string
	// TargetAPIVersion and TargetKind are the exact schema constants the
	// reference pins. A conforming spec can carry no other pair here.
	TargetAPIVersion string
	TargetKind       string
	// Binding names the Binding Definition governing this reference, or is
	// empty when the reference is a plain cross-resource reference. The exact
	// digest-bound BindingRef is resolved from the owning Form Definition's
	// acceptedBindings, which is where the digest already lives.
	Binding string
	// Required reports whether every step of Pointer is a required member of
	// its parent object. A relation that is not required may legitimately be
	// absent from a valid desired spec; a required one is always present.
	Required bool
}

// IsBinding reports whether this relation is a typed capability binding.
func (r Relation) IsBinding() bool { return r.Binding != "" }

// DeriveRelations walks one desired schema and returns every cross-resource
// reference it declares, in a stable pointer order.
//
// A node is reference-shaped when it is a closed object requiring exactly
// apiVersion, kind, and name, whose apiVersion and kind are `const`. That shape
// is emitted by exactly one construct in this model, so recognizing it is the
// same as recognizing a declared reference — and unlike the superseded
// "any member literally named resource" rule, it finds every reference a Form
// declares instead of only the ones inside binding lists.
func DeriveRelations(schema map[string]any) ([]Relation, error) {
	walker := relationWalker{}
	if err := walker.walk(schema, "", "", true); err != nil {
		return nil, err
	}
	sort.Slice(walker.out, func(i, j int) bool { return walker.out[i].Pointer < walker.out[j].Pointer })
	return walker.out, nil
}

// maxRelationDepth bounds the derivation walk. The desired schemas of this
// model are shallow by construction; the bound exists so a hostile or
// malformed document cannot turn derivation into unbounded work.
const maxRelationDepth = 32

type relationWalker struct {
	out []Relation
}

func (w *relationWalker) walk(node map[string]any, pointer, binding string, required bool) error {
	return w.walkAt(node, pointer, binding, required, 0)
}

func (w *relationWalker) walkAt(node map[string]any, pointer, binding string, required bool, depth int) error {
	if node == nil {
		return nil
	}
	if depth > maxRelationDepth {
		return fmt.Errorf("relation derivation exceeded depth %d at %q", maxRelationDepth, pointer)
	}
	if group, kind, ok := referenceConstants(node); ok {
		if pointer == "" {
			return fmt.Errorf("relation derivation found a reference at the desired-state root")
		}
		w.out = append(w.out, Relation{
			Pointer:          pointer,
			TargetAPIVersion: group,
			TargetKind:       kind,
			Binding:          binding,
			Required:         required,
		})
		return nil
	}
	if items, ok := node["items"].(map[string]any); ok {
		// Every element of an array shares one pointer with "*" in place of the
		// index: the relation is a property of the schema, and the concrete
		// index only exists once a spec is materialized. An element is always
		// "present" when the array itself is, so requiredness passes through.
		if err := w.walkAt(items, pointer+"/*", relationBinding(node, binding), required, depth+1); err != nil {
			return err
		}
	}
	properties, _ := node["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	requiredMembers := map[string]bool{}
	for _, member := range anyStrings(node["required"]) {
		requiredMembers[member] = true
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		child, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		if err := w.walkAt(
			child,
			pointer+"/"+escapeJSONPointerToken(name),
			relationBinding(child, binding),
			required && requiredMembers[name],
			depth+1,
		); err != nil {
			return err
		}
	}
	return nil
}

// relationBinding carries the Binding annotation down from the property that
// declares it to the reference node inside it.
func relationBinding(node map[string]any, inherited string) string {
	if annotated, ok := node[BindingAnnotationKey].(string); ok && annotated != "" {
		return annotated
	}
	return inherited
}

// referenceConstants recognizes the exact closed reference shape and returns
// the pinned group and kind.
func referenceConstants(node map[string]any) (string, string, bool) {
	if kind, _ := node["type"].(string); kind != "object" {
		return "", "", false
	}
	if closed, ok := node["additionalProperties"].(bool); !ok || closed {
		return "", "", false
	}
	properties, _ := node["properties"].(map[string]any)
	if len(properties) != 3 {
		return "", "", false
	}
	requiredMembers := anyStrings(node["required"])
	sorted := append([]string(nil), requiredMembers...)
	sort.Strings(sorted)
	if strings.Join(sorted, ",") != "apiVersion,kind,name" {
		return "", "", false
	}
	group, groupOK := constantString(properties["apiVersion"])
	kind, kindOK := constantString(properties["kind"])
	if _, present := properties["name"]; !present || !groupOK || !kindOK {
		return "", "", false
	}
	return group, kind, true
}

func constantString(node any) (string, bool) {
	object, ok := node.(map[string]any)
	if !ok {
		return "", false
	}
	value, ok := object["const"].(string)
	return value, ok && value != ""
}

// anyStrings reads a schema string array that may have travelled through JSON
// decoding ([]any) or been built in Go ([]string).
func anyStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

// escapeJSONPointerToken renders one RFC 6901 reference token.
func escapeJSONPointerToken(token string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(token)
}

// RelationInstance is one relation resolved against a concrete desired spec:
// the derived relation plus the exact pointer and target name it names there.
type RelationInstance struct {
	// Pointer is the concrete instance pointer, with array indices resolved.
	Pointer string
	// Relation is the derived schema pointer this instance came from.
	Relation string
	// TargetAPIVersion, TargetKind, and TargetName address the target.
	TargetAPIVersion string
	TargetKind       string
	TargetName       string
	// Binding names the governing Binding Definition, empty for a plain
	// reference.
	Binding string
}

// RelationInstances resolves every derived relation against one materialized
// desired spec, in a stable pointer order. A relation whose node is absent from
// the spec yields nothing; an optional relation is simply not there.
func RelationInstances(relations []Relation, spec map[string]any) []RelationInstance {
	var out []RelationInstance
	for _, relation := range relations {
		out = append(out, resolveRelation(relation, spec)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pointer < out[j].Pointer })
	return out
}

func resolveRelation(relation Relation, spec map[string]any) []RelationInstance {
	tokens := strings.Split(strings.TrimPrefix(relation.Pointer, "/"), "/")
	return descend(relation, any(spec), tokens, "")
}

func descend(relation Relation, value any, tokens []string, pointer string) []RelationInstance {
	if len(tokens) == 0 {
		reference, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		name, _ := reference["name"].(string)
		if name == "" {
			return nil
		}
		return []RelationInstance{{
			Pointer:          pointer,
			Relation:         relation.Pointer,
			TargetAPIVersion: relation.TargetAPIVersion,
			TargetKind:       relation.TargetKind,
			TargetName:       name,
			Binding:          relation.Binding,
		}}
	}
	token, rest := tokens[0], tokens[1:]
	if token == "*" {
		items, ok := value.([]any)
		if !ok {
			return nil
		}
		var out []RelationInstance
		for index, item := range items {
			out = append(out, descend(relation, item, rest, pointer+"/"+strconv.Itoa(index))...)
		}
		return out
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	member := unescapeJSONPointerToken(token)
	child, present := object[member]
	if !present {
		return nil
	}
	return descend(relation, child, rest, pointer+"/"+token)
}

func unescapeJSONPointerToken(token string) string {
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(token)
}
