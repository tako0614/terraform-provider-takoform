package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const v3Provider3CodecGoldenPath = "testdata/v3-provider3-codec-golden.json"

// v3Provider3CodecGolden locks the parts of Provider 3 that a schema-only
// golden cannot see: HCL-to-wire projection, the exact apply envelope, and
// the complete Terraform value written after a Host readback. The fixture was
// captured from the immutable v3.0.0 source and deliberately has no ordinary
// refresh path.
type v3Provider3CodecGolden struct {
	Format        string                                    `json:"format"`
	SourceTag     string                                    `json:"sourceTag"`
	SourceCommit  string                                    `json:"sourceCommit"`
	ResourceCount int                                       `json:"resourceCount"`
	Resources     map[string]v3Provider3CodecGoldenResource `json:"resources"`
}

type v3Provider3CodecGoldenResource struct {
	FormRef       v3FormRef `json:"formRef"`
	FieldCodec    string    `json:"fieldCodecDigest"`
	ApplyRequest  string    `json:"applyRequestDigest"`
	ReadbackState string    `json:"readbackStateDigest"`
}

func TestV3Provider3CodecGoldenLocksAllResources(t *testing.T) {
	want := readV3Provider3CodecGolden(t)
	got := deriveV3Provider3CodecGolden(t)
	if !reflect.DeepEqual(got, want) {
		wantRaw, _ := json.MarshalIndent(want, "", "  ")
		gotRaw, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("Provider 3 codec golden drifted:\nwant:\n%s\ngot:\n%s", wantRaw, gotRaw)
	}
}

func deriveV3Provider3CodecGolden(t *testing.T) v3Provider3CodecGolden {
	t.Helper()
	const wantResourceCount = 31
	ctx := context.Background()
	registry := mustProviderV3SnapshotAssembly().registry
	typesByRef := v3TerraformResourceTypes()
	codecs := v3Codecs()
	resources := make(map[string]v3Provider3CodecGoldenResource, wantResourceCount)

	for _, form := range mustProviderV3SnapshotAssembly().currentForms {
		form := form
		_, artifactBacked := v3ProviderArtifactForForm(t, form)
		line := v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		ref, err := registry.DefaultCreate(line)
		if err != nil {
			t.Fatalf("default ref for %s/%s: %v", line.APIVersion, line.Kind, err)
		}
		resourceType, ok := typesByRef.Lookup(ref.ExactKey())
		if !ok {
			t.Fatalf("resource type for %s: missing", ref.ExactKey())
		}
		codec, ok := codecs.forStateKey(ref.ExactKey())
		if !ok {
			t.Fatalf("codec for %s: missing", ref.ExactKey())
		}
		if codec.Ref != ref {
			t.Fatalf("codec for %s carries %#v, want %#v", ref.ExactKey(), codec.Ref, ref)
		}

		values := map[string]attr.Value{
			"name":  types.StringValue(form.Slug),
			"space": types.StringValue("provider3-golden"),
		}
		fieldVectors := make([]map[string]any, 0, len(form.Fields))
		for _, field := range form.Fields {
			raw := field.Example
			if raw == nil {
				raw = field.Default
			}
			if raw == nil {
				t.Fatalf("%s/%s field %s has no immutable codec vector", line.APIVersion, line.Kind, field.Wire)
			}
			var readDiags diag.Diagnostics
			value := v3FieldValueFromSpec(ctx, line.APIVersion, field, decodedWire(t, raw), &readDiags)
			if readDiags.HasError() {
				t.Fatalf("decode %s/%s.%s: %v", line.APIVersion, line.Kind, field.Wire, readDiags)
			}
			wire, wireDiags := v3FieldToWire(ctx, line.APIVersion, field, v3AttributeName(field), value)
			if wireDiags.HasError() {
				t.Fatalf("encode %s/%s.%s: %v", line.APIVersion, line.Kind, field.Wire, wireDiags)
			}
			var roundDiags diag.Diagnostics
			roundValue := v3FieldValueFromSpec(ctx, line.APIVersion, field, decodedWire(t, wire), &roundDiags)
			if roundDiags.HasError() {
				t.Fatalf("round-trip decode %s/%s.%s: %v", line.APIVersion, line.Kind, field.Wire, roundDiags)
			}
			roundWire, roundWireDiags := v3FieldToWire(ctx, line.APIVersion, field, v3AttributeName(field), roundValue)
			if roundWireDiags.HasError() {
				t.Fatalf("round-trip encode %s/%s.%s: %v", line.APIVersion, line.Kind, field.Wire, roundWireDiags)
			}
			if !v3CanonicalValueEqual(t, wire, roundWire) {
				t.Fatalf("%s/%s.%s changed across HCL/wire round trip: %#v != %#v", line.APIVersion, line.Kind, field.Wire, wire, roundWire)
			}
			fieldVectors = append(fieldVectors, map[string]any{
				"hcl": v3AttributeName(field), "wire": field.Wire, "kind": field.Kind,
				"value": wire,
			})
			if !artifactBacked {
				values[v3AttributeName(field)] = value
			}
		}
		if artifactBacked {
			values["manifest_digest"] = types.StringValue("sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43")
		}

		host := newV3FakeHost(t)
		host.assignedOutputs = v3Provider3GoldenOutputs(t, form.Kind, form.Outputs)
		resource := v3Provider3CurrentResourceHarness(
			t, form, resourceType, newV3TestProviderData(t, host), codecs,
		)
		schemaResponse := v3SchemaOf(t, resource)
		plan := v3PlanWith(t, ctx, schemaResponse, values)
		createResponse := frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
		if createResponse.Diagnostics.HasError() {
			t.Fatalf("create %s: %v", resourceType, createResponse.Diagnostics)
		}
		if len(host.applyBodies) != 1 {
			t.Fatalf("%s apply body count = %d, want 1", resourceType, len(host.applyBodies))
		}
		v3AssertProvider3GoldenRequest(t, host.applyBodies[0], ref)

		readResponse := frameworkresource.ReadResponse{State: createResponse.State}
		resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
		if readResponse.Diagnostics.HasError() {
			t.Fatalf("read %s: %v", resourceType, readResponse.Diagnostics)
		}
		for name, want := range map[string]string{
			"form_api_version": ref.APIVersion, "form_kind": ref.Kind,
			"form_definition_version": ref.DefinitionVersion,
			"form_schema_digest":      ref.SchemaDigest,
			"form_package_digest":     ref.PackageDigest,
		} {
			if got := v3StateString(t, ctx, readResponse.State, name).ValueString(); got != want {
				t.Fatalf("%s state %s = %q, want %q", resourceType, name, got, want)
			}
		}

		resources[resourceType] = v3Provider3CodecGoldenResource{
			FormRef:       ref,
			FieldCodec:    v3CanonicalDigest(t, fieldVectors),
			ApplyRequest:  v3CanonicalDigest(t, host.applyBodies[0]),
			ReadbackState: formpackage.DigestBytes([]byte(readResponse.State.Raw.String())),
		}
	}
	if len(resources) != wantResourceCount {
		t.Fatalf("codec golden resource count = %d, want %d", len(resources), wantResourceCount)
	}
	return v3Provider3CodecGolden{
		Format:        "takoform.provider3-codec-golden@v1",
		SourceTag:     "v3.0.0",
		SourceCommit:  "a225cfa7c84aa551981cc8ad56c9a281fa6e051a",
		ResourceCount: wantResourceCount,
		Resources:     resources,
	}
}

