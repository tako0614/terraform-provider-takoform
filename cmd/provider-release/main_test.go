package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReleaseDescriptorPinsPublicIdentityAndSigner(t *testing.T) {
	repo := testRepoRoot(t)
	desc, err := loadDescriptor(repo)
	if err != nil {
		t.Fatalf("loadDescriptor: %v", err)
	}
	if desc.ProviderAddress != "registry.terraform.io/tako0614/takoform" {
		t.Fatalf("unexpected provider address %q", desc.ProviderAddress)
	}
	if desc.SigningFingerprint != "3510E75E05BBCC303B92D77934FC18AC897FB709" {
		t.Fatalf("unexpected signer %q", desc.SigningFingerprint)
	}
	if desc.Tag != "v"+desc.Version {
		t.Fatalf("Registry tag mismatch: %q", desc.Tag)
	}
	if err := validateCLIMatrix(desc.CLIMatrix); err != nil {
		t.Fatalf("CLI/FQN matrix: %v", err)
	}
}

func TestReleaseDescriptorRejectsWrongSigner(t *testing.T) {
	repo := testRepoRoot(t)
	desc, err := loadDescriptor(repo)
	if err != nil {
		t.Fatalf("loadDescriptor: %v", err)
	}
	desc.SigningFingerprint = strings.Repeat("A", 40)
	fixture := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fixture, "release", "keys"), 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(repo, desc.PublicKeyPath), filepath.Join(fixture, desc.PublicKeyPath))
	raw, _ := json.Marshal(desc)
	if err := os.WriteFile(filepath.Join(fixture, descriptorPath), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDescriptor(fixture); err == nil || !strings.Contains(err.Error(), "signing identity mismatch") {
		t.Fatalf("expected wrong signer rejection, got %v", err)
	}
}

func TestReleaseDescriptorRejectsAliasedCLIMatrix(t *testing.T) {
	desc := testDescriptor()
	desc.CLIMatrix[0].ProviderAddress = desc.ProviderAddress
	if err := validateCLIMatrix(desc.CLIMatrix); err == nil || !strings.Contains(err.Error(), "invalid release CLI/FQN matrix") {
		t.Fatalf("expected aliased CLI/FQN matrix rejection, got %v", err)
	}
}

func TestValidSignatureFingerprintParsesGPGStatusOnly(t *testing.T) {
	const fingerprint = "3510E75E05BBCC303B92D77934FC18AC897FB709"
	status := "[GNUPG:] GOODSIG 34FC18AC897FB709 Takoform\n[GNUPG:] VALIDSIG " + fingerprint + " 2026-07-16 0 4 0 1 10 00 " + fingerprint + "\n"
	if got := validSignatureFingerprint(status); got != fingerprint {
		t.Fatalf("got %q, want %q", got, fingerprint)
	}
	if got := validSignatureFingerprint("gpg: Good signature from somebody"); got != "" {
		t.Fatalf("human text must not establish signer identity: %q", got)
	}
}

func TestVerifyPinnedTagSignerRejectsWrongSigner(t *testing.T) {
	const expected = "3510E75E05BBCC303B92D77934FC18AC897FB709"
	const wrong = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	status := "[GNUPG:] VALIDSIG " + wrong + " 2026-07-16 0 4 0 1 10 00 " + wrong + "\n"
	if _, err := verifyPinnedTagSigner(status, nil, expected); err == nil || !strings.Contains(err.Error(), "does not match pinned signer") {
		t.Fatalf("expected wrong-signer rejection, got %v", err)
	}
}

func TestVerifyExpectedTriggerTagRequiresExactDescriptorTag(t *testing.T) {
	if err := verifyExpectedTriggerTag("v0.1.0-rc.2", "v0.1.0-rc.2"); err != nil {
		t.Fatalf("exact trigger tag: %v", err)
	}
	if err := verifyExpectedTriggerTag("", "v0.1.0-rc.2"); err != nil {
		t.Fatalf("local verification may omit trigger tag: %v", err)
	}
	if err := verifyExpectedTriggerTag("v0.1.0", "v0.1.0-rc.2"); err == nil || !strings.Contains(err.Error(), "does not match descriptor tag") {
		t.Fatalf("expected mismatched trigger rejection, got %v", err)
	}
}

