package portableconformancev3

// declared_outputs_test.go is the teeth of the declared-output check
// (spec/decisions/0025): the corpus pins the Form's own output contract, a host
// that answers with a well-formed-LOOKING value the contract forbids fails, and
// a host that answers with a legal non-string value passes.
//
// The last one has no Form in the lane yet — WorkerEndpoint publishes two
// strings — and that is exactly why it is written. A check that assumed
// "output" meant "non-empty string" would pass every host today and reject the
// first Form that declared an integer, which the authoring model permits.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// TestCorpusPinsTheFormsOwnOutputSchema proves the pinned contract IS the
// Form's, at the exact FormRef the corpus pins.
//
// The corpus carries the output contract because no wire surface serves one:
// the published form-definition response is closed and its bytes are immutable
// (decision 0014). Carrying it is only honest while it cannot drift from the
// Definition a host installs, which is what this compares.
func TestCorpusPinsTheFormsOwnOutputSchema(t *testing.T) {
	root := corpusRoot(t)
	contract, err := Verify(root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(root, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalog(repoRoot, contract)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	declaring := 0
	for _, probe := range allResourceProbes(contract.RunnerInput) {
		form := catalog.exact(probe.Identity.FormRef)
		if form == nil {
			t.Fatalf("the catalog installs no Form at the pinned identity %s", probe.Identity.FormRef.Kind)
		}
		// Canonical bytes, not decoded values: the corpus decodes its numbers as
		// exact literals and the Definition loader need not, and what has to
		// agree is the CONTRACT rather than one loader's Go types.
		corpus := canonicalSchema(t, probe.DeclaredOutputSchema)
		declared := canonicalSchema(t, form.OutputSchema)
		if corpus != declared {
			t.Fatalf(
				"the corpus pins a %s output contract the installed Definition does not declare:\ncorpus: %s\nform:   %s",
				probe.Identity.FormRef.Kind, corpus, declared,
			)
		}
		if probe.declaresOutputs() {
			declaring++
		}
	}
	if declaring != 1 {
		t.Fatalf("%d probes declare an output contract; the lane has exactly one, so this comparison is not what it claims", declaring)
	}
}

// TestDeclaredOutputCheckFailsAWellFormedLookingViolation is the first tooth.
//
// The host answers the endpoint with `hostname: "not a hostname"` and the
// MATCHING `url`, so the two members agree, the scheme is https, the path root
// is `/`, and the address is stable across reads — every assertion the endpoint
// checks make. Only the declared schema says the value is not a hostname, so
// only a check that validates against it can fail this host.
func TestDeclaredOutputCheckFailsAWellFormedLookingViolation(t *testing.T) {
	report, err := runAgainstRewrittenEndpointOutputs(t, constantAddress(map[string]any{
		"hostname": "not a hostname",
		"url":      "https://not a hostname/",
	}))
	if err == nil {
		t.Fatalf("a host publishing an address its Form's outputSchema forbids passed the lane: %+v", report)
	}
	for _, want := range []string{"WorkerEndpoint", "outputSchema", "pattern"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("failure %q does not name %q; the diagnostic must say what was violated", err, want)
		}
	}
}

// TestDeclaredOutputCheckFailsAWrongTypedValue is the same tooth on the other
// axis: the member is present and non-empty, and its TYPE is not the declared
// one. A member-name comparison accepts it.
func TestDeclaredOutputCheckFailsAWrongTypedValue(t *testing.T) {
	if _, err := runAgainstRewrittenEndpointOutputs(t, constantAddress(map[string]any{
		"hostname": 8443,
		"url":      "https://e0123456789abcdef.endpoints.portable-conformance.invalid/",
	})); err == nil {
		t.Fatal("a host publishing a number where its Form declares a string passed the lane")
	}
}

// TestEndpointLaneAcceptsAnAddressDerivedFromTheResourceName is the
// counterpart, and the reason the address checks assert nothing about shape.
//
// This host allocates `<resource name>.tenant.example`. That is a legal
// allocation: decision 0024 rule 3 leaves which label, which subdomain, and
// whether the address resembles the resource name entirely to the host, and the
// endpoint's desired state carries no address for such a host to have echoed.
// The lane must pass it end to end, and a check that banned the resource name
// appearing in the address would fail it for making a decision the contract
// gave it.
func TestEndpointLaneAcceptsAnAddressDerivedFromTheResourceName(t *testing.T) {
	report, err := runAgainstRewrittenEndpointOutputs(t, func(name string) map[string]any {
		hostname := name + ".tenant.example"
		return map[string]any{"hostname": hostname, "url": "https://" + hostname + "/"}
	})
	if err != nil {
		t.Fatalf("a host naming its endpoints after the resource failed the lane: %v", err)
	}
	if report.Status != "passed" {
		t.Fatalf("report status = %q, want passed", report.Status)
	}
}

// TestEndpointLaneRejectsOneAddressForTwoWorkers is what replaces the banned
// shape assertion.
//
// This host assigns one perfectly legal address — it satisfies the declared
// schema, it is https, its members agree, and it never moves — to every
// endpoint it serves. Every per-resource assertion passes, and the Form's
// guarantee is still false: one address at one path root cannot invoke the
// active deployments of two different workers.
func TestEndpointLaneRejectsOneAddressForTwoWorkers(t *testing.T) {
	_, err := runAgainstRewrittenEndpointOutputs(t, constantAddress(map[string]any{
		"hostname": "shared.endpoints.portable-conformance.invalid",
		"url":      "https://shared.endpoints.portable-conformance.invalid/",
	}))
	if err == nil {
		t.Fatal("a host answering every endpoint with one address passed the lane")
	}
	if !strings.Contains(err.Error(), "two workers") {
		t.Errorf("failure %q does not say that one address cannot serve two workers", err)
	}
}

// constantAddress is a host that answers every endpoint with one document.
func constantAddress(outputs map[string]any) func(string) map[string]any {
	return func(string) map[string]any { return outputs }
}

// TestDeclaredOutputContractAcceptsALegalNonStringOutput is the second tooth.
//
// It renders an output contract through the SAME authoring model the family's
// Forms are rendered from, declaring an integer and a boolean — both of which
// decision 0025 permits and no Form in this lane declares yet — and holds a
// conforming document to it. A validator that assumed strings would fail here,
// which is the regression this test exists to catch before a Form declares one.
func TestDeclaredOutputContractAcceptsALegalNonStringOutput(t *testing.T) {
	t.Parallel()
	form := currentformmodel.Form{
		Family: currentformmodel.Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"},
		Kind:   "SyntheticOutputProbe",
		Title:  "Synthetic Output Probe",
		Outputs: []currentformmodel.Field{
			{HCL: "port", Wire: "port", Kind: currentformmodel.KindInteger, Doc: "Port the host bound."},
			{HCL: "secure", Wire: "secure", Kind: currentformmodel.KindBoolean, Doc: "Whether TLS terminates here."},
		},
	}
	declared, err := form.OutputSchema()
	if err != nil {
		t.Fatalf("render the output schema: %v", err)
	}
	probe := ResourceProbe{
		Identity:             InstalledFormReference{FormRef: FormRef{Kind: form.Kind}},
		DeclaredOutputSchema: declared,
	}
	members, schema, err := probe.outputContract()
	if err != nil {
		t.Fatalf("compile the output contract: %v", err)
	}
	if !reflect.DeepEqual(members, []string{"port", "secure"}) {
		t.Fatalf("declared members = %v, want [port secure]", members)
	}
	legal := decodedOutputs(t, `{"port":8443,"secure":true}`)
	if err := validateDeclaredOutputs(schema, legal); err != nil {
		t.Fatalf("a host returning the declared integer and boolean was rejected: %v", err)
	}
	stringed := decodedOutputs(t, `{"port":"8443","secure":"true"}`)
	if err := validateDeclaredOutputs(schema, stringed); err == nil {
		t.Fatal("a host returning strings where the Form declares an integer and a boolean was accepted")
	}
}

// canonicalSchema renders one schema as sorted-key JSON with exact numeric
// literals, so two loaders' decodings of the same contract compare equal.
func canonicalSchema(t *testing.T, schema map[string]any) string {
	t.Helper()
	if schema == nil {
		return ""
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("encode schema: %v", err)
	}
	return string(raw)
}

// decodedOutputs decodes one outputs document the way the runner decodes a host
// response, so a number arrives as the exact literal the host sent.
func decodedOutputs(t *testing.T, raw string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var outputs map[string]any
	if err := decoder.Decode(&outputs); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return outputs
}

// allResourceProbes is every probe the corpus pins, read from the one
// enumeration the contract keeps, so this comparison cannot quietly skip a
// probe someone adds later.
func allResourceProbes(input RunnerInput) []ResourceProbe {
	probes := make([]ResourceProbe, 0, 14)
	for _, entry := range probeInventory(&input) {
		probes = append(probes, *entry.Probe)
	}
	return probes
}

// runAgainstRewrittenEndpointOutputs drives the complete matrix against the
// reference host with one thing changed: every WorkerEndpoint representation
// leaves the host carrying the `status.outputs` document `assign` computes from
// the resource's name.
//
// It rewrites the RESPONSE rather than the host so the host stays the one the
// self-test proves; the mutation is the black-box behavior of some other host,
// which is what the runner is for.
func runAgainstRewrittenEndpointOutputs(
	t *testing.T,
	assign func(name string) map[string]any,
) (HostRunnerReport, error) {
	t.Helper()
	root := corpusRoot(t)
	contract, err := Verify(root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(root, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalog(repoRoot, contract)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	server := httptest.NewServer(&endpointOutputRewriter{
		inner:  NewReferenceHost(contract, catalog),
		assign: assign,
	})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return RunEndpoint(ctx, contract, EndpointOptions{
		Endpoint:             server.URL,
		Token:                referencePrimaryToken,
		AlternateToken:       referenceAlternateToken,
		AlternateTenantToken: referenceAlternateTenantToken,
		HTTPClient:           server.Client(),
		Classification:       ReferenceHostSelfTest,
		Subject:              "rewritten-endpoint-outputs",
	})
}

// endpointOutputRewriter is a disposable host that answers exactly like the
// reference host except for one Form's published outputs.
type endpointOutputRewriter struct {
	inner  http.Handler
	assign func(name string) map[string]any
}

func (rewriter *endpointOutputRewriter) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	recorder := httptest.NewRecorder()
	rewriter.inner.ServeHTTP(recorder, request)
	body := recorder.Body.Bytes()
	if rewritten, ok := rewriteEndpointOutputs(body, rewriter.assign); ok {
		body = rewritten
	}
	for key, values := range recorder.Header() {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(recorder.Code)
	_, _ = w.Write(body)
}

// rewriteEndpointOutputs replaces the outputs of one WorkerEndpoint
// representation, leaving every other response byte-identical.
func rewriteEndpointOutputs(body []byte, assign func(name string) map[string]any) ([]byte, bool) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, false
	}
	// A read answers with the resource document; `observe` answers with it
	// wrapped in a `resource` envelope. Both are representations of the same
	// resource, so a host that rewrote one and not the other would be two hosts.
	resource := document
	envelope := false
	if nested, wrapped := document["resource"].(map[string]any); wrapped {
		resource = nested
		envelope = true
	}
	if kind, _ := resource["kind"].(string); kind != workerEndpointKind {
		return nil, false
	}
	status, _ := resource["status"].(map[string]any)
	if status == nil {
		return nil, false
	}
	metadata, _ := resource["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)
	status["outputs"] = assign(name)
	if envelope {
		document["resource"] = resource
	}
	rewritten, err := json.Marshal(document)
	if err != nil {
		return nil, false
	}
	return rewritten, true
}
