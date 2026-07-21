package composition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

var (
	nameRE      = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	versionRE   = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	interfaceRE = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	commitIDRE  = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// Verify parses a closed Composition Manifest and returns its RFC 8785 digest.
// Unknown fields fail closed so a future authority-like field cannot be
// silently accepted by an older host.
func Verify(raw []byte) (Manifest, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, "", fmt.Errorf("decode composition manifest: %w", err)
	}
	if decoder.More() {
		return Manifest{}, "", fmt.Errorf("composition manifest has trailing JSON values")
	}
	if err := validate(manifest); err != nil {
		return Manifest{}, "", err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("canonicalize composition manifest: %w", err)
	}
	digest, err := formpackage.DigestCanonicalJSON(canonical)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("digest composition manifest: %w", err)
	}
	return manifest, digest, nil
}

func validate(manifest Manifest) error {
	if manifest.APIVersion != APIVersion || manifest.Kind != Kind {
		return fmt.Errorf("composition manifest must be %s/%s", APIVersion, Kind)
	}
	if !nameRE.MatchString(manifest.Metadata.Name) {
		return fmt.Errorf("metadata.name must be a lowercase composition token")
	}
	if !versionRE.MatchString(manifest.Metadata.Version) {
		return fmt.Errorf("metadata.version must be SemVer")
	}
	if strings.TrimSpace(manifest.Metadata.Title) == "" {
		return fmt.Errorf("metadata.title is required")
	}
	if len(manifest.Components) == 0 || len(manifest.Components) > 32 {
		return fmt.Errorf("components must contain between 1 and 32 entries")
	}
	components := make(map[string]struct{}, len(manifest.Components))
	for _, component := range manifest.Components {
		if !nameRE.MatchString(component.ID) {
			return fmt.Errorf("component.id must be a lowercase composition token")
		}
		if _, exists := components[component.ID]; exists {
			return fmt.Errorf("duplicate component id %q", component.ID)
		}
		components[component.ID] = struct{}{}
		if strings.TrimSpace(component.Title) == "" {
			return fmt.Errorf("component %q title is required", component.ID)
		}
		if err := validateSource(component.Source); err != nil {
			return fmt.Errorf("component %q: %w", component.ID, err)
		}
	}
	for _, connection := range manifest.Connections {
		if _, exists := components[connection.From.Component]; !exists {
			return fmt.Errorf("connection source component %q is unknown", connection.From.Component)
		}
		if _, exists := components[connection.To.Component]; !exists {
			return fmt.Errorf("connection destination component %q is unknown", connection.To.Component)
		}
		if connection.From.Component == connection.To.Component {
			return fmt.Errorf("connection must span distinct components")
		}
		if !interfaceRE.MatchString(connection.From.Interface) || !interfaceRE.MatchString(connection.To.Interface) {
			return fmt.Errorf("connection interface names must be portable tokens")
		}
	}
	return nil
}

func validateSource(source Source) error {
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("source.url must be a credential-free https URL")
	}
	if !commitIDRE.MatchString(source.Ref) {
		return fmt.Errorf("source.ref must be a full lowercase Git commit object ID")
	}
	if source.Path == "" {
		return fmt.Errorf("source.path is required")
	}
	if path.IsAbs(source.Path) || strings.Contains(source.Path, "\\") || strings.Contains(source.Path, "\x00") || path.Clean(source.Path) != source.Path || strings.HasPrefix(source.Path, "../") || source.Path == ".." {
		return fmt.Errorf("source.path must be a safe relative module path")
	}
	return nil
}
