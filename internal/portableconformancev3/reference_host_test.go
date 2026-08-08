package portableconformancev3

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func fallbackHost(t *testing.T) (*ReferenceHost, Contract) {
	t.Helper()
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	catalog, err := FallbackCatalog(contract)
	if err != nil {
		t.Fatalf("fallback catalog: %v", err)
	}
	return NewReferenceHost(contract, catalog), contract
}

func TestNextResourceMintsAndAdvancesIdentity(t *testing.T) {
	host, contract := fallbackHost(t)
	form := host.catalog.exact(contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef)
	if form == nil {
		t.Fatalf("queue form is not installed")
	}
	firstSpec := map[string]any{"messageRetentionSeconds": json.Number("345600")}
	firstDigest, err := specCanonicalDigest(firstSpec)
	if err != nil {
		t.Fatal(err)
	}
	created := host.nextResource(
		form, nil, true, referencePrimaryAuth.Tenant, "conformance", "queue-probe", firstSpec, firstDigest, false,
	)
	if created.UID != "uid-1" || created.Generation != 1 || created.Revision != 1 {
		t.Fatalf("create identity = %+v", created)
	}
	// A spec change advances generation and revision together.
	secondSpec := map[string]any{"messageRetentionSeconds": json.Number("600000")}
	secondDigest, err := specCanonicalDigest(secondSpec)
	if err != nil {
		t.Fatal(err)
	}
	updated := host.nextResource(
		form, created, false, referencePrimaryAuth.Tenant, "conformance", "queue-probe", secondSpec, secondDigest, false,
	)
	if updated.UID != created.UID || updated.Generation != 2 || updated.Revision != 2 {
		t.Fatalf("spec-change identity = %+v", updated)
	}
	// A byte-identical spec advances neither generation nor revision.
	unchanged := host.nextResource(
		form, updated, false, referencePrimaryAuth.Tenant, "conformance", "queue-probe", secondSpec, secondDigest, false,
	)
	if unchanged.Generation != 2 || unchanged.Revision != 2 {
		t.Fatalf("identical-spec identity = %+v", unchanged)
	}
	// Delete followed by re-create mints a NEW uid for the same name.
	recreated := host.nextResource(
		form, nil, true, referencePrimaryAuth.Tenant, "conformance", "queue-probe", firstSpec, firstDigest, false,
	)
	if recreated.UID == created.UID {
		t.Fatalf("re-create returned the deleted uid %q", recreated.UID)
	}
}

func fenceRequest(t *testing.T, headers map[string]string) mutationFence {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "http://host/apply", nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return mutationFenceOf(request)
}

func TestMutationFences(t *testing.T) {
	host, contract := fallbackHost(t)
	queueRef := contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef
	form := host.catalog.exact(queueRef)
	existing := &storedResource{
		Ref:  queueRef,
		Name: "queue-probe", Space: "conformance",
		UID: "uid-7", Generation: 3, Revision: 5,
	}
	host.storeResource(existing)

	cases := []struct {
		name     string
		form     *InstalledForm
		resource string
		headers  map[string]string
		bodyGen  string
		uid      string
		wantCode string
	}{
		{
			name: "create-conflict", form: form, resource: "queue-probe",
			headers: map[string]string{"If-None-Match": "*"}, wantCode: "generation_conflict",
		},
		{
			name: "create-with-expected-uid", form: form, resource: "fresh-name",
			headers: map[string]string{"If-None-Match": "*"}, uid: "uid-7", wantCode: "uid_mismatch",
		},
		{
			name: "missing-fence", form: form, resource: "queue-probe",
			headers: map[string]string{}, wantCode: "invalid_argument",
		},
		{
			name: "stale-generation", form: form, resource: "queue-probe",
			headers: map[string]string{expectedGenerationHeader: "2"}, wantCode: "generation_conflict",
		},
		{
			name: "conflicting-fences", form: form, resource: "queue-probe",
			headers: map[string]string{expectedGenerationHeader: "3"}, bodyGen: "2",
			wantCode: "invalid_argument",
		},
		{
			name: "uid-mismatch", form: form, resource: "queue-probe",
			headers: map[string]string{expectedGenerationHeader: "3"}, uid: "uid-other",
			wantCode: "uid_mismatch",
		},
		{
			name: "absent-update", form: form, resource: "missing-name",
			headers: map[string]string{expectedGenerationHeader: "1"}, wantCode: "resource_not_found",
		},
	}
	for _, testCase := range cases {
		_, _, hostErr := host.mutationFences(
			fenceRequest(t, testCase.headers), testCase.form,
			"conformance", testCase.resource, testCase.bodyGen, testCase.uid,
		)
		if hostErr == nil || hostErr.Code != testCase.wantCode {
			t.Fatalf("%s: hostErr = %+v, want code %s", testCase.name, hostErr, testCase.wantCode)
		}
	}

	// The exact fence over the existing generation admits the update.
	resolved, create, hostErr := host.mutationFences(
		fenceRequest(t, map[string]string{expectedGenerationHeader: "3"}),
		form, "conformance", "queue-probe", "", "uid-7",
	)
	if hostErr != nil || create || resolved != existing {
		t.Fatalf("exact fence rejected: %+v %v", hostErr, create)
	}
}

