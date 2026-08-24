package currentformregistry

import (
	"fmt"
	"sort"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// ExactFamilySource is one family adapter for TargetResolver. A source owns
// how its immutable Definitions are rendered, but the aggregate resolver owns
// selection: group first, then kind, then the whole exact Form identity. This
// prevents a same-named kind in another installed family from changing which
// contract a ResourceTarget means.
type ExactFamilySource interface {
	FamilyAPIVersion() string
	ResourceNamePattern() string
	TargetFormRefs(kind string) ([]model.TargetFormRef, error)
	ExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error)
}

// RequiredInterfaceSource resolves the shared exact Interface vocabulary.
// It is separate from family lookup because an Interface contract is open to
// every Form that proves it provides that exact behavior; it is not a latest
// or default Form selection.
type RequiredInterfaceSource interface {
	RequiredInterface(name, version string) (model.RequiredInterface, error)
}

// TargetResolver combines injected family adapters into the Form-neutral
// ResourceTarget resolver. It holds no built-in family list and creates no
// definitions. A host or generation pipeline supplies the exact families it
// installed; adding another input widens only the explicitly keyed group.
type TargetResolver struct {
	families        map[string]ExactFamilySource
	interfaceSource RequiredInterfaceSource
}

// NewTargetResolver constructs an immutable union of exact family sources.
// Duplicate or empty group keys are refused because lookup order must never
// decide which Definition a ResourceTarget means.
func NewTargetResolver(interfaceSource RequiredInterfaceSource, sources ...ExactFamilySource) (*TargetResolver, error) {
	resolver := &TargetResolver{
		families: make(map[string]ExactFamilySource, len(sources)), interfaceSource: interfaceSource,
	}
	for _, source := range sources {
		if source == nil {
			return nil, fmt.Errorf("takoform: target resolver received a nil family source")
		}
		group := source.FamilyAPIVersion()
		if group == "" {
			return nil, fmt.Errorf("takoform: target resolver family source has an empty group")
		}
		if _, duplicate := resolver.families[group]; duplicate {
			return nil, fmt.Errorf("takoform: target resolver has two sources for family group %q", group)
		}
		if source.ResourceNamePattern() == "" {
			return nil, fmt.Errorf("takoform: target resolver family %q has no resource name pattern", group)
		}
		resolver.families[group] = source
	}
	return resolver, nil
}

// ResolveResourceTarget resolves exactly the group+kind+contract tuple. It
// never searches another family, chooses a latest Definition, or substitutes
// a default create target.
func (r *TargetResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	source, known := r.families[target.Group]
	if !known {
		return model.ResolvedResourceTarget{}, fmt.Errorf(
			"takoform: ResourceTarget group %q is not among the injected exact families", target.Group,
		)
	}
	resolved := model.ResolvedResourceTarget{ResourceNamePattern: source.ResourceNamePattern()}
	switch {
	case target.Contract.ExactForm:
		refs, err := source.TargetFormRefs(target.Kind)
		if err != nil {
			return model.ResolvedResourceTarget{}, err
		}
		if err := validateExactTargetRefs(target, refs); err != nil {
			return model.ResolvedResourceTarget{}, err
		}
		resolved.TargetFormRefs = refs
	case target.Contract.Interface != nil:
		if r.interfaceSource == nil {
			return model.ResolvedResourceTarget{}, fmt.Errorf(
				"takoform: ResourceTarget %s/%s requires Interface %s@%s but no Interface source was injected",
				target.Group, target.Kind, target.Contract.Interface.Name, target.Contract.Interface.Version,
			)
		}
		required, err := r.interfaceSource.RequiredInterface(
			target.Contract.Interface.Name, target.Contract.Interface.Version,
		)
		if err != nil {
			return model.ResolvedResourceTarget{}, err
		}
		resolved.RequiredInterface = &required
	default:
		return model.ResolvedResourceTarget{}, fmt.Errorf("takoform: ResourceTarget %s/%s has no contract", target.Group, target.Kind)
	}
	return resolved, nil
}

// ResolveExactFormRelations returns the relation contract of the one exact
// Definition named by ref. The ref must first be present in that family's
// declared exact set; a source cannot use this call as a latest/default alias.
func (r *TargetResolver) ResolveExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	source, known := r.families[ref.APIVersion]
	if !known {
		return nil, fmt.Errorf("takoform: exact target Form group %q is not among the injected exact families", ref.APIVersion)
	}
	refs, err := source.TargetFormRefs(ref.Kind)
	if err != nil {
		return nil, err
	}
	matched := false
	for _, candidate := range refs {
		if candidate == ref {
			matched = true
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("takoform: exact target Form %s is not in the injected Definition set", ref.String())
	}
	relations, err := source.ExactFormRelations(ref)
	if err != nil {
		return nil, err
	}
	return append([]model.Relation(nil), relations...), nil
}

func validateExactTargetRefs(target model.ResourceTarget, refs []model.TargetFormRef) error {
	if len(refs) == 0 {
		return fmt.Errorf("takoform: ResourceTarget %s/%s resolved no exact Definitions", target.Group, target.Kind)
	}
	seen := map[model.TargetFormRef]bool{}
	for _, ref := range refs {
		if ref.APIVersion != target.Group || ref.Kind != target.Kind || ref.DefinitionVersion == "" || ref.SchemaDigest == "" {
			return fmt.Errorf(
				"takoform: exact Definition %s does not match ResourceTarget %s/%s",
				ref.String(), target.Group, target.Kind,
			)
		}
		if seen[ref] {
			return fmt.Errorf("takoform: ResourceTarget %s/%s resolved duplicate exact Definition %s", target.Group, target.Kind, ref.String())
		}
		seen[ref] = true
	}
	if !sort.SliceIsSorted(refs, func(i, j int) bool {
		if refs[i].DefinitionVersion != refs[j].DefinitionVersion {
			return refs[i].DefinitionVersion < refs[j].DefinitionVersion
		}
		return refs[i].SchemaDigest < refs[j].SchemaDigest
	}) {
		return fmt.Errorf("takoform: ResourceTarget %s/%s exact Definition set is not deterministically ordered", target.Group, target.Kind)
	}
	return nil
}

var (
	_ model.TargetContractResolver    = (*TargetResolver)(nil)
	_ model.ExactFormRelationResolver = (*TargetResolver)(nil)
)
