package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

type hostAPIContract struct {
	Format                   string   `json:"format"`
	APIGroup                 string   `json:"apiGroup"`
	DiscoveryPath            string   `json:"discoveryPath"`
	RequiredFeatures         []string `json:"requiredFeatures"`
	OptionalFeatures         []string `json:"optionalFeatures"`
	RequiredEndpoints        []string `json:"requiredEndpoints"`
	OptionalEndpoints        []string `json:"optionalEndpoints"`
	ExactFormQueryParameters []string `json:"exactFormQueryParameters"`
	Operations               []struct {
		Name           string `json:"name"`
		Method         string `json:"method"`
		Path           string `json:"path"`
		ExactFormQuery bool   `json:"exactFormQuery"`
		Precondition   string `json:"precondition"`
		IdempotencyKey bool   `json:"idempotencyKey"`
		Mutates        bool   `json:"mutates"`
		Optional       bool   `json:"optional"`
	} `json:"operations"`
	ErrorEnvelope struct {
		Codes                  []string `json:"codes"`
		AutomaticallyRetryable []string `json:"automaticallyRetryable"`
	} `json:"errorEnvelope"`
}

// TestClientSpeaksThePublishedHostContract drives every non-optional operation
// and compares what actually went over the wire with the published contract.
//
// The prose contract binds a host; this proves the reference client is held to
// the same document, so a host implementer reading spec/host-api can rely on it
// describing real behaviour rather than intent.
func TestClientSpeaksThePublishedHostContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "host-api", "operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract hostAPIContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Format != "takoform.host-api@v1alpha1" || contract.APIGroup != APIVersion {
		t.Fatalf("contract identity drift: %s %s", contract.Format, contract.APIGroup)
	}
	if contract.DiscoveryPath != "/.well-known/takoform" {
		t.Fatalf("discovery path drift: %s", contract.DiscoveryPath)
	}

	type observed struct {
		method, ifMatch, ifNone, idempotency string
		query                                []string
	}
	var mutex sync.Mutex
	seen := map[string]observed{}
	base := "/apis/" + APIVersion
	resource := Resource{
		APIVersion: APIVersion, Kind: KindObjectBucket, Form: &exactObjectBucketFixture,
		Metadata: Metadata{Name: "assets", Space: "prod", ResourceVersion: "1"},
		Spec:     map[string]any{"name": "assets"},
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := strings.TrimPrefix(r.URL.Path, base)
		keys := make([]string, 0, len(r.URL.Query()))
		for key := range r.URL.Query() {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		mutex.Lock()
		seen[r.Method+" "+route] = observed{
			method: r.Method, ifMatch: r.Header.Get("If-Match"), ifNone: r.Header.Get("If-None-Match"),
			idempotency: r.Header.Get("Idempotency-Key"), query: keys,
		}
		mutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"1"`)
		switch {
		case r.URL.Path == "/.well-known/takoform":
			writeVersionedDiscovery(t, w, server.URL)
		case route == "/forms":
			_ = json.NewEncoder(w).Encode(map[string]any{"forms": []FormAvailability{{
				Identity: exactObjectBucketFixture, DefinitionKnown: true, Installed: true, Executable: true,
				Activated: true, AvailableToPrincipal: true,
				Operations: []string{"create", "read", "update", "delete", "import", "observe", "refresh"},
			}}})
		case route == "/resources/preview":
			var desired Resource
			_ = json.NewDecoder(r.Body).Decode(&desired)
			_ = json.NewEncoder(w).Encode(PreviewResourceResult{Resource: desired, Review: PreviewReview{PlanDigest: "sha256:plan"}})
		case strings.HasSuffix(route, "/observe"), strings.HasSuffix(route, "/refresh"), strings.HasSuffix(route, "/import"):
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "observation": map[string]any{"status": "current"}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_ = json.NewEncoder(w).Encode(resource)
		}
	}))
	defer server.Close()

	client := New(server.URL, "token", server.Client())
	ctx := context.Background()
	if _, err := client.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	desired := &Resource{
		APIVersion: APIVersion, Kind: KindObjectBucket, Form: &exactObjectBucketFixture,
		Metadata: Metadata{Name: "assets", Space: "prod"},
		Spec:     map[string]any{"name": "assets"},
	}
	fence := MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture}
	if _, err := client.PutResource(ctx, KindObjectBucket, "assets", desired); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImportResource(ctx, KindObjectBucket, "assets", "native", desired); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetResource(ctx, KindObjectBucket, "assets", "prod", exactObjectBucketFixture); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObserveResource(ctx, KindObjectBucket, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RefreshResource(ctx, KindObjectBucket, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteResource(ctx, KindObjectBucket, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}

	for _, operation := range contract.Operations {
		if operation.Optional {
			continue
		}
		route := strings.ReplaceAll(strings.ReplaceAll(operation.Path, "{kind}", KindObjectBucket), "{name}", "assets")
		record, ok := seen[operation.Method+" "+route]
		if !ok {
			t.Errorf("contract operation %s (%s %s) never reached the host", operation.Name, operation.Method, route)
			continue
		}
		if operation.IdempotencyKey && record.idempotency == "" {
			t.Errorf("%s sent no Idempotency-Key", operation.Name)
		}
		if !operation.IdempotencyKey && record.idempotency != "" {
			t.Errorf("%s sent an unexpected Idempotency-Key", operation.Name)
		}
		if operation.Precondition == "if-match" && record.ifMatch == "" {
			t.Errorf("%s sent no If-Match fence", operation.Name)
		}
		if operation.ExactFormQuery {
			for _, parameter := range contract.ExactFormQueryParameters {
				if !containsString(record.query, parameter) {
					t.Errorf("%s omitted exact-identity query parameter %s", operation.Name, parameter)
				}
			}
		}
	}
	for _, code := range contract.ErrorEnvelope.AutomaticallyRetryable {
		if !containsString(contract.ErrorEnvelope.Codes, code) {
			t.Errorf("retryable code %s is not in the published error taxonomy", code)
		}
	}
	for _, feature := range contract.RequiredFeatures {
		if !client.Discovery.HasFeature(feature) {
			t.Errorf("required feature %s was not negotiated", feature)
		}
	}
}
