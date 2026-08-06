package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// TestImportThenReadPopulatesEveryDeclaredField proves an imported resource
// arrives with the state a plan can compare against.
//
// Import writes only the identity; the read that follows is what adopts the
// host's desired state. If it does not, every field looks unset and the next
// plan proposes to rewrite a resource that is already correct.
func TestImportThenReadPopulatesEveryDeclaredField(t *testing.T) {
	for _, kind := range currentformcatalog.Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			desired := kind.CanonicalDesired()
			form := providerCandidateForms()[kind.Kind]
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/takoform/v1alpha2" {
					writeProviderDiscovery(t, w, srv.URL)
					return
				}
				res := client.Resource{
					APIVersion: client.APIVersion, Kind: kind.Kind, Form: ptrForm(form),
					Metadata: client.Metadata{Name: kind.FixtureName(), Space: "prod", ResourceVersion: "1"},
					Spec:     jsonRoundTrip(t, desired),
					Status:   providerPortableStatus(kind.Kind, kind.FixtureName(), 1),
				}
				w.Header().Set("ETag", `"1"`)
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode(res)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": res})
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
			assertStateFormIdentity(t, ctx, readResponse.State, form)
			var providerID types.String
			if diags := readResponse.State.GetAttribute(ctx, path.Root("id"), &providerID); diags.HasError() {
				t.Fatalf("read id: %v", diags)
			}
			if want := kind.Kind + "/" + kind.FixtureName(); providerID.ValueString() != want {
				t.Fatalf("provider-owned id = %q, want %q", providerID.ValueString(), want)
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
				var url types.String
				readResponse.State.GetAttribute(ctx, path.Root("artifact_url"), &url)
				if url.IsNull() {
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
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	form := providerCandidateForms()[kind.Kind]
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			writeProviderDiscovery(t, w, srv.URL)
			return
		}
		res := client.Resource{
			APIVersion: client.APIVersion, Kind: kind.Kind, Form: ptrForm(form),
			Metadata: client.Metadata{Name: "assets", Space: "prod", ResourceVersion: "1"},
			Spec:     map[string]any{"name": "assets", "versioning": false},
			Status:   providerPortableStatus(kind.Kind, "assets", 1),
		}
		w.Header().Set("ETag", `"1"`)
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"resource": res})
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
		"resource_version": types.StringValue("1"),
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("seed %s: %v", name, diags)
		}
	}
	if diags := state.SetAttribute(ctx, path.Root("versioning"), types.BoolValue(true)); diags.HasError() {
		t.Fatalf("seed versioning: %v", diags)
	}
	if diags := setFormIdentityState(ctx, &state, form); diags.HasError() {
		t.Fatalf("seed exact Form identity: %v", diags)
	}

	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	var versioning types.Bool
	readResponse.State.GetAttribute(ctx, path.Root("versioning"), &versioning)
	if versioning.ValueBool() {
		t.Fatal("versioning remained true, want the host-observed false")
	}
}

func TestSetStateRejectsInvalidPortableStatusBeforeAnyStateWrite(t *testing.T) {
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	resource := &formResource{kind: kind, data: &providerData{forms: providerCandidateForms()}}
	ctx := context.Background()
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{
		Schema: schemaResponse.Schema,
		Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
	}
	hostResource := canonicalPortableResource(kind, 1)
	hostResource.Status.Output["credential"] = "must-not-enter-state"
	diags := resource.setState(
		ctx,
		&state,
		hostResource.Metadata.Name,
		hostResource.Spec,
		&hostResource,
		"prod",
		formValues{Fields: map[string]attr.Value{}, Artifact: nullArtifactSourceValues()},
		true,
	)
	if !diags.HasError() {
		t.Fatal("host output outside the exact Form contract entered state")
	}
	if !state.Raw.IsNull() {
		t.Fatal("provider wrote partial state before rejecting invalid host output")
	}
}

func TestSetStateRejectsHostDesiredSpecBeforeAnyStateWrite(t *testing.T) {
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	resource := &formResource{kind: kind, data: &providerData{forms: providerCandidateForms()}}
	ctx := context.Background()
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)

	tests := []struct {
		name   string
		mutate func(*client.Resource)
	}{
		{
			name: "authority field",
			mutate: func(hostResource *client.Resource) {
				hostResource.Spec["selectedTarget"] = "private-target"
			},
		},
		{
			name: "valid but substituted value",
			mutate: func(hostResource *client.Resource) {
				hostResource.Spec["versioning"] = false
			},
		},
		{
			name: "spec name substitution",
			mutate: func(hostResource *client.Resource) {
				hostResource.Spec["name"] = "other"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := tfsdk.State{
				Schema: schemaResponse.Schema,
				Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
			}
			hostResource := canonicalPortableResource(kind, 1)
			test.mutate(&hostResource)
			diags := resource.setState(
				ctx,
				&state,
				kind.FixtureName(),
				kind.CanonicalDesired(),
				&hostResource,
				"prod",
				formValues{Fields: map[string]attr.Value{}, Artifact: nullArtifactSourceValues()},
				true,
			)
			if !diags.HasError() {
				t.Fatal("host desired state outside the exact requested Form contract entered state")
			}
			if !state.Raw.IsNull() {
				t.Fatal("provider wrote partial state before rejecting invalid host desired state")
			}
		})
	}
}

