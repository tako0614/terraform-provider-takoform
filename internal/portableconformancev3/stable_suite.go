package portableconformancev3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	StableSuiteFormat       = "takoform.conformance-suite@v1"
	StableGenericFormat     = "takoform.generic-host-corpus@v1"
	StableFamilyFormat      = "takoform.family-semantic-corpus@v1"
	StableCompositionFormat = "takoform.all-family-composition-corpus@v1"
	StableSuiteReportFormat = "takoform.reference-host-suite-report@v1"
)

type stableDigestRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type stableSuiteCorpusRecord struct {
	Path           string   `json:"path"`
	SHA256         string   `json:"sha256"`
	RequiredChecks []string `json:"requiredChecks"`
}

type stableSuiteFamilyRecord struct {
	Group            string   `json:"group"`
	Path             string   `json:"path"`
	SHA256           string   `json:"sha256"`
	RequiredChecks   []string `json:"requiredChecks"`
	DependencyGroups []string `json:"dependencyGroups"`
}

type StableSuiteManifest struct {
	Format      string                    `json:"format"`
	HostAPILane string                    `json:"hostApiLane"`
	Generic     stableSuiteCorpusRecord   `json:"generic"`
	Families    []stableSuiteFamilyRecord `json:"families"`
	Composition stableSuiteCorpusRecord   `json:"composition"`
	Runner      struct {
		Command      []string `json:"command"`
		ReportFormat string   `json:"reportFormat"`
	} `json:"runner"`
}

type stableScenario struct {
	Check    string         `json:"check"`
	Input    map[string]any `json:"input"`
	Expected map[string]any `json:"expected"`
}

type stableGenericCorpus struct {
	Format               string             `json:"format"`
	HostAPILane          string             `json:"hostApiLane"`
	RequiredChecks       []string           `json:"requiredChecks"`
	Scenarios            []stableScenario   `json:"scenarios"`
	PortableHostContract stableDigestRecord `json:"portableHostContract"`
}

type stableDesiredSchemaPin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type stableFamilyProbe struct {
	Name     string `json:"name"`
	Identity struct {
		FormRef       FormRef `json:"formRef"`
		PackageDigest string  `json:"packageDigest"`
	} `json:"identity"`
	LifecycleCapabilities []string               `json:"lifecycleCapabilities"`
	Desired               map[string]any         `json:"desired"`
	DesiredSchema         stableDesiredSchemaPin `json:"desiredSchema"`
}

type stableServiceFixture struct {
	ServiceRef  formpackage.StandardServiceRef `json:"serviceRef"`
	Satisfiable bool                           `json:"satisfiable"`
}

type stableFamilyCorpus struct {
	Format                  string                       `json:"format"`
	HostAPILane             string                       `json:"hostApiLane"`
	Group                   string                       `json:"group"`
	CandidateSet            stableDigestRecord           `json:"candidateSet"`
	RequiredChecks          []string                     `json:"requiredChecks"`
	Scenarios               []stableScenario             `json:"scenarios"`
	RunnerInput             map[string]stableFamilyProbe `json:"runnerInput"`
	StandardServiceFixtures []stableServiceFixture       `json:"standardServiceFixtures"`
}

type stableCompositionCorpus struct {
	Format         string           `json:"format"`
	HostAPILane    string           `json:"hostApiLane"`
	FamilyGroups   []string         `json:"familyGroups"`
	RequiredChecks []string         `json:"requiredChecks"`
	Scenarios      []stableScenario `json:"scenarios"`
}

type stableCandidateSet struct {
	Format            string `json:"format"`
	Family            string `json:"family"`
	FormMaturity      string `json:"formMaturity"`
	PackageAPIVersion string `json:"packageApiVersion"`
	PublicationStatus string `json:"publicationStatus"`
	AuthoringSource   string `json:"authoringSource"`
	AuthoringPolicy   string `json:"authoringPolicy"`
	Forms             []struct {
		Kind          string  `json:"kind"`
		Role          string  `json:"role"`
		Path          string  `json:"path"`
		FormRef       FormRef `json:"formRef"`
		PackageDigest string  `json:"packageDigest"`
	} `json:"forms"`
}

type StableSuiteCheckReport struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type StableSuiteCorpusReport struct {
	Path   string                   `json:"path"`
	SHA256 string                   `json:"sha256"`
	Checks []StableSuiteCheckReport `json:"checks"`
}

type StableSuiteFamilyReport struct {
	Group          string                   `json:"group"`
	Path           string                   `json:"path"`
	SHA256         string                   `json:"sha256"`
	Checks         []StableSuiteCheckReport `json:"checks"`
	RunnerFormRefs []FormRef                `json:"runnerFormRefs"`
}

