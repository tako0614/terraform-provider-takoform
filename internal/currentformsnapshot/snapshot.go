// Package currentformsnapshot compiles already acquired, data-only Form
// Package artifacts into one immutable, provider-neutral exact-identity graph.
//
// It deliberately imports no built-in publisher family, Terraform Provider, Host, or
// conformance implementation. Acquisition, publisher trust, package-signature
// admission, and executable Host adapters are separate owners.
package currentformsnapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/blang/semver"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// DiagnosticCode is one stable compiler failure class. Callers may branch on
// Code, not Message; Message exists for people and may become clearer.
type DiagnosticCode string

const (
	DiagnosticInvalidInput        DiagnosticCode = "invalid_input"
	DiagnosticInvalidArtifact     DiagnosticCode = "invalid_artifact"
	DiagnosticDigestMismatch      DiagnosticCode = "digest_mismatch"
	DiagnosticDuplicateIdentity   DiagnosticCode = "duplicate_identity"
	DiagnosticUnresolvedReference DiagnosticCode = "unresolved_reference"
	DiagnosticAmbiguousDefault    DiagnosticCode = "ambiguous_default"
	DiagnosticMissingDefault      DiagnosticCode = "missing_default"
	DiagnosticUnsupportedHostAPI  DiagnosticCode = "unsupported_host_api"
)

// Diagnostic identifies one deterministic compiler failure. Subject is an
// exact FormRef when available and otherwise the artifact Origin (or a digest
// derived from its bytes). Pointer is an RFC 6901 pointer when the failure is
// inside a document.
type Diagnostic struct {
	Code    DiagnosticCode `json:"code"`
	Subject string         `json:"subject"`
	Pointer string         `json:"pointer,omitempty"`
	Message string         `json:"message"`
}

// PackageArtifact carries the non-forgeable result of Core's complete package
// verifier plus the caller's explicit digest pin. Acquisition fetches bytes;
// formpackage.VerifyDirectory closes every listed payload, fixture, file-mode,
// content-policy, schema, and identity invariant before it can issue Package.
//
// Origin is diagnostics-only and never participates in FormRef equality or
// Snapshot identity.
type PackageArtifact struct {
	Origin         string
	ExpectedDigest string
	Package        formpackage.VerifiedPackage
}

// DefaultPin explicitly selects the create identity of one group+kind. It is
// separate from the supported exact-identity set so advancing a create default
// never makes older recorded state unreadable.
type DefaultPin struct {
	Group string              `json:"group"`
	Kind  string              `json:"kind"`
	Ref   formpackage.FormRef `json:"ref"`
}

// Input is the complete, order-independent compiler input.
type Input struct {
	HostAPI        string
	Packages       []PackageArtifact
	Interfaces     []InterfaceArtifact
	Bindings       []BindingArtifact
	DefaultCreates []DefaultPin
}

// Form is the immutable public view of one compiled exact Form identity.
type Form struct {
	Ref           formpackage.FormRef `json:"ref"`
	PackageDigest string              `json:"packageDigest"`
}

type exactKey struct {
	group, kind, version, digest string
}

func keyFor(ref formpackage.FormRef) exactKey {
	return exactKey{ref.APIVersion, ref.Kind, ref.DefinitionVersion, ref.SchemaDigest}
}

func (key exactKey) ref() formpackage.FormRef {
	return formpackage.FormRef{
		APIVersion:        key.group,
		Kind:              key.kind,
		DefinitionVersion: key.version,
		SchemaDigest:      key.digest,
	}
}

func (key exactKey) String() string {
	return fmt.Sprintf("%s/%s@%s schema=%s", key.group, key.kind, key.version, key.digest)
}

type groupKind struct {
	group, kind string
}

type compiledForm struct {
	public     Form
	definition formpackage.FormDefinition
	canonical  []byte
	origin     string
}

// Snapshot is a complete immutable result. Its fields are private, every byte
// input is copied, and every slice or byte view returned to callers is copied.
// A failed Compile returns no Snapshot.
type Snapshot struct {
	hostAPI        string
	digest         string
	ordered        []exactKey
	forms          map[exactKey]compiledForm
	interfaceOrder []interfaceKey
	interfaces     map[interfaceKey]compiledInterface
	bindingOrder   []bindingKey
	bindings       map[bindingKey]compiledBinding
	defaults       map[groupKind]exactKey
}

