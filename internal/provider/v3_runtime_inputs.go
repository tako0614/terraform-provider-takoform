package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	v3RuntimeInputsMaximumBindings  = 64
	v3RuntimeInputMaximumValueBytes = 32 << 10
	v3RuntimeOperationKeyDomain     = "takoform.provider.worker-version-runtime-operation-key@v1"
	v3RuntimeOperationKeyPrefix     = "takoform-worker-runtime-v1-"
)

var v3RuntimeInputsNoncePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22,128}$`)

// v3RuntimeInputs is deliberately short-lived. It is never embedded in a
// Terraform model, diagnostic, Host Resource, plan, or state value. Callers
// release its references as soon as the private prepare request has consumed
// the values. Mutable buffers are wiped best-effort; Go/runtime/transport
// copies and crash dumps are outside that guarantee.
type v3RuntimeInputs struct {
	MaterialGenerationNonce string
	CanonicalPublicOrigin   string
	Bindings                map[string][]byte
}

func (material *v3RuntimeInputs) release() {
	if material == nil {
		return
	}
	for name := range material.Bindings {
		clearV3RuntimeInputBytes(material.Bindings[name])
		material.Bindings[name] = nil
		delete(material.Bindings, name)
	}
	material.Bindings = nil
	material.MaterialGenerationNonce = ""
	material.CanonicalPublicOrigin = ""
}

var v3SensitiveBindingNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func clearV3RuntimeInputBytes(raw []byte) {
	for index := range raw {
		raw[index] = 0
	}
}

func validateV3RuntimeInputProviderConfiguration(nonce string, bindings map[string]string) error {
	if nonce == "" && len(bindings) == 0 {
		return nil
	}
	decodedNonce, err := base64.RawURLEncoding.DecodeString(nonce)
	validNonce := v3RuntimeInputsNoncePattern.MatchString(nonce) && err == nil &&
		len(decodedNonce) >= 16 && base64.RawURLEncoding.EncodeToString(decodedNonce) == nonce
	clearV3RuntimeInputBytes(decodedNonce)
	if !validNonce {
		return errors.New("runtime_input_nonce must be 22..128 unpadded base64url characters")
	}
	if len(bindings) > v3RuntimeInputsMaximumBindings {
		return fmt.Errorf("runtime_inputs count must be in 0..%d", v3RuntimeInputsMaximumBindings)
	}
	for name, value := range bindings {
		if !v3SensitiveBindingNamePattern.MatchString(name) {
			return errors.New("runtime_inputs binding name is invalid")
		}
		if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 || len(value) < 1 || len(value) > v3RuntimeInputMaximumValueBytes {
			return fmt.Errorf("runtime_inputs value for %q must be 1..%d UTF-8 bytes without NUL", name, v3RuntimeInputMaximumValueBytes)
		}
	}
	return nil
}

func cloneV3RuntimeInputStrings(input map[string]string) map[string]string {
	copy := make(map[string]string, len(input))
	for name, value := range input {
		copy[name] = value
	}
	return copy
}

func v3RuntimeInputsFromProviderData(data *providerData, declaredNames []string, requireValues bool) (*v3RuntimeInputs, error) {
	if data == nil || data.clientV3 == nil {
		return nil, errors.New("the Provider's stable Host client is unavailable")
	}
	if err := validateV3RuntimeInputProviderConfiguration(data.runtimeInputNonce, data.runtimeInputs); err != nil {
		return nil, err
	}
	if data.runtimeInputNonce == "" {
		return nil, errors.New("runtime_input_nonce is not configured")
	}
	declared := make(map[string]struct{}, len(declaredNames))
	for _, name := range declaredNames {
		if !v3SensitiveBindingNamePattern.MatchString(name) {
			return nil, errors.New("declared sensitive binding name is invalid")
		}
		if _, duplicate := declared[name]; duplicate {
			return nil, errors.New("declared sensitive binding names are not unique")
		}
		declared[name] = struct{}{}
	}
	if !requireValues {
		if len(data.runtimeInputs) != 0 {
			return nil, errors.New("runtime_inputs must be empty during Plan")
		}
		return &v3RuntimeInputs{
			MaterialGenerationNonce: data.runtimeInputNonce,
			CanonicalPublicOrigin:   data.clientV3.Endpoint(),
			Bindings:                map[string][]byte{},
		}, nil
	}
	if len(declared) != len(data.runtimeInputs) {
		return nil, v3RuntimeInputMapBindingSetError(declared, data.runtimeInputs)
	}
	bindings := make(map[string][]byte, len(data.runtimeInputs))
	for name, value := range data.runtimeInputs {
		if _, ok := declared[name]; !ok {
			return nil, v3RuntimeInputMapBindingSetError(declared, data.runtimeInputs)
		}
		bindings[name] = append([]byte(nil), value...)
	}
	return &v3RuntimeInputs{
		MaterialGenerationNonce: data.runtimeInputNonce,
		CanonicalPublicOrigin:   data.clientV3.Endpoint(),
		Bindings:                bindings,
	}, nil
}

func v3RuntimeInputMapBindingSetError(declared map[string]struct{}, supplied map[string]string) error {
	missing := make([]string, 0)
	extra := make([]string, 0)
	for name := range declared {
		if _, ok := supplied[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range supplied {
		if _, ok := declared[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return fmt.Errorf("runtime_inputs names do not exactly match required_sensitive_vars (missing=%v extra=%v)", missing, extra)
}

func v3RequiredSensitiveNames(value attr.Value) ([]string, bool) {
	set, ok := value.(types.Set)
	if !ok || set.IsUnknown() {
		return nil, false
	}
	if set.IsNull() {
		return []string{}, true
	}
	names := make([]string, 0, len(set.Elements()))
	for _, element := range set.Elements() {
		name, ok := element.(types.String)
		if !ok || name.IsNull() || name.IsUnknown() {
			return nil, false
		}
		names = append(names, name.ValueString())
	}
	sort.Strings(names)
	return names, true
}

type v3RuntimeOperationLogicalIdentity struct {
	Domain                  string           `json:"domain"`
	MaterialGenerationNonce string           `json:"materialGenerationNonce"`
	CanonicalPublicOrigin   string           `json:"canonicalPublicOrigin"`
	Form                    v3RuntimeFormRef `json:"form"`
	Space                   string           `json:"space"`
	Identity                v3NameIdentity   `json:"identity"`
	Spec                    map[string]any   `json:"spec"`
}

// clientFormRef is the exact value-free public FormRef shape used solely for
// operation-key canonicalization. PackageDigest is intentionally absent: it is
// not in the public apply envelope or Resource identity.
type v3RuntimeFormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

type v3NameIdentity struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
}

func v3RuntimeOperationKey(
	material *v3RuntimeInputs,
	ref v3FormRef,
	space string,
	identity v3NameIdentity,
	spec map[string]any,
) (string, error) {
	if material == nil || identity.Value == "" {
		return "", errors.New("runtime operation identity is incomplete")
	}
	payload := v3RuntimeOperationLogicalIdentity{
		Domain:                  v3RuntimeOperationKeyDomain,
		MaterialGenerationNonce: material.MaterialGenerationNonce,
		CanonicalPublicOrigin:   material.CanonicalPublicOrigin,
		Form: v3RuntimeFormRef{
			APIVersion:        ref.APIVersion,
			Kind:              ref.Kind,
			DefinitionVersion: ref.DefinitionVersion,
			SchemaDigest:      ref.SchemaDigest,
		},
		Space:    space,
		Identity: identity,
		Spec:     spec,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode value-free runtime operation identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return v3RuntimeOperationKeyPrefix + hex.EncodeToString(sum[:]), nil
}

func v3RuntimeLogicalName(values v3Values) (v3NameIdentity, bool) {
	if owner, known := v3PlanKnownString(values.RevisionOwner); known {
		return v3NameIdentity{Selector: "revisionOwner", Value: owner}, true
	}
	if name, known := v3PlanKnownString(values.Name); known {
		return v3NameIdentity{Selector: "name", Value: name}, true
	}
	return v3NameIdentity{}, false
}

func (r *v3FormResource) v3PlanRuntimeInputs(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if !r.supportsApplyIdempotencyKey() || resp.Plan.Raw.IsNull() {
		return
	}
	values, valueDiags := r.v3ValuesFrom(ctx, resp.Plan)
	resp.Diagnostics.Append(valueDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	names, namesKnown := v3RequiredSensitiveNames(values.Fields["required_sensitive_vars"])
	hasRuntimeInput := r.data != nil && r.data.runtimeInputNonce != ""

	var configuredKey types.String
	if req.Config.Schema != nil && !req.Config.Raw.IsNull() {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &configuredKey)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if !hasRuntimeInput {
		if namesKnown && len(names) > 0 {
			resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
				"WorkerVersion sensitive inputs have no provider input",
				"This WorkerVersion declares required_sensitive_vars, but runtime_input_nonce is not configured on its exact provider instance. The provider refuses before any Host mutation.",
			))
			return
		}
		// Optional+Computed would otherwise propose an unknown or prior computed
		// value when configuration omits this attribute. The ordinary no-file
		// lane historically plans null in that case, including during replacement
		// of an existing WorkerVersion. Preserve that behavior exactly; an explicit
		// configured value already occupies the plan and is left untouched.
		if configuredKey.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringNull())...)
		}
		return
	}
	if !namesKnown {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion sensitive input names are unknown",
			"runtime_input_nonce is configured, but required_sensitive_vars is not wholly known during Plan. The provider cannot compute a plan-stable operation identity.",
		))
		return
	}
	if len(names) == 0 {
		if configuredKey.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringNull())...)
		}
		return
	}
	if configuredKey.IsUnknown() || !configuredKey.IsNull() {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"Sensitive WorkerVersion operation key is provider-owned",
			"When runtime_input_nonce is set, apply_idempotency_key must be omitted. The provider computes it from the material generation nonce and value-free logical apply identity.",
		))
		return
	}
	if r.data == nil || r.data.clientV3 == nil {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"Cannot bind runtime inputs to a Host origin",
			"The Provider's stable Host client is unavailable, so the runtime file origin cannot be checked before plan.",
		))
		return
	}
	material, err := v3RuntimeInputsFromProviderData(r.data, names, false)
	if err != nil {
		resp.Diagnostics.Append(v3RuntimeInputsError(err))
		return
	}
	defer material.release()

	spec, resolved := r.v3PlannedSpec(ctx, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	if !resolved {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion apply identity is unknown",
			"The sensitive WorkerVersion's exact desired spec must be wholly known during plan so the provider can compute a plan-stable operation key.",
		))
		return
	}
	codec, ok := r.v3PlanCodec(ctx, resp)
	if !ok {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion exact Form identity is unknown",
			"The provider cannot compute a runtime operation key without the exact WorkerVersion FormRef.",
		))
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	identity, ok := v3RuntimeLogicalName(values)
	if !ok {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion logical name is unknown",
			"Set a wholly known name, or omit name and set a wholly known revision_owner, so the provider can compute the sensitive operation identity.",
		))
		return
	}
	key, err := v3RuntimeOperationKey(material, codec.Ref, space, identity, spec)
	if err != nil {
		resp.Diagnostics.Append(v3RuntimeInputsError(err))
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringValue(key))...)
}

func v3RuntimeInputsDiagnostic(summary, detail string) diag.Diagnostic {
	return v3Diagnostic{
		Summary: summary,
		Pointer: "/spec/requiredSensitiveVars",
		Code:    v3CodeRuntimeInputsInvalid,
		Detail:  detail,
		Repair:  "Re-plan with the exact provider-scoped nonce, then re-supply the matching ephemeral runtime_inputs map only for Apply.",
	}.error()
}

func v3RuntimeInputsError(err error) diag.Diagnostic {
	return v3RuntimeInputsDiagnostic(
		"WorkerVersion runtime inputs are invalid",
		"The provider rejected its provider-scoped runtime input configuration before any Host mutation: "+err.Error(),
	)
}

func (r *v3FormResource) v3RuntimeInputsForApply(
	values v3Values,
	ref v3FormRef,
	space string,
	spec map[string]any,
	diags *diag.Diagnostics,
) (*v3RuntimeInputs, bool) {
	if !r.supportsApplyIdempotencyKey() {
		return nil, true
	}
	names, namesKnown := v3RequiredSensitiveNames(values.Fields["required_sensitive_vars"])
	if !namesKnown {
		diags.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion sensitive input names are unknown",
			"required_sensitive_vars must be wholly known before apply. The provider refuses before any Host mutation.",
		))
		return nil, false
	}
	hasRuntimeInput := r.data != nil && r.data.runtimeInputNonce != ""
	if !hasRuntimeInput && len(names) == 0 {
		return nil, true
	}
	if !hasRuntimeInput {
		diags.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion sensitive inputs have no provider input",
			"This WorkerVersion declares required_sensitive_vars, but runtime_input_nonce is not configured on its exact provider instance. The provider refuses before any Host mutation.",
		))
		return nil, false
	}
	if len(names) == 0 {
		return nil, true
	}
	if r.data == nil || r.data.clientV3 == nil {
		diags.Append(v3RuntimeInputsDiagnostic(
			"Cannot bind runtime inputs to a Host origin",
			"The Provider's stable Host client is unavailable, so the provider-scoped runtime input cannot be bound to an exact Host origin before apply.",
		))
		return nil, false
	}
	if values.ApplyIdempotencyKey.IsNull() || values.ApplyIdempotencyKey.IsUnknown() {
		diags.Append(v3RuntimeInputsDiagnostic(
			"Sensitive WorkerVersion has no plan-stable operation key",
			"The apply plan does not carry the provider-computed operation key. Re-plan with the exact provider-scoped runtime_input_nonce before applying.",
		))
		return nil, false
	}
	material, err := v3RuntimeInputsFromProviderData(r.data, names, true)
	if err != nil {
		diags.Append(v3RuntimeInputsError(err))
		return nil, false
	}
	identity, ok := v3RuntimeLogicalName(values)
	if !ok {
		material.release()
		diags.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion logical name is unknown",
			"The planned name or revision_owner needed to re-check the runtime operation identity is unknown at apply.",
		))
		return nil, false
	}
	expected, err := v3RuntimeOperationKey(material, ref, space, identity, spec)
	if err != nil {
		material.release()
		diags.Append(v3RuntimeInputsError(err))
		return nil, false
	}
	if expected != values.ApplyIdempotencyKey.ValueString() {
		material.release()
		diags.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion runtime inputs changed after plan",
			"The material generation nonce, canonical origin, or value-free logical apply identity no longer matches the provider-computed plan key. No private or public Host mutation was sent.",
		))
		return nil, false
	}
	return material, true
}
