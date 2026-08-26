package portableconformancev3

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestGenericMemoryConstraintStructuralKinds(t *testing.T) {
	tests := []struct {
		name       string
		constraint formpackage.FormConstraint
		desired    map[string]any
		want       string
	}{
		{
			name:       "sum accepts exact integer total",
			constraint: formpackage.FormConstraint{Kind: "sum", List: "/weights", Member: "weight", Total: 100},
			desired:    map[string]any{"weights": []any{map[string]any{"weight": json.Number("40")}, map[string]any{"weight": json.Number("60")}}},
			want:       "ok",
		},
		{
			name:       "sum rejects wrong total",
			constraint: formpackage.FormConstraint{Kind: "sum", List: "/weights", Member: "weight", Total: 100},
			desired:    map[string]any{"weights": []any{map[string]any{"weight": json.Number("40")}, map[string]any{"weight": json.Number("59")}}},
			want:       "invalid_argument",
		},
		{
			name:       "sum rejects fractional member",
			constraint: formpackage.FormConstraint{Kind: "sum", List: "/weights", Member: "weight", Total: 100},
			desired:    map[string]any{"weights": []any{map[string]any{"weight": json.Number("40.5")}, map[string]any{"weight": json.Number("59.5")}}},
			want:       "invalid_argument",
		},
		{
			name:       "sum treats omitted optional list as empty",
			constraint: formpackage.FormConstraint{Kind: "sum", List: "/weights", Member: "weight", Total: 0},
			desired:    map[string]any{},
			want:       "ok",
		},
		{
			name:       "hostAssigned accepts omitted output",
			constraint: formpackage.FormConstraint{Kind: "hostAssigned", Output: "/address"},
			desired:    map[string]any{},
			want:       "ok",
		},
		{
			name:       "hostAssigned rejects desired substitute",
			constraint: formpackage.FormConstraint{Kind: "hostAssigned", Output: "/address"},
			desired:    map[string]any{"address": "https://example.test/"},
			want:       "invalid_argument",
		},
		{
			name:       "hostAssigned follows escaped object and array pointer",
			constraint: formpackage.FormConstraint{Kind: "hostAssigned", Output: "/items/0/a~1b~0c"},
			desired: map[string]any{"items": []any{
				map[string]any{"a/b~c": "caller substitute"},
			}},
			want: "invalid_argument",
		},
		{
			name:       "orderedPair accepts equal numbers",
			constraint: formpackage.FormConstraint{Kind: "orderedPair", References: []string{"/minimum", "/maximum"}},
			desired:    map[string]any{"minimum": json.Number("1.0"), "maximum": json.Number("1")},
			want:       "ok",
		},
		{
			name:       "orderedPair rejects reversed numbers",
			constraint: formpackage.FormConstraint{Kind: "orderedPair", References: []string{"/minimum", "/maximum"}},
			desired:    map[string]any{"minimum": json.Number("2"), "maximum": json.Number("1")},
			want:       "invalid_argument",
		},
		{
			name:       "orderedPair rejects a non-JSON numeric spelling",
			constraint: formpackage.FormConstraint{Kind: "orderedPair", References: []string{"/minimum", "/maximum"}},
			desired:    map[string]any{"minimum": json.Number("01"), "maximum": json.Number("2")},
			want:       "invalid_argument",
		},
		{
			name:       "uniqueBy accepts typed distinct scalars",
			constraint: formpackage.FormConstraint{Kind: "uniqueBy", List: "/indexes", Member: "name"},
			desired:    map[string]any{"indexes": []any{map[string]any{"name": "1"}, map[string]any{"name": json.Number("1")}, map[string]any{"name": true}}},
			want:       "ok",
		},
		{
			name:       "uniqueBy equates numeric spellings",
			constraint: formpackage.FormConstraint{Kind: "uniqueBy", List: "/indexes", Member: "name"},
			desired:    map[string]any{"indexes": []any{map[string]any{"name": json.Number("1")}, map[string]any{"name": json.Number("1.0")}}},
			want:       "invalid_argument",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := genericConstraintTestRef("Structural")
			adapter := genericConstraintTestAdapter(ref, genericConstraintStructuralDefinition(test.constraint))
			input := genericResourceInput{Ref: ref, Name: "candidate", Space: "space", Desired: test.desired}
			if got := genericValidateMemoryConstraints(adapter, genericMemoryActor{tenant: "tenant-a"}, genericMemoryAddressKey("tenant-a", "space", ref.APIVersion, ref.Kind, input.Name), input, test.desired, nil); got != test.want {
				t.Fatalf("constraint result = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGenericMemoryConstraintValueSemantics(t *testing.T) {
	document := map[string]any{
		"a/b~c": []any{map[string]any{"value": json.Number("1e0")}},
	}
	value, present := genericMemoryPointerValue(document, "/a~1b~0c/0/value")
	if !present || value != json.Number("1e0") {
		t.Fatalf("escaped array pointer = %#v/%v, want literal 1e0", value, present)
	}
	for _, pointer := range []string{
		"/a~1b~0c/00/value", // non-canonical array index
		"/a~1b~0c/1/value",  // out of bounds
		"/a~2b",             // invalid RFC 6901 escape
	} {
		if value, present := genericMemoryPointerValue(document, pointer); present {
			t.Fatalf("invalid pointer %q resolved to %#v", pointer, value)
		}
	}

	wantNumber, ok := genericMemoryScalarKey(json.Number("1"))
	if !ok {
		t.Fatal("canonical number 1 was rejected")
	}
	for _, spelling := range []json.Number{"1.0", "1e0", "10e-1"} {
		got, ok := genericMemoryScalarKey(spelling)
		if !ok || got != wantNumber {
			t.Fatalf("numeric spelling %q key = %q/%v, want %q", spelling, got, ok, wantNumber)
		}
	}
	for _, spelling := range []json.Number{"01", "+1", "1.", "1e", "1/1", "0x1"} {
		if got, ok := genericMemoryScalarKey(spelling); ok {
			t.Fatalf("non-JSON number %q produced key %q", spelling, got)
		}
	}
	textKey, ok := genericMemoryScalarKey("1")
	if !ok || textKey == wantNumber {
		t.Fatalf("string and number domains collapsed: text=%q number=%q", textKey, wantNumber)
	}
}

func TestGenericMemoryConstraintExclusiveExactFormTenantScopeAndReplacement(t *testing.T) {
	targetRef := genericConstraintTestRef("Target")
	sourceRef := genericConstraintTestRef("Holder")
	nearRef := sourceRef
	nearRef.SchemaDigest = formpackage.DigestBytes([]byte("near-definition"))
	definition := genericConstraintRelationDefinition(
		[]string{"/target"}, []FormRef{targetRef},
		formpackage.FormConstraint{Kind: "exclusive", Reference: "/target"},
	)
	nearDefinition := genericConstraintRelationDefinition(
		[]string{"/target"}, []FormRef{targetRef},
		formpackage.FormConstraint{Kind: "exclusive", Reference: "/target"},
	)
	adapter := genericConstraintTestAdapter(sourceRef, definition)
	adapter.forms[stableFormRefKey(nearRef)] = genericMemoryForm{ref: nearRef, definition: nearDefinition}
	adapter.forms[stableFormRefKey(targetRef)] = genericMemoryForm{ref: targetRef, definition: formpackage.FormDefinition{DesiredSchema: map[string]any{"type": "object"}}}
	auth := genericMemoryActor{tenant: "tenant-a", principal: "p"}
	target := genericConstraintResource(targetRef, "space", "target", "target#1", map[string]any{})
	adapter.resources[genericMemoryAddressKey(auth.tenant, target.address.Space, targetRef.APIVersion, targetRef.Kind, target.address.Name)] = target
	holder := genericConstraintResource(sourceRef, "space", "holder", "holder#1", map[string]any{"target": genericConstraintTarget("target", targetRef)})
	holderKey := genericMemoryAddressKey(auth.tenant, holder.address.Space, sourceRef.APIVersion, sourceRef.Kind, holder.address.Name)
	adapter.resources[holderKey] = holder
	candidate := genericResourceInput{Ref: sourceRef, Name: "candidate", Space: "space", Desired: map[string]any{"target": genericConstraintTarget("target", targetRef)}}
	candidateKey := genericMemoryAddressKey(auth.tenant, candidate.Space, sourceRef.APIVersion, sourceRef.Kind, candidate.Name)
	genericConstraintPersistPins(t, adapter, auth, holder)
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, candidate.Desired, nil); got != "invalid_argument" {
		t.Fatalf("same exact Form exclusive result = %q, want invalid_argument", got)
	}
	near := genericConstraintResource(nearRef, "space", "near", "near#1", map[string]any{"target": genericConstraintTarget("target", targetRef)})
	nearKey := genericMemoryAddressKey(auth.tenant, near.address.Space, nearRef.APIVersion, nearRef.Kind, near.address.Name)
	adapter.resources[nearKey] = near
	delete(adapter.resources, holderKey)
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, candidate.Desired, nil); got != "ok" {
		t.Fatalf("near exact Form exclusive result = %q, want ok", got)
	}
	otherTenantKey := genericMemoryAddressKey("tenant-b", holder.address.Space, sourceRef.APIVersion, sourceRef.Kind, holder.address.Name)
	adapter.resources[otherTenantKey] = holder
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, candidate.Desired, nil); got != "ok" {
		t.Fatalf("cross-tenant exclusive result = %q, want ok", got)
	}
	if got := genericValidateMemoryConstraints(adapter, auth, nearKey, near.address, near.desired, near); got != "ok" {
		t.Fatalf("replacement/deletion self exclusion result = %q, want ok", got)
	}
}

func TestGenericMemoryConstraintClaimIsTenantWideAcrossSpacesAndKinds(t *testing.T) {
	ref := genericConstraintTestRef("ClaimA")
	otherRef := genericConstraintTestRef("ClaimB")
	definition := formpackage.FormDefinition{
		DesiredSchema: map[string]any{"type": "object"},
		Constraints:   []formpackage.FormConstraint{{Kind: "claim", Property: "/hostname"}},
	}
	otherDefinition := formpackage.FormDefinition{
		DesiredSchema: map[string]any{"type": "object"},
		Constraints:   []formpackage.FormConstraint{{Kind: "claim", Property: "/alias"}},
	}
	adapter := genericConstraintTestAdapter(ref, definition)
	adapter.forms[stableFormRefKey(otherRef)] = genericMemoryForm{ref: otherRef, definition: otherDefinition}
	auth := genericMemoryActor{tenant: "tenant-a", principal: "p"}
	holder := genericConstraintResource(otherRef, "other-space", "holder", "holder#1", map[string]any{"alias": "edge.example"})
	holderKey := genericMemoryAddressKey(auth.tenant, holder.address.Space, otherRef.APIVersion, otherRef.Kind, holder.address.Name)
	adapter.resources[holderKey] = holder
	candidate := genericResourceInput{Ref: ref, Name: "candidate", Space: "space", Desired: map[string]any{"hostname": "edge.example"}}
	candidateKey := genericMemoryAddressKey(auth.tenant, candidate.Space, ref.APIVersion, ref.Kind, candidate.Name)
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, candidate.Desired, nil); got != "invalid_argument" {
		t.Fatalf("cross-kind, cross-property tenant-wide claim result = %q, want invalid_argument", got)
	}
	delete(adapter.resources, holderKey)
	otherTenantKey := genericMemoryAddressKey("tenant-b", holder.address.Space, otherRef.APIVersion, otherRef.Kind, holder.address.Name)
	adapter.resources[otherTenantKey] = holder
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, candidate.Desired, nil); got != "ok" {
		t.Fatalf("cross-tenant claim result = %q, want ok", got)
	}
}

