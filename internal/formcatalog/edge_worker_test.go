package formcatalog

import "testing"

func TestEdgeWorkerIsPortableExecutionIntentRatherThanHTTPInterfaceName(t *testing.T) {
	t.Parallel()
	edge, ok := ByKind("EdgeWorker")
	if !ok {
		t.Fatal("Legacy compatibility catalog omits EdgeWorker")
	}
	if edge.Version() != "4.0.0" || edge.Slug != "edge-worker" ||
		edge.ResourceType != "takoform_edge_worker" {
		t.Fatalf("unexpected Legacy EdgeWorker identity: %#v", edge)
	}
	if _, ok := ByKind("HttpService"); ok {
		t.Fatal("over-broad HttpService remains in the compatibility catalog")
	}
	desired := edge.CanonicalDesired()
	for _, providerSpecific := range []string{"compatibilityDate", "compatibilityFlags"} {
		if _, exists := desired[providerSpecific]; exists {
			t.Fatalf("portable EdgeWorker retains provider-specific field %s", providerSpecific)
		}
	}
	if desired["runtime"] != "javascript" {
		t.Fatalf("portable runtime intent is missing: %#v", desired)
	}
	if len(edge.Interfaces) != 1 || edge.Interfaces[0].Name != "http.request" {
		t.Fatalf("HTTP capability must remain an Interface declaration: %#v", edge.Interfaces)
	}
}
