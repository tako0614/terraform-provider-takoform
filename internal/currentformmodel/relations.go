package currentformmodel

import (
	"errors"
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
	// VariantSelectors is the closed tagged-object branch (or nested branches)
	// this relation belongs to. Instance traversal reads a relation only when
	// every discriminator selects that branch.
	VariantSelectors []VariantSelector
	// TargetFormRefs is the closed set of exact Form identities this relation
	// accepts, or nil when the relation states an Interface requirement instead.
	TargetFormRefs []TargetFormRef
	// Exclusive is the declared cardinality of this reference, or nil when the
	// reference states none. A host reads it and enforces "at most one live
	// holder of this target" without knowing which Form kind it is enforcing
	// for, which is what keeps the rule out of the protocol document.
	Exclusive *ExclusiveHold
	// RequiredInterface is the exact Interface contract the target must
	// provide, or nil when the relation pins exact Forms instead.
	//
	// Exactly one of the two is set: a reference that stated neither would be
	// satisfied by any resource of the right group and kind, whatever contract
	// its Definition has since moved to (decision 0022).
	RequiredInterface *RequiredInterface
}

// VariantSelector pins one relation to one closed tagged-object branch.
type VariantSelector struct {
	Pointer string
	Value   string
}

// TargetFormRef is one exact Form identity a relation accepts as its target.
// It is the same four-member identity a FormRef carries everywhere else.
type TargetFormRef struct {
	APIVersion        string
	Kind              string
	DefinitionVersion string
	SchemaDigest      string
}

// String renders the identity for a refusal message.
func (t TargetFormRef) String() string {
	return t.APIVersion + " " + t.Kind + "@" + t.DefinitionVersion + " " + t.SchemaDigest
}

// ExclusiveHold is the declared cardinality of a reference: how many live
// resources of one kind may point at one resolved target.
//
// KeyedBy optionally names a sibling property of the desired spec, by JSON
// Pointer, that joins the target in the key. Without it the target alone is
// the key — a queue has one consumer. With it the pair is — one worker carries
// one holder PER CLASS NAME, and two holders of different classes on one
// worker are not a conflict.
//
// It says nothing about what a host does with a conflict beyond refusing it,
// because there is nothing else to say: the request is well formed and what is
// untrue is what it says about the target it points at.
type ExclusiveHold struct {
	KeyedBy string
}

// RequiredInterface is the exact Interface contract a relation's target must
// provide. The digest is carried because "provides a KV interface" is worth
// nothing until it says WHICH contract (decision 0010).
type RequiredInterface struct {
	APIVersion   string
	Name         string
	Version      string
	SchemaDigest string
}

// String renders the contract for a refusal message.
func (r RequiredInterface) String() string {
	return r.Name + "@" + r.Version + " " + r.SchemaDigest
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
	return DeriveRelationsWithConstraints(schema, nil)
}

