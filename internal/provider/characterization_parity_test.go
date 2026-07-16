package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	frameworkdiag "github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/tako0614/terraform-provider-takoform/internal/characterization"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

func TestCompatibilityCandidateProviderParity(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "conformance", "compatibility-candidate-v1")
	ctx := context.Background()

	schemaDoc := mustLoadCases[characterization.ProviderSchemaCase](t, root, "providerSchema")
	desiredDoc := mustLoadCases[characterization.ResourceCase](t, root, "desired")
	observedDoc := mustLoadCases[characterization.ResourceCase](t, root, "observed")
	outputDoc := mustLoadCases[characterization.OutputCase](t, root, "output")
	importDoc := mustLoadCases[characterization.ImportCase](t, root, "import")
	errorDoc := mustLoadCases[characterization.ErrorCase](t, root, "error")

	desired := resourceCasesByKind(t, desiredDoc.Cases)
	observed := resourceCasesByKind(t, observedDoc.Cases)
	outputs := outputCasesByKind(outputDoc.Cases)
	imports := importCasesByKind(importDoc.Cases)
	errors := errorCasesByKind(errorDoc.Cases)

	for _, fixture := range schemaDoc.Cases {
		fixture := fixture
		t.Run(fixture.Kind, func(t *testing.T) {
			resource := candidateResourceForKind(t, fixture.Kind)
			var schemaResponse frameworkresource.SchemaResponse
			resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
			if schemaResponse.Diagnostics.HasError() {
				t.Fatalf("schema diagnostics: %v", schemaResponse.Diagnostics)
			}
			gotSchema := characterizeAttributes(t, schemaResponse.Schema.Attributes)
			if !reflect.DeepEqual(gotSchema, fixture.Attributes) {
				want, _ := json.MarshalIndent(fixture.Attributes, "", "  ")
				got, _ := json.MarshalIndent(gotSchema, "", "  ")
				t.Fatalf("provider schema drifted from candidate fixture\nwant: %s\n got: %s", want, got)
			}

			wantDesired := desired[fixture.Kind]
			gotDesired := providerDesiredResource(t, ctx, fixture.Kind, wantDesired)
			assertSameJSON(t, gotDesired, wantDesired)

			observedResource := observed[fixture.Kind]
			gotOutput := providerOutputState(t, ctx, fixture.Kind, observedResource)
			if !reflect.DeepEqual(gotOutput, outputs[fixture.Kind].State) {
				t.Fatalf("provider output drifted\nwant: %#v\n got: %#v", outputs[fixture.Kind].State, gotOutput)
			}

			importFixture := imports[fixture.Kind]
			space, name, ok := cutSpaceName(importFixture.Input)
			if !ok || space != importFixture.Expected.Space || name != importFixture.Expected.Name {
				t.Fatalf("import parsing drifted: ok=%v space=%q name=%q", ok, space, name)
			}

			assertProviderError(t, ctx, errors[fixture.Kind], wantDesired)
		})
	}
}

func candidateResourceForKind(t *testing.T, kind string) frameworkresource.Resource {
	t.Helper()
	switch kind {
	case client.KindEdgeWorker:
		return NewEdgeWorkerResource()
	case client.KindObjectBucket:
		return NewObjectBucketResource()
	case client.KindKVStore:
		return NewKVStoreResource()
	case client.KindQueue:
		return NewQueueResource()
	case client.KindSQLDatabase:
		return NewSQLDatabaseResource()
	case client.KindContainerService:
		return NewContainerServiceResource()
	case client.KindVectorIndex:
		return NewVectorIndexResource()
	case client.KindDurableWorkflow:
		return NewDurableWorkflowResource()
	case client.KindStatefulActorNamespace:
		return NewStatefulActorNamespaceResource()
	case client.KindSchedule:
		return NewScheduleResource()
	default:
		t.Fatalf("unknown candidate kind %q", kind)
		return nil
	}
}

func providerDesiredResource(t *testing.T, ctx context.Context, kind string, fixture client.Resource) *client.Resource {
	t.Helper()
	if kind == client.KindEdgeWorker {
		var model edgeWorkerModel
		if diags := refreshEdgeWorkerSpec(&fixture, &model); diags.HasError() {
			t.Fatalf("refresh EdgeWorker: %v", diags)
		}
		got, _, diags := model.toResource(ctx, "")
		assertNoDiagnostics(t, diags)
		return got
	}
	shape := candidateResourceForKind(t, kind).(*serviceShapeResource)
	var model serviceShapeModel
	if diags := refreshServiceShapeSpec(ctx, &fixture, shape.cfg.spec, &model); diags.HasError() {
		t.Fatalf("refresh %s: %v", kind, diags)
	}
	got, _, diags := model.toResource(ctx, "", kind, shape.cfg.spec)
	assertNoDiagnostics(t, diags)
	return got
}

