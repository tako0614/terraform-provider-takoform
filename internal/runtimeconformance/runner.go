package runtimeconformance

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/fakeruntime"
)

const (
	// ReportFormat identifies the runtime ABI runner report.
	ReportFormat = "takoform.runtime-abi-runner-report@v1"
	// FakeRuntimeSelfTest classifies a run against the in-process stand-in.
	// It proves the corpus and the runner; it proves nothing about a runtime.
	FakeRuntimeSelfTest = "in-process-fake-runtime-self-test"
	// DeployedRuntimeRun classifies a run against a deployed worker on a
	// runtime under test. This is the only classification that says anything
	// about a runtime.
	DeployedRuntimeRun = "deployed-runtime-conformance-run"

	// selfTestProves and deployedProves are the report's own statement of what
	// the run it describes is worth. They exist so a reader who sees only the
	// report cannot mistake one for the other.
	selfTestProves = "this run exercised conformance/runtime-abi-v1 against an in-process stand-in that has no JavaScript engine: it proves the corpus and the runner, it proves nothing about any runtime, and it never closes publication blocker V3-008"
	deployedProves = "this run executed conformance/runtime-abi-v1 against a deployed worker built from the corpus bundle: what passed is the named runtime's own behaviour under the measured ABI"

	// blockerEvidence states the exact evidence that closes V3-008.
	blockerEvidence = "V3-008 closes on a passed deployed-runtime-conformance-run whose load lane was measured, against a runtime this repository does not own; a self-test never closes it"
)

// Check outcomes.
const (
	OutcomePassed     = "passed"
	OutcomeFailed     = "failed"
	OutcomeUnmeasured = "unmeasured"
)

// CheckEvidence records one executed, refused, or unmeasured check.
type CheckEvidence struct {
	Name      string `json:"name"`
	Operation string `json:"operation"`
	Procedure string `json:"procedure"`
	Outcome   string `json:"outcome"`
	Proves    string `json:"proves"`
	Observed  string `json:"observed"`
	Detail    string `json:"detail,omitempty"`
}

// Report is the data-only result of one run. PublicationReady is always false:
// this report proves conformance behaviour only and grants no publication,
// admission, support, or maturity state.
type Report struct {
	Format           string          `json:"format"`
	Classification   string          `json:"classification"`
	Proves           string          `json:"proves"`
	PublicationReady bool            `json:"publicationReady"`
	Status           string          `json:"status"`
	Completeness     string          `json:"completeness"`
	Interface        InterfaceRef    `json:"interface"`
	ContractDigest   string          `json:"contractDigest"`
	Subject          string          `json:"subject"`
	WorkerEndpoint   string          `json:"workerEndpoint"`
	LoaderEndpoint   string          `json:"loaderEndpoint"`
	RunToken         string          `json:"runToken"`
	Measured         int             `json:"measured"`
	Unmeasured       int             `json:"unmeasured"`
	Failed           int             `json:"failed"`
	Checks           []CheckEvidence `json:"checks"`
	BlockerEvidence  string          `json:"blockerEvidence"`
}

// EndpointOptions configures one run against a deployed worker.
type EndpointOptions struct {
	// Endpoint is the base URL the corpus worker answers on. Every probe
	// route is addressed under it.
	Endpoint string
	// LoaderEndpoint is the disposable adapter over the runtime's own module
	// loader. When it is empty the load lane is reported unmeasured and the
	// run is incomplete, because those outcomes are decided before any
	// traffic arrives and no request to a running worker can observe them.
	LoaderEndpoint string
	// Token and LoaderToken are whatever credential the operator's deployment
	// requires; each is sent as a bearer credential when present.
	Token       string
	LoaderToken string

	HTTPClient     *http.Client
	Classification string
	Subject        string
}

// SelfTest runs the whole matrix against the in-process stand-in over real
// HTTP. It is corpus evidence only: the classification and the report's own
// `proves` sentence say so, and no report from this path is ever runtime
// evidence.
func SelfTest(ctx context.Context, contract Contract) (Report, error) {
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		return Report{}, err
	}
	runtime, err := fakeruntime.New(deployment, fakeruntime.Options{})
	if err != nil {
		return Report{}, err
	}
	defer runtime.Close()
	worker := httptest.NewServer(runtime.WorkerHandler())
	defer worker.Close()
	loader := httptest.NewServer(runtime.LoaderHandler())
	defer loader.Close()
	return RunEndpoint(ctx, contract, EndpointOptions{
		Endpoint:       worker.URL,
		LoaderEndpoint: loader.URL,
		HTTPClient:     worker.Client(),
		Classification: FakeRuntimeSelfTest,
		Subject:        "in-process-fake-runtime",
	})
}

