package edgeformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

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

	// RuntimeLoadableProperty is the property of the loadModule input schema
	// whose item mediaType enum IS the set of module media types the module
	// graph may import — and therefore the set mainModule may name.
	RuntimeLoadableProperty = "modules"
	// RuntimeAuxiliaryProperty is the property whose item mediaType enum IS
	// the set a bundle may carry without ever importing it.
	RuntimeAuxiliaryProperty = "auxiliaryModules"
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

// closedObject builds one closed object schema. It carries no `$schema`, so it
// nests inside another schema without minting a second dialect declaration.
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

// operationObject builds one closed Draft 2020-12 operation payload schema.
func operationObject(required []string, properties map[string]any) map[string]any {
	schema := closedObject(required, properties)
	schema["$schema"] = draft2020
	return schema
}

// withDialect promotes one nested closed object to a top-level operation
// payload schema.
func withDialect(schema map[string]any) map[string]any {
	out := map[string]any{"$schema": draft2020}
	for key, value := range schema {
		out[key] = value
	}
	return out
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

// Portable ceilings the data-plane contracts declare. They are constants
// because each one is stated in two places that must agree — the structural
// bound inside an operation schema and the declared `limits` entry — and a
// literal repeated in both is a literal that eventually stops matching.
const (
	// kvMaxValueBytes bounds the DECODED length of one edge.kv value.
	kvMaxValueBytes = 26214400
	// queueMaxMessageBytes bounds the DECODED length of one queue message body.
	queueMaxMessageBytes = 131072
	// objectsMaxObjectBytes is the largest object, reachable only through a
	// multipart upload.
	objectsMaxObjectBytes = 5368709120
	// objectsMaxSinglePutBytes is the largest object one put may write. Above
	// it, and whenever the producer does not know the size in advance, the
	// multipart operations are the path.
	objectsMaxSinglePutBytes = 314572800
	// objectsMaxMultipartParts bounds the parts of one multipart upload.
	objectsMaxMultipartParts = 10000
)

// base64Value builds one encoded-bytes fixture value from its base64 text.
func base64Value(data string) map[string]any {
	return map[string]any{"encoding": "base64", "data": data}
}

// The five tagged SQLite values, as fixture literals.
func sqlNull() map[string]any { return map[string]any{"type": "null"} }
func sqlInteger(decimal string) map[string]any {
	return map[string]any{"type": "integer", "value": decimal}
}
func sqlReal(value float64) map[string]any { return map[string]any{"type": "real", "value": value} }
func sqlText(value string) map[string]any  { return map[string]any{"type": "text", "value": value} }
func sqlBlob(base64 string) map[string]any { return map[string]any{"type": "blob", "base64": base64} }

// base64Length is the exact length of the standard base64 encoding, with
// padding, of maxDecodedBytes bytes. It is what bounds the `data` string of an
// encoded-bytes value: the JSON ceiling and the byte limit then agree instead
// of measuring two different quantities
// ([decision 0020](../../spec/decisions/0020-the-edge-interfaces-state-their-data-and-delivery-model.md)).
func base64Length(maxDecodedBytes int) int {
	return 4 * ((maxDecodedBytes + 2) / 3)
}

// encodedBytes is the ONE encoded-bytes shape of the family. Every contract
// that carries opaque bytes through JSON carries them exactly this way, so a
// value that moves from a queue message into a KV namespace does not change
// representation on the way.
//
// It exists because `maxValueBytes` and a JSON Schema `maxLength` do not
// measure the same thing: `maxLength` counts UTF-16 code units of a string,
// while a byte limit counts bytes, and the two disagree for every value outside
// ASCII. Making the value an explicitly encoded byte string removes the
// question — the structural ceiling bounds the ENCODING, and the declared limit
// bounds the DECODED bytes.
func encodedBytes(maxDecodedBytes int) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"data", "encoding"},
		"properties": map[string]any{
			"encoding": map[string]any{"type": "string", "enum": []any{"base64"}},
			"data":     map[string]any{"type": "string", "maxLength": base64Length(maxDecodedBytes)},
		},
	}
}

// sqlDecimalInteger carries a 64-bit SQLite INTEGER as its canonical decimal
// text. A JSON number cannot: IEEE-754 double has 53 bits of mantissa, so
// 9223372036854775807 would arrive as 9223372036854775808 and a rowid past 2^53
// would silently drift. The pattern admits a few values above the 64-bit range,
// which the definition's own normative sentence closes — a structural minimum
// with the semantic rule stated alongside it (decision 0014).
func sqlDecimalInteger() map[string]any {
	return map[string]any{
		"type":      "string",
		"maxLength": 20,
		"pattern":   `^(?:0|-?[1-9][0-9]{0,18})$`,
	}
}

// sqlValue is the tagged SQLite storage-class value: exactly one of NULL,
// INTEGER, REAL, TEXT, and BLOB, each carrying its payload in the member that
// can hold it losslessly.
//
// The untagged `boolean|null|number|string` union it replaces could carry
// neither a 64-bit INTEGER nor a BLOB: the first lost precision above 2^53 and
// the second had nowhere to go at all, so a column round-tripping through this
// interface came back as a different value. SQLite has no boolean storage
// class, so there is no boolean member either; 0 and 1 are INTEGERs.
func sqlValue() map[string]any {
	tagged := func(tag string, payload map[string]any, member string) map[string]any {
		properties := map[string]any{"type": map[string]any{"type": "string", "enum": []any{tag}}}
		required := []string{"type"}
		if member != "" {
			properties[member] = payload
			required = []string{member, "type"}
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties":           properties,
		}
	}
	return map[string]any{
		"type": "object",
		"oneOf": []any{
			tagged("null", nil, ""),
			tagged("integer", sqlDecimalInteger(), "value"),
			tagged("real", map[string]any{"type": "number"}, "value"),
			tagged("text", map[string]any{"type": "string", "maxLength": 1000000}, "value"),
			tagged("blob", map[string]any{"type": "string", "maxLength": base64Length(1000000)}, "base64"),
		},
	}
}

func sqlParams() map[string]any {
	return map[string]any{
		"type":     "array",
		"maxItems": 100,
		"items":    sqlValue(),
	}
}

// sqlStatementResult is the ONE result shape of every statement, whether it ran
// alone or inside a transaction. A transaction that could report only a write
// count could not carry the rows a `SELECT` inside it returned, which made the
// atomic path strictly weaker than the non-atomic one — a reason to leave the
// transaction, which is the opposite of what it is for.
func sqlStatementResult() map[string]any {
	return closedObject([]string{"rows", "rowsWritten"}, map[string]any{
		"rows":            sqlRows(),
		"rowsWritten":     map[string]any{"type": "integer", "minimum": 0},
		"lastInsertRowId": sqlDecimalInteger(),
	})
}

