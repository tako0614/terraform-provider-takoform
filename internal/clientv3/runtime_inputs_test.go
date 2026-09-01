package clientv3

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	testRuntimeOperationKey = "takoform-worker-runtime-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testRuntimeSecret       = "private-runtime-value-do-not-leak"
)

func TestRuntimeInputPublicApplyCommitmentCrossLanguageVector(t *testing.T) {
	commitment, err := RuntimeInputPublicApplyCommitment(
		"PUT",
		"/apis/forms.takoform.com/v1/resources/edge.forms.takoform.com/v1beta1/Worker/app",
		"*",
		[]byte(`{"x":"snowman-☃"}`),
	)
	if err != nil {
		t.Fatalf("derive public apply commitment: %v", err)
	}
	const want = "sha256:8f0b6943f90f58ab75a194b84eea1958af202d1f8b8cad35549a0144daf7e8b2"
	if commitment != want {
		t.Fatalf("public apply commitment = %q, want literal cross-language vector %q", commitment, want)
	}
}

func TestRuntimeInputPublicApplyCommitmentEnforcesOneMiBBodyBoundary(t *testing.T) {
	tests := map[string]struct {
		bodySize int
		wantErr  bool
	}{
		"exactly 1048576 bytes": {bodySize: 1_048_576},
		"1048577 bytes":         {bodySize: 1_048_577, wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			const emptyBody = `{"padding":""}`
			body := []byte(`{"padding":"` + strings.Repeat("x", test.bodySize-len(emptyBody)) + `"}`)
			if len(body) != test.bodySize {
				t.Fatalf("boundary public apply body = %d bytes, want %d", len(body), test.bodySize)
			}
			commitment, err := RuntimeInputPublicApplyCommitment(
				http.MethodPut,
				"/apis/forms.takoform.com/v1/resources/edge.forms.takoform.com/v1beta1/Worker/app",
				"*",
				body,
			)
			if test.wantErr {
				if err == nil || commitment != "" {
					t.Fatalf("%d-byte public apply body commitment = %q, error = %v; want no commitment and an error", test.bodySize, commitment, err)
				}
				return
			}
			if err != nil || commitment == "" {
				t.Fatalf("%d-byte public apply body commitment = %q, error = %v", test.bodySize, commitment, err)
			}
		})
	}
}

func TestRuntimeInputPublicApplyCommitmentRejectsUnencodableOrOverlongFields(t *testing.T) {
	tests := map[string]struct {
		method string
		path   string
		fence  string
		body   []byte
	}{
		"wrong method":      {method: "POST", path: "/resource", fence: "*", body: []byte(`{}`)},
		"overlong path":     {method: "PUT", path: "/" + strings.Repeat("p", runtimeInputPublicApplyMaximumPathBytes), fence: "*", body: []byte(`{}`)},
		"wrong fence":       {method: "PUT", path: "/resource", fence: `"revision"`, body: []byte(`{}`)},
		"invalid utf8":      {method: "PUT", path: "/resource", fence: "*", body: []byte{0xff}},
		"invalid path utf8": {method: "PUT", path: "/" + string([]byte{0xff}), fence: "*", body: []byte(`{}`)},
		"empty body":        {method: "PUT", path: "/resource", fence: "*", body: nil},
		"relative target":   {method: "PUT", path: "resource", fence: "*", body: []byte(`{}`)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if commitment, err := RuntimeInputPublicApplyCommitment(test.method, test.path, test.fence, test.body); err == nil {
				t.Fatalf("invalid public apply commitment input produced %q", commitment)
			}
		})
	}
}

func newRuntimeInputTestClient(t *testing.T, handle func(http.ResponseWriter, *http.Request) bool) *Client {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == DiscoveryPath {
			writeJSON(t, response, http.StatusOK, discoveryDoc(server.URL))
			return
		}
		if handle != nil && handle(response, request) {
			return
		}
		t.Errorf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
		http.Error(response, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := New(server.URL, "test-token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	return client
}

func testRuntimePublicApplyCommitment(t *testing.T, spec map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(applyRequestBody{
		resourceRequestBody: requestBodyFor(testResourceRequest(spec)),
		Review:              applyReview{PrepareDigest: testPrepareDigest},
	})
	if err != nil {
		t.Fatalf("marshal test public apply: %v", err)
	}
	commitment, err := RuntimeInputPublicApplyCommitment(
		http.MethodPut, splitGroupResourcePath("app", ""), "*", raw,
	)
	if err != nil {
		t.Fatalf("derive test public apply commitment: %v", err)
	}
	return commitment
}

func testRuntimeSpecForPublicApplyBodySize(t *testing.T, target int) map[string]any {
	t.Helper()
	spec := map[string]any{
		"payload":               "",
		"requiredSensitiveVars": []any{"API_TOKEN"},
	}
	encode := func() []byte {
		raw, err := json.Marshal(applyRequestBody{
			resourceRequestBody: requestBodyFor(testResourceRequest(spec)),
			Review:              applyReview{PrepareDigest: testPrepareDigest},
		})
		if err != nil {
			t.Fatalf("marshal sized public apply body: %v", err)
		}
		return raw
	}
	base := encode()
	if target < len(base) {
		t.Fatalf("target public apply body size %d is smaller than its %d-byte envelope", target, len(base))
	}
	spec["payload"] = strings.Repeat("x", target-len(base))
	if raw := encode(); len(raw) != target {
		t.Fatalf("sized public apply body = %d bytes, want %d", len(raw), target)
	}
	return spec
}

func TestApplyResourceWithRuntimeInputsRejects1048577BytePublicBodyBeforeMutation(t *testing.T) {
	spec := testRuntimeSpecForPublicApplyBodySize(t, 1_048_577)
	var privateCalls, publicPuts int
	client := newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case strings.HasPrefix(r.URL.Path, runtimeInputPreparationRoot+"/"):
			privateCalls++
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			return true
		}
		return false
	})
	bindings := map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)}
	result, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), bindings,
	)
	if err == nil || result != nil {
		t.Fatalf("1048577-byte public apply body result = %#v, error = %v; want rejection", result, err)
	}
	if privateCalls != 0 || publicPuts != 0 {
		t.Fatalf("overlong public apply body reached private/public mutation: private=%d public=%d", privateCalls, publicPuts)
	}
	if len(bindings) != 0 {
		t.Fatalf("overlong public apply body left %d runtime bindings reachable", len(bindings))
	}
}