func providerOutputState(t *testing.T, ctx context.Context, kind string, fixture client.Resource) characterization.OutputState {
	t.Helper()
	if kind == client.KindEdgeWorker {
		var model edgeWorkerModel
		assertNoDiagnostics(t, refreshEdgeWorkerSpec(&fixture, &model))
		assertNoDiagnostics(t, applyEdgeWorkerStatus(ctx, &fixture, fixture.Metadata.Space, &model))
		return outputState(model.ID, model.Name, model.Space, model.SelectedImplementation, model.Target, model.Locked, model.Portability, model.Outputs)
	}
	shape := candidateResourceForKind(t, kind).(*serviceShapeResource)
	var model serviceShapeModel
	assertNoDiagnostics(t, refreshServiceShapeSpec(ctx, &fixture, shape.cfg.spec, &model))
	assertNoDiagnostics(t, applyServiceShapeStatus(ctx, &fixture, kind, fixture.Metadata.Space, &model))
	return outputState(model.ID, model.Name, model.Space, model.SelectedImplementation, model.Target, model.Locked, model.Portability, model.Outputs)
}

func outputState(id, name, space, selected, target types.String, locked types.Bool, portability types.String, outputs types.Map) characterization.OutputState {
	values := make(map[string]string, len(outputs.Elements()))
	for key, value := range outputs.Elements() {
		values[key] = value.(types.String).ValueString()
	}
	return characterization.OutputState{
		ID: id.ValueString(), Name: name.ValueString(), Space: space.ValueString(),
		SelectedImplementation: selected.ValueString(), Target: target.ValueString(),
		Locked: locked.ValueBool(), Portability: portability.ValueString(), Outputs: values,
	}
}

func assertProviderError(t *testing.T, ctx context.Context, fixture characterization.ErrorCase, desired client.Resource) {
	t.Helper()
	var diags frameworkdiag.Diagnostics
	if fixture.Kind == client.KindEdgeWorker {
		var model edgeWorkerModel
		assertNoDiagnostics(t, refreshEdgeWorkerSpec(&desired, &model))
		model.ArtifactPath = types.StringNull()
		model.ArtifactURL = types.StringNull()
		model.ArtifactRef = types.StringNull()
		model.ArtifactSHA256 = types.StringNull()
		_, _, diags = model.toResource(ctx, "")
	} else {
		shape := candidateResourceForKind(t, fixture.Kind).(*serviceShapeResource)
		var model serviceShapeModel
		assertNoDiagnostics(t, refreshServiceShapeSpec(ctx, &desired, shape.cfg.spec, &model))
		switch fixture.Scenario {
		case "invalid_storage_class":
			model.StorageClass = types.StringValue("archive")
		case "missing_space":
			model.Space = types.StringNull()
		case "nonpositive_dimensions":
			model.Dimensions = types.Int64Value(0)
		case "missing_artifact_source":
			model.ArtifactPath = types.StringNull()
			model.ArtifactURL = types.StringNull()
			model.ArtifactRef = types.StringNull()
			model.ArtifactSHA256 = types.StringNull()
		case "invalid_class_name":
			model.ClassName = types.StringValue("invalid class")
		case "invalid_cron":
			model.Cron = types.StringValue("not a cron")
		default:
			t.Fatalf("unknown provider error scenario %q", fixture.Scenario)
		}
		_, _, diags = model.toResource(ctx, "", fixture.Kind, shape.cfg.spec)
	}
	if len(diags) == 0 || !diags.HasError() {
		t.Fatalf("scenario %q produced no error diagnostic", fixture.Scenario)
	}
	if diags[0].Summary() != fixture.Expected.Summary {
		t.Fatalf("diagnostic summary = %q, want %q", diags[0].Summary(), fixture.Expected.Summary)
	}
	withPath, ok := diags[0].(frameworkdiag.DiagnosticWithPath)
	if !ok || withPath.Path().String() != fixture.Expected.Path {
		t.Fatalf("diagnostic path = %v, want %q", withPath, fixture.Expected.Path)
	}
}

func characterizeAttributes(t *testing.T, attributes map[string]schema.Attribute) []characterization.AttributeCase {
	t.Helper()
	names := make([]string, 0, len(attributes))
	for name := range attributes {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]characterization.AttributeCase, 0, len(names))
	for _, name := range names {
		result = append(result, characterizeAttribute(t, name, attributes[name]))
	}
	return result
}

