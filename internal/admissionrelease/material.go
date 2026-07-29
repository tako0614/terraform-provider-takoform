package admissionrelease

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

// FixtureDigestBinding binds one named package fixture to both its retained
// source bytes and the canonical effective input executed by a host.
type FixtureDigestBinding struct {
	PackageFixtureDigest string
	EffectiveInputDigest string
	// Stage is meaningful only for negative fixtures. Empty is treated as
	// "desired" for compatibility with callers predating stage-aware reports.
	Stage string
}

// NegativeFixtureExpectation declares one exact provider negative-fixture
// name and validation stage. Provider reports cover desired and observed
// stages; host reports cover desired-stage fixture bindings only.
type NegativeFixtureExpectation struct {
	Name  string
	Stage string
}

const (
	negativeFixtureStageDesired  = "desired"
	negativeFixtureStageObserved = "observed"
)

// ValidateCanonicalHostRunnerReport verifies one unsigned host-report subject
// against an exact Form identity and exact package/effective fixture bytes. It
// does not authenticate a bundle, retain evidence, sign, publish, or admit it.
func ValidateCanonicalHostRunnerReport(
	raw []byte,
	identity standardform.InstalledFormReference,
	positiveBindings, negativeBindings map[string]FixtureDigestBinding,
) (RunnerReport, error) {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return RunnerReport{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return RunnerReport{}, fmt.Errorf("host-report bytes are not RFC 8785 canonical")
	}
	var report RunnerReport
	if err := decodeStrictJSON(raw, &report); err != nil {
		return RunnerReport{}, err
	}
	positive, err := convertFixtureBindings("positive", positiveBindings)
	if err != nil {
		return RunnerReport{}, err
	}
	negative, err := convertFixtureBindings("negative", negativeBindings)
	if err != nil {
		return RunnerReport{}, err
	}
	negative, err = negativeFixtureBindingsForRole(roleHostReport, negative)
	if err != nil {
		return RunnerReport{}, err
	}
	positiveNames := sortedBindingNames(positive)
	negativeNames := sortedBindingNames(negative)
	proof := standardform.ConformanceProof{
		Subject: report.Subject, RunnerVersion: report.RunnerVersion, Identity: identity, Status: "passed",
		PositiveFixtures: positiveNames, NegativeFixtures: negativeNames,
	}
	if err := validateRunnerReport(report, roleHostReport, proof, positiveNames, negativeNames, positive, negative); err != nil {
		return RunnerReport{}, err
	}
	return report, nil
}

func convertFixtureBindings(label string, values map[string]FixtureDigestBinding) (map[string]fixtureDigestBinding, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%s fixture bindings are required", label)
	}
	result := make(map[string]fixtureDigestBinding, len(values))
	for name, value := range values {
		if name == "" || !formpackage.ValidDigest(value.PackageFixtureDigest) || !formpackage.ValidDigest(value.EffectiveInputDigest) {
			return nil, fmt.Errorf("%s fixture binding %q is invalid", label, name)
		}
		stage := value.Stage
		switch label {
		case "positive":
			if stage != "" {
				return nil, fmt.Errorf("positive fixture binding %q must not declare a stage", name)
			}
		case "negative":
			var err error
			stage, err = normalizeNegativeFixtureStage(stage)
			if err != nil {
				return nil, fmt.Errorf("negative fixture binding %q: %w", name, err)
			}
		default:
			return nil, fmt.Errorf("unknown fixture binding class %q", label)
		}
		result[name] = fixtureDigestBinding{
			PackageFixtureDigest: value.PackageFixtureDigest,
			EffectiveInputDigest: value.EffectiveInputDigest,
			Stage:                stage,
		}
	}
	return result, nil
}

func normalizeNegativeFixtureStage(stage string) (string, error) {
	switch stage {
	case "":
		return negativeFixtureStageDesired, nil
	case negativeFixtureStageDesired:
		return negativeFixtureStageDesired, nil
	case negativeFixtureStageObserved:
		return negativeFixtureStageObserved, nil
	default:
		return "", fmt.Errorf("stage %q is not supported; want desired or observed", stage)
	}
}

