package projectpolicy

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type trustProfile struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Provider      struct {
		Status            string `json:"status"`
		ArtifactDigest    string `json:"artifactDigest"`
		ReleaseTagPattern string `json:"releaseTagPattern"`
		Signature         struct {
			Format      string `json:"format"`
			SignedAsset string `json:"signedAsset"`
			Fingerprint string `json:"fingerprint"`
		} `json:"signature"`
		Provenance struct {
			Envelope         string `json:"envelope"`
			Predicate        string `json:"predicate"`
			Canonicalization string `json:"canonicalization"`
			PayloadSubjects  struct {
				Count      int      `json:"count"`
				Fields     []string `json:"fields"`
				Included   []string `json:"included"`
				Excluded   []string `json:"excluded"`
				Derivation string   `json:"derivation"`
			} `json:"payloadSubjects"`
			BuildBindings []string `json:"buildBindings"`
			Signature     struct {
				Format      string `json:"format"`
				SignedAsset string `json:"signedAsset"`
				Fingerprint string `json:"fingerprint"`
			} `json:"signature"`
			PublicInventoryAssetCount int `json:"publicInventoryAssetCount"`
		} `json:"provenance"`
		Distribution struct {
			Source                   string `json:"source"`
			Registry                 string `json:"registry"`
			OverwriteExistingVersion bool   `json:"overwriteExistingVersion"`
		} `json:"distribution"`
	} `json:"provider"`
	FormPackage struct {
		Status           string `json:"status"`
		Canonicalization struct {
			Format   string   `json:"format"`
			Encoding string   `json:"encoding"`
			Reject   []string `json:"reject"`
		} `json:"canonicalization"`
		Identity struct {
			Digest                  string `json:"digest"`
			Subject                 string `json:"subject"`
			ArchiveBytesAreIdentity bool   `json:"archiveBytesAreIdentity"`
		} `json:"identity"`
		Signature struct {
			Format        string `json:"format"`
			Mode          string `json:"mode"`
			SignedSubject string `json:"signedSubject"`
		} `json:"signature"`
		PublisherPolicy struct {
			OIDCIssuer       string `json:"oidcIssuer"`
			SourceRepository string `json:"sourceRepository"`
			Workflow         string `json:"workflow"`
			WorkflowIdentity string `json:"workflowIdentity"`
			TagPattern       string `json:"tagPattern"`
		} `json:"publisherPolicy"`
		TagProtection struct {
			TagPattern             string `json:"tagPattern"`
			RestrictCreation       bool   `json:"restrictCreation"`
			RestrictDeletion       bool   `json:"restrictDeletion"`
			RestrictNonFastForward bool   `json:"restrictNonFastForward"`
			ReleaseEnvironment     string `json:"releaseEnvironment"`
			DeploymentRef          string `json:"deploymentRef"`
		} `json:"tagProtection"`
		Transparency struct {
			Authority                         string `json:"authority"`
			RequiredEvidence                  string `json:"requiredEvidence"`
			OfflineBundleVerificationRequired bool   `json:"offlineBundleVerificationRequired"`
		} `json:"transparency"`
		Provenance struct {
			Envelope  string `json:"envelope"`
			Predicate string `json:"predicate"`
			SBOM      string `json:"sbom"`
		} `json:"provenance"`
		Distribution struct {
			Status                   string `json:"status"`
			InitialSource            string `json:"initialSource"`
			MirrorPolicy             string `json:"mirrorPolicy"`
			CustomerRequestFetch     bool   `json:"customerRequestFetch"`
			OverwriteExistingVersion bool   `json:"overwriteExistingVersion"`
		} `json:"distribution"`
		Revocation struct {
			Status           string `json:"status"`
			Metadata         string `json:"metadata"`
			Workflow         string `json:"workflow"`
			WorkflowIdentity string `json:"workflowIdentity"`
			TagPattern       string `json:"tagPattern"`
			Checkpoint       struct {
				SignedSubject            string   `json:"signedSubject"`
				Cumulative               bool     `json:"cumulative"`
				SequenceStartsAt         uint64   `json:"sequenceStartsAt"`
				PreviousCheckpointDigest string   `json:"previousCheckpointDigest"`
				HostPinRequired          bool     `json:"hostPinRequired"`
				HostPinFields            []string `json:"hostPinFields"`
			} `json:"checkpoint"`
			BlockNewCreateOrUpdate                   bool `json:"blockNewCreateOrUpdate"`
			RetainReferencedBytesForObserveAndDelete bool `json:"retainReferencedBytesForObserveAndDelete"`
			ReplaceBytesInPlace                      bool `json:"replaceBytesInPlace"`
		} `json:"revocation"`
		ContentPolicy struct {
			DataOnly                         bool `json:"dataOnly"`
			AllowSymlinks                    bool `json:"allowSymlinks"`
			AllowExecutableFiles             bool `json:"allowExecutableFiles"`
			AllowCredentialsOrOperatorFields bool `json:"allowCredentialsOrOperatorFields"`
		} `json:"contentPolicy"`
	} `json:"formPackage"`
	RunnerReport struct {
		HostFormat           string `json:"hostFormat"`
		ProviderFormat       string `json:"providerFormat"`
		ProviderBinaryDigest string `json:"providerBinaryDigest"`
		UnknownFields        string `json:"unknownFields"`
	} `json:"runnerReport"`
	Separation struct {
		ReuseProviderGPGKeyForFormPackages bool `json:"reuseProviderGPGKeyForFormPackages"`
		ReuseForTakosumiLegacyProvider     bool `json:"reuseForTakosumiLegacyProvider"`
	} `json:"separation"`
}

