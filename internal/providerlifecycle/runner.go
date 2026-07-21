package providerlifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/tako0614/terraform-provider-takoform/internal/characterization"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
)

const (
	ReportFormat          = "takoform.provider-lifecycle-candidate@v3"
	MatrixReportFormat    = "takoform.provider-cli-fqn-matrix@v2"
	RunnerSubject         = "takoform.provider-binary-cli-runner@v1"
	LocalDevOverride      = "local-dev-override"
	DirectRegistryInstall = "direct-registry-install"
	providerProtocol      = "Terraform provider protocol v6 + versioned Form host HTTP"

	OpenTofuProviderAddress  = "registry.opentofu.org/tako0614/takoform"
	TerraformProviderAddress = "registry.terraform.io/tako0614/takoform"
	// CanonicalProviderAddress is the provider handshake and Terraform Registry
	// identity. OpenTofuProviderAddress is a separately published alternative
	// state identity, never an alias for this address.
	CanonicalProviderAddress = TerraformProviderAddress
)

type CheckEvidence struct {
	Create       bool `json:"create"`
	Read         bool `json:"read"`
	Update       bool `json:"update"`
	Observe      bool `json:"observe"`
	Refresh      bool `json:"refresh"`
	NativeImport bool `json:"nativeImport"`
	CLIImport    bool `json:"cliImport"`
	Delete       bool `json:"delete"`
	DriftState   bool `json:"driftState"`
	NameReplace  bool `json:"nameReplace"`
}

type ResourceEvidence struct {
	Kind         string        `json:"kind"`
	ResourceType string        `json:"resourceType"`
	Checks       CheckEvidence `json:"checks"`
}

type NegativeEvidence struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Fixture string `json:"fixture"`
	Passed  bool   `json:"passed"`
}

type CLIIdentity struct {
	Product          string `json:"product"`
	Version          string `json:"version"`
	ProviderAddress  string `json:"providerAddress"`
	ExecutableName   string `json:"executableName"`
	ExecutableSHA256 string `json:"executableSha256"`
}

type ProviderBinaryIdentity struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type ImmutableReplaceEvidence struct {
	Kind   string `json:"kind"`
	Field  string `json:"field"`
	Passed bool   `json:"passed"`
}

type Report struct {
	Format               string                     `json:"format"`
	Classification       string                     `json:"classification"`
	PublicationReady     bool                       `json:"publicationReady"`
	BindingStatus        string                     `json:"bindingStatus"`
	RunnerSubject        string                     `json:"runnerSubject"`
	Protocol             string                     `json:"protocol"`
	InstallationSource   string                     `json:"installationSource"`
	CandidateSetSHA256   string                     `json:"candidateSetSha256"`
	ProviderSchemaSHA256 string                     `json:"providerSchemaSha256"`
	ProviderBinary       ProviderBinaryIdentity     `json:"providerBinary"`
	CLI                  CLIIdentity                `json:"cli"`
	Resources            []ResourceEvidence         `json:"resources"`
	NegativeChecks       []NegativeEvidence         `json:"negativeChecks"`
	ImmutableReplace     []ImmutableReplaceEvidence `json:"immutableReplace"`
}

type CLIRequirement struct {
	Product         string `json:"product"`
	Version         string `json:"version"`
	ProviderAddress string `json:"providerAddress"`
}

type MatrixReport struct {
	Format                  string   `json:"format"`
	Classification          string   `json:"classification"`
	PublicationReady        bool     `json:"publicationReady"`
	ReleaseDescriptorSHA256 string   `json:"releaseDescriptorSha256"`
	CandidateSetSHA256      string   `json:"candidateSetSha256"`
	ProviderSchemaSHA256    string   `json:"providerSchemaSha256"`
	InstallationSource      string   `json:"installationSource"`
	Reports                 []Report `json:"reports"`
}

type resourceCase struct {
	Kind         string
	ResourceType string
	Address      string
	Name         string
}

var resourceCases = []resourceCase{
	{client.KindEdgeWorker, "takoform_edge_worker", "takoform_edge_worker.api", "api"},
	{client.KindObjectBucket, "takoform_object_bucket", "takoform_object_bucket.assets", "assets"},
	{client.KindKVStore, "takoform_kv_store", "takoform_kv_store.cache", "cache"},
	{client.KindQueue, "takoform_queue", "takoform_queue.delivery", "delivery"},
	{client.KindSQLDatabase, "takoform_sql_database", "takoform_sql_database.main", "main"},
	{client.KindContainerService, "takoform_container_service", "takoform_container_service.agent", "agent"},
	{client.KindVectorIndex, "takoform_vector_index", "takoform_vector_index.embeddings", "embeddings"},
	{client.KindDurableWorkflow, "takoform_durable_workflow", "takoform_durable_workflow.ingest", "ingest"},
	{client.KindStatefulActorNamespace, "takoform_stateful_actor_namespace", "takoform_stateful_actor_namespace.rooms", "rooms"},
	{client.KindSchedule, "takoform_schedule", "takoform_schedule.nightly_ingest", "nightly-ingest"},
}

func Run(ctx context.Context, repoRoot, cliPath string) (Report, error) {
	return run(ctx, repoRoot, cliPath, LocalDevOverride)
}

// RunRegistry executes the same lifecycle without dev_overrides. The exact
// version pinned by release/version.json must be installed by the CLI from its
// canonical Registry address before any lifecycle action runs.
func RunRegistry(ctx context.Context, repoRoot, cliPath string) (Report, error) {
	return run(ctx, repoRoot, cliPath, DirectRegistryInstall)
}

