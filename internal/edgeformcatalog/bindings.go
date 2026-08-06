package edgeformcatalog

import (
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// The five typed Binding contracts of the Edge Platform Family (decision
// 0010). A Binding grants a capability and a concrete runtime API together,
// without exposing credentials; outward capability use always belongs to a
// revision resource.

// BindingAPIVersion is the fixed group of typed Binding contracts.
const BindingAPIVersion = "bindings.takoform.com/v1alpha1"

// BindingDefinition mirrors binding-definition-v1alpha1.schema.json.
type BindingDefinition struct {
	APIVersion         string                   `json:"apiVersion"`
	Kind               string                   `json:"kind"`
	Name               string                   `json:"name"`
	Version            string                   `json:"version"`
	Title              string                   `json:"title,omitempty"`
	Description        string                   `json:"description,omitempty"`
	SourceRole         string                   `json:"sourceRole"`
	TargetInterface    formpackage.InterfaceRef `json:"targetInterface"`
	AllowedTargetForms []AllowedTargetForm      `json:"allowedTargetForms"`
	BindingNameGrammar string                   `json:"bindingNameGrammar"`
	RuntimeProjection  RuntimeProjection        `json:"runtimeProjection"`
	Lifecycle          BindingLifecycle         `json:"lifecycle"`
}

type AllowedTargetForm struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

type RuntimeProjection struct {
	Operations []string `json:"operations"`
}

type BindingLifecycle struct {
	TargetDeletion string `json:"targetDeletion"`
}

type bindingSpec struct {
	name        string
	title       string
	description string
	iface       string
	targetKind  string
	operations  []string // empty projects every interface operation
}

var bindingSpecs = []bindingSpec{
	{
		name:  "module-worker.edge-kv",
		title: "Module Worker edge KV binding",
		description: "Projects the complete edge.kv runtime API into a Worker Version under one " +
			"JavaScript-identifier binding name. The binding grants capability and API together; " +
			"no credential or endpoint ever reaches the consumer.",
		iface:      "edge.kv",
		targetKind: "EdgeKVNamespace",
	},
	{
		name:  "module-worker.object-bucket",
		title: "Module Worker object bucket binding",
		description: "Projects the complete edge.objects runtime API into a Worker Version under one " +
			"JavaScript-identifier binding name, without exposing credentials or endpoints.",
		iface:      "edge.objects",
		targetKind: "ObjectBucket",
	},
	{
		name:  "module-worker.sqlite",
		title: "Module Worker SQLite binding",
		description: "Projects the complete edge.sql runtime API into a Worker Version under one " +
			"JavaScript-identifier binding name, without exposing credentials or connection material.",
		iface:      "edge.sql",
		targetKind: "SQLiteDatabase",
	},
	{
		name:  "module-worker.queue-producer",
		title: "Module Worker queue producer binding",
		description: "Projects only the submission half of edge.queue (send, sendBatch) into a Worker " +
			"Version. Consumption is never a binding: it is the QueueConsumer attachment resource.",
		iface:      "edge.queue",
		targetKind: "AtLeastOnceQueue",
		operations: []string{"send", "sendBatch"},
	},
	{
		name:  "module-worker.service",
		title: "Module Worker service binding",
		description: "Projects the worker.service fetch operation into a Worker Version, addressing " +
			"another Module Worker by logical identity without any network endpoint.",
		iface:      "worker.service",
		targetKind: "ModuleWorker",
	},
}

// BindingDefinitions lists the Edge Platform Family binding catalog in a
// stable order. Every targetInterface carries the exact resolved digest of
// the referenced Interface Definition.
func BindingDefinitions() ([]BindingDefinition, error) {
	out := make([]BindingDefinition, 0, len(bindingSpecs))
	for _, spec := range bindingSpecs {
		iface, err := interfaceDefinitionByName(spec.iface)
		if err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.name, err)
		}
		ref, err := InterfaceRefFor(spec.iface, iface.Version)
		if err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.name, err)
		}
		operations := spec.operations
		if len(operations) == 0 {
			operations = make([]string, 0, len(iface.Operations))
			for _, operation := range iface.Operations {
				operations = append(operations, operation.Name)
			}
		} else {
			for _, operation := range operations {
				if !interfaceHasOperation(iface, operation) {
					return nil, fmt.Errorf("binding %s projects unknown %s operation %s", spec.name, spec.iface, operation)
				}
			}
		}
		out = append(out, BindingDefinition{
			APIVersion: BindingAPIVersion, Kind: "BindingDefinition",
			Name: spec.name, Version: "1.0.0",
			Title: spec.title, Description: spec.description,
			SourceRole:      string(currentformmodel.RoleRevision),
			TargetInterface: ref,
			AllowedTargetForms: []AllowedTargetForm{
				{APIVersion: Family.APIVersion(), Kind: spec.targetKind},
			},
			BindingNameGrammar: currentformmodel.PatternBindingName,
			RuntimeProjection:  RuntimeProjection{Operations: operations},
			Lifecycle:          BindingLifecycle{TargetDeletion: "refuse_while_bound"},
		})
	}
	return out, nil
}

func interfaceDefinitionByName(name string) (InterfaceDefinition, error) {
	for _, candidate := range InterfaceDefinitions() {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the catalog", name)
}

func interfaceHasOperation(iface InterfaceDefinition, name string) bool {
	for _, operation := range iface.Operations {
		if operation.Name == name {
			return true
		}
	}
	return false
}
