package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/edgev2"
	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
	"github.com/tako0614/terraform-provider-takoform/internal/privateinputsv2"
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

func TestV2WorkerVersionUpdatePrivateInputsAreOptionalButExactWhenSelected(t *testing.T) {
	resource := &v2Resource{}
	spec := map[string]any{"requiredSensitiveVars": []any{"API_TOKEN"}}
	if err := os.Unsetenv(privateinputsv2.EnvironmentVariable); err != nil {
		t.Fatal(err)
	}
	got, names, err := resource.privateInputsForUpdate("prod", "worker", spec)
	if err != nil || got != nil || names != nil {
		t.Fatalf("absent source: inputs=%v names=%v error=%v; want omitted inputs", got, names, err)
	}
	t.Setenv(privateinputsv2.EnvironmentVariable, `{"prod":{"worker":{"API_TOKEN":"secret"}}}`)
	got, names, err = resource.privateInputsForUpdate("prod", "worker", spec)
	if err != nil || got == nil || (*got)["API_TOKEN"] != "secret" || len(names) != 1 {
		t.Fatalf("selected source: inputs=%v names=%v error=%v", got, names, err)
	}
	t.Setenv(privateinputsv2.EnvironmentVariable, `{"prod":{"worker":{"OTHER":"secret"}}}`)
	if got, names, err = resource.privateInputsForUpdate("prod", "worker", spec); err == nil || got != nil || names != nil {
		t.Fatalf("mismatched selected source accepted: inputs=%v names=%v error=%v", got, names, err)
	}
	for _, noNames := range []map[string]any{{}, {"requiredSensitiveVars": []any{}}} {
		if got, names, err = resource.privateInputsForUpdate("prod", "worker", noNames); err == nil || got != nil || names != nil {
			t.Fatalf("undeclared selected source accepted: inputs=%v names=%v error=%v", got, names, err)
		}
	}
}

func TestV2WorkerVersionCreateRejectsUndeclaredSelectedPrivateInputs(t *testing.T) {
	resource := &v2Resource{def: v2FormDef{form: edgev2.WorkerVersionURL}}
	t.Setenv(privateinputsv2.EnvironmentVariable, `{"prod":{"worker":{"API_TOKEN":"secret"}}}`)
	for _, noNames := range []map[string]any{{}, {"requiredSensitiveVars": []any{}}} {
		got, names, err := resource.privateInputs("prod", "worker", noNames, hostv2.Discovery{}, hostv2.Support{})
		if err == nil || got != nil || names != nil {
			t.Fatalf("undeclared selected Create input accepted: inputs=%v names=%v error=%v", got, names, err)
		}
	}
}

func TestV2PlanUsesProviderSpaceOnlyWhenResourceConfigOmitsIt(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	data := v2TestData(t, server)
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		configured types.String
		wantKnown  bool
	}{
		{"omitted", types.StringNull(), true},
		{"explicit unknown", types.StringUnknown(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPlan := v2TestPlan(t, schemaResponse, "app")
			if diags := configPlan.SetAttribute(ctx, path.Root("space"), tc.configured); diags.HasError() {
				t.Fatal(diags)
			}
			config := tfsdk.Config{Schema: schemaResponse.Schema, Raw: configPlan.Raw}
			plan := v2TestPlan(t, schemaResponse, "app")
			if diags := plan.SetAttribute(ctx, path.Root("space"), types.StringUnknown()); diags.HasError() {
				t.Fatal(diags)
			}
			response := resource.ModifyPlanResponse{Plan: plan}
			res.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: config, Plan: plan}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %v", response.Diagnostics)
			}
			var space types.String
			if diags := response.Plan.GetAttribute(ctx, path.Root("space"), &space); diags.HasError() {
				t.Fatal(diags)
			}
			if tc.wantKnown && (space.IsUnknown() || space.ValueString() != "prod") {
				t.Fatalf("omitted resource space did not resolve provider default: %v", space)
			}
			if !tc.wantKnown && !space.IsUnknown() {
				t.Fatalf("explicit unknown space was replaced: %v", space)
			}
		})
	}
}

