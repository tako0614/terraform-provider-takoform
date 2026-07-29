package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	FeatureInterfaceDeclarations      = "interface_declarations"
	FeatureInterfaceDeclarationWrites = "interface_declaration_writes"
)

var (
	ErrInterfaceDeclarationsUnsupported      = errors.New("takoform: host does not advertise features.interface_declarations")
	ErrInterfaceDeclarationWritesUnsupported = errors.New("takoform: host does not advertise features.interface_declaration_writes")
	ErrInterfaceIdentityAmbiguous            = errors.New("takoform: interface name resolves to multiple versions")
	ErrInterfaceInstanceAmbiguous            = errors.New("takoform: interface identity resolves to multiple resource instances")
)

type InterfaceResourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type InterfaceSelector struct {
	Name         string
	Version      string
	ResourceKind string
	ResourceName string
}

// DeclaredInterface is one read-only runtime declaration reported by a host.
// Identity is the exact (Name, Version) pair. Document and Values are
// non-secret data; presence implies no consumer authorization.
type DeclaredInterface struct {
	Name             string                                  `json:"name"`
	Version          string                                  `json:"version"`
	Resource         InterfaceResourceRef                    `json:"resource"`
	Document         map[string]any                          `json:"document"`
	DocumentSchema   map[string]any                          `json:"documentSchema,omitempty"`
	Inputs           []formpackage.InterfaceInputDeclaration `json:"inputs,omitempty"`
	ResourceURIInput string                                  `json:"resourceUriInput,omitempty"`
	Values           map[string]any                          `json:"values,omitempty"`
	ResourceURI      string                                  `json:"resourceUri,omitempty"`
	ResourceVersion  string                                  `json:"resourceVersion,omitempty"`
	Form             *InstalledFormReference                 `json:"form,omitempty"`
}

func (c *Client) SupportsInterfaceDeclarations() bool {
	return c.interfacesURL != ""
}

func (c *Client) SupportsInterfaceDeclarationWrites() bool {
	return c.SupportsInterfaceDeclarations() &&
		c.Discovery.HasFeature(FeatureInterfaceDeclarationWrites)
}

// ListInterfaces reads all declarations visible to the caller in a space.
func (c *Client) ListInterfaces(ctx context.Context, space string) ([]DeclaredInterface, error) {
	if !c.SupportsInterfaceDeclarations() {
		return nil, ErrInterfaceDeclarationsUnsupported
	}
	target := c.interfacesURL
	if query := spaceQuery(space); len(query) > 0 {
		target += "?" + query.Encode()
	}
	var response struct {
		Interfaces []DeclaredInterface `json:"interfaces"`
	}
	if err := c.doJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(response.Interfaces))
	for _, declared := range response.Interfaces {
		if err := validateDeclaredInterfaceIdentity(declared); err != nil {
			return nil, err
		}
		key := declared.Resource.Kind + "\x00" + declared.Resource.Name + "\x00" + declared.Name + "\x00" + declared.Version
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("takoform: host returned duplicate runtime interface identity %s/%s %q@%q", declared.Resource.Kind, declared.Resource.Name, declared.Name, declared.Version)
		}
		seen[key] = struct{}{}
	}
	return response.Interfaces, nil
}

