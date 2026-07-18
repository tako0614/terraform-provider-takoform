package admissionrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	runnerReportFormat         = "takoform.standard-runner-report@v1"
	registryReadbackFormat     = "takoform.provider-registry-readback@v1"
	packageReleaseSchema       = 1
	packageReleaseType         = "form-package"
	sourceRepository           = "github.com/tako0614/terraform-provider-takoform"
	packageReleaseWorkflow     = ".github/workflows/form-package-release.yml"
	packageIndexMediaType      = "application/vnd.takoform.package-index.v1+json"
	packagePublisherIssuer     = "https://token.actions.githubusercontent.com"
	packagePublisherIdentity   = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main"
	packagePublisherTagPattern = "refs/tags/forms/k-*/v*"
	registryProviderAddress    = "registry.terraform.io/tako0614/takoform"
	maxReportBytes             = 16 << 20
	maxReleaseManifestBytes    = 4 << 20
	maxReleaseAssetBytes       = 64 << 20
)

// RunnerReport is the signed, portable summary emitted independently by a
// host runner or provider runner for one exact Form Package. It contains no
// credential, target, placement, billing, or operator authority.
type RunnerReport struct {
	Format           string                              `json:"format"`
	Role             string                              `json:"role"`
	Subject          string                              `json:"subject"`
	RunnerVersion    string                              `json:"runnerVersion"`
	Identity         standardform.InstalledFormReference `json:"identity"`
	Status           string                              `json:"status"`
	Lifecycle        standardform.LifecycleAudit         `json:"lifecycle"`
	PositiveFixtures []PositiveFixtureResult             `json:"positiveFixtures"`
	NegativeFixtures []NegativeFixtureResult             `json:"negativeFixtures"`
}

type PositiveFixtureResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

type NegativeFixtureResult struct {
	Name      string `json:"name"`
	ErrorCode string `json:"errorCode"`
	Passed    bool   `json:"passed"`
}

type packageReleaseManifest struct {
	SchemaVersion       int                    `json:"schemaVersion"`
	ReleaseType         string                 `json:"releaseType"`
	Tag                 string                 `json:"tag"`
	SourceRepository    string                 `json:"sourceRepository"`
	SourceCommit        string                 `json:"sourceCommit"`
	Workflow            string                 `json:"workflow"`
	PackageVersion      string                 `json:"packageVersion"`
	ReleaseID           string                 `json:"releaseId"`
	PackageDigest       string                 `json:"packageDigest"`
	FormRef             formpackage.FormRef    `json:"formRef"`
	Canonicalization    string                 `json:"canonicalization"`
	SignedSubject       string                 `json:"signedSubject"`
	SignatureBundle     string                 `json:"signatureBundle"`
	SignatureMediaType  string                 `json:"signatureMediaType"`
	PublisherPolicy     releasePublisherPolicy `json:"publisherPolicy"`
	Assets              []releaseAsset         `json:"assets"`
	PublicationReady    bool                   `json:"publicationReady"`
	PublicationBlockers []string               `json:"publicationBlockers"`
}

type releasePublisherPolicy struct {
	OIDCIssuer string `json:"oidcIssuer"`
	Identity   string `json:"identity"`
	TagPattern string `json:"tagPattern"`
}

type releaseAsset struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// ProviderRegistryReadback is a signed post-publication report. It binds a
// direct Registry install for both supported CLIs to the full provider
// lifecycle matrix and the exact lock files retained beside the report.
type ProviderRegistryReadback struct {
	Format                string            `json:"format"`
	PublicationReady      bool              `json:"publicationReady"`
	ProviderAddress       string            `json:"providerAddress"`
	ProviderVersion       string            `json:"providerVersion"`
	ProviderReleaseTag    string            `json:"providerReleaseTag"`
	ProviderReleaseCommit string            `json:"providerReleaseCommit"`
	CandidateSetSHA256    string            `json:"candidateSetSha256"`
	ProviderSchemaSHA256  string            `json:"providerSchemaSha256"`
	LifecycleMatrixPath   string            `json:"lifecycleMatrixPath"`
	LifecycleMatrixDigest string            `json:"lifecycleMatrixDigest"`
	Installs              []RegistryInstall `json:"installs"`
}

type RegistryInstall struct {
	Product              string `json:"product"`
	CLIVersion           string `json:"cliVersion"`
	ProviderAddress      string `json:"providerAddress"`
	ProviderVersion      string `json:"providerVersion"`
	ProviderBinarySHA256 string `json:"providerBinarySha256"`
	ProviderSchemaSHA256 string `json:"providerSchemaSha256"`
}

