package currentformmodel

import (
	"errors"
	"fmt"
	"sort"
)

// Portable string grammars. Every pattern is RE2-safe, so the same expression
// constrains the Terraform schema, the Draft 2020-12 desired schema, and a
// host's own validator without any of them needing a different engine. These
// are declared locally on purpose: the v1alpha2 catalog is retained prior art
// and the family lane must not inherit silent edits from it.
const (
	// PatternResourceName is the envelope's metadata.name grammar, reused
	// wherever a Form references another resource by name. It is the lane's
	// grammar exactly: a name that ends in a hyphen is not a DNS label, and a
	// Form that accepted one would accept a reference to a name the envelope
	// can never carry — a hole that only ever opens on the reference side.
	PatternResourceName = `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`
	// PatternExternalServiceName is the SCREAMING_SNAKE grammar sealed-value
	// slots use: the projection mints NAME_-prefixed members, so a lowercase
	// or $-bearing name would make those unpronounceable
	// (spec/standard-services).
	PatternExternalServiceName = `^[A-Z][A-Z0-9_]*$`
	// ExclusiveAnnotationKey marks the reference whose target may be held by
	// at most one live resource of this kind. A host reads it and enforces the
	// cardinality without knowing which Form it is enforcing for.
	ExclusiveAnnotationKey = "x-takoform-exclusive"
	// SumAnnotationKey marks the object list whose elements' named integer
	// member must total an exact value. A schema can bound each element and
	// cannot add a column, so a host reading only the Definition would
	// otherwise have no way to know the total is part of the contract.
	SumAnnotationKey = "x-takoform-sum"
	// ClaimAnnotationKey marks the property whose value at most one live
	// resource per tenant may hold. The canonical form is the property's own
	// schema's to define; that the comparison happens on it is the lane's.
	ClaimAnnotationKey = "x-takoform-claim"
	// HostAssignedAnnotationKey marks the declared output the host mints. A
	// client reads it to know the value is not derivable from anything it
	// wrote and must not be reconstructed from a resource name.
	HostAssignedAnnotationKey = "x-takoform-host-assigned"
	// RequiredEntrypointAnnotationKey marks the reference whose inward
	// activation invokes one entrypoint of the target's runtime contract. The
	// lane's attachment gate reads it, so a family adds an attachment without
	// the protocol document — or a host — learning a new Form kind.
	RequiredEntrypointAnnotationKey = "x-takoform-required-entrypoint"
	// StandardServiceAnnotationKey marks the array property embedding sealed
	// external-service slots, exactly parallel to BindingAnnotationKey, so a
	// host holding only the Definition derives every slot it must enforce.
	StandardServiceAnnotationKey = "x-takoform-standard-services"
	// StandardServiceAPIVersion is the closed slot vocabulary's identity.
	StandardServiceAPIVersion = "standards.takoform.com/v1alpha1"

	// PatternBindingName is the JavaScript identifier grammar worker-family
	// binding names use (decision 0010).
	PatternBindingName = `^[A-Za-z_$][A-Za-z0-9_$]*$`
	// PatternRelativePath is a non-escaping relative module path.
	PatternRelativePath = `^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`
	// PatternHostname is a dotted DNS hostname. It admits the two spellings
	// DNS treats as one name and canonicalization removes — an uppercase
	// letter and the trailing root dot — because refusing a legitimate
	// spelling outright is not the same rule as agreeing on one identity for
	// it (see CanonicalHostname). It admits no non-ASCII byte, so an
	// internationalized name travels as its A-label.
	PatternHostname = `^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+\.?$`
	// PatternCanonicalHostname is PatternHostname less BOTH spellings
	// canonicalization removes: no trailing root dot, and no uppercase letter.
	// It is the grammar of a hostname a HOST produced rather than one an author
	// wrote. Desired state admits the variant spellings because the host
	// canonicalizes them (CanonicalHostname); an assigned value has already
	// been through that, so admitting either spelling here would let a host
	// publish a name its own canonical form forbids — and, where a Form also
	// publishes a URL built from the hostname, would admit a pair whose two
	// members cannot both be satisfied by one address.
	//
	// It is a NARROWING of this one constant rather than a third grammar
	// beside it. CanonicalHostname performs two rewrites and this constant is
	// named for their result, so a "canonical" pattern that admitted
	// `A.Example.com` was not a second legitimate rule but this rule stated
	// incompletely: it described half of what the function does. A third
	// constant would leave two spellings of one idea in the model with nothing
	// saying which an author of the next Form should reach for, and the laxer
	// one carrying the better name.
	PatternCanonicalHostname = `^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`
	// PatternCanonicalSHA256 is the algorithm-prefixed lowercase digest used
	// by every exact Takoform identity.
	PatternCanonicalSHA256 = `^sha256:[0-9a-f]{64}$`
	// PatternSensitiveVarName names one host-supplied sensitive value.
	PatternSensitiveVarName = `^[A-Z][A-Z0-9_]{0,63}$`

	// PortableMapKeyPattern is the reviewed key grammar of the data-only map
	// escape; formpackage requires this exact propertyNames declaration.
	PortableMapKeyPattern = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
	portableMapPolicyKey  = "x-takoform-fieldPolicy"
	portableMapPolicy     = "portable-data-only-v1"

	// ResourceNameMaxLength bounds every portable resource name.
	ResourceNameMaxLength = 63
	// jsonMapMaxDepth bounds nested containers inside a KindJSONMap value.
	jsonMapMaxDepth = 8
	// jsonMapMaxKeys bounds keys per object and items per array at every
	// nesting level of a KindJSONMap value.
	jsonMapMaxKeys = 64
	// jsonMapMaxStringLength bounds every string leaf of a KindJSONMap value.
	jsonMapMaxStringLength = 8192
	// bindingListMaxItems bounds declared bindings per binding list.
	bindingListMaxItems = 64
	// externalServiceNameMaxLength bounds one slot name.
	externalServiceNameMaxLength = 64
	// externalServiceListMaxItems bounds a revision's slot list.
	externalServiceListMaxItems = 16

	// bindingNameMaxLength bounds one binding instance name.
	bindingNameMaxLength = 64

	// BindingAnnotationKey names the Binding contract a binding-list property
	// carries. It is a JSON Schema annotation on the property itself, so the
	// desired schema a host already serves states which Binding Definition
	// governs each declared list. The Form Definition's acceptedBindings
	// resolves that name to the exact digest-bound BindingRef, which keeps one
	// source of truth for the digest (decision 0014).
	BindingAnnotationKey = "x-takoform-binding"

	// TargetFormRefsAnnotationKey lists the exact Form identities a
	// reference-shaped node accepts as its target. It is the annotation a
	// relation carries when it depends on the target's exact DESIRED CONTRACT —
	// when the source, or the host acting for it, reads a member of the target's
	// desired spec, or enforces a rule stated over the target Form itself
	// (decision 0022).
	TargetFormRefsAnnotationKey = "x-takoform-target-formrefs"

	// RequiredInterfaceAnnotationKey names the one exact Interface contract a
	// reference-shaped node's target must provide. It is the annotation a
	// relation carries when it depends only on an INTERFACE — when what the
	// source needs is the behavior the contract fixes and any Form providing it
	// would serve (decision 0022).
	RequiredInterfaceAnnotationKey = "x-takoform-required-interface"
)

