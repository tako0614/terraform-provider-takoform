package provider

// v3_fake_host_test.go is an in-package fake Host API v1alpha3 host: just
// enough of discovery, form availability, prepare/apply/read/delete, one
// 202-Operation path, and the content-addressed artifact upload to drive the
// v3-lane resources end to end. Conventions: namespaced groups travel as one
// %2F-escaped path segment, uids are "uid-N", generations and revisions are
// canonical decimal strings, and the strong ETag is the quoted revision.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

const v3TestAPIRoot = "/apis/forms.takoform.com/v1alpha3"

const v3TestPrepareDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type v3HostRecord struct {
	uid        string
	generation int
	revision   int
	apiVersion string
	kind       string
	form       map[string]any
	space      string
	spec       map[string]any
	outputs    map[string]any
}

type v3FakeHost struct {
	t      *testing.T
	mu     sync.Mutex
	server *httptest.Server

	resources  map[string]*v3HostRecord
	uidCounter int

	// events records the order of side-effecting requests:
	// "artifact-start", "blob:<digest>", "commit", "apply:<Kind>/<name>",
	// "delete:<Kind>/<name>".
	events []string
	// applySpecs and applyHeaders record each PUT apply in order.
	applySpecs   []map[string]any
	applyBodies  []map[string]any
	applyHeaders []http.Header
	getRequests  int

	manifestRaw []byte
	blobs       map[string][]byte

	// resourceQueries records the exact FormRef dispatch of each non-apply
	// resource request as "<METHOD> <group> <schemaDigest>", so a test can prove
	// which FormRef a read or delete actually queried.
	resourceQueries []string

	// apply202 makes the next create return 202 with a polled Operation.
	apply202 bool
	// apply202Pending makes the next create return 202 with an Operation that
	// never reaches a terminal state: the host accepted the mutation and is
	// still working when the caller's deadline expires.
	apply202Pending bool
	// operationResults maps operation id to its terminal result body.
	operationResults map[string]map[string]any
	// pendingOperations maps operation id to the target uid it discloses while
	// staying non-terminal forever.
	pendingOperations map[string]string
}

func newV3FakeHost(t *testing.T) *v3FakeHost {
	t.Helper()
	host := &v3FakeHost{
		t:                 t,
		resources:         map[string]*v3HostRecord{},
		blobs:             map[string][]byte{},
		operationResults:  map[string]map[string]any{},
		pendingOperations: map[string]string{},
	}
	host.server = httptest.NewServer(http.HandlerFunc(host.serve))
	t.Cleanup(host.server.Close)
	return host
}

func (h *v3FakeHost) writeJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Errorf("encoding fake host response: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func (h *v3FakeHost) writeStableError(w http.ResponseWriter, status int, code string) {
	h.writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code": code, "message": "fake host " + code,
			"requestId": "req-1", "retryable": false,
		},
	})
}

func (h *v3FakeHost) discoveryDoc() map[string]any {
	origin := h.server.URL
	return map[string]any{
		"api_versions": []string{clientv3.APIVersion},
		"features": map[string]bool{
			"service_forms": true, "exact_form_ref": true,
			"optimistic_concurrency": true, "idempotent_lifecycle": true,
			"operations": true, "artifact_upload": true, "support_profiles": true,
		},
		"endpoints": map[string]any{"api": origin + v3TestAPIRoot},
	}
}

func (h *v3FakeHost) resourceKey(kind, name string) string { return kind + "/" + name }

func (h *v3FakeHost) wireResource(record *v3HostRecord, name string) map[string]any {
	status := map[string]any{
		"observedGeneration": strconv.Itoa(record.generation),
		"conditions": []map[string]any{{
			"type": "Ready", "status": "True", "reason": "Available",
			"lastTransitionTime": "2026-08-06T00:00:00Z",
		}},
	}
	if len(record.outputs) > 0 {
		status["outputs"] = record.outputs
	}
	return map[string]any{
		"apiVersion": record.apiVersion,
		"kind":       record.kind,
		"form":       record.form,
		"metadata": map[string]any{
			"name": name, "space": record.space,
			"uid":        record.uid,
			"generation": strconv.Itoa(record.generation),
			"revision":   strconv.Itoa(record.revision),
		},
		"spec":   record.spec,
		"status": status,
	}
}

