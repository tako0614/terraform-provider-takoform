package provider

import (
	"context"
	"encoding/json"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// TestV3Provider211RetainedCanonicalImportAdoptsHostExactRefs proves the
// import path for every mapped retained identity. The canonical import ID
// carries the historical exact FormRef, and the subsequent Host read echoes
// and dispatches that same ref; a short/default import would not satisfy the
// query assertion when the retained identity differs from the current line.
func TestV3Provider211RetainedCanonicalImportAdoptsHostExactRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	want := readV3Provider211RetainedGolden(t)
	codecs := v3Codecs()

	for _, expected := range want.Identities {
		expected := expected
		if !expected.Provider3Readable {
			continue
		}
		t.Run(expected.FormRef.Kind, func(t *testing.T) {
			ref := expected.FormRef
			codec, ok := codecs.forStateKey(ref.ExactKey())
			if !ok || codec.Ref != ref {
				t.Fatalf("retained import codec for %s = %#v/%t, want %#v", ref.Kind, codec.Ref, ok, ref)
			}
			form := v3ProviderCurrentFormByKind(t, ref.Kind)
			host := newV3FakeHost(t)
			resource := v3Provider3CurrentResourceHarness(
				t, form, expected.Provider3Mapping, newV3TestProviderData(t, host), codecs,
			)
			schemaResponse := v3SchemaOf(t, resource)
			spec := v3Provider211RetainedHostSpec(t, codec)
			const space = "retained-import-space"
			const name = "retained-imported"
			const uid = "uid-imported"
			host.resources[host.resourceKey(ref.Kind, name)] = &v3HostRecord{
				uid: uid, generation: 1, revision: 1,
				apiVersion: ref.APIVersion, kind: ref.Kind, form: v3Provider211RetainedWireForm(ref),
				space: space, spec: spec, outputs: v3Provider211RetainedOutputs(t, form.Outputs),
			}

			idRaw, err := json.Marshal(v3ImportDocument{
				Space:             space,
				APIVersion:        ref.APIVersion,
				Kind:              ref.Kind,
				DefinitionVersion: ref.DefinitionVersion,
				SchemaDigest:      ref.SchemaDigest,
				Name:              name,
			})
			if err != nil {
				t.Fatal(err)
			}
			canonicalID, err := formpackage.Canonicalize(idRaw)
			if err != nil {
				t.Fatal(err)
			}
			importResponse := frameworkresource.ImportStateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: string(canonicalID)}, &importResponse)
			if importResponse.Diagnostics.HasError() {
				t.Fatalf("canonical import of retained %s: %v", ref.Kind, importResponse.Diagnostics)
			}
			for attribute, wantValue := range map[string]string{
				"name":                    name,
				"space":                   space,
				"form_api_version":        ref.APIVersion,
				"form_kind":               ref.Kind,
				"form_definition_version": ref.DefinitionVersion,
				"form_schema_digest":      ref.SchemaDigest,
				"form_package_digest":     ref.PackageDigest,
			} {
				if got := v3StateString(t, ctx, importResponse.State, attribute).ValueString(); got != wantValue {
					t.Fatalf("retained %s imported %s = %q, want %q", ref.Kind, attribute, got, wantValue)
				}
			}

			readResponse := frameworkresource.ReadResponse{State: importResponse.State}
			resource.Read(ctx, frameworkresource.ReadRequest{State: importResponse.State}, &readResponse)
			if readResponse.Diagnostics.HasError() {
				t.Fatalf("read after retained %s import: %v", ref.Kind, readResponse.Diagnostics)
			}
			if len(host.resourceQueries) != 1 || host.resourceQueries[0] != "GET "+ref.APIVersion+" "+ref.SchemaDigest {
				t.Fatalf("retained %s import read query = %v, want exact %s", ref.Kind, host.resourceQueries, ref.SchemaDigest)
			}
			for attribute, wantValue := range map[string]string{
				"uid":                     uid,
				"generation":              "1",
				"revision":                "1",
				"form_api_version":        ref.APIVersion,
				"form_kind":               ref.Kind,
				"form_definition_version": ref.DefinitionVersion,
				"form_schema_digest":      ref.SchemaDigest,
				"form_package_digest":     ref.PackageDigest,
			} {
				if got := v3StateString(t, ctx, readResponse.State, attribute).ValueString(); got != wantValue {
					t.Fatalf("retained %s readback %s = %q, want %q", ref.Kind, attribute, got, wantValue)
				}
			}
		})
	}
}
