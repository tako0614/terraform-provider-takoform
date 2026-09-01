package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	providerV3ProjectionFormat        = "takoform.provider-v3-projection@v1"
	providerPublisherProjectionFormat = "takoform.provider-publisher-set-projection@v1"
	providerV3HostAPI                 = "forms.takoform.com/v1"

	v3ProjectionCurrent    = "current"
	v3ProjectionRetained   = "retained-readable"
	v3ProjectionUnreadable = "retained-unreadable"

	v3ArtifactModeWorkerBundle = "worker-bundle"
	v3ArtifactModeFileBundle   = "file-bundle"

	providerV3CurrentFormCount     = 31
	providerV3RetainedFormCount    = 15
	providerV3UnreadableFormCount  = 1
	providerV3ReadableFormCount    = 45
	providerV3ResourceMappingCount = 45
)

type v3ProjectionContract struct {
	format           string
	currentForms     int
	retainedForms    int
	unreadableForms  int
	readableForms    int
	resourceMappings int
}

var (
	providerV3ProjectionContract = v3ProjectionContract{
		format: providerV3ProjectionFormat, currentForms: providerV3CurrentFormCount,
		retainedForms: providerV3RetainedFormCount, unreadableForms: providerV3UnreadableFormCount,
		readableForms: providerV3ReadableFormCount, resourceMappings: providerV3ResourceMappingCount,
	}
	providerPublisherProjectionContract = v3ProjectionContract{
		format: providerPublisherProjectionFormat, currentForms: publisherProviderFormCount,
		retainedForms: 15, unreadableForms: 0, readableForms: 32, resourceMappings: 32,
	}
)

// v3ProviderProjection is the Provider-owned immutable Terraform projection.
// Form Definitions deliberately do not carry any of these client facts. The
// exact Form model is included because Definition bytes alone cannot recover
// HCL names, collection/set choices, provider defaults, validators or the
// retained state codec declarations shipped by Provider 3.0.0.
type v3ProviderProjection struct {
	Format         string                `json:"format"`
	HostAPI        string                `json:"hostApi"`
	Forms          []v3ProjectedForm     `json:"forms"`
	Resources      []v3ProjectedResource `json:"resources"`
	DefaultCreates []v3FormRef           `json:"defaultCreates"`
	ReadableRefs   []v3FormRef           `json:"readableRefs"`
}

// v3ProjectedForm owns the exact authoring declaration used by Terraform.
// Current Definition bytes come only from the compiled Snapshot. A retained
// entry additionally carries its immutable canonical Definition because it is
// outside the current 31-package Snapshot but remains state-readable history.
type v3ProjectedForm struct {
	Generation string          `json:"generation"`
	Ref        v3FormRef       `json:"ref"`
	Form       model.Form      `json:"form"`
	Definition json.RawMessage `json:"definition,omitempty"`
}

// v3ProjectedResource is one exact FormRef-to-Terraform mapping. Register is
// true only for the 31 current resources exposed by Provider 3. Retained
// readable refs reuse a resource type solely for exact state dispatch and are
// never separately registered.
type v3ProjectedResource struct {
	Ref               v3FormRef             `json:"ref"`
	ResourceType      string                `json:"resourceType"`
	Register          bool                  `json:"register"`
	RegistrationOrder *int                  `json:"registrationOrder,omitempty"`
	Artifact          *v3ArtifactProjection `json:"artifact,omitempty"`
}

// v3ArtifactProjection selects the bounded Provider-only local authoring
// implementation for an exact current resource. The portable Definition still
// carries only manifestDigest; these fields own the Terraform file/module
// convenience and its exact manifest constraints.
type v3ArtifactProjection struct {
	Mode            string   `json:"mode"`
	ManifestKind    string   `json:"manifestKind"`
	MaximumFiles    int      `json:"maximumFiles"`
	MaximumFileSize int64    `json:"maximumFileSize"`
	MediaTypes      []string `json:"mediaTypes,omitempty"`
}

