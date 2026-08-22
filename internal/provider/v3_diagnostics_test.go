package provider

// v3_diagnostics_test.go holds the lane's diagnostics to the contract stated in
// v3_diagnostics.go: every error carries the identity, the fences, the host's
// own answer, a stable code, whether waiting helps, and one concrete repair.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// TestV3DiagnosticRendersEveryRecordedFact pins the rendered shape. The example
// is the relation-drift report the decision record spells out, so the test is a
// direct reading of it.
func TestV3DiagnosticRendersEveryRecordedFact(t *testing.T) {
	t.Parallel()
	rendered := v3Diagnostic{
		Summary:      "Relation target changed incarnation.",
		ResourceType: "takoform_worker_version",
		Space:        "prod",
		Name:         "version-6c37aa755eee",
		Ref: currentformregistry.V3Ref{
			APIVersion: "edge.forms.takoform.com/v1beta1", Kind: "WorkerVersion",
			DefinitionVersion: "0.1.0", SchemaDigest: "sha256:abc",
		},
		Pointer:            "/kvBindings/0/resource",
		ExpectedUID:        "uid-old",
		CurrentUID:         "uid-new",
		ExpectedGeneration: "3",
		CurrentGeneration:  "4",
		ExpectedRevision:   "7",
		CurrentRevision:    "9",
		OperationID:        "op-1",
		Host: &v3HostFault{
			Code: "uid_mismatch", StatusCode: http.StatusConflict, RequestID: "req-1", HostCode: "backend-42",
		},
		Repair: "Re-apply WorkerVersion to bind the new target, restore the old target, " +
			"or create a new WorkerVersion if the snapshot must remain immutable.",
	}.error()

	if rendered.Summary() != "Relation target changed incarnation." {
		t.Fatalf("summary = %q", rendered.Summary())
	}
	detail := rendered.Detail()
	for _, want := range []string{
		"Resource: takoform_worker_version (prod/version-6c37aa755eee)",
		"Form: edge.forms.takoform.com/v1beta1 WorkerVersion@0.1.0 schema=sha256:abc",
		"Pointer: /kvBindings/0/resource",
		"Expected UID: uid-old",
		"Current UID: uid-new",
		"Expected generation: 3",
		"Current generation: 4",
		"Expected revision: 7",
		"Current revision: 9",
		"Operation: op-1",
		"Request: req-1",
		"Host reason: backend-42",
		"Code: uid_mismatch (host, HTTP 409)",
		"Retryable: no",
		"Re-apply WorkerVersion to bind the new target",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not carry %q:\n%s", want, detail)
		}
	}
	// A fact the diagnostic does not have is omitted rather than rendered empty.
	sparse := v3Diagnostic{Summary: "s", Code: v3CodeProviderBug, Repair: "r"}.error().Detail()
	for _, absent := range []string{"Expected UID", "Operation", "Request", "Form:"} {
		if strings.Contains(sparse, absent) {
			t.Errorf("a diagnostic with no %s rendered one anyway:\n%s", absent, sparse)
		}
	}
	if !strings.Contains(sparse, "Code: "+v3CodeProviderBug) || !strings.Contains(sparse, "Retryable: no") {
		t.Errorf("a sparse diagnostic dropped the code or the retryable flag:\n%s", sparse)
	}
}

