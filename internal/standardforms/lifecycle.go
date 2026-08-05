package standardforms

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	projectLifecyclePath       = "forms/lifecycle.json"
	projectLifecycleSchemaPath = "forms/lifecycle.schema.json"
	projectLifecycleSchemaID   = "https://forms.takoform.invalid/authoring/lifecycle.schema.json"
	maxLifecycleSchemaBytes    = 1 << 20
)

type closedLifecycleSchemaLoader struct{}

func (closedLifecycleSchemaLoader) Load(resourceURL string) (any, error) {
	return nil, fmt.Errorf("schema resource %q is outside the repository-local lifecycle schema closure", resourceURL)
}

type projectLifecycleAuthority struct {
	Format        string   `json:"format"`
	ProjectStatus string   `json:"projectStatus"`
	CurrentEpoch  string   `json:"currentEpoch"`
	States        []string `json:"states"`
	Legacy        struct {
		APIVersion             string `json:"apiVersion"`
		Decision               string `json:"decision"`
		EpochDecision          string `json:"epochDecision"`
		ReleaseSources         string `json:"releaseSources"`
		ReleaseSourceInventory *struct {
			Format string `json:"format"`
			Count  int    `json:"count"`
			Digest string `json:"digest"`
		} `json:"releaseSourceInventory"`
		HistoricalAdmissionRoots []string `json:"historicalAdmissionRoots"`
		NewCreatePolicy          string   `json:"newCreatePolicy"`
		RetainedCapabilities     []string `json:"retainedCapabilities"`
	} `json:"legacy"`
	CurrentForms []formLifecycleRecord `json:"currentForms"`
	Proposals    []formProposalRecord  `json:"proposals"`
}

type formProposalRecord struct {
	ID                     string                   `json:"id"`
	Document               string                   `json:"document"`
	Owner                  string                   `json:"owner"`
	Consumer               string                   `json:"consumer"`
	IntendedHosts          []string                 `json:"intendedHosts"`
	Workload               string                   `json:"workload"`
	PortableBoundary       string                   `json:"portableBoundary"`
	PortableFields         []string                 `json:"portableFields"`
	HostDecisions          []string                 `json:"hostDecisions"`
	LifecycleRisks         proposalLifecycleRisks   `json:"lifecycleRisks"`
	SecurityBoundary       proposalSecurityBoundary `json:"securityBoundary"`
	PriorArt               []proposalPriorArt       `json:"priorArt"`
	ExistingAbstractionGap string                   `json:"existingAbstractionGap"`
}

type proposalLifecycleRisks struct {
	Replacement string `json:"replacement"`
	DataLoss    string `json:"dataLoss"`
	Delete      string `json:"delete"`
	Import      string `json:"import"`
	Drift       string `json:"drift"`
}

type proposalSecurityBoundary struct {
	Credentials string `json:"credentials"`
	Network     string `json:"network"`
	Artifacts   string `json:"artifacts"`
	Secrets     string `json:"secrets"`
}

type proposalPriorArt struct {
	Name          string `json:"name"`
	Applicability string `json:"applicability"`
	Finding       string `json:"finding"`
}

type formLifecycleRecord struct {
	ProposalID    string                    `json:"proposalId"`
	State         string                    `json:"state"`
	Owner         string                    `json:"owner"`
	FormRef       formpackage.FormRef       `json:"formRef"`
	PackageDigest string                    `json:"packageDigest"`
	PackagePath   string                    `json:"packagePath"`
	History       []formLifecycleTransition `json:"history"`
	Evidence      formLifecycleEvidence     `json:"evidence"`
	Legacy        *formLegacyDisposition    `json:"legacy,omitempty"`
}

type formLifecycleTransition struct {
	State    string `json:"state"`
	Decision string `json:"decision"`
}

type lifecycleEvidenceRef struct {
	Subject    string `json:"subject"`
	Maintainer string `json:"maintainer,omitempty"`
	Evidence   string `json:"evidence"`
}

