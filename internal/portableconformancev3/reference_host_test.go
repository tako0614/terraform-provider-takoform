package portableconformancev3

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
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
	form := host.catalog.form(
		contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef.APIVersion,
		contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef.Kind,
	)
	if form == nil {
		t.Fatalf("queue form is not installed")
	}
	firstSpec := map[string]any{"messageRetentionSeconds": json.Number("345600")}
	firstDigest, err := specCanonicalDigest(firstSpec)
	if err != nil {
		t.Fatal(err)
	}
	created := host.nextResource(form, nil, true, "conformance", "queue-probe", firstSpec, firstDigest, false)
	if created.UID != "uid-1" || created.Generation != 1 || created.Revision != 1 {
		t.Fatalf("create identity = %+v", created)
	}
	// A spec change advances generation and revision together.
	secondSpec := map[string]any{"messageRetentionSeconds": json.Number("600000")}
	secondDigest, err := specCanonicalDigest(secondSpec)
	if err != nil {
		t.Fatal(err)
	}
	updated := host.nextResource(form, created, false, "conformance", "queue-probe", secondSpec, secondDigest, false)
	if updated.UID != created.UID || updated.Generation != 2 || updated.Revision != 2 {
		t.Fatalf("spec-change identity = %+v", updated)
	}
	// A byte-identical spec advances neither generation nor revision.
	unchanged := host.nextResource(form, updated, false, "conformance", "queue-probe", secondSpec, secondDigest, false)
	if unchanged.Generation != 2 || unchanged.Revision != 2 {
		t.Fatalf("identical-spec identity = %+v", unchanged)
	}
	// Delete followed by re-create mints a NEW uid for the same name.
	recreated := host.nextResource(form, nil, true, "conformance", "queue-probe", firstSpec, firstDigest, false)
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
	form := host.catalog.form(queueRef.APIVersion, queueRef.Kind)
	existing := &storedResource{
		Group: queueRef.APIVersion, Kind: queueRef.Kind,
		Name: "queue-probe", Space: "conformance",
		UID: "uid-7", Generation: 3, Revision: 5,
	}
	host.storeResource(existing)

	cases := []struct {
		name     string
		form     *installedForm
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

func TestBindingReferencesScan(t *testing.T) {
	spec := map[string]any{
		"worker": map[string]any{"kind": "ModuleWorker", "name": "module-worker-probe"},
		"bundle": map[string]any{"kind": "WorkerBundle", "name": "worker-bundle-probe"},
		"kvBindings": []any{
			map[string]any{
				"name":     "CACHE",
				"resource": map[string]any{"kind": "EdgeKVNamespace", "name": "edge-kv-probe"},
			},
		},
	}
	references := bindingReferences(spec)
	if len(references) != 1 || references[0] != [2]string{"EdgeKVNamespace", "edge-kv-probe"} {
		t.Fatalf("bindingReferences = %v, want exactly the kvBinding target", references)
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
		Group: kvRef.APIVersion, Kind: kvRef.Kind,
		Name: "edge-kv-probe", Space: "conformance",
		UID: "uid-1", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	host.storeResource(&storedResource{
		Group: versionRef.APIVersion, Kind: versionRef.Kind,
		Name: "worker-version-probe", Space: "conformance",
		UID: "uid-2", Generation: 1, Revision: 1,
		Spec: map[string]any{
			"kvBindings": []any{map[string]any{
				"name":     "CACHE",
				"resource": map[string]any{"kind": "EdgeKVNamespace", "name": "edge-kv-probe"},
			}},
		},
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
		url.PathEscape(kvRef.APIVersion) + "/" + kvRef.Kind + "/edge-kv-probe?" + query.Encode()
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
	delete(host.resources, resourceKey("conformance", versionRef.APIVersion, versionRef.Kind, "worker-version-probe"))
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
		Group: queueRef.APIVersion, Kind: queueRef.Kind,
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

// TestStaleRevisionDeleteRejected proves the If-Match revision fence over
// real HTTP.
func TestStaleRevisionDeleteRejected(t *testing.T) {
	host, contract := fallbackHost(t)
	kvRef := contract.RunnerInput.EdgeKvNamespace.Identity.FormRef
	host.storeResource(&storedResource{
		Group: kvRef.APIVersion, Kind: kvRef.Kind,
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
		url.PathEscape(kvRef.APIVersion) + "/" + kvRef.Kind + "/edge-kv-probe?" + query.Encode()
	status, body := hostRequest(t, server, http.MethodDelete, target, map[string]string{
		"If-Match":        `"3"`,
		"Idempotency-Key": "key-stale-delete",
	}, nil)
	if status != http.StatusPreconditionFailed || !strings.Contains(string(body), "revision_conflict") {
		t.Fatalf("stale delete = %d %s, want 412 revision_conflict", status, strings.TrimSpace(string(body)))
	}
}
