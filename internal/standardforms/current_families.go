package standardforms

import (
	"fmt"
	"strings"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

// currentFamilyInventory is a documentation view grouped from the Provider's
// exact runtime projection. It is not a family-selection authority.
type currentFamilyInventory struct {
	Group string
	Forms []model.Form
}

func currentFamilies() []currentFamilyInventory {
	surfaces, err := provider.CurrentProviderV3ReferenceSurfaces()
	if err != nil {
		panic(fmt.Errorf("load Provider 3 reference projection: %w", err))
	}
	output := make([]currentFamilyInventory, 0)
	positions := make(map[string]int)
	for _, surface := range surfaces {
		group := surface.Form.Family.APIVersion()
		position, exists := positions[group]
		if !exists {
			position = len(output)
			positions[group] = position
			output = append(output, currentFamilyInventory{Group: group})
		}
		output[position].Forms = append(output[position].Forms, surface.Form)
	}
	return output
}

func currentFormCount() int {
	total := 0
	for _, family := range currentFamilies() {
		total += len(family.Forms)
	}
	return total
}

func providerReferenceTerraformType(form model.Form) (string, error) {
	surfaces, err := provider.CurrentProviderV3ReferenceSurfaces()
	if err != nil {
		return "", err
	}
	for _, surface := range surfaces {
		if surface.Form.Family.APIVersion() == form.Family.APIVersion() && surface.Form.Kind == form.Kind &&
			surface.Form.DefinitionVersion == form.DefinitionVersion {
			return surface.ResourceType, nil
		}
	}
	return "", fmt.Errorf("official-provider reference surface has no Terraform mapping for %s/%s", form.Family.APIVersion(), form.Kind)
}

func mustProviderReferenceTerraformType(form model.Form) string {
	resourceType, err := providerReferenceTerraformType(form)
	if err != nil {
		panic(err)
	}
	return resourceType
}

func providerDocBasename(resourceType string) string {
	return strings.TrimPrefix(resourceType, "takoform_") + ".md"
}
