package edgeformcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// interfaceFixtureCase builds one minimal Interface Definition carrying a
// single operation and a single fixture step, which is all the authoring rules
// of decision 0020 need in order to be shown failing.
func interfaceFixtureCase(step InterfaceFixtureStep) InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "test.contract", Version: "1.0.0",
		Title:     "Test contract",
		Semantics: InterfaceSemantics{Consistency: "eventual"},
		Operations: []InterfaceOperation{{
			Name:         "get",
			InputSchema:  operationObject([]string{"key"}, map[string]any{"key": stringSchema(1, 64)}),
			OutputSchema: operationObject(nil, map[string]any{}),
			Errors:       []string{"not_found"},
		}},
		Fixtures: []InterfaceFixture{{Name: "trace", Steps: []InterfaceFixtureStep{step}}},
	}
}

// TestInterfaceFixturesMustBePassable proves the authoring rule that keeps a
// contract from shipping a trace no conforming host could satisfy: a step must
// exercise a declared operation, and an expected failure must name an error
// that operation's closed vocabulary carries.
//
// The second half is the one that matters. A fixture expecting an error the
// operation does not declare describes a conforming implementation as failing,
// which is the same defect as a required conformance check no correct host can
// complete (spec/decisions/0020).
func TestInterfaceFixturesMustBePassable(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name     string
		step     InterfaceFixtureStep
		contains string
	}{
		{
			name:     "undeclared operation",
			step:     InterfaceFixtureStep{Operation: "put", Input: map[string]any{"key": "a"}},
			contains: "exercises undeclared operation",
		},
		{
			name: "error outside the closed vocabulary",
			step: InterfaceFixtureStep{
				Operation: "get", Input: map[string]any{"key": "a"}, ExpectedError: "backend_unavailable",
			},
			contains: "which operation get does not declare",
		},
		{
			name: "both an output and an error",
			step: InterfaceFixtureStep{
				Operation: "get", Input: map[string]any{"key": "a"},
				Expected: map[string]any{"value": "x"}, ExpectedError: "not_found",
			},
			contains: "a step has one outcome",
		},
	} {
		err := ValidateInterfaceDefinitions([]InterfaceDefinition{interfaceFixtureCase(testCase.step)})
		if err == nil {
			t.Fatalf("%s was accepted", testCase.name)
		}
		if !strings.Contains(err.Error(), testCase.contains) {
			t.Fatalf("%s said %q, which does not name %q", testCase.name, err, testCase.contains)
		}
	}
	passable := interfaceFixtureCase(InterfaceFixtureStep{
		Operation: "get", Input: map[string]any{"key": "a"}, ExpectedError: "not_found",
	})
	if err := ValidateInterfaceDefinitions([]InterfaceDefinition{passable}); err != nil {
		t.Fatalf("a passable trace was refused: %v", err)
	}
}

// TestEdgeKVFixturesDoNotRequireConvergence proves the correction decision 0020
// makes: the contract declares eventual consistency, so no deterministic
// fixture may require a write to be visible to the next read. A put-then-get
// trace is exactly what a correct eventually consistent store is allowed to
// fail, and it was in this contract's fixture set until now.
func TestEdgeKVFixturesDoNotRequireConvergence(t *testing.T) {
	t.Parallel()
	definition, err := interfaceDefinitionByName("edge.kv")
	if err != nil {
		t.Fatal(err)
	}
	if definition.Semantics.Consistency != "eventual" {
		t.Fatalf("edge.kv declares consistency %q", definition.Semantics.Consistency)
	}
	if len(definition.Fixtures) == 0 {
		t.Fatal("edge.kv must still prove what is provable with fixtures")
	}
	for _, fixture := range definition.Fixtures {
		wrote := map[string]bool{}
		for _, step := range fixture.Steps {
			key, _ := step.Input["key"].(string)
			switch step.Operation {
			case "put", "delete":
				wrote[key] = true
			case "get", "getWithMetadata":
				if wrote[key] {
					t.Fatalf(
						"edge.kv fixture %s reads key %q after writing it; an eventually consistent store may fail that",
						fixture.Name, key,
					)
				}
			}
		}
	}
}