func cloneV3ArtifactProjection(source *v3ArtifactProjection) *v3ArtifactProjection {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.MediaTypes = append([]string(nil), source.MediaTypes...)
	return &cloned
}

func v3ProjectionContains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (r *v3FormResource) v3ArtifactRule() (*v3ArtifactProjection, bool) {
	if r == nil || r.artifact == nil {
		return nil, false
	}
	return cloneV3ArtifactProjection(r.artifact), true
}

func v3FormRequiresArtifactRule(form model.Form) bool {
	return len(form.Fields) == 1 && form.Fields[0].HCL == "manifest_digest" &&
		form.Fields[0].Wire == "manifestDigest" && form.Fields[0].Kind == model.KindString && form.Fields[0].Required
}

func (r *v3FormResource) v3WorkerBundleArtifact() (*v3ArtifactProjection, bool) {
	artifact, ok := r.v3ArtifactRule()
	return artifact, ok && artifact.Mode == v3ArtifactModeWorkerBundle
}

func (r *v3FormResource) v3FileBundleArtifact() (*v3ArtifactProjection, bool) {
	artifact, ok := r.v3ArtifactRule()
	return artifact, ok && artifact.Mode == v3ArtifactModeFileBundle
}

func (r *v3FormResource) v3ArtifactBackedRevision() bool {
	_, ok := r.v3ArtifactRule()
	return ok
}

type v3ProjectionIndex struct {
	document           v3ProviderProjection
	forms              map[v3ExactFormKey]v3ProjectedForm
	resources          map[v3ExactFormKey]v3ProjectedResource
	defaults           map[v3GroupKind]v3FormRef
	readable           map[v3ExactFormKey]v3FormRef
	currentOrder       []v3ExactFormKey
	currentKeys        map[v3ExactFormKey]struct{}
	retainedKeys       map[v3ExactFormKey]struct{}
	unreadableRetained map[v3ExactFormKey]struct{}
}

var projectedTerraformResourceTypePattern = regexp.MustCompile(`^takoform_[a-z0-9_]+$`)

func decodeProviderV3Projection(raw []byte) (*v3ProjectionIndex, error) {
	return decodeProviderProjection(raw, providerV3ProjectionContract)
}

func decodePublisherProviderProjection(raw []byte) (*v3ProjectionIndex, error) {
	return decodeProviderProjection(raw, providerPublisherProjectionContract)
}

