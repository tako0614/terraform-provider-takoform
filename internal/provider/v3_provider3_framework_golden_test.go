package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	frameworkschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const v3Provider3FrameworkGoldenPath = "testdata/v3-provider3-framework-golden.json"

type v3Provider3FrameworkGolden struct {
	Format        string                                        `json:"format"`
	SourceTag     string                                        `json:"sourceTag"`
	SourceCommit  string                                        `json:"sourceCommit"`
	ResourceCount int                                           `json:"resourceCount"`
	Resources     map[string]v3Provider3FrameworkGoldenResource `json:"resources"`
}

type v3Provider3FrameworkGoldenResource struct {
	FormRef        v3FormRef `json:"formRef"`
	BehaviorDigest string    `json:"behaviorDigest"`
}

// TestV3Provider3FrameworkBehaviorGoldenLocksAllResources covers the behavior
// intentionally absent from Terraform's protocol schema: Framework defaults,
// validators and plan modifiers, plus the Form properties selecting the
// generic resource's revision/artifact/update branches.
func TestV3Provider3FrameworkBehaviorGoldenLocksAllResources(t *testing.T) {
	want := readV3Provider3FrameworkGolden(t)
	got := deriveV3Provider3FrameworkGolden(t)
	if !reflect.DeepEqual(got, want) {
		wantRaw, _ := json.MarshalIndent(want, "", "  ")
		gotRaw, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("Provider 3 Framework behavior golden drifted:\nwant:\n%s\ngot:\n%s", wantRaw, gotRaw)
	}
}

func deriveV3Provider3FrameworkGolden(t *testing.T) v3Provider3FrameworkGolden {
	t.Helper()
	registry := mustProviderV3SnapshotAssembly().registry
	typesByRef := v3TerraformResourceTypes()
	resources := make(map[string]v3Provider3FrameworkGoldenResource, 31)
	for _, form := range mustProviderV3SnapshotAssembly().currentForms {
		line := v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		ref, err := registry.DefaultCreate(line)
		if err != nil {
			t.Fatal(err)
		}
		resourceType, ok := typesByRef.Lookup(ref.ExactKey())
		if !ok {
			t.Fatalf("resource type for %s is absent", ref.ExactKey())
		}
		candidate := v3Provider3HistoricalResourceHarness(t, form, resourceType, nil, v3Codecs())
		var response frameworkresource.SchemaResponse
		candidate.Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("schema for %s: %v", resourceType, response.Diagnostics)
		}
		_, artifactBacked := v3ProviderArtifactForForm(t, form)
		vector := map[string]any{
			"attributes":          v3Provider3FrameworkAttributes(t, response.Schema.Attributes),
			"role":                form.Role,
			"declaresUpdate":      form.DeclaresUpdate(),
			"derivesRevisionName": candidate.derivesRevisionName(),
			"artifactBacked":      artifactBacked,
			"desiredFieldCount":   len(form.Fields),
			"declaredOutputCount": len(form.Outputs),
			"schemaDescription":   response.Schema.Description,
			"schemaMarkdown":      response.Schema.MarkdownDescription,
			"schemaDeprecation":   response.Schema.DeprecationMessage,
			"schemaBlockCount":    len(response.Schema.Blocks),
			"hasModifyPlan":       implementsV3ModifyPlan(candidate),
			"hasConfigValidation": implementsV3ConfigValidation(candidate),
			"hasStateUpgrade":     implementsV3StateUpgrade(candidate),
			"hasImportState":      implementsV3ImportState(candidate),
		}
		resources[resourceType] = v3Provider3FrameworkGoldenResource{
			FormRef: ref, BehaviorDigest: v3CanonicalDigest(t, vector),
		}
	}
	if len(resources) != 31 {
		t.Fatalf("Framework behavior resource count = %d, want 31", len(resources))
	}
	return v3Provider3FrameworkGolden{
		Format:        "takoform.provider3-framework-golden@v1",
		SourceTag:     "v3.0.0",
		SourceCommit:  "a225cfa7c84aa551981cc8ad56c9a281fa6e051a",
		ResourceCount: 31,
		Resources:     resources,
	}
}

func v3Provider3FrameworkAttributes(t *testing.T, attributes map[string]frameworkschema.Attribute) map[string]any {
	t.Helper()
	result := make(map[string]any, len(attributes))
	for name, attribute := range attributes {
		entry := map[string]any{"implementation": reflect.TypeOf(attribute).String()}
		if value, ok := v3AttributeDefaultValue(t, attribute); ok {
			terraformValue, err := value.ToTerraformValue(context.Background())
			if err != nil {
				t.Fatalf("default for %s: %v", name, err)
			}
			entry["default"] = terraformValue.String()
		}
		validators, modifiers, nested := v3Provider3FrameworkMembers(t, attribute)
		if len(validators) != 0 {
			entry["validators"] = validators
		}
		if len(modifiers) != 0 {
			entry["planModifiers"] = modifiers
		}
		if len(nested) != 0 {
			entry["nested"] = nested
		}
		result[name] = entry
	}
	return result
}

