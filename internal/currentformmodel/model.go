// Package currentformmodel is the rich authoring model for v1beta1 Form
// Definitions (the current Beta Form Family contract fixed by decision 0035).
//
// A Form is described once here — its role, its typed fields, the exact
// Interface and Binding contracts it references — and every derived surface
// (the Draft 2020-12 desired schema, the canonical fixtures, the negative
// fixtures) is emitted from that single declaration.
//
// Unlike the retained v1alpha2 vocabulary (internal/formcatalog), this model
// never emits a "name" desired property: the v1beta1 resource envelope owns
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
// "edge.forms.takoform.com/v1beta1".
type Family struct {
	Group   string
	Version string
}

// APIVersion renders the DNS-like namespaced group with its version.
func (f Family) APIVersion() string { return f.Group + "/" + f.Version }

// Role is the closed v1beta1 resource role (decision 0009).
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
	// KindExternalServiceList declares sealed external standard-service
	// slots (decision 0045): a SCREAMING_SNAKE name plus a protocol from the
	// closed standards vocabulary, with an optional requiredness flag.
	// Portable state never carries the endpoint or credential that satisfies
	// a slot; the host projects protocol-fixed members into the same runtime
	// namespace binding names occupy.
	KindExternalServiceList FieldKind = "external-service-list"
	// KindBindingList declares typed capability bindings: binding name plus a
	// target-kind resource reference (decision 0010).
	KindBindingList FieldKind = "binding-list"
	KindObjectList  FieldKind = "object-list"
	KindObject      FieldKind = "object"
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
	// Target states WHAT the referenced resource must still satisfy. Every
	// reference-shaped field declares exactly one of the two requirements
	// (decision 0022); a field that declared none would emit a reference
	// satisfied by any target of the right group and kind, whatever contract
	// that target's Definition has since moved to.
	Target TargetContract
	// BindingType names the Binding Definition a KindBindingList carries.
	BindingType string
	// Fields declares the closed members of KindObject and KindObjectList.
	Fields []Field

	// ProjectsEnvironmentNames marks a field whose VALUES name entries in the
	// same runtime environment namespace that binding names occupy: the map
	// KEYS of a KindJSONMap, the ITEMS of a KindStringSet. A binding list always
	// projects names by construction, so only the non-binding sources need the
	// marker.
	//
	// It exists because the collision is unrepresentable in a desired schema:
	// `uniqueItems` rejects a duplicated whole object, never two objects that
	// agree only on `name`, and no keyword reaches across sibling properties at
	// all (spec/decisions/0016).
	ProjectsEnvironmentNames bool

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

// TargetContract declares which of the two portable requirements one
// reference-shaped field states about the resource it points at. Exactly one
// member is set (decision 0022).
//
// The choice is a statement about the DEPENDENCY, not about convenience.
// ExactForm is correct when the source — or the host acting for it — reads a
// member of the target's desired spec, or enforces a rule stated over the
// target Form itself: those break the moment the target's Definition changes
// shape, so nothing weaker than the exact contract states the requirement.
// Interface is correct when what the source needs is behavior a contract fixes
// and any Form providing it would serve: pinning the Form there would refuse a
// perfectly good target for a reason the source does not actually have.
type TargetContract struct {
	// ExactForm requires the target to be one of the exact Form identities the
	// build renders for the field's TargetKind.
	ExactForm bool
	// Interface names the exact Interface contract the target must provide.
	Interface *InterfaceRefSource
}

// Declared reports whether this field states a target contract at all.
func (t TargetContract) Declared() bool { return t.ExactForm || t.Interface != nil }

// BindingRefSource names an exact Binding contract by name and version.
type BindingRefSource struct {
	Name    string
	Version string
}

// Form is one member of a Form Family.
type Form struct {
	// Family is the API group this Form belongs to. It is the apiVersion every
	// cross-resource reference this Form declares points into: a reference is
	// {apiVersion, kind, name}, and both the group and the kind are constants
	// of the referring field.
	Family       Family
	Kind         string // PascalCase portable kind
	Slug         string // kebab-case package directory
	ResourceType string // takoform_* Terraform resource type
	Role         Role
	Title        string
	Description  string
	// DefinitionVersion is the SemVer of this Form's definition.
	DefinitionVersion string

	Fields []Field

	// Outputs is the closed set of host-computed values this Form publishes in
	// `status.outputs`. It is the Form's OUTPUT contract: a host that supports
	// the Form returns every declared output, and returns nothing else, which
	// is what makes an output readable as a typed value rather than as an
	// untyped JSON blob a consumer has to guess the shape of.
	//
	// An output is not desired state. It is never written, never defaulted,
	// never immutable, and never a cross-resource reference: those words all
	// describe what an author asks for, and an output is what the host answers.
	// The declarations here are held to that by validateOutput.
	Outputs []Field

	ProvidedInterfaces []InterfaceRefSource
	AcceptedBindings   []BindingRefSource
}