var hostAPIPattern = regexp.MustCompile(`^forms\.takoform\.com/v[0-9]+(?:(?:alpha|beta)[0-9]+)?$`)

// hostAPILaneOrder is the closed set of Host API lanes this Core build knows
// how to compare. Withdrawn and future-looking identities are not guessed
// into an order: a syntactically plausible unknown lane fails closed.
var hostAPILaneOrder = map[string]int{
	"forms.takoform.com/v1beta1": 1,
	"forms.takoform.com/v1beta4": 2,
	"forms.takoform.com/v1":      3,
}

// selectableHostAPILanes is the subset of known lanes this Core may compile
// and serve today. Historical lanes remain in hostAPILaneOrder so lower-bound
// comparisons stay deterministic, but retaining comparison knowledge does not
// make an unserved lane selectable.
var selectableHostAPILanes = map[string]struct{}{
	"forms.takoform.com/v1": {},
}

// Compile verifies and closes one exact artifact graph. Compilation is staged:
// a failed stage returns stable diagnostics immediately, so later failures do
// not depend on which invalid artifact happened to be visited first.
func Compile(input Input) (*Snapshot, []Diagnostic) {
	if !hostAPIPattern.MatchString(input.HostAPI) {
		return nil, []Diagnostic{{
			Code:    DiagnosticInvalidInput,
			Subject: "input",
			Pointer: "/hostApi",
			Message: fmt.Sprintf("HostAPI %q is not a Takoform Host API lane identity", input.HostAPI),
		}}
	}
	if _, known := hostAPILaneOrder[input.HostAPI]; !known {
		return nil, []Diagnostic{{
			Code:    DiagnosticInvalidInput,
			Subject: "input",
			Pointer: "/hostApi",
			Message: fmt.Sprintf("HostAPI %q is not a Host API lane known by this Core build", input.HostAPI),
		}}
	}
	if _, selectable := selectableHostAPILanes[input.HostAPI]; !selectable {
		return nil, []Diagnostic{{
			Code:    DiagnosticUnsupportedHostAPI,
			Subject: "input",
			Pointer: "/hostApi",
			Message: fmt.Sprintf("HostAPI %q is a historical or unserved lane; only forms.takoform.com/v1 is currently selectable", input.HostAPI),
		}}
	}

	diagnostics := make([]Diagnostic, 0)
	candidates := make(map[exactKey][]compiledForm)
	for _, artifact := range input.Packages {
		compiled, artifactDiagnostics := compileArtifact(artifact)
		diagnostics = append(diagnostics, artifactDiagnostics...)
		if len(artifactDiagnostics) == 0 {
			key := keyFor(compiled.public.Ref)
			candidates[key] = append(candidates[key], compiled)
		}
	}
	if len(diagnostics) != 0 {
		return nil, sortedDiagnostics(diagnostics)
	}

	forms := make(map[exactKey]compiledForm, len(candidates))
	keys := sortedKeys(candidates)
	for _, key := range keys {
		matches := candidates[key]
		if len(matches) != 1 {
			origins := make([]string, 0, len(matches))
			for _, match := range matches {
				origins = append(origins, match.origin)
			}
			sort.Strings(origins)
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticDuplicateIdentity,
				Subject: key.String(),
				Message: fmt.Sprintf("exact Form identity is supplied %d times by %s", len(matches), strings.Join(origins, ", ")),
			})
			continue
		}
		forms[key] = matches[0]
	}
	if len(diagnostics) != 0 {
		return nil, sortedDiagnostics(diagnostics)
	}

	interfaces, interfaceOrder, bindings, bindingOrder, contractDiagnostics := compileContracts(input.Interfaces, input.Bindings)
	if len(contractDiagnostics) != 0 {
		return nil, sortedDiagnostics(contractDiagnostics)
	}
	if referenceDiagnostics := closeContractReferences(forms, keys, interfaces, bindings); len(referenceDiagnostics) != 0 {
		return nil, sortedDiagnostics(referenceDiagnostics)
	}

	for _, key := range keys {
		required := forms[key].definition.RequiresHostAPI
		if required == "" {
			continue
		}
		if !hostAPISatisfies(input.HostAPI, required) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticUnsupportedHostAPI,
				Subject: key.String(),
				Pointer: "/requiresHostApi",
				Message: fmt.Sprintf("Form requires Host API %s, but this Snapshot serves %s", required, input.HostAPI),
			})
		}
	}
	if len(diagnostics) != 0 {
		return nil, sortedDiagnostics(diagnostics)
	}

	for _, key := range keys {
		for _, target := range targetRefs(forms[key].definition.DesiredSchema, "/desiredSchema") {
			if _, ok := forms[keyFor(target.ref)]; !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Code:    DiagnosticUnresolvedReference,
					Subject: key.String(),
					Pointer: target.pointer,
					Message: fmt.Sprintf("exact target Form %s is absent from the Snapshot", keyFor(target.ref)),
				})
			}
		}
	}
	if len(diagnostics) != 0 {
		return nil, sortedDiagnostics(diagnostics)
	}

	defaults, defaultDiagnostics := compileDefaults(input.DefaultCreates, keys, forms)
	if len(defaultDiagnostics) != 0 {
		return nil, sortedDiagnostics(defaultDiagnostics)
	}

	digest, err := snapshotDigest(input.HostAPI, keys, forms, interfaceOrder, interfaces, bindingOrder, bindings, defaults)
	if err != nil {
		return nil, []Diagnostic{{
			Code:    DiagnosticInvalidInput,
			Subject: "input",
			Message: fmt.Sprintf("encode normalized Snapshot: %v", err),
		}}
	}
	return &Snapshot{
		hostAPI:        input.HostAPI,
		digest:         digest,
		ordered:        append([]exactKey(nil), keys...),
		forms:          forms,
		interfaceOrder: append([]interfaceKey(nil), interfaceOrder...),
		interfaces:     interfaces,
		bindingOrder:   append([]bindingKey(nil), bindingOrder...),
		bindings:       bindings,
		defaults:       defaults,
	}, nil
}

