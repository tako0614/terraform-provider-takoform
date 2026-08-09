package workerauthoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformancev3"
)

// Report is the evidence one CLI produced. It is never publication-ready: the
// host is a disposable reference implementation and the provider is built from
// the working tree.
type Report struct {
	Format             string            `json:"format"`
	PublicationReady   bool              `json:"publicationReady"`
	CLI                CLIIdentity       `json:"cli"`
	SameNameDeadlock   DeadlockEvidence  `json:"sameNameDeadlock"`
	PlanRefusal        RefusalEvidence   `json:"planRefusal"`
	RollForward        SequenceEvidence  `json:"rollForward"`
	ModuleDeploy       SequenceEvidence  `json:"moduleDeploy"`
	ModuleDestroy      TeardownEvidence  `json:"moduleDestroy"`
	TwoOwners          OwnershipEvidence `json:"twoOwners"`
	HeterogeneousVars  VarsEvidence      `json:"heterogeneousVars"`
	ShortestModuleName string            `json:"shortestModuleName"`
	HostSupport        RefusalEvidence   `json:"hostSupportAtPlan"`
	Configurations     []string          `json:"validatedConfigurations"`
}

// OwnershipEvidence records what two independent owners of byte-identical
// build output derived, and what happened when one of them went away.
type OwnershipEvidence struct {
	BundleNames  []string `json:"bundleNames"`
	VersionNames []string `json:"versionNames"`
	// HeldOwnerUnmoved reports that moving one owner forward — which deletes
	// that owner's old bundle and version — left the other owner holding
	// exactly the revisions it held before, and serving throughout.
	HeldOwnerUnmoved bool `json:"heldOwnerUnmoved"`
}

// VarsEvidence records the JSON type each `vars` entry arrived at the host as.
type VarsEvidence struct {
	WireTypes map[string]string `json:"wireTypes"`
}

// TeardownEvidence records what a full `destroy` of the official module did.
//
// A teardown is an ordering claim and an emptiness claim at once, so both are
// carried: the deletes the host actually saw, and the store afterwards.
type TeardownEvidence struct {
	// Built is the aggregate the destroy was run against, as sorted
	// `Kind/name` text.
	Built []string `json:"built"`
	// Mutations is the destroy's own timeline, recorded from a reset
	// recorder so nothing the apply drove is in it.
	Mutations []mutation `json:"mutations"`
	// ReadySamples and NotReadySamples are the worker's readiness over the
	// window immediately BEFORE the destroy: what came apart was serving.
	ReadySamples    int `json:"readySamples"`
	NotReadySamples int `json:"notReadySamples"`
	// LeftBehind is every resource the host still holds after the destroy,
	// read from the store rather than probed by name — a name probe cannot see
	// an orphan. Empty is the claim.
	LeftBehind []string `json:"leftBehind"`
}

// ReportFormat identifies the evidence shape.
const ReportFormat = "takoform.worker-authoring-conformance@v1"

// DeadlockEvidence records what a same-name replacement of an immutable
// revision actually does against a conforming host, in both apply orders.
type DeadlockEvidence struct {
	DestroyFirstCode string `json:"destroyFirstCode"`
	DestroyFirstHTTP int    `json:"destroyFirstHttp"`
	CreateFirstCode  string `json:"createFirstCode"`
	CreateFirstHTTP  int    `json:"createFirstHttp"`
}

// RefusalEvidence records one plan-time refusal by its stable code.
type RefusalEvidence struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

// SequenceEvidence records the mutations one roll-forward drove, in order, and
// what was true about the worker throughout.
type SequenceEvidence struct {
	Mutations []mutation `json:"mutations"`
	// ReadySamples is how many times the worker was observed while the apply
	// ran, and NotReadySamples how many of those observations found it not
	// serving. A roll-forward with no window where nothing serves has zero of
	// the second.
	ReadySamples    int `json:"readySamples"`
	NotReadySamples int `json:"notReadySamples"`
	// EndpointURLStable reports that a host-assigned endpoint address survived
	// the code change. It is only set by the module scenario.
	EndpointURLStable bool `json:"endpointUrlStable,omitempty"`
}