func TestGenericMemoryConstraintAcyclicBoundedAndPinned(t *testing.T) {
	ref := genericConstraintTestRef("Node")
	definition := genericConstraintRelationDefinition([]string{"/next"}, []FormRef{ref}, formpackage.FormConstraint{Kind: "acyclic", Reference: "/next"})
	adapter := genericConstraintTestAdapter(ref, definition)
	auth := genericMemoryActor{tenant: "tenant-a", principal: "p"}
	terminal := genericConstraintResource(ref, "space", "terminal", "terminal#1", map[string]any{})
	adapter.resources[genericMemoryAddressKey(auth.tenant, "space", ref.APIVersion, ref.Kind, "terminal")] = terminal
	positive := genericResourceInput{Ref: ref, Name: "candidate", Space: "space", Desired: map[string]any{"next": genericConstraintTarget("terminal", ref)}}
	positiveKey := genericMemoryAddressKey(auth.tenant, positive.Space, ref.APIVersion, ref.Kind, positive.Name)
	if got := genericValidateMemoryConstraints(adapter, auth, positiveKey, positive, positive.Desired, nil); got != "ok" {
		t.Fatalf("acyclic terminal result = %q, want ok", got)
	}
	replacing := genericConstraintResource(ref, "space", "candidate", "candidate#1", map[string]any{"next": genericConstraintTarget("terminal", ref)})
	replacingKey := genericMemoryAddressKey(auth.tenant, replacing.address.Space, ref.APIVersion, ref.Kind, replacing.address.Name)
	adapter.resources[replacingKey] = replacing
	cycleTarget := genericConstraintResource(ref, "space", "cycle-target", "cycle-target#1", map[string]any{"next": genericConstraintTarget("candidate", ref)})
	cycleTargetKey := genericMemoryAddressKey(auth.tenant, cycleTarget.address.Space, ref.APIVersion, ref.Kind, cycleTarget.address.Name)
	adapter.resources[cycleTargetKey] = cycleTarget
	cycle := replacing.address
	cycle.Desired = map[string]any{"next": genericConstraintTarget("cycle-target", ref)}
	replacing.desired = cycle.Desired
	if got := genericValidateMemoryConstraints(adapter, auth, replacingKey, cycle, cycle.Desired, replacing); got != "invalid_argument" {
		t.Fatalf("acyclic UID cycle result = %q, want invalid_argument", got)
	}
	for index := 0; index <= genericMemoryResolvedTraversalLimit; index++ {
		name := fmt.Sprintf("chain-%03d", index)
		next := map[string]any{}
		if index < genericMemoryResolvedTraversalLimit {
			next = map[string]any{"next": genericConstraintTarget(fmt.Sprintf("chain-%03d", index+1), ref)}
		}
		resource := genericConstraintResource(ref, "bounded", name, name+"#1", next)
		adapter.resources[genericMemoryAddressKey(auth.tenant, "bounded", ref.APIVersion, ref.Kind, name)] = resource
	}
	for index := 0; index <= genericMemoryResolvedTraversalLimit; index++ {
		name := fmt.Sprintf("chain-%03d", index)
		genericConstraintPersistPins(t, adapter, auth, adapter.resources[genericMemoryAddressKey(auth.tenant, "bounded", ref.APIVersion, ref.Kind, name)])
	}
	bounded := genericResourceInput{Ref: ref, Name: "bounded-candidate", Space: "bounded", Desired: map[string]any{"next": genericConstraintTarget("chain-000", ref)}}
	boundedKey := genericMemoryAddressKey(auth.tenant, bounded.Space, ref.APIVersion, ref.Kind, bounded.Name)
	if got := genericValidateMemoryConstraints(adapter, auth, boundedKey, bounded, bounded.Desired, nil); got != "invalid_argument" {
		t.Fatalf("acyclic traversal bound result = %q, want invalid_argument", got)
	}
}

