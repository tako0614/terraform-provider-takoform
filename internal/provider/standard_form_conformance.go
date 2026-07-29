package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// VerifyStandardFormStructure inspects the actual provider resource schema
// used by this build. This is structural coverage only: it does not run the
// Terraform protocol lifecycle, contact a host, or emit admission evidence.
//
// Because the Terraform schema and the Form Definition are both derived from
// one declaration, this proves the derivation actually happened rather than
// restating the declaration a second time.
func VerifyStandardFormStructure(kind string, desired map[string]any) error {
	name, ok := desired["name"].(string)
	if !ok || !validPortableName(name) || validPortableName("") {
		return fmt.Errorf("provider portable-name validation does not cover canonical positive/negative fixtures for %s", kind)
	}
	declared, ok := formcatalog.ByKind(kind)
	if !ok {
		return fmt.Errorf("provider has no standard resource for %q", kind)
	}
	implementation := NewFormResource(declared)()
	var response resource.SchemaResponse
	implementation.Schema(context.Background(), resource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		return fmt.Errorf("provider schema for %s: %s", kind, response.Diagnostics.Errors()[0].Detail())
	}
	if _, ok := implementation.(resource.ResourceWithImportState); !ok {
		return fmt.Errorf("provider resource %s lacks import", kind)
	}
	for field := range desired {
		for _, providerField := range providerFieldsForDesired(declared, field) {
			if _, ok := response.Schema.Attributes[providerField]; !ok {
				return fmt.Errorf("provider schema for %s lacks %s projected from %s", kind, providerField, field)
			}
		}
	}
	// Every field the definition calls immutable must actually replace the
	// resource. A Form that says "changing this replaces it" and a provider
	// that quietly updates in place are not the same contract.
	for _, pointer := range declared.ImmutableFields() {
		attribute := strings.TrimPrefix(pointer, "/")
		hcl := "name"
		if attribute != "name" {
			field, ok := fieldByWire(declared, attribute)
			if !ok {
				return fmt.Errorf("provider schema for %s declares immutable %s with no field", kind, attribute)
			}
			hcl = field.HCL
		}
		if err := requireReplace(response.Schema.Attributes[hcl], hcl); err != nil {
			return fmt.Errorf("provider schema for %s: %w", kind, err)
		}
	}
	return nil
}

func fieldByWire(kind formcatalog.Kind, wire string) (formcatalog.Field, bool) {
	for _, field := range kind.Fields {
		if field.Wire == wire {
			return field, true
		}
	}
	return formcatalog.Field{}, false
}

func providerFieldsForDesired(kind formcatalog.Kind, field string) []string {
	if field == "source" {
		return []string{"artifact_url", "artifact_sha256", "artifact_media_type"}
	}
	if field == "connections" || field == "name" {
		return []string{field}
	}
	if declared, ok := fieldByWire(kind, field); ok {
		return []string{declared.HCL}
	}
	return []string{camelToSnake(field)}
}

func requireReplace(attribute schema.Attribute, field string) error {
	var modifiers []any
	switch typed := attribute.(type) {
	case schema.StringAttribute:
		for _, modifier := range typed.PlanModifiers {
			modifiers = append(modifiers, modifier)
		}
	case schema.Int64Attribute:
		for _, modifier := range typed.PlanModifiers {
			modifiers = append(modifiers, modifier)
		}
	case schema.BoolAttribute:
		for _, modifier := range typed.PlanModifiers {
			modifiers = append(modifiers, modifier)
		}
	case schema.SetAttribute:
		for _, modifier := range typed.PlanModifiers {
			modifiers = append(modifiers, modifier)
		}
	case schema.ListAttribute:
		for _, modifier := range typed.PlanModifiers {
			modifiers = append(modifiers, modifier)
		}
	case schema.ListNestedAttribute:
		for _, modifier := range typed.PlanModifiers {
			modifiers = append(modifiers, modifier)
		}
	}
	for _, modifier := range modifiers {
		if strings.Contains(strings.ToLower(fmt.Sprintf("%T", modifier)), "requiresreplace") {
			return nil
		}
	}
	return fmt.Errorf("%s lacks RequiresReplace", field)
}

func camelToSnake(value string) string {
	var result strings.Builder
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}
