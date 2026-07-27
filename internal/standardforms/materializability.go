package standardforms

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

var unsafeFixtureString = regexp.MustCompile(`(?i)(?:example\.(?:com|org|net)|\.test(?:/|$)|localhost|127\.0\.0\.1)`)

// VerifyMaterializableCandidate proves that every declared Form ships a
// canonical fixture a host could actually attempt, and that its declared
// runtime interfaces are exactly the reviewed portable descriptors.
//
// It deliberately proves no more than that. Whether a host can really realize
// these Forms — and whether executable bytes exist behind an artifact
// reference — is external evidence this repository does not synthesize.
func VerifyMaterializableCandidate(root string) error {
	if len(Specs) == 0 {
		return fmt.Errorf("the portable Form set is empty")
	}
	seen := make(map[string]struct{}, len(Specs))
	for _, spec := range Specs {
		if _, duplicate := seen[spec.Kind]; duplicate {
			return fmt.Errorf("materializable candidate duplicates %s", spec.Kind)
		}
		seen[spec.Kind] = struct{}{}

		kind, ok := formcatalog.ByKind(spec.Kind)
		if !ok {
			// A bespoke Form owns its own generator and its own fixture proof.
			continue
		}
		desired := kind.CanonicalDesired()
		if !reflect.DeepEqual(sortedKeys(desired), kind.DesiredKeys()) {
			return fmt.Errorf("%s canonical desired keys drifted from its declaration", spec.Kind)
		}
		if err := rejectUnsafeFixtureStrings(spec.Kind, desired); err != nil {
			return err
		}
		if _, err := kind.NegativeDesired(); err != nil {
			return err
		}

		var definition formpackage.FormDefinition
		definitionPath := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", spec.Slug, "definition.json")
		if err := readJSON(definitionPath, &definition); err != nil {
			return fmt.Errorf("read %s portable Interface descriptors: %w", spec.Kind, err)
		}
		if err := verifyInterfaceDescriptors(kind, definition); err != nil {
			return err
		}
	}
	return nil
}

func verifyInterfaceDescriptors(kind formcatalog.Kind, definition formpackage.FormDefinition) error {
	reviewed := kind.InterfaceDescriptors()
	if len(definition.Interfaces) != len(reviewed) {
		return fmt.Errorf("%s declares %d Interfaces, want %d", kind.Kind, len(definition.Interfaces), len(reviewed))
	}
	actualRaw, err := json.Marshal(definition.Interfaces)
	if err != nil {
		return err
	}
	reviewedRaw, err := json.Marshal(reviewed)
	if err != nil {
		return err
	}
	if string(actualRaw) != string(reviewedRaw) {
		return fmt.Errorf("%s portable Interface descriptor is not the reviewed declaration", kind.Kind)
	}
	for _, descriptor := range definition.Interfaces {
		if descriptor.Version != "1" || !descriptor.Required || descriptor.Document == nil || descriptor.DocumentSchema == nil {
			return fmt.Errorf("%s portable Interface identity or required contract is invalid", kind.Kind)
		}
		for _, input := range descriptor.Inputs {
			if !formpackage.PortableInterfaceInputSource(input.Source) {
				return fmt.Errorf("%s Interface input %s is not portable", kind.Kind, input.Name)
			}
		}
		raw, err := json.Marshal(descriptor)
		if err != nil {
			return err
		}
		// An interface name is open, but it may not smuggle one host's
		// commercial identity in as if it were part of the portable contract.
		if lowered := strings.ToLower(string(raw)); strings.Contains(lowered, ".cloud/") || strings.Contains(lowered, "cloud.com") {
			return fmt.Errorf("%s Interface descriptor contains a Cloud-specific identity", kind.Kind)
		}
	}
	return nil
}

func rejectUnsafeFixtureStrings(kind string, value any) error {
	switch typed := value.(type) {
	case string:
		if unsafeFixtureString.MatchString(typed) || illustrativeDigest(typed) {
			return fmt.Errorf("%s canonical fixture contains an illustrative or unsafe value", kind)
		}
	case []any:
		for _, item := range typed {
			if err := rejectUnsafeFixtureStrings(kind, item); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range typed {
			if err := rejectUnsafeFixtureStrings(kind, item); err != nil {
				return err
			}
		}
	}
	return nil
}

// illustrativeDigest reports a digest that is obviously filler rather than the
// hash of real bytes.
func illustrativeDigest(value string) bool {
	value = strings.TrimPrefix(strings.ToLower(value), "sha256:")
	if len(value) != 64 {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] != value[0] {
			return false
		}
	}
	return strings.ContainsRune("0123456789abcdef", rune(value[0]))
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
