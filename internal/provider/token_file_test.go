package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func configureTokenSourceTest(t *testing.T, endpoint string, token, tokenFile any) frameworkprovider.ConfigureResponse {
	t.Helper()
	candidate := &takoformProvider{}
	var schemaResponse frameworkprovider.SchemaResponse
	candidate.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &schemaResponse)
	configType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"endpoint": tftypes.String, "space": tftypes.String, "token": tftypes.String,
		"token_file": tftypes.String, "runtime_input_nonce": tftypes.String,
		"runtime_inputs": tftypes.Map{ElementType: tftypes.String},
	}}
	request := frameworkprovider.ConfigureRequest{Config: tfsdk.Config{
		Schema: schemaResponse.Schema,
		Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
			"endpoint":            tftypes.NewValue(tftypes.String, endpoint),
			"space":               tftypes.NewValue(tftypes.String, nil),
			"token":               tftypes.NewValue(tftypes.String, token),
			"token_file":          tftypes.NewValue(tftypes.String, tokenFile),
			"runtime_input_nonce": tftypes.NewValue(tftypes.String, nil),
			"runtime_inputs":      tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil),
		}),
	}}
	var response frameworkprovider.ConfigureResponse
	candidate.Configure(context.Background(), request, &response)
	return response
}

func TestProviderTokenSourcePrecedenceAndConflict(t *testing.T) {
	var requests atomic.Int32
	var gotAuthorization atomic.Value
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		gotAuthorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api_versions":["forms.takoform.com/v1"],"features":{"service_forms":true,"exact_form_ref":true,"optimistic_concurrency":true,"idempotent_lifecycle":true,"operations":true,"artifact_upload":true,"support_profiles":true},"endpoints":{"api":"` + server.URL + `/apis/forms.takoform.com/v1"}}`))
	}))
	defer server.Close()
	t.Setenv(envToken, "environment-static")
	t.Setenv(envTokenFile, "")

	response := configureTokenSourceTest(t, server.URL, "explicit-static", nil)
	if response.Diagnostics.HasError() || response.ResourceData == nil || gotAuthorization.Load() != "Bearer explicit-static" {
		t.Fatalf("explicit token did not override same-kind environment: diagnostics=%v auth=%v", response.Diagnostics, gotAuthorization.Load())
	}
	before := requests.Load()
	response = configureTokenSourceTest(t, server.URL, "explicit-static", "some-path")
	if !response.Diagnostics.HasError() || response.ResourceData != nil || requests.Load() != before {
		t.Fatalf("static and file sources were not rejected before HTTP: diagnostics=%v requests=%d", response.Diagnostics, requests.Load()-before)
	}

	t.Setenv(envToken, "")
	t.Setenv(envTokenFile, "missing-environment-path")
	path := filepath.Join(t.TempDir(), "current")
	if err := os.WriteFile(path, []byte("explicit-file"), 0600); err != nil {
		t.Fatal(err)
	}
	response = configureTokenSourceTest(t, server.URL, nil, path)
	if response.Diagnostics.HasError() || response.ResourceData == nil || gotAuthorization.Load() != "Bearer explicit-file" {
		t.Fatalf("explicit file did not override same-kind environment: diagnostics=%v auth=%v", response.Diagnostics, gotAuthorization.Load())
	}
	before = requests.Load()
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	configured := response.ResourceData.(*providerData)
	if _, err := configured.clientV3.GetOperation(context.Background(), "op_current"); err == nil {
		t.Fatal("insecure rotated token file allowed an operation poll")
	}
	if requests.Load() != before {
		t.Fatalf("insecure rotated file sent %d Host requests", requests.Load()-before)
	}

	t.Setenv(envTokenFile, "")
	response = configureTokenSourceTest(t, server.URL, nil, nil)
	if response.Diagnostics.HasError() || response.ResourceData == nil || gotAuthorization.Load() != "" {
		t.Fatalf("anonymous provider behavior changed: diagnostics=%v auth=%v", response.Diagnostics, gotAuthorization.Load())
	}
}

func TestProviderInvalidTokenFileFailsBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv(envToken, "")
	t.Setenv(envTokenFile, "")
	path := filepath.Join(t.TempDir(), "missing-secret-path")
	response := configureTokenSourceTest(t, server.URL, nil, path)
	data, ok := response.ResourceData.(*providerData)
	if !ok || data.v3Err == nil || data.clientV3 != nil || requests.Load() != 0 {
		t.Fatalf("invalid token source did not fail closed before HTTP: data=%T requests=%d", response.ResourceData, requests.Load())
	}
	if strings.Contains(data.v3Err.Error(), path) || strings.Contains(data.v3Err.Error(), "missing-secret-path") {
		t.Fatalf("credential path escaped into diagnostic: %v", data.v3Err)
	}
}
