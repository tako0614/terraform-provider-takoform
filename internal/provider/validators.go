package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringOneOfValidator validates that a configured string is one of a fixed
// allow-list. It is a tiny in-tree validator so the provider keeps its
// dependency surface to terraform-plugin-framework alone.
type stringOneOfValidator struct {
	allowed []string
}

// StringOneOf returns a validator.String enforcing membership in allowed.
func StringOneOf(allowed ...string) validator.String {
	return stringOneOfValidator{allowed: allowed}
}

func (v stringOneOfValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %s", strings.Join(v.allowed, ", "))
}

func (v stringOneOfValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringOneOfValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, a := range v.allowed {
		if val == a {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid value",
		fmt.Sprintf("%q is not a valid value; must be one of: %s", val, strings.Join(v.allowed, ", ")),
	)
}

// stringTokenValidator validates an extensible capability token. It is used for
// fields where the Takoform endpoint, not the provider binary, owns the final
// allow-list.
type stringTokenValidator struct{}

// StringToken returns a validator.String enforcing a non-empty token without
// whitespace when the optional value is configured.
func StringToken() validator.String {
	return stringTokenValidator{}
}

type stringPatternValidator struct {
	pattern     string
	description string
}

// StringMatches validates an exact portable Form string grammar while the
// host remains authoritative for capability availability.
func StringMatches(pattern, description string) validator.String {
	return stringPatternValidator{pattern: pattern, description: description}
}

func (v stringPatternValidator) Description(_ context.Context) string {
	return v.description
}

func (v stringPatternValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringPatternValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	matched, err := regexp.MatchString(v.pattern, req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid provider validator", err.Error())
		return
	}
	if !matched {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", v.description)
	}
}

type stringOCIDigestReferenceValidator struct{}

// StringOCIDigestReference requires an immutable OCI image digest rather than
// a mutable tag. The exact rule mirrors the current ContainerService standard.
func StringOCIDigestReference() validator.String {
	return stringOCIDigestReferenceValidator{}
}

func (stringOCIDigestReferenceValidator) Description(_ context.Context) string {
	return "value must be an OCI image reference pinned by sha256 digest"
}

func (v stringOCIDigestReferenceValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (stringOCIDigestReferenceValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validOCIDigestReference(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid OCI image reference", "image must be pinned as repository@sha256:<64 hexadecimal characters>")
	}
}

type int64AtLeastValidator struct {
	minimum int64
}

// Int64AtLeast validates a lower bound owned by the portable Form schema.
func Int64AtLeast(minimum int64) validator.Int64 {
	return int64AtLeastValidator{minimum: minimum}
}

func (v int64AtLeastValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be at least %d", v.minimum)
}

func (v int64AtLeastValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64AtLeastValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueInt64() < v.minimum {
		resp.Diagnostics.AddAttributeError(req.Path, "Value below minimum", v.Description(context.Background()))
	}
}

type setInt64RangeValidator struct {
	minItems int
	minimum  int64
	maximum  int64
}

// SetInt64Range validates integer set elements and an optional minimum size.
func SetInt64Range(minItems int, minimum, maximum int64) validator.Set {
	return setInt64RangeValidator{minItems: minItems, minimum: minimum, maximum: maximum}
}

func (v setInt64RangeValidator) Description(_ context.Context) string {
	return fmt.Sprintf("at least %d value(s), each between %d and %d", v.minItems, v.minimum, v.maximum)
}

func (v setInt64RangeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v setInt64RangeValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var values []types.Int64
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &values, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(values) < v.minItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too few values", v.Description(ctx))
	}
	for _, value := range values {
		if value.IsNull() || value.IsUnknown() {
			continue
		}
		if candidate := value.ValueInt64(); candidate < v.minimum || candidate > v.maximum {
			resp.Diagnostics.AddAttributeError(req.Path, "Value outside range", fmt.Sprintf("%d must be between %d and %d", candidate, v.minimum, v.maximum))
		}
	}
}

func (v stringTokenValidator) Description(_ context.Context) string {
	return "value must be a non-empty token without whitespace"
}

func (v stringTokenValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringTokenValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if strings.TrimSpace(val) == "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", "value must not be blank")
		return
	}
	if strings.ContainsFunc(val, unicode.IsSpace) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("%q contains whitespace", val))
	}
}

type setStringsPatternValidator struct {
	minItems    int
	pattern     string
	description string
}

// SetStringsMatch validates cardinality and the exact grammar of each string.
func SetStringsMatch(minItems int, pattern, description string) validator.Set {
	return setStringsPatternValidator{minItems: minItems, pattern: pattern, description: description}
}

