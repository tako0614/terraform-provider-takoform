package formpackage

import (
	"strings"
	"testing"
)

func definitionWithInterfaces(t *testing.T, descriptors []any) []byte {
	t.Helper()
	closedSchema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
	}
	definition := map[string]any{
		"apiVersion":            FormAPIVersion,
		"kind":                  "InterfaceDeclarationExample",
		"definitionVersion":     "1.0.0",
		"title":                 "Interface declaration example",
		"status":                "compatibility-candidate",
		"desiredSchema":         closedSchema,
		"observedSchema":        closedSchema,
		"lifecycleCapabilities": []any{"create", "observe", "delete"},
		"interfaces":            descriptors,
	}
	return canonicalMarshal(t, definition)
}

func descriptor(name, version string, inputs []any) map[string]any {
	value := map[string]any{"name": name, "version": version}
	if inputs != nil {
		value["inputs"] = inputs
	}
	return value
}

func TestInterfaceDescriptorIdentityIsNameAndVersion(t *testing.T) {
	for _, version := range []string{"1", "v1", "v1alpha1", "2025-11-25", "2025.11"} {
		raw := definitionWithInterfaces(t, []any{descriptor("vendor.custom_protocol", version, nil)})
		definition, err := ValidateDefinition(raw)
		if err != nil {
			t.Fatalf("version %q: %v", version, err)
		}
		if definition.Interfaces[0].Version != version {
			t.Fatalf("version %q did not round-trip: %+v", version, definition.Interfaces)
		}
	}

	// A name is not an identity by itself. Distinct versions may coexist.
	raw := definitionWithInterfaces(t, []any{
		descriptor("mcp.server", "1", nil),
		descriptor("mcp.server", "2", nil),
	})
	if _, err := ValidateDefinition(raw); err != nil {
		t.Fatalf("same name with distinct versions must be valid: %v", err)
	}
}

func TestInterfaceDescriptorRejectsDuplicatePair(t *testing.T) {
	raw := definitionWithInterfaces(t, []any{
		descriptor("mcp.server", "1", nil),
		descriptor("mcp.server", "1", []any{map[string]any{"name": "endpoint", "source": "output"}}),
	})
	_, err := ValidateDefinition(raw)
	if err == nil || !strings.Contains(err.Error(), "duplicate Interface") {
		t.Fatalf("err = %v, want duplicate Interface", err)
	}
}

func TestInterfaceInputMappingGrammar(t *testing.T) {
	tests := []struct {
		name      string
		inputs    []any
		wantError string
	}{
		{
			name: "portable sources namespaced source and RFC6901 pointers",
			inputs: []any{
				map[string]any{"name": "whole_output", "source": "output", "pointer": ""},
				map[string]any{"name": "escaped", "source": "output", "pointer": "/a~1b/~0key"},
				map[string]any{"name": "protocol", "source": "literal", "value": "streamable-http"},
				map[string]any{"name": "nullable", "source": "literal", "value": nil},
				map[string]any{"name": "hint", "source": "example-host.surface_hint"},
			},
		},
		{name: "literal without a value", inputs: []any{map[string]any{"name": "protocol", "source": "literal"}}, wantError: "is a literal without a value"},
		{name: "literal carrying a pointer", inputs: []any{map[string]any{"name": "protocol", "source": "literal", "value": "x", "pointer": "/protocol"}}, wantError: "must not carry a pointer"},
		{name: "non-literal carrying a value", inputs: []any{map[string]any{"name": "endpoint", "source": "output", "value": "x"}}, wantError: "carries a value with source"},
		{
			name: "duplicate input name",
			inputs: []any{
				map[string]any{"name": "endpoint", "source": "output", "pointer": "/a"},
				map[string]any{"name": "endpoint", "source": "output", "pointer": "/b"},
			},
			wantError: "duplicate input",
		},
		{name: "host source that is not namespaced", inputs: []any{map[string]any{"name": "endpoint", "source": "custom"}}, wantError: "'anyOf' failed"},
		{name: "pointer without leading slash", inputs: []any{map[string]any{"name": "endpoint", "source": "output", "pointer": "endpoint"}}, wantError: "pointer"},
		{name: "pointer with dangling escape", inputs: []any{map[string]any{"name": "endpoint", "source": "output", "pointer": "/bad~"}}, wantError: "pointer"},
		{name: "pointer with invalid escape", inputs: []any{map[string]any{"name": "endpoint", "source": "output", "pointer": "/bad~2escape"}}, wantError: "pointer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := definitionWithInterfaces(t, []any{descriptor("mcp.server", "2025-11-25", test.inputs)})
			_, err := ValidateDefinition(raw)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("err = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestInterfaceDocumentMustMatchItsSchema(t *testing.T) {
	documentSchema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
		"required": []any{"title"},
	}

	valid := descriptor("mcp.server", "1", nil)
	valid["documentSchema"] = documentSchema
	valid["document"] = map[string]any{"title": "Portable MCP"}
	definition, err := ValidateDefinition(definitionWithInterfaces(t, []any{valid}))
	if err != nil {
		t.Fatalf("valid document: %v", err)
	}
	if definition.Interfaces[0].Document["title"] != "Portable MCP" {
		t.Fatalf("document did not round-trip: %+v", definition.Interfaces[0].Document)
	}

	for name, mutate := range map[string]func(map[string]any){
		"mismatched explicit document":                               func(value map[string]any) { value["document"] = map[string]any{"title": 42} },
		"omitted document defaults only when empty object validates": func(value map[string]any) { delete(value, "document") },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := descriptor("mcp.server", "1", nil)
			candidate["documentSchema"] = documentSchema
			candidate["document"] = map[string]any{"title": "Portable MCP"}
			mutate(candidate)
			_, err := ValidateDefinition(definitionWithInterfaces(t, []any{candidate}))
			if err == nil || !strings.Contains(err.Error(), "does not satisfy documentSchema") {
				t.Fatalf("err = %v, want documentSchema failure", err)
			}
		})
	}

	allowsEmpty := descriptor("mcp.server", "1", nil)
	allowsEmpty["documentSchema"] = map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"additionalProperties": false, "properties": map[string]any{},
	}
	if _, err := ValidateDefinition(definitionWithInterfaces(t, []any{allowsEmpty})); err != nil {
		t.Fatalf("omitted document may default to an accepted empty object: %v", err)
	}
}

func TestPortableInterfaceInputSource(t *testing.T) {
	for _, source := range []string{InterfaceInputSourceLiteral, InterfaceInputSourceOutput} {
		if !PortableInterfaceInputSource(source) {
			t.Fatalf("%q must be portable", source)
		}
	}
	for _, source := range []string{"capsule_output", "resource_output", "example-host.surface_hint"} {
		if PortableInterfaceInputSource(source) {
			t.Fatalf("%q must not be portable", source)
		}
	}
}

func TestInterfaceDocumentSchemaUsesPortableClosureProof(t *testing.T) {
	open := map[string]any{
		"name": "mcp.server", "version": "1",
		"documentSchema": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
			"properties": map[string]any{"title": map[string]any{"type": "string"}},
		},
	}
	_, err := ValidateDefinition(definitionWithInterfaces(t, []any{open}))
	if err == nil || !strings.Contains(err.Error(), "interfaces[0].documentSchema") {
		t.Fatalf("err = %v, want descriptor location", err)
	}
}
