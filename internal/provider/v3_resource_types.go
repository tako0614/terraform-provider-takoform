package provider

import (
	"fmt"
	"regexp"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// v3ResourceTypeLine is provider-owned authoring metadata. A Form Definition
// does not name Terraform, and changing this mapping cannot change the Form's
// canonical bytes or schema digest. The provider expands each declared line to
// every exact FormRef it supports on that line, so runtime dispatch remains
// exact even while an additive Form evolution keeps one Terraform type.
type v3ResourceTypeLine struct {
	GroupKind    v3GroupKind
	ResourceType string
}

// v3ResourceTypeRegistry is the provider's exact FormRef -> Terraform resource
// type mapping. It is deliberately separate from currentformmodel.Form: an
// alternative client can consume the same Form declarations without carrying
// or agreeing with any Terraform name.
type v3ResourceTypeRegistry struct {
	byRef         map[v3ExactFormKey]string
	artifactByRef map[v3ExactFormKey]*v3ArtifactProjection
}

var terraformResourceTypePattern = regexp.MustCompile(`^takoform_[a-z0-9_]+$`)

func newV3ResourceTypeRegistry(
	forms v3FormRegistry,
	lines []v3ResourceTypeLine,
) (*v3ResourceTypeRegistry, error) {
	if forms == nil {
		return nil, fmt.Errorf("takoform provider: exact Form registry is nil")
	}
	registry := &v3ResourceTypeRegistry{
		byRef: map[v3ExactFormKey]string{}, artifactByRef: map[v3ExactFormKey]*v3ArtifactProjection{},
	}
	seenLines := map[v3GroupKind]struct{}{}
	typeKinds := map[string]string{}
	for _, line := range lines {
		if line.GroupKind.APIVersion == "" || line.GroupKind.Kind == "" {
			return nil, fmt.Errorf("takoform provider: Terraform resource type mapping has an incomplete Form line")
		}
		if !terraformResourceTypePattern.MatchString(line.ResourceType) {
			return nil, fmt.Errorf("takoform provider: resource type %q for %s/%s is invalid", line.ResourceType, line.GroupKind.APIVersion, line.GroupKind.Kind)
		}
		if _, duplicate := seenLines[line.GroupKind]; duplicate {
			return nil, fmt.Errorf("takoform provider: duplicate Terraform mapping for %s/%s", line.GroupKind.APIVersion, line.GroupKind.Kind)
		}
		seenLines[line.GroupKind] = struct{}{}
		if priorKind, taken := typeKinds[line.ResourceType]; taken && priorKind != line.GroupKind.Kind {
			return nil, fmt.Errorf("takoform provider: resource type %q maps both %s and %s", line.ResourceType, priorKind, line.GroupKind.Kind)
		}
		typeKinds[line.ResourceType] = line.GroupKind.Kind

		refs := forms.SupportedRefsFor(line.GroupKind)
		if len(refs) == 0 {
			return nil, fmt.Errorf("takoform provider: resource type %q maps a Form line with no supported exact FormRef: %s/%s", line.ResourceType, line.GroupKind.APIVersion, line.GroupKind.Kind)
		}
		for _, ref := range refs {
			key := ref.ExactKey()
			if _, duplicate := registry.byRef[key]; duplicate {
				return nil, fmt.Errorf("takoform provider: duplicate Terraform mapping for exact FormRef %s", key)
			}
			registry.byRef[key] = line.ResourceType
		}
	}
	return registry, nil
}

func (r *v3ResourceTypeRegistry) Lookup(key v3ExactFormKey) (string, bool) {
	if r == nil {
		return "", false
	}
	resourceType, ok := r.byRef[key]
	return resourceType, ok
}

func (r *v3ResourceTypeRegistry) Artifact(key v3ExactFormKey) (*v3ArtifactProjection, bool) {
	if r == nil {
		return nil, false
	}
	artifact, ok := r.artifactByRef[key]
	return cloneV3ArtifactProjection(artifact), ok
}

// compileV3FormResources is provider registration. Missing or duplicate
// Terraform mappings fail here only; Form validation and canonical rendering
// remain completely independent of the official provider.
func compileV3FormResources(
	forms []model.Form,
	formRegistry v3FormRegistry,
	resourceTypes *v3ResourceTypeRegistry,
	codecs *v3CodecTable,
) ([]func() frameworkresource.Resource, error) {
	if formRegistry == nil || resourceTypes == nil || codecs == nil {
		return nil, fmt.Errorf("takoform provider: resource registration dependency is nil")
	}
	factories := make([]func() frameworkresource.Resource, 0, len(forms))
	registeredTypes := map[string]v3GroupKind{}
	for _, form := range forms {
		line := v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		ref, err := formRegistry.DefaultCreate(line)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: registering %s: %w", form.Kind, err)
		}
		resourceType, mapped := resourceTypes.Lookup(ref.ExactKey())
		if !mapped {
			return nil, fmt.Errorf("takoform provider: registering %s requires an exact Terraform mapping for %s", form.Kind, ref.ExactKey())
		}
		if prior, duplicate := registeredTypes[resourceType]; duplicate {
			return nil, fmt.Errorf("takoform provider: resource type %q is registered for both %s/%s and %s/%s", resourceType, prior.APIVersion, prior.Kind, line.APIVersion, line.Kind)
		}
		registeredTypes[resourceType] = line
		declared := form
		typeName := resourceType
		artifact, _ := resourceTypes.Artifact(ref.ExactKey())
		factories = append(factories, func() frameworkresource.Resource {
			return &v3FormResource{form: declared, resourceType: typeName, codecs: codecs, artifact: cloneV3ArtifactProjection(artifact)}
		})
	}
	return factories, nil
}