func decodeProviderProjection(raw []byte, contract v3ProjectionContract) (*v3ProjectionIndex, error) {
	var document v3ProviderProjection
	if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
		return nil, fmt.Errorf("takoform provider: decode Provider 3 projection: %w", err)
	}
	if document.Format != contract.format {
		return nil, fmt.Errorf("takoform provider: projection format %q, want %q", document.Format, contract.format)
	}
	if document.HostAPI != providerV3HostAPI {
		return nil, fmt.Errorf("takoform provider: projection Host API %q, want %q", document.HostAPI, providerV3HostAPI)
	}
	index := &v3ProjectionIndex{
		document:           document,
		forms:              make(map[v3ExactFormKey]v3ProjectedForm, len(document.Forms)),
		resources:          make(map[v3ExactFormKey]v3ProjectedResource, len(document.Resources)),
		defaults:           make(map[v3GroupKind]v3FormRef, len(document.DefaultCreates)),
		readable:           make(map[v3ExactFormKey]v3FormRef, len(document.ReadableRefs)),
		currentKeys:        make(map[v3ExactFormKey]struct{}),
		retainedKeys:       make(map[v3ExactFormKey]struct{}),
		unreadableRetained: make(map[v3ExactFormKey]struct{}),
	}

	for position := range document.Forms {
		entry := document.Forms[position]
		normalizeProjectedForm(&entry.Form)
		key := entry.Ref.ExactKey()
		if err := validateProjectedRef(entry.Ref); err != nil {
			return nil, fmt.Errorf("takoform provider: projection forms[%d]: %w", position, err)
		}
		if _, duplicate := index.forms[key]; duplicate {
			return nil, fmt.Errorf("takoform provider: projection duplicate exact FormRef %s", key)
		}
		if entry.Form.Family.APIVersion() != entry.Ref.APIVersion || entry.Form.Kind != entry.Ref.Kind ||
			entry.Form.DefinitionVersion != entry.Ref.DefinitionVersion {
			return nil, fmt.Errorf("takoform provider: projected Form model identity %s/%s@%s does not match exact ref %s",
				entry.Form.Family.APIVersion(), entry.Form.Kind, entry.Form.DefinitionVersion, key)
		}
		if err := entry.Form.Validate(); err != nil {
			return nil, fmt.Errorf("takoform provider: projected Form %s is invalid: %w", key, err)
		}
		switch entry.Generation {
		case v3ProjectionCurrent:
			if len(entry.Definition) != 0 {
				return nil, fmt.Errorf("takoform provider: current projected Form %s duplicates Snapshot Definition bytes", key)
			}
			index.currentKeys[key] = struct{}{}
		case v3ProjectionRetained, v3ProjectionUnreadable:
			if len(entry.Definition) == 0 {
				return nil, fmt.Errorf("takoform provider: retained projected Form %s has no exact Definition", key)
			}
			if err := validateRetainedProjectionDefinition(entry); err != nil {
				return nil, err
			}
			index.retainedKeys[key] = struct{}{}
			if entry.Generation == v3ProjectionUnreadable {
				index.unreadableRetained[key] = struct{}{}
			}
		default:
			return nil, fmt.Errorf("takoform provider: projected Form %s has unknown generation %q", key, entry.Generation)
		}
		index.forms[key] = entry
		index.document.Forms[position] = entry
	}
	if len(index.currentKeys) != contract.currentForms || len(index.retainedKeys) != contract.retainedForms ||
		len(index.unreadableRetained) != contract.unreadableForms || len(index.forms) != contract.currentForms+contract.retainedForms {
		return nil, fmt.Errorf(
			"takoform provider: projection Form history is %d current/%d retained/%d unreadable (%d total), want %d/%d/%d (%d total)",
			len(index.currentKeys), len(index.retainedKeys), len(index.unreadableRetained), len(index.forms),
			contract.currentForms, contract.retainedForms, contract.unreadableForms,
			contract.currentForms+contract.retainedForms,
		)
	}

	registeredOrders := make(map[int]v3ExactFormKey)
	registeredTypes := make(map[string]v3ExactFormKey)
	artifactRules, workerArtifactRules, fileArtifactRules := 0, 0, 0
	for position, resource := range document.Resources {
		key := resource.Ref.ExactKey()
		if err := validateProjectedRef(resource.Ref); err != nil {
			return nil, fmt.Errorf("takoform provider: projection resources[%d]: %w", position, err)
		}
		form, exists := index.forms[key]
		if !exists || form.Ref != resource.Ref {
			return nil, fmt.Errorf("takoform provider: projection resource mapping for extra or mismatched exact FormRef %s", key)
		}
		if _, duplicate := index.resources[key]; duplicate {
			return nil, fmt.Errorf("takoform provider: projection duplicate resource mapping for exact FormRef %s", key)
		}
		if !projectedTerraformResourceTypePattern.MatchString(resource.ResourceType) {
			return nil, fmt.Errorf("takoform provider: projected resource type %q for %s is invalid", resource.ResourceType, key)
		}
		if resource.Register {
			if form.Generation != v3ProjectionCurrent {
				return nil, fmt.Errorf("takoform provider: retained Form %s is marked for Provider 3 registration", key)
			}
			if resource.RegistrationOrder == nil || *resource.RegistrationOrder < 0 {
				return nil, fmt.Errorf("takoform provider: registered resource %s has no non-negative registration order", key)
			}
			if prior, duplicate := registeredOrders[*resource.RegistrationOrder]; duplicate {
				return nil, fmt.Errorf("takoform provider: registration order %d is shared by %s and %s", *resource.RegistrationOrder, prior, key)
			}
			registeredOrders[*resource.RegistrationOrder] = key
			if prior, duplicate := registeredTypes[resource.ResourceType]; duplicate {
				return nil, fmt.Errorf("takoform provider: registered resource type %q maps both %s and %s", resource.ResourceType, prior, key)
			}
			registeredTypes[resource.ResourceType] = key
		} else if resource.RegistrationOrder != nil {
			return nil, fmt.Errorf("takoform provider: unregistered retained mapping %s declares a registration order", key)
		}
		if err := validateArtifactProjection(resource, form); err != nil {
			return nil, err
		}
		if resource.Artifact != nil {
			artifactRules++
			switch resource.Artifact.Mode {
			case v3ArtifactModeWorkerBundle:
				workerArtifactRules++
			case v3ArtifactModeFileBundle:
				fileArtifactRules++
			}
		}
		index.resources[key] = resource
	}
	if len(index.resources) != contract.resourceMappings {
		return nil, fmt.Errorf("takoform provider: projection has %d exact resource mappings, want %d", len(index.resources), contract.resourceMappings)
	}
	if artifactRules != 3 || workerArtifactRules != 1 || fileArtifactRules != 2 {
		return nil, fmt.Errorf("takoform provider: projection has %d artifact rules (%d worker/%d file), want 3 (1/2)", artifactRules, workerArtifactRules, fileArtifactRules)
	}

	if len(registeredOrders) != len(index.currentKeys) {
		return nil, fmt.Errorf("takoform provider: projection has %d registered resource types for %d current exact refs", len(registeredOrders), len(index.currentKeys))
	}
	index.currentOrder = make([]v3ExactFormKey, len(registeredOrders))
	for order := range index.currentOrder {
		key, ok := registeredOrders[order]
		if !ok {
			return nil, fmt.Errorf("takoform provider: projection registration order is missing position %d", order)
		}
		index.currentOrder[order] = key
	}

	for position, ref := range document.DefaultCreates {
		if err := validateProjectedRef(ref); err != nil {
			return nil, fmt.Errorf("takoform provider: projection defaultCreates[%d]: %w", position, err)
		}
		key := ref.ExactKey()
		form, exists := index.forms[key]
		if !exists || form.Ref != ref || form.Generation != v3ProjectionCurrent {
			return nil, fmt.Errorf("takoform provider: default-create ref %s is not one exact current Form", key)
		}
		groupKind := key.GroupKind()
		if prior, duplicate := index.defaults[groupKind]; duplicate {
			return nil, fmt.Errorf("takoform provider: duplicate default-create refs %s and %s for %s/%s", prior.ExactKey(), key, groupKind.APIVersion, groupKind.Kind)
		}
		index.defaults[groupKind] = ref
	}
	if len(index.defaults) != len(index.currentKeys) {
		return nil, fmt.Errorf("takoform provider: projection has %d default-create refs for %d current exact refs", len(index.defaults), len(index.currentKeys))
	}
	if len(index.defaults) != contract.currentForms {
		return nil, fmt.Errorf("takoform provider: projection has %d default-create refs, want %d", len(index.defaults), contract.currentForms)
	}

	for position, ref := range document.ReadableRefs {
		if err := validateProjectedRef(ref); err != nil {
			return nil, fmt.Errorf("takoform provider: projection readableRefs[%d]: %w", position, err)
		}
		key := ref.ExactKey()
		form, exists := index.forms[key]
		if !exists || form.Ref != ref || form.Generation == v3ProjectionUnreadable {
			return nil, fmt.Errorf("takoform provider: readable ref %s is extra, mismatched, or explicitly unreadable", key)
		}
		if _, duplicate := index.readable[key]; duplicate {
			return nil, fmt.Errorf("takoform provider: duplicate readable exact FormRef %s", key)
		}
		index.readable[key] = ref
	}
	if len(index.resources) != len(index.readable) {
		return nil, fmt.Errorf("takoform provider: projection has %d resource mappings for %d readable exact refs", len(index.resources), len(index.readable))
	}
	if len(index.readable) != contract.readableForms {
		return nil, fmt.Errorf("takoform provider: projection has %d readable exact refs, want %d", len(index.readable), contract.readableForms)
	}
	for key, ref := range index.readable {
		resource, ok := index.resources[key]
		if !ok || resource.Ref != ref {
			return nil, fmt.Errorf("takoform provider: readable exact FormRef %s has no exact resource type mapping", key)
		}
	}
	for key := range index.resources {
		if _, ok := index.readable[key]; !ok {
			return nil, fmt.Errorf("takoform provider: resource type mapping for %s is extra to the readable exact-ref set", key)
		}
	}
	for key := range index.currentKeys {
		if _, ok := index.defaults[key.GroupKind()]; !ok {
			return nil, fmt.Errorf("takoform provider: current exact FormRef %s is missing a default-create selection", key)
		}
		if _, ok := index.readable[key]; !ok {
			return nil, fmt.Errorf("takoform provider: current exact FormRef %s is missing from readable history", key)
		}
		resource, ok := index.resources[key]
		if !ok || !resource.Register {
			return nil, fmt.Errorf("takoform provider: current exact FormRef %s is missing a registered resource type", key)
		}
	}
	for key := range index.unreadableRetained {
		if _, mapped := index.resources[key]; mapped {
			return nil, fmt.Errorf("takoform provider: explicitly unreadable retained Form %s has a resource mapping", key)
		}
		if _, readable := index.readable[key]; readable {
			return nil, fmt.Errorf("takoform provider: explicitly unreadable retained Form %s has a codec", key)
		}
	}
	return index, nil
}

