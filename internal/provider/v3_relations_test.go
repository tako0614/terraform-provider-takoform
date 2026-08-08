package provider

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// TestV3EveryReferenceCarriesTheExactGroup proves the provider sends the full
// three-member reference for every reference shape it can render — a top-level
// reference, a nested object-list member, and a typed binding element — while
// the HCL surface stays the bare target name.
func TestV3EveryReferenceCarriesTheExactGroup(t *testing.T) {
	ctx := context.Background()
	group := edgeformcatalog.Family.APIVersion()

	deployment, _ := edgeformcatalog.ByKind("WorkerDeployment")
	var versions model.Field
	for _, field := range deployment.Fields {
		if field.Wire == "versions" {
			versions = field
		}
	}
	if versions.Wire == "" {
		t.Fatal("WorkerDeployment declares no versions field")
	}
	elementType := v3ObjectListType(versions)
	list := types.ListValueMust(elementType, []attr.Value{
		types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"worker_version": types.StringValue("worker-version"),
			"weight":         types.Int64Value(10000),
		}),
	})
	wire, diags := v3FieldToWire(ctx, group, versions, "versions", list)
	if diags.HasError() {
		t.Fatalf("versions projection: %v", diags)
	}
	entries, _ := wire.([]any)
	if len(entries) != 1 {
		t.Fatalf("versions wire = %#v", wire)
	}
	entry, _ := entries[0].(map[string]any)
	want := map[string]any{"apiVersion": group, "kind": "WorkerVersion", "name": "worker-version"}
	if !reflect.DeepEqual(entry["workerVersion"], want) {
		t.Fatalf("nested reference = %#v, want %#v", entry["workerVersion"], want)
	}

	// The same rule for the top-level reference of the same Form.
	var worker model.Field
	for _, field := range deployment.Fields {
		if field.Wire == "worker" {
			worker = field
		}
	}
	workerWire, diags := v3FieldToWire(ctx, group, worker, "worker", types.StringValue("module-worker"))
	if diags.HasError() {
		t.Fatalf("worker projection: %v", diags)
	}
	if !reflect.DeepEqual(workerWire, map[string]any{
		"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker",
	}) {
		t.Fatalf("top-level reference = %#v", workerWire)
	}
}

// TestV3RelationConditionSurfacesADiagnostic proves the read-side contract: a
// resource whose stored relation no longer resolves fails the read with a clear
// diagnostic instead of converging on state that claims everything is fine.
func TestV3RelationConditionSurfacesADiagnostic(t *testing.T) {
	cases := []struct {
		name       string
		condition  clientv3.Condition
		wantError  bool
		wantDetail string
	}{
		{
			name: "ready",
			condition: clientv3.Condition{
				Type: "Ready", Status: "True", Reason: "Available",
			},
		},
		{
			name: "unrelated not-ready reason",
			condition: clientv3.Condition{
				Type: "Ready", Status: "False", Reason: "Provisioning",
			},
		},
		{
			name: "target incarnation changed",
			condition: clientv3.Condition{
				Type: "Ready", Status: "False", Reason: "ExternalChange",
				HostReason: "relation /kvBindings/0/resource changed incarnation from uid uid-1 to uid uid-2",
			},
			wantError:  true,
			wantDetail: "uid-2",
		},
		{
			name: "target gone",
			condition: clientv3.Condition{
				Type: "Ready", Status: "False", Reason: "DependencyMissing",
				HostReason: "relation /worker target no longer exists",
			},
			wantError:  true,
			wantDetail: "no longer exists",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var diags diag.Diagnostics
			res := &clientv3.Resource{
				Status: &clientv3.Status{Conditions: []clientv3.Condition{testCase.condition}},
			}
			v3ReportRelationCondition("WorkerVersion", "conformance", "worker-version", res, &diags)
			if diags.HasError() != testCase.wantError {
				t.Fatalf("diagnostics = %v, wantError = %v", diags, testCase.wantError)
			}
			if !testCase.wantError {
				return
			}
			detail := diags.Errors()[0].Detail()
			if !strings.Contains(detail, testCase.wantDetail) || !strings.Contains(detail, "Re-apply") {
				t.Fatalf("diagnostic detail = %q", detail)
			}
		})
	}
}