type formLifecycleEvidence struct {
	Definition                 string                 `json:"definition"`
	Fixtures                   string                 `json:"fixtures"`
	HostImplementations        []lifecycleEvidenceRef `json:"hostImplementations"`
	RealConsumers              []lifecycleEvidenceRef `json:"realConsumers"`
	KnownLimitations           string                 `json:"knownLimitations"`
	Compatibility              string                 `json:"compatibility"`
	Migration                  string                 `json:"migration"`
	SecurityReview             string                 `json:"securityReview"`
	Documentation              string                 `json:"documentation"`
	PublicationPlan            string                 `json:"publicationPlan"`
	LifecycleAgreement         string                 `json:"lifecycleAgreement,omitempty"`
	Interoperability           string                 `json:"interoperability,omitempty"`
	IndependentPublication     string                 `json:"independentPublication,omitempty"`
	DeprecationExercise        string                 `json:"deprecationExercise,omitempty"`
	SecurityRevocationExercise string                 `json:"securityRevocationExercise,omitempty"`
}

type formLegacyDisposition struct {
	Reason                 string   `json:"reason"`
	SuccessorOrAlternative string   `json:"successorOrAlternative"`
	NewCreatePolicy        string   `json:"newCreatePolicy"`
	RetainedCapabilities   []string `json:"retainedCapabilities"`
	Migration              string   `json:"migration"`
}

func readProjectLifecycleAuthority(root string) (projectLifecycleAuthority, error) {
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	info, err := os.Stat(path)
	if err != nil {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority: %w", err)
	}
	if !info.Mode().IsRegular() {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority is not a regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority: %w", err)
	}
	var authority projectLifecycleAuthority
	if err := formpackage.DecodeStrictIJSON(raw, &authority); err != nil {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority: %w", err)
	}
	if authority.Format != "takoform.form-lifecycle@v2" {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority format is %q, want takoform.form-lifecycle@v2", authority.Format)
	}
	if authority.ProjectStatus != "experimental" {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority projectStatus is %q, want experimental", authority.ProjectStatus)
	}
	if authority.CurrentEpoch != formpackage.CurrentFormAPIVersion {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority currentEpoch is %q, want %s", authority.CurrentEpoch, formpackage.CurrentFormAPIVersion)
	}
	wantStates := []string{"proposal", "experimental", "stable", "legacy"}
	if !slices.Equal(authority.States, wantStates) {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority states must be exactly proposal, experimental, stable, legacy")
	}
	if authority.Legacy.ReleaseSourceInventory == nil {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority legacy release source inventory pin is required")
	}
	if authority.Legacy.Decision != "spec/decisions/0004-takoform-is-an-experimental-specification.md" {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority legacy decision must be spec/decisions/0004-takoform-is-an-experimental-specification.md")
	}
	if authority.Legacy.APIVersion != formpackage.LegacyFormAPIVersion {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority legacy apiVersion is %q, want %s", authority.Legacy.APIVersion, formpackage.LegacyFormAPIVersion)
	}
	if authority.Legacy.EpochDecision != "spec/decisions/0006-v1alpha2-restarts-form-lines.md" {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority legacy epochDecision must be spec/decisions/0006-v1alpha2-restarts-form-lines.md")
	}
	wantAdmissionRoots := []string{"admission/v1", "admission/v3", "admission/v4"}
	if !slices.Equal(authority.Legacy.HistoricalAdmissionRoots, wantAdmissionRoots) {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority historicalAdmissionRoots must be exactly admission/v1, admission/v3, admission/v4")
	}
	if authority.Legacy.NewCreatePolicy != "host-policy" {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority legacy newCreatePolicy is %q, want host-policy", authority.Legacy.NewCreatePolicy)
	}
	wantRetainedCapabilities := []string{"read", "observe", "delete", "recovery", "migration"}
	if !slices.Equal(authority.Legacy.RetainedCapabilities, wantRetainedCapabilities) {
		return projectLifecycleAuthority{}, fmt.Errorf("project lifecycle authority legacy retainedCapabilities must be exactly read, observe, delete, recovery, migration")
	}
	if err := validateLifecycleRecords(root, authority); err != nil {
		return projectLifecycleAuthority{}, err
	}
	if err := validateProjectLifecycleSchema(root, raw); err != nil {
		return projectLifecycleAuthority{}, err
	}
	centralAdmissionPath := filepath.Join(root, "forms", "admission-candidate-set.json")
	if _, err := os.Lstat(centralAdmissionPath); err == nil {
		return projectLifecycleAuthority{}, fmt.Errorf("central admission candidate authority must not exist: forms/admission-candidate-set.json")
	} else if !os.IsNotExist(err) {
		return projectLifecycleAuthority{}, fmt.Errorf("inspect central admission candidate authority: %w", err)
	}
	return authority, nil
}

