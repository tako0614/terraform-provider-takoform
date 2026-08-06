package provider

// v3_artifact.go is the takoform_worker_bundle authoring path. The wire
// desired fields of a WorkerBundle are mainModule plus modules pinned by
// {name, mediaType, size, digest}; the provider authors them from local
// files: each module declares a content_file, whose bytes are hashed locally
// and uploaded through the content-addressed artifact API
// (spec/decisions/0012) before the WorkerBundle resource is applied. Only
// paths, sizes, and digests enter state — raw bytes never do.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// artifactManifestAPIVersion is the closed identity of the content-addressed
// artifact manifest (spec/schemas/artifact-manifest-v1alpha1.schema.json).
const artifactManifestAPIVersion = "artifacts.takoform.com/v1alpha1"

// workerBundleMediaTypes is the closed module media-type enum, exactly the
// catalog's WorkerBundle media_type field.
var workerBundleMediaTypes = []string{
	"application/javascript+module",
	"application/wasm",
	"text/plain",
	"application/octet-stream",
	"application/source-map+json",
}

// workerBundleMaxModuleBytes mirrors the schema's per-module size ceiling.
const workerBundleMaxModuleBytes = 268435456

// workerBundleAttributes is the authoring surface of takoform_worker_bundle.
// Every attribute requires replacement: a bundle is an immutable revision;
// different bytes are a different bundle.
func workerBundleAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"main_module": schema.StringAttribute{
			Required: true,
			Description: "Relative path of the ES module the runtime instantiates first. " +
				"It must name one declared module.",
			Validators: []validator.String{StringMatches(
				model.PatternRelativePath,
				"main_module must be a non-escaping relative module path",
			)},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"modules": schema.ListNestedAttribute{
			Required: true,
			Description: "Every module of the bundle. The provider reads each content_file locally, " +
				"computes its exact size and sha256 digest, uploads the bytes through the " +
				"content-addressed artifact API, and pins the module by digest. File paths stay in " +
				"state; file bytes never do.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required:    true,
						Description: "Relative module path inside the bundle.",
						Validators: []validator.String{StringMatches(
							model.PatternRelativePath,
							"module name must be a non-escaping relative module path",
						)},
					},
					"content_type": schema.StringAttribute{
						Required:    true,
						Description: "Closed media type deciding how the runtime links the module.",
						Validators:  []validator.String{StringOneOf(workerBundleMediaTypes...)},
					},
					"content_file": schema.StringAttribute{
						Required:    true,
						Description: "Local path of the module file whose bytes this bundle pins.",
					},
					"size": schema.Int64Attribute{
						Computed:    true,
						Description: "Exact module size in bytes, computed from content_file.",
					},
					"digest": schema.StringAttribute{
						Computed:    true,
						Description: "Canonical lowercase sha256 digest of the module bytes, computed from content_file.",
					},
				},
			},
			Validators:    []validator.List{v3ListSizeValidator{minItems: 1}},
			PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
		},
	}
}

func v3WorkerBundleModuleType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":         types.StringType,
		"content_type": types.StringType,
		"content_file": types.StringType,
		"size":         types.Int64Type,
		"digest":       types.StringType,
	}}
}

// v3BundleModule is one authored module resolved against its local file.
type v3BundleModule struct {
	Name        string
	ContentType string
	ContentFile string
	Size        int64
	Digest      string
	bytes       []byte
}

