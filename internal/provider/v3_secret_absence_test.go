package provider

// v3_secret_absence_test.go proves the provider never turns state into a secret
// store. Two independent properties:
//
//   - No resource or data-source attribute is declared Sensitive. The provider
//     carries no credential, no token, and no secret material in state, so
//     there is nothing for Terraform to mask. Sensitive provider-block token
//     and ephemeral runtime_inputs are configuration inputs and are never
//     persisted into resource state.
//   - A worker_bundle apply pins module bytes by digest and uploads them
//     through the content-addressed artifact API. The bytes themselves must
//     never appear anywhere in the resulting state object.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// attributeCarriesObjectShape reports whether an attribute type nests an
// object, which is how the guards below notice a nesting form they do not yet
// walk instead of silently skipping it.
func attributeCarriesObjectShape(attributeType attr.Type) bool {
	switch typed := attributeType.(type) {
	case types.ObjectType:
		return true
	case types.ListType:
		return attributeCarriesObjectShape(typed.ElemType)
	case types.SetType:
		return attributeCarriesObjectShape(typed.ElemType)
	case types.MapType:
		return attributeCarriesObjectShape(typed.ElemType)
	default:
		return false
	}
}

func assertNoSensitiveResourceAttributes(t *testing.T, label string, attributes map[string]rschema.Attribute) {
	t.Helper()
	for name, attribute := range attributes {
		if attribute.IsSensitive() {
			t.Errorf("%s.%s is Sensitive: resource state must never hold secret material", label, name)
		}
		switch typed := attribute.(type) {
		case rschema.ListNestedAttribute:
			assertNoSensitiveResourceAttributes(t, label+"."+name, typed.NestedObject.Attributes)
		case rschema.SetNestedAttribute:
			assertNoSensitiveResourceAttributes(t, label+"."+name, typed.NestedObject.Attributes)
		case rschema.MapNestedAttribute:
			assertNoSensitiveResourceAttributes(t, label+"."+name, typed.NestedObject.Attributes)
		case rschema.SingleNestedAttribute:
			assertNoSensitiveResourceAttributes(t, label+"."+name, typed.Attributes)
		case rschema.ObjectAttribute:
			// A flat object of primitive types: it has no member attributes that
			// could carry their own Sensitive flag.
		default:
			if attributeCarriesObjectShape(attribute.GetType()) {
				t.Errorf("%s.%s nests an object through unhandled attribute type %T; extend this guard",
					label, name, attribute)
			}
		}
	}
}

func assertNoSensitiveDataSourceAttributes(t *testing.T, label string, attributes map[string]dschema.Attribute) {
	t.Helper()
	for name, attribute := range attributes {
		if attribute.IsSensitive() {
			t.Errorf("%s.%s is Sensitive: declaration reads must never hold secret material", label, name)
		}
		switch typed := attribute.(type) {
		case dschema.ListNestedAttribute:
			assertNoSensitiveDataSourceAttributes(t, label+"."+name, typed.NestedObject.Attributes)
		case dschema.SetNestedAttribute:
			assertNoSensitiveDataSourceAttributes(t, label+"."+name, typed.NestedObject.Attributes)
		case dschema.MapNestedAttribute:
			assertNoSensitiveDataSourceAttributes(t, label+"."+name, typed.NestedObject.Attributes)
		case dschema.SingleNestedAttribute:
			assertNoSensitiveDataSourceAttributes(t, label+"."+name, typed.Attributes)
		case dschema.ObjectAttribute:
		default:
			if attributeCarriesObjectShape(attribute.GetType()) {
				t.Errorf("%s.%s nests an object through unhandled attribute type %T; extend this guard",
					label, name, attribute)
			}
		}
	}
}

