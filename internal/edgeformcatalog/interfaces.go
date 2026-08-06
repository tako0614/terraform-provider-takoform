package edgeformcatalog

// The five exact Interface contracts of the Edge Platform Family (decision
// 0010). An Interface Definition fixes operations with typed input/output
// schemas, a closed error vocabulary, consistency and pagination semantics,
// portable minimum limits, and data-only behavior fixtures. Its RFC 8785
// digest is the identity every Form and Binding references.

const (
	// InterfaceAPIVersion is the fixed group of exact Interface contracts.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	draft2020           = "https://json-schema.org/draft/2020-12/schema"
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

// operationObject builds one closed Draft 2020-12 operation payload schema.
func operationObject(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"$schema":              draft2020,
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(minLength, maxLength int) map[string]any {
	schema := map[string]any{"type": "string", "maxLength": maxLength}
	if minLength > 0 {
		schema["minLength"] = minLength
	}
	return schema
}

func closedStringMap(maxProperties int) map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        maxProperties,
		"propertyNames":        map[string]any{"type": "string", "maxLength": 256},
		"additionalProperties": map[string]any{"type": "string", "maxLength": 8192},
	}
}

func sqlParams() map[string]any {
	return map[string]any{
		"type":     "array",
		"maxItems": 100,
		"items":    map[string]any{"type": []any{"boolean", "null", "number", "string"}},
	}
}

func sqlStatement() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"sql"},
		"properties": map[string]any{
			"sql":    stringSchema(1, 100000),
			"params": sqlParams(),
		},
	}
}

// InterfaceDefinitions lists the Edge Platform Family interface catalog in a
// stable order.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{
		edgeKVInterface(),
		edgeObjectsInterface(),
		edgeSQLInterface(),
		edgeQueueInterface(),
		workerServiceInterface(),
	}
}

func edgeKVInterface() InterfaceDefinition {
	key := stringSchema(1, 512)
	metadata := closedStringMap(64)
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.kv", Version: "1.0.0",
		Title: "Edge key/value namespace",
		Description: "Globally replicated key/value reads and writes with eventual consistency: " +
			"a read after a write may return the previous value until replication converges. " +
			"Keys are UTF-8 strings; values are opaque text. Deleting an absent key succeeds.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Pagination: "cursor"},
		Limits:    map[string]int64{"maxKeyBytes": 512, "maxValueBytes": 26214400, "maxMetadataBytes": 1024},
		Operations: []InterfaceOperation{
			{
				Name:        "get",
				Description: "Read one value by key. Absent keys fail with not_found.",
				InputSchema: operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"value"}, map[string]any{
					"value": stringSchema(0, 26214400),
				}),
				Errors:     []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name:        "getWithMetadata",
				Description: "Read one value and its non-secret metadata document by key.",
				InputSchema: operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"value"}, map[string]any{
					"value":    stringSchema(0, 26214400),
					"metadata": metadata,
				}),
				Errors:     []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "put",
				Description: "Write one value, replacing any previous value for the key. " +
					"An optional expiration TTL removes the entry after at least that many seconds.",
				InputSchema: operationObject([]string{"key", "value"}, map[string]any{
					"key":                  key,
					"value":                stringSchema(0, 26214400),
					"expirationTtlSeconds": map[string]any{"type": "integer", "minimum": 60, "maximum": 315360000},
					"metadata":             metadata,
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "value_too_large", "backend_unavailable"},
			},
			{
				Name:         "delete",
				Description:  "Delete one key. Deleting an absent key succeeds.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name:        "list",
				Description: "List key names in lexicographic order, optionally under a prefix, one cursor page at a time.",
				InputSchema: operationObject(nil, map[string]any{
					"prefix": stringSchema(0, 512),
					"cursor": stringSchema(1, 4096),
					"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 1000},
				}),
				OutputSchema: operationObject([]string{"keys", "listComplete"}, map[string]any{
					"keys": map[string]any{
						"type":     "array",
						"maxItems": 1000,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"name"},
							"properties":           map[string]any{"name": key},
						},
					},
					"listComplete": map[string]any{"type": "boolean"},
					"cursor":       stringSchema(1, 4096),
				}),
				Errors:     []string{"invalid_cursor", "backend_unavailable"},
				Idempotent: true,
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "put-then-get",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{"key": "greeting", "value": "hello"}},
					{Operation: "get", Input: map[string]any{"key": "greeting"}, Expected: map[string]any{"value": "hello"}},
				},
			},
			{
				Name: "delete-then-get-not-found",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{"key": "ephemeral", "value": "x"}},
					{Operation: "delete", Input: map[string]any{"key": "ephemeral"}},
					{Operation: "get", Input: map[string]any{"key": "ephemeral"}, ExpectedError: "not_found"},
				},
			},
			{
				Name: "list-prefix",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{"key": "config/a", "value": "1"}},
					{Operation: "put", Input: map[string]any{"key": "config/b", "value": "2"}},
					{Operation: "put", Input: map[string]any{"key": "other/c", "value": "3"}},
					{Operation: "list", Input: map[string]any{"prefix": "config/"}, Expected: map[string]any{
						"keys":         []any{map[string]any{"name": "config/a"}, map[string]any{"name": "config/b"}},
						"listComplete": true,
					}},
				},
			},
		},
	}
}

