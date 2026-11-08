package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

func TestV2ExactResourceReadDistinguishesAuthorityRefusal(t *testing.T) {
	for _, mode := range []string{"import", "refresh", "update", "delete"} {
		for _, failure := range []struct {
			name, code, want string
			status           int
			malformed        bool
		}{
			{name: "unauthenticated", status: 401, code: "unauthenticated", want: "HTTP 401 (unauthenticated)"},
			{name: "forbidden", status: 403, code: "forbidden", want: "HTTP 403 (forbidden)"},
			{name: "malformed-forbidden", status: 403, malformed: true},
			{name: "unavailable", status: 503, code: "temporarily_unavailable"},
		} {
			t.Run(mode+"/"+failure.name, func(t *testing.T) {
				const sensitive = "fixture-host-detail-must-not-be-shown"
				var resourceReads, mutations atomic.Int32
				var server *httptest.Server
				server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case req.URL.Path == hostv2.DiscoveryPath && req.Method == http.MethodGet:
						_ = json.NewEncoder(w).Encode(hostv2.Discovery{
							API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2",
							Documentation:  "https://host.example/docs",
							Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"},
							Limits:         hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600},
						})
					case req.URL.Path == "/api/v2/resources/uid-1" && req.Method == http.MethodGet:
						resourceReads.Add(1)
						if failure.malformed {
							w.WriteHeader(failure.status)
							_, _ = w.Write([]byte(sensitive))
							return
						}
						w.Header().Set("Content-Type", "application/problem+json")
						w.WriteHeader(failure.status)
						_ = json.NewEncoder(w).Encode(map[string]any{
							"type": "about:blank", "title": sensitive, "detail": sensitive,
							"status": failure.status, "code": failure.code,
						})
					default:
						if req.Method != http.MethodGet {
							mutations.Add(1)
						}
						t.Errorf("unexpected Host request: %s %s", req.Method, req.URL.Path)
						http.NotFound(w, req)
					}
				}))
				defer server.Close()

				data := v2TestData(t, server)
				res, schema := v2TestResource(t, data, hostv2TestModuleWorkerURL)
				ctx := context.Background()
				state := tfsdk.State{Schema: schema.Schema, Raw: v2RawState(t, schema)}
				target := res.target("prod", "app")
				if mode == "import" {
					imported := resource.ImportStateResponse{State: state}
					res.ImportState(ctx, resource.ImportStateRequest{ID: "uid-1"}, &imported)
					if imported.Diagnostics.HasError() {
						t.Fatal(imported.Diagnostics)
					}
					state = imported.State
				} else {
					_, err := data.store.BindImportedResource(ctx, intentv2.ImportRequest{
						Target: target, UID: "uid-1", Form: hostv2TestModuleWorkerURL,
						Evidence: intentv2.ImportEvidence{HostAPI: data.hostAPIRoot, Path: "/resources/uid-1", StatusCode: 200,
							UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 1},
					})
					if err != nil {
						t.Fatal(err)
					}
					for key, value := range map[string]string{"name": "app", "space": "prod", "uid": "uid-1", "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID, "phase": "idle"} {
						if d := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); d.HasError() {
							t.Fatal(d)
						}
					}
					if d := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(1)); d.HasError() {
						t.Fatal(d)
					}
				}
				var got diag.Diagnostics
				switch mode {
				case "import", "refresh":
					response := resource.ReadResponse{State: state}
					res.Read(ctx, resource.ReadRequest{State: state}, &response)
					if !response.State.Raw.Equal(state.Raw) {
						t.Fatal("failed exact UID read changed state")
					}
					got = response.Diagnostics
				case "update":
					response := resource.UpdateResponse{State: state}
					res.Update(ctx, resource.UpdateRequest{State: state, Plan: v2TestPlan(t, schema, "app")}, &response)
					if !response.State.Raw.Equal(state.Raw) {
						t.Fatal("failed exact UID read changed state")
					}
					got = response.Diagnostics
				case "delete":
					response := resource.DeleteResponse{State: state}
					res.Delete(ctx, resource.DeleteRequest{State: state}, &response)
					if !response.State.Raw.Equal(state.Raw) {
						t.Fatal("failed exact UID read cleared state")
					}
					got = response.Diagnostics
				}
				if !got.HasError() || len(got.Errors()) != 1 {
					t.Fatalf("expected one exact UID read diagnostic, got %v", got)
				}
				detail := got.Errors()[0].Detail()
				if failure.want != "" && !strings.Contains(detail, failure.want) {
					t.Errorf("validated authority response was lost: %s", detail)
				}
				if failure.want == "" && (strings.Contains(detail, "HTTP ") || strings.Contains(detail, "unauthenticated") || strings.Contains(detail, "forbidden")) {
					t.Errorf("unverified response was classified as authority proof: %s", detail)
				}
				if strings.Contains(detail, sensitive) || resourceReads.Load() != 1 || mutations.Load() != 0 {
					t.Fatalf("read detail leaked or mutation occurred: detail=%s reads=%d mutations=%d", detail, resourceReads.Load(), mutations.Load())
				}
				history, err := data.store.Read(ctx, target)
				if err != nil || len(history) != 0 {
					t.Fatalf("failed read created an intent: history=%+v err=%v", history, err)
				}
				_, bound, err := data.store.ReadImportBinding(ctx, target)
				if err != nil || bound != (mode != "import") {
					t.Fatalf("failed read changed import binding: bound=%v err=%v", bound, err)
				}
			})
		}
	}
}

