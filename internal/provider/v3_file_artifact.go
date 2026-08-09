package provider

// v3_file_artifact.go is the provider-only local-file authoring path shared by
// StaticAssetBundle and SQLiteMigrationSet. Their portable desired state is
// still exactly {manifestDigest}: paths, media types, sizes, and digests live
// in the content-addressed artifact manifest, while raw file bytes travel only
// through the artifact upload API and never enter Terraform state.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

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

const (
	migrationBundleManifestKind = "MigrationBundle"
	artifactFileMaxBytes        = 268435456
	artifactPathMaxBytes        = 240
	artifactMediaTypeMaxBytes   = 255
	artifactBundleMaxFiles      = 16384
)

// v3FileBundleManifestKind maps a Form identity to the artifact-manifest kind
// it admits. SQLiteMigrationSet intentionally commits a MigrationBundle: the
// Form names an immutable desired-state revision, while the manifest kind
// names the portable artifact representation shared with the host.
func v3FileBundleManifestKind(formKind string) (string, bool) {
	switch formKind {
	case staticAssetBundleKind:
		return staticAssetBundleKind, true
	case sqliteMigrationSetKind:
		return migrationBundleManifestKind, true
	default:
		return "", false
	}
}

func v3ArtifactBackedRevision(formKind string) bool {
	if formKind == workerBundleKind {
		return true
	}
	_, ok := v3FileBundleManifestKind(formKind)
	return ok
}

func fileBundleAttributes(formKind string) map[string]schema.Attribute {
	mediaValidators := []validator.String{StringMatches(
		`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`,
		"media_type must be a normalized v1alpha1 type/subtype token without parameters",
	)}
	if formKind == sqliteMigrationSetKind {
		mediaValidators = []validator.String{StringOneOf("application/sql")}
	}
	return map[string]schema.Attribute{
		"manifest_digest": schema.StringAttribute{
			Optional: true,
			Computed: true,
			Description: "Immutable digest of the committed artifact manifest this revision is. Set it to " +
				"reference a manifest already committed to the host, or leave it unset and author files: " +
				"the provider commits the manifest and records the digest returned by the host.",
			Validators: []validator.String{StringMatches(
				"^sha256:[0-9a-f]{64}$",
				"manifest_digest must be a canonical lowercase sha256:<hex> digest",
			)},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"files": schema.ListNestedAttribute{
			Optional: true,
			Description: "Local authoring only: ordered files of the bundle. The provider reads each content_file, " +
				"computes its exact size and sha256 digest, commits the artifact manifest, and records the returned " +
				"manifest_digest. Local paths and computed evidence stay in state; file bytes never do.",
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"path": schema.StringAttribute{
					Required:    true,
					Description: "Relative slash-separated path inside the artifact bundle.",
					Validators: []validator.String{StringMatches(
						"^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$",
						"path must be a non-escaping relative artifact path",
					)},
				},
				"media_type": schema.StringAttribute{
					Required:    true,
					Description: "Portable media type recorded in the artifact manifest.",
					Validators:  mediaValidators,
				},
				"content_file": schema.StringAttribute{
					Optional:    true,
					Description: "Local path of the file whose bytes this manifest entry pins.",
				},
				"size": schema.Int64Attribute{
					Computed:    true,
					Description: "Exact file size in bytes, computed from content_file.",
				},
				"digest": schema.StringAttribute{
					Computed:    true,
					Description: "Canonical lowercase sha256 digest of the file bytes, computed from content_file.",
				},
			}},
			Validators: []validator.List{v3ListSizeValidator{minItems: 1, maxItems: artifactBundleMaxFiles}},
			PlanModifiers: []planmodifier.List{
				listplanmodifier.RequiresReplace(),
			},
		},
	}
}

func v3ArtifactFileType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"path":         types.StringType,
		"media_type":   types.StringType,
		"content_file": types.StringType,
		"size":         types.Int64Type,
		"digest":       types.StringType,
	}}
}