// BuildRegistryReadback creates the canonical unsigned readback subject from
// a direct Registry matrix. Authentication remains a separate Phase 2
// workflow action; this helper never signs or upgrades admission state.
func BuildRegistryReadback(root, matrixFile, providerReleaseCommit string) (ProviderRegistryReadback, []byte, error) {
	if !releaseCommitPattern.MatchString(providerReleaseCommit) {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("provider release commit must be lowercase 40-hex")
	}
	matrixRaw, err := readRetainedRegularFile(matrixFile, maxReportBytes)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	var matrix providerlifecycle.MatrixReport
	if err := decodeStrictJSON(matrixRaw, &matrix); err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	requirements, descriptorDigest, err := providerlifecycle.LoadCLIMatrix(root)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	if err := providerlifecycle.ValidateRegistryMatrix(matrix, requirements); err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	if matrix.ReleaseDescriptorSHA256 != descriptorDigest {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("direct Registry matrix does not bind the current release descriptor")
	}
	providerVersion, err := readProviderVersion(root)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	installs := make([]RegistryInstall, 0, len(matrix.Reports))
	for _, report := range matrix.Reports {
		if report.ProviderBinary.Version != providerVersion {
			return ProviderRegistryReadback{}, nil, fmt.Errorf("%s installed provider version is %q, want %q", report.CLI.Product, report.ProviderBinary.Version, providerVersion)
		}
		installs = append(installs, RegistryInstall{
			Product: report.CLI.Product, CLIVersion: report.CLI.Version, ProviderAddress: report.CLI.ProviderAddress,
			ProviderVersion: providerVersion, ProviderBinarySHA256: report.ProviderBinary.SHA256,
			ProviderSchemaSHA256: report.ProviderSchemaSHA256,
		})
	}
	readback := ProviderRegistryReadback{
		Format: registryReadbackFormat, PublicationReady: true, ProviderAddress: registryProviderAddress,
		ProviderVersion: providerVersion, ProviderReleaseTag: "v" + providerVersion, ProviderReleaseCommit: providerReleaseCommit,
		CandidateSetSHA256: matrix.CandidateSetSHA256, ProviderSchemaSHA256: matrix.ProviderSchemaSHA256,
		LifecycleMatrixPath: "registry/provider-lifecycle-matrix.json", LifecycleMatrixDigest: formpackage.DigestBytes(matrixRaw),
		Installs: installs,
	}
	raw, err := json.Marshal(readback)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	return readback, canonical, nil
}

