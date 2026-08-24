package provider

// v3_environment_namespace.go is the provider's plan-time half of the single
// runtime environment namespace rule
// (spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md).
//
// `vars` keys, sealed-value names, and every typed binding `name` are projected
// into ONE environment object the module receives, so two of them sharing a
// name specifies two different values under one identifier. A conforming host
// refuses it before any mutation; refusing it here as well is what lets the
// author see the mistake in `terraform plan`, against the configuration they
// wrote, instead of after a round trip that names a wire pointer.

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

var _ resource.ResourceWithValidateConfig = (*v3FormResource)(nil)

// ValidateConfig runs the cross-attribute rules no single-attribute validator
// can express. The environment namespace spans up to seven attributes of one
// resource, so it belongs here rather than on any one of them.
func (r *v3FormResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	if req.Config.Raw.IsNull() {
		return
	}
	v3ValidateEnvironmentNamespace(ctx, r.form, req.Config, &resp.Diagnostics)
}

// v3EnvironmentClaim is one declared environment name and the exact
// configuration location that declared it.
type v3EnvironmentClaim struct {
	Name string
	Path path.Path
}

// v3ValidateEnvironmentNamespace reports every name claimed twice across the
// Form's environment-projecting attributes.
//
// An unknown or absent value is skipped rather than guessed: a name that is not
// known at plan time cannot be proved to collide, and the host re-proves the
// whole rule before it mutates anything.
func v3ValidateEnvironmentNamespace(
	ctx context.Context,
	form model.Form,
	config v3AttributeGetter,
	diags *diag.Diagnostics,
) {
	claimed := map[string]path.Path{}
	for _, entry := range form.EnvironmentNameFields() {
		for _, claim := range v3EnvironmentClaims(ctx, entry, config, diags) {
			previous, taken := claimed[claim.Name]
			if !taken {
				claimed[claim.Name] = claim.Path
				continue
			}
			diags.AddAttributeError(
				claim.Path,
				"Duplicate module environment name",
				"The name "+claim.Name+" is already declared at "+previous.String()+". "+
					"vars, required_sensitive_vars, every typed binding, and every sealed standard-service "+
					"binding project into one module "+
					"environment namespace, so a name belongs to exactly one of them. A conforming "+
					"host refuses this configuration before it mutates anything.",
			)
		}
	}
}

// v3EnvironmentClaims reads the names one environment-projecting attribute
// declares in the configuration.
func v3EnvironmentClaims(
	ctx context.Context,
	entry model.EnvironmentNameField,
	config v3AttributeGetter,
	diags *diag.Diagnostics,
) []v3EnvironmentClaim {
	attribute := path.Root(v3AttributeName(entry.Field))
	switch entry.Source {
	case model.EnvironmentBindingNames, model.EnvironmentExternalServiceNames:
		var list types.List
		if readDiags := config.GetAttribute(ctx, attribute, &list); readDiags.HasError() {
			diags.Append(readDiags...)
			return nil
		}
		if list.IsNull() || list.IsUnknown() {
			return nil
		}
		var out []v3EnvironmentClaim
		for index, element := range list.Elements() {
			object, ok := element.(types.Object)
			if !ok || object.IsNull() || object.IsUnknown() {
				continue
			}
			name, ok := object.Attributes()["name"].(types.String)
			if !ok || name.IsNull() || name.IsUnknown() {
				continue
			}
			out = append(out, v3EnvironmentClaim{
				Name: name.ValueString(),
				Path: attribute.AtListIndex(index).AtName("name"),
			})
		}
		return out
	case model.EnvironmentMapKeys:
		var text types.String
		if readDiags := config.GetAttribute(ctx, attribute, &text); readDiags.HasError() {
			diags.Append(readDiags...)
			return nil
		}
		if text.IsNull() || text.IsUnknown() {
			return nil
		}
		// A malformed document is already reported by the attribute's own JSON
		// validator; this rule stays silent about it rather than repeating it.
		parsed, err := v3ParseJSONObject(text.ValueString())
		if err != nil {
			return nil
		}
		keys := make([]string, 0, len(parsed))
		for key := range parsed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]v3EnvironmentClaim, 0, len(keys))
		for _, key := range keys {
			out = append(out, v3EnvironmentClaim{Name: key, Path: attribute})
		}
		return out
	case model.EnvironmentSetItems:
		var set types.Set
		if readDiags := config.GetAttribute(ctx, attribute, &set); readDiags.HasError() {
			diags.Append(readDiags...)
			return nil
		}
		if set.IsNull() || set.IsUnknown() {
			return nil
		}
		var out []v3EnvironmentClaim
		for _, element := range set.Elements() {
			member, ok := element.(types.String)
			if !ok || member.IsNull() || member.IsUnknown() {
				continue
			}
			out = append(out, v3EnvironmentClaim{Name: member.ValueString(), Path: attribute})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	default:
		return nil
	}
}
