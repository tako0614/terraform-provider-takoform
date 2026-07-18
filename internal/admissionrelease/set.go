// Package admissionrelease verifies retained, externally authenticated
// standard-admission release evidence. It does not publish evidence, contact a
// registry, or grant host, placement, credential, or commercial authority.
package admissionrelease

import "github.com/tako0614/terraform-provider-takoform/formpackage"

const setFormat = "takoform.standard-admission-set@v1"

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
	Format              string     `json:"format"`
	DefinitionVersion   string     `json:"definitionVersion"`
	PackageVersion      string     `json:"packageVersion"`
	AdmissionReleaseTag string     `json:"admissionReleaseTag"`
	Entries             []SetEntry `json:"entries"`
}

// SetEntry binds one exact candidate package to its retained release,
// admission evidence, and conformance reports.
type SetEntry struct {
	Kind                       string              `json:"kind"`
	Slug                       string              `json:"slug"`
	FormRef                    formpackage.FormRef `json:"formRef"`
	PackageDigest              string              `json:"packageDigest"`
	ReleaseTag                 string              `json:"releaseTag"`
	ReleaseCommit              string              `json:"releaseCommit"`
	PackageIndexSigstoreBundle string              `json:"packageIndexSigstoreBundle"`
	EvidencePath               string              `json:"evidencePath"`
	EvidenceDigest             string              `json:"evidenceDigest"`
	HostReportPath             string              `json:"hostReportPath"`
	ProviderReportPath         string              `json:"providerReportPath"`
	AdmissionStatus            string              `json:"admissionStatus"`
}

// RetainedSubject is a canonical evidence document whose exact bytes and
// detached authentication material must be verified offline before release.
type RetainedSubject struct {
	Kind         string
	Path         string
	Canonical    []byte
	SigstorePath string
}

// RetainedEvidenceVerifier authenticates the admission evidence subjects in
// one set. This intentionally narrower seam does not claim that host/provider
// reports or package release readback have been authenticated. The outer
// release gate remains blocked until those independent checks are implemented.
type RetainedEvidenceVerifier interface {
	VerifyRetainedEvidence(admissionRoot string, set Set, subjects []RetainedSubject) error
}
