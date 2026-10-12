package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
	"github.com/tako0614/terraform-provider-takoform/internal/privateinputsv2"
)

var _ resource.Resource = (*v2Resource)(nil)
var _ resource.ResourceWithConfigure = (*v2Resource)(nil)
var _ resource.ResourceWithImportState = (*v2Resource)(nil)
var _ resource.ResourceWithUpgradeState = (*v2Resource)(nil)

type v2Resource struct {
	def  v2FormDef
	data *v2ProviderData
}

func (r *v2Resource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.def.typeName
}
func (r *v2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*v2ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Invalid v2 provider data", "The resource received another provider lane")
		return
	}
	r.data = data
}

// Historical v0/v1 states have no exact Host API v2 identity or durable intent
// custody. The upgrader must refuse them, never infer a Form or contact a Host.
func (r *v2Resource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	refuse := func(_ context.Context, _ resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
		resp.Diagnostics.AddError("Historical state cannot enter Host API v2", "State schema v0/v1 lacks the exact v2 Host root, Form URL, and intent custody. Keep it with the released v1 Provider or perform an explicit operator-controlled migration; no Host request was made.")
	}
	return map[int64]resource.StateUpgrader{0: {StateUpgrader: refuse}, 1: {StateUpgrader: refuse}}
}

func (r *v2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.data == nil || r.data.store == nil {
		resp.Diagnostics.AddError("v2 provider not configured", "Configure an initialized intent store first")
		return
	}
	if !v2IDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid import UID", "Import ID must be one exact Host Resource UID")
		return
	}
	if r.data.space == "" {
		resp.Diagnostics.AddError("Import requires Space", "Configure a default exact space to import by UID")
		return
	}
	for name, value := range map[string]string{
		"uid": req.ID, "space": r.data.space, "form_url": r.def.form, "host_api_root": r.data.hostAPIRoot, "store_id": r.data.storeID,
	} {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), types.StringValue(value))...)
	}
}

type v2Identity struct {
	name, space, uid, form, root, storeID, intentID, operationID, phase string
	generation                                                          int64
	importing                                                           bool
}

func v2GetString(ctx context.Context, state tfsdk.State, name string) (types.String, diag.Diagnostics) {
	var value types.String
	diags := state.GetAttribute(ctx, path.Root(name), &value)
	return value, diags
}

func (r *v2Resource) identity(ctx context.Context, state tfsdk.State, allowImport bool) (v2Identity, error) {
	if r.data == nil || r.data.store == nil {
		return v2Identity{}, errors.New("v2 provider not configured")
	}
	identity := v2Identity{}
	fields := map[string]*string{"name": &identity.name, "space": &identity.space, "uid": &identity.uid,
		"form_url": &identity.form, "host_api_root": &identity.root, "store_id": &identity.storeID,
		"intent_id": &identity.intentID, "operation_id": &identity.operationID, "phase": &identity.phase}
	for key, target := range fields {
		value, diags := v2GetString(ctx, state, key)
		if diags.HasError() || value.IsUnknown() {
			return v2Identity{}, fmt.Errorf("v2 state %s is malformed", key)
		}
		if !value.IsNull() {
			*target = value.ValueString()
		}
	}
	var generation types.Int64
	if diags := state.GetAttribute(ctx, path.Root("generation"), &generation); diags.HasError() || generation.IsUnknown() {
		return v2Identity{}, errors.New("v2 state generation is malformed")
	}
	if !generation.IsNull() {
		identity.generation = generation.ValueInt64()
	}
	identity.importing = allowImport && identity.uid != "" && identity.name == "" && identity.generation == 0
	if identity.form != r.def.form || identity.root != r.data.hostAPIRoot || identity.storeID != r.data.storeID ||
		!v2IDPattern.MatchString(identity.uid) || !v2IDPattern.MatchString(identity.space) ||
		(identity.name != "" && !v2IDPattern.MatchString(identity.name)) ||
		(!identity.importing && (!v2IDPattern.MatchString(identity.name) || identity.generation < 1 || identity.generation > 9007199254740991)) ||
		(identity.intentID == "") != (identity.operationID == "") {
		return v2Identity{}, errors.New("v2 state identity, generation, or linkage does not match this exact Provider/Host/Form/store; no Host request was made")
	}
	if identity.intentID != "" && !v2KeyPattern.MatchString(identity.intentID) {
		return v2Identity{}, errors.New("v2 state intent linkage is malformed")
	}
	if identity.operationID != "" && !v2IDPattern.MatchString(identity.operationID) {
		return v2Identity{}, errors.New("v2 state Operation linkage is malformed")
	}
	return identity, nil
}

