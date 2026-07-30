package admissionrelease

import (
	"bytes"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissioncheckpoint"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	admissionRootPath = "admission/v1"
	setManifestName   = "standard-admission-set.json"
	setManifestPath   = admissionRootPath + "/" + setManifestName
	maxSetBytes       = 1 << 20
	maxEvidenceBytes  = 16 << 20
)

var (
	admissionReleaseTagPattern = regexp.MustCompile(`^forms/admissions/v[0-9][0-9A-Za-z.+-]*$`)
	packageReleaseTagPattern   = regexp.MustCompile(`^forms/k-[a-z2-7]{2,103}/v[0-9][0-9A-Za-z.+-]*$`)
	releaseCommitPattern       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	slugPattern                = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
)

// VerifyAdmissionSet is the fail-closed admission-closure entry point. It
// performs strict retained-set, exact-candidate, canonical-digest, package,
// and admission-structure checks. Every retained report, package index, and
// Registry readback is then authenticated offline before Form admission opens.
func VerifyAdmissionSet(root string, candidates CandidateSet) error {
	return VerifyAdmissionSetAt(root, admissionRootPath, candidates)
}

// VerifyAdmissionSetAt verifies one explicitly selected immutable admission
// generation. It lets the retired v2 set remain pinned under admission/v1
// while current mixed-version sets advance under a different retained root.
func VerifyAdmissionSetAt(root, retainedRoot string, candidates CandidateSet) error {
	return verifyAdmissionSetAt(root, retainedRoot, candidates, nil, gitReleaseRefVerifier{})
}

// VerifyAdmissionMaterialAt authenticates one complete source-retained
// admission closure before its checkpoint tag exists. It verifies every
// retained subject plus provider/package release ref, but deliberately omits
// only the not-yet-minted admission tag.
func VerifyAdmissionMaterialAt(root, retainedRoot string, candidates CandidateSet) error {
	return verifyAdmissionSetAt(root, retainedRoot, candidates, nil, gitMaterialRefVerifier{})
}

func verifyAdmissionSet(root string, candidates CandidateSet, verifier RetainedSubjectVerifier, refVerifier ReleaseRefVerifier) error {
	return verifyAdmissionSetAt(root, admissionRootPath, candidates, verifier, refVerifier)
}