func TestInspectSourceRejectsUnsignedExactTag(t *testing.T) {
	repo := newGitFixture(t)
	desc := testDescriptor()
	run(t, repo, "git", "tag", "-a", desc.Tag, "-m", "unsigned release tag")
	_, err := inspectSource(repo, desc, false, false)
	if err == nil || !strings.Contains(err.Error(), "not signed by pinned signer") {
		t.Fatalf("expected unsigned tag rejection, got %v", err)
	}
}

func TestInspectSourceAllowsOnlyExplicitDirtyUntaggedCandidate(t *testing.T) {
	repo := newGitFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	desc := testDescriptor()
	if _, err := inspectSource(repo, desc, false, true); err == nil || !strings.Contains(err.Error(), "source tree is dirty") {
		t.Fatalf("expected default dirty rejection, got %v", err)
	}
	evidence, err := inspectSource(repo, desc, true, true)
	if err != nil {
		t.Fatalf("explicit candidate seam: %v", err)
	}
	if !evidence.Dirty || evidence.TagPresent || evidence.PublicationReady {
		t.Fatalf("unsafe candidate evidence %#v", evidence)
	}
}

func TestDeterministicZip(t *testing.T) {
	root := t.TempDir()
	payload := filepath.Join(root, "provider")
	if err := os.WriteFile(payload, []byte("same bytes\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(root, "first.zip"), filepath.Join(root, "second.zip")
	inputs := []zipInput{{Name: "provider", Path: payload, Mode: 0o755}}
	if err := deterministicZip(first, inputs); err != nil {
		t.Fatal(err)
	}
	if err := deterministicZip(second, inputs); err != nil {
		t.Fatal(err)
	}
	left, _, _ := fileDigest(first)
	right, _, _ := fileDigest(second)
	if left != right {
		t.Fatalf("deterministic ZIP digest drift: %s != %s", left, right)
	}
}

func TestCandidateBuildManifestAndNoOverwrite(t *testing.T) {
	repo := testRepoRoot(t)
	desc, err := loadDescriptor(repo)
	if err != nil {
		t.Fatal(err)
	}
	desc.Platforms = []string{runtime.GOOS + "_" + runtime.GOARCH}
	desc.GoVersion = runtime.Version()
	output := filepath.Join(t.TempDir(), "candidate")
	manifest, err := build(repo, output, desc, true, true)
	if err != nil {
		t.Fatalf("build candidate: %v", err)
	}
	if manifest.PublicationReady || len(manifest.Artifacts) != 1 {
		t.Fatalf("unexpected candidate manifest %#v", manifest)
	}
	if manifest.Artifacts[0].EmbeddedVersionLDFlag != "-X main.version="+desc.Version {
		t.Fatalf("missing embedded version evidence %#v", manifest.Artifacts[0])
	}
	for _, name := range []string{"SHA256SUMS", "manifest.json", "provenance.json", "sbom.spdx.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("candidate missing %s: %v", name, err)
		}
	}
	if _, err := build(repo, output, desc, true, true); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected no-overwrite rejection, got %v", err)
	}
}

