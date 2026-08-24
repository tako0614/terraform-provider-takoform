package currentformmodel

import (
	"strings"
	"testing"
)

// stubResolver answers target-contract resolution with fixed synthetic
// identities. The model never mints a digest of its own, so a unit test has to
// supply one exactly as the generation pipeline does.
type stubResolver struct{}

func (stubResolver) ResolveResourceTarget(target ResourceTarget) (ResolvedResourceTarget, error) {
	resolved := ResolvedResourceTarget{ResourceNamePattern: PatternResourceName}
	if target.Contract.ExactForm {
		resolved.TargetFormRefs = []TargetFormRef{{
			APIVersion:        target.Group,
			Kind:              target.Kind,
			DefinitionVersion: "0.1.0",
			SchemaDigest:      "sha256:" + strings.Repeat("a", 64),
		}}
	}
	if target.Contract.Interface != nil {
		resolved.RequiredInterface = &RequiredInterface{
			APIVersion:   "interfaces.takoform.com/v1alpha1",
			Name:         target.Contract.Interface.Name,
			Version:      target.Contract.Interface.Version,
			SchemaDigest: "sha256:" + strings.Repeat("b", 64),
		}
	}
	return resolved, nil
}

func (stubResolver) ResolveExactFormRelations(TargetFormRef) ([]Relation, error) {
	return []Relation{{
		Pointer: "/function", TargetAPIVersion: "function.forms.takoform.com", TargetKind: "Function",
	}}, nil
}

// testInterfaceContract is the contract every reference-shaped field in this
// package's test forms requires of its target.
func testInterfaceContract() TargetContract {
	return TargetContract{Interface: &InterfaceRefSource{Name: "edge.kv", Version: "1.0.0"}}
}

func mustDesiredSchema(t *testing.T, form Form) map[string]any {
	t.Helper()
	schema, err := form.DesiredSchema(stubResolver{})
	if err != nil {
		t.Fatalf("desired schema: %v", err)
	}
	return schema
}

// TestReferenceWithoutATargetContractIsRefused proves the authoring model
// refuses the shape decision 0022 closes: a reference whose only statement
// about its target is a group and a kind.
func TestReferenceWithoutATargetContractIsRefused(t *testing.T) {
	t.Parallel()
	form := testForm()
	for index := range form.Fields {
		if form.Fields[index].Kind == KindBindingList {
			form.Fields[index].Target = TargetContract{}
		}
	}
	err := form.Validate()
	if err == nil || !strings.Contains(err.Error(), "no target contract") {
		t.Fatalf("validate error = %v, want the missing-target-contract rule", err)
	}
}

// TestReferenceWithBothTargetContractsIsRefused proves the two annotations are
// alternatives, never a pair: a relation depends on the exact Form or on an
// Interface, and stating both would be two sources of truth for one dependency.
func TestReferenceWithBothTargetContractsIsRefused(t *testing.T) {
	t.Parallel()
	form := testForm()
	for index := range form.Fields {
		if form.Fields[index].Kind == KindBindingList {
			form.Fields[index].Target = TargetContract{
				ExactForm: true,
				Interface: &InterfaceRefSource{Name: "edge.kv", Version: "1.0.0"},
			}
		}
	}
	err := form.Validate()
	if err == nil || !strings.Contains(err.Error(), "both an exact Form contract") {
		t.Fatalf("validate error = %v, want the both-contracts rule", err)
	}
}

// TestDerivedRelationCarriesItsTargetContract proves the annotation survives
// the round trip a host makes: it is emitted onto the reference node and read
// back out of the served desired schema.
func TestDerivedRelationCarriesItsTargetContract(t *testing.T) {
	t.Parallel()
	relations, err := DeriveRelations(mustDesiredSchema(t, testForm()))
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 {
		t.Fatalf("derived %d relations, want 1", len(relations))
	}
	relation := relations[0]
	if relation.RequiredInterface == nil || relation.RequiredInterface.Name != "edge.kv" {
		t.Fatalf("required interface = %#v", relation.RequiredInterface)
	}
	if len(relation.TargetFormRefs) != 0 {
		t.Fatalf("an Interface relation also pinned exact Forms: %#v", relation.TargetFormRefs)
	}
}