func TestApplyResourceWithRuntimeInputsReadsBeforeFirstPrivatePut(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	var sequence []string
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			sequence = append(sequence, "private "+r.Method)
			if r.Method == http.MethodGet {
				writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
				return true
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			sequence = append(sequence, "public PUT")
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err != nil {
		t.Fatalf("apply with GET-first runtime inputs: %v", err)
	}
	want := []string{"private GET", "private PUT", "public PUT"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("runtime-input request order = %v, want %v", sequence, want)
	}
}

func TestApplyResourceWithRuntimeInputsResumesAcceptedPreparationWithoutMutation(t *testing.T) {
	for _, status := range []string{"accepted", "dispatched", "consumed"} {
		t.Run(status, func(t *testing.T) {
			spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
			const hostOperationID = "op_existing_runtime"
			var privateGets, privatePuts, publicPuts, operationGets int
			var client *Client
			client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
					writeJSON(t, w, http.StatusOK, wireAvailability("create"))
					return true
				case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
					handlePrepare(t, w, r)
					return true
				case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
					if r.Method == http.MethodPut {
						privatePuts++
					} else {
						privateGets++
					}
					writeJSON(t, w, http.StatusOK, map[string]any{
						"format":                runtimeInputPreparationFormat,
						"status":                status,
						"operationKey":          testRuntimeOperationKey,
						"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
						"canonicalPublicOrigin": client.Endpoint(),
						"bindingNames":          []string{"API_TOKEN"},
						"hostOperationId":       hostOperationID,
					})
					return true
				case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
					publicPuts++
					return true
				case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+hostOperationID:
					operationGets++
					writeJSON(t, w, http.StatusOK, map[string]any{
						"apiVersion": OperationAPIVersion,
						"kind":       OperationKind,
						"id":         hostOperationID,
						"done":       true,
						"result": map[string]any{
							"resource": wireResource("app", "uid-1", "1", "7", spec),
						},
					})
					return true
				}
				return false
			})

			out, err := client.ApplyResourceWithRuntimeInputs(
				context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
				client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
			)
			if err != nil {
				t.Fatalf("resume %s preparation: %v", status, err)
			}
			if out.Metadata.UID != "uid-1" {
				t.Fatalf("resumed resource uid = %q", out.Metadata.UID)
			}
			if privateGets != 1 || privatePuts != 0 || publicPuts != 0 || operationGets != 1 {
				t.Fatalf("GET/private PUT/public PUT/operation GET = %d/%d/%d/%d, want 1/0/0/1", privateGets, privatePuts, publicPuts, operationGets)
			}
		})
	}
}

func TestRuntimeInputSpecDriftCommitmentMismatchStopsBeforePublicMutation(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	driftedSpec := map[string]any{"image": "drifted", "requiredSensitiveVars": []any{"API_TOKEN"}}
	var privateGets, privatePuts, publicPuts int
	var prepared bool
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodGet {
				privateGets++
				if !prepared {
					writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
					return true
				}
			} else {
				privatePuts++
				prepared = true
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, driftedSpec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err == nil {
		t.Fatal("runtime preparation with a drifted public apply commitment was accepted")
	}
	var runtimeErr *RuntimeInputApplyError
	if !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorApplyCommitmentMismatch {
		t.Fatalf("spec drift error = %v, want closed apply commitment mismatch", err)
	}
	if privateGets != 2 || privatePuts != 1 || publicPuts != 0 {
		t.Fatalf("private GET/PUT and public PUT = %d/%d/%d, want 2/1/0", privateGets, privatePuts, publicPuts)
	}
}

func TestRuntimeInputExistingPreparationCommitmentMismatchStopsBeforePollOrMutation(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	driftedSpec := map[string]any{"image": "another-spec", "requiredSensitiveVars": []any{"API_TOKEN"}}
	const hostOperationID = "op_mismatched_runtime"
	var privateGets, privatePuts, publicPuts, operationGets int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privateGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format": runtimeInputPreparationFormat, "status": "accepted",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, driftedSpec),
				"canonicalPublicOrigin": client.Endpoint(), "bindingNames": []string{"API_TOKEN"},
				"hostOperationId": hostOperationID,
			})
			return true
		case r.Method == http.MethodPut && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privatePuts++
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+hostOperationID:
			operationGets++
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	var runtimeErr *RuntimeInputApplyError
	if !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorApplyCommitmentMismatch {
		t.Fatalf("existing preparation mismatch = %v, want closed commitment mismatch", err)
	}
	if privateGets != 1 || privatePuts != 0 || publicPuts != 0 || operationGets != 0 {
		t.Fatalf("mismatch GET/private PUT/public PUT/operation GET = %d/%d/%d/%d, want 1/0/0/0", privateGets, privatePuts, publicPuts, operationGets)
	}
}