type v3ArtifactFile struct {
	Path        string
	MediaType   string
	ContentFile string
	Size        int64
	Digest      string
	bytes       []byte
}

type v3FileBundleAuthoring struct {
	Local    bool
	Digest   string
	Manifest map[string]any
	Blobs    map[string][]byte
}

func (authoring v3FileBundleAuthoring) Spec() map[string]any {
	return map[string]any{"manifestDigest": authoring.Digest}
}

func (r *v3FormResource) fileBundleAuthoring(values *v3Values) (v3FileBundleAuthoring, diag.Diagnostics) {
	var diags diag.Diagnostics
	manifestKind, supported := v3FileBundleManifestKind(r.form.Kind)
	if !supported {
		diags.AddError("Unsupported file artifact Form", "The provider has no file-manifest mapping for "+r.form.Kind+".")
		return v3FileBundleAuthoring{}, diags
	}
	written, writtenSet := v3PlanKnownString(values.Fields["manifest_digest"])
	filesValue, isList := values.Fields["files"].(types.List)
	filesSet := isList && !filesValue.IsNull() && !filesValue.IsUnknown()
	if !filesSet {
		if !writtenSet {
			diags.AddAttributeError(path.Root("manifest_digest"), "No artifact authored",
				"This revision is either referenced by manifest_digest or authored locally with files. Declare one of the two.")
			return v3FileBundleAuthoring{}, diags
		}
		values.Fields["manifest_digest"] = types.StringValue(written)
		return v3FileBundleAuthoring{Digest: written}, diags
	}

	files, fileDiags := v3AuthoredArtifactFiles(filesValue, r.form.Kind)
	diags.Append(fileDiags...)
	if diags.HasError() {
		return v3FileBundleAuthoring{}, diags
	}
	blobs := make(map[string][]byte, len(files))
	for index := range files {
		file := &files[index]
		if file.ContentFile == "" {
			diags.AddAttributeError(path.Root("files"), "Missing file content_file",
				fmt.Sprintf("file %q declares no content_file; local authoring reads every entry from a local file.", file.Path))
			return v3FileBundleAuthoring{}, diags
		}
		if err := readArtifactFile(file); err != nil {
			diags.AddAttributeError(path.Root("files"), "Unreadable artifact file", err.Error())
			return v3FileBundleAuthoring{}, diags
		}
		blobs[file.Digest] = file.bytes
	}
	manifest := fileBundleManifest(manifestKind, files)
	digest, err := digestArtifactManifest(manifest)
	if err != nil {
		diags.AddAttributeError(path.Root("files"), "Unencodable artifact manifest", err.Error())
		return v3FileBundleAuthoring{}, diags
	}
	if writtenSet && written != digest {
		diags.AddAttributeError(path.Root("manifest_digest"), "Conflicting artifact authoring",
			fmt.Sprintf("manifest_digest is %s but the authored files commit manifest %s. Write the digest of the authored bytes, or omit manifest_digest.", written, digest))
		return v3FileBundleAuthoring{}, diags
	}
	values.Fields["files"] = v3ArtifactFilesValue(files, &diags)
	values.Fields["manifest_digest"] = types.StringValue(digest)
	return v3FileBundleAuthoring{Local: true, Digest: digest, Manifest: manifest, Blobs: blobs}, diags
}

