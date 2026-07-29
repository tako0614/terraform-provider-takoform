package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	if desc.Version != "1.0.0" {
		t.Fatalf("first stable provider candidate must be 1.0.0, got %q", desc.Version)
	}
	if err := validateVersioningPolicy(desc.Versioning); err != nil {
		t.Fatalf("versioning policy: %v", err)
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

func TestReleaseDescriptorRejectsNonCanonicalCLIMatrix(t *testing.T) {
	desc := testDescriptor()
	desc.CLIMatrix[0].ProviderAddress = "registry.opentofu.org/tako0614/takoform"
	if err := validateCLIMatrix(desc.CLIMatrix); err == nil || !strings.Contains(err.Error(), "invalid release CLI/FQN matrix") {
		t.Fatalf("expected non-canonical CLI/FQN matrix rejection, got %v", err)
	}
}

func TestReleaseDescriptorRejectsConflatedVersionStreams(t *testing.T) {
	policy := testDescriptor().Versioning
	policy.FormDefinitionVersions = "match-provider-version"
	if err := validateVersioningPolicy(policy); err == nil || !strings.Contains(err.Error(), "version streams") {
		t.Fatalf("expected conflated version streams rejection, got %v", err)
	}
}

func TestRegistryChecksumTargetsContainArchivesAndManifestForTerraformAndOpenTofu(t *testing.T) {
	repo := testRepoRoot(t)
	desc, err := loadDescriptor(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"terraform-provider-takoform_" + desc.Version + "_darwin_amd64.zip",
		"terraform-provider-takoform_" + desc.Version + "_darwin_arm64.zip",
		"terraform-provider-takoform_" + desc.Version + "_linux_amd64.zip",
		"terraform-provider-takoform_" + desc.Version + "_linux_arm64.zip",
		"terraform-provider-takoform_" + desc.Version + "_manifest.json",
		"terraform-provider-takoform_" + desc.Version + "_windows_amd64.zip",
	}
	var first []string
	for _, product := range []string{"Terraform", "OpenTofu"} {
		got, err := registryChecksumTargets(desc, product)
		if err != nil {
			t.Fatalf("%s checksum targets: %v", product, err)
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s checksum targets = %v, want %v", product, got, want)
		}
		for _, name := range got {
			manifest := "terraform-provider-takoform_" + desc.Version + "_manifest.json"
			if strings.Contains(name, ".spdx.json") || (!strings.HasSuffix(name, ".zip") && name != manifest) {
				t.Fatalf("%s checksum manifest contains non-Registry asset %q", product, name)
			}
		}
		if first == nil {
			first = got
		} else if strings.Join(first, "\n") != strings.Join(got, "\n") {
			t.Fatalf("Terraform and OpenTofu checksum contracts diverged: %v != %v", first, got)
		}
	}
	if _, err := registryChecksumTargets(desc, "Other"); err == nil || !strings.Contains(err.Error(), "unsupported Registry checksum product") {
		t.Fatalf("unknown Registry product was accepted: %v", err)
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

func TestProviderTagWorkflowExportsReadOnlySignedObject(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(testRepoRoot(t), ".github", "workflows", "provider-release-tag.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"run-name: ${{ inputs.request_id }}",
		"request_id:",
		`[[ ! "$REQUEST_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]`,
		"contents: read",
		"persist-credentials: false",
		"takoform.provider-signed-tag-artifact@v1",
		"request-id: %s",
		`--arg requestId "$REQUEST_ID"`,
		`--arg runId "$GITHUB_RUN_ID"`,
		`--arg runAttempt "$GITHUB_RUN_ATTEMPT"`,
		"requestId:$requestId",
		"runId:$runId",
		"runAttempt:$runAttempt",
		"preflight-sha256:",
		"git cat-file tag",
		"provider-tag-preflight-${{ inputs.request_id }}-${{ github.run_id }}-${{ github.run_attempt }}-${{ github.sha }}",
		"provider-signed-tag-${{ inputs.request_id }}-${{ github.run_id }}-${{ github.run_attempt }}",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("provider tag workflow lacks %q", required)
		}
	}
	for _, forbidden := range []string{"contents: write", "persist-credentials: true", "git push origin"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("provider tag signing workflow retains remote write authority %q", forbidden)
		}
	}
}

func TestProviderTagPreflightRejectsRerunAndRequestSubstitution(t *testing.T) {
	requestID := "123e4567-e89b-42d3-a456-426614174000"
	commit := strings.Repeat("a", 40)
	preflight := providerTagPreflight{
		Format:       "takoform.provider-tag-preflight@v1",
		RequestID:    requestID,
		RunID:        "123",
		RunAttempt:   "2",
		SourceCommit: commit,
	}
	if err := verifyProviderTagPreflightBinding(preflight, requestID, "123", "2", commit); err != nil {
		t.Fatalf("exact preflight run binding: %v", err)
	}
	for name, mutate := range map[string]func(*providerTagPreflight){
		"request": func(value *providerTagPreflight) { value.RequestID = "223e4567-e89b-42d3-a456-426614174000" },
		"run":     func(value *providerTagPreflight) { value.RunID = "124" },
		"attempt": func(value *providerTagPreflight) { value.RunAttempt = "1" },
		"commit":  func(value *providerTagPreflight) { value.SourceCommit = strings.Repeat("b", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			substituted := preflight
			mutate(&substituted)
			if err := verifyProviderTagPreflightBinding(substituted, requestID, "123", "2", commit); err == nil {
				t.Fatal("substituted preflight run binding was accepted")
			}
		})
	}
}

func TestVerifyClosedChecksumsRejectsExtraAndTamperedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "payload"), []byte("trusted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, _, err := fileDigest(filepath.Join(root, "payload"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(digest+"  payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyClosedChecksums(root, []string{"SHA256SUMS", "payload"}, []string{"payload"}); err != nil {
		t.Fatalf("valid closed inventory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "extra"), []byte("unexpected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyClosedChecksums(root, []string{"SHA256SUMS", "payload"}, []string{"payload"}); err == nil || !strings.Contains(err.Error(), "inventory mismatch") {
		t.Fatalf("expected extra-file rejection, got %v", err)
	}
}

func TestVerifyTagObjectBindingsRequiresExactRunAndPreflight(t *testing.T) {
	commit := strings.Repeat("a", 40)
	preflight := "sha256:" + strings.Repeat("b", 64)
	requestID := "123e4567-e89b-42d3-a456-426614174000"
	runURL := "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/1"
	raw := []byte("object " + commit + "\ntype commit\ntag v0.1.0-rc.2\ntagger Takoform Provider Release <release@takoform.invalid> 1784408928 +0000\n\n" +
		"Takoform provider v0.1.0-rc.2\n\nsource-commit: " + commit + "\nrequest-id: " + requestID + "\npreflight-sha256: " + preflight + "\nworkflow-run: " + runURL + "\n" +
		"-----BEGIN PGP SIGNATURE-----\nfixture\n")
	if err := verifyTagObjectBindings(raw, "v0.1.0-rc.2", commit, requestID, preflight, runURL); err != nil {
		t.Fatalf("exact binding: %v", err)
	}
	if err := verifyTagObjectBindings(raw, "v0.1.0-rc.2", commit, requestID, "sha256:"+strings.Repeat("c", 64), runURL); err == nil {
		t.Fatal("mismatched preflight digest was accepted")
	}
	if err := verifyTagObjectBindings(raw, "v0.1.0-rc.2", commit, "223e4567-e89b-42d3-a456-426614174000", preflight, runURL); err == nil {
		t.Fatal("mismatched request id was accepted")
	}
}

func TestCanonicalRequestIDRequiresLowercaseUUIDv4(t *testing.T) {
	for _, valid := range []string{
		"123e4567-e89b-42d3-a456-426614174000",
		"ffffffff-ffff-4fff-bfff-ffffffffffff",
	} {
		if !requestIDPattern.MatchString(valid) {
			t.Errorf("canonical UUIDv4 rejected: %s", valid)
		}
	}
	for _, invalid := range []string{
		"",
		"123E4567-E89B-42D3-A456-426614174000",
		"123e4567-e89b-72d3-a456-426614174000",
		"123e4567-e89b-42d3-c456-426614174000",
		"123e4567e89b42d3a456426614174000",
	} {
		if requestIDPattern.MatchString(invalid) {
			t.Errorf("non-canonical UUIDv4 accepted: %s", invalid)
		}
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
	want := "direct Registry install/readback for provider " + desc.Version + " is post-publication evidence"
	if !containsString(evidence.Blockers, want) {
		t.Fatalf("candidate blockers do not bind the exact provider version: %#v", evidence.Blockers)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func TestVerifySPDXFilesRequiresPathsAndRejectsInvalidOrTrailingJSON(t *testing.T) {
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
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := verifySPDXFiles(repo, nil); err == nil || !strings.Contains(err.Error(), "one or more SPDX JSON paths") {
		t.Fatalf("verify-sbom accepted no paths: %v", err)
	}

	valid := filepath.Join(t.TempDir(), "valid.spdx.json")
	if err := os.WriteFile(valid, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	verified, err := verifySPDXFiles(repo, []string{valid})
	if err != nil || len(verified) != 1 || verified[0] != filepath.Base(valid) {
		t.Fatalf("valid SPDX verification = %#v, %v", verified, err)
	}

	var invalid map[string]any
	if err := json.Unmarshal(raw, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid["creationInfo"] = "not-an-object"
	invalidRaw, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(t.TempDir(), "invalid.spdx.json")
	if err := os.WriteFile(invalidPath, invalidRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifySPDXFiles(repo, []string{invalidPath}); err == nil {
		t.Fatal("verify-sbom accepted a document rejected by the pinned SPDX schema")
	}

	trailing := filepath.Join(t.TempDir(), "trailing.spdx.json")
	if err := os.WriteFile(trailing, append(append(raw, '\n'), []byte("{}")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifySPDXFiles(repo, []string{trailing}); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("verify-sbom accepted trailing JSON: %v", err)
	}
}

func TestProviderReleaseWorkflowExercisesTheExactReproducibleFinalArchiveBeforeClosure(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(testRepoRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	firstBuild := strings.Index(text, "Build the first final unsigned release inventory")
	secondBuild := strings.Index(text, "Build the second and final unsigned release inventory")
	finalComparison := strings.Index(text, "Verify the exact final archives are reproducible")
	finalExercise := strings.Index(text, "Exercise the exact final linux amd64 provider bytes")
	validation := strings.Index(text, "Validate final Syft SBOMs against the pinned SPDX schema")
	closure := strings.Index(text, "Close and export the unsigned inventory")
	if firstBuild < 0 || secondBuild <= firstBuild || finalComparison <= secondBuild ||
		finalExercise <= finalComparison || validation <= finalExercise || closure <= validation {
		t.Fatal("provider release must exercise the exact second reproducible final archive and validate its SBOMs before inventory closure")
	}
	const finalBuildCommand = "args: release --config ../.goreleaser.yml --clean --skip=publish,sign"
	if strings.Count(text[firstBuild:finalComparison], finalBuildCommand) != 2 {
		t.Fatal("provider release must run the same final non-publishing GoReleaser command exactly twice")
	}
	if strings.Contains(text, "--snapshot") {
		t.Fatal("provider release must not compare snapshot-version bytes with final-version bytes")
	}
	for _, required := range []string{
		`first="$RUNNER_TEMP/goreleaser-final-1"`,
		`final="$GITHUB_WORKSPACE/release-source/dist"`,
		`diff -u "$RUNNER_TEMP/first-final-archives.txt" "$RUNNER_TEMP/second-final-archives.txt"`,
		`cmp -s "$first/$name" "$final/$name"`,
		`test "$(sha256sum "$first/$name" | cut -d' ' -f1)" = "$(sha256sum "$final/$name" | cut -d' ' -f1)"`,
	} {
		if !strings.Contains(text[finalComparison:finalExercise], required) {
			t.Fatalf("final archive reproducibility comparison lacks %q", required)
		}
	}
	for _, required := range []string{
		`archive="$GITHUB_WORKSPACE/release-source/dist/terraform-provider-takoform_${version}_linux_amd64.zip"`,
		`printf '%s\n' LICENSE "$entry"`,
		`unzip -Z1 "$archive"`,
		`unzip -p "$archive" "$entry" > "$binary"`,
		`test "$("$binary" -version)" = "$version"`,
		`go -C release-source run ./cmd/provider-lifecycle-conformance render-matrix`,
		`--provider-binary "$binary"`,
		`.providerBinary.sha256 == $sha256`,
		`test "$(sha256sum "$archive" | cut -d' ' -f1)" = "$archive_sha256"`,
		`test "sha256:$(sha256sum "$binary" | cut -d' ' -f1)" = "$binary_sha256"`,
	} {
		if !strings.Contains(text[finalExercise:validation], required) {
			t.Fatalf("exact final provider lifecycle step lacks %q", required)
		}
	}
	if strings.Contains(text[:firstBuild], "provider-lifecycle-conformance matrix --") {
		t.Fatal("provider lifecycle must not be satisfied by rebuilding a pre-GoReleaser provider")
	}
	for _, required := range []string{
		"-name '*.zip.spdx.json'",
		`[[ "${#sboms[@]}" -ne 5 ]]`,
		`go -C cmd/provider-release run . verify-sbom "${sboms[@]}"`,
	} {
		if !strings.Contains(text[validation:closure], required) {
			t.Fatalf("final SBOM validation step lacks %q", required)
		}
	}
}

func TestProviderReleaseWorkflowPreparesChecksumClosedCandidateWithoutProductionMutation(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(testRepoRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"workflow_dispatch:",
		"run-name: ${{ inputs.request_id }}",
		"request_id:",
		`[[ ! "$REQUEST_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]`,
		"environment: provider-release",
		`github_release_status="$(curl`,
		`if [[ "$github_release_status" != "404" ]]`,
		`if [[ "$registry_status" != "404" ]]`,
		`test "$(wc -l < "$RUNNER_TEMP/candidate-assets.txt")" -eq 15`,
		`--arg format "takoform.provider-release-candidate.v1"`,
		`--arg requestId "$REQUEST_ID"`,
		`--arg tagObjectOid "${{ steps.inventory.outputs.tag_object_oid }}"`,
		`--arg tagObjectSha256 "${{ steps.inventory.outputs.tag_object_sha256 }}"`,
		`--arg sha256 "sha256:$digest"`,
		"requestId: $requestId",
		"tagObjectOid: $tagObjectOid",
		"tagObjectSha256: $tagObjectSha256",
		`git cat-file tag "$tag_object_oid" > "$tag_object"`,
		`test "$(git hash-object -t tag "$tag_object")" = "$tag_object_oid"`,
		"sourceCommit: $sourceCommit",
		"toolingCommit: $toolingCommit",
		`printf '%s\n' SHA256SUMS assets metadata.json`,
		`test "$(wc -l < SHA256SUMS)" -eq 16`,
		"sha256sum --check --strict SHA256SUMS",
		"name: provider-release-candidate-${{ github.run_id }}-${{ github.run_attempt }}",
		"retention-days: 1",
		`"$GITHUB_REF" != "refs/tags/$RELEASE_TAG"`,
		`expected_workflow_ref="$GITHUB_REPOSITORY/.github/workflows/release.yml@refs/tags/$RELEASE_TAG"`,
		`test "$GITHUB_REF" = "refs/tags/$RELEASE_TAG"`,
		`test "$GITHUB_WORKFLOW_REF" = "$GITHUB_REPOSITORY/.github/workflows/release.yml@refs/tags/$RELEASE_TAG"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("provider preparation workflow lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"id-token: write",
		"attestations: write",
		"actions/attest@",
		"gh release create",
		"gh release upload",
		"--method POST",
		"--method PATCH",
		"--method DELETE",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("provider preparation workflow retains production mutation %q", forbidden)
		}
	}
	if strings.Count(text, `test "$tag_commit" = "$GITHUB_SHA"`) != 2 {
		t.Fatal("both provider jobs must bind the peeled signed-tag source and tooling commit to the exact workflow commit")
	}
	if strings.Contains(text, "refs/heads/main") || strings.Contains(text, "protected-main-release-tooling") {
		t.Fatal("provider release candidate workflow must not retain mutable-main execution or provenance identity")
	}
}

func TestProviderReleaseWorkflowProducesCanonicalSignedProvenance(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(testRepoRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	checksumSignature := strings.Index(text, `--detach-sign "$checksum"`)
	provenance := strings.Index(text, `provenance="${base}_provenance.intoto.json"`)
	provenanceSignature := strings.Index(text, `--detach-sign "$provenance"`)
	if checksumSignature < 0 || provenance <= checksumSignature || provenanceSignature <= provenance {
		t.Fatal("provider provenance must be created and signed only after the checksum signature exists")
	}
	for _, required := range []string{
		`test "$(wc -l < "$RUNNER_TEMP/expected-payload-inventory.txt")" -eq 13`,
		`annotations: {size}`,
		`"_type": "https://in-toto.io/Statement/v1"`,
		`predicateType: "https://slsa.dev/provenance/v1"`,
		`buildType: "https://takoform.com/buildtypes/provider-release/v1"`,
		`canonicalization: "RFC8785"`,
		`externalParameters: {tag, requestId}`,
		`sourceCommit,`,
		`toolingCommit,`,
		`workflow: {path: workflowPath, ref: workflowRef}`,
		`run: {id: runId, attempt: runAttempt}`,
		`tagObject: {oid: tagObjectOid, sha256: tagObjectSha256}`,
		`metadata: {invocationId}`,
		`node <<'NODE'`,
		`Number.isSafeInteger(size)`,
		`JSON.stringify(subjectNames) !== JSON.stringify(expectedNames)`,
		`JSON.stringify(recursivelySort(statement))`,
		`gpg --batch --verify "$provenance_signature" "$provenance"`,
		`test "$(wc -l < "$RUNNER_TEMP/expected-signed-inventory.txt")" -eq 15`,
	} {
		if !strings.Contains(text[checksumSignature:], required) {
			t.Fatalf("provider provenance closure lacks %q", required)
		}
	}
}

func TestProviderReleaseWorkflowDestroysSigningAuthorityBeforeRepositoryCode(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(testRepoRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	importKey := strings.Index(text, "Import provider signing key only after static verification")
	checksumSignature := strings.Index(text, `--detach-sign "$checksum"`)
	provenanceSignature := strings.Index(text, `--detach-sign "$provenance"`)
	deleteKey := strings.Index(text, `--delete-secret-keys "$EXPECTED_GPG_FINGERPRINT"`)
	killAgent := strings.Index(text, "gpgconf --kill gpg-agent")
	verifyJob := strings.Index(text, "\n  verify:\n")
	repositoryVerifier := strings.Index(text, "go -C cmd/provider-release run . verify-release-provenance")
	if importKey < 0 || checksumSignature <= importKey || provenanceSignature <= checksumSignature ||
		deleteKey <= provenanceSignature || killAgent <= deleteKey || verifyJob <= killAgent ||
		repositoryVerifier <= verifyJob {
		t.Fatal("provider workflow must sign, destroy secret-key authority, and only then run repository verification in a separate job")
	}
	signingJob := text[importKey:verifyJob]
	for _, forbidden := range []string{
		"go run ",
		"go -C ",
		"bun ",
		"release-source/",
		"provider-lifecycle-conformance",
		"./cmd/",
		"unzip ",
	} {
		if strings.Contains(signingJob, forbidden) {
			t.Fatalf("protected provider signing job executes repository-controlled candidate code %q while signing authority may still exist", forbidden)
		}
	}
	verifyBlock := text[verifyJob:]
	if strings.Contains(verifyBlock, "environment:") || !strings.Contains(verifyBlock, "needs: prepare") ||
		!strings.Contains(verifyBlock, `--assets "$candidate/assets"`) {
		t.Fatal("provider provenance verifier must be a no-Environment downstream consumer of the exact candidate")
	}
}

func TestVerifyCanonicalJSONFileRejectsNonRFC8785AndTrailingBytes(t *testing.T) {
	repo := testRepoRoot(t)
	for _, test := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "canonical", raw: `{"a":1,"b":2}`, ok: true},
		{name: "unsorted", raw: `{"b":2,"a":1}`},
		{name: "trailing newline", raw: "{\"a\":1,\"b\":2}\n"},
		{name: "duplicate key", raw: `{"a":1,"a":1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "provenance.json")
			raw := []byte(test.raw)
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			err := verifyCanonicalJSONFile(repo, path, raw)
			if test.ok && err != nil {
				t.Fatalf("canonical document rejected: %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("noncanonical document accepted")
			}
		})
	}
}

func TestReleaseProvenanceStrictShapeAndBindingsRejectAliasesAndMismatch(t *testing.T) {
	want := releaseProvenanceStatement{
		Type:          "https://in-toto.io/Statement/v1",
		PredicateType: "https://slsa.dev/provenance/v1",
		Predicate: releaseProvenancePredicate{
			BuildDefinition: releaseProvenanceBuildDefinition{
				ExternalParameters: releaseProvenanceExternalParameters{
					Tag: "v1.0.0", RequestID: "123e4567-e89b-42d3-a456-426614174000",
				},
			},
		},
	}
	if err := verifyReleaseProvenanceSemantics(want, want); err != nil {
		t.Fatalf("exact semantic binding rejected: %v", err)
	}
	wrong := want
	wrong.Predicate.BuildDefinition.ExternalParameters.RequestID = "223e4567-e89b-42d3-a456-426614174000"
	if err := verifyReleaseProvenanceSemantics(wrong, want); err == nil {
		t.Fatal("wrong request binding accepted")
	}
	external := []byte(`{"requestId":"123e4567-e89b-42d3-a456-426614174000","tag":"v1.0.0"}`)
	if _, err := exactJSONObject(external, "externalParameters", "requestId", "tag"); err != nil {
		t.Fatalf("exact external parameter keys rejected: %v", err)
	}
	alias := strings.Replace(string(external), `"requestId"`, `"requestID"`, 1)
	if _, err := exactJSONObject([]byte(alias), "externalParameters", "requestId", "tag"); err == nil {
		t.Fatal("unknown requestId alias accepted")
	}
}

func TestPinnedDetachedSignatureRejectsTamperedSubject(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := command("", nil, "gpg", "--homedir", home, "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "Takoform Provenance Test <test@takoform.invalid>", "rsa2048", "sign", "0"); err != nil {
		t.Fatal(err)
	}
	keys, err := command("", nil, "gpg", "--homedir", home, "--batch", "--with-colons", "--list-secret-keys")
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := ""
	for _, line := range strings.Split(keys, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "fpr" {
			fingerprint = strings.ToUpper(fields[9])
			break
		}
	}
	if !regexp.MustCompile(`^[0-9A-F]{40}$`).MatchString(fingerprint) {
		t.Fatalf("test key has invalid fingerprint %q", fingerprint)
	}
	publicKey, err := command("", nil, "gpg", "--homedir", home, "--batch", "--armor", "--export", fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyPath := filepath.Join(t.TempDir(), "provider-signing-key.asc")
	if err := os.WriteFile(publicKeyPath, []byte(publicKey), 0o644); err != nil {
		t.Fatal(err)
	}
	subjectPath := filepath.Join(t.TempDir(), "provenance.intoto.json")
	signaturePath := subjectPath + ".sig"
	if err := os.WriteFile(subjectPath, []byte(`{"signed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := command("", nil, "gpg", "--homedir", home, "--batch", "--local-user", fingerprint,
		"--output", signaturePath, "--detach-sign", subjectPath); err != nil {
		t.Fatal(err)
	}
	if signer, err := verifyPinnedDetachedSignature(publicKeyPath, fingerprint, signaturePath, subjectPath); err != nil || signer != fingerprint {
		t.Fatalf("valid pinned signature rejected: signer=%q err=%v", signer, err)
	}
	if err := os.WriteFile(subjectPath, []byte(`{"signed":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyPinnedDetachedSignature(publicKeyPath, fingerprint, signaturePath, subjectPath); err == nil {
		t.Fatal("tampered provenance subject accepted")
	}
}

func TestProviderReleaseWorkflowSeparatesRegistryChecksumsFromEvidenceInventory(t *testing.T) {
	repo := testRepoRoot(t)
	workflow, err := os.ReadFile(filepath.Join(repo, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"registry-checksum-targets --product Terraform",
		"registry-checksum-targets --product OpenTofu",
		"expected-registry-checksum-assets.txt",
		`diff -u "$terraform_checksum_targets" "$opentofu_checksum_targets"`,
		`diff -u "$expected_registry_checksums" "$RUNNER_TEMP/checksum-assets.txt"`,
		`printf '%s\n' "terraform-provider-takoform_${version}_manifest.json"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("provider release workflow lacks Registry checksum contract %q", required)
		}
	}
	if strings.Contains(text, `for name in "${unsigned[@]}"; do sha256sum "$name"`) {
		t.Fatal("provider release still checksums the full evidence inventory")
	}

	goreleaser, err := os.ReadFile(filepath.Join(repo, ".goreleaser.yml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(goreleaser)
	checksumStart := strings.Index(config, "checksum:\n")
	signsStart := strings.Index(config, "\nsigns:\n")
	if checksumStart < 0 || signsStart <= checksumStart {
		t.Fatal("cannot locate GoReleaser checksum contract")
	}
	checksumBlock := config[checksumStart:signsStart]
	if !strings.Contains(checksumBlock, "extra_files") || !strings.Contains(checksumBlock, "*_manifest.json") || strings.Contains(checksumBlock, "spdx") {
		t.Fatalf("GoReleaser checksum contract must include only archives plus the Registry manifest:\n%s", checksumBlock)
	}
	if !strings.Contains(config, "sboms:\n") || !strings.Contains(config, "release:\n") || !strings.Contains(config, ".release-tmp/*_manifest.json") {
		t.Fatal("SBOM or Registry manifest evidence was removed instead of being separately attached")
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
			{Product: "OpenTofu", Version: "1.12.1", ProviderAddress: "registry.terraform.io/tako0614/takoform"},
			{Product: "Terraform", Version: "1.15.8", ProviderAddress: "registry.terraform.io/tako0614/takoform"},
		},
		GoModule:           "github.com/tako0614/terraform-provider-takoform",
		GoVersion:          runtime.Version(),
		SigningFingerprint: "3510E75E05BBCC303B92D77934FC18AC897FB709",
		PublicKeyPath:      "release/keys/provider-signing-key.asc",
		Platforms:          []string{"linux_amd64"},
		PublicationStatus:  "candidate-only",
		Versioning: versioningPolicy{
			ProviderCompatibility:  "semver-major",
			PortableAPIVersion:     "forms.takoform.com/v1alpha1",
			FormDefinitionVersions: "independent-immutable-semver",
			FormPackageVersions:    "independent-immutable-semver",
			AdmissionGenerations:   "independent-non-semver",
		},
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
