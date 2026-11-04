package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestV2MutationAuthorityResponseKeepsOriginalClaim(t *testing.T) {
	for _, action := range []intentv2.Action{intentv2.ActionCreate, intentv2.ActionUpdate, intentv2.ActionDelete} {
		for _, response := range []struct {
			name, code, want string
			status           int
			malformed, lost  bool
		}{
			{name: "forbidden", status: 403, code: "forbidden", want: "HTTP 403 (forbidden)"},
			{name: "unauthenticated", status: 401, code: "unauthenticated", want: "HTTP 401 (unauthenticated)"},
			{name: "malformed403", status: 403, malformed: true},
			{name: "unavailable", status: 503, code: "temporarily_unavailable"},
			{name: "lost-response", lost: true},
		} {
			t.Run(string(action)+"/"+response.name, func(t *testing.T) {
				const sensitive = "fixture-body-detail-must-not-be-printed"
				var requests, mutations atomic.Int32
				var sentKey atomic.Value
				var server *httptest.Server
				server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					switch {
					case req.URL.Path == hostv2.DiscoveryPath:
						_ = json.NewEncoder(w).Encode(hostv2.Discovery{API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs", Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600}})
					case req.URL.Path == "/api/v2/support":
						_ = json.NewEncoder(w).Encode(hostv2.Support{Form: hostv2TestModuleWorkerURL, Supported: true, Operations: []string{"create", "read", "update", "delete"}, PrivateInputs: false})
					case req.URL.Path == "/api/v2/resources/uid-1" && req.Method == http.MethodGet:
						_ = json.NewEncoder(w).Encode(map[string]any{"uid": "uid-1", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "app", "generation": 1, "observedGeneration": 1, "observedAt": "2026-11-04T00:00:00Z", "phase": "idle", "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-original"})
					case req.URL.Path == "/api/v2/resources" && req.Method == http.MethodPost || req.URL.Path == "/api/v2/resources/uid-1" && (req.Method == http.MethodPut || req.Method == http.MethodDelete):
						mutations.Add(1)
						sentKey.Store(req.Header.Get("Idempotency-Key"))
						if response.lost {
							connection, _, err := w.(http.Hijacker).Hijack()
							if err != nil {
								t.Error(err)
								return
							}
							_ = connection.Close()
							return
						}
						if response.malformed {
							w.WriteHeader(response.status)
							_, _ = w.Write([]byte(sensitive))
							return
						}
						w.Header().Set("Content-Type", "application/problem+json")
						w.WriteHeader(response.status)
						_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": sensitive, "status": response.status, "code": response.code, "detail": sensitive, "operationId": "op-private-untrusted"})
					default:
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
				if action != intentv2.ActionCreate {
					_, err := data.store.BindImportedResource(ctx, intentv2.ImportRequest{Target: target, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Evidence: intentv2.ImportEvidence{HostAPI: data.hostAPIRoot, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 1}})
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
				invoke := func() diag.Diagnostics {
					plan := v2TestPlan(t, schema, "app")
					switch action {
					case intentv2.ActionCreate:
						got := resource.CreateResponse{State: state}
						res.Create(ctx, resource.CreateRequest{Plan: plan}, &got)
						if !got.State.Raw.Equal(state.Raw) {
							t.Fatal("unacknowledged Create altered state")
						}
						return got.Diagnostics
					case intentv2.ActionUpdate:
						got := resource.UpdateResponse{State: state}
						res.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &got)
						if !got.State.Raw.Equal(state.Raw) {
							t.Fatal("unacknowledged Update altered state")
						}
						return got.Diagnostics
					default:
						got := resource.DeleteResponse{State: state}
						res.Delete(ctx, resource.DeleteRequest{State: state}, &got)
						if !got.State.Raw.Equal(state.Raw) {
							t.Fatal("unacknowledged Delete cleared state")
						}
						return got.Diagnostics
					}
				}
				got := invoke()
				if !got.HasError() || mutations.Load() != 1 {
					t.Fatalf("mutation not refused once: diagnostics=%v mutations=%d", got, mutations.Load())
				}
				detail := got.Errors()[0].Detail()
				if response.want != "" && !strings.Contains(detail, response.want) {
					t.Errorf("validated authority response was lost: %s", detail)
				}
				if response.want == "" && (strings.Contains(detail, "HTTP ") || strings.Contains(detail, "forbidden") || strings.Contains(detail, "unauthenticated")) {
					t.Errorf("unvalidated/unavailable response was classified as authority proof: %s", detail)
				}
				if strings.Contains(detail, sensitive) || strings.Contains(detail, "op-private-untrusted") || !strings.Contains(detail, "no automatic") {
					t.Errorf("sensitive detail leaked or retry fence lost: %s", detail)
				}
				history, err := data.store.Read(ctx, target)
				if err != nil || len(history) != 1 || history[0].Status != intentv2.StatusClaimed || history[0].OperationID != "" || history[0].Key != sentKey.Load() || history[0].Action != action || history[0].AdvertisedReplayWindowSecs != 3600 {
					t.Fatalf("original pre-ACK custody changed: history=%+v err=%v", history, err)
				}
				beforeRetry := requests.Load()
				data.client = hostv2.New(server.URL, "corrected-credential", server.Client())
				if again := invoke(); !again.HasError() || requests.Load() != beforeRetry {
					t.Fatalf("credential change resent an unacknowledged intent: diagnostics=%v requests=%d→%d", again, beforeRetry, requests.Load())
				}
				again, err := data.store.Read(ctx, target)
				if err != nil || !reflect.DeepEqual(history, again) {
					t.Fatal("blocked retry rewrote the original intent")
				}
			})
		}
	}
}
