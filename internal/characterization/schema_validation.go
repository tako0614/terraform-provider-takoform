package characterization

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type closedEvidenceLoader struct{}

func (closedEvidenceLoader) Load(resourceURL string) (any, error) {
	return nil, fmt.Errorf("schema resource %q is outside the closed candidate evidence set", resourceURL)
}

type compiledEvidenceSchemas struct {
	byCategory map[string]*jsonschema.Schema
}

func validateEvidenceSchemas(root string) error {
	compiled, err := compileEvidenceSchemas(root)
	if err != nil {
		return err
	}
	for category, fixturePath := range CaseFiles {
		schema := compiled.byCategory[category]
		if schema == nil {
			return fmt.Errorf("no compiled schema for fixture category %q", category)
		}
		if err := validateJSONFile(schema, filepath.Join(root, fixturePath)); err != nil {
			return fmt.Errorf("%s fixture does not satisfy Draft 2020-12 schema: %w", category, err)
		}
	}
	discoverySchema := compiled.byCategory["discovery"]
	if discoverySchema == nil {
		return errors.New("no compiled discovery schema")
	}
	if err := validateJSONFile(discoverySchema, filepath.Join(root, DiscoveryFile)); err != nil {
		return fmt.Errorf("discovery fixture does not satisfy Draft 2020-12 schema: %w", err)
	}
	return nil
}

func compileEvidenceSchemas(root string) (compiledEvidenceSchemas, error) {
	if err := verifySchemaDirectoryClosure(root); err != nil {
		return compiledEvidenceSchemas{}, err
	}

	type schemaDocument struct {
		category string
		relative string
		id       string
		value    any
	}
	documents := make([]schemaDocument, 0, len(SchemaFiles))
	categories := make([]string, 0, len(SchemaFiles))
	for category := range SchemaFiles {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	for _, category := range categories {
		relative := SchemaFiles[category]
		fullPath := filepath.Join(root, filepath.FromSlash(relative))
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			return compiledEvidenceSchemas{}, fmt.Errorf("read %s: %w", relative, err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return compiledEvidenceSchemas{}, fmt.Errorf("decode schema %s: %w", relative, err)
		}
		object, ok := value.(map[string]any)
		if !ok {
			return compiledEvidenceSchemas{}, fmt.Errorf("schema %s must be an object", relative)
		}
		if object["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			return compiledEvidenceSchemas{}, fmt.Errorf("schema %s is not Draft 2020-12", relative)
		}
		if !strings.Contains(strings.ToLower(fmt.Sprint(object["title"])), "compatibility candidate") {
			return compiledEvidenceSchemas{}, fmt.Errorf("schema %s title must identify compatibility candidate scope", relative)
		}
		id, ok := object["$id"].(string)
		if !ok || id == "" {
			return compiledEvidenceSchemas{}, fmt.Errorf("schema %s has no absolute $id", relative)
		}
		parsedID, err := url.Parse(id)
		if err != nil || !parsedID.IsAbs() || parsedID.Fragment != "" {
			return compiledEvidenceSchemas{}, fmt.Errorf("schema %s has invalid $id %q", relative, id)
		}
		wantID := "https://forms.takoform.com/conformance/compatibility-candidate-v1/" + strings.TrimPrefix(relative, "schemas/")
		if id != wantID {
			return compiledEvidenceSchemas{}, fmt.Errorf("schema %s has $id %q, want closed candidate id %q", relative, id, wantID)
		}
		if err := verifyClosedReferences(value, relative); err != nil {
			return compiledEvidenceSchemas{}, err
		}
		documents = append(documents, schemaDocument{category: category, relative: relative, id: id, value: value})
	}

	// A pinned standards implementation is intentional here: title/$schema
	// inspection or a hand-written subset cannot prove Draft 2020-12 behavior,
	// especially allOf plus unevaluatedProperties composition.
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(closedEvidenceLoader{})
	for _, document := range documents {
		if err := compiler.AddResource(document.id, document.value); err != nil {
			return compiledEvidenceSchemas{}, fmt.Errorf("register schema %s: %w", document.relative, err)
		}
	}
	result := compiledEvidenceSchemas{byCategory: make(map[string]*jsonschema.Schema, len(documents))}
	for _, document := range documents {
		compiled, err := compiler.Compile(document.id)
		if err != nil {
			return compiledEvidenceSchemas{}, fmt.Errorf("compile Draft 2020-12 schema %s: %w", document.relative, err)
		}
		result.byCategory[document.category] = compiled
	}
	return result, nil
}

func verifySchemaDirectoryClosure(root string) error {
	want := make([]string, 0, len(SchemaFiles))
	for _, relative := range SchemaFiles {
		want = append(want, filepath.ToSlash(relative))
	}
	sort.Strings(want)
	entries, err := os.ReadDir(filepath.Join(root, "schemas"))
	if err != nil {
		return fmt.Errorf("read schemas directory: %w", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("unexpected non-schema entry schemas/%s", entry.Name())
		}
		got = append(got, "schemas/"+entry.Name())
	}
	sort.Strings(got)
	if len(got) != len(want) {
		return fmt.Errorf("schema closure has %d files, manifest set has %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("schema closure entry %d is %q, want %q", index, got[index], want[index])
		}
	}
	return nil
}

func verifyClosedReferences(value any, schemaPath string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" || key == "$dynamicRef" {
				reference, ok := child.(string)
				if !ok {
					return fmt.Errorf("schema %s has non-string $ref", schemaPath)
				}
				if err := verifyClosedReference(reference, schemaPath); err != nil {
					return err
				}
			}
			if err := verifyClosedReferences(child, schemaPath); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := verifyClosedReferences(child, schemaPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyClosedReference(reference, schemaPath string) error {
	parsed, err := url.Parse(reference)
	if err != nil {
		return fmt.Errorf("schema %s has invalid $ref %q: %w", schemaPath, reference, err)
	}
	if parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(reference, "//") {
		return fmt.Errorf("schema %s has forbidden network $ref %q", schemaPath, reference)
	}
	if parsed.Path == "" {
		return nil
	}
	if strings.Contains(parsed.Path, "\\") {
		return fmt.Errorf("schema %s has forbidden backslash-path $ref %q", schemaPath, reference)
	}
	if strings.HasPrefix(parsed.Path, "/") {
		return fmt.Errorf("schema %s has forbidden absolute-path $ref %q", schemaPath, reference)
	}
	cleaned := path.Clean(parsed.Path)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("schema %s has path-escaping $ref %q", schemaPath, reference)
	}
	return nil
}

func validateJSONFile(schema *jsonschema.Schema, filePath string) error {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}