func hostAPISatisfies(served, required string) bool {
	servedRank, servedKnown := hostAPILaneOrder[served]
	requiredRank, requiredKnown := hostAPILaneOrder[required]
	return servedKnown && requiredKnown && servedRank >= requiredRank
}

func compileArtifact(artifact PackageArtifact) (compiledForm, []Diagnostic) {
	subject := artifactSubject(artifact)
	if !artifact.Package.Valid() {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticInvalidArtifact,
			Subject: subject,
			Pointer: "/package",
			Message: "package was not issued by the complete Form Package verifier",
		}}
	}
	if !formpackage.ValidDigest(artifact.ExpectedDigest) {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticInvalidArtifact,
			Subject: subject,
			Pointer: "/expectedDigest",
			Message: "expected package digest is not a canonical lowercase sha256 digest",
		}}
	}
	actualPackageDigest := artifact.Package.PackageDigest()
	if actualPackageDigest != artifact.ExpectedDigest {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticDigestMismatch,
			Subject: subject,
			Pointer: "/package-index.json",
			Message: fmt.Sprintf("package index digest is %s, expected %s", actualPackageDigest, artifact.ExpectedDigest),
		}}
	}
	index := artifact.Package.PackageIndex()
	definitionRaw := artifact.Package.Definition()

	_, ok := definitionEntry(index)
	if !ok {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticInvalidArtifact,
			Subject: subject,
			Pointer: "/package-index.json/definitionPath",
			Message: "definitionPath does not name the one Form Definition payload",
		}}
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticInvalidArtifact,
			Subject: subject,
			Pointer: "/" + escapePointer(index.DefinitionPath),
			Message: err.Error(),
		}}
	}
	definitionDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticInvalidArtifact,
			Subject: subject,
			Pointer: "/" + escapePointer(index.DefinitionPath),
			Message: err.Error(),
		}}
	}
	definitionRef := formpackage.FormRef{
		APIVersion:        definition.APIVersion,
		Kind:              definition.Kind,
		DefinitionVersion: definition.DefinitionVersion,
		SchemaDigest:      definitionDigest,
	}
	if definitionRef != index.FormRef {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticDigestMismatch,
			Subject: subject,
			Pointer: "/package-index.json/formRef",
			Message: fmt.Sprintf("package FormRef %#v does not match Definition %#v", index.FormRef, definitionRef),
		}}
	}
	canonical, err := formpackage.Canonicalize(definitionRaw)
	if err != nil {
		return compiledForm{}, []Diagnostic{{
			Code:    DiagnosticInvalidArtifact,
			Subject: subject,
			Pointer: "/" + escapePointer(index.DefinitionPath),
			Message: err.Error(),
		}}
	}
	return compiledForm{
		public: Form{
			Ref:           index.FormRef,
			PackageDigest: artifact.ExpectedDigest,
		},
		definition: definition,
		canonical:  append([]byte(nil), canonical...),
		origin:     subject,
	}, nil
}

