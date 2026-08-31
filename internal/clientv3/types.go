package clientv3

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// FormRef pins one immutable typed Form Definition in a namespaced group
// (spec/schemas/form-ref-v1beta1.schema.json). Publication and admission
// are external to this value.
type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

// FormReference is the resource envelope's form block. PackageDigest is
// audit evidence recording the package the definition was installed from; it
// never enters resource identity, queries, or mutation fences.
type FormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest,omitempty"`
}

// Metadata is the v1beta1 resource metadata block. Name and Space are
// client-owned; UID, Generation, and Revision are host-owned identity
// (spec/decisions/0011) and required on every response.
type Metadata struct {
	Name       string `json:"name"`
	Space      string `json:"space"`
	UID        string `json:"uid,omitempty"`
	Generation string `json:"generation,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

// Condition is one closed portable status condition.
type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	HostReason         string `json:"hostReason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

// Status carries the observed generation, the closed conditions, and the
// Form's output document. There is no observed document in this lane: the
// envelope owns status, and what a host observed reached a consumer through
// outputs already.
type Status struct {
	ObservedGeneration string         `json:"observedGeneration"`
	Conditions         []Condition    `json:"conditions"`
	Outputs            map[string]any `json:"outputs,omitempty"`
}

// Resource is the v1beta1 resource envelope. Spec is kept generic so the
// same transport carries every Service Form; the provider's resource layer
// owns the per-shape spec contents.
type Resource struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Form       *FormReference `json:"form,omitempty"`
	Metadata   Metadata       `json:"metadata"`
	Spec       map[string]any `json:"spec,omitempty"`
	Status     *Status        `json:"status,omitempty"`
}

// Fence carries the optimistic-concurrency expectations of one apply. An
// empty ExpectedGeneration means create (If-None-Match: *); a non-empty one
// means update fenced by Takoform-Expected-Generation. ExpectedUID
// optionally pins the host-issued identity; mismatch fails uid_mismatch.
type Fence struct {
	ExpectedUID        string
	ExpectedGeneration string
}

// Diagnostic is one validate finding.
type Diagnostic struct {
	Severity string `json:"severity"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

// ValidateResult reports validation diagnostics. Validation performs no
// mutation and issues no prepare digest.
type ValidateResult struct {
	Valid       bool         `json:"valid"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// PrepareReview binds the exact spec, identity, and fences to a short-lived
// prepare digest; substitution after prepare fails invalid_argument before
// mutation.
type PrepareReview struct {
	PrepareDigest string `json:"prepareDigest"`
	SpecDigest    string `json:"specDigest"`
}

// PrepareResult echoes the prepared resource and its review fence.
type PrepareResult struct {
	Resource Resource      `json:"resource"`
	Review   PrepareReview `json:"review"`
}

var (
	// The version segment is OPTIONAL. A family group carries one only while
	// it has a published generation to keep apart; a group minted without one
	// is a whole identity on its own, because kind, definitionVersion and
	// schemaDigest already say which contract it is (decision 0049).
	groupVersionPattern = regexp.MustCompile(
		`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+(?:/v[0-9]+(?:(?:alpha|beta)[0-9]+)?)?$`,
	)
	kindPattern              = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
	definitionVersionPattern = regexp.MustCompile(
		`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)` +
			`(-((0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?` +
			`(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$`,
	)
	resourceNamePattern = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	uidPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	conditionTypes      = map[string]struct{}{
		"Ready": {}, "Reconciling": {}, "Degraded": {}, "Drifted": {}, "Blocked": {}, "Deleting": {},
	}
	conditionStatuses = map[string]struct{}{"True": {}, "False": {}, "Unknown": {}}
)

// ValidateFormRef enforces the v1beta1 exact FormRef grammar. The apex
// forms.takoform.com group is refused at any version: it belongs to the Host
// API and the withdrawn pre-family epochs, while a Form group is a namespaced
// family (decision 0009) — a subdomain of forms.takoform.com or a third
// party's own domain. Refusing the apex wholesale means no withdrawn epoch
// has to be named here for the next one to be refused.
func ValidateFormRef(ref FormRef) error {
	if len(ref.APIVersion) > 320 || !groupVersionPattern.MatchString(ref.APIVersion) {
		return errors.New("takoform: FormRef apiVersion must be a namespaced group/version")
	}
	if strings.HasPrefix(ref.APIVersion, "forms.takoform.com/") {
		return fmt.Errorf("takoform: FormRef group %s is the apex Host API group, not a Form family", ref.APIVersion)
	}
	if !kindPattern.MatchString(ref.Kind) {
		return errors.New("takoform: FormRef kind must match ^[A-Z][A-Za-z0-9]{0,63}$")
	}
	if !definitionVersionPattern.MatchString(ref.DefinitionVersion) {
		return errors.New("takoform: FormRef definitionVersion must be a semantic version")
	}
	if !formpackage.ValidDigest(ref.SchemaDigest) {
		return errors.New("takoform: FormRef schemaDigest must be a lowercase sha256:<hex> digest")
	}
	return nil
}

// ValidateResourceName enforces the canonical portable name grammar.
// Resource names are stable wire identities, so callers must not trim,
// normalize, or case-fold them.
func ValidateResourceName(value string) error {
	if !resourceNamePattern.MatchString(value) {
		return errors.New("takoform: resource name must match ^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$")
	}
	return nil
}

// ValidateIdempotencyKey enforces the Host API idempotency-key grammar. The
// wire contract measures this value in bytes, not Unicode code points: every
// byte must be visible ASCII (0x21-0x7e), and the complete key must be between
// one and 255 bytes. Callers must pass the exact value they intend to send;
// this validator deliberately does not trim, normalize, or otherwise rewrite
// it.
func ValidateIdempotencyKey(value string) error {
	if len(value) < 1 || len(value) > 255 {
		return errors.New("takoform: Idempotency-Key must contain 1..255 visible ASCII bytes")
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return errors.New("takoform: Idempotency-Key must contain only visible ASCII bytes (0x21-0x7e)")
		}
	}
	return nil
}