func readCanonicalRunnerReport(admissionRoot, relative string, maximum int64) (RunnerReport, []byte, error) {
	raw, err := readRetainedRelativeFile(admissionRoot, relative, maximum)
	if err != nil {
		return RunnerReport{}, nil, err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil || !bytes.Equal(raw, canonical) {
		if err == nil {
			err = fmt.Errorf("retained bytes are not RFC 8785 canonical")
		}
		return RunnerReport{}, nil, err
	}
	var report RunnerReport
	if err := decodeStrictJSON(raw, &report); err != nil {
		return RunnerReport{}, nil, err
	}
	return report, raw, nil
}

func validateRunnerReport(report RunnerReport, role string, proof standardform.ConformanceProof, positives, negatives []string) error {
	if report.Format != runnerReportFormat || report.Role != role || report.Status != "passed" ||
		strings.TrimSpace(report.Subject) == "" || strings.TrimSpace(report.RunnerVersion) == "" ||
		report.Subject != proof.Subject || report.RunnerVersion != proof.RunnerVersion ||
		!reflect.DeepEqual(report.Identity, proof.Identity) || proof.Status != "passed" {
		return fmt.Errorf("%s runner report identity does not match admission evidence", role)
	}
	lifecycle := report.Lifecycle
	if !lifecycle.Create || !lifecycle.Read || !lifecycle.Update || !lifecycle.Delete || !lifecycle.Import ||
		!lifecycle.Observe || !lifecycle.Refresh || !lifecycle.Drift {
		return fmt.Errorf("%s runner report lacks the complete portable lifecycle", role)
	}
	positiveNames := make([]string, 0, len(report.PositiveFixtures))
	for _, result := range report.PositiveFixtures {
		if strings.TrimSpace(result.Name) == "" || !result.Passed {
			return fmt.Errorf("%s positive fixture is empty or failed", role)
		}
		positiveNames = append(positiveNames, result.Name)
	}
	negativeNames := make([]string, 0, len(report.NegativeFixtures))
	for _, result := range report.NegativeFixtures {
		if strings.TrimSpace(result.Name) == "" || !result.Passed || result.ErrorCode != standardform.InvalidArgumentErrorCode {
			return fmt.Errorf("%s negative fixture did not return %q", role, standardform.InvalidArgumentErrorCode)
		}
		negativeNames = append(negativeNames, result.Name)
	}
	if !sameStringSet(positiveNames, positives) || !sameStringSet(negativeNames, negatives) ||
		!sameStringSet(proof.PositiveFixtures, positives) || !sameStringSet(proof.NegativeFixtures, negatives) {
		return fmt.Errorf("%s runner report fixture closure does not match admission evidence", role)
	}
	return nil
}

func verifyPackageReleaseReadback(admissionRoot string, pair matchedEntry, packageVersion string) ([]byte, error) {
	entry := pair.entry
	manifestRaw, err := readRetainedRelativeFile(admissionRoot, entry.PackageReleaseManifestPath, maxReleaseManifestBytes)
	if err != nil {
		return nil, err
	}
	if formpackage.DigestBytes(manifestRaw) != entry.PackageReleaseManifestDigest {
		return nil, fmt.Errorf("package release manifest digest mismatch")
	}
	var manifest packageReleaseManifest
	if err := decodeStrictJSON(manifestRaw, &manifest); err != nil {
		return nil, err
	}
	expectedReleaseID := releaseIDForKind(entry.Kind)
	if manifest.SchemaVersion != packageReleaseSchema || manifest.ReleaseType != packageReleaseType ||
		manifest.Tag != entry.ReleaseTag || manifest.SourceRepository != sourceRepository || manifest.SourceCommit != entry.ReleaseCommit ||
		manifest.Workflow != packageReleaseWorkflow || manifest.PackageVersion != packageVersion ||
		manifest.ReleaseID != expectedReleaseID || manifest.PackageDigest != entry.PackageDigest || manifest.FormRef != entry.FormRef ||
		manifest.Canonicalization != "RFC8785" || manifest.SignatureMediaType != sigstoreBundleMediaTypeV03 ||
		!manifest.PublicationReady || len(manifest.PublicationBlockers) != 0 {
		return nil, fmt.Errorf("package release manifest does not bind the immutable admitted package")
	}
	manifestDir := path.Dir(entry.PackageReleaseManifestPath)
	if entry.PackageIndexPath != path.Join(manifestDir, manifest.SignedSubject) ||
		entry.PackageIndexSigstoreBundle != path.Join(manifestDir, manifest.SignatureBundle) {
		return nil, fmt.Errorf("package release signed subject/bundle path drift")
	}
	if manifest.PublisherPolicy.OIDCIssuer != packagePublisherIssuer || manifest.PublisherPolicy.Identity != packagePublisherIdentity ||
		manifest.PublisherPolicy.TagPattern != packagePublisherTagPattern {
		return nil, fmt.Errorf("package release publisher policy is not the protected Takoform package workflow")
	}
	if len(manifest.Assets) != 5 {
		return nil, fmt.Errorf("package release asset closure has %d entries, want exactly 5", len(manifest.Assets))
	}
	assets := make(map[string]releaseAsset, len(manifest.Assets))
	assetBytes := make(map[string][]byte, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if asset.Name == "" || path.Base(asset.Name) != asset.Name || asset.Size < 0 || asset.Size > maxReleaseAssetBytes ||
			!formpackage.ValidDigest(asset.Digest) || asset.MediaType == "" {
			return nil, fmt.Errorf("package release contains an invalid asset descriptor")
		}
		if _, duplicate := assets[asset.Name]; duplicate {
			return nil, fmt.Errorf("package release duplicates asset %q", asset.Name)
		}
		assetRaw, err := readRetainedRelativeFile(admissionRoot, path.Join(manifestDir, asset.Name), maxReleaseAssetBytes)
		if err != nil {
			return nil, fmt.Errorf("read package release asset %q: %w", asset.Name, err)
		}
		if int64(len(assetRaw)) != asset.Size || formpackage.DigestBytes(assetRaw) != asset.Digest {
			return nil, fmt.Errorf("package release asset %q readback mismatch", asset.Name)
		}
		assets[asset.Name] = asset
		assetBytes[asset.Name] = assetRaw
	}
	indexAsset, ok := assets[manifest.SignedSubject]
	if !ok || indexAsset.MediaType != packageIndexMediaType {
		return nil, fmt.Errorf("package release omits the canonical signed package index")
	}
	if bundleAsset, ok := assets[manifest.SignatureBundle]; !ok || bundleAsset.MediaType != sigstoreBundleMediaTypeV03 {
		return nil, fmt.Errorf("package release omits the Sigstore bundle")
	}
	assetBase := strings.TrimSuffix(manifest.SignedSubject, "_package-index.json")
	for name, mediaType := range map[string]string{
		assetBase + ".tar.gz":                 "application/gzip",
		assetBase + "_sbom.spdx.json":         "application/spdx+json",
		assetBase + "_provenance.intoto.json": "application/vnd.in-toto+json",
	} {
		if asset, ok := assets[name]; !ok || asset.MediaType != mediaType {
			return nil, fmt.Errorf("package release omits required %q asset", name)
		}
	}
	indexRaw, err := readRetainedRelativeFile(admissionRoot, entry.PackageIndexPath, maxEvidenceBytes)
	if err != nil {
		return nil, err
	}
	canonical, err := formpackage.Canonicalize(indexRaw)
	if err != nil || !bytes.Equal(indexRaw, canonical) || formpackage.DigestBytes(indexRaw) != entry.PackageDigest {
		return nil, fmt.Errorf("package release index is not the exact canonical admitted package index")
	}
	localIndex, err := readRetainedRegularFile(filepath.Join(admissionRoot, "..", "..", filepath.FromSlash(pair.candidate.PackagePath), formpackage.PackageIndexFilename), maxEvidenceBytes)
	if err != nil {
		return nil, err
	}
	localCanonical, err := formpackage.Canonicalize(localIndex)
	if err != nil || !bytes.Equal(indexRaw, localCanonical) {
		return nil, fmt.Errorf("package release index bytes differ from the provider-compiled candidate")
	}
	if err := verifyPackageArchive(assetBytes[assetBase+".tar.gz"], indexRaw); err != nil {
		return nil, fmt.Errorf("package release archive readback: %w", err)
	}
	return indexRaw, nil
}

func verifyPackageArchive(archiveRaw, canonicalIndex []byte) error {
	index, err := formpackage.ValidatePackageIndex(canonicalIndex)
	if err != nil {
		return err
	}
	compressed := bytes.NewReader(archiveRaw)
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer reader.Close()
	if !reader.ModTime.IsZero() || reader.OS != 255 || reader.Name != "" || reader.Comment != "" || len(reader.Extra) != 0 {
		return fmt.Errorf("gzip header is not deterministic: modTime=%s os=%d name=%q comment=%q extra=%d", reader.ModTime.UTC().Format(time.RFC3339), reader.OS, reader.Name, reader.Comment, len(reader.Extra))
	}
	reader.Multistream(false)
	type expectedFile struct {
		name   string
		size   int64
		digest string
		raw    []byte
	}
	expected := []expectedFile{{
		name: formpackage.PackageIndexFilename, size: int64(len(canonicalIndex)),
		digest: formpackage.DigestBytes(canonicalIndex), raw: canonicalIndex,
	}}
	files := append([]formpackage.PackageFile(nil), index.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		expected = append(expected, expectedFile{name: file.Path, size: file.Size, digest: file.Digest})
	}
	tarReader := tar.NewReader(reader)
	for position, want := range expected {
		header, err := tarReader.Next()
		if err != nil {
			return fmt.Errorf("entry %d %q: %w", position, want.name, err)
		}
		epoch := time.Unix(0, 0).UTC()
		expectedPAX := map[string]string{"atime": "0", "ctime": "0"}
		if header.Name != want.name || header.Typeflag != tar.TypeReg || header.Mode != 0o644 || header.Size != want.size ||
			!header.ModTime.Equal(epoch) || !header.AccessTime.Equal(epoch) || !header.ChangeTime.Equal(epoch) ||
			header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" || header.Linkname != "" ||
			header.Devmajor != 0 || header.Devminor != 0 || header.Format != tar.FormatPAX ||
			!reflect.DeepEqual(header.PAXRecords, expectedPAX) || len(header.Xattrs) != 0 {
			return fmt.Errorf("entry %d is not the deterministic regular file %q: uid=%d gid=%d uname=%q gname=%q format=%s pax=%v xattrs=%v", position, want.name, header.Uid, header.Gid, header.Uname, header.Gname, header.Format, header.PAXRecords, header.Xattrs)
		}
		payload, err := io.ReadAll(io.LimitReader(tarReader, want.size+1))
		if err != nil {
			return err
		}
		if int64(len(payload)) != want.size || formpackage.DigestBytes(payload) != want.digest ||
			(want.raw != nil && !bytes.Equal(payload, want.raw)) {
			return fmt.Errorf("entry %q payload does not match the signed package index", want.name)
		}
	}
	if header, err := tarReader.Next(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("archive contains unlisted entry %q", header.Name)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("finish gzip member: %w", err)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close gzip member: %w", err)
	}
	if compressed.Len() != 0 {
		return fmt.Errorf("archive contains %d trailing bytes or an additional gzip member", compressed.Len())
	}
	return nil
}

func verifyRegistryReadback(root, admissionRoot string, set Set) (ProviderRegistryReadback, []byte, error) {
	ref := set.ProviderRegistryReadback
	raw, err := readRetainedRelativeFile(admissionRoot, ref.Path, maxReportBytes)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil || !bytes.Equal(raw, canonical) || formpackage.DigestBytes(raw) != ref.Digest {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry readback is not the exact retained canonical subject")
	}
	var readback ProviderRegistryReadback
	if err := decodeStrictJSON(raw, &readback); err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	requirements, descriptorDigest, err := providerlifecycle.LoadCLIMatrix(root)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	providerVersion, err := readProviderVersion(root)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	if readback.Format != registryReadbackFormat || !readback.PublicationReady || readback.ProviderAddress != registryProviderAddress ||
		readback.ProviderVersion != providerVersion || readback.ProviderReleaseTag != "v"+providerVersion ||
		!releaseCommitPattern.MatchString(readback.ProviderReleaseCommit) ||
		readback.CandidateSetSHA256 != providerlifecycle.CandidateSetSHA256() ||
		!formpackage.ValidDigest(readback.ProviderSchemaSHA256) || !formpackage.ValidDigest(readback.LifecycleMatrixDigest) ||
		len(readback.Installs) != len(requirements) {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry readback identity is invalid")
	}
	if err := validateRelativePath(readback.LifecycleMatrixPath); err != nil {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry lifecycle matrix path: %w", err)
	}
	matrixRaw, err := readRetainedRelativeFile(admissionRoot, readback.LifecycleMatrixPath, maxReportBytes)
	if err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	if formpackage.DigestBytes(matrixRaw) != readback.LifecycleMatrixDigest {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry lifecycle matrix digest mismatch")
	}
	var matrix providerlifecycle.MatrixReport
	if err := decodeStrictJSON(matrixRaw, &matrix); err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	if err := providerlifecycle.ValidateRegistryMatrix(matrix, requirements); err != nil {
		return ProviderRegistryReadback{}, nil, err
	}
	if matrix.ReleaseDescriptorSHA256 != descriptorDigest || matrix.CandidateSetSHA256 != readback.CandidateSetSHA256 ||
		matrix.ProviderSchemaSHA256 != readback.ProviderSchemaSHA256 {
		return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry matrix/readback identity mismatch")
	}
	installByProduct := make(map[string]RegistryInstall, len(readback.Installs))
	for _, install := range readback.Installs {
		if _, duplicate := installByProduct[install.Product]; duplicate {
			return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry readback duplicates %q", install.Product)
		}
		installByProduct[install.Product] = install
		if install.ProviderVersion != providerVersion || !formpackage.ValidDigest(install.ProviderBinarySHA256) ||
			install.ProviderSchemaSHA256 != readback.ProviderSchemaSHA256 {
			return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry install identity is invalid for %s", install.Product)
		}
	}
	for _, report := range matrix.Reports {
		install, ok := installByProduct[report.CLI.Product]
		if !ok || install.CLIVersion != report.CLI.Version || install.ProviderAddress != report.CLI.ProviderAddress ||
			install.ProviderBinarySHA256 != report.ProviderBinary.SHA256 || install.ProviderSchemaSHA256 != report.ProviderSchemaSHA256 {
			return ProviderRegistryReadback{}, nil, fmt.Errorf("provider Registry install does not bind the %s lifecycle report", report.CLI.Product)
		}
	}
	return readback, raw, nil
}

func readProviderVersion(root string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "release", "version.json"))
	if err != nil {
		return "", err
	}
	var descriptor struct {
		Version string `json:"version"`
		Tag     string `json:"tag"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return "", err
	}
	if descriptor.Version == "" || descriptor.Tag != "v"+descriptor.Version {
		return "", fmt.Errorf("provider release descriptor is invalid")
	}
	return descriptor.Version, nil
}

func sameStringSet(left, right []string) bool {
	a := append([]string(nil), left...)
	b := append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}