func TestV2TypedHCLSpecPreservesSemanticStateForAllThirteenExactForms(t *testing.T) {
	cases := map[string]string{
		"takoform_module_worker":                `{}`,
		"takoform_worker_bundle":                `{"artifact":{"url":"https://artifacts.example/bundle","sha256":"` + strings.Repeat("a", 64) + `"}}`,
		"takoform_worker_version":               `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":["fetch"],"vars":{"COUNT":0.1234567890123456789},"requiredSensitiveVars":["API_TOKEN"],"kvBindings":[{"name":"KV","resource":{"resourceUid":"kv"}}]}`,
		"takoform_worker_deployment":            `{"worker":{"resourceUid":"w"},"versions":[{"workerVersion":{"resourceUid":"v"},"weight":10000}]}`,
		"takoform_worker_endpoint":              `{"worker":{"resourceUid":"w"}}`,
		"takoform_worker_cron_trigger":          `{"worker":{"resourceUid":"w"},"cron":"*/5 * * * *"}`,
		"takoform_edge_kv_namespace":            `{}`,
		"takoform_edge_object_bucket":           `{}`,
		"takoform_sqlite_database":              `{}`,
		"takoform_sqlite_migration_set":         `{"artifact":{"url":"https://artifacts.example/migrations","sha256":"` + strings.Repeat("b", 64) + `"}}`,
		"takoform_sqlite_migration_application": `{"database":{"resourceUid":"db"},"migrationSet":{"resourceUid":"set"}}`,
		"takoform_at_least_once_queue":          `{"messageRetentionSeconds":60}`,
		"takoform_queue_consumer":               `{"queue":{"resourceUid":"q"},"worker":{"resourceUid":"w"},"maxBatchSize":1,"maxBatchTimeoutSeconds":0,"maxConcurrency":1,"maxRetries":0,"retryDelaySeconds":0}`,
	}
	ctx := context.Background()
	for _, def := range v2Forms {
		t.Run(def.typeName, func(t *testing.T) {
			decoded, err := jsonv2.Decode([]byte(cases[def.typeName]))
			if err != nil {
				t.Fatal(err)
			}
			wire, ok := decoded.(map[string]any)
			if !ok {
				t.Fatal("fixture is not a spec object")
			}
			value, err := v2StateSpec(ctx, def, wire)
			if err != nil {
				t.Fatal(err)
			}
			res, schemaResponse := v2TestResource(t, nil, def.form)
			_ = res
			plan := v2TestPlan(t, schemaResponse, "app")
			if value != nil {
				if diags := plan.SetAttribute(ctx, path.Root("spec"), value); diags.HasError() {
					t.Fatal(diags)
				}
			}
			roundTrip, err := v2SpecFromPlan(ctx, plan, def)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := v2StateSpec(ctx, def, roundTrip)
			if err != nil {
				t.Fatalf("round-tripped wire spec rejected by exact Form: %v", err)
			}
			if value == nil && projected != nil || value != nil && (projected == nil || !value.Equal(projected)) {
				got, _ := jsonv2.Marshal(roundTrip)
				want, _ := jsonv2.Marshal(wire)
				t.Fatalf("typed projection changed Form-semantic state:\ngot  %s\nwant %s", got, want)
			}
		})
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
				_ = json.NewEncoder(w).Encode(map[string]any{"uid": "uid-1", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "app", "generation": generation, "observedGeneration": generation, "observedAt": "2026-10-12T00:00:00Z", "phase": "idle", "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-1"})
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
	// Simulate ACK having reached the journal while Terraform never persisted
	// the returned state. A fresh Create recovers by exact Operation+Resource
	// reads; it must not send even the old mutation key a second time.
	recovered := resource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}}
	res.Create(ctx, resource.CreateRequest{Plan: plan}, &recovered)
	if recovered.Diagnostics.HasError() || mutationCalls.Load() != 1 {
		t.Fatalf("ACK state-loss recovery: diagnostics=%v mutations=%d", recovered.Diagnostics, mutationCalls.Load())
	}
	generation = 2
	wrongGeneration := resource.ReadResponse{State: create.State}
	res.Read(ctx, resource.ReadRequest{State: create.State}, &wrongGeneration)
	if !wrongGeneration.Diagnostics.HasError() || mutationCalls.Load() != 1 {
		t.Fatalf("wrong Host generation: diagnostics=%v mutations=%d", wrongGeneration.Diagnostics, mutationCalls.Load())
	}
	generation = 1
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