type releaseDescriptor struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Version          string `json:"version"`
	Tag              string `json:"tag"`
	SourceRepository string `json:"sourceRepository"`
	ProviderAddress  string `json:"providerAddress"`
	CLIMatrix        []struct {
		Product         string `json:"product"`
		Version         string `json:"version"`
		ProviderAddress string `json:"providerAddress"`
	} `json:"cliMatrix"`
	GoModule           string   `json:"goModule"`
	GoVersion          string   `json:"goVersion"`
	SigningFingerprint string   `json:"signingFingerprint"`
	PublicKeyPath      string   `json:"publicKeyPath"`
	Platforms          []string `json:"platforms"`
	PublicationStatus  string   `json:"publicationStatus"`
	Versioning         struct {
		ProviderCompatibility  string `json:"providerCompatibility"`
		PortableAPIVersion     string `json:"portableApiVersion"`
		FormDefinitionVersions string `json:"formDefinitionVersions"`
		FormPackageVersions    string `json:"formPackageVersions"`
		AdmissionGenerations   string `json:"admissionGenerations"`
	} `json:"versioning"`
}

func TestD08TrustProfileRemainsFailClosedAndSeparated(t *testing.T) {
	root := repositoryRoot(t)
	var profile trustProfile
	readStrictJSON(t, filepath.Join(root, "spec", "trust", "profile.json"), &profile)
	var release releaseDescriptor
	readStrictJSON(t, filepath.Join(root, "release", "version.json"), &release)

	if profile.SchemaVersion != 2 || profile.Status != "decision-approved-implementation-in-progress" {
		t.Fatalf("unexpected trust profile identity: version=%d status=%q", profile.SchemaVersion, profile.Status)
	}
	if profile.Provider.Status != "implemented-registry-key-registered-first-install-proof-pending" ||
		profile.Provider.ArtifactDigest != "sha256" || profile.Provider.ReleaseTagPattern != "v*" {
		t.Fatalf("provider trust lane is not fail-closed: %#v", profile.Provider)
	}
	if profile.Provider.Signature.Format != "openpgp-detached-binary" ||
		!strings.Contains(profile.Provider.Signature.SignedAsset, "SHA256SUMS") ||
		profile.Provider.Signature.Fingerprint != release.SigningFingerprint {
		t.Fatalf("provider signing profile differs from release descriptor")
	}
	providerAssetBase := "terraform-provider-takoform_<version>_"
	wantPayloadSubjects := make([]string, 0, len(release.Platforms)*2+3)
	for _, platform := range release.Platforms {
		wantPayloadSubjects = append(wantPayloadSubjects, providerAssetBase+platform+".zip")
	}
	for _, platform := range release.Platforms {
		wantPayloadSubjects = append(wantPayloadSubjects, providerAssetBase+platform+".zip.spdx.json")
	}
	wantPayloadSubjects = append(wantPayloadSubjects,
		providerAssetBase+"manifest.json",
		providerAssetBase+"SHA256SUMS",
		providerAssetBase+"SHA256SUMS.sig",
	)
	if profile.Provider.Provenance.Envelope != "in-toto-statement-v1" ||
		profile.Provider.Provenance.Predicate != "slsa-provenance-v1" ||
		profile.Provider.Provenance.Canonicalization != "RFC8785" ||
		profile.Provider.Provenance.PayloadSubjects.Count != 13 ||
		profile.Provider.Provenance.PayloadSubjects.Count != len(profile.Provider.Provenance.PayloadSubjects.Included) ||
		strings.Join(profile.Provider.Provenance.PayloadSubjects.Fields, ",") !=
			"name,digest.sha256,annotations.size" ||
		strings.Join(profile.Provider.Provenance.PayloadSubjects.Included, ",") != strings.Join(wantPayloadSubjects, ",") ||
		strings.Join(profile.Provider.Provenance.PayloadSubjects.Excluded, ",") !=
			"terraform-provider-takoform_<version>_provenance.intoto.json,terraform-provider-takoform_<version>_provenance.intoto.json.sig" ||
		profile.Provider.Provenance.PayloadSubjects.Derivation != "public-inventory-minus-provenance-and-signature" ||
		strings.Join(profile.Provider.Provenance.BuildBindings, ",") !=
			"buildDefinition.externalParameters.tag,buildDefinition.externalParameters.requestId,"+
				"buildDefinition.internalParameters.sourceCommit,buildDefinition.internalParameters.toolingCommit,"+
				"buildDefinition.internalParameters.workflow.path,buildDefinition.internalParameters.workflow.ref,"+
				"buildDefinition.internalParameters.run.id,buildDefinition.internalParameters.run.attempt,"+
				"buildDefinition.internalParameters.tagObject.oid,buildDefinition.internalParameters.tagObject.sha256,"+
				"runDetails.metadata.invocationId" ||
		profile.Provider.Provenance.Signature.Format != "openpgp-detached-binary" ||
		profile.Provider.Provenance.Signature.SignedAsset != "terraform-provider-takoform_<version>_provenance.intoto.json" ||
		profile.Provider.Provenance.Signature.Fingerprint != profile.Provider.Signature.Fingerprint ||
		profile.Provider.Provenance.PublicInventoryAssetCount != 15 ||
		profile.Provider.Provenance.PublicInventoryAssetCount != profile.Provider.Provenance.PayloadSubjects.Count+2 {
		t.Fatalf("provider provenance profile is incomplete")
	}
	if profile.Provider.Distribution.Source != "github-release" ||
		profile.Provider.Distribution.Registry != "registry.terraform.io/tako0614/takoform" ||
		profile.Provider.Distribution.OverwriteExistingVersion {
		t.Fatalf("provider distribution is mutable or has the wrong registry")
	}
	if release.Version != "1.0.4" || release.Tag != "v1.0.4" ||
		release.Versioning.ProviderCompatibility != "semver-major" ||
		release.Versioning.PortableAPIVersion != "forms.takoform.com/v1alpha1" ||
		release.Versioning.FormDefinitionVersions != "independent-immutable-semver" ||
		release.Versioning.FormPackageVersions != "independent-immutable-semver" ||
		release.Versioning.AdmissionGenerations != "independent-non-semver" {
		t.Fatalf("provider v1 version streams are not independently locked: %#v", release.Versioning)
	}
	if profile.RunnerReport.HostFormat != "takoform.standard-runner-report@v1" ||
		profile.RunnerReport.ProviderFormat != "takoform.standard-provider-runner-report@v2" ||
		profile.RunnerReport.ProviderBinaryDigest != "sha256" ||
		profile.RunnerReport.UnknownFields != "reject" {
		t.Fatalf("runner-report contract lock is invalid: %#v", profile.RunnerReport)
	}

	packageTrust := profile.FormPackage
	if packageTrust.Status != "data-contract-verifier-and-keyless-release-lane-implemented-first-release-pending" ||
		packageTrust.Canonicalization.Format != "RFC8785" ||
		packageTrust.Canonicalization.Encoding != "UTF-8 I-JSON" ||
		packageTrust.Identity.Digest != "sha256" ||
		packageTrust.Identity.Subject != "RFC8785 bytes of the package index" ||
		packageTrust.Identity.ArchiveBytesAreIdentity {
		t.Fatalf("unexpected Form Package identity policy: %#v", packageTrust)
	}
	wantReject := []string{"duplicate-object-name", "invalid-unicode", "nan-or-infinity", "negative-zero"}
	if strings.Join(packageTrust.Canonicalization.Reject, ",") != strings.Join(wantReject, ",") {
		t.Fatalf("canonicalization rejection policy changed: %#v", packageTrust.Canonicalization.Reject)
	}
	if packageTrust.Signature.Format != "application/vnd.dev.sigstore.bundle.v0.3+json" ||
		packageTrust.Signature.Mode != "keyless-blob-signature" ||
		packageTrust.Signature.SignedSubject != packageTrust.Identity.Subject ||
		packageTrust.PublisherPolicy.OIDCIssuer != "https://token.actions.githubusercontent.com" ||
		packageTrust.PublisherPolicy.SourceRepository != "github.com/tako0614/terraform-provider-takoform" ||
		packageTrust.PublisherPolicy.Workflow != ".github/workflows/form-package-release.yml" ||
		packageTrust.PublisherPolicy.WorkflowIdentity != "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main" ||
		packageTrust.PublisherPolicy.TagPattern != "refs/tags/forms/k-*/v*" {
		t.Fatalf("unexpected Form Package publisher policy")
	}
	if packageTrust.TagProtection.TagPattern != "refs/tags/forms/*/v*" ||
		!packageTrust.TagProtection.RestrictCreation || !packageTrust.TagProtection.RestrictDeletion ||
		!packageTrust.TagProtection.RestrictNonFastForward || packageTrust.TagProtection.ReleaseEnvironment != "form-package-release" ||
		packageTrust.TagProtection.DeploymentRef != "refs/heads/main" {
		t.Fatalf("Form Package protected-tag policy is incomplete")
	}
	if packageTrust.Transparency.Authority != "sigstore-public-good-instance" ||
		!packageTrust.Transparency.OfflineBundleVerificationRequired ||
		!strings.Contains(packageTrust.Transparency.RequiredEvidence, "inclusion proof") {
		t.Fatalf("Form Package transparency evidence is incomplete")
	}
	if packageTrust.Provenance.Envelope != "in-toto-statement-v1" ||
		packageTrust.Provenance.Predicate != "slsa-provenance-v1" ||
		packageTrust.Provenance.SBOM != "spdx-2.3" {
		t.Fatalf("Form Package provenance profile is incomplete")
	}
	if packageTrust.Distribution.Status != "github-release-workflow-implemented-first-release-pending" ||
		packageTrust.Distribution.InitialSource != "github-release" ||
		!strings.Contains(packageTrust.Distribution.MirrorPolicy, "exact assets") ||
		packageTrust.Distribution.CustomerRequestFetch || packageTrust.Distribution.OverwriteExistingVersion ||
		packageTrust.Revocation.Status != "signed-append-only-delivery-lane-implemented-no-statements-published" ||
		!strings.Contains(packageTrust.Revocation.Metadata, "exact package digest") ||
		packageTrust.Revocation.Workflow != ".github/workflows/form-package-revocation.yml" ||
		packageTrust.Revocation.WorkflowIdentity != "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-revocation.yml@refs/heads/main" ||
		packageTrust.Revocation.TagPattern != "refs/tags/forms/revocations/v*" ||
		packageTrust.Revocation.Checkpoint.SignedSubject != "RFC8785 bytes of the cumulative revocation checkpoint" ||
		!packageTrust.Revocation.Checkpoint.Cumulative || packageTrust.Revocation.Checkpoint.SequenceStartsAt != 1 ||
		packageTrust.Revocation.Checkpoint.PreviousCheckpointDigest != "sha256" || !packageTrust.Revocation.Checkpoint.HostPinRequired ||
		strings.Join(packageTrust.Revocation.Checkpoint.HostPinFields, ",") != "sequence,checkpointDigest,cumulativeEntriesDigest" ||
		!packageTrust.Revocation.BlockNewCreateOrUpdate ||
		!packageTrust.Revocation.RetainReferencedBytesForObserveAndDelete ||
		packageTrust.Revocation.ReplaceBytesInPlace ||
		!packageTrust.ContentPolicy.DataOnly || packageTrust.ContentPolicy.AllowSymlinks ||
		packageTrust.ContentPolicy.AllowExecutableFiles || packageTrust.ContentPolicy.AllowCredentialsOrOperatorFields {
		t.Fatalf("Form Package install/revocation/content policy is unsafe")
	}
	if profile.Separation.ReuseProviderGPGKeyForFormPackages || profile.Separation.ReuseForTakosumiLegacyProvider {
		t.Fatal("release trust lanes must remain independent")
	}
}