// DeriveRelationsWithConstraints is DeriveRelations plus the Definition's
// constraint list, which is where an exclusive hold now lives: it is a rule
// about resources rather than about the shape of a document, so it no longer
// rides in the schema (decision 0049). A hold naming a pointer no relation
// occupies is a Definition that contradicts itself and is refused, because a
// constraint nothing enforces is worse than one nobody wrote.
func DeriveRelationsWithConstraints(schema map[string]any, constraints []Constraint) ([]Relation, error) {
	walker := relationWalker{}
	if err := walker.walk(schema, "", "", true); err != nil {
		return nil, err
	}
	sort.Slice(walker.out, func(i, j int) bool {
		return relationSortKey(walker.out[i]) < relationSortKey(walker.out[j])
	})
	for _, constraint := range constraints {
		// The vocabulary is CLOSED, and an unreadable entry is refused rather
		// than skipped. Skipping is what a `continue` on anything that is not
		// an exclusive hold did, and it meant a Form could declare a rule this
		// host does not implement, install cleanly, and enforce nothing — the
		// Definition promising a constraint no one keeps. Whoever calls this
		// turns the error into unsupported_capability at install time.
		switch constraint.Kind {
		case ConstraintExclusive:
		case ConstraintSum:
			if constraint.List == "" || constraint.Member == "" {
				return nil, fmt.Errorf("a summed member names %q in %q, and a sum needs both",
					constraint.Member, constraint.List)
			}
			continue
		case ConstraintClaim:
			if constraint.Property == "" {
				return nil, errors.New("a claim names no property")
			}
			continue
		case ConstraintHostAssigned:
			if constraint.Output == "" {
				return nil, errors.New("a host-assigned constraint names no output")
			}
			continue
		case ConstraintOrderedPair, ConstraintUniqueBy:
			if err := validateStructuralConstraintAgainstSchema(schema, constraint); err != nil {
				return nil, err
			}
			continue
		case ConstraintAcyclic, ConstraintDistinctPair, ConstraintUniquePair, ConstraintSameResolvedTarget:
			if err := validateResolvedUIDConstraintShape(constraint); err != nil {
				return nil, err
			}
			for _, pointer := range localConstraintPointers(constraint) {
				declared := false
				for _, relation := range walker.out {
					if relation.Pointer == pointer {
						declared = true
						break
					}
				}
				if !declared {
					return nil, fmt.Errorf(
						"resolved-UID constraint %s names %s, which is not a reference this Form declares",
						constraint.Kind, pointer,
					)
				}
			}
			continue
		default:
			return nil, fmt.Errorf(
				"constraint kind %q is not one this host implements; the closed vocabulary is "+
					"exclusive, sum, claim, hostAssigned, orderedPair, uniqueBy, acyclic, distinctPair, uniquePair, sameResolvedTarget",
				constraint.Kind,
			)
		}
		if constraint.Reference == "" {
			return nil, errors.New("an exclusive hold names no reference")
		}
		attached := false
		for index := range walker.out {
			if walker.out[index].Pointer != constraint.Reference {
				continue
			}
			hold := &ExclusiveHold{KeyedBy: constraint.KeyedBy}
			walker.out[index].Exclusive = hold
			attached = true
		}
		if !attached {
			return nil, fmt.Errorf(
				"an exclusive hold names %s, which is not a reference this Form declares",
				constraint.Reference,
			)
		}
	}
	return walker.out, nil
}

func relationSortKey(relation Relation) string {
	var key strings.Builder
	key.WriteString(relation.Pointer)
	for _, selector := range relation.VariantSelectors {
		key.WriteByte(0)
		key.WriteString(selector.Pointer)
		key.WriteByte('=')
		key.WriteString(selector.Value)
	}
	key.WriteByte(0)
	key.WriteString(relation.TargetAPIVersion)
	key.WriteByte(0)
	key.WriteString(relation.TargetKind)
	key.WriteByte(0)
	key.WriteString(relation.Binding)
	return key.String()
}

// maxRelationDepth bounds the derivation walk. The desired schemas of this
// model are shallow by construction; the bound exists so a hostile or
// malformed document cannot turn derivation into unbounded work.
const maxRelationDepth = 32

type relationWalker struct {
	out []Relation
}

func (w *relationWalker) walk(node map[string]any, pointer, binding string, required bool) error {
	return w.walkAt(node, pointer, binding, required, nil, 0)
}

func (w *relationWalker) walkAt(
	node map[string]any,
	pointer, binding string,
	required bool,
	selectors []VariantSelector,
	depth int,
) error {
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
		targetRefs, requiredInterface, err := targetContract(node, pointer)
		if err != nil {
			return err
		}
		for _, ref := range targetRefs {
			if ref.APIVersion != group || ref.Kind != kind {
				return fmt.Errorf(
					"reference %s pins %s %s but lists exact target Form %s",
					pointer, group, kind, ref.String(),
				)
			}
		}
		exclusive, err := exclusiveHold(node, pointer)
		if err != nil {
			return err
		}
		w.out = append(w.out, Relation{
			Pointer:           pointer,
			TargetAPIVersion:  group,
			TargetKind:        kind,
			Binding:           binding,
			Required:          required,
			VariantSelectors:  append([]VariantSelector(nil), selectors...),
			TargetFormRefs:    targetRefs,
			RequiredInterface: requiredInterface,
			Exclusive:         exclusive,
		})
		return nil
	}
	if discriminator, _ := node[TaggedObjectDiscriminatorAnnotationKey].(string); discriminator != "" {
		return w.walkTaggedObject(node, pointer, binding, required, selectors, depth)
	}
	if items, ok := node["items"].(map[string]any); ok {
		// Every element of an array shares one pointer with "*" in place of the
		// index: the relation is a property of the schema, and the concrete
		// index only exists once a spec is materialized. An element is always
		// "present" when the array itself is, so requiredness passes through.
		if err := w.walkAt(items, pointer+"/*", relationBinding(node, binding), required, selectors, depth+1); err != nil {
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
			selectors,
			depth+1,
		); err != nil {
			return err
		}
	}
	return nil
}

