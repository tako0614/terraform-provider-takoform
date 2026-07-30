package admissionmaterial

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissioncheckpoint"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
	"github.com/tako0614/terraform-provider-takoform/internal/hostpolicy"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	currentAdmissionRoot            = "admission/v4"
	currentAdmissionGeneration      = "ga-core-v2"
	currentProviderGeneration       = "portable-v1"
	registryManifestName            = "provider-registry-readback-manifest.json"
	registrySignedName              = "signed-provider-registry-readback-candidate.json"
	registryManifestFormat          = "takoform.provider-registry-readback-candidate@v1"
	registrySignedFormat            = "takoform.provider-registry-readback-signed-candidate@v1"
	registryWorkflow                = ".github/workflows/provider-registry-readback.yml"
	registryIdentity                = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/provider-registry-readback.yml@refs/heads/main"
	registryMatrixName              = "provider-lifecycle-matrix.json"
	registryReadbackName            = "provider-readback.json"
	registryBundleName              = "provider-readback.sigstore.json"
	registryProofType               = "registry-readback"
	currentProviderSubject          = "provider:registry.terraform.io/tako0614/takoform"
	currentProviderRepository       = "https://github.com/tako0614/terraform-provider-takoform.git"
	currentProviderRegistryAddress  = "registry.terraform.io/tako0614/takoform"
	currentSignedCandidateFileCount = 6
)

// CurrentBuildOptions selects independently signed current-generation
// evidence. The builder is deterministic and non-publishing.
type CurrentBuildOptions struct {
	BuildOptions
	HostRequestID              string
	HostWorkflowRunAttempt     string
	HostHeadSHA                string
	ProviderRequestID          string
	ProviderWorkflowRunAttempt string
	ProviderHeadSHA            string
	RegistryReadback           string
	RegistryRequestID          string
	RegistryWorkflowRunID      string
	RegistryWorkflowRunAttempt string
	RegistryHeadSHA            string
}

type registryProviderIdentity struct {
	Address string `json:"address"`
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
}

type registryArtifactRef struct {
	Path       string `json:"path"`
	Digest     string `json:"digest"`
	BundlePath string `json:"bundlePath,omitempty"`
}

type registryManifest struct {
	Format     string                   `json:"format"`
	Status     string                   `json:"status"`
	ProofType  string                   `json:"proofType"`
	Generation string                   `json:"generation"`
	Source     sourceRef                `json:"source"`
	Provider   registryProviderIdentity `json:"provider"`
	Matrix     registryArtifactRef      `json:"matrix"`
	Readback   registryArtifactRef      `json:"readback"`
}

type signedRegistryReadbackRef struct {
	Path         string `json:"path"`
	Digest       string `json:"digest"`
	BundlePath   string `json:"bundlePath"`
	BundleDigest string `json:"bundleDigest"`
}

type signedRegistryManifest struct {
	Format              string                    `json:"format"`
	Status              string                    `json:"status"`
	ProofType           string                    `json:"proofType"`
	Generation          string                    `json:"generation"`
	CertificateIdentity string                    `json:"certificateIdentity"`
	Workflow            string                    `json:"workflow"`
	RequestID           string                    `json:"requestId"`
	WorkflowRunID       string                    `json:"workflowRunId"`
	WorkflowRunAttempt  int                       `json:"workflowRunAttempt"`
	Source              sourceRef                 `json:"source"`
	Manifest            retainedRef               `json:"manifest"`
	Readback            signedRegistryReadbackRef `json:"readback"`
}

type registryArtifactSet struct {
	matrix   []byte
	readback []byte
	bundle   []byte
	parsed   admissionrelease.ProviderRegistryReadback
}

type currentArtifactConfig struct {
	repositoryRoot     string
	hostID             string
	root               string
	role               string
	closure            admissionrelease.CandidateSet
	selected           admissionrelease.CandidateSet
	takoformCommit     string
	sourceCommit       string
	requestID          string
	workflowRunID      string
	workflowRunAttempt string
	headSHA            string
	expectedSubject    string
	providerVersion    string
	retainedPolicyRoot string
}

