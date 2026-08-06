package provider

// v3_recovery_test.go covers the state-integrity properties of the v1alpha3
// lane: a create the host ACCEPTED but that never completed must leave enough
// identity behind to reconcile, dispatch must follow the FormRef recorded in
// state rather than the current create default, and null must not pass as a
// JSON object.

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

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// TestV3CreateAcceptedButUnfinishedWritesRecoverableState drives a host that
// accepts the apply as a 202 Operation and never finishes it. Terraform commits
// the state a failed Create leaves behind, so a Create that returns an error
// WITHOUT writing state orphans the resource the host now owns and the next
// plan proposes creating it a second time.
func TestV3CreateAcceptedButUnfinishedWritesRecoverableState(t *testing.T) {
	host := newV3FakeHost(t)
	host.apply202Pending = true
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":           types.StringValue("module-worker"),
		"space":          types.StringValue("prod"),
		"create_timeout": types.StringValue("400ms"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)

	if !createResponse.Diagnostics.HasError() {
		t.Fatal("a create whose operation never completed reported success")
	}
	if createResponse.State.Raw.IsNull() {
		t.Fatal("accepted-but-unfinished create wrote no state: the host-owned resource is orphaned")
	}

	defaultRef, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"name":                    "module-worker",
		"space":                   "prod",
		"uid":                     "uid-1",
		"form_api_version":        defaultRef.APIVersion,
		"form_kind":               "ModuleWorker",
		"form_definition_version": defaultRef.DefinitionVersion,
		"form_schema_digest":      defaultRef.SchemaDigest,
		"pending_operation_id":    "op_apply_pending",
	} {
		if got := v3StateString(t, ctx, createResponse.State, name).ValueString(); got != want {
			t.Errorf("recovered state %s = %q, want %q", name, got, want)
		}
	}
	if warnings := createResponse.Diagnostics.Warnings(); len(warnings) == 0 ||
		!strings.Contains(warnings[0].Detail(), "op_apply_pending") {
		t.Errorf("no diagnostic told the operator what to resume: %v", createResponse.Diagnostics)
	}

	// The recorded identity must be enough to re-read the resource. The next
	// plan therefore reconciles the existing resource instead of creating a
	// duplicate, and the settled read clears the recovery marker.
	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("recovered state could not be re-read: %v", readResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, readResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("re-read state uid = %q, want uid-1", got)
	}
	if got := v3StateString(t, ctx, readResponse.State, "revision").ValueString(); got != "1" {
		t.Fatalf("re-read state revision = %q, want the host revision 1", got)
	}
	if pending := v3StateString(t, ctx, readResponse.State, "pending_operation_id"); !pending.IsNull() {
		t.Fatalf("a settled read left the recovery marker behind: %q", pending.ValueString())
	}
}

// TestV3GenericCreateAcceptedButUnfinishedWritesRecoverableState is the same
// property for the generic exact-FormRef carrier.
func TestV3GenericCreateAcceptedButUnfinishedWritesRecoverableState(t *testing.T) {
	host := newV3FakeHost(t)
	host.apply202Pending = true
	generic := &v3GenericResource{data: newV3TestProviderData(t, host)}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, generic)

	const digest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"form_api_version":        types.StringValue("forms.example.com/v1alpha1"),
		"form_kind":               types.StringValue("Widget"),
		"form_definition_version": types.StringValue("1.0.0"),
		"form_schema_digest":      types.StringValue(digest),
		"name":                    types.StringValue("my-widget"),
		"create_timeout":          types.StringValue("400ms"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	generic.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)

	if !createResponse.Diagnostics.HasError() {
		t.Fatal("a generic create whose operation never completed reported success")
	}
	if createResponse.State.Raw.IsNull() {
		t.Fatal("accepted-but-unfinished generic create wrote no state")
	}
	for name, want := range map[string]string{
		"name":                 "my-widget",
		"space":                "prod",
		"uid":                  "uid-1",
		"form_api_version":     "forms.example.com/v1alpha1",
		"form_kind":            "Widget",
		"form_schema_digest":   digest,
		"pending_operation_id": "op_apply_pending",
	} {
		if got := v3StateString(t, ctx, createResponse.State, name).ValueString(); got != want {
			t.Errorf("recovered generic state %s = %q, want %q", name, got, want)
		}
	}
}

