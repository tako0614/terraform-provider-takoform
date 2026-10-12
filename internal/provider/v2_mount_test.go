package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

func TestV2ProviderRegistersOnlyExactPublishedEdgeForms(t *testing.T) {
	got := make([]string, 0)
	for _, factory := range New("5.0.0")().Resources(context.Background()) {
		var metadata resource.MetadataResponse
		factory().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		got = append(got, metadata.TypeName)
	}
	sort.Strings(got)
	want := []string{
		"takoform_at_least_once_queue", "takoform_edge_kv_namespace", "takoform_edge_object_bucket",
		"takoform_module_worker", "takoform_queue_consumer", "takoform_sqlite_database",
		"takoform_sqlite_migration_application", "takoform_sqlite_migration_set", "takoform_worker_bundle",
		"takoform_worker_cron_trigger", "takoform_worker_deployment", "takoform_worker_endpoint", "takoform_worker_version",
	}
	if len(got) != len(want) {
		t.Fatalf("registered %d resources, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("resource %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func v2TestResource(t *testing.T, data *v2ProviderData, form string) (*v2Resource, resource.SchemaResponse) {
	t.Helper()
	for _, def := range v2Forms {
		if def.form == form {
			candidate := &v2Resource{def: def, data: data}
			var schemaResponse resource.SchemaResponse
			candidate.Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
			if schemaResponse.Diagnostics.HasError() {
				t.Fatalf("v2 schema: %v", schemaResponse.Diagnostics)
			}
			return candidate, schemaResponse
		}
	}
	t.Fatal("missing test Form")
	return nil, resource.SchemaResponse{}
}

func v2RawState(t *testing.T, schemaResponse resource.SchemaResponse) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(context.Background()), nil)
}

func v2TestPlan(t *testing.T, schemaResponse resource.SchemaResponse, name string) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
	if diags := plan.SetAttribute(ctx, path.Root("name"), types.StringValue(name)); diags.HasError() {
		t.Fatal(diags)
	}
	return plan
}

func v2TestData(t *testing.T, server *httptest.Server) *v2ProviderData {
	t.Helper()
	root := server.URL + "/api/v2"
	storePath := filepath.Join(t.TempDir(), "intents")
	if err := intentv2.InitializeBound(storePath, "scope-1", intentv2.StoreBinding{HostAPI: root}); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	store, err := intentv2.Open(storePath, "scope-1")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &v2ProviderData{client: hostv2.New(server.URL, "test-token", server.Client()), store: store, endpoint: server.URL, hostAPIRoot: root, storeID: "scope-1", space: "prod"}
}

func TestV2ModuleWorkerCreateReadUpdateDelete(t *testing.T) {
	var server *httptest.Server
	var uidExists = true
	var generation int64
	var mutationCalls atomic.Int32
	var lastAction string
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch req.URL.Path {
		case hostv2.DiscoveryPath:
			_ = json.NewEncoder(w).Encode(hostv2.Discovery{API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs", Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"}, Capabilities: hostv2.Capabilities{}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600}})
		case "/api/v2/support":
			_ = json.NewEncoder(w).Encode(hostv2.Support{Form: req.URL.Query().Get("form"), Supported: true, Operations: []string{"create", "read", "update", "delete"}, PrivateInputs: false})
		case "/api/v2/resources":
			if req.Method != http.MethodPost || req.Header.Get("Idempotency-Key") == "" {
				t.Errorf("bad Create headers: %s", req.Method)
			}
			mutationCalls.Add(1)
			generation = 1
			uidExists = true
			lastAction = "create"
			v2WriteOperation(t, w, server.URL, lastAction, generation)
		case "/api/v2/resources/uid-1":
			switch req.Method {
			case http.MethodGet:
				if !uidExists {
					v2WriteNotFound(w)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"uid": "uid-1", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "app", "generation": generation, "observedGeneration": generation, "observedAt": nil, "phase": "idle", "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-1"})
			case http.MethodPut:
				if req.Header.Get("Takoform-Expected-Generation") != "1" {
					t.Errorf("bad Update fence")
				}
				mutationCalls.Add(1)
				generation = 2
				lastAction = "update"
				v2WriteOperation(t, w, server.URL, lastAction, generation)
			case http.MethodDelete:
				if req.Header.Get("Takoform-Expected-Generation") != "2" {
					t.Errorf("bad Delete fence")
				}
				mutationCalls.Add(1)
				generation = 3
				uidExists = false
				lastAction = "delete"
				v2WriteOperation(t, w, server.URL, lastAction, generation)
			}
		case "/api/v2/operations/op-1":
			v2WriteOperation(t, w, server.URL, lastAction, generation)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	data := v2TestData(t, server)
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	ctx := context.Background()
	plan := v2TestPlan(t, schemaResponse, "app")
	create := resource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}}
	res.Create(ctx, resource.CreateRequest{Plan: plan}, &create)
	if create.Diagnostics.HasError() {
		t.Fatalf("create: %v", create.Diagnostics)
	}
	read := resource.ReadResponse{State: create.State}
	res.Read(ctx, resource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatalf("read: %v", read.Diagnostics)
	}
	updatePlan := v2TestPlan(t, schemaResponse, "app")
	update := resource.UpdateResponse{State: read.State}
	res.Update(ctx, resource.UpdateRequest{State: read.State, Plan: updatePlan}, &update)
	if update.Diagnostics.HasError() {
		t.Fatalf("update: %v", update.Diagnostics)
	}
	deleteResponse := resource.DeleteResponse{State: update.State}
	res.Delete(ctx, resource.DeleteRequest{State: update.State}, &deleteResponse)
	if deleteResponse.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResponse.Diagnostics)
	}
	if got := mutationCalls.Load(); got != 3 {
		t.Fatalf("mutation calls = %d, want 3", got)
	}
}

const hostv2TestModuleWorkerURL = "https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/"

func v2WriteOperation(t *testing.T, w http.ResponseWriter, serverURL, action string, generation int64) {
	t.Helper()
	w.Header().Set("Location", serverURL+"/api/v2/operations/op-1")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": "op-1", "resourceUid": "uid-1", "action": action, "generation": generation, "status": "succeeded", "effect": "complete", "createdAt": "2026-10-12T00:00:00Z", "updatedAt": "2026-10-12T00:00:00Z", "retainUntil": time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)})
}

func v2WriteNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"type":"about:blank","title":"Not found","status":404,"code":"not_found"}`))
}
