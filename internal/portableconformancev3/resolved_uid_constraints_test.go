package portableconformancev3

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const constraintTestGroup = "constraints.forms.takoform.com"

var constraintTestInterface = formpackage.InterfaceRef{
	APIVersion:   "interfaces.takoform.com/v1alpha1",
	Name:         "constraint.node",
	Version:      "1.0.0",
	SchemaDigest: "sha256:" + strings.Repeat("1", 64),
}

func constraintTestRef(kind, version, digestDigit string) FormRef {
	return FormRef{
		APIVersion:        constraintTestGroup,
		Kind:              kind,
		DefinitionVersion: version,
		SchemaDigest:      "sha256:" + strings.Repeat(digestDigit, 64),
	}
}

func constraintReferenceSchema(kind string, requiredInterface *formpackage.InterfaceRef) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"apiVersion": map[string]any{"type": "string", "const": constraintTestGroup},
			"kind":       map[string]any{"type": "string", "const": kind},
			"name": map[string]any{
				"type": "string", "minLength": 1, "maxLength": 63,
				"pattern": "^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$",
			},
		},
		"required": []any{"apiVersion", "kind", "name"},
	}
	if requiredInterface != nil {
		schema["x-takoform-required-interface"] = map[string]any{
			"apiVersion": requiredInterface.APIVersion,
			"name":       requiredInterface.Name, "version": requiredInterface.Version,
			"schemaDigest": requiredInterface.SchemaDigest,
		}
	}
	return schema
}

func constraintExactReferenceSchema(ref FormRef) map[string]any {
	schema := constraintReferenceSchema(ref.Kind, nil)
	schema["x-takoform-target-formrefs"] = []any{map[string]any{
		"apiVersion": ref.APIVersion, "kind": ref.Kind,
		"definitionVersion": ref.DefinitionVersion, "schemaDigest": ref.SchemaDigest,
	}}
	return schema
}

func closedConstraintSchema(properties map[string]any, required ...string) map[string]any {
	requiredValues := make([]any, 0, len(required))
	for _, name := range required {
		requiredValues = append(requiredValues, name)
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             requiredValues,
	}
}

func installConstraintTestForm(t *testing.T, catalog *Catalog, form *InstalledForm) {
	t.Helper()
	if err := catalog.install(form); err != nil {
		t.Fatalf("install %s: %v", form.Ref.Kind, err)
	}
}

type constraintTestRefs struct {
	Node       FormRef
	NodeV2     FormRef
	Distinct   FormRef
	Unique     FormRef
	UniqueV2   FormRef
	Member     FormRef
	SameTarget FormRef
}

