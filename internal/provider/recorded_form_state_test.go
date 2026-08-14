package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

var provider102RelationalDatabaseRef = client.InstalledFormReference{
	FormRef: client.FormRef{
		APIVersion: client.APIVersion, Kind: "RelationalDatabase", DefinitionVersion: "2.0.0",
		SchemaDigest: "sha256:3898f8ee507bcebd9e03e80fbc1931b67b477299b1ebe2ff395facb7acf018de",
	},
	PackageDigest: "sha256:dc131e4858ddedbb84d553fdf7808c55fc898a37f15d84839e414fe3ca57c910",
}

var provider102EdgeWorkerRef = client.InstalledFormReference{
	FormRef: client.FormRef{
		APIVersion: client.APIVersion, Kind: "EdgeWorker", DefinitionVersion: "3.0.0",
		SchemaDigest: "sha256:c7fb07db10c937fd6ab119b192552ac239cbcad45dcc12bccd7993decffd2781",
	},
	PackageDigest: "sha256:f03ede50c6b04459e669ed7aaef3e63397b127882a6b4b19dad45ea2da232381",
}

func TestMaintenanceProviderUsesOneDiscoveredOriginForRecordedOldLifecycle(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	spec := provider102RelationalDatabaseSpec()
	var requests []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		if got := request.Header.Get("Authorization"); got != "Bearer run-token" {
			t.Errorf("%s Authorization = %q, want configured bearer", request.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/forms"):
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			writeProviderFormAvailability(t, w, provider102RelationalDatabaseRef)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			w.Header().Set("ETag", `"7"`)
			_ = json.NewEncoder(w).Encode(recordedProviderResource(kind, provider102RelationalDatabaseRef, spec, "7"))
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database/observe"):
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			if got := request.Header.Get("If-Match"); got != `"7"` {
				t.Errorf("observe If-Match = %q, want 7", got)
			}
			w.Header().Set("ETag", `"7"`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": recordedProviderResource(kind, provider102RelationalDatabaseRef, spec, "7"),
			})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/preview"):
			var preview client.Resource
			if err := json.NewDecoder(request.Body).Decode(&preview); err != nil {
				t.Fatal(err)
			}
			assertRecordedUpdateBody(t, &preview, provider102RelationalDatabaseRef)
			digest, err := formpackage.DigestCanonicalJSON(mustJSON(t, preview.Spec))
			if err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": preview,
				"review": map[string]any{
					"planDigest": "sha256:" + strings.Repeat("a", 64),
					"specDigest": digest,
				},
			})
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			var apply struct {
				client.Resource
				Review client.DeploymentReview `json:"review"`
			}
			if err := json.NewDecoder(request.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			assertRecordedUpdateBody(t, &apply.Resource, provider102RelationalDatabaseRef)
			applied := recordedProviderResource(kind, provider102RelationalDatabaseRef, apply.Spec, "8")
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(applied)
		case request.Method == http.MethodDelete && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			if got := request.Header.Get("If-Match"); got != `"8"` {
				t.Errorf("delete If-Match = %q, want 8", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	formClient, err := configureClient(context.Background(), server.URL, "run-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	read, err := formClient.GetResource(
		context.Background(), kind.Kind, "relational-database", "prod", provider102RelationalDatabaseRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formClient.ObserveResource(
		context.Background(), kind.Kind, "relational-database", "prod",
		client.MutationFence{ResourceVersion: "7", Form: provider102RelationalDatabaseRef},
	); err != nil {
		t.Fatal(err)
	}
	read.Status = nil
	read.Spec["storageGib"] = int64(24)
	applied, err := formClient.PutResource(context.Background(), kind.Kind, "relational-database", read)
	if err != nil {
		t.Fatal(err)
	}
	if err := formClient.DeleteResource(
		context.Background(), kind.Kind, "relational-database", "prod",
		client.MutationFence{ResourceVersion: applied.Metadata.ResourceVersion, Form: provider102RelationalDatabaseRef},
	); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /.well-known/takoform",
		"GET /apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/relational-database",
		"POST /apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/relational-database/observe",
		"GET /apis/forms.takoform.com/v1alpha1/forms",
		"POST /apis/forms.takoform.com/v1alpha1/resources/preview",
		"PUT /apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/relational-database",
		"DELETE /apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/relational-database",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("full discovery/lifecycle route sequence = %#v, want %#v", requests, want)
	}
}

func TestExplicitFormTransitionMarkerExistsOnlyOnRelationalDatabase(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		kind string
		want bool
	}{
		{kind: "RelationalDatabase", want: true},
		{kind: "EdgeWorker", want: false},
		{kind: "ObjectBucket", want: false},
	} {
		t.Run(test.kind, func(t *testing.T) {
			resource := &formResource{kind: applyTestKind(t, test.kind)}
			var response frameworkresource.SchemaResponse
			resource.Schema(ctx, frameworkresource.SchemaRequest{}, &response)
			attribute, ok := response.Schema.Attributes["form_transition"]
			if ok != test.want {
				t.Fatalf("form_transition present = %t, want %t", ok, test.want)
			}
			if ok && (!attribute.IsOptional() || attribute.IsRequired() || attribute.IsComputed()) {
				t.Fatal("form_transition must be an optional configuration/state attribute")
			}
		})
	}
}

func TestReadDispatchesSerializedProvider102StateThroughRecordedDatabaseCodec(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	oldSpec := provider102RelationalDatabaseSpec()
	resourceRequests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			resourceRequests++
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			w.Header().Set("ETag", `"7"`)
			_ = json.NewEncoder(w).Encode(recordedProviderResource(kind, provider102RelationalDatabaseRef, oldSpec, "7"))
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database/observe"):
			resourceRequests++
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			w.Header().Set("ETag", `"7"`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": recordedProviderResource(kind, provider102RelationalDatabaseRef, oldSpec, "7"),
			})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	response := frameworkresource.ReadResponse{State: state}
	resource.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("read provider 1.0.2 state: %v", response.Diagnostics)
	}
	if resourceRequests != 2 {
		t.Fatalf("resource requests = %d, want exact GET and observe", resourceRequests)
	}
	assertStateFormIdentity(t, context.Background(), response.State, provider102RelationalDatabaseRef)
}

