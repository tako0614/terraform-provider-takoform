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
// until an AuthenticatedRetainedSetVerifier authenticates every retained
// subject and its release provenance.
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
	Kind             string
	Path             string
	Canonical        []byte
	SigstorePath     string
	HostReport       string
	HostSigstore     string
	ProviderReport   string
	ProviderSigstore string
}

// AuthenticatedRetainedSetVerifier is the mandatory authentication seam for
// release evidence. Implementations must verify retained bytes offline,
// including publisher policy, trusted-root digest pins, certificate/SCT
// validity, and Rekor inclusion proof. Structural validation alone must never
// implement this interface as a successful verifier.
type AuthenticatedRetainedSetVerifier interface {
	VerifyRetainedSet(admissionRoot string, set Set, subjects []RetainedSubject) error
}
