package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

const (
	v3SupportCacheTestGroup  = "cache.example.takoform.com/v1beta1"
	v3SupportCacheTestKind   = "CacheWorker"
	v3SupportCacheTestSchema = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func v3SupportCacheTestRef(schemaDigest string) currentformregistry.V3Ref {
	return currentformregistry.V3Ref{
		APIVersion:        v3SupportCacheTestGroup,
		Kind:              v3SupportCacheTestKind,
		DefinitionVersion: "1.0.0",
		SchemaDigest:      schemaDigest,
	}
}

func newV3SupportCacheTestClient(
	t *testing.T,
	handleSupport func(http.ResponseWriter, *http.Request),
) *clientv3.Client {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == clientv3.DiscoveryPath {
			writeV3SupportCacheTestJSON(w, http.StatusOK, map[string]any{
				"api_versions": []string{clientv3.APIVersion},
				"features": map[string]bool{
					"service_forms": true, "exact_form_ref": true,
					"optimistic_concurrency": true, "idempotent_lifecycle": true,
					"operations": true, "artifact_upload": true, "support_profiles": true,
				},
				"endpoints": map[string]any{"api": server.URL + clientv3.APIRootPath},
			})
			return
		}
		handleSupport(w, r)
	}))
	t.Cleanup(server.Close)

	client := clientv3.New(server.URL, "", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("discover support cache test host: %v", err)
	}
	return client
}

func writeV3SupportCacheTestJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func v3SupportCacheTestProfile(kind string) map[string]any {
	return map[string]any{
		"apiVersion": clientv3.SupportProfileAPIVersion,
		"kind":       kind,
	}
}

func v3SupportCacheTestError(code string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code": code, "message": "support cache test " + code,
			"requestId": "support-cache-test", "retryable": false,
		},
	}
}

func TestV3SupportCacheDistinctKeysOverlap(t *testing.T) {
	const wantRequests = 4

	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	arrived := make(chan string, wantRequests)

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		arrived <- r.URL.Path
		<-release
		switch {
		case strings.Contains(r.URL.Path, "/support/forms/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestProfile("FormSupport"))
		case strings.Contains(r.URL.Path, "/support/interfaces/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK, map[string]any{
				"apiVersion": clientv3.SupportProfileAPIVersion,
				"kind":       "InterfaceSupport",
				"interfaceRef": map[string]any{
					"name": "cache.interface", "version": "1.0.0",
				},
			})
		case strings.Contains(r.URL.Path, "/support/bindings/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK, map[string]any{
				"apiVersion": clientv3.SupportProfileAPIVersion,
				"kind":       "BindingSupport",
				"bindingRef": map[string]any{
					"name": "cache.binding", "version": "1.0.0",
				},
			})
		default:
			http.Error(w, "unknown support route", http.StatusNotFound)
		}
	})

	cache := newV3SupportCache()
	refA := v3SupportCacheTestRef(v3SupportCacheTestSchema)
	refB := v3SupportCacheTestRef("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	type result struct {
		answer v3SupportAnswer
	}
	results := make(chan result, wantRequests)
	go func() { results <- result{cache.formSupport(context.Background(), client, refA)} }()
	go func() { results <- result{cache.formSupport(context.Background(), client, refB)} }()
	go func() {
		results <- result{cache.interfaceSupport(context.Background(), client, "cache.interface", "1.0.0")}
	}()
	go func() {
		results <- result{cache.bindingSupport(context.Background(), client, "cache.binding", "1.0.0")}
	}()

	for i := 0; i < wantRequests; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			t.Errorf("support cache serialized distinct key request %d/%d", i, wantRequests)
			releaseOnce.Do(func() { close(release) })
			return
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < wantRequests; i++ {
		select {
		case got := <-results:
			if got.answer.Err != nil || got.answer.Refused {
				t.Errorf("distinct support key returned refusal/error: %+v", got.answer)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("support cache distinct key call did not complete")
		}
	}
}

