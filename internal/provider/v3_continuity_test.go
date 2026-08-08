package provider

// v3_continuity_test.go covers the state-continuity contract of the v1alpha3
// lane (spec/decisions/0017): per-exact-ref codecs, the fail-closed rule for an
// identity this build cannot decode, the exact-identity import forms, a UID
// mismatch that preserves state, and pending-operation resumption against a
// host where the resource does not exist until the operation commits.

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// v3SeedStateRef rewrites the exact identity recorded in one state value, the
// way a state file written by an older provider build carries it.
func v3SeedStateRef(t *testing.T, ctx context.Context, state tfsdk.State, ref currentformregistry.V3Ref) tfsdk.State {
	t.Helper()
	for name, value := range map[string]string{
		"form_api_version":        ref.APIVersion,
		"form_kind":               ref.Kind,
		"form_definition_version": ref.DefinitionVersion,
		"form_schema_digest":      ref.SchemaDigest,
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatalf("seed state %s: %v", name, diags)
		}
	}
	return state
}

// TestV3PerRefCodecEncodesTheStateRefFieldSet proves the codec, not the current
// schema, decides what a mutation of existing state carries. The synthetic
// prior definition version of AtLeastOnceQueue declares a SMALLER field set;
// state bound to it must apply exactly that set, even though the compiled
// schema still offers the newer one.
func TestV3PerRefCodecEncodesTheStateRefFieldSet(t *testing.T) {
	host := newV3FakeHost(t)
	ctx := context.Background()

	current, ok := edgeformcatalog.ByKind("AtLeastOnceQueue")
	if !ok {
		t.Fatal("AtLeastOnceQueue is not a declared family Form")
	}
	if len(current.Fields) < 2 {
		t.Fatalf("AtLeastOnceQueue declares %d fields; this test needs at least two", len(current.Fields))
	}
	// The prior definition version knew only the first declared field: every
	// later one is an addition the older contract cannot carry.
	priorForm := current
	priorForm.Fields = []model.Field{current.Fields[0]}
	priorRef := currentformregistry.V3Ref{
		APIVersion:        edgeformcatalog.Family.APIVersion(),
		Kind:              "AtLeastOnceQueue",
		DefinitionVersion: "0.0.9",
		SchemaDigest:      "sha256:aaaa111111111111111111111111111111111111111111111111111111111111",
		PackageDigest:     "sha256:aaaa222222222222222222222222222222222222222222222222222222222222",
	}
	resource := v3ResourceWithSecondDefinitionVersion(
		t, "AtLeastOnceQueue", newV3TestProviderData(t, host), priorRef, priorForm, false,
	)
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":                      types.StringValue("events"),
		"message_retention_seconds": types.Int64Value(345600),
		"delivery_delay_seconds":    types.Int64Value(0),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	createdSpec := host.applySpecs[0]
	// The create used the DEFAULT ref, so the current, complete field set
	// travelled.
	if len(createdSpec) != len(current.Fields) {
		t.Fatalf("create under the default ref sent %#v, want the current field set", createdSpec)
	}

	// Existing state is rebound onto the prior identity, and updated. The
	// update must speak the PRIOR contract.
	priorState := v3SeedStateRef(t, ctx, tfsdk.State{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}, priorRef)
	updatePlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: priorState.Raw}
	updateResponse := frameworkresource.UpdateResponse{State: priorState}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: updatePlan, State: priorState}, &updateResponse)
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("update under the prior codec: %v", updateResponse.Diagnostics)
	}
	updatedSpec := host.applySpecs[len(host.applySpecs)-1]
	if len(updatedSpec) != 1 {
		t.Fatalf("update under the prior codec sent %#v, want only %q", updatedSpec, current.Fields[0].Wire)
	}
	if _, carried := updatedSpec[current.Fields[0].Wire]; !carried {
		t.Fatalf("update under the prior codec dropped its own field: %#v", updatedSpec)
	}
	// The mutation travelled under the prior identity, not the create default.
	updatedForm := host.applyBodies[len(host.applyBodies)-1]["form"].(map[string]any)["formRef"].(map[string]any)
	if updatedForm["definitionVersion"] != priorRef.DefinitionVersion ||
		updatedForm["schemaDigest"] != priorRef.SchemaDigest {
		t.Fatalf("update rebound the resource to %#v", updatedForm)
	}
}