func constraintTestHost(t *testing.T) (*ReferenceHost, Contract, constraintTestRefs) {
	t.Helper()
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify corpus: %v", err)
	}
	catalog := newCatalog(contract.APIVersion)
	catalog.family = constraintTestGroup
	refs := constraintTestRefs{
		Node:       constraintTestRef("ConstraintNode", "0.1.0", "2"),
		NodeV2:     constraintTestRef("ConstraintNode", "0.2.0", "a"),
		Distinct:   constraintTestRef("DistinctPairHolder", "0.1.0", "3"),
		Unique:     constraintTestRef("UniquePairHolder", "0.1.0", "4"),
		UniqueV2:   constraintTestRef("UniquePairHolder", "0.2.0", "5"),
		Member:     constraintTestRef("ConstraintMember", "0.1.0", "6"),
		SameTarget: constraintTestRef("SameTargetHolder", "0.1.0", "7"),
	}
	installConstraintTestForm(t, catalog, &InstalledForm{
		Ref: refs.Node, Role: "identity", Title: "Constraint node",
		DesiredSchema: closedConstraintSchema(map[string]any{
			"next": constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
		}),
		Lifecycle: lifecycleCapabilitiesWithUpdate(),
		ProvidedInterfaces: []formpackage.InterfaceRef{
			constraintTestInterface,
		},
		RequiresHostAPI: contract.APIVersion,
		Constraints:     []formpackage.FormConstraint{{Kind: "acyclic", Reference: "/next"}},
	})
	installConstraintTestForm(t, catalog, &InstalledForm{
		Ref: refs.NodeV2, Role: "identity", Title: "Constraint node second exact Form",
		DesiredSchema: closedConstraintSchema(map[string]any{
			"next": constraintReferenceSchema(refs.NodeV2.Kind, &constraintTestInterface),
		}),
		Lifecycle: lifecycleCapabilitiesWithUpdate(),
		ProvidedInterfaces: []formpackage.InterfaceRef{
			constraintTestInterface,
		},
		RequiresHostAPI: contract.APIVersion,
		Constraints:     []formpackage.FormConstraint{{Kind: "acyclic", Reference: "/next"}},
	})
	installConstraintTestForm(t, catalog, &InstalledForm{
		Ref: refs.Distinct, Role: "attachment", Title: "Distinct pair holder",
		DesiredSchema: closedConstraintSchema(map[string]any{
			"left":  constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
			"right": constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
		}, "left"),
		Lifecycle:       lifecycleCapabilitiesWithUpdate(),
		RequiresHostAPI: contract.APIVersion,
		Constraints: []formpackage.FormConstraint{{
			Kind: "distinctPair", References: []string{"/left", "/right"},
		}},
	})
	for _, ref := range []FormRef{refs.Unique, refs.UniqueV2} {
		installConstraintTestForm(t, catalog, &InstalledForm{
			Ref: ref, Role: "attachment", Title: "Unique pair holder",
			DesiredSchema: closedConstraintSchema(map[string]any{
				"left":  constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
				"right": constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
			}, "left", "right"),
			Lifecycle:       lifecycleCapabilitiesWithUpdate(),
			RequiresHostAPI: contract.APIVersion,
			Constraints: []formpackage.FormConstraint{{
				Kind: "uniquePair", References: []string{"/left", "/right"},
			}},
		})
	}
	installConstraintTestForm(t, catalog, &InstalledForm{
		Ref: refs.Member, Role: "revision", Title: "Constraint member",
		DesiredSchema: closedConstraintSchema(map[string]any{
			"through": constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
		}, "through"),
		Lifecycle:       baseLifecycleCapabilities(),
		RequiresHostAPI: contract.APIVersion,
	})
	installConstraintTestForm(t, catalog, &InstalledForm{
		Ref: refs.SameTarget, Role: "deployment", Title: "Same target holder",
		DesiredSchema: closedConstraintSchema(map[string]any{
			"anchor": constraintReferenceSchema(refs.Node.Kind, &constraintTestInterface),
			"members": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 8,
				"items": constraintExactReferenceSchema(refs.Member),
			},
		}, "anchor", "members"),
		Lifecycle:       lifecycleCapabilitiesWithUpdate(),
		RequiresHostAPI: contract.APIVersion,
		Constraints: []formpackage.FormConstraint{{
			Kind: "sameResolvedTarget", Anchor: "/anchor", Members: "/members/*", Through: "/through",
		}},
	})
	return NewReferenceHost(contract, catalog), contract, refs
}

func constraintResourceBody(ref FormRef, name, space string, spec map[string]any) []byte {
	body, _ := json.Marshal(map[string]any{
		"apiVersion": ref.APIVersion,
		"kind":       ref.Kind,
		"form":       map[string]any{"formRef": refJSON(ref)},
		"metadata":   map[string]any{"name": name, "space": space},
		"spec":       spec,
	})
	return body
}

func constraintRef(ref FormRef, name string) map[string]any {
	return map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind, "name": name}
}

func createConstraintResource(
	t *testing.T,
	server *httptest.Server,
	contract Contract,
	ref FormRef,
	name, space string,
	spec map[string]any,
	idempotencyKey string,
) wireResource {
	t.Helper()
	review := hostPrepare(t, server, contract, ref, name, space, spec, "")
	status, raw := hostApply(t, server, contract, ref, name, space, spec, review, map[string]string{
		"If-None-Match":   "*",
		"Idempotency-Key": idempotencyKey,
	})
	if status != http.StatusCreated {
		t.Fatalf("create %s/%s = %d %s", ref.Kind, name, status, strings.TrimSpace(string(raw)))
	}
	return decodeHostResource(t, raw)
}

