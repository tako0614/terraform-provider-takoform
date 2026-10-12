package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"github.com/tako0614/terraform-provider-takoform/internal/edgev2"
)

const v2SchemaVersion int64 = 2

var v2IDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type v2FormDef struct {
	typeName, form string
	spec           map[string]schema.Attribute
}

func v2String(required bool) schema.StringAttribute {
	return schema.StringAttribute{Required: required, Optional: !required}
}
func v2Int(required bool) schema.Int64Attribute {
	return schema.Int64Attribute{Required: required, Optional: !required}
}
func v2Bool(required bool) schema.BoolAttribute {
	return schema.BoolAttribute{Required: required, Optional: !required}
}
func v2Ref() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Required: true, Attributes: map[string]schema.Attribute{"resource_uid": v2String(true)}}
}
func v2OptionalRef() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Optional: true, Attributes: map[string]schema.Attribute{"resource_uid": v2String(true)}}
}
func v2Artifact() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Required: true, Attributes: map[string]schema.Attribute{"url": v2String(true), "sha256": v2String(true)}}
}
func v2Binding() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{"name": v2String(true), "resource": v2Ref()}}}
}

var v2Forms = []v2FormDef{
	{"takoform_module_worker", edgev2.ModuleWorkerURL, nil},
	{"takoform_worker_bundle", edgev2.WorkerBundleURL, map[string]schema.Attribute{"artifact": v2Artifact()}},
	{"takoform_worker_version", edgev2.WorkerVersionURL, map[string]schema.Attribute{
		"worker": v2Ref(), "bundle": v2OptionalRef(),
		"handlers":                schema.ListAttribute{Required: true, ElementType: schema.StringAttribute{}.GetType()},
		"vars_json":               v2String(false),
		"required_sensitive_vars": schema.ListAttribute{Optional: true, ElementType: schema.StringAttribute{}.GetType()},
		"kv_bindings":             v2Binding(), "sqlite_bindings": v2Binding(), "bucket_bindings": v2Binding(),
		"queue_producer_bindings": v2Binding(), "service_bindings": v2Binding(), "actor_bindings": v2Binding(), "workflow_bindings": v2Binding(),
		"assets": schema.SingleNestedAttribute{Optional: true, Attributes: map[string]schema.Attribute{
			"bundle": v2Ref(), "run_worker_first": v2Bool(true), "not_found_handling": v2String(true),
		}},
	}},
	{"takoform_worker_deployment", edgev2.WorkerDeploymentURL, map[string]schema.Attribute{
		"worker": v2Ref(), "versions": schema.ListNestedAttribute{Required: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"worker_version": v2Ref(), "weight": v2Int(true),
		}}},
	}},
	{"takoform_worker_endpoint", edgev2.WorkerEndpointURL, map[string]schema.Attribute{"worker": v2Ref()}},
	{"takoform_worker_cron_trigger", edgev2.WorkerCronTriggerURL, map[string]schema.Attribute{"worker": v2Ref(), "cron": v2String(true)}},
	{"takoform_edge_kv_namespace", edgev2.EdgeKVNamespaceURL, nil},
	{"takoform_edge_object_bucket", edgev2.ObjectBucketURL, nil},
	{"takoform_sqlite_database", edgev2.SQLiteDatabaseURL, nil},
	{"takoform_sqlite_migration_set", edgev2.SQLiteMigrationSetURL, map[string]schema.Attribute{"artifact": v2Artifact()}},
	{"takoform_sqlite_migration_application", edgev2.SQLiteMigrationApplicationURL, map[string]schema.Attribute{"database": v2Ref(), "migration_set": v2Ref()}},
	{"takoform_at_least_once_queue", edgev2.AtLeastOnceQueueURL, map[string]schema.Attribute{"delivery_delay_seconds": v2Int(false), "message_retention_seconds": v2Int(true)}},
	{"takoform_queue_consumer", edgev2.QueueConsumerURL, map[string]schema.Attribute{
		"queue": v2Ref(), "worker": v2Ref(), "max_batch_size": v2Int(true), "max_batch_timeout_seconds": v2Int(true),
		"max_concurrency": v2Int(true), "max_retries": v2Int(true), "retry_delay_seconds": v2Int(true), "dead_letter_queue": v2OptionalRef(),
	}},
}

func newV2Resources() []func() resource.Resource {
	out := make([]func() resource.Resource, 0, len(v2Forms))
	for _, def := range v2Forms {
		def := def
		out = append(out, func() resource.Resource { return &v2Resource{def: def} })
	}
	return out
}

func (r *v2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"name":              schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"space":             schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}},
		"uid":               schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"form_url":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"host_api_root":     schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"store_id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"generation":        schema.Int64Attribute{Computed: true},
		"intent_id":         schema.StringAttribute{Computed: true},
		"operation_id":      schema.StringAttribute{Computed: true},
		"phase":             schema.StringAttribute{Computed: true},
		"offering_id":       schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"offering_revision": schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
	}
	if r.def.spec != nil {
		attrs["spec"] = schema.SingleNestedAttribute{Required: true, Attributes: r.def.spec, Description: "Typed public desired input of this exact Form; private values are not allowed."}
	}
	resp.Schema = schema.Schema{Version: v2SchemaVersion, Description: "Exact Host API v2 Form " + r.def.form, Attributes: attrs}
}