// TestV3DiagnosticDestructuresTheHostAnswer proves a failed host call renders
// the host's own code, request id, and retryable flag rather than a wrapped
// error string, and picks the repair keyed to that code.
func TestV3DiagnosticDestructuresTheHostAnswer(t *testing.T) {
	t.Parallel()
	err := &clientv3.APIError{
		StatusCode: http.StatusTooManyRequests, Code: "rate_limited",
		Message: "slow down", RequestID: "req-7", Retryable: true,
	}
	rendered := v3HostCallDiagnostic("Failed to create WorkerBundle", err, v3Diagnostic{
		ResourceType: "takoform_worker_bundle", Space: "prod", Name: "bundle-abc", Pointer: "/spec",
	})
	detail := rendered.Detail()
	for _, want := range []string{
		"Host: slow down",
		"Resource: takoform_worker_bundle (prod/bundle-abc)",
		"Pointer: /spec",
		"Request: req-7",
		"Code: rate_limited (host, HTTP 429)",
		"Retryable: yes",
		"re-run the same apply",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not carry %q:\n%s", want, detail)
		}
	}

	accepted := &clientv3.AcceptedError{OperationID: "op-9", UID: "uid-9", Err: errors.New("deadline")}
	acceptedDetail := v3HostCallDiagnostic("Failed to create WorkerBundle", accepted, v3Diagnostic{
		ResourceType: "takoform_worker_bundle",
	}).Detail()
	for _, want := range []string{"Operation: op-9", "accepted-without-representation (host)"} {
		if !strings.Contains(acceptedDetail, want) {
			t.Errorf("an accepted mutation lost %q:\n%s", want, acceptedDetail)
		}
	}

	// A response outside the closed taxonomy is labelled, never presented as a
	// portable code.
	invalid := &clientv3.APIError{StatusCode: http.StatusBadGateway, ProtocolInvalid: true, Message: "<html>"}
	invalidDetail := v3HostCallDiagnostic("Failed to read WorkerBundle", invalid, v3Diagnostic{
		ResourceType: "takoform_worker_bundle",
	}).Detail()
	if !strings.Contains(invalidDetail, "protocol-invalid host response (HTTP 502)") {
		t.Errorf("a protocol-invalid response was not labelled:\n%s", invalidDetail)
	}
	if !strings.Contains(invalidDetail, "will not guess a remedy") {
		t.Errorf("a protocol-invalid response offered a guessed remedy:\n%s", invalidDetail)
	}
}

// TestV3HostRepairsCoverTheClosedTaxonomy proves the repair table is complete
// against the PUBLISHED error enum rather than against whatever the provider
// happens to have seen. A code with no repair renders a diagnostic that names a
// fault and no next action, which is the shape this work exists to remove.
func TestV3HostRepairsCoverTheClosedTaxonomy(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "host-api", "operations-v1beta1.json"))
	if err != nil {
		t.Fatalf("read the published operations document: %v", err)
	}
	var document struct {
		ErrorEnvelope struct {
			Codes []string `json:"codes"`
		} `json:"errorEnvelope"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode the published operations document: %v", err)
	}
	if len(document.ErrorEnvelope.Codes) == 0 {
		t.Fatal("the published operations document declares no error codes")
	}
	published := map[string]bool{}
	for _, code := range document.ErrorEnvelope.Codes {
		published[code] = true
		if _, covered := v3HostRepairs[code]; !covered {
			t.Errorf("no repair is stated for the published host error code %q", code)
		}
	}
	for code := range v3HostRepairs {
		if !published[code] {
			t.Errorf("a repair is stated for %q, which is not a published host error code", code)
		}
	}
}

// TestV3LaneDiagnosticsCarryTheRecordedError proves the resources report the
// recorded negotiation error rather than a generic not-configured shrug.
func TestV3LaneDiagnosticsCarryTheRecordedError(t *testing.T) {
	t.Parallel()
	resource := v3TestFormResource(t, "WorkerBundle", &providerData{
		v3Err: errors.New("discovering Takoform v1beta1 endpoint: 404"),
	})
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	response := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("a v2-only host produced no diagnostic")
	}
	detail := response.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{
		"takoform_worker_bundle",
		"discovering Takoform v1beta1 endpoint: 404",
		"Code: " + v3CodeLaneUnavailable,
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not carry %q:\n%s", want, detail)
		}
	}
}

// TestV3ParseRelationHostReasonLiftsThePointerAndUIDs covers the promotion of
// the host's free-form relation reason into the identity block, in both
// directions and in the tolerant fallback.
func TestV3ParseRelationHostReasonLiftsThePointerAndUIDs(t *testing.T) {
	t.Parallel()
	pointer, expected, current := v3ParseRelationHostReason(
		"relation /kvBindings/0/resource target edge.forms.takoform.com/v1beta1 EdgeKVNamespace cache " +
			"changed incarnation from uid uid-old (formRef a) to uid uid-new (formRef b); re-apply this resource")
	if pointer != "/kvBindings/0/resource" || expected != "uid-old" || current != "uid-new" {
		t.Fatalf("parsed %q %q %q", pointer, expected, current)
	}
	pointer, expected, current = v3ParseRelationHostReason(
		"relation /worker target edge.forms.takoform.com/v1beta1 ModuleWorker counter uid uid-3 no longer exists")
	if pointer != "/worker" || expected != "uid-3" || current != "" {
		t.Fatalf("parsed %q %q %q", pointer, expected, current)
	}
	if pointer, _, _ := v3ParseRelationHostReason("some host wrote prose"); pointer != "" {
		t.Fatalf("an unrecognised hostReason produced pointer %q", pointer)
	}
}
