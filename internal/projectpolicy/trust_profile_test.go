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
			Envelope    string `json:"envelope"`
			Predicate   string `json:"predicate"`
			Attestation string `json:"attestation"`
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
}

func TestD08TrustProfileRemainsFailClosedAndSeparated(t *testing.T) {
	root := repositoryRoot(t)
	var profile trustProfile
	readStrictJSON(t, filepath.Join(root, "spec", "trust", "profile.json"), &profile)
	var release releaseDescriptor
	readStrictJSON(t, filepath.Join(root, "release", "version.json"), &release)

	if profile.SchemaVersion != 1 || profile.Status != "decision-approved-implementation-in-progress" {
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
	if profile.Provider.Provenance.Envelope != "in-toto-statement-v1" ||
		profile.Provider.Provenance.Predicate != "slsa-provenance-v1" ||
		profile.Provider.Provenance.Attestation != "github-artifact-attestation" {
		t.Fatalf("provider provenance profile is incomplete")
	}
	if profile.Provider.Distribution.Source != "github-release" ||
		profile.Provider.Distribution.Registry != "registry.terraform.io/tako0614/takoform" ||
		profile.Provider.Distribution.OverwriteExistingVersion {
		t.Fatalf("provider distribution is mutable or has the wrong registry")
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

func TestFormPackageWorkflowsRemainKeylessSeparatedAndCommitPinned(t *testing.T) {
	root := repositoryRoot(t)
	packageWorkflow := readText(t, filepath.Join(root, ".github", "workflows", "form-package-release.yml"))
	revocationWorkflow := readText(t, filepath.Join(root, ".github", "workflows", "form-package-revocation.yml"))
	for name, workflow := range map[string]string{"package": packageWorkflow, "revocation": revocationWorkflow} {
		for _, required := range []string{
			"workflow_dispatch:",
			"expected_commit:",
			"id-token: write",
			"attestations: write",
			"environment: form-package-release",
			"sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6",
			"cosign sign-blob",
			"cosign verify-blob",
			"--certificate-oidc-issuer \"https://token.actions.githubusercontent.com\"",
			"actions/attest@f7c74d28b9d84cb8768d0b8ca14a4bac6ef463e6",
			"refusing overwrite",
			"refs/heads/main",
			"git merge-base --is-ancestor",
			"path: release-source",
			"--repo \"$GITHUB_WORKSPACE/release-source\"",
			"steps.draft.outputs.release_id",
		} {
			if !strings.Contains(workflow, required) {
				t.Fatalf("%s workflow lacks %q", name, required)
			}
		}
		for _, forbidden := range []string{"push:", "GPG_PRIVATE_KEY", "PASSPHRASE", "gpg --", "cosign.key", "--key "} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s workflow crosses the keyless/provider trust boundary with %q", name, forbidden)
			}
		}
		cleanupMarker := "- name: Remove partial draft after failure"
		cleanupIndex := strings.Index(workflow, cleanupMarker)
		if cleanupIndex < 0 {
			t.Fatalf("%s workflow has no failure cleanup", name)
		}
		cleanup := workflow[cleanupIndex:]
		if !strings.Contains(cleanup, "RELEASE_ID: ${{ steps.draft.outputs.release_id }}") ||
			!strings.Contains(cleanup, "releases/$RELEASE_ID") || strings.Contains(cleanup, "releases/tags/") ||
			strings.Contains(cleanup, "gh release view") {
			t.Fatalf("%s workflow cleanup can target a pre-existing release", name)
		}
	}
	if !strings.Contains(packageWorkflow, `^forms/k-[a-z2-7]`) ||
		!strings.Contains(revocationWorkflow, `^forms/revocations/`) {
		t.Fatal("package and revocation tag namespaces are not disjoint")
	}
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
