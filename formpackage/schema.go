package formpackage

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	formRefSchemaID        = "https://forms.takoform.com/schemas/v1alpha1/form-ref.schema.json"
	formDefinitionSchemaID = "https://forms.takoform.com/schemas/v1alpha1/form-definition.schema.json"
	packageIndexSchemaID   = "https://forms.takoform.com/schemas/v1alpha1/package-index.schema.json"
)

//go:embed schemas/*.schema.json
var schemaFiles embed.FS

type closedSchemaLoader struct{}

func (closedSchemaLoader) Load(resourceURL string) (any, error) {
	return nil, fmt.Errorf("schema resource %q is outside the embedded Form Package schema closure", resourceURL)
}

type compiledSchemas struct {
	formRef    *jsonschema.Schema
	definition *jsonschema.Schema
	index      *jsonschema.Schema
}

var (
	schemasOnce  sync.Once
	schemasValue compiledSchemas
	schemasErr   error
)

func loadSchemas() (compiledSchemas, error) {
	schemasOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.AssertFormat()
		compiler.UseLoader(closedSchemaLoader{})
		files := []string{"form-ref.schema.json", "form-definition.schema.json", "package-index.schema.json"}
		entries, err := schemaFiles.ReadDir("schemas")
		if err != nil {
			schemasErr = fmt.Errorf("read embedded schema closure: %w", err)
			return
		}
		if len(entries) != len(files) {
			schemasErr = fmt.Errorf("embedded schema closure has %d entries, want %d", len(entries), len(files))
			return
		}
		wantFiles := make(map[string]struct{}, len(files))
		for _, file := range files {
			wantFiles[file] = struct{}{}
		}
		for _, entry := range entries {
			if entry.IsDir() {
				schemasErr = fmt.Errorf("embedded schema closure contains directory %q", entry.Name())
				return
			}
			if _, ok := wantFiles[entry.Name()]; !ok {
				schemasErr = fmt.Errorf("embedded schema closure contains unexpected file %q", entry.Name())
				return
			}
		}
		for _, file := range files {
			raw, err := schemaFiles.ReadFile("schemas/" + file)
			if err != nil {
				schemasErr = fmt.Errorf("read embedded schema %s: %w", file, err)
				return
			}
			if _, err := Canonicalize(raw); err != nil {
				schemasErr = fmt.Errorf("embedded schema %s is not I-JSON: %w", file, err)
				return
			}
			value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				schemasErr = fmt.Errorf("decode embedded schema %s: %w", file, err)
				return
			}
			object, ok := value.(map[string]any)
			if !ok || object["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
				schemasErr = fmt.Errorf("embedded schema %s must declare Draft 2020-12", file)
				return
			}
			id, ok := object["$id"].(string)
			if !ok || id == "" {
				schemasErr = fmt.Errorf("embedded schema %s has no $id", file)
				return
			}
			if err := compiler.AddResource(id, value); err != nil {
				schemasErr = fmt.Errorf("register embedded schema %s: %w", file, err)
				return
			}
		}
		schemasValue.formRef, schemasErr = compiler.Compile(formRefSchemaID)
		if schemasErr != nil {
			schemasErr = fmt.Errorf("compile FormRef schema: %w", schemasErr)
			return
		}
		schemasValue.definition, schemasErr = compiler.Compile(formDefinitionSchemaID)
		if schemasErr != nil {
			schemasErr = fmt.Errorf("compile Form Definition schema: %w", schemasErr)
			return
		}
		schemasValue.index, schemasErr = compiler.Compile(packageIndexSchemaID)
		if schemasErr != nil {
			schemasErr = fmt.Errorf("compile package-index schema: %w", schemasErr)
		}
	})
	return schemasValue, schemasErr
}

func validateFormRef(raw []byte) (FormRef, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return FormRef{}, err
	}
	var ref FormRef
	if err := validateDocument(raw, schemas.formRef, &ref); err != nil {
		return FormRef{}, fmt.Errorf("FormRef: %w", err)
	}
	return ref, nil
}

// ValidateFormRef validates the exact four-field Draft 2020-12 FormRef and
// returns its typed value.
func ValidateFormRef(raw []byte) (FormRef, error) {
	return validateFormRef(raw)
}

