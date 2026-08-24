package portableconformancev3

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// constraintProbeTarget builds a scratch resource under one byte-pinned
// conformance-only Definition. These Forms are not product-family probes: they
// exist only to make each generic stable-v1 constraint independently observable.
func (r *v3Runner) constraintProbeTarget(
	probe ConstraintDefinitionProbe,
	name string,
	spec map[string]any,
) probeTarget {
	target := probeTarget{
		Ref: probe.FormRef, Name: name, Space: r.contract.RunnerInput.Space,
		Spec: spec,
	}
	if probe.Definition != nil {
		target.Lifecycle = append([]string(nil), probe.Definition.LifecycleCapabilities...)
	}
	return target
}

func constraintReference(target probeTarget, name string) map[string]any {
	return map[string]any{
		"apiVersion": target.Ref.APIVersion,
		"kind":       target.Ref.Kind,
		"name":       name,
	}
}

func (r *v3Runner) createConstraintProbe(target probeTarget, key string) (wireResource, error) {
	created, _, err := r.applyResource(target, applyOptions{
		Create: true, IdempotencyKey: key,
	}, http.StatusCreated)
	return created, err
}

func (r *v3Runner) expectConstraintPrepareInvalid(target probeTarget, subject string) error {
	response, err := r.prepareRequest(target, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	return nil
}

func (r *v3Runner) expectConstraintValidateInvalid(target probeTarget, subject string) error {
	valid, diagnostics, err := r.validateResource(target, target.Spec)
	if err != nil {
		return err
	}
	if valid || diagnostics == 0 {
		return fmt.Errorf("%s passed validate without a diagnostic", subject)
	}
	return nil
}

func acceptedConstraintOperation(response wireResponse, subject string) (wireOperation, error) {
	if response.Status != http.StatusAccepted {
		return wireOperation{}, fmt.Errorf(
			"%s HTTP %d, want 202; body=%s",
			subject, response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(response, &envelope); err != nil {
		return wireOperation{}, err
	}
	if envelope.Operation.ID == "" || envelope.Operation.Done {
		return wireOperation{}, fmt.Errorf("%s returned an invalid pending operation", subject)
	}
	return envelope.Operation, nil
}

// checkDeclaredConstraintSemanticsEnforced is the exact stable-v1 black-box
// matrix for all six newly modeled mechanisms. The Form Definition check has
// already compared their byte-pinned declarations; this check proves those
// declarations affect validate, prepare, synchronous apply/import, and async
// commit rather than being parsed and discarded.
func (r *v3Runner) checkDeclaredConstraintSemanticsEnforced() error {
	input := r.contract.RunnerInput.ConstraintSemantics
	nodeA := r.constraintProbeTarget(input.Node, "constraint-node-a", map[string]any{})
	nodeB := r.constraintProbeTarget(input.Node, "constraint-node-b", map[string]any{})
	nodeACreated, err := r.createConstraintProbe(nodeA, "key-constraint-node-a")
	if err != nil {
		return err
	}
	if _, err := r.createConstraintProbe(nodeB, "key-constraint-node-b"); err != nil {
		return err
	}

	// orderedPair and uniqueBy are required desired-document rules. They are
	// driven separately so a host implementing either one cannot mask the other.
	structuralDescending := r.constraintProbeTarget(input.Structural, "constraint-structural-descending", map[string]any{
		"lower": 3, "upper": 2,
		"rows": []any{map[string]any{"key": "one", "value": 1}},
	})
	if err := r.expectConstraintValidateInvalid(structuralDescending, "orderedPair descending operands"); err != nil {
		return err
	}
	if err := r.expectConstraintPrepareInvalid(structuralDescending, "orderedPair descending operands"); err != nil {
		return err
	}
	structuralDuplicate := r.constraintProbeTarget(input.Structural, "constraint-structural-duplicate", map[string]any{
		"lower": 1, "upper": 2,
		"rows": []any{
			map[string]any{"key": "one", "value": 1},
			map[string]any{"key": "one", "value": 2},
		},
	})
	if err := r.expectConstraintValidateInvalid(structuralDuplicate, "uniqueBy repeated scalar member"); err != nil {
		return err
	}
	if err := r.expectConstraintPrepareInvalid(structuralDuplicate, "uniqueBy repeated scalar member"); err != nil {
		return err
	}
	structuralValid := r.constraintProbeTarget(input.Structural, "constraint-structural-valid", map[string]any{
		"lower": 1, "upper": 2,
		"rows": []any{
			map[string]any{"key": "one", "value": 1},
			map[string]any{"key": "two", "value": 2},
		},
	})
	if _, err := r.createConstraintProbe(structuralValid, "key-constraint-structural-valid"); err != nil {
		return fmt.Errorf("valid orderedPair/uniqueBy resource: %w", err)
	}

	// distinctPair is inactive when its optional right operand is absent, but
	// compares immutable resolved UIDs once both operands are present.
	distinctMissing := r.constraintProbeTarget(input.DistinctPair, "constraint-distinct-missing", map[string]any{
		"left": constraintReference(nodeA, nodeA.Name),
	})
	if _, err := r.prepare(distinctMissing); err != nil {
		return fmt.Errorf("distinctPair with an absent optional operand: %w", err)
	}
	distinctSame := r.constraintProbeTarget(input.DistinctPair, "constraint-distinct-same", map[string]any{
		"left":  constraintReference(nodeA, nodeA.Name),
		"right": constraintReference(nodeA, nodeA.Name),
	})
	if err := r.expectConstraintValidateInvalid(distinctSame, "distinctPair with one resolved UID"); err != nil {
		return err
	}
	if err := r.expectConstraintPrepareInvalid(distinctSame, "distinctPair with one resolved UID"); err != nil {
		return err
	}
	imported, err := r.importResource(distinctSame, importOptions{
		NativeID: "native-constraint-distinct-same", Create: true,
		IdempotencyKey: "key-constraint-distinct-import",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(imported, "invalid_argument"); err != nil {
		return fmt.Errorf("distinctPair import bypass: %w", err)
	}

	// uniquePair keys the ORDERED UID pair by tenant and exact FormRef. The
	// reverse order and a second Definition version are both distinct keys.
	pairAB := map[string]any{
		"left":  constraintReference(nodeA, nodeA.Name),
		"right": constraintReference(nodeB, nodeB.Name),
	}
	uniqueOne := r.constraintProbeTarget(input.UniquePair, "constraint-unique-one", pairAB)
	if _, err := r.createConstraintProbe(uniqueOne, "key-constraint-unique-one"); err != nil {
		return err
	}
	uniqueDuplicate := r.constraintProbeTarget(input.UniquePair, "constraint-unique-duplicate", pairAB)
	if err := r.expectConstraintPrepareInvalid(uniqueDuplicate, "uniquePair duplicate ordered UID pair"); err != nil {
		return err
	}
	if err := r.deleteExisting(uniqueOne, "key-constraint-unique-one-delete"); err != nil {
		return fmt.Errorf("delete uniquePair holder: %w", err)
	}
	if _, err := r.createConstraintProbe(uniqueDuplicate, "key-constraint-unique-after-release"); err != nil {
		return fmt.Errorf("uniquePair was not released by holder deletion: %w", err)
	}
	uniqueReversed := r.constraintProbeTarget(input.UniquePair, "constraint-unique-reversed", map[string]any{
		"left":  constraintReference(nodeB, nodeB.Name),
		"right": constraintReference(nodeA, nodeA.Name),
	})
	if _, err := r.createConstraintProbe(uniqueReversed, "key-constraint-unique-reversed"); err != nil {
		return fmt.Errorf("uniquePair reversed order: %w", err)
	}
	uniqueOtherForm := r.constraintProbeTarget(input.UniquePairSecond, "constraint-unique-other-form", pairAB)
	if _, err := r.createConstraintProbe(uniqueOtherForm, "key-constraint-unique-other-form"); err != nil {
		return fmt.Errorf("uniquePair exact-Form scope: %w", err)
	}

	// The same names in another tenant resolve inside that tenant and may hold
	// its own pair. A global index would refuse this positive control.
	for _, target := range []probeTarget{nodeA, nodeB} {
		response, err := r.applyAs(r.alternateTenantToken, target, applyOptions{
			Create: true, IdempotencyKey: "key-other-tenant-" + target.Name,
		})
		if err != nil {
			return err
		}
		if _, err := decodeResource(response, http.StatusCreated); err != nil {
			return fmt.Errorf("other tenant creating %s: %w", target.Name, err)
		}
	}
	foreignPair := r.constraintProbeTarget(input.UniquePair, "constraint-unique-other-tenant", pairAB)
	foreignResponse, err := r.applyAs(r.alternateTenantToken, foreignPair, applyOptions{
		Create: true, IdempotencyKey: "key-constraint-unique-other-tenant",
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(foreignResponse, http.StatusCreated); err != nil {
		return fmt.Errorf("uniquePair tenant scope: %w", err)
	}

	// Two reviews may be prepared while a pair is free, but atomic check/write
	// permits only one commit. This is the race a prepare-only implementation
	// misses and the reason a production host needs a serializable reservation.
	nodeC := r.constraintProbeTarget(input.Node, "constraint-node-c", map[string]any{})
	nodeD := r.constraintProbeTarget(input.Node, "constraint-node-d", map[string]any{})
	if _, err := r.createConstraintProbe(nodeC, "key-constraint-node-c"); err != nil {
		return err
	}
	if _, err := r.createConstraintProbe(nodeD, "key-constraint-node-d"); err != nil {
		return err
	}
	pairCD := map[string]any{
		"left":  constraintReference(nodeC, nodeC.Name),
		"right": constraintReference(nodeD, nodeD.Name),
	}
	tracerOne := r.constraintProbeTarget(input.UniquePair, "constraint-unique-race-one", pairCD)
	tracerTwo := r.constraintProbeTarget(input.UniquePair, "constraint-unique-race-two", pairCD)
	preparedOne, err := r.prepare(tracerOne)
	if err != nil {
		return err
	}
	preparedTwo, err := r.prepare(tracerTwo)
	if err != nil {
		return err
	}
	acceptedOne, err := r.apply(tracerOne, applyOptions{
		Create: true, IdempotencyKey: "key-constraint-unique-race-one",
		PrepareDigest: preparedOne.PrepareDigest,
		ExtraHeaders:  map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	operationOne, err := acceptedConstraintOperation(acceptedOne, "first uniquePair race apply")
	if err != nil {
		return err
	}
	acceptedTwo, err := r.apply(tracerTwo, applyOptions{
		Create: true, IdempotencyKey: "key-constraint-unique-race-two",
		PrepareDigest: preparedTwo.PrepareDigest,
		ExtraHeaders:  map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	var operationTwo wireOperation
	if acceptedTwo.Status == http.StatusAccepted {
		operationTwo, err = acceptedConstraintOperation(acceptedTwo, "second uniquePair race apply")
		if err != nil {
			return err
		}
	} else if err := r.expectStableError(acceptedTwo, "invalid_argument"); err != nil {
		return fmt.Errorf("second uniquePair race apply was neither reserved away nor accepted: %w", err)
	}
	terminalOne, err := r.pollOperation(operationOne.ID)
	if err != nil {
		return err
	}
	if terminalOne.Error != nil || terminalOne.Result == nil {
		return errors.New("the first uniquePair race operation did not commit")
	}
	if operationTwo.ID != "" {
		terminalTwo, err := r.pollOperation(operationTwo.ID)
		if err != nil {
			return err
		}
		if err := requireTerminalOperationError(terminalTwo, "invalid_argument"); err != nil {
			return fmt.Errorf("atomic uniquePair race: %w", err)
		}
	}
	if err := r.expectResourceAbsent(tracerTwo); err != nil {
		return fmt.Errorf("losing uniquePair operation mutated state: %w", err)
	}

	// acyclic follows the declared relation through pinned resources, rejects
	// self/cycles, and repeats the walk when an accepted async update commits.
	nodeBLinked := nodeB
	nodeBLinked.Spec = map[string]any{"next": constraintReference(nodeA, nodeA.Name)}
	if _, _, err := r.applyResource(nodeBLinked, applyOptions{
		ExpectedGeneration: "1", IdempotencyKey: "key-constraint-node-b-link",
	}, http.StatusOK); err != nil {
		return err
	}
	nodeCycle := nodeA
	nodeCycle.Spec = map[string]any{"next": constraintReference(nodeB, nodeB.Name)}
	if err := r.expectConstraintPrepareInvalid(nodeCycle, "acyclic two-node cycle"); err != nil {
		return err
	}
	nodeSelf := nodeA
	nodeSelf.Spec = map[string]any{"next": constraintReference(nodeA, nodeA.Name)}
	if err := r.expectConstraintValidateInvalid(nodeSelf, "acyclic self edge"); err != nil {
		return err
	}

	nodeE := r.constraintProbeTarget(input.Node, "constraint-node-e", map[string]any{})
	nodeF := r.constraintProbeTarget(input.Node, "constraint-node-f", map[string]any{})
	if _, err := r.createConstraintProbe(nodeE, "key-constraint-node-e"); err != nil {
		return err
	}
	if _, err := r.createConstraintProbe(nodeF, "key-constraint-node-f"); err != nil {
		return err
	}
	nodeE.Spec = map[string]any{"next": constraintReference(nodeF, nodeF.Name)}
	preparedE, err := r.prepareWithFence(nodeE, "1")
	if err != nil {
		return err
	}
	acceptedE, err := r.apply(nodeE, applyOptions{
		ExpectedGeneration: "1", IdempotencyKey: "key-constraint-node-e-link",
		PrepareDigest: preparedE.PrepareDigest,
		ExtraHeaders:  map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	operationE, err := acceptedConstraintOperation(acceptedE, "acyclic async update")
	if err != nil {
		return err
	}
	nodeF.Spec = map[string]any{"next": constraintReference(nodeE, nodeE.Name)}
	if _, _, err := r.applyResource(nodeF, applyOptions{
		ExpectedGeneration: "1", IdempotencyKey: "key-constraint-node-f-link",
	}, http.StatusOK); err != nil {
		return err
	}
	terminalE, err := r.pollOperation(operationE.ID)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(terminalE, "invalid_argument"); err != nil {
		return fmt.Errorf("acyclic async commit revalidation: %w", err)
	}

	// A stored edge is a UID pin, not a reusable name. An out-of-band loss and
	// replacement makes the next walk fail closed instead of silently rebinding.
	nodeG := r.constraintProbeTarget(input.Node, "constraint-node-g", map[string]any{})
	nodeGCreated, err := r.createConstraintProbe(nodeG, "key-constraint-node-g")
	if err != nil {
		return err
	}
	nodeH := r.constraintProbeTarget(input.Node, "constraint-node-h", map[string]any{
		"next": constraintReference(nodeG, nodeG.Name),
	})
	if _, err := r.createConstraintProbe(nodeH, "key-constraint-node-h"); err != nil {
		return err
	}
	removedG, err := r.deleteResource(
		nodeG, nodeGCreated.Metadata.Generation, "key-constraint-node-g-external-delete",
		map[string]string{ErrorProbeHeader: ProbeExternalChange},
	)
	if err != nil {
		return err
	}
	if removedG.Status != http.StatusNoContent {
		return fmt.Errorf("acyclic drift target external delete HTTP %d, want 204", removedG.Status)
	}
	nodeGReplacement, err := r.createConstraintProbe(nodeG, "key-constraint-node-g-recreate")
	if err != nil {
		return err
	}
	if nodeGReplacement.Metadata.UID == nodeGCreated.Metadata.UID {
		return errors.New("acyclic drift target replacement reused its UID")
	}
	nodeI := r.constraintProbeTarget(input.Node, "constraint-node-i", map[string]any{
		"next": constraintReference(nodeH, nodeH.Name),
	})
	if err := r.expectConstraintPrepareInvalid(nodeI, "acyclic replacement drift"); err != nil {
		return err
	}

	// sameResolvedTarget resolves every member resource and the relation that
	// its exact Form declares at through; all concrete UIDs must equal anchor.
	memberOne := r.constraintProbeTarget(input.Member, "constraint-member-one", map[string]any{
		"through": constraintReference(nodeA, nodeA.Name),
	})
	memberTwo := r.constraintProbeTarget(input.Member, "constraint-member-two", map[string]any{
		"through": constraintReference(nodeA, nodeA.Name),
	})
	if _, err := r.createConstraintProbe(memberOne, "key-constraint-member-one"); err != nil {
		return err
	}
	if _, err := r.createConstraintProbe(memberTwo, "key-constraint-member-two"); err != nil {
		return err
	}
	sameValid := r.constraintProbeTarget(input.SameTarget, "constraint-same-valid", map[string]any{
		"anchor": constraintReference(nodeA, nodeA.Name),
		"members": []any{
			constraintReference(memberOne, memberOne.Name),
			constraintReference(memberTwo, memberTwo.Name),
		},
	})
	if _, err := r.createConstraintProbe(sameValid, "key-constraint-same-valid"); err != nil {
		return fmt.Errorf("sameResolvedTarget equal UIDs: %w", err)
	}
	sameInvalid := r.constraintProbeTarget(input.SameTarget, "constraint-same-invalid", map[string]any{
		"anchor": constraintReference(nodeB, nodeB.Name),
		"members": []any{
			constraintReference(memberOne, memberOne.Name),
			constraintReference(memberTwo, memberTwo.Name),
		},
	})
	if err := r.expectConstraintValidateInvalid(sameInvalid, "sameResolvedTarget unequal through UID"); err != nil {
		return err
	}
	if err := r.expectConstraintPrepareInvalid(sameInvalid, "sameResolvedTarget unequal through UID"); err != nil {
		return err
	}

	// Replacing the anchor target proves both remaining UID rules are about
	// incarnations: sameResolvedTarget refuses members pinned to the old UID,
	// while uniquePair admits the genuinely new ordered pair even with the old
	// holder still live.
	removedA, err := r.deleteResource(
		nodeA, nodeACreated.Metadata.Generation, "key-constraint-node-a-external-delete",
		map[string]string{ErrorProbeHeader: ProbeExternalChange},
	)
	if err != nil {
		return err
	}
	if removedA.Status != http.StatusNoContent {
		return fmt.Errorf("constraint anchor external delete HTTP %d, want 204", removedA.Status)
	}
	nodeAReplacement, err := r.createConstraintProbe(nodeA, "key-constraint-node-a-recreate")
	if err != nil {
		return err
	}
	if nodeAReplacement.Metadata.UID == nodeACreated.Metadata.UID {
		return errors.New("constraint anchor replacement reused its UID")
	}
	sameDrift := r.constraintProbeTarget(input.SameTarget, "constraint-same-drift", map[string]any{
		"anchor": constraintReference(nodeA, nodeA.Name),
		"members": []any{
			constraintReference(memberOne, memberOne.Name),
		},
	})
	if err := r.expectConstraintPrepareInvalid(sameDrift, "sameResolvedTarget replacement drift"); err != nil {
		return err
	}
	uniqueReplacementPair := r.constraintProbeTarget(input.UniquePair, "constraint-unique-replacement-pair", pairAB)
	if _, err := r.createConstraintProbe(uniqueReplacementPair, "key-constraint-unique-replacement-pair"); err != nil {
		return fmt.Errorf("uniquePair replacement UID pair: %w", err)
	}

	r.complete("declared-constraint-semantics-enforced")
	return nil
}
