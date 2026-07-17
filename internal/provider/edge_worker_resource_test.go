package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

func TestEdgeWorkerPlanDoesNotStartRemotePreview(t *testing.T) {
	if _, ok := NewEdgeWorkerResource().(frameworkresource.ResourceWithModifyPlan); ok {
		t.Fatal("EdgeWorker must not start a discarded remote preview during OpenTofu planning")
	}
}

func TestRefreshEdgeWorkerSpecClearsAbsentOptionalFields(t *testing.T) {
	m := edgeWorkerModel{
		Name:              types.StringValue("api"),
		ArtifactPath:      types.StringValue("/old/dist/worker.js"),
		ArtifactURL:       types.StringValue("https://example.com/old-worker.js"),
		ArtifactSHA256:    types.StringValue("sha256:old"),
		CompatibilityDate: types.StringValue("2026-06-29"),
		CompatibilityFlags: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("nodejs_compat"),
		}),
		Profiles: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("workers_bindings"),
		}),
		Connections: testConnectionList(t, "ASSETS", "ObjectBucket/assets", []string{"read"}, "runtime_binding"),
	}
	res := &client.Resource{
		Metadata: client.Metadata{Name: "api", Space: "prod"},
		Spec: map[string]any{
			"name": "api",
		},
	}

	diags := refreshEdgeWorkerSpec(res, &m)
	if diags.HasError() {
		t.Fatalf("refreshEdgeWorkerSpec diagnostics: %v", diags)
	}
	if !m.ArtifactPath.IsNull() {
		t.Fatalf("expected artifact_path to be cleared, got %q", m.ArtifactPath.ValueString())
	}
	if !m.ArtifactURL.IsNull() {
		t.Fatalf("expected artifact_url to be cleared, got %q", m.ArtifactURL.ValueString())
	}
	if !m.ArtifactSHA256.IsNull() {
		t.Fatalf("expected artifact_sha256 to be cleared, got %q", m.ArtifactSHA256.ValueString())
	}
	if !m.CompatibilityDate.IsNull() {
		t.Fatalf("expected compatibility_date to be cleared, got %q", m.CompatibilityDate.ValueString())
	}
	if !m.CompatibilityFlags.IsNull() {
		t.Fatalf("expected compatibility_flags to be cleared")
	}
	if !m.Profiles.IsNull() {
		t.Fatalf("expected profiles to be cleared")
	}
	if !m.Connections.IsNull() {
		t.Fatalf("expected connections to be cleared")
	}
}

func TestEdgeWorkerCreateAcceptsEndpointDefinedProfileTokens(t *testing.T) {
	ctx := context.Background()
	var gotProfiles []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req client.Resource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rawProfiles, ok := req.Spec["profiles"].([]any)
		if !ok {
			t.Errorf("expected profiles list in request, got %#v", req.Spec["profiles"])
		}
		gotProfiles = rawProfiles
		if r.Method == http.MethodPost && r.URL.Path == "/v1/resources/preview" {
			_ = json.NewEncoder(w).Encode(client.PreviewResourceResult{
				Resource:              req,
				PlanDigest:            "sha256:plan",
				SpecDigest:            "sha256:spec",
				ResolutionFingerprint: "sha256:resolution",
			})
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/resources/EdgeWorker/api" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		req.Status = &client.Status{
			Phase: "Ready",
			Resolution: client.Resolution{
				SelectedImplementation: "custom_worker_runtime",
				Target:                 "operator-runtime",
				Locked:                 true,
				Portability:            "portable",
			},
			Outputs: map[string]any{"url": "https://api.example.com"},
		}
		raw, _ := json.Marshal(req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	r := &edgeWorkerResource{
		data: &providerData{
			client:       client.NewCompatibilityFallback(srv.URL, "", srv.Client()),
			forms:        providerReleaseForms(),
			defaultSpace: "prod",
			capabilities: client.ProductCapabilities{
				Resources: map[string]bool{client.KindEdgeWorker: true},
			},
		},
	}
	var schemaResp frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, edgeWorkerModel{
		ID:                 types.StringUnknown(),
		Name:               types.StringValue("api"),
		ArtifactPath:       types.StringValue("/work/dist/worker.js"),
		CompatibilityFlags: types.SetNull(types.StringType),
		Profiles: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("runtime.workers.next"),
			types.StringValue("bindings.custom"),
		}),
		Connections:     types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}),
		Space:           types.StringNull(),
		ResourceVersion: types.StringUnknown(),
		Portability:     types.StringUnknown(),
		Outputs:         types.MapUnknown(types.StringType),
	})
	if diags.HasError() {
		t.Fatalf("plan diagnostics: %v", diags)
	}
	resp := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}
	r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if len(gotProfiles) != 2 {
		t.Fatalf("expected two profile tokens, got %#v", gotProfiles)
	}
	want := map[string]bool{
		"runtime.workers.next": true,
		"bindings.custom":      true,
	}
	for _, got := range gotProfiles {
		value, ok := got.(string)
		if !ok || !want[value] {
			t.Fatalf("unexpected profile token %#v in %#v", got, gotProfiles)
		}
		delete(want, value)
	}
	if len(want) != 0 {
		t.Fatalf("missing profile tokens: %#v", want)
	}
}

