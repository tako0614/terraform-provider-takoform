package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

func workerVersionAssetsValue() types.Object {
	typeShape := types.ObjectType{AttrTypes: map[string]attr.Type{
		"bundle":             types.StringType,
		"run_worker_first":   types.BoolType,
		"not_found_handling": types.StringType,
	}}
	return types.ObjectValueMust(typeShape.AttrTypes, map[string]attr.Value{
		"bundle":             types.StringValue("static-asset-bundle"),
		"run_worker_first":   types.BoolValue(true),
		"not_found_handling": types.StringValue("single_page_application"),
	})
}

func TestV3WorkerVersionAssetsIsOneTypedClosedObject(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	assetsSchema, ok := schemaResponse.Schema.Attributes["assets"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("assets schema = %T, want schema.SingleNestedAttribute", schemaResponse.Schema.Attributes["assets"])
	}
	if !assetsSchema.IsOptional() || assetsSchema.IsComputed() || assetsSchema.IsRequired() {
		t.Fatalf("assets flags required=%v optional=%v computed=%v", assetsSchema.IsRequired(), assetsSchema.IsOptional(), assetsSchema.IsComputed())
	}
	for _, name := range []string{"bundle", "run_worker_first", "not_found_handling"} {
		member, declared := assetsSchema.Attributes[name]
		if !declared || !member.IsRequired() {
			t.Errorf("assets.%s is not a required typed member", name)
		}
	}

	handlers := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")})
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":     types.StringValue("worker-version"),
		"worker":   types.StringValue("module-worker"),
		"bundle":   types.StringValue("worker-bundle"),
		"handlers": handlers,
		"assets":   workerVersionAssetsValue(),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create WorkerVersion with assets: %v", createResponse.Diagnostics)
	}
	want := map[string]any{
		"worker": map[string]any{
			"apiVersion": edgeformcatalog.Family.APIVersion(), "kind": "ModuleWorker", "name": "module-worker",
		},
		"bundle": map[string]any{
			"apiVersion": edgeformcatalog.Family.APIVersion(), "kind": "WorkerBundle", "name": "worker-bundle",
		},
		"handlers": []any{"fetch"},
		"assets": map[string]any{
			"bundle": map[string]any{
				"apiVersion": edgeformcatalog.Family.APIVersion(), "kind": "StaticAssetBundle", "name": "static-asset-bundle",
			},
			"runWorkerFirst":   true,
			"notFoundHandling": "single_page_application",
		},
	}
	if len(host.applySpecs) != 1 || !reflect.DeepEqual(host.applySpecs[0], want) {
		t.Fatalf("WorkerVersion wire spec = %#v, want %#v", host.applySpecs, want)
	}

	var stateAssets types.Object
	if diags := createResponse.State.GetAttribute(ctx, path.Root("assets"), &stateAssets); diags.HasError() {
		t.Fatalf("read assets state: %v", diags)
	}
	if !stateAssets.Equal(workerVersionAssetsValue()) {
		t.Fatalf("assets state = %v, want exact authored object", stateAssets)
	}
}

func edgeAppArtifactFilesValue(pathName, mediaType, contentFile string) types.List {
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"path":         types.StringType,
		"media_type":   types.StringType,
		"content_file": types.StringType,
		"size":         types.Int64Type,
		"digest":       types.StringType,
	}}
	return types.ListValueMust(elementType, []attr.Value{
		types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"path":         types.StringValue(pathName),
			"media_type":   types.StringValue(mediaType),
			"content_file": types.StringValue(contentFile),
			"size":         types.Int64Unknown(),
			"digest":       types.StringUnknown(),
		}),
	})
}

