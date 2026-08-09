package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

func v3EmptyRaw(t *testing.T, ctx context.Context, schemaResponse frameworkresource.SchemaResponse) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)
}

func v3SchemaOf(t *testing.T, candidate frameworkresource.Resource) frameworkresource.SchemaResponse {
	t.Helper()
	var schemaResponse frameworkresource.SchemaResponse
	candidate.Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResponse.Diagnostics)
	}
	return schemaResponse
}

// v3TestRevisionOwner is the owner a test declares for a revision whose name
// the provider derives. Every derived name is a function of content AND owner,
// so a test that omits the owner is asking the provider to hand one host
// address to whoever else builds the same bytes.
const v3TestRevisionOwner = "counter"

func v3PlanWith(t *testing.T, ctx context.Context, schemaResponse frameworkresource.SchemaResponse, values map[string]attr.Value) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}
	for name, value := range values {
		if diags := plan.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("set plan %s: %v", name, diags)
		}
	}
	return plan
}

func v3StateString(t *testing.T, ctx context.Context, state tfsdk.State, name string) types.String {
	t.Helper()
	var value types.String
	if diags := state.GetAttribute(ctx, path.Root(name), &value); diags.HasError() {
		t.Fatalf("get state %s: %v", name, diags)
	}
	return value
}

func TestV3ModuleWorkerCreateReadDeleteRoundTrip(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":  types.StringValue("module-worker"),
		"space": types.StringValue("prod"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	for name, want := range map[string]string{
		"uid": "uid-1", "generation": "1", "revision": "1",
		"form_api_version": "edge.forms.takoform.com/v1alpha1", "form_kind": "ModuleWorker",
		"outputs_json": "{}",
	} {
		if got := v3StateString(t, ctx, createResponse.State, name).ValueString(); got != want {
			t.Errorf("created state %s = %q, want %q", name, got, want)
		}
	}
	var ready types.Bool
	if diags := createResponse.State.GetAttribute(ctx, path.Root("ready"), &ready); diags.HasError() || !ready.ValueBool() {
		t.Errorf("created state ready = %v (%v), want true", ready, createResponse.State.Raw)
	}

	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, readResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("read state uid = %q, want uid-1", got)
	}

	deleteResponse := frameworkresource.DeleteResponse{}
	resource.Delete(ctx, frameworkresource.DeleteRequest{State: readResponse.State}, &deleteResponse)
	if deleteResponse.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResponse.Diagnostics)
	}
	if host.eventIndex("delete:ModuleWorker/module-worker") < 0 {
		t.Fatalf("delete never reached the host: %v", host.events)
	}
}

func TestV3Apply202OperationPathResolvesToResource(t *testing.T) {
	host := newV3FakeHost(t)
	host.apply202 = true
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("module-worker"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create through 202 operation: %v", createResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, createResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("202-created state uid = %q, want uid-1", got)
	}
}

// v3BundleFile writes one module file and returns its path.
func v3BundleFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

// v3BundleModulesValue builds the authored modules list of one single-module
// bundle, with the computed size and digest still unknown exactly as a fresh
// plan carries them.
func v3BundleModulesValue(name, contentFile string) types.List {
	moduleType := v3WorkerBundleModuleType()
	return types.ListValueMust(moduleType, []attr.Value{
		types.ObjectValueMust(moduleType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue(name),
			"content_type": types.StringValue("application/javascript+module"),
			"content_file": types.StringValue(contentFile),
			"size":         types.Int64Unknown(),
			"digest":       types.StringUnknown(),
		}),
	})
}

