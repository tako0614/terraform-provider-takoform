package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// publisherProviderFamilyGroup is the only Form publisher line selected by
	// the tako0614/takoform Provider after the Provider 3 aggregate. Other
	// OpenTofu providers remain ordinary peers in the module; this Provider does
	// not wrap or re-publish their resources as Takoform Forms.
	publisherProviderFamilyGroup = "edge.forms.takoform.com"
	publisherProviderFormCount   = 17
)

// currentPublisherProviderForms returns the exact publisher-selected Form set registered
// by the next Provider major. Provider 3's broader 31-Form projection remains
// immutable readable release history, but it is not the current registration
// authority.
func currentPublisherProviderForms() []model.Form {
	all := mustPublisherProviderSnapshotAssembly().currentForms
	selected := make([]model.Form, 0, len(all))
	for _, form := range all {
		if form.Family.Group == publisherProviderFamilyGroup {
			selected = append(selected, form)
		}
	}
	if len(selected) != publisherProviderFormCount {
		panic(fmt.Sprintf(
			"takoform provider: publisher-selected Form projection contains %d Forms, want %d",
			len(selected), publisherProviderFormCount,
		))
	}
	return selected
}

// newPublisherFormResources compiles only the publisher-selected Form projection. It
// deliberately reuses Provider 3's exact verified FormRef/codecs so the
// publisher-set cut does not mint or reinterpret a Form contract.
func newPublisherFormResources() []func() resource.Resource {
	assembly := mustPublisherProviderSnapshotAssembly()
	out, err := compileV3FormResources(
		currentPublisherProviderForms(), assembly.registry, assembly.resourceTypes, assembly.codecs,
	)
	if err != nil {
		panic(err)
	}
	return out
}

// CurrentPublisherProviderReferenceSurfaces returns the current documentation
// projection for the tako0614/takoform source address. It excludes the former
// Provider 3 aggregate Forms while leaving their immutable release projection
// available through CurrentProviderV3ReferenceSurfaces.
func CurrentPublisherProviderReferenceSurfaces() ([]ProviderV3ReferenceSurface, error) {
	assembly, err := publisherProviderSnapshotAssembly()
	if err != nil {
		return nil, err
	}
	selected, err := referenceSurfacesFromAssembly(assembly)
	if err != nil {
		return nil, err
	}
	if len(selected) != publisherProviderFormCount {
		return nil, fmt.Errorf(
			"takoform provider: publisher-selected reference projection contains %d Forms, want %d",
			len(selected), publisherProviderFormCount,
		)
	}
	return selected, nil
}