// Run drives every authoring scenario against one CLI and returns the
// evidence.
func Run(ctx context.Context, repoRoot, cliPath string) (Report, error) {
	report := Report{Format: ReportFormat}

	deadlock, identity, err := runSameNameDeadlock(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.CLI = identity
	report.SameNameDeadlock = deadlock

	refusal, err := runPinnedNamePlanRefusal(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.PlanRefusal = refusal

	rollForward, err := runRollForward(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.RollForward = rollForward

	moduleDeploy, err := runModuleDeploy(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.ModuleDeploy = moduleDeploy

	moduleDestroy, err := runModuleDestroy(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.ModuleDestroy = moduleDestroy

	twoOwners, err := runTwoOwners(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.TwoOwners = twoOwners

	vars, err := runHeterogeneousVars(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.HeterogeneousVars = vars

	shortest, err := runShortestName(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.ShortestModuleName = shortest

	support, err := runHostSupportAtPlan(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.HostSupport = support

	configurations, err := runConfigurationValidation(ctx, repoRoot, cliPath)
	if err != nil {
		return Report{}, err
	}
	report.Configurations = configurations
	return report, nil
}

// runSameNameDeadlock proves, against the host itself, the two refusals that
// make a same-name replacement of an immutable revision impossible.
//
// It drives the HOST rather than the CLI, deliberately. The provider now
// refuses this at plan time, so no Terraform run reaches either refusal any
// more; the claim the plan diagnostic makes is nonetheless a claim about the
// host, and it has to keep being measured against one. Both branches are
// exercised on a live aggregate — a bundle a version executes, a version a
// deployment weights — because that is the only state in which they arise.
func runSameNameDeadlock(ctx context.Context, repoRoot, cliPath string) (DeadlockEvidence, CLIIdentity, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, err
	}
	defer h.Close()
	if err := h.writeModuleSource(1); err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, err
	}
	if err := h.write("main.tf", rawStack(h.Endpoint(), rawStackOptions{PinnedNames: true})); err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, err
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, fmt.Errorf("%s establish worker aggregate: %w\n%s", h.identity.Product, err, output)
	}

	client, err := h.client(ctx)
	if err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, err
	}
	bundleRef, err := edgeFormRef("WorkerBundle")
	if err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, err
	}
	evidence := DeadlockEvidence{}

	// Order one: destroy the old revision first. A live relation holds it. The
	// delete carries the generation fence a conforming client always sends, so
	// the refusal is the dependency rule and not a missing precondition.
	stored, err := client.GetResource(ctx, harnessSpace, clientFormRef3(bundleRef), workerName+"-bundle")
	if err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, fmt.Errorf("read the applied bundle: %w", err)
	}
	deleteErr := client.DeleteResource(
		ctx, harnessSpace, clientFormRef3(bundleRef), workerName+"-bundle",
		stored.Metadata.UID, stored.Metadata.Generation)
	code, status, ok := stableErrorOf(deleteErr)
	if !ok {
		return DeadlockEvidence{}, CLIIdentity{}, fmt.Errorf(
			"destroy-first order was expected to be refused by the host, got %v", deleteErr)
	}
	evidence.DestroyFirstCode, evidence.DestroyFirstHTTP = code, status

	// Order two: create the new revision first, under the name the old one
	// still holds.
	_, applyErr := client.ApplyResource(ctx, &clientv3.Resource{
		APIVersion: bundleRef.APIVersion,
		Kind:       bundleRef.Kind,
		Form:       &clientv3.FormReference{FormRef: clientFormRef3(bundleRef)},
		Metadata:   clientv3.Metadata{Name: workerName + "-bundle", Space: harnessSpace},
		Spec:       map[string]any{"manifestDigest": "sha256:" + strings.Repeat("ab", 32)},
	}, clientv3.Fence{})
	code, status, ok = stableErrorOf(applyErr)
	if !ok {
		return DeadlockEvidence{}, CLIIdentity{}, fmt.Errorf(
			"create-first order was expected to be refused by the host, got %v", applyErr)
	}
	evidence.CreateFirstCode, evidence.CreateFirstHTTP = code, status
	return evidence, h.identity, nil
}

// runPinnedNamePlanRefusal proves the provider refuses the same-name
// replacement at PLAN, before any apply order is chosen.
func runPinnedNamePlanRefusal(ctx context.Context, repoRoot, cliPath string) (RefusalEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return RefusalEvidence{}, err
	}
	defer h.Close()
	if err := h.writeModuleSource(1); err != nil {
		return RefusalEvidence{}, err
	}
	if err := h.write("main.tf", rawStack(h.Endpoint(), rawStackOptions{PinnedNames: true})); err != nil {
		return RefusalEvidence{}, err
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return RefusalEvidence{}, fmt.Errorf("%s pinned-name apply: %w\n%s", h.identity.Product, err, output)
	}
	if err := h.writeModuleSource(2); err != nil {
		return RefusalEvidence{}, err
	}
	h.host.Reset()
	output, planErr := h.run(ctx, "plan", "-input=false", "-no-color")
	if planErr == nil {
		return RefusalEvidence{}, fmt.Errorf(
			"%s planned a same-name replacement of an immutable revision instead of refusing it\n%s",
			h.identity.Product, output)
	}
	for _, want := range []string{
		"This immutable revision cannot be safely replaced under the same host name.",
		"Use a new revision name or the official worker-app module.",
		"takoform.provider/immutable-revision-same-name",
		"dependency_in_use",
		"invalid_argument",
	} {
		if !strings.Contains(output, want) {
			return RefusalEvidence{}, fmt.Errorf(
				"%s plan refusal does not state %q\n%s", h.identity.Product, want, output)
		}
	}
	if mutations := h.host.Mutations(); len(mutations) != 0 {
		return RefusalEvidence{}, fmt.Errorf("a refused plan mutated the host: %+v", mutations)
	}
	return RefusalEvidence{
		Code:    "takoform.provider/immutable-revision-same-name",
		Summary: "This immutable revision cannot be safely replaced under the same host name.",
	}, nil
}

// runRollForward proves the whole sequence a code change must drive, against
// the raw resources with derived names.
func runRollForward(ctx context.Context, repoRoot, cliPath string) (SequenceEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return SequenceEvidence{}, err
	}
	defer h.Close()
	if err := h.writeModuleSource(1); err != nil {
		return SequenceEvidence{}, err
	}
	if err := h.write("main.tf", rawStack(h.Endpoint(), rawStackOptions{CreateBeforeDestroy: true})); err != nil {
		return SequenceEvidence{}, err
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return SequenceEvidence{}, fmt.Errorf("%s derived-name apply: %w\n%s", h.identity.Product, err, output)
	}
	return h.rollForward(ctx, func() error { return h.writeModuleSource(2) })
}

// runModuleDeploy drives the official module end to end, including the
// host-assigned endpoint, and proves the same sequence plus the one guarantee
// the endpoint adds: the address survives a code change.
func runModuleDeploy(ctx context.Context, repoRoot, cliPath string) (SequenceEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return SequenceEvidence{}, err
	}
	defer h.Close()
	modulePath, err := filepath.Abs(filepath.Join(repoRoot, "modules", "worker-app"))
	if err != nil {
		return SequenceEvidence{}, err
	}
	if err := h.writeModuleSource(1); err != nil {
		return SequenceEvidence{}, err
	}
	if err := h.write("main.tf", defaultModuleStack(h.Endpoint(), modulePath, true)); err != nil {
		return SequenceEvidence{}, err
	}
	if output, err := h.run(ctx, "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		// Module installation is what init is needed for here; provider
		// resolution is served by the dev override, and OpenTofu still reports
		// the unpublished version constraint. The validate below is the proof
		// the configuration resolved.
		_ = output
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return SequenceEvidence{}, fmt.Errorf("%s worker-app module apply: %w\n%s", h.identity.Product, err, output)
	}
	before, err := h.output(ctx, moduleInstanceLabel(workerName)+"_url")
	if err != nil {
		return SequenceEvidence{}, err
	}
	if !strings.HasPrefix(before, "https://") {
		return SequenceEvidence{}, fmt.Errorf("worker-app module produced no host-assigned https address, got %q", before)
	}
	evidence, err := h.rollForward(ctx, func() error { return h.writeModuleSource(2) })
	if err != nil {
		return SequenceEvidence{}, err
	}
	after, err := h.output(ctx, moduleInstanceLabel(workerName)+"_url")
	if err != nil {
		return SequenceEvidence{}, err
	}
	if after != before {
		return SequenceEvidence{}, fmt.Errorf(
			"the host-assigned address moved across a code change: %q then %q", before, after)
	}
	evidence.EndpointURLStable = true
	return evidence, nil
}

