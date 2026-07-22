package admissionrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
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
	"unicode"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	runnerReportFormat             = "takoform.standard-runner-report@v1"
	registryReadbackFormat         = "takoform.provider-registry-readback@v1"
	packageReleaseSchema           = 1
	packageReleaseType             = "form-package"
	sourceRepository               = "github.com/tako0614/terraform-provider-takoform"
	packageReleaseWorkflow         = ".github/workflows/standard-form-package-set-release.yml"
	packageIndexMediaType          = "application/vnd.takoform.package-index.v1+json"
	packagePublisherIssuer         = "https://token.actions.githubusercontent.com"
	packagePublisherIdentityPrefix = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/standard-form-package-set-release.yml@refs/tags/standard-forms/v"
	packagePublisherTagPattern     = "refs/tags/standard-forms/v*"
	registryProviderAddress        = "registry.terraform.io/tako0614/takoform"
	maxReportBytes                 = 16 << 20
	maxReleaseManifestBytes        = 4 << 20
	maxReleaseAssetBytes           = 64 << 20
)

func packagePublisherIdentity(packageVersion string) string {
	return packagePublisherIdentityPrefix + packageVersion
}

// RunnerReport is the signed, portable summary emitted independently by a
// host runner or provider runner for one exact Form Package. It contains no
// credential, target, placement, billing, or operator authority.
type RunnerReport struct {
	Format                  string                              `json:"format"`
	Role                    string                              `json:"role"`
	Subject                 string                              `json:"subject"`
	RunnerVersion           string                              `json:"runnerVersion"`
	Identity                standardform.InstalledFormReference `json:"identity"`
	Status                  string                              `json:"status"`
	Lifecycle               standardform.LifecycleAudit         `json:"lifecycle"`
	ExecutionEvidence       *HostExecutionEvidence              `json:"executionEvidence,omitempty"`
	ExecutionEvidenceDigest string                              `json:"executionEvidenceDigest,omitempty"`
	PositiveFixtures        []PositiveFixtureResult             `json:"positiveFixtures"`
	NegativeFixtures        []NegativeFixtureResult             `json:"negativeFixtures"`
}

type HostExecutionEvidence struct {
	APIVersion          string                              `json:"apiVersion"`
	Identity            standardform.InstalledFormReference `json:"identity"`
	EndpointOrigin      string                              `json:"endpointOrigin"`
	Status              string                              `json:"status"`
	Checks              []string                            `json:"checks"`
	Fixtures            HostExecutionFixtures               `json:"fixtures"`
	CanonicalResourceID string                              `json:"canonicalResourceId"`
}

type HostExecutionFixtures struct {
	Positive []HostPositiveExecutionFixture `json:"positive"`
	Negative []HostNegativeExecutionFixture `json:"negative"`
}

type HostPositiveExecutionFixture struct {
	Name                 string `json:"name"`
	InputDigest          string `json:"inputDigest"`
	PackageFixtureDigest string `json:"packageFixtureDigest"`
}

type HostNegativeExecutionFixture struct {
	Name                 string `json:"name"`
	Stage                string `json:"stage"`
	InputDigest          string `json:"inputDigest"`
	PackageFixtureDigest string `json:"packageFixtureDigest"`
	HTTPStatus           int    `json:"httpStatus"`
	ErrorCode            string `json:"errorCode"`
}

type PositiveFixtureResult struct {
	Name                 string `json:"name"`
	PackageFixtureDigest string `json:"packageFixtureDigest,omitempty"`
	EffectiveInputDigest string `json:"effectiveInputDigest,omitempty"`
	Passed               bool   `json:"passed"`
}

type NegativeFixtureResult struct {
	Name                 string `json:"name"`
	PackageFixtureDigest string `json:"packageFixtureDigest,omitempty"`
	EffectiveInputDigest string `json:"effectiveInputDigest,omitempty"`
	ErrorCode            string `json:"errorCode"`
	Passed               bool   `json:"passed"`
}

