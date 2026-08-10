package provider

// v3_artifact.go is the takoform_worker_bundle authoring path.
//
// The portable desired state of a WorkerBundle is exactly one field:
// manifestDigest, the immutable identity of a committed artifact manifest.
// The manifest — not the Form — describes the modules, so the bytes have one
// source of truth (spec/artifact-transport, spec/decisions/0012 and 0014).
//
// Local-file authoring survives as a PROVIDER-ONLY convenience: main_module
// plus modules[] name local files whose bytes are read and hashed here, built
// into the artifact manifest, and uploaded through the content-addressed
// artifact API before the resource is applied. What travels to the host is the
// digest the commit returned. Only paths, sizes, and digests enter state — raw
// module bytes never do.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// artifactManifestAPIVersion is the closed identity of the content-addressed
// artifact manifest (spec/schemas/artifact-manifest-v1alpha1.schema.json).
const artifactManifestAPIVersion = "artifacts.takoform.com/v1alpha1"

// workerBundleMediaTypes is the closed module media-type set of a WorkerBundle
// artifact manifest: every type the module graph may import, plus every type
// the bundle may merely carry. It is READ from the lane's single media-type
// statement, never spelled a second time, so the authoring surface can never
// admit a module the host would refuse or the runtime could not link
// (spec/decisions/0012 and 0019).
var workerBundleMediaTypes = model.BundleModuleMediaTypes()

// workerBundleMaxModuleBytes mirrors the artifact manifest's per-module size
// ceiling.
const workerBundleMaxModuleBytes = 268435456

// These are the portable minimum ceilings the v1beta1 Host Support Profile
// publishes for every artifact-backed Form.  A host may advertise a higher
// byte ceiling, but the provider always checks the exact profile at plan time
// before upload; the per-entry size remains bounded by the manifest schema.
const (
	// The public artifact-manifest schema intentionally keeps the WorkerBundle
	// module list at 4096. File-backed bundles have their independent
	// 16384-entry ceiling in v3_file_artifact.go.
	workerBundleMaxModules = 4096
)

