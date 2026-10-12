package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

var (
	_ basetypes.StringTypable                    = v2JSONSemanticStringType{}
	_ basetypes.StringValuableWithSemanticEquals = v2JSONSemanticStringValue{}
)

// v2JSONSemanticStringType keeps vars_json as an authored JSON object string,
// while asking Terraform to compare it using the exact Form's JSON semantics.
type v2JSONSemanticStringType struct{ basetypes.StringType }

func (t v2JSONSemanticStringType) Equal(other attr.Type) bool {
	otherType, ok := other.(v2JSONSemanticStringType)
	return ok && t.StringType.Equal(otherType.StringType)
}

func (v2JSONSemanticStringType) String() string { return "v2JSONSemanticStringType" }

func (t v2JSONSemanticStringType) ValueFromString(ctx context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return v2JSONSemanticStringValue{StringValue: value}, nil
}

func (t v2JSONSemanticStringType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
	base, err := t.StringType.ValueFromTerraform(ctx, value)
	if err != nil {
		return nil, err
	}
	stringValue, ok := base.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected Terraform value type %T; want basetypes.StringValue", base)
	}
	converted, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to semantic JSON string: %v", diags)
	}
	return converted, nil
}

func (v2JSONSemanticStringType) ValueType(context.Context) attr.Value {
	return v2JSONSemanticStringValue{}
}

type v2JSONSemanticStringValue struct{ basetypes.StringValue }

func (v v2JSONSemanticStringValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(v2JSONSemanticStringValue)
	return ok && v.StringValue.Equal(otherValue.StringValue)
}

func (v v2JSONSemanticStringValue) Type(context.Context) attr.Type {
	return v2JSONSemanticStringType{}
}

func (v v2JSONSemanticStringValue) StringSemanticEquals(_ context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	otherValue, ok := other.(v2JSONSemanticStringValue)
	if !ok {
		var diags diag.Diagnostics
		diags.AddError("Invalid vars_json comparison", "Terraform supplied an unexpected value type for the exact Form JSON comparison.")
		return false, diags
	}
	if v.IsNull() || v.IsUnknown() || otherValue.IsNull() || otherValue.IsUnknown() {
		return v.IsNull() && otherValue.IsNull() || v.IsUnknown() && otherValue.IsUnknown(), nil
	}
	left, err := decodeV2VarsJSON(v.ValueString())
	if err != nil {
		return false, invalidV2VarsJSONDiagnostics()
	}
	right, err := decodeV2VarsJSON(otherValue.ValueString())
	if err != nil {
		return false, invalidV2VarsJSONDiagnostics()
	}
	return equalV2JSON(left, right), nil
}

func decodeV2VarsJSON(raw string) (any, error) {
	value, err := jsonv2.Decode([]byte(raw))
	if err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("vars_json must contain an object")
	}
	return value, nil
}

func invalidV2VarsJSONDiagnostics() diag.Diagnostics {
	var diags diag.Diagnostics
	diags.AddError("Invalid vars_json comparison", "The exact Form requires vars_json to be a strict JSON object; values are omitted from diagnostics.")
	return diags
}

func equalV2JSON(left, right any) bool {
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case jsonNumber:
		rightValue, ok := right.(jsonNumber)
		return ok && equalV2JSONNumbers(string(leftValue), string(rightValue))
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for i := range leftValue {
			if !equalV2JSON(leftValue[i], rightValue[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, leftItem := range leftValue {
			rightItem, exists := rightValue[key]
			if !exists || !equalV2JSON(leftItem, rightItem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// jsonNumber is an alias so the semantic switch is explicit while retaining
// json.Number's lossless text representation.
type jsonNumber = json.Number

func equalV2JSONNumbers(left, right string) bool {
	leftCanonical, leftOK := canonicalV2JSONNumber(left)
	rightCanonical, rightOK := canonicalV2JSONNumber(right)
	return leftOK && rightOK && leftCanonical == rightCanonical
}

func canonicalV2JSONNumber(raw string) (string, bool) {
	negative := strings.HasPrefix(raw, "-")
	unsigned := raw
	if negative {
		unsigned = raw[1:]
	}
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(unsigned), "e")
	exponent := new(big.Int)
	if hasExponent {
		if _, ok := exponent.SetString(exponentText, 10); !ok {
			return "", false
		}
	}
	integerPart, fractionalPart, hasFraction := strings.Cut(mantissa, ".")
	digits := integerPart + fractionalPart
	if hasFraction {
		exponent.Sub(exponent, big.NewInt(int64(len(fractionalPart))))
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "0", true
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent.Add(exponent, big.NewInt(int64(len(digits)-len(trimmed))))
	digits = trimmed
	sign := ""
	if negative {
		sign = "-"
	}
	return sign + digits + "e" + exponent.String(), true
}
