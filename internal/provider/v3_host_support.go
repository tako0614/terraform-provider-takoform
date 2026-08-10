package provider

// v3_host_support.go decides host capability at PLAN time.
//
// Three planes meet at an apply and are kept apart here, deliberately and
// visibly:
//
//   - FORM semantics belong to Takoform. What a `WorkerVersion` means, which
//     fields it has, which values they may take, and which Interfaces and
//     Bindings it references are properties of the exact FormRef, identical on
//     every host. Nothing on this page changes them, and no host-specific value
//     ever enters a Form's desired state.
//   - HOST capability belongs to the Host Support Profile
//     (spec/schemas/host-support-profile-v1alpha1.schema.json). Whether THIS
//     host implements that exact FormRef, that Interface, that Binding, and
//     which subset of a closed enum, which numeric range, and which ceilings it
//     accepts. That is what this file reads and what it decides against.
//   - CAPACITY, price, region, and SLA belong to a Service Offering, which is
//     not part of this API at all. A support profile that carried a price would
//     be invalid against the published schema, and this file would ignore it if
//     it did.
//
// Why the check is transparent rather than a `data "takoform_host_support"`
// source. A data source reports; the plan has to DECIDE. An author who never
// writes the data block would keep discovering an unsupported bucket binding at
// apply, which is the whole defect, and one who does write it would have to
// restate every Form's own requirements as `precondition` blocks — the same
// facts a third time, in HCL, maintained by hand, on every worker. The
// requirements are already declared: the catalog Form names its Interfaces, its
// Bindings, its enums and its ranges, and the profile names what the host
// implements. Comparing two declarations the provider already holds needs no
// configuration at all, so it is done for every resource, always.
//
// What the check refuses is a host that SAYS no. A host that says nothing —
// no profile route, an unreadable answer, a profile that omits a limit — is not
// treated as a refusal: the lane lets a profile omit everything but identity
// and operations, so reading omission as denial would refuse conforming hosts.
// An unreadable support surface is a warning that names what could not be
// decided, and apply remains the backstop it is today.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// v3SupportCache reads each support profile at most once per provider
// configuration. A plan touches many resources of few kinds, and the profile is
// a static statement about the host, so re-reading it per resource would turn
// one fact into N round trips inside `terraform plan`.
type v3SupportCache struct {
	mu         sync.Mutex
	forms      map[currentformregistry.ExactFormKey]v3SupportAnswer
	interfaces map[string]v3SupportAnswer
	bindings   map[string]v3SupportAnswer
}

// v3SupportAnswer is one resolved support question: the profile the host
// served, or the reason it served none.
type v3SupportAnswer struct {
	Profile map[string]any
	// Refused means the host answered that it does NOT support this identity.
	Refused bool
	// Err means the question could not be decided at all.
	Err error
}

func newV3SupportCache() *v3SupportCache {
	return &v3SupportCache{
		forms:      map[currentformregistry.ExactFormKey]v3SupportAnswer{},
		interfaces: map[string]v3SupportAnswer{},
		bindings:   map[string]v3SupportAnswer{},
	}
}

// v3SupportRefusal reports whether a client error is the host stating that it
// does not carry an identity, rather than a failure to ask. `form_unknown` and
// `resource_not_found` on a support route are the two refusals the lane
// defines; everything else — unauthenticated, unavailable, a transport failure
// — leaves the question undecided.
func v3SupportRefusal(err error) bool {
	var apiErr *clientv3.APIError
	if !errors.As(err, &apiErr) || apiErr.ProtocolInvalid {
		return false
	}
	return apiErr.Code == "form_unknown" || apiErr.Code == "resource_not_found"
}

func (c *v3SupportCache) formSupport(ctx context.Context, client *clientv3.Client, ref currentformregistry.V3Ref) v3SupportAnswer {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := ref.ExactKey()
	if answer, cached := c.forms[key]; cached {
		return answer
	}
	profile, err := client.GetFormSupport(ctx, clientFormRef(ref))
	answer := v3SupportAnswer{Profile: profile, Err: err, Refused: v3SupportRefusal(err)}
	if answer.Refused {
		answer.Err = nil
	}
	c.forms[key] = answer
	return answer
}

func (c *v3SupportCache) interfaceSupport(ctx context.Context, client *clientv3.Client, name, version string) v3SupportAnswer {
	return c.contractSupport(ctx, &c.interfaces, name+"@"+version, func() (map[string]any, error) {
		return client.GetInterfaceSupport(ctx, name, version)
	})
}