func (w *relationWalker) walkTaggedObject(
	node map[string]any,
	pointer, binding string,
	required bool,
	selectors []VariantSelector,
	depth int,
) error {
	discriminator, _ := node[TaggedObjectDiscriminatorAnnotationKey].(string)
	branches, ok := node["oneOf"].([]any)
	if !ok || len(branches) < 2 {
		return fmt.Errorf("tagged object at %q carries fewer than two oneOf branches", pointer)
	}
	seen := map[string]struct{}{}
	for _, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("tagged object at %q carries a non-object branch", pointer)
		}
		if branch["type"] != "object" || branch["additionalProperties"] != false {
			return fmt.Errorf("tagged object at %q carries a branch that is not a closed object", pointer)
		}
		properties, _ := branch["properties"].(map[string]any)
		tag, ok := constantString(properties[discriminator])
		if !ok {
			return fmt.Errorf("tagged object at %q carries a branch without const discriminator %s", pointer, discriminator)
		}
		requiredMembers := anyStrings(branch["required"])
		if !containsString(requiredMembers, discriminator) {
			return fmt.Errorf("tagged object at %q carries branch %s with an optional discriminator", pointer, tag)
		}
		if _, duplicate := seen[tag]; duplicate {
			return fmt.Errorf("tagged object at %q repeats discriminator value %q", pointer, tag)
		}
		seen[tag] = struct{}{}
		branchSelectors := append([]VariantSelector(nil), selectors...)
		branchSelectors = append(branchSelectors, VariantSelector{
			Pointer: pointer + "/" + escapeJSONPointerToken(discriminator),
			Value:   tag,
		})
		if err := w.walkAt(branch, pointer, binding, required, branchSelectors, depth+1); err != nil {
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

// exclusiveHold reads the declared cardinality of a reference. A malformed
// annotation is refused rather than ignored: this is a rule a host enforces
// before it mutates anything, and an annotation nobody could read would make
// the Form silently unconstrained.
func exclusiveHold(node map[string]any, pointer string) (*ExclusiveHold, error) {
	raw, present := node[ExclusiveAnnotationKey]
	if !present {
		return nil, nil
	}
	declared, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("reference %s carries a malformed %s", pointer, ExclusiveAnnotationKey)
	}
	hold := &ExclusiveHold{}
	for member, value := range declared {
		if member != "keyedBy" {
			return nil, fmt.Errorf(
				"reference %s carries %s member %q, which the annotation does not define",
				pointer, ExclusiveAnnotationKey, member,
			)
		}
		key, ok := value.(string)
		if !ok || !strings.HasPrefix(key, "/") {
			return nil, fmt.Errorf(
				"reference %s carries a %s keyedBy that is not a JSON Pointer", pointer, ExclusiveAnnotationKey,
			)
		}
		hold.KeyedBy = key
	}
	return hold, nil
}

