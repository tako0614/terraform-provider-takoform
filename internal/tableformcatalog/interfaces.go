package tableformcatalog

import (
	"fmt"
	"regexp"
	"slices"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// InterfaceAPIVersion is the exact Interface Definition vocabulary.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// TableDocumentInterfaceName is the Interface provided by Table.
	TableDocumentInterfaceName = "table.document"
	draft2020                  = "https://json-schema.org/draft/2020-12/schema"

	tableMaxAttributeNameBytes  = 255
	tableMaxPartitionKeyBytes   = 2048
	tableMaxSortKeyBytes        = 1024
	tableMaxSecondaryIndexes    = 20
	tableMaxItemBytes           = 409600
	tableMaxDocumentDepth       = 32
	tableMaxDocumentProperties  = 1024
	tableMaxDocumentItems       = 1024
	tableMaxQueryPageItems      = 1000
	tableMaxResultBytesPerQuery = 1048576
	tableMaxCursorBytes         = 4096
	tableMaxConditionEntries    = 32
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

func stringSchema(pattern string, maxLength int) map[string]any {
	return map[string]any{"type": "string", "pattern": pattern, "maxLength": maxLength}
}

func boundedString(maxLength int) map[string]any {
	return map[string]any{"type": "string", "maxLength": maxLength}
}

func tableDocumentBytes() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"data", "encoding"},
		"properties": map[string]any{
			"encoding": map[string]any{"type": "string", "enum": []any{"base64"}},
			"data":     boundedString(4 * ((tableMaxItemBytes + 2) / 3)),
		},
	}
}

func tableDocumentObject(child string) map[string]any {
	// A document map whose exact members are {encoding, data} with encoding
	// base64 is reserved for the encoded-bytes value. Excluding that shape from
	// the ordinary map branch keeps the oneOf below unambiguous.
	return map[string]any{
		"type":                 "object",
		"maxProperties":        tableMaxDocumentProperties,
		"propertyNames":        stringSchema(attributeNamePattern, tableMaxAttributeNameBytes),
		"additionalProperties": map[string]any{"$ref": child},
		"not":                  tableDocumentBytes(),
	}
}

// tableDocumentDefs is the closed recursive document value model. The
// encoded-bytes object is reserved by the contract: a plain map with exactly
// those two members is represented as bytes, not as an ordinary document map.
func tableDocumentDefs() map[string]any {
	defs := make(map[string]any, tableMaxDocumentDepth)
	for depth := tableMaxDocumentDepth; depth >= 1; depth-- {
		oneOf := []any{
			map[string]any{"type": "null"},
			map[string]any{"type": "boolean"},
			map[string]any{"type": "number"},
			map[string]any{"type": "string", "maxLength": tableMaxItemBytes},
			tableDocumentBytes(),
		}
		if depth < tableMaxDocumentDepth {
			child := fmt.Sprintf("#/$defs/documentValueDepth%d", depth+1)
			oneOf = append(oneOf,
				map[string]any{
					"type":     "array",
					"maxItems": tableMaxDocumentItems,
					"items":    map[string]any{"$ref": child},
				},
				tableDocumentObject(child),
			)
		}
		defs[fmt.Sprintf("documentValueDepth%d", depth)] = map[string]any{"oneOf": oneOf}
	}
	return defs
}

func withDocumentDefs(schema map[string]any) map[string]any {
	schema["$defs"] = tableDocumentDefs()
	return schema
}

func tableDocumentValue() map[string]any {
	return map[string]any{"$ref": "#/$defs/documentValueDepth1"}
}

func tableKeyScalar(maxBytes int) map[string]any {
	return map[string]any{
		"oneOf": []any{
			map[string]any{"type": "string", "maxLength": maxBytes},
			map[string]any{
				"type": "number", "multipleOf": 1,
				"minimum": -9007199254740991, "maximum": 9007199254740991,
			},
			map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"data", "encoding"},
				"properties": map[string]any{
					"encoding": map[string]any{"type": "string", "enum": []any{"base64"}},
					"data":     boundedString(4 * ((maxBytes + 2) / 3)),
				},
			},
		},
	}
}

func tablePrimaryKey(maxBytes int) map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        2,
		"propertyNames":        stringSchema(attributeNamePattern, tableMaxAttributeNameBytes),
		"additionalProperties": tableKeyScalar(maxBytes),
	}
}

func tableCondition() map[string]any {
	return closedObject(nil, map[string]any{
		"exists": map[string]any{"type": "boolean"},
		"equals": map[string]any{
			"type":          "object",
			"maxProperties": tableMaxConditionEntries,
			"propertyNames": stringSchema(attributeNamePattern, tableMaxAttributeNameBytes),
			"additionalProperties": map[string]any{
				"oneOf": []any{
					map[string]any{"type": "null"}, map[string]any{"type": "boolean"},
					map[string]any{"type": "number"}, map[string]any{"type": "string", "maxLength": tableMaxAttributeNameBytes},
					tableDocumentBytes(),
				},
			},
		},
	})
}

