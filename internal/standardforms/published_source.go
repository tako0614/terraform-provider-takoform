package standardforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissioncheckpoint"
	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
)

type publishedReleaseSource struct {
	AdmissionGeneration string
	ManifestPath        string
	ManifestDigest      string
	ReleaseID           string
	ArtifactID          string
	Tag                 string
	FormRef             formpackage.FormRef
	PackageDigest       string
	SourcePath          string
}

type retainedReleaseManifest struct {
	SchemaVersion       int                 `json:"schemaVersion"`
	ReleaseType         string              `json:"releaseType"`
	Tag                 string              `json:"tag"`
	PackageVersion      string              `json:"packageVersion"`
	ReleaseID           string              `json:"releaseId"`
	PackageDigest       string              `json:"packageDigest"`
	FormRef             formpackage.FormRef `json:"formRef"`
	Canonicalization    string              `json:"canonicalization"`
	SignedSubject       string              `json:"signedSubject"`
	Assets              []retainedAsset     `json:"assets"`
	PublicationReady    bool                `json:"publicationReady"`
	PublicationBlockers []string            `json:"publicationBlockers"`
}

type retainedAsset struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type retainedPublishedPackageSet struct {
	Format            string                             `json:"format"`
	PublicationStatus string                             `json:"publicationStatus"`
	Entries           []retainedPublishedPackageSetEntry `json:"entries"`
}

type retainedPublishedPackageSetEntry struct {
	Immutable                    bool                `json:"immutable"`
	Kind                         string              `json:"kind"`
	ReleaseTag                   string              `json:"releaseTag"`
	PackageDigest                string              `json:"packageDigest"`
	FormRef                      formpackage.FormRef `json:"formRef"`
	PackageReleaseManifestPath   string              `json:"packageReleaseManifestPath"`
	PackageReleaseManifestDigest string              `json:"packageReleaseManifestDigest"`
}

// VerifyPublishedReleaseSources is the read-only no-overwrite gate for every
// Form Package identity for which this repository retains publication
// evidence. It authenticates the exact signed package-index bytes and then
// proves that forms/releases still contains the byte-exact payload closure
// that index names.
func VerifyPublishedReleaseSources(root string) error {
	_, err := discoverPublishedReleaseSources(root)
	return err
}

func discoverPublishedReleaseSources(root string) (map[string]publishedReleaseSource, error) {
	admissionRoot := filepath.Join(root, "admission")
	generations, err := os.ReadDir(admissionRoot)
	if err != nil {
		return nil, fmt.Errorf("read retained admission roots: %w", err)
	}
	published := make(map[string]publishedReleaseSource)
	byManifest := make(map[string]publishedReleaseSource)
	for _, generation := range generations {
		if !generation.IsDir() {
			continue
		}
		generationRoot := filepath.Join(admissionRoot, generation.Name())
		releasesRoot := filepath.Join(generationRoot, "releases")
		releaseIDs, err := os.ReadDir(releasesRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read retained releases for admission/%s: %w", generation.Name(), err)
		}
		for _, releaseID := range releaseIDs {
			if !releaseID.IsDir() {
				return nil, fmt.Errorf("admission/%s/releases contains non-directory %s", generation.Name(), releaseID.Name())
			}
			versions, err := os.ReadDir(filepath.Join(releasesRoot, releaseID.Name()))
			if err != nil {
				return nil, fmt.Errorf("read retained releases for %s: %w", releaseID.Name(), err)
			}
			if len(versions) == 0 {
				return nil, fmt.Errorf("retained release directory %s is empty", releaseID.Name())
			}
			for _, version := range versions {
				if !version.IsDir() {
					return nil, fmt.Errorf("retained release %s contains non-directory %s", releaseID.Name(), version.Name())
				}
				source, err := verifyRetainedReleaseSource(root, generation.Name(), releaseID.Name(), version.Name())
				if err != nil {
					return nil, err
				}
				key := publishedReleaseKey(source.ReleaseID, source.ArtifactID)
				if previous, duplicate := published[key]; duplicate {
					return nil, fmt.Errorf(
						"published Form identity %s@%s is retained by both admission/%s and admission/%s",
						source.FormRef.Kind, source.ArtifactID, previous.AdmissionGeneration, source.AdmissionGeneration,
					)
				}
				published[key] = source
				byManifest[generation.Name()+"\x00"+source.ManifestPath] = source
			}
		}
	}
	if err := verifyRetainedPublishedPackageSets(root, generations, byManifest); err != nil {
		return nil, err
	}
	if err := mergeHistoricalCheckpointPublications(root, published); err != nil {
		return nil, err
	}
	if err := verifyLocalPublishedFormTags(root, published); err != nil {
		return nil, err
	}
	return published, nil
}

