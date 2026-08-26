package portableconformancev3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformsnapshot"
)

var stableGenericRequiredChecks = []string{
	"apply-idempotency-replay",
	"artifact-commit-binds-declared-size",
	"artifact-digest-mismatch",
	"artifact-upload-missing-blob",
	"async-operation-flow",
	"create-conflict-when-exists",
	"declared-constraint-semantics-enforced",
	"declared-exclusive-holds-enforced",
	"delete-generation-fence",
	"delete-revision-fence",
	"delete-then-recreate-uid-changes",
	"error-envelope-taxonomy",
	"exact-form-ref-fails-closed-on-unknown-definition",
	"fence-matrix-observed",
	"operation-bound-to-its-creating-principal",
	"operation-cancel",
	"operation-replay-terminal",
	"operation-resumable-after-settlement",
	"portable-defaults-materialized",
	"prepare-binds-exact-spec",
	"prepare-is-tenant-scoped",
	"prepare-requires-update-fence",
	"replay-record-retires-with-its-incarnation",
	"spec-change-bumps-generation",
	"stale-revision-rejected",
	"status-change-bumps-revision-not-generation",
	"unauthenticated-request-refused",
	"upload-session-bound-to-its-creating-principal",
}

type stableGenericPackageInput struct {
	Path          string `json:"path"`
	PackageDigest string `json:"packageDigest"`
}

type stableGenericSnapshotInput struct {
	Name             string                           `json:"name"`
	Packages         []stableGenericPackageInput      `json:"packages"`
	Interfaces       []stableGenericInterfaceInput    `json:"interfaces"`
	Bindings         []stableGenericInterfaceInput    `json:"bindings"`
	DefaultCreates   []currentformsnapshot.DefaultPin `json:"defaultCreates"`
	ExpectedFormRefs []FormRef                        `json:"expectedFormRefs"`
}

type stableGenericInterfaceInput struct {
	Path         string `json:"path"`
	SchemaDigest string `json:"schemaDigest"`
}

type stableGenericExternalServiceProbe struct {
	Property                   string         `json:"property"`
	ServiceAPIVersion          string         `json:"serviceApiVersion"`
	SupportedProtocol          string         `json:"supportedProtocol"`
	UnsupportedProtocol        string         `json:"unsupportedProtocol"`
	SupportedDesired           map[string]any `json:"supportedDesired"`
	RequiredUnsupportedDesired map[string]any `json:"requiredUnsupportedDesired"`
	OptionalUnsupportedDesired map[string]any `json:"optionalUnsupportedDesired"`
}

type stableGenericResourceProbe struct {
	FormRef              FormRef        `json:"formRef"`
	Name                 string         `json:"name"`
	Desired              map[string]any `json:"desired"`
	UpdatedDesired       map[string]any `json:"updatedDesired"`
	SecondUpdatedDesired map[string]any `json:"secondUpdatedDesired"`
	HostAssignedOutputs  map[string]any `json:"hostAssignedOutputs"`
}

type stableGenericConstraintProbe struct {
	Name    string  `json:"name"`
	FormRef FormRef `json:"formRef"`
}

type stableGenericConstraintProbes struct {
	Node             stableGenericConstraintProbe `json:"node"`
	DistinctPair     stableGenericConstraintProbe `json:"distinctPair"`
	UniquePair       stableGenericConstraintProbe `json:"uniquePair"`
	UniquePairSecond stableGenericConstraintProbe `json:"uniquePairSecond"`
	Member           stableGenericConstraintProbe `json:"member"`
	SameTarget       stableGenericConstraintProbe `json:"sameTarget"`
	Structural       stableGenericConstraintProbe `json:"structural"`
	Sum              stableGenericConstraintProbe `json:"sum"`
	ClaimPrimary     stableGenericConstraintProbe `json:"claimPrimary"`
	ClaimAlternate   stableGenericConstraintProbe `json:"claimAlternate"`
	ExclusiveSecond  stableGenericConstraintProbe `json:"exclusiveSecond"`
}

type stableGenericArtifactTransport struct {
	BlobSource   string `json:"blobSource"`
	DeclaredSize int    `json:"declaredSize"`
	ContentType  string `json:"contentType"`
}

type stableGenericHostProbe struct {
	Snapshot                 string                            `json:"snapshot"`
	FormRef                  FormRef                           `json:"formRef"`
	PackageDigest            string                            `json:"packageDigest"`
	Name                     string                            `json:"name"`
	Space                    string                            `json:"space"`
	Desired                  map[string]any                    `json:"desired"`
	UpdatedDesired           map[string]any                    `json:"updatedDesired"`
	InvalidSchemaDesired     map[string]any                    `json:"invalidSchemaDesired"`
	InvalidConstraintDesired map[string]any                    `json:"invalidConstraintDesired"`
	ExternalServices         stableGenericExternalServiceProbe `json:"externalServices"`
	Resources                struct {
		Keyed       stableGenericResourceProbe `json:"keyed"`
		Sequenced   stableGenericResourceProbe `json:"sequenced"`
		Revision    stableGenericResourceProbe `json:"revision"`
		Output      stableGenericResourceProbe `json:"output"`
		Lease       stableGenericResourceProbe `json:"lease"`
		Reservation stableGenericResourceProbe `json:"reservation"`
	} `json:"resources"`
	SyntheticSecondGroup             FormRef                        `json:"syntheticSecondGroup"`
	SyntheticSecondDefinitionVersion FormRef                        `json:"syntheticSecondDefinitionVersion"`
	ConstraintSemantics              stableGenericConstraintProbes  `json:"constraintSemantics"`
	ArtifactTransport                stableGenericArtifactTransport `json:"artifactTransport"`
	Support                          struct {
		Interface NameVersion `json:"interface"`
		Binding   NameVersion `json:"binding"`
	} `json:"support"`
}

type stableGenericCorpus struct {
	Format            string                       `json:"format"`
	HostAPILane       string                       `json:"hostApiLane"`
	RequiredChecks    []string                     `json:"requiredChecks"`
	SnapshotInputs    []stableGenericSnapshotInput `json:"snapshotInputs"`
	ExternalHostProbe stableGenericHostProbe       `json:"externalHostProbe"`
}

// stableGenericSnapshotReport is the data-only evidence that the
// generic suite compiled one complete neutral input. FormRefs is empty for the
// required zero-family witness and carries only independently supplied exact
// identities for external witnesses.
type stableGenericSnapshotReport struct {
	Name           string    `json:"name"`
	SnapshotDigest string    `json:"snapshotDigest"`
	FormRefs       []FormRef `json:"formRefs"`
}

type compiledStableGenericSnapshot struct {
	snapshot *currentformsnapshot.Snapshot
}