func expectConstraintPrepareError(
	t *testing.T,
	server *httptest.Server,
	contract Contract,
	ref FormRef,
	name, space string,
	spec map[string]any,
	fence, kind string,
) {
	t.Helper()
	headers := map[string]string{}
	if fence != "" {
		headers[expectedGenerationHeader] = fence
	}
	status, raw := hostRequest(
		t, server, http.MethodPost, contract.APIPath+"/resources/prepare", headers,
		constraintResourceBody(ref, name, space, spec),
	)
	if status != http.StatusBadRequest || !strings.Contains(string(raw), kind) {
		t.Fatalf("prepare %s = %d %s, want 400 naming %s", name, status, strings.TrimSpace(string(raw)), kind)
	}
}

func TestDistinctPairIsInactiveWhenOptionalOperandIsMissingAndRejectsOneUID(t *testing.T) {
	host, contract, refs := constraintTestHost(t)
	host.storeResource(&storedResource{
		Ref: refs.Node, Name: "node-a", Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-node-a", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	server := httptest.NewServer(host)
	defer server.Close()

	missingRight := constraintResourceBody(refs.Distinct, "holder-missing", "conformance", map[string]any{
		"left": constraintRef(refs.Node, "node-a"),
	})
	status, body := hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", nil, missingRight)
	if status != http.StatusOK {
		t.Fatalf("prepare with absent optional right = %d %s, want 200", status, strings.TrimSpace(string(body)))
	}

	sameUID := constraintResourceBody(refs.Distinct, "holder-same", "conformance", map[string]any{
		"left":  constraintRef(refs.Node, "node-a"),
		"right": constraintRef(refs.Node, "node-a"),
	})
	status, body = hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", nil, sameUID)
	if status != http.StatusBadRequest || !strings.Contains(string(body), "distinctPair") {
		t.Fatalf("prepare with one resolved UID = %d %s, want 400 distinctPair", status, strings.TrimSpace(string(body)))
	}
	status, body = hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/validate", nil, sameUID)
	if status != http.StatusOK || !strings.Contains(string(body), `"valid":false`) || !strings.Contains(string(body), "distinctPair") {
		t.Fatalf("validate with one resolved UID = %d %s, want valid=false distinctPair diagnostic", status, strings.TrimSpace(string(body)))
	}
}

func TestUniquePairIsExactFormScopedOrderedAndUIDBased(t *testing.T) {
	host, contract, refs := constraintTestHost(t)
	server := httptest.NewServer(host)
	defer server.Close()
	createConstraintResource(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{}, "key-unique-node-a")
	createConstraintResource(t, server, contract, refs.Node, "node-b", "conformance", map[string]any{}, "key-unique-node-b")
	pair := map[string]any{
		"left": constraintRef(refs.Node, "node-a"), "right": constraintRef(refs.Node, "node-b"),
	}
	missingRight := constraintResourceBody(refs.Unique, "holder-missing-right", "conformance", map[string]any{
		"left": constraintRef(refs.Node, "node-a"),
	})
	status, raw := hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/validate", nil, missingRight)
	if status != http.StatusOK || !strings.Contains(string(raw), `"valid":false`) {
		t.Fatalf("uniquePair missing required operand validate = %d %s", status, strings.TrimSpace(string(raw)))
	}
	status, raw = hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", nil, missingRight)
	if status != http.StatusBadRequest {
		t.Fatalf("uniquePair missing required operand prepare = %d %s, want 400", status, strings.TrimSpace(string(raw)))
	}
	createConstraintResource(t, server, contract, refs.Unique, "holder-one", "conformance", pair, "key-unique-holder-one")
	expectConstraintPrepareError(t, server, contract, refs.Unique, "holder-two", "conformance", pair, "", "uniquePair")
	// Space does not partition the exact-Form hold. Preserve the literal UID
	// operands in a second Space to make that scope observable even on a host
	// whose ordinary UID allocator never reuses text.
	for _, name := range []string{"node-a", "node-b"} {
		primary := host.resources[resourceKey(referencePrimaryAuth.scope("conformance"), refs.Node.APIVersion, refs.Node.Kind, name)]
		otherSpace := *primary
		otherSpace.Space = "other-space"
		host.storeResource(&otherSpace)
	}
	expectConstraintPrepareError(t, server, contract, refs.Unique, "holder-other-space", "other-space", pair, "", "uniquePair")

	// Pair order and exact FormRef are both part of the key.
	reversed := map[string]any{
		"left": constraintRef(refs.Node, "node-b"), "right": constraintRef(refs.Node, "node-a"),
	}
	createConstraintResource(t, server, contract, refs.Unique, "holder-reversed", "conformance", reversed, "key-unique-holder-reversed")
	createConstraintResource(t, server, contract, refs.UniqueV2, "holder-other-form", "conformance", pair, "key-unique-holder-other-form")
	// UIDs are opaque inside a tenant; a host may mint the same text in two
	// tenant partitions. Reusing the literal pair in another tenant must not
	// expose or collide with the primary tenant's holder.
	for _, name := range []string{"node-a", "node-b"} {
		primary := host.resources[resourceKey(referencePrimaryAuth.scope("conformance"), refs.Node.APIVersion, refs.Node.Kind, name)]
		foreign := *primary
		foreign.Tenant = referenceOtherTenantAuth.Tenant
		host.storeResource(&foreign)
	}
	status, raw = hostRequest(
		t, server, http.MethodPost, contract.APIPath+"/resources/prepare",
		map[string]string{"Authorization": "Bearer " + referenceAlternateTenantToken},
		constraintResourceBody(refs.Unique, "holder-other-tenant", "conformance", pair),
	)
	if status != http.StatusOK {
		t.Fatalf("same UID pair in another tenant = %d %s, want 200", status, strings.TrimSpace(string(raw)))
	}

	// Reusing the same two names with fresh UIDs produces a different pair. The
	// old holder deliberately remains live and drifted while the replacement
	// pair is admitted, proving the index is not name-based.
	host.removeResource(resourceKey(referencePrimaryAuth.scope("conformance"), refs.Node.APIVersion, refs.Node.Kind, "node-a"))
	host.removeResource(resourceKey(referencePrimaryAuth.scope("conformance"), refs.Node.APIVersion, refs.Node.Kind, "node-b"))
	createConstraintResource(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{}, "key-unique-node-a-recreated")
	createConstraintResource(t, server, contract, refs.Node, "node-b", "conformance", map[string]any{}, "key-unique-node-b-recreated")
	createConstraintResource(t, server, contract, refs.Unique, "holder-recreated-pair", "conformance", pair, "key-unique-holder-recreated-pair")
}

func TestAcyclicRejectsSelfCyclesReplacementDriftAndRevalidatesAsyncCommit(t *testing.T) {
	host, contract, refs := constraintTestHost(t)
	server := httptest.NewServer(host)
	defer server.Close()
	expectConstraintPrepareError(t, server, contract, refs.Node, "new-self", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "new-self"),
	}, "", "acyclic")
	createConstraintResource(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{}, "key-cycle-node-a")
	createConstraintResource(t, server, contract, refs.NodeV2, "exact-boundary", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "node-a"),
	}, "key-cycle-exact-boundary")
	boundaryBody := constraintResourceBody(refs.Node, "node-a", "conformance", map[string]any{
		"next": constraintRef(refs.NodeV2, "exact-boundary"),
	})
	status, raw := hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", map[string]string{
		expectedGenerationHeader: "1",
	}, boundaryBody)
	if status != http.StatusOK {
		t.Fatalf("acyclic exact-Form boundary prepare = %d %s, want 200", status, strings.TrimSpace(string(raw)))
	}
	createConstraintResource(t, server, contract, refs.Node, "node-b", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "node-a"),
	}, "key-cycle-node-b")
	expectConstraintPrepareError(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "node-b"),
	}, "1", "acyclic")
	expectConstraintPrepareError(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "node-a"),
	}, "1", "acyclic")

	createConstraintResource(t, server, contract, refs.Node, "drift-target", "conformance", map[string]any{}, "key-drift-target")
	createConstraintResource(t, server, contract, refs.Node, "drift-holder", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "drift-target"),
	}, "key-drift-holder")
	host.removeResource(resourceKey(referencePrimaryAuth.scope("conformance"), refs.Node.APIVersion, refs.Node.Kind, "drift-target"))
	createConstraintResource(t, server, contract, refs.Node, "drift-target", "conformance", map[string]any{}, "key-drift-target-recreated")
	expectConstraintPrepareError(t, server, contract, refs.Node, "drift-source", "conformance", map[string]any{
		"next": constraintRef(refs.Node, "drift-holder"),
	}, "", "acyclic")

	createConstraintResource(t, server, contract, refs.Node, "async-a", "conformance", map[string]any{}, "key-async-a")
	createConstraintResource(t, server, contract, refs.Node, "async-b", "conformance", map[string]any{}, "key-async-b")
	asyncSpec := map[string]any{"next": constraintRef(refs.Node, "async-b")}
	review := hostPrepare(t, server, contract, refs.Node, "async-a", "conformance", asyncSpec, "1")
	status, raw = hostApply(t, server, contract, refs.Node, "async-a", "conformance", asyncSpec, review, map[string]string{
		expectedGenerationHeader: "1", "Idempotency-Key": "key-async-a-update", ErrorProbeHeader: ProbeAsync,
	})
	if status != http.StatusAccepted {
		t.Fatalf("async apply = %d %s", status, strings.TrimSpace(string(raw)))
	}
	var accepted struct {
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatal(err)
	}
	bSpec := map[string]any{"next": constraintRef(refs.Node, "async-a")}
	bReview := hostPrepare(t, server, contract, refs.Node, "async-b", "conformance", bSpec, "1")
	status, raw = hostApply(t, server, contract, refs.Node, "async-b", "conformance", bSpec, bReview, map[string]string{
		expectedGenerationHeader: "1", "Idempotency-Key": "key-async-b-update",
	})
	if status != http.StatusOK {
		t.Fatalf("intervening apply = %d %s", status, strings.TrimSpace(string(raw)))
	}
	operationURL := contract.APIPath + "/operations/" + url.PathEscape(accepted.Operation.ID)
	for poll := 0; poll < asyncOperationPolls; poll++ {
		status, raw = hostRequest(t, server, http.MethodGet, operationURL, nil, nil)
	}
	if status != http.StatusOK || !strings.Contains(string(raw), `"code":"invalid_argument"`) || !strings.Contains(string(raw), "acyclic") {
		t.Fatalf("async terminal = %d %s, want invalid_argument acyclic", status, strings.TrimSpace(string(raw)))
	}
}