// GetInterface reads one runtime declaration. Descriptor identity is
// (name, version); runtime identity additionally includes the space-scoped
// Resource (kind, name). Omitted selector components are accepted only when
// the visible result is unique, then followed by an exact re-read.
func (c *Client) GetInterface(ctx context.Context, space string, selector InterfaceSelector) (DeclaredInterface, error) {
	if !c.SupportsInterfaceDeclarations() {
		return DeclaredInterface{}, ErrInterfaceDeclarationsUnsupported
	}
	if strings.TrimSpace(selector.Name) == "" {
		return DeclaredInterface{}, errors.New("takoform: interface name is required")
	}
	if (selector.ResourceKind == "") != (selector.ResourceName == "") {
		return DeclaredInterface{}, errors.New("takoform: resource kind and resource name must be provided together")
	}
	if selector.Version != "" && strings.TrimSpace(selector.Version) == "" {
		return DeclaredInterface{}, errors.New("takoform: interface version must be a non-empty token when provided")
	}

	if selector.Version == "" || selector.ResourceKind == "" {
		declared, err := c.ListInterfaces(ctx, space)
		if err != nil {
			return DeclaredInterface{}, err
		}
		matches := make([]DeclaredInterface, 0, 1)
		for _, candidate := range declared {
			if candidate.Name != selector.Name || (selector.Version != "" && candidate.Version != selector.Version) {
				continue
			}
			if selector.ResourceKind != "" && (candidate.Resource.Kind != selector.ResourceKind || candidate.Resource.Name != selector.ResourceName) {
				continue
			}
			matches = append(matches, candidate)
		}
		if len(matches) == 0 {
			return DeclaredInterface{}, ErrNotFound
		}
		if selector.Version == "" {
			versions := map[string]struct{}{}
			for _, candidate := range matches {
				versions[candidate.Version] = struct{}{}
			}
			if len(versions) > 1 {
				ordered := make([]string, 0, len(versions))
				for version := range versions {
					ordered = append(ordered, version)
				}
				sort.Strings(ordered)
				return DeclaredInterface{}, fmt.Errorf("%w: %q has versions %s", ErrInterfaceIdentityAmbiguous, selector.Name, strings.Join(ordered, ", "))
			}
		}
		if len(matches) > 1 {
			resources := make([]string, 0, len(matches))
			for _, candidate := range matches {
				resources = append(resources, candidate.Resource.Kind+"/"+candidate.Resource.Name)
			}
			sort.Strings(resources)
			return DeclaredInterface{}, fmt.Errorf("%w: %q@%q is exposed by %s", ErrInterfaceInstanceAmbiguous, matches[0].Name, matches[0].Version, strings.Join(resources, ", "))
		}
		exact := InterfaceSelector{
			Name: matches[0].Name, Version: matches[0].Version,
			ResourceKind: matches[0].Resource.Kind, ResourceName: matches[0].Resource.Name,
		}
		return c.GetInterface(ctx, space, exact)
	}

	query := spaceQuery(space)
	if query == nil {
		query = url.Values{}
	}
	query.Set("version", selector.Version)
	query.Set("resourceKind", selector.ResourceKind)
	query.Set("resourceName", selector.ResourceName)
	target := fmt.Sprintf("%s/%s?%s", c.interfacesURL, url.PathEscape(selector.Name), query.Encode())
	var declared DeclaredInterface
	if err := c.doJSON(ctx, http.MethodGet, target, nil, &declared); err != nil {
		if code, ok := statusCode(err); ok && code == http.StatusNotFound {
			return DeclaredInterface{}, ErrNotFound
		}
		return DeclaredInterface{}, err
	}
	if err := validateDeclaredInterfaceIdentity(declared); err != nil {
		return DeclaredInterface{}, err
	}
	if declared.Name != selector.Name || declared.Version != selector.Version ||
		declared.Resource.Kind != selector.ResourceKind || declared.Resource.Name != selector.ResourceName {
		return DeclaredInterface{}, fmt.Errorf(
			"takoform: host returned interface %s/%s %q@%q for requested %s/%s %q@%q",
			declared.Resource.Kind, declared.Resource.Name, declared.Name, declared.Version,
			selector.ResourceKind, selector.ResourceName, selector.Name, selector.Version,
		)
	}
	return declared, nil
}

// PutInterface creates or updates one exact generic declaration. The provider
// carries application-authored data only; a host owns authorization, bindings,
// value resolution, and the canonical record.
func (c *Client) PutInterface(ctx context.Context, space string, desired DeclaredInterface) (DeclaredInterface, error) {
	if !c.SupportsInterfaceDeclarationWrites() {
		return DeclaredInterface{}, ErrInterfaceDeclarationWritesUnsupported
	}
	if strings.TrimSpace(space) == "" {
		return DeclaredInterface{}, errors.New("takoform: interface declaration write requires a space")
	}
	if err := validateWritableInterface(desired); err != nil {
		return DeclaredInterface{}, err
	}
	target := c.interfaceURL(space, InterfaceSelector{
		Name: desired.Name, Version: desired.Version,
		ResourceKind: desired.Resource.Kind, ResourceName: desired.Resource.Name,
	})
	headers := map[string]string{
		"Idempotency-Key": interfaceMutationKey("apply", space, desired),
	}
	if desired.ResourceVersion == "" {
		headers["If-None-Match"] = "*"
	} else {
		if !validResourceVersion(desired.ResourceVersion) {
			return DeclaredInterface{}, errors.New("takoform: interface resourceVersion is invalid")
		}
		headers["If-Match"] = quoteResourceVersion(desired.ResourceVersion)
	}
	transport := desired
	transport.Values = nil
	transport.ResourceURI = ""
	transport.Form = nil
	var declared DeclaredInterface
	responseHeaders, err := c.doJSONWithHeaders(ctx, http.MethodPut, target, headers, &transport, &declared, true)
	if err != nil {
		return DeclaredInterface{}, err
	}
	if err := verifyInterfaceResponse(desired, declared); err != nil {
		return DeclaredInterface{}, err
	}
	if err := captureInterfaceResourceVersion(&declared, responseHeaders); err != nil {
		return DeclaredInterface{}, err
	}
	return declared, nil
}

