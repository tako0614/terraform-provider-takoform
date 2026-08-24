package clientv3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// SupportProfileAPIVersion is the closed identity of every stable Host Support
// Profile this lane reads (spec/schemas/host-support-profile-v1.schema.json).
// The stable profile carries the exact Form, Interface, Binding, and opaque
// StandardService support vocabulary for the stable Host API.
const SupportProfileAPIVersion = "support.takoform.com/v1"

var supportProfileKinds = map[string]struct{}{
	"FormSupport": {}, "InterfaceSupport": {}, "BindingSupport": {},
	// Decision 0045's profile kind. It is a support profile like the other
	// three and carries the same identity, which is the whole reason it can be
	// read by a client that knows nothing about standard services yet.
	"StandardServiceSupport": {},
}

var (
	supportProfileNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.][a-z][a-z0-9]*)*$`)
	supportProfileBindingName  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z][a-z0-9]*)*$`)
	supportProfileSemver       = regexp.MustCompile(`^(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)$`)
	supportProfilePhasePattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
)

var supportProfileOperations = map[string]struct{}{
	"create": {}, "read": {}, "update": {}, "delete": {}, "import": {}, "observe": {},
}

var supportProfileKeys = map[string]struct{}{
	"apiVersion": {}, "kind": {}, "formRef": {}, "interfaceRef": {}, "bindingRef": {},
	"operations": {}, "supportedEnums": {}, "supportedRanges": {}, "supportedBindings": {},
	"limits": {}, "serviceRef": {}, "satisfiable": {},
}

func validateSupportProfile(profile map[string]any) error {
	for key := range profile {
		if _, ok := supportProfileKeys[key]; !ok {
			return fmt.Errorf("takoform: support profile contains unknown field %q", key)
		}
	}
	apiVersion, _ := profile["apiVersion"].(string)
	if apiVersion != SupportProfileAPIVersion {
		return fmt.Errorf("takoform: support profile apiVersion must be %s", SupportProfileAPIVersion)
	}
	kind, _ := profile["kind"].(string)
	if _, known := supportProfileKinds[kind]; !known {
		return fmt.Errorf("takoform: support profile kind %q is not a closed profile kind", kind)
	}
	if err := validateSupportProfileRefs(profile, kind); err != nil {
		return err
	}
	if err := validateSupportProfileOperations(profile); err != nil {
		return err
	}
	if err := validateSupportProfileCapabilities(profile); err != nil {
		return err
	}
	return nil
}

func validateSupportProfileRefs(profile map[string]any, kind string) error {
	for _, entry := range []struct {
		key, label string
		required   bool
	}{
		{"formRef", "formRef", kind == "FormSupport"},
		{"interfaceRef", "interfaceRef", kind == "InterfaceSupport"},
		{"bindingRef", "bindingRef", kind == "BindingSupport"},
		{"serviceRef", "serviceRef", kind == "StandardServiceSupport"},
	} {
		value, present := profile[entry.key]
		if entry.required && !present {
			return fmt.Errorf("takoform: %s profile omits %s", kind, entry.label)
		}
		if !entry.required && present {
			return fmt.Errorf("takoform: %s profile must not contain %s", kind, entry.label)
		}
		if !present {
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("takoform: %s profile %s must be an object", kind, entry.label)
		}
		switch entry.key {
		case "formRef":
			if err := validateSupportFormRef(object); err != nil {
				return err
			}
		case "interfaceRef":
			if err := validateSupportInterfaceRef(object); err != nil {
				return err
			}
		case "bindingRef":
			if err := validateSupportBindingRef(object); err != nil {
				return err
			}
		case "serviceRef":
			if err := validateSupportServiceRef(object); err != nil {
				return err
			}
		}
	}
	if kind == "StandardServiceSupport" {
		if _, present := profile["operations"]; present {
			return errors.New("takoform: StandardServiceSupport profile must not contain operations")
		}
		value, ok := profile["satisfiable"]
		if !ok {
			return errors.New("takoform: StandardServiceSupport profile omits satisfiable")
		}
		if _, ok := value.(bool); !ok {
			return errors.New("takoform: StandardServiceSupport satisfiable must be boolean")
		}
	} else if _, present := profile["satisfiable"]; present {
		return fmt.Errorf("takoform: %s profile must not contain satisfiable", kind)
	}
	return nil
}

