// Package currentformmodel is the shared rich authoring model for retained
// retained v1beta1 Form Definitions and the current closed stable-v1 vocabulary.
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
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Family is one Form Family API group and its group version. It renders the
// namespaced apiVersion of every member FormRef, for example
// "edge.forms.takoform.com/v1beta1".
type Family struct {
	Group string
	// Version is the group's version segment, EMPTY for a family that carries
	// none. A version here never varied independently of the exact FormRef
	// that already carries kind, definitionVersion and schemaDigest — its only
	// effect was to make every member move together (decision 0049). Retained
	// generations keep theirs because their bytes are published.
	Version string
}

// APIVersion renders the DNS-like namespaced group, with its version when it
// has one.
func (f Family) APIVersion() string {
	if f.Version == "" {
		return f.Group
	}
	return f.Group + "/" + f.Version
}

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
	// KindStringList is an ordered string array. Duplicates are values, not an
	// authoring error: command/argument lists may repeat the same token.
	KindStringList FieldKind = "string-list"
	// KindStringSet is a unique-item string array closed by ItemPattern or Enum.
	KindStringSet FieldKind = "string-set"
	// KindStringMap is a bounded map from portable keys to bounded strings.
	KindStringMap FieldKind = "string-map"
	// KindStringSetMap is a bounded map whose values are bounded string sets.
	// Defaults and examples sort each set lexically so a semantic set has one
	// deterministic document spelling.
	KindStringSetMap FieldKind = "string-set-map"
	// KindJSONMap is the data-only vars map: reviewed key grammar, any JSON
	// value bounded to depth 8 containers, and a per-object key ceiling.
	KindJSONMap FieldKind = "json-map"
	// KindResourceRef references one resource of an exact target kind by name.
	KindResourceRef     FieldKind = "resource-ref"
	KindResourceRefList FieldKind = "resource-ref-list"
	// KindExternalServiceList declares sealed external standard-service
	// slots: a SCREAMING_SNAKE binding name plus an opaque normalized
	// reverse-DNS protocol identifier, with an optional requiredness flag.
	// Portable state never carries the endpoint or credential that satisfies
	// a slot; the Host integration projects one sealed runtime-native binding
	// under the slot name and owns its internal entries.
	KindExternalServiceList FieldKind = "external-service-list"
	// KindBindingList declares typed capability bindings: binding name plus a
	// target-kind resource reference (decision 0010).
	KindBindingList FieldKind = "binding-list"
	KindObjectList  FieldKind = "object-list"
	KindObject      FieldKind = "object"
	// KindTaggedObject is a closed discriminated union. Every variant is a
	// closed object with one discriminator const.
	KindTaggedObject FieldKind = "tagged-object"
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
	// MinProperties and MaxProperties bound typed map entries. Map keys use the
	// reviewed portable key grammar and RFC 8785 supplies deterministic key
	// order at canonical encoding time.
	MinProperties, MaxProperties int
	// ResourceTarget is the exact cross-family address and contract of a new
	// reference-shaped field. Group is never inherited from the source Form:
	// two families may use the same kind name, and resolving by kind alone would
	// make the target depend on which catalogs a host happened to install.
	ResourceTarget *ResourceTarget
	// TargetKind and Target are the retained same-family authoring spelling used
	// by the already-published v1beta1 declarations. New declarations use the
	// aggregate ResourceTarget above. Keeping the retained spelling here avoids
	// rewriting immutable Definition bytes while the one helper below gives both
	// spellings identical schema and relation behavior.
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
	// Discriminator and Variants declare KindTaggedObject. Discriminator is the
	// wire member whose const selects exactly one closed variant.
	Discriminator string
	Variants      []TaggedObjectVariant

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
	// RequiredEntrypoint names the entrypoint of the target's runtime
	// Interface that this reference's inward activation invokes. It is what
	// makes the lane's attachment gate derivable from a Definition instead of
	// from a table of Form kinds the protocol document would have to carry: a
	// host holding only this document knows which export every weighted
	// version of the target must have.
	RequiredEntrypoint string
	// Exclusive declares that at most one LIVE resource of this Form kind may
	// hold the target this reference resolves to. It is the mechanism four
	// separate hand-written rules used to be — one active deployment per
	// worker, one consumer per queue, one live migration application per
	// database, one class holder per worker and class — each of which was a
	// paragraph in the protocol document naming a Form kind, and therefore a
	// reason the protocol had to change whenever a family gained one.
	Exclusive *ExclusiveHold
	// Sum declares that one integer member of this object list's elements
	// must total an exact value. A schema bounds each element and cannot add
	// a column, so this was a sentence in the protocol document about one
	// Form's traffic weights — which is a reason the protocol had to change
	// whenever a family gained a list that sums.
	Sum *SummedMember
	// Claimed declares that this property's value is held by at most one live
	// resource per tenant, across every space, compared on the canonical form
	// the property's own schema defines. It was a paragraph in the protocol
	// document about one Form's hostnames — a reason the protocol had to
	// change whenever a family gained a value that is claimed rather than
	// merely unique.
	Claimed bool
	// HostAssigned marks a declared OUTPUT the host mints: it is immutable for
	// the lifetime of the resource's UID and no desired property may state it.
	// It was a paragraph in the protocol document about one Form's endpoint
	// address, which is a reason the protocol had to change whenever a family
	// gained an address it hands out.
	HostAssigned bool

	// Example is the value used by the canonical conformance fixture.
	Example any
	// AltExample is a second valid value for immutable-field lifecycle proofs.
	AltExample any
	// CounterExample is a value the desired schema must reject. When nil, one
	// is derived from the declared constraint where possible.
	CounterExample any
}