func TestApplyResourceWithRuntimeInputsBindsExactFinalPublicEnvelope(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	var privatePublicBody string
	var privatePublicPath string
	var publicBody string
	var publicPath string
	var privateCalls, publicCalls int

	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
			return true
		case r.Method == http.MethodPut && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privateCalls++
			if got := r.Header.Get("Idempotency-Key"); got != testRuntimeOperationKey {
				t.Errorf("private Idempotency-Key = %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("private bearer = %q", got)
			}
			raw, _ := io.ReadAll(r.Body)
			var body map[string]json.RawMessage
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("private request JSON: %v", err)
			}
			if got := sortedRawKeys(body); !reflect.DeepEqual(got, []string{"bindings", "canonicalPublicOrigin", "format", "publicApply"}) {
				t.Fatalf("private request keys = %v", got)
			}
			for _, forbidden := range []string{
				`"workerUid"`, `"workerVersionName"`, `"endpointName"`, `"reservationRef"`,
				`"callerSecretDigest"`, `"callerOrganization"`, `"organization"`,
			} {
				if strings.Contains(string(raw), forbidden) {
					t.Fatalf("private request contains forbidden identity %s: %s", forbidden, raw)
				}
			}
			var format, origin string
			var bindings map[string]string
			var publicApply map[string]json.RawMessage
			var apply struct {
				Method string            `json:"method"`
				Path   string            `json:"path"`
				Fences map[string]string `json:"fences"`
				Body   string            `json:"body"`
			}
			_ = json.Unmarshal(body["format"], &format)
			_ = json.Unmarshal(body["canonicalPublicOrigin"], &origin)
			_ = json.Unmarshal(body["bindings"], &bindings)
			_ = json.Unmarshal(body["publicApply"], &apply)
			_ = json.Unmarshal(body["publicApply"], &publicApply)
			if got := sortedRawKeys(publicApply); !reflect.DeepEqual(got, []string{"body", "fences", "method", "path"}) {
				t.Fatalf("private publicApply keys = %v", got)
			}
			if format != runtimeInputPreparationFormat || origin != client.Endpoint() {
				t.Fatalf("private identity format=%q origin=%q", format, origin)
			}
			if apply.Method != http.MethodPut || !reflect.DeepEqual(apply.Fences, map[string]string{"ifNoneMatch": "*"}) {
				t.Fatalf("public apply envelope method=%q fences=%v", apply.Method, apply.Fences)
			}
			if !reflect.DeepEqual(bindings, map[string]string{"API_TOKEN": testRuntimeSecret}) {
				t.Fatal("private request did not carry the exact binding")
			}
			privatePublicBody = apply.Body
			privatePublicPath = apply.Path
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicCalls++
			raw, _ := io.ReadAll(r.Body)
			publicBody = string(raw)
			publicPath = r.URL.RequestURI()
			if strings.Contains(publicBody, testRuntimeSecret) || strings.Contains(publicBody, "bindings") {
				t.Fatalf("runtime values leaked into public apply: %s", publicBody)
			}
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(),
		testResourceRequest(spec),
		Fence{},
		testRuntimeOperationKey,
		client.Endpoint(),
		map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err != nil {
		t.Fatalf("apply with runtime inputs: %v", err)
	}
	if privateCalls != 1 || publicCalls != 1 {
		t.Fatalf("private/public calls = %d/%d", privateCalls, publicCalls)
	}
	if privatePublicBody != publicBody || privatePublicPath != publicPath {
		t.Fatalf("private envelope did not bind exact public request: body equal=%t path %q/%q", privatePublicBody == publicBody, privatePublicPath, publicPath)
	}
}