type fixtureDigestBinding struct {
	PackageFixtureDigest string
	EffectiveInputDigest string
}

type packageReleaseManifest struct {
	SchemaVersion       int                    `json:"schemaVersion"`
	ReleaseType         string                 `json:"releaseType"`
	Tag                 string                 `json:"tag"`
	SourceRepository    string                 `json:"sourceRepository"`
	SourceCommit        string                 `json:"sourceCommit"`
	ToolingCommit       string                 `json:"toolingCommit"`
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
	OIDCIssuer    string `json:"oidcIssuer"`
	Identity      string `json:"identity"`
	TagPattern    string `json:"tagPattern"`
	ToolingCommit string `json:"toolingCommit"`
}

type releaseAsset struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type packageSBOM struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      spdxCreationInfo   `json:"creationInfo"`
	Packages          []spdxPackage      `json:"packages"`
	Files             []spdxFile         `json:"files"`
	Relationships     []spdxRelationship `json:"relationships"`
}

type spdxCreationInfo struct {
	Creators []string `json:"creators"`
	Created  string   `json:"created"`
}

type spdxPackage struct {
	Name                    string                      `json:"name"`
	SPDXID                  string                      `json:"SPDXID"`
	VersionInfo             string                      `json:"versionInfo"`
	DownloadLocation        string                      `json:"downloadLocation"`
	FilesAnalyzed           bool                        `json:"filesAnalyzed"`
	PackageVerificationCode spdxPackageVerificationCode `json:"packageVerificationCode"`
	LicenseConcluded        string                      `json:"licenseConcluded"`
	LicenseDeclared         string                      `json:"licenseDeclared"`
	CopyrightText           string                      `json:"copyrightText"`
}

type spdxPackageVerificationCode struct {
	Value string `json:"packageVerificationCodeValue"`
}

type spdxFile struct {
	FileName           string         `json:"fileName"`
	SPDXID             string         `json:"SPDXID"`
	Checksums          []spdxChecksum `json:"checksums"`
	LicenseConcluded   string         `json:"licenseConcluded"`
	LicenseInfoInFiles []string       `json:"licenseInfoInFiles"`
	CopyrightText      string         `json:"copyrightText"`
}

type spdxChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type spdxRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

type packageProvenance struct {
	Type          string              `json:"_type"`
	Subject       []provenanceSubject `json:"subject"`
	PredicateType string              `json:"predicateType"`
	Predicate     provenancePredicate `json:"predicate"`
}

type provenanceSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type provenancePredicate struct {
	BuildDefinition provenanceBuildDefinition `json:"buildDefinition"`
	RunDetails      provenanceRunDetails      `json:"runDetails"`
}

type provenanceBuildDefinition struct {
	BuildType            string                 `json:"buildType"`
	ExternalParameters   map[string]string      `json:"externalParameters"`
	InternalParameters   map[string]string      `json:"internalParameters"`
	ResolvedDependencies []provenanceDependency `json:"resolvedDependencies"`
}

type provenanceDependency struct {
	Name   string            `json:"name"`
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest"`
}

type provenanceRunDetails struct {
	Builder provenanceBuilder `json:"builder"`
}

