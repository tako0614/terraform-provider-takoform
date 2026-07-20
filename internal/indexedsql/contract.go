// Package indexedsql owns the provider-neutral SQLDatabase@2.0.0 schema and
// the data.indexed@1 request contract. It contains no host implementation,
// target selection, credentials, or commercial policy.
package indexedsql

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	DefinitionVersion  = "2.0.0"
	PackageVersion     = "2.0.0"
	InterfaceName      = "data.indexed"
	InterfaceVersion   = "1"
	OperationMethod    = "POST"
	OperationPath      = "/indexed/v1/operations"
	RequestSchemaPath  = "interfaces/data.indexed/v1/request.schema.json"
	ResponseSchemaPath = "interfaces/data.indexed/v1/response.schema.json"

	MaxTables                = 16
	MaxColumnsPerTable       = 32
	MaxIndexesPerTable       = 8
	MaxKeyColumns            = 4
	MaxPageSize              = 100
	MaxBatchMutations        = 25
	MaxRowBytes              = 8 << 10
	MaxStringBytes           = 4 << 10
	MaxRequestBytes          = 1 << 20
	MaxResultBytes           = 1 << 20
	MaxCursorBytes           = 64 << 10
	CursorTTLSeconds         = 900
	MaxSafeInteger     int64 = 9007199254740991
	MinSafeInteger     int64 = -MaxSafeInteger
	MaxRevision        int64 = MaxSafeInteger
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	valueTypes        = []string{"string", "integer", "number", "boolean"}
	keyValueTypes     = map[string]struct{}{"string": {}, "integer": {}, "boolean": {}}
	operations        = []string{"get", "get_unique", "page", "put", "delete", "batch"}
)

type columnSpec struct {
	kind     string
	nullable bool
}

// DesiredSchema returns the closed SQLDatabase@2.0.0 desired-state schema.
func DesiredSchema() map[string]any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"name", "schemaVersion", "tables"},
		"properties": map[string]any{
			"name":          portableNameSchema(128),
			"schemaVersion": map[string]any{"type": "integer", "const": 1},
			"tables": map[string]any{
				"type": "array", "minItems": 1, "maxItems": MaxTables, "uniqueItems": true,
				"items": map[string]any{"$ref": "#/$defs/table"},
			},
		},
		"$defs": tableDefinitions(),
	}
}

// OutputSchema returns the public, sanitized output required to materialize
// data.indexed@1. It deliberately contains no endpoint credential or host id.
func OutputSchema() map[string]any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "kind", "name", "generation", "portability", "schemaVersion", "tables"},
		"properties": map[string]any{
			"id":            map[string]any{"type": "string", "minLength": 1},
			"kind":          map[string]any{"type": "string", "const": "SQLDatabase"},
			"name":          map[string]any{"type": "string", "minLength": 1},
			"generation":    map[string]any{"type": "integer", "minimum": 1},
			"portability":   map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
			"schemaVersion": map[string]any{"type": "integer", "const": 1},
			"tables": map[string]any{
				"type": "array", "minItems": 1, "maxItems": MaxTables, "uniqueItems": true,
				"items": map[string]any{"$ref": "#/$defs/table"},
			},
		},
		"$defs": tableDefinitions(),
	}
}

