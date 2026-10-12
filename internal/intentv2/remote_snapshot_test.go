package intentv2

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type remoteFixture struct {
	mu             sync.Mutex
	version        uint64
	data           []byte
	receipts       map[string]remoteReceipt
	grant          string
	ackLoss        bool
	unknownNoWrite bool
	badReceipt     bool
	denied         bool
	tamper         bool
	redirect       bool
	redirectURL    string
	readBarrier    chan struct{}
	readCount      int
	onLostAck      func()
	resolveGrant   string
	resolveCount   int
}

func newRemoteFixture(t *testing.T) (*remoteFixture, *httptest.Server) {
	t.Helper()
	f := &remoteFixture{grant: "private-grant-must-not-leak", receipts: make(map[string]remoteReceipt)}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv(PrivateRunFilesEnvironment, fmt.Sprintf(`[{"slot":"intent-one","endpoint":%q,"grant":%q}]`, server.URL+privateRunFilesPath, f.grant))
	return f, server
}

func (f *remoteFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+f.grant || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"kind":"authority_rejected"}`))
		return
	}
	if f.denied {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"kind":"authority_rejected"}`))
		return
	}
	if f.redirect {
		w.Header().Set("Location", f.redirectURL)
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case privateRunFilesPath + "/read":
		if f.readBarrier != nil && f.version > 0 {
			f.readCount++
			if f.readCount == 2 {
				close(f.readBarrier)
			}
			// Release the mutex while the other request reaches this point.
			barrier := f.readBarrier
			f.mu.Unlock()
			<-barrier
			f.mu.Lock()
		}
		if f.version == 0 {
			_, _ = w.Write([]byte(`{"kind":"absent"}`))
			return
		}
		digest := snapshotDigest(f.data)
		if f.tamper {
			digest = "sha256:" + strings.Repeat("0", 64)
		}
		_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "present", Version: f.version, Digest: digest, BytesBase64: base64.StdEncoding.EncodeToString(f.data)})
	case privateRunFilesPath + "/commit":
		var request struct {
			ExpectedVersion uint64 `json:"expectedVersion"`
			CommitID        string `json:"commitId"`
			BytesBase64     string `json:"bytesBase64"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, err := base64.StdEncoding.Strict().DecodeString(request.BytesBase64)
		if err != nil || len(data) > maxSnapshotBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		digest := snapshotDigest(data)
		if f.unknownNoWrite {
			_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "unknown", CommitID: request.CommitID, ExpectedVersion: request.ExpectedVersion, Digest: digest})
			return
		}
		if prior, ok := f.receipts[request.CommitID]; ok {
			if prior.ExpectedVersion == request.ExpectedVersion && prior.Digest == digest {
				_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "replayed", Receipt: &prior})
			} else {
				_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "conflict", CommitID: request.CommitID, ExpectedVersion: request.ExpectedVersion, Digest: digest})
			}
			return
		}
		if request.ExpectedVersion != f.version {
			_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "conflict", CommitID: request.CommitID, ExpectedVersion: request.ExpectedVersion, Digest: digest})
			return
		}
		f.version++
		f.data = data
		receipt := remoteReceipt{Version: f.version, ExpectedVersion: request.ExpectedVersion, CommitID: request.CommitID, Digest: digest}
		f.receipts[request.CommitID] = receipt
		if f.badReceipt {
			receipt.Digest = "sha256:" + strings.Repeat("0", 64)
			_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "committed", Receipt: &receipt})
			return
		}
		if f.ackLoss {
			f.ackLoss = false
			if f.onLostAck != nil {
				f.onLostAck()
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"kind":"unknown"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "committed", Receipt: &receipt})
	case privateRunFilesPath + "/resolve":
		f.resolveCount++
		f.resolveGrant = r.Header.Get("Authorization")
		var request struct {
			ExpectedVersion uint64 `json:"expectedVersion"`
			CommitID        string `json:"commitId"`
			Digest          string `json:"digest"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if receipt, ok := f.receipts[request.CommitID]; ok && receipt.ExpectedVersion == request.ExpectedVersion && receipt.Digest == request.Digest {
			_ = json.NewEncoder(w).Encode(remoteResponse{Kind: "committed", Receipt: &receipt})
			return
		}
		_, _ = w.Write([]byte(`{"kind":"not_found"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func openRemoteFixture(t *testing.T) *Store {
	t.Helper()
	store, err := OpenRemote(context.Background(), "intent-one", testStoreID, testStoreBinding())
	if err != nil {
		t.Fatalf("OpenRemote(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRemoteSnapshotRestartAndOriginalKey(t *testing.T) {
	f, _ := newRemoteFixture(t)
	store := openRemoteFixture(t)
	first, err := store.Begin(context.Background(), testBeginRequest())
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	reopened := openRemoteFixture(t)
	second, err := reopened.Begin(context.Background(), testBeginRequest())
	if err != nil || second.Key != first.Key || second.ID != first.ID || !second.FirstSentAt.Equal(first.FirstSentAt) {
		t.Fatalf("reopened original claim changed: error %v, same key %t", err, second.Key == first.Key)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if bytes.Contains(f.data, []byte(f.grant)) {
		t.Fatal("remote snapshot persisted its bearer grant")
	}
}

func TestRemoteSnapshotLostAckUsesExactReceipt(t *testing.T) {
	f, _ := newRemoteFixture(t)
	store := openRemoteFixture(t)
	f.mu.Lock()
	f.ackLoss = true
	f.mu.Unlock()
	intent, err := store.Begin(context.Background(), testBeginRequest())
	if err != nil || intent.Key == "" {
		t.Fatalf("lost ACK was not resolved: %v", err)
	}
	if got, err := store.Read(context.Background(), testBeginRequest().Target); err != nil || len(got) != 1 || got[0].Key != intent.Key {
		t.Fatalf("acknowledged claim missing: %v", err)
	}
}

func TestRemoteSnapshotUnresolvedCommitReturnsNoHostKey(t *testing.T) {
	f, _ := newRemoteFixture(t)
	store := openRemoteFixture(t)
	f.mu.Lock()
	f.unknownNoWrite = true
	f.mu.Unlock()
	intent, err := store.Begin(context.Background(), testBeginRequest())
	if !errors.Is(err, ErrStoreUncertain) || intent.Key != "" || intent.ID != "" {
		t.Fatalf("unresolved commit returned a Host claim: key present %t, error %v", intent.Key != "", err)
	}
	f.mu.Lock()
	f.unknownNoWrite = false
	f.mu.Unlock()
	records, err := store.Read(context.Background(), testBeginRequest().Target)
	if err != nil || len(records) != 0 {
		t.Fatalf("unacknowledged claim became visible: error %v, count %d", err, len(records))
	}
}

func TestRemoteSnapshotInvalidReceiptReturnsNoHostKey(t *testing.T) {
	f, _ := newRemoteFixture(t)
	store := openRemoteFixture(t)
	f.mu.Lock()
	f.badReceipt = true
	f.mu.Unlock()
	intent, err := store.Begin(context.Background(), testBeginRequest())
	if !errors.Is(err, ErrStoreUncertain) || intent.Key != "" {
		t.Fatalf("invalid receipt authorized a Host key: key present %t, error %v", intent.Key != "", err)
	}
}

func TestRemoteSnapshotCompetingStoresHaveOneWinner(t *testing.T) {
	f, _ := newRemoteFixture(t)
	first := openRemoteFixture(t)
	second := openRemoteFixture(t)
	f.mu.Lock()
	f.readBarrier = make(chan struct{})
	f.mu.Unlock()
	results := make(chan error, 2)
	for _, store := range []*Store{first, second} {
		go func(s *Store) {
			_, err := s.Begin(context.Background(), testBeginRequest())
			results <- err
		}(store)
	}
	one, two := <-results, <-results
	if (one == nil) == (two == nil) || one != nil && !errors.Is(one, ErrStoreConflict) || two != nil && !errors.Is(two, ErrStoreConflict) {
		t.Fatalf("competing claims = %v, %v; want one ACK and one conflict", one, two)
	}
	f.mu.Lock()
	f.readBarrier = nil
	f.mu.Unlock()
	records, err := first.Read(context.Background(), testBeginRequest().Target)
	if err != nil || len(records) != 1 {
		t.Fatalf("winner snapshot invalid: %v, %d records", err, len(records))
	}
}

func TestRemoteSnapshotRejectsForeignBindingStaleGrantAndTamper(t *testing.T) {
	f, _ := newRemoteFixture(t)
	store := openRemoteFixture(t)
	if _, err := OpenRemote(context.Background(), "intent-one", "foreign-store", testStoreBinding()); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("foreign store adopted: %v", err)
	}
	if _, err := OpenRemote(context.Background(), "intent-one", testStoreID, StoreBinding{HostAPI: "https://foreign.example/api"}); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("foreign Host adopted: %v", err)
	}
	f.mu.Lock()
	f.denied = true
	f.mu.Unlock()
	if _, err := store.Begin(context.Background(), testBeginRequest()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("stale grant accepted: %v", err)
	}
	f.mu.Lock()
	f.denied, f.tamper = false, true
	f.mu.Unlock()
	if _, err := store.Begin(context.Background(), testBeginRequest()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("tampered digest accepted: %v", err)
	}
}

func TestRemoteSnapshotRejectsRedirectOversizeAndLeaks(t *testing.T) {
	f, server := newRemoteFixture(t)
	var redirected atomic.Int32
	unsafeTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer unsafeTarget.Close()
	for _, endpoint := range []string{
		"http://example.com" + privateRunFilesPath,
		server.URL + privateRunFilesPath + "?token=secret",
		server.URL + privateRunFilesPath + "#fragment",
		"http://user@127.0.0.1" + privateRunFilesPath,
	} {
		if validPrivateRunFilesEndpoint(endpoint) {
			t.Fatalf("unsafe endpoint accepted: %q", endpoint)
		}
	}
	store := openRemoteFixture(t)
	f.mu.Lock()
	f.redirect = true
	f.redirectURL = unsafeTarget.URL + "/steal"
	f.mu.Unlock()
	if _, err := store.Begin(context.Background(), testBeginRequest()); err == nil || strings.Contains(err.Error(), f.grant) {
		t.Fatalf("redirect followed or grant leaked: %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatal("remote store followed a redirect with its grant")
	}
	f.mu.Lock()
	f.redirect = false
	f.data = make([]byte, maxSnapshotBytes+1)
	f.mu.Unlock()
	if _, err := store.Begin(context.Background(), testBeginRequest()); !errors.Is(err, ErrStoreUnavailable) || strings.Contains(err.Error(), f.grant) {
		t.Fatalf("oversize snapshot accepted or grant leaked: %v", err)
	}
}
