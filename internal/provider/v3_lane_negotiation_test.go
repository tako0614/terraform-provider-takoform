package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The dedicated discovery deadline is the property that survived the
// single-lane collapse: an endpoint that accepts the connection and never
// answers discovery must cost Configure the short discovery budget, not the
// resource-operation timeout. The dual-lane independence tests that used to
// live here went with the withdrawn v1alpha2 lane (decision 0042).
func TestLaneNegotiationIsBoundedByTheDiscoveryDeadline(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	// Deferred LIFO on purpose: the handler must be released BEFORE Close
	// waits for it, or the cleanup deadlocks on its own fixture.
	defer hanging.Close()
	defer close(release)

	started := time.Now()
	clientV3, err := negotiateLane(
		context.Background(), hanging.URL, "token", hanging.Client(), 150*time.Millisecond,
	)
	elapsed := time.Since(started)
	if err == nil || clientV3 != nil {
		t.Fatalf("a hanging endpoint negotiated: client=%v err=%v", clientV3, err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("negotiation was bounded by something other than the discovery deadline: %s", elapsed)
	}
}

// A conforming v1beta1 host negotiates, and the negotiated client is usable.
func TestLaneNegotiationAcceptsAConformingHost(t *testing.T) {
	t.Parallel()

	host := newV3FakeHost(t)

	clientV3, err := negotiateLane(
		context.Background(), host.server.URL, "test-token", host.server.Client(), 2*time.Second,
	)
	if err != nil {
		t.Fatalf("conforming host did not negotiate: %v", err)
	}
	if clientV3 == nil {
		t.Fatal("negotiation returned no client")
	}
}