// TestRelationInstancesCoverEveryReference is the regression guard for the
// defect this lane had: recognizing a reference by the literal member name
// "resource" found typed bindings only, so worker, bundle, deployment version,
// and dead-letter references were never resolved at all. The derived relations
// find every one of them.
func TestRelationInstancesCoverEveryReference(t *testing.T) {
	host, contract := fallbackHost(t)
	ref := contract.RunnerInput.WorkerVersion.Identity.FormRef
	form := host.catalog.exact(ref)
	group := ref.APIVersion
	spec := map[string]any{
		"worker": map[string]any{"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker-probe"},
		"bundle": map[string]any{"apiVersion": group, "kind": "WorkerBundle", "name": "worker-bundle-probe"},
		"kvBindings": []any{
			map[string]any{
				"name":     "CACHE",
				"resource": map[string]any{"apiVersion": group, "kind": "EdgeKVNamespace", "name": "edge-kv-probe"},
			},
		},
	}
	instances := currentformmodel.RelationInstances(form.Relations, spec)
	got := map[string]string{}
	for _, instance := range instances {
		got[instance.Pointer] = instance.TargetKind + "/" + instance.TargetName
	}
	want := map[string]string{
		"/bundle":                "WorkerBundle/worker-bundle-probe",
		"/kvBindings/0/resource": "EdgeKVNamespace/edge-kv-probe",
		"/worker":                "ModuleWorker/module-worker-probe",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relation instances = %v, want %v", got, want)
	}
	for _, instance := range instances {
		if (instance.Binding != "") != (instance.Pointer == "/kvBindings/0/resource") {
			t.Fatalf("relation %s binding = %q", instance.Pointer, instance.Binding)
		}
	}
}

func TestValidateArtifactManifest(t *testing.T) {
	moduleDigest := formpackage.DigestBytes([]byte("module"))
	valid := artifactManifest{
		APIVersion: artifactAPIVersion,
		Kind:       "WorkerBundle",
		MainModule: "index.js",
		Modules: []artifactModule{{
			Name: "index.js", MediaType: "application/javascript+module",
			Size: json.Number("6"), Digest: moduleDigest,
		}},
	}
	if hostErr := validateArtifactManifest(valid); hostErr != nil {
		t.Fatalf("valid manifest rejected: %+v", hostErr)
	}
	cases := []struct {
		name   string
		mutate func(manifest artifactManifest) artifactManifest
	}{
		{"wrong-api-version", func(m artifactManifest) artifactManifest { m.APIVersion = "other/v1"; return m }},
		{"unknown-kind", func(m artifactManifest) artifactManifest { m.Kind = "OtherBundle"; return m }},
		{"main-module-unlisted", func(m artifactManifest) artifactManifest { m.MainModule = "other.js"; return m }},
		{"bad-path", func(m artifactManifest) artifactManifest {
			m.Modules[0].Name = "../escape.js"
			m.MainModule = "../escape.js"
			return m
		}},
		{"bad-media-type", func(m artifactManifest) artifactManifest {
			m.Modules[0].MediaType = "application/x-unknown"
			return m
		}},
		{"bad-digest", func(m artifactManifest) artifactManifest { m.Modules[0].Digest = "sha256:xyz"; return m }},
		{"duplicate-modules", func(m artifactManifest) artifactManifest {
			m.Modules = append(m.Modules, m.Modules[0])
			return m
		}},
		{"missing-modules", func(m artifactManifest) artifactManifest { m.Modules = nil; return m }},
	}
	for _, testCase := range cases {
		clone := valid
		clone.Modules = append([]artifactModule(nil), valid.Modules...)
		mutated := testCase.mutate(clone)
		if hostErr := validateArtifactManifest(mutated); hostErr == nil || hostErr.Code != "artifact_invalid" {
			t.Fatalf("%s: hostErr = %+v, want artifact_invalid", testCase.name, hostErr)
		}
	}
}

