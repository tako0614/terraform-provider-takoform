package provider

// v3_resources.go derives the typed HCL surface of every registered Form from
// the Provider-owned exact projection and maps values between that surface and
// the portable stable-v1 wire spec.
//
// Surface conventions of the v3 lane:
//   - resource references (worker, bundle, queue, ...) are the target's
//     resource NAME as a plain string; the wire carries the typed
//     {"kind": <TargetKind>, "name": ...} reference object.
//   - binding lists (kv_bindings, ...) are nested lists of
//     {name, target_name}; the wire element is
//     {"name": ..., "resource": {"kind": <TargetKind>, "name": target_name}}.
//     The wire key is `resource`, never `target`.
//   - the data-only vars map is authored as `vars_json`, an Optional string
//     holding one JSON object; it is parsed and sent as the wire object.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	workerBundleKind               = "WorkerBundle"
	workerVersionKind              = "WorkerVersion"
	v3ApplyIdempotencyKeyAttribute = "apply_idempotency_key"
	staticAssetBundleKind          = "StaticAssetBundle"
	sqliteMigrationSetKind         = "SQLiteMigrationSet"
	workerDeploymentKind           = "WorkerDeployment"

	// workerDeploymentWeightSum is the exact basis-point total every
	// WorkerDeployment versions list must reach.
	workerDeploymentWeightSum = 10000
)

// v3AttributeName maps one catalog field to its HCL attribute name. The
// data-only JSON map surfaces as <field>_json because its values are
// arbitrary bounded JSON, which HCL maps of strings cannot carry faithfully.
// The rule lives in the authoring model, so the model can refuse a Form whose
// field or output would take an attribute name the envelope already owns.
func v3AttributeName(field model.Field) string { return field.AttributeName() }

func (r *v3FormResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := v3CommonAttributesForSurface(r.form, r.supportsApplyIdempotencyKey())
	artifact, artifactInjected := r.v3ArtifactRule()
	requiresArtifact := v3FormRequiresArtifactRule(r.form)
	if requiresArtifact && !artifactInjected {
		resp.Diagnostics.AddError(
			"Provider artifact projection is missing",
			fmt.Sprintf("The exact %s/%s Form requires Provider-only artifact metadata, but its constructor did not inject an exact projection rule. This is a provider bug.", r.form.Family.APIVersion(), r.form.Kind),
		)
		return
	}
	if artifactInjected && !requiresArtifact {
		resp.Diagnostics.AddError(
			"Provider artifact projection is attached to the wrong Form",
			fmt.Sprintf("The exact %s/%s Form does not declare the required manifestDigest revision shape. This is a provider bug.", r.form.Family.APIVersion(), r.form.Kind),
		)
		return
	}
	if artifactInjected && artifact.Mode == v3ArtifactModeWorkerBundle {
		for name, attribute := range workerBundleAttributesForProjection(*artifact) {
			attrs[name] = attribute
		}
	} else if artifactInjected && artifact.Mode == v3ArtifactModeFileBundle {
		for name, attribute := range fileBundleAttributesForProjection(*artifact) {
			attrs[name] = attribute
		}
	} else if artifactInjected {
		resp.Diagnostics.AddError("Unknown Provider artifact projection mode", fmt.Sprintf("The exact %s/%s Form carries unsupported artifact mode %q. This is a provider bug.", r.form.Family.APIVersion(), r.form.Kind, artifact.Mode))
		return
	} else {
		for _, field := range r.form.Fields {
			attribute, err := v3FieldAttribute(r.form, field)
			if err != nil {
				resp.Diagnostics.AddError(
					"Form field cannot be represented by the provider",
					fmt.Sprintf("The %s Form field %s cannot be compiled into a typed Terraform schema: %v", r.form.Kind, field.Wire, err),
				)
				return
			}
			attrs[v3AttributeName(field)] = attribute
		}
	}
	// Provider 3 no longer exposes ObjectBucket or bucket bindings as current
	// desired state. It must nevertheless keep the old attribute TYPE in the
	// WorkerVersion Terraform state envelope so a retained Provider 2.1.1
	// WorkerVersion exact codec can decode and refresh the state it wrote. The
	// attribute is computed-only: configurations cannot author it, current
	// codecs never send it, and no ObjectBucket resource mapping is restored.
	if r.form.Kind == workerVersionKind {
		attrs[v3RetainedBucketBindingsAttribute] = v3RetainedBucketBindingsStateAttribute()
	}
	// The Form's declared outputs become typed computed attributes. They are
	// added last and fail closed on any name already taken: an output silently
	// overwriting a desired attribute or an envelope attribute would give one
	// name two meanings, and whichever the resource wrote last would win.
	for _, output := range r.form.Outputs {
		name := v3AttributeName(output)
		if _, taken := attrs[name]; taken {
			resp.Diagnostics.AddError(
				"Form output collides with an existing attribute",
				fmt.Sprintf(
					"The %s Form declares the output %q, which this resource already carries as an attribute. "+
						"The provider refuses to serve a schema in which one name holds two facts. This is a provider bug.",
					r.form.Kind, name,
				),
			)
			return
		}
		attrs[name] = v3OutputAttribute(output)
	}
	description := r.form.Description +
		" Role: " + string(r.form.Role) + " (Host API v1beta1 lane)."
	resp.Schema = schema.Schema{
		Version:     v3SchemaVersion,
		Description: description,
		Attributes:  attrs,
	}
}

const v3RetainedBucketBindingsAttribute = "bucket_bindings"

func v3RetainedBucketBindingsStateAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed: true,
		Description: "Retained Provider 2.1.1 state only. Records the historical ObjectBucket binding list " +
			"when refreshing a WorkerVersion created under its exact v1beta1 FormRef. Provider 3 does not " +
			"accept this attribute in configuration and does not expose an ObjectBucket resource.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Historical JavaScript binding name retained in state.",
			},
			"target_name": schema.StringAttribute{
				Computed:    true,
				Description: "Historical ObjectBucket resource name retained in state.",
			},
		}},
	}
}

// v3CommonAttributes is the shared v3 state model: client-owned identity
// (name, space), host-owned identity and fences (uid, generation, revision),
// the exact Form identity, readiness, typed outputs, and operation timeouts.
// The packageDigest is deliberately NOT part of state identity; it is an
// audit-only computed attribute (spec/decisions/0011).
func v3CommonAttributes(form model.Form) map[string]schema.Attribute {
	return v3CommonAttributesForSurface(form, form.Kind == workerVersionKind)
}