// targetContract reads the one target-contract annotation a reference-shaped
// node carries. A node carrying neither or both is refused: the annotation is
// what a host verifies before it mutates anything, so a relation with no
// stated requirement — or with two — has no answer to give (decision 0022).
func targetContract(node map[string]any, pointer string) ([]TargetFormRef, *RequiredInterface, error) {
	rawRefs, hasRefs := node[TargetFormRefsAnnotationKey]
	rawInterface, hasInterface := node[RequiredInterfaceAnnotationKey]
	switch {
	case hasRefs && hasInterface:
		return nil, nil, fmt.Errorf(
			"reference %s carries both %s and %s; a relation depends on the target's exact Form or on an Interface, never both",
			pointer, TargetFormRefsAnnotationKey, RequiredInterfaceAnnotationKey,
		)
	case hasRefs:
		items, ok := rawRefs.([]any)
		if !ok || len(items) == 0 {
			return nil, nil, fmt.Errorf("reference %s carries an empty or malformed %s", pointer, TargetFormRefsAnnotationKey)
		}
		out := make([]TargetFormRef, 0, len(items))
		for _, item := range items {
			member, _ := item.(map[string]any)
			ref := TargetFormRef{
				APIVersion:        annotationString(member, "apiVersion"),
				Kind:              annotationString(member, "kind"),
				DefinitionVersion: annotationString(member, "definitionVersion"),
				SchemaDigest:      annotationString(member, "schemaDigest"),
			}
			if ref.APIVersion == "" || ref.Kind == "" || ref.DefinitionVersion == "" || ref.SchemaDigest == "" {
				return nil, nil, fmt.Errorf(
					"reference %s lists an incomplete target Form identity; all four members are required",
					pointer,
				)
			}
			out = append(out, ref)
		}
		return out, nil, nil
	case hasInterface:
		member, ok := rawInterface.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("reference %s carries a malformed %s", pointer, RequiredInterfaceAnnotationKey)
		}
		required := RequiredInterface{
			APIVersion:   annotationString(member, "apiVersion"),
			Name:         annotationString(member, "name"),
			Version:      annotationString(member, "version"),
			SchemaDigest: annotationString(member, "schemaDigest"),
		}
		if required.APIVersion == "" || required.Name == "" ||
			required.Version == "" || required.SchemaDigest == "" {
			return nil, nil, fmt.Errorf(
				"reference %s names an incomplete required Interface; all four members are required",
				pointer,
			)
		}
		return nil, &required, nil
	default:
		return nil, nil, fmt.Errorf(
			"reference %s states no target contract; declare %s or %s so a host can verify what the "+
				"relation requires before it mutates anything (decision 0022)",
			pointer, TargetFormRefsAnnotationKey, RequiredInterfaceAnnotationKey,
		)
	}
}

// annotationString reads one string member of an annotation object, whether it
// travelled through JSON decoding or was built in Go.
func annotationString(node map[string]any, member string) string {
	value, _ := node[member].(string)
	return value
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
	// TargetFormRefs and RequiredInterface are the one target contract the
	// declaring reference states, carried through to the host so verification
	// happens against the instance it is about (decision 0022).
	TargetFormRefs    []TargetFormRef
	RequiredInterface *RequiredInterface
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
	return descend(relation, any(spec), any(spec), tokens, "", nil)
}

func descend(
	relation Relation,
	root, value any,
	tokens []string,
	pointer string,
	wildcards []string,
) []RelationInstance {
	if len(tokens) == 0 {
		if !variantSelected(relation.VariantSelectors, root, wildcards) {
			return nil
		}
		reference, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		name, _ := reference["name"].(string)
		if name == "" {
			return nil
		}
		return []RelationInstance{{
			Pointer:           pointer,
			Relation:          relation.Pointer,
			TargetAPIVersion:  relation.TargetAPIVersion,
			TargetKind:        relation.TargetKind,
			TargetName:        name,
			Binding:           relation.Binding,
			TargetFormRefs:    relation.TargetFormRefs,
			RequiredInterface: relation.RequiredInterface,
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
			indexText := strconv.Itoa(index)
			out = append(out, descend(
				relation, root, item, rest, pointer+"/"+indexText,
				append(wildcards, indexText),
			)...)
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
	return descend(relation, root, child, rest, pointer+"/"+token, wildcards)
}

func variantSelected(selectors []VariantSelector, root any, wildcards []string) bool {
	for _, selector := range selectors {
		pointer, ok := resolveWildcardPointer(selector.Pointer, wildcards)
		if !ok {
			return false
		}
		value, ok := valueAtJSONPointer(root, pointer)
		if !ok || value != selector.Value {
			return false
		}
	}
	return true
}

func resolveWildcardPointer(pointer string, wildcards []string) (string, bool) {
	tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	used := 0
	for index, token := range tokens {
		if token != "*" {
			continue
		}
		if used >= len(wildcards) {
			return "", false
		}
		tokens[index] = wildcards[used]
		used++
	}
	if len(tokens) == 1 && tokens[0] == "" {
		return "", true
	}
	return "/" + strings.Join(tokens, "/"), true
}

func valueAtJSONPointer(root any, pointer string) (any, bool) {
	if pointer == "" {
		return root, true
	}
	value := root
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		switch current := value.(type) {
		case map[string]any:
			var present bool
			value, present = current[unescapeJSONPointerToken(token)]
			if !present {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func unescapeJSONPointerToken(token string) string {
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(token)
}
