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

// TestSQLValuesCarryEveryStorageClass proves the tagged value model covers
// SQLite's five storage classes and nothing else, and that INTEGER travels as
// text rather than as a JSON number that cannot hold it.
func TestSQLValuesCarryEveryStorageClass(t *testing.T) {
	t.Parallel()
	variants, _ := sqlValue()["oneOf"].([]any)
	got := map[string]bool{}
	for _, variant := range variants {
		schema, _ := variant.(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		tag, _ := properties["type"].(map[string]any)
		values, _ := tag["enum"].([]any)
		if len(values) != 1 {
			t.Fatalf("a tagged value variant declares %d tags", len(values))
		}
		name, _ := values[0].(string)
		got[name] = true
	}
	for _, want := range []string{"null", "integer", "real", "text", "blob"} {
		if !got[want] {
			t.Fatalf("the tagged SQL value model is missing the %s storage class", want)
		}
	}
	if len(got) != 5 {
		t.Fatalf("the tagged SQL value model carries %d variants, want the five storage classes", len(got))
	}
	if got["boolean"] {
		t.Fatal("SQLite has no boolean storage class")
	}
	integer, _ := sqlDecimalInteger()["type"].(string)
	if integer != "string" {
		t.Fatalf("a 64-bit INTEGER travels as %q; a JSON number cannot carry 9223372036854775807", integer)
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