func TestRuntimeInputPrivateEncodingUsesWipeableValueBuffers(t *testing.T) {
	value := []byte("line\nquote\"slash\\snow-雪")
	originalBacking := value
	material := &runtimeInputMaterial{
		CanonicalPublicOrigin: "https://host.example",
		Bindings:              map[string][]byte{"API_TOKEN": value},
	}
	raw, err := encodeRuntimeInputPrepareRequest(material, "/public/path", []byte(`{"public":"body"}`))
	if err != nil {
		t.Fatalf("encode private preparation: %v", err)
	}
	if len(raw) != cap(raw) {
		t.Fatalf("private encoder grew beyond exact preallocation: len/cap=%d/%d", len(raw), cap(raw))
	}
	var decoded struct {
		Bindings map[string]string `json:"bindings"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode custom private JSON: %v", err)
	}
	if decoded.Bindings["API_TOKEN"] != string(value) {
		t.Fatal("custom private encoder changed a UTF-8 binding value")
	}
	clearRuntimeInputMaterial(material)
	if len(material.Bindings) != 0 {
		t.Fatal("private material map was not consumed")
	}
	for index, current := range originalBacking {
		if current != 0 {
			t.Fatalf("mutable binding backing byte %d was not wiped", index)
		}
	}
	clearRuntimeInputBytes(raw)
	for index, current := range raw {
		if current != 0 {
			t.Fatalf("mutable private request byte %d was not wiped", index)
		}
	}
}

type runtimeInputLossTransport struct {
	base      http.RoundTripper
	method    string
	path      string
	remaining int
	cancel    context.CancelFunc
	lost      bool
	freshGet  bool
	freshGets map[string]bool
}

type runtimeInputPartialReadTransport struct {
	base      http.RoundTripper
	method    string
	path      string
	remaining int
	prefix    []byte
	err       error
}

func (transport *runtimeInputPartialReadTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if transport.remaining > 0 && request.Method == transport.method && request.URL.Path == transport.path {
		transport.remaining--
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		response.Body = &runtimeInputPartialErrorBody{
			remaining: append([]byte(nil), transport.prefix...),
			err:       transport.err,
		}
		response.ContentLength = -1
		response.Header.Del("Content-Length")
	}
	return response, nil
}

type runtimeInputPartialErrorBody struct {
	remaining []byte
	err       error
}

func (body *runtimeInputPartialErrorBody) Read(target []byte) (int, error) {
	if len(body.remaining) > 0 {
		count := copy(target, body.remaining)
		body.remaining = body.remaining[count:]
		return count, nil
	}
	return 0, body.err
}

func (body *runtimeInputPartialErrorBody) Close() error {
	clearRuntimeInputBytes(body.remaining)
	body.remaining = nil
	return nil
}

func assertRuntimeInputErrorSanitized(t *testing.T, err error, sentinels ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("runtime-input failure was unexpectedly nil")
	}
	for current, depth := err, 0; current != nil && depth < 16; current, depth = errors.Unwrap(current), depth+1 {
		for _, sentinel := range sentinels {
			if strings.Contains(current.Error(), sentinel) {
				t.Fatalf("runtime-input error exposed %q: %v", sentinel, err)
			}
		}
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("runtime-input error retained Host API response data: %#v", apiErr)
	}
}

func (transport *runtimeInputLossTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.lost && request.Method == http.MethodGet && request.URL.Path == transport.path {
		_, bounded := request.Context().Deadline()
		transport.freshGet = request.Context().Err() == nil && bounded
	}
	if transport.lost && request.Method == http.MethodGet {
		_, bounded := request.Context().Deadline()
		if request.Context().Err() == nil && bounded {
			if transport.freshGets == nil {
				transport.freshGets = map[string]bool{}
			}
			transport.freshGets[request.URL.Path] = true
		}
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if transport.remaining > 0 && request.Method == transport.method && request.URL.Path == transport.path {
		transport.remaining--
		transport.lost = true
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if transport.cancel != nil {
			transport.cancel()
		}
		return nil, errors.New("simulated lost acknowledgement")
	}
	return response, nil
}

func TestPrivatePreparationDeadlineAcknowledgementLossUsesFreshBoundedReadback(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	bindings := map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)}
	var prepared bool
	var privateGets, privatePuts, publicPuts int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodGet {
				privateGets++
				if !prepared {
					writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
					return true
				}
			} else {
				privatePuts++
				prepared = true
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			return true
		}
		return false
	})
	ctx, cancel := context.WithCancel(context.Background())
	transport := &runtimeInputLossTransport{
		base: client.httpClient.Transport, method: http.MethodPut,
		path: runtimeInputPreparationPath(testRuntimeOperationKey), remaining: 1, cancel: cancel,
	}
	client.httpClient.Transport = transport

	_, err := client.ApplyResourceWithRuntimeInputs(
		ctx, testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), bindings,
	)
	var indeterminate *RuntimeInputApplyIndeterminateError
	if !errors.As(err, &indeterminate) {
		t.Fatalf("deadline acknowledgement loss error = %v, want indeterminate", err)
	}
	if privateGets != 2 || privatePuts != 1 || publicPuts != 0 {
		t.Fatalf("private GET/PUT and public PUT = %d/%d/%d, want 2/1/0", privateGets, privatePuts, publicPuts)
	}
	if !transport.freshGet {
		t.Fatal("private acknowledgement recovery did not use a fresh bounded context")
	}
	if len(bindings) != 0 {
		t.Fatal("runtime bindings remain reachable after private acknowledgement loss")
	}
}

func TestPublicApplyDeadlineAcknowledgementLossUsesFreshBoundedReadbackAndPoll(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	const hostOperationID = "op_deadline_runtime"
	var publicAccepted bool
	var privateGets, privatePuts, publicPuts, operationGets int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodPut {
				privatePuts++
			} else {
				privateGets++
			}
			response := map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			}
			if publicAccepted {
				response["status"] = "accepted"
				response["hostOperationId"] = hostOperationID
			}
			writeJSON(t, w, http.StatusOK, response)
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			publicAccepted = true
			writeJSON(t, w, http.StatusAccepted, map[string]any{
				"operation": map[string]any{
					"apiVersion": OperationAPIVersion,
					"kind":       OperationKind,
					"id":         hostOperationID,
					"done":       false,
				},
			})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+hostOperationID:
			operationGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion,
				"kind":       OperationKind,
				"id":         hostOperationID,
				"done":       true,
				"result": map[string]any{
					"resource": wireResource("app", "uid-1", "1", "7", spec),
				},
			})
			return true
		}
		return false
	})
	ctx, cancel := context.WithCancel(context.Background())
	transport := &runtimeInputLossTransport{
		base: client.httpClient.Transport, method: http.MethodPut,
		path: splitGroupResourcePath("app", ""), remaining: 1, cancel: cancel,
	}
	client.httpClient.Transport = transport

	out, err := client.ApplyResourceWithRuntimeInputs(
		ctx, testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err != nil {
		t.Fatalf("recover deadline-lost public acknowledgement: %v", err)
	}
	if out.Metadata.UID != "uid-1" {
		t.Fatalf("recovered resource uid = %q", out.Metadata.UID)
	}
	if privateGets != 2 || privatePuts != 0 || publicPuts != 1 || operationGets != 1 {
		t.Fatalf("private GET/PUT, public PUT, operation GET = %d/%d/%d/%d, want 2/0/1/1", privateGets, privatePuts, publicPuts, operationGets)
	}
	if !transport.freshGets[runtimeInputPreparationPath(testRuntimeOperationKey)] ||
		!transport.freshGets[APIRootPath+"/operations/"+hostOperationID] {
		t.Fatalf("public acknowledgement recovery requests did not use fresh bounded contexts: %v", transport.freshGets)
	}
}

func TestRuntimeInputPreparedByExitedProcessIsResumedWithoutValueReplay(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	var prepared bool
	var privateGets, privatePuts, publicPuts int
	var privateBodies [][]byte
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == DiscoveryPath:
			writeJSON(t, w, http.StatusOK, discoveryDoc(server.URL))
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodGet {
				privateGets++
				if !prepared {
					writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
					return
				}
			} else {
				privatePuts++
				prepared = true
				raw, _ := io.ReadAll(r.Body)
				privateBodies = append(privateBodies, raw)
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": server.URL,
				"bindingNames":          []string{"API_TOKEN"},
			})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	newClient := func() *Client {
		client := New(server.URL, "test-token", server.Client())
		if _, err := client.Discover(context.Background()); err != nil {
			t.Fatalf("discover process client: %v", err)
		}
		return client
	}
	first := newClient()
	firstContext, cancelFirst := context.WithCancel(context.Background())
	first.httpClient.Transport = &runtimeInputLossTransport{
		base: first.httpClient.Transport, method: http.MethodPut,
		path: runtimeInputPreparationPath(testRuntimeOperationKey), remaining: 1, cancel: cancelFirst,
	}
	firstBindings := map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)}
	_, firstErr := first.ApplyResourceWithRuntimeInputs(
		firstContext, testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		first.Endpoint(), firstBindings,
	)
	var indeterminate *RuntimeInputApplyIndeterminateError
	if !errors.As(firstErr, &indeterminate) {
		t.Fatalf("exiting process error = %v, want indeterminate", firstErr)
	}

	second := newClient()
	const secondValue = "second-process-value-must-not-be-sent"
	secondBindings := map[string][]byte{"API_TOKEN": []byte(secondValue)}
	out, err := second.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		second.Endpoint(), secondBindings,
	)
	if err != nil {
		t.Fatalf("resume preparation in second process: %v", err)
	}
	if out.Metadata.UID != "uid-1" {
		t.Fatalf("second process resource uid = %q", out.Metadata.UID)
	}
	if privateGets != 3 || privatePuts != 1 || publicPuts != 1 {
		t.Fatalf("two-process private GET/PUT and public PUT = %d/%d/%d, want 3/1/1", privateGets, privatePuts, publicPuts)
	}
	if len(privateBodies) != 1 || strings.Contains(string(privateBodies[0]), secondValue) {
		t.Fatalf("second process resent values through private preparation: bodies=%d", len(privateBodies))
	}
	if len(firstBindings) != 0 || len(secondBindings) != 0 {
		t.Fatalf("runtime bindings survived process calls: first=%d second=%d", len(firstBindings), len(secondBindings))
	}
}

func TestRuntimeInputPublicRejectionCannotReflectHostControlledData(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	sentinels := []string{
		testRuntimeSecret,
		"reflected-public-message",
		"reflected-public-details",
		"reflected-public-host-code",
		"reflected-public-request-id",
	}
	var privateGets, publicPuts int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privateGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			writeJSON(t, w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{
					"code":      "invalid_argument",
					"message":   "reflected-public-message " + testRuntimeSecret,
					"requestId": "reflected-public-request-id",
					"retryable": false,
					"hostCode":  "reflected-public-host-code",
					"details":   map[string]any{"value": "reflected-public-details"},
				},
			})
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err == nil {
		t.Fatal("public runtime-input rejection was accepted")
	}
	for _, sentinel := range sentinels {
		if strings.Contains(err.Error(), sentinel) {
			t.Fatalf("runtime-input public rejection exposed %q: %v", sentinel, err)
		}
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("runtime-input public rejection retained Host API error data: %#v", apiErr)
	}
	if privateGets != 2 || publicPuts != 1 {
		t.Fatalf("public rejection private GET/public PUT = %d/%d, want 2/1", privateGets, publicPuts)
	}
}

func TestRuntimeInputReadOnlyPrerequisiteErrorsCannotReflectHostData(t *testing.T) {
	for _, phase := range []string{"availability", "prepare"} {
		t.Run(phase, func(t *testing.T) {
			spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
			const reflected = "reflected-prerequisite-host-data"
			var privateMutations, publicMutations int
			client := newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
					if phase == "availability" {
						writeJSON(t, w, http.StatusInternalServerError, reflectedStableError(reflected))
					} else {
						writeJSON(t, w, http.StatusOK, wireAvailability("create"))
					}
					return true
				case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
					if phase == "prepare" {
						writeJSON(t, w, http.StatusInternalServerError, reflectedStableError(reflected))
					} else {
						handlePrepare(t, w, r)
					}
					return true
				case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
					privateMutations++
					return true
				case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
					publicMutations++
					return true
				}
				return false
			})

			_, err := client.ApplyResourceWithRuntimeInputs(
				context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
				client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
			)
			assertRuntimeInputErrorSanitized(t, err, testRuntimeSecret, reflected)
			var runtimeErr *RuntimeInputApplyError
			if !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorPrerequisiteUnavailable {
				t.Fatalf("%s error = %v, want closed prerequisite error", phase, err)
			}
			if privateMutations != 0 || publicMutations != 0 {
				t.Fatalf("%s failure reached mutation: private/public=%d/%d", phase, privateMutations, publicMutations)
			}
		})
	}
}

func reflectedStableError(reflected string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code":      "internal_error",
			"message":   reflected + "-message " + testRuntimeSecret,
			"requestId": reflected + "-request-id",
			"retryable": false,
			"hostCode":  reflected + "-host-code",
			"details":   map[string]any{"value": reflected + "-details"},
		},
	}
}

func TestRuntimeInputPrivateLookupErrorsAndPartialReadsAreClosed(t *testing.T) {
	for _, mode := range []string{"structured", "malformed", "partial-read"} {
		t.Run(mode, func(t *testing.T) {
			spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
			const reflected = "reflected-private-lookup-data"
			var privateGets, privatePuts, publicPuts int
			var client *Client
			client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
					writeJSON(t, w, http.StatusOK, wireAvailability("create"))
					return true
				case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
					handlePrepare(t, w, r)
					return true
				case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
					privateGets++
					switch mode {
					case "structured":
						writeJSON(t, w, http.StatusInternalServerError, reflectedStableError(reflected))
					case "malformed":
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"format":"` + reflected + ` ` + testRuntimeSecret + `"`))
					case "partial-read":
						writeJSON(t, w, http.StatusOK, map[string]any{
							"format": runtimeInputPreparationFormat,
						})
					}
					return true
				case r.Method == http.MethodPut && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
					privatePuts++
					return true
				case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
					publicPuts++
					return true
				}
				return false
			})
			if mode == "partial-read" {
				client.httpClient.Transport = &runtimeInputPartialReadTransport{
					base: client.httpClient.Transport, method: http.MethodGet,
					path: runtimeInputPreparationPath(testRuntimeOperationKey), remaining: 1,
					prefix: []byte(`{"format":"partial`), err: errors.New(reflected + " " + testRuntimeSecret),
				}
			}

			_, err := client.ApplyResourceWithRuntimeInputs(
				context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
				client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
			)
			assertRuntimeInputErrorSanitized(t, err, testRuntimeSecret, reflected)
			var runtimeErr *RuntimeInputApplyError
			if !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorPreparationLookupFailed {
				t.Fatalf("%s private lookup error = %v", mode, err)
			}
			if privateGets != 1 || privatePuts != 0 || publicPuts != 0 {
				t.Fatalf("%s private GET/PUT/public PUT = %d/%d/%d, want 1/0/0", mode, privateGets, privatePuts, publicPuts)
			}
		})
	}
}

