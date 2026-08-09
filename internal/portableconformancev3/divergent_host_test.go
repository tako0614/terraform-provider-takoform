package portableconformancev3

// divergent_host_test.go holds the hosts this lane exists to fail, and it holds
// them together because they are one defect wearing several faces: an oracle
// that lets the subject supply the standard it is measured against, or that
// measures a narrower rule than the one the contract publishes.
//
// Every one of them was passed by the corpus that preceded it. That is what
// makes each a finding rather than a precaution, and each test says which check
// now refuses it.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// runAgainstWrappedReferenceHost drives the complete matrix against the
// reference host seen through one wrapper.
//
// The wrapper is the black-box behavior of some OTHER host: the reference host
// underneath stays the one the self-test proves, so a failure is attributable to
// the one thing the wrapper changed rather than to a second implementation
// written for the occasion.
func runAgainstWrappedReferenceHost(
	t *testing.T,
	subject string,
	wrap func(http.Handler) http.Handler,
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
	server := httptest.NewServer(wrap(NewReferenceHost(contract, catalog)))
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
		Subject:              subject,
	})
}

// ---- Hole 1: the host that publishes its own defaults ----

// TestServedDesiredSchemaMustBeThePinnedOne is the tooth of the served-schema
// pin (spec/decisions/0022, amended).
//
// This host is conforming in every way a black-box lifecycle can see EXCEPT
// one: the `AtLeastOnceQueue` Form Definition it serves declares
// `deliveryDelaySeconds` with the default 60 where the Definition at that exact
// FormRef declares 0. Nothing else differs — the identity it echoes is the
// requested one, the schema is well formed, the bound is legal, and the host
// stores and echoes exactly what it is sent.
//
// A runner that materializes probe specs against the SERVED schema agrees with
// it about every byte it then sends, and the whole matrix passes. A real client
// materializes the normative 0, computes a different `specDigest`, and has every
// `prepare` bound to a spec this host does not recognise — which is the failure
// mode with no local symptom at all.
//
// The old 112-check corpus passed this host. `form-definition-exact` now
// compares the served desired schema against the corpus-pinned bytes, so the
// divergence is named where it happens rather than absorbed.
func TestServedDesiredSchemaMustBeThePinnedOne(t *testing.T) {
	report, err := runAgainstWrappedReferenceHost(t, "served-desired-schema-drift", func(inner http.Handler) http.Handler {
		return &desiredSchemaRewriter{
			inner: inner,
			kind:  "AtLeastOnceQueue",
			rewrite: func(schema map[string]any) {
				properties, _ := schema["properties"].(map[string]any)
				delay, _ := properties["deliveryDelaySeconds"].(map[string]any)
				if delay != nil {
					delay["default"] = json.Number("60")
				}
			},
		}
	})
	if err == nil {
		t.Fatalf("a host serving a default its Definition does not declare passed the lane: %+v", report)
	}
	t.Logf("the lane refuses this host with: %v", err)
	for _, want := range []string{"form-definition", "AtLeastOnceQueue", "desiredSchema"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("failure %q does not name %q; the diagnostic must say which Form's schema drifted", err, want)
		}
	}
}

// desiredSchemaRewriter answers exactly like the reference host except that one
// kind's served `desiredSchema` is passed through `rewrite` first.
//
// It rewrites the RESPONSE and not the catalog, so the host's own
// materialization, validation, digesting, and echo all stay the conforming ones.
// That is the whole point: this is a host that is self-consistent and still
// unusable by a client holding the real Definition.
type desiredSchemaRewriter struct {
	inner   http.Handler
	kind    string
	rewrite func(schema map[string]any)
}

func (rewriter *desiredSchemaRewriter) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	recorder := httptest.NewRecorder()
	rewriter.inner.ServeHTTP(recorder, request)
	body := recorder.Body.Bytes()
	if rewritten, ok := rewriter.rewriteDefinition(body); ok {
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

func (rewriter *desiredSchemaRewriter) rewriteDefinition(body []byte) ([]byte, bool) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, false
	}
	identity, _ := document["identity"].(map[string]any)
	formRef, _ := identity["formRef"].(map[string]any)
	if kind, _ := formRef["kind"].(string); kind != rewriter.kind {
		return nil, false
	}
	schema, _ := document["desiredSchema"].(map[string]any)
	if schema == nil {
		return nil, false
	}
	rewriter.rewrite(schema)
	rewritten, err := json.Marshal(document)
	if err != nil {
		return nil, false
	}
	return rewritten, true
}

