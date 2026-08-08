package edgeformcatalog

import "fmt"

// The six exact Interface contracts of the Edge Platform Family (decision
// 0010). An Interface Definition fixes operations with typed input/output
// schemas, a closed error vocabulary, consistency and pagination semantics,
// portable minimum limits, and data-only behavior fixtures. Its RFC 8785
// digest is the identity every Form and Binding references.

const (
	// InterfaceAPIVersion is the fixed group of exact Interface contracts.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	draft2020           = "https://json-schema.org/draft/2020-12/schema"

	// WorkerRuntimeInterfaceName is the exact runtime ABI contract a
	// ModuleWorker provides (decision 0019). The Interface name grammar of the
	// published interface-ref and interface-definition schemas is dotted
	// lowercase alphanumeric segments with no hyphen, so the contract cannot be
	// spelled "module-worker.runtime"; it takes the same "worker." prefix the
	// family's other worker-scoped Interface (worker.service) already uses,
	// while the BINDING namespace keeps the hyphenated "module-worker." prefix.
	WorkerRuntimeInterfaceName = "worker.runtime"

	// RuntimeHandlerOperation names the operation whose input schema carries
	// the closed handler vocabulary of the runtime contract. A host reads the
	// enum of its declaredHandlers property to learn which module handlers the
	// ABI defines; nothing else in the definition states that set, so there is
	// one source of truth for it.
	RuntimeHandlerOperation = "loadModule"
	// RuntimeHandlerProperty is the property of that operation's input schema
	// whose item enum IS the handler vocabulary.
	RuntimeHandlerProperty = "declaredHandlers"
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
		workerRuntimeInterface(),
	}
}

// RuntimeHandlers reads the closed handler vocabulary out of the runtime
// contract exactly the way a conforming host must: from the enum of the
// loadModule operation's declaredHandlers items. It is derived, never listed
// twice.
func RuntimeHandlers() ([]string, error) {
	definition, err := interfaceDefinitionByName(WorkerRuntimeInterfaceName)
	if err != nil {
		return nil, err
	}
	return runtimeHandlersOf(definition)
}

func runtimeHandlersOf(definition InterfaceDefinition) ([]string, error) {
	for _, operation := range definition.Operations {
		if operation.Name != RuntimeHandlerOperation {
			continue
		}
		properties, _ := operation.InputSchema["properties"].(map[string]any)
		property, _ := properties[RuntimeHandlerProperty].(map[string]any)
		items, _ := property["items"].(map[string]any)
		values, _ := items["enum"].([]any)
		if len(values) == 0 {
			return nil, fmt.Errorf(
				"interface %s operation %s declares no %s vocabulary",
				definition.Name, RuntimeHandlerOperation, RuntimeHandlerProperty,
			)
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("interface %s handler vocabulary carries a non-string member", definition.Name)
			}
			out = append(out, text)
		}
		return out, nil
	}
	return nil, fmt.Errorf("interface %s declares no %s operation", definition.Name, RuntimeHandlerOperation)
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

// runtimeHandlerNames is the closed handler vocabulary the runtime contract
// publishes through the loadModule operation. It is written once here and read
// back through RuntimeHandlers, so the Form's `handlers` enum and the contract
// can never disagree.
var runtimeHandlerNames = []any{"fetch", "scheduled", "queue", "tail"}

// runtimeModuleMediaTypes is the closed set of module media types a conforming
// runtime loads out of a Worker Bundle.
var runtimeModuleMediaTypes = []any{
	"application/javascript+module",
	"application/json",
	"application/octet-stream",
	"application/wasm",
	"text/plain",
}

// runtimeGlobals is the portable minimum Web platform surface. It is
// deliberately modest: every member is something a worker cannot be written
// without, and nothing on it is a convenience a host might reasonably omit.
var runtimeGlobals = []any{
	"AbortController",
	"AbortSignal",
	"Headers",
	"ReadableStream",
	"Request",
	"Response",
	"TextDecoder",
	"TextEncoder",
	"URL",
	"URLSearchParams",
	"WritableStream",
	"clearInterval",
	"clearTimeout",
	"crypto.getRandomValues",
	"crypto.subtle.digest",
	"fetch",
	"setInterval",
	"setTimeout",
	"structuredClone",
}

