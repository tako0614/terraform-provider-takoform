package provider

// v3_outputs_test.go covers the typed computed attributes derived from a Form's
// declared outputSchema (spec/decisions/0025).
//
// The interesting cases are not "the happy path types a string". They are the
// three places a computed attribute goes wrong in Terraform: a schema that
// declares the attribute in the wrong mode, a state write that leaves it unset
// on a recovery path, and a refresh that carries a stale value forward. All
// three produce the same two symptoms in the field — "Provider produced
// inconsistent result after apply" and a perpetual diff — long after the change
// that caused them.

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	v3TestEndpointHostname = "e0123456789abcdef.endpoints.example.invalid"
	v3TestEndpointURL      = "https://" + v3TestEndpointHostname + "/"
)

// v3EndpointOutputs is the address a host assigns to the endpoint probe used
// throughout this file. Nothing in any configuration below writes it.
func v3EndpointOutputs() map[string]any {
	return map[string]any{"hostname": v3TestEndpointHostname, "url": v3TestEndpointURL}
}

// TestV3DeclaredOutputsAreTypedComputedAttributes proves the schema shape.
//
// Optional would advertise an argument the provider must then refuse; a
// framework Default would put a value in the plan that no host produced. Both
// are the classic ways a computed attribute acquires a perpetual diff, so the
// mode is asserted rather than assumed.
func TestV3DeclaredOutputsAreTypedComputedAttributes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, form := range mustProviderV3SnapshotAssembly().currentForms {
		if len(form.Outputs) == 0 {
			continue
		}
		resource := v3Provider3CurrentResourceHarness(t, form, "", nil, v3Codecs())
		var response frameworkresource.SchemaResponse
		resource.Schema(ctx, frameworkresource.SchemaRequest{}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("%s schema: %v", form.Kind, response.Diagnostics)
		}
		for _, output := range form.Outputs {
			attribute, declared := response.Schema.Attributes[output.AttributeName()]
			if !declared {
				t.Fatalf("%s declares output %s but the schema has no such attribute", form.Kind, output.Wire)
			}
			if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
				t.Errorf(
					"%s.%s is computed=%v optional=%v required=%v; a host-computed output is plain Computed",
					form.Kind, output.AttributeName(),
					attribute.IsComputed(), attribute.IsOptional(), attribute.IsRequired(),
				)
			}
			if text, ok := attribute.(schema.StringAttribute); ok && text.Default != nil {
				t.Errorf(
					"%s.%s carries a framework default; no default can describe a value the host computes",
					form.Kind, output.AttributeName(),
				)
			}
		}
	}
}

// TestV3ReservedAttributesCoverTheEnvelope proves the authoring model's
// reserved list is the provider's actual envelope surface.
//
// The model is what refuses a Form whose field or output would take an envelope
// name, and it can only do that against a list. A list that drifted from the
// provider's real attribute set would refuse harmless names and, far worse,
// admit a colliding one — after which one attribute would hold two facts and
// whichever the resource wrote last would win.
func TestV3ReservedAttributesCoverTheEnvelope(t *testing.T) {
	t.Parallel()
	reserved := map[string]bool{}
	for _, name := range model.ReservedResourceAttributes {
		reserved[name] = true
	}
	for name := range v3CommonAttributes(model.Form{Kind: "Probe", Role: model.RoleIdentity, Fields: []model.Field{{HCL: "probe", Wire: "probe", Kind: model.KindString}}}) {
		if !reserved[name] {
			t.Errorf(
				"the v3 resource surface carries %q, which currentformmodel.ReservedResourceAttributes does not list; "+
					"a Form could declare a field or output under that name and the model would not refuse it",
				name,
			)
		}
	}
}

