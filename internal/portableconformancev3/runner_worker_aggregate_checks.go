package portableconformancev3

// runner_worker_aggregate_checks.go carries the black-box evidence for the
// Worker aggregate rules of spec/decisions/0016. Every check here fails a host
// that is laxer than the decision: each one drives a configuration a laxer host
// would accept, and then proves that nothing was stored.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// aggregateHostname renders one distinct hostname per custom-domain probe, so
// two attachments never collide on a name outside the rules under test.
func aggregateHostname(label string) string {
	return label + ".portable-conformance.invalid"
}

// workerVersionOf builds one Worker Version target of a named worker, with no
// typed binding: the declared default of every binding list is the empty list,
// so stating it needs no binding target to exist.
func (r *v3Runner) workerVersionOf(name, worker string, handlers ...string) probeTarget {
	version := r.target(r.contract.RunnerInput.WorkerVersion)
	version.Name = name
	version.Spec = cloneJSONMap(version.Spec)
	version.Spec["worker"] = exactReference(r.target(r.contract.RunnerInput.ModuleWorker), worker)
	version.Spec["kvBindings"] = []any{}
	declared := make([]any, 0, len(handlers))
	for _, handler := range handlers {
		declared = append(declared, handler)
	}
	version.Spec["handlers"] = declared
	return version
}

// deploymentOf builds one Worker Deployment target sending all traffic to one
// named version of one named worker.
func (r *v3Runner) deploymentOf(name, worker string, versions ...string) probeTarget {
	deployment := r.target(r.contract.RunnerInput.WorkerDeployment)
	deployment.Name = name
	deployment.Spec = cloneJSONMap(deployment.Spec)
	deployment.Spec["worker"] = exactReference(r.target(r.contract.RunnerInput.ModuleWorker), worker)
	// Distinct weights on purpose. `uniqueItems` already rejects two identical
	// entries, so a duplicate-version probe has to split one version across two
	// DIFFERENT shares — which is exactly the shape a schema cannot catch and
	// the host must.
	entries := make([]any, 0, len(versions))
	remaining := 10000
	for index, version := range versions {
		share := 1000 * (index + 1)
		if index == len(versions)-1 {
			share = remaining
		}
		remaining -= share
		entries = append(entries, map[string]any{
			"workerVersion": exactReference(r.target(r.contract.RunnerInput.WorkerVersion), version),
			"weight":        share,
		})
	}
	deployment.Spec["versions"] = entries
	return deployment
}

// attachmentsOf builds the three inward-activation attachments of one worker.
func (r *v3Runner) attachmentsOf(prefix, worker string) []probeTarget {
	input := r.contract.RunnerInput
	moduleWorker := r.target(input.ModuleWorker)

	domain := r.target(input.WorkerCustomDomain)
	domain.Name = prefix + "-domain"
	domain.Spec = cloneJSONMap(domain.Spec)
	domain.Spec["worker"] = exactReference(moduleWorker, worker)
	domain.Spec["hostname"] = aggregateHostname(prefix)

	cron := r.target(input.WorkerCronTrigger)
	cron.Name = prefix + "-cron"
	cron.Spec = cloneJSONMap(cron.Spec)
	cron.Spec["worker"] = exactReference(moduleWorker, worker)

	consumer := r.target(input.QueueConsumer)
	consumer.Name = prefix + "-consumer"
	consumer.Spec = cloneJSONMap(consumer.Spec)
	consumer.Spec["worker"] = exactReference(moduleWorker, worker)

	return []probeTarget{domain, cron, consumer}
}

// refuseCreate drives one create the host must refuse before any mutation and
// proves the resource is still absent.
func (r *v3Runner) refuseCreate(target probeTarget, code, key, subject string) error {
	response, err := r.apply(target, applyOptions{Create: true, IdempotencyKey: key})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, code); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	if err := r.expectResourceAbsent(target); err != nil {
		return fmt.Errorf("%s mutated state: %w", subject, err)
	}
	return nil
}