func TestCreateSBOMUsesDeterministicSPDXCreationTime(t *testing.T) {
	repo := testRepoRoot(t)
	desc, err := loadDescriptor(repo)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "HEAD"))
	first, err := createSBOM(repo, desc, commit)
	if err != nil {
		t.Fatalf("create first SBOM: %v", err)
	}
	second, err := createSBOM(repo, desc, commit)
	if err != nil {
		t.Fatalf("create second SBOM: %v", err)
	}
	if first.CreationInfo.Created == "" || first.CreationInfo.Created != second.CreationInfo.Created {
		t.Fatalf("SPDX creationInfo.created must be present and deterministic: %q != %q", first.CreationInfo.Created, second.CreationInfo.Created)
	}
	created, err := time.Parse(time.RFC3339, first.CreationInfo.Created)
	if err != nil {
		t.Fatalf("SPDX creationInfo.created is not an RFC 3339 timestamp: %v", err)
	}
	if created.Location() != time.UTC {
		t.Fatalf("SPDX creationInfo.created must be normalized to UTC, got %q", first.CreationInfo.Created)
	}
	want := strings.TrimSpace(runOutput(t, repo, "git", "show", "-s", "--format=%cI", commit))
	wantTime, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatal(err)
	}
	if first.CreationInfo.Created != wantTime.UTC().Format(time.RFC3339) {
		t.Fatalf("SPDX creation time %q does not match source commit %q", first.CreationInfo.Created, want)
	}
	if err := validateSPDX(repo, first); err != nil {
		t.Fatalf("official SPDX 2.3 schema rejected candidate SBOM: %v", err)
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"checksums"`) {
		t.Fatal("Go module sums are not artifact SHA256 digests and must not be emitted as SPDX checksums")
	}
}

func TestSPDXValidationRejectsMissingCreated(t *testing.T) {
	repo := testRepoRoot(t)
	desc, err := loadDescriptor(repo)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "HEAD"))
	document, err := createSBOM(repo, desc, commit)
	if err != nil {
		t.Fatal(err)
	}
	document.CreationInfo.Created = ""
	if err := validateSPDX(repo, document); err == nil {
		t.Fatal("SPDX validation accepted an empty creationInfo.created")
	}
}

func TestOfficialInTotoAndSLSAValidatorsAcceptCandidateProvenance(t *testing.T) {
	desc := testDescriptor()
	evidence := sourceEvidence{Commit: strings.Repeat("a", 40), GoVersion: desc.GoVersion}
	artifacts := []artifact{{Archive: "terraform-provider-takoform_0.1.0-rc.2_linux_amd64.zip", ArchiveSHA256: strings.Repeat("b", 64)}}
	document := createProvenance(desc, evidence, artifacts)
	if err := validateSLSAProvenance(document); err != nil {
		t.Fatalf("official in-toto/SLSA validators rejected candidate provenance: %v", err)
	}
	delete(document.Predicate.BuildDefinition, "internalParameters")
	if err := validateSLSAProvenance(document); err == nil || !strings.Contains(err.Error(), "internalParameters") {
		t.Fatalf("expected explicit internalParameters rejection, got %v", err)
	}
}

func testDescriptor() descriptor {
	return descriptor{
		SchemaVersion:    1,
		Version:          "0.1.0-rc.2",
		Tag:              "v0.1.0-rc.2",
		SourceRepository: "github.com/tako0614/terraform-provider-takoform",
		ProviderAddress:  "registry.terraform.io/tako0614/takoform",
		CLIMatrix: []cliCompatibility{
			{Product: "OpenTofu", Version: "1.12.1", ProviderAddress: "registry.opentofu.org/tako0614/takoform"},
			{Product: "Terraform", Version: "1.15.8", ProviderAddress: "registry.terraform.io/tako0614/takoform"},
		},
		GoModule:           "github.com/tako0614/terraform-provider-takoform",
		GoVersion:          runtime.Version(),
		SigningFingerprint: "3510E75E05BBCC303B92D77934FC18AC897FB709",
		PublicKeyPath:      "release/keys/provider-signing-key.asc",
		Platforms:          []string{"linux_amd64"},
		PublicationStatus:  "candidate-only",
	}
}

func newGitFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "git", "init", "--quiet")
	run(t, repo, "git", "config", "user.name", "Release Test")
	run(t, repo, "git", "config", "user.email", "release-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "add", "README.md")
	run(t, repo, "git", "commit", "--quiet", "-m", "fixture")
	return repo
}

func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
}

func runOutput(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func copyFile(t *testing.T, source, target string) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