// v3ExpectedManifestDigest computes the manifest digest one authored module
// commits, through the same helpers the provider uses.
func v3ExpectedManifestDigest(t *testing.T, mainModule, contentFile string, content []byte) string {
	t.Helper()
	sum := sha256.Sum256(content)
	manifest := workerBundleManifest(mainModule, []v3BundleModule{{
		Name:        mainModule,
		ContentType: "application/javascript+module",
		ContentFile: contentFile,
		Size:        int64(len(content)),
		Digest:      "sha256:" + hex.EncodeToString(sum[:]),
	}})
	digest, err := digestWorkerBundleManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// TestV3WorkerBundleCreateUploadsThenAppliesOnlyTheManifestDigest proves the
// collapse onto one source of truth: local authoring commits the artifact
// manifest first, and the resource that follows carries exactly the returned
// manifest digest — no module list, no main module, no bytes.
func TestV3WorkerBundleCreateUploadsThenAppliesOnlyTheManifestDigest(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	workerBytes := []byte("export default { fetch() { return new Response(\"ok\") } }\n")
	workerFile := v3BundleFile(t, t.TempDir(), "worker.mjs", workerBytes)
	sum := sha256.Sum256(workerBytes)
	wantBlobDigest := "sha256:" + hex.EncodeToString(sum[:])
	wantManifestDigest := v3ExpectedManifestDigest(t, "worker.mjs", workerFile, workerBytes)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":        types.StringValue("worker-bundle"),
		"main_module": types.StringValue("worker.mjs"),
		"modules":     v3BundleModulesValue("worker.mjs", workerFile),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	// The host must receive: upload start, the blob, the manifest commit, and
	// only then the WorkerBundle apply.
	start := host.eventIndex("artifact-start")
	blob := host.eventIndex("blob:" + wantBlobDigest)
	commit := host.eventIndex("commit")
	apply := host.eventPrefixIndex("apply:WorkerBundle/")
	if start < 0 || blob < 0 || commit < 0 || apply < 0 || !(start < blob && blob < commit && commit < apply) {
		t.Fatalf("event order = %v, want artifact-start < blob < commit < apply", host.events)
	}
	if !reflect.DeepEqual(host.blobs[wantBlobDigest], workerBytes) {
		t.Fatalf("host blob bytes differ from the module file")
	}
	// The applied desired state is the manifest digest and nothing else: the
	// manifest, not the resource, describes the modules.
	if len(host.applySpecs) != 1 {
		t.Fatalf("apply count = %d, want exactly 1", len(host.applySpecs))
	}
	if !reflect.DeepEqual(host.applySpecs[0], map[string]any{"manifestDigest": wantManifestDigest}) {
		t.Fatalf("applied spec = %v, want exactly {manifestDigest: %s}", host.applySpecs[0], wantManifestDigest)
	}

	if got := v3StateString(t, ctx, createResponse.State, "manifest_digest").ValueString(); got != wantManifestDigest {
		t.Fatalf("state manifest_digest = %q, want %q", got, wantManifestDigest)
	}
	var stateModules types.List
	if diags := createResponse.State.GetAttribute(ctx, path.Root("modules"), &stateModules); diags.HasError() {
		t.Fatalf("state modules: %v", diags)
	}
	element := stateModules.Elements()[0].(types.Object).Attributes()
	if got := element["digest"].(types.String).ValueString(); got != wantBlobDigest {
		t.Fatalf("state module digest = %q, want %q", got, wantBlobDigest)
	}
	if got := element["size"].(types.Int64).ValueInt64(); got != int64(len(workerBytes)) {
		t.Fatalf("state module size = %d, want %d", got, len(workerBytes))
	}
	if got := element["content_file"].(types.String).ValueString(); got != workerFile {
		t.Fatalf("state module content_file = %q, want the local path %q", got, workerFile)
	}
}

// TestV3WorkerBundleManifestDigestAuthoringPerformsNoUpload proves the second
// authoring mode: a digest that already names a committed manifest is applied
// as-is, and the provider touches the artifact API not at all.
func TestV3WorkerBundleManifestDigestAuthoringPerformsNoUpload(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	const digest = "sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43"
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":            types.StringValue("worker-bundle"),
		"manifest_digest": types.StringValue(digest),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if host.eventIndex("artifact-start") >= 0 || host.eventIndex("commit") >= 0 {
		t.Fatalf("manifest-digest authoring reached the artifact API: %v", host.events)
	}
	if len(host.applySpecs) != 1 ||
		!reflect.DeepEqual(host.applySpecs[0], map[string]any{"manifestDigest": digest}) {
		t.Fatalf("applied spec = %v, want exactly {manifestDigest: %s}", host.applySpecs, digest)
	}
	var mainModule types.String
	if diags := createResponse.State.GetAttribute(ctx, path.Root("main_module"), &mainModule); diags.HasError() {
		t.Fatalf("state main_module: %v", diags)
	}
	if !mainModule.IsNull() {
		t.Fatalf("state main_module = %v, want null for manifest-digest authoring", mainModule)
	}
}

// TestV3WorkerBundleBothSpellingsMustAgree proves the one rule that keeps two
// spellings of one identity honest: writing a manifest_digest alongside local
// authoring is accepted only when the authored bytes commit exactly that
// manifest, and the disagreement is caught before any host call.
func TestV3WorkerBundleBothSpellingsMustAgree(t *testing.T) {
	ctx := context.Background()
	workerBytes := []byte("export default { fetch() { return new Response(\"agree\") } }\n")
	dir := t.TempDir()
	workerFile := v3BundleFile(t, dir, "worker.mjs", workerBytes)
	agreeing := v3ExpectedManifestDigest(t, "worker.mjs", workerFile, workerBytes)

	disagreeingHost := newV3FakeHost(t)
	disagreeing := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, disagreeingHost))
	schemaResponse := v3SchemaOf(t, disagreeing)
	conflicting := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":            types.StringValue("worker-bundle"),
		"manifest_digest": types.StringValue("sha256:" + strings.Repeat("0", 64)),
		"main_module":     types.StringValue("worker.mjs"),
		"modules":         v3BundleModulesValue("worker.mjs", workerFile),
	})
	conflictResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	disagreeing.Create(ctx, frameworkresource.CreateRequest{Plan: conflicting}, &conflictResponse)
	if !conflictResponse.Diagnostics.HasError() {
		t.Fatal("a manifest_digest that contradicts the authored bytes was accepted")
	}
	if len(disagreeingHost.events) != 0 {
		t.Fatalf("the rejected bundle still reached the host: %v", disagreeingHost.events)
	}

	agreeingHost := newV3FakeHost(t)
	accepted := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, agreeingHost))
	agreeingPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":            types.StringValue("worker-bundle"),
		"manifest_digest": types.StringValue(agreeing),
		"main_module":     types.StringValue("worker.mjs"),
		"modules":         v3BundleModulesValue("worker.mjs", workerFile),
	})
	acceptedResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	accepted.Create(ctx, frameworkresource.CreateRequest{Plan: agreeingPlan}, &acceptedResponse)
	if acceptedResponse.Diagnostics.HasError() {
		t.Fatalf("agreeing spellings rejected: %v", acceptedResponse.Diagnostics)
	}
	if len(agreeingHost.applySpecs) != 1 ||
		!reflect.DeepEqual(agreeingHost.applySpecs[0], map[string]any{"manifestDigest": agreeing}) {
		t.Fatalf("applied spec = %v, want exactly {manifestDigest: %s}", agreeingHost.applySpecs, agreeing)
	}
}

