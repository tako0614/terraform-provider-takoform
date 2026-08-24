package queueformcatalog

import (
	"fmt"
	"regexp"
	"slices"
)

const (
	// InterfaceAPIVersion is the exact Interface Definition vocabulary.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// QueuePullInterfaceName is the pull queue data-plane contract provided by
	// PullQueue. The Form itself carries only this exact reference; providers
	// and hosts own the implementation and any transport.
	QueuePullInterfaceName = "queue.pull"
	// PullQueueInterfaceName is kept as a descriptive alias for callers that
	// name the contract after the Form rather than its operation surface.
	PullQueueInterfaceName = QueuePullInterfaceName

	draft2020 = "https://json-schema.org/draft/2020-12/schema"

	queueMaxMessageBytes      = 262144
	queueMaxMessageAttributes = 10
	queueMaxReceiveMessages   = 10
	queueMaxVisibilitySeconds = 43200
	queueMaxWaitSeconds       = 20
)

// InterfaceDefinition mirrors interface-definition-v1alpha1.schema.json.
// These declarations are data-only operation contracts. They intentionally do
// not name a standard service, endpoint, credential, or provider wire.
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

// encodedBytes is the common canonical bytes shape used by the queue and
// topic proposals. The decoded byte bound is stated in the Interface prose;
// maxLength bounds only the base64 representation admitted by this structural
// schema.
func encodedBytes() map[string]any {
	maxEncoded := 4 * ((queueMaxMessageBytes + 2) / 3)
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"data", "encoding"},
		"properties": map[string]any{
			"encoding": map[string]any{"type": "string", "enum": []any{"base64"}},
			"data":     boundedString(maxEncoded),
		},
	}
}

func messageBody() map[string]any {
	return map[string]any{
		"oneOf": []any{
			boundedString(queueMaxMessageBytes),
			encodedBytes(),
		},
	}
}

func messageAttributes() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        queueMaxMessageAttributes,
		"propertyNames":        boundedString(queueMaxMessageBytes),
		"additionalProperties": boundedString(queueMaxMessageBytes),
	}
}

func messageID() map[string]any { return boundedString(256) }

func receiptHandle() map[string]any { return boundedString(512) }

func queueMessage() map[string]any {
	return closedObject([]string{"messageId", "body", "attributes", "acceptedAt", "receiveCount", "receiptHandle"}, map[string]any{
		"messageId":     messageID(),
		"body":          messageBody(),
		"attributes":    messageAttributes(),
		"acceptedAt":    map[string]any{"type": "integer", "minimum": 0},
		"receiveCount":  map[string]any{"type": "integer", "minimum": 1},
		"receiptHandle": receiptHandle(),
	})
}

func emptyOutput() map[string]any { return operationObject(nil, map[string]any{}) }

// InterfaceDefinitions lists the Queue Family's exact Interface contracts in
// stable order.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{queuePullInterface()}
}