func run(ctx context.Context, repoRoot, cliPath, installationSource string) (Report, error) {
	cli, identity, err := identifyCLI(ctx, cliPath)
	if err != nil {
		return Report{}, err
	}
	temp, err := os.MkdirTemp("", "takoform-provider-lifecycle-")
	if err != nil {
		return Report{}, err
	}
	defer os.RemoveAll(temp)

	host := newFormHost()
	server := httptest.NewServer(host)
	defer server.Close()

	binDir := filepath.Join(temp, "bin")
	workDir := filepath.Join(temp, "stack")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return Report{}, err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return Report{}, err
	}
	providerVersion, err := loadProviderVersion(repoRoot)
	if err != nil {
		return Report{}, err
	}
	cliConfig := filepath.Join(temp, "terraformrc")
	cliConfigBody := `provider_installation {
	  direct {}
}
`
	providerBinary := ""
	if installationSource == LocalDevOverride {
		providerBinary = filepath.Join(binDir, "terraform-provider-takoform")
		if output, err := runCommand(ctx, repoRoot, nil, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-buildid= -X main.version="+providerVersion, "-o", providerBinary, "."); err != nil {
			return Report{}, fmt.Errorf("build provider binary: %w\n%s", err, output)
		}
		cliConfigBody = fmt.Sprintf(`provider_installation {
  dev_overrides {
    %q = %q
  }
  direct {}
}
`, identity.ProviderAddress, binDir)
	} else if installationSource != DirectRegistryInstall {
		return Report{}, fmt.Errorf("unsupported provider installation source %q", installationSource)
	}
	if err := os.WriteFile(cliConfig, []byte(cliConfigBody), 0o600); err != nil {
		return Report{}, err
	}
	env := terraformRunnerEnvironment(cliConfig)
	configPath := filepath.Join(workDir, "main.tf")
	if err := os.WriteFile(configPath, []byte(stackConfig(server.URL, identity.ProviderAddress, providerVersion, 1)), 0o600); err != nil {
		return Report{}, err
	}
	terraformRun := func(args ...string) (string, error) {
		base := []string{"-chdir=" + workDir}
		base = append(base, args...)
		return runCommand(ctx, repoRoot, env, cli, base...)
	}
	if installationSource == DirectRegistryInstall {
		if output, err := terraformRun("init", "-input=false", "-no-color"); err != nil {
			return Report{}, fmt.Errorf("%s direct Registry install of %s v%s: %w\n%s", identity.Product, identity.ProviderAddress, providerVersion, err, output)
		}
		providerBinary, err = findInstalledProviderBinary(workDir, providerVersion)
		if err != nil {
			return Report{}, err
		}
	}
	reportedVersion, err := runCommand(ctx, repoRoot, nil, providerBinary, "-version")
	if err != nil || strings.TrimSpace(reportedVersion) != providerVersion {
		return Report{}, fmt.Errorf("provider binary reported version %q, want %q: %w", strings.TrimSpace(reportedVersion), providerVersion, err)
	}
	providerBinarySHA256, err := fileSHA256(providerBinary)
	if err != nil {
		return Report{}, err
	}
	providerSchemaSHA256, err := captureProviderSchema(terraformRun, identity.ProviderAddress)
	if err != nil {
		return Report{}, fmt.Errorf("%s provider schema/FQN proof: %w", identity.Product, err)
	}
	if output, err := terraformRun("apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return Report{}, fmt.Errorf("%s create apply: %w\n%s", identity.Product, err, output)
	}
	if output, err := terraformRun("plan", "-refresh-only", "-input=false", "-no-color", "-detailed-exitcode"); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
			return Report{}, fmt.Errorf("%s read/observe plan: %w\n%s", identity.Product, err, output)
		}
	}

	host.setDrift(true)
	if output, err := terraformRun("apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return Report{}, fmt.Errorf("%s drift refresh: %w\n%s", identity.Product, err, output)
	}
	show, err := terraformRun("show", "-json")
	if err != nil {
		return Report{}, fmt.Errorf("%s state read: %w\n%s", identity.Product, err, show)
	}
	if err := verifyDriftState([]byte(show), identity.ProviderAddress); err != nil {
		return Report{}, err
	}
	host.markDriftStateVerified()
	host.setDrift(false)

	host.setSubstitution(client.KindObjectBucket, "name")
	negativeOutput, negativeErr := terraformRun("plan", "-refresh-only", "-input=false", "-no-color")
	host.setSubstitution("", "")
	if negativeErr == nil || !strings.Contains(negativeOutput, "changed the requested Resource name or space") {
		return Report{}, fmt.Errorf("identity-substitution negative fixture was not rejected\n%s", negativeOutput)
	}
	host.setSubstitution(client.KindKVStore, "packageDigest")
	negativeFormOutput, negativeFormErr := terraformRun("plan", "-refresh-only", "-input=false", "-no-color")
	host.setSubstitution("", "")
	if negativeFormErr == nil || !strings.Contains(negativeFormOutput, "changed the exact FormRef/package identity") {
		return Report{}, fmt.Errorf("exact FormRef/package substitution negative fixture was not rejected\n%s", negativeFormOutput)
	}

	if err := os.WriteFile(configPath, []byte(stackConfig(server.URL, identity.ProviderAddress, providerVersion, 2)), 0o600); err != nil {
		return Report{}, err
	}
	if output, err := terraformRun("apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return Report{}, fmt.Errorf("%s update apply: %w\n%s", identity.Product, err, output)
	}
	immutableEvidence, err := exerciseReplacementPlans(workDir, configPath, server.URL, identity.ProviderAddress, providerVersion, terraformRun)
	if err != nil {
		return Report{}, err
	}

	if err := exerciseExplicitHostActions(ctx, server.URL, server.Client()); err != nil {
		return Report{}, err
	}

	addresses := make([]string, 0, len(resourceCases))
	for _, item := range resourceCases {
		addresses = append(addresses, item.Address)
	}
	stateRM := append([]string{"state", "rm", "-no-color"}, addresses...)
	if output, err := terraformRun(stateRM...); err != nil {
		return Report{}, fmt.Errorf("%s state rm before canonical import: %w\n%s", identity.Product, err, output)
	}
	host.setCLIImport(true)
	for _, item := range resourceCases {
		if output, err := terraformRun("import", "-input=false", "-no-color", item.Address, "prod/"+item.Name); err != nil {
			return Report{}, fmt.Errorf("%s import %s: %w\n%s", identity.Product, item.Kind, err, output)
		}
	}
	host.setCLIImport(false)
	if output, err := terraformRun("destroy", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return Report{}, fmt.Errorf("%s destroy: %w\n%s", identity.Product, err, output)
	}

	return host.report(identity, ProviderBinaryIdentity{Version: providerVersion, SHA256: providerBinarySHA256}, providerSchemaSHA256, installationSource, immutableEvidence), nil
}

func Validate(report Report) error {
	return validateReport(report, LocalDevOverride)
}

