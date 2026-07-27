package formcatalog

import "github.com/tako0614/terraform-provider-takoform/formpackage"

// InterfaceDescriptors derives the portable runtime interface declarations of
// a Form.
//
// The names are author-defined and open. A descriptor states what exists and
// how its non-secret values are filled; it never carries a credential and
// never grants a consumer anything. Creating the record, authorizing it, and
// running its lifecycle stay with the host.
func (k Kind) InterfaceDescriptors() []formpackage.InterfaceDescriptor {
	if len(k.Interfaces) == 0 {
		return nil
	}
	descriptors := make([]formpackage.InterfaceDescriptor, 0, len(k.Interfaces))
	for _, declared := range k.Interfaces {
		operations := make([]any, 0, len(declared.Operations))
		for _, operation := range declared.Operations {
			operations = append(operations, operation)
		}
		inputs := []formpackage.InterfaceInputDeclaration{
			{Name: "resource", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/id"},
			{Name: "name", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/name"},
		}
		for _, extra := range declared.ExtraInputs {
			inputs = append(inputs, formpackage.InterfaceInputDeclaration{
				Name: extra, Source: formpackage.InterfaceInputSourceOutput, Pointer: "/" + extra,
			})
		}
		descriptors = append(descriptors, formpackage.InterfaceDescriptor{
			Name: declared.Name, Version: "1", Description: declared.Description, Required: true,
			Document: map[string]any{"operations": operations},
			DocumentSchema: map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
				"additionalProperties": false, "required": []string{"operations"},
				"properties": map[string]any{
					"operations": map[string]any{
						"type": "array", "minItems": len(operations), "maxItems": len(operations),
						"uniqueItems": true,
						"items":       map[string]any{"type": "string", "enum": operations},
					},
				},
			},
			Inputs: inputs,
		})
	}
	return descriptors
}