func validateProjectedRef(ref v3FormRef) error {
	if strings.TrimSpace(ref.APIVersion) == "" || strings.TrimSpace(ref.Kind) == "" || strings.TrimSpace(ref.DefinitionVersion) == "" {
		return fmt.Errorf("exact FormRef is incomplete: %#v", ref)
	}
	if !formpackage.ValidDigest(ref.SchemaDigest) || !formpackage.ValidDigest(ref.PackageDigest) {
		return fmt.Errorf("exact FormRef %s has a non-canonical schema or package digest", ref.ExactKey())
	}
	return nil
}

func validateRetainedProjectionDefinition(entry v3ProjectedForm) error {
	if _, err := formpackage.ValidateDefinition(entry.Definition); err != nil {
		return fmt.Errorf("takoform provider: retained Definition for %s is invalid: %w", entry.Ref.ExactKey(), err)
	}
	digest, err := formpackage.DigestCanonicalJSON(entry.Definition)
	if err != nil {
		return fmt.Errorf("takoform provider: digest retained Definition for %s: %w", entry.Ref.ExactKey(), err)
	}
	if digest != entry.Ref.SchemaDigest {
		return fmt.Errorf("takoform provider: retained Definition for %s has digest %s", entry.Ref.ExactKey(), digest)
	}
	definition, err := formpackage.ValidateDefinition(entry.Definition)
	if err != nil {
		return err
	}
	if definition.APIVersion != entry.Ref.APIVersion || definition.Kind != entry.Ref.Kind || definition.DefinitionVersion != entry.Ref.DefinitionVersion {
		return fmt.Errorf("takoform provider: retained Definition identity does not match %s", entry.Ref.ExactKey())
	}
	return nil
}

