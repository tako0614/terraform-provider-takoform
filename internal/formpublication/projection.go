package formpublication

import (
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

// ProjectEntries selects exact compiled candidates from an already verified
// all-Form publication Set and converts them to the shared release-readback
// shape. It revalidates the fail-closed set identity and canonical seven-asset
// mapping so admission builders do not need to duplicate publication logic.
func ProjectEntries(
	set Set,
	selected admissionrelease.CandidateSet,
) (map[string]admissionrelease.PublishedPackageEntry, error) {
	if err := validateSetIdentity(set); err != nil {
		return nil, err
	}
	if len(set.Entries) != exactPortablePackageCount {
		return nil, fmt.Errorf(
			"publication entry closure has %d entries, want exactly %d",
			len(set.Entries), exactPortablePackageCount,
		)
	}
	if err := validateProjectionCandidateSet(selected); err != nil {
		return nil, fmt.Errorf("selected candidate set: %w", err)
	}

	byKind := make(map[string]Entry, len(set.Entries))
	seenReleaseIDs := make(map[string]struct{}, len(set.Entries))
	seenTags := make(map[string]struct{}, len(set.Entries))
	seenTagObjects := make(map[string]struct{}, len(set.Entries))
	seenGitHubReleaseIDs := make(map[string]struct{}, len(set.Entries))
	for position, entry := range set.Entries {
		if err := validateProjectableEntry(position, entry); err != nil {
			return nil, err
		}
		if _, duplicate := byKind[entry.Kind]; duplicate {
			return nil, fmt.Errorf("entries[%d] duplicates kind %q", position, entry.Kind)
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
				return nil, fmt.Errorf(
					"entries[%d] duplicates %s %q",
					position, identity.label, identity.value,
				)
			}
			identity.seen[identity.value] = struct{}{}
		}
		byKind[entry.Kind] = entry
	}

	projected := make(map[string]admissionrelease.PublishedPackageEntry, len(selected.Entries))
	for position, candidate := range selected.Entries {
		entry, ok := byKind[candidate.Kind]
		if !ok {
			return nil, fmt.Errorf(
				"selected entries[%d] kind %q is absent from the all-Form publication",
				position, candidate.Kind,
			)
		}
		if entry.FormRef != candidate.FormRef ||
			entry.PackageDigest != candidate.PackageDigest ||
			entry.SourcePath != candidate.PackagePath {
			return nil, fmt.Errorf(
				"selected entries[%d] kind %q identity differs from the all-Form publication",
				position, candidate.Kind,
			)
		}
		release, err := projectEntry(entry, candidate)
		if err != nil {
			return nil, fmt.Errorf("selected entries[%d] kind %q: %w", position, candidate.Kind, err)
		}
		projected[candidate.Kind] = release
	}
	return projected, nil
}

