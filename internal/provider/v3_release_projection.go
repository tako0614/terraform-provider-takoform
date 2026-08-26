package provider

import (
	"fmt"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// ProviderV3ReleaseIdentityProjection is the provider-owned release view of
// the exact current Form projection. It deliberately carries Terraform names
// outside Form Definitions while binding each name to the complete FormRef and
// package provenance embedded by this provider build.
type ProviderV3ReleaseIdentityProjection struct {
	ProviderVersion    string                          `json:"providerVersion"`
	PortableAPIVersion string                          `json:"portableApiVersion"`
	Families           []string                        `json:"families"`
	FormMaturity       string                          `json:"formMaturity"`
	Forms              []ProviderV3ReleaseFormIdentity `json:"forms"`
}

// ProviderV3ReleaseFormIdentity binds one provider-owned Terraform resource
// type to the exact portable Form identity and package bytes it embeds.
type ProviderV3ReleaseFormIdentity struct {
	ResourceType  string                   `json:"resourceType"`
	FormRef       ProviderV3ReleaseFormRef `json:"formRef"`
	PackageDigest string                   `json:"packageDigest"`
}

// ProviderV3ReleaseFormRef is the exact four-field Form identity. Distribution
// provenance remains the sibling packageDigest and is not part of FormRef.
type ProviderV3ReleaseFormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

// CurrentProviderV3ReleaseIdentityProjection returns the deterministic 3.0.0 release
// projection directly from the same family declarations, exact registry, and
// Terraform mapping used by provider registration. Retained v1beta1 state
// codecs are intentionally excluded: they are readable compatibility state,
// not Provider 3 create/resource surface.
func CurrentProviderV3ReleaseIdentityProjection() (ProviderV3ReleaseIdentityProjection, error) {
	assembly, err := providerV3SnapshotAssembly()
	if err != nil {
		return ProviderV3ReleaseIdentityProjection{}, err
	}

	projection := ProviderV3ReleaseIdentityProjection{
		ProviderVersion:    "3.0.0",
		PortableAPIVersion: "forms.takoform.com/v1",
		Families:           make([]string, 0, 8),
		FormMaturity:       "experimental",
		Forms:              make([]ProviderV3ReleaseFormIdentity, 0, len(assembly.projection.currentOrder)),
	}
	seenFamilies := map[string]struct{}{}
	seenResourceTypes := map[string]currentformregistry.GroupKind{}
	for _, key := range assembly.projection.currentOrder {
		entry := assembly.projection.forms[key]
		mapping, ok := assembly.projection.resources[key]
		if !ok || !mapping.Register || mapping.Ref != entry.Ref {
			return ProviderV3ReleaseIdentityProjection{}, fmt.Errorf("takoform provider: current release Form %s has no exact registered Terraform mapping", key)
		}
		line := key.GroupKind()
		if _, seen := seenFamilies[line.APIVersion]; !seen {
			seenFamilies[line.APIVersion] = struct{}{}
			projection.Families = append(projection.Families, line.APIVersion)
		}
		if prior, duplicate := seenResourceTypes[mapping.ResourceType]; duplicate {
			return ProviderV3ReleaseIdentityProjection{}, fmt.Errorf("takoform provider: release resource type %q maps both %s/%s and %s/%s", mapping.ResourceType, prior.APIVersion, prior.Kind, line.APIVersion, line.Kind)
		}
		seenResourceTypes[mapping.ResourceType] = line
		projection.Forms = append(projection.Forms, ProviderV3ReleaseFormIdentity{
			ResourceType: mapping.ResourceType,
			FormRef: ProviderV3ReleaseFormRef{
				APIVersion: entry.Ref.APIVersion, Kind: entry.Ref.Kind,
				DefinitionVersion: entry.Ref.DefinitionVersion, SchemaDigest: entry.Ref.SchemaDigest,
			},
			PackageDigest: entry.Ref.PackageDigest,
		})
	}
	sort.Strings(projection.Families)
	sort.Slice(projection.Forms, func(i, j int) bool {
		return projection.Forms[i].ResourceType < projection.Forms[j].ResourceType
	})
	return projection, nil
}
