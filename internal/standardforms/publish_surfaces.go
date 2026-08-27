package standardforms

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// generatePublishedSurfaces writes the HCL example and the reference document
// of every declared Form.
//
// They are generated for the same reason the schema and the Form Definition
// are: a Form is declared once. A hand-written example is a second, slowly
// diverging description of the same contract.
func generatePublishedSurfaces(root string) error {
	if err := validatePublishedSurfaceCatalog(); err != nil {
		return err
	}
	for _, relativeParent := range []string{"docs", "examples", "forms"} {
		if err := prepareGeneratedDirectoryPath(root, relativeParent); err != nil {
			return err
		}
	}
	for _, relativeRoot := range []string{"docs/resources", "examples/resources"} {
		if err := resetGeneratedTree(filepath.Join(root, filepath.FromSlash(relativeRoot))); err != nil {
			return err
		}
	}
	for _, surface := range renderPublishedSurfaces() {
		path := filepath.Join(root, filepath.FromSlash(surface.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if info, err := os.Lstat(path); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("generated public surface %s is not a regular file", surface.path)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, surface.content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

type publishedSurface struct {
	path    string
	content []byte
}

// renderPublishedSurfaces is the single pure renderer for every
// catalog-derived human-facing surface. Both generation and verification use
// these exact bytes, so the read-only owner gate cannot silently bless
// hand-written drift.
func renderPublishedSurfaces() []publishedSurface {
	surfaces := make([]publishedSurface, 0, currentFormCount()*2+1)
	surfaces = append(surfaces, v3PublishedSurfaces()...)
	surfaces = append(surfaces, publishedSurface{
		path:    "forms/README.md",
		content: []byte(formInventoryDoc()),
	})
	sort.Slice(surfaces, func(left, right int) bool {
		return surfaces[left].path < surfaces[right].path
	})
	return surfaces
}

// VerifyPublishedSurfaces fails closed when a generated public file is
// missing, edited, replaced by a non-regular file, or joined by an undeclared
// file. It never writes the worktree.
func VerifyPublishedSurfaces(root string) error {
	if err := validatePublishedSurfaceCatalog(); err != nil {
		return err
	}
	for _, relativeRoot := range []string{"docs/resources", "examples/resources", "forms"} {
		if err := verifyGeneratedDirectoryPath(root, relativeRoot); err != nil {
			return err
		}
	}
	surfaces := renderPublishedSurfaces()
	for _, surface := range surfaces {
		path := filepath.Join(root, filepath.FromSlash(surface.path))
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("generated public surface %s: %w", surface.path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("generated public surface %s is not a regular file", surface.path)
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read generated public surface %s: %w", surface.path, err)
		}
		if !bytes.Equal(actual, surface.content) {
			return fmt.Errorf("generated public surface %s differs from the canonical Form catalog rendering", surface.path)
		}
	}
	if err := verifyPublishedSurfaceInventory(root, surfaces); err != nil {
		return err
	}
	return nil
}

// validatePublishedSurfaceCatalog refuses two Forms rendering one public
// surface: a colliding resource type or doc basename fails generation and
// verification alike.
func validatePublishedSurfaceCatalog() error {
	paths := make(map[string]string, currentFormCount()*2)
	for _, family := range currentFamilies() {
		for _, form := range family.Forms {
			resourceType, err := providerReferenceTerraformType(form)
			if err != nil {
				return err
			}
			owner := family.Group + "/" + form.Kind
			for _, path := range []string{
				filepath.ToSlash(filepath.Join("docs", "resources", v3DocBasename(form))),
				filepath.ToSlash(filepath.Join("examples", "resources", resourceType, "resource.tf")),
			} {
				if previous, duplicate := paths[path]; duplicate {
					return fmt.Errorf("Forms %s and %s render the same public surface %s", previous, owner, path)
				}
				paths[path] = owner
			}
		}
	}
	return nil
}

func verifyGeneratedDirectoryPath(root, relative string) error {
	current := root
	parts := []string{"."}
	if info, err := os.Lstat(current); err != nil {
		return fmt.Errorf("generated public surface root: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("generated public surface root is not a real directory")
	}
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		parts = append(parts, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("generated public surface directory %s: %w", filepath.ToSlash(filepath.Join(parts...)), err)
		}
		if !info.IsDir() {
			return fmt.Errorf("generated public surface directory %s is not a real directory", filepath.ToSlash(filepath.Join(parts...)))
		}
	}
	return nil
}

func prepareGeneratedDirectoryPath(root, relative string) error {
	current := root
	if info, err := os.Lstat(current); err != nil {
		return fmt.Errorf("generated public surface root: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("generated public surface root is not a real directory")
	}
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("generated public surface parent %s is not a real directory", filepath.ToSlash(relative))
		}
	}
	return nil
}

func verifyPublishedSurfaceInventory(root string, surfaces []publishedSurface) error {
	expectedDocs := make(map[string]struct{}, currentFormCount())
	expectedExamples := make(map[string]struct{}, currentFormCount()*2)
	for _, surface := range surfaces {
		switch {
		case strings.HasPrefix(surface.path, "docs/resources/"):
			expectedDocs[strings.TrimPrefix(surface.path, "docs/resources/")] = struct{}{}
		case strings.HasPrefix(surface.path, "examples/resources/"):
			relative := strings.TrimPrefix(surface.path, "examples/resources/")
			expectedExamples[relative] = struct{}{}
			expectedExamples[filepath.ToSlash(filepath.Dir(relative))] = struct{}{}
		}
	}
	if err := verifyExactGeneratedTree(root, "docs/resources", expectedDocs); err != nil {
		return err
	}
	return verifyExactGeneratedTree(root, "examples/resources", expectedExamples)
}

func verifyExactGeneratedTree(root, relativeRoot string, expected map[string]struct{}) error {
	treeRoot := filepath.Join(root, filepath.FromSlash(relativeRoot))
	return filepath.WalkDir(treeRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk generated public surfaces under %s: %w", relativeRoot, walkErr)
		}
		if path == treeRoot {
			return nil
		}
		relative, err := filepath.Rel(treeRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, ok := expected[relative]; !ok {
			return fmt.Errorf("undeclared generated public surface %s", filepath.ToSlash(filepath.Join(relativeRoot, relative)))
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect generated public surface %s: %w", filepath.ToSlash(filepath.Join(relativeRoot, relative)), err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("generated public surface %s is not a regular file", filepath.ToSlash(filepath.Join(relativeRoot, relative)))
		}
		return nil
	})
}

// quoteHCL renders one desired value as HCL for a generated example.
func quoteHCL(value any) string {
	switch typed := value.(type) {
	case string:
		return fmt.Sprintf("%q", typed)
	case bool:
		return fmt.Sprintf("%t", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		return fmt.Sprintf("%d", int64(typed))
	case []any:
		members := make([]string, 0, len(typed))
		for _, member := range typed {
			members = append(members, quoteHCL(member))
		}
		sort.Strings(members)
		return "[" + strings.Join(members, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]string, 0, len(keys))
		for _, key := range keys {
			entries = append(entries, fmt.Sprintf("%q = %s", key, quoteHCL(typed[key])))
		}
		return "{ " + strings.Join(entries, ", ") + " }"
	default:
		return fmt.Sprintf("%q", fmt.Sprint(value))
	}
}

func resetGeneratedTree(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return os.MkdirAll(root, 0o755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if err := os.RemoveAll(root); err != nil {
			return err
		}
		return os.MkdirAll(root, 0o755)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func sortedStringKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// formInventoryDoc renders the human-readable Form inventory so the published
// list of Forms can never disagree with the declaration the packages are built
// from.
func formInventoryDoc() string {
	return `# Form inventory

The Provider's retained Form snapshot is provider-neutral. Terraform resource
type names, provider schema choices, and provider releases are reference-implementation
metadata; none participates in Form validation, canonical bytes, or digest.

The design has exactly two domain version axes. They are never one maturity
label. API/Core release SemVer uses human-readable compatibility checkpoints;
the current public checkpoint is ` + "`v1.0.1`" + ` (see the [Core release](https://github.com/tako0614/takoform/releases/tag/v1.0.1)), and compatible ` + "`1.x`" + ` releases
remain on the ` + "`forms.takoform.com/v1`" + ` wire/discovery lane. Each Form carries
its own ` + "`definitionVersion`" + `.

| Domain axis | Current design target | Meaning |
| --- | --- | --- |
| API/Core release SemVer | ` + "`v1.0.1`" + ` (public Core checkpoint) | Public Core/API release checkpoint on ` + "`forms.takoform.com/v1`" + `; future compatible ` + "`v1.y.0`" + ` checkpoints remain on /v1. |
| Form ` + "`definitionVersion`" + ` | ` + "`0.1.0`" + ` per exact FormRef | Independent immutable version; Provider's retained Forms are Experimental. |

The versionless Form Family group, ` + "`schemaDigest`" + `, package envelope, Provider
SemVer, and Specification numbers are artifact/history identities rather than
additional domain axes. The historical Specification 1.1 is a sealed source
receipt: it is not API release 1.1 or 1.1.0, does not create /v1.1, and is not
an ongoing Specification stream. Form Family membership remains the exact
versionless ` + "`apiVersion`" + ` group, and the current package envelope is
` + "`packages.forms.takoform.com/v1alpha5`" + `.

The active standalone publisher is ` + "`takoform-forms`" + ` at
https://github.com/tako0614/takoform-forms. Its Edge family
` + "`edge.forms.takoform.com`" + ` currently has 16 candidate Forms and no
published package artifacts. Provider 3 retains typed compatibility mappings
for the broader Provider snapshot; that 31-Form projection is not the active
publisher roster.

The generated candidate index is ` + "`forms/candidates/current-family-index.json`" + `.
It binds the Provider compatibility snapshot's eight family candidate sets plus
the global Interface and Binding candidate sets by exact SHA-256. Provider
release and publication evidence are separate authorities; a provider mapping
cannot widen Form semantics or change a Form digest.
` + v3FormInventorySection()
}

// GenerateCurrentPublishedSurfaces writes the generated current and
// compatibility docs and examples. Legacy package/release verification remains
// read-only elsewhere.
func GenerateCurrentPublishedSurfaces(root string) error {
	return generatePublishedSurfaces(root)
}
