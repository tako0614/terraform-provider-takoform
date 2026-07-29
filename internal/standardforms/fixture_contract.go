package standardforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

type exactNegativeFixture struct {
	stage string
	path  string
	input map[string]any
}

// verifyGeneratedFixtureContract keeps read-only verification as strong as
// generation. Updating package digests cannot make a hand-edited candidate
// omit one required-field rejection or weaken its kind-bound observed schema.
func verifyGeneratedFixtureContract(packageRoot string, kind formcatalog.Kind) error {
	raw, err := os.ReadFile(filepath.Join(packageRoot, "definition.json"))
	if err != nil {
		return err
	}
	definition, err := formpackage.ValidateDefinition(raw)
	if err != nil {
		return err
	}
	if definition.Kind != kind.Kind {
		return fmt.Errorf("fixture contract kind = %s, want %s", definition.Kind, kind.Kind)
	}
	equal, err := canonicalValuesEqual(definition.ObservedSchema, kind.ObservedSchema())
	if err != nil {
		return err
	}
	if !equal {
		return fmt.Errorf("%s observedSchema is not the exact kind-bound catalog schema", kind.Kind)
	}

	desiredCases, err := kind.NegativeCases()
	if err != nil {
		return err
	}
	expected := make(map[string]exactNegativeFixture, len(desiredCases)+1)
	for _, negative := range desiredCases {
		name := "reject-" + negative.Name
		expected[name] = exactNegativeFixture{
			stage: "desired",
			path:  "fixtures/negative-" + negative.Name + ".json",
			input: negative.Desired,
		}
	}
	expected["reject-observed-foreign-kind-id"] = exactNegativeFixture{
		stage: "observed",
		path:  "fixtures/negative-observed-foreign-kind-id.json",
		input: kind.ForeignKindObserved(),
	}
	if len(expected) > 32 || len(definition.NegativeFixtures) != len(expected) {
		return fmt.Errorf("%s has %d negative fixtures, want exact generated set of %d within maximum 32",
			kind.Kind, len(definition.NegativeFixtures), len(expected))
	}
	seen := make(map[string]struct{}, len(definition.NegativeFixtures))
	for _, fixture := range definition.NegativeFixtures {
		want, ok := expected[fixture.Name]
		if !ok || fixture.Stage != want.stage || fixture.InputPath != want.path ||
			fixture.ExpectedFailure != "schema_validation_failed" {
			return fmt.Errorf("%s negative fixture %q differs from the exact generated contract", kind.Kind, fixture.Name)
		}
		if _, duplicate := seen[fixture.Name]; duplicate {
			return fmt.Errorf("%s duplicates negative fixture %q", kind.Kind, fixture.Name)
		}
		seen[fixture.Name] = struct{}{}
		inputRaw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(fixture.InputPath)))
		if err != nil {
			return err
		}
		wantRaw, err := json.Marshal(want.input)
		if err != nil {
			return err
		}
		inputCanonical, err := formpackage.Canonicalize(inputRaw)
		if err != nil {
			return err
		}
		wantCanonical, err := formpackage.Canonicalize(wantRaw)
		if err != nil {
			return err
		}
		if !bytes.Equal(inputCanonical, wantCanonical) {
			return fmt.Errorf("%s negative fixture %q input differs from the exact catalog case", kind.Kind, fixture.Name)
		}
	}
	return nil
}

func canonicalValuesEqual(left, right any) (bool, error) {
	leftRaw, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	rightRaw, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	leftCanonical, err := formpackage.Canonicalize(leftRaw)
	if err != nil {
		return false, err
	}
	rightCanonical, err := formpackage.Canonicalize(rightRaw)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftCanonical, rightCanonical), nil
}
