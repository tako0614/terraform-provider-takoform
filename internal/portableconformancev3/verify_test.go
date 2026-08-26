package portableconformancev3

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

func corpusRoot(t *testing.T) string {
	t.Helper()
	// The Edge-shaped complete Host matrix is retained as a concrete Host/family
	// adapter. The stable generic suite is deliberately Snapshot-backed and
	// family-neutral; tests that need the 125-check Edge topology must not make
	// that topology a generic-suite input again.
	root, err := filepath.Abs(filepath.Join(
		"..", "..", "conformance", "takoform-v1", "family-host", "edge", "portable-host",
	))
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
	// The corpus states which lane it is, and the lane states its own
	// identity. Comparing against package constants would pin this test to
	// whichever lane those constants happened to name.
	lane := contract.lane
	if contract.Format != lane.ContractFormat || contract.APIVersion != lane.APIVersion ||
		contract.DiscoveryPath != lane.DiscoveryPath || contract.APIPath != lane.APIPath {
		t.Fatalf("contract identity drifted from its declared lane: %s", contract.APIVersion)
	}
	if len(contract.ErrorEnvelope.Codes) != len(lane.ErrorHTTPStatus) {
		t.Fatalf(
			"error taxonomy carries %d codes, want the lane's %d",
			len(contract.ErrorEnvelope.Codes), len(lane.ErrorHTTPStatus),
		)
	}
	if len(contract.ErrorEnvelope.AutomaticallyRetryable) != 4 {
		t.Fatalf("retryable set carries %d codes, want 4", len(contract.ErrorEnvelope.AutomaticallyRetryable))
	}
	// The lane's checks PLUS the ones this corpus's family generation adds:
	// comparing against the lane alone would say a lane can only ever be
	// driven against one family (decision 0047).
	wantChecks := requiredChecksFor(lane, contract.RunnerInput)
	if len(contract.RequiredRunnerChecks) != len(wantChecks) {
		t.Fatalf("required checks = %d, want %d", len(contract.RequiredRunnerChecks), len(wantChecks))
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
//
// It reads probeInventory rather than a list of its own. The list of its own is
// what this test used to be, it omitted WorkerCustomDomain and WorkerEndpoint,
// and the corpus's WorkerEndpoint packageDigest drifted from the registry with
// nothing able to notice — the same "enumerated by hand, complete until someone
// forgets" defect the tenant matrix already had to fix once.
func TestVerifyPinsRegistryIdentities(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	inventory := probeInventory(&contract.RunnerInput)
	if len(inventory) != 16 {
		t.Fatalf("the corpus pins %d resource probes; the Edge Family lane drives sixteen", len(inventory))
	}
	for _, entry := range inventory {
		probe := entry.Probe
		registered, err := currentformregistry.V3ForKind(probe.Identity.FormRef.APIVersion, probe.Identity.FormRef.Kind)
		if err != nil {
			t.Fatalf("registry lookup %s: %v", probe.Identity.FormRef.Kind, err)
		}
		if registered.SchemaDigest != probe.Identity.FormRef.SchemaDigest ||
			registered.PackageDigest != probe.Identity.PackageDigest ||
			registered.DefinitionVersion != probe.Identity.FormRef.DefinitionVersion {
			t.Fatalf(
				"probe %s drifted from the generated registry: corpus %s/%s/%s, registry %s/%s/%s",
				probe.Identity.FormRef.Kind,
				probe.Identity.FormRef.DefinitionVersion, probe.Identity.FormRef.SchemaDigest,
				probe.Identity.PackageDigest,
				registered.DefinitionVersion, registered.SchemaDigest, registered.PackageDigest,
			)
		}
	}
}

// copyCorpus reproduces the whole corpus directory, so a tamper test measures
// the corpus as shipped. It walks rather than listing: a hand-written list would
// silently stop copying whatever was added next, and the digest tests would then
// pass on a corpus missing the file they were meant to protect.
func copyCorpus(t *testing.T, destination string) {
	t.Helper()
	source := corpusRoot(t)
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	}); err != nil {
		t.Fatal(err)
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
		"format":   stableLane.ManifestFormat,
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
					exported = []byte(`["fetch","scheduled","queue"]`)
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
		"format":   stableLane.ManifestFormat,
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
