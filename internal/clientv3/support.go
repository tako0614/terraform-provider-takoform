package clientv3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// SupportProfileAPIVersion is the closed identity of every Host Support
// Profile (spec/schemas/host-support-profile-v1alpha1.schema.json).
const SupportProfileAPIVersion = "support.takoform.com/v1alpha1"

var supportProfileKinds = map[string]struct{}{
	"FormSupport": {}, "InterfaceSupport": {}, "BindingSupport": {},
}

func validateSupportProfile(profile map[string]any) error {
	apiVersion, _ := profile["apiVersion"].(string)
	if apiVersion != SupportProfileAPIVersion {
		return fmt.Errorf("takoform: support profile apiVersion must be %s", SupportProfileAPIVersion)
	}
	kind, _ := profile["kind"].(string)
	if _, known := supportProfileKinds[kind]; !known {
		return fmt.Errorf("takoform: support profile kind %q is not a closed profile kind", kind)
	}
	return nil
}

// ListFormSupport lists every Form support profile the host declares.
// Profiles are loosely typed; only the closed apiVersion/kind identity is
// enforced. Price, SKU, region, quota, and commercial policy never appear
// in this surface.
func (c *Client) ListFormSupport(ctx context.Context) ([]map[string]any, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	fullURL := c.apiBase + "/support/forms"
	_, _, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := decodeBody(data, fullURL, &response); err != nil {
		return nil, err
	}
	if response.Profiles == nil {
		return nil, errors.New("takoform: host support response omitted profiles")
	}
	for _, profile := range response.Profiles {
		if err := validateSupportProfile(profile); err != nil {
			return nil, err
		}
	}
	return response.Profiles, nil
}

// GetFormSupport reads the support profile for one exact FormRef line
// (group, kind, definitionVersion). When the profile names a formRef it must
// match the request; when that formRef also carries a schemaDigest it must be
// the exact definition the caller asked about, so a profile describing a
// different immutable definition of the same line fails closed instead of
// being read as an answer.
func (c *Client) GetFormSupport(ctx context.Context, ref FormRef) (map[string]any, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := ValidateFormRef(ref); err != nil {
		return nil, err
	}
	fullURL := fmt.Sprintf(
		"%s/support/forms/%s/%s/%s",
		c.apiBase,
		groupPathSegments(ref.APIVersion),
		url.PathEscape(ref.Kind),
		url.PathEscape(ref.DefinitionVersion),
	)
	_, _, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var profile map[string]any
	if err := decodeBody(data, fullURL, &profile); err != nil {
		return nil, err
	}
	if err := validateSupportProfile(profile); err != nil {
		return nil, err
	}
	if rawRef, present := profile["formRef"].(map[string]any); present {
		got := FormRef{}
		got.APIVersion, _ = rawRef["apiVersion"].(string)
		got.Kind, _ = rawRef["kind"].(string)
		got.DefinitionVersion, _ = rawRef["definitionVersion"].(string)
		got.SchemaDigest, _ = rawRef["schemaDigest"].(string)
		if got.APIVersion != ref.APIVersion || got.Kind != ref.Kind || got.DefinitionVersion != ref.DefinitionVersion {
			return nil, errors.New("takoform: host support profile names a different FormRef line")
		}
		// The line path (group/kind/definitionVersion) does not name the exact
		// definition. A profile that volunteers a schemaDigest is claiming which
		// immutable definition it describes, and that claim must be the request's.
		if got.SchemaDigest != "" && got.SchemaDigest != ref.SchemaDigest {
			return nil, errors.New("takoform: host support profile names a different exact Form schemaDigest")
		}
	}
	return profile, nil
}
