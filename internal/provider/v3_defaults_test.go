package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// v3AttributeDefaultValue resolves the framework default of one attribute back
// to its typed value, the same way the framework resolves it while building a
// plan for a null configuration value.
func v3AttributeDefaultValue(t *testing.T, attribute schema.Attribute) (attr.Value, bool) {
	t.Helper()
	ctx := context.Background()
	switch typed := attribute.(type) {
	case schema.BoolAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.BoolResponse
		typed.Default.DefaultBool(ctx, defaults.BoolRequest{}, &response)
		return response.PlanValue, true
	case schema.Int64Attribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.Int64Response
		typed.Default.DefaultInt64(ctx, defaults.Int64Request{}, &response)
		return response.PlanValue, true
	case schema.StringAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.StringResponse
		typed.Default.DefaultString(ctx, defaults.StringRequest{}, &response)
		return response.PlanValue, true
	case schema.SetAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.SetResponse
		typed.Default.DefaultSet(ctx, defaults.SetRequest{}, &response)
		return response.PlanValue, true
	case schema.ListAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.ListResponse
		typed.Default.DefaultList(ctx, defaults.ListRequest{}, &response)
		return response.PlanValue, true
	case schema.MapAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.MapResponse
		typed.Default.DefaultMap(ctx, defaults.MapRequest{}, &response)
		return response.PlanValue, true
	case schema.ListNestedAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.ListResponse
		typed.Default.DefaultList(ctx, defaults.ListRequest{}, &response)
		return response.PlanValue, true
	case schema.SingleNestedAttribute:
		if typed.Default == nil {
			return nil, false
		}
		var response defaults.ObjectResponse
		typed.Default.DefaultObject(ctx, defaults.ObjectRequest{}, &response)
		return response.PlanValue, true
	default:
		return nil, false
	}
}

func v3AttributeIsOptionalComputed(attribute schema.Attribute) bool {
	return attribute.IsOptional() && attribute.IsComputed()
}

