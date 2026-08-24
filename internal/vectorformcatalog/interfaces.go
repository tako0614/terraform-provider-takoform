package vectorformcatalog

import (
	"fmt"
	"regexp"
	"slices"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// InterfaceAPIVersion is the exact Interface Definition vocabulary.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// VectorIndexInterfaceName is the Interface provided by VectorIndex.
	VectorIndexInterfaceName = "vector.index"
	draft2020                = "https://json-schema.org/draft/2020-12/schema"

	vectorMaxDimension          = 1536
	vectorMaxIDBytes            = 128
	vectorMaxNamespaceBytes     = 63
	vectorMaxMetadataBytes      = 40960
	vectorMaxMetadataProperties = 128
	vectorMaxMetadataString     = 8192
	vectorMaxUpsertBatch        = 100
	vectorMaxFetchIDs           = 100
	vectorMaxDeleteIDs          = 1000
	vectorMaxTopK               = 256
	vectorMaxFilterClauses      = 16
	vectorMaxInValues           = 64
)

// InterfaceDefinition mirrors interface-definition-v1alpha1.schema.json.
type InterfaceDefinition struct {
	APIVersion  string               `json:"apiVersion"`
	Kind        string               `json:"kind"`
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Operations  []InterfaceOperation `json:"operations"`
	Semantics   InterfaceSemantics   `json:"semantics"`
	Limits      map[string]int64     `json:"limits,omitempty"`
	Fixtures    []InterfaceFixture   `json:"fixtures,omitempty"`
}

type InterfaceOperation struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
	Errors       []string       `json:"errors"`
	Idempotent   bool           `json:"idempotent,omitempty"`
}

type InterfaceSemantics struct {
	Consistency string `json:"consistency"`
	Pagination  string `json:"pagination,omitempty"`
	Delivery    string `json:"delivery,omitempty"`
	Ordering    string `json:"ordering,omitempty"`
}

type InterfaceFixture struct {
	Name  string                 `json:"name"`
	Steps []InterfaceFixtureStep `json:"steps"`
}

type InterfaceFixtureStep struct {
	Operation     string         `json:"operation"`
	Input         map[string]any `json:"input"`
	Expected      map[string]any `json:"expected,omitempty"`
	ExpectedError string         `json:"expectedError,omitempty"`
}

func closedObject(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func operationObject(required []string, properties map[string]any) map[string]any {
	schema := closedObject(required, properties)
	schema["$schema"] = draft2020
	return schema
}

func boundedString(maxLength int) map[string]any {
	return map[string]any{"type": "string", "maxLength": maxLength}
}

func patternedString(pattern string, maxLength int) map[string]any {
	return map[string]any{"type": "string", "pattern": pattern, "maxLength": maxLength}
}

func vectorIDSchema() map[string]any {
	return patternedString(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, vectorMaxIDBytes)
}

func namespaceSchema() map[string]any {
	return patternedString(model.PatternResourceName, vectorMaxNamespaceBytes)
}

func vectorSchema() map[string]any {
	return map[string]any{
		"type":     "array",
		"minItems": 1,
		"maxItems": vectorMaxDimension,
		"items":    map[string]any{"type": "number"},
	}
}

func metadataScalarSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "maxLength": vectorMaxMetadataString},
		map[string]any{"type": "number"},
		map[string]any{"type": "boolean"},
	}}
}

func metadataSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        vectorMaxMetadataProperties,
		"propertyNames":        map[string]any{"type": "string", "maxLength": 128},
		"additionalProperties": metadataScalarSchema(),
	}
}

func vectorRecordSchema() map[string]any {
	return closedObject([]string{"id", "values"}, map[string]any{
		"id":       vectorIDSchema(),
		"values":   vectorSchema(),
		"metadata": metadataSchema(),
	})
}

func filterClauseSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		metadataScalarSchema(),
		closedObject([]string{"in"}, map[string]any{
			"in": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": vectorMaxInValues,
				"items":    metadataScalarSchema(),
			},
		}),
	}}
}

func filterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        vectorMaxFilterClauses,
		"propertyNames":        map[string]any{"type": "string", "maxLength": 128},
		"additionalProperties": filterClauseSchema(),
	}
}

func vectorQueryMatchSchema() map[string]any {
	return closedObject([]string{"id", "score"}, map[string]any{
		"id":       vectorIDSchema(),
		"score":    map[string]any{"type": "number"},
		"values":   vectorSchema(),
		"metadata": metadataSchema(),
	})
}

