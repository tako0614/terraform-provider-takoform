// Package vectorformcatalog declares the provider-neutral dense Vector Index
// Form Family. It contains only desired Form semantics and exact Interface
// references; provider resource mappings and backend behavior live elsewhere.
package vectorformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the versionless Vector Form Family group. Each Form carries its
// own independent definition SemVer and exact digest-bound identity.
var Family = model.Family{Group: "vector.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	firstHostAPI      = "forms.takoform.com/v1"

	metricCosine    = "cosine"
	metricEuclidean = "euclidean"
	metricDot       = "dotproduct"
)

// Forms is the complete Vector Family MVP set, in stable order.
// ResourceType is intentionally empty: a Terraform/OpenTofu provider owns
// that mapping and it is not part of Form identity or semantics.
var Forms = []model.Form{
	{
		Family: Family, Kind: "VectorIndex", Slug: "vector-index", Role: model.RoleIdentity,
		DefinitionVersion: definitionVersion, RequiresHostAPI: firstHostAPI,
		Title: "Vector Index", Description: "Fixed-dimension dense vector index with a creation-time " +
			"distance metric. The vector.index Interface fixes namespaced whole-record upsert, " +
			"read-after-write fetch, approximate top-k query, closed metadata filtering, and deletion; " +
			"this identity carries only the embedding dimension and metric.",
		Fields: []model.Field{
			{
				HCL: "dimension", Wire: "dimension", Kind: model.KindInteger,
				Required: true, Immutable: true, Min: model.I64(1), Max: model.I64(1536),
				Doc:     "Embedding vector dimension fixed for the lifetime of this index. Changing it replaces the index and its records.",
				Example: 1536, AltExample: 768, CounterExample: 0,
			},
			{
				HCL: "metric", Wire: "metric", Kind: model.KindStringEnum,
				Required: true, Immutable: true,
				Enum:    []string{metricCosine, metricEuclidean, metricDot},
				Doc:     "Distance metric fixed for the lifetime of this dense index. Changing it replaces the index and its records.",
				Example: metricCosine, AltExample: metricDot, CounterExample: "manhattan",
			},
		},
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: VectorIndexInterfaceName, Version: "1.0.0"}},
	},
}

// Validate proves the provider-neutral catalog is closed and every Form is
// internally coherent. Interface and Definition checks also run during
// RenderForms, but remain explicit here for callers that only inspect source.
func Validate() error {
	if err := model.ValidateNoOpenTokens(Forms); err != nil {
		return err
	}
	seenKinds, seenSlugs := map[string]bool{}, map[string]bool{}
	for _, form := range Forms {
		if err := form.Validate(); err != nil {
			return err
		}
		if form.Family != Family {
			return fmt.Errorf("form %s belongs to family %s, want %s", form.Kind, form.Family.APIVersion(), Family.APIVersion())
		}
		if seenKinds[form.Kind] || seenSlugs[form.Slug] {
			return fmt.Errorf("duplicate Vector family identity %s/%s", form.Kind, form.Slug)
		}
		seenKinds[form.Kind], seenSlugs[form.Slug] = true, true
		if form.DefinitionVersion != definitionVersion {
			return fmt.Errorf("form %s definition version %q, want %q", form.Kind, form.DefinitionVersion, definitionVersion)
		}
	}
	return ValidateInterfaceDefinitions(InterfaceDefinitions())
}

// ByKind returns one source Form by its exact portable kind.
func ByKind(kind string) (model.Form, bool) {
	for _, form := range Forms {
		if form.Kind == kind {
			return form, true
		}
	}
	return model.Form{}, false
}