// TestV3WorkerBundleModifyPlanDetectsChangedBytes proves plan-time byte
// identity: rewriting a module file at an UNCHANGED content_file path makes
// the plan carry a different MANIFEST digest and forces replacement.
func TestV3WorkerBundleModifyPlanDetectsChangedBytes(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	dir := t.TempDir()
	originalBytes := []byte("export default { fetch() { return new Response(\"one\") } }\n")
	workerFile := v3BundleFile(t, dir, "worker.mjs", originalBytes)
	originalManifest := v3ExpectedManifestDigest(t, "worker.mjs", workerFile, originalBytes)

	// The name is deliberately NOT written: a revision Form derives it from its
	// own content and its declared owner, so the bundle's identity and its host
	// name move together.
	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"main_module":            types.StringValue("worker.mjs"),
		"modules":                v3BundleModulesValue("worker.mjs", workerFile),
		v3RevisionOwnerAttribute: types.StringValue(v3TestRevisionOwner),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, createResponse.State, "manifest_digest").ValueString(); got != originalManifest {
		t.Fatalf("created state manifest_digest = %q, want %q", got, originalManifest)
	}
	originalName := v3StateString(t, ctx, createResponse.State, "name").ValueString()
	if want, _ := v3DerivedRevisionName("bundle", v3TestRevisionOwner, originalManifest); originalName != want {
		t.Fatalf("created state name = %q, want the derived %q", originalName, want)
	}

	// Rewrite the module bytes at the SAME path.
	changedBytes := []byte("export default { fetch() { return new Response(\"two\") } }\n")
	if err := os.WriteFile(workerFile, changedBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(changedBytes)
	wantBlobDigest := "sha256:" + hex.EncodeToString(sum[:])
	wantManifestDigest := v3ExpectedManifestDigest(t, "worker.mjs", workerFile, changedBytes)

	// Terraform's proposed new state for an unchanged config equals prior
	// state; without plan-time byte identity this plan would claim no change.
	proposed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	modifyResponse := frameworkresource.ModifyPlanResponse{Plan: proposed}
	authored := v3ConfigWith(t, ctx, schemaResponse, map[string]attr.Value{
		"main_module":            types.StringValue("worker.mjs"),
		"modules":                v3BundleModulesValue("worker.mjs", workerFile),
		v3RevisionOwnerAttribute: types.StringValue(v3TestRevisionOwner),
	})
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  createResponse.State,
		Plan:   proposed,
		Config: authored,
	}, &modifyResponse)
	if modifyResponse.Diagnostics.HasError() {
		t.Fatalf("modify plan: %v", modifyResponse.Diagnostics)
	}
	var plannedDigest types.String
	if diags := modifyResponse.Plan.GetAttribute(ctx, path.Root("manifest_digest"), &plannedDigest); diags.HasError() {
		t.Fatalf("planned manifest_digest: %v", diags)
	}
	if plannedDigest.ValueString() != wantManifestDigest || wantManifestDigest == originalManifest {
		t.Fatalf(
			"planned manifest_digest = %q, want the rewritten bytes' manifest %q (prior %q)",
			plannedDigest.ValueString(), wantManifestDigest, originalManifest,
		)
	}
	var plannedModules types.List
	if diags := modifyResponse.Plan.GetAttribute(ctx, path.Root("modules"), &plannedModules); diags.HasError() {
		t.Fatalf("planned modules: %v", diags)
	}
	element := plannedModules.Elements()[0].(types.Object).Attributes()
	if got := element["digest"].(types.String).ValueString(); got != wantBlobDigest {
		t.Fatalf("planned module digest = %q, want the rewritten bytes' digest %q", got, wantBlobDigest)
	}
	if got := element["size"].(types.Int64).ValueInt64(); got != int64(len(changedBytes)) {
		t.Fatalf("planned module size = %d, want %d", got, len(changedBytes))
	}
	if !modifyResponse.RequiresReplace.Contains(path.Root("manifest_digest")) {
		t.Fatalf("changed module bytes did not force replacement: %v", modifyResponse.RequiresReplace)
	}
	// The derived name moves with the bytes, which is what makes the
	// replacement land beside the old revision instead of on top of it.
	var plannedName types.String
	if diags := modifyResponse.Plan.GetAttribute(ctx, path.Root("name"), &plannedName); diags.HasError() {
		t.Fatalf("planned name: %v", diags)
	}
	wantName, _ := v3DerivedRevisionName("bundle", v3TestRevisionOwner, wantManifestDigest)
	if plannedName.ValueString() != wantName || plannedName.ValueString() == originalName {
		t.Fatalf("planned name = %q, want the derived %q (prior %q)", plannedName.ValueString(), wantName, originalName)
	}
	if !modifyResponse.RequiresReplace.Contains(path.Root("name")) {
		t.Fatalf("a changed derived name did not force replacement: %v", modifyResponse.RequiresReplace)
	}

	// Restoring the original bytes removes the diff: the proposed plan (prior
	// state identity) is left untouched and no replacement is forced.
	if err := os.WriteFile(workerFile, originalBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	unchangedProposed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	unchangedResponse := frameworkresource.ModifyPlanResponse{Plan: unchangedProposed}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  createResponse.State,
		Plan:   unchangedProposed,
		Config: authored,
	}, &unchangedResponse)
	if unchangedResponse.Diagnostics.HasError() {
		t.Fatalf("unchanged modify plan: %v", unchangedResponse.Diagnostics)
	}
	if len(unchangedResponse.RequiresReplace) != 0 {
		t.Fatalf("identical bytes forced replacement: %v", unchangedResponse.RequiresReplace)
	}
	if !unchangedResponse.Plan.Raw.Equal(createResponse.State.Raw) {
		t.Fatal("identical bytes changed the proposed plan")
	}
}

