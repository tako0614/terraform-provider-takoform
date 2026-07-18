package admissionrelease

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"path"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	offlineSigstorePinsFormat           = "takoform.offline-sigstore-pins@v2"
	publisherPolicyFormat               = "takoform.sigstore-publisher-policy@v1"
	offlineSigstorePinsPath             = "trust/offline-sigstore-pins.json"
	canonicalTrustedRootPath            = "trust/trusted-root.json"
	canonicalPublisherPolicyPath        = "trust/publisher-policy.json"
	canonicalHostReportPolicyPath       = "trust/host-report-policy.json"
	canonicalProviderReportPolicyPath   = "trust/provider-report-policy.json"
	canonicalPackageIndexPolicyPath     = "trust/package-index-policy.json"
	canonicalRegistryReadbackPolicyPath = "trust/registry-readback-policy.json"
	sigstoreBundleMediaTypeV03          = "application/vnd.dev.sigstore.bundle.v0.3+json"
	maxOfflineSigstorePinsBytes         = 64 << 10
	maxPublisherPolicyBytes             = 64 << 10
	maxTrustedRootBytes                 = 4 << 20
	maxSigstoreBundleBytes              = 16 << 20
	requiredTransparencyLogEntries      = 1
	requiredIntegratedTimestamps        = 1
	requiredCertificateTimestamps       = 1
	roleAdmissionEvidence               = "admission-evidence"
	roleHostReport                      = "host-report"
	roleProviderReport                  = "provider-report"
	rolePackageIndex                    = "package-index"
	roleRegistryReadback                = "registry-readback"
)

// OfflineSigstorePins binds the reviewed trust inputs used by release-check.
// The pin manifest itself is protected source, not evidence discovered from a
// release or distribution endpoint.
type OfflineSigstorePins struct {
	Format                  string       `json:"format"`
	TrustedRoot             RetainedFile `json:"trustedRoot"`
	AdmissionEvidencePolicy RetainedFile `json:"admissionEvidencePolicy"`
	HostReportPolicy        RetainedFile `json:"hostReportPolicy"`
	ProviderReportPolicy    RetainedFile `json:"providerReportPolicy"`
	PackageIndexPolicy      RetainedFile `json:"packageIndexPolicy"`
	RegistryReadbackPolicy  RetainedFile `json:"registryReadbackPolicy"`
}

// RetainedFile identifies one repository-retained trust input by exact bytes.
type RetainedFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// PublisherPolicy pins the exact Fulcio issuer and certificate identity. The
// trust-root and policy byte digests are reviewed independently so replacing
// either file never silently changes admission authority.
type PublisherPolicy struct {
	Format              string `json:"format"`
	OIDCIssuer          string `json:"oidcIssuer"`
	CertificateIdentity string `json:"certificateIdentity"`
	BundleMediaType     string `json:"bundleMediaType"`
}

type offlineRoleVerifier struct {
	verifier  *verify.Verifier
	identity  verify.CertificateIdentity
	mediaType string
}

type offlineRetainedSubjectVerifier struct {
	roles map[string]*offlineRoleVerifier
}

