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
			TagPattern       string `json:"tagPattern"`
		} `json:"publisherPolicy"`
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
			InitialSource            string `json:"initialSource"`
			MirrorPolicy             string `json:"mirrorPolicy"`
			CustomerRequestFetch     bool   `json:"customerRequestFetch"`
			OverwriteExistingVersion bool   `json:"overwriteExistingVersion"`
		} `json:"distribution"`
		Revocation struct {
			Metadata                                 string `json:"metadata"`
			BlockNewCreateOrUpdate                   bool   `json:"blockNewCreateOrUpdate"`
			RetainReferencedBytesForObserveAndDelete bool   `json:"retainReferencedBytesForObserveAndDelete"`
			ReplaceBytesInPlace                      bool   `json:"replaceBytesInPlace"`
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
	SchemaVersion      int      `json:"schemaVersion"`
	Version            string   `json:"version"`
	Tag                string   `json:"tag"`
	SourceRepository   string   `json:"sourceRepository"`
	ProviderAddress    string   `json:"providerAddress"`
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

	if profile.SchemaVersion != 1 || profile.Status != "decision-approved-implementation-pending" {
		t.Fatalf("unexpected trust profile identity: version=%d status=%q", profile.SchemaVersion, profile.Status)
	}
	if profile.Provider.Status != "implemented-external-registration-and-install-proof-pending" ||
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
	if packageTrust.Status != "contract-approved-implementation-pending" ||
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
		packageTrust.PublisherPolicy.TagPattern != "refs/tags/forms/*/v*" {
		t.Fatalf("unexpected Form Package publisher policy")
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
	if packageTrust.Distribution.InitialSource != "github-release" ||
		!strings.Contains(packageTrust.Distribution.MirrorPolicy, "exact assets") ||
		packageTrust.Distribution.CustomerRequestFetch || packageTrust.Distribution.OverwriteExistingVersion ||
		!strings.Contains(packageTrust.Revocation.Metadata, "exact package digest") ||
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

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
