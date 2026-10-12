// Package privateinputsv2 reads caller-supplied WorkerVersion private inputs
// for an exact Host v2 Space/resource pair. It does not persist or transmit
// values and does not decide whether a Host accepts them.
package privateinputsv2

import (
	"errors"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

const (
	// EnvironmentVariable contains a JSON object nested by Host Space, resource
	// name, and WorkerVersion private-input name.
	EnvironmentVariable = "TAKOFORM_V2_PRIVATE_INPUTS_JSON"

	// MaxDocumentBytes bounds the process-local JSON source.
	MaxDocumentBytes = 1 << 20
	// MaxValueBytes bounds each private value accepted by the Provider runtime
	// input contract.
	MaxValueBytes = 32768
)

var errInvalidSource = errors.New("takoform v2: invalid private-input source")

// Load selects only the values declared by expectedNames at the exact Host
// Space and resource name. An absent source is valid only when no values are
// expected. The returned map is caller-owned; this helper does not retain it.
func Load(space, resourceName string, expectedNames []string) (map[string]string, error) {
	if !validHostIdentifier(space) || !validHostIdentifier(resourceName) || !validExpectedNames(expectedNames) {
		return nil, errInvalidSource
	}

	raw, present := os.LookupEnv(EnvironmentVariable)
	if !present {
		if len(expectedNames) == 0 {
			return map[string]string{}, nil
		}
		return nil, errInvalidSource
	}
	if len(raw) > MaxDocumentBytes {
		return nil, errInvalidSource
	}

	value, err := jsonv2.Decode([]byte(raw))
	if err != nil {
		return nil, errInvalidSource
	}
	root, ok := value.(map[string]any)
	if !ok || !validDocumentShape(root) {
		return nil, errInvalidSource
	}

	selectedSpace, ok := root[space]
	if !ok {
		if len(expectedNames) == 0 {
			return map[string]string{}, nil
		}
		return nil, errInvalidSource
	}
	resources, ok := selectedSpace.(map[string]any)
	if !ok {
		return nil, errInvalidSource
	}
	selectedResource, ok := resources[resourceName]
	if !ok {
		if len(expectedNames) == 0 {
			return map[string]string{}, nil
		}
		return nil, errInvalidSource
	}
	inputs, ok := selectedResource.(map[string]any)
	if !ok {
		return nil, errInvalidSource
	}
	if len(inputs) != len(expectedNames) {
		return nil, errInvalidSource
	}

	wanted := make(map[string]struct{}, len(expectedNames))
	for _, name := range expectedNames {
		wanted[name] = struct{}{}
	}
	selected := make(map[string]string, len(inputs))
	for name, rawValue := range inputs {
		if _, ok := wanted[name]; !ok {
			return nil, errInvalidSource
		}
		value, ok := rawValue.(string)
		if !ok || !validPrivateValue(value) {
			return nil, errInvalidSource
		}
		selected[name] = value
	}
	return selected, nil
}

func validHostIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 || !isASCIIAlphaNumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIIAlphaNumeric(character) && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func validExpectedNames(names []string) bool {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !validExpectedName(name) {
			return false
		}
		if _, duplicate := seen[name]; duplicate {
			return false
		}
		seen[name] = struct{}{}
	}
	return true
}

func validExpectedName(name string) bool {
	if len(name) == 0 || len(name) > 64 || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if !(character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_') {
			return false
		}
	}
	return true
}

func validPrivateValue(value string) bool {
	return len(value) > 0 && len(value) <= MaxValueBytes && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validDocumentShape(root map[string]any) bool {
	for space, rawResources := range root {
		if !validHostIdentifier(space) {
			return false
		}
		resources, ok := rawResources.(map[string]any)
		if !ok {
			return false
		}
		for resourceName, rawInputs := range resources {
			if !validHostIdentifier(resourceName) {
				return false
			}
			inputs, ok := rawInputs.(map[string]any)
			if !ok {
				return false
			}
			for name, rawValue := range inputs {
				if !validExpectedName(name) {
					return false
				}
				value, ok := rawValue.(string)
				if !ok || !validPrivateValue(value) {
					return false
				}
			}
		}
	}
	return true
}