func (c *v3SupportCache) bindingSupport(ctx context.Context, client *clientv3.Client, name, version string) v3SupportAnswer {
	return c.contractSupport(ctx, &c.bindings, name+"@"+version, func() (map[string]any, error) {
		return client.GetBindingSupport(ctx, name, version)
	})
}

func (c *v3SupportCache) contractSupport(
	_ context.Context, table *map[string]v3SupportAnswer, key string, read func() (map[string]any, error),
) v3SupportAnswer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if answer, cached := (*table)[key]; cached {
		return answer
	}
	profile, err := read()
	answer := v3SupportAnswer{Profile: profile, Err: err, Refused: v3SupportRefusal(err)}
	if answer.Refused {
		answer.Err = nil
	}
	(*table)[key] = answer
	return answer
}

// v3PlanHostSupport is the plan-time capability decision for one resource.
func (r *v3FormResource) v3PlanHostSupport(ctx context.Context, resp *resource.ModifyPlanResponse) {
	if resp.Plan.Raw.IsNull() || r.data == nil || r.data.clientV3 == nil || r.data.support == nil {
		return
	}
	codec, ok := r.v3PlanCodec(ctx, resp)
	if !ok {
		return
	}
	planCtx, cancel := context.WithTimeout(ctx, planPreviewTimeout)
	defer cancel()

	answer := r.data.support.formSupport(planCtx, r.data.clientV3, codec.Ref)
	switch {
	case answer.Refused:
		resp.Diagnostics.Append(v3Diagnostic{
			Summary:      "This host does not support " + r.form.Kind,
			ResourceType: r.form.ResourceType,
			Ref:          codec.Ref,
			Pointer:      "/form",
			Code:         v3CodeFormUnsupported,
			Detail: "The host's Support Profile surface answers that it carries no support for this exact Form " +
				"identity, so the apply would be refused. This is a statement about the HOST, not about the " +
				"Form: the same configuration applies unchanged against a host that implements it.",
			Repair: "Remove the " + r.form.ResourceType + " resources from this configuration, or point the " +
				"provider at a host whose Host Support Profile declares this exact FormRef.",
		}.error())
		return
	case answer.Err != nil:
		resp.Diagnostics.Append(v3SupportUnreadable(r.form.ResourceType, "Form "+r.form.Kind, answer.Err))
		return
	}
	profile := v3SupportProfile(answer.Profile)
	r.checkSupportedOperations(profile, codec.Ref, resp)
	r.checkSupportedContracts(planCtx, codec, resp)
	r.checkPlannedValues(ctx, profile, codec, resp)
}

// v3SupportUnreadable is the warning for a capability question the plan could
// not decide. It is a warning and not an error on purpose: the provider must
// not refuse an apply because it failed to ASK.
func v3SupportUnreadable(resourceType, subject string, err error) diag.Diagnostic {
	return v3Diagnostic{
		Summary:      "Host support for " + subject + " could not be read at plan time",
		ResourceType: resourceType,
		Code:         v3CodeHostSupportUnknown,
		Cause:        err,
		Detail: "The plan therefore decides nothing about this capability, and apply remains the place the " +
			"host answers. Nothing has been refused.",
		Repair: "If this keeps happening, check that the endpoint serves the Host Support Profile routes the " +
			"v1beta1 lane requires; capacity, price, region, and SLA are Service Offering facts and are never " +
			"part of that surface.",
	}.warning()
}

// v3SupportProfile is a small reader over the loosely typed profile document.
type v3SupportProfile map[string]any

