package tableformcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// RenderedContract is one Interface Definition rendered to publishable bytes
// and its RFC 8785 digest.
type RenderedContract struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	DefinitionJSON string `json:"definitionJson"`
	SchemaDigest   string `json:"schemaDigest"`
}

// RenderedForm is one Form Definition rendered with its canonical and
// negative fixtures. Provider resource mappings are intentionally absent from
// this provider-neutral result.
type RenderedForm struct {
	Kind           string                     `json:"kind"`
	Slug           string                     `json:"slug"`
	Role           string                     `json:"role"`
	Definition     formpackage.FormDefinition `json:"definition"`
	DefinitionJSON string                     `json:"definitionJson"`
	Fixtures       map[string]map[string]any  `json:"fixtures"`
}

func marshalIndented(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func renderContract(name, version string, definition any) (RenderedContract, error) {
	text, err := marshalIndented(definition)
	if err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	digest, err := formpackage.DigestCanonicalJSON([]byte(text))
	if err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	return RenderedContract{Name: name, Version: version, DefinitionJSON: text, SchemaDigest: digest}, nil
}

func renderInterfaceContract(name, version string, definition any) (RenderedContract, error) {
	rendered, err := renderContract(name, version, definition)
	if err != nil {
		return RenderedContract{}, err
	}
	if err := formpackage.ValidateInterfaceDefinition([]byte(rendered.DefinitionJSON)); err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	return rendered, nil
}

// RenderInterfaces renders every Interface Definition in stable catalog order.
func RenderInterfaces() ([]RenderedContract, error) {
	if err := ValidateInterfaceDefinitions(InterfaceDefinitions()); err != nil {
		return nil, err
	}
	out := make([]RenderedContract, 0, len(InterfaceDefinitions()))
	for _, definition := range InterfaceDefinitions() {
		rendered, err := renderInterfaceContract(definition.Name, definition.Version, definition)
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}
	return out, nil
}

// InterfaceRefFor resolves one exact digest-bound InterfaceRef.
func InterfaceRefFor(name, version string) (formpackage.InterfaceRef, error) {
	definition, err := interfaceDefinitionByName(name)
	if err != nil {
		return formpackage.InterfaceRef{}, err
	}
	if definition.Version != version {
		return formpackage.InterfaceRef{}, fmt.Errorf("interface %s is version %s, not %s", name, definition.Version, version)
	}
	rendered, err := renderInterfaceContract(name, version, definition)
	if err != nil {
		return formpackage.InterfaceRef{}, err
	}
	return formpackage.InterfaceRef{
		APIVersion: InterfaceAPIVersion, Name: name, Version: version, SchemaDigest: rendered.SchemaDigest,
	}, nil
}

// targetContractResolver adapts this family to the aggregate resolver seam.
// Table has no cross-resource fields today, but keeping the exact resolver
// here makes the render path ready for future provider-neutral relations
// without a second digest or lookup implementation.
type targetContractResolver struct {
	rendered            map[string]RenderedForm
	refs                map[string]model.TargetFormRef
	relations           map[string][]model.Relation
	inProgress          map[string]bool
	relationsInProgress map[string]bool
	aggregate           *currentformregistry.TargetResolver
}

func newTargetContractResolver() *targetContractResolver {
	resolver := &targetContractResolver{
		rendered:            map[string]RenderedForm{},
		refs:                map[string]model.TargetFormRef{},
		relations:           map[string][]model.Relation{},
		inProgress:          map[string]bool{},
		relationsInProgress: map[string]bool{},
	}
	aggregate, err := currentformregistry.NewTargetResolver(resolver, resolver)
	if err != nil {
		panic(fmt.Sprintf("constructing Table Form target resolver: %v", err))
	}
	resolver.aggregate = aggregate
	return resolver
}

func (*targetContractResolver) FamilyAPIVersion() string { return Family.APIVersion() }

func (*targetContractResolver) ResourceNamePattern() string { return model.PatternResourceName }

func (r *targetContractResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	return r.aggregate.ResolveResourceTarget(target)
}

func (r *targetContractResolver) TargetFormRefs(targetKind string) ([]model.TargetFormRef, error) {
	if ref, known := r.refs[targetKind]; known {
		return []model.TargetFormRef{ref}, nil
	}
	if r.inProgress[targetKind] {
		return nil, fmt.Errorf("exact-Form target cycle through %s", targetKind)
	}
	form, known := ByKind(targetKind)
	if !known {
		return nil, fmt.Errorf("exact-Form target %q is not a Table catalog Form", targetKind)
	}
	r.inProgress[targetKind] = true
	rendered, err := r.renderedForm(form)
	delete(r.inProgress, targetKind)
	if err != nil {
		return nil, err
	}
	digest, err := formpackage.DigestCanonicalJSON([]byte(rendered.DefinitionJSON))
	if err != nil {
		return nil, err
	}
	ref := model.TargetFormRef{
		APIVersion: Family.APIVersion(), Kind: targetKind,
		DefinitionVersion: form.DefinitionVersion, SchemaDigest: digest,
	}
	r.refs[targetKind] = ref
	return []model.TargetFormRef{ref}, nil
}

func (r *targetContractResolver) ResolveExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	return r.aggregate.ResolveExactFormRelations(ref)
}