// InterfaceDefinitions lists the Vector Family's exact Interface contracts in
// stable order.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{vectorIndexInterface()}
}

func vectorIndexInterface() InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: VectorIndexInterfaceName, Version: "1.0.0",
		Title: "Dense vector index",
		Description: "Fixed-dimension dense vector index with namespaced whole-record upsert, " +
			"read-after-write fetch, approximate top-k similarity query, closed equality or inclusion " +
			"metadata filters, and idempotent deletion. Query freshness is eventual and the host may use " +
			"approximate nearest-neighbor retrieval; the declared metric and returned scores remain exact " +
			"for the stored precision.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Ordering: "total"},
		Limits: map[string]int64{
			"maxDimension":          vectorMaxDimension,
			"maxIdBytes":            vectorMaxIDBytes,
			"maxNamespaceNameBytes": vectorMaxNamespaceBytes,
			"maxMetadataBytes":      vectorMaxMetadataBytes,
			"maxUpsertBatchVectors": vectorMaxUpsertBatch,
			"maxFetchIds":           vectorMaxFetchIDs,
			"maxDeleteIds":          vectorMaxDeleteIDs,
			"maxTopK":               vectorMaxTopK,
			"maxFilterClauses":      vectorMaxFilterClauses,
			"maxInValues":           vectorMaxInValues,
		},
		Operations: []InterfaceOperation{
			{
				Name: "upsert",
				Description: "Insert or replace whole records in one namespace. Values and metadata are replaced together; " +
					"a batch larger than the portable floor is rejected rather than partially applied.",
				InputSchema: operationObject([]string{"records"}, map[string]any{
					"namespace": namespaceSchema(),
					"records": map[string]any{
						"type":     "array",
						"minItems": 1,
						"maxItems": vectorMaxUpsertBatch,
						"items":    vectorRecordSchema(),
					},
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "invalid_value", "metadata_too_large", "batch_too_large", "backend_unavailable"},
			},
			{
				Name: "fetch",
				Description: "Read records by id from one namespace. Absent ids are omitted from the result rather than " +
					"reported as errors; a resolved upsert or delete is visible to a subsequent fetch.",
				InputSchema: operationObject([]string{"ids"}, map[string]any{
					"namespace": namespaceSchema(),
					"ids": map[string]any{
						"type": "array", "minItems": 1, "maxItems": vectorMaxFetchIDs, "items": vectorIDSchema(),
					},
				}),
				OutputSchema: operationObject([]string{"records"}, map[string]any{
					"records": map[string]any{"type": "array", "maxItems": vectorMaxFetchIDs, "items": vectorRecordSchema()},
				}),
				Errors:     []string{"invalid_key", "batch_too_large", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "query",
				Description: "Return at most topK approximate nearest neighbors for a query vector in one namespace. " +
					"A closed equality or inclusion filter may narrow metadata matches, and flags control whether values " +
					"and metadata are returned. Query results are eventually fresh.",
				InputSchema: operationObject([]string{"vector", "topK"}, map[string]any{
					"namespace":       namespaceSchema(),
					"vector":          vectorSchema(),
					"topK":            map[string]any{"type": "integer", "minimum": 1, "maximum": vectorMaxTopK},
					"filter":          filterSchema(),
					"includeValues":   map[string]any{"type": "boolean"},
					"includeMetadata": map[string]any{"type": "boolean"},
				}),
				OutputSchema: operationObject([]string{"matches"}, map[string]any{
					"matches": map[string]any{
						"type": "array", "maxItems": vectorMaxTopK, "items": vectorQueryMatchSchema(),
					},
				}),
				Errors:     []string{"invalid_key", "invalid_value", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "delete",
				Description: "Delete up to the portable batch limit of records by id. Deleting an already absent id succeeds " +
					"and query indexes converge asynchronously.",
				InputSchema: operationObject([]string{"ids"}, map[string]any{
					"namespace": namespaceSchema(),
					"ids": map[string]any{
						"type": "array", "minItems": 1, "maxItems": vectorMaxDeleteIDs, "items": vectorIDSchema(),
					},
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "batch_too_large", "backend_unavailable"},
				Idempotent:   true,
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "fetch-missing",
				Steps: []InterfaceFixtureStep{{
					Operation: "fetch", Input: map[string]any{"ids": []any{"missing"}},
					Expected: map[string]any{"records": []any{}},
				}},
			},
			{
				Name: "delete-absent",
				Steps: []InterfaceFixtureStep{{
					Operation: "delete", Input: map[string]any{"ids": []any{"absent"}},
					Expected: map[string]any{},
				}},
			},
		},
	}
}

var (
	interfaceOperationNamePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,63}$`)
	interfaceErrorCodePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)
	interfaceFixtureNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

const (
	interfaceDescriptionMaxLength          = 4096
	interfaceOperationDescriptionMaxLength = 1024
	interfaceTitleMaxLength                = 160
)

// ValidateInterfaceDefinitions proves the local Interface catalog's identity,
// grammar, operation, and fixture invariants before any bytes are rendered.
func ValidateInterfaceDefinitions(definitions []InterfaceDefinition) error {
	names := map[string]struct{}{}
	for _, definition := range definitions {
		if definition.APIVersion != InterfaceAPIVersion || definition.Kind != "InterfaceDefinition" {
			return fmt.Errorf("interface %s has invalid identity %s/%s", definition.Name, definition.APIVersion, definition.Kind)
		}
		if _, duplicate := names[definition.Name]; duplicate {
			return fmt.Errorf("duplicate interface identity %q", definition.Name)
		}
		names[definition.Name] = struct{}{}
		if len(definition.Title) > interfaceTitleMaxLength {
			return fmt.Errorf("interface %s title exceeds %d characters", definition.Name, interfaceTitleMaxLength)
		}
		if len(definition.Description) > interfaceDescriptionMaxLength {
			return fmt.Errorf("interface %s description exceeds %d characters", definition.Name, interfaceDescriptionMaxLength)
		}
		operations := map[string]InterfaceOperation{}
		for _, operation := range definition.Operations {
			if !interfaceOperationNamePattern.MatchString(operation.Name) {
				return fmt.Errorf("interface %s declares operation %q outside the published name grammar", definition.Name, operation.Name)
			}
			if _, duplicate := operations[operation.Name]; duplicate {
				return fmt.Errorf("interface %s declares operation %q twice", definition.Name, operation.Name)
			}
			if len(operation.Description) > interfaceOperationDescriptionMaxLength {
				return fmt.Errorf("interface %s operation %s description exceeds %d characters", definition.Name, operation.Name, interfaceOperationDescriptionMaxLength)
			}
			codes := map[string]struct{}{}
			for _, code := range operation.Errors {
				if !interfaceErrorCodePattern.MatchString(code) {
					return fmt.Errorf("interface %s operation %s declares error %q outside the published grammar", definition.Name, operation.Name, code)
				}
				if _, duplicate := codes[code]; duplicate {
					return fmt.Errorf("interface %s operation %s declares error %q twice", definition.Name, operation.Name, code)
				}
				codes[code] = struct{}{}
			}
			operations[operation.Name] = operation
		}
		fixtures := map[string]struct{}{}
		for _, fixture := range definition.Fixtures {
			if !interfaceFixtureNamePattern.MatchString(fixture.Name) {
				return fmt.Errorf("interface %s fixture %q is outside the published name grammar", definition.Name, fixture.Name)
			}
			if _, duplicate := fixtures[fixture.Name]; duplicate {
				return fmt.Errorf("interface %s declares fixture %q twice", definition.Name, fixture.Name)
			}
			fixtures[fixture.Name] = struct{}{}
			if len(fixture.Steps) == 0 {
				return fmt.Errorf("interface %s fixture %s has no steps", definition.Name, fixture.Name)
			}
			for _, step := range fixture.Steps {
				operation, declared := operations[step.Operation]
				if !declared {
					return fmt.Errorf("interface %s fixture %s exercises undeclared operation %q", definition.Name, fixture.Name, step.Operation)
				}
				if step.Input == nil {
					return fmt.Errorf("interface %s fixture %s step %s carries no input", definition.Name, fixture.Name, step.Operation)
				}
				if step.ExpectedError == "" {
					continue
				}
				if len(step.Expected) > 0 {
					return fmt.Errorf("interface %s fixture %s step %s expects both an output and error", definition.Name, fixture.Name, step.Operation)
				}
				if !slices.Contains(operation.Errors, step.ExpectedError) {
					return fmt.Errorf("interface %s fixture %s expects error %q, which operation %s does not declare", definition.Name, fixture.Name, step.ExpectedError, step.Operation)
				}
			}
		}
	}
	return nil
}

func interfaceDefinitionByName(name string) (InterfaceDefinition, error) {
	for _, definition := range InterfaceDefinitions() {
		if definition.Name == name {
			return definition, nil
		}
	}
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the Vector catalog", name)
}
