package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
)

func requestedFormTransition(values formValues, diags *diag.Diagnostics) string {
	if values.FormTransition.IsNull() {
		return ""
	}
	if values.FormTransition.IsUnknown() {
		diags.AddAttributeError(
			path.Root("form_transition"),
			"Unknown Form transition",
			"form_transition must be wholly known before the provider can choose a lifecycle protocol; no host request was made.",
		)
		return ""
	}
	marker := strings.TrimSpace(values.FormTransition.ValueString())
	if marker == "" {
		return ""
	}
	if marker != relationalDatabaseV2ToV3Transition {
		diags.AddAttributeError(
			path.Root("form_transition"),
			"Unsupported Form transition",
			fmt.Sprintf(
				"form_transition accepts only %q. No generic Form identity change is available and no host request was made.",
				relationalDatabaseV2ToV3Transition,
			),
		)
		return ""
	}
	return marker
}

func relationalDatabaseTransitionCodecs() (formCodec, formCodec, error) {
	fromRef, err := formregistry.ForKindVersion("RelationalDatabase", "2.0.0")
	if err != nil {
		return formCodec{}, formCodec{}, err
	}
	toRef, err := formregistry.ForKind("RelationalDatabase")
	if err != nil {
		return formCodec{}, formCodec{}, err
	}
	toInstalled := installedFormReference(toRef)
	from, err := providerExactFormCodec(installedFormReference(fromRef))
	if err != nil {
		return formCodec{}, formCodec{}, err
	}
	to, err := providerExactFormCodec(toInstalled)
	if err != nil {
		return formCodec{}, formCodec{}, err
	}
	if from.kind.DefinitionVersion != "2.0.0" || to.kind.DefinitionVersion != "3.0.0" {
		return formCodec{}, formCodec{}, fmt.Errorf("takoform: compiled database transition pair is not DB2-to-DB3")
	}
	return from, to, nil
}

func installedFormReference(ref formregistry.Ref) client.InstalledFormReference {
	return client.InstalledFormReference{
		FormRef: client.FormRef{
			APIVersion: ref.APIVersion, Kind: ref.Kind,
			DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
		},
		PackageDigest: ref.PackageDigest,
	}
}

func (r *formResource) transitionDatabaseForm(
	ctx context.Context,
	values formValues,
	stateCodec formCodec,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	from, to, err := relationalDatabaseTransitionCodecs()
	if err != nil {
		diags.AddError("Database Form transition is unavailable", err.Error())
		return
	}
	if stateCodec.form != from.form {
		diags.AddAttributeError(
			path.Root("form_transition"),
			"Form transition does not match recorded state",
			fmt.Sprintf(
				"%q is limited to exact %s -> %s, but state records %s. No host request was made.",
				relationalDatabaseV2ToV3Transition,
				formatFormIdentity(from.form),
				formatFormIdentity(to.form),
				formatFormIdentity(stateCodec.form),
			),
		)
		return
	}
	body, space, bodyDiags := r.toResourceWithCodec(ctx, values, to)
	diags.Append(bodyDiags...)
	if diags.HasError() {
		return
	}
	body.Form = &to.form
	body.Metadata.ResourceVersion = values.ResourceVersion.ValueString()
	evidence, err := client.NewFormTransitionEvidence(
		relationalDatabaseV2ToV3Transition,
		from.form,
		to.form,
	)
	if err != nil {
		diags.AddError("Failed to bind Form transition evidence", err.Error())
		return
	}
	request, err := client.NewFormTransitionRequest(
		from.form,
		to.form,
		*body,
		client.FormTransitionExpected{ResourceVersion: body.Metadata.ResourceVersion},
		evidence,
	)
	if err != nil {
		diags.AddError("Failed to bind Form transition request", err.Error())
		return
	}
	r.data.serviceFormMutate.Lock()
	defer r.data.serviceFormMutate.Unlock()
	response, err := r.data.client.TransitionResourceForm(ctx, request)
	if err != nil {
		diags.AddError(
			"Failed to prove RelationalDatabase Form transition",
			err.Error()+". Terraform state retains its exact DB2 Form identity until committed same-resource proof is returned.",
		)
		return
	}
	diags.Append(r.setStateWithCodec(
		ctx,
		state,
		body.Metadata.Name,
		body.Spec,
		response.Resource,
		space,
		values,
		false,
		to,
	)...)
}
