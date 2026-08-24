package currentformmodel

import (
	"fmt"
	"sort"
	"strings"
)

// CanonicalDesired builds the exact desired fixture of a Form from its field
// examples. A fixture is real input a host can attempt, never a placeholder.
// The document carries no "name": the envelope owns metadata.name.
func (f Form) CanonicalDesired() map[string]any {
	desired := map[string]any{}
	for _, field := range f.Fields {
		if field.Example == nil {
			continue
		}
		desired[field.Wire] = canonicalFieldValue(field, field.Example)
	}
	return desired
}

// NegativeCase is one input a conforming host must reject.
type NegativeCase struct {
	Name    string
	Desired map[string]any
}

// NegativeCases derives the rejectable desired inputs of a Form:
//
//   - one unexpected-property case for every Form, proving the closed object;
//   - one missing-required case per required field;
//   - one case per declared or derivable field counter-example;
//   - one invalid-binding-name case per binding list;
//   - one out-of-vocabulary protocol case per external-service list, proving
//     the closed slot vocabulary is enforced at the desired stage rather than
//     discovered when a host tries to resolve an unknown protocol.
//
// Every Form therefore always ends up with at least one desired-stage
// negative fixture.
func (f Form) NegativeCases() ([]NegativeCase, error) {
	var cases []NegativeCase
	appendCase := func(name string, desired map[string]any) {
		cases = append(cases, NegativeCase{Name: name, Desired: desired})
	}

	unexpected := f.CanonicalDesired()
	unexpected["takoformUnexpected"] = true
	appendCase("unexpected-property", unexpected)

	for _, field := range f.Fields {
		if !field.Required {
			continue
		}
		desired := f.CanonicalDesired()
		delete(desired, field.Wire)
		appendCase("missing-"+fixtureToken(field.HCL), desired)
	}
	for _, field := range f.Fields {
		base := field.Example
		if base == nil {
			base = field.Default
		}
		for _, counter := range field.counterExamples(f.Family.APIVersion(), base) {
			desired := f.CanonicalDesired()
			desired[field.Wire] = cloneValue(counter.Value)
			name := fixtureToken(field.HCL)
			if counter.Name != "" {
				name += "-" + counter.Name
			}
			appendCase(name, desired)
		}
	}
	for _, field := range f.Fields {
		if field.Kind != KindBindingList {
			continue
		}
		target, err := field.effectiveResourceTarget(f.Family.APIVersion())
		if err != nil {
			return nil, fmt.Errorf("form %s binding field %s: %w", f.Kind, field.Wire, err)
		}
		desired := f.CanonicalDesired()
		desired[field.Wire] = []any{map[string]any{
			"name": "1binding",
			"resource": map[string]any{
				"apiVersion": target.Group,
				"kind":       target.Kind,
				"name":       "example-target",
			},
		}}
		appendCase(fixtureToken(field.HCL)+"-binding-name", desired)
	}
	for _, field := range f.Fields {
		if field.Kind != KindExternalServiceList {
			continue
		}
		desired := f.CanonicalDesired()
		desired[field.Wire] = []any{map[string]any{
			"name": "PRIMARY",
			"service": map[string]any{
				"apiVersion": StandardServiceAPIVersion,
				"protocol":   "not-namespaced",
			},
		}}
		appendCase(fixtureToken(field.HCL)+"-invalid-protocol", desired)
	}

	seen := map[string]struct{}{}
	for _, item := range cases {
		if _, duplicate := seen[item.Name]; duplicate {
			return nil, fmt.Errorf("form %s derives duplicate negative case %q", f.Kind, item.Name)
		}
		seen[item.Name] = struct{}{}
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("form %s derives no negative case", f.Kind)
	}
	return cases, nil
}

type namedCounterExample struct {
	Name  string
	Value any
}

// counterExamples includes the field's own invalid value and recursively
// mutates members of closed objects, object lists, and the selected tagged
// branch. This makes a newly admitted nested FieldKind observable in the
// generated negative corpus instead of being masked by a valid outer object.
func (f Field) counterExamples(group string, base any) []namedCounterExample {
	var out []namedCounterExample
	if counter, ok := f.counterExample(group); ok {
		out = append(out, namedCounterExample{Value: counter})
	}
	appendNested := func(name string, value any) {
		out = append(out, namedCounterExample{Name: fixtureToken(name), Value: value})
	}
	switch f.Kind {
	case KindObject:
		object, ok := stringKeyedObject(base)
		if !ok {
			return out
		}
		for _, member := range f.Fields {
			for _, nested := range member.counterExamples(group, object[member.Wire]) {
				mutated := cloneValue(object).(map[string]any)
				mutated[member.Wire] = cloneValue(nested.Value)
				name := fixtureToken(member.HCL)
				if nested.Name != "" {
					name += "-" + nested.Name
				}
				appendNested(name, mutated)
			}
		}
	case KindObjectList:
		items, ok := defaultSlice(base)
		if !ok || len(items) == 0 {
			return out
		}
		first, ok := stringKeyedObject(items[0])
		if !ok {
			return out
		}
		for _, member := range f.Fields {
			for _, nested := range member.counterExamples(group, first[member.Wire]) {
				mutated := cloneValue(items).([]any)
				entry, _ := stringKeyedObject(mutated[0])
				entry[member.Wire] = cloneValue(nested.Value)
				mutated[0] = entry
				name := fixtureToken(member.HCL)
				if nested.Name != "" {
					name += "-" + nested.Name
				}
				appendNested(name, mutated)
			}
		}
	case KindTaggedObject:
		object, ok := stringKeyedObject(base)
		if !ok {
			return out
		}
		tag, _ := object[f.Discriminator].(string)
		for _, variant := range f.Variants {
			if variant.Tag != tag {
				continue
			}
			for _, member := range variant.Fields {
				for _, nested := range member.counterExamples(group, object[member.Wire]) {
					mutated := cloneValue(object).(map[string]any)
					mutated[member.Wire] = cloneValue(nested.Value)
					name := fixtureToken(variant.Tag) + "-" + fixtureToken(member.HCL)
					if nested.Name != "" {
						name += "-" + nested.Name
					}
					appendNested(name, mutated)
				}
			}
			break
		}
	}
	return out
}