func TestRuntimeInputPollFailureCannotReflectHostData(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	const hostOperationID = "op_reflected_runtime"
	const reflected = "reflected-operation-host-data"
	var privateGets, publicPuts, operationGets int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privateGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format": runtimeInputPreparationFormat, "status": "accepted",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(), "bindingNames": []string{"API_TOKEN"},
				"hostOperationId": hostOperationID,
			})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+hostOperationID:
			operationGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": hostOperationID, "done": true,
				"error": map[string]any{
					"code": "internal_error", "message": reflected + " " + testRuntimeSecret,
					"requestId": reflected + "-request-id", "retryable": false,
					"hostCode": reflected + "-host-code",
				},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	assertRuntimeInputErrorSanitized(t, err, testRuntimeSecret, reflected)
	var accepted *AcceptedError
	var runtimeErr *RuntimeInputApplyError
	if !errors.As(err, &accepted) || !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorOperationPollFailed {
		t.Fatalf("poll failure = %v, want accepted closed operation error", err)
	}
	if privateGets != 1 || publicPuts != 0 || operationGets != 1 {
		t.Fatalf("poll failure private GET/public PUT/operation GET = %d/%d/%d, want 1/0/1", privateGets, publicPuts, operationGets)
	}
}

func TestRuntimeInputPollPartialReadCannotReflectTransportData(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	const hostOperationID = "op_partial_runtime"
	const reflected = "reflected-operation-partial-read"
	var privateGets, operationGets int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privateGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format": runtimeInputPreparationFormat, "status": "accepted",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(), "bindingNames": []string{"API_TOKEN"},
				"hostOperationId": hostOperationID,
			})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+hostOperationID:
			operationGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": hostOperationID, "done": false,
			})
			return true
		}
		return false
	})
	client.httpClient.Transport = &runtimeInputPartialReadTransport{
		base: client.httpClient.Transport, method: http.MethodGet,
		path: APIRootPath + "/operations/" + hostOperationID, remaining: 1,
		prefix: []byte(`{"apiVersion":"partial`), err: errors.New(reflected + " " + testRuntimeSecret),
	}

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	assertRuntimeInputErrorSanitized(t, err, testRuntimeSecret, reflected)
	var accepted *AcceptedError
	var runtimeErr *RuntimeInputApplyError
	if !errors.As(err, &accepted) || !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorOperationPollFailed {
		t.Fatalf("partial poll failure = %v, want accepted closed operation error", err)
	}
	if privateGets != 1 || operationGets != 1 {
		t.Fatalf("partial poll private/operation GET = %d/%d, want 1/1", privateGets, operationGets)
	}
}

