package formcatalog

import "testing"

// TestEveryFormHasAnUpdatableField keeps the protocol lifecycle honest: a Form
// whose every field is immutable can never prove an in-place update, so the
// conformance run would silently skip that operation.
func TestEveryFormHasAnUpdatableField(t *testing.T) {
	for _, kind := range Kinds {
		updatable := false
		for _, field := range kind.Fields {
			if field.Immutable || field.Example == nil {
				continue
			}
			switch {
			case field.AltExample != nil,
				len(field.Enum) > 1,
				field.Type == TypeInt,
				field.Type == TypeBool,
				field.Type == TypeStringSet && (len(field.Enum) > 0 || field.Grammar == GrammarToken):
				updatable = true
			}
		}
		if !updatable {
			t.Errorf("%s declares no field an update can change", kind.Kind)
		}
	}
}

// TestEveryImmutableFieldCarriesAnAlternative keeps replacement provable.
func TestEveryImmutableFieldCarriesAnAlternative(t *testing.T) {
	for _, kind := range Kinds {
		for _, field := range kind.Fields {
			if field.Immutable && field.AltExample == nil {
				t.Errorf("%s.%s is immutable but declares no alternative value", kind.Kind, field.HCL)
			}
		}
	}
}

// TestEveryFormDeclaresIdentity guards the catalogue's own invariants.
func TestEveryFormDeclaresIdentity(t *testing.T) {
	kinds := map[string]struct{}{}
	slugs := map[string]struct{}{}
	types := map[string]struct{}{}
	for _, kind := range Kinds {
		if kind.Kind == "" || kind.Slug == "" || kind.ResourceType == "" || kind.Title == "" || kind.Description == "" {
			t.Fatalf("incomplete declaration: %#v", kind.Kind)
		}
		for _, set := range []struct {
			seen  map[string]struct{}
			value string
			label string
		}{{kinds, kind.Kind, "kind"}, {slugs, kind.Slug, "slug"}, {types, kind.ResourceType, "resource type"}} {
			if _, duplicate := set.seen[set.value]; duplicate {
				t.Fatalf("duplicate %s %q", set.label, set.value)
			}
			set.seen[set.value] = struct{}{}
		}
		hcl := map[string]struct{}{}
		wire := map[string]struct{}{}
		for _, field := range kind.Fields {
			if _, duplicate := hcl[field.HCL]; duplicate {
				t.Fatalf("%s duplicates attribute %s", kind.Kind, field.HCL)
			}
			if _, duplicate := wire[field.Wire]; duplicate {
				t.Fatalf("%s duplicates wire key %s", kind.Kind, field.Wire)
			}
			hcl[field.HCL] = struct{}{}
			wire[field.Wire] = struct{}{}
		}
	}
}
