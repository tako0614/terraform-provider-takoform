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
// locator; v1alpha2, v1alpha3, and v1alpha4 use the package digest and forbid
// a second version clock. v1alpha4 (Form Family) release IDs encode the
// namespaced group together with the kind so a family kind can never collide
// with a frozen central-epoch kind of the same name.
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
	case LegacyContentAddressedPackageAPIVersion, CurrentPackageAPIVersion, FamilyPackageAPIVersion, VersionlessFamilyPackageAPIVersion:
		if index.PackageVersion != "" {
			return PublicationLocator{}, fmt.Errorf("content-addressed package publication identity forbids packageVersion")
		}
		if FamilyPackageLane(index.APIVersion) {
			if !NamespacedFormGroup(index.FormRef.APIVersion) {
				return PublicationLocator{}, fmt.Errorf("family package publication identity requires a namespaced Form group, not %q", index.FormRef.APIVersion)
			}
			releaseID = ReleaseIDForGroupKind(index.FormRef.APIVersion, index.FormRef.Kind)
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

// ParsePublicationTag recognizes the three immutable publication locator
// profiles without interpreting any of them as Form maturity: the retained
// v1alpha1 SemVer lane, the content-addressed provider-v2 v1alpha3 lane, and
// the content-addressed Form Family v1alpha4 lane.
//
// The two content-addressed lanes share an artifact grammar, so the release ID
// is what tells them apart: a family release ID decodes to "<group>/<Kind>"
// (ReleaseIDForGroupKind) while a central one decodes to a bare Kind, and the
// Kind grammar admits no "/", so any decoded value containing one is a family
// locator. Without that distinction every Edge Family tag decoded as the
// retained v1alpha3 lane and nothing in the family could ever be published.
func ParsePublicationTag(tag string) (PublicationLocator, error) {
	parts := strings.Split(tag, "/")
	if len(parts) != 3 || parts[0] != "forms" {
		return PublicationLocator{}, fmt.Errorf("Form Package tag %q is not a canonical publication locator", tag)
	}
	releaseID := parts[1]
	decoded, err := KindFromReleaseID(releaseID)
	if err != nil {
		return PublicationLocator{}, err
	}
	family := strings.Contains(decoded, "/")
	segment := parts[2]
	if strings.HasPrefix(segment, "v") && legacyPackageArtifactPattern.MatchString(strings.TrimPrefix(segment, "v")) {
		if family {
			return PublicationLocator{}, fmt.Errorf(
				"Form Package tag %q carries a Form Family release id with a retained SemVer artifact locator", tag)
		}
		artifactID := strings.TrimPrefix(segment, "v")
		return PublicationLocator{
			APIVersion: PackageAPIVersion, ReleaseID: releaseID, ArtifactID: artifactID,
			Tag: tag, SourcePath: path.Join("forms", "releases", releaseID, artifactID),
		}, nil
	}
	if currentPackageArtifactPattern.MatchString(segment) {
		apiVersion := CurrentPackageAPIVersion
		if family {
			// The decoded release id is "<group>/<Kind>", so the group is
			// recoverable here, and which family lane a tag belongs to is
			// exactly whether that group carries a version segment: a
			// versionless one cannot be described by the v1alpha4 index
			// schema, whose FormRef reference requires one (decision 0049).
			apiVersion = FamilyPackageAPIVersion
			if !strings.Contains(strings.TrimSuffix(decoded, "/"+path.Base(decoded)), "/") {
				apiVersion = VersionlessFamilyPackageAPIVersion
			}
		}
		return PublicationLocator{
			APIVersion: apiVersion, ReleaseID: releaseID, ArtifactID: segment,
			Tag: tag, SourcePath: path.Join("forms", "releases", releaseID, segment),
		}, nil
	}
	return PublicationLocator{}, fmt.Errorf("Form Package tag %q has an unsupported artifact locator", tag)
}

// FamilyPackageLane reports whether a package index apiVersion is one of the
// Form Family lanes, which are distinguished from each other only by whether
// the family group carries a version segment (decision 0049).
func FamilyPackageLane(apiVersion string) bool {
	return apiVersion == FamilyPackageAPIVersion || apiVersion == VersionlessFamilyPackageAPIVersion
}

// FamilyReleaseID reports whether a release ID encodes the v1alpha4 Form
// Family identity "<group>/<Kind>" rather than a bare central Kind, and
// returns the two components when it does. It is the one place that knows the
// separator rule stated on ReleaseIDForGroupKind.
func FamilyReleaseID(releaseID string) (group, kind string, family bool) {
	decoded, err := KindFromReleaseID(releaseID)
	if err != nil {
		return "", "", false
	}
	index := strings.LastIndex(decoded, "/")
	if index < 0 {
		return "", "", false
	}
	return decoded[:index], decoded[index+1:], true
}

// ReleaseIDForKind is a reversible path-safe encoding of the exact Form kind.
func ReleaseIDForKind(kind string) string {
	return "k-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind)))
}

// ReleaseIDForGroupKind is the v1alpha4 release identity: it encodes the
// namespaced Form group (the FormRef apiVersion) together with the kind, so
// "edge.forms.takoform.com/v1beta1 ObjectBucket" and the frozen central
// "ObjectBucket" own different release lines.
//
// The joined string contains MORE than one "/": a Form group is itself
// "<dns-name>/<groupVersion>", so "edge.forms.takoform.com/v1beta1" plus
// "ObjectBucket" joins to a value with two separators. The separator is
// therefore not unique and never was. What makes the encoding unambiguous is
// the Kind grammar (^[A-Z][A-Za-z0-9]{0,63}$, spec/schemas/form-ref-*.json),
// which admits no "/" at all:
//
//   - the LAST "/" of a decoded release ID always splits the group from the
//     kind (see FamilyReleaseID), and
//   - a decoded release ID containing any "/" is a family locator, because a
//     bare central Kind can never contain one.
//
// KindFromReleaseID round-trips the joined string verbatim, so neither the
// encoding nor the split loses information.
func ReleaseIDForGroupKind(group, kind string) string {
	return ReleaseIDForKind(group + "/" + kind)
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