func v3CommonAttributesForSurface(form model.Form, includeApplyIdempotencyKey bool) map[string]schema.Attribute {
	hasUpdate := form.DeclaresUpdate()
	attrs := map[string]schema.Attribute{
		"name": v3NameAttribute(form),
		"space": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "Exact opaque SpaceID for this resource. Overrides the provider default; changing it replaces the resource.",
			Validators:  []validator.String{StringSpaceID()},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"uid": schema.StringAttribute{
			Computed:    true,
			Description: "Host-issued immutable resource identity. Delete followed by re-create of the same name yields a new UID.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"generation": schema.StringAttribute{
			Computed:    true,
			Description: "Canonical decimal desired-state generation; increments only when the portable desired spec changes. Update fences carry it as the expected generation.",
		},
		"revision": schema.StringAttribute{
			Computed:    true,
			Description: "Canonical decimal representation revision; increments whenever the full representation changes, including when a change to another resource re-renders this one. It is the strong ETag, not the delete fence.",
		},
		"conditions": v3ConditionsAttribute(),
		"ready": schema.BoolAttribute{
			Computed: true,
			Description: "Derived convenience: true when `conditions` carries the closed Ready condition with " +
				"status True. Read `conditions` for the reason a resource is not ready.",
		},
		"outputs_json": schema.StringAttribute{
			Computed:    true,
			Description: "JSON-serialized status.outputs document of the last read or apply; \"{}\" when the Form publishes no outputs.",
		},
		"form_api_version": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact namespaced Form group/version recorded in state; reads dispatch on this exact identity.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_kind": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact Form kind recorded in state.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_definition_version": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact immutable Form definition version recorded in state.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_schema_digest": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact immutable Form schema digest recorded in state.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_package_digest": schema.StringAttribute{
			Computed: true,
			Description: "Audit-only digest of the Form Package this provider build carries for the exact FormRef. " +
				"It is distribution provenance, never resource identity, and never enters queries or fences.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"pending_operation_id":   v3PendingOperationIDAttribute(),
		v3RelationDriftAttribute: v3RelationDriftReasonAttribute(),
		"create_timeout": schema.StringAttribute{
			Optional:    true,
			Description: "Overall create deadline as a Go duration (default \"20m\").",
			Validators:  []validator.String{v3DurationValidator{}},
		},
		"delete_timeout": schema.StringAttribute{
			Optional:    true,
			Description: "Overall delete deadline as a Go duration (default \"30m\").",
			Validators:  []validator.String{v3DurationValidator{}},
		},
	}
	// Only a revision-role Form derives its name, so only a revision-role Form
	// has an owner to declare. Declaring the attribute anywhere else would offer
	// a knob that decides nothing.
	if form.Role == model.RoleRevision {
		attrs[v3RevisionOwnerAttribute] = v3RevisionOwnerSchemaAttribute(form)
	}
	// WorkerVersion alone accepts a provider-only operation-key override. It is
	// deliberately absent from every other resource and never participates in
	// the portable Form schema or wire spec.
	if includeApplyIdempotencyKey && form.Kind == workerVersionKind {
		attrs[v3ApplyIdempotencyKeyAttribute] = schema.StringAttribute{
			Optional: true,
			Computed: true,
			Description: "Provider-only Host API Idempotency-Key for this WorkerVersion apply. " +
				"Without TAKOFORM_RUNTIME_INPUTS_FILE an explicit visible-ASCII value is preserved in state, " +
				"while omission keeps the established deterministic Host key. With a declared sensitive input " +
				"file the provider computes this value from the file's generation nonce and the value-free logical " +
				"apply identity. It never includes a secret value or enters the portable desired spec. Changing it " +
				"replaces this immutable version.",
			Validators:    []validator.String{StringIdempotencyKey()},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
	}
	// A Form that declares no update capability has no update to bound: the
	// attribute is not declared at all, so a configuration that sets it fails
	// at validate time instead of silently naming a deadline nothing observes.
	if hasUpdate {
		attrs["update_timeout"] = schema.StringAttribute{
			Optional:    true,
			Description: "Overall update deadline as a Go duration (default \"20m\").",
			Validators:  []validator.String{v3DurationValidator{}},
		}
	}
	return attrs
}

// v3RevisionOwnerSchemaAttribute declares who owns a derived revision.
//
// It is provider-side authoring input, never portable desired state: no wire
// member carries it and the host never sees it. What it decides is the derived
// NAME. A content digest names the bytes, so two independent resources built
// from identical bytes derive one name — and one host address with two
// Terraform owners is an address whose destroy breaks the other owner. The
// owner is what separates them, and it cannot be inferred: the framework never
// shows a provider its own resource address, and the content says nothing about
// who declared it (spec/decisions/0029).
func v3RevisionOwnerSchemaAttribute(form model.Form) schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true,
		Description: "Stable name of whatever owns this revision — the `takoform_module_worker` it belongs to " +
			"is the usual answer. Required whenever `name` is omitted, because the derived name is a function " +
			"of this revision's content and two owners built from identical content would otherwise derive one " +
			"name and manage one host address. It is folded into the derived name as \"" +
			v3RevisionNamePrefix(form) + "-<content digest prefix>-<owner digest prefix>\", never sent to the " +
			"host, and never part of the portable desired spec. Changing it names a different revision, so it " +
			"replaces this one.",
		Validators:    []validator.String{StringToken()},
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

// v3NameAttribute declares the portable resource name.
//
// It is Required on every Form whose name is the author's own stable handle,
// and Optional+Computed on a `revision`-role Form, whose name is derived from
// the revision's content and declared owner when the author omits it
// (v3_revision_names.go). There is deliberately NO UseStateForUnknown modifier
// on the derived case: holding the prior name known across a content change is
// precisely the deadlock this derivation exists to remove, and the plan
// computes the new name itself rather than leaving it unknown.
func v3NameAttribute(form model.Form) schema.StringAttribute {
	validators := []validator.String{StringMatches(
		model.PatternResourceName,
		"name must start with a lowercase letter and contain only lowercase letters, digits, or hyphens",
	)}
	if form.Role != model.RoleRevision {
		return schema.StringAttribute{
			Required:      true,
			Description:   "Portable resource name (metadata.name). Changing it replaces the resource.",
			Validators:    validators,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
	}
	return schema.StringAttribute{
		Optional: true,
		Computed: true,
		Description: "Portable resource name (metadata.name). Omit it and set `" + v3RevisionOwnerAttribute +
			"` instead: this Form is an immutable revision, so the provider derives \"" +
			v3RevisionNamePrefix(form) + "-<content digest prefix>-<owner digest prefix>\" from the revision's own " +
			"content and its declared owner, and changed content is therefore a new revision beside the old one. " +
			"Setting it pins the name, and the provider then refuses at plan time any change that would replace " +
			"this revision under it.",
		Validators:    validators,
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

// v3PendingOperationIDAttribute declares the internal-recovery-only record of
// an accepted-but-unfinished mutation. It is NOT part of resource identity and
// carries no host authority: it exists purely so a create the host ACCEPTED as
// a long-running Operation — but that did not reach a terminal state before
// the operation deadline — is not silently orphaned. State written on that path
// records the operation id here so the operator (and the next plan, which
// reconciles against the recorded name/space/FormRef) can see what to resume.
// Every successful read or apply clears it back to null.
func v3PendingOperationIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Computed: true,
		Description: "Internal recovery only. Host operation id of a mutation the host accepted but that did not " +
			"reach a terminal state before the operation deadline; null in steady state and cleared by the next " +
			"successful read or apply. It is not resource identity and configurations must not depend on it.",
	}
}

// v3RelationDriftAttribute is the state attribute recording that the host
// reports one of this resource's cross-resource relations as broken.
const v3RelationDriftAttribute = "relation_drift_reason"

// v3RelationDriftReasonAttribute declares the internal-recovery-only record of
// a relation the host reports as no longer resolving to the incarnation this
// resource was applied against: `ExternalChange` when the target name came back
// on a different resource, `DependencyMissing` when it is gone.
//
// It exists so drift recovery stays REACHABLE from Terraform. The host never
// re-binds a reference by name, so nothing about the desired spec changes when
// a target is replaced; without a recorded signal the refreshed plan would be
// empty, and an empty plan offers no apply to run. Recording the reason gives
// ModifyPlan the one fact it needs to propose the apply that re-pins the
// relation. It is provider-side recovery bookkeeping, never portable desired
// state: no v1beta1 wire member carries it, and configurations must not
// depend on it.
func v3RelationDriftReasonAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Computed: true,
		Description: "Internal recovery only. The portable condition reason (\"ExternalChange\" or " +
			"\"DependencyMissing\") when the host reports that a resource this one references was " +
			"replaced or removed out of band; null in steady state and cleared by the next successful " +
			"read or apply. It is not desired state, it is not part of the portable wire spec, and " +
			"configurations must not depend on it.",
	}
}

// v3FieldAttribute maps one declared catalog field to its framework schema
// attribute.
//
// Two rules run through every case:
//
//   - a field requires replacement when it is immutable, or when its Form
//     declares no update at all — in that Form nothing is changeable in place,
//     so every desired attribute must force replacement;
//   - a field that declares a portable default becomes Optional AND Computed
//     with the matching framework default, so the plan holds the same value
//     the host will materialize. Without the framework default the plan would
//     read null, the host would answer the default, and every apply would be
//     followed by a permanent diff.
func v3FieldAttribute(form model.Form, field model.Field) (schema.Attribute, error) {
	return v3AttributeForField(form.Family.APIVersion(), form.Kind, field, field.Immutable || !form.DeclaresUpdate())
}

// v3AttributeForField is the recursive typed-schema compiler for Form fields.
// Every admitted FieldKind has one concrete Terraform value type. There is no
// catch-all string representation: a model kind added without a provider
// mapping is a schema error, never a lossy coercion.
func v3AttributeForField(sourceGroup, formKind string, field model.Field, replace bool) (schema.Attribute, error) {
	computed := field.Default != nil
	optional := !field.Required
	description := field.Doc
	if computed {
		description += " Omitting it means " + v3DefaultProse(field) + "."
	}
	switch field.Kind {
	case model.KindBoolean:
		attribute := schema.BoolAttribute{Required: field.Required, Optional: optional, Computed: computed, Description: description}
		if computed {
			attribute.Default = booldefault.StaticBool(field.Default.(bool))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindInteger:
		var validators []validator.Int64
		if field.Min != nil {
			validators = append(validators, Int64AtLeast(*field.Min))
		}
		if field.Max != nil {
			validators = append(validators, Int64AtMost(*field.Max))
		}
		attribute := schema.Int64Attribute{
			Required: field.Required, Optional: optional, Computed: computed, Description: description, Validators: validators,
		}
		if computed {
			attribute.Default = int64default.StaticInt64(v3DefaultInt64(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindString, model.KindStringEnum:
		attribute := schema.StringAttribute{
			Required: field.Required, Optional: optional, Computed: computed, Description: description,
			Validators: v3StringValidators(field),
		}
		if computed {
			attribute.Default = stringdefault.StaticString(field.Default.(string))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindStringList:
		attribute := schema.ListAttribute{
			Required: field.Required, Optional: optional, Computed: computed, Description: description,
			ElementType: types.StringType,
			Validators:  []validator.List{v3StringListValidator{field: field}},
		}
		if computed {
			attribute.Default = listdefault.StaticValue(v3DefaultStringList(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindStringSet:
		var validators []validator.Set
		if len(field.Enum) > 0 {
			validators = append(validators, SetStringsOneOf(field.MinItems, field.Enum...))
		} else {
			validators = append(validators, SetStringsMatch(field.MinItems, field.ItemPattern, field.HCL+" items must match "+field.ItemPattern))
		}
		validators = append(validators, v3SetSizeValidator{maxItems: field.MaxItems})
		attribute := schema.SetAttribute{
			Required: field.Required, Optional: optional, Computed: computed, Description: description,
			ElementType: types.StringType, Validators: validators,
		}
		if computed {
			attribute.Default = setdefault.StaticValue(v3DefaultStringSet(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindStringMap, model.KindStringSetMap:
		elementType := attr.Type(types.StringType)
		if field.Kind == model.KindStringSetMap {
			elementType = types.SetType{ElemType: types.StringType}
		}
		attribute := schema.MapAttribute{
			Required: field.Required, Optional: optional, Computed: computed, Description: description,
			ElementType: elementType, Validators: []validator.Map{v3TypedStringMapValidator{field: field}},
		}
		if computed {
			attribute.Default = mapdefault.StaticValue(v3DefaultStringMap(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Map{mapplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindJSONMap:
		attribute := schema.StringAttribute{
			Optional: optional, Required: field.Required, Computed: computed,
			Description: description + " Authored as one JSON object string (for example jsonencode({...})); the provider sends the parsed object.",
			Validators:  []validator.String{v3JSONObjectValidator{}},
		}
		if computed {
			attribute.Default = stringdefault.StaticString(v3DefaultJSONText(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindResourceRef:
		target, err := field.EffectiveResourceTarget(sourceGroup)
		if err != nil {
			return nil, err
		}
		attribute := schema.StringAttribute{
			Required: field.Required, Optional: optional, Computed: computed,
			Description: description + " Set the name of the exact target " + target.Group + "/" + target.Kind + " resource.",
			Validators:  []validator.String{StringMatches(model.PatternResourceName, field.HCL+" must be a portable resource name")},
		}
		if computed {
			attribute.Default = stringdefault.StaticString(v3DefaultReferenceName(field.Default))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindResourceRefList:
		target, err := field.EffectiveResourceTarget(sourceGroup)
		if err != nil {
			return nil, err
		}
		attribute := schema.ListAttribute{
			Required: field.Required, Optional: optional, Computed: computed,
			Description: description + " Each item names an exact target " + target.Group + "/" + target.Kind + " resource.",
			ElementType: types.StringType,
			Validators:  []validator.List{v3ResourceNameListValidator{minItems: field.MinItems, maxItems: field.MaxItems}},
		}
		if computed {
			attribute.Default = listdefault.StaticValue(v3DefaultReferenceList(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindBindingList:
		target, err := field.EffectiveResourceTarget(sourceGroup)
		if err != nil {
			return nil, err
		}
		attribute := schema.ListNestedAttribute{
			Optional: optional, Required: field.Required, Computed: computed, Description: description,
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Required: true, Description: "JavaScript identifier the binding is projected under.",
					Validators: []validator.String{StringMatches(model.PatternBindingName, "binding name must be a JavaScript identifier")},
				},
				"target_name": schema.StringAttribute{
					Required: true, Description: "Name of the exact target " + target.Group + "/" + target.Kind + " resource.",
					Validators: []validator.String{StringMatches(model.PatternResourceName, "target_name must be a portable resource name")},
				},
			}},
			Validators: []validator.List{v3ListSizeValidator{minItems: field.MinItems, maxItems: field.MaxItems}},
		}
		if computed {
			attribute.Default = listdefault.StaticValue(v3DefaultBindingList(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindExternalServiceList:
		attribute := schema.ListNestedAttribute{
			Optional: optional, Required: field.Required, Computed: computed, Description: description,
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Required: true, Description: "SCREAMING_SNAKE slot name; the runtime members are projected under it.",
					Validators: []validator.String{StringMatches(model.PatternExternalServiceName, "slot name must be SCREAMING_SNAKE")},
				},
				"protocol": schema.StringAttribute{
					Required: true,
					Description: "Opaque normalized reverse-DNS protocol identifier. The Host must advertise exact support; " +
						"the provider does not maintain a protocol registry.",
					Validators: []validator.String{
						StringMatches(
							model.PatternStandardServiceProtocol,
							"protocol must be a normalized reverse-DNS owner namespace plus protocol segment",
						),
						StringRuneLengthAtMost(model.StandardServiceProtocolMaxLength),
					},
				},
				"required": schema.BoolAttribute{
					Optional: true, Computed: true, Default: booldefault.StaticBool(true),
					Description: "Whether the host must satisfy this slot for the resource to be Ready. Defaults to true.",
				},
			}},
			Validators: []validator.List{v3ListSizeValidator{minItems: field.MinItems, maxItems: field.MaxItems}},
		}
		if computed {
			attribute.Default = listdefault.StaticValue(v3DefaultExternalServiceList(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindObject, model.KindObjectList:
		nested := map[string]schema.Attribute{}
		for _, member := range field.Fields {
			attribute, err := v3AttributeForField(sourceGroup, formKind, member, false)
			if err != nil {
				return nil, fmt.Errorf("nested field %s: %w", member.Wire, err)
			}
			nested[member.HCL] = attribute
		}
		if field.Kind == model.KindObject {
			attribute := schema.SingleNestedAttribute{
				Required: field.Required, Optional: optional, Computed: computed, Description: description, Attributes: nested,
			}
			if computed {
				attribute.Default = objectdefault.StaticValue(v3DefaultObject(field))
			}
			if replace {
				attribute.PlanModifiers = []planmodifier.Object{objectplanmodifier.RequiresReplace()}
			}
			return attribute, nil
		}
		validators := []validator.List{v3ListSizeValidator{minItems: field.MinItems, maxItems: field.MaxItems}}
		if formKind == workerDeploymentKind && field.Wire == "versions" {
			validators = append(validators, v3WeightSumValidator{})
		}
		attribute := schema.ListNestedAttribute{
			Optional: optional, Required: field.Required, Computed: computed, Description: description,
			NestedObject: schema.NestedAttributeObject{Attributes: nested}, Validators: validators,
		}
		if computed {
			attribute.Default = listdefault.StaticValue(v3DefaultObjectList(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	case model.KindTaggedObject:
		nested, err := v3TaggedObjectAttributes(sourceGroup, formKind, field)
		if err != nil {
			return nil, err
		}
		attribute := schema.SingleNestedAttribute{
			Required: field.Required, Optional: optional, Computed: computed, Description: description,
			Attributes: nested, Validators: []validator.Object{v3TaggedObjectValidator{field: field}},
		}
		if computed {
			attribute.Default = objectdefault.StaticValue(v3DefaultTaggedObject(field))
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Object{objectplanmodifier.RequiresReplace()}
		}
		return attribute, nil
	default:
		return nil, fmt.Errorf("unsupported FieldKind %q", field.Kind)
	}
}

func v3StringValidators(field model.Field) []validator.String {
	if len(field.Enum) > 0 {
		return []validator.String{StringOneOf(field.Enum...)}
	}
	switch field.Pattern {
	case model.PatternCron:
		return []validator.String{v3CronValidator{}}
	case model.PatternHostname:
		return []validator.String{v3HostnameValidator{}}
	case "":
		return nil
	default:
		return []validator.String{StringMatches(field.Pattern, field.HCL+" must match "+field.Pattern)}
	}
}

func v3TaggedObjectAttributes(sourceGroup, formKind string, field model.Field) (map[string]schema.Attribute, error) {
	nested := map[string]schema.Attribute{
		field.Discriminator: schema.StringAttribute{
			Required: true, Description: "Selects exactly one closed variant.",
			Validators: []validator.String{StringOneOf(v3TaggedObjectTags(field)...)}},
	}
	declared := map[string]model.Field{}
	for _, variant := range field.Variants {
		for _, member := range variant.Fields {
			if previous, duplicate := declared[member.HCL]; duplicate {
				merged, compatible := mergeTaggedVariantMember(previous, member)
				if !compatible {
					return nil, fmt.Errorf("tagged variants declare incompatible member %q", member.HCL)
				}
				declared[member.HCL] = merged
				attribute, err := v3AttributeForField(sourceGroup, formKind, merged, false)
				if err != nil {
					return nil, fmt.Errorf("tagged variant %s member %s: %w", variant.Tag, member.Wire, err)
				}
				nested[member.HCL] = attribute
				continue
			}
			declared[member.HCL] = member
			flattened := member
			flattened.Required = false
			flattened.Default = nil // selected-branch defaults are materialized by the codec
			flattened.AbsenceIsSemantic = true
			attribute, err := v3AttributeForField(sourceGroup, formKind, flattened, false)
			if err != nil {
				return nil, fmt.Errorf("tagged variant %s member %s: %w", variant.Tag, member.Wire, err)
			}
			nested[member.HCL] = attribute
		}
	}
	return nested, nil
}

// mergeTaggedVariantMember projects one HCL attribute shared by multiple
// tagged branches. A branch may tighten the same scalar's pattern/length (the
// schedule/topic message body does this for UTF-8 vs base64); the outer typed
// schema must still expose one attribute, so it carries the conservative union
// while the exact Form schema validates the selected branch before a mutation
// is sent. Different wire names, kinds, targets, or nested shapes remain a
// hard schema error rather than a lossy coercion.
func mergeTaggedVariantMember(previous, current model.Field) (model.Field, bool) {
	if previous.HCL != current.HCL || previous.Wire != current.Wire || previous.Kind != current.Kind {
		return model.Field{}, false
	}
	left, right := previous, current
	for _, field := range []*model.Field{&left, &right} {
		field.Doc = ""
		field.Default = nil
		field.Pattern = ""
		field.MaxLength = 0
		field.Enum = nil
		field.Min, field.Max = nil, nil
		field.ItemPattern = ""
		field.MinItems, field.MaxItems = 0, 0
		field.MinProperties, field.MaxProperties = 0, 0
		field.Example, field.AltExample, field.CounterExample = nil, nil, nil
	}
	if !reflect.DeepEqual(left, right) {
		return model.Field{}, false
	}
	merged := previous
	merged.Default = nil
	merged.AbsenceIsSemantic = true
	merged.Pattern = ""
	merged.ItemPattern = ""
	merged.Enum = unionStrings(previous.Enum, current.Enum)
	merged.Min = conservativeMin(previous.Min, current.Min)
	merged.Max = conservativeMax(previous.Max, current.Max)
	merged.MaxLength = maxPositive(previous.MaxLength, current.MaxLength)
	merged.MinItems = conservativeLowerBound(previous.MinItems, current.MinItems)
	merged.MaxItems = conservativeUpperBound(previous.MaxItems, current.MaxItems)
	merged.MinProperties = conservativeLowerBound(previous.MinProperties, current.MinProperties)
	merged.MaxProperties = conservativeUpperBound(previous.MaxProperties, current.MaxProperties)
	return merged, true
}

func unionStrings(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, value := range append(append([]string(nil), left...), right...) {
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func conservativeMin(left, right *int64) *int64 {
	if left == nil || right == nil {
		return nil
	}
	if *left < *right {
		return left
	}
	return right
}

func conservativeMax(left, right *int64) *int64 {
	if left == nil || right == nil {
		return nil
	}
	if *left > *right {
		return left
	}
	return right
}

func maxPositive(left, right int) int {
	if left == 0 || right == 0 {
		return 0
	}
	if left > right {
		return left
	}
	return right
}

func conservativeLowerBound(left, right int) int {
	if left == 0 || right == 0 {
		return 0
	}
	if left < right {
		return left
	}
	return right
}

func conservativeUpperBound(left, right int) int {
	if left == 0 || right == 0 {
		return 0
	}
	if left > right {
		return left
	}
	return right
}

func v3TaggedObjectTags(field model.Field) []string {
	tags := make([]string, 0, len(field.Variants))
	for _, variant := range field.Variants {
		tags = append(tags, variant.Tag)
	}
	return tags
}

// v3OutputAttribute maps one declared Form output to its framework attribute.
//
// Every output is plain Computed — never Optional+Computed, and never carrying
// a framework Default. That is not a style choice, it is what the value IS: an
// output is computed by the host and a configuration cannot write one, so
// Optional would advertise an argument the provider must then refuse, and a
// Default would put a value in the plan that no host produced. It is exactly
// how `outputs_json`, `generation`, `revision`, and `ready` already behave, and
// a typed output is the same fact under a name.
//
// There is deliberately no UseStateForUnknown plan modifier either. An output
// is what the address, the URL, or the host-assigned name currently IS, and a
// change to this resource can move it; holding the prior value known through
// such a plan would show the operator a value the apply may replace, which is
// the perpetual-diff failure in its other direction. Terraform already keeps
// the prior value for a resource with no diff at all, so the cost is nothing.
func v3OutputAttribute(output model.Field) schema.Attribute {
	description := output.Doc + " Computed by the host; a configuration cannot set it."
	switch output.Kind {
	case model.KindBoolean:
		return schema.BoolAttribute{Computed: true, Description: description}
	case model.KindInteger:
		return schema.Int64Attribute{Computed: true, Description: description}
	default:
		// KindString and KindStringEnum. Validators are deliberately absent:
		// a validator runs against CONFIGURATION, and no configuration writes an
		// output. The bound that matters is the Form's outputSchema, which the
		// host is held to.
		return schema.StringAttribute{Computed: true, Description: description}
	}
}

// v3OutputValue projects one host output value onto its typed state value. An
// output the host did not return is null rather than an invented zero: null is
// how "the host has not answered this yet" reads in state, and it is the value
// an interrupted apply legitimately leaves behind.
func v3OutputValue(output model.Field, raw any) attr.Value {
	switch output.Kind {
	case model.KindBoolean:
		if value, ok := raw.(bool); ok {
			return types.BoolValue(value)
		}
		return types.BoolNull()
	case model.KindInteger:
		if raw == nil {
			return types.Int64Null()
		}
		return int64FromSpec(raw)
	default:
		if text, ok := raw.(string); ok {
			return types.StringValue(text)
		}
		return types.StringNull()
	}
}

// v3DefaultProse renders the declared default for the attribute description,
// so an operator reading the schema sees the same value the host will
// materialize.
func v3DefaultProse(field model.Field) string {
	if model.EmptyCollectionDefault(field) {
		if field.Kind == model.KindJSONMap {
			return "the empty object `{}`"
		}
		return "the empty list `[]`"
	}
	text, err := v3MarshalJSON(field.Default)
	if err != nil {
		return "the Form's declared default"
	}
	return "`" + text + "`"
}

func v3DefaultInt64(field model.Field) int64 {
	switch typed := field.Default.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	default:
		return 0
	}
}

func v3DefaultJSONText(field model.Field) string {
	text, err := v3MarshalJSON(field.Default)
	if err != nil {
		return "{}"
	}
	return text
}

func v3DefaultReferenceName(value any) string {
	reference, _ := value.(map[string]any)
	name, _ := reference["name"].(string)
	return name
}

func v3DefaultAnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func v3DefaultStringList(field model.Field) types.List {
	items := v3DefaultAnySlice(field.Default)
	elements := make([]attr.Value, 0, len(items))
	for _, item := range items {
		text, _ := item.(string)
		elements = append(elements, types.StringValue(text))
	}
	return types.ListValueMust(types.StringType, elements)
}

func v3DefaultReferenceList(field model.Field) types.List {
	items := v3DefaultAnySlice(field.Default)
	elements := make([]attr.Value, 0, len(items))
	for _, item := range items {
		elements = append(elements, types.StringValue(v3DefaultReferenceName(item)))
	}
	return types.ListValueMust(types.StringType, elements)
}

func v3DefaultStringSet(field model.Field) types.Set {
	items := v3DefaultAnySlice(field.Default)
	elements := make([]attr.Value, 0, len(items))
	for _, item := range items {
		text, _ := item.(string)
		elements = append(elements, types.StringValue(text))
	}
	return types.SetValueMust(types.StringType, elements)
}

func v3DefaultStringMap(field model.Field) types.Map {
	object := v3AnyMap(field.Default)
	elements := make(map[string]attr.Value, len(object))
	if field.Kind == model.KindStringMap {
		for key, raw := range object {
			text, _ := raw.(string)
			elements[key] = types.StringValue(text)
		}
		return types.MapValueMust(types.StringType, elements)
	}
	setType := types.SetType{ElemType: types.StringType}
	for key, raw := range object {
		items := v3DefaultAnySlice(raw)
		values := make([]attr.Value, 0, len(items))
		for _, item := range items {
			text, _ := item.(string)
			values = append(values, types.StringValue(text))
		}
		elements[key] = types.SetValueMust(types.StringType, values)
	}
	return types.MapValueMust(setType, elements)
}

func v3DefaultExternalServiceList(field model.Field) types.List {
	elementType := v3ExternalServiceObjectType()
	items := v3DefaultAnySlice(field.Default)
	elements := make([]attr.Value, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]any)
		name, _ := entry["name"].(string)
		var protocol string
		if service, ok := entry["service"].(map[string]any); ok {
			protocol, _ = service["protocol"].(string)
		}
		required := true
		if declared, ok := entry["required"].(bool); ok {
			required = declared
		}
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"name":     types.StringValue(name),
			"protocol": types.StringValue(protocol),
			"required": types.BoolValue(required),
		}))
	}
	return types.ListValueMust(elementType, elements)
}

func v3DefaultBindingList(field model.Field) types.List {
	elementType := v3BindingObjectType()
	items := v3DefaultAnySlice(field.Default)
	elements := make([]attr.Value, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]any)
		name, _ := entry["name"].(string)
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"name":        types.StringValue(name),
			"target_name": types.StringValue(v3DefaultReferenceName(entry["resource"])),
		}))
	}
	return types.ListValueMust(elementType, elements)
}

func v3DefaultObjectList(field model.Field) types.List {
	elementType := v3ObjectListType(field)
	items := v3DefaultAnySlice(field.Default)
	elements := make([]attr.Value, 0, len(items))
	for _, item := range items {
		entry := v3AnyMap(item)
		memberValues := map[string]attr.Value{}
		for _, member := range field.Fields {
			memberValues[member.HCL] = v3FieldValueFromDefault(member, entry[member.Wire])
		}
		elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, memberValues))
	}
	return types.ListValueMust(elementType, elements)
}

func v3DefaultObject(field model.Field) types.Object {
	entry := v3AnyMap(field.Default)
	memberValues := map[string]attr.Value{}
	for _, member := range field.Fields {
		memberValues[member.HCL] = v3FieldValueFromDefault(member, entry[member.Wire])
	}
	return types.ObjectValueMust(v3ObjectType(field).AttrTypes, memberValues)
}

func v3FieldValueFromDefault(field model.Field, raw any) attr.Value {
	if raw == nil {
		if field.Default != nil {
			raw = field.Default
		} else {
			return v3NullFieldValue(field)
		}
	}
	switch field.Kind {
	case model.KindBoolean:
		value, _ := raw.(bool)
		return types.BoolValue(value)
	case model.KindInteger:
		return int64FromSpec(raw)
	case model.KindString, model.KindStringEnum:
		text, _ := raw.(string)
		return types.StringValue(text)
	case model.KindStringList:
		copy := field
		copy.Default = raw
		return v3DefaultStringList(copy)
	case model.KindStringSet:
		copy := field
		copy.Default = raw
		return v3DefaultStringSet(copy)
	case model.KindStringMap, model.KindStringSetMap:
		copy := field
		copy.Default = raw
		return v3DefaultStringMap(copy)
	case model.KindJSONMap:
		text, _ := v3MarshalJSON(raw)
		return types.StringValue(text)
	case model.KindResourceRef:
		return types.StringValue(v3DefaultReferenceName(raw))
	case model.KindResourceRefList:
		copy := field
		copy.Default = raw
		return v3DefaultReferenceList(copy)
	case model.KindExternalServiceList:
		copy := field
		copy.Default = raw
		return v3DefaultExternalServiceList(copy)
	case model.KindBindingList:
		copy := field
		copy.Default = raw
		return v3DefaultBindingList(copy)
	case model.KindObject:
		nested := v3AnyMap(raw)
		values := map[string]attr.Value{}
		for _, member := range field.Fields {
			values[member.HCL] = v3FieldValueFromDefault(member, nested[member.Wire])
		}
		return types.ObjectValueMust(v3ObjectType(field).AttrTypes, values)
	case model.KindObjectList:
		copy := field
		copy.Default = raw
		return v3DefaultObjectList(copy)
	case model.KindTaggedObject:
		copy := field
		copy.Default = raw
		return v3DefaultTaggedObject(copy)
	default:
		panic(fmt.Sprintf("unsupported FieldKind %q in default conversion", field.Kind))
	}
}

func v3DefaultTaggedObject(field model.Field) types.Object {
	entry := v3AnyMap(field.Default)
	tag, _ := entry[field.Discriminator].(string)
	values := v3NullTaggedObjectMembers(field)
	values[field.Discriminator] = types.StringValue(tag)
	if variant, ok := v3TaggedObjectVariant(field, tag); ok {
		for _, member := range variant.Fields {
			values[member.HCL] = v3FieldValueFromDefault(member, entry[member.Wire])
		}
	}
	return types.ObjectValueMust(v3TaggedObjectType(field).AttrTypes, values)
}

func v3AnyMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	case map[string][]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return map[string]any{}
	}
}

// v3SpecFromValues projects planned values onto the portable wire spec for one
// exact FormRef. Only the fields THAT ref's codec declares travel, and an
// unknown plan value never becomes desired state. Encoding through the codec is
// what keeps an update to a resource created under an older definition version
// a mutation of that contract rather than a migration onto the current one
// (spec/decisions/0017).
func (r *v3FormResource) v3SpecFromValues(
	ctx context.Context,
	codec v3FormCodec,
	values v3Values,
) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	// A revision Form's name is DERIVED from the spec this function builds, so
	// it cannot be required before it. The caller resolves it afterwards
	// (v3EnsureRevisionName) and refuses an apply that still has none.
	if !r.derivesRevisionName() && (values.Name.IsUnknown() || values.Name.IsNull()) {
		diags.AddAttributeError(
			path.Root("name"),
			"Unknown "+r.form.Kind+" name",
			"name must be wholly known before the provider can apply this Form.",
		)
	}
	spec := map[string]any{}
	for _, field := range codec.Form.Fields {
		name := v3AttributeName(field)
		value, carried := values.Fields[name]
		if !carried {
			diags.Append(v3CodecFieldMissingError(codec, name))
			continue
		}
		if value == nil || value.IsNull() {
			continue
		}
		if value.IsUnknown() {
			diags.AddAttributeError(
				path.Root(name),
				"Unknown "+r.form.Kind+" field",
				name+" must be wholly known before the provider can apply this Form.",
			)
			continue
		}
		wire, fieldDiags := v3FieldToWire(ctx, codec.Form.Family.APIVersion(), field, name, value)
		diags.Append(fieldDiags...)
		if fieldDiags.HasError() {
			continue
		}
		if wire != nil {
			spec[field.Wire] = wire
		}
	}
	if diags.HasError() {
		return spec, diags
	}
	// Framework defaults normally arrive in the plan, but the portable wire
	// contract cannot depend on a particular planner having run them. Always
	// materialize the exact Form's defaults before naming or sending desired
	// state; semantic absence remains absent by definition.
	materialized := model.MaterializeDefaults(codec.DesiredSchema, spec)
	if err := formpackage.ValidateDesiredInstance(codec.DesiredSchema, materialized); err != nil {
		diags.AddAttributeError(
			path.Root("spec"),
			"Desired Form values violate the exact Form schema",
			fmt.Sprintf("The %s/%s desired document is invalid under %s: %v", codec.Ref.APIVersion, codec.Ref.Kind, codec.Ref.ExactKey().String(), err),
		)
	}
	return materialized, diags
}

// v3FieldToWire projects one typed HCL value onto its portable wire form.
// group is the Form's own family group: a cross-resource reference travels as
// the exact {apiVersion, kind, name} triple even though the author writes only
// the target's name.
func v3FieldToWire(
	ctx context.Context,
	group string,
	field model.Field,
	attrName string,
	value attr.Value,
) (any, diag.Diagnostics) {
	var diags diag.Diagnostics
	switch field.Kind {
	case model.KindBoolean:
		return value.(types.Bool).ValueBool(), diags
	case model.KindInteger:
		return value.(types.Int64).ValueInt64(), diags
	case model.KindString, model.KindStringEnum:
		return value.(types.String).ValueString(), diags
	case model.KindStringList:
		return v3StringListToWire(ctx, attrName, value.(types.List), &diags), diags
	case model.KindStringSet:
		set := value.(types.Set)
		members := make([]string, 0, len(set.Elements()))
		for _, member := range set.Elements() {
			if member == nil || member.IsNull() || member.IsUnknown() {
				diags.AddAttributeError(
					path.Root(attrName),
					"Unknown or null set member",
					attrName+" must contain only wholly known, non-null values.",
				)
				return nil, diags
			}
		}
		diags.Append(set.ElementsAs(ctx, &members, false)...)
		sort.Strings(members)
		return members, diags
	case model.KindStringMap:
		mapping := value.(types.Map)
		out := make(map[string]any, len(mapping.Elements()))
		for key, member := range mapping.Elements() {
			text, memberDiags := v3KnownString(attrName, key, member)
			diags.Append(memberDiags...)
			if memberDiags.HasError() {
				return nil, diags
			}
			out[key] = text
		}
		return out, diags
	case model.KindStringSetMap:
		mapping := value.(types.Map)
		out := make(map[string]any, len(mapping.Elements()))
		for key, member := range mapping.Elements() {
			set, ok := member.(types.Set)
			if !ok || set.IsNull() || set.IsUnknown() {
				diags.AddAttributeError(path.Root(attrName), "Unknown or null map value", attrName+"["+key+"] must be a wholly known string set.")
				return nil, diags
			}
			values := make([]string, 0, len(set.Elements()))
			diags.Append(set.ElementsAs(ctx, &values, false)...)
			if diags.HasError() {
				return nil, diags
			}
			sort.Strings(values)
			out[key] = values
		}
		return out, diags
	case model.KindJSONMap:
		text := value.(types.String).ValueString()
		parsed, err := v3ParseJSONObject(text)
		if err != nil {
			diags.AddAttributeError(path.Root(attrName), "Invalid JSON object", err.Error())
			return nil, diags
		}
		return parsed, diags
	case model.KindResourceRef:
		target, err := field.EffectiveResourceTarget(group)
		if err != nil {
			diags.AddAttributeError(path.Root(attrName), "Invalid ResourceTarget", err.Error())
			return nil, diags
		}
		return v3WireReference(target.Group, target.Kind, value.(types.String).ValueString()), diags
	case model.KindResourceRefList:
		target, err := field.EffectiveResourceTarget(group)
		if err != nil {
			diags.AddAttributeError(path.Root(attrName), "Invalid ResourceTarget", err.Error())
			return nil, diags
		}
		list := value.(types.List)
		names := v3StringListToWire(ctx, attrName, list, &diags)
		if diags.HasError() {
			return nil, diags
		}
		out := make([]any, 0, len(names))
		for _, name := range names {
			out = append(out, v3WireReference(target.Group, target.Kind, name))
		}
		return out, diags
	case model.KindExternalServiceList:
		list := value.(types.List)
		out := make([]any, 0, len(list.Elements()))
		for index, element := range list.Elements() {
			object, objectDiags := v3KnownObject(attrName, index, element)
			diags.Append(objectDiags...)
			if objectDiags.HasError() {
				return nil, diags
			}
			attributes := object.Attributes()
			name, nameDiags := v3KnownString(attrName, "name", attributes["name"])
			protocol, protocolDiags := v3KnownString(attrName, "protocol", attributes["protocol"])
			diags.Append(nameDiags...)
			diags.Append(protocolDiags...)
			if diags.HasError() {
				return nil, diags
			}
			entry := map[string]any{
				"name": name,
				"service": map[string]any{
					"apiVersion": model.StandardServiceAPIVersion,
					"protocol":   protocol,
				},
			}
			if required, ok := attributes["required"].(types.Bool); ok && !required.IsNull() && !required.IsUnknown() {
				entry["required"] = required.ValueBool()
			}
			out = append(out, entry)
		}
		return out, diags
	case model.KindBindingList:
		targetRef, err := field.EffectiveResourceTarget(group)
		if err != nil {
			diags.AddAttributeError(path.Root(attrName), "Invalid ResourceTarget", err.Error())
			return nil, diags
		}
		list := value.(types.List)
		out := make([]any, 0, len(list.Elements()))
		for index, element := range list.Elements() {
			object, objectDiags := v3KnownObject(attrName, index, element)
			diags.Append(objectDiags...)
			if objectDiags.HasError() {
				return nil, diags
			}
			attributes := object.Attributes()
			name, nameDiags := v3KnownString(attrName, "name", attributes["name"])
			target, targetDiags := v3KnownString(attrName, "target_name", attributes["target_name"])
			diags.Append(nameDiags...)
			diags.Append(targetDiags...)
			if diags.HasError() {
				return nil, diags
			}
			out = append(out, map[string]any{
				"name": name,
				// The wire key is `resource`, never `target` (decision 0010).
				"resource": v3WireReference(targetRef.Group, targetRef.Kind, target),
			})
		}
		return out, diags
	case model.KindObjectList:
		list := value.(types.List)
		out := make([]any, 0, len(list.Elements()))
		for index, element := range list.Elements() {
			object, objectDiags := v3KnownObject(attrName, index, element)
			diags.Append(objectDiags...)
			if objectDiags.HasError() {
				return nil, diags
			}
			entry, objectDiags := v3ObjectToWire(ctx, group, field.Fields, fmt.Sprintf("%s[%d]", attrName, index), object)
			diags.Append(objectDiags...)
			if objectDiags.HasError() {
				return nil, diags
			}
			out = append(out, entry)
		}
		return out, diags
	case model.KindObject:
		object, ok := value.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			diags.AddAttributeError(path.Root(attrName), "Unknown or null nested object",
				attrName+" must be a wholly known object.")
			return nil, diags
		}
		return v3ObjectToWire(ctx, group, field.Fields, attrName, object)
	case model.KindTaggedObject:
		object, ok := value.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			diags.AddAttributeError(path.Root(attrName), "Unknown or null tagged object", attrName+" must be a wholly known tagged object.")
			return nil, diags
		}
		return v3TaggedObjectToWire(ctx, group, field, attrName, object)
	default:
		diags.AddAttributeError(path.Root(attrName), "Unsupported Form field kind", fmt.Sprintf("Field %s uses unsupported FieldKind %q.", attrName, field.Kind))
		return nil, diags
	}
}

func v3StringListToWire(ctx context.Context, attrName string, list types.List, diags *diag.Diagnostics) []string {
	for _, member := range list.Elements() {
		if member == nil || member.IsNull() || member.IsUnknown() {
			diags.AddAttributeError(path.Root(attrName), "Unknown or null list member", attrName+" must contain only wholly known, non-null strings.")
			return nil
		}
	}
	out := make([]string, 0, len(list.Elements()))
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}

func v3ObjectToWire(ctx context.Context, group string, fields []model.Field, attrName string, object types.Object) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := map[string]any{}
	attributes := object.Attributes()
	for _, member := range fields {
		memberValue, present := attributes[member.HCL]
		if !present || memberValue == nil || memberValue.IsNull() {
			if member.Default != nil {
				memberValue = v3FieldValueFromDefault(member, member.Default)
			} else if member.Required {
				diags.AddAttributeError(path.Root(attrName), "Missing required nested value", attrName+"."+member.HCL+" is required by the Form.")
				return nil, diags
			} else {
				continue
			}
		}
		if memberValue.IsUnknown() {
			diags.AddAttributeError(path.Root(attrName), "Unknown nested value", attrName+"."+member.HCL+" must be wholly known.")
			return nil, diags
		}
		wire, memberDiags := v3FieldToWire(ctx, group, member, attrName+"."+member.HCL, memberValue)
		diags.Append(memberDiags...)
		if memberDiags.HasError() {
			return nil, diags
		}
		out[member.Wire] = wire
	}
	return out, diags
}

func v3TaggedObjectToWire(ctx context.Context, group string, field model.Field, attrName string, object types.Object) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	attributes := object.Attributes()
	tag, tagDiags := v3KnownString(attrName, field.Discriminator, attributes[field.Discriminator])
	diags.Append(tagDiags...)
	if tagDiags.HasError() {
		return nil, diags
	}
	variant, known := v3TaggedObjectVariant(field, tag)
	if !known {
		diags.AddAttributeError(path.Root(attrName), "Unknown tagged-object variant", fmt.Sprintf("%s.%s is %q; expected one of %s.", attrName, field.Discriminator, tag, strings.Join(v3TaggedObjectTags(field), ", ")))
		return nil, diags
	}
	selected := map[string]bool{}
	for _, member := range variant.Fields {
		selected[member.HCL] = true
	}
	for name, member := range attributes {
		if name == field.Discriminator || selected[name] || member == nil || member.IsNull() {
			continue
		}
		diags.AddAttributeError(path.Root(attrName), "Member belongs to another tagged-object variant", fmt.Sprintf("%s.%s is not admitted by selected variant %q.", attrName, name, tag))
		return nil, diags
	}
	body, bodyDiags := v3ObjectToWire(ctx, group, variant.Fields, attrName, object)
	diags.Append(bodyDiags...)
	if bodyDiags.HasError() {
		return nil, diags
	}
	body[field.Discriminator] = tag
	return body, diags
}

func v3TaggedObjectVariant(field model.Field, tag string) (model.TaggedObjectVariant, bool) {
	for _, variant := range field.Variants {
		if variant.Tag == tag {
			return variant, true
		}
	}
	return model.TaggedObjectVariant{}, false
}

// v3WireReference builds the exact three-member cross-resource reference.
func v3WireReference(group, targetKind, name string) map[string]any {
	return map[string]any{"apiVersion": group, "kind": targetKind, "name": name}
}

func v3KnownObject(attrName string, index int, element attr.Value) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	object, ok := element.(types.Object)
	if !ok || object.IsNull() || object.IsUnknown() {
		diags.AddAttributeError(
			path.Root(attrName),
			"Unknown or null nested object",
			fmt.Sprintf("%s[%d] must be a wholly known object.", attrName, index),
		)
		return types.Object{}, diags
	}
	return object, diags
}

func v3KnownString(attrName, member string, value attr.Value) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	text, ok := value.(types.String)
	if !ok || text.IsNull() || text.IsUnknown() {
		diags.AddAttributeError(
			path.Root(attrName),
			"Unknown or null nested value",
			attrName+"."+member+" must be wholly known and non-null.",
		)
		return "", diags
	}
	return text.ValueString(), diags
}

// v3FieldValueFromSpec is the inverse projection: one wire spec value back to
// its typed HCL representation, used by reads that adopt out-of-band change.
// sourceGroup is required for retained same-family ResourceTargets; new
// aggregate targets still carry their own group and therefore never inherit a
// kind-only lookup.
func v3FieldValueFromSpec(ctx context.Context, sourceGroup string, field model.Field, raw any, diags *diag.Diagnostics) attr.Value {
	if raw == nil {
		return v3NullFieldValue(field)
	}
	switch field.Kind {
	case model.KindBoolean:
		if value, ok := raw.(bool); ok {
			return types.BoolValue(value)
		}
		v3HostFieldTypeError(field, "boolean", raw, diags)
		return types.BoolNull()
	case model.KindInteger:
		if value, ok := v3ExactInt64(raw); ok {
			return types.Int64Value(value)
		}
		v3HostFieldTypeError(field, "exact 64-bit integer", raw, diags)
		return types.Int64Null()
	case model.KindString, model.KindStringEnum:
		if value, ok := raw.(string); ok {
			return types.StringValue(value)
		}
		v3HostFieldTypeError(field, "string", raw, diags)
		return types.StringNull()
	case model.KindStringList:
		return v3StringListFromSpec(ctx, field, raw, diags)
	case model.KindStringSet:
		items, ok := v3StrictStringSlice(raw)
		if !ok {
			v3HostFieldTypeError(field, "array of strings", raw, diags)
			return types.SetNull(types.StringType)
		}
		set, setDiags := types.SetValueFrom(ctx, types.StringType, items)
		diags.Append(setDiags...)
		return set
	case model.KindStringMap:
		object, ok := raw.(map[string]any)
		if !ok {
			v3HostFieldTypeError(field, "object of strings", raw, diags)
			return types.MapNull(types.StringType)
		}
		elements := make(map[string]attr.Value, len(object))
		for key, rawValue := range object {
			text, ok := rawValue.(string)
			if !ok {
				v3HostFieldTypeError(field, "object of strings", raw, diags)
				return types.MapNull(types.StringType)
			}
			elements[key] = types.StringValue(text)
		}
		return types.MapValueMust(types.StringType, elements)
	case model.KindStringSetMap:
		setType := types.SetType{ElemType: types.StringType}
		object, ok := raw.(map[string]any)
		if !ok {
			v3HostFieldTypeError(field, "object of string sets", raw, diags)
			return types.MapNull(setType)
		}
		elements := make(map[string]attr.Value, len(object))
		for key, rawValue := range object {
			items, ok := v3StrictStringSlice(rawValue)
			if !ok {
				v3HostFieldTypeError(field, "object of string sets", raw, diags)
				return types.MapNull(setType)
			}
			elements[key] = types.SetValueMust(types.StringType, v3StringValues(items))
		}
		return types.MapValueMust(setType, elements)
	case model.KindJSONMap:
		text, err := v3MarshalJSON(raw)
		if err != nil {
			diags.AddError("Host spec value is not serializable", err.Error())
			return types.StringNull()
		}
		return types.StringValue(text)
	case model.KindResourceRef:
		name, ok := v3ReferenceNameFromSpec(sourceGroup, field, raw, diags)
		if !ok {
			return types.StringNull()
		}
		return types.StringValue(name)
	case model.KindResourceRefList:
		items, ok := v3StrictAnySlice(raw)
		if !ok {
			v3HostFieldTypeError(field, "array of exact resource references", raw, diags)
			return types.ListNull(types.StringType)
		}
		elements := make([]attr.Value, 0, len(items))
		for _, item := range items {
			name, valid := v3ReferenceNameFromSpec(sourceGroup, field, item, diags)
			if !valid {
				return types.ListNull(types.StringType)
			}
			elements = append(elements, types.StringValue(name))
		}
		return types.ListValueMust(types.StringType, elements)
	case model.KindExternalServiceList:
		elementType := v3ExternalServiceObjectType()
		items, ok := v3StrictAnySlice(raw)
		if !ok {
			v3HostFieldTypeError(field, "array of external service slots", raw, diags)
			return types.ListNull(elementType)
		}
		elements := make([]attr.Value, 0, len(items))
		for _, item := range items {
			entry, entryOK := item.(map[string]any)
			name, nameOK := entry["name"].(string)
			service, serviceOK := entry["service"].(map[string]any)
			apiVersion, apiOK := service["apiVersion"].(string)
			protocol, protocolOK := service["protocol"].(string)
			if !entryOK || !nameOK || !serviceOK || !apiOK || !protocolOK || apiVersion != model.StandardServiceAPIVersion {
				v3HostFieldTypeError(field, "closed external service slot", item, diags)
				return types.ListNull(elementType)
			}
			// An absent `required` means the contract's default, true; the
			// state must carry the effective value, not the absence.
			required := true
			if declared, ok := entry["required"].(bool); ok {
				required = declared
			}
			elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
				"name":     types.StringValue(name),
				"protocol": types.StringValue(protocol),
				"required": types.BoolValue(required),
			}))
		}
		list, listDiags := types.ListValue(elementType, elements)
		diags.Append(listDiags...)
		return list
	case model.KindBindingList:
		elementType := v3BindingObjectType()
		items, ok := v3StrictAnySlice(raw)
		if !ok {
			v3HostFieldTypeError(field, "array of bindings", raw, diags)
			return types.ListNull(elementType)
		}
		elements := make([]attr.Value, 0, len(items))
		for _, item := range items {
			entry, entryOK := item.(map[string]any)
			name, nameOK := entry["name"].(string)
			targetName, targetOK := v3ReferenceNameFromSpec(sourceGroup, field, entry["resource"], diags)
			if !entryOK || !nameOK || !targetOK {
				v3HostFieldTypeError(field, "closed binding object", item, diags)
				return types.ListNull(elementType)
			}
			elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
				"name":        types.StringValue(name),
				"target_name": types.StringValue(targetName),
			}))
		}
		list, listDiags := types.ListValue(elementType, elements)
		diags.Append(listDiags...)
		return list
	case model.KindObjectList:
		elementType := v3ObjectListType(field)
		items, ok := v3StrictAnySlice(raw)
		if !ok {
			v3HostFieldTypeError(field, "array of typed objects", raw, diags)
			return types.ListNull(elementType)
		}
		elements := make([]attr.Value, 0, len(items))
		for _, item := range items {
			entry, entryOK := item.(map[string]any)
			if !entryOK {
				v3HostFieldTypeError(field, "array of typed objects", raw, diags)
				return types.ListNull(elementType)
			}
			elements = append(elements, v3ObjectValueFromSpec(ctx, sourceGroup, field.Fields, entry, diags))
		}
		list, listDiags := types.ListValue(elementType, elements)
		diags.Append(listDiags...)
		return list
	case model.KindObject:
		elementType := v3ObjectType(field)
		entry, ok := raw.(map[string]any)
		if !ok {
			v3HostFieldTypeError(field, "typed object", raw, diags)
			return types.ObjectNull(elementType.AttrTypes)
		}
		return v3ObjectValueFromSpec(ctx, sourceGroup, field.Fields, entry, diags)
	case model.KindTaggedObject:
		return v3TaggedObjectValueFromSpec(ctx, sourceGroup, field, raw, diags)
	default:
		diags.AddError("Unsupported Form field kind", fmt.Sprintf("Field %s uses unsupported FieldKind %q.", field.Wire, field.Kind))
		return v3NullFieldValue(field)
	}
}