// ValidateRegistry verifies a report that was produced only after the CLI
// performed a direct Registry install of the pinned provider version.
func ValidateRegistry(report Report) error {
	return validateReport(report, DirectRegistryInstall)
}

func validateReport(report Report, installationSource string) error {
	if report.Format != ReportFormat || report.Classification != "generic-lifecycle-candidate" || report.PublicationReady ||
		report.BindingStatus != "exact-structural-candidate-set" || report.RunnerSubject != RunnerSubject || len(report.Resources) != len(resourceCases) ||
		report.Protocol != providerProtocol || report.InstallationSource != installationSource ||
		report.CandidateSetSHA256 != candidateSetSHA256() || !validDigest(report.ProviderSchemaSHA256) ||
		report.ProviderBinary.Version == "" || !validDigest(report.ProviderBinary.SHA256) ||
		report.CLI.Product == "" || report.CLI.Version == "" || report.CLI.ExecutableName == "" || !validDigest(report.CLI.ExecutableSHA256) {
		return errors.New("provider lifecycle candidate report identity is invalid")
	}
	expectedAddress, err := providerAddressForProduct(report.CLI.Product)
	if err != nil || report.CLI.ProviderAddress != expectedAddress {
		return errors.New("provider lifecycle candidate used an unsupported CLI/FQN identity")
	}
	expectedResources := make(map[string]string, len(resourceCases))
	for _, item := range resourceCases {
		expectedResources[item.Kind] = item.ResourceType
	}
	seenResources := make(map[string]struct{}, len(report.Resources))
	for _, resource := range report.Resources {
		expectedType, known := expectedResources[resource.Kind]
		if !known || resource.ResourceType != expectedType {
			return fmt.Errorf("provider lifecycle candidate contains unexpected resource identity %s/%s", resource.Kind, resource.ResourceType)
		}
		if _, duplicate := seenResources[resource.Kind]; duplicate {
			return fmt.Errorf("provider lifecycle candidate duplicates resource %s", resource.Kind)
		}
		seenResources[resource.Kind] = struct{}{}
		checks := resource.Checks
		if !checks.Create || !checks.Read || !checks.Update || !checks.Observe || !checks.Refresh || !checks.NativeImport ||
			!checks.CLIImport || !checks.Delete || !checks.DriftState || !checks.NameReplace {
			return fmt.Errorf("provider lifecycle candidate is incomplete for %s", resource.Kind)
		}
	}
	expectedNegative := map[string]string{
		"response-name-substitution-rejected":           client.KindObjectBucket,
		"response-package-digest-substitution-rejected": client.KindKVStore,
	}
	if len(report.NegativeChecks) != len(expectedNegative) {
		return errors.New("provider lifecycle negative fixture is incomplete")
	}
	seenNegative := make(map[string]struct{}, len(report.NegativeChecks))
	for _, evidence := range report.NegativeChecks {
		expectedKind, known := expectedNegative[evidence.Name]
		if !known || evidence.Kind != expectedKind || strings.TrimSpace(evidence.Fixture) == "" || !evidence.Passed {
			return errors.New("provider lifecycle negative fixture is incomplete")
		}
		if _, duplicate := seenNegative[evidence.Name]; duplicate {
			return fmt.Errorf("provider lifecycle duplicates negative fixture %q", evidence.Name)
		}
		seenNegative[evidence.Name] = struct{}{}
	}
	if len(report.ImmutableReplace) != len(resourceCases)+2 {
		return errors.New("provider lifecycle immutable replacement evidence is incomplete")
	}
	expectedImmutable := make(map[string]struct{}, len(resourceCases)+2)
	for _, item := range resourceCases {
		expectedImmutable[item.Kind+"/name"] = struct{}{}
	}
	expectedImmutable[client.KindSQLDatabase+"/engine"] = struct{}{}
	expectedImmutable[client.KindVectorIndex+"/dimensions"] = struct{}{}
	seenImmutable := make(map[string]struct{}, len(report.ImmutableReplace))
	for _, evidence := range report.ImmutableReplace {
		key := evidence.Kind + evidence.Field
		if _, known := expectedImmutable[key]; !known || !evidence.Passed {
			return fmt.Errorf("provider lifecycle immutable replacement failed for %s%s", evidence.Kind, evidence.Field)
		}
		if _, duplicate := seenImmutable[key]; duplicate {
			return fmt.Errorf("provider lifecycle duplicates immutable replacement %s", key)
		}
		seenImmutable[key] = struct{}{}
	}
	return nil
}

func LoadCLIMatrix(repoRoot string) ([]CLIRequirement, string, error) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "release", "version.json"))
	if err != nil {
		return nil, "", err
	}
	var descriptor struct {
		CLIMatrix []CLIRequirement `json:"cliMatrix"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return nil, "", err
	}
	if len(descriptor.CLIMatrix) != 2 {
		return nil, "", errors.New("release descriptor must pin exactly OpenTofu and Terraform CLI/FQN entries")
	}
	seen := map[string]bool{}
	for _, requirement := range descriptor.CLIMatrix {
		expectedAddress, err := providerAddressForProduct(requirement.Product)
		if err != nil || requirement.Version == "" || requirement.ProviderAddress != expectedAddress || seen[requirement.Product] {
			return nil, "", fmt.Errorf("invalid release CLI/FQN matrix entry for %q", requirement.Product)
		}
		seen[requirement.Product] = true
	}
	if !seen["OpenTofu"] || !seen["Terraform"] {
		return nil, "", errors.New("release CLI/FQN matrix must contain OpenTofu and Terraform")
	}
	sum := sha256.Sum256(raw)
	return descriptor.CLIMatrix, fmt.Sprintf("sha256:%x", sum), nil
}

func loadProviderVersion(repoRoot string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "release", "version.json"))
	if err != nil {
		return "", err
	}
	var descriptor struct {
		Version string `json:"version"`
		Tag     string `json:"tag"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return "", err
	}
	if descriptor.Version == "" || descriptor.Tag != "v"+descriptor.Version {
		return "", errors.New("release descriptor provider version is invalid")
	}
	return descriptor.Version, nil
}

func RunMatrix(ctx context.Context, repoRoot, openTofuPath, terraformPath string) (MatrixReport, error) {
	return runMatrix(ctx, repoRoot, openTofuPath, terraformPath, LocalDevOverride)
}