func v3Provider3FrameworkMembers(t *testing.T, attribute frameworkschema.Attribute) ([]map[string]string, []map[string]string, map[string]any) {
	t.Helper()
	var validators, modifiers []any
	var nested map[string]frameworkschema.Attribute
	switch typed := attribute.(type) {
	case frameworkschema.BoolAttribute:
		validators, modifiers = anySlice(typed.Validators), anySlice(typed.PlanModifiers)
	case frameworkschema.Int64Attribute:
		validators, modifiers = anySlice(typed.Validators), anySlice(typed.PlanModifiers)
	case frameworkschema.StringAttribute:
		validators, modifiers = anySlice(typed.Validators), anySlice(typed.PlanModifiers)
	case frameworkschema.ListAttribute:
		validators, modifiers = anySlice(typed.Validators), anySlice(typed.PlanModifiers)
	case frameworkschema.SetAttribute:
		validators, modifiers = anySlice(typed.Validators), anySlice(typed.PlanModifiers)
	case frameworkschema.MapAttribute:
		validators, modifiers = anySlice(typed.Validators), anySlice(typed.PlanModifiers)
	case frameworkschema.SingleNestedAttribute:
		validators, modifiers, nested = anySlice(typed.Validators), anySlice(typed.PlanModifiers), typed.Attributes
	case frameworkschema.ListNestedAttribute:
		validators, modifiers, nested = anySlice(typed.Validators), anySlice(typed.PlanModifiers), typed.NestedObject.Attributes
	case frameworkschema.SetNestedAttribute:
		validators, modifiers, nested = anySlice(typed.Validators), anySlice(typed.PlanModifiers), typed.NestedObject.Attributes
	case frameworkschema.MapNestedAttribute:
		validators, modifiers, nested = anySlice(typed.Validators), anySlice(typed.PlanModifiers), typed.NestedObject.Attributes
	default:
		t.Fatalf("unsupported Framework attribute implementation %T", attribute)
	}
	return v3Provider3Describers(t, validators), v3Provider3Describers(t, modifiers), v3Provider3FrameworkAttributes(t, nested)
}

type v3Provider3Describer interface {
	Description(context.Context) string
	MarkdownDescription(context.Context) string
}

func v3Provider3Describers(t *testing.T, values []any) []map[string]string {
	t.Helper()
	result := make([]map[string]string, 0, len(values))
	for _, value := range values {
		describer, ok := value.(v3Provider3Describer)
		if !ok {
			t.Fatalf("Framework behavior %T has no stable description interface", value)
		}
		result = append(result, map[string]string{
			"implementation": reflect.TypeOf(value).String(),
			"description":    describer.Description(context.Background()),
			"markdown":       describer.MarkdownDescription(context.Background()),
		})
	}
	return result
}

func anySlice[T any](values []T) []any {
	result := make([]any, len(values))
	for index := range values {
		result[index] = values[index]
	}
	return result
}

func implementsV3ModifyPlan(resource any) bool {
	_, ok := resource.(frameworkresource.ResourceWithModifyPlan)
	return ok
}

func implementsV3ConfigValidation(resource any) bool {
	_, ok := resource.(frameworkresource.ResourceWithConfigValidators)
	return ok
}

func implementsV3StateUpgrade(resource any) bool {
	_, ok := resource.(frameworkresource.ResourceWithUpgradeState)
	return ok
}

func implementsV3ImportState(resource any) bool {
	_, ok := resource.(frameworkresource.ResourceWithImportState)
	return ok
}

func readV3Provider3FrameworkGolden(t *testing.T) v3Provider3FrameworkGolden {
	t.Helper()
	raw, err := os.ReadFile(v3Provider3FrameworkGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, canonical) {
		t.Fatalf("%s is not canonical JSON", v3Provider3FrameworkGoldenPath)
	}
	var golden v3Provider3FrameworkGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if golden.Format != "takoform.provider3-framework-golden@v1" || golden.SourceTag != "v3.0.0" ||
		golden.SourceCommit != "a225cfa7c84aa551981cc8ad56c9a281fa6e051a" ||
		golden.ResourceCount != 31 || len(golden.Resources) != 31 {
		t.Fatalf("invalid immutable Framework golden header: %#v", golden)
	}
	return golden
}
