package standardforms

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// ReleasePlanPath records how each declared Form is released.
const ReleasePlanPath = "forms/release-plan.json"

const releasePlanFormat = "takoform.release-plan@v1"

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:[-+][0-9A-Za-z.-]+)?$`)

// ReleasePlan is the exact, derived list of per-Form releases this source can
// produce.
//
// Forms carry independent versions, so there is no single set version to
// release them under: each Form is released on its own tag, from its own
// reviewed release source, and is published or not published independently of
// every other Form. This plan is what an operator dispatches from; it exists
// so nobody has to hand-derive a release identifier or guess a version.
type ReleasePlan struct {
	Format     string               `json:"format"`
	Generation string               `json:"generation"`
	Repository string               `json:"repository"`
	Note       string               `json:"note"`
	Releases   []PlannedFormRelease `json:"releases"`
}

// PlannedFormRelease is one Form's release identity.
type PlannedFormRelease struct {
	Kind          string              `json:"kind"`
	Slug          string              `json:"slug"`
	ReleaseID     string              `json:"releaseId"`
	Version       string              `json:"version"`
	Tag           string              `json:"tag"`
	SourcePath    string              `json:"sourcePath"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

func generateReleasePlan(root string, entries []InventoryEntry) error {
	plan, err := buildReleasePlan(root, entries)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), plan)
}

func buildReleasePlan(root string, entries []InventoryEntry) (ReleasePlan, error) {
	releases := make([]PlannedFormRelease, 0, len(entries))
	for _, entry := range entries {
		version := entry.FormRef.DefinitionVersion
		if !semverPattern.MatchString(version) {
			return ReleasePlan{}, fmt.Errorf("%s version %q is not SemVer", entry.Kind, version)
		}
		releaseID := releaseIDForKind(entry.Kind)
		sourcePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseID, version))
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(sourcePath)))
		if err != nil {
			return ReleasePlan{}, fmt.Errorf("%s release source: %w", entry.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return ReleasePlan{}, fmt.Errorf("%s release source identity differs from the reviewed candidate", entry.Kind)
		}
		releases = append(releases, PlannedFormRelease{
			Kind: entry.Kind, Slug: filepath.Base(entry.Path), ReleaseID: releaseID,
			Version: version, Tag: "forms/" + releaseID + "/v" + version, SourcePath: sourcePath,
			FormRef: report.FormRef, PackageDigest: report.PackageDigest,
		})
	}
	return ReleasePlan{
		Format: releasePlanFormat, Generation: portableGeneration, Repository: publishRepository,
		Note: "Each Form is released on its own tag from its own reviewed source. " +
			"A release proves published bytes only; admission stays external.",
		Releases: releases,
	}, nil
}

const publishRepository = "tako0614/terraform-provider-takoform"

// VerifyReleasePlan proves the committed plan still describes exactly the
// declared Forms and their reviewed release sources.
//
// It grants nothing. A planned release is a tag that could be created, not a
// release that exists, and creating one publishes bytes without admitting a
// Form.
func VerifyReleasePlan(root string) error {
	var committed ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &committed); err != nil {
		return err
	}
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		return err
	}
	expected, err := buildReleasePlan(root, inventory.Packages)
	if err != nil {
		return err
	}
	if committed.Format != expected.Format || committed.Generation != expected.Generation ||
		committed.Repository != expected.Repository || len(committed.Releases) != len(expected.Releases) {
		return fmt.Errorf("release plan identity or closure drifted from the declared Form set")
	}
	byKind := make(map[string]PlannedFormRelease, len(expected.Releases))
	for _, release := range expected.Releases {
		byKind[release.Kind] = release
	}
	seenTags := make(map[string]struct{}, len(committed.Releases))
	for _, release := range committed.Releases {
		want, ok := byKind[release.Kind]
		if !ok || release != want {
			return fmt.Errorf("release plan entry for %s drifted", release.Kind)
		}
		if _, duplicate := seenTags[release.Tag]; duplicate {
			return fmt.Errorf("release plan reuses tag %s", release.Tag)
		}
		seenTags[release.Tag] = struct{}{}
	}
	return nil
}

// RetiredReleaseTags lists the tags the retired generation already owns. A
// planned tag may never collide with one: published bytes are immutable, so a
// reused tag would either fail or overwrite a proof.
func RetiredReleaseTags() []string {
	tags := make([]string, 0, len(RetiredKinds))
	for _, spec := range RetiredKinds {
		tags = append(tags, "forms/"+releaseIDForKind(spec.Kind)+"/v"+spec.Version)
	}
	return tags
}

// RenderReleasePlan prints the dispatch list an operator works from.
func RenderReleasePlan(root string) (string, error) {
	if err := VerifyReleasePlan(root); err != nil {
		return "", err
	}
	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		return "", err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s: %d Form releases, each independent\n\n", plan.Generation, len(plan.Releases))
	for _, release := range plan.Releases {
		fmt.Fprintf(&builder, "%-28s %s\n  source %s\n  digest %s\n",
			release.Kind, release.Tag, release.SourcePath, release.PackageDigest)
	}
	builder.WriteString("\nDispatch the Release Form Package workflow once per tag, after that exact\n" +
		"tag exists on the reviewed commit. Publishing proves bytes only: admission\n" +
		"still requires a conforming host's signed lifecycle report.\n")
	return builder.String(), nil
}
