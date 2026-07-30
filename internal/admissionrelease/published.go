package admissionrelease

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	publishedPackageSetFormatV1 = "takoform.published-package-set@v1"
	publishedPackageSetFormatV2 = "takoform.published-package-set@v2"
	publishedPackageTrustPath   = "trust/published-package-trust.json"
	publishedPackageTrustFmtV1  = "takoform.published-package-trust@v1"
	publishedPackageTrustFmtV2  = "takoform.published-package-trust@v2"
	publishedRepository         = "tako0614/terraform-provider-takoform"
	registryPublisherIssuer     = "https://token.actions.githubusercontent.com"
	registryPublisherIdentityV1 = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/standard-admission-release.yml@refs/heads/main"
	currentPackagePublisherID   = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main"
)

// PublishedPackageSet is a source-reviewed immutable Form Package release
// closure. It proves only distribution publication; portable-standard
// admission remains governed by standard-admission-set.json.
type PublishedPackageSet struct {
	Format                     string                   `json:"format"`
	Repository                 string                   `json:"repository"`
	Generation                 string                   `json:"generation,omitempty"`
	DefinitionVersion          string                   `json:"definitionVersion,omitempty"`
	PackageVersion             string                   `json:"packageVersion,omitempty"`
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
	return VerifyPublishedPackageSetAt(root, admissionRootPath, candidates)
}

// VerifyPublishedPackageSetAt verifies one explicitly selected retained
// admission generation. Historical callers remain pinned to admission/v1;
// current mixed-version callers select their own immutable admission root.
func VerifyPublishedPackageSetAt(root, retainedRoot string, candidates CandidateSet) error {
	if err := validateCandidateSet(candidates); err != nil {
		return fmt.Errorf("published-package candidate set: %w", err)
	}
	if err := validateRelativePath(retainedRoot); err != nil {
		return fmt.Errorf("published-package retained root: %w", err)
	}
	setPath := path.Join(retainedRoot, "published-package-set.json")
	raw, err := readRetainedRelativeFile(root, setPath, maxSetBytes)
	if err != nil {
		return fmt.Errorf("read %s: %w", setPath, err)
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return fmt.Errorf("%s must contain RFC 8785-compatible I-JSON: %w", setPath, err)
	}
	var set PublishedPackageSet
	if err := decodeStrictJSON(raw, &set); err != nil {
		return fmt.Errorf("decode %s: %w", setPath, err)
	}
	ordered, err := validatePublishedPackageSet(set, candidates)
	if err != nil {
		return fmt.Errorf("verify %s: %w", setPath, err)
	}
	admissionRoot := admissionRootPathFor(root, retainedRoot)
	return verifyPublishedPackageSetValue(root, admissionRoot, admissionRoot, set, ordered)
}

// VerifyPublishedPackageSetValue authenticates a caller-decoded immutable
// publication manifest against one evidence root. A distinct trust root is
// accepted so an external create-only download can be verified against the
// reviewed repository policy before those exact bytes are retained under the
// admission root.
func VerifyPublishedPackageSetValue(
	repositoryRoot string,
	evidenceRoot string,
	trustRoot string,
	candidates CandidateSet,
	set PublishedPackageSet,
) error {
	if err := validateCandidateSet(candidates); err != nil {
		return fmt.Errorf("published-package candidate set: %w", err)
	}
	ordered, err := validatePublishedPackageSet(set, candidates)
	if err != nil {
		return fmt.Errorf("verify published package set value: %w", err)
	}
	return verifyPublishedPackageSetValue(repositoryRoot, evidenceRoot, trustRoot, set, ordered)
}

