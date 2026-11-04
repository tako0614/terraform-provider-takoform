package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

func TestV2ReadRecoversDurableOperationAfterStateLoss(t *testing.T) {
	tests := []struct {
		name            string
		action          intentv2.Action
		settled         bool
		claimedOnly     bool
		failed          bool
		stateGeneration int64
		stateUID        string
		returnedUID     string
		unavailable     bool
		badResource     bool
		badLastOp       bool
		gone            bool
		malformedGone   bool
		linked          bool
		wantError       bool
		wantOperation   int32
		wantResource    int32
		wantStatus      intentv2.Status
	}{
		{name: "accepted update", action: intentv2.ActionUpdate, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusApplied},
		{name: "completed update", action: intentv2.ActionUpdate, settled: true, wantResource: 1, wantStatus: intentv2.StatusApplied},
		{name: "completed update with pending state and expired operation", action: intentv2.ActionUpdate, settled: true, linked: true, stateGeneration: 2, wantResource: 1, wantStatus: intentv2.StatusApplied},
		{name: "accepted delete", action: intentv2.ActionDelete, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusDeleteConfirmed},
		{name: "accepted delete permits retained deletion marker", action: intentv2.ActionDelete, gone: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusDeleteConfirmed},
		{name: "malformed delete marker is not absence", action: intentv2.ActionDelete, gone: true, malformedGone: true, wantError: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusAccepted},
		{name: "completed delete", action: intentv2.ActionDelete, settled: true, wantResource: 1, wantStatus: intentv2.StatusDeleteConfirmed},
		{name: "completed delete permits retained deletion marker", action: intentv2.ActionDelete, settled: true, gone: true, wantResource: 1, wantStatus: intentv2.StatusDeleteConfirmed},
		{name: "completed delete with pending state", action: intentv2.ActionDelete, settled: true, linked: true, stateGeneration: 2, wantResource: 1, wantStatus: intentv2.StatusDeleteConfirmed},
		{name: "accepted update with pending state", action: intentv2.ActionUpdate, linked: true, stateGeneration: 2, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusApplied},
		{name: "accepted failed update", action: intentv2.ActionUpdate, failed: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusFailedReserved},
		{name: "failed delete cannot use retained deletion marker", action: intentv2.ActionDelete, failed: true, gone: true, wantError: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusAccepted},
		{name: "completed failed update", action: intentv2.ActionUpdate, failed: true, settled: true, wantResource: 1, wantStatus: intentv2.StatusFailedReserved},
		{name: "completed failed update with pending state", action: intentv2.ActionUpdate, failed: true, settled: true, linked: true, stateGeneration: 2, wantResource: 1, wantStatus: intentv2.StatusFailedReserved},
		{name: "different old generation", action: intentv2.ActionUpdate, stateGeneration: 7, wantError: true, wantStatus: intentv2.StatusAccepted},
		{name: "different state UID", action: intentv2.ActionUpdate, stateUID: "other-uid", wantError: true, wantStatus: intentv2.StatusAccepted},
		{name: "unacknowledged claim", action: intentv2.ActionUpdate, claimedOnly: true, wantError: true, wantStatus: intentv2.StatusClaimed},
		{name: "unacknowledged delete cannot use retained deletion marker", action: intentv2.ActionDelete, claimedOnly: true, gone: true, wantError: true, wantStatus: intentv2.StatusClaimed},
		{name: "operation UID changed", action: intentv2.ActionUpdate, returnedUID: "other-uid", wantError: true, wantOperation: 1, wantStatus: intentv2.StatusAccepted},
		{name: "operation unavailable", action: intentv2.ActionUpdate, unavailable: true, wantError: true, wantOperation: 1, wantStatus: intentv2.StatusAccepted},
		{name: "wrong update resource", action: intentv2.ActionUpdate, badResource: true, wantError: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusAccepted},
		{name: "gone is not successful update", action: intentv2.ActionUpdate, gone: true, wantError: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusAccepted},
		{name: "delete resource still present", action: intentv2.ActionDelete, badResource: true, wantError: true, wantOperation: 1, wantResource: 1, wantStatus: intentv2.StatusAccepted},
		{name: "completed update resource changed", action: intentv2.ActionUpdate, settled: true, badResource: true, wantError: true, wantResource: 1, wantStatus: intentv2.StatusApplied},
		{name: "completed update last operation changed", action: intentv2.ActionUpdate, settled: true, badLastOp: true, wantError: true, wantResource: 1, wantStatus: intentv2.StatusApplied},
		{name: "completed delete resource present", action: intentv2.ActionDelete, settled: true, badResource: true, wantError: true, wantResource: 1, wantStatus: intentv2.StatusDeleteConfirmed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var operations, reads, mutations atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == hostv2.DiscoveryPath && req.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(hostv2.Discovery{API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs", Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600}})
				case req.URL.Path == "/api/v2/operations/op-original" && req.Method == http.MethodGet:
					operations.Add(1)
					if tc.settled {
						v2WriteNotFound(w)
						return
					}
					if tc.unavailable {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					uid := tc.returnedUID
					if uid == "" {
						uid = "uid-1"
					}
					status, effect := "succeeded", "complete"
					if tc.failed {
						status, effect = "failed", "none"
					}
					v2WriteOperationFor(w, "op-original", uid, string(tc.action), 2, status, effect)
				case req.URL.Path == "/api/v2/resources/uid-1" && req.Method == http.MethodGet:
					reads.Add(1)
					if tc.gone {
						if tc.malformedGone {
							http.Error(w, "not a problem document", http.StatusGone)
							return
						}
						w.Header().Set("Content-Type", "application/problem+json")
						w.WriteHeader(http.StatusGone)
						_, _ = w.Write([]byte(`{"type":"about:blank","title":"Gone","status":410,"code":"gone"}`))
						return
					}
					if tc.action == intentv2.ActionDelete && !tc.failed && !tc.badResource {
						v2WriteNotFound(w)
						return
					}
					phase := "idle"
					if tc.failed {
						phase = "error"
					}
					resourceGeneration := 2
					if tc.badResource {
						resourceGeneration = 3
					}
					lastOperation := "op-original"
					if tc.badLastOp {
						lastOperation = "op-other"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"uid": "uid-1", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "app", "generation": resourceGeneration, "observedGeneration": resourceGeneration, "observedAt": "2026-10-12T00:00:00Z", "phase": phase, "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": lastOperation})
				default:
					if req.Method != http.MethodGet {
						mutations.Add(1)
					}
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			data := v2TestData(t, server)
			res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			target := intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "app"}
			_, err := data.store.BindImportedResource(ctx, intentv2.ImportRequest{Target: target, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Evidence: intentv2.ImportEvidence{HostAPI: data.hostAPIRoot, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 1}})
			if err != nil {
				t.Fatal(err)
			}
			method, public := "PUT", []byte(`{"spec":{}}`)
			if tc.action == intentv2.ActionDelete {
				method, public = "DELETE", nil
			}
			intent, err := data.store.Begin(ctx, intentv2.BeginRequest{Target: target, Action: tc.action, Form: hostv2TestModuleWorkerURL, Method: method, Path: "/resources/uid-1", UID: "uid-1", Generation: "1", PublicJSON: public, AdvertisedReplayWindowSecs: 3600})
			if err != nil {
				t.Fatal(err)
			}
			retain := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
			if !tc.claimedOnly {
				_, err = data.store.RecordAccepted(ctx, intent.ID, intentv2.OperationAck{ID: "op-original", ResourceUID: "uid-1", Action: tc.action, Generation: 2, Status: "running", Effect: "none", RetainUntil: retain})
				if err != nil {
					t.Fatal(err)
				}
			}
			if tc.settled {
				status, effect := "succeeded", "complete"
				observation := intentv2.ResourceObservation{Present: true, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 2, Phase: "idle"}
				if tc.action == intentv2.ActionDelete {
					observation = intentv2.ResourceObservation{Path: "/resources/uid-1", StatusCode: 404}
				}
				if tc.failed {
					status, effect, observation.Phase = "failed", "none", "error"
				}
				_, err = data.store.Complete(ctx, intent.ID, intentv2.CompletionEvidence{Operation: intentv2.OperationAck{ID: "op-original", ResourceUID: "uid-1", Action: tc.action, Generation: 2, Status: status, Effect: effect, RetainUntil: retain}, Resource: observation})
				if err != nil {
					t.Fatal(err)
				}
			}
			state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
			uid := tc.stateUID
			if uid == "" {
				uid = "uid-1"
			}
			for key, value := range map[string]string{"name": "app", "space": "prod", "uid": uid, "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID, "phase": "idle"} {
				if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
					t.Fatal(diags)
				}
			}
			generation := tc.stateGeneration
			if generation == 0 {
				generation = 1
			}
			if diags := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(generation)); diags.HasError() {
				t.Fatal(diags)
			}
			if tc.linked {
				for key, value := range map[string]string{"intent_id": intent.ID, "operation_id": "op-original"} {
					if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
						t.Fatal(diags)
					}
				}
				if diags := state.SetAttribute(ctx, path.Root("phase"), types.StringValue("pending")); diags.HasError() {
					t.Fatal(diags)
				}
			}
			response := resource.ReadResponse{State: state}
			res.Read(ctx, resource.ReadRequest{State: state}, &response)
			if response.Diagnostics.HasError() != tc.wantError || operations.Load() != tc.wantOperation || reads.Load() != tc.wantResource || mutations.Load() != 0 {
				t.Fatalf("state recovery: diagnostics=%v operation GET=%d resource GET=%d mutations=%d", response.Diagnostics, operations.Load(), reads.Load(), mutations.Load())
			}
			history, err := data.store.Read(ctx, target)
			if err != nil || len(history) != 1 || history[0].Status != tc.wantStatus || history[0].ID != intent.ID {
				t.Fatalf("retained intent: count=%d error=%v", len(history), err)
			}
			if tc.wantError {
				if response.State.Raw.IsNull() {
					t.Fatal("refusal cleared the original Terraform state")
				}
				var unchanged types.Int64
				if diags := response.State.GetAttribute(ctx, path.Root("generation"), &unchanged); diags.HasError() || unchanged.ValueInt64() != generation {
					t.Fatalf("refusal advanced state: %v diagnostics=%v", unchanged, diags)
				}
				return
			}
			// The same old state may be read again if Terraform did not persist
			// the previous response. The durable completion and a fresh exact-UID
			// observation must still suffice without another Resource mutation.
			repeated := resource.ReadResponse{State: state}
			res.Read(ctx, resource.ReadRequest{State: state}, &repeated)
			if repeated.Diagnostics.HasError() || operations.Load() != tc.wantOperation || reads.Load() != tc.wantResource+1 || mutations.Load() != 0 {
				t.Fatalf("repeated old state: diagnostics=%v operation GET=%d resource GET=%d mutations=%d", repeated.Diagnostics, operations.Load(), reads.Load(), mutations.Load())
			}
			if tc.failed && len(repeated.Diagnostics.Warnings()) == 0 {
				t.Fatal("repeated failed operation must remain visible as a warning")
			}
			if tc.action == intentv2.ActionDelete {
				if !response.State.Raw.IsNull() {
					t.Fatal("confirmed delete left Terraform state")
				}
				return
			}
			var resultGeneration types.Int64
			if diags := response.State.GetAttribute(ctx, path.Root("generation"), &resultGeneration); diags.HasError() || resultGeneration.ValueInt64() != 2 {
				t.Fatalf("recovered generation: %v diagnostics=%v", resultGeneration, diags)
			}
			if tc.failed {
				if len(response.Diagnostics.Warnings()) == 0 {
					t.Fatal("failed operation must remain visible as a warning")
				}
				var resultPhase, resultIntent types.String
				if diags := response.State.GetAttribute(ctx, path.Root("phase"), &resultPhase); diags.HasError() || resultPhase.ValueString() != "error" {
					t.Fatalf("failed operation phase: %v diagnostics=%v", resultPhase, diags)
				}
				if diags := response.State.GetAttribute(ctx, path.Root("intent_id"), &resultIntent); diags.HasError() || resultIntent.ValueString() != intent.ID {
					t.Fatalf("failed operation linkage: %v diagnostics=%v", resultIntent, diags)
				}
			}
		})
	}
}