// ---- Hole 2: the host whose import is a create ----

// TestImportMustClaimTheNativeIdentity is the tooth of the adoption claim
// (spec/decisions/0011, amended).
//
// This host consults the `nativeId` it is given for nothing. Every adoption
// reaches the store as though the identity named had never been seen, which is
// the observable behavior of an `/import` implemented as a plain create: a
// create consults no native identity at all, so no sequence of adoptions can
// collide. Blinding the field rather than re-routing the request isolates
// exactly that variable — every other rule of the import path stays the
// conforming one, so a failure cannot be about validation, fences, claims, or
// replay.
//
// The practitioner cost is the one nothing local reports: `terraform import`
// against such a host mints a new backend object and orphans the one being
// adopted, and a second `terraform import` of the same object mints a second.
//
// The old 112-check corpus passed this host: `import-adopts-native-resource`
// sent a `nativeId` no answer ever had to reflect, and `import_conflict` had no
// organic producer anywhere in the matrix.
func TestImportMustClaimTheNativeIdentity(t *testing.T) {
	report, err := runAgainstWrappedReferenceHost(t, "import-ignores-native-identity", func(inner http.Handler) http.Handler {
		return &nativeIdentityBlinder{inner: inner}
	})
	if err == nil {
		t.Fatalf("a host whose import ignores the native identity it is given passed the lane: %+v", report)
	}
	t.Logf("the lane refuses this host with: %v", err)
	if !strings.Contains(err.Error(), "import_conflict") {
		t.Errorf("failure %q does not name the refusal a conforming host owes: import_conflict", err)
	}
}

// nativeIdentityBlinder replaces the `nativeId` of every adoption with one
// nothing else will ever name.
type nativeIdentityBlinder struct {
	inner http.Handler

	mu     sync.Mutex
	minted int
}

func (blinder *nativeIdentityBlinder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if rewritten, ok := rewriteAdoptedIdentity(request, func(_ map[string]any, _ string) string {
		blinder.mu.Lock()
		defer blinder.mu.Unlock()
		blinder.minted++
		return "blinded/" + strconv.Itoa(blinder.minted)
	}); ok {
		request = rewritten
	}
	blinder.inner.ServeHTTP(w, request)
}

// ---- Hole 3: the host that scopes the claim to one Form kind ----

// TestNativeIdentityClaimSpansEveryFormKind is the tooth of the claim's SCOPE
// (spec/decisions/0011, amended).
//
// This host records the adoption claim, holds it exclusively, releases it with
// its holder, and refuses to move it — inside one Form kind. Its index is
// `(tenant, kind, nativeId)`, which is what falls out of keeping the claim in
// the per-kind table the resource already lives in, and nothing about it looks
// like a defect from inside a check that derives holder and rival from one
// probe.
//
// It is a defect because a backend object has one identity no matter which Form
// adopted it. Managing it once as a queue and once as a KV namespace is the same
// doubled infrastructure, on the same bill, as managing it twice as a queue —
// and the contract says "one live resource per native identity for the tenant",
// with no kind in the key.
//
// The 114-check corpus that preceded this test passed this host, because
// `import-claims-its-native-identity` drove holder and rival through the KV
// probe alone.
func TestNativeIdentityClaimSpansEveryFormKind(t *testing.T) {
	report, err := runAgainstWrappedReferenceHost(
		t, "native-claim-scoped-per-form-kind",
		func(inner http.Handler) http.Handler { return &nativeIdentityKindScoper{inner: inner} },
	)
	if err == nil {
		t.Fatalf("a host holding one native identity once PER KIND passed the lane: %+v", report)
	}
	t.Logf("the lane refuses this host with: %v", err)
	for _, want := range []string{"import_conflict", "AtLeastOnceQueue", "EdgeKVNamespace"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("failure %q does not name %q; the diagnostic must say the claim crosses kinds", err, want)
		}
	}
}

// nativeIdentityKindScoper qualifies every adoption claim by the kind adopting
// it, so two Forms may adopt one object and no single-kind sequence can tell.
type nativeIdentityKindScoper struct{ inner http.Handler }

