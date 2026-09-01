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

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

const (
	v3SupportCacheTestGroup      = "cache.example.takoform.com/v1beta1"
	v3SupportCacheTestDefinition = "1.0.0"
	v3SupportCacheTestSchemaA    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	v3SupportCacheTestSchemaB    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	v3SupportCacheTestInterface  = "cache.interface"
	v3SupportCacheTestBinding    = "cache.binding"
	v3SupportCacheTestService    = "com.example.cache"
)

func v3SupportCacheTestRef(kind, schemaDigest string) v3FormRef {
	return v3FormRef{
		APIVersion:        v3SupportCacheTestGroup,
		Kind:              kind,
		DefinitionVersion: v3SupportCacheTestDefinition,
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
					"operations": true, "artifact_upload": true,
					"support_profiles": true,
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

func v3SupportCacheTestFormProfile(ref v3FormRef) map[string]any {
	return map[string]any{
		"apiVersion": clientv3.SupportProfileAPIVersion,
		"kind":       "FormSupport",
		"formRef": map[string]any{
			"apiVersion":        ref.APIVersion,
			"kind":              ref.Kind,
			"definitionVersion": ref.DefinitionVersion,
			"schemaDigest":      ref.SchemaDigest,
		},
		"operations": []string{"create", "read", "update", "delete"},
	}
}

func v3SupportCacheTestContractProfile(kind, name, version string) map[string]any {
	key := "interfaceRef"
	apiVersion := "interfaces.takoform.com/v1alpha1"
	if kind == "BindingSupport" {
		key = "bindingRef"
		apiVersion = "bindings.takoform.com/v1alpha2"
	}
	return map[string]any{
		"apiVersion": clientv3.SupportProfileAPIVersion,
		"kind":       kind,
		key: map[string]any{
			"apiVersion": apiVersion,
			"name":       name, "version": version,
			"schemaDigest": v3SupportCacheTestSchemaA,
		},
	}
}

func v3SupportCacheTestServiceProfile(protocol string) map[string]any {
	return map[string]any{
		"apiVersion": clientv3.SupportProfileAPIVersion,
		"kind":       "StandardServiceSupport",
		"serviceRef": map[string]any{
			"apiVersion": formpackage.StandardServiceAPIVersion,
			"protocol":   protocol,
		},
		"satisfiable": true,
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

func TestV3SupportCacheDistinctKeysOverlapAcrossCategories(t *testing.T) {
	const wantRequests = 5

	refA := v3SupportCacheTestRef("CacheWorkerA", v3SupportCacheTestSchemaA)
	refB := v3SupportCacheTestRef("CacheWorkerB", v3SupportCacheTestSchemaB)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	arrived := make(chan string, wantRequests)
	var requestMu sync.Mutex
	requestCounts := map[string]int{}

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		requestMu.Lock()
		requestCounts[r.URL.Path]++
		requestMu.Unlock()
		arrived <- r.URL.Path
		<-release
		switch {
		case strings.Contains(r.URL.Path, "/support/forms/") && strings.Contains(r.URL.Path, "/CacheWorkerA/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestFormProfile(refA))
		case strings.Contains(r.URL.Path, "/support/forms/") && strings.Contains(r.URL.Path, "/CacheWorkerB/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestFormProfile(refB))
		case strings.Contains(r.URL.Path, "/support/interfaces/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK,
				v3SupportCacheTestContractProfile("InterfaceSupport", v3SupportCacheTestInterface, "1.0.0"))
		case strings.Contains(r.URL.Path, "/support/bindings/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK,
				v3SupportCacheTestContractProfile("BindingSupport", v3SupportCacheTestBinding, "1.0.0"))
		case strings.Contains(r.URL.Path, "/support/standard-services/"):
			writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestServiceProfile(v3SupportCacheTestService))
		default:
			http.Error(w, "unknown support route", http.StatusNotFound)
		}
	})

	cache := newV3SupportCache()
	results := make(chan v3SupportAnswer, wantRequests)
	go func() { results <- cache.formSupport(context.Background(), client, refA) }()
	go func() { results <- cache.formSupport(context.Background(), client, refB) }()
	go func() {
		results <- cache.interfaceSupport(context.Background(), client, v3SupportCacheTestInterface, "1.0.0")
	}()
	go func() {
		results <- cache.bindingSupport(context.Background(), client, v3SupportCacheTestBinding, "1.0.0")
	}()
	go func() {
		results <- cache.standardServiceSupport(context.Background(), client, v3SupportCacheTestService)
	}()

	for i := 0; i < wantRequests; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			t.Fatalf("support cache serialized distinct key request %d/%d", i, wantRequests)
		}
	}
	releaseOnce.Do(func() { close(release) })

	for i := 0; i < wantRequests; i++ {
		select {
		case answer := <-results:
			if answer.Err != nil || answer.Refused {
				t.Errorf("distinct support key returned refusal/error: %+v", answer)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("support cache distinct key call did not complete")
		}
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if len(requestCounts) != wantRequests {
		t.Fatalf("distinct support request paths = %d, want %d (%v)", len(requestCounts), wantRequests, requestCounts)
	}
	for path, count := range requestCounts {
		if count != 1 {
			t.Errorf("support request %s count = %d, want 1", path, count)
		}
	}
}

func TestV3SupportCacheSameKeyExactlyOneGetPerCategory(t *testing.T) {
	const waiterCount = 16

	tests := []struct {
		name string
		path string
		read func(*v3SupportCache, context.Context, *clientv3.Client) v3SupportAnswer
		body func() map[string]any
	}{
		{
			name: "form",
			path: "/support/forms/",
			read: func(cache *v3SupportCache, ctx context.Context, client *clientv3.Client) v3SupportAnswer {
				return cache.formSupport(ctx, client, v3SupportCacheTestRef("CacheWorker", v3SupportCacheTestSchemaA))
			},
			body: func() map[string]any {
				return v3SupportCacheTestFormProfile(v3SupportCacheTestRef("CacheWorker", v3SupportCacheTestSchemaA))
			},
		},
		{
			name: "interface",
			path: "/support/interfaces/",
			read: func(cache *v3SupportCache, ctx context.Context, client *clientv3.Client) v3SupportAnswer {
				return cache.interfaceSupport(ctx, client, v3SupportCacheTestInterface, "1.0.0")
			},
			body: func() map[string]any {
				return v3SupportCacheTestContractProfile("InterfaceSupport", v3SupportCacheTestInterface, "1.0.0")
			},
		},
		{
			name: "binding",
			path: "/support/bindings/",
			read: func(cache *v3SupportCache, ctx context.Context, client *clientv3.Client) v3SupportAnswer {
				return cache.bindingSupport(ctx, client, v3SupportCacheTestBinding, "1.0.0")
			},
			body: func() map[string]any {
				return v3SupportCacheTestContractProfile("BindingSupport", v3SupportCacheTestBinding, "1.0.0")
			},
		},
		{
			name: "standard service",
			path: "/support/standard-services/",
			read: func(cache *v3SupportCache, ctx context.Context, client *clientv3.Client) v3SupportAnswer {
				return cache.standardServiceSupport(ctx, client, v3SupportCacheTestService)
			},
			body: func() map[string]any {
				return v3SupportCacheTestServiceProfile(v3SupportCacheTestService)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstEntered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			var countMu sync.Mutex
			requestCount := 0

			client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, test.path) {
					http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
					return
				}
				countMu.Lock()
				requestCount++
				if requestCount == 1 {
					close(firstEntered)
				}
				countMu.Unlock()
				<-release
				writeV3SupportCacheTestJSON(w, http.StatusOK, test.body())
			})

			cache := newV3SupportCache()
			leader := make(chan v3SupportAnswer, 1)
			go func() { leader <- test.read(cache, context.Background(), client) }()
			select {
			case <-firstEntered:
			case <-time.After(2 * time.Second):
				t.Fatal("leader did not issue support GET")
			}

			waiters := make(chan v3SupportAnswer, waiterCount)
			started := make(chan struct{}, waiterCount)
			for i := 0; i < waiterCount; i++ {
				go func() {
					started <- struct{}{}
					waiters <- test.read(cache, context.Background(), client)
				}()
			}
			for i := 0; i < waiterCount; i++ {
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("same-key waiter did not start")
				}
			}
			releaseOnce.Do(func() { close(release) })

			if answer := <-leader; answer.Err != nil || answer.Refused {
				t.Fatalf("leader answer = %+v", answer)
			}
			for i := 0; i < waiterCount; i++ {
				select {
				case answer := <-waiters:
					if answer.Err != nil || answer.Refused {
						t.Fatalf("waiter %d answer = %+v", i, answer)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("same-key waiter did not complete")
				}
			}
			countMu.Lock()
			defer countMu.Unlock()
			if requestCount != 1 {
				t.Fatalf("same-key support GET count = %d, want 1", requestCount)
			}
		})
	}
}