func negativeFixtureBindingsForRole(role string, values map[string]fixtureDigestBinding) (map[string]fixtureDigestBinding, error) {
	expectations := make([]NegativeFixtureExpectation, 0, len(values))
	normalized := make(map[string]fixtureDigestBinding, len(values))
	for name, value := range values {
		stage, err := normalizeNegativeFixtureStage(value.Stage)
		if err != nil {
			return nil, fmt.Errorf("negative fixture binding %q: %w", name, err)
		}
		value.Stage = stage
		normalized[name] = value
		expectations = append(expectations, NegativeFixtureExpectation{Name: name, Stage: stage})
	}
	names, err := negativeFixtureNamesForRole(role, expectations)
	if err != nil {
		return nil, err
	}
	result := make(map[string]fixtureDigestBinding, len(names))
	for _, name := range names {
		result[name] = normalized[name]
	}
	return result, nil
}

func negativeFixtureNamesForRole(role string, values []NegativeFixtureExpectation) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%s negative fixture expectations are required", role)
	}
	fixtures := make([]standardform.NegativeFixture, 0, len(values))
	for _, value := range values {
		stage, err := normalizeNegativeFixtureStage(value.Stage)
		if err != nil {
			return nil, fmt.Errorf("%s negative fixture %q: %w", role, value.Name, err)
		}
		fixtures = append(fixtures, standardform.NegativeFixture{Name: value.Name, Stage: stage})
	}
	var (
		result []string
		err    error
	)
	switch role {
	case roleHostReport:
		result, err = standardform.HostNegativeFixtureNames(fixtures)
	case roleProviderReport:
		result, err = standardform.ProviderNegativeFixtureNames(fixtures)
	default:
		return nil, fmt.Errorf("unknown runner report role %q", role)
	}
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%s negative fixture expectations are required", role)
	}
	sort.Strings(result)
	return result, nil
}

func sortedBindingNames(values map[string]fixtureDigestBinding) []string {
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// BuildCanonicalSet constructs deterministic set@v2 bytes from exact retained
// inputs. It validates and orders entries by the compiled candidate set. It
// never writes, signs, publishes, or activates the set.
func BuildCanonicalSet(
	candidates CandidateSet,
	admissionReleaseTag string,
	registry RegistryReadbackRef,
	entries []SetEntry,
	providerClosures ...ProviderReportClosure,
) (Set, []byte, error) {
	format := setFormatV2
	if candidates.Generation != "" {
		format = setFormatV3
	}
	set := Set{
		Format: format, Generation: candidates.Generation, DefinitionVersion: candidates.DefinitionVersion,
		PackageVersion: candidates.PackageVersion, AdmissionReleaseTag: admissionReleaseTag,
		ProviderRegistryReadback: registry, Entries: append([]SetEntry(nil), entries...),
	}
	if len(providerClosures) > 1 {
		return Set{}, nil, fmt.Errorf("at most one provider report closure is allowed")
	}
	if len(providerClosures) == 1 {
		closure := providerClosures[0]
		set.ProviderReportClosure = &closure
	}
	if _, err := validateSet(set, candidates); err != nil {
		return Set{}, nil, err
	}
	byKind := make(map[string]SetEntry, len(entries))
	for _, entry := range entries {
		byKind[entry.Kind] = entry
	}
	set.Entries = make([]SetEntry, 0, len(candidates.Entries))
	for _, candidate := range candidates.Entries {
		set.Entries = append(set.Entries, byKind[candidate.Kind])
	}
	if _, err := validateSet(set, candidates); err != nil {
		return Set{}, nil, err
	}
	raw, err := json.Marshal(set)
	if err != nil {
		return Set{}, nil, err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return Set{}, nil, err
	}
	return set, canonical, nil
}
