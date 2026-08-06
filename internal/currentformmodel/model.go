// Package currentformmodel is the rich authoring model for v1alpha3 Form
// Definitions (the Form Family lane decided in spec/decisions/0008..0013).
//
// A Form is described once here — its role, its typed fields, the exact
// Interface and Binding contracts it references — and every derived surface
// (the Draft 2020-12 desired schema, the canonical fixtures, the negative
// fixtures) is emitted from that single declaration.
//
// Unlike the retained v1alpha2 vocabulary (internal/formcatalog), this model
// never emits a "name" desired property: the v1alpha3 resource envelope owns
// metadata.name (decision 0011). It also carries no open capability tokens:
// every string is closed by an anchored grammar or an enum (decision 0008).
package currentformmodel

import (
	"fmt"
	"sort"
	"strings"
)

// Family is one Form Family API group and its group version. It renders the
// namespaced apiVersion of every member FormRef, for example
// "edge.forms.takoform.com/v1alpha1".
type Family struct {
	Group   string
	Version string
}

// APIVersion renders the DNS-like namespaced group with its version.
func (f Family) APIVersion() string { return f.Group + "/" + f.Version }

// Role is the closed v1alpha3 resource role (decision 0009).
type Role string

const (
	RoleIdentity   Role = "identity"
	RoleRevision   Role = "revision"
	RoleDeployment Role = "deployment"
	RoleAttachment Role = "attachment"
	RolePolicy     Role = "policy"
)

// LifecycleCapabilities derives the closed capability set from the role.
// Revisions are immutable snapshots: they never declare update or refresh.
func (r Role) LifecycleCapabilities() []string {
	switch r {
	case RoleRevision:
		return []string{"create", "read", "delete", "import", "observe"}
	case RoleIdentity, RoleDeployment, RoleAttachment, RolePolicy:
		return []string{"create", "read", "update", "delete", "import", "observe", "refresh"}
	default:
		return nil
	}
}

// Valid reports whether the role is a member of the closed enum.
func (r Role) Valid() bool {
	switch r {
	case RoleIdentity, RoleRevision, RoleDeployment, RoleAttachment, RolePolicy:
		return true
	}
	return false
}

// FieldKind is the portable type of one declared field.
type FieldKind string

const (
	// KindString is a closed string: an anchored Pattern or MaxLength bounds it.
	KindString FieldKind = "string"
	// KindStringEnum is a closed enumeration of exact values.
	KindStringEnum FieldKind = "string-enum"
	KindInteger    FieldKind = "integer"
	KindBoolean    FieldKind = "boolean"
	// KindStringSet is a unique-item string array closed by ItemPattern or Enum.
	KindStringSet FieldKind = "string-set"
	// KindJSONMap is the data-only vars map: reviewed key grammar, any JSON
	// value bounded to depth 8 containers, and a per-object key ceiling.
	KindJSONMap FieldKind = "json-map"
	// KindResourceRef references one resource of an exact target kind by name.
	KindResourceRef     FieldKind = "resource-ref"
	KindResourceRefList FieldKind = "resource-ref-list"
	// KindBindingList declares typed capability bindings: binding name plus a
	// target-kind resource reference (decision 0010).
	KindBindingList FieldKind = "binding-list"
	KindObjectList  FieldKind = "object-list"
	KindObject      FieldKind = "object"
	// KindDateString is a calendar date, YYYY-MM-DD.
	KindDateString FieldKind = "date-string"
)

// Field is one typed portable field of a Form.
type Field struct {
	// HCL is the snake_case attribute name; Wire is the camelCase spec key.
	HCL, Wire string
	Kind      FieldKind
	Doc       string

	Required  bool
	Immutable bool

	// Pattern anchors a KindString value; MaxLength optionally bounds it.
	Pattern   string
	MaxLength int
	// Enum closes a KindStringEnum value or KindStringSet items.
	Enum []string
	// Min and Max are inclusive KindInteger bounds.
	Min, Max *int64
	// ItemPattern anchors KindStringSet items when Enum is empty.
	ItemPattern        string
	MinItems, MaxItems int
	// TargetKind names the exact Form kind a KindResourceRef,
	// KindResourceRefList, or KindBindingList points at.
	TargetKind string
	// BindingType names the Binding Definition a KindBindingList carries.
	BindingType string
	// Fields declares the closed members of KindObject and KindObjectList.
	Fields []Field

	// Example is the value used by the canonical conformance fixture.
	Example any
	// AltExample is a second valid value for immutable-field lifecycle proofs.
	AltExample any
	// CounterExample is a value the desired schema must reject. When nil, one
	// is derived from the declared constraint where possible.
	CounterExample any
}

// InterfaceRefSource names an exact Interface contract by name and version.
// The generation pipeline resolves the schemaDigest from the interface
// catalog, so a Form Definition always embeds the exact digest-bound ref.
type InterfaceRefSource struct {
	Name    string
	Version string
}