// BuildCurrent builds generation-aware v4 material. Provider reports are
// verified over all 34 portable-v1 Forms before the exact ga-core-v2 ten are
// selected; host reports must close over those ten directly.
func BuildCurrent(options CurrentBuildOptions) error {
	if err := validateCurrentBuildOptions(options); err != nil {
		return err
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return err
	}
	descriptor, _, err := admissioncheckpoint.LoadCurrent(root)
	if err != nil {
		return fmt.Errorf("current admission checkpoint descriptor: %w", err)
	}
	if options.AdmissionVersion != descriptor.Version {
		return fmt.Errorf("admission version %q does not equal the source descriptor version %q", options.AdmissionVersion, descriptor.Version)
	}
	output, err := prepareOutputPath(root, options.OutputDir)
	if err != nil {
		return err
	}
	selected, err := standardforms.CurrentAdmissionCandidateSet(root)
	if err != nil {
		return err
	}
	if selected.Generation != currentAdmissionGeneration || len(selected.Entries) != 10 {
		return fmt.Errorf("current admission requires exact ga-core-v2 ten-Form candidates")
	}
	portable, err := standardforms.CurrentPortableCandidateSet(root)
	if err != nil {
		return err
	}
	if portable.Generation != currentProviderGeneration || len(portable.Entries) != 34 {
		return fmt.Errorf("current provider reports require exact portable-v1 34-Form closure")
	}
	publishedSet, err := standardforms.CurrentPublishedPackageSet(root)
	if err != nil {
		return fmt.Errorf("current published package closure: %w", err)
	}
	if err := admissionrelease.VerifyOfflineTrust(filepath.Join(root, currentAdmissionRoot)); err != nil {
		return fmt.Errorf("current offline admission trust: %w", err)
	}
	published, err := projectCurrentPublishedSet(publishedSet, selected)
	if err != nil {
		return fmt.Errorf("current published package projection: %w", err)
	}
	providerVersion, err := loadProviderVersion(root)
	if err != nil {
		return err
	}
	hosts, err := loadCurrentArtifactSet(currentArtifactConfig{
		repositoryRoot: root, hostID: options.HostID, root: options.HostReports,
		role: "host-report", closure: selected, selected: selected,
		takoformCommit: options.HostTakoformSourceCommit, sourceCommit: options.HostSourceCommit,
		requestID: options.HostRequestID, workflowRunID: options.HostWorkflowRunID,
		workflowRunAttempt: options.HostWorkflowRunAttempt, headSHA: options.HostHeadSHA,
		providerVersion:    providerVersion,
		retainedPolicyRoot: currentAdmissionRoot,
	})
	if err != nil {
		return fmt.Errorf("current host report candidate: %w", err)
	}
	providers, err := loadCurrentArtifactSet(currentArtifactConfig{
		repositoryRoot: root, root: options.ProviderReports,
		role: "provider-report", closure: portable, selected: selected,
		takoformCommit: options.ProviderSourceCommit, sourceCommit: options.ProviderSourceCommit,
		requestID: options.ProviderRequestID, workflowRunID: options.ProviderWorkflowRunID,
		workflowRunAttempt: options.ProviderWorkflowRunAttempt, headSHA: options.ProviderHeadSHA,
		expectedSubject: currentProviderSubject,
		providerVersion: providerVersion, retainedPolicyRoot: currentAdmissionRoot,
	})
	if err != nil {
		return fmt.Errorf("current provider report candidate: %w", err)
	}
	registry, err := loadCurrentRegistryArtifact(
		root,
		options.RegistryReadback,
		options.ProviderSourceCommit,
		options.RegistryRequestID,
		options.RegistryWorkflowRunID,
		options.RegistryWorkflowRunAttempt,
		options.RegistryHeadSHA,
	)
	if err != nil {
		return fmt.Errorf("current provider Registry candidate: %w", err)
	}
	if err := validateCurrentProviderRegistryIdentity(providers, registry); err != nil {
		return fmt.Errorf("current provider report/Registry identity: %w", err)
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

	entries := make([]admissionrelease.SetEntry, 0, len(selected.Entries))
	for _, candidate := range selected.Entries {
		host := hosts.byKind[candidate.Kind]
		provider := providers.byKind[candidate.Kind]
		packageRoot := filepath.Join(root, filepath.FromSlash(candidate.PackagePath))
		_, canonical, err := buildEvidence(packageRoot, candidate, host, provider)
		if err != nil {
			return fmt.Errorf("%s current evidence: %w", candidate.Kind, err)
		}
		directory := path.Join("packages", candidate.Slug)
		for _, file := range []struct {
			name string
			raw  []byte
		}{
			{path.Join(directory, "host-report.json"), host.raw},
			{path.Join(directory, "host-report.sigstore.json"), host.bundle},
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
			ProviderReportPath: path.Join("provider-closure", provider.entry.Path), ProviderReportDigest: formpackage.DigestBytes(provider.raw), ProviderReportSigstoreBundle: path.Join("provider-closure", provider.entry.BundlePath),
			AdmissionStatus: "portable-standard",
		})
	}
	providerClosure := admissionrelease.ProviderReportClosure{
		Generation:           currentProviderGeneration,
		ManifestPath:         path.Join("provider-closure", providerManifestName),
		ManifestDigest:       formpackage.DigestBytes(providers.manifestRaw),
		SignedManifestPath:   path.Join("provider-closure", providerSignedName),
		SignedManifestDigest: formpackage.DigestBytes(providers.signedRaw),
		ChecksumsPath:        path.Join("provider-closure", checksumsName),
		ChecksumsDigest:      formpackage.DigestBytes(providers.checksumsRaw),
		Reports:              make([]admissionrelease.ProviderReportClosureEntry, 0, len(portable.Entries)),
	}
	for _, candidate := range portable.Entries {
		provider := providers.allByKind[candidate.Kind]
		for _, file := range []struct {
			name string
			raw  []byte
		}{
			{path.Join("provider-closure", provider.entry.Path), provider.raw},
			{path.Join("provider-closure", provider.entry.BundlePath), provider.bundle},
		} {
			if err := writeCreateOnly(output, file.name, file.raw); err != nil {
				return err
			}
		}
		providerClosure.Reports = append(providerClosure.Reports, admissionrelease.ProviderReportClosureEntry{
			Kind: candidate.Kind, Slug: candidate.Slug,
			Identity:       standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest},
			ReportPath:     path.Join("provider-closure", provider.entry.Path),
			ReportDigest:   formpackage.DigestBytes(provider.raw),
			SigstoreBundle: path.Join("provider-closure", provider.entry.BundlePath),
		})
	}
	for _, file := range []struct {
		name string
		raw  []byte
	}{
		{providerClosure.ManifestPath, providers.manifestRaw},
		{providerClosure.SignedManifestPath, providers.signedRaw},
		{providerClosure.ChecksumsPath, providers.checksumsRaw},
	} {
		if err := writeCreateOnly(output, file.name, file.raw); err != nil {
			return err
		}
	}
	for _, file := range []struct {
		name string
		raw  []byte
	}{
		{path.Join("registry", registryMatrixName), registry.matrix},
		{path.Join("registry", registryReadbackName), registry.readback},
		{path.Join("registry", registryBundleName), registry.bundle},
	} {
		if err := writeCreateOnly(output, file.name, file.raw); err != nil {
			return err
		}
	}
	_, setRaw, err := admissionrelease.BuildCanonicalSet(selected, descriptor.Tag, admissionrelease.RegistryReadbackRef{
		Path: path.Join("registry", registryReadbackName), Digest: formpackage.DigestBytes(registry.readback),
		SigstoreBundle: path.Join("registry", registryBundleName),
	}, entries, providerClosure)
	if err != nil {
		return err
	}
	if err := writeCreateOnly(output, "standard-admission-set.json", setRaw); err != nil {
		return err
	}
	complete = true
	return nil
}

