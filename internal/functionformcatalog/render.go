package functionformcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// RenderedForm is one Function Form Definition and its derived fixtures. The
// provider registry is intentionally absent; consumers pin the FormRef made
// from DefinitionJSON rather than a Terraform resource name.
type RenderedForm struct {
	Kind           string                     `json:"kind"`
	Slug           string                     `json:"slug"`
	Role           string                     `json:"role"`
	Definition     formpackage.FormDefinition `json:"definition"`
	DefinitionJSON string                     `json:"definitionJson"`
	Fixtures       map[string]map[string]any  `json:"fixtures"`
}

// RenderedContract is one exact runtime Interface Definition and its
// RFC 8785 digest. It is provider-neutral and carries no binding or provider
// resource mapping.
type RenderedContract struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	DefinitionJSON string `json:"definitionJson"`
	SchemaDigest   string `json:"schemaDigest"`
}

// TargetResolver resolves exact same-family Form identities for references
// while rendering. It deliberately has no latest/default fallback and no
// cross-family lookup.
type TargetResolver struct {
	rendered            map[string]RenderedForm
	refs                map[string]model.TargetFormRef
	relations           map[string][]model.Relation
	inProgress          map[string]bool
	relationsInProgress map[string]bool
}

var _ model.TargetContractResolver = (*TargetResolver)(nil)
var _ model.ExactFormRelationResolver = (*TargetResolver)(nil)

// NewTargetResolver constructs a fresh exact resolver for this catalog.
func NewTargetResolver() *TargetResolver {
	return &TargetResolver{
		rendered:            map[string]RenderedForm{},
		refs:                map[string]model.TargetFormRef{},
		relations:           map[string][]model.Relation{},
		inProgress:          map[string]bool{},
		relationsInProgress: map[string]bool{},
	}
}

func (r *TargetResolver) FamilyAPIVersion() string    { return Family.APIVersion() }
func (r *TargetResolver) ResourceNamePattern() string { return model.PatternResourceName }

// ResolveResourceTarget implements the current model's exact target seam.
func (r *TargetResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	if target.Group != Family.APIVersion() {
		return model.ResolvedResourceTarget{}, fmt.Errorf("function target group %q is outside %q", target.Group, Family.APIVersion())
	}
	if target.Contract.ExactForm && target.Contract.Interface != nil {
		return model.ResolvedResourceTarget{}, fmt.Errorf("function target %s/%s declares both exact Form and Interface contracts", target.Group, target.Kind)
	}
	switch {
	case target.Contract.ExactForm:
		refs, err := r.TargetFormRefs(target.Kind)
		if err != nil {
			return model.ResolvedResourceTarget{}, err
		}
		return model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName, TargetFormRefs: refs}, nil
	case target.Contract.Interface != nil:
		if target.Kind != "Function" {
			return model.ResolvedResourceTarget{}, fmt.Errorf("function runtime Interface targets must address Function, got %s", target.Kind)
		}
		requested := target.Contract.Interface
		if requested.Name != FunctionRuntimeInterfaceName || requested.Version != interfaceVersion {
			return model.ResolvedResourceTarget{}, fmt.Errorf("function target requires %s@%s, got %s@%s", FunctionRuntimeInterfaceName, interfaceVersion, requested.Name, requested.Version)
		}
		resolved, err := r.RequiredInterface(requested.Name, requested.Version)
		if err != nil {
			return model.ResolvedResourceTarget{}, err
		}
		return model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName, RequiredInterface: &resolved}, nil
	default:
		return model.ResolvedResourceTarget{}, fmt.Errorf("function target %s/%s declares no exact contract", target.Group, target.Kind)
	}
}

// TargetFormRefs returns the one exact Definition identity rendered for kind.
func (r *TargetResolver) TargetFormRefs(kind string) ([]model.TargetFormRef, error) {
	if ref, ok := r.refs[kind]; ok {
		return []model.TargetFormRef{ref}, nil
	}
	if r.inProgress[kind] {
		return nil, fmt.Errorf("function exact target cycle through %s", kind)
	}
	form, ok := ByKind(kind)
	if !ok {
		return nil, fmt.Errorf("function exact target %q is not in the catalog", kind)
	}
	r.inProgress[kind] = true
	rendered, err := r.renderedForm(form)
	delete(r.inProgress, kind)
	if err != nil {
		return nil, err
	}
	digest, err := formpackage.DigestCanonicalJSON([]byte(rendered.DefinitionJSON))
	if err != nil {
		return nil, err
	}
	ref := model.TargetFormRef{
		APIVersion: Family.APIVersion(), Kind: kind,
		DefinitionVersion: form.DefinitionVersion, SchemaDigest: digest,
	}
	r.refs[kind] = ref
	return []model.TargetFormRef{ref}, nil
}

