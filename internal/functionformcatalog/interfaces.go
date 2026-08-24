package functionformcatalog

import (
	"fmt"
	"regexp"
	"slices"
)

const (
	// InterfaceAPIVersion is the exact Interface Definition vocabulary used by
	// all provider-neutral runtime capability descriptors.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// FunctionRuntimeInterfaceName is the host-provided invocation ABI of a
	// Function identity. It is an Interface, not a provider or runtime name.
	FunctionRuntimeInterfaceName = "function.runtime"
	// FunctionRuntimeEntrypoint is the closed event/attachment entrypoint in
	// function.runtime@1.0.0. FunctionEndpoint carries this same value in its
	// RequiredEntrypoint gate.
	FunctionRuntimeEntrypoint = "http"
	interfaceVersion          = "1.0.0"
	draft2020                 = "https://json-schema.org/draft/2020-12/schema"

	functionRuntimeMaxEventBytes        = 1048576
	functionRuntimeMaxResponseBytes     = 1048576
	functionRuntimeMaxEnvironmentItems  = 256
	functionRuntimeMaxContextBudgetMS   = 900000
	functionRuntimeMaxInvocationIDBytes = 256
)

// InterfaceDefinition mirrors interface-definition-v1alpha1.schema.json.
// These declarations are data-only runtime capability contracts: no endpoint,
// credential, provider, or executable behavior is embedded.
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
	if len(required) != 0 {
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

func functionHTTPHeadersSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        128,
		"propertyNames":        map[string]any{"type": "string", "maxLength": 256},
		"additionalProperties": boundedString(8192),
	}
}

func functionHTTPEventSchema() map[string]any {
	return closedObject([]string{"kind", "method", "url", "headers", "body"}, map[string]any{
		"kind":    map[string]any{"const": FunctionRuntimeEntrypoint},
		"method":  map[string]any{"type": "string", "enum": []any{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}},
		"url":     map[string]any{"type": "string", "pattern": `^https://[^\x00\r\n]+$`, "maxLength": 8192},
		"headers": functionHTTPHeadersSchema(),
		"body":    boundedString(functionRuntimeMaxEventBytes),
	})
}

// functionEventSchema is a discriminated union even though v1beta1 currently
// has one event kind. The branch is intentionally closed: a future event kind
// must arrive with a new exact Interface version and its attachment.
func functionEventSchema() map[string]any {
	return map[string]any{"oneOf": []any{functionHTTPEventSchema()}}
}

func functionContextSchema() map[string]any {
	return closedObject([]string{"invocationId", "remainingTimeMs"}, map[string]any{
		"invocationId": map[string]any{
			"type": "string", "pattern": `^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`,
			"maxLength": functionRuntimeMaxInvocationIDBytes,
		},
		"remainingTimeMs": map[string]any{
			"type": "integer", "minimum": 0, "maximum": functionRuntimeMaxContextBudgetMS,
		},
	})
}

// InterfaceDefinitions lists the Function Family's exact runtime contract in
// stable order. The runtime is a custom capability descriptor; it is not a
// standard-service protocol and has no binding lane in this MVP.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{functionRuntimeInterface()}
}

func functionRuntimeInterface() InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: FunctionRuntimeInterfaceName, Version: interfaceVersion,
		Title: "Function invocation runtime",
		Description: "The exact host-provided invocation ABI for one Function. The main module exports a " +
			"declared handler invoked exactly as handler(event, context); the event vocabulary is a closed " +
			"discriminated union whose only member in this version is kind=http, and context is a closed object " +
			"carrying invocationId and remainingTimeMs. The host awaits an optional promise and returns a bounded " +
			"HTTP-shaped result. The invoking FunctionVersion timeout is the wall-clock budget; remainingTimeMs " +
			"never exceeds that declared budget. The process environment contains only the version's vars, " +
			"required sensitive names, and sealed standard-service projections; credentials and endpoints are never " +
			"part of this Interface Definition. A runtime behavior change requires a new exact Interface version " +
			"and a Form revision.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxEventBytes":        functionRuntimeMaxEventBytes,
			"maxResponseBytes":     functionRuntimeMaxResponseBytes,
			"maxEnvironmentItems":  functionRuntimeMaxEnvironmentItems,
			"maxContextBudgetMs":   functionRuntimeMaxContextBudgetMS,
			"maxInvocationIdBytes": functionRuntimeMaxInvocationIDBytes,
		},
		Operations: []InterfaceOperation{{
			Name: FunctionRuntimeEntrypoint,
			Description: "Invoke the declared named export exactly as handler(event, context) for one HTTP " +
				"event. The host awaits asynchronous completion; an uncaught failure is reported as handler_failed " +
				"rather than leaving the invocation hanging. The handler reads its sealed environment projection " +
				"from the process namespace, not from a third argument.",
			InputSchema: operationObject([]string{"event", "context"}, map[string]any{
				"event":   functionEventSchema(),
				"context": functionContextSchema(),
			}),
			OutputSchema: operationObject([]string{"status", "headers", "body"}, map[string]any{
				"status":  map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
				"headers": functionHTTPHeadersSchema(),
				"body":    boundedString(functionRuntimeMaxResponseBytes),
			}),
			Errors:     []string{"invalid_event", "invalid_context", "handler_failed", "event_too_large", "response_too_large", "backend_unavailable"},
			Idempotent: false,
		}},
		Fixtures: []InterfaceFixture{
			{
				Name: "http-event-returns-response",
				Steps: []InterfaceFixtureStep{{
					Operation: FunctionRuntimeEntrypoint,
					Input: map[string]any{
						"event": map[string]any{
							"kind": "http", "method": "GET", "url": "https://fn.example.invalid/health",
							"headers": map[string]any{}, "body": "",
						},
						"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000},
					},
					Expected: map[string]any{"status": 200, "headers": map[string]any{}, "body": "ok"},
				}},
			},
			{
				Name: "unknown-event-kind-is-rejected",
				Steps: []InterfaceFixtureStep{{
					Operation: FunctionRuntimeEntrypoint,
					Input: map[string]any{
						"event": map[string]any{
							"kind": "schedule", "method": "GET", "url": "https://fn.example.invalid/health",
							"headers": map[string]any{}, "body": "",
						},
						"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000},
					},
					ExpectedError: "invalid_event",
				}},
			},
			{
				Name: "unknown-event-member-is-rejected",
				Steps: []InterfaceFixtureStep{{
					Operation: FunctionRuntimeEntrypoint,
					Input: map[string]any{
						"event": map[string]any{
							"kind": "http", "method": "GET", "url": "https://fn.example.invalid/health",
							"headers": map[string]any{}, "body": "", "unexpected": "no",
						},
						"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000},
					},
					ExpectedError: "invalid_event",
				}},
			},
			{
				Name: "unknown-context-member-is-rejected",
				Steps: []InterfaceFixtureStep{{
					Operation: FunctionRuntimeEntrypoint,
					Input: map[string]any{
						"event": map[string]any{
							"kind": "http", "method": "GET", "url": "https://fn.example.invalid/health",
							"headers": map[string]any{}, "body": "",
						},
						"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000, "unexpected": "no"},
					},
					ExpectedError: "invalid_context",
				}},
			},
		},
	}
}