// workerBundleAttributes is the authoring surface of takoform_worker_bundle.
// Every attribute requires replacement: a bundle is an immutable revision, and
// different bytes are a different manifest and therefore a different bundle.
func workerBundleAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"manifest_digest": schema.StringAttribute{
			Optional: true,
			Computed: true,
			Description: "Immutable digest of the committed artifact manifest this bundle is — the whole " +
				"portable desired state. Set it to reference a manifest already committed to the host, or " +
				"leave it unset and author main_module plus modules: the provider then commits the manifest " +
				"and records the digest the host returned.",
			Validators: []validator.String{StringMatches(
				model.PatternCanonicalSHA256,
				"manifest_digest must be a canonical lowercase sha256:<hex> digest",
			)},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"main_module": schema.StringAttribute{
			Optional: true,
			Description: "Local authoring only: relative path of the ES module the runtime instantiates first. " +
				"It must name one declared module. It is not portable desired state; it describes the artifact " +
				"manifest this provider commits.",
			Validators: []validator.String{StringMatches(
				model.PatternRelativePath,
				"main_module must be a non-escaping relative module path",
			)},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"modules": schema.ListNestedAttribute{
			Optional: true,
			Description: "Local authoring only: every module of the bundle. The provider reads each content_file, " +
				"computes its exact size and sha256 digest, commits the artifact manifest through the " +
				"content-addressed artifact API, and records the returned manifest_digest. File paths stay in " +
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
					// content_file is deliberately NOT Required: an imported bundle
					// carries no local path, and requiring one would make imported
					// state unrepresentable.
					"content_file": schema.StringAttribute{
						Optional:    true,
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
			Validators:    []validator.List{v3ListSizeValidator{minItems: 1, maxItems: workerBundleMaxModules}},
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

// v3BundleAuthoring is the resolved authoring intent of one worker bundle:
// which of the two modes was used, the manifest digest that identifies the
// bundle, and — in local mode — the manifest document and blobs that still
// have to be committed.
type v3BundleAuthoring struct {
	// Local reports local-file authoring; false means the configuration
	// referenced an already-committed manifest by digest.
	Local bool
	// Digest is the manifest digest the desired spec carries. In local mode it
	// is the RFC 8785 canonical digest computed from the manifest below, which
	// the host must return verbatim from the commit.
	Digest   string
	Manifest map[string]any
	Blobs    map[string][]byte
}

// Spec renders the complete portable desired state of a worker bundle.
func (authoring v3BundleAuthoring) Spec() map[string]any {
	return map[string]any{"manifestDigest": authoring.Digest}
}

// workerBundleAuthoring resolves the authoring mode of a worker bundle and
// everything derived from it. It performs no network traffic: local files are
// read and hashed, the artifact manifest is built, and its canonical digest is
// computed locally, so every mode error is raised before any host call. The
// resolved sizes and digests are written back onto values.Fields for the state
// projection, together with the resolved manifest digest.
func (r *v3FormResource) workerBundleAuthoring(values *v3Values) (v3BundleAuthoring, diag.Diagnostics) {
	var diags diag.Diagnostics
	written, writtenSet := v3PlanKnownString(values.Fields["manifest_digest"])
	mainModule, mainSet := v3PlanKnownString(values.Fields["main_module"])
	modulesValue, modulesList := values.Fields["modules"].(types.List)
	modulesSet := modulesList && !modulesValue.IsNull() && !modulesValue.IsUnknown()

	if !mainSet && !modulesSet {
		if !writtenSet {
			diags.AddAttributeError(path.Root("manifest_digest"), "No bundle authored",
				"A worker bundle is either referenced by manifest_digest or authored locally with "+
					"main_module plus modules. Declare one of the two.")
			return v3BundleAuthoring{}, diags
		}
		values.Fields["manifest_digest"] = types.StringValue(written)
		return v3BundleAuthoring{Digest: written}, diags
	}
	if !mainSet || !modulesSet {
		diags.AddAttributeError(path.Root("modules"), "Incomplete local bundle authoring",
			"Local authoring declares main_module and modules together; declare both, or reference a "+
				"committed manifest with manifest_digest alone.")
		return v3BundleAuthoring{}, diags
	}

	modules, moduleDiags := v3AuthoredBundleModules(modulesValue, mainModule)
	diags.Append(moduleDiags...)
	if diags.HasError() {
		return v3BundleAuthoring{}, diags
	}
	blobs := make(map[string][]byte, len(modules))
	for index := range modules {
		module := &modules[index]
		if module.ContentFile == "" {
			diags.AddAttributeError(path.Root("modules"), "Missing module content_file",
				fmt.Sprintf("module %q declares no content_file; local authoring reads each module from a local file.", module.Name))
			return v3BundleAuthoring{}, diags
		}
		if err := readWorkerBundleModule(module); err != nil {
			diags.AddAttributeError(path.Root("modules"), "Unreadable module file", err.Error())
			return v3BundleAuthoring{}, diags
		}
		blobs[module.Digest] = module.bytes
	}
	manifest := workerBundleManifest(mainModule, modules)
	digest, err := digestWorkerBundleManifest(manifest)
	if err != nil {
		diags.AddAttributeError(path.Root("modules"), "Unencodable artifact manifest", err.Error())
		return v3BundleAuthoring{}, diags
	}
	// Both spellings may be written together, but then they must agree: the
	// digest is the identity, so a written digest that does not name the bytes
	// on disk is a contradiction, not a preference.
	if writtenSet && written != digest {
		diags.AddAttributeError(path.Root("manifest_digest"), "Conflicting bundle authoring",
			fmt.Sprintf(
				"manifest_digest is %s but the authored modules commit manifest %s. "+
					"Write the digest of the authored bytes, or drop manifest_digest and let the provider record it.",
				written, digest,
			))
		return v3BundleAuthoring{}, diags
	}
	values.Fields["modules"] = v3WorkerBundleModulesValue(modules, &diags)
	values.Fields["manifest_digest"] = types.StringValue(digest)
	return v3BundleAuthoring{Local: true, Digest: digest, Manifest: manifest, Blobs: blobs}, diags
}

// v3AuthoredBundleModules reads the declared modules and proves the two rules
// a manifest must satisfy before it is built: module names are unique and
// main_module names one of them.
func v3AuthoredBundleModules(list types.List, mainModule string) ([]v3BundleModule, diag.Diagnostics) {
	var diags diag.Diagnostics
	elements := list.Elements()
	if len(elements) == 0 {
		diags.AddAttributeError(path.Root("modules"), "Empty bundle",
			"Local authoring requires at least one module.")
		return nil, diags
	}
	if len(elements) > workerBundleMaxModules {
		diags.AddAttributeError(path.Root("modules"), "Too many bundle modules",
			fmt.Sprintf("Local authoring declares %d modules; the portable ceiling is %d.", len(elements), workerBundleMaxModules))
		return nil, diags
	}
	modules := make([]v3BundleModule, 0, len(elements))
	names := map[string]struct{}{}
	for index, element := range elements {
		object, objectDiags := v3KnownObject("modules", index, element)
		diags.Append(objectDiags...)
		if objectDiags.HasError() {
			return nil, diags
		}
		attributes := object.Attributes()
		name, nameDiags := v3KnownString("modules", "name", attributes["name"])
		contentType, typeDiags := v3KnownString("modules", "content_type", attributes["content_type"])
		diags.Append(nameDiags...)
		diags.Append(typeDiags...)
		if diags.HasError() {
			return nil, diags
		}
		if _, duplicate := names[name]; duplicate {
			diags.AddAttributeError(path.Root("modules"), "Duplicate module name",
				fmt.Sprintf("module %q is declared more than once.", name))
			return nil, diags
		}
		names[name] = struct{}{}
		contentFile, _ := v3PlanKnownString(attributes["content_file"])
		modules = append(modules, v3BundleModule{Name: name, ContentType: contentType, ContentFile: contentFile})
	}
	if _, declared := names[mainModule]; !declared {
		diags.AddAttributeError(path.Root("main_module"), "Unknown main module",
			fmt.Sprintf("main_module %q does not name a declared module.", mainModule))
		return nil, diags
	}
	// An auxiliary module may sit in the bundle and may never be linked, so it
	// can never be the module the runtime instantiates first. Refusing it here
	// means the author sees it in the plan rather than at commit.
	for _, module := range modules {
		if module.Name != mainModule {
			continue
		}
		if !model.ModuleMediaTypeLoadable(module.ContentType) {
			diags.AddAttributeError(path.Root("main_module"), "Main module is not loadable",
				fmt.Sprintf(
					"main_module %q is declared as %s, which the module graph never imports. "+
						"main_module must name a module the runtime can instantiate: %s.",
					mainModule, module.ContentType, strings.Join(model.LoadableModuleMediaTypes(), ", "),
				))
			return nil, diags
		}
	}
	return modules, diags
}

// workerBundleManifest renders the artifact manifest document of one locally
// authored bundle. It is the exact document the upload commits, so its
// canonical digest is the bundle's identity.
func workerBundleManifest(mainModule string, modules []v3BundleModule) map[string]any {
	entries := make([]any, 0, len(modules))
	for _, module := range modules {
		entries = append(entries, map[string]any{
			"name":      module.Name,
			"mediaType": module.ContentType,
			"size":      module.Size,
			"digest":    module.Digest,
		})
	}
	return map[string]any{
		"apiVersion": artifactManifestAPIVersion,
		"kind":       workerBundleKind,
		"mainModule": mainModule,
		"modules":    entries,
	}
}

// digestWorkerBundleManifest computes the manifest's immutable identity with
// exactly the RFC 8785 canonicalization the host commits under, so the digest
// the plan shows is the digest the host will return.
func digestWorkerBundleManifest(manifest map[string]any) (string, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
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

// modifyWorkerBundlePlan implements plan-time byte identity for worker
// bundles. A bundle IS its manifest digest, so changed bytes at an UNCHANGED
// content_file path must surface as a real diff on manifest_digest. When prior
// state exists and the planned modules are wholly known, each file is re-read,
// the artifact manifest is rebuilt, and its canonical digest is written into
// the plan whenever the configuration did not pin one itself.
//
// Replacement then follows the DIGEST, not the authoring attributes: identical
// bytes authored a different way (a manifest_digest reference replaced by the
// local files that commit that exact manifest) are the same bundle, so the
// attribute-level replacement those authoring attributes carry is withdrawn.
// An unreadable content_file leaves the computed values unknown instead of
// erroring the plan.
func (r *v3FormResource) modifyWorkerBundlePlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy has no planned modules. A CREATE does, and it is resolved here
	// as well: the bundle's derived revision name follows its manifest digest,
	// so the digest has to be known in the plan for the name to be, and an
	// author reading `terraform plan` should see the identity the apply will
	// commit rather than "(known after apply)".
	if req.Plan.Raw.IsNull() {
		return
	}
	creating := req.State.Raw.IsNull()
	var planned types.List
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("modules"), &planned)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if planned.IsNull() || planned.IsUnknown() {
		// Manifest-digest authoring: there is no local file to re-read, and the
		// attribute-level replacement on manifest_digest already speaks for any
		// change.
		return
	}
	var plannedMain, plannedDigest, priorDigest types.String
	priorDigest = types.StringNull()
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("main_module"), &plannedMain)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("manifest_digest"), &plannedDigest)...)
	if !creating {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("manifest_digest"), &priorDigest)...)
	}
	// A configuration that pins manifest_digest owns that value. Terraform
	// always supplies the configuration here; an absent one is a direct-call
	// harness, where "nothing was configured" is the honest reading.
	configuredDigest := types.StringNull()
	if req.Config.Schema != nil && !req.Config.Raw.IsNull() {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("manifest_digest"), &configuredDigest)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	mainModule, mainKnown := v3PlanKnownString(plannedMain)
	if !mainKnown {
		return
	}
	modules, moduleDiags := v3AuthoredBundleModules(planned, mainModule)
	if moduleDiags.HasError() {
		// A malformed authoring block is reported by the apply-time resolver
		// against the same attribute paths; the plan stays silent rather than
		// duplicating the diagnostic.
		return
	}
	resolved := true
	for index := range modules {
		module := &modules[index]
		if module.ContentFile == "" || readWorkerBundleModule(module) != nil {
			resolved = false
		}
	}
	elementType := v3WorkerBundleModuleType()
	elements := make([]attr.Value, 0, len(modules))
	for _, module := range modules {
		size := types.Int64Unknown()
		digest := types.StringUnknown()
		if resolved {
			size = types.Int64Value(module.Size)
			digest = types.StringValue(module.Digest)
		}
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue(module.Name),
			"content_type": types.StringValue(module.ContentType),
			"content_file": v3OptionalStateString(module.ContentFile),
			"size":         size,
			"digest":       digest,
		}))
	}
	resolvedModules, listDiags := types.ListValue(elementType, elements)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	computed := types.StringUnknown()
	if resolved {
		digest, err := digestWorkerBundleManifest(workerBundleManifest(mainModule, modules))
		if err != nil {
			return
		}
		computed = types.StringValue(digest)
	}
	if !resolvedModules.Equal(planned) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("modules"), resolvedModules)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	// A pinned manifest_digest is never overwritten here; the computed digest is
	// checked against it at apply time, where a disagreement is a hard error
	// rather than a silent substitution.
	if configuredDigest.IsNull() && !computed.Equal(plannedDigest) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("manifest_digest"), computed)...)
		if resp.Diagnostics.HasError() {
			return
		}
		plannedDigest = computed
	}
	if creating {
		// A create has nothing to replace; the resolved digest is the whole point
		// of running here.
		return
	}
	if plannedDigest.Equal(priorDigest) {
		resp.RequiresReplace = v3WithoutPaths(resp.RequiresReplace,
			path.Root("main_module"), path.Root("modules"))
		return
	}
	// The attribute-level modifier already ran over the framework's proposed
	// plan, which still carried the prior byte identity; the refreshed identity
	// marks the replacement here.
	if !resp.RequiresReplace.Contains(path.Root("manifest_digest")) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("manifest_digest"))
	}
}

