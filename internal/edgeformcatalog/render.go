package edgeformcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// RenderedContract is one catalog Interface or Binding Definition rendered to
// its exact publishable bytes and RFC 8785 identity.
type RenderedContract struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// DefinitionJSON is the exact two-space-indented definition.json text.
	DefinitionJSON string `json:"definitionJson"`
	SchemaDigest   string `json:"schemaDigest"`
}

// RenderedForm is one catalog Form rendered to its exact publishable
// Definition bytes and derived fixtures.
type RenderedForm struct {
	Kind         string                     `json:"kind"`
	Slug         string                     `json:"slug"`
	Role         string                     `json:"role"`
	ResourceType string                     `json:"resourceType"`
	Definition   formpackage.FormDefinition `json:"definition"`
	// DefinitionJSON is the exact two-space-indented definition.json text.
	// The candidate writer consumes it verbatim so large integer schema
	// values survive the JavaScript pipeline without float64 rounding.
	DefinitionJSON string                    `json:"definitionJson"`
	Fixtures       map[string]map[string]any `json:"fixtures"`
}

// RenderInterfaces renders the interface catalog to exact bytes and digests.
func RenderInterfaces() ([]RenderedContract, error) {
	definitions := InterfaceDefinitions()
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

// RenderBindings renders the binding catalog to exact bytes and digests.
func RenderBindings() ([]RenderedContract, error) {
	definitions, err := BindingDefinitions()
	if err != nil {
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

// InterfaceRefFor resolves the exact digest-bound InterfaceRef of one catalog
// Interface at generation time.
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

// BindingRefFor resolves the exact digest-bound BindingRef of one catalog
// Binding at generation time.
func BindingRefFor(name, version string) (formpackage.BindingRef, error) {
	definitions, err := BindingDefinitions()
	if err != nil {
		return formpackage.BindingRef{}, err
	}
	for _, definition := range definitions {
		if definition.Name != name {
			continue
		}
		if definition.Version != version {
			return formpackage.BindingRef{}, fmt.Errorf("binding %s is version %s, not %s", name, definition.Version, version)
		}
		rendered, err := renderContract(name, version, definition)
		if err != nil {
			return formpackage.BindingRef{}, err
		}
		return formpackage.BindingRef{
			APIVersion: BindingAPIVersion, Name: name, Version: version, SchemaDigest: rendered.SchemaDigest,
		}, nil
	}
	return formpackage.BindingRef{}, fmt.Errorf("binding %q is not in the catalog", name)
}

// targetContractResolver resolves the exact identities a reference-shaped
// field's declared target contract names (decision 0022).
//
// A required Interface resolves straight out of the interface catalog. An exact
// Form identity is the digest of the TARGET's rendered Definition, so resolving
// one renders that Definition first, memoizes it, and reuses it wherever the
// same Form is rendered again. The reference graph is a DAG by construction —
// a Form whose contract another Form pins may not pin that Form back — and the
// in-progress set turns any future cycle into a refusal instead of a hang.
type targetContractResolver struct {
	rendered   map[string]RenderedForm
	refs       map[string]currentformmodel.TargetFormRef
	inProgress map[string]bool
}

func newTargetContractResolver() *targetContractResolver {
	return &targetContractResolver{
		rendered:   map[string]RenderedForm{},
		refs:       map[string]currentformmodel.TargetFormRef{},
		inProgress: map[string]bool{},
	}
}

// TargetFormRefs returns the exact identity this build renders for one kind.
// The family declares one definition version per Form, so the accepted set is
// exactly one identity; the annotation is a LIST because a Form line that
// carried two live definition versions would accept either.
func (r *targetContractResolver) TargetFormRefs(targetKind string) ([]currentformmodel.TargetFormRef, error) {
	if ref, known := r.refs[targetKind]; known {
		return []currentformmodel.TargetFormRef{ref}, nil
	}
	if r.inProgress[targetKind] {
		return nil, fmt.Errorf("exact-Form target cycle through %s", targetKind)
	}
	form, known := ByKind(targetKind)
	if !known {
		return nil, fmt.Errorf("exact-Form target %q is not a catalog Form", targetKind)
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
	ref := currentformmodel.TargetFormRef{
		APIVersion:        Family.APIVersion(),
		Kind:              targetKind,
		DefinitionVersion: form.DefinitionVersion,
		SchemaDigest:      digest,
	}
	r.refs[targetKind] = ref
	return []currentformmodel.TargetFormRef{ref}, nil
}

// RequiredInterface resolves one exact Interface contract out of the catalog,
// which is where its digest already lives.
func (r *targetContractResolver) RequiredInterface(name, version string) (currentformmodel.RequiredInterface, error) {
	ref, err := InterfaceRefFor(name, version)
	if err != nil {
		return currentformmodel.RequiredInterface{}, err
	}
	return currentformmodel.RequiredInterface{
		APIVersion: ref.APIVersion, Name: ref.Name, Version: ref.Version, SchemaDigest: ref.SchemaDigest,
	}, nil
}

func (r *targetContractResolver) renderedForm(form currentformmodel.Form) (RenderedForm, error) {
	if cached, known := r.rendered[form.Kind]; known {
		return cached, nil
	}
	rendered, err := renderForm(form, r)
	if err != nil {
		return RenderedForm{}, fmt.Errorf("%s: %w", form.Kind, err)
	}
	r.rendered[form.Kind] = rendered
	return rendered, nil
}

// RenderForms renders every catalog Form to its exact Form Definition bytes
// and derived fixture documents.
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

func renderForm(form currentformmodel.Form, resolver currentformmodel.TargetContractResolver) (RenderedForm, error) {
	fixtures := map[string]map[string]any{
		"desired.json": form.CanonicalDesired(),
	}
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
	accepted, err := resolveBindingRefs(form.AcceptedBindings)
	if err != nil {
		return RenderedForm{}, err
	}
	desiredSchema, err := form.DesiredSchema(resolver)
	if err != nil {
		return RenderedForm{}, err
	}
	// The output contract travels in the Definition a host installs, on the
	// member the published Form Definition schema already declares. Nothing is
	// minted for it: the meta-schema admits outputSchema, and the wire schema
	// already reads it to decide that status.outputs is required for a Form that
	// declares one and omitted for a Form that does not (decision 0014, rung 1).
	outputSchema, err := form.OutputSchema()
	if err != nil {
		return RenderedForm{}, err
	}
	definition := formpackage.FormDefinition{
		APIVersion:            Family.APIVersion(),
		Kind:                  form.Kind,
		DefinitionVersion:     form.DefinitionVersion,
		Title:                 form.Title,
		Description:           form.Description,
		Role:                  string(form.Role),
		DesiredSchema:         desiredSchema,
		OutputSchema:          outputSchema,
		ImmutableFields:       form.ImmutableFields(),
		LifecycleCapabilities: form.LifecycleCapabilities(),
		ProvidedInterfaces:    provided,
		AcceptedBindings:      accepted,
		ConformanceFixtures: []formpackage.ConformanceFixture{
			{Name: "canonical", DesiredPath: "fixtures/desired.json"},
		},
		NegativeFixtures: negative,
	}
	definitionJSON, err := marshalIndented(definition)
	if err != nil {
		return RenderedForm{}, err
	}
	if _, err := formpackage.ValidateDefinition([]byte(definitionJSON)); err != nil {
		return RenderedForm{}, fmt.Errorf("rendered definition is invalid: %w", err)
	}
	return RenderedForm{
		Kind: form.Kind, Slug: form.Slug, Role: string(form.Role), ResourceType: form.ResourceType,
		Definition: definition, DefinitionJSON: definitionJSON, Fixtures: fixtures,
	}, nil
}

func resolveInterfaceRefs(sources []currentformmodel.InterfaceRefSource) ([]formpackage.InterfaceRef, error) {
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

func resolveBindingRefs(sources []currentformmodel.BindingRefSource) ([]formpackage.BindingRef, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	out := make([]formpackage.BindingRef, 0, len(sources))
	for _, source := range sources {
		ref, err := BindingRefFor(source.Name, source.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}

// marshalIndented renders value as two-space-indented JSON text with HTML
// escaping disabled and a trailing newline, matching the byte conventions of
// every generated candidate document.
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