// RunRegistryMatrix runs the reviewed CLI/FQN matrix using direct Registry
// installation rather than a locally built dev override.
func RunRegistryMatrix(ctx context.Context, repoRoot, openTofuPath, terraformPath string) (MatrixReport, error) {
	return runMatrix(ctx, repoRoot, openTofuPath, terraformPath, DirectRegistryInstall)
}

func runMatrix(ctx context.Context, repoRoot, openTofuPath, terraformPath, installationSource string) (MatrixReport, error) {
	requirements, descriptorSHA256, err := LoadCLIMatrix(repoRoot)
	if err != nil {
		return MatrixReport{}, err
	}
	paths := map[string]string{"OpenTofu": openTofuPath, "Terraform": terraformPath}
	reports := make([]Report, 0, len(requirements))
	for _, requirement := range requirements {
		var report Report
		if installationSource == DirectRegistryInstall {
			report, err = RunRegistry(ctx, repoRoot, paths[requirement.Product])
		} else {
			report, err = Run(ctx, repoRoot, paths[requirement.Product])
		}
		if err != nil {
			return MatrixReport{}, err
		}
		reports = append(reports, report)
	}
	matrix := MatrixReport{
		Format: MatrixReportFormat, Classification: "supported-cli-fqn-candidate-matrix", PublicationReady: false,
		ReleaseDescriptorSHA256: descriptorSHA256, CandidateSetSHA256: candidateSetSHA256(), InstallationSource: installationSource, Reports: reports,
	}
	if len(reports) > 0 {
		matrix.ProviderSchemaSHA256 = reports[0].ProviderSchemaSHA256
	}
	if err := validateMatrix(matrix, requirements, installationSource); err != nil {
		return MatrixReport{}, err
	}
	return matrix, nil
}

func ValidateMatrix(matrix MatrixReport, requirements []CLIRequirement) error {
	return validateMatrix(matrix, requirements, LocalDevOverride)
}

// ValidateRegistryMatrix requires the direct Registry installation marker on
// the matrix and every nested lifecycle report.
func ValidateRegistryMatrix(matrix MatrixReport, requirements []CLIRequirement) error {
	return validateMatrix(matrix, requirements, DirectRegistryInstall)
}

func validateMatrix(matrix MatrixReport, requirements []CLIRequirement, installationSource string) error {
	if matrix.Format != MatrixReportFormat || matrix.Classification != "supported-cli-fqn-candidate-matrix" || matrix.PublicationReady ||
		matrix.InstallationSource != installationSource ||
		!validDigest(matrix.ReleaseDescriptorSHA256) || matrix.CandidateSetSHA256 != candidateSetSHA256() ||
		!validDigest(matrix.ProviderSchemaSHA256) || len(matrix.Reports) != len(requirements) || len(requirements) != 2 {
		return errors.New("provider CLI/FQN matrix identity is invalid")
	}
	requirementByProduct := map[string]CLIRequirement{}
	for _, requirement := range requirements {
		requirementByProduct[requirement.Product] = requirement
	}
	var baseline *Report
	seen := map[string]bool{}
	for index := range matrix.Reports {
		report := matrix.Reports[index]
		if err := validateReport(report, installationSource); err != nil {
			return err
		}
		requirement, ok := requirementByProduct[report.CLI.Product]
		if !ok || seen[report.CLI.Product] || report.CLI.Version != requirement.Version || report.CLI.ProviderAddress != requirement.ProviderAddress {
			return fmt.Errorf("provider CLI/FQN matrix does not match the reviewed %s entry", report.CLI.Product)
		}
		seen[report.CLI.Product] = true
		if report.ProviderSchemaSHA256 != matrix.ProviderSchemaSHA256 || report.CandidateSetSHA256 != matrix.CandidateSetSHA256 {
			return errors.New("provider CLI/FQN matrix exposed different schema or Form candidate identities")
		}
		if baseline == nil {
			baseline = &matrix.Reports[index]
			continue
		}
		if !reflect.DeepEqual(report.Resources, baseline.Resources) ||
			!reflect.DeepEqual(report.NegativeChecks, baseline.NegativeChecks) ||
			!reflect.DeepEqual(report.ImmutableReplace, baseline.ImmutableReplace) ||
			report.ProviderBinary != baseline.ProviderBinary {
			return errors.New("OpenTofu and Terraform lifecycle evidence differs")
		}
	}
	if !seen["OpenTofu"] || !seen["Terraform"] ||
		requirementByProduct["OpenTofu"].ProviderAddress == requirementByProduct["Terraform"].ProviderAddress {
		return errors.New("provider CLI/FQN matrix must preserve the canonical Terraform identity and distinct dual-published OpenTofu identity")
	}
	return nil
}

func exerciseExplicitHostActions(ctx context.Context, endpoint string, httpClient *http.Client) error {
	formClient := client.New(endpoint, "", httpClient)
	if _, err := formClient.Discover(ctx); err != nil {
		return err
	}
	forms := candidateForms()
	for _, item := range resourceCases {
		form := forms[item.Kind]
		current, err := formClient.GetResource(ctx, item.Kind, item.Name, "prod", form)
		if err != nil {
			return fmt.Errorf("get %s before explicit actions: %w", item.Kind, err)
		}
		fence := client.MutationFence{ResourceVersion: current.Metadata.ResourceVersion, Form: form}
		refreshed, err := formClient.RefreshResource(ctx, item.Kind, item.Name, "prod", fence)
		if err != nil {
			return fmt.Errorf("refresh %s: %w", item.Kind, err)
		}
		desired := &client.Resource{
			APIVersion: client.APIVersion, Kind: item.Kind, Form: &form,
			Metadata: client.Metadata{Name: item.Name, Space: "prod", ResourceVersion: refreshed.Metadata.ResourceVersion},
			Spec:     refreshed.Spec,
		}
		if _, err := formClient.ImportResource(ctx, item.Kind, item.Name, "native-"+item.Name, desired); err != nil {
			return fmt.Errorf("native import %s: %w", item.Kind, err)
		}
	}
	return nil
}