// InterfaceDescriptor returns the exact portable data.indexed@1 declaration.
// Runtime records, routes, grants, and lifecycle remain host-owned.
func InterfaceDescriptor() formpackage.InterfaceDescriptor {
	requestSchema := mustJSONNativeSchema(RequestSchema())
	responseSchema := mustJSONNativeSchema(ResponseSchema())
	schemas := map[string]any{
		"request": map[string]any{
			"packagePath": RequestSchemaPath, "schemaDigest": canonicalSchemaDigest(requestSchema), "schema": requestSchema,
		},
		"response": map[string]any{
			"packagePath": ResponseSchemaPath, "schemaDigest": canonicalSchemaDigest(responseSchema), "schema": responseSchema,
		},
	}
	responses := map[string]any{
		"successStatus": 200, "conflictStatus": 409,
		"operations": map[string]any{
			"get": "#/$defs/getResult", "get_unique": "#/$defs/getUniqueResult",
			"page": "#/$defs/pageResult", "put": "#/$defs/putResult",
			"delete": "#/$defs/deleteResult", "batch": "#/$defs/batchResult",
		},
		"conflicts": map[string]any{
			"revision_conflict": "#/$defs/revisionConflict",
			"unique_conflict":   "#/$defs/uniqueConflict",
		},
	}
	ordering := map[string]any{
		"direction": "asc", "string": "unsigned-utf8-byte-lexicographic",
		"integer": "signed-numeric", "boolean": "false-before-true",
		"composite":   "declared-index-columns-then-missing-primary-key-columns",
		"keyPresence": "required-and-non-null", "prefix": "exact-leading-index-columns",
		"range": "next-index-column-only",
	}
	cursor := map[string]any{
		"mode": "exclusive-live-keyset", "integrity": "tamper-evident",
		"boundTo": []any{
			"resourceIdentity", "resourceGeneration", "formIdentity", "schema",
			"query", "filter", "ordering",
		},
		"unchangedDataset":    "no-duplicates-or-omissions",
		"concurrentMutations": "live-no-snapshot",
	}
	document := map[string]any{
		"method":     OperationMethod,
		"path":       OperationPath,
		"operations": stringValues(operations),
		"valueTypes": stringValues(valueTypes),
		"schemas":    schemas,
		"responses":  responses,
		"ordering":   ordering,
		"cursor":     cursor,
		"limits": map[string]any{
			"tables": MaxTables, "columnsPerTable": MaxColumnsPerTable,
			"indexesPerTable": MaxIndexesPerTable, "keyColumns": MaxKeyColumns,
			"pageSize": MaxPageSize, "batchMutations": MaxBatchMutations,
			"rowBytes": MaxRowBytes, "stringBytes": MaxStringBytes,
			"requestBytes": MaxRequestBytes, "resultBytes": MaxResultBytes,
			"cursorBytes": MaxCursorBytes, "cursorTtlSeconds": CursorTTLSeconds, "maxRevision": MaxRevision,
			"numericMinimum": MinSafeInteger, "numericMaximum": MaxSafeInteger,
		},
	}
	return formpackage.InterfaceDescriptor{
		Name: InterfaceName, Version: InterfaceVersion,
		Description: "Bounded primary-key and declared-index data operations without caller-provided SQL, DDL, filters, ordering, or offsets.",
		Required:    true, ResourceURIInput: "resource_uri",
		Document: document,
		DocumentSchema: map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
			"required": []string{"method", "path", "operations", "valueTypes", "schemas", "responses", "ordering", "cursor", "limits"},
			"properties": map[string]any{
				"method": map[string]any{"type": "string", "const": OperationMethod},
				"path":   map[string]any{"type": "string", "const": OperationPath},
				"operations": map[string]any{
					"type": "array", "minItems": len(operations), "maxItems": len(operations), "uniqueItems": true,
					"items": map[string]any{"type": "string", "enum": append([]string(nil), operations...)},
				},
				"valueTypes": map[string]any{
					"type": "array", "minItems": len(valueTypes), "maxItems": len(valueTypes), "uniqueItems": true,
					"items": map[string]any{"type": "string", "enum": append([]string(nil), valueTypes...)},
				},
				"schemas":   closedConstObjectSchema(schemas),
				"responses": closedConstObjectSchema(responses),
				"ordering":  closedConstObjectSchema(ordering),
				"cursor":    closedConstObjectSchema(cursor),
				"limits":    interfaceLimitsSchema(),
			},
		},
		Inputs: []formpackage.InterfaceInputDeclaration{
			{Name: "resource", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/id"},
			{Name: "name", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/name"},
			{Name: "generation", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/generation"},
			{Name: "schemaVersion", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/schemaVersion"},
			{Name: "tables", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/tables"},
			{Name: "resource_uri", Source: formpackage.InterfaceInputSourceResourceURI},
		},
	}
}

// RequestSchema is the canonical single-endpoint data.indexed@1 request union.
// Closed objects intentionally reject raw SQL/DDL, arbitrary filters/order,
// and offset pagination.
func RequestSchema() map[string]any {
	identifier := identifierSchema()
	keyValue := keyValueSchema()
	key := keySchema()
	prefix := map[string]any{
		"type": "object", "minProperties": 0, "maxProperties": MaxKeyColumns,
		"propertyNames": identifier, "additionalProperties": keyValue,
	}
	row := rowSchema()
	revision := revisionSchema()
	rangeSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"column"},
		"properties": map[string]any{
			"column": identifier, "gt": keyValue, "gte": keyValue, "lt": keyValue, "lte": keyValue,
		},
		"allOf": []any{
			map[string]any{"anyOf": []any{
				map[string]any{"required": []string{"gt"}}, map[string]any{"required": []string{"gte"}},
				map[string]any{"required": []string{"lt"}}, map[string]any{"required": []string{"lte"}},
			}},
			map[string]any{"not": map[string]any{"required": []string{"gt", "gte"}}},
			map[string]any{"not": map[string]any{"required": []string{"lt", "lte"}}},
		},
	}
	putMutation := closedOperation("put", []string{"table", "row"}, map[string]any{
		"table": identifier, "row": row, "expectedRevision": revision,
	})
	deleteMutation := closedOperation("delete", []string{"table", "key"}, map[string]any{
		"table": identifier, "key": key, "expectedRevision": revision,
	})
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"oneOf": []any{
			closedOperation("get", []string{"table", "key"}, map[string]any{"table": identifier, "key": key}),
			closedOperation("get_unique", []string{"table", "index", "key"}, map[string]any{"table": identifier, "index": identifier, "key": key}),
			closedOperation("page", []string{"table", "index", "prefix"}, map[string]any{
				"table": identifier, "index": identifier, "prefix": prefix, "range": rangeSchema,
				"cursor": cursorSchema(),
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": MaxPageSize},
			}),
			putMutation,
			deleteMutation,
			closedOperation("batch", []string{"mutations"}, map[string]any{
				"mutations": map[string]any{
					"type": "array", "minItems": 1, "maxItems": MaxBatchMutations,
					"items": map[string]any{"oneOf": []any{putMutation, deleteMutation}},
				},
			}),
		},
		"$defs": map[string]any{},
	}
}

// ResponseSchema is the canonical data.indexed@1 success and whole-operation
// conflict union. Successful operation bodies use HTTP 200; revision and
// uniqueness conflicts use HTTP 409. Authentication and malformed-request
// failures are transport concerns and cannot masquerade as this union.
func ResponseSchema() map[string]any {
	rowResult := closedObject([]string{"row", "revision"}, map[string]any{
		"row": rowSchema(), "revision": storedRevisionSchema(),
	})
	item := map[string]any{"oneOf": []any{
		map[string]any{"type": "null"}, map[string]any{"$ref": "#/$defs/rowResult"},
	}}
	getResult := closedObject([]string{"operation", "item"}, map[string]any{
		"operation": map[string]any{"const": "get"}, "item": item,
	})
	getUniqueResult := closedObject([]string{"operation", "item"}, map[string]any{
		"operation": map[string]any{"const": "get_unique"}, "item": item,
	})
	pageResult := closedObject([]string{"operation", "items", "nextCursor"}, map[string]any{
		"operation": map[string]any{"const": "page"},
		"items": map[string]any{
			"type": "array", "maxItems": MaxPageSize, "items": map[string]any{"$ref": "#/$defs/rowResult"},
		},
		"nextCursor": map[string]any{"oneOf": []any{cursorSchema(), map[string]any{"type": "null"}}},
	})
	putResult := closedObject([]string{"operation", "item"}, map[string]any{
		"operation": map[string]any{"const": "put"}, "item": map[string]any{"$ref": "#/$defs/rowResult"},
	})
	deleteResult := closedObject([]string{"operation", "deleted"}, map[string]any{
		"operation": map[string]any{"const": "delete"}, "deleted": map[string]any{"type": "boolean"},
	})
	batchResult := closedObject([]string{"operation", "results"}, map[string]any{
		"operation": map[string]any{"const": "batch"},
		"results": map[string]any{
			"type": "array", "minItems": 1, "maxItems": MaxBatchMutations,
			"items": map[string]any{"oneOf": []any{
				map[string]any{"$ref": "#/$defs/putResult"}, map[string]any{"$ref": "#/$defs/deleteResult"},
			}},
		},
	})
	revisionConflict := closedObject([]string{"operation", "conflict"}, map[string]any{
		"operation": map[string]any{"type": "string", "enum": []string{"put", "delete", "batch"}},
		"conflict": closedObject([]string{"reason", "table", "key"}, map[string]any{
			"reason": map[string]any{"const": "revision_conflict"}, "table": identifierSchema(), "key": keySchema(),
		}),
	})
	uniqueConflict := closedObject([]string{"operation", "conflict"}, map[string]any{
		"operation": map[string]any{"type": "string", "enum": []string{"put", "batch"}},
		"conflict": closedObject([]string{"reason", "table", "index"}, map[string]any{
			"reason": map[string]any{"const": "unique_conflict"}, "table": identifierSchema(), "index": identifierSchema(),
		}),
	})
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"oneOf": []any{
			map[string]any{"$ref": "#/$defs/getResult"}, map[string]any{"$ref": "#/$defs/getUniqueResult"},
			map[string]any{"$ref": "#/$defs/pageResult"},
			map[string]any{"$ref": "#/$defs/putResult"}, map[string]any{"$ref": "#/$defs/deleteResult"},
			map[string]any{"$ref": "#/$defs/batchResult"}, map[string]any{"$ref": "#/$defs/revisionConflict"},
			map[string]any{"$ref": "#/$defs/uniqueConflict"},
		},
		"$defs": map[string]any{
			"rowResult": rowResult, "getResult": getResult, "getUniqueResult": getUniqueResult, "pageResult": pageResult,
			"putResult": putResult, "deleteResult": deleteResult, "batchResult": batchResult,
			"revisionConflict": revisionConflict, "uniqueConflict": uniqueConflict,
		},
	}
}