type provenanceBuilder struct {
	ID string `json:"id"`
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

// ValidateCanonicalProviderRunnerReport verifies an unsigned provider-report
// subject against one exact published package and its reviewed fixture names.
// It does not authenticate, retain, sign, publish, or admit the report.
func ValidateCanonicalProviderRunnerReport(raw []byte, identity standardform.InstalledFormReference, positives, negatives []string) (RunnerReport, error) {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return RunnerReport{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return RunnerReport{}, fmt.Errorf("provider-report bytes are not RFC 8785 canonical")
	}
	var report RunnerReport
	if err := decodeStrictJSON(raw, &report); err != nil {
		return RunnerReport{}, err
	}
	proof := standardform.ConformanceProof{
		Subject: report.Subject, RunnerVersion: report.RunnerVersion, Identity: identity, Status: "passed",
		PositiveFixtures: append([]string(nil), positives...), NegativeFixtures: append([]string(nil), negatives...),
	}
	if err := validateRunnerReport(report, roleProviderReport, proof, positives, negatives, nil, nil); err != nil {
		return RunnerReport{}, err
	}
	return report, nil
}

func validateRunnerReport(
	report RunnerReport,
	role string,
	proof standardform.ConformanceProof,
	positives, negatives []string,
	positiveBindings, negativeBindings map[string]fixtureDigestBinding,
) error {
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
		if role == roleHostReport {
			binding, ok := positiveBindings[result.Name]
			if !ok || result.PackageFixtureDigest != binding.PackageFixtureDigest ||
				result.EffectiveInputDigest != binding.EffectiveInputDigest {
				return fmt.Errorf("%s positive fixture %q does not bind the exact package and effective input bytes", role, result.Name)
			}
		} else if result.PackageFixtureDigest != "" || result.EffectiveInputDigest != "" {
			return fmt.Errorf("%s provider report must not claim host execution input bindings", role)
		}
	}
	negativeNames := make([]string, 0, len(report.NegativeFixtures))
	for _, result := range report.NegativeFixtures {
		if strings.TrimSpace(result.Name) == "" || !result.Passed || result.ErrorCode != standardform.InvalidArgumentErrorCode {
			return fmt.Errorf("%s negative fixture did not return %q", role, standardform.InvalidArgumentErrorCode)
		}
		negativeNames = append(negativeNames, result.Name)
		if role == roleHostReport {
			binding, ok := negativeBindings[result.Name]
			if !ok || result.PackageFixtureDigest != binding.PackageFixtureDigest ||
				result.EffectiveInputDigest != binding.EffectiveInputDigest {
				return fmt.Errorf("%s negative fixture %q does not bind the exact package and effective input bytes", role, result.Name)
			}
		} else if result.PackageFixtureDigest != "" || result.EffectiveInputDigest != "" {
			return fmt.Errorf("%s provider report must not claim host execution input bindings", role)
		}
	}
	if role == roleHostReport {
		if err := validateHostExecutionEvidence(report); err != nil {
			return fmt.Errorf("%s execution evidence: %w", role, err)
		}
	} else if report.ExecutionEvidence != nil || report.ExecutionEvidenceDigest != "" {
		return fmt.Errorf("%s provider report must not claim host execution evidence", role)
	}
	if !sameStringSet(positiveNames, positives) || !sameStringSet(negativeNames, negatives) ||
		!sameStringSet(proof.PositiveFixtures, positives) || !sameStringSet(proof.NegativeFixtures, negatives) {
		return fmt.Errorf("%s runner report fixture closure does not match admission evidence", role)
	}
	return nil
}