func verifyAdmissionSetAt(root, retainedRoot string, candidates CandidateSet, verifier RetainedSubjectVerifier, refVerifier ReleaseRefVerifier) error {
	if err := validateCandidateSet(candidates); err != nil {
		return fmt.Errorf("standard-admission candidate set: %w", err)
	}
	if err := validateRelativePath(retainedRoot); err != nil {
		return fmt.Errorf("standard-admission retained root: %w", err)
	}

	admissionRoot := filepath.Join(root, filepath.FromSlash(retainedRoot))
	manifestPath := path.Join(retainedRoot, setManifestName)
	raw, err := readRetainedRelativeFile(root, manifestPath, maxSetBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Form admission activation is blocked: missing %s", manifestPath)
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	var set Set
	if err := decodeStrictJSON(raw, &set); err != nil {
		return fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	ordered, err := validateSet(set, candidates)
	if err != nil {
		return fmt.Errorf("verify %s: %w", manifestPath, err)
	}
	if candidates.Generation == "ga-core-v2" {
		descriptor, _, err := admissioncheckpoint.LoadCurrent(root)
		if err != nil {
			return fmt.Errorf("verify current admission checkpoint descriptor: %w", err)
		}
		if retainedRoot != descriptor.RetainedRoot {
			return fmt.Errorf("current admission retained root %q does not match descriptor %q", retainedRoot, descriptor.RetainedRoot)
		}
		if err := descriptor.ValidateSetProjection(set.Generation, set.AdmissionReleaseTag); err != nil {
			return fmt.Errorf("verify current admission checkpoint projection: %w", err)
		}
	}

	subjects := make([]RetainedSubject, 0, expectedRetainedSubjectCount(set))
	selectedKinds := make(map[string]struct{}, len(ordered))
	for _, pair := range ordered {
		selectedKinds[pair.entry.Kind] = struct{}{}
		evidenceFile := filepath.Join(admissionRoot, filepath.FromSlash(pair.entry.EvidencePath))
		evidenceRaw, err := readRetainedRelativeFile(root, path.Join(retainedRoot, pair.entry.EvidencePath), maxEvidenceBytes)
		if err != nil {
			return fmt.Errorf("read %s evidence %q: %w", pair.entry.Kind, pair.entry.EvidencePath, err)
		}
		canonical, err := formpackage.Canonicalize(evidenceRaw)
		if err != nil {
			return fmt.Errorf("%s evidence is not RFC 8785 I-JSON: %w", pair.entry.Kind, err)
		}
		if !bytes.Equal(evidenceRaw, canonical) {
			return fmt.Errorf("%s evidence bytes are not the retained RFC 8785 canonical bytes", pair.entry.Kind)
		}
		if digest := formpackage.DigestBytes(canonical); digest != pair.entry.EvidenceDigest {
			return fmt.Errorf("%s retained evidence digest mismatch: manifest=%s actual=%s", pair.entry.Kind, pair.entry.EvidenceDigest, digest)
		}

		packageRoot := filepath.Join(root, filepath.FromSlash(pair.candidate.PackagePath))
		report, err := formpackage.VerifyDirectory(packageRoot)
		if err != nil {
			return fmt.Errorf("%s candidate package: %w", pair.entry.Kind, err)
		}
		if report.FormRef != pair.candidate.FormRef || report.PackageDigest != pair.candidate.PackageDigest {
			return fmt.Errorf("%s verified candidate package identity drift", pair.entry.Kind)
		}
		definition, err := readDefinition(packageRoot)
		if err != nil {
			return fmt.Errorf("%s candidate definition: %w", pair.entry.Kind, err)
		}
		evidence, err := standardform.ValidateEvidenceStructure(evidenceFile, report, definition)
		if err != nil {
			return fmt.Errorf("%s retained admission structure: %w", pair.entry.Kind, err)
		}
		positiveBindings, negativeBindings, err := bindExactPackageFixtures(packageRoot, definition, evidence)
		if err != nil {
			return fmt.Errorf("%s retained admission fixture closure: %w", pair.entry.Kind, err)
		}

		directory := path.Dir(pair.entry.EvidencePath)
		subjects = append(subjects, RetainedSubject{
			Kind: pair.entry.Kind, Role: roleAdmissionEvidence,
			Path:         pair.entry.EvidencePath,
			Canonical:    append([]byte(nil), canonical...),
			SigstorePath: path.Join(directory, "evidence.sigstore.json"),
		})

		positiveNames := make([]string, 0, len(evidence.Fixtures.Positive))
		for _, fixture := range evidence.Fixtures.Positive {
			positiveNames = append(positiveNames, fixture.Name)
		}
		for _, retained := range []struct {
			role   string
			path   string
			digest string
			bundle string
			proof  standardform.ConformanceProof
		}{
			{role: roleHostReport, path: pair.entry.HostReportPath, digest: pair.entry.HostReportDigest, bundle: pair.entry.HostReportSigstoreBundle, proof: evidence.Conformance.Host},
			{role: roleProviderReport, path: pair.entry.ProviderReportPath, digest: pair.entry.ProviderReportDigest, bundle: pair.entry.ProviderReportSigstoreBundle, proof: evidence.Conformance.Provider},
		} {
			runnerReport, reportRaw, err := readCanonicalRunnerReport(admissionRoot, retained.path, maxReportBytes)
			if err != nil {
				return fmt.Errorf("%s %s: %w", pair.entry.Kind, retained.role, err)
			}
			if formpackage.DigestBytes(reportRaw) != retained.digest || retained.proof.EvidenceDigest != retained.digest {
				return fmt.Errorf("%s %s digest does not match the admission proof", pair.entry.Kind, retained.role)
			}
			roleNegativeBindings, err := negativeFixtureBindingsForRole(retained.role, negativeBindings)
			if err != nil {
				return fmt.Errorf("%s %s negative fixture stages: %w", pair.entry.Kind, retained.role, err)
			}
			roleNegativeNames := sortedBindingNames(roleNegativeBindings)
			if err := validateRunnerReport(runnerReport, retained.role, retained.proof, positiveNames, roleNegativeNames, positiveBindings, roleNegativeBindings); err != nil {
				return fmt.Errorf("%s: %w", pair.entry.Kind, err)
			}
			subjects = append(subjects, RetainedSubject{
				Kind: pair.entry.Kind, Role: retained.role, Path: retained.path,
				Canonical: reportRaw, SigstorePath: retained.bundle,
			})
		}

		packageVersion := set.PackageVersion
		if set.Generation != "" {
			packageVersion = pair.entry.FormRef.DefinitionVersion
		}
		packageIndex, err := verifyPackageReleaseReadback(root, admissionRoot, pair, packageVersion, set.Generation != "")
		if err != nil {
			return fmt.Errorf("%s package release readback: %w", pair.entry.Kind, err)
		}
		subjects = append(subjects, RetainedSubject{
			Kind: pair.entry.Kind, Role: rolePackageIndex, Path: pair.entry.PackageIndexPath,
			Canonical: packageIndex, SigstorePath: pair.entry.PackageIndexSigstoreBundle,
		})
	}

	extraProviderSubjects, providerIdentity, err := verifyFullProviderReportClosure(root, retainedRoot, set, selectedKinds)
	if err != nil {
		return fmt.Errorf("full provider-report closure: %w", err)
	}
	subjects = append(subjects, extraProviderSubjects...)

	registryReadback, registryRaw, err := verifyRegistryReadback(root, admissionRoot, set)
	if err != nil {
		return fmt.Errorf("provider Registry install/readback: %w", err)
	}
	if set.Generation != "" {
		if err := validateProviderRegistryIdentity(providerIdentity, registryReadback); err != nil {
			return err
		}
	}
	subjects = append(subjects, RetainedSubject{
		Kind: "provider", Role: roleRegistryReadback, Path: set.ProviderRegistryReadback.Path,
		Canonical: registryRaw, SigstorePath: set.ProviderRegistryReadback.SigstoreBundle,
	})
	if refVerifier == nil {
		return fmt.Errorf("Form admission activation is blocked: release-ref verifier is required")
	}
	if err := refVerifier.VerifyReleaseRefs(root, set, registryReadback); err != nil {
		return fmt.Errorf("Form admission activation is blocked: immutable release refs: %w", err)
	}

	if verifier == nil {
		verifier, err = loadOfflineRetainedSubjectVerifier(admissionRoot)
		if err != nil {
			return fmt.Errorf("Form admission activation is blocked: load offline retained-subject trust: %w", err)
		}
	}
	if err := verifier.VerifyRetainedSubjects(admissionRoot, set, subjects); err != nil {
		return fmt.Errorf("Form admission activation is blocked: authenticate retained standard-admission closure: %w", err)
	}
	return nil
}

func bindExactPackageFixtures(
	packageRoot string,
	definition formpackage.FormDefinition,
	evidence standardform.AdmissionEvidence,
) (map[string]fixtureDigestBinding, map[string]fixtureDigestBinding, error) {
	positiveEvidence := make(map[string]standardform.PositiveFixture, len(evidence.Fixtures.Positive))
	for _, fixture := range evidence.Fixtures.Positive {
		positiveEvidence[fixture.Name] = fixture
	}
	negativeEvidence := make(map[string]standardform.NegativeFixture, len(evidence.Fixtures.Negative))
	for _, fixture := range evidence.Fixtures.Negative {
		negativeEvidence[fixture.Name] = fixture
	}
	positive := make(map[string]fixtureDigestBinding, len(definition.ConformanceFixtures))
	for _, fixture := range definition.ConformanceFixtures {
		retained, ok := positiveEvidence[fixture.Name]
		if !ok {
			return nil, nil, fmt.Errorf("positive fixture %q is missing from admission evidence", fixture.Name)
		}
		desiredRaw, desired, err := readFixtureObject(packageRoot, fixture.DesiredPath)
		if err != nil {
			return nil, nil, fmt.Errorf("positive fixture %q desired: %w", fixture.Name, err)
		}
		if !reflect.DeepEqual(retained.Desired, desired) {
			return nil, nil, fmt.Errorf("positive fixture %q desired does not equal the retained package fixture", fixture.Name)
		}
		if err := requireFixtureObject(packageRoot, fixture.ObservedPath, retained.Observed, fixture.Name+" observed"); err != nil {
			return nil, nil, err
		}
		if err := requireFixtureObject(packageRoot, fixture.OutputPath, retained.Output, fixture.Name+" output"); err != nil {
			return nil, nil, err
		}
		effectiveDigest, err := formpackage.DigestCanonicalJSON(desiredRaw)
		if err != nil {
			return nil, nil, err
		}
		positive[fixture.Name] = fixtureDigestBinding{
			PackageFixtureDigest: formpackage.DigestBytes(desiredRaw),
			EffectiveInputDigest: effectiveDigest,
		}
	}
	negative := make(map[string]fixtureDigestBinding, len(definition.NegativeFixtures))
	for _, fixture := range definition.NegativeFixtures {
		if fixture.Stage == "" {
			return nil, nil, fmt.Errorf("negative fixture %q has an empty stage", fixture.Name)
		}
		stage, err := normalizeNegativeFixtureStage(fixture.Stage)
		if err != nil {
			return nil, nil, fmt.Errorf("negative fixture %q: %w", fixture.Name, err)
		}
		retained, ok := negativeEvidence[fixture.Name]
		if !ok {
			return nil, nil, fmt.Errorf("negative fixture %q is missing from admission evidence", fixture.Name)
		}
		inputRaw, input, err := readFixtureObject(packageRoot, fixture.InputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("negative fixture %q input: %w", fixture.Name, err)
		}
		if retained.Stage != fixture.Stage || !reflect.DeepEqual(retained.Input, input) {
			return nil, nil, fmt.Errorf("negative fixture %q does not equal the retained package fixture", fixture.Name)
		}
		effectiveDigest, err := formpackage.DigestCanonicalJSON(inputRaw)
		if err != nil {
			return nil, nil, err
		}
		negative[fixture.Name] = fixtureDigestBinding{
			PackageFixtureDigest: formpackage.DigestBytes(inputRaw),
			EffectiveInputDigest: effectiveDigest,
			Stage:                stage,
		}
	}
	if len(positive) != len(positiveEvidence) || len(negative) != len(negativeEvidence) {
		return nil, nil, fmt.Errorf("admission evidence fixture names do not equal the retained package closure")
	}
	return positive, negative, nil
}

func readFixtureObject(packageRoot, relative string) ([]byte, map[string]any, error) {
	if strings.TrimSpace(relative) == "" {
		return nil, nil, fmt.Errorf("fixture path is empty")
	}
	raw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(relative)))
	if err != nil {
		return nil, nil, err
	}
	var value map[string]any
	if err := decodeStrictJSON(raw, &value); err != nil {
		return nil, nil, err
	}
	return raw, value, nil
}

