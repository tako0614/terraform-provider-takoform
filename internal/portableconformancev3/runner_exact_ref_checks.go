package portableconformancev3

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// The exact-identity half of the lane (spec/decisions/0022).
//
// Everything here rests on one corpus fact: the host installs TWO definition
// versions of one group and kind. Without a second contract of one Form line,
// "answers for the exact ref" and "answers for the kind" are the same behavior
// and no check can tell them apart — which is exactly why the corpus pins the
// second Definition as bytes rather than inventing one at runtime.

// syntheticDefinitionTarget builds one probe target of the corpus-pinned SECOND
// definition version of the ModuleWorker line. Its desired state is the empty
// object, exactly as the first version's is, so any difference the lane
// observes is a difference of identity rather than of content.
func (r *v3Runner) syntheticDefinitionTarget(name string) probeTarget {
	probe := r.contract.RunnerInput.SyntheticSecondDefinitionVersion
	return probeTarget{
		Ref:   probe.FormRef,
		Name:  name,
		Space: r.contract.RunnerInput.Space,
		Spec:  map[string]any{},
		// A Module Worker's readiness is a claim about service, and nothing
		// serves until a deployment does; the checks that care assert the exact
		// condition themselves.
		ReadyOptional: true,
	}
}

// formDefinitionURL renders the exact Form Definition route for one identity.
func (r *v3Runner) formDefinitionURL(ref FormRef) string {
	return fmt.Sprintf(
		"%s/form-definitions/%s/%s?%s",
		r.apiBase, groupSegments(ref.APIVersion), url.PathEscape(ref.Kind),
		r.exactQuery(r.contract.RunnerInput.Space, ref).Encode(),
	)
}

// formSupportURL renders the support profile route of one Form line. The
// published path carries the definition version and no digest, so this is the
// one surface where a host resolves the exact identity from a version.
func (r *v3Runner) formSupportURL(ref FormRef) string {
	return fmt.Sprintf(
		"%s/support/forms/%s/%s/%s",
		r.apiBase, groupSegments(ref.APIVersion), url.PathEscape(ref.Kind),
		url.PathEscape(ref.DefinitionVersion),
	)
}