// TestV3StateRefWithNoCompiledCodecFailsClosed proves the provider refuses to
// serve state bound to an identity it cannot decode, and names both sides. The
// alternative — reading under the current ref — would reinterpret one contract
// as another and read that query's 404 as deletion.
func TestV3StateRefWithNoCompiledCodecFailsClosed(t *testing.T) {
	host := newV3FakeHost(t)
	ctx := context.Background()
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("module-worker"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	unknown := v3PriorModuleWorkerRef
	state := v3SeedStateRef(t, ctx, tfsdk.State{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}, unknown)
	defaultRef, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}

	before := host.getRequests
	for name, run := range map[string]func() diag.Diagnostics{
		"read": func() diag.Diagnostics {
			response := frameworkresource.ReadResponse{State: state}
			resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
			return response.Diagnostics
		},
		"delete": func() diag.Diagnostics {
			response := frameworkresource.DeleteResponse{}
			resource.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &response)
			return response.Diagnostics
		},
	} {
		diagnostics := run()
		if !diagnostics.HasError() {
			t.Fatalf("%s accepted state bound to an identity with no compiled codec", name)
		}
		detail := diagnostics.Errors()[0].Detail()
		if !strings.Contains(detail, unknown.SchemaDigest) {
			t.Fatalf("%s diagnostic does not name the state identity: %s", name, detail)
		}
		if !strings.Contains(detail, defaultRef.SchemaDigest) {
			t.Fatalf("%s diagnostic does not name the identities the build knows: %s", name, detail)
		}
	}
	if host.getRequests != before {
		t.Fatalf("an unsupported state identity reached the host (%d new GETs)", host.getRequests-before)
	}
}

// TestV3ImportIdentityForms covers every accepted import ID and proves the
// canonical JSON form binds the EXACT identity it names, including one an
// earlier definition version created.
func TestV3ImportIdentityForms(t *testing.T) {
	ctx := context.Background()
	defaultRef, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	priorRef := v3PriorModuleWorkerRef
	priorForm, _ := edgeformcatalog.ByKind("ModuleWorker")

	t.Run("short forms resolve to the default create ref", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
		schemaResponse := v3SchemaOf(t, resource)
		for _, id := range []string{"module-worker", "prod/module-worker"} {
			response := frameworkresource.ImportStateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: id}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("import %q: %v", id, response.Diagnostics)
			}
			if got := v3StateString(t, ctx, response.State, "name").ValueString(); got != "module-worker" {
				t.Fatalf("import %q name = %q", id, got)
			}
			if got := v3StateString(t, ctx, response.State, "form_schema_digest").ValueString(); got != defaultRef.SchemaDigest {
				t.Fatalf("import %q did not resolve the default create ref: %q", id, got)
			}
		}
	})

	t.Run("canonical JSON binds an older definition version", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource := v3ResourceWithSecondDefinitionVersion(
			t, "ModuleWorker", newV3TestProviderData(t, host), priorRef, priorForm, false,
		)
		schemaResponse := v3SchemaOf(t, resource)
		// A SpaceID may contain any character but `/` — including the spaces and
		// punctuation no delimiter-joined import ID could carry unambiguously.
		id := `{"space":"acme corp: team a",` +
			`"apiVersion":"` + priorRef.APIVersion + `","kind":"ModuleWorker",` +
			`"definitionVersion":"` + priorRef.DefinitionVersion + `",` +
			`"schemaDigest":"` + priorRef.SchemaDigest + `","name":"module-worker"}`
		response := frameworkresource.ImportStateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: id}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("canonical import: %v", response.Diagnostics)
		}
		for name, want := range map[string]string{
			"name":                    "module-worker",
			"space":                   "acme corp: team a",
			"form_definition_version": priorRef.DefinitionVersion,
			"form_schema_digest":      priorRef.SchemaDigest,
			"form_package_digest":     priorRef.PackageDigest,
		} {
			if got := v3StateString(t, ctx, response.State, name).ValueString(); got != want {
				t.Fatalf("canonical import %s = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("an identity the build cannot decode is refused", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
		schemaResponse := v3SchemaOf(t, resource)
		id := `{"apiVersion":"` + priorRef.APIVersion + `","kind":"ModuleWorker",` +
			`"definitionVersion":"` + priorRef.DefinitionVersion + `",` +
			`"schemaDigest":"` + priorRef.SchemaDigest + `","name":"module-worker"}`
		response := frameworkresource.ImportStateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: id}, &response)
		if !response.Diagnostics.HasError() {
			t.Fatal("import silently rebound an unknown identity to the default create ref")
		}
		if detail := response.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, priorRef.SchemaDigest) {
			t.Fatalf("import diagnostic does not name the requested identity: %s", detail)
		}
	})

	t.Run("malformed identities are rejected", func(t *testing.T) {
		for _, id := range []string{
			"",
			"{",
			`{"name":"module-worker","apiVersion":"edge.forms.takoform.com/v1alpha1"}`,
			`{"name":"Module-Worker"}`,
			`{"name":"module-worker","space":"bad/space"}`,
			`{"name":"module-worker","unknown":"member"}`,
			`{"name":"module-worker"} {"name":"other"}`,
		} {
			if _, err := v3ParseImportID(id); err == nil {
				t.Fatalf("import ID %q was accepted", id)
			}
		}
		identity, err := v3ParseImportID(`{"name":"module-worker","space":"prod"}`)
		if err != nil || identity.HasFormRef || identity.Space != "prod" || identity.Name != "module-worker" {
			t.Fatalf("JSON short form = %#v (err %v)", identity, err)
		}
	})
}