func TestUpdateDispatchesSerializedProvider102StateThroughRecordedDatabaseCodec(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	var applied *client.Resource
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/forms"):
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			writeProviderFormAvailability(t, w, provider102RelationalDatabaseRef)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/preview"):
			var preview client.Resource
			if err := json.NewDecoder(request.Body).Decode(&preview); err != nil {
				t.Fatal(err)
			}
			assertRecordedUpdateBody(t, &preview, provider102RelationalDatabaseRef)
			specDigest, err := formpackage.DigestCanonicalJSON(mustJSON(t, preview.Spec))
			if err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": preview,
				"review": map[string]any{
					"planDigest": "sha256:" + strings.Repeat("a", 64),
					"specDigest": specDigest,
				},
			})
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			var apply struct {
				client.Resource
				Review client.DeploymentReview `json:"review"`
			}
			if err := json.NewDecoder(request.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			assertRecordedUpdateBody(t, &apply.Resource, provider102RelationalDatabaseRef)
			applied = &apply.Resource
			applied.Metadata.ResourceVersion = "8"
			applied.Status = providerPortableStatus(kind.Kind, applied.Metadata.Name, 8)
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(applied)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	if diags := plan.SetAttribute(context.Background(), path.Root("storage_gib"), types.Int64Value(24)); diags.HasError() {
		t.Fatal(diags)
	}
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(context.Background(), frameworkresource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("update provider 1.0.2 state: %v", response.Diagnostics)
	}
	if applied == nil {
		t.Fatal("recorded-ref update did not reach the ordinary apply endpoint")
	}
	assertStateFormIdentity(t, context.Background(), response.State, provider102RelationalDatabaseRef)
}