// TargetContractResolver resolves the exact identities a declared target
// contract names.
//
// The authoring model deliberately holds no digest of its own: a schemaDigest
// is derived from rendered bytes, so only the generation pipeline that renders
// them can state it. Declaring the requirement here and resolving it there
// keeps one source of truth for every digest, exactly as `acceptedBindings`
// already does for a Binding contract (decision 0014).
type TargetContractResolver interface {
	// TargetFormRefs returns every exact Form identity this build renders for
	// one target kind.
	TargetFormRefs(targetKind string) ([]TargetFormRef, error)
	// RequiredInterface returns the exact identity of one Interface contract.
	RequiredInterface(name, version string) (RequiredInterface, error)
	// ResourceNamePattern is the name grammar THIS generation's references
	// admit. It is asked for rather than read from a constant because a
	// published generation renders the grammar it published: a shared constant
	// would let a later generation's correction silently rewrite frozen bytes,
	// which is the one thing a retained declaration set must never do.
	ResourceNamePattern() string
}

// DesiredSchema derives the Draft 2020-12 closed desired schema of a Form.
// It never contains a "name" property: the v1beta1 envelope owns
// metadata.name (decision 0011).
//
// Every reference-shaped node it emits carries exactly one target-contract
// annotation, resolved through resolver. A reference that stated only a group
// and a kind would be satisfied by a target whose Definition later moved to an
// incompatible version, because group and kind are all anyone ever checked
// (decision 0022).
func (f Form) DesiredSchema(resolver TargetContractResolver) (map[string]any, error) {
	group := f.Family.APIVersion()
	properties := map[string]any{}
	var required []string
	needsJSONMapDefs := false
	for _, field := range f.Fields {
		schema, err := field.jsonSchema(group, resolver)
		if err != nil {
			return nil, fmt.Errorf("form %s field %s: %w", f.Kind, field.Wire, err)
		}
		properties[field.Wire] = schema
		if field.Required {
			required = append(required, field.Wire)
		}
		if field.usesJSONMap() {
			needsJSONMapDefs = true
		}
	}
	sort.Strings(required)
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"title":                f.Title + " desired state",
		"description":          f.Description,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	if needsJSONMapDefs {
		schema["$defs"] = jsonMapValueDefinitions()
	}
	return schema, nil
}

