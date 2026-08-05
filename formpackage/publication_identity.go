package formpackage

import (
	"encoding/base32"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	legacyPackageArtifactPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$`)
	currentPackageArtifactPattern = regexp.MustCompile(`^sha256-[0-9a-f]{64}$`)
)

// PublicationLocator is the immutable repository/Git locator for one exact
// package artifact. ArtifactID is a compatibility locator, not Form maturity.
type PublicationLocator struct {
	APIVersion string `json:"apiVersion"`
	ReleaseID  string `json:"releaseId"`
	ArtifactID string `json:"artifactId"`
	Tag        string `json:"tag"`
	SourcePath string `json:"sourcePath"`
}

// PublicationLocatorFor derives the only canonical locator for an already
// verified package index and digest. v1alpha1 retains its immutable SemVer
// locator; v1alpha2 and v1alpha3 use the package digest and forbid a second
// version clock.
func PublicationLocatorFor(index PackageIndex, packageDigest string) (PublicationLocator, error) {
	if index.Kind != PackageKind || index.FormRef.Kind == "" || !ValidDigest(packageDigest) {
		return PublicationLocator{}, fmt.Errorf("package publication identity requires FormPackage, FormRef kind, and canonical package digest")
	}
	releaseID := ReleaseIDForKind(index.FormRef.Kind)
	switch index.APIVersion {
	case PackageAPIVersion:
		if index.PackageVersion == "" {
			return PublicationLocator{}, fmt.Errorf("v1alpha1 package publication identity requires packageVersion")
		}
		return PublicationLocator{
			APIVersion: PackageAPIVersion, ReleaseID: releaseID, ArtifactID: index.PackageVersion,
			Tag:        "forms/" + releaseID + "/v" + index.PackageVersion,
			SourcePath: path.Join("forms", "releases", releaseID, index.PackageVersion),
		}, nil
	case LegacyContentAddressedPackageAPIVersion, CurrentPackageAPIVersion:
		if index.PackageVersion != "" {
			return PublicationLocator{}, fmt.Errorf("content-addressed package publication identity forbids packageVersion")
		}
		artifactID := strings.Replace(packageDigest, ":", "-", 1)
		return PublicationLocator{
			APIVersion: index.APIVersion, ReleaseID: releaseID, ArtifactID: artifactID,
			Tag:        "forms/" + releaseID + "/" + artifactID,
			SourcePath: path.Join("forms", "releases", releaseID, artifactID),
		}, nil
	default:
		return PublicationLocator{}, fmt.Errorf("unsupported package apiVersion %q", index.APIVersion)
	}
}

// ParsePublicationTag recognizes the two immutable publication locator
// profiles without interpreting either one as Form maturity.
func ParsePublicationTag(tag string) (PublicationLocator, error) {
	parts := strings.Split(tag, "/")
	if len(parts) != 3 || parts[0] != "forms" {
		return PublicationLocator{}, fmt.Errorf("Form Package tag %q is not a canonical publication locator", tag)
	}
	releaseID := parts[1]
	if _, err := KindFromReleaseID(releaseID); err != nil {
		return PublicationLocator{}, err
	}
	segment := parts[2]
	if strings.HasPrefix(segment, "v") && legacyPackageArtifactPattern.MatchString(strings.TrimPrefix(segment, "v")) {
		artifactID := strings.TrimPrefix(segment, "v")
		return PublicationLocator{
			APIVersion: PackageAPIVersion, ReleaseID: releaseID, ArtifactID: artifactID,
			Tag: tag, SourcePath: path.Join("forms", "releases", releaseID, artifactID),
		}, nil
	}
	if currentPackageArtifactPattern.MatchString(segment) {
		return PublicationLocator{
			APIVersion: CurrentPackageAPIVersion, ReleaseID: releaseID, ArtifactID: segment,
			Tag: tag, SourcePath: path.Join("forms", "releases", releaseID, segment),
		}, nil
	}
	return PublicationLocator{}, fmt.Errorf("Form Package tag %q has an unsupported artifact locator", tag)
}

// ReleaseIDForKind is a reversible path-safe encoding of the exact Form kind.
func ReleaseIDForKind(kind string) string {
	return "k-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind)))
}

// KindFromReleaseID reverses and canonicalizes a path-safe release ID.
func KindFromReleaseID(releaseID string) (string, error) {
	if !strings.HasPrefix(releaseID, "k-") {
		return "", fmt.Errorf("release id %q is outside k-<lowercase-base32-kind>", releaseID)
	}
	encoded := strings.TrimPrefix(releaseID, "k-")
	if encoded == "" || encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("release id %q is not canonical lowercase base32", releaseID)
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encoded))
	if err != nil {
		return "", fmt.Errorf("decode release id %q: %w", releaseID, err)
	}
	kind := string(raw)
	if kind == "" || ReleaseIDForKind(kind) != releaseID {
		return "", fmt.Errorf("release id %q is not the canonical encoding of a Form kind", releaseID)
	}
	return kind, nil
}