func TestV2ReadRecoversOnlyExactSupersededFailure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		completed    bool
		failed       bool
		wrongPriorOp bool
		missingLink  bool
		priorCreate  bool
		wantError    bool
		wantOpReads  int32
		wantStatus   intentv2.Status
	}{
		{name: "accepted successor", wantOpReads: 1, wantStatus: intentv2.StatusApplied},
		{name: "successor to failed create", priorCreate: true, wantOpReads: 1, wantStatus: intentv2.StatusApplied},
		{name: "completed successor", completed: true, wantStatus: intentv2.StatusApplied},
		{name: "failed successor", failed: true, wantOpReads: 1, wantStatus: intentv2.StatusFailedReserved},
		{name: "wrong predecessor operation", wrongPriorOp: true, wantError: true, wantStatus: intentv2.StatusAccepted},
		{name: "missing predecessor linkage", missingLink: true, wantError: true, wantStatus: intentv2.StatusAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priorGeneration, latestGeneration := int64(2), int64(3)
			if tc.priorCreate {
				priorGeneration, latestGeneration = 1, 2
			}
			var operationReads, resourceReads, mutations atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == hostv2.DiscoveryPath && req.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(hostv2.Discovery{API: hostv2.APIVersion, BaseURL: server.URL + "/api/v2", Documentation: "https://host.example/docs", Authentication: hostv2.Authentication{Schemes: []string{"Bearer"}, Documentation: "https://host.example/auth"}, Limits: hostv2.Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600}})
				case req.URL.Path == "/api/v2/operations/op-successor" && req.Method == http.MethodGet:
					operationReads.Add(1)
					status, effect := "succeeded", "complete"
					if tc.failed {
						status, effect = "failed", "none"
					}
					v2WriteOperationFor(w, "op-successor", "uid-1", "update", latestGeneration, status, effect)
				case req.URL.Path == "/api/v2/resources/uid-1" && req.Method == http.MethodGet:
					resourceReads.Add(1)
					phase := "idle"
					if tc.failed {
						phase = "error"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"uid": "uid-1", "form": hostv2TestModuleWorkerURL, "space": "prod", "name": "app", "generation": latestGeneration, "observedGeneration": latestGeneration, "observedAt": "2026-10-12T00:00:00Z", "phase": phase, "spec": map[string]any{}, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": "op-successor"})
				default:
					if req.Method != http.MethodGet {
						mutations.Add(1)
					}
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			data := v2TestData(t, server)
			res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			target := intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "app"}
			begin := intentv2.BeginRequest{Target: target, Action: intentv2.ActionUpdate, Form: hostv2TestModuleWorkerURL, Method: "PUT", Path: "/resources/uid-1", UID: "uid-1", Generation: "1", PublicJSON: []byte(`{"spec":{}}`), AdvertisedReplayWindowSecs: 3600}
			if tc.priorCreate {
				begin.Action, begin.Method, begin.Path, begin.UID, begin.Generation = intentv2.ActionCreate, "POST", "/resources", "", ""
				begin.PublicJSON = []byte(`{"form":"` + hostv2TestModuleWorkerURL + `","space":"prod","name":"app","spec":{}}`)
			} else {
				_, err := data.store.BindImportedResource(ctx, intentv2.ImportRequest{Target: target, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Evidence: intentv2.ImportEvidence{HostAPI: data.hostAPIRoot, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 1}})
				if err != nil {
					t.Fatal(err)
				}
			}
			prior, err := data.store.Begin(ctx, begin)
			if err != nil {
				t.Fatal(err)
			}
			retain := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
			priorAck := intentv2.OperationAck{ID: "op-failed", ResourceUID: "uid-1", Action: begin.Action, Generation: priorGeneration, Status: "failed", Effect: "none", RetainUntil: retain}
			if _, err = data.store.RecordAccepted(ctx, prior.ID, priorAck); err != nil {
				t.Fatal(err)
			}
			priorResource := intentv2.ResourceObservation{Present: true, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: priorGeneration, Phase: "error"}
			if _, err = data.store.Complete(ctx, prior.ID, intentv2.CompletionEvidence{Operation: priorAck, Resource: priorResource}); err != nil {
				t.Fatal(err)
			}
			begin.Action, begin.Method, begin.Path, begin.UID = intentv2.ActionUpdate, "PUT", "/resources/uid-1", "uid-1"
			begin.Generation, begin.SupersedesIntentID, begin.PublicJSON = strconv.FormatInt(priorGeneration, 10), prior.ID, []byte(`{"spec":{}}`)
			latest, err := data.store.Begin(ctx, begin)
			if err != nil {
				t.Fatal(err)
			}
			latestAck := intentv2.OperationAck{ID: "op-successor", ResourceUID: "uid-1", Action: intentv2.ActionUpdate, Generation: latestGeneration, Status: "running", Effect: "none", RetainUntil: retain}
			if _, err = data.store.RecordAccepted(ctx, latest.ID, latestAck); err != nil {
				t.Fatal(err)
			}
			if tc.completed {
				latestAck.Status, latestAck.Effect = "succeeded", "complete"
				observation := priorResource
				observation.Generation, observation.Phase = latestGeneration, "idle"
				if _, err = data.store.Complete(ctx, latest.ID, intentv2.CompletionEvidence{Operation: latestAck, Resource: observation}); err != nil {
					t.Fatal(err)
				}
			}
			state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
			for key, value := range map[string]string{"name": "app", "space": "prod", "uid": "uid-1", "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID, "phase": "error"} {
				if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
					t.Fatal(diags)
				}
			}
			if diags := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(priorGeneration)); diags.HasError() {
				t.Fatal(diags)
			}
			if !tc.missingLink {
				priorOp := "op-failed"
				if tc.wrongPriorOp {
					priorOp = "op-other"
				}
				for key, value := range map[string]string{"intent_id": prior.ID, "operation_id": priorOp} {
					if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
						t.Fatal(diags)
					}
				}
			}
			response := resource.ReadResponse{State: state}
			res.Read(ctx, resource.ReadRequest{State: state}, &response)
			wantReads := int32(1)
			if tc.wantError {
				wantReads = 0
			}
			if response.Diagnostics.HasError() != tc.wantError || operationReads.Load() != tc.wantOpReads || resourceReads.Load() != wantReads || mutations.Load() != 0 {
				t.Fatalf("successor recovery: diagnostics=%v operation GET=%d resource GET=%d mutations=%d", response.Diagnostics, operationReads.Load(), resourceReads.Load(), mutations.Load())
			}
			history, err := data.store.Read(ctx, target)
			if err != nil || len(history) != 2 || history[0].ID != prior.ID || history[1].ID != latest.ID || history[1].Status != tc.wantStatus {
				t.Fatalf("successor history: count=%d error=%v", len(history), err)
			}
			if tc.wantError {
				return
			}
			var generation types.Int64
			if diags := response.State.GetAttribute(ctx, path.Root("generation"), &generation); diags.HasError() || generation.ValueInt64() != latestGeneration {
				t.Fatalf("successor generation: %v diagnostics=%v", generation, diags)
			}
			if tc.failed && len(response.Diagnostics.Warnings()) == 0 {
				t.Fatal("failed successor must remain visible as a warning")
			}
			repeated := resource.ReadResponse{State: state}
			res.Read(ctx, resource.ReadRequest{State: state}, &repeated)
			if repeated.Diagnostics.HasError() || operationReads.Load() != tc.wantOpReads || resourceReads.Load() != 2 || mutations.Load() != 0 || tc.failed && len(repeated.Diagnostics.Warnings()) == 0 {
				t.Fatalf("repeated predecessor state: diagnostics=%v operation GET=%d resource GET=%d mutations=%d", repeated.Diagnostics, operationReads.Load(), resourceReads.Load(), mutations.Load())
			}
		})
	}
}

