package standardforms

import (
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

func candidateAt(kind, version, schemaDigest, packageDigest string) admissionrelease.Candidate {
	return admissionrelease.Candidate{
		Kind: kind,
		Slug: "slug",
		FormRef: formpackage.FormRef{
			APIVersion:        formpackage.FormAPIVersion,
			Kind:              kind,
			DefinitionVersion: version,
			SchemaDigest:      schemaDigest,
		},
		PackageDigest: packageDigest,
	}
}

const (
	digestA = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digestB = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func TestSourceMayLeadPublicationButNeverContradictIt(t *testing.T) {
	published := admissionrelease.CandidateSet{
		Entries: []admissionrelease.Candidate{
			candidateAt("EdgeWorker", "3.0.0", digestA, digestA),
		},
	}

	// The settled case: source is exactly what is published.
	same := admissionrelease.CandidateSet{Entries: []admissionrelease.Candidate{
		candidateAt("EdgeWorker", "3.0.0", digestA, digestA),
	}}
	if err := assertSourceIsPublishedOrSuccessor(same, published); err != nil {
		t.Fatalf("published version must verify: %v", err)
	}

	// The authoring case: a version exists in source before it is published.
	// Rejecting this made the first release of any new Form unreachable,
	// because publication runs behind the gate that demanded its evidence.
	ahead := admissionrelease.CandidateSet{Entries: []admissionrelease.Candidate{
		candidateAt("EdgeWorker", "4.0.0", digestB, digestB),
	}}
	if err := assertSourceIsPublishedOrSuccessor(ahead, published); err != nil {
		t.Fatalf("unpublished successor must be allowed: %v", err)
	}

	// A published release is immutable: same version, different bytes is
	// tampering, not authoring.
	reshaped := admissionrelease.CandidateSet{Entries: []admissionrelease.Candidate{
		candidateAt("EdgeWorker", "3.0.0", digestB, digestB),
	}}
	if err := assertSourceIsPublishedOrSuccessor(reshaped, published); err == nil {
		t.Fatal("reshaping a published version must fail")
	}

	// Source must never fall behind publication either.
	behind := admissionrelease.CandidateSet{Entries: []admissionrelease.Candidate{
		candidateAt("EdgeWorker", "2.0.0", digestB, digestB),
	}}
	if err := assertSourceIsPublishedOrSuccessor(behind, published); err == nil {
		t.Fatal("downgrading below publication must fail")
	}
}