func definitionEntry(index formpackage.PackageIndex) (formpackage.PackageFile, bool) {
	for _, file := range index.Files {
		if file.Path == index.DefinitionPath && file.MediaType == formpackage.DefinitionMediaType {
			return file, true
		}
	}
	return formpackage.PackageFile{}, false
}

func compileDefaults(pins []DefaultPin, keys []exactKey, forms map[exactKey]compiledForm) (map[groupKind]exactKey, []Diagnostic) {
	byGroupKind := make(map[groupKind][]DefaultPin)
	for _, pin := range pins {
		group := groupKind{pin.Group, pin.Kind}
		byGroupKind[group] = append(byGroupKind[group], pin)
	}

	diagnostics := make([]Diagnostic, 0)
	defaults := make(map[groupKind]exactKey)
	allGroups := make(map[groupKind]struct{})
	for _, key := range keys {
		allGroups[groupKind{key.group, key.kind}] = struct{}{}
	}
	for group := range byGroupKind {
		allGroups[group] = struct{}{}
	}
	groups := make([]groupKind, 0, len(allGroups))
	for group := range allGroups {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].group != groups[j].group {
			return groups[i].group < groups[j].group
		}
		return groups[i].kind < groups[j].kind
	})

	for _, group := range groups {
		groupPins := byGroupKind[group]
		subject := group.group + "/" + group.kind
		if len(groupPins) == 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticMissingDefault,
				Subject: subject,
				Message: "supported group+kind has no explicit default-create FormRef",
			})
			continue
		}
		if len(groupPins) != 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticAmbiguousDefault,
				Subject: subject,
				Message: fmt.Sprintf("group+kind has %d default-create pins; exactly one is required", len(groupPins)),
			})
			continue
		}
		pin := groupPins[0]
		if pin.Group != pin.Ref.APIVersion || pin.Kind != pin.Ref.Kind {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticAmbiguousDefault,
				Subject: subject,
				Message: "default-create group+kind does not match its exact FormRef",
			})
			continue
		}
		key := keyFor(pin.Ref)
		if _, ok := forms[key]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticUnresolvedReference,
				Subject: subject,
				Message: fmt.Sprintf("default-create exact Form %s is absent from the Snapshot", key),
			})
			continue
		}
		defaults[group] = key
	}
	return defaults, diagnostics
}

type targetRef struct {
	pointer string
	ref     formpackage.FormRef
}

func targetRefs(value any, pointer string) []targetRef {
	var output []targetRef
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPointer := pointer + "/" + escapePointer(key)
			if key == "x-takoform-target-formrefs" {
				if entries, ok := typed[key].([]any); ok {
					for index, entry := range entries {
						encoded, err := json.Marshal(entry)
						if err != nil {
							continue
						}
						var ref formpackage.FormRef
						if err := formpackage.DecodeStrictIJSON(encoded, &ref); err == nil {
							output = append(output, targetRef{
								pointer: fmt.Sprintf("%s/%d", childPointer, index),
								ref:     ref,
							})
						}
					}
				}
				continue
			}
			output = append(output, targetRefs(typed[key], childPointer)...)
		}
	case []any:
		for index, entry := range typed {
			output = append(output, targetRefs(entry, fmt.Sprintf("%s/%d", pointer, index))...)
		}
	}
	return output
}

