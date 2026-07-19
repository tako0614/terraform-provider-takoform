package admissionrelease

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	publishedPackageSetFormat = "takoform.published-package-set@v1"
	publishedPackageSetPath   = admissionRootPath + "/published-package-set.json"
	publishedPackageTrustPath = "trust/published-package-trust.json"
	publishedPackageTrustFmt  = "takoform.published-package-trust@v1"
	publishedRepository       = "tako0614/terraform-provider-takoform"
	registryPublisherIssuer   = "https://token.actions.githubusercontent.com"
	registryPublisherIdentity = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/standard-admission-release.yml@refs/heads/main"
)

// PublishedPackageSet is a source-reviewed snapshot of the ten live,
// immutable Form Package releases. It proves only distribution publication;
// portable-standard admission remains governed by standard-admission-set.json.
type PublishedPackageSet struct {
	Format                     string                   `json:"format"`
	Repository                 string                   `json:"repository"`
	DefinitionVersion          string                   `json:"definitionVersion"`
	PackageVersion             string                   `json:"packageVersion"`
	PublicationStatus          string                   `json:"publicationStatus"`
	AdmissionStatus            string                   `json:"admissionStatus"`
	RevocationCheckpointStatus string                   `json:"revocationCheckpointStatus"`
	Trust                      PublishedPackageTrustRef `json:"trust"`
	Entries                    []PublishedPackageEntry  `json:"entries"`
}

// PublishedPackageTrustRef pins the exact offline trust document.
type PublishedPackageTrustRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// PublishedPackageTrust pins the production Sigstore root and the two settled
// workflow policies without inventing policies for admission roles whose
// publisher authority has not been approved yet.
type PublishedPackageTrust struct {
	Format                  string       `json:"format"`
	TrustedRoot             RetainedFile `json:"trustedRoot"`
	PackageIndexPolicy      RetainedFile `json:"packageIndexPolicy"`
	RegistryReadbackPolicy  RetainedFile `json:"registryReadbackPolicy"`
	UnsettledPublisherRoles []string     `json:"unsettledPublisherRoles"`
}

// PublishedPackageEntry binds one candidate to the exact immutable GitHub
// Release snapshot and its repository-retained release closure.
type PublishedPackageEntry struct {
	Kind                         string              `json:"kind"`
	Slug                         string              `json:"slug"`
	FormRef                      formpackage.FormRef `json:"formRef"`
	PackageDigest                string              `json:"packageDigest"`
	ReleaseTag                   string              `json:"releaseTag"`
	ReleaseCommit                string              `json:"releaseCommit"`
	ReleaseToolingCommit         string              `json:"releaseToolingCommit"`
	GitHubReleaseID              int64               `json:"githubReleaseId"`
	PublishedAt                  string              `json:"publishedAt"`
	Immutable                    bool                `json:"immutable"`
	PackageReleaseManifestPath   string              `json:"packageReleaseManifestPath"`
	PackageReleaseManifestDigest string              `json:"packageReleaseManifestDigest"`
	ChecksumsPath                string              `json:"checksumsPath"`
	ChecksumsDigest              string              `json:"checksumsDigest"`
	PackageIndexPath             string              `json:"packageIndexPath"`
	PackageIndexSigstoreBundle   string              `json:"packageIndexSigstoreBundle"`
}