func mergeHistoricalCheckpointPublications(
	root string,
	published map[string]publishedReleaseSource,
) error {
	ledgerPath := filepath.Join(root, filepath.FromSlash(admissioncheckpoint.IdentityLedgerPath))
	if _, err := os.Lstat(ledgerPath); err != nil {
		if os.IsNotExist(err) {
			// Minimal package fixtures predate the admission identity ledger.
			return nil
		}
		return fmt.Errorf("inspect admission identity ledger: %w", err)
	}
	ledger, err := admissioncheckpoint.LoadHistory(root)
	if err != nil {
		return fmt.Errorf("load historical admission identities: %w", err)
	}
	for _, identity := range ledger.Entries {
		if identity.Status != "assigned-historical" {
			continue
		}
		sets, err := formpublication.VerifyHistoricalCheckpoint(
			root,
			formpublication.HistoricalCheckpoint{
				Tag: identity.Tag, TagObject: identity.TagObject, Commit: identity.Commit,
			},
		)
		if err != nil {
			return err
		}
		for _, set := range sets {
			for _, entry := range set.Entries {
				source := publishedReleaseSource{
					AdmissionGeneration: "historical:" + identity.Version,
					ReleaseID:           entry.ReleaseID,
					ArtifactID:          entry.Version,
					Tag:                 entry.Tag,
					FormRef:             entry.FormRef,
					PackageDigest:       entry.PackageDigest,
					SourcePath:          entry.SourcePath,
				}
				key := publishedReleaseKey(source.ReleaseID, source.ArtifactID)
				previous, duplicate := published[key]
				if duplicate {
					if !samePublishedReleaseIdentity(previous, source) {
						return fmt.Errorf(
							"historical admission checkpoint %s conflicts with retained Form %s@%s",
							identity.Tag, source.FormRef.Kind, source.ArtifactID,
						)
					}
					continue
				}
				published[key] = source
			}
		}
	}
	return nil
}

func samePublishedReleaseIdentity(left, right publishedReleaseSource) bool {
	return left.ReleaseID == right.ReleaseID &&
		left.ArtifactID == right.ArtifactID &&
		left.Tag == right.Tag &&
		left.FormRef == right.FormRef &&
		left.PackageDigest == right.PackageDigest &&
		left.SourcePath == right.SourcePath
}

