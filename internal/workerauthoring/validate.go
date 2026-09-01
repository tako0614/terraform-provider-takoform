package workerauthoring

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/internal/providerdiagnostics"
)

// MatrixReport is the evidence both supported CLIs produced.
type MatrixReport struct {
	Format           string `json:"format"`
	PublicationReady bool   `json:"publicationReady"`
	// ProviderBinarySHA256 is set only when the matrix was driven with an
	// explicit already-built provider binary; it is the digest of the exact
	// bytes both CLIs exercised.
	ProviderBinarySHA256 string   `json:"providerBinarySha256,omitempty"`
	Reports              []Report `json:"reports"`
}

// MatrixFormat identifies the two-CLI evidence shape.
const MatrixFormat = "takoform.worker-authoring-conformance-matrix@v1"

// RunMatrix drives every scenario under both supported CLIs with a provider
// built from source.
func RunMatrix(ctx context.Context, repoRoot, openTofuPath, terraformPath string) (MatrixReport, error) {
	return RunMatrixWithProvider(ctx, repoRoot, openTofuPath, terraformPath, "")
}

// RunMatrixWithProvider drives every scenario under both supported CLIs with
// an explicit already-built provider binary; an empty path builds from source.
func RunMatrixWithProvider(ctx context.Context, repoRoot, openTofuPath, terraformPath, providerBinary string) (MatrixReport, error) {
	matrix := MatrixReport{Format: MatrixFormat}
	if providerBinary != "" {
		digest, err := fileSHA256(providerBinary)
		if err != nil {
			return MatrixReport{}, err
		}
		matrix.ProviderBinarySHA256 = digest
	}
	for _, cli := range []string{openTofuPath, terraformPath} {
		report, err := RunWithProvider(ctx, repoRoot, cli, providerBinary)
		if err != nil {
			return MatrixReport{}, err
		}
		matrix.Reports = append(matrix.Reports, report)
	}
	return matrix, nil
}

// ValidateMatrix holds the evidence to what the authoring surface promises,
// and to the two CLIs agreeing about it.
func ValidateMatrix(matrix MatrixReport) error {
	if matrix.Format != MatrixFormat || matrix.PublicationReady {
		return errors.New("worker authoring matrix identity drifted")
	}
	if len(matrix.Reports) != 2 {
		return fmt.Errorf("worker authoring matrix carries %d CLI reports, want 2", len(matrix.Reports))
	}
	products := map[string]bool{}
	for _, report := range matrix.Reports {
		if err := Validate(report); err != nil {
			return err
		}
		products[report.CLI.Product] = true
	}
	if !products["OpenTofu"] || !products["Terraform"] {
		return errors.New("worker authoring matrix must cover OpenTofu and Terraform")
	}
	first, second := matrix.Reports[0], matrix.Reports[1]
	if !reflect.DeepEqual(first.SameNameDeadlock, second.SameNameDeadlock) {
		return errors.New("OpenTofu and Terraform observed different host refusals")
	}
	if !reflect.DeepEqual(first.PlanRefusal, second.PlanRefusal) ||
		!reflect.DeepEqual(first.HostSupport, second.HostSupport) {
		return errors.New("OpenTofu and Terraform observed different plan refusals")
	}
	if !reflect.DeepEqual(first.RollForward.Mutations, second.RollForward.Mutations) ||
		!reflect.DeepEqual(first.ModuleDeploy.Mutations, second.ModuleDeploy.Mutations) {
		return errors.New("OpenTofu and Terraform drove different roll-forward sequences")
	}
	if !reflect.DeepEqual(first.Configurations, second.Configurations) {
		return errors.New("OpenTofu and Terraform validated different configurations")
	}
	if !reflect.DeepEqual(first.TwoOwners, second.TwoOwners) ||
		!reflect.DeepEqual(first.HeterogeneousVars, second.HeterogeneousVars) {
		return errors.New("OpenTofu and Terraform derived different owner names or vars types")
	}
	// The teardown is compared on what it built and what it left, not on the
	// exact interleaving. The graph fixes a partial order and both CLIs honor it
	// (Validate holds each one to it), but the two are free to walk independent
	// branches in different orders, so requiring one sequence would measure a
	// scheduler rather than the contract.
	if !reflect.DeepEqual(first.ModuleDestroy.Built, second.ModuleDestroy.Built) ||
		!reflect.DeepEqual(first.ModuleDestroy.LeftBehind, second.ModuleDestroy.LeftBehind) ||
		!reflect.DeepEqual(deletedKinds(first.ModuleDestroy.Mutations), deletedKinds(second.ModuleDestroy.Mutations)) {
		return errors.New("OpenTofu and Terraform tore the worker-app module down differently")
	}
	return nil
}