// ValidateDesired enforces the semantic constraints that JSON Schema cannot
// express: unique names, declared key columns, and non-null/indexable key
// column types.
func ValidateDesired(value map[string]any) error {
	if value == nil {
		return fmt.Errorf("desired document is required")
	}
	if version, ok := integerValue(value["schemaVersion"]); !ok || version != 1 {
		return fmt.Errorf("schemaVersion must be 1")
	}
	tables, ok := anySlice(value["tables"])
	if !ok || len(tables) < 1 || len(tables) > MaxTables {
		return fmt.Errorf("tables must contain 1..%d entries", MaxTables)
	}
	tableNames := map[string]struct{}{}
	for tableIndex, rawTable := range tables {
		table, ok := rawTable.(map[string]any)
		if !ok {
			return fmt.Errorf("tables[%d] must be an object", tableIndex)
		}
		tableName, _ := table["name"].(string)
		if !identifierPattern.MatchString(tableName) {
			return fmt.Errorf("tables[%d].name is invalid", tableIndex)
		}
		if _, duplicate := tableNames[tableName]; duplicate {
			return fmt.Errorf("duplicate table name %q", tableName)
		}
		tableNames[tableName] = struct{}{}
		if err := validateTable(tableName, table); err != nil {
			return err
		}
	}
	return nil
}