// RunEndpoint executes the whole matrix against a deployed worker over HTTP.
// This is the entry point a real runtime is measured with: the operator
// deploys the corpus bundle exactly as `deployment` states, and supplies the
// worker's base URL, whatever credential it needs, and — to measure the load
// lane at all — an adapter over the runtime's module loader.
func RunEndpoint(ctx context.Context, contract Contract, options EndpointOptions) (Report, error) {
	if err := validateContract(contract); err != nil {
		return Report{}, fmt.Errorf("runtime ABI contract: %w", err)
	}
	runToken, err := mintRunToken(contract.RunCorrelation)
	if err != nil {
		return Report{}, err
	}
	return runEndpoint(ctx, contract, options, runToken)
}

// mintRunToken mints the token that makes one run's observations its own. It is
// unpredictable rather than merely unique because the corpus is public: a
// counter or a clock reading would let a runtime precompute the observations a
// future run will look for, which is the shape of evidence this lane refuses.
//
// A fresh token per run does not make a run's OUTCOME vary. Every check resolves
// its pinned template against whatever token it was given, so a conforming
// runtime passes for every token and a non-conforming one fails for every
// token; the self-test is reproducible in the only sense that matters.
func mintRunToken(correlation RunCorrelation) (string, error) {
	raw := make([]byte, correlation.TokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mint a runtime ABI run token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// runEndpoint is RunEndpoint with the run's token supplied rather than minted,
// so a test can state what happens when a corpus reuses one correlation value
// across runs — the defect the run token exists to close.
func runEndpoint(
	ctx context.Context, contract Contract, options EndpointOptions, runToken string,
) (Report, error) {
	classification := options.Classification
	if classification == "" {
		classification = DeployedRuntimeRun
	}
	if classification != FakeRuntimeSelfTest && classification != DeployedRuntimeRun {
		return Report{}, fmt.Errorf("runtime ABI run classification %q is not one this lane issues", classification)
	}
	endpoint, err := normalizeEndpoint(options.Endpoint, classification)
	if err != nil {
		return Report{}, err
	}
	if strings.TrimSpace(runToken) == "" {
		return Report{}, errors.New("a runtime ABI run must carry a correlation token")
	}
	loaderEndpoint := ""
	if strings.TrimSpace(options.LoaderEndpoint) != "" {
		loaderEndpoint, err = normalizeEndpoint(options.LoaderEndpoint, classification)
		if err != nil {
			return Report{}, err
		}
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	subject := strings.TrimSpace(options.Subject)
	if subject == "" {
		subject = endpoint
	}
	target := &target{
		client: client, worker: endpoint, loader: loaderEndpoint,
		token: strings.TrimSpace(options.Token), loaderToken: strings.TrimSpace(options.LoaderToken),
		runToken: runToken,
	}
	report := Report{
		Format:           ReportFormat,
		Classification:   classification,
		Proves:           selfTestProves,
		PublicationReady: false,
		Interface:        contract.Interface,
		ContractDigest:   contract.Digest(),
		Subject:          subject,
		WorkerEndpoint:   endpoint,
		LoaderEndpoint:   loaderEndpoint,
		RunToken:         runToken,
		BlockerEvidence:  blockerEvidence,
	}
	if classification == DeployedRuntimeRun {
		report.Proves = deployedProves
	}
	for _, check := range contract.Checks {
		evidence := runCheck(ctx, contract, target, check)
		report.Checks = append(report.Checks, evidence)
		switch evidence.Outcome {
		case OutcomePassed:
			report.Measured++
		case OutcomeUnmeasured:
			report.Unmeasured++
		default:
			report.Failed++
		}
	}
	report.Status = OutcomePassed
	if report.Failed > 0 {
		report.Status = OutcomeFailed
	}
	report.Completeness = "complete"
	if report.Unmeasured > 0 {
		report.Completeness = "partial"
	}
	if report.Failed > 0 {
		return report, fmt.Errorf("runtime ABI conformance: %d of %d checks failed",
			report.Failed, len(contract.Checks))
	}
	return report, nil
}

// normalizeEndpoint refuses an endpoint a run must never be pointed at. A
// deployed run travels over HTTPS unless the operator is measuring something
// on their own loopback, where there is no network to protect.
func normalizeEndpoint(raw, classification string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", errors.New("runtime ABI runner requires an endpoint")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("runtime ABI runner endpoint: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("runtime ABI runner endpoint scheme %q is not http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("runtime ABI runner endpoint must carry a host")
	}
	if parsed.User != nil {
		return "", errors.New("runtime ABI runner endpoint must not carry userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("runtime ABI runner endpoint must not carry a query or fragment")
	}
	if parsed.Scheme == "http" && classification == DeployedRuntimeRun && !isLoopback(parsed.Hostname()) {
		return "", errors.New("a deployed runtime ABI run requires https unless the endpoint is loopback")
	}
	return trimmed, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func encodeBase64(input []byte) string {
	return base64.StdEncoding.EncodeToString(input)
}