// TestV3WorkerBundleImportRestoresManifestDigest proves an imported bundle is
// manageable: import restores the digest the host serves and leaves the local
// authoring attributes null, the next plan is empty, and adopting local files
// that commit exactly that manifest is not a change either.
func TestV3WorkerBundleImportRestoresManifestDigest(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	workerBytes := []byte("export default { fetch() { return new Response(\"imported\") } }\n")
	workerFile := v3BundleFile(t, t.TempDir(), "worker.mjs", workerBytes)
	digest := v3ExpectedManifestDigest(t, "worker.mjs", workerFile, workerBytes)

	// The bundle already exists on the host, referencing a committed manifest.
	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":            types.StringValue("worker-bundle"),
		"manifest_digest": types.StringValue(digest),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	importResponse := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: "prod/worker-bundle"}, &importResponse)
	if importResponse.Diagnostics.HasError() {
		t.Fatalf("import: %v", importResponse.Diagnostics)
	}
	readResponse := frameworkresource.ReadResponse{State: importResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: importResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read after import: %v", readResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, readResponse.State, "manifest_digest").ValueString(); got != digest {
		t.Fatalf("imported manifest_digest = %q, want the host's %q", got, digest)
	}
	if got := v3StateString(t, ctx, readResponse.State, "main_module"); !got.IsNull() {
		t.Fatalf("imported main_module = %v, want null", got)
	}
	var importedModules types.List
	if diags := readResponse.State.GetAttribute(ctx, path.Root("modules"), &importedModules); diags.HasError() {
		t.Fatalf("imported modules: %v", diags)
	}
	if !importedModules.IsNull() {
		t.Fatalf("imported modules = %v, want null", importedModules)
	}

	// A plan that re-states the same manifest digest is empty.
	sameDigest := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: readResponse.State.Raw}
	sameResponse := frameworkresource.ModifyPlanResponse{Plan: sameDigest}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: readResponse.State,
		Plan:  sameDigest,
		// An imported bundle's name came from the import ID, so the configuration
		// that adopts it pins that name rather than deriving one.
		Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: readResponse.State.Raw},
	}, &sameResponse)
	if sameResponse.Diagnostics.HasError() {
		t.Fatalf("same-digest plan: %v", sameResponse.Diagnostics)
	}
	if len(sameResponse.RequiresReplace) != 0 || !sameResponse.Plan.Raw.Equal(readResponse.State.Raw) {
		t.Fatalf("re-stating the same manifest digest was not an empty plan: %v", sameResponse.RequiresReplace)
	}

	// Adopting the local files that commit exactly that manifest is the same
	// bundle: the authoring attributes carry attribute-level replacement, and
	// the unchanged digest withdraws it.
	local := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: readResponse.State.Raw}
	if diags := local.SetAttribute(ctx, path.Root("main_module"), types.StringValue("worker.mjs")); diags.HasError() {
		t.Fatalf("set planned main_module: %v", diags)
	}
	if diags := local.SetAttribute(ctx, path.Root("modules"), v3BundleModulesValue("worker.mjs", workerFile)); diags.HasError() {
		t.Fatalf("set planned modules: %v", diags)
	}
	localResponse := frameworkresource.ModifyPlanResponse{
		Plan:            local,
		RequiresReplace: path.Paths{path.Root("main_module"), path.Root("modules")},
	}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  readResponse.State,
		Plan:   local,
		Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: readResponse.State.Raw},
	}, &localResponse)
	if localResponse.Diagnostics.HasError() {
		t.Fatalf("local-authoring plan: %v", localResponse.Diagnostics)
	}
	if len(localResponse.RequiresReplace) != 0 {
		t.Fatalf("adopting byte-identical local authoring forced replacement: %v", localResponse.RequiresReplace)
	}
	var adoptedDigest types.String
	if diags := localResponse.Plan.GetAttribute(ctx, path.Root("manifest_digest"), &adoptedDigest); diags.HasError() {
		t.Fatalf("planned manifest_digest: %v", diags)
	}
	if adoptedDigest.ValueString() != digest {
		t.Fatalf("local authoring planned manifest_digest = %q, want the unchanged %q", adoptedDigest.ValueString(), digest)
	}

	// And the in-place apply of that plan touches no host state.
	eventsBefore := len(host.events)
	updateResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{
		Plan: localResponse.Plan, State: readResponse.State,
	}, &updateResponse)
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("adopting local authoring: %v", updateResponse.Diagnostics)
	}
	if len(host.events) != eventsBefore {
		t.Fatalf("adopting local authoring mutated the host: %v", host.events[eventsBefore:])
	}
	if got := v3StateString(t, ctx, updateResponse.State, "main_module").ValueString(); got != "worker.mjs" {
		t.Fatalf("adopted state main_module = %q, want worker.mjs", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "manifest_digest").ValueString(); got != digest {
		t.Fatalf("adopted state manifest_digest = %q, want %q", got, digest)
	}
}

