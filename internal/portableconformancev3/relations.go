package portableconformancev3

import (
	"sort"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// storedRelation is one resolved cross-resource reference the host keeps
// alongside the resource that declared it.
//
// The UID is the whole point. A name is a label a client chose and can reuse;
// the UID is the host-issued identity of one incarnation. Storing only the name
// would silently re-bind a source to a different resource the moment a target
// was deleted and recreated under the same name, which is exactly the failure
// the ExternalChange condition exists to report.
type storedRelation struct {
	// Pointer is the concrete JSON Pointer of the reference inside the stored
	// spec; Relation is the derived schema pointer it came from.
	Pointer  string
	Relation string
	// The exact target identity at resolution time.
	TargetAPIVersion string
	TargetKind       string
	TargetName       string
	TargetUID        string
	// TargetRef is the exact FormRef the resolved target was recorded under.
	//
	// The UID pins WHICH incarnation the source is bound to; this pins WHAT
	// CONTRACT that incarnation satisfies. Without it a target whose Form moved
	// to an incompatible definition version would keep satisfying every
	// reference to it, because group and kind are all anyone ever checked
	// (decision 0022).
	TargetRef FormRef
	// BindingRef is the exact digest-bound Binding contract this relation is
	// governed by, absent for a plain cross-resource reference.
	BindingRef *formpackage.BindingRef
}

// resolveRelations resolves every relation the Form derives from its desired
// schema against the materialized spec, verifies the binding contracts, and
// returns the records to store. It runs before any mutation on the apply and
// import paths alike, and is re-run at async commit time.
func (h *ReferenceHost) resolveRelations(
	form *InstalledForm,
	space string,
	spec map[string]any,
) ([]storedRelation, *hostError) {
	instances := currentformmodel.RelationInstances(form.Relations, spec)
	out := make([]storedRelation, 0, len(instances))
	for _, instance := range instances {
		target := h.resources[resourceKey(space, instance.TargetAPIVersion, instance.TargetKind, instance.TargetName)]
		if target == nil {
			return nil, stableError(
				"resource_not_found",
				"relation "+instance.Pointer+" names absent "+instance.TargetAPIVersion+" "+
					instance.TargetKind+" "+instance.TargetName,
			)
		}
		// The reference pins an exact group and kind, so a host must resolve to
		// a resource of exactly that Form. The target's OWN recorded ref names
		// that Form: it is the contract the target was applied under, and the
		// only one this host may answer about it (decision 0022).
		targetForm := h.catalog.exact(target.Ref)
		if targetForm == nil ||
			targetForm.Ref.APIVersion != instance.TargetAPIVersion ||
			targetForm.Ref.Kind != instance.TargetKind {
			return nil, stableError(
				"invalid_argument",
				"relation "+instance.Pointer+" resolved to a resource whose Form is not "+
					instance.TargetAPIVersion+" "+instance.TargetKind,
			)
		}
		record := storedRelation{
			Pointer:          instance.Pointer,
			Relation:         instance.Relation,
			TargetAPIVersion: instance.TargetAPIVersion,
			TargetKind:       instance.TargetKind,
			TargetName:       instance.TargetName,
			TargetUID:        target.UID,
			TargetRef:        target.Ref,
		}
		// A binding relation is held to its Binding Definition first, in the
		// published order of decision 0015 rule 8, and the annotated requirement
		// second. The order matters only for which message a caller gets — the
		// catalog refuses to render a binding list whose annotation differs from
		// its contract's targetInterface, so the two can never disagree about the
		// verdict — and the published order is the one clients were told.
		if instance.Binding != "" {
			ref, hostErr := h.verifyBindingContract(form, targetForm, instance)
			if hostErr != nil {
				return nil, hostErr
			}
			record.BindingRef = &ref
		}
		// The annotated requirement is verified BEFORE anything is stored, for
		// the same reason the binding contract is: a relation that resolved to a
		// target satisfying a different contract is not a relation this host can
		// keep, and every later dependency and drift decision is made from the
		// record above.
		if hostErr := verifyTargetContract(instance, target, targetForm); hostErr != nil {
			return nil, hostErr
		}
		out = append(out, record)
	}
	return out, nil
}

// verifyTargetContract proves the resolved target still satisfies what the
// declaring reference stated about it (decision 0022).
//
// The two annotations are alternatives and both are checked against the
// target's OWN recorded identity, never against whatever the catalog currently
// holds for that kind. An exact-Form relation is satisfied only by a target
// recorded under one of the listed identities; an Interface relation is
// satisfied by any target whose Form declares that exact contract in
// providedInterfaces. A relation that stated neither is refused rather than
// admitted, because there would be nothing to verify — and the derivation that
// produced it already refuses, so reaching this branch means a Form Definition
// changed underneath a running host.
//
// The refusal is `invalid_argument` and names the pointer and what was
// required: the request is well formed and states something untrue about the
// resource it points at, which is exactly the class of refusal the binding
// rules already use for a target the client chose.
func verifyTargetContract(
	instance currentformmodel.RelationInstance,
	target *storedResource,
	targetForm *InstalledForm,
) *hostError {
	identity := instance.TargetKind + " " + instance.TargetName +
		" (recorded as " + exactFormKey(target.Ref).String() + ")"
	switch {
	case len(instance.TargetFormRefs) > 0:
		accepted := make([]string, 0, len(instance.TargetFormRefs))
		for _, want := range instance.TargetFormRefs {
			if want.APIVersion == target.Ref.APIVersion && want.Kind == target.Ref.Kind &&
				want.DefinitionVersion == target.Ref.DefinitionVersion &&
				want.SchemaDigest == target.Ref.SchemaDigest {
				return nil
			}
			accepted = append(accepted, want.String())
		}
		return stableError(
			"invalid_argument",
			"relation "+instance.Pointer+" requires its target to be one of ["+
				strings.Join(accepted, ", ")+"], but "+identity+" is not",
		)
	case instance.RequiredInterface != nil:
		want := formpackage.InterfaceRef{
			APIVersion:   instance.RequiredInterface.APIVersion,
			Name:         instance.RequiredInterface.Name,
			Version:      instance.RequiredInterface.Version,
			SchemaDigest: instance.RequiredInterface.SchemaDigest,
		}
		if targetForm.providesInterface(want) {
			return nil
		}
		return stableError(
			"invalid_argument",
			"relation "+instance.Pointer+" requires its target to provide interface "+
				instance.RequiredInterface.String()+", which "+identity+" does not",
		)
	default:
		return stableError(
			"invalid_argument",
			"relation "+instance.Pointer+" states no target contract; a host cannot verify what it requires",
		)
	}
}

// verifyBindingContract proves every rule of spec/binding-contract/README.md
// before a binding is stored. None of it is assumed from the fact that the
// desired spec validated: a schema states the shape of a reference, never
// whether the contract behind it can be honored.
func (h *ReferenceHost) verifyBindingContract(
	form *InstalledForm,
	targetForm *InstalledForm,
	instance currentformmodel.RelationInstance,
) (formpackage.BindingRef, *hostError) {
	// 1. The declaring Form Definition must accept this Binding contract.
	ref, accepted := form.acceptedBinding(instance.Binding)
	if !accepted {
		return formpackage.BindingRef{}, stableError(
			"invalid_argument",
			"relation "+instance.Pointer+" carries binding "+instance.Binding+
				", which the installed "+form.Ref.Kind+" Definition does not accept",
		)
	}
	// 2. The host must have installed that exact contract; an unknown or
	//    differently-digested contract is a capability this host does not have.
	contract, installed := h.catalog.bindingContractByName(ref.Name, ref.Version)
	if !installed || contract.Ref.SchemaDigest != ref.SchemaDigest {
		return formpackage.BindingRef{}, stableError(
			"unsupported_capability",
			"the host declares no support for binding "+ref.Name+"@"+ref.Version+
				" at the exact digest the Form Definition accepts",
		)
	}
	// 3. The declaring Form's role must be the Binding's sourceRole.
	if form.Role != contract.SourceRole {
		return formpackage.BindingRef{}, stableError(
			"invalid_argument",
			"binding "+ref.Name+" is held by sourceRole "+contract.SourceRole+
				", but "+form.Ref.Kind+" has role "+form.Role,
		)
	}
	// 4. The resolved target's exact Form must be an allowed target.
	if !contract.allowsTarget(targetForm.Ref.APIVersion, targetForm.Ref.Kind) {
		return formpackage.BindingRef{}, stableError(
			"invalid_argument",
			"binding "+ref.Name+" does not list "+targetForm.Ref.APIVersion+" "+
				targetForm.Ref.Kind+" in allowedTargetForms",
		)
	}
	// 5. The target Form must declare the Binding's targetInterface. A binding
	//    projects an Interface, so a target that provides none cannot be bound.
	if !targetForm.providesInterface(contract.TargetInterface) {
		return formpackage.BindingRef{}, stableError(
			"invalid_argument",
			"binding "+ref.Name+" requires interface "+contract.TargetInterface.Name+"@"+
				contract.TargetInterface.Version+", which Form "+targetForm.Ref.Kind+" does not provide",
		)
	}
	// 6. Same space. A reference carries no space member at all, so the target
	//    was resolved inside the source's own space by construction; a
	//    cross-space binding is unrepresentable rather than refused.
	return ref, nil
}

// repinRelations attaches the relations THIS apply resolved to the
// post-mutation resource.
//
// Every accepted mutation re-resolves and re-pins, including a spec-identical
// one, which is what makes re-applying the remedy for a target that was
// replaced out of band: the source stops pointing at an incarnation that is
// gone. Re-pinning is host-owned bookkeeping, never desired state, so it MUST
// NOT move generation — the client's desired spec is byte-identical. It does
// change the representation the host serves, because the Ready condition stops
// reporting ExternalChange, so it moves revision when the mutation has not
// moved it already (spec/decisions/0011).
func repinRelations(next, existing *storedResource, relations []storedRelation) {
	next.Relations = relations
	if existing == nil || relationsEqual(existing.Relations, relations) {
		return
	}
	if next.Generation == existing.Generation && next.Revision == existing.Revision {
		next.Revision++
	}
}

// relationsEqual compares two resolved relation sets. Both are produced in one
// stable pointer order, so position-wise comparison is exact.
func relationsEqual(left, right []storedRelation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !relationEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func relationEqual(left, right storedRelation) bool {
	switch {
	case left.BindingRef == nil && right.BindingRef == nil:
	case left.BindingRef == nil || right.BindingRef == nil:
		return false
	case *left.BindingRef != *right.BindingRef:
		return false
	}
	left.BindingRef, right.BindingRef = nil, nil
	return left == right
}

// relationTargetUID returns the UID one stored relation pointer resolved to,
// or the empty string when the resource declares no such relation.
func relationTargetUID(relations []storedRelation, pointer string) string {
	for _, relation := range relations {
		if relation.Pointer == pointer {
			return relation.TargetUID
		}
	}
	return ""
}

// indexRelations records this resource as a holder of every target UID it
// references. The index is keyed by UID, never by name: a name can be reused
// by a different resource, and a holder of the old incarnation must not look
// like a holder of the new one.
func (h *ReferenceHost) indexRelations(resource *storedResource) {
	key := resource.key()
	for _, relation := range resource.Relations {
		holders := h.relationHolders[relation.TargetUID]
		if holders == nil {
			holders = map[string]struct{}{}
			h.relationHolders[relation.TargetUID] = holders
		}
		holders[key] = struct{}{}
	}
}

func (h *ReferenceHost) unindexRelations(resource *storedResource) {
	key := resource.key()
	for _, relation := range resource.Relations {
		holders := h.relationHolders[relation.TargetUID]
		delete(holders, key)
		if len(holders) == 0 {
			delete(h.relationHolders, relation.TargetUID)
		}
	}
}

// dependencyInUse refuses to delete a resource any live dependent needs.
//
// Two kinds of dependency are covered. The first is every stored relation that
// references this resource by UID — not only typed bindings, but a Worker
// Version pinned by a deployment, a bundle a version executes, and a
// dead-letter queue a consumer drains. The second is the Worker aggregate rule
// of spec/decisions/0016: nothing references a WorkerDeployment, yet it is what
// serves every attachment and every inbound service binding of its worker, so
// removing it while one of those lives is the same class of mistake.
func (h *ReferenceHost) dependencyInUse(resource *storedResource) *hostError {
	if hostErr := h.deploymentDeleteBlocked(resource); hostErr != nil {
		return hostErr
	}
	key := resource.key()
	holders := make([]string, 0, len(h.relationHolders[resource.UID]))
	for holder := range h.relationHolders[resource.UID] {
		if holder == key {
			// A self-reference never blocks a resource's own deletion.
			continue
		}
		holders = append(holders, holder)
	}
	if len(holders) == 0 {
		return nil
	}
	sort.Strings(holders)
	first := h.resources[holders[0]]
	detail := ""
	if first != nil {
		detail = " held by " + first.kind() + " " + first.Name
	}
	return stableError(
		"dependency_in_use",
		"resource is the target of "+strconv.Itoa(len(holders))+" live relation holder(s)"+detail,
	)
}

// relationDrift reports the first stored relation whose target no longer
// matches what this resource was bound to — a different UID, or the same name
// under a different exact contract.
//
// A host MUST NOT quietly re-resolve the name: re-binding would make a
// delete-and-recreate of a target invisible, and the source would start
// pointing at a resource its author never named. The source reports the change
// and stays bound to nothing until it is re-applied.
//
// The recorded target FormRef travels in the same report rather than in a
// parallel one. A target whose contract moved is the same class of fact as a
// target whose incarnation moved — the source is pinned to something that is no
// longer there — so it is `ExternalChange` with the same remedy, and a client
// that already renders relation drift renders this without learning anything
// new (decision 0022).
func (h *ReferenceHost) relationDrift(resource *storedResource) (reason, hostReason string, drifted bool) {
	for _, relation := range resource.Relations {
		current := h.resources[resourceKey(
			resource.Space, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName,
		)]
		identity := relation.TargetAPIVersion + " " + relation.TargetKind + " " + relation.TargetName
		if current == nil {
			return "DependencyMissing",
				"relation " + relation.Pointer + " target " + identity + " uid " +
					relation.TargetUID + " no longer exists", true
		}
		if current.UID != relation.TargetUID {
			return "ExternalChange",
				"relation " + relation.Pointer + " target " + identity + " changed incarnation from uid " +
					relation.TargetUID + " (formRef " + exactFormKey(relation.TargetRef).String() +
					") to uid " + current.UID + " (formRef " + exactFormKey(current.Ref).String() +
					"); the host does not re-bind automatically, re-apply this resource", true
		}
		if current.Ref != relation.TargetRef {
			return "ExternalChange",
				"relation " + relation.Pointer + " target " + identity + " uid " + relation.TargetUID +
					" changed contract from formRef " + exactFormKey(relation.TargetRef).String() +
					" to formRef " + exactFormKey(current.Ref).String() +
					"; the host does not re-bind automatically, re-apply this resource", true
		}
	}
	return "", "", false
}