func sqlRows() map[string]any {
	return map[string]any{
		"type":     "array",
		"maxItems": 10000,
		"items": map[string]any{
			"type":                 "object",
			"maxProperties":        256,
			"propertyNames":        map[string]any{"type": "string", "maxLength": 128},
			"additionalProperties": sqlValue(),
		},
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

// RuntimeLoadableMediaTypes reads the importable module media types back out
// of the runtime contract, the way a conforming host must: from the mediaType
// enum of the loadModule operation's `modules` items. Nothing else in the
// definition states that set.
func RuntimeLoadableMediaTypes() ([]string, error) {
	return runtimeMediaTypesOf(RuntimeLoadableProperty)
}

// RuntimeAuxiliaryMediaTypes reads the carry-only module media types back out
// of the same operation's `auxiliaryModules` items.
func RuntimeAuxiliaryMediaTypes() ([]string, error) {
	return runtimeMediaTypesOf(RuntimeAuxiliaryProperty)
}

func runtimeMediaTypesOf(property string) ([]string, error) {
	definition, err := interfaceDefinitionByName(WorkerRuntimeInterfaceName)
	if err != nil {
		return nil, err
	}
	for _, operation := range definition.Operations {
		if operation.Name != RuntimeHandlerOperation {
			continue
		}
		properties, _ := operation.InputSchema["properties"].(map[string]any)
		list, _ := properties[property].(map[string]any)
		items, _ := list["items"].(map[string]any)
		itemProperties, _ := items["properties"].(map[string]any)
		mediaType, _ := itemProperties["mediaType"].(map[string]any)
		values, _ := mediaType["enum"].([]any)
		if len(values) == 0 {
			return nil, fmt.Errorf(
				"interface %s operation %s declares no %s media-type set",
				definition.Name, RuntimeHandlerOperation, property,
			)
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf(
					"interface %s %s media-type set carries a non-string member",
					definition.Name, property,
				)
			}
			out = append(out, text)
		}
		return out, nil
	}
	return nil, fmt.Errorf("interface %s declares no %s operation", definition.Name, RuntimeHandlerOperation)
}

// edgeKVInterface is the eventually consistent edge key/value contract.
//
// Two things about it were incoherent before decision 0020. Its fixtures
// required a put to be visible to the very next get and a delete to make the
// next get miss, while its semantics declared eventual consistency: both cannot
// be normative, and the fixtures were the half that a correct eventually
// consistent host would fail. And its value model measured two different
// quantities at once — `maxValueBytes` counts bytes while a JSON Schema
// `maxLength` counts string length — so no host could tell which one bounded a
// value. Values are now explicitly bytes, carried in the family's one
// encoded-bytes shape, and the fixtures assert only what one client can observe
// without waiting for convergence.
func edgeKVInterface() InterfaceDefinition {
	key := stringSchema(1, 512)
	metadata := closedStringMap(64)
	value := encodedBytes(kvMaxValueBytes)
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.kv", Version: "1.0.0",
		Title: "Edge key/value namespace",
		Description: "Globally replicated key/value storage of OPAQUE BYTES with eventual consistency. " +
			"A value is a byte string, never text: it travels in the family's encoded-bytes shape " +
			"{\"encoding\": \"base64\", \"data\": \"…\"}, where data is RFC 4648 section 4 base64 with padding " +
			"and without line breaks. maxValueBytes bounds the DECODED length; the maxLength on data is the " +
			"structural ceiling of that encoding and is not the limit. A put whose decoded value exceeds " +
			"maxValueBytes fails with value_too_large, and one whose data is not decodable base64 fails with " +
			"invalid_value. A key is a UTF-8 string and maxKeyBytes bounds its UTF-8 ENCODED length, so a key " +
			"of 512 astral characters is 2048 bytes and fails with invalid_key even though it is 512 code " +
			"points. Metadata is a text map, never bytes and never secret; maxMetadataBytes bounds the UTF-8 " +
			"encoding of its canonical JSON and exceeding it fails with metadata_too_large. " +
			"Consistency is eventual, and that is this contract's identity rather than an option a host " +
			"chooses. A read is served by whichever replica is nearest, so a read that follows a write MAY " +
			"return the previous value, or not_found, until replication converges — including a read the same " +
			"client issues immediately after its own write, on the same connection, from the same location. " +
			"This contract states NO read-your-writes guarantee, no session in which one would hold, and no " +
			"bound on convergence time; a host states its own convergence target in its Host Support Profile. " +
			"Deleting an absent key succeeds, and a delete converges the same way, so a get after a resolved " +
			"delete may still return the deleted value. " +
			"The behavior fixtures below are exactly what one client can prove about such a store: each runs " +
			"against a fresh scope and asserts only facts that no write has to converge for. That a write " +
			"eventually becomes visible everywhere is a HOST OBLIGATION rather than a proven property; the " +
			"split is written down in the Host API specification.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Pagination: "cursor"},
		Limits: map[string]int64{
			"maxKeyBytes": 512, "maxValueBytes": kvMaxValueBytes,
			"maxMetadataBytes": 1024, "maxListPageKeys": 1000,
		},
		Operations: []InterfaceOperation{
			{
				Name: "get",
				Description: "Read one value by key. Absent keys fail with not_found. Eventual consistency applies to this " +
					"read: a key written moments ago, by this client or another, may still fail with not_found, and a key " +
					"overwritten moments ago may still return the previous value. The value is the family's encoded-bytes " +
					"shape; a host never returns a bare string here.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"value"}, map[string]any{"value": value}),
				Errors:       []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name: "getWithMetadata",
				Description: "Read one value and its non-secret metadata document by key. A host that stored no metadata " +
					"omits the property rather than returning an empty object. Everything get states about eventual " +
					"consistency applies unchanged: the value and the metadata are read from one replica, so they agree " +
					"with each other but are not necessarily current.",
				InputSchema: operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"value"}, map[string]any{
					"value":    value,
					"metadata": metadata,
				}),
				Errors:     []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "put",
				Description: "Write one value, replacing any previous value for the key. The value is the family's " +
					"encoded-bytes shape and maxValueBytes bounds its DECODED length. An optional expiration TTL removes " +
					"the entry after AT LEAST that many seconds; it is not gone at exactly that instant, and a get starts " +
					"failing with not_found once the removal converges. A resolved put means the write is durable at the " +
					"accepting replica; it is not a promise that the next get anywhere observes it.",
				InputSchema: operationObject([]string{"key", "value"}, map[string]any{
					"key":                  key,
					"value":                value,
					"expirationTtlSeconds": map[string]any{"type": "integer", "minimum": 60, "maximum": 315360000},
					"metadata":             metadata,
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors: []string{
					"invalid_key", "invalid_value", "value_too_large",
					"metadata_too_large", "backend_unavailable",
				},
			},
			{
				Name: "delete",
				Description: "Delete one key. Deleting an absent key succeeds, so delete can never be used to test " +
					"existence. Like every write the removal converges: a get after a resolved delete may still return the " +
					"deleted value.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name: "list",
				Description: "List key names in lexicographic order by their UTF-8 bytes, optionally under a prefix, one " +
					"cursor page at a time. limit bounds one page and a host may return fewer. listComplete is true exactly " +
					"when this page is the last; when it is false the cursor addresses the next page and MUST be passed back " +
					"unmodified. A cursor is opaque, and one a host did not issue fails with invalid_cursor. The listing is " +
					"eventually consistent like every other read: a key written moments ago may be missing from it, and a " +
					"key deleted moments ago may still appear.",
				InputSchema: operationObject(nil, map[string]any{
					"prefix": stringSchema(0, 512),
					"cursor": stringSchema(1, 4096),
					"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 1000},
				}),
				OutputSchema: operationObject([]string{"keys", "listComplete"}, map[string]any{
					"keys": map[string]any{
						"type":     "array",
						"maxItems": 1000,
						"items":    closedObject([]string{"name"}, map[string]any{"name": key}),
					},
					"listComplete": map[string]any{"type": "boolean"},
					"cursor":       stringSchema(1, 4096),
				}),
				Errors:     []string{"invalid_cursor", "backend_unavailable"},
				Idempotent: true,
			},
		},
		// Every fixture runs against a fresh scope and asserts only outcomes that
		// do not depend on replication having converged. A put-then-get trace is
		// deliberately absent: it is the one thing an eventually consistent store
		// is allowed to fail, so requiring it would have made this contract
		// unimplementable by exactly the systems it describes.
		Fixtures: []InterfaceFixture{
			{
				Name: "put-accepted",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "greeting", "value": base64Value("aGVsbG8="),
					}},
				},
			},
			{
				Name: "never-written-key-is-not-found",
				Steps: []InterfaceFixtureStep{
					{Operation: "get", Input: map[string]any{"key": "never-written"}, ExpectedError: "not_found"},
				},
			},
			{
				Name: "delete-absent-key-succeeds",
				Steps: []InterfaceFixtureStep{
					{Operation: "delete", Input: map[string]any{"key": "never-written"}},
				},
			},
			{
				Name: "list-of-an-empty-scope-is-complete",
				Steps: []InterfaceFixtureStep{
					{Operation: "list", Input: map[string]any{}, Expected: map[string]any{
						"keys": []any{}, "listComplete": true,
					}},
				},
			},
		},
	}
}

