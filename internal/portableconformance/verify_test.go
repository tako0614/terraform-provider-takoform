package portableconformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

func TestPublishedHostAPIKeepsInterfaceProjectionReadOnly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "host-api", "operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		OptionalFeatures  []string `json:"optionalFeatures"`
		OptionalEndpoints []string `json:"optionalEndpoints"`
		Operations        []struct {
			Name    string `json:"name"`
			Method  string `json:"method"`
			Path    string `json:"path"`
			Mutates bool   `json:"mutates"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if !contains(contract.OptionalFeatures, "interface_declarations") {
		t.Fatal("published host API does not advertise the read-only Interface projection")
	}
	if contains(contract.OptionalFeatures, "interface_declaration_writes") {
		t.Fatal("published host API still advertises portable Interface writes")
	}
	if !contains(contract.OptionalEndpoints, "interfaces") {
		t.Fatal("published host API does not advertise the optional Interface endpoint")
	}

	seen := map[string]bool{}
	for _, operation := range contract.Operations {
		if operation.Path != "/interfaces" && operation.Path != "/interfaces/{name}" {
			continue
		}
		if operation.Method != "GET" || operation.Mutates {
			t.Fatalf("portable Interface operation %s is not read-only: %s %s", operation.Name, operation.Method, operation.Path)
		}
		seen[operation.Path] = true
	}
	for _, path := range []string{"/interfaces", "/interfaces/{name}"} {
		if !seen[path] {
			t.Fatalf("published host API is missing read-only Interface path %s", path)
		}
	}
}

func TestPublishedHostAPIUsesPortableStableErrorTaxonomy(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "host-api", "operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		ErrorEnvelope struct {
			Codes                  []string `json:"codes"`
			AutomaticallyRetryable []string `json:"automaticallyRetryable"`
		} `json:"errorEnvelope"`
	}
	if err := json.Unmarshal(raw, &published); err != nil {
		t.Fatal(err)
	}
	portable, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(published.ErrorEnvelope.Codes, portable.StableErrorCodes) {
		t.Fatalf("published host API error codes = %v, want portable taxonomy %v", published.ErrorEnvelope.Codes, portable.StableErrorCodes)
	}
	if !reflect.DeepEqual(published.ErrorEnvelope.AutomaticallyRetryable, portable.RetryableCodes) {
		t.Fatalf("published host API retryable codes = %v, want portable taxonomy %v", published.ErrorEnvelope.AutomaticallyRetryable, portable.RetryableCodes)
	}
}

func TestPublishedResourceVersionContractMatchesPortableConformance(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "host-api", "operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		ResourceVersion ResourceVersionContract `json:"resourceVersion"`
	}
	if err := json.Unmarshal(raw, &published); err != nil {
		t.Fatal(err)
	}
	portable, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	if published.ResourceVersion.Encoding != portable.ResourceVersion.Encoding ||
		published.ResourceVersion.Minimum != portable.ResourceVersion.Minimum ||
		published.ResourceVersion.Maximum != portable.ResourceVersion.Maximum ||
		published.ResourceVersion.ETag != portable.ResourceVersion.ETag {
		t.Fatalf("published resourceVersion contract = %#v, portable = %#v", published.ResourceVersion, portable.ResourceVersion)
	}
}

func TestPortableHostContractMatchesCurrentEdgeWorkerCandidate(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := currentformregistry.ForKind("EdgeWorker")
	if err != nil {
		t.Fatal(err)
	}
	identity := contract.RunnerInput.Identity
	if identity.FormRef.APIVersion != release.APIVersion || identity.FormRef.Kind != release.Kind ||
		identity.FormRef.DefinitionVersion != release.DefinitionVersion || identity.FormRef.SchemaDigest != release.SchemaDigest ||
		identity.PackageDigest != release.PackageDigest {
		t.Fatalf("cross-repo runner FormRef %#v differs from provider release %#v", identity, release)
	}
	var canonicalDesired map[string]any
	if err := decodeStrict(filepath.Join("..", "..", "forms", "candidates", "v1alpha2", "edge-worker", "fixtures", "desired.json"), &canonicalDesired); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.RunnerInput.Desired, canonicalDesired) {
		t.Fatalf("portable runner desired %#v differs from EdgeWorker canonical fixture %#v", contract.RunnerInput.Desired, canonicalDesired)
	}
	if contract.RunnerInput.Name != canonicalDesired["name"] {
		t.Fatalf("portable runner name %q differs from canonical desired.name %#v", contract.RunnerInput.Name, canonicalDesired["name"])
	}
}

func TestPortableHostContractRejectsRunnerNameDrift(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	var contract Contract
	if err := decodeStrict(path, &contract); err != nil {
		t.Fatal(err)
	}
	contract.RunnerInput.Name = "different-name"
	digest, err := RunnerEvidenceDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	contract.RunnerEvidence.SHA256 = digest
	if err := validate(contract); err == nil {
		t.Fatal("portable host contract accepted a route name different from desired.name")
	}
}

func TestPortableHostContractRequiresDistinctResourceSpaces(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	contract.RunnerInput.AlternateSpace = contract.RunnerInput.Space
	digest, err := RunnerEvidenceDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	contract.RunnerEvidence.SHA256 = digest
	if err := validate(contract); err == nil {
		t.Fatal("portable host contract accepted the primary Space as alternateSpace")
	}
}

func TestPortableHostContractRejectsNonCanonicalInstalledFormReference(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	var base Contract
	if err := decodeStrict(path, &base); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*InstalledFormReference)
	}{
		{name: "lowercase kind", mutate: func(identity *InstalledFormReference) { identity.FormRef.Kind = "objectBucket" }},
		{name: "invalid SemVer", mutate: func(identity *InstalledFormReference) { identity.FormRef.DefinitionVersion = "1.0" }},
		{name: "uppercase schema digest", mutate: func(identity *InstalledFormReference) {
			identity.FormRef.SchemaDigest = "sha256:" + strings.Repeat("A", 64)
		}},
		{name: "uppercase package digest", mutate: func(identity *InstalledFormReference) {
			identity.PackageDigest = "sha256:" + strings.Repeat("B", 64)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate.RunnerInput.Identity)
			runnerDigest, err := RunnerEvidenceDigest(candidate)
			if err != nil {
				t.Fatal(err)
			}
			candidate.RunnerEvidence.SHA256 = runnerDigest
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := verifyRawContract(t, raw); err == nil {
				t.Fatalf("portable host contract accepted non-canonical FormRef: %#v", candidate.RunnerInput.Identity.FormRef)
			}
		})
	}
}

func TestPortableHostContractRejectsUnknownFormRefField(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	runnerInput := document["runnerInput"].(map[string]any)
	identity := runnerInput["identity"].(map[string]any)
	formRef := identity["formRef"].(map[string]any)
	formRef["extension"] = "must-not-be-ignored"
	raw, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyRawContract(t, raw); err == nil || !strings.Contains(err.Error(), "extension") {
		t.Fatalf("unknown nested FormRef field error = %v", err)
	}
}

func TestPortableHostContractRejectsDuplicateFormRefField(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(
		string(raw),
		`"kind": "EdgeWorker",`,
		`"kind": "edgeWorker", "kind": "EdgeWorker",`,
		1,
	)
	if duplicate == string(raw) {
		t.Fatal("portable host fixture did not contain the expected FormRef kind")
	}
	if _, err := verifyRawContract(t, []byte(duplicate)); err == nil || !strings.Contains(err.Error(), "duplicate object name") {
		t.Fatalf("duplicate nested FormRef field error = %v", err)
	}
}

func TestPortableHostContractRejectsLegacyCompatibilityPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.json")
	if err := os.WriteFile(path, []byte(`{"compatibilityPath":"/v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var contract Contract
	err := decodeStrict(path, &contract)
	if err == nil || !strings.Contains(err.Error(), `json: unknown field "compatibilityPath"`) {
		t.Fatalf("strict contract decoder error = %v, want unknown legacy compatibilityPath field", err)
	}
}

func TestPortableHostContractRejectsDuplicateMembersBeforeTypedDecode(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const format = `"format": "takoform.portable-host-conformance@v2"`
	duplicate := []byte(format + `,` + format)
	raw = bytes.Replace(raw, []byte(format), duplicate, 1)
	if bytes.Count(raw, []byte(format)) != 2 {
		t.Fatal("test did not insert a duplicate contract member")
	}
	_, err = verifyRawContract(t, raw)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("duplicate contract member error = %v, want raw duplicate rejection", err)
	}
}

