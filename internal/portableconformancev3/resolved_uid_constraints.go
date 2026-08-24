package portableconformancev3

import (
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// maxResolvedUIDConstraintTraversal is the hard ceiling for one declared graph
// walk. A constraint that needs an unbounded store scan is not portable host
// input: the host must fail closed instead of timing out after partially
// deciding a mutation.
const maxResolvedUIDConstraintTraversal = 256

func isResolvedUIDConstraintKind(kind string) bool {
	switch kind {
	case "acyclic", "distinctPair", "uniquePair", "sameResolvedTarget":
		return true
	default:
		return false
	}
}

func (form *InstalledForm) declaresResolvedUIDConstraints() bool {
	for _, constraint := range form.Constraints {
		if isResolvedUIDConstraintKind(constraint.Kind) {
			return true
		}
	}
	return false
}

// validateResolvedUIDConstraintRequest is the prepare/validate phase of the
// four UID constraints. It deliberately resolves only when the exact Form
// declares one of them, so adding these mechanisms does not make an unrelated
// Form's diagnostics depend on live resources.
func (h *ReferenceHost) validateResolvedUIDConstraintRequest(
	form *InstalledForm,
	scope resourceScope,
	name string,
	spec map[string]any,
) *hostError {
	if !form.declaresResolvedUIDConstraints() {
		return nil
	}
	// A create has no source UID until commit, but a reference to its own exact
	// resource address is already necessarily a self-edge. Reject it before
	// ordinary relation resolution reports the not-yet-created target as merely
	// missing; on an update the same check is equivalent to comparing the live
	// source and target UID and avoids a special case later in the walk.
	for _, constraint := range form.Constraints {
		if constraint.Kind != "acyclic" {
			continue
		}
		value, present := desiredValueAtPointer(spec, constraint.Reference)
		target, object := value.(map[string]any)
		if !present || !object {
			continue
		}
		group, _ := target["apiVersion"].(string)
		kind, _ := target["kind"].(string)
		targetName, _ := target["name"].(string)
		if group == form.Ref.APIVersion && kind == form.Ref.Kind && targetName == name {
			return stableError("invalid_argument", "acyclic relation "+constraint.Reference+" is a self-edge")
		}
	}
	relations, hostErr := h.resolveRelations(form, scope, spec)
	if hostErr != nil {
		return hostErr
	}
	return h.validateResolvedUIDConstraints(form, scope, name, relations)
}

// validateResolvedUIDConstraints enforces the four closed constraint kinds
// against already-resolved relation records. It runs in the ReferenceHost's
// request mutex together with storeResource, making the uniquePair check and
// insertion one atomic critical section. A production host owes the equivalent
// serializable check-and-write or an atomic reservation keyed by tenant, exact
// FormRef, and the ordered UID pair.
func (h *ReferenceHost) validateResolvedUIDConstraints(
	form *InstalledForm,
	scope resourceScope,
	name string,
	relations []storedRelation,
) *hostError {
	for _, constraint := range form.Constraints {
		switch constraint.Kind {
		case "acyclic":
			if hostErr := h.validateAcyclicConstraint(form, scope, name, constraint, relations); hostErr != nil {
				return hostErr
			}
		case "distinctPair":
			if hostErr := validateDistinctPairConstraint(constraint, relations); hostErr != nil {
				return hostErr
			}
		case "uniquePair":
			if hostErr := h.validateUniquePairConstraint(form, scope, name, constraint, relations); hostErr != nil {
				return hostErr
			}
		case "sameResolvedTarget":
			if hostErr := h.validateSameResolvedTargetConstraint(form, scope, constraint, relations); hostErr != nil {
				return hostErr
			}
		}
	}
	return nil
}

func relationsForDeclaration(relations []storedRelation, pointer string) []storedRelation {
	out := make([]storedRelation, 0, 1)
	for _, relation := range relations {
		if relation.Relation == pointer {
			out = append(out, relation)
		}
	}
	return out
}

func oneConstraintOperand(kind, pointer string, relations []storedRelation) (storedRelation, bool, *hostError) {
	matches := relationsForDeclaration(relations, pointer)
	if len(matches) == 0 {
		return storedRelation{}, false, nil
	}
	if len(matches) != 1 || matches[0].TargetUID == "" {
		return storedRelation{}, false, stableError(
			"invalid_argument",
			fmt.Sprintf("%s relation %s resolved to %d concrete UID operands; require exactly one", kind, pointer, len(matches)),
		)
	}
	return matches[0], true, nil
}

func validateDistinctPairConstraint(
	constraint formpackage.FormConstraint,
	relations []storedRelation,
) *hostError {
	if len(constraint.References) != 2 {
		return stableError("invalid_argument", "the installed distinctPair declaration does not carry exactly two relations")
	}
	left, leftPresent, hostErr := oneConstraintOperand("distinctPair", constraint.References[0], relations)
	if hostErr != nil {
		return hostErr
	}
	right, rightPresent, hostErr := oneConstraintOperand("distinctPair", constraint.References[1], relations)
	if hostErr != nil {
		return hostErr
	}
	// distinctPair is the one optional-pair rule: if either operand is absent,
	// there is no pair to compare. Presence of both turns it on.
	if !leftPresent || !rightPresent {
		return nil
	}
	if left.TargetUID == right.TargetUID {
		return stableError(
			"invalid_argument",
			"distinctPair relations "+constraint.References[0]+" and "+constraint.References[1]+
				" resolve to the same immutable UID "+left.TargetUID,
		)
	}
	return nil
}

func requiredUIDPair(
	kind string,
	constraint formpackage.FormConstraint,
	relations []storedRelation,
) ([2]string, *hostError) {
	if len(constraint.References) != 2 {
		return [2]string{}, stableError("invalid_argument", "the installed "+kind+" declaration does not carry exactly two relations")
	}
	var pair [2]string
	for index, pointer := range constraint.References {
		relation, present, hostErr := oneConstraintOperand(kind, pointer, relations)
		if hostErr != nil {
			return [2]string{}, hostErr
		}
		if !present {
			return [2]string{}, stableError(
				"invalid_argument",
				kind+" relation "+pointer+" has no concrete resolved UID operand",
			)
		}
		pair[index] = relation.TargetUID
	}
	return pair, nil
}

func (h *ReferenceHost) validateUniquePairConstraint(
	form *InstalledForm,
	scope resourceScope,
	name string,
	constraint formpackage.FormConstraint,
	relations []storedRelation,
) *hostError {
	pair, hostErr := requiredUIDPair("uniquePair", constraint, relations)
	if hostErr != nil {
		return hostErr
	}
	selfKey := resourceKey(scope, form.Ref.APIVersion, form.Ref.Kind, name)
	for _, candidate := range h.sortedResources() {
		if candidate.Tenant != scope.Tenant || candidate.Ref != form.Ref || candidate.key() == selfKey {
			continue
		}
		candidatePair, candidateErr := requiredUIDPair("uniquePair", constraint, candidate.Relations)
		if candidateErr != nil {
			return stableError(
				"invalid_argument",
				"live resource "+candidate.Name+" of exact Form "+exactFormKey(form.Ref).String()+
					" has an unreadable uniquePair hold: "+candidateErr.Message,
			)
		}
		if candidatePair == pair {
			return stableError(
				"invalid_argument",
				"uniquePair ordered UID pair ["+pair[0]+", "+pair[1]+"] is already held by live resource "+candidate.Name+
					" of exact Form "+exactFormKey(form.Ref).String(),
			)
		}
	}
	return nil
}

func (h *ReferenceHost) livePinnedConstraintTarget(
	scope resourceScope,
	kind string,
	relation storedRelation,
) (*storedResource, *hostError) {
	current := h.resources[resourceKey(
		scope, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName,
	)]
	if current == nil || current.UID != relation.TargetUID || current.Ref != relation.TargetRef {
		currentUID := "absent"
		if current != nil {
			currentUID = current.UID
		}
		return nil, stableError(
			"invalid_argument",
			kind+" traversal found replacement drift at relation "+relation.Pointer+
				": pinned UID "+relation.TargetUID+", current UID "+currentUID,
		)
	}
	return current, nil
}

func (h *ReferenceHost) validateAcyclicConstraint(
	form *InstalledForm,
	scope resourceScope,
	name string,
	constraint formpackage.FormConstraint,
	relations []storedRelation,
) *hostError {
	edge, present, hostErr := oneConstraintOperand("acyclic", constraint.Reference, relations)
	if hostErr != nil || !present {
		return hostErr
	}
	seen := map[string]bool{}
	if source := h.resourceUnderExactRef(scope, form.Ref, name); source != nil {
		seen[source.UID] = true
	}
	for step := 0; ; step++ {
		if step >= maxResolvedUIDConstraintTraversal {
			return stableError(
				"invalid_argument",
				fmt.Sprintf("acyclic relation %s exceeds the bounded %d-resource traversal", constraint.Reference, maxResolvedUIDConstraintTraversal),
			)
		}
		if seen[edge.TargetUID] {
			return stableError(
				"invalid_argument",
				"acyclic relation "+constraint.Reference+" closes a cycle at immutable UID "+edge.TargetUID,
			)
		}
		seen[edge.TargetUID] = true
		target, targetErr := h.livePinnedConstraintTarget(scope, "acyclic", edge)
		if targetErr != nil {
			return targetErr
		}
		// The graph belongs to one exact FormRef. A resource under another exact
		// contract is a boundary node, not an instance of this declaration.
		if target.Ref != form.Ref {
			return nil
		}
		next, nextPresent, nextErr := oneConstraintOperand("acyclic", constraint.Reference, target.Relations)
		if nextErr != nil || !nextPresent {
			return nextErr
		}
		edge = next
	}
}

func (h *ReferenceHost) validateSameResolvedTargetConstraint(
	_ *InstalledForm,
	scope resourceScope,
	constraint formpackage.FormConstraint,
	relations []storedRelation,
) *hostError {
	anchor, present, hostErr := oneConstraintOperand("sameResolvedTarget anchor", constraint.Anchor, relations)
	if hostErr != nil {
		return hostErr
	}
	if !present {
		return stableError("invalid_argument", "sameResolvedTarget anchor "+constraint.Anchor+" has no concrete resolved UID")
	}
	if _, hostErr := h.livePinnedConstraintTarget(scope, "sameResolvedTarget anchor", anchor); hostErr != nil {
		return hostErr
	}
	members := relationsForDeclaration(relations, constraint.Members)
	for _, memberRelation := range members {
		member, memberErr := h.livePinnedConstraintTarget(scope, "sameResolvedTarget member", memberRelation)
		if memberErr != nil {
			return memberErr
		}
		memberForm := h.catalog.exact(member.Ref)
		if memberForm == nil {
			return stableError("invalid_argument", "sameResolvedTarget member exact Form is not installed")
		}
		declared := 0
		for _, relation := range memberForm.Relations {
			if relation.Pointer == constraint.Through {
				declared++
			}
		}
		if declared != 1 {
			return stableError(
				"invalid_argument",
				fmt.Sprintf("sameResolvedTarget through %s resolves to %d declared relations on member exact Form %s; require exactly one", constraint.Through, declared, exactFormKey(member.Ref).String()),
			)
		}
		through, throughPresent, throughErr := oneConstraintOperand("sameResolvedTarget through", constraint.Through, member.Relations)
		if throughErr != nil {
			return throughErr
		}
		if !throughPresent {
			return stableError(
				"invalid_argument",
				"sameResolvedTarget member "+member.Name+" has no concrete UID at through relation "+constraint.Through,
			)
		}
		if _, throughErr := h.livePinnedConstraintTarget(member.scope(), "sameResolvedTarget through", through); throughErr != nil {
			return throughErr
		}
		if through.TargetUID != anchor.TargetUID {
			return stableError(
				"invalid_argument",
				"sameResolvedTarget member "+member.Name+" resolves through "+constraint.Through+
					" to UID "+through.TargetUID+", want anchor UID "+anchor.TargetUID,
			)
		}
	}
	return nil
}