func stableVerifyGeneric(
	ctx context.Context,
	repositoryRoot, corpusPath string,
	record stableSuiteCorpusRecord,
) ([]stableGenericSnapshotReport, error) {
	var corpus stableGenericCorpus
	raw, err := stableReadStrict(corpusPath, &corpus)
	if err != nil {
		return nil, err
	}
	if stableDigest(raw) != record.SHA256 || corpus.Format != StableGenericFormat ||
		corpus.HostAPILane != stableLane.APIVersion ||
		!reflect.DeepEqual(corpus.RequiredChecks, record.RequiredChecks) {
		return nil, errors.New("stable generic corpus identity or digest drifted")
	}
	if !reflect.DeepEqual(record.RequiredChecks, stableGenericRequiredChecks) {
		return nil, errors.New("stable generic required runner checks drifted")
	}
	compiled, reports, err := stableCompileGenericSnapshots(repositoryRoot, corpusPath, corpus)
	if err != nil {
		return nil, err
	}
	completed := map[string]bool{}
	if err := stableRunGenericHostChecks(ctx, corpus, compiled, completed); err != nil {
		return nil, fmt.Errorf("stable generic Snapshot-backed Host checks: %w", err)
	}
	if len(completed) != len(corpus.RequiredChecks) {
		missing := make([]string, 0, len(corpus.RequiredChecks)-len(completed))
		for _, check := range corpus.RequiredChecks {
			if !completed[check] {
				missing = append(missing, check)
			}
		}
		return nil, fmt.Errorf("stable generic runner completed %d/%d required checks; missing %v", len(completed), len(corpus.RequiredChecks), missing)
	}
	for _, check := range corpus.RequiredChecks {
		if !completed[check] {
			return nil, fmt.Errorf("stable generic runner did not execute required check %q", check)
		}
	}
	return reports, nil
}

func stableCompileGenericSnapshots(
	repositoryRoot, corpusPath string,
	corpus stableGenericCorpus,
) (map[string]compiledStableGenericSnapshot, []stableGenericSnapshotReport, error) {
	if len(corpus.SnapshotInputs) < 2 {
		return nil, nil, errors.New("stable generic corpus must carry zero-family and external Snapshot inputs")
	}
	compiled := make(map[string]compiledStableGenericSnapshot, len(corpus.SnapshotInputs))
	reports := make([]stableGenericSnapshotReport, 0, len(corpus.SnapshotInputs))
	hasZero, hasExternal := false, false
	externalRoot := filepath.Join(filepath.Dir(corpusPath), "generic-host", "external-family")
	for index, input := range corpus.SnapshotInputs {
		if input.Name == "" || (index > 0 && corpus.SnapshotInputs[index-1].Name >= input.Name) {
			return nil, nil, errors.New("stable generic Snapshot inputs are not sorted and unique")
		}
		artifacts := make([]currentformsnapshot.PackageArtifact, 0, len(input.Packages))
		interfaces := make([]currentformsnapshot.InterfaceArtifact, 0, len(input.Interfaces))
		bindings := make([]currentformsnapshot.BindingArtifact, 0, len(input.Bindings))
		packageRefs := make(map[formpackage.FormRef]string, len(input.Packages))
		for _, packageInput := range input.Packages {
			indexPath, err := stableResolve(repositoryRoot, corpusPath, packageInput.Path)
			if err != nil {
				return nil, nil, err
			}
			if filepath.Base(indexPath) != formpackage.PackageIndexFilename {
				return nil, nil, fmt.Errorf("generic package input %q does not name %s", packageInput.Path, formpackage.PackageIndexFilename)
			}
			if err := stableRequireGenericSource(externalRoot, indexPath); err != nil {
				return nil, nil, err
			}
			verification, err := formpackage.VerifyDirectory(filepath.Dir(indexPath))
			if err != nil {
				return nil, nil, fmt.Errorf("verify generic external package %s: %w", packageInput.Path, err)
			}
			verified, ok := verification.VerifiedPackage()
			if !ok {
				return nil, nil, fmt.Errorf("generic external package %s issued no verified capability", packageInput.Path)
			}
			if verified.PackageDigest() != packageInput.PackageDigest {
				return nil, nil, fmt.Errorf("generic external package %s digest drifted", packageInput.Path)
			}
			if err := stableRequireExternalGroup(verified.FormRef().APIVersion); err != nil {
				return nil, nil, fmt.Errorf("generic external package %s: %w", packageInput.Path, err)
			}
			if _, repeated := packageRefs[verified.FormRef()]; repeated {
				return nil, nil, fmt.Errorf("generic Snapshot input %s repeats exact FormRef", input.Name)
			}
			packageRefs[verified.FormRef()] = verified.PackageDigest()
			artifacts = append(artifacts, currentformsnapshot.PackageArtifact{
				Origin:         "generic-conformance://" + filepath.ToSlash(packageInput.Path),
				ExpectedDigest: packageInput.PackageDigest,
				Package:        verified,
			})
		}
		for _, pin := range input.DefaultCreates {
			if err := stableRequireExternalGroup(pin.Group); err != nil {
				return nil, nil, fmt.Errorf("generic Snapshot input %s default: %w", input.Name, err)
			}
			if pin.Group != pin.Ref.APIVersion {
				return nil, nil, fmt.Errorf("generic Snapshot input %s default group and exact ref differ", input.Name)
			}
		}
		for _, interfaceInput := range input.Interfaces {
			definitionPath, err := stableResolve(repositoryRoot, corpusPath, interfaceInput.Path)
			if err != nil {
				return nil, nil, err
			}
			if err := stableRequireGenericSource(externalRoot, definitionPath); err != nil {
				return nil, nil, err
			}
			definition, err := os.ReadFile(definitionPath)
			if err != nil {
				return nil, nil, err
			}
			interfaces = append(interfaces, currentformsnapshot.InterfaceArtifact{
				Origin: interfaceInput.Path, ExpectedDigest: interfaceInput.SchemaDigest,
				Definition: definition,
			})
		}
		for _, bindingInput := range input.Bindings {
			definitionPath, err := stableResolve(repositoryRoot, corpusPath, bindingInput.Path)
			if err != nil {
				return nil, nil, err
			}
			if err := stableRequireGenericSource(externalRoot, definitionPath); err != nil {
				return nil, nil, err
			}
			definition, err := os.ReadFile(definitionPath)
			if err != nil {
				return nil, nil, err
			}
			bindings = append(bindings, currentformsnapshot.BindingArtifact{
				Origin: bindingInput.Path, ExpectedDigest: bindingInput.SchemaDigest,
				Definition: definition,
			})
		}
		snapshot, diagnostics := currentformsnapshot.Compile(currentformsnapshot.Input{
			HostAPI: corpus.HostAPILane, Packages: artifacts, Interfaces: interfaces, Bindings: bindings,
			DefaultCreates: input.DefaultCreates,
		})
		if snapshot == nil || len(diagnostics) != 0 {
			return nil, nil, fmt.Errorf("generic Snapshot input %s did not compile completely: %#v", input.Name, diagnostics)
		}
		if snapshot.HostAPI() != corpus.HostAPILane || snapshot.Digest() == "" {
			return nil, nil, fmt.Errorf("generic Snapshot input %s returned incomplete identity", input.Name)
		}
		gotRefs := make([]FormRef, 0, len(snapshot.Forms()))
		for _, form := range snapshot.Forms() {
			if err := stableRequireExternalGroup(form.Ref.APIVersion); err != nil {
				return nil, nil, fmt.Errorf("generic Snapshot input %s: %w", input.Name, err)
			}
			if packageRefs[form.Ref] != form.PackageDigest {
				return nil, nil, fmt.Errorf("generic Snapshot input %s changed package provenance", input.Name)
			}
			gotRefs = append(gotRefs, portableFormRef(form.Ref))
		}
		if !reflect.DeepEqual(gotRefs, input.ExpectedFormRefs) {
			return nil, nil, fmt.Errorf("generic Snapshot input %s exact Form roster drifted", input.Name)
		}
		if len(gotRefs) == 0 {
			if len(input.Packages) != 0 || len(input.Interfaces) != 0 || len(input.Bindings) != 0 || len(input.DefaultCreates) != 0 {
				return nil, nil, fmt.Errorf("generic zero-family Snapshot input %s carries hidden artifacts", input.Name)
			}
			hasZero = true
		} else {
			hasExternal = true
		}
		report := stableGenericSnapshotReport{
			Name: input.Name, SnapshotDigest: snapshot.Digest(), FormRefs: gotRefs,
		}
		compiled[input.Name] = compiledStableGenericSnapshot{snapshot: snapshot}
		reports = append(reports, report)
	}
	if !hasZero || !hasExternal {
		return nil, nil, errors.New("stable generic corpus does not prove both zero-family and external-family Snapshots")
	}
	return compiled, reports, nil
}

