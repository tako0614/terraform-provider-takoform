package currentformmodel

import (
	"reflect"
	"testing"
)

func TestConstraintsTraverseObjectsListsAndTaggedVariants(t *testing.T) {
	t.Parallel()
	form := Form{Fields: []Field{
		{
			HCL: "policy", Wire: "policy", Kind: KindObject,
			Fields: []Field{{
				HCL: "worker", Wire: "worker", Kind: KindResourceRef,
				Exclusive: &ExclusiveHold{KeyedBy: "class"},
			}},
		},
		{
			HCL: "groups", Wire: "groups", Kind: KindObjectList,
			Fields: []Field{
				{HCL: "name", Wire: "name", Kind: KindString, Claimed: true},
				{
					HCL: "weights", Wire: "weights", Kind: KindObjectList,
					Sum: &SummedMember{Member: "weight", Total: 100},
				},
			},
		},
		{
			HCL: "destination", Wire: "destination", Kind: KindTaggedObject,
			Variants: []TaggedObjectVariant{
				{Tag: "queue", Fields: []Field{{HCL: "alias", Wire: "alias", Kind: KindString, Claimed: true}}},
				// The same pointer and marker in another branch is one rule, not
				// two order-dependent copies in the published Definition.
				{Tag: "topic", Fields: []Field{{HCL: "alias", Wire: "alias", Kind: KindString, Claimed: true}}},
			},
		},
	}}
	want := []Constraint{
		{Kind: ConstraintExclusive, Reference: "/policy/worker", KeyedBy: "class"},
		{Kind: ConstraintClaim, Property: "/groups/*/name"},
		{Kind: ConstraintSum, List: "/groups/*/weights", Member: "weight", Total: 100},
		{Kind: ConstraintClaim, Property: "/destination/alias"},
	}
	if got := form.Constraints(); !reflect.DeepEqual(got, want) {
		t.Fatalf("nested constraints = %#v, want %#v", got, want)
	}
}