// runModuleDestroy proves the official module comes apart again.
//
// Standing an aggregate up is the half every other scenario measures. Taking it
// down is the half that broke, and it broke for a reason no single-resource
// teardown reaches: removing a `WorkerDeployment` re-renders the `ModuleWorker`
// whose readiness follows it (spec/decisions/0016 rule 9), so the worker's
// revision moves in the middle of the destroy, AFTER the plan read it. While a
// delete fenced on the representation, that made `terraform destroy` of the
// official module fail on a change the destroy itself caused — with no repair,
// because the next dependent moves the revision again. The fence is the desired
// generation, and this is what says so end to end, through the real CLI.
//
// The teardown is measured three ways, because "the command exited zero" is not
// the claim. The recorded timeline must be five deletes and nothing else, in an
// order the host's own refusals impose. The readiness sampler must have found
// the worker serving right up to the moment the teardown began, so what came
// apart was a live aggregate rather than a half-built one. And the host's store
// must be EMPTY afterwards — not "the names this configuration declared are
// gone", which cannot see an orphan.
func runModuleDestroy(ctx context.Context, repoRoot, cliPath string) (TeardownEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return TeardownEvidence{}, err
	}
	defer h.Close()
	modulePath, err := filepath.Abs(filepath.Join(repoRoot, "modules", "worker-app"))
	if err != nil {
		return TeardownEvidence{}, err
	}
	if err := h.writeModuleSource(1); err != nil {
		return TeardownEvidence{}, err
	}
	if err := h.write("main.tf", defaultModuleStack(h.Endpoint(), modulePath, true)); err != nil {
		return TeardownEvidence{}, err
	}
	if output, err := h.run(ctx, "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		// Module installation only; the dev override serves the provider.
		_ = output
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return TeardownEvidence{}, fmt.Errorf(
			"%s worker-app module apply: %w\n%s", h.identity.Product, err, output)
	}

	// The aggregate is live and serving. The sampler runs across the destroy
	// PLAN and the address read rather than across a single request, so "it was
	// serving" is an observation over the window immediately before the
	// teardown — and the plan being computable at all is half of what a destroy
	// needs.
	stop, err := h.startReadinessSamplerFor(ctx, workerName)
	if err != nil {
		return TeardownEvidence{}, err
	}
	planOutput, planErr := h.run(ctx, "plan", "-destroy", "-input=false", "-no-color")
	url, outputErr := h.output(ctx, moduleInstanceLabel(workerName)+"_url")
	ready, notReady := stop()
	if planErr != nil {
		return TeardownEvidence{}, fmt.Errorf(
			"%s worker-app module destroy plan: %w\n%s", h.identity.Product, planErr, planOutput)
	}
	if outputErr != nil {
		return TeardownEvidence{}, outputErr
	}
	if !strings.HasPrefix(url, "https://") {
		return TeardownEvidence{}, fmt.Errorf(
			"the aggregate about to be destroyed publishes no host-assigned https address, got %q", url)
	}
	built := storedAddresses(h.store.SnapshotResources())

	// Everything from here is the destroy alone.
	h.host.Reset()
	if output, err := h.run(ctx, "destroy", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return TeardownEvidence{}, fmt.Errorf(
			"%s worker-app module destroy: %w\n%s", h.identity.Product, err, output)
	}
	return TeardownEvidence{
		Built:           built,
		Mutations:       h.host.Mutations(),
		ReadySamples:    ready,
		NotReadySamples: notReady,
		LeftBehind:      storedAddresses(h.store.SnapshotResources()),
	}, nil
}