func TestDeleteDispatchesSerializedProvider102StateThroughRecordedDatabaseRef(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	deleted := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodDelete && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			assertProviderExactQuery(t, request, provider102RelationalDatabaseRef)
			if got := request.Header.Get("If-Match"); got != `"7"` {
				t.Errorf("If-Match = %q, want generation 7", got)
			}
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	response := frameworkresource.DeleteResponse{}
	resource.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("delete provider 1.0.2 state: %v", response.Diagnostics)
	}
	if !deleted {
		t.Fatal("delete did not use the state-recorded exact FormRef")
	}
}

func TestEdgeArtifactUpdateKeepsSerializedProvider102Edge3Identity(t *testing.T) {
	kind := applyTestKind(t, "EdgeWorker")
	transitionRequests := 0
	var applied *client.Resource
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case strings.Contains(request.URL.Path, "/form-transitions"):
			transitionRequests++
			http.Error(w, "EdgeWorker must not transition", http.StatusInternalServerError)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/forms"):
			assertProviderExactQuery(t, request, provider102EdgeWorkerRef)
			writeProviderFormAvailability(t, w, provider102EdgeWorkerRef)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/preview"):
			var preview client.Resource
			if err := json.NewDecoder(request.Body).Decode(&preview); err != nil {
				t.Fatal(err)
			}
			assertRecordedUpdateBody(t, &preview, provider102EdgeWorkerRef)
			specDigest, err := formpackage.DigestCanonicalJSON(mustJSON(t, preview.Spec))
			if err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": preview,
				"review": map[string]any{
					"planDigest": "sha256:" + strings.Repeat("a", 64),
					"specDigest": specDigest,
				},
			})
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/resources/EdgeWorker/edge-worker"):
			var apply struct {
				client.Resource
				Review client.DeploymentReview `json:"review"`
			}
			if err := json.NewDecoder(request.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			assertRecordedUpdateBody(t, &apply.Resource, provider102EdgeWorkerRef)
			applied = &apply.Resource
			applied.Metadata.ResourceVersion = "8"
			applied.Status = providerPortableStatus(kind.Kind, applied.Metadata.Name, 8)
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(applied)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-edge-worker-state.json", server)
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	if diags := plan.SetAttribute(
		context.Background(),
		path.Root("artifact_sha256"),
		types.StringValue("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	); diags.HasError() {
		t.Fatal(diags)
	}
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(context.Background(), frameworkresource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("artifact update: %v", response.Diagnostics)
	}
	if applied == nil || transitionRequests != 0 {
		t.Fatalf("ordinary apply=%t transition requests=%d, want Edge3 ordinary apply only", applied != nil, transitionRequests)
	}
	assertStateFormIdentity(t, context.Background(), response.State, provider102EdgeWorkerRef)
}

func TestOldDatabaseSchemaFieldsWithoutExplicitTransitionFailBeforeHost(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	hostRequests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests++
		http.Error(w, "transition without marker reached host", http.StatusInternalServerError)
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	for name, value := range map[string]string{
		"schema_url":    "https://artifacts.portable-conformance.invalid/schema.json",
		"schema_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"schema_format": "takosumi.resource-migrations",
	} {
		if diags := plan.SetAttribute(context.Background(), path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatal(diags)
		}
	}
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(context.Background(), frameworkresource.UpdateRequest{Plan: plan, State: state}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("old database accepted successor fields without explicit transition marker")
	}
	if hostRequests != 0 {
		t.Fatalf("host requests = %d, want 0", hostRequests)
	}
	assertStateFormIdentity(t, context.Background(), response.State, provider102RelationalDatabaseRef)
}

func TestUnknownDatabaseTransitionMarkerFailsBeforeHost(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	hostRequests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests++
		http.Error(w, "unknown marker reached host", http.StatusInternalServerError)
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	if diags := plan.SetAttribute(context.Background(), path.Root("form_transition"), types.StringValue("database-next")); diags.HasError() {
		t.Fatal(diags)
	}
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(context.Background(), frameworkresource.UpdateRequest{Plan: plan, State: state}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("unknown transition marker was accepted")
	}
	if hostRequests != 0 {
		t.Fatalf("host requests = %d, want 0", hostRequests)
	}
	assertStateFormIdentity(t, context.Background(), response.State, provider102RelationalDatabaseRef)
}

func TestExplicitDatabaseMarkerTransitionsExactDB2StateToExactDB3Proof(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	newForm := providerCandidateForms()[kind.Kind]
	transitionRequests := 0
	transitionReadbacks := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/form-transitions/"):
			transitionReadbacks++
			writeProviderTransitionOperationAbsent(t, w)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database/form-transitions"):
			transitionRequests++
			var body client.FormTransitionRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if request.Header.Get("Idempotency-Key") != body.OperationID {
				t.Errorf("Idempotency-Key = %q, want operationId %q", request.Header.Get("Idempotency-Key"), body.OperationID)
			}
			if body.FromForm != provider102RelationalDatabaseRef || body.ToForm != newForm ||
				body.Resource.Form == nil || *body.Resource.Form != newForm {
				t.Fatalf("transition pair/resource = %#v -> %#v / %#v", body.FromForm, body.ToForm, body.Resource.Form)
			}
			if body.Expected == nil || body.Expected.ResourceVersion != "7" || body.Resource.Metadata.ResourceVersion != "7" {
				t.Fatalf("generation binding = %#v / %q", body.Expected, body.Resource.Metadata.ResourceVersion)
			}
			if _, leaked := body.Resource.Spec["form_transition"]; leaked {
				t.Fatal("provider-only transition marker leaked into portable desired spec")
			}
			for _, field := range []string{"schemaUrl", "schemaSha256", "schemaFormat"} {
				if _, ok := body.Resource.Spec[field]; !ok {
					t.Errorf("transition desired Resource omits %s", field)
				}
			}
			resource := body.Resource
			resource.Metadata.ResourceVersion = "8"
			resource.Status = providerPortableStatus(kind.Kind, resource.Metadata.Name, 8)
			observedSpecDigest, err := formpackage.DigestCanonicalJSON(mustJSON(t, resource.Spec))
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(client.FormTransitionResponse{
				Operation: client.FormTransitionOperation{
					OperationID: body.OperationID, Status: "committed",
					RequestDigest: providerTransitionRequestDigest(t, body),
					ReconcilePath: request.URL.Path + "/" + body.OperationID,
				},
				Resource: &resource,
				TransitionProof: &client.FormTransitionProof{
					OperationID: body.OperationID, FromForm: body.FromForm, ToForm: body.ToForm,
					TransitionEvidenceDigest: body.TransitionEvidence.Digest,
					ObservedSpecDigest:       observedSpecDigest,
					ResourceVersion:          "8",
					NativeIdentity: client.NativeResourceIdentity{
						Type: "cloudflare-d1-database", ID: "native-db-1",
					},
					Committed: true,
				},
			})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	plan := transitionDatabasePlan(t, state)
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(context.Background(), frameworkresource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("explicit transition: %v", response.Diagnostics)
	}
	if transitionRequests != 1 {
		t.Fatalf("transition requests = %d, want 1", transitionRequests)
	}
	if transitionReadbacks != 1 {
		t.Fatalf("transition preflight readbacks = %d, want 1", transitionReadbacks)
	}
	assertStateFormIdentity(t, context.Background(), response.State, newForm)
	var marker types.String
	if diags := response.State.GetAttribute(context.Background(), path.Root("form_transition"), &marker); diags.HasError() {
		t.Fatal(diags)
	}
	if marker.ValueString() != relationalDatabaseV2ToV3Transition {
		t.Fatalf("persisted marker = %q", marker.ValueString())
	}
}

func TestPersistedDatabaseTransitionMarkerDoesNotRetriggerExactDB3State(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	currentForm := providerCandidateForms()[kind.Kind]
	var transitionRequests, ordinaryApplies int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/.well-known/takoform":
			writeProviderDiscovery(t, w, server.URL)
		case strings.Contains(request.URL.Path, "/form-transitions"):
			transitionRequests++
			http.Error(w, "persisted marker retriggered transition", http.StatusInternalServerError)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/forms"):
			assertProviderExactQuery(t, request, currentForm)
			writeProviderFormAvailability(t, w, currentForm)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/resources/preview"):
			var preview client.Resource
			if err := json.NewDecoder(request.Body).Decode(&preview); err != nil {
				t.Fatal(err)
			}
			if preview.Form == nil || *preview.Form != currentForm {
				t.Fatalf("preview Form = %#v, want exact DB3", preview.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": preview,
				"review": map[string]any{
					"planDigest": "sha256:" + strings.Repeat("a", 64),
					"specDigest": mustCanonicalDigest(t, preview.Spec),
				},
			})
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/resources/RelationalDatabase/relational-database"):
			ordinaryApplies++
			var apply struct {
				client.Resource
				Review client.DeploymentReview `json:"review"`
			}
			if err := json.NewDecoder(request.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			if apply.Form == nil || *apply.Form != currentForm {
				t.Fatalf("apply Form = %#v, want exact DB3", apply.Form)
			}
			apply.Metadata.ResourceVersion = "8"
			apply.Status = providerPortableStatus(kind.Kind, apply.Metadata.Name, 8)
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(apply.Resource)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	if diags := setFormIdentityState(context.Background(), &state, currentForm); diags.HasError() {
		t.Fatal(diags)
	}
	for name, value := range map[string]string{
		"form_transition": relationalDatabaseV2ToV3Transition,
		"schema_url":      "https://artifacts.portable-conformance.invalid/schema.json",
		"schema_sha256":   strings.Repeat("a", 64),
		"schema_format":   "takosumi.resource-migrations",
	} {
		if diags := state.SetAttribute(context.Background(), path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatal(diags)
		}
	}
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	if diags := plan.SetAttribute(context.Background(), path.Root("storage_gib"), types.Int64Value(24)); diags.HasError() {
		t.Fatal(diags)
	}
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(context.Background(), frameworkresource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("ordinary DB3 update with persisted marker: %v", response.Diagnostics)
	}
	if transitionRequests != 0 || ordinaryApplies != 1 {
		t.Fatalf("transition=%d ordinary apply=%d, want persisted marker to stay inert after exact commit", transitionRequests, ordinaryApplies)
	}
	assertStateFormIdentity(t, context.Background(), response.State, currentForm)
}

func TestFailedOrIndeterminateDatabaseTransitionKeepsExactDB2State(t *testing.T) {
	for _, outcome := range []string{"host-failure", "lost-ack"} {
		t.Run(outcome, func(t *testing.T) {
			kind := applyTestKind(t, "RelationalDatabase")
			var posts, readbacks int
			var postedRequest client.FormTransitionRequest
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case request.URL.Path == "/.well-known/takoform":
					writeProviderDiscovery(t, w, server.URL)
				case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/form-transitions/") && posts == 0:
					readbacks++
					writeProviderTransitionOperationAbsent(t, w)
				case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/form-transitions"):
					posts++
					if err := json.NewDecoder(request.Body).Decode(&postedRequest); err != nil {
						t.Fatal(err)
					}
					if outcome == "host-failure" {
						w.WriteHeader(http.StatusConflict)
						_, _ = w.Write([]byte(`{"error":{"code":"form_identity_conflict","message":"transition refused","requestId":"req-refused","retryable":false}}`))
						return
					}
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Fatal(err)
					}
					_ = connection.Close()
				case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/form-transitions/"):
					readbacks++
					operationID := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(client.FormTransitionResponse{
						Operation: client.FormTransitionOperation{
							OperationID: operationID, Status: "indeterminate",
							RequestDigest:     providerTransitionRequestDigest(t, postedRequest),
							ReconcilePath:     request.URL.Path,
							DispatchAttempted: providerBoolPointer(true),
						},
					})
				default:
					http.NotFound(w, request)
				}
			}))
			defer server.Close()

			resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
			response := frameworkresource.UpdateResponse{State: state}
			resource.Update(
				context.Background(),
				frameworkresource.UpdateRequest{Plan: transitionDatabasePlan(t, state), State: state},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("unproved transition changed state without a diagnostic")
			}
			if posts != 1 {
				t.Fatalf("transition POSTs = %d, want exactly 1", posts)
			}
			if (outcome == "lost-ack" && readbacks != 2) || (outcome == "host-failure" && readbacks != 1) {
				t.Fatalf("readbacks = %d for %s", readbacks, outcome)
			}
			assertStateFormIdentity(t, context.Background(), response.State, provider102RelationalDatabaseRef)
		})
	}
}

