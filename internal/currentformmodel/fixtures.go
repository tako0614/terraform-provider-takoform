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
//   - one invalid-binding-name case per binding list.
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
		counter, ok := field.counterExample()
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
			"name":     "1binding",
			"resource": map[string]any{"kind": field.TargetKind, "name": "example-target"},
		}}
		appendCase(fixtureToken(field.HCL)+"-binding-name", desired)
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
func (f Field) counterExample() (any, bool) {
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
	case KindDateString:
		return "not-a-date", true
	case KindJSONMap:
		return map[string]any{"1 invalid key": "value"}, true
	case KindResourceRef:
		return map[string]any{"kind": f.TargetKind, "name": "Not A Resource Name"}, true
	case KindResourceRefList:
		return []any{map[string]any{"kind": f.TargetKind, "name": "Not A Resource Name"}}, true
	case KindStringSet:
		if len(f.Enum) > 0 {
			return []any{"not-a-declared-choice"}, true
		}
		return []any{"not an item"}, true
	default:
		return nil, false
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
