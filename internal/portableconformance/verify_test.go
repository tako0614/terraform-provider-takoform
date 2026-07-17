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