// VerifyPublishedPackageSet authenticates the exact retained publication
// readback with no network lookup. GitHub release immutability is a reviewed
// live snapshot claim; the cryptographic package closure is independently
// enforced by the release manifest, SHA256SUMS, Git refs, and Sigstore.
func VerifyPublishedPackageSet(root string, candidates CandidateSet) error {
	if err := validateCandidateSet(candidates); err != nil {
		return fmt.Errorf("published-package candidate set: %w", err)
	}
	raw, err := readRetainedRelativeFile(root, publishedPackageSetPath, maxSetBytes)
	if err != nil {
		return fmt.Errorf("read %s: %w", publishedPackageSetPath, err)
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return fmt.Errorf("%s must contain RFC 8785-compatible I-JSON: %w", publishedPackageSetPath, err)
	}
	var set PublishedPackageSet
	if err := decodeStrictJSON(raw, &set); err != nil {
		return fmt.Errorf("decode %s: %w", publishedPackageSetPath, err)
	}
	ordered, err := validatePublishedPackageSet(set, candidates)
	if err != nil {
		return fmt.Errorf("verify %s: %w", publishedPackageSetPath, err)
	}

	admissionRoot := path.Join(root, admissionRootPath)
	_, verifier, err := loadPublishedPackageTrust(admissionRoot, set.Trust)
	if err != nil {
		return fmt.Errorf("published-package trust: %w", err)
	}
	for _, pair := range ordered {
		indexRaw, err := verifyPackageReleaseReadback(admissionRoot, pair.matchedEntry, set.PackageVersion)
		if err != nil {
			return fmt.Errorf("%s package release readback: %w", pair.entry.Kind, err)
		}
		published := set.Entries[pair.position]
		if err := verifyReleaseChecksums(admissionRoot, published); err != nil {
			return fmt.Errorf("%s release checksums: %w", pair.entry.Kind, err)
		}
		bundleRaw, err := readRetainedRelativeFile(admissionRoot, pair.entry.PackageIndexSigstoreBundle, maxSigstoreBundleBytes)
		if err != nil {
			return fmt.Errorf("%s package-index bundle: %w", pair.entry.Kind, err)
		}
		var retainedBundle bundle.Bundle
		if err := retainedBundle.UnmarshalJSON(bundleRaw); err != nil {
			return fmt.Errorf("%s package-index bundle: %w", pair.entry.Kind, err)
		}
		if err := verifier.verifyCanonicalSubject(&retainedBundle, indexRaw); err != nil {
			return fmt.Errorf("%s package-index publisher: %w", pair.entry.Kind, err)
		}
		if err := requireTagCommit(root, pair.entry.Kind+" package release", pair.entry.ReleaseTag, pair.entry.ReleaseCommit); err != nil {
			return err
		}
		head, err := resolveCommit(root, "HEAD")
		if err != nil {
			return err
		}
		if err := requireCommitAncestor(root, pair.entry.Kind+" release tooling", pair.entry.ReleaseToolingCommit, head); err != nil {
			return err
		}
	}
	return nil
}

type positionedPublishedEntry struct {
	matchedEntry
	position int
}

func validatePublishedPackageSet(set PublishedPackageSet, candidates CandidateSet) ([]positionedPublishedEntry, error) {
	if set.Format != publishedPackageSetFormat || set.Repository != publishedRepository {
		return nil, fmt.Errorf("format/repository does not identify the Takoform published-package set")
	}
	if set.DefinitionVersion != candidates.DefinitionVersion || set.PackageVersion != candidates.PackageVersion {
		return nil, fmt.Errorf("definition/package version does not match the compiled candidate set")
	}
	if set.PublicationStatus != "published-immutable" || set.AdmissionStatus != "external-required" || set.RevocationCheckpointStatus != "external-required" {
		return nil, fmt.Errorf("published packages must remain immutable publication proof with external admission and revocation proof")
	}
	if set.Trust.Path != publishedPackageTrustPath || !formpackage.ValidDigest(set.Trust.Digest) {
		return nil, fmt.Errorf("trust must pin %s by canonical SHA-256", publishedPackageTrustPath)
	}
	if len(set.Entries) != len(candidates.Entries) {
		return nil, fmt.Errorf("entry closure has %d entries, want exactly %d", len(set.Entries), len(candidates.Entries))
	}
	expected := make(map[string]Candidate, len(candidates.Entries))
	for _, candidate := range candidates.Entries {
		expected[candidate.Kind] = candidate
	}
	seenKinds := make(map[string]struct{}, len(set.Entries))
	seenReleaseIDs := make(map[int64]struct{}, len(set.Entries))
	ordered := make([]positionedPublishedEntry, 0, len(set.Entries))
	for position, entry := range set.Entries {
		candidate, ok := expected[entry.Kind]
		if !ok {
			return nil, fmt.Errorf("entries[%d] contains unknown kind %q", position, entry.Kind)
		}
		if _, duplicate := seenKinds[entry.Kind]; duplicate {
			return nil, fmt.Errorf("entries[%d] duplicates kind %q", position, entry.Kind)
		}
		seenKinds[entry.Kind] = struct{}{}
		if entry.Slug != candidate.Slug || entry.FormRef != candidate.FormRef || entry.PackageDigest != candidate.PackageDigest {
			return nil, fmt.Errorf("%s published identity does not match the compiled candidate", entry.Kind)
		}
		expectedReleaseTag := "forms/" + releaseIDForKind(entry.Kind) + "/v" + set.PackageVersion
		if entry.ReleaseTag != expectedReleaseTag || !packageReleaseTagPattern.MatchString(entry.ReleaseTag) ||
			!releaseCommitPattern.MatchString(entry.ReleaseCommit) || !releaseCommitPattern.MatchString(entry.ReleaseToolingCommit) {
			return nil, fmt.Errorf("%s does not bind the canonical immutable release ref", entry.Kind)
		}
		if entry.GitHubReleaseID <= 0 || !entry.Immutable {
			return nil, fmt.Errorf("%s does not retain an immutable GitHub Release identity", entry.Kind)
		}
		if _, duplicate := seenReleaseIDs[entry.GitHubReleaseID]; duplicate {
			return nil, fmt.Errorf("%s duplicates GitHub release id %d", entry.Kind, entry.GitHubReleaseID)
		}
		seenReleaseIDs[entry.GitHubReleaseID] = struct{}{}
		if _, err := time.Parse(time.RFC3339, entry.PublishedAt); err != nil {
			return nil, fmt.Errorf("%s publishedAt is not RFC 3339: %w", entry.Kind, err)
		}
		for label, digest := range map[string]string{
			"packageReleaseManifestDigest": entry.PackageReleaseManifestDigest,
			"checksumsDigest":              entry.ChecksumsDigest,
		} {
			if !formpackage.ValidDigest(digest) {
				return nil, fmt.Errorf("%s %s is not a canonical SHA-256", entry.Kind, label)
			}
		}
		setEntry := SetEntry{
			Kind: entry.Kind, Slug: entry.Slug, FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
			ReleaseTag: entry.ReleaseTag, ReleaseCommit: entry.ReleaseCommit, ReleaseToolingCommit: entry.ReleaseToolingCommit,
			PackageReleaseManifestPath:   entry.PackageReleaseManifestPath,
			PackageReleaseManifestDigest: entry.PackageReleaseManifestDigest,
			PackageIndexPath:             entry.PackageIndexPath, PackageIndexSigstoreBundle: entry.PackageIndexSigstoreBundle,
		}
		if err := validatePublishedReleasePaths(setEntry, entry.ChecksumsPath, set.PackageVersion); err != nil {
			return nil, fmt.Errorf("%s retained release paths: %w", entry.Kind, err)
		}
		ordered = append(ordered, positionedPublishedEntry{matchedEntry: matchedEntry{entry: setEntry, candidate: candidate}, position: position})
	}
	return ordered, nil
}