func TestAcyclicTraversalFailsClosedAtItsBound(t *testing.T) {
	host, _, refs := constraintTestHost(t)
	form := host.catalog.exact(refs.Node)
	const count = maxResolvedUIDConstraintTraversal + 1
	for index := 0; index < count; index++ {
		name := fmt.Sprintf("bounded-%03d", index)
		resource := &storedResource{
			Ref: refs.Node, Name: name, Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
			UID: fmt.Sprintf("uid-bounded-%03d", index), Generation: 1, Revision: 1,
		}
		if index+1 < count {
			resource.Relations = []storedRelation{{
				Pointer: "/next", Relation: "/next", TargetAPIVersion: refs.Node.APIVersion,
				TargetKind: refs.Node.Kind, TargetName: fmt.Sprintf("bounded-%03d", index+1),
				TargetUID: fmt.Sprintf("uid-bounded-%03d", index+1), TargetRef: refs.Node,
			}}
		}
		host.storeResource(resource)
	}
	first := storedRelation{
		Pointer: "/next", Relation: "/next", TargetAPIVersion: refs.Node.APIVersion,
		TargetKind: refs.Node.Kind, TargetName: "bounded-000", TargetUID: "uid-bounded-000", TargetRef: refs.Node,
	}
	hostErr := host.validateResolvedUIDConstraints(form, referencePrimaryAuth.scope("conformance"), "new-source", []storedRelation{first})
	if hostErr == nil || hostErr.Code != "invalid_argument" || !strings.Contains(hostErr.Message, "bounded") {
		t.Fatalf("bounded traversal error = %+v", hostErr)
	}
}

