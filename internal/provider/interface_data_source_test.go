package provider

import (
	"context"
	"strings"
	"testing"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestEffectiveInterfaceSpaceIsExplicitOrProviderDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    types.String
		fallback string
		want     string
		wantErr  bool
	}{
		{
			name:     "explicit selector wins",
			value:    types.StringValue("project"),
			fallback: "provider-default",
			want:     "project",
		},
		{
			name:     "null selector uses provider default",
			value:    types.StringNull(),
			fallback: "provider-default",
			want:     "provider-default",
		},
		{
			name:     "empty selector uses provider default",
			value:    types.StringValue(""),
			fallback: "provider-default",
			want:     "provider-default",
		},
		{
			name:    "missing selector and default fails closed",
			value:   types.StringNull(),
			wantErr: true,
		},
		{
			name:     "explicit blank selector does not silently use default",
			value:    types.StringValue("   "),
			fallback: "provider-default",
			wantErr:  true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := effectiveInterfaceSpace(test.value, test.fallback)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("effective Space = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInterfaceDataSourceResourceNameUsesCanonicalPortableGrammar(t *testing.T) {
	t.Parallel()

	var schemaResponse frameworkdatasource.SchemaResponse
	NewInterfaceDataSource().Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		&schemaResponse,
	)
	attribute, ok := schemaResponse.Schema.Attributes["resource_name"].(datasourceschema.StringAttribute)
	if !ok {
		t.Fatalf("resource_name attribute = %T, want schema.StringAttribute", schemaResponse.Schema.Attributes["resource_name"])
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "single lowercase letter", value: "a"},
		{name: "lowercase digit and hyphen", value: "asset-01"},
		{name: "63 character boundary", value: "a" + strings.Repeat("0", 62)},
		{name: "empty", value: "", wantErr: true},
		{name: "uppercase", value: "Assets", wantErr: true},
		{name: "numeric leading", value: "1assets", wantErr: true},
		{name: "underscore", value: "asset_name", wantErr: true},
		{name: "dot", value: "asset.name", wantErr: true},
		{name: "64 characters", value: "a" + strings.Repeat("0", 63), wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := validator.StringRequest{
				ConfigValue: types.StringValue(test.value),
				Path:        path.Root("resource_name"),
			}
			var response validator.StringResponse
			for _, configuredValidator := range attribute.Validators {
				configuredValidator.ValidateString(context.Background(), request, &response)
			}
			if response.Diagnostics.HasError() != test.wantErr {
				t.Fatalf(
					"resource_name %q diagnostics = %v, wantErr %t",
					test.value,
					response.Diagnostics,
					test.wantErr,
				)
			}
		})
	}
}

func TestInterfaceDataSourceProjectsOptionalHostResourceURI(t *testing.T) {
	t.Parallel()

	if got := interfaceResourceURIValue(""); !got.IsNull() {
		t.Fatalf("omitted resource URI = %#v, want null", got)
	}
	const endpoint = "https://api.example.test/"
	if got := interfaceResourceURIValue(endpoint); got.IsNull() || got.ValueString() != endpoint {
		t.Fatalf("resource URI = %#v, want %q", got, endpoint)
	}
}
