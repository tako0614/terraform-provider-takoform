package portableconformancev3

// runner_derived_rendering_checks.go carries the black-box evidence for the
// derived-rendering revision rule of spec/host-api/v1alpha3.md: when a host
// renders any part of a representation from OTHER resources, mutating those
// resources changes the document it serves, so it MUST advance that resource's
// revision — and therefore its ETag — while leaving its generation alone.

import (
	"fmt"
	"net/http"
	"strings"
)

// checkDependentRevisionAdvancesWithRendering proves the rule in both places
// this lane renders a representation from the store, and proves it does not
// overshoot.
//
// Case 1 is Worker readiness (decision 0016). A worker with no deployment
// reports Ready=False; creating a serving deployment makes the SAME worker
// report Ready=True without any request touching the worker. A host that leaves
// the worker's revision alone hands a client two different documents under one
// ETag, so the client that read the first is entitled to treat it as current
// forever — and `If-Match` becomes a fence on a representation nobody saw.
//
// Case 2 is relation drift (decision 0015), the same failure reached from the
// other side: the mutation is a DELETE of a source's target, and the source's
// Ready flips to False/DependencyMissing while it is not itself mutated. The
// out-of-band delete goes through the host's ordinary removal path, so it is the
// same rule and not a second one.
//
// The no-churn half matters as much as the advance. A host that bumped every
// revision on every mutation would satisfy "the revision moved" while making the
// ETag useless as a validator, so an apply that changes nothing must leave the
// dependent exactly where it was.
func (r *v3Runner) checkDependentRevisionAdvancesWithRendering() error {
	input := r.contract.RunnerInput

	// ---- Case 1: worker readiness follows the deployment ----
	worker := r.target(input.ModuleWorker)
	worker.Name = "rendering-worker"
	if _, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-rendering-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("derived-rendering worker: %w", err)
	}
	version := r.workerVersionOf("rendering-version", worker.Name, fetchHandler)
	if _, _, err := r.applyResource(version, applyOptions{
		Create: true, IdempotencyKey: "key-rendering-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("derived-rendering version: %w", err)
	}
	// Read AFTER the version exists. A stored version is not a running one, so
	// the worker is still serving nothing, and this is the exact representation
	// the ETag below was issued for.
	before, beforeResponse, err := r.readRawResponse(worker)
	if err != nil {
		return err
	}
	if err := requireNotReady(before, "Provisioning"); err != nil {
		return fmt.Errorf("a worker whose versions no deployment selects: %w", err)
	}
	if err := verifyRevisionETag(beforeResponse, before.Metadata.Revision); err != nil {
		return fmt.Errorf("a worker with no deployment: %w", err)
	}

	deployment := r.deploymentOf("rendering-deployment", worker.Name, version.Name)
	deployed, _, err := r.applyResource(deployment, applyOptions{
		Create: true, IdempotencyKey: "key-rendering-deployment",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("derived-rendering deployment: %w", err)
	}
	after, afterResponse, err := r.readRawResponse(worker)
	if err != nil {
		return err
	}
	if condition := readyCondition(after); condition.Status != "True" || condition.Reason != "Available" {
		return fmt.Errorf(
			"the worker reports Ready=%s/%s once its deployment serves fetch, want True/Available",
			condition.Status, condition.Reason,
		)
	}
	if err := requireRevisionAdvanced(
		before, after, afterResponse,
		"creating the WorkerDeployment that makes a worker Ready",
	); err != nil {
		return err
	}
	r.revisionTransitions = append(r.revisionTransitions, "worker-readiness:"+after.Metadata.Revision)

	// The rule does not overshoot: this apply re-pins the same relations to the
	// same UIDs and changes no rendering anywhere, so nothing may move.
	if _, _, err := r.applyResource(deployment, applyOptions{
		ExpectedGeneration: deployed.Metadata.Generation,
		IdempotencyKey:     "key-rendering-deployment-again",
	}, http.StatusOK); err != nil {
		return fmt.Errorf("a spec-identical re-apply of the deployment: %w", err)
	}
	unchanged, _, err := r.readRawResponse(worker)
	if err != nil {
		return err
	}
	if unchanged.Metadata.Revision != after.Metadata.Revision ||
		unchanged.Metadata.Generation != after.Metadata.Generation {
		return fmt.Errorf(
			"an apply that changed nothing moved the worker from generation %s revision %s to %s/%s",
			after.Metadata.Generation, after.Metadata.Revision,
			unchanged.Metadata.Generation, unchanged.Metadata.Revision,
		)
	}

	// ---- Case 2: relation drift ----
	kv := r.target(input.EdgeKvNamespace)
	kv.Name = "rendering-kv"
	relationTarget, _, err := r.applyResource(kv, applyOptions{
		Create: true, IdempotencyKey: "key-rendering-kv",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("derived-rendering relation target: %w", err)
	}
	source := r.workerVersionOf("rendering-source-version", worker.Name, fetchHandler)
	source.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(kv, kv.Name),
	}}
	bound, _, err := r.applyResource(source, applyOptions{
		Create: true, IdempotencyKey: "key-rendering-source",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("derived-rendering relation source: %w", err)
	}
	removed, err := r.deleteResource(
		kv, relationTarget.Metadata.Revision, "key-rendering-kv-delete",
		map[string]string{ErrorProbeHeader: ProbeExternalChange},
	)
	if err != nil {
		return err
	}
	if removed.Status != http.StatusNoContent {
		return fmt.Errorf(
			"out-of-band target delete HTTP %d, want 204; body=%s",
			removed.Status, strings.TrimSpace(string(removed.Body)),
		)
	}
	drifted, driftedResponse, err := r.readRawResponse(source)
	if err != nil {
		return err
	}
	if err := requireNotReady(drifted, "DependencyMissing"); err != nil {
		return fmt.Errorf("source of a vanished target: %w", err)
	}
	if err := requireRevisionAdvanced(
		bound, drifted, driftedResponse,
		"an out-of-band delete of the target a live relation pins",
	); err != nil {
		return err
	}
	r.revisionTransitions = append(r.revisionTransitions, "relation-drift:"+drifted.Metadata.Revision)
	r.complete("dependent-revision-advances-with-rendering")
	return nil
}
