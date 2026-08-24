package topicformcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
)

// RenderedForm is one Topic Form Definition and its derived fixtures.
// Provider resource mappings are intentionally absent from the source result.
type RenderedForm struct {
	Kind           string                     `json:"kind"`
	Slug           string                     `json:"slug"`
	Role           string                     `json:"role"`
	Definition     formpackage.FormDefinition `json:"definition"`
	DefinitionJSON string                     `json:"definitionJson"`
	Fixtures       map[string]map[string]any  `json:"fixtures"`
}

// TargetResolver resolves exact Topic Form identities and injected Interface
// identities. Queue is intentionally Interface-open: the host resolves a
// concrete PullQueue Resource and pins that resource's FormRef at admission,
// while this source records only queue.pull@1.0.0.
type TargetResolver struct {
	rendered            map[string]RenderedForm
	refs                map[string]model.TargetFormRef
	relations           map[string][]model.Relation
	inProgress          map[string]bool
	relationsInProgress map[string]bool
	queueInterface      formpackage.InterfaceRef
	aggregate           *currentformregistry.TargetResolver
}

var _ model.TargetContractResolver = (*TargetResolver)(nil)
var _ model.ExactFormRelationResolver = (*TargetResolver)(nil)
var _ currentformregistry.ExactFamilySource = (*TargetResolver)(nil)
var _ currentformregistry.RequiredInterfaceSource = (*TargetResolver)(nil)

// QueuePullInterfaceRef returns the canonical queue.pull Interface identity
// from the Queue family source. It is the one cross-family dependency this
// package accepts; the digest is never copied or hand-written here.
func QueuePullInterfaceRef() (formpackage.InterfaceRef, error) {
	return queueformcatalog.InterfaceRefFor(queueformcatalog.QueuePullInterfaceName, "1.0.0")
}

// NewTargetResolver constructs a fresh resolver. Supplying one queue
// InterfaceRef is the injection seam used by an aggregate generation; when no
// argument is supplied the canonical Queue family source is consulted. A
// caller-provided ref is validated as an exact queue.pull@1.0.0 identity.
func NewTargetResolver(queueRefs ...formpackage.InterfaceRef) (*TargetResolver, error) {
	if len(queueRefs) > 1 {
		return nil, fmt.Errorf("topic target resolver accepts one queue.pull InterfaceRef")
	}
	queueRef, err := QueuePullInterfaceRef()
	if err != nil {
		return nil, err
	}
	if len(queueRefs) == 1 {
		queueRef = queueRefs[0]
	}
	if err := validateQueueInterfaceRef(queueRef); err != nil {
		return nil, err
	}
	resolver := &TargetResolver{
		rendered:            map[string]RenderedForm{},
		refs:                map[string]model.TargetFormRef{},
		relations:           map[string][]model.Relation{},
		inProgress:          map[string]bool{},
		relationsInProgress: map[string]bool{},
		queueInterface:      queueRef,
	}
	aggregate, err := currentformregistry.NewTargetResolver(resolver, resolver)
	if err != nil {
		return nil, fmt.Errorf("constructing Topic Form target resolver: %w", err)
	}
	resolver.aggregate = aggregate
	return resolver, nil
}

func validateQueueInterfaceRef(ref formpackage.InterfaceRef) error {
	if ref.APIVersion != queueformcatalog.InterfaceAPIVersion ||
		ref.Name != queueformcatalog.QueuePullInterfaceName || ref.Version != "1.0.0" || ref.SchemaDigest == "" {
		return fmt.Errorf("topic resolver requires exact %s@1.0.0 InterfaceRef, got %+v", queueformcatalog.QueuePullInterfaceName, ref)
	}
	return nil
}

func (r *TargetResolver) FamilyAPIVersion() string    { return Family.APIVersion() }
func (r *TargetResolver) ResourceNamePattern() string { return model.PatternResourceName }

// ResolveResourceTarget implements the aggregate target seam. Interface-open
// targets from another family are resolved locally from their injected exact
// Interface ref; exact same-family targets continue through the aggregate so
// the target Definition digest is derived from its rendered bytes.
func (r *TargetResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	if target.Group == queueFamilyGroup {
		if target.Contract.ExactForm || target.Kind != queueKind || target.Contract.Interface == nil ||
			target.Contract.Interface.Name != queuePullInterfaceName || target.Contract.Interface.Version != "1.0.0" {
			return model.ResolvedResourceTarget{}, fmt.Errorf("topic queue target requires %s/%s with %s@1.0.0", queueFamilyGroup, queueKind, queuePullInterfaceName)
		}
		required, err := r.RequiredInterface(target.Contract.Interface.Name, target.Contract.Interface.Version)
		if err != nil {
			return model.ResolvedResourceTarget{}, err
		}
		return model.ResolvedResourceTarget{
			ResourceNamePattern: model.PatternResourceName,
			RequiredInterface:   &required,
		}, nil
	}
	if target.Group != Family.APIVersion() {
		return model.ResolvedResourceTarget{}, fmt.Errorf("topic target group %q is outside the Topic and Queue families", target.Group)
	}
	if target.Contract.Interface != nil {
		return model.ResolvedResourceTarget{}, fmt.Errorf("topic family target %s/%s does not accept an Interface-open contract", target.Group, target.Kind)
	}
	if r.aggregate == nil {
		return model.ResolvedResourceTarget{}, fmt.Errorf("topic target resolver is not initialized")
	}
	return r.aggregate.ResolveResourceTarget(target)
}