type StableSuiteReport struct {
	Format      string                    `json:"format"`
	Status      string                    `json:"status"`
	HostAPILane string                    `json:"hostApiLane"`
	Suite       stableDigestRecord        `json:"suite"`
	Generic     StableSuiteCorpusReport   `json:"generic"`
	Families    []StableSuiteFamilyReport `json:"families"`
	Composition StableSuiteCorpusReport   `json:"composition"`
}

func stableDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func stableDigestHex(value string) (string, error) {
	if strings.HasPrefix(value, "sha256:") {
		value = strings.TrimPrefix(value, "sha256:")
	}
	if len(value) != 64 {
		return "", errors.New("SHA-256 digest must carry 64 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(value); err != nil || value != strings.ToLower(value) {
		return "", errors.New("SHA-256 digest must carry 64 lowercase hexadecimal characters")
	}
	return value, nil
}

func stableReadStrict(path string, target any) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := formpackage.DecodeStrictIJSON(raw, target); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return raw, nil
}

func stableResolve(root, parent, referenced string) (string, error) {
	if referenced == "" || filepath.IsAbs(referenced) || filepath.Clean(referenced) != referenced {
		return "", fmt.Errorf("invalid repository-relative path %q", referenced)
	}
	candidates := []string{filepath.Join(filepath.Dir(parent), referenced), filepath.Join(root, referenced)}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(absolute); err == nil && !info.IsDir() {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("referenced path %q does not name a repository file", referenced)
}

func stableVerifyDigest(path, want string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	wantHex, err := stableDigestHex(want)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if got := stableDigest(raw); got != wantHex {
		return nil, fmt.Errorf("%s digest = %s, want %s", path, got, wantHex)
	}
	return raw, nil
}

func stableChecks(checks []string) []StableSuiteCheckReport {
	out := make([]StableSuiteCheckReport, 0, len(checks))
	for _, check := range checks {
		out = append(out, StableSuiteCheckReport{Name: check, Status: "passed"})
	}
	return out
}

func validateStableScenarioCoverage(checks []string, scenarios []stableScenario, subject string) error {
	if len(checks) == 0 || len(scenarios) != len(checks) {
		return fmt.Errorf("%s scenario coverage is incomplete", subject)
	}
	for index, check := range checks {
		if scenarios[index].Check != check || len(scenarios[index].Input) == 0 || len(scenarios[index].Expected) == 0 {
			return fmt.Errorf("%s scenario %d does not concretely execute %q", subject, index, check)
		}
		if index > 0 && checks[index-1] >= check {
			return fmt.Errorf("%s required checks are not sorted and unique", subject)
		}
	}
	return nil
}

func stableFormRefKey(ref FormRef) string {
	return ref.APIVersion + "/" + ref.Kind + "@" + ref.DefinitionVersion + "#" + ref.SchemaDigest
}

func stableVerifyGeneric(ctx context.Context, root, corpusPath string, record stableSuiteCorpusRecord) error {
	var corpus stableGenericCorpus
	raw, err := stableReadStrict(corpusPath, &corpus)
	if err != nil {
		return err
	}
	if stableDigest(raw) != record.SHA256 || corpus.Format != StableGenericFormat ||
		corpus.HostAPILane != stableLane.APIVersion || !reflect.DeepEqual(corpus.RequiredChecks, record.RequiredChecks) {
		return errors.New("stable generic corpus identity or digest drifted")
	}
	if err := validateStableScenarioCoverage(record.RequiredChecks, corpus.Scenarios, "stable generic corpus"); err != nil {
		return err
	}
	contractPath, err := stableResolve(root, corpusPath, corpus.PortableHostContract.Path)
	if err != nil {
		return err
	}
	if _, err := stableVerifyDigest(contractPath, corpus.PortableHostContract.SHA256); err != nil {
		return err
	}
	contract, err := Verify(filepath.Dir(contractPath))
	if err != nil {
		return fmt.Errorf("verify stable portable Host contract: %w", err)
	}
	if contract.Lane() != stableLane.APIVersion {
		return fmt.Errorf("generic corpus drives %s, want %s", contract.Lane(), stableLane.APIVersion)
	}
	report, err := SelfTest(ctx, contract)
	if err != nil {
		return fmt.Errorf("stable generic Host lifecycle and constraint matrix: %w", err)
	}
	if report.Status != "passed" || !reflect.DeepEqual(report.Checks, contract.RequiredRunnerChecks) {
		return errors.New("stable generic Host runner returned an incomplete report")
	}
	return nil
}

func stableValidateDesired(definition formpackage.FormDefinition, desired map[string]any) error {
	installed := &InstalledForm{Ref: FormRef{
		APIVersion: definition.APIVersion, Kind: definition.Kind,
		DefinitionVersion: definition.DefinitionVersion,
	}, DesiredSchema: definition.DesiredSchema}
	if err := installed.compileDesiredSchema(); err != nil {
		return err
	}
	if err := installed.compiled.Validate(desired); err != nil {
		return fmt.Errorf("desired fixture violates the exact Form Definition: %w", err)
	}
	return nil
}

func stableVerifyFamily(
	root, corpusPath string,
	record stableSuiteFamilyRecord,
) (StableSuiteFamilyReport, map[string]FormRef, error) {
	var corpus stableFamilyCorpus
	raw, err := stableReadStrict(corpusPath, &corpus)
	if err != nil {
		return StableSuiteFamilyReport{}, nil, err
	}
	if stableDigest(raw) != record.SHA256 || corpus.Format != StableFamilyFormat ||
		corpus.HostAPILane != stableLane.APIVersion || corpus.Group != record.Group ||
		!reflect.DeepEqual(corpus.RequiredChecks, record.RequiredChecks) {
		return StableSuiteFamilyReport{}, nil, fmt.Errorf("stable family corpus %s identity or digest drifted", record.Group)
	}
	if err := validateStableScenarioCoverage(record.RequiredChecks, corpus.Scenarios, "stable family "+record.Group); err != nil {
		return StableSuiteFamilyReport{}, nil, err
	}
	candidatePath, err := stableResolve(root, corpusPath, corpus.CandidateSet.Path)
	if err != nil {
		return StableSuiteFamilyReport{}, nil, err
	}
	candidateRaw, err := stableVerifyDigest(candidatePath, corpus.CandidateSet.SHA256)
	if err != nil {
		return StableSuiteFamilyReport{}, nil, err
	}
	var set stableCandidateSet
	if err := formpackage.DecodeStrictIJSON(candidateRaw, &set); err != nil {
		return StableSuiteFamilyReport{}, nil, fmt.Errorf("decode family candidate set %s: %w", record.Group, err)
	}
	if set.Family != record.Group || len(set.Forms) == 0 || len(corpus.RunnerInput) != len(set.Forms) {
		return StableSuiteFamilyReport{}, nil, fmt.Errorf("stable family %s does not cover its exact candidate roster", record.Group)
	}
	candidates := make(map[string]struct {
		ref     FormRef
		pkg     string
		caps    []string
		desired map[string]any
	}, len(set.Forms))
	for _, candidate := range set.Forms {
		definitionPath := filepath.Join(root, filepath.FromSlash(candidate.Path), "definition.json")
		definitionRaw, err := os.ReadFile(definitionPath)
		if err != nil {
			return StableSuiteFamilyReport{}, nil, err
		}
		definition, err := formpackage.ValidateDefinition(definitionRaw)
		if err != nil {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("validate %s: %w", definitionPath, err)
		}
		digest, err := formpackage.DigestCanonicalJSON(definitionRaw)
		if err != nil {
			return StableSuiteFamilyReport{}, nil, err
		}
		if digest != candidate.FormRef.SchemaDigest || definition.RequiresHostAPI != stableLane.APIVersion {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("candidate %s is not an exact stable Form", candidate.Kind)
		}
		candidates[stableFormRefKey(candidate.FormRef)] = struct {
			ref     FormRef
			pkg     string
			caps    []string
			desired map[string]any
		}{candidate.FormRef, candidate.PackageDigest, definition.LifecycleCapabilities, definition.DesiredSchema}
	}

	runnerRefs := make([]FormRef, 0, len(corpus.RunnerInput))
	live := map[string]map[string]any{}
	for probeName, probe := range corpus.RunnerInput {
		key := stableFormRefKey(probe.Identity.FormRef)
		candidate, known := candidates[key]
		if !known || candidate.pkg != probe.Identity.PackageDigest ||
			!reflect.DeepEqual(candidate.caps, probe.LifecycleCapabilities) {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s probe %s does not pin one exact candidate", record.Group, probeName)
		}
		schemaPath, err := stableResolve(root, corpusPath, probe.DesiredSchema.Path)
		if err != nil {
			return StableSuiteFamilyReport{}, nil, err
		}
		schemaRaw, err := stableVerifyDigest(schemaPath, probe.DesiredSchema.SHA256)
		if err != nil {
			return StableSuiteFamilyReport{}, nil, err
		}
		var pinnedSchema map[string]any
		if err := formpackage.DecodeStrictIJSON(schemaRaw, &pinnedSchema); err != nil {
			return StableSuiteFamilyReport{}, nil, err
		}
		pinned, err := canonicalJSON(pinnedSchema)
		if err != nil {
			return StableSuiteFamilyReport{}, nil, err
		}
		defined, err := canonicalJSON(candidate.desired)
		if err != nil || pinned != defined {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s probe %s desired-schema pin drifted", record.Group, probeName)
		}
		definition := formpackage.FormDefinition{
			APIVersion: probe.Identity.FormRef.APIVersion, Kind: probe.Identity.FormRef.Kind,
			DefinitionVersion: probe.Identity.FormRef.DefinitionVersion, DesiredSchema: candidate.desired,
		}
		if err := stableValidateDesired(definition, probe.Desired); err != nil {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s probe %s: %w", record.Group, probeName, err)
		}
		if _, exists := live[key]; exists {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s exact ref lifecycle repeated %s", record.Group, key)
		}
		live[key] = probe.Desired
		if !reflect.DeepEqual(live[key], probe.Desired) {
			return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s exact ref readback drifted", record.Group)
		}
		delete(live, key)
		runnerRefs = append(runnerRefs, probe.Identity.FormRef)
	}
	if len(live) != 0 {
		return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s lifecycle cleanup left state", record.Group)
	}
	sort.Slice(runnerRefs, func(i, j int) bool { return stableFormRefKey(runnerRefs[i]) < stableFormRefKey(runnerRefs[j]) })
	if len(runnerRefs) != len(candidates) {
		return StableSuiteFamilyReport{}, nil, fmt.Errorf("family %s runner does not cover every exact ref", record.Group)
	}
	if record.Group == "edge.forms.takoform.com" {
		supported, refused := false, false
		for _, fixture := range corpus.StandardServiceFixtures {
			if err := formpackage.ValidateStandardServiceRef(fixture.ServiceRef); err != nil {
				return StableSuiteFamilyReport{}, nil, err
			}
			if fixture.ServiceRef.Protocol == "com.amazonaws.s3" && fixture.Satisfiable {
				supported = true
			}
			if fixture.ServiceRef.Protocol != "com.amazonaws.s3" && !fixture.Satisfiable {
				refused = true
			}
		}
		if !supported || !refused {
			return StableSuiteFamilyReport{}, nil, errors.New("Edge corpus does not observe structured S3 support and unknown-valid refusal")
		}
	}
	return StableSuiteFamilyReport{
		Group: record.Group, Path: record.Path, SHA256: record.SHA256,
		Checks: stableChecks(record.RequiredChecks), RunnerFormRefs: runnerRefs,
	}, candidatesToRefs(candidates), nil
}

func candidatesToRefs(values map[string]struct {
	ref     FormRef
	pkg     string
	caps    []string
	desired map[string]any
}) map[string]FormRef {
	out := make(map[string]FormRef, len(values))
	for key, value := range values {
		out[key] = value.ref
	}
	return out
}

func stableScenarioFormRefs(input map[string]any) ([]FormRef, error) {
	raw, err := json.Marshal(input["formRefs"])
	if err != nil {
		return nil, err
	}
	var refs []FormRef
	if err := formpackage.DecodeStrictIJSON(raw, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func stableVerifyComposition(
	corpusPath string,
	record stableSuiteCorpusRecord,
	familyGroups []string,
	all map[string]FormRef,
) error {
	var corpus stableCompositionCorpus
	raw, err := stableReadStrict(corpusPath, &corpus)
	if err != nil {
		return err
	}
	if stableDigest(raw) != record.SHA256 || corpus.Format != StableCompositionFormat ||
		corpus.HostAPILane != stableLane.APIVersion || !reflect.DeepEqual(corpus.FamilyGroups, familyGroups) ||
		!reflect.DeepEqual(corpus.RequiredChecks, record.RequiredChecks) {
		return errors.New("stable all-family composition corpus identity or digest drifted")
	}
	if err := validateStableScenarioCoverage(record.RequiredChecks, corpus.Scenarios, "stable composition corpus"); err != nil {
		return err
	}
	resolved := map[string]bool{}
	for _, scenario := range corpus.Scenarios {
		groupsRaw, ok := scenario.Input["familyGroups"].([]any)
		if !ok || len(groupsRaw) != len(familyGroups) {
			return fmt.Errorf("composition scenario %s does not carry every family group", scenario.Check)
		}
		for index, rawGroup := range groupsRaw {
			group, ok := rawGroup.(string)
			if !ok || group != familyGroups[index] {
				return fmt.Errorf("composition scenario %s family groups do not match the manifest", scenario.Check)
			}
		}
		refs, err := stableScenarioFormRefs(scenario.Input)
		if err != nil {
			return fmt.Errorf("composition scenario %s: %w", scenario.Check, err)
		}
		if scenario.Check == "all-family-composition-resolves" {
			for _, ref := range refs {
				key := stableFormRefKey(ref)
				if _, known := all[key]; !known {
					return fmt.Errorf("composition could not resolve exact FormRef %s", key)
				}
				resolved[key] = true
			}
		}
		if scenario.Check == "wrong-digest-refused" {
			for _, ref := range refs {
				if _, known := all[stableFormRefKey(ref)]; known {
					return errors.New("composition accepted a wrong-digest FormRef")
				}
			}
		}
	}
	if len(resolved) != len(all) {
		return fmt.Errorf("composition resolved %d/%d exact current Forms", len(resolved), len(all))
	}
	return nil
}

// RunStableSuite executes the manifest-owned stable v1 reference suite and
// returns the exact data-only report consumed by the publication verifier.
func RunStableSuite(ctx context.Context, manifestPath string) (StableSuiteReport, error) {
	absoluteManifest, err := filepath.Abs(manifestPath)
	if err != nil {
		return StableSuiteReport{}, err
	}
	var manifest StableSuiteManifest
	manifestRaw, err := stableReadStrict(absoluteManifest, &manifest)
	if err != nil {
		return StableSuiteReport{}, err
	}
	if manifest.Format != StableSuiteFormat || manifest.HostAPILane != stableLane.APIVersion ||
		manifest.Runner.ReportFormat != StableSuiteReportFormat || len(manifest.Families) == 0 {
		return StableSuiteReport{}, errors.New("stable suite manifest identity is invalid")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(absoluteManifest), "..", ".."))
	genericPath, err := stableResolve(root, absoluteManifest, manifest.Generic.Path)
	if err != nil {
		return StableSuiteReport{}, err
	}
	if _, err := stableVerifyDigest(genericPath, manifest.Generic.SHA256); err != nil {
		return StableSuiteReport{}, err
	}
	if err := stableVerifyGeneric(ctx, root, genericPath, manifest.Generic); err != nil {
		return StableSuiteReport{}, err
	}

	result := StableSuiteReport{
		Format: StableSuiteReportFormat, Status: "passed", HostAPILane: stableLane.APIVersion,
		Suite: stableDigestRecord{Path: "conformance/takoform-v1/manifest.json", SHA256: stableDigest(manifestRaw)},
		Generic: StableSuiteCorpusReport{
			Path: manifest.Generic.Path, SHA256: manifest.Generic.SHA256,
			Checks: stableChecks(manifest.Generic.RequiredChecks),
		},
	}
	all := map[string]FormRef{}
	familyGroups := make([]string, 0, len(manifest.Families))
	for index, record := range manifest.Families {
		if index > 0 && manifest.Families[index-1].Group >= record.Group {
			return StableSuiteReport{}, errors.New("stable suite families are not sorted and unique")
		}
		familyPath, err := stableResolve(root, absoluteManifest, record.Path)
		if err != nil {
			return StableSuiteReport{}, err
		}
		if _, err := stableVerifyDigest(familyPath, record.SHA256); err != nil {
			return StableSuiteReport{}, err
		}
		familyReport, refs, err := stableVerifyFamily(root, familyPath, record)
		if err != nil {
			return StableSuiteReport{}, err
		}
		for key, ref := range refs {
			if _, repeated := all[key]; repeated {
				return StableSuiteReport{}, fmt.Errorf("stable suite repeats exact FormRef %s", key)
			}
			all[key] = ref
		}
		result.Families = append(result.Families, familyReport)
		familyGroups = append(familyGroups, record.Group)
	}
	compositionPath, err := stableResolve(root, absoluteManifest, manifest.Composition.Path)
	if err != nil {
		return StableSuiteReport{}, err
	}
	if _, err := stableVerifyDigest(compositionPath, manifest.Composition.SHA256); err != nil {
		return StableSuiteReport{}, err
	}
	if err := stableVerifyComposition(compositionPath, manifest.Composition, familyGroups, all); err != nil {
		return StableSuiteReport{}, err
	}
	result.Composition = StableSuiteCorpusReport{
		Path: manifest.Composition.Path, SHA256: manifest.Composition.SHA256,
		Checks: stableChecks(manifest.Composition.RequiredChecks),
	}
	return result, nil
}
