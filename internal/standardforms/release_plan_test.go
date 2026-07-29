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

func TestRenderReleasePlanUsesOwnerLocalPrepareThenPublish(t *testing.T) {
	t.Parallel()
	plan := ReleasePlan{
		Generation: "portable-v1",
		Releases: []PlannedFormRelease{{
			Kind:          "ObjectBucket",
			ReleaseID:     "k-j5rguzldorbhky3lmv2a",
			Version:       "1.0.0",
			Tag:           "forms/k-j5rguzldorbhky3lmv2a/v1.0.0",
			SourcePath:    "forms/releases/k-j5rguzldorbhky3lmv2a/1.0.0",
			PackageDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	}
	const expected = `portable-v1: 1 Form releases, each independent

ObjectBucket                 forms/k-j5rguzldorbhky3lmv2a/v1.0.0
  source forms/releases/k-j5rguzldorbhky3lmv2a/1.0.0
  digest sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  prepare bun run deploy -- takoform-form-package-release prepare --tag forms/k-j5rguzldorbhky3lmv2a/v1.0.0 --expected-commit <current-protected-main-commit>

Run each prepare while its planned tag and Release are absent. After its
reviewed candidate run completes, publish through the same owner entrypoint:
  bun run deploy -- takoform-form-package-release publish --tag <same-planned-tag> --expected-commit <same-40-character-reviewed-commit> --run-id <candidate-run-id> --run-attempt <candidate-run-attempt>
Publish verifies the candidate, then create-only materializes the tag and
immutable Release. Publication proves bytes only; admission still requires
a conforming host's signed lifecycle report.
`
	rendered := renderReleasePlan(plan)
	if rendered != expected {
		t.Fatalf("release-plan owner flow drifted\n--- got ---\n%s--- want ---\n%s", rendered, expected)
	}
	for _, forbidden := range []string{
		"workflow_dispatch",
		"gh workflow",
		"Dispatch the Release Form Package",
		"after that exact tag exists",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("release-plan output retained tag-first/direct-dispatch wording %q", forbidden)
		}
	}
}
