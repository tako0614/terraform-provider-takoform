package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestKindToSlugPreservesAcronyms(t *testing.T) {
	t.Parallel()
	for kind, want := range map[string]string{
		"ObjectBucket": "object-bucket", "SQLDatabase": "sql-database", "KVStore": "kv-store",
		"StatefulActorNamespace": "stateful-actor-namespace", "EdgeWorker": "edge-worker",
	} {
		if got := kindToSlug(kind); got != want {
			t.Fatalf("kindToSlug(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestBuildPackageIsDeterministicAndCanonical(t *testing.T) {
	repo := makeTestRepo(t)
	packageDir := filepath.Join(repo, "package")
	copyTree(t, filepath.Join(repositoryRoot(t), "conformance", "form-package-v1", "positive", "example-store"), packageDir)
	gitCommitAll(t, repo, "package")

	outputs := []string{filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")}
	for _, output := range outputs {
		if err := run([]string{
			"build-package", "--repo", repo, "--tag", "forms/example-store/v1.0.0",
			"--package-dir", packageDir, "--output", output, "--allow-untagged-candidate",
		}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	var manifest releaseManifest
	readJSON(t, filepath.Join(outputs[0], "release-manifest.json"), &manifest)
	wantIdentity := "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/tags/forms/example-store/v1.0.0"
	if manifest.PublisherPolicy.Identity != wantIdentity {
		t.Fatalf("publisher identity = %q, want %q", manifest.PublisherPolicy.Identity, wantIdentity)
	}
	for _, name := range []string{
		"takoform-form-example-store_1.0.0.tar.gz",
		"takoform-form-example-store_1.0.0_package-index.json",
		"takoform-form-example-store_1.0.0_provenance.intoto.json",
		"takoform-form-example-store_1.0.0_sbom.spdx.json",
		"release-manifest.json",
	} {
		first, err := os.ReadFile(filepath.Join(outputs[0], name))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(outputs[1], name))
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(second) {
			t.Fatalf("%s is not reproducible", name)
		}
	}

	archive, err := os.Open(filepath.Join(outputs[0], "takoform-form-example-store_1.0.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "package-index.json" || header.Mode != 0o644 || !header.ModTime.IsZero() && header.ModTime.Unix() != 0 {
		t.Fatalf("unexpected deterministic archive header: %+v", header)
	}
	indexInArchive, err := io.ReadAll(tarReader)
	if err != nil {
		t.Fatal(err)
	}
	signedIndex, err := os.ReadFile(filepath.Join(outputs[0], "takoform-form-example-store_1.0.0_package-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(indexInArchive) != string(signedIndex) || strings.Contains(string(signedIndex), "\n") {
		t.Fatal("archive index differs from the newline-free canonical signed subject")
	}

	var sbom map[string]any
	sbomPath := filepath.Join(outputs[0], "takoform-form-example-store_1.0.0_sbom.spdx.json")
	readJSON(t, sbomPath, &sbom)
	if sbom["spdxVersion"] != "SPDX-2.3" {
		t.Fatalf("unexpected SBOM: %+v", sbom)
	}
	validateSPDX(t, sbomPath)
	var provenance statement
	readJSON(t, filepath.Join(outputs[0], "takoform-form-example-store_1.0.0_provenance.intoto.json"), &provenance)
	if provenance.Type != "https://in-toto.io/Statement/v1" || provenance.PredicateType != "https://slsa.dev/provenance/v1" || len(provenance.Subject) != 2 {
		t.Fatalf("unexpected provenance: %+v", provenance)
	}
}

func TestFinalizeRequiresTransparencyEvidence(t *testing.T) {
	repo := makeTestRepo(t)
	packageDir := filepath.Join(repo, "package")
	copyTree(t, filepath.Join(repositoryRoot(t), "conformance", "form-package-v1", "positive", "example-store"), packageDir)
	gitCommitAll(t, repo, "package")
	output := filepath.Join(t.TempDir(), "release")
	if err := run([]string{
		"build-package", "--repo", repo, "--tag", "forms/example-store/v1.0.0",
		"--package-dir", packageDir, "--output", output, "--allow-untagged-candidate",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	bundleName := "takoform-form-example-store_1.0.0_package-index.sigstore.json"
	invalid := `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{"tlogEntries":[]},"content":{"messageSignature":{}}}`
	if err := os.WriteFile(filepath.Join(output, bundleName), []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"finalize-bundle", "--output", output}, io.Discard); err == nil || !strings.Contains(err.Error(), "transparency-log") {
		t.Fatalf("expected missing transparency proof rejection, got %v", err)
	}
	valid := `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{"certificate":{"rawBytes":"AA=="},"tlogEntries":[{"logIndex":"1"}]},"content":{"messageSignature":{"messageDigest":{"algorithm":"SHA2_256","digest":"AA=="},"signature":"AA=="}}}`
	if err := os.WriteFile(filepath.Join(output, bundleName), []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"finalize-bundle", "--output", output}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var manifest releaseManifest
	readJSON(t, filepath.Join(output, "release-manifest.json"), &manifest)
	if manifest.PublicationReady || len(manifest.PublicationBlockers) == 0 {
		t.Fatalf("untagged candidate became publishable: %+v", manifest)
	}
	checksums, err := os.ReadFile(filepath.Join(output, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(checksums), bundleName) || !strings.Contains(string(checksums), "release-manifest.json") {
		t.Fatalf("final inventory is incomplete:\n%s", checksums)
	}
}

func TestBuildRevocationAndAppendOnlyGuard(t *testing.T) {
	repo := makeTestRepo(t)
	revocationDir := filepath.Join(repo, "forms", "revocations")
	if err := os.MkdirAll(revocationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	statementPath := filepath.Join(revocationDir, "1.0.0.json")
	statement := `{"apiVersion":"trust.forms.takoform.com/v1alpha1","kind":"FormPackageRevocation","statementVersion":"1.0.0","packageDigest":"` + digest + `","formRef":{"apiVersion":"forms.takoform.com/v1alpha1","kind":"ObjectBucket","definitionVersion":"1.0.0","schemaDigest":"` + digest + `"},"reasonCode":"signature-invalid","summary":"invalid retained signature","issuedAt":"2026-07-17T00:00:00Z","effects":{"blockNewCreateOrUpdate":true,"blockActivation":true,"retainBytesForObserveAndDelete":true}}`
	if err := os.WriteFile(statementPath, []byte(statement), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "revocation")
	base := strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	output := filepath.Join(t.TempDir(), "release")
	if err := run([]string{
		"build-revocation", "--repo", repo, "--tag", "forms/revocations/v1.0.0",
		"--statement", statementPath, "--output", output, "--allow-untagged-candidate",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var manifest releaseManifest
	readJSON(t, filepath.Join(output, "release-manifest.json"), &manifest)
	if manifest.ReleaseType != "form-package-revocation" || manifest.PackageDigest != digest || manifest.Workflow != revokeWorkflow {
		t.Fatalf("unexpected revocation manifest: %+v", manifest)
	}
	if err := os.WriteFile(statementPath, []byte(strings.Replace(statement, "invalid retained signature", "changed", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "rewrite")
	if err := run([]string{"check-revocations", "--repo", repo, "--base", base}, io.Discard); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only rejection, got %v", err)
	}
}

func makeTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runCommand(t, repo, "git", "init", "-q")
	runCommand(t, repo, "git", "config", "user.email", "test@example.com")
	runCommand(t, repo, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "initial")
	return repo
}

func gitCommitAll(t *testing.T, repo, message string) {
	t.Helper()
	runCommand(t, repo, "git", "add", ".")
	runCommand(t, repo, "git", "commit", "-q", "-m", message)
}

func runCommand(t *testing.T, dir, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %s: %v", name, strings.Join(arguments, " "), output, err)
	}
	return string(output)
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, destination any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		t.Fatal(err)
	}
}

func validateSPDX(t *testing.T, documentPath string) {
	t.Helper()
	schemaPath := filepath.Join(repositoryRoot(t), "release", "schemas", "spdx-2.3.schema.json")
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaRaw, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const schemaURL = "https://raw.githubusercontent.com/spdx/spdx-spec/refs/tags/v2.3/schemas/spdx-schema.json"
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	documentRaw, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(documentRaw, &document); err != nil {
		t.Fatal(err)
	}
	if err := compiled.Validate(document); err != nil {
		t.Fatalf("official SPDX 2.3 schema rejected Form Package SBOM: %v", err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
