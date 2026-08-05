// Package admissionrelease verifies retained, externally authenticated Legacy
// admission artifacts as historical evidence. It does not reinterpret their
// semantic claims or grant current lifecycle, host, placement, credential, or
// commercial authority.
package admissionrelease

import (
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	setFormatV2 = "takoform.standard-admission-set@v2"
	setFormatV3 = "takoform.standard-admission-set@v3"
)

// CandidateSet is the legacy-named exact Form set consumed by historical
// verification and publication helpers. It is not the current lifecycle set.
type CandidateSet struct {
	Generation        string
	DefinitionVersion string
	PackageVersion    string
	Entries           []Candidate
}

// Candidate identifies one local, data-only Form Package candidate.
type Candidate struct {
	Kind          string
	Slug          string
	PackagePath   string
	FormRef       formpackage.FormRef
	PackageDigest string
}

// Set is a retained Standard-admission manifest. Current code treats its
// portable-standard fields as immutable historical assertions, never as
// present Form maturity or approval.
type Set struct {
	Format                   string                 `json:"format"`
	Generation               string                 `json:"generation,omitempty"`
	DefinitionVersion        string                 `json:"definitionVersion,omitempty"`
	PackageVersion           string                 `json:"packageVersion,omitempty"`
	AdmissionReleaseTag      string                 `json:"admissionReleaseTag"`
	ProviderReportClosure    *ProviderReportClosure `json:"providerReportClosure,omitempty"`
	ProviderRegistryReadback RegistryReadbackRef    `json:"providerRegistryReadback"`
	Entries                  []SetEntry             `json:"entries"`
}

// ProviderReportClosure retains the full current provider-report generation.
// The admission subset may select fewer Forms, but it cannot discard the
// signed proof that the provider executed every current portable Form.
type ProviderReportClosure struct {
	Generation           string                       `json:"generation"`
	ManifestPath         string                       `json:"manifestPath"`
	ManifestDigest       string                       `json:"manifestDigest"`
	SignedManifestPath   string                       `json:"signedManifestPath"`
	SignedManifestDigest string                       `json:"signedManifestDigest"`
	ChecksumsPath        string                       `json:"checksumsPath"`
	ChecksumsDigest      string                       `json:"checksumsDigest"`
	Reports              []ProviderReportClosureEntry `json:"reports"`
}

// ProviderReportClosureEntry binds one full-catalog report and signature.
type ProviderReportClosureEntry struct {
	Kind           string                              `json:"kind"`
	Slug           string                              `json:"slug"`
	Identity       standardform.InstalledFormReference `json:"identity"`
	ReportPath     string                              `json:"reportPath"`
	ReportDigest   string                              `json:"reportDigest"`
	SigstoreBundle string                              `json:"sigstoreBundle"`
}

// RegistryReadbackRef binds the one provider-version install/readback report
// that closes over the complete candidate set. The report is retained and
// authenticated independently from every per-Form runner report.
type RegistryReadbackRef struct {
	Path           string `json:"path"`
	Digest         string `json:"digest"`
	SigstoreBundle string `json:"sigstoreBundle"`
}

// SetEntry binds one exact candidate package to its retained release,
// admission evidence, and conformance reports.
type SetEntry struct {
	Kind                         string              `json:"kind"`
	Slug                         string              `json:"slug"`
	FormRef                      formpackage.FormRef `json:"formRef"`
	PackageDigest                string              `json:"packageDigest"`
	ReleaseTag                   string              `json:"releaseTag"`
	ReleaseCommit                string              `json:"releaseCommit"`
	ReleaseToolingCommit         string              `json:"releaseToolingCommit"`
	PackageReleaseManifestPath   string              `json:"packageReleaseManifestPath"`
	PackageReleaseManifestDigest string              `json:"packageReleaseManifestDigest"`
	PackageIndexPath             string              `json:"packageIndexPath"`
	PackageIndexSigstoreBundle   string              `json:"packageIndexSigstoreBundle"`
	EvidencePath                 string              `json:"evidencePath"`
	EvidenceDigest               string              `json:"evidenceDigest"`
	HostReportPath               string              `json:"hostReportPath"`
	HostReportDigest             string              `json:"hostReportDigest"`
	HostReportSigstoreBundle     string              `json:"hostReportSigstoreBundle"`
	ProviderReportPath           string              `json:"providerReportPath"`
	ProviderReportDigest         string              `json:"providerReportDigest"`
	ProviderReportSigstoreBundle string              `json:"providerReportSigstoreBundle"`
	AdmissionStatus              string              `json:"admissionStatus"`
}

// RetainedSubject is a canonical evidence document whose exact bytes and
// detached authentication material must be verified offline before release.
type RetainedSubject struct {
	Kind         string
	Role         string
	Path         string
	Canonical    []byte
	SigstorePath string
}

// RetainedSubjectVerifier authenticates every retained subject in one set
// against its role-specific, source-pinned publisher policy. Structural and
// digest verification is performed before this seam is called.
type RetainedSubjectVerifier interface {
	VerifyRetainedSubjects(admissionRoot string, set Set, subjects []RetainedSubject) error
}

// ReleaseRefVerifier proves that every retained release claim resolves to the
// exact immutable Git tag and commit in the checked-out source history.
type ReleaseRefVerifier interface {
	VerifyReleaseRefs(root string, set Set, readback ProviderRegistryReadback) error
}
