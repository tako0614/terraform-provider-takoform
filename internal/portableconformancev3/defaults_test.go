package portableconformancev3

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func exactQueryValues(space string, ref FormRef) url.Values {
	query := url.Values{}
	query.Set("space", space)
	query.Set("group", ref.APIVersion)
	query.Set("kind", ref.Kind)
	query.Set("definitionVersion", ref.DefinitionVersion)
	query.Set("schemaDigest", ref.SchemaDigest)
	return query
}

func resourcePath(contract Contract, ref FormRef, name, action string, query url.Values) string {
	target := contract.APIPath + "/resources/" +
		groupSegments(ref.APIVersion) + "/" + url.PathEscape(ref.Kind) + "/" + url.PathEscape(name)
	if action != "" {
		target += "/" + action
	}
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return target
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type preparedReview struct {
	PrepareDigest string
	SpecDigest    string
}

func hostPrepare(
	t *testing.T,
	server *httptest.Server,
	contract Contract,
	ref FormRef,
	name, space string,
	spec map[string]any,
	fence string,
) preparedReview {
	t.Helper()
	body := mustMarshal(t, map[string]any{
		"apiVersion": ref.APIVersion,
		"kind":       ref.Kind,
		"form":       map[string]any{"formRef": refJSON(ref)},
		"metadata":   map[string]any{"name": name, "space": space},
		"spec":       spec,
	})
	headers := map[string]string{}
	if fence != "" {
		headers[expectedGenerationHeader] = fence
	}
	status, raw := hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", headers, body)
	if status != http.StatusOK {
		t.Fatalf("prepare = %d %s", status, strings.TrimSpace(string(raw)))
	}
	var decoded struct {
		Review preparedReview `json:"review"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Review
}

func hostApply(
	t *testing.T,
	server *httptest.Server,
	contract Contract,
	ref FormRef,
	name, space string,
	spec map[string]any,
	review preparedReview,
	headers map[string]string,
) (int, []byte) {
	t.Helper()
	body := mustMarshal(t, map[string]any{
		"apiVersion": ref.APIVersion,
		"kind":       ref.Kind,
		"form":       map[string]any{"formRef": refJSON(ref)},
		"metadata":   map[string]any{"name": name, "space": space},
		"spec":       spec,
		"review":     map[string]any{"prepareDigest": review.PrepareDigest},
	})
	return hostRequest(t, server, http.MethodPut, resourcePath(contract, ref, name, "", nil), headers, body)
}

func decodeHostResource(t *testing.T, raw []byte) wireResource {
	t.Helper()
	var resource wireResource
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatalf("decode resource: %v (%s)", err, strings.TrimSpace(string(raw)))
	}
	return resource
}

// TestOmittedAndWrittenDefaultsAreOneDesiredState drives the whole portable
// default contract over real HTTP: the two spellings bind the same specDigest,
// and applying the explicit spelling on top of the omitted one is a no-op — no
// generation, no revision, no update. Without materialization at the host entry
// point this is exactly where a client would be told its unchanged
// configuration is a change, forever.
func TestOmittedAndWrittenDefaultsAreOneDesiredState(t *testing.T) {
	host, contract := fallbackHost(t)
	server := httptest.NewServer(host)
	defer server.Close()
	ref := contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef
	const name, space = "queue-probe", "conformance"

	omitted := map[string]any{"messageRetentionSeconds": 345600}
	written := map[string]any{"messageRetentionSeconds": 345600, "deliveryDelaySeconds": 0}

	createReview := hostPrepare(t, server, contract, ref, name, space, omitted, "")
	status, raw := hostApply(t, server, contract, ref, name, space, omitted, createReview, map[string]string{
		"If-None-Match":   "*",
		"Idempotency-Key": "key-create-omitted",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %s", status, strings.TrimSpace(string(raw)))
	}
	created := decodeHostResource(t, raw)
	if created.Metadata.Generation != "1" {
		t.Fatalf("create generation = %s", created.Metadata.Generation)
	}
	// The echo is the MATERIALIZED spec: the client's own desired state now
	// carries the value the Form declared.
	if _, present := created.Spec["deliveryDelaySeconds"]; !present {
		t.Fatalf("apply echo omitted the materialized default: %#v", created.Spec)
	}

	writtenReview := hostPrepare(t, server, contract, ref, name, space, written, created.Metadata.Generation)
	if writtenReview.SpecDigest != createReview.SpecDigest {
		t.Fatalf(
			"specDigest of the written spelling = %s, of the omitted spelling = %s; one desired state must have one digest",
			writtenReview.SpecDigest, createReview.SpecDigest,
		)
	}
	status, raw = hostApply(t, server, contract, ref, name, space, written, writtenReview, map[string]string{
		expectedGenerationHeader: created.Metadata.Generation,
		"Idempotency-Key":        "key-apply-written",
	})
	if status != http.StatusOK {
		t.Fatalf("re-apply = %d %s", status, strings.TrimSpace(string(raw)))
	}
	reapplied := decodeHostResource(t, raw)
	if reapplied.Metadata.Generation != created.Metadata.Generation {
		t.Fatalf(
			"writing the declared default advanced generation %s -> %s",
			created.Metadata.Generation, reapplied.Metadata.Generation,
		)
	}
	if reapplied.Metadata.Revision != created.Metadata.Revision {
		t.Fatalf(
			"writing the declared default advanced revision %s -> %s",
			created.Metadata.Revision, reapplied.Metadata.Revision,
		)
	}
}

// TestSpecChangingApplyToFormWithoutUpdateIsRefused proves the capability rule
// at the wire: a Form whose Definition omits update refuses a spec-changing
// apply, names the missing capability, and leaves the stored resource exactly
// as it was.
func TestSpecChangingApplyToFormWithoutUpdateIsRefused(t *testing.T) {
	host, contract := fallbackHost(t)
	ref := contract.RunnerInput.WorkerVersion.Identity.FormRef
	form := host.catalog.exact(ref)
	if form.declaresUpdate() {
		t.Fatal("this test needs a Form whose Definition omits update")
	}
	const name, space = "worker-version-probe", "conformance"
	group := ref.APIVersion
	stored := map[string]any{
		"worker":   map[string]any{"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker-probe"},
		"bundle":   map[string]any{"apiVersion": group, "kind": "WorkerBundle", "name": "worker-bundle-probe"},
		"handlers": []any{"fetch"},
	}
	storedDigest, err := specCanonicalDigest(stored)
	if err != nil {
		t.Fatal(err)
	}
	host.storeResource(&storedResource{
		Ref: ref, Name: name, Tenant: referencePrimaryAuth.Tenant, Space: space,
		UID: "uid-1", Generation: 1, Revision: 1,
		Spec: stored, SpecDigest: storedDigest,
	})
	// The referenced resources exist: this test is about the capability
	// refusal, and an unresolvable relation would refuse first.
	host.storeResource(&storedResource{
		Ref: mustProbeRef(t, contract, "ModuleWorker"), Name: "module-worker-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: space,
		UID: "uid-2", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	host.storeResource(&storedResource{
		Ref: mustProbeRef(t, contract, "WorkerBundle"), Name: "worker-bundle-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: space,
		UID: "uid-3", Generation: 1, Revision: 1,
		Spec: map[string]any{"manifestDigest": formpackage.DigestBytes([]byte("bundle"))},
	})
	server := httptest.NewServer(host)
	defer server.Close()

	changed := map[string]any{
		"worker":   map[string]any{"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker-probe"},
		"bundle":   map[string]any{"apiVersion": group, "kind": "WorkerBundle", "name": "worker-bundle-probe"},
		"handlers": []any{"fetch", "scheduled"},
	}
	review := hostPrepare(t, server, contract, ref, name, space, changed, "1")
	status, raw := hostApply(t, server, contract, ref, name, space, changed, review, map[string]string{
		expectedGenerationHeader: "1",
		"Idempotency-Key":        "key-no-update-change",
	})
	if status != http.StatusBadRequest || !strings.Contains(string(raw), "invalid_argument") {
		t.Fatalf("spec-changing apply = %d %s, want 400 invalid_argument", status, strings.TrimSpace(string(raw)))
	}
	if !strings.Contains(string(raw), "no update capability") {
		t.Fatalf("refusal does not name the missing capability: %s", strings.TrimSpace(string(raw)))
	}
	current := host.resources[resourceKey(referencePrimaryAuth.scope(space), ref.APIVersion, ref.Kind, name)]
	if current.Generation != 1 || current.Revision != 1 || current.SpecDigest != storedDigest {
		t.Fatalf("the refused apply mutated state: %+v", current)
	}
}

// TestV1Alpha3HostAdvertisesNoRefresh pins the removal on both surfaces a
// client can see: no installed Form declares the capability, and the route
// does not exist.
func TestV1Alpha3HostAdvertisesNoRefresh(t *testing.T) {
	host, contract := fallbackHost(t)
	for key, form := range host.catalog.Forms {
		operations := form.operations()
		if len(operations) == 0 {
			t.Fatalf("installed form %s advertises no operation at all", key)
		}
		for _, operation := range operations {
			if operation == "refresh" {
				t.Errorf("installed form %s declares refresh", key)
			}
		}
	}
	server := httptest.NewServer(host)
	defer server.Close()
	ref := contract.RunnerInput.EdgeKvNamespace.Identity.FormRef
	host.storeResource(&storedResource{
		Ref: ref, Name: "edge-kv-probe", Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-1", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	query := exactQueryValues("conformance", ref)
	status, raw := hostRequest(t, server, http.MethodPost,
		resourcePath(contract, ref, "edge-kv-probe", "refresh", query),
		map[string]string{
			expectedGenerationHeader: "1",
			"Idempotency-Key":        "key-refresh",
		}, nil)
	if status != http.StatusNotFound || !strings.Contains(string(raw), "resource_not_found") {
		t.Fatalf("POST /refresh = %d %s, want 404 resource_not_found", status, strings.TrimSpace(string(raw)))
	}
	// observe, the operation that remains, still works on the same resource.
	observeStatus, observeBody := hostRequest(t, server, http.MethodPost,
		resourcePath(contract, ref, "edge-kv-probe", "observe", query),
		map[string]string{
			expectedGenerationHeader: "1",
			"Idempotency-Key":        "key-observe",
		}, nil)
	if observeStatus != http.StatusOK {
		t.Fatalf("POST /observe = %d %s, want 200", observeStatus, strings.TrimSpace(string(observeBody)))
	}
}

// TestMaterializedNumbersSatisfyHostSemanticRules proves the representation
// choice is not cosmetic: validateDeploymentWeightSum is a real portable rule
// that type-asserts json.Number, and it must accept a value that arrived
// through materialization rather than through the request body.
func TestMaterializedNumbersSatisfyHostSemanticRules(t *testing.T) {
	form := &InstalledForm{
		Ref: FormRef{APIVersion: testEdgeFormsGroup, Kind: "WorkerDeployment"},
		DesiredSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"versions": map[string]any{
					"type": "array",
					// The total is where the rule comes from now: a host reads
					// it off the Definition rather than knowing this kind.
					currentformmodel.SumAnnotationKey: map[string]any{
						"member": "weight", "total": int64(10000),
					},
					"default": []any{map[string]any{
						"workerVersion": map[string]any{"kind": "WorkerVersion", "name": "worker-version-probe"},
						// A plain Go int in the Definition, exactly as an authored
						// default arrives before it reaches the wire.
						"weight": 10000,
					}},
				},
			},
		},
	}
	materialized := form.materialize(nil)
	versions, _ := materialized["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("materialized versions = %#v", materialized["versions"])
	}
	entry, _ := versions[0].(map[string]any)
	if _, ok := entry["weight"].(json.Number); !ok {
		t.Fatalf("materialized weight is %T, want json.Number", entry["weight"])
	}
	if hostErr := validateSummedMembers(form, materialized); hostErr != nil {
		t.Fatalf("a host semantic rule rejected a materialized value: %+v", hostErr)
	}
	// And the rule still rejects a wrong total that arrived the same way.
	form.DesiredSchema["properties"].(map[string]any)["versions"].(map[string]any)["default"] =
		[]any{map[string]any{
			"workerVersion": map[string]any{"kind": "WorkerVersion", "name": "worker-version-probe"},
			"weight":        9999,
		}}
	if hostErr := validateSummedMembers(form, form.materialize(nil)); hostErr == nil {
		t.Fatal("a materialized weight total of 9999 was accepted")
	}
}

// TestHostMaterializationUsesTheSharedImplementation guards against a second,
// divergent copy of the rule appearing inside the host.
func TestHostMaterializationUsesTheSharedImplementation(t *testing.T) {
	host, contract := fallbackHost(t)
	ref := contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef
	form := host.catalog.exact(ref)
	spec := map[string]any{"messageRetentionSeconds": json.Number("345600")}
	fromHost, err := json.Marshal(form.materialize(spec))
	if err != nil {
		t.Fatal(err)
	}
	fromModel, err := json.Marshal(currentformmodel.MaterializeDefaults(form.DesiredSchema, spec))
	if err != nil {
		t.Fatal(err)
	}
	if string(fromHost) != string(fromModel) {
		t.Fatalf("host materialization %s differs from the shared implementation %s", fromHost, fromModel)
	}
}