// doneObservedContext lets the test distinguish a waiter that reached the
// per-key wait from one blocked behind the leader's network request. The
// wrapper is only a context; it does not depend on the cache implementation.
type doneObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestV3SupportCacheWaiterCancellationDoesNotCancelLeader(t *testing.T) {
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var requestMu sync.Mutex
	requestCount := 0
	ref := v3SupportCacheTestRef("CacheWorker", v3SupportCacheTestSchemaA)

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/forms/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		requestMu.Lock()
		requestCount++
		if requestCount == 1 {
			close(firstEntered)
		}
		requestMu.Unlock()
		<-release
		writeV3SupportCacheTestJSON(w, http.StatusOK, v3SupportCacheTestFormProfile(ref))
	})

	cache := newV3SupportCache()
	leader := make(chan v3SupportAnswer, 1)
	go func() { leader <- cache.formSupport(context.Background(), client, ref) }()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not issue support GET")
	}

	waiterObserved := make(chan struct{})
	waiterBase, cancel := context.WithCancel(context.Background())
	waiterCtx := &doneObservedContext{
		Context:  waiterBase,
		observed: waiterObserved,
	}
	waiter := make(chan v3SupportAnswer, 1)
	go func() { waiter <- cache.formSupport(waiterCtx, client, ref) }()
	select {
	case <-waiterObserved:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("same-key waiter did not reach its cancellable wait")
	}
	cancel()
	select {
	case answer := <-waiter:
		if !errors.Is(answer.Err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v, want context canceled", answer.Err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("same-key waiter did not honor context cancellation")
	}

	releaseOnce.Do(func() { close(release) })
	if answer := <-leader; answer.Err != nil || answer.Refused {
		t.Fatalf("leader answer = %+v", answer)
	}
	if answer := cache.formSupport(context.Background(), client, ref); answer.Err != nil || answer.Refused {
		t.Fatalf("cached answer after canceled waiter = %+v", answer)
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if requestCount != 1 {
		t.Fatalf("canceled waiter triggered another support GET: %d", requestCount)
	}
}

func TestV3SupportCacheLeaderCancellationPublishesAndCleansInflight(t *testing.T) {
	firstEntered := make(chan struct{})
	var requestMu sync.Mutex
	requestCount := 0
	ref := v3SupportCacheTestRef("CacheWorker", v3SupportCacheTestSchemaA)

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/forms/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		requestMu.Lock()
		requestCount++
		if requestCount == 1 {
			close(firstEntered)
		}
		requestMu.Unlock()
		<-r.Context().Done()
	})

	cache := newV3SupportCache()
	leaderCtx, cancel := context.WithCancel(context.Background())
	leader := make(chan v3SupportAnswer, 1)
	go func() { leader <- cache.formSupport(leaderCtx, client, ref) }()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not issue support GET")
	}
	cancel()
	select {
	case answer := <-leader:
		if !errors.Is(answer.Err, context.Canceled) {
			t.Fatalf("leader cancellation error = %v, want context canceled", answer.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled leader did not complete")
	}

	answer := cache.formSupport(context.Background(), client, ref)
	if !errors.Is(answer.Err, context.Canceled) {
		t.Fatalf("cached leader cancellation error = %v, want context canceled", answer.Err)
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if requestCount != 1 {
		t.Fatalf("leader cancellation triggered another support GET: %d", requestCount)
	}
}

func TestV3SupportCacheLeaderErrorPublishesAndCleansInflight(t *testing.T) {
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var requestMu sync.Mutex
	requestCount := 0
	ref := v3SupportCacheTestRef("CacheWorker", v3SupportCacheTestSchemaA)

	client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/support/forms/") {
			http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
			return
		}
		requestMu.Lock()
		requestCount++
		if requestCount == 1 {
			close(firstEntered)
		}
		requestMu.Unlock()
		<-release
		writeV3SupportCacheTestJSON(w, http.StatusInternalServerError, v3SupportCacheTestError("backend_unavailable"))
	})

	cache := newV3SupportCache()
	leader := make(chan v3SupportAnswer, 1)
	go func() { leader <- cache.formSupport(context.Background(), client, ref) }()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not issue support GET")
	}

	waiterObserved := make(chan struct{})
	waiter := make(chan v3SupportAnswer, 1)
	go func() {
		waiterCtx := &doneObservedContext{Context: context.Background(), observed: waiterObserved}
		waiter <- cache.formSupport(waiterCtx, client, ref)
	}()
	select {
	case <-waiterObserved:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("same-key waiter did not reach its wait")
	}
	releaseOnce.Do(func() { close(release) })

	for name, result := range map[string]chan v3SupportAnswer{"leader": leader, "waiter": waiter} {
		select {
		case answer := <-result:
			if answer.Refused || answer.Err == nil {
				t.Fatalf("%s answer = %+v, want cached ordinary error", name, answer)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not receive leader error", name)
		}
	}
	answer := cache.formSupport(context.Background(), client, ref)
	if answer.Refused || answer.Err == nil {
		t.Fatalf("cached leader error answer = %+v", answer)
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if requestCount != 1 {
		t.Fatalf("leader error triggered another support GET: %d", requestCount)
	}
}

func TestV3SupportCacheRefusalAndErrorStayCached(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		read   func(*v3SupportCache, context.Context, *clientv3.Client) v3SupportAnswer
		code   string
		status int
	}{
		{
			name: "form refusal",
			path: "/support/forms/",
			read: func(cache *v3SupportCache, ctx context.Context, client *clientv3.Client) v3SupportAnswer {
				return cache.formSupport(ctx, client, v3SupportCacheTestRef("CacheWorker", v3SupportCacheTestSchemaA))
			},
			code: "form_unknown", status: http.StatusNotFound,
		},
		{
			name: "interface error",
			path: "/support/interfaces/",
			read: func(cache *v3SupportCache, ctx context.Context, client *clientv3.Client) v3SupportAnswer {
				return cache.interfaceSupport(ctx, client, v3SupportCacheTestInterface, "1.0.0")
			},
			code: "backend_unavailable", status: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestMu sync.Mutex
			requestCount := 0
			client := newV3SupportCacheTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, test.path) {
					http.Error(w, "unexpected support cache request", http.StatusInternalServerError)
					return
				}
				requestMu.Lock()
				requestCount++
				requestMu.Unlock()
				writeV3SupportCacheTestJSON(w, test.status, v3SupportCacheTestError(test.code))
			})
			cache := newV3SupportCache()
			first := test.read(cache, context.Background(), client)
			second := test.read(cache, context.Background(), client)
			if test.status == http.StatusNotFound {
				if !first.Refused || first.Err != nil || !second.Refused || second.Err != nil {
					t.Fatalf("refusal answers = first=%+v second=%+v", first, second)
				}
			} else if first.Refused || first.Err == nil || second.Refused || second.Err == nil {
				t.Fatalf("error answers = first=%+v second=%+v", first, second)
			}
			requestMu.Lock()
			defer requestMu.Unlock()
			if requestCount != 1 {
				t.Fatalf("cached support GET count = %d, want 1", requestCount)
			}
		})
	}
}
