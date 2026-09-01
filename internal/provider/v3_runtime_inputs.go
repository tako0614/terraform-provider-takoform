package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	v3RuntimeInputsEnvironmentVariable = "TAKOFORM_RUNTIME_INPUTS_FILE"
	v3RuntimeInputsFileFormat          = "takoform.worker-runtime-inputs@v1"
	v3RuntimeInputsMaximumFileBytes    = 1 << 20
	v3RuntimeInputsMaximumBindings     = 64
	v3RuntimeInputMaximumValueBytes    = 32 << 10
	v3RuntimeOperationKeyDomain        = "takoform.provider.worker-version-runtime-operation-key@v1"
	v3RuntimeOperationKeyPrefix        = "takoform-worker-runtime-v1-"
)

var v3RuntimeInputsNoncePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22,128}$`)

type v3RuntimeInputsFile struct {
	Format                  string            `json:"format"`
	MaterialGenerationNonce string            `json:"materialGenerationNonce"`
	CanonicalPublicOrigin   string            `json:"canonicalPublicOrigin"`
	Bindings                map[string]string `json:"bindings"`
}

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

func loadV3RuntimeInputs(path, expectedOrigin string, declaredNames []string) (*v3RuntimeInputs, error) {
	raw, err := secureReadV3RuntimeInputsFile(path)
	if err != nil {
		return nil, fmt.Errorf("runtime inputs file is not secure: %w", err)
	}
	defer clearV3RuntimeInputBytes(raw)

	var document v3RuntimeInputsFile
	// encoding/json necessarily materializes immutable strings. Drop those
	// references promptly after validation and copy only the accepted values
	// into mutable buffers that later stages can wipe best-effort.
	defer func() {
		for name := range document.Bindings {
			document.Bindings[name] = ""
			delete(document.Bindings, name)
		}
	}()
	if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
		return nil, errors.New("runtime inputs file is not a closed valid I-JSON document")
	}
	if document.Format != v3RuntimeInputsFileFormat {
		return nil, fmt.Errorf("runtime inputs file format must be %q", v3RuntimeInputsFileFormat)
	}
	decodedNonce, nonceErr := base64.RawURLEncoding.DecodeString(document.MaterialGenerationNonce)
	validNonce := v3RuntimeInputsNoncePattern.MatchString(document.MaterialGenerationNonce) && nonceErr == nil &&
		len(decodedNonce) >= 16 && base64.RawURLEncoding.EncodeToString(decodedNonce) == document.MaterialGenerationNonce
	clearV3RuntimeInputBytes(decodedNonce)
	if !validNonce {
		return nil, errors.New("runtime inputs materialGenerationNonce must be 22..128 unpadded base64url characters")
	}
	if err := validateV3RuntimeInputsOrigin(document.CanonicalPublicOrigin, expectedOrigin); err != nil {
		return nil, err
	}
	if document.Bindings == nil {
		return nil, errors.New("runtime inputs bindings must be an object")
	}
	if len(document.Bindings) < 1 || len(document.Bindings) > v3RuntimeInputsMaximumBindings {
		return nil, fmt.Errorf("runtime inputs bindings count must be in 1..%d", v3RuntimeInputsMaximumBindings)
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
	if len(declared) != len(document.Bindings) {
		return nil, v3RuntimeInputsBindingSetError(declared, document.Bindings)
	}
	for name, value := range document.Bindings {
		if !v3SensitiveBindingNamePattern.MatchString(name) {
			return nil, errors.New("runtime inputs binding name is invalid")
		}
		if _, ok := declared[name]; !ok {
			return nil, v3RuntimeInputsBindingSetError(declared, document.Bindings)
		}
		if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("runtime input %q must be UTF-8 text without NUL", name)
		}
		if len(value) < 1 || len(value) > v3RuntimeInputMaximumValueBytes {
			return nil, fmt.Errorf("runtime input %q value size must be in 1..%d UTF-8 bytes", name, v3RuntimeInputMaximumValueBytes)
		}
	}
	bindings := make(map[string][]byte, len(document.Bindings))
	for name, value := range document.Bindings {
		bindings[name] = append([]byte(nil), value...)
	}

	return &v3RuntimeInputs{
		MaterialGenerationNonce: document.MaterialGenerationNonce,
		CanonicalPublicOrigin:   document.CanonicalPublicOrigin,
		Bindings:                bindings,
	}, nil
}

var v3SensitiveBindingNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func validateV3RuntimeInputsOrigin(origin, expected string) error {
	parsed, err := url.Parse(origin)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("runtime inputs canonicalPublicOrigin must be an absolute HTTPS origin")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("runtime inputs canonicalPublicOrigin must contain only scheme and authority")
	}
	if parsed.Host != strings.ToLower(parsed.Host) || parsed.Port() == "443" || parsed.String() != origin {
		return errors.New("runtime inputs canonicalPublicOrigin is not in canonical origin form")
	}
	if origin != expected {
		return errors.New("runtime inputs canonicalPublicOrigin does not exactly match the configured Host origin")
	}
	return nil
}

func v3RuntimeInputsBindingSetError(declared map[string]struct{}, supplied map[string]string) error {
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
	return fmt.Errorf("runtime inputs binding names do not exactly match required_sensitive_vars (missing=%v extra=%v)", missing, extra)
}

func clearV3RuntimeInputBytes(raw []byte) {
	for index := range raw {
		raw[index] = 0
	}
}

func v3RuntimeInputFilePath() (string, bool) {
	path, present := os.LookupEnv(v3RuntimeInputsEnvironmentVariable)
	return path, present
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
	filePath, hasFile := v3RuntimeInputFilePath()

	var configuredKey types.String
	if req.Config.Schema != nil && !req.Config.Raw.IsNull() {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &configuredKey)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if !hasFile {
		if namesKnown && len(names) > 0 {
			resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
				"WorkerVersion sensitive inputs have no runtime file",
				"This WorkerVersion declares required_sensitive_vars, but TAKOFORM_RUNTIME_INPUTS_FILE is not set. The provider refuses before any Host mutation.",
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
			"TAKOFORM_RUNTIME_INPUTS_FILE is set, but required_sensitive_vars is not wholly known during plan. The provider cannot compute a plan-stable operation identity.",
		))
		return
	}
	if len(names) == 0 {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"Runtime inputs file has no WorkerVersion declaration",
			"TAKOFORM_RUNTIME_INPUTS_FILE is set, but this WorkerVersion declares no required_sensitive_vars. The provider refuses to route undeclared values.",
		))
		return
	}
	if configuredKey.IsUnknown() || !configuredKey.IsNull() {
		resp.Diagnostics.Append(v3RuntimeInputsDiagnostic(
			"Sensitive WorkerVersion operation key is provider-owned",
			"When TAKOFORM_RUNTIME_INPUTS_FILE is set, apply_idempotency_key must be omitted. The provider computes it from the material generation nonce and value-free logical apply identity.",
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
	material, err := loadV3RuntimeInputs(filePath, r.data.clientV3.Endpoint(), names)
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
		Repair:  "Regenerate the one run-scoped runtime input file for this exact plan, or remove every sensitive declaration and the file together.",
	}.error()
}

func v3RuntimeInputsError(err error) diag.Diagnostic {
	return v3RuntimeInputsDiagnostic(
		"WorkerVersion runtime inputs are invalid",
		"The provider rejected TAKOFORM_RUNTIME_INPUTS_FILE before any Host mutation: "+err.Error(),
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
	filePath, hasFile := v3RuntimeInputFilePath()
	if !hasFile && len(names) == 0 {
		return nil, true
	}
	if !hasFile {
		diags.Append(v3RuntimeInputsDiagnostic(
			"WorkerVersion sensitive inputs have no runtime file",
			"This WorkerVersion declares required_sensitive_vars, but TAKOFORM_RUNTIME_INPUTS_FILE is not set. The provider refuses before any Host mutation.",
		))
		return nil, false
	}
	if len(names) == 0 {
		diags.Append(v3RuntimeInputsDiagnostic(
			"Runtime inputs file has no WorkerVersion declaration",
			"TAKOFORM_RUNTIME_INPUTS_FILE is set, but this WorkerVersion declares no required_sensitive_vars. The provider refuses to route undeclared values.",
		))
		return nil, false
	}
	if r.data == nil || r.data.clientV3 == nil {
		diags.Append(v3RuntimeInputsDiagnostic(
			"Cannot bind runtime inputs to a Host origin",
			"The Provider's stable Host client is unavailable, so the runtime file origin cannot be checked before apply.",
		))
		return nil, false
	}
	if values.ApplyIdempotencyKey.IsNull() || values.ApplyIdempotencyKey.IsUnknown() {
		diags.Append(v3RuntimeInputsDiagnostic(
			"Sensitive WorkerVersion has no plan-stable operation key",
			"The apply plan does not carry the provider-computed operation key. Re-plan with the exact run-scoped runtime input file before applying.",
		))
		return nil, false
	}
	material, err := loadV3RuntimeInputs(filePath, r.data.clientV3.Endpoint(), names)
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
