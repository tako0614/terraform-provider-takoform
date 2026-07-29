package portableconformance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

func TestBlackBoxRunnerIndependentlyProbesEveryPlanBindingDimension(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		key       string
		wantError string
	}{
		{name: "desired spec", key: "portable-plan-substitution", wantError: "plan/spec binding"},
		{name: "Resource apiVersion", key: "portable-plan-substitution-resource-identity-api-version", wantError: "Resource identity binding"},
		{name: "Resource kind", key: "portable-plan-substitution-resource-identity-kind", wantError: "Resource identity binding"},
		{name: "Resource name", key: "portable-plan-substitution-resource-identity-name", wantError: "Resource identity binding"},
		{name: "Resource version", key: "portable-plan-substitution-resource-identity-resource-version", wantError: "Resource identity binding"},
		{name: "Space", key: "portable-plan-substitution-space", wantError: "Space binding"},
		{name: "FormRef apiVersion", key: "portable-plan-substitution-form-ref-api-version", wantError: "exact FormRef binding"},
		{name: "FormRef kind", key: "portable-plan-substitution-form-ref-kind", wantError: "exact FormRef binding"},
		{name: "FormRef definitionVersion", key: "portable-plan-substitution-form-ref-definition-version", wantError: "exact FormRef binding"},
		{name: "FormRef schemaDigest", key: "portable-plan-substitution-form-ref-schema-digest", wantError: "exact FormRef binding"},
		{name: "package digest", key: "portable-plan-substitution-package-digest", wantError: "package digest binding"},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := newReferenceHost(contract)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Idempotency-Key") == test.key {
					writeReferenceJSON(w, http.StatusCreated, `"1"`, map[string]any{"accepted": true})
					return
				}
				host.ServeHTTP(w, request)
			}))
			defer server.Close()

			_, err := RunEndpoint(context.Background(), contract, EndpointOptions{
				Endpoint:       server.URL,
				HTTPClient:     server.Client(),
				Classification: ReferenceHostSelfTest,
				Subject:        "reference-host-accepting-" + test.name,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("runner error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestRunnerRejectsOrdinaryValidationAsInstrumentedPlanEvidence(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get(PlanBindingProbeHeader) != "" {
			request.Header.Del(PlanBindingProbeHeader)
		}
		host.ServeHTTP(w, request)
	}))
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-without-plan-binding-instrumentation",
	})
	if err == nil || !strings.Contains(err.Error(), "did not prove the plan-binding seam") {
		t.Fatalf("runner error = %v, want missing plan-binding seam evidence rejection", err)
	}
}