func requireFixtureObject(packageRoot, relative string, retained map[string]any, label string) error {
	if relative == "" {
		if len(retained) != 0 {
			return fmt.Errorf("%s exists without a retained package path", label)
		}
		return nil
	}
	_, value, err := readFixtureObject(packageRoot, relative)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if !reflect.DeepEqual(retained, value) {
		return fmt.Errorf("%s does not equal the retained package fixture", label)
	}
	return nil
}

type matchedEntry struct {
	entry     SetEntry
	candidate Candidate
}

func validateSet(set Set, candidates CandidateSet) ([]matchedEntry, error) {
	if candidates.Generation == "" {
		if set.Format != setFormatV2 {
			return nil, fmt.Errorf("format is %q, want %q", set.Format, setFormatV2)
		}
		if set.Generation != "" || set.DefinitionVersion != candidates.DefinitionVersion || set.PackageVersion != candidates.PackageVersion {
			return nil, fmt.Errorf("definition/package version does not match the compiled candidate set")
		}
		if set.ProviderReportClosure != nil {
			return nil, fmt.Errorf("historical set must not relabel a current provider-report closure")
		}
	} else {
		if set.Format != setFormatV3 {
			return nil, fmt.Errorf("format is %q, want %q", set.Format, setFormatV3)
		}
		if set.Generation != candidates.Generation || set.DefinitionVersion != "" || set.PackageVersion != "" {
			return nil, fmt.Errorf("generation does not match the compiled mixed-version candidate set")
		}
	}
	if !admissionReleaseTagPattern.MatchString(set.AdmissionReleaseTag) {
		return nil, fmt.Errorf("admissionReleaseTag %q is not a forms/admissions/v* release tag", set.AdmissionReleaseTag)
	}
	if len(set.Entries) != len(candidates.Entries) {
		return nil, fmt.Errorf("entry closure has %d entries, want exactly %d", len(set.Entries), len(candidates.Entries))
	}
	if set.ProviderRegistryReadback.Path != "registry/provider-readback.json" ||
		set.ProviderRegistryReadback.SigstoreBundle != "registry/provider-readback.sigstore.json" ||
		!formpackage.ValidDigest(set.ProviderRegistryReadback.Digest) {
		return nil, fmt.Errorf("providerRegistryReadback must bind the canonical retained report and bundle")
	}

	expected := make(map[string]Candidate, len(candidates.Entries))
	for _, candidate := range candidates.Entries {
		expected[candidate.Kind] = candidate
	}
	providerClosure, err := validateProviderReportClosure(set.ProviderReportClosure, candidates)
	if err != nil {
		return nil, err
	}
	seenKinds := make(map[string]struct{}, len(set.Entries))
	seenSlugs := make(map[string]struct{}, len(set.Entries))
	ordered := make([]matchedEntry, 0, len(set.Entries))
	for index, entry := range set.Entries {
		candidate, ok := expected[entry.Kind]
		if !ok {
			return nil, fmt.Errorf("entries[%d] contains unknown kind %q", index, entry.Kind)
		}
		if _, duplicate := seenKinds[entry.Kind]; duplicate {
			return nil, fmt.Errorf("entries[%d] duplicates kind %q", index, entry.Kind)
		}
		seenKinds[entry.Kind] = struct{}{}
		if _, duplicate := seenSlugs[entry.Slug]; duplicate {
			return nil, fmt.Errorf("entries[%d] duplicates slug %q", index, entry.Slug)
		}
		seenSlugs[entry.Slug] = struct{}{}
		if entry.Slug != candidate.Slug || entry.FormRef != candidate.FormRef || entry.PackageDigest != candidate.PackageDigest {
			return nil, fmt.Errorf("%s retained set identity does not match the compiled candidate", entry.Kind)
		}
		if entry.AdmissionStatus != "portable-standard" {
			return nil, fmt.Errorf("%s admissionStatus is %q, want portable-standard", entry.Kind, entry.AdmissionStatus)
		}
		packageVersion := candidates.PackageVersion
		if candidates.Generation != "" {
			packageVersion = entry.FormRef.DefinitionVersion
		}
		expectedReleaseTag := "forms/" + releaseIDForKind(entry.Kind) + "/v" + packageVersion
		if !packageReleaseTagPattern.MatchString(entry.ReleaseTag) || entry.ReleaseTag != expectedReleaseTag {
			return nil, fmt.Errorf("%s releaseTag %q is not the canonical kind release tag", entry.Kind, entry.ReleaseTag)
		}
		if !releaseCommitPattern.MatchString(entry.ReleaseCommit) || !releaseCommitPattern.MatchString(entry.ReleaseToolingCommit) {
			return nil, fmt.Errorf("%s releaseCommit and releaseToolingCommit must be lowercase 40-hex commits", entry.Kind)
		}
		for label, digest := range map[string]string{
			"evidenceDigest":               entry.EvidenceDigest,
			"hostReportDigest":             entry.HostReportDigest,
			"providerReportDigest":         entry.ProviderReportDigest,
			"packageReleaseManifestDigest": entry.PackageReleaseManifestDigest,
		} {
			if !formpackage.ValidDigest(digest) {
				return nil, fmt.Errorf("%s %s is not a canonical SHA-256 digest", entry.Kind, label)
			}
		}
		if err := validateEntryPaths(entry, packageVersion, candidates.Generation != ""); err != nil {
			return nil, fmt.Errorf("%s retained paths: %w", entry.Kind, err)
		}
		if candidates.Generation != "" {
			retained, ok := providerClosure[entry.Kind]
			if !ok || retained.Identity != (standardform.InstalledFormReference{FormRef: entry.FormRef, PackageDigest: entry.PackageDigest}) ||
				retained.ReportPath != entry.ProviderReportPath ||
				retained.ReportDigest != entry.ProviderReportDigest ||
				retained.SigstoreBundle != entry.ProviderReportSigstoreBundle {
				return nil, fmt.Errorf("%s selected provider report is not the exact member of the full closure", entry.Kind)
			}
		}
		ordered = append(ordered, matchedEntry{entry: entry, candidate: candidate})
	}
	return ordered, nil
}