func validateTable(tableName string, table map[string]any) error {
	columns, ok := anySlice(table["columns"])
	if !ok || len(columns) < 1 || len(columns) > MaxColumnsPerTable {
		return fmt.Errorf("table %q columns must contain 1..%d entries", tableName, MaxColumnsPerTable)
	}
	columnNames := map[string]columnSpec{}
	for index, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			return fmt.Errorf("table %q columns[%d] must be an object", tableName, index)
		}
		name, _ := column["name"].(string)
		kind, _ := column["type"].(string)
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("table %q columns[%d].name is invalid", tableName, index)
		}
		if _, duplicate := columnNames[name]; duplicate {
			return fmt.Errorf("table %q has duplicate column %q", tableName, name)
		}
		if !contains(valueTypes, kind) {
			return fmt.Errorf("table %q column %q has invalid type %q", tableName, name, kind)
		}
		nullable, _ := column["nullable"].(bool)
		columnNames[name] = columnSpec{kind: kind, nullable: nullable}
	}
	primaryKey, ok := stringSlice(table["primaryKey"])
	if !ok {
		return fmt.Errorf("table %q primaryKey must contain string column names", tableName)
	}
	if err := validateKeyColumns(tableName, "primaryKey", primaryKey, columnNames); err != nil {
		return err
	}
	indexes, ok := anySliceOptional(table["indexes"])
	if !ok || len(indexes) > MaxIndexesPerTable {
		return fmt.Errorf("table %q indexes must contain at most %d entries", tableName, MaxIndexesPerTable)
	}
	indexNames := map[string]struct{}{}
	for position, rawIndex := range indexes {
		index, ok := rawIndex.(map[string]any)
		if !ok {
			return fmt.Errorf("table %q indexes[%d] must be an object", tableName, position)
		}
		name, _ := index["name"].(string)
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("table %q indexes[%d].name is invalid", tableName, position)
		}
		if _, duplicate := indexNames[name]; duplicate {
			return fmt.Errorf("table %q has duplicate index %q", tableName, name)
		}
		indexNames[name] = struct{}{}
		keyColumns, ok := stringSlice(index["columns"])
		if !ok {
			return fmt.Errorf("table %q index %q columns must be strings", tableName, name)
		}
		if err := validateKeyColumns(tableName, "index "+name, keyColumns, columnNames); err != nil {
			return err
		}
	}
	return nil
}