func TestV2PreACKClaimRefusesWithoutHostRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	data := v2TestData(t, server)
	_, err := data.store.Begin(context.Background(), intentv2.BeginRequest{
		Target: intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "app"},
		Action: intentv2.ActionCreate, Form: hostv2TestModuleWorkerURL, Method: "POST", Path: "/resources",
		PublicJSON:                 []byte(`{"form":"https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/","space":"prod","name":"app","spec":{}}`),
		AdvertisedReplayWindowSecs: 3600,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	plan := v2TestPlan(t, schemaResponse, "app")
	response := resource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}}
	res.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() || requests.Load() != 0 {
		t.Fatalf("unknown claim: diagnostics=%v Host requests=%d", response.Diagnostics, requests.Load())
	}
}

func TestV2MalformedOrHistoricalStateRefusesBeforeHostAccess(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	data := v2TestData(t, server)
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	ctx := context.Background()
	for _, version := range []int64{0, 1} {
		response := resource.UpgradeStateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}}
		res.UpgradeState(ctx)[version].StateUpgrader(ctx, resource.UpgradeStateRequest{}, &response)
		if !response.Diagnostics.HasError() {
			t.Fatalf("legacy version %d was upgraded", version)
		}
	}
	for _, test := range []struct{ name, value string }{
		{"form_url", "https://edge.forms.takoform.com/forms/ObjectBucket/0.2.0/"},
		{"form_url", ""},
		{"host_api_root", server.URL + "/wrong"},
		{"store_id", "another-store"},
		{"uid", "bad/uid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
			for key, value := range map[string]string{"name": "app", "space": "prod", "uid": "uid-1", "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID} {
				if key == test.name {
					value = test.value
				}
				if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
					t.Fatal(diags)
				}
			}
			if diags := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(1)); diags.HasError() {
				t.Fatal(diags)
			}
			response := resource.ReadResponse{State: state}
			res.Read(ctx, resource.ReadRequest{State: state}, &response)
			if !response.Diagnostics.HasError() || requests.Load() != 0 {
				t.Fatalf("invalid %s: diagnostics=%v Host requests=%d", test.name, response.Diagnostics, requests.Load())
			}
		})
	}
	t.Run("generation", func(t *testing.T) {
		state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
		for key, value := range map[string]string{"name": "app", "space": "prod", "uid": "uid-1", "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID} {
			if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
				t.Fatal(diags)
			}
		}
		if diags := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(0)); diags.HasError() {
			t.Fatal(diags)
		}
		response := resource.ReadResponse{State: state}
		res.Read(ctx, resource.ReadRequest{State: state}, &response)
		if !response.Diagnostics.HasError() || requests.Load() != 0 {
			t.Fatalf("invalid generation: diagnostics=%v Host requests=%d", response.Diagnostics, requests.Load())
		}
	})
	if requests.Load() != 0 {
		t.Fatalf("legacy/malformed state made %d Host requests", requests.Load())
	}
}

func TestV2UIDImportBindsOnlyAuthorizedExactResource(t *testing.T) {
	var server *httptest.Server
	var mutationCalls atomic.Int32
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch req.URL.Path {
		case hostv2.DiscoveryPath:
			_ = json.NewEncoder(w).Encode(hostv2.Discovery{API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs", Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"}, Capabilities: hostv2.Capabilities{}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600}})
		case "/api/v2/resources/import-1":
			if req.Method != http.MethodGet {
				mutationCalls.Add(1)
				http.Error(w, "unexpected mutation", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"uid": "import-1", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "imported", "generation": 5, "observedGeneration": 5, "observedAt": "2026-10-12T00:00:00Z", "phase": "idle", "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-prior"})
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	data := v2TestData(t, server)
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	ctx := context.Background()
	importResponse := resource.ImportStateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}}
	res.ImportState(ctx, resource.ImportStateRequest{ID: "import-1"}, &importResponse)
	if importResponse.Diagnostics.HasError() {
		t.Fatalf("import ID: %v", importResponse.Diagnostics)
	}
	read := resource.ReadResponse{State: importResponse.State}
	res.Read(ctx, resource.ReadRequest{State: importResponse.State}, &read)
	if read.Diagnostics.HasError() || mutationCalls.Load() != 0 {
		t.Fatalf("import Read: diagnostics=%v mutations=%d", read.Diagnostics, mutationCalls.Load())
	}
	if got := v3StateString(t, ctx, read.State, "uid").ValueString(); got != "import-1" {
		t.Fatalf("imported UID = %q", got)
	}
	binding, exists, err := data.store.ReadImportBinding(ctx, intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "imported"})
	if err != nil || !exists || binding.UID != "import-1" || binding.Generation != 5 {
		t.Fatalf("import custody = %+v, exists=%v, err=%v", binding, exists, err)
	}
}

