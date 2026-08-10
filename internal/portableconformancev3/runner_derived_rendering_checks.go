package portableconformancev3

// runner_derived_rendering_checks.go carries the black-box evidence for the
// derived-rendering revision rule of spec/host-api/v1beta1.md: when a host
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
		kv, relationTarget.Metadata.Generation, "key-rendering-kv-delete",
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

// checkDeleteFenceSurvivesDerivedRendering proves the other half of the rule
// above: the revision it advances MUST NOT be what a delete is fenced on.
//
// The two rules meet in an ordinary teardown, and before this check nothing in
// the lane drove them together. Removing an aggregate means removing the
// dependents first, and removing a `WorkerDeployment` re-renders the
// `ModuleWorker` whose readiness follows it — so by the time the worker's own
// delete is issued, the worker's revision has moved. It moved BECAUSE OF THIS
// CLIENT'S OWN TEARDOWN, after the plan that computed the teardown read the
// worker. A host that fenced the delete on the representation refuses it, and
// the author is told their destroy is stale by a host reporting a change the
// destroy itself caused. Nothing else can be done about it either: re-reading
// and retrying would waive the fence rather than satisfy it, and the next
// dependent moves the revision again.
//
// So the delete fences on the DESIRED GENERATION, which moves only when a
// desired spec changes and therefore only when some client actually asked for
// something (spec/decisions/0011). The teardown below carries the generation the
// worker had before any of it started, and it must be honored at the end.
//
// The revision fence is proven to be genuinely stale first, rather than assumed:
// the same delete carrying `If-Match` on the revision the client read is refused
// `revision_conflict`. That single exchange establishes both halves — the
// representation did move, and a host still honors a client that asks about it —
// so the success that follows cannot be a host that simply stopped fencing.
func (r *v3Runner) checkDeleteFenceSurvivesDerivedRendering() error {
	input := r.contract.RunnerInput
	worker := r.target(input.ModuleWorker)
	worker.Name = "teardown-worker"
	if _, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-teardown-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the teardown worker: %w", err)
	}
	version := r.workerVersionOf("teardown-version", worker.Name, fetchHandler)
	versionCreated, _, err := r.applyResource(version, applyOptions{
		Create: true, IdempotencyKey: "key-teardown-version",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the teardown version: %w", err)
	}
	deployment := r.deploymentOf("teardown-deployment", worker.Name, version.Name)
	deploymentCreated, _, err := r.applyResource(deployment, applyOptions{
		Create: true, IdempotencyKey: "key-teardown-deployment",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the teardown deployment: %w", err)
	}

	// What a refresh sees before the teardown starts: one serving worker, at one
	// generation and one revision. Every fence below is this reading, and nothing
	// re-reads the worker afterwards, because a client tearing down an aggregate
	// does not either.
	live, _, err := r.readRawResponse(worker)
	if err != nil {
		return err
	}
	if condition := readyCondition(live); condition.Status != "True" {
		return fmt.Errorf(
			"the teardown worker reports Ready=%s/%s before the teardown, want True",
			condition.Status, condition.Reason,
		)
	}

	// The teardown, in the order the refusals of decision 0016 impose.
	removedDeployment, err := r.deleteResource(
		deployment, deploymentCreated.Metadata.Generation, "key-teardown-deployment-delete", nil,
	)
	if err != nil {
		return err
	}
	if removedDeployment.Status != http.StatusNoContent {
		return fmt.Errorf(
			"deleting the teardown deployment HTTP %d, want 204; body=%s",
			removedDeployment.Status, strings.TrimSpace(string(removedDeployment.Body)),
		)
	}
	removedVersion, err := r.deleteResource(
		version, versionCreated.Metadata.Generation, "key-teardown-version-delete", nil,
	)
	if err != nil {
		return err
	}
	if removedVersion.Status != http.StatusNoContent {
		return fmt.Errorf(
			"deleting the teardown version HTTP %d, want 204; body=%s",
			removedVersion.Status, strings.TrimSpace(string(removedVersion.Body)),
		)
	}

	// The representation the plan read is genuinely gone: the deployment's
	// removal re-rendered the worker's readiness, so the revision beside it is
	// stale and a client that fenced on it is told so.
	fenced, err := r.deleteResource(worker, live.Metadata.Generation, "key-teardown-worker-stale",
		map[string]string{"If-Match": `"` + live.Metadata.Revision + `"`})
	if err != nil {
		return err
	}
	if err := r.expectStableError(fenced, "revision_conflict"); err != nil {
		return fmt.Errorf(
			"a worker delete fenced on the revision a refresh read before its own deployment was removed: %w", err,
		)
	}
	if _, _, err := r.read(worker); err != nil {
		return fmt.Errorf("the refused delete removed the worker anyway: %w", err)
	}

	// The same delete, fenced the way this lane fences a delete. The desired
	// spec of the worker never moved, so the generation a refresh read before
	// the teardown is still current, and the teardown completes.
	removedWorker, err := r.deleteResource(worker, live.Metadata.Generation, "key-teardown-worker-delete", nil)
	if err != nil {
		return err
	}
	if removedWorker.Status != http.StatusNoContent {
		return fmt.Errorf(
			"deleting the worker under the generation a refresh read before the teardown started: HTTP %d, "+
				"want 204; body=%s. The revision moved because this teardown removed the deployment that "+
				"rendered the worker's readiness, and a delete does not fence on it",
			removedWorker.Status, strings.TrimSpace(string(removedWorker.Body)),
		)
	}
	if err := r.expectResourceAbsent(worker); err != nil {
		return err
	}
	r.complete("delete-fence-survives-derived-rendering")
	return nil
}
