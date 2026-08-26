package currentformmodel

import (
	"reflect"
	"strings"
	"testing"
)

type exactFamilyStub struct {
	group     string
	refs      map[string][]TargetFormRef
	relations map[TargetFormRef][]Relation
}

func (s exactFamilyStub) FamilyAPIVersion() string  { return s.group }
func (exactFamilyStub) ResourceNamePattern() string { return PatternResourceName }
func (s exactFamilyStub) TargetFormRefs(kind string) ([]TargetFormRef, error) {
	return append([]TargetFormRef(nil), s.refs[kind]...), nil
}
func (s exactFamilyStub) ExactFormRelations(ref TargetFormRef) ([]Relation, error) {
	return append([]Relation(nil), s.relations[ref]...), nil
}

func TestTargetResolverKeepsSameKindInTwoFamiliesExact(t *testing.T) {
	t.Parallel()
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	a1 := TargetFormRef{APIVersion: "a.forms.example", Kind: "Queue", DefinitionVersion: "0.1.0", SchemaDigest: digest("a")}
	a2 := TargetFormRef{APIVersion: "a.forms.example", Kind: "Queue", DefinitionVersion: "0.2.0", SchemaDigest: digest("b")}
	b1 := TargetFormRef{APIVersion: "b.forms.example", Kind: "Queue", DefinitionVersion: "1.0.0", SchemaDigest: digest("c")}
	aRelation := Relation{Pointer: "/owner", TargetAPIVersion: "identity.forms.example", TargetKind: "Owner"}
	bRelation := Relation{Pointer: "/topic", TargetAPIVersion: "topic.forms.example", TargetKind: "Topic"}
	resolver, err := NewTargetResolver(nil,
		exactFamilyStub{group: "a.forms.example", refs: map[string][]TargetFormRef{"Queue": {a1, a2}}, relations: map[TargetFormRef][]Relation{a1: {aRelation}, a2: {aRelation}}},
		exactFamilyStub{group: "b.forms.example", refs: map[string][]TargetFormRef{"Queue": {b1}}, relations: map[TargetFormRef][]Relation{b1: {bRelation}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		group string
		want  []TargetFormRef
	}{{"a.forms.example", []TargetFormRef{a1, a2}}, {"b.forms.example", []TargetFormRef{b1}}} {
		resolved, err := resolver.ResolveResourceTarget(ResourceTarget{Group: test.group, Kind: "Queue", Contract: TargetContract{ExactForm: true}})
		if err != nil {
			t.Fatalf("resolve %s: %v", test.group, err)
		}
		if !reflect.DeepEqual(resolved.TargetFormRefs, test.want) {
			t.Fatalf("%s refs = %#v, want %#v", test.group, resolved.TargetFormRefs, test.want)
		}
	}
	if got, err := resolver.ResolveExactFormRelations(a1); err != nil || !reflect.DeepEqual(got, []Relation{aRelation}) {
		t.Fatalf("a exact relations = %#v (err %v)", got, err)
	}
	if got, err := resolver.ResolveExactFormRelations(b1); err != nil || !reflect.DeepEqual(got, []Relation{bRelation}) {
		t.Fatalf("b exact relations = %#v (err %v)", got, err)
	}

	for name, ref := range map[string]TargetFormRef{
		"wrong group":   {APIVersion: "missing.forms.example", Kind: "Queue", DefinitionVersion: a1.DefinitionVersion, SchemaDigest: a1.SchemaDigest},
		"wrong version": {APIVersion: a1.APIVersion, Kind: a1.Kind, DefinitionVersion: "latest", SchemaDigest: a1.SchemaDigest},
		"wrong digest":  {APIVersion: a1.APIVersion, Kind: a1.Kind, DefinitionVersion: a1.DefinitionVersion, SchemaDigest: digest("f")},
	} {
		name, ref := name, ref
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := resolver.ResolveExactFormRelations(ref); err == nil {
				t.Fatalf("inexact ref %#v resolved through a latest/default fallback", ref)
			}
		})
	}
}

func TestTargetResolverRefusesAmbiguousOrMalformedFamilyInputs(t *testing.T) {
	t.Parallel()
	first := exactFamilyStub{group: "a.forms.example"}
	if _, err := NewTargetResolver(nil, first, first); err == nil {
		t.Fatal("two sources for one family group were accepted")
	}
	wrong := exactFamilyStub{group: "a.forms.example", refs: map[string][]TargetFormRef{"Queue": {{
		APIVersion: "b.forms.example", Kind: "Queue", DefinitionVersion: "0.1.0", SchemaDigest: "sha256:" + strings.Repeat("a", 64),
	}}}}
	resolver, err := NewTargetResolver(nil, wrong)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveResourceTarget(ResourceTarget{Group: "a.forms.example", Kind: "Queue", Contract: TargetContract{ExactForm: true}}); err == nil {
		t.Fatal("a source returned an exact Definition from another group and the union accepted it")
	}
}