func (v setStringsPatternValidator) Description(_ context.Context) string {
	return fmt.Sprintf("at least %d value(s); each %s", v.minItems, v.description)
}

func (v setStringsPatternValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v setStringsPatternValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var values []types.String
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &values, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(values) < v.minItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too few values", v.Description(ctx))
	}
	for _, value := range values {
		if value.IsNull() || value.IsUnknown() {
			continue
		}
		matched, err := regexp.MatchString(v.pattern, value.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid provider validator", err.Error())
			return
		}
		if !matched {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", v.description)
		}
	}
}

// setStringsOneOfValidator validates that every element of a set of strings is
// in a fixed allow-list and that the set has at least minItems elements.
type setStringsOneOfValidator struct {
	allowed  []string
	minItems int
}

// SetStringsOneOf returns a validator.Set enforcing a minimum size and that
// every element is in allowed.
func SetStringsOneOf(minItems int, allowed ...string) validator.Set {
	return setStringsOneOfValidator{allowed: allowed, minItems: minItems}
}

func (v setStringsOneOfValidator) Description(_ context.Context) string {
	return fmt.Sprintf("each value must be one of: %s", strings.Join(v.allowed, ", "))
}

func (v setStringsOneOfValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v setStringsOneOfValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var elems []types.String
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &elems, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(elems) < v.minItems {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Too few values",
			fmt.Sprintf("at least %d value(s) required, got %d", v.minItems, len(elems)),
		)
	}

	for _, e := range elems {
		if e.IsNull() || e.IsUnknown() {
			continue
		}
		val := e.ValueString()
		ok := false
		for _, a := range v.allowed {
			if val == a {
				ok = true
				break
			}
		}
		if !ok {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid value",
				fmt.Sprintf("%q is not a valid value; must be one of: %s", val, strings.Join(v.allowed, ", ")),
			)
		}
	}
}

// setStringsNonEmptyValidator validates that every element of a set is a
// non-empty capability token and that the set has at least minItems elements.
// It deliberately does not enforce a fixed allow-list: extensible surfaces are
// accepted or rejected by the selected host's capabilities and policy.
type setStringsNonEmptyValidator struct {
	minItems int
}

// SetStringsNonEmpty returns a validator.Set enforcing a minimum size and
// non-empty token strings without whitespace.
func SetStringsNonEmpty(minItems int) validator.Set {
	return setStringsNonEmptyValidator{minItems: minItems}
}

func (v setStringsNonEmptyValidator) Description(_ context.Context) string {
	return "each value must be a non-empty token without whitespace"
}

func (v setStringsNonEmptyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v setStringsNonEmptyValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var elems []types.String
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &elems, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(elems) < v.minItems {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Too few values",
			fmt.Sprintf("at least %d value(s) required, got %d", v.minItems, len(elems)),
		)
	}

	for _, e := range elems {
		if e.IsNull() || e.IsUnknown() {
			continue
		}
		val := e.ValueString()
		if strings.TrimSpace(val) == "" {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", "value must not be blank")
			continue
		}
		if strings.ContainsFunc(val, unicode.IsSpace) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("%q contains whitespace", val))
		}
	}
}

// Int64AtMost rejects a value above an inclusive portable maximum.
func Int64AtMost(maximum int64) validator.Int64 {
	return int64AtMostValidator{maximum: maximum}
}

type int64AtMostValidator struct{ maximum int64 }

func (v int64AtMostValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be at most %d", v.maximum)
}

func (v int64AtMostValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64AtMostValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueInt64() > v.maximum {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", v.Description(context.Background()))
	}
}

// MapKeysMatch rejects a configuration key that is not portable. Map keys are
// held to the same grammar as declared fields so a host never has to guess
// what an author meant by a key it cannot parse.
func MapKeysMatch(pattern, description string) validator.Map {
	return mapKeysPatternValidator{pattern: regexp.MustCompile(pattern), description: description}
}

type mapKeysPatternValidator struct {
	pattern     *regexp.Regexp
	description string
}

func (v mapKeysPatternValidator) Description(_ context.Context) string { return v.description }

func (v mapKeysPatternValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v mapKeysPatternValidator) ValidateMap(_ context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	// Only map keys belong to this validator. Values may legitimately remain
	// unknown while OpenTofu validates a module whose configuration is derived
	// from variables or another Resource. Iterating the framework values keeps
	// that unknown state intact instead of trying to coerce it into Go strings.
	for key := range req.ConfigValue.Elements() {
		if !v.pattern.MatchString(key) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", v.description)
			return
		}
	}
}
