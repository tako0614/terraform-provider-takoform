package currentformmodel

import (
	"fmt"
	"sort"
)

// environment.go carries the authoring-side view of the single runtime
// environment namespace decided by
// spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md.
//
// A Form's `vars` keys, its sealed-value declaration entries, and the `name` of
// every typed binding are all projected into ONE environment object the code
// receives. Two of them sharing a name specifies two different values under one
// identifier, and a desired schema cannot express the constraint: `uniqueItems`
// rejects a duplicated whole object, never two objects agreeing only on `name`,
// and no keyword reaches across sibling properties.
//
// What the authoring model can prove is bounded, and the bound is worth stating
// plainly: whether two declarations collide is a property of one INSTANCE, not
// of the Form. A Form that declares five binding lists is a Form whose author
// may write `CACHE` in two of them; nothing about the declaration is wrong. So
// the model proves the two things that are static — that every field projecting
// into the namespace is declared as such, and that the Form's own canonical
// example is admissible — and the instance rule is enforced by the provider at
// plan time and by the host before mutation.

// EnvironmentNameField is one declared source of environment names.
type EnvironmentNameField struct {
	Field Field
	// Source names how the field contributes: "binding-names", "map-keys", or
	// "items".
	Source string
}

// Environment name sources.
const (
	EnvironmentBindingNames = "binding-names"
	EnvironmentMapKeys      = "map-keys"
	EnvironmentSetItems     = "items"
	// EnvironmentExternalServiceNames is a sealed runtime-native binding list.
	// Takoform owns only the binding key (the slot name); the Host integration
	// for the opaque protocol owns its internal entries and projection shape.
	EnvironmentExternalServiceNames = "external-service-names"
)

// EnvironmentNameFields lists every declared field of one Form that projects
// names into the runtime environment namespace, in declaration order.
func (f Form) EnvironmentNameFields() []EnvironmentNameField {
	var out []EnvironmentNameField
	for _, field := range f.Fields {
		switch {
		case field.Kind == KindBindingList:
			out = append(out, EnvironmentNameField{Field: field, Source: EnvironmentBindingNames})
		case field.Kind == KindExternalServiceList:
			out = append(out, EnvironmentNameField{Field: field, Source: EnvironmentExternalServiceNames})
		case !field.ProjectsEnvironmentNames:
		case field.Kind == KindJSONMap:
			out = append(out, EnvironmentNameField{Field: field, Source: EnvironmentMapKeys})
		case field.Kind == KindStringSet:
			out = append(out, EnvironmentNameField{Field: field, Source: EnvironmentSetItems})
		}
	}
	return out
}

// EnvironmentNamesOfExample reads the environment names one field's canonical
// Example declares.
func EnvironmentNamesOfExample(entry EnvironmentNameField) []string {
	var out []string
	switch entry.Source {
	case EnvironmentExternalServiceNames:
		items, _ := entry.Field.Example.([]any)
		for _, item := range items {
			slot, _ := item.(map[string]any)
			name, _ := slot["name"].(string)
			if name != "" {
				out = append(out, name)
			}
		}
	case EnvironmentBindingNames:
		items, _ := entry.Field.Example.([]any)
		for _, item := range items {
			instance, _ := item.(map[string]any)
			if name, _ := instance["name"].(string); name != "" {
				out = append(out, name)
			}
		}
	case EnvironmentMapKeys:
		values, _ := entry.Field.Example.(map[string]any)
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out = append(out, keys...)
	case EnvironmentSetItems:
		items, _ := entry.Field.Example.([]any)
		for _, item := range items {
			if name, _ := item.(string); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// ValidateEnvironmentNamespace proves that a Form's own canonical Examples
// declare a collision-free environment namespace.
//
// The canonical fixture is the document every conformance corpus and every
// generated example is built from, so a Form whose Examples collided would ship
// a fixture a conforming host must refuse. It is the one collision the
// authoring model can decide, because the Examples are the only instance the
// Form declaration owns.
func ValidateEnvironmentNamespace(f Form) error {
	claimed := map[string]string{}
	for _, entry := range f.EnvironmentNameFields() {
		for _, name := range EnvironmentNamesOfExample(entry) {
			if previous, taken := claimed[name]; taken {
				return fmt.Errorf(
					"form %s declares the environment name %q in both %s and %s; "+
						"vars, sealed-value names, and every binding list project into one namespace",
					f.Kind, name, previous, entry.Field.Wire,
				)
			}
			claimed[name] = entry.Field.Wire
		}
	}
	return nil
}