// TestV3WorkerBundleTimeoutOnlyUpdateSucceedsWithoutHostMutation proves the
// one legal in-place update of a revision-role resource: changing only the
// provider-side timeout attributes writes the plan to state without any host
// call, while an update carrying a desired-spec change stays a hard error.
func TestV3WorkerBundleTimeoutOnlyUpdateSucceedsWithoutHostMutation(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	workerBytes := []byte("export default {}\n")
	workerFile := v3BundleFile(t, t.TempDir(), "worker.mjs", workerBytes)
	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":        types.StringValue("worker-bundle"),
		"main_module": types.StringValue("worker.mjs"),
		"modules":     v3BundleModulesValue("worker.mjs", workerFile),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	hostEventsAfterCreate := len(host.events)

	// A timeout-only change: the plan is the prior state with a new
	// create_timeout. It must succeed with no host traffic.
	timeoutPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	if diags := timeoutPlan.SetAttribute(ctx, path.Root("create_timeout"), types.StringValue("25m")); diags.HasError() {
		t.Fatalf("set planned create_timeout: %v", diags)
	}
	updateResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: timeoutPlan, State: createResponse.State}, &updateResponse)
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("timeout-only update: %v", updateResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "create_timeout").ValueString(); got != "25m" {
		t.Fatalf("updated state create_timeout = %q, want 25m", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("timeout-only update lost state uid: %q", got)
	}
	if len(host.events) != hostEventsAfterCreate {
		t.Fatalf("timeout-only update reached the host: %v", host.events[hostEventsAfterCreate:])
	}

	// A desired-spec change planned as an in-place update stays a hard error
	// and still performs no host mutation.
	specPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	for name, value := range map[string]attr.Value{
		"manifest_digest": types.StringValue("sha256:" + strings.Repeat("1", 64)),
		"main_module":     types.StringNull(),
		"modules":         types.ListNull(v3WorkerBundleModuleType()),
	} {
		if diags := specPlan.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("set planned %s: %v", name, diags)
		}
	}
	rejectedResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: specPlan, State: createResponse.State}, &rejectedResponse)
	if !rejectedResponse.Diagnostics.HasError() {
		t.Fatal("spec-changing in-place update was accepted")
	}
	if len(host.events) != hostEventsAfterCreate {
		t.Fatalf("rejected update reached the host: %v", host.events[hostEventsAfterCreate:])
	}
}