func v3StringListFromSpec(ctx context.Context, field model.Field, raw any, diags *diag.Diagnostics) attr.Value {
	items, ok := v3StrictStringSlice(raw)
	if !ok {
		v3HostFieldTypeError(field, "array of strings", raw, diags)
		return types.ListNull(types.StringType)
	}
	value, valueDiags := types.ListValueFrom(ctx, types.StringType, items)
	diags.Append(valueDiags...)
	return value
}

func v3ObjectValueFromSpec(ctx context.Context, sourceGroup string, fields []model.Field, entry map[string]any, diags *diag.Diagnostics) types.Object {
	typeField := model.Field{Kind: model.KindObject, Fields: fields}
	elementType := v3ObjectType(typeField)
	memberValues := make(map[string]attr.Value, len(fields))
	known := make(map[string]bool, len(fields))
	for _, member := range fields {
		known[member.Wire] = true
		memberValues[member.HCL] = v3FieldValueFromSpec(ctx, sourceGroup, member, entry[member.Wire], diags)
	}
	for name := range entry {
		if !known[name] {
			diags.AddError("Host returned an undeclared object member", fmt.Sprintf("The host returned member %q outside the Form's closed object schema.", name))
		}
	}
	object, objectDiags := types.ObjectValue(elementType.AttrTypes, memberValues)
	diags.Append(objectDiags...)
	return object
}