// spaceIDMaxLength is measured in Unicode code points, matching JSON
// Schema's maxLength semantics for the normative host wire contract.
const spaceIDMaxLength = 255

// ValidateSpaceID enforces the portable Space identity contract without
// normalizing it: opaque, case-sensitive UTF-8; embedded whitespace is data;
// boundary whitespace (including U+FEFF), controls, and slash are forbidden.
func ValidateSpaceID(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("SpaceID must be valid UTF-8")
	}
	length := utf8.RuneCountInString(value)
	if length < 1 || length > spaceIDMaxLength {
		return fmt.Errorf("SpaceID must contain between 1 and %d Unicode code points", spaceIDMaxLength)
	}
	first, _ := utf8.DecodeRuneInString(value)
	last, _ := utf8.DecodeLastRuneInString(value)
	if isSpaceIDBoundaryWhitespace(first) || isSpaceIDBoundaryWhitespace(last) {
		return errors.New("SpaceID must not start or end with whitespace")
	}
	for _, candidate := range value {
		switch {
		case candidate == '/':
			return errors.New("SpaceID must not contain '/'")
		case isSpaceIDControl(candidate):
			return errors.New("SpaceID must not contain control characters")
		}
	}
	return nil
}

// These explicit code-point sets mirror the normative JSON Schema. U+FEFF is
// treated as boundary whitespace even though modern Unicode removed it from
// White_Space; a BOM at an identity boundary is never useful.
func isSpaceIDBoundaryWhitespace(candidate rune) bool {
	switch {
	case candidate >= '\u0009' && candidate <= '\u000d':
		return true
	case candidate == '\u0020',
		candidate == '\u0085',
		candidate == '\u00a0',
		candidate == '\u1680',
		candidate >= '\u2000' && candidate <= '\u200a',
		candidate == '\u2028',
		candidate == '\u2029',
		candidate == '\u202f',
		candidate == '\u205f',
		candidate == '\u3000',
		candidate == '\ufeff':
		return true
	default:
		return false
	}
}

func isSpaceIDControl(candidate rune) bool {
	return candidate >= '\u0000' && candidate <= '\u001f' ||
		candidate >= '\u007f' && candidate <= '\u009f'
}

// validCanonicalDecimal reports whether value is a canonical positive
// decimal in 1..9223372036854775807, the shared generation/revision grammar.
func validCanonicalDecimal(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0
}

// sameFormRef reports whether two exact FormRefs carry the same contract
// identity. The packageDigest is excluded by design: it is distribution
// provenance, not contract identity, and a host that installed the same
// FormRef from a different legitimate package serves the same resources.
func sameFormRef(left, right FormRef) bool {
	return left.APIVersion == right.APIVersion &&
		left.Kind == right.Kind &&
		left.DefinitionVersion == right.DefinitionVersion &&
		left.SchemaDigest == right.SchemaDigest
}