func stableRequireGenericSource(genericRoot, sourcePath string) error {
	resolvedRoot, err := filepath.EvalSymlinks(genericRoot)
	if err != nil {
		return err
	}
	resolvedSource, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedSource)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("generic input imports an official or out-of-corpus catalog source %q", sourcePath)
	}
	return nil
}

func stableRequireExternalGroup(group string) error {
	family := strings.SplitN(group, "/", 2)[0]
	if family == "" || family == "forms.takoform.com" || strings.HasSuffix(family, ".forms.takoform.com") {
		return fmt.Errorf("generic input names forbidden official family group %q", group)
	}
	return nil
}

func stableRunGenericHostChecks(
	ctx context.Context,
	corpus stableGenericCorpus,
	compiled map[string]compiledStableGenericSnapshot,
	completed map[string]bool,
) error {
	probe := corpus.ExternalHostProbe
	external, ok := compiled[probe.Snapshot]
	if !ok || len(external.snapshot.Forms()) == 0 {
		return errors.New("external Host probe names no non-empty compiled Snapshot")
	}
	if err := stableRequireExternalGroup(probe.FormRef.APIVersion); err != nil {
		return err
	}
	if !resourceNamePattern.MatchString(probe.Name) || !spacePattern.MatchString(probe.Space) ||
		len(probe.Desired) == 0 || len(probe.UpdatedDesired) == 0 ||
		len(probe.InvalidSchemaDesired) == 0 || len(probe.InvalidConstraintDesired) == 0 {
		return errors.New("stable generic external Host probe is incomplete")
	}
	externalRef := formpackageFormRef(probe.FormRef)
	definitionRaw, known := external.snapshot.Definition(externalRef)
	if !known {
		return errors.New("stable generic external Host probe exact FormRef is absent from its Snapshot")
	}
	if _, err := formpackage.ValidateDefinition(definitionRaw); err != nil {
		return err
	}
	packageDigest := ""
	for _, form := range external.snapshot.Forms() {
		if form.Ref == externalRef {
			packageDigest = form.PackageDigest
		}
	}
	if packageDigest == "" || packageDigest != probe.PackageDigest {
		return errors.New("stable generic external Host probe package digest drifted")
	}

	contract, err := stableGenericHostContract(corpus, probe, external.snapshot)
	if err != nil {
		return err
	}
	if contract.genericRoles == nil || contract.hasLegacyConcreteFormSlots() {
		return errors.New("stable generic Host contract selected concrete-family Form slots")
	}
	zeroSeen := false
	for _, entry := range compiled {
		if len(entry.snapshot.Forms()) != 0 {
			continue
		}
		zeroSeen = true
		catalog, err := stableGenericCatalog(entry.snapshot, nil)
		if err != nil {
			return err
		}
		if err := stableCheckZeroFamilyHost(ctx, contract, catalog); err != nil {
			return err
		}
	}
	if !zeroSeen {
		return errors.New("stable generic Host checks did not execute a zero-family Snapshot")
	}
	seed := genericPlanSeed{
		Snapshot: external.snapshot, Contract: contract,
		Probe: probe, Artifact: probe.ArtifactTransport,
	}
	plan, err := stableBuildGenericPlan(seed)
	if err != nil {
		return err
	}
	typedCompleted, err := stableRunGenericPlan(
		ctx, seed, plan, stableGenericRequiredChecks,
		genericMemoryFactory, genericHTTPFactory,
	)
	if err != nil {
		return err
	}
	for check := range typedCompleted {
		completed[check] = true
	}

	catalog, err := stableGenericCatalog(external.snapshot, &probe)
	if err != nil {
		return err
	}
	adapterArtifact, err := stableGenericHTTPArtifact(probe.ArtifactTransport)
	if err != nil {
		return err
	}
	if catalog.family != "" {
		return errors.New("neutral generic catalog selected an official family")
	}
	for _, form := range catalog.Forms {
		if form.EnforcedFamily != "" {
			return fmt.Errorf("neutral generic Form %s selected family %q", form.Ref.Kind, form.EnforcedFamily)
		}
	}
	host := NewReferenceHost(contract, catalog)
	server := httptest.NewServer(host)
	defer server.Close()
	runner := stableGenericRunner(ctx, contract, server)
	runner.adapterArtifact = &adapterArtifact
	runner.pinDesiredSchemas()
	setMutations := func(input stableGenericResourceProbe, desired ...map[string]any) error {
		for _, value := range desired {
			if len(value) == 0 {
				continue
			}
			materialized, err := stableGenericMaterialize(external.snapshot, input.FormRef, value)
			if err != nil {
				return err
			}
			runner.desiredMutations[input.FormRef] = append(runner.desiredMutations[input.FormRef], materialized)
		}
		return nil
	}
	if err := setMutations(stableGenericResourceProbe{FormRef: probe.FormRef}, probe.UpdatedDesired); err != nil {
		return err
	}
	for _, input := range []stableGenericResourceProbe{
		probe.Resources.Keyed, probe.Resources.Sequenced,
	} {
		if err := setMutations(input, input.UpdatedDesired, input.SecondUpdatedDesired); err != nil {
			return err
		}
	}
	if err := setMutations(probe.Resources.Revision, probe.Resources.Revision.UpdatedDesired); err != nil {
		return err
	}
	roles := contract.genericRoles
	primary := runner.target(roles.Primary)
	keyed := runner.target(roles.Keyed)
	sequenced := runner.target(roles.Sequenced)
	revision := runner.target(roles.Revision)
	steps := []func() error{
		runner.checkDiscovery,
		runner.checkErrorTaxonomy,
		runner.checkUnauthenticatedRequestRefused,
		func() error { return runner.checkFormsAvailability(primary) },
		runner.checkFormDefinitions,
		func() error { return runner.checkValidate(primary, keyed, sequenced) },
		runner.checkNegativeFixtures,
		func() error { return runner.checkPrepareBinding(sequenced) },
		func() error { return runner.checkPrepareSubstitution(sequenced) },
		func() error { return runner.checkApplyHeaders(keyed) },
		func() error { return runner.checkCreateLifecycle(keyed, primary, sequenced) },
		func() error { return runner.checkReplayRecordRetiresWithItsIncarnation(keyed) },
		func() error { return runner.checkGenerationFences(sequenced) },
		func() error { return runner.checkPrepareFences(sequenced) },
		func() error { return runner.checkReadETags(keyed, sequenced) },
		func() error { return runner.checkNamespacedGroupPathSegments(keyed) },
		func() error { return runner.checkConditionReasonsClosed(keyed, primary, sequenced) },
		func() error { return runner.checkSpaceIDGrammar(keyed) },
		func() error { return runner.checkConcurrentUnrelatedMutation(keyed, sequenced) },
		func() error { return runner.checkObserveAndStatusTouch(sequenced) },
		func() error { return runner.checkExpectedUID(sequenced) },
		func() error { return runner.checkPackageDigestNotIdentity(sequenced) },
		runner.checkSameKindTwoGroups,
		func() error { return stableCheckGenericRevisionRole(runner, revision) },
		func() error { return stableCheckGenericArtifactTransport(runner) },
		runner.checkUploadSessionOwnership,
		runner.checkArtifactDigestIsNotACapability,
		func() error { return stableCheckGenericImportFlows(runner, keyed, probe.InvalidSchemaDesired) },
		func() error { return runner.checkNativeIdentityClaim(keyed, sequenced) },
		func() error { return runner.checkRecordedNativeIdentity(keyed, sequenced) },
		func() error { return runner.checkDeleteFences(revision, keyed) },
		func() error { return runner.checkOperations(keyed) },
		func() error { return runner.checkOperationOwnership(keyed) },
		func() error { return runner.checkOperationResumableAfterSettlement(sequenced) },
		func() error { return runner.checkExactFormRefFailsClosedOnUnknownDefinition(keyed) },
		runner.checkTwoDefinitionVersionsAnswerIndependently,
		runner.checkResourceAnswersOnlyUnderItsRecordedFormRef,
		func() error { return stableCheckGenericAsyncCommitRevalidates(runner, keyed) },
		runner.checkAsyncCommitBindsAcceptedIdentity,
		func() error { return runner.checkCrossSpace(keyed) },
		func() error { return runner.checkDeclaredExclusiveHoldsEnforced() },
		runner.checkDeclaredConstraintSemanticsEnforced,
		func() error { return stableCheckGenericDeclaredOutputs(runner) },
		func() error { return runner.checkResourceAddressIsTenantScoped(primary) },
		func() error { return runner.checkResourceReadIsTenantIsolated(primary) },
		func() error { return runner.checkResourceObserveIsTenantIsolated(primary) },
		func() error { return runner.checkResourceUpdateIsTenantIsolated(primary) },
		func() error { return runner.checkResourceImportIsTenantIsolated(primary) },
		func() error { return runner.checkResourceDeleteIsTenantIsolated(primary) },
		func() error { return runner.checkPrepareIsTenantScoped(primary) },
		func() error { return runner.checkIdempotencyIsTenantScoped(primary) },
		runner.checkEachTenantMutatesItsOwnPlane,
		func() error { return runner.checkFenceMatrixObserved(keyed) },
		func() error { return runner.checkFormsRouteEnumerates(primary) },
		func() error { return runner.checkAvailabilityTruthConditions(keyed) },
		func() error { return runner.checkCancelOutcomesClosed(keyed) },
		func() error { return runner.checkStableStandardServiceSupportEnforced(primary) },
		func() error { return runner.checkPortableDefaultsMaterialized(primary) },
		func() error { return stableCheckGenericFormSupportProfile(runner, external.snapshot, probe) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	if selections := host.FamilyBranchSelections(); selections != 0 {
		return fmt.Errorf("neutral generic run selected %d concrete-family branches", selections)
	}
	if selections := host.LegacySemanticSelections(); selections != 0 {
		return fmt.Errorf("neutral generic Host selected %d legacy concrete semantic inputs", selections)
	}
	if selections := host.ConcreteKindSelections(); selections != 0 {
		return fmt.Errorf("neutral generic Host selected %d concrete Form-kind hooks", selections)
	}
	if selections := runner.LegacySemanticSelections(); selections != 0 {
		return fmt.Errorf("neutral generic run selected %d legacy concrete semantic inputs", selections)
	}
	return nil
}

func stableCheckGenericRevisionRole(runner *v3Runner, target probeTarget) error {
	if _, _, err := runner.applyResource(target, applyOptions{
		Create: true, IdempotencyKey: "key-neutral-revision-create",
	}, http.StatusCreated); err != nil {
		return err
	}
	sameSpec, err := runner.apply(target, applyOptions{
		ExpectedGeneration: "1", IdempotencyKey: "key-neutral-revision-update",
	})
	if err != nil {
		return err
	}
	if err := runner.expectStableError(sameSpec, "invalid_argument"); err != nil {
		return fmt.Errorf("neutral revision-role update: %w", err)
	}
	runner.complete("revision-role-update-rejected")
	return runner.checkNoUpdateSpecChangeRejected(target)
}

func stableCheckGenericImportFlows(runner *v3Runner, target probeTarget, invalid map[string]any) error {
	adopted := target
	adopted.Name = "neutral-import-probe"
	response, err := runner.importResource(adopted, importOptions{
		NativeID: "native/neutral-import", IdempotencyKey: "key-neutral-import", Create: true,
	})
	if err != nil {
		return err
	}
	resource, err := decodeResource(response, http.StatusCreated)
	if err != nil {
		return err
	}
	if err := runner.contract.lane.verifyResourceIdentity(resource, adopted); err != nil {
		return err
	}
	if !uidPattern.MatchString(resource.Metadata.UID) || resource.Metadata.Generation != "1" || resource.Metadata.Revision != "1" {
		return fmt.Errorf("neutral import returned invalid identity %+v", resource.Metadata)
	}
	readBack, _, err := runner.read(adopted)
	if err != nil {
		return err
	}
	if readBack.Metadata.UID != resource.Metadata.UID {
		return errors.New("neutral imported resource did not read back under its minted uid")
	}
	runner.complete("import-adopts-native-resource")

	invalidTarget := target
	invalidTarget.Name = "neutral-import-invalid"
	invalidTarget.Spec = cloneJSONMap(invalid)
	rejected, err := runner.importResource(invalidTarget, importOptions{
		NativeID: "native/neutral-import-invalid", IdempotencyKey: "key-neutral-import-invalid", Create: true,
	})
	if err != nil {
		return err
	}
	if err := runner.expectStableError(rejected, "invalid_argument"); err != nil {
		return fmt.Errorf("neutral import schema validation: %w", err)
	}
	if err := runner.expectResourceAbsent(invalidTarget); err != nil {
		return fmt.Errorf("neutral rejected import mutated state: %w", err)
	}
	runner.complete("import-validates-like-apply")
	return nil
}

func stableCheckGenericDeclaredOutputs(runner *v3Runner) error {
	declaring := runner.contract.semanticOutput()
	declaringTarget := runner.target(declaring)
	created, _, err := runner.applyResource(declaringTarget, applyOptions{
		Create: true, IdempotencyKey: "key-neutral-output-create",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	wantMembers, schema, err := declaring.outputContract()
	if err != nil {
		return err
	}
	if len(wantMembers) == 0 || created.Status == nil {
		return errors.New("neutral output Form returned no declared outputs")
	}
	gotMembers := make([]string, 0, len(created.Status.Outputs))
	for name := range created.Status.Outputs {
		gotMembers = append(gotMembers, name)
	}
	sort.Strings(gotMembers)
	if !reflect.DeepEqual(gotMembers, wantMembers) {
		return fmt.Errorf("neutral declared outputs = %v, want %v", gotMembers, wantMembers)
	}
	if err := validateDeclaredOutputs(schema, created.Status.Outputs); err != nil {
		return fmt.Errorf("neutral declared outputs violate their schema: %w", err)
	}

	omittingTarget := runner.target(runner.contract.semanticPrimary())
	omitting, _, err := runner.read(omittingTarget)
	if err != nil {
		return err
	}
	if omitting.Status != nil && omitting.Status.Outputs != nil {
		return fmt.Errorf("neutral Form without outputSchema returned outputs %v", omitting.Status.Outputs)
	}
	runner.complete("form-declared-outputs-are-exact")
	return nil
}

func stableCheckGenericFormSupportProfile(
	runner *v3Runner,
	snapshot *currentformsnapshot.Snapshot,
	probe stableGenericHostProbe,
) error {
	listResponse, err := runner.request(http.MethodGet, runner.apiBase+"/support/forms", nil, nil)
	if err != nil {
		return err
	}
	if listResponse.Status != http.StatusOK {
		return fmt.Errorf("neutral support forms HTTP %d", listResponse.Status)
	}
	var list struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := decodeStrictResponse(listResponse, &list); err != nil {
		return err
	}
	if len(list.Profiles) != len(snapshot.Forms()) {
		return fmt.Errorf("neutral support profiles = %d, want every %d Snapshot Forms", len(list.Profiles), len(snapshot.Forms()))
	}
	profiles := map[FormRef]map[string]any{}
	for _, profile := range list.Profiles {
		if profile["apiVersion"] != runner.supportAPIVersion() || profile["kind"] != "FormSupport" {
			return fmt.Errorf("neutral support profile identity is invalid: %v", profile)
		}
		reference, _ := profile["formRef"].(map[string]any)
		raw, err := encodeRunnerJSON(reference)
		if err != nil {
			return err
		}
		var ref FormRef
		if err := formpackage.DecodeStrictIJSON(raw, &ref); err != nil {
			return err
		}
		if _, repeated := profiles[ref]; repeated {
			return fmt.Errorf("neutral support profiles repeat %+v", ref)
		}
		profiles[ref] = profile
	}
	for _, compiled := range snapshot.Forms() {
		ref := portableFormRef(compiled.Ref)
		profile := profiles[ref]
		if profile == nil {
			return fmt.Errorf("neutral support profiles omit exact Form %+v", ref)
		}
		raw, _ := snapshot.Definition(compiled.Ref)
		definition, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			return err
		}
		operationValues, ok := profile["operations"].([]any)
		if !ok {
			return fmt.Errorf("neutral support profile for %s omitted operations", ref.Kind)
		}
		operations := anyStringSlice(operationValues)
		if !reflect.DeepEqual(operations, definition.LifecycleCapabilities) {
			return fmt.Errorf("neutral support operations for %s = %v, want %v", ref.Kind, operations, definition.LifecycleCapabilities)
		}
	}

	selected, err := runner.request(http.MethodGet, runner.formSupportURL(probe.FormRef), nil, nil)
	if err != nil {
		return err
	}
	if selected.Status != http.StatusOK {
		return fmt.Errorf("neutral selected Form support HTTP %d", selected.Status)
	}
	var selectedProfile map[string]any
	if err := decodeStrictResponse(selected, &selectedProfile); err != nil {
		return err
	}
	selectedRef, _ := selectedProfile["formRef"].(map[string]any)
	if selectedRef["schemaDigest"] != probe.FormRef.SchemaDigest {
		return errors.New("neutral selected Form support substituted the exact identity")
	}

	for _, body := range [][]byte{listResponse.Body, selected.Body} {
		lowered := strings.ToLower(string(body))
		for _, token := range forbiddenSupportTokens {
			if strings.Contains(lowered, token) {
				return fmt.Errorf("neutral support surface carries forbidden commercial key %s", token)
			}
		}
	}
	runner.complete("form-support-profile-exact")
	return nil
}

// stableCheckGenericAsyncCommitRevalidates uses only generic identity fences:
// a pending update is accepted at generation N, another synchronous update
// advances the resource, and commit must revalidate to generation_conflict.
func stableCheckGenericAsyncCommitRevalidates(runner *v3Runner, target probeTarget) error {
	subject := target
	subject.Name = "neutral-async-revalidate"
	created, _, err := runner.applyResource(subject, applyOptions{
		Create: true, IdempotencyKey: "key-neutral-revalidate-create",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	firstMutation := subject
	firstMutation.Spec = runner.desiredMutation(target, 0, target.Spec)
	accepted, err := runner.apply(firstMutation, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-neutral-revalidate-accepted",
		ExtraHeaders:       map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	if accepted.Status != http.StatusAccepted {
		return fmt.Errorf("neutral async update HTTP %d, want 202: %s", accepted.Status, strings.TrimSpace(string(accepted.Body)))
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(accepted, &envelope); err != nil {
		return err
	}
	secondMutation := subject
	secondMutation.Spec = runner.desiredMutation(target, 1, target.Spec)
	advanced, _, err := runner.applyResource(secondMutation, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-neutral-revalidate-advance",
	}, http.StatusOK)
	if err != nil {
		return err
	}
	terminal, err := runner.pollOperation(envelope.Operation.ID)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(terminal, "generation_conflict"); err != nil {
		return err
	}
	current, _, err := runner.read(subject)
	if err != nil {
		return err
	}
	if current.Metadata.UID != created.Metadata.UID || current.Metadata.Generation != advanced.Metadata.Generation {
		return errors.New("neutral rejected async commit changed the live incarnation")
	}
	wantDigest, err := specCanonicalDigest(secondMutation.Spec)
	if err != nil {
		return err
	}
	gotDigest, err := specCanonicalDigest(current.Spec)
	if err != nil {
		return err
	}
	if gotDigest != wantDigest {
		return errors.New("neutral rejected async commit overwrote the intervening desired state")
	}
	runner.complete("async-commit-revalidates")
	return nil
}

// stableCheckGenericArtifactTransport drives only the family-neutral artifact
// transport contract. Concrete family corpora retain artifact/resource
// composition checks for their own artifact-backed Forms.
func stableCheckGenericArtifactTransport(runner *v3Runner) error {
	transport := runner.hostArtifactTransport()
	manifest, wantDigest, err := runner.bundleManifest()
	if err != nil {
		return err
	}
	blobSource := transport.BlobSource
	blobDigest := formpackage.DigestBytes([]byte(blobSource))
	uploadID, missing, err := runner.startArtifactUpload(manifest, "key-neutral-artifact-start")
	if err != nil {
		return err
	}
	if len(missing) != 1 || missing[0] != blobDigest {
		return fmt.Errorf("neutral artifact missingBlobs = %v, want payload digest", missing)
	}
	early, err := runner.commitArtifact(uploadID, "key-neutral-artifact-early")
	if err != nil {
		return err
	}
	if err := runner.expectStableError(early, "artifact_missing"); err != nil {
		return fmt.Errorf("neutral artifact commit before blob: %w", err)
	}
	runner.complete("artifact-upload-missing-blob")

	blobURL := runner.apiBase + "/artifacts/uploads/" + url.PathEscape(uploadID) + "/blobs/" + url.PathEscape(blobDigest)
	wrong, err := runner.request(http.MethodPut, blobURL,
		map[string]string{"Content-Type": "application/octet-stream"}, []byte("wrong-neutral-bytes"))
	if err != nil {
		return err
	}
	if err := runner.expectStableError(wrong, "artifact_invalid"); err != nil {
		return fmt.Errorf("neutral artifact digest mismatch: %w", err)
	}
	runner.complete("artifact-digest-mismatch")

	uploaded, err := runner.request(http.MethodPut, blobURL,
		map[string]string{"Content-Type": transport.ContentType}, []byte(blobSource))
	if err != nil {
		return err
	}
	if uploaded.Status != http.StatusCreated && uploaded.Status != http.StatusNoContent {
		return fmt.Errorf("neutral artifact blob upload HTTP %d", uploaded.Status)
	}
	first, err := runner.commitArtifact(uploadID, "key-neutral-artifact-commit")
	if err != nil {
		return err
	}
	var committed struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if first.Status != http.StatusOK && first.Status != http.StatusCreated {
		return fmt.Errorf("neutral artifact commit HTTP %d: %s", first.Status, strings.TrimSpace(string(first.Body)))
	}
	if err := decodeStrictResponse(first, &committed); err != nil {
		return err
	}
	if committed.ManifestDigest != wantDigest {
		return fmt.Errorf("neutral artifact digest = %s, want %s", committed.ManifestDigest, wantDigest)
	}
	second, err := runner.commitArtifact(uploadID, "key-neutral-artifact-commit-again")
	if err != nil {
		return err
	}
	var replay struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if second.Status != http.StatusOK && second.Status != http.StatusCreated {
		return fmt.Errorf("neutral artifact recommit HTTP %d", second.Status)
	}
	if err := decodeStrictResponse(second, &replay); err != nil {
		return err
	}
	if replay.ManifestDigest != wantDigest {
		return errors.New("neutral artifact recommit changed its digest")
	}
	runner.artifactManifestDigest = wantDigest
	runner.complete("artifact-commit-idempotent")

	files, _ := manifest["files"].([]any)
	file, _ := files[0].(map[string]any)
	if file == nil {
		return errors.New("neutral artifact fixture omitted its payload")
	}
	mislabelled := cloneJSONMap(file)
	mislabelled["size"] = len(blobSource) + 1
	sizeUploadID, sizeMissing, err := runner.startArtifactUpload(map[string]any{
		"apiVersion": artifactAPIVersion, "kind": manifest["kind"],
		"files": []any{mislabelled},
	}, "key-neutral-artifact-size-lie")
	if err != nil {
		return err
	}
	if len(sizeMissing) != 0 {
		return fmt.Errorf("neutral size-binding missingBlobs = %v, want none", sizeMissing)
	}
	sizeCommit, err := runner.commitArtifact(sizeUploadID, "key-neutral-artifact-size-commit")
	if err != nil {
		return err
	}
	if err := runner.expectStableError(sizeCommit, "artifact_invalid"); err != nil {
		return fmt.Errorf("neutral artifact declared size binding: %w", err)
	}
	head, err := runner.request(http.MethodHead,
		runner.apiBase+"/artifacts/blobs/"+url.PathEscape(blobDigest), nil, nil)
	if err != nil {
		return err
	}
	if head.Status != http.StatusOK {
		return fmt.Errorf("neutral rejected size commit disturbed blob: HTTP %d", head.Status)
	}
	runner.complete("artifact-commit-binds-declared-size")
	return nil
}

func stableGenericHostContract(
	corpus stableGenericCorpus,
	probe stableGenericHostProbe,
	snapshot *currentformsnapshot.Snapshot,
) (Contract, error) {
	lane, known := laneFor(corpus.HostAPILane)
	if !known || lane.APIVersion != stableLane.APIVersion {
		return Contract{}, fmt.Errorf("generic Host probe names unknown lane %q", corpus.HostAPILane)
	}
	services := probe.ExternalServices
	for _, protocol := range []string{services.SupportedProtocol, services.UnsupportedProtocol} {
		if err := formpackage.ValidateStandardServiceRef(formpackage.StandardServiceRef{
			APIVersion: services.ServiceAPIVersion, Protocol: protocol,
		}); err != nil {
			return Contract{}, err
		}
	}
	if services.Property != "externalServices" ||
		services.ServiceAPIVersion != formpackage.StandardServiceAPIVersion ||
		services.SupportedProtocol == services.UnsupportedProtocol ||
		len(services.SupportedDesired) == 0 || len(services.RequiredUnsupportedDesired) == 0 ||
		len(services.OptionalUnsupportedDesired) == 0 {
		return Contract{}, errors.New("stable generic standard-service probe is incomplete")
	}
	contract := Contract{
		Format: lane.ContractFormat, APIVersion: lane.APIVersion,
		DiscoveryPath: lane.DiscoveryPath, APIPath: lane.APIPath, lane: lane,
		ErrorEnvelope: ErrorEnvelopeContract{
			Codes:                  append([]string(nil), lane.ErrorCodeOrder...),
			AutomaticallyRetryable: append([]string(nil), automaticallyRetryableCodes...),
			HTTPStatusByCode:       map[string]int{},
		},
	}
	for code, status := range lane.ErrorHTTPStatus {
		contract.ErrorEnvelope.HTTPStatusByCode[code] = status
	}
	contract.RunnerInput.Space = probe.Space
	contract.RunnerInput.AlternateSpace = probe.Space + "-alt"
	primaryInput := stableGenericResourceProbe{
		FormRef: probe.FormRef, Name: probe.Name, Desired: probe.Desired,
	}
	primary, err := stableGenericResourceFromSnapshot(snapshot, primaryInput)
	if err != nil {
		return Contract{}, err
	}
	keyed, err := stableGenericResourceFromSnapshot(snapshot, probe.Resources.Keyed)
	if err != nil {
		return Contract{}, err
	}
	sequenced, err := stableGenericResourceFromSnapshot(snapshot, probe.Resources.Sequenced)
	if err != nil {
		return Contract{}, err
	}
	revision, err := stableGenericResourceFromSnapshot(snapshot, probe.Resources.Revision)
	if err != nil {
		return Contract{}, err
	}
	output, err := stableGenericResourceFromSnapshot(snapshot, probe.Resources.Output)
	if err != nil {
		return Contract{}, err
	}
	lease, err := stableGenericResourceFromSnapshot(snapshot, probe.Resources.Lease)
	if err != nil {
		return Contract{}, err
	}
	reservation, err := stableGenericResourceFromSnapshot(snapshot, probe.Resources.Reservation)
	if err != nil {
		return Contract{}, err
	}
	secondDefinition, _, err := stableGenericSnapshotDefinition(snapshot, probe.SyntheticSecondDefinitionVersion)
	if err != nil {
		return Contract{}, err
	}
	second := SyntheticDefinitionProbe{
		Name: "neutral-second-definition", FormRef: probe.SyntheticSecondDefinitionVersion,
		Path: "snapshot://neutral-second-definition", SHA256: probe.SyntheticSecondDefinitionVersion.SchemaDigest,
		Definition: &secondDefinition,
	}
	constraintDefinition := func(input stableGenericConstraintProbe) (ConstraintDefinitionProbe, error) {
		definition, _, err := stableGenericSnapshotDefinition(snapshot, input.FormRef)
		if err != nil {
			return ConstraintDefinitionProbe{}, err
		}
		return ConstraintDefinitionProbe{
			Name: input.Name, FormRef: input.FormRef,
			Path: "snapshot://" + input.Name, SHA256: input.FormRef.SchemaDigest,
			Definition: &definition,
		}, nil
	}
	constraints := ConstraintSemanticsProbe{}
	for _, entry := range []struct {
		input       stableGenericConstraintProbe
		destination *ConstraintDefinitionProbe
	}{
		{probe.ConstraintSemantics.Node, &constraints.Node},
		{probe.ConstraintSemantics.DistinctPair, &constraints.DistinctPair},
		{probe.ConstraintSemantics.UniquePair, &constraints.UniquePair},
		{probe.ConstraintSemantics.UniquePairSecond, &constraints.UniquePairSecond},
		{probe.ConstraintSemantics.Member, &constraints.Member},
		{probe.ConstraintSemantics.SameTarget, &constraints.SameTarget},
		{probe.ConstraintSemantics.Structural, &constraints.Structural},
	} {
		value, err := constraintDefinition(entry.input)
		if err != nil {
			return Contract{}, err
		}
		*entry.destination = value
	}
	negativeRaw, err := json.Marshal(probe.InvalidSchemaDesired)
	if err != nil {
		return Contract{}, err
	}
	negativeFixtures := []NegativeFixture{{
		Name: "neutral-schema-negative", Kind: probe.FormRef.Kind, Stage: "desired",
		SHA256: formpackage.DigestBytes(negativeRaw), Input: cloneJSONMap(probe.InvalidSchemaDesired),
	}}
	externalServices := ExternalServiceProbe{
		Property: services.Property, ServiceAPIVersion: services.ServiceAPIVersion,
		Protocols:   []string{services.SupportedProtocol},
		DesiredSpec: services.SupportedDesired, UnknownProtocolSpec: services.RequiredUnsupportedDesired,
		OptionalUnsupportedSpec: services.OptionalUnsupportedDesired,
	}
	contract.genericRoles = &genericSemanticRoles{
		Primary: primary, Keyed: keyed, Sequenced: sequenced, Revision: revision,
		Artifact: genericArtifactTransport{
			BlobSource: probe.ArtifactTransport.BlobSource, DeclaredSize: probe.ArtifactTransport.DeclaredSize,
			ContentType: probe.ArtifactTransport.ContentType,
		},
		Output: output, ExclusiveSubjects: []ResourceProbe{lease, reservation},
		SecondGroup: probe.SyntheticSecondGroup, SecondDefinition: second,
		Constraints: constraints, NegativeFixtures: negativeFixtures,
		ExternalServices: externalServices,
		SupportInterface: probe.Support.Interface, SupportBinding: probe.Support.Binding,
	}
	return contract, nil
}

func stableGenericSnapshotDefinition(
	snapshot *currentformsnapshot.Snapshot,
	ref FormRef,
) (formpackage.FormDefinition, string, error) {
	packageRef := formpackageFormRef(ref)
	raw, ok := snapshot.Definition(packageRef)
	if !ok {
		return formpackage.FormDefinition{}, "", fmt.Errorf("generic Snapshot omitted exact FormRef %+v", ref)
	}
	definition, err := formpackage.ValidateDefinition(raw)
	if err != nil {
		return formpackage.FormDefinition{}, "", err
	}
	for _, form := range snapshot.Forms() {
		if form.Ref == packageRef {
			return definition, form.PackageDigest, nil
		}
	}
	return formpackage.FormDefinition{}, "", fmt.Errorf("generic Snapshot omitted package evidence for %+v", ref)
}

func stableGenericResourceFromSnapshot(
	snapshot *currentformsnapshot.Snapshot,
	input stableGenericResourceProbe,
) (ResourceProbe, error) {
	if input.Name == "" || len(input.Desired) == 0 {
		return ResourceProbe{}, errors.New("generic resource probe is incomplete")
	}
	definition, packageDigest, err := stableGenericSnapshotDefinition(snapshot, input.FormRef)
	if err != nil {
		return ResourceProbe{}, err
	}
	materialized, err := stableGenericMaterialize(snapshot, input.FormRef, input.Desired)
	if err != nil {
		return ResourceProbe{}, err
	}
	return ResourceProbe{
		Name:                  input.Name,
		Identity:              InstalledFormReference{FormRef: input.FormRef, PackageDigest: packageDigest},
		LifecycleCapabilities: append([]string(nil), definition.LifecycleCapabilities...),
		Desired:               materialized,
		DesiredSchema:         PinnedSchema{Schema: cloneJSONMap(definition.DesiredSchema)},
		Constraints:           append([]formpackage.FormConstraint(nil), definition.Constraints...),
		DeclaredOutputSchema:  cloneJSONMap(definition.OutputSchema),
	}, nil
}

func stableGenericCatalog(snapshot *currentformsnapshot.Snapshot, probe *stableGenericHostProbe) (*Catalog, error) {
	catalog := newCatalog(snapshot.HostAPI())
	metadata := map[FormRef]stableGenericResourceProbe{}
	if probe != nil {
		metadata[probe.Resources.Output.FormRef] = probe.Resources.Output
	}
	for _, compiled := range snapshot.Forms() {
		raw, ok := snapshot.Definition(compiled.Ref)
		if !ok {
			return nil, errors.New("compiled generic Snapshot omitted one Definition")
		}
		definition, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			return nil, err
		}
		installed := &InstalledForm{
			Ref:                portableFormRef(compiled.Ref),
			PackageDigest:      compiled.PackageDigest,
			Role:               definition.Role,
			Title:              definition.Title,
			Description:        definition.Description,
			DesiredSchema:      definition.DesiredSchema,
			OutputSchema:       definition.OutputSchema,
			Lifecycle:          definition.LifecycleCapabilities,
			ProvidedInterfaces: definition.ProvidedInterfaces,
			AcceptedBindings:   definition.AcceptedBindings,
			RequiresHostAPI:    definition.RequiresHostAPI,
			Constraints:        definition.Constraints,
		}
		if input, ok := metadata[portableFormRef(compiled.Ref)]; ok {
			installed.HostAssignedOutputs = cloneJSONMap(input.HostAssignedOutputs)
		}
		if err := catalog.install(installed); err != nil {
			return nil, err
		}
	}
	for _, compiled := range snapshot.Interfaces() {
		raw, ok := snapshot.InterfaceDefinition(compiled.Ref)
		if !ok {
			return nil, errors.New("compiled generic Snapshot omitted one Interface Definition")
		}
		var document interfaceDefinitionDocument
		if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
			return nil, err
		}
		key := compiled.Ref.Name + "@" + compiled.Ref.Version
		catalog.interfaces[key] = supportRef{
			Name: compiled.Ref.Name, Version: compiled.Ref.Version, SchemaDigest: compiled.Ref.SchemaDigest,
		}
		catalog.abis[key] = interfaceContract{Ref: compiled.Ref, Handlers: runtimeHandlerVocabulary(document)}
	}
	for _, compiled := range snapshot.Bindings() {
		raw, ok := snapshot.BindingDefinition(compiled.Ref)
		if !ok {
			return nil, errors.New("compiled generic Snapshot omitted one Binding Definition")
		}
		var document bindingDefinitionDocument
		if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
			return nil, err
		}
		key := compiled.Ref.Name + "@" + compiled.Ref.Version
		catalog.bindings[key] = supportRef{
			Name: compiled.Ref.Name, Version: compiled.Ref.Version, SchemaDigest: compiled.Ref.SchemaDigest,
		}
		catalog.contracts[key] = bindingContract{
			Ref: compiled.Ref, SourceRole: document.SourceRole,
			TargetInterface:    document.TargetInterface,
			AllowedTargetForms: append([]allowedTargetForm(nil), document.AllowedTargetForms...),
		}
	}
	return catalog, nil
}

func stableCheckZeroFamilyHost(ctx context.Context, contract Contract, catalog *Catalog) error {
	host := NewReferenceHost(contract, catalog)
	server := httptest.NewServer(host)
	defer server.Close()
	runner := stableGenericRunner(ctx, contract, server)
	response, err := runner.request(http.MethodGet, runner.apiBase+"/forms?space="+url.QueryEscape(contract.RunnerInput.Space), nil, nil)
	if err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf("zero-family /forms HTTP %d", response.Status)
	}
	var answer struct {
		Forms []json.RawMessage `json:"forms"`
	}
	if err := decodeStrictResponse(response, &answer); err != nil {
		return err
	}
	if len(answer.Forms) != 0 {
		return fmt.Errorf("zero-family Host enumerated %d Forms", len(answer.Forms))
	}
	return nil
}

func stableGenericRunner(ctx context.Context, contract Contract, server *httptest.Server) *v3Runner {
	client := *server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &v3Runner{
		ctx: ctx, contract: contract, endpoint: server.URL,
		token: referencePrimaryToken, alternateToken: referenceAlternateToken,
		alternateTenantToken: referenceAlternateTenantToken,
		httpClient:           &client, apiBase: server.URL + contract.APIPath,
		completed: map[string]bool{}, desiredSchemas: map[FormRef]map[string]any{},
		desiredMutations: map[FormRef][]map[string]any{},
	}
}

func stableGenericMaterialize(
	snapshot *currentformsnapshot.Snapshot,
	ref FormRef,
	desired map[string]any,
) (map[string]any, error) {
	raw, err := json.Marshal(desired)
	if err != nil {
		return nil, err
	}
	materialized, err := snapshot.Materialize(formpackageFormRef(ref), raw)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := formpackage.DecodeStrictIJSON(materialized, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func portableFormRef(ref formpackage.FormRef) FormRef {
	return FormRef{
		APIVersion: ref.APIVersion, Kind: ref.Kind,
		DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
	}
}

func formpackageFormRef(ref FormRef) formpackage.FormRef {
	return formpackage.FormRef{
		APIVersion: ref.APIVersion, Kind: ref.Kind,
		DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
	}
}
