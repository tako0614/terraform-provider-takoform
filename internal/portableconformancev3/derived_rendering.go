package portableconformancev3

// derived_rendering.go carries the one rule that keeps a strong ETag honest on a
// host that renders part of a representation from OTHER resources.
//
// This lane renders two facts at read time rather than storing them: the
// relation drift of decision 0015, and the Worker readiness of decision 0016. A
// resource's conditions therefore change when a DIFFERENT resource is mutated —
// creating a deployment makes a worker Ready, deleting a target makes its source
// report DependencyMissing — while nothing about the resource itself moved.
//
// Under decision 0011 `metadata.revision` increments whenever the representation
// changes and the strong ETag is exactly the quoted revision. Leaving the
// revision alone would therefore serve two different documents under one ETag: a
// client could read Ready=False, create the serving deployment, read Ready=True,
// and find the same revision and the same ETag on both — which is a validator
// that says "unchanged" about a change. Worse, it would make If-Match a fence on
// nothing: a mutation conditioned on the representation the client last saw
// would pass against a representation it never saw.
//
// The rule is therefore: after any accepted mutation, every resource whose
// rendered representation actually differs advances its revision, and none of
// them moves its generation — no desired spec changed.

// derivedConditions renders the `status.conditions` of one resource: the ONLY
// part of a v1alpha3 representation this host computes from resources other
// than the one being rendered. Everything else renderResource emits comes from
// the resource's own stored identity, spec, and status counters.
//
// It is the single source of both the served conditions and the comparison key
// below, so the two cannot drift apart: a future condition rendered from the
// store is covered by the revision rule the moment it is added here.
func (h *ReferenceHost) derivedConditions(resource *storedResource) []map[string]any {
	ready := map[string]any{
		"type":               "Ready",
		"status":             "True",
		"reason":             "Available",
		"lastTransitionTime": fixedTransitionTime,
	}
	// A stored relation whose target moved is reported, never repaired. The
	// reason stays inside the closed portable vocabulary; the pointer and both
	// uids travel in the free-form hostReason.
	if reason, hostReason, drifted := h.relationDrift(resource); drifted {
		ready["status"] = "False"
		ready["reason"] = reason
		ready["hostReason"] = hostReason
	} else if reason, hostReason, unavailable := h.workerServiceUnavailable(resource); unavailable {
		// A Module Worker's readiness is a claim about SERVICE, and what serves
		// is whatever its active deployment selects (spec/decisions/0016).
		ready["status"] = "False"
		ready["reason"] = reason
		ready["hostReason"] = hostReason
	}
	return []map[string]any{ready}
}

// derivedRendering is the exact comparison key for those conditions. The bytes
// are the served bytes, so "the rendering changed" means precisely "a client
// reading again would receive a different document", never an approximation of
// it.
func (h *ReferenceHost) derivedRendering(resource *storedResource) string {
	return string(encodeJSONBody(h.derivedConditions(resource)))
}

// advanceDerivedRevisions restores the invariant that every stored derived
// rendering equals what the store now renders, advancing the revision of each
// resource whose rendering actually changed. mutated is the resource key the
// caller has already settled — apply, import, and delete each decide their own
// subject's identity — and is skipped here.
//
// Scope. The mutated resource's SPACE is the exact blast radius. A reference
// carries no space member, so a relation never crosses a space, and every
// Worker aggregate lookup is space-filtered; no resource outside the space can
// render differently because of this mutation.
//
// Termination. One bounded pass over that space, no recursion and no fixpoint
// loop. The derived rendering is a pure function of the stored specs,
// relations, and UIDs — it reads no revision — so advancing a revision cannot
// change any rendering, and a second pass would find nothing left to do.
//
// No churn. A resource is touched only when its rendering DIFFERS from the one
// it was last serving, so a mutation unrelated to it — an idempotent re-apply
// that changes nothing, a resource created in another corner of the space —
// leaves its revision exactly where it was.
//
// The generation is deliberately untouched: the desired spec of a resource
// rendered differently because of someone else's mutation did not change, and a
// host that moved it would break every client's update fence for a change the
// client did not make.
func (h *ReferenceHost) advanceDerivedRevisions(space, mutated string) {
	for _, candidate := range h.sortedResources() {
		if candidate.Space != space {
			continue
		}
		key := resourceKey(candidate.Space, candidate.Group, candidate.Kind, candidate.Name)
		if key == mutated {
			continue
		}
		rendered := h.derivedRendering(candidate)
		if rendered == candidate.DerivedRendering {
			continue
		}
		candidate.DerivedRendering = rendered
		candidate.Revision++
	}
}