// DeleteInterface removes one exact declaration. Missing is already deleted.
func (c *Client) DeleteInterface(ctx context.Context, space string, selector InterfaceSelector, resourceVersion string) error {
	if !c.SupportsInterfaceDeclarationWrites() {
		return ErrInterfaceDeclarationWritesUnsupported
	}
	if strings.TrimSpace(space) == "" {
		return errors.New("takoform: interface declaration delete requires a space")
	}
	if !validResourceVersion(resourceVersion) {
		return errors.New("takoform: interface delete requires a valid resourceVersion")
	}
	if selector.Name == "" || selector.Version == "" || selector.ResourceKind == "" || selector.ResourceName == "" {
		return errors.New("takoform: interface delete requires the exact complete identity")
	}
	headers := map[string]string{
		"If-Match":        quoteResourceVersion(resourceVersion),
		"Idempotency-Key": interfaceMutationKey("delete", space, selector, resourceVersion),
	}
	if _, err := c.doJSONWithHeaders(ctx, http.MethodDelete, c.interfaceURL(space, selector), headers, nil, nil, true); err != nil {
		if code, ok := statusCode(err); ok && code == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) interfaceURL(space string, selector InterfaceSelector) string {
	query := spaceQuery(space)
	if query == nil {
		query = url.Values{}
	}
	query.Set("version", selector.Version)
	query.Set("resourceKind", selector.ResourceKind)
	query.Set("resourceName", selector.ResourceName)
	return fmt.Sprintf("%s/%s?%s", c.interfacesURL, url.PathEscape(selector.Name), query.Encode())
}

func interfaceMutationKey(values ...any) string {
	raw, _ := json.Marshal(values)
	digest := sha256.Sum256(raw)
	return "interface-" + hex.EncodeToString(digest[:])
}

func validateWritableInterface(declared DeclaredInterface) error {
	if declared.ResourceVersion != "" && !validResourceVersion(declared.ResourceVersion) {
		return errors.New("takoform: interface resourceVersion is invalid")
	}
	if err := validateDeclaredInterfaceIdentity(declared); err != nil {
		return err
	}
	if err := formpackage.ValidatePortableData(declared.DocumentSchema); err != nil {
		return fmt.Errorf("takoform: forbidden interface document schema: %w", err)
	}
	if err := validateInterfaceInputs(declared.ResourceURIInput, declared.Inputs); err != nil {
		return err
	}
	return nil
}

func validateInterfaceInputs(resourceURIInput string, inputs []formpackage.InterfaceInputDeclaration) error {
	seen := make(map[string]struct{}, len(inputs))
	resourceURICount := 0
	resourceURIMatch := 0
	for _, input := range inputs {
		if strings.TrimSpace(input.Name) == "" {
			return errors.New("takoform: interface input name is required")
		}
		if _, duplicate := seen[input.Name]; duplicate {
			return fmt.Errorf("takoform: duplicate interface input %q", input.Name)
		}
		seen[input.Name] = struct{}{}
		switch input.Source {
		case formpackage.InterfaceInputSourceLiteral:
			if len(input.Value) == 0 || input.Pointer != "" {
				return fmt.Errorf("takoform: literal interface input %q requires value and forbids pointer", input.Name)
			}
			var value any
			if err := json.Unmarshal(input.Value, &value); err != nil {
				return fmt.Errorf("takoform: literal interface input %q has invalid JSON: %w", input.Name, err)
			}
			if err := formpackage.ValidatePortableData(value); err != nil {
				return fmt.Errorf("takoform: literal interface input %q is forbidden: %w", input.Name, err)
			}
		case formpackage.InterfaceInputSourceOutput:
			if len(input.Value) != 0 {
				return fmt.Errorf("takoform: output interface input %q must not carry value", input.Name)
			}
		case formpackage.InterfaceInputSourceResourceURI:
			resourceURICount++
			if input.Name == resourceURIInput {
				resourceURIMatch++
			}
			if len(input.Value) != 0 || input.Pointer != "" {
				return fmt.Errorf("takoform: resource_uri interface input %q must not carry pointer or value", input.Name)
			}
		default:
			if !strings.Contains(input.Source, ".") || len(input.Value) != 0 {
				return fmt.Errorf("takoform: interface input %q has invalid source %q", input.Name, input.Source)
			}
		}
	}
	if resourceURIInput == "" && resourceURICount != 0 {
		return errors.New("takoform: resource_uri input requires resource_uri_input")
	}
	if resourceURIInput != "" && (resourceURICount != 1 || resourceURIMatch != 1) {
		return errors.New("takoform: resource_uri_input must name the single resource_uri input")
	}
	return nil
}

func verifyInterfaceResponse(want, got DeclaredInterface) error {
	if err := validateDeclaredInterfaceIdentity(got); err != nil {
		return err
	}
	if got.Name != want.Name || got.Version != want.Version ||
		got.Resource.Kind != want.Resource.Kind || got.Resource.Name != want.Resource.Name {
		return errors.New("takoform: host substituted interface identity")
	}
	wantAuthored, _ := json.Marshal(struct {
		Document         map[string]any                          `json:"document"`
		DocumentSchema   map[string]any                          `json:"documentSchema,omitempty"`
		Inputs           []formpackage.InterfaceInputDeclaration `json:"inputs,omitempty"`
		ResourceURIInput string                                  `json:"resourceUriInput,omitempty"`
	}{
		Document:         want.Document,
		DocumentSchema:   want.DocumentSchema,
		Inputs:           want.Inputs,
		ResourceURIInput: want.ResourceURIInput,
	})
	gotAuthored, _ := json.Marshal(struct {
		Document         map[string]any                          `json:"document"`
		DocumentSchema   map[string]any                          `json:"documentSchema,omitempty"`
		Inputs           []formpackage.InterfaceInputDeclaration `json:"inputs,omitempty"`
		ResourceURIInput string                                  `json:"resourceUriInput,omitempty"`
	}{
		Document:         got.Document,
		DocumentSchema:   got.DocumentSchema,
		Inputs:           got.Inputs,
		ResourceURIInput: got.ResourceURIInput,
	})
	if string(wantAuthored) != string(gotAuthored) {
		return errors.New("takoform: host altered the authored interface declaration")
	}
	return nil
}

func captureInterfaceResourceVersion(declared *DeclaredInterface, headers http.Header) error {
	bodyVersion := declared.ResourceVersion
	etag := headers.Get("ETag")
	etagVersion := ""
	if etag != "" {
		if len(etag) < 3 || etag[0] != '"' || etag[len(etag)-1] != '"' {
			return errors.New("takoform: host returned an invalid Interface ETag")
		}
		etagVersion = etag[1 : len(etag)-1]
	}
	if bodyVersion == "" {
		bodyVersion = etagVersion
	}
	if !validResourceVersion(bodyVersion) || (etagVersion != "" && etagVersion != bodyVersion) {
		return errors.New("takoform: host omitted or substituted the interface resourceVersion fence")
	}
	declared.ResourceVersion = bodyVersion
	return nil
}

func validateDeclaredInterfaceIdentity(declared DeclaredInterface) error {
	if strings.TrimSpace(declared.Name) == "" || strings.TrimSpace(declared.Version) == "" ||
		strings.TrimSpace(declared.Resource.Kind) == "" || strings.TrimSpace(declared.Resource.Name) == "" {
		return errors.New("takoform: host returned an interface without exact descriptor and resource identity")
	}
	if declared.Document == nil {
		return errors.New("takoform: host returned an interface without the exact declared document")
	}
	if err := formpackage.ValidatePortableData(declared.Document); err != nil {
		return fmt.Errorf("takoform: host returned a forbidden interface document: %w", err)
	}
	if err := formpackage.ValidatePortableData(declared.Values); err != nil {
		return fmt.Errorf("takoform: host returned forbidden interface values: %w", err)
	}
	if declared.ResourceURI != "" {
		parsed, err := url.Parse(declared.ResourceURI)
		if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" ||
			parsed.User != nil || parsed.Fragment != "" {
			return errors.New("takoform: host returned an invalid credential-free HTTPS resourceUri")
		}
	}
	if declared.Form != nil {
		if err := validateInstalledFormReference(declared.Form.FormRef.Kind, *declared.Form); err != nil {
			return fmt.Errorf("takoform: host returned an interface with invalid Form identity: %w", err)
		}
	}
	return nil
}
