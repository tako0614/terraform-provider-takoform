package formpublication

import (
	"bytes"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

const (
	maxSetBytes         = 4 << 20
	maxReleasePlanBytes = 1 << 20
	maxTrustedRootBytes = 4 << 20
	maxAssetBytes       = 64 << 20
	maxClosureBytes     = 512 << 20
)

var (
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	slugPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
	versionPattern = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]*$`)
)

// VerifyAt authenticates the retained all-Form publication closure under one
// repository-relative admission generation.
func VerifyAt(
	repositoryRoot string,
	retainedRoot string,
	expected admissionrelease.CandidateSet,
) (Set, error) {
	if err := validateRelativePath(retainedRoot); err != nil {
		return Set{}, fmt.Errorf("publication retained root: %w", err)
	}
	root := filepath.Join(repositoryRoot, filepath.FromSlash(retainedRoot))
	return Verify(repositoryRoot, root, root, expected)
}

// Verify authenticates one publication directory against repository-pinned
// trust. It first runs the same structural verifier exposed to download
// staging, then proves Git authority and performs the existing complete
// offline release/Sigstore verification through admissionrelease.
func Verify(
	repositoryRoot string,
	publicationRoot string,
	trustRoot string,
	expected admissionrelease.CandidateSet,
) (Set, error) {
	set, err := VerifyStructure(publicationRoot, expected)
	if err != nil {
		return Set{}, err
	}
	publicationRoot, err = resolveRealDirectory(publicationRoot)
	if err != nil {
		return Set{}, fmt.Errorf("publication root: %w", err)
	}
	trustRoot, err = resolveRealDirectory(trustRoot)
	if err != nil {
		return Set{}, fmt.Errorf("publication trust root: %w", err)
	}
	repositoryRoot, err = resolveRealDirectory(repositoryRoot)
	if err != nil {
		return Set{}, fmt.Errorf("source repository root: %w", err)
	}

	publicationTrustedRoot, err := readRelativeRegularFile(
		publicationRoot, set.VerificationPolicy.TrustedRoot.Path, maxTrustedRootBytes,
	)
	if err != nil {
		return Set{}, fmt.Errorf("publication trusted root: %w", err)
	}
	pinnedTrustedRoot, err := readRelativeRegularFile(
		trustRoot, trustedRootPublicationPath, maxTrustedRootBytes,
	)
	if err != nil {
		return Set{}, fmt.Errorf("repository-pinned trusted root: %w", err)
	}
	if !bytes.Equal(publicationTrustedRoot, pinnedTrustedRoot) {
		return Set{}, fmt.Errorf("publication trusted root differs from the repository-pinned trust root")
	}
	if err := verifyGitAuthority(repositoryRoot, publicationRoot, set); err != nil {
		return Set{}, fmt.Errorf("publication Git authority: %w", err)
	}
	projected, err := projectPublishedPackageSet(trustRoot, set, expected)
	if err != nil {
		return Set{}, fmt.Errorf("publication projection: %w", err)
	}
	if err := admissionrelease.VerifyPublishedPackageSetValue(
		repositoryRoot, publicationRoot, trustRoot, expected, projected,
	); err != nil {
		return Set{}, fmt.Errorf("publication release authentication: %w", err)
	}
	return set, nil
}

// VerifyStructure checks the exact manifest, release plan, authority files,
// and seven-asset filesystem closure without invoking Git or cryptography.
// Download staging uses this seam before the complete Verify step so synthetic
// tests can exercise the same schema and byte-closure rules.
func VerifyStructure(
	publicationRoot string,
	expected admissionrelease.CandidateSet,
) (Set, error) {
	if err := validatePortableCandidateSet(expected); err != nil {
		return Set{}, fmt.Errorf("publication candidate set: %w", err)
	}
	root, err := resolveRealDirectory(publicationRoot)
	if err != nil {
		return Set{}, fmt.Errorf("publication root: %w", err)
	}
	setRaw, err := readRelativeRegularFile(root, SetFilename, maxSetBytes)
	if err != nil {
		return Set{}, fmt.Errorf("read %s: %w", SetFilename, err)
	}
	canonicalSet, err := formpackage.Canonicalize(setRaw)
	if err != nil {
		return Set{}, fmt.Errorf("%s must contain RFC 8785-compatible I-JSON: %w", SetFilename, err)
	}
	if !bytes.Equal(setRaw, canonicalSet) {
		return Set{}, fmt.Errorf("%s bytes are not RFC 8785 canonical", SetFilename)
	}
	var set Set
	if err := decodeStrictJSON(setRaw, &set); err != nil {
		return Set{}, fmt.Errorf("decode %s: %w", SetFilename, err)
	}
	if err := validateSetIdentity(set); err != nil {
		return Set{}, err
	}

	planRaw, err := readRelativeRegularFile(root, set.SourcePlan.Path, maxReleasePlanBytes)
	if err != nil {
		return Set{}, fmt.Errorf("source release plan: %w", err)
	}
	if formpackage.DigestBytes(planRaw) != set.SourcePlan.SHA256 {
		return Set{}, fmt.Errorf("source release plan digest mismatch")
	}
	plan, err := decodeReleasePlan(planRaw)
	if err != nil {
		return Set{}, fmt.Errorf("source release plan: %w", err)
	}
	if err := validateReleasePlan(plan, expected); err != nil {
		return Set{}, fmt.Errorf("source release plan: %w", err)
	}

	trustedRootRaw, err := readRelativeRegularFile(
		root, set.VerificationPolicy.TrustedRoot.Path, maxTrustedRootBytes,
	)
	if err != nil {
		return Set{}, fmt.Errorf("publication trusted root: %w", err)
	}
	if formpackage.DigestBytes(trustedRootRaw) != set.VerificationPolicy.TrustedRoot.SHA256 {
		return Set{}, fmt.Errorf("publication trusted root digest mismatch")
	}
	trustedRootCanonical, err := formpackage.Canonicalize(trustedRootRaw)
	if err != nil {
		return Set{}, fmt.Errorf("publication trusted root is not I-JSON: %w", err)
	}
	if _, err := sigstoreroot.NewTrustedRootFromJSON(trustedRootRaw); err != nil {
		return Set{}, fmt.Errorf("decode publication Sigstore trusted root: %w", err)
	}

	expectedFiles := make(map[string]struct{}, exactPortablePackageCount*exactPackageReleaseAssetCount+4)
	type retainedAuthority struct {
		planRaw []byte
		plan    releasePlan
		rootRaw []byte
	}
	authorities := make(map[string]retainedAuthority)
	seenReleaseIDs := make(map[string]struct{}, len(set.Entries))
	seenTags := make(map[string]struct{}, len(set.Entries))
	seenTagObjects := make(map[string]struct{}, len(set.Entries))
	seenGitHubReleaseIDs := make(map[string]struct{}, len(set.Entries))
	totalAssetBytes := int64(0)

	if len(set.Entries) != exactPortablePackageCount {
		return Set{}, fmt.Errorf(
			"publication entry closure has %d entries, want exactly %d",
			len(set.Entries), exactPortablePackageCount,
		)
	}
	for position, entry := range set.Entries {
		candidate := expected.Entries[position]
		planned := plan.Releases[position]
		if err := validateProjectableEntry(position, entry); err != nil {
			return Set{}, err
		}
		if err := validateEntryIdentity(position, entry, candidate, planned); err != nil {
			return Set{}, err
		}
		for _, identity := range []struct {
			label string
			value string
			seen  map[string]struct{}
		}{
			{"release id", entry.ReleaseID, seenReleaseIDs},
			{"tag", entry.Tag, seenTags},
			{"tag object", entry.TagObjectOID, seenTagObjects},
			{"GitHub release id", entry.GitHubReleaseID, seenGitHubReleaseIDs},
		} {
			if _, duplicate := identity.seen[identity.value]; duplicate {
				return Set{}, fmt.Errorf(
					"entries[%d] duplicates %s %q",
					position, identity.label, identity.value,
				)
			}
			identity.seen[identity.value] = struct{}{}
		}

		authorityDirectory := path.Join("authority", entry.ToolingCommit)
		wantPlanPath := path.Join(authorityDirectory, "release-plan.json")
		wantRootPath := path.Join(authorityDirectory, "trusted-root.json")
		if entry.ReleasePlan != (SourcePlan{
			Path: wantPlanPath, SourcePath: releasePlanSourcePath, SHA256: entry.ReleasePlan.SHA256,
		}) || !formpackage.ValidDigest(entry.ReleasePlan.SHA256) {
			return Set{}, fmt.Errorf("entries[%d] releasePlan does not pin the exact tooling authority", position)
		}
		if entry.TrustedRoot != (SourcePlan{
			Path: wantRootPath, SourcePath: trustedRootSourcePath,
			SHA256: entry.TrustedRoot.SHA256,
		}) || !formpackage.ValidDigest(entry.TrustedRoot.SHA256) {
			return Set{}, fmt.Errorf("entries[%d] trustedRoot does not pin the exact tooling authority", position)
		}
		expectedFiles[wantPlanPath] = struct{}{}
		expectedFiles[wantRootPath] = struct{}{}
		authority, loaded := authorities[entry.ToolingCommit]
		if !loaded {
			retainedPlan, err := readRelativeRegularFile(root, wantPlanPath, maxReleasePlanBytes)
			if err != nil {
				return Set{}, fmt.Errorf("entries[%d] authority release plan: %w", position, err)
			}
			if formpackage.DigestBytes(retainedPlan) != entry.ReleasePlan.SHA256 {
				return Set{}, fmt.Errorf("entries[%d] authority release plan digest mismatch", position)
			}
			authorityPlan, err := decodeReleasePlan(retainedPlan)
			if err != nil {
				return Set{}, fmt.Errorf("entries[%d] authority release plan: %w", position, err)
			}
			if err := validateAuthorityReleasePlan(authorityPlan); err != nil {
				return Set{}, fmt.Errorf("entries[%d] authority release plan: %w", position, err)
			}
			retainedRoot, err := readRelativeRegularFile(root, wantRootPath, maxTrustedRootBytes)
			if err != nil {
				return Set{}, fmt.Errorf("entries[%d] authority trusted root: %w", position, err)
			}
			if formpackage.DigestBytes(retainedRoot) != entry.TrustedRoot.SHA256 {
				return Set{}, fmt.Errorf("entries[%d] authority trusted root digest mismatch", position)
			}
			retainedRootCanonical, err := formpackage.Canonicalize(retainedRoot)
			if err != nil {
				return Set{}, fmt.Errorf("entries[%d] authority trusted root is not I-JSON: %w", position, err)
			}
			if _, err := sigstoreroot.NewTrustedRootFromJSON(retainedRoot); err != nil {
				return Set{}, fmt.Errorf("entries[%d] authority Sigstore trusted root: %w", position, err)
			}
			if !bytes.Equal(retainedRootCanonical, trustedRootCanonical) {
				return Set{}, fmt.Errorf(
					"entries[%d] authority trusted root differs semantically from the publication trusted root",
					position,
				)
			}
			authority = retainedAuthority{
				planRaw: retainedPlan, plan: authorityPlan, rootRaw: retainedRoot,
			}
			authorities[entry.ToolingCommit] = authority
		} else if formpackage.DigestBytes(authority.planRaw) != entry.ReleasePlan.SHA256 ||
			formpackage.DigestBytes(authority.rootRaw) != entry.TrustedRoot.SHA256 {
			return Set{}, fmt.Errorf("entries[%d] changes shared tooling authority digests", position)
		}
		if err := requireAuthoritySelection(authority.plan, candidate, entry); err != nil {
			return Set{}, fmt.Errorf("entries[%d] authority release plan: %w", position, err)
		}

		wantAssetNames := canonicalAssetNames(entry.ReleaseID, entry.Version)
		if len(entry.Assets) != exactPackageReleaseAssetCount {
			return Set{}, fmt.Errorf(
				"entries[%d] asset closure has %d entries, want exactly %d",
				position, len(entry.Assets), exactPackageReleaseAssetCount,
			)
		}
		releaseDirectory := path.Join("releases", entry.ReleaseID, entry.Version)
		for assetPosition, asset := range entry.Assets {
			if asset.Name != wantAssetNames[assetPosition] {
				return Set{}, fmt.Errorf(
					"entries[%d].assets[%d] is %q, want canonical %q",
					position, assetPosition, asset.Name, wantAssetNames[assetPosition],
				)
			}
			if !formpackage.ValidDigest(asset.SHA256) || asset.Size <= 0 || asset.Size > maxAssetBytes {
				return Set{}, fmt.Errorf(
					"entries[%d].assets[%d] has an invalid digest or size",
					position, assetPosition,
				)
			}
			assetPath := path.Join(releaseDirectory, asset.Name)
			raw, err := readRelativeRegularFile(root, assetPath, maxAssetBytes)
			if err != nil {
				return Set{}, fmt.Errorf("entries[%d] asset %q: %w", position, asset.Name, err)
			}
			if int64(len(raw)) != asset.Size || formpackage.DigestBytes(raw) != asset.SHA256 {
				return Set{}, fmt.Errorf("entries[%d] asset %q byte readback mismatch", position, asset.Name)
			}
			totalAssetBytes += int64(len(raw))
			if totalAssetBytes > maxClosureBytes {
				return Set{}, fmt.Errorf("publication asset closure exceeds %d bytes", maxClosureBytes)
			}
			expectedFiles[assetPath] = struct{}{}
		}
	}
	if err := verifyExactSubtree(root, "authority", expectedFiles); err != nil {
		return Set{}, err
	}
	if err := verifyExactSubtree(root, "releases", expectedFiles); err != nil {
		return Set{}, err
	}
	return set, nil
}

func validateSetIdentity(set Set) error {
	if set.Format != SetFormat || set.Generation != portableGeneration ||
		set.Repository != repository {
		return fmt.Errorf("%s does not identify the exact portable Takoform publication set", SetFilename)
	}
	if set.PublicationStatus != publicationStatus ||
		set.AdmissionStatus != externalRequiredStatus ||
		set.RevocationCheckpointStatus != externalRequiredStatus {
		return fmt.Errorf("%s changes the publication/admission/revocation authority boundary", SetFilename)
	}
	if set.GitObjectFormat != gitObjectFormat || !commitPattern.MatchString(set.ProtectedMainCommit) {
		return fmt.Errorf("%s does not pin an exact SHA-1 protected-main commit", SetFilename)
	}
	if set.SourcePlan.Path != "release-plan.json" ||
		set.SourcePlan.SourcePath != releasePlanSourcePath ||
		!formpackage.ValidDigest(set.SourcePlan.SHA256) {
		return fmt.Errorf("%s sourcePlan is not the canonical release-plan pin", SetFilename)
	}
	if set.VerificationPolicy.TrustedRoot.Path != trustedRootPublicationPath ||
		set.VerificationPolicy.TrustedRoot.SourcePath != trustedRootSourcePath ||
		!formpackage.ValidDigest(set.VerificationPolicy.TrustedRoot.SHA256) ||
		set.VerificationPolicy.CertificateIdentity != packageWorkflowIdentity ||
		set.VerificationPolicy.OIDCIssuer != packageWorkflowOIDCIssuer ||
		set.VerificationPolicy.BundleMediaType != sigstoreBundleMediaTypeV03 {
		return fmt.Errorf("%s verificationPolicy is not the protected Form Package workflow", SetFilename)
	}
	return nil
}

func validatePortableCandidateSet(expected admissionrelease.CandidateSet) error {
	if expected.Generation != portableGeneration || expected.DefinitionVersion != "" ||
		expected.PackageVersion != "" || len(expected.Entries) != exactPortablePackageCount {
		return fmt.Errorf(
			"candidate set must be the exact %s %d-entry generation",
			portableGeneration, exactPortablePackageCount,
		)
	}
	seenKinds := make(map[string]struct{}, len(expected.Entries))
	seenSlugs := make(map[string]struct{}, len(expected.Entries))
	for index, candidate := range expected.Entries {
		if candidate.Kind == "" || candidate.FormRef.APIVersion != formpackage.FormAPIVersion ||
			candidate.FormRef.Kind != candidate.Kind || !versionPattern.MatchString(candidate.FormRef.DefinitionVersion) ||
			!formpackage.ValidDigest(candidate.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(candidate.PackageDigest) ||
			!slugPattern.MatchString(candidate.Slug) {
			return fmt.Errorf("entries[%d] has an invalid exact candidate identity", index)
		}
		if err := validateRelativePath(candidate.PackagePath); err != nil {
			return fmt.Errorf("entries[%d] package path: %w", index, err)
		}
		if _, duplicate := seenKinds[candidate.Kind]; duplicate {
			return fmt.Errorf("entries[%d] duplicates kind %q", index, candidate.Kind)
		}
		if _, duplicate := seenSlugs[candidate.Slug]; duplicate {
			return fmt.Errorf("entries[%d] duplicates slug %q", index, candidate.Slug)
		}
		seenKinds[candidate.Kind] = struct{}{}
		seenSlugs[candidate.Slug] = struct{}{}
	}
	return nil
}

func decodeReleasePlan(raw []byte) (releasePlan, error) {
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return releasePlan{}, fmt.Errorf("not RFC 8785-compatible I-JSON: %w", err)
	}
	var plan releasePlan
	if err := decodeStrictJSON(raw, &plan); err != nil {
		return releasePlan{}, err
	}
	return plan, nil
}

func validateReleasePlan(plan releasePlan, expected admissionrelease.CandidateSet) error {
	if plan.Format != releasePlanFormat || plan.Generation != portableGeneration ||
		plan.Repository != repository || strings.TrimSpace(plan.Note) == "" ||
		len(plan.Releases) != exactPortablePackageCount {
		return fmt.Errorf("does not identify the exact %d-release portable plan", exactPortablePackageCount)
	}
	for index, planned := range plan.Releases {
		candidate := expected.Entries[index]
		releaseID, err := releaseIDForKind(candidate.Kind)
		if err != nil {
			return fmt.Errorf("releases[%d]: %w", index, err)
		}
		version := candidate.FormRef.DefinitionVersion
		want := plannedRelease{
			Kind: candidate.Kind, Slug: candidate.Slug, ReleaseID: releaseID,
			Version: version, Tag: "forms/" + releaseID + "/v" + version,
			SourcePath: path.Join("forms", "releases", releaseID, version),
			FormRef:    candidate.FormRef, PackageDigest: candidate.PackageDigest,
		}
		if planned != want {
			return fmt.Errorf("releases[%d] does not equal compiled candidate %q", index, candidate.Kind)
		}
	}
	return nil
}

func validateAuthorityReleasePlan(plan releasePlan) error {
	if plan.Format != releasePlanFormat || plan.Generation != portableGeneration ||
		plan.Repository != repository || strings.TrimSpace(plan.Note) == "" ||
		len(plan.Releases) != exactPortablePackageCount {
		return fmt.Errorf("does not identify an exact %d-release portable authority", exactPortablePackageCount)
	}
	seenKinds := make(map[string]struct{}, len(plan.Releases))
	seenReleaseIDs := make(map[string]struct{}, len(plan.Releases))
	seenTags := make(map[string]struct{}, len(plan.Releases))
	for index, planned := range plan.Releases {
		releaseID, err := releaseIDForKind(planned.Kind)
		if err != nil {
			return fmt.Errorf("releases[%d]: %w", index, err)
		}
		if planned.ReleaseID != releaseID || !slugPattern.MatchString(planned.Slug) ||
			!versionPattern.MatchString(planned.Version) ||
			planned.Tag != "forms/"+releaseID+"/v"+planned.Version ||
			planned.SourcePath != path.Join("forms", "releases", releaseID, planned.Version) ||
			planned.FormRef.APIVersion != formpackage.FormAPIVersion ||
			planned.FormRef.Kind != planned.Kind ||
			planned.FormRef.DefinitionVersion != planned.Version ||
			!formpackage.ValidDigest(planned.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(planned.PackageDigest) {
			return fmt.Errorf("releases[%d] has an invalid exact release identity", index)
		}
		if _, duplicate := seenKinds[planned.Kind]; duplicate {
			return fmt.Errorf("releases[%d] duplicates kind %q", index, planned.Kind)
		}
		if _, duplicate := seenReleaseIDs[planned.ReleaseID]; duplicate {
			return fmt.Errorf("releases[%d] duplicates release id %q", index, planned.ReleaseID)
		}
		if _, duplicate := seenTags[planned.Tag]; duplicate {
			return fmt.Errorf("releases[%d] duplicates tag %q", index, planned.Tag)
		}
		seenKinds[planned.Kind] = struct{}{}
		seenReleaseIDs[planned.ReleaseID] = struct{}{}
		seenTags[planned.Tag] = struct{}{}
	}
	return nil
}

func requireAuthoritySelection(
	plan releasePlan,
	candidate admissionrelease.Candidate,
	entry Entry,
) error {
	want := plannedRelease{
		Kind: candidate.Kind, Slug: candidate.Slug, ReleaseID: entry.ReleaseID,
		Version: entry.Version, Tag: entry.Tag, SourcePath: entry.SourcePath,
		FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
	}
	found := false
	for _, planned := range plan.Releases {
		if planned.Kind == entry.Kind || planned.Tag == entry.Tag {
			if found || planned != want {
				return fmt.Errorf(
					"kind %q and tag %q do not select one exact historical authority entry",
					entry.Kind, entry.Tag,
				)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf(
			"kind %q and tag %q are absent from historical tooling authority",
			entry.Kind, entry.Tag,
		)
	}
	return nil
}

func validateEntryIdentity(
	position int,
	entry Entry,
	candidate admissionrelease.Candidate,
	planned plannedRelease,
) error {
	if entry.Kind != candidate.Kind || entry.FormRef != candidate.FormRef ||
		entry.PackageDigest != candidate.PackageDigest ||
		entry.Kind != planned.Kind || entry.ReleaseID != planned.ReleaseID ||
		entry.Version != planned.Version || entry.Tag != planned.Tag ||
		entry.SourcePath != planned.SourcePath {
		return fmt.Errorf("entries[%d] does not equal compiled/planned candidate %q", position, candidate.Kind)
	}
	return nil
}

func projectPublishedPackageSet(
	trustRoot string,
	set Set,
	expected admissionrelease.CandidateSet,
) (admissionrelease.PublishedPackageSet, error) {
	trustRaw, err := readRelativeRegularFile(trustRoot, publishedTrustPath, maxSetBytes)
	if err != nil {
		return admissionrelease.PublishedPackageSet{}, fmt.Errorf("published-package trust: %w", err)
	}
	entriesByKind, err := ProjectEntries(set, expected)
	if err != nil {
		return admissionrelease.PublishedPackageSet{}, err
	}
	projected := admissionrelease.PublishedPackageSet{
		Format:                     "takoform.published-package-set@v2",
		Repository:                 repository,
		Generation:                 portableGeneration,
		PublicationStatus:          publicationStatus,
		AdmissionStatus:            externalRequiredStatus,
		RevocationCheckpointStatus: externalRequiredStatus,
		Trust: admissionrelease.PublishedPackageTrustRef{
			Path: publishedTrustPath, Digest: formpackage.DigestBytes(trustRaw),
		},
		Entries: make([]admissionrelease.PublishedPackageEntry, 0, len(set.Entries)),
	}
	for _, entry := range set.Entries {
		projected.Entries = append(projected.Entries, entriesByKind[entry.Kind])
	}
	return projected, nil
}

func canonicalAssetNames(releaseID, version string) []string {
	base := "takoform-form-" + releaseID + "_" + version
	names := []string{
		"release-manifest.json",
		"SHA256SUMS",
		base + ".tar.gz",
		base + "_package-index.json",
		base + "_package-index.sigstore.json",
		base + "_provenance.intoto.json",
		base + "_sbom.spdx.json",
	}
	sort.Strings(names)
	return names
}

func releaseIDForKind(kind string) (string, error) {
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind))
	return "k-" + strings.ToLower(encoded), nil
}

func verifyExactSubtree(root, subtree string, expectedFiles map[string]struct{}) error {
	subtreeRoot := filepath.Join(root, filepath.FromSlash(subtree))
	info, err := os.Lstat(subtreeRoot)
	if err != nil {
		return fmt.Errorf("inspect publication %s closure: %w", subtree, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("publication %s closure must be a real directory", subtree)
	}
	expectedDirectories := map[string]struct{}{subtree: {}}
	for relative := range expectedFiles {
		if relative != subtree && !strings.HasPrefix(relative, subtree+"/") {
			continue
		}
		for directory := path.Dir(relative); directory != "." && directory != "/"; directory = path.Dir(directory) {
			expectedDirectories[directory] = struct{}{}
			if directory == subtree {
				break
			}
		}
	}
	seenFiles := make(map[string]struct{})
	err = filepath.WalkDir(subtreeRoot, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativeOS, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative := filepath.ToSlash(relativeOS)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("publication %s closure contains symlink %q", subtree, relative)
		}
		if entry.IsDir() {
			if _, ok := expectedDirectories[relative]; !ok {
				return fmt.Errorf("publication %s closure contains extra directory %q", subtree, relative)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("publication %s closure contains non-regular entry %q", subtree, relative)
		}
		if _, ok := expectedFiles[relative]; !ok {
			return fmt.Errorf("publication %s closure contains extra file %q", subtree, relative)
		}
		seenFiles[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	for relative := range expectedFiles {
		if relative != subtree && !strings.HasPrefix(relative, subtree+"/") {
			continue
		}
		if _, ok := seenFiles[relative]; !ok {
			return fmt.Errorf("publication %s closure omits %q", subtree, relative)
		}
	}
	return nil
}

func resolveRealDirectory(name string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(name))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%q must be a real directory, not a symlink", name)
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if filepath.Clean(real) != absolute {
		return "", fmt.Errorf("%q must not traverse symlinks", name)
	}
	return absolute, nil
}

func readRelativeRegularFile(root, relative string, maximum int64) ([]byte, error) {
	if err := validateRelativePath(relative); err != nil {
		return nil, err
	}
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q must be a regular file, not a symlink", relative)
	}
	if info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("%q size must be within 1..%d bytes", relative, maximum)
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(real) != filepath.Clean(absolute) {
		return nil, fmt.Errorf("%q must not traverse symlinks", relative)
	}
	handle, err := os.Open(absolute)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	openedInfo, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("%q changed before it was opened", relative)
	}
	raw, err := io.ReadAll(io.LimitReader(handle, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("%q exceeded %d bytes while it was read", relative, maximum)
	}
	if int64(len(raw)) != openedInfo.Size() {
		return nil, fmt.Errorf("%q changed while it was read", relative)
	}
	return raw, nil
}

func validateRelativePath(value string) error {
	if value == "" || strings.Contains(value, `\`) || path.IsAbs(value) ||
		path.Clean(value) != value || value == "." || value == ".." ||
		strings.HasPrefix(value, "../") {
		return fmt.Errorf("%q is not a clean relative slash path", value)
	}
	return nil
}

func decodeStrictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON contains a trailing value")
		}
		return fmt.Errorf("JSON trailing bytes: %w", err)
	}
	return nil
}