// refuseUpdate drives one generation-fenced update the host must refuse before
// any mutation and proves the stored identity did not move.
func (r *v3Runner) refuseUpdate(target probeTarget, generation, code, key, subject string) error {
	response, err := r.apply(target, applyOptions{
		ExpectedGeneration: generation, IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, code); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	after, _, err := r.read(target)
	if err != nil {
		return fmt.Errorf("%s left the resource unreadable: %w", subject, err)
	}
	if after.Metadata.Generation != generation {
		return fmt.Errorf(
			"%s moved generation %s -> %s despite being refused",
			subject, generation, after.Metadata.Generation,
		)
	}
	return nil
}

// deleteExisting removes one resource at its current revision.
func (r *v3Runner) deleteExisting(target probeTarget, key string) error {
	current, _, err := r.read(target)
	if err != nil {
		return err
	}
	response, err := r.deleteResource(target, current.Metadata.Revision, key, nil)
	if err != nil {
		return err
	}
	if response.Status != http.StatusNoContent {
		return fmt.Errorf(
			"deleting %s HTTP %d, want 204; body=%s",
			target.Name, response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	return nil
}

// checkAttachmentRequiresActiveDeployment proves the attachment gate reads the
// worker's ACTIVE DEPLOYMENT, not any stored version.
//
// At this point the worker already has stored Worker Versions declaring every
// handler the three attachments invoke, and no deployment. A host that gated on
// "some stored version declares the handler" — the rule this lane shipped
// before decision 0016 — would admit all three here, activating a hostname, a
// schedule, and a queue consumer against code nothing runs. The same state also
// pins the worker's own readiness: it exists and serves nothing.
func (r *v3Runner) checkAttachmentRequiresActiveDeployment() error {
	input := r.contract.RunnerInput
	worker := r.target(input.ModuleWorker)
	before, err := r.readRaw(worker)
	if err != nil {
		return err
	}
	if err := requireNotReady(before, "Provisioning"); err != nil {
		return fmt.Errorf("a worker with no deployment: %w", err)
	}
	if hostReason := readyCondition(before).HostReason; !strings.Contains(hostReason, worker.Name) {
		return fmt.Errorf("Ready=False/Provisioning hostReason %q must name the worker", hostReason)
	}

	// The corpus attachment probes themselves: the exact documents a later step
	// applies successfully once a deployment serves them.
	probes := []probeTarget{
		r.target(input.WorkerCustomDomain),
		r.target(input.WorkerCronTrigger),
		r.target(input.QueueConsumer),
	}
	for _, attachment := range probes {
		if err := r.refuseCreate(
			attachment, "unsupported_capability",
			"key-nodeploy-"+attachment.Ref.Kind,
			attachment.Ref.Kind+" of a worker with no active deployment",
		); err != nil {
			return err
		}
	}

	// An inbound module-worker.service binding is refused at BIND time for the
	// same reason: worker.service is provided by the identity, and the identity
	// projects nothing until its deployment serves fetch.
	callerWorker := worker
	callerWorker.Name = "service-caller-worker"
	if _, _, err := r.applyResource(callerWorker, applyOptions{
		Create: true, IdempotencyKey: "key-service-caller-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("service-binding caller worker: %w", err)
	}
	caller := r.workerVersionOf("service-caller-version", callerWorker.Name, fetchHandler)
	caller.Spec["serviceBindings"] = []any{map[string]any{
		"name":     "UPSTREAM",
		"resource": exactReference(worker, worker.Name),
	}}
	if err := r.refuseCreate(
		caller, "unsupported_capability", "key-service-binding-undeployed",
		"a module-worker.service binding to a worker that serves nothing",
	); err != nil {
		return err
	}
	r.complete("attachment-requires-active-deployment")
	return nil
}

// checkDeploymentIntegrity proves the three rules that make one deployment the
// coherent answer to "what serves this worker": exactly one deployment per
// worker, a weighted set that belongs to that worker, and each version named
// once.
//
// The duplicate case matters because a schema cannot catch it: `uniqueItems`
// rejects two identical entries, so an author splits one version across two
// weights and the array is still valid while the deployment says two different
// things about one revision.
func (r *v3Runner) checkDeploymentIntegrity() error {
	input := r.contract.RunnerInput
	worker := r.target(input.ModuleWorker)

	second := r.deploymentOf("second-deployment", worker.Name, input.WorkerVersion.Name)
	if err := r.refuseCreate(
		second, "invalid_argument", "key-second-deployment",
		"a second WorkerDeployment of one worker",
	); err != nil {
		return err
	}
	r.complete("deployment-single-active-per-worker")

	// A separate worker with its own version, so ownership and duplication can
	// be driven without colliding with the single-deployment rule above.
	other := worker
	other.Name = "other-worker"
	if _, _, err := r.applyResource(other, applyOptions{
		Create: true, IdempotencyKey: "key-other-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("second worker: %w", err)
	}
	otherVersion := r.workerVersionOf("other-version", other.Name, fetchHandler)
	if _, _, err := r.applyResource(otherVersion, applyOptions{
		Create: true, IdempotencyKey: "key-other-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("second worker version: %w", err)
	}

	foreign := r.deploymentOf("ownership-deployment", other.Name, input.WorkerVersion.Name)
	if err := r.refuseCreate(
		foreign, "invalid_argument", "key-ownership-deployment",
		"a deployment weighting a version of a different worker",
	); err != nil {
		return err
	}
	r.complete("deployment-version-ownership")

	duplicate := r.deploymentOf("duplicate-deployment", other.Name, otherVersion.Name, otherVersion.Name)
	if err := r.refuseCreate(
		duplicate, "invalid_argument", "key-duplicate-deployment",
		"a deployment weighting one version twice",
	); err != nil {
		return err
	}
	r.complete("deployment-version-duplicate-rejected")
	return nil
}

// checkHandlerGatedAttachments proves the handler half of the attachment gate
// on a worker that DOES have an active deployment: every version that
// deployment weights must export the handler the attachment invokes.
//
// Every, not some. A request served by any weighted version has to find the
// handler, so a deployment that serves fetch answers a custom domain and
// refuses a cron trigger, and re-weighting to a version that exports all three
// is what opens the remaining gates.
func (r *v3Runner) checkHandlerGatedAttachments() error {
	worker := r.target(r.contract.RunnerInput.ModuleWorker)
	worker.Name = "gate-worker"
	if _, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-gate-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("handler-gate worker: %w", err)
	}
	fetchOnly := r.workerVersionOf("gate-fetch-version", worker.Name, fetchHandler)
	if _, _, err := r.applyResource(fetchOnly, applyOptions{
		Create: true, IdempotencyKey: "key-gate-fetch-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("handler-gate fetch version: %w", err)
	}
	deployment := r.deploymentOf("gate-deployment", worker.Name, fetchOnly.Name)
	created, _, err := r.applyResource(deployment, applyOptions{
		Create: true, IdempotencyKey: "key-gate-deployment",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("handler-gate deployment: %w", err)
	}

	attachments := r.attachmentsOf("gate", worker.Name)
	domain, cron, consumer := attachments[0], attachments[1], attachments[2]
	if _, _, err := r.applyResource(domain, applyOptions{
		Create: true, IdempotencyKey: "key-gate-domain",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a custom domain on a deployment that serves fetch: %w", err)
	}
	for _, ungated := range []probeTarget{cron, consumer} {
		if err := r.refuseCreate(
			ungated, "unsupported_capability", "key-ungated-"+ungated.Ref.Kind,
			ungated.Ref.Kind+" whose handler the active deployment does not serve",
		); err != nil {
			return err
		}
	}

	// Re-weighting to a version that exports every handler opens both gates and
	// keeps the live custom domain served.
	full := r.workerVersionOf("gate-full-version", worker.Name, fetchHandler, scheduledHandler, queueHandler)
	if _, _, err := r.applyResource(full, applyOptions{
		Create: true, IdempotencyKey: "key-gate-full-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("handler-gate full version: %w", err)
	}
	moved := r.deploymentOf("gate-deployment", worker.Name, full.Name)
	if _, _, err := r.applyResource(moved, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-gate-deployment-move",
	}, http.StatusOK); err != nil {
		return fmt.Errorf("re-weighting to a version that exports every handler: %w", err)
	}
	for _, gated := range []probeTarget{cron, consumer} {
		if _, _, err := r.applyResource(gated, applyOptions{
			Create: true, IdempotencyKey: "key-gated-" + gated.Ref.Kind,
		}, http.StatusCreated); err != nil {
			return fmt.Errorf("%s once the deployment serves its handler: %w", gated.Ref.Kind, err)
		}
	}
	r.complete("handler-gated-attachments")
	return nil
}

// checkBindingNameCollision proves the environment namespace is single.
//
// `vars` keys, `requiredSensitiveVars` entries, and every binding `name` are
// projected into the ONE environment object the module receives. A schema
// cannot see the collision: `uniqueItems` rejects a duplicated whole object,
// never two objects agreeing only on `name`, and it has no reach across sibling
// properties at all.
func (r *v3Runner) checkBindingNameCollision() error {
	input := r.contract.RunnerInput
	kv := r.target(input.EdgeKvNamespace)
	binding := []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(kv, kv.Name),
	}}

	collisions := []struct {
		key      string
		subject  string
		mutation func(spec map[string]any)
	}{
		{
			key:     "key-collision-vars",
			subject: "a vars key that collides with a binding name",
			mutation: func(spec map[string]any) {
				spec["vars"] = map[string]any{"CACHE": "https://cache.invalid"}
			},
		},
		{
			key:     "key-collision-sensitive",
			subject: "a requiredSensitiveVars entry that collides with a binding name",
			mutation: func(spec map[string]any) {
				spec["requiredSensitiveVars"] = []any{"CACHE"}
			},
		},
	}
	for _, collision := range collisions {
		offender := r.workerVersionOf("collision-version", input.ModuleWorker.Name, fetchHandler)
		offender.Spec["kvBindings"] = binding
		collision.mutation(offender.Spec)
		if err := r.refuseCreate(offender, "invalid_argument", collision.key, collision.subject); err != nil {
			return err
		}
	}

	// The same declarations under distinct names are one valid environment.
	accepted := r.workerVersionOf("collision-version", input.ModuleWorker.Name, fetchHandler)
	accepted.Spec["kvBindings"] = binding
	accepted.Spec["vars"] = map[string]any{"CACHE_URL": "https://cache.invalid"}
	accepted.Spec["requiredSensitiveVars"] = []any{"CACHE_TOKEN_NAME"}
	if _, _, err := r.applyResource(accepted, applyOptions{
		Create: true, IdempotencyKey: "key-collision-free",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a collision-free environment namespace: %w", err)
	}
	// Released again so the shared namespace it binds stays deletable by the
	// later relation checks.
	if err := r.deleteExisting(accepted, "key-collision-free-delete"); err != nil {
		return err
	}
	r.complete("binding-name-collision-rejected")
	return nil
}

// checkDeploymentChangePreservesDependents proves the reverse direction of the
// attachment gate: moving traffic must not leave a live dependent unserved.
//
// Without it the gate is one-way — a cron trigger could be admitted against a
// deployment that serves `scheduled`, and the next deployment could quietly
// move all traffic to a version that exports no `scheduled` at all. The four
// dependents are the three attachments and one INBOUND module-worker.service
// binding, which is a dependent because `worker.service` is provided by the
// worker identity and answered by whatever its deployment selects.
func (r *v3Runner) checkDeploymentChangePreservesDependents() error {
	input := r.contract.RunnerInput
	worker := r.target(input.ModuleWorker)
	worker.Name = "dependents-worker"
	if _, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-dependents-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("dependents worker: %w", err)
	}
	full := r.workerVersionOf(
		"dependents-full-version", worker.Name, fetchHandler, scheduledHandler, queueHandler,
	)
	if _, _, err := r.applyResource(full, applyOptions{
		Create: true, IdempotencyKey: "key-dependents-full-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("dependents full version: %w", err)
	}
	deployment := r.deploymentOf("dependents-deployment", worker.Name, full.Name)
	created, _, err := r.applyResource(deployment, applyOptions{
		Create: true, IdempotencyKey: "key-dependents-deployment",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("dependents deployment: %w", err)
	}
	generation := created.Metadata.Generation

	attachments := r.attachmentsOf("dependents", worker.Name)
	for _, attachment := range attachments {
		if _, _, err := r.applyResource(attachment, applyOptions{
			Create: true, IdempotencyKey: "key-dependents-" + attachment.Ref.Kind,
		}, http.StatusCreated); err != nil {
			return fmt.Errorf("dependent %s: %w", attachment.Ref.Kind, err)
		}
	}
	caller := r.workerVersionOf("dependents-caller-version", input.ModuleWorker.Name, fetchHandler)
	caller.Spec["serviceBindings"] = []any{map[string]any{
		"name":     "UPSTREAM",
		"resource": exactReference(worker, worker.Name),
	}}
	if _, _, err := r.applyResource(caller, applyOptions{
		Create: true, IdempotencyKey: "key-dependents-caller",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("inbound service binding to a serving worker: %w", err)
	}

	fetchOnly := r.workerVersionOf("dependents-fetch-version", worker.Name, fetchHandler)
	if _, _, err := r.applyResource(fetchOnly, applyOptions{
		Create: true, IdempotencyKey: "key-dependents-fetch-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("dependents fetch-only version: %w", err)
	}
	noFetch := r.workerVersionOf("dependents-nofetch-version", worker.Name, scheduledHandler, queueHandler)
	if _, _, err := r.applyResource(noFetch, applyOptions{
		Create: true, IdempotencyKey: "key-dependents-nofetch-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("dependents no-fetch version: %w", err)
	}

	// Dropping scheduled and queue breaks the cron trigger and the consumer.
	toFetchOnly := r.deploymentOf("dependents-deployment", worker.Name, fetchOnly.Name)
	if err := r.refuseUpdate(
		toFetchOnly, generation, "unsupported_capability", "key-dependents-drop-scheduled",
		"moving traffic to a version that exports no scheduled handler",
	); err != nil {
		return err
	}
	// Dropping fetch breaks the custom domain and the inbound service binding.
	toNoFetch := r.deploymentOf("dependents-deployment", worker.Name, noFetch.Name)
	if err := r.refuseUpdate(
		toNoFetch, generation, "unsupported_capability", "key-dependents-drop-fetch",
		"moving traffic to a version that exports no fetch handler",
	); err != nil {
		return err
	}

	// With every attachment removed the inbound service binding is the ONLY
	// remaining dependent, and it alone still refuses the same change.
	for _, attachment := range attachments {
		if err := r.deleteExisting(attachment, "key-dependents-release-"+attachment.Ref.Kind); err != nil {
			return err
		}
	}
	if err := r.refuseUpdate(
		toNoFetch, generation, "unsupported_capability", "key-dependents-drop-fetch-binding",
		"moving traffic away from fetch while an inbound service binding is live",
	); err != nil {
		return err
	}

	// Removing that binding is what makes the change representable.
	if err := r.deleteExisting(caller, "key-dependents-release-caller"); err != nil {
		return err
	}
	if _, _, err := r.applyResource(toNoFetch, applyOptions{
		ExpectedGeneration: generation, IdempotencyKey: "key-dependents-drop-fetch-accepted",
	}, http.StatusOK); err != nil {
		return fmt.Errorf("the same change once no dependent needs fetch: %w", err)
	}
	r.complete("deployment-change-preserves-dependents")
	return nil
}

// checkDeploymentDeleteBlockedByDependent proves deletion is refused while a
// dependent needs what the deployment serves.
//
// Failing closed is the decision (spec/decisions/0016): the alternative is one
// delete fanning out into a set of silently broken attachments whose repair
// order the author has to reconstruct. The refusal is `dependency_in_use`, the
// same closed outcome that protects a referenced resource, because it is the
// same statement — something live still needs this.
func (r *v3Runner) checkDeploymentDeleteBlockedByDependent() error {
	input := r.contract.RunnerInput
	deployment := r.target(input.WorkerDeployment)
	attachments := []probeTarget{
		r.target(input.WorkerCustomDomain),
		r.target(input.WorkerCronTrigger),
		r.target(input.QueueConsumer),
	}
	for _, attachment := range attachments {
		if _, _, err := r.applyResource(attachment, applyOptions{
			Create: true, IdempotencyKey: "key-blocking-" + attachment.Ref.Kind,
		}, http.StatusCreated); err != nil {
			return fmt.Errorf("%s on the serving deployment: %w", attachment.Ref.Kind, err)
		}
		current, _, err := r.read(deployment)
		if err != nil {
			return err
		}
		blocked, err := r.deleteResource(
			deployment, current.Metadata.Revision,
			"key-deployment-blocked-"+attachment.Ref.Kind, nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(blocked, "dependency_in_use"); err != nil {
			return fmt.Errorf("deleting a deployment a live %s needs: %w", attachment.Ref.Kind, err)
		}
		if _, _, err := r.read(deployment); err != nil {
			return fmt.Errorf("a refused deployment delete removed it: %w", err)
		}
		// Released again, so the next dependent is proved on its own and the
		// deployment stays available to the relation checks that follow.
		if err := r.deleteExisting(attachment, "key-blocking-release-"+attachment.Ref.Kind); err != nil {
			return err
		}
	}
	if len(attachments) == 0 {
		return errors.New("no attachment probes were available to block the deployment delete")
	}
	r.complete("deployment-delete-blocked-by-dependent")

	// The FOURTH dependent, on its own. Every attachment above was released, so
	// nothing but the inbound `module-worker.service` binding can block this
	// delete: a host that guards the three attachment Forms and forgets the
	// inbound edge passes every case above and fails only here, which is exactly
	// why the fourth is proved in isolation rather than alongside them.
	//
	// The caller is a SECOND worker's version, because a service binding names a
	// worker IDENTITY and is only representable while that worker serves fetch
	// (spec/decisions/0016) — which is the state the deployment under test
	// provides, and the state its deletion would take away.
	moduleWorker := r.target(input.ModuleWorker)
	callerWorker := moduleWorker
	callerWorker.Name = "inbound-delete-caller-worker"
	if _, _, err := r.applyResource(callerWorker, applyOptions{
		Create: true, IdempotencyKey: "key-inbound-delete-caller-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("inbound-binding caller worker: %w", err)
	}
	caller := r.workerVersionOf("inbound-delete-caller-version", callerWorker.Name, fetchHandler)
	caller.Spec["serviceBindings"] = []any{map[string]any{
		"name":     "UPSTREAM",
		"resource": exactReference(moduleWorker, moduleWorker.Name),
	}}
	if _, _, err := r.applyResource(caller, applyOptions{
		Create: true, IdempotencyKey: "key-inbound-delete-caller",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("inbound service binding to the serving worker: %w", err)
	}
	current, _, err := r.read(deployment)
	if err != nil {
		return err
	}
	blocked, err := r.deleteResource(
		deployment, current.Metadata.Revision, "key-deployment-blocked-inbound-binding", nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(blocked, "dependency_in_use"); err != nil {
		return fmt.Errorf("deleting a deployment a live inbound service binding needs: %w", err)
	}
	if _, _, err := r.read(deployment); err != nil {
		return fmt.Errorf("a refused deployment delete removed it: %w", err)
	}
	// Released again, so the deployment stays deletable by the relation checks
	// that follow.
	if err := r.deleteExisting(caller, "key-inbound-delete-caller-release"); err != nil {
		return err
	}
	r.complete("deployment-delete-blocked-by-inbound-binding")
	return nil
}
