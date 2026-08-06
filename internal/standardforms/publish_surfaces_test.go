package standardforms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestPublishedSurfaceManifestCoversCatalogExactly(t *testing.T) {
	t.Parallel()

	expected := map[string]struct{}{"forms/README.md": {}}
	for _, kind := range currentformcatalog.Kinds {
		expected[filepath.ToSlash(filepath.Join("docs", "resources", docBasename(kind)))] = struct{}{}
		expected[filepath.ToSlash(filepath.Join("examples", "resources", kind.ResourceType, "resource.tf"))] = struct{}{}
	}
	for _, form := range edgeformcatalog.Forms {
		expected[filepath.ToSlash(filepath.Join("docs", "resources", v3DocBasename(form)))] = struct{}{}
		expected[filepath.ToSlash(filepath.Join("examples", "resources", form.ResourceType, "resource.tf"))] = struct{}{}
	}
	// The generic takoform_resource carrier has no Form Definition, so its two
	// surfaces are authored in the renderer instead of derived from a catalog.
	expected["docs/resources/resource.md"] = struct{}{}
	expected["examples/resources/takoform_resource/resource.tf"] = struct{}{}
	surfaces := renderPublishedSurfaces()
	wantCount := 2*(len(currentformcatalog.Kinds)+len(edgeformcatalog.Forms)+1) + 1
	if len(surfaces) != wantCount {
		t.Fatalf("published surface count = %d, want %d", len(surfaces), wantCount)
	}
	for _, surface := range surfaces {
		if _, ok := expected[surface.path]; !ok {
			t.Fatalf("renderer emitted undeclared surface %s", surface.path)
		}
		delete(expected, surface.path)
	}
	if len(expected) != 0 {
		t.Fatalf("renderer omitted declared surfaces: %#v", expected)
	}
}

func TestCommittedPublishedSurfacesMatchCatalog(t *testing.T) {
	t.Parallel()

	if err := VerifyPublishedSurfaces(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedSurfaceVerificationRejectsContentDrift(t *testing.T) {
	t.Parallel()

	for _, relative := range []string{
		"docs/resources/object_bucket.md",
		"examples/resources/takoform_object_bucket/resource.tf",
		"forms/README.md",
	} {
		relative := relative
		t.Run(strings.ReplaceAll(relative, "/", "_"), func(t *testing.T) {
			root := t.TempDir()
			if err := generatePublishedSurfaces(root); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(relative))
			if err := os.WriteFile(path, []byte("hand-written replacement\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			err := VerifyPublishedSurfaces(root)
			if err == nil || !strings.Contains(err.Error(), relative) {
				t.Fatalf("content drift error = %v", err)
			}
		})
	}
}

func TestPublishedSurfaceVerificationRejectsMissingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := generatePublishedSurfaces(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "examples", "resources", "takoform_edge_worker", "resource.tf")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	err := VerifyPublishedSurfaces(root)
	if err == nil || !strings.Contains(err.Error(), "examples/resources/takoform_edge_worker/resource.tf") {
		t.Fatalf("missing surface error = %v", err)
	}
}

func TestPublishedSurfaceVerificationRejectsUndeclaredFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := generatePublishedSurfaces(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "examples", "resources", "takoform_http_service", "resource.tf")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("resource \"takoform_http_service\" \"stale\" {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := VerifyPublishedSurfaces(root)
	if err == nil || !strings.Contains(err.Error(), "examples/resources/takoform_http_service") {
		t.Fatalf("undeclared surface error = %v", err)
	}
}

func TestPublishedSurfaceVerificationRejectsSymlinkedGeneratedDirectories(t *testing.T) {
	t.Parallel()

	for _, relative := range []string{"docs", "examples/resources"} {
		relative := relative
		t.Run(strings.ReplaceAll(relative, "/", "_"), func(t *testing.T) {
			root := t.TempDir()
			if err := generatePublishedSurfaces(root); err != nil {
				t.Fatal(err)
			}
			generatedRoot := filepath.Join(root, filepath.FromSlash(relative))
			target := filepath.Join(root, "symlink-target-"+strings.ReplaceAll(relative, "/", "-"))
			if err := os.Rename(generatedRoot, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, generatedRoot); err != nil {
				t.Fatal(err)
			}

			err := VerifyPublishedSurfaces(root)
			if err == nil || !strings.Contains(err.Error(), relative) || !strings.Contains(err.Error(), "not a real directory") {
				t.Fatalf("symlinked generated directory error = %v", err)
			}
		})
	}
}

func TestGenerationPrunesNestedStalePublishedSurfaces(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := generatePublishedSurfaces(root); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "examples", "resources", "takoform_edge_worker", "README.md")
	if err := os.WriteFile(stale, []byte("stale generated note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generatePublishedSurfaces(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale nested surface survived regeneration: %v", err)
	}
	if err := VerifyPublishedSurfaces(root); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationRejectsSymlinkedPublishedSurfaceLeaf(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := generatePublishedSurfaces(root); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.md")
	original := []byte("must remain untouched\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(root, "forms", "README.md")
	if err := os.Remove(readme); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, readme); err != nil {
		t.Fatal(err)
	}

	err := generatePublishedSurfaces(root)
	if err == nil || !strings.Contains(err.Error(), "forms/README.md") || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlinked generated leaf error = %v", err)
	}
	after, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("symlink target changed to %q", after)
	}
}

func TestPublishedSurfaceCatalogRejectsUnknownInventoryDomain(t *testing.T) {
	t.Parallel()

	kind := currentformcatalog.Kinds[0]
	kind.Domain = "storag"
	err := validatePublishedSurfaceCatalog([]formcatalog.Kind{kind})
	if err == nil || !strings.Contains(err.Error(), `unknown public inventory domain "storag"`) {
		t.Fatalf("unknown domain error = %v", err)
	}
}
