package standardforms

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// generatePublishedSurfaces writes the HCL example and the reference document
// of every declared Form.
//
// They are generated for the same reason the schema and the Form Definition
// are: a Form is declared once. A hand-written example is a second, slowly
// diverging description of the same contract.
func generatePublishedSurfaces(root string) error {
	if err := generateExamples(root); err != nil {
		return err
	}
	return generateResourceDocs(root)
}

func generateExamples(root string) error {
	examplesRoot := filepath.Join(root, "examples", "resources")
	if err := pruneGeneratedDirectories(examplesRoot, declaredResourceTypes()); err != nil {
		return err
	}
	for _, kind := range formcatalog.Kinds {
		directory := filepath.Join(examplesRoot, kind.ResourceType)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, "resource.tf"), []byte(exampleHCL(kind)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func exampleHCL(kind formcatalog.Kind) string {
	var builder strings.Builder
	builder.WriteString(`terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
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
	case "artifactPath":
		return "artifact_path"
	case "artifactUrl":
		return "artifact_url"
	case "artifactRef":
		return "artifact_ref"
	default:
		return "artifact_sha256"
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
	default:
		return fmt.Sprintf("%q", fmt.Sprint(value))
	}
}

func generateResourceDocs(root string) error {
	docsRoot := filepath.Join(root, "docs", "resources")
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		return err
	}
	expected := map[string]struct{}{}
	for _, kind := range formcatalog.Kinds {
		expected[docBasename(kind)] = struct{}{}
	}
	entries, err := os.ReadDir(docsRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, keep := expected[entry.Name()]; !keep {
			if err := os.RemoveAll(filepath.Join(docsRoot, entry.Name())); err != nil {
				return err
			}
		}
	}
	for _, kind := range formcatalog.Kinds {
		if err := os.WriteFile(filepath.Join(docsRoot, docBasename(kind)), []byte(resourceDoc(kind)), 0o644); err != nil {
			return err
		}
	}
	return nil
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
an implementation. See the [complete example](../../examples/resources/%s/resource.tf).

## Arguments

`, kind.ResourceType, kind.Description, kind.ResourceType, kind.Description, kind.ResourceType)

	fmt.Fprintf(&builder, "- `name` (String, required, forces replacement) — Resource name.\n")
	if kind.Artifact {
		builder.WriteString("- `artifact_path` / `artifact_url` / `artifact_ref` (String, optional) — Exactly one immutable artifact source. `artifact_url` and `artifact_ref` require `artifact_sha256`.\n")
		builder.WriteString("- `artifact_sha256` (String, optional) — Expected artifact digest.\n")
	}
	for _, field := range kind.Fields {
		fmt.Fprintf(&builder, "- `%s` (%s, %s) — %s%s\n",
			field.HCL, docType(field), docRequirement(field), field.Doc, docConstraint(field))
	}
	switch kind.Connections {
	case formcatalog.ConnectionsRequired:
		builder.WriteString("- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.\n")
	case formcatalog.ConnectionsOptional:
		builder.WriteString("- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.\n")
	}
	builder.WriteString("- `space` (String, optional, forces replacement) — Overrides the provider default.\n")

	builder.WriteString(`
## Read-only attributes

` + "`id`" + `, ` + "`resource_version`" + `, ` + "`drift_status`" + `, ` + "`portability`" + `, and ` + "`outputs`" + ` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.
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
	expected := make(map[string]struct{}, len(formcatalog.Kinds))
	for _, kind := range formcatalog.Kinds {
		expected[kind.ResourceType] = struct{}{}
	}
	return expected
}

func pruneGeneratedDirectories(root string, expected map[string]struct{}) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, keep := expected[entry.Name()]; !keep {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
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