// v3WithoutPaths drops the named replacement triggers from a plan response.
func v3WithoutPaths(paths path.Paths, drop ...path.Path) path.Paths {
	kept := make(path.Paths, 0, len(paths))
	for _, candidate := range paths {
		remove := false
		for _, unwanted := range drop {
			if candidate.Equal(unwanted) {
				remove = true
			}
		}
		if !remove {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// v3PlanKnownString unwraps one wholly known planned string.
func v3PlanKnownString(value attr.Value) (string, bool) {
	text, ok := value.(types.String)
	if !ok || text.IsNull() || text.IsUnknown() {
		return "", false
	}
	return text.ValueString(), true
}

// uploadWorkerBundle commits the authored artifact manifest and returns the
// digest the host issued. That returned digest — never a locally asserted one
// — is what the WorkerBundle desired state carries.
func (r *v3FormResource) uploadWorkerBundle(
	ctx context.Context,
	authoring v3BundleAuthoring,
	diags *diag.Diagnostics,
) (string, bool) {
	committed, err := r.data.clientV3.UploadArtifact(ctx, authoring.Manifest, authoring.Blobs)
	if err != nil {
		diags.Append(v3HostCallDiagnostic("Worker bundle upload failed", err, v3Diagnostic{
			ResourceType: r.form.ResourceType,
			Pointer:      "/manifestDigest",
			Detail: fmt.Sprintf(
				"The content-addressed upload of artifact manifest %s (%d module blob(s)) did not commit, "+
					"so no %s desired state was sent. Nothing was mutated.",
				authoring.Digest, len(authoring.Blobs), r.form.Kind,
			),
		}))
		return "", false
	}
	return committed, true
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
			"content_file": v3OptionalStateString(module.ContentFile),
			"size":         types.Int64Value(module.Size),
			"digest":       types.StringValue(module.Digest),
		}))
	}
	list, listDiags := types.ListValue(elementType, elements)
	diags.Append(listDiags...)
	return list
}

