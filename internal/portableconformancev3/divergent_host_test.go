package portableconformancev3

// divergent_host_test.go holds the two hosts this lane exists to fail, and it
// holds them together because they are one defect wearing two faces: an oracle
// that lets the subject supply the standard it is measured against.
//
// Both were passed by the 112-check corpus that preceded them. That is what
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
	if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/import") {
		if rewritten, ok := blinder.blind(request); ok {
			request = rewritten
		}
	}
	blinder.inner.ServeHTTP(w, request)
}

func (blinder *nativeIdentityBlinder) blind(request *http.Request) (*http.Request, bool) {
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
	if _, present := document["nativeId"]; !present {
		return nil, false
	}
	blinder.mu.Lock()
	blinder.minted++
	document["nativeId"] = "blinded/" + strconv.Itoa(blinder.minted)
	blinder.mu.Unlock()
	rewritten, err := json.Marshal(document)
	if err != nil {
		return nil, false
	}
	next := request.Clone(request.Context())
	next.Body = io.NopCloser(bytes.NewReader(rewritten))
	next.ContentLength = int64(len(rewritten))
	return next, true
}
