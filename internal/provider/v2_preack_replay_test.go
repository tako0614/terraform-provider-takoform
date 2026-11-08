package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// A current credential and a valid journal window are not an authenticated
// original-principal witness. This is a negative local seam, not a Host replay.
func TestV2PreACKReadHoldsWithoutOriginalPrincipalCustody(t *testing.T) {
	for _, replaceClient := range []bool{false, true} {
		name := "same configured client"
		if replaceClient {
			name = "different configured client"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				http.Error(w, "unexpected request", http.StatusServiceUnavailable)
			}))
			defer server.Close()
			data := v2TestData(t, server)
			target := intentv2.Target{HostAPI: data.hostAPIRoot, PrincipalScope: data.storeID, Space: "prod", Name: "app"}
			_, err := data.store.BindImportedResource(ctx, intentv2.ImportRequest{
				Target: target, UID: "uid-original", Form: hostv2TestModuleWorkerURL,
				Evidence: intentv2.ImportEvidence{HostAPI: data.hostAPIRoot, Path: "/resources/uid-original", StatusCode: 200,
					UID: "uid-original", Form: hostv2TestModuleWorkerURL, Space: "prod", Name: "app", Generation: 1},
			})
			if err != nil {
				t.Fatal("synthetic import custody setup failed")
			}
			claim, err := data.store.Begin(ctx, intentv2.BeginRequest{
				Target: target, Action: intentv2.ActionUpdate, Form: hostv2TestModuleWorkerURL,
				Method: http.MethodPut, Path: "/resources/uid-original", UID: "uid-original", Generation: "1",
				PublicJSON: []byte(`{"spec":{}}`), AdvertisedReplayWindowSecs: 3600,
			})
			if err != nil || claim.FirstSentAt.IsZero() || time.Since(claim.FirstSentAt) < 0 || time.Since(claim.FirstSentAt) >= time.Hour {
				t.Fatal("synthetic current-window claim setup failed")
			}
			before, err := data.store.Read(ctx, target)
			if err != nil {
				t.Fatal("synthetic journal read failed")
			}
			res, schema := v2TestResource(t, data, hostv2TestModuleWorkerURL)
			state := tfsdk.State{Schema: schema.Schema, Raw: v2RawState(t, schema)}
			for key, value := range map[string]string{
				"name": "app", "space": "prod", "uid": "uid-original", "form_url": hostv2TestModuleWorkerURL,
				"host_api_root": data.hostAPIRoot, "store_id": data.storeID, "phase": "idle",
			} {
				if state.SetAttribute(ctx, path.Root(key), types.StringValue(value)).HasError() {
					t.Fatal("synthetic state setup failed")
				}
			}
			if state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(1)).HasError() {
				t.Fatal("synthetic generation setup failed")
			}
			if replaceClient {
				data.client = hostv2.New(server.URL, "other-synthetic-only", server.Client())
			}
			response := resource.ReadResponse{State: state}
			res.Read(ctx, resource.ReadRequest{State: state}, &response)
			if !response.Diagnostics.HasError() || len(response.Diagnostics) != 1 || response.Diagnostics[0].Summary() != "Unknown pre-ACK v2 outcome" {
				t.Fatal("expected the original pre-ACK HOLD")
			}
			after, err := data.store.Read(ctx, target)
			if err != nil || !reflect.DeepEqual(before, after) || !response.State.Raw.Equal(state.Raw) || calls.Load() != 0 {
				t.Fatal("HOLD must preserve the original state/journal and make zero Host requests")
			}
		})
	}
}
