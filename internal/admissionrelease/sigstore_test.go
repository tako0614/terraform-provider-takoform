package admissionrelease

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	sigstorebundle "github.com/sigstore/sigstore-go/pkg/bundle"
	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/testing/data"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"google.golang.org/protobuf/proto"
)

func TestOfflineSigstoreVerifierChecksProofTimeSCTIdentityAndDigest(t *testing.T) {
	t.Parallel()
	trustedRoot := data.TrustedRoot(t, "scaffolding.json")
	retainedBundle := data.Bundle(t, "othername.sigstore.json")
	policy := testPublisherPolicy()

	verifier, err := newOfflineRoleVerifier(trustedRoot, policy)
	if err != nil {
		t.Fatal(err)
	}
	signatureContent, err := retainedBundle.SignatureContent()
	if err != nil {
		t.Fatal(err)
	}
	digest := signatureContent.MessageSignatureContent().Digest()
	if err := verifier.verifyBundleDigest(retainedBundle, digest); err != nil {
		t.Fatalf("full offline verification failed: %v", err)
	}

	wrongDigest := append([]byte(nil), digest...)
	wrongDigest[0] ^= 0xff
	if err := verifier.verifyBundleDigest(retainedBundle, wrongDigest); err == nil || !strings.Contains(err.Error(), "does not bind") {
		t.Fatalf("wrong digest error = %v", err)
	}

	wrongPolicy := policy
	wrongPolicy.CertificateIdentity = "unexpected@example.invalid"
	wrongIdentityVerifier, err := newOfflineRoleVerifier(trustedRoot, wrongPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if err := wrongIdentityVerifier.verifyBundleDigest(retainedBundle, digest); err == nil || !strings.Contains(err.Error(), "certificate identity") {
		t.Fatalf("wrong identity error = %v", err)
	}

	withoutCTVerifier, err := newOfflineRoleVerifier(trustedMaterialWithoutCT{TrustedMaterial: trustedRoot}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := withoutCTVerifier.verifyBundleDigest(retainedBundle, digest); err == nil || !strings.Contains(err.Error(), "signed certificate timestamp") {
		t.Fatalf("missing SCT authority error = %v", err)
	}

	withoutPromiseProto := proto.Clone(retainedBundle.Bundle).(*protobundle.Bundle)
	withoutPromiseProto.VerificationMaterial.TlogEntries[0].InclusionPromise = nil
	withoutPromise, err := sigstorebundle.NewBundle(withoutPromiseProto)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.verifyBundleDigest(withoutPromise, digest); err == nil || !strings.Contains(err.Error(), "integrated timestamps") {
		t.Fatalf("unsigned integrated-time error = %v", err)
	}
}

func TestOfflineSigstoreVerifierRequiresV03InclusionProof(t *testing.T) {
	t.Parallel()
	retainedBundle := data.Bundle(t, "othername.sigstore.json")
	withoutProofProto := proto.Clone(retainedBundle.Bundle).(*protobundle.Bundle)
	withoutProofProto.VerificationMaterial.TlogEntries[0].InclusionProof = nil
	if _, err := sigstorebundle.NewBundle(withoutProofProto); err == nil || !strings.Contains(err.Error(), "inclusion proof missing") {
		t.Fatalf("missing inclusion proof error = %v", err)
	}
}

func TestLoadOfflineVerifierRequiresPinnedRetainedTrust(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := loadOfflineRetainedSubjectVerifier(root); err == nil || !strings.Contains(err.Error(), offlineSigstorePinsPath) {
		t.Fatalf("missing pins error = %v", err)
	}

	trustedRootRaw, err := data.TrustedRoot(t, "scaffolding.json").MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	writeRetainedTestFile(t, root, canonicalTrustedRootPath, trustedRootRaw)
	policyFiles := []struct {
		path     string
		identity string
	}{
		{canonicalPublisherPolicyPath, "admission-evidence!oidc.local"},
		{canonicalHostReportPolicyPath, "host-report!oidc.local"},
		{canonicalProviderReportPolicyPath, "provider-report!oidc.local"},
		{canonicalPackageIndexPolicyPath, "package-index!oidc.local"},
		{canonicalRegistryReadbackPolicyPath, "registry-readback!oidc.local"},
	}
	policyRaws := make(map[string][]byte, len(policyFiles))
	for _, file := range policyFiles {
		policy := testPublisherPolicy()
		policy.CertificateIdentity = file.identity
		policyRaw, err := json.Marshal(policy)
		if err != nil {
			t.Fatal(err)
		}
		writeRetainedTestFile(t, root, file.path, policyRaw)
		policyRaws[file.path] = policyRaw
	}
	pins := OfflineSigstorePins{
		Format: offlineSigstorePinsFormat,
		TrustedRoot: RetainedFile{
			Path: canonicalTrustedRootPath, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		AdmissionEvidencePolicy: RetainedFile{
			Path: canonicalPublisherPolicyPath, Digest: formDigest(policyRaws[canonicalPublisherPolicyPath]),
		},
		HostReportPolicy:       RetainedFile{Path: canonicalHostReportPolicyPath, Digest: formDigest(policyRaws[canonicalHostReportPolicyPath])},
		ProviderReportPolicy:   RetainedFile{Path: canonicalProviderReportPolicyPath, Digest: formDigest(policyRaws[canonicalProviderReportPolicyPath])},
		PackageIndexPolicy:     RetainedFile{Path: canonicalPackageIndexPolicyPath, Digest: formDigest(policyRaws[canonicalPackageIndexPolicyPath])},
		RegistryReadbackPolicy: RetainedFile{Path: canonicalRegistryReadbackPolicyPath, Digest: formDigest(policyRaws[canonicalRegistryReadbackPolicyPath])},
	}
	writeRetainedTestJSON(t, root, offlineSigstorePinsPath, pins)
	if _, err := loadOfflineRetainedSubjectVerifier(root); err == nil || !strings.Contains(err.Error(), "pinned trusted root digest mismatch") {
		t.Fatalf("trusted-root pin mismatch error = %v", err)
	}

	pins.TrustedRoot.Digest = formDigest(trustedRootRaw)
	writeRetainedTestJSON(t, root, offlineSigstorePinsPath, pins)
	if _, err := loadOfflineRetainedSubjectVerifier(root); err != nil {
		t.Fatalf("load exact retained trust: %v", err)
	}

	duplicatePolicyRaw := policyRaws[canonicalPublisherPolicyPath]
	writeRetainedTestFile(t, root, canonicalHostReportPolicyPath, duplicatePolicyRaw)
	pins.HostReportPolicy.Digest = formDigest(duplicatePolicyRaw)
	writeRetainedTestJSON(t, root, offlineSigstorePinsPath, pins)
	if _, err := loadOfflineRetainedSubjectVerifier(root); err == nil || !strings.Contains(err.Error(), "reuse the same certificate identity") {
		t.Fatalf("duplicate role publisher identity error = %v", err)
	}
}

func TestReadRetainedRelativeFileRejectsSymlinkedParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "trusted-root.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "trust")); err != nil {
		t.Fatal(err)
	}
	if _, err := readRetainedRelativeFile(root, canonicalTrustedRootPath, maxTrustedRootBytes); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("symlinked-parent error = %v", err)
	}
}

type trustedMaterialWithoutCT struct {
	sigstoreroot.TrustedMaterial
}

func (trustedMaterialWithoutCT) CTLogs() map[string]*sigstoreroot.TransparencyLog {
	return nil
}

func testPublisherPolicy() PublisherPolicy {
	return PublisherPolicy{
		Format:              publisherPolicyFormat,
		OIDCIssuer:          "http://oidc.local:8080",
		CertificateIdentity: "foo!oidc.local",
		BundleMediaType:     sigstoreBundleMediaTypeV03,
	}
}

func formDigest(raw []byte) string {
	return formpackage.DigestBytes(raw)
}
