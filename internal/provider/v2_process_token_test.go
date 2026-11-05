package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

func configureV2ProcessToken(t *testing.T, token any, storePath string) (frameworkprovider.ConfigureResponse, tftypes.Value) {
	return configureV2ProcessTokenAt(t, token, storePath, "https://host.example", "https://host.example/api")
}

func configureV2ProcessTokenAt(t *testing.T, token any, storePath, endpoint, hostAPIRoot string) (frameworkprovider.ConfigureResponse, tftypes.Value) {
	t.Helper()
	candidate := New("test")()
	var schemaResponse frameworkprovider.SchemaResponse
	candidate.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &schemaResponse)
	typesByName := map[string]tftypes.Type{
		"endpoint": tftypes.String, "host_api_root": tftypes.String, "space": tftypes.String,
		"token": tftypes.String, "store_path": tftypes.String, "intent_store_slot": tftypes.String, "store_id": tftypes.String,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: typesByName}, map[string]tftypes.Value{
		"endpoint":          tftypes.NewValue(tftypes.String, endpoint),
		"host_api_root":     tftypes.NewValue(tftypes.String, hostAPIRoot),
		"space":             tftypes.NewValue(tftypes.String, "team-a"),
		"token":             tftypes.NewValue(tftypes.String, token),
		"store_path":        tftypes.NewValue(tftypes.String, storePath),
		"intent_store_slot": tftypes.NewValue(tftypes.String, nil),
		"store_id":          tftypes.NewValue(tftypes.String, "process-token-test"),
	})
	request := frameworkprovider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: raw}}
	var response frameworkprovider.ConfigureResponse
	candidate.Configure(context.Background(), request, &response)
	return response, raw
}

func initializedProcessTokenStore(t *testing.T) string {
	return initializedProcessTokenStoreAt(t, "https://host.example/api")
}

func initializedProcessTokenStoreAt(t *testing.T, hostAPIRoot string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "intent-store")
	if err := intentv2.InitializeBound(path, "process-token-test", intentv2.StoreBinding{HostAPI: hostAPIRoot}); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestV2ProcessTokenSchemaAndEnvironmentFallback(t *testing.T) {
	candidate := New("test")()
	var schemaResponse frameworkprovider.SchemaResponse
	candidate.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &schemaResponse)
	attr := schemaResponse.Schema.Attributes["token"].(schema.StringAttribute)
	if attr.Required || !attr.Optional || !attr.Sensitive {
		t.Fatalf("token schema = %#v; want Optional and Sensitive", attr)
	}

	const secret = "synthetic-process-env-token"
	t.Setenv(envToken, secret)
	storePath := initializedProcessTokenStore(t)
	response, raw := configureV2ProcessToken(t, nil, storePath)
	if response.Diagnostics.HasError() || response.ResourceData == nil {
		t.Fatalf("omitted token with %s rejected: %#v", envToken, response.Diagnostics)
	}
	var rawAttributes map[string]tftypes.Value
	if err := raw.As(&rawAttributes); err != nil {
		t.Fatal(err)
	}
	if !rawAttributes["token"].IsNull() {
		t.Fatal("raw provider configuration did not retain null token")
	}
	data := response.ResourceData.(*v2ProviderData)
	if data.client == nil {
		t.Fatal("configured provider has no Host client")
	}
	_ = data.store.Close()
	entries, err := os.ReadDir(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("intent store has no persisted files")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(storePath, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(contents, []byte(secret)) {
			t.Fatalf("intent store file %q contains process credential", entry.Name())
		}
	}
}

func TestV2ProcessTokenExplicitValueAndNullFallbackConfigure(t *testing.T) {
	const envSecret = "synthetic-environment-token"
	t.Setenv(envToken, envSecret)
	storePath := initializedProcessTokenStore(t)
	for _, test := range []struct {
		name  string
		token any
	}{
		{name: "explicit overrides environment", token: "synthetic-explicit-token"},
		{name: "explicit null uses environment", token: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, raw := configureV2ProcessToken(t, test.token, storePath)
			if response.Diagnostics.HasError() || response.ResourceData == nil {
				t.Fatalf("configuration failed: %#v", response.Diagnostics)
			}
			var rawAttributes map[string]tftypes.Value
			if err := raw.As(&rawAttributes); err != nil {
				t.Fatal(err)
			}
			if test.token == nil && !rawAttributes["token"].IsNull() {
				t.Fatal("null token was rewritten in raw provider configuration")
			}
			if explicit, ok := test.token.(string); ok {
				var configured string
				if err := rawAttributes["token"].As(&configured); err != nil || configured != explicit {
					t.Fatal("explicit token was rewritten in raw provider configuration")
				}
			}
			_ = response.ResourceData.(*v2ProviderData).store.Close()
		})
	}
}

