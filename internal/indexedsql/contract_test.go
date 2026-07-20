package indexedsql

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCanonicalDesiredPassesSemanticValidation(t *testing.T) {
	t.Parallel()
	if err := ValidateDesired(canonicalDesired()); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticValidationRejectsUnsafeKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "unknown primary key",
			mutate: func(value map[string]any) {
				firstTable(value)["primaryKey"] = []any{"missing"}
			},
			want: "unknown column",
		},
		{
			name: "nullable primary key",
			mutate: func(value map[string]any) {
				firstColumn(value)["nullable"] = true
			},
			want: "must be non-null",
		},
		{
			name: "number primary key",
			mutate: func(value map[string]any) {
				firstColumn(value)["type"] = "number"
			},
			want: "not indexable",
		},
		{
			name: "duplicate table",
			mutate: func(value map[string]any) {
				table := clone(firstTable(value))
				value["tables"] = append(value["tables"].([]any), table)
			},
			want: "duplicate table",
		},
		{
			name: "duplicate index",
			mutate: func(value map[string]any) {
				table := firstTable(value)
				indexes := table["indexes"].([]any)
				table["indexes"] = append(indexes, clone(indexes[0].(map[string]any)))
			},
			want: "duplicate index",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := canonicalDesired()
			tt.mutate(value)
			err := ValidateDesired(value)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRequestSchemaAcceptsOnlyBoundedOperations(t *testing.T) {
	t.Parallel()
	schema := compile(t, RequestSchema())
	positive := []any{
		map[string]any{"operation": "get", "table": "users", "key": map[string]any{"id": "u1"}},
		map[string]any{"operation": "get_unique", "table": "users", "index": "by_email", "key": map[string]any{"email": "a@example.com"}},
		map[string]any{"operation": "page", "table": "users", "index": "by_tenant_created", "prefix": map[string]any{"tenant_id": "t1"}, "range": map[string]any{"column": "created_at", "gte": 1}, "limit": 100},
		map[string]any{"operation": "put", "table": "users", "row": map[string]any{"id": "u1", "active": true}},
		map[string]any{"operation": "delete", "table": "users", "key": map[string]any{"id": "u1"}, "expectedRevision": 1},
		map[string]any{"operation": "batch", "mutations": []any{
			map[string]any{"operation": "put", "table": "users", "row": map[string]any{"id": "u1"}},
			map[string]any{"operation": "delete", "table": "users", "key": map[string]any{"id": "u2"}},
		}},
	}
	for index, value := range positive {
		if err := schema.Validate(value); err != nil {
			t.Fatalf("positive[%d]: %v", index, err)
		}
	}

	negative := []any{
		map[string]any{"operation": "query", "sql": "select * from users"},
		map[string]any{"operation": "page", "table": "users", "index": "by_created", "prefix": map[string]any{}, "order": "desc"},
		map[string]any{"operation": "page", "table": "users", "index": "by_created", "prefix": map[string]any{}, "offset": 100},
		map[string]any{"operation": "page", "table": "users", "index": "by_created", "prefix": map[string]any{}, "limit": MaxPageSize + 1},
		map[string]any{"operation": "page", "table": "users", "index": "by_created", "prefix": map[string]any{}, "range": map[string]any{"column": "created_at", "gt": 1, "gte": 1}},
		map[string]any{"operation": "batch", "mutations": repeatedMutations(MaxBatchMutations + 1)},
		map[string]any{"operation": "get", "table": "users", "key": map[string]any{"score": 1.5}},
		map[string]any{"operation": "delete", "table": "users", "key": map[string]any{"id": "u1"}, "expectedRevision": "1"},
		map[string]any{"operation": "put", "table": "users", "row": map[string]any{"id": "u1"}, "expectedRevision": MaxRevision + 1},
		map[string]any{"operation": "page", "table": "users", "index": "by_email", "prefix": map[string]any{}, "cursor": strings.Repeat("c", MaxCursorBytes+1)},
		map[string]any{"operation": "get", "table": "users", "key": map[string]any{"sequence": MaxSafeInteger + 1}},
		map[string]any{"operation": "put", "table": "users", "row": map[string]any{"id": "u1", "sequence": MinSafeInteger - 1}},
	}
	for index, value := range negative {
		if err := schema.Validate(value); err == nil {
			t.Fatalf("negative[%d] unexpectedly passed: %#v", index, value)
		}
	}
}

func TestResponseSchemaAcceptsOnlyBoundedOperationResultsAndConflicts(t *testing.T) {
	t.Parallel()
	schema := compile(t, ResponseSchema())
	item := map[string]any{
		"row": map[string]any{"id": "u1", "active": true, "score": nil}, "revision": 1,
	}
	positive := []any{
		map[string]any{"operation": "get", "item": nil},
		map[string]any{"operation": "get", "item": item},
		map[string]any{"operation": "get_unique", "item": item},
		map[string]any{"operation": "page", "items": []any{item}, "nextCursor": "opaque-cursor"},
		map[string]any{"operation": "page", "items": []any{}, "nextCursor": nil},
		map[string]any{"operation": "put", "item": item},
		map[string]any{"operation": "delete", "deleted": true},
		map[string]any{"operation": "batch", "results": []any{
			map[string]any{"operation": "put", "item": item},
			map[string]any{"operation": "delete", "deleted": false},
		}},
		map[string]any{"operation": "delete", "conflict": map[string]any{
			"reason": "revision_conflict", "table": "users", "key": map[string]any{"id": "u1"},
		}},
		map[string]any{"operation": "batch", "conflict": map[string]any{
			"reason": "unique_conflict", "table": "users", "index": "by_email",
		}},
	}
	for index, value := range positive {
		if err := schema.Validate(value); err != nil {
			t.Fatalf("positive[%d]: %v", index, err)
		}
	}

	tooManyItems := make([]any, MaxPageSize+1)
	for index := range tooManyItems {
		tooManyItems[index] = item
	}
	tooManyResults := make([]any, MaxBatchMutations+1)
	for index := range tooManyResults {
		tooManyResults[index] = map[string]any{"operation": "delete", "deleted": true}
	}
	negative := []any{
		map[string]any{"found": false},
		map[string]any{"operation": "get"},
		map[string]any{"operation": "get", "item": map[string]any{"row": map[string]any{"id": "u1"}}},
		map[string]any{"operation": "page", "items": []any{}},
		map[string]any{"operation": "page", "items": tooManyItems, "nextCursor": nil},
		map[string]any{"operation": "page", "items": []any{}, "nextCursor": strings.Repeat("c", MaxCursorBytes+1)},
		map[string]any{"operation": "put", "item": map[string]any{"row": map[string]any{"id": "u1"}, "revision": 0}},
		map[string]any{"operation": "put", "item": map[string]any{"row": map[string]any{"id": "u1"}, "revision": MaxRevision + 1}},
		map[string]any{"operation": "delete", "deleted": true, "revision": 1},
		map[string]any{"operation": "batch", "results": []any{}},
		map[string]any{"operation": "batch", "results": tooManyResults},
		map[string]any{"operation": "get", "conflict": map[string]any{
			"reason": "revision_conflict", "table": "users", "key": map[string]any{"id": "u1"},
		}},
		map[string]any{"operation": "delete", "conflict": map[string]any{
			"reason": "unique_conflict", "table": "users", "index": "by_email",
		}},
		map[string]any{"operation": "put", "conflict": map[string]any{
			"reason": "revision_conflict", "table": "users", "key": map[string]any{"id": 1.5},
		}},
	}
	for index, value := range negative {
		if err := schema.Validate(value); err == nil {
			t.Fatalf("negative[%d] unexpectedly passed: %#v", index, value)
		}
	}
}

func TestInterfaceDescriptorPinsPortableIdentityAndLimits(t *testing.T) {
	t.Parallel()
	descriptor := InterfaceDescriptor()
	if descriptor.Name != InterfaceName || descriptor.Version != InterfaceVersion || !descriptor.Required {
		t.Fatalf("descriptor identity = %#v", descriptor)
	}
	if descriptor.Document["method"] != OperationMethod || descriptor.Document["path"] != OperationPath {
		t.Fatalf("operation endpoint drift: %#v", descriptor.Document)
	}
	if descriptor.ResourceURIInput != "resource_uri" || len(descriptor.Inputs) != 6 || descriptor.Inputs[2].Pointer != "/generation" || descriptor.Inputs[3].Pointer != "/schemaVersion" || descriptor.Inputs[4].Pointer != "/tables" || descriptor.Inputs[5].Source != "resource_uri" {
		t.Fatalf("descriptor inputs = %#v", descriptor.Inputs)
	}
	if err := compile(t, descriptor.DocumentSchema).Validate(descriptor.Document); err != nil {
		t.Fatal(err)
	}
	limits := descriptor.Document["limits"].(map[string]any)
	if limits["cursorBytes"] != MaxCursorBytes || limits["cursorTtlSeconds"] != CursorTTLSeconds || limits["maxRevision"] != MaxRevision || limits["numericMinimum"] != MinSafeInteger || limits["numericMaximum"] != MaxSafeInteger || limits["requestBytes"] != MaxRequestBytes || limits["resultBytes"] != MaxResultBytes {
		t.Fatalf("descriptor limits drifted: %#v", limits)
	}
	schemas := descriptor.Document["schemas"].(map[string]any)
	request := schemas["request"].(map[string]any)
	response := schemas["response"].(map[string]any)
	if request["packagePath"] != RequestSchemaPath || request["schemaDigest"] != canonicalSchemaDigest(RequestSchema()) || response["packagePath"] != ResponseSchemaPath || response["schemaDigest"] != canonicalSchemaDigest(ResponseSchema()) {
		t.Fatalf("descriptor schema references drifted: %#v", schemas)
	}
	for name, want := range map[string]map[string]any{"request": RequestSchema(), "response": ResponseSchema()} {
		entry := schemas[name].(map[string]any)
		got, ok := entry["schema"].(map[string]any)
		if !ok || canonicalSchemaDigest(got) != canonicalSchemaDigest(want) {
			t.Fatalf("descriptor inline %s schema drifted", name)
		}
	}
	responses := descriptor.Document["responses"].(map[string]any)
	if responses["successStatus"] != 200 || responses["conflictStatus"] != 409 {
		t.Fatalf("descriptor response statuses drifted: %#v", responses)
	}
	ordering := descriptor.Document["ordering"].(map[string]any)
	if ordering["direction"] != "asc" || ordering["string"] != "unsigned-utf8-byte-lexicographic" || ordering["composite"] != "declared-index-columns-then-missing-primary-key-columns" {
		t.Fatalf("descriptor ordering drifted: %#v", ordering)
	}
	cursor := descriptor.Document["cursor"].(map[string]any)
	if cursor["mode"] != "exclusive-live-keyset" || cursor["integrity"] != "tamper-evident" || cursor["concurrentMutations"] != "live-no-snapshot" {
		t.Fatalf("descriptor cursor semantics drifted: %#v", cursor)
	}
}

func compile(t *testing.T, value map[string]any) *jsonschema.Schema {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("urn:takoform:test", document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("urn:takoform:test")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func canonicalDesired() map[string]any {
	return map[string]any{
		"name": "main", "schemaVersion": 1,
		"tables": []any{map[string]any{
			"name": "users",
			"columns": []any{
				map[string]any{"name": "id", "type": "string", "nullable": false},
				map[string]any{"name": "email", "type": "string", "nullable": false},
				map[string]any{"name": "created_at", "type": "integer", "nullable": false},
			},
			"primaryKey": []any{"id"},
			"indexes":    []any{map[string]any{"name": "by_email", "columns": []any{"email"}, "unique": true}},
		}},
	}
}

func firstTable(value map[string]any) map[string]any {
	return value["tables"].([]any)[0].(map[string]any)
}

func firstColumn(value map[string]any) map[string]any {
	return firstTable(value)["columns"].([]any)[0].(map[string]any)
}

func clone(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func repeatedMutations(count int) []any {
	result := make([]any, count)
	for index := range result {
		result[index] = map[string]any{"operation": "delete", "table": "users", "key": map[string]any{"id": index}}
	}
	return result
}
