// Command current-form-source renders the independently authored v1alpha2
// definitions and fixtures consumed by the deterministic package builder.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

type sourceForm struct {
	Kind       string                     `json:"kind"`
	ProposalID string                     `json:"proposalId"`
	Slug       string                     `json:"slug"`
	Definition formpackage.FormDefinition `json:"definition"`
	// DefinitionJSON is the exact indented JSON text of Definition. The
	// candidate writer consumes this verbatim so large integer schema values
	// (for example the generation maximum 9223372036854775807) survive the
	// JavaScript pipeline without IEEE 754 float64 rounding.
	DefinitionJSON string                    `json:"definitionJson"`
	Fixtures       map[string]map[string]any `json:"fixtures"`
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: current-form-source")
		os.Exit(2)
	}
	forms := make([]sourceForm, 0, len(currentformcatalog.Kinds))
	for _, kind := range currentformcatalog.Kinds {
		form, err := render(kind)
		if err != nil {
			fmt.Fprintln(os.Stderr, "current-form-source:", err)
			os.Exit(1)
		}
		forms = append(forms, form)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(forms); err != nil {
		fmt.Fprintln(os.Stderr, "current-form-source:", err)
		os.Exit(1)
	}
}

func render(kind formcatalog.Kind) (sourceForm, error) {
	fixtures := map[string]map[string]any{
		"desired.json":  kind.CanonicalDesired(),
		"observed.json": kind.CanonicalObserved(),
		"output.json":   kind.CanonicalOutput(),
	}
	negativeCases, err := kind.NegativeCases()
	if err != nil {
		return sourceForm{}, fmt.Errorf("%s: %w", kind.Kind, err)
	}
	negative := make([]formpackage.NegativeFixture, 0, len(negativeCases)+1)
	for _, item := range negativeCases {
		path := "fixtures/negative-" + item.Name + ".json"
		fixtures[path[len("fixtures/"):]] = item.Desired
		negative = append(negative, formpackage.NegativeFixture{
			Name: "reject-" + item.Name, Stage: "desired", InputPath: path,
			ExpectedFailure: "schema_validation_failed",
		})
	}
	observedPath := "fixtures/negative-observed-foreign-kind-id.json"
	fixtures[observedPath[len("fixtures/"):]] = kind.ForeignKindObserved()
	negative = append(negative, formpackage.NegativeFixture{
		Name: "reject-observed-foreign-kind-id", Stage: "observed", InputPath: observedPath,
		ExpectedFailure: "schema_validation_failed",
	})
	definition := formpackage.FormDefinition{
		APIVersion: formpackage.CurrentFormAPIVersion, Kind: kind.Kind,
		DefinitionVersion: kind.DefinitionVersion, Title: kind.Title,
		Description:   kind.Description,
		DesiredSchema: kind.DesiredSchema(), ObservedSchema: kind.ObservedSchema(),
		OutputSchema: kind.OutputSchema(), ImmutableFields: kind.ImmutableFields(),
		LifecycleCapabilities: []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		Interfaces:            kind.InterfaceDescriptors(),
		ConformanceFixtures:   []formpackage.ConformanceFixture{{Name: "canonical", DesiredPath: "fixtures/desired.json", ObservedPath: "fixtures/observed.json", OutputPath: "fixtures/output.json"}},
		NegativeFixtures:      negative,
	}
	definitionJSON, err := marshalIndented(definition)
	if err != nil {
		return sourceForm{}, fmt.Errorf("%s: %w", kind.Kind, err)
	}
	return sourceForm{
		Kind: kind.Kind, ProposalID: kind.ProposalID, Slug: kind.Slug, Fixtures: fixtures,
		Definition: definition, DefinitionJSON: definitionJSON,
	}, nil
}

// marshalIndented renders value as two-space-indented JSON text with HTML
// escaping disabled, matching the JSON text the candidate writer emits. The
// trailing newline mirrors the previous JavaScript writer output.
func marshalIndented(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
