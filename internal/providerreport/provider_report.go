// Package providerreport renders unsigned, canonical provider conformance
// reports by executing the current provider candidate against the same exact
// reviewed candidate release-source fixtures it embeds. Historical publication
// authentication is separate; this package does not sign, publish, retain, or
// admit reports.
package providerreport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	reportFormat     = "takoform.standard-runner-report@v1"
	providerRole     = "provider-report"
	providerProtocol = "Terraform provider protocol v6 + versioned Form host HTTP"
	maxArchiveBytes  = 64 << 20
	maxPayloadBytes  = 16 << 20
)

type PublishedFixture struct {
	Kind         string
	Slug         string
	Identity     standardform.InstalledFormReference
	PositiveName string
	Positive     map[string]any
	NegativeName string
	Negative     map[string]any
}

type GeneratedReport struct {
	kind      string
	slug      string
	report    admissionrelease.RunnerReport
	canonical []byte
	digest    string
}

func (report GeneratedReport) Subject() string { return report.report.Subject }

// Generate verifies the provider's current all-or-nothing candidate and its
// reviewed release-source bytes, executes those exact fixtures plus the full
// provider lifecycle through the real provider protocol, and returns one
// unsigned canonical report per kind. Immutable publication authentication is
// a separate post-publication gate and must never relabel a different run.
func Generate(ctx context.Context, root, cliPath string) ([]GeneratedReport, error) {
	fixtures, err := LoadCandidateFixtures(root)
	if err != nil {
		return nil, err
	}
	lifecycle, err := providerlifecycle.Run(ctx, root, cliPath)
	if err != nil {
		return nil, err
	}
	if err := providerlifecycle.Validate(lifecycle); err != nil {
		return nil, err
	}
	cases := make([]providerlifecycle.StandardFixtureCase, 0, len(fixtures))
	for _, fixture := range fixtures {
		cases = append(cases, providerlifecycle.StandardFixtureCase{
			Kind: fixture.Kind, Identity: fixture.Identity, PositiveName: fixture.PositiveName, Positive: fixture.Positive,
			NegativeName: fixture.NegativeName, Negative: fixture.Negative,
		})
	}
	fixtureRun, err := providerlifecycle.RunStandardFixtures(ctx, root, cliPath, cases)
	if err != nil {
		return nil, err
	}
	if lifecycle.CLI != fixtureRun.CLI || lifecycle.ProviderBinary != fixtureRun.ProviderBinary {
		return nil, fmt.Errorf("provider lifecycle and exact-fixture executions used different CLI or provider binary identities")
	}

	resources := make(map[string]providerlifecycle.ResourceEvidence, len(lifecycle.Resources))
	for _, resource := range lifecycle.Resources {
		resources[resource.Kind] = resource
	}
	fixtureEvidence := make(map[string]providerlifecycle.StandardFixtureEvidence, len(fixtureRun.Evidence))
	for _, evidence := range fixtureRun.Evidence {
		fixtureEvidence[evidence.Kind] = evidence
	}

	generated := make([]GeneratedReport, 0, len(fixtures))
	for _, fixture := range fixtures {
		resource, ok := resources[fixture.Kind]
		if !ok {
			return nil, fmt.Errorf("provider lifecycle omitted %s", fixture.Kind)
		}
		exact, ok := fixtureEvidence[fixture.Kind]
		if !ok || !reflect.DeepEqual(exact.Identity, fixture.Identity) || !exact.PositivePassed || !exact.NegativePassed || exact.PositiveName != fixture.PositiveName || exact.NegativeName != fixture.NegativeName || exact.NegativeErrorCode != standardform.InvalidArgumentErrorCode {
			return nil, fmt.Errorf("provider protocol fixture evidence is incomplete for %s", fixture.Kind)
		}
		checks := resource.Checks
		report := admissionrelease.RunnerReport{
			Format: reportFormat, Role: providerRole,
			Subject: "provider:" + lifecycle.CLI.ProviderAddress, RunnerVersion: lifecycle.ProviderBinary.Version,
			Identity: fixture.Identity, Status: "passed",
			Lifecycle: standardform.LifecycleAudit{
				Create: checks.Create, Read: checks.Read, Update: checks.Update, Delete: checks.Delete,
				Import: checks.NativeImport && checks.CLIImport, Observe: checks.Observe, Refresh: checks.Refresh, Drift: checks.DriftState,
			},
			PositiveFixtures: []admissionrelease.PositiveFixtureResult{{Name: exact.PositiveName, Passed: true}},
			NegativeFixtures: []admissionrelease.NegativeFixtureResult{{Name: exact.NegativeName, ErrorCode: exact.NegativeErrorCode, Passed: true}},
		}
		raw, err := json.Marshal(report)
		if err != nil {
			return nil, err
		}
		canonical, err := formpackage.Canonicalize(raw)
		if err != nil {
			return nil, err
		}
		if _, err := admissionrelease.ValidateCanonicalProviderRunnerReport(canonical, fixture.Identity, []string{fixture.PositiveName}, []string{fixture.NegativeName}); err != nil {
			return nil, fmt.Errorf("%s canonical provider-report: %w", fixture.Kind, err)
		}
		generated = append(generated, GeneratedReport{
			kind: fixture.Kind, slug: fixture.Slug, report: report, canonical: canonical, digest: formpackage.DigestBytes(canonical),
		})
	}
	return generated, nil
}