// teardownAggregate is the Worker aggregate the module builds.
var teardownAggregate = []string{
	"WorkerEndpoint", "WorkerDeployment", "WorkerVersion", "WorkerBundle", "ModuleWorker",
}

// teardownEdges is what a destroy is held to: a PARTIAL order, one entry per
// edge the host itself refuses to have reversed.
//
// Every pair here is a `dependency_in_use` (409) or a
// `dependency_in_use`-shaped deployment refusal if the destroy takes them the
// other way round (spec/decisions/0015, 0016), so this is the contract's
// ordering and not a preference about scheduling. What is deliberately NOT here
// is the `WorkerBundle`/`ModuleWorker` pair: once the version is gone neither
// holds the other, so the two are independent leaves and either CLI may take
// them in either order. Asserting a total order would measure a graph walker.
var teardownEdges = [][2]string{
	{"WorkerEndpoint", "WorkerDeployment"},
	{"WorkerEndpoint", "ModuleWorker"},
	{"WorkerDeployment", "WorkerVersion"},
	{"WorkerDeployment", "ModuleWorker"},
	{"WorkerVersion", "WorkerBundle"},
	{"WorkerVersion", "ModuleWorker"},
}

// deletedKinds is the sorted multiset of kinds one timeline deleted.
func deletedKinds(mutations []mutation) []string {
	out := make([]string, 0, len(mutations))
	for _, entry := range mutations {
		if entry.Method == http.MethodDelete {
			out = append(out, entry.Kind)
		}
	}
	sort.Strings(out)
	return out
}

// assertTeardown holds one destroy to the three things it claims: it removed a
// live aggregate, it removed it in the order the host's refusals impose, and it
// left nothing.
func assertTeardown(evidence TeardownEvidence) error {
	built := append([]string(nil), evidence.Built...)
	sort.Strings(built)
	if len(built) != len(teardownAggregate) {
		return fmt.Errorf(
			"the destroy ran against %d resources (%v), want the five-resource worker aggregate", len(built), built)
	}
	if evidence.ReadySamples == 0 || evidence.NotReadySamples != 0 {
		return fmt.Errorf(
			"the worker was not observed serving before the teardown (%d ready, %d not)",
			evidence.ReadySamples, evidence.NotReadySamples)
	}
	position := map[string]int{}
	for index, entry := range evidence.Mutations {
		if entry.Method != http.MethodDelete {
			return fmt.Errorf(
				"the destroy drove a %s of %s; a teardown writes nothing", entry.Method, entry.Kind)
		}
		if entry.Status != http.StatusNoContent && entry.Status != http.StatusAccepted {
			return fmt.Errorf(
				"deleting %s %s answered HTTP %d; the delete fence is the desired generation, and a "+
					"teardown moves revisions it does not own",
				entry.Kind, entry.Name, entry.Status)
		}
		if _, seen := position[entry.Kind]; seen {
			return fmt.Errorf("the destroy deleted two %s resources", entry.Kind)
		}
		position[entry.Kind] = index
	}
	for _, kind := range teardownAggregate {
		if _, deleted := position[kind]; !deleted {
			return fmt.Errorf("the destroy never deleted the %s; it deleted %d resources", kind, len(position))
		}
	}
	for _, edge := range teardownEdges {
		if position[edge[0]] > position[edge[1]] {
			return fmt.Errorf(
				"the destroy removed the %s before the %s, which a conforming host refuses",
				edge[1], edge[0])
		}
	}
	if len(evidence.LeftBehind) != 0 {
		return fmt.Errorf(
			"the destroy left %v behind; a completed teardown holds nothing, and the store is read "+
				"whole rather than probed by name so an orphan cannot hide", evidence.LeftBehind)
	}
	return nil
}

