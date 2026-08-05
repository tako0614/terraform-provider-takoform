package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// artifactSourceValues is the shared typed-provider projection of the immutable
// artifact contract used by every artifact-backed Form.
type artifactSourceValues struct {
	URL       types.String
	SHA256    types.String
	MediaType types.String
}

func (v artifactSourceValues) toSpec(owner string) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	for _, field := range []struct {
		name  string
		value types.String
	}{
		{name: "artifact_url", value: v.URL},
		{name: "artifact_sha256", value: v.SHA256},
		{name: "artifact_media_type", value: v.MediaType},
	} {
		if field.value.IsUnknown() {
			diags.AddAttributeError(
				path.Root(field.name),
				"Unknown "+owner+" artifact field",
				field.name+" must be wholly known before the provider can preview or apply this Form.",
			)
		}
	}
	if diags.HasError() {
		return nil, diags
	}
	artifactURL := knownTrimmedString(v.URL)
	artifactSHA256 := knownTrimmedString(v.SHA256)
	artifactMediaType := knownTrimmedString(v.MediaType)

	if artifactURL == "" {
		diags.AddAttributeError(
			path.Root("artifact_url"),
			"Missing "+owner+" artifact URL",
			"artifact_url is required and must identify portable HTTPS bytes.",
		)
		return nil, diags
	}
	if !validCredentialFreeArtifactURL(artifactURL) {
		diags.AddAttributeError(
			path.Root("artifact_url"),
			"Invalid "+owner+" artifact URL",
			"artifact_url must be an absolute credential-free https URL without userinfo, query, or fragment.",
		)
		return nil, diags
	}
	if artifactSHA256 == "" {
		diags.AddAttributeError(
			path.Root("artifact_sha256"),
			"Missing "+owner+" artifact digest",
			"artifact_sha256 is required to bind artifact_url to exact bytes.",
		)
		return nil, diags
	}
	if !artifactSHA256Pattern.MatchString(artifactSHA256) {
		diags.AddAttributeError(
			path.Root("artifact_sha256"),
			"Invalid "+owner+" artifact digest",
			formcatalog.GrammarCanonicalSHA256.Message("artifact_sha256"),
		)
		return nil, diags
	}
	if !artifactMediaTypePattern.MatchString(artifactMediaType) {
		diags.AddAttributeError(
			path.Root("artifact_media_type"),
			"Invalid "+owner+" artifact media type",
			"artifact_media_type must be a lowercase type/subtype token without parameters.",
		)
		return nil, diags
	}

	return map[string]any{
		"artifactUrl":       artifactURL,
		"artifactSha256":    artifactSHA256,
		"artifactMediaType": artifactMediaType,
	}, diags
}

func artifactSourceValuesFromSpec(raw any) artifactSourceValues {
	source, ok := raw.(map[string]any)
	if !ok {
		return nullArtifactSourceValues()
	}
	return artifactSourceValues{
		URL:       optionalStringFromAny(source["artifactUrl"]),
		SHA256:    optionalStringFromAny(source["artifactSha256"]),
		MediaType: optionalStringFromAny(source["artifactMediaType"]),
	}
}

func nullArtifactSourceValues() artifactSourceValues {
	return artifactSourceValues{
		URL: types.StringNull(), SHA256: types.StringNull(), MediaType: types.StringNull(),
	}
}

func optionalStringFromAny(value any) types.String {
	if text, ok := value.(string); ok && text != "" {
		return types.StringValue(text)
	}
	return types.StringNull()
}

func knownTrimmedString(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return strings.TrimSpace(value.ValueString())
}
