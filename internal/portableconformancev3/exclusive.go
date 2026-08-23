package portableconformancev3

// exclusive.go enforces declared reference cardinality — the mechanism that
// replaced four hand-written rules.
//
// A worker carries one active deployment. A queue carries one consumer. A
// database carries one live migration application. A worker carries one class
// holder per class name. Those were four paragraphs in the protocol document,
// four functions in this host, and four Form kinds the protocol had to name —
// which is why adding a Form to a family meant editing the protocol.
//
// They are one rule: at most one LIVE resource of a kind may hold the target
// one of its references resolves to. What differs between them is only the
// KEY: three are keyed by the target alone and one by the target paired with a
// sibling property. A Form declares which, the host reads the declaration, and
// nothing here knows what a worker or a queue is.

import (
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// validateExclusiveHolds refuses a create or update that would make a second
// live resource of one kind hold a target that already has one.
//
// It is invalid_argument rather than dependency_in_use because the request is
// well formed and what is untrue is what it says about the target it points
// at — the same reading the four rules it replaces each arrived at
// separately.
func (h *ReferenceHost) validateExclusiveHolds(
	form *InstalledForm,
	scope resourceScope,
	name string,
	spec map[string]any,
	relations []storedRelation,
) *hostError {
	selfKey := resourceKey(scope, form.Ref.APIVersion, form.Ref.Kind, name)
	for _, declared := range form.Relations {
		if declared.Exclusive == nil {
			continue
		}
		targetUID := relationTargetUID(relations, declared.Pointer)
		if targetUID == "" {
			// The reference did not resolve to a live target. Whatever is
			// wrong with that, it is not this rule's to report: relation
			// resolution already refuses an unresolvable required reference,
			// and an absent optional one holds nothing.
			continue
		}
		key, keyed := exclusiveKey(declared.Exclusive, spec)
		if declared.Exclusive.KeyedBy != "" && !keyed {
			// The key member the declaration names is absent from a spec that
			// validated, which can only mean the Definition declares a key
			// that is not a required property of its own desired schema.
			return stableError(
				"invalid_argument",
				form.Ref.Kind+" "+name+" declares an exclusive hold keyed by "+
					quoteText(declared.Exclusive.KeyedBy)+", which this spec does not carry",
			)
		}
		if holder := h.exclusiveHolder(scope, form, declared, targetUID, key, selfKey); holder != "" {
			return stableError("invalid_argument", exclusiveConflictMessage(
				form.Ref.Kind, name, holder, declared, targetUID, key,
			))
		}
	}
	return nil
}

// exclusiveHolder returns the name of the live resource already holding this
// target under this key, or the empty string when the target is free.
func (h *ReferenceHost) exclusiveHolder(
	scope resourceScope,
	form *InstalledForm,
	declared currentformmodel.Relation,
	targetUID, key, selfKey string,
) string {
	// Sorted, so a tenant holding two conflicting resources is reported the
	// same way twice rather than by whichever the map yielded first.
	candidates := h.scopedResources(scope)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Name < candidates[j].Name })
	for _, candidate := range candidates {
		if candidate.key() == selfKey {
			continue
		}
		// The same exact kind, in the same family. A different kind pointing
		// at one target is not a conflict: the rule is one holder OF THIS
		// KIND, which is what lets a worker carry a deployment and an
		// endpoint at once.
		if candidate.group() != form.Ref.APIVersion || candidate.kind() != form.Ref.Kind {
			continue
		}
		if relationTargetUID(candidate.Relations, declared.Pointer) != targetUID {
			continue
		}
		candidateKey, _ := exclusiveKey(declared.Exclusive, candidate.Spec)
		if candidateKey != key {
			continue
		}
		return candidate.Name
	}
	return ""
}

// exclusiveKey reads the sibling member that joins the target in the key. An
// unkeyed hold has one key for every holder, so the target alone decides.
func exclusiveKey(hold *currentformmodel.ExclusiveHold, spec map[string]any) (string, bool) {
	if hold.KeyedBy == "" {
		return "", true
	}
	value, present := specValueAtPointer(spec, hold.KeyedBy)
	if !present {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return text, true
}

// specValueAtPointer resolves one JSON Pointer against a desired spec. It
// walks objects only: an exclusive key is a scalar property of the spec, and a
// pointer into an array would name a member whose position is not part of any
// identity.
func specValueAtPointer(spec map[string]any, pointer string) (any, bool) {
	node := any(spec)
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		object, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		next, present := object[unescapeJSONPointerToken(token)]
		if !present {
			return nil, false
		}
		node = next
	}
	return node, true
}

// unescapeJSONPointerToken reverses RFC 6901 token escaping.
func unescapeJSONPointerToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}

// exclusiveConflictMessage names what is held, by whom, and under which key,
// because a refusal that only says "conflict" leaves an author guessing which
// of several references was the one that collided.
func exclusiveConflictMessage(
	kind, name, holder string,
	declared currentformmodel.Relation,
	targetUID, key string,
) string {
	message := kind + " " + name + " holds the " + declared.TargetKind + " at uid " + targetUID +
		" through " + declared.Pointer + ", which " + kind + " " + holder + " already holds"
	if declared.Exclusive.KeyedBy != "" {
		message += " for " + declared.Exclusive.KeyedBy + " " + quoteText(key)
	}
	return message + "; that target admits one live holder of this kind"
}