// workerBundleSpec resolves the planned bundle: it validates main_module
// against the declared module names, reads and hashes every content_file,
// and returns the wire spec {mainModule, modules} together with the resolved
// modules (whose bytes feed the upload). It performs no network traffic; the
// upload happens in uploadWorkerBundle. The resolved sizes and digests are
// written back onto values.Fields for the state projection.
func (r *v3FormResource) workerBundleSpec(_ context.Context, values *v3Values) (map[string]any, []v3BundleModule, diag.Diagnostics) {
	var diags diag.Diagnostics
	mainModule, mainDiags := v3KnownString("main_module", "value", values.Fields["main_module"])
	diags.Append(mainDiags...)
	modulesValue, ok := values.Fields["modules"].(types.List)
	if !ok || modulesValue.IsNull() || modulesValue.IsUnknown() {
		diags.AddAttributeError(path.Root("modules"), "Unknown modules",
			"modules must be wholly known before the provider can author this bundle.")
		return nil, nil, diags
	}
	if diags.HasError() {
		return nil, nil, diags
	}
	modules := make([]v3BundleModule, 0, len(modulesValue.Elements()))
	names := map[string]struct{}{}
	for index, element := range modulesValue.Elements() {
		object, objectDiags := v3KnownObject("modules", index, element)
		diags.Append(objectDiags...)
		if objectDiags.HasError() {
			return nil, nil, diags
		}
		attributes := object.Attributes()
		name, nameDiags := v3KnownString("modules", "name", attributes["name"])
		contentType, typeDiags := v3KnownString("modules", "content_type", attributes["content_type"])
		contentFile, fileDiags := v3KnownString("modules", "content_file", attributes["content_file"])
		diags.Append(nameDiags...)
		diags.Append(typeDiags...)
		diags.Append(fileDiags...)
		if diags.HasError() {
			return nil, nil, diags
		}
		if _, duplicate := names[name]; duplicate {
			diags.AddAttributeError(path.Root("modules"), "Duplicate module name",
				fmt.Sprintf("module %q is declared more than once.", name))
			return nil, nil, diags
		}
		names[name] = struct{}{}
		modules = append(modules, v3BundleModule{Name: name, ContentType: contentType, ContentFile: contentFile})
	}
	if _, declared := names[mainModule]; !declared {
		diags.AddAttributeError(path.Root("main_module"), "Unknown main module",
			fmt.Sprintf("main_module %q does not name a declared module.", mainModule))
		return nil, nil, diags
	}
	wireModules := make([]any, 0, len(modules))
	for index := range modules {
		module := &modules[index]
		if err := readWorkerBundleModule(module); err != nil {
			diags.AddAttributeError(path.Root("modules"), "Unreadable module file", err.Error())
			return nil, nil, diags
		}
		wireModules = append(wireModules, map[string]any{
			"name":      module.Name,
			"mediaType": module.ContentType,
			"size":      module.Size,
			"digest":    module.Digest,
		})
	}
	values.Fields["modules"] = v3WorkerBundleModulesValue(modules, &diags)
	return map[string]any{"mainModule": mainModule, "modules": wireModules}, modules, diags
}

// readWorkerBundleModule is the single read+hash authority for one module's
// content_file: plan-time byte identity (ModifyPlan) and the apply path both
// resolve Size, Digest, and bytes through it, so the two can never drift.
func readWorkerBundleModule(module *v3BundleModule) error {
	raw, err := os.ReadFile(module.ContentFile)
	if err != nil {
		return fmt.Errorf("module %q content_file: %v", module.Name, err)
	}
	if len(raw) > workerBundleMaxModuleBytes {
		return fmt.Errorf(
			"module %q is %d bytes; the portable ceiling is %d",
			module.Name, len(raw), workerBundleMaxModuleBytes,
		)
	}
	sum := sha256.Sum256(raw)
	module.Size = int64(len(raw))
	module.Digest = "sha256:" + hex.EncodeToString(sum[:])
	module.bytes = raw
	return nil
}