func validateProjectLifecycleSchema(root string, lifecycleRaw []byte) error {
	if err := requireLifecycleFile(root, projectLifecycleSchemaPath, "forms/"); err != nil {
		return fmt.Errorf("project lifecycle authoring schema: %w", err)
	}
	schemaPath := filepath.Join(root, filepath.FromSlash(projectLifecycleSchemaPath))
	info, err := os.Lstat(schemaPath)
	if err != nil {
		return fmt.Errorf("project lifecycle authoring schema: %w", err)
	}
	if info.Size() > maxLifecycleSchemaBytes {
		return fmt.Errorf("project lifecycle authoring schema is %d bytes, limit is %d", info.Size(), maxLifecycleSchemaBytes)
	}
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("project lifecycle authoring schema: %w", err)
	}
	if _, err := formpackage.Canonicalize(schemaRaw); err != nil {
		return fmt.Errorf("project lifecycle authoring schema is not I-JSON: %w", err)
	}
	schemaValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaRaw))
	if err != nil {
		return fmt.Errorf("project lifecycle authoring schema decode: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(closedLifecycleSchemaLoader{})
	if err := compiler.AddResource(projectLifecycleSchemaID, schemaValue); err != nil {
		return fmt.Errorf("project lifecycle authoring schema registration: %w", err)
	}
	compiled, err := compiler.Compile(projectLifecycleSchemaID)
	if err != nil {
		return fmt.Errorf("project lifecycle authoring schema compilation: %w", err)
	}
	lifecycleValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(lifecycleRaw))
	if err != nil {
		return fmt.Errorf("project lifecycle authority schema input: %w", err)
	}
	if err := compiled.Validate(lifecycleValue); err != nil {
		return fmt.Errorf("project lifecycle authority does not satisfy its authoring schema: %w", err)
	}
	return nil
}

