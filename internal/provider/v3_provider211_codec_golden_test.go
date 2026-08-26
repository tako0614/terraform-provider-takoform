package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// TestV3Provider211RetainedCodecsReadExactHostResponses drives every one of
// the fourteen readable retained identities through the real Provider 3 read
// path. The fake Host echoes the exact FormRef in the response and records the
// exact query, so this cannot pass by reading the current default identity.
func TestV3Provider211RetainedCodecsReadExactHostResponses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	want := readV3Provider211RetainedGolden(t)
	codecs := v3Codecs()
	registry := mustProviderV3SnapshotAssembly().registry

	for _, expected := range want.Identities {
		expected := expected
		if !expected.Provider3Readable {
			continue
		}
		t.Run(expected.FormRef.Kind, func(t *testing.T) {
			ref := expected.FormRef
			codec, ok := codecs.forStateKey(ref.ExactKey())
			if !ok {
				t.Fatalf("retained %s has no exact codec", ref.ExactKey())
			}
			if codec.Ref != ref {
				t.Fatalf("codec selection for %s = %#v, want %#v", ref.ExactKey(), codec.Ref, ref)
			}
			if registryRef, ok := registry.Lookup(ref.ExactKey()); !ok || registryRef != ref {
				t.Fatalf("retained registry lookup for %s = %#v/%t", ref.ExactKey(), registryRef, ok)
			}

			form := v3ProviderCurrentFormByKind(t, ref.Kind)
			host := newV3FakeHost(t)
			resource := v3Provider3CurrentResourceHarness(
				t, form, expected.Provider3Mapping, newV3TestProviderData(t, host), codecs,
			)
			schemaResponse := v3SchemaOf(t, resource)
			spec := v3Provider211RetainedHostSpec(t, codec)
			const space = "retained-space"
			const name = "retained-resource"
			const uid = "uid-retained"
			outputs := v3Provider211RetainedOutputs(t, form.Outputs)
			host.resources[host.resourceKey(ref.Kind, name)] = &v3HostRecord{
				uid: uid, generation: 1, revision: 1,
				apiVersion: ref.APIVersion, kind: ref.Kind, form: v3Provider211RetainedWireForm(ref),
				space: space, spec: spec, outputs: outputs,
			}

			state := v3Provider211RetainedReadState(t, ctx, schemaResponse, resource, codec, ref, spec, name, space, uid)
			response := frameworkresource.ReadResponse{State: state}
			resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("read retained %s through exact codec: %v", ref.Kind, response.Diagnostics)
			}
			if len(host.resourceQueries) != 1 || host.resourceQueries[0] != "GET "+ref.APIVersion+" "+ref.SchemaDigest {
				t.Fatalf("retained %s read query = %v, want exact %s", ref.Kind, host.resourceQueries, ref.SchemaDigest)
			}
			for attribute, wantValue := range map[string]string{
				"name":                    name,
				"space":                   space,
				"uid":                     uid,
				"generation":              "1",
				"revision":                "1",
				"form_api_version":        ref.APIVersion,
				"form_kind":               ref.Kind,
				"form_definition_version": ref.DefinitionVersion,
				"form_schema_digest":      ref.SchemaDigest,
				"form_package_digest":     ref.PackageDigest,
			} {
				if got := v3StateString(t, ctx, response.State, attribute).ValueString(); got != wantValue {
					t.Fatalf("retained %s readback %s = %q, want %q", ref.Kind, attribute, got, wantValue)
				}
			}
		})
	}
}

func v3Provider211RetainedOutputs(t *testing.T, fields []model.Field) map[string]any {
	t.Helper()
	if len(fields) == 0 {
		return nil
	}
	outputs := make(map[string]any, len(fields))
	for _, field := range fields {
		switch field.Wire {
		case "hostname":
			outputs[field.Wire] = "retained.example.invalid"
		case "url":
			outputs[field.Wire] = "https://retained.example.invalid/"
		default:
			t.Fatalf("retained %s output %s has no immutable readback vector", field.HCL, field.Wire)
		}
	}
	return outputs
}