func validateKeyColumns(tableName, subject string, names []string, columns map[string]columnSpec) error {
	if len(names) < 1 || len(names) > MaxKeyColumns {
		return fmt.Errorf("table %q %s must contain 1..%d columns", tableName, subject, MaxKeyColumns)
	}
	seen := map[string]struct{}{}
	for _, name := range names {
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("table %q %s repeats column %q", tableName, subject, name)
		}
		seen[name] = struct{}{}
		column, ok := columns[name]
		if !ok {
			return fmt.Errorf("table %q %s references unknown column %q", tableName, subject, name)
		}
		if column.nullable {
			return fmt.Errorf("table %q %s column %q must be non-null", tableName, subject, name)
		}
		if _, indexable := keyValueTypes[column.kind]; !indexable {
			return fmt.Errorf("table %q %s column %q type %q is not indexable", tableName, subject, name, column.kind)
		}
	}
	return nil
}

func tableDefinitions() map[string]any {
	return map[string]any{
		"identifier": identifierSchema(),
		"column": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"name", "type"},
			"properties": map[string]any{
				"name":     map[string]any{"$ref": "#/$defs/identifier"},
				"type":     map[string]any{"type": "string", "enum": append([]string(nil), valueTypes...)},
				"nullable": map[string]any{"type": "boolean", "default": false},
			},
		},
		"index": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"name", "columns"},
			"properties": map[string]any{
				"name": map[string]any{"$ref": "#/$defs/identifier"},
				"columns": map[string]any{
					"type": "array", "minItems": 1, "maxItems": MaxKeyColumns, "uniqueItems": true,
					"items": map[string]any{"$ref": "#/$defs/identifier"},
				},
				"unique": map[string]any{"type": "boolean", "default": false},
			},
		},
		"table": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"name", "columns", "primaryKey"},
			"properties": map[string]any{
				"name": map[string]any{"$ref": "#/$defs/identifier"},
				"columns": map[string]any{
					"type": "array", "minItems": 1, "maxItems": MaxColumnsPerTable, "uniqueItems": true,
					"items": map[string]any{"$ref": "#/$defs/column"},
				},
				"primaryKey": map[string]any{
					"type": "array", "minItems": 1, "maxItems": MaxKeyColumns, "uniqueItems": true,
					"items": map[string]any{"$ref": "#/$defs/identifier"},
				},
				"indexes": map[string]any{
					"type": "array", "maxItems": MaxIndexesPerTable, "uniqueItems": true,
					"items": map[string]any{"$ref": "#/$defs/index"},
				},
			},
		},
	}
}