func queuePullInterface() InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: QueuePullInterfaceName, Version: "1.0.0",
		Title: "Pull queue",
		Description: "Unordered at-least-once pull queue. A message body is a UTF-8 string or the " +
			"canonical encoded-bytes object {\"encoding\":\"base64\",\"data\":\"…\"}; at most ten " +
			"string attributes accompany it, and body plus attributes are bounded to 262144 bytes. " +
			"send accepts one message and returns an opaque stable identity; the acceptance timestamp never " +
			"moves. receive returns at most ten messages with their identity, body, attributes, acceptance " +
			"timestamp, receive count (one on first delivery), and a fresh receipt handle. Delivery is at-least-once " +
			"and unordered, so consumers must be idempotent. A message is invisible for the call's visibility " +
			"timeout and is redelivered unless deleted. Each receive invalidates earlier handles. A stale or " +
			"unknown handle is an error, never a silent no-op. An optional dead-letter policy belongs to the " +
			"PullQueue Form; when exhausted, a message moves as a new message with a new identity and acceptance " +
			"timestamp. An empty receive result is a normal response.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxMessageBytes":      queueMaxMessageBytes,
			"maxMessageAttributes": queueMaxMessageAttributes,
			"maxReceiveMessages":   queueMaxReceiveMessages,
			"maxVisibilitySeconds": queueMaxVisibilitySeconds,
			"maxWaitSeconds":       queueMaxWaitSeconds,
		},
		Operations: []InterfaceOperation{
			{
				Name: "send",
				Description: "Accept one message durably. Body and attributes use the queue message shape; " +
					"acceptance does not imply delivery and the returned messageId remains stable across redeliveries.",
				InputSchema: operationObject([]string{"body"}, map[string]any{
					"body":       messageBody(),
					"attributes": messageAttributes(),
				}),
				OutputSchema: operationObject([]string{"messageId"}, map[string]any{"messageId": messageID()}),
				Errors:       []string{"invalid_message", "message_too_large", "backend_unavailable"},
			},
			{
				Name: "receive",
				Description: "Return up to ten currently deliverable messages. Each message receives a fresh " +
					"receipt handle and becomes invisible for visibilityTimeoutSeconds, or the queue default when omitted. " +
					"waitSeconds long-polls up to the queue's declared bound; no messages is an empty successful result.",
				InputSchema: operationObject(nil, map[string]any{
					"visibilityTimeoutSeconds": map[string]any{"type": "integer", "minimum": 0, "maximum": queueMaxVisibilitySeconds},
					"waitSeconds":              map[string]any{"type": "integer", "minimum": 0, "maximum": queueMaxWaitSeconds},
				}),
				OutputSchema: operationObject([]string{"messages"}, map[string]any{
					"messages": map[string]any{
						"type": "array", "maxItems": queueMaxReceiveMessages, "items": queueMessage(),
					},
				}),
				Errors:     []string{"invalid_argument", "backend_unavailable"},
				Idempotent: false,
			},
			{
				Name: "delete",
				Description: "Delete the message represented by the newest valid receipt handle. An invalidated, " +
					"unknown, or expired handle fails with stale_receipt_handle and has no effect.",
				InputSchema:  operationObject([]string{"receiptHandle"}, map[string]any{"receiptHandle": receiptHandle()}),
				OutputSchema: emptyOutput(),
				Errors:       []string{"stale_receipt_handle", "backend_unavailable"},
			},
			{
				Name: "changeVisibility",
				Description: "Set the remaining invisibility of the newest valid receipt handle to a value from " +
					"zero through 43200 seconds. Zero makes the message immediately deliverable; stale handles fail.",
				InputSchema: operationObject([]string{"receiptHandle", "visibilityTimeoutSeconds"}, map[string]any{
					"receiptHandle":            receiptHandle(),
					"visibilityTimeoutSeconds": map[string]any{"type": "integer", "minimum": 0, "maximum": queueMaxVisibilitySeconds},
				}),
				OutputSchema: emptyOutput(),
				Errors:       []string{"stale_receipt_handle", "backend_unavailable"},
			},
		},
		Fixtures: []InterfaceFixture{
			{
				Name: "send-accepted",
				Steps: []InterfaceFixtureStep{{
					Operation: "send",
					Input: map[string]any{
						"body":       "hello",
						"attributes": map[string]any{"kind": "greeting"},
					},
				}},
			},
			{
				Name: "receive-empty-is-success",
				Steps: []InterfaceFixtureStep{{
					Operation: "receive",
					Input:     map[string]any{"waitSeconds": 0},
					Expected:  map[string]any{"messages": []any{}},
				}},
			},
			{
				Name: "stale-handle-is-an-error",
				Steps: []InterfaceFixtureStep{{
					Operation:     "delete",
					Input:         map[string]any{"receiptHandle": "stale"},
					ExpectedError: "stale_receipt_handle",
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
// grammar, operation, and fixture invariants before bytes are rendered.
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
		if definition.Name == "" || definition.Version == "" {
			return fmt.Errorf("interface identity is incomplete")
		}
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
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the Queue catalog", name)
}