func edgeAppManifestDigest(t *testing.T, kind, pathName, mediaType string, raw []byte) (string, string) {
	t.Helper()
	sum := sha256.Sum256(raw)
	blobDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifest := map[string]any{
		"apiVersion": artifactManifestAPIVersion,
		"kind":       kind,
		"files": []any{map[string]any{
			"path": pathName, "mediaType": mediaType, "size": int64(len(raw)), "digest": blobDigest,
		}},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := formpackage.DigestCanonicalJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return manifestDigest, blobDigest
}

func TestV3EdgeAppFileBundlesUploadBytesButStateOnlyCarriesEvidence(t *testing.T) {
	tests := []struct {
		formKind, manifestKind, pathName, mediaType string
		content                                     []byte
	}{
		{
			formKind: "StaticAssetBundle", manifestKind: "StaticAssetBundle",
			pathName: "index.html", mediaType: "text/html",
			content: []byte("<!doctype html><title>portable edge app</title>\n"),
		},
		{
			formKind: "SQLiteMigrationSet", manifestKind: "MigrationBundle",
			pathName: "0001_create_messages.sql", mediaType: "application/sql",
			content: []byte("CREATE TABLE messages (id TEXT PRIMARY KEY);\n"),
		},
	}
	for _, test := range tests {
		t.Run(test.formKind, func(t *testing.T) {
			host := newV3FakeHost(t)
			resource := v3TestFormResource(t, test.formKind, newV3TestProviderData(t, host))
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)

			manifestAttribute, declared := schemaResponse.Schema.Attributes["manifest_digest"]
			if !declared || !manifestAttribute.IsOptional() || !manifestAttribute.IsComputed() {
				t.Fatalf("manifest_digest must support either committed-digest or local-file authoring")
			}
			if _, declared := schemaResponse.Schema.Attributes["sql"]; declared {
				t.Fatal("raw SQL is a provider attribute")
			}
			if _, declared := schemaResponse.Schema.Attributes["files"].(schema.ListNestedAttribute); !declared {
				t.Fatalf("files schema = %T, want ListNestedAttribute", schemaResponse.Schema.Attributes["files"])
			}

			contentFile := v3BundleFile(t, t.TempDir(), "payload", test.content)
			wantManifest, wantBlob := edgeAppManifestDigest(t, test.manifestKind, test.pathName, test.mediaType, test.content)
			plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
				"name":  types.StringValue(strings.ToLower(test.formKind)),
				"files": edgeAppArtifactFilesValue(test.pathName, test.mediaType, contentFile),
			})
			createResponse := frameworkresource.CreateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
			if createResponse.Diagnostics.HasError() {
				t.Fatalf("create: %v", createResponse.Diagnostics)
			}
			if !reflect.DeepEqual(host.blobs[wantBlob], test.content) {
				t.Fatal("uploaded blob bytes differ from the authored file")
			}
			if len(host.applySpecs) != 1 || !reflect.DeepEqual(host.applySpecs[0], map[string]any{"manifestDigest": wantManifest}) {
				t.Fatalf("applied spec = %#v, want digest-only desired state", host.applySpecs)
			}
			if got := v3StateString(t, ctx, createResponse.State, "manifest_digest").ValueString(); got != wantManifest {
				t.Fatalf("manifest_digest state = %q, want %q", got, wantManifest)
			}
			var stateFiles types.List
			if diags := createResponse.State.GetAttribute(ctx, path.Root("files"), &stateFiles); diags.HasError() {
				t.Fatalf("files state: %v", diags)
			}
			fileState := stateFiles.Elements()[0].(types.Object).Attributes()
			if got := fileState["content_file"].(types.String).ValueString(); got != contentFile {
				t.Fatalf("content_file state = %q", got)
			}
			if got := fileState["digest"].(types.String).ValueString(); got != wantBlob {
				t.Fatalf("file digest state = %q, want %q", got, wantBlob)
			}
			if got := fileState["size"].(types.Int64).ValueInt64(); got != int64(len(test.content)) {
				t.Fatalf("file size state = %d, want %d", got, len(test.content))
			}
			var offenders []string
			if err := tftypes.Walk(createResponse.State.Raw, func(attributePath *tftypes.AttributePath, value tftypes.Value) (bool, error) {
				if !value.IsKnown() || value.IsNull() || !value.Type().Is(tftypes.String) {
					return true, nil
				}
				var text string
				if err := value.As(&text); err == nil && strings.Contains(text, string(test.content)) {
					offenders = append(offenders, attributePath.String())
				}
				return true, nil
			}); err != nil {
				t.Fatalf("walking state: %v", err)
			}
			if len(offenders) != 0 {
				t.Fatalf("raw file bytes entered provider state at %v", offenders)
			}
		})
	}
}

func TestV3SQLiteMigrationSetRejectsNonSQLFileBeforeUpload(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "SQLiteMigrationSet", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	contentFile := v3BundleFile(t, t.TempDir(), "0001.sql", []byte("SELECT 1;\n"))
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":  types.StringValue("migration-set"),
		"files": edgeAppArtifactFilesValue("0001.sql", "text/plain", contentFile),
	})
	response := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("a MigrationBundle file outside application/sql was accepted")
	}
	if len(host.events) != 0 {
		t.Fatalf("invalid migration reached artifact/resource mutation: %v", host.events)
	}
}

func TestV3FileBundleAuthoringEnforcesPortableManifestStringCeilings(t *testing.T) {
	tests := []struct {
		name, pathName, mediaType string
	}{
		{name: "path", pathName: strings.Repeat("a", 241), mediaType: "text/plain"},
		{name: "media type", pathName: "asset.txt", mediaType: "application/" + strings.Repeat("a", 244)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := newV3FakeHost(t)
			resource := v3TestFormResource(t, "StaticAssetBundle", newV3TestProviderData(t, host))
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)
			contentFile := v3BundleFile(t, t.TempDir(), "payload", []byte("portable\n"))
			plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
				"name":  types.StringValue("static-assets"),
				"files": edgeAppArtifactFilesValue(test.pathName, test.mediaType, contentFile),
			})
			response := frameworkresource.CreateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("an artifact manifest string above its portable ceiling was accepted")
			}
			if len(host.events) != 0 {
				t.Fatalf("invalid manifest string reached artifact/resource mutation: %v", host.events)
			}
		})
	}
}