// edgeObjectsInterface is the strongly consistent object bucket.
//
// Before decision 0020 its description promised range reads that `get` had no
// input for, and its body was a JSON string bounded by nothing while the object
// size limit said 5 GiB — a shape no host could produce and no client could
// consume. It is now a real object API: head, ranged and conditional get,
// conditional put, delete, list with delimiter roll-up, and the four multipart
// operations, with bodies STREAMING beside the operation document rather than
// inside it.
func edgeObjectsInterface() InterfaceDefinition {
	key := stringSchema(1, 1024)
	etag := stringSchema(1, 256)
	contentType := stringSchema(1, 256)
	size := map[string]any{"type": "integer", "minimum": 0, "maximum": objectsMaxObjectBytes}
	millis := map[string]any{"type": "integer", "minimum": 0}
	bodyStream := map[string]any{"type": "boolean"}
	uploadID := stringSchema(1, 256)
	partNumber := map[string]any{"type": "integer", "minimum": 1, "maximum": objectsMaxMultipartParts}
	byteRange := closedObject([]string{"offset"}, map[string]any{
		"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": objectsMaxObjectBytes},
		"length": map[string]any{"type": "integer", "minimum": 1, "maximum": objectsMaxObjectBytes},
	})
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.objects", Version: "1.0.0",
		Title: "Object bucket",
		Description: "Flat-namespace object storage with strong read-after-write consistency and STREAMING bodies. " +
			"An object body never travels as a JSON member: maxObjectBytes is 5 GiB and no JSON string carries " +
			"that, so get and put declare bodyStream and the bytes move as a stream beside the operation " +
			"document. contentLength states the exact byte count in both directions, and a put whose stream " +
			"does not deliver exactly contentLength bytes fails with invalid_body having stored nothing. " +
			"Consistency: a get, head, or list issued after a put or a delete that has already resolved " +
			"observes that put or that delete. Writes to one key are last-writer-wins; there is no cross-key " +
			"atomicity, no transaction, and no versioning, so a replaced object is gone. An etag is a STRONG " +
			"validator of the exact bytes of one object. A conditional get or put fences with ifMatch (act only " +
			"when the current etag is exactly this one); a get may instead fence with ifNoneMatch on an etag, " +
			"and a put with ifNoneMatch \"*\" to write only when the key is absent. A failed precondition is " +
			"precondition_failed and changes nothing. A ranged get returns a contiguous subrange of ONE object: " +
			"offset is the first byte, length the count, an omitted length runs to the end, and an offset at or " +
			"past the object size fails with range_not_satisfiable rather than returning an empty body. " +
			"Objects above maxSinglePutBytes, and any object whose size the producer does not know in advance, " +
			"are written through the multipart operations: createMultipartUpload opens an upload, uploadPart " +
			"writes one numbered part, completeMultipartUpload assembles the parts in part-number order into " +
			"one object with one etag, and abortMultipartUpload discards the upload. Every part except the " +
			"highest-numbered one MUST be at least 5242880 bytes. An upload holds no key until it completes, so " +
			"a get of that key before completion behaves as though the upload did not exist. " +
			"In a behavior trace, a step whose input declares bodyStream sends exactly contentLength bytes " +
			"whose value at index i is i modulo 256, which makes every trace below byte-for-byte reproducible. " +
			"Multipart traces are absent from that set because their steps depend on part etags the host mints " +
			"during the trace; multipart assembly is a HOST OBLIGATION stated in the Host API specification.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write", Pagination: "cursor"},
		Limits: map[string]int64{
			"maxKeyBytes": 1024, "maxObjectBytes": objectsMaxObjectBytes,
			"maxSinglePutBytes": objectsMaxSinglePutBytes, "maxMultipartParts": objectsMaxMultipartParts,
			"maxListPageObjects": 1000,
		},
		Operations: []InterfaceOperation{
			{
				Name: "head",
				Description: "Read one object's size, strong etag, content type, and write time without its body. Absent " +
					"keys fail with not_found. uploadedAtMillis is milliseconds since the Unix epoch, in UTC, of the write " +
					"that produced the current bytes.",
				InputSchema: operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject([]string{"etag", "size"}, map[string]any{
					"contentType":      contentType,
					"etag":             etag,
					"size":             size,
					"uploadedAtMillis": millis,
				}),
				Errors:     []string{"invalid_key", "not_found", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "get",
				Description: "Read one object as a byte stream. bodyStream is always true in the output: the body is " +
					"streamed beside this document and is never a member of it, so a caller may begin consuming before the " +
					"whole object has arrived. size is the size of the WHOLE object, partial says whether a range was " +
					"applied, and range echoes the exact subrange served. A conditional get fences with ifMatch (serve only " +
					"when the current etag is exactly this one) or ifNoneMatch (serve only when it is not); a failed " +
					"precondition is precondition_failed and carries no body. A range whose offset is at or past size fails " +
					"with range_not_satisfiable.",
				InputSchema: operationObject([]string{"key"}, map[string]any{
					"key":         key,
					"range":       byteRange,
					"ifMatch":     etag,
					"ifNoneMatch": etag,
				}),
				OutputSchema: operationObject([]string{"bodyStream", "etag", "partial", "size"}, map[string]any{
					"bodyStream":  bodyStream,
					"contentType": contentType,
					"etag":        etag,
					"partial":     map[string]any{"type": "boolean"},
					"range":       byteRange,
					"size":        size,
				}),
				Errors: []string{
					"invalid_key", "not_found", "precondition_failed",
					"range_not_satisfiable", "backend_unavailable",
				},
				Idempotent: true,
			},
			{
				Name: "put",
				Description: "Write one object from a byte stream, replacing any previous object at the key, and return " +
					"the strong etag of the bytes written. bodyStream MUST be true and contentLength MUST be the exact byte " +
					"count of the stream; a stream delivering a different count fails with invalid_body and stores nothing. " +
					"A conditional put fences with ifMatch on the current etag, or with ifNoneMatch \"*\" to write only when " +
					"the key is absent; a failed precondition is precondition_failed and writes nothing. A contentLength " +
					"above maxSinglePutBytes fails with value_too_large — that object belongs in a multipart upload.",
				InputSchema: operationObject([]string{"bodyStream", "contentLength", "key"}, map[string]any{
					"key":           key,
					"bodyStream":    bodyStream,
					"contentLength": size,
					"contentType":   contentType,
					"ifMatch":       etag,
					"ifNoneMatch":   map[string]any{"type": "string", "enum": []any{"*"}},
				}),
				OutputSchema: operationObject([]string{"etag", "size"}, map[string]any{"etag": etag, "size": size}),
				Errors: []string{
					"invalid_key", "invalid_body", "value_too_large",
					"precondition_failed", "backend_unavailable",
				},
			},
			{
				Name: "delete",
				Description: "Delete one object. Deleting an absent key succeeds, so delete never reports existence. A " +
					"head, get, or list issued after a resolved delete does not see the object.",
				InputSchema:  operationObject([]string{"key"}, map[string]any{"key": key}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name: "list",
				Description: "List objects in lexicographic key order by their UTF-8 bytes, optionally under a prefix, one " +
					"cursor page at a time. limit bounds one page and a host may return fewer. truncated is true exactly " +
					"when another page follows, and the cursor then addresses it and MUST be passed back unmodified. A " +
					"delimiter rolls every key whose remainder after the prefix contains it up into prefixes, and those keys " +
					"are then absent from objects. The listing observes every put and delete that has already resolved.",
				InputSchema: operationObject(nil, map[string]any{
					"prefix":    stringSchema(0, 1024),
					"delimiter": stringSchema(1, 16),
					"cursor":    stringSchema(1, 4096),
					"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 1000},
				}),
				OutputSchema: operationObject([]string{"objects", "truncated"}, map[string]any{
					"objects": map[string]any{
						"type":     "array",
						"maxItems": 1000,
						"items": closedObject([]string{"etag", "key", "size"}, map[string]any{
							"key": key, "etag": etag, "size": size, "uploadedAtMillis": millis,
						}),
					},
					"prefixes": map[string]any{
						"type": "array", "maxItems": 1000, "uniqueItems": true,
						"items": stringSchema(1, 1024),
					},
					"truncated": map[string]any{"type": "boolean"},
					"cursor":    stringSchema(1, 4096),
				}),
				Errors:     []string{"invalid_cursor", "backend_unavailable"},
				Idempotent: true,
			},
			{
				Name: "createMultipartUpload",
				Description: "Open a multipart upload for one key and return its opaque uploadId. The key is not written " +
					"and not reserved: another writer may put or complete the same key meanwhile, and last writer wins. An " +
					"upload a host has abandoned fails every later part with upload_not_found.",
				InputSchema: operationObject([]string{"key"}, map[string]any{
					"key": key, "contentType": contentType,
				}),
				OutputSchema: operationObject([]string{"uploadId"}, map[string]any{"uploadId": uploadID}),
				Errors:       []string{"invalid_key", "backend_unavailable"},
			},
			{
				Name: "uploadPart",
				Description: "Write one numbered part of an open upload from a byte stream and return the part's strong " +
					"etag, which completeMultipartUpload requires back. Part numbers are 1..maxMultipartParts and need not " +
					"be contiguous; re-uploading a number replaces that part. Every part except the highest-numbered one " +
					"MUST be at least 5242880 bytes, and a shorter one fails with invalid_part. contentLength MUST be the " +
					"exact byte count of the stream.",
				InputSchema: operationObject(
					[]string{"bodyStream", "contentLength", "key", "partNumber", "uploadId"},
					map[string]any{
						"key": key, "uploadId": uploadID, "partNumber": partNumber,
						"bodyStream": bodyStream, "contentLength": size,
					},
				),
				OutputSchema: operationObject([]string{"etag", "partNumber"}, map[string]any{
					"etag": etag, "partNumber": partNumber,
				}),
				Errors: []string{
					"invalid_key", "invalid_body", "invalid_part",
					"upload_not_found", "value_too_large", "backend_unavailable",
				},
			},
			{
				Name: "completeMultipartUpload",
				Description: "Assemble the named parts, in ascending part-number order, into one object at the key and " +
					"return the object's strong etag and total size. Every part listed must have been uploaded and must " +
					"carry the etag uploadPart returned for it; a mismatch fails with invalid_part and assembles nothing. " +
					"The object becomes visible atomically: no reader ever observes a partially assembled object. " +
					"Completing an upload the host no longer holds fails with upload_not_found.",
				InputSchema: operationObject([]string{"key", "parts", "uploadId"}, map[string]any{
					"key": key, "uploadId": uploadID,
					"parts": map[string]any{
						"type": "array", "minItems": 1, "maxItems": objectsMaxMultipartParts,
						"items": closedObject([]string{"etag", "partNumber"}, map[string]any{
							"etag": etag, "partNumber": partNumber,
						}),
					},
				}),
				OutputSchema: operationObject([]string{"etag", "size"}, map[string]any{"etag": etag, "size": size}),
				Errors: []string{
					"invalid_key", "invalid_part", "upload_not_found",
					"value_too_large", "precondition_failed", "backend_unavailable",
				},
			},
			{
				Name: "abortMultipartUpload",
				Description: "Discard an open upload and every part written to it. It never touches the key: an object " +
					"already at that key is untouched. Aborting an upload the host no longer holds fails with " +
					"upload_not_found, so abort is not a way to test whether an upload exists.",
				InputSchema: operationObject([]string{"key", "uploadId"}, map[string]any{
					"key": key, "uploadId": uploadID,
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"invalid_key", "upload_not_found", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "put-then-head-observes-the-write",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "reports/summary.txt", "bodyStream": true,
						"contentLength": 8, "contentType": "text/plain",
					}},
					{Operation: "head", Input: map[string]any{"key": "reports/summary.txt"}, Expected: map[string]any{
						"size": 8,
					}},
				},
			},
			{
				Name: "ranged-get-returns-a-subrange-of-one-object",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "blobs/data.bin", "bodyStream": true, "contentLength": 1024,
					}},
					{Operation: "get", Input: map[string]any{
						"key": "blobs/data.bin", "range": map[string]any{"offset": 512, "length": 256},
					}, Expected: map[string]any{
						"bodyStream": true, "partial": true, "size": 1024,
						"range": map[string]any{"offset": 512, "length": 256},
					}},
				},
			},
			{
				Name: "range-past-the-end-is-refused",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "blobs/small.bin", "bodyStream": true, "contentLength": 4,
					}},
					{Operation: "get", Input: map[string]any{
						"key": "blobs/small.bin", "range": map[string]any{"offset": 64},
					}, ExpectedError: "range_not_satisfiable"},
				},
			},
			{
				Name: "conditional-put-refuses-an-existing-key",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "once.txt", "bodyStream": true, "contentLength": 1,
					}},
					{Operation: "put", Input: map[string]any{
						"key": "once.txt", "bodyStream": true, "contentLength": 1, "ifNoneMatch": "*",
					}, ExpectedError: "precondition_failed"},
				},
			},
			{
				Name: "delete-then-head-not-found",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "tmp/scratch.txt", "bodyStream": true, "contentLength": 1,
					}},
					{Operation: "delete", Input: map[string]any{"key": "tmp/scratch.txt"}},
					{Operation: "head", Input: map[string]any{"key": "tmp/scratch.txt"}, ExpectedError: "not_found"},
				},
			},
			{
				Name: "list-truncates-at-the-requested-limit",
				Steps: []InterfaceFixtureStep{
					{Operation: "put", Input: map[string]any{
						"key": "page/a", "bodyStream": true, "contentLength": 1,
					}},
					{Operation: "put", Input: map[string]any{
						"key": "page/b", "bodyStream": true, "contentLength": 1,
					}},
					{Operation: "list", Input: map[string]any{"prefix": "page/", "limit": 1}, Expected: map[string]any{
						"truncated": true,
					}},
					{Operation: "list", Input: map[string]any{"prefix": "page/"}, Expected: map[string]any{
						"truncated": false,
					}},
				},
			},
		},
	}
}

