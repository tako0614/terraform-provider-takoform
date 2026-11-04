package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
)

func TestV2CreateDistinguishesUnreadableSupportFromUnsupportedForm(t *testing.T) {
	for _, test := range []struct {
		name, code, wantAuthority string
		status                    int
		malformed, unsupported    bool
		partialCRUD               bool
	}{
		{name: "unauthenticated", status: 401, code: "unauthenticated", wantAuthority: "HTTP 401 (unauthenticated)"},
		{name: "forbidden", status: 403, code: "forbidden", wantAuthority: "HTTP 403 (forbidden)"},
		{name: "unavailable", status: 503, code: "temporarily_unavailable"},
		{name: "malformed-unauthorized", status: 401, malformed: true},
		{name: "malformed-support", status: 200, malformed: true},
		{name: "declared-unsupported", status: 200, unsupported: true},
		{name: "missing-crud", status: 200, partialCRUD: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const sensitive = "fixture-host-detail-token-must-not-be-shown"
			var supportReads, mutations atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == hostv2.DiscoveryPath:
					_ = json.NewEncoder(w).Encode(hostv2.Discovery{
						API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2",
						Documentation:  "https://host.example/docs",
						Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"},
						Limits:         hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600},
					})
				case req.URL.Path == "/api/v2/support" && req.Method == http.MethodGet:
					supportReads.Add(1)
					if test.malformed {
						w.WriteHeader(test.status)
						_, _ = w.Write([]byte(sensitive))
						return
					}
					if test.status != http.StatusOK {
						w.Header().Set("Content-Type", "application/problem+json")
						w.WriteHeader(test.status)
						_ = json.NewEncoder(w).Encode(map[string]any{
							"type": "about:blank", "title": sensitive, "detail": sensitive,
							"status": test.status, "code": test.code,
						})
						return
					}
					operations := []string{}
					if test.partialCRUD {
						operations = []string{"read"}
					}
					_ = json.NewEncoder(w).Encode(hostv2.Support{
						Form: hostv2TestModuleWorkerURL, Supported: !test.unsupported,
						Operations: operations, PrivateInputs: false,
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
			state := tfsdk.State{Schema: schema.Schema, Raw: v2RawState(t, schema)}
			response := resource.CreateResponse{State: state}
			ctx := context.Background()
			res.Create(ctx, resource.CreateRequest{Plan: v2TestPlan(t, schema, "app")}, &response)
			if !response.Diagnostics.HasError() || len(response.Diagnostics.Errors()) != 1 {
				t.Fatalf("expected one pre-mutation diagnostic, got %v", response.Diagnostics)
			}
			got := response.Diagnostics.Errors()[0]
			wantSummary := "v2 Form support unavailable"
			if test.unsupported || test.partialCRUD {
				wantSummary = "Exact Form is not supported"
			}
			if got.Summary() != wantSummary {
				t.Errorf("summary = %q, want %q", got.Summary(), wantSummary)
			}
			if test.wantAuthority != "" && !strings.Contains(got.Detail(), test.wantAuthority) {
				t.Errorf("validated authority response was lost: %s", got.Detail())
			}
			if test.wantAuthority == "" && (strings.Contains(got.Detail(), "HTTP ") || strings.Contains(got.Detail(), "unauthenticated") || strings.Contains(got.Detail(), "forbidden")) {
				t.Errorf("unverified response was classified as authority proof: %s", got.Detail())
			}
			if strings.Contains(got.Detail(), sensitive) || !strings.Contains(got.Detail(), "no mutation was sent") {
				t.Errorf("sensitive detail leaked or pre-mutation boundary lost: %s", got.Detail())
			}
			if supportReads.Load() != 1 || mutations.Load() != 0 || !response.State.Raw.Equal(state.Raw) {
				t.Fatalf("refusal changed request count or state: support=%d mutations=%d", supportReads.Load(), mutations.Load())
			}
			history, err := data.store.Read(ctx, res.target("prod", "app"))
			if err != nil || len(history) != 0 {
				t.Fatalf("support refusal claimed a new intent: history=%+v err=%v", history, err)
			}
		})
	}
}