func v3TaggedObjectValueFromSpec(ctx context.Context, sourceGroup string, field model.Field, raw any, diags *diag.Diagnostics) attr.Value {
	objectType := v3TaggedObjectType(field)
	entry, ok := raw.(map[string]any)
	if !ok {
		v3HostFieldTypeError(field, "tagged object", raw, diags)
		return types.ObjectNull(objectType.AttrTypes)
	}
	tag, ok := entry[field.Discriminator].(string)
	variant, known := v3TaggedObjectVariant(field, tag)
	if !ok || !known {
		v3HostFieldTypeError(field, "tagged object with a declared discriminator", raw, diags)
		return types.ObjectNull(objectType.AttrTypes)
	}
	values := v3NullTaggedObjectMembers(field)
	values[field.Discriminator] = types.StringValue(tag)
	allowed := map[string]bool{field.Discriminator: true}
	for _, member := range variant.Fields {
		allowed[member.Wire] = true
		values[member.HCL] = v3FieldValueFromSpec(ctx, sourceGroup, member, entry[member.Wire], diags)
	}
	for name := range entry {
		if !allowed[name] {
			diags.AddError("Host returned a member from another tagged-object variant", fmt.Sprintf("Field %s selected variant %q but the host returned member %q.", field.Wire, tag, name))
		}
	}
	value, valueDiags := types.ObjectValue(objectType.AttrTypes, values)
	diags.Append(valueDiags...)
	return value
}