// TaggedObjectVariant is one closed branch of a KindTaggedObject. Tag is the
// discriminator value and Fields are every other member admitted in that
// branch.
type TaggedObjectVariant struct {
	Tag    string
	Fields []Field
}

// SummedMember is the declared cross-element arithmetic of one object list:
// the member that is added up, and the total it must reach exactly.
type SummedMember struct {
	// Member is the wire name of the integer member summed across elements.
	Member string
	// Total is the exact value those members must add to.
	Total int64
}

// Constraint is one entry of a Form Definition's closed constraint list.
//
// These are rules about RESOURCES, not about the shape of a document, so they
// do not belong in a JSON Schema — where they rode in extension slots no
// standard validator reads (decision 0049). As a first-class list with a
// closed `Kind` vocabulary, adding a constraint kind is one reviewed change in
// one place instead of a new `x-` key that may appear anywhere in a schema
// tree, and the desired schema goes back to being plain JSON Schema.
//
// Every pointer is an RFC 6901 JSON Pointer into the DESIRED instance (or, for
// a host-assigned member, into the outputs), which is the same addressing the
// lane already uses for relations.
type Constraint struct {
	Kind ConstraintKind `json:"kind"`
	// Reference is the relation an exclusive hold is taken through.
	Reference string `json:"reference,omitempty"`
	// KeyedBy narrows an exclusive hold to one value of a sibling member, so
	// one target may carry one holder per key rather than one in total.
	KeyedBy string `json:"keyedBy,omitempty"`
	// List, Member and Total carry a summed list: which list, which integer
	// member of its elements, and the exact figure they must reach.
	List   string `json:"list,omitempty"`
	Member string `json:"member,omitempty"`
	Total  int64  `json:"total,omitempty"`
	// Property is the claimed desired member.
	Property string `json:"property,omitempty"`
	// Output is the declared output member the host mints.
	Output string `json:"output,omitempty"`
	// References is one ordered pair. orderedPair compares two required numeric
	// desired values; distinctPair and uniquePair compare two resolved
	// relations. The kind determines the value domain and no coercion occurs.
	References []string `json:"references,omitempty"`
	// Anchor, Members and Through define sameResolvedTarget. Anchor is one
	// resolved relation in this resource; Members is a list relation in this
	// resource; Through is the single relation each resolved member target must
	// itself hold to Anchor's UID.
	Anchor  string `json:"anchor,omitempty"`
	Members string `json:"members,omitempty"`
	Through string `json:"through,omitempty"`
}

