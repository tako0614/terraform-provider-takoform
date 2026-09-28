package clientv3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAcceptedOperationPollReadsRotatedTokenWithoutReplayingMutation(t *testing.T) {
	var token atomic.Value
	token.Store("initial")
	var mutations atomic.Int32
	var polls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "Bearer " + token.Load().(string)
		if got := r.Header.Get("Authorization"); got != expected {
			t.Errorf("%s authorization = %q, want current token", r.URL.Path, got)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == DiscoveryPath:
			writeJSON(t, w, http.StatusOK, discoveryDoc(server.URL))
		case r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
		case r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
		case r.Method == http.MethodPut:
			mutations.Add(1)
			token.Store("rotated")
			w.Header().Set("Retry-After", "0")
			writeJSON(t, w, http.StatusAccepted, map[string]any{"operation": map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_rotate", "done": false,
			}})
		case r.URL.Path == APIRootPath+"/operations/op_rotate":
			polls.Add(1)
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_rotate", "done": true,
				"result": map[string]any{"resource": wireResource("app", "uid-1", "1", "3", map[string]any{"image": "example"})},
			})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewWithOptions(server.URL, "", server.Client(), Options{TokenSource: func() (string, error) {
		return token.Load().(string), nil
	}})
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApplyResource(context.Background(), testResourceRequest(map[string]any{"image": "example"}), Fence{}); err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 1 || polls.Load() == 0 {
		t.Fatalf("mutations=%d polls=%d, want one mutation and a poll", mutations.Load(), polls.Load())
	}
}

func TestTokenSourceFailurePreventsRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewWithOptions(server.URL, "", server.Client(), Options{TokenSource: func() (string, error) {
		return "", errors.New("credential unavailable")
	}})
	if _, err := client.Discover(context.Background()); err == nil {
		t.Fatal("missing credential allowed discovery")
	}
	if requests.Load() != 0 {
		t.Fatalf("credential failure made %d HTTP requests", requests.Load())
	}
}

func TestParallelRequestsUseTokenSourceIndependently(t *testing.T) {
	var calls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer parallel" {
			t.Errorf("missing authorization on %s", r.URL.Path)
		}
		if r.URL.Path == DiscoveryPath {
			writeJSON(t, w, http.StatusOK, discoveryDoc(server.URL))
			return
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"apiVersion": OperationAPIVersion, "kind": OperationKind,
			"id": "op_parallel", "done": false,
		})
	}))
	defer server.Close()
	client := NewWithOptions(server.URL, "", server.Client(), Options{TokenSource: func() (string, error) {
		calls.Add(1)
		return "parallel", nil
	}})
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	const count = 20
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.GetOperation(context.Background(), "op_parallel"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != count+1 {
		t.Fatalf("token source called %d times, want %d", calls.Load(), count+1)
	}
}

func TestTokenSourceRefreshesRetryButDoesNotReplayUnauthorizedMutation(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		code         string
		retryable    bool
		wantAttempts int32
	}{
		{"retryable host error", http.StatusTooManyRequests, "rate_limited", true, 2},
		{"unauthorized", http.StatusUnauthorized, "unauthenticated", false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var current atomic.Value
			current.Store("before")
			var attempts atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+current.Load().(string) {
					t.Errorf("attempt used stale token")
				}
				if r.URL.Path == DiscoveryPath {
					writeJSON(t, w, http.StatusOK, discoveryDoc(server.URL))
					return
				}
				if attempts.Add(1) == 1 {
					current.Store("after")
					writeStableError(t, w, test.status, test.code, test.retryable)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			client := NewWithOptions(server.URL, "", server.Client(), Options{TokenSource: func() (string, error) {
				return current.Load().(string), nil
			}})
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			err := client.AbandonUpload(context.Background(), "up_test")
			if test.wantAttempts == 2 && err != nil || test.wantAttempts == 1 && err == nil {
				t.Fatalf("unexpected mutation outcome: %v", err)
			}
			if attempts.Load() != test.wantAttempts {
				t.Fatalf("mutation attempts = %d, want %d", attempts.Load(), test.wantAttempts)
			}
		})
	}
}