func v3Provider3GoldenOutputs(t *testing.T, kind string, outputs []model.Field) map[string]any {
	t.Helper()
	if len(outputs) == 0 {
		return nil
	}
	result := make(map[string]any, len(outputs))
	for _, output := range outputs {
		switch output.Wire {
		case "hostname":
			result[output.Wire] = "assigned.example.invalid"
		case "url":
			result[output.Wire] = "https://assigned.example.invalid/"
		default:
			t.Fatalf("%s output %s has no immutable readback vector", kind, output.Wire)
		}
	}
	return result
}

func v3AssertProvider3GoldenRequest(t *testing.T, body map[string]any, ref v3FormRef) {
	t.Helper()
	if _, leaked := body["packageDigest"]; leaked {
		t.Fatal("apply request asserted a packageDigest")
	}
	formEnvelope, ok := body["form"].(map[string]any)
	if !ok {
		t.Fatalf("apply request form = %#v", body["form"])
	}
	form, ok := formEnvelope["formRef"].(map[string]any)
	if !ok || len(formEnvelope) != 1 {
		t.Fatalf("apply request form envelope = %#v", formEnvelope)
	}
	if _, leaked := form["packageDigest"]; leaked {
		t.Fatal("apply request FormRef asserted a packageDigest")
	}
	want := map[string]any{
		"apiVersion": ref.APIVersion, "kind": ref.Kind,
		"definitionVersion": ref.DefinitionVersion, "schemaDigest": ref.SchemaDigest,
	}
	if !reflect.DeepEqual(form, want) {
		t.Fatalf("apply request FormRef = %#v, want %#v", form, want)
	}
}

func v3CanonicalValueEqual(t *testing.T, left, right any) bool {
	t.Helper()
	return v3CanonicalDigest(t, left) == v3CanonicalDigest(t, right)
}

func v3CanonicalDigest(t *testing.T, value any) string {
	t.Helper()
	// Canonicalize accepts a JSON object document. Wrapping also lets scalar
	// field values use the exact same digest path as maps and arrays.
	raw, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return formpackage.DigestBytes(canonical)
}

func readV3Provider3CodecGolden(t *testing.T) v3Provider3CodecGolden {
	t.Helper()
	raw, err := os.ReadFile(v3Provider3CodecGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v", v3Provider3CodecGoldenPath, err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", v3Provider3CodecGoldenPath, err)
	}
	if !bytes.Equal(raw, canonical) {
		t.Fatalf("%s is not canonical JSON", v3Provider3CodecGoldenPath)
	}
	var golden v3Provider3CodecGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode %s: %v", v3Provider3CodecGoldenPath, err)
	}
	if golden.Format != "takoform.provider3-codec-golden@v1" || golden.SourceTag != "v3.0.0" ||
		golden.SourceCommit != "a225cfa7c84aa551981cc8ad56c9a281fa6e051a" {
		t.Fatalf("codec golden immutable source fence drifted: format=%q tag=%q commit=%q", golden.Format, golden.SourceTag, golden.SourceCommit)
	}
	if golden.ResourceCount != 31 || golden.ResourceCount != len(golden.Resources) {
		t.Fatalf("codec golden count = %d resources=%d, want 31", golden.ResourceCount, len(golden.Resources))
	}
	return golden
}