func verifyRetainedReleaseSource(root, generation, releaseID, version string) (publishedReleaseSource, error) {
	relativeManifest := path.Join("releases", releaseID, version, "release-manifest.json")
	manifestPath := filepath.Join(root, "admission", generation, filepath.FromSlash(relativeManifest))
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return publishedReleaseSource{}, fmt.Errorf("read retained %s: %w", filepath.ToSlash(manifestPath), err)
	}
	var manifest retainedReleaseManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return publishedReleaseSource{}, fmt.Errorf("decode retained %s: %w", filepath.ToSlash(manifestPath), err)
	}
	wantTag := "forms/" + releaseID + "/v" + version
	if manifest.SchemaVersion != 1 ||
		manifest.ReleaseType != "form-package" ||
		manifest.ReleaseID != releaseID ||
		manifest.PackageVersion != version ||
		manifest.Tag != wantTag ||
		manifest.Canonicalization != "RFC8785" ||
		!manifest.PublicationReady ||
		len(manifest.PublicationBlockers) != 0 ||
		manifest.FormRef.APIVersion != formpackage.FormAPIVersion ||
		manifest.FormRef.Kind == "" ||
		releaseIDForKind(manifest.FormRef.Kind) != releaseID ||
		manifest.FormRef.DefinitionVersion != version ||
		!formpackage.ValidDigest(manifest.FormRef.SchemaDigest) ||
		!formpackage.ValidDigest(manifest.PackageDigest) {
		return publishedReleaseSource{}, fmt.Errorf(
			"retained publication manifest admission/%s/%s has invalid immutable Form identity",
			generation, relativeManifest,
		)
	}
	wantSubject := "takoform-form-" + releaseID + "_" + version + "_package-index.json"
	if manifest.SignedSubject != wantSubject {
		return publishedReleaseSource{}, fmt.Errorf(
			"retained publication manifest admission/%s/%s signed subject = %q, want %q",
			generation, relativeManifest, manifest.SignedSubject, wantSubject,
		)
	}
	var subjectAsset *retainedAsset
	for index := range manifest.Assets {
		asset := &manifest.Assets[index]
		if asset.Name != manifest.SignedSubject {
			continue
		}
		if subjectAsset != nil {
			return publishedReleaseSource{}, fmt.Errorf(
				"retained publication manifest admission/%s/%s duplicates signed subject asset",
				generation, relativeManifest,
			)
		}
		subjectAsset = asset
	}
	if subjectAsset == nil ||
		subjectAsset.MediaType != "application/vnd.takoform.package-index.v1+json" ||
		subjectAsset.Size < 0 ||
		!formpackage.ValidDigest(subjectAsset.Digest) {
		return publishedReleaseSource{}, fmt.Errorf(
			"retained publication manifest admission/%s/%s has invalid signed subject asset",
			generation, relativeManifest,
		)
	}
	retainedIndexPath := filepath.Join(filepath.Dir(manifestPath), manifest.SignedSubject)
	retainedIndexRaw, err := os.ReadFile(retainedIndexPath)
	if err != nil {
		return publishedReleaseSource{}, fmt.Errorf("read retained signed package index: %w", err)
	}
	if int64(len(retainedIndexRaw)) != subjectAsset.Size ||
		formpackage.DigestBytes(retainedIndexRaw) != subjectAsset.Digest {
		return publishedReleaseSource{}, fmt.Errorf(
			"retained signed package index bytes drift for %s@%s",
			manifest.FormRef.Kind, version,
		)
	}
	index, err := formpackage.ValidatePackageIndex(retainedIndexRaw)
	if err != nil {
		return publishedReleaseSource{}, fmt.Errorf("retained signed package index %s@%s: %w", manifest.FormRef.Kind, version, err)
	}
	indexDigest, err := formpackage.DigestCanonicalJSON(retainedIndexRaw)
	if err != nil {
		return publishedReleaseSource{}, err
	}
	if index.PackageVersion != version ||
		index.FormRef != manifest.FormRef ||
		indexDigest != manifest.PackageDigest {
		return publishedReleaseSource{}, fmt.Errorf(
			"retained signed package index identity drift for %s@%s",
			manifest.FormRef.Kind, version,
		)
	}
	sourcePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseID, version))
	sourceRoot := filepath.Join(root, filepath.FromSlash(sourcePath))
	report, err := formpackage.VerifyDirectory(sourceRoot)
	if err != nil {
		return publishedReleaseSource{}, fmt.Errorf("published release source %s@%s: %w", manifest.FormRef.Kind, version, err)
	}
	if report.FormRef != manifest.FormRef || report.PackageDigest != manifest.PackageDigest {
		return publishedReleaseSource{}, fmt.Errorf(
			"published release source identity drift for %s@%s",
			manifest.FormRef.Kind, version,
		)
	}
	return publishedReleaseSource{
		AdmissionGeneration: generation,
		ManifestPath:        relativeManifest,
		ManifestDigest:      formpackage.DigestBytes(manifestRaw),
		ReleaseID:           releaseID,
		ArtifactID:          version,
		Tag:                 manifest.Tag,
		FormRef:             manifest.FormRef,
		PackageDigest:       manifest.PackageDigest,
		SourcePath:          sourcePath,
	}, nil
}