func (scoper *nativeIdentityKindScoper) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if rewritten, ok := rewriteAdoptedIdentity(request, func(document map[string]any, nativeID string) string {
		kind, _ := document["kind"].(string)
		return kind + "|" + nativeID
	}); ok {
		request = rewritten
	}
	scoper.inner.ServeHTTP(w, request)
}

// ---- Hole 4: the host that records the claim only when import creates ----

// TestFirstImportOfAHostCreatedResourceRecordsItsClaim is the tooth of the first
// adoption of a resource the host made itself (spec/decisions/0011, amended).
//
// This host records the `nativeId` where a create is the obvious place to record
// it — the branch that mints the resource — and treats the field as spent for a
// resource that already exists. Every adoption that BRINGS a resource into being
// claims its object correctly, so the exclusivity, the immutability, and the
// release all hold for resources born by `import`.
//
// The path it drops is the ordinary one: `terraform import` onto an address a
// configuration already manages, which is a fenced import of a resource this
// host created. That object's identity is never written down, so the next
// workspace adopts the same object unopposed, and the same address will later
// accept a DIFFERENT object without complaint — the resource can be walked onto
// another backend object one import at a time.
//
// The 114-check corpus that preceded this test passed this host, because
// `import-records-its-native-identity` built its target through a create-intent
// import, so the claim was already recorded before the first fenced case ran.
func TestFirstImportOfAHostCreatedResourceRecordsItsClaim(t *testing.T) {
	report, err := runAgainstWrappedReferenceHost(
		t, "native-claim-recorded-only-at-create",
		func(inner http.Handler) http.Handler {
			return &nativeClaimRecordedOnlyAtCreate{inner: inner, bornByImport: map[string]bool{}, blinded: map[string]string{}}
		},
	)
	if err == nil {
		t.Fatalf("a host recording the adopted identity only when import CREATES passed the lane: %+v", report)
	}
	t.Logf("the lane refuses this host with: %v", err)
	if !strings.Contains(err.Error(), "import_conflict") {
		t.Errorf("failure %q does not name the refusal a conforming host owes: import_conflict", err)
	}
}

// nativeClaimRecordedOnlyAtCreate passes create-intent adoptions through
// untouched and blinds every fenced adoption of an address no adoption created.
//
// Blinding with a value stable per address is what makes this the host described
// rather than a noisier one: the identity handed to a resource this host created
// is never recorded, and a later import of that same address is compared against
// the blind value instead of against what the client actually named — which is
// exactly "the field is read on the create branch and nowhere else".
type nativeClaimRecordedOnlyAtCreate struct {
	inner http.Handler

	mu           sync.Mutex
	bornByImport map[string]bool
	blinded      map[string]string
}

func (host *nativeClaimRecordedOnlyAtCreate) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	createIntent := request.Header.Get("If-None-Match") == "*"
	caller := request.Header.Get("Authorization") + " " + request.URL.Path
	if rewritten, ok := rewriteAdoptedIdentity(request, func(document map[string]any, nativeID string) string {
		metadata, _ := document["metadata"].(map[string]any)
		space, _ := metadata["space"].(string)
		address := caller + " " + space
		host.mu.Lock()
		defer host.mu.Unlock()
		if createIntent {
			host.bornByImport[address] = true
			return nativeID
		}
		if host.bornByImport[address] {
			return nativeID
		}
		if blind, assigned := host.blinded[address]; assigned {
			return blind
		}
		blind := "unrecorded/" + strconv.Itoa(len(host.blinded)+1)
		host.blinded[address] = blind
		return blind
	}); ok {
		request = rewritten
	}
	host.inner.ServeHTTP(w, request)
}

// rewriteAdoptedIdentity re-encodes one adoption under whatever native identity
// `next` answers for it, and leaves every other request exactly as it arrived.
func rewriteAdoptedIdentity(
	request *http.Request,
	next func(document map[string]any, nativeID string) string,
) (*http.Request, bool) {
	if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/import") {
		return nil, false
	}
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, false
	}
	nativeID, present := document["nativeId"].(string)
	if !present {
		return nil, false
	}
	document["nativeId"] = next(document, nativeID)
	rewritten, err := json.Marshal(document)
	if err != nil {
		return nil, false
	}
	replacement := request.Clone(request.Context())
	replacement.Body = io.NopCloser(bytes.NewReader(rewritten))
	replacement.ContentLength = int64(len(rewritten))
	return replacement, true
}
