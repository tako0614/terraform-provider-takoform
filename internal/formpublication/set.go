// Package formpublication verifies the retained, immutable publication
// readback for every portable Form Package. It does not publish releases,
// contact GitHub, or grant admission, revocation, host, credential, or
// commercial authority.
package formpublication

import (
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	// SetFilename is the canonical publication manifest name within an
	// admission generation.
	SetFilename = "form-package-publication-set.json"
	// SetFormat is the exact versioned identity of Set.
	SetFormat = "takoform.form-package-publication-set@v1"

	releasePlanFormat             = "takoform.release-plan@v1"
	portableGeneration            = "portable-v1"
	repository                    = "tako0614/terraform-provider-takoform"
	publicationStatus             = "published-immutable"
	externalRequiredStatus        = "external-required"
	gitObjectFormat               = "sha1"
	releasePlanSourcePath         = "forms/release-plan.json"
	trustedRootSourcePath         = "admission/v4/trust/trusted-root.json"
	trustedRootPublicationPath    = "trust/trusted-root.json"
	publishedTrustPath            = "trust/published-package-trust.json"
	packageWorkflowIdentity       = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main"
	packageWorkflowOIDCIssuer     = "https://token.actions.githubusercontent.com"
	sigstoreBundleMediaTypeV03    = "application/vnd.dev.sigstore.bundle.v0.3+json"
	exactPortablePackageCount     = 34
	exactPackageReleaseAssetCount = 7
)

// Set is the create-only live publication readback for all portable Form
// Packages. The top-level claims are not trusted until Verify succeeds.
type Set struct {
	Format                     string             `json:"format"`
	Generation                 string             `json:"generation"`
	Repository                 string             `json:"repository"`
	PublicationStatus          string             `json:"publicationStatus"`
	AdmissionStatus            string             `json:"admissionStatus"`
	RevocationCheckpointStatus string             `json:"revocationCheckpointStatus"`
	GitObjectFormat            string             `json:"gitObjectFormat"`
	ProtectedMainCommit        string             `json:"protectedMainCommit"`
	SourcePlan                 SourcePlan         `json:"sourcePlan"`
	VerificationPolicy         VerificationPolicy `json:"verificationPolicy"`
	Entries                    []Entry            `json:"entries"`
}

// SourcePlan pins one retained file to its source-repository path.
type SourcePlan struct {
	Path       string `json:"path"`
	SourcePath string `json:"sourcePath"`
	SHA256     string `json:"sha256"`
}

// VerificationPolicy records the exact offline trust and workload identity
// used to authenticate each package-index bundle.
type VerificationPolicy struct {
	TrustedRoot         SourcePlan `json:"trustedRoot"`
	CertificateIdentity string     `json:"certificateIdentity"`
	OIDCIssuer          string     `json:"oidcIssuer"`
	BundleMediaType     string     `json:"bundleMediaType"`
}

// Entry binds one compiled candidate to its immutable Git tag, GitHub release
// identity, tooling authority, and exact seven-file asset closure.
type Entry struct {
	Kind            string              `json:"kind"`
	ReleaseID       string              `json:"releaseId"`
	Version         string              `json:"version"`
	Tag             string              `json:"tag"`
	SourcePath      string              `json:"sourcePath"`
	FormRef         formpackage.FormRef `json:"formRef"`
	PackageDigest   string              `json:"packageDigest"`
	TagObjectOID    string              `json:"tagObjectOid"`
	PeeledCommit    string              `json:"peeledCommit"`
	SourceCommit    string              `json:"sourceCommit"`
	ToolingCommit   string              `json:"toolingCommit"`
	ReleasePlan     SourcePlan          `json:"releasePlan"`
	TrustedRoot     SourcePlan          `json:"trustedRoot"`
	GitHubReleaseID string              `json:"githubReleaseId"`
	PublishedAt     string              `json:"publishedAt"`
	Immutable       bool                `json:"immutable"`
	Assets          []Asset             `json:"assets"`
}

// Asset pins one immutable GitHub Release asset by canonical name, size, and
// digest.
type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type releasePlan struct {
	Format     string           `json:"format"`
	Generation string           `json:"generation"`
	Repository string           `json:"repository"`
	Note       string           `json:"note"`
	Releases   []plannedRelease `json:"releases"`
}

type plannedRelease struct {
	Kind          string              `json:"kind"`
	Slug          string              `json:"slug"`
	ReleaseID     string              `json:"releaseId"`
	Version       string              `json:"version"`
	Tag           string              `json:"tag"`
	SourcePath    string              `json:"sourcePath"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}
