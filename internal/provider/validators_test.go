package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestMapKeysMatchAcceptsUnknownComputedValues(t *testing.T) {
	t.Parallel()

	configuration := types.MapValueMust(types.StringType, map[string]attr.Value{
		"DELIVERY_QUEUE_NAME": types.StringUnknown(),
		"DELIVERY_DLQ_NAME":   types.StringUnknown(),
	})
	request := validator.MapRequest{
		ConfigValue: configuration,
		Path:        path.Root("configuration"),
	}
	var response validator.MapResponse

	MapKeysMatch(
		formcatalog.PortableMapKeyPattern,
		"configuration keys must use the portable map-key grammar",
	).ValidateMap(context.Background(), request, &response)

	if response.Diagnostics.HasError() {
		t.Fatalf("computed configuration values must remain unknown during validation: %v", response.Diagnostics)
	}
}

func TestMapKeysMatchStillRejectsInvalidKeyWithUnknownValue(t *testing.T) {
	t.Parallel()

	configuration := types.MapValueMust(types.StringType, map[string]attr.Value{
		"not portable": types.StringUnknown(),
	})
	request := validator.MapRequest{
		ConfigValue: configuration,
		Path:        path.Root("configuration"),
	}
	var response validator.MapResponse

	MapKeysMatch(
		formcatalog.PortableMapKeyPattern,
		"configuration keys must use the portable map-key grammar",
	).ValidateMap(context.Background(), request, &response)

	if !response.Diagnostics.HasError() {
		t.Fatal("invalid configuration key was accepted because its value was unknown")
	}
}