func validateHostExecutionEvidence(report RunnerReport) error {
	evidence := report.ExecutionEvidence
	if evidence == nil || !formpackage.ValidDigest(report.ExecutionEvidenceDigest) {
		return fmt.Errorf("embedded evidence and its canonical digest are required")
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return err
	}
	if formpackage.DigestBytes(canonical) != report.ExecutionEvidenceDigest {
		return fmt.Errorf("embedded evidence digest mismatch")
	}
	if evidence.APIVersion != "takosumi.portable-form-host-conformance/v1" || evidence.Status != "passed" ||
		!reflect.DeepEqual(evidence.Identity, report.Identity) || strings.TrimSpace(evidence.EndpointOrigin) == "" ||
		report.Subject != "host:"+evidence.EndpointOrigin || strings.TrimSpace(evidence.CanonicalResourceID) == "" {
		return fmt.Errorf("embedded evidence identity mismatch")
	}
	requiredChecks := []string{"apply", "read", "update", "delete-idempotency", "import-idempotency", "observe", "refresh", "drift"}
	for _, required := range requiredChecks {
		if !containsString(evidence.Checks, required) {
			return fmt.Errorf("embedded evidence lacks %s", required)
		}
	}
	positive := make(map[string]HostPositiveExecutionFixture, len(evidence.Fixtures.Positive))
	for _, fixture := range evidence.Fixtures.Positive {
		if fixture.Name == "" || !formpackage.ValidDigest(fixture.InputDigest) || !formpackage.ValidDigest(fixture.PackageFixtureDigest) {
			return fmt.Errorf("embedded positive fixture is invalid")
		}
		if _, duplicate := positive[fixture.Name]; duplicate {
			return fmt.Errorf("embedded positive fixture %q is duplicated", fixture.Name)
		}
		positive[fixture.Name] = fixture
	}
	negative := make(map[string]HostNegativeExecutionFixture, len(evidence.Fixtures.Negative))
	for _, fixture := range evidence.Fixtures.Negative {
		if fixture.Name == "" || fixture.Stage != "desired" || fixture.HTTPStatus != 400 || fixture.ErrorCode != standardform.InvalidArgumentErrorCode ||
			!formpackage.ValidDigest(fixture.InputDigest) || !formpackage.ValidDigest(fixture.PackageFixtureDigest) {
			return fmt.Errorf("embedded negative fixture is invalid")
		}
		if _, duplicate := negative[fixture.Name]; duplicate {
			return fmt.Errorf("embedded negative fixture %q is duplicated", fixture.Name)
		}
		negative[fixture.Name] = fixture
	}
	if len(positive) != len(report.PositiveFixtures) || len(negative) != len(report.NegativeFixtures) {
		return fmt.Errorf("embedded fixture closure differs from the report")
	}
	for _, fixture := range report.PositiveFixtures {
		executed, ok := positive[fixture.Name]
		if !ok || executed.PackageFixtureDigest != fixture.PackageFixtureDigest || executed.InputDigest != fixture.EffectiveInputDigest {
			return fmt.Errorf("embedded positive fixture %q differs from the report", fixture.Name)
		}
	}
	for _, fixture := range report.NegativeFixtures {
		executed, ok := negative[fixture.Name]
		if !ok || executed.PackageFixtureDigest != fixture.PackageFixtureDigest || executed.InputDigest != fixture.EffectiveInputDigest ||
			executed.ErrorCode != fixture.ErrorCode {
			return fmt.Errorf("embedded negative fixture %q differs from the report", fixture.Name)
		}
	}
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
		manifest.ToolingCommit != entry.ReleaseToolingCommit ||
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
	if manifest.PublisherPolicy.OIDCIssuer != packagePublisherIssuer || manifest.PublisherPolicy.Identity != packagePublisherIdentity(packageVersion) ||
		manifest.PublisherPolicy.TagPattern != packagePublisherTagPattern || manifest.PublisherPolicy.ToolingCommit != manifest.ToolingCommit {
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
	packageRoot := filepath.Join(admissionRoot, "..", "..", filepath.FromSlash(pair.candidate.PackagePath))
	localIndex, err := readRetainedRegularFile(filepath.Join(packageRoot, formpackage.PackageIndexFilename), maxEvidenceBytes)
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
	if err := verifyPackageSBOM(assetBytes[assetBase+"_sbom.spdx.json"], indexRaw, packageRoot, manifest); err != nil {
		return nil, fmt.Errorf("package release SBOM: %w", err)
	}
	if err := verifyPackageProvenance(assetBytes[assetBase+"_provenance.intoto.json"], manifest, assets); err != nil {
		return nil, fmt.Errorf("package release provenance: %w", err)
	}
	return indexRaw, nil
}

func verifyPackageSBOM(raw, canonicalIndex []byte, packageRoot string, manifest packageReleaseManifest) error {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return fmt.Errorf("invalid RFC 8785 I-JSON: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("bytes are not RFC 8785 canonical")
	}
	var document packageSBOM
	if err := decodeStrictJSON(raw, &document); err != nil {
		return fmt.Errorf("strict SPDX document: %w", err)
	}
	index, err := formpackage.ValidatePackageIndex(canonicalIndex)
	if err != nil {
		return err
	}
	if document.SPDXVersion != "SPDX-2.3" || document.DataLicense != "CC0-1.0" || document.SPDXID != "SPDXRef-DOCUMENT" ||
		document.Name != "Takoform Form Package "+manifest.FormRef.Kind+" "+manifest.PackageVersion ||
		document.DocumentNamespace != "https://forms.takoform.com/spdx/package/"+strings.TrimPrefix(manifest.PackageDigest, "sha256:") ||
		!reflect.DeepEqual(document.CreationInfo.Creators, []string{"Tool: takoform-form-package-release"}) {
		return fmt.Errorf("document identity does not bind the exact FormRef and package digest")
	}
	if _, err := time.Parse(time.RFC3339, document.CreationInfo.Created); err != nil {
		return fmt.Errorf("creationInfo.created is not RFC 3339: %w", err)
	}
	if len(document.Packages) != 1 {
		return fmt.Errorf("package closure has %d entries, want exactly 1", len(document.Packages))
	}
	verificationCode, err := packageVerificationCode(packageRoot, canonicalIndex, index)
	if err != nil {
		return err
	}
	wantPackage := spdxPackage{
		Name: manifest.FormRef.Kind, SPDXID: "SPDXRef-Package", VersionInfo: manifest.PackageVersion,
		DownloadLocation: "NOASSERTION", FilesAnalyzed: true,
		PackageVerificationCode: spdxPackageVerificationCode{Value: verificationCode},
		LicenseConcluded:        "NOASSERTION", LicenseDeclared: "NOASSERTION", CopyrightText: "NOASSERTION",
	}
	if document.Packages[0] != wantPackage {
		return fmt.Errorf("SPDX package does not bind the exact package identity and file verification code")
	}
	expectedFiles := make([]struct {
		path   string
		digest string
	}, 0, len(index.Files)+1)
	expectedFiles = append(expectedFiles, struct {
		path   string
		digest string
	}{path: formpackage.PackageIndexFilename, digest: formpackage.DigestBytes(canonicalIndex)})
	for _, file := range index.Files {
		expectedFiles = append(expectedFiles, struct {
			path   string
			digest string
		}{path: file.Path, digest: file.Digest})
	}
	if len(document.Files) != len(expectedFiles) {
		return fmt.Errorf("file closure has %d entries, want %d", len(document.Files), len(expectedFiles))
	}
	seenIDs := make(map[string]struct{}, len(document.Files))
	wantRelationships := make([]spdxRelationship, 0, len(document.Files)+1)
	wantRelationships = append(wantRelationships, spdxRelationship{
		SPDXElementID: "SPDXRef-DOCUMENT", RelationshipType: "DESCRIBES", RelatedSPDXElement: "SPDXRef-Package",
	})
	for position, expected := range expectedFiles {
		file := document.Files[position]
		digest := strings.TrimPrefix(expected.digest, "sha256:")
		wantID := "SPDXRef-File-" + releaseSPDXID(expected.path) + "-" + digest[:12]
		if file.FileName != "./"+expected.path || file.SPDXID != wantID ||
			!reflect.DeepEqual(file.Checksums, []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: digest}}) ||
			file.LicenseConcluded != "NOASSERTION" || !reflect.DeepEqual(file.LicenseInfoInFiles, []string{"NOASSERTION"}) ||
			file.CopyrightText != "NOASSERTION" {
			return fmt.Errorf("file entry %d does not bind %q and its exact SHA-256", position, expected.path)
		}
		if _, duplicate := seenIDs[file.SPDXID]; duplicate {
			return fmt.Errorf("duplicate SPDX file id %q", file.SPDXID)
		}
		seenIDs[file.SPDXID] = struct{}{}
		wantRelationships = append(wantRelationships, spdxRelationship{
			SPDXElementID: "SPDXRef-Package", RelationshipType: "CONTAINS", RelatedSPDXElement: wantID,
		})
	}
	if !reflect.DeepEqual(document.Relationships, wantRelationships) {
		return fmt.Errorf("document relationship closure does not exactly DESCRIBE the package and CONTAIN every file in order")
	}
	return nil
}