// wireResourceAs renders one record under the exact FormRef named by a read's
// query instead of the identity that created it.
func (h *v3FakeHost) wireResourceAs(record *v3HostRecord, name string, query url.Values) map[string]any {
	if query.Get("group") == "" || query.Get("schemaDigest") == "" {
		return h.wireResource(record, name)
	}
	echoed := *record
	echoed.apiVersion = query.Get("group")
	echoed.kind = query.Get("kind")
	echoed.form = map[string]any{"formRef": map[string]any{
		"apiVersion":        query.Get("group"),
		"kind":              query.Get("kind"),
		"definitionVersion": query.Get("definitionVersion"),
		"schemaDigest":      query.Get("schemaDigest"),
	}}
	return h.wireResource(&echoed, name)
}

func (h *v3FakeHost) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	escaped := r.URL.EscapedPath()
	switch {
	case escaped == clientv3.DiscoveryPath:
		h.writeJSON(w, http.StatusOK, h.discoveryDoc())
	case escaped == v3TestAPIRoot+"/forms" && r.Method == http.MethodGet:
		h.serveForms(w, r)
	case escaped == v3TestAPIRoot+"/resources/validate" && r.Method == http.MethodPost:
		h.writeJSON(w, http.StatusOK, map[string]any{"valid": true, "diagnostics": []any{}})
	case escaped == v3TestAPIRoot+"/resources/prepare" && r.Method == http.MethodPost:
		h.servePrepare(w, r)
	case strings.HasPrefix(escaped, v3TestAPIRoot+"/resources/"):
		h.serveResource(w, r, strings.TrimPrefix(escaped, v3TestAPIRoot+"/resources/"))
	case escaped == v3TestAPIRoot+"/artifacts/uploads" && r.Method == http.MethodPost:
		h.serveUploadStart(w, r)
	case strings.HasPrefix(escaped, v3TestAPIRoot+"/artifacts/uploads/up_1/blobs/") && r.Method == http.MethodPut:
		digest, _ := unescapeSegment(strings.TrimPrefix(escaped, v3TestAPIRoot+"/artifacts/uploads/up_1/blobs/"))
		raw, _ := io.ReadAll(r.Body)
		h.blobs[digest] = raw
		h.events = append(h.events, "blob:"+digest)
		w.WriteHeader(http.StatusCreated)
	case escaped == v3TestAPIRoot+"/artifacts/uploads/up_1/commit" && r.Method == http.MethodPost:
		digest, err := formpackage.DigestCanonicalJSON(h.manifestRaw)
		if err != nil {
			h.t.Errorf("canonicalizing stored manifest: %v", err)
		}
		h.events = append(h.events, "commit")
		h.writeJSON(w, http.StatusOK, map[string]any{"manifestDigest": digest})
	case strings.HasPrefix(escaped, v3TestAPIRoot+"/operations/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(escaped, v3TestAPIRoot+"/operations/")
		if uid, pending := h.pendingOperations[id]; pending {
			body := map[string]any{
				"apiVersion": clientv3.OperationAPIVersion, "kind": clientv3.OperationKind,
				"id": id, "done": false,
			}
			if uid != "" {
				body["target"] = map[string]any{"uid": uid}
			}
			w.Header().Set("Retry-After", "0")
			h.writeJSON(w, http.StatusOK, body)
			return
		}
		result, ok := h.operationResults[id]
		if !ok {
			h.writeStableError(w, http.StatusNotFound, "operation_not_found")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{
			"apiVersion": clientv3.OperationAPIVersion, "kind": clientv3.OperationKind,
			"id": id, "done": true, "result": result,
		})
	default:
		h.t.Errorf("unexpected fake host request %s %s", r.Method, escaped)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}
}

func unescapeSegment(segment string) (string, error) {
	unescaped := strings.ReplaceAll(segment, "%2F", "/")
	return strings.ReplaceAll(unescaped, "%2f", "/"), nil
}

func (h *v3FakeHost) serveForms(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	identity := map[string]any{
		"formRef": map[string]any{
			"apiVersion":        query.Get("group"),
			"kind":              query.Get("kind"),
			"definitionVersion": query.Get("definitionVersion"),
			"schemaDigest":      query.Get("schemaDigest"),
		},
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"forms": []map[string]any{{
			"identity":        identity,
			"definitionKnown": true, "installed": true, "executable": true,
			"activated": true, "availableToPrincipal": true,
			"operations": []string{"create", "read", "update", "delete", "import", "observe", "refresh"},
		}},
	})
}