func (p v3SupportProfile) stringSlice(key string) []string {
	raw, _ := p[key].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func (p v3SupportProfile) enum(field string) ([]string, bool) {
	enums, _ := p["supportedEnums"].(map[string]any)
	raw, present := enums[field].([]any)
	if !present {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out, true
}

func (p v3SupportProfile) rangeFor(field string) (int64, bool, int64, bool) {
	ranges, _ := p["supportedRanges"].(map[string]any)
	bounds, present := ranges[field].(map[string]any)
	if !present {
		return 0, false, 0, false
	}
	minimum, hasMinimum := v3SupportInt(bounds["minimum"])
	maximum, hasMaximum := v3SupportInt(bounds["maximum"])
	return minimum, hasMinimum, maximum, hasMaximum
}

func (p v3SupportProfile) limit(name string) (int64, bool) {
	limits, _ := p["limits"].(map[string]any)
	if limits == nil {
		return 0, false
	}
	return v3SupportInt(limits[name])
}

func v3SupportInt(value any) (int64, bool) {
	parsed := int64FromSpec(value)
	if parsed.IsNull() {
		return 0, false
	}
	return parsed.ValueInt64(), true
}

// checkSupportedOperations proves the host implements the lifecycle this Form's
// resources actually need: create and delete for every resource, and update
// only where the Form declares one.
func (r *v3FormResource) checkSupportedOperations(
	profile v3SupportProfile, ref currentformregistry.V3Ref, resp *resource.ModifyPlanResponse,
) {
	declared := profile.stringSlice("operations")
	if len(declared) == 0 {
		return
	}
	offered := map[string]bool{}
	for _, operation := range declared {
		offered[operation] = true
	}
	required := []string{"create", "read", "delete"}
	if r.form.DeclaresUpdate() {
		required = append(required, "update")
	}
	missing := make([]string, 0, len(required))
	for _, operation := range required {
		if !offered[operation] {
			missing = append(missing, operation)
		}
	}
	if len(missing) == 0 {
		return
	}
	resp.Diagnostics.Append(v3Diagnostic{
		Summary:      "This host does not offer the whole " + r.form.Kind + " lifecycle",
		ResourceType: r.form.ResourceType,
		Ref:          ref,
		Pointer:      "/form",
		Code:         v3CodeCapabilityUnsupported,
		Detail: fmt.Sprintf(
			"The host's Form Support Profile declares operations %s, which omits %s. Terraform manages a "+
				"resource for its whole life, so a missing operation is discovered on the day it is needed.",
			strings.Join(declared, ", "), strings.Join(missing, ", "),
		),
		Repair: "Apply these resources against a host that declares the full lifecycle for this exact FormRef.",
	}.error())
}

// checkSupportedContracts proves the host implements every exact Interface this
// Form states and every Binding contract the planned configuration uses.
//
// The Interfaces are the Form's own: a `ModuleWorker` claims `worker.runtime`
// and `worker.service`, and a host that supports the Form without implementing
// those contracts supports a different thing under the same name. The Bindings
// are the ones the plan actually reaches for, so a worker with only KV bindings
// is never refused for a bucket contract it does not use — which is exactly the
// host shape this check exists for.
func (r *v3FormResource) checkSupportedContracts(
	ctx context.Context, codec v3FormCodec, resp *resource.ModifyPlanResponse,
) {
	for _, contract := range codec.Form.ProvidedInterfaces {
		answer := r.data.support.interfaceSupport(ctx, r.data.clientV3, contract.Name, contract.Version)
		switch {
		case answer.Refused:
			resp.Diagnostics.Append(v3Diagnostic{
				Summary:      "This host does not implement " + contract.Name + "@" + contract.Version,
				ResourceType: r.form.ResourceType,
				Ref:          codec.Ref,
				Pointer:      "/form",
				Code:         v3CodeInterfaceUnsupported,
				Detail: r.form.Kind + " states that contract in its providedInterfaces, so a host serving this " +
					"Form without implementing it would answer a different contract under the same identity.",
				Repair: "Apply against a host whose Interface Support Profile declares " + contract.Name +
					"@" + contract.Version + ".",
			}.error())
		case answer.Err != nil:
			resp.Diagnostics.Append(v3SupportUnreadable(
				r.form.ResourceType, "Interface "+contract.Name+"@"+contract.Version, answer.Err))
		}
	}
	for _, field := range codec.Form.Fields {
		if field.Kind != model.KindBindingList || !r.plannedBindingUsed(ctx, field, resp) {
			continue
		}
		version := v3AcceptedBindingVersion(codec.Form, field.BindingType)
		answer := r.data.support.bindingSupport(ctx, r.data.clientV3, field.BindingType, version)
		attribute := path.Root(v3AttributeName(field))
		switch {
		case answer.Refused:
			resp.Diagnostics.Append(v3Diagnostic{
				Summary:      "This host does not implement the " + field.BindingType + " binding",
				ResourceType: r.form.ResourceType,
				Ref:          codec.Ref,
				Pointer:      "/" + field.Wire,
				Attribute:    &attribute,
				Code:         v3CodeBindingUnsupported,
				Detail: "A host may implement " + r.form.Kind + " and some of its binding contracts without " +
					"implementing them all. This one is declared in the configuration and the host's Binding " +
					"Support Profile does not carry it, so the apply would be refused with " +
					"unsupported_capability (422).",
				Repair: "Remove this binding list, or apply against a host whose Binding Support Profile declares " +
					field.BindingType + "@" + version + ".",
			}.error())
		case answer.Err != nil:
			resp.Diagnostics.Append(v3SupportUnreadable(
				r.form.ResourceType, "Binding "+field.BindingType+"@"+version, answer.Err))
		}
	}
}

// v3AcceptedBindingVersion resolves the version of one Binding contract from
// the Form's own acceptedBindings, so the version is never spelled twice.
func v3AcceptedBindingVersion(form model.Form, name string) string {
	for _, accepted := range form.AcceptedBindings {
		if accepted.Name == name {
			return accepted.Version
		}
	}
	return "1.0.0"
}

// plannedBindingUsed reports whether the plan actually declares an entry in one
// binding list.
func (r *v3FormResource) plannedBindingUsed(
	ctx context.Context, field model.Field, resp *resource.ModifyPlanResponse,
) bool {
	var list types.List
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root(v3AttributeName(field)), &list)...)
	if resp.Diagnostics.HasError() || list.IsNull() || list.IsUnknown() {
		return false
	}
	return len(list.Elements()) > 0
}

