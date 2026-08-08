package portableconformancev3

// runner_worker_endpoint_checks.go carries the black-box evidence for the
// host-assigned endpoint of spec/decisions/0024 and for the declared-output
// contract of spec/decisions/0025.
//
// The endpoint checks all rest on one property of the corpus: a Worker
// Endpoint's desired state is the worker reference and nothing else, which the
// contract loader proves before the run starts. Every address the host returns
// is therefore an address the host chose, and a check that finds one has found
// something no configuration could have supplied.

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

// checkWorkerEndpointAddressIsHostAssigned proves the one thing this Form
// exists for: a worker becomes reachable over HTTPS without the author owning a
// domain, naming a hostname, or choosing anything but the worker.
//
// What it asserts is exactly what a portable author may rely on — that a value
// comes back, that it is HTTPS, and that the two published members agree — and
// deliberately nothing about the SHAPE of the address. Asserting a suffix, a
// label length, or a subdomain layout here would write one host's private
// naming into the lane, and every other host would fail a check about a
// decision the contract leaves to it.
//
// A host that cannot assign an address at all must refuse the endpoint with
// `unsupported_capability` (422). That branch is not driven here, because a
// black-box runner cannot take a capability away from the host under test; it
// is stated normatively and listed among the obligations the lane does not
// prove (spec/host-api/v1alpha3.md). What this check does close is the failure
// that branch exists to prevent: a host that accepts the endpoint and then
// answers with an address it did not assign, an incomplete one, or a plaintext
// one fails here rather than being discovered by whoever tried the URL.
func (r *v3Runner) checkWorkerEndpointAddressIsHostAssigned() error {
	input := r.contract.RunnerInput
	endpoint := r.target(input.WorkerEndpoint)
	created, _, err := r.applyResource(endpoint, applyOptions{
		Create: true, IdempotencyKey: "key-worker-endpoint",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("a WorkerEndpoint on a worker whose deployment serves fetch: %w", err)
	}
	// The stored desired state must still be only the worker: a host that
	// answered the address by writing it into the spec would have turned a
	// host decision into desired state the next plan compares against.
	if len(created.Spec) != 1 || nestedName(created.Spec, "worker") != input.ModuleWorker.Name {
		return fmt.Errorf(
			"the host rewrote the endpoint's desired state to %v; the assigned address is status, never spec",
			created.Spec,
		)
	}
	hostname, url, err := endpointAddress(created, "the created WorkerEndpoint")
	if err != nil {
		return err
	}
	// Nothing in the request could have produced this address. The corpus
	// desired state carries no hostname at all, so the only way the runner could
	// predict it is if the host had derived it from the resource name — which is
	// exactly the "assigned" claim being false.
	if strings.Contains(hostname, endpoint.Name) {
		return fmt.Errorf(
			"assigned hostname %q is the resource name %q with decoration; a host-assigned address is the host's to choose, not a rendering of what the author wrote",
			hostname, endpoint.Name,
		)
	}
	// A read answers the same address. An endpoint whose address moved between
	// the apply response and the next read would be unusable: every consumer of
	// the URL would be one refresh away from a different origin.
	after, _, err := r.read(endpoint)
	if err != nil {
		return err
	}
	readHostname, readURL, err := endpointAddress(after, "the WorkerEndpoint on read")
	if err != nil {
		return err
	}
	if readHostname != hostname || readURL != url {
		return fmt.Errorf(
			"the endpoint address changed between apply (%s) and read (%s) without any mutation",
			url, readURL,
		)
	}
	r.complete("worker-endpoint-address-is-host-assigned")
	return nil
}

// checkWorkerEndpointSinglePerWorker proves the cardinality rule.
//
// It is host-enforced because it is unstatable anywhere else: one endpoint's
// desired document says nothing about any other, so no schema keyword and no
// client-side rule can see the second one. A laxer host would accept this and
// leave one worker with two addresses and nothing saying which is canonical.
func (r *v3Runner) checkWorkerEndpointSinglePerWorker() error {
	input := r.contract.RunnerInput
	second := r.target(input.WorkerEndpoint)
	second.Name = "second-endpoint"
	if err := r.refuseCreate(
		second, "invalid_argument", "key-second-worker-endpoint",
		"a second WorkerEndpoint of one worker",
	); err != nil {
		return err
	}
	r.complete("worker-endpoint-single-per-worker")
	return nil
}

// checkWorkerEndpointFollowsTheActiveDeployment proves the endpoint invokes
// what the DEPLOYMENT selects, in the two ways a desired-state runner can
// observe it.
//
// First, the endpoint is a dependent of the deployment: deleting the deployment
// while the endpoint lives is refused, because removing what serves an address
// would leave the address answering nothing. Second — and this is the claim the
// Form actually makes — moving traffic to a different version leaves the
// assigned address untouched and the endpoint Ready, with no apply against the
// endpoint at all. A host that re-minted the address on promotion would break
// every consumer holding the URL on a change the author made to something else.
func (r *v3Runner) checkWorkerEndpointFollowsTheActiveDeployment() error {
	input := r.contract.RunnerInput
	worker := r.target(input.ModuleWorker)
	worker.Name = "endpoint-worker"
	if _, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-endpoint-worker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("endpoint worker: %w", err)
	}
	first := r.workerVersionOf("endpoint-first-version", worker.Name, fetchHandler)
	if _, _, err := r.applyResource(first, applyOptions{
		Create: true, IdempotencyKey: "key-endpoint-first-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("endpoint first version: %w", err)
	}
	deployment := r.deploymentOf("endpoint-deployment", worker.Name, first.Name)
	created, _, err := r.applyResource(deployment, applyOptions{
		Create: true, IdempotencyKey: "key-endpoint-deployment",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("endpoint deployment: %w", err)
	}

	endpoint := r.target(input.WorkerEndpoint)
	endpoint.Name = "promotion-endpoint"
	endpoint.Spec = cloneJSONMap(endpoint.Spec)
	endpoint.Spec["worker"] = exactReference(r.target(input.ModuleWorker), worker.Name)
	applied, _, err := r.applyResource(endpoint, applyOptions{
		Create: true, IdempotencyKey: "key-promotion-endpoint",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("endpoint on its own worker: %w", err)
	}
	before, _, err := endpointAddress(applied, "the endpoint before promotion")
	if err != nil {
		return err
	}

	// Deleting what serves the address is refused while the address is live.
	current, _, err := r.read(deployment)
	if err != nil {
		return err
	}
	blocked, err := r.deleteResource(deployment, current.Metadata.Revision, "key-endpoint-deployment-blocked", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(blocked, "dependency_in_use"); err != nil {
		return fmt.Errorf("deleting the deployment a live WorkerEndpoint is served by: %w", err)
	}

	// Promotion: all traffic moves to a second version, and the endpoint is not
	// touched by the author at any point.
	second := r.workerVersionOf("endpoint-second-version", worker.Name, fetchHandler)
	if _, _, err := r.applyResource(second, applyOptions{
		Create: true, IdempotencyKey: "key-endpoint-second-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("endpoint second version: %w", err)
	}
	promoted := r.deploymentOf("endpoint-deployment", worker.Name, second.Name)
	if _, _, err := r.applyResource(promoted, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-endpoint-promotion",
	}, http.StatusOK); err != nil {
		return fmt.Errorf("promoting the endpoint's worker to a second version: %w", err)
	}
	after, _, err := r.read(endpoint)
	if err != nil {
		return fmt.Errorf("reading the endpoint after promotion: %w", err)
	}
	promotedAddress, _, err := endpointAddress(after, "the endpoint after promotion")
	if err != nil {
		return err
	}
	if promotedAddress != before {
		return fmt.Errorf(
			"promotion moved the assigned address %s -> %s; the endpoint follows the active deployment, it does not follow the version",
			before, promotedAddress,
		)
	}
	if condition := readyCondition(after); condition.Status != "True" {
		return fmt.Errorf(
			"the endpoint reports Ready=%s/%s after its worker was promoted; nothing about it changed",
			condition.Status, condition.Reason,
		)
	}

	// Both endpoints are released here, at the end of the endpoint group: a live
	// endpoint is a dependent of the deployment that serves it, so leaving
	// either one behind would block a later check for a reason that has nothing
	// to do with what that check proves. The corpus probe belongs to this group
	// even though it was created by the first check — the group is what created
	// it, and the group is what cleans up.
	for _, live := range []probeTarget{endpoint, r.target(input.WorkerEndpoint)} {
		if err := r.deleteExisting(live, "key-endpoint-release-"+live.Name); err != nil {
			return err
		}
	}
	r.complete("worker-endpoint-follows-the-active-deployment")
	return nil
}

// checkFormDeclaredOutputsAreExact proves the published wire rule that nothing
// proved before: `status.outputs` is present exactly for a Form whose
// Definition declares an outputSchema, carries exactly that schema's members,
// and is OMITTED for every other Form
// (spec/schemas/host-api-wire-v1alpha3.schema.json).
//
// Both halves matter and they fail different hosts. A host that returned an
// empty outputs object everywhere would pass a "the outputs I declared are
// there" check and would still be publishing a document its Forms never
// described. A host that returned an extra member would be handing a client a
// value the contract cannot type, which is exactly the untyped blob the typed
// attribute surface exists to replace (spec/decisions/0025).
//
// The member set comes from the corpus rather than from the host, because the
// published form-definition response carries no outputSchema and its bytes are
// immutable: asking the host what it declared and then checking that it
// returned it would be asking the host to grade itself.
func (r *v3Runner) checkFormDeclaredOutputsAreExact() error {
	input := r.contract.RunnerInput
	// The probes that are live at this point in the ordered matrix: the four
	// identities and revisions the run keeps for its whole length, the
	// deployment that serves them, and the endpoint the checks above created.
	// The three other attachments are created and released by later checks, so
	// reading them here would assert against resources the matrix has not made
	// yet — a check that fails on ORDER rather than on behavior.
	probes := []ResourceProbe{
		input.ModuleWorker, input.EdgeKvNamespace, input.AtLeastOnceQueue,
		input.WorkerBundle.ResourceProbe, input.WorkerDeployment, input.WorkerEndpoint,
	}
	declaring := 0
	omitting := 0
	for _, probe := range probes {
		target := r.target(probe)
		resource, _, err := r.read(target)
		if err != nil {
			return fmt.Errorf("reading %s to compare its outputs: %w", probe.Name, err)
		}
		var returned []string
		if resource.Status != nil {
			for name := range resource.Status.Outputs {
				returned = append(returned, name)
			}
		}
		sort.Strings(returned)
		want := append([]string(nil), probe.DeclaredOutputs...)
		sort.Strings(want)
		if len(want) == 0 {
			if resource.Status != nil && resource.Status.Outputs != nil {
				return fmt.Errorf(
					"%s declares no outputSchema, so status.outputs must be omitted; the host returned %v",
					target.Ref.Kind, returned,
				)
			}
			omitting++
			continue
		}
		declaring++
		if !reflect.DeepEqual(returned, want) {
			return fmt.Errorf(
				"%s outputs = %v, want exactly the declared %v", target.Ref.Kind, returned, want,
			)
		}
		for _, name := range want {
			text, ok := resource.Status.Outputs[name].(string)
			if !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("%s output %q is not a non-empty string", target.Ref.Kind, name)
			}
		}
	}
	// Both sides have to have been exercised, or the check is vacuous in one
	// direction: with nothing declaring outputs it would pass a host that
	// publishes none at all, and with nothing omitting them it would pass a host
	// that publishes an outputs document for every Form it serves.
	if declaring == 0 || omitting == 0 {
		return fmt.Errorf(
			"the declared-output comparison covered %d Form(s) that declare outputs and %d that do not; it needs both",
			declaring, omitting,
		)
	}
	r.complete("form-declared-outputs-are-exact")
	return nil
}

// endpointAddress reads and holds one Worker Endpoint's published address to
// everything the Form fixes: both members present, the scheme https, the path
// root `/`, and the URL being exactly the hostname it reports. The last one is
// what stops a host from publishing two addresses under one resource.
func endpointAddress(resource wireResource, subject string) (hostname, address string, err error) {
	if resource.Status == nil || len(resource.Status.Outputs) == 0 {
		return "", "", fmt.Errorf("%s published no status.outputs; the assigned address is the Form's whole answer", subject)
	}
	hostname, _ = resource.Status.Outputs["hostname"].(string)
	address, _ = resource.Status.Outputs["url"].(string)
	if strings.TrimSpace(hostname) == "" {
		return "", "", fmt.Errorf("%s published no hostname", subject)
	}
	if want := "https://" + hostname + "/"; address != want {
		return "", "", fmt.Errorf(
			"%s published url %q; the Form fixes it as exactly %q — https, the assigned hostname, and the path root",
			subject, address, want,
		)
	}
	return hostname, address, nil
}