// workerRuntimeInterface is the exact ES Module Worker runtime ABI
// (decision 0019). Before it existed, `ModuleWorker` claimed to fix "the ES
// Module Worker ABI" by identity while nothing in the repository said what
// that ABI was: no handler signatures, no request/response types, no
// environment rule, no `waitUntil` meaning, no exception behavior, and no Web
// API floor. A conforming host now reads all of it here, at one exact digest.
//
// The published interface-definition meta-schema admits operations with
// input/output schemas, closed errors, semantics, limits, fixtures, and
// descriptions, and nothing else. Rules that are behavioral rather than
// structural are therefore stated as normative sentences in the descriptions
// and proven by fixtures where a fixture can prove them, exactly the model
// decision 0014 sets out.
func workerRuntimeInterface() InterfaceDefinition {
	handlerSet := func(maxItems int) map[string]any {
		return map[string]any{
			"type": "array", "uniqueItems": true, "maxItems": maxItems,
			"items": map[string]any{"type": "string", "enum": runtimeHandlerNames},
		}
	}
	nameList := func(maxItems, maxLength int) map[string]any {
		return map[string]any{
			"type": "array", "uniqueItems": true, "maxItems": maxItems,
			"items": stringSchema(1, maxLength),
		}
	}
	headers := closedStringMap(128)
	empty := operationObject(nil, map[string]any{})
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: WorkerRuntimeInterfaceName, Version: "1.0.0",
		Title: "ES Module Worker runtime ABI",
		Description: "The exact runtime ABI a conforming host provides to the code of one Module Worker, " +
			"and the exact shape that code must present. The main module is an ES module whose DEFAULT EXPORT " +
			"is a plain object; each portable handler is an OPTIONAL own property of that object named exactly " +
			"fetch, scheduled, queue, or tail, and every handler the Worker Version declares MUST exist there " +
			"as a callable property — a declared handler that is absent fails the version, not the request. " +
			"Every handler takes three arguments in this order: the event value, the binding environment env, " +
			"and the invocation context ctx. A handler may be async; a returned promise is awaited. " +
			"Request and response bodies are STREAMS, never buffered strings: a host never requires a handler " +
			"to read a request body before responding, and streams a response body out as the handler produces it. " +
			"env is a plain object whose own enumerable properties are exactly the names the Worker Version " +
			"declares — every vars key, every requiredSensitiveVars name, and every binding name — and nothing " +
			"else portable; a sensitive-variable slot appears as the host-supplied value under its declared name, " +
			"and that value never enters portable state. " +
			"ctx.waitUntil(promise) registers work the host keeps the isolate alive for until the promise settles; " +
			"a rejection is reported to host diagnostics only, never changes an already-sent response, and never " +
			"turns a successful invocation into a failed one. " +
			"An uncaught throw or unhandled rejection in fetch is a HOST-GENERATED 500 response, never a hung " +
			"request; in scheduled and queue it is a failed invocation with the retry semantics the relevant " +
			"contract already states. " +
			"The ABI is fixed by identity, not by a date: a runtime whose behavior differs is a different exact " +
			"contract version, and a Form that wants that runtime is a different Form version. " +
			"Behavior fixtures run against a conformance bundle of two modules: index.js, whose default export " +
			"exposes all four handlers, answers GET /health with 200, throws on GET /throw, and throws from its " +
			"queue handler when a message body is exactly \"fail\"; and fetch-only.js, whose default export " +
			"exposes fetch alone.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxRequestBytes":        104857600,
			"maxQueueBatchMessages":  100,
			"maxQueueMessageBytes":   131072,
			"maxEnvironmentEntries":  256,
			"maxWaitUntilTasks":      32,
			"maxTailEventsPerBatch":  100,
			"maxModulesPerBundleSet": 512,
		},
		Operations: []InterfaceOperation{
			{
				Name: RuntimeHandlerOperation,
				Description: "Loads one Worker Bundle into a fresh isolate before the first invocation. mainModule names " +
					"the ES module whose default export carries the handlers; modules lists the whole bundle. Exactly " +
					"these media types load: application/javascript+module as an ES module; application/json, default " +
					"export the parsed document; text/plain, default export the decoded UTF-8 text; " +
					"application/octet-stream, default export an ArrayBuffer; and application/wasm, whose default export " +
					"is a COMPILED WebAssembly.Module, never an instance — the runtime compiles it once per isolate and " +
					"the JavaScript module instantiates it with new WebAssembly.Instance(module, imports), so imports " +
					"stay the application's choice. exportedHandlers is the subset of the vocabulary the default export " +
					"exposes as callable own properties; a declared handler the module does not export fails with " +
					"handler_not_exported before any traffic arrives. The declaredHandlers enum IS this ABI's handler " +
					"vocabulary: a Worker Version declaring anything outside it is refused.",
				InputSchema: operationObject([]string{"declaredHandlers", "mainModule", "modules"}, map[string]any{
					"mainModule":           stringSchema(1, 1024),
					RuntimeHandlerProperty: handlerSet(len(runtimeHandlerNames)),
					"modules": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 512, "uniqueItems": true,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"mediaType", "name"},
							"properties": map[string]any{
								"name":      stringSchema(1, 1024),
								"mediaType": map[string]any{"type": "string", "enum": runtimeModuleMediaTypes},
							},
						},
					},
				}),
				OutputSchema: operationObject([]string{"exportedHandlers"}, map[string]any{
					"exportedHandlers": handlerSet(len(runtimeHandlerNames)),
				}),
				Errors:     []string{"module_not_found", "unsupported_media_type", "module_syntax_error", "handler_not_exported"},
				Idempotent: true,
			},
			{
				Name: "environment",
				Description: "Resolves the env object one invocation receives. Given everything a Worker Version declares, " +
					"propertyNames is the COMPLETE set of own enumerable properties of env in lexicographic order: every " +
					"vars key, every requiredSensitiveVars name carrying its host-supplied value, and every binding name " +
					"carrying the runtime API its Binding contract projects. Nothing else portable appears, so code that " +
					"reads a property a host added on its own is not portable. The three sources share ONE namespace: a " +
					"name declared twice is refused before the version is stored, never silently merged.",
				InputSchema: operationObject(
					[]string{"declaredBindingNames", "declaredSensitiveVars", "declaredVars"},
					map[string]any{
						"declaredVars":          nameList(256, 256),
						"declaredSensitiveVars": nameList(256, 64),
						"declaredBindingNames":  nameList(256, 64),
					},
				),
				OutputSchema: operationObject([]string{"propertyNames"}, map[string]any{
					"propertyNames": nameList(768, 256),
				}),
				Errors:     []string{"environment_name_collision"},
				Idempotent: true,
			},
			{
				Name: "globals",
				Description: "Reports the minimum Web platform surface every conforming runtime provides to module code. " +
					"present is the exact portable floor, named by dotted path: a runtime missing any member does not " +
					"implement this contract, and a runtime that adds members does not make those additions portable. " +
					"Timers are setTimeout, clearTimeout, setInterval, and clearInterval. fetch is the outbound HTTP " +
					"client and is the only member here whose calls leave the isolate.",
				InputSchema: empty,
				OutputSchema: operationObject([]string{"present"}, map[string]any{
					"present": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 128, "uniqueItems": true,
						"items": stringSchema(1, 128),
					},
				}),
				Errors:     []string{},
				Idempotent: true,
			},
			{
				Name: "fetch",
				Description: "Invokes fetch(request, env, ctx) with one HTTP request and returns its response. The handler " +
					"receives a Request and returns a Response or a promise of one; returning anything else, or nothing, " +
					"is an uncaught error. bodyStream states that the request body arrives as a ReadableStream the handler " +
					"may consume incrementally — a conforming host never buffers it into a string before invoking the " +
					"handler, and streams the response body back as it is produced. An uncaught throw or unhandled " +
					"rejection is a host-generated 500 response, never a hung request and never a truncated connection " +
					"with no status; handlerOutcome reports which of the two happened.",
				InputSchema: operationObject([]string{"method", "url"}, map[string]any{
					"method":     map[string]any{"type": "string", "enum": []any{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}},
					"url":        stringSchema(1, 8192),
					"headers":    headers,
					"bodyStream": map[string]any{"type": "boolean"},
				}),
				OutputSchema: operationObject([]string{"handlerOutcome", "status"}, map[string]any{
					"status":         map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
					"headers":        headers,
					"bodyStream":     map[string]any{"type": "boolean"},
					"handlerOutcome": map[string]any{"type": "string", "enum": []any{"returned", "uncaught"}},
				}),
				Errors: []string{"handler_not_declared", "request_too_large", "backend_unavailable"},
			},
			{
				Name: "scheduled",
				Description: "Invokes scheduled(event, env, ctx) at each match of a Worker Cron Trigger. The event carries " +
					"scheduledTime, the UTC instant the schedule matched as milliseconds since the Unix epoch, and cron, " +
					"the exact five-field UTC expression that matched. The handler returns nothing; a returned promise is " +
					"awaited before the invocation completes, and ctx.waitUntil extends it further. An uncaught throw or " +
					"unhandled rejection is a FAILED invocation reported to host diagnostics: it never becomes an HTTP " +
					"response, and it is not retried within the same matched minute.",
				InputSchema: operationObject([]string{"cron", "scheduledTime"}, map[string]any{
					"scheduledTime": map[string]any{"type": "integer", "minimum": 0},
					"cron":          stringSchema(1, 64),
				}),
				OutputSchema: empty,
				Errors:       []string{"handler_not_declared", "handler_failed", "backend_unavailable"},
			},
			{
				Name: "queue",
				Description: "Invokes queue(batch, env, ctx) with one message batch from a Queue Consumer attachment. The " +
					"batch carries queue, the portable name of the At-Least-Once Queue it drained, and messages, an ordered " +
					"array whose entries carry id (the host's message identity), timestamp (milliseconds since the Unix " +
					"epoch when the message was accepted), body (the exact bytes the producer sent), and attempts (1 on " +
					"first delivery, incremented on each redelivery). Batch size and timing are the consumer's " +
					"maxBatchSize and maxBatchTimeoutSeconds. Returning normally acknowledges the whole batch. An uncaught " +
					"throw or unhandled rejection FAILS the whole batch, which is redelivered under the at-least-once " +
					"delivery edge.queue states and the maxRetries and retryDelaySeconds of the invoking consumer, so " +
					"duplicates are visible and handlers must be idempotent.",
				InputSchema: operationObject([]string{"messages", "queue"}, map[string]any{
					"queue": stringSchema(1, 63),
					"messages": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 100,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"attempts", "body", "id", "timestamp"},
							"properties": map[string]any{
								"id":        stringSchema(1, 256),
								"timestamp": map[string]any{"type": "integer", "minimum": 0},
								"body":      stringSchema(0, 131072),
								"attempts":  map[string]any{"type": "integer", "minimum": 1},
							},
						},
					},
				}),
				OutputSchema: empty,
				Errors:       []string{"handler_not_declared", "handler_failed", "backend_unavailable"},
			},
			{
				Name: "tail",
				Description: "Invokes tail(events, env, ctx) with the diagnostic trace of invocations this worker is " +
					"attached to. Each event carries scriptName (the portable name of the worker that produced it), " +
					"outcome, eventTimestamp in milliseconds since the Unix epoch, logs, and exceptions. Trace delivery is " +
					"best effort and never changes the traced invocation: a failed tail handler never fails the " +
					"invocation it observed. The handler returns nothing.",
				InputSchema: operationObject([]string{"events"}, map[string]any{
					"events": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 100,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"eventTimestamp", "outcome", "scriptName"},
							"properties": map[string]any{
								"scriptName":     stringSchema(1, 63),
								"outcome":        map[string]any{"type": "string", "enum": []any{"cancelled", "exceededCpu", "exception", "ok"}},
								"eventTimestamp": map[string]any{"type": "integer", "minimum": 0},
								"logs":           nameList(256, 8192),
								"exceptions":     nameList(256, 8192),
							},
						},
					},
				}),
				OutputSchema: empty,
				Errors:       []string{"handler_not_declared", "handler_failed"},
			},
			{
				Name: "waitUntil",
				Description: "Registers one promise through ctx.waitUntil(promise). The host keeps the isolate alive until " +
					"that promise settles, even after the handler returned and its response was sent, and never cancels the " +
					"work to reclaim the isolate earlier. taskOutcome is what the registered promise does. Settlement never " +
					"rewrites an already-sent response, and a rejected task never turns a successful invocation into a " +
					"failed one: it is reported to host diagnostics only. Calling waitUntil after the isolate has been " +
					"reclaimed fails with context_expired.",
				InputSchema: operationObject([]string{"taskOutcome"}, map[string]any{
					"taskOutcome": map[string]any{"type": "string", "enum": []any{"rejected", "resolved"}},
				}),
				OutputSchema: operationObject(
					[]string{"invocationFailed", "isolateHeldUntilSettled", "responseChanged"},
					map[string]any{
						"isolateHeldUntilSettled": map[string]any{"type": "boolean"},
						"responseChanged":         map[string]any{"type": "boolean"},
						"invocationFailed":        map[string]any{"type": "boolean"},
					},
				),
				Errors: []string{"context_expired"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "declared-handler-must-be-exported",
				Steps: []InterfaceFixtureStep{
					{Operation: RuntimeHandlerOperation, Input: map[string]any{
						"mainModule":           "index.js",
						"modules":              []any{map[string]any{"name": "index.js", "mediaType": "application/javascript+module"}},
						RuntimeHandlerProperty: []any{"fetch", "scheduled", "queue", "tail"},
					}, Expected: map[string]any{"exportedHandlers": []any{"fetch", "scheduled", "queue", "tail"}}},
					{Operation: RuntimeHandlerOperation, Input: map[string]any{
						"mainModule":           "fetch-only.js",
						"modules":              []any{map[string]any{"name": "fetch-only.js", "mediaType": "application/javascript+module"}},
						RuntimeHandlerProperty: []any{"fetch", "scheduled"},
					}, ExpectedError: "handler_not_exported"},
				},
			},
			{
				Name: "globals-minimum-surface",
				Steps: []InterfaceFixtureStep{
					{Operation: "globals", Input: map[string]any{}, Expected: map[string]any{"present": runtimeGlobals}},
				},
			},
			{
				Name: "environment-projects-exactly-the-declared-names",
				Steps: []InterfaceFixtureStep{
					{Operation: "environment", Input: map[string]any{
						"declaredVars":          []any{"LOG_LEVEL"},
						"declaredSensitiveVars": []any{"API_SIGNING_TOKEN_NAME"},
						"declaredBindingNames":  []any{"CACHE"},
					}, Expected: map[string]any{
						"propertyNames": []any{"API_SIGNING_TOKEN_NAME", "CACHE", "LOG_LEVEL"},
					}},
				},
			},
			{
				Name: "environment-name-collision-refused",
				Steps: []InterfaceFixtureStep{
					{Operation: "environment", Input: map[string]any{
						"declaredVars":          []any{"CACHE"},
						"declaredSensitiveVars": []any{},
						"declaredBindingNames":  []any{"CACHE"},
					}, ExpectedError: "environment_name_collision"},
				},
			},
			{
				Name: "fetch-returns-a-response",
				Steps: []InterfaceFixtureStep{
					{Operation: "fetch", Input: map[string]any{
						"method": "GET", "url": "https://conformance.invalid/health", "bodyStream": true,
					}, Expected: map[string]any{"status": 200, "handlerOutcome": "returned"}},
				},
			},
			{
				Name: "fetch-uncaught-throw-is-a-host-error-response",
				Steps: []InterfaceFixtureStep{
					{Operation: "fetch", Input: map[string]any{
						"method": "GET", "url": "https://conformance.invalid/throw",
					}, Expected: map[string]any{"status": 500, "handlerOutcome": "uncaught"}},
				},
			},
			{
				Name: "queue-failure-fails-the-whole-batch",
				Steps: []InterfaceFixtureStep{
					{Operation: "queue", Input: map[string]any{
						"queue": "at-least-once-queue",
						"messages": []any{map[string]any{
							"id": "message-1", "timestamp": 0, "body": "fail", "attempts": 1,
						}},
					}, ExpectedError: "handler_failed"},
				},
			},
			{
				Name: "wait-until-rejection-does-not-change-the-response",
				Steps: []InterfaceFixtureStep{
					{Operation: "waitUntil", Input: map[string]any{"taskOutcome": "rejected"}, Expected: map[string]any{
						"isolateHeldUntilSettled": true,
						"responseChanged":         false,
						"invocationFailed":        false,
					}},
				},
			},
		},
	}
}