var v2KeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{15,127}$`)

func (r *v2Resource) target(space, name string) intentv2.Target {
	return intentv2.Target{HostAPI: r.data.hostAPIRoot, PrincipalScope: r.data.storeID, Space: space, Name: name}
}

func (r *v2Resource) discover(ctx context.Context) (hostv2.Discovery, error) {
	discoveryCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()
	discovery, err := r.data.client.Discover(discoveryCtx)
	if err != nil {
		return hostv2.Discovery{}, errors.New("Host API v2 discovery failed; no v1 fallback or mutation was attempted")
	}
	if discovery.BaseURL != r.data.hostAPIRoot {
		return hostv2.Discovery{}, errors.New("discovered Host API root differs from the initialized store binding; no mutation was attempted")
	}
	return discovery, nil
}

func (r *v2Resource) planNameSpace(ctx context.Context, plan tfsdk.Plan) (string, string, error) {
	var name, space types.String
	if plan.GetAttribute(ctx, path.Root("name"), &name).HasError() || name.IsNull() || name.IsUnknown() || !v2IDPattern.MatchString(name.ValueString()) {
		return "", "", errors.New("name must be a known Host API v2 ID")
	}
	if plan.GetAttribute(ctx, path.Root("space"), &space).HasError() || space.IsUnknown() {
		return "", "", errors.New("space must be known")
	}
	spaceID := r.data.space
	if !space.IsNull() && space.ValueString() != "" {
		spaceID = space.ValueString()
	}
	if !v2IDPattern.MatchString(spaceID) {
		return "", "", errors.New("space must be a known Host API v2 ID")
	}
	return name.ValueString(), spaceID, nil
}

func (r *v2Resource) offering(ctx context.Context, plan tfsdk.Plan, discovery hostv2.Discovery) (*hostv2.OfferingSelection, error) {
	var id, rev types.String
	if plan.GetAttribute(ctx, path.Root("offering_id"), &id).HasError() || plan.GetAttribute(ctx, path.Root("offering_revision"), &rev).HasError() || id.IsUnknown() || rev.IsUnknown() {
		return nil, errors.New("Offering selection must be known")
	}
	if !discovery.Capabilities.Offerings {
		if !id.IsNull() || !rev.IsNull() {
			return nil, errors.New("Host does not advertise Offerings; omit both offering fields")
		}
		return nil, nil
	}
	if id.IsNull() || rev.IsNull() || !v2IDPattern.MatchString(id.ValueString()) || rev.ValueString() == "" {
		return nil, errors.New("Host advertises Offerings; both exact offering_id and offering_revision are required")
	}
	return &hostv2.OfferingSelection{ID: id.ValueString(), Revision: rev.ValueString()}, nil
}

func (r *v2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.data == nil {
		resp.Diagnostics.AddError("v2 provider not configured", "Configure the intent store first")
		return
	}
	name, space, err := r.planNameSpace(ctx, req.Plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid v2 plan", err.Error())
		return
	}
	spec, err := v2SpecFromPlan(ctx, req.Plan, r.def)
	if err != nil {
		resp.Diagnostics.AddError("Invalid exact Form spec", err.Error())
		return
	}
	target := r.target(space, name)
	history, err := r.data.store.Read(ctx, target)
	if err != nil {
		resp.Diagnostics.AddError("v2 intent store unavailable", "No mutation was sent")
		return
	}
	if len(history) > 0 && (history[len(history)-1].Status == intentv2.StatusClaimed || history[len(history)-1].Status == intentv2.StatusAccepted || history[len(history)-1].Status == intentv2.StatusAcceptedFailed) {
		latest := history[len(history)-1]
		if latest.OperationID == "" {
			resp.Diagnostics.AddError("Unknown pre-ACK v2 outcome", "An original intent was durably claimed but no Operation ID is known. Do not create a new key, replay, or adopt by name. Reconcile with the Host/operator using the original principal and replay-window evidence.")
			return
		}
		if latest.Form != r.def.form || latest.Action != intentv2.ActionCreate {
			resp.Diagnostics.AddError("v2 name slot is occupied", "The durable store binds this Space/name to another Form or action; no mutation was sent")
			return
		}
		if _, err := r.discover(ctx); err != nil {
			resp.Diagnostics.AddError("v2 discovery refused", err.Error())
			return
		}
		r.finish(ctx, latest, req.Plan, &resp.State, &resp.Diagnostics, 0)
		return
	}
	discovery, err := r.discover(ctx)
	if err != nil {
		resp.Diagnostics.AddError("v2 discovery refused", err.Error())
		return
	}
	support, err := r.data.client.GetSupport(ctx, r.def.form)
	if err != nil || !support.Supported || !v2HasCRUD(support.Operations) {
		resp.Diagnostics.AddError("Exact Form is not supported", "The Host must declare complete CRUD support for this exact Form URL; no mutation was sent")
		return
	}
	offering, err := r.offering(ctx, req.Plan, discovery)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Offering selection", err.Error())
		return
	}
	private, names, err := r.privateInputs(space, name, spec, discovery, support)
	if err != nil {
		resp.Diagnostics.AddError("Private inputs unavailable", err.Error())
		return
	}
	public := hostv2.CreateRequest{Form: r.def.form, Space: space, Name: name, Offering: offering, Spec: spec}
	publicJSON, err := jsonv2.Marshal(public)
	if err != nil {
		resp.Diagnostics.AddError("Invalid public request", "The exact Form request cannot be encoded")
		return
	}
	intent, err := r.data.store.Begin(ctx, intentv2.BeginRequest{Target: target, Action: intentv2.ActionCreate, Form: r.def.form, Method: "POST", Path: "/resources", PublicJSON: publicJSON, PrivateInputNames: names, AdvertisedReplayWindowSecs: discovery.Limits.ReplayWindowSeconds})
	if err != nil {
		resp.Diagnostics.AddError("v2 name slot or intent conflict", "The durable store refused a new Create; no mutation was sent. Inspect unresolved intent, import, and same-name reservations.")
		return
	}
	public.PrivateInputs = private
	op, err := r.data.client.CreateResource(ctx, intent.Key, public)
	if err != nil {
		resp.Diagnostics.AddError("Unknown v2 Create outcome", "The original key and request are held in the store, but no Operation is durably acknowledged. Stop and reconcile with the Host/operator; no automatic resend or new key is safe.")
		return
	}
	intent, err = r.data.store.RecordAccepted(ctx, intent.ID, v2Ack(op))
	if err != nil {
		resp.Diagnostics.AddError("Accepted v2 Create could not be journaled", "The Host may own a Resource. Preserve the store and reconcile the exact Operation; do not create again.")
		return
	}
	resp.State = tfsdk.State{Schema: req.Plan.Schema, Raw: req.Plan.Raw}
	r.writePending(ctx, &resp.State, space, name, op, intent.ID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.finish(ctx, intent, req.Plan, &resp.State, &resp.Diagnostics, op.RetryAfterSeconds)
}

func v2HasCRUD(operations []string) bool {
	seen := make(map[string]bool)
	for _, op := range operations {
		seen[op] = true
	}
	return seen["create"] && seen["read"] && seen["update"] && seen["delete"]
}

func (r *v2Resource) privateInputs(space, name string, spec map[string]any, discovery hostv2.Discovery, support hostv2.Support) (*map[string]string, []string, error) {
	if r.def.form != "https://edge.forms.takoform.com/forms/WorkerVersion/0.5.0/" {
		return nil, nil, nil
	}
	raw, present := spec["requiredSensitiveVars"]
	if !present {
		return nil, nil, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, nil, nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, nil, errors.New("required sensitive input names are invalid")
		}
		names = append(names, name)
	}
	if !discovery.Capabilities.PrivateInputs || !support.PrivateInputs {
		return nil, nil, errors.New("Host and exact Form must both advertise private-input support")
	}
	values, err := privateinputsv2.Load(space, name, names)
	if err != nil {
		return nil, nil, errors.New("the exact apply-time private-input map is unavailable; no mutation was sent")
	}
	return &values, names, nil
}

func v2Ack(op hostv2.Operation) intentv2.OperationAck {
	return intentv2.OperationAck{ID: op.ID, ResourceUID: op.ResourceUID, Action: intentv2.Action(op.Action), Generation: op.Generation, Status: op.Status, Effect: op.Effect, RetainUntil: op.RetainUntil}
}

func (r *v2Resource) writePending(ctx context.Context, state *tfsdk.State, space, name string, op hostv2.Operation, intentID string, diags *diag.Diagnostics) {
	for key, value := range map[string]string{"name": name, "space": space, "uid": op.ResourceUID, "form_url": r.def.form, "host_api_root": r.data.hostAPIRoot, "store_id": r.data.storeID, "intent_id": intentID, "operation_id": op.ID, "phase": "pending"} {
		diags.Append(state.SetAttribute(ctx, path.Root(key), types.StringValue(value))...)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(op.Generation))...)
}

func (r *v2Resource) writeResource(ctx context.Context, state *tfsdk.State, record hostv2.Resource, intentID, operationID string, diags *diag.Diagnostics) {
	if record.Form != r.def.form || record.Space == "" || record.Name == "" {
		diags.AddError("Wrong Host Resource identity", "Host returned a different exact Form or incomplete Resource; state was not adopted")
		return
	}
	specValue, err := v2StateSpec(ctx, r.def, record.Spec)
	if err != nil {
		diags.AddError("Invalid Host Resource spec", err.Error())
		return
	}
	for key, value := range map[string]string{"name": record.Name, "space": record.Space, "uid": record.UID, "form_url": r.def.form, "host_api_root": r.data.hostAPIRoot, "store_id": r.data.storeID, "intent_id": intentID, "operation_id": operationID, "phase": record.Phase} {
		if value == "" && (key == "intent_id" || key == "operation_id") {
			diags.Append(state.SetAttribute(ctx, path.Root(key), types.StringNull())...)
			continue
		}
		diags.Append(state.SetAttribute(ctx, path.Root(key), types.StringValue(value))...)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("generation"), types.Int64Value(record.Generation))...)
	if specValue != nil {
		diags.Append(state.SetAttribute(ctx, path.Root("spec"), specValue)...)
	}
	if record.Offering == nil {
		diags.Append(state.SetAttribute(ctx, path.Root("offering_id"), types.StringNull())...)
		diags.Append(state.SetAttribute(ctx, path.Root("offering_revision"), types.StringNull())...)
	} else {
		diags.Append(state.SetAttribute(ctx, path.Root("offering_id"), types.StringValue(record.Offering.ID))...)
		diags.Append(state.SetAttribute(ctx, path.Root("offering_revision"), types.StringValue(record.Offering.Revision))...)
	}
}

func (r *v2Resource) finish(ctx context.Context, intent intentv2.Intent, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics, initialDelay int64) {
	if initialDelay > 0 {
		if err := v2Wait(ctx, initialDelay); err != nil {
			diags.AddError("v2 Operation still pending", "Context ended before the Host retry delay; retained intent and UID must be reconciled")
			return
		}
	}
	op, err := r.data.client.ReadOperation(ctx, intent.OperationID)
	if err != nil {
		diags.AddError("Accepted v2 Operation unreadable", "The exact Operation must be reconciled under the original principal; no new mutation was sent")
		return
	}
	for op.Status != "succeeded" && op.Status != "failed" {
		if op.Status == "waiting_input" {
			// Only the original apply-time map may continue this Operation. Never
			// generate a new intent or place its values in state/journal.
			if intent.Action == intentv2.ActionDelete || len(intent.PrivateInputNames) == 0 {
				diags.AddError("v2 Operation waits for private inputs", "The original map is unavailable; continue this exact Operation with operator assistance")
				return
			}
			values, err := privateinputsv2.Load(intent.Target.Space, intent.Target.Name, intent.PrivateInputNames)
			if err != nil {
				diags.AddError("v2 original private inputs unavailable", "Continue the exact Operation only after the original apply-time map is present")
				return
			}
			if _, err := r.data.client.ContinuePrivateInputs(ctx, op, values); err != nil {
				diags.AddError("v2 private-input continuation uncertain", "Read and reconcile the same Operation; do not issue a new mutation")
				return
			}
		}
		if err := v2Wait(ctx, op.RetryAfterSeconds); err != nil {
			diags.AddError("v2 Operation still pending", "Context ended before the accepted Operation settled; retained intent and UID must be reconciled")
			return
		}
		op, err = r.data.client.ReadOperation(ctx, intent.OperationID)
		if err != nil {
			diags.AddError("Accepted v2 Operation unreadable", "Read and reconcile the exact Operation; no new mutation was sent")
			return
		}
	}
	var record hostv2.Resource
	observation := intentv2.ResourceObservation{Path: "/resources/" + op.ResourceUID}
	if intent.Action == intentv2.ActionDelete && op.Status == "succeeded" {
		_, err = r.data.client.ReadResource(ctx, op.ResourceUID)
		var apiErr *hostv2.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			diags.AddError("v2 Delete absence unproven", "The Host must return exact UID 404 before the name slot and state can be released")
			return
		}
		observation.StatusCode = 404
	} else {
		record, err = r.data.client.ReadResource(ctx, op.ResourceUID)
		if err != nil || record.Form != r.def.form || record.Space != intent.Target.Space || record.Name != intent.Target.Name || record.Generation != op.Generation {
			diags.AddError("v2 Resource evidence mismatch", "Exact UID, Form, Space, name, and generation must match the accepted Operation; state was not adopted")
			return
		}
		observation = intentv2.ResourceObservation{Present: true, Path: "/resources/" + record.UID, StatusCode: 200, UID: record.UID, Form: record.Form, Space: record.Space, Name: record.Name, Generation: record.Generation, Phase: record.Phase}
	}
	_, err = r.data.store.Complete(ctx, intent.ID, intentv2.CompletionEvidence{Operation: v2Ack(op), Resource: observation})
	if err != nil {
		diags.AddError("v2 completion custody refused", "The durable store rejected the exact Operation/Resource evidence; state was not advanced")
		return
	}
	if intent.Action == intentv2.ActionDelete && op.Status == "succeeded" {
		state.RemoveResource(ctx)
		return
	}
	if op.Status == "failed" {
		r.writeResource(ctx, state, record, intent.ID, op.ID, diags)
		diags.AddError("v2 Operation failed with retained UID", "The Host retains this UID/name and any known partial effects. Use an explicit update or delete on this same UID; no new Create is safe.")
		return
	}
	r.writeResource(ctx, state, record, "", "", diags)
}

func v2Wait(ctx context.Context, seconds int64) error {
	if seconds < 1 {
		seconds = 1
	}
	maxSeconds := int64((1<<63 - 1) / int64(time.Second))
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