func characterizeAttribute(t *testing.T, name string, attribute schema.Attribute) characterization.AttributeCase {
	t.Helper()
	result := characterization.AttributeCase{Name: name}
	switch value := attribute.(type) {
	case schema.StringAttribute:
		result.Type, result.Required, result.Optional, result.Computed = "string", value.Required, value.Optional, value.Computed
		result.Validators, result.PlanModifiers = len(value.Validators), len(value.PlanModifiers)
		if value.Default != nil {
			var response defaults.StringResponse
			value.Default.DefaultString(context.Background(), defaults.StringRequest{}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("default for %s: %v", name, response.Diagnostics)
			}
			defaultValue := response.PlanValue.ValueString()
			result.Default = &defaultValue
		}
	case schema.BoolAttribute:
		result.Type, result.Required, result.Optional, result.Computed = "bool", value.Required, value.Optional, value.Computed
		result.Validators, result.PlanModifiers = len(value.Validators), len(value.PlanModifiers)
	case schema.Int64Attribute:
		result.Type, result.Required, result.Optional, result.Computed = "int64", value.Required, value.Optional, value.Computed
		result.Validators, result.PlanModifiers = len(value.Validators), len(value.PlanModifiers)
	case schema.SetAttribute:
		result.Type = "set<" + terraformElementType(t, value.ElementType) + ">"
		result.Required, result.Optional, result.Computed = value.Required, value.Optional, value.Computed
		result.Validators, result.PlanModifiers = len(value.Validators), len(value.PlanModifiers)
	case schema.MapAttribute:
		result.Type = "map<" + terraformElementType(t, value.ElementType) + ">"
		result.Required, result.Optional, result.Computed = value.Required, value.Optional, value.Computed
		result.Validators, result.PlanModifiers = len(value.Validators), len(value.PlanModifiers)
	case schema.ListNestedAttribute:
		result.Type = "list<object>"
		result.Required, result.Optional, result.Computed = value.Required, value.Optional, value.Computed
		result.Validators, result.PlanModifiers = len(value.Validators), len(value.PlanModifiers)
		result.Attributes = characterizeAttributes(t, value.NestedObject.Attributes)
	default:
		t.Fatalf("unsupported schema attribute %s (%T)", name, attribute)
	}
	return result
}

func terraformElementType(t *testing.T, value any) string {
	t.Helper()
	switch value.(type) {
	case basetypes.StringType:
		return "string"
	case basetypes.Int64Type:
		return "int64"
	default:
		t.Fatalf("unsupported element type %T", value)
		return ""
	}
}

func mustLoadCases[T any](t *testing.T, root, category string) characterization.CaseDocument[T] {
	t.Helper()
	document, err := characterization.LoadCases[T](root, category)
	if err != nil {
		t.Fatalf("load %s cases: %v", category, err)
	}
	return document
}

func resourceCasesByKind(t *testing.T, fixtures []characterization.ResourceCase) map[string]client.Resource {
	t.Helper()
	result := make(map[string]client.Resource, len(fixtures))
	for _, fixture := range fixtures {
		var resource client.Resource
		if err := json.Unmarshal(fixture.Resource, &resource); err != nil {
			t.Fatalf("decode %s: %v", fixture.Kind, err)
		}
		result[fixture.Kind] = resource
	}
	return result
}

func outputCasesByKind(fixtures []characterization.OutputCase) map[string]characterization.OutputCase {
	result := make(map[string]characterization.OutputCase, len(fixtures))
	for _, fixture := range fixtures {
		result[fixture.Kind] = fixture
	}
	return result
}

func importCasesByKind(fixtures []characterization.ImportCase) map[string]characterization.ImportCase {
	result := make(map[string]characterization.ImportCase, len(fixtures))
	for _, fixture := range fixtures {
		result[fixture.Kind] = fixture
	}
	return result
}

func errorCasesByKind(fixtures []characterization.ErrorCase) map[string]characterization.ErrorCase {
	result := make(map[string]characterization.ErrorCase, len(fixtures))
	for _, fixture := range fixtures {
		result[fixture.Kind] = fixture
	}
	return result
}

func assertSameJSON(t *testing.T, got, want any) {
	t.Helper()
	gotDigest, err := characterization.DigestJSONValue(got)
	if err != nil {
		t.Fatalf("digest got JSON: %v", err)
	}
	wantDigest, err := characterization.DigestJSONValue(want)
	if err != nil {
		t.Fatalf("digest wanted JSON: %v", err)
	}
	if gotDigest != wantDigest {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("JSON drifted\nwant: %s\n got: %s", wantJSON, gotJSON)
	}
}

func assertNoDiagnostics(t *testing.T, diags frameworkdiag.Diagnostics) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %s", fmt.Sprint(diags))
	}
}