// OutputSchema derives the Draft 2020-12 closed output contract of a Form: the
// exact shape of the `status.outputs` document a host returns for it. A Form
// that publishes no output returns nil, and its Definition carries no
// outputSchema at all — which is what the published wire schema reads to decide
// that `status.outputs` must then be OMITTED rather than empty
// (spec/schemas/host-api-wire-v1beta1.schema.json).
//
// Every declared output is required and the object is closed, in both
// directions on purpose. Required, because an output a host may omit is an
// output no consumer can use without a fallback, and the fallback would be that
// consumer's private guess at what the value means. Closed, because an
// undeclared member is a value the contract never described, and a client that
// typed it would be typing one host's private extension as if it were portable.
func (f Form) OutputSchema() (map[string]any, error) {
	if len(f.Outputs) == 0 {
		return nil, nil
	}
	properties := map[string]any{}
	required := make([]string, 0, len(f.Outputs))
	for _, output := range f.Outputs {
		// Outputs declare no cross-resource reference, so no target-contract
		// resolver is reachable from here; validateOutputs refuses the kinds
		// that would need one.
		schema, err := output.jsonSchema(f.Family.APIVersion(), nil)
		if err != nil {
			return nil, fmt.Errorf("form %s output %s: %w", f.Kind, output.Wire, err)
		}
		properties[output.Wire] = schema
		required = append(required, output.Wire)
	}
	sort.Strings(required)
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"title":                f.Title + " outputs",
		"description": "Host-computed values this Form publishes in status.outputs. Every member is " +
			"returned by a conforming host; the object is closed.",
		"properties": properties,
		"required":   required,
	}, nil
}

func (f Field) usesJSONMap() bool {
	if f.Kind == KindJSONMap {
		return true
	}
	for _, member := range f.Fields {
		if member.usesJSONMap() {
			return true
		}
	}
	return false
}

func (f Field) jsonSchema(group string, resolver TargetContractResolver) (map[string]any, error) {
	schema, err := f.jsonSchemaShape(group, resolver)
	if err != nil {
		return nil, err
	}
	if f.Doc != "" {
		schema["description"] = f.Doc
	}
	// The portable default travels inside the desired schema, on the property
	// that declares it. That is what makes it portable at all: it reaches every
	// host through the Form Definition the host already installed, so an
	// omitted optional field means one thing everywhere (decision 0008).
	if f.Default != nil {
		schema["default"] = cloneValue(f.Default)
	}
	return schema, nil
}