func TestV2ProcessTokenRejectsInvalidCredentialBeforeStoreOpen(t *testing.T) {
	const envSecret = "synthetic-environment-token"
	for _, test := range []struct {
		name  string
		token any
		env   string
		unset bool
	}{
		{name: "unknown despite environment", token: tftypes.UnknownValue, env: envSecret},
		{name: "explicit empty despite environment", token: "", env: envSecret},
		{name: "missing environment", token: nil, unset: true},
		{name: "empty environment", token: nil, env: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.unset {
				old, hadOld := os.LookupEnv(envToken)
				if err := os.Unsetenv(envToken); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if hadOld {
						_ = os.Setenv(envToken, old)
					} else {
						_ = os.Unsetenv(envToken)
					}
				})
			} else {
				t.Setenv(envToken, test.env)
			}
			missingPath := filepath.Join(t.TempDir(), "must-not-be-opened")
			response, _ := configureV2ProcessToken(t, test.token, missingPath)
			if !response.Diagnostics.HasError() || response.ResourceData != nil {
				t.Fatalf("invalid credential configured: %#v", response.Diagnostics)
			}
			for _, diagnostic := range response.Diagnostics.Errors() {
				if diagnostic.Summary() != "Invalid v2 provider configuration" {
					t.Fatalf("credential refusal reached another configuration boundary: %#v", response.Diagnostics)
				}
			}
			if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
				t.Fatalf("intent store was accessed before credential refusal: stat err=%v", err)
			}
		})
	}
}

func TestV2ProcessTokenOutboundBearerInIsolatedSubprocess(t *testing.T) {
	const childMarker = "TAKOFORM_V2_PROCESS_TOKEN_CHILD"
	if os.Getenv(childMarker) == "1" {
		endpoint := os.Getenv("TAKOFORM_V2_PROCESS_TOKEN_ENDPOINT")
		storePath := os.Getenv("TAKOFORM_V2_PROCESS_TOKEN_STORE")
		token := any(nil)
		if explicit := os.Getenv("TAKOFORM_V2_PROCESS_TOKEN_EXPLICIT"); explicit != "" {
			token = explicit
		}
		response, _ := configureV2ProcessTokenAt(t, token, storePath, endpoint, endpoint+"/api")
		if response.Diagnostics.HasError() || response.ResourceData == nil {
			t.Fatal("provider configuration failed")
		}
		data := response.ResourceData.(*v2ProviderData)
		defer data.store.Close()
		if err := os.Setenv(envToken, "synthetic-after-configure-token"); err != nil {
			t.Fatal("could not change synthetic environment after configure")
		}
		if _, err := data.client.Discover(context.Background()); err != nil {
			t.Fatal("v2 discovery failed")
		}
		if _, err := data.client.GetSupport(context.Background(), "https://forms.example/Widget/1.0.0/"); err != nil {
			t.Fatal("v2 support request failed")
		}
		return
	}

	for _, test := range []struct {
		name        string
		environment string
		explicit    string
		wantBearer  string
	}{
		{name: "environment credential is snapshotted", environment: "synthetic-env-token-before-configure", wantBearer: "synthetic-env-token-before-configure"},
		{name: "explicit credential overrides environment", environment: "synthetic-different-env-token", explicit: "synthetic-explicit-token", wantBearer: "synthetic-explicit-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var supportRequests atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/takoform/v2":
					if got := r.Header.Get("Authorization"); got != "" {
						t.Errorf("discovery Authorization = %q, want no credential", got)
						http.Error(w, "unexpected discovery credential", http.StatusUnauthorized)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"api": "forms.takoform.com/v2", "baseUrl": server.URL + "/api", "documentation": "https://host.example/docs",
						"authentication": map[string]any{"schemes": []string{"Bearer"}, "documentation": "https://host.example/auth"},
						"capabilities":   map[string]any{"offerings": false, "previews": false, "privateInputs": false},
						"limits":         map[string]any{"maxRequestBytes": 65536, "maxPageSize": 50, "replayWindowSeconds": 3600},
					})
				case "/api/support":
					supportRequests.Add(1)
					if got, want := r.Header.Get("Authorization"), "Bearer "+test.wantBearer; got != want {
						t.Errorf("support Authorization = %q, want %q", got, want)
						http.Error(w, "unexpected support credential", http.StatusUnauthorized)
						return
					}
					if r.URL.Query().Get("form") != "https://forms.example/Widget/1.0.0/" {
						t.Errorf("support form query = %q", r.URL.RawQuery)
						http.Error(w, "unexpected support form", http.StatusBadRequest)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"form":"https://forms.example/Widget/1.0.0/","supported":true,"operations":["read"],"privateInputs":false}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			storePath := initializedProcessTokenStoreAt(t, server.URL+"/api")
			caPath := filepath.Join(t.TempDir(), "loopback-ca.pem")
			pem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
			if err := os.WriteFile(caPath, pem, 0o600); err != nil {
				t.Fatal(err)
			}
			childEnv := []string{
				childMarker + "=1",
				"SSL_CERT_FILE=" + caPath,
				envToken + "=" + test.environment,
				"TAKOFORM_V2_PROCESS_TOKEN_ENDPOINT=" + server.URL,
				"TAKOFORM_V2_PROCESS_TOKEN_STORE=" + storePath,
			}
			if test.explicit != "" {
				childEnv = append(childEnv, "TAKOFORM_V2_PROCESS_TOKEN_EXPLICIT="+test.explicit)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestV2ProcessTokenOutboundBearerInIsolatedSubprocess$")
			command.Env = childEnv
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("subprocess provider request failed: %v\n%s", err, output)
			}
			if got := supportRequests.Load(); got != 1 {
				t.Fatalf("authenticated support requests = %d, want one", got)
			}
		})
	}
}