// TestByteCarryingContractsShareOneEncodedShape proves the family carries bytes
// exactly one way, and that the structural ceiling on the encoding matches the
// declared byte limit rather than measuring string length against a byte count.
func TestByteCarryingContractsShareOneEncodedShape(t *testing.T) {
	t.Parallel()
	if queueMaxMessageBytes != 127000 {
		t.Fatalf("queueMaxMessageBytes = %d, want the Cloudflare portable minimum 127000", queueMaxMessageBytes)
	}
	for _, testCase := range []struct {
		contract string
		limit    string
		decoded  int
	}{
		{contract: "edge.kv", limit: "maxValueBytes", decoded: kvMaxValueBytes},
		{contract: "edge.queue", limit: "maxMessageBytes", decoded: queueMaxMessageBytes},
	} {
		definition, err := interfaceDefinitionByName(testCase.contract)
		if err != nil {
			t.Fatal(err)
		}
		if got := definition.Limits[testCase.limit]; got != int64(testCase.decoded) {
			t.Fatalf("%s %s is %d, want %d", testCase.contract, testCase.limit, got, testCase.decoded)
		}
		shape := encodedBytes(testCase.decoded)
		properties, _ := shape["properties"].(map[string]any)
		data, _ := properties["data"].(map[string]any)
		if got := data["maxLength"]; got != base64Length(testCase.decoded) {
			t.Fatalf("%s encoded ceiling is %v, want %d", testCase.contract, got, base64Length(testCase.decoded))
		}
		encoding, _ := properties["encoding"].(map[string]any)
		values, _ := encoding["enum"].([]any)
		if len(values) != 1 || values[0] != "base64" {
			t.Fatalf("%s encoding enum is %v", testCase.contract, values)
		}
	}
}

