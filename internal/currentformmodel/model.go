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

	// Default is the portable normative value of an optional field. It is the
	// ONLY way an omitted optional field acquires meaning: every host
	// materializes it into the effective spec before validation, so omitting
	// the field and writing the default produce byte-identical desired state.
	// A required field must never declare one.
	Default any
	// AbsenceIsSemantic marks the one shape a default cannot express: a field
	// whose ABSENCE is itself the portable semantics, so materializing any
	// value would change behavior. It is a narrow, audited exemption from the
	// "optional means defaulted" rule; the Doc of such a field must state what
	// the absent case does.
	AbsenceIsSemantic bool

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

// MutableFields lists the portable desired fields a spec-changing update may
// move. A revision Form fixes every field by role, so it has none, and an
// otherwise-mutable role whose every field is Immutable has none either.
func (f Form) MutableFields() []Field {
	if f.Role == RoleRevision {
		return nil
	}
	var out []Field
	for _, field := range f.Fields {
		if !field.Immutable {
			out = append(out, field)
		}
	}
	return out
}

// DeclaresUpdate reports whether this Form has anything an update could
// change. Capability follows the declared fields, never the role alone: a Form
// whose every desired field is immutable (or which declares no field at all)
// cannot represent an in-place update, so advertising one would promise an
// operation no conforming host can perform.
func (f Form) DeclaresUpdate() bool { return len(f.MutableFields()) > 0 }

// LifecycleCapabilities derives the closed capability set of this Form. The
// base set is exactly create, read, delete, import, observe; update is added
// only when the Form has at least one mutable desired field. The v1alpha3 lane
// has no refresh capability at all: observe is the one read-only host-side
// re-observation operation (spec/host-api/v1alpha3.md).
func (f Form) LifecycleCapabilities() []string {
	capabilities := make([]string, 0, 6)
	capabilities = append(capabilities, "create", "read")
	if f.DeclaresUpdate() {
		capabilities = append(capabilities, "update")
	}
	return append(capabilities, "delete", "import", "observe")
}

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
		// An optional field with no declared meaning for its own absence is
		// exactly the portability hole this lane forbids: two conforming hosts
		// would be free to pick different behavior for the same document. Every
		// optional field therefore either carries a portable default that every
		// host materializes, or states that its absence IS the semantics.
		if !field.Required && field.Default == nil && !field.AbsenceIsSemantic {
			return fmt.Errorf(
				"form %s optional field %s declares neither a Default nor AbsenceIsSemantic; "+
					"an omitted optional field must have one portable meaning",
				f.Kind, field.Wire,
			)
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

// absenceSemanticPhrases are the ways a Doc may state what an absent value
// does. The marker alone is not auditable: a reader of the Form Definition
// must be able to see the absent-case behavior in the field's own prose.
var absenceSemanticPhrases = []string{
	"without it", "when absent", "if absent", "when omitted", "if omitted", "in its absence",
}

func validateField(kind string, field Field) error {
	if field.HCL == "" || field.Wire == "" || field.Doc == "" {
		return fmt.Errorf("form %s field %q must declare HCL, Wire, and Doc", kind, field.Wire)
	}
	if field.Required && field.Default != nil {
		return fmt.Errorf("form %s required field %s declares a Default; a required value is never omitted", kind, field.Wire)
	}
	if field.AbsenceIsSemantic {
		if field.Required {
			return fmt.Errorf("form %s field %s marks AbsenceIsSemantic while being required", kind, field.Wire)
		}
		if field.Default != nil {
			return fmt.Errorf("form %s field %s marks AbsenceIsSemantic and also declares a Default", kind, field.Wire)
		}
		stated := false
		lowered := strings.ToLower(field.Doc)
		for _, phrase := range absenceSemanticPhrases {
			if strings.Contains(lowered, phrase) {
				stated = true
				break
			}
		}
		if !stated {
			return fmt.Errorf(
				"form %s field %s marks AbsenceIsSemantic but its Doc never states the absent-case behavior (expected one of %s)",
				kind, field.Wire, strings.Join(absenceSemanticPhrases, ", "),
			)
		}
	}
	if err := validateFieldDefault(kind, field); err != nil {
		return err
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
