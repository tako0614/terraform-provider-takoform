package scheduleformcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/topicformcatalog"
)

// RenderedForm is one exact Schedule Form Definition and its derived fixtures.
// Provider resource mappings are intentionally absent from the source result.
type RenderedForm struct {
	Kind           string                     `json:"kind"`
	Slug           string                     `json:"slug"`
	Role           string                     `json:"role"`
	Definition     formpackage.FormDefinition `json:"definition"`
	DefinitionJSON string                     `json:"definitionJson"`
	Fixtures       map[string]map[string]any  `json:"fixtures"`
}

// TargetResolver resolves the two injected cross-family Interface identities
// and exact Schedule Form identities. Queue and Topic references are
// Interface-open: the host resolves a concrete target Resource and pins that
// Resource's FormRef at admission, while this source records only the exact
// required behavior.
type TargetResolver struct {
	rendered            map[string]RenderedForm
	refs                map[string]model.TargetFormRef
	relations           map[string][]model.Relation
	inProgress          map[string]bool
	relationsInProgress map[string]bool
	queueInterface      formpackage.InterfaceRef
	topicInterface      formpackage.InterfaceRef
	aggregate           *model.TargetResolver
}

var _ model.TargetContractResolver = (*TargetResolver)(nil)
var _ model.ExactFormRelationResolver = (*TargetResolver)(nil)
var _ model.ExactFamilySource = (*TargetResolver)(nil)
var _ model.RequiredInterfaceSource = (*TargetResolver)(nil)

// NewTargetResolver constructs a fresh resolver. Supplying two refs is the
// aggregate generation seam (queue.pull@1.0.0 then topic.publish@1.0.0). With
// no args, the canonical family sources are consulted. A partial injection is
// rejected so a caller cannot accidentally render against a mixed fixture and
// canonical set.
func NewTargetResolver(interfaceRefs ...formpackage.InterfaceRef) (*TargetResolver, error) {
	if len(interfaceRefs) != 0 && len(interfaceRefs) != 2 {
		return nil, fmt.Errorf("schedule target resolver accepts either no InterfaceRefs or queue.pull and topic.publish")
	}
	queueRef, err := queueformcatalog.InterfaceRefFor(queueformcatalog.QueuePullInterfaceName, "1.0.0")
	if err != nil {
		return nil, err
	}
	topicRef, err := topicformcatalog.InterfaceRefFor(topicformcatalog.TopicPublishInterfaceName, "1.0.0")
	if err != nil {
		return nil, err
	}
	if len(interfaceRefs) == 2 {
		queueRef, topicRef = interfaceRefs[0], interfaceRefs[1]
	}
	if err := validateQueueInterfaceRef(queueRef); err != nil {
		return nil, err
	}
	if err := validateTopicInterfaceRef(topicRef); err != nil {
		return nil, err
	}
	resolver := &TargetResolver{
		rendered:            map[string]RenderedForm{},
		refs:                map[string]model.TargetFormRef{},
		relations:           map[string][]model.Relation{},
		inProgress:          map[string]bool{},
		relationsInProgress: map[string]bool{},
		queueInterface:      queueRef,
		topicInterface:      topicRef,
	}
	aggregate, err := model.NewTargetResolver(resolver, resolver)
	if err != nil {
		return nil, fmt.Errorf("constructing Schedule Form target resolver: %w", err)
	}
	resolver.aggregate = aggregate
	return resolver, nil
}

func validateQueueInterfaceRef(ref formpackage.InterfaceRef) error {
	if ref.APIVersion != queueformcatalog.InterfaceAPIVersion ||
		ref.Name != queueformcatalog.QueuePullInterfaceName || ref.Version != "1.0.0" || ref.SchemaDigest == "" {
		return fmt.Errorf("schedule resolver requires exact %s@1.0.0 InterfaceRef, got %+v", queueformcatalog.QueuePullInterfaceName, ref)
	}
	return nil
}

func validateTopicInterfaceRef(ref formpackage.InterfaceRef) error {
	if ref.APIVersion != topicformcatalog.InterfaceAPIVersion ||
		ref.Name != topicformcatalog.TopicPublishInterfaceName || ref.Version != "1.0.0" || ref.SchemaDigest == "" {
		return fmt.Errorf("schedule resolver requires exact %s@1.0.0 InterfaceRef, got %+v", topicformcatalog.TopicPublishInterfaceName, ref)
	}
	return nil
}

func (r *TargetResolver) FamilyAPIVersion() string    { return Family.APIVersion() }
func (r *TargetResolver) ResourceNamePattern() string { return model.PatternResourceName }

