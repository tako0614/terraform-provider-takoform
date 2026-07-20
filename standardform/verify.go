package standardform

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

var stableCode = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,127}$`)
var forbiddenEvidenceKeys = map[string]struct{}{
	"credential": {}, "credentials": {}, "secret": {}, "password": {}, "token": {},
	"provider": {}, "providers": {}, "target": {}, "targets": {}, "manager": {}, "capacity": {},
	"price": {}, "pricing": {}, "sku": {}, "billing": {}, "quota": {}, "sla": {},
	"command": {}, "script": {}, "executable": {}, "binary": {}, "code": {},
}

// ValidateEvidenceStructure validates provider-neutral admission evidence against an
// already data-only-verified package. A caller must separately authenticate
// the package-adjacent evidence and both runner reports; this parser never
// synthesizes or upgrades structural checks into passed lifecycle proof.
func ValidateEvidenceStructure(path string, report formpackage.VerificationReport, definition formpackage.FormDefinition) (AdmissionEvidence, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AdmissionEvidence{}, err
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence: %w", err)
	}
	var untyped any
	if err := json.Unmarshal(raw, &untyped); err != nil {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence: %w", err)
	}
	if err := rejectForbiddenEvidenceKeys(untyped, "$"); err != nil {
		return AdmissionEvidence{}, err
	}
	var evidence AdmissionEvidence
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence: %w", err)
	}
	wantIdentity := InstalledFormReference{FormRef: report.FormRef, PackageDigest: report.PackageDigest}
	if evidence.APIVersion != APIVersion || evidence.Classification != "portable-standard" ||
		!reflect.DeepEqual(evidence.Identity, wantIdentity) || evidence.ApprovedSchemaDigest != report.FormRef.SchemaDigest {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence exact identity or classification mismatch")
	}
	if definition.Status != "standard" || definition.DefinitionVersion != report.FormRef.DefinitionVersion {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence requires the exact standard definition version")
	}
	wantLifecycle := []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"}
	if !sameSet(definition.LifecycleCapabilities, wantLifecycle) ||
		!evidence.Audit.Lifecycle.Create || !evidence.Audit.Lifecycle.Read || !evidence.Audit.Lifecycle.Update ||
		!evidence.Audit.Lifecycle.Delete || !evidence.Audit.Lifecycle.Import || !evidence.Audit.Lifecycle.Observe ||
		!evidence.Audit.Lifecycle.Refresh || !evidence.Audit.Lifecycle.Drift {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence lacks the complete portable lifecycle")
	}
	if !evidence.Audit.Immutability.Reviewed || !sameSet(evidence.Audit.Immutability.Fields, definition.ImmutableFields) {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence immutability audit mismatch")
	}
	if !evidence.Audit.Security.SecretFreeDesiredState || !evidence.Audit.Security.CredentialBoundaryExternal || !evidence.Audit.Security.DataOnlyPackage ||
		!evidence.Audit.Interfaces.Reviewed || !evidence.Audit.Interfaces.BindingAuthorityExternal || !evidence.Audit.Interfaces.SecretFreeDocuments {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence security or Interface audit is incomplete")
	}
	positiveNames := namesOfPositive(evidence.Fixtures.Positive)
	negativeNames := namesOfNegative(evidence.Fixtures.Negative)
	if len(positiveNames) == 0 || len(negativeNames) == 0 || len(positiveNames) != len(evidence.Fixtures.Positive) || len(negativeNames) != len(evidence.Fixtures.Negative) {
		return AdmissionEvidence{}, fmt.Errorf("admission evidence fixture closure is empty or duplicated")
	}
	for _, fixture := range evidence.Fixtures.Negative {
		if !validPortableNegativeErrorCode(fixture.ExpectedErrorCode) {
			return AdmissionEvidence{}, fmt.Errorf("negative fixture %q must use portable wire error code %q", fixture.Name, InvalidArgumentErrorCode)
		}
	}
	if err := verifyProof("host", evidence.Conformance.Host, wantIdentity, positiveNames, negativeNames); err != nil {
		return AdmissionEvidence{}, err
	}
	if err := verifyProof("provider", evidence.Conformance.Provider, wantIdentity, positiveNames, negativeNames); err != nil {
		return AdmissionEvidence{}, err
	}
	return evidence, nil
}

func validPortableNegativeErrorCode(value string) bool {
	return stableCode.MatchString(value) && value == InvalidArgumentErrorCode
}

func rejectForbiddenEvidenceKeys(value any, location string) error {
	switch typed := value.(type) {
	case []any:
		for index, child := range typed {
			if err := rejectForbiddenEvidenceKeys(child, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, child := range typed {
			_, forbidden := forbiddenEvidenceKeys[strings.ToLower(key)]
			if forbidden && !(location == "$.conformance" && key == "provider") {
				return fmt.Errorf("forbidden standard-admission field %q at %s", key, location)
			}
			if err := rejectForbiddenEvidenceKeys(child, location+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyProof(label string, proof ConformanceProof, identity InstalledFormReference, positives, negatives []string) error {
	if proof.Status != "passed" || strings.TrimSpace(proof.Subject) == "" || strings.TrimSpace(proof.RunnerVersion) == "" || !reflect.DeepEqual(proof.Identity, identity) ||
		!sameSet(proof.PositiveFixtures, positives) || !sameSet(proof.NegativeFixtures, negatives) || !formpackage.ValidDigest(proof.EvidenceDigest) {
		return fmt.Errorf("%s conformance proof is invalid or lacks exact fixture coverage", label)
	}
	return nil
}

func namesOfPositive(fixtures []PositiveFixture) []string {
	names := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		names = append(names, fixture.Name)
	}
	return names
}

func namesOfNegative(fixtures []NegativeFixture) []string {
	names := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		names = append(names, fixture.Name)
	}
	return names
}

func sameSet(left, right []string) bool {
	a, b := sortedCopy(left), sortedCopy(right)
	return reflect.DeepEqual(a, b)
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