func (f Field) jsonSchemaShape(group string, resolver TargetContractResolver) (map[string]any, error) {
	switch f.Kind {
	case KindBoolean:
		return map[string]any{"type": "boolean"}, nil
	case KindInteger:
		schema := map[string]any{"type": "integer"}
		if f.Min != nil {
			schema["minimum"] = *f.Min
		}
		if f.Max != nil {
			schema["maximum"] = *f.Max
		}
		return schema, nil
	case KindString:
		schema := map[string]any{"type": "string"}
		if f.Pattern != "" {
			schema["pattern"] = f.Pattern
		} else {
			schema["minLength"] = 1
		}
		if f.MaxLength > 0 {
			schema["maxLength"] = f.MaxLength
		}
		return schema, nil
	case KindStringEnum:
		return map[string]any{"type": "string", "enum": anySlice(f.Enum)}, nil
	case KindStringSet:
		items := map[string]any{"type": "string"}
		if len(f.Enum) > 0 {
			items["enum"] = anySlice(f.Enum)
		} else {
			items["pattern"] = f.ItemPattern
		}
		return f.arraySchema(items), nil
	case KindJSONMap:
		// The reviewed typed-map escape: exact key policy, values bounded by
		// the finite depth chain in $defs. formpackage's portable schema
		// verifier proves this shape closed.
		return map[string]any{
			"type":                 "object",
			"maxProperties":        jsonMapMaxKeys,
			"propertyNames":        portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/jsonValueDepth1"},
		}, nil
	case KindResourceRef:
		return f.resourceRefSchema(group, resolver)
	case KindResourceRefList:
		reference, err := f.resourceRefSchema(group, resolver)
		if err != nil {
			return nil, err
		}
		return f.arraySchema(reference), nil
	case KindBindingList:
		reference, err := f.resourceRefSchema(group, resolver)
		if err != nil {
			return nil, err
		}
		schema := f.arraySchema(map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"name", "resource"},
			"properties": map[string]any{
				"name": map[string]any{
					"type":      "string",
					"pattern":   PatternBindingName,
					"maxLength": bindingNameMaxLength,
				},
				"resource": reference,
			},
		})
		if f.MaxItems == 0 {
			schema["maxItems"] = bindingListMaxItems
		}
		// The Binding contract this list carries is stated on the property, so
		// a host that has only the desired schema still knows which Binding
		// Definition governs each reference it finds.
		schema[BindingAnnotationKey] = f.BindingType
		return schema, nil
	case KindExternalServiceList:
		schema := f.arraySchema(map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"name", "service"},
			"properties": map[string]any{
				"name": map[string]any{
					"type":      "string",
					"pattern":   PatternExternalServiceName,
					"maxLength": externalServiceNameMaxLength,
				},
				"service": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"apiVersion", "protocol"},
					"properties": map[string]any{
						"apiVersion": map[string]any{"const": StandardServiceAPIVersion},
						"protocol":   map[string]any{"enum": toAnySlice(ExternalServiceProtocols)},
					},
				},
				"required": map[string]any{
					"type":        "boolean",
					"description": "Optional, defaulting to true. A REQUIRED slot the host cannot satisfy blocks readiness; an optional one it does not satisfy projects nothing and blocks nothing.",
				},
			},
		})
		if f.MaxItems == 0 {
			schema["maxItems"] = externalServiceListMaxItems
		}
		// The embedding property is annotated so a host holding only the
		// desired schema derives every slot it must enforce
		// (spec/standard-services, decision 0045).
		schema[StandardServiceAnnotationKey] = StandardServiceAPIVersion
		return schema, nil
	case KindObject:
		return objectSchema(group, f.Fields, resolver)
	case KindObjectList:
		members, err := objectSchema(group, f.Fields, resolver)
		if err != nil {
			return nil, err
		}
		list := f.arraySchema(members)
		return list, nil
	default:
		panic(fmt.Sprintf("unknown field kind %q", f.Kind))
	}
}

// ExternalServiceProtocols is the closed decision-0045 protocol vocabulary.
// Widening it is a reviewed spec change held to decision 0043's test.
var ExternalServiceProtocols = []string{"postgresql", "redis", "s3-compatible", "smtp"}

// ExternalServiceProjectedNames lists the runtime members one slot projects,
// per protocol (spec/standard-services): env-style consumers receive exactly
// these, so the single-namespace uniqueness rule is enforced over this
// closure rather than over the declared slot names alone.
func ExternalServiceProjectedNames(slotName, protocol string) []string {
	switch protocol {
	case "s3-compatible":
		return []string{
			slotName + "_ENDPOINT", slotName + "_REGION", slotName + "_BUCKET",
			slotName + "_ACCESS_KEY_ID", slotName + "_SECRET_ACCESS_KEY",
		}
	default:
		return []string{slotName + "_URL"}
	}
}

func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func (f Field) arraySchema(items map[string]any) map[string]any {
	schema := map[string]any{"type": "array", "uniqueItems": true, "items": items}
	if f.MinItems > 0 {
		schema["minItems"] = f.MinItems
	}
	if f.MaxItems > 0 {
		schema["maxItems"] = f.MaxItems
	}
	return schema
}

func objectSchema(group string, fields []Field, resolver TargetContractResolver) (map[string]any, error) {
	properties := map[string]any{}
	var required []string
	for _, member := range fields {
		schema, err := member.jsonSchema(group, resolver)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member.Wire, err)
		}
		properties[member.Wire] = schema
		if member.Required {
			required = append(required, member.Wire)
		}
	}
	sort.Strings(required)
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, nil
}

