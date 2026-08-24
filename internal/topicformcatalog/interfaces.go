package topicformcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// InterfaceAPIVersion is the exact Interface Definition vocabulary used by
	// topic.publish. It is separate from the Topic Form family group.
	InterfaceAPIVersion = "interfaces.takoform.com/v1alpha1"
	// TopicPublishInterfaceName is the custom message-publish contract provided
	// by Topic. It is intentionally not a provider or standard-protocol name.
	TopicPublishInterfaceName = "topic.publish"
	draft2020                 = "https://json-schema.org/draft/2020-12/schema"
)

// InterfaceDefinition mirrors interface-definition-v1alpha1.schema.json.
// Keeping the source data-only makes the interface digest reproducible without
// bringing a runtime or provider implementation into this package.
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

func messageBodySchema() map[string]any {
	return map[string]any{
		"oneOf": []any{
			closedObject([]string{"encoding", "data"}, map[string]any{
				"encoding": map[string]any{"type": "string", "const": "utf8"},
				"data": map[string]any{
					"type": "string", "pattern": messageBodyUTF8Pattern, "maxLength": messageBodyDataMaxLength,
				},
			}),
			closedObject([]string{"encoding", "data"}, map[string]any{
				"encoding": map[string]any{"type": "string", "const": "base64"},
				"data": map[string]any{
					"type": "string", "pattern": messageBodyBase64Pattern, "maxLength": messageBodyBase64MaxLen,
				},
			}),
		},
	}
}

func messageAttributesSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"maxProperties":        10,
		"propertyNames":        map[string]any{"type": "string", "pattern": model.PortableMapKeyPattern},
		"additionalProperties": map[string]any{"type": "string", "pattern": messageAttributePattern, "maxLength": messageAttributeMaxLen},
	}
}

func emptyOutput() map[string]any { return operationObject(nil, map[string]any{}) }

// InterfaceDefinitions lists the Topic family's exact Interface contracts in
// stable order.
func InterfaceDefinitions() []InterfaceDefinition {
	return []InterfaceDefinition{topicPublishInterface()}
}

func topicPublishInterface() InterfaceDefinition {
	return InterfaceDefinition{
		APIVersion: InterfaceAPIVersion, Kind: "InterfaceDefinition",
		Name: TopicPublishInterfaceName, Version: "1.0.0",
		Title: "Fanout topic publish",
		Description: "Publish one declarative message to a Topic. An accepted publish " +
			"is delivered at least once to every matching subscription present at " +
			"acceptance; the Topic retains and replays nothing and does not guarantee ordering.",
		Semantics: InterfaceSemantics{Consistency: "eventual", Delivery: "at_least_once", Ordering: "none"},
		Limits: map[string]int64{
			"maxMessageBytes": messageBodyDataMaxLength,
			"maxAttributes":   10,
		},
		Operations: []InterfaceOperation{{
			Name: "publish",
			Description: "Accept one message for fanout. Acceptance is durable only as the " +
				"fanout decision; a subscription's independent delivery may retry or dead-letter later.",
			InputSchema: operationObject([]string{"body", "attributes"}, map[string]any{
				"body":       messageBodySchema(),
				"attributes": messageAttributesSchema(),
			}),
			OutputSchema: emptyOutput(),
			Errors:       []string{"invalid_body", "message_too_large", "backend_unavailable"},
		}},
		Fixtures: []InterfaceFixture{
			{
				Name: "publish-accepted",
				Steps: []InterfaceFixtureStep{{
					Operation: "publish",
					Input: map[string]any{
						"body":       map[string]any{"encoding": "utf8", "data": "order.created"},
						"attributes": map[string]any{"eventType": "order.created"},
					},
				}},
			},
			{
				Name: "invalid-body-rejected",
				Steps: []InterfaceFixtureStep{{
					Operation:     "publish",
					Input:         map[string]any{"body": map[string]any{"encoding": "unknown", "data": "x"}},
					ExpectedError: "invalid_body",
				}},
			},
		},
	}
}

var (
	interfaceNamePattern          = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)*$`)
	interfaceVersionPattern       = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	interfaceOperationNamePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,63}$`)
	interfaceErrorCodePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)
	interfaceFixtureNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