// checkTwoDefinitionVersionsAnswerIndependently proves the property a
// kind-keyed catalog cannot have: two contracts of ONE group and kind are
// installed at the same time, and every identity surface answers each of them
// about itself.
//
// It also proves the mixed identity fails closed. A query naming one version's
// definitionVersion with the other's schemaDigest names no Definition this host
// holds, and a host that resolved the group and kind first would answer it
// about whichever contract it found.
func (r *v3Runner) checkTwoDefinitionVersionsAnswerIndependently() error {
	first := r.contract.RunnerInput.ModuleWorker.Identity.FormRef
	second := r.contract.RunnerInput.SyntheticSecondDefinitionVersion.FormRef
	if first.APIVersion != second.APIVersion || first.Kind != second.Kind ||
		first.DefinitionVersion == second.DefinitionVersion {
		return errors.New("the corpus does not pin two definition versions of one group and kind")
	}

	descriptions := map[string]string{}
	for _, ref := range []FormRef{first, second} {
		availability, err := r.request(
			http.MethodGet, r.apiBase+"/forms?"+r.exactQuery(r.contract.RunnerInput.Space, ref).Encode(),
			nil, nil,
		)
		if err != nil {
			return err
		}
		if availability.Status != http.StatusOK {
			return fmt.Errorf(
				"availability for %s@%s HTTP %d; a host holding one contract per kind cannot serve both",
				ref.Kind, ref.DefinitionVersion, availability.Status,
			)
		}
		var document struct {
			Forms []struct {
				Identity struct {
					FormRef       FormRef `json:"formRef"`
					PackageDigest string  `json:"packageDigest,omitempty"`
				} `json:"identity"`
				DefinitionKnown      bool     `json:"definitionKnown"`
				Installed            bool     `json:"installed"`
				Executable           bool     `json:"executable"`
				Activated            bool     `json:"activated"`
				AvailableToPrincipal bool     `json:"availableToPrincipal"`
				Operations           []string `json:"operations"`
				Deprecated           bool     `json:"deprecated,omitempty"`
			} `json:"forms"`
		}
		if err := decodeStrictResponse(availability, &document); err != nil {
			return err
		}
		if len(document.Forms) != 1 || document.Forms[0].Identity.FormRef != ref {
			return fmt.Errorf("availability for %s@%s answered about another identity", ref.Kind, ref.DefinitionVersion)
		}

		definition, err := r.request(http.MethodGet, r.formDefinitionURL(ref), nil, nil)
		if err != nil {
			return err
		}
		if definition.Status != http.StatusOK {
			return fmt.Errorf("Form Definition for %s@%s HTTP %d", ref.Kind, ref.DefinitionVersion, definition.Status)
		}
		var served wireFormDefinition
		if err := decodeStrictResponse(definition, &served); err != nil {
			return err
		}
		if served.Identity.FormRef != ref {
			return fmt.Errorf(
				"the Form Definition surface served identity %+v for the %s request",
				served.Identity.FormRef, ref.DefinitionVersion,
			)
		}
		descriptions[ref.DefinitionVersion] = served.Description

		profile, err := r.request(http.MethodGet, r.formSupportURL(ref), nil, nil)
		if err != nil {
			return err
		}
		if profile.Status != http.StatusOK {
			return fmt.Errorf("support profile for %s@%s HTTP %d", ref.Kind, ref.DefinitionVersion, profile.Status)
		}
		var support struct {
			APIVersion string   `json:"apiVersion"`
			Kind       string   `json:"kind"`
			FormRef    FormRef  `json:"formRef"`
			Operations []string `json:"operations"`
		}
		if err := decodeStrictResponse(profile, &support); err != nil {
			return err
		}
		if support.FormRef != ref {
			return fmt.Errorf(
				"the support profile for definition version %s echoed %+v",
				ref.DefinitionVersion, support.FormRef,
			)
		}
	}
	// Two identities that served the SAME document would mean the host answered
	// one request about the other's contract.
	if descriptions[first.DefinitionVersion] == descriptions[second.DefinitionVersion] {
		return errors.New("both definition versions served one Form Definition document")
	}

	// A mixed identity names no installed Definition.
	mixed := first
	mixed.DefinitionVersion = second.DefinitionVersion
	for _, surface := range []struct {
		label  string
		target string
	}{
		{"availability", r.apiBase + "/forms?" + r.exactQuery(r.contract.RunnerInput.Space, mixed).Encode()},
		{"the Form Definition surface", r.formDefinitionURL(mixed)},
	} {
		response, err := r.request(http.MethodGet, surface.target, nil, nil)
		if err != nil {
			return err
		}
		if err := r.expectStableError(response, "form_unknown"); err != nil {
			return fmt.Errorf(
				"%s answered a definitionVersion and schemaDigest that belong to different contracts: %w",
				surface.label, err,
			)
		}
	}
	r.complete("two-definition-versions-answer-independently")
	return nil
}

