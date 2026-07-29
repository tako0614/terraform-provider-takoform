package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

const FeatureInterfaceDeclarations = "interface_declarations"

var (
	interfaceNamePattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	interfaceVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	resourceKindPattern     = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
	resourceNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// ValidateResourceName enforces the canonical portable PatternName grammar.
// Resource names are stable wire identities, so callers must not trim,
// normalize, or case-fold them.
func ValidateResourceName(value string) error {
	if !resourceNamePattern.MatchString(value) {
		return errors.New("Resource name must match ^[a-z][a-z0-9-]{0,62}$")
	}
	return nil
}

var (
	ErrInterfaceDeclarationsUnsupported = errors.New("takoform: host does not advertise features.interface_declarations")
	ErrInterfaceIdentityAmbiguous       = errors.New("takoform: interface name resolves to multiple versions")
	ErrInterfaceInstanceAmbiguous       = errors.New("takoform: interface identity resolves to multiple resource instances")
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
	Name        string                  `json:"name"`
	Version     string                  `json:"version"`
	Resource    InterfaceResourceRef    `json:"resource"`
	Document    map[string]any          `json:"document"`
	Values      map[string]any          `json:"values"`
	ResourceURI string                  `json:"resourceUri,omitempty"`
	Form        *InstalledFormReference `json:"form,omitempty"`

	resourceURIPresent bool
}

// UnmarshalJSON retains the distinction between an omitted optional
// resourceUri and a present empty string. The latter is not a valid URI and
// must fail closed instead of collapsing into the zero value.
func (declared *DeclaredInterface) UnmarshalJSON(raw []byte) error {
	type wire DeclaredInterface
	var decoded wire
	if err := decodeStrictJSON(raw, &decoded); err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return err
	}
	*declared = DeclaredInterface(decoded)
	_, declared.resourceURIPresent = members["resourceUri"]
	return nil
}

func (c *Client) SupportsInterfaceDeclarations() bool {
	return c.interfacesURL != ""
}

// ListInterfaces reads all declarations visible to the caller in a space.
func (c *Client) ListInterfaces(ctx context.Context, space string) ([]DeclaredInterface, error) {
	if !c.SupportsInterfaceDeclarations() {
		return nil, ErrInterfaceDeclarationsUnsupported
	}
	if err := ValidateSpaceID(space); err != nil {
		return nil, fmt.Errorf("takoform: interface read requires a space with a valid SpaceID: %w", err)
	}
	target := c.interfacesURL + "?" + spaceQuery(space).Encode()
	var response struct {
		Interfaces *[]DeclaredInterface `json:"interfaces"`
	}
	if err := c.doJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return nil, err
	}
	if response.Interfaces == nil {
		return nil, errors.New("takoform: host returned an Interface list without the required interfaces array")
	}
	seen := make(map[string]struct{}, len(*response.Interfaces))
	for _, declared := range *response.Interfaces {
		if err := validateDeclaredInterfaceIdentity(declared); err != nil {
			return nil, err
		}
		key := declared.Resource.Kind + "\x00" + declared.Resource.Name + "\x00" + declared.Name + "\x00" + declared.Version
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("takoform: host returned duplicate runtime interface identity %s/%s %q@%q", declared.Resource.Kind, declared.Resource.Name, declared.Name, declared.Version)
		}
		seen[key] = struct{}{}
	}
	return *response.Interfaces, nil
}

// GetInterface reads one runtime declaration. Descriptor identity is
// (name, version); runtime identity additionally includes the space-scoped
// Resource (kind, name). Omitted selector components are accepted only when
// the visible result is unique, then followed by an exact re-read.
func (c *Client) GetInterface(ctx context.Context, space string, selector InterfaceSelector) (DeclaredInterface, error) {
	if !c.SupportsInterfaceDeclarations() {
		return DeclaredInterface{}, ErrInterfaceDeclarationsUnsupported
	}
	if err := ValidateSpaceID(space); err != nil {
		return DeclaredInterface{}, fmt.Errorf("takoform: interface read requires a space with a valid SpaceID: %w", err)
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
	query.Set("version", selector.Version)
	query.Set("resourceKind", selector.ResourceKind)
	query.Set("resourceName", selector.ResourceName)
	target := fmt.Sprintf("%s/%s?%s", c.interfacesURL, url.PathEscape(selector.Name), query.Encode())
	var declared DeclaredInterface
	if err := c.doJSON(ctx, http.MethodGet, target, nil, &declared); err != nil {
		if isResourceNotFound(err) {
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

func validateDeclaredInterfaceIdentity(declared DeclaredInterface) error {
	if len(declared.Name) > 128 || !interfaceNamePattern.MatchString(declared.Name) ||
		!interfaceVersionPattern.MatchString(declared.Version) ||
		!resourceKindPattern.MatchString(declared.Resource.Kind) ||
		ValidateResourceName(declared.Resource.Name) != nil {
		return errors.New("takoform: host returned an interface without exact descriptor and resource identity")
	}
	if declared.Document == nil {
		return errors.New("takoform: host returned an interface without the exact declared document")
	}
	if declared.Values == nil {
		return errors.New("takoform: host returned an interface without the exact resolved values")
	}
	if err := formpackage.ValidatePortableData(declared.Document); err != nil {
		return fmt.Errorf("takoform: host returned a forbidden interface document: %w", err)
	}
	if err := formpackage.ValidatePortableData(declared.Values); err != nil {
		return fmt.Errorf("takoform: host returned forbidden interface values: %w", err)
	}
	if declared.resourceURIPresent || declared.ResourceURI != "" {
		if !formcatalog.ValidCredentialFreeHTTPSURL(declared.ResourceURI) {
			return errors.New("takoform: host returned an invalid credential-free HTTPS resourceUri")
		}
	}
	if declared.Form != nil {
		if err := validateInstalledFormReference(declared.Resource.Kind, *declared.Form); err != nil {
			return fmt.Errorf("takoform: host returned an interface with invalid Form identity: %w", err)
		}
	}
	return nil
}
