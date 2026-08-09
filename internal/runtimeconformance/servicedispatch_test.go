package runtimeconformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/fakeruntime"
)

// The two `worker.service` checks measure a DISPATCH. What they used to measure
// was a route: the peer ran the caller's own byte-pinned bundle, so a host that
// answered `env.PEER.fetch(...)` out of the caller's fetch handler produced the
// same accounting, the same chunk timing and the same status as the projection
// it never implemented, and passed both. The tests below drive the short
// circuit itself and require the corpus to refuse it, and then keep the corpus
// from being edited back into accepting it.

// runServiceChecks drives the two worker-to-worker checks against one stand-in.
func runServiceChecks(t *testing.T, contract Contract, options fakeruntime.Options) map[string]CheckEvidence {
	t.Helper()
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}
	runtime, err := fakeruntime.New(deployment, options)
	if err != nil {
		t.Fatalf("stand-in: %v", err)
	}
	defer runtime.Close()
	worker := httptest.NewServer(runtime.WorkerHandler())
	defer worker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := worker.Client()
	client.Timeout = 2 * time.Minute
	outcomes := map[string]CheckEvidence{}
	for _, name := range serviceBindingChecks {
		outcomes[name] = runCheck(ctx, contract, &target{
			client: client, worker: worker.URL, runToken: "servicedispatch",
		}, checkFor(t, contract, name))
	}
	return outcomes
}

// TestAShortCircuitedServiceCallFailsBothServiceChecks is the line this fix is
// verified by. The runtime is conforming in every other respect — it streams
// both directions incrementally, it separates its chunks in time, it answers
// the response head before the body — and it never dispatches: the binding
// re-enters the caller's own fetch handler. Before the peer carried an identity
// its bytes alone can produce, both checks passed it.
func TestAShortCircuitedServiceCallFailsBothServiceChecks(t *testing.T) {
	contract := fastCorpus(t)
	outcomes := runServiceChecks(t, contract, fakeruntime.Options{ShortCircuitServiceBinding: true})
	for _, name := range serviceBindingChecks {
		evidence := outcomes[name]
		if evidence.Outcome != OutcomeFailed {
			t.Fatalf("%q passed a runtime that answered the service call out of the caller: %s",
				name, evidence.Observed)
		}
		if !strings.Contains(evidence.Detail, "no callee identity") {
			t.Fatalf("the failure of %q must name what was missing, got %q", name, evidence.Detail)
		}
		if !strings.Contains(evidence.Detail, contract.Deployment.Peer.Identity) {
			t.Fatalf("the failure of %q must name the callee it expected, got %q", name, evidence.Detail)
		}
	}
}

// TestTheDispatchingRuntimePassesTheSameChecks is the other half, and the
// reason the refusal above is a measurement rather than a wall: the same
// stand-in with the binding wired to the peer passes both.
func TestTheDispatchingRuntimePassesTheSameChecks(t *testing.T) {
	contract := fastCorpus(t)
	outcomes := runServiceChecks(t, contract, fakeruntime.Options{})
	for _, name := range serviceBindingChecks {
		evidence := outcomes[name]
		if evidence.Outcome != OutcomePassed {
			t.Fatalf("%q failed a runtime that dispatched: %s (%s)", name, evidence.Detail, evidence.Observed)
		}
		if !strings.Contains(evidence.Observed, contract.Deployment.Peer.Identity) {
			t.Fatalf("a passed %q must record which callee answered, got %q", name, evidence.Observed)
		}
	}
}

// TestAServiceCheckCannotStopCrossingTheBinding is the tooth under the rule.
// Dropping the marker is exactly the edit that restores the defect, so the
// loader refuses the corpus by name rather than running two checks a
// self-binding passes.
func TestAServiceCheckCannotStopCrossingTheBinding(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		delete(checkNamed(document, "service-response-body-streams-from-the-callee"), "throughBinding")
	})
	if _, err := Verify(root); err == nil ||
		!strings.Contains(err.Error(), "does not state the binding it crosses") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestThePeerIdentityMustComeFromThePeersOwnBytes refuses an identity the peer
// does not actually stamp, and one no run could read at all. A corpus stating
// an identity the callee never emits fails every conforming runtime; the
// loader says so rather than letting a run discover it.
func TestThePeerIdentityMustComeFromThePeersOwnBytes(t *testing.T) {
	for name, expect := range map[string]struct {
		identity string
		detail   string
	}{
		"an identity the peer does not stamp": {
			identity: "takoform-runtime-abi-peer-0000000000000000",
			detail:   "the corpus states what the peer",
		},
		"an identity outside the portable grammar": {
			identity: "peer",
			detail:   "not a portable identity",
		},
	} {
		root := copyCorpus(t)
		mutateCorpus(t, root, func(document map[string]any) {
			document["deployment"].(map[string]any)["peer"].(map[string]any)["identity"] = expect.identity
		})
		_, err := Verify(root)
		if err == nil {
			t.Fatalf("%s: the corpus was accepted", name)
		}
		if !strings.Contains(err.Error(), expect.detail) {
			t.Fatalf("%s: refusal %q does not say why", name, err)
		}
	}
}

// TestAnIdentityTheCallerAlsoCarriesIsRefused is the half that makes the
// identity worth having, stated as the byte fact it is.
//
// A peer that stamps its identity truthfully still proves nothing if the
// CALLER's bytes contain the same string: the caller could stamp it too, and
// the short circuit would pass again wearing the fix as a disguise. The loader
// therefore scans every other bundle of the corpus, so the caller's own pinned
// module carrying the string — even in a comment it never reads — is a corpus
// that fails verification.
func TestAnIdentityTheCallerAlsoCarriesIsRefused(t *testing.T) {
	root := copyCorpus(t)
	identity := verifiedContract(t).Deployment.Peer.Identity
	rewriteCorpusModule(t, root, filepath.Join("bundles", "conformance-probe", "index.js"),
		func(source []byte) []byte {
			return append([]byte("// "+identity+"\n"), source...)
		})
	_, err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "also appears in") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// rewriteCorpusModule rewrites one module of a copied corpus and re-pins every
// digest that named it, so a test states the MODULE-BYTE rule rather than the
// pin that guards the bytes.
func rewriteCorpusModule(t *testing.T, root, relative string, rewrite func([]byte) []byte) {
	t.Helper()
	path := filepath.Join(root, relative)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read module: %v", err)
	}
	rewritten := rewrite(source)
	if err := os.WriteFile(path, rewritten, 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	before := "sha256:" + hex.EncodeToString(hashOf(source))
	after := "sha256:" + hex.EncodeToString(hashOf(rewritten))
	mutateCorpus(t, root, func(document map[string]any) {
		for _, entry := range document["bundles"].([]any) {
			for _, moduleEntry := range entry.(map[string]any)["modules"].([]any) {
				module := moduleEntry.(map[string]any)
				if module["sha256"] == before {
					module["sha256"] = after
				}
			}
		}
	})
}

func hashOf(source []byte) []byte {
	sum := sha256.Sum256(source)
	return sum[:]
}
