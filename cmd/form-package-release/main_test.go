package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestSetWideReleaseSurfaceIsNotExposed(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"build-standard-set",
		"finalize-standard-set",
		"verify-standard-set-candidate",
		"build-standard-set-readback",
	} {
		if err := run([]string{command}, io.Discard); err == nil || err.Error() != usageError().Error() {
			t.Fatalf("retired command %q remains exposed: %v", command, err)
		}
	}
	if err := run([]string{
		"build-package",
		"--tag", "forms/k-j5rguzldorbhky3lmv2a/v1.0.0",
		"--output", filepath.Join(t.TempDir(), "release"),
		"--tooling-commit", testToolingCommit,
		"--coordinated-standard-set",
	}, io.Discard); err == nil || err.Error() != usageError().Error() {
		t.Fatalf("retired --coordinated-standard-set flag remains exposed: %v", err)
	}
	if strings.Contains(usageError().Error(), "standard-set") {
		t.Fatalf("usage advertises the retired set-wide surface: %s", usageError())
	}
}

const testToolingCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

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
	if _, err := formpackage.ParsePublicationTag("forms/" + maximum + "/v1.0.0"); len(maximum) != 105 || err != nil {
		t.Fatalf("64-character Kind release id is not accepted: len=%d id=%q", len(maximum), maximum)
	}
}