func validateDefinition(raw []byte) (FormDefinition, any, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return FormDefinition{}, nil, err
	}
	var definition FormDefinition
	var value any
	if err := validateDocumentWithValue(raw, schemas.definition, &definition, &value); err != nil {
		return FormDefinition{}, nil, fmt.Errorf("Form Definition: %w", err)
	}
	if err := rejectForbiddenContent(value, "$"); err != nil {
		return FormDefinition{}, nil, fmt.Errorf("Form Definition content policy: %w", err)
	}
	if _, err := compileInlineSchema(definition.DesiredSchema, "desiredSchema"); err != nil {
		return FormDefinition{}, nil, err
	}
	if _, err := compileInlineSchema(definition.ObservedSchema, "observedSchema"); err != nil {
		return FormDefinition{}, nil, err
	}
	for index, descriptor := range definition.Interfaces {
		if descriptor.DocumentSchema != nil {
			if _, err := compileInlineSchema(descriptor.DocumentSchema, fmt.Sprintf("interfaces[%d].documentSchema", index)); err != nil {
				return FormDefinition{}, nil, err
			}
		}
	}
	if err := validateDefinitionSemantics(definition); err != nil {
		return FormDefinition{}, nil, err
	}
	return definition, value, nil
}

// ValidateDefinition validates the Draft 2020-12 Form Definition, its inline
// schemas, and the fail-closed data-only content policy.
func ValidateDefinition(raw []byte) (FormDefinition, error) {
	definition, _, err := validateDefinition(raw)
	return definition, err
}

func validateIndex(raw []byte) (PackageIndex, any, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return PackageIndex{}, nil, err
	}
	var index PackageIndex
	var value any
	if err := validateDocumentWithValue(raw, schemas.index, &index, &value); err != nil {
		return PackageIndex{}, nil, fmt.Errorf("package index: %w", err)
	}
	return index, value, nil
}

// ValidatePackageIndex validates the exact Draft 2020-12 package-index
// document. Filesystem closure and payload bytes are verified separately by
// VerifyDirectory.
func ValidatePackageIndex(raw []byte) (PackageIndex, error) {
	index, _, err := validateIndex(raw)
	return index, err
}

func validateDocument(raw []byte, schema *jsonschema.Schema, destination any) error {
	var value any
	return validateDocumentWithValue(raw, schema, destination, &value)
}

func validateDocumentWithValue(raw []byte, schema *jsonschema.Schema, destination, valueDestination any) error {
	if _, err := Canonicalize(raw); err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("does not satisfy Draft 2020-12 schema: %w", err)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return err
	}
	if pointer, ok := valueDestination.(*any); ok {
		*pointer = value
	}
	return nil
}

func compileInlineSchema(value map[string]any, field string) (*jsonschema.Schema, error) {
	if err := verifyFragmentOnlyReferences(value, field); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(closedSchemaLoader{})
	id := "https://forms.takoform.com/inline/" + url.PathEscape(field) + ".schema.json"
	if err := compiler.AddResource(id, value); err != nil {
		return nil, fmt.Errorf("%s register Draft 2020-12 schema: %w", field, err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("%s compile Draft 2020-12 schema: %w", field, err)
	}
	return compiled, nil
}

func validateDefinitionSemantics(definition FormDefinition) error {
	interfaces := map[string]struct{}{}
	for _, descriptor := range definition.Interfaces {
		key := descriptor.Name + "@" + descriptor.Version
		if _, duplicate := interfaces[key]; duplicate {
			return fmt.Errorf("Form Definition has duplicate Interface %q", key)
		}
		interfaces[key] = struct{}{}
	}
	fixtures := map[string]struct{}{}
	for _, fixture := range definition.ConformanceFixtures {
		if _, duplicate := fixtures[fixture.Name]; duplicate {
			return fmt.Errorf("Form Definition has duplicate conformance fixture name %q", fixture.Name)
		}
		fixtures[fixture.Name] = struct{}{}
		if err := validatePackagePath(fixture.DesiredPath); err != nil {
			return fmt.Errorf("conformance fixture %q desiredPath: %w", fixture.Name, err)
		}
		if fixture.ObservedPath != "" {
			if err := validatePackagePath(fixture.ObservedPath); err != nil {
				return fmt.Errorf("conformance fixture %q observedPath: %w", fixture.Name, err)
			}
		}
	}
	return nil
}

func verifyFragmentOnlyReferences(value any, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childLocation := location + "." + key
			if key == "$ref" || key == "$dynamicRef" {
				reference, ok := child.(string)
				if !ok || !strings.HasPrefix(reference, "#") {
					return fmt.Errorf("%s must be a document-local fragment; network and package-path references are forbidden", childLocation)
				}
			}
			if err := verifyFragmentOnlyReferences(child, childLocation); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := verifyFragmentOnlyReferences(child, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	}
	return nil
}