func TestV2CompletedOperationReadKeepsStateOnAuthorityRefusal(t *testing.T) {
	for _, status := range []intentv2.Status{intentv2.StatusApplied, intentv2.StatusDeleteConfirmed} {
		for _, failure := range []struct {
			name, code, want string
			status           int
			malformed        bool
		}{
			{name: "unauthenticated", status: 401, code: "unauthenticated", want: "HTTP 401 (unauthenticated)"},
			{name: "forbidden", status: 403, code: "forbidden", want: "HTTP 403 (forbidden)"},
			{name: "malformed-forbidden", status: 403, malformed: true},
		} {
			t.Run(string(status)+"/"+failure.name, func(t *testing.T) {
				const sensitive = "fixture-completed-detail-must-not-be-shown"
				var reads, mutations atomic.Int32
				var server *httptest.Server
				server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if req.Method == http.MethodGet && req.URL.Path == hostv2.DiscoveryPath {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(hostv2.Discovery{
							API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2",
							Documentation:  "https://host.example/docs",
							Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"},
							Limits:         hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600},
						})
						return
					}
					if req.Method == http.MethodGet && req.URL.Path == "/api/v2/resources/uid-1" {
						reads.Add(1)
						if failure.malformed {
							w.WriteHeader(failure.status)
							_, _ = w.Write([]byte(sensitive))
							return
						}
						w.Header().Set("Content-Type", "application/problem+json")
						w.WriteHeader(failure.status)
						_ = json.NewEncoder(w).Encode(map[string]any{
							"type": "about:blank", "title": sensitive, "detail": sensitive,
							"status": failure.status, "code": failure.code,
						})
						return
					}
					mutations.Add(1)
					t.Errorf("unexpected Host request: %s %s", req.Method, req.URL.Path)
					http.NotFound(w, req)
				}))
				defer server.Close()
				data := v2TestData(t, server)
				res, schema := v2TestResource(t, data, hostv2TestModuleWorkerURL)
				if _, err := res.discover(context.Background()); err != nil {
					t.Fatal(err)
				}
				state := tfsdk.State{Schema: schema.Schema, Raw: v2RawState(t, schema)}
				original := state.Raw
				var got diag.Diagnostics
				res.readCompletedOperation(context.Background(), intentv2.Intent{
					Status: status, OperationUID: "uid-1",
				}, &state, &got)
				if !got.HasError() || len(got.Errors()) != 1 || !state.Raw.Equal(original) {
					t.Fatalf("completed read did not retain state: diagnostics=%v", got)
				}
				detail := got.Errors()[0].Detail()
				if failure.want != "" && !strings.Contains(detail, failure.want) {
					t.Errorf("validated authority response was lost: %s", detail)
				}
				if failure.want == "" && (strings.Contains(detail, "HTTP ") || strings.Contains(detail, "forbidden")) {
					t.Errorf("unverified response was classified as authority proof: %s", detail)
				}
				if strings.Contains(detail, sensitive) || reads.Load() != 1 || mutations.Load() != 0 {
					t.Fatalf("completed read leaked or mutated: detail=%s reads=%d mutations=%d", detail, reads.Load(), mutations.Load())
				}
			})
		}
	}
}