func TestRuntimeInputPublicDecodeAndPartialReadFailuresCannotReflectDataOrReplay(t *testing.T) {
	for _, mode := range []string{"malformed", "partial-read"} {
		t.Run(mode, func(t *testing.T) {
			spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
			const reflected = "reflected-public-decode-data"
			var privateGets, publicPuts int
			var client *Client
			client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
					writeJSON(t, w, http.StatusOK, wireAvailability("create"))
					return true
				case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
					handlePrepare(t, w, r)
					return true
				case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
					privateGets++
					writeJSON(t, w, http.StatusOK, map[string]any{
						"format": runtimeInputPreparationFormat, "status": "prepared",
						"operationKey":          testRuntimeOperationKey,
						"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
						"canonicalPublicOrigin": client.Endpoint(), "bindingNames": []string{"API_TOKEN"},
					})
					return true
				case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
					publicPuts++
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"reflected":"` + reflected + ` ` + testRuntimeSecret + `"`))
					return true
				}
				return false
			})
			if mode == "partial-read" {
				client.httpClient.Transport = &runtimeInputPartialReadTransport{
					base: client.httpClient.Transport, method: http.MethodPut,
					path: splitGroupResourcePath("app", ""), remaining: 1,
					prefix: []byte(`{"reflected":"partial`), err: errors.New(reflected + " " + testRuntimeSecret),
				}
			}

			_, err := client.ApplyResourceWithRuntimeInputs(
				context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
				client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
			)
			assertRuntimeInputErrorSanitized(t, err, testRuntimeSecret, reflected)
			var indeterminate *RuntimeInputApplyIndeterminateError
			if !errors.As(err, &indeterminate) {
				t.Fatalf("%s public response error = %v, want indeterminate", mode, err)
			}
			if privateGets != 2 || publicPuts != 1 {
				t.Fatalf("%s private GET/public PUT = %d/%d, want 2/1", mode, privateGets, publicPuts)
			}
		})
	}
}