// RequiredInterface resolves the local topic.publish contract and the
// injected queue.pull contract. No other Interface name is silently accepted.
func (r *TargetResolver) RequiredInterface(name, version string) (model.RequiredInterface, error) {
	var ref formpackage.InterfaceRef
	switch name {
	case TopicPublishInterfaceName:
		var err error
		ref, err = InterfaceRefFor(name, version)
		if err != nil {
			return model.RequiredInterface{}, err
		}
	case queueformcatalog.QueuePullInterfaceName:
		if version != "1.0.0" {
			return model.RequiredInterface{}, fmt.Errorf("interface %s is version 1.0.0, not %s", name, version)
		}
		ref = r.queueInterface
	default:
		return model.RequiredInterface{}, fmt.Errorf("interface %s@%s is not injected into Topic catalog", name, version)
	}
	return model.RequiredInterface{
		APIVersion: ref.APIVersion, Name: ref.Name, Version: ref.Version, SchemaDigest: ref.SchemaDigest,
	}, nil
}

// TargetFormRefs returns the one exact rendered Topic identity for kind.
func (r *TargetResolver) TargetFormRefs(kind string) ([]model.TargetFormRef, error) {
	if ref, ok := r.refs[kind]; ok {
		return []model.TargetFormRef{ref}, nil
	}
	if r.inProgress[kind] {
		return nil, fmt.Errorf("topic exact target cycle through %s", kind)
	}
	form, ok := ByKind(kind)
	if !ok {
		return nil, fmt.Errorf("topic exact target %q is not in the catalog", kind)
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
// rendered Topic Definition, never against a latest/default kind alias.
func (r *TargetResolver) ResolveExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	if r.aggregate == nil {
		return nil, fmt.Errorf("topic target resolver is not initialized")
	}
	return r.aggregate.ResolveExactFormRelations(ref)
}

// ExactFormRelations is the family-adapter spelling consumed by the aggregate
// resolver after it has checked the complete FormRef.
func (r *TargetResolver) ExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	if ref.APIVersion != Family.APIVersion() {
		return nil, fmt.Errorf("topic exact target %s is outside catalog group", ref.String())
	}
	key := ref.String()
	if cached, ok := r.relations[key]; ok {
		return append([]model.Relation(nil), cached...), nil
	}
	if r.relationsInProgress[key] {
		return nil, fmt.Errorf("topic exact relation cycle through %s", key)
	}
	form, ok := ByKind(ref.Kind)
	if !ok || form.DefinitionVersion != ref.DefinitionVersion {
		return nil, fmt.Errorf("topic exact target %s is not in this catalog", ref.String())
	}
	refs, err := r.TargetFormRefs(ref.Kind)
	if err != nil {
		return nil, err
	}
	if len(refs) != 1 || refs[0] != ref {
		return nil, fmt.Errorf("topic exact target %s does not match rendered catalog identity", ref.String())
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

// RenderForms renders every Topic Form. The optional queue InterfaceRef is an
// explicit cross-family input; with no argument the canonical Queue source is
// loaded, while tests and aggregate composition can provide an exact fixture.
func RenderForms(queueRefs ...formpackage.InterfaceRef) ([]RenderedForm, error) {
	if err := Validate(); err != nil {
		return nil, err
	}
	resolver, err := NewTargetResolver(queueRefs...)
	if err != nil {
		return nil, err
	}
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
		name := fixtureName(item.Name)
		path := "fixtures/negative-" + name + ".json"
		fixtures["negative-"+name+".json"] = item.Desired
		negative = append(negative, formpackage.NegativeFixture{
			Name: "reject-" + name, Stage: "desired", InputPath: path,
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
	definitionJSON, err := marshalFormIndented(definition)
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

// fixtureName adapts HCL/variant-derived negative case labels to the
// published fixture-name grammar. Tagged variant labels are intentionally
// camelCase in the desired discriminator, while package fixture names are
// lower-case path tokens.
func fixtureName(name string) string { return strings.ToLower(name) }

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

func marshalFormIndented(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// SortedKinds returns source kinds in deterministic order for callers and
// direct catalog tests.
func SortedKinds() []string {
	got := make([]string, 0, len(Forms))
	for _, form := range Forms {
		got = append(got, form.Kind)
	}
	sort.Strings(got)
	return got
}
