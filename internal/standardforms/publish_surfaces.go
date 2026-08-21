package standardforms

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// generatePublishedSurfaces writes the HCL example and the reference document
// of every declared Form.
//
// They are generated for the same reason the schema and the Form Definition
// are: a Form is declared once. A hand-written example is a second, slowly
// diverging description of the same contract.
func generatePublishedSurfaces(root string) error {
	if err := validatePublishedSurfaceCatalog(currentformcatalog.Kinds); err != nil {
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

type formInventoryDomain struct {
	name  string
	title string
}

var formInventoryDomains = []formInventoryDomain{
	{name: "compute", title: "Compute and application"},
	{name: "data", title: "Data and storage"},
	{name: "analytics", title: "Analytics and inference"},
	{name: "network", title: "Network and delivery"},
	{name: "operations", title: "Operations and integration"},
}

// renderPublishedSurfaces is the single pure renderer for every
// catalog-derived human-facing surface. Both generation and verification use
// these exact bytes, so the read-only owner gate cannot silently bless
// hand-written drift.
func renderPublishedSurfaces() []publishedSurface {
	surfaces := make([]publishedSurface, 0, (len(currentformcatalog.Kinds)+len(edgeformcatalog.Forms))*2+1)
	for _, kind := range currentformcatalog.Kinds {
		surfaces = append(surfaces,
			publishedSurface{
				path:    filepath.ToSlash(filepath.Join("docs", "resources", docBasename(kind))),
				content: []byte(resourceDoc(kind)),
			},
			publishedSurface{
				path:    filepath.ToSlash(filepath.Join("examples", "resources", kind.ResourceType, "resource.tf")),
				content: []byte(exampleHCL(kind)),
			},
		)
	}
	// The Host API v1beta1 lane surfaces render from the family catalog; the
	// retained v2 renderer above stays byte-identical.
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
	if err := validatePublishedSurfaceCatalog(currentformcatalog.Kinds); err != nil {
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

func validatePublishedSurfaceCatalog(kinds []formcatalog.Kind) error {
	domains := make(map[string]struct{}, len(formInventoryDomains))
	for _, domain := range formInventoryDomains {
		domains[domain.name] = struct{}{}
	}
	paths := make(map[string]string, (len(kinds)+len(edgeformcatalog.Forms))*2)
	for _, kind := range kinds {
		if _, ok := domains[kind.Domain]; !ok {
			return fmt.Errorf("Form %s has unknown public inventory domain %q", kind.Kind, kind.Domain)
		}
		for _, path := range []string{
			filepath.ToSlash(filepath.Join("docs", "resources", docBasename(kind))),
			filepath.ToSlash(filepath.Join("examples", "resources", kind.ResourceType, "resource.tf")),
		} {
			if owner, duplicate := paths[path]; duplicate {
				return fmt.Errorf("Forms %s and %s render the same public surface %s", owner, kind.Kind, path)
			}
			paths[path] = kind.Kind
		}
	}
	// The family-lane surfaces share the same trees; a colliding resource
	// type or doc basename across lanes fails generation and verification.
	for _, form := range edgeformcatalog.Forms {
		for _, path := range []string{
			filepath.ToSlash(filepath.Join("docs", "resources", v3DocBasename(form))),
			filepath.ToSlash(filepath.Join("examples", "resources", form.ResourceType, "resource.tf")),
		} {
			if owner, duplicate := paths[path]; duplicate {
				return fmt.Errorf("Forms %s and %s render the same public surface %s", owner, form.Kind, path)
			}
			paths[path] = form.Kind
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
	expectedDocs := make(map[string]struct{}, len(currentformcatalog.Kinds))
	expectedExamples := make(map[string]struct{}, len(currentformcatalog.Kinds)*2)
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

func exampleHCL(kind formcatalog.Kind) string {
	var builder strings.Builder
	builder.WriteString(`terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

`)
	fmt.Fprintf(&builder, "resource %q \"example\" {\n", kind.ResourceType)
	desired := kind.CanonicalDesired()
	lines := [][2]string{{"name", quoteHCL(desired["name"])}}
	if kind.Artifact {
		source, _ := desired["source"].(map[string]any)
		for _, key := range sortedStringKeys(source) {
			lines = append(lines, [2]string{artifactAttributeName(key), quoteHCL(source[key])})
		}
	}
	for _, field := range kind.Fields {
		value, ok := desired[field.Wire]
		if !ok {
			continue
		}
		lines = append(lines, [2]string{field.HCL, quoteHCL(value)})
	}
	width := 0
	for _, line := range lines {
		if len(line[0]) > width {
			width = len(line[0])
		}
	}
	for _, line := range lines {
		fmt.Fprintf(&builder, "  %-*s = %s\n", width, line[0], line[1])
	}
	if connections, ok := desired["connections"].(map[string]any); ok {
		builder.WriteString("\n  connections = [\n")
		for _, name := range sortedStringKeys(connections) {
			entry, _ := connections[name].(map[string]any)
			fmt.Fprintf(&builder, "    {\n      name        = %q\n      resource    = %q\n      permissions = %s\n      projection  = %q\n    },\n",
				name, entry["resource"], quoteHCL(entry["permissions"]), entry["projection"])
		}
		builder.WriteString("  ]\n")
	}
	builder.WriteString("}\n\n")
	fmt.Fprintf(&builder, "output %q {\n  value = %s.example.outputs\n}\n",
		strings.TrimPrefix(kind.ResourceType, "takoform_")+"_outputs", kind.ResourceType)
	return builder.String()
}

func artifactAttributeName(wireKey string) string {
	switch wireKey {
	case "artifactUrl":
		return "artifact_url"
	case "artifactSha256":
		return "artifact_sha256"
	case "artifactMediaType":
		return "artifact_media_type"
	default:
		return wireKey
	}
}

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

func docBasename(kind formcatalog.Kind) string {
	return strings.TrimPrefix(kind.ResourceType, "takoform_") + ".md"
}

func resourceDoc(kind formcatalog.Kind) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, `---
page_title: "%s Resource - takoform"
subcategory: "Service Forms"
description: |-
  %s
---

# %s

%s

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](https://takoform.com/examples/resources/%s/resource.tf).

## Arguments

`, kind.ResourceType, kind.Description, kind.ResourceType, kind.Description, kind.ResourceType)

	fmt.Fprintf(&builder, "- `name` (String, required, forces replacement) — Resource name.\n")
	if kind.Artifact {
		builder.WriteString("- `artifact_url` (String, required) — Absolute credential-free HTTPS location any conforming host can fetch; userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.\n")
		builder.WriteString("- `artifact_sha256` (String, required) — Digest binding the URL to exact immutable bytes.\n")
		builder.WriteString("- `artifact_media_type` (String, required) — Lowercase type/subtype describing how the bytes are interpreted.\n")
	}
	for _, field := range kind.Fields {
		fmt.Fprintf(&builder, "- `%s` (%s, %s) — %s%s\n",
			field.HCL, docType(field), docRequirement(field), field.Doc, docConstraint(field))
	}
	switch kind.Connections {
	case formcatalog.ConnectionsRequired:
		if kind.MaxConnections == 1 {
			builder.WriteString("- `connections` (List of Object, exactly one) — One declared Resource reference with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.\n")
		} else {
			builder.WriteString("- `connections` (List of Object, one or more) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. Names must be unique. A connection is a request the host validates; it grants nothing by itself.\n")
		}
	case formcatalog.ConnectionsOptional:
		builder.WriteString("- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.\n")
	}
	builder.WriteString("- `space` (String, optional, forces replacement) — Overrides the provider default.\n")

	builder.WriteString(`
## Read-only attributes

` + "`form_api_version`" + `, ` + "`form_kind`" + `, ` + "`form_definition_version`" + `, ` + "`form_schema_digest`" + `, and
` + "`form_package_digest`" + ` bind state to the exact immutable Form identity.
` + "`id`" + ` is the provider-synthesized ` + "`Kind/name`" + ` identity and ` + "`resource_version`" + ` is
the host generation fence. ` + "`drift_status`" + `, ` + "`portability`" + `, and ` + "`outputs`" + ` are written
only after the host's observed and output documents satisfy this exact Form's
closed schemas, identities, and generation. Undeclared host keys are rejected;
backend placement is never provider state.
`)
	if len(kind.Interfaces) > 0 {
		builder.WriteString("\n## Declared runtime interfaces\n\n")
		for _, declared := range kind.Interfaces {
			fmt.Fprintf(&builder, "- `%s@1` — %s Operations: %s.\n",
				declared.Name, declared.Description, "`"+strings.Join(declared.Operations, "`, `")+"`")
		}
		builder.WriteString("\nA declaration says what exists. It carries no credential and grants no\nconsumer access; the host creates the record and authorizes its use.\n")
	}
	builder.WriteString("\n## Import\n\n```console\nterraform import " + kind.ResourceType + ".example NAME\nterraform import " + kind.ResourceType + ".example SPACE/NAME\n```\n")
	return builder.String()
}

func docType(field formcatalog.Field) string {
	switch field.Type {
	case formcatalog.TypeBool:
		return "Bool"
	case formcatalog.TypeInt:
		return "Number"
	case formcatalog.TypeIntSet:
		return "Set of Number"
	case formcatalog.TypeStringSet:
		return "Set of String"
	case formcatalog.TypeStringMap:
		return "Map of String"
	default:
		return "String"
	}
}

func docRequirement(field formcatalog.Field) string {
	requirement := "optional"
	if field.Required {
		requirement = "required"
	}
	if field.Immutable {
		requirement += ", forces replacement"
	}
	return requirement
}

func docConstraint(field formcatalog.Field) string {
	var parts []string
	if len(field.Enum) > 0 {
		parts = append(parts, "One of `"+strings.Join(field.Enum, "`, `")+"`.")
	}
	if field.Default != "" {
		parts = append(parts, "Defaults to `"+field.Default+"`.")
	}
	if field.Min != nil && field.Max != nil {
		parts = append(parts, fmt.Sprintf("Between %d and %d.", *field.Min, *field.Max))
	} else if field.Min != nil {
		parts = append(parts, fmt.Sprintf("At least %d.", *field.Min))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func declaredResourceTypes() map[string]struct{} {
	expected := make(map[string]struct{}, len(currentformcatalog.Kinds))
	for _, kind := range currentformcatalog.Kinds {
		expected[kind.ResourceType] = struct{}{}
	}
	return expected
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

// RetiredInventoryPath records the Forms this project published before the
// portable set was rebuilt.
const RetiredInventoryPath = "forms/retired-package-set.json"

// RetiredInventory is the immutable published generation. Its bytes, releases,
// and admission evidence stay verifiable forever; it is simply no longer the
// set this provider implements.
type RetiredInventory struct {
	Format            string           `json:"format"`
	Classification    string           `json:"classification"`
	DefinitionVersion string           `json:"definitionVersion"`
	PackageVersion    string           `json:"packageVersion"`
	SupersededBy      string           `json:"supersededBy"`
	Note              string           `json:"note"`
	Packages          []InventoryEntry `json:"packages"`
}

func generateRetiredInventory(root string) error {
	set, err := AdmissionCandidateSet(root)
	if err != nil {
		return err
	}
	packages := make([]InventoryEntry, 0, len(set.Entries))
	for _, entry := range set.Entries {
		packages = append(packages, InventoryEntry{
			Kind: entry.Kind, Path: entry.PackagePath, AdmissionStatus: "retired",
			ConformanceCase: "retired-" + entry.Slug + "-package",
			FormRef:         entry.FormRef, PackageDigest: entry.PackageDigest,
		})
	}
	inventory := RetiredInventory{
		Format: "takoform.retired-package-set@v1", Classification: "retired",
		DefinitionVersion: set.DefinitionVersion, PackageVersion: set.PackageVersion,
		SupersededBy: portableGeneration,
		Note: "Published immutable bytes. They are never rewritten, re-signed, or reshaped; " +
			"the rebuilt portable Forms start new identities instead.",
		Packages: packages,
	}
	return writeJSON(filepath.Join(root, filepath.FromSlash(RetiredInventoryPath)), inventory)
}

// formInventoryDoc renders the human-readable Form inventory so the published
// list of Forms can never disagree with the declaration the packages are built
// from.
func formInventoryDoc() string {
	return `# Form inventory

The current design target has five independent version axes. They are never a
single maturity label:

| Axis | Current design target | Meaning |
| --- | --- | --- |
| Provider | **Provider 2.1.1** | Registry-published stable-distribution SemVer for Terraform and OpenTofu clients. |
| Host API | ` + "`forms.takoform.com/v1beta1`" + ` | Beta HTTP protocol identifier. |
| Form Family | ` + "`edge.forms.takoform.com/v1beta1`" + ` | Beta family namespace and membership contract. |
| Form definition | ` + "`0.1.0`" + ` | Independent immutable version; each current Form is Experimental. |
| Form Package API | ` + "`packages.forms.takoform.com/v1alpha4`" + ` | Package-envelope schema identifier; current package artifacts remain unpublished. |

Provider 2.1.1 is the current Registry-published client; Provider 2.0.0 is
the published compatibility predecessor and Provider 1.0.3 is published
Legacy. The repository release descriptor remains ` + "`candidate-only`" + ` metadata by
design after owner publication. The Beta label does not apply to Provider 2.1.1.
Conversely, shipping a Form through a stable Provider SemVer does not make that
Form Stable. The 15 current Form Packages remain unpublished until their own
publication authority advances them.
` + v3FormInventorySection() + retainedFormInventorySection()
}

// retainedFormInventorySection renders compatibility and historical evidence
// after the current Edge family. The bytes remain generated and verifiable,
// but old lines no longer define the first thing a reader sees as current.
func retainedFormInventorySection() string {
	var builder strings.Builder
	builder.WriteString(`
## Compatibility: Provider 2.0.0 / retained v1alpha2 Form candidates

This is the Provider 2.0.0 source candidate inventory for the nine Form-backed
Resources retained from the v1alpha2 reset. That dated reset was evaluated
against a Takosumi-hosted preview, but it is provenance only: it does not claim
that a hosted product provides or runs these Resources now, or that any host is
the first host. Every entry is a local Proposal-derived publication candidate
under ` + "`forms.takoform.com/v1alpha2`" + `, awaiting an explicit lifecycle transition
before Experimental; none is published, Experimental, Stable, centrally
approved, or guaranteed commercially available.
Each contract describes what a caller wants without
naming a target, credential, placement, price, or implementation. A host may
publish support and activate an exact FormRef under its own policy.

The frozen v1alpha1 inventory remains verifiable through
` + "[`standard-package-set.json`](standard-package-set.json)" + ` and immutable
release sources, but it is not rendered as the current provider catalog.

`)
	for _, domain := range formInventoryDomains {
		var kinds []formcatalog.Kind
		for _, kind := range currentformcatalog.Kinds {
			if kind.Domain == domain.name {
				kinds = append(kinds, kind)
			}
		}
		if len(kinds) == 0 {
			continue
		}
		fmt.Fprintf(&builder, "### %s\n\n| Kind | Resource | Version | Portable intent |\n| --- | --- | --- | --- |\n", domain.title)
		for _, kind := range kinds {
			fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | %s |\n", kind.Kind, kind.ResourceType, kind.Version(), kind.Description)
		}
		builder.WriteString("\n")
	}
	builder.WriteString(`### Declared runtime interfaces

A Form may declare the runtime interfaces its service exposes. The names are
author-defined and open: there is no registry, no allowlist, and no central
approval. A declaration states what exists and how its non-secret values are
filled; the host creates the record, authorizes consumers, and owns its
lifecycle.

| Kind | Interface |
| --- | --- |
`)
	for _, kind := range currentformcatalog.Kinds {
		for _, declared := range kind.Interfaces {
			fmt.Fprintf(&builder, "| `%s` | `%s@1` (%s) |\n", kind.Kind, declared.Name, strings.Join(declared.Operations, ", "))
		}
	}
	builder.WriteString(`
### Immutable fields

Every Form fixes its ` + "`/name`" + `. A Form that additionally fixes a field states so
in its definition, and the provider enforces replacement for exactly those
fields; the protocol lifecycle proves both.

| Kind | Immutable |
| --- | --- |
`)
	for _, kind := range currentformcatalog.Kinds {
		fmt.Fprintf(&builder, "| `%s` | `%s` |\n", kind.Kind, strings.Join(kind.ImmutableFields(), "`, `"))
	}
	builder.WriteString(`
### Status

Every entry in this retained inventory is an unpublished ` + "`0.1.0`" + ` candidate.
Takoform owns maturity and publication authority. An independent host
implementation or workload — possibly a Takosumi deployment — is one
conformance/adoption evidence source only; it does not establish current
availability, first-host status, or maturity.

The earlier ten-package generation is also retired, not erased. Its immutable
bytes and admission evidence stay verifiable through
[` + "`retired-package-set.json`" + `](retired-package-set.json). Neither retained
set may be rewritten, re-signed, promoted, or used to derive a current approved
subset. Current maturity truth comes from the applicable scoped lifecycle or
family candidate record; Host Support and activation remain separate host-owned
facts.
`)
	return builder.String()
}

// GenerateCurrentPublishedSurfaces writes the generated current and
// compatibility docs and examples. Legacy package/release verification remains
// read-only elsewhere.
func GenerateCurrentPublishedSurfaces(root string) error {
	return generatePublishedSurfaces(root)
}