func tableSortCondition() map[string]any {
	return closedObject([]string{"operator", "value"}, map[string]any{
		"operator":    map[string]any{"type": "string", "enum": []any{"eq", "lt", "lte", "gt", "gte", "between", "beginsWith"}},
		"value":       tableKeyScalar(tableMaxSortKeyBytes),
		"secondValue": tableKeyScalar(tableMaxSortKeyBytes),
	})
}

func tableGetOutput() map[string]any {
	return withDocumentDefs(operationObject([]string{"item"}, map[string]any{"item": tableDocumentValue()}))
}

func tableQueryOutput() map[string]any {
	return withDocumentDefs(operationObject([]string{"items"}, map[string]any{
		"items": map[string]any{
			"type":     "array",
			"maxItems": tableMaxQueryPageItems,
			"items":    tableDocumentValue(),
		},
		"cursor": boundedString(tableMaxCursorBytes),
	}))
}

func emptyOutput() map[string]any { return operationObject(nil, map[string]any{}) }

// InterfaceDefinitions lists the Table Family's exact Interface contracts in
// stable order.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{tableDocumentInterface()}
}

func tableDocumentInterface() InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: TableDocumentInterfaceName, Version: "1.0.0",
		Title: "Document table",
		Description: "Key-addressed document storage with a declared partition key, optional sort key, " +
			"consistent single-item reads, atomic conditional whole-item writes, key-ordered partition queries, " +
			"cursor pagination, declared secondary indexes, and lazy TTL. Documents use the closed recursive " +
			"value model and its reserved encoded-bytes shape. Base-table reads are consistent by default; " +
			"secondary-index queries are eventually consistent while an index backfills or converges.",
		Semantics: InterfaceSemantics{Consistency: "per_key_linearizable", Pagination: "cursor", Ordering: "per_key"},
		Limits: map[string]int64{
			"maxAttributeNameBytes":  tableMaxAttributeNameBytes,
			"maxPartitionKeyBytes":   tableMaxPartitionKeyBytes,
			"maxSortKeyBytes":        tableMaxSortKeyBytes,
			"maxSecondaryIndexes":    tableMaxSecondaryIndexes,
			"maxItemBytes":           tableMaxItemBytes,
			"maxDocumentDepth":       tableMaxDocumentDepth,
			"maxQueryPageItems":      tableMaxQueryPageItems,
			"maxResultBytesPerQuery": tableMaxResultBytesPerQuery,
		},
		Operations: []InterfaceOperation{
			{
				Name: "get",
				Description: "Read one item by its complete primary key. A missing item fails with not_found; " +
					"a resolved put or delete is visible to a subsequent get.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": tablePrimaryKey(tableMaxPartitionKeyBytes)}),
				OutputSchema: tableGetOutput(),
				Errors:       []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name: "put",
				Description: "Replace the complete document at one primary key. An optional exists and/or equals " +
					"condition is evaluated and applied atomically; a failed condition changes nothing.",
				InputSchema: withDocumentDefs(operationObject([]string{"item"}, map[string]any{
					"item":      tableDocumentValue(),
					"condition": tableCondition(),
				})),
				OutputSchema: emptyOutput(),
				Errors:       []string{"invalid_key", "invalid_value", "value_too_large", "precondition_failed", "backend_unavailable"},
			},
			{
				Name: "delete",
				Description: "Remove one item by complete primary key. Deleting an absent item succeeds; " +
					"the removal is reflected in the base table and declared indexes.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": tablePrimaryKey(tableMaxPartitionKeyBytes)}),
				OutputSchema: emptyOutput(),
				Errors:       []string{"invalid_key", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name: "query",
				Description: "Read a partition in key order. An optional sort-key condition, direction, limit, " +
					"cursor, or declared secondary-index name narrows the page. Base-table reads are consistent; " +
					"index reads are eventually consistent and a backfilling index is temporarily busy.",
				InputSchema: operationObject([]string{"partitionKey"}, map[string]any{
					"partitionKey": tableKeyScalar(tableMaxPartitionKeyBytes),
					"sortKey":      tableSortCondition(),
					"indexName":    stringSchema(model.PatternResourceName, model.ResourceNameMaxLength),
					"direction":    map[string]any{"type": "string", "enum": []any{"asc", "desc"}},
					"limit":        map[string]any{"type": "integer", "minimum": 1, "maximum": tableMaxQueryPageItems},
					"cursor":       boundedString(tableMaxCursorBytes),
				}),
				OutputSchema: tableQueryOutput(),
				Errors:       []string{"invalid_key", "invalid_cursor", "not_found", "resource_busy", "backend_unavailable"},
				Idempotent:   true,
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "missing-item",
				Steps: []InterfaceFixtureStep{{
					Operation: "get", Input: map[string]any{"key": map[string]any{"tenantId": "missing"}},
					ExpectedError: "not_found",
				}},
			},
			{
				Name: "delete-absent-item",
				Steps: []InterfaceFixtureStep{{
					Operation: "delete", Input: map[string]any{"key": map[string]any{"tenantId": "absent"}},
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
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the Table catalog", name)
}
