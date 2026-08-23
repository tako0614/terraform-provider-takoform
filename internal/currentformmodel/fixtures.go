package currentformmodel

import (
	"fmt"
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
		desired[field.Wire] = cloneValue(field.Example)
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
		counter, ok := field.counterExample(f.Family.APIVersion())
		if !ok {
			continue
		}
		desired := f.CanonicalDesired()
		desired[field.Wire] = cloneValue(counter)
		appendCase(fixtureToken(field.HCL), desired)
	}
	for _, field := range f.Fields {
		if field.Kind != KindBindingList {
			continue
		}
		desired := f.CanonicalDesired()
		desired[field.Wire] = []any{map[string]any{
			"name": "1binding",
			"resource": map[string]any{
				"apiVersion": f.Family.APIVersion(),
				"kind":       field.TargetKind,
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
				"protocol":   "not-a-standard-protocol",
			},
		}}
		appendCase(fixtureToken(field.HCL)+"-unknown-protocol", desired)
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
	case KindJSONMap:
		return map[string]any{"1 invalid key": "value"}, true
	case KindResourceRef:
		return counterExampleReference(group, f.TargetKind), true
	case KindResourceRefList:
		return []any{counterExampleReference(group, f.TargetKind)}, true
	case KindStringSet:
		if len(f.Enum) > 0 {
			return []any{"not-a-declared-choice"}, true
		}
		return []any{"not an item"}, true
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
	default:
		return value
	}
}