func captureProviderSchema(run terraformRunFunc, providerAddress string) (string, error) {
	raw, err := run("providers", "schema", "-json")
	if err != nil {
		return "", fmt.Errorf("read provider schema: %w\n%s", err, raw)
	}
	var document struct {
		ProviderSchemas map[string]json.RawMessage `json:"provider_schemas"`
	}
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return "", err
	}
	schema, ok := document.ProviderSchemas[providerAddress]
	if !ok || len(document.ProviderSchemas) != 1 {
		return "", fmt.Errorf("schema omitted the exact provider FQN %s", providerAddress)
	}
	var normalized any
	if err := json.Unmarshal(schema, &normalized); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func verifyDriftState(raw []byte, providerAddress string) error {
	var state struct {
		Values struct {
			RootModule struct {
				Resources []struct {
					Address      string         `json:"address"`
					ProviderName string         `json:"provider_name"`
					Values       map[string]any `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
		} `json:"values"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, resource := range state.Values.RootModule.Resources {
		if resource.ProviderName != providerAddress {
			return fmt.Errorf("state provider FQN for %s = %q, want %q", resource.Address, resource.ProviderName, providerAddress)
		}
		if resource.Values["drift_status"] == "drifted" {
			seen[resource.Address] = true
		}
	}
	for _, item := range resourceCases {
		if !seen[item.Address] {
			return fmt.Errorf("Terraform-compatible state did not map drifted observation for %s", item.Address)
		}
	}
	return nil
}

func candidateForms() map[string]client.InstalledFormReference {
	out := map[string]client.InstalledFormReference{}
	for kind, ref := range formregistry.All() {
		out[kind] = client.InstalledFormReference{
			FormRef:       client.FormRef{APIVersion: ref.APIVersion, Kind: ref.Kind, DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest},
			PackageDigest: ref.PackageDigest,
		}
	}
	return out
}

func candidateSetSHA256() string {
	forms := candidateForms()
	kinds := make([]string, 0, len(forms))
	for kind := range forms {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	ordered := make([]client.InstalledFormReference, 0, len(kinds))
	for _, kind := range kinds {
		ordered = append(ordered, forms[kind])
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum)
}

// CandidateSetSHA256 is the deterministic identity shared by local lifecycle
// reports and authenticated Registry install/readback reports.
func CandidateSetSHA256() string {
	return candidateSetSHA256()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hasher.Sum(nil)), nil
}

func findInstalledProviderBinary(workDir, version string) (string, error) {
	providerRoot := filepath.Join(workDir, ".terraform", "providers")
	matches := []string{}
	err := filepath.WalkDir(providerRoot, func(filename string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "terraform-provider-takoform") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			matches = append(matches, filename)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspect directly installed provider: %w", err)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("direct Registry install produced %d provider binaries for v%s, want exactly one", len(matches), version)
	}
	return matches[0], nil
}

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	if env != nil {
		command.Env = env
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func identifyCLI(ctx context.Context, requested string) (string, CLIIdentity, error) {
	if requested == "" {
		requested = "tofu"
	}
	executable, err := exec.LookPath(requested)
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("provider lifecycle conformance requires a Terraform-compatible CLI (%s): %w", requested, err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", CLIIdentity{}, err
	}
	identityEnv := sanitizedTerraformBaseEnvironment()
	versionOutput, err := runCommand(ctx, ".", identityEnv, executable, "version", "-json")
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("inspect Terraform-compatible CLI version: %w\n%s", err, versionOutput)
	}
	var version struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal([]byte(versionOutput), &version); err != nil {
		return "", CLIIdentity{}, fmt.Errorf("inspect Terraform-compatible CLI version JSON: %w", err)
	}
	if version.TerraformVersion == "" {
		return "", CLIIdentity{}, errors.New("inspect Terraform-compatible CLI version JSON: terraform_version is empty")
	}
	plainOutput, err := runCommand(ctx, ".", identityEnv, executable, "version")
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("inspect Terraform-compatible CLI product: %w\n%s", err, plainOutput)
	}
	product := ""
	if strings.HasPrefix(plainOutput, "OpenTofu ") {
		product = "OpenTofu"
	} else if strings.HasPrefix(plainOutput, "Terraform ") {
		product = "Terraform"
	}
	providerAddress, err := providerAddressForProduct(product)
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("inspect Terraform-compatible CLI product: %w", err)
	}
	binary, err := os.Open(executable)
	if err != nil {
		return "", CLIIdentity{}, err
	}
	defer binary.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, binary); err != nil {
		return "", CLIIdentity{}, err
	}
	return executable, CLIIdentity{
		Product: product, Version: version.TerraformVersion, ProviderAddress: providerAddress, ExecutableName: filepath.Base(executable),
		ExecutableSHA256: fmt.Sprintf("sha256:%x", hasher.Sum(nil)),
	}, nil
}

func providerAddressForProduct(product string) (string, error) {
	switch product {
	case "OpenTofu":
		return OpenTofuProviderAddress, nil
	case "Terraform":
		return TerraformProviderAddress, nil
	default:
		return "", fmt.Errorf("unsupported Terraform-compatible CLI product %q", product)
	}
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

type terraformRunFunc func(args ...string) (string, error)

func exerciseReplacementPlans(workDir, configPath, endpoint, providerAddress, providerVersion string, run terraformRunFunc) ([]ImmutableReplaceEvidence, error) {
	defer func() {
		_ = os.WriteFile(configPath, []byte(stackConfig(endpoint, providerAddress, providerVersion, 2)), 0o600)
	}()
	verifyPlan := func(revision int, expected map[string]bool) error {
		if err := os.WriteFile(configPath, []byte(stackConfig(endpoint, providerAddress, providerVersion, revision)), 0o600); err != nil {
			return err
		}
		planPath := filepath.Join(workDir, fmt.Sprintf("replace-%d.tfplan", revision))
		if output, err := run("plan", "-refresh=false", "-input=false", "-no-color", "-out="+planPath); err != nil {
			return fmt.Errorf("replacement plan %d: %w\n%s", revision, err, output)
		}
		raw, err := run("show", "-json", planPath)
		if err != nil {
			return fmt.Errorf("replacement plan %d JSON: %w\n%s", revision, err, raw)
		}
		var plan struct {
			ResourceChanges []struct {
				Address string `json:"address"`
				Change  struct {
					Actions []string `json:"actions"`
				} `json:"change"`
			} `json:"resource_changes"`
		}
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, change := range plan.ResourceChanges {
			replaced := len(change.Change.Actions) == 2 && change.Change.Actions[0] == "delete" && change.Change.Actions[1] == "create"
			if expected[change.Address] != replaced {
				return fmt.Errorf("replacement plan %d actions for %s = %v", revision, change.Address, change.Change.Actions)
			}
			seen[change.Address] = true
		}
		for address := range expected {
			if !seen[address] {
				return fmt.Errorf("replacement plan %d omitted %s", revision, address)
			}
		}
		return nil
	}
	fieldExpected := map[string]bool{}
	nameExpected := map[string]bool{}
	for _, item := range resourceCases {
		fieldExpected[item.Address] = item.Kind == client.KindSQLDatabase || item.Kind == client.KindVectorIndex
		nameExpected[item.Address] = true
	}
	if err := verifyPlan(3, fieldExpected); err != nil {
		return nil, err
	}
	if err := verifyPlan(4, nameExpected); err != nil {
		return nil, err
	}
	evidence := make([]ImmutableReplaceEvidence, 0, len(resourceCases)+2)
	for _, item := range resourceCases {
		evidence = append(evidence, ImmutableReplaceEvidence{Kind: item.Kind, Field: "/name", Passed: true})
	}
	evidence = append(evidence,
		ImmutableReplaceEvidence{Kind: client.KindSQLDatabase, Field: "/engine", Passed: true},
		ImmutableReplaceEvidence{Kind: client.KindVectorIndex, Field: "/dimensions", Passed: true},
	)
	return evidence, nil
}

func stackConfig(endpoint, providerAddress, providerVersion string, revision int) string {
	artifactDigit, artifactRevision, storageClass, consistency, engine := "1", 1, "standard", "strong", "sqlite"
	queueRetries, queueBatch, port, dimensions := 5, 25, 8080, 1536
	imageDigest, migrationsPath, metric := strings.Repeat("c", 64), "migrations/v1", "cosine"
	workflowEntrypoint, actorTag, cron, nameSuffix := "IngestWorkflow", "v1", "0 0 * * *", ""
	if revision >= 2 {
		artifactDigit, artifactRevision, storageClass, consistency = "3", 2, "infrequent_access", "eventual"
		queueRetries, queueBatch, port = 6, 30, 9090
		imageDigest, migrationsPath, metric = strings.Repeat("d", 64), "migrations/v2", "dot"
		workflowEntrypoint, actorTag, cron = "IngestWorkflowV2", "v2", "15 0 * * *"
	}
	if revision == 3 {
		engine, dimensions = "postgres", 1024
	}
	if revision == 4 {
		nameSuffix = "-replacement"
	}
	digest := strings.Repeat(artifactDigit, 64)
	return fmt.Sprintf(`terraform {
  required_providers {
    takoform = { source = %q, version = %q }
  }
}
provider "takoform" {
  endpoint = %q
  space = "prod"
}
resource "takoform_edge_worker" "api" {
  name = "api%s"
  artifact_url = "https://example.test/api-v%d.js"
  artifact_sha256 = "sha256:%s"
}
resource "takoform_object_bucket" "assets" {
  name = "assets%s"
  storage_class = %q
  interfaces = ["s3_api"]
}
resource "takoform_kv_store" "cache" {
  name = "cache%s"
  consistency = %q
}
resource "takoform_queue" "delivery" {
  name = "delivery%s"
  max_retries = %d
  max_batch_size = %d
}
resource "takoform_sql_database" "main" {
  name = "main%s"
  engine = %q
  migrations_path = %q
}
resource "takoform_container_service" "agent" {
  name = "agent%s"
  image = "ghcr.io/example/agent@sha256:%s"
  ports = [%d]
  public_http = true
}
resource "takoform_vector_index" "embeddings" {
  name = "embeddings%s"
  dimensions = %d
  metric = %q
}
resource "takoform_durable_workflow" "ingest" {
  name = "ingest%s"
  artifact_url = "https://example.test/workflow-v%d.js"
  artifact_sha256 = "sha256:%s"
  entrypoint = %q
}
resource "takoform_stateful_actor_namespace" "rooms" {
  name = "rooms%s"
  class_name = "RoomActor"
  storage_profile = "durable_sqlite"
  migration_tag = %q
}
resource "takoform_schedule" "nightly_ingest" {
  name = "nightly-ingest%s"
  cron = %q
  timezone = "UTC"
  connections = [{ name = "workflow", resource = "DurableWorkflow/ingest", permissions = ["invoke"], projection = "schedule_trigger" }]
}
`, providerAddress, providerVersion, endpoint,
		nameSuffix, artifactRevision, digest,
		nameSuffix, storageClass,
		nameSuffix, consistency,
		nameSuffix, queueRetries, queueBatch,
		nameSuffix, engine, migrationsPath,
		nameSuffix, imageDigest, port,
		nameSuffix, dimensions, metric,
		nameSuffix, artifactRevision, digest, workflowEntrypoint,
		nameSuffix, actorTag,
		nameSuffix, cron,
	)
}

type lifecycleCounts struct {
	create, read, update, observe, refresh, nativeImport, cliImport, delete, driftState int
}

// The declaration the in-process conformance host reports. It matches the
// portable-host contract's optional interface surface.
const (
	conformanceInterfaceName         = "s3.api"
	conformanceInterfaceVersion      = "2025-11-25"
	conformanceInterfaceKind         = "ObjectBucket"
	conformanceInterfaceResourceName = "assets"
)

type formHost struct {
	mu                sync.Mutex
	forms             map[string]client.InstalledFormReference
	resources         map[string]client.Resource
	counts            map[string]*lifecycleCounts
	drift             bool
	substitutionKind  string
	substitutionField string
	cliImportPhase    bool
}

func newFormHost() *formHost {
	return newFormHostWithForms(candidateForms())
}

func newFormHostWithForms(forms map[string]client.InstalledFormReference) *formHost {
	counts := map[string]*lifecycleCounts{}
	for _, item := range resourceCases {
		counts[item.Kind] = &lifecycleCounts{}
	}
	return &formHost{forms: forms, resources: map[string]client.Resource{}, counts: counts}
}

func (h *formHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/.well-known/takoform" {
		origin := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_versions": []string{client.APIVersion},
			"features": map[string]bool{
				"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true, "idempotent_lifecycle": true,
				// Optional surface, advertised so the matrix covers the
				// declaration read without needing an external host.
				client.FeatureInterfaceDeclarations: true,
			},
			"endpoints": map[string]string{"api": origin + "/apis/forms.takoform.com/v1alpha1"},
		})
		return
	}
	const base = "/apis/forms.takoform.com/v1alpha1"
	if r.URL.Path == base+"/forms" {
		h.handleForms(w, r)
		return
	}
	if r.URL.Path == base+"/interfaces" || strings.HasPrefix(r.URL.Path, base+"/interfaces/") {
		h.handleInterfaces(w, r, strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, base+"/interfaces"), "/"))
		return
	}
	if r.URL.Path == base+"/resources/preview" {
		h.handlePreview(w, r)
		return
	}
	prefix := base + "/resources/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	kind, name := parts[0], parts[1]
	if _, ok := h.forms[kind]; !ok {
		http.NotFound(w, r)
		return
	}
	action := ""
	if len(parts) == 3 {
		action = parts[2]
	}
	switch {
	case r.Method == http.MethodPut && action == "":
		h.handleApply(w, r, kind, name)
	case r.Method == http.MethodGet && action == "":
		h.handleGet(w, r, kind, name)
	case r.Method == http.MethodPost && action == "observe":
		h.handleObserve(w, r, kind, name)
	case r.Method == http.MethodPost && action == "refresh":
		h.handleRefresh(w, r, kind, name)
	case r.Method == http.MethodPost && action == "import":
		h.handleImport(w, r, kind, name)
	case r.Method == http.MethodDelete && action == "":
		h.handleDelete(w, r, kind, name)
	default:
		http.NotFound(w, r)
	}
}