func TestQueueMessageLimitCoversThePooledRuntimeEnvelope(t *testing.T) {
	t.Parallel()
	const want = int64(127000)

	for _, testCase := range []struct {
		contract string
		limit    string
	}{
		{contract: "edge.queue", limit: "maxMessageBytes"},
		{contract: WorkerRuntimeInterfaceName, limit: "maxQueueMessageBytes"},
	} {
		definition, err := interfaceDefinitionByName(testCase.contract)
		if err != nil {
			t.Fatal(err)
		}
		if got := definition.Limits[testCase.limit]; got != want {
			t.Fatalf("%s %s = %d, want %d", testCase.contract, testCase.limit, got, want)
		}
	}

	queue, err := interfaceDefinitionByName("edge.queue")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range queue.Operations {
		var body map[string]any
		properties, _ := operation.InputSchema["properties"].(map[string]any)
		switch operation.Name {
		case "send":
			body, _ = properties["body"].(map[string]any)
		case "sendBatch":
			messages, _ := properties["messages"].(map[string]any)
			items, _ := messages["items"].(map[string]any)
			itemProperties, _ := items["properties"].(map[string]any)
			body, _ = itemProperties["body"].(map[string]any)
		default:
			continue
		}
		if got := encodedBodyMaxLength(body); got != base64Length(int(want)) {
			t.Fatalf("edge.queue %s body maxLength = %d, want %d", operation.Name, got, base64Length(int(want)))
		}
	}

	for _, testCase := range []struct {
		name  string
		limit string
	}{
		{name: "edge.queue", limit: "maxMessageBytes"},
		{name: WorkerRuntimeInterfaceName, limit: "maxQueueMessageBytes"},
	} {
		raw, err := os.ReadFile(filepath.Join(
			"..", "..", "interfaces", "candidates", "v1alpha1", testCase.name, "definition.json",
		))
		if err != nil {
			t.Fatal(err)
		}
		var candidate struct {
			Limits     map[string]int64 `json:"limits"`
			Operations []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"operations"`
		}
		if err := json.Unmarshal(raw, &candidate); err != nil {
			t.Fatal(err)
		}
		if got := candidate.Limits[testCase.limit]; got != want {
			t.Fatalf("generated %s %s = %d, want %d", testCase.name, testCase.limit, got, want)
		}
		checked := 0
		for _, operation := range candidate.Operations {
			var body map[string]any
			properties, _ := operation.InputSchema["properties"].(map[string]any)
			switch {
			case testCase.name == "edge.queue" && operation.Name == "send":
				body, _ = properties["body"].(map[string]any)
			case testCase.name == "edge.queue" && operation.Name == "sendBatch":
				messages, _ := properties["messages"].(map[string]any)
				items, _ := messages["items"].(map[string]any)
				itemProperties, _ := items["properties"].(map[string]any)
				body, _ = itemProperties["body"].(map[string]any)
			case testCase.name == WorkerRuntimeInterfaceName && operation.Name == "queue":
				messages, _ := properties["messages"].(map[string]any)
				items, _ := messages["items"].(map[string]any)
				itemProperties, _ := items["properties"].(map[string]any)
				body, _ = itemProperties["body"].(map[string]any)
			default:
				continue
			}
			if got := encodedBodyMaxLength(body); got != base64Length(int(want)) {
				t.Fatalf("generated %s %s body maxLength = %d, want %d", testCase.name, operation.Name, got, base64Length(int(want)))
			}
			checked++
		}
		wantChecked := 1
		if testCase.name == "edge.queue" {
			wantChecked = 2
		}
		if checked != wantChecked {
			t.Fatalf("generated %s checked %d binary body schemas, want %d", testCase.name, checked, wantChecked)
		}
	}

	runtime, err := interfaceDefinitionByName(WorkerRuntimeInterfaceName)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range runtime.Operations {
		if operation.Name != "queue" {
			continue
		}
		properties, _ := operation.InputSchema["properties"].(map[string]any)
		messages, _ := properties["messages"].(map[string]any)
		items, _ := messages["items"].(map[string]any)
		itemProperties, _ := items["properties"].(map[string]any)
		if got := encodedBodyMaxLength(itemProperties["body"].(map[string]any)); got != base64Length(int(want)) {
			t.Fatalf("worker.runtime queue body maxLength = %d, want %d", got, base64Length(int(want)))
		}
		return
	}
	t.Fatal("worker.runtime has no queue operation")
}

func TestPooledKeyBudgetsReserveTheCloudflareEnvelope(t *testing.T) {
	t.Parallel()
	const pooledPrefixBytes = 45
	if kvMaxKeyBytes != 512-pooledPrefixBytes {
		t.Fatalf("kvMaxKeyBytes = %d, want %d", kvMaxKeyBytes, 512-pooledPrefixBytes)
	}
	if objectsMaxKeyBytes != 1024-pooledPrefixBytes {
		t.Fatalf("objectsMaxKeyBytes = %d, want %d", objectsMaxKeyBytes, 1024-pooledPrefixBytes)
	}

	for _, testCase := range []struct {
		contract string
		limit    string
		want     int
	}{
		{contract: "edge.kv", limit: "maxKeyBytes", want: kvMaxKeyBytes},
		{contract: "edge.objects", limit: "maxKeyBytes", want: objectsMaxKeyBytes},
	} {
		definition, err := interfaceDefinitionByName(testCase.contract)
		if err != nil {
			t.Fatal(err)
		}
		if got := definition.Limits[testCase.limit]; got != int64(testCase.want) {
			t.Fatalf("%s %s = %d, want %d", testCase.contract, testCase.limit, got, testCase.want)
		}
		checked := 0
		for _, operation := range definition.Operations {
			checked += assertKeySchemaBudget(t, operation.InputSchema, testCase.contract, operation.Name, testCase.want)
		}
		if checked == 0 {
			t.Fatalf("%s has no key schema to check", testCase.contract)
		}

		raw, err := os.ReadFile(filepath.Join(
			"..", "..", "interfaces", "candidates", "v1alpha1", testCase.contract, "definition.json",
		))
		if err != nil {
			t.Fatal(err)
		}
		var candidate struct {
			Limits     map[string]int64 `json:"limits"`
			Operations []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"operations"`
		}
		if err := json.Unmarshal(raw, &candidate); err != nil {
			t.Fatal(err)
		}
		if got := candidate.Limits[testCase.limit]; got != int64(testCase.want) {
			t.Fatalf("generated %s %s = %d, want %d", testCase.contract, testCase.limit, got, testCase.want)
		}
		generatedChecked := 0
		for _, operation := range candidate.Operations {
			generatedChecked += assertKeySchemaBudget(t, operation.InputSchema, testCase.contract, operation.Name, testCase.want)
		}
		if generatedChecked != checked {
			t.Fatalf("generated %s checked %d key schemas, want %d", testCase.contract, generatedChecked, checked)
		}
	}
}

func assertKeySchemaBudget(t *testing.T, schema map[string]any, contract, operation string, want int) int {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	checked := 0
	for _, propertyName := range []string{"key", "prefix"} {
		if raw, ok := properties[propertyName].(map[string]any); ok {
			maxLength, ok := raw["maxLength"]
			if !ok {
				t.Fatalf("%s %s %s schema has no maxLength", contract, operation, propertyName)
			}
			var got int
			switch value := maxLength.(type) {
			case int:
				got = value
			case float64:
				got = int(value)
			default:
				t.Fatalf("%s %s %s schema maxLength has type %T", contract, operation, propertyName, maxLength)
			}
			if got != want {
				t.Fatalf("%s %s %s maxLength = %d, want %d", contract, operation, propertyName, got, want)
			}
			checked++
		}
	}
	for _, raw := range properties {
		child, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		checked += assertKeySchemaBudget(t, child, contract, operation, want)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		checked += assertKeySchemaBudget(t, items, contract, operation, want)
	}
	return checked
}