func TestPortableHostContractRejectsInvalidUTF8BeforeTypedDecode(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("takoform.portable-host-conformance@v2")
	index := bytes.Index(raw, marker)
	if index < 0 {
		t.Fatal("test format marker is absent")
	}
	raw[index] = 0xff
	_, err = verifyRawContract(t, raw)
	if err == nil {
		t.Fatal("invalid UTF-8 contract passed raw validation")
	}
}

func TestNeutralRunnerEvidenceDigestCoversSubjectAndInputs(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v2", "contract.json")
	var contract Contract
	if err := decodeStrict(path, &contract); err != nil {
		t.Fatal(err)
	}
	got, err := RunnerEvidenceDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if got != contract.RunnerEvidence.SHA256 {
		t.Fatalf("runner evidence digest = %q, contract has %q", got, contract.RunnerEvidence.SHA256)
	}
	mutated := contract
	mutated.RunnerEvidence.Subject = "implementation-specific-runner"
	mutatedDigest, err := RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("runner subject substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.RunnerEvidence.Entrypoint = "cmd/checklist-only"
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("runner entrypoint substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.RunnerInput.NegativeFixtures[0].SHA256 = "sha256:" + strings.Repeat("f", 64)
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("desired negative fixture substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.InterfaceDeclarations.RuntimeIdentity = []string{"name", "version"}
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("interface identity substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.PlanBinding.PortableInputs = []string{"specDigest"}
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("plan binding substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.PlanBinding.InstrumentedAdapter.BindingPath = "ordinary-validation"
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("instrumented plan-binding path substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.Idempotency.ScopeDimensions = []string{"idempotency-key"}
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("idempotency authorization substitution did not change the evidence digest")
	}
	mutated = contract
	mutated.WireJSON.RawValidationOrder = "typed-decode-first"
	mutatedDigest, err = RunnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("raw JSON contract substitution did not change the evidence digest")
	}
}

func verifyRawContract(t *testing.T, raw []byte) (Contract, error) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "contract.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	manifestRaw := []byte(fmt.Sprintf(
		`{"format":"takoform.portable-host-conformance-manifest@v1","contract":"contract.json","sha256":"%x"}`,
		sum,
	))
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifestRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	return Verify(root)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