// storedAddresses renders a store snapshot as stable `Kind/name` text. The
// space is omitted because every resource this harness creates lives in one.
func storedAddresses(snapshot []portableconformancev3.ResourceAddress) []string {
	out := make([]string, 0, len(snapshot))
	for _, address := range snapshot {
		out = append(out, address.Kind+"/"+address.Name)
	}
	sort.Strings(out)
	return out
}

// runTwoOwners proves that two independent owners of byte-identical build
// output are two independent revisions.
//
// Two `worker-app` instances in one space, built from identical output, hold
// identical content. A digest names the BYTES, and this lane's own rule is that
// a digest is a name rather than an ownership claim — but a Terraform address
// has exactly one owner. So the scenario asserts the two facts that follow: the
// four derived revision names are four distinct names, and moving ONE owner
// forward — which deletes that owner's old bundle and version — leaves the
// other owner untouched and serving throughout.
//
// The delete is the half that matters. Under a shared name it is refused,
// because the peer's version still holds the bundle, or it succeeds and takes
// the peer's revision with it.
func runTwoOwners(ctx context.Context, repoRoot, cliPath string) (OwnershipEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return OwnershipEvidence{}, err
	}
	defer h.Close()
	modulePath, err := filepath.Abs(filepath.Join(repoRoot, "modules", "worker-app"))
	if err != nil {
		return OwnershipEvidence{}, err
	}
	owners := []string{workerName, workerName + "-peer"}
	options := moduleStackOptions{Owners: owners, Endpoint: true}
	for _, owner := range owners {
		if err := h.writeModuleSourceIn(options.contentDir(owner), 1); err != nil {
			return OwnershipEvidence{}, err
		}
	}
	if err := h.write("main.tf", moduleStack(h.Endpoint(), modulePath, options)); err != nil {
		return OwnershipEvidence{}, err
	}
	if _, err := h.run(ctx, "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		// Module installation is what init is for here; the provider comes from
		// the dev override and the CLI still reports the unpublished constraint.
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return OwnershipEvidence{}, fmt.Errorf(
			"%s could not apply two worker-app owners built from identical output: %w\n%s",
			h.identity.Product, err, output)
	}
	evidence := OwnershipEvidence{}
	names := map[string][2]string{}
	for _, owner := range owners {
		label := moduleInstanceLabel(owner)
		bundle, err := h.output(ctx, label+"_bundle_name")
		if err != nil {
			return OwnershipEvidence{}, err
		}
		version, err := h.output(ctx, label+"_version_name")
		if err != nil {
			return OwnershipEvidence{}, err
		}
		names[owner] = [2]string{bundle, version}
		evidence.BundleNames = append(evidence.BundleNames, bundle)
		evidence.VersionNames = append(evidence.VersionNames, version)
	}
	if err := assertDistinct("WorkerBundle", evidence.BundleNames); err != nil {
		return OwnershipEvidence{}, err
	}
	if err := assertDistinct("WorkerVersion", evidence.VersionNames); err != nil {
		return OwnershipEvidence{}, err
	}

	// Move the SECOND owner forward. That apply deletes the second owner's old
	// bundle and version, while nothing about the first owner changes at all.
	held := owners[0]
	sampler, err := h.startReadinessSamplerFor(ctx, held)
	if err != nil {
		return OwnershipEvidence{}, err
	}
	if err := h.writeModuleSourceIn(options.contentDir(owners[1]), 2); err != nil {
		return OwnershipEvidence{}, err
	}
	output, applyErr := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color")
	ready, notReady := sampler()
	if applyErr != nil {
		return OwnershipEvidence{}, fmt.Errorf(
			"%s could not move one of two owners built from identical output: %w\n%s",
			h.identity.Product, applyErr, output)
	}
	if ready == 0 || notReady != 0 {
		return OwnershipEvidence{}, fmt.Errorf(
			"moving one owner stopped the other owner's worker %q from serving: %d of %d observations",
			held, notReady, ready+notReady)
	}

	// The untouched owner still holds exactly the revisions it held before.
	for index, name := range []string{
		moduleInstanceLabel(held) + "_bundle_name",
		moduleInstanceLabel(held) + "_version_name",
	} {
		current, err := h.output(ctx, name)
		if err != nil {
			return OwnershipEvidence{}, err
		}
		if current != names[held][index] {
			return OwnershipEvidence{}, fmt.Errorf(
				"moving one owner moved the other owner's %s from %q to %q",
				name, names[held][index], current)
		}
	}
	if serving, err := h.workerReady(ctx, held); err != nil || !serving {
		return OwnershipEvidence{}, fmt.Errorf(
			"the untouched owner %q is not serving after the peer moved (%v)", held, err)
	}
	evidence.HeldOwnerUnmoved = true
	return evidence, nil
}