func loadOfflineRetainedSubjectVerifier(admissionRoot string) (RetainedSubjectVerifier, error) {
	pinsRaw, err := readRetainedRelativeFile(admissionRoot, offlineSigstorePinsPath, maxOfflineSigstorePinsBytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", offlineSigstorePinsPath, err)
	}
	if _, err := formpackage.Canonicalize(pinsRaw); err != nil {
		return nil, fmt.Errorf("validate %s I-JSON: %w", offlineSigstorePinsPath, err)
	}
	var pins OfflineSigstorePins
	if err := decodeStrictJSON(pinsRaw, &pins); err != nil {
		return nil, fmt.Errorf("decode %s: %w", offlineSigstorePinsPath, err)
	}
	if err := validateOfflineSigstorePins(pins); err != nil {
		return nil, fmt.Errorf("validate %s: %w", offlineSigstorePinsPath, err)
	}

	trustedRootRaw, err := readPinnedRetainedFile(admissionRoot, "trusted root", pins.TrustedRoot, maxTrustedRootBytes)
	if err != nil {
		return nil, err
	}
	trustedRoot, err := sigstoreroot.NewTrustedRootFromJSON(trustedRootRaw)
	if err != nil {
		return nil, fmt.Errorf("decode pinned trusted root: %w", err)
	}

	retainedPolicies := []struct {
		role     string
		retained RetainedFile
	}{
		{role: roleAdmissionEvidence, retained: pins.AdmissionEvidencePolicy},
		{role: roleHostReport, retained: pins.HostReportPolicy},
		{role: roleProviderReport, retained: pins.ProviderReportPolicy},
		{role: rolePackageIndex, retained: pins.PackageIndexPolicy},
		{role: roleRegistryReadback, retained: pins.RegistryReadbackPolicy},
	}
	result := &offlineRetainedSubjectVerifier{roles: make(map[string]*offlineRoleVerifier, len(retainedPolicies))}
	seenPublisherIdentities := make(map[string]string, len(retainedPolicies))
	for _, item := range retainedPolicies {
		role, retained := item.role, item.retained
		policyRaw, err := readPinnedRetainedFile(admissionRoot, role+" publisher policy", retained, maxPublisherPolicyBytes)
		if err != nil {
			return nil, err
		}
		if _, err := formpackage.Canonicalize(policyRaw); err != nil {
			return nil, fmt.Errorf("validate pinned %s publisher policy I-JSON: %w", role, err)
		}
		var policy PublisherPolicy
		if err := decodeStrictJSON(policyRaw, &policy); err != nil {
			return nil, fmt.Errorf("decode pinned %s publisher policy: %w", role, err)
		}
		identityKey := policy.OIDCIssuer + "\x00" + policy.CertificateIdentity
		if priorRole, duplicate := seenPublisherIdentities[identityKey]; duplicate {
			return nil, fmt.Errorf("%s and %s publisher policies reuse the same certificate identity", priorRole, role)
		}
		seenPublisherIdentities[identityKey] = role
		verifier, err := newOfflineRoleVerifier(trustedRoot, policy)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", role, err)
		}
		result.roles[role] = verifier
	}
	return result, nil
}

func validateOfflineSigstorePins(pins OfflineSigstorePins) error {
	if pins.Format != offlineSigstorePinsFormat {
		return fmt.Errorf("format is %q, want %q", pins.Format, offlineSigstorePinsFormat)
	}
	for _, item := range []struct {
		label    string
		retained RetainedFile
	}{
		{label: "trustedRoot", retained: pins.TrustedRoot},
		{label: "admissionEvidencePolicy", retained: pins.AdmissionEvidencePolicy},
		{label: "hostReportPolicy", retained: pins.HostReportPolicy},
		{label: "providerReportPolicy", retained: pins.ProviderReportPolicy},
		{label: "packageIndexPolicy", retained: pins.PackageIndexPolicy},
		{label: "registryReadbackPolicy", retained: pins.RegistryReadbackPolicy},
	} {
		label, retained := item.label, item.retained
		if err := validateRelativePath(retained.Path); err != nil {
			return fmt.Errorf("%s path: %w", label, err)
		}
		if !formpackage.ValidDigest(retained.Digest) {
			return fmt.Errorf("%s digest is not a canonical SHA-256 digest", label)
		}
	}
	if pins.TrustedRoot.Path != canonicalTrustedRootPath {
		return fmt.Errorf("trustedRoot path is %q, want %q", pins.TrustedRoot.Path, canonicalTrustedRootPath)
	}
	for _, item := range []struct {
		label string
		got   string
		want  string
	}{
		{label: "admissionEvidencePolicy", got: pins.AdmissionEvidencePolicy.Path, want: canonicalPublisherPolicyPath},
		{label: "hostReportPolicy", got: pins.HostReportPolicy.Path, want: canonicalHostReportPolicyPath},
		{label: "providerReportPolicy", got: pins.ProviderReportPolicy.Path, want: canonicalProviderReportPolicyPath},
		{label: "packageIndexPolicy", got: pins.PackageIndexPolicy.Path, want: canonicalPackageIndexPolicyPath},
		{label: "registryReadbackPolicy", got: pins.RegistryReadbackPolicy.Path, want: canonicalRegistryReadbackPolicyPath},
	} {
		if item.got != item.want {
			return fmt.Errorf("%s path is %q, want %q", item.label, item.got, item.want)
		}
	}
	return nil
}