func validateArtifactProjection(resource v3ProjectedResource, form v3ProjectedForm) error {
	if resource.Artifact == nil {
		return nil
	}
	if form.Generation != v3ProjectionCurrent || !resource.Register {
		return fmt.Errorf("takoform provider: artifact authoring rule is attached to non-current resource %s", resource.Ref.ExactKey())
	}
	artifact := resource.Artifact
	if len(form.Form.Fields) != 1 || form.Form.Fields[0].HCL != "manifest_digest" || form.Form.Fields[0].Wire != "manifestDigest" ||
		form.Form.Fields[0].Kind != model.KindString || !form.Form.Fields[0].Required {
		return fmt.Errorf("takoform provider: artifact authoring rule for %s is not attached to an exact required manifestDigest revision", resource.Ref.ExactKey())
	}
	if artifact.MaximumFiles <= 0 || artifact.MaximumFileSize <= 0 || strings.TrimSpace(artifact.ManifestKind) == "" {
		return fmt.Errorf("takoform provider: artifact authoring rule for %s is incomplete", resource.Ref.ExactKey())
	}
	switch artifact.Mode {
	case v3ArtifactModeWorkerBundle:
		if len(artifact.MediaTypes) == 0 {
			return fmt.Errorf("takoform provider: Worker Bundle artifact rule for %s has no closed media types", resource.Ref.ExactKey())
		}
	case v3ArtifactModeFileBundle:
		// Empty MediaTypes means the generic normalized media-type grammar;
		// a non-empty set is a stricter exact allowlist such as application/sql.
	default:
		return fmt.Errorf("takoform provider: artifact authoring rule for %s has unknown mode %q", resource.Ref.ExactKey(), artifact.Mode)
	}
	seen := make(map[string]struct{}, len(artifact.MediaTypes))
	for _, mediaType := range artifact.MediaTypes {
		if strings.TrimSpace(mediaType) == "" {
			return fmt.Errorf("takoform provider: artifact authoring rule for %s has an empty media type", resource.Ref.ExactKey())
		}
		if _, duplicate := seen[mediaType]; duplicate {
			return fmt.Errorf("takoform provider: artifact authoring rule for %s repeats media type %q", resource.Ref.ExactKey(), mediaType)
		}
		seen[mediaType] = struct{}{}
	}
	return nil
}