func packageVerificationCode(packageRoot string, canonicalIndex []byte, index formpackage.PackageIndex) (string, error) {
	digests := make([]string, 0, len(index.Files)+1)
	appendDigest := func(raw []byte) {
		digest := sha1.Sum(raw) // SPDX 2.3 defines the package verification code in terms of SHA-1 file checksums.
		digests = append(digests, hex.EncodeToString(digest[:]))
	}
	appendDigest(canonicalIndex)
	for _, file := range index.Files {
		raw, err := readRetainedRegularFile(filepath.Join(packageRoot, filepath.FromSlash(file.Path)), maxReleaseAssetBytes)
		if err != nil {
			return "", fmt.Errorf("read local package file %q for SPDX verification: %w", file.Path, err)
		}
		if int64(len(raw)) != file.Size || formpackage.DigestBytes(raw) != file.Digest {
			return "", fmt.Errorf("local package file %q drifted from the signed index", file.Path)
		}
		appendDigest(raw)
	}
	sort.Strings(digests)
	code := sha1.Sum([]byte(strings.Join(digests, "")))
	return hex.EncodeToString(code[:]), nil
}

func releaseSPDXID(value string) string {
	var builder strings.Builder
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '.' || current == '-' {
			builder.WriteRune(current)
		} else {
			builder.WriteRune('-')
		}
	}
	return builder.String()
}