func validateLifecycleRecords(root string, authority projectLifecycleAuthority) error {
	proposals := make(map[string]formProposalRecord, len(authority.Proposals))
	for index, proposal := range authority.Proposals {
		prefix := fmt.Sprintf("project lifecycle authority proposals[%d]", index)
		if proposal.ID == "" || proposal.Owner == "" || proposal.Consumer == "" ||
			proposal.Workload == "" || proposal.PortableBoundary == "" ||
			proposal.ExistingAbstractionGap == "" {
			return fmt.Errorf("%s omits required identity, ownership, consumer, workload, boundary, or abstraction-gap material", prefix)
		}
		if _, duplicate := proposals[proposal.ID]; duplicate {
			return fmt.Errorf("%s duplicates proposal id %q", prefix, proposal.ID)
		}
		if err := requireLifecycleFile(root, proposal.Document, "proposals/"); err != nil {
			return fmt.Errorf("%s document: %w", prefix, err)
		}
		if err := requireNonEmptyUniqueStrings(proposal.IntendedHosts); err != nil {
			return fmt.Errorf("%s intendedHosts: %w", prefix, err)
		}
		if err := requireNonEmptyUniqueStrings(proposal.HostDecisions); err != nil {
			return fmt.Errorf("%s hostDecisions: %w", prefix, err)
		}
		if err := requireNonEmptyUniqueStrings(proposal.PortableFields); err != nil {
			return fmt.Errorf("%s portableFields: %w", prefix, err)
		}
		for label, value := range map[string]string{
			"replacement": proposal.LifecycleRisks.Replacement,
			"dataLoss":    proposal.LifecycleRisks.DataLoss,
			"delete":      proposal.LifecycleRisks.Delete,
			"import":      proposal.LifecycleRisks.Import,
			"drift":       proposal.LifecycleRisks.Drift,
			"credentials": proposal.SecurityBoundary.Credentials,
			"network":     proposal.SecurityBoundary.Network,
			"artifacts":   proposal.SecurityBoundary.Artifacts,
			"secrets":     proposal.SecurityBoundary.Secrets,
		} {
			if value == "" {
				return fmt.Errorf("%s omits %s analysis", prefix, label)
			}
		}
		if err := validateProposalPriorArt(proposal.PriorArt); err != nil {
			return fmt.Errorf("%s priorArt: %w", prefix, err)
		}
		proposals[proposal.ID] = proposal
	}

	seenForms := make(map[string]struct{}, len(authority.CurrentForms))
	for index, record := range authority.CurrentForms {
		prefix := fmt.Sprintf("project lifecycle authority currentForms[%d]", index)
		proposal, ok := proposals[record.ProposalID]
		if !ok {
			return fmt.Errorf("%s references unknown proposalId %q", prefix, record.ProposalID)
		}
		if record.Owner == "" || record.Owner != proposal.Owner {
			return fmt.Errorf("%s owner must equal proposal owner", prefix)
		}
		if record.State != "experimental" && record.State != "stable" && record.State != "legacy" {
			return fmt.Errorf("%s state %q is not experimental, stable, or legacy", prefix, record.State)
		}
		if record.FormRef.APIVersion != formpackage.CurrentFormAPIVersion || record.FormRef.Kind == "" ||
			!formpackage.ValidDigest(record.FormRef.SchemaDigest) || !formpackage.ValidDigest(record.PackageDigest) {
			return fmt.Errorf("%s has an invalid exact FormRef or package digest", prefix)
		}
		key := record.FormRef.Kind + "@" + record.FormRef.DefinitionVersion
		if _, duplicate := seenForms[key]; duplicate {
			return fmt.Errorf("%s duplicates exact Form identity %s", prefix, key)
		}
		seenForms[key] = struct{}{}
		version, err := parseStableFormVersion(record.FormRef.DefinitionVersion)
		if err != nil {
			return fmt.Errorf("%s definitionVersion: %w", prefix, err)
		}
		if record.State == "experimental" && version.Major != 0 {
			return fmt.Errorf("%s Experimental Form must use a 0.x definitionVersion", prefix)
		}
		if record.State == "stable" && version.Major == 0 {
			return fmt.Errorf("%s Stable Form must use a 1.x or later definitionVersion", prefix)
		}
		if err := validateLifecycleHistory(root, prefix, record.State, record.History); err != nil {
			return err
		}
		if err := verifyLifecyclePackage(root, prefix, record); err != nil {
			return err
		}
		if err := validateLifecycleEvidence(root, prefix, record); err != nil {
			return err
		}
	}
	return nil
}

func validateProposalPriorArt(entries []proposalPriorArt) error {
	want := []string{"OCCI", "CIMI", "TOSCA", "Kubernetes/Crossplane", "Terraform/OpenTofu"}
	if len(entries) != len(want) {
		return fmt.Errorf("must review exactly OCCI, CIMI, TOSCA, Kubernetes/Crossplane, and Terraform/OpenTofu")
	}
	for index, name := range want {
		entry := entries[index]
		if entry.Name != name || (entry.Applicability != "applicable" && entry.Applicability != "not-applicable") || entry.Finding == "" {
			return fmt.Errorf("entry %d must be the ordered %s review with applicability and a finding", index, name)
		}
	}
	return nil
}

