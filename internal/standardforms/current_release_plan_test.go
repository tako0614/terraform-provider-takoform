package standardforms

import (
	"path/filepath"
	"testing"
)

func TestCurrentReleasePlanIsEmptyWhenAuthorityHasNoCurrentForms(t *testing.T) {
	t.Parallel()
	plan, err := CurrentReleasePlan(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Format != "takoform.current-form-release-plan@v2" || plan.Repository != "tako0614/terraform-provider-takoform" || len(plan.Releases) != 0 {
		t.Fatalf("empty current release plan = %#v", plan)
	}
}

func TestCurrentReleasePlanDerivesOneExperimentalPackageFromLifecycleAuthority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExperimentalLifecycleFixture(t, root, true)

	plan, err := CurrentReleasePlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != 1 {
		t.Fatalf("current release plan = %#v", plan)
	}
	release := plan.Releases[0]
	wantReleaseID := releaseIDForKind("Example")
	wantArtifactID := "sha256-" + release.PackageDigest[len("sha256:"):]
	if release.State != "experimental" || release.Kind != "Example" || release.ReleaseID != wantReleaseID ||
		release.ArtifactID != wantArtifactID || release.Tag != "forms/"+wantReleaseID+"/"+wantArtifactID ||
		release.SourcePath != "forms/releases/"+wantReleaseID+"/"+wantArtifactID || release.FormRef.DefinitionVersion != "0.1.0" {
		t.Fatalf("Experimental current release = %#v", release)
	}
}