// ValidateInterfaceDefinitions proves the local Interface catalog's closed
// identity and operation/fixture vocabulary before bytes are rendered.
func ValidateInterfaceDefinitions(definitions []InterfaceDefinition) error {
	if len(definitions) == 0 || len(definitions) > 64 {
		return fmt.Errorf("topic interface catalog contains %d definitions, require 1 through 64", len(definitions))
	}
	seenNames := map[string]struct{}{}
	for _, definition := range definitions {
		if definition.APIVersion != InterfaceAPIVersion || definition.Kind != "InterfaceDefinition" {
			return fmt.Errorf("interface %s has invalid identity %s/%s", definition.Name, definition.APIVersion, definition.Kind)
		}
		if !interfaceNamePattern.MatchString(definition.Name) || len(definition.Name) > 128 {
			return fmt.Errorf("interface %s has invalid name", definition.Name)
		}
		if !interfaceVersionPattern.MatchString(definition.Version) {
			return fmt.Errorf("interface %s has invalid version %q", definition.Name, definition.Version)
		}
		if _, duplicate := seenNames[definition.Name+"@"+definition.Version]; duplicate {
			return fmt.Errorf("duplicate interface identity %s@%s", definition.Name, definition.Version)
		}
		seenNames[definition.Name+"@"+definition.Version] = struct{}{}
		if definition.Title == "" || len(definition.Title) > 160 {
			return fmt.Errorf("interface %s title is empty or too long", definition.Name)
		}
		if len(definition.Description) > 4096 {
			return fmt.Errorf("interface %s description exceeds 4096 characters", definition.Name)
		}
		if definition.Semantics.Consistency == "" {
			return fmt.Errorf("interface %s declares no consistency semantics", definition.Name)
		}
		if !slices.Contains([]string{"eventual", "read_after_write", "per_key_linearizable", "serializable"}, definition.Semantics.Consistency) {
			return fmt.Errorf("interface %s declares unknown consistency %q", definition.Name, definition.Semantics.Consistency)
		}
		if !slices.Contains([]string{"", "none", "cursor"}, definition.Semantics.Pagination) {
			return fmt.Errorf("interface %s declares unknown pagination %q", definition.Name, definition.Semantics.Pagination)
		}
		if !slices.Contains([]string{"", "at_least_once", "exactly_once_effect"}, definition.Semantics.Delivery) {
			return fmt.Errorf("interface %s declares unknown delivery %q", definition.Name, definition.Semantics.Delivery)
		}
		if !slices.Contains([]string{"", "none", "per_key", "total"}, definition.Semantics.Ordering) {
			return fmt.Errorf("interface %s declares unknown ordering %q", definition.Name, definition.Semantics.Ordering)
		}
		if len(definition.Operations) == 0 || len(definition.Operations) > 64 {
			return fmt.Errorf("interface %s declares %d operations, require 1 through 64", definition.Name, len(definition.Operations))
		}
		operations := map[string]InterfaceOperation{}
		for _, operation := range definition.Operations {
			if !interfaceOperationNamePattern.MatchString(operation.Name) {
				return fmt.Errorf("interface %s declares operation %q outside the published name grammar", definition.Name, operation.Name)
			}
			if _, duplicate := operations[operation.Name]; duplicate {
				return fmt.Errorf("interface %s declares operation %q twice", definition.Name, operation.Name)
			}
			if len(operation.Description) > 1024 || operation.InputSchema == nil || operation.OutputSchema == nil {
				return fmt.Errorf("interface %s operation %s has invalid description or schemas", definition.Name, operation.Name)
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
		for _, fixture := range definition.Fixtures {
			if !interfaceFixtureNamePattern.MatchString(fixture.Name) {
				return fmt.Errorf("interface %s fixture %q is outside the published name grammar", definition.Name, fixture.Name)
			}
			if len(fixture.Steps) == 0 {
				return fmt.Errorf("interface %s fixture %s has no steps", definition.Name, fixture.Name)
			}
			for _, step := range fixture.Steps {
				operation, declared := operations[step.Operation]
				if !declared || step.Input == nil {
					return fmt.Errorf("interface %s fixture %s has an invalid %s step", definition.Name, fixture.Name, step.Operation)
				}
				if step.ExpectedError != "" && !slices.Contains(operation.Errors, step.ExpectedError) {
					return fmt.Errorf("interface %s fixture %s expects unknown error %q", definition.Name, fixture.Name, step.ExpectedError)
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
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the Topic catalog", name)
}

func marshalIndented(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// RenderedContract is one exact Interface Definition and its digest.
type RenderedContract struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	DefinitionJSON string `json:"definitionJson"`
	SchemaDigest   string `json:"schemaDigest"`
}

// RenderInterfaces renders the custom topic.publish Interface to exact bytes.
func RenderInterfaces() ([]RenderedContract, error) {
	if err := ValidateInterfaceDefinitions(InterfaceDefinitions()); err != nil {
		return nil, err
	}
	out := make([]RenderedContract, 0, len(InterfaceDefinitions()))
	for _, definition := range InterfaceDefinitions() {
		rendered, err := renderInterfaceContract(definition.Name, definition.Version, definition)
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}
	return out, nil
}

// InterfaceRefFor resolves the one exact digest-bound topic InterfaceRef.
func InterfaceRefFor(name, version string) (formpackage.InterfaceRef, error) {
	definition, err := interfaceDefinitionByName(name)
	if err != nil {
		return formpackage.InterfaceRef{}, err
	}
	if definition.Version != version {
		return formpackage.InterfaceRef{}, fmt.Errorf("interface %s is version %s, not %s", name, definition.Version, version)
	}
	rendered, err := renderInterfaceContract(name, version, definition)
	if err != nil {
		return formpackage.InterfaceRef{}, err
	}
	return formpackage.InterfaceRef{
		APIVersion: InterfaceAPIVersion, Name: name, Version: version, SchemaDigest: rendered.SchemaDigest,
	}, nil
}

func renderInterfaceContract(name, version string, definition any) (RenderedContract, error) {
	text, err := marshalIndented(definition)
	if err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	if err := formpackage.ValidateInterfaceDefinition([]byte(text)); err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	digest, err := formpackage.DigestCanonicalJSON([]byte(text))
	if err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	return RenderedContract{Name: name, Version: version, DefinitionJSON: text, SchemaDigest: digest}, nil
}