func TestReadFailsClosedBeforeHostWhenStateHasNoExactFormIdentity(t *testing.T) {
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	requests := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			writeProviderDiscovery(t, w, srv.URL)
			return
		}
		http.NotFound(w, r)
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
		"name": types.StringValue("legacy"), "space": types.StringValue("prod"),
		"resource_version": types.StringValue("7"),
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("seed %s: %v", name, diags)
		}
	}
	if diags := state.SetAttribute(ctx, path.Root("versioning"), types.BoolValue(true)); diags.HasError() {
		t.Fatalf("seed versioning: %v", diags)
	}

	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if !readResponse.Diagnostics.HasError() {
		t.Fatal("legacy state without an exact Form identity was refreshed")
	}
	if requests != 1 {
		t.Fatalf("host requests = %d, want discovery only; resource GET must not run", requests)
	}
	if readResponse.State.Raw.IsNull() {
		t.Fatal("legacy state was removed instead of being retained behind a migration diagnostic")
	}
}

func TestReadFailsClosedBeforeHostWhenStateFormIdentityChanged(t *testing.T) {
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	requests := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			writeProviderDiscovery(t, w, srv.URL)
			return
		}
		http.NotFound(w, r)
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
		"name": types.StringValue("changed"), "space": types.StringValue("prod"),
		"resource_version": types.StringValue("7"),
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("seed %s: %v", name, diags)
		}
	}
	if diags := state.SetAttribute(ctx, path.Root("versioning"), types.BoolValue(true)); diags.HasError() {
		t.Fatalf("seed versioning: %v", diags)
	}
	previous := providerCandidateForms()[kind.Kind]
	previous.FormRef.SchemaDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if diags := setFormIdentityState(ctx, &state, previous); diags.HasError() {
		t.Fatalf("seed previous exact Form identity: %v", diags)
	}

	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if !readResponse.Diagnostics.HasError() {
		t.Fatal("state bound to another exact Form identity was refreshed")
	}
	if requests != 1 {
		t.Fatalf("host requests = %d, want discovery only; resource GET must not run", requests)
	}
	if readResponse.State.Raw.IsNull() {
		t.Fatal("mismatched exact-identity state was removed instead of retained")
	}
}

// TestReadAcceptsStateBoundToAnySupportedFormRef verifies the multi-FormRef
// dispatch: state written against an older FormRef of the same kind stays
// readable as long as that FormRef is in the provider's supported set, even
// though it is no longer the default create target.
func TestReadAcceptsStateBoundToAnySupportedFormRef(t *testing.T) {
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	requests := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			writeProviderDiscovery(t, w, srv.URL)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	defaultForm := providerCandidateForms()[kind.Kind]
	olderForm := defaultForm
	olderForm.FormRef.DefinitionVersion = "0.0.9"
	olderForm.FormRef.SchemaDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	unsupportedForm := defaultForm
	unsupportedForm.FormRef.DefinitionVersion = "9.9.9"
	unsupportedForm.FormRef.SchemaDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	resource := &formResource{kind: kind, data: &providerData{
		client:       mustDiscoveredProviderClient(t, srv),
		forms:        map[string]client.InstalledFormReference{kind.Kind: defaultForm},
		defaultSpace: "prod",
		supported:    []client.InstalledFormReference{defaultForm, olderForm},
	}}
	ctx := context.Background()
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	seedState := func(form client.InstalledFormReference) tfsdk.State {
		state := tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)}
		for name, value := range map[string]types.String{
			"name": types.StringValue("assets"), "space": types.StringValue("prod"),
			"resource_version": types.StringValue("7"),
		} {
			if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
				t.Fatalf("seed %s: %v", name, diags)
			}
		}
		if diags := state.SetAttribute(ctx, path.Root("versioning"), types.BoolValue(true)); diags.HasError() {
			t.Fatalf("seed versioning: %v", diags)
		}
		if diags := setFormIdentityState(ctx, &state, form); diags.HasError() {
			t.Fatalf("seed exact Form identity: %v", diags)
		}
		return state
	}

	// State bound to the older supported FormRef passes the identity fence
	// and reaches the host (the plain 404 response fails as a protocol error,
	// proving the client was invoked instead of the fence blocking first).
	before := requests
	readResponse := frameworkresource.ReadResponse{State: seedState(olderForm)}
	resource.Read(ctx, frameworkresource.ReadRequest{State: seedState(olderForm)}, &readResponse)
	if requests == before {
		t.Fatal("supported older FormRef state did not reach the host")
	}

	// State bound to a FormRef outside the supported set fails closed before
	// any host request.
	before = requests
	readResponse = frameworkresource.ReadResponse{State: seedState(unsupportedForm)}
	resource.Read(ctx, frameworkresource.ReadRequest{State: seedState(unsupportedForm)}, &readResponse)
	if !readResponse.Diagnostics.HasError() {
		t.Fatal("unsupported FormRef state was accepted")
	}
	if requests != before {
		t.Fatal("unsupported FormRef state reached the host")
	}
}

