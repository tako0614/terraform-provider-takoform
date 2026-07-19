package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestReleaseIDIsInjectiveReversibleAndFilesystemSafe(t *testing.T) {
	t.Parallel()
	kinds := []string{"SQLDatabase", "SqlDatabase", "SqLDatabase", "A" + strings.Repeat("a", 63)}
	seen := map[string]string{}
	for _, kind := range kinds {
		releaseID, err := releaseIDForKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^k-[a-z2-7]+$`).MatchString(releaseID) {
			t.Fatalf("release id %q is not case-insensitive-filesystem safe", releaseID)
		}
		if previous, duplicate := seen[releaseID]; duplicate {
			t.Fatalf("kinds %q and %q collide at %q", previous, kind, releaseID)
		}
		seen[releaseID] = kind
		decoded, err := kindFromReleaseID(releaseID)
		if err != nil || decoded != kind {
			t.Fatalf("release id %q decoded to %q, err=%v, want %q", releaseID, decoded, err, kind)
		}
	}
	maximum := mustReleaseID(t, "A"+strings.Repeat("a", 63))
	if len(maximum) != 105 || !packageTagPattern.MatchString("forms/"+maximum+"/v1.0.0") {
		t.Fatalf("64-character Kind release id is not accepted: len=%d id=%q", len(maximum), maximum)
	}
}

func TestBuildPackageIsDeterministicAndCanonical(t *testing.T) {
	repo := makeTestRepo(t)
	packageDir := filepath.Join(repo, "package")
	copyTree(t, filepath.Join(repositoryRoot(t), "conformance", "form-package-v1", "positive", "example-store"), packageDir)
	gitCommitAll(t, repo, "package")
	releaseID := mustReleaseID(t, "ExampleStore")
	tag := "forms/" + releaseID + "/v1.0.0"
	baseName := "takoform-form-" + releaseID + "_1.0.0"

	outputs := []string{filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")}
	for _, output := range outputs {
		if err := run([]string{
			"build-package", "--repo", repo, "--tag", tag,
			"--package-dir", packageDir, "--output", output, "--allow-untagged-candidate",
		}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	var manifest releaseManifest
	readJSON(t, filepath.Join(outputs[0], "release-manifest.json"), &manifest)
	wantIdentity := "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main"
	if manifest.PublisherPolicy.Identity != wantIdentity {
		t.Fatalf("publisher identity = %q, want %q", manifest.PublisherPolicy.Identity, wantIdentity)
	}
	for _, name := range []string{
		baseName + ".tar.gz",
		baseName + "_package-index.json",
		baseName + "_provenance.intoto.json",
		baseName + "_sbom.spdx.json",
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

	archive, err := os.Open(filepath.Join(outputs[0], baseName+".tar.gz"))
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
	signedIndex, err := os.ReadFile(filepath.Join(outputs[0], baseName+"_package-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(indexInArchive) != string(signedIndex) || strings.Contains(string(signedIndex), "\n") {
		t.Fatal("archive index differs from the newline-free canonical signed subject")
	}

	var sbom map[string]any
	sbomPath := filepath.Join(outputs[0], baseName+"_sbom.spdx.json")
	readJSON(t, sbomPath, &sbom)
	for _, name := range []string{baseName + "_sbom.spdx.json", baseName + "_provenance.intoto.json"} {
		raw, err := os.ReadFile(filepath.Join(outputs[0], name))
		if err != nil {
			t.Fatal(err)
		}
		canonical, err := formpackage.Canonicalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, canonical) {
			t.Fatalf("%s is not the exact RFC 8785 release evidence bytes", name)
		}
	}
	if sbom["spdxVersion"] != "SPDX-2.3" {
		t.Fatalf("unexpected SBOM: %+v", sbom)
	}
	validateSPDX(t, sbomPath)
	var provenance statement
	readJSON(t, filepath.Join(outputs[0], baseName+"_provenance.intoto.json"), &provenance)
	if provenance.Type != "https://in-toto.io/Statement/v1" || provenance.PredicateType != "https://slsa.dev/provenance/v1" || len(provenance.Subject) != 2 {
		t.Fatalf("unexpected provenance: %+v", provenance)
	}
}

func TestFinalizeRequiresTransparencyEvidence(t *testing.T) {
	repo := makeTestRepo(t)
	packageDir := filepath.Join(repo, "package")
	copyTree(t, filepath.Join(repositoryRoot(t), "conformance", "form-package-v1", "positive", "example-store"), packageDir)
	gitCommitAll(t, repo, "package")
	releaseID := mustReleaseID(t, "ExampleStore")
	tag := "forms/" + releaseID + "/v1.0.0"
	baseName := "takoform-form-" + releaseID + "_1.0.0"
	output := filepath.Join(t.TempDir(), "release")
	if err := run([]string{
		"build-package", "--repo", repo, "--tag", tag,
		"--package-dir", packageDir, "--output", output, "--allow-untagged-candidate",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	bundleName := baseName + "_package-index.sigstore.json"
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
	checkpointDir := filepath.Join(revocationDir, "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	statementPath := filepath.Join(revocationDir, "1.0.0.json")
	statement := `{"apiVersion":"trust.forms.takoform.com/v1alpha1","kind":"FormPackageRevocation","sequence":1,"statementVersion":"1.0.0","packageDigest":"` + digest + `","formRef":{"apiVersion":"forms.takoform.com/v1alpha1","kind":"ObjectBucket","definitionVersion":"1.0.0","schemaDigest":"` + digest + `"},"reasonCode":"signature-invalid","summary":"invalid retained signature","issuedAt":"2026-07-17T00:00:00Z","effects":{"blockNewCreateOrUpdate":true,"blockActivation":true,"retainBytesForObserveAndDelete":true}}`
	if err := os.WriteFile(statementPath, []byte(statement), 0o644); err != nil {
		t.Fatal(err)
	}
	statementDigest, err := formpackage.DigestCanonicalJSON([]byte(statement))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := formpackage.RevocationCheckpoint{
		APIVersion: formpackage.TrustAPIVersion, Kind: formpackage.RevocationCheckpointKind,
		CheckpointVersion: "1.0.0", Sequence: 1,
		Entries: []formpackage.RevocationCheckpointEntry{{
			Sequence: 1, StatementVersion: "1.0.0", StatementDigest: statementDigest,
			PackageDigest: digest, FormRef: formpackage.FormRef{
				APIVersion: formpackage.FormAPIVersion, Kind: "ObjectBucket",
				DefinitionVersion: "1.0.0", SchemaDigest: digest,
			},
		}},
	}
	if err := writeJSON(filepath.Join(checkpointDir, "1.0.0.json"), checkpoint); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "revocation")
	base := strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	output := filepath.Join(t.TempDir(), "release")
	if err := run([]string{
		"build-revocation", "--repo", repo, "--tag", "forms/revocations/v1.0.0",
		"--output", output, "--allow-untagged-candidate",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var manifest releaseManifest
	readJSON(t, filepath.Join(output, "release-manifest.json"), &manifest)
	if manifest.ReleaseType != "form-package-revocation" || manifest.PackageDigest != digest || manifest.Workflow != revokeWorkflow || manifest.CheckpointSequence != 1 {
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

func TestVerifyRevocationSourceChainRejectsOmissionAndFork(t *testing.T) {
	repo := t.TempDir()
	revocationDir := filepath.Join(repo, "forms", "revocations")
	checkpointDir := filepath.Join(revocationDir, "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(repositoryRoot(t), "conformance", "revocation-checkpoint-v1", "positive")
	copyFixture := func(sourceName, destination string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(fixtureDir, sourceName))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	firstStatement := filepath.Join(revocationDir, "1.0.0.json")
	secondStatement := filepath.Join(revocationDir, "1.1.0.json")
	firstCheckpoint := filepath.Join(checkpointDir, "1.0.0.json")
	secondCheckpoint := filepath.Join(checkpointDir, "1.1.0.json")
	copyFixture("statement-1.json", firstStatement)
	copyFixture("statement-2.json", secondStatement)
	copyFixture("checkpoint-1.json", firstCheckpoint)
	copyFixture("checkpoint-2.json", secondCheckpoint)

	statement, _, checkpoint, _, err := verifyRevocationSourceChain(secondStatement, secondCheckpoint)
	if err != nil {
		t.Fatal(err)
	}
	if statement.Sequence != 2 || checkpoint.Sequence != 2 {
		t.Fatalf("unexpected current chain: statement=%+v checkpoint=%+v", statement, checkpoint)
	}
	if err := os.Remove(firstStatement); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := verifyRevocationSourceChain(secondStatement, secondCheckpoint); err == nil {
		t.Fatal("statement omission unexpectedly accepted")
	}
	copyFixture("statement-1.json", firstStatement)
	checkpointRaw, err := os.ReadFile(secondCheckpoint)
	if err != nil {
		t.Fatal(err)
	}
	forked := strings.Replace(string(checkpointRaw), "sha256:58f8fac67f3abec6ab92d0ba53514e7d020f6e34f71ab014490564086b460521", "sha256:"+strings.Repeat("e", 64), 1)
	if err := os.WriteFile(secondCheckpoint, []byte(forked), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := verifyRevocationSourceChain(secondStatement, secondCheckpoint); err == nil {
		t.Fatal("checkpoint fork unexpectedly accepted")
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

func mustReleaseID(t *testing.T, kind string) string {
	t.Helper()
	releaseID, err := releaseIDForKind(kind)
	if err != nil {
		t.Fatal(err)
	}
	return releaseID
}
