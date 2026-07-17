package portableconformance

import (
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
)

func TestPortableHostContractMatchesReleaseOwnedObjectBucket(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := formregistry.ForKind("ObjectBucket")
	if err != nil {
		t.Fatal(err)
	}
	identity := contract.RunnerInput.Identity
	if identity.FormRef.APIVersion != release.APIVersion || identity.FormRef.Kind != release.Kind ||
		identity.FormRef.DefinitionVersion != release.DefinitionVersion || identity.FormRef.SchemaDigest != release.SchemaDigest ||
		identity.PackageDigest != release.PackageDigest {
		t.Fatalf("cross-repo runner FormRef %#v differs from provider release %#v", identity, release)
	}
}

func TestNeutralRunnerEvidenceDigestCoversSubjectAndInputs(t *testing.T) {
	path := filepath.Join("..", "..", "conformance", "portable-host-v1", "contract.json")
	var contract Contract
	if err := decodeStrict(path, &contract); err != nil {
		t.Fatal(err)
	}
	got, err := runnerEvidenceDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if got != contract.RunnerEvidence.SHA256 {
		t.Fatalf("runner evidence digest = %q, contract has %q", got, contract.RunnerEvidence.SHA256)
	}
	mutated := contract
	mutated.RunnerEvidence.Subject = "implementation-specific-runner"
	mutatedDigest, err := runnerEvidenceDigest(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedDigest == contract.RunnerEvidence.SHA256 {
		t.Fatal("runner subject substitution did not change the evidence digest")
	}
}