func validateSupportFormRef(value map[string]any) error {
	if err := exactObjectKeys(value, "apiVersion", "kind", "definitionVersion", "schemaDigest"); err != nil {
		return fmt.Errorf("takoform: support formRef: %w", err)
	}
	ref := FormRef{}
	ref.APIVersion, _ = value["apiVersion"].(string)
	ref.Kind, _ = value["kind"].(string)
	ref.DefinitionVersion, _ = value["definitionVersion"].(string)
	ref.SchemaDigest, _ = value["schemaDigest"].(string)
	if err := ValidateFormRef(ref); err != nil {
		return fmt.Errorf("takoform: support formRef: %w", err)
	}
	return nil
}

func validateSupportInterfaceRef(value map[string]any) error {
	if err := exactObjectKeys(value, "apiVersion", "name", "version", "schemaDigest"); err != nil {
		return fmt.Errorf("takoform: support interfaceRef: %w", err)
	}
	apiVersion, _ := value["apiVersion"].(string)
	name, _ := value["name"].(string)
	version, _ := value["version"].(string)
	digest, _ := value["schemaDigest"].(string)
	if apiVersion != "interfaces.takoform.com/v1alpha1" || len(name) > 128 || !supportProfileNamePattern.MatchString(name) ||
		!supportProfileSemver.MatchString(version) || !formpackage.ValidDigest(digest) {
		return errors.New("takoform: support interfaceRef is not an exact v1alpha1 InterfaceRef")
	}
	return nil
}

func validateSupportBindingRef(value map[string]any) error {
	if err := exactObjectKeys(value, "apiVersion", "name", "version", "schemaDigest"); err != nil {
		return fmt.Errorf("takoform: support bindingRef: %w", err)
	}
	apiVersion, _ := value["apiVersion"].(string)
	name, _ := value["name"].(string)
	version, _ := value["version"].(string)
	digest, _ := value["schemaDigest"].(string)
	if apiVersion != "bindings.takoform.com/v1alpha2" || len(name) > 128 || !supportProfileBindingName.MatchString(name) ||
		!supportProfileSemver.MatchString(version) || !formpackage.ValidDigest(digest) {
		return errors.New("takoform: support bindingRef is not an exact v1alpha2 BindingRef")
	}
	return nil
}

func validateSupportServiceRef(value map[string]any) error {
	if err := exactObjectKeys(value, "apiVersion", "protocol"); err != nil {
		return fmt.Errorf("takoform: support serviceRef: %w", err)
	}
	ref := formpackage.StandardServiceRef{}
	ref.APIVersion, _ = value["apiVersion"].(string)
	ref.Protocol, _ = value["protocol"].(string)
	if err := formpackage.ValidateStandardServiceRef(ref); err != nil {
		return fmt.Errorf("takoform: support serviceRef: %w", err)
	}
	return nil
}

func exactObjectKeys(value map[string]any, required ...string) error {
	want := make(map[string]struct{}, len(required))
	for _, key := range required {
		want[key] = struct{}{}
		if _, ok := value[key]; !ok {
			return fmt.Errorf("missing required field %q", key)
		}
	}
	for key := range value {
		if _, ok := want[key]; !ok {
			return fmt.Errorf("contains unknown field %q", key)
		}
	}
	return nil
}

func validateSupportProfileOperations(profile map[string]any) error {
	value, present := profile["operations"]
	if !present {
		if profile["kind"] == "FormSupport" {
			return errors.New("takoform: FormSupport profile omits operations")
		}
		return nil
	}
	operations, ok := value.([]any)
	if !ok {
		return errors.New("takoform: support profile operations must be an array")
	}
	seen := map[string]struct{}{}
	for _, raw := range operations {
		operation, ok := raw.(string)
		if !ok {
			return errors.New("takoform: support profile operations must contain strings")
		}
		if _, known := supportProfileOperations[operation]; !known {
			return fmt.Errorf("takoform: support profile operation %q is not in the closed vocabulary", operation)
		}
		if _, duplicate := seen[operation]; duplicate {
			return fmt.Errorf("takoform: support profile operations contains duplicate %q", operation)
		}
		seen[operation] = struct{}{}
	}
	return nil
}

