package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// TestImportThenReadPopulatesEveryDeclaredField proves an imported resource
// arrives with the state a plan can compare against.
//
// Import writes only the identity; the read that follows is what adopts the
// host's desired state. If it does not, every field looks unset and the next
// plan proposes to rewrite a resource that is already correct.
func TestImportThenReadPopulatesEveryDeclaredField(t *testing.T) {
	for _, kind := range formcatalog.Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			desired := kind.CanonicalDesired()
			form := providerCandidateForms()[kind.Kind]
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/takoform" {
					writeProviderDiscovery(t, w, srv.URL)
					return
				}
				res := client.Resource{
					APIVersion: client.APIVersion, Kind: kind.Kind, Form: ptrForm(form),
					Metadata: client.Metadata{Name: kind.FixtureName(), Space: "prod", ResourceVersion: "1"},
					Spec:     jsonRoundTrip(t, desired),
					Status:   &client.Status{Portability: "portable"},
				}
				w.Header().Set("ETag", `"1"`)
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode(res)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"resource": res, "observation": map[string]any{"status": "current"},
				})
			}))
			defer srv.Close()

			resource := &formResource{kind: kind, data: &providerData{
				client: mustDiscoveredProviderClient(t, srv), forms: providerCandidateForms(), defaultSpace: "prod",
			}}
			ctx := context.Background()
			var schemaResponse frameworkresource.SchemaResponse
			resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
			empty := tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)}

			importResponse := frameworkresource.ImportStateResponse{State: empty}
			resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: "prod/" + kind.FixtureName()}, &importResponse)
			if importResponse.Diagnostics.HasError() {
				t.Fatalf("import: %v", importResponse.Diagnostics)
			}
			readResponse := frameworkresource.ReadResponse{State: importResponse.State}
			resource.Read(ctx, frameworkresource.ReadRequest{State: importResponse.State}, &readResponse)
			if readResponse.Diagnostics.HasError() {
				t.Fatalf("read: %v", readResponse.Diagnostics)
			}

			for _, field := range kind.Fields {
				if _, present := desired[field.Wire]; !present {
					continue
				}
				var value types.String
				var number types.Int64
				var boolean types.Bool
				var set types.Set
				var isNull bool
				switch field.Type {
				case formcatalog.TypeInt:
					readResponse.State.GetAttribute(ctx, path.Root(field.HCL), &number)
					isNull = number.IsNull()
				case formcatalog.TypeBool:
					readResponse.State.GetAttribute(ctx, path.Root(field.HCL), &boolean)
					isNull = boolean.IsNull()
				case formcatalog.TypeStringSet, formcatalog.TypeIntSet:
					readResponse.State.GetAttribute(ctx, path.Root(field.HCL), &set)
					isNull = set.IsNull()
				case formcatalog.TypeStringMap:
					var mapped types.Map
					readResponse.State.GetAttribute(ctx, path.Root(field.HCL), &mapped)
					isNull = mapped.IsNull()
				default:
					readResponse.State.GetAttribute(ctx, path.Root(field.HCL), &value)
					isNull = value.IsNull()
				}
				if isNull {
					t.Errorf("imported %s left %s unset in state", kind.Kind, field.HCL)
				}
			}
			if kind.Artifact {
				var ref, url, localPath types.String
				readResponse.State.GetAttribute(ctx, path.Root("artifact_ref"), &ref)
				readResponse.State.GetAttribute(ctx, path.Root("artifact_url"), &url)
				readResponse.State.GetAttribute(ctx, path.Root("artifact_path"), &localPath)
				if ref.IsNull() && url.IsNull() && localPath.IsNull() {
					t.Errorf("imported %s left its artifact source unset in state", kind.Kind)
				}
			}
			if kind.Connections == formcatalog.ConnectionsRequired {
				var connections types.List
				readResponse.State.GetAttribute(ctx, path.Root("connections"), &connections)
				if connections.IsNull() {
					t.Errorf("imported %s left its required connections unset in state", kind.Kind)
				}
			}
		})
	}
}

// TestReadAdoptsOutOfBandChange proves a field a host reports differently is
// visible in state, so the next plan can show the drift instead of hiding it
// behind the drift_status flag.
func TestReadAdoptsOutOfBandChange(t *testing.T) {
	kind, ok := formcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	form := providerCandidateForms()[kind.Kind]
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, srv.URL)
			return
		}
		res := client.Resource{
			APIVersion: client.APIVersion, Kind: kind.Kind, Form: ptrForm(form),
			Metadata: client.Metadata{Name: "assets", Space: "prod", ResourceVersion: "1"},
			Spec:     map[string]any{"name": "assets", "storageClass": "archive"},
			Status:   &client.Status{Portability: "portable"},
		}
		w.Header().Set("ETag", `"1"`)
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource": res, "observation": map[string]any{"status": "drifted"},
		})
	}))
	defer srv.Close()

	resource := &formResource{kind: kind, data: &providerData{
		client: mustDiscoveredProviderClient(t, srv), forms: providerCandidateForms(), defaultSpace: "prod",
	}}
	ctx := context.Background()
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)}
	for name, value := range map[string]types.String{
		"name": types.StringValue("assets"), "space": types.StringValue("prod"),
		"storage_class": types.StringValue("standard"), "resource_version": types.StringValue("1"),
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("seed %s: %v", name, diags)
		}
	}

	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	var storageClass types.String
	readResponse.State.GetAttribute(ctx, path.Root("storage_class"), &storageClass)
	if storageClass.ValueString() != "archive" {
		t.Fatalf("storage_class = %q, want the host-observed archive", storageClass.ValueString())
	}
}

func jsonRoundTrip(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