// Validate holds one CLI's evidence to the claims this lane makes.
func Validate(report Report) error {
	if report.Format != ReportFormat || report.PublicationReady {
		return errors.New("worker authoring report identity drifted")
	}
	if report.CLI.Product == "" || report.CLI.Version == "" {
		return errors.New("worker authoring report names no CLI")
	}
	// The deadlock is the finding the derived names exist for, so it is asserted
	// by its exact codes rather than by "something failed".
	if report.SameNameDeadlock.DestroyFirstCode != "dependency_in_use" ||
		report.SameNameDeadlock.DestroyFirstHTTP != 409 {
		return fmt.Errorf("destroy-first refusal was %s (%d), want dependency_in_use (409)",
			report.SameNameDeadlock.DestroyFirstCode, report.SameNameDeadlock.DestroyFirstHTTP)
	}
	if report.SameNameDeadlock.CreateFirstCode != "invalid_argument" ||
		report.SameNameDeadlock.CreateFirstHTTP != 400 {
		return fmt.Errorf("create-first refusal was %s (%d), want invalid_argument (400)",
			report.SameNameDeadlock.CreateFirstCode, report.SameNameDeadlock.CreateFirstHTTP)
	}
	if report.PlanRefusal.Code != providerdiagnostics.ImmutableRevisionSameName {
		return errors.New("the same-name replacement was not refused at plan")
	}
	if report.HostSupport.Code != providerdiagnostics.HostDoesNotSupportValue {
		return errors.New("an unsupported Form was not refused at plan")
	}
	for name, sequence := range map[string]SequenceEvidence{
		"raw resources":     report.RollForward,
		"worker-app module": report.ModuleDeploy,
	} {
		if err := assertRollForwardOrder(sequence.Mutations); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if sequence.ReadySamples == 0 || sequence.NotReadySamples != 0 {
			return fmt.Errorf("%s: the worker was observed unserved during the roll-forward (%d of %d)",
				name, sequence.NotReadySamples, sequence.ReadySamples+sequence.NotReadySamples)
		}
	}
	if !report.ModuleDeploy.EndpointURLStable {
		return errors.New("the host-assigned endpoint address did not survive the code change")
	}
	if err := assertTeardown(report.ModuleDestroy); err != nil {
		return fmt.Errorf("worker-app module destroy: %w", err)
	}
	// Two owners of byte-identical build output are two owners. A digest names
	// the bytes; it is not an ownership claim, and a Terraform address has
	// exactly one owner.
	if err := assertDistinct("WorkerBundle", report.TwoOwners.BundleNames); err != nil {
		return err
	}
	if err := assertDistinct("WorkerVersion", report.TwoOwners.VersionNames); err != nil {
		return err
	}
	if len(report.TwoOwners.BundleNames) != 2 || len(report.TwoOwners.VersionNames) != 2 {
		return errors.New("the two-owner scenario did not measure two owners")
	}
	if !report.TwoOwners.HeldOwnerUnmoved {
		return errors.New("moving one owner disturbed the other owner's revisions")
	}
	for name, wanted := range map[string]string{"enabled": "boolean", "retries": "number", "label": "string"} {
		if got := report.HeterogeneousVars.WireTypes[name]; got != wanted {
			return fmt.Errorf("the module sent vars.%s to the host as a JSON %s, want %s", name, got, wanted)
		}
	}
	if report.ShortestModuleName != shortestPortableName {
		return fmt.Errorf("the repository module did not accept the shortest portable name %q", shortestPortableName)
	}
	if len(report.Configurations) == 0 {
		return errors.New("no configuration was validated")
	}
	return nil
}
