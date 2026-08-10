// Package workerauthoring drives the Edge Platform Family authoring surface —
// the raw v1beta1 resources and the official `worker-app` module — through a
// REAL Terraform-compatible CLI against the deterministic v1beta1 reference
// host.
//
// It exists because two of the properties the authoring surface promises are
// not decidable from the provider's unit tests. Whether an immutable revision
// can be replaced under the same host name is a question about the ORDER
// Terraform picks and the refusals the host raises, and whether a worker keeps
// serving across a code change is a question about what is true BETWEEN two
// applies. Both need the real planner, the real graph, and a real host.
//
// Nothing here is publishable evidence: the host is a disposable in-process
// reference implementation and the provider is built from the working tree.
package workerauthoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformancev3"
)

// ProviderAddress is the one canonical FQN both supported CLIs install the
// provider under.
const ProviderAddress = "registry.terraform.io/tako0614/takoform"

// CLIIdentity is the exact CLI one scenario ran under.
type CLIIdentity struct {
	Product string `json:"product"`
	Version string `json:"version"`
}

// harness is one disposable stack: a built provider binary, a dev-override CLI
// configuration, a working directory, and the reference host the configuration
// points at.
type harness struct {
	repoRoot string
	cli      string
	identity CLIIdentity
	workDir  string
	env      []string
	server   *httptest.Server
	host     *recordingHost
	// store is the same reference host the recorder wraps, kept typed so a
	// teardown scenario can ask what is LEFT. The wire has no list route, so
	// nothing else can answer "nothing was left behind" rather than "the names
	// I remembered are gone".
	store   *portableconformancev3.ReferenceHost
	cleanup []func()
}

// harnessOptions selects which Forms the disposable host answers support for.
type harnessOptions struct {
	// unsupportedKinds are Form kinds the host still installs but declares no
	// support profile for, so a plan-time capability check has something real
	// to refuse.
	unsupportedKinds []string
}

func startHarness(ctx context.Context, repoRoot, cliPath string, options harnessOptions) (*harness, error) {
	executable, identity, err := identifyCLI(ctx, cliPath)
	if err != nil {
		return nil, err
	}
	contract, err := portableconformancev3.Verify(filepath.Join(repoRoot, "conformance", "portable-host-v3"))
	if err != nil {
		return nil, fmt.Errorf("verify portable host v3 contract: %w", err)
	}
	catalog, err := portableconformancev3.LoadCatalog(repoRoot, contract)
	if err != nil {
		return nil, fmt.Errorf("load Edge Family candidate catalog: %w", err)
	}
	h := &harness{repoRoot: repoRoot, cli: executable, identity: identity}
	temp, err := os.MkdirTemp("", "takoform-worker-authoring-")
	if err != nil {
		return nil, err
	}
	h.cleanup = append(h.cleanup, func() { _ = os.RemoveAll(temp) })

	h.store = portableconformancev3.NewReferenceHost(contract, catalog)
	h.host = newRecordingHost(h.store, options.unsupportedKinds)
	h.server = httptest.NewServer(h.host)
	h.cleanup = append(h.cleanup, h.server.Close)

	binDir := filepath.Join(temp, "bin")
	h.workDir = filepath.Join(temp, "stack")
	for _, dir := range []string{binDir, h.workDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			h.Close()
			return nil, err
		}
	}
	version, err := providerVersion(repoRoot)
	if err != nil {
		h.Close()
		return nil, err
	}
	binary := filepath.Join(binDir, "terraform-provider-takoform")
	if output, err := buildProvider(ctx, repoRoot, version, binary); err != nil {
		h.Close()
		return nil, fmt.Errorf("build provider binary: %w\n%s", err, output)
	}
	cliConfig := filepath.Join(temp, "terraformrc")
	body := fmt.Sprintf(`provider_installation {
  dev_overrides {
    %q = %q
  }
  direct {}
}
`, ProviderAddress, binDir)
	if err := os.WriteFile(cliConfig, []byte(body), 0o600); err != nil {
		h.Close()
		return nil, err
	}
	h.env = append(sanitizedEnvironment(),
		"TF_CLI_CONFIG_FILE="+cliConfig,
		"TF_IN_AUTOMATION=1",
		"CHECKPOINT_DISABLE=1",
		"TF_PLUGIN_CACHE_DIR=",
	)
	return h, nil
}

