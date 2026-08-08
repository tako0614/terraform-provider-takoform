package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
)

func TestProviderSpaceAttributesUseTheDedicatedSpaceIDContract(t *testing.T) {
	t.Parallel()

	var providerResponse frameworkprovider.SchemaResponse
	(&takoformProvider{}).Schema(
		context.Background(),
		frameworkprovider.SchemaRequest{},
		&providerResponse,
	)
	providerSpace, ok := providerResponse.Schema.Attributes["space"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("provider space = %T", providerResponse.Schema.Attributes["space"])
	}

	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket Form is unavailable")
	}
	resource := &formResource{kind: kind}
	var resourceResponse frameworkresource.SchemaResponse
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&resourceResponse,
	)
	resourceSpace, ok := resourceResponse.Schema.Attributes["space"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("resource space = %T", resourceResponse.Schema.Attributes["space"])
	}

	var dataSourceResponse frameworkdatasource.SchemaResponse
	NewInterfaceDataSource().Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		&dataSourceResponse,
	)
	dataSourceSpace, ok := dataSourceResponse.Schema.Attributes["space"].(datasourceschema.StringAttribute)
	if !ok {
		t.Fatalf("data-source space = %T", dataSourceResponse.Schema.Attributes["space"])
	}

	for name, configured := range map[string][]validator.String{
		"provider":    providerSpace.Validators,
		"resource":    resourceSpace.Validators,
		"data source": dataSourceSpace.Validators,
	} {
		t.Run(name, func(t *testing.T) {
			assertSpaceValidators(t, configured, "Prod North", false)
			assertSpaceValidators(t, configured, " leading", true)
			assertSpaceValidators(t, configured, "has/slash", true)
		})
	}
}

func TestEffectiveSpacePreservesExactValueAndRejectsInvalidFallback(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		value    types.String
		fallback string
		want     string
		wantErr  bool
	}{
		{
			name:     "explicit embedded whitespace",
			value:    types.StringValue("Prod North"),
			fallback: "fallback",
			want:     "Prod North",
		},
		{
			name:     "provider default embedded whitespace",
			value:    types.StringNull(),
			fallback: "Prod North",
			want:     "Prod North",
		},
		{
			name:     "invalid explicit is not trimmed",
			value:    types.StringValue(" Prod"),
			fallback: "fallback",
			wantErr:  true,
		},
		{
			name:     "invalid fallback",
			value:    types.StringNull(),
			fallback: "Prod/",
			wantErr:  true,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := validatedEffectiveSpace(test.value, test.fallback)
			if (err != nil) != test.wantErr {
				t.Fatalf("validatedEffectiveSpace error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("validatedEffectiveSpace = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProviderConfigureValidatesAndPreservesEnvironmentSpaceID(t *testing.T) {
	t.Setenv(envToken, "")

	for _, test := range []struct {
		name         string
		space        string
		wantErr      bool
		wantRequests int
	}{
		{
			name:    "invalid fallback fails before discovery",
			space:   " leading",
			wantErr: true,
		},
		{
			name:  "embedded whitespace is preserved",
			space: "Prod North",
			// Configure probes both Host API lanes: v1alpha2 succeeds and the
			// v1alpha3 probe records its 404 as the per-lane error.
			wantRequests: 2,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envSpace, test.space)
			// Configure negotiates both lanes CONCURRENTLY (spec/decisions/0018),
			// so the two discovery requests arrive on different goroutines.
			var requests atomic.Int64
			handler := discoveryHandler(t, true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				handler(w, request)
			}))
			defer server.Close()

			candidate := &takoformProvider{}
			var schemaResponse frameworkprovider.SchemaResponse
			candidate.Schema(
				context.Background(),
				frameworkprovider.SchemaRequest{},
				&schemaResponse,
			)
			configType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"endpoint": tftypes.String,
				"space":    tftypes.String,
				"token":    tftypes.String,
			}}
			request := frameworkprovider.ConfigureRequest{
				Config: tfsdk.Config{
					Schema: schemaResponse.Schema,
					Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
						"endpoint": tftypes.NewValue(tftypes.String, server.URL),
						"space":    tftypes.NewValue(tftypes.String, nil),
						"token":    tftypes.NewValue(tftypes.String, nil),
					}),
				},
			}
			var response frameworkprovider.ConfigureResponse
			candidate.Configure(context.Background(), request, &response)

			if response.Diagnostics.HasError() != test.wantErr {
				t.Fatalf(
					"Configure diagnostics error=%t want=%t: %v",
					response.Diagnostics.HasError(),
					test.wantErr,
					response.Diagnostics,
				)
			}
			if got := int(requests.Load()); got != test.wantRequests {
				t.Fatalf("Configure made %d request(s), want %d", got, test.wantRequests)
			}
			if test.wantErr {
				if response.ResourceData != nil || response.DataSourceData != nil {
					t.Fatal("invalid environment SpaceID produced configured provider data")
				}
				return
			}
			data, ok := response.ResourceData.(*providerData)
			if !ok {
				t.Fatalf("configured resource data = %T", response.ResourceData)
			}
			if data.defaultSpace != test.space {
				t.Fatalf(
					"configured default SpaceID = %q, want exact %q",
					data.defaultSpace,
					test.space,
				)
			}
		})
	}
}

