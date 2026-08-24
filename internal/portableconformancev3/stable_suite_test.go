package portableconformancev3

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStableSuiteExecutesAllNestedPortableHostChecks(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := Verify(filepath.Join(
		repositoryRoot,
		"conformance", "takoform-v1", "generic-host", "portable-host",
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(contract.RequiredRunnerChecks); got != 125 {
		t.Fatalf("stable nested portable Host contract has %d checks, want 125", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := SelfTest(ctx, contract)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(report.Checks); got != 125 {
		t.Fatalf("stable nested portable Host runner executed %d checks, want 125", got)
	}
	for index, check := range report.Checks {
		if check != contract.RequiredRunnerChecks[index] {
			t.Fatalf("stable nested portable Host report check %d = %q, want %q", index, check, contract.RequiredRunnerChecks[index])
		}
	}
}

func TestStableSuiteRejectsMissingNestedPortableHostCheck(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	temporaryRoot := t.TempDir()
	if err := os.CopyFS(
		filepath.Join(temporaryRoot, "conformance", "takoform-v1"),
		os.DirFS(filepath.Join(repositoryRoot, "conformance", "takoform-v1")),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "go.mod"), []byte("module stable-suite-tamper\n\ngo 1.25.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repositoryRoot, "forms"), filepath.Join(temporaryRoot, "forms")); err != nil {
		t.Fatal(err)
	}

	contractPath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "generic-host", "portable-host", "contract.json")
	contractRaw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(contractRaw, &document); err != nil {
		t.Fatal(err)
	}
	var checks []string
	if err := json.Unmarshal(document["requiredRunnerChecks"], &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks) != 125 {
		t.Fatalf("stable nested portable Host contract has %d checks before tamper, want 125", len(checks))
	}
	omitted := checks[len(checks)-1]
	document["requiredRunnerChecks"], err = json.Marshal(checks[:len(checks)-1])
	if err != nil {
		t.Fatal(err)
	}
	contractRaw, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	contractRaw = append(contractRaw, '\n')
	if err := os.WriteFile(contractPath, contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	nestedManifestPath := filepath.Join(filepath.Dir(contractPath), "manifest.json")
	var nestedManifest manifest
	if _, err := stableReadStrict(nestedManifestPath, &nestedManifest); err != nil {
		t.Fatal(err)
	}
	nestedManifest.SHA256 = stableDigest(contractRaw)
	writeStableTestJSON(t, nestedManifestPath, nestedManifest)

	genericPath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "generic.json")
	var generic stableGenericCorpus
	if _, err := stableReadStrict(genericPath, &generic); err != nil {
		t.Fatal(err)
	}
	generic.PortableHostContract.SHA256 = "sha256:" + stableDigest(contractRaw)
	genericRaw := writeStableTestJSON(t, genericPath, generic)

	suitePath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "manifest.json")
	var suite StableSuiteManifest
	if _, err := stableReadStrict(suitePath, &suite); err != nil {
		t.Fatal(err)
	}
	suite.Generic.SHA256 = stableDigest(genericRaw)
	writeStableTestJSON(t, suitePath, suite)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := RunStableSuite(ctx, suitePath); err == nil || !strings.Contains(err.Error(), "required runner checks drifted") {
		t.Fatalf("suite accepted nested portable Host contract missing %q: %v", omitted, err)
	}
}

func writeStableTestJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return raw
}
