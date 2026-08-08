package provider

// v3_lane_negotiation_test.go proves the property spec/decisions/0018 states
// about configuration: the two Host API lanes are negotiated independently and
// under a short dedicated deadline, so one unresponsive lane cannot make the
// other lane's resources unusable.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

// hangingLaneHost answers one lane's discovery immediately and never answers
// the other's. It is the shape a real deployment produces when one lane sits
// behind a dead upstream: the connection is accepted, the request is read, and
// no response ever comes.
func hangingLaneHost(t *testing.T, hangingPath string) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == hangingPath {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		switch r.URL.Path {
		case client.DiscoveryPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{client.APIVersion},
				"features": map[string]bool{
					"service_forms": true, "exact_form_ref": true,
					"optimistic_concurrency": true, "idempotent_lifecycle": true,
				},
				"endpoints": map[string]string{
					"api":   server.URL + "/apis/forms.takoform.com/v1alpha2",
					"forms": server.URL + "/apis/forms.takoform.com/v1alpha2/forms",
				},
			})
		case clientv3.DiscoveryPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{clientv3.APIVersion},
				"features": map[string]bool{
					"service_forms": true, "exact_form_ref": true,
					"optimistic_concurrency": true, "idempotent_lifecycle": true,
					"operations": true, "artifact_upload": true, "support_profiles": true,
				},
				"endpoints": map[string]string{"api": server.URL + clientv3.APIRootPath},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestOneHangingLaneLeavesTheOtherLaneUsable drives both directions of the same
// failure: whichever lane hangs, the other one negotiates, and the hung lane
// gives up on its OWN short deadline rather than holding configuration for the
// long resource-operation timeout.
func TestOneHangingLaneLeavesTheOtherLaneUsable(t *testing.T) {
	const negotiationTimeout = 300 * time.Millisecond
	tests := []struct {
		name        string
		hangingPath string
	}{
		{"the v1alpha3 lane hangs", clientv3.DiscoveryPath},
		{"the retained v1alpha2 lane hangs", client.DiscoveryPath},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			server := hangingLaneHost(t, testCase.hangingPath)
			// The negotiated clients keep the LONG resource-operation timeout; only
			// discovery is bounded by the short one, so this is the real pairing.
			httpClient := newResourceAPIHTTPClient()

			started := time.Now()
			v2Client, v3Client, v2Err, v3Err := negotiateLanes(
				context.Background(), server.URL, "token", httpClient, negotiationTimeout,
			)
			elapsed := time.Since(started)

			hungErr, hungLane := v3Err, "v1alpha3"
			liveClient, liveErr, liveLane := any(v2Client), v2Err, "v1alpha2"
			if testCase.hangingPath == client.DiscoveryPath {
				hungErr, hungLane = v2Err, "v1alpha2"
				liveClient, liveErr, liveLane = any(v3Client), v3Err, "v1alpha3"
			}
			if hungErr == nil {
				t.Fatalf("the %s lane hung and still reported success", hungLane)
			}
			if !strings.Contains(hungErr.Error(), "deadline exceeded") &&
				!strings.Contains(hungErr.Error(), "context canceled") {
				t.Fatalf("the %s lane failed for an unexpected reason: %v", hungLane, hungErr)
			}
			if liveErr != nil {
				t.Fatalf("the %s lane did not negotiate while the other lane hung: %v", liveLane, liveErr)
			}
			switch typed := liveClient.(type) {
			case *client.Client:
				if typed == nil {
					t.Fatalf("the %s lane returned no client", liveLane)
				}
			case *clientv3.Client:
				if typed == nil {
					t.Fatalf("the %s lane returned no client", liveLane)
				}
			}
			// Concurrency and the short deadline together: a serial negotiation under
			// the long resource timeout would still be blocked here.
			if elapsed >= negotiationTimeout*4 {
				t.Fatalf(
					"lane negotiation took %s; one hanging lane must cost at most its own %s deadline",
					elapsed, negotiationTimeout,
				)
			}
		})
	}
}

// TestBothLanesNegotiateConcurrently proves the two negotiations overlap rather
// than run one after the other. Each lane's discovery blocks until BOTH have
// arrived, which a serial negotiator can never satisfy.
func TestBothLanesNegotiateConcurrently(t *testing.T) {
	arrived := make(chan struct{}, 2)
	both := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case client.DiscoveryPath, clientv3.DiscoveryPath:
			arrived <- struct{}{}
			if len(arrived) == 2 {
				select {
				case <-both:
				default:
					close(both)
				}
			}
			select {
			case <-both:
			case <-time.After(10 * time.Second):
				t.Error("the two lane negotiations never overlapped: they are serial")
				return
			}
		default:
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == client.DiscoveryPath {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{client.APIVersion},
				"features": map[string]bool{
					"service_forms": true, "exact_form_ref": true,
					"optimistic_concurrency": true, "idempotent_lifecycle": true,
				},
				"endpoints": map[string]string{
					"api":   server.URL + "/apis/forms.takoform.com/v1alpha2",
					"forms": server.URL + "/apis/forms.takoform.com/v1alpha2/forms",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_versions": []string{clientv3.APIVersion},
			"features": map[string]bool{
				"service_forms": true, "exact_form_ref": true,
				"optimistic_concurrency": true, "idempotent_lifecycle": true,
				"operations": true, "artifact_upload": true, "support_profiles": true,
			},
			"endpoints": map[string]string{"api": server.URL + clientv3.APIRootPath},
		})
	}))
	defer server.Close()

	v2Client, v3Client, v2Err, v3Err := negotiateLanes(
		context.Background(), server.URL, "token", server.Client(), 20*time.Second,
	)
	if v2Err != nil || v3Err != nil {
		t.Fatalf("lane negotiation failed: v1alpha2=%v v1alpha3=%v", v2Err, v3Err)
	}
	if v2Client == nil || v3Client == nil {
		t.Fatal("both lanes reported success without returning a client")
	}
}
