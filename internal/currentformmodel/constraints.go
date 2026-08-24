package currentformmodel

import (
	"fmt"
	"strings"
)

// validateResolvedUIDConstraintTargets proves the cross-Form half of
// sameResolvedTarget. Shape validation alone can prove that `members` is a
// local list relation, but only the exact target Form can say whether
// `through` is a relation there and whether it resolves in the anchor's UID
// domain (group + kind). An Interface-open member admits target Forms whose
// schemas are not closed by this Definition, so it is refused unless the
// Definition pins exact Forms that can each be inspected.
func validateResolvedUIDConstraintTargets(
	schema map[string]any,
	constraints []Constraint,
	resolver TargetContractResolver,
) error {
	hasSameResolvedTarget := false
	for _, constraint := range constraints {
		if constraint.Kind == ConstraintSameResolvedTarget {
			hasSameResolvedTarget = true
			break
		}
	}
	if !hasSameResolvedTarget {
		return nil
	}
	relations, err := DeriveRelationsWithConstraints(schema, constraints)
	if err != nil {
		return err
	}
	exactResolver, ok := resolver.(ExactFormRelationResolver)
	if !ok {
		return fmt.Errorf("sameResolvedTarget requires an exact-Form relation resolver")
	}
	byPointer := make(map[string][]Relation, len(relations))
	for _, relation := range relations {
		byPointer[relation.Pointer] = append(byPointer[relation.Pointer], relation)
	}
	for _, constraint := range constraints {
		if constraint.Kind != ConstraintSameResolvedTarget {
			continue
		}
		anchors := byPointer[constraint.Anchor]
		members := byPointer[constraint.Members]
		if len(anchors) != 1 {
			return fmt.Errorf("sameResolvedTarget anchor %s resolves to %d relation contracts, require exactly one", constraint.Anchor, len(anchors))
		}
		if len(members) != 1 {
			return fmt.Errorf("sameResolvedTarget members %s resolves to %d relation contracts, require exactly one", constraint.Members, len(members))
		}
		anchor, member := anchors[0], members[0]
		if len(member.TargetFormRefs) == 0 || member.RequiredInterface != nil {
			return fmt.Errorf(
				"sameResolvedTarget members %s is Interface-open; pin exact target Forms so through %s can be proven",
				constraint.Members, constraint.Through,
			)
		}
		for _, ref := range member.TargetFormRefs {
			targetRelations, err := exactResolver.ResolveExactFormRelations(ref)
			if err != nil {
				return fmt.Errorf("sameResolvedTarget member target %s: %w", ref.String(), err)
			}
			matches := 0
			for _, through := range targetRelations {
				if through.Pointer != constraint.Through {
					continue
				}
				matches++
				if through.TargetAPIVersion != anchor.TargetAPIVersion || through.TargetKind != anchor.TargetKind {
					return fmt.Errorf(
						"sameResolvedTarget through %s on %s resolves in UID domain %s %s, want anchor domain %s %s",
						constraint.Through, ref.String(), through.TargetAPIVersion, through.TargetKind,
						anchor.TargetAPIVersion, anchor.TargetKind,
					)
				}
			}
			if matches != 1 {
				return fmt.Errorf(
					"sameResolvedTarget through %s resolves to %d relations on exact member target %s, require exactly one",
					constraint.Through, matches, ref.String(),
				)
			}
		}
	}
	return nil
}

func validateResolvedUIDConstraints(form Form) error {
	relations := declaredRelationPointers(form.Fields, "")
	for index, constraint := range form.ResolvedUIDConstraints {
		if err := validateResolvedUIDConstraintShape(constraint); err != nil {
			return fmt.Errorf("form %s resolved-UID constraint %d: %w", form.Kind, index, err)
		}
		for _, pointer := range localConstraintPointers(constraint) {
			if _, declared := relations[pointer]; !declared {
				return fmt.Errorf(
					"form %s resolved-UID constraint %d names %s, which is not a declared relation",
					form.Kind, index, pointer,
				)
			}
		}
	}
	return nil
}