// edgeSQLInterface is the embedded SQLite contract.
//
// Its value model used to be the untagged union boolean|null|number|string,
// which could carry neither of the two things SQLite users depend on: a 64-bit
// INTEGER (IEEE-754 double has 53 bits of mantissa, so a rowid past 2^53 came
// back changed) and a BLOB (which had nowhere to go at all). Values are now
// tagged by storage class, integers and rowids travel as canonical decimal
// text, and every statement — inside a transaction or not — reports the same
// result shape, so the atomic path is no longer strictly weaker than the
// non-atomic one.
func edgeSQLInterface() InterfaceDefinition {
	statementResult := sqlStatementResult()
	sql := stringSchema(1, 100000)
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.sql", Version: "1.0.0",
		Title: "Embedded SQLite database",
		Description: "SQL statements executed against one embedded SQLite database with serializable transactions. " +
			"Values are TAGGED, never bare JSON scalars: every bound parameter and every returned column value is " +
			"exactly one of {\"type\":\"null\"}, {\"type\":\"integer\",\"value\":\"<decimal>\"}, " +
			"{\"type\":\"real\",\"value\":<number>}, {\"type\":\"text\",\"value\":\"<string>\"}, and " +
			"{\"type\":\"blob\",\"base64\":\"<base64>\"} — SQLite's five storage classes and nothing else. An " +
			"INTEGER travels as canonical decimal TEXT because IEEE-754 double carries 53 bits of mantissa, so " +
			"9223372036854775807 would arrive as 9223372036854775808 if it were a JSON number; a host MUST reject " +
			"with sql_error an integer outside the 64-bit two's-complement range " +
			"-9223372036854775808..9223372036854775807, and one carrying a leading zero, a leading plus, or " +
			"surrounding space. A BLOB travels as RFC 4648 section 4 base64 with padding, so arbitrary bytes " +
			"survive; TEXT is UTF-8. There is no boolean member because SQLite has no boolean storage class: 0 and " +
			"1 are INTEGERs. lastInsertRowId is the same canonical decimal text for the same reason. " +
			"Every statement reports the same result whether it ran alone or inside a transaction: rows, the rows " +
			"it returned in order and empty when it returned none; rowsWritten, the number of rows it inserted, " +
			"updated, or deleted; and lastInsertRowId when it inserted at least one row. Parameters bind " +
			"POSITIONALLY, so ?1 is params[0], and a statement referencing an index the array does not carry fails " +
			"with sql_error. " +
			"A transaction applies its ordered statements atomically under serializable isolation: it observes one " +
			"consistent snapshot, and either every statement applied or none did. A failure anywhere rolls the " +
			"whole transaction back, leaves the database exactly as it was, and reports sql_error without partial " +
			"results — a result list describing statements whose effects were discarded would record a state that " +
			"never existed. Concurrent writers may fail with busy, which is the only retryable outcome; a caller " +
			"that retries re-runs the whole call. query is declared idempotent, which is only true if it cannot " +
			"write, so a host MUST refuse a writing statement submitted through query with sql_error.",
		Semantics: InterfaceSemantics{Consistency: "serializable"},
		Limits: map[string]int64{
			"maxStatementBytes": 100000, "maxBoundParameters": 100,
			"maxStatementsPerTransaction": 100, "maxRowsPerStatement": 10000,
			"maxColumnsPerRow": 256,
		},
		Operations: []InterfaceOperation{
			{
				Name: "execute",
				Description: "Run one statement with positionally bound parameters and return its result. A statement that " +
					"returns rows returns them here too; a statement that writes reports rowsWritten and, when it inserted at " +
					"least one row, lastInsertRowId as canonical decimal text. execute is deliberately NOT idempotent: " +
					"re-running an INSERT inserts again, so a caller that retries after a timeout must decide for itself " +
					"whether the first attempt applied.",
				InputSchema:  operationObject([]string{"sql"}, map[string]any{"sql": sql, "params": sqlParams()}),
				OutputSchema: withDialect(statementResult),
				Errors:       []string{"sql_error", "busy", "backend_unavailable"},
			},
			{
				Name: "query",
				Description: "Run one READ statement with positionally bound parameters and return its rows. A host MUST " +
					"refuse a statement that writes with sql_error: this operation is declared idempotent, and a writing " +
					"statement behind that declaration would let a safe retry duplicate the write. rowsWritten is therefore " +
					"always 0 here and lastInsertRowId never appears.",
				InputSchema:  operationObject([]string{"sql"}, map[string]any{"sql": sql, "params": sqlParams()}),
				OutputSchema: withDialect(statementResult),
				Errors:       []string{"sql_error", "busy", "backend_unavailable"},
				Idempotent:   true,
			},
			{
				Name: "transaction",
				Description: "Apply an ordered statement list atomically under serializable isolation and return one result " +
					"per statement, in statement order. results carries exactly as many entries as statements, and each entry " +
					"is the full statement result — including the rows a SELECT inside the transaction returned, which is why " +
					"a read-modify-write no longer has to leave the transaction to see what it read.",
				InputSchema: operationObject([]string{"statements"}, map[string]any{
					"statements": map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": sqlStatement()},
				}),
				OutputSchema: operationObject([]string{"results"}, map[string]any{
					"results": map[string]any{"type": "array", "maxItems": 100, "items": statementResult},
				}),
				Errors: []string{"sql_error", "busy", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "create-insert-select",
				Steps: []InterfaceFixtureStep{
					{Operation: "execute", Input: map[string]any{
						"sql": "CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL)",
					}, Expected: map[string]any{"rows": []any{}, "rowsWritten": 0}},
					{Operation: "execute", Input: map[string]any{
						"sql":    "INSERT INTO notes (body) VALUES (?1)",
						"params": []any{sqlText("first note")},
					}, Expected: map[string]any{"rowsWritten": 1, "lastInsertRowId": "1"}},
					{Operation: "query", Input: map[string]any{
						"sql": "SELECT body FROM notes ORDER BY id",
					}, Expected: map[string]any{
						"rows":        []any{map[string]any{"body": sqlText("first note")}},
						"rowsWritten": 0,
					}},
				},
			},
			{
				// The whole reason integers are tagged decimal text. As a JSON
				// number this value comes back as 9223372036854775808.
				Name: "a-64-bit-integer-round-trips-losslessly",
				Steps: []InterfaceFixtureStep{
					{Operation: "execute", Input: map[string]any{"sql": "CREATE TABLE counters (v INTEGER NOT NULL)"}},
					{Operation: "execute", Input: map[string]any{
						"sql":    "INSERT INTO counters (v) VALUES (?1)",
						"params": []any{sqlInteger("9223372036854775807")},
					}, Expected: map[string]any{"rowsWritten": 1}},
					{Operation: "query", Input: map[string]any{"sql": "SELECT v FROM counters"}, Expected: map[string]any{
						"rows": []any{map[string]any{"v": sqlInteger("9223372036854775807")}},
					}},
				},
			},
			{
				// The other value the untagged union could not carry at all.
				Name: "a-blob-round-trips-losslessly",
				Steps: []InterfaceFixtureStep{
					{Operation: "execute", Input: map[string]any{"sql": "CREATE TABLE payloads (v BLOB NOT NULL)"}},
					{Operation: "execute", Input: map[string]any{
						"sql":    "INSERT INTO payloads (v) VALUES (?1)",
						"params": []any{sqlBlob("AAECg/8=")},
					}, Expected: map[string]any{"rowsWritten": 1}},
					{Operation: "query", Input: map[string]any{"sql": "SELECT v FROM payloads"}, Expected: map[string]any{
						"rows": []any{map[string]any{"v": sqlBlob("AAECg/8=")}},
					}},
				},
			},
			{
				Name: "null-and-real-keep-their-storage-classes",
				Steps: []InterfaceFixtureStep{
					{Operation: "execute", Input: map[string]any{"sql": "CREATE TABLE readings (a REAL, b TEXT)"}},
					{Operation: "execute", Input: map[string]any{
						"sql":    "INSERT INTO readings (a, b) VALUES (?1, ?2)",
						"params": []any{sqlReal(1.5), sqlNull()},
					}, Expected: map[string]any{"rowsWritten": 1}},
					{Operation: "query", Input: map[string]any{"sql": "SELECT a, b FROM readings"}, Expected: map[string]any{
						"rows": []any{map[string]any{"a": sqlReal(1.5), "b": sqlNull()}},
					}},
				},
			},
			{
				Name: "a-transaction-reports-one-result-per-statement",
				Steps: []InterfaceFixtureStep{
					{Operation: "transaction", Input: map[string]any{
						"statements": []any{
							map[string]any{"sql": "CREATE TABLE events (id INTEGER PRIMARY KEY, n INTEGER NOT NULL)"},
							map[string]any{
								"sql":    "INSERT INTO events (n) VALUES (?1)",
								"params": []any{sqlInteger("7")},
							},
							map[string]any{"sql": "SELECT n FROM events ORDER BY id"},
						},
					}, Expected: map[string]any{
						"results": []any{
							map[string]any{"rows": []any{}, "rowsWritten": 0},
							map[string]any{"rows": []any{}, "rowsWritten": 1, "lastInsertRowId": "1"},
							map[string]any{"rows": []any{map[string]any{"n": sqlInteger("7")}}, "rowsWritten": 0},
						},
					}},
				},
			},
			{
				// Atomicity is provable from one client: the table the first
				// statement created must not exist after the transaction failed.
				Name: "a-failed-transaction-applies-nothing",
				Steps: []InterfaceFixtureStep{
					{Operation: "transaction", Input: map[string]any{
						"statements": []any{
							map[string]any{"sql": "CREATE TABLE rolled_back (id INTEGER PRIMARY KEY)"},
							map[string]any{"sql": "INSERT INTO no_such_table (id) VALUES (1)"},
						},
					}, ExpectedError: "sql_error"},
					{Operation: "query", Input: map[string]any{
						"sql": "SELECT id FROM rolled_back",
					}, ExpectedError: "sql_error"},
				},
			},
		},
	}
}