func normalizeProjectedForm(form *model.Form) {
	for index := range form.Fields {
		normalizeProjectedField(&form.Fields[index])
	}
	for index := range form.Outputs {
		normalizeProjectedField(&form.Outputs[index])
	}
}

func normalizeProjectedField(field *model.Field) {
	field.Default = normalizeProjectedValue(field.Default)
	field.Example = normalizeProjectedValue(field.Example)
	field.AltExample = normalizeProjectedValue(field.AltExample)
	field.CounterExample = normalizeProjectedValue(field.CounterExample)
	for index := range field.Fields {
		normalizeProjectedField(&field.Fields[index])
	}
	for variantIndex := range field.Variants {
		for fieldIndex := range field.Variants[variantIndex].Fields {
			normalizeProjectedField(&field.Variants[variantIndex].Fields[fieldIndex])
		}
	}
}

func normalizeProjectedValue(value any) any {
	switch typed := value.(type) {
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		floating, err := typed.Float64()
		if err == nil {
			return floating
		}
		return typed
	case []any:
		for index := range typed {
			typed[index] = normalizeProjectedValue(typed[index])
		}
		return typed
	case map[string]any:
		for key := range typed {
			typed[key] = normalizeProjectedValue(typed[key])
		}
		return typed
	default:
		return value
	}
}

// v3FormRegistry is the provider-local exact-ref seam backed by the immutable
// Provider projection. Form identity is always resolved through this seam;
// the removed generated registry is not a runtime fallback.
type v3FormRegistry interface {
	DefaultCreate(v3GroupKind) (v3FormRef, error)
	Lookup(v3ExactFormKey) (v3FormRef, bool)
	SupportedRefs() []v3FormRef
	SupportedRefsFor(v3GroupKind) []v3FormRef
	SupportedRefsForKind(string) []v3FormRef
}

type v3ProjectedRegistry struct {
	defaultCreates map[v3GroupKind]v3ExactFormKey
	supported      map[v3ExactFormKey]v3FormRef
}

