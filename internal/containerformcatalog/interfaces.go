package containerformcatalog

import (
	"fmt"
	"regexp"
	"slices"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// InterfaceAPIVersion is the exact Interface Definition vocabulary used by
	// all provider-neutral runtime capability descriptors.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// ContainerRuntimeInterfaceName is the host-provided request/runtime ABI
	// of a ContainerService identity. It is not a provider or image name.
	ContainerRuntimeInterfaceName = "container.runtime"
	// ContainerRuntimeEntrypoint is the HTTP entrypoint activated by
	// ContainerEndpoint. Keeping the operation name equal to the attachment
	// gate makes inward activation derivable from this Interface contract.
	ContainerRuntimeEntrypoint = "http"
	interfaceVersion           = "1.0.0"
	draft2020                  = "https://json-schema.org/draft/2020-12/schema"

	containerRuntimeMaxRequestBytes     = 1048576
	containerRuntimeMaxResponseBytes    = 1048576
	containerRuntimeMaxEnvironmentItems = 256
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

func containerRequestSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"method", "path"},
		"properties": map[string]any{
			"method": map[string]any{"type": "string", "enum": []any{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}},
			"path":   map[string]any{"type": "string", "pattern": "^/[^\\x00\\r\\n]{0,2047}$", "maxLength": 2048},
			"headers": map[string]any{
				"type":                 "object",
				"maxProperties":        128,
				"propertyNames":        map[string]any{"type": "string", "maxLength": 256},
				"additionalProperties": boundedString(8192),
			},
			"body": boundedString(containerRuntimeMaxRequestBytes),
		},
	}
}

func containerEnvironmentSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        containerRuntimeMaxEnvironmentItems,
		"propertyNames":        map[string]any{"type": "string", "pattern": model.PatternBindingName, "maxLength": 64},
		"additionalProperties": boundedString(8192),
	}
}

func containerHeadersSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        128,
		"propertyNames":        map[string]any{"type": "string", "maxLength": 256},
		"additionalProperties": boundedString(8192),
	}
}

// InterfaceDefinitions lists the Container Family's exact runtime contract
// in stable order. The runtime is a custom capability descriptor; it is not a
// standard-service protocol and has no binding lane in this MVP.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{containerRuntimeInterface()}
}

func containerRuntimeInterface() InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: ContainerRuntimeInterfaceName, Version: interfaceVersion,
		Title: "Container request runtime",
		Description: "The exact host-provided request ABI for one Container Service. The host starts " +
			"the digest-pinned process, projects declared environment names, and delivers bounded HTTP-shaped " +
			"requests to it. Image execution, networking, certificate handling, and credentials remain host duties " +
			"outside this data-only Interface Definition. A runtime behavior change requires a new exact Interface " +
			"version and a Form revision.",
		Semantics: InterfaceSemantics{Consistency: "read_after_write", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxRequestBytes":     containerRuntimeMaxRequestBytes,
			"maxResponseBytes":    containerRuntimeMaxResponseBytes,
			"maxEnvironmentItems": containerRuntimeMaxEnvironmentItems,
		},
		Operations: []InterfaceOperation{{
			Name: ContainerRuntimeEntrypoint,
			Description: "Deliver one HTTP-shaped request to the ready container process with its sealed " +
				"environment projection. The host awaits the response; an unavailable or failed process is " +
				"reported as a runtime error rather than hanging the request.",
			InputSchema: operationObject([]string{"request", "environment"}, map[string]any{
				"request":     containerRequestSchema(),
				"environment": containerEnvironmentSchema(),
			}),
			OutputSchema: operationObject([]string{"status", "headers", "body"}, map[string]any{
				"status":  map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
				"headers": containerHeadersSchema(),
				"body":    boundedString(containerRuntimeMaxResponseBytes),
			}),
			Errors:     []string{"process_unavailable", "request_too_large", "response_too_large", "backend_unavailable"},
			Idempotent: false,
		}},
		Fixtures: []InterfaceFixture{{
			Name: "request-returns-response",
			Steps: []InterfaceFixtureStep{{
				Operation: ContainerRuntimeEntrypoint,
				Input: map[string]any{
					"request":     map[string]any{"method": "GET", "path": "/health"},
					"environment": map[string]any{"LOG_LEVEL": "info"},
				},
				Expected: map[string]any{"status": 200, "headers": map[string]any{}, "body": "ok"},
			}},
		}},
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
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the Container catalog", name)
}