func (h *harness) Close() {
	for index := len(h.cleanup) - 1; index >= 0; index-- {
		h.cleanup[index]()
	}
	h.cleanup = nil
}

// Endpoint is the disposable host origin the generated configuration points at.
func (h *harness) Endpoint() string { return h.server.URL }

// write replaces the stack configuration.
func (h *harness) write(name, body string) error {
	return os.WriteFile(filepath.Join(h.workDir, name), []byte(body), 0o600)
}

// run executes one CLI action inside the stack directory.
func (h *harness) run(ctx context.Context, args ...string) (string, error) {
	base := append([]string{"-chdir=" + h.workDir}, args...)
	return runCommand(ctx, h.repoRoot, h.env, h.cli, base...)
}

// runIn executes one CLI action inside an arbitrary directory, which is what a
// configuration-only check (validate) needs.
func (h *harness) runIn(ctx context.Context, dir string, args ...string) (string, error) {
	base := append([]string{"-chdir=" + dir}, args...)
	return runCommand(ctx, h.repoRoot, h.env, h.cli, base...)
}

func identifyCLI(ctx context.Context, requested string) (string, CLIIdentity, error) {
	if requested == "" {
		requested = "tofu"
	}
	executable, err := exec.LookPath(requested)
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("worker authoring conformance requires a Terraform-compatible CLI (%s): %w", requested, err)
	}
	if executable, err = filepath.Abs(executable); err != nil {
		return "", CLIIdentity{}, err
	}
	base := sanitizedEnvironment()
	versionJSON, err := runCommand(ctx, ".", base, executable, "version", "-json")
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("inspect CLI version: %w\n%s", err, versionJSON)
	}
	var decoded struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal([]byte(versionJSON), &decoded); err != nil || decoded.TerraformVersion == "" {
		return "", CLIIdentity{}, fmt.Errorf("inspect CLI version JSON: %v", err)
	}
	plain, err := runCommand(ctx, ".", base, executable, "version")
	if err != nil {
		return "", CLIIdentity{}, fmt.Errorf("inspect CLI product: %w\n%s", err, plain)
	}
	switch {
	case strings.HasPrefix(plain, "OpenTofu "):
		return executable, CLIIdentity{Product: "OpenTofu", Version: decoded.TerraformVersion}, nil
	case strings.HasPrefix(plain, "Terraform "):
		return executable, CLIIdentity{Product: "Terraform", Version: decoded.TerraformVersion}, nil
	}
	return "", CLIIdentity{}, errors.New("unsupported Terraform-compatible CLI product")
}

func providerVersion(repoRoot string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "release", "version.json"))
	if err != nil {
		return "", err
	}
	var descriptor struct {
		Version   string `json:"version"`
		GoVersion string `json:"goVersion"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return "", err
	}
	if descriptor.Version == "" {
		return "", errors.New("release descriptor carries no provider version")
	}
	return descriptor.Version, nil
}

func buildProvider(ctx context.Context, repoRoot, version, outputPath string) (string, error) {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "CGO_ENABLED=") {
			continue
		}
		environment = append(environment, item)
	}
	environment = append(environment, "CGO_ENABLED=0")
	return runCommand(ctx, repoRoot, environment, "go", "build",
		"-buildvcs=false", "-ldflags", "-X main.version="+version, "-o", outputPath, ".")
}

func sanitizedEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "TF_") || strings.HasPrefix(key, "TOFU_") ||
			strings.HasPrefix(key, "TAKOFORM_") || key == "CHECKPOINT_DISABLE" {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
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