// ConstraintKind is the closed vocabulary. A kind outside it is not a
// constraint this lane knows, and a host refuses a Definition carrying one
// rather than ignoring it — an ignored constraint is an unenforced rule.
type ConstraintKind string

const (
	// ConstraintExclusive: at most one LIVE resource of this Form's kind may
	// hold the target its reference resolves to.
	ConstraintExclusive ConstraintKind = "exclusive"
	// ConstraintSum: the named integer member of a list's elements totals
	// exactly the stated figure.
	ConstraintSum ConstraintKind = "sum"
	// ConstraintClaim: at most one live resource per tenant holds this value,
	// compared on the canonical form the property's own schema admits.
	ConstraintClaim ConstraintKind = "claim"
	// ConstraintHostAssigned: the host mints this output; no desired property
	// states it and no configuration reconstructs it.
	ConstraintHostAssigned ConstraintKind = "hostAssigned"
	// ConstraintOrderedPair requires the numeric value at References[0] to be
	// less than or equal to the numeric value at References[1]. Both pointers
	// name required desired fields.
	ConstraintOrderedPair ConstraintKind = "orderedPair"
	// ConstraintUniqueBy requires one scalar member to be unique among the
	// elements of one object list.
	ConstraintUniqueBy ConstraintKind = "uniqueBy"
	// ConstraintAcyclic rejects a relation edge that would close a UID graph
	// cycle through the same declared relation.
	ConstraintAcyclic ConstraintKind = "acyclic"
	// ConstraintDistinctPair requires two relations in one desired resource to
	// resolve to different UIDs.
	ConstraintDistinctPair ConstraintKind = "distinctPair"
	// ConstraintUniquePair allows at most one live resource of this Form kind to
	// hold one ordered pair of resolved UIDs.
	ConstraintUniquePair ConstraintKind = "uniquePair"
	// ConstraintSameResolvedTarget requires every member target's Through
	// relation to resolve to the Anchor UID.
	ConstraintSameResolvedTarget ConstraintKind = "sameResolvedTarget"
)

// Constraints derives the Form's constraint list from what its fields and
// outputs declare. Authoring stays on the member the rule is about — which is
// where a reader looks for it — and the PUBLISHED document carries one list,
// which is where an implementer looks for it.
func (f Form) Constraints() []Constraint {
	out := fieldConstraints(f.Fields, "")
	for _, output := range f.Outputs {
		if output.HostAssigned {
			out = append(out, Constraint{Kind: ConstraintHostAssigned, Output: "/" + output.Wire})
		}
	}
	out = append(out, f.StructuralConstraints...)
	out = append(out, f.ResolvedUIDConstraints...)
	if len(out) == 0 {
		return nil
	}
	return out
}

// fieldConstraints walks the same recursive object/list/tagged tree as schema
// and relation derivation. A marker on a nested member therefore cannot be
// accepted by authoring validation and then disappear from the published
// Definition. Exact duplicates can arise when two tagged variants deliberately
// share one identical member shape; they are one pointer rule, so emit it once.
func fieldConstraints(fields []Field, parent string) []Constraint {
	var out []Constraint
	seen := map[string]bool{}
	var walk func([]Field, string)
	appendUnique := func(entry Constraint) {
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d", entry.Kind, entry.Reference, entry.KeyedBy, entry.List, entry.Member, entry.Total)
		key += "\x00" + entry.Property
		if !seen[key] {
			seen[key] = true
			out = append(out, entry)
		}
	}
	walk = func(fields []Field, prefix string) {
		for _, field := range fields {
			pointer := prefix + "/" + escapeJSONPointerToken(field.Wire)
			if field.Exclusive != nil {
				appendUnique(Constraint{Kind: ConstraintExclusive, Reference: pointer, KeyedBy: field.Exclusive.KeyedBy})
			}
			if field.Sum != nil {
				appendUnique(Constraint{Kind: ConstraintSum, List: pointer, Member: field.Sum.Member, Total: field.Sum.Total})
			}
			if field.Claimed {
				appendUnique(Constraint{Kind: ConstraintClaim, Property: pointer})
			}
			switch field.Kind {
			case KindObject:
				walk(field.Fields, pointer)
			case KindObjectList:
				walk(field.Fields, pointer+"/*")
			case KindTaggedObject:
				for _, variant := range field.Variants {
					walk(variant.Fields, pointer)
				}
			}
		}
	}
	walk(fields, parent)
	return out
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
	// aggregate resolver returns for the ResourceTarget's group and kind.
	ExactForm bool
	// Interface names the exact Interface contract the target must provide.
	Interface *InterfaceRefSource
}

