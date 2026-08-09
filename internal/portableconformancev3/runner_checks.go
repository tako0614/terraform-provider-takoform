package portableconformancev3

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// run executes the complete ordered v1alpha3 matrix. Every named required
// check is marked completed exactly where its evidence was actually
// exercised over real HTTP.
func (r *v3Runner) run() error {
	// The served Form Definitions come first: every probe spec is materialized
	// against the declared defaults before it is sent, so the runner speaks the
	// same effective specs the host stores and echoes.
	if err := r.loadDesiredSchemas(); err != nil {
		return err
	}
	input := r.contract.RunnerInput
	kv := r.target(input.EdgeKvNamespace)
	mw := r.target(input.ModuleWorker)
	queue := r.target(input.AtLeastOnceQueue)
	bundle := r.target(input.WorkerBundle.ResourceProbe)
	version := r.target(input.WorkerVersion)

	steps := []func() error{
		r.checkDiscovery,
		r.checkErrorTaxonomy,
		func() error { return r.checkFormsAvailability(mw) },
		func() error { return r.checkFormDefinitions(mw, kv) },
		func() error { return r.checkValidate(mw, kv, queue) },
		r.checkNegativeFixtures,
		func() error { return r.checkPrepareBinding(queue) },
		func() error { return r.checkPrepareSubstitution(queue) },
		func() error { return r.checkApplyHeaders(kv) },
		func() error { return r.checkCreateLifecycle(kv, mw, queue) },
		func() error { return r.checkGenerationFences(queue) },
		func() error { return r.checkPrepareFences(queue) },
		func() error { return r.checkReadETags(kv, queue) },
		// The lane's URL shape is proved against a resource that exists, so the
		// split path is shown ADDRESSING something rather than merely routing.
		func() error { return r.checkNamespacedGroupPathSegments(kv) },
		func() error { return r.checkConditionReasonsClosed(kv, mw, queue) },
		func() error { return r.checkSpaceIDGrammar(kv) },
		func() error { return r.checkConcurrentUnrelatedMutation(kv, queue) },
		func() error { return r.checkObserveAndStatusTouch(queue) },
		func() error { return r.checkExpectedUID(queue) },
		func() error { return r.checkPackageDigestNotIdentity(queue) },
		r.checkSameKindTwoGroups,
		func() error { return r.checkArtifacts(bundle) },
		r.checkArtifactRejectList,
		r.checkArtifactManifestKindExclusive,
		r.checkBundleMainModuleIsLoadable,
		func() error { return r.checkArtifactRetentionWhileReferenced(bundle) },
		// Ownership of the artifact surface: an upload id is a handle, and a
		// content address is a name rather than an entitlement
		// (spec/decisions/0018).
		r.checkUploadSessionOwnership,
		r.checkArtifactDigestIsNotACapability,
		func() error { return r.checkManifestReferenceIsNotACapability(bundle) },
		func() error { return r.checkWorkerVersionFlow(version, kv) },
		func() error { return r.checkBindingContractVerified(version) },
		func() error { return r.checkRelationIncarnationChange(version, kv) },
		// The Worker aggregate sequence (spec/decisions/0016) is ordered by the
		// state it needs: the attachment gate is proved while the worker still
		// has NO deployment, the deployment is created next, and every later
		// aggregate rule reads that deployment.
		r.checkAttachmentRequiresActiveDeployment,
		r.checkDeploymentWeightSum,
		r.checkDeploymentIntegrity,
		r.checkHandlerGatedAttachments,
		// The two attachment rules a schema cannot state: a cron expression is a
		// schedule rather than a shape, and one queue has one consumer
		// (spec/decisions/0020). Both run against the worker the gate check just
		// deployed, whose versions export every handler.
		r.checkCronGrammarEnforced,
		r.checkSingleQueueConsumerEnforced,
		// What an attachment CLAIMS, decided on the identity the claim names
		// rather than on the bytes a client wrote (spec/decisions/0026). The
		// hostname pair runs in order: the first stores the claim under its
		// canonical spelling, the second proves nothing else may take it.
		r.checkCustomDomainHostnameCanonicalized,
		r.checkCustomDomainHostnameClaimUnique,
		r.checkDeadLetterCycleRejected,
		// The ABI closes the handler surface those attachments are gated on
		// (spec/decisions/0019): first by vocabulary, then by what the code a
		// version actually runs exports.
		r.checkUndeclaredRuntimeHandlerRejected,
		r.checkDeclaredHandlerNotExportedRejected,
		r.checkBindingNameCollision,
		// Reachability without a customer-owned domain (spec/decisions/0024).
		// It runs here because it needs exactly the state the gate check built:
		// a worker whose active deployment serves fetch.
		r.checkWorkerEndpointAddressIsHostAssigned,
		r.checkWorkerEndpointSinglePerWorker,
		// The one Form that declares an output contract is live right here, and
		// so are five that declare none, which is what the declared-output
		// comparison needs on both sides (spec/decisions/0025).
		r.checkFormDeclaredOutputsAreExact,
		r.checkWorkerEndpointFollowsTheActiveDeployment,
		// And the address is the same address afterwards, for the same uid,
		// across every trigger a black-box runner can cause (decision 0024).
		r.checkWorkerEndpointAddressIsStableForItsUID,
		r.checkDeploymentChangePreservesDependents,
		r.checkDeploymentDeleteBlockedByDependent,
		// Readiness rendered from the deployment is the second half of the
		// aggregate: what a worker SERVES changes when its deployment changes, so
		// the representation the host hands out changes with it.
		r.checkDependentRevisionAdvancesWithRendering,
		r.checkRelationReapplyRepins,
		func() error { return r.checkRelationDeletionProtection(version, bundle) },
		func() error { return r.checkImportFlows(kv, version) },
		func() error { return r.checkDeleteFences(version, kv) },
		func() error { return r.checkOperations(kv) },
		func() error { return r.checkOperationOwnership(kv) },
		// State continuity: what a client that persists an operation id and
		// dispatches on the exact FormRef in its state depends on the host for
		// (spec/decisions/0017).
		func() error { return r.checkOperationResumableAfterSettlement(queue) },
		func() error { return r.checkExactFormRefFailsClosedOnUnknownDefinition(kv) },
		// The exact identity a host answers for, and the contract a relation
		// pins (spec/decisions/0022). The two definition versions of one Form
		// line are installed throughout; these are the checks that can tell.
		r.checkTwoDefinitionVersionsAnswerIndependently,
		r.checkResourceAnswersOnlyUnderItsRecordedFormRef,
		func() error {
			worker, legacy, err := r.exactRefRelationFixture()
			if err != nil {
				return err
			}
			if err := r.checkRelationTargetFormRefVerified(worker, legacy); err != nil {
				return err
			}
			return r.checkRelationTargetInterfaceVerified(legacy)
		},
		r.checkRelationPinRecordsTargetFormRef,
		func() error { return r.checkAsyncCommitRevalidates(kv, version) },
		r.checkAsyncCommitBindsAcceptedIdentity,
		func() error { return r.checkCrossSpace(kv) },
		r.checkSupportProfiles,
		r.checkRuntimeContractAdvertised,
		r.checkEdgeInterfaceContractsAdvertised,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (r *v3Runner) checkDiscovery() error {
	response, err := r.request(http.MethodGet, r.endpoint+r.contract.DiscoveryPath, nil, nil)
	if err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf("discovery HTTP %d", response.Status)
	}
	var discovery struct {
		APIVersions []string          `json:"api_versions"`
		Features    map[string]bool   `json:"features"`
		Endpoints   map[string]string `json:"endpoints"`
	}
	if err := decodeStrictResponse(response, &discovery); err != nil {
		return err
	}
	if len(discovery.APIVersions) != 1 || discovery.APIVersions[0] != r.contract.APIVersion {
		return fmt.Errorf("discovery api_versions = %v", discovery.APIVersions)
	}
	for _, feature := range []string{
		"service_forms", "exact_form_ref", "optimistic_concurrency",
		"idempotent_lifecycle", "operations", "artifact_upload", "support_profiles",
	} {
		if !discovery.Features[feature] {
			return fmt.Errorf("discovery does not advertise features.%s", feature)
		}
	}
	wantEndpoints := map[string]string{
		"api":        r.contract.APIPath,
		"artifacts":  r.contract.APIPath + "/artifacts",
		"operations": r.contract.APIPath + "/operations",
		"support":    r.contract.APIPath + "/support",
	}
	configured, err := url.Parse(r.endpoint)
	if err != nil {
		return err
	}
	for name, wantPath := range wantEndpoints {
		raw := discovery.Endpoints[name]
		if raw == "" {
			if name == "api" {
				return errors.New("discovery omitted endpoints.api")
			}
			continue
		}
		advertised, err := url.Parse(raw)
		if err != nil || !advertised.IsAbs() {
			return fmt.Errorf("discovery %s endpoint is not absolute: %q", name, raw)
		}
		if !strings.EqualFold(advertised.Scheme, configured.Scheme) || advertised.Host != configured.Host {
			return fmt.Errorf("discovery %s endpoint is not same-origin: %q", name, raw)
		}
		if advertised.EscapedPath() != wantPath {
			return fmt.Errorf("discovery %s endpoint path = %q, want %q", name, advertised.EscapedPath(), wantPath)
		}
	}
	r.complete("discovery-exact")
	return nil
}

func (r *v3Runner) checkErrorTaxonomy() error {
	for _, code := range r.contract.ErrorEnvelope.Codes {
		response, err := r.request(
			http.MethodGet,
			r.apiBase+"/forms",
			map[string]string{ErrorProbeHeader: ProbeErrorPrefix + code},
			nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(response, code); err != nil {
			return fmt.Errorf("error probe %s: %w", code, err)
		}
		r.errorProbes = append(r.errorProbes, ErrorProbeEvidence{
			Code:       code,
			HTTPStatus: response.Status,
			Retryable:  isAutomaticallyRetryable(code),
		})
	}
	r.complete("error-envelope-taxonomy")
	return nil
}

func (r *v3Runner) checkFormsAvailability(target probeTarget) error {
	response, err := r.request(
		http.MethodGet,
		r.apiBase+"/forms?"+r.exactQuery(target.Space, target.Ref).Encode(),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf("forms HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	var availability struct {
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
	if err := decodeStrictResponse(response, &availability); err != nil {
		return err
	}
	if len(availability.Forms) != 1 {
		return fmt.Errorf("forms returned %d availabilities, want exactly 1", len(availability.Forms))
	}
	entry := availability.Forms[0]
	if entry.Identity.FormRef != target.Ref {
		return errors.New("forms availability identity is not the requested exact FormRef")
	}
	if entry.Identity.PackageDigest != target.PackageDigest {
		return errors.New("forms availability packageDigest audit evidence drifted")
	}
	if !entry.DefinitionKnown || !entry.Installed || !entry.Executable ||
		!entry.Activated || !entry.AvailableToPrincipal {
		return errors.New("forms availability is not fully available")
	}
	// The advertised capability set must be EXACTLY the set the corpus pins for
	// this exact Form. An omitted capability breaks a client that was promised
	// it; an extra one promises an operation the Definition never declared, and
	// a client will hold the host to it. Asserting a hardcoded "must include
	// update" instead would have baked one Form's shape into every Form.
	if !equalStringSlices(entry.Operations, target.Lifecycle) {
		return fmt.Errorf(
			"forms availability operations = %v, want exactly the contract-declared %v",
			entry.Operations, target.Lifecycle,
		)
	}
	// A substituted schemaDigest is a different exact Form and must be
	// unknown, not silently resolved.
	substituted := r.exactQuery(target.Space, target.Ref)
	substituted.Set("schemaDigest", formpackage.DigestBytes([]byte("substituted-schema")))
	wrongResponse, err := r.request(http.MethodGet, r.apiBase+"/forms?"+substituted.Encode(), nil, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(wrongResponse, "form_unknown"); err != nil {
		return err
	}
	r.complete("forms-exact-availability")
	return nil
}

func (r *v3Runner) checkFormDefinitions(targets ...probeTarget) error {
	for _, target := range targets {
		definition, err := r.formDefinition(target.Ref)
		if err != nil {
			return err
		}
		// packageDigest is optional audit evidence on this surface; when a host
		// does state it, it must be the installed one.
		if definition.Identity.PackageDigest != "" &&
			definition.Identity.PackageDigest != target.PackageDigest {
			return errors.New("form-definition packageDigest audit evidence drifted")
		}
	}
	r.complete("form-definition-exact")
	return nil
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func (r *v3Runner) validateResource(target probeTarget, spec map[string]any) (bool, int, error) {
	document := r.resourceBody(probeTarget{
		Ref: target.Ref, PackageDigest: target.PackageDigest,
		Name: target.Name, Space: target.Space, Spec: spec,
	})
	body, err := encodeRunnerJSON(document)
	if err != nil {
		return false, 0, err
	}
	response, err := r.request(http.MethodPost, r.apiBase+"/resources/validate", nil, body)
	if err != nil {
		return false, 0, err
	}
	if response.Status != http.StatusOK {
		return false, 0, fmt.Errorf("validate HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	var result struct {
		Valid       bool `json:"valid"`
		Diagnostics []struct {
			Severity string `json:"severity"`
			Field    string `json:"field,omitempty"`
			Message  string `json:"message"`
		} `json:"diagnostics"`
	}
	if err := decodeStrictResponse(response, &result); err != nil {
		return false, 0, err
	}
	if result.Diagnostics == nil {
		return false, 0, errors.New("validate omitted diagnostics")
	}
	return result.Valid, len(result.Diagnostics), nil
}

func (r *v3Runner) checkValidate(targets ...probeTarget) error {
	for _, target := range targets {
		valid, diagnostics, err := r.validateResource(target, target.Spec)
		if err != nil {
			return err
		}
		if !valid || diagnostics != 0 {
			return fmt.Errorf("validate rejected the canonical %s probe", target.Ref.Kind)
		}
	}
	r.complete("validate-accepts-canonical")
	return nil
}

func (r *v3Runner) checkNegativeFixtures() error {
	byKind := map[string]probeTarget{
		r.contract.RunnerInput.ModuleWorker.Identity.FormRef.Kind:     r.target(r.contract.RunnerInput.ModuleWorker),
		r.contract.RunnerInput.EdgeKvNamespace.Identity.FormRef.Kind:  r.target(r.contract.RunnerInput.EdgeKvNamespace),
		r.contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef.Kind: r.target(r.contract.RunnerInput.AtLeastOnceQueue),
		r.contract.RunnerInput.WorkerVersion.Identity.FormRef.Kind:    r.target(r.contract.RunnerInput.WorkerVersion),
		r.contract.RunnerInput.WorkerBundle.Identity.FormRef.Kind:     r.target(r.contract.RunnerInput.WorkerBundle.ResourceProbe),
	}
	for _, fixture := range r.contract.RunnerInput.NegativeFixtures {
		target, known := byKind[fixture.Kind]
		if !known {
			return fmt.Errorf("negative fixture %q names an unknown probe kind", fixture.Name)
		}
		valid, diagnostics, err := r.validateResource(target, fixture.Input)
		if err != nil {
			return err
		}
		if valid || diagnostics == 0 {
			return fmt.Errorf("validate accepted negative fixture %q", fixture.Name)
		}
		r.negativeFixtures = append(r.negativeFixtures, NegativeFixtureEvidence{
			Name: fixture.Name, Kind: fixture.Kind, Stage: fixture.Stage, SHA256: fixture.SHA256,
		})
	}
	r.complete("validate-rejects-negative-fixtures")
	return nil
}

func (r *v3Runner) checkPrepareBinding(queue probeTarget) error {
	prepared, err := r.prepare(queue)
	if err != nil {
		return err
	}
	wantSpecDigest, err := specCanonicalDigest(queue.Spec)
	if err != nil {
		return err
	}
	if prepared.SpecDigest != wantSpecDigest {
		return errors.New("prepare specDigest is not the RFC 8785 canonical digest of the requested spec")
	}
	r.complete("prepare-binds-exact-spec")
	return nil
}

func (r *v3Runner) checkPrepareSubstitution(queue probeTarget) error {
	prepared, err := r.prepare(queue)
	if err != nil {
		return err
	}
	substituted := queue
	substituted.Spec = map[string]any{"messageRetentionSeconds": 600}
	response, err := r.apply(substituted, applyOptions{
		Create:         true,
		IdempotencyKey: "key-prepare-substitution",
		PrepareDigest:  prepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("prepare substitution: %w", err)
	}
	if err := r.expectResourceAbsent(queue); err != nil {
		return fmt.Errorf("prepare substitution mutated state: %w", err)
	}
	r.complete("prepare-substitution-rejected")
	return nil
}

func (r *v3Runner) checkApplyHeaders(kv probeTarget) error {
	headersProbe := kv
	headersProbe.Name = "headers-probe"
	prepared, err := r.prepare(headersProbe)
	if err != nil {
		return err
	}
	missingKey, err := r.apply(headersProbe, applyOptions{
		Create:             true,
		OmitIdempotencyKey: true,
		PrepareDigest:      prepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(missingKey, "invalid_argument"); err != nil {
		return fmt.Errorf("missing Idempotency-Key: %w", err)
	}
	missingPrecondition, err := r.apply(headersProbe, applyOptions{
		IdempotencyKey:   "key-headers-probe",
		OmitPrecondition: true,
		PrepareDigest:    prepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(missingPrecondition, "invalid_argument"); err != nil {
		return fmt.Errorf("missing If-None-Match: %w", err)
	}
	if err := r.expectResourceAbsent(headersProbe); err != nil {
		return fmt.Errorf("header rejection mutated state: %w", err)
	}
	r.complete("apply-headers-required")
	return nil
}

func (r *v3Runner) checkCreateLifecycle(kv, mw, queue probeTarget) error {
	// Create the KV namespace and keep the exact request for byte-replay.
	prepared, err := r.prepare(kv)
	if err != nil {
		return err
	}
	createOptions := applyOptions{
		Create:         true,
		IdempotencyKey: "key-create-kv",
		PrepareDigest:  prepared.PrepareDigest,
	}
	fullURL, headers, body, err := r.applyRequestParts(kv, createOptions)
	if err != nil {
		return err
	}
	createResponse, err := r.request(http.MethodPut, fullURL, headers, body)
	if err != nil {
		return err
	}
	created, err := decodeResource(createResponse, http.StatusCreated)
	if err != nil {
		return err
	}
	if err := verifyResourceIdentity(created, kv); err != nil {
		return err
	}
	// The uid is host-issued and opaque: the runner asserts only the wire
	// grammar ($defs/uid), never a host-private format.
	if created.Metadata.UID == "" || !uidPattern.MatchString(created.Metadata.UID) {
		return fmt.Errorf("create did not mint a grammar-valid host-issued uid, got %q", created.Metadata.UID)
	}
	if created.Metadata.Generation != "1" || created.Metadata.Revision != "1" {
		return fmt.Errorf(
			"create identity = generation %s revision %s, want 1/1",
			created.Metadata.Generation, created.Metadata.Revision,
		)
	}
	if err := verifyRevisionETag(createResponse, created.Metadata.Revision); err != nil {
		return err
	}
	r.uidTransitions = append(r.uidTransitions, "create:"+created.Metadata.UID)
	r.complete("apply-create-uid-minted")

	// Exact byte replay must return the recorded response byte-for-byte.
	replayResponse, err := r.request(http.MethodPut, fullURL, headers, body)
	if err != nil {
		return err
	}
	if replayResponse.Status != createResponse.Status ||
		!bytes.Equal(replayResponse.Body, createResponse.Body) ||
		replayResponse.Header.Get("ETag") != createResponse.Header.Get("ETag") {
		return errors.New("idempotent replay was not byte-identical")
	}
	// The same key with a different fingerprint is a client defect.
	mutated := kv
	mutated.PackageDigest = formpackage.DigestBytes([]byte("fingerprint-change"))
	conflictURL, conflictHeaders, conflictBody, err := r.applyRequestParts(mutated, createOptions)
	if err != nil {
		return err
	}
	conflictResponse, err := r.request(http.MethodPut, conflictURL, conflictHeaders, conflictBody)
	if err != nil {
		return err
	}
	if err := r.expectStableError(conflictResponse, "invalid_argument"); err != nil {
		return fmt.Errorf("idempotency fingerprint conflict: %w", err)
	}
	r.complete("apply-idempotency-replay")

	// Another principal and another tenant must not address the primary
	// principal's cached success: their independent execution collides with
	// the existing resource instead of replaying 201.
	for _, token := range []string{r.alternateToken, r.alternateTenantToken} {
		crossResponse, err := r.requestWithToken(token, http.MethodPut, fullURL, headers, body)
		if err != nil {
			return err
		}
		if err := r.expectStableError(crossResponse, "generation_conflict"); err != nil {
			return fmt.Errorf("cross-principal idempotency isolation: %w", err)
		}
	}
	r.complete("cross-principal-idempotency-isolation")

	// A fresh create of an existing resource conflicts. The If-None-Match: *
	// fence is checked before the prepare binding, so the recorded create
	// digest is reused; a fresh create-intent prepare would itself fail closed
	// because the target already exists.
	conflictCreate, err := r.apply(kv, applyOptions{
		Create:         true,
		IdempotencyKey: "key-create-kv-again",
		PrepareDigest:  prepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(conflictCreate, "generation_conflict"); err != nil {
		return fmt.Errorf("create conflict: %w", err)
	}
	r.complete("create-conflict-when-exists")

	// The remaining probes used by later steps.
	if _, _, err := r.applyResource(mw, applyOptions{
		Create: true, IdempotencyKey: "key-create-mw",
	}, http.StatusCreated); err != nil {
		return err
	}
	queueCreated, _, err := r.applyResource(queue, applyOptions{
		Create: true, IdempotencyKey: "key-create-queue",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	r.generationTransitions = append(r.generationTransitions, "create:"+queueCreated.Metadata.Generation)
	r.revisionTransitions = append(r.revisionTransitions, "create:"+queueCreated.Metadata.Revision)
	return nil
}

func (r *v3Runner) checkGenerationFences(queue probeTarget) error {
	updated := queue
	updated.Spec = map[string]any{
		"deliveryDelaySeconds":    60,
		"messageRetentionSeconds": 600000,
	}
	resource, _, err := r.applyResource(updated, applyOptions{
		ExpectedGeneration: "1",
		IdempotencyKey:     "key-update-queue",
	}, http.StatusOK)
	if err != nil {
		return err
	}
	if resource.Metadata.Generation != "2" || resource.Metadata.Revision != "2" {
		return fmt.Errorf(
			"spec change advanced generation/revision to %s/%s, want 2/2",
			resource.Metadata.Generation, resource.Metadata.Revision,
		)
	}
	r.generationTransitions = append(r.generationTransitions, "spec-change:"+resource.Metadata.Generation)
	r.revisionTransitions = append(r.revisionTransitions, "spec-change:"+resource.Metadata.Revision)
	r.complete("update-generation-fence")
	r.complete("spec-change-bumps-generation")

	// The stale-fence apply carries a prepareDigest minted under the CURRENT
	// generation, so the rejection below is the apply fence itself.
	stalePrepared, err := r.prepareWithFence(updated, resource.Metadata.Generation)
	if err != nil {
		return err
	}
	stale, err := r.apply(updated, applyOptions{
		ExpectedGeneration: "1",
		IdempotencyKey:     "key-update-queue-stale",
		PrepareDigest:      stalePrepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(stale, "generation_conflict"); err != nil {
		return fmt.Errorf("stale generation: %w", err)
	}
	r.complete("stale-generation-rejected")
	return nil
}

// checkPrepareFences proves the prepare precondition ("generation-fence-when-
// updating") and the generation binding of the prepareDigest on a dedicated
// probe resource, leaving every other probe untouched.
func (r *v3Runner) checkPrepareFences(queue probeTarget) error {
	probe := queue
	probe.Name = "prepare-fence-probe"
	created, _, err := r.applyResource(probe, applyOptions{
		Create: true, IdempotencyKey: "key-create-prepare-fence",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	// Prepare on an existing resource without the update fence fails closed
	// before any digest is minted.
	missingFence, err := r.prepareRequest(probe, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(missingFence, "invalid_argument"); err != nil {
		return fmt.Errorf("prepare without update fence: %w", err)
	}
	// A stale fence is a generation conflict.
	staleFence, err := r.prepareRequest(probe, map[string]string{expectedGenerationHeader: "7"})
	if err != nil {
		return err
	}
	if err := r.expectStableError(staleFence, "generation_conflict"); err != nil {
		return fmt.Errorf("prepare with stale fence: %w", err)
	}
	r.complete("prepare-requires-update-fence")

	// Bind a prepare at generation N under a VALID fence...
	oldPrepared, err := r.prepareWithFence(probe, created.Metadata.Generation)
	if err != nil {
		return err
	}
	// ...perform a real update to N+1...
	updated := probe
	updated.Spec = map[string]any{
		"deliveryDelaySeconds":    30,
		"messageRetentionSeconds": 700000,
	}
	afterUpdate, _, err := r.applyResource(updated, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-update-prepare-fence",
	}, http.StatusOK)
	if err != nil {
		return err
	}
	if afterUpdate.Metadata.Generation == created.Metadata.Generation {
		return errors.New("prepare-fence probe update did not advance the generation")
	}
	// ...then apply the OLD prepareDigest under a FRESH, valid fence. Both
	// digests bind the same spec, so the invalid_argument below is exactly the
	// cross-generation prepare binding — and it fails before mutation.
	staleApply, err := r.apply(probe, applyOptions{
		ExpectedGeneration: afterUpdate.Metadata.Generation,
		IdempotencyKey:     "key-stale-prepare-apply",
		PrepareDigest:      oldPrepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(staleApply, "invalid_argument"); err != nil {
		return fmt.Errorf("stale prepareDigest apply: %w", err)
	}
	unchanged, _, err := r.read(probe)
	if err != nil {
		return err
	}
	if unchanged.Metadata.Generation != afterUpdate.Metadata.Generation ||
		unchanged.Metadata.Revision != afterUpdate.Metadata.Revision {
		return errors.New("stale prepareDigest apply mutated the resource")
	}
	staleSpecDigest, err := specCanonicalDigest(unchanged.Spec)
	if err != nil {
		return err
	}
	wantSpecDigest, err := specCanonicalDigest(updated.Spec)
	if err != nil {
		return err
	}
	if staleSpecDigest != wantSpecDigest {
		return errors.New("stale prepareDigest apply changed the desired spec")
	}
	r.complete("stale-prepare-rejected")
	return nil
}

func (r *v3Runner) checkReadETags(targets ...probeTarget) error {
	for _, target := range targets {
		if _, _, err := r.read(target); err != nil {
			return err
		}
	}
	r.complete("revision-etag-exact")
	return nil
}

// checkConditionReasonsClosed proves the portable condition reason
// vocabulary is closed. A probe that forced an out-of-enum reason would
// require the host to emit a document its own wire schema rejects, so the
// evidence is the assertion itself: every reason the host actually returns is
// held to the closed set, and the assertion is shown to reject a reason
// outside it.
func (r *v3Runner) checkConditionReasonsClosed(targets ...probeTarget) error {
	seen := 0
	for _, target := range targets {
		resource, _, err := r.read(target)
		if err != nil {
			return err
		}
		if resource.Status == nil || len(resource.Status.Conditions) == 0 {
			return fmt.Errorf("%s returned no conditions to hold to the closed vocabulary", target.Ref.Kind)
		}
		if err := verifyClosedConditionReasons(resource); err != nil {
			return err
		}
		for _, condition := range resource.Status.Conditions {
			if condition.Reason == condition.HostReason && condition.HostReason != "" {
				return errors.New("hostReason must carry host detail, not a copy of the portable reason")
			}
			seen++
		}
	}
	if seen == 0 {
		return errors.New("no condition reason evidence was collected")
	}
	// The assertion has teeth: a PascalCase reason that is merely well formed
	// is still rejected when it is outside the closed vocabulary.
	outside := wireResource{Status: &wireStatus{Conditions: []wireCondition{{
		Type: "Ready", Status: "True", Reason: "HostSpecificMeltdown",
	}}}}
	if err := verifyClosedConditionReasons(outside); err == nil {
		return errors.New("the closed condition reason assertion accepts a non-portable reason")
	}
	r.complete("condition-reason-closed")
	return nil
}

// spaceIDViolations are the exact grammar violations $defs/spaceId forbids.
// A host that accepts any of them is laxer than the wire contract, and two
// such hosts can disagree about which space a resource lives in.
func spaceIDViolations() []string {
	return []string{
		"",
		" leading-space",
		"trailing-space ",
		"\uFEFFleading-bom",
		"trailing-bom\uFEFF",
		"\u00A0leading-no-break-space",
		"embedded/slash",
		"embedded\u0001control",
		"embedded\u007Fcontrol",
		strings.Repeat("s", 256),
	}
}

// checkSpaceIDGrammar proves the host enforces the closed SpaceID grammar on
// both the query surface and the request-body surface.
func (r *v3Runner) checkSpaceIDGrammar(kv probeTarget) error {
	for _, space := range spaceIDViolations() {
		query := r.exactQuery(space, kv.Ref)
		queryResponse, err := r.request(http.MethodGet, r.resourceURL(kv.Ref, kv.Name, "", query), nil, nil)
		if err != nil {
			return err
		}
		if err := r.expectStableError(queryResponse, "invalid_argument"); err != nil {
			return fmt.Errorf("space query %q: %w", space, err)
		}
		body := kv
		body.Space = space
		document, err := encodeRunnerJSON(r.resourceBody(body))
		if err != nil {
			return err
		}
		bodyResponse, err := r.request(http.MethodPost, r.apiBase+"/resources/validate", nil, document)
		if err != nil {
			return err
		}
		if err := r.expectStableError(bodyResponse, "invalid_argument"); err != nil {
			return fmt.Errorf("space body %q: %w", space, err)
		}
	}
	// The exact contract space, which sits inside the grammar, still resolves.
	if _, _, err := r.read(kv); err != nil {
		return fmt.Errorf("SpaceID grammar enforcement rejected the contract space: %w", err)
	}
	r.complete("space-id-grammar-enforced")
	return nil
}

// checkConcurrentUnrelatedMutation proves the host does not serialize the
// whole space behind one lock: two different resources mutate at the same
// time and both succeed with distinct host-issued uids.
func (r *v3Runner) checkConcurrentUnrelatedMutation(kv, queue probeTarget) error {
	first := kv
	first.Name = "concurrent-probe-one"
	second := queue
	second.Name = "concurrent-probe-two"
	targets := []probeTarget{first, second}
	keys := []string{"key-concurrent-one", "key-concurrent-two"}
	resources := make([]wireResource, len(targets))
	failures := make([]error, len(targets))
	var group sync.WaitGroup
	for index := range targets {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			resource, _, err := r.applyResource(targets[index], applyOptions{
				Create: true, IdempotencyKey: keys[index],
			}, http.StatusCreated)
			resources[index], failures[index] = resource, err
		}(index)
	}
	group.Wait()
	for index, err := range failures {
		if err != nil {
			return fmt.Errorf("concurrent create of %s: %w", targets[index].Name, err)
		}
	}
	if resources[0].Metadata.UID == resources[1].Metadata.UID {
		return errors.New("concurrent unrelated creates shared one uid")
	}
	for index, target := range targets {
		readBack, _, err := r.read(target)
		if err != nil {
			return err
		}
		if readBack.Metadata.UID != resources[index].Metadata.UID {
			return fmt.Errorf("concurrent create of %s did not persist its uid", target.Name)
		}
	}
	r.complete("concurrent-unrelated-mutation")
	return nil
}

func (r *v3Runner) checkObserveAndStatusTouch(queue probeTarget) error {
	staleObserve, err := r.statusAction(queue, "observe", "1", "key-observe-stale", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(staleObserve, "generation_conflict"); err != nil {
		return fmt.Errorf("stale observe fence: %w", err)
	}
	observeResponse, err := r.statusAction(queue, "observe", "2", "key-observe", nil)
	if err != nil {
		return err
	}
	observed, err := decodeResourceEnvelope(observeResponse)
	if err != nil {
		return err
	}
	if err := verifyResourceIdentity(observed, queue); err != nil {
		return err
	}
	if observed.Metadata.Generation != "2" {
		return fmt.Errorf("observe changed the fenced generation to %s", observed.Metadata.Generation)
	}
	r.complete("observe-fence-exact")

	before, _, err := r.read(queue)
	if err != nil {
		return err
	}
	touchResponse, err := r.statusAction(queue, "observe", "2", "key-observe-touch", map[string]string{
		ErrorProbeHeader: ProbeTouchStatus,
	})
	if err != nil {
		return err
	}
	touched, err := decodeResourceEnvelope(touchResponse)
	if err != nil {
		return err
	}
	if touched.Metadata.Generation != before.Metadata.Generation {
		return errors.New("host-side status touch changed the desired generation")
	}
	if touched.Metadata.Revision == before.Metadata.Revision {
		return errors.New("host-side status touch did not advance the representation revision")
	}
	after, _, err := r.read(queue)
	if err != nil {
		return err
	}
	if after.Metadata.Revision != touched.Metadata.Revision ||
		after.Metadata.Generation != before.Metadata.Generation {
		return errors.New("status touch identity did not persist on read")
	}
	r.revisionTransitions = append(r.revisionTransitions, "status-touch:"+touched.Metadata.Revision)
	r.generationTransitions = append(r.generationTransitions, "status-touch:"+touched.Metadata.Generation)
	r.complete("status-change-bumps-revision-not-generation")
	return nil
}

func (r *v3Runner) checkExpectedUID(queue probeTarget) error {
	current, _, err := r.read(queue)
	if err != nil {
		return err
	}
	response, err := r.apply(queue, applyOptions{
		ExpectedGeneration: current.Metadata.Generation,
		ExpectedUID:        "uid-never-issued",
		IdempotencyKey:     "key-uid-mismatch",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "uid_mismatch"); err != nil {
		return fmt.Errorf("expectedUid mismatch: %w", err)
	}
	r.complete("expected-uid-mismatch-rejected")
	return nil
}

// checkPackageDigestNotIdentity drives a generation-fenced apply, so it runs
// against a Form that actually declares update. The applied spec is the
// resource's CURRENT desired spec, so the only thing that differs between the
// two requests is the audit-only package digest.
func (r *v3Runner) checkPackageDigestNotIdentity(queue probeTarget) error {
	// The exact read query has no packageDigest parameter: supplying one is
	// outside the closed vocabulary and fails closed.
	query := r.exactQuery(queue.Space, queue.Ref)
	query.Set("packageDigest", queue.PackageDigest)
	queryResponse, err := r.request(http.MethodGet, r.resourceURL(queue.Ref, queue.Name, "", query), nil, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(queryResponse, "invalid_argument"); err != nil {
		return fmt.Errorf("packageDigest query key: %w", err)
	}
	before, _, err := r.read(queue)
	if err != nil {
		return err
	}
	// A different audit digest in the apply body addresses the SAME resource
	// and is not echoed back as a new identity.
	substituted := queue
	substituted.Spec = before.Spec
	substituted.PackageDigest = formpackage.DigestBytes([]byte("other-legitimate-package"))
	resource, _, err := r.applyResource(substituted, applyOptions{
		ExpectedGeneration: before.Metadata.Generation,
		IdempotencyKey:     "key-package-digest-audit",
	}, http.StatusOK)
	if err != nil {
		return err
	}
	if resource.Metadata.UID != before.Metadata.UID {
		return errors.New("a substituted audit packageDigest changed the resource identity")
	}
	if resource.Form.PackageDigest == substituted.PackageDigest {
		return errors.New("host echoed a caller-substituted audit packageDigest as its own")
	}
	if resource.Metadata.Generation != before.Metadata.Generation {
		return errors.New("an audit-only digest change advanced the desired generation")
	}
	r.complete("package-digest-not-identity")
	return nil
}

func (r *v3Runner) checkSameKindTwoGroups() error {
	input := r.contract.RunnerInput
	edge := r.target(input.EdgeKvNamespace)
	edge.Name = "two-group-probe"
	other := probeTarget{
		Ref:   input.SyntheticSecondGroup,
		Name:  "two-group-probe",
		Space: input.Space,
		Spec:  map[string]any{},
	}
	edgeCreated, _, err := r.applyResource(edge, applyOptions{
		Create: true, IdempotencyKey: "key-two-group-edge",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	otherCreated, _, err := r.applyResource(other, applyOptions{
		Create: true, IdempotencyKey: "key-two-group-other",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	if edgeCreated.Metadata.UID == otherCreated.Metadata.UID {
		return errors.New("one kind name in two groups shared a uid")
	}
	edgeRead, _, err := r.read(edge)
	if err != nil {
		return err
	}
	otherRead, _, err := r.read(other)
	if err != nil {
		return err
	}
	if edgeRead.APIVersion != edge.Ref.APIVersion || otherRead.APIVersion != other.Ref.APIVersion {
		return errors.New("group-scoped reads substituted the namespaced group")
	}
	deleteResponse, err := r.deleteResource(other, otherRead.Metadata.Revision, "key-two-group-other-delete", nil)
	if err != nil {
		return err
	}
	if deleteResponse.Status != http.StatusNoContent {
		return fmt.Errorf("second-group delete HTTP %d", deleteResponse.Status)
	}
	if _, _, err := r.read(edge); err != nil {
		return fmt.Errorf("deleting the second-group resource affected the first group: %w", err)
	}
	if err := r.expectResourceAbsent(other); err != nil {
		return err
	}
	r.complete("same-kind-two-groups")
	return nil
}

// bundleManifest returns the pinned WorkerBundle artifact manifest and its
// immutable identity. The corpus already proves that identity is the probe's
// desired manifestDigest, so the runner uploads the manifest and then drives
// the resource with the digest the host returned for it.
func (r *v3Runner) bundleManifest() (map[string]any, string, error) {
	manifest := r.contract.RunnerInput.WorkerBundle.Manifest
	raw, err := encodeRunnerJSON(manifest)
	if err != nil {
		return nil, "", err
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		return nil, "", err
	}
	return manifest, digest, nil
}

func (r *v3Runner) startArtifactUpload(manifest map[string]any, key string) (string, []string, error) {
	body, err := encodeRunnerJSON(map[string]any{"manifest": manifest})
	if err != nil {
		return "", nil, err
	}
	response, err := r.request(
		http.MethodPost, r.apiBase+"/artifacts/uploads",
		map[string]string{"Idempotency-Key": key}, body,
	)
	if err != nil {
		return "", nil, err
	}
	if response.Status != http.StatusOK && response.Status != http.StatusCreated {
		return "", nil, fmt.Errorf("artifact upload start HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	var status struct {
		UploadID     string   `json:"uploadId"`
		MissingBlobs []string `json:"missingBlobs"`
	}
	if err := decodeStrictResponse(response, &status); err != nil {
		return "", nil, err
	}
	if !strings.HasPrefix(status.UploadID, "up_") || status.MissingBlobs == nil {
		return "", nil, errors.New("artifact upload status shape is invalid")
	}
	return status.UploadID, status.MissingBlobs, nil
}

func (r *v3Runner) commitArtifact(uploadID, key string) (wireResponse, error) {
	return r.request(
		http.MethodPost,
		r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID)+"/commit",
		map[string]string{"Idempotency-Key": key}, nil,
	)
}

func (r *v3Runner) checkArtifacts(bundle probeTarget) error {
	manifest, wantDigest, err := r.bundleManifest()
	if err != nil {
		return err
	}
	moduleDigest := formpackage.DigestBytes([]byte(r.contract.RunnerInput.WorkerBundle.ModuleSource))
	uploadID, missing, err := r.startArtifactUpload(manifest, "key-artifact-start")
	if err != nil {
		return err
	}
	if len(missing) != 1 || missing[0] != moduleDigest {
		return fmt.Errorf("missingBlobs = %v, want exactly the module digest", missing)
	}
	// Committing before the blob exists fails closed.
	earlyCommit, err := r.commitArtifact(uploadID, "key-artifact-early-commit")
	if err != nil {
		return err
	}
	if err := r.expectStableError(earlyCommit, "artifact_missing"); err != nil {
		return fmt.Errorf("commit before upload: %w", err)
	}
	r.complete("artifact-upload-missing-blob")

	blobURL := r.apiBase + "/artifacts/uploads/" + url.PathEscape(uploadID) + "/blobs/" + url.PathEscape(moduleDigest)
	wrongBytes, err := r.request(http.MethodPut, blobURL,
		map[string]string{"Content-Type": "application/octet-stream"},
		[]byte("these are not the declared bytes"),
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(wrongBytes, "artifact_invalid"); err != nil {
		return fmt.Errorf("blob digest mismatch: %w", err)
	}
	r.complete("artifact-digest-mismatch")

	uploaded, err := r.request(http.MethodPut, blobURL,
		map[string]string{"Content-Type": "application/octet-stream"},
		[]byte(r.contract.RunnerInput.WorkerBundle.ModuleSource),
	)
	if err != nil {
		return err
	}
	if uploaded.Status != http.StatusCreated && uploaded.Status != http.StatusNoContent {
		return fmt.Errorf("blob upload HTTP %d", uploaded.Status)
	}
	firstCommit, err := r.commitArtifact(uploadID, "key-artifact-commit")
	if err != nil {
		return err
	}
	var commitResult struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if firstCommit.Status != http.StatusOK && firstCommit.Status != http.StatusCreated {
		return fmt.Errorf("artifact commit HTTP %d; body=%s", firstCommit.Status, strings.TrimSpace(string(firstCommit.Body)))
	}
	if err := decodeStrictResponse(firstCommit, &commitResult); err != nil {
		return err
	}
	if commitResult.ManifestDigest != wantDigest {
		return fmt.Errorf(
			"commit digest %s is not the RFC 8785 canonical manifest digest %s",
			commitResult.ManifestDigest, wantDigest,
		)
	}
	secondCommit, err := r.commitArtifact(uploadID, "key-artifact-commit-again")
	if err != nil {
		return err
	}
	var secondResult struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if secondCommit.Status != http.StatusOK && secondCommit.Status != http.StatusCreated {
		return fmt.Errorf("artifact re-commit HTTP %d", secondCommit.Status)
	}
	if err := decodeStrictResponse(secondCommit, &secondResult); err != nil {
		return err
	}
	if secondResult.ManifestDigest != wantDigest {
		return errors.New("artifact re-commit was not idempotent")
	}
	manifestResponse, err := r.request(http.MethodGet, r.apiBase+"/artifacts/"+url.PathEscape(wantDigest), nil, nil)
	if err != nil {
		return err
	}
	if manifestResponse.Status != http.StatusOK {
		return fmt.Errorf("manifest read HTTP %d", manifestResponse.Status)
	}
	roundTrip, err := formpackage.DigestCanonicalJSON(manifestResponse.Body)
	if err != nil || roundTrip != wantDigest {
		return errors.New("manifest read is not content-addressed by its canonical digest")
	}
	headBlob, err := r.request(http.MethodHead, r.apiBase+"/artifacts/blobs/"+url.PathEscape(moduleDigest), nil, nil)
	if err != nil {
		return err
	}
	if headBlob.Status != http.StatusOK {
		return fmt.Errorf("blob HEAD HTTP %d", headBlob.Status)
	}
	headAbsent, err := r.request(
		http.MethodHead,
		r.apiBase+"/artifacts/blobs/"+url.PathEscape(formpackage.DigestBytes([]byte("absent-blob"))),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if headAbsent.Status != http.StatusNotFound {
		return fmt.Errorf("absent blob HEAD HTTP %d, want 404", headAbsent.Status)
	}
	r.artifactManifestDigest = wantDigest
	r.complete("artifact-commit-idempotent")

	// A bundle's desired state is the manifest digest and nothing else, so the
	// resource is applied under the digest the COMMIT returned.
	bundle.Spec = map[string]any{"manifestDigest": commitResult.ManifestDigest}
	if _, _, err := r.applyResource(bundle, applyOptions{
		Create: true, IdempotencyKey: "key-create-bundle",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("bundle apply after artifact commit: %w", err)
	}

	// A digest that names no committed manifest is not desired state a host may
	// accept: it would leave a resource whose bytes nobody ever uploaded.
	uncommitted := bundle
	uncommitted.Name = "bundle-uncommitted-probe"
	uncommitted.Spec = map[string]any{"manifestDigest": formpackage.DigestBytes([]byte("never-committed-manifest"))}
	missingResponse, err := r.apply(uncommitted, applyOptions{
		Create: true, IdempotencyKey: "key-bundle-uncommitted",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(missingResponse, "artifact_missing"); err != nil {
		return fmt.Errorf("bundle referencing an uncommitted manifest: %w", err)
	}
	if err := r.expectResourceAbsent(uncommitted); err != nil {
		return fmt.Errorf("uncommitted manifest reference mutated state: %w", err)
	}

	// A manifest digest carries WHAT the document is, not only which bytes it
	// is: a committed asset manifest is not a worker bundle.
	assetSource := "<!doctype html><title>portable-host-v3</title>\n"
	assetDigest, err := r.uploadAndCommitManifest(map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "StaticAssetBundle",
		"files": []any{map[string]any{
			"path":      "index.html",
			"mediaType": "text/html",
			"size":      len(assetSource),
			"digest":    formpackage.DigestBytes([]byte(assetSource)),
		}},
	}, map[string][]byte{
		formpackage.DigestBytes([]byte(assetSource)): []byte(assetSource),
	}, "key-artifact-asset")
	if err != nil {
		return fmt.Errorf("committing the asset manifest: %w", err)
	}
	wrongKind := bundle
	wrongKind.Name = "bundle-wrong-kind-probe"
	wrongKind.Spec = map[string]any{"manifestDigest": assetDigest}
	wrongKindResponse, err := r.apply(wrongKind, applyOptions{
		Create: true, IdempotencyKey: "key-bundle-wrong-kind",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(wrongKindResponse, "artifact_invalid"); err != nil {
		return fmt.Errorf("bundle referencing a non-WorkerBundle manifest: %w", err)
	}
	if err := r.expectResourceAbsent(wrongKind); err != nil {
		return fmt.Errorf("wrong-kind manifest reference mutated state: %w", err)
	}
	r.complete("artifact-then-bundle-apply")
	return nil
}

// uploadAndCommitManifest drives one complete upload — start, the blobs the
// host reports missing, commit — as the primary caller, and returns the
// committed manifest digest.
func (r *v3Runner) uploadAndCommitManifest(
	manifest map[string]any,
	blobs map[string][]byte,
	keyPrefix string,
) (string, error) {
	return r.uploadAndCommitManifestAs(r.token, manifest, blobs, keyPrefix)
}

// checkArtifactManifestKindExclusive proves the per-kind closure the published
// manifest schema cannot state. That schema is the structural minimum
// (spec/decisions/0014): it declares mainModule, modules, and files as
// properties for every kind, so only the host stops a manifest from carrying
// two shapes at once — and a manifest with two shapes has two meanings, which
// is precisely what a content-addressed identity must never have. A laxer host
// that accepts either mixture fails the lane here.
func (r *v3Runner) checkArtifactManifestKindExclusive() error {
	bundle := r.contract.RunnerInput.WorkerBundle
	modules, _ := bundle.Manifest["modules"].([]any)
	mainModule, _ := bundle.Manifest["mainModule"].(string)
	if len(modules) == 0 || mainModule == "" {
		return errors.New("workerBundle probe manifest declares no module")
	}
	assetSource := "<!doctype html><title>kind-exclusive</title>\n"
	assetFile := map[string]any{
		"path":      "index.html",
		"mediaType": "text/html",
		"size":      len(assetSource),
		"digest":    formpackage.DigestBytes([]byte(assetSource)),
	}
	mixtures := []struct {
		name     string
		key      string
		manifest map[string]any
	}{
		{
			name: "a WorkerBundle manifest carrying files",
			key:  "key-artifact-bundle-with-files",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "WorkerBundle",
				"mainModule": mainModule,
				"modules":    modules,
				"files":      []any{assetFile},
			},
		},
		{
			name: "a StaticAssetBundle manifest carrying modules",
			key:  "key-artifact-assets-with-modules",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "StaticAssetBundle",
				"files":      []any{assetFile},
				"modules":    modules,
			},
		},
		{
			name: "a MigrationBundle manifest carrying a main module",
			key:  "key-artifact-migration-with-main-module",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "MigrationBundle",
				"files":      []any{assetFile},
				"mainModule": mainModule,
			},
		},
	}
	for _, mixture := range mixtures {
		response, err := r.startArtifactUploadRaw(mixture.manifest, mixture.key)
		if err != nil {
			return err
		}
		if err := r.expectStableError(response, "artifact_invalid"); err != nil {
			return fmt.Errorf("%s: %w", mixture.name, err)
		}
		// A rejected manifest must not have become an immutable identity.
		raw, err := encodeRunnerJSON(mixture.manifest)
		if err != nil {
			return err
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return err
		}
		read, err := r.request(http.MethodGet, r.apiBase+"/artifacts/"+url.PathEscape(digest), nil, nil)
		if err != nil {
			return err
		}
		if err := r.expectStableError(read, "artifact_missing"); err != nil {
			return fmt.Errorf("%s became readable: %w", mixture.name, err)
		}
	}
	r.complete("artifact-manifest-kind-exclusive")
	return nil
}

// checkArtifactRetentionWhileReferenced proves the retention rule a bundle
// resource depends on: its desired state is a manifest digest, so a committed
// manifest and its blobs must stay resolvable while any resource references
// them. An unrelated upload session stages fresh bytes and is abandoned; the
// abandoned session's own bytes may be collected, but the referenced manifest
// and blob must survive it untouched.
func (r *v3Runner) checkArtifactRetentionWhileReferenced(bundle probeTarget) error {
	if _, _, err := r.read(bundle); err != nil {
		return fmt.Errorf("the referencing bundle is not readable: %w", err)
	}
	referencedBlob := formpackage.DigestBytes([]byte(r.contract.RunnerInput.WorkerBundle.ModuleSource))
	unrelatedSource := "export default { async fetch() { return new Response(\"unrelated\"); } };\n"
	unrelatedBlob := formpackage.DigestBytes([]byte(unrelatedSource))
	uploadID, missing, err := r.startArtifactUpload(map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "unrelated.js",
		"modules": []any{map[string]any{
			"name":      "unrelated.js",
			"mediaType": "application/javascript+module",
			"size":      len(unrelatedSource),
			"digest":    unrelatedBlob,
		}},
	}, "key-artifact-unrelated-start")
	if err != nil {
		return err
	}
	if len(missing) != 1 || missing[0] != unrelatedBlob {
		return fmt.Errorf("unrelated upload missingBlobs = %v, want exactly its own module", missing)
	}
	staged, err := r.request(
		http.MethodPut,
		r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID)+"/blobs/"+url.PathEscape(unrelatedBlob),
		map[string]string{"Content-Type": "application/octet-stream"}, []byte(unrelatedSource),
	)
	if err != nil {
		return err
	}
	if staged.Status != http.StatusCreated && staged.Status != http.StatusNoContent {
		return fmt.Errorf("unrelated blob upload HTTP %d", staged.Status)
	}
	abandoned, err := r.request(
		http.MethodDelete, r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID),
		map[string]string{"Idempotency-Key": "key-artifact-unrelated-abandon"}, nil,
	)
	if err != nil {
		return err
	}
	if abandoned.Status != http.StatusNoContent {
		return fmt.Errorf("upload abandon HTTP %d, want 204", abandoned.Status)
	}
	manifestResponse, err := r.request(
		http.MethodGet, r.apiBase+"/artifacts/"+url.PathEscape(r.artifactManifestDigest), nil, nil,
	)
	if err != nil {
		return err
	}
	if manifestResponse.Status != http.StatusOK {
		return fmt.Errorf("referenced manifest read HTTP %d after an unrelated abandon", manifestResponse.Status)
	}
	roundTrip, err := formpackage.DigestCanonicalJSON(manifestResponse.Body)
	if err != nil || roundTrip != r.artifactManifestDigest {
		return errors.New("the referenced manifest changed while a resource referenced it")
	}
	referenced, err := r.request(http.MethodHead, r.apiBase+"/artifacts/blobs/"+url.PathEscape(referencedBlob), nil, nil)
	if err != nil {
		return err
	}
	if referenced.Status != http.StatusOK {
		return fmt.Errorf("referenced manifest blob HEAD HTTP %d after an unrelated abandon", referenced.Status)
	}
	if _, _, err := r.read(bundle); err != nil {
		return fmt.Errorf("the referencing bundle stopped resolving: %w", err)
	}
	r.complete("artifact-retention-while-referenced")
	return nil
}

// startArtifactUploadRaw posts one manifest and returns the raw response so a
// rejected manifest can be held to its exact stable error.
func (r *v3Runner) startArtifactUploadRaw(manifest map[string]any, key string) (wireResponse, error) {
	body, err := encodeRunnerJSON(map[string]any{"manifest": manifest})
	if err != nil {
		return wireResponse{}, err
	}
	return r.request(
		http.MethodPost, r.apiBase+"/artifacts/uploads",
		map[string]string{"Idempotency-Key": key}, body,
	)
}

// checkArtifactRejectList drives the manifest reject-list items that
// spec/artifact-transport/README.md states, plus the commit-time size binding
// that an upload-time size check alone cannot cover: blobs are
// content-addressed and shared, so a later manifest can name an
// already-stored digest under a size it never had.
func (r *v3Runner) checkArtifactRejectList() error {
	bundle := r.contract.RunnerInput.WorkerBundle
	modules, _ := bundle.Manifest["modules"].([]any)
	module, _ := modules[0].(map[string]any)
	if module == nil {
		return errors.New("workerBundle probe declares no module")
	}
	mainModule, _ := bundle.Manifest["mainModule"].(string)
	moduleDigest := formpackage.DigestBytes([]byte(bundle.ModuleSource))

	// A source map whose target module is absent describes nothing.
	orphan, err := r.startArtifactUploadRaw(map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": mainModule,
		"modules": []any{module, map[string]any{
			"name":      "absent-module.js.map",
			"mediaType": "application/source-map+json",
			"size":      0,
			"digest":    formpackage.DigestBytes([]byte("orphan-source-map")),
		}},
	}, "key-artifact-orphan-source-map")
	if err != nil {
		return err
	}
	if err := r.expectStableError(orphan, "artifact_invalid"); err != nil {
		return fmt.Errorf("orphan source map: %w", err)
	}

	// A manifest whose declared module sizes overrun the host's published
	// maximumBundleBytes is rejected before any blob is accepted.
	overrun, err := r.startArtifactUploadRaw(map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "oversized.js",
		"modules": []any{map[string]any{
			"name":      "oversized.js",
			"mediaType": "application/javascript+module",
			"size":      maximumBundleBytes + 1,
			"digest":    formpackage.DigestBytes([]byte("oversized-module")),
		}},
	}, "key-artifact-overrun")
	if err != nil {
		return err
	}
	if err := r.expectStableError(overrun, "artifact_invalid"); err != nil {
		return fmt.Errorf("published limit overrun: %w", err)
	}
	r.complete("artifact-manifest-reject-list")

	// The blob is already stored from the committed manifest, so this upload
	// needs no bytes at all: only commit can catch the lie.
	mislabelled := cloneJSONMap(module)
	mislabelled["size"] = len(bundle.ModuleSource) + 1
	uploadID, missing, err := r.startArtifactUpload(map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": mainModule,
		"modules":    []any{mislabelled},
	}, "key-artifact-size-lie")
	if err != nil {
		return err
	}
	if len(missing) != 0 {
		return fmt.Errorf("missingBlobs = %v, want none for an already-stored blob", missing)
	}
	commit, err := r.commitArtifact(uploadID, "key-artifact-size-lie-commit")
	if err != nil {
		return err
	}
	if err := r.expectStableError(commit, "artifact_invalid"); err != nil {
		return fmt.Errorf("commit-time declared size binding: %w", err)
	}
	// The honest manifest and its blob are untouched by the rejected commit.
	headBlob, err := r.request(http.MethodHead, r.apiBase+"/artifacts/blobs/"+url.PathEscape(moduleDigest), nil, nil)
	if err != nil {
		return err
	}
	if headBlob.Status != http.StatusOK {
		return fmt.Errorf("rejected commit disturbed the stored blob: HEAD HTTP %d", headBlob.Status)
	}
	r.complete("artifact-commit-binds-declared-size")
	return nil
}

// exactReference builds the exact three-member reference addressing one probe
// Form. The group and kind travel on the wire; only the name is authored.
func exactReference(target probeTarget, name string) map[string]any {
	return map[string]any{
		"apiVersion": target.Ref.APIVersion,
		"kind":       target.Ref.Kind,
		"name":       name,
	}
}

func (r *v3Runner) checkWorkerVersionFlow(version, kv probeTarget) error {
	// A NON-binding reference to a missing resource fails before mutation, and
	// stores nothing. This is the reference class a name-only "resource" scan
	// never saw: worker is a plain cross-resource reference, not a binding.
	missingWorker := version
	missingWorker.Name = "relation-missing-version"
	missingWorker.Spec = cloneJSONMap(version.Spec)
	missingWorker.Spec["worker"] = map[string]any{
		"apiVersion": version.Ref.APIVersion,
		"kind":       "ModuleWorker",
		"name":       "absent-worker",
	}

	missingWorkerResponse, err := r.apply(missingWorker, applyOptions{
		Create: true, IdempotencyKey: "key-version-missing-worker",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(missingWorkerResponse, "resource_not_found"); err != nil {
		return fmt.Errorf("missing non-binding relation target: %w", err)
	}
	if err := r.expectResourceAbsent(missingWorker); err != nil {
		return fmt.Errorf("missing non-binding relation target mutated state: %w", err)
	}
	r.complete("relation-target-missing-rejected")

	// A typed binding to a missing namespace fails before mutation.
	missingTarget := version
	missingTarget.Spec = cloneJSONMap(version.Spec)
	missingTarget.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(kv, "absent-kv"),
	}}
	response, err := r.apply(missingTarget, applyOptions{
		Create: true, IdempotencyKey: "key-version-missing-binding",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "resource_not_found"); err != nil {
		return fmt.Errorf("missing binding target: %w", err)
	}
	if err := r.expectResourceAbsent(version); err != nil {
		return fmt.Errorf("missing binding target mutated state: %w", err)
	}
	r.complete("binding-target-missing-404-before-mutation")

	if _, _, err := r.applyResource(version, applyOptions{
		Create: true, IdempotencyKey: "key-create-version",
	}, http.StatusCreated); err != nil {
		return err
	}

	// WorkerVersion is a revision-role Form: update is not representable.
	update, err := r.apply(version, applyOptions{
		ExpectedGeneration: "1",
		IdempotencyKey:     "key-version-update",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(update, "invalid_argument"); err != nil {
		return fmt.Errorf("revision-role update: %w", err)
	}
	r.complete("revision-role-update-rejected")

	// The capability rule, independent of the role rule above: this Form's
	// Definition omits update, so a SPEC-CHANGING apply is refused before any
	// mutation and the stored identity does not move.
	if err := r.checkNoUpdateSpecChangeRejected(version); err != nil {
		return err
	}

	// Deleting the bound KV namespace while the version references it fails.
	kvCurrent, _, err := r.read(kv)
	if err != nil {
		return err
	}
	boundDelete, err := r.deleteResource(kv, kvCurrent.Metadata.Revision, "key-kv-bound-delete", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(boundDelete, "dependency_in_use"); err != nil {
		return fmt.Errorf("bound target delete: %w", err)
	}
	if _, _, err := r.read(kv); err != nil {
		return fmt.Errorf("dependency_in_use delete mutated state: %w", err)
	}
	r.complete("dependency-in-use-on-bound-target-delete")
	return nil
}

// checkBindingContractVerified proves a host does not take a declared binding
// on trust. The probe points a typed kvBindings entry at the synthetic
// second-group EdgeKVNamespace: an INSTALLED Form, of the same kind name, in a
// different group — which the binding does not list in allowedTargetForms and
// which provides no Interface at all. A conforming host refuses it before any
// mutation, whether it refuses at the reference's pinned apiVersion constant or
// at the binding-contract verification behind it.
//
// The check also proves the host serves the two facts that verification needs:
// the Binding contract each binding list carries, stated on the served desired
// schema, and a support profile for that exact contract.
func (r *v3Runner) checkBindingContractVerified(version probeTarget) error {
	input := r.contract.RunnerInput
	definition, err := r.formDefinition(version.Ref)
	if err != nil {
		return err
	}
	properties, _ := definition.DesiredSchema["properties"].(map[string]any)
	bindingList, _ := properties["kvBindings"].(map[string]any)
	if bindingList == nil {
		return errors.New("the served WorkerVersion desiredSchema declares no kvBindings property")
	}
	contractName, _ := bindingList[currentformmodel.BindingAnnotationKey].(string)
	if contractName != input.SupportProbes.Binding.Name {
		return fmt.Errorf(
			"kvBindings declares binding contract %q, want the pinned %q; without it a host cannot know "+
				"which Binding Definition governs the references it finds",
			contractName, input.SupportProbes.Binding.Name,
		)
	}
	profile, err := r.request(
		http.MethodGet,
		r.apiBase+"/support/bindings/"+url.PathEscape(contractName)+"/"+
			url.PathEscape(input.SupportProbes.Binding.Version),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if profile.Status != http.StatusOK {
		return fmt.Errorf("binding support profile HTTP %d for the contract kvBindings declares", profile.Status)
	}

	// The out-of-contract target exists, so the refusal cannot be "absent".
	foreign := probeTarget{
		Ref:   input.SyntheticSecondGroup,
		Name:  "binding-contract-kv",
		Space: input.Space,
		Spec:  map[string]any{},
	}
	if _, _, err := r.applyResource(foreign, applyOptions{
		Create: true, IdempotencyKey: "key-binding-contract-foreign",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("second-group binding target: %w", err)
	}
	offender := version
	offender.Name = "binding-contract-version"
	offender.Spec = cloneJSONMap(version.Spec)
	offender.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(foreign, foreign.Name),
	}}
	// The refusal may land at prepare or at apply; both are before any
	// mutation, which is the property under test. A host that admits the
	// document all the way to apply must still refuse there.
	response, err := r.prepareRequest(offender, nil)
	if err != nil {
		return err
	}
	if response.Status == http.StatusOK {
		var prepared struct {
			Review struct {
				PrepareDigest string `json:"prepareDigest"`
			} `json:"review"`
		}
		if err := decodeStrictResponse(response, &prepared); err != nil {
			return err
		}
		response, err = r.apply(offender, applyOptions{
			Create:         true,
			IdempotencyKey: "key-binding-contract-offender",
			PrepareDigest:  prepared.Review.PrepareDigest,
		})
		if err != nil {
			return err
		}
	}
	switch response.Status {
	case http.StatusBadRequest:
		if err := r.expectStableError(response, "invalid_argument"); err != nil {
			return fmt.Errorf("binding whose target Form provides no Interface: %w", err)
		}
	case http.StatusUnprocessableEntity:
		if err := r.expectStableError(response, "unsupported_capability"); err != nil {
			return fmt.Errorf("binding whose target Form provides no Interface: %w", err)
		}
	default:
		return fmt.Errorf(
			"binding to a Form outside the contract HTTP %d, want a pre-mutation refusal; body=%s",
			response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	if err := r.expectResourceAbsent(offender); err != nil {
		return fmt.Errorf("an unverifiable binding still mutated state: %w", err)
	}
	r.complete("binding-contract-verified")
	return nil
}

// checkRelationIncarnationChange proves the rule that makes storing a UID
// worth anything: a target deleted and recreated under the same name is a
// DIFFERENT resource, and the source that referenced the old one is not
// silently re-bound to the new one.
//
// The delete uses the out-of-band probe because relation protection otherwise
// makes this state unreachable through the API — which is the point: the state
// arises when a backend loses a resource, not when a client asks for it.
func (r *v3Runner) checkRelationIncarnationChange(version, kv probeTarget) error {
	target := kv
	target.Name = "incarnation-kv"
	created, _, err := r.applyResource(target, applyOptions{
		Create: true, IdempotencyKey: "key-incarnation-kv",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	source := version
	source.Name = "incarnation-version"
	source.Spec = cloneJSONMap(version.Spec)
	source.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(target, target.Name),
	}}
	bound, _, err := r.applyResource(source, applyOptions{
		Create: true, IdempotencyKey: "key-incarnation-version",
	}, http.StatusCreated)
	if err != nil {
		return err
	}

	// The target vanishes out of band. The source must report it rather than
	// carry on as if nothing happened.
	removed, err := r.deleteResource(
		target, created.Metadata.Revision, "key-incarnation-kv-delete",
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
	missing, err := r.readRaw(source)
	if err != nil {
		return err
	}
	if err := requireNotReady(missing, "DependencyMissing"); err != nil {
		return fmt.Errorf("source of a vanished target: %w", err)
	}

	// The name comes back on a NEW incarnation.
	recreated, _, err := r.applyResource(target, applyOptions{
		Create: true, IdempotencyKey: "key-incarnation-kv-recreate",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	if recreated.Metadata.UID == created.Metadata.UID {
		return errors.New("the recreated target reused its uid, so no incarnation changed")
	}
	changed, err := r.readRaw(source)
	if err != nil {
		return err
	}
	if err := requireNotReady(changed, "ExternalChange"); err != nil {
		return fmt.Errorf("source of a recreated target: %w", err)
	}
	hostReason := readyCondition(changed).HostReason
	for _, want := range []string{"/kvBindings/0/resource", created.Metadata.UID, recreated.Metadata.UID} {
		if !strings.Contains(hostReason, want) {
			return fmt.Errorf(
				"ExternalChange hostReason %q must name the relation pointer and both uids (missing %q)",
				hostReason, want,
			)
		}
	}
	if changed.Metadata.Generation != bound.Metadata.Generation {
		return errors.New("an unre-applied source changed generation, so the host altered desired state")
	}
	// Reading again must not heal it: only a re-apply re-resolves the name.
	again, err := r.readRaw(source)
	if err != nil {
		return err
	}
	if err := requireNotReady(again, "ExternalChange"); err != nil {
		return fmt.Errorf("a second read silently re-bound the source: %w", err)
	}
	r.complete("relation-incarnation-change-detected")
	return nil
}

// checkRelationReapplyRepins proves the remedy the incarnation rule leaves
// open. A host never re-binds a reference by itself, so without this rule the
// only way out of an ExternalChange would be editing state by hand: the
// desired spec is unchanged, so nothing about it can express "resolve this
// again".
//
// The rule is that ACCEPTING an apply is what re-resolves and re-pins, even
// when the spec is byte-identical. Re-pinning is host-owned bookkeeping, so it
// must not touch generation — the client's desired state did not change — but
// it does change the representation, because the Ready condition stops
// reporting the drift, so revision must move (spec/decisions/0011 and 0015).
// Applying once more after that moves nothing at all: the remedy is
// idempotent, not a per-apply revision treadmill.
func (r *v3Runner) checkRelationReapplyRepins() error {
	input := r.contract.RunnerInput
	// Dedicated resources, so the probes other checks drive are untouched.
	queue := r.target(input.AtLeastOnceQueue)
	queue.Name = "repin-queue"
	created, _, err := r.applyResource(queue, applyOptions{
		Create: true, IdempotencyKey: "key-repin-queue",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("re-pin probe queue: %w", err)
	}
	consumer := r.target(input.QueueConsumer)
	consumer.Name = "repin-consumer"
	consumer.Spec = cloneJSONMap(consumer.Spec)
	consumer.Spec["queue"] = exactReference(queue, queue.Name)
	bound, _, err := r.applyResource(consumer, applyOptions{
		Create: true, IdempotencyKey: "key-repin-consumer",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("re-pin probe consumer: %w", err)
	}
	if !hasCapability(consumer.Lifecycle, "update") {
		return errors.New("the re-pin probe needs a Form that declares an in-place update")
	}

	// The queue is destroyed out of band and the name comes back on a new
	// incarnation, exactly the state the incarnation rule reports.
	removed, err := r.deleteResource(
		queue, created.Metadata.Revision, "key-repin-queue-delete",
		map[string]string{ErrorProbeHeader: ProbeExternalChange},
	)
	if err != nil {
		return err
	}
	if removed.Status != http.StatusNoContent {
		return fmt.Errorf(
			"out-of-band queue delete HTTP %d, want 204; body=%s",
			removed.Status, strings.TrimSpace(string(removed.Body)),
		)
	}
	if _, _, err := r.applyResource(queue, applyOptions{
		Create: true, IdempotencyKey: "key-repin-queue-recreate",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("re-created queue: %w", err)
	}
	drifted, err := r.readRaw(consumer)
	if err != nil {
		return err
	}
	if err := requireNotReady(drifted, "ExternalChange"); err != nil {
		return fmt.Errorf("consumer of a recreated queue: %w", err)
	}

	// The same desired spec, applied again, is the whole remedy.
	repinned, _, err := r.applyResource(consumer, applyOptions{
		ExpectedGeneration: bound.Metadata.Generation,
		IdempotencyKey:     "key-repin-consumer-reapply",
	}, http.StatusOK)
	if err != nil {
		return fmt.Errorf("spec-identical re-apply of a drifted source: %w", err)
	}
	if repinned.Metadata.Generation != bound.Metadata.Generation {
		return fmt.Errorf(
			"a spec-identical re-apply moved generation %s -> %s; re-pinning is not a desired-state change",
			bound.Metadata.Generation, repinned.Metadata.Generation,
		)
	}
	if repinned.Metadata.Revision == drifted.Metadata.Revision {
		return fmt.Errorf(
			"re-pinning left revision at %s while the Ready condition changed; the representation moved",
			repinned.Metadata.Revision,
		)
	}

	// And it is idempotent: with nothing left to re-pin, nothing moves.
	again, _, err := r.applyResource(consumer, applyOptions{
		ExpectedGeneration: repinned.Metadata.Generation,
		IdempotencyKey:     "key-repin-consumer-again",
	}, http.StatusOK)
	if err != nil {
		return err
	}
	if again.Metadata.Generation != repinned.Metadata.Generation ||
		again.Metadata.Revision != repinned.Metadata.Revision {
		return fmt.Errorf(
			"a second identical apply moved identity %s/%s -> %s/%s",
			repinned.Metadata.Generation, repinned.Metadata.Revision,
			again.Metadata.Generation, again.Metadata.Revision,
		)
	}
	// The consumer is released again. It is an attachment of the shared worker,
	// so leaving it live would keep that worker's deployment undeletable
	// (spec/decisions/0016) and turn this check into a precondition of the
	// relation-deletion checks that follow.
	released, err := r.deleteResource(consumer, again.Metadata.Revision, "key-repin-consumer-delete", nil)
	if err != nil {
		return err
	}
	if released.Status != http.StatusNoContent {
		return fmt.Errorf(
			"releasing the re-pin probe consumer HTTP %d, want 204; body=%s",
			released.Status, strings.TrimSpace(string(released.Body)),
		)
	}
	r.complete("relation-reapply-repins")
	return nil
}

// hasCapability reports whether a pinned lifecycle set carries one capability.
func hasCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

// checkRelationDeletionProtection proves dependency protection covers every
// relation, not only typed bindings: a Worker Version pinned by a deployment
// and a bundle executed by a version are both live dependencies.
func (r *v3Runner) checkRelationDeletionProtection(version, bundle probeTarget) error {
	current, _, err := r.read(version)
	if err != nil {
		return err
	}
	blocked, err := r.deleteResource(version, current.Metadata.Revision, "key-version-relation-delete", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(blocked, "dependency_in_use"); err != nil {
		return fmt.Errorf("deleting a WorkerVersion a WorkerDeployment weights: %w", err)
	}
	if _, _, err := r.read(version); err != nil {
		return fmt.Errorf("a refused delete removed the resource: %w", err)
	}
	bundleCurrent, _, err := r.read(bundle)
	if err != nil {
		return err
	}
	blockedBundle, err := r.deleteResource(bundle, bundleCurrent.Metadata.Revision, "key-bundle-relation-delete", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(blockedBundle, "dependency_in_use"); err != nil {
		return fmt.Errorf("deleting a WorkerBundle a WorkerVersion executes: %w", err)
	}
	// Removing the holder releases the dependency; the deployment itself is
	// referenced by nothing, so it deletes cleanly.
	deployment := r.target(r.contract.RunnerInput.WorkerDeployment)
	deploymentCurrent, _, err := r.read(deployment)
	if err != nil {
		return err
	}
	released, err := r.deleteResource(
		deployment, deploymentCurrent.Metadata.Revision, "key-deployment-delete", nil,
	)
	if err != nil {
		return err
	}
	if released.Status != http.StatusNoContent {
		return fmt.Errorf(
			"deleting an unreferenced WorkerDeployment HTTP %d, want 204; body=%s",
			released.Status, strings.TrimSpace(string(released.Body)),
		)
	}
	r.complete("relation-target-deletion-blocked")
	return nil
}

// checkNoUpdateSpecChangeRejected proves the capability contract in the
// direction that matters: a Form Definition whose lifecycleCapabilities omit
// update must refuse a spec-changing apply to an existing resource, and must
// refuse it BEFORE mutating anything. A host that quietly performed the update
// would make the advertised capability set a lie, and clients that planned a
// replacement around the missing capability would diverge from the host.
func (r *v3Runner) checkNoUpdateSpecChangeRejected(target probeTarget) error {
	for _, capability := range target.Lifecycle {
		if capability == "update" {
			return fmt.Errorf(
				"no-update probe %s declares update; this check needs a Form whose Definition omits it",
				target.Ref.Kind,
			)
		}
	}
	before, _, err := r.read(target)
	if err != nil {
		return err
	}
	changed := target
	changed.Spec = cloneJSONMap(target.Spec)
	// Any portable spec change will do; `vars` is chosen because it is optional,
	// defaulted, and touches no relation, so the refusal is unambiguously about
	// the missing update capability and not about a dangling reference.
	changed.Spec["vars"] = map[string]any{"LOG_LEVEL": "debug"}
	response, err := r.apply(changed, applyOptions{
		ExpectedGeneration: before.Metadata.Generation,
		IdempotencyKey:     "key-no-update-spec-change",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("spec-changing apply to a Form without update: %w", err)
	}
	after, _, err := r.read(target)
	if err != nil {
		return err
	}
	if after.Metadata.Generation != before.Metadata.Generation ||
		after.Metadata.Revision != before.Metadata.Revision {
		return fmt.Errorf(
			"a refused spec change moved identity from generation %s revision %s to %s/%s",
			before.Metadata.Generation, before.Metadata.Revision,
			after.Metadata.Generation, after.Metadata.Revision,
		)
	}
	afterDigest, err := specCanonicalDigest(after.Spec)
	if err != nil {
		return err
	}
	beforeDigest, err := specCanonicalDigest(before.Spec)
	if err != nil {
		return err
	}
	if afterDigest != beforeDigest {
		return errors.New("a refused spec change still altered the stored desired spec")
	}
	r.complete("no-update-spec-change-rejected")
	return nil
}

// checkDeploymentWeightSum proves the WorkerDeployment rule the Form
// description calls host-validated: a JSON Schema cannot add numbers, so the
// exact 10000 basis-point sum has to be enforced by the host before mutation.
//
// It also records the state transition the rest of the aggregate depends on:
// creating the deployment is what makes the Module Worker Ready, because a
// worker's readiness is a claim about service and nothing serves until a
// deployment selects the versions that do (spec/decisions/0016).
func (r *v3Runner) checkDeploymentWeightSum() error {
	deployment := r.target(r.contract.RunnerInput.WorkerDeployment)
	versions, _ := deployment.Spec["versions"].([]any)
	entry, _ := versions[0].(map[string]any)
	if entry == nil {
		return errors.New("workerDeployment probe declares no weighted version")
	}
	underweight := cloneJSONMap(entry)
	underweight["weight"] = 9999
	short := deployment
	short.Spec = cloneJSONMap(deployment.Spec)
	short.Spec["versions"] = []any{underweight}
	response, err := r.apply(short, applyOptions{
		Create: true, IdempotencyKey: "key-deployment-underweight",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("weights summing to 9999: %w", err)
	}
	if err := r.expectResourceAbsent(deployment); err != nil {
		return fmt.Errorf("rejected weight sum mutated state: %w", err)
	}
	if _, _, err := r.applyResource(deployment, applyOptions{
		Create: true, IdempotencyKey: "key-deployment-exact",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("weights summing to exactly 10000: %w", err)
	}
	worker, err := r.readRaw(r.target(r.contract.RunnerInput.ModuleWorker))
	if err != nil {
		return err
	}
	if condition := readyCondition(worker); condition.Status != "True" {
		return fmt.Errorf(
			"the worker reports Ready=%s/%s once its deployment serves the fetch handler, want True",
			condition.Status, condition.Reason,
		)
	}
	r.complete("deployment-weight-sum-enforced")
	return nil
}

// checkImportFlows proves adoption is a real mutation: it mints identity like
// a create, and it passes the same pre-mutation gauntlet as apply instead of
// bypassing validation.
func (r *v3Runner) checkImportFlows(kv, version probeTarget) error {
	adopted := kv
	adopted.Name = "import-probe"
	response, err := r.importResource(adopted, importOptions{
		NativeID: "native/import-probe", IdempotencyKey: "key-import-adopt", Create: true,
	})
	if err != nil {
		return err
	}
	resource, err := decodeResource(response, http.StatusCreated)
	if err != nil {
		return err
	}
	if err := verifyResourceIdentity(resource, adopted); err != nil {
		return err
	}
	if !uidPattern.MatchString(resource.Metadata.UID) {
		return fmt.Errorf("import did not mint a grammar-valid host-issued uid, got %q", resource.Metadata.UID)
	}
	if resource.Metadata.Generation != "1" || resource.Metadata.Revision != "1" {
		return fmt.Errorf(
			"import identity = generation %s revision %s, want 1/1",
			resource.Metadata.Generation, resource.Metadata.Revision,
		)
	}
	if err := verifyRevisionETag(response, resource.Metadata.Revision); err != nil {
		return err
	}
	readBack, _, err := r.read(adopted)
	if err != nil {
		return err
	}
	if readBack.Metadata.UID != resource.Metadata.UID {
		return errors.New("the adopted resource is not readable under its minted uid")
	}
	r.uidTransitions = append(r.uidTransitions, "import:"+resource.Metadata.UID)
	r.complete("import-adopts-native-resource")

	// The same typed binding an apply rejects before mutation is rejected by
	// import before mutation, and stores nothing.
	invalid := version
	invalid.Name = "import-invalid-probe"
	invalid.Spec = cloneJSONMap(version.Spec)
	invalid.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(kv, "absent-kv"),
	}}
	rejected, err := r.importResource(invalid, importOptions{
		NativeID: "native/import-invalid-probe", IdempotencyKey: "key-import-invalid", Create: true,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(rejected, "resource_not_found"); err != nil {
		return fmt.Errorf("import with a missing binding target: %w", err)
	}
	if err := r.expectResourceAbsent(invalid); err != nil {
		return fmt.Errorf("rejected import stored state: %w", err)
	}
	r.complete("import-validates-like-apply")
	return nil
}

func (r *v3Runner) checkDeleteFences(version, kv probeTarget) error {
	current, _, err := r.read(version)
	if err != nil {
		return err
	}
	stale, err := r.deleteResource(version, current.Metadata.Revision+"9", "key-version-delete-stale", nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(stale, "revision_conflict"); err != nil {
		return fmt.Errorf("stale delete revision: %w", err)
	}
	r.complete("stale-revision-rejected")

	deleted, err := r.deleteResource(version, current.Metadata.Revision, "key-version-delete", nil)
	if err != nil {
		return err
	}
	if deleted.Status != http.StatusNoContent {
		return fmt.Errorf("delete HTTP %d, want 204", deleted.Status)
	}
	if err := r.expectResourceAbsent(version); err != nil {
		return err
	}
	r.complete("delete-revision-fence")

	// Delete and re-create of the same name must mint a NEW uid.
	kvCurrent, _, err := r.read(kv)
	if err != nil {
		return err
	}
	kvDeleted, err := r.deleteResource(kv, kvCurrent.Metadata.Revision, "key-kv-delete", nil)
	if err != nil {
		return err
	}
	if kvDeleted.Status != http.StatusNoContent {
		return fmt.Errorf("kv delete HTTP %d, want 204", kvDeleted.Status)
	}
	recreated, _, err := r.applyResource(kv, applyOptions{
		Create: true, IdempotencyKey: "key-recreate-kv",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	if recreated.Metadata.UID == kvCurrent.Metadata.UID {
		return errors.New("delete and re-create returned the same uid")
	}
	r.uidTransitions = append(r.uidTransitions, "recreate:"+recreated.Metadata.UID)
	r.complete("delete-then-recreate-uid-changes")
	return nil
}

type wireOperation struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	ID         string         `json:"id"`
	Done       bool           `json:"done"`
	Target     map[string]any `json:"target,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Error      map[string]any `json:"error,omitempty"`
}

func (r *v3Runner) checkOperations(kv probeTarget) error {
	asyncTarget := kv
	asyncTarget.Name = "async-probe"
	accepted, err := r.apply(asyncTarget, applyOptions{
		Create:         true,
		IdempotencyKey: "key-async-create",
		ExtraHeaders:   map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	if accepted.Status != http.StatusAccepted {
		return fmt.Errorf("async apply HTTP %d, want 202", accepted.Status)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(accepted, &envelope); err != nil {
		return err
	}
	operation := envelope.Operation
	if operation.APIVersion != "operations.takoform.com/v1alpha1" || operation.Kind != "Operation" ||
		!strings.HasPrefix(operation.ID, "op_") || operation.Done {
		return fmt.Errorf("202 operation envelope is invalid: %+v", operation)
	}
	operationURL := r.apiBase + "/operations/" + url.PathEscape(operation.ID)
	var terminal wireOperation
	var terminalBody []byte
	sawRetryAfter := false
	for poll := 0; poll < asyncOperationPolls+2; poll++ {
		pollResponse, err := r.request(http.MethodGet, operationURL, nil, nil)
		if err != nil {
			return err
		}
		if pollResponse.Status != http.StatusOK {
			return fmt.Errorf("operation poll HTTP %d", pollResponse.Status)
		}
		var polled wireOperation
		if err := decodeStrictResponse(pollResponse, &polled); err != nil {
			return err
		}
		if polled.ID != operation.ID {
			return errors.New("operation id is not stable across polls")
		}
		if !polled.Done {
			if pollResponse.Header.Get("Retry-After") == "" {
				return errors.New("pending operation response omitted Retry-After")
			}
			sawRetryAfter = true
			continue
		}
		terminal = polled
		terminalBody = pollResponse.Body
		break
	}
	if !terminal.Done || !sawRetryAfter {
		return errors.New("async operation did not complete after honored Retry-After polling")
	}
	if terminal.Error != nil || terminal.Result == nil {
		return errors.New("terminal operation must carry exactly the result")
	}
	resultResource, err := encodeRunnerJSON(terminal.Result["resource"])
	if err != nil {
		return err
	}
	var resource wireResource
	if err := formpackage.DecodeStrictIJSON(resultResource, &resource); err != nil {
		return fmt.Errorf("terminal result resource: %w", err)
	}
	if err := verifyResourceIdentity(resource, asyncTarget); err != nil {
		return err
	}
	if _, _, err := r.read(asyncTarget); err != nil {
		return fmt.Errorf("async-created resource is not readable: %w", err)
	}
	r.asyncOperationID = operation.ID
	r.complete("async-operation-flow")

	replay, err := r.request(http.MethodGet, operationURL, nil, nil)
	if err != nil {
		return err
	}
	if replay.Status != http.StatusOK || !bytes.Equal(replay.Body, terminalBody) {
		return errors.New("terminal operation replay was not byte-identical")
	}
	r.complete("operation-replay-terminal")

	cancelTarget := kv
	cancelTarget.Name = "cancel-probe"
	cancelAccepted, err := r.apply(cancelTarget, applyOptions{
		Create:         true,
		IdempotencyKey: "key-cancel-create",
		ExtraHeaders:   map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	if cancelAccepted.Status != http.StatusAccepted {
		return fmt.Errorf("async cancel-probe apply HTTP %d, want 202", cancelAccepted.Status)
	}
	var cancelEnvelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(cancelAccepted, &cancelEnvelope); err != nil {
		return err
	}
	cancelResponse, err := r.request(
		http.MethodPost,
		r.apiBase+"/operations/"+url.PathEscape(cancelEnvelope.Operation.ID)+"/cancel",
		map[string]string{"Idempotency-Key": "key-cancel-operation"},
		nil,
	)
	if err != nil {
		return err
	}
	if cancelResponse.Status != http.StatusOK {
		return fmt.Errorf("operation cancel HTTP %d", cancelResponse.Status)
	}
	var cancelled wireOperation
	if err := decodeStrictResponse(cancelResponse, &cancelled); err != nil {
		return err
	}
	if !cancelled.Done || cancelled.Result != nil || cancelled.Error == nil ||
		cancelled.Error["code"] != "operation_cancelled" {
		return fmt.Errorf("cancelled operation shape is invalid: %+v", cancelled)
	}
	if err := r.expectResourceAbsent(cancelTarget); err != nil {
		return fmt.Errorf("cancelled operation still mutated state: %w", err)
	}
	unknown, err := r.request(http.MethodGet, r.apiBase+"/operations/op_absent", nil, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(unknown, "operation_not_found"); err != nil {
		return fmt.Errorf("unknown operation: %w", err)
	}
	r.cancelledOperationID = cancelEnvelope.Operation.ID
	r.complete("operation-cancel")
	return nil
}

// pollOperation drives one accepted operation to its terminal document.
func (r *v3Runner) pollOperation(id string) (wireOperation, error) {
	operationURL := r.apiBase + "/operations/" + url.PathEscape(id)
	for poll := 0; poll < asyncOperationPolls+2; poll++ {
		response, err := r.request(http.MethodGet, operationURL, nil, nil)
		if err != nil {
			return wireOperation{}, err
		}
		if response.Status != http.StatusOK {
			return wireOperation{}, fmt.Errorf("operation poll HTTP %d", response.Status)
		}
		var polled wireOperation
		if err := decodeStrictResponse(response, &polled); err != nil {
			return wireOperation{}, err
		}
		if polled.ID != id {
			return wireOperation{}, errors.New("operation id is not stable across polls")
		}
		if polled.Done {
			return polled, nil
		}
	}
	return wireOperation{}, errors.New("operation never reached a terminal state")
}

// checkAsyncCommitRevalidates proves a 202 is an acceptance, not a decision:
// the preconditions an accepted mutation was admitted under are re-derived at
// COMMIT time. Here the typed binding target is removed by a second request
// while the operation is still pending, so the operation must terminate with
// resource_not_found and commit nothing.
func (r *v3Runner) checkAsyncCommitRevalidates(kv, version probeTarget) error {
	target := kv
	target.Name = "revalidate-kv-probe"
	created, _, err := r.applyResource(target, applyOptions{
		Create: true, IdempotencyKey: "key-revalidate-kv",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	pending := version
	pending.Name = "revalidate-version-probe"
	pending.Spec = cloneJSONMap(version.Spec)
	pending.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(target, target.Name),
	}}
	accepted, err := r.apply(pending, applyOptions{
		Create:         true,
		IdempotencyKey: "key-revalidate-async",
		ExtraHeaders:   map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	if accepted.Status != http.StatusAccepted {
		return fmt.Errorf(
			"async apply HTTP %d, want 202; body=%s",
			accepted.Status, strings.TrimSpace(string(accepted.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(accepted, &envelope); err != nil {
		return err
	}
	if envelope.Operation.Done {
		return errors.New("the 202 operation was already terminal, so nothing could be revalidated")
	}
	// A second request removes the binding target while the accepted apply is
	// still pending. Nothing holds it: the pending resource is not committed.
	deleted, err := r.deleteResource(target, created.Metadata.Revision, "key-revalidate-kv-delete", nil)
	if err != nil {
		return err
	}
	if deleted.Status != http.StatusNoContent {
		return fmt.Errorf(
			"binding-target delete HTTP %d, want 204; body=%s",
			deleted.Status, strings.TrimSpace(string(deleted.Body)),
		)
	}
	terminal, err := r.pollOperation(envelope.Operation.ID)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(terminal, "resource_not_found"); err != nil {
		return err
	}
	if err := r.expectResourceAbsent(pending); err != nil {
		return fmt.Errorf("a revalidated-away operation still committed: %w", err)
	}
	if err := r.expectResourceAbsent(target); err != nil {
		return fmt.Errorf("the deleted binding target reappeared: %w", err)
	}
	// The same rule holds in the other direction: an accepted DELETE re-runs
	// the live-binding scan at commit time, so a resource that acquired a
	// holder while the operation was pending survives.
	held := kv
	held.Name = "revalidate-held-probe"
	heldCreated, _, err := r.applyResource(held, applyOptions{
		Create: true, IdempotencyKey: "key-revalidate-held",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	acceptedDelete, err := r.deleteResource(
		held, heldCreated.Metadata.Revision, "key-revalidate-held-delete",
		map[string]string{ErrorProbeHeader: ProbeAsync},
	)
	if err != nil {
		return err
	}
	if acceptedDelete.Status != http.StatusAccepted {
		return fmt.Errorf(
			"async delete HTTP %d, want 202; body=%s",
			acceptedDelete.Status, strings.TrimSpace(string(acceptedDelete.Body)),
		)
	}
	var deleteEnvelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(acceptedDelete, &deleteEnvelope); err != nil {
		return err
	}
	holder := version
	holder.Name = "revalidate-holder-version"
	holder.Spec = cloneJSONMap(version.Spec)
	holder.Spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": exactReference(held, held.Name),
	}}
	if _, _, err := r.applyResource(holder, applyOptions{
		Create: true, IdempotencyKey: "key-revalidate-holder",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("binding holder created while the delete was pending: %w", err)
	}
	terminalDelete, err := r.pollOperation(deleteEnvelope.Operation.ID)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(terminalDelete, "dependency_in_use"); err != nil {
		return fmt.Errorf("an accepted delete whose target acquired a holder: %w", err)
	}
	if _, _, err := r.read(held); err != nil {
		return fmt.Errorf("an accepted delete removed a resource that acquired a holder: %w", err)
	}
	r.complete("async-commit-revalidates")
	return nil
}

// acceptedDeleteRun is one accepted-then-undermined delete: the resource it was
// accepted for, the incarnation that holds the name when the operation commits
// (absent when the name was left free), and the terminal Operation.
type acceptedDeleteRun struct {
	Created     wireResource
	Replacement wireResource
	Terminal    wireOperation
}

// acceptDeleteAndReplace accepts an async delete of one resource, removes that
// resource out of band while the operation is still pending, and optionally lets
// `replacement` re-create the same name before the operation is polled to its
// terminal state.
//
// The replacement is held to the two facts that make the scenario a substitution
// rather than a stale fence: it is a different incarnation, and it sits at the
// same revision the accepted delete was fenced at. A conforming host cannot
// answer this with a revision check.
func (r *v3Runner) acceptDeleteAndReplace(
	subject probeTarget,
	replacement *probeTarget,
	keyPrefix string,
) (acceptedDeleteRun, error) {
	run := acceptedDeleteRun{}
	created, _, err := r.applyResource(subject, applyOptions{
		Create: true, IdempotencyKey: keyPrefix + "-create",
	}, http.StatusCreated)
	if err != nil {
		return run, err
	}
	run.Created = created
	acceptedDelete, err := r.deleteResource(
		subject, created.Metadata.Revision, keyPrefix+"-async-delete",
		map[string]string{ErrorProbeHeader: ProbeAsync},
	)
	if err != nil {
		return run, err
	}
	if acceptedDelete.Status != http.StatusAccepted {
		return run, fmt.Errorf(
			"async delete HTTP %d, want 202; body=%s",
			acceptedDelete.Status, strings.TrimSpace(string(acceptedDelete.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(acceptedDelete, &envelope); err != nil {
		return run, err
	}
	if envelope.Operation.Done {
		return run, errors.New("the 202 delete was already terminal, so nothing could move underneath it")
	}
	// The resource leaves out of band, exactly as a backend deletion the host did
	// not perform: relation protection is bypassed, the accepted operation is not.
	removed, err := r.deleteResource(
		subject, created.Metadata.Revision, keyPrefix+"-external-delete",
		map[string]string{ErrorProbeHeader: ProbeExternalChange},
	)
	if err != nil {
		return run, err
	}
	if removed.Status != http.StatusNoContent {
		return run, fmt.Errorf(
			"out-of-band delete HTTP %d, want 204; body=%s",
			removed.Status, strings.TrimSpace(string(removed.Body)),
		)
	}
	if replacement != nil {
		recreated, _, err := r.applyResource(*replacement, applyOptions{
			Create: true, IdempotencyKey: keyPrefix + "-recreate",
		}, http.StatusCreated)
		if err != nil {
			return run, fmt.Errorf("the replacement incarnation could not be created: %w", err)
		}
		if recreated.Metadata.UID == created.Metadata.UID {
			return run, errors.New("the replacement reused the removed resource's uid, so no incarnation changed")
		}
		if recreated.Metadata.Revision != created.Metadata.Revision {
			return run, fmt.Errorf(
				"the replacement is at revision %s while the accepted delete was fenced at %s; unless the two "+
					"coincide this proves only that a revision fence holds",
				recreated.Metadata.Revision, created.Metadata.Revision,
			)
		}
		run.Replacement = recreated
	}
	terminal, err := r.pollOperation(envelope.Operation.ID)
	if err != nil {
		return run, err
	}
	run.Terminal = terminal
	return run, nil
}

// requireTerminalOperationError holds one terminal Operation to exactly one
// stable error code, and to the retryability that code carries: neither identity
// failure is automatically retryable, because no amount of waiting turns one
// incarnation back into another.
func requireTerminalOperationError(operation wireOperation, code string) error {
	if operation.Result != nil || operation.Error == nil {
		return fmt.Errorf("terminal operation must carry exactly the error: %+v", operation)
	}
	if operation.Error["code"] != code {
		return fmt.Errorf("terminal operation error = %v, want %s", operation.Error["code"], code)
	}
	if retryable, _ := operation.Error["retryable"].(bool); retryable {
		return fmt.Errorf("%s was reported as automatically retryable", code)
	}
	return nil
}

// checkAsyncCommitBindsAcceptedIdentity proves the identity half of the same
// commitment: a 202 accepts a mutation to ONE resource, so the commit is bound
// to the incarnation it was accepted for and never re-derived from the name that
// addressed it.
//
// A name is reusable and a fresh resource starts at revision 1, so a target
// removed out of band and re-created under the same name presents a NEW
// incarnation at the very revision the accepted delete was fenced at. A host
// that re-resolved its target by name at commit time would find the
// replacement, see the fence satisfied, delete a resource under a different uid
// — here under a different exact contract as well — and report success. That is
// the substitution decision 0015 closed for relations, stated about the
// operation's own target.
//
// The three cases are the three things that can be behind the name when the
// operation commits: another contract's incarnation, another incarnation of the
// same contract, and nothing at all. The first two are `uid_mismatch`, the third
// `resource_not_found`, and in neither of the first two may the replacement be
// touched.
func (r *v3Runner) checkAsyncCommitBindsAcceptedIdentity() error {
	// The replacement returns under the OTHER definition version, so it differs
	// from the accepted resource in both halves of its identity — its uid and its
	// exact FormRef — while occupying exactly the same name.
	contractMoved := r.target(r.contract.RunnerInput.ModuleWorker)
	contractMoved.Name = "accepted-identity-contract"
	otherContract := r.syntheticDefinitionTarget(contractMoved.Name)
	run, err := r.acceptDeleteAndReplace(contractMoved, &otherContract, "key-accepted-identity-contract")
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(run.Terminal, "uid_mismatch"); err != nil {
		return fmt.Errorf(
			"a delete accepted for %s under %s settled against a replacement under %s: %w",
			run.Created.Metadata.UID, contractMoved.Ref.DefinitionVersion,
			otherContract.Ref.DefinitionVersion, err,
		)
	}
	survivor, err := r.readRaw(otherContract)
	if err != nil {
		return fmt.Errorf(
			"the accepted delete removed the replacement it was never accepted for: %w", err,
		)
	}
	if survivor.Metadata.UID != run.Replacement.Metadata.UID || survivor.Form.FormRef != otherContract.Ref {
		return fmt.Errorf(
			"the surviving resource is %+v, want the replacement %s under %+v",
			survivor.Metadata, run.Replacement.Metadata.UID, otherContract.Ref,
		)
	}

	// The uid alone is enough to make it a different resource: the replacement
	// comes back under the SAME exact contract, so a host that compared only the
	// recorded FormRef still has to refuse.
	incarnationMoved := r.target(r.contract.RunnerInput.ModuleWorker)
	incarnationMoved.Name = "accepted-identity-incarnation"
	sameContract := incarnationMoved
	sameRun, err := r.acceptDeleteAndReplace(
		incarnationMoved, &sameContract, "key-accepted-identity-incarnation",
	)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(sameRun.Terminal, "uid_mismatch"); err != nil {
		return fmt.Errorf(
			"a delete accepted for %s settled against %s, a later incarnation of the same name: %w",
			sameRun.Created.Metadata.UID, sameRun.Replacement.Metadata.UID, err,
		)
	}
	kept, err := r.readRaw(sameContract)
	if err != nil {
		return fmt.Errorf("the accepted delete removed a later incarnation of the same name: %w", err)
	}
	if kept.Metadata.UID != sameRun.Replacement.Metadata.UID {
		return fmt.Errorf(
			"the name is held by %s, want the replacement %s",
			kept.Metadata.UID, sameRun.Replacement.Metadata.UID,
		)
	}

	// Nothing holds the name at all, which is the other closed answer: the
	// resource the operation was accepted for is simply gone.
	vanished := r.target(r.contract.RunnerInput.ModuleWorker)
	vanished.Name = "accepted-identity-vanished"
	goneRun, err := r.acceptDeleteAndReplace(vanished, nil, "key-accepted-identity-vanished")
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(goneRun.Terminal, "resource_not_found"); err != nil {
		return fmt.Errorf("a delete whose target vanished before commit: %w", err)
	}
	if err := r.expectResourceAbsent(vanished); err != nil {
		return fmt.Errorf("the vanished resource reappeared: %w", err)
	}
	r.complete("async-commit-binds-the-accepted-identity")
	return nil
}

func (r *v3Runner) checkCrossSpace(kv probeTarget) error {
	primary, _, err := r.read(kv)
	if err != nil {
		return err
	}
	alternate := kv
	alternate.Space = r.contract.RunnerInput.AlternateSpace
	// Reuse the primary Space's create key: replay records are namespaced by
	// space, so this must execute independently instead of replaying or
	// failing the fingerprint.
	created, _, err := r.applyResource(alternate, applyOptions{
		Create: true, IdempotencyKey: "key-recreate-kv",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("alternate-space create with a reused key: %w", err)
	}
	if created.Metadata.UID == primary.Metadata.UID {
		return errors.New("resources in two spaces shared a uid")
	}
	primaryAgain, _, err := r.read(kv)
	if err != nil {
		return err
	}
	if primaryAgain.Metadata.UID != primary.Metadata.UID {
		return errors.New("alternate-space create changed the primary-space resource")
	}
	absent := kv
	absent.Name = "async-probe"
	absent.Space = r.contract.RunnerInput.AlternateSpace
	if err := r.expectResourceAbsent(absent); err != nil {
		return fmt.Errorf("cross-space read isolation: %w", err)
	}
	r.complete("cross-space-isolation")
	return nil
}

var forbiddenSupportTokens = []string{`"price"`, `"sku"`, `"region"`, `"quota"`, `"billing"`}

func (r *v3Runner) checkSupportProfiles() error {
	listResponse, err := r.request(http.MethodGet, r.apiBase+"/support/forms", nil, nil)
	if err != nil {
		return err
	}
	if listResponse.Status != http.StatusOK {
		return fmt.Errorf("support forms HTTP %d", listResponse.Status)
	}
	var list struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := decodeStrictResponse(listResponse, &list); err != nil {
		return err
	}
	if len(list.Profiles) == 0 {
		return errors.New("support profiles are empty")
	}
	var workerVersionProfile map[string]any
	for _, profile := range list.Profiles {
		if profile["apiVersion"] != supportAPIVersion || profile["kind"] != "FormSupport" {
			return fmt.Errorf("support profile identity is invalid: %v", profile)
		}
		reference, _ := profile["formRef"].(map[string]any)
		if reference == nil {
			return errors.New("FormSupport profile omitted formRef")
		}
		if reference["kind"] == "WorkerVersion" {
			workerVersionProfile = profile
		}
	}
	if workerVersionProfile == nil {
		return errors.New("support profiles omit the WorkerVersion line")
	}
	if err := verifyWorkerVersionProfile(workerVersionProfile, r.contract.RunnerInput.SupportProbes.RuntimeContract); err != nil {
		return err
	}
	version := r.contract.RunnerInput.WorkerVersion.Identity.FormRef
	oneURL := fmt.Sprintf(
		"%s/support/forms/%s/%s/%s",
		r.apiBase, groupSegments(version.APIVersion),
		url.PathEscape(version.Kind), url.PathEscape(version.DefinitionVersion),
	)
	oneResponse, err := r.request(http.MethodGet, oneURL, nil, nil)
	if err != nil {
		return err
	}
	if oneResponse.Status != http.StatusOK {
		return fmt.Errorf("support form HTTP %d", oneResponse.Status)
	}
	var one map[string]any
	if err := decodeStrictResponse(oneResponse, &one); err != nil {
		return err
	}
	if err := verifyWorkerVersionProfile(one, r.contract.RunnerInput.SupportProbes.RuntimeContract); err != nil {
		return err
	}
	probes := r.contract.RunnerInput.SupportProbes
	interfaceResponse, err := r.request(
		http.MethodGet,
		fmt.Sprintf("%s/support/interfaces/%s/%s", r.apiBase,
			url.PathEscape(probes.Interface.Name), url.PathEscape(probes.Interface.Version)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if err := verifySupportRefProfile(interfaceResponse, "InterfaceSupport", "interfaceRef", probes.Interface); err != nil {
		return err
	}
	bindingResponse, err := r.request(
		http.MethodGet,
		fmt.Sprintf("%s/support/bindings/%s/%s", r.apiBase,
			url.PathEscape(probes.Binding.Name), url.PathEscape(probes.Binding.Version)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if err := verifySupportRefProfile(bindingResponse, "BindingSupport", "bindingRef", probes.Binding); err != nil {
		return err
	}
	for _, body := range [][]byte{listResponse.Body, oneResponse.Body, interfaceResponse.Body, bindingResponse.Body} {
		lowered := strings.ToLower(string(body))
		for _, token := range forbiddenSupportTokens {
			if strings.Contains(lowered, token) {
				return fmt.Errorf("support surface carries forbidden commercial key %s", token)
			}
		}
	}
	r.complete("support-profiles-present")
	return nil
}

// verifyWorkerVersionProfile holds a host's WorkerVersion support profile to
// the runtime ABI contract rather than to a runtime selector.
//
// The advertised handler enum must be EXACTLY the handler vocabulary the pinned
// worker.runtime contract defines: a host that supports more handlers than the
// ABI describes, or fewer, is not implementing that contract. And the profile
// must carry no compatibility date or flag at all — those were removed from the
// lane because a date is meaningless without a registry stating which behavior
// each date changes, so a profile that still advertises one is advertising
// portability it cannot deliver (spec/decisions/0019).
func verifyWorkerVersionProfile(profile map[string]any, runtime RuntimeContractProbe) error {
	enums, _ := profile["supportedEnums"].(map[string]any)
	if enums == nil {
		return errors.New("WorkerVersion profile omitted supportedEnums")
	}
	if !stringSliceEquals(enums["handlers"], runtime.Handlers) {
		return fmt.Errorf(
			"WorkerVersion supportedEnums.handlers = %v, want the %s@%s vocabulary %v",
			enums["handlers"], runtime.Name, runtime.Version, runtime.Handlers,
		)
	}
	if _, present := enums["compatibilityFlags"]; present {
		return errors.New(
			"WorkerVersion supportedEnums advertises compatibilityFlags; the lane has no compatibility flag",
		)
	}
	ranges, _ := profile["supportedRanges"].(map[string]any)
	if _, present := ranges["compatibilityDate"]; present {
		return errors.New(
			"WorkerVersion supportedRanges advertises a compatibilityDate range; the runtime is the exact " +
				runtime.Name + " contract, not a date",
		)
	}
	limits, _ := profile["limits"].(map[string]any)
	if limits == nil || fmt.Sprintf("%v", limits["maximumBundleBytes"]) != "10485760" {
		return fmt.Errorf("WorkerVersion limits = %v", limits)
	}
	return nil
}

func stringSliceEquals(value any, want []string) bool {
	items, ok := value.([]any)
	if !ok || len(items) != len(want) {
		return false
	}
	for index, item := range items {
		if item != want[index] {
			return false
		}
	}
	return true
}

func verifySupportRefProfile(response wireResponse, kind, refMember string, want NameVersion) error {
	if response.Status != http.StatusOK {
		return fmt.Errorf("%s HTTP %d; body=%s", kind, response.Status, strings.TrimSpace(string(response.Body)))
	}
	var profile map[string]any
	if err := decodeStrictResponse(response, &profile); err != nil {
		return err
	}
	if profile["apiVersion"] != supportAPIVersion || profile["kind"] != kind {
		return fmt.Errorf("%s profile identity is invalid", kind)
	}
	reference, _ := profile[refMember].(map[string]any)
	if reference == nil || reference["name"] != want.Name || reference["version"] != want.Version {
		return fmt.Errorf("%s profile %s does not match the probe", kind, refMember)
	}
	digest, _ := reference["schemaDigest"].(string)
	if !formpackage.ValidDigest(digest) {
		return fmt.Errorf("%s profile schemaDigest is invalid", kind)
	}
	return nil
}

func cloneJSONMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