func TestSameResolvedTargetUsesEveryMembersPinnedThroughUID(t *testing.T) {
	host, contract, refs := constraintTestHost(t)
	server := httptest.NewServer(host)
	defer server.Close()
	createConstraintResource(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{}, "key-same-node-a")
	createConstraintResource(t, server, contract, refs.Node, "node-b", "conformance", map[string]any{}, "key-same-node-b")
	for _, name := range []string{"member-one", "member-two"} {
		createConstraintResource(t, server, contract, refs.Member, name, "conformance", map[string]any{
			"through": constraintRef(refs.Node, "node-a"),
		}, "key-same-"+name)
	}
	valid := map[string]any{
		"anchor":  constraintRef(refs.Node, "node-a"),
		"members": []any{constraintRef(refs.Member, "member-one"), constraintRef(refs.Member, "member-two")},
	}
	createConstraintResource(t, server, contract, refs.SameTarget, "same-holder", "conformance", valid, "key-same-holder")
	invalid := map[string]any{
		"anchor":  constraintRef(refs.Node, "node-b"),
		"members": []any{constraintRef(refs.Member, "member-one"), constraintRef(refs.Member, "member-two")},
	}
	expectConstraintPrepareError(t, server, contract, refs.SameTarget, "different-holder", "conformance", invalid, "", "sameResolvedTarget")

	// A missing or ambiguous through declaration and a damaged stored concrete
	// operand all fail closed. None may turn the declared rule into a no-op.
	memberForm := host.catalog.exact(refs.Member)
	declaredRelations := append([]currentformmodel.Relation(nil), memberForm.Relations...)
	memberForm.Relations = nil
	expectConstraintPrepareError(t, server, contract, refs.SameTarget, "missing-through-declaration", "conformance", valid, "", "sameResolvedTarget")
	memberForm.Relations = append(append([]currentformmodel.Relation(nil), declaredRelations...), declaredRelations[0])
	expectConstraintPrepareError(t, server, contract, refs.SameTarget, "multiple-through-declarations", "conformance", valid, "", "sameResolvedTarget")
	memberForm.Relations = declaredRelations
	memberKey := resourceKey(referencePrimaryAuth.scope("conformance"), refs.Member.APIVersion, refs.Member.Kind, "member-one")
	memberOne := host.resources[memberKey]
	storedRelations := append([]storedRelation(nil), memberOne.Relations...)
	memberOne.Relations = nil
	expectConstraintPrepareError(t, server, contract, refs.SameTarget, "missing-concrete-through", "conformance", valid, "", "sameResolvedTarget")
	memberOne.Relations = storedRelations

	// The same target name with a fresh UID cannot satisfy a member still
	// pinned to the deleted incarnation.
	host.removeResource(resourceKey(referencePrimaryAuth.scope("conformance"), refs.Node.APIVersion, refs.Node.Kind, "node-a"))
	createConstraintResource(t, server, contract, refs.Node, "node-a", "conformance", map[string]any{}, "key-same-node-a-recreated")
	expectConstraintPrepareError(t, server, contract, refs.SameTarget, "drift-holder", "conformance", map[string]any{
		"anchor":  constraintRef(refs.Node, "node-a"),
		"members": []any{constraintRef(refs.Member, "member-one")},
	}, "", "sameResolvedTarget")
}

func TestCatalogRejectsUnknownConstraintKinds(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	catalog := newCatalog(contract.APIVersion)
	catalog.family = constraintTestGroup
	err = catalog.install(&InstalledForm{
		Ref:  constraintTestRef("UnknownConstraintHolder", "0.1.0", "8"),
		Role: "attachment", Title: "Unknown constraint holder",
		DesiredSchema: closedConstraintSchema(map[string]any{}), Lifecycle: baseLifecycleCapabilities(),
		RequiresHostAPI: contract.APIVersion,
		Constraints:     []formpackage.FormConstraint{{Kind: "notAConstraint"}},
	})
	if err == nil || !strings.Contains(err.Error(), "notAConstraint") {
		t.Fatalf("unknown constraint install error = %v", err)
	}
}
