package standardforms

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// ReleasePlanPath records the immutable pre-reset publication plan.
const ReleasePlanPath = "forms/release-plan.json"

const releasePlanFormat = "takoform.release-plan@v1"

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:[-+][0-9A-Za-z.-]+)?$`)

// ReleasePlan is the exact, derived list of pre-reset per-Form releases.
//
// Forms carry independent versions, so there is no single set version to
// release them under: each Form is released on its own tag, from its own
// reviewed release source, and is published or not published independently of
// every other Form. All entries are now published Legacy evidence. This type
// remains only to verify the retained plan and must not drive a new release.
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
		Note:     "Historical pre-reset plan. Every listed tag is published immutable Legacy evidence and authorizes no new release.",
		Releases: releases,
	}, nil
}

const publishRepository = "tako0614/terraform-provider-takoform"

// VerifyReleasePlan proves the committed historical plan still describes
// exactly the Legacy Forms and their retained release sources.
//
// It grants nothing and authorizes no release. Every tag in the plan is
// already published and immutable.
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

// RenderReleasePlan renders the non-actionable historical publication inventory.
func RenderReleasePlan(root string) (string, error) {
	if err := VerifyReleasePlan(root); err != nil {
		return "", err
	}
	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		return "", err
	}
	return renderReleasePlan(plan), nil
}

func renderReleasePlan(plan ReleasePlan) string {
	var builder strings.Builder
	noun := "releases"
	if len(plan.Releases) == 1 {
		noun = "release"
	}
	fmt.Fprintf(&builder, "%s: %d published Legacy Form %s\n\n", plan.Generation, len(plan.Releases), noun)
	builder.WriteString("This is an immutable historical publication plan. Every listed tag is already\n" +
		"published. Do not prepare, publish, move, delete, or reuse these tags.\n\n")
	for _, release := range plan.Releases {
		fmt.Fprintf(
			&builder,
			"%-28s %s\n  source %s\n  digest %s\n",
			release.Kind, release.Tag, release.SourcePath, release.PackageDigest,
		)
	}
	builder.WriteString("\nPublication proves these exact historical bytes. It grants no current Form\n" +
		"maturity, Host Support, activation, placement, or commercial authority.\n")
	return builder.String()
}