func verifyRetainedPublishedPackageSets(root string, generations []os.DirEntry, byManifest map[string]publishedReleaseSource) error {
	for _, generation := range generations {
		if !generation.IsDir() {
			continue
		}
		setPath := filepath.Join(root, "admission", generation.Name(), "published-package-set.json")
		raw, err := os.ReadFile(setPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		var set retainedPublishedPackageSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return fmt.Errorf("decode %s: %w", filepath.ToSlash(setPath), err)
		}
		if (set.Format != "takoform.published-package-set@v1" &&
			set.Format != "takoform.published-package-set@v2") ||
			set.PublicationStatus != "published-immutable" ||
			len(set.Entries) == 0 {
			return fmt.Errorf("admission/%s published package set has invalid immutable identity", generation.Name())
		}
		seen := make(map[string]struct{}, len(set.Entries))
		for index, entry := range set.Entries {
			if !entry.Immutable ||
				path.Clean(entry.PackageReleaseManifestPath) != entry.PackageReleaseManifestPath ||
				!strings.HasPrefix(entry.PackageReleaseManifestPath, "releases/") ||
				!formpackage.ValidDigest(entry.PackageReleaseManifestDigest) {
				return fmt.Errorf("admission/%s published packages[%d] has invalid retained manifest reference", generation.Name(), index)
			}
			source, ok := byManifest[generation.Name()+"\x00"+entry.PackageReleaseManifestPath]
			if !ok {
				return fmt.Errorf(
					"admission/%s published packages[%d] references an unretained release manifest %s",
					generation.Name(), index, entry.PackageReleaseManifestPath,
				)
			}
			key := publishedReleaseKey(source.ReleaseID, source.ArtifactID)
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("admission/%s published package set duplicates %s", generation.Name(), source.Tag)
			}
			seen[key] = struct{}{}
			if entry.Kind != source.FormRef.Kind ||
				entry.ReleaseTag != source.Tag ||
				entry.FormRef != source.FormRef ||
				entry.PackageDigest != source.PackageDigest ||
				entry.PackageReleaseManifestDigest != source.ManifestDigest {
				return fmt.Errorf(
					"admission/%s published packages[%d] drifts from retained release manifest %s",
					generation.Name(), index, entry.PackageReleaseManifestPath,
				)
			}
		}
	}
	return nil
}

func verifyLocalPublishedFormTags(root string, published map[string]publishedReleaseSource) error {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect repository metadata: %w", err)
	}
	command := exec.Command("git", "-C", root, "tag", "--list", "forms/k-*/*")
	raw, err := command.Output()
	if err != nil {
		return fmt.Errorf("enumerate published Form tags: %w", err)
	}
	tags := strings.Fields(string(raw))
	sort.Strings(tags)
	var lifecycleByTag map[string]CurrentFormReleaseIdentity
	for _, tag := range tags {
		locator, err := formpackage.ParsePublicationTag(tag)
		if err != nil {
			return fmt.Errorf("published Form tag identity: %w", err)
		}
		command := exec.Command("git", "-C", root, "cat-file", "-t", tag)
		objectType, err := command.Output()
		if err != nil {
			return fmt.Errorf("inspect published Form tag object %s: %w", tag, err)
		}
		if strings.TrimSpace(string(objectType)) != "tag" {
			return fmt.Errorf("published Form tag %s must be an annotated tag", tag)
		}
		key := publishedReleaseKey(locator.ReleaseID, locator.ArtifactID)
		source, ok := published[key]
		unretained := !ok
		if !ok {
			if lifecycleByTag == nil {
				var err error
				lifecycleByTag, err = verifiedCurrentLifecycleReleaseByTag(root)
				if err != nil {
					return fmt.Errorf("published Form tag %s is unretained and the current lifecycle authority is invalid: %w", tag, err)
				}
			}
			planned, exists := lifecycleByTag[tag]
			if !exists {
				return fmt.Errorf(
					"published Form tag %s has no retained Legacy release manifest and is not in the current lifecycle authority",
					tag,
				)
			}
			report, err := formpackage.VerifyDirectory(
				filepath.Join(root, filepath.FromSlash(planned.SourcePath)),
			)
			if err != nil {
				return fmt.Errorf("lifecycle release source for unretained Form tag %s: %w", tag, err)
			}
			if planned.ReleaseID != locator.ReleaseID ||
				planned.ArtifactID != locator.ArtifactID ||
				report.FormRef != planned.FormRef ||
				report.PackageDigest != planned.PackageDigest {
				return fmt.Errorf("unretained Form tag %s drifts from the current lifecycle identity", tag)
			}
			source = publishedReleaseSource{
				ReleaseID:     planned.ReleaseID,
				ArtifactID:    planned.ArtifactID,
				Tag:           planned.Tag,
				FormRef:       planned.FormRef,
				PackageDigest: planned.PackageDigest,
				SourcePath:    planned.SourcePath,
			}
		}
		if source.Tag != tag {
			return fmt.Errorf("published Form tag %s drifts from retained admission release manifest", tag)
		}
		compareCommand := exec.Command(
			"git", "-C", root, "diff", "--quiet", "--no-ext-diff", "--no-textconv",
			tag, "--", source.SourcePath,
		)
		if err := compareCommand.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
				return fmt.Errorf("published release source %s differs byte-for-byte from immutable tag %s", source.SourcePath, tag)
			}
			return fmt.Errorf("compare published release source with immutable tag %s: %w", tag, err)
		}
		if unretained {
			published[key] = source
		}
	}
	return nil
}