func TestResourceStatePreservesSpaceIDExactly(t *testing.T) {
	t.Parallel()

	const space = "Prod North"
	ctx := context.Background()
	declared, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket Form is unavailable")
	}
	forms := providerCandidateForms()
	resource := &formResource{
		kind: declared,
		data: &providerData{
			defaultSpace: "fallback",
			forms:        forms,
		},
	}
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{
		Schema: schemaResponse.Schema,
		Raw: tftypes.NewValue(
			schemaResponse.Schema.Type().TerraformType(ctx),
			nil,
		),
	}
	observed := providerObservedResource(declared.Kind, forms[declared.Kind], "7")
	observed.Metadata.Space = space
	values := formValues{
		Fields:   map[string]attr.Value{},
		Artifact: nullArtifactSourceValues(),
	}
	if diags := resource.setState(
		ctx,
		&state,
		observed.Metadata.Name,
		observed.Spec,
		&observed,
		space,
		values,
		true,
	); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	var stateSpace types.String
	if diags := state.GetAttribute(ctx, path.Root("space"), &stateSpace); diags.HasError() {
		t.Fatalf("read state space: %v", diags)
	}
	if got := stateSpace.ValueString(); got != space {
		t.Fatalf("state SpaceID = %q, want exact %q", got, space)
	}
}

func TestImportIDUsesSpaceIDWithoutConflatingResourceNameGrammar(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		id        string
		wantSpace string
		wantName  string
		wantErr   bool
	}{
		{id: "Prod North/assets", wantSpace: "Prod North", wantName: "assets"},
		{id: "assets", wantName: "assets"},
		{id: " leading/assets", wantErr: true},
		{id: "Prod/assets/extra", wantErr: true},
		{id: "Prod/Assets", wantErr: true},
	} {
		test := test
		t.Run(test.id, func(t *testing.T) {
			space, name, err := splitImportID(test.id)
			if (err != nil) != test.wantErr {
				t.Fatalf("splitImportID(%q) error = %v, wantErr %t", test.id, err, test.wantErr)
			}
			if space != test.wantSpace || name != test.wantName {
				t.Fatalf(
					"splitImportID(%q) = %q/%q, want %q/%q",
					test.id,
					space,
					name,
					test.wantSpace,
					test.wantName,
				)
			}
		})
	}
}

func assertSpaceValidators(
	t *testing.T,
	configured []validator.String,
	value string,
	wantErr bool,
) {
	t.Helper()
	if len(configured) == 0 {
		t.Fatal("Space attribute has no validator")
	}
	request := validator.StringRequest{
		ConfigValue: types.StringValue(value),
		Path:        path.Root("space"),
	}
	var response validator.StringResponse
	for _, candidate := range configured {
		candidate.ValidateString(context.Background(), request, &response)
	}
	if response.Diagnostics.HasError() != wantErr {
		t.Fatalf(
			"Space validators for %q error=%t want=%t diagnostics=%v",
			value,
			response.Diagnostics.HasError(),
			wantErr,
			response.Diagnostics,
		)
	}
}

func TestSpaceIDValidatorAcceptsUnicodeCodePointBoundary(t *testing.T) {
	t.Parallel()
	assertSpaceValidators(
		t,
		[]validator.String{StringSpaceID()},
		strings.Repeat("界", client.SpaceIDMaxLength),
		false,
	)
	assertSpaceValidators(
		t,
		[]validator.String{StringSpaceID()},
		strings.Repeat("界", client.SpaceIDMaxLength+1),
		true,
	)
}
