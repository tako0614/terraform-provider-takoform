package provider

// v3_concurrency_test.go proves the v1beta1 lane holds no global mutation
// lock. The claim is structural — nothing in the lane serializes — so the only
// honest proof is a host that refuses to answer the FIRST apply until a SECOND
// one has arrived. If a global mutex is ever reintroduced the second apply can
// never arrive and this test fails instead of quietly losing parallelism.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

// v3RendezvousHost answers applies only once `want` of them are simultaneously
// in flight. Unlike v3FakeHost it never holds one lock across a whole request,
// so the rendezvous measures the CLIENT's concurrency, not the fake's.
type v3RendezvousHost struct {
	t      *testing.T
	server *httptest.Server
	want   int

	mu       sync.Mutex
	inFlight int
	uids     int
	arrived  []string
	proceed  chan struct{}
}

func newV3RendezvousHost(t *testing.T, want int) *v3RendezvousHost {
	t.Helper()
	host := &v3RendezvousHost{t: t, want: want, proceed: make(chan struct{})}
	host.server = httptest.NewServer(http.HandlerFunc(host.serve))
	t.Cleanup(host.server.Close)
	return host
}

func (h *v3RendezvousHost) writeJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Errorf("encoding rendezvous host response: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func (h *v3RendezvousHost) serve(w http.ResponseWriter, r *http.Request) {
	escaped := r.URL.EscapedPath()
	switch {
	case escaped == clientv3.DiscoveryPath:
		h.writeJSON(w, http.StatusOK, map[string]any{
			"api_versions": []string{clientv3.APIVersion},
			"features": map[string]bool{
				"service_forms": true, "exact_form_ref": true,
				"optimistic_concurrency": true, "idempotent_lifecycle": true,
				"operations": true, "artifact_upload": true, "support_profiles": true,
			},
			"endpoints": map[string]any{"api": h.server.URL + v3TestAPIRoot},
		})
	case escaped == v3TestAPIRoot+"/forms":
		query := r.URL.Query()
		apiVersion := query.Get("group")
		if version := query.Get("version"); version != "" {
			apiVersion += "/" + version
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"forms": []map[string]any{{
			"identity": map[string]any{"formRef": map[string]any{
				"apiVersion":        apiVersion,
				"kind":              query.Get("kind"),
				"definitionVersion": query.Get("definitionVersion"),
				"schemaDigest":      query.Get("schemaDigest"),
			}},
			"definitionKnown": true, "installed": true, "executable": true,
			"activated": true, "availableToPrincipal": true,
			"operations": []string{"create", "read", "update", "delete"},
		}}})
	case escaped == v3TestAPIRoot+"/resources/prepare":
		h.servePrepare(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(escaped, v3TestAPIRoot+"/resources/"):
		h.serveApply(w, r, strings.TrimPrefix(escaped, v3TestAPIRoot+"/resources/"))
	default:
		h.t.Errorf("unexpected rendezvous host request %s %s", r.Method, escaped)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}
}

func (h *v3RendezvousHost) servePrepare(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		h.t.Errorf("prepare request not JSON: %v", err)
	}
	spec, _ := request["spec"].(map[string]any)
	specRaw, _ := json.Marshal(spec)
	specDigest, err := formpackage.DigestCanonicalJSON(specRaw)
	if err != nil {
		h.t.Errorf("digesting prepare spec: %v", err)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"resource": request,
		"review":   map[string]any{"prepareDigest": v3TestPrepareDigest, "specDigest": specDigest},
	})
}

// serveApply blocks until `want` applies are in flight simultaneously.
func (h *v3RendezvousHost) serveApply(w http.ResponseWriter, r *http.Request, remainder string) {
	// /resources/{formGroup}/{formVersion}/{kind}/{name}: the namespaced group
	// travels as two ordinary path segments (spec/decisions/0018).
	segments := strings.Split(remainder, "/")
	groupName, _ := unescapeSegment(segments[0])
	groupVersion, _ := unescapeSegment(segments[1])
	group := groupName + "/" + groupVersion
	kind, name := segments[2], segments[3]
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		h.t.Errorf("apply body not JSON: %v", err)
	}

	h.mu.Lock()
	h.inFlight++
	h.arrived = append(h.arrived, kind+"/"+name)
	h.uids++
	uid := "uid-" + string(rune('0'+h.uids))
	reached := h.inFlight == h.want
	h.mu.Unlock()
	if reached {
		close(h.proceed)
	}
	select {
	case <-h.proceed:
	case <-time.After(20 * time.Second):
		// Only a serializing client can get here: the second apply never left
		// the provider while the first was outstanding.
		h.t.Error("applies did not reach the host concurrently: the lane serialized them")
		http.Error(w, "serialized", http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", `"1"`)
	h.writeJSON(w, http.StatusCreated, map[string]any{
		"apiVersion": group, "kind": kind,
		"form":     body["form"],
		"metadata": map[string]any{"name": name, "space": "prod", "uid": uid, "generation": "1", "revision": "1"},
		"spec":     body["spec"],
		"status": map[string]any{
			"observedGeneration": "1",
			"conditions": []map[string]any{{
				"type": "Ready", "status": "True", "reason": "Available",
				"lastTransitionTime": "2026-08-06T00:00:00Z",
			}},
		},
	})
}

func TestV3LaneRunsUnrelatedResourceOperationsConcurrently(t *testing.T) {
	host := newV3RendezvousHost(t, 2)
	data := &providerData{defaultSpace: "prod"}
	client := clientv3.NewWithOptions(host.server.URL, "test-token", host.server.Client(), clientv3.Options{})
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("v1beta1 discovery: %v", err)
	}
	data.clientV3 = client

	ctx := context.Background()
	cases := []struct {
		kind string
		name string
	}{
		{"ModuleWorker", "worker-one"},
		{"EdgeKVNamespace", "kv-two"},
	}

	var wait sync.WaitGroup
	results := make([]frameworkresource.CreateResponse, len(cases))
	for index, testCase := range cases {
		index, testCase := index, testCase
		resource := v3TestFormResource(t, testCase.kind, data)
		schemaResponse := v3SchemaOf(t, resource)
		plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name":  types.StringValue(testCase.name),
			"space": types.StringValue("prod"),
		})
		results[index] = frameworkresource.CreateResponse{
			State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &results[index])
		}()
	}
	wait.Wait()

	for index, testCase := range cases {
		if results[index].Diagnostics.HasError() {
			t.Fatalf("concurrent create of %s: %v", testCase.kind, results[index].Diagnostics)
		}
		if got := v3StateString(t, ctx, results[index].State, "name").ValueString(); got != testCase.name {
			t.Fatalf("concurrent create %d wrote name %q, want %q", index, got, testCase.name)
		}
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.arrived) != len(cases) {
		t.Fatalf("host saw %d applies, want %d: %v", len(host.arrived), len(cases), host.arrived)
	}
}