// counterExample returns the value this field's constraint must reject. A
// declared CounterExample wins; otherwise one is derived from the constraint
// the field already states so a stated constraint cannot ship unproven.
func (f Field) counterExample(group string) (any, bool) {
	if f.CounterExample != nil {
		return f.CounterExample, true
	}
	switch f.Kind {
	case KindInteger:
		if f.Min != nil {
			return *f.Min - 1, true
		}
		if f.Max != nil {
			return *f.Max + 1, true
		}
		return nil, false
	case KindStringEnum:
		return "not-a-declared-choice", true
	case KindStringList:
		return []any{true}, true
	case KindStringMap:
		return map[string]any{"1 invalid key": "value"}, true
	case KindStringSetMap:
		return map[string]any{"1 invalid key": []any{"value"}}, true
	case KindJSONMap:
		return map[string]any{"1 invalid key": "value"}, true
	case KindResourceRef:
		target, err := f.effectiveResourceTarget(group)
		if err != nil {
			return nil, false
		}
		return counterExampleReference(target.Group, target.Kind), true
	case KindResourceRefList:
		target, err := f.effectiveResourceTarget(group)
		if err != nil {
			return nil, false
		}
		return []any{counterExampleReference(target.Group, target.Kind)}, true
	case KindStringSet:
		if len(f.Enum) > 0 {
			return []any{"not-a-declared-choice"}, true
		}
		return []any{true}, true
	case KindObject:
		return map[string]any{"takoformUnexpected": true}, true
	case KindObjectList:
		return []any{map[string]any{"takoformUnexpected": true}}, true
	case KindTaggedObject:
		return map[string]any{f.Discriminator: "not-a-declared-variant"}, true
	default:
		return nil, false
	}
}

// counterExampleReference is a well-formed reference whose NAME violates the
// portable resource-name grammar. The group and kind stay exact so the fixture
// proves the one constraint it names.
func counterExampleReference(group, targetKind string) map[string]any {
	return map[string]any{
		"apiVersion": group,
		"kind":       targetKind,
		"name":       "Not A Resource Name",
	}
}

// fixtureToken renders one fixture-name token from a snake_case HCL name.
func fixtureToken(hcl string) string {
	return strings.ReplaceAll(hcl, "_", "-")
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneValue(item))
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	case map[string][]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneValue(item)
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return value
	}
}

// canonicalFieldValue deep-copies an authored value and applies only the
// field semantics that have more than one JSON spelling. Ordered lists retain
// their authored order and duplicates; string-set-map values sort lexically.
func canonicalFieldValue(field Field, value any) any {
	switch field.Kind {
	case KindObject:
		return canonicalObjectValue(field.Fields, value)
	case KindObjectList:
		items, ok := defaultSlice(value)
		if !ok {
			return cloneValue(value)
		}
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, canonicalObjectValue(field.Fields, item))
		}
		return out
	case KindTaggedObject:
		object, ok := stringKeyedObject(value)
		if !ok {
			return cloneValue(value)
		}
		tag, _ := object[field.Discriminator].(string)
		for _, variant := range field.Variants {
			if variant.Tag == tag {
				return canonicalObjectValue(variant.Fields, object)
			}
		}
		return cloneValue(value)
	case KindStringSetMap:
		// handled below
	default:
		return cloneValue(value)
	}
	object, ok := stringKeyedObject(value)
	if !ok {
		return cloneValue(value)
	}
	out := make(map[string]any, len(object))
	for key, raw := range object {
		items, ok := defaultSlice(raw)
		if !ok {
			out[key] = cloneValue(raw)
			continue
		}
		values := make([]string, 0, len(items))
		allStrings := true
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				allStrings = false
				break
			}
			values = append(values, text)
		}
		if !allStrings {
			out[key] = cloneValue(raw)
			continue
		}
		sort.Strings(values)
		out[key] = cloneValue(values)
	}
	return out
}

func canonicalObjectValue(fields []Field, value any) any {
	object, ok := stringKeyedObject(value)
	if !ok {
		return cloneValue(value)
	}
	byName := make(map[string]Field, len(fields))
	for _, field := range fields {
		byName[field.Wire] = field
	}
	out := make(map[string]any, len(object))
	for name, item := range object {
		field, known := byName[name]
		if !known {
			out[name] = cloneValue(item)
			continue
		}
		out[name] = canonicalFieldValue(field, item)
	}
	return out
}