func validateCurrentBuildOptions(options CurrentBuildOptions) error {
	for label, value := range map[string]string{
		"source commit":               options.SourceCommit,
		"host source commit":          options.HostSourceCommit,
		"host Takoform source commit": options.HostTakoformSourceCommit,
		"provider source commit":      options.ProviderSourceCommit,
		"host head SHA":               options.HostHeadSHA,
		"provider head SHA":           options.ProviderHeadSHA,
		"registry head SHA":           options.RegistryHeadSHA,
	} {
		if !commitPattern.MatchString(value) {
			return fmt.Errorf("%s must be lowercase 40-hex", label)
		}
	}
	for label, value := range map[string]string{
		"host workflow run id":          options.HostWorkflowRunID,
		"host workflow run attempt":     options.HostWorkflowRunAttempt,
		"provider workflow run id":      options.ProviderWorkflowRunID,
		"provider workflow run attempt": options.ProviderWorkflowRunAttempt,
		"registry workflow run id":      options.RegistryWorkflowRunID,
		"registry workflow run attempt": options.RegistryWorkflowRunAttempt,
	} {
		if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(value) {
			return fmt.Errorf("%s must be a positive decimal integer", label)
		}
	}
	if !versionPattern.MatchString(options.AdmissionVersion) {
		return fmt.Errorf("admission version is not a canonical version token")
	}
	for label, value := range map[string]string{
		"host request id":     options.HostRequestID,
		"provider request id": options.ProviderRequestID,
		"registry request id": options.RegistryRequestID,
	} {
		if !canonicalUUIDv4Pattern.MatchString(value) {
			return fmt.Errorf("%s must be a canonical lowercase UUIDv4", label)
		}
	}
	if options.HostHeadSHA != options.HostSourceCommit {
		return fmt.Errorf("host head SHA must equal the host source commit")
	}
	if options.ProviderHeadSHA != options.ProviderSourceCommit {
		return fmt.Errorf("provider head SHA must equal the provider source commit")
	}
	if options.RegistryHeadSHA != options.ProviderSourceCommit {
		return fmt.Errorf("registry head SHA must equal the provider source commit")
	}
	if options.HostReports == "" || options.ProviderReports == "" || options.RegistryReadback == "" ||
		options.OutputDir == "" || options.HostID == "" {
		return fmt.Errorf("current host, provider, Registry, output, and host-id inputs are required")
	}
	return nil
}