func validateLifecycleHistory(root, prefix, state string, history []formLifecycleTransition) error {
	want := []string{"proposal", "experimental"}
	if state == "stable" {
		want = append(want, "stable")
	} else if state == "legacy" {
		if len(history) == 3 {
			want = append(want, "legacy")
		} else {
			want = append(want, "stable", "legacy")
		}
	}
	if len(history) != len(want) {
		return fmt.Errorf("%s history must be exactly %s", prefix, strings.Join(want, ", "))
	}
	for index, state := range want {
		if history[index].State != state {
			return fmt.Errorf("%s history must be exactly %s", prefix, strings.Join(want, ", "))
		}
		if err := requireLifecycleFile(root, history[index].Decision, ""); err != nil {
			return fmt.Errorf("%s history[%d] decision: %w", prefix, index, err)
		}
	}
	return nil
}

func verifyLifecyclePackage(root, prefix string, record formLifecycleRecord) error {
	if err := requireLifecycleDirectory(root, record.PackagePath, "forms/releases/"); err != nil {
		return fmt.Errorf("%s packagePath: %w", prefix, err)
	}
	packageRoot := filepath.Join(root, filepath.FromSlash(record.PackagePath))
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return fmt.Errorf("%s package: %w", prefix, err)
	}
	if report.FormRef != record.FormRef || report.PackageDigest != record.PackageDigest {
		return fmt.Errorf("%s exact package bytes drift from its lifecycle identity", prefix)
	}
	indexRaw, err := os.ReadFile(filepath.Join(packageRoot, formpackage.PackageIndexFilename))
	if err != nil {
		return fmt.Errorf("%s package index: %w", prefix, err)
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return fmt.Errorf("%s package index: %w", prefix, err)
	}
	if index.APIVersion != formpackage.CurrentPackageAPIVersion {
		return fmt.Errorf("%s current Form package must use %s", prefix, formpackage.CurrentPackageAPIVersion)
	}
	locator, err := formpackage.PublicationLocatorFor(index, report.PackageDigest)
	if err != nil {
		return fmt.Errorf("%s package publication identity: %w", prefix, err)
	}
	if record.PackagePath != locator.SourcePath {
		return fmt.Errorf("%s packagePath must be canonical %s", prefix, locator.SourcePath)
	}
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(index.DefinitionPath)))
	if err != nil {
		return fmt.Errorf("%s package definition: %w", prefix, err)
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return fmt.Errorf("%s package definition: %w", prefix, err)
	}
	if len(definition.ConformanceFixtures) == 0 || len(definition.NegativeFixtures) == 0 {
		return fmt.Errorf("%s Experimental-or-later package must retain positive and negative conformance fixtures", prefix)
	}
	wantDefinitionEvidence := filepath.ToSlash(filepath.Join(record.PackagePath, index.DefinitionPath))
	if record.Evidence.Definition != wantDefinitionEvidence {
		return fmt.Errorf("%s evidence.definition must be the package's exact canonical definition %s", prefix, wantDefinitionEvidence)
	}
	return nil
}

func requireLifecycleDirectory(root, relativePath, prefix string) error {
	if err := validateLifecyclePath(relativePath, prefix); err != nil {
		return err
	}
	info, err := inspectLifecyclePath(root, relativePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s must be one directory and not a symlink", relativePath)
	}
	return nil
}

