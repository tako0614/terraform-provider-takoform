package standardforms

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestReleasePlanCoversEveryDeclaredFormExactlyOnce proves an operator can
// release the whole set without deriving anything by hand.
func TestReleasePlanCoversEveryDeclaredFormExactlyOnce(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	if err := VerifyReleasePlan(root); err != nil {
		t.Fatal(err)
	}
	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != len(Specs) {
		t.Fatalf("plan holds %d releases, want %d declared Forms", len(plan.Releases), len(Specs))
	}
	seen := map[string]struct{}{}
	for _, release := range plan.Releases {
		if _, duplicate := seen[release.Kind]; duplicate {
			t.Fatalf("plan duplicates %s", release.Kind)
		}
		seen[release.Kind] = struct{}{}
		if release.Tag != "forms/"+release.ReleaseID+"/v"+release.Version {
			t.Fatalf("%s tag %q is not derived from its release identity", release.Kind, release.Tag)
		}
		if !strings.HasPrefix(release.SourcePath, "forms/releases/"+release.ReleaseID+"/") {
			t.Fatalf("%s source %q is not its own release source", release.Kind, release.SourcePath)
		}
	}
	for _, spec := range Specs {
		if _, ok := seen[spec.Kind]; !ok {
			t.Fatalf("plan omits %s", spec.Kind)
		}
	}
}

// TestPlannedReleasesNeverCollideWithPublishedTags keeps published bytes
// immutable. A planned tag that already belongs to the retired generation
// would either fail at publication or overwrite a proof that exists.
func TestPlannedReleasesNeverCollideWithPublishedTags(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		t.Fatal(err)
	}
	retired := map[string]struct{}{}
	for _, tag := range RetiredReleaseTags() {
		retired[tag] = struct{}{}
	}
	if len(retired) != len(RetiredKinds) {
		t.Fatalf("retired tag set has %d entries, want %d", len(retired), len(RetiredKinds))
	}
	for _, release := range plan.Releases {
		if _, collision := retired[release.Tag]; collision {
			t.Errorf("%s plans to reuse published tag %s", release.Kind, release.Tag)
		}
	}
}
