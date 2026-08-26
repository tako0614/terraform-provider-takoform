package portableconformancev3

import (
	"math/big"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// genericValidateMemoryConstraints is the data-only constraint evaluator for
// the neutral memory adapter.  It deliberately has no write side effects:
// callers perform the serializable check-and-write (or reservation) around
// this read-only operation.  candidateKey identifies the resource that is
// about to be written; replacing is its current incarnation when this is an
// update.  Both are excluded from holder scans so an update can retain its
// own claims while changing their values.
func genericValidateMemoryConstraints(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	desired map[string]any,
	replacing *genericMemoryResource,
) string {
	return genericValidateMemoryConstraintsMode(adapter, auth, candidateKey, input, desired, replacing, true)
}

// genericValidateMemoryAdmissionConstraints is the validate/prepare phase of
// the neutral adapter.  An exclusive hold is a commit-time occupancy rule:
// ReferenceHost reserves no holder during validate or prepare and checks it
// only in the atomic apply path.  All shape and resolved-UID constraints still
// run here, so an admission response cannot defer a malformed relation or
// graph-cycle to commit.
func genericValidateMemoryAdmissionConstraints(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	desired map[string]any,
	replacing *genericMemoryResource,
) string {
	return genericValidateMemoryConstraintsMode(adapter, auth, candidateKey, input, desired, replacing, false)
}

func genericValidateMemoryConstraintsMode(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	desired map[string]any,
	replacing *genericMemoryResource,
	includeExclusive bool,
) string {
	if adapter == nil {
		return "invalid_argument"
	}
	form, ok := adapter.forms[stableFormRefKey(input.Ref)]
	if !ok {
		return "form_unknown"
	}
	for _, constraint := range form.definition.Constraints {
		if !genericMemoryKnownConstraintKind(constraint.Kind) {
			return "unsupported_capability"
		}
	}
	relations, code := genericMemoryDefinitionRelations(form.definition)
	if code != "ok" {
		return code
	}
	if genericMemoryHasAcyclicSelfEdge(form.definition.Constraints, input, desired) {
		return "invalid_argument"
	}
	resolved, code := genericMemoryResolveRelations(adapter, auth.tenant, input.Space, relations, desired, candidateKey, replacing)
	if code != "ok" {
		return code
	}

	for _, constraint := range form.definition.Constraints {
		switch constraint.Kind {
		case "exclusive":
			if !includeExclusive {
				continue
			}
			if code := genericMemoryValidateExclusive(adapter, auth, candidateKey, input, desired, replacing, constraint, relations, resolved); code != "ok" {
				return code
			}
		case "sum":
			if code := genericMemoryValidateSum(desired, constraint); code != "ok" {
				return code
			}
		case "claim":
			if code := genericMemoryValidateClaim(adapter, auth, candidateKey, input, desired, replacing, constraint); code != "ok" {
				return code
			}
		case "hostAssigned":
			if code := genericMemoryValidateHostAssigned(desired, constraint); code != "ok" {
				return code
			}
		case "orderedPair":
			if code := genericMemoryValidateOrderedPair(desired, constraint); code != "ok" {
				return code
			}
		case "uniqueBy":
			if code := genericMemoryValidateUniqueBy(desired, constraint); code != "ok" {
				return code
			}
		case "acyclic":
			if code := genericMemoryValidateAcyclic(adapter, auth, candidateKey, input, replacing, constraint, relations, resolved); code != "ok" {
				return code
			}
		case "distinctPair":
			if code := genericMemoryValidateDistinctPair(constraint, resolved); code != "ok" {
				return code
			}
		case "uniquePair":
			if code := genericMemoryValidateUniquePair(adapter, auth, candidateKey, input, replacing, constraint, relations, resolved); code != "ok" {
				return code
			}
		case "sameResolvedTarget":
			if code := genericMemoryValidateSameResolvedTarget(adapter, auth, candidateKey, input, replacing, constraint, relations, resolved); code != "ok" {
				return code
			}
		}
	}
	return "ok"
}

func genericMemoryHasAcyclicSelfEdge(constraints []formpackage.FormConstraint, input genericResourceInput, desired map[string]any) bool {
	for _, constraint := range constraints {
		if constraint.Kind != "acyclic" {
			continue
		}
		value, present := genericMemoryPointerValue(desired, constraint.Reference)
		target, object := value.(map[string]any)
		if !present || !object {
			continue
		}
		apiVersion, _ := target["apiVersion"].(string)
		kind, _ := target["kind"].(string)
		name, _ := target["name"].(string)
		if apiVersion == input.Ref.APIVersion && kind == input.Ref.Kind && name == input.Name {
			return true
		}
	}
	return false
}

func genericMemoryKnownConstraintKind(kind string) bool {
	switch kind {
	case "exclusive", "sum", "claim", "hostAssigned", "orderedPair", "uniqueBy", "acyclic", "distinctPair", "uniquePair", "sameResolvedTarget":
		return true
	default:
		return false
	}
}

func genericMemoryDefinitionRelations(definition formpackage.FormDefinition) ([]currentformmodel.Relation, string) {
	constraints := make([]currentformmodel.Constraint, 0, len(definition.Constraints))
	for _, entry := range definition.Constraints {
		constraints = append(constraints, currentformmodel.Constraint{
			Kind:       currentformmodel.ConstraintKind(entry.Kind),
			Reference:  entry.Reference,
			KeyedBy:    entry.KeyedBy,
			List:       entry.List,
			Member:     entry.Member,
			Total:      entry.Total,
			Property:   entry.Property,
			Output:     entry.Output,
			References: append([]string(nil), entry.References...),
			Anchor:     entry.Anchor,
			Members:    entry.Members,
			Through:    entry.Through,
		})
	}
	relations, err := currentformmodel.DeriveRelationsWithConstraints(definition.DesiredSchema, constraints)
	if err != nil {
		return nil, "invalid_argument"
	}
	return relations, "ok"
}

// genericMemoryResolvedRelation carries one concrete relation instance and
// the currently live target it resolved to.  UID and exact FormRef are read
// from the target resource on every evaluation; a missing or replaced target
// therefore fails closed instead of silently rebinding by name.
type genericMemoryResolvedRelation struct {
	declared currentformmodel.Relation
	instance currentformmodel.RelationInstance
	target   *genericMemoryResource
}

// genericMemoryRelationPin is the immutable relation evidence stored with a
// live memory resource. Names are retained only to identify the pinned
// address; TargetUID and TargetRef are the authority used by later traversals.
type genericMemoryRelationPin struct {
	Pointer          string
	Relation         string
	TargetAPIVersion string
	TargetKind       string
	TargetName       string
	TargetUID        string
	TargetRef        FormRef
}

// genericMemoryCaptureRelationPins resolves a candidate once and returns the
// UID/exact-Form evidence that a successful write must retain. It is read-only
// and is intended to run inside the caller's atomic check-and-write boundary.
func genericMemoryCaptureRelationPins(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	desired map[string]any,
	replacing *genericMemoryResource,
) ([]genericMemoryRelationPin, string) {
	if adapter == nil {
		return nil, "invalid_argument"
	}
	form, ok := adapter.forms[stableFormRefKey(input.Ref)]
	if !ok {
		return nil, "form_unknown"
	}
	for _, constraint := range form.definition.Constraints {
		if !genericMemoryKnownConstraintKind(constraint.Kind) {
			return nil, "unsupported_capability"
		}
	}
	relations, code := genericMemoryDefinitionRelations(form.definition)
	if code != "ok" {
		return nil, code
	}
	resolved, code := genericMemoryResolveRelations(adapter, auth.tenant, input.Space, relations, desired, candidateKey, replacing)
	if code != "ok" {
		return nil, code
	}
	pins := make([]genericMemoryRelationPin, 0, len(resolved))
	for _, relation := range resolved {
		pins = append(pins, genericMemoryRelationPin{
			Pointer: relation.instance.Pointer, Relation: relation.instance.Relation,
			TargetAPIVersion: relation.instance.TargetAPIVersion,
			TargetKind:       relation.instance.TargetKind, TargetName: relation.instance.TargetName,
			TargetUID: relation.target.uid, TargetRef: relation.target.address.Ref,
		})
	}
	sort.Slice(pins, func(i, j int) bool {
		if pins[i].Pointer != pins[j].Pointer {
			return pins[i].Pointer < pins[j].Pointer
		}
		return pins[i].Relation < pins[j].Relation
	})
	return pins, "ok"
}

func genericMemoryResolveRelations(
	adapter *genericMemoryAdapter,
	tenant, space string,
	relations []currentformmodel.Relation,
	desired map[string]any,
	candidateKey string,
	replacing *genericMemoryResource,
) ([]genericMemoryResolvedRelation, string) {
	instances := currentformmodel.RelationInstances(relations, desired)
	out := make([]genericMemoryResolvedRelation, 0, len(instances))
	for _, instance := range instances {
		declared := genericMemoryDeclaredRelation(relations, instance.Relation)
		if declared == nil {
			return nil, "invalid_argument"
		}
		key := genericMemoryAddressKey(tenant, space, instance.TargetAPIVersion, instance.TargetKind, instance.TargetName)
		target := adapter.resources[key]
		if key == candidateKey && replacing != nil {
			target = replacing
		}
		if target == nil {
			return nil, "resource_not_found"
		}
		if target.address.Space != space || target.address.Ref.APIVersion != instance.TargetAPIVersion || target.address.Ref.Kind != instance.TargetKind {
			return nil, "invalid_argument"
		}
		if _, installed := adapter.forms[stableFormRefKey(target.address.Ref)]; !installed {
			return nil, "invalid_argument"
		}
		if len(instance.TargetFormRefs) == 0 && instance.RequiredInterface == nil {
			return nil, "invalid_argument"
		}
		if len(instance.TargetFormRefs) > 0 {
			accepted := false
			for _, ref := range instance.TargetFormRefs {
				if genericMemoryFormRefEqual(target.address.Ref, genericMemoryFormRefFromTarget(ref)) {
					accepted = true
					break
				}
			}
			if !accepted {
				return nil, "invalid_argument"
			}
		}
		if instance.RequiredInterface != nil {
			targetForm := adapter.forms[stableFormRefKey(target.address.Ref)]
			provided := false
			for _, candidate := range targetForm.definition.ProvidedInterfaces {
				if candidate.APIVersion == instance.RequiredInterface.APIVersion &&
					candidate.Name == instance.RequiredInterface.Name &&
					candidate.Version == instance.RequiredInterface.Version &&
					candidate.SchemaDigest == instance.RequiredInterface.SchemaDigest {
					provided = true
					break
				}
			}
			if !provided {
				return nil, "invalid_argument"
			}
		}
		out = append(out, genericMemoryResolvedRelation{declared: *declared, instance: instance, target: target})
	}
	return out, "ok"
}

// genericMemoryResolvePinnedRelations re-resolves a stored resource only to
// prove that every live target still has the UID and exact FormRef recorded at
// the resource's last successful write. A missing pin or any replacement
// drift is a closed invalid state, not permission to bind by name again.
func genericMemoryResolvePinnedRelations(
	adapter *genericMemoryAdapter,
	tenant string,
	resource *genericMemoryResource,
	relations []currentformmodel.Relation,
) ([]genericMemoryResolvedRelation, string) {
	if resource == nil {
		return nil, "invalid_argument"
	}
	resolved, code := genericMemoryResolveRelations(adapter, tenant, resource.address.Space, relations, resource.desired, "", nil)
	if code != "ok" {
		return nil, "invalid_argument"
	}
	pins := resource.relationPins
	if len(pins) != len(resolved) {
		return nil, "invalid_argument"
	}
	byPointer := make(map[string]genericMemoryRelationPin, len(pins))
	for _, pin := range pins {
		if _, duplicate := byPointer[pin.Pointer]; duplicate {
			return nil, "invalid_argument"
		}
		byPointer[pin.Pointer] = pin
	}
	for _, relation := range resolved {
		pin, ok := byPointer[relation.instance.Pointer]
		if !ok || pin.Relation != relation.instance.Relation ||
			pin.TargetAPIVersion != relation.instance.TargetAPIVersion || pin.TargetKind != relation.instance.TargetKind ||
			pin.TargetName != relation.instance.TargetName || pin.TargetUID != relation.target.uid || pin.TargetRef != relation.target.address.Ref {
			return nil, "invalid_argument"
		}
	}
	return resolved, "ok"
}

func genericMemoryDeclaredRelation(relations []currentformmodel.Relation, pointer string) *currentformmodel.Relation {
	var found *currentformmodel.Relation
	for index := range relations {
		if relations[index].Pointer != pointer {
			continue
		}
		if found != nil {
			return nil
		}
		found = &relations[index]
	}
	return found
}

func genericMemoryAddressKey(tenant, space, apiVersion, kind, name string) string {
	return tenant + "\x00" + space + "\x00" + apiVersion + "\x00" + kind + "\x00" + name
}

func genericMemoryFormRefEqual(left, right FormRef) bool { return left == right }

func genericMemoryFormRefFromTarget(ref currentformmodel.TargetFormRef) FormRef {
	return FormRef{
		APIVersion: ref.APIVersion, Kind: ref.Kind,
		DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
	}
}

func genericMemoryRelationsFor(resolved []genericMemoryResolvedRelation, pointer string) []genericMemoryResolvedRelation {
	out := make([]genericMemoryResolvedRelation, 0, 1)
	for _, relation := range resolved {
		if relation.instance.Relation == pointer {
			out = append(out, relation)
		}
	}
	return out
}

func genericMemoryOneRelation(resolved []genericMemoryResolvedRelation, kind, pointer string) (genericMemoryResolvedRelation, bool, string) {
	matches := genericMemoryRelationsFor(resolved, pointer)
	if len(matches) == 0 {
		return genericMemoryResolvedRelation{}, false, "ok"
	}
	if len(matches) != 1 || matches[0].target == nil || matches[0].target.uid == "" {
		return genericMemoryResolvedRelation{}, false, "invalid_argument"
	}
	return matches[0], true, "ok"
}

func genericMemoryPinnedUID(resource *genericMemoryResource, relation string) (string, bool, string) {
	if resource == nil {
		return "", false, "invalid_argument"
	}
	var found *genericMemoryRelationPin
	for index := range resource.relationPins {
		pin := &resource.relationPins[index]
		if pin.Relation != relation {
			continue
		}
		if found != nil {
			return "", false, "invalid_argument"
		}
		found = pin
	}
	if found == nil || found.TargetUID == "" {
		return "", false, "invalid_argument"
	}
	return found.TargetUID, true, "ok"
}

func genericMemoryPinnedPair(resource *genericMemoryResource, references []string) ([2]string, bool, string) {
	var pair [2]string
	if len(references) != 2 {
		return pair, false, "invalid_argument"
	}
	for index, pointer := range references {
		uid, present, code := genericMemoryPinnedUID(resource, pointer)
		if code != "ok" {
			return pair, false, code
		}
		if !present {
			return pair, false, "invalid_argument"
		}
		pair[index] = uid
	}
	return pair, true, "ok"
}

func genericMemoryValidateExclusive(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	desired map[string]any,
	replacing *genericMemoryResource,
	constraint formpackage.FormConstraint,
	relations []currentformmodel.Relation,
	resolved []genericMemoryResolvedRelation,
) string {
	declared := genericMemoryDeclaredRelation(relations, constraint.Reference)
	if declared == nil {
		return "invalid_argument"
	}
	target, present, code := genericMemoryOneRelation(resolved, "exclusive", constraint.Reference)
	if code != "ok" {
		return code
	}
	if !present {
		return "ok"
	}
	keyedValue, keyed, code := genericMemoryConstraintKey(desired, constraint.KeyedBy)
	if code != "ok" {
		return code
	}
	for holderKey, candidate := range adapter.resources {
		if genericMemorySkipCandidate(holderKey, candidate, candidateKey, replacing) || !strings.HasPrefix(holderKey, auth.tenant+"\x00") || candidate.address.Ref != input.Ref || candidate.address.Space != input.Space {
			continue
		}
		candidateForm, ok := adapter.forms[stableFormRefKey(candidate.address.Ref)]
		if !ok {
			return "invalid_argument"
		}
		_ = candidateForm
		candidateUID, candidatePresent, candidateCode := genericMemoryPinnedUID(candidate, constraint.Reference)
		if candidateCode != "ok" {
			return candidateCode
		}
		if !candidatePresent || candidateUID != target.target.uid {
			continue
		}
		candidateKeyValue, candidateKeyed, candidateCode := genericMemoryConstraintKey(candidate.desired, constraint.KeyedBy)
		if candidateCode != "ok" {
			return candidateCode
		}
		if keyed != candidateKeyed || (keyed && candidateKeyValue != keyedValue) {
			continue
		}
		return "invalid_argument"
	}
	return "ok"
}

func genericMemoryConstraintKey(value map[string]any, pointer string) (string, bool, string) {
	if pointer == "" {
		return "", true, "ok"
	}
	item, present := genericMemoryPointerValue(value, pointer)
	if !present {
		return "", false, "invalid_argument"
	}
	key, scalar := genericMemoryScalarKey(item)
	if !scalar {
		return "", false, "invalid_argument"
	}
	return key, true, "ok"
}

func genericMemoryValidateSum(desired map[string]any, constraint formpackage.FormConstraint) string {
	value, present := genericMemoryPointerValue(desired, constraint.List)
	if !present {
		if constraint.Total == 0 {
			return "ok"
		}
		return "invalid_argument"
	}
	items, ok := value.([]any)
	if !ok {
		return "invalid_argument"
	}
	total := new(big.Int)
	for _, raw := range items {
		entry, ok := raw.(map[string]any)
		if !ok {
			return "invalid_argument"
		}
		member, present := entry[constraint.Member]
		if !present {
			return "invalid_argument"
		}
		value, ok := genericMemoryInteger(member)
		if !ok {
			return "invalid_argument"
		}
		total.Add(total, value)
	}
	want := big.NewInt(constraint.Total)
	if total.Cmp(want) != 0 {
		return "invalid_argument"
	}
	return "ok"
}

func genericMemoryValidateClaim(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	desired map[string]any,
	replacing *genericMemoryResource,
	constraint formpackage.FormConstraint,
) string {
	value, present := genericMemoryPointerValue(desired, constraint.Property)
	if !present {
		return "invalid_argument"
	}
	claim, scalar := genericMemoryScalarKey(value)
	if !scalar {
		return "invalid_argument"
	}
	for holderKey, candidate := range adapter.resources {
		if genericMemorySkipCandidate(holderKey, candidate, candidateKey, replacing) || !strings.HasPrefix(holderKey, auth.tenant+"\x00") {
			continue
		}
		candidateForm, ok := adapter.forms[stableFormRefKey(candidate.address.Ref)]
		if !ok {
			return "invalid_argument"
		}
		for _, candidateConstraint := range candidateForm.definition.Constraints {
			if candidateConstraint.Kind != "claim" {
				continue
			}
			candidateValue, candidatePresent := genericMemoryPointerValue(candidate.desired, candidateConstraint.Property)
			if !candidatePresent {
				return "invalid_argument"
			}
			candidateClaim, candidateScalar := genericMemoryScalarKey(candidateValue)
			if !candidateScalar {
				return "invalid_argument"
			}
			if candidateClaim == claim {
				return "invalid_argument"
			}
		}
	}
	return "ok"
}

func genericMemoryValidateHostAssigned(desired map[string]any, constraint formpackage.FormConstraint) string {
	if _, present := genericMemoryPointerValue(desired, constraint.Output); present {
		return "invalid_argument"
	}
	return "ok"
}

func genericMemoryValidateOrderedPair(desired map[string]any, constraint formpackage.FormConstraint) string {
	if len(constraint.References) != 2 {
		return "invalid_argument"
	}
	leftValue, leftPresent := genericMemoryPointerValue(desired, constraint.References[0])
	rightValue, rightPresent := genericMemoryPointerValue(desired, constraint.References[1])
	left, leftNumeric := genericMemoryCanonicalNumber(leftValue)
	right, rightNumeric := genericMemoryCanonicalNumber(rightValue)
	if !leftPresent || !rightPresent || !leftNumeric || !rightNumeric || left.Cmp(right) > 0 {
		return "invalid_argument"
	}
	return "ok"
}

func genericMemoryValidateUniqueBy(desired map[string]any, constraint formpackage.FormConstraint) string {
	value, present := genericMemoryPointerValue(desired, constraint.List)
	if !present {
		return "ok"
	}
	items, ok := value.([]any)
	if !ok {
		return "invalid_argument"
	}
	seen := map[string]struct{}{}
	for _, raw := range items {
		entry, object := raw.(map[string]any)
		member, memberPresent := entry[constraint.Member]
		key, scalar := genericMemoryScalarKey(member)
		if !object || !memberPresent || !scalar {
			return "invalid_argument"
		}
		if _, duplicate := seen[key]; duplicate {
			return "invalid_argument"
		}
		seen[key] = struct{}{}
	}
	return "ok"
}

func genericMemoryValidateAcyclic(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	replacing *genericMemoryResource,
	constraint formpackage.FormConstraint,
	relations []currentformmodel.Relation,
	resolved []genericMemoryResolvedRelation,
) string {
	edge, present, code := genericMemoryOneRelation(resolved, "acyclic", constraint.Reference)
	if code != "ok" || !present {
		return code
	}
	if edge.instance.TargetAPIVersion == input.Ref.APIVersion && edge.instance.TargetKind == input.Ref.Kind && edge.instance.TargetName == input.Name {
		return "invalid_argument"
	}
	seen := map[string]struct{}{}
	if replacing != nil {
		seen[replacing.uid] = struct{}{}
	}
	form := adapter.forms[stableFormRefKey(input.Ref)]
	for step := 0; ; step++ {
		if step >= genericMemoryResolvedTraversalLimit {
			return "invalid_argument"
		}
		if _, cycle := seen[edge.target.uid]; cycle {
			return "invalid_argument"
		}
		seen[edge.target.uid] = struct{}{}
		if edge.target.address.Ref != input.Ref {
			return "ok"
		}
		nextRelations, nextCode := genericMemoryDefinitionRelations(form.definition)
		if nextCode != "ok" {
			return nextCode
		}
		nextResolved, nextCode := genericMemoryResolvePinnedRelations(adapter, auth.tenant, edge.target, nextRelations)
		if nextCode != "ok" {
			return "invalid_argument"
		}
		next, nextPresent, nextCode := genericMemoryOneRelation(nextResolved, "acyclic", constraint.Reference)
		if nextCode != "ok" {
			return nextCode
		}
		if !nextPresent {
			return "ok"
		}
		edge = next
	}
}

func genericMemoryValidateDistinctPair(constraint formpackage.FormConstraint, resolved []genericMemoryResolvedRelation) string {
	if len(constraint.References) != 2 {
		return "invalid_argument"
	}
	left, leftPresent, code := genericMemoryOneRelation(resolved, "distinctPair", constraint.References[0])
	if code != "ok" {
		return code
	}
	right, rightPresent, code := genericMemoryOneRelation(resolved, "distinctPair", constraint.References[1])
	if code != "ok" {
		return code
	}
	if !leftPresent || !rightPresent {
		return "ok"
	}
	if left.target.uid == right.target.uid {
		return "invalid_argument"
	}
	return "ok"
}

func genericMemoryValidateUniquePair(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	replacing *genericMemoryResource,
	constraint formpackage.FormConstraint,
	relations []currentformmodel.Relation,
	resolved []genericMemoryResolvedRelation,
) string {
	if len(constraint.References) != 2 {
		return "invalid_argument"
	}
	pair, present, code := genericMemoryRequiredPair(constraint, resolved)
	if code != "ok" || !present {
		return code
	}
	for holderKey, candidate := range adapter.resources {
		if genericMemorySkipCandidate(holderKey, candidate, candidateKey, replacing) || !strings.HasPrefix(holderKey, auth.tenant+"\x00") || candidate.address.Ref != input.Ref {
			continue
		}
		candidateForm, ok := adapter.forms[stableFormRefKey(candidate.address.Ref)]
		if !ok {
			return "invalid_argument"
		}
		_ = candidateForm
		candidatePair, candidatePresent, candidateCode := genericMemoryPinnedPair(candidate, constraint.References)
		if candidateCode != "ok" {
			return candidateCode
		}
		if candidatePresent && candidatePair == pair {
			return "invalid_argument"
		}
	}
	return "ok"
}

func genericMemoryRequiredPair(constraint formpackage.FormConstraint, resolved []genericMemoryResolvedRelation) ([2]string, bool, string) {
	var pair [2]string
	for index, pointer := range constraint.References {
		relation, present, code := genericMemoryOneRelation(resolved, constraint.Kind, pointer)
		if code != "ok" {
			return pair, false, code
		}
		if !present {
			return pair, false, "invalid_argument"
		}
		pair[index] = relation.target.uid
	}
	return pair, true, "ok"
}

func genericMemoryValidateSameResolvedTarget(
	adapter *genericMemoryAdapter,
	auth genericMemoryActor,
	candidateKey string,
	input genericResourceInput,
	replacing *genericMemoryResource,
	constraint formpackage.FormConstraint,
	relations []currentformmodel.Relation,
	resolved []genericMemoryResolvedRelation,
) string {
	anchor, present, code := genericMemoryOneRelation(resolved, "sameResolvedTarget anchor", constraint.Anchor)
	if code != "ok" {
		return code
	}
	if !present {
		return "invalid_argument"
	}
	if _, code := genericMemoryPinnedTarget(adapter, auth.tenant, anchor.target); code != "ok" {
		return code
	}
	memberDeclaration := genericMemoryDeclaredRelation(relations, constraint.Members)
	if memberDeclaration == nil || memberDeclaration.RequiredInterface != nil || len(memberDeclaration.TargetFormRefs) == 0 {
		return "invalid_argument"
	}
	members := genericMemoryRelationsFor(resolved, constraint.Members)
	for _, member := range members {
		if _, code := genericMemoryPinnedTarget(adapter, auth.tenant, member.target); code != "ok" {
			return code
		}
		memberForm, ok := adapter.forms[stableFormRefKey(member.target.address.Ref)]
		if !ok {
			return "invalid_argument"
		}
		memberRelations, code := genericMemoryDefinitionRelations(memberForm.definition)
		if code != "ok" {
			return code
		}
		declaredThrough := make([]currentformmodel.Relation, 0, 1)
		for _, relation := range memberRelations {
			if relation.Pointer == constraint.Through {
				declaredThrough = append(declaredThrough, relation)
			}
		}
		if len(declaredThrough) != 1 {
			return "invalid_argument"
		}
		throughResolved, code := genericMemoryResolvePinnedRelations(adapter, auth.tenant, member.target, memberRelations)
		if code != "ok" {
			return "invalid_argument"
		}
		through, throughPresent, code := genericMemoryOneRelation(throughResolved, "sameResolvedTarget through", constraint.Through)
		if code != "ok" {
			return code
		}
		if !throughPresent || through.target.uid == "" {
			return "invalid_argument"
		}
		if _, code := genericMemoryPinnedTarget(adapter, auth.tenant, through.target); code != "ok" {
			return code
		}
		if through.instance.TargetAPIVersion != anchor.instance.TargetAPIVersion || through.instance.TargetKind != anchor.instance.TargetKind {
			return "invalid_argument"
		}
		if through.target.uid != anchor.target.uid {
			return "invalid_argument"
		}
	}
	return "ok"
}

func genericMemoryPinnedTarget(adapter *genericMemoryAdapter, tenant string, target *genericMemoryResource) (*genericMemoryResource, string) {
	if target == nil || target.uid == "" {
		return nil, "invalid_argument"
	}
	key := genericMemoryResourceKeyForStored(tenant, target)
	current := adapter.resources[key]
	if current == nil || current.uid != target.uid || current.address.Ref != target.address.Ref {
		return nil, "invalid_argument"
	}
	return current, "ok"
}

func genericMemoryResourceKeyForStored(tenant string, resource *genericMemoryResource) string {
	if resource == nil {
		return ""
	}
	return genericMemoryAddressKey(tenant, resource.address.Space, resource.address.Ref.APIVersion, resource.address.Ref.Kind, resource.address.Name)
}

func genericMemorySkipCandidate(key string, candidate *genericMemoryResource, candidateKey string, replacing *genericMemoryResource) bool {
	return key == candidateKey || candidate == replacing
}

func genericMemoryInteger(value any) (*big.Int, bool) {
	rat, ok := genericMemoryCanonicalNumber(value)
	if !ok || rat.Denom().Cmp(big.NewInt(1)) != 0 {
		return nil, false
	}
	return new(big.Int).Set(rat.Num()), true
}