// TestV3TypedOutputsSurviveCreateReadAndDrift walks the paths that actually
// break: create, a read that adopts a CHANGED host value, and the drift
// condition decision 0017 cares about.
//
// The read case is the one worth writing. An output is host-computed state, so
// a provider that carried the prior value forward on refresh would report an
// address the host has stopped serving, and no plan would ever correct it
// because no desired attribute changed.
func TestV3TypedOutputsSurviveCreateReadAndDrift(t *testing.T) {
	host := newV3FakeHost(t)
	host.assignedOutputs = v3EndpointOutputs()
	resource := v3TestFormResource(t, "WorkerEndpoint", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":   types.StringValue("worker-endpoint"),
		"worker": types.StringValue("module-worker"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	for name, want := range map[string]string{
		"hostname": v3TestEndpointHostname,
		"url":      v3TestEndpointURL,
	} {
		if got := v3StateString(t, ctx, createResponse.State, name).ValueString(); got != want {
			t.Errorf("created state %s = %q, want %q", name, got, want)
		}
	}
	// The whole document is still reachable, unnarrowed: an existing
	// configuration decoding outputs_json keeps reading the same members.
	wantJSON := `{"hostname":"` + v3TestEndpointHostname + `","url":"` + v3TestEndpointURL + `"}`
	if got := v3StateString(t, ctx, createResponse.State, "outputs_json").ValueString(); got != wantJSON {
		t.Errorf("outputs_json = %q, want the whole document %q", got, wantJSON)
	}
	// The desired attribute is untouched by any of it: an assigned address is
	// status, and a provider that wrote it into a desired attribute would plan a
	// change on every refresh.
	if got := v3StateString(t, ctx, createResponse.State, "worker").ValueString(); got != "module-worker" {
		t.Errorf("worker = %q, want the configured module-worker", got)
	}

	// The host reassigns the address out of band. A refresh must adopt it.
	const movedHostname = "e99999999999999ff.endpoints.example.invalid"
	host.reassignOutputs("WorkerEndpoint", "worker-endpoint", map[string]any{
		"hostname": movedHostname,
		"url":      "https://" + movedHostname + "/",
	})
	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, readResponse.State, "hostname").ValueString(); got != movedHostname {
		t.Errorf("refreshed hostname = %q, want the host's current %q", got, movedHostname)
	}

	// Relation drift is reported without disturbing the typed outputs: the
	// reference is broken, the address is still what the host serves.
	host.relationDriftReason = "ExternalChange"
	driftResponse := frameworkresource.ReadResponse{State: readResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: readResponse.State}, &driftResponse)
	if driftResponse.Diagnostics.HasError() {
		t.Fatalf("read under relation drift: %v", driftResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, driftResponse.State, v3RelationDriftAttribute).ValueString(); got != "ExternalChange" {
		t.Fatalf("relation_drift_reason = %q, want ExternalChange", got)
	}
	if got := v3StateString(t, ctx, driftResponse.State, "hostname").ValueString(); got != movedHostname {
		t.Errorf("hostname under relation drift = %q, want %q", got, movedHostname)
	}
}

// TestV3TypedOutputsAreWrittenOnAnInterruptedApply is the framework-contract
// case (spec/decisions/0017).
//
// A create the host ACCEPTED but did not finish writes recovery state and
// returns no representation. Terraform commits that state, and the plan that
// produced it marked every computed attribute unknown — so an output the
// provider leaves unset here is "Provider produced inconsistent result after
// apply", on the one path nobody exercises by hand. Null is the correct answer
// and it must be written.
func TestV3TypedOutputsAreWrittenOnAnInterruptedApply(t *testing.T) {
	host := newV3FakeHost(t)
	host.assignedOutputs = v3EndpointOutputs()
	host.apply202Pending = true
	resource := v3TestFormResource(t, "WorkerEndpoint", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":           types.StringValue("worker-endpoint"),
		"worker":         types.StringValue("module-worker"),
		"create_timeout": types.StringValue("400ms"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if !createResponse.Diagnostics.HasError() {
		t.Fatal("an accepted-but-unfinished create must report an error")
	}
	if got := v3StateString(t, ctx, createResponse.State, "pending_operation_id").ValueString(); got == "" {
		t.Fatal("the interrupted create recorded no pending_operation_id")
	}
	for _, name := range []string{"hostname", "url"} {
		var value types.String
		if diags := createResponse.State.GetAttribute(ctx, path.Root(name), &value); diags.HasError() {
			t.Fatalf("get %s: %v", name, diags)
		}
		if !value.IsNull() {
			t.Errorf("%s = %v after an interrupted apply, want null", name, value)
		}
		if value.IsUnknown() {
			t.Errorf("%s is still unknown after apply; the framework rejects an unset computed attribute", name)
		}
	}
	// And the null is not reported as the host's fault, because it is not one.
	// This is the whole distinction: no representation came back, so there is no
	// declared output to have been missing from it.
	if summary := v3OutputFaultSummary(createResponse.Diagnostics.Errors()); summary != "" {
		t.Fatalf("the accepted-but-unfinished path reported %q; a null output is correct there", summary)
	}
}

// TestV3AMalformedDeclaredOutputFailsTheWrite is the other side of that
// distinction (spec/decisions/0025 rule 6).
//
// A response carrying `status` carries every output the Form declares, with the
// declared type. Recording a violation as a null would leave the practitioner
// with `takoform_worker_endpoint.x.url == null` — no address, no error, and
// nothing naming the host that owed them one — while every expression consuming
// it silently evaluates to null too.
func TestV3AMalformedDeclaredOutputFailsTheWrite(t *testing.T) {
	for _, test := range []struct {
		name    string
		outputs map[string]any
		want    string
	}{
		{
			name:    "the declared member is omitted",
			outputs: map[string]any{"url": v3TestEndpointURL},
			want:    "status.outputs omits it",
		},
		{
			name:    "the whole outputs document is absent",
			outputs: nil,
			want:    "status.outputs omits it",
		},
		{
			name:    "the declared member has the wrong type",
			outputs: map[string]any{"hostname": 8443, "url": v3TestEndpointURL},
			want:    "declared string and the host returned the number 8443",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := newV3FakeHost(t)
			host.assignedOutputs = test.outputs
			resource := v3TestFormResource(t, "WorkerEndpoint", newV3TestProviderData(t, host))
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)
			plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
				"name":   types.StringValue("worker-endpoint"),
				"worker": types.StringValue("module-worker"),
			})
			createResponse := frameworkresource.CreateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
			if !createResponse.Diagnostics.HasError() {
				t.Fatal("a create whose response violated the Form's output contract succeeded")
			}
			summary := v3OutputFaultSummary(createResponse.Diagnostics.Errors())
			if summary != "Host response does not satisfy the declared output hostname" {
				t.Fatalf("diagnostic summary = %q, want one naming the missing output", summary)
			}
			if detail := v3OutputFaultDetail(createResponse.Diagnostics.Errors()); !strings.Contains(detail, test.want) {
				t.Fatalf("diagnostic detail = %q, want it to say %q", detail, test.want)
			}
		})
	}
}