func loadCurrentArtifactSet(config currentArtifactConfig) (artifactSet, error) {
	absolute, err := filepath.Abs(config.root)
	if err != nil {
		return artifactSet{}, err
	}
	manifestName, signedName := hostManifestName, hostSignedName
	manifestFormat, signedFormat := "", ""
	workflow, identity := "", ""
	var acceptedHost hostpolicy.Host
	if config.role == "provider-report" {
		manifestName, signedName = providerManifestName, providerSignedName
		manifestFormat, signedFormat = providerManifestFormat, providerSignedFormat
		workflow, identity = providerWorkflow, providerIdentity
	} else {
		policy, err := hostpolicy.LoadAt(config.repositoryRoot, config.retainedPolicyRoot)
		if err != nil {
			return artifactSet{}, err
		}
		acceptedHost, err = policy.ByHostID(config.hostID)
		if err != nil {
			return artifactSet{}, err
		}
		manifestFormat, signedFormat = acceptedHost.ManifestFormat, acceptedHost.SignedFormat
		workflow, identity = acceptedHost.Workflow, acceptedHost.CertificateIdentity
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
	checksumsRaw, err := readRegular(absolute, checksumsName, maximumMaterialBytes)
	if err != nil {
		return artifactSet{}, err
	}
	if manifest.Format != manifestFormat || signed.Format != signedFormat ||
		manifest.Status != "candidate-only" || signed.Status != "candidate-only" ||
		manifest.Generation != config.closure.Generation || signed.Generation != config.closure.Generation ||
		manifest.DefinitionVersion != "" || manifest.PackageVersion != "" ||
		manifest.Subject == "" || manifest.RunnerVersion == "" ||
		signed.Subject != manifest.Subject || signed.ProofType != manifest.ProofType ||
		signed.CertificateIdentity != identity || signed.Workflow != workflow ||
		signed.RequestID != config.requestID ||
		!canonicalUUIDv4Pattern.MatchString(signed.RequestID) ||
		signed.WorkflowRunID != config.workflowRunID ||
		fmt.Sprint(signed.WorkflowRunAttempt) != config.workflowRunAttempt ||
		signed.Source != manifest.Source ||
		signed.Source.Commit != config.headSHA {
		return artifactSet{}, fmt.Errorf("current candidate manifest identity or workflow closure is invalid")
	}
	expectedFiles := map[string]struct{}{manifestName: {}, signedName: {}, checksumsName: {}}
	if config.role == "host-report" {
		contract, err := portableconformance.Verify(
			filepath.Join(config.repositoryRoot, "conformance", "portable-host-v1"),
		)
		if err != nil {
			return artifactSet{}, fmt.Errorf("current portable host runner contract: %w", err)
		}
		if manifest.Workflow != workflow ||
			!canonicalUUIDv4Pattern.MatchString(manifest.RequestID) ||
			signed.RequestID != manifest.RequestID ||
			signed.WorkflowRunAttempt != 1 ||
			manifest.PortableRunner == nil ||
			signed.PortableRunner == nil ||
			signed.Closure == nil {
			return artifactSet{}, fmt.Errorf("current host candidate request/runner closure is invalid")
		}
		portable := manifest.PortableRunner
		signedPortable := signed.PortableRunner
		signedClosure := signed.Closure
		if portable.Path != "portable-host-runner-report.json" ||
			portable.BundlePath != "portable-host-runner-report.sigstore.json" ||
			portable.Format != "takoform.portable-host-runner-report@v1" ||
			portable.RunnerSubject != contract.RunnerEvidence.Subject ||
			portable.RunnerInputDigest != contract.RunnerEvidence.SHA256 ||
			!formpackage.ValidDigest(portable.Digest) ||
			!formpackage.ValidDigest(portable.RunnerInputDigest) ||
			signedPortable.ReportPath != portable.Path ||
			signedPortable.ReportDigest != portable.Digest ||
			signedPortable.BundlePath != portable.BundlePath ||
			signedPortable.RunnerSubject != portable.RunnerSubject ||
			signedPortable.RunnerInputDigest != portable.RunnerInputDigest ||
			signedClosure.ChecksumsPath != checksumsName ||
			signedClosure.BundlePath != checksumsBundleName ||
			signedClosure.CertificateIdentity != identity {
			return artifactSet{}, fmt.Errorf("current host portable runner identity is invalid")
		}
		portableRaw, err := readRegular(absolute, portable.Path, maximumMaterialBytes)
		if err != nil {
			return artifactSet{}, err
		}
		if err := requireCanonical(portableRaw, portable.Path); err != nil {
			return artifactSet{}, err
		}
		portableBundle, err := readRegular(absolute, portable.BundlePath, maximumMaterialBytes)
		if err != nil {
			return artifactSet{}, err
		}
		if portable.Digest != formpackage.DigestBytes(portableRaw) ||
			signedPortable.BundleDigest != formpackage.DigestBytes(portableBundle) {
			return artifactSet{}, fmt.Errorf("current host portable runner digest closure is invalid")
		}
		var portableReport portableconformance.HostRunnerReport
		if err := decodeStrict(portableRaw, &portableReport); err != nil {
			return artifactSet{}, fmt.Errorf("current host portable runner report: %w", err)
		}
		if portableReport.Format != portable.Format ||
			portableReport.RunnerSubject != portable.RunnerSubject ||
			portableReport.RunnerInputDigest != portable.RunnerInputDigest {
			return artifactSet{}, fmt.Errorf("current host portable runner report does not match its manifest reference")
		}
		if err := portableconformance.ValidateHostRunnerReport(contract, portableReport); err != nil {
			return artifactSet{}, fmt.Errorf("current host portable runner semantics: %w", err)
		}
		if err := validateBundle(portableBundle); err != nil {
			return artifactSet{}, err
		}
		checksumsBundle, err := readRegular(absolute, signedClosure.BundlePath, maximumMaterialBytes)
		if err != nil {
			return artifactSet{}, err
		}
		if err := validateBundle(checksumsBundle); err != nil {
			return artifactSet{}, fmt.Errorf("current host checksum signature bundle: %w", err)
		}
		expectedFiles[portable.Path] = struct{}{}
		expectedFiles[portable.BundlePath] = struct{}{}
		expectedFiles[signedClosure.BundlePath] = struct{}{}
	} else if manifest.PortableRunner != nil || signed.PortableRunner != nil || signed.Closure != nil {
		return artifactSet{}, fmt.Errorf("current provider candidate must not claim host runner or checksum-signature fields")
	}
	if config.expectedSubject != "" && manifest.Subject != config.expectedSubject {
		return artifactSet{}, fmt.Errorf("subject is %q, want %q", manifest.Subject, config.expectedSubject)
	}
	if config.role == "provider-report" {
		if manifest.ProofType != "provider" || manifest.RunnerVersion != config.providerVersion ||
			manifest.Source.Repository != currentProviderRepository || manifest.Source.Commit != config.sourceCommit ||
			signed.HeadSHA != config.headSHA ||
			manifest.TakoformSource != nil || signed.TakoformSource != nil {
			return artifactSet{}, fmt.Errorf("current provider candidate source binding is invalid")
		}
	} else if manifest.ProofType != acceptedHost.ProofType ||
		manifest.Subject != acceptedHost.Subject ||
		manifest.Source.Repository != acceptedHost.SourceRepository ||
		manifest.Source.Commit != config.sourceCommit ||
		manifest.RunnerVersion != acceptedHost.RunnerVersionPrefix+manifest.Source.Commit ||
		manifest.TakoformSource == nil ||
		manifest.TakoformSource.Repository != currentProviderRepository ||
		manifest.TakoformSource.Commit != config.takoformCommit ||
		signed.TakoformSource == nil ||
		*signed.TakoformSource != *manifest.TakoformSource {
		return artifactSet{}, fmt.Errorf("current host candidate source binding is invalid")
	}
	if signed.Source != manifest.Source || signed.Manifest.Path != manifestName ||
		signed.Manifest.Digest != formpackage.DigestBytes(manifestRaw) {
		return artifactSet{}, fmt.Errorf("current signed candidate does not bind its manifest and source")
	}
	if len(manifest.Reports) != len(config.closure.Entries) ||
		len(signed.Entries) != len(config.closure.Entries) {
		return artifactSet{}, fmt.Errorf("current candidate has %d/%d reports, want exact %d closure", len(manifest.Reports), len(signed.Entries), len(config.closure.Entries))
	}
	bySigned := make(map[string]signedEntry, len(signed.Entries))
	for _, entry := range signed.Entries {
		if _, duplicate := bySigned[entry.Kind]; duplicate {
			return artifactSet{}, fmt.Errorf("current signed candidate duplicates %s", entry.Kind)
		}
		bySigned[entry.Kind] = entry
	}
	allArtifacts := make(map[string]reportArtifact, len(config.closure.Entries))
	providerBinarySHA256 := ""
	for _, candidate := range config.closure.Entries {
		var entry *reportManifestEntry
		for index := range manifest.Reports {
			if manifest.Reports[index].Kind == candidate.Kind {
				entry = &manifest.Reports[index]
				break
			}
		}
		identity := standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest}
		if entry == nil || entry.Slug != candidate.Slug || !reflect.DeepEqual(entry.Identity, identity) {
			return artifactSet{}, fmt.Errorf("current candidate omits exact %s identity", candidate.Kind)
		}
		wantReport := path.Join("packages", candidate.Slug, config.role+".json")
		wantBundle := path.Join("packages", candidate.Slug, config.role+".sigstore.json")
		if entry.Path != wantReport || entry.BundlePath != wantBundle || !formpackage.ValidDigest(entry.Digest) {
			return artifactSet{}, fmt.Errorf("%s current report paths or digest are not canonical", candidate.Kind)
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
			return artifactSet{}, fmt.Errorf("%s current report digest mismatch", candidate.Kind)
		}
		if err := validateBundle(bundleRaw); err != nil {
			return artifactSet{}, fmt.Errorf("%s current bundle: %w", candidate.Kind, err)
		}
		fixtures, err := loadFixtureClosure(filepath.Join(config.repositoryRoot, filepath.FromSlash(candidate.PackagePath)))
		if err != nil {
			return artifactSet{}, err
		}
		var parsed admissionrelease.RunnerReport
		if config.role == "provider-report" {
			positive := make([]string, 0, len(fixtures.positive))
			negative := make([]admissionrelease.NegativeFixtureExpectation, 0, len(fixtures.negative))
			for _, fixture := range fixtures.positive {
				positive = append(positive, fixture.Name)
			}
			for _, fixture := range fixtures.negative {
				negative = append(negative, admissionrelease.NegativeFixtureExpectation{
					Name: fixture.Name, Stage: fixture.Stage,
				})
			}
			parsed, err = admissionrelease.ValidateCanonicalProviderRunnerReportWithStages(reportRaw, identity, positive, negative)
		} else {
			parsed, err = admissionrelease.ValidateCanonicalHostRunnerReport(reportRaw, identity, fixtures.positiveBindings, fixtures.negativeBindings)
		}
		if err != nil || parsed.Role != config.role || parsed.Subject != manifest.Subject ||
			parsed.RunnerVersion != manifest.RunnerVersion || !reflect.DeepEqual(parsed.Identity, identity) {
			return artifactSet{}, fmt.Errorf("%s current report identity or fixture closure is invalid: %w", candidate.Kind, err)
		}
		if config.role == "provider-report" {
			if !formpackage.ValidDigest(parsed.ProviderBinarySHA256) {
				return artifactSet{}, fmt.Errorf("%s current provider report does not bind an exact provider binary", candidate.Kind)
			}
			if providerBinarySHA256 == "" {
				providerBinarySHA256 = parsed.ProviderBinarySHA256
			} else if parsed.ProviderBinarySHA256 != providerBinarySHA256 {
				return artifactSet{}, fmt.Errorf("%s current provider report used a different provider binary", candidate.Kind)
			}
		}
		signedEntry, ok := bySigned[candidate.Kind]
		if !ok || signedEntry.Slug != candidate.Slug || signedEntry.ReportPath != entry.Path ||
			signedEntry.ReportDigest != entry.Digest || signedEntry.BundlePath != entry.BundlePath ||
			signedEntry.BundleDigest != formpackage.DigestBytes(bundleRaw) {
			return artifactSet{}, fmt.Errorf("current signed candidate does not close over %s", candidate.Kind)
		}
		expectedFiles[entry.Path], expectedFiles[entry.BundlePath] = struct{}{}, struct{}{}
		allArtifacts[candidate.Kind] = reportArtifact{entry: *entry, report: parsed, raw: reportRaw, bundle: bundleRaw}
	}
	files, err := listRegularFiles(absolute)
	if err != nil {
		return artifactSet{}, err
	}
	if !sameFileSet(files, expectedFiles) {
		return artifactSet{}, fmt.Errorf("current candidate file inventory is not the exact %d-file closure", len(expectedFiles))
	}
	if config.role == "host-report" {
		if err := verifyChecksumsExcluding(absolute, expectedFiles, map[string]struct{}{
			checksumsBundleName: {},
		}); err != nil {
			return artifactSet{}, err
		}
	} else if err := verifyChecksums(absolute, expectedFiles); err != nil {
		return artifactSet{}, err
	}
	selected := make(map[string]reportArtifact, len(config.selected.Entries))
	for _, candidate := range config.selected.Entries {
		artifact, ok := allArtifacts[candidate.Kind]
		if !ok || artifact.entry.Identity != (standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest}) {
			return artifactSet{}, fmt.Errorf("current full closure cannot select exact %s identity", candidate.Kind)
		}
		selected[candidate.Kind] = artifact
	}
	return artifactSet{
		manifest: manifest, manifestRaw: manifestRaw, signedRaw: signedRaw, checksumsRaw: checksumsRaw,
		byKind: selected, allByKind: allArtifacts, providerBinarySHA256: providerBinarySHA256,
	}, nil
}