func (h *formHost) handleForms(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	form, ok := h.forms[kind]
	if !ok || !exactQuery(r, "prod", form) {
		http.Error(w, `{"error":{"code":"form_unknown","message":"unknown form","retryable":false}}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"forms": []client.FormAvailability{{
		Identity: form, DefinitionKnown: true, Installed: true, Executable: true, Activated: true, AvailableToPrincipal: true,
		Operations: []string{"create", "read", "update", "delete", "import", "refresh"},
	}}})
}

// handleInterfaces answers what this host declares. It is read-only and says
// nothing about who may consume a declaration: no binding, permission, or
// token is expressed here or anywhere else in the portable protocol.
func (h *formHost) handleInterfaces(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	declared := []client.DeclaredInterface{{
		Name:     conformanceInterfaceName,
		Version:  conformanceInterfaceVersion,
		Resource: client.InterfaceResourceRef{Kind: conformanceInterfaceKind, Name: conformanceInterfaceResourceName},
		Document: map[string]any{"title": "Portable assets bucket"},
		Values:   map[string]any{"endpoint": "https://example.test/s3"},
	}}
	if name == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": declared})
		return
	}
	for _, item := range declared {
		if item.Name == name {
			_ = json.NewEncoder(w).Encode(item)
			return
		}
	}
	http.Error(w, `{"error":{"code":"resource_not_found","message":"unknown interface","retryable":false}}`, http.StatusNotFound)
}

func (h *formHost) handlePreview(w http.ResponseWriter, r *http.Request) {
	var desired client.Resource
	if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(client.PreviewResourceResult{
		Resource: desired,
		Review:   client.PreviewReview{PlanDigest: "sha256:" + strings.Repeat("a", 64), SpecDigest: "sha256:" + strings.Repeat("b", 64)},
	})
}

func (h *formHost) handleApply(w http.ResponseWriter, r *http.Request, kind, name string) {
	var request struct {
		client.Resource
		Review client.DeploymentReview `json:"review"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := resourceKey(kind, name)
	current, exists := h.resources[key]
	version := 1
	if exists {
		if !matchFence(r, current.Metadata.ResourceVersion) || r.Header.Get("Idempotency-Key") == "" {
			http.Error(w, `{"error":{"code":"resource_version_conflict","message":"missing update fence","retryable":false}}`, http.StatusPreconditionFailed)
			return
		}
		version = decimalVersion(current.Metadata.ResourceVersion) + 1
		h.counts[kind].update++
	} else {
		if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Idempotency-Key") == "" {
			http.Error(w, `{"error":{"code":"resource_version_conflict","message":"missing create fence","retryable":false}}`, http.StatusPreconditionFailed)
			return
		}
		h.counts[kind].create++
	}
	resource := responseResource(request.Resource, version)
	h.resources[key] = resource
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, version))
	_ = json.NewEncoder(w).Encode(resource)
}