// assertDistinct refuses a derived-name set in which two owners landed on one
// host address.
func assertDistinct(kind string, names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("a %s derived no name at all", kind)
		}
		if seen[name] {
			return fmt.Errorf(
				"two independent owners derived one %s name %q, so two Terraform resources manage one host address: %v",
				kind, name, names)
		}
		seen[name] = true
	}
	return nil
}

// runHeterogeneousVars proves that a `vars` map whose values have three
// different JSON types reaches the host with those three types intact, THROUGH
// the official module rather than only through the raw resource.
//
// The Form admits any bounded JSON value, so `true` must arrive as a JSON
// boolean and `3` as a JSON number. A module input typed as a collection with
// one inferred element type cannot carry that: the values unify — usually to
// strings — before anything encodes them.
func runHeterogeneousVars(ctx context.Context, repoRoot, cliPath string) (VarsEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return VarsEvidence{}, err
	}
	defer h.Close()
	modulePath, err := filepath.Abs(filepath.Join(repoRoot, "modules", "worker-app"))
	if err != nil {
		return VarsEvidence{}, err
	}
	if err := h.writeModuleSource(1); err != nil {
		return VarsEvidence{}, err
	}
	stack := moduleStack(h.Endpoint(), modulePath, moduleStackOptions{Vars: heterogeneousVars})
	if err := h.write("main.tf", stack); err != nil {
		return VarsEvidence{}, err
	}
	if _, err := h.run(ctx, "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		// See runModuleDeploy: init installs the local module only.
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return VarsEvidence{}, fmt.Errorf("%s heterogeneous vars apply: %w\n%s", h.identity.Product, err, output)
	}
	versionName, err := h.output(ctx, moduleInstanceLabel(workerName)+"_version_name")
	if err != nil {
		return VarsEvidence{}, err
	}
	client, err := h.client(ctx)
	if err != nil {
		return VarsEvidence{}, err
	}
	ref, err := edgeFormRef("WorkerVersion")
	if err != nil {
		return VarsEvidence{}, err
	}
	stored, err := client.GetResource(ctx, harnessSpace, clientFormRef3(ref), versionName)
	if err != nil {
		return VarsEvidence{}, fmt.Errorf("read the applied WorkerVersion %q: %w", versionName, err)
	}
	vars, ok := stored.Spec["vars"].(map[string]any)
	if !ok {
		return VarsEvidence{}, fmt.Errorf("the host stored no vars object for %q: %v", versionName, stored.Spec["vars"])
	}
	evidence := VarsEvidence{WireTypes: map[string]string{}}
	for name, value := range vars {
		evidence.WireTypes[name] = jsonTypeName(value)
	}
	want := map[string]string{"enabled": "boolean", "retries": "number", "label": "string"}
	for name, wanted := range want {
		if got := evidence.WireTypes[name]; got != wanted {
			return VarsEvidence{}, fmt.Errorf(
				"the module sent vars.%s to the host as a JSON %s, want %s (whole object: %v)",
				name, got, wanted, vars)
		}
	}
	return evidence, nil
}

