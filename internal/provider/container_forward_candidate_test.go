package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestContainerCandidateDoesNotChangeNormalWorkerVersionSchemas(t *testing.T) {
	baselineProtocol, baselineDesired := normalWorkerVersionSchemas(t)
	source, err := os.ReadFile("../../cmd/provider-container-candidate/container-http-candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	candidateFactory := NewContainerCandidate(source)
	assertNormalWorkerVersionSchemasUnchanged(t, baselineProtocol, baselineDesired, "candidate construction")
	if got := len(candidateFactory().(*takoformProvider).Resources(context.Background())); got != 19 {
		t.Fatalf("candidate resource count = %d, want 19", got)
	}
	assertNormalWorkerVersionSchemasUnchanged(t, baselineProtocol, baselineDesired, "candidate Resources")
}

func assertNormalWorkerVersionSchemasUnchanged(t *testing.T, baselineProtocol, baselineDesired []byte, stage string) {
	t.Helper()
	protocol, desired := normalWorkerVersionSchemas(t)
	if !bytes.Equal(protocol, baselineProtocol) {
		t.Fatalf("normal Provider protocol schema changed after %s", stage)
	}
	if !bytes.Equal(desired, baselineDesired) {
		t.Fatalf("normal WorkerVersion desired schema changed after %s", stage)
	}
}

func normalWorkerVersionSchemas(t *testing.T) ([]byte, []byte) {
	t.Helper()
	normal := New("4.0.0")()
	server := providerserver.NewProtocol6(normal)()
	response, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil || len(response.Diagnostics) != 0 {
		t.Fatalf("normal GetProviderSchema: %v, diagnostics = %#v", err, response.Diagnostics)
	}
	if got := len(response.ResourceSchemas); got != 17 {
		t.Fatalf("normal Provider resource count = %d, want 17", got)
	}
	protocol := v3Provider3TofuSchemaDocument(t, response)
	for _, factory := range normal.Resources(context.Background()) {
		instance := factory()
		var metadata resource.MetadataResponse
		instance.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		if metadata.TypeName != "takoform_worker_version" {
			continue
		}
		worker := instance.(*v3FormResource)
		assembly := mustPublisherProviderSnapshotAssembly()
		desired, err := worker.form.DesiredSchema(v3SnapshotProjectionResolver{snapshot: assembly.snapshot})
		if err != nil {
			t.Fatal(err)
		}
		properties := desired["properties"].(map[string]any)
		for _, field := range []struct{ name, contract, version string }{
			{"worker", "worker.runtime", "1.1.0"},
			{"actorBindings", "worker.actor", "1.0.0"},
		} {
			property := properties[field.name].(map[string]any)
			if field.name == "actorBindings" {
				property = property["items"].(map[string]any)["properties"].(map[string]any)["resource"].(map[string]any)
			}
			ref := property["x-takoform-required-interface"].(map[string]any)
			if ref["name"] != field.contract || ref["version"] != field.version {
				t.Fatalf("normal WorkerVersion %s interface = %v, want %s@%s", field.name, ref, field.contract, field.version)
			}
		}
		encoded, err := json.Marshal(desired)
		if err != nil {
			t.Fatal(err)
		}
		return protocol, encoded
	}
	t.Fatal("normal Provider has no WorkerVersion")
	return nil, nil
}

func TestContainerForwardCandidateCompilesTypedResourcesWithoutChangingDefaultSet(t *testing.T) {
	source, err := os.ReadFile("../../cmd/provider-container-candidate/container-http-candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate := NewContainerCandidate(source)().(*takoformProvider)
	var configure frameworkprovider.ConfigureResponse
	candidate.Configure(context.Background(), frameworkprovider.ConfigureRequest{}, &configure)
	if !configure.Diagnostics.HasError() || len(configure.Diagnostics) != 1 ||
		configure.Diagnostics[0].Summary() != "Unpublished Provider candidate" {
		t.Fatalf("candidate Configure did not refuse planning and mutation: %#v", configure.Diagnostics)
	}
	var providerMetadata frameworkprovider.MetadataResponse
	candidate.Metadata(context.Background(), frameworkprovider.MetadataRequest{}, &providerMetadata)
	if providerMetadata.Version != "0.0.0-dev+35a77bdb37483fccbb365998923824040dad67cc" {
		t.Fatalf("candidate Provider version = %q", providerMetadata.Version)
	}
	resources := candidate.Resources(context.Background())
	if len(resources) != 19 {
		t.Fatalf("candidate resource count = %d, want 17 published resources plus ContainerService and VectorIndex", len(resources))
	}
	byType := map[string]resource.Resource{}
	for _, factory := range resources {
		instance := factory()
		var metadata resource.MetadataResponse
		instance.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		byType[metadata.TypeName] = instance
	}

	for _, typeName := range []string{"takoform_container_service", "takoform_vector_index", "takoform_worker_version"} {
		if byType[typeName] == nil {
			t.Fatalf("candidate provider does not expose %s", typeName)
		}
	}
	if byType["takoform_worker_deployment"] == nil {
		t.Fatal("candidate provider does not expose the candidate WorkerDeployment")
	}
	deploymentSchema := resourceSchema(t, byType["takoform_worker_deployment"])
	versions, ok := deploymentSchema.Schema.Attributes["versions"].(schema.ListNestedAttribute)
	if !ok || versions.NestedObject.Attributes["weight"] == nil || versions.NestedObject.Attributes["worker_version"] == nil {
		t.Fatal("candidate WorkerDeployment versions is not a typed weight/WorkerVersion reference list")
	}

	containerSchema := resourceSchema(t, byType["takoform_container_service"])
	for _, name := range []string{"image", "port", "health_path", "environment", "required_sensitive_vars", "workload_revision", "outbound_internet"} {
		if _, ok := containerSchema.Schema.Attributes[name]; !ok {
			t.Errorf("ContainerService has no typed HCL attribute %q", name)
		}
	}
	environment, ok := containerSchema.Schema.Attributes["environment"].(schema.ListNestedAttribute)
	if !ok || environment.NestedObject.Attributes["name"] == nil || environment.NestedObject.Attributes["value"] == nil {
		t.Fatal("ContainerService environment is not a typed nested HCL list")
	}

	workerSchema := resourceSchema(t, byType["takoform_worker_version"])
	for _, name := range []string{"actor_bindings", "vector_bindings", "container_http_bindings"} {
		if _, ok := workerSchema.Schema.Attributes[name]; !ok {
			t.Errorf("candidate WorkerVersion has no typed HCL binding attribute %q", name)
		}
	}
	bindings, ok := workerSchema.Schema.Attributes["container_http_bindings"].(schema.ListNestedAttribute)
	if !ok || bindings.NestedObject.Attributes["name"] == nil || bindings.NestedObject.Attributes["target_name"] == nil {
		t.Fatal("container_http_bindings is not a typed name/target resource binding list")
	}

	vectorSchema := resourceSchema(t, byType["takoform_vector_index"])
	for _, name := range []string{"dimension", "filter_keys", "metric"} {
		if !v3AttributeRequiresReplace(vectorSchema.Schema.Attributes[name]) {
			t.Errorf("candidate VectorIndex.%s does not force replacement", name)
		}
	}
	if got := containerCandidateVectorIndex().ImmutableFields(); !reflect.DeepEqual(got, []string{"/dimension", "/filterKeys", "/metric"}) {
		t.Fatalf("candidate VectorIndex immutable fields = %#v", got)
	}
	if got := containerCandidateVectorIndex().ProvidedInterfaces; !reflect.DeepEqual(got, []model.InterfaceRefSource{{Name: "edge.vector", Version: "0.1.0"}}) {
		t.Fatalf("candidate VectorIndex provided Interfaces = %#v", got)
	}
	if got := containerCandidateService().ProvidedInterfaces; !reflect.DeepEqual(got, []model.InterfaceRefSource{{Name: "container.http", Version: "0.1.0"}}) {
		t.Fatalf("candidate ContainerService provided Interfaces = %#v", got)
	}

	testContainerEnvironmentNamespace(t, byType["takoform_container_service"])
	testCandidateWorkerDeploymentTarget(t, source)

	baseline := New("4.0.0")().(*takoformProvider)
	if got := len(baseline.Resources(context.Background())); got != 17 {
		t.Fatalf("normal Provider resource count = %d, want the unchanged published set of 17", got)
	}
	for _, factory := range baseline.Resources(context.Background()) {
		instance := factory()
		var metadata resource.MetadataResponse
		instance.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		if metadata.TypeName == "takoform_container_service" || metadata.TypeName == "takoform_vector_index" {
			t.Errorf("default provider unexpectedly includes candidate resource %q", metadata.TypeName)
		}
	}
}

func TestContainerForwardCandidateRejectsProvidedInterfaceDigestDrift(t *testing.T) {
	source, err := os.ReadFile("../../cmd/provider-container-candidate/container-http-candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range [][]string{{"form"}, {"vector", "form"}} {
		var document map[string]any
		if err := json.Unmarshal(source, &document); err != nil {
			t.Fatal(err)
		}
		form := document
		for _, key := range route {
			form = form[key].(map[string]any)
		}
		definitionRaw := form["definitionJson"].(string)
		var definition map[string]any
		if err := json.Unmarshal([]byte(definitionRaw), &definition); err != nil {
			t.Fatal(err)
		}
		provided := definition["providedInterfaces"].([]any)[0].(map[string]any)
		provided["schemaDigest"] = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		encodedDefinition, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		form["definitionJson"] = string(encodedDefinition)
		encodedSource, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateContainerForwardCandidateSource(encodedSource); err == nil {
			t.Fatalf("accepted provided Interface digest drift at %v", route)
		}
	}
}

func TestContainerForwardCandidateRejectsWorkerVersionSchemaDrift(t *testing.T) {
	source, err := os.ReadFile("../../cmd/provider-container-candidate/container-http-candidate.json")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "container HTTP target Interface digest",
			mutate: func(definition map[string]any) {
				properties := definition["desiredSchema"].(map[string]any)["properties"].(map[string]any)
				resource := properties["containerHttpBindings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["resource"].(map[string]any)
				resource["x-takoform-required-interface"].(map[string]any)["schemaDigest"] = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
			},
		},
		{
			name: "container HTTP binding cardinality",
			mutate: func(definition map[string]any) {
				properties := definition["desiredSchema"].(map[string]any)["properties"].(map[string]any)
				properties["containerHttpBindings"].(map[string]any)["maxItems"] = float64(63)
			},
		},
		{
			name: "accepted Container binding digest",
			mutate: func(definition map[string]any) {
				bindings := definition["acceptedBindings"].([]any)
				bindings[8].(map[string]any)["schemaDigest"] = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
			},
		},
		{
			name: "accepted Container binding version",
			mutate: func(definition map[string]any) {
				bindings := definition["acceptedBindings"].([]any)
				bindings[8].(map[string]any)["version"] = "0.1.1"
			},
		},
		{
			name: "WorkerVersion API version",
			mutate: func(definition map[string]any) {
				definition["apiVersion"] = "edge.forms.changed.example"
			},
		},
		{
			name: "WorkerVersion definition kind",
			mutate: func(definition map[string]any) {
				definition["kind"] = "ContainerService"
			},
		},
		{
			name: "WorkerVersion role",
			mutate: func(definition map[string]any) {
				definition["role"] = "identity"
			},
		},
		{
			name: "WorkerVersion required Host API",
			mutate: func(definition map[string]any) {
				definition["requiresHostApi"] = "forms.takoform.com/v2"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(source, &document); err != nil {
				t.Fatal(err)
			}
			worker := document["workerVersion"].(map[string]any)
			definition := worker["definition"].(map[string]any)
			test.mutate(definition)
			encodedSource, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateContainerForwardCandidateSource(encodedSource); err == nil {
				t.Fatal("accepted WorkerVersion desired-schema drift with unchanged identity and binding roster")
			}
		})
	}
}

func testContainerEnvironmentNamespace(t *testing.T, instance resource.Resource) {
	t.Helper()
	validated, ok := instance.(resource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("candidate ContainerService has no configuration semantic validation")
	}
	ctx := context.Background()
	schemaResponse := resourceSchema(t, instance)
	environmentType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name": types.StringType, "value": types.StringType,
	}}
	environment := types.ListValueMust(environmentType, []attr.Value{
		types.ObjectValueMust(environmentType.AttrTypes, map[string]attr.Value{
			"name": types.StringValue("DATABASE_URL"), "value": types.StringValue("postgres://db.invalid"),
		}),
	})
	values := map[string]attr.Value{
		"name":                    types.StringValue("api"),
		"environment":             environment,
		"required_sensitive_vars": v3StringSetValue(t, "DATABASE_URL"),
	}
	var response resource.ValidateConfigResponse
	validated.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: v3ConfigWith(t, ctx, schemaResponse, values)}, &response)
	duplicateNameDiagnostic := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if diagnostic.Summary() == "Duplicate Container environment name" {
			duplicateNameDiagnostic = true
		}
	}
	if !duplicateNameDiagnostic {
		t.Fatalf("ContainerService did not reject the environment-name overlap with its semantic diagnostic: %v", response.Diagnostics)
	}

	values["required_sensitive_vars"] = v3StringSetValue(t, "DATABASE_PASSWORD")
	response = resource.ValidateConfigResponse{}
	validated.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: v3ConfigWith(t, ctx, schemaResponse, values)}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("ContainerService rejected disjoint environment and sensitive names: %v", response.Diagnostics)
	}
}