// TestV3ReadAndDeleteDispatchOnTheStateFormRef records state under a SECOND
// exact FormRef of the same Kind and proves read and delete query that
// identity, not the current create default. Membership — not equality with the
// default — is what keeps existing state readable across a Form line bump.
func TestV3ReadAndDeleteDispatchOnTheStateFormRef(t *testing.T) {
	host := newV3FakeHost(t)
	ctx := context.Background()

	defaultRef, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	// A second exact FormRef for the same Kind, in another namespaced group.
	priorRef := currentformregistry.V3Ref{
		APIVersion:        "prior-edge.forms.takoform.com/v1alpha1",
		Kind:              defaultRef.Kind,
		DefinitionVersion: "0.0.9",
		SchemaDigest:      "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		PackageDigest:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	resource.supportedRefs = []currentformregistry.V3Ref{defaultRef, priorRef}
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":  types.StringValue("module-worker"),
		"space": types.StringValue("prod"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	// The create used the recommended default target, not the prior line.
	createdForm := host.applyBodies[0]["form"].(map[string]any)["formRef"].(map[string]any)
	if createdForm["apiVersion"] != defaultRef.APIVersion || createdForm["schemaDigest"] != defaultRef.SchemaDigest {
		t.Fatalf("create did not target the default FormRef: %#v", createdForm)
	}

	// Rewrite state onto the prior FormRef, as a state file written by an older
	// provider build would carry it.
	priorState := tfsdk.State{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	for name, value := range map[string]string{
		"form_api_version":        priorRef.APIVersion,
		"form_definition_version": priorRef.DefinitionVersion,
		"form_schema_digest":      priorRef.SchemaDigest,
	} {
		if diags := priorState.SetAttribute(ctx, path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatalf("seed prior state %s: %v", name, diags)
		}
	}

	readResponse := frameworkresource.ReadResponse{State: priorState}
	resource.Read(ctx, frameworkresource.ReadRequest{State: priorState}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read under the prior state FormRef: %v", readResponse.Diagnostics)
	}
	wantRead := "GET " + priorRef.APIVersion + " " + priorRef.SchemaDigest
	if len(host.resourceQueries) != 1 || host.resourceQueries[0] != wantRead {
		t.Fatalf("read dispatch = %v, want [%q]", host.resourceQueries, wantRead)
	}
	// The read must project the state FormRef back, not silently rebind to the
	// default create target.
	if got := v3StateString(t, ctx, readResponse.State, "form_api_version").ValueString(); got != priorRef.APIVersion {
		t.Fatalf("read rebound state to %q, want the recorded %q", got, priorRef.APIVersion)
	}

	deleteResponse := frameworkresource.DeleteResponse{}
	resource.Delete(ctx, frameworkresource.DeleteRequest{State: readResponse.State}, &deleteResponse)
	if deleteResponse.Diagnostics.HasError() {
		t.Fatalf("delete under the prior state FormRef: %v", deleteResponse.Diagnostics)
	}
	wantDelete := "DELETE " + priorRef.APIVersion + " " + priorRef.SchemaDigest
	if len(host.resourceQueries) != 2 || host.resourceQueries[1] != wantDelete {
		t.Fatalf("delete dispatch = %v, want [..., %q]", host.resourceQueries, wantDelete)
	}
}

// TestV3JSONObjectAttributesRejectTheLiteralNull covers both data-only JSON
// attributes. encoding/json accepts `null` into a map target and leaves the map
// nil, so without an explicit guard a nil desired document travels to the host
// as an absent object.
func TestV3JSONObjectAttributesRejectTheLiteralNull(t *testing.T) {
	ctx := context.Background()

	t.Run("vars_json", func(t *testing.T) {
		host := newV3FakeHost(t)
		resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
		schemaResponse := v3SchemaOf(t, resource)
		if _, declared := schemaResponse.Schema.Attributes["vars_json"]; !declared {
			t.Fatal("WorkerVersion no longer declares vars_json")
		}
		plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name":               types.StringValue("worker-version"),
			"worker":             types.StringValue("module-worker"),
			"bundle":             types.StringValue("worker-bundle"),
			"compatibility_date": types.StringValue("2026-08-06"),
			"handlers":           types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
			"vars_json":          types.StringValue("null"),
		})
		createResponse := frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
		assertNullJSONObjectRejected(t, createResponse.Diagnostics, "vars_json")
		if len(host.applySpecs) != 0 {
			t.Fatal("a null vars_json reached the host")
		}
	})

	t.Run("spec_json", func(t *testing.T) {
		host := newV3FakeHost(t)
		generic := &v3GenericResource{data: newV3TestProviderData(t, host)}
		schemaResponse := v3SchemaOf(t, generic)
		plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"form_api_version":        types.StringValue("forms.example.com/v1alpha1"),
			"form_kind":               types.StringValue("Widget"),
			"form_definition_version": types.StringValue("1.0.0"),
			"form_schema_digest":      types.StringValue("sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
			"name":                    types.StringValue("my-widget"),
			"spec_json":               types.StringValue("null"),
		})
		createResponse := frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		generic.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
		assertNullJSONObjectRejected(t, createResponse.Diagnostics, "spec_json")
		if len(host.applySpecs) != 0 {
			t.Fatal("a null spec_json reached the host")
		}
	})

	t.Run("validator", func(t *testing.T) {
		if _, err := v3ParseJSONObject("null"); err == nil {
			t.Fatal("v3ParseJSONObject accepted the literal null")
		}
		if parsed, err := v3ParseJSONObject("{}"); err != nil || parsed == nil {
			t.Fatalf("v3ParseJSONObject rejected the empty object: %v", err)
		}
	})
}

func assertNullJSONObjectRejected(t *testing.T, diagnostics diag.Diagnostics, attribute string) {
	t.Helper()
	for _, diagnostic := range diagnostics.Errors() {
		withPath, ok := diagnostic.(diag.DiagnosticWithPath)
		if !ok {
			continue
		}
		if withPath.Path().Equal(path.Root(attribute)) && strings.Contains(diagnostic.Detail(), "null") {
			return
		}
	}
	t.Fatalf("no attribute error named %s and the literal null: %v", attribute, diagnostics)
}