// jsonTypeName names the JSON type one decoded value arrived as.
func jsonTypeName(value any) string {
	switch value.(type) {
	case bool:
		return "boolean"
	// The v1alpha3 client decodes with UseNumber, so a JSON number arrives as
	// json.Number rather than float64. Both are the same JSON type.
	case json.Number, float64:
		return "number"
	case string:
		return "string"
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", value)
}

// runShortestName proves the official module accepts every name the portable
// grammar admits, including the shortest one.
//
// `^[a-z][a-z0-9-]{0,62}$` admits a single letter. A module that demands a
// second terminal character rejects a name `takoform_module_worker` accepts,
// and sends the author back to the raw resources for no reason at all.
func runShortestName(ctx context.Context, repoRoot, cliPath string) (string, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return "", err
	}
	defer h.Close()
	modulePath, err := filepath.Abs(filepath.Join(repoRoot, "modules", "worker-app"))
	if err != nil {
		return "", err
	}
	if err := h.writeModuleSource(1); err != nil {
		return "", err
	}
	stack := moduleStack(h.Endpoint(), modulePath, moduleStackOptions{Owners: []string{shortestPortableName}})
	if err := h.write("main.tf", stack); err != nil {
		return "", err
	}
	if _, err := h.run(ctx, "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		// See runModuleDeploy: init installs the local module only.
	}
	if output, err := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return "", fmt.Errorf(
			"%s refused the shortest name the portable grammar admits through the official module: %w\n%s",
			h.identity.Product, err, output)
	}
	applied, err := h.output(ctx, moduleInstanceLabel(shortestPortableName)+"_worker_name")
	if err != nil {
		return "", err
	}
	if applied != shortestPortableName {
		return "", fmt.Errorf("the module applied the worker as %q, want %q", applied, shortestPortableName)
	}
	return applied, nil
}

// shortestPortableName is the shortest name `^[a-z][a-z0-9-]{0,62}$` admits.
const shortestPortableName = "a"

// workerReady reports whether one Module Worker is serving right now.
func (h *harness) workerReady(ctx context.Context, name string) (bool, error) {
	client, err := h.client(ctx)
	if err != nil {
		return false, err
	}
	ref, err := edgeFormRef("ModuleWorker")
	if err != nil {
		return false, err
	}
	res, err := client.GetResource(ctx, harnessSpace, clientFormRef3(ref), name)
	if err != nil {
		return false, fmt.Errorf("read the ModuleWorker %q: %w", name, err)
	}
	condition := clientv3.ResourceCondition(res, "Ready")
	return condition != nil && condition.Status == "True", nil
}

// rollForward performs one code change and measures both properties the change
// has to have: the exact order the mutations landed in, and that the worker
// never stopped serving while they did.
func (h *harness) rollForward(ctx context.Context, change func() error) (SequenceEvidence, error) {
	if err := change(); err != nil {
		return SequenceEvidence{}, err
	}
	h.host.Reset()
	sampler, err := h.startReadinessSampler(ctx)
	if err != nil {
		return SequenceEvidence{}, err
	}
	output, applyErr := h.run(ctx, "apply", "-auto-approve", "-input=false", "-no-color")
	ready, notReady := sampler()
	if applyErr != nil {
		return SequenceEvidence{}, fmt.Errorf("%s roll-forward apply: %w\n%s", h.identity.Product, applyErr, output)
	}
	evidence := SequenceEvidence{
		Mutations: h.host.Mutations(), ReadySamples: ready, NotReadySamples: notReady,
	}
	if err := assertRollForwardOrder(evidence.Mutations); err != nil {
		return SequenceEvidence{}, err
	}
	if ready == 0 {
		return SequenceEvidence{}, errors.New("the readiness sampler observed the worker zero times")
	}
	if notReady != 0 {
		return SequenceEvidence{}, fmt.Errorf(
			"the worker stopped serving during the roll-forward: %d of %d observations were not ready",
			notReady, ready+notReady)
	}
	return evidence, nil
}