func verifyPublishedPackageSetValue(
	repositoryRoot string,
	evidenceRoot string,
	trustRoot string,
	set PublishedPackageSet,
	ordered []positionedPublishedEntry,
) error {
	_, verifier, err := loadPublishedPackageTrust(trustRoot, set.Trust, set)
	if err != nil {
		return fmt.Errorf("published-package trust: %w", err)
	}
	for _, pair := range ordered {
		packageVersion := set.PackageVersion
		if set.Generation != "" {
			packageVersion = pair.entry.FormRef.DefinitionVersion
		}
		indexRaw, err := verifyPackageReleaseReadback(repositoryRoot, evidenceRoot, pair.matchedEntry, packageVersion, set.Generation != "")
		if err != nil {
			return fmt.Errorf("%s package release readback: %w", pair.entry.Kind, err)
		}
		published := set.Entries[pair.position]
		if err := verifyReleaseChecksums(evidenceRoot, published); err != nil {
			return fmt.Errorf("%s release checksums: %w", pair.entry.Kind, err)
		}
		bundleRaw, err := readRetainedRelativeFile(evidenceRoot, pair.entry.PackageIndexSigstoreBundle, maxSigstoreBundleBytes)
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
		if err := requireTagCommit(repositoryRoot, pair.entry.Kind+" package release", pair.entry.ReleaseTag, pair.entry.ReleaseCommit); err != nil {
			return err
		}
		head, err := resolveCommit(repositoryRoot, "HEAD")
		if err != nil {
			return err
		}
		if err := requireCommitAncestor(repositoryRoot, pair.entry.Kind+" release tooling", pair.entry.ReleaseToolingCommit, head); err != nil {
			return err
		}
	}
	return nil
}

func admissionRootPathFor(root, retainedRoot string) string {
	return filepath.Join(root, filepath.FromSlash(retainedRoot))
}

type positionedPublishedEntry struct {
	matchedEntry
	position int
}

func validatePublishedPackageSet(set PublishedPackageSet, candidates CandidateSet) ([]positionedPublishedEntry, error) {
	if set.Repository != publishedRepository {
		return nil, fmt.Errorf("format/repository does not identify the Takoform published-package set")
	}
	if candidates.Generation == "" {
		if set.Format != publishedPackageSetFormatV1 || set.Generation != "" ||
			set.DefinitionVersion != candidates.DefinitionVersion || set.PackageVersion != candidates.PackageVersion {
			return nil, fmt.Errorf("definition/package version does not match the compiled candidate set")
		}
	} else if set.Format != publishedPackageSetFormatV2 || set.Generation != candidates.Generation ||
		set.DefinitionVersion != "" || set.PackageVersion != "" {
		return nil, fmt.Errorf("generation does not match the compiled mixed-version candidate set")
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
		packageVersion := set.PackageVersion
		if set.Generation != "" {
			packageVersion = entry.FormRef.DefinitionVersion
		}
		expectedReleaseTag := "forms/" + releaseIDForKind(entry.Kind) + "/v" + packageVersion
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
		if err := validatePublishedReleasePaths(setEntry, entry.ChecksumsPath, packageVersion); err != nil {
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

func loadPublishedPackageTrust(admissionRoot string, ref PublishedPackageTrustRef, set PublishedPackageSet) (PublishedPackageTrust, *offlineRoleVerifier, error) {
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
	if trust.TrustedRoot.Path != canonicalTrustedRootPath ||
		trust.PackageIndexPolicy.Path != canonicalPackageIndexPolicyPath {
		return PublishedPackageTrust{}, nil, fmt.Errorf("published-package trust paths/format are not canonical")
	}
	wantUnsettled := []string{roleAdmissionEvidence, roleHostReport, roleProviderReport}
	if set.Generation == "" {
		if trust.Format != publishedPackageTrustFmtV1 || trust.RegistryReadbackPolicy.Path != canonicalRegistryReadbackPolicyPath {
			return PublishedPackageTrust{}, nil, fmt.Errorf("historical published-package trust paths/format are not canonical")
		}
	} else {
		if trust.Format != publishedPackageTrustFmtV2 || trust.RegistryReadbackPolicy != (RetainedFile{}) {
			return PublishedPackageTrust{}, nil, fmt.Errorf("current published-package trust paths/format are not canonical")
		}
		wantUnsettled = append(wantUnsettled, roleRegistryReadback)
	}
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
	wantPackageIdentity := currentPackagePublisherID
	if set.Generation == "" {
		wantPackageIdentity = packagePublisherIdentity(set.PackageVersion)
	}
	if policy.OIDCIssuer != packagePublisherIssuer || policy.CertificateIdentity != wantPackageIdentity {
		return PublishedPackageTrust{}, nil, fmt.Errorf("package-index policy is not the protected package release workflow")
	}
	if set.Generation != "" {
		verifier, err := newOfflineRoleVerifier(trustedRoot, policy)
		return trust, verifier, err
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
	if registryPolicy.OIDCIssuer != registryPublisherIssuer || registryPolicy.CertificateIdentity != registryPublisherIdentityV1 {
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