func TestV3SupportCacheSameKeyExactlyOneGet(t *testing.T) {
	const waiterCount = 16
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var mu sync.Mutex
	requestCount := 0

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/forms/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		requestCount++
		if requestCount == 1 {
			close(firstEntered)
		}
		mu.Unlock()
		<-release
		writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestProfile("FormSupport"))
	})

	cache := newV3SupportCache()
	ref := v3SupportCacheTestRef(v3SupportCacheTestSchema)
	leader := make(chan v3SupportAnswer, 1)
	go func() { leader <- cache.formSupport(context.Background(), client, ref) }()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not issue support GET")
	}

	started := make(chan struct{}, waiterCount)
	waiters := make(chan v3SupportAnswer, waiterCount)
	for i := 0; i < waiterCount; i++ {
		go func() {
			started <- struct{}{}
			waiters <- cache.formSupport(context.Background(), client, ref)
		}()
	}
	for i := 0; i < waiterCount; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("same-key waiter did not start")
		}
	}
	once.Do(func() { close(release) })

	if answer := <-leader; answer.Err != nil || answer.Refused {
		t.Fatalf("leader answer = %+v", answer)
	}
	for i := 0; i < waiterCount; i++ {
		if answer := <-waiters; answer.Err != nil || answer.Refused {
			t.Fatalf("waiter %d answer = %+v", i, answer)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if requestCount != 1 {
		t.Fatalf("same-key support GET count = %d, want 1", requestCount)
	}
}

func TestV3SupportCacheSameKeyWaiterContextCancel(t *testing.T) {
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var mu sync.Mutex
	requestCount := 0

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/forms/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		requestCount++
		if requestCount == 1 {
			close(firstEntered)
		}
		mu.Unlock()
		<-release
		writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestProfile("FormSupport"))
	})

	cache := newV3SupportCache()
	ref := v3SupportCacheTestRef(v3SupportCacheTestSchema)
	leader := make(chan v3SupportAnswer, 1)
	go func() { leader <- cache.formSupport(context.Background(), client, ref) }()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not issue support GET")
	}

	waiterCtx, cancel := context.WithCancel(context.Background())
	waiter := make(chan v3SupportAnswer, 1)
	go func() { waiter <- cache.formSupport(waiterCtx, client, ref) }()
	cancel()
	select {
	case answer := <-waiter:
		if !errors.Is(answer.Err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v, want context canceled", answer.Err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("same-key waiter did not honor context cancellation")
	}

	once.Do(func() { close(release) })
	if answer := <-leader; answer.Err != nil || answer.Refused {
		t.Fatalf("leader answer = %+v", answer)
	}
	if answer := cache.formSupport(context.Background(), client, ref); answer.Err != nil || answer.Refused {
		t.Fatalf("cached answer after canceled waiter = %+v", answer)
	}
	mu.Lock()
	defer mu.Unlock()
	if requestCount != 1 {
		t.Fatalf("canceled waiter triggered another support GET: %d", requestCount)
	}
}

func TestV3SupportCacheRefusalAndErrorStayCached(t *testing.T) {
	t.Run("refusal", func(t *testing.T) {
		var mu sync.Mutex
		requestCount := 0
		client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/forms/") {
				http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
				return
			}
			mu.Lock()
			requestCount++
			mu.Unlock()
			writeV3SupportCacheTestJSON(w, http.StatusNotFound, v3SupportCacheTestError("form_unknown"))
		})
		cache := newV3SupportCache()
		ref := v3SupportCacheTestRef(v3SupportCacheTestSchema)
		first := cache.formSupport(context.Background(), client, ref)
		second := cache.formSupport(context.Background(), client, ref)
		if !first.Refused || first.Err != nil || !second.Refused || second.Err != nil {
			t.Fatalf("refusal answers = first=%+v second=%+v", first, second)
		}
		mu.Lock()
		defer mu.Unlock()
		if requestCount != 1 {
			t.Fatalf("refusal support GET count = %d, want 1", requestCount)
		}
	})

	t.Run("error", func(t *testing.T) {
		var mu sync.Mutex
		requestCount := 0
		client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/interfaces/") {
				http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
				return
			}
			mu.Lock()
			requestCount++
			mu.Unlock()
			writeV3SupportCacheTestJSON(w, http.StatusInternalServerError, v3SupportCacheTestError("internal_error"))
		})
		cache := newV3SupportCache()
		first := cache.interfaceSupport(context.Background(), client, "cache.interface", "1.0.0")
		second := cache.interfaceSupport(context.Background(), client, "cache.interface", "1.0.0")
		if first.Refused || first.Err == nil || second.Refused || second.Err == nil {
			t.Fatalf("error answers = first=%+v second=%+v", first, second)
		}
		mu.Lock()
		defer mu.Unlock()
		if requestCount != 1 {
			t.Fatalf("error support GET count = %d, want 1", requestCount)
		}
	})
}
