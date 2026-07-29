package admissionrelease

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	currentProviderManifestFormat = "takoform.standard-provider-report-candidate@v1"
	currentProviderSignedFormat   = "takoform.standard-provider-report-signed-candidate@v1"
	currentProviderWorkflow       = ".github/workflows/standard-provider-report.yml"
	currentProviderIdentity       = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/standard-provider-report.yml@refs/heads/main"
	currentProviderRepository     = "https://github.com/tako0614/terraform-provider-takoform.git"
	currentProviderSubject        = "provider:registry.terraform.io/tako0614/takoform"
)

var providerClosurePositiveDecimalPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
var providerClosureChecksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type providerClosureIdentity struct {
	sourceCommit  string
	runnerVersion string
}

type providerClosureSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

type providerClosureManifestEntry struct {
	Kind       string                              `json:"kind"`
	Slug       string                              `json:"slug"`
	Path       string                              `json:"path"`
	BundlePath string                              `json:"bundlePath"`
	Digest     string                              `json:"digest"`
	Identity   standardform.InstalledFormReference `json:"identity"`
}

type providerClosureManifest struct {
	Format        string                         `json:"format"`
	Status        string                         `json:"status"`
	ProofType     string                         `json:"proofType"`
	Subject       string                         `json:"subject"`
	Generation    string                         `json:"generation"`
	RunnerVersion string                         `json:"runnerVersion"`
	Source        providerClosureSource          `json:"source"`
	Reports       []providerClosureManifestEntry `json:"reports"`
}

type providerClosureSignedEntry struct {
	Kind         string `json:"kind"`
	Slug         string `json:"slug"`
	ReportPath   string `json:"reportPath"`
	ReportDigest string `json:"reportDigest"`
	BundlePath   string `json:"bundlePath"`
	BundleDigest string `json:"bundleDigest"`
}

type providerClosureRetainedRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type providerClosureSignedManifest struct {
	Format              string                       `json:"format"`
	Status              string                       `json:"status"`
	ProofType           string                       `json:"proofType"`
	Generation          string                       `json:"generation"`
	Subject             string                       `json:"subject"`
	CertificateIdentity string                       `json:"certificateIdentity"`
	Workflow            string                       `json:"workflow"`
	WorkflowRunID       string                       `json:"workflowRunId"`
	WorkflowRunAttempt  int                          `json:"workflowRunAttempt"`
	Source              providerClosureSource        `json:"source"`
	Manifest            providerClosureRetainedRef   `json:"manifest"`
	Entries             []providerClosureSignedEntry `json:"entries"`
}

type providerClosureInventory struct {
	Format              string                          `json:"format"`
	Classification      string                          `json:"classification"`
	Generation          string                          `json:"generation"`
	LocalConformance    string                          `json:"localConformance"`
	PublicationReady    bool                            `json:"publicationReady"`
	AdmissionStatus     string                          `json:"admissionStatus"`
	ExternalRequired    []string                        `json:"externalRequired"`
	ConformanceManifest string                          `json:"conformanceManifest"`
	Packages            []providerClosureInventoryEntry `json:"packages"`
}

type providerClosureInventoryEntry struct {
	Kind            string              `json:"kind"`
	Path            string              `json:"path"`
	AdmissionStatus string              `json:"admissionStatus"`
	ConformanceCase string              `json:"conformanceCase"`
	FormRef         formpackage.FormRef `json:"formRef"`
	PackageDigest   string              `json:"packageDigest"`
}