func (h *formHost) handleGet(w http.ResponseWriter, r *http.Request, kind, name string) {
	resource, ok := h.lookupExact(w, r, kind, name)
	if !ok {
		return
	}
	h.counts[kind].read++
	resource = h.maybeSubstitute(resource, kind)
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, resource.Metadata.ResourceVersion))
	_ = json.NewEncoder(w).Encode(resource)
}

func (h *formHost) handleObserve(w http.ResponseWriter, r *http.Request, kind, name string) {
	resource, ok := h.lookupExact(w, r, kind, name)
	if !ok {
		return
	}
	if !matchFence(r, resource.Metadata.ResourceVersion) || r.Header.Get("Idempotency-Key") == "" {
		http.Error(w, `{"error":{"code":"resource_version_conflict","message":"stale","retryable":false}}`, http.StatusPreconditionFailed)
		return
	}
	h.counts[kind].observe++
	if h.cliImportPhase {
		h.counts[kind].cliImport++
	}
	status := "current"
	if h.drift {
		status = "drifted"
	}
	resource = h.maybeSubstitute(resource, kind)
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, resource.Metadata.ResourceVersion))
	_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "observation": map[string]any{"status": status, "summary": status}})
}

func (h *formHost) handleRefresh(w http.ResponseWriter, r *http.Request, kind, name string) {
	resource, ok := h.lookupExact(w, r, kind, name)
	if !ok {
		return
	}
	if !matchFence(r, resource.Metadata.ResourceVersion) || r.Header.Get("Idempotency-Key") == "" {
		http.Error(w, `{"error":{"code":"resource_version_conflict","message":"stale","retryable":false}}`, http.StatusPreconditionFailed)
		return
	}
	h.counts[kind].refresh++
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, resource.Metadata.ResourceVersion))
	_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "refresh": map[string]any{"summary": "published"}})
}

