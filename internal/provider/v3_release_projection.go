package provider

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
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

// ProviderV3ReferenceSurface is the Provider-owned input for generated
// Terraform docs and examples. Portable Form authoring data and Terraform
// naming travel together here so documentation never carries another family
// roster or Group+Kind-to-resource-type map.
type ProviderV3ReferenceSurface struct {
	Form          model.Form
	ResourceType  string
	FormRef       ProviderV3ReleaseFormRef
	PackageDigest string
}

// CurrentProviderV3ReferenceSurfaces returns the current registered surfaces
// in Provider registration order from the same embedded projection used by
// runtime registration.
func CurrentProviderV3ReferenceSurfaces() ([]ProviderV3ReferenceSurface, error) {
	assembly, err := providerV3SnapshotAssembly()
	if err != nil {
		return nil, err
	}
	return referenceSurfacesFromAssembly(assembly)
}

func referenceSurfacesFromAssembly(assembly *v3ProviderAssembly) ([]ProviderV3ReferenceSurface, error) {
	if assembly == nil || assembly.projection == nil {
		return nil, fmt.Errorf("takoform provider: reference assembly is unavailable")
	}
	output := make([]ProviderV3ReferenceSurface, 0, len(assembly.projection.currentOrder))
	for _, key := range assembly.projection.currentOrder {
		entry := assembly.projection.forms[key]
		mapping, ok := assembly.projection.resources[key]
		if !ok || !mapping.Register || mapping.Ref != entry.Ref {
			return nil, fmt.Errorf("takoform provider: current reference Form %s has no exact registered Terraform mapping", key)
		}
		form, err := cloneProviderV3ReferenceForm(entry.Form)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: clone current reference Form %s: %w", key, err)
		}
		output = append(output, ProviderV3ReferenceSurface{
			Form: form, ResourceType: mapping.ResourceType,
			FormRef: ProviderV3ReleaseFormRef{
				APIVersion: entry.Ref.APIVersion, Kind: entry.Ref.Kind,
				DefinitionVersion: entry.Ref.DefinitionVersion, SchemaDigest: entry.Ref.SchemaDigest,
			},
			PackageDigest: entry.Ref.PackageDigest,
		})
	}
	return output, nil
}

func cloneProviderV3ReferenceForm(source model.Form) (model.Form, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return model.Form{}, err
	}
	var cloned model.Form
	if err := formpackage.DecodeStrictIJSON(raw, &cloned); err != nil {
		return model.Form{}, err
	}
	normalizeProjectedForm(&cloned)
	return cloned, nil
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
	seenResourceTypes := map[string]v3GroupKind{}
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