func verifyPackageProvenance(raw []byte, manifest packageReleaseManifest, assets map[string]releaseAsset) error {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return fmt.Errorf("invalid RFC 8785 I-JSON: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("bytes are not RFC 8785 canonical")
	}
	var statement packageProvenance
	if err := decodeStrictJSON(raw, &statement); err != nil {
		return fmt.Errorf("strict in-toto statement: %w", err)
	}
	archiveName := strings.TrimSuffix(manifest.SignedSubject, "_package-index.json") + ".tar.gz"
	expectedSubjects := make([]provenanceSubject, 0, 2)
	for _, name := range []string{manifest.SignedSubject, archiveName} {
		asset, ok := assets[name]
		if !ok {
			return fmt.Errorf("required provenance subject %q is absent from the release", name)
		}
		expectedSubjects = append(expectedSubjects, provenanceSubject{
			Name: name, Digest: map[string]string{"sha256": strings.TrimPrefix(asset.Digest, "sha256:")},
		})
	}
	sort.Slice(expectedSubjects, func(i, j int) bool { return expectedSubjects[i].Name < expectedSubjects[j].Name })
	wantPredicate := provenancePredicate{
		BuildDefinition: provenanceBuildDefinition{
			BuildType:          "https://forms.takoform.com/buildtypes/data-release/v1",
			ExternalParameters: map[string]string{"tag": manifest.Tag},
			InternalParameters: map[string]string{"canonicalization": "RFC8785"},
			ResolvedDependencies: []provenanceDependency{{
				Name: "tagged-release-source", URI: "git+https://" + manifest.SourceRepository,
				Digest: map[string]string{"gitCommit": manifest.SourceCommit},
			}, {
				Name: "protected-main-release-tooling", URI: "git+https://" + manifest.SourceRepository,
				Digest: map[string]string{"gitCommit": manifest.ToolingCommit},
			}},
		},
		RunDetails: provenanceRunDetails{Builder: provenanceBuilder{
			ID: "https://" + manifest.SourceRepository + "/" + manifest.Workflow + "@" + manifest.ToolingCommit,
		}},
	}
	if statement.Type != "https://in-toto.io/Statement/v1" || statement.PredicateType != "https://slsa.dev/provenance/v1" ||
		!reflect.DeepEqual(statement.Subject, expectedSubjects) || !reflect.DeepEqual(statement.Predicate, wantPredicate) {
		return fmt.Errorf("statement does not bind the exact index/archive, source, tag, commit, workflow, and canonicalization")
	}
	return nil
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