func edgeObjectsInterface() InterfaceDefinition {
	key := stringSchema(1, 1024)
	etag := stringSchema(1, 256)
	contentType := stringSchema(1, 256)
	size := map[string]any{"type": "integer", "minimum": 0, "maximum": 5368709120}
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.objects", Version: "1.0.0",
		Title: "Object bucket",
		Description: "Flat-namespace object reads and writes with read-after-write consistency: " +
			"a get or head after a successful put observes that put. Writes to one key are last-writer-wins. " +
			"Conditional requests fence on the strong etag returned by put and head; range reads return a " +
			"contiguous byte subrange of one object version.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write", Pagination: "cursor"},
		Limits:    map[string]int64{"maxKeyBytes": 1024, "maxObjectBytes": 5368709120},
		Operations: []InterfaceOperation{
			{
				Name:        "get",
				Description: "Read one object body and its content type by key.",
				InputSchema: operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"body", "etag", "size"}, map[string]any{
					"body":        map[string]any{"type": "string"},
					"contentType": contentType,
					"etag":        etag,
					"size":        size,
				}),
				Errors:     []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name:        "head",
				Description: "Read one object's size, etag, and content type without its body.",
				InputSchema: operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"etag", "size"}, map[string]any{
					"contentType": contentType,
					"etag":        etag,
					"size":        size,
				}),
				Errors:     []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "put",
				Description: "Write one object, replacing any previous object at the key, and return its strong etag. " +
					"A conditional put carries the expected etag and fails with precondition_failed on mismatch.",
				InputSchema: operationObject([]string{"body", "key"}, map[string]any{
					"key":          key,
					"body":         map[string]any{"type": "string"},
					"contentType":  contentType,
					"expectedEtag": etag,
				}),
				OutputSchema: operationObject([]string{"etag"}, map[string]any{"etag": etag}),
				Errors:       []string{"invalid_key", "value_too_large", "precondition_failed", "backend_unavailable"},
			},
			{
				Name:         "delete",
				Description:  "Delete one object. Deleting an absent key succeeds.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name:        "list",
				Description: "List objects in lexicographic key order, optionally under a prefix, one cursor page at a time.",
				InputSchema: operationObject(nil, map[string]any{
					"prefix": stringSchema(0, 1024),
					"cursor": stringSchema(1, 4096),
					"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 1000},
				}),
				OutputSchema: operationObject([]string{"objects", "truncated"}, map[string]any{
					"objects": map[string]any{
						"type":     "array",
						"maxItems": 1000,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"etag", "key", "size"},
							"properties":           map[string]any{"key": key, "etag": etag, "size": size},
						},
					},
					"truncated": map[string]any{"type": "boolean"},
					"cursor":    stringSchema(1, 4096),
				}),
				Errors:     []string{"invalid_cursor", "backend_unavailable"},
				Idempotent: true,
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "put-then-get",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{"key": "reports/summary.txt", "body": "total: 3", "contentType": "text/plain"}},
					{Operation: "get", Input: map[string]any{"key": "reports/summary.txt"}, Expected: map[string]any{"body": "total: 3"}},
				},
			},
			{
				Name: "delete-then-head-not-found",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{"key": "tmp/scratch.txt", "body": "x"}},
					{Operation: "delete", Input: map[string]any{"key": "tmp/scratch.txt"}},
					{Operation: "head", Input: map[string]any{"key": "tmp/scratch.txt"}, ExpectedError: "not_found"},
				},
			},
		},
	}
}

