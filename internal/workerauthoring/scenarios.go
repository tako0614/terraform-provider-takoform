package workerauthoring

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// Report is the evidence one CLI produced. It is never publication-ready: the
// host is a disposable reference implementation and the provider is built from
// the working tree.
type Report struct {
	Format           string           `json:"format"`
	PublicationReady bool             `json:"publicationReady"`
	CLI              CLIIdentity      `json:"cli"`
	SameNameDeadlock DeadlockEvidence `json:"sameNameDeadlock"`
	PlanRefusal      RefusalEvidence  `json:"planRefusal"`
	RollForward      SequenceEvidence `json:"rollForward"`
	ModuleDeploy     SequenceEvidence `json:"moduleDeploy"`
	HostSupport      RefusalEvidence  `json:"hostSupportAtPlan"`
	Configurations   []string         `json:"validatedConfigurations"`
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
	// delete carries the revision fence a conforming client always sends, so the
	// refusal is the dependency rule and not a missing precondition.
	stored, err := client.GetResource(ctx, harnessSpace, clientFormRef3(bundleRef), workerName+"-bundle")
	if err != nil {
		return DeadlockEvidence{}, CLIIdentity{}, fmt.Errorf("read the applied bundle: %w", err)
	}
	deleteErr := client.DeleteResource(
		ctx, harnessSpace, clientFormRef3(bundleRef), workerName+"-bundle", stored.Metadata.Revision)
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
	if err := h.write("main.tf", moduleStack(h.Endpoint(), modulePath, true)); err != nil {
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
	before, err := h.output(ctx, "worker_app_url")
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
	after, err := h.output(ctx, "worker_app_url")
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
			res, err := client.GetResource(sampleCtx, harnessSpace, clientFormRef3(ref), workerName)
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