// ModifyPlan implements plan-time byte identity for worker bundles: a bundle
// is pinned by module bytes, so changed bytes at an UNCHANGED content_file
// path must surface as a real diff. When prior state exists and the planned
// modules list plus its content_file values are wholly known, each file is
// re-read and its exact size and sha256 digest are written into the planned
// modules, and any resulting difference forces replacement (a bundle is an
// immutable revision). An unreadable content_file leaves that module's
// planned size and digest unknown instead of erroring the plan.
func (r *v3FormResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.form.Kind != workerBundleKind {
		return
	}
	// A create resolves its bytes at apply time and a destroy has no planned
	// modules; only an existing resource can carry stale byte identity.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var planned types.List
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("modules"), &planned)...)
	if resp.Diagnostics.HasError() || planned.IsNull() || planned.IsUnknown() {
		return
	}
	modules := make([]v3BundleModule, 0, len(planned.Elements()))
	for _, element := range planned.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			return
		}
		attributes := object.Attributes()
		name, nameOK := v3PlanKnownString(attributes["name"])
		contentType, typeOK := v3PlanKnownString(attributes["content_type"])
		contentFile, fileOK := v3PlanKnownString(attributes["content_file"])
		if !nameOK || !typeOK || !fileOK {
			return
		}
		modules = append(modules, v3BundleModule{Name: name, ContentType: contentType, ContentFile: contentFile})
	}
	elementType := v3WorkerBundleModuleType()
	elements := make([]attr.Value, 0, len(modules))
	for index := range modules {
		module := &modules[index]
		size := types.Int64Unknown()
		digest := types.StringUnknown()
		if err := readWorkerBundleModule(module); err == nil {
			size = types.Int64Value(module.Size)
			digest = types.StringValue(module.Digest)
		}
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue(module.Name),
			"content_type": types.StringValue(module.ContentType),
			"content_file": types.StringValue(module.ContentFile),
			"size":         size,
			"digest":       digest,
		}))
	}
	resolved, listDiags := types.ListValue(elementType, elements)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() || resolved.Equal(planned) {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("modules"), resolved)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The attribute-level RequiresReplace modifiers already ran over the
	// framework's proposed plan, which still carried the prior byte identity;
	// the refreshed identity marks the replacement here.
	resp.RequiresReplace = append(resp.RequiresReplace, path.Root("modules"))
}

// v3PlanKnownString unwraps one wholly known planned string.
func v3PlanKnownString(value attr.Value) (string, bool) {
	text, ok := value.(types.String)
	if !ok || text.IsNull() || text.IsUnknown() {
		return "", false
	}
	return text.ValueString(), true
}

// uploadWorkerBundle uploads the resolved module blobs and commits the
// bundle's artifact manifest before the WorkerBundle resource is applied.
func (r *v3FormResource) uploadWorkerBundle(ctx context.Context, spec map[string]any, modules []v3BundleModule, diags *diag.Diagnostics) bool {
	manifestModules := make([]any, 0, len(modules))
	blobs := make(map[string][]byte, len(modules))
	for _, module := range modules {
		manifestModules = append(manifestModules, map[string]any{
			"name":      module.Name,
			"mediaType": module.ContentType,
			"size":      module.Size,
			"digest":    module.Digest,
		})
		blobs[module.Digest] = module.bytes
	}
	manifest := map[string]any{
		"apiVersion": artifactManifestAPIVersion,
		"kind":       workerBundleKind,
		"mainModule": spec["mainModule"],
		"modules":    manifestModules,
	}
	if _, err := r.data.clientV3.UploadArtifact(ctx, manifest, blobs); err != nil {
		diags.AddError("Worker bundle upload failed", err.Error())
		return false
	}
	return true
}

// v3WorkerBundleModulesValue renders the resolved modules — including the
// computed size and digest — as the typed state list.
func v3WorkerBundleModulesValue(modules []v3BundleModule, diags *diag.Diagnostics) types.List {
	elementType := v3WorkerBundleModuleType()
	elements := make([]attr.Value, 0, len(modules))
	for _, module := range modules {
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue(module.Name),
			"content_type": types.StringValue(module.ContentType),
			"content_file": types.StringValue(module.ContentFile),
			"size":         types.Int64Value(module.Size),
			"digest":       types.StringValue(module.Digest),
		}))
	}
	list, listDiags := types.ListValue(elementType, elements)
	diags.Append(listDiags...)
	return list
}

// writeWorkerBundleState preserves the authored bundle fields. Reads never
// rewrite them: a bundle is an immutable revision, and content_file paths are
// local authoring facts the wire cannot echo.
func (r *v3FormResource) writeWorkerBundleState(ctx context.Context, state *tfsdk.State, values v3Values) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("main_module"), values.Fields["main_module"])...)
	modules := values.Fields["modules"]
	if modules == nil {
		modules = types.ListNull(v3WorkerBundleModuleType())
	}
	diags.Append(state.SetAttribute(ctx, path.Root("modules"), modules)...)
	return diags
}
