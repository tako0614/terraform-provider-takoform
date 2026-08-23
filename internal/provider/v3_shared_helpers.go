// Shared helpers that survived the withdrawal of the v1alpha2 resource
// files they used to live in (decision 0042).
package provider

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"regexp"
	"strings"
)

func int64FromSpec(value any) types.Int64 {
	switch typed := value.(type) {
	case float64:
		return types.Int64Value(int64(typed))
	case int64:
		return types.Int64Value(typed)
	case int:
		return types.Int64Value(int64(typed))
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return types.Int64Value(parsed)
		}
		return types.Int64Null()
	default:
		return types.Int64Null()
	}
}

func validatedEffectiveSpace(value types.String, fallback string) (string, error) {
	space := effectiveSpace(value, fallback)
	if err := clientv3.ValidateSpaceID(space); err != nil {
		return "", fmt.Errorf("invalid SpaceID: %w", err)
	}
	return space, nil
}

func splitImportID(id string) (space, name string, err error) {
	if index := strings.Index(id, "/"); index >= 0 {
		space, name = id[:index], id[index+1:]
		if validationErr := clientv3.ValidateSpaceID(space); validationErr != nil {
			return "", "", fmt.Errorf("import ID SpaceID is invalid: %w", validationErr)
		}
	} else {
		name = id
	}
	if !portableNamePattern.MatchString(name) {
		return "", "", fmt.Errorf(
			"import ID Resource name %q does not match the canonical PatternName grammar",
			name,
		)
	}
	return space, name, nil
}

func toStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func optionalString(value string) types.String {
	if strings.TrimSpace(value) == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func optionalStringFromAny(value any) types.String {
	if text, ok := value.(string); ok && text != "" {
		return types.StringValue(text)
	}
	return types.StringNull()
}

func effectiveSpace(value types.String, fallback string) string {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return fallback
	}
	return value.ValueString()
}

// portableNamePattern is the portable resource-name grammar the wire schema
// states: `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`. A provider that accepted a
// trailing hyphen would plan a name the host refuses, turning a local typo
// into a failed apply instead of a refused plan.
var portableNamePattern = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

// portableMapKeyPattern is the key grammar every portable typed map uses,
// stated by the wire schema and lane-neutral. It used to be imported from the
// withdrawn central-epoch catalog.
const portableMapKeyPattern = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