func (r *targetContractResolver) ExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	key := ref.String()
	if cached, ok := r.relations[key]; ok {
		return append([]model.Relation(nil), cached...), nil
	}
	if r.relationsInProgress[key] {
		return nil, fmt.Errorf("exact-Form relation cycle through %s", key)
	}
	if ref.APIVersion != Family.APIVersion() {
		return nil, fmt.Errorf("exact target Form %s is outside Table catalog group %q", key, Family.APIVersion())
	}
	form, known := ByKind(ref.Kind)
	if !known || form.DefinitionVersion != ref.DefinitionVersion {
		return nil, fmt.Errorf("exact target Form %s is not in this Table catalog", key)
	}
	refs, err := r.TargetFormRefs(ref.Kind)
	if err != nil {
		return nil, err
	}
	if len(refs) != 1 || refs[0] != ref {
		return nil, fmt.Errorf("exact target Form %s does not match rendered Table identity", key)
	}
	r.relationsInProgress[key] = true
	schema, err := form.DesiredSchema(r.aggregate)
	if err == nil {
		var relations []model.Relation
		relations, err = model.DeriveRelationsWithConstraints(schema, form.Constraints())
		if err == nil {
			r.relations[key] = append([]model.Relation(nil), relations...)
		}
	}
	delete(r.relationsInProgress, key)
	if err != nil {
		return nil, err
	}
	return append([]model.Relation(nil), r.relations[key]...), nil
}

func (r *targetContractResolver) RequiredInterface(name, version string) (model.RequiredInterface, error) {
	ref, err := InterfaceRefFor(name, version)
	if err != nil {
		return model.RequiredInterface{}, err
	}
	return model.RequiredInterface{
		APIVersion: ref.APIVersion, Name: ref.Name, Version: ref.Version, SchemaDigest: ref.SchemaDigest,
	}, nil
}

func (r *targetContractResolver) renderedForm(form model.Form) (RenderedForm, error) {
	if cached, known := r.rendered[form.Kind]; known {
		return cached, nil
	}
	rendered, err := renderForm(form, r.aggregate)
	if err != nil {
		return RenderedForm{}, fmt.Errorf("%s: %w", form.Kind, err)
	}
	r.rendered[form.Kind] = rendered
	return rendered, nil
}

func renderConstraints(form model.Form) []formpackage.FormConstraint {
	declared := form.Constraints()
	if len(declared) == 0 {
		return nil
	}
	out := make([]formpackage.FormConstraint, 0, len(declared))
	for _, entry := range declared {
		out = append(out, formpackage.FormConstraint{
			Kind: string(entry.Kind), Reference: entry.Reference, KeyedBy: entry.KeyedBy,
			List: entry.List, Member: entry.Member, Total: entry.Total,
			Property: entry.Property, Output: entry.Output,
			References: append([]string(nil), entry.References...),
			Anchor:     entry.Anchor, Members: entry.Members, Through: entry.Through,
		})
	}
	return out
}

// RenderForms renders every Table Form to its exact Form Definition bytes and
// derived fixtures.
func RenderForms() ([]RenderedForm, error) {
	if err := Validate(); err != nil {
		return nil, err
	}
	resolver := newTargetContractResolver()
	out := make([]RenderedForm, 0, len(Forms))
	for _, form := range Forms {
		rendered, err := resolver.renderedForm(form)
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}
	return out, nil
}

func renderForm(form model.Form, resolver model.TargetContractResolver) (RenderedForm, error) {
	fixtures := map[string]map[string]any{"desired.json": form.CanonicalDesired()}
	negativeCases, err := form.NegativeCases()
	if err != nil {
		return RenderedForm{}, err
	}
	negative := make([]formpackage.NegativeFixture, 0, len(negativeCases))
	for _, item := range negativeCases {
		fixturePath := "fixtures/negative-" + item.Name + ".json"
		fixtures["negative-"+item.Name+".json"] = item.Desired
		negative = append(negative, formpackage.NegativeFixture{
			Name: "reject-" + item.Name, Stage: "desired", InputPath: fixturePath,
			ExpectedFailure: "schema_validation_failed",
		})
	}
	provided, err := resolveInterfaceRefs(form.ProvidedInterfaces)
	if err != nil {
		return RenderedForm{}, err
	}
	desiredSchema, err := form.DesiredSchema(resolver)
	if err != nil {
		return RenderedForm{}, err
	}
	outputSchema, err := form.OutputSchema()
	if err != nil {
		return RenderedForm{}, err
	}
	definition := formpackage.FormDefinition{
		APIVersion: Family.APIVersion(), Kind: form.Kind,
		DefinitionVersion: form.DefinitionVersion, Title: form.Title,
		Description: form.Description, Role: string(form.Role),
		RequiresHostAPI: form.RequiresHostAPI, Constraints: renderConstraints(form),
		DesiredSchema: desiredSchema, OutputSchema: outputSchema,
		ImmutableFields: form.ImmutableFields(), LifecycleCapabilities: form.LifecycleCapabilities(),
		ProvidedInterfaces:  provided,
		ConformanceFixtures: []formpackage.ConformanceFixture{{Name: "canonical", DesiredPath: "fixtures/desired.json"}},
		NegativeFixtures:    negative,
	}
	definitionJSON, err := marshalIndented(definition)
	if err != nil {
		return RenderedForm{}, err
	}
	if _, err := formpackage.ValidateDefinition([]byte(definitionJSON)); err != nil {
		return RenderedForm{}, fmt.Errorf("rendered definition is invalid: %w", err)
	}
	return RenderedForm{
		Kind: form.Kind, Slug: form.Slug, Role: string(form.Role),
		Definition: definition, DefinitionJSON: definitionJSON, Fixtures: fixtures,
	}, nil
}

func resolveInterfaceRefs(sources []model.InterfaceRefSource) ([]formpackage.InterfaceRef, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	out := make([]formpackage.InterfaceRef, 0, len(sources))
	for _, source := range sources {
		ref, err := InterfaceRefFor(source.Name, source.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}