func TestV3WorkerVersionBindingBlocksUseResourceWireKey(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	bindingType := v3BindingObjectType()
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":     types.StringValue("worker-version"),
		"worker":   types.StringValue("module-worker"),
		"bundle":   types.StringValue("worker-bundle"),
		"handlers": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		"kv_bindings": types.ListValueMust(bindingType, []attr.Value{
			types.ObjectValueMust(bindingType.AttrTypes, map[string]attr.Value{
				"name":        types.StringValue("CACHE"),
				"target_name": types.StringValue("edge-kv-namespace"),
			}),
		}),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if len(host.applySpecs) == 0 {
		t.Fatal("apply never reached the host")
	}
	spec := host.applySpecs[len(host.applySpecs)-1]
	group := edgeformcatalog.Family.APIVersion()
	// The wire carries the exact three-member reference; the HCL surface stays
	// the bare target name.
	want := []any{map[string]any{
		"name": "CACHE",
		"resource": map[string]any{
			"apiVersion": group, "kind": "EdgeKVNamespace", "name": "edge-kv-namespace",
		},
	}}
	if !reflect.DeepEqual(spec["kvBindings"], want) {
		t.Fatalf("kvBindings wire shape = %#v, want %#v", spec["kvBindings"], want)
	}
	if entry, ok := spec["kvBindings"].([]any)[0].(map[string]any); ok {
		if _, hasTarget := entry["target"]; hasTarget {
			t.Fatal("binding element must carry `resource`, never `target`")
		}
	}
	if !reflect.DeepEqual(spec["worker"], map[string]any{
		"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker",
	}) {
		t.Fatalf("worker reference wire shape = %#v", spec["worker"])
	}
}