// checkPlannedValues holds the planned desired state to the capability subsets
// the host advertises: the enum values it accepts, the inclusive ranges it
// accepts, and the ceilings it publishes.
//
// The ceilings are read by convention from the field's own wire name —
// `maximumHandlers` bounds `handlers`, `maximumVersions` bounds a deployment's
// `versions`, `maximumRequiredSensitiveVars` bounds the secret slots a version
// asks for — plus the one ceiling the lane names in prose,
// `maximumBundleBytes`, which is checked against the artifact this plan would
// commit. Nothing is invented for a limit the host does not publish.
func (r *v3FormResource) checkPlannedValues(
	ctx context.Context, profile v3SupportProfile, codec v3FormCodec, resp *resource.ModifyPlanResponse,
) {
	values, diags := r.v3ValuesFrom(ctx, resp.Plan)
	if diags.HasError() {
		return
	}
	if v3ArtifactBackedRevision(r.form.Kind) {
		r.checkArtifactCeiling(profile, codec, values, resp)
		return
	}
	for _, field := range codec.Form.Fields {
		name := v3AttributeName(field)
		value, carried := values.Fields[name]
		if !carried || value == nil || value.IsNull() || value.IsUnknown() {
			continue
		}
		attribute := path.Root(name)
		if accepted, advertised := profile.enum(field.Wire); advertised {
			for _, written := range v3PlannedStrings(value) {
				if v3Contains(accepted, written) {
					continue
				}
				resp.Diagnostics.Append(v3Diagnostic{
					Summary:      "This host does not accept " + name + " = " + written,
					ResourceType: r.form.ResourceType,
					Ref:          codec.Ref,
					Pointer:      "/" + field.Wire,
					Attribute:    &attribute,
					Code:         v3CodeCapabilityUnsupported,
					Detail: fmt.Sprintf(
						"The Form admits this value; this host's Support Profile declares it accepts exactly %s "+
							"for that field. The value is portable, the subset is not.",
						strings.Join(accepted, ", "),
					),
					Repair: "Write one of the values this host accepts, or apply against a host that accepts " +
						written + ".",
				}.error())
			}
		}
		if count, countable := v3PlannedCount(value); countable {
			if ceiling, published := profile.limit(v3MaximumLimitName(field.Wire)); published && count > ceiling {
				resp.Diagnostics.Append(v3Diagnostic{
					Summary:      "This host accepts fewer " + name + " entries than the plan declares",
					ResourceType: r.form.ResourceType,
					Ref:          codec.Ref,
					Pointer:      "/" + field.Wire,
					Attribute:    &attribute,
					Code:         v3CodeLimitExceeded,
					Detail: fmt.Sprintf(
						"The plan declares %d entries and the host publishes the ceiling %s = %d.",
						count, v3MaximumLimitName(field.Wire), ceiling,
					),
					Repair: "Declare at most the ceiling above, or apply against a host that publishes a higher one.",
				}.error())
			}
		}
		if number, isNumber := value.(types.Int64); isNumber {
			minimum, hasMinimum, maximum, hasMaximum := profile.rangeFor(field.Wire)
			written := number.ValueInt64()
			if (hasMinimum && written < minimum) || (hasMaximum && written > maximum) {
				resp.Diagnostics.Append(v3Diagnostic{
					Summary:      "This host does not accept " + name + " = " + fmt.Sprint(written),
					ResourceType: r.form.ResourceType,
					Ref:          codec.Ref,
					Pointer:      "/" + field.Wire,
					Attribute:    &attribute,
					Code:         v3CodeCapabilityUnsupported,
					Detail: "The host's Support Profile declares the inclusive range it accepts for that field: " +
						v3RangeText(minimum, hasMinimum, maximum, hasMaximum) + ".",
					Repair: "Write a value inside that range, or apply against a host that accepts this one.",
				}.error())
			}
		}
	}
}