// ReservedResourceAttributes is the closed set of attribute names the
// v1beta1 resource surface owns on every Form: the portable envelope
// (name, space, uid, generation, revision, conditions), the derived
// conveniences a client renders from it, the exact recorded FormRef, the two
// internal recovery records, and the operation timeouts.
//
// A Form field or output may not take one of these names. The clash would not
// be a naming inconvenience: the envelope member and the Form member would be
// one attribute holding two different facts, and whichever the resource wrote
// last would silently win. The list lives here rather than in the provider so
// the AUTHORING model refuses the Form, before any surface is derived from it;
// the provider proves the two agree.
var ReservedResourceAttributes = []string{
	"conditions",
	"create_timeout",
	"delete_timeout",
	"form_api_version",
	"form_definition_version",
	"form_kind",
	"form_package_digest",
	"form_schema_digest",
	"generation",
	"name",
	"outputs_json",
	"pending_operation_id",
	"ready",
	"relation_drift_reason",
	"revision",
	"space",
	"uid",
	"update_timeout",
}

func reservedResourceAttribute(name string) bool {
	for _, reserved := range ReservedResourceAttributes {
		if reserved == name {
			return true
		}
	}
	return false
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
// only when the Form has at least one mutable desired field. The v1beta1 channel
// has no refresh capability at all: observe is the one read-only host-side
// re-observation operation (spec/host-api/v1beta1.md).
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
			return fmt.Errorf("form %s declares a top-level name field; the v1beta1 envelope owns metadata.name (decision 0011)", f.Kind)
		}
		if reservedResourceAttribute(field.AttributeName()) {
			return fmt.Errorf(
				"form %s field %s takes the reserved resource attribute %q; the v1beta1 envelope owns it",
				f.Kind, field.Wire, field.AttributeName(),
			)
		}
		if field.Kind == KindExternalServiceList && f.Role != RoleRevision {
			return fmt.Errorf("%s.%s: external-service slots are held by revision-role Forms", f.Kind, field.Wire)
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
	if err := f.validateOutputs(seen); err != nil {
		return err
	}
	if len(f.AcceptedBindings) > 0 && f.Role != RoleRevision {
		return fmt.Errorf("form %s role %s accepts bindings; only revision Forms hold them", f.Kind, f.Role)
	}
	// A reference names its target group as a constant, so a Form that declares
	// one must know which group it belongs to. Without it the emitted schema
	// would pin an empty apiVersion and every reference would be unresolvable.
	if f.declaresReference() && (f.Family.Group == "" || f.Family.Version == "") {
		return fmt.Errorf("form %s declares a cross-resource reference without a Family group", f.Kind)
	}
	return nil
}

// validateOutputs proves the Form's output contract is one a host can answer
// and a client can type. desiredNames is the wire-name set of the desired
// fields, so an output can never be a second spelling of something the author
// already writes.
//
// The rules are all one rule read from different sides: an output is what the
// HOST computed, so nothing that describes an author's request may appear on
// it. A required flag would be meaningless (every declared output is returned,
// which is what the schema states), a default would name a value no host
// produced, an immutable flag would fence a value no one writes, and a
// reference would make an output a relation — a thing the lane resolves, pins
// by UID, and protects from deletion, none of which a computed value has.
func (f Form) validateOutputs(desiredNames map[string]struct{}) error {
	seen := map[string]struct{}{}
	attributes := map[string]struct{}{}
	for _, field := range f.Fields {
		attributes[field.AttributeName()] = struct{}{}
	}
	for _, output := range f.Outputs {
		if output.HCL == "" || output.Wire == "" || output.Doc == "" {
			return fmt.Errorf("form %s output %q must declare HCL, Wire, and Doc", f.Kind, output.Wire)
		}
		if _, duplicate := seen[output.Wire]; duplicate {
			return fmt.Errorf("form %s declares duplicate output %s", f.Kind, output.Wire)
		}
		seen[output.Wire] = struct{}{}
		if _, taken := desiredNames[output.Wire]; taken {
			return fmt.Errorf(
				"form %s output %s is also a desired property; an output is what the host answers, never a second spelling of what the author wrote",
				f.Kind, output.Wire,
			)
		}
		if _, taken := attributes[output.AttributeName()]; taken {
			return fmt.Errorf("form %s output %s collides with a desired attribute name", f.Kind, output.Wire)
		}
		if reservedResourceAttribute(output.AttributeName()) {
			return fmt.Errorf(
				"form %s output %s takes the reserved resource attribute %q; the v1beta1 envelope owns it",
				f.Kind, output.Wire, output.AttributeName(),
			)
		}
		switch {
		case output.Required:
			return fmt.Errorf(
				"form %s output %s is marked Required; every declared output is returned, which the output schema states, so the flag could only disagree with it",
				f.Kind, output.Wire,
			)
		case output.Immutable:
			return fmt.Errorf("form %s output %s is marked Immutable; nothing writes an output", f.Kind, output.Wire)
		case output.Default != nil || output.AbsenceIsSemantic:
			return fmt.Errorf(
				"form %s output %s declares an absence meaning; a host returns every declared output, so there is no absent case",
				f.Kind, output.Wire,
			)
		case output.Target.Declared() || output.TargetKind != "" || output.BindingType != "":
			return fmt.Errorf("form %s output %s points at another resource; an output is a value, never a relation", f.Kind, output.Wire)
		case output.ProjectsEnvironmentNames:
			return fmt.Errorf("form %s output %s claims to project environment names; only desired state does", f.Kind, output.Wire)
		}
		switch output.Kind {
		case KindString:
			if output.Pattern == "" && output.MaxLength == 0 {
				return fmt.Errorf("form %s output %s is an unbounded string; declare Pattern or MaxLength", f.Kind, output.Wire)
			}
		case KindStringEnum:
			if len(output.Enum) == 0 {
				return fmt.Errorf("form %s output %s declares no enum values", f.Kind, output.Wire)
			}
		case KindInteger, KindBoolean:
		default:
			return fmt.Errorf(
				"form %s output %s has kind %q; an output is one closed scalar, so a consumer reads a typed value rather than a document",
				f.Kind, output.Wire, output.Kind,
			)
		}
	}
	return nil
}