func TestReleaseWorkflowsPinTheReviewedCLIMatrix(t *testing.T) {
	root := repositoryRoot(t)
	var release releaseDescriptor
	readStrictJSON(t, filepath.Join(root, "release", "version.json"), &release)

	versions := make(map[string]string, len(release.CLIMatrix))
	for _, entry := range release.CLIMatrix {
		if entry.Version == "" {
			t.Fatalf("release CLI matrix has no version for %s", entry.Product)
		}
		versions[entry.Product] = entry.Version
	}
	if len(versions) != 2 || versions["OpenTofu"] == "" || versions["Terraform"] == "" {
		t.Fatalf("release CLI matrix is incomplete: %#v", versions)
	}

	workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	tools := []struct {
		product string
		action  string
		key     string
	}{
		{product: "OpenTofu", action: "opentofu/setup-opentofu@", key: "tofu_version"},
		{product: "Terraform", action: "hashicorp/setup-terraform@", key: "terraform_version"},
	}
	for _, workflowPath := range workflows {
		workflow := readText(t, workflowPath)
		for _, tool := range tools {
			actionCount := strings.Count(workflow, tool.action)
			if actionCount == 0 {
				continue
			}
			want := tool.key + ": " + versions[tool.product]
			versionCount := 0
			for _, line := range strings.Split(workflow, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, tool.key+":") {
					continue
				}
				versionCount++
				if line != want {
					t.Errorf(
						"%s pins %q, want the reviewed %q",
						filepath.Base(workflowPath),
						line,
						want,
					)
				}
			}
			if versionCount != actionCount {
				t.Errorf(
					"%s has %d %s setup actions but %d exact version pins",
					filepath.Base(workflowPath),
					actionCount,
					tool.product,
					versionCount,
				)
			}
		}
	}
}