// ResolveExactFormRelations proves a through relation against the exact
// rendered Definition, not against whichever version happens to be current.
func (r *TargetResolver) ResolveExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	if ref.APIVersion != Family.APIVersion() {
		return nil, fmt.Errorf("function exact target %s is outside catalog group", ref.String())
	}
	key := ref.String()
	if cached, ok := r.relations[key]; ok {
		return append([]model.Relation(nil), cached...), nil
	}
	if r.relationsInProgress[key] {
		return nil, fmt.Errorf("function exact relation cycle through %s", key)
	}
	refs, err := r.TargetFormRefs(ref.Kind)
	if err != nil {
		return nil, err
	}
	if len(refs) != 1 || refs[0] != ref {
		return nil, fmt.Errorf("function exact target %s does not match rendered catalog identity", ref.String())
	}
	form, ok := ByKind(ref.Kind)
	if !ok || form.DefinitionVersion != ref.DefinitionVersion {
		return nil, fmt.Errorf("function exact target %s is not in this catalog", ref.String())
	}
	r.relationsInProgress[key] = true
	schema, err := form.DesiredSchema(r)
	var relations []model.Relation
	if err == nil {
		relations, err = model.DeriveRelationsWithConstraints(schema, form.Constraints())
	}
	delete(r.relationsInProgress, key)
	if err != nil {
		return nil, err
	}
	r.relations[key] = append([]model.Relation(nil), relations...)
	return append([]model.Relation(nil), relations...), nil
}

// ExactFormRelations is the family-adapter spelling consumed by the aggregate
// currentformregistry resolver. The aggregate has already checked the full
// exact FormRef; this method repeats the same check through the local seam.
func (r *TargetResolver) ExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	return r.ResolveExactFormRelations(ref)
}

// RequiredInterface resolves one exact digest-bound function.runtime
// contract. Name and version are never widened to a family default.
func (r *TargetResolver) RequiredInterface(name, version string) (model.RequiredInterface, error) {
	ref, err := InterfaceRefFor(name, version)
	if err != nil {
		return model.RequiredInterface{}, err
	}
	return model.RequiredInterface{
		APIVersion: ref.APIVersion, Name: ref.Name, Version: ref.Version, SchemaDigest: ref.SchemaDigest,
	}, nil
}

func (r *TargetResolver) renderedForm(form model.Form) (RenderedForm, error) {
	if cached, ok := r.rendered[form.Kind]; ok {
		return cached, nil
	}
	rendered, err := renderForm(form, r)
	if err != nil {
		return RenderedForm{}, fmt.Errorf("%s: %w", form.Kind, err)
	}
	r.rendered[form.Kind] = rendered
	return rendered, nil
}

// RenderForms renders every Function Form to its exact definition bytes.
func RenderForms() ([]RenderedForm, error) {
	if err := Validate(); err != nil {
		return nil, err
	}
	resolver := NewTargetResolver()
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

// RenderInterfaces renders the exact Function runtime Interface Definition.
func RenderInterfaces() ([]RenderedContract, error) {
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		return nil, err
	}
	out := make([]RenderedContract, 0, len(definitions))
	for _, definition := range definitions {
		rendered, err := renderContract(definition.Name, definition.Version, definition)
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}
	return out, nil
}

// InterfaceRefFor resolves one exact digest-bound runtime InterfaceRef.
func InterfaceRefFor(name, version string) (formpackage.InterfaceRef, error) {
	definition, err := interfaceDefinitionByName(name)
	if err != nil {
		return formpackage.InterfaceRef{}, err
	}
	if definition.Version != version {
		return formpackage.InterfaceRef{}, fmt.Errorf("interface %s is version %s, not %s", name, definition.Version, version)
	}
	rendered, err := renderContract(name, version, definition)
	if err != nil {
		return formpackage.InterfaceRef{}, err
	}
	return formpackage.InterfaceRef{
		APIVersion: InterfaceAPIVersion, Name: name, Version: version, SchemaDigest: rendered.SchemaDigest,
	}, nil
}

func renderForm(form model.Form, resolver model.TargetContractResolver) (RenderedForm, error) {
	fixtures := map[string]map[string]any{"desired.json": form.CanonicalDesired()}
	negativeCases, err := form.NegativeCases()
	if err != nil {
		return RenderedForm{}, err
	}
	negative := make([]formpackage.NegativeFixture, 0, len(negativeCases))
	for _, item := range negativeCases {
		path := "fixtures/negative-" + item.Name + ".json"
		fixtures["negative-"+item.Name+".json"] = item.Desired
		negative = append(negative, formpackage.NegativeFixture{
			Name: "reject-" + item.Name, Stage: "desired", InputPath: path,
			ExpectedFailure: "schema_validation_failed",
		})
	}
	desiredSchema, err := form.DesiredSchema(resolver)
	if err != nil {
		return RenderedForm{}, err
	}
	outputSchema, err := form.OutputSchema()
	if err != nil {
		return RenderedForm{}, err
	}
	provided, err := resolveInterfaceRefs(form.ProvidedInterfaces)
	if err != nil {
		return RenderedForm{}, err
	}
	definition := formpackage.FormDefinition{
		APIVersion: form.Family.APIVersion(), Kind: form.Kind,
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
		Kind: form.Kind, Slug: form.Slug, Role: string(form.Role), Definition: definition,
		DefinitionJSON: definitionJSON, Fixtures: fixtures,
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

func renderContract(name, version string, definition any) (RenderedContract, error) {
	text, err := marshalIndented(definition)
	if err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	if err := formpackage.ValidateInterfaceDefinition([]byte(text)); err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: invalid Interface Definition: %w", name, version, err)
	}
	digest, err := formpackage.DigestCanonicalJSON([]byte(text))
	if err != nil {
		return RenderedContract{}, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	return RenderedContract{Name: name, Version: version, DefinitionJSON: text, SchemaDigest: digest}, nil
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

// SortedKinds is a small source-inspection helper used by direct catalog
// tests and by tooling that wants deterministic diagnostics.
func SortedKinds() []string {
	got := make([]string, 0, len(Forms))
	for _, form := range Forms {
		got = append(got, form.Kind)
	}
	sort.Strings(got)
	return got
}