func snapshotDigest(
	hostAPI string,
	keys []exactKey,
	forms map[exactKey]compiledForm,
	interfaceOrder []interfaceKey,
	interfaces map[interfaceKey]compiledInterface,
	bindingOrder []bindingKey,
	bindings map[bindingKey]compiledBinding,
	defaults map[groupKind]exactKey,
) (string, error) {
	type wireDefault struct {
		Group string              `json:"group"`
		Kind  string              `json:"kind"`
		Ref   formpackage.FormRef `json:"ref"`
	}
	type wireSnapshot struct {
		Format     string        `json:"format"`
		HostAPI    string        `json:"hostApi"`
		Forms      []Form        `json:"forms"`
		Interfaces []Interface   `json:"interfaces"`
		Bindings   []Binding     `json:"bindings"`
		Defaults   []wireDefault `json:"defaults"`
	}
	wire := wireSnapshot{
		Format:  "takoform.compiled-snapshot@v1",
		HostAPI: hostAPI,
		Forms:   make([]Form, 0, len(keys)),
	}
	for _, key := range keys {
		wire.Forms = append(wire.Forms, forms[key].public)
	}
	for _, key := range interfaceOrder {
		wire.Interfaces = append(wire.Interfaces, interfaces[key].public)
	}
	for _, key := range bindingOrder {
		wire.Bindings = append(wire.Bindings, bindings[key].public)
	}
	groups := make([]groupKind, 0, len(defaults))
	for group := range defaults {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].group != groups[j].group {
			return groups[i].group < groups[j].group
		}
		return groups[i].kind < groups[j].kind
	})
	for _, group := range groups {
		wire.Defaults = append(wire.Defaults, wireDefault{
			Group: group.group,
			Kind:  group.kind,
			Ref:   defaults[group].ref(),
		})
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

func sortedKeys[T any](values map[exactKey]T) []exactKey {
	keys := make([]exactKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return lessKey(keys[i], keys[j]) })
	return keys
}

func lessKey(left, right exactKey) bool {
	if left.group != right.group {
		return left.group < right.group
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	leftVersion, leftErr := semver.Parse(left.version)
	rightVersion, rightErr := semver.Parse(right.version)
	if leftErr == nil && rightErr == nil && !leftVersion.Equals(rightVersion) {
		return leftVersion.LT(rightVersion)
	}
	if left.version != right.version {
		return left.version < right.version
	}
	return left.digest < right.digest
}

func sortedDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	ordered := append([]Diagnostic(nil), diagnostics...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Subject != ordered[j].Subject {
			return ordered[i].Subject < ordered[j].Subject
		}
		if ordered[i].Pointer != ordered[j].Pointer {
			return ordered[i].Pointer < ordered[j].Pointer
		}
		if ordered[i].Code != ordered[j].Code {
			return ordered[i].Code < ordered[j].Code
		}
		return ordered[i].Message < ordered[j].Message
	})
	return ordered
}

func artifactSubject(artifact PackageArtifact) string {
	if strings.TrimSpace(artifact.Origin) != "" {
		return artifact.Origin
	}
	if artifact.Package.Valid() {
		return "package:" + artifact.Package.PackageDigest()
	}
	return "package:unverified"
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

// HostAPI returns the exact Host API lane the Snapshot was compiled for.
func (snapshot *Snapshot) HostAPI() string {
	if snapshot == nil {
		return ""
	}
	return snapshot.hostAPI
}

// Digest returns the deterministic digest of exact Forms, package provenance,
// default pins, and Host API selection. Artifact Origin is excluded.
func (snapshot *Snapshot) Digest() string {
	if snapshot == nil {
		return ""
	}
	return snapshot.digest
}

// Forms returns every compiled exact Form in stable group, kind, SemVer and
// digest order.
func (snapshot *Snapshot) Forms() []Form {
	if snapshot == nil {
		return nil
	}
	output := make([]Form, 0, len(snapshot.ordered))
	for _, key := range snapshot.ordered {
		output = append(output, snapshot.forms[key].public)
	}
	return output
}

// Default resolves only an exact group+kind create selection. It never falls
// back by Kind or to a latest version.
func (snapshot *Snapshot) Default(group, kind string) (formpackage.FormRef, bool) {
	if snapshot == nil {
		return formpackage.FormRef{}, false
	}
	key, ok := snapshot.defaults[groupKind{group, kind}]
	if !ok {
		return formpackage.FormRef{}, false
	}
	return key.ref(), true
}

// Definition returns a copy of the RFC 8785 canonical Definition bytes for one
// exact FormRef. A wrong group, version, or digest is a miss.
func (snapshot *Snapshot) Definition(ref formpackage.FormRef) ([]byte, bool) {
	if snapshot == nil {
		return nil, false
	}
	form, ok := snapshot.forms[keyFor(ref)]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), form.canonical...), true
}