func TestReleaseWorkflowsOnlyPrepareChecksumClosedCandidates(t *testing.T) {
	root := repositoryRoot(t)
	workflows := []struct {
		name         string
		path         string
		environment  string
		artifact     string
		signing      string
		executionRef string
		required     []string
	}{
		{
			name:         "provider",
			path:         ".github/workflows/release.yml",
			environment:  "provider-release",
			artifact:     "provider-release-candidate-${{ github.run_id }}-${{ github.run_attempt }}",
			signing:      "gpg --batch --local-user",
			executionRef: `refs/tags/$RELEASE_TAG`,
			required: []string{
				"takoform.provider-release-candidate.v1",
				`candidate="$RUNNER_TEMP/provider-release-candidate"`,
				`cd "$candidate"`,
				`test "$(wc -l < "$RUNNER_TEMP/candidate-assets.txt")" -eq 15`,
				`test "$(jq -r '.assetCount' "$candidate/metadata.json")" -eq 15`,
				`provenance="${base}_provenance.intoto.json"`,
				`provenance_signature="${provenance}.sig"`,
				`test "$(wc -l < SHA256SUMS)" -eq 16`,
				"expected-candidate-root.txt",
				"actual-candidate-root.txt",
				`diff -u "$RUNNER_TEMP/expected-candidate-root.txt" "$RUNNER_TEMP/actual-candidate-root.txt"`,
				`echo "path=$candidate" >> "$GITHUB_OUTPUT"`,
				"path: ${{ steps.candidate.outputs.path }}",
				"gpg --batch --verify",
				"refusing overwrite",
				"Build the second and final unsigned release inventory",
				"Exercise the exact final linux amd64 provider bytes",
				`--provider-binary "$binary"`,
			},
		},
		{
			name:         "Form Package",
			path:         ".github/workflows/form-package-release.yml",
			environment:  "form-package-release",
			artifact:     "form-package-release-candidate-${{ github.run_id }}-${{ github.run_attempt }}",
			signing:      "cosign sign-blob",
			executionRef: "refs/heads/main",
			required: []string{
				"id-token: write",
				"takoform.form-package-release-candidate@v1",
				`candidate="$RUNNER_TEMP/form-package-release-candidate"`,
				`output="$candidate/assets"`,
				`cd "$candidate"`,
				"cosign sign-blob",
				"cosign verify-blob",
				"sha256sum metadata.json tag-object",
				`test "$(wc -l < SHA256SUMS)" -eq 9`,
				"path: ${{ runner.temp }}/form-package-release-candidate",
				"refusing overwrite",
			},
		},
		{
			name:         "Form Package revocation",
			path:         ".github/workflows/form-package-revocation.yml",
			environment:  "form-package-release",
			artifact:     "form-package-revocation-candidate-${{ github.run_id }}-${{ github.run_attempt }}",
			signing:      "cosign sign-blob",
			executionRef: "refs/heads/main",
			required: []string{
				"id-token: write",
				"takoform.form-package-revocation-candidate@v1",
				`candidate="$RUNNER_TEMP/form-package-revocation-candidate"`,
				`cd "$candidate"`,
				"cosign sign-blob",
				"cosign verify-blob",
				`(cd "$assets" && sha256sum --check --strict SHA256SUMS)`,
				"tagObjectSha256",
				`test "$(wc -l < SHA256SUMS)" -eq 8`,
				"expected-candidate-root.txt",
				"actual-candidate-root.txt",
				`diff -u "$RUNNER_TEMP/expected-candidate-root.txt" "$RUNNER_TEMP/actual-candidate-root.txt"`,
				`echo "path=$candidate" >> "$GITHUB_OUTPUT"`,
				"path: ${{ steps.candidate.outputs.path }}",
			},
		},
	}

	for _, contract := range workflows {
		t.Run(contract.name, func(t *testing.T) {
			workflow := readText(t, filepath.Join(root, filepath.FromSlash(contract.path)))
			common := []string{
				"workflow_dispatch:",
				"expected_commit:",
				"contents: read",
				"environment: " + contract.environment,
				contract.executionRef,
				"git merge-base --is-ancestor",
				"SHA256SUMS",
				"sha256sum --check --strict SHA256SUMS",
				"metadata.json",
				"actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
				"name: " + contract.artifact,
				"if-no-files-found: error",
				"retention-days: 1",
			}
			for _, required := range append(common, contract.required...) {
				if !strings.Contains(workflow, required) {
					t.Errorf("workflow lacks candidate-only contract marker %q", required)
				}
			}
			signingJob := workflowJobContaining(t, workflow, contract.signing)
			if !strings.Contains(signingJob, "environment: "+contract.environment) {
				t.Errorf("job performing %q is not protected by environment %q", contract.signing, contract.environment)
			}
			assertWorkflowHasNoPublicationMutation(t, workflow)
		})
	}

	packageWorkflow := readText(t, filepath.Join(root, ".github", "workflows", "form-package-release.yml"))
	revocationWorkflow := readText(t, filepath.Join(root, ".github", "workflows", "form-package-revocation.yml"))
	if !strings.Contains(packageWorkflow, `^forms/k-[a-z2-7]`) ||
		!strings.Contains(revocationWorkflow, `^forms/revocations/`) {
		t.Fatal("package and revocation tag namespaces are not disjoint")
	}
}