func TestBuildPackageIsDeterministicAndCanonical(t *testing.T) {
	repo, packageDir, report := stageFamilyPackage(t)
	releaseID := formpackage.ReleaseIDForGroupKind(testFamilyGroup, "ModuleWorker")
	artifactID := strings.Replace(report.PackageDigest, ":", "-", 1)
	tag := "forms/" + releaseID + "/" + artifactID
	baseName := "takoform-form-" + releaseID + "_" + artifactID

	outputs := []string{filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")}
	for _, output := range outputs {
		if err := run([]string{
			"build-package", "--repo", repo, "--tag", tag,
			"--package-dir", packageDir, "--output", output, "--tooling-commit", testToolingCommit, "--allow-untagged-candidate",
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
	if manifest.ToolingCommit != testToolingCommit || manifest.PublisherPolicy.ToolingCommit != testToolingCommit {
		t.Fatalf("release tooling commit is not pinned in the manifest and publisher policy: %+v", manifest)
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
	files, ok := sbom["files"].([]any)
	if !ok {
		t.Fatalf("SBOM files are missing: %+v", sbom)
	}
	relationships, ok := sbom["relationships"].([]any)
	if !ok || len(relationships) != len(files)+1 {
		t.Fatalf("SBOM relationship closure has %d relationships for %d files", len(relationships), len(files))
	}
	for index, rawFile := range files {
		file := rawFile.(map[string]any)
		relationship := relationships[index+1].(map[string]any)
		if relationship["spdxElementId"] != "SPDXRef-Package" || relationship["relationshipType"] != "CONTAINS" || relationship["relatedSpdxElement"] != file["SPDXID"] {
			t.Fatalf("SBOM file %d is not deterministically contained by the package: file=%+v relationship=%+v", index, file, relationship)
		}
	}
	validateSPDX(t, sbomPath)
	var provenance statement
	readJSON(t, filepath.Join(outputs[0], baseName+"_provenance.intoto.json"), &provenance)
	if provenance.Type != "https://in-toto.io/Statement/v1" || provenance.PredicateType != "https://slsa.dev/provenance/v1" || len(provenance.Subject) != 2 {
		t.Fatalf("unexpected provenance: %+v", provenance)
	}
}

func TestFinalizeRequiresTransparencyEvidence(t *testing.T) {
	repo, packageDir, report := stageFamilyPackage(t)
	releaseID := formpackage.ReleaseIDForGroupKind(testFamilyGroup, "ModuleWorker")
	artifactID := strings.Replace(report.PackageDigest, ":", "-", 1)
	tag := "forms/" + releaseID + "/" + artifactID
	baseName := "takoform-form-" + releaseID + "_" + artifactID
	output := filepath.Join(t.TempDir(), "release")
	if err := run([]string{
		"build-package", "--repo", repo, "--tag", tag,
		"--package-dir", packageDir, "--output", output, "--tooling-commit", testToolingCommit, "--allow-untagged-candidate",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	bundleName := baseName + "_package-index.sigstore.json"
	invalid := `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{"tlogEntries":[]},"messageSignature":{}}`
	if err := os.WriteFile(filepath.Join(output, bundleName), []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"finalize-bundle", "--output", output}, io.Discard); err == nil || !strings.Contains(err.Error(), "transparency-log") {
		t.Fatalf("expected missing transparency proof rejection, got %v", err)
	}
	legacyNested := `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{"certificate":{"rawBytes":"AA=="},"tlogEntries":[{"logIndex":"1"}]},"content":{"messageSignature":{"messageDigest":{"algorithm":"SHA2_256","digest":"AA=="},"signature":"AA=="}}}`
	if err := os.WriteFile(filepath.Join(output, bundleName), []byte(legacyNested), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"finalize-bundle", "--output", output}, io.Discard); err == nil || !strings.Contains(err.Error(), "message signature") {
		t.Fatalf("expected nested non-v0.3 message signature rejection, got %v", err)
	}
	valid := `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{"certificate":{"rawBytes":"AA=="},"tlogEntries":[{"logIndex":"1"}]},"messageSignature":{"messageDigest":{"algorithm":"SHA2_256","digest":"AA=="},"signature":"AA=="}}`
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

func TestValidateSigstoreBundleAcceptsActualCosignV3MessageSignature(t *testing.T) {
	fixture := filepath.Join("testdata", "cosign-v3.0.6-message-signature.sigstore.json")
	if err := validateSigstoreBundle(fixture); err != nil {
		t.Fatalf("actual Cosign v3.0.6 bundle was rejected: %v", err)
	}
	var bundle map[string]any
	readJSON(t, fixture, &bundle)
	if _, ok := bundle["messageSignature"].(map[string]any); !ok {
		t.Fatal("actual Cosign v3.0.6 fixture lacks its top-level messageSignature")
	}
	if _, legacyNested := bundle["content"]; legacyNested {
		t.Fatal("actual Cosign v3.0.6 fixture unexpectedly has a nested content object")
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
		"--output", output, "--tooling-commit", testToolingCommit, "--allow-untagged-candidate",
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

func TestVerifyRevocationDirectoryAcceptsExactSixAssetSemanticClosure(t *testing.T) {
	fixture := newRevocationVerificationFixture(t)
	var output bytes.Buffer
	if err := run([]string{
		"verify-revocation-directory",
		"--asset-root", fixture.assetRoot,
		"--source-root", fixture.repo,
		"--tag", fixture.tag,
		"--source-commit", fixture.commit,
		"--tooling-commit", fixture.commit,
		"--trusted-root", fixture.trustedRoot,
	}, &output); err != nil {
		t.Fatalf("verify revocation directory: %v", err)
	}
	raw := bytes.TrimSuffix(output.Bytes(), []byte("\n"))
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, canonical) {
		t.Fatal("revocation verification report is not canonical JSON")
	}
	var report revocationDirectoryVerification
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	trustedRaw, err := os.ReadFile(fixture.trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	trustedAbsolute, err := filepath.Abs(fixture.trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if report.Format != revocationVerificationFormat ||
		report.SemanticStatus != "verified" ||
		report.CryptographicStatus != "external-required" ||
		report.Tag != fixture.tag || report.Version != "1.0.0" ||
		report.SourceCommit != fixture.commit || report.ToolingCommit != fixture.commit ||
		report.CheckpointSequence != 1 || !formpackage.ValidDigest(report.CheckpointDigest) ||
		!formpackage.ValidDigest(report.PackageDigest) ||
		report.FormRef.Kind != "ObjectBucket" ||
		report.TrustedRoot.Path != trustedAbsolute ||
		report.TrustedRoot.SHA256 != formpackage.DigestBytes(trustedRaw) ||
		len(report.Assets) != 6 {
		t.Fatalf("revocation verification report drifted: %+v", report)
	}
	lastName := ""
	for _, asset := range report.Assets {
		retained, err := os.ReadFile(filepath.Join(fixture.assetRoot, asset.Name))
		if err != nil {
			t.Fatal(err)
		}
		if asset.Name <= lastName || asset.Size != int64(len(retained)) ||
			asset.SHA256 != formpackage.DigestBytes(retained) {
			t.Fatalf("invalid ordered revocation report asset: %+v", asset)
		}
		lastName = asset.Name
	}
}

func TestVerifyRevocationDirectoryRejectsResealedStatementDrift(t *testing.T) {
	fixture := newRevocationVerificationFixture(t)
	statementPath := filepath.Join(fixture.assetRoot, fixture.base+"_statement.json")
	raw, err := os.ReadFile(statementPath)
	if err != nil {
		t.Fatal(err)
	}
	var statement map[string]any
	if err := json.Unmarshal(raw, &statement); err != nil {
		t.Fatal(err)
	}
	statement["summary"] = "resealed but not repository-backed"
	mutated, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	mutated, err = formpackage.Canonicalize(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statementPath, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
	resealRevocationRelease(t, fixture)

	_, err = verifyRevocationDirectory(
		fixture.repo, fixture.assetRoot, fixture.tag, fixture.commit, fixture.commit, fixture.trustedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "released statement differs") {
		t.Fatalf("resealed revocation statement drift error = %v", err)
	}
}

func TestVerifyRevocationDirectoryUsesHistoricalToolingTreeAfterAppend(t *testing.T) {
	fixture := newRevocationVerificationFixture(t)
	fixtureDir := filepath.Join(repositoryRoot(t), "conformance", "revocation-checkpoint-v1", "positive")
	copyFileForTest(
		t,
		filepath.Join(fixtureDir, "statement-2.json"),
		filepath.Join(fixture.repo, "forms", "revocations", "1.1.0.json"),
	)
	copyFileForTest(
		t,
		filepath.Join(fixtureDir, "checkpoint-2.json"),
		filepath.Join(fixture.repo, "forms", "revocations", "checkpoints", "1.1.0.json"),
	)
	var currentTrustedRoot any
	trustedRaw, err := os.ReadFile(fixture.trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(trustedRaw, &currentTrustedRoot); err != nil {
		t.Fatal(err)
	}
	rotatedFormatting, err := json.MarshalIndent(currentTrustedRoot, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	rotatedFormatting = append(rotatedFormatting, '\n')
	if bytes.Equal(trustedRaw, rotatedFormatting) {
		t.Fatal("trusted-root fixture formatting did not change")
	}
	if err := os.WriteFile(fixture.trustedRoot, rotatedFormatting, 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, fixture.repo, "append revocation and reformat trusted root")
	historicalTrustedRoot := filepath.Join(t.TempDir(), "trusted-root.json")
	if err := os.WriteFile(historicalTrustedRoot, trustedRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := verifyRevocationDirectory(
		fixture.repo, fixture.assetRoot, fixture.tag,
		fixture.commit, fixture.commit, historicalTrustedRoot,
	)
	if err != nil {
		t.Fatalf("historical revocation closure rejected after later append: %v", err)
	}
	logicalTrustedRootAbsolute, err := filepath.Abs(fixture.trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if report.CheckpointSequence != 1 ||
		report.TrustedRoot.Path != logicalTrustedRootAbsolute ||
		report.TrustedRoot.SHA256 != formpackage.DigestBytes(trustedRaw) ||
		report.TrustedRoot.SHA256 == formpackage.DigestBytes(rotatedFormatting) {
		t.Fatalf("historical report did not bind tooling-commit source/root: %+v", report)
	}

	_, err = verifyRevocationDirectory(
		fixture.repo, fixture.assetRoot, fixture.tag,
		fixture.commit, fixture.commit, fixture.trustedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "trusted-root bytes differ from the exact tooling commit") {
		t.Fatalf("later substituted trusted-root error = %v", err)
	}

	statementPath := filepath.Join(fixture.assetRoot, fixture.base+"_statement.json")
	var statement map[string]any
	readJSON(t, statementPath, &statement)
	statement["summary"] = "resealed after a later source append"
	if err := writeCanonicalJSON(statementPath, statement); err != nil {
		t.Fatal(err)
	}
	resealRevocationRelease(t, fixture)
	_, err = verifyRevocationDirectory(
		fixture.repo, fixture.assetRoot, fixture.tag,
		fixture.commit, fixture.commit, historicalTrustedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "released statement differs") {
		t.Fatalf("historical reseal drift error = %v", err)
	}
}

func TestVerifyRevocationDirectoryRejectsMalformedChainAndTrustedRoot(t *testing.T) {
	t.Run("forked source chain", func(t *testing.T) {
		fixture := newRevocationVerificationFixture(t)
		checkpointPath := filepath.Join(
			fixture.repo, "forms", "revocations", "checkpoints", "1.0.0.json",
		)
		raw, err := os.ReadFile(checkpointPath)
		if err != nil {
			t.Fatal(err)
		}
		var checkpoint map[string]any
		if err := json.Unmarshal(raw, &checkpoint); err != nil {
			t.Fatal(err)
		}
		entries := checkpoint["entries"].([]any)
		entries[0].(map[string]any)["statementDigest"] = "sha256:" + strings.Repeat("e", 64)
		forked, err := json.Marshal(checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(checkpointPath, forked, 0o644); err != nil {
			t.Fatal(err)
		}
		gitCommitAll(t, fixture.repo, "fork checkpoint")
		forkCommit := strings.TrimSpace(runCommand(t, fixture.repo, "git", "rev-parse", "HEAD"))
		_, err = verifyRevocationDirectory(
			fixture.repo, fixture.assetRoot, fixture.tag, forkCommit, forkCommit, fixture.trustedRoot,
		)
		if err == nil || !strings.Contains(err.Error(), "revocation chain") {
			t.Fatalf("forked revocation chain error = %v", err)
		}
	})

	t.Run("malformed retained trusted root", func(t *testing.T) {
		fixture := newRevocationVerificationFixture(t)
		if err := os.WriteFile(fixture.trustedRoot, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitCommitAll(t, fixture.repo, "malformed trusted root")
		toolingCommit := strings.TrimSpace(runCommand(t, fixture.repo, "git", "rev-parse", "HEAD"))
		_, err := verifyRevocationDirectory(
			fixture.repo, fixture.assetRoot, fixture.tag,
			fixture.commit, toolingCommit, fixture.trustedRoot,
		)
		if err == nil || !strings.Contains(err.Error(), "decode Sigstore trusted root") {
			t.Fatalf("malformed trusted-root error = %v", err)
		}
	})
}

func TestVerifyRevocationDirectoryRejectsSymlinkedOrExtraAsset(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		fixture := newRevocationVerificationFixture(t)
		checksums := filepath.Join(fixture.assetRoot, "SHA256SUMS")
		if err := os.Remove(checksums); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("release-manifest.json", checksums); err != nil {
			t.Fatal(err)
		}
		_, err := verifyRevocationDirectory(
			fixture.repo, fixture.assetRoot, fixture.tag,
			fixture.commit, fixture.commit, fixture.trustedRoot,
		)
		if err == nil || !strings.Contains(err.Error(), "regular file, not a symlink") {
			t.Fatalf("symlinked revocation asset error = %v", err)
		}
	})

	t.Run("extra", func(t *testing.T) {
		fixture := newRevocationVerificationFixture(t)
		if err := os.WriteFile(filepath.Join(fixture.assetRoot, "extra.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := verifyRevocationDirectory(
			fixture.repo, fixture.assetRoot, fixture.tag,
			fixture.commit, fixture.commit, fixture.trustedRoot,
		)
		if err == nil || !strings.Contains(err.Error(), "exact six-asset closure") {
			t.Fatalf("extra revocation asset error = %v", err)
		}
	})
}

type revocationVerificationFixture struct {
	repo        string
	assetRoot   string
	tag         string
	commit      string
	trustedRoot string
	base        string
}

func newRevocationVerificationFixture(t *testing.T) revocationVerificationFixture {
	t.Helper()
	repo := makeTestRepo(t)
	revocationDir := filepath.Join(repo, "forms", "revocations")
	checkpointDir := filepath.Join(revocationDir, "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(repositoryRoot(t), "conformance", "revocation-checkpoint-v1", "positive")
	copyFileForTest(t, filepath.Join(fixtureDir, "statement-1.json"), filepath.Join(revocationDir, "1.0.0.json"))
	copyFileForTest(t, filepath.Join(fixtureDir, "checkpoint-1.json"), filepath.Join(checkpointDir, "1.0.0.json"))
	trustedRoot := filepath.Join(repo, filepath.FromSlash(trustedRootPath))
	copyFileForTest(
		t,
		filepath.Join(repositoryRoot(t), filepath.FromSlash(trustedRootPath)),
		trustedRoot,
	)
	gitCommitAll(t, repo, "revocation verification source")
	commit := strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	tag := "forms/revocations/v1.0.0"
	runCommand(t, repo, "git", "tag", tag)
	assetRoot := filepath.Join(t.TempDir(), "revocation-release")
	if err := run([]string{
		"build-revocation", "--repo", repo, "--tag", tag,
		"--output", assetRoot, "--tooling-commit", commit,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	base := "takoform-form-revocation_1.0.0"
	checkpointRaw, err := os.ReadFile(filepath.Join(assetRoot, base+"_checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	bundleRaw := buildRevocationSigstoreFixture(t, checkpointRaw)
	if err := os.WriteFile(filepath.Join(assetRoot, base+"_checkpoint.sigstore.json"), bundleRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"finalize-bundle", "--output", assetRoot}, io.Discard); err != nil {
		t.Fatal(err)
	}
	return revocationVerificationFixture{
		repo: repo, assetRoot: assetRoot, tag: tag, commit: commit,
		trustedRoot: trustedRoot, base: base,
	}
}

func buildRevocationSigstoreFixture(t *testing.T, subject []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(subject)
	value := map[string]any{
		"mediaType": bundleMediaType,
		"verificationMaterial": map[string]any{
			"certificate": map[string]string{
				"rawBytes": base64.StdEncoding.EncodeToString([]byte("fixture-certificate")),
			},
			"tlogEntries": []any{map[string]any{
				"canonicalizedBody": base64.StdEncoding.EncodeToString([]byte("fixture-tlog-body")),
				"inclusionProof": map[string]any{
					"logIndex": "1", "treeSize": "1",
					"rootHash": base64.StdEncoding.EncodeToString(make([]byte, sha256.Size)),
					"hashes":   []any{},
					"checkpoint": map[string]string{
						"envelope": "fixture checkpoint",
					},
				},
			}},
		},
		"messageSignature": map[string]any{
			"messageDigest": map[string]string{
				"algorithm": "SHA2_256",
				"digest":    base64.StdEncoding.EncodeToString(digest[:]),
			},
			"signature": base64.StdEncoding.EncodeToString([]byte("fixture-signature")),
		},
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func resealRevocationRelease(t *testing.T, fixture revocationVerificationFixture) {
	t.Helper()
	manifestPath := filepath.Join(fixture.assetRoot, "release-manifest.json")
	var manifest releaseManifest
	readJSON(t, manifestPath, &manifest)
	checkpointName := fixture.base + "_checkpoint.json"
	statementName := fixture.base + "_statement.json"
	for index := range manifest.Assets {
		if manifest.Assets[index].Name == fixture.base+"_provenance.intoto.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(fixture.assetRoot, manifest.Assets[index].Name))
		if err != nil {
			t.Fatal(err)
		}
		manifest.Assets[index].Size = int64(len(raw))
		manifest.Assets[index].Digest = formpackage.DigestBytes(raw)
	}
	byName := make(map[string]releaseAsset, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		byName[asset.Name] = asset
	}
	provenance := createProvenance(
		manifest.Tag, manifest.Workflow, manifest.SourceCommit, manifest.ToolingCommit,
		[]releaseAsset{byName[checkpointName], byName[statementName]},
	)
	if err := writeCanonicalJSON(
		filepath.Join(fixture.assetRoot, fixture.base+"_provenance.intoto.json"),
		provenance,
	); err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Assets {
		raw, err := os.ReadFile(filepath.Join(fixture.assetRoot, manifest.Assets[index].Name))
		if err != nil {
			t.Fatal(err)
		}
		manifest.Assets[index].Size = int64(len(raw))
		manifest.Assets[index].Digest = formpackage.DigestBytes(raw)
	}
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	if err := writeJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	if err := writeChecksums(fixture.assetRoot); err != nil {
		t.Fatal(err)
	}
}

func copyFileForTest(t *testing.T, source, destination string) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, raw, 0o644); err != nil {
		t.Fatal(err)
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