// checkResourceAnswersOnlyUnderItsRecordedFormRef proves what the exact catalog
// is FOR: a resource is answered under the contract it was created under, and
// under no other, on every route that addresses it.
//
// The alternative a kind-keyed host has is worse than an error. Every one of
// these requests would succeed, describing the resource under a contract it was
// never applied under, and the client's own state — which records the exact ref
// — would have no way to notice.
func (r *v3Runner) checkResourceAnswersOnlyUnderItsRecordedFormRef() error {
	first := r.target(r.contract.RunnerInput.ModuleWorker)
	first.Name = "recorded-ref-probe"
	created, _, err := r.applyResource(first, applyOptions{
		Create: true, IdempotencyKey: "key-recorded-ref-create",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	if created.Form.FormRef != first.Ref {
		return errors.New("create echoed a FormRef other than the one it was applied under")
	}

	// The SAME name under the other definition version addresses nothing. The
	// Form itself is installed, so `form_unknown` would be the wrong answer:
	// what is absent is a resource of that name under that contract.
	other := r.syntheticDefinitionTarget(first.Name)
	read, err := r.request(
		http.MethodGet, r.resourceURL(other.Ref, other.Name, "", r.exactQuery(other.Space, other.Ref)), nil, nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(read, "resource_not_found"); err != nil {
		return fmt.Errorf("a read under the other definition version was answered about the stored resource: %w", err)
	}
	observe, err := r.statusAction(other, "observe", created.Metadata.Generation, "key-recorded-ref-observe", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(observe, "resource_not_found"); err != nil {
		return fmt.Errorf("an observe under the other definition version reached the stored resource: %w", err)
	}
	update, err := r.apply(other, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-recorded-ref-update",
		// A syntactically valid digest that binds nothing: the identity refusal
		// must land before the prepared review is even consulted.
		PrepareDigest: "sha256:" + strings.Repeat("0", 64),
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(update, "resource_not_found"); err != nil {
		return fmt.Errorf("an apply under the other definition version reached the stored resource: %w", err)
	}
	remove, err := r.deleteResource(other, created.Metadata.Generation, "key-recorded-ref-delete", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(remove, "resource_not_found"); err != nil {
		return fmt.Errorf("a delete under the other definition version reached the stored resource: %w", err)
	}

	// A resource of the SECOND contract lives alongside the first.
	second := r.syntheticDefinitionTarget("recorded-ref-probe-second")
	secondCreated, _, err := r.applyResource(second, applyOptions{
		Create: true, IdempotencyKey: "key-recorded-ref-second-create",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("a resource of the second definition version could not be created: %w", err)
	}
	if secondCreated.Form.FormRef != second.Ref {
		return errors.New("the second definition version's resource echoed another identity")
	}
	if secondCreated.Metadata.UID == created.Metadata.UID {
		return errors.New("resources of two contracts shared a uid")
	}

	// Both are readable, and NEITHER response rewrites the other's recorded ref.
	firstRead, _, err := r.readRawResponse(first)
	if err != nil {
		return err
	}
	if firstRead.Form.FormRef != first.Ref {
		return fmt.Errorf(
			"reading the older resource returned formRef %+v; a host that rewrote it to the newer contract "+
				"would silently reinterpret state written against another one",
			firstRead.Form.FormRef,
		)
	}
	if firstRead.Metadata.UID != created.Metadata.UID {
		return errors.New("the older resource changed incarnation when a second contract was exercised")
	}
	secondRead, _, err := r.readRawResponse(second)
	if err != nil {
		return err
	}
	if secondRead.Form.FormRef != second.Ref {
		return fmt.Errorf("reading the newer resource returned formRef %+v", secondRead.Form.FormRef)
	}
	r.complete("resource-answers-only-under-its-recorded-form-ref")
	return nil
}

// exactRefRelationFixture creates the two Module Workers the relation checks
// point at: one under the family contract, one under the second definition
// version that provides no runtime ABI at all.
func (r *v3Runner) exactRefRelationFixture() (worker, legacy probeTarget, err error) {
	worker = r.target(r.contract.RunnerInput.ModuleWorker)
	worker.Name = "target-contract-worker"
	if _, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-target-contract-worker",
	}, http.StatusCreated); err != nil {
		return probeTarget{}, probeTarget{}, err
	}
	legacy = r.syntheticDefinitionTarget("target-contract-other")
	if _, _, err := r.applyResource(legacy, applyOptions{
		Create: true, IdempotencyKey: "key-target-contract-other",
	}, http.StatusCreated); err != nil {
		return probeTarget{}, probeTarget{}, err
	}
	return worker, legacy, nil
}

// checkRelationTargetFormRefVerified proves the exact-Form half of the relation
// contract: a reference whose annotation pins exact Form identities is refused
// when the resolved target is recorded under another one, before any mutation.
//
// The target EXISTS and has exactly the referenced group and kind, so neither
// the absent-target rule nor the wrong-kind rule can account for the refusal.
// What fails is the only thing left: the contract the target satisfies.
func (r *v3Runner) checkRelationTargetFormRefVerified(worker, legacy probeTarget) error {
	version := r.workerVersionOf("target-contract-version", worker.Name, "fetch")
	if _, _, err := r.applyResource(version, applyOptions{
		Create: true, IdempotencyKey: "key-target-contract-version",
	}, http.StatusCreated); err != nil {
		return err
	}
	// A WorkerDeployment's /worker is the family's one exact-Form reference to a
	// Module Worker: the deployment renders that worker's readiness and is
	// governed by rules stated over the Module Worker Form itself.
	offender := r.deploymentOf("target-contract-deployment", legacy.Name, version.Name)
	response, err := r.apply(offender, applyOptions{
		Create: true, IdempotencyKey: "key-target-contract-deployment",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("a reference to a target of the wrong exact Form was accepted: %w", err)
	}
	message := strings.TrimSpace(string(response.Body))
	for _, want := range []string{"/worker", legacy.Ref.DefinitionVersion, worker.Ref.SchemaDigest} {
		if !strings.Contains(message, want) {
			return fmt.Errorf(
				"the refusal must name the relation pointer, what was required, and what the target is "+
					"(missing %q): %s",
				want, message,
			)
		}
	}
	if err := r.expectResourceAbsent(offender); err != nil {
		return fmt.Errorf("the refused deployment mutated state: %w", err)
	}
	r.complete("relation-target-form-ref-verified")
	return nil
}

// checkRelationTargetInterfaceVerified proves the other half: a reference whose
// annotation names an Interface is refused when the resolved target's Form does
// not declare that exact contract.
//
// A Worker Version's /worker requires the ES Module Worker runtime ABI, because
// what a version needs from the identity it belongs to is the contract that
// decides which handlers exist at all. The second definition version withholds
// exactly that Interface, so the target is of the right group and kind, exists,
// and still cannot answer for the relation.
func (r *v3Runner) checkRelationTargetInterfaceVerified(legacy probeTarget) error {
	withheld := r.contract.RunnerInput.SyntheticSecondDefinitionVersion.WithheldInterface
	offender := r.workerVersionOf("target-interface-version", legacy.Name, "fetch")
	response, err := r.apply(offender, applyOptions{
		Create: true, IdempotencyKey: "key-target-interface-version",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("a reference to a target providing no required Interface was accepted: %w", err)
	}
	message := strings.TrimSpace(string(response.Body))
	for _, want := range []string{"/worker", withheld.Name, withheld.Version} {
		if !strings.Contains(message, want) {
			return fmt.Errorf(
				"the refusal must name the relation pointer and the required Interface (missing %q): %s",
				want, message,
			)
		}
	}
	if err := r.expectResourceAbsent(offender); err != nil {
		return fmt.Errorf("the refused version mutated state: %w", err)
	}
	r.complete("relation-target-interface-verified")
	return nil
}

// checkRelationPinRecordsTargetFormRef proves the stored pin carries the
// target's exact FormRef alongside its UID.
//
// The pin is host-owned bookkeeping, so the lane observes it where the host
// already reports it: a target replaced out of band produces the relation-drift
// condition, and a host that recorded the contract names BOTH the ref it was
// bound to and the ref that is there now. A host that stored only the UID could
// not name either.
//
// The replacement deliberately comes back under the OTHER definition version,
// so the recorded and current refs differ and the report has to distinguish
// them.
func (r *v3Runner) checkRelationPinRecordsTargetFormRef() error {
	worker := r.target(r.contract.RunnerInput.ModuleWorker)
	worker.Name = "pin-ref-worker"
	created, _, err := r.applyResource(worker, applyOptions{
		Create: true, IdempotencyKey: "key-pin-ref-worker",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	source := r.workerVersionOf("pin-ref-version", worker.Name, "fetch")
	bound, _, err := r.applyResource(source, applyOptions{
		Create: true, IdempotencyKey: "key-pin-ref-version",
	}, http.StatusCreated)
	if err != nil {
		return err
	}

	removed, err := r.deleteResource(
		worker, created.Metadata.Generation, "key-pin-ref-worker-delete",
		map[string]string{ErrorProbeHeader: ProbeExternalChange},
	)
	if err != nil {
		return err
	}
	if removed.Status != http.StatusNoContent {
		return fmt.Errorf(
			"out-of-band worker delete HTTP %d, want 204; body=%s",
			removed.Status, strings.TrimSpace(string(removed.Body)),
		)
	}
	replacement := r.syntheticDefinitionTarget(worker.Name)
	recreated, _, err := r.applyResource(replacement, applyOptions{
		Create: true, IdempotencyKey: "key-pin-ref-worker-recreate",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the replacement under the second definition version was refused: %w", err)
	}

	drifted, err := r.readRaw(source)
	if err != nil {
		return err
	}
	if err := requireNotReady(drifted, "ExternalChange"); err != nil {
		return fmt.Errorf("a source whose target was replaced under another contract: %w", err)
	}
	hostReason := readyCondition(drifted).HostReason
	for _, want := range []string{
		"/worker",
		created.Metadata.UID, recreated.Metadata.UID,
		worker.Ref.DefinitionVersion, worker.Ref.SchemaDigest,
		replacement.Ref.DefinitionVersion, replacement.Ref.SchemaDigest,
	} {
		if !strings.Contains(hostReason, want) {
			return fmt.Errorf(
				"the stored pin must record the target's exact FormRef as well as its uid; the drift report "+
					"%q does not name %q",
				hostReason, want,
			)
		}
	}
	if drifted.Metadata.Generation != bound.Metadata.Generation {
		return errors.New("an unre-applied source changed generation, so the host altered desired state")
	}
	r.complete("relation-pin-records-target-form-ref")
	return nil
}