// TestV3UIDMismatchIsAnErrorThatPreservesState is the recovery property of
// decision 0017. Removing state here would leave the operator stuck: the next
// apply fences on If-None-Match:* against a resource that exists and fails,
// with no plan that repairs it.
func TestV3UIDMismatchIsAnErrorThatPreservesState(t *testing.T) {
	ctx := context.Background()

	t.Run("typed", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
		schemaResponse := v3SchemaOf(t, resource)
		plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name": types.StringValue("module-worker"),
		})
		createResponse := frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
		if createResponse.Diagnostics.HasError() {
			t.Fatalf("create: %v", createResponse.Diagnostics)
		}
		// The host now serves a DIFFERENT incarnation under the same name.
		host.replaceIncarnation("ModuleWorker", "module-worker", "uid-99")

		readResponse := frameworkresource.ReadResponse{State: createResponse.State}
		resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
		assertUIDMismatchPreservesState(t, ctx, readResponse.Diagnostics, readResponse.State, "uid-1", "uid-99")
	})

	t.Run("resumed after a pending operation", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
		schemaResponse := v3SchemaOf(t, resource)
		plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name": types.StringValue("module-worker"),
		})
		createResponse := frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
		if createResponse.Diagnostics.HasError() {
			t.Fatalf("create: %v", createResponse.Diagnostics)
		}
		// A pending marker must not weaken the UID rule: the operation is
		// consulted first, and the mismatch it then finds is still a hard error
		// that keeps the resource under management.
		state := createResponse.State
		if diags := state.SetAttribute(ctx, path.Root("pending_operation_id"), types.StringValue("op_missing")); diags.HasError() {
			t.Fatalf("seed pending marker: %v", diags)
		}
		host.replaceIncarnation("ModuleWorker", "module-worker", "uid-99")

		readResponse := frameworkresource.ReadResponse{State: state}
		resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
		assertUIDMismatchPreservesState(t, ctx, readResponse.Diagnostics, readResponse.State, "uid-1", "uid-99")
	})
}

func assertUIDMismatchPreservesState(
	t *testing.T,
	ctx context.Context,
	diagnostics diag.Diagnostics,
	state tfsdk.State,
	stateUID, hostUID string,
) {
	t.Helper()
	if !diagnostics.HasError() {
		t.Fatalf("a UID mismatch was not a hard error: %v", diagnostics)
	}
	if state.Raw.IsNull() {
		t.Fatal("a UID mismatch removed the resource from state")
	}
	if got := v3StateString(t, ctx, state, "uid").ValueString(); got != stateUID {
		t.Fatalf("state uid = %q, want the preserved %q", got, stateUID)
	}
	detail := diagnostics.Errors()[0].Detail()
	for _, want := range []string{stateUID, hostUID, "import", "restore", "delete"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("UID mismatch diagnostic does not name %q: %s", want, detail)
		}
	}
}

