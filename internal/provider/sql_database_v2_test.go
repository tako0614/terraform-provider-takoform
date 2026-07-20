package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	frameworkschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/indexedsql"
)

func TestSQLDatabaseV2PutSelectsSuccessorAndOmitsLegacyFields(t *testing.T) {
	ctx := context.Background()
	tables := testSQLDatabaseV2Tables(t, canonicalSQLDatabaseV2Tables())
	forms := providerCandidateForms()
	selected, ok := providerCandidateFormVersion(client.KindSQLDatabase, indexedsql.DefinitionVersion)
	if !ok {
		t.Fatal("missing SQLDatabase@2.0.0 FormRef")
	}
	var received client.Resource
	var sawAvailability, sawPreview, sawApply bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodGet && request.URL.Path == "/apis/forms.takoform.com/v1alpha1/forms":
			assertProviderExactQuery(t, request, selected)
			sawAvailability = true
			_ = json.NewEncoder(w).Encode(map[string]any{"forms": []client.FormAvailability{{
				Identity: selected, DefinitionKnown: true, Installed: true, Executable: true,
				Activated: true, AvailableToPrincipal: true, Operations: []string{"create", "update"},
			}}})
		case request.Method == http.MethodPost && request.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/preview":
			var body client.Resource
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode preview: %v", err)
			}
			if body.Form == nil || *body.Form != selected {
				t.Errorf("preview FormRef = %#v, want %#v", body.Form, selected)
			}
			sawPreview = true
			_ = json.NewEncoder(w).Encode(client.PreviewResourceResult{
				Resource: body, Review: client.PreviewReview{PlanDigest: "sha256:plan", SpecDigest: "sha256:spec"},
			})
		case request.Method == http.MethodPut && request.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/SQLDatabase/main":
			var body struct {
				client.Resource
				Review client.DeploymentReview `json:"review"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode apply: %v", err)
			}
			if body.Form == nil || *body.Form != selected || body.Review.PlanDigest != "sha256:plan" {
				t.Errorf("apply identity/review = %#v/%#v, want %#v", body.Form, body.Review, selected)
			}
			sawApply = true
			received = body.Resource
			received.ID = "tkrn:prod:SQLDatabase:main"
			received.Metadata.ResourceVersion = "1"
			received.Status = &client.Status{Portability: "portable", Conditions: []client.Condition{{Type: "Drifted", Status: "False"}}}
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(received)
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()
	formClient := client.New(server.URL, "token", server.Client())
	if _, err := formClient.Discover(ctx); err != nil {
		t.Fatal(err)
	}

	resource := &serviceShapeResource{
		data: &providerData{
			client: formClient, defaultSpace: "prod", forms: forms,
			capabilities: client.ProductCapabilities{Resources: map[string]bool{client.KindSQLDatabase: true}},
		},
		cfg: serviceShapeConfig{kind: client.KindSQLDatabase, spec: specSQLDatabase},
	}
	plan := serviceShapeModel{
		Name: types.StringValue("main"), Space: types.StringNull(), Engine: types.StringValue("sqlite"),
		MigrationsPath: types.StringNull(), SchemaVersion: types.Int64Unknown(), Tables: tables,
		ResourceVersion: types.StringUnknown(), Portability: types.StringUnknown(), Outputs: types.MapUnknown(types.StringType),
	}
	selectedFromPlan, ok := resource.formForModel(plan)
	if !ok || selectedFromPlan != selected {
		t.Fatalf("selected FormRef = %#v, want exact SQLDatabase@%s", selectedFromPlan, indexedsql.DefinitionVersion)
	}
	var diagnostics diag.Diagnostics
	resource.put(ctx, &plan, &diagnostics)
	if diagnostics.HasError() {
		t.Fatalf("put diagnostics: %v", diagnostics)
	}
	if received.Spec["schemaVersion"] != float64(1) && received.Spec["schemaVersion"] != 1 {
		t.Fatalf("schemaVersion = %#v, want 1", received.Spec["schemaVersion"])
	}
	if _, present := received.Spec["engine"]; present {
		t.Fatalf("SQLDatabase@2.0.0 sent historical engine: %#v", received.Spec)
	}
	if _, present := received.Spec["migrationsPath"]; present {
		t.Fatalf("SQLDatabase@2.0.0 sent historical migrationsPath: %#v", received.Spec)
	}
	if plan.SchemaVersion.ValueInt64() != 1 {
		t.Fatalf("schema_version state = %d, want 1", plan.SchemaVersion.ValueInt64())
	}
	if !sawAvailability || !sawPreview || !sawApply {
		t.Fatalf("versioned v2 request coverage availability=%v preview=%v apply=%v", sawAvailability, sawPreview, sawApply)
	}
}

func TestSQLDatabaseStateSelectsExactHistoricalOrSuccessorForm(t *testing.T) {
	resource := &serviceShapeResource{
		data: &providerData{forms: providerCandidateForms()},
		cfg:  serviceShapeConfig{kind: client.KindSQLDatabase, spec: specSQLDatabase},
	}
	legacy, ok := resource.formForModel(serviceShapeModel{Tables: types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes})})
	if !ok || legacy.FormRef.DefinitionVersion != "1.0.1" {
		t.Fatalf("historical state FormRef = %#v, want SQLDatabase@1.0.1", legacy)
	}
	v2, ok := resource.formForModel(serviceShapeModel{Tables: testSQLDatabaseV2Tables(t, canonicalSQLDatabaseV2Tables())})
	if !ok || v2.FormRef.DefinitionVersion != indexedsql.DefinitionVersion {
		t.Fatalf("indexed state FormRef = %#v, want SQLDatabase@%s", v2, indexedsql.DefinitionVersion)
	}
}

func TestSQLDatabaseUnknownTablesFailsClosedBeforeFormSelectionOrNetwork(t *testing.T) {
	ctx := context.Background()
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	resource := &serviceShapeResource{
		data: &providerData{
			client: client.New(server.URL, "token", server.Client()), defaultSpace: "prod",
			forms: providerCandidateForms(),
		},
		cfg: serviceShapeConfig{kind: client.KindSQLDatabase, spec: specSQLDatabase},
	}
	plan := serviceShapeModel{
		Name: types.StringValue("main"), Engine: types.StringValue("sqlite"),
		MigrationsPath: types.StringNull(), SchemaVersion: types.Int64Unknown(),
		Tables: types.ListUnknown(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}),
		Space:  types.StringNull(), ResourceVersion: types.StringUnknown(),
		Portability: types.StringUnknown(), Outputs: types.MapUnknown(types.StringType),
	}
	if selected, ok := resource.formForModel(plan); ok {
		t.Fatalf("unknown tables selected Form %#v, want no historical or successor Form", selected)
	}
	var diagnostics diag.Diagnostics
	resource.put(ctx, &plan, &diagnostics)
	if !diagnostics.HasError() {
		t.Fatal("unknown tables unexpectedly passed")
	}
	found := false
	for _, diagnostic := range diagnostics.Errors() {
		if diagnostic.Summary() == "Unknown indexed database schema" && strings.Contains(diagnostic.Detail(), "no request was sent") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing unknown-tables fail-closed diagnostic: %v", diagnostics)
	}
	if requestCount != 0 {
		t.Fatalf("unknown tables sent %d HTTP requests, want zero", requestCount)
	}
}

func TestSQLDatabaseV2TablesSchemaRequiresReplacement(t *testing.T) {
	resource := NewSQLDatabaseResource()
	var response frameworkresource.SchemaResponse
	resource.Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", response.Diagnostics)
	}
	tables, ok := response.Schema.Attributes["tables"].(frameworkschema.ListNestedAttribute)
	if !ok || len(tables.PlanModifiers) != 1 {
		t.Fatalf("tables plan modifiers = %#v, want one RequiresReplace modifier", tables.PlanModifiers)
	}
}

func TestSQLDatabaseV2RejectsCompatibilityFallbackBeforeNetwork(t *testing.T) {
	ctx := context.Background()
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/resources/preview":
			_ = json.NewEncoder(w).Encode(client.PreviewResourceResult{PlanDigest: "sha256:legacy-plan"})
		case request.Method == http.MethodPut && request.URL.Path == "/v1/resources/SQLDatabase/main":
			_ = json.NewEncoder(w).Encode(client.Resource{
				APIVersion: client.APIVersion,
				Kind:       client.KindSQLDatabase,
				Metadata: client.Metadata{
					Name: "main", Space: "prod",
				},
				Spec: map[string]any{"name": "main", "engine": "sqlite"},
			})
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()

	shape := &serviceShapeResource{
		data: &providerData{
			client: client.NewCompatibilityFallback(server.URL, "", server.Client()), defaultSpace: "prod",
			forms: providerCandidateForms(),
			capabilities: client.ProductCapabilities{Resources: map[string]bool{
				client.KindSQLDatabase: true,
			}},
		},
		cfg: serviceShapeConfig{kind: client.KindSQLDatabase, spec: specSQLDatabase},
	}
	var schemaResponse frameworkresource.SchemaResponse
	shape.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResponse.Diagnostics)
	}

	tables := testSQLDatabaseV2Tables(t, canonicalSQLDatabaseV2Tables())
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diagnostics := plan.Set(ctx, sqlDatabaseModel{
		ID: types.StringUnknown(), Name: types.StringValue("main"), Engine: types.StringValue("sqlite"),
		MigrationsPath: types.StringNull(), SchemaVersion: types.Int64Unknown(), Tables: tables,
		Space: types.StringNull(), ResourceVersion: types.StringUnknown(), DriftStatus: types.StringUnknown(),
		Portability: types.StringUnknown(), Outputs: types.MapUnknown(types.StringType),
	}); diagnostics.HasError() {
		t.Fatalf("initialize plan: %v", diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diagnostics := state.Set(ctx, sqlDatabaseModel{
		ID: types.StringValue("tkrn:prod:SQLDatabase:main"), Name: types.StringValue("main"),
		Engine: types.StringValue("sqlite"), MigrationsPath: types.StringNull(),
		SchemaVersion: types.Int64Value(1), Tables: tables, Space: types.StringValue("prod"),
		ResourceVersion: types.StringValue("1"), DriftStatus: types.StringValue("current"),
		Portability: types.StringValue("portable"), Outputs: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}); diagnostics.HasError() {
		t.Fatalf("initialize state: %v", diagnostics)
	}

	createResponse := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema}}
	shape.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	assertVersionedHostRequired(t, createResponse.Diagnostics)

	readResponse := frameworkresource.ReadResponse{State: state}
	shape.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	assertVersionedHostRequired(t, readResponse.Diagnostics)

	updateResponse := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: schemaResponse.Schema}}
	shape.Update(ctx, frameworkresource.UpdateRequest{Plan: plan, State: state}, &updateResponse)
	assertVersionedHostRequired(t, updateResponse.Diagnostics)

	deleteResponse := frameworkresource.DeleteResponse{}
	shape.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &deleteResponse)
	assertVersionedHostRequired(t, deleteResponse.Diagnostics)

	if requestCount != 0 {
		t.Fatalf("SQLDatabase@2.0.0 compatibility fallback sent %d requests, want zero", requestCount)
	}

	legacy := serviceShapeModel{
		Name: types.StringValue("main"), Engine: types.StringValue("sqlite"),
		MigrationsPath: types.StringNull(), SchemaVersion: types.Int64Unknown(),
		Tables: types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}),
		Space:  types.StringNull(), ResourceVersion: types.StringUnknown(),
		Portability: types.StringUnknown(), Outputs: types.MapUnknown(types.StringType),
	}
	var legacyDiagnostics diag.Diagnostics
	shape.put(ctx, &legacy, &legacyDiagnostics)
	if legacyDiagnostics.HasError() {
		t.Fatalf("historical SQLDatabase compatibility fallback diagnostics: %v", legacyDiagnostics)
	}
	if requestCount != 2 {
		t.Fatalf("historical SQLDatabase compatibility fallback requests = %d, want preview + apply", requestCount)
	}
}

func assertVersionedHostRequired(t *testing.T, diagnostics diag.Diagnostics) {
	t.Helper()
	if !diagnostics.HasError() {
		t.Fatal("SQLDatabase@2.0.0 compatibility fallback unexpectedly succeeded")
	}
	for _, diagnostic := range diagnostics.Errors() {
		if diagnostic.Summary() == "Versioned Form host required" && strings.Contains(diagnostic.Detail(), "no request was sent") {
			return
		}
	}
	t.Fatalf("missing versioned-host fail-closed diagnostic: %v", diagnostics)
}

func TestSQLDatabaseHistoricalReadAndDeleteSendExact1xFormRef(t *testing.T) {
	ctx := context.Background()
	legacy := providerCandidateForms()[client.KindSQLDatabase]
	if legacy.FormRef.DefinitionVersion != "1.0.1" {
		t.Fatalf("test requires historical SQLDatabase@1.0.1, got %#v", legacy)
	}
	var sawGet, sawObserve, sawDelete bool
	legacyResource := client.Resource{
		APIVersion: client.APIVersion, Kind: client.KindSQLDatabase, Form: &legacy,
		Metadata: client.Metadata{Name: "main", Space: "prod", ResourceVersion: "1"},
		Spec:     map[string]any{"name": "main", "engine": "sqlite"},
		Status:   &client.Status{DriftStatus: "current", Portability: "portable", Outputs: map[string]any{}},
		ID:       "tkrn:prod:SQLDatabase:main",
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodGet && request.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/SQLDatabase/main":
			assertProviderExactQuery(t, request, legacy)
			sawGet = true
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(legacyResource)
		case request.Method == http.MethodPost && request.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/SQLDatabase/main/observe":
			assertProviderExactQuery(t, request, legacy)
			if request.Header.Get("If-Match") != `"1"` {
				t.Errorf("observe If-Match = %q, want quoted generation 1", request.Header.Get("If-Match"))
			}
			sawObserve = true
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": legacyResource, "observation": map[string]any{"status": "current", "summary": "current"},
			})
		case request.Method == http.MethodDelete && request.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/SQLDatabase/main":
			assertProviderExactQuery(t, request, legacy)
			if request.Header.Get("If-Match") != `"1"` {
				t.Errorf("delete If-Match = %q, want quoted generation 1", request.Header.Get("If-Match"))
			}
			sawDelete = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()
	formClient := client.New(server.URL, "token", server.Client())
	if _, err := formClient.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	shape := &serviceShapeResource{
		data: &providerData{client: formClient, defaultSpace: "prod", forms: providerCandidateForms()},
		cfg:  serviceShapeConfig{kind: client.KindSQLDatabase, spec: specSQLDatabase},
	}
	var schemaResponse frameworkresource.SchemaResponse
	shape.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResponse.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diagnostics := state.Set(ctx, sqlDatabaseModel{
		ID: types.StringValue(legacyResource.ID), Name: types.StringValue("main"), Engine: types.StringValue("sqlite"),
		MigrationsPath: types.StringNull(), SchemaVersion: types.Int64Null(),
		Tables: types.ListNull(types.ObjectType{AttrTypes: sqlDatabaseTableAttrTypes}), Space: types.StringValue("prod"),
		ResourceVersion: types.StringValue("1"), DriftStatus: types.StringValue("current"),
		Portability: types.StringValue("portable"), Outputs: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}); diagnostics.HasError() {
		t.Fatalf("initialize state: %v", diagnostics)
	}
	readResponse := frameworkresource.ReadResponse{State: state}
	shape.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResponse.Diagnostics)
	}
	deleteResponse := frameworkresource.DeleteResponse{}
	shape.Delete(ctx, frameworkresource.DeleteRequest{State: readResponse.State}, &deleteResponse)
	if deleteResponse.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResponse.Diagnostics)
	}
	if !sawGet || !sawObserve || !sawDelete {
		t.Fatalf("historical exact request coverage get=%v observe=%v delete=%v", sawGet, sawObserve, sawDelete)
	}
}

func TestSQLDatabaseV2RefreshRoundTrip(t *testing.T) {
	ctx := context.Background()
	wantTables := canonicalSQLDatabaseV2Tables()
	model := serviceShapeModel{}
	diagnostics := refreshServiceShapeSpec(ctx, &client.Resource{Spec: map[string]any{
		"name": "main", "schemaVersion": float64(1), "tables": wantTables,
	}}, specSQLDatabase, &model)
	if diagnostics.HasError() {
		t.Fatalf("refresh diagnostics: %v", diagnostics)
	}
	if model.SchemaVersion.ValueInt64() != 1 || model.Tables.IsNull() {
		t.Fatalf("indexed state was not retained: %#v", model)
	}
	model.Name = types.StringValue("main")
	model.Space = types.StringValue("prod")
	resource, _, diagnostics := model.toResource(ctx, "", client.KindSQLDatabase, specSQLDatabase)
	if diagnostics.HasError() {
		t.Fatalf("round-trip diagnostics: %v", diagnostics)
	}
	if !reflect.DeepEqual(resource.Spec["tables"], wantTables) {
		got, _ := json.Marshal(resource.Spec["tables"])
		want, _ := json.Marshal(wantTables)
		t.Fatalf("tables round-trip drift\n got: %s\nwant: %s", got, want)
	}
}

func TestSQLDatabaseV2RejectsUnsafeSchemaAndLegacyMix(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func([]any){
		"nullable primary key": func(tables []any) {
			tables[0].(map[string]any)["columns"].([]any)[0].(map[string]any)["nullable"] = true
		},
		"number primary key": func(tables []any) {
			tables[0].(map[string]any)["columns"].([]any)[0].(map[string]any)["type"] = "number"
		},
		"duplicate table": func(tables []any) {
			tables = append(tables, cloneTestJSONMap(tables[0].(map[string]any)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			tables := cloneTestJSONArray(canonicalSQLDatabaseV2Tables())
			if name == "duplicate table" {
				tables = append(tables, cloneTestJSONMap(tables[0].(map[string]any)))
			} else {
				mutate(tables)
			}
			model := serviceShapeModel{Name: types.StringValue("main"), Tables: testSQLDatabaseV2Tables(t, tables)}
			if _, _, diagnostics := model.toResource(ctx, "prod", client.KindSQLDatabase, specSQLDatabase); !diagnostics.HasError() {
				t.Fatal("unsafe indexed schema unexpectedly passed")
			}
		})
	}

	model := serviceShapeModel{
		Name: types.StringValue("main"), Tables: testSQLDatabaseV2Tables(t, canonicalSQLDatabaseV2Tables()),
		MigrationsPath: types.StringValue("./migrations"),
	}
	if _, _, diagnostics := model.toResource(ctx, "prod", client.KindSQLDatabase, specSQLDatabase); !diagnostics.HasError() {
		t.Fatal("tables combined with migrations_path unexpectedly passed")
	}
	model.MigrationsPath = types.StringNull()
	model.Engine = types.StringValue("postgres")
	if _, _, diagnostics := model.toResource(ctx, "prod", client.KindSQLDatabase, specSQLDatabase); !diagnostics.HasError() {
		t.Fatal("tables combined with a historical non-default engine unexpectedly passed")
	}
}

func canonicalSQLDatabaseV2Tables() []any {
	return []any{map[string]any{
		"name": "records",
		"columns": []any{
			map[string]any{"name": "id", "type": "string", "nullable": false},
			map[string]any{"name": "tenant_id", "type": "string", "nullable": false},
			map[string]any{"name": "created_at", "type": "integer", "nullable": false},
			map[string]any{"name": "score", "type": "number", "nullable": true},
		},
		"primaryKey": []any{"id"},
		"indexes": []any{map[string]any{
			"name": "by_tenant_created", "columns": []any{"tenant_id", "created_at"}, "unique": true,
		}},
	}}
}

func testSQLDatabaseV2Tables(t *testing.T, raw []any) types.List {
	t.Helper()
	value, diagnostics := sqlDatabaseTablesFromSpec(context.Background(), raw)
	if diagnostics.HasError() {
		t.Fatalf("build tables value: %v", diagnostics)
	}
	return value
}

func cloneTestJSONArray(value []any) []any {
	raw, _ := json.Marshal(value)
	var clone []any
	_ = json.Unmarshal(raw, &clone)
	return clone
}

func cloneTestJSONMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var clone map[string]any
	_ = json.Unmarshal(raw, &clone)
	return clone
}