// TestDerivationRefusesAnUnannotatedReference proves the host-side half of the
// same rule: a Form Definition whose reference states no requirement has
// nothing for a host to verify, so it fails closed rather than resolving.
func TestDerivationRefusesAnUnannotatedReference(t *testing.T) {
	t.Parallel()
	schema := mustDesiredSchema(t, testForm())
	properties := schema["properties"].(map[string]any)
	list := properties["kvBindings"].(map[string]any)
	items := list["items"].(map[string]any)
	reference := items["properties"].(map[string]any)["resource"].(map[string]any)
	delete(reference, RequiredInterfaceAnnotationKey)
	_, err := DeriveRelations(schema)
	if err == nil || !strings.Contains(err.Error(), "states no target contract") {
		t.Fatalf("derive error = %v, want the unannotated-reference refusal", err)
	}
}

type resourceTargetResolverFunc func(ResourceTarget) (ResolvedResourceTarget, error)

func (resolve resourceTargetResolverFunc) ResolveResourceTarget(target ResourceTarget) (ResolvedResourceTarget, error) {
	return resolve(target)
}

// TestCrossFamilyResourceTargetPinsItsOwnGroup proves a reference does not
// inherit the source Form's family. Cross-family references state the exact
// target group, kind, and contract as one ResourceTarget.
func TestCrossFamilyResourceTargetPinsItsOwnGroup(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "queue", Wire: "queue", Kind: KindResourceRef, Required: true,
		Doc: "Queue receiving the delivery.", Example: map[string]any{
			"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs",
		},
		ResourceTarget: &ResourceTarget{
			Group: "queue.forms.takoform.com", Kind: "PullQueue",
			Contract: TargetContract{ExactForm: true},
		},
	})
	schema, err := form.DesiredSchema(stubResolver{})
	if err != nil {
		t.Fatal(err)
	}
	queue := schema["properties"].(map[string]any)["queue"].(map[string]any)
	properties := queue["properties"].(map[string]any)
	if got := properties["apiVersion"].(map[string]any)["const"]; got != "queue.forms.takoform.com" {
		t.Fatalf("apiVersion const = %v, want the ResourceTarget group", got)
	}
}

// TestResolvedExactFormMustMatchTheWholeResourceTarget covers both exact
// identity coordinates a resolver could accidentally look up by kind alone.
func TestResolvedExactFormMustMatchTheWholeResourceTarget(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "queue", Wire: "queue", Kind: KindResourceRef, Required: true,
		Doc: "Queue receiving the delivery.", Example: map[string]any{
			"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs",
		},
		ResourceTarget: &ResourceTarget{
			Group: "queue.forms.takoform.com", Kind: "PullQueue",
			Contract: TargetContract{ExactForm: true},
		},
	})
	for name, mismatch := range map[string]TargetFormRef{
		"wrong group": {
			APIVersion: "topic.forms.takoform.com", Kind: "PullQueue",
			DefinitionVersion: "0.1.0", SchemaDigest: "sha256:" + strings.Repeat("a", 64),
		},
		"wrong kind": {
			APIVersion: "queue.forms.takoform.com", Kind: "DifferentQueue",
			DefinitionVersion: "0.1.0", SchemaDigest: "sha256:" + strings.Repeat("a", 64),
		},
	} {
		name, mismatch := name, mismatch
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := resourceTargetResolverFunc(func(ResourceTarget) (ResolvedResourceTarget, error) {
				return ResolvedResourceTarget{
					ResourceNamePattern: PatternResourceName,
					TargetFormRefs:      []TargetFormRef{mismatch},
				}, nil
			})
			_, err := form.DesiredSchema(resolver)
			if err == nil || !strings.Contains(err.Error(), "does not match ResourceTarget") {
				t.Fatalf("DesiredSchema error = %v, want the exact target mismatch", err)
			}
		})
	}
}

func TestResourceTargetDefaultMustMatchTheExactGroup(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "queue", Wire: "queue", Kind: KindResourceRef,
		Doc: "Queue receiving the delivery.",
		Default: map[string]any{
			"apiVersion": "topic.forms.takoform.com", "kind": "PullQueue", "name": "jobs",
		},
		ResourceTarget: &ResourceTarget{
			Group: "queue.forms.takoform.com", Kind: "PullQueue",
			Contract: TargetContract{ExactForm: true},
		},
	})
	err := form.Validate()
	if err == nil || !strings.Contains(err.Error(), "queue.forms.takoform.com") {
		t.Fatalf("Validate error = %v, want exact target group refusal", err)
	}
}