func TestRuntimeInputPreparationResponseLossUsesOnePutThenGetWithoutValues(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	bindings := map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)}
	var privatePuts, privateGets, publicPuts int
	var prepared bool
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodPut {
				privatePuts++
				prepared = true
				raw, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(raw), testRuntimeSecret) {
					t.Error("private PUT omitted runtime value")
				}
			} else if r.Method == http.MethodGet {
				privateGets++
				if !prepared {
					writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
					return true
				}
				if len(bindings) != 0 {
					t.Fatalf("runtime values remain reachable during private recovery GET: %v", sortedRuntimeInputBindingNames(bindings))
				}
				raw, _ := io.ReadAll(r.Body)
				if len(raw) != 0 || strings.Contains(string(raw), testRuntimeSecret) {
					t.Fatalf("private recovery GET resent runtime values: %q", raw)
				}
			} else {
				return false
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})
	client.httpClient.Transport = &runtimeInputLossTransport{
		base: client.httpClient.Transport, method: http.MethodPut,
		path: runtimeInputPreparationPath(testRuntimeOperationKey), remaining: 1,
	}

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), bindings,
	)
	if err != nil {
		t.Fatalf("apply after private acknowledgement loss: %v", err)
	}
	if privatePuts != 1 || privateGets != 2 || publicPuts != 1 {
		t.Fatalf("private PUT/GET and public PUT = %d/%d/%d, want 1/2/1", privatePuts, privateGets, publicPuts)
	}
}

func TestPublicApplyAcknowledgementLossRecoversByPrivateReadAndOrdinaryOperation(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	const hostOperationID = "op_runtime_1"
	var publicPuts, privateGets, operationGets int
	publicAccepted := false
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			status := "prepared"
			response := map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                status,
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			}
			if r.Method == http.MethodGet {
				privateGets++
				if publicAccepted {
					response["status"] = "accepted"
					response["hostOperationId"] = hostOperationID
				}
			}
			writeJSON(t, w, http.StatusOK, response)
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			publicAccepted = true
			writeJSON(t, w, http.StatusAccepted, map[string]any{
				"operation": map[string]any{
					"apiVersion": OperationAPIVersion,
					"kind":       OperationKind,
					"id":         hostOperationID,
					"done":       false,
				},
			})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+hostOperationID:
			operationGets++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion,
				"kind":       OperationKind,
				"id":         hostOperationID,
				"done":       true,
				"result": map[string]any{
					"resource": wireResource("app", "uid-1", "1", "7", spec),
				},
			})
			return true
		}
		return false
	})
	client.httpClient.Transport = &runtimeInputLossTransport{
		base: client.httpClient.Transport, method: http.MethodPut,
		path: splitGroupResourcePath("app", ""), remaining: 1,
	}

	out, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err != nil {
		t.Fatalf("recover public acknowledgement loss: %v", err)
	}
	if out.Metadata.UID != "uid-1" {
		t.Fatalf("recovered resource uid = %q", out.Metadata.UID)
	}
	if publicPuts != 1 || privateGets != 2 || operationGets != 1 {
		t.Fatalf("public PUT/private GET/operation GET = %d/%d/%d, want 1/2/1", publicPuts, privateGets, operationGets)
	}
}