// Validate checks one I-JSON desired document against the schema carried by an
// exact compiled FormRef.
func (snapshot *Snapshot) Validate(ref formpackage.FormRef, desired []byte) error {
	if snapshot == nil {
		return errors.New("takoform: nil compiled Snapshot")
	}
	form, ok := snapshot.forms[keyFor(ref)]
	if !ok {
		return fmt.Errorf("takoform: exact Form %s is absent from the compiled Snapshot", keyFor(ref))
	}
	var value any
	if err := formpackage.DecodeStrictIJSON(desired, &value); err != nil {
		return fmt.Errorf("takoform: desired document: %w", err)
	}
	if err := formpackage.ValidateDesiredInstance(form.definition.DesiredSchema, value); err != nil {
		return fmt.Errorf("takoform: exact Form %s: %w", keyFor(ref), err)
	}
	return nil
}

// Materialize returns the canonical effective desired document for one exact
// FormRef. Schema defaults are inserted recursively before exact-schema
// validation, so omitted and explicitly written defaults have identical wire
// bytes. Neither the caller's bytes nor the compiled Definition are mutated.
func (snapshot *Snapshot) Materialize(ref formpackage.FormRef, desired []byte) ([]byte, error) {
	if snapshot == nil {
		return nil, errors.New("takoform: nil compiled Snapshot")
	}
	form, ok := snapshot.forms[keyFor(ref)]
	if !ok {
		return nil, fmt.Errorf("takoform: exact Form %s is absent from the compiled Snapshot", keyFor(ref))
	}
	var value any
	if err := formpackage.DecodeStrictIJSON(desired, &value); err != nil {
		return nil, fmt.Errorf("takoform: desired document: %w", err)
	}
	materialized := materializeSchemaValue(form.definition.DesiredSchema, value)
	if err := formpackage.ValidateDesiredInstance(form.definition.DesiredSchema, materialized); err != nil {
		return nil, fmt.Errorf("takoform: exact Form %s: %w", keyFor(ref), err)
	}
	raw, err := json.Marshal(materialized)
	if err != nil {
		return nil, fmt.Errorf("takoform: encode materialized desired document: %w", err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return nil, fmt.Errorf("takoform: canonicalize materialized desired document: %w", err)
	}
	return append([]byte(nil), canonical...), nil
}

func materializeSchemaValue(schema map[string]any, value any) any {
	if discriminator, _ := schema["x-takoform-discriminator"].(string); discriminator != "" {
		object, ok := value.(map[string]any)
		if !ok {
			return cloneJSONValue(value)
		}
		tag, _ := object[discriminator].(string)
		branches, _ := schema["oneOf"].([]any)
		for _, rawBranch := range branches {
			branch, _ := rawBranch.(map[string]any)
			properties, _ := branch["properties"].(map[string]any)
			tagSchema, _ := properties[discriminator].(map[string]any)
			declared, _ := tagSchema["const"].(string)
			if declared == tag {
				return materializeSchemaValue(branch, object)
			}
		}
		return cloneJSONValue(value)
	}

	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return cloneJSONValue(value)
		}
		properties, _ := schema["properties"].(map[string]any)
		additional, _ := schema["additionalProperties"].(map[string]any)
		output := make(map[string]any, len(object)+len(properties))
		for key, item := range object {
			child, _ := properties[key].(map[string]any)
			if child == nil {
				child = additional
			}
			if child == nil {
				output[key] = cloneJSONValue(item)
				continue
			}
			output[key] = materializeSchemaValue(child, item)
		}
		for name, rawProperty := range properties {
			if _, present := output[name]; present {
				continue
			}
			property, _ := rawProperty.(map[string]any)
			declared, hasDefault := property["default"]
			if !hasDefault {
				continue
			}
			output[name] = materializeSchemaValue(property, cloneJSONValue(declared))
		}
		return output
	case "array":
		items, ok := value.([]any)
		if !ok {
			return cloneJSONValue(value)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		output := make([]any, 0, len(items))
		for _, item := range items {
			if itemSchema == nil {
				output = append(output, cloneJSONValue(item))
				continue
			}
			output = append(output, materializeSchemaValue(itemSchema, item))
		}
		return output
	default:
		return cloneJSONValue(value)
	}
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		output := make(map[string]any, len(typed))
		for key, item := range typed {
			output[key] = cloneJSONValue(item)
		}
		return output
	case []any:
		output := make([]any, 0, len(typed))
		for _, item := range typed {
			output = append(output, cloneJSONValue(item))
		}
		return output
	default:
		return typed
	}
}