func TestReferencePlanBindingInstrumentationIsClosedAuthenticatedAndNonMutating(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.enforcePlanBinding = false
	server := httptest.NewServer(host)
	defer server.Close()

	resource := client.Resource{
		APIVersion: contract.APIVersion,
		Kind:       contract.RunnerInput.Identity.FormRef.Kind,
		Form:       pointerToIdentity(toClientIdentity(contract.RunnerInput.Identity)),
		Metadata: client.Metadata{
			Name:  contract.RunnerInput.Name,
			Space: contract.RunnerInput.Space,
		},
		Spec: cloneMap(contract.RunnerInput.Desired),
	}
	body, err := json.Marshal(applyRequest{
		Resource: resource,
		Review:   client.DeploymentReview{PlanDigest: digestBytes([]byte("not-reviewed"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := server.URL + contract.APIPath + "/resources/" + resource.Kind + "/" + resource.Metadata.Name

	request, err := http.NewRequest(http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(PlanBindingProbeHeader, "unknown-field")
	request.Header.Set("Authorization", "Bearer "+referencePrimaryToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown plan probe returned HTTP %d, want 400", response.StatusCode)
	}
	_ = response.Body.Close()

	request, err = http.NewRequest(http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(PlanBindingProbeHeader, "resource.apiVersion")
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated plan probe returned HTTP %d, want 401", response.StatusCode)
	}
	_ = response.Body.Close()

	request, err = http.NewRequest(http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(PlanBindingProbeHeader, "resource.apiVersion")
	request.Header.Set("Authorization", "Bearer "+referencePrimaryToken)
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent ||
		response.Header.Get(PlanBindingProbeResultHeader) != PlanBindingProbeResultAcceptedNoMutation {
		t.Fatalf(
			"accepted plan probe = HTTP %d %s=%q, want 204 accepted-no-mutation",
			response.StatusCode,
			PlanBindingProbeResultHeader,
			response.Header.Get(PlanBindingProbeResultHeader),
		)
	}
	_ = response.Body.Close()
	if len(host.resources) != 0 || len(host.replays) != 0 {
		t.Fatalf("accepted instrumentation mutated state: resources=%d replays=%d", len(host.resources), len(host.replays))
	}
}

func pointerToIdentity(identity client.InstalledFormReference) *client.InstalledFormReference {
	return &identity
}

func TestBlackBoxRunnerRejectsReplaySharedAcrossPrincipals(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.scopeReplayToPrincipal = false
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		Token:          referencePrimaryToken,
		AlternateToken: referenceAlternateToken,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-with-shared-principal-replay-cache",
	})
	if err == nil || !strings.Contains(err.Error(), "cross-principal") {
		t.Fatalf("runner error = %v, want cross-principal replay isolation rejection", err)
	}
}

func TestBlackBoxRunnerRejectsReplaySharedAcrossTenants(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.scopeReplayToTenant = false
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:             server.URL,
		Token:                referencePrimaryToken,
		AlternateToken:       referenceAlternateToken,
		AlternateTenantToken: referenceAlternateTenantToken,
		HTTPClient:           server.Client(),
		Classification:       ReferenceHostSelfTest,
		Subject:              "reference-host-with-shared-tenant-replay-cache",
	})
	if err == nil || !strings.Contains(err.Error(), "cross-tenant") {
		t.Fatalf("runner error = %v, want cross-tenant replay isolation rejection", err)
	}
}

func TestBlackBoxRunnerRejectsReplayWithoutCurrentAuthorization(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		disable   func(*referenceHost)
		wantError string
	}{
		{
			name: "authentication",
			disable: func(host *referenceHost) {
				host.reauthorizeReplayAuthentication = false
			},
			wantError: "credential revocation before replay lookup",
		},
		{
			name: "permission",
			disable: func(host *referenceHost) {
				host.reauthorizeReplayPermission = false
			},
			wantError: "permission revocation before replay lookup",
		},
		{
			name: "policy",
			disable: func(host *referenceHost) {
				host.reauthorizeReplayPolicy = false
			},
			wantError: "policy revocation before replay lookup",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := newReferenceHost(contract)
			test.disable(host)
			server := httptest.NewServer(host)
			defer server.Close()

			_, err := RunEndpoint(context.Background(), contract, EndpointOptions{
				Endpoint:             server.URL,
				Token:                referencePrimaryToken,
				AlternateToken:       referenceAlternateToken,
				AlternateTenantToken: referenceAlternateTenantToken,
				HTTPClient:           server.Client(),
				Classification:       ReferenceHostSelfTest,
				Subject:              "reference-host-without-replay-" + test.name,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("runner error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestBlackBoxRunnerRejectsReplaySharedAcrossSpaces(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.scopeReplayToSpace = false
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-with-shared-space-replay-cache",
	})
	if err == nil || !strings.Contains(err.Error(), "Resource alternate-Space apply") {
		t.Fatalf("runner error = %v, want cross-Space replay isolation rejection", err)
	}
}

func TestBlackBoxRunnerRejectsResourceStorageSharedAcrossSpaces(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.scopeResourcesToSpace = false
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-with-shared-resource-space",
	})
	if err == nil || !strings.Contains(err.Error(), "Resource alternate-Space read isolation") {
		t.Fatalf("runner error = %v, want Resource Space isolation rejection", err)
	}
}

func TestBlackBoxRunnerRejectsConnectionResolutionOutsideSourceSpace(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.resolveConnectionsAcrossSpaces = true
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-resolving-connections-across-spaces",
	})
	if err == nil || !strings.Contains(err.Error(), "Connection source-Space isolation") {
		t.Fatalf("runner error = %v, want cross-Space Connection resolution rejection", err)
	}
}

func TestEndpointRunnerRequiresThreeDistinctAuthorityCredentials(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		alternate       string
		alternateTenant string
		want            string
	}{
		{
			name: "missing alternate credentials",
			want: "primary, same-tenant alternate-principal, and alternate-tenant credentials",
		},
		{
			name:      "missing alternate tenant",
			alternate: "alternate-token",
			want:      "primary, same-tenant alternate-principal, and alternate-tenant credentials",
		},
		{
			name:            "same credential",
			alternate:       "primary-token",
			alternateTenant: "tenant-token",
			want:            "three distinct bearer credentials",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := RunEndpoint(context.Background(), contract, EndpointOptions{
				Endpoint:             "https://disposable.example",
				Token:                "primary-token",
				AlternateToken:       test.alternate,
				AlternateTenantToken: test.alternateTenant,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runner error = %v, want %q", err, test.want)
			}
		})
	}
}
