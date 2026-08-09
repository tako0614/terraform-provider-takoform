// Package runtimeconformance is the ES Module Worker runtime ABI conformance
// lane: the loader for the conformance/runtime-abi-v1 corpus and the runner
// that executes it against a runtime.
//
// It measures a different subject from internal/portableconformancev3. That
// lane drives the Host API and therefore proves what a host SAYS about the
// runtime it implements and what it REFUSES to store; it cannot observe a
// JavaScript isolate at all, and spec/host-api/v1alpha3.md lists the seven
// obligations it leaves unproven. This lane executes those obligations against
// a runtime: it hands a module loader bytes, and it drives a deployed worker
// built from the corpus bundle over HTTP.
//
// A run against the in-process stand-in proves the CORPUS. Only a run against
// a deployed worker proves a RUNTIME, and the report says which one happened
// ([decision 0023](../../spec/decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)).
package runtimeconformance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/fakeruntime"
	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/workerbundle"
)

// Corpus identities.
const (
	// ContractFormat identifies the runtime ABI conformance contract.
	ContractFormat = "takoform.runtime-abi-conformance@v1"
	// ManifestFormat identifies the corpus manifest that pins it.
	ManifestFormat = "takoform.runtime-abi-conformance-manifest@v1"
	// InterfaceAPIVersion is the exact Interface lane the measured contract
	// belongs to.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// InterfaceName is the exact Interface this corpus measures.
	InterfaceName = "worker.runtime"
	// InterfaceVersion is the exact Interface version this corpus measures.
	InterfaceVersion = "1.0.0"
	// ServiceInterfaceName is the second exact Interface this corpus reaches:
	// worker-to-worker invocation, measured through the `module-worker.service`
	// binding the deployment declares. It is pinned separately, and by digest,
	// for the reason the runtime ABI is: its delivery model is a claim about
	// bytes moving, and a corpus measuring a contract it does not name could
	// go on measuring one that no longer exists.
	ServiceInterfaceName = "worker.service"
	// ServiceInterfaceVersion is the exact version of that contract.
	ServiceInterfaceVersion = "1.0.0"
)

// The closed procedure vocabulary: one runner procedure decides each check.
const (
	ProcedureLoad                 = "load"
	ProcedureRequest              = "request"
	ProcedureEnvironment          = "environment"
	ProcedureGlobals              = "globals"
	ProcedureKVRoundTrip          = "kvRoundTrip"
	ProcedureRequestStream        = "requestStream"
	ProcedureResponseStream       = "responseStream"
	ProcedureWaitUntil            = "waitUntil"
	ProcedureScheduledObservation = "scheduledObservation"
	ProcedureQueueRoundTrip       = "queueRoundTrip"
	ProcedureUnmeasured           = "unmeasured"
)

// requiredChecks is the closed executed-check list every runtime ABI run must
// account for, in contract order. A corpus that names a different list is a
// different corpus and is rejected rather than run.
var requiredChecks = []string{
	"module-loads-and-exports-are-derived-from-bytes",
	"unsupported-module-media-type-refused",
	"main-module-missing-refused",
	"unparseable-module-refused",
	"declared-handler-not-exported-refused",
	"fetch-receives-request-env-and-context",
	"fetch-returns-a-response",
	"fetch-uncaught-throw-is-a-host-generated-response",
	"environment-projects-exactly-the-declared-names",
	"globals-floor-present",
	"kv-binding-round-trips-bytes-through-env",
	"request-body-streams-rather-than-buffering",
	"response-body-streams-rather-than-buffering",
	"service-request-body-streams-to-the-callee",
	"service-response-body-streams-from-the-callee",
	"wait-until-holds-the-isolate-and-a-rejection-leaves-the-response-alone",
	"scheduled-invoked-by-the-host-cron-attachment",
	"queue-batch-delivered-to-the-queue-handler",
}

