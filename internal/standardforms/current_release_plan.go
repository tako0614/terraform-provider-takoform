package standardforms

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const currentReleasePlanFormat = "takoform.current-form-release-plan@v2"

type CurrentFormReleasePlan struct {
	Format     string                       `json:"format"`
	Repository string                       `json:"repository"`
	Releases   []CurrentFormReleaseIdentity `json:"releases"`
}

type CurrentFormReleaseIdentity struct {
	ProposalID    string              `json:"proposalId"`
	State         string              `json:"state"`
	Kind          string              `json:"kind"`
	ReleaseID     string              `json:"releaseId"`
	ArtifactID    string              `json:"artifactId"`
	Tag           string              `json:"tag"`
	SourcePath    string              `json:"sourcePath"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

// CurrentReleasePlan derives exact publishable identities only from the
// post-reset lifecycle authority. It never imports the retained portable-v1
// catalog or historical admission subset. Publication remains an external
// operator action and must independently reject an already-existing tag.
func CurrentReleasePlan(root string) (CurrentFormReleasePlan, error) {
	authority, err := readProjectLifecycleAuthority(root)
	if err != nil {
		return CurrentFormReleasePlan{}, err
	}
	identities, err := currentLifecycleReleaseIdentities(root, authority)
	if err != nil {
		return CurrentFormReleasePlan{}, err
	}
	plan := CurrentFormReleasePlan{
		Format:     currentReleasePlanFormat,
		Repository: "tako0614/terraform-provider-takoform",
		Releases:   make([]CurrentFormReleaseIdentity, 0, len(identities)),
	}
	for _, identity := range identities {
		if identity.State != "legacy" {
			plan.Releases = append(plan.Releases, identity)
		}
	}
	return plan, nil
}

// currentLifecycleReleaseIdentities reconstructs every post-reset package
// identity, including a Form that later transitioned to Legacy. New
// publication uses CurrentReleasePlan and therefore excludes Legacy, while
// immutable tag verification and forward recovery still need the original
// exact identity after such a transition.
func currentLifecycleReleaseIdentities(root string, authority projectLifecycleAuthority) ([]CurrentFormReleaseIdentity, error) {
	identities := make([]CurrentFormReleaseIdentity, 0, len(authority.CurrentForms))
	seenTags := make(map[string]struct{}, len(authority.CurrentForms))
	for index, record := range authority.CurrentForms {
		indexPath := filepath.Join(root, filepath.FromSlash(record.PackagePath), formpackage.PackageIndexFilename)
		indexRaw, err := os.ReadFile(indexPath)
		if err != nil {
			return nil, fmt.Errorf("currentForms[%d] package index: %w", index, err)
		}
		packageIndex, err := formpackage.ValidatePackageIndex(indexRaw)
		if err != nil {
			return nil, fmt.Errorf("currentForms[%d] package index: %w", index, err)
		}
		locator, err := formpackage.PublicationLocatorFor(packageIndex, record.PackageDigest)
		if err != nil {
			return nil, fmt.Errorf("currentForms[%d] publication identity: %w", index, err)
		}
		if packageIndex.APIVersion != formpackage.CurrentPackageAPIVersion {
			return nil, fmt.Errorf("currentForms[%d] package uses retained %s; current Forms require %s", index, packageIndex.APIVersion, formpackage.CurrentPackageAPIVersion)
		}
		if record.PackagePath != locator.SourcePath {
			return nil, fmt.Errorf("currentForms[%d] packagePath is %q, want canonical %q", index, record.PackagePath, locator.SourcePath)
		}
		if packageIndex.FormRef != record.FormRef {
			return nil, fmt.Errorf("currentForms[%d] package index FormRef differs from lifecycle authority", index)
		}
		if _, duplicate := seenTags[locator.Tag]; duplicate {
			return nil, fmt.Errorf("currentForms[%d] duplicates release tag %s", index, locator.Tag)
		}
		seenTags[locator.Tag] = struct{}{}
		identities = append(identities, CurrentFormReleaseIdentity{
			ProposalID: record.ProposalID, State: record.State, Kind: record.FormRef.Kind,
			ReleaseID: locator.ReleaseID, ArtifactID: locator.ArtifactID, Tag: locator.Tag,
			SourcePath: record.PackagePath, FormRef: record.FormRef, PackageDigest: record.PackageDigest,
		})
	}
	sort.Slice(identities, func(left, right int) bool {
		return identities[left].Tag < identities[right].Tag
	})
	return identities, nil
}

func RenderCurrentReleasePlan(root string) (string, error) {
	plan, err := CurrentReleasePlan(root)
	if err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode current Form release plan: %w", err)
	}
	return string(raw) + "\n", nil
}