func TestGenericMemoryConstraintDistinctAndUniquePairs(t *testing.T) {
	ref := genericConstraintTestRef("Pair")
	targetRef := genericConstraintTestRef("TargetPair")
	definition := genericConstraintRelationDefinition([]string{"/left", "/right"}, []FormRef{targetRef},
		formpackage.FormConstraint{Kind: "distinctPair", References: []string{"/left", "/right"}},
		formpackage.FormConstraint{Kind: "uniquePair", References: []string{"/left", "/right"}},
	)
	adapter := genericConstraintTestAdapter(ref, definition)
	adapter.forms[stableFormRefKey(targetRef)] = genericMemoryForm{ref: targetRef, definition: formpackage.FormDefinition{DesiredSchema: map[string]any{"type": "object"}}}
	auth := genericMemoryActor{tenant: "tenant-a", principal: "p"}
	for _, name := range []string{"one", "two", "three"} {
		resource := genericConstraintResource(targetRef, "space", name, name+"#1", map[string]any{})
		adapter.resources[genericMemoryAddressKey(auth.tenant, "space", targetRef.APIVersion, targetRef.Kind, name)] = resource
	}
	base := map[string]any{
		"left":  genericConstraintTarget("one", targetRef),
		"right": genericConstraintTarget("two", targetRef),
	}
	candidate := genericResourceInput{Ref: ref, Name: "candidate", Space: "space", Desired: base}
	candidateKey := genericMemoryAddressKey(auth.tenant, candidate.Space, ref.APIVersion, ref.Kind, candidate.Name)
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, base, nil); got != "ok" {
		t.Fatalf("distinct/unique distinct pair result = %q, want ok", got)
	}
	same := map[string]any{"left": genericConstraintTarget("one", targetRef), "right": genericConstraintTarget("one", targetRef)}
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, same, nil); got != "invalid_argument" {
		t.Fatalf("distinct pair equal UID result = %q, want invalid_argument", got)
	}
	holder := genericConstraintResource(ref, "space", "holder", "holder#1", base)
	holderKey := genericMemoryAddressKey(auth.tenant, holder.address.Space, ref.APIVersion, ref.Kind, holder.address.Name)
	adapter.resources[holderKey] = holder
	genericConstraintPersistPins(t, adapter, auth, holder)
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, base, nil); got != "invalid_argument" {
		t.Fatalf("unique pair duplicate result = %q, want invalid_argument", got)
	}
	reverse := map[string]any{"left": genericConstraintTarget("two", targetRef), "right": genericConstraintTarget("one", targetRef)}
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, reverse, nil); got != "ok" {
		t.Fatalf("unique pair reversed order result = %q, want ok", got)
	}
	delete(adapter.resources, holderKey)
	otherTenant := genericMemoryAddressKey("tenant-b", holder.address.Space, ref.APIVersion, ref.Kind, holder.address.Name)
	adapter.resources[otherTenant] = holder
	if got := genericValidateMemoryConstraints(adapter, auth, candidateKey, candidate, base, nil); got != "ok" {
		t.Fatalf("unique pair cross-tenant result = %q, want ok", got)
	}
}

