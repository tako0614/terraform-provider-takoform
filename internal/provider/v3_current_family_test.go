package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

func TestV3CurrentProjectionIsExhaustiveAndUnique(t *testing.T) {
	t.Parallel()
	forms := providerV3CurrentForms()
	if len(forms) != 31 {
		t.Fatalf("current provider Form projection = %d, want 31", len(forms))
	}
	codecs := v3Codecs()
	seenLines := map[currentformregistry.GroupKind]bool{}
	seenRefs := map[currentformregistry.ExactFormKey]bool{}
	seenTypes := map[string]currentformregistry.GroupKind{}
	for _, form := range forms {
		if strings.Contains(form.Family.APIVersion(), "/") {
			t.Fatalf("current Form %s/%s carries a versioned family group", form.Family.APIVersion(), form.Kind)
		}
		if form.Kind == "ObjectBucket" {
			t.Fatal("withdrawn ObjectBucket leaked into current provider projection")
		}
		line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		if seenLines[line] {
			t.Fatalf("duplicate current Group+Kind projection %s/%s", line.APIVersion, line.Kind)
		}
		seenLines[line] = true
		ref, err := currentformregistry.V3Current().DefaultCreate(line)
		if err != nil {
			t.Fatalf("current registry has no exact create target for %s/%s: %v", line.APIVersion, line.Kind, err)
		}
		if ref.APIVersion != line.APIVersion || ref.Kind != line.Kind {
			t.Fatalf("registry target %s does not preserve line %s/%s", ref.ExactKey(), line.APIVersion, line.Kind)
		}
		if seenRefs[ref.ExactKey()] {
			t.Fatalf("duplicate exact current ref %s", ref.ExactKey())
		}
		seenRefs[ref.ExactKey()] = true
		if codec, ok := codecs.forStateKey(ref.ExactKey()); !ok || codec.Form.Kind != form.Kind || codec.Form.Family.APIVersion() != line.APIVersion {
			t.Fatalf("current exact ref %s has no matching codec: codec=%#v ok=%t", ref.ExactKey(), codec, ok)
		}
		resourceType, ok := v3TerraformResourceTypes().Lookup(ref.ExactKey())
		if !ok {
			t.Fatalf("current exact ref %s has no provider Terraform mapping", ref.ExactKey())
		}
		if previous, duplicate := seenTypes[resourceType]; duplicate && previous != line {
			t.Fatalf("Terraform type %q ambiguously maps %s/%s and %s/%s", resourceType, previous.APIVersion, previous.Kind, line.APIVersion, line.Kind)
		}
		seenTypes[resourceType] = line
	}
	currentRefs := 0
	for _, ref := range currentformregistry.V3Current().SupportedRefs() {
		if !strings.Contains(ref.APIVersion, "/") {
			currentRefs++
			if !seenRefs[ref.ExactKey()] {
				t.Fatalf("generated current registry ref %s is absent from provider projection", ref.ExactKey())
			}
		}
	}
	if currentRefs != len(forms) {
		t.Fatalf("generated current registry has %d exact refs, provider projection has %d Forms", currentRefs, len(forms))
	}
}

func TestV3ExactMappingRejectsWrongGroupWithoutKindFallback(t *testing.T) {
	t.Parallel()
	base := currentformregistry.V3Current()
	a := currentformregistry.V3Ref{
		APIVersion: "alpha.forms.example", Kind: "SharedThing", DefinitionVersion: "0.1.0",
		SchemaDigest: "sha256:" + strings.Repeat("a", 64), PackageDigest: "sha256:" + strings.Repeat("b", 64),
	}
	b := currentformregistry.V3Ref{
		APIVersion: "beta.forms.example", Kind: "SharedThing", DefinitionVersion: "0.1.0",
		SchemaDigest: "sha256:" + strings.Repeat("c", 64), PackageDigest: "sha256:" + strings.Repeat("d", 64),
	}
	withA, err := base.Register(a, true)
	if err != nil {
		t.Fatal(err)
	}
	withBoth, err := withA.Register(b, true)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := newV3ResourceTypeRegistry(withBoth, []v3ResourceTypeLine{
		{GroupKind: a.ExactKey().GroupKind(), ResourceType: "takoform_alpha_shared_thing"},
		{GroupKind: b.ExactKey().GroupKind(), ResourceType: "takoform_beta_shared_thing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := mapped.Lookup(a.ExactKey()); !ok || got != "takoform_alpha_shared_thing" {
		t.Fatalf("alpha exact mapping = %q, ok=%t", got, ok)
	}
	if got, ok := mapped.Lookup(b.ExactKey()); !ok || got != "takoform_beta_shared_thing" {
		t.Fatalf("beta exact mapping = %q, ok=%t", got, ok)
	}
	wrongGroup := a.ExactKey()
	wrongGroup.APIVersion = "gamma.forms.example"
	if _, ok := mapped.Lookup(wrongGroup); ok {
		t.Fatal("unknown group resolved through same-kind mapping")
	}
	wrongDigest := a.ExactKey()
	wrongDigest.SchemaDigest = "sha256:" + strings.Repeat("e", 64)
	if _, ok := mapped.Lookup(wrongDigest); ok {
		t.Fatal("unknown exact digest resolved through same-kind mapping")
	}
}

func TestV3RepresentativeLifecycleCoversEveryNewFamily(t *testing.T) {
	// Identity Forms are deliberately selected where possible, while the
	// remaining families use their first declared Form with its canonical
	// example values. This exercises registration, exact codec selection,
	// schema compilation, typed wire encoding, fake-host apply, and readback for
	// every newly admitted family without inventing provider-specific fixtures.
	ctx := context.Background()
	for _, family := range providerV3CurrentFamilies() {
		family := family
		t.Run(family.family.APIVersion(), func(t *testing.T) {
			t.Parallel()
			form := family.forms[0]
			host := newV3FakeHost(t)
			resource := v3Provider3CurrentResourceHarness(
				t, form, "", newV3TestProviderData(t, host), v3Codecs(),
			)
			schemaResponse := v3SchemaOf(t, resource)
			values := map[string]attr.Value{
				"name": types.StringValue(form.Slug), "space": types.StringValue("prod"),
			}
			for _, field := range form.Fields {
				raw := field.Example
				if raw == nil {
					raw = field.Default
				}
				if raw == nil {
					t.Fatalf("%s/%s field %s has neither example nor default", form.Family.APIVersion(), form.Kind, field.HCL)
				}
				var fieldDiags diag.Diagnostics
				value := v3FieldValueFromSpec(ctx, form.Family.APIVersion(), field, raw, &fieldDiags)
				if fieldDiags.HasError() {
					t.Fatalf("%s/%s example %s: %v", form.Family.APIVersion(), form.Kind, field.HCL, fieldDiags)
				}
				values[field.AttributeName()] = value
			}
			plan := v3PlanWith(t, ctx, schemaResponse, values)
			createResponse := frameworkresource.CreateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
			if createResponse.Diagnostics.HasError() {
				t.Fatalf("create: %v", createResponse.Diagnostics)
			}
			if len(host.applySpecs) != 1 {
				t.Fatalf("apply count = %d, want 1", len(host.applySpecs))
			}
			readResponse := frameworkresource.ReadResponse{State: createResponse.State}
			resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
			if readResponse.Diagnostics.HasError() {
				t.Fatalf("read: %v", readResponse.Diagnostics)
			}
			if got := v3StateString(t, ctx, readResponse.State, "form_api_version").ValueString(); got != form.Family.APIVersion() {
				t.Fatalf("read Form group = %q, want %q", got, form.Family.APIVersion())
			}
		})
	}
}