func TestReleaseIdentityAbsenceProbesFailClosedOnTransportAndAmbiguousStatus(t *testing.T) {
	root := repositoryRoot(t)
	providerTag := readText(t, filepath.Join(root, ".github", "workflows", "provider-release-tag.yml"))
	packageRelease := readText(t, filepath.Join(root, ".github", "workflows", "form-package-release.yml"))
	revocation := readText(t, filepath.Join(root, ".github", "workflows", "form-package-revocation.yml"))
	providerRelease := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))

	for name, workflow := range map[string]string{
		"provider tag":            providerTag,
		"Form Package":            packageRelease,
		"Form Package revocation": revocation,
	} {
		t.Run(name+" remote tag", func(t *testing.T) {
			for _, required := range []string{
				"if ! git ls-remote --tags origin",
				`> "$remote_tag_refs"; then`,
				`if [[ -s "$remote_tag_refs" ]]; then`,
			} {
				if !strings.Contains(workflow, required) {
					t.Errorf("remote tag absence does not require successful exact-empty ls-remote output: missing %q", required)
				}
			}
			if strings.Contains(workflow, `test -z "$(git ls-remote`) {
				t.Fatal("remote tag absence treats an ls-remote transport failure as empty output")
			}
		})
	}
	if strings.Count(providerTag, "if ! git ls-remote --tags origin") != 2 {
		t.Fatal("provider tag lane must prove remote absence before signing and re-prove it immediately before tag-object creation")
	}

	for name, workflow := range map[string]string{
		"provider tag":            providerTag,
		"provider release":        providerRelease,
		"Form Package":            packageRelease,
		"Form Package revocation": revocation,
	} {
		t.Run(name+" GitHub release", func(t *testing.T) {
			for _, required := range []string{
				`if ! github_repository_status="$(curl`,
				`if [[ "$github_repository_status" != "200" ]]; then`,
				`/releases?per_page=1`,
				`type == "array"`,
				`encoded_release_tag="$(jq -rn --arg tag "$RELEASE_TAG" '$tag | @uri')"`,
				`if ! github_release_status="$(curl`,
				`if [[ "$github_release_status" != "404" ]]; then`,
				`"https://api.github.com/repos/$GITHUB_REPOSITORY/releases/tags/$encoded_release_tag"`,
			} {
				if !strings.Contains(workflow, required) {
					t.Errorf("authenticated GitHub Release absence proof lacks %q", required)
				}
			}
			for _, forbidden := range []string{
				"if gh release view",
				`grep -F "HTTP 404"`,
				`/releases/tags/$RELEASE_TAG`,
			} {
				if strings.Contains(workflow, forbidden) {
					t.Errorf("GitHub Release absence retains ambiguous negative probe %q", forbidden)
				}
			}
		})
	}
	for name, workflow := range map[string]string{
		"provider tag":     providerTag,
		"provider release": providerRelease,
	} {
		t.Run(name+" Registry", func(t *testing.T) {
			if !strings.Contains(workflow, `if ! registry_status="$(curl`) ||
				!strings.Contains(workflow, `if [[ "$registry_status" != "404" ]]; then`) {
				t.Fatal("Registry absence must reject transport failure and every status other than exact 404")
			}
		})
	}

	revocationProbe := strings.Index(revocation, "if ! git ls-remote --tags origin")
	revocationRelease := strings.Index(revocation, `if ! github_release_status="$(curl`)
	revocationSign := strings.Index(revocation, "cosign sign-blob")
	revocationCandidate := strings.Index(revocation, `candidate="$RUNNER_TEMP/form-package-revocation-candidate"`)
	if revocationProbe < 0 || revocationRelease <= revocationProbe ||
		revocationSign <= revocationRelease || revocationCandidate <= revocationSign {
		t.Fatal("revocation tag and Release absence must be proven before protected signing and candidate creation")
	}
}