// AttributeName is the Terraform attribute one field or output surfaces under.
// The data-only JSON map surfaces as <name>_json because its values are
// arbitrary bounded JSON, which an HCL map of strings cannot carry faithfully.
func (f Field) AttributeName() string {
	if f.Kind == KindJSONMap {
		return f.HCL + "_json"
	}
	return f.HCL
}

// declaresReference reports whether any declared field, at any depth, is a
// cross-resource reference.
func (f Form) declaresReference() bool {
	return fieldsDeclareReference(f.Fields)
}

func fieldsDeclareReference(fields []Field) bool {
	for _, field := range fields {
		switch field.Kind {
		case KindResourceRef, KindResourceRefList, KindBindingList:
			return true
		}
		if fieldsDeclareReference(field.Fields) {
			return true
		}
	}
	return false
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
	if field.ProjectsEnvironmentNames {
		switch field.Kind {
		// An external-service list names environment entries INDIRECTLY: what
		// joins the namespace is the projected closure of each slot, not the
		// slot names, which is why the uniqueness rule is stated over that
		// closure (spec/standard-services).
		case KindJSONMap, KindStringSet, KindExternalServiceList:
		default:
			return fmt.Errorf(
				"form %s field %s marks ProjectsEnvironmentNames on kind %q; only a json-map's keys, "+
					"a string-set's items, or an external-service list's projected members can name "+
					"environment entries",
				kind, field.Wire, field.Kind,
			)
		}
	}
	switch field.Kind {
	case KindResourceRef, KindResourceRefList, KindBindingList:
		if err := validateTargetContract(kind, field); err != nil {
			return err
		}
	default:
		if field.Target.Declared() {
			return fmt.Errorf(
				"form %s field %s declares a target contract on kind %q; only a reference-shaped field points at another resource",
				kind, field.Wire, field.Kind,
			)
		}
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
	case KindResourceRef, KindResourceRefList:
		if field.TargetKind == "" {
			return fmt.Errorf("form %s field %s declares no target kind", kind, field.Wire)
		}
	case KindBindingList:
		if field.TargetKind == "" {
			return fmt.Errorf("form %s field %s declares no target kind", kind, field.Wire)
		}
		// The Binding contract travels into the desired schema as an
		// annotation, so a list without one would emit a reference no host
		// could hold to any Binding Definition.
		if field.BindingType == "" {
			return fmt.Errorf("form %s binding list %s declares no binding contract", kind, field.Wire)
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
	case KindExternalServiceList:
		// The slot shape is fixed by the standard-services contract; nothing
		// on the Field may vary it, so a declaration that tries to carry a
		// target kind, contract, or nested members is a defect.
		if field.TargetKind != "" || field.BindingType != "" || len(field.Fields) != 0 {
			return fmt.Errorf("form %s external-service list %s carries members the slot shape does not admit", kind, field.Wire)
		}
	case KindInteger, KindBoolean, KindJSONMap:
	default:
		return fmt.Errorf("form %s field %s has unknown field kind %q", kind, field.Wire, field.Kind)
	}
	return nil
}

// validateTargetContract proves one reference-shaped field states exactly one
// requirement about its target. Both would be two sources of truth for one
// dependency; neither would emit a reference that group and kind alone
// satisfy, which is precisely the hole decision 0022 closes.
func validateTargetContract(kind string, field Field) error {
	switch {
	case field.Target.ExactForm && field.Target.Interface != nil:
		return fmt.Errorf(
			"form %s reference field %s declares both an exact Form contract and a required Interface; "+
				"a relation depends on one or the other",
			kind, field.Wire,
		)
	case !field.Target.Declared():
		return fmt.Errorf(
			"form %s reference field %s declares no target contract; state the exact Form the relation "+
				"depends on, or the Interface the target must provide (decision 0022)",
			kind, field.Wire,
		)
	case field.Target.Interface != nil &&
		(field.Target.Interface.Name == "" || field.Target.Interface.Version == ""):
		return fmt.Errorf("form %s reference field %s names an incomplete required Interface", kind, field.Wire)
	}
	return nil
}

// I64 declares an inclusive integer bound.
func I64(value int64) *int64 { return &value }