func TestV3DeploymentWeightSumValidator(t *testing.T) {
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"worker_version": types.StringType,
		"weight":         types.Int64Type,
	}}
	buildRequest := func(weights ...int64) validator.ListRequest {
		elements := make([]attr.Value, 0, len(weights))
		for _, weight := range weights {
			elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
				"worker_version": types.StringValue("worker-version"),
				"weight":         types.Int64Value(weight),
			}))
		}
		return validator.ListRequest{
			Path:        path.Root("versions"),
			ConfigValue: types.ListValueMust(elementType, elements),
		}
	}

	var rejected validator.ListResponse
	v3WeightSumValidator{}.ValidateList(context.Background(), buildRequest(9999), &rejected)
	if !rejected.Diagnostics.HasError() {
		t.Fatal("weight sum 9999 was accepted")
	}
	var accepted validator.ListResponse
	v3WeightSumValidator{}.ValidateList(context.Background(), buildRequest(4000, 6000), &accepted)
	if accepted.Diagnostics.HasError() {
		t.Fatalf("weight sum 10000 was rejected: %v", accepted.Diagnostics)
	}
}

func TestV3UpdateCarriesGenerationAndUIDFence(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerCronTrigger", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":   types.StringValue("worker-cron-trigger"),
		"worker": types.StringValue("module-worker"),
		"cron":   types.StringValue("0 3 * * *"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	updatePlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":   types.StringValue("worker-cron-trigger"),
		"space":  types.StringValue("prod"),
		"worker": types.StringValue("module-worker"),
		"cron":   types.StringValue("15 0 * * *"),
	})
	updateResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: updatePlan, State: createResponse.State}, &updateResponse)
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResponse.Diagnostics)
	}

	updateHeaders := host.applyHeaders[len(host.applyHeaders)-1]
	if got := updateHeaders.Get("Takoform-Expected-Generation"); got != "1" {
		t.Fatalf("update generation fence = %q, want %q", got, "1")
	}
	if got := updateHeaders.Get("If-None-Match"); got != "" {
		t.Fatalf("update must not send If-None-Match, got %q", got)
	}
	updateBody := host.applyBodies[len(host.applyBodies)-1]
	if got, _ := updateBody["expectedUid"].(string); got != "uid-1" {
		t.Fatalf("update expectedUid = %q, want uid-1", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "generation").ValueString(); got != "2" {
		t.Fatalf("updated state generation = %q, want 2", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "revision").ValueString(); got != "2" {
		t.Fatalf("updated state revision = %q, want 2", got)
	}
}

func TestV3ReadRejectsUnknownStateFormRefBeforeHost(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}
	for name, value := range map[string]string{
		"name":                    "module-worker",
		"space":                   "prod",
		"uid":                     "uid-1",
		"generation":              "1",
		"revision":                "1",
		"form_api_version":        "other.example.com/v1alpha1",
		"form_kind":               "ModuleWorker",
		"form_definition_version": "0.1.0",
		"form_schema_digest":      "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatalf("seed state %s: %v", name, diags)
		}
	}
	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if !readResponse.Diagnostics.HasError() {
		t.Fatal("unknown state FormRef was accepted")
	}
	detail := readResponse.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "other.example.com/v1alpha1") {
		t.Fatalf("diagnostic does not name the state identity: %s", detail)
	}
	defaultRef, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, defaultRef.SchemaDigest) {
		t.Fatalf("diagnostic does not name the supported identity: %s", detail)
	}
	if host.getRequests != 0 {
		t.Fatalf("unsupported state FormRef reached the host with %d GETs", host.getRequests)
	}
}

