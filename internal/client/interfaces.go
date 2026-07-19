package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const FeatureInterfaceDeclarations = "interface_declarations"

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
	Name     string                  `json:"name"`
	Version  string                  `json:"version"`
	Resource InterfaceResourceRef    `json:"resource"`
	Document map[string]any          `json:"document"`
	Values   map[string]any          `json:"values,omitempty"`
	Form     *InstalledFormReference `json:"form,omitempty"`
}

func (c *Client) SupportsInterfaceDeclarations() bool {
	return c.interfacesURL != ""
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
	if declared.Form != nil {
		if err := validateInstalledFormReference(declared.Form.FormRef.Kind, *declared.Form); err != nil {
			return fmt.Errorf("takoform: host returned an interface with invalid Form identity: %w", err)
		}
	}
	return nil
}