func TestV2FailedCreateCanRefreshThenExplicitlyDeleteSameUID(t *testing.T) {
	var server *httptest.Server
	var deleted bool
	var deletes atomic.Int32
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch req.URL.Path {
		case hostv2.DiscoveryPath:
			_ = json.NewEncoder(w).Encode(hostv2.Discovery{API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs", Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"}, Capabilities: hostv2.Capabilities{}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600}})
		case "/api/v2/resources/failed-uid":
			if req.Method == http.MethodDelete {
				deletes.Add(1)
				deleted = true
				w.Header().Set("Location", server.URL+"/api/v2/operations/op-delete")
				v2WriteOperationFor(w, "op-delete", "failed-uid", "delete", 2, "succeeded", "complete")
				return
			}
			if deleted {
				v2WriteNotFound(w)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"uid": "failed-uid", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "failed", "generation": 1, "observedGeneration": 0, "observedAt": nil, "phase": "error", "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-failed"})
		case "/api/v2/operations/op-failed":
			v2WriteOperationFor(w, "op-failed", "failed-uid", "create", 1, "failed", "none")
		case "/api/v2/operations/op-delete":
			v2WriteOperationFor(w, "op-delete", "failed-uid", "delete", 2, "succeeded", "complete")
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	data := v2TestData(t, server)
	ctx := context.Background()
	target := intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "failed"}
	intent, err := data.store.Begin(ctx, intentv2.BeginRequest{Target: target, Action: intentv2.ActionCreate, Form: hostv2TestModuleWorkerURL, Method: "POST", Path: "/resources", PublicJSON: []byte(`{"form":"https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/","space":"prod","name":"failed","spec":{}}`), AdvertisedReplayWindowSecs: 3600})
	if err != nil {
		t.Fatal(err)
	}
	retention := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	ack := intentv2.OperationAck{ID: "op-failed", ResourceUID: "failed-uid", Action: intentv2.ActionCreate, Generation: 1, Status: "failed", Effect: "none", RetainUntil: retention}
	if _, err = data.store.RecordAccepted(ctx, intent.ID, ack); err != nil {
		t.Fatal(err)
	}
	if _, err = data.store.Complete(ctx, intent.ID, intentv2.CompletionEvidence{Operation: ack, Resource: intentv2.ResourceObservation{Present: true, Path: "/resources/failed-uid", StatusCode: 200, UID: "failed-uid", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "failed", Generation: 1, Phase: "error"}}); err != nil {
		t.Fatal(err)
	}
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
	for key, value := range map[string]string{"name": "failed", "space": "prod", "uid": "failed-uid", "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID, "intent_id": intent.ID, "operation_id": "op-failed", "phase": "error"} {
		if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
			t.Fatal(diags)
		}
	}
	if diags := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(1)); diags.HasError() {
		t.Fatal(diags)
	}
	read := resource.ReadResponse{State: state}
	res.Read(ctx, resource.ReadRequest{State: state}, &read)
	if read.Diagnostics.HasError() {
		t.Fatalf("failed Resource refresh must permit cleanup: %v", read.Diagnostics)
	}
	remove := resource.DeleteResponse{State: read.State}
	res.Delete(ctx, resource.DeleteRequest{State: read.State}, &remove)
	if remove.Diagnostics.HasError() || deletes.Load() != 1 {
		t.Fatalf("same-UID Delete: diagnostics=%v calls=%d", remove.Diagnostics, deletes.Load())
	}
}

func v2WriteOperationFor(w http.ResponseWriter, id, uid, action string, generation int64, status, effect string) {
	body := map[string]any{"id": id, "resourceUid": uid, "action": action, "generation": generation, "status": status, "effect": effect, "createdAt": "2026-10-12T00:00:00Z", "updatedAt": "2026-10-12T00:00:00Z", "retainUntil": time.Now().UTC().Add(3 * time.Hour).Format(time.RFC3339)}
	if status == "failed" {
		body["error"] = map[string]string{"code": "backend_error", "message": "managed object unavailable"}
	}
	_ = json.NewEncoder(w).Encode(body)
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