func testCandidateWorkerDeploymentTarget(t *testing.T, source []byte) {
	t.Helper()
	var candidate struct {
		WorkerDeployment struct {
			DefinitionJSON string `json:"definitionJson"`
		} `json:"workerDeployment"`
	}
	if err := json.Unmarshal(source, &candidate); err != nil {
		t.Fatal(err)
	}
	var definition struct {
		DesiredSchema struct {
			Properties struct {
				Versions struct {
					Items struct {
						Properties struct {
							WorkerVersion struct {
								TargetFormRefs []model.TargetFormRef `json:"x-takoform-target-formrefs"`
							} `json:"workerVersion"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"versions"`
			} `json:"properties"`
		} `json:"desiredSchema"`
	}
	if err := json.Unmarshal([]byte(candidate.WorkerDeployment.DefinitionJSON), &definition); err != nil {
		t.Fatal(err)
	}
	want := model.TargetFormRef{
		APIVersion: "edge.forms.takoform.com", Kind: "WorkerVersion",
		DefinitionVersion: containerForwardCandidateWorkerVersion,
		SchemaDigest:      containerForwardCandidateWorkerVersionSchemaDigest,
	}
	if !reflect.DeepEqual(definition.DesiredSchema.Properties.Versions.Items.Properties.WorkerVersion.TargetFormRefs, []model.TargetFormRef{want}) {
		t.Fatalf("embedded WorkerDeployment target refs = %#v, want exact candidate ref %#v",
			definition.DesiredSchema.Properties.Versions.Items.Properties.WorkerVersion.TargetFormRefs, want)
	}

	deployment := containerCandidateWorkerDeployment()
	var versionField *model.Field
	for _, field := range deployment.Fields {
		if field.Wire != "versions" {
			continue
		}
		for index := range field.Fields {
			if field.Fields[index].Wire == "workerVersion" {
				versionField = &field.Fields[index]
			}
		}
	}
	if versionField == nil || versionField.ResourceTarget == nil || versionField.ResourceTarget.Kind != "WorkerVersion" || !versionField.ResourceTarget.Contract.ExactForm {
		t.Fatalf("candidate WorkerDeployment does not type versions.workerVersion as an exact WorkerVersion ref: %#v", versionField)
	}
}

func resourceSchema(t *testing.T, instance resource.Resource) resource.SchemaResponse {
	t.Helper()
	var response resource.SchemaResponse
	instance.Schema(context.Background(), resource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("resource schema diagnostics: %v", response.Diagnostics)
	}
	return response
}