func validateSupportProfileCapabilities(profile map[string]any) error {
	if value, present := profile["supportedEnums"]; present {
		object, ok := value.(map[string]any)
		if !ok {
			return errors.New("takoform: support profile supportedEnums must be an object")
		}
		for pointer, raw := range object {
			if err := validateSupportPointer(pointer); err != nil {
				return fmt.Errorf("takoform: supportedEnums key %q: %w", pointer, err)
			}
			values, ok := raw.([]any)
			if !ok || len(values) == 0 {
				return fmt.Errorf("takoform: supportedEnums[%q] must be a non-empty string array", pointer)
			}
			seen := map[string]struct{}{}
			for _, entry := range values {
				stringValue, ok := entry.(string)
				if !ok || utf8.RuneCountInString(stringValue) > 128 {
					return fmt.Errorf("takoform: supportedEnums[%q] contains an invalid value", pointer)
				}
				if _, duplicate := seen[stringValue]; duplicate {
					return fmt.Errorf("takoform: supportedEnums[%q] contains duplicate %q", pointer, stringValue)
				}
				seen[stringValue] = struct{}{}
			}
		}
	}
	if value, present := profile["supportedRanges"]; present {
		object, ok := value.(map[string]any)
		if !ok {
			return errors.New("takoform: support profile supportedRanges must be an object")
		}
		for pointer, raw := range object {
			if err := validateSupportPointer(pointer); err != nil {
				return fmt.Errorf("takoform: supportedRanges key %q: %w", pointer, err)
			}
			bounds, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("takoform: supportedRanges[%q] must be an object", pointer)
			}
			for key, bound := range bounds {
				if key != "minimum" && key != "maximum" {
					return fmt.Errorf("takoform: supportedRanges[%q] contains unknown bound %q", pointer, key)
				}
				if !isSupportRangeValue(bound) {
					return fmt.Errorf("takoform: supportedRanges[%q].%s must be an integer or string", pointer, key)
				}
			}
		}
	}
	if value, present := profile["supportedBindings"]; present {
		entries, ok := value.([]any)
		if !ok || len(entries) > 256 {
			return errors.New("takoform: support profile supportedBindings must contain at most 256 strings")
		}
		seen := map[string]struct{}{}
		for _, raw := range entries {
			entry, ok := raw.(string)
			if !ok || len(entry) > 256 || !supportProfileBindingNameVersion.MatchString(entry) {
				return fmt.Errorf("takoform: support profile supportedBindings contains invalid binding %v", raw)
			}
			if _, duplicate := seen[entry]; duplicate {
				return fmt.Errorf("takoform: support profile supportedBindings contains duplicate %q", entry)
			}
			seen[entry] = struct{}{}
		}
	}
	if value, present := profile["limits"]; present {
		object, ok := value.(map[string]any)
		if !ok {
			return errors.New("takoform: support profile limits must be an object")
		}
		for pointer, raw := range object {
			if err := validateSupportPointer(pointer); err != nil {
				return fmt.Errorf("takoform: limits key %q: %w", pointer, err)
			}
			number, ok := raw.(json.Number)
			if !ok {
				return fmt.Errorf("takoform: limits[%q] must be an integer", pointer)
			}
			value, err := strconv.ParseInt(string(number), 10, 64)
			if err != nil || value < 0 || value > 1<<53-1 {
				return fmt.Errorf("takoform: limits[%q] must be an integer between 0 and 2^53-1", pointer)
			}
		}
	}
	return nil
}

var supportProfileBindingNameVersion = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z][a-z0-9]*)*@(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)$`)

func validateSupportPointer(pointer string) error {
	if utf8.RuneCountInString(pointer) > 512 || !strings.HasPrefix(pointer, "/") || strings.ContainsRune(pointer, '\x00') {
		return errors.New("must be a JSON Pointer with at most 512 characters")
	}
	return nil
}

func isSupportRangeValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := strconv.ParseInt(string(number), 10, 64)
	return err == nil
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
	if len(response.Profiles) > 1024 {
		return nil, errors.New("takoform: host support response contains more than 1024 profiles")
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

// GetStandardServiceSupport reads the stable Host-owned support decision for
// one opaque StandardServiceRef. A grammar-valid protocol may be unknown to
// Takoform and still receives a 200 profile with satisfiable=false; that is a
// support decision, not a missing registry entry.
func (c *Client) GetStandardServiceSupport(ctx context.Context, protocol string) (map[string]any, error) {
	ref := formpackage.StandardServiceRef{
		APIVersion: formpackage.StandardServiceAPIVersion,
		Protocol:   protocol,
	}
	if err := formpackage.ValidateStandardServiceRef(ref); err != nil {
		return nil, err
	}
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	fullURL := fmt.Sprintf("%s/support/standard-services/%s", c.apiBase, url.PathEscape(protocol))
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
	if _, err := formpackage.ValidateStandardServiceSupport(ref, profile); err != nil {
		return nil, err
	}
	return profile, nil
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