func validatePublishedReleasePaths(entry SetEntry, checksumsPath, packageVersion string) error {
	for label, value := range map[string]string{
		"packageReleaseManifestPath": entry.PackageReleaseManifestPath,
		"packageIndexPath":           entry.PackageIndexPath,
		"packageIndexSigstoreBundle": entry.PackageIndexSigstoreBundle,
		"checksumsPath":              checksumsPath,
	} {
		if err := validateRelativePath(value); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	releaseDirectory := path.Join("releases", releaseIDForKind(entry.Kind), packageVersion)
	base := "takoform-form-" + releaseIDForKind(entry.Kind) + "_" + packageVersion + "_package-index"
	if entry.PackageReleaseManifestPath != path.Join(releaseDirectory, "release-manifest.json") ||
		checksumsPath != path.Join(releaseDirectory, "SHA256SUMS") ||
		entry.PackageIndexPath != path.Join(releaseDirectory, base+".json") ||
		entry.PackageIndexSigstoreBundle != path.Join(releaseDirectory, base+".sigstore.json") {
		return fmt.Errorf("paths must use canonical directory %s and asset names", releaseDirectory)
	}
	return nil
}

func loadPublishedPackageTrust(admissionRoot string, ref PublishedPackageTrustRef) (PublishedPackageTrust, *offlineRoleVerifier, error) {
	raw, err := readRetainedRelativeFile(admissionRoot, ref.Path, maxOfflineSigstorePinsBytes)
	if err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	if formpackage.DigestBytes(raw) != ref.Digest {
		return PublishedPackageTrust{}, nil, fmt.Errorf("published-package trust digest mismatch")
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return PublishedPackageTrust{}, nil, fmt.Errorf("published-package trust must be RFC 8785-compatible I-JSON: %w", err)
	}
	var trust PublishedPackageTrust
	if err := decodeStrictJSON(raw, &trust); err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	if trust.Format != publishedPackageTrustFmt || trust.TrustedRoot.Path != canonicalTrustedRootPath ||
		trust.PackageIndexPolicy.Path != canonicalPackageIndexPolicyPath ||
		trust.RegistryReadbackPolicy.Path != canonicalRegistryReadbackPolicyPath {
		return PublishedPackageTrust{}, nil, fmt.Errorf("published-package trust paths/format are not canonical")
	}
	wantUnsettled := []string{roleAdmissionEvidence, roleHostReport, roleProviderReport}
	if len(trust.UnsettledPublisherRoles) != len(wantUnsettled) {
		return PublishedPackageTrust{}, nil, fmt.Errorf("published-package trust must retain the three unsettled publisher roles")
	}
	for index, role := range wantUnsettled {
		if trust.UnsettledPublisherRoles[index] != role {
			return PublishedPackageTrust{}, nil, fmt.Errorf("unsettledPublisherRoles[%d] is %q, want %q", index, trust.UnsettledPublisherRoles[index], role)
		}
	}
	trustedRootRaw, err := readPinnedRetainedFile(admissionRoot, "trusted root", trust.TrustedRoot, maxTrustedRootBytes)
	if err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	trustedRoot, err := sigstoreroot.NewTrustedRootFromJSON(trustedRootRaw)
	if err != nil {
		return PublishedPackageTrust{}, nil, fmt.Errorf("decode pinned trusted root: %w", err)
	}
	policyRaw, err := readPinnedRetainedFile(admissionRoot, "package-index publisher policy", trust.PackageIndexPolicy, maxPublisherPolicyBytes)
	if err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	if _, err := formpackage.Canonicalize(policyRaw); err != nil {
		return PublishedPackageTrust{}, nil, fmt.Errorf("package-index publisher policy must be RFC 8785-compatible I-JSON: %w", err)
	}
	var policy PublisherPolicy
	if err := decodeStrictJSON(policyRaw, &policy); err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	if policy.OIDCIssuer != packagePublisherIssuer || policy.CertificateIdentity != packagePublisherIdentity {
		return PublishedPackageTrust{}, nil, fmt.Errorf("package-index policy is not the protected package release workflow")
	}
	registryPolicyRaw, err := readPinnedRetainedFile(admissionRoot, "registry-readback publisher policy", trust.RegistryReadbackPolicy, maxPublisherPolicyBytes)
	if err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	if _, err := formpackage.Canonicalize(registryPolicyRaw); err != nil {
		return PublishedPackageTrust{}, nil, fmt.Errorf("registry-readback publisher policy must be RFC 8785-compatible I-JSON: %w", err)
	}
	var registryPolicy PublisherPolicy
	if err := decodeStrictJSON(registryPolicyRaw, &registryPolicy); err != nil {
		return PublishedPackageTrust{}, nil, err
	}
	if registryPolicy.OIDCIssuer != registryPublisherIssuer || registryPolicy.CertificateIdentity != registryPublisherIdentity {
		return PublishedPackageTrust{}, nil, fmt.Errorf("registry-readback policy is not the protected standard-admission workflow")
	}
	if _, err := newOfflineRoleVerifier(trustedRoot, registryPolicy); err != nil {
		return PublishedPackageTrust{}, nil, fmt.Errorf("registry-readback publisher policy: %w", err)
	}
	verifier, err := newOfflineRoleVerifier(trustedRoot, policy)
	return trust, verifier, err
}

func verifyReleaseChecksums(admissionRoot string, entry PublishedPackageEntry) error {
	raw, err := readRetainedRelativeFile(admissionRoot, entry.ChecksumsPath, maxReleaseManifestBytes)
	if err != nil {
		return err
	}
	if formpackage.DigestBytes(raw) != entry.ChecksumsDigest {
		return fmt.Errorf("SHA256SUMS digest mismatch")
	}
	manifestRaw, err := readRetainedRelativeFile(admissionRoot, entry.PackageReleaseManifestPath, maxReleaseManifestBytes)
	if err != nil {
		return err
	}
	var manifest packageReleaseManifest
	if err := decodeStrictJSON(manifestRaw, &manifest); err != nil {
		return err
	}
	expected := map[string]string{"release-manifest.json": entry.PackageReleaseManifestDigest}
	for _, asset := range manifest.Assets {
		expected[asset.Name] = asset.Digest
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != len(expected) || len(lines) != 6 {
		return fmt.Errorf("SHA256SUMS closure has %d lines, want exactly 6", len(lines))
	}
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || len(parts[0]) != 64 || path.Base(parts[1]) != parts[1] {
			return fmt.Errorf("invalid SHA256SUMS line %q", line)
		}
		want, ok := expected[parts[1]]
		if !ok || "sha256:"+parts[0] != want {
			return fmt.Errorf("SHA256SUMS does not bind %q to the release manifest", parts[1])
		}
		if _, duplicate := seen[parts[1]]; duplicate {
			return fmt.Errorf("SHA256SUMS duplicates %q", parts[1])
		}
		seen[parts[1]] = struct{}{}
	}
	return nil
}