// ResolveResourceTarget implements the aggregate target seam. The target
// group, kind, and required Interface are checked as one closed tuple; no
// latest/default/fallback target is accepted.
func (r *TargetResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	if target.Group == queueFamilyGroup {
		if target.Contract.ExactForm || target.Kind != queueKind || target.Contract.Interface == nil ||
			target.Contract.Interface.Name != queueformcatalog.QueuePullInterfaceName || target.Contract.Interface.Version != "1.0.0" {
			return model.ResolvedResourceTarget{}, fmt.Errorf("schedule queue target requires %s/%s with %s@1.0.0", queueFamilyGroup, queueKind, queueformcatalog.QueuePullInterfaceName)
		}
		return model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName, RequiredInterface: requiredInterfaceRef(r.queueInterface)}, nil
	}
	if target.Group == topicFamilyGroup {
		if target.Contract.ExactForm || target.Kind != topicKind || target.Contract.Interface == nil ||
			target.Contract.Interface.Name != topicformcatalog.TopicPublishInterfaceName || target.Contract.Interface.Version != "1.0.0" {
			return model.ResolvedResourceTarget{}, fmt.Errorf("schedule topic target requires %s/%s with %s@1.0.0", topicFamilyGroup, topicKind, topicformcatalog.TopicPublishInterfaceName)
		}
		return model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName, RequiredInterface: requiredInterfaceRef(r.topicInterface)}, nil
	}
	if r.aggregate == nil {
		return model.ResolvedResourceTarget{}, fmt.Errorf("schedule target resolver is not initialized")
	}
	return r.aggregate.ResolveResourceTarget(target)
}

func requiredInterfaceRef(ref formpackage.InterfaceRef) *model.RequiredInterface {
	return &model.RequiredInterface{APIVersion: ref.APIVersion, Name: ref.Name, Version: ref.Version, SchemaDigest: ref.SchemaDigest}
}

// RequiredInterface resolves only the two exact Interface contracts accepted
// by this Schedule source.
func (r *TargetResolver) RequiredInterface(name, version string) (model.RequiredInterface, error) {
	switch name {
	case queueformcatalog.QueuePullInterfaceName:
		if version != "1.0.0" {
			return model.RequiredInterface{}, fmt.Errorf("interface %s is version 1.0.0, not %s", name, version)
		}
		return *requiredInterfaceRef(r.queueInterface), nil
	case topicformcatalog.TopicPublishInterfaceName:
		if version != "1.0.0" {
			return model.RequiredInterface{}, fmt.Errorf("interface %s is version 1.0.0, not %s", name, version)
		}
		return *requiredInterfaceRef(r.topicInterface), nil
	default:
		return model.RequiredInterface{}, fmt.Errorf("interface %s@%s is not accepted by Schedule targets", name, version)
	}
}

// TargetFormRefs returns the one exact rendered Schedule identity for kind.
// Schedule currently has no exact self-reference, but exposing the family
// adapter keeps aggregate generation and relation admission uniform.
func (r *TargetResolver) TargetFormRefs(kind string) ([]model.TargetFormRef, error) {
	if ref, ok := r.refs[kind]; ok {
		return []model.TargetFormRef{ref}, nil
	}
	if r.inProgress[kind] {
		return nil, fmt.Errorf("schedule exact target cycle through %s", kind)
	}
	form, ok := ByKind(kind)
	if !ok {
		return nil, fmt.Errorf("schedule exact target %q is not in the catalog", kind)
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
	ref := model.TargetFormRef{APIVersion: Family.APIVersion(), Kind: kind, DefinitionVersion: form.DefinitionVersion, SchemaDigest: digest}
	r.refs[kind] = ref
	return []model.TargetFormRef{ref}, nil
}

func (r *TargetResolver) ResolveExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	if r.aggregate == nil {
		return nil, fmt.Errorf("schedule target resolver is not initialized")
	}
	return r.aggregate.ResolveExactFormRelations(ref)
}

// ExactFormRelations is the family-adapter spelling consumed by the aggregate
// resolver after it has checked the complete FormRef.
func (r *TargetResolver) ExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	if ref.APIVersion != Family.APIVersion() {
		return nil, fmt.Errorf("schedule exact target %s is outside catalog group", ref.String())
	}
	key := ref.String()
	if cached, ok := r.relations[key]; ok {
		return append([]model.Relation(nil), cached...), nil
	}
	if r.relationsInProgress[key] {
		return nil, fmt.Errorf("schedule exact relation cycle through %s", key)
	}
	form, ok := ByKind(ref.Kind)
	if !ok || form.DefinitionVersion != ref.DefinitionVersion {
		return nil, fmt.Errorf("schedule exact target %s is not in this catalog", ref.String())
	}
	refs, err := r.TargetFormRefs(ref.Kind)
	if err != nil {
		return nil, err
	}
	if len(refs) != 1 || refs[0] != ref {
		return nil, fmt.Errorf("schedule exact target %s does not match rendered catalog identity", ref.String())
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

// RenderForms renders every Schedule Form. Optional InterfaceRefs are an
// explicit cross-family input; with no args, canonical queue/topic sources are
// loaded.
func RenderForms(interfaceRefs ...formpackage.InterfaceRef) ([]RenderedForm, error) {
	if err := Validate(); err != nil {
		return nil, err
	}
	resolver, err := NewTargetResolver(interfaceRefs...)
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
	return RenderedForm{Kind: form.Kind, Slug: form.Slug, Role: string(form.Role), Definition: definition, DefinitionJSON: definitionJSON, Fixtures: fixtures}, nil
}

// fixtureName adapts HCL/variant-derived negative case labels to the
// published fixture-name grammar. Tagged variant labels are intentionally
// camelCase in the desired discriminator, while package fixture names are
// lower-case path tokens.
func fixtureName(name string) string { return strings.ToLower(name) }

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