// Write writes a complete report set to a new or empty directory. The active
// admission tree is deliberately rejected so generation cannot activate or
// overwrite retained admission evidence.
func Write(repoRoot, outputRoot string, reports []GeneratedReport) error {
	if len(reports) != 10 {
		return fmt.Errorf("provider-report set has %d reports, want exactly 10", len(reports))
	}
	fixtures, err := LoadCandidateFixtures(repoRoot)
	if err != nil {
		return fmt.Errorf("reverify exact candidate release-source fixtures before writing provider reports: %w", err)
	}
	expected := make(map[string]PublishedFixture, len(fixtures))
	for _, fixture := range fixtures {
		expected[fixture.Kind] = fixture
	}
	seen := make(map[string]struct{}, len(reports))
	for _, generated := range reports {
		fixture, ok := expected[generated.kind]
		if !ok || generated.slug != fixture.Slug || generated.digest != formpackage.DigestBytes(generated.canonical) {
			return fmt.Errorf("provider-report set contains an incomplete or substituted %q output", generated.kind)
		}
		if _, duplicate := seen[generated.kind]; duplicate {
			return fmt.Errorf("provider-report set duplicates %s", generated.kind)
		}
		seen[generated.kind] = struct{}{}
		parsed, err := admissionrelease.ValidateCanonicalProviderRunnerReport(generated.canonical, fixture.Identity, []string{fixture.PositiveName}, []string{fixture.NegativeName})
		if err != nil {
			return fmt.Errorf("%s provider-report revalidation: %w", generated.kind, err)
		}
		if !reflect.DeepEqual(parsed, generated.report) {
			return fmt.Errorf("%s provider-report structured value differs from canonical bytes", generated.kind)
		}
	}
	repoAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	repoAbs, err = evalPathWithMissingLeaf(repoAbs)
	if err != nil {
		return err
	}
	outputAbs, err := filepath.Abs(outputRoot)
	if err != nil {
		return err
	}
	outputAbs, err = evalPathWithMissingLeaf(outputAbs)
	if err != nil {
		return err
	}
	admissionAbs := filepath.Join(repoAbs, "admission")
	if outputAbs == admissionAbs || strings.HasPrefix(outputAbs, admissionAbs+string(filepath.Separator)) {
		return fmt.Errorf("refusing to write unsigned provider reports under the admission tree")
	}
	if info, err := os.Stat(outputAbs); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("output path is not a directory")
		}
		entries, err := os.ReadDir(outputAbs)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("output directory must be empty")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err := os.MkdirAll(outputAbs, 0o755); err != nil {
		return err
	}

	for _, generated := range reports {
		directory := filepath.Join(outputAbs, generated.slug)
		relative, err := filepath.Rel(outputAbs, directory)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("%s provider-report output escapes the selected directory", generated.kind)
		}
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		filename := filepath.Join(directory, "provider-report.json")
		file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(generated.canonical)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func evalPathWithMissingLeaf(value string) (string, error) {
	missing := []string{}
	current := value
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return resolved, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("cannot resolve output path %q", value)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// LoadCandidateFixtures verifies and loads the exact reviewed release-source
// bytes embedded by the current provider candidate. It deliberately does not
// claim publication, signature, Registry readback, or admission authority.
func LoadCandidateFixtures(root string) ([]PublishedFixture, error) {
	if err := standardforms.Verify(root); err != nil {
		return nil, fmt.Errorf("verify exact standard Form candidate: %w", err)
	}
	var inventory standardforms.Inventory
	raw, err := os.ReadFile(filepath.Join(root, "forms", "standard-package-set.json"))
	if err != nil {
		return nil, err
	}
	if err := decodeStrictJSON(raw, &inventory); err != nil {
		return nil, err
	}
	if inventory.Format != "takoform.standard-package-set@v1" || inventory.Classification != "structural-candidate" || inventory.PublicationReady || inventory.AdmissionStatus != "external-required" || len(inventory.Packages) != len(standardforms.Specs) {
		return nil, fmt.Errorf("candidate package set does not retain the exact external-admission boundary")
	}
	entries := make(map[string]standardforms.InventoryEntry, len(inventory.Packages))
	for _, entry := range inventory.Packages {
		if _, duplicate := entries[entry.Kind]; duplicate {
			return nil, fmt.Errorf("candidate package set duplicates %s", entry.Kind)
		}
		entries[entry.Kind] = entry
	}
	fixtures := make([]PublishedFixture, 0, len(standardforms.Specs))
	for _, spec := range standardforms.Specs {
		entry, ok := entries[spec.Kind]
		if !ok || entry.FormRef.Kind != spec.Kind || entry.AdmissionStatus != "external-required" {
			return nil, fmt.Errorf("candidate package set omits exact %s identity", spec.Kind)
		}
		releaseID := "k-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(spec.Kind)))
		releaseRoot := filepath.Join(root, "forms", "releases", releaseID, inventory.PackageVersion)
		fixture, err := loadCandidateFixture(releaseRoot, spec.Slug, entry)
		if err != nil {
			return nil, fmt.Errorf("%s candidate release-source fixtures: %w", spec.Kind, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures, nil
}

func loadCandidateFixture(root, slug string, entry standardforms.InventoryEntry) (PublishedFixture, error) {
	report, err := formpackage.VerifyDirectory(root)
	if err != nil {
		return PublishedFixture{}, err
	}
	if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
		return PublishedFixture{}, fmt.Errorf("release source identity differs from the current candidate")
	}
	indexRaw, err := os.ReadFile(filepath.Join(root, formpackage.PackageIndexFilename))
	if err != nil {
		return PublishedFixture{}, err
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return PublishedFixture{}, err
	}
	definitionRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(index.DefinitionPath)))
	if err != nil {
		return PublishedFixture{}, err
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return PublishedFixture{}, err
	}
	if definition.Kind != entry.Kind || len(definition.ConformanceFixtures) != 1 || len(definition.NegativeFixtures) != 1 {
		return PublishedFixture{}, fmt.Errorf("definition does not retain exactly one positive and negative fixture")
	}
	positive := definition.ConformanceFixtures[0]
	negative := definition.NegativeFixtures[0]
	if negative.Stage != "desired" || negative.ExpectedFailure != "schema_validation_failed" {
		return PublishedFixture{}, fmt.Errorf("negative fixture is not the reviewed desired schema failure")
	}
	positiveDesired, err := readJSONMapFile(root, positive.DesiredPath)
	if err != nil {
		return PublishedFixture{}, fmt.Errorf("positive desired fixture: %w", err)
	}
	negativeDesired, err := readJSONMapFile(root, negative.InputPath)
	if err != nil {
		return PublishedFixture{}, fmt.Errorf("negative desired fixture: %w", err)
	}
	return PublishedFixture{
		Kind: entry.Kind, Slug: slug,
		Identity:     standardform.InstalledFormReference{FormRef: entry.FormRef, PackageDigest: entry.PackageDigest},
		PositiveName: positive.Name, Positive: positiveDesired, NegativeName: negative.Name, Negative: negativeDesired,
	}, nil
}

func readJSONMapFile(root, relative string) (map[string]any, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("package fixture path escapes its release source")
	}
	raw, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		return nil, err
	}
	return decodeJSONMap(raw)
}