func readPinnedRetainedFile(admissionRoot, label string, retained RetainedFile, maximum int64) ([]byte, error) {
	raw, err := readRetainedRelativeFile(admissionRoot, retained.Path, maximum)
	if err != nil {
		return nil, fmt.Errorf("read pinned %s %q: %w", label, retained.Path, err)
	}
	actual := formpackage.DigestBytes(raw)
	if actual != retained.Digest {
		return nil, fmt.Errorf("pinned %s digest mismatch: pin=%s actual=%s", label, retained.Digest, actual)
	}
	return raw, nil
}

func newOfflineRoleVerifier(trustedMaterial sigstoreroot.TrustedMaterial, policy PublisherPolicy) (*offlineRoleVerifier, error) {
	if err := validatePublisherPolicy(policy); err != nil {
		return nil, fmt.Errorf("publisher policy: %w", err)
	}
	identity, err := verify.NewShortCertificateIdentity(policy.OIDCIssuer, "", policy.CertificateIdentity, "")
	if err != nil {
		return nil, fmt.Errorf("publisher certificate identity: %w", err)
	}
	verifier, err := verify.NewVerifier(
		trustedMaterial,
		verify.WithTransparencyLog(requiredTransparencyLogEntries),
		verify.WithIntegratedTimestamps(requiredIntegratedTimestamps),
		verify.WithSignedCertificateTimestamps(requiredCertificateTimestamps),
	)
	if err != nil {
		return nil, fmt.Errorf("offline Sigstore verifier: %w", err)
	}
	return &offlineRoleVerifier{verifier: verifier, identity: identity, mediaType: policy.BundleMediaType}, nil
}

func validatePublisherPolicy(policy PublisherPolicy) error {
	if policy.Format != publisherPolicyFormat {
		return fmt.Errorf("format is %q, want %q", policy.Format, publisherPolicyFormat)
	}
	if policy.OIDCIssuer == "" || policy.CertificateIdentity == "" {
		return fmt.Errorf("exact oidcIssuer and certificateIdentity are required")
	}
	if policy.BundleMediaType != sigstoreBundleMediaTypeV03 {
		return fmt.Errorf("bundleMediaType is %q, want %q", policy.BundleMediaType, sigstoreBundleMediaTypeV03)
	}
	return nil
}

func (v *offlineRetainedSubjectVerifier) VerifyRetainedSubjects(admissionRoot string, set Set, subjects []RetainedSubject) error {
	wantCount := len(set.Entries)*4 + 1
	if len(subjects) != wantCount {
		return fmt.Errorf("retained subject closure has %d entries, want %d", len(subjects), wantCount)
	}
	expected := expectedRetainedSubjects(set)
	seen := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		if subject.Kind == "" || subject.Role == "" || len(subject.Canonical) == 0 {
			return fmt.Errorf("retained subject kind, role, and canonical bytes are required")
		}
		key := subject.Role + ":" + subject.Kind
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("retained subjects duplicate %q", key)
		}
		seen[key] = struct{}{}
		want, ok := expected[key]
		if !ok {
			return fmt.Errorf("retained subject %q is not in the admission set", key)
		}
		canonical, err := formpackage.Canonicalize(subject.Canonical)
		if err != nil || !bytes.Equal(canonical, subject.Canonical) {
			return fmt.Errorf("%s retained subject is not the exact RFC 8785 canonical evidence bytes", subject.Kind)
		}
		if want.Path != subject.Path || want.Digest != formpackage.DigestBytes(subject.Canonical) || want.Bundle != subject.SigstorePath {
			return fmt.Errorf("%s retained subject path/digest is not bound by the admission set", subject.Kind)
		}
		if err := validateRelativePath(subject.SigstorePath); err != nil {
			return fmt.Errorf("%s evidence bundle path: %w", subject.Kind, err)
		}
		bundleRaw, err := readRetainedRelativeFile(admissionRoot, subject.SigstorePath, maxSigstoreBundleBytes)
		if err != nil {
			return fmt.Errorf("%s evidence bundle %q: %w", subject.Kind, subject.SigstorePath, err)
		}
		var retainedBundle bundle.Bundle
		if err := retainedBundle.UnmarshalJSON(bundleRaw); err != nil {
			return fmt.Errorf("%s evidence bundle: %w", subject.Kind, err)
		}
		roleVerifier, ok := v.roles[subject.Role]
		if !ok {
			return fmt.Errorf("%s has no pinned publisher policy", subject.Role)
		}
		if err := roleVerifier.verifyCanonicalSubject(&retainedBundle, subject.Canonical); err != nil {
			return fmt.Errorf("%s evidence bundle: %w", subject.Kind, err)
		}
	}
	return nil
}