// TestV3AMalformedDeclaredOutputFailsARefresh proves a refresh is held to the
// same rule. An address the host stops publishing is not drift a plan can
// converge — no desired attribute changed — so a quiet null would be permanent.
func TestV3AMalformedDeclaredOutputFailsARefresh(t *testing.T) {
	host := newV3FakeHost(t)
	host.assignedOutputs = v3EndpointOutputs()
	resource := v3TestFormResource(t, "WorkerEndpoint", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":   types.StringValue("worker-endpoint"),
		"worker": types.StringValue("module-worker"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	host.reassignOutputs("WorkerEndpoint", "worker-endpoint", map[string]any{"url": v3TestEndpointURL})
	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if !readResponse.Diagnostics.HasError() {
		t.Fatal("a refresh whose response dropped a declared output succeeded")
	}
	if summary := v3OutputFaultSummary(readResponse.Diagnostics.Errors()); summary == "" {
		t.Fatalf("refresh diagnostics = %v, want one naming the dropped output", readResponse.Diagnostics)
	}
}

// v3OutputFaultSummary returns the summary of the declared-output diagnostic,
// or "" when no diagnostic reported one.
func v3OutputFaultSummary(errors []diag.Diagnostic) string {
	for _, diagnostic := range errors {
		if strings.HasPrefix(diagnostic.Summary(), "Host response does not satisfy the declared output") {
			return diagnostic.Summary()
		}
	}
	return ""
}

// v3OutputFaultDetail returns the detail of the declared-output diagnostic.
func v3OutputFaultDetail(errors []diag.Diagnostic) string {
	for _, diagnostic := range errors {
		if strings.HasPrefix(diagnostic.Summary(), "Host response does not satisfy the declared output") {
			return diagnostic.Detail()
		}
	}
	return ""
}

// TestV3FormsWithoutOutputsDeclareNoOutputAttributes is the negative side. A
// Form that publishes no outputs must not grow an attribute for one, because
// `status.outputs` is omitted on the wire for such a Form and the attribute
// would be permanently null with nothing able to fill it.
func TestV3FormsWithoutOutputsDeclareNoOutputAttributes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	declaring := 0
	for _, form := range mustProviderV3SnapshotAssembly().currentForms {
		if len(form.Outputs) > 0 {
			declaring++
			continue
		}
		resource := v3Provider3CurrentResourceHarness(t, form, "", nil, v3Codecs())
		var response frameworkresource.SchemaResponse
		resource.Schema(ctx, frameworkresource.SchemaRequest{}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("%s schema: %v", form.Kind, response.Diagnostics)
		}
		want := len(v3CommonAttributes(form))
		if artifact, artifactBacked := v3ProviderArtifactForForm(t, form); artifactBacked && artifact.Mode == v3ArtifactModeWorkerBundle {
			want += len(workerBundleAttributesForProjection(*artifact))
		} else if artifact != nil && artifact.Mode == v3ArtifactModeFileBundle {
			want += len(fileBundleAttributesForProjection(*artifact))
		} else {
			want += len(form.Fields)
		}
		if form.Kind == workerVersionKind {
			// One computed-only attribute preserves retained Provider 2.1.1
			// WorkerVersion state. It is not a current Form field or output.
			want++
		}
		if got := len(response.Schema.Attributes); got != want {
			t.Errorf("%s declares %d attributes, want exactly the %d it derives", form.Kind, got, want)
		}
	}
	if declaring == 0 {
		t.Fatal("no family Form declares outputs, so this test proves nothing about the ones that do")
	}
}