func TestPublicApplyAcknowledgementLossPreparedOnlyIsIndeterminateWithoutReplay(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	var publicPuts, privateGets int
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodGet {
				privateGets++
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})
	client.httpClient.Transport = &runtimeInputLossTransport{
		base: client.httpClient.Transport, method: http.MethodPut,
		path: splitGroupResourcePath("app", ""), remaining: 1,
	}

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	var indeterminate *RuntimeInputApplyIndeterminateError
	if !errors.As(err, &indeterminate) {
		t.Fatalf("prepared-only recovery error = %v", err)
	}
	if publicPuts != 1 || privateGets != 2 {
		t.Fatalf("public PUT/private GET = %d/%d, want 1/2", publicPuts, privateGets)
	}
}

func TestRuntimeInputValuesAreClearedBeforePublicApply(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	valueBuffer := []byte(testRuntimeSecret)
	bindings := map[string][]byte{"API_TOKEN": valueBuffer}
	var client *Client
	client = newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodGet && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
			return true
		case r.Method == http.MethodPut && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       testRuntimePublicApplyCommitment(t, spec),
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			if len(bindings) != 0 {
				t.Fatalf("runtime values remain reachable when public apply starts: %v", sortedRuntimeInputBindingNames(bindings))
			}
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), bindings,
	)
	if err != nil {
		t.Fatalf("apply with short-lived runtime values: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("runtime values remain reachable after private use: %v", sortedRuntimeInputBindingNames(bindings))
	}
	for index, current := range valueBuffer {
		if current != 0 {
			t.Fatalf("runtime value backing byte %d was not wiped after private use", index)
		}
	}
}

func TestRuntimeInputPreparationRejectionCannotReflectValues(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	bindings := map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)}
	var privatePuts, privateGets, publicPuts int
	client := newRuntimeInputTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			if r.Method == http.MethodGet {
				privateGets++
				writeStableError(t, w, http.StatusNotFound, "operation_not_found", false)
				return true
			}
			privatePuts++
			writeJSON(t, w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{
					"code":      "invalid_argument",
					"message":   "reflected-private-message " + testRuntimeSecret,
					"requestId": "reflected-private-request-id",
					"retryable": false,
					"hostCode":  "reflected-private-host-code",
					"details":   map[string]any{"reflected": "reflected-private-details"},
				},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), bindings,
	)
	if err == nil {
		t.Fatal("private runtime-input rejection was accepted")
	}
	assertRuntimeInputErrorSanitized(
		t, err, testRuntimeSecret, "reflected-private-message", "reflected-private-request-id",
		"reflected-private-host-code", "reflected-private-details",
	)
	var runtimeErr *RuntimeInputApplyError
	if !errors.As(err, &runtimeErr) || runtimeErr.Code != runtimeInputErrorPreparationRejected {
		t.Fatalf("private rejection error = %v, want closed preparation rejection", err)
	}
	if privatePuts != 1 || privateGets != 2 || publicPuts != 0 {
		t.Fatalf("rejected private PUT/GET and public PUT = %d/%d/%d, want 1/2/0", privatePuts, privateGets, publicPuts)
	}
	if len(bindings) != 0 {
		t.Fatalf("runtime values remain reachable after private rejection: %v", sortedRuntimeInputBindingNames(bindings))
	}
}

func TestRuntimeInputsRejectPlaintextOriginBeforeMutation(t *testing.T) {
	spec := map[string]any{"image": "example", "requiredSensitiveVars": []any{"API_TOKEN"}}
	var privatePuts, publicPuts int
	var client *Client
	client = newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut && r.URL.Path == runtimeInputPreparationPath(testRuntimeOperationKey):
			privatePuts++
			writeJSON(t, w, http.StatusOK, map[string]any{
				"format":                runtimeInputPreparationFormat,
				"status":                "prepared",
				"operationKey":          testRuntimeOperationKey,
				"applyCommitment":       "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				"canonicalPublicOrigin": client.Endpoint(),
				"bindingNames":          []string{"API_TOKEN"},
			})
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			publicPuts++
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})

	_, err := client.ApplyResourceWithRuntimeInputs(
		context.Background(), testResourceRequest(spec), Fence{}, testRuntimeOperationKey,
		client.Endpoint(), map[string][]byte{"API_TOKEN": []byte(testRuntimeSecret)},
	)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("plaintext runtime-input origin error = %v", err)
	}
	if privatePuts != 0 || publicPuts != 0 {
		t.Fatalf("plaintext runtime-input origin reached mutation: private/public=%d/%d", privatePuts, publicPuts)
	}
}

func sortedRawKeys(value map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