func (h *v3FakeHost) servePrepare(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var request struct {
		APIVersion string         `json:"apiVersion"`
		Kind       string         `json:"kind"`
		Form       map[string]any `json:"form"`
		Metadata   map[string]any `json:"metadata"`
		Spec       map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		h.t.Errorf("prepare request not JSON: %v", err)
	}
	specRaw, _ := json.Marshal(request.Spec)
	specDigest, err := formpackage.DigestCanonicalJSON(specRaw)
	if err != nil {
		h.t.Errorf("digesting prepare spec: %v", err)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"resource": map[string]any{
			"apiVersion": request.APIVersion, "kind": request.Kind, "form": request.Form,
			"metadata": request.Metadata, "spec": request.Spec,
		},
		"review": map[string]any{"prepareDigest": v3TestPrepareDigest, "specDigest": specDigest},
	})
}

func (h *v3FakeHost) serveUploadStart(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var request struct {
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		h.t.Errorf("upload start request not JSON: %v", err)
	}
	h.manifestRaw = request.Manifest
	var manifest struct {
		Modules []struct {
			Digest string `json:"digest"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(request.Manifest, &manifest); err != nil {
		h.t.Errorf("upload start manifest not JSON: %v", err)
	}
	missing := make([]string, 0, len(manifest.Modules))
	for _, module := range manifest.Modules {
		if _, held := h.blobs[module.Digest]; !held {
			missing = append(missing, module.Digest)
		}
	}
	h.events = append(h.events, "artifact-start")
	h.writeJSON(w, http.StatusCreated, map[string]any{"uploadId": "up_1", "missingBlobs": missing})
}

func (h *v3FakeHost) serveResource(w http.ResponseWriter, r *http.Request, remainder string) {
	segments := strings.Split(remainder, "/")
	if len(segments) < 3 {
		h.t.Errorf("unexpected resource path %q", remainder)
		http.NotFound(w, r)
		return
	}
	group, _ := unescapeSegment(segments[0])
	kind := segments[1]
	name := segments[2]
	if !strings.Contains(segments[0], "%2F") && !strings.Contains(segments[0], "%2f") {
		h.t.Errorf("namespaced group must travel as one %%2F-escaped segment, got %q", segments[0])
	}
	key := h.resourceKey(kind, name)
	switch r.Method {
	case http.MethodPut:
		h.serveApply(w, r, group, kind, name, key)
	case http.MethodGet:
		h.getRequests++
		h.recordResourceQuery(r, group)
		record, ok := h.resources[key]
		if !ok {
			h.writeStableError(w, http.StatusNotFound, "resource_not_found")
			return
		}
		w.Header().Set("ETag", `"`+strconv.Itoa(record.revision)+`"`)
		// A read is dispatched by exact FormRef query, and a conforming host
		// answers under the identity it was asked about. Echoing the query (rather
		// than whichever line created the record) is what lets a test drive two
		// FormRefs of one Kind through the same resource.
		h.writeJSON(w, http.StatusOK, h.wireResourceAs(record, name, r.URL.Query()))
	case http.MethodDelete:
		h.recordResourceQuery(r, group)
		record, ok := h.resources[key]
		if !ok {
			h.writeStableError(w, http.StatusNotFound, "resource_not_found")
			return
		}
		if want := `"` + strconv.Itoa(record.revision) + `"`; r.Header.Get("If-Match") != want {
			h.t.Errorf("delete If-Match = %q, want %q", r.Header.Get("If-Match"), want)
			h.writeStableError(w, http.StatusPreconditionFailed, "revision_conflict")
			return
		}
		if r.Header.Get("Idempotency-Key") == "" {
			h.t.Errorf("delete must carry an Idempotency-Key")
		}
		delete(h.resources, key)
		h.events = append(h.events, "delete:"+key)
		w.WriteHeader(http.StatusNoContent)
	default:
		h.t.Errorf("unexpected resource method %s %s", r.Method, remainder)
		http.NotFound(w, r)
	}
}

// recordResourceQuery captures the exact FormRef a read or delete dispatched
// on. The path group and the query group must always agree.
func (h *v3FakeHost) recordResourceQuery(r *http.Request, pathGroup string) {
	query := r.URL.Query()
	if got := query.Get("group"); got != pathGroup {
		h.t.Errorf("resource path group %q and query group %q disagree", pathGroup, got)
	}
	h.resourceQueries = append(h.resourceQueries,
		r.Method+" "+query.Get("group")+" "+query.Get("schemaDigest"))
}

func (h *v3FakeHost) serveApply(w http.ResponseWriter, r *http.Request, group, kind, name, key string) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		h.t.Errorf("apply body not JSON: %v", err)
	}
	spec, _ := body["spec"].(map[string]any)
	form, _ := body["form"].(map[string]any)
	metadata, _ := body["metadata"].(map[string]any)
	space, _ := metadata["space"].(string)
	h.applySpecs = append(h.applySpecs, spec)
	h.applyBodies = append(h.applyBodies, body)
	h.applyHeaders = append(h.applyHeaders, r.Header.Clone())
	h.events = append(h.events, "apply:"+key)

	record, exists := h.resources[key]
	if r.Header.Get("If-None-Match") == "*" {
		if exists {
			h.writeStableError(w, http.StatusConflict, "form_identity_conflict")
			return
		}
		h.uidCounter++
		record = &v3HostRecord{
			uid:        "uid-" + strconv.Itoa(h.uidCounter),
			generation: 1, revision: 1,
			apiVersion: group, kind: kind, form: form, space: space, spec: spec,
		}
		h.resources[key] = record
		if h.apply202Pending {
			h.apply202Pending = false
			h.pendingOperations["op_apply_pending"] = record.uid
			w.Header().Set("Retry-After", "0")
			h.writeJSON(w, http.StatusAccepted, map[string]any{
				"operation": map[string]any{
					"apiVersion": clientv3.OperationAPIVersion, "kind": clientv3.OperationKind,
					"id": "op_apply_pending", "done": false,
					"target": map[string]any{"uid": record.uid},
				},
			})
			return
		}
		if h.apply202 {
			h.apply202 = false
			h.operationResults["op_apply1"] = map[string]any{"resource": h.wireResource(record, name)}
			h.writeJSON(w, http.StatusAccepted, map[string]any{
				"operation": map[string]any{
					"apiVersion": clientv3.OperationAPIVersion, "kind": clientv3.OperationKind,
					"id": "op_apply1", "done": false,
				},
			})
			return
		}
		w.Header().Set("ETag", `"1"`)
		h.writeJSON(w, http.StatusCreated, h.wireResource(record, name))
		return
	}
	if !exists {
		h.writeStableError(w, http.StatusNotFound, "resource_not_found")
		return
	}
	if fence := r.Header.Get("Takoform-Expected-Generation"); fence != strconv.Itoa(record.generation) {
		h.writeStableError(w, http.StatusPreconditionFailed, "generation_conflict")
		return
	}
	if expectedUID, _ := body["expectedUid"].(string); expectedUID != "" && expectedUID != record.uid {
		h.writeStableError(w, http.StatusConflict, "uid_mismatch")
		return
	}
	record.generation++
	record.revision++
	record.spec = spec
	w.Header().Set("ETag", `"`+strconv.Itoa(record.revision)+`"`)
	h.writeJSON(w, http.StatusOK, h.wireResource(record, name))
}

// eventIndex returns the position of the first matching event or -1.
func (h *v3FakeHost) eventIndex(event string) int {
	for index, candidate := range h.events {
		if candidate == event {
			return index
		}
	}
	return -1
}

func (h *v3FakeHost) eventPrefixIndex(prefix string) int {
	for index, candidate := range h.events {
		if strings.HasPrefix(candidate, prefix) {
			return index
		}
	}
	return -1
}

// newV3TestProviderData negotiates the fake host's v1alpha3 lane.
func newV3TestProviderData(t *testing.T, host *v3FakeHost) *providerData {
	t.Helper()
	c := clientv3.NewWithOptions(host.server.URL, "test-token", host.server.Client(), clientv3.Options{})
	if _, err := c.Discover(context.Background()); err != nil {
		t.Fatalf("v1alpha3 discovery: %v", err)
	}
	return &providerData{clientV3: c, defaultSpace: "prod"}
}

func v3TestFormResource(t *testing.T, kind string, data *providerData) *v3FormResource {
	t.Helper()
	form, ok := edgeformcatalog.ByKind(kind)
	if !ok {
		t.Fatalf("%s is not a declared family Form", kind)
	}
	return &v3FormResource{form: form, data: data}
}