func providerBoolPointer(value bool) *bool { return &value }

func writeProviderTransitionOperationAbsent(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": "resource_not_found", "message": "transition operation not found",
		"requestId": "request-form-transition-absent", "retryable": false,
		"hostCode": "form_transition_operation_not_found",
	}})
}

func providerTransitionRequestDigest(t *testing.T, request client.FormTransitionRequest) string {
	t.Helper()
	raw, err := json.Marshal(struct {
		Format             string                         `json:"format"`
		OperationID        string                         `json:"operationId"`
		FromForm           client.InstalledFormReference  `json:"fromForm"`
		ToForm             client.InstalledFormReference  `json:"toForm"`
		DesiredSpecDigest  string                         `json:"desiredSpecDigest"`
		Expected           *client.FormTransitionExpected `json:"expected,omitempty"`
		TransitionEvidence client.FormTransitionEvidence  `json:"transitionEvidence"`
	}{
		Format: "takoform.resource-form-transition-request@v1", OperationID: request.OperationID,
		FromForm: request.FromForm, ToForm: request.ToForm,
		DesiredSpecDigest: mustCanonicalDigest(t, request.Resource.Spec), Expected: request.Expected,
		TransitionEvidence: request.TransitionEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustCanonicalDigest(t *testing.T, value any) string {
	t.Helper()
	digest, err := formpackage.DigestCanonicalJSON(mustJSON(t, value))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestExplicitDatabaseTransitionRequiresHostCapabilityBeforeMutation(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	hostRequests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{client.APIVersion},
				"features": map[string]bool{
					"service_forms": true, "exact_form_ref": true,
					"optimistic_concurrency": true, "idempotent_lifecycle": true,
				},
				"endpoints": map[string]string{"api": server.URL + "/apis/forms.takoform.com/v1alpha1"},
			})
			return
		}
		hostRequests++
		http.Error(w, "missing capability reached mutation", http.StatusInternalServerError)
	}))
	defer server.Close()

	resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
	response := frameworkresource.UpdateResponse{State: state}
	resource.Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: transitionDatabasePlan(t, state), State: state},
		&response,
	)
	if !response.Diagnostics.HasError() {
		t.Fatal("transition proceeded without resource_form_transition capability")
	}
	if hostRequests != 0 {
		t.Fatalf("host requests = %d, want 0", hostRequests)
	}
	assertStateFormIdentity(t, context.Background(), response.State, provider102RelationalDatabaseRef)
}