func encodedBodyMaxLength(schema map[string]any) int {
	properties, _ := schema["properties"].(map[string]any)
	data, _ := properties["data"].(map[string]any)
	switch maxLength := data["maxLength"].(type) {
	case int:
		return maxLength
	case float64:
		return int(maxLength)
	default:
		return 0
	}
}

// TestSQLValuesUseThePortableWireUnion proves edge.sql has one JSON-shaped
// value model rather than exposing SQLite's private INTEGER/REAL storage-class
// distinction. Binary values reuse the family's canonical encoded-bytes shape;
// boolean, bigint, and the withdrawn tagged objects have no representation.
func TestSQLValuesUseThePortableWireUnion(t *testing.T) {
	t.Parallel()
	variants, _ := sqlValue()["oneOf"].([]any)
	if len(variants) != 4 {
		t.Fatalf("edge.sql value union has %d variants, want null, number, string, encoded bytes", len(variants))
	}
	got := map[string]map[string]any{}
	for _, variant := range variants {
		schema, _ := variant.(map[string]any)
		kind, _ := schema["type"].(string)
		if kind == "" {
			t.Fatalf("edge.sql value variant has no type: %#v", schema)
		}
		if _, duplicate := got[kind]; duplicate {
			t.Fatalf("edge.sql value union declares %s twice", kind)
		}
		got[kind] = schema
	}
	for _, want := range []string{"null", "number", "string", "object"} {
		if _, present := got[want]; !present {
			t.Fatalf("edge.sql value union is missing %s", want)
		}
	}
	number := got["number"]
	if number["minimum"] != -float64(9007199254740991) || number["maximum"] != float64(9007199254740991) {
		t.Fatalf("edge.sql number range is %v..%v, want +/- Number.MAX_SAFE_INTEGER", number["minimum"], number["maximum"])
	}
	text := got["string"]
	if text["maxLength"] != 1000000 {
		t.Fatalf("edge.sql text structural ceiling is %v, want 1000000", text["maxLength"])
	}
	blob := got["object"]
	if max := encodedBodyMaxLength(blob); max != base64Length(1000000) {
		t.Fatalf("edge.sql BLOB encoded ceiling is %d, want %d", max, base64Length(1000000))
	}
	properties, _ := blob["properties"].(map[string]any)
	if _, tagged := properties["type"]; tagged {
		t.Fatal("edge.sql BLOB still exposes the withdrawn tagged-value member")
	}
}

