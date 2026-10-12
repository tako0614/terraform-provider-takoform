package provider

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
	"github.com/tako0614/terraform-provider-takoform/internal/privateinputsv2"
)

func (r *v2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	identity, err := r.identity(ctx, req.State, true)
	if err != nil {
		resp.Diagnostics.AddError("v2 state refused before Host access", err.Error())
		return
	}
	if identity.importing {
		if _, err := r.discover(ctx); err != nil {
			resp.Diagnostics.AddError("v2 discovery refused", err.Error())
			return
		}
		record, err := r.data.client.ReadResource(ctx, identity.uid)
		if err != nil || record.UID != identity.uid || record.Form != r.def.form || record.Space != identity.space || record.Generation < 1 {
			resp.Diagnostics.AddError("v2 import evidence refused", "An authorized exact-UID 200 Resource with this Form and Space is required; no Create or adoption-by-name occurred")
			return
		}
		target := r.target(record.Space, record.Name)
		_, err = r.data.store.BindImportedResource(ctx, intentv2.ImportRequest{Target: target, UID: identity.uid, Form: r.def.form, Evidence: intentv2.ImportEvidence{
			HostAPI: r.data.hostAPIRoot, Path: "/resources/" + identity.uid, StatusCode: http.StatusOK,
			UID: record.UID, Form: record.Form, Space: record.Space, Name: record.Name, Generation: record.Generation,
		}})
		if err != nil {
			resp.Diagnostics.AddError("v2 import custody refused", "The durable store rejected this exact UID/Form/Space/name binding; state was not adopted")
			return
		}
		resp.State = req.State
		r.writeResource(ctx, &resp.State, record, "", "", &resp.Diagnostics)
		return
	}
	target := r.target(identity.space, identity.name)
	history, err := r.data.store.Read(ctx, target)
	if err != nil {
		resp.Diagnostics.AddError("v2 intent store unavailable", "No Host request was made")
		return
	}
	imported, hasImport, err := r.data.store.ReadImportBinding(ctx, target)
	if err != nil {
		resp.Diagnostics.AddError("v2 import custody unavailable", "No Host request was made")
		return
	}
	if len(history) == 0 && (!hasImport || imported.UID != identity.uid || imported.Form != r.def.form) {
		resp.Diagnostics.AddError("v2 state lacks local custody", "This UID is not backed by an exact stored intent or explicit import; no Host request was made")
		return
	}
	if identity.intentID != "" {
		var pending *intentv2.Intent
		for i := range history {
			if history[i].ID == identity.intentID {
				pending = &history[i]
				break
			}
		}
		if pending == nil || pending.OperationID != identity.operationID || pending.OperationUID != identity.uid {
			resp.Diagnostics.AddError("v2 state Operation linkage mismatch", "The state and durable store disagree; no Host request was made")
			return
		}
		if _, err := r.discover(ctx); err != nil {
			resp.Diagnostics.AddError("v2 discovery refused", err.Error())
			return
		}
		resp.State = req.State
		r.finish(ctx, *pending, tfsdk.Plan{}, &resp.State, &resp.Diagnostics, 0)
		return
	}
	if _, err := r.discover(ctx); err != nil {
		resp.Diagnostics.AddError("v2 discovery refused", err.Error())
		return
	}
	record, err := r.data.client.ReadResource(ctx, identity.uid)
	if err != nil || record.UID != identity.uid || record.Form != r.def.form || record.Space != identity.space || record.Name != identity.name || record.Generation < identity.generation {
		resp.Diagnostics.AddError("v2 Resource evidence mismatch", "Exact UID, Form, Space, name, and non-regressing generation are required. A 404/410 does not prove backend effects absent; no state was cleared")
		return
	}
	resp.State = req.State
	r.writeResource(ctx, &resp.State, record, "", "", &resp.Diagnostics)
}

