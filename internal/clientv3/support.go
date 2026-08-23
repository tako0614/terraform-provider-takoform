package clientv3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SupportProfileAPIVersion is the closed identity of every Host Support
// Profile this lane reads
// (spec/schemas/host-support-profile-v1alpha2.schema.json). It moved with the
// lane: a v1beta2 host answers v1alpha2 profiles, and a client still
// demanding v1alpha1 would refuse a conforming host before any plan could
// complete.
const SupportProfileAPIVersion = "support.takoform.com/v1alpha2"

var supportProfileKinds = map[string]struct{}{
	"FormSupport": {}, "InterfaceSupport": {}, "BindingSupport": {},
	// Decision 0045's profile kind. It is a support profile like the other
	// three and carries the same identity, which is the whole reason it can be
	// read by a client that knows nothing about standard services yet.
	"StandardServiceSupport": {},
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

// GetInterfaceSupport reads the support profile of one exact Interface
// contract line. A host that implements the contract answers with an
// InterfaceSupport profile naming it; a host that does not answers 404, which
// is the fact a client needs before it plans a resource whose Form requires
// that contract.
func (c *Client) GetInterfaceSupport(ctx context.Context, name, version string) (map[string]any, error) {
	return c.contractSupport(ctx, "interfaces", "InterfaceSupport", "interfaceRef", name, version)
}

// GetBindingSupport reads the support profile of one exact Binding contract
// line, on the same terms.
func (c *Client) GetBindingSupport(ctx context.Context, name, version string) (map[string]any, error) {
	return c.contractSupport(ctx, "bindings", "BindingSupport", "bindingRef", name, version)
}

// contractSupport is the shared read of the two contract support routes. The
// answer must be the closed profile kind for the route that was asked, and it
// must name the contract line the caller asked about: a profile describing a
// different contract is a fail-closed protocol fault, not an answer.
func (c *Client) contractSupport(ctx context.Context, route, kind, refKey, name, version string) (map[string]any, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return nil, errors.New("takoform: contract support requires a name and a version")
	}
	fullURL := fmt.Sprintf("%s/support/%s/%s/%s", c.apiBase, route, url.PathEscape(name), url.PathEscape(version))
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
	if got, _ := profile["kind"].(string); got != kind {
		return nil, fmt.Errorf("takoform: host support profile kind %q is not %s", got, kind)
	}
	reference, present := profile[refKey].(map[string]any)
	if !present {
		return nil, fmt.Errorf("takoform: %s profile omits %s", kind, refKey)
	}
	gotName, _ := reference["name"].(string)
	gotVersion, _ := reference["version"].(string)
	if gotName != name || gotVersion != version {
		return nil, errors.New("takoform: host support profile names a different contract line")
	}
	return profile, nil
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
