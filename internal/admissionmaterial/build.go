// Package admissionmaterial builds non-publishable standard-admission source
// material from independently signed host and provider report candidates.
package admissionmaterial

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	hostManifestName       = "host-report-manifest.json"
	hostSignedName         = "signed-host-report-candidate.json"
	providerManifestName   = "provider-report-manifest.json"
	providerSignedName     = "signed-provider-report-candidate.json"
	checksumsName          = "SHA256SUMS"
	hostManifestFormat     = "takosumi.standard-form-host-report-candidate@v1"
	hostSignedFormat       = "takosumi.standard-form-host-report-signed-candidate@v1"
	providerManifestFormat = "takoform.standard-provider-report-candidate@v1"
	providerSignedFormat   = "takoform.standard-provider-report-signed-candidate@v1"
	hostWorkflow           = ".github/workflows/standard-form-host-report.yml"
	providerWorkflow       = ".github/workflows/standard-provider-report.yml"
	hostIdentity           = "https://github.com/tako0614/takosumi/.github/workflows/standard-form-host-report.yml@refs/heads/main"
	providerIdentity       = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/standard-provider-report.yml@refs/heads/main"
	hostProofType          = "oss-reference-host-source-conformance"
	hostSubject            = "host:https://in-process.takosumi.test"
	providerSubject        = "provider:registry.opentofu.org/tako0614/takoform"
	bundleMediaType        = "application/vnd.dev.sigstore.bundle.v0.3+json"
	maximumMaterialBytes   = 16 << 20
)