func (r *v2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.mutateExisting(ctx, intentv2.ActionUpdate, req.State, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *v2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.mutateExisting(ctx, intentv2.ActionDelete, req.State, tfsdk.Plan{}, &resp.State, &resp.Diagnostics)
}

func (r *v2Resource) mutateExisting(ctx context.Context, action intentv2.Action, old tfsdk.State, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	identity, err := r.identity(ctx, old, false)
	if err != nil {
		diags.AddError("v2 state refused before Host access", err.Error())
		return
	}
	target := r.target(identity.space, identity.name)
	history, err := r.data.store.Read(ctx, target)
	if err != nil {
		diags.AddError("v2 intent store unavailable", "No Host request was made")
		return
	}
	imported, hasImport, err := r.data.store.ReadImportBinding(ctx, target)
	if err != nil || len(history) == 0 && (!hasImport || imported.UID != identity.uid || imported.Form != r.def.form) {
		diags.AddError("v2 state lacks local custody", "The exact UID is not a locally retained intent or import; no Host request was made")
		return
	}
	if len(history) > 0 {
		latest := history[len(history)-1]
		if latest.Status == intentv2.StatusClaimed && latest.OperationID == "" {
			diags.AddError("Unknown pre-ACK v2 outcome", "The durable store has an unknown claim. Reconcile the original principal/window/request with the operator; no automatic replay or new key is safe")
			return
		}
		if latest.Status == intentv2.StatusAccepted || latest.Status == intentv2.StatusAcceptedFailed {
			diags.AddError("Earlier v2 Operation unresolved", "Refresh and reconcile the exact accepted Operation before another explicit mutation; no new key was minted")
			return
		}
	}
	var spec map[string]any
	var private *map[string]string
	var names []string
	if action == intentv2.ActionUpdate {
		name, space, err := r.planNameSpace(ctx, plan)
		if err != nil || name != identity.name || space != identity.space {
			diags.AddError("v2 update identity changed", "Name or Space replacement requires an explicit new Resource, not an in-place update")
			return
		}
		spec, err = v2SpecFromPlan(ctx, plan, r.def)
		if err != nil {
			diags.AddError("Invalid exact Form spec", err.Error())
			return
		}
	}
	discovery, err := r.discover(ctx)
	if err != nil {
		diags.AddError("v2 discovery refused", err.Error())
		return
	}
	record, err := r.data.client.ReadResource(ctx, identity.uid)
	if err != nil || record.UID != identity.uid || record.Form != r.def.form || record.Space != identity.space || record.Name != identity.name || record.Generation != identity.generation {
		diags.AddError("v2 generation or identity fence mismatch", "Read and reconcile the exact UID/Form/Space/name/generation; no mutation was sent")
		return
	}
	var publicJSON []byte
	method := "DELETE"
	if action == intentv2.ActionUpdate {
		method = "PUT"
		// Supplying private inputs on a new Update is explicit and must come
		// from this apply's exact process-local source. Omission preserves Host
		// values; no secret is pulled from previous state or a public spec.
		if r.def.form == "https://edge.forms.takoform.com/forms/WorkerVersion/0.5.0/" {
			private, names, err = r.privateInputsForUpdate(identity.space, identity.name, spec)
			if err != nil {
				diags.AddError("Private inputs unavailable", err.Error())
				return
			}
		}
		publicJSON, err = jsonv2.Marshal(hostv2.UpdateRequest{Spec: spec})
		if err != nil {
			diags.AddError("Invalid public update", "The exact request cannot be encoded")
			return
		}
	}
	supersedes := ""
	if len(history) > 0 && history[len(history)-1].Status == intentv2.StatusFailedReserved {
		supersedes = history[len(history)-1].ID
	}
	intent, err := r.data.store.Begin(ctx, intentv2.BeginRequest{
		Target: target, Action: action, Form: r.def.form, Method: method, Path: "/resources/" + identity.uid,
		UID: identity.uid, Generation: strconv.FormatInt(identity.generation, 10), SupersedesIntentID: supersedes,
		PublicJSON: publicJSON, PrivateInputNames: names, AdvertisedReplayWindowSecs: discovery.Limits.ReplayWindowSeconds,
	})
	if err != nil {
		diags.AddError("v2 intent custody refused", "The store rejected the exact UID/generation/new-intent boundary; no mutation was sent")
		return
	}
	var op hostv2.Operation
	if action == intentv2.ActionUpdate {
		op, err = r.data.client.UpdateResource(ctx, identity.uid, intent.Key, identity.generation, hostv2.UpdateRequest{Spec: spec, PrivateInputs: private})
	} else {
		op, err = r.data.client.DeleteResource(ctx, identity.uid, intent.Key, identity.generation)
	}
	if err != nil {
		diags.AddError("Unknown v2 mutation outcome", "Original key/request/window remain in the durable store, but no Operation was acknowledged. Stop; no automatic replay or new key is safe.")
		return
	}
	intent, err = r.data.store.RecordAccepted(ctx, intent.ID, v2Ack(op))
	if err != nil {
		diags.AddError("Accepted v2 mutation could not be journaled", "Preserve the exact Operation and store for operator reconciliation; no new mutation is safe")
		return
	}
	*state = old
	if action == intentv2.ActionUpdate {
		*state = tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
	}
	r.writePending(ctx, state, identity.space, identity.name, op, intent.ID, diags)
	if diags.HasError() {
		return
	}
	r.finish(ctx, intent, plan, state, diags, op.RetryAfterSeconds)
}

func (r *v2Resource) privateInputsForUpdate(space, name string, spec map[string]any) (*map[string]string, []string, error) {
	// Updates with no declared private names omit privateInputs. When names
	// are declared, this major requires an exact apply-time map; it never
	// reads or reconstructs values from state or journal.
	if _, ok := spec["requiredSensitiveVars"]; !ok {
		return nil, nil, nil
	}
	// Discovery and support were already checked for a fresh Create, but
	// their mutable advertisements are not replay authority for an Update.
	// A fresh explicit Update is checked by the Host itself.
	items, ok := spec["requiredSensitiveVars"].([]any)
	if !ok || len(items) == 0 {
		return nil, nil, nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, nil, errors.New("invalid private-input name")
		}
		names = append(names, value)
	}
	values, err := privateinputsv2.Load(space, name, names)
	if err != nil {
		return nil, nil, errors.New("exact apply-time private-input map is unavailable")
	}
	return &values, names, nil
}
