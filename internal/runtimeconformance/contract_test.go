package runtimeconformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyCorpus reproduces the committed corpus in a temporary directory so a
// test can state what a corpus must NOT be able to say.
func copyCorpus(t *testing.T) string {
	t.Helper()
	source := corpusRoot(t)
	target := t.TempDir()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, raw, 0o644)
	})
	if err != nil {
		t.Fatalf("copy corpus: %v", err)
	}
	return target
}

// mutateCorpus rewrites the contract of a copied corpus and re-pins its
// manifest, so what the test proves is the contract rule rather than the
// digest pin.
func mutateCorpus(t *testing.T, root string, mutate func(document map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "contract.json"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	mutate(document)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode contract: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "contract.json"), encoded, 0o644); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	digest := sha256.Sum256(encoded)
	manifest, err := json.Marshal(map[string]string{
		"format": ManifestFormat, "contract": "contract.json",
		"sha256": hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func bundleNamed(document map[string]any, name string) map[string]any {
	for _, entry := range document["bundles"].([]any) {
		bundle := entry.(map[string]any)
		if bundle["name"] == name {
			return bundle
		}
	}
	return nil
}

func checkNamed(document map[string]any, name string) map[string]any {
	for _, entry := range document["checks"].([]any) {
		check := entry.(map[string]any)
		if check["name"] == name {
			return check
		}
	}
	return nil
}

// TestVerifyPinsTheCommittedCorpus states what a verified contract is.
func TestVerifyPinsTheCommittedCorpus(t *testing.T) {
	contract := verifiedContract(t)
	if contract.Format != ContractFormat {
		t.Fatalf("format = %q", contract.Format)
	}
	if contract.Interface.Name != InterfaceName || contract.Interface.Version != InterfaceVersion {
		t.Fatalf("the corpus measures %+v", contract.Interface)
	}
	if !strings.HasPrefix(contract.Digest(), "sha256:") {
		t.Fatalf("digest = %q", contract.Digest())
	}
	if len(contract.Checks) != len(requiredChecks) {
		t.Fatalf("the corpus declares %d checks", len(contract.Checks))
	}
}

// TestContractBytesArePinned proves the manifest is a pin and not a comment.
func TestContractBytesArePinned(t *testing.T) {
	root := copyCorpus(t)
	path := filepath.Join(root, "contract.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := os.WriteFile(path, append(raw, ' '), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "digest drifted") {
		t.Fatalf("expected a digest refusal, got %v", err)
	}
}

// TestModuleBytesArePinned proves the corpus pins the exact bytes a runtime is
// handed, not only the name of a file.
func TestModuleBytesArePinned(t *testing.T) {
	root := copyCorpus(t)
	path := filepath.Join(root, "bundles", "fetch-only", "fetch-only.js")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "digest drifted") {
		t.Fatalf("expected a module digest refusal, got %v", err)
	}
}

// TestABundleCannotClaimAnExportItsBytesDoNotHave is the honesty rule the host
// corpus learned, enforced here: a case that declares a handler its module
// does not export is a case no conforming runtime can pass.
func TestABundleCannotClaimAnExportItsBytesDoNotHave(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		bundleNamed(document, "fetch-only")["exportedHandlers"] = []any{"fetch", "scheduled"}
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "its bytes export") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestALoadCheckCannotExpectAFailureThatCannotHappen refuses the mirror image:
// a required check no correct runtime can fail proves nothing.
func TestALoadCheckCannotExpectAFailureThatCannotHappen(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		check := checkNamed(document, "declared-handler-not-exported-refused")
		check["bundle"] = "conformance-probe"
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "a check no runtime can fail proves nothing") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestABundleCannotStateAFailureItsBytesDoNotProduce covers the load-error
// half of the same rule.
func TestABundleCannotStateAFailureItsBytesDoNotProduce(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		bundleNamed(document, "unparseable")["unloadableError"] = "unsupported_media_type"
		checkNamed(document, "unparseable-module-refused")["load"] = map[string]any{
			"declaredHandlers": []any{"fetch"},
			"expectError":      "unsupported_media_type",
		}
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "its bytes fail module_syntax_error") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestTheEnvironmentExpectationIsDerivedNotRestated proves the `env` check
// cannot drift from the deployment an operator reproduces.
func TestTheEnvironmentExpectationIsDerivedNotRestated(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		deployment := document["deployment"].(map[string]any)
		deployment["environmentPropertyNames"] = []any{"CACHE", "EVENTS", "LOG_LEVEL"}
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "the declaration projects") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestTheDeploymentCannotDeclareAnUnexportedHandler refuses a deployment no
// conforming runtime could accept in the first place.
func TestTheDeploymentCannotDeclareAnUnexportedHandler(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		document["deployment"].(map[string]any)["bundle"] = "fetch-only"
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "does not export") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestTheCheckListIsClosed proves a corpus cannot quietly drop a check.
func TestTheCheckListIsClosed(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		checks := document["checks"].([]any)
		document["checks"] = checks[:len(checks)-1]
		required := document["requiredChecks"].([]any)
		document["requiredChecks"] = required[:len(required)-1]
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "required check list drifted") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestAnUnmeasuredCheckMustAccountForItself refuses the shape this whole
// corpus exists to avoid: a declared surface nothing checks and nothing
// explains.
func TestAnUnmeasuredCheckMustAccountForItself(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		check := checkNamed(document, "tail-trace-delivered-to-the-tail-handler")
		delete(check, "unmeasured")
	})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "must say why") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}