func validateCandidateSet(candidates CandidateSet) error {
	legacy := strings.TrimSpace(candidates.Generation) == "" &&
		strings.TrimSpace(candidates.DefinitionVersion) != "" &&
		strings.TrimSpace(candidates.PackageVersion) != ""
	generationAware := strings.TrimSpace(candidates.Generation) != "" &&
		candidates.DefinitionVersion == "" && candidates.PackageVersion == ""
	if (!legacy && !generationAware) || len(candidates.Entries) == 0 {
		return fmt.Errorf("exactly one legacy-version or generation-aware identity and entries are required")
	}
	seenKinds := make(map[string]struct{}, len(candidates.Entries))
	seenSlugs := make(map[string]struct{}, len(candidates.Entries))
	for index, candidate := range candidates.Entries {
		if _, duplicate := seenKinds[candidate.Kind]; duplicate {
			return fmt.Errorf("entries[%d] duplicates kind %q", index, candidate.Kind)
		}
		seenKinds[candidate.Kind] = struct{}{}
		if !slugPattern.MatchString(candidate.Slug) {
			return fmt.Errorf("entries[%d] has invalid slug %q", index, candidate.Slug)
		}
		if _, duplicate := seenSlugs[candidate.Slug]; duplicate {
			return fmt.Errorf("entries[%d] duplicates slug %q", index, candidate.Slug)
		}
		seenSlugs[candidate.Slug] = struct{}{}
		if candidate.FormRef.APIVersion != formpackage.FormAPIVersion || candidate.FormRef.Kind != candidate.Kind ||
			candidate.FormRef.DefinitionVersion == "" ||
			(legacy && candidate.FormRef.DefinitionVersion != candidates.DefinitionVersion) ||
			!formpackage.ValidDigest(candidate.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(candidate.PackageDigest) {
			return fmt.Errorf("entries[%d] has an invalid exact candidate identity", index)
		}
		if err := validateRelativePath(candidate.PackagePath); err != nil {
			return fmt.Errorf("entries[%d] package path: %w", index, err)
		}
	}
	return nil
}

func validateEntryPaths(entry SetEntry, packageVersion string, current bool) error {
	directory := path.Join("packages", entry.Slug)
	for label, value := range map[string]string{
		"evidencePath":                 entry.EvidencePath,
		"hostReportPath":               entry.HostReportPath,
		"hostReportSigstoreBundle":     entry.HostReportSigstoreBundle,
		"providerReportPath":           entry.ProviderReportPath,
		"providerReportSigstoreBundle": entry.ProviderReportSigstoreBundle,
		"packageReleaseManifestPath":   entry.PackageReleaseManifestPath,
		"packageIndexPath":             entry.PackageIndexPath,
		"packageIndexSigstoreBundle":   entry.PackageIndexSigstoreBundle,
	} {
		if err := validateRelativePath(value); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	wantProviderReport := path.Join(directory, "provider-report.json")
	wantProviderBundle := path.Join(directory, "provider-report.sigstore.json")
	if current {
		wantProviderReport = path.Join("provider-closure", "packages", entry.Slug, "provider-report.json")
		wantProviderBundle = path.Join("provider-closure", "packages", entry.Slug, "provider-report.sigstore.json")
	}
	if entry.EvidencePath != path.Join(directory, "evidence.json") ||
		entry.HostReportPath != path.Join(directory, "host-report.json") ||
		entry.HostReportSigstoreBundle != path.Join(directory, "host-report.sigstore.json") ||
		entry.ProviderReportPath != wantProviderReport ||
		entry.ProviderReportSigstoreBundle != wantProviderBundle {
		return fmt.Errorf("package evidence/report paths must use the canonical %s directory", directory)
	}
	releaseID := releaseIDForKind(entry.Kind)
	releaseDirectory := path.Join("releases", releaseID, packageVersion)
	base := "takoform-form-" + releaseID + "_" + packageVersion + "_package-index"
	if entry.PackageReleaseManifestPath != path.Join(releaseDirectory, "release-manifest.json") ||
		entry.PackageIndexPath != path.Join(releaseDirectory, base+".json") ||
		entry.PackageIndexSigstoreBundle != path.Join(releaseDirectory, base+".sigstore.json") {
		return fmt.Errorf("package release paths must use the canonical %s directory and asset names", releaseDirectory)
	}
	return nil
}

func validateProviderReportClosure(closure *ProviderReportClosure, candidates CandidateSet) (map[string]ProviderReportClosureEntry, error) {
	if candidates.Generation == "" {
		return nil, nil
	}
	if closure == nil || closure.Generation != "portable-v1" || len(closure.Reports) != 34 {
		return nil, fmt.Errorf("current admission requires the exact portable-v1 34-report provider closure")
	}
	for label, value := range map[string]string{
		"manifestPath":       closure.ManifestPath,
		"signedManifestPath": closure.SignedManifestPath,
		"checksumsPath":      closure.ChecksumsPath,
	} {
		if err := validateRelativePath(value); err != nil {
			return nil, fmt.Errorf("providerReportClosure %s: %w", label, err)
		}
	}
	if closure.ManifestPath != "provider-closure/provider-report-manifest.json" ||
		closure.SignedManifestPath != "provider-closure/signed-provider-report-candidate.json" ||
		closure.ChecksumsPath != "provider-closure/SHA256SUMS" {
		return nil, fmt.Errorf("providerReportClosure manifest paths are not canonical")
	}
	for label, digest := range map[string]string{
		"manifestDigest":       closure.ManifestDigest,
		"signedManifestDigest": closure.SignedManifestDigest,
		"checksumsDigest":      closure.ChecksumsDigest,
	} {
		if !formpackage.ValidDigest(digest) {
			return nil, fmt.Errorf("providerReportClosure %s is not a canonical SHA-256", label)
		}
	}
	byKind := make(map[string]ProviderReportClosureEntry, len(closure.Reports))
	seenSlugs := make(map[string]struct{}, len(closure.Reports))
	for index, report := range closure.Reports {
		if report.Kind == "" || report.Identity.FormRef.Kind != report.Kind ||
			report.Identity.FormRef.APIVersion != formpackage.FormAPIVersion ||
			report.Identity.FormRef.DefinitionVersion == "" ||
			!formpackage.ValidDigest(report.Identity.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(report.Identity.PackageDigest) ||
			!slugPattern.MatchString(report.Slug) ||
			!formpackage.ValidDigest(report.ReportDigest) {
			return nil, fmt.Errorf("providerReportClosure reports[%d] has invalid identity", index)
		}
		if _, duplicate := byKind[report.Kind]; duplicate {
			return nil, fmt.Errorf("providerReportClosure reports[%d] duplicates kind %s", index, report.Kind)
		}
		if _, duplicate := seenSlugs[report.Slug]; duplicate {
			return nil, fmt.Errorf("providerReportClosure reports[%d] duplicates slug %s", index, report.Slug)
		}
		seenSlugs[report.Slug] = struct{}{}
		wantDirectory := path.Join("provider-closure", "packages", report.Slug)
		if report.ReportPath != path.Join(wantDirectory, "provider-report.json") ||
			report.SigstoreBundle != path.Join(wantDirectory, "provider-report.sigstore.json") {
			return nil, fmt.Errorf("providerReportClosure reports[%d] paths are not canonical", index)
		}
		byKind[report.Kind] = report
	}
	for _, candidate := range candidates.Entries {
		report, ok := byKind[candidate.Kind]
		if !ok || report.Slug != candidate.Slug ||
			report.Identity != (standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest}) {
			return nil, fmt.Errorf("providerReportClosure omits exact selected %s identity", candidate.Kind)
		}
	}
	return byKind, nil
}

func validateRelativePath(value string) error {
	if value == "" || strings.Contains(value, `\`) || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("%q is not a clean repository-relative slash path", value)
	}
	return nil
}

func releaseIDForKind(kind string) string {
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind))
	return "k-" + strings.ToLower(encoded)
}

func readDefinition(packageRoot string) (formpackage.FormDefinition, error) {
	indexRaw, err := readRetainedRegularFile(filepath.Join(packageRoot, formpackage.PackageIndexFilename), maxSetBytes)
	if err != nil {
		return formpackage.FormDefinition{}, err
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return formpackage.FormDefinition{}, err
	}
	definitionRaw, err := readRetainedRegularFile(filepath.Join(packageRoot, filepath.FromSlash(index.DefinitionPath)), maxEvidenceBytes)
	if err != nil {
		return formpackage.FormDefinition{}, err
	}
	return formpackage.ValidateDefinition(definitionRaw)
}

func decodeStrictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func readRetainedRegularFile(filename string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("retained artifact is not a regular file")
	}
	if info.Size() > maximum {
		return nil, fmt.Errorf("retained artifact exceeds %d bytes", maximum)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("retained artifact exceeds %d bytes", maximum)
	}
	return raw, nil
}

func readRetainedRelativeFile(root, relative string, maximum int64) ([]byte, error) {
	if err := validateRelativePath(relative); err != nil {
		return nil, err
	}
	current := root
	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if index < len(parts)-1 {
			if !info.IsDir() {
				return nil, fmt.Errorf("retained path component %q is not a directory", strings.Join(parts[:index+1], "/"))
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("retained artifact is not a regular file")
		}
	}
	return readRetainedRegularFile(current, maximum)
}