// checkArtifactCeiling holds the bundle this plan would commit to the host's
// published artifact ceiling, before a single byte is uploaded.
func (r *v3FormResource) checkArtifactCeiling(
	profile v3SupportProfile, codec v3FormCodec, values v3Values, resp *resource.ModifyPlanResponse,
) {
	attributeName := "modules"
	entryLabel := "modules"
	if _, fileArtifact := v3FileBundleManifestKind(r.form.Kind); fileArtifact {
		attributeName = "files"
		entryLabel = "files"
	}
	entries, ok := values.Fields[attributeName].(types.List)
	if !ok || entries.IsNull() || entries.IsUnknown() {
		return
	}
	if ceiling, published := profile.limit("maximumBundleFiles"); published && int64(len(entries.Elements())) > ceiling {
		attribute := path.Root(attributeName)
		resp.Diagnostics.Append(v3Diagnostic{
			Summary:      "This bundle has more files than the host accepts",
			ResourceType: r.form.ResourceType,
			Ref:          codec.Ref,
			Pointer:      "/manifestDigest",
			Attribute:    &attribute,
			Code:         v3CodeLimitExceeded,
			Detail: fmt.Sprintf(
				"The authored %s count %d and the host publishes maximumBundleFiles = %d. The plan refuses before the upload rather than after it, so no bytes are sent.",
				entryLabel, len(entries.Elements()), ceiling,
			),
			Repair: "Reduce the bundle below the file-count ceiling above, or apply against a host that publishes a higher one.",
		}.error())
		return
	}
	ceiling, published := profile.limit("maximumBundleBytes")
	if !published {
		return
	}
	var total int64
	for _, element := range entries.Elements() {
		object, isObject := element.(types.Object)
		if !isObject || object.IsNull() || object.IsUnknown() {
			return
		}
		size, isSize := object.Attributes()["size"].(types.Int64)
		if !isSize || size.IsNull() || size.IsUnknown() {
			return
		}
		total += size.ValueInt64()
	}
	if total <= ceiling {
		return
	}
	attribute := path.Root(attributeName)
	resp.Diagnostics.Append(v3Diagnostic{
		Summary:      "This bundle is larger than the host accepts",
		ResourceType: r.form.ResourceType,
		Ref:          codec.Ref,
		Pointer:      "/manifestDigest",
		Attribute:    &attribute,
		Code:         v3CodeLimitExceeded,
		Detail: fmt.Sprintf(
			"The authored %s total %d bytes and the host publishes maximumBundleBytes = %d. "+
				"The plan refuses before the upload rather than after it, so no bytes are sent.",
			entryLabel, total, ceiling,
		),
		Repair: "Reduce the bundle below the ceiling above, or apply against a host that publishes a higher one.",
	}.error())
}

// v3MaximumLimitName is the published ceiling that bounds one collection field:
// `handlers` is bounded by `maximumHandlers`. The convention is the profile
// schema's own property grammar (lowerCamelCase), so a host publishing a limit
// for a field needs no client change to be honoured.
func v3MaximumLimitName(wire string) string {
	if wire == "" {
		return ""
	}
	return "maximum" + strings.ToUpper(wire[:1]) + wire[1:]
}

func v3RangeText(minimum int64, hasMinimum bool, maximum int64, hasMaximum bool) string {
	switch {
	case hasMinimum && hasMaximum:
		return fmt.Sprintf("%d to %d", minimum, maximum)
	case hasMinimum:
		return fmt.Sprintf("%d or more", minimum)
	case hasMaximum:
		return fmt.Sprintf("%d or less", maximum)
	}
	return "no bound"
}

// v3PlannedStrings lists the string values one planned attribute carries: the
// value itself for a string, every member for a set.
func v3PlannedStrings(value attr.Value) []string {
	switch typed := value.(type) {
	case types.String:
		return []string{typed.ValueString()}
	case types.Set:
		out := make([]string, 0, len(typed.Elements()))
		for _, element := range typed.Elements() {
			if text, ok := element.(types.String); ok && !text.IsNull() && !text.IsUnknown() {
				out = append(out, text.ValueString())
			}
		}
		sort.Strings(out)
		return out
	}
	return nil
}

// v3PlannedCount is the cardinality of one planned collection attribute.
func v3PlannedCount(value attr.Value) (int64, bool) {
	switch typed := value.(type) {
	case types.Set:
		return int64(len(typed.Elements())), true
	case types.List:
		return int64(len(typed.Elements())), true
	}
	return 0, false
}

func v3Contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
