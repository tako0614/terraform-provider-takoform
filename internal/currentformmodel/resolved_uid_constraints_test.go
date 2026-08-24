package currentformmodel

import (
	"strings"
	"testing"
)

func exactRefField(hcl, wire, group, kind string) Field {
	return Field{
		HCL: hcl, Wire: wire, Kind: KindResourceRef, Required: true,
		Doc:     "Exact resolved-UID constraint participant.",
		Example: map[string]any{"apiVersion": group, "kind": kind, "name": hcl},
		ResourceTarget: &ResourceTarget{
			Group: group, Kind: kind, Contract: TargetContract{ExactForm: true},
		},
	}
}

func resolvedUIDConstraintForm(constraint Constraint) Form {
	var fields []Field
	switch constraint.Kind {
	case ConstraintAcyclic:
		fields = []Field{{
			HCL: "dead_letter", Wire: "deadLetter", Kind: KindObject,
			Doc:               "Dead-letter rule. When absent, failed delivery is dropped.",
			AbsenceIsSemantic: true,
			Fields:            []Field{exactRefField("queue", "queue", "queue.forms.takoform.com", "PullQueue")},
		}}
	case ConstraintDistinctPair:
		fields = []Field{
			exactRefField("target", "target", "topic.forms.takoform.com", "Topic"),
			exactRefField("dead_letter", "deadLetter", "topic.forms.takoform.com", "Topic"),
		}
	case ConstraintUniquePair:
		fields = []Field{
			exactRefField("topic", "topic", "topic.forms.takoform.com", "Topic"),
			exactRefField("target", "target", "function.forms.takoform.com", "Function"),
		}
	case ConstraintSameResolvedTarget:
		fields = []Field{
			exactRefField("function", "function", "function.forms.takoform.com", "Function"),
			{
				HCL: "versions", Wire: "versions", Kind: KindObjectList,
				Doc: "Selected versions.", Required: true, MinItems: 1, MaxItems: 8,
				Example: []any{map[string]any{
					"functionVersion": map[string]any{
						"apiVersion": "function.forms.takoform.com", "kind": "FunctionVersion", "name": "version-a",
					},
				}},
				Fields: []Field{exactRefField(
					"function_version", "functionVersion", "function.forms.takoform.com", "FunctionVersion",
				)},
			},
		}
	}
	form := semanticForm(fields...)
	form.RequiresHostAPI = "forms.takoform.com/v1"
	form.ResolvedUIDConstraints = []Constraint{constraint}
	return form
}

func TestResolvedUIDConstraintGrammarAcceptsEachNarrowMechanism(t *testing.T) {
	t.Parallel()
	for name, constraint := range map[string]Constraint{
		"acyclic": {
			Kind: ConstraintAcyclic, Reference: "/deadLetter/queue",
		},
		"distinct pair": {
			Kind: ConstraintDistinctPair, References: []string{"/target", "/deadLetter"},
		},
		"unique pair": {
			Kind: ConstraintUniquePair, References: []string{"/topic", "/target"},
		},
		"same resolved target": {
			Kind:   ConstraintSameResolvedTarget,
			Anchor: "/function", Members: "/versions/*/functionVersion", Through: "/function",
		},
	} {
		name, constraint := name, constraint
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			form := resolvedUIDConstraintForm(constraint)
			if err := form.Validate(); err != nil {
				t.Fatal(err)
			}
			if _, err := DeriveRelationsWithConstraints(mustDesiredSchema(t, form), form.Constraints()); err != nil {
				t.Fatalf("constraint %+v was refused: %v", constraint, err)
			}
		})
	}
}

func TestResolvedUIDConstraintsRefuseUnknownOrNonRelationPointers(t *testing.T) {
	t.Parallel()
	for name, constraint := range map[string]Constraint{
		"acyclic non-relation": {
			Kind: ConstraintAcyclic, Reference: "/absent",
		},
		"distinct pair repeats one relation": {
			Kind: ConstraintDistinctPair, References: []string{"/target", "/target"},
		},
		"unique pair has three members": {
			Kind: ConstraintUniquePair, References: []string{"/topic", "/target", "/source"},
		},
		"same target members are not a list relation": {
			Kind:   ConstraintSameResolvedTarget,
			Anchor: "/function", Members: "/function", Through: "/function",
		},
	} {
		name, constraint := name, constraint
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := resolvedUIDConstraintForm(constraint).Validate()
			if err == nil || (!strings.Contains(err.Error(), "constraint") && !strings.Contains(err.Error(), "relation")) {
				t.Fatalf("Validate error = %v, want resolved-UID constraint refusal", err)
			}
		})
	}
}

type sameTargetResolver struct {
	resource func(ResourceTarget) (ResolvedResourceTarget, error)
	exact    func(TargetFormRef) ([]Relation, error)
}

func (r sameTargetResolver) ResolveResourceTarget(target ResourceTarget) (ResolvedResourceTarget, error) {
	if r.resource != nil {
		return r.resource(target)
	}
	return stubResolver{}.ResolveResourceTarget(target)
}

func (r sameTargetResolver) ResolveExactFormRelations(ref TargetFormRef) ([]Relation, error) {
	return r.exact(ref)
}

func TestSameResolvedTargetProvesEveryExactMemberThroughRelationAndUIDDomain(t *testing.T) {
	t.Parallel()
	constraint := Constraint{
		Kind:   ConstraintSameResolvedTarget,
		Anchor: "/function", Members: "/versions/*/functionVersion", Through: "/function",
	}
	form := resolvedUIDConstraintForm(constraint)
	for name, exact := range map[string]func(TargetFormRef) ([]Relation, error){
		"missing through relation": func(TargetFormRef) ([]Relation, error) { return nil, nil },
		"wrong UID domain": func(TargetFormRef) ([]Relation, error) {
			return []Relation{{
				Pointer: "/function", TargetAPIVersion: "other.forms.takoform.com", TargetKind: "Function",
			}}, nil
		},
	} {
		name, exact := name, exact
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := form.DesiredSchema(sameTargetResolver{exact: exact})
			if err == nil || !strings.Contains(err.Error(), "sameResolvedTarget") {
				t.Fatalf("DesiredSchema error = %v, want exact through-contract refusal", err)
			}
		})
	}
}

func TestSameResolvedTargetRefusesInterfaceOpenMembers(t *testing.T) {
	t.Parallel()
	constraint := Constraint{
		Kind:   ConstraintSameResolvedTarget,
		Anchor: "/function", Members: "/versions/*/functionVersion", Through: "/function",
	}
	form := resolvedUIDConstraintForm(constraint)
	member := &form.Fields[1].Fields[0]
	member.ResourceTarget.Contract = testInterfaceContract()
	_, err := form.DesiredSchema(sameTargetResolver{
		exact: func(TargetFormRef) ([]Relation, error) {
			t.Fatal("Interface-open members must fail before exact Form resolution")
			return nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Interface-open") {
		t.Fatalf("DesiredSchema error = %v, want Interface-open guarantee refusal", err)
	}
}