func validateLifecycleEvidence(root, prefix string, record formLifecycleRecord) error {
	paths := map[string]string{
		"definition":       record.Evidence.Definition,
		"fixtures":         record.Evidence.Fixtures,
		"knownLimitations": record.Evidence.KnownLimitations,
		"compatibility":    record.Evidence.Compatibility,
		"migration":        record.Evidence.Migration,
		"securityReview":   record.Evidence.SecurityReview,
		"documentation":    record.Evidence.Documentation,
		"publicationPlan":  record.Evidence.PublicationPlan,
	}
	for label, value := range paths {
		if err := requireLifecycleFile(root, value, ""); err != nil {
			return fmt.Errorf("%s evidence.%s: %w", prefix, label, err)
		}
	}
	if err := validateEvidenceRefs(root, record.Evidence.HostImplementations, true); err != nil {
		return fmt.Errorf("%s evidence.hostImplementations: %w", prefix, err)
	}
	if err := validateEvidenceRefs(root, record.Evidence.RealConsumers, false); err != nil {
		return fmt.Errorf("%s evidence.realConsumers: %w", prefix, err)
	}
	if record.State == "stable" {
		if len(record.Evidence.HostImplementations) < 2 {
			return fmt.Errorf("%s Stable Form needs at least two host implementations", prefix)
		}
		hosts := make(map[string]struct{}, len(record.Evidence.HostImplementations))
		maintainers := make(map[string]struct{}, len(record.Evidence.HostImplementations))
		for _, implementation := range record.Evidence.HostImplementations {
			hosts[implementation.Subject] = struct{}{}
			maintainers[implementation.Maintainer] = struct{}{}
		}
		if len(hosts) < 2 || len(maintainers) < 2 {
			return fmt.Errorf("%s Stable Form needs two distinct hosts and independently named maintainers", prefix)
		}
		for label, value := range map[string]string{
			"lifecycleAgreement":         record.Evidence.LifecycleAgreement,
			"interoperability":           record.Evidence.Interoperability,
			"independentPublication":     record.Evidence.IndependentPublication,
			"deprecationExercise":        record.Evidence.DeprecationExercise,
			"securityRevocationExercise": record.Evidence.SecurityRevocationExercise,
		} {
			if err := requireLifecycleFile(root, value, ""); err != nil {
				return fmt.Errorf("%s evidence.%s: %w", prefix, label, err)
			}
		}
	}
	if record.State == "legacy" {
		if record.Legacy == nil || record.Legacy.Reason == "" || record.Legacy.SuccessorOrAlternative == "" ||
			record.Legacy.NewCreatePolicy != "host-policy" ||
			!slices.Equal(record.Legacy.RetainedCapabilities, []string{"read", "observe", "delete", "recovery", "migration"}) {
			return fmt.Errorf("%s Legacy Form needs reason, successor or alternative, host-policy new creates, and retained recovery capabilities", prefix)
		}
		if err := requireLifecycleFile(root, record.Legacy.Migration, ""); err != nil {
			return fmt.Errorf("%s legacy migration: %w", prefix, err)
		}
	} else if record.Legacy != nil {
		return fmt.Errorf("%s non-Legacy Form must not carry a legacy disposition", prefix)
	}
	return nil
}

func validateEvidenceRefs(root string, entries []lifecycleEvidenceRef, requireMaintainer bool) error {
	if len(entries) == 0 {
		return fmt.Errorf("must contain at least one evidence reference")
	}
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if entry.Subject == "" || (requireMaintainer && entry.Maintainer == "") {
			return fmt.Errorf("entry %d omits subject or maintainer", index)
		}
		if _, duplicate := seen[entry.Subject]; duplicate {
			return fmt.Errorf("entry %d duplicates subject %q", index, entry.Subject)
		}
		seen[entry.Subject] = struct{}{}
		if err := requireLifecycleFile(root, entry.Evidence, ""); err != nil {
			return fmt.Errorf("entry %d evidence: %w", index, err)
		}
	}
	return nil
}