func validateResolvedUIDConstraintShape(constraint Constraint) error {
	foreign := func(allowReference, allowReferences, allowSameTarget bool) error {
		if !allowReference && constraint.Reference != "" {
			return fmt.Errorf("constraint kind %s does not define reference", constraint.Kind)
		}
		if !allowReferences && len(constraint.References) != 0 {
			return fmt.Errorf("constraint kind %s does not define references", constraint.Kind)
		}
		if !allowSameTarget && (constraint.Anchor != "" || constraint.Members != "" || constraint.Through != "") {
			return fmt.Errorf("constraint kind %s does not define anchor, members, or through", constraint.Kind)
		}
		if constraint.KeyedBy != "" || constraint.List != "" || constraint.Member != "" ||
			constraint.Total != 0 || constraint.Property != "" || constraint.Output != "" {
			return fmt.Errorf("constraint kind %s carries members from another constraint grammar", constraint.Kind)
		}
		return nil
	}
	switch constraint.Kind {
	case ConstraintAcyclic:
		if err := foreign(true, false, false); err != nil {
			return err
		}
		if err := validateConstraintPointer(constraint.Reference, 0, 0); err != nil {
			return fmt.Errorf("acyclic reference: %w", err)
		}
	case ConstraintDistinctPair, ConstraintUniquePair:
		if err := foreign(false, true, false); err != nil {
			return err
		}
		if len(constraint.References) != 2 {
			return fmt.Errorf("%s requires exactly two references", constraint.Kind)
		}
		if constraint.References[0] == constraint.References[1] {
			return fmt.Errorf("%s requires two distinct relation pointers", constraint.Kind)
		}
		for _, pointer := range constraint.References {
			if err := validateConstraintPointer(pointer, 0, 0); err != nil {
				return fmt.Errorf("%s reference: %w", constraint.Kind, err)
			}
		}
	case ConstraintSameResolvedTarget:
		if err := foreign(false, false, true); err != nil {
			return err
		}
		if err := validateConstraintPointer(constraint.Anchor, 0, 0); err != nil {
			return fmt.Errorf("sameResolvedTarget anchor: %w", err)
		}
		if err := validateConstraintPointer(constraint.Members, 1, 1); err != nil {
			return fmt.Errorf("sameResolvedTarget members: %w", err)
		}
		if err := validateConstraintPointer(constraint.Through, 0, 0); err != nil {
			return fmt.Errorf("sameResolvedTarget through: %w", err)
		}
	default:
		return fmt.Errorf("constraint kind %q is not a resolved-UID constraint", constraint.Kind)
	}
	return nil
}

func localConstraintPointers(constraint Constraint) []string {
	switch constraint.Kind {
	case ConstraintAcyclic:
		return []string{constraint.Reference}
	case ConstraintDistinctPair, ConstraintUniquePair:
		return constraint.References
	case ConstraintSameResolvedTarget:
		// Through is a relation on every resolved member TARGET, not on this Form.
		return []string{constraint.Anchor, constraint.Members}
	default:
		return nil
	}
}

func validateConstraintPointer(pointer string, minimumWildcards, maximumWildcards int) error {
	if pointer == "" || !strings.HasPrefix(pointer, "/") || strings.HasSuffix(pointer, "/") {
		return fmt.Errorf("%q is not a non-root RFC 6901 pointer", pointer)
	}
	wildcards := 0
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if token == "" {
			return fmt.Errorf("%q contains an empty pointer token", pointer)
		}
		if token == "*" {
			wildcards++
			continue
		}
		for index := 0; index < len(token); index++ {
			if token[index] != '~' {
				continue
			}
			if index+1 >= len(token) || (token[index+1] != '0' && token[index+1] != '1') {
				return fmt.Errorf("%q contains an invalid RFC 6901 escape", pointer)
			}
			index++
		}
	}
	if wildcards < minimumWildcards || wildcards > maximumWildcards {
		return fmt.Errorf(
			"%q carries %d array wildcards, require %d through %d",
			pointer, wildcards, minimumWildcards, maximumWildcards,
		)
	}
	return nil
}

func declaredRelationPointers(fields []Field, prefix string) map[string]struct{} {
	out := map[string]struct{}{}
	var walk func([]Field, string)
	walk = func(fields []Field, parent string) {
		for _, field := range fields {
			pointer := parent + "/" + escapeJSONPointerToken(field.Wire)
			switch field.Kind {
			case KindResourceRef:
				out[pointer] = struct{}{}
			case KindResourceRefList:
				out[pointer+"/*"] = struct{}{}
			case KindBindingList:
				out[pointer+"/*/resource"] = struct{}{}
			case KindObject:
				walk(field.Fields, pointer)
			case KindObjectList:
				walk(field.Fields, pointer+"/*")
			case KindTaggedObject:
				for _, variant := range field.Variants {
					walk(variant.Fields, pointer)
				}
			}
		}
	}
	walk(fields, prefix)
	return out
}