func assertStateFormIdentity(t *testing.T, ctx context.Context, state tfsdk.State, want client.InstalledFormReference) {
	t.Helper()
	got := formStateIdentity{}
	for name, target := range map[string]*types.String{
		"form_api_version":        &got.APIVersion,
		"form_kind":               &got.Kind,
		"form_definition_version": &got.DefinitionVersion,
		"form_schema_digest":      &got.SchemaDigest,
		"form_package_digest":     &got.PackageDigest,
	} {
		if diags := state.GetAttribute(ctx, path.Root(name), target); diags.HasError() {
			t.Fatalf("read %s: %v", name, diags)
		}
	}
	reference, complete := got.reference()
	if !complete || reference != want {
		t.Fatalf("state Form identity = %#v complete=%t, want %#v", reference, complete, want)
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

// TestModifyPlanSurfacesHostPreview verifies that `terraform plan` performs a
// read-only host preview and surfaces its outcome as diagnostics: no warning
// when the host accepts the planned desired state, and a warning when the
// host rejects it (apply still reports the authoritative error).
func TestModifyPlanSurfacesHostPreview(t *testing.T) {
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket is not declared")
	}
	var hostRequests atomic.Int32
	var rejectPreview atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests.Add(1)
		if r.URL.Path == "/apis/forms.takoform.com/v1alpha2/forms" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"forms": []client.FormAvailability{{
				Identity:             providerCandidateForms()[kind.Kind],
				DefinitionKnown:      true,
				Installed:            true,
				Executable:           true,
				Activated:            true,
				AvailableToPrincipal: true,
				Operations:           []string{"create", "update", "read", "delete", "import", "observe", "refresh"},
			}}})
			return
		}
		if r.URL.Path == "/apis/forms.takoform.com/v1alpha2/resources/preview" && r.Method == http.MethodPost {
			if rejectPreview.Load() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid_argument","message":"planned spec is invalid","requestId":"req-preview","retryable":false}}`))
				return
			}
			var previewBody client.Resource
			if err := json.NewDecoder(r.Body).Decode(&previewBody); err != nil {
				t.Fatalf("decoding preview request: %v", err)
			}
			specRaw, err := json.Marshal(previewBody.Spec)
			if err != nil {
				t.Fatalf("marshaling preview spec: %v", err)
			}
			specDigest, err := formpackage.DigestCanonicalJSON(specRaw)
			if err != nil {
				t.Fatalf("digesting preview spec: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": previewBody,
				"review": map[string]any{
					"planDigest": "sha256:" + strings.Repeat("d", 64),
					"specDigest": specDigest,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource := &formResource{kind: kind, data: &providerData{
		client: mustDiscoveredProviderClient(t, server), forms: providerCandidateForms(),
		supported: providerSupportedForms(), defaultSpace: "prod",
	}}
	ctx := context.Background()
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)

	plan := tfsdk.Plan{
		Schema: schemaResponse.Schema,
		Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
	}
	for name, value := range map[string]types.String{
		"name": types.StringValue("assets"), "space": types.StringValue("prod"),
	} {
		if diags := plan.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("seed plan %s: %v", name, diags)
		}
	}
	if diags := plan.SetAttribute(ctx, path.Root("versioning"), types.BoolValue(true)); diags.HasError() {
		t.Fatalf("seed plan versioning: %v", diags)
	}
	config := tfsdk.Config{Schema: schemaResponse.Schema, Raw: plan.Raw.Copy()}

	// Accepted preview: the plan carries no warning and the preview endpoint
	// was reached.
	before := hostRequests.Load()
	modifyResponse := frameworkresource.ModifyPlanResponse{}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		Config: config, Plan: plan,
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)},
	}, &modifyResponse)
	if modifyResponse.Diagnostics.HasError() || len(modifyResponse.Diagnostics.Warnings()) != 0 {
		t.Fatalf("accepted preview produced diagnostics: %v", modifyResponse.Diagnostics)
	}
	if hostRequests.Load() == before {
		t.Fatal("plan did not reach the host preview endpoint")
	}

	// Rejected preview: the plan carries a warning and still completes.
	rejectPreview.Store(true)
	modifyResponse = frameworkresource.ModifyPlanResponse{}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		Config: config, Plan: plan,
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)},
	}, &modifyResponse)
	if modifyResponse.Diagnostics.HasError() {
		t.Fatal("rejected preview aborted the plan; apply must remain authoritative")
	}
	if len(modifyResponse.Diagnostics.Warnings()) == 0 {
		t.Fatal("rejected preview did not surface a warning")
	}
}