// TestV3ReadResumesPendingOperation drives the resumption table against a host
// where the resource does NOT exist until its operation commits. The fake that
// creates the resource at 202 cannot exercise this: there, a read during the
// window happens to find the resource, so the "404 means deleted" rule never
// fires.
func TestV3ReadResumesPendingOperation(t *testing.T) {
	ctx := context.Background()

	// acceptedState drives one create the host accepts as an uncommitted
	// operation and returns the state it left behind.
	acceptedState := func(t *testing.T, host *v3FakeHost) (*v3FormResource, tfsdk.State) {
		t.Helper()
		host.apply202Uncommitted = true
		resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
		schemaResponse := v3SchemaOf(t, resource)
		plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name":           types.StringValue("module-worker"),
			"create_timeout": types.StringValue("400ms"),
		})
		createResponse := frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
		if !createResponse.Diagnostics.HasError() {
			t.Fatal("an uncommitted create reported success")
		}
		if got := v3StateString(t, ctx, createResponse.State, "pending_operation_id").ValueString(); got != "op_apply_uncommitted" {
			t.Fatalf("accepted create recorded pending_operation_id = %q", got)
		}
		return resource, createResponse.State
	}

	t.Run("still running keeps the resource under management", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource, state := acceptedState(t, host)
		response := frameworkresource.ReadResponse{State: state}
		resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("read while the operation runs: %v", response.Diagnostics)
		}
		if response.State.Raw.IsNull() {
			t.Fatal("a 404 during the pending window removed the resource from state")
		}
		if got := v3StateString(t, ctx, response.State, "pending_operation_id").ValueString(); got != "op_apply_uncommitted" {
			t.Fatalf("the pending marker was dropped: %q", got)
		}
		if warnings := response.Diagnostics.Warnings(); len(warnings) == 0 ||
			!strings.Contains(warnings[0].Detail(), "op_apply_uncommitted") {
			t.Fatalf("no warning named the operation still running: %v", response.Diagnostics)
		}

		// Once it commits, the next read settles and clears the marker.
		host.commitDeferredOperation("op_apply_uncommitted")
		settled := frameworkresource.ReadResponse{State: response.State}
		resource.Read(ctx, frameworkresource.ReadRequest{State: response.State}, &settled)
		if settled.Diagnostics.HasError() {
			t.Fatalf("read after commit: %v", settled.Diagnostics)
		}
		if got := v3StateString(t, ctx, settled.State, "uid").ValueString(); got != "uid-1" {
			t.Fatalf("settled uid = %q, want uid-1", got)
		}
		if pending := v3StateString(t, ctx, settled.State, "pending_operation_id"); !pending.IsNull() {
			t.Fatalf("a settled read kept the marker: %q", pending.ValueString())
		}
	})

	t.Run("terminal error with nothing committed removes state", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource, state := acceptedState(t, host)
		host.failDeferredOperation("op_apply_uncommitted", "backend_unavailable")
		response := frameworkresource.ReadResponse{State: state}
		resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("read after a failed operation: %v", response.Diagnostics)
		}
		if !response.State.Raw.IsNull() {
			t.Fatal("a failed operation that committed nothing left the resource in state")
		}
		if warnings := response.Diagnostics.Warnings(); len(warnings) == 0 ||
			!strings.Contains(warnings[0].Detail(), "backend_unavailable") {
			t.Fatalf("no warning named the terminal failure: %v", response.Diagnostics)
		}
	})

	t.Run("terminal error with a committed replacement is a hard error", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource, state := acceptedState(t, host)
		host.failDeferredOperation("op_apply_uncommitted", "backend_unavailable")
		// The host nonetheless holds a DIFFERENT incarnation under that name.
		host.storeResource(
			"ModuleWorker", "module-worker", "prod",
			edgeformcatalog.Family.APIVersion(), "uid-42", map[string]any{},
		)
		response := frameworkresource.ReadResponse{State: state}
		resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
		assertUIDMismatchPreservesState(t, ctx, response.Diagnostics, response.State, "uid-1", "uid-42")
	})

	t.Run("a forgotten operation defers to the exact resource read", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource, state := acceptedState(t, host)
		host.forgetOperation("op_apply_uncommitted")
		response := frameworkresource.ReadResponse{State: state}
		resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("read after the operation record expired: %v", response.Diagnostics)
		}
		if !response.State.Raw.IsNull() {
			t.Fatal("a forgotten operation over an absent resource left state behind")
		}

		// With the resource present under ANOTHER incarnation, the same path
		// verifies the uid rather than re-binding by name.
		host2 := newV3FakeHost(t)
		resource2, state2 := acceptedState(t, host2)
		host2.forgetOperation("op_apply_uncommitted")
		host2.storeResource(
			"ModuleWorker", "module-worker", "prod",
			edgeformcatalog.Family.APIVersion(), "uid-42", map[string]any{},
		)
		response2 := frameworkresource.ReadResponse{State: state2}
		resource2.Read(ctx, frameworkresource.ReadRequest{State: state2}, &response2)
		assertUIDMismatchPreservesState(t, ctx, response2.Diagnostics, response2.State, "uid-1", "uid-42")
	})

	t.Run("a terminal success settles and clears the marker", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource, state := acceptedState(t, host)
		host.commitDeferredOperation("op_apply_uncommitted")
		response := frameworkresource.ReadResponse{State: state}
		resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("read after commit: %v", response.Diagnostics)
		}
		if got := v3StateString(t, ctx, response.State, "uid").ValueString(); got != "uid-1" {
			t.Fatalf("settled uid = %q, want uid-1", got)
		}
		if pending := v3StateString(t, ctx, response.State, "pending_operation_id"); !pending.IsNull() {
			t.Fatalf("a settled read kept the marker: %q", pending.ValueString())
		}
	})
}