// edgeQueueInterface is the at-least-once queue.
//
// It used to state only the producer half, and even that left the message model
// open: a body was an unbounded JSON string, nothing said what a messageId or a
// timestamp meant, nothing said how a delivery was settled, and nothing said
// whether a queue could have two consumers. Every one of those is now data or a
// normative sentence, and the answers are held to the QueueConsumer Form's
// fields and to the queue-producer binding's projection, which still grants only
// send and sendBatch.
func edgeQueueInterface() InterfaceDefinition {
	body := encodedBytes(queueMaxMessageBytes)
	delay := map[string]any{"type": "integer", "minimum": 0, "maximum": 43200}
	messageID := stringSchema(1, 256)
	batchID := stringSchema(1, 256)
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "edge.queue", Version: "1.0.0",
		Title: "At-least-once queue",
		Description: "Asynchronous message submission and batch consumption with at-least-once delivery and no " +
			"ordering guarantee. A message body is OPAQUE BYTES in the family's encoded-bytes shape " +
			"{\"encoding\": \"base64\", \"data\": \"…\"}; maxMessageBytes bounds the DECODED length and a larger " +
			"message fails with message_too_large. A messageId is host-issued, opaque, unique within the queue " +
			"for the lifetime of the message, and STABLE across redeliveries, so a consumer can deduplicate by " +
			"it. timestampMillis is milliseconds since the Unix epoch, in UTC, of the instant the host ACCEPTED " +
			"the message, and it does not change across redeliveries. attempts is 1 on a message's first " +
			"delivery and increments by one on each redelivery. A resolved send means ACCEPTED and durable, " +
			"never delivered: delivery follows later, at least once, possibly more than once, and possibly out " +
			"of send order, so consumers must be idempotent. " +
			"Consumption is not a binding; it is the Queue Consumer attachment, whose maxBatchSize, " +
			"maxBatchTimeoutSeconds, maxRetries, retryDelaySeconds, maxConcurrency, and optional dead-letter " +
			"queue are the operational parameters of every rule here. Settlement is per message or per batch: " +
			"acknowledge settles one message as delivered and it is never redelivered; retry returns one message " +
			"to the queue, delayed by delaySeconds when given and by the consumer's retryDelaySeconds otherwise; " +
			"acknowledgeAll and retryAll settle every message of the batch still unsettled. A handler that " +
			"returns normally without settling anything acknowledges the WHOLE batch. A handler that throws, or " +
			"whose returned promise rejects, retries every message of the batch that was not already explicitly " +
			"acknowledged — acknowledgements taken before the throw stand. " +
			"maxRetries counts REDELIVERIES only: the first delivery does not count toward it, so a message is " +
			"delivered at most 1 + maxRetries times and maxRetries 0 means one delivery and no retry. A message " +
			"that exhausts its retries moves to the consumer's dead-letter queue when it declares one, and is " +
			"dropped otherwise. The dead-letter copy is a NEW message on that queue: new messageId, new " +
			"timestampMillis of the moment it was accepted there, and attempts starting again at 1. This " +
			"contract carries no portable link back to the original. " +
			"One queue has AT MOST ONE consumer. Two would split the stream between two retry policies and two " +
			"dead-letter destinations, leaving the queue's own behavior unstatable, so a host refuses the second " +
			"attachment. " +
			"The fixtures below cover only what one client can prove without a consumer and without waiting; " +
			"delivery, retry, dead-lettering, and attempt counts are HOST OBLIGATIONS stated in the Host API " +
			"specification.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxMessageBytes": queueMaxMessageBytes, "maxBatchMessages": 100, "maxDelaySeconds": 43200,
		},
		Operations: []InterfaceOperation{
			{
				Name: "send",
				Description: "Submit one message, optionally delayed before it becomes deliverable. The body is the " +
					"family's encoded-bytes shape and maxMessageBytes bounds its decoded length. A resolved send means the " +
					"host accepted and durably stored the message; it is not a statement that any consumer has seen it, or " +
					"will see it within any bound this contract states.",
				InputSchema: operationObject([]string{"body"}, map[string]any{
					"body":         body,
					"delaySeconds": delay,
				}),
				OutputSchema: operationObject([]string{"messageId"}, map[string]any{"messageId": messageID}),
				Errors:       []string{"invalid_body", "message_too_large", "backend_unavailable"},
			},
			{
				Name: "sendBatch",
				Description: "Submit an ordered batch of messages; acceptance is all-or-nothing, so a rejected sendBatch " +
					"stored none of them. messageIds is returned in exactly the order the messages were given. Acceptance " +
					"order is not delivery order: the queue guarantees no ordering at all.",
				InputSchema: operationObject([]string{"messages"}, map[string]any{
					"messages": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 100,
						"items": closedObject([]string{"body"}, map[string]any{"body": body, "delaySeconds": delay}),
					},
				}),
				OutputSchema: operationObject([]string{"messageIds"}, map[string]any{
					"messageIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": messageID},
				}),
				Errors: []string{"invalid_body", "message_too_large", "batch_too_large", "backend_unavailable"},
			},
			{
				Name: "acknowledge",
				Description: "Settle one message of the batch this invocation is handling as delivered: it is never " +
					"redelivered and never dead-lettered. batchId is the identity the delivered batch carried and messageId " +
					"one of that batch's messages; anything else fails with unknown_batch or unknown_message. Settling a " +
					"message twice fails with already_settled, so a double acknowledgement is an error rather than a silent " +
					"no-op that hides a bug in the handler.",
				InputSchema: operationObject([]string{"batchId", "messageId"}, map[string]any{
					"batchId": batchID, "messageId": messageID,
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors: []string{
					"unknown_batch", "unknown_message", "already_settled", "backend_unavailable",
				},
			},
			{
				Name: "retry",
				Description: "Return one message of this batch to the queue for redelivery, incrementing its attempts. " +
					"delaySeconds overrides the consumer's retryDelaySeconds for this message only. A message whose attempts " +
					"has already reached 1 + maxRetries is dead-lettered or dropped instead of redelivered, exactly as an " +
					"unhandled failure would leave it.",
				InputSchema: operationObject([]string{"batchId", "messageId"}, map[string]any{
					"batchId": batchID, "messageId": messageID, "delaySeconds": delay,
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors: []string{
					"unknown_batch", "unknown_message", "already_settled", "backend_unavailable",
				},
			},
			{
				Name: "acknowledgeAll",
				Description: "Settle every still-unsettled message of this batch as delivered. Messages already settled by " +
					"acknowledge or retry keep the outcome they were given; this operation never reverses one.",
				InputSchema:  operationObject([]string{"batchId"}, map[string]any{"batchId": batchID}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"unknown_batch", "backend_unavailable"},
			},
			{
				Name: "retryAll",
				Description: "Return every still-unsettled message of this batch to the queue, incrementing each attempts. " +
					"It is exactly what an uncaught handler exception does, stated as an operation so a handler that caught " +
					"its own error can choose the same outcome deliberately.",
				InputSchema: operationObject([]string{"batchId"}, map[string]any{
					"batchId": batchID, "delaySeconds": delay,
				}),
				OutputSchema: operationObject(nil, map[string]any{}),
				Errors:       []string{"unknown_batch", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "send-accepted",
				Steps: []InterfaceFixtureStep{
					{Operation: "send", Input: map[string]any{
						"body": base64Value("eyJldmVudCI6InVzZXIuY3JlYXRlZCJ9"),
					}},
				},
			},
			{
				Name: "send-batch-accepted",
				Steps: []InterfaceFixtureStep{
					{Operation: "sendBatch", Input: map[string]any{
						"messages": []any{
							map[string]any{"body": base64Value("Zmlyc3Q=")},
							map[string]any{"body": base64Value("c2Vjb25k"), "delaySeconds": 30},
						},
					}},
				},
			},
			{
				// Settlement is scoped to a batch this caller is handling, so a
				// batch nothing delivered is refusable from one client with no
				// consumer and no waiting.
				Name: "settling-an-undelivered-batch-is-refused",
				Steps: []InterfaceFixtureStep{
					{Operation: "acknowledge", Input: map[string]any{
						"batchId": "batch-that-was-never-delivered", "messageId": "message-1",
					}, ExpectedError: "unknown_batch"},
					{Operation: "retryAll", Input: map[string]any{
						"batchId": "batch-that-was-never-delivered",
					}, ExpectedError: "unknown_batch"},
				},
			},
		},
	}
}

// serviceMaxBodyBytes is the portable minimum total body every conforming host
// accepts in each direction of a worker.service call. It is a FLOOR: a host
// that accepts more has not made the excess portable, and a host may not accept
// less.
const (
	serviceMaxBodyBytes    = 104857600
	serviceMaxHeaders      = 64
	serviceMaxHeaderBytes  = 16384
	serviceMaxPathBytes    = 8192
	serviceHeaderSlotCount = 128
)

// workerServiceInterface is worker-to-worker invocation, and it STREAMS.
//
// Its first version declared both bodies as JSON strings while the
// module-worker.service Binding Definition said the projection streams request
// to response in both directions and buffers neither. Both could not be true,
// and buffering a body into a JSON member is the exact defect decision 0020
// exists to prevent: it is the same mistake edge.objects made with a 5 GiB
// ceiling, one order of magnitude down. The contract therefore moves to the
// streaming model the binding already promised — the operation document says
// whether a body exists and what is known of its size, and the bytes travel
// beside it.
//
// A REQUIRED EXACT COUNT would have undone that in one member. The call
// completes at the response HEAD, and a body generated as it is written has no
// byte count then, so a host asked for an exact number at the head could only
// buffer the body to learn one — the defect this contract exists to prevent —
// or invent one. contentLength is therefore nullable in BOTH directions: an
// integer is an exact count the writer knows, null is a count it does not have.
// The request direction is not a lesser case of the same problem, it is the
// same problem: a caller streaming a body it is still producing stands exactly
// where the callee does, and the conformance corpus's own request-stream probe
// sends such a body, so under the first version of this member the corpus could
// not have described the traffic it already sends.
//
// Everything a portable caller can observe about that model is stated, because
// an unstated one is a portability hole rather than a detail: absent against
// empty against unknown, what a declared count obliges, when each stream starts,
// backpressure, cancellation, the two aborts, the floors, what a callee
// exception produces, and what a call that never happened produces. The
// published meta-schema has a member for none of it, so it is normative prose in
// the descriptions and fixtures where a fixture can carry it (decision 0014).
func workerServiceInterface() InterfaceDefinition {
	headers := closedStringMap(serviceHeaderSlotCount)
	// An exact count, or null for a length the writer does not have. The
	// bound still applies to the count it states; an unknown-length body is
	// bounded by maxRequestBytes / maxResponseBytes as its bytes arrive.
	bodyLength := map[string]any{
		"type": []any{"integer", "null"}, "minimum": 0, "maximum": serviceMaxBodyBytes,
	}
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: "worker.service", Version: "1.0.0",
		Title: "Worker-to-worker service invocation",
		Description: "Direct HTTP-shaped invocation of another Module Worker, STREAMING in both directions, with " +
			"read-after-write consistency toward the invoked worker's own state: a response reflects every effect " +
			"the callee completed before responding. The call never leaves the host. " +
			"NEITHER BODY IS A MEMBER OF THIS DOCUMENT. The document states whether a body exists and what is known " +
			"of its size; the bytes travel beside it as a stream, so no side ever buffers a body to produce a value. " +
			"ABSENT, EMPTY AND UNKNOWN ARE THREE STATES. bodyStream false pairs with contentLength 0 and means there " +
			"is no body at all: the callee's request.body is null. bodyStream true means a body exists, and " +
			"contentLength says what is KNOWN of its size — an integer is its exact byte count, null is a count the " +
			"writer does not have. bodyStream true with contentLength 0 is a PRESENT body that ends immediately; " +
			"with null it is a present body of unknown length. No one of the three collapses into another, in " +
			"either direction. " +
			"A COUNT IS NEVER INVENTED AND NEVER LEARNED BY BUFFERING. A side that knows its byte count states it, " +
			"a side that does not states null, and a host MUST NOT read a body to compute a count or replace null " +
			"with one: the call completes at the response head, where a body still being generated has no count to " +
			"give. " +
			"A DECLARED COUNT IS A PROMISE. When contentLength is an integer and the stream delivers a different " +
			"number of bytes — fewer or more — the receiving side observes an ERRORED body, reported as " +
			"request_aborted or response_aborted by which side of the response head it fell on. A host never " +
			"rewrites the head to match the bytes and never truncates the bytes to match the head. Under null there " +
			"is no count to disagree with: end of stream ends the body, and only a transport failure aborts. " +
			"WHEN A STREAM STARTS. The callee is invoked as soon as the request head has arrived, before any body " +
			"byte is read, and the caller's call completes as soon as the response head has arrived, before any " +
			"response byte is read. Neither side waits for the other's end of stream: a caller may still be writing " +
			"while it reads, and a callee may answer without having consumed its request. " +
			"BACKPRESSURE reaches the writer from the reader, end to end: a producer that outruns its consumer is " +
			"suspended, never buffered without bound. " +
			"CANCELLATION propagates. A caller that cancels the response stream cancels the callee's; a callee that " +
			"cancels the request stream fails the caller's remaining writes. Neither is a host fault, a retryable " +
			"condition, or backend_unavailable. " +
			"THE TWO ABORTS ARE DIFFERENT OUTCOMES, because they fall on different sides of the response head. " +
			"request_aborted is a request stream that ended before the callee held the whole body: the callee sees " +
			"an errored request body and MAY still answer with a status. response_aborted is the same fault AFTER " +
			"the status was delivered: the caller holds a status and an errored body, and the status is never " +
			"retroactively rewritten. A caller can therefore tell a truncated answer from an unanswered call. " +
			"LIMITS ARE FLOORS. maxRequestHeaders, maxRequestHeaderBytes, maxRequestBytes and their response " +
			"counterparts are the portable minimum every conforming host accepts, not a ceiling; a request above " +
			"what the host accepts fails request_too_large before the callee is invoked, and an unknown-length body " +
			"that grows past it fails the same way once it does. " +
			"A CALLEE EXCEPTION IS A COMPLETE RESPONSE. An uncaught throw or unhandled rejection in the callee's " +
			"fetch handler is a host-generated 500 with a status, headers and a terminated body — never a hung " +
			"call, never a truncated connection — and this operation SUCCEEDS with it. " +
			"A CALL THAT NEVER HAPPENED IS NOT A 500. When the call could not be dispatched at all — no active " +
			"deployment, no fetch handler, the callee unreachable — there is no status, and the operation FAILS " +
			"backend_unavailable; the binding projects the difference as resolve versus reject. " +
			"Behavior fixtures address a callee answering /health with 200 whatever the method and throwing on " +
			"GET /throw.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write"},
		Limits: map[string]int64{
			"maxRequestBytes":        serviceMaxBodyBytes,
			"maxResponseBytes":       serviceMaxBodyBytes,
			"maxRequestHeaders":      serviceMaxHeaders,
			"maxResponseHeaders":     serviceMaxHeaders,
			"maxRequestHeaderBytes":  serviceMaxHeaderBytes,
			"maxResponseHeaderBytes": serviceMaxHeaderBytes,
		},
		Operations: []InterfaceOperation{
			{
				Name: "fetch",
				Description: "Invoke the target worker's fetch handler with one request and return its response. Both " +
					"documents carry bodyStream and contentLength and no bytes. bodyStream false with contentLength 0 is " +
					"no body. bodyStream true is a body whose contentLength is its exact byte count when the writer knows " +
					"one, null when it does not, so a caller still producing its request, or a callee generating its " +
					"response, states null rather than buffering or inventing a count. An integer is a promise: " +
					"delivering fewer or more bytes than declared is an abort, not a short body. Under null, end of " +
					"stream is the end. The response document is produced at the response HEAD, so status and headers " +
					"are readable while the body is still written — which is why a count cannot be required there. " +
					"request_too_large is refused before the callee is invoked; request_aborted and response_aborted are " +
					"the two truncations and only the second carries a status; backend_unavailable produced none. " +
					"A callee that throws is not an error here: it is a 500 in status.",
				InputSchema: operationObject([]string{"bodyStream", "contentLength", "method", "path"}, map[string]any{
					"method":        map[string]any{"type": "string", "enum": []any{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}},
					"path":          stringSchema(1, serviceMaxPathBytes),
					"headers":       headers,
					"bodyStream":    map[string]any{"type": "boolean"},
					"contentLength": bodyLength,
				}),
				OutputSchema: operationObject([]string{"bodyStream", "contentLength", "status"}, map[string]any{
					"status":        map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
					"headers":       headers,
					"bodyStream":    map[string]any{"type": "boolean"},
					"contentLength": bodyLength,
				}),
				Errors: []string{"request_too_large", "request_aborted", "response_aborted", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				// A call with no body at all, which is the shape a GET takes and
				// the one an "empty body" would be indistinguishable from if the
				// contract had only contentLength to say it with.
				Name: "fetch-roundtrip-carries-no-body",
				Steps: []InterfaceFixtureStep{
					{Operation: "fetch", Input: map[string]any{
						"method": "GET", "path": "/health", "bodyStream": false, "contentLength": 0,
					}, Expected: map[string]any{"status": 200, "bodyStream": true}},
				},
			},
			{
				// A caller that is still producing its body has a length to
				// state, and it is not a number. A trace is where that is a fact
				// rather than a sentence: under a required exact count this step
				// could not have been written, and a host that answers it has
				// dispatched a call whose size nobody knew at the head.
				Name: "request-body-of-unknown-length-is-declarable",
				Steps: []InterfaceFixtureStep{
					{Operation: "fetch", Input: map[string]any{
						"method": "POST", "path": "/health", "bodyStream": true, "contentLength": nil,
					}, Expected: map[string]any{"status": 200, "bodyStream": true}},
				},
			},
			{
				// The callee throws and the CALL still succeeds, with a complete
				// response. A trace is the only place this can be stated as a
				// fact rather than as a sentence: an implementation that hung, or
				// that reported the throw as a failed call, fails this step.
				Name: "callee-uncaught-throw-is-a-complete-500",
				Steps: []InterfaceFixtureStep{
					{Operation: "fetch", Input: map[string]any{
						"method": "GET", "path": "/throw", "bodyStream": false, "contentLength": 0,
					}, Expected: map[string]any{"status": 500, "bodyStream": true}},
				},
			},
		},
	}
}

// runtimeHandlerNames is the closed handler vocabulary the runtime contract
// publishes through the loadModule operation. It is written once here and read
// back through RuntimeHandlers, so the Form's `handlers` enum and the contract
// can never disagree.
var runtimeHandlerNames = []any{"fetch", "scheduled", "queue"}

// runtimeLoadableMediaTypes and runtimeAuxiliaryMediaTypes are the ABI's two
// module classes, read out of the lane's single media-type statement
// (internal/currentformmodel/artifact_media.go) rather than spelled a second
// time here. Their union is exactly what the published artifact-manifest
// schema admits, so a bundle a host commits is a bundle this runtime can load.
var (
	runtimeLoadableMediaTypes  = enumValues(model.LoadableModuleMediaTypes())
	runtimeAuxiliaryMediaTypes = enumValues(model.AuxiliaryModuleMediaTypes())
)

// enumValues renders one closed string set as the []any a JSON Schema enum
// takes.
func enumValues(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
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
			"fetch, scheduled, or queue, and every handler the Worker Version declares MUST exist there " +
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
			"exposes all three handlers, answers GET /health with 200, throws on GET /throw, and throws from its " +
			"queue handler when a message body decodes to exactly the ASCII bytes fail; and fetch-only.js, whose " +
			"default export exposes fetch alone.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxRequestBytes":        104857600,
			"maxQueueBatchMessages":  100,
			"maxQueueMessageBytes":   131072,
			"maxEnvironmentEntries":  256,
			"maxWaitUntilTasks":      32,
			"maxModulesPerBundleSet": 512,
		},
		Operations: []InterfaceOperation{
			{
				Name: RuntimeHandlerOperation,
				Description: "Loads one Worker Bundle into a fresh isolate. mainModule names the ES module whose default " +
					"export carries the handlers, and MUST name one entry of modules. modules is the IMPORTABLE " +
					"module graph; exactly these load: application/javascript+module as an ES module; text/plain as " +
					"decoded UTF-8 text; application/octet-stream as an ArrayBuffer; and application/wasm as a " +
					"COMPILED WebAssembly.Module, never an instance, so imports stay the application's choice. " +
					"auxiliaryModules is what the bundle CARRIES without linking: source-map evidence. Its presence " +
					"is not an error, it is never mainModule, and resolving an import to one fails " +
					"unsupported_media_type. This ABI version loads no application/json. exportedHandlers is the " +
					"subset of the vocabulary the default export exposes as callable own properties; a declared " +
					"handler the module does not export fails handler_not_exported before traffic arrives. The " +
					"declaredHandlers enum IS this ABI's handler vocabulary: anything outside it is refused.",
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
								"mediaType": map[string]any{"type": "string", "enum": runtimeLoadableMediaTypes},
							},
						},
					},
					"auxiliaryModules": map[string]any{
						"type": "array", "maxItems": 512, "uniqueItems": true,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"mediaType", "name"},
							"properties": map[string]any{
								"name":      stringSchema(1, 1024),
								"mediaType": map[string]any{"type": "string", "enum": runtimeAuxiliaryMediaTypes},
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
					"batch carries batchId, the identity every settlement operation of edge.queue is scoped to; queue, the " +
					"portable name of the At-Least-Once Queue it drained; and messages, an ordered array whose entries carry " +
					"id (the host's stable message identity), timestampMillis (the UTC instant the host accepted the " +
					"message, in milliseconds since the Unix epoch), body (the producer's exact bytes in the family's " +
					"encoded-bytes shape), and attempts (1 on first delivery, incremented on each redelivery). All of it is " +
					"defined by edge.queue. Batch size and timing are the consumer's " +
					"maxBatchSize and maxBatchTimeoutSeconds. Returning normally without settling " +
					"anything acknowledges the whole batch. An uncaught throw or unhandled rejection RETRIES every message " +
					"not already explicitly acknowledged, under the maxRetries and retryDelaySeconds of the invoking " +
					"consumer, so duplicates are visible and handlers must be idempotent.",
				InputSchema: operationObject([]string{"batchId", "messages", "queue"}, map[string]any{
					"batchId": stringSchema(1, 256),
					"queue":   stringSchema(1, 63),
					"messages": map[string]any{
						"type": "array", "minItems": 1, "maxItems": 100,
						"items": closedObject([]string{"attempts", "body", "id", "timestampMillis"}, map[string]any{
							"id":              stringSchema(1, 256),
							"timestampMillis": map[string]any{"type": "integer", "minimum": 0},
							"body":            encodedBytes(queueMaxMessageBytes),
							"attempts":        map[string]any{"type": "integer", "minimum": 1},
						}),
					},
				}),
				OutputSchema: empty,
				Errors:       []string{"handler_not_declared", "handler_failed", "backend_unavailable"},
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
						RuntimeHandlerProperty: []any{"fetch", "scheduled", "queue"},
					}, Expected: map[string]any{"exportedHandlers": []any{"fetch", "scheduled", "queue"}}},
					{Operation: RuntimeHandlerOperation, Input: map[string]any{
						"mainModule":           "fetch-only.js",
						"modules":              []any{map[string]any{"name": "fetch-only.js", "mediaType": "application/javascript+module"}},
						RuntimeHandlerProperty: []any{"fetch", "scheduled"},
					}, ExpectedError: "handler_not_exported"},
				},
			},
			{
				// An auxiliary module rides along and is never linked. Both
				// halves are stated, because a runtime that merely tolerated
				// the source map and a runtime that imported it would be
				// indistinguishable from the first step alone.
				Name: "auxiliary-module-is-carried-never-loaded",
				Steps: []InterfaceFixtureStep{
					{Operation: RuntimeHandlerOperation, Input: map[string]any{
						"mainModule":            "index.js",
						RuntimeLoadableProperty: []any{map[string]any{"name": "index.js", "mediaType": "application/javascript+module"}},
						RuntimeAuxiliaryProperty: []any{
							map[string]any{"name": "index.js.map", "mediaType": "application/source-map+json"},
						},
						RuntimeHandlerProperty: []any{"fetch"},
					}, Expected: map[string]any{"exportedHandlers": []any{"fetch", "scheduled", "queue"}}},
					{Operation: RuntimeHandlerOperation, Input: map[string]any{
						"mainModule":            "index.js.map",
						RuntimeLoadableProperty: []any{map[string]any{"name": "index.js", "mediaType": "application/javascript+module"}},
						RuntimeAuxiliaryProperty: []any{
							map[string]any{"name": "index.js.map", "mediaType": "application/source-map+json"},
						},
						RuntimeHandlerProperty: []any{"fetch"},
					}, ExpectedError: "unsupported_media_type"},
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
						"batchId": "conformance-batch-1",
						"queue":   "at-least-once-queue",
						"messages": []any{map[string]any{
							"id": "message-1", "timestampMillis": 0, "attempts": 1,
							"body": base64Value("ZmFpbA=="),
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