// LoadPublishedFixtures verifies the historical offline immutable publication
// proof and reads its retained release archives. It is not an execution input
// for a newer provider candidate.
func LoadPublishedFixtures(root string) ([]PublishedFixture, error) {
	if err := standardforms.VerifyPublishedPackageSet(root); err != nil {
		return nil, fmt.Errorf("verify exact published package set: %w", err)
	}
	setRaw, err := os.ReadFile(filepath.Join(root, "admission", "v1", "published-package-set.json"))
	if err != nil {
		return nil, err
	}
	var set admissionrelease.PublishedPackageSet
	if err := decodeStrictJSON(setRaw, &set); err != nil {
		return nil, err
	}
	if set.Format != "takoform.published-package-set@v1" || set.PublicationStatus != "published-immutable" || set.AdmissionStatus != "external-required" || len(set.Entries) != 10 {
		return nil, fmt.Errorf("published package set does not retain the exact ten-package external-admission boundary")
	}

	fixtures := make([]PublishedFixture, 0, len(set.Entries))
	seen := make(map[string]struct{}, len(set.Entries))
	for _, entry := range set.Entries {
		if _, duplicate := seen[entry.Kind]; duplicate {
			return nil, fmt.Errorf("published package set duplicates %s", entry.Kind)
		}
		seen[entry.Kind] = struct{}{}
		fixture, err := loadPublishedFixture(root, set.PackageVersion, entry)
		if err != nil {
			return nil, fmt.Errorf("%s published package fixtures: %w", entry.Kind, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures, nil
}

func loadPublishedFixture(root, packageVersion string, entry admissionrelease.PublishedPackageEntry) (PublishedFixture, error) {
	indexPath := filepath.Join(root, "admission", "v1", filepath.FromSlash(entry.PackageIndexPath))
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		return PublishedFixture{}, err
	}
	if formpackage.DigestBytes(indexRaw) != entry.PackageDigest {
		return PublishedFixture{}, fmt.Errorf("retained package index digest does not match published package identity")
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return PublishedFixture{}, err
	}
	if index.FormRef != entry.FormRef || index.PackageVersion != packageVersion {
		return PublishedFixture{}, fmt.Errorf("retained package index identity drift")
	}
	base := strings.TrimSuffix(path.Base(entry.PackageIndexPath), "_package-index.json")
	if base == path.Base(entry.PackageIndexPath) {
		return PublishedFixture{}, fmt.Errorf("published package index path has no canonical suffix")
	}
	archiveRelative := path.Join(path.Dir(entry.PackageIndexPath), base+".tar.gz")
	archiveFiles, err := readRetainedArchive(filepath.Join(root, "admission", "v1", filepath.FromSlash(archiveRelative)))
	if err != nil {
		return PublishedFixture{}, err
	}
	if !bytes.Equal(archiveFiles[formpackage.PackageIndexFilename], indexRaw) {
		return PublishedFixture{}, fmt.Errorf("archive package index differs from retained signed index")
	}
	if len(archiveFiles) != len(index.Files)+1 {
		return PublishedFixture{}, fmt.Errorf("archive contains %d files, want exact index closure %d", len(archiveFiles), len(index.Files)+1)
	}
	listed := make(map[string]formpackage.PackageFile, len(index.Files))
	for _, file := range index.Files {
		payload, ok := archiveFiles[file.Path]
		if !ok || int64(len(payload)) != file.Size || formpackage.DigestBytes(payload) != file.Digest {
			return PublishedFixture{}, fmt.Errorf("archive payload %q does not match retained package index", file.Path)
		}
		listed[file.Path] = file
	}
	definitionRaw, ok := archiveFiles[index.DefinitionPath]
	if !ok {
		return PublishedFixture{}, fmt.Errorf("archive omits definition")
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return PublishedFixture{}, err
	}
	if definition.Kind != entry.Kind || len(definition.ConformanceFixtures) != 1 || len(definition.NegativeFixtures) != 1 {
		return PublishedFixture{}, fmt.Errorf("definition does not retain exactly one positive and negative fixture")
	}
	positive := definition.ConformanceFixtures[0]
	negative := definition.NegativeFixtures[0]
	if negative.Stage != "desired" || negative.ExpectedFailure != "schema_validation_failed" {
		return PublishedFixture{}, fmt.Errorf("negative fixture is not the reviewed desired schema failure")
	}
	if _, ok := listed[positive.DesiredPath]; !ok {
		return PublishedFixture{}, fmt.Errorf("positive desired fixture is not listed")
	}
	if _, ok := listed[negative.InputPath]; !ok {
		return PublishedFixture{}, fmt.Errorf("negative desired fixture is not listed")
	}
	positiveDesired, err := decodeJSONMap(archiveFiles[positive.DesiredPath])
	if err != nil {
		return PublishedFixture{}, fmt.Errorf("positive desired fixture: %w", err)
	}
	negativeDesired, err := decodeJSONMap(archiveFiles[negative.InputPath])
	if err != nil {
		return PublishedFixture{}, fmt.Errorf("negative desired fixture: %w", err)
	}
	return PublishedFixture{
		Kind: entry.Kind, Slug: entry.Slug,
		Identity:     standardform.InstalledFormReference{FormRef: entry.FormRef, PackageDigest: entry.PackageDigest},
		PositiveName: positive.Name, Positive: positiveDesired, NegativeName: negative.Name, Negative: negativeDesired,
	}, nil
}

func readRetainedArchive(filename string) (map[string][]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArchiveBytes {
		return nil, fmt.Errorf("retained package archive is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(io.LimitReader(file, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	files := map[string][]byte{}
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || header.Name == "" || path.Clean(header.Name) != header.Name || path.IsAbs(header.Name) || strings.HasPrefix(header.Name, "../") || strings.Contains(header.Name, `\`) || header.Size < 0 || header.Size > maxPayloadBytes {
			return nil, fmt.Errorf("archive entry %q is not a bounded regular relative file", header.Name)
		}
		if _, duplicate := files[header.Name]; duplicate {
			return nil, fmt.Errorf("archive duplicates %q", header.Name)
		}
		payload, err := io.ReadAll(io.LimitReader(reader, maxPayloadBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) != header.Size {
			return nil, fmt.Errorf("archive entry %q size mismatch", header.Name)
		}
		total += int64(len(payload))
		if total > maxArchiveBytes {
			return nil, fmt.Errorf("retained package archive exceeds the uncompressed payload limit")
		}
		files[header.Name] = payload
	}
	return files, nil
}

func decodeJSONMap(raw []byte) (map[string]any, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("desired fixture is not an object")
	}
	return value, nil
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

func SortedReportDigests(reports []GeneratedReport) []string {
	values := make([]string, 0, len(reports))
	for _, report := range reports {
		values = append(values, report.kind+"="+report.digest)
	}
	sort.Strings(values)
	return values
}

func ProtocolDescription() string {
	return providerProtocol
}
