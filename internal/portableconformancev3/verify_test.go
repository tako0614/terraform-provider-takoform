package portableconformancev3

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

func corpusRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "conformance", "portable-host-v3"))
	if err != nil {
		t.Fatalf("resolve corpus root: %v", err)
	}
	return root
}

func TestVerifyCorpus(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if contract.Format != ContractFormat || contract.APIVersion != APIVersion ||
		contract.DiscoveryPath != DiscoveryPath || contract.APIPath != APIPath {
		t.Fatalf("contract identity drifted: %+v", contract)
	}
	if len(contract.ErrorEnvelope.Codes) != 26 {
		t.Fatalf("error taxonomy carries %d codes, want 26", len(contract.ErrorEnvelope.Codes))
	}
	if len(contract.ErrorEnvelope.AutomaticallyRetryable) != 4 {
		t.Fatalf("retryable set carries %d codes, want 4", len(contract.ErrorEnvelope.AutomaticallyRetryable))
	}
	if len(contract.RequiredRunnerChecks) != len(requiredRunnerChecks) {
		t.Fatalf("required checks = %d, want %d", len(contract.RequiredRunnerChecks), len(requiredRunnerChecks))
	}
	if contract.RunnerInput.Space == contract.RunnerInput.AlternateSpace {
		t.Fatalf("runner spaces must differ")
	}
	for _, fixture := range contract.RunnerInput.NegativeFixtures {
		if len(fixture.Input) == 0 {
			t.Fatalf("fixture %q was not hydrated", fixture.Name)
		}
	}
}

// TestVerifyPinsRegistryIdentities proves the contract's probe FormRefs are
// the exact generated Edge Family registry identities, byte for byte.
func TestVerifyPinsRegistryIdentities(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	probes := []ResourceProbe{
		contract.RunnerInput.ModuleWorker,
		contract.RunnerInput.EdgeKvNamespace,
		contract.RunnerInput.AtLeastOnceQueue,
		contract.RunnerInput.WorkerVersion,
		contract.RunnerInput.WorkerBundle.ResourceProbe,
		contract.RunnerInput.WorkerDeployment,
		contract.RunnerInput.WorkerCronTrigger,
		contract.RunnerInput.QueueConsumer,
	}
	for _, probe := range probes {
		registered, err := currentformregistry.V3ForKind(probe.Identity.FormRef.APIVersion, probe.Identity.FormRef.Kind)
		if err != nil {
			t.Fatalf("registry lookup %s: %v", probe.Identity.FormRef.Kind, err)
		}
		if registered.SchemaDigest != probe.Identity.FormRef.SchemaDigest ||
			registered.PackageDigest != probe.Identity.PackageDigest ||
			registered.DefinitionVersion != probe.Identity.FormRef.DefinitionVersion {
			t.Fatalf("probe %s drifted from the generated registry", probe.Identity.FormRef.Kind)
		}
	}
}

func copyCorpus(t *testing.T, destination string) {
	t.Helper()
	source := corpusRoot(t)
	if err := os.MkdirAll(filepath.Join(destination, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries := []string{
		"manifest.json",
		"contract.json",
		"fixtures/negative-module-worker-unexpected-property.json",
		"fixtures/negative-queue-retention.json",
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(entry)))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, filepath.FromSlash(entry)), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerifyRejectsTamperedContractBytes(t *testing.T) {
	root := t.TempDir()
	copyCorpus(t, root)
	contractPath := filepath.Join(root, "contract.json")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "digest drifted") {
		t.Fatalf("expected contract digest drift, got %v", err)
	}
}

func TestVerifyRejectsDriftedRequiredChecks(t *testing.T) {
	root := t.TempDir()
	copyCorpus(t, root)
	contractPath := filepath.Join(root, "contract.json")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	var checks []string
	if err := json.Unmarshal(document["requiredRunnerChecks"], &checks); err != nil {
		t.Fatal(err)
	}
	drifted, err := json.Marshal(checks[:len(checks)-1])
	if err != nil {
		t.Fatal(err)
	}
	document["requiredRunnerChecks"] = drifted
	updated, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(updated)
	manifest := map[string]string{
		"format":   ManifestFormat,
		"contract": "contract.json",
		"sha256":   hex.EncodeToString(digest[:]),
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifestRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "required runner checks drifted") {
		t.Fatalf("expected required check drift, got %v", err)
	}
}

func TestVerifyRejectsTamperedFixtureBytes(t *testing.T) {
	root := t.TempDir()
	copyCorpus(t, root)
	fixturePath := filepath.Join(root, "fixtures", "negative-queue-retention.json")
	if err := os.WriteFile(fixturePath, []byte("{\"messageRetentionSeconds\": 59}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "byte digest drifted") {
		t.Fatalf("expected fixture digest drift, got %v", err)
	}
}

// TestVerifyRejectsABundleThatDoesNotExportTheVocabulary is the corpus half of
// the ES Module Worker ABI's handler rule.
//
// The lane's positive control for `undeclared-runtime-handler-rejected` is a
// Worker Version declaring the WHOLE handler vocabulary, driven against the
// workerBundle probe. If that bundle's module exported less, the same contract
// that closes the handler surface would require a conforming host to refuse the
// control (`handler_not_exported`), so the required check could be completed
// only by a host that does NOT implement the contract. A required check no
// correct host can pass is worse than a missing one, so the corpus refuses to
// load rather than shipping the incoherence.
func TestVerifyRejectsABundleThatDoesNotExportTheVocabulary(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		probe string
		want  string
	}{
		{"the corpus bundle exports less than the vocabulary", "workerBundle", "does not export"},
		{"the fetch-only bundle exports all of it", "fetchOnlyBundle", "exports the whole"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			copyCorpus(t, root)
			repinCorpus(t, root, func(input map[string]json.RawMessage) {
				bundle := map[string]json.RawMessage{}
				if err := json.Unmarshal(input[testCase.probe], &bundle); err != nil {
					t.Fatal(err)
				}
				exported := []byte(`["fetch"]`)
				if testCase.probe == "fetchOnlyBundle" {
					exported = []byte(`["fetch","scheduled","queue","tail"]`)
				}
				bundle["exportedHandlers"] = exported
				drifted, err := json.Marshal(bundle)
				if err != nil {
					t.Fatal(err)
				}
				input[testCase.probe] = drifted
			})
			if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("expected %q, got %v", testCase.want, err)
			}
		})
	}
}

// repinCorpus rewrites one copied corpus's runnerInput and re-pins the manifest
// digest over the result, so a test reaches the rule it is about rather than
// the byte-digest gate in front of it.
func repinCorpus(t *testing.T, root string, mutate func(input map[string]json.RawMessage)) {
	t.Helper()
	contractPath := filepath.Join(root, "contract.json")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(document["runnerInput"], &input); err != nil {
		t.Fatal(err)
	}
	mutate(input)
	drifted, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	document["runnerInput"] = drifted
	updated, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(updated)
	manifestRaw, err := json.Marshal(map[string]string{
		"format":   ManifestFormat,
		"contract": "contract.json",
		"sha256":   hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifestRaw, 0o644); err != nil {
		t.Fatal(err)
	}
}
