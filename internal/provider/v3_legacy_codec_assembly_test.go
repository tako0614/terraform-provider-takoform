package provider

// This test-only file retains the catalog-backed Provider 3 codec assembly strictly for
// W02-W07 comparison and synthetic compatibility tests. Production resources
// receive the projection-backed codec table from providerV3SnapshotAssembly.

import (
	"sync"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

var legacyV3Codecs = sync.OnceValue(func() *v3CodecTable {
	return newV3CodecTable(currentformregistry.V3Current())
})

func newV3CodecTable(registry v3FormRegistry) *v3CodecTable {
	table := &v3CodecTable{
		registry: registry,
		codecs:   map[currentformregistry.ExactFormKey]v3CodecDeclaration{},
	}
	generations := make([]catalogGeneration, 0, len(providerV3CurrentFamilies())+1)
	for _, family := range providerV3CurrentFamilies() {
		generation := catalogGeneration{family: family.family, forms: family.forms}
		generation.rendered, generation.renderErr = family.render()
		generations = append(generations, generation)
	}
	generations = append(generations, catalogGeneration{
		family: retainededgeformcatalog.Family,
		forms:  retainededgeformcatalog.Forms,
	})
	retainedRendered, retainedErr := retainededgeformcatalog.RenderForms()
	retainedGeneration := &generations[len(generations)-1]
	retainedGeneration.rendered = make([]catalogRenderedForm, 0, len(retainedRendered))
	for _, form := range retainedRendered {
		retainedGeneration.rendered = append(retainedGeneration.rendered, catalogRenderedForm{DefinitionJSON: form.DefinitionJSON})
	}
	retainedGeneration.renderErr = retainedErr
	for _, ref := range registry.SupportedRefs() {
		if ref.APIVersion == retainededgeformcatalog.Family.APIVersion() && ref.Kind == "ObjectBucket" {
			continue
		}
		declaration, declared := formForExactRef(ref, generations)
		if !declared {
			continue
		}
		table.codecs[ref.ExactKey()] = declaration
	}
	return table
}

type catalogGeneration struct {
	family    model.Family
	forms     []model.Form
	rendered  []catalogRenderedForm
	renderErr error
}

type catalogRenderedForm struct {
	DefinitionJSON string
}

func formForExactRef(
	ref currentformregistry.V3Ref,
	generations []catalogGeneration,
) (v3CodecDeclaration, bool) {
	for _, generation := range generations {
		if generation.family.APIVersion() != ref.APIVersion {
			continue
		}
		if generation.renderErr != nil || len(generation.rendered) != len(generation.forms) {
			return v3CodecDeclaration{}, false
		}
		for index, candidate := range generation.forms {
			if candidate.Kind != ref.Kind || candidate.DefinitionVersion != ref.DefinitionVersion {
				continue
			}
			definitionJSON := []byte(generation.rendered[index].DefinitionJSON)
			if generation.family.APIVersion() == retainededgeformcatalog.Family.APIVersion() {
				if frozen, ok := retainedFrozenDefinition(candidate.Kind); ok {
					definitionJSON = frozen
				}
			}
			digest, err := formpackage.DigestCanonicalJSON(definitionJSON)
			if err != nil || digest != ref.SchemaDigest {
				continue
			}
			definition, err := formpackage.ValidateDefinition(definitionJSON)
			if err != nil {
				return v3CodecDeclaration{}, false
			}
			return v3CodecDeclaration{Form: candidate, DesiredSchema: definition.DesiredSchema}, true
		}
	}
	return v3CodecDeclaration{}, false
}