func hostRequest(
	t *testing.T,
	server *httptest.Server,
	method, target string,
	headers map[string]string,
	body []byte,
) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, server.URL+target, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+referencePrimaryToken)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

// TestDeleteOfBoundTargetFailsDependencyInUse drives the live-binding rule
// over real HTTP: a resource referenced by another stored resource's typed
// binding cannot be deleted, and the delete leaves it readable.
func TestDeleteOfBoundTargetFailsDependencyInUse(t *testing.T) {
	host, contract := fallbackHost(t)
	kvRef := contract.RunnerInput.EdgeKvNamespace.Identity.FormRef
	versionRef := contract.RunnerInput.WorkerVersion.Identity.FormRef
	host.storeResource(&storedResource{
		Ref:  kvRef,
		Name: "edge-kv-probe", Space: "conformance",
		UID: "uid-1", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	host.storeResource(&storedResource{
		Ref:  versionRef,
		Name: "worker-version-probe", Space: "conformance",
		UID: "uid-2", Generation: 1, Revision: 1,
		Spec: map[string]any{
			"kvBindings": []any{map[string]any{
				"name": "CACHE",
				"resource": map[string]any{
					"apiVersion": kvRef.APIVersion, "kind": "EdgeKVNamespace", "name": "edge-kv-probe",
				},
			}},
		},
		// The stored relation pins the target UID, which is what the reverse
		// index is keyed by. A holder that stored only the name would not
		// protect this target at all.
		Relations: []storedRelation{{
			Pointer: "/kvBindings/0/resource", Relation: "/kvBindings/*/resource",
			TargetAPIVersion: kvRef.APIVersion, TargetKind: kvRef.Kind,
			TargetName: "edge-kv-probe", TargetUID: "uid-1",
		}},
	})
	server := httptest.NewServer(host)
	defer server.Close()
	query := url.Values{}
	query.Set("space", "conformance")
	query.Set("group", kvRef.APIVersion)
	query.Set("kind", kvRef.Kind)
	query.Set("definitionVersion", kvRef.DefinitionVersion)
	query.Set("schemaDigest", kvRef.SchemaDigest)
	target := contract.APIPath + "/resources/" +
		groupSegments(kvRef.APIVersion) + "/" + kvRef.Kind + "/edge-kv-probe?" + query.Encode()
	status, body := hostRequest(t, server, http.MethodDelete, target, map[string]string{
		"If-Match":        `"1"`,
		"Idempotency-Key": "key-bound-delete",
	}, nil)
	if status != http.StatusConflict || !strings.Contains(string(body), "dependency_in_use") {
		t.Fatalf("bound delete = %d %s, want 409 dependency_in_use", status, strings.TrimSpace(string(body)))
	}
	readStatus, _ := hostRequest(t, server, http.MethodGet, target, nil, nil)
	if readStatus != http.StatusOK {
		t.Fatalf("bound target vanished after rejected delete: HTTP %d", readStatus)
	}
	// After the holder is gone the same fenced delete succeeds.
	host.removeResource(resourceKey("conformance", versionRef.APIVersion, versionRef.Kind, "worker-version-probe"))
	deleteStatus, _ := hostRequest(t, server, http.MethodDelete, target, map[string]string{
		"If-Match":        `"1"`,
		"Idempotency-Key": "key-unbound-delete",
	}, nil)
	if deleteStatus != http.StatusNoContent {
		t.Fatalf("unbound delete = HTTP %d, want 204", deleteStatus)
	}
}

// TestPrepareFenceOnExistingResource proves the prepare precondition
// ("generation-fence-when-updating") over real HTTP: an existing target
// requires the exact update fence, and the minted digest binds the current
// uid and generation, so the create-marker digest differs from the update
// digest.
func TestPrepareFenceOnExistingResource(t *testing.T) {
	host, contract := fallbackHost(t)
	queueRef := contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef
	host.storeResource(&storedResource{
		Ref:  queueRef,
		Name: "queue-probe", Space: "conformance",
		UID: "uid-7", Generation: 3, Revision: 5,
	})
	server := httptest.NewServer(host)
	defer server.Close()
	body, err := json.Marshal(map[string]any{
		"apiVersion": queueRef.APIVersion,
		"kind":       queueRef.Kind,
		"form":       map[string]any{"formRef": refJSON(queueRef)},
		"metadata":   map[string]any{"name": "queue-probe", "space": "conformance"},
		"spec":       map[string]any{"messageRetentionSeconds": 345600},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepareTarget := contract.APIPath + "/resources/prepare"

	missingStatus, missingBody := hostRequest(t, server, http.MethodPost, prepareTarget, nil, body)
	if missingStatus != http.StatusBadRequest || !strings.Contains(string(missingBody), "invalid_argument") {
		t.Fatalf("fence-less prepare = %d %s, want 400 invalid_argument", missingStatus, strings.TrimSpace(string(missingBody)))
	}
	staleStatus, staleBody := hostRequest(t, server, http.MethodPost, prepareTarget, map[string]string{
		expectedGenerationHeader: "2",
	}, body)
	if staleStatus != http.StatusPreconditionFailed || !strings.Contains(string(staleBody), "generation_conflict") {
		t.Fatalf("stale-fence prepare = %d %s, want 412 generation_conflict", staleStatus, strings.TrimSpace(string(staleBody)))
	}
	exactStatus, exactBody := hostRequest(t, server, http.MethodPost, prepareTarget, map[string]string{
		expectedGenerationHeader: "3",
	}, body)
	if exactStatus != http.StatusOK {
		t.Fatalf("exact-fence prepare = %d %s, want 200", exactStatus, strings.TrimSpace(string(exactBody)))
	}
	var prepared struct {
		Review struct {
			PrepareDigest string `json:"prepareDigest"`
			SpecDigest    string `json:"specDigest"`
		} `json:"review"`
	}
	if err := json.Unmarshal(exactBody, &prepared); err != nil {
		t.Fatal(err)
	}
	// The digest binds the CURRENT uid and generation: the same request bound
	// with create markers yields a different digest. The bound spec digest is
	// the digest of the MATERIALIZED spec — the request omitted
	// deliveryDelaySeconds, whose declared default the host filled in.
	specDigest, err := specCanonicalDigest(map[string]any{
		"messageRetentionSeconds": json.Number("345600"),
		"deliveryDelaySeconds":    json.Number("0"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Review.SpecDigest != specDigest {
		t.Fatalf("prepare specDigest = %s, want the materialized %s", prepared.Review.SpecDigest, specDigest)
	}
	_, createDigest, err := prepareBindingPayload(
		specDigest, queueRef, "queue-probe", "conformance", prepareCreateUID, prepareCreateGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, updateDigest, err := prepareBindingPayload(specDigest, queueRef, "queue-probe", "conformance", "uid-7", "3")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Review.PrepareDigest != updateDigest || prepared.Review.PrepareDigest == createDigest {
		t.Fatalf(
			"prepareDigest = %s, want the uid+generation binding %s (create binding %s)",
			prepared.Review.PrepareDigest, updateDigest, createDigest,
		)
	}
}

// TestAcceptedApplyBindsTheIncarnationItWasAcceptedFor proves the apply half of
// the accepted-identity rule over real HTTP. The delete half is proved by the
// required conformance check `async-commit-binds-the-accepted-identity`; an
// accepted UPDATE has no such check because a client cannot mint a prepared
// review for an incarnation that does not exist yet, so the substitution is
// only reachable from out-of-band replacement.
//
// The replacement here keeps the removed resource's name, contract, spec,
// generation, and revision, and differs in exactly one thing: its uid. Every
// fence the operation was accepted under therefore still passes, which is the
// point — an identity is not a fence, and the answer must say what actually
// happened rather than blaming the prepared review.
func TestAcceptedApplyBindsTheIncarnationItWasAcceptedFor(t *testing.T) {
	host, contract := fallbackHost(t)
	queueRef := contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef
	spec := map[string]any{
		"messageRetentionSeconds": json.Number("345600"),
		"deliveryDelaySeconds":    json.Number("0"),
	}
	specDigest, err := specCanonicalDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	accepted := &storedResource{
		Ref:  queueRef,
		Name: "queue-probe", Space: "conformance",
		UID: "uid-7", Generation: 3, Revision: 5,
		Spec: spec, SpecDigest: specDigest,
	}
	host.storeResource(accepted)
	server := httptest.NewServer(host)
	defer server.Close()

	document := map[string]any{
		"apiVersion": queueRef.APIVersion,
		"kind":       queueRef.Kind,
		"form":       map[string]any{"formRef": refJSON(queueRef)},
		"metadata":   map[string]any{"name": "queue-probe", "space": "conformance"},
		"spec":       spec,
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	prepareStatus, prepareBody := hostRequest(t, server, http.MethodPost,
		contract.APIPath+"/resources/prepare",
		map[string]string{expectedGenerationHeader: "3"}, body)
	if prepareStatus != http.StatusOK {
		t.Fatalf("prepare = %d %s, want 200", prepareStatus, strings.TrimSpace(string(prepareBody)))
	}
	var prepared struct {
		Review struct {
			PrepareDigest string `json:"prepareDigest"`
		} `json:"review"`
	}
	if err := json.Unmarshal(prepareBody, &prepared); err != nil {
		t.Fatal(err)
	}
	document["review"] = map[string]any{"prepareDigest": prepared.Review.PrepareDigest}
	applyBody, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	applyStatus, applyResponse := hostRequest(t, server, http.MethodPut,
		contract.APIPath+"/resources/"+groupSegments(queueRef.APIVersion)+"/"+queueRef.Kind+"/queue-probe",
		map[string]string{
			expectedGenerationHeader: "3",
			"Idempotency-Key":        "key-accepted-apply",
			ErrorProbeHeader:         ProbeAsync,
		}, applyBody)
	if applyStatus != http.StatusAccepted {
		t.Fatalf("async apply = %d %s, want 202", applyStatus, strings.TrimSpace(string(applyResponse)))
	}
	var envelope struct {
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(applyResponse, &envelope); err != nil {
		t.Fatal(err)
	}

	// The incarnation is replaced out of band while the operation is pending.
	replacement := *accepted
	replacement.UID = "uid-9"
	host.removeResource(accepted.key())
	host.storeResource(&replacement)

	var terminal struct {
		Done  bool `json:"done"`
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
		Result map[string]any `json:"result"`
	}
	for poll := 0; poll < asyncOperationPolls; poll++ {
		status, pollBody := hostRequest(t, server, http.MethodGet,
			contract.APIPath+"/operations/"+url.PathEscape(envelope.Operation.ID), nil, nil)
		if status != http.StatusOK {
			t.Fatalf("operation poll = %d %s, want 200", status, strings.TrimSpace(string(pollBody)))
		}
		if err := json.Unmarshal(pollBody, &terminal); err != nil {
			t.Fatal(err)
		}
	}
	if !terminal.Done || terminal.Result != nil || terminal.Error.Code != "uid_mismatch" {
		t.Fatalf("terminal operation = %+v, want a uid_mismatch error", terminal)
	}
	if terminal.Error.Retryable {
		t.Fatalf("uid_mismatch was reported as automatically retryable")
	}
	current := host.resources[replacement.key()]
	if current == nil || current.UID != "uid-9" || current.Generation != 3 || current.Revision != 5 {
		t.Fatalf("the accepted apply rewrote the replacement it was never accepted for: %+v", current)
	}
}

// TestStaleRevisionDeleteRejected proves the If-Match revision fence over
// real HTTP.
func TestStaleRevisionDeleteRejected(t *testing.T) {
	host, contract := fallbackHost(t)
	kvRef := contract.RunnerInput.EdgeKvNamespace.Identity.FormRef
	host.storeResource(&storedResource{
		Ref:  kvRef,
		Name: "edge-kv-probe", Space: "conformance",
		UID: "uid-1", Generation: 1, Revision: 4, Spec: map[string]any{},
	})
	server := httptest.NewServer(host)
	defer server.Close()
	query := url.Values{}
	query.Set("space", "conformance")
	query.Set("group", kvRef.APIVersion)
	query.Set("kind", kvRef.Kind)
	query.Set("definitionVersion", kvRef.DefinitionVersion)
	query.Set("schemaDigest", kvRef.SchemaDigest)
	target := contract.APIPath + "/resources/" +
		groupSegments(kvRef.APIVersion) + "/" + kvRef.Kind + "/edge-kv-probe?" + query.Encode()
	status, body := hostRequest(t, server, http.MethodDelete, target, map[string]string{
		"If-Match":        `"3"`,
		"Idempotency-Key": "key-stale-delete",
	}, nil)
	if status != http.StatusPreconditionFailed || !strings.Contains(string(body), "revision_conflict") {
		t.Fatalf("stale delete = %d %s, want 412 revision_conflict", status, strings.TrimSpace(string(body)))
	}
}
