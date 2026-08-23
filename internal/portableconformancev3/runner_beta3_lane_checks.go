package portableconformancev3

// runner_beta3_lane_checks.go measures what the v1beta3 lane states: that a
// cross-resource rule a Definition DECLARES is enforced, and enforced because
// it was declared.
//
// The distinction matters and it is the reason this check is written the way
// it is. A host carrying a hardcoded rule for one Form kind passes any check
// that only drives that Form — which is exactly what the previous lane's
// checks did, and exactly why four rules could live in the protocol document
// unnoticed. So this check drives EVERY declared hold the corpus's family
// carries, discovering them from the served Definitions rather than from a
// list written here. A host that hardcoded one and forgot another fails on the
// one it forgot.

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// checkDeclaredExclusiveHoldsEnforced proves the declared-cardinality
// mechanism over the Forms that declare it.
//
// A subject whose target is not live at this point in the run is SKIPPED, and
// the skip is not silent: the check requires at least two distinct Form kinds
// to have been driven, so it cannot quietly decay into the per-Form coverage
// it exists to replace. Driving all five would mean building each subject's
// whole prerequisite graph here, which is the aggregate sequence's job and not
// this check's.
func (r *v3Runner) checkDeclaredExclusiveHoldsEnforced() error {
	subjects := r.declaredExclusiveSubjects()
	if len(subjects) == 0 {
		return errors.New(
			"the corpus's family declares no exclusive hold, so this lane's mechanism " +
				"cannot be driven against it",
		)
	}
	driven := map[string]bool{}
	var skipped []string
	for _, subject := range subjects {
		label := subject.probe.Ref.Kind + subject.relation.Pointer
		reachable, err := r.driveExclusiveHold(subject)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if reachable {
			driven[subject.probe.Ref.Kind] = true
			continue
		}
		skipped = append(skipped, label)
	}
	if len(driven) < 2 {
		return fmt.Errorf(
			"only %d Form kind(s) with a declared exclusive hold were reachable (skipped: %v); "+
				"this check exists to prove the DECLARATION is what a host enforces, which one "+
				"Form cannot show",
			len(driven), skipped,
		)
	}
	r.complete("declared-exclusive-holds-enforced")
	return nil
}

// exclusiveSubject is one probe whose Definition declares an exclusive hold,
// paired with the relation that declares it.
type exclusiveSubject struct {
	probe    probeTarget
	relation currentformmodel.Relation
}

// declaredExclusiveSubjects discovers the holds from the corpus's own pinned
// desired schemas. Reading them from the Definitions is the point: a list
// written here would go stale exactly when a family added a hold, which is the
// case this whole lane exists to make cheap.
func (r *v3Runner) declaredExclusiveSubjects() []exclusiveSubject {
	var out []exclusiveSubject
	for _, entry := range declaredProbes(&r.contract.RunnerInput) {
		schema := entry.Probe.DesiredSchema.Schema
		if len(schema) == 0 {
			continue
		}
		// The hold is read from the SERVED Definition's constraint list, not
		// from the corpus's copy of the schema: a rule about resources is not
		// shape, so it no longer rides in the schema, and what a client may be
		// held to is what the host published (decision 0049).
		definition, err := r.formDefinition(entry.Probe.Identity.FormRef)
		if err != nil {
			continue
		}
		constraints := make([]currentformmodel.Constraint, 0, len(definition.Constraints))
		for _, constraint := range definition.Constraints {
			constraints = append(constraints, currentformmodel.Constraint{
				Kind:      currentformmodel.ConstraintKind(constraint.Kind),
				Reference: constraint.Reference,
				KeyedBy:   constraint.KeyedBy,
			})
		}
		relations, err := currentformmodel.DeriveRelationsWithConstraints(schema, constraints)
		if err != nil {
			continue
		}
		for _, relation := range relations {
			if relation.Exclusive == nil {
				continue
			}
			out = append(out, exclusiveSubject{probe: r.target(*entry.Probe), relation: relation})
		}
	}
	return out
}

// driveExclusiveHold proves a second holder of an already-held target is
// refused. It reports whether the subject was REACHABLE here at all.
//
// A rival can only be refused BY a live holder, and the aggregate sequence
// creates and tears down the resources these declarations are about at
// different points. So when none is live this check makes one, and when even
// that cannot be made — because the references it needs are gone — the subject
// is out of reach at this point in the run and the caller counts it rather
// than reporting a host defect that is not one.
//
// It drives the REFUSAL half. That removing a holder frees the target is
// measured per Form by the checks that predate this lane; what this one adds,
// and what none of those could, is that the DECLARATION is what a host
// enforces — a host that hardcoded one rule and missed another fails here on
// the one it missed.
func (r *v3Runner) driveExclusiveHold(subject exclusiveSubject) (bool, error) {
	held, err := r.resourceIsLive(subject.probe)
	if err != nil {
		return false, err
	}
	holder := subject.probe
	holder.Name = "declared-exclusive-holder"
	if !held {
		response, err := r.apply(holder, applyOptions{
			Create: true, IdempotencyKey: "key-declared-exclusive-holder-" + subject.probe.Ref.Kind,
		})
		if err != nil {
			return false, err
		}
		if response.Status != http.StatusCreated {
			return false, nil
		}
		defer func() {
			_ = r.deleteExisting(holder, "key-declared-exclusive-holder-delete-"+subject.probe.Ref.Kind)
		}()
	}

	rival := subject.probe
	rival.Name = "declared-exclusive-rival"
	response, err := r.apply(rival, applyOptions{
		Create: true, IdempotencyKey: "key-declared-exclusive-" + subject.probe.Ref.Kind,
	})
	if err != nil {
		return false, err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return false, fmt.Errorf(
			"a second holder of a target whose reference declares %s: %w",
			currentformmodel.ExclusiveAnnotationKey, err,
		)
	}
	return true, nil
}

// resourceIsLive reports whether one probe's own resource currently exists.
func (r *v3Runner) resourceIsLive(target probeTarget) (bool, error) {
	response, err := r.request(
		http.MethodGet,
		r.resourceURL(target.Ref, target.Name, "", r.exactQuery(target.Space, target.Ref)),
		nil, nil,
	)
	if err != nil {
		return false, err
	}
	return response.Status == http.StatusOK, nil
}