// BindingRefSource names an exact Binding contract by name and version.
type BindingRefSource struct {
	Name    string
	Version string
}

// Form is one member of a Form Family.
type Form struct {
	Kind         string // PascalCase portable kind
	Slug         string // kebab-case package directory
	ResourceType string // takoform_* Terraform resource type
	Role         Role
	Title        string
	Description  string
	// DefinitionVersion is the SemVer of this Form's definition.
	DefinitionVersion string

	Fields []Field

	ProvidedInterfaces []InterfaceRefSource
	AcceptedBindings   []BindingRefSource
}

// LifecycleCapabilities derives the closed capability set from the role.
func (f Form) LifecycleCapabilities() []string { return f.Role.LifecycleCapabilities() }

// ImmutableFields lists the JSON Pointers a host must treat as replacement
// triggers. Every field of a revision Form is immutable by role.
func (f Form) ImmutableFields() []string {
	var pointers []string
	for _, field := range f.Fields {
		if field.Immutable || f.Role == RoleRevision {
			pointers = append(pointers, "/"+field.Wire)
		}
	}
	sort.Strings(pointers)
	return pointers
}

// FixtureName is the resource name the Form's fixtures reference targets by.
func (f Form) FixtureName() string { return f.Slug }

// Validate proves the structural rules a Form must satisfy before any surface
// is derived from it.
func (f Form) Validate() error {
	if f.Kind == "" || f.Slug == "" || f.ResourceType == "" || f.Title == "" || f.DefinitionVersion == "" {
		return fmt.Errorf("form %q is missing identity fields", f.Kind)
	}
	if !f.Role.Valid() {
		return fmt.Errorf("form %s declares unknown role %q", f.Kind, f.Role)
	}
	if !strings.HasPrefix(f.ResourceType, "takoform_") {
		return fmt.Errorf("form %s resource type %q is outside takoform_*", f.Kind, f.ResourceType)
	}
	seen := map[string]struct{}{}
	for _, field := range f.Fields {
		if _, duplicate := seen[field.Wire]; duplicate {
			return fmt.Errorf("form %s declares duplicate field %s", f.Kind, field.Wire)
		}
		seen[field.Wire] = struct{}{}
		if field.Wire == "name" || field.HCL == "name" {
			return fmt.Errorf("form %s declares a top-level name field; the v1alpha3 envelope owns metadata.name (decision 0011)", f.Kind)
		}
		if field.Kind == KindBindingList && f.Role != RoleRevision {
			return fmt.Errorf("form %s role %s declares binding list %s; capability bindings belong to revision Forms", f.Kind, f.Role, field.Wire)
		}
		if field.Required && field.Example == nil {
			return fmt.Errorf("form %s required field %s has no Example", f.Kind, field.Wire)
		}
		if err := validateField(f.Kind, field); err != nil {
			return err
		}
	}
	if len(f.AcceptedBindings) > 0 && f.Role != RoleRevision {
		return fmt.Errorf("form %s role %s accepts bindings; only revision Forms hold them", f.Kind, f.Role)
	}
	return nil
}

func validateField(kind string, field Field) error {
	if field.HCL == "" || field.Wire == "" || field.Doc == "" {
		return fmt.Errorf("form %s field %q must declare HCL, Wire, and Doc", kind, field.Wire)
	}
	switch field.Kind {
	case KindString:
		if field.Pattern == "" && field.MaxLength == 0 {
			return fmt.Errorf("form %s string field %s is unbounded; declare Pattern or MaxLength", kind, field.Wire)
		}
	case KindStringEnum:
		if len(field.Enum) == 0 {
			return fmt.Errorf("form %s enum field %s declares no values", kind, field.Wire)
		}
	case KindStringSet:
		if len(field.Enum) == 0 && field.ItemPattern == "" {
			return fmt.Errorf("form %s string-set field %s is unbounded; declare Enum or ItemPattern", kind, field.Wire)
		}
	case KindResourceRef, KindResourceRefList, KindBindingList:
		if field.TargetKind == "" {
			return fmt.Errorf("form %s field %s declares no target kind", kind, field.Wire)
		}
	case KindObject, KindObjectList:
		if len(field.Fields) == 0 {
			return fmt.Errorf("form %s object field %s declares no members", kind, field.Wire)
		}
		nested := map[string]struct{}{}
		for _, member := range field.Fields {
			if _, duplicate := nested[member.Wire]; duplicate {
				return fmt.Errorf("form %s field %s declares duplicate member %s", kind, field.Wire, member.Wire)
			}
			nested[member.Wire] = struct{}{}
			if err := validateField(kind, member); err != nil {
				return err
			}
		}
	case KindInteger, KindBoolean, KindJSONMap, KindDateString:
	default:
		return fmt.Errorf("form %s field %s has unknown field kind %q", kind, field.Wire, field.Kind)
	}
	return nil
}

// I64 declares an inclusive integer bound.
func I64(value int64) *int64 { return &value }
