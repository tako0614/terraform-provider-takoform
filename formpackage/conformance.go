package formpackage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConformanceReport struct {
	PositivePackages int `json:"positivePackages"`
	NegativeCases    int `json:"negativeCases"`
}

type conformanceManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Positive      []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"positive"`
	Negative []struct {
		Name      string `json:"name"`
		Operation string `json:"operation"`
		Path      string `json:"path,omitempty"`
		Value     string `json:"value,omitempty"`
		WantError string `json:"wantError"`
	} `json:"negative"`
}

// VerifyConformance runs the committed Form Package v1 corpus. It is local
// and deterministic: no network, trust-root, signature, host, or provider
// operation is performed.
func VerifyConformance(root string) (ConformanceReport, error) {
	manifestPath := filepath.Join(root, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ConformanceReport{}, fmt.Errorf("read conformance manifest: %w", err)
	}
	if _, err := Canonicalize(raw); err != nil {
		return ConformanceReport{}, fmt.Errorf("conformance manifest: %w", err)
	}
	var manifest conformanceManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ConformanceReport{}, err
	}
	if manifest.SchemaVersion != 1 || len(manifest.Positive) == 0 || len(manifest.Negative) == 0 {
		return ConformanceReport{}, fmt.Errorf("conformance manifest must use schemaVersion 1 with positive and negative cases")
	}
	seen := map[string]struct{}{}
	for _, test := range manifest.Positive {
		if test.Name == "" || test.Path == "" || test.Kind == "" {
			return ConformanceReport{}, fmt.Errorf("positive conformance case has empty required field")
		}
		if _, duplicate := seen[test.Name]; duplicate {
			return ConformanceReport{}, fmt.Errorf("duplicate conformance case %q", test.Name)
		}
		seen[test.Name] = struct{}{}
		if err := validatePackagePath(test.Path); err != nil {
			return ConformanceReport{}, fmt.Errorf("positive case %q path: %w", test.Name, err)
		}
		report, err := VerifyDirectory(filepath.Join(root, filepath.FromSlash(test.Path)))
		if err != nil {
			return ConformanceReport{}, fmt.Errorf("positive case %q: %w", test.Name, err)
		}
		if report.FormRef.Kind != test.Kind {
			return ConformanceReport{}, fmt.Errorf("positive case %q kind is %q, want %q", test.Name, report.FormRef.Kind, test.Kind)
		}
	}
	for _, test := range manifest.Negative {
		if test.Name == "" || test.Operation == "" || test.WantError == "" {
			return ConformanceReport{}, fmt.Errorf("negative conformance case has empty required field")
		}
		if _, duplicate := seen[test.Name]; duplicate {
			return ConformanceReport{}, fmt.Errorf("duplicate conformance case %q", test.Name)
		}
		seen[test.Name] = struct{}{}
		var testErr error
		switch test.Operation {
		case "canonicalize":
			input, err := readConformanceInput(root, test.Path)
			if err != nil {
				return ConformanceReport{}, err
			}
			_, testErr = Canonicalize(input)
		case "content-policy":
			input, err := readConformanceInput(root, test.Path)
			if err != nil {
				return ConformanceReport{}, err
			}
			if _, err := Canonicalize(input); err != nil {
				testErr = err
			} else {
				var value any
				if err := json.Unmarshal(input, &value); err != nil {
					testErr = err
				} else {
					testErr = rejectForbiddenContent(value, "$")
				}
			}
		case "schema-policy":
			input, err := readConformanceInput(root, test.Path)
			if err != nil {
				return ConformanceReport{}, err
			}
			if _, err := Canonicalize(input); err != nil {
				testErr = err
			} else {
				var value any
				if err := json.Unmarshal(input, &value); err != nil {
					testErr = err
				} else {
					testErr = verifyFragmentOnlyReferences(value, "schema")
					if testErr == nil {
						testErr = validatePortableSchemaStructure(value, "schema")
					}
				}
			}
		case "definition":
			input, err := readConformanceInput(root, test.Path)
			if err != nil {
				return ConformanceReport{}, err
			}
			_, testErr = ValidateDefinition(input)
		case "package-path":
			testErr = validatePackagePath(test.Value)
		default:
			return ConformanceReport{}, fmt.Errorf("negative case %q has unknown operation %q", test.Name, test.Operation)
		}
		if testErr == nil || !strings.Contains(testErr.Error(), test.WantError) {
			return ConformanceReport{}, fmt.Errorf("negative case %q error = %v, want containing %q", test.Name, testErr, test.WantError)
		}
	}
	return ConformanceReport{PositivePackages: len(manifest.Positive), NegativeCases: len(manifest.Negative)}, nil
}

func readConformanceInput(root, relative string) ([]byte, error) {
	if err := validatePackagePath(relative); err != nil {
		return nil, fmt.Errorf("conformance input path: %w", err)
	}
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
}