func validateProjectionCandidateSet(selected admissionrelease.CandidateSet) error {
	if selected.Generation == "" || selected.DefinitionVersion != "" ||
		selected.PackageVersion != "" || len(selected.Entries) == 0 ||
		len(selected.Entries) > exactPortablePackageCount {
		return fmt.Errorf("one non-empty generation-aware subset is required")
	}
	seenKinds := make(map[string]struct{}, len(selected.Entries))
	seenSlugs := make(map[string]struct{}, len(selected.Entries))
	for position, candidate := range selected.Entries {
		if candidate.Kind == "" || !slugPattern.MatchString(candidate.Slug) ||
			candidate.FormRef.APIVersion != formpackage.FormAPIVersion ||
			candidate.FormRef.Kind != candidate.Kind ||
			!versionPattern.MatchString(candidate.FormRef.DefinitionVersion) ||
			!formpackage.ValidDigest(candidate.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(candidate.PackageDigest) {
			return fmt.Errorf("entries[%d] has an invalid exact candidate identity", position)
		}
		if err := validateRelativePath(candidate.PackagePath); err != nil {
			return fmt.Errorf("entries[%d] package path: %w", position, err)
		}
		if _, duplicate := seenKinds[candidate.Kind]; duplicate {
			return fmt.Errorf("entries[%d] duplicates kind %q", position, candidate.Kind)
		}
		if _, duplicate := seenSlugs[candidate.Slug]; duplicate {
			return fmt.Errorf("entries[%d] duplicates slug %q", position, candidate.Slug)
		}
		seenKinds[candidate.Kind] = struct{}{}
		seenSlugs[candidate.Slug] = struct{}{}
	}
	return nil
}

func validateProjectableEntry(position int, entry Entry) error {
	releaseID, err := releaseIDForKind(entry.Kind)
	if err != nil {
		return fmt.Errorf("entries[%d]: %w", position, err)
	}
	if entry.ReleaseID != releaseID ||
		entry.FormRef.APIVersion != formpackage.FormAPIVersion ||
		entry.FormRef.Kind != entry.Kind ||
		entry.FormRef.DefinitionVersion != entry.Version ||
		!versionPattern.MatchString(entry.Version) ||
		!formpackage.ValidDigest(entry.FormRef.SchemaDigest) ||
		!formpackage.ValidDigest(entry.PackageDigest) ||
		entry.Tag != "forms/"+releaseID+"/v"+entry.Version ||
		entry.SourcePath != path.Join("forms", "releases", releaseID, entry.Version) {
		return fmt.Errorf("entries[%d] has an invalid canonical package release identity", position)
	}
	if !commitPattern.MatchString(entry.TagObjectOID) ||
		!commitPattern.MatchString(entry.SourceCommit) ||
		!commitPattern.MatchString(entry.ToolingCommit) ||
		entry.PeeledCommit != entry.SourceCommit {
		return fmt.Errorf("entries[%d] does not bind exact SHA-1 tag/source/tooling objects", position)
	}
	authorityDirectory := path.Join("authority", entry.ToolingCommit)
	if entry.ReleasePlan.Path != path.Join(authorityDirectory, "release-plan.json") ||
		entry.ReleasePlan.SourcePath != releasePlanSourcePath ||
		!formpackage.ValidDigest(entry.ReleasePlan.SHA256) ||
		entry.TrustedRoot.Path != path.Join(authorityDirectory, "trusted-root.json") ||
		entry.TrustedRoot.SourcePath != trustedRootSourcePath ||
		!formpackage.ValidDigest(entry.TrustedRoot.SHA256) {
		return fmt.Errorf("entries[%d] does not pin canonical tooling authority paths", position)
	}
	releaseIDValue, err := strconv.ParseInt(entry.GitHubReleaseID, 10, 64)
	if err != nil || releaseIDValue <= 0 ||
		strconv.FormatInt(releaseIDValue, 10) != entry.GitHubReleaseID ||
		!entry.Immutable {
		return fmt.Errorf("entries[%d] does not retain an immutable canonical GitHub release id", position)
	}
	published, err := time.Parse(time.RFC3339, entry.PublishedAt)
	if err != nil || published.UTC().Format(time.RFC3339) != entry.PublishedAt {
		return fmt.Errorf("entries[%d] publishedAt is not canonical UTC RFC 3339", position)
	}
	wantNames := canonicalAssetNames(entry.ReleaseID, entry.Version)
	if len(entry.Assets) != exactPackageReleaseAssetCount {
		return fmt.Errorf(
			"entries[%d] asset closure has %d entries, want exactly %d",
			position, len(entry.Assets), exactPackageReleaseAssetCount,
		)
	}
	for assetPosition, asset := range entry.Assets {
		if asset.Name != wantNames[assetPosition] ||
			!formpackage.ValidDigest(asset.SHA256) ||
			asset.Size <= 0 || asset.Size > maxAssetBytes {
			return fmt.Errorf(
				"entries[%d].assets[%d] is not a canonical release asset",
				position, assetPosition,
			)
		}
	}
	base := "takoform-form-" + entry.ReleaseID + "_" + entry.Version
	indexDigest := ""
	for _, asset := range entry.Assets {
		if asset.Name == base+"_package-index.json" {
			indexDigest = asset.SHA256
			break
		}
	}
	if indexDigest != entry.PackageDigest {
		return fmt.Errorf("entries[%d] package-index asset digest differs from packageDigest", position)
	}
	return nil
}

func projectEntry(
	entry Entry,
	candidate admissionrelease.Candidate,
) (admissionrelease.PublishedPackageEntry, error) {
	assets := make(map[string]Asset, len(entry.Assets))
	for _, asset := range entry.Assets {
		assets[asset.Name] = asset
	}
	base := "takoform-form-" + entry.ReleaseID + "_" + entry.Version
	manifest, manifestOK := assets["release-manifest.json"]
	checksums, checksumsOK := assets["SHA256SUMS"]
	indexName := base + "_package-index.json"
	bundleName := base + "_package-index.sigstore.json"
	if !manifestOK || !checksumsOK {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("canonical manifest/checksums assets are absent")
	}
	if _, ok := assets[indexName]; !ok {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("canonical package-index asset is absent")
	}
	if _, ok := assets[bundleName]; !ok {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("canonical package-index bundle is absent")
	}
	githubReleaseID, err := strconv.ParseInt(entry.GitHubReleaseID, 10, 64)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	releaseDirectory := path.Join("releases", entry.ReleaseID, entry.Version)
	return admissionrelease.PublishedPackageEntry{
		Kind: candidate.Kind, Slug: candidate.Slug,
		FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
		ReleaseTag: entry.Tag, ReleaseCommit: entry.SourceCommit,
		ReleaseToolingCommit: entry.ToolingCommit,
		GitHubReleaseID:      githubReleaseID, PublishedAt: entry.PublishedAt,
		Immutable:                    entry.Immutable,
		PackageReleaseManifestPath:   path.Join(releaseDirectory, manifest.Name),
		PackageReleaseManifestDigest: manifest.SHA256,
		ChecksumsPath:                path.Join(releaseDirectory, checksums.Name),
		ChecksumsDigest:              checksums.SHA256,
		PackageIndexPath:             path.Join(releaseDirectory, indexName),
		PackageIndexSigstoreBundle:   path.Join(releaseDirectory, bundleName),
	}, nil
}