var (
	interfaceNamePattern        = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)*$`)
	interfaceVersionPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	interfaceOperationPattern   = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,63}$`)
	interfaceErrorCodePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)
	interfaceFixtureNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

const (
	interfaceDescriptionMaxLength          = 4096
	interfaceOperationDescriptionMaxLength = 1024
	interfaceTitleMaxLength                = 160
)

// ValidateInterfaceDefinitions proves the local runtime Interface catalog's
// identity, grammar, operations, and fixture references before rendering.
func ValidateInterfaceDefinitions(definitions []InterfaceDefinition) error {
	names := map[string]struct{}{}
	for _, definition := range definitions {
		if definition.APIVersion != InterfaceAPIVersion || definition.Kind != "InterfaceDefinition" {
			return fmt.Errorf("interface %s has invalid identity %s/%s", definition.Name, definition.APIVersion, definition.Kind)
		}
		if !interfaceNamePattern.MatchString(definition.Name) || len(definition.Name) > 128 {
			return fmt.Errorf("interface %q has invalid name", definition.Name)
		}
		if !interfaceVersionPattern.MatchString(definition.Version) {
			return fmt.Errorf("interface %s has invalid version %q", definition.Name, definition.Version)
		}
		if _, duplicate := names[definition.Name]; duplicate {
			return fmt.Errorf("duplicate interface identity %q", definition.Name)
		}
		names[definition.Name] = struct{}{}
		if len(definition.Title) == 0 || len(definition.Title) > interfaceTitleMaxLength {
			return fmt.Errorf("interface %s title is empty or exceeds %d characters", definition.Name, interfaceTitleMaxLength)
		}
		if len(definition.Description) > interfaceDescriptionMaxLength {
			return fmt.Errorf("interface %s description exceeds %d characters", definition.Name, interfaceDescriptionMaxLength)
		}
		if len(definition.Operations) == 0 || len(definition.Operations) > 64 {
			return fmt.Errorf("interface %s must declare one through 64 operations", definition.Name)
		}
		operations := map[string]InterfaceOperation{}
		for _, operation := range definition.Operations {
			if !interfaceOperationPattern.MatchString(operation.Name) {
				return fmt.Errorf("interface %s declares operation %q outside the published name grammar", definition.Name, operation.Name)
			}
			if _, duplicate := operations[operation.Name]; duplicate {
				return fmt.Errorf("interface %s declares operation %q twice", definition.Name, operation.Name)
			}
			if len(operation.Description) > interfaceOperationDescriptionMaxLength {
				return fmt.Errorf("interface %s operation %s description exceeds %d characters", definition.Name, operation.Name, interfaceOperationDescriptionMaxLength)
			}
			if operation.InputSchema == nil || operation.OutputSchema == nil {
				return fmt.Errorf("interface %s operation %s must declare input and output schemas", definition.Name, operation.Name)
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
		if !slices.Contains([]string{"eventual", "read_after_write", "per_key_linearizable", "serializable"}, definition.Semantics.Consistency) {
			return fmt.Errorf("interface %s declares unsupported consistency %q", definition.Name, definition.Semantics.Consistency)
		}
		if definition.Semantics.Delivery != "" && !slices.Contains([]string{"at_least_once", "exactly_once_effect"}, definition.Semantics.Delivery) {
			return fmt.Errorf("interface %s declares unsupported delivery %q", definition.Name, definition.Semantics.Delivery)
		}
		if definition.Semantics.Ordering != "" && !slices.Contains([]string{"none", "per_key", "total"}, definition.Semantics.Ordering) {
			return fmt.Errorf("interface %s declares unsupported ordering %q", definition.Name, definition.Semantics.Ordering)
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
				if step.ExpectedError != "" {
					if len(step.Expected) != 0 {
						return fmt.Errorf("interface %s fixture %s step %s expects both output and error", definition.Name, fixture.Name, step.Operation)
					}
					if !slices.Contains(operation.Errors, step.ExpectedError) {
						return fmt.Errorf("interface %s fixture %s expects error %q not declared by %s", definition.Name, fixture.Name, step.ExpectedError, step.Operation)
					}
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
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the Function catalog", name)
}