func v3ReferenceNameFromSpec(sourceGroup string, field model.Field, raw any, diags *diag.Diagnostics) (string, bool) {
	target, err := field.EffectiveResourceTarget(sourceGroup)
	if err != nil {
		diags.AddError("Invalid ResourceTarget", err.Error())
		return "", false
	}
	reference, ok := raw.(map[string]any)
	if !ok {
		v3HostFieldTypeError(field, "exact resource reference", raw, diags)
		return "", false
	}
	apiVersion, apiOK := reference["apiVersion"].(string)
	kind, kindOK := reference["kind"].(string)
	name, nameOK := reference["name"].(string)
	if !apiOK || !kindOK || !nameOK || apiVersion != target.Group || kind != target.Kind {
		diags.AddError(
			"Host returned a resource reference outside the declared ResourceTarget",
			fmt.Sprintf("Field %s requires %s/%s, but the host returned apiVersion=%q kind=%q.", field.Wire, target.Group, target.Kind, apiVersion, kind),
		)
		return "", false
	}
	return name, true
}

func v3StrictStringSlice(raw any) ([]string, bool) {
	switch typed := raw.(type) {
	case []string:
		return append(make([]string, 0, len(typed)), typed...), true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func v3StrictAnySlice(raw any) ([]any, bool) {
	if items, ok := raw.([]any); ok {
		return items, true
	}
	return nil, false
}

func v3StringValues(items []string) []attr.Value {
	out := make([]attr.Value, 0, len(items))
	for _, item := range items {
		out = append(out, types.StringValue(item))
	}
	return out
}

func v3ExactInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	case float64:
		if value < -9223372036854775808 || value > 9223372036854775807 || value != float64(int64(value)) {
			return 0, false
		}
		return int64(value), true
	default:
		return 0, false
	}
}