func transitionDatabasePlan(t *testing.T, state tfsdk.State) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	for name, value := range map[string]string{
		"form_transition": relationalDatabaseV2ToV3Transition,
		"schema_url":      "https://artifacts.portable-conformance.invalid/schema.json",
		"schema_sha256":   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"schema_format":   "takosumi.resource-migrations",
	} {
		if diags := plan.SetAttribute(context.Background(), path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatal(diags)
		}
	}
	return plan
}

func TestRecordedLifecycleRejectsMissingOrUnknownExactRefBeforeHost(t *testing.T) {
	kind := applyTestKind(t, "RelationalDatabase")
	identities := []struct {
		name   string
		mutate func(tfsdk.State) tfsdk.State
	}{
		{
			name: "missing",
			mutate: func(state tfsdk.State) tfsdk.State {
				if diags := state.SetAttribute(context.Background(), path.Root("form_package_digest"), types.StringNull()); diags.HasError() {
					t.Fatal(diags)
				}
				return state
			},
		},
		{
			name: "unknown",
			mutate: func(state tfsdk.State) tfsdk.State {
				if diags := state.SetAttribute(
					context.Background(),
					path.Root("form_schema_digest"),
					types.StringValue("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				); diags.HasError() {
					t.Fatal(diags)
				}
				return state
			},
		},
	}
	operations := []string{"read", "update", "delete"}
	for _, identity := range identities {
		identity := identity
		for _, operation := range operations {
			operation := operation
			t.Run(identity.name+"/"+operation, func(t *testing.T) {
				hostRequests := 0
				var server *httptest.Server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/.well-known/takoform" {
						writeProviderDiscovery(t, w, server.URL)
						return
					}
					hostRequests++
					http.Error(w, "unrecognized state identity reached host", http.StatusInternalServerError)
				}))
				defer server.Close()

				resource, state := recordedProvider102Resource(t, kind, "testdata/provider-1.0.2-relational-database-state.json", server)
				state = identity.mutate(state)
				var hasError bool
				switch operation {
				case "read":
					response := frameworkresource.ReadResponse{State: state}
					resource.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &response)
					hasError = response.Diagnostics.HasError()
				case "update":
					response := frameworkresource.UpdateResponse{State: state}
					resource.Update(
						context.Background(),
						frameworkresource.UpdateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}, State: state},
						&response,
					)
					hasError = response.Diagnostics.HasError()
				case "delete":
					response := frameworkresource.DeleteResponse{}
					resource.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &response)
					hasError = response.Diagnostics.HasError()
				}
				if !hasError {
					t.Fatal("lifecycle accepted missing or unknown exact state identity")
				}
				if hostRequests != 0 {
					t.Fatalf("host requests = %d, want 0", hostRequests)
				}
			})
		}
	}
}