func TestGenericMemoryConstraintSameResolvedTarget(t *testing.T) {
	anchorRef := genericConstraintTestRef("Anchor")
	memberRef := genericConstraintTestRef("Member")
	formRef := genericConstraintTestRef("Source")
	anchorDefinition := genericConstraintRelationDefinition([]string{"/anchor", "/members/*/member"}, []FormRef{anchorRef, memberRef}, formpackage.FormConstraint{
		Kind: "sameResolvedTarget", Anchor: "/anchor", Members: "/members/*/member", Through: "/through",
	})
	memberDefinition := genericConstraintRelationDefinition([]string{"/through"}, []FormRef{anchorRef})
	adapter := genericConstraintTestAdapter(formRef, anchorDefinition)
	adapter.forms[stableFormRefKey(memberRef)] = genericMemoryForm{ref: memberRef, definition: memberDefinition}
	adapter.forms[stableFormRefKey(anchorRef)] = genericMemoryForm{ref: anchorRef, definition: formpackage.FormDefinition{DesiredSchema: map[string]any{"type": "object"}}}
	anchorProperties := anchorDefinition.DesiredSchema["properties"].(map[string]any)
	membersSchema := anchorProperties["members"].(map[string]any)
	memberItems := membersSchema["items"].(map[string]any)
	memberProperties := memberItems["properties"].(map[string]any)
	anchorProperties["anchor"] = genericConstraintRelationSchema([]FormRef{anchorRef})
	memberProperties["member"] = genericConstraintRelationSchema([]FormRef{memberRef})
	anchor := genericConstraintResource(anchorRef, "space", "anchor", "anchor#1", map[string]any{})
	member := genericConstraintResource(memberRef, "space", "member", "member#1", map[string]any{"through": genericConstraintTarget("anchor", anchorRef)})
	auth := genericMemoryActor{tenant: "tenant-a", principal: "p"}
	adapter.resources[genericMemoryAddressKey(auth.tenant, "space", anchorRef.APIVersion, anchorRef.Kind, "anchor")] = anchor
	adapter.resources[genericMemoryAddressKey(auth.tenant, "space", memberRef.APIVersion, memberRef.Kind, "member")] = member
	genericConstraintPersistPins(t, adapter, auth, member)
	desired := map[string]any{
		"anchor":  genericConstraintTarget("anchor", anchorRef),
		"members": []any{map[string]any{"member": genericConstraintTarget("member", memberRef)}},
	}
	input := genericResourceInput{Ref: formRef, Name: "source", Space: "space", Desired: desired}
	key := genericMemoryAddressKey(auth.tenant, input.Space, formRef.APIVersion, formRef.Kind, input.Name)
	if got := genericValidateMemoryConstraints(adapter, auth, key, input, desired, nil); got != "ok" {
		t.Fatalf("sameResolvedTarget equal UID result = %q, want ok", got)
	}
	member.desired = map[string]any{"through": genericConstraintTarget("other", anchorRef)}
	other := genericConstraintResource(anchorRef, "space", "other", "other#1", map[string]any{})
	adapter.resources[genericMemoryAddressKey(auth.tenant, "space", anchorRef.APIVersion, anchorRef.Kind, "other")] = other
	if got := genericValidateMemoryConstraints(adapter, auth, key, input, desired, nil); got != "invalid_argument" {
		t.Fatalf("sameResolvedTarget unequal UID result = %q, want invalid_argument", got)
	}
	missingAnchor := map[string]any{"members": desired["members"]}
	if got := genericValidateMemoryConstraints(adapter, auth, key, input, missingAnchor, nil); got != "invalid_argument" {
		t.Fatalf("sameResolvedTarget missing anchor result = %q, want invalid_argument", got)
	}
}

