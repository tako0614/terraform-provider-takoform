package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

func configureV2Store(t *testing.T, storePath, slot any) frameworkprovider.ConfigureResponse {
	t.Helper()
	candidate := &v2Provider{}
	var schemaResponse frameworkprovider.SchemaResponse
	candidate.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &schemaResponse)
	typesByName := map[string]tftypes.Type{
		"endpoint": tftypes.String, "host_api_root": tftypes.String, "space": tftypes.String,
		"token": tftypes.String, "store_path": tftypes.String, "intent_store_slot": tftypes.String, "store_id": tftypes.String,
	}
	request := frameworkprovider.ConfigureRequest{Config: tfsdk.Config{
		Schema: schemaResponse.Schema,
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: typesByName}, map[string]tftypes.Value{
			"endpoint":          tftypes.NewValue(tftypes.String, "https://host.example"),
			"host_api_root":     tftypes.NewValue(tftypes.String, "https://host.example/custom/api"),
			"space":             tftypes.NewValue(tftypes.String, "team-a"),
			"token":             tftypes.NewValue(tftypes.String, "host-token"),
			"store_path":        tftypes.NewValue(tftypes.String, storePath),
			"intent_store_slot": tftypes.NewValue(tftypes.String, slot),
			"store_id":          tftypes.NewValue(tftypes.String, "workspace-provider-v2"),
		}),
	}}
	var response frameworkprovider.ConfigureResponse
	candidate.Configure(context.Background(), request, &response)
	return response
}

func TestV2ProviderRequiresExactlyOneIntentStoreBackend(t *testing.T) {
	for _, tc := range []struct {
		name string
		path any
		slot any
	}{
		{"neither", nil, nil},
		{"both", "/tmp/a", "intent-one"},
		{"unknown path", tftypes.UnknownValue, "intent-one"},
		{"empty slot", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := configureV2Store(t, tc.path, tc.slot)
			if !response.Diagnostics.HasError() || response.ResourceData != nil {
				t.Fatalf("invalid selection was configured: %#v", response.Diagnostics)
			}
		})
	}
}

func TestV2ProviderLocalStoreSelectionRemainsExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-intents")
	if err := intentv2.InitializeBound(path, "workspace-provider-v2", intentv2.StoreBinding{HostAPI: "https://host.example/custom/api"}); err != nil {
		t.Fatal(err)
	}
	response := configureV2Store(t, path, nil)
	if response.Diagnostics.HasError() || response.ResourceData == nil {
		t.Fatalf("explicit local store rejected: %#v", response.Diagnostics)
	}
	_ = response.ResourceData.(*v2ProviderData).store.Close()
}

func TestV2ProviderRemoteSlotInitialization(t *testing.T) {
	var snapshot []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer grant-value" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/private-run-files/v1/read":
			if snapshot == nil {
				_, _ = w.Write([]byte(`{"kind":"absent"}`))
				return
			}
			sum := sha256.Sum256(snapshot)
			_ = json.NewEncoder(w).Encode(map[string]any{"kind": "present", "version": 1, "digest": "sha256:" + hex.EncodeToString(sum[:]), "bytesBase64": snapshot})
		case "/internal/private-run-files/v1/commit":
			var request struct {
				ExpectedVersion uint64 `json:"expectedVersion"`
				CommitID        string `json:"commitId"`
				BytesBase64     []byte `json:"bytesBase64"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.ExpectedVersion != 0 {
				t.Errorf("invalid initial CAS: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			snapshot = request.BytesBase64
			sum := sha256.Sum256(snapshot)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"kind":    "committed",
				"receipt": map[string]any{"version": 1, "expectedVersion": 0, "commitId": request.CommitID, "digest": "sha256:" + hex.EncodeToString(sum[:])},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(intentv2.PrivateRunFilesEnvironment, fmt.Sprintf(`[{"slot":"intent-one","endpoint":%q,"grant":"grant-value"}]`, server.URL+"/internal/private-run-files/v1"))
	response := configureV2Store(t, nil, "intent-one")
	if response.Diagnostics.HasError() || response.ResourceData == nil {
		t.Fatalf("remote slot not configured: %#v", response.Diagnostics)
	}
	if binding, err := response.ResourceData.(*v2ProviderData).store.Binding(context.Background()); err != nil || binding.HostAPI != "https://host.example/custom/api" {
		t.Fatalf("remote binding mismatch: %v, %#v", err, binding)
	}
	_ = response.ResourceData.(*v2ProviderData).store.Close()
}