// TestSQLContractIsBoundedAndRollbackSafe fixes the operation-level contract:
// execute is the effectful one-statement path, query earns idempotency by
// executing and materializing inside an always-rolled-back transaction, and a
// transaction commits only after every result is materialized and validated.
func TestSQLContractIsBoundedAndRollbackSafe(t *testing.T) {
	t.Parallel()
	definition, err := interfaceDefinitionByName("edge.sql")
	if err != nil {
		t.Fatal(err)
	}
	wantLimits := map[string]int64{
		"maxStatementBytes": 100000, "maxBoundParameters": 100,
		"maxStatementsPerTransaction": 100, "maxRowsPerStatement": 10000,
		"maxColumnsPerRow": 100, "maxColumnNameBytes": 128,
		"maxTextBytesPerValue": 1000000, "maxBlobBytesPerValue": 1000000,
		"maxRowBytes": 2000000, "maxResultBytesPerCall": 8388608,
	}
	if len(definition.Limits) != len(wantLimits) {
		t.Fatalf("edge.sql declares %d limits, want %d: %#v", len(definition.Limits), len(wantLimits), definition.Limits)
	}
	for name, want := range wantLimits {
		if got := definition.Limits[name]; got != want {
			t.Fatalf("edge.sql %s = %d, want %d", name, got, want)
		}
	}

	operations := map[string]InterfaceOperation{}
	for _, operation := range definition.Operations {
		operations[operation.Name] = operation
		foundNumericError := false
		for _, code := range operation.Errors {
			if code == "numeric_out_of_range" {
				foundNumericError = true
			}
		}
		if !foundNumericError {
			t.Fatalf("edge.sql %s does not declare numeric_out_of_range", operation.Name)
		}
	}
	if operations["execute"].Idempotent {
		t.Fatal("edge.sql execute must remain effectful and non-idempotent")
	}
	if !operations["query"].Idempotent {
		t.Fatal("edge.sql query must be idempotent")
	}
	for _, phrase := range []string{"rollback-only transaction", "always rolls back", "without pre-classifying"} {
		if !strings.Contains(operations["query"].Description, phrase) {
			t.Fatalf("edge.sql query description does not state %q", phrase)
		}
	}
	queryProperties, _ := operations["query"].OutputSchema["properties"].(map[string]any)
	queryRowsWritten, _ := queryProperties["rowsWritten"].(map[string]any)
	if got := queryRowsWritten["const"]; got != 0 {
		t.Fatalf("edge.sql query rowsWritten const = %v, want 0", got)
	}
	if !strings.Contains(operations["transaction"].Description, "materialized and validated before commit") {
		t.Fatal("edge.sql transaction does not bind materialization and validation before commit")
	}
	transactionInput, _ := operations["transaction"].InputSchema["properties"].(map[string]any)
	statements, _ := transactionInput["statements"].(map[string]any)
	transactionOutput, _ := operations["transaction"].OutputSchema["properties"].(map[string]any)
	results, _ := transactionOutput["results"].(map[string]any)
	for name, schema := range map[string]map[string]any{"statements": statements, "results": results} {
		if schema["minItems"] != 1 || schema["maxItems"] != sqlMaxStatementsPerTransaction {
			t.Fatalf("edge.sql transaction %s bounds = %v..%v, want 1..%d", name, schema["minItems"], schema["maxItems"], sqlMaxStatementsPerTransaction)
		}
	}

	result := sqlStatementResult()
	properties, _ := result["properties"].(map[string]any)
	if len(properties) != 2 || properties["rows"] == nil || properties["rowsWritten"] == nil {
		t.Fatalf("edge.sql statement result properties are %#v, want exactly rows and rowsWritten", properties)
	}
	if _, legacy := properties["lastInsertRowId"]; legacy {
		t.Fatal("edge.sql still exposes lastInsertRowId")
	}
	fixtureJSON, err := json.Marshal(definition.Fixtures)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"lastInsertRowId", `\"type\":\"integer\"`, `\"type\":\"real\"`, `\"type\":\"text\"`, `\"type\":\"blob\"`} {
		if strings.Contains(string(fixtureJSON), legacy) {
			t.Fatalf("edge.sql fixtures still contain withdrawn value shape %q", legacy)
		}
	}
}

func TestSQLColumnLimitIsThePortableCandidateMinimum(t *testing.T) {
	t.Parallel()
	const want = int64(100)

	definition, err := interfaceDefinitionByName("edge.sql")
	if err != nil {
		t.Fatal(err)
	}
	if got := definition.Limits["maxColumnsPerRow"]; got != want {
		t.Fatalf("edge.sql maxColumnsPerRow = %d, want %d", got, want)
	}
	rows := sqlRows()
	items, ok := rows["items"].(map[string]any)
	if !ok {
		t.Fatal("edge.sql rows schema has no item object")
	}
	if got, ok := items["maxProperties"].(int); !ok || int64(got) != want {
		t.Fatalf("edge.sql row maxProperties = %v, want %d", items["maxProperties"], want)
	}

	raw, err := os.ReadFile(filepath.Join(
		"..", "..", "interfaces", "candidates", "v1alpha1", "edge.sql", "definition.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var candidate struct {
		Limits     map[string]int64 `json:"limits"`
		Operations []struct {
			OutputSchema map[string]any `json:"outputSchema"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(raw, &candidate); err != nil {
		t.Fatal(err)
	}
	if got := candidate.Limits["maxColumnsPerRow"]; got != want {
		t.Fatalf("generated edge.sql maxColumnsPerRow = %d, want %d", got, want)
	}
	checked := 0
	for _, operation := range candidate.Operations {
		properties, _ := operation.OutputSchema["properties"].(map[string]any)
		rowsSchema, ok := properties["rows"].(map[string]any)
		if !ok {
			continue
		}
		rowItems, _ := rowsSchema["items"].(map[string]any)
		got, ok := rowItems["maxProperties"].(float64)
		if !ok || int64(got) != want {
			t.Fatalf("generated edge.sql operation row maxProperties = %v, want %d", rowItems["maxProperties"], want)
		}
		checked++
	}
	if checked != 2 {
		t.Fatalf("generated edge.sql checked %d direct row result schemas, want 2", checked)
	}
}