func validateCurrentProviderRegistryIdentity(providers artifactSet, registry registryArtifactSet) error {
	if !formpackage.ValidDigest(providers.providerBinarySHA256) ||
		providers.manifest.RunnerVersion != "" && providers.manifest.RunnerVersion != registry.parsed.ProviderVersion ||
		len(providers.allByKind) == 0 || len(registry.parsed.Installs) == 0 {
		return fmt.Errorf("provider reports or Registry readback lack a complete binary identity")
	}
	for kind, provider := range providers.allByKind {
		if provider.report.RunnerVersion != registry.parsed.ProviderVersion ||
			provider.report.ProviderBinarySHA256 != providers.providerBinarySHA256 {
			return fmt.Errorf("%s provider report binary identity differs from the provider closure", kind)
		}
	}
	for _, install := range registry.parsed.Installs {
		if install.ProviderVersion != registry.parsed.ProviderVersion ||
			install.ProviderBinarySHA256 != providers.providerBinarySHA256 {
			return fmt.Errorf("%s Registry binary identity differs from provider reports", install.Product)
		}
	}
	return nil
}

func loadCurrentRegistryArtifact(
	repositoryRoot,
	artifactRoot,
	sourceCommit,
	requestID,
	workflowRunID,
	workflowRunAttempt,
	headSHA string,
) (registryArtifactSet, error) {
	absolute, err := filepath.Abs(artifactRoot)
	if err != nil {
		return registryArtifactSet{}, err
	}
	manifestRaw, err := readRegular(absolute, registryManifestName, maximumMaterialBytes)
	if err != nil {
		return registryArtifactSet{}, err
	}
	if err := requireCanonical(manifestRaw, registryManifestName); err != nil {
		return registryArtifactSet{}, err
	}
	var manifest registryManifest
	if err := decodeStrict(manifestRaw, &manifest); err != nil {
		return registryArtifactSet{}, err
	}
	signedRaw, err := readRegular(absolute, registrySignedName, maximumMaterialBytes)
	if err != nil {
		return registryArtifactSet{}, err
	}
	if err := requireCanonical(signedRaw, registrySignedName); err != nil {
		return registryArtifactSet{}, err
	}
	var signed signedRegistryManifest
	if err := decodeStrict(signedRaw, &signed); err != nil {
		return registryArtifactSet{}, err
	}
	if manifest.Format != registryManifestFormat || signed.Format != registrySignedFormat ||
		manifest.Status != "candidate-only" || signed.Status != "candidate-only" ||
		manifest.ProofType != registryProofType || signed.ProofType != registryProofType ||
		manifest.Generation != currentProviderGeneration || signed.Generation != currentProviderGeneration ||
		manifest.Source != (sourceRef{Repository: currentProviderRepository, Commit: sourceCommit}) ||
		signed.Source != manifest.Source || signed.CertificateIdentity != registryIdentity ||
		signed.Workflow != registryWorkflow ||
		signed.RequestID != requestID ||
		!canonicalUUIDv4Pattern.MatchString(signed.RequestID) ||
		signed.WorkflowRunID != workflowRunID ||
		fmt.Sprint(signed.WorkflowRunAttempt) != workflowRunAttempt ||
		signed.Source.Commit != headSHA ||
		signed.Manifest != (retainedRef{Path: registryManifestName, Digest: formpackage.DigestBytes(manifestRaw)}) {
		return registryArtifactSet{}, fmt.Errorf("Registry candidate identity or workflow closure is invalid")
	}
	if manifest.Provider.Address != currentProviderRegistryAddress ||
		manifest.Provider.Tag != "v"+manifest.Provider.Version ||
		!versionPattern.MatchString(manifest.Provider.Version) ||
		!commitPattern.MatchString(manifest.Provider.Commit) ||
		manifest.Matrix.Path != registryMatrixName ||
		manifest.Readback.Path != registryReadbackName ||
		manifest.Readback.BundlePath != registryBundleName {
		return registryArtifactSet{}, fmt.Errorf("Registry candidate provider or path binding is invalid")
	}
	matrix, err := readRegular(absolute, registryMatrixName, maximumMaterialBytes)
	if err != nil {
		return registryArtifactSet{}, err
	}
	readback, err := readRegular(absolute, registryReadbackName, maximumMaterialBytes)
	if err != nil {
		return registryArtifactSet{}, err
	}
	bundle, err := readRegular(absolute, registryBundleName, maximumMaterialBytes)
	if err != nil {
		return registryArtifactSet{}, err
	}
	if manifest.Matrix.Digest != formpackage.DigestBytes(matrix) ||
		manifest.Readback.Digest != formpackage.DigestBytes(readback) ||
		signed.Readback != (signedRegistryReadbackRef{
			Path: registryReadbackName, Digest: manifest.Readback.Digest,
			BundlePath: registryBundleName, BundleDigest: formpackage.DigestBytes(bundle),
		}) {
		return registryArtifactSet{}, fmt.Errorf("Registry candidate digest closure is invalid")
	}
	if err := validateBundle(bundle); err != nil {
		return registryArtifactSet{}, err
	}
	parsed, canonical, err := admissionrelease.BuildRegistryReadback(repositoryRoot, filepath.Join(absolute, registryMatrixName), manifest.Provider.Commit)
	if err != nil {
		return registryArtifactSet{}, err
	}
	if !bytes.Equal(readback, canonical) || parsed.ProviderVersion != manifest.Provider.Version ||
		parsed.ProviderReleaseTag != manifest.Provider.Tag ||
		parsed.ProviderReleaseCommit != manifest.Provider.Commit {
		return registryArtifactSet{}, fmt.Errorf("Registry candidate readback differs from the rederived canonical subject")
	}
	expectedFiles := map[string]struct{}{
		registryManifestName: {}, registrySignedName: {}, checksumsName: {},
		registryMatrixName: {}, registryReadbackName: {}, registryBundleName: {},
	}
	files, err := listRegularFiles(absolute)
	if err != nil {
		return registryArtifactSet{}, err
	}
	if len(files) != currentSignedCandidateFileCount || !sameFileSet(files, expectedFiles) {
		return registryArtifactSet{}, fmt.Errorf("Registry candidate is not the exact six-file closure")
	}
	if err := verifyChecksums(absolute, expectedFiles); err != nil {
		return registryArtifactSet{}, err
	}
	return registryArtifactSet{matrix: matrix, readback: readback, bundle: bundle, parsed: parsed}, nil
}

func projectCurrentPublishedSet(
	set formpublication.Set,
	selected admissionrelease.CandidateSet,
) (map[string]admissionrelease.PublishedPackageEntry, error) {
	if selected.Generation != currentAdmissionGeneration || len(selected.Entries) != 10 {
		return nil, fmt.Errorf("current published package projection requires exact ga-core-v2 ten-Form selection")
	}
	projected, err := formpublication.ProjectEntries(set, selected)
	if err != nil {
		return nil, err
	}
	if len(projected) != 10 {
		return nil, fmt.Errorf("current published package projection returned %d entries, want 10", len(projected))
	}
	return projected, nil
}
