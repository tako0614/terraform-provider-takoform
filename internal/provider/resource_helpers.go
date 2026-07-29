package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

// observeResourceForRead keeps Terraform's ordinary state refresh separate
// from the host's explicit state/output publication operation. A host first
// returns the current desired-generation fence via exact GET; observe then
// performs the read-only native drift check against that exact generation.
// The GET spec is retained so setState can require the observation response
// to repeat the exact current desired state before writing anything.
func observeResourceForRead(
	ctx context.Context,
	c *client.Client,
	kind,
	name,
	space string,
	form client.InstalledFormReference,
) (*client.Resource, map[string]any, error) {
	current, err := c.GetResource(ctx, kind, name, space, form)
	if err != nil {
		return nil, nil, err
	}
	observed, err := c.ObserveResource(ctx, kind, name, space, client.MutationFence{
		ResourceVersion: current.Metadata.ResourceVersion,
		Form:            form,
	})
	if err != nil {
		return nil, nil, err
	}
	return observed, current.Spec, nil
}

func resourceIDForKind(kind, name string) string {
	return kind + "/" + name
}

func effectiveSpace(value types.String, fallback string) string {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return fallback
	}
	return value.ValueString()
}

func validatedEffectiveSpace(value types.String, fallback string) (string, error) {
	space := effectiveSpace(value, fallback)
	if err := client.ValidateSpaceID(space); err != nil {
		return "", fmt.Errorf("invalid SpaceID: %w", err)
	}
	return space, nil
}

func outputsToStringMap(outputs map[string]any) map[string]string {
	if len(outputs) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(outputs))
	for key, value := range outputs {
		switch v := value.(type) {
		case string:
			out[key] = v
		case nil:
			out[key] = ""
		default:
			if raw, err := json.Marshal(v); err == nil {
				out[key] = string(raw)
			} else {
				out[key] = fmt.Sprint(v)
			}
		}
	}
	return out
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

func toInt64Slice(raw any) []int64 {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			out = append(out, int64(typed))
		case int64:
			out = append(out, typed)
		case int:
			out = append(out, int64(typed))
		case json.Number:
			parsed, err := typed.Int64()
			if err != nil {
				return nil
			}
			out = append(out, parsed)
		}
	}
	return out
}

func toStringMap(raw any) map[string]string {
	values, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}
