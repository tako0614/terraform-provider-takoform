package portableconformancev3

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// runner_runtime_abi_checks.go proves the two facts that make the ES Module
// Worker ABI a contract rather than a claim (spec/decisions/0019).
//
// Before this, `ModuleWorker` fixed "the ES Module Worker ABI" by identity while
// nothing said what that ABI was, and `WorkerVersion` carried a compatibility
// date that could only mean something against a behavior registry this project
// does not have. Two conforming hosts could therefore read the same document
// and run observably different runtimes — the incompleteness the boundary rule
// forbids.
//
// What a black-box runner can hold a host to is exactly two things: that the
// host advertises the runtime contract at the EXACT digest the lane pins, and
// that it refuses a Worker Version declaring a handler that contract does not
// define.

// checkRuntimeContractAdvertised proves the host implements one exact runtime
// ABI and says which one.
//
// The interface support profile must return the pinned schemaDigest, not merely
// a well-formed one: a digest that differs is a different contract, so the host
// is answering about a runtime the lane did not ask for. The ModuleWorker Form
// Definition carries the same contract in its providedInterfaces, and its own
// schemaDigest covers those bytes, so serving a definition that omits or moves
// the runtime ref already fails `form-definition-exact` at the exact FormRef the
// corpus pins — the two checks close the loop from opposite ends.
func (r *v3Runner) checkRuntimeContractAdvertised() error {
	runtime := r.contract.RunnerInput.SupportProbes.RuntimeContract
	response, err := r.request(
		http.MethodGet,
		fmt.Sprintf("%s/support/interfaces/%s/%s", r.apiBase,
			url.PathEscape(runtime.Name), url.PathEscape(runtime.Version)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf(
			"the host does not advertise the %s@%s runtime contract: HTTP %d",
			runtime.Name, runtime.Version, response.Status,
		)
	}
	var profile map[string]any
	if err := decodeStrictResponse(response, &profile); err != nil {
		return err
	}
	if profile["apiVersion"] != supportAPIVersion || profile["kind"] != "InterfaceSupport" {
		return errors.New("runtime contract support profile identity is invalid")
	}
	reference, _ := profile["interfaceRef"].(map[string]any)
	if reference == nil {
		return errors.New("runtime contract support profile omitted interfaceRef")
	}
	if reference["name"] != runtime.Name || reference["version"] != runtime.Version {
		return fmt.Errorf("runtime contract support profile names %v/%v", reference["name"], reference["version"])
	}
	digest, _ := reference["schemaDigest"].(string)
	if !formpackage.ValidDigest(digest) {
		return errors.New("runtime contract schemaDigest is not a canonical digest")
	}
	if digest != runtime.SchemaDigest {
		return fmt.Errorf(
			"the host advertises %s@%s at %s; the lane requires the exact contract %s",
			runtime.Name, runtime.Version, digest, runtime.SchemaDigest,
		)
	}
	// The worker identity is what a host implements, so the lane also requires
	// its exact Form line to be supported: a host advertising the ABI while
	// supporting no ModuleWorker implements it for nobody.
	worker := r.contract.RunnerInput.ModuleWorker.Identity.FormRef
	workerResponse, err := r.request(
		http.MethodGet,
		fmt.Sprintf("%s/support/forms/%s/%s/%s", r.apiBase,
			groupSegments(worker.APIVersion), url.PathEscape(worker.Kind),
			url.PathEscape(worker.DefinitionVersion)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if workerResponse.Status != http.StatusOK {
		return fmt.Errorf("ModuleWorker support profile HTTP %d", workerResponse.Status)
	}
	var workerProfile map[string]any
	if err := decodeStrictResponse(workerResponse, &workerProfile); err != nil {
		return err
	}
	if workerProfile["apiVersion"] != supportAPIVersion || workerProfile["kind"] != "FormSupport" {
		return errors.New("ModuleWorker support profile identity is invalid")
	}
	operations, _ := workerProfile["operations"].([]any)
	if len(operations) == 0 {
		return errors.New("ModuleWorker support profile advertises no operations")
	}
	r.complete("module-worker-runtime-contract-advertised")
	return nil
}

// checkUndeclaredRuntimeHandlerRejected proves the handler surface is closed by
// the ABI.
//
// `handlers` is what every inward-activation attachment is admitted against, so
// a stored version claiming an entry point no conforming runtime invokes would
// let one host attach a schedule or a queue to something another host cannot
// run. The refusal is asserted at BOTH pre-mutation gates a client meets — the
// advisory `validate` surface and the binding `prepare` surface — because a host
// that only refused at one of them would let a client plan a version it can
// never apply. Nothing is stored either way, and a version declaring only
// handlers the contract DOES define must still be accepted, or a host could
// pass this check by refusing every version.
func (r *v3Runner) checkUndeclaredRuntimeHandlerRejected() error {
	input := r.contract.RunnerInput
	runtime := input.SupportProbes.RuntimeContract
	subject := "a WorkerVersion declaring handler " + runtime.UndefinedHandler +
		", which the " + runtime.Name + "@" + runtime.Version + " contract does not define"

	offender := r.workerVersionOf("runtime-abi-version", input.ModuleWorker.Name, fetchHandler, runtime.UndefinedHandler)
	valid, diagnostics, err := r.validateResource(offender, offender.Spec)
	if err != nil {
		return err
	}
	if valid || diagnostics == 0 {
		return fmt.Errorf("validate accepted %s", subject)
	}
	response, err := r.prepareRequest(offender, map[string]string{})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("prepare of %s: %w", subject, err)
	}
	if err := r.expectResourceAbsent(offender); err != nil {
		return fmt.Errorf("%s mutated state: %w", subject, err)
	}

	accepted := r.workerVersionOf("runtime-abi-version", input.ModuleWorker.Name, runtime.Handlers...)
	if _, _, err := r.applyResource(accepted, applyOptions{
		Create: true, IdempotencyKey: "key-declared-runtime-handlers",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a WorkerVersion declaring only handlers the runtime contract defines: %w", err)
	}
	if err := r.deleteExisting(accepted, "key-declared-runtime-handlers-delete"); err != nil {
		return err
	}
	r.complete("undeclared-runtime-handler-rejected")
	return nil
}