func (h *formHost) handleImport(w http.ResponseWriter, r *http.Request, kind, name string) {
	var request struct {
		client.Resource
		NativeID string `json:"nativeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := resourceKey(kind, name)
	current, exists := h.resources[key]
	version := 1
	if exists {
		if !matchFence(r, current.Metadata.ResourceVersion) || r.Header.Get("Idempotency-Key") == "" {
			http.Error(w, `{"error":{"code":"resource_version_conflict","message":"missing import fence","retryable":false}}`, http.StatusPreconditionFailed)
			return
		}
		version = decimalVersion(current.Metadata.ResourceVersion) + 1
	} else if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Idempotency-Key") == "" {
		http.Error(w, `{"error":{"code":"resource_version_conflict","message":"missing import create fence","retryable":false}}`, http.StatusPreconditionFailed)
		return
	}
	resource := responseResource(request.Resource, version)
	h.resources[key] = resource
	h.counts[kind].nativeImport++
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, version))
	_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "import": map[string]any{"summary": "adopted"}})
}

func (h *formHost) handleDelete(w http.ResponseWriter, r *http.Request, kind, name string) {
	resource, ok := h.lookupExact(w, r, kind, name)
	if !ok {
		return
	}
	if !matchFence(r, resource.Metadata.ResourceVersion) || r.Header.Get("Idempotency-Key") == "" {
		http.Error(w, `{"error":{"code":"resource_version_conflict","message":"stale","retryable":false}}`, http.StatusPreconditionFailed)
		return
	}
	delete(h.resources, resourceKey(kind, name))
	h.counts[kind].delete++
	w.WriteHeader(http.StatusNoContent)
}

func (h *formHost) lookupExact(w http.ResponseWriter, r *http.Request, kind, name string) (client.Resource, bool) {
	form := h.forms[kind]
	if !exactQuery(r, "prod", form) {
		http.Error(w, `{"error":{"code":"form_identity_conflict","message":"exact form mismatch","retryable":false}}`, http.StatusConflict)
		return client.Resource{}, false
	}
	resource, ok := h.resources[resourceKey(kind, name)]
	if !ok {
		http.Error(w, `{"error":{"code":"resource_not_found","message":"missing","retryable":false}}`, http.StatusNotFound)
		return client.Resource{}, false
	}
	return resource, true
}

func (h *formHost) maybeSubstitute(resource client.Resource, kind string) client.Resource {
	if h.substitutionKind == kind {
		switch h.substitutionField {
		case "name":
			resource.Metadata.Name = "substituted"
		case "packageDigest":
			if resource.Form != nil {
				form := *resource.Form
				form.PackageDigest = "sha256:" + strings.Repeat("f", 64)
				resource.Form = &form
			}
		}
	}
	return resource
}

func (h *formHost) setDrift(value bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drift = value
}

func (h *formHost) setSubstitution(kind, field string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.substitutionKind = kind
	h.substitutionField = field
}

func (h *formHost) setCLIImport(value bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cliImportPhase = value
}

func (h *formHost) markDriftStateVerified() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, item := range resourceCases {
		h.counts[item.Kind].driftState++
	}
}

func (h *formHost) report(identity CLIIdentity, providerBinary ProviderBinaryIdentity, providerSchemaSHA256, installationSource string, immutable []ImmutableReplaceEvidence) Report {
	h.mu.Lock()
	defer h.mu.Unlock()
	immutableByKindField := map[string]bool{}
	for _, evidence := range immutable {
		immutableByKindField[evidence.Kind+evidence.Field] = evidence.Passed
	}
	resources := make([]ResourceEvidence, 0, len(resourceCases))
	for _, item := range resourceCases {
		counts := h.counts[item.Kind]
		resources = append(resources, ResourceEvidence{
			Kind: item.Kind, ResourceType: item.ResourceType,
			Checks: CheckEvidence{
				Create: counts.create > 0, Read: counts.read > 0, Update: counts.update > 0,
				Observe: counts.observe > 0, Refresh: counts.refresh > 0, NativeImport: counts.nativeImport > 0,
				CLIImport: counts.cliImport > 0, Delete: counts.delete > 0, DriftState: counts.driftState > 0,
				NameReplace: immutableByKindField[item.Kind+"/name"],
			},
		})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Kind < resources[j].Kind })
	return Report{
		Format: ReportFormat, Classification: "generic-lifecycle-candidate", PublicationReady: false,
		BindingStatus: "exact-structural-candidate-set", RunnerSubject: RunnerSubject,
		Protocol: providerProtocol, InstallationSource: installationSource,
		CandidateSetSHA256: candidateSetSHA256(), ProviderSchemaSHA256: providerSchemaSHA256,
		ProviderBinary: providerBinary, CLI: identity,
		Resources: resources,
		NegativeChecks: []NegativeEvidence{
			{Name: "response-name-substitution-rejected", Kind: client.KindObjectBucket,
				Fixture: "versioned host observe response with substituted metadata.name", Passed: true},
			{Name: "response-package-digest-substitution-rejected", Kind: client.KindKVStore,
				Fixture: "versioned host observe response with substituted exact FormRef packageDigest", Passed: true},
		},
		ImmutableReplace: immutable,
	}
}

func responseResource(resource client.Resource, version int) client.Resource {
	resource.Metadata.ResourceVersion = fmt.Sprintf("%d", version)
	resource.Status = &client.Status{Phase: "Ready", Portability: "portable", Outputs: map[string]any{"reference": resource.Metadata.Name + "-output"}}
	return resource
}

func exactQuery(r *http.Request, space string, form client.InstalledFormReference) bool {
	query := r.URL.Query()
	return query.Get("space") == space && query.Get("apiVersion") == form.FormRef.APIVersion &&
		query.Get("kind") == form.FormRef.Kind && query.Get("definitionVersion") == form.FormRef.DefinitionVersion &&
		query.Get("schemaDigest") == form.FormRef.SchemaDigest && query.Get("packageDigest") == form.PackageDigest
}

func matchFence(r *http.Request, version string) bool {
	return r.Header.Get("If-Match") == `"`+version+`"`
}
func resourceKey(kind, name string) string { return kind + "/" + name }
func decimalVersion(value string) int {
	var version int
	_, _ = fmt.Sscanf(value, "%d", &version)
	return version
}

func RepoRoot(start string) (string, error) { return characterization.FindRepoRoot(start) }