type retainedSubjectBinding struct {
	Path   string
	Digest string
	Bundle string
}

func expectedRetainedSubjects(set Set) map[string]retainedSubjectBinding {
	expected := make(map[string]retainedSubjectBinding, len(set.Entries)*4+1)
	for _, entry := range set.Entries {
		expected[roleAdmissionEvidence+":"+entry.Kind] = retainedSubjectBinding{
			Path: entry.EvidencePath, Digest: entry.EvidenceDigest,
			Bundle: path.Join(path.Dir(entry.EvidencePath), "evidence.sigstore.json"),
		}
		expected[roleHostReport+":"+entry.Kind] = retainedSubjectBinding{
			Path: entry.HostReportPath, Digest: entry.HostReportDigest, Bundle: entry.HostReportSigstoreBundle,
		}
		expected[roleProviderReport+":"+entry.Kind] = retainedSubjectBinding{
			Path: entry.ProviderReportPath, Digest: entry.ProviderReportDigest, Bundle: entry.ProviderReportSigstoreBundle,
		}
		expected[rolePackageIndex+":"+entry.Kind] = retainedSubjectBinding{
			Path: entry.PackageIndexPath, Digest: entry.PackageDigest, Bundle: entry.PackageIndexSigstoreBundle,
		}
	}
	expected[roleRegistryReadback+":provider"] = retainedSubjectBinding{
		Path: set.ProviderRegistryReadback.Path, Digest: set.ProviderRegistryReadback.Digest, Bundle: set.ProviderRegistryReadback.SigstoreBundle,
	}
	return expected
}

func (v *offlineRoleVerifier) verifyCanonicalSubject(retainedBundle *bundle.Bundle, canonical []byte) error {
	digest := sha256.Sum256(canonical)
	return v.verifyBundleDigest(retainedBundle, digest[:])
}

func (v *offlineRoleVerifier) verifyBundleDigest(retainedBundle *bundle.Bundle, digest []byte) error {
	if retainedBundle.MediaType != v.mediaType {
		return fmt.Errorf("media type is %q, want %q", retainedBundle.MediaType, v.mediaType)
	}
	if !retainedBundle.HasInclusionProof() {
		return fmt.Errorf("Rekor inclusion proof is required")
	}
	signatureContent, err := retainedBundle.SignatureContent()
	if err != nil {
		return fmt.Errorf("read message signature: %w", err)
	}
	messageSignature := signatureContent.MessageSignatureContent()
	if messageSignature == nil || signatureContent.EnvelopeContent() != nil {
		return fmt.Errorf("keyless blob evidence requires one message signature")
	}
	if messageSignature.DigestAlgorithm() != "SHA2_256" {
		return fmt.Errorf("message digest algorithm is %q, want SHA2_256", messageSignature.DigestAlgorithm())
	}
	if !bytes.Equal(messageSignature.Digest(), digest) {
		return fmt.Errorf("message digest does not bind the retained canonical evidence bytes")
	}
	result, err := v.verifier.Verify(retainedBundle, verify.NewPolicy(
		verify.WithArtifactDigest("sha256", digest),
		verify.WithCertificateIdentity(v.identity),
	))
	if err != nil {
		return fmt.Errorf("offline Sigstore verification: %w", err)
	}
	if result == nil || result.Signature == nil || result.Signature.Certificate == nil || result.VerifiedIdentity == nil {
		return fmt.Errorf("offline Sigstore verification did not return a verified Fulcio identity")
	}
	return nil
}
