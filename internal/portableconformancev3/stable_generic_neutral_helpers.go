package portableconformancev3

import (
	"encoding/json"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// These helpers are owned by the family-neutral executable model. Keeping
// them here prevents the in-memory adapter from acquiring implementation
// dependencies on the HTTP runner or ReferenceHost merely for small value
// operations.

func genericCloneJSONMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func genericSpecCanonicalDigest(spec map[string]any) (string, error) {
	if spec == nil {
		spec = map[string]any{}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

func genericContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