func interfaceLimitsSchema() map[string]any {
	properties := map[string]any{
		"tables": MaxTables, "columnsPerTable": MaxColumnsPerTable,
		"indexesPerTable": MaxIndexesPerTable, "keyColumns": MaxKeyColumns,
		"pageSize": MaxPageSize, "batchMutations": MaxBatchMutations,
		"rowBytes": MaxRowBytes, "stringBytes": MaxStringBytes,
		"requestBytes": MaxRequestBytes, "resultBytes": MaxResultBytes,
		"cursorBytes": MaxCursorBytes, "cursorTtlSeconds": CursorTTLSeconds, "maxRevision": MaxRevision,
		"numericMinimum": MinSafeInteger, "numericMaximum": MaxSafeInteger,
	}
	names := make([]string, 0, len(properties))
	schemaProperties := make(map[string]any, len(properties))
	for name, value := range properties {
		names = append(names, name)
		schemaProperties[name] = map[string]any{"type": "integer", "const": value}
	}
	sort.Strings(names)
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": names, "properties": schemaProperties,
	}
}

func closedConstObjectSchema(document map[string]any) map[string]any {
	names := make([]string, 0, len(document))
	properties := make(map[string]any, len(document))
	for name, value := range document {
		names = append(names, name)
		if nested, ok := value.(map[string]any); ok {
			// The embedded interface schema is immutable package data. Keeping it
			// as one exact const avoids interpreting its JSON Schema keywords as
			// keywords of the descriptor's own document schema.
			if name == "schema" {
				properties[name] = map[string]any{"const": nested}
			} else {
				properties[name] = closedConstObjectSchema(nested)
			}
		} else {
			properties[name] = map[string]any{"const": value}
		}
	}
	sort.Strings(names)
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": names, "properties": properties,
	}
}

func closedOperation(operation string, required []string, properties map[string]any) map[string]any {
	copyProperties := make(map[string]any, len(properties)+1)
	copyProperties["operation"] = map[string]any{"type": "string", "const": operation}
	for key, value := range properties {
		copyProperties[key] = value
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": append([]string{"operation"}, required...), "properties": copyProperties,
	}
}

func closedObject(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": append([]string(nil), required...), "properties": properties,
	}
}

func integerValueSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": MinSafeInteger, "maximum": MaxSafeInteger}
}

func keyValueSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "maxLength": MaxStringBytes},
		integerValueSchema(), map[string]any{"type": "boolean"},
	}}
}

func rowValueSchema() map[string]any {
	return map[string]any{"anyOf": []any{
		map[string]any{"type": "string", "maxLength": MaxStringBytes},
		integerValueSchema(), map[string]any{"type": "number", "minimum": MinSafeInteger, "maximum": MaxSafeInteger},
		map[string]any{"type": "boolean"}, map[string]any{"type": "null"},
	}}
}

func keySchema() map[string]any {
	return map[string]any{
		"type": "object", "minProperties": 1, "maxProperties": MaxKeyColumns,
		"propertyNames": identifierSchema(), "additionalProperties": keyValueSchema(),
	}
}

func rowSchema() map[string]any {
	return map[string]any{
		"type": "object", "minProperties": 1, "maxProperties": MaxColumnsPerTable,
		"propertyNames": identifierSchema(), "additionalProperties": rowValueSchema(),
	}
}

func revisionSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "maximum": MaxRevision}
}

func storedRevisionSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": MaxRevision}
}

func cursorSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": MaxCursorBytes}
}

func canonicalSchemaDigest(schema map[string]any) string {
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal generated data.indexed schema: %v", err))
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		panic(fmt.Sprintf("digest generated data.indexed schema: %v", err))
	}
	return digest
}

func mustJSONNativeSchema(schema map[string]any) map[string]any {
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal generated data.indexed schema: %v", err))
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		panic(fmt.Sprintf("normalize generated data.indexed schema: %v", err))
	}
	return normalized
}

func identifierSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": identifierPattern.String()}
}

func portableNameSchema(maxLength int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": maxLength, "pattern": ".*\\S.*"}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringValues(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func anySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	default:
		return nil, false
	}
}

func anySliceOptional(value any) ([]any, bool) {
	if value == nil {
		return nil, true
	}
	return anySlice(value)
}

func stringSlice(value any) ([]string, bool) {
	raw, ok := anySlice(value)
	if !ok {
		if typed, ok := value.([]string); ok {
			return typed, true
		}
		return nil, false
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func integerValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}