func TestVersionedEdgeWorkerUpdateSendsIfMatchFromProviderState(t *testing.T) {
	ctx := context.Background()
	form := providerReleaseForms()[client.KindEdgeWorker]
	var server *httptest.Server
	sawUpdate := false
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case r.Method == http.MethodGet && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/forms":
			_ = json.NewEncoder(w).Encode(map[string]any{"forms": []client.FormAvailability{{
				Identity: form, DefinitionKnown: true, Installed: true, Executable: true,
				Activated: true, AvailableToPrincipal: true, Operations: []string{"update"},
			}}})
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/preview":
			var desired client.Resource
			if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
				t.Fatal(err)
			}
			if desired.Metadata.ResourceVersion != "7" {
				t.Errorf("preview resourceVersion = %q, want 7", desired.Metadata.ResourceVersion)
			}
			_ = json.NewEncoder(w).Encode(client.PreviewResourceResult{
				Resource: desired, Review: client.PreviewReview{PlanDigest: "sha256:plan", SpecDigest: "sha256:spec"},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/EdgeWorker/api":
			sawUpdate = true
			if r.Header.Get("If-Match") != `"7"` {
				t.Errorf("If-Match = %q, want quoted provider state generation 7", r.Header.Get("If-Match"))
			}
			var apply struct {
				client.Resource
				Review client.DeploymentReview `json:"review"`
			}
			if err := json.NewDecoder(r.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			apply.Resource.Metadata.ResourceVersion = "8"
			apply.Resource.Status = &client.Status{Portability: "portable", Outputs: map[string]any{"url": "https://api.example.test"}}
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(apply.Resource)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	formClient := client.New(server.URL, "", server.Client())
	if _, err := formClient.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	r := &edgeWorkerResource{data: &providerData{
		client: formClient, forms: providerReleaseForms(), defaultSpace: "prod",
	}}
	var schemaResp frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, edgeWorkerModel{
		ID:                 types.StringValue("tkrn:prod:EdgeWorker:api"),
		Name:               types.StringValue("api"),
		ArtifactPath:       types.StringValue("/work/dist/worker.js"),
		ArtifactURL:        types.StringNull(),
		ArtifactRef:        types.StringNull(),
		ArtifactSHA256:     types.StringNull(),
		CompatibilityDate:  types.StringNull(),
		CompatibilityFlags: types.SetNull(types.StringType),
		Profiles:           types.SetNull(types.StringType),
		Connections:        types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}),
		Space:              types.StringValue("prod"),
		ResourceVersion:    types.StringValue("7"),
		DriftStatus:        types.StringValue("current"),
		Portability:        types.StringValue("portable"),
		Outputs:            types.MapValueMust(types.StringType, map[string]attr.Value{}),
	})
	if diags.HasError() {
		t.Fatalf("plan diagnostics: %v", diags)
	}
	resp := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, frameworkresource.UpdateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	if !sawUpdate {
		t.Fatal("provider did not execute the versioned update")
	}
	var state edgeWorkerModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if state.ResourceVersion.ValueString() != "8" {
		t.Fatalf("state resource_version = %q, want 8", state.ResourceVersion.ValueString())
	}
}

func testConnectionList(t *testing.T, name, resource string, permissions []string, projection string) types.List {
	t.Helper()
	permissionValues := make([]attr.Value, 0, len(permissions))
	for _, permission := range permissions {
		permissionValues = append(permissionValues, types.StringValue(permission))
	}
	permissionSet, diags := types.SetValue(types.StringType, permissionValues)
	if diags.HasError() {
		t.Fatalf("permission set diagnostics: %v", diags)
	}
	value, diags := types.ObjectValue(resourceConnectionAttrTypes, map[string]attr.Value{
		"name":        types.StringValue(name),
		"resource":    types.StringValue(resource),
		"permissions": permissionSet,
		"projection":  types.StringValue(projection),
	})
	if diags.HasError() {
		t.Fatalf("connection object diagnostics: %v", diags)
	}
	list, diags := types.ListValue(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}, []attr.Value{value})
	if diags.HasError() {
		t.Fatalf("connection list diagnostics: %v", diags)
	}
	return list
}

func TestEdgeWorkerToResourceAcceptsArtifactURLWithDigest(t *testing.T) {
	model := edgeWorkerModel{
		Name:           types.StringValue("api"),
		ArtifactURL:    types.StringValue("https://example.com/releases/api-worker.js"),
		ArtifactSHA256: types.StringValue("sha256:1111111111111111111111111111111111111111111111111111111111111111"),
	}

	resource, _, diags := model.toResource(context.Background(), "prod")
	if diags.HasError() {
		t.Fatalf("toResource diagnostics: %v", diags)
	}
	source, ok := resource.Spec["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected source map, got %#v", resource.Spec["source"])
	}
	if source["artifactUrl"] != "https://example.com/releases/api-worker.js" {
		t.Fatalf("expected artifactUrl to be carried, got %#v", source)
	}
	if source["artifactSha256"] != "sha256:1111111111111111111111111111111111111111111111111111111111111111" {
		t.Fatalf("expected artifactSha256 to be carried, got %#v", source)
	}
}

func TestEdgeWorkerToResourceRejectsInvalidArtifactSources(t *testing.T) {
	cases := []edgeWorkerModel{
		{Name: types.StringValue("api")},
		{
			Name:         types.StringValue("api"),
			ArtifactPath: types.StringValue("/work/dist/worker.js"),
			ArtifactURL:  types.StringValue("https://example.com/releases/api-worker.js"),
		},
		{
			Name:        types.StringValue("api"),
			ArtifactURL: types.StringValue("https://example.com/releases/api-worker.js"),
		},
		{
			Name:           types.StringValue("api"),
			ArtifactURL:    types.StringValue("http://example.com/releases/api-worker.js"),
			ArtifactSHA256: types.StringValue("1111111111111111111111111111111111111111111111111111111111111111"),
		},
	}
	for _, model := range cases {
		_, _, diags := model.toResource(context.Background(), "prod")
		if !diags.HasError() {
			t.Fatalf("expected diagnostics for %#v", model)
		}
	}
}