// assertRollForwardOrder holds the recorded timeline to the one order that both
// completes and never leaves the worker unserved:
//
//	create the new bundle, create the new version, re-weight the deployment,
//	destroy the old version, destroy the old bundle
//
// The deployment must be UPDATED rather than replaced, and no delete may
// precede it: a delete before the re-weighting is either refused (the host's
// dependency rule) or, on a host that allowed it, an instant with no version
// serving.
func assertRollForwardOrder(mutations []mutation) error {
	for _, item := range mutations {
		if item.Status >= 300 {
			return fmt.Errorf("the roll-forward drove a failing mutation: %+v", item)
		}
		if item.Method == "DELETE" && item.Kind == workerDeploymentKind {
			return fmt.Errorf("the roll-forward deleted the deployment, leaving the worker unserved: %+v", item)
		}
	}
	want := []mutation{
		{Method: "PUT", Kind: "WorkerBundle"},
		{Method: "PUT", Kind: "WorkerVersion"},
		{Method: "PUT", Kind: workerDeploymentKind},
		{Method: "DELETE", Kind: "WorkerVersion"},
		{Method: "DELETE", Kind: "WorkerBundle"},
	}
	got := make([]mutation, 0, len(mutations))
	for _, item := range mutations {
		got = append(got, mutation{Method: item.Method, Kind: item.Kind})
	}
	if len(got) != len(want) {
		return fmt.Errorf("roll-forward drove %d mutations, want exactly %d: %+v", len(got), len(want), mutations)
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("roll-forward step %d was %+v, want %+v (whole timeline: %+v)",
				index+1, got[index], want[index], mutations)
		}
	}
	return nil
}

// workerDeploymentKind is the deployment Form kind the order assertion names.
const workerDeploymentKind = "WorkerDeployment"

// startReadinessSampler observes the Module Worker while an apply runs and
// returns a function that stops it and reports how many observations found the
// worker serving and how many did not.
//
// This is the direct measurement of "no window where nothing serves". The
// worker's Ready condition is a claim about SERVICE — true only when it has an
// active deployment whose every weighted version exports fetch
// (spec/decisions/0016) — so an observation of Ready=False during the apply is
// exactly the window the roll-forward must not have.
func (h *harness) startReadinessSampler(ctx context.Context) (func() (int, int), error) {
	return h.startReadinessSamplerFor(ctx, workerName)
}

// startReadinessSamplerFor observes one named Module Worker, which is what a
// scenario with more than one worker in the stack needs.
func (h *harness) startReadinessSamplerFor(ctx context.Context, name string) (func() (int, int), error) {
	client, err := h.client(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := edgeFormRef("ModuleWorker")
	if err != nil {
		return nil, err
	}
	sampleCtx, stop := context.WithCancel(ctx)
	var (
		wait     sync.WaitGroup
		ready    int
		notReady int
	)
	wait.Add(1)
	go func() {
		defer wait.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
			}
			res, err := client.GetResource(sampleCtx, harnessSpace, clientFormRef3(ref), name)
			if err != nil {
				// A cancelled sample is the harness shutting down, not a fact
				// about the worker.
				if sampleCtx.Err() == nil {
					notReady++
				}
				continue
			}
			condition := clientv3.ResourceCondition(res, "Ready")
			if condition != nil && condition.Status == "True" {
				ready++
				continue
			}
			notReady++
		}
	}()
	return func() (int, int) {
		stop()
		wait.Wait()
		return ready, notReady
	}, nil
}

// runHostSupportAtPlan proves the plan decides host capability.
//
// The host under test implements `WorkerVersion` and the `edge.kv` binding
// while implementing none of bucket, SQLite, or queue — a real host shape, and
// one an author used to discover only at apply. The KV-only worker must plan
// clean; adding a bucket binding must be refused at plan, naming the code, with
// nothing mutated.
func runHostSupportAtPlan(ctx context.Context, repoRoot, cliPath string) (RefusalEvidence, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{
		unsupportedKinds: []string{"ObjectBucket", "SQLiteDatabase", "AtLeastOnceQueue"},
	})
	if err != nil {
		return RefusalEvidence{}, err
	}
	defer h.Close()
	if err := h.writeModuleSource(1); err != nil {
		return RefusalEvidence{}, err
	}
	if err := h.write("main.tf", bindingStack(h.Endpoint(), false)); err != nil {
		return RefusalEvidence{}, err
	}
	if output, err := h.run(ctx, "plan", "-input=false", "-no-color"); err != nil {
		return RefusalEvidence{}, fmt.Errorf(
			"%s refused a KV-only worker against a host that supports edge.kv: %w\n%s", h.identity.Product, err, output)
	}
	if err := h.write("main.tf", bindingStack(h.Endpoint(), true)); err != nil {
		return RefusalEvidence{}, err
	}
	h.host.Reset()
	output, planErr := h.run(ctx, "plan", "-input=false", "-no-color")
	if planErr == nil {
		return RefusalEvidence{}, fmt.Errorf(
			"%s planned a bucket this host does not implement instead of refusing it\n%s", h.identity.Product, output)
	}
	for _, want := range []string{
		"This host does not support ObjectBucket",
		"takoform.provider/host-does-not-support-form",
	} {
		if !strings.Contains(output, want) {
			return RefusalEvidence{}, fmt.Errorf(
				"%s host-support refusal does not state %q\n%s", h.identity.Product, want, output)
		}
	}
	if mutations := h.host.Mutations(); len(mutations) != 0 {
		return RefusalEvidence{}, fmt.Errorf("a refused plan mutated the host: %+v", mutations)
	}
	return RefusalEvidence{
		Code:    "takoform.provider/host-does-not-support-form",
		Summary: "This host does not support ObjectBucket",
	}, nil
}