// writeWorkerBundleState writes the bundle's state. manifest_digest is adopted
// from the host's desired spec whenever the host stated one — that is what
// makes an imported bundle manageable, because the wire carries the digest and
// nothing else. The local authoring attributes are preserved from the
// plan or prior state: they are local facts the wire cannot echo, and an
// imported bundle simply has none.
func (r *v3FormResource) writeWorkerBundleState(
	ctx context.Context,
	state *tfsdk.State,
	values v3Values,
	res *clientv3.Resource,
) diag.Diagnostics {
	var diags diag.Diagnostics
	digest := v3StateStringOf(values.Fields["manifest_digest"])
	if hosted, ok := res.Spec["manifestDigest"].(string); ok && hosted != "" {
		digest = types.StringValue(hosted)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("manifest_digest"), digest)...)
	diags.Append(state.SetAttribute(ctx, path.Root("main_module"), v3StateStringOf(values.Fields["main_module"]))...)
	modules, ok := values.Fields["modules"].(types.List)
	if !ok || modules.IsUnknown() {
		modules = types.ListNull(v3WorkerBundleModuleType())
	}
	diags.Append(state.SetAttribute(ctx, path.Root("modules"), modules)...)
	return diags
}

// v3StateStringOf renders one plan or state string as a storable value: an
// absent or still-unknown value becomes null rather than an illegal unknown in
// committed state.
func v3StateStringOf(value attr.Value) types.String {
	text, ok := value.(types.String)
	if !ok || text.IsUnknown() {
		return types.StringNull()
	}
	return text
}