// TestV3LaneCarriesNoUntypedFormRef proves the v1alpha3 lane exposes no
// resource that accepts an arbitrary FormRef. The withdrawn generic carrier
// (spec/decisions/0021) is the shape this asserts against: a resource whose
// Form identity is CONFIGURED rather than compiled in, which is exactly the
// shape that cannot verify the identity it is handed.
func TestV3LaneCarriesNoUntypedFormRef(t *testing.T) {
	ctx := context.Background()
	for _, factory := range newV3FormResources() {
		candidate := factory()
		var metadata frameworkresource.MetadataResponse
		candidate.Metadata(ctx, frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		if metadata.TypeName == "takoform_resource" {
			t.Fatal("the withdrawn generic takoform_resource carrier is registered again")
		}
		var schemaResponse frameworkresource.SchemaResponse
		candidate.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
		for _, configured := range []string{"spec_json", "form_api_version", "form_kind", "form_definition_version", "form_schema_digest"} {
			attribute, declared := schemaResponse.Schema.Attributes[configured]
			if !declared {
				continue
			}
			// The four form_* attributes exist on every typed resource, but only
			// as host-owned computed state recording which exact identity the
			// resource was applied under. An OPTIONAL or REQUIRED one would mean
			// the configuration names the Form, which is the withdrawn carrier.
			if attribute.IsRequired() || attribute.IsOptional() {
				t.Errorf("%s declares a configurable Form identity attribute %s", metadata.TypeName, configured)
			}
		}
	}
}
