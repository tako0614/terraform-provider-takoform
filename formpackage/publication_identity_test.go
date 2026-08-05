package formpackage

import (
	"strings"
	"testing"
)

func TestPublicationLocatorUsesPackageDigestForCurrentProfile(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	locator, err := PublicationLocatorFor(PackageIndex{
		APIVersion: CurrentPackageAPIVersion,
		Kind:       PackageKind,
		FormRef:    FormRef{Kind: "Example"},
	}, digest)
	if err != nil {
		t.Fatal(err)
	}
	wantArtifact := "sha256-" + strings.Repeat("a", 64)
	if locator.ArtifactID != wantArtifact ||
		locator.Tag != "forms/"+locator.ReleaseID+"/"+wantArtifact ||
		locator.SourcePath != "forms/releases/"+locator.ReleaseID+"/"+wantArtifact {
		t.Fatalf("current locator = %#v", locator)
	}
}

func TestPublicationLocatorRetainsV1Alpha1SemVerIdentity(t *testing.T) {
	t.Parallel()
	locator, err := PublicationLocatorFor(PackageIndex{
		APIVersion:     PackageAPIVersion,
		Kind:           PackageKind,
		PackageVersion: "1.2.3",
		FormRef:        FormRef{Kind: "Example"},
	}, "sha256:"+strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if locator.ArtifactID != "1.2.3" ||
		locator.Tag != "forms/"+locator.ReleaseID+"/v1.2.3" ||
		locator.SourcePath != "forms/releases/"+locator.ReleaseID+"/1.2.3" {
		t.Fatalf("v1alpha1 locator = %#v", locator)
	}
}

func TestPublicationLocatorRejectsMismatchedPackageProfiles(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("c", 64)
	for name, index := range map[string]PackageIndex{
		"v1alpha1 without version": {APIVersion: PackageAPIVersion, Kind: PackageKind, FormRef: FormRef{Kind: "Example"}},
		"current with version":     {APIVersion: CurrentPackageAPIVersion, Kind: PackageKind, PackageVersion: "0.1.0", FormRef: FormRef{Kind: "Example"}},
		"unknown api":              {APIVersion: "packages.forms.takoform.com/v9", Kind: PackageKind, FormRef: FormRef{Kind: "Example"}},
	} {
		name, index := name, index
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := PublicationLocatorFor(index, digest); err == nil {
				t.Fatal("PublicationLocatorFor unexpectedly accepted mismatched package profile")
			}
		})
	}
}

func TestParsePublicationTagRoundTripsBothProfiles(t *testing.T) {
	t.Parallel()
	releaseID := ReleaseIDForKind("Example")
	for name, tag := range map[string]string{
		"Legacy":  "forms/" + releaseID + "/v1.2.3",
		"current": "forms/" + releaseID + "/sha256-" + strings.Repeat("d", 64),
	} {
		name, tag := name, tag
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			locator, err := ParsePublicationTag(tag)
			if err != nil {
				t.Fatal(err)
			}
			if locator.Tag != tag || locator.ReleaseID != releaseID || locator.SourcePath == "" {
				t.Fatalf("parsed locator = %#v", locator)
			}
		})
	}
}

func TestParsePublicationTagRejectsNonCanonicalLocators(t *testing.T) {
	t.Parallel()
	for _, tag := range []string{
		"forms/k-invalid/v1.0.0",
		"forms/" + ReleaseIDForKind("Example") + "/1.0.0",
		"forms/" + ReleaseIDForKind("Example") + "/sha256-" + strings.Repeat("A", 64),
		"forms/" + ReleaseIDForKind("Example") + "/sha256-" + strings.Repeat("a", 63),
	} {
		if _, err := ParsePublicationTag(tag); err == nil {
			t.Fatalf("ParsePublicationTag accepted %q", tag)
		}
	}
}