func v3HostFieldTypeError(field model.Field, want string, raw any, diags *diag.Diagnostics) {
	diags.AddError("Host returned a value outside the Form field type", fmt.Sprintf("Field %s must be a %s; the host returned %T.", field.Wire, want, raw))
}

func v3ExternalServiceObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":     types.StringType,
		"protocol": types.StringType,
		"required": types.BoolType,
	}}
}

func v3BindingObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"target_name": types.StringType,
	}}
}

func v3ObjectListType(field model.Field) types.ObjectType {
	return v3ObjectType(field)
}

func v3ObjectType(field model.Field) types.ObjectType {
	memberTypes := map[string]attr.Type{}
	for _, member := range field.Fields {
		memberTypes[member.HCL] = v3FieldType(member)
	}
	return types.ObjectType{AttrTypes: memberTypes}
}

func v3TaggedObjectType(field model.Field) types.ObjectType {
	memberTypes := map[string]attr.Type{field.Discriminator: types.StringType}
	for _, variant := range field.Variants {
		for _, member := range variant.Fields {
			if previous, exists := memberTypes[member.HCL]; exists && !previous.Equal(v3FieldType(member)) {
				panic(fmt.Sprintf("tagged variants declare incompatible Terraform type for %q", member.HCL))
			}
			memberTypes[member.HCL] = v3FieldType(member)
		}
	}
	return types.ObjectType{AttrTypes: memberTypes}
}

