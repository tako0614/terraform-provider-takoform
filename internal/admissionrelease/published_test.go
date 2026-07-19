package admissionrelease

import (
	"strings"
	"testing"
)

func TestValidatePublishedPackageSetKeepsAdmissionAndRevocationExternal(t *testing.T) {
	t.Parallel()
	candidates := testCandidates()
	set := testPublishedSet(candidates)
	if _, err := validatePublishedPackageSet(set, candidates); err != nil {
		t.Fatalf("valid published set rejected: %v", err)
	}

	set.AdmissionStatus = "portable-standard"
	if _, err := validatePublishedPackageSet(set, candidates); err == nil || !strings.Contains(err.Error(), "external admission") {
		t.Fatalf("admitted publication snapshot error = %v", err)
	}
	set.AdmissionStatus = "external-required"
	set.RevocationCheckpointStatus = "passed"
	if _, err := validatePublishedPackageSet(set, candidates); err == nil || !strings.Contains(err.Error(), "revocation proof") {
		t.Fatalf("synthetic revocation snapshot error = %v", err)
	}
}

func TestValidatePublishedPackageSetRequiresImmutableUniqueRelease(t *testing.T) {
	t.Parallel()
	candidates := testCandidates()
	set := testPublishedSet(candidates)
	set.Entries[0].Immutable = false
	if _, err := validatePublishedPackageSet(set, candidates); err == nil || !strings.Contains(err.Error(), "immutable GitHub Release") {
		t.Fatalf("mutable release error = %v", err)
	}
}

func testPublishedSet(candidates CandidateSet) PublishedPackageSet {
	candidate := candidates.Entries[0]
	releaseID := releaseIDForKind(candidate.Kind)
	releaseDirectory := "releases/" + releaseID + "/" + candidates.PackageVersion
	base := "takoform-form-" + releaseID + "_" + candidates.PackageVersion + "_package-index"
	return PublishedPackageSet{
		Format: publishedPackageSetFormat, Repository: publishedRepository,
		DefinitionVersion: candidates.DefinitionVersion, PackageVersion: candidates.PackageVersion,
		PublicationStatus: "published-immutable", AdmissionStatus: "external-required", RevocationCheckpointStatus: "external-required",
		Trust: PublishedPackageTrustRef{Path: publishedPackageTrustPath, Digest: testEvidenceDigest},
		Entries: []PublishedPackageEntry{{
			Kind: candidate.Kind, Slug: candidate.Slug, FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest,
			ReleaseTag:    "forms/" + releaseID + "/v" + candidates.PackageVersion,
			ReleaseCommit: "0123456789abcdef0123456789abcdef01234567", ReleaseToolingCommit: "89abcdef0123456789abcdef0123456789abcdef",
			GitHubReleaseID: 1, PublishedAt: "2026-07-19T01:20:33Z", Immutable: true,
			PackageReleaseManifestPath: releaseDirectory + "/release-manifest.json", PackageReleaseManifestDigest: testEvidenceDigest,
			ChecksumsPath: releaseDirectory + "/SHA256SUMS", ChecksumsDigest: testEvidenceDigest,
			PackageIndexPath: releaseDirectory + "/" + base + ".json", PackageIndexSigstoreBundle: releaseDirectory + "/" + base + ".sigstore.json",
		}},
	}
}