func edgeSQLInterface() InterfaceDefinition {
	rows := map[string]any{
		"type":     "array",
		"maxItems": 10000,
		"items": map[string]any{
			"type":                 "object",
			"maxProperties":        256,
			"propertyNames":        map[string]any{"type": "string", "maxLength": 128},
			"additionalProperties": map[string]any{"type": []any{"boolean", "null", "number", "string"}},
		},
	}
	writeResult := operationObject([]string{"rowsWritten"}, map[string]any{
		"rowsWritten":     map[string]any{"type": "integer", "minimum": 0},
		"lastInsertRowId": map[string]any{"type": "integer"},
	})
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.sql", Version: "1.0.0",
		Title: "Embedded SQLite database",
		Description: "SQL statements executed against one embedded SQLite database with serializable " +
			"transactions: a transaction observes a single consistent snapshot and its statements apply " +
			"atomically or not at all. Concurrent writers may fail with busy and must retry.",
		Semantics: InterfaceSemantics{Consistency: "serializable"},
		Limits:    map[string]int64{"maxStatementBytes": 100000, "maxBoundParameters": 100},
		Operations: []InterfaceOperation{
			{
				Name:         "execute",
				Description:  "Run one write statement with bound parameters and return the written row count.",
				InputSchema:  operationObject([]string{"sql"}, map[string]any{"sql": stringSchema(1, 100000), "params": sqlParams()}),
				OutputSchema: writeResult,
				Errors:       []string{"sql_error", "busy", "backend_unavailable"},
			},
			{
				Name:        "query",
				Description: "Run one read statement with bound parameters and return its rows.",
				InputSchema: operationObject([]string{"sql"}, map[string]any{"sql": stringSchema(1, 100000), "params": sqlParams()}),
				OutputSchema: operationObject([]string{"rows"}, map[string]any{
					"rows": rows,
				}),
				Errors:     []string{"sql_error", "busy", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name:        "transaction",
				Description: "Apply an ordered statement list atomically under serializable isolation.",
				InputSchema: operationObject([]string{"statements"}, map[string]any{
					"statements": map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": sqlStatement()},
				}),
				OutputSchema: operationObject([]string{"results"}, map[string]any{
					"results": map[string]any{"type": "array", "maxItems": 100, "items": writeResult},
				}),
				Errors: []string{"sql_error", "busy", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "create-insert-select",
				Steps: []InterfaceFixtureStep{
					{Operation: "execute", Input: map[string]any{"sql": "CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL)"}},
					{Operation: "execute", Input: map[string]any{"sql": "INSERT INTO notes (body) VALUES (?1)", "params": []any{"first note"}}, Expected: map[string]any{"rowsWritten": 1}},
					{Operation: "query", Input: map[string]any{"sql": "SELECT body FROM notes ORDER BY id"}, Expected: map[string]any{"rows": []any{map[string]any{"body": "first note"}}}},
				},
			},
		},
	}
}

func edgeQueueInterface() InterfaceDefinition {
	body := stringSchema(0, 131072)
	delay := map[string]any{"type": "integer", "minimum": 0, "maximum": 43200}
	messageID := stringSchema(1, 256)
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.queue", Version: "1.0.0",
		Title: "At-least-once queue producer",
		Description: "Asynchronous message submission with at-least-once delivery and no ordering guarantee: " +
			"an accepted message is delivered to the consumer one or more times, possibly out of send order. " +
			"Consumers must be idempotent against duplicates.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Delivery: "at_least_once", Ordering: "none"},
		Limits:    map[string]int64{"maxMessageBytes": 131072, "maxBatchMessages": 100},
		Operations: []InterfaceOperation{
			{
				Name:        "send",
				Description: "Submit one message, optionally delayed before it becomes deliverable.",
				InputSchema: operationObject([]string{"body"}, map[string]any{
					"body":         body,
					"delaySeconds": delay,
				}),
				OutputSchema: operationObject([]string{"messageId"}, map[string]any{"messageId": messageID}),
				Errors:       []string{"message_too_large", "backend_unavailable"},
			},
			{
				Name:        "sendBatch",
				Description: "Submit an ordered batch of messages; acceptance is all-or-nothing.",
				InputSchema: operationObject([]string{"messages"}, map[string]any{
					"messages": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 100,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"body"},
							"properties":           map[string]any{"body": body, "delaySeconds": delay},
						},
					},
				}),
				OutputSchema: operationObject([]string{"messageIds"}, map[string]any{
					"messageIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": messageID},
				}),
				Errors: []string{"message_too_large", "batch_too_large", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "send-accepted",
				Steps: []InterfaceFixtureStep{
					{Operation: "send", Input: map[string]any{"body": "{\"event\":\"user.created\"}"}},
				},
			},
		},
	}
}

func workerServiceInterface() InterfaceDefinition {
	headers := closedStringMap(128)
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "worker.service", Version: "1.0.0",
		Title: "Worker-to-worker service invocation",
		Description: "Direct HTTP-shaped request/response invocation of another Module Worker with " +
			"read-after-write consistency toward the invoked worker's own state: a response reflects every " +
			"effect the invoked handler completed before responding. The call never leaves the host.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write"},
		Limits:    map[string]int64{"maxRequestBytes": 104857600},
		Operations: []InterfaceOperation{
			{
				Name:        "fetch",
				Description: "Invoke the target worker's fetch handler with one request and return its response.",
				InputSchema: operationObject([]string{"method", "path"}, map[string]any{
					"method":  map[string]any{"type": "string", "enum": []any{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}},
					"path":    stringSchema(1, 8192),
					"headers": headers,
					"body":    map[string]any{"type": "string"},
				}),
				OutputSchema: operationObject([]string{"status"}, map[string]any{
					"status":  map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
					"headers": headers,
					"body":    map[string]any{"type": "string"},
				}),
				Errors: []string{"backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "fetch-roundtrip",
				Steps: []InterfaceFixtureStep{
					{Operation: "fetch", Input: map[string]any{"method": "GET", "path": "/health"}, Expected: map[string]any{"status": 200}},
				},
			},
		},
	}
}
