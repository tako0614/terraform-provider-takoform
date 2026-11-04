package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/tako0614/terraform-provider-takoform/internal/edgev2"
	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
	"github.com/tako0614/terraform-provider-takoform/internal/privateinputsv2"
)

func TestV2RecoveryBindsOperationBeforeContinuing(t *testing.T) {
	cases := []struct {
		name              string
		id                string
		uid               string
		action            string
		generation        int64
		afterPoll         bool
		afterContinuation bool
		terminal          bool
		success           bool
	}{
		{name: "different UID", id: "op-1", uid: "other-uid", action: "create", generation: 1},
		{name: "different action", id: "op-1", uid: "original-uid", action: "update", generation: 1},
		{name: "different generation", id: "op-1", uid: "original-uid", action: "create", generation: 2},
		{name: "different operation ID", id: "op-other", uid: "original-uid", action: "create", generation: 1},
		{name: "UID changes after poll", id: "op-1", uid: "other-uid", action: "create", generation: 1, afterPoll: true},
		{name: "UID changes after continuation", id: "op-1", uid: "other-uid", action: "create", generation: 1, afterContinuation: true},
		{name: "terminal different UID", id: "op-1", uid: "other-uid", action: "create", generation: 1, terminal: true},
		{name: "same operation continues and settles", id: "op-1", uid: "original-uid", action: "create", generation: 1, success: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(privateinputsv2.EnvironmentVariable, `{"prod":{"app":{"API_TOKEN":"fixture-only"}}}`)
			var operationReads, continuationCalls, resourceReads, resourceWrites atomic.Int32
			retention := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
			var returnedSpec map[string]any
			var server *httptest.Server
			writeOperation := func(w http.ResponseWriter, id, uid, action string, generation int64, status string) {
				effect := "none"
				if status == "succeeded" {
					effect = "complete"
				}
				body := map[string]any{
					"id": id, "resourceUid": uid, "action": action, "generation": generation,
					"status": status, "effect": effect, "createdAt": "2026-10-12T00:00:00Z",
					"updatedAt": "2026-10-12T00:00:00Z", "retainUntil": retention,
				}
				if status == "waiting_input" {
					body["inputRequired"] = map[string]any{"names": []string{"API_TOKEN"}, "reason": "unavailable"}
				}
				_ = json.NewEncoder(w).Encode(body)
			}
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == hostv2.DiscoveryPath && req.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(hostv2.Discovery{
						API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs",
						Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"},
						Capabilities:   hostv2.Capabilities{PrivateInputs: true}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600},
					})
				case req.URL.Path == "/api/v2/operations/op-1" && req.Method == http.MethodGet:
					count := operationReads.Add(1)
					if tc.afterPoll && count == 1 {
						writeOperation(w, "op-1", "original-uid", "create", 1, "running")
					} else if tc.afterContinuation && count == 1 {
						writeOperation(w, "op-1", "original-uid", "create", 1, "waiting_input")
					} else if tc.success && count > 1 {
						writeOperation(w, tc.id, tc.uid, tc.action, tc.generation, "succeeded")
					} else if tc.terminal {
						writeOperation(w, tc.id, tc.uid, tc.action, tc.generation, "succeeded")
					} else {
						writeOperation(w, tc.id, tc.uid, tc.action, tc.generation, "waiting_input")
					}
				case req.URL.Path == "/api/v2/operations/op-1/private-inputs" && req.Method == http.MethodPut:
					if continuationCalls.Add(1) > 1 {
						http.Error(w, "unexpected repeated continuation", http.StatusServiceUnavailable)
						return
					}
					if !tc.success && !tc.afterContinuation {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					var body struct {
						PrivateInputs map[string]string `json:"privateInputs"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil || len(body.PrivateInputs) != 1 || body.PrivateInputs["API_TOKEN"] != "fixture-only" {
						t.Error("continuation did not carry the original synthetic private-input map")
						http.Error(w, "invalid fixture input", http.StatusBadRequest)
						return
					}
					writeOperation(w, "op-1", "original-uid", "create", 1, "running")
				case strings.HasPrefix(req.URL.Path, "/api/v2/resources/") && req.Method == http.MethodGet:
					resourceReads.Add(1)
					if !tc.success || req.URL.Path != "/api/v2/resources/original-uid" {
						http.NotFound(w, req)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"uid": "original-uid", "form": edgev2.WorkerVersionURL, "space": "prod", "name": "app",
						"generation": 1, "observedGeneration": 1, "observedAt": "2026-10-12T00:00:00Z",
						"phase": "idle", "spec": returnedSpec, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-1",
					})
				default:
					if strings.HasPrefix(req.URL.Path, "/api/v2/resources") && req.Method != http.MethodGet {
						resourceWrites.Add(1)
					}
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			data := v2TestData(t, server)
			res, schemaResponse := v2TestResource(t, data, edgev2.WorkerVersionURL)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			plan := v2TestPlan(t, schemaResponse, "app")
			value, err := v2StateSpec(ctx, res.def, map[string]any{
				"worker": map[string]any{"resourceUid": "worker-1"}, "bundle": map[string]any{"resourceUid": "bundle-1"},
				"handlers": []any{"fetch"}, "requiredSensitiveVars": []any{"API_TOKEN"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if diagnostics := plan.SetAttribute(ctx, path.Root("spec"), value); diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			returnedSpec, err = v2SpecFromPlan(ctx, plan, res.def)
			if err != nil {
				t.Fatal(err)
			}
			public, err := jsonv2.Marshal(hostv2.CreateRequest{Form: res.def.form, Space: "prod", Name: "app", Spec: returnedSpec})
			if err != nil {
				t.Fatal(err)
			}
			target := intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "app"}
			intent, err := data.store.Begin(ctx, intentv2.BeginRequest{
				Target: target,
				Action: intentv2.ActionCreate, Form: res.def.form, Method: "POST", Path: "/resources",
				PublicJSON: public, PrivateInputNames: []string{"API_TOKEN"}, AdvertisedReplayWindowSecs: 3600,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = data.store.RecordAccepted(ctx, intent.ID, intentv2.OperationAck{
				ID: "op-1", ResourceUID: "original-uid", Action: intentv2.ActionCreate,
				Generation: 1, Status: "queued", Effect: "none", RetainUntil: retention,
			})
			if err != nil {
				t.Fatal(err)
			}
			response := resource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}}
			res.Create(ctx, resource.CreateRequest{Plan: plan}, &response)
			wantContinuations := int32(0)
			if tc.afterContinuation {
				wantContinuations = 1
			}
			if tc.success {
				if response.Diagnostics.HasError() || continuationCalls.Load() != 1 || resourceReads.Load() != 1 {
					t.Fatalf("same Operation: diagnostics=%v continuations=%d resource reads=%d", response.Diagnostics, continuationCalls.Load(), resourceReads.Load())
				}
			} else if !response.Diagnostics.HasError() || continuationCalls.Load() != wantContinuations || resourceReads.Load() != 0 {
				t.Fatalf("mismatched Operation: diagnostics=%v continuations=%d resource reads=%d", response.Diagnostics, continuationCalls.Load(), resourceReads.Load())
			}
			if resourceWrites.Load() != 0 {
				t.Fatal("recovery sent a new Resource mutation")
			}
			wantReads := int32(1)
			if tc.afterPoll || tc.afterContinuation || tc.success {
				wantReads = 2
			}
			if got := operationReads.Load(); got != wantReads {
				t.Fatalf("Operation reads=%d, want %d", got, wantReads)
			}
			history, err := data.store.Read(ctx, target)
			if err != nil || len(history) != 1 {
				t.Fatalf("read retained intent: count=%d error=%v", len(history), err)
			}
			wantStatus := intentv2.StatusAccepted
			if tc.success {
				wantStatus = intentv2.StatusApplied
			}
			if history[0].Status != wantStatus || history[0].OperationID != "op-1" || history[0].OperationUID != "original-uid" || history[0].OperationGeneration != 1 {
				t.Fatal("retained original Operation or status changed unexpectedly")
			}
		})
	}
}