func genericConstraintTestRef(kind string) FormRef {
	return FormRef{APIVersion: "forms.example/v1", Kind: kind, DefinitionVersion: "1.0.0", SchemaDigest: formpackage.DigestBytes([]byte(kind))}
}

func genericConstraintTestAdapter(ref FormRef, definition formpackage.FormDefinition) *genericMemoryAdapter {
	return &genericMemoryAdapter{
		forms:     map[string]genericMemoryForm{stableFormRefKey(ref): {ref: ref, definition: definition}},
		resources: map[string]*genericMemoryResource{}, nativeClaims: map[string]string{}, replays: map[string]genericMemoryReplay{},
		preparations: map[string]genericMemoryPreparation{}, operations: map[string]*genericMemoryOperation{}, uploads: map[string]*genericMemoryUpload{},
		blobs: map[string]map[string][]byte{}, manifests: map[string]map[string]string{}, incarnations: map[string]int{},
	}
}

func genericConstraintStructuralDefinition(constraint formpackage.FormConstraint) formpackage.FormDefinition {
	properties := map[string]any{}
	required := []any{}
	switch constraint.Kind {
	case "sum":
		properties["weights"] = map[string]any{
			"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{constraint.Member: map[string]any{"type": "integer"}}, "required": []any{constraint.Member},
			},
		}
	case "hostAssigned":
		properties[strings.TrimPrefix(constraint.Output, "/")] = map[string]any{"type": "string"}
	case "orderedPair":
		properties["minimum"] = map[string]any{"type": "number"}
		properties["maximum"] = map[string]any{"type": "number"}
		required = []any{"minimum", "maximum"}
	case "uniqueBy":
		properties["indexes"] = map[string]any{
			"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{constraint.Member: map[string]any{"type": "string"}}, "required": []any{constraint.Member},
			},
		}
	}
	return formpackage.FormDefinition{DesiredSchema: map[string]any{"type": "object", "properties": properties, "required": required}, Constraints: []formpackage.FormConstraint{constraint}}
}