func TestV2ReadRefusesAnotherIncarnationAfterStateLoss(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		http.NotFound(w, req)
	}))
	defer server.Close()
	data := v2TestData(t, server)
	res, schemaResponse := v2TestResource(t, data, hostv2TestModuleWorkerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	target := intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "app"}
	_, err := data.store.BindImportedResource(ctx, intentv2.ImportRequest{Target: target, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Evidence: intentv2.ImportEvidence{HostAPI: data.hostAPIRoot, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 1}})
	if err != nil {
		t.Fatal(err)
	}
	old, err := data.store.Begin(ctx, intentv2.BeginRequest{Target: target, Action: intentv2.ActionDelete, Form: hostv2TestModuleWorkerURL, Method: "DELETE", Path: "/resources/uid-1", UID: "uid-1", Generation: "1", AdvertisedReplayWindowSecs: 3600})
	if err != nil {
		t.Fatal(err)
	}
	retain := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	oldAck := intentv2.OperationAck{ID: "op-delete", ResourceUID: "uid-1", Action: intentv2.ActionDelete, Generation: 2, Status: "succeeded", Effect: "complete", RetainUntil: retain}
	if _, err = data.store.RecordAccepted(ctx, old.ID, oldAck); err != nil {
		t.Fatal(err)
	}
	if _, err = data.store.Complete(ctx, old.ID, intentv2.CompletionEvidence{Operation: oldAck, Resource: intentv2.ResourceObservation{Path: "/resources/uid-1", StatusCode: 404}}); err != nil {
		t.Fatal(err)
	}
	public := []byte(`{"form":"` + hostv2TestModuleWorkerURL + `","space":"prod","name":"app","spec":{}}`)
	newer, err := data.store.Begin(ctx, intentv2.BeginRequest{Target: target, Action: intentv2.ActionCreate, Form: hostv2TestModuleWorkerURL, Method: "POST", Path: "/resources", PublicJSON: public, AdvertisedReplayWindowSecs: 3600})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = data.store.RecordAccepted(ctx, newer.ID, intentv2.OperationAck{ID: "op-new", ResourceUID: "uid-2", Action: intentv2.ActionCreate, Generation: 1, Status: "running", Effect: "none", RetainUntil: retain}); err != nil {
		t.Fatal(err)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v2RawState(t, schemaResponse)}
	for key, value := range map[string]string{"name": "app", "space": "prod", "uid": "uid-1", "form_url": hostv2TestModuleWorkerURL, "host_api_root": data.hostAPIRoot, "store_id": data.storeID, "phase": "idle"} {
		if diags := state.SetAttribute(ctx, path.Root(key), types.StringValue(value)); diags.HasError() {
			t.Fatal(diags)
		}
	}
	if diags := state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(1)); diags.HasError() {
		t.Fatal(diags)
	}
	response := resource.ReadResponse{State: state}
	res.Read(ctx, resource.ReadRequest{State: state}, &response)
	if !response.Diagnostics.HasError() || requests.Load() != 0 || response.State.Raw.IsNull() {
		t.Fatalf("old incarnation was not refused: diagnostics=%v requests=%d", response.Diagnostics, requests.Load())
	}
	history, err := data.store.Read(ctx, target)
	if err != nil || len(history) != 2 || history[0].Status != intentv2.StatusDeleteConfirmed || history[1].ID != newer.ID || history[1].Status != intentv2.StatusAccepted {
		t.Fatalf("incarnation history changed: count=%d error=%v", len(history), err)
	}
}
