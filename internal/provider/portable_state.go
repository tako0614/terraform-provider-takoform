package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

type portableStateProjection struct {
	ID          string
	DriftStatus string
	Portability string
	Outputs     map[string]string
}

// validatePortableStateProjection is the only path from a host-produced
// Resource into provider state. The exact Form schemas close desired,
// observed, and output documents before any value is projected. The returned
// desired spec must also be the RFC 8785-canonical equal of the request (or,
// during Read, the exact current desired state obtained by the preceding
// GET), so host defaults, authority fields, and substitutions cannot silently
// become Terraform state.
func validatePortableStateProjection(
	kind formcatalog.Kind,
	expectedName string,
	expectedSpec map[string]any,
	resource *client.Resource,
) (portableStateProjection, error) {
	if resource == nil {
		return portableStateProjection{}, fmt.Errorf("host returned no Resource")
	}
	if expectedName == "" {
		return portableStateProjection{}, fmt.Errorf("provider has no expected Resource name")
	}
	if resource.Metadata.Name != expectedName {
		return portableStateProjection{}, fmt.Errorf(
			"metadata.name = %q, want requested %q",
			resource.Metadata.Name,
			expectedName,
		)
	}
	if err := validatePortableDesiredSpec(kind, expectedName, expectedSpec, resource.Spec); err != nil {
		return portableStateProjection{}, err
	}
	if resource.Status == nil {
		return portableStateProjection{}, fmt.Errorf("host returned no portable status")
	}
	observed := resource.Status.Observed
	output := resource.Status.Output
	if err := kind.ValidateObserved(observed); err != nil {
		return portableStateProjection{}, err
	}
	if err := kind.ValidateOutput(output); err != nil {
		return portableStateProjection{}, err
	}

	name := resource.Metadata.Name
	canonicalID := resourceIDForKind(kind.Kind, name)
	if got, _ := observed["id"].(string); got != canonicalID {
		return portableStateProjection{}, fmt.Errorf("observed.id = %q, want provider-owned %q", got, canonicalID)
	}
	if got, _ := output["id"].(string); got != canonicalID {
		return portableStateProjection{}, fmt.Errorf("output.id = %q, want provider-owned %q", got, canonicalID)
	}
	if got, _ := output["kind"].(string); got != kind.Kind {
		return portableStateProjection{}, fmt.Errorf("output.kind = %q, want %q", got, kind.Kind)
	}
	if got, _ := output["name"].(string); got != name {
		return portableStateProjection{}, fmt.Errorf("output.name = %q, want %q", got, name)
	}

	resourceGeneration, err := strconv.ParseInt(resource.Metadata.ResourceVersion, 10, 64)
	if err != nil || resourceGeneration < 1 ||
		strconv.FormatInt(resourceGeneration, 10) != resource.Metadata.ResourceVersion {
		return portableStateProjection{}, fmt.Errorf(
			"resourceVersion %q is outside the portable generation range 1..%d",
			resource.Metadata.ResourceVersion,
			formcatalog.MaxPortableGeneration,
		)
	}
	for document, value := range map[string]any{
		"observed": observed["generation"],
		"output":   output["generation"],
	} {
		generation, ok := portableGeneration(value)
		if !ok || generation != resourceGeneration {
			return portableStateProjection{}, fmt.Errorf(
				"%s.generation = %v, want resourceVersion generation %d",
				document,
				value,
				resourceGeneration,
			)
		}
	}

	observedPortability, _ := observed["portability"].(string)
	outputPortability, _ := output["portability"].(string)
	if observedPortability != outputPortability {
		return portableStateProjection{}, fmt.Errorf(
			"observed.portability = %q differs from output.portability = %q",
			observedPortability,
			outputPortability,
		)
	}

	driftedFields, ok := portableStringArray(observed["driftedFields"])
	if !ok {
		// The exact schema should have rejected this first. Keep the projection
		// fail-closed if a validator implementation ever regresses.
		return portableStateProjection{}, fmt.Errorf("observed.driftedFields is not a portable string array")
	}
	driftStatus := "current"
	if len(driftedFields) > 0 {
		driftStatus = "drifted"
	}

	return portableStateProjection{
		ID:          canonicalID,
		DriftStatus: driftStatus,
		Portability: outputPortability,
		Outputs:     outputsToStringMap(output),
	}, nil
}

func validatePortableDesiredSpec(
	kind formcatalog.Kind,
	expectedName string,
	expectedSpec map[string]any,
	returnedSpec map[string]any,
) error {
	specs := []struct {
		label string
		value map[string]any
	}{
		{label: "expected", value: expectedSpec},
		{label: "returned", value: returnedSpec},
	}
	for _, spec := range specs {
		if err := kind.ValidateDesired(spec.value); err != nil {
			return fmt.Errorf("%s spec: %w", spec.label, err)
		}
		name, ok := spec.value["name"].(string)
		if !ok || name != expectedName {
			return fmt.Errorf(
				"%s spec.name = %q, want Resource name %q",
				spec.label,
				name,
				expectedName,
			)
		}
	}

	expectedCanonical, err := canonicalPortableSpec(expectedSpec)
	if err != nil {
		return fmt.Errorf("canonicalize expected spec: %w", err)
	}
	returnedCanonical, err := canonicalPortableSpec(returnedSpec)
	if err != nil {
		return fmt.Errorf("canonicalize returned spec: %w", err)
	}
	if !bytes.Equal(returnedCanonical, expectedCanonical) {
		return fmt.Errorf("returned spec is not canonically exact to requested/current desired state")
	}
	return nil
}

func canonicalPortableSpec(spec map[string]any) ([]byte, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return formpackage.Canonicalize(raw)
}

func portableGeneration(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed > 0
	case int64:
		return typed, typed > 0
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed < 1 {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func portableStringArray(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			values = append(values, text)
		}
		return values, true
	default:
		return nil, false
	}
}