// TestV3DefaultedAttributesSurviveApplyWithoutDiff is the regression proof for
// the Optional+Computed fix. A second plan is empty only when the whole chain
// agrees for every defaulted attribute:
//
//	config null → framework default D → wire value W → host echo W →
//	state value read back from W → D again.
//
// A missing framework default breaks the first arrow (plan reads null while
// the host answers the default); a dropped empty collection breaks the second
// (the host never receives what the plan promised). Either way the operator
// gets a diff that never converges, so this walks the entire chain per field.
func TestV3DefaultedAttributesSurviveApplyWithoutDiff(t *testing.T) {
	ctx := context.Background()
	checked := 0
	for _, form := range edgeformcatalog.Forms {
		if v3ArtifactBackedRevision(form.Kind) {
			// Artifact-backed revisions project manifestDigest through their
			// provider-only local-file authoring surfaces.
			continue
		}
		resource := v3Provider3CurrentResourceHarness(t, form, "", nil, nil)
		schemaResponse := v3SchemaOf(t, resource)
		for _, field := range form.Fields {
			name := v3AttributeName(field)
			attribute, present := schemaResponse.Schema.Attributes[name]
			if !present {
				t.Fatalf("%s: schema has no attribute %s", form.Kind, name)
			}
			if field.Default == nil {
				if _, hasDefault := v3AttributeDefaultValue(t, attribute); hasDefault {
					t.Errorf("%s.%s declares a framework default the Form does not", form.Kind, name)
				}
				if attribute.IsComputed() {
					t.Errorf("%s.%s is Computed without a declared portable default", form.Kind, name)
				}
				continue
			}
			checked++
			if !v3AttributeIsOptionalComputed(attribute) {
				t.Errorf("%s.%s declares a default but is not Optional+Computed", form.Kind, name)
				continue
			}
			planned, hasDefault := v3AttributeDefaultValue(t, attribute)
			if !hasDefault {
				t.Errorf("%s.%s declares a portable default but no framework default", form.Kind, name)
				continue
			}

			// plan → wire
			wire, diags := v3FieldToWire(ctx, form.Family.APIVersion(), field, name, planned)
			if diags.HasError() {
				t.Errorf("%s.%s: projecting the default onto the wire: %v", form.Kind, name, diags)
				continue
			}
			if wire == nil {
				t.Errorf("%s.%s: the defaulted value was dropped instead of sent", form.Kind, name)
				continue
			}
			gotJSON, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(field.Default)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("%s.%s wire value = %s, want the declared default %s",
					form.Kind, name, gotJSON, wantJSON)
				continue
			}

			// host echo → state, which must be exactly what the plan held.
			var readDiags diag.Diagnostics
			roundTripped := v3FieldValueFromSpec(ctx, form.Family.APIVersion(), field, decodedWire(t, wire), &readDiags)
			if readDiags.HasError() {
				t.Errorf("%s.%s: reading the echoed default back: %v", form.Kind, name, readDiags)
				continue
			}
			if !roundTripped.Equal(planned) {
				t.Errorf("%s.%s: state after apply = %v, plan held %v; the next plan would show a diff",
					form.Kind, name, roundTripped, planned)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no defaulted attribute was exercised; the table lost its subject")
	}
}

// decodedWire round-trips a projected wire value through JSON exactly as it
// travels to the host and back, so numbers arrive as json.Number the way a
// real response body does.
func decodedWire(t *testing.T, wire any) any {
	t.Helper()
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// TestV3WorkerVersionOmittedDefaultsTravelAndReturn drives the chain the table
// test models through a real create and read against the fake host: every
// defaulted attribute left at its framework default reaches the host and comes
// back into state unchanged.
func TestV3WorkerVersionOmittedDefaultsTravelAndReturn(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	form, _ := edgeformcatalog.ByKind("WorkerVersion")
	planValues := map[string]attr.Value{
		"name":     types.StringValue("worker-version"),
		"space":    types.StringValue("prod"),
		"worker":   types.StringValue("module-worker"),
		"bundle":   types.StringValue("worker-bundle"),
		"handlers": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
	}
	defaulted := map[string]attr.Value{}
	for _, field := range form.Fields {
		if field.Default == nil {
			continue
		}
		name := v3AttributeName(field)
		value, ok := v3AttributeDefaultValue(t, schemaResponse.Schema.Attributes[name])
		if !ok {
			t.Fatalf("%s has no framework default", name)
		}
		planValues[name] = value
		defaulted[name] = value
	}
	// vars_json, the six binding lists, required_sensitive_vars, and
	// external_services.
	if len(defaulted) != 9 {
		t.Fatalf("WorkerVersion exercised %d defaulted attributes, want 9", len(defaulted))
	}

	plan := v3PlanWith(t, ctx, schemaResponse, planValues)
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if len(host.applySpecs) != 1 {
		t.Fatalf("host saw %d applies, want 1", len(host.applySpecs))
	}
	sent := host.applySpecs[0]
	for _, field := range form.Fields {
		if field.Default == nil {
			continue
		}
		if _, present := sent[field.Wire]; !present {
			t.Errorf("the wire spec dropped defaulted field %s; the host would materialize it back and the plan would never settle", field.Wire)
		}
	}

	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	for name, want := range defaulted {
		got := v3StateValue(t, ctx, readResponse.State, name, want)
		if !got.Equal(want) {
			t.Errorf("state %s after read = %v, plan held %v", name, got, want)
		}
	}
}

// TestV3NoUpdateFormsDeclareNoUpdateTimeout pins the surface consequence of a
// Form without the update capability: there is no update deadline to set,
// because there is no update.
func TestV3NoUpdateFormsDeclareNoUpdateTimeout(t *testing.T) {
	for _, form := range edgeformcatalog.Forms {
		resource := v3Provider3CurrentResourceHarness(t, form, "", nil, nil)
		schemaResponse := v3SchemaOf(t, resource)
		_, present := schemaResponse.Schema.Attributes["update_timeout"]
		if present != form.DeclaresUpdate() {
			t.Errorf("%s declares update=%v but update_timeout present=%v",
				form.Kind, form.DeclaresUpdate(), present)
		}
		if form.DeclaresUpdate() {
			continue
		}
		// Without an in-place update path, every desired attribute must force
		// replacement; otherwise the plan would propose a change the provider
		// has no way to perform.
		for _, field := range form.Fields {
			attribute := schemaResponse.Schema.Attributes[v3AttributeName(field)]
			if !v3AttributeRequiresReplace(attribute) {
				t.Errorf("%s.%s does not force replacement on a Form with no update capability",
					form.Kind, field.HCL)
			}
		}
	}
}

func v3AttributeRequiresReplace(attribute schema.Attribute) bool {
	switch typed := attribute.(type) {
	case schema.BoolAttribute:
		return len(typed.PlanModifiers) > 0
	case schema.Int64Attribute:
		return len(typed.PlanModifiers) > 0
	case schema.StringAttribute:
		return len(typed.PlanModifiers) > 0
	case schema.SetAttribute:
		return len(typed.PlanModifiers) > 0
	case schema.ListAttribute:
		return len(typed.PlanModifiers) > 0
	case schema.MapAttribute:
		return len(typed.PlanModifiers) > 0
	case schema.ListNestedAttribute:
		return len(typed.PlanModifiers) > 0
	case schema.SingleNestedAttribute:
		return len(typed.PlanModifiers) > 0
	default:
		return false
	}
}

// TestV3DefaultedFieldModelIsInternallyConsistent guards the one shape the
// wire projection special-cases: an empty collection default must be emitted,
// never dropped, and that decision comes from the declared default alone.
func TestV3EmptyCollectionDefaultsAreEmitted(t *testing.T) {
	ctx := context.Background()
	form, _ := edgeformcatalog.ByKind("WorkerVersion")
	for _, field := range form.Fields {
		if !model.EmptyCollectionDefault(field) {
			continue
		}
		name := v3AttributeName(field)
		resource := v3Provider3CurrentResourceHarness(t, form, "", nil, nil)
		attribute := v3SchemaOf(t, resource).Schema.Attributes[name]
		planned, ok := v3AttributeDefaultValue(t, attribute)
		if !ok {
			t.Fatalf("%s has no framework default", name)
		}
		wire, diags := v3FieldToWire(ctx, form.Family.APIVersion(), field, name, planned)
		if diags.HasError() || wire == nil {
			t.Fatalf("%s: empty collection default was dropped (%v)", name, diags)
		}
	}
}

// v3StateValue reads one attribute out of state as the same concrete typed
// value the plan held, so the comparison is value-for-value.
func v3StateValue(t *testing.T, ctx context.Context, state tfsdk.State, name string, like attr.Value) attr.Value {
	t.Helper()
	get := func(target any) {
		if diags := state.GetAttribute(ctx, path.Root(name), target); diags.HasError() {
			t.Fatalf("get state %s: %v", name, diags)
		}
	}
	switch like.(type) {
	case types.Bool:
		var value types.Bool
		get(&value)
		return value
	case types.Int64:
		var value types.Int64
		get(&value)
		return value
	case types.Set:
		var value types.Set
		get(&value)
		return value
	case types.List:
		var value types.List
		get(&value)
		return value
	case types.Map:
		var value types.Map
		get(&value)
		return value
	case types.Object:
		var value types.Object
		get(&value)
		return value
	default:
		var value types.String
		get(&value)
		return value
	}
}