func v3Provider211RetainedHostSpec(t *testing.T, codec v3FormCodec) map[string]any {
	t.Helper()
	if codec.DesiredSchema == nil {
		t.Fatalf("codec %s has no desired schema", codec.Ref.ExactKey())
	}
	return model.MaterializeDefaults(codec.DesiredSchema, codec.Form.CanonicalDesired())
}

func v3Provider211RetainedWireForm(ref v3FormRef) map[string]any {
	return map[string]any{"formRef": map[string]any{
		"apiVersion":        ref.APIVersion,
		"kind":              ref.Kind,
		"definitionVersion": ref.DefinitionVersion,
		"schemaDigest":      ref.SchemaDigest,
	}}
}

func v3Provider211RetainedReadState(
	t *testing.T,
	ctx context.Context,
	schemaResponse frameworkresource.SchemaResponse,
	resource *v3FormResource,
	codec v3FormCodec,
	ref v3FormRef,
	spec map[string]any,
	name, space, uid string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}
	for attribute, value := range map[string]string{
		"name": name, "space": space, "uid": uid, "generation": "1", "revision": "1",
	} {
		if diags := state.SetAttribute(ctx, path.Root(attribute), types.StringValue(value)); diags.HasError() {
			t.Fatalf("seed retained state %s: %v", attribute, diags)
		}
	}
	state = v3SeedStateRef(t, ctx, state, ref)

	// Seed authoring fields with the Host's exact retained desired document. A
	// read of an immutable revision preserves these values by contract, while a
	// mutable attachment may adopt them from the same Host response. Decode
	// through the RETAINED field declarations: current declarations intentionally
	// use a different versionless family group, so validating a retained
	// v1beta1 reference through them would reject a truthful old Host response.
	for _, field := range codec.Form.Fields {
		if field.Wire == "bucketBindings" {
			continue
		}
		value, fieldDiags := v3Provider211RetainedFieldValue(ctx, ref.APIVersion, field, spec[field.Wire])
		if fieldDiags.HasError() {
			t.Fatalf("seed retained %s.%s: %v", ref.Kind, field.Wire, fieldDiags)
		}
		if diags := state.SetAttribute(ctx, path.Root(v3AttributeName(field)), value); diags.HasError() {
			t.Fatalf("seed retained state %s.%s: %v", ref.Kind, field.Wire, diags)
		}
	}
	// WorkerVersion's current schema retains the historical bucket binding
	// attribute solely in state. It is absent from the current desired Form but
	// remains part of the retained codec's exact field set.
	if resource.form.Kind == workerVersionKind {
		var bucketField model.Field
		for _, field := range codec.Form.Fields {
			if field.Wire == "bucketBindings" {
				bucketField = field
				break
			}
		}
		if bucketField.Wire == "" {
			t.Fatal("retained WorkerVersion codec dropped bucketBindings")
		}
		value, fieldDiags := v3Provider211RetainedFieldValue(ctx, ref.APIVersion, bucketField, spec[bucketField.Wire])
		if fieldDiags.HasError() {
			t.Fatalf("seed retained WorkerVersion.bucketBindings: %v", fieldDiags)
		}
		if diags := state.SetAttribute(ctx, path.Root(v3RetainedBucketBindingsAttribute), value); diags.HasError() {
			t.Fatalf("seed retained WorkerVersion bucket bindings state: %v", diags)
		}
	}
	// Artifact-backed revisions carry only the manifest digest on the wire; the
	// local file authoring attributes are intentionally null after import/read.
	if resource.form.Kind == workerBundleKind || resource.form.Kind == staticAssetBundleKind {
		digest, ok := spec["manifestDigest"].(string)
		if !ok {
			t.Fatalf("retained %s spec has no manifestDigest", resource.form.Kind)
		}
		if diags := state.SetAttribute(ctx, path.Root("manifest_digest"), types.StringValue(digest)); diags.HasError() {
			t.Fatalf("seed retained %s manifest digest: %v", resource.form.Kind, diags)
		}
	}
	return state
}

func v3Provider211RetainedFieldValue(ctx context.Context, sourceGroup string, field model.Field, raw any) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics
	value := v3FieldValueFromSpec(ctx, sourceGroup, field, raw, &diags)
	return value, diags
}