// Declared reports whether this field states a target contract at all.
func (t TargetContract) Declared() bool { return t.ExactForm || t.Interface != nil }

// ResourceTarget is one exact cross-resource dependency: the target Form
// group, kind, and the one contract the source requires of it. Group is the
// complete Form group identity as it appears in FormRef.apiVersion; it may be a
// retained versioned group or a current versionless group, but it is never a
// floating family/catalog lookup.
type ResourceTarget struct {
	Group    string
	Kind     string
	Contract TargetContract
}

// Declared reports whether any part of the target was supplied.
func (t ResourceTarget) Declared() bool {
	return t.Group != "" || t.Kind != "" || t.Contract.Declared()
}

// BindingRefSource names an exact Binding contract by name and version.
type BindingRefSource struct {
	Name    string
	Version string
}

// Form is one member of a Form Family.
type Form struct {
	// Family is the API group this Form belongs to. Retained same-family
	// references inherit it; every new ResourceTarget pins its own group, so a
	// source may address another family without a kind-only catalog lookup.
	Family Family
	Kind   string // PascalCase portable kind
	Slug   string // kebab-case package directory
	Role   Role
	// RequiresHostAPI is the earliest Host API lane this Form's contract needs
	// (decision 0047). It is declared per FORM, not per family: what a Form
	// needs from the substrate is a property of its own contract, so one
	// generation may hold members with different requirements.
	RequiresHostAPI string
	Title           string
	Description     string
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
	// StructuralConstraints carries closed rules across desired properties
	// that standard JSON Schema cannot express: numeric ordering between two
	// required fields and scalar uniqueness within one object list. They are
	// provider-neutral Form semantics and remain distinct from UID resolution.
	StructuralConstraints []Constraint
	// ResolvedUIDConstraints carries only the four closed cross-relation rules
	// whose truth is decided after name resolution. Document-shape constraints
	// remain JSON Schema; no universal graph expression is admitted here.
	ResolvedUIDConstraints []Constraint
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

// hostAPILanePattern is the Host API lane identity grammar a Form states its
// substrate requirement in (decision 0047).
var hostAPILanePattern = regexp.MustCompile(`^forms\.takoform\.com/v[0-9]+(?:(?:alpha|beta)[0-9]+)?$`)
var formKindPattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
var formSlugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// Validate proves the structural rules a Form must satisfy before any surface
// is derived from it.
func (f Form) Validate() error {
	// Every Form states the substrate it needs. It is the one dependency a
	// Form has always had and the only one that used to travel by convention,
	// which is what let a family and a lane move only together (decision
	// 0047).
	if !hostAPILanePattern.MatchString(f.RequiresHostAPI) {
		return fmt.Errorf("form %s declares requiresHostApi %q, which is not a Host API lane identity", f.Kind, f.RequiresHostAPI)
	}
	if f.Kind == "" || f.Slug == "" || f.Title == "" || f.DefinitionVersion == "" {
		return fmt.Errorf("form %q is missing identity fields", f.Kind)
	}
	if !formKindPattern.MatchString(f.Kind) || !formSlugPattern.MatchString(f.Slug) || len(f.Slug) > 127 {
		return fmt.Errorf("form %q has an unsafe authoring kind or package slug %q", f.Kind, f.Slug)
	}
	if !f.Role.Valid() {
		return fmt.Errorf("form %s declares unknown role %q", f.Kind, f.Role)
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
		if err := validateField(f.Kind, f.Family.APIVersion(), field); err != nil {
			return err
		}
	}
	if err := f.validateOutputs(seen); err != nil {
		return err
	}
	if len(f.AcceptedBindings) > 0 && f.Role != RoleRevision {
		return fmt.Errorf("form %s role %s accepts bindings; only revision Forms hold them", f.Kind, f.Role)
	}
	// A Form that declares a relation must carry its own group identity. Retained
	// same-family references also inherit this value; without it their emitted
	// schema would pin an empty apiVersion and be unresolvable.
	// The GROUP is what must be present; the version segment is optional,
	// because a group carries one only while it has a published generation to
	// keep apart (decision 0049).
	if f.declaresReference() && f.Family.Group == "" {
		return fmt.Errorf("form %s declares a cross-resource reference without a Family group", f.Kind)
	}
	if err := validateResolvedUIDConstraints(f); err != nil {
		return err
	}
	if err := validateStructuralConstraints(f); err != nil {
		return err
	}
	minimum := f.minimumRequiredHostAPI()
	sufficient, err := hostAPILaneAtLeast(f.RequiresHostAPI, minimum)
	if err != nil {
		return fmt.Errorf("form %s compares requiresHostApi: %w", f.Kind, err)
	}
	if !sufficient {
		return fmt.Errorf(
			"form %s declares requiresHostApi %s below the mechanism-derived minimum %s",
			f.Kind, f.RequiresHostAPI, minimum,
		)
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
		case output.ResourceTarget != nil || output.Target.Declared() || output.TargetKind != "" || output.BindingType != "":
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
		for _, variant := range field.Variants {
			if fieldsDeclareReference(variant.Fields) {
				return true
			}
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

func validateField(kind, sourceGroup string, field Field) error {
	if field.HCL == "" || field.Wire == "" || field.Doc == "" {
		return fmt.Errorf("form %s field %q must declare HCL, Wire, and Doc", kind, field.Wire)
	}
	if field.MinItems < 0 || field.MaxItems < 0 ||
		(field.MaxItems > 0 && field.MinItems > field.MaxItems) {
		return fmt.Errorf(
			"form %s field %s declares invalid item bounds %d through %d",
			kind, field.Wire, field.MinItems, field.MaxItems,
		)
	}
	if field.MinProperties < 0 || field.MaxProperties < 0 ||
		(field.MaxProperties > 0 && field.MinProperties > field.MaxProperties) {
		return fmt.Errorf(
			"form %s field %s declares invalid property bounds %d through %d",
			kind, field.Wire, field.MinProperties, field.MaxProperties,
		)
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
	if err := validateFieldDefault(kind, sourceGroup, field); err != nil {
		return err
	}
	if field.HostAssigned && field.Kind != KindString {
		return fmt.Errorf(
			"form %s output %s is host-assigned on kind %q; a minted address is a string",
			kind, field.Wire, field.Kind,
		)
	}
	if field.Claimed && field.Kind != KindString {
		return fmt.Errorf(
			"form %s field %s is claimed on kind %q; a claim is compared on a canonical STRING, "+
				"and a value with no canonical spelling has nothing to compare",
			kind, field.Wire, field.Kind,
		)
	}
	if field.Sum != nil {
		if field.Kind != KindObjectList {
			return fmt.Errorf(
				"form %s field %s declares a summed member on kind %q; only an object list has elements to add up",
				kind, field.Wire, field.Kind,
			)
		}
		summed := false
		for _, member := range field.Fields {
			if member.Wire != field.Sum.Member {
				continue
			}
			if member.Kind != KindInteger {
				return fmt.Errorf(
					"form %s field %s sums member %s, which is %q rather than an integer",
					kind, field.Wire, member.Wire, member.Kind,
				)
			}
			summed = true
		}
		if !summed {
			return fmt.Errorf(
				"form %s field %s sums member %s, which its elements do not declare",
				kind, field.Wire, field.Sum.Member,
			)
		}
	}
	if field.RequiredEntrypoint != "" && field.Kind != KindResourceRef {
		return fmt.Errorf(
			"form %s field %s requires entrypoint %q on kind %q; only a reference activates a target",
			kind, field.Wire, field.RequiredEntrypoint, field.Kind,
		)
	}
	if field.ProjectsEnvironmentNames {
		switch field.Kind {
		// An external-service list contributes the sealed runtime-native
		// binding key. Its opaque protocol integration owns the binding's
		// internal entries; the Form model must not duplicate that projection.
		case KindJSONMap, KindStringSet, KindExternalServiceList:
		default:
			return fmt.Errorf(
				"form %s field %s marks ProjectsEnvironmentNames on kind %q; only a json-map's keys, "+
					"a string-set's items, or an external-service list's slot names can name "+
					"environment entries",
				kind, field.Wire, field.Kind,
			)
		}
	}
	switch field.Kind {
	case KindResourceRef, KindResourceRefList, KindBindingList:
		if err := validateResourceTarget(kind, field); err != nil {
			return err
		}
	default:
		if field.ResourceTarget != nil || field.Target.Declared() || field.TargetKind != "" {
			return fmt.Errorf(
				"form %s field %s declares a ResourceTarget on kind %q; only a reference-shaped field points at another resource",
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
	case KindStringList:
		if len(field.Enum) == 0 && field.ItemPattern == "" {
			return fmt.Errorf("form %s string collection field %s is open; declare Enum or ItemPattern", kind, field.Wire)
		}
		if field.MaxItems == 0 {
			return fmt.Errorf("form %s string collection field %s is unbounded; declare MaxItems", kind, field.Wire)
		}
		if len(field.Enum) == 0 && field.MaxLength == 0 {
			return fmt.Errorf("form %s string collection field %s has unbounded string items; declare MaxLength", kind, field.Wire)
		}
	case KindStringSet:
		if len(field.Enum) == 0 && field.ItemPattern == "" {
			return fmt.Errorf("form %s string-set field %s is unbounded; declare Enum or ItemPattern", kind, field.Wire)
		}
	case KindStringMap:
		if field.MaxProperties == 0 {
			return fmt.Errorf("form %s string-map field %s is not bounded; declare MaxProperties", kind, field.Wire)
		}
		if len(field.Enum) == 0 && (field.ItemPattern == "" || field.MaxLength == 0) {
			return fmt.Errorf("form %s string-map field %s has unbounded values; declare Enum or ItemPattern plus MaxLength", kind, field.Wire)
		}
	case KindStringSetMap:
		if field.MaxProperties == 0 || field.MaxItems == 0 {
			return fmt.Errorf("form %s string-set-map field %s is not bounded; declare MaxProperties and MaxItems", kind, field.Wire)
		}
		if len(field.Enum) == 0 && (field.ItemPattern == "" || field.MaxLength == 0) {
			return fmt.Errorf("form %s string-set-map field %s has unbounded values; declare Enum or ItemPattern plus MaxLength", kind, field.Wire)
		}
	case KindResourceRef, KindResourceRefList:
		if field.ResourceTarget == nil && field.TargetKind == "" {
			return fmt.Errorf("form %s field %s declares no target kind", kind, field.Wire)
		}
	case KindBindingList:
		if field.ResourceTarget == nil && field.TargetKind == "" {
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
			if err := validateField(kind, sourceGroup, member); err != nil {
				return err
			}
		}
	case KindTaggedObject:
		if field.Discriminator == "" {
			return fmt.Errorf("form %s tagged object %s declares no discriminator", kind, field.Wire)
		}
		if len(field.Fields) != 0 {
			return fmt.Errorf("form %s tagged object %s declares shared Fields; every member belongs to one closed variant", kind, field.Wire)
		}
		if len(field.Variants) < 2 || len(field.Variants) > 16 {
			return fmt.Errorf("form %s tagged object %s declares %d variants; require 2 through 16", kind, field.Wire, len(field.Variants))
		}
		seenTags := map[string]struct{}{}
		for _, variant := range field.Variants {
			if !taggedVariantPattern.MatchString(variant.Tag) {
				return fmt.Errorf("form %s tagged object %s declares invalid variant tag %q", kind, field.Wire, variant.Tag)
			}
			if _, duplicate := seenTags[variant.Tag]; duplicate {
				return fmt.Errorf("form %s tagged object %s repeats variant tag %q", kind, field.Wire, variant.Tag)
			}
			seenTags[variant.Tag] = struct{}{}
			seenMembers := map[string]struct{}{}
			for _, member := range variant.Fields {
				if member.Wire == field.Discriminator {
					return fmt.Errorf("form %s tagged object %s variant %s redeclares discriminator %s", kind, field.Wire, variant.Tag, field.Discriminator)
				}
				if _, duplicate := seenMembers[member.Wire]; duplicate {
					return fmt.Errorf("form %s tagged object %s variant %s repeats member %s", kind, field.Wire, variant.Tag, member.Wire)
				}
				seenMembers[member.Wire] = struct{}{}
				if err := validateField(kind, sourceGroup, member); err != nil {
					return err
				}
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

// effectiveResourceTarget returns the new aggregate target or translates the
// retained same-family spelling without changing its emitted bytes.
func (f Field) effectiveResourceTarget(sourceGroup string) (ResourceTarget, error) {
	if f.ResourceTarget != nil {
		if f.TargetKind != "" || f.Target.Declared() {
			return ResourceTarget{}, errors.New("a reference declares both ResourceTarget and retained TargetKind/Target members")
		}
		return *f.ResourceTarget, nil
	}
	return ResourceTarget{Group: sourceGroup, Kind: f.TargetKind, Contract: f.Target}, nil
}

// EffectiveResourceTarget returns the complete target address and contract
// used by provider/client adapters. New fields keep their explicit group;
// retained declarations are translated through sourceGroup without exposing a
// kind-only lookup seam.
func (f Field) EffectiveResourceTarget(sourceGroup string) (ResourceTarget, error) {
	return f.effectiveResourceTarget(sourceGroup)
}

var formGroupPattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+(?:/v[0-9]+(?:(?:alpha|beta)[0-9]+)?)?$`,
)

var taggedVariantPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,63}$`)

// validateResourceTarget proves that a reference has one complete exact
// address and one target contract. The new spelling names its own group; the
// retained spelling receives the source group when the schema is rendered.
func validateResourceTarget(kind string, field Field) error {
	if field.ResourceTarget == nil {
		return validateTargetContract(kind, field.Wire, field.Target)
	}
	if field.TargetKind != "" || field.Target.Declared() {
		return fmt.Errorf(
			"form %s reference field %s declares both ResourceTarget and retained TargetKind/Target members",
			kind, field.Wire,
		)
	}
	target := *field.ResourceTarget
	if !formGroupPattern.MatchString(target.Group) {
		return fmt.Errorf("form %s reference field %s declares invalid target group %q", kind, field.Wire, target.Group)
	}
	if !formKindPattern.MatchString(target.Kind) {
		return fmt.Errorf("form %s reference field %s declares invalid target kind %q", kind, field.Wire, target.Kind)
	}
	return validateTargetContract(kind, field.Wire, target.Contract)
}

// validateTargetContract proves one reference-shaped field states exactly one
// requirement about its target. Both would be two sources of truth for one
// dependency; neither would emit a reference that group and kind alone
// satisfy, which is precisely the hole decision 0022 closes.
func validateTargetContract(kind, wire string, contract TargetContract) error {
	switch {
	case contract.ExactForm && contract.Interface != nil:
		return fmt.Errorf(
			"form %s reference field %s declares both an exact Form contract and a required Interface; "+
				"a relation depends on one or the other",
			kind, wire,
		)
	case !contract.Declared():
		return fmt.Errorf(
			"form %s reference field %s declares no target contract; state the exact Form the relation "+
				"depends on, or the Interface the target must provide (decision 0022)",
			kind, wire,
		)
	case contract.Interface != nil &&
		(contract.Interface.Name == "" || contract.Interface.Version == ""):
		return fmt.Errorf("form %s reference field %s names an incomplete required Interface", kind, wire)
	}
	return nil
}

// I64 declares an inclusive integer bound.
func I64(value int64) *int64 { return &value }