func recordedProvider102Resource(
	t *testing.T,
	kind formcatalog.Kind,
	fixture string,
	server *httptest.Server,
) (*formResource, tfsdk.State) {
	t.Helper()
	ctx := context.Background()
	resource := &formResource{kind: kind, data: &providerData{
		client: mustDiscoveredProviderClient(t, server), forms: providerCandidateForms(), defaultSpace: "prod",
	}}
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	rawJSON, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tftypes.ValueFromJSON(rawJSON, schemaResponse.Schema.Type().TerraformType(ctx))
	if err != nil {
		t.Fatalf("decode serialized provider 1.0.2 state: %v", err)
	}
	return resource, tfsdk.State{Schema: schemaResponse.Schema, Raw: raw}
}

func provider102RelationalDatabaseSpec() map[string]any {
	return map[string]any{
		"name": "relational-database", "engine": "postgres", "engineVersion": "16",
		"storageGib": int64(20), "sizeClass": "db.small", "databaseName": "app", "highAvailability": false,
	}
}

func recordedProviderResource(
	kind formcatalog.Kind,
	form client.InstalledFormReference,
	spec map[string]any,
	version string,
) client.Resource {
	generation := int64(7)
	if version == "8" {
		generation = 8
	}
	return client.Resource{
		APIVersion: client.APIVersion,
		Kind:       kind.Kind,
		Form:       ptrForm(form),
		Metadata: client.Metadata{
			Name: spec["name"].(string), Space: "prod", ResourceVersion: version,
		},
		Spec:   jsonRoundTripNoTest(spec),
		Status: providerPortableStatus(kind.Kind, spec["name"].(string), generation),
	}
}

func assertRecordedUpdateBody(t *testing.T, resource *client.Resource, want client.InstalledFormReference) {
	t.Helper()
	if resource.Form == nil || *resource.Form != want {
		t.Fatalf("request FormRef = %#v, want state-recorded %#v", resource.Form, want)
	}
	for _, successorOnly := range []string{"schemaUrl", "schemaSha256", "schemaFormat", "assetsPath", "assetsNotFoundHandling"} {
		if _, ok := resource.Spec[successorOnly]; ok {
			t.Errorf("ordinary recorded-codec update widened to successor field %q", successorOnly)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func jsonRoundTripNoTest(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