func v3RegistryWithRef(registry v3FormRegistry, ref v3FormRef, asDefaultCreate bool) (v3FormRegistry, error) {
	typed, ok := registry.(*v3ProjectedRegistry)
	if !ok {
		return nil, fmt.Errorf("takoform: unsupported exact Form registry implementation %T", registry)
	}
	return typed.withRef(ref, asDefaultCreate)
}

func newV3ProjectedRegistry(index *v3ProjectionIndex) *v3ProjectedRegistry {
	registry := &v3ProjectedRegistry{
		defaultCreates: make(map[v3GroupKind]v3ExactFormKey, len(index.defaults)),
		supported:      make(map[v3ExactFormKey]v3FormRef, len(index.forms)),
	}
	for groupKind, ref := range index.defaults {
		registry.defaultCreates[groupKind] = ref.ExactKey()
	}
	for key, entry := range index.forms {
		registry.supported[key] = entry.Ref
	}
	return registry
}

func (registry *v3ProjectedRegistry) DefaultCreate(groupKind v3GroupKind) (v3FormRef, error) {
	key, ok := registry.defaultCreates[groupKind]
	if !ok {
		return v3FormRef{}, fmt.Errorf("takoform: provider v3 has no default create Form for %s kind %q", groupKind.APIVersion, groupKind.Kind)
	}
	ref, ok := registry.supported[key]
	if !ok {
		return v3FormRef{}, fmt.Errorf("takoform: default create Form %s is not in the supported set", key)
	}
	return ref, nil
}

func (registry *v3ProjectedRegistry) Lookup(key v3ExactFormKey) (v3FormRef, bool) {
	ref, ok := registry.supported[key]
	return ref, ok
}

func (registry *v3ProjectedRegistry) SupportedRefs() []v3FormRef {
	return registry.sortedRefs(func(v3ExactFormKey) bool { return true })
}

func (registry *v3ProjectedRegistry) SupportedRefsFor(groupKind v3GroupKind) []v3FormRef {
	return registry.sortedRefs(func(key v3ExactFormKey) bool { return key.GroupKind() == groupKind })
}

func (registry *v3ProjectedRegistry) SupportedRefsForKind(kind string) []v3FormRef {
	return registry.sortedRefs(func(key v3ExactFormKey) bool { return key.Kind == kind })
}

func (registry *v3ProjectedRegistry) sortedRefs(keep func(v3ExactFormKey) bool) []v3FormRef {
	refs := make([]v3FormRef, 0, len(registry.supported))
	for key, ref := range registry.supported {
		if keep(key) {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		left, right := refs[i].ExactKey(), refs[j].ExactKey()
		if left.APIVersion != right.APIVersion {
			return left.APIVersion < right.APIVersion
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.DefinitionVersion != right.DefinitionVersion {
			return left.DefinitionVersion < right.DefinitionVersion
		}
		return left.SchemaDigest < right.SchemaDigest
	})
	return refs
}

func (registry *v3ProjectedRegistry) withRef(ref v3FormRef, asDefaultCreate bool) (*v3ProjectedRegistry, error) {
	if err := validateProjectedRef(ref); err != nil {
		return nil, err
	}
	next := &v3ProjectedRegistry{
		defaultCreates: make(map[v3GroupKind]v3ExactFormKey, len(registry.defaultCreates)+1),
		supported:      make(map[v3ExactFormKey]v3FormRef, len(registry.supported)+1),
	}
	for groupKind, key := range registry.defaultCreates {
		next.defaultCreates[groupKind] = key
	}
	for key, existing := range registry.supported {
		next.supported[key] = existing
	}
	key := ref.ExactKey()
	if existing, ok := next.supported[key]; ok && existing != ref {
		return nil, fmt.Errorf("takoform: exact Form identity %s is already registered with different package provenance", key)
	}
	next.supported[key] = ref
	if asDefaultCreate {
		next.defaultCreates[key.GroupKind()] = key
	}
	return next, nil
}
