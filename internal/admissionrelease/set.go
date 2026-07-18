// Package admissionrelease verifies retained, externally authenticated
// standard-admission release evidence. It does not publish evidence, contact a
// registry, or grant host, placement, credential, or commercial authority.
package admissionrelease

import "github.com/tako0614/terraform-provider-takoform/formpackage"

const setFormat = "takoform.standard-admission-set@v2"

// CandidateSet is the exact structural Form set compiled into the provider.
// Admission evidence must close over this set without adding, dropping, or
// replacing a candidate identity.
type CandidateSet struct {
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

// Set is the retained standard-admission set manifest. The manifest only
// binds artifacts together; its portable-standard claims are not trusted
// until every retained subject and its release provenance are authenticated.
type Set struct {
	Format                   string              `json:"format"`
	DefinitionVersion        string              `json:"definitionVersion"`
	PackageVersion           string              `json:"packageVersion"`
	AdmissionReleaseTag      string              `json:"admissionReleaseTag"`
	ProviderRegistryReadback RegistryReadbackRef `json:"providerRegistryReadback"`
	Entries                  []SetEntry          `json:"entries"`
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