func v3FieldType(field model.Field) attr.Type {
	switch field.Kind {
	case model.KindBoolean:
		return types.BoolType
	case model.KindInteger:
		return types.Int64Type
	case model.KindString, model.KindStringEnum, model.KindJSONMap, model.KindResourceRef:
		return types.StringType
	case model.KindStringList, model.KindResourceRefList:
		return types.ListType{ElemType: types.StringType}
	case model.KindStringSet:
		return types.SetType{ElemType: types.StringType}
	case model.KindStringMap:
		return types.MapType{ElemType: types.StringType}
	case model.KindStringSetMap:
		return types.MapType{ElemType: types.SetType{ElemType: types.StringType}}
	case model.KindExternalServiceList:
		return types.ListType{ElemType: v3ExternalServiceObjectType()}
	case model.KindBindingList:
		return types.ListType{ElemType: v3BindingObjectType()}
	case model.KindObjectList:
		return types.ListType{ElemType: v3ObjectListType(field)}
	case model.KindObject:
		return v3ObjectType(field)
	case model.KindTaggedObject:
		return v3TaggedObjectType(field)
	default:
		panic(fmt.Sprintf("unsupported FieldKind %q in Terraform type derivation", field.Kind))
	}
}

func v3NullFieldValue(field model.Field) attr.Value {
	switch field.Kind {
	case model.KindBoolean:
		return types.BoolNull()
	case model.KindInteger:
		return types.Int64Null()
	case model.KindString, model.KindStringEnum, model.KindJSONMap, model.KindResourceRef:
		return types.StringNull()
	case model.KindStringList, model.KindResourceRefList:
		return types.ListNull(types.StringType)
	case model.KindStringSet:
		return types.SetNull(types.StringType)
	case model.KindStringMap:
		return types.MapNull(types.StringType)
	case model.KindStringSetMap:
		return types.MapNull(types.SetType{ElemType: types.StringType})
	case model.KindExternalServiceList:
		return types.ListNull(v3ExternalServiceObjectType())
	case model.KindBindingList:
		return types.ListNull(v3BindingObjectType())
	case model.KindObjectList:
		return types.ListNull(v3ObjectListType(field))
	case model.KindObject:
		return types.ObjectNull(v3ObjectType(field).AttrTypes)
	case model.KindTaggedObject:
		return types.ObjectNull(v3TaggedObjectType(field).AttrTypes)
	default:
		panic(fmt.Sprintf("unsupported FieldKind %q in Terraform null derivation", field.Kind))
	}
}

func v3NullTaggedObjectMembers(field model.Field) map[string]attr.Value {
	typed := v3TaggedObjectType(field)
	values := make(map[string]attr.Value, len(typed.AttrTypes))
	values[field.Discriminator] = types.StringNull()
	seen := map[string]bool{}
	for _, variant := range field.Variants {
		for _, member := range variant.Fields {
			if !seen[member.HCL] {
				values[member.HCL] = v3NullFieldValue(member)
				seen[member.HCL] = true
			}
		}
	}
	return values
}

// v3ParseJSONObject parses one strict JSON object; anything else fails.
//
// The literal `null` is rejected explicitly: encoding/json accepts it into a
// map target and leaves the map nil, so it would pass as "valid JSON" and then
// travel to the host as an absent object rather than the desired state the
// configuration actually names. Callers surface the returned error against the
// attribute path, so the diagnostic names vars_json / spec_json itself.
func v3ParseJSONObject(text string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("value must be one JSON object: %w", err)
	}
	if parsed == nil {
		return nil, errors.New("value must be one JSON object, not the literal null; omit the attribute instead")
	}
	return parsed, nil
}

// v3MarshalJSON renders JSON deterministically (Go sorts object keys).
func v3MarshalJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// v3DurationValidator validates optional Go-duration timeout attributes.
type v3DurationValidator struct{}

func (v3DurationValidator) Description(context.Context) string {
	return "value must be a positive Go duration such as \"20m\""
}

func (v v3DurationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3DurationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, ok := v3ParseTimeout(req.ConfigValue.ValueString()); !ok {
		resp.Diagnostics.AddAttributeError(
			req.Path, "Invalid operation timeout",
			"value must be a positive Go duration such as \"20m\".",
		)
	}
}

// v3JSONObjectValidator validates that a configured string is one JSON
// object.
type v3JSONObjectValidator struct{}

func (v3JSONObjectValidator) Description(context.Context) string {
	return "value must be one JSON object"
}

func (v v3JSONObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3JSONObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := v3ParseJSONObject(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON object", err.Error())
	}
}

// v3ListSizeValidator enforces catalog min/max list cardinality.
type v3ListSizeValidator struct {
	minItems, maxItems int
}

func (v v3ListSizeValidator) Description(context.Context) string {
	return fmt.Sprintf("list must declare between %d and %d items", v.minItems, v.maxItems)
}

func (v v3ListSizeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v v3ListSizeValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	count := len(req.ConfigValue.Elements())
	if v.minItems > 0 && count < v.minItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too few items",
			fmt.Sprintf("at least %d item(s) required, got %d", v.minItems, count))
	}
	if v.maxItems > 0 && count > v.maxItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too many items",
			fmt.Sprintf("at most %d item(s) allowed, got %d", v.maxItems, count))
	}
}

// v3StringListValidator preserves the distinction between an ordered list and
// a set while enforcing the same item and cardinality bounds as the portable
// desired schema. It deliberately does not reject duplicates.
type v3StringListValidator struct{ field model.Field }

