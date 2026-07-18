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
	offlineSigstorePinsFormat      = "takoform.offline-sigstore-pins@v1"
	publisherPolicyFormat          = "takoform.sigstore-publisher-policy@v1"
	offlineSigstorePinsPath        = "trust/offline-sigstore-pins.json"
	canonicalTrustedRootPath       = "trust/trusted-root.json"
	canonicalPublisherPolicyPath   = "trust/publisher-policy.json"
	sigstoreBundleMediaTypeV03     = "application/vnd.dev.sigstore.bundle.v0.3+json"
	maxOfflineSigstorePinsBytes    = 64 << 10
	maxPublisherPolicyBytes        = 64 << 10
	maxTrustedRootBytes            = 4 << 20
	maxSigstoreBundleBytes         = 16 << 20
	requiredTransparencyLogEntries = 1
	requiredIntegratedTimestamps   = 1
	requiredCertificateTimestamps  = 1
)

// OfflineSigstorePins binds the reviewed trust inputs used by release-check.
// The pin manifest itself is protected source, not evidence discovered from a
// release or distribution endpoint.
type OfflineSigstorePins struct {
	Format          string       `json:"format"`
	TrustedRoot     RetainedFile `json:"trustedRoot"`
	PublisherPolicy RetainedFile `json:"publisherPolicy"`
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

type offlineRetainedEvidenceVerifier struct {
	verifier  *verify.Verifier
	identity  verify.CertificateIdentity
	mediaType string
}

func loadOfflineRetainedEvidenceVerifier(admissionRoot string) (RetainedEvidenceVerifier, error) {
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

	policyRaw, err := readPinnedRetainedFile(admissionRoot, "publisher policy", pins.PublisherPolicy, maxPublisherPolicyBytes)
	if err != nil {
		return nil, err
	}
	if _, err := formpackage.Canonicalize(policyRaw); err != nil {
		return nil, fmt.Errorf("validate pinned publisher policy I-JSON: %w", err)
	}
	var policy PublisherPolicy
	if err := decodeStrictJSON(policyRaw, &policy); err != nil {
		return nil, fmt.Errorf("decode pinned publisher policy: %w", err)
	}
	return newOfflineRetainedEvidenceVerifier(trustedRoot, policy)
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
		{label: "publisherPolicy", retained: pins.PublisherPolicy},
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
	if pins.PublisherPolicy.Path != canonicalPublisherPolicyPath {
		return fmt.Errorf("publisherPolicy path is %q, want %q", pins.PublisherPolicy.Path, canonicalPublisherPolicyPath)
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

func newOfflineRetainedEvidenceVerifier(trustedMaterial sigstoreroot.TrustedMaterial, policy PublisherPolicy) (*offlineRetainedEvidenceVerifier, error) {
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
	return &offlineRetainedEvidenceVerifier{verifier: verifier, identity: identity, mediaType: policy.BundleMediaType}, nil
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

func (v *offlineRetainedEvidenceVerifier) VerifyRetainedEvidence(admissionRoot string, set Set, subjects []RetainedSubject) error {
	if len(subjects) != len(set.Entries) {
		return fmt.Errorf("retained subject closure has %d entries, want %d", len(subjects), len(set.Entries))
	}
	entries := make(map[string]SetEntry, len(set.Entries))
	for _, entry := range set.Entries {
		if _, duplicate := entries[entry.Kind]; duplicate {
			return fmt.Errorf("retained set duplicates kind %q", entry.Kind)
		}
		entries[entry.Kind] = entry
	}
	seen := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		if subject.Kind == "" || len(subject.Canonical) == 0 {
			return fmt.Errorf("retained subject kind and canonical bytes are required")
		}
		if _, duplicate := seen[subject.Kind]; duplicate {
			return fmt.Errorf("retained subjects duplicate kind %q", subject.Kind)
		}
		seen[subject.Kind] = struct{}{}
		entry, ok := entries[subject.Kind]
		if !ok {
			return fmt.Errorf("retained subject kind %q is not in the admission set", subject.Kind)
		}
		canonical, err := formpackage.Canonicalize(subject.Canonical)
		if err != nil || !bytes.Equal(canonical, subject.Canonical) {
			return fmt.Errorf("%s retained subject is not the exact RFC 8785 canonical evidence bytes", subject.Kind)
		}
		if entry.EvidencePath != subject.Path || entry.EvidenceDigest != formpackage.DigestBytes(subject.Canonical) {
			return fmt.Errorf("%s retained subject path/digest is not bound by the admission set", subject.Kind)
		}
		expectedBundlePath := path.Join(path.Dir(subject.Path), "evidence.sigstore.json")
		if subject.SigstorePath != expectedBundlePath {
			return fmt.Errorf("%s evidence bundle path is %q, want %q", subject.Kind, subject.SigstorePath, expectedBundlePath)
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
		if err := v.verifyCanonicalSubject(&retainedBundle, subject.Canonical); err != nil {
			return fmt.Errorf("%s evidence bundle: %w", subject.Kind, err)
		}
	}
	return nil
}

func (v *offlineRetainedEvidenceVerifier) verifyCanonicalSubject(retainedBundle *bundle.Bundle, canonical []byte) error {
	digest := sha256.Sum256(canonical)
	return v.verifyBundleDigest(retainedBundle, digest[:])
}

func (v *offlineRetainedEvidenceVerifier) verifyBundleDigest(retainedBundle *bundle.Bundle, digest []byte) error {
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