// validateRequestResource fails closed before any wire traffic when the
// caller-supplied resource does not carry a coherent v1beta1 identity.
func validateRequestResource(resource *Resource) error {
	if resource == nil || resource.Form == nil {
		return errors.New("takoform: v1beta1 resource requires an exact FormRef")
	}
	if err := ValidateFormRef(resource.Form.FormRef); err != nil {
		return err
	}
	if resource.Form.PackageDigest != "" && !formpackage.ValidDigest(resource.Form.PackageDigest) {
		return errors.New("takoform: form.packageDigest must be a lowercase sha256:<hex> digest when present")
	}
	if resource.APIVersion != resource.Form.FormRef.APIVersion || resource.Kind != resource.Form.FormRef.Kind {
		return errors.New("takoform: resource apiVersion/kind and exact FormRef identities do not match")
	}
	if err := ValidateResourceName(resource.Metadata.Name); err != nil {
		return fmt.Errorf("takoform: resource metadata.name is invalid: %w", err)
	}
	if err := ValidateSpaceID(resource.Metadata.Space); err != nil {
		return fmt.Errorf("takoform: resource metadata.space has invalid SpaceID: %w", err)
	}
	return nil
}

// verifyResourceResponse enforces the response-side identity contract: the
// host may not change apiVersion, kind, name, space, or the exact FormRef
// (packageDigest excluded); uid/generation/revision must be present and
// canonical; status with observedGeneration and closed conditions must be
// present.
func verifyResourceResponse(ref FormRef, expectedName, expectedSpace string, got *Resource) error {
	if got == nil || got.Form == nil || !sameFormRef(ref, got.Form.FormRef) {
		return errors.New("takoform: host response changed the exact FormRef identity")
	}
	if got.APIVersion != ref.APIVersion || got.Kind != ref.Kind {
		return errors.New("takoform: host response changed the resource identity")
	}
	if got.Metadata.Name != expectedName || got.Metadata.Space != expectedSpace {
		return errors.New("takoform: host response changed the requested resource name or space")
	}
	if !uidPattern.MatchString(got.Metadata.UID) {
		return errors.New("takoform: host response omitted a valid metadata.uid")
	}
	if !validCanonicalDecimal(got.Metadata.Generation) {
		return errors.New("takoform: host response omitted a canonical metadata.generation")
	}
	if !validCanonicalDecimal(got.Metadata.Revision) {
		return errors.New("takoform: host response omitted a canonical metadata.revision")
	}
	if got.Status == nil {
		return errors.New("takoform: host response omitted status")
	}
	if !validCanonicalDecimal(got.Status.ObservedGeneration) {
		return errors.New("takoform: host response omitted a canonical status.observedGeneration")
	}
	if got.Status.Conditions == nil {
		return errors.New("takoform: host response omitted status.conditions")
	}
	for _, condition := range got.Status.Conditions {
		if _, known := conditionTypes[condition.Type]; !known {
			return fmt.Errorf("takoform: host response carries unknown condition type %q", condition.Type)
		}
		if _, known := conditionStatuses[condition.Status]; !known {
			return fmt.Errorf("takoform: host response carries unknown condition status %q", condition.Status)
		}
	}
	return nil
}

// ResourceCondition returns the first condition of the given closed type, or
// nil when absent.
func ResourceCondition(resource *Resource, conditionType string) *Condition {
	if resource == nil || resource.Status == nil {
		return nil
	}
	for index := range resource.Status.Conditions {
		if resource.Status.Conditions[index].Type == conditionType {
			return &resource.Status.Conditions[index]
		}
	}
	return nil
}

// ResourceReady reports whether the Ready condition is present with status
// True.
func ResourceReady(resource *Resource) bool {
	condition := ResourceCondition(resource, "Ready")
	return condition != nil && condition.Status == "True"
}

func quoteRevision(revision string) string { return `"` + revision + `"` }

// requireRevisionETag enforces the strong-validator contract: exactly one
// ETag header whose value is the quoted metadata.revision.
func requireRevisionETag(headerValues []string, revision string) error {
	if len(headerValues) != 1 {
		return errors.New("takoform: host response must return exactly one ETag representation revision")
	}
	if headerValues[0] != quoteRevision(revision) {
		return errors.New("takoform: host response ETag does not match the representation revision")
	}
	return nil
}

// validWireDigest and digestCanonical delegate to formpackage's stable
// canonical-JSON helpers so the digest grammar has one implementation.
func validWireDigest(digest string) bool { return formpackage.ValidDigest(digest) }

func digestCanonical(raw []byte) (string, error) { return formpackage.DigestCanonicalJSON(raw) }