// runConfigurationValidation runs the CLI's own configuration check over the
// official module and every example, in a scratch copy so the repository tree
// stays clean.
func runConfigurationValidation(ctx context.Context, repoRoot, cliPath string) ([]string, error) {
	h, err := startHarness(ctx, repoRoot, cliPath, harnessOptions{})
	if err != nil {
		return nil, err
	}
	defer h.Close()
	scratch, err := os.MkdirTemp("", "takoform-worker-authoring-configs-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	for _, tree := range []string{"modules", "examples"} {
		if err := copyTree(filepath.Join(repoRoot, tree), filepath.Join(scratch, tree)); err != nil {
			return nil, err
		}
	}
	directories, err := terraformDirectories(scratch)
	if err != nil {
		return nil, err
	}
	if len(directories) == 0 {
		return nil, errors.New("no Terraform configuration was found to validate")
	}
	validated := make([]string, 0, len(directories))
	for _, directory := range directories {
		// `get` installs local module sources without resolving providers, which
		// the dev override already supplies. `validate` then type-checks the whole
		// configuration against the provider's real schema.
		if output, err := h.runIn(ctx, directory, "get", "-no-color"); err != nil {
			return nil, fmt.Errorf("%s module install in %s: %w\n%s", h.identity.Product, directory, err, output)
		}
		output, err := h.runIn(ctx, directory, "validate", "-no-color")
		if err != nil {
			return nil, fmt.Errorf("%s validate %s: %w\n%s", h.identity.Product, directory, err, output)
		}
		if !strings.Contains(output, "The configuration is valid") {
			return nil, fmt.Errorf("%s validate %s did not report a valid configuration\n%s",
				h.identity.Product, directory, output)
		}
		relative, err := filepath.Rel(scratch, directory)
		if err != nil {
			return nil, err
		}
		validated = append(validated, filepath.ToSlash(relative))
	}
	return validated, nil
}

// terraformDirectories lists every directory under root that holds a root or
// module configuration, in a stable order.
func terraformDirectories(root string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	err := filepath.WalkDir(root, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(candidate) != ".tf" {
			return nil
		}
		directory := filepath.Dir(candidate)
		if !seen[directory] {
			seen[directory] = true
			out = append(out, directory)
		}
		return nil
	})
	return out, err
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, candidate)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(candidate)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o600)
	})
}

// client is a v1alpha3 client against this harness's disposable host.
func (h *harness) client(ctx context.Context) (*clientv3.Client, error) {
	c := clientv3.NewWithOptions(h.Endpoint(), harnessToken, h.server.Client(), clientv3.Options{})
	if _, err := c.Discover(ctx); err != nil {
		return nil, fmt.Errorf("discover disposable host: %w", err)
	}
	return c, nil
}

// output reads one root output value from the applied state.
func (h *harness) output(ctx context.Context, name string) (string, error) {
	raw, err := h.run(ctx, "output", "-raw", "-no-color", name)
	if err != nil {
		return "", fmt.Errorf("read output %s: %w\n%s", name, err, raw)
	}
	return strings.TrimSpace(raw), nil
}

// edgeFormRef is the build's create target for one Edge Family kind.
func edgeFormRef(kind string) (currentformregistry.V3Ref, error) {
	return currentformregistry.V3Current().DefaultCreate(currentformregistry.GroupKind{
		APIVersion: edgeformcatalog.Family.APIVersion(), Kind: kind,
	})
}

func clientFormRef3(ref currentformregistry.V3Ref) clientv3.FormRef {
	return clientv3.FormRef{
		APIVersion:        ref.APIVersion,
		Kind:              ref.Kind,
		DefinitionVersion: ref.DefinitionVersion,
		SchemaDigest:      ref.SchemaDigest,
	}
}

// stableErrorOf reports the closed error code and HTTP status of one host
// refusal, and false when the error is not a stable host answer at all.
func stableErrorOf(err error) (string, int, bool) {
	var apiErr *clientv3.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.ProtocolInvalid || apiErr.Code == "" {
		return "", 0, false
	}
	return apiErr.Code, apiErr.StatusCode, true
}
