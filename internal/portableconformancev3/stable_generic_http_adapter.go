package portableconformancev3

import (
	"errors"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// stableGenericHTTPArtifact translates the generic plan's opaque blob facts
// to one currently published HTTP artifact envelope. This concrete token stays
// at the adapter boundary; neither generic.json nor stable_generic.go knows it.
func stableGenericHTTPArtifact(input stableGenericArtifactTransport) (hostArtifactTransport, error) {
	if input.BlobSource == "" || input.DeclaredSize != len(input.BlobSource) || input.ContentType == "" {
		return hostArtifactTransport{}, errors.New("generic artifact blob evidence is incomplete")
	}
	digest := formpackage.DigestBytes([]byte(input.BlobSource))
	const manifestKind = "StaticAssetBundle"
	return hostArtifactTransport{
		BlobSource: input.BlobSource, ContentType: input.ContentType, ManifestKind: manifestKind,
		Manifest: map[string]any{
			"apiVersion": artifactAPIVersion,
			"kind":       manifestKind,
			"files": []any{map[string]any{
				"path": "opaque.bin", "mediaType": input.ContentType,
				"size": input.DeclaredSize, "digest": digest,
			}},
		},
	}, nil
}