func requireNonEmptyUniqueStrings(values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("must contain at least one value")
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value == "" {
			return fmt.Errorf("entry %d is empty", index)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("entry %d duplicates %q", index, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func requireLifecycleFile(root, relativePath, prefix string) error {
	if err := validateLifecyclePath(relativePath, prefix); err != nil {
		return err
	}
	info, err := inspectLifecyclePath(root, relativePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be one regular non-symlink file", relativePath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s must not be empty", relativePath)
	}
	return nil
}

func inspectLifecyclePath(root, relativePath string) (os.FileInfo, error) {
	current := root
	parts := strings.Split(relativePath, "/")
	for index, part := range parts {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s traverses symlink component %s", relativePath, strings.Join(parts[:index+1], "/"))
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("%s traverses non-directory component %s", relativePath, strings.Join(parts[:index+1], "/"))
		}
		if index == len(parts)-1 {
			return info, nil
		}
	}
	return nil, fmt.Errorf("path %q has no components", relativePath)
}

func validateLifecyclePath(relativePath, prefix string) error {
	if relativePath == "" || filepath.IsAbs(relativePath) || filepath.ToSlash(filepath.Clean(relativePath)) != relativePath ||
		relativePath == "." || strings.HasPrefix(relativePath, "../") || (prefix != "" && !strings.HasPrefix(relativePath, prefix)) {
		return fmt.Errorf("path %q is not an allowed repository-relative path", relativePath)
	}
	return nil
}

func verifyLegacyReleaseInventory(root string, authority projectLifecycleAuthority, published map[string]publishedReleaseSource) error {
	pin := authority.Legacy.ReleaseSourceInventory
	if pin.Format != "takoform.legacy-release-inventory@v1" {
		return fmt.Errorf("project lifecycle authority legacy release source inventory format is %q", pin.Format)
	}
	legacyPublished := make(map[string]publishedReleaseSource, len(published))
	for key, source := range published {
		if source.AdmissionGeneration != "" {
			legacyPublished[key] = source
		}
	}
	current, err := currentLifecycleReleaseIdentities(root, authority)
	if err != nil {
		return fmt.Errorf("current lifecycle release identities: %w", err)
	}
	currentSources := make(map[string]struct{}, len(current))
	legacyTags := make(map[string]struct{}, len(legacyPublished))
	for _, source := range legacyPublished {
		legacyTags[source.Tag] = struct{}{}
	}
	for _, identity := range current {
		if _, collision := legacyTags[identity.Tag]; collision {
			return fmt.Errorf("current lifecycle identity %s reuses immutable Legacy tag %s", identity.Kind, identity.Tag)
		}
		currentSources[identity.SourcePath] = struct{}{}
	}
	keys := make([]string, 0, len(legacyPublished))
	for key := range legacyPublished {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var inventory strings.Builder
	for _, key := range keys {
		source := legacyPublished[key]
		fmt.Fprintf(
			&inventory,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			source.ReleaseID,
			source.ArtifactID,
			source.FormRef.APIVersion,
			source.FormRef.Kind,
			source.FormRef.DefinitionVersion,
			source.FormRef.SchemaDigest,
			source.PackageDigest,
			source.SourcePath,
		)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(inventory.String())))
	if pin.Count != len(legacyPublished) {
		return fmt.Errorf("project lifecycle authority legacy release source inventory count is %d, discovered %d", pin.Count, len(legacyPublished))
	}
	if pin.Digest != digest {
		return fmt.Errorf("project lifecycle authority legacy release source inventory digest drift: got %s, want %s", digest, pin.Digest)
	}
	if filepath.Clean(filepath.FromSlash(authority.Legacy.ReleaseSources)) != filepath.FromSlash("forms/releases") || authority.Legacy.ReleaseSources != "forms/releases" {
		return fmt.Errorf("project lifecycle authority legacy releaseSources is %q, want forms/releases", authority.Legacy.ReleaseSources)
	}
	releaseRoot := filepath.Join(root, filepath.FromSlash(authority.Legacy.ReleaseSources))
	families, err := os.ReadDir(releaseRoot)
	if err != nil {
		return fmt.Errorf("read lifecycle release sources: %w", err)
	}
	for _, family := range families {
		if !family.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(releaseRoot, family.Name()))
		if err != nil {
			return fmt.Errorf("read lifecycle release family %s: %w", family.Name(), err)
		}
		for _, version := range versions {
			if !version.IsDir() {
				return fmt.Errorf("release source family %s contains non-directory %s", family.Name(), version.Name())
			}
			relativeSource := filepath.ToSlash(filepath.Join(authority.Legacy.ReleaseSources, family.Name(), version.Name()))
			_, legacy := legacyPublished[publishedReleaseKey(family.Name(), version.Name())]
			_, current := currentSources[relativeSource]
			if !legacy && !current {
				return fmt.Errorf("release source is outside both Legacy inventory and current lifecycle authority: %s/%s", family.Name(), version.Name())
			}
		}
	}
	return nil
}
