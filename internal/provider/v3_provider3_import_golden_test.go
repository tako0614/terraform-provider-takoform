package provider

import (
	"context"
	"encoding/json"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// TestV3Provider3GoldenLocksEveryCurrentImportPath prevents a resource added
// to the 31-resource Provider surface from inheriting an untested import path.
// Short IDs must resolve that resource's exact default ref; canonical JSON
// must bind the exact identity it names. Retained-identity edge cases and the
// malformed grammar corpus remain in TestV3ImportIdentityForms.
func TestV3Provider3GoldenLocksEveryCurrentImportPath(t *testing.T) {
	ctx := context.Background()
	host := newV3FakeHost(t)
	data := newV3TestProviderData(t, host)
	registry := currentformregistry.V3Current()
	codecs := v3Codecs()
	forms := providerV3CurrentForms()
	if len(forms) != 31 {
		t.Fatalf("current Provider 3 projection = %d, want 31", len(forms))
	}

	for _, form := range forms {
		form := form
		ref, err := registry.DefaultCreate(currentformregistry.GroupKind{
			APIVersion: form.Family.APIVersion(), Kind: form.Kind,
		})
		if err != nil {
			t.Fatalf("default ref for %s/%s: %v", form.Family.APIVersion(), form.Kind, err)
		}
		resource := v3Provider3CurrentResourceHarness(t, form, "", data, codecs)
		schemaResponse := v3SchemaOf(t, resource)

		canonicalRaw, err := json.Marshal(v3ImportDocument{
			Space:             "tenant space: blue",
			APIVersion:        ref.APIVersion,
			Kind:              ref.Kind,
			DefinitionVersion: ref.DefinitionVersion,
			SchemaDigest:      ref.SchemaDigest,
			Name:              "imported-resource",
		})
		if err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name      string
			id        string
			wantSpace string
		}{
			{name: "name", id: "imported-resource"},
			{name: "space-name", id: "prod/imported-resource", wantSpace: "prod"},
			{name: "canonical-exact", id: string(canonicalRaw), wantSpace: "tenant space: blue"},
		}
		for _, testCase := range cases {
			t.Run(resource.resourceTypeName()+"/"+testCase.name, func(t *testing.T) {
				response := frameworkresource.ImportStateResponse{
					State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
				}
				resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: testCase.id}, &response)
				if response.Diagnostics.HasError() {
					t.Fatalf("import %q: %v", testCase.id, response.Diagnostics)
				}
				for attribute, want := range map[string]string{
					"name":                    "imported-resource",
					"form_api_version":        ref.APIVersion,
					"form_kind":               ref.Kind,
					"form_definition_version": ref.DefinitionVersion,
					"form_schema_digest":      ref.SchemaDigest,
					"form_package_digest":     ref.PackageDigest,
				} {
					if got := v3StateString(t, ctx, response.State, attribute).ValueString(); got != want {
						t.Fatalf("import %s = %q, want %q", attribute, got, want)
					}
				}
				space := v3StateString(t, ctx, response.State, "space")
				if testCase.wantSpace == "" {
					if !space.IsNull() {
						t.Fatalf("import space = %#v, want null provider-default placeholder", space)
					}
				} else if got := space.ValueString(); got != testCase.wantSpace {
					t.Fatalf("import space = %q, want %q", got, testCase.wantSpace)
				}
			})
		}
	}
}