func genericConstraintRelationDefinition(pointers []string, targetRefs []FormRef, constraints ...formpackage.FormConstraint) formpackage.FormDefinition {
	properties := map[string]any{}
	required := []any{}
	for _, pointer := range pointers {
		tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
		if len(tokens) == 1 {
			properties[tokens[0]] = genericConstraintRelationSchema(targetRefs)
			required = append(required, tokens[0])
			continue
		}
		if len(tokens) == 3 && tokens[1] == "*" {
			list, _ := properties[tokens[0]].(map[string]any)
			if list == nil {
				list = map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}}}
				properties[tokens[0]] = list
				required = append(required, tokens[0])
			}
			items := list["items"].(map[string]any)
			itemProperties := items["properties"].(map[string]any)
			itemProperties[tokens[2]] = genericConstraintRelationSchema(targetRefs)
			items["required"] = append(items["required"].([]any), tokens[2])
		}
	}
	return formpackage.FormDefinition{DesiredSchema: map[string]any{"type": "object", "properties": properties, "required": required}, Constraints: constraints}
}

func genericConstraintRelationSchema(targetRefs []FormRef) map[string]any {
	refs := make([]any, 0, len(targetRefs))
	for _, ref := range targetRefs {
		refs = append(refs, map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind, "definitionVersion": ref.DefinitionVersion, "schemaDigest": ref.SchemaDigest})
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"apiVersion": map[string]any{"type": "string", "const": targetRefs[0].APIVersion}, "kind": map[string]any{"type": "string", "const": targetRefs[0].Kind}, "name": map[string]any{"type": "string"}},
		"required":   []any{"apiVersion", "kind", "name"}, "x-takoform-target-formrefs": refs,
	}
}

func genericConstraintTarget(name string, ref FormRef) map[string]any {
	return map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind, "name": name}
}

func genericConstraintResource(ref FormRef, space, name, uid string, desired map[string]any) *genericMemoryResource {
	return &genericMemoryResource{address: genericResourceInput{Ref: ref, Space: space, Name: name, Desired: desired}, uid: uid, generation: 1, revision: 1, desired: desired}
}

func genericConstraintPersistPins(t *testing.T, adapter *genericMemoryAdapter, auth genericMemoryActor, resource *genericMemoryResource) {
	t.Helper()
	key := genericMemoryResourceKeyForStored(auth.tenant, resource)
	pins, code := genericMemoryCaptureRelationPins(adapter, auth, key, resource.address, resource.desired, resource)
	if code != "ok" {
		t.Fatalf("capture relation pins for %s = %q", resource.address.Name, code)
	}
	resource.relationPins = pins
}