func v3AuthoredArtifactFiles(list types.List, formKind string) ([]v3ArtifactFile, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(list.Elements()) == 0 {
		diags.AddAttributeError(path.Root("files"), "Empty artifact", "Local authoring requires at least one file.")
		return nil, diags
	}
	files := make([]v3ArtifactFile, 0, len(list.Elements()))
	paths := map[string]struct{}{}
	for index, element := range list.Elements() {
		object, objectDiags := v3KnownObject("files", index, element)
		diags.Append(objectDiags...)
		if objectDiags.HasError() {
			return nil, diags
		}
		attributes := object.Attributes()
		filePath, pathDiags := v3KnownString("files", "path", attributes["path"])
		mediaType, mediaDiags := v3KnownString("files", "media_type", attributes["media_type"])
		diags.Append(pathDiags...)
		diags.Append(mediaDiags...)
		if diags.HasError() {
			return nil, diags
		}
		if !artifactPathPattern.MatchString(filePath) || len(filePath) > artifactPathMaxBytes {
			diags.AddAttributeError(path.Root("files"), "Invalid artifact path", fmt.Sprintf("file %q is not a non-escaping relative artifact path.", filePath))
			return nil, diags
		}
		if _, duplicate := paths[filePath]; duplicate {
			diags.AddAttributeError(path.Root("files"), "Duplicate artifact path", fmt.Sprintf("file path %q is declared more than once.", filePath))
			return nil, diags
		}
		paths[filePath] = struct{}{}
		if !model.ValidNormalizedMediaType(mediaType) || len(mediaType) > artifactMediaTypeMaxBytes {
			diags.AddAttributeError(path.Root("files"), "Invalid artifact media type", fmt.Sprintf("file %q has invalid media type %q.", filePath, mediaType))
			return nil, diags
		}
		if formKind == sqliteMigrationSetKind && mediaType != "application/sql" {
			diags.AddAttributeError(path.Root("files"), "Migration file is not SQL", fmt.Sprintf("file %q uses %q; every SQLiteMigrationSet entry must use application/sql.", filePath, mediaType))
			return nil, diags
		}
		contentFile, _ := v3PlanKnownString(attributes["content_file"])
		files = append(files, v3ArtifactFile{Path: filePath, MediaType: mediaType, ContentFile: contentFile})
	}
	return files, diags
}

var artifactPathPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`)

func readArtifactFile(file *v3ArtifactFile) error {
	raw, err := os.ReadFile(file.ContentFile)
	if err != nil {
		return fmt.Errorf("file %q content_file: %v", file.Path, err)
	}
	if len(raw) > artifactFileMaxBytes {
		return fmt.Errorf("file %q is %d bytes; the portable ceiling is %d", file.Path, len(raw), artifactFileMaxBytes)
	}
	sum := sha256.Sum256(raw)
	file.Size = int64(len(raw))
	file.Digest = "sha256:" + hex.EncodeToString(sum[:])
	file.bytes = raw
	return nil
}

func fileBundleManifest(kind string, files []v3ArtifactFile) map[string]any {
	entries := make([]any, 0, len(files))
	for _, file := range files {
		entries = append(entries, map[string]any{
			"path": file.Path, "mediaType": file.MediaType, "size": file.Size, "digest": file.Digest,
		})
	}
	return map[string]any{"apiVersion": artifactManifestAPIVersion, "kind": kind, "files": entries}
}

func digestArtifactManifest(manifest map[string]any) (string, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

func v3ArtifactFilesValue(files []v3ArtifactFile, diags *diag.Diagnostics) types.List {
	elementType := v3ArtifactFileType()
	elements := make([]attr.Value, 0, len(files))
	for _, file := range files {
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"path":         types.StringValue(file.Path),
			"media_type":   types.StringValue(file.MediaType),
			"content_file": v3OptionalStateString(file.ContentFile),
			"size":         types.Int64Value(file.Size),
			"digest":       types.StringValue(file.Digest),
		}))
	}
	list, listDiags := types.ListValue(elementType, elements)
	diags.Append(listDiags...)
	return list
}

func (r *v3FormResource) modifyFileBundlePlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	creating := req.State.Raw.IsNull()
	var planned types.List
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("files"), &planned)...)
	if resp.Diagnostics.HasError() || planned.IsNull() || planned.IsUnknown() {
		return
	}
	var plannedDigest, priorDigest types.String
	priorDigest = types.StringNull()
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("manifest_digest"), &plannedDigest)...)
	if !creating {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("manifest_digest"), &priorDigest)...)
	}
	configuredDigest := types.StringNull()
	if req.Config.Schema != nil && !req.Config.Raw.IsNull() {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("manifest_digest"), &configuredDigest)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	files, fileDiags := v3AuthoredArtifactFiles(planned, r.form.Kind)
	if fileDiags.HasError() {
		return
	}
	resolved := true
	for index := range files {
		if files[index].ContentFile == "" || readArtifactFile(&files[index]) != nil {
			resolved = false
		}
	}
	resolvedFiles := v3ArtifactFilesPlanValue(files, resolved, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	computed := types.StringUnknown()
	if resolved {
		manifestKind, _ := v3FileBundleManifestKind(r.form.Kind)
		digest, err := digestArtifactManifest(fileBundleManifest(manifestKind, files))
		if err != nil {
			return
		}
		computed = types.StringValue(digest)
	}
	if !resolvedFiles.Equal(planned) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("files"), resolvedFiles)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if configuredDigest.IsNull() && !computed.Equal(plannedDigest) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("manifest_digest"), computed)...)
		if resp.Diagnostics.HasError() {
			return
		}
		plannedDigest = computed
	}
	if creating {
		return
	}
	if plannedDigest.Equal(priorDigest) {
		resp.RequiresReplace = v3WithoutPaths(resp.RequiresReplace, path.Root("files"))
		return
	}
	if !resp.RequiresReplace.Contains(path.Root("manifest_digest")) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("manifest_digest"))
	}
}

func v3ArtifactFilesPlanValue(files []v3ArtifactFile, resolved bool, diags *diag.Diagnostics) types.List {
	elementType := v3ArtifactFileType()
	elements := make([]attr.Value, 0, len(files))
	for _, file := range files {
		size := types.Int64Unknown()
		digest := types.StringUnknown()
		if resolved {
			size = types.Int64Value(file.Size)
			digest = types.StringValue(file.Digest)
		}
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"path":         types.StringValue(file.Path),
			"media_type":   types.StringValue(file.MediaType),
			"content_file": v3OptionalStateString(file.ContentFile),
			"size":         size,
			"digest":       digest,
		}))
	}
	list, listDiags := types.ListValue(elementType, elements)
	diags.Append(listDiags...)
	return list
}

func (r *v3FormResource) uploadFileBundle(ctx context.Context, authoring v3FileBundleAuthoring, diags *diag.Diagnostics) (string, bool) {
	committed, err := r.data.clientV3.UploadArtifact(ctx, authoring.Manifest, authoring.Blobs)
	if err != nil {
		diags.Append(v3HostCallDiagnostic(r.form.Kind+" artifact upload failed", err, v3Diagnostic{
			ResourceType: r.form.ResourceType,
			Pointer:      "/manifestDigest",
			Detail: fmt.Sprintf(
				"The content-addressed upload of artifact manifest %s (%d file blob(s)) did not commit, so no %s desired state was sent. Nothing was mutated.",
				authoring.Digest, len(authoring.Blobs), r.form.Kind,
			),
		}))
		return "", false
	}
	return committed, true
}

func (r *v3FormResource) writeFileBundleState(ctx context.Context, state *tfsdk.State, values v3Values, res *clientv3.Resource) diag.Diagnostics {
	var diags diag.Diagnostics
	digest := v3StateStringOf(values.Fields["manifest_digest"])
	if hosted, ok := res.Spec["manifestDigest"].(string); ok && hosted != "" {
		digest = types.StringValue(hosted)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("manifest_digest"), digest)...)
	files, ok := values.Fields["files"].(types.List)
	if !ok || files.IsUnknown() {
		files = types.ListNull(v3ArtifactFileType())
	}
	diags.Append(state.SetAttribute(ctx, path.Root("files"), files)...)
	return diags
}