var (
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-.][0-9A-Za-z.-]+)?$`)
)

type BuildOptions struct {
	Root                     string
	HostReports              string
	ProviderReports          string
	OutputDir                string
	AdmissionVersion         string
	SourceCommit             string
	HostSourceCommit         string
	HostTakoformSourceCommit string
	ProviderSourceCommit     string
	HostWorkflowRunID        string
	ProviderWorkflowRunID    string
}

type sourceRef struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

type reportManifestEntry struct {
	Kind       string                              `json:"kind"`
	Slug       string                              `json:"slug"`
	Path       string                              `json:"path"`
	BundlePath string                              `json:"bundlePath"`
	Digest     string                              `json:"digest"`
	Identity   standardform.InstalledFormReference `json:"identity"`
}

type reportManifest struct {
	Format            string                `json:"format"`
	Status            string                `json:"status"`
	ProofType         string                `json:"proofType"`
	Subject           string                `json:"subject"`
	DefinitionVersion string                `json:"definitionVersion"`
	PackageVersion    string                `json:"packageVersion"`
	RunnerVersion     string                `json:"runnerVersion"`
	Source            sourceRef             `json:"source"`
	TakoformSource    *sourceRef            `json:"takoformSource,omitempty"`
	Reports           []reportManifestEntry `json:"reports"`
}

type signedEntry struct {
	Kind         string `json:"kind"`
	Slug         string `json:"slug"`
	ReportPath   string `json:"reportPath"`
	ReportDigest string `json:"reportDigest"`
	BundlePath   string `json:"bundlePath"`
	BundleDigest string `json:"bundleDigest"`
}

type retainedRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type signedManifest struct {
	Format              string        `json:"format"`
	Status              string        `json:"status"`
	ProofType           string        `json:"proofType"`
	Subject             string        `json:"subject"`
	CertificateIdentity string        `json:"certificateIdentity"`
	Workflow            string        `json:"workflow"`
	WorkflowRunID       string        `json:"workflowRunId"`
	WorkflowRunAttempt  int           `json:"workflowRunAttempt"`
	Source              sourceRef     `json:"source"`
	TakoformSource      *sourceRef    `json:"takoformSource,omitempty"`
	Manifest            retainedRef   `json:"manifest"`
	Entries             []signedEntry `json:"entries"`
}

type reportArtifact struct {
	entry  reportManifestEntry
	report admissionrelease.RunnerReport
	raw    []byte
	bundle []byte
}

type artifactSet struct {
	manifest reportManifest
	byKind   map[string]reportArtifact
}

// Build verifies exact artifact inventories and writes a new, non-publishable
// admission material directory. Signature cryptography is performed by the
// protected workflow before this function is called; bundle presence and media
// type remain part of this deterministic input contract.
func Build(options BuildOptions) error {
	if !commitPattern.MatchString(options.SourceCommit) {
		return fmt.Errorf("source commit must be lowercase 40-hex")
	}
	if !commitPattern.MatchString(options.HostSourceCommit) {
		return fmt.Errorf("host source commit must be lowercase 40-hex")
	}
	if !commitPattern.MatchString(options.HostTakoformSourceCommit) {
		return fmt.Errorf("host Takoform source commit must be lowercase 40-hex")
	}
	if !commitPattern.MatchString(options.ProviderSourceCommit) {
		return fmt.Errorf("provider source commit must be lowercase 40-hex")
	}
	for label, value := range map[string]string{
		"host workflow run id": options.HostWorkflowRunID, "provider workflow run id": options.ProviderWorkflowRunID,
	} {
		if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(value) {
			return fmt.Errorf("%s must be a positive decimal integer", label)
		}
	}
	if !versionPattern.MatchString(options.AdmissionVersion) {
		return fmt.Errorf("admission version is not a canonical version token")
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return err
	}
	output, err := prepareOutputPath(root, options.OutputDir)
	if err != nil {
		return err
	}
	candidates, err := standardforms.AdmissionCandidateSet()
	if err != nil {
		return err
	}
	if candidates.DefinitionVersion != "1.0.1" || candidates.PackageVersion != "1.0.1" || len(candidates.Entries) != 10 {
		return fmt.Errorf("admission material requires the exact Standard Form 1.0.1 ten-package candidate")
	}
	if err := standardforms.VerifyPublishedPackageSet(root); err != nil {
		return fmt.Errorf("published package closure: %w", err)
	}
	if err := admissionrelease.VerifyOfflineTrust(filepath.Join(root, "admission", "v1")); err != nil {
		return fmt.Errorf("offline admission trust: %w", err)
	}
	published, err := loadPublishedSet(root, candidates)
	if err != nil {
		return err
	}
	providerVersion, err := loadProviderVersion(root)
	if err != nil {
		return err
	}
	hosts, err := loadArtifactSet(options.HostReports, "host-report", candidates, options.HostTakoformSourceCommit, options.HostSourceCommit, options.HostWorkflowRunID, "", providerVersion)
	if err != nil {
		return fmt.Errorf("host report candidate: %w", err)
	}
	providers, err := loadArtifactSet(options.ProviderReports, "provider-report", candidates, options.ProviderSourceCommit, options.ProviderSourceCommit, options.ProviderWorkflowRunID, providerSubject, providerVersion)
	if err != nil {
		return fmt.Errorf("provider report candidate: %w", err)
	}

	if err := os.Mkdir(output, 0o700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(output)
		}
	}()

	entries := make([]admissionrelease.SetEntry, 0, len(candidates.Entries))
	for _, candidate := range candidates.Entries {
		host := hosts.byKind[candidate.Kind]
		provider := providers.byKind[candidate.Kind]
		packageRoot := filepath.Join(root, filepath.FromSlash(candidate.PackagePath))
		_, canonical, err := buildEvidence(packageRoot, candidate, host, provider)
		if err != nil {
			return fmt.Errorf("%s evidence: %w", candidate.Kind, err)
		}
		directory := path.Join("packages", candidate.Slug)
		for _, file := range []struct {
			name string
			raw  []byte
		}{
			{path.Join(directory, "host-report.json"), host.raw},
			{path.Join(directory, "host-report.sigstore.json"), host.bundle},
			{path.Join(directory, "provider-report.json"), provider.raw},
			{path.Join(directory, "provider-report.sigstore.json"), provider.bundle},
			{path.Join(directory, "evidence.json"), canonical},
		} {
			if err := writeCreateOnly(output, file.name, file.raw); err != nil {
				return err
			}
		}
		publishedEntry := published[candidate.Kind]
		entries = append(entries, admissionrelease.SetEntry{
			Kind: candidate.Kind, Slug: candidate.Slug, FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest,
			ReleaseTag: publishedEntry.ReleaseTag, ReleaseCommit: publishedEntry.ReleaseCommit, ReleaseToolingCommit: publishedEntry.ReleaseToolingCommit,
			PackageReleaseManifestPath: publishedEntry.PackageReleaseManifestPath, PackageReleaseManifestDigest: publishedEntry.PackageReleaseManifestDigest,
			PackageIndexPath: publishedEntry.PackageIndexPath, PackageIndexSigstoreBundle: publishedEntry.PackageIndexSigstoreBundle,
			EvidencePath: path.Join(directory, "evidence.json"), EvidenceDigest: formpackage.DigestBytes(canonical),
			HostReportPath: path.Join(directory, "host-report.json"), HostReportDigest: formpackage.DigestBytes(host.raw), HostReportSigstoreBundle: path.Join(directory, "host-report.sigstore.json"),
			ProviderReportPath: path.Join(directory, "provider-report.json"), ProviderReportDigest: formpackage.DigestBytes(provider.raw), ProviderReportSigstoreBundle: path.Join(directory, "provider-report.sigstore.json"),
			AdmissionStatus: "portable-standard",
		})
	}
	registryRaw, err := readRegular(root, "admission/v1/registry/provider-readback.json", maximumMaterialBytes)
	if err != nil {
		return fmt.Errorf("provider Registry readback: %w", err)
	}
	_, setRaw, err := admissionrelease.BuildCanonicalSet(candidates, "forms/admissions/v"+options.AdmissionVersion, admissionrelease.RegistryReadbackRef{
		Path: "registry/provider-readback.json", Digest: formpackage.DigestBytes(registryRaw), SigstoreBundle: "registry/provider-readback.sigstore.json",
	}, entries)
	if err != nil {
		return err
	}
	if err := writeCreateOnly(output, "standard-admission-set.json", setRaw); err != nil {
		return err
	}
	complete = true
	return nil
}

func loadArtifactSet(root, role string, candidates admissionrelease.CandidateSet, takoformSourceCommit, expectedSourceCommit, expectedWorkflowRunID, expectedSubject, providerVersion string) (artifactSet, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return artifactSet{}, err
	}
	manifestName, signedName := hostManifestName, hostSignedName
	manifestFormat, signedFormat := hostManifestFormat, hostSignedFormat
	workflow, identity := hostWorkflow, hostIdentity
	if role == "provider-report" {
		manifestName, signedName = providerManifestName, providerSignedName
		manifestFormat, signedFormat = providerManifestFormat, providerSignedFormat
		workflow, identity = providerWorkflow, providerIdentity
	}
	manifestRaw, err := readRegular(absolute, manifestName, maximumMaterialBytes)
	if err != nil {
		return artifactSet{}, err
	}
	if err := requireCanonical(manifestRaw, manifestName); err != nil {
		return artifactSet{}, err
	}
	var manifest reportManifest
	if err := decodeStrict(manifestRaw, &manifest); err != nil {
		return artifactSet{}, err
	}
	signedRaw, err := readRegular(absolute, signedName, maximumMaterialBytes)
	if err != nil {
		return artifactSet{}, err
	}
	if err := requireCanonical(signedRaw, signedName); err != nil {
		return artifactSet{}, err
	}
	var signed signedManifest
	if err := decodeStrict(signedRaw, &signed); err != nil {
		return artifactSet{}, err
	}
	if manifest.Format != manifestFormat || signed.Format != signedFormat || manifest.Status != "candidate-only" || signed.Status != "candidate-only" ||
		manifest.DefinitionVersion != candidates.DefinitionVersion || manifest.PackageVersion != candidates.PackageVersion || manifest.Subject == "" || manifest.RunnerVersion == "" ||
		signed.Subject != manifest.Subject || signed.ProofType != manifest.ProofType || signed.CertificateIdentity != identity || signed.Workflow != workflow || signed.WorkflowRunID != expectedWorkflowRunID || signed.WorkflowRunAttempt != 1 {
		return artifactSet{}, fmt.Errorf("candidate manifest identity or workflow closure is invalid")
	}
	if expectedSubject != "" && manifest.Subject != expectedSubject {
		return artifactSet{}, fmt.Errorf("subject is %q, want %q", manifest.Subject, expectedSubject)
	}
	if role == "provider-report" && manifest.RunnerVersion != providerVersion {
		return artifactSet{}, fmt.Errorf("provider runner version is %q, want %q", manifest.RunnerVersion, providerVersion)
	}
	if role == "host-report" {
		if manifest.ProofType != hostProofType || manifest.Subject != hostSubject || manifest.Source.Repository != "https://github.com/tako0614/takosumi.git" || manifest.Source.Commit != expectedSourceCommit || manifest.RunnerVersion != "1.1.0+git."+manifest.Source.Commit || manifest.TakoformSource == nil || manifest.TakoformSource.Repository != "https://github.com/tako0614/terraform-provider-takoform.git" || manifest.TakoformSource.Commit != takoformSourceCommit || signed.TakoformSource == nil || *signed.TakoformSource != *manifest.TakoformSource {
			return artifactSet{}, fmt.Errorf("host candidate source binding is invalid")
		}
	} else if manifest.ProofType != "provider" || manifest.Source.Repository != "https://github.com/tako0614/terraform-provider-takoform.git" || manifest.Source.Commit != expectedSourceCommit || manifest.TakoformSource != nil || signed.TakoformSource != nil {
		return artifactSet{}, fmt.Errorf("provider candidate source binding is invalid")
	}
	if signed.Source != manifest.Source || signed.Manifest.Path != manifestName || signed.Manifest.Digest != formpackage.DigestBytes(manifestRaw) {
		return artifactSet{}, fmt.Errorf("signed candidate does not bind its manifest and source")
	}
	if len(manifest.Reports) != 10 || len(signed.Entries) != 10 {
		return artifactSet{}, fmt.Errorf("candidate must contain exactly ten reports")
	}
	expectedFiles := map[string]struct{}{manifestName: {}, signedName: {}, checksumsName: {}}
	bySigned := make(map[string]signedEntry, 10)
	for _, entry := range signed.Entries {
		if _, duplicate := bySigned[entry.Kind]; duplicate {
			return artifactSet{}, fmt.Errorf("signed candidate duplicates %s", entry.Kind)
		}
		bySigned[entry.Kind] = entry
	}
	byKind := make(map[string]reportArtifact, 10)
	for _, candidate := range candidates.Entries {
		var entry *reportManifestEntry
		for index := range manifest.Reports {
			if manifest.Reports[index].Kind == candidate.Kind {
				entry = &manifest.Reports[index]
				break
			}
		}
		if entry == nil || entry.Slug != candidate.Slug || !reflect.DeepEqual(entry.Identity, standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest}) {
			return artifactSet{}, fmt.Errorf("candidate omits exact %s identity", candidate.Kind)
		}
		wantReport := path.Join("packages", candidate.Slug, role+".json")
		wantBundle := path.Join("packages", candidate.Slug, role+".sigstore.json")
		if entry.Path != wantReport || entry.BundlePath != wantBundle || !formpackage.ValidDigest(entry.Digest) {
			return artifactSet{}, fmt.Errorf("%s paths or digest are not canonical", candidate.Kind)
		}
		reportRaw, err := readRegular(absolute, entry.Path, maximumMaterialBytes)
		if err != nil {
			return artifactSet{}, err
		}
		bundleRaw, err := readRegular(absolute, entry.BundlePath, maximumMaterialBytes)
		if err != nil {
			return artifactSet{}, err
		}
		if formpackage.DigestBytes(reportRaw) != entry.Digest {
			return artifactSet{}, fmt.Errorf("%s report digest mismatch", candidate.Kind)
		}
		if err := validateBundle(bundleRaw); err != nil {
			return artifactSet{}, fmt.Errorf("%s bundle: %w", candidate.Kind, err)
		}
		var parsed admissionrelease.RunnerReport
		if role == "provider-report" {
			parsed, err = admissionrelease.ValidateCanonicalProviderRunnerReport(reportRaw, entry.Identity, []string{"canonical"}, []string{"reject-invalid-semantics"})
		} else {
			// Exact host fixture byte bindings are checked during evidence assembly,
			// where the package root is available.
			if err := requireCanonical(reportRaw, entry.Path); err == nil {
				err = decodeStrict(reportRaw, &parsed)
			}
		}
		if err != nil || parsed.Role != role || parsed.Subject != manifest.Subject || parsed.RunnerVersion != manifest.RunnerVersion || !reflect.DeepEqual(parsed.Identity, entry.Identity) {
			return artifactSet{}, fmt.Errorf("%s report identity or fixture closure is invalid: %w", candidate.Kind, err)
		}
		signedEntry, ok := bySigned[candidate.Kind]
		if !ok || signedEntry.Slug != candidate.Slug || signedEntry.ReportPath != entry.Path || signedEntry.ReportDigest != entry.Digest || signedEntry.BundlePath != entry.BundlePath || signedEntry.BundleDigest != formpackage.DigestBytes(bundleRaw) {
			return artifactSet{}, fmt.Errorf("signed candidate does not close over %s", candidate.Kind)
		}
		expectedFiles[entry.Path], expectedFiles[entry.BundlePath] = struct{}{}, struct{}{}
		byKind[candidate.Kind] = reportArtifact{entry: *entry, report: parsed, raw: reportRaw, bundle: bundleRaw}
	}
	files, err := listRegularFiles(absolute)
	if err != nil {
		return artifactSet{}, err
	}
	if !sameFileSet(files, expectedFiles) {
		return artifactSet{}, fmt.Errorf("candidate file inventory is not the exact 23-file closure")
	}
	if err := verifyChecksums(absolute, expectedFiles); err != nil {
		return artifactSet{}, err
	}
	return artifactSet{manifest: manifest, byKind: byKind}, nil
}

type fixtureClosure struct {
	report           formpackage.VerificationReport
	definition       formpackage.FormDefinition
	positive         []standardform.PositiveFixture
	negative         []standardform.NegativeFixture
	positiveBindings map[string]admissionrelease.FixtureDigestBinding
	negativeBindings map[string]admissionrelease.FixtureDigestBinding
}

func loadFixtureClosure(packageRoot string) (fixtureClosure, error) {
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return fixtureClosure{}, err
	}
	indexRaw, err := os.ReadFile(filepath.Join(packageRoot, formpackage.PackageIndexFilename))
	if err != nil {
		return fixtureClosure{}, err
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return fixtureClosure{}, err
	}
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(index.DefinitionPath)))
	if err != nil {
		return fixtureClosure{}, err
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return fixtureClosure{}, err
	}
	result := fixtureClosure{report: report, definition: definition, positiveBindings: map[string]admissionrelease.FixtureDigestBinding{}, negativeBindings: map[string]admissionrelease.FixtureDigestBinding{}}
	for _, fixture := range definition.ConformanceFixtures {
		desiredRaw, desired, err := readObject(packageRoot, fixture.DesiredPath)
		if err != nil {
			return fixtureClosure{}, err
		}
		_, observed, err := readOptionalObject(packageRoot, fixture.ObservedPath)
		if err != nil {
			return fixtureClosure{}, err
		}
		_, output, err := readOptionalObject(packageRoot, fixture.OutputPath)
		if err != nil {
			return fixtureClosure{}, err
		}
		effective, err := formpackage.DigestCanonicalJSON(desiredRaw)
		if err != nil {
			return fixtureClosure{}, err
		}
		result.positive = append(result.positive, standardform.PositiveFixture{Name: fixture.Name, Desired: desired, Observed: observed, Output: output})
		result.positiveBindings[fixture.Name] = admissionrelease.FixtureDigestBinding{PackageFixtureDigest: formpackage.DigestBytes(desiredRaw), EffectiveInputDigest: effective}
	}
	for _, fixture := range definition.NegativeFixtures {
		inputRaw, input, err := readObject(packageRoot, fixture.InputPath)
		if err != nil {
			return fixtureClosure{}, err
		}
		effective, err := formpackage.DigestCanonicalJSON(inputRaw)
		if err != nil {
			return fixtureClosure{}, err
		}
		result.negative = append(result.negative, standardform.NegativeFixture{Name: fixture.Name, Stage: fixture.Stage, Input: input, ExpectedErrorCode: standardform.InvalidArgumentErrorCode})
		result.negativeBindings[fixture.Name] = admissionrelease.FixtureDigestBinding{PackageFixtureDigest: formpackage.DigestBytes(inputRaw), EffectiveInputDigest: effective}
	}
	return result, nil
}

func buildEvidence(packageRoot string, candidate admissionrelease.Candidate, host, provider reportArtifact) (standardform.AdmissionEvidence, []byte, error) {
	fixtures, err := loadFixtureClosure(packageRoot)
	if err != nil {
		return standardform.AdmissionEvidence{}, nil, err
	}
	identity := standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest}
	if fixtures.report.FormRef != candidate.FormRef || fixtures.report.PackageDigest != candidate.PackageDigest {
		return standardform.AdmissionEvidence{}, nil, fmt.Errorf("candidate package identity drift")
	}
	hostReport, err := admissionrelease.ValidateCanonicalHostRunnerReport(host.raw, identity, fixtures.positiveBindings, fixtures.negativeBindings)
	if err != nil {
		return standardform.AdmissionEvidence{}, nil, err
	}
	positiveNames := make([]string, 0, len(fixtures.positive))
	for _, item := range fixtures.positive {
		positiveNames = append(positiveNames, item.Name)
	}
	negativeNames := make([]string, 0, len(fixtures.negative))
	for _, item := range fixtures.negative {
		negativeNames = append(negativeNames, item.Name)
	}
	providerReport, err := admissionrelease.ValidateCanonicalProviderRunnerReport(provider.raw, identity, positiveNames, negativeNames)
	if err != nil {
		return standardform.AdmissionEvidence{}, nil, err
	}
	proof := func(report admissionrelease.RunnerReport, raw []byte) standardform.ConformanceProof {
		return standardform.ConformanceProof{Subject: report.Subject, RunnerVersion: report.RunnerVersion, Identity: report.Identity, Status: "passed", PositiveFixtures: positiveNames, NegativeFixtures: negativeNames, EvidenceDigest: formpackage.DigestBytes(raw)}
	}
	evidence := standardform.AdmissionEvidence{
		APIVersion: standardform.APIVersion, Identity: identity, Classification: "portable-standard", ApprovedSchemaDigest: candidate.FormRef.SchemaDigest,
		Audit: standardform.Audit{
			Lifecycle:    standardform.LifecycleAudit{Create: true, Read: true, Update: true, Delete: true, Import: true, Observe: true, Refresh: true, Drift: true},
			Immutability: standardform.ImmutabilityAudit{Reviewed: true, Fields: append([]string(nil), fixtures.definition.ImmutableFields...)},
			Security:     standardform.SecurityAudit{SecretFreeDesiredState: true, CredentialBoundaryExternal: true, DataOnlyPackage: true},
			Interfaces:   standardform.InterfaceAudit{Reviewed: true, BindingAuthorityExternal: true, SecretFreeDocuments: true},
		},
		Fixtures:    standardform.Fixtures{Positive: fixtures.positive, Negative: fixtures.negative},
		Conformance: standardform.Conformance{Host: proof(hostReport, host.raw), Provider: proof(providerReport, provider.raw)},
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return standardform.AdmissionEvidence{}, nil, err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return standardform.AdmissionEvidence{}, nil, err
	}
	validated, err := standardform.ValidateEvidenceBytes(canonical, fixtures.report, fixtures.definition)
	if err != nil {
		return standardform.AdmissionEvidence{}, nil, err
	}
	return validated, canonical, nil
}

func loadPublishedSet(root string, candidates admissionrelease.CandidateSet) (map[string]admissionrelease.PublishedPackageEntry, error) {
	raw, err := readRegular(root, "admission/v1/published-package-set.json", maximumMaterialBytes)
	if err != nil {
		return nil, err
	}
	var set admissionrelease.PublishedPackageSet
	if err := decodeStrict(raw, &set); err != nil {
		return nil, err
	}
	if set.DefinitionVersion != candidates.DefinitionVersion || set.PackageVersion != candidates.PackageVersion || set.PublicationStatus != "published-immutable" || len(set.Entries) != len(candidates.Entries) {
		return nil, fmt.Errorf("published package set does not match the current candidate")
	}
	result := make(map[string]admissionrelease.PublishedPackageEntry, len(set.Entries))
	for _, entry := range set.Entries {
		result[entry.Kind] = entry
	}
	for _, candidate := range candidates.Entries {
		entry, ok := result[candidate.Kind]
		if !ok || entry.Slug != candidate.Slug || entry.FormRef != candidate.FormRef || entry.PackageDigest != candidate.PackageDigest {
			return nil, fmt.Errorf("published package set omits exact %s", candidate.Kind)
		}
	}
	return result, nil
}

func loadProviderVersion(root string) (string, error) {
	var descriptor struct {
		Version string `json:"version"`
	}
	raw, err := readRegular(root, "release/version.json", maximumMaterialBytes)
	if err != nil {
		return "", err
	}
	if err := decodeStrict(raw, &descriptor); err != nil {
		// release/version.json has more fields; decode only the value safely.
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		descriptor.Version, _ = value["version"].(string)
	}
	if descriptor.Version == "" {
		return "", fmt.Errorf("provider version is empty")
	}
	return descriptor.Version, nil
}

func prepareOutputPath(repoRoot, value string) (string, error) {
	output, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(output); err == nil {
		return "", fmt.Errorf("output directory already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	resolvedRepository, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return "", err
	}
	resolvedOutput := filepath.Join(parent, filepath.Base(output))
	if resolvedOutput == resolvedRepository || strings.HasPrefix(resolvedOutput, resolvedRepository+string(filepath.Separator)) {
		return "", fmt.Errorf("output directory must be outside the repository")
	}
	return resolvedOutput, nil
}

func writeCreateOnly(root, relative string, raw []byte) error {
	filename := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func readObject(root, relative string) ([]byte, map[string]any, error) {
	raw, err := readRegular(root, filepath.ToSlash(relative), maximumMaterialBytes)
	if err != nil {
		return nil, nil, err
	}
	var value map[string]any
	if err := decodeStrict(raw, &value); err != nil {
		return nil, nil, err
	}
	if value == nil {
		return nil, nil, fmt.Errorf("fixture is not an object")
	}
	return raw, value, nil
}

func readOptionalObject(root, relative string) ([]byte, map[string]any, error) {
	if relative == "" {
		return nil, map[string]any{}, nil
	}
	return readObject(root, relative)
}

func readRegular(root, relative string, maximum int64) ([]byte, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, `\`) || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") {
		return nil, fmt.Errorf("invalid relative path %q", relative)
	}
	current := root
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlinked artifact path %q", relative)
		}
	}
	info, err := os.Lstat(current)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fmt.Errorf("artifact %q is not a bounded regular file", relative)
	}
	raw, err := os.ReadFile(current)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("artifact %q exceeds limit", relative)
	}
	return raw, nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON")
		}
		return err
	}
	return nil
}

func requireCanonical(raw []byte, label string) error {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("%s is not RFC 8785 canonical", label)
	}
	return nil
}

func validateBundle(raw []byte) error {
	var value struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if value.MediaType != bundleMediaType {
		return fmt.Errorf("mediaType is %q", value.MediaType)
	}
	return nil
}

func listRegularFiles(root string) ([]string, error) {
	var result []string
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filename == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("candidate contains symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("candidate contains non-regular file")
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		result = append(result, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(result)
	return result, err
}

func sameFileSet(files []string, expected map[string]struct{}) bool {
	if len(files) != len(expected) {
		return false
	}
	for _, file := range files {
		if _, ok := expected[file]; !ok {
			return false
		}
	}
	return true
}

func verifyChecksums(root string, expected map[string]struct{}) error {
	raw, err := readRegular(root, checksumsName, maximumMaterialBytes)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "  ", 2)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return fmt.Errorf("invalid SHA256SUMS line")
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return err
		}
		name := fields[1]
		if name == checksumsName {
			return fmt.Errorf("SHA256SUMS must not checksum itself")
		}
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("SHA256SUMS lists unexpected %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("SHA256SUMS duplicates %q", name)
		}
		payload, err := readRegular(root, name, maximumMaterialBytes)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(payload)
		if hex.EncodeToString(digest[:]) != fields[0] {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
		seen[name] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(seen) != len(expected)-1 {
		return fmt.Errorf("SHA256SUMS does not close over every candidate file")
	}
	return nil
}
