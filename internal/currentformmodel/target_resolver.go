package currentformmodel

import (
	"fmt"
	"sort"
)

// ExactFamilySource is one publisher-authoring adapter for TargetResolver. A
// source owns how its immutable Definitions are rendered, while the aggregate
// resolver owns group-first, kind-second, exact-identity selection.
type ExactFamilySource interface {
	FamilyAPIVersion() string
	ResourceNamePattern() string
	TargetFormRefs(kind string) ([]TargetFormRef, error)
	ExactFormRelations(ref TargetFormRef) ([]Relation, error)
}

// RequiredInterfaceSource resolves the shared exact Interface vocabulary. It
// is independent of family lookup because an Interface is an exact contract,
// not a latest/default Form selection.
type RequiredInterfaceSource interface {
	RequiredInterface(name, version string) (RequiredInterface, error)
}

// TargetResolver is the publisher-private union of explicitly injected family
// authoring sources. It has no built-in official-family roster.
type TargetResolver struct {
	families        map[string]ExactFamilySource
	interfaceSource RequiredInterfaceSource
}

// NewTargetResolver constructs an immutable union. Empty and duplicate group
// keys fail closed so input order can never decide an exact target.
func NewTargetResolver(interfaceSource RequiredInterfaceSource, sources ...ExactFamilySource) (*TargetResolver, error) {
	resolver := &TargetResolver{
		families:        make(map[string]ExactFamilySource, len(sources)),
		interfaceSource: interfaceSource,
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
// never searches another family or substitutes a latest/default Definition.
func (r *TargetResolver) ResolveResourceTarget(target ResourceTarget) (ResolvedResourceTarget, error) {
	source, known := r.families[target.Group]
	if !known {
		return ResolvedResourceTarget{}, fmt.Errorf(
			"takoform: ResourceTarget group %q is not among the injected exact families", target.Group,
		)
	}
	resolved := ResolvedResourceTarget{ResourceNamePattern: source.ResourceNamePattern()}
	switch {
	case target.Contract.ExactForm:
		refs, err := source.TargetFormRefs(target.Kind)
		if err != nil {
			return ResolvedResourceTarget{}, err
		}
		if err := validateExactTargetRefs(target, refs); err != nil {
			return ResolvedResourceTarget{}, err
		}
		resolved.TargetFormRefs = append([]TargetFormRef(nil), refs...)
	case target.Contract.Interface != nil:
		if r.interfaceSource == nil {
			return ResolvedResourceTarget{}, fmt.Errorf(
				"takoform: ResourceTarget %s/%s requires Interface %s@%s but no Interface source was injected",
				target.Group, target.Kind, target.Contract.Interface.Name, target.Contract.Interface.Version,
			)
		}
		required, err := r.interfaceSource.RequiredInterface(
			target.Contract.Interface.Name, target.Contract.Interface.Version,
		)
		if err != nil {
			return ResolvedResourceTarget{}, err
		}
		resolved.RequiredInterface = &required
	default:
		return ResolvedResourceTarget{}, fmt.Errorf("takoform: ResourceTarget %s/%s has no contract", target.Group, target.Kind)
	}
	return resolved, nil
}

// ResolveExactFormRelations returns the relation contract for one exact
// Definition after proving the ref belongs to the injected Definition set.
func (r *TargetResolver) ResolveExactFormRelations(ref TargetFormRef) ([]Relation, error) {
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
	return append([]Relation(nil), relations...), nil
}

func validateExactTargetRefs(target ResourceTarget, refs []TargetFormRef) error {
	if len(refs) == 0 {
		return fmt.Errorf("takoform: ResourceTarget %s/%s resolved no exact Definitions", target.Group, target.Kind)
	}
	seen := map[TargetFormRef]bool{}
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
	_ TargetContractResolver    = (*TargetResolver)(nil)
	_ ExactFormRelationResolver = (*TargetResolver)(nil)
)