func verifyFullProviderReportClosure(root, retainedRoot string, set Set, selected map[string]struct{}) ([]RetainedSubject, providerClosureIdentity, error) {
	closure := set.ProviderReportClosure
	if closure == nil {
		return nil, providerClosureIdentity{}, nil
	}
	read := func(relative string, maximum int64) ([]byte, error) {
		return readRetainedRelativeFile(root, path.Join(retainedRoot, relative), maximum)
	}
	manifestRaw, err := read(closure.ManifestPath, maxReportBytes)
	if err != nil {
		return nil, providerClosureIdentity{}, err
	}
	signedRaw, err := read(closure.SignedManifestPath, maxReportBytes)
	if err != nil {
		return nil, providerClosureIdentity{}, err
	}
	checksumsRaw, err := read(closure.ChecksumsPath, maxReportBytes)
	if err != nil {
		return nil, providerClosureIdentity{}, err
	}
	if formpackage.DigestBytes(manifestRaw) != closure.ManifestDigest ||
		formpackage.DigestBytes(signedRaw) != closure.SignedManifestDigest ||
		formpackage.DigestBytes(checksumsRaw) != closure.ChecksumsDigest {
		return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure metadata digest mismatch")
	}
	for name, raw := range map[string][]byte{
		closure.ManifestPath:       manifestRaw,
		closure.SignedManifestPath: signedRaw,
	} {
		canonical, err := formpackage.Canonicalize(raw)
		if err != nil || !bytes.Equal(raw, canonical) {
			return nil, providerClosureIdentity{}, fmt.Errorf("%s is not exact canonical JSON", name)
		}
	}
	var manifest providerClosureManifest
	if err := decodeStrictJSON(manifestRaw, &manifest); err != nil {
		return nil, providerClosureIdentity{}, err
	}
	var signed providerClosureSignedManifest
	if err := decodeStrictJSON(signedRaw, &signed); err != nil {
		return nil, providerClosureIdentity{}, err
	}
	if manifest.Format != currentProviderManifestFormat ||
		signed.Format != currentProviderSignedFormat ||
		manifest.Status != "candidate-only" || signed.Status != "candidate-only" ||
		manifest.ProofType != "provider" || signed.ProofType != "provider" ||
		manifest.Generation != closure.Generation || signed.Generation != closure.Generation ||
		manifest.Subject != currentProviderSubject || signed.Subject != manifest.Subject ||
		manifest.RunnerVersion == "" ||
		manifest.Source.Repository != currentProviderRepository ||
		!releaseCommitPattern.MatchString(manifest.Source.Commit) ||
		signed.Source != manifest.Source ||
		signed.CertificateIdentity != currentProviderIdentity ||
		signed.Workflow != currentProviderWorkflow ||
		signed.WorkflowRunAttempt != 1 ||
		!providerClosurePositiveDecimalPattern.MatchString(signed.WorkflowRunID) ||
		signed.Manifest != (providerClosureRetainedRef{Path: "provider-report-manifest.json", Digest: formpackage.DigestBytes(manifestRaw)}) ||
		len(manifest.Reports) != len(closure.Reports) ||
		len(signed.Entries) != len(closure.Reports) {
		return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure manifest identity is invalid")
	}
	inventoryRaw, err := readRetainedRelativeFile(root, "forms/standard-package-set.json", maxEvidenceBytes)
	if err != nil {
		return nil, providerClosureIdentity{}, fmt.Errorf("read current portable inventory: %w", err)
	}
	var inventory providerClosureInventory
	if err := decodeStrictJSON(inventoryRaw, &inventory); err != nil {
		return nil, providerClosureIdentity{}, fmt.Errorf("decode current portable inventory: %w", err)
	}
	if inventory.Format != "takoform.standard-package-set@v1" ||
		inventory.Classification != "structural-candidate" ||
		inventory.Generation != closure.Generation ||
		inventory.LocalConformance != "structural-only" ||
		inventory.PublicationReady ||
		inventory.AdmissionStatus != "external-required" ||
		inventory.ConformanceManifest != "conformance/form-package-v1/manifest.json" ||
		len(inventory.Packages) != len(closure.Reports) {
		return nil, providerClosureIdentity{}, fmt.Errorf("current portable inventory identity is invalid")
	}
	inventoryByKind := make(map[string]providerClosureInventoryEntry, len(inventory.Packages))
	for _, item := range inventory.Packages {
		if item.AdmissionStatus != "external-required" || item.Kind != item.FormRef.Kind ||
			!formpackage.ValidDigest(item.FormRef.SchemaDigest) || !formpackage.ValidDigest(item.PackageDigest) {
			return nil, providerClosureIdentity{}, fmt.Errorf("current portable inventory contains invalid %s identity", item.Kind)
		}
		if _, duplicate := inventoryByKind[item.Kind]; duplicate {
			return nil, providerClosureIdentity{}, fmt.Errorf("current portable inventory duplicates %s", item.Kind)
		}
		inventoryByKind[item.Kind] = item
	}

	manifestByKind := make(map[string]providerClosureManifestEntry, len(manifest.Reports))
	for _, report := range manifest.Reports {
		if _, duplicate := manifestByKind[report.Kind]; duplicate {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report manifest duplicates %s", report.Kind)
		}
		manifestByKind[report.Kind] = report
	}
	signedByKind := make(map[string]providerClosureSignedEntry, len(signed.Entries))
	for _, report := range signed.Entries {
		if _, duplicate := signedByKind[report.Kind]; duplicate {
			return nil, providerClosureIdentity{}, fmt.Errorf("signed provider-report closure duplicates %s", report.Kind)
		}
		signedByKind[report.Kind] = report
	}
	checksums, err := parseProviderClosureChecksums(checksumsRaw)
	if err != nil {
		return nil, providerClosureIdentity{}, err
	}
	expectedChecksumEntries := len(closure.Reports)*2 + 2
	if len(checksums) != expectedChecksumEntries {
		return nil, providerClosureIdentity{}, fmt.Errorf("provider-report SHA256SUMS has %d entries, want %d", len(checksums), expectedChecksumEntries)
	}
	for relative, digest := range map[string]string{
		"provider-report-manifest.json":         closure.ManifestDigest,
		"signed-provider-report-candidate.json": closure.SignedManifestDigest,
	} {
		if checksums[relative] != digest {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report SHA256SUMS does not bind %s", relative)
		}
	}

	subjects := make([]RetainedSubject, 0, len(closure.Reports)-len(selected))
	for _, retained := range closure.Reports {
		inventoryEntry, inventoryOK := inventoryByKind[retained.Kind]
		if !inventoryOK ||
			filepath.Base(inventoryEntry.Path) != retained.Slug ||
			inventoryEntry.FormRef != retained.Identity.FormRef ||
			inventoryEntry.PackageDigest != retained.Identity.PackageDigest {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure %s is not the exact current portable identity", retained.Kind)
		}
		manifestEntry, ok := manifestByKind[retained.Kind]
		signedEntry, signedOK := signedByKind[retained.Kind]
		relativeReport := strings.TrimPrefix(retained.ReportPath, "provider-closure/")
		relativeBundle := strings.TrimPrefix(retained.SigstoreBundle, "provider-closure/")
		if !ok || !signedOK ||
			manifestEntry.Kind != retained.Kind || manifestEntry.Slug != retained.Slug ||
			manifestEntry.Path != relativeReport || manifestEntry.BundlePath != relativeBundle ||
			manifestEntry.Digest != retained.ReportDigest ||
			!reflect.DeepEqual(manifestEntry.Identity, retained.Identity) ||
			signedEntry.Kind != retained.Kind || signedEntry.Slug != retained.Slug ||
			signedEntry.ReportPath != relativeReport || signedEntry.ReportDigest != retained.ReportDigest ||
			signedEntry.BundlePath != relativeBundle {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure metadata differs for %s", retained.Kind)
		}
		reportRaw, err := read(retained.ReportPath, maxReportBytes)
		if err != nil {
			return nil, providerClosureIdentity{}, err
		}
		bundleRaw, err := read(retained.SigstoreBundle, maxSigstoreBundleBytes)
		if err != nil {
			return nil, providerClosureIdentity{}, err
		}
		if formpackage.DigestBytes(reportRaw) != retained.ReportDigest ||
			formpackage.DigestBytes(bundleRaw) != signedEntry.BundleDigest ||
			checksums[relativeReport] != retained.ReportDigest ||
			checksums[relativeBundle] != formpackage.DigestBytes(bundleRaw) {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure bytes differ for %s", retained.Kind)
		}
		packageRoot := filepath.Join(root, "forms", "releases", releaseIDForKind(retained.Kind), retained.Identity.FormRef.DefinitionVersion)
		packageReport, err := formpackage.VerifyDirectory(packageRoot)
		if err != nil {
			return nil, providerClosureIdentity{}, fmt.Errorf("%s provider-report package: %w", retained.Kind, err)
		}
		if packageReport.FormRef != retained.Identity.FormRef || packageReport.PackageDigest != retained.Identity.PackageDigest {
			return nil, providerClosureIdentity{}, fmt.Errorf("%s provider-report package identity drift", retained.Kind)
		}
		definition, err := readDefinition(packageRoot)
		if err != nil {
			return nil, providerClosureIdentity{}, fmt.Errorf("%s provider-report definition: %w", retained.Kind, err)
		}
		positives := make([]string, 0, len(definition.ConformanceFixtures))
		for _, fixture := range definition.ConformanceFixtures {
			positives = append(positives, fixture.Name)
		}
		negatives := make([]string, 0, len(definition.NegativeFixtures))
		for _, fixture := range definition.NegativeFixtures {
			negatives = append(negatives, fixture.Name)
		}
		report, err := ValidateCanonicalProviderRunnerReport(reportRaw, retained.Identity, positives, negatives)
		if err != nil {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure report %s is invalid: %w", retained.Kind, err)
		}
		if report.Subject != manifest.Subject || report.RunnerVersion != manifest.RunnerVersion {
			return nil, providerClosureIdentity{}, fmt.Errorf("provider-report closure report %s subject or runner version differs from its manifest", retained.Kind)
		}
		if _, alreadySelected := selected[retained.Kind]; !alreadySelected {
			subjects = append(subjects, RetainedSubject{
				Kind: retained.Kind, Role: roleProviderReport,
				Path: retained.ReportPath, Canonical: reportRaw,
				SigstorePath: retained.SigstoreBundle,
			})
		}
	}
	return subjects, providerClosureIdentity{
		sourceCommit: manifest.Source.Commit, runnerVersion: manifest.RunnerVersion,
	}, nil
}

func parseProviderClosureChecksums(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("provider-report SHA256SUMS must end with one newline")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	result := make(map[string]string, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || !providerClosureChecksumPattern.MatchString(parts[0]) {
			return nil, fmt.Errorf("invalid provider-report SHA256SUMS line %q", line)
		}
		if err := validateRelativePath(parts[1]); err != nil {
			return nil, fmt.Errorf("invalid provider-report SHA256SUMS path %q: %w", parts[1], err)
		}
		metadata := parts[1] == "provider-report-manifest.json" || parts[1] == "signed-provider-report-candidate.json"
		reportArtifact := strings.HasPrefix(parts[1], "packages/") &&
			(strings.HasSuffix(parts[1], "/provider-report.json") || strings.HasSuffix(parts[1], "/provider-report.sigstore.json"))
		if !metadata && !reportArtifact {
			return nil, fmt.Errorf("provider-report SHA256SUMS contains unexpected path %q", parts[1])
		}
		if _, duplicate := result[parts[1]]; duplicate {
			return nil, fmt.Errorf("provider-report SHA256SUMS duplicates %s", parts[1])
		}
		result[parts[1]] = "sha256:" + parts[0]
	}
	return result, nil
}