func TestDispatchedReleaseWorkflowsRequireExactRequestCorrelation(t *testing.T) {
	root := repositoryRoot(t)
	workflows := []string{
		".github/workflows/provider-release-tag.yml",
		".github/workflows/release.yml",
		".github/workflows/provider-registry-readback.yml",
		".github/workflows/form-package-release.yml",
		".github/workflows/form-package-revocation.yml",
		".github/workflows/standard-provider-report.yml",
		".github/workflows/standard-admission-evidence.yml",
	}
	const canonicalRequestIDValidation = `[[ ! "$REQUEST_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]`
	for _, path := range workflows {
		t.Run(filepath.Base(path), func(t *testing.T) {
			workflow := readText(t, filepath.Join(root, filepath.FromSlash(path)))
			requestInput := workflowDispatchInputBlock(t, workflow, "request_id")
			if !strings.Contains(requestInput, "required: true") ||
				!strings.Contains(requestInput, "type: string") {
				t.Fatal("request_id is not one required string workflow_dispatch input")
			}
			var runNames []string
			for _, line := range strings.Split(workflow, "\n") {
				if strings.HasPrefix(line, "run-name:") {
					runNames = append(runNames, strings.TrimSpace(line))
				}
			}
			if len(runNames) != 1 || runNames[0] != "run-name: ${{ inputs.request_id }}" {
				t.Fatalf("run-name must equal only the exact request UUID, got %v", runNames)
			}
			for _, required := range []string{
				"REQUEST_ID: ${{ inputs.request_id }}",
				canonicalRequestIDValidation,
				"requestId",
			} {
				if !strings.Contains(workflow, required) {
					t.Errorf("workflow lacks request correlation binding %q", required)
				}
			}
			if path == ".github/workflows/standard-provider-report.yml" {
				for _, required := range []string{
					"workflowRunId",
					"workflowRunAttempt",
					"headSha",
				} {
					if !strings.Contains(workflow, required) {
						t.Errorf("provider-report signed metadata lacks exact run binding %q", required)
					}
				}
			}
			if path == ".github/workflows/provider-registry-readback.yml" {
				for _, required := range []string{
					"workflowRunAttempt:Number(process.env.GITHUB_RUN_ATTEMPT)",
					"takoform-provider-registry-readback-unsigned-${{ inputs.request_id }}-${{ github.run_id }}-${{ github.run_attempt }}-${{ steps.source.outputs.source_commit_short }}",
					"takoform-provider-registry-readback-candidate-${{ inputs.request_id }}-${{ github.run_id }}-${{ github.run_attempt }}-${{ needs.generate.outputs.source_commit_short }}",
				} {
					if !strings.Contains(workflow, required) {
						t.Errorf("Registry candidate lacks exact rerun binding %q", required)
					}
				}
			}
		})
	}
}