func verifiedCurrentLifecycleReleaseByTag(root string) (map[string]CurrentFormReleaseIdentity, error) {
	authority, err := readProjectLifecycleAuthority(root)
	if err != nil {
		return nil, err
	}
	identities, err := currentLifecycleReleaseIdentities(root, authority)
	if err != nil {
		return nil, err
	}
	byTag := make(map[string]CurrentFormReleaseIdentity, len(identities))
	for _, release := range identities {
		if _, duplicate := byTag[release.Tag]; duplicate {
			return nil, fmt.Errorf("current lifecycle authority reuses tag %s", release.Tag)
		}
		byTag[release.Tag] = release
	}
	return byTag, nil
}

func verifyNoPublishedReleaseOverwrite(root, stagingRoot string, entries []InventoryEntry, published map[string]publishedReleaseSource) error {
	for _, entry := range entries {
		releaseID := releaseIDForKind(entry.Kind)
		legacyKey := publishedReleaseKey(releaseID, entry.FormRef.DefinitionVersion)
		currentKey := publishedReleaseKey(releaseID, strings.Replace(entry.PackageDigest, ":", "-", 1))
		source, exists := published[currentKey]
		if legacy, legacyExists := published[legacyKey]; legacyExists {
			if exists && !samePublishedReleaseIdentity(source, legacy) {
				return fmt.Errorf("Form %s has conflicting Legacy and content-addressed publication identities", entry.Kind)
			}
			source, exists = legacy, true
		}
		if !exists {
			continue
		}
		if entry.FormRef != source.FormRef || entry.PackageDigest != source.PackageDigest {
			return fmt.Errorf(
				"refusing to overwrite published Form %s@%s with a different exact identity",
				entry.Kind, entry.FormRef.DefinitionVersion,
			)
		}
		stagedRoot := filepath.Join(stagingRoot, filepath.FromSlash(entry.Path))
		retainedRoot := filepath.Join(root, filepath.FromSlash(source.SourcePath))
		if err := verifyExactPackageSourceBytes(retainedRoot, stagedRoot); err != nil {
			return fmt.Errorf(
				"refusing to overwrite published Form %s@%s: %w",
				entry.Kind, entry.FormRef.DefinitionVersion, err,
			)
		}
	}
	return nil
}

func verifyExactPackageSourceBytes(retainedRoot, candidateRoot string) error {
	retainedFiles, err := packageSourceFiles(retainedRoot)
	if err != nil {
		return err
	}
	candidateFiles, err := packageSourceFiles(candidateRoot)
	if err != nil {
		return err
	}
	if len(retainedFiles) != len(candidateFiles) {
		return fmt.Errorf("package file closure differs")
	}
	for index, relative := range retainedFiles {
		if candidateFiles[index] != relative {
			return fmt.Errorf("package file closure differs")
		}
		retainedRaw, err := os.ReadFile(filepath.Join(retainedRoot, filepath.FromSlash(relative)))
		if err != nil {
			return err
		}
		candidateRaw, err := os.ReadFile(filepath.Join(candidateRoot, filepath.FromSlash(relative)))
		if err != nil {
			return err
		}
		if !bytes.Equal(retainedRaw, candidateRaw) {
			return fmt.Errorf("%s bytes differ", relative)
		}
	}
	return nil
}

func packageSourceFiles(root string) ([]string, error) {
	if _, err := formpackage.VerifyDirectory(root); err != nil {
		return nil, err
	}
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func publishedReleaseKey(releaseID, version string) string {
	return releaseID + "\x00" + version
}