// resourceRefSchema is the closed three-member cross-resource reference. The
// group is pinned as a const alongside the kind: a reference that named only
// {kind, name} could never address two Form Families at once, and a host
// resolving it would have to guess which installed Form a bare kind meant.
//
// The node also carries exactly one target-contract annotation. Group and kind
// say WHICH resource; they say nothing about what that resource must still
// satisfy, so a target whose Definition moved to an incompatible version would
// keep satisfying every reference to it. The annotation states the requirement
// the source actually has, and a host verifies it before any mutation
// (decision 0022).
func (f Field) resourceRefSchema(group string, resolver TargetContractResolver) (map[string]any, error) {
	node := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"apiVersion", "kind", "name"},
		"properties": map[string]any{
			"apiVersion": map[string]any{"type": "string", "const": group},
			"kind":       map[string]any{"type": "string", "const": f.TargetKind},
			"name": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": ResourceNameMaxLength,
			},
		},
	}
	switch {
	case resolver == nil:
		return nil, errors.New("a reference-shaped field needs a target-contract resolver")
	}
	pattern := resolver.ResourceNamePattern()
	if pattern == "" {
		return nil, errors.New("a reference-shaped field needs its generation's resource-name grammar")
	}
	node["properties"].(map[string]any)["name"].(map[string]any)["pattern"] = pattern
	switch {
	case f.Target.ExactForm && f.Target.Interface != nil:
		return nil, errors.New("a reference states either an exact Form contract or an Interface, never both")
	case f.Target.ExactForm:
		refs, err := resolver.TargetFormRefs(f.TargetKind)
		if err != nil {
			return nil, err
		}
		if len(refs) == 0 {
			return nil, fmt.Errorf("no exact Form identity is rendered for target kind %q", f.TargetKind)
		}
		encoded := make([]any, 0, len(refs))
		for _, ref := range refs {
			if ref.Kind != f.TargetKind {
				return nil, fmt.Errorf("target Form identity %s is not of kind %q", ref.Kind, f.TargetKind)
			}
			encoded = append(encoded, map[string]any{
				"apiVersion":        ref.APIVersion,
				"kind":              ref.Kind,
				"definitionVersion": ref.DefinitionVersion,
				"schemaDigest":      ref.SchemaDigest,
			})
		}
		node[TargetFormRefsAnnotationKey] = encoded
	case f.Target.Interface != nil:
		required, err := resolver.RequiredInterface(f.Target.Interface.Name, f.Target.Interface.Version)
		if err != nil {
			return nil, err
		}
		node[RequiredInterfaceAnnotationKey] = map[string]any{
			"apiVersion":   required.APIVersion,
			"name":         required.Name,
			"version":      required.Version,
			"schemaDigest": required.SchemaDigest,
		}
	default:
		return nil, errors.New("a reference must state either an exact Form contract or a required Interface")
	}
	if f.RequiredEntrypoint != "" {
		node[RequiredEntrypointAnnotationKey] = f.RequiredEntrypoint
	}
	return node, nil
}

func portableMapKeys() map[string]any {
	return map[string]any{
		"type":               "string",
		"pattern":            PortableMapKeyPattern,
		portableMapPolicyKey: portableMapPolicy,
	}
}

// jsonMapValueDefinitions unrolls the bounded any-JSON-value schema of the
// data-only vars map. Cyclic references are rejected by the portable closure
// proof, so the depth bound is expressed as a finite chain: containers at
// depth N admit values at depth N+1, and the deepest level admits scalars
// only.
func jsonMapValueDefinitions() map[string]any {
	defs := map[string]any{}
	for depth := 1; depth < jsonMapMaxDepth; depth++ {
		defs[fmt.Sprintf("jsonValueDepth%d", depth)] = map[string]any{
			"type":                 []any{"array", "boolean", "null", "number", "object", "string"},
			"maxLength":            jsonMapMaxStringLength,
			"maxItems":             jsonMapMaxKeys,
			"maxProperties":        jsonMapMaxKeys,
			"items":                map[string]any{"$ref": fmt.Sprintf("#/$defs/jsonValueDepth%d", depth+1)},
			"propertyNames":        portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": fmt.Sprintf("#/$defs/jsonValueDepth%d", depth+1)},
		}
	}
	defs[fmt.Sprintf("jsonValueDepth%d", jsonMapMaxDepth)] = map[string]any{
		"type":      []any{"boolean", "null", "number", "string"},
		"maxLength": jsonMapMaxStringLength,
	}
	return defs
}

func anySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