func (v v3StringListValidator) Description(context.Context) string {
	return "ordered string list must satisfy the Form's item and cardinality bounds"
}
func (v v3StringListValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v v3StringListValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v3ValidateListCount(req.Path, len(req.ConfigValue.Elements()), v.field.MinItems, v.field.MaxItems, &resp.Diagnostics)
	for _, item := range req.ConfigValue.Elements() {
		text, ok := item.(types.String)
		if !ok || text.IsNull() || text.IsUnknown() {
			continue
		}
		if err := v3ValidateStringItem(v.field, text.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid ordered-list item", err.Error())
		}
	}
}

type v3ResourceNameListValidator struct{ minItems, maxItems int }

func (v v3ResourceNameListValidator) Description(context.Context) string {
	return "resource reference names must satisfy the portable name grammar and list bounds"
}
func (v v3ResourceNameListValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v v3ResourceNameListValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v3ValidateListCount(req.Path, len(req.ConfigValue.Elements()), v.minItems, v.maxItems, &resp.Diagnostics)
	pattern := regexp.MustCompile(model.PatternResourceName)
	for _, item := range req.ConfigValue.Elements() {
		text, ok := item.(types.String)
		if ok && !text.IsNull() && !text.IsUnknown() && !pattern.MatchString(text.ValueString()) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid resource reference name", "Every item must be a portable resource name.")
		}
	}
}

type v3SetSizeValidator struct{ maxItems int }

func (v v3SetSizeValidator) Description(context.Context) string {
	return "set must not exceed its Form maximum"
}
func (v v3SetSizeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v v3SetSizeValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || v.maxItems == 0 {
		return
	}
	if count := len(req.ConfigValue.Elements()); count > v.maxItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too many set items", fmt.Sprintf("At most %d item(s) are allowed, got %d.", v.maxItems, count))
	}
}

type v3TypedStringMapValidator struct{ field model.Field }

func (v v3TypedStringMapValidator) Description(context.Context) string {
	return "typed map keys and values must satisfy the Form bounds"
}
func (v v3TypedStringMapValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v v3TypedStringMapValidator) ValidateMap(_ context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	count := len(req.ConfigValue.Elements())
	if v.field.MinProperties > 0 && count < v.field.MinProperties {
		resp.Diagnostics.AddAttributeError(req.Path, "Too few map entries", fmt.Sprintf("At least %d entries are required, got %d.", v.field.MinProperties, count))
	}
	if v.field.MaxProperties > 0 && count > v.field.MaxProperties {
		resp.Diagnostics.AddAttributeError(req.Path, "Too many map entries", fmt.Sprintf("At most %d entries are allowed, got %d.", v.field.MaxProperties, count))
	}
	keys := regexp.MustCompile(model.PortableMapKeyPattern)
	for key, value := range req.ConfigValue.Elements() {
		if !keys.MatchString(key) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid portable map key", fmt.Sprintf("Map key %q does not match %s.", key, model.PortableMapKeyPattern))
		}
		if v.field.Kind == model.KindStringMap {
			text, ok := value.(types.String)
			if ok && !text.IsNull() && !text.IsUnknown() {
				if err := v3ValidateStringItem(v.field, text.ValueString()); err != nil {
					resp.Diagnostics.AddAttributeError(req.Path, "Invalid map value", fmt.Sprintf("Map value %q: %v", key, err))
				}
			}
			continue
		}
		set, ok := value.(types.Set)
		if !ok || set.IsNull() || set.IsUnknown() {
			continue
		}
		v3ValidateListCount(req.Path, len(set.Elements()), v.field.MinItems, v.field.MaxItems, &resp.Diagnostics)
		for _, item := range set.Elements() {
			text, ok := item.(types.String)
			if ok && !text.IsNull() && !text.IsUnknown() {
				if err := v3ValidateStringItem(v.field, text.ValueString()); err != nil {
					resp.Diagnostics.AddAttributeError(req.Path, "Invalid set-map value", fmt.Sprintf("Map value %q: %v", key, err))
				}
			}
		}
	}
}

type v3TaggedObjectValidator struct{ field model.Field }

func (v v3TaggedObjectValidator) Description(context.Context) string {
	return "tagged object must select exactly one closed variant"
}
func (v v3TaggedObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v v3TaggedObjectValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attributes := req.ConfigValue.Attributes()
	discriminator, ok := attributes[v.field.Discriminator].(types.String)
	if !ok || discriminator.IsNull() || discriminator.IsUnknown() {
		resp.Diagnostics.AddAttributeError(req.Path, "Missing tagged-object discriminator", v.field.Discriminator+" must select exactly one variant.")
		return
	}
	variant, known := v3TaggedObjectVariant(v.field, discriminator.ValueString())
	if !known {
		resp.Diagnostics.AddAttributeError(req.Path, "Unknown tagged-object variant", fmt.Sprintf("%q is not one of %s.", discriminator.ValueString(), strings.Join(v3TaggedObjectTags(v.field), ", ")))
		return
	}
	selected := map[string]bool{}
	for _, member := range variant.Fields {
		selected[member.HCL] = true
		if member.Required {
			value := attributes[member.HCL]
			if value == nil || value.IsNull() {
				resp.Diagnostics.AddAttributeError(req.Path, "Missing selected-variant member", fmt.Sprintf("Variant %q requires member %s.", variant.Tag, member.HCL))
			}
		}
	}
	for name, value := range attributes {
		if name == v.field.Discriminator || selected[name] || value == nil || value.IsNull() {
			continue
		}
		resp.Diagnostics.AddAttributeError(req.Path, "Member belongs to another tagged-object variant", fmt.Sprintf("Variant %q does not admit member %s.", variant.Tag, name))
	}
}

func v3ValidateListCount(attributePath path.Path, count, minItems, maxItems int, diags *diag.Diagnostics) {
	if minItems > 0 && count < minItems {
		diags.AddAttributeError(attributePath, "Too few items", fmt.Sprintf("At least %d item(s) are required, got %d.", minItems, count))
	}
	if maxItems > 0 && count > maxItems {
		diags.AddAttributeError(attributePath, "Too many items", fmt.Sprintf("At most %d item(s) are allowed, got %d.", maxItems, count))
	}
}

func v3ValidateStringItem(field model.Field, value string) error {
	if len(field.Enum) > 0 {
		for _, candidate := range field.Enum {
			if value == candidate {
				return nil
			}
		}
		return fmt.Errorf("%q is outside the declared vocabulary", value)
	}
	if field.ItemPattern != "" && !regexp.MustCompile(field.ItemPattern).MatchString(value) {
		return fmt.Errorf("%q does not match %s", value, field.ItemPattern)
	}
	if field.MaxLength > 0 && utf8.RuneCountInString(value) > field.MaxLength {
		return fmt.Errorf("%q exceeds maximum length %d", value, field.MaxLength)
	}
	return nil
}

// v3WeightSumValidator proves WorkerDeployment traffic weights sum to exactly
// 10000 basis points. The sum is checked only when every weight is known; a
// schema cannot add weights, so the provider does (and the host re-proves).
type v3WeightSumValidator struct{}

func (v3WeightSumValidator) Description(context.Context) string {
	return fmt.Sprintf("version weights must sum to exactly %d basis points", workerDeploymentWeightSum)
}

func (v v3WeightSumValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3WeightSumValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var sum int64
	for _, element := range req.ConfigValue.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			return
		}
		weight, ok := object.Attributes()["weight"].(types.Int64)
		if !ok || weight.IsNull() || weight.IsUnknown() {
			return
		}
		sum += weight.ValueInt64()
	}
	if sum != workerDeploymentWeightSum {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid deployment weight total",
			fmt.Sprintf("version weights must sum to exactly %d basis points, got %d", workerDeploymentWeightSum, sum),
		)
	}
}

// v3CronValidator proves a configured cron expression is one a conforming host
// accepts, using the same parser the host runs.
//
// The regex in the desired schema is the structural half of the grammar: it
// admits `5-1`, `0-99`, and `*/0`, which are shapes rather than schedules. The
// provider therefore parses, exactly as the host does, so a plan never shows a
// trigger that apply will refuse — and the diagnostic names the offending field
// instead of echoing a pattern the author has to decode.
type v3CronValidator struct{}

func (v3CronValidator) Description(context.Context) string {
	return "value must be a five-field UTC cron expression the portable grammar accepts"
}

func (v v3CronValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3CronValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := model.ValidateCron(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid cron expression",
			fmt.Sprintf("%q is not a portable UTC cron expression: %s", req.ConfigValue.ValueString(), err.Error()),
		)
	}
}

// v3HostnameValidator refuses a DNS hostname written in any spelling but its
// canonical one.
//
// A host canonicalizes before it compares and before it stores
// (spec/decisions/0026), so "API.Example.com" and "api.example.com." are
// applied as "api.example.com". A Terraform attribute holds the literal string
// the author wrote, so a configuration the host would rewrite plans one value
// and reads back another on every refresh, forever. Terraform also forbids a
// plan modifier from replacing a Required attribute's configured value, so the
// honest client-side rule is a refusal that names the canonical spelling
// rather than a silent rewrite.
type v3HostnameValidator struct{}

func (v3HostnameValidator) Description(context.Context) string {
	return "value must be a canonical DNS hostname: lowercase, with no trailing dot"
}

func (v v3HostnameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3HostnameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	written := req.ConfigValue.ValueString()
	if !v3HostnamePattern.MatchString(written) {
		resp.Diagnostics.AddAttributeError(
			req.Path, "Invalid hostname",
			fmt.Sprintf("%q is not a dotted ASCII DNS hostname. An internationalized name is written as its A-label.", written),
		)
		return
	}
	if model.HostnameIsCanonical(written) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path, "Hostname is not canonical",
		fmt.Sprintf(
			"%q and %q are one hostname to DNS, and a host stores the canonical spelling. Write %q.",
			written, model.CanonicalHostname(written), model.CanonicalHostname(written),
		),
	)
}

var v3HostnamePattern = regexp.MustCompile(model.PatternHostname)

// v3ParseTimeout parses one positive Go duration.
func v3ParseTimeout(text string) (time.Duration, bool) {
	parsed, err := time.ParseDuration(text)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