func TestNoResourceOrDataSourceAttributeIsSensitive(t *testing.T) {
	ctx := context.Background()
	takoform := New("1.0.0")()

	for _, factory := range takoform.Resources(ctx) {
		candidate := factory()
		var metadata frameworkresource.MetadataResponse
		candidate.Metadata(ctx, frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		var schemaResponse frameworkresource.SchemaResponse
		candidate.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
		if schemaResponse.Diagnostics.HasError() {
			t.Fatalf("%s schema: %v", metadata.TypeName, schemaResponse.Diagnostics)
		}
		assertNoSensitiveResourceAttributes(t, metadata.TypeName, schemaResponse.Schema.Attributes)
	}

	for _, factory := range takoform.DataSources(ctx) {
		candidate := factory()
		var metadata frameworkdatasource.MetadataResponse
		candidate.Metadata(ctx, frameworkdatasource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		var schemaResponse frameworkdatasource.SchemaResponse
		candidate.Schema(ctx, frameworkdatasource.SchemaRequest{}, &schemaResponse)
		if schemaResponse.Diagnostics.HasError() {
			t.Fatalf("%s schema: %v", metadata.TypeName, schemaResponse.Diagnostics)
		}
		assertNoSensitiveDataSourceAttributes(t, metadata.TypeName, schemaResponse.Schema.Attributes)
	}

	// The provider block's bearer token is the one Sensitive value in the whole
	// provider. It is configuration input consumed by Configure; it is never
	// projected into any resource state.
	var providerSchema frameworkprovider.SchemaResponse
	takoform.Schema(ctx, frameworkprovider.SchemaRequest{}, &providerSchema)
	var sensitive []string
	for name, attribute := range providerSchema.Schema.Attributes {
		if attribute.IsSensitive() {
			sensitive = append(sensitive, name)
		}
	}
	sort.Strings(sensitive)
	if len(sensitive) != 2 || sensitive[0] != "runtime_inputs" || sensitive[1] != "token" {
		t.Fatalf("provider configuration Sensitive attributes = %v, want exactly [runtime_inputs token]", sensitive)
	}
}

// TestWorkerBundleApplyKeepsModuleBytesOutOfState authors a module whose bytes
// carry a unique marker, applies the bundle, and walks every value in the
// resulting state object for that marker. Only the path, size, and digest may
// survive; the bytes travel through the content-addressed artifact upload.
func TestWorkerBundleApplyKeepsModuleBytesOutOfState(t *testing.T) {
	const marker = "TAKOFORM-MODULE-BYTES-MUST-NOT-REACH-STATE-9f3a71"

	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	dir := t.TempDir()
	workerBytes := []byte("export default { fetch() { return new Response(\"" + marker + "\") } }\n")
	workerFile := filepath.Join(dir, "worker.mjs")
	if err := os.WriteFile(workerFile, workerBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(workerBytes)
	wantDigest := "sha256:" + hex.EncodeToString(sum[:])

	moduleType := v3WorkerBundleModuleType()
	modules := types.ListValueMust(moduleType, []attr.Value{
		types.ObjectValueMust(moduleType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue("worker.mjs"),
			"content_type": types.StringValue("application/javascript+module"),
			"content_file": types.StringValue(workerFile),
			"size":         types.Int64Unknown(),
			"digest":       types.StringUnknown(),
		}),
	})
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":        types.StringValue("worker-bundle"),
		"main_module": types.StringValue("worker.mjs"),
		"modules":     modules,
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	// The marker really is the module's content: the host received those exact
	// bytes as a content-addressed blob.
	if !strings.Contains(string(host.blobs[wantDigest]), marker) {
		t.Fatalf("the marker bytes never reached the artifact upload; the walk below would prove nothing")
	}

	var offenders []string
	walkErr := tftypes.Walk(createResponse.State.Raw, func(attributePath *tftypes.AttributePath, value tftypes.Value) (bool, error) {
		if !value.IsKnown() || value.IsNull() || !value.Type().Is(tftypes.String) {
			return true, nil
		}
		var text string
		if err := value.As(&text); err != nil {
			return true, nil
		}
		if strings.Contains(text, marker) {
			offenders = append(offenders, attributePath.String())
		}
		return true, nil
	})
	if walkErr != nil {
		t.Fatalf("walking state: %v", walkErr)
	}
	if len(offenders) != 0 {
		t.Fatalf("module file bytes reached state at %v", offenders)
	}
	// Belt and braces: nothing anywhere in the serialized state object either.
	if strings.Contains(createResponse.State.Raw.String(), marker) {
		t.Fatal("module file bytes reached the serialized state object")
	}

	// What state DOES record is the pin: path, size, and digest.
	var stateModules types.List
	if diags := createResponse.State.GetAttribute(ctx, path.Root("modules"), &stateModules); diags.HasError() {
		t.Fatalf("state modules: %v", diags)
	}
	element := stateModules.Elements()[0].(types.Object).Attributes()
	if got := element["digest"].(types.String).ValueString(); got != wantDigest {
		t.Fatalf("state module digest = %q, want %q", got, wantDigest)
	}
	if got := element["size"].(types.Int64).ValueInt64(); got != int64(len(workerBytes)) {
		t.Fatalf("state module size = %d, want %d", got, len(workerBytes))
	}
}
