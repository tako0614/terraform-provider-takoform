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
}

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
		result[name] = fixtureDigestBinding{
			PackageFixtureDigest: value.PackageFixtureDigest,
			EffectiveInputDigest: value.EffectiveInputDigest,
		}
	}
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
) (Set, []byte, error) {
	set := Set{
		Format: setFormat, DefinitionVersion: candidates.DefinitionVersion,
		PackageVersion: candidates.PackageVersion, AdmissionReleaseTag: admissionReleaseTag,
		ProviderRegistryReadback: registry, Entries: append([]SetEntry(nil), entries...),
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