var (
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	noncePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// InterfaceRef is the exact identity of the Interface contract a corpus
// measures. Two definitions with different canonical bytes are different
// Interfaces even under one display name, so the digest is part of the
// identity rather than a checksum of it.
type InterfaceRef struct {
	APIVersion   string `json:"apiVersion"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

// ProbeProtocol describes the observation surface the corpus module serves.
type ProbeProtocol struct {
	Protocol       string `json:"protocol"`
	RoutePrefix    string `json:"routePrefix"`
	KVBinding      string `json:"kvBinding"`
	QueueBinding   string `json:"queueBinding"`
	ServiceBinding string `json:"serviceBinding"`
	Note           string `json:"note"`
}

// LoaderProtocol describes the disposable adapter over a runtime's own module
// loader. The loader is not part of the ABI; it is how a run reaches an
// operation whose outcome is decided before any traffic arrives.
type LoaderProtocol struct {
	Protocol string `json:"protocol"`
	Method   string `json:"method"`
	Note     string `json:"note"`
}

// DeploymentBinding is one binding the measured deployment must declare.
type DeploymentBinding struct {
	Name      string `json:"name"`
	Interface string `json:"interface"`
}

// PeerContract is the SECOND worker an operator deploys: the callee the
// deployment's `worker.service` binding addresses.
//
// It is a worker of its own rather than the measured worker under another name.
// The binding projects a call to ANOTHER Module Worker, so a corpus that bound
// the worker to itself would let a host pass by short-circuiting into its own
// handler, and the two service checks would measure nothing the direct
// streaming checks do not already measure. It runs the same byte-pinned bundle,
// which is what lets one module answer both sides.
type PeerContract struct {
	Bundle           string   `json:"bundle"`
	DeclaredHandlers []string `json:"declaredHandlers"`
	Note             string   `json:"note"`
}

// DeploymentContract is the one worker a conforming run is measured against.
// An operator reproduces it exactly; everything the run expects about `env`
// and about host-driven invocation follows from it.
type DeploymentContract struct {
	Bundle                   string              `json:"bundle"`
	DeclaredHandlers         []string            `json:"declaredHandlers"`
	Vars                     []string            `json:"vars"`
	SensitiveVars            []string            `json:"sensitiveVars"`
	Bindings                 []DeploymentBinding `json:"bindings"`
	EnvironmentPropertyNames []string            `json:"environmentPropertyNames"`
	Cron                     string              `json:"cron"`
	Queue                    string              `json:"queue"`
	Peer                     *PeerContract       `json:"peer,omitempty"`
}

// ModuleContract is one byte-pinned module of a corpus bundle.
type ModuleContract struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Source    string `json:"source"`
	SHA256    string `json:"sha256"`

	// bytes is the verified content; it is read, never decoded.
	bytes []byte
}

// BundleContract is one module set the corpus drives the loader with. Exactly
// one of ExportedHandlers and UnloadableError is present: a bundle either
// loads and exports a stated set, or fails for a stated reason, and the loader
// below proves the stated outcome is the one its bytes actually produce.
type BundleContract struct {
	Name             string           `json:"name"`
	MainModule       string           `json:"mainModule"`
	Modules          []ModuleContract `json:"modules"`
	ExportedHandlers []string         `json:"exportedHandlers,omitempty"`
	UnloadableError  string           `json:"unloadableError,omitempty"`
}

// LoadCase is what a load check hands the loader and what it expects back.
type LoadCase struct {
	DeclaredHandlers       []string `json:"declaredHandlers"`
	ExpectExportedHandlers []string `json:"expectExportedHandlers,omitempty"`
	ExpectError            string   `json:"expectError,omitempty"`
}

// RequestCase is one HTTP request against the deployed worker.
type RequestCase struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// ExpectCase is the observation that decides a request check.
type ExpectCase struct {
	Status int            `json:"status"`
	JSON   map[string]any `json:"json,omitempty"`
}

// TimingCase carries the bounds a timing-sensitive procedure needs. Every one
// of them is a maximum a conforming runtime stays inside, never a delay the
// runner adds for its own convenience.
type TimingCase struct {
	DeadlineSeconds  int   `json:"deadlineSeconds,omitempty"`
	PollMillis       int   `json:"pollMillis,omitempty"`
	GapMillis        int   `json:"gapMillis,omitempty"`
	DelayMillis      int   `json:"delayMillis,omitempty"`
	Chunks           int   `json:"chunks,omitempty"`
	RequestChunkSize []int `json:"requestChunkSize,omitempty"`
}

// PayloadCase is what a correlated check writes: the exact bytes it round
// trips, and the TEMPLATE its correlation value is derived from. The template
// carries the corpus's run-correlation placeholder; the value that reaches the
// runtime is the template with this run's token substituted, so no observation
// a previous run stored can answer this one.
type PayloadCase struct {
	NonceTemplate string `json:"nonceTemplate"`
	Bytes         string `json:"bytesBase64,omitempty"`
}

// RunCorrelation is how a byte-pinned corpus states a per-run value. A pinned
// constant cannot correlate a run and a per-run value cannot be pinned, so the
// corpus pins the TEMPLATE and the placeholder within it: the runner mints one
// token per run, substitutes it everywhere the placeholder appears, and derives
// the observation it expects by the same substitution. The corpus bytes never
// move; the token is never stored in them.
type RunCorrelation struct {
	Placeholder string `json:"placeholder"`
	TokenBytes  int    `json:"tokenBytes"`
	Note        string `json:"note"`
}

// Resolve substitutes a run's token for every occurrence of the placeholder.
// It is the ONE substitution: the value the runner sends and the observation
// the runner expects are both produced by it, so they cannot drift apart.
func (c RunCorrelation) Resolve(template, token string) string {
	return strings.ReplaceAll(template, c.Placeholder, token)
}

// UnmeasuredCase states why a declared surface is not measured, and what would
// have to exist for it to be. It is the alternative to leaving a handler in a
// published-shaped contract that nothing ever checks.
type UnmeasuredCase struct {
	Reason      string `json:"reason"`
	WouldUse    string `json:"wouldUse"`
	ClosedBy    string `json:"closedBy"`
	BlockerID   string `json:"blockerId"`
	Recommended string `json:"recommended"`
}

// Check is one required conformance check: what it proves, which procedure
// decides it, and the exact inputs and expected observations.
type Check struct {
	Name       string          `json:"name"`
	Operation  string          `json:"operation"`
	Procedure  string          `json:"procedure"`
	Proves     string          `json:"proves"`
	Bundle     string          `json:"bundle,omitempty"`
	Load       *LoadCase       `json:"load,omitempty"`
	Request    *RequestCase    `json:"request,omitempty"`
	Expect     *ExpectCase     `json:"expect,omitempty"`
	Timing     *TimingCase     `json:"timing,omitempty"`
	Payload    *PayloadCase    `json:"payload,omitempty"`
	Unmeasured *UnmeasuredCase `json:"unmeasured,omitempty"`
}

// Contract is the verified conformance/runtime-abi-v1 contract.
type Contract struct {
	Format             string             `json:"format"`
	Interface          InterfaceRef       `json:"interface"`
	ServiceInterface   InterfaceRef       `json:"serviceInterface"`
	HandlerVocabulary  []string           `json:"handlerVocabulary"`
	LoadableMediaTypes []string           `json:"loadableMediaTypes"`
	GlobalsFloor       []string           `json:"globalsFloor"`
	ProbeProtocol      ProbeProtocol      `json:"probeProtocol"`
	LoaderProtocol     LoaderProtocol     `json:"loaderProtocol"`
	RunCorrelation     RunCorrelation     `json:"runCorrelation"`
	Deployment         DeploymentContract `json:"deployment"`
	Bundles            []BundleContract   `json:"bundles"`
	Checks             []Check            `json:"checks"`
	RequiredChecks     []string           `json:"requiredChecks"`

	// root and digest are derived from the verified corpus, never decoded.
	root   string
	digest string
}

// Root returns the corpus directory this contract was verified from.
func (c Contract) Root() string { return c.root }

// Digest returns the sha256 of the exact contract bytes the manifest pinned.
func (c Contract) Digest() string { return c.digest }

// Bundle returns the named corpus bundle.
func (c Contract) Bundle(name string) (BundleContract, bool) {
	for _, bundle := range c.Bundles {
		if bundle.Name == name {
			return bundle, true
		}
	}
	return BundleContract{}, false
}

// WorkerBundle converts a corpus bundle into the module-level value the loader
// and the stand-in share.
func (b BundleContract) WorkerBundle() workerbundle.Bundle {
	modules := make([]workerbundle.Module, 0, len(b.Modules))
	for _, module := range b.Modules {
		modules = append(modules, workerbundle.Module{
			Name: module.Name, MediaType: module.MediaType, Source: module.bytes,
		})
	}
	return workerbundle.Bundle{Name: b.Name, MainModule: b.MainModule, Modules: modules}
}

// Deployment converts the contract's deployment description into the value a
// runtime is constructed from.
func (c Contract) WorkerDeployment() (workerbundle.Deployment, error) {
	bundle, ok := c.Bundle(c.Deployment.Bundle)
	if !ok {
		return workerbundle.Deployment{}, fmt.Errorf(
			"runtime ABI corpus deployment names unknown bundle %q", c.Deployment.Bundle)
	}
	bindings := make([]workerbundle.Binding, 0, len(c.Deployment.Bindings))
	for _, binding := range c.Deployment.Bindings {
		bindings = append(bindings, workerbundle.Binding{
			Name: binding.Name, Interface: binding.Interface,
		})
	}
	deployment := workerbundle.Deployment{
		Bundle:           bundle.WorkerBundle(),
		DeclaredHandlers: append([]string(nil), c.Deployment.DeclaredHandlers...),
		Vars:             append([]string(nil), c.Deployment.Vars...),
		SensitiveVars:    append([]string(nil), c.Deployment.SensitiveVars...),
		Bindings:         bindings,
		Cron:             c.Deployment.Cron,
		Queue:            c.Deployment.Queue,
	}
	if c.Deployment.Peer != nil {
		peerBundle, ok := c.Bundle(c.Deployment.Peer.Bundle)
		if !ok {
			return workerbundle.Deployment{}, fmt.Errorf(
				"runtime ABI corpus peer deployment names unknown bundle %q", c.Deployment.Peer.Bundle)
		}
		deployment.Peer = &workerbundle.Deployment{
			Bundle:           peerBundle.WorkerBundle(),
			DeclaredHandlers: append([]string(nil), c.Deployment.Peer.DeclaredHandlers...),
		}
	}
	return deployment, nil
}

type manifest struct {
	Format   string `json:"format"`
	Contract string `json:"contract"`
	SHA256   string `json:"sha256"`
}

// Verify loads and fail-closed-verifies a runtime-abi-v1 corpus: manifest
// identity, contract byte digest, strict I-JSON decoding, module bytes, the
// pinned Interface identity and vocabularies, and the honesty rules that make
// every check passable by a conforming runtime and failable by a
// non-conforming one.
func Verify(root string) (Contract, error) {
	var index manifest
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Contract{}, err
	}
	if err := formpackage.DecodeStrictIJSON(raw, &index); err != nil {
		return Contract{}, fmt.Errorf("decode runtime ABI corpus manifest: %w", err)
	}
	if index.Format != ManifestFormat || index.Contract != "contract.json" {
		return Contract{}, errors.New("runtime ABI conformance manifest identity is invalid")
	}
	contractPath := filepath.Join(root, index.Contract)
	contractBytes, err := os.ReadFile(contractPath)
	if err != nil {
		return Contract{}, err
	}
	sum := sha256.Sum256(contractBytes)
	digest := hex.EncodeToString(sum[:])
	if digest != index.SHA256 {
		return Contract{}, errors.New("runtime ABI conformance contract digest drifted")
	}
	var contract Contract
	if err := formpackage.DecodeStrictIJSON(contractBytes, &contract); err != nil {
		return Contract{}, fmt.Errorf("decode %s: %w", contractPath, err)
	}
	contract.root = root
	contract.digest = "sha256:" + digest
	if err := hydrateModules(root, &contract); err != nil {
		return Contract{}, err
	}
	if err := validateContract(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

// hydrateModules reads every pinned module and refuses one whose bytes moved.
func hydrateModules(root string, contract *Contract) error {
	for bundleIndex := range contract.Bundles {
		bundle := &contract.Bundles[bundleIndex]
		for moduleIndex := range bundle.Modules {
			module := &bundle.Modules[moduleIndex]
			if module.Source == "" || strings.Contains(module.Source, "..") ||
				strings.HasPrefix(module.Source, "/") || strings.Contains(module.Source, `\`) {
				return fmt.Errorf(
					"runtime ABI corpus bundle %q module %q source must be a corpus-relative path",
					bundle.Name, module.Name)
			}
			bytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(module.Source)))
			if err != nil {
				return err
			}
			sum := sha256.Sum256(bytes)
			if "sha256:"+hex.EncodeToString(sum[:]) != module.SHA256 {
				return fmt.Errorf(
					"runtime ABI corpus module %s digest drifted; the corpus pins the exact bytes a runtime is handed",
					module.Source)
			}
			module.bytes = bytes
		}
	}
	return nil
}

func validateContract(contract Contract) error {
	if contract.Format != ContractFormat {
		return errors.New("runtime ABI conformance contract identity is invalid")
	}
	if contract.Interface.APIVersion != InterfaceAPIVersion ||
		contract.Interface.Name != InterfaceName ||
		contract.Interface.Version != InterfaceVersion ||
		!digestPattern.MatchString(contract.Interface.SchemaDigest) {
		return errors.New("runtime ABI conformance contract measures the wrong Interface identity")
	}
	if contract.ServiceInterface.APIVersion != InterfaceAPIVersion ||
		contract.ServiceInterface.Name != ServiceInterfaceName ||
		contract.ServiceInterface.Version != ServiceInterfaceVersion ||
		!digestPattern.MatchString(contract.ServiceInterface.SchemaDigest) {
		return errors.New(
			"runtime ABI conformance contract measures the wrong worker-to-worker Interface identity")
	}
	if !reflect.DeepEqual(contract.HandlerVocabulary, workerbundle.HandlerVocabulary) {
		return errors.New("runtime ABI handler vocabulary drifted from the ABI's declaredHandlers enum")
	}
	if !reflect.DeepEqual(contract.LoadableMediaTypes, workerbundle.LoadableMediaTypes) {
		return errors.New("runtime ABI loadable media types drifted from the ABI's closed set")
	}
	if err := validateGlobalsFloor(contract.GlobalsFloor); err != nil {
		return err
	}
	if contract.ProbeProtocol.Protocol != fakeruntime.ProbeProtocol ||
		contract.ProbeProtocol.RoutePrefix == "" ||
		contract.ProbeProtocol.Note == "" {
		return errors.New("runtime ABI probe protocol identity is invalid")
	}
	if contract.LoaderProtocol.Protocol != fakeruntime.LoaderProtocol ||
		contract.LoaderProtocol.Method != "POST" || contract.LoaderProtocol.Note == "" {
		return errors.New("runtime ABI loader protocol identity is invalid")
	}
	if err := validateRunCorrelation(contract.RunCorrelation); err != nil {
		return err
	}
	if err := validateBundles(contract); err != nil {
		return err
	}
	if err := validateDeployment(contract); err != nil {
		return err
	}
	return validateChecks(contract)
}

// Run token bounds. A token short enough to repeat across runs correlates
// nothing; one long enough to push a resolved template past the portable nonce
// shape would not survive a binding's own key rules.
const (
	minRunTokenBytes = 8
	maxRunTokenBytes = 32
)

// validateRunCorrelation pins the mechanism itself. The placeholder must be
// something no resolved nonce could be, so a template can never be mistaken for
// a constant, and the token must be long enough that two runs cannot share one.
func validateRunCorrelation(correlation RunCorrelation) error {
	if correlation.Placeholder == "" || correlation.Note == "" {
		return errors.New(
			"runtime ABI corpus must state its run-correlation placeholder and why a correlated check cannot pin a constant")
	}
	if noncePattern.MatchString(correlation.Placeholder) {
		return errors.New(
			"runtime ABI run-correlation placeholder must not itself read as a portable nonce; " +
				"a template indistinguishable from a constant would substitute nothing and say nothing")
	}
	if correlation.TokenBytes < minRunTokenBytes || correlation.TokenBytes > maxRunTokenBytes {
		return fmt.Errorf(
			"runtime ABI run token must be %d..%d bytes; %d does not distinguish one run from another",
			minRunTokenBytes, maxRunTokenBytes, correlation.TokenBytes)
	}
	return nil
}

// sampleRunToken is the canonical token shape a minted one takes: the corpus is
// validated against it, so a template that cannot resolve to a portable nonce
// is refused at load time rather than at the first run that mints a real token.
func sampleRunToken(correlation RunCorrelation) string {
	return strings.Repeat("0", correlation.TokenBytes*2)
}

func validateGlobalsFloor(floor []string) error {
	if len(floor) == 0 {
		return errors.New("runtime ABI globals floor must not be empty")
	}
	sorted := append([]string(nil), floor...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, floor) {
		return errors.New("runtime ABI globals floor must be sorted; the probe answers in sorted order")
	}
	for index := 1; index < len(floor); index++ {
		if floor[index] == floor[index-1] {
			return fmt.Errorf("runtime ABI globals floor repeats %q", floor[index])
		}
	}
	return nil
}

// validateBundles enforces the honesty rule the host corpus learned: a case
// that declares a handler its module does not export is a case no conforming
// runtime can pass. Every bundle's stated outcome is recomputed from its own
// bytes, so a corpus cannot state one a loader will never reach.
func validateBundles(contract Contract) error {
	if len(contract.Bundles) == 0 {
		return errors.New("runtime ABI corpus carries no bundles")
	}
	seen := map[string]bool{}
	for _, bundle := range contract.Bundles {
		if bundle.Name == "" || seen[bundle.Name] {
			return fmt.Errorf("runtime ABI corpus bundle name %q is empty or repeated", bundle.Name)
		}
		seen[bundle.Name] = true
		if len(bundle.Modules) == 0 {
			return fmt.Errorf("runtime ABI corpus bundle %q carries no modules", bundle.Name)
		}
		loadable := bundle.UnloadableError == ""
		if loadable != (bundle.ExportedHandlers != nil) {
			return fmt.Errorf(
				"runtime ABI corpus bundle %q must state exactly one of exportedHandlers and unloadableError",
				bundle.Name)
		}
		request := loadRequestFor(contract, bundle, nil)
		exported, failure := fakeruntime.LoadModule(request)
		if loadable {
			if failure != nil {
				return fmt.Errorf(
					"runtime ABI corpus bundle %q states an export set but its bytes fail to load (%s)",
					bundle.Name, failure.Code)
			}
			if !reflect.DeepEqual(exported, bundle.ExportedHandlers) {
				return fmt.Errorf(
					"runtime ABI corpus bundle %q declares exports %v; its bytes export %v",
					bundle.Name, bundle.ExportedHandlers, exported)
			}
			continue
		}
		if failure == nil {
			return fmt.Errorf(
				"runtime ABI corpus bundle %q states %s but its bytes load successfully",
				bundle.Name, bundle.UnloadableError)
		}
		if failure.Code != bundle.UnloadableError {
			return fmt.Errorf(
				"runtime ABI corpus bundle %q states %s; its bytes fail %s",
				bundle.Name, bundle.UnloadableError, failure.Code)
		}
	}
	return nil
}

func validateDeployment(contract Contract) error {
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		return err
	}
	names, err := deployment.EnvironmentPropertyNames()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(names, contract.Deployment.EnvironmentPropertyNames) {
		return fmt.Errorf(
			"runtime ABI deployment environmentPropertyNames = %v; the declaration projects %v",
			contract.Deployment.EnvironmentPropertyNames, names)
	}
	vocabulary := map[string]bool{}
	for _, handler := range workerbundle.HandlerVocabulary {
		vocabulary[handler] = true
	}
	for _, handler := range contract.Deployment.DeclaredHandlers {
		if !vocabulary[handler] {
			return fmt.Errorf("runtime ABI deployment declares handler %q outside the ABI vocabulary", handler)
		}
	}
	bundle, _ := contract.Bundle(contract.Deployment.Bundle)
	exported := map[string]bool{}
	for _, handler := range bundle.ExportedHandlers {
		exported[handler] = true
	}
	for _, handler := range contract.Deployment.DeclaredHandlers {
		if !exported[handler] {
			return fmt.Errorf(
				"runtime ABI deployment declares %q, which bundle %q does not export; no conforming runtime could accept it",
				handler, bundle.Name)
		}
	}
	for _, required := range []struct {
		name     string
		contract string
	}{
		{contract.ProbeProtocol.KVBinding, "edge.kv"},
		{contract.ProbeProtocol.QueueBinding, "edge.queue"},
		{contract.ProbeProtocol.ServiceBinding, ServiceInterfaceName},
	} {
		binding, ok := deployment.BindingNamed(required.name)
		if !ok || binding.Interface != required.contract {
			return fmt.Errorf(
				"runtime ABI deployment must declare binding %q of %s; the probe module calls it",
				required.name, required.contract)
		}
	}
	if contract.Deployment.Cron == "" || contract.Deployment.Queue == "" {
		return errors.New("runtime ABI deployment must state the cron expression and queue its attachments carry")
	}
	return validatePeer(contract)
}

// validatePeer holds the second worker to the same honesty rules as the first.
//
// A `worker.service` binding addresses ANOTHER Module Worker, so the corpus
// must say which one an operator deploys; a binding with no stated callee is a
// deployment nobody can reproduce, and a stated callee no binding addresses is
// a worker nobody would deploy. The peer must also export what it declares, or
// the run would ask a conforming host to accept a version it has to refuse.
func validatePeer(contract Contract) error {
	peer := contract.Deployment.Peer
	if peer == nil {
		return errors.New(
			"runtime ABI deployment declares a worker.service binding and no peer; " +
				"the corpus must state the SECOND worker an operator deploys as its callee")
	}
	if peer.Note == "" {
		return errors.New("runtime ABI peer deployment must say what it is for")
	}
	bundle, ok := contract.Bundle(peer.Bundle)
	if !ok {
		return fmt.Errorf("runtime ABI peer deployment names unknown bundle %q", peer.Bundle)
	}
	exported := map[string]bool{}
	for _, handler := range bundle.ExportedHandlers {
		exported[handler] = true
	}
	if len(peer.DeclaredHandlers) == 0 {
		return errors.New("runtime ABI peer deployment must declare the handlers it serves")
	}
	for _, handler := range peer.DeclaredHandlers {
		if !exported[handler] {
			return fmt.Errorf(
				"runtime ABI peer declares %q, which bundle %q does not export; no conforming runtime could accept it",
				handler, bundle.Name)
		}
	}
	if !exported["fetch"] {
		return fmt.Errorf(
			"runtime ABI peer bundle %q exports no fetch handler; a worker.service callee is invoked through fetch",
			bundle.Name)
	}
	return nil
}

func validateChecks(contract Contract) error {
	if !reflect.DeepEqual(contract.RequiredChecks, requiredChecks) {
		return errors.New("runtime ABI required check list drifted")
	}
	if len(contract.Checks) != len(requiredChecks) {
		return fmt.Errorf("runtime ABI corpus declares %d checks for %d required names",
			len(contract.Checks), len(requiredChecks))
	}
	templates := map[string]string{}
	for index, check := range contract.Checks {
		if check.Name != requiredChecks[index] {
			return fmt.Errorf("runtime ABI check %d is %q, want %q", index, check.Name, requiredChecks[index])
		}
		if check.Proves == "" || check.Operation == "" {
			return fmt.Errorf("runtime ABI check %q must state the operation it exercises and what it proves", check.Name)
		}
		if err := validateCheckShape(contract, check); err != nil {
			return err
		}
		if check.Payload == nil {
			continue
		}
		if owner, seen := templates[check.Payload.NonceTemplate]; seen {
			return fmt.Errorf(
				"runtime ABI checks %q and %q share the nonce template %q; two checks reading one observation are one check",
				owner, check.Name, check.Payload.NonceTemplate)
		}
		templates[check.Payload.NonceTemplate] = check.Name
	}
	return validateEveryDeclaredHandlerIsMeasured(contract)
}

// validateEveryDeclaredHandlerIsMeasured enforces the property the ABI now has
// rather than describing it: every handler the contract DECLARES is exercised
// by a check that can fail.
//
// The corpus previously carried `tail` as an explicitly unmeasured entry, which
// was the honest thing to do about a handler nothing could invoke but was still
// a statement rather than a mechanism — the next handler added without a check
// would have been caught by review or not at all. Removing `tail` made "every
// declared handler is measured" true; this gate is what keeps it true. It is
// deliberately blind to which handler: it reads the vocabulary the loader
// already holds to the ABI's own `declaredHandlers` enum, so widening that enum
// without measuring the addition fails the corpus by name.
//
// An `unmeasured` check does not count. That is the whole point: the corpus can
// still record an unmeasurable surface — the mechanism decision 0023 built is
// intact and the load lane still reports itself unmeasured when no adapter is
// supplied — but it can no longer discharge a HANDLER that way.
func validateEveryDeclaredHandlerIsMeasured(contract Contract) error {
	measured := map[string]bool{}
	for _, check := range contract.Checks {
		if check.Procedure == ProcedureUnmeasured {
			continue
		}
		measured[check.Operation] = true
	}
	for _, handler := range contract.HandlerVocabulary {
		if measured[handler] {
			continue
		}
		return fmt.Errorf(
			"runtime ABI corpus declares handler %q and no check measures it; "+
				"a handler in a published-shaped contract that nothing reaches is a divergence between two hosts "+
				"nothing can detect, so either measure it or remove it from the ABI",
			handler)
	}
	return nil
}

// validateNonceTemplate enforces what makes an observation a run's own: a
// correlated check states a TEMPLATE carrying the run-correlation placeholder
// exactly once, and the value it resolves to is a portable nonce. A constant
// here is the defect this corpus refuses everywhere else — against a deployment
// whose `edge.kv` namespace persists, the previous run's observation answers
// the check, so no incorrect runtime can fail it.
func validateNonceTemplate(contract Contract, check Check) error {
	template := check.Payload.NonceTemplate
	if strings.Count(template, contract.RunCorrelation.Placeholder) != 1 {
		return fmt.Errorf(
			"runtime ABI check %q must carry the run-correlation placeholder %s exactly once in its nonce template; "+
				"a pinned constant reads back the observation a previous run stored",
			check.Name, contract.RunCorrelation.Placeholder)
	}
	resolved := contract.RunCorrelation.Resolve(template, sampleRunToken(contract.RunCorrelation))
	if !noncePattern.MatchString(resolved) {
		return fmt.Errorf(
			"runtime ABI check %q resolves its nonce template to %q, which is not a portable nonce",
			check.Name, resolved)
	}
	return nil
}

func validateCheckShape(contract Contract, check Check) error {
	switch check.Procedure {
	case ProcedureLoad:
		return validateLoadCheck(contract, check)
	case ProcedureRequest:
		if check.Request == nil || check.Expect == nil || check.Expect.Status == 0 {
			return fmt.Errorf("runtime ABI check %q must carry a request and an expected status", check.Name)
		}
	case ProcedureEnvironment, ProcedureGlobals:
		if check.Request == nil || check.Expect == nil || check.Expect.Status == 0 {
			return fmt.Errorf("runtime ABI check %q must carry a request and an expected status", check.Name)
		}
		if check.Expect.JSON != nil {
			return fmt.Errorf(
				"runtime ABI check %q must not restate its expected observation; it is derived from the contract",
				check.Name)
		}
	case ProcedureKVRoundTrip, ProcedureQueueRoundTrip:
		if check.Request == nil || check.Payload == nil || check.Timing == nil ||
			check.Timing.DeadlineSeconds <= 0 || check.Timing.PollMillis <= 0 {
			return fmt.Errorf("runtime ABI check %q must carry a request, a payload and a poll deadline", check.Name)
		}
		if err := validateNonceTemplate(contract, check); err != nil {
			return err
		}
		if check.Payload.Bytes == "" {
			return fmt.Errorf(
				"runtime ABI check %q must pin the exact bytes it round trips; the byte fidelity is the claim", check.Name)
		}
	case ProcedureRequestStream:
		if check.Request == nil || check.Timing == nil || len(check.Timing.RequestChunkSize) < 2 ||
			check.Timing.GapMillis <= 0 || check.Timing.DeadlineSeconds <= 0 {
			return fmt.Errorf(
				"runtime ABI check %q must send at least two request chunks separated by a gap", check.Name)
		}
	case ProcedureResponseStream:
		if check.Request == nil || check.Timing == nil || check.Timing.Chunks < 2 ||
			check.Timing.GapMillis <= 0 || check.Timing.DeadlineSeconds <= 0 {
			return fmt.Errorf(
				"runtime ABI check %q must read at least two response chunks separated by a gap", check.Name)
		}
	case ProcedureWaitUntil:
		if check.Request == nil || check.Expect == nil || check.Payload == nil || check.Timing == nil ||
			check.Timing.DelayMillis <= 0 || check.Timing.DeadlineSeconds <= 0 || check.Timing.PollMillis <= 0 {
			return fmt.Errorf(
				"runtime ABI check %q must register deferred work with a delay and a settle deadline", check.Name)
		}
		if err := validateNonceTemplate(contract, check); err != nil {
			return err
		}
	case ProcedureScheduledObservation:
		if check.Request == nil || check.Timing == nil ||
			check.Timing.DeadlineSeconds <= 0 || check.Timing.PollMillis <= 0 {
			return fmt.Errorf("runtime ABI check %q must poll for a host-driven invocation", check.Name)
		}
	case ProcedureUnmeasured:
		if check.Unmeasured == nil || check.Unmeasured.Reason == "" ||
			check.Unmeasured.ClosedBy == "" || check.Unmeasured.BlockerID == "" {
			return fmt.Errorf(
				"runtime ABI check %q is unmeasured and must say why, what would close it, and which blocker records it",
				check.Name)
		}
	default:
		return fmt.Errorf("runtime ABI check %q names unknown procedure %q", check.Name, check.Procedure)
	}
	if check.Procedure != ProcedureLoad && check.Procedure != ProcedureUnmeasured &&
		!strings.HasPrefix(check.Request.Path, contract.ProbeProtocol.RoutePrefix) {
		return fmt.Errorf("runtime ABI check %q must address the probe route prefix", check.Name)
	}
	return nil
}

func validateLoadCheck(contract Contract, check Check) error {
	if check.Load == nil || check.Bundle == "" {
		return fmt.Errorf("runtime ABI load check %q must name a bundle and a declaration", check.Name)
	}
	bundle, ok := contract.Bundle(check.Bundle)
	if !ok {
		return fmt.Errorf("runtime ABI load check %q names unknown bundle %q", check.Name, check.Bundle)
	}
	if (check.Load.ExpectError == "") == (check.Load.ExpectExportedHandlers == nil) {
		return fmt.Errorf(
			"runtime ABI load check %q must expect exactly one of an export set and a closed error code", check.Name)
	}
	exported, failure := fakeruntime.LoadModule(loadRequestFor(contract, bundle, check.Load.DeclaredHandlers))
	if check.Load.ExpectError != "" {
		if failure == nil {
			return fmt.Errorf(
				"runtime ABI load check %q expects %s, but bundle %q loads: a check no runtime can fail proves nothing",
				check.Name, check.Load.ExpectError, bundle.Name)
		}
		if failure.Code != check.Load.ExpectError {
			return fmt.Errorf(
				"runtime ABI load check %q expects %s; bundle %q fails %s",
				check.Name, check.Load.ExpectError, bundle.Name, failure.Code)
		}
		return nil
	}
	if failure != nil {
		return fmt.Errorf(
			"runtime ABI load check %q expects an export set, but bundle %q fails %s: no conforming runtime could pass it",
			check.Name, bundle.Name, failure.Code)
	}
	if !reflect.DeepEqual(exported, check.Load.ExpectExportedHandlers) {
		return fmt.Errorf(
			"runtime ABI load check %q expects exports %v; bundle %q exports %v",
			check.Name, check.Load.ExpectExportedHandlers, bundle.Name, exported)
	}
	return nil
}

// loadRequestFor builds the wire body a load check sends, from the bundle's
// verified bytes.
func loadRequestFor(contract Contract, bundle BundleContract, declared []string) fakeruntime.LoadRequest {
	modules := make([]fakeruntime.LoadRequestModule, 0, len(bundle.Modules))
	for _, module := range bundle.Modules {
		modules = append(modules, fakeruntime.LoadRequestModule{
			Name:          module.Name,
			MediaType:     module.MediaType,
			ContentBase64: encodeBase64(module.bytes),
		})
	}
	return fakeruntime.LoadRequest{
		Protocol:         contract.LoaderProtocol.Protocol,
		MainModule:       bundle.MainModule,
		DeclaredHandlers: declared,
		Modules:          modules,
	}
}
