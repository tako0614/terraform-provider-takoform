package provider

// These helpers preserve the pre-W07 kind-oriented test vocabulary only for
// comparison vectors. Production artifact code requires an exact projection
// rule argument and has no hardcoded fallback.

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func legacyV3ArtifactProjectionForKind(formKind string) (*v3ArtifactProjection, bool) {
	switch formKind {
	case workerBundleKind:
		return &v3ArtifactProjection{
			Mode: v3ArtifactModeWorkerBundle, ManifestKind: workerBundleKind,
			MaximumFiles: workerBundleMaxModules, MaximumFileSize: workerBundleMaxModuleBytes,
			MediaTypes: append([]string(nil), workerBundleMediaTypes...),
		}, true
	case staticAssetBundleKind:
		return &v3ArtifactProjection{
			Mode: v3ArtifactModeFileBundle, ManifestKind: staticAssetBundleKind,
			MaximumFiles: artifactBundleMaxFiles, MaximumFileSize: artifactFileMaxBytes,
		}, true
	case sqliteMigrationSetKind:
		return &v3ArtifactProjection{
			Mode: v3ArtifactModeFileBundle, ManifestKind: migrationBundleManifestKind,
			MaximumFiles: artifactBundleMaxFiles, MaximumFileSize: artifactFileMaxBytes,
			MediaTypes: []string{"application/sql"},
		}, true
	default:
		return nil, false
	}
}

func v3FileBundleManifestKind(formKind string) (string, bool) {
	artifact, ok := legacyV3ArtifactProjectionForKind(formKind)
	if !ok || artifact.Mode != v3ArtifactModeFileBundle {
		return "", false
	}
	return artifact.ManifestKind, true
}

func v3ArtifactBackedRevision(formKind string) bool {
	_, ok := legacyV3ArtifactProjectionForKind(formKind)
	return ok
}

func workerBundleAttributes() map[string]schema.Attribute {
	artifact, _ := legacyV3ArtifactProjectionForKind(workerBundleKind)
	return workerBundleAttributesForProjection(*artifact)
}

func fileBundleAttributes(formKind string) map[string]schema.Attribute {
	artifact, _ := legacyV3ArtifactProjectionForKind(formKind)
	return fileBundleAttributesForProjection(*artifact)
}

func v3AuthoredBundleModules(list types.List, mainModule string) ([]v3BundleModule, diag.Diagnostics) {
	artifact, _ := legacyV3ArtifactProjectionForKind(workerBundleKind)
	return v3AuthoredBundleModulesForProjection(list, mainModule, *artifact)
}

func v3AuthoredArtifactFiles(list types.List, formKind string) ([]v3ArtifactFile, diag.Diagnostics) {
	artifact, _ := legacyV3ArtifactProjectionForKind(formKind)
	return v3AuthoredArtifactFilesForProjection(list, *artifact)
}

func workerBundleManifest(mainModule string, modules []v3BundleModule) map[string]any {
	artifact, _ := legacyV3ArtifactProjectionForKind(workerBundleKind)
	return workerBundleManifestForProjection(*artifact, mainModule, modules)
}