func TestReleasePublicationIsOwnedByTheLocalDeployEntrypoint(t *testing.T) {
	root := repositoryRoot(t)
	packageJSON := readText(t, filepath.Join(root, "package.json"))
	if !strings.Contains(packageJSON, `"deploy": "bun scripts/deploy.mjs"`) {
		t.Fatal("bun run deploy does not route to the repository-owned deploy entrypoint")
	}

	deploy := readText(t, filepath.Join(root, "scripts", "deploy.mjs"))
	releaseDeploy := readText(t, filepath.Join(root, "scripts", "release-deploy.mjs"))
	for _, required := range []string{
		"./release-deploy.mjs",
		"RELEASE_SURFACES",
		"isReleaseSurface",
		"runReleaseSurface",
	} {
		if !strings.Contains(deploy, required) {
			t.Errorf("deploy.mjs does not wire the owner-local release module marker %q", required)
		}
	}
	dispatchStart := strings.Index(deploy, "if (isReleaseSurface(invocation[0])) {")
	dispatchEnd := strings.Index(deploy, "\nconst acknowledgedInitialSchemaOriginMint")
	if dispatchStart < 0 || dispatchEnd <= dispatchStart {
		t.Fatal("deploy.mjs has no reachable release-surface dispatch branch")
	}
	dispatch := deploy[dispatchStart:dispatchEnd]
	for _, required := range []string{
		"runReleaseSurface({",
		"surface: invocation[0]",
		"args: invocation.slice(1)",
		"repo,",
	} {
		if !strings.Contains(dispatch, required) {
			t.Errorf("release-surface dispatch branch lacks %q", required)
		}
	}
	wiring := deploy + "\n" + releaseDeploy
	for _, required := range []string{
		"takoform-provider-release",
		"takoform-form-package-release",
		"prepare",
		"publish",
		"readback",
		"verify",
		"prepare-revocation",
		"publish-revocation",
		"verify-revocation",
	} {
		if !strings.Contains(wiring, required) {
			t.Errorf("owner deploy entrypoint lacks release surface marker %q", required)
		}
	}
	localMutation := strings.NewReplacer(
		`"`, " ",
		`'`, " ",
		",", " ",
		"[", " ",
		"]", " ",
		"(", " ",
		")", " ",
	).Replace(releaseDeploy)
	localMutation = strings.Join(strings.Fields(localMutation), " ")
	for _, required := range []string{
		"git push",
		"gh api --method POST",
		"releases/${releaseId}/assets?name=",
		"gh api --method PATCH",
		"immutable",
	} {
		if !strings.Contains(localMutation, required) {
			t.Errorf("owner-local release deploy lacks publication marker %q", required)
		}
	}
}

func assertWorkflowHasNoPublicationMutation(t *testing.T, workflow string) {
	t.Helper()
	normalized := strings.ToLower(strings.Join(strings.Fields(workflow), " "))
	normalized = strings.NewReplacer(`"`, "", `'`, "").Replace(normalized)
	allowedAPIRead := "gh api repos/$github_repository/releases/tags/$release_tag"
	for _, rawLine := range strings.Split(strings.ReplaceAll(workflow, "\\\n", " "), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.ToLower(strings.NewReplacer(`"`, "", `'`, "").Replace(line))
		if strings.Contains(line, "gh api ") {
			if !strings.Contains(line, allowedAPIRead) {
				t.Errorf("workflow contains a GitHub API command other than the allowlisted release-existence GET: %s", rawLine)
			}
			for _, field := range strings.Fields(line) {
				field = strings.Trim(field, "\\")
				if strings.HasPrefix(field, "-f") || strings.HasPrefix(field, "--field") ||
					strings.HasPrefix(field, "--raw-field") || strings.HasPrefix(field, "--input") {
					t.Errorf("allowlisted GitHub API GET contains a request-body flag that implies POST: %s", rawLine)
				}
			}
		}
		if strings.Contains(line, "gh release ") && !strings.Contains(line, "gh release view ") {
			t.Errorf("workflow contains a gh release command other than the allowlisted read-only view: %s", rawLine)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"contents:write",
		"attestations: write",
		"attestations:write",
		"write-all",
		"actions/attest@",
		"git push",
		"gh release create",
		"gh release upload",
		"gh release edit",
		"gh release delete",
		"--method post",
		"--method=post",
		"--method patch",
		"--method=patch",
		"--method delete",
		"--method=delete",
		"-x post",
		"-xpost",
		"-x patch",
		"-xpatch",
		"-x delete",
		"-xdelete",
		"--request post",
		"--request=post",
		"--request patch",
		"--request=patch",
		"--request delete",
		"--request=delete",
		"--data ",
		"--data=",
		"--data-binary ",
		"--data-binary=",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("workflow retains CI publication mutation %q", forbidden)
		}
	}
}

func workflowJobContaining(t *testing.T, workflow, marker string) string {
	t.Helper()
	lines := strings.Split(workflow, "\n")
	markerLine := -1
	for index, line := range lines {
		if strings.Contains(line, marker) {
			markerLine = index
			break
		}
	}
	if markerLine < 0 {
		t.Fatalf("workflow lacks job marker %q", marker)
	}
	jobStart := -1
	for index := markerLine; index >= 0; index-- {
		line := lines[index]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") &&
			strings.HasSuffix(strings.TrimSpace(line), ":") {
			jobStart = index
			break
		}
	}
	if jobStart < 0 {
		t.Fatalf("cannot locate job containing %q", marker)
	}
	jobEnd := len(lines)
	for index := jobStart + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") &&
			strings.HasSuffix(strings.TrimSpace(line), ":") {
			jobEnd = index
			break
		}
	}
	return strings.Join(lines[jobStart:jobEnd], "\n")
}

func workflowDispatchInputBlock(t *testing.T, workflow, name string) string {
	t.Helper()
	marker := "      " + name + ":"
	lines := strings.Split(workflow, "\n")
	start := -1
	for index, line := range lines {
		if line == marker {
			start = index
			break
		}
	}
	if start < 0 {
		t.Fatalf("workflow_dispatch lacks input %q", name)
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if line == "" {
			end = index
			break
		}
		if len(line)-len(strings.TrimLeft(line, " ")) <= 6 {
			end = index
			break
		}
	}
	return strings.Join(lines[start+1:end], "\n")
}

func readStrictJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("decode %s: trailing JSON or parse error: %v", path, err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
