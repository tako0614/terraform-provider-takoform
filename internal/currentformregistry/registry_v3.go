package currentformregistry

import (
	"errors"
	"fmt"
	"sort"
)

// V3Ref identifies one exact Form Family candidate Definition and package for
// the provider-v3 (v1alpha3 wire) lane. Unlike the v1alpha2 Ref, its
// APIVersion is a namespaced family group, so one kind name can exist in
// several groups without colliding.
type V3Ref struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	PackageDigest     string `json:"packageDigest"`
}

// ExactFormKey is the WHOLE contract identity of one Form Definition. State
// dispatch looks up this complete tuple and never a prefix of it: a Form line
// that advances its definition version is a different key, so both the old and
// the new definition can be supported at once
// (spec/decisions/0017-provider-state-survives-form-evolution-and-interruption.md).
type ExactFormKey struct {
	APIVersion        string
	Kind              string
	DefinitionVersion string
	SchemaDigest      string
}

// GroupKind is the (namespaced group, kind) pair one Form line lives under. It
// is the key of the create DEFAULT only; it is never the key of state.
type GroupKind struct {
	APIVersion string
	Kind       string
}

// ExactKey projects one ref onto its contract identity. The packageDigest is
// excluded by design: distribution provenance is audit evidence, never
// identity (spec/decisions/0011).
func (ref V3Ref) ExactKey() ExactFormKey {
	return ExactFormKey{
		APIVersion:        ref.APIVersion,
		Kind:              ref.Kind,
		DefinitionVersion: ref.DefinitionVersion,
		SchemaDigest:      ref.SchemaDigest,
	}
}

// GroupKind is the line this exact identity belongs to.
func (key ExactFormKey) GroupKind() GroupKind {
	return GroupKind{APIVersion: key.APIVersion, Kind: key.Kind}
}

// String renders one exact identity the way diagnostics name it.
func (key ExactFormKey) String() string {
	return fmt.Sprintf("%s/%s@%s schema=%s", key.APIVersion, key.Kind, key.DefinitionVersion, key.SchemaDigest)
}

// V3Registry is one immutable snapshot of the exact Form identities a build
// can serve. It holds two independent facts:
//
//   - Supported: every exact identity this build can read, observe, update, and
//     delete. State dispatches on membership here, so a Form line that has
//     advanced keeps its older definitions readable as long as they stay
//     registered.
//   - DefaultCreates: the ONE identity a new resource of each group+kind is
//     created under. It is a recommendation about new resources; it says
//     nothing about what existing state is bound to.
//
// Today each group+kind has exactly one supported identity, so the two maps
// have the same cardinality. The structure exists so that stops being true
// without a state migration.
type V3Registry struct {
	defaultCreates map[GroupKind]ExactFormKey
	supported      map[ExactFormKey]V3Ref
}

// v3Current is the registry this provider build carries, assembled from the
// generated data. It is built once and never mutated; Register returns copies.
var v3Current = newV3Registry(v3DefaultCreates, v3Supported)

func newV3Registry(defaults map[GroupKind]ExactFormKey, supported map[ExactFormKey]V3Ref) *V3Registry {
	registry := &V3Registry{
		defaultCreates: make(map[GroupKind]ExactFormKey, len(defaults)),
		supported:      make(map[ExactFormKey]V3Ref, len(supported)),
	}
	for groupKind, key := range defaults {
		registry.defaultCreates[groupKind] = key
	}
	for key, ref := range supported {
		registry.supported[key] = ref
	}
	return registry
}

// V3Current returns the exact-FormRef registry of this provider build.
func V3Current() *V3Registry { return v3Current }

// Register returns a COPY of the registry that also supports ref. When
// asDefaultCreate is true the ref also becomes the create target of its
// group+kind. The receiver is never modified, so the build's own registry
// cannot be reshaped at a distance.
//
// Registering an identity that is already supported under different package
// provenance is refused: two package digests for one exact contract identity
// would make `form_package_digest` depend on lookup order.
func (r *V3Registry) Register(ref V3Ref, asDefaultCreate bool) (*V3Registry, error) {
	if ref.APIVersion == "" || ref.Kind == "" || ref.DefinitionVersion == "" || ref.SchemaDigest == "" {
		return nil, errors.New("takoform: a registered v3 FormRef must carry group, kind, definitionVersion, and schemaDigest")
	}
	key := ref.ExactKey()
	if existing, present := r.supported[key]; present && existing != ref {
		return nil, fmt.Errorf("takoform: exact Form identity %s is already registered with different package provenance", key)
	}
	next := newV3Registry(r.defaultCreates, r.supported)
	next.supported[key] = ref
	if asDefaultCreate {
		next.defaultCreates[key.GroupKind()] = key
	}
	return next, nil
}

// DefaultCreate returns the recommended create target of one group+kind.
func (r *V3Registry) DefaultCreate(groupKind GroupKind) (V3Ref, error) {
	key, ok := r.defaultCreates[groupKind]
	if !ok {
		return V3Ref{}, fmt.Errorf(
			"takoform: provider v3 has no default create Form for %s kind %q",
			groupKind.APIVersion, groupKind.Kind,
		)
	}
	ref, ok := r.supported[key]
	if !ok {
		return V3Ref{}, fmt.Errorf("takoform: default create Form %s is not in the supported set", key)
	}
	return ref, nil
}

// Lookup resolves one EXACT recorded identity. A miss is the fail-closed
// signal: the caller must never fall back to another identity of the same
// group+kind.
func (r *V3Registry) Lookup(key ExactFormKey) (V3Ref, bool) {
	ref, ok := r.supported[key]
	return ref, ok
}

// SupportedRefs returns every exact FormRef this registry can serve, in a
// stable order.
func (r *V3Registry) SupportedRefs() []V3Ref {
	return sortedRefs(r.supported, func(ExactFormKey) bool { return true })
}

// SupportedRefsFor returns every exact FormRef of one group+kind, in a stable
// order. A fail-closed diagnostic names exactly this set.
func (r *V3Registry) SupportedRefsFor(groupKind GroupKind) []V3Ref {
	return sortedRefs(r.supported, func(key ExactFormKey) bool { return key.GroupKind() == groupKind })
}

// SupportedRefsForKind is SupportedRefsFor across every group carrying that
// kind. State written by an older provider build may name another group, so a
// diagnostic about a kind names every identity of that kind the build knows.
func (r *V3Registry) SupportedRefsForKind(kind string) []V3Ref {
	return sortedRefs(r.supported, func(key ExactFormKey) bool { return key.Kind == kind })
}

func sortedRefs(supported map[ExactFormKey]V3Ref, keep func(ExactFormKey) bool) []V3Ref {
	out := make([]V3Ref, 0, len(supported))
	for key, ref := range supported {
		if keep(key) {
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].ExactKey(), out[j].ExactKey()
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
	return out
}

// V3ForKind returns the recommended create target for one exact family kind.
// State written by an older FormRef of the same kind stays readable through
// V3Current().Lookup when a Form line advances.
func V3ForKind(apiVersion, kind string) (V3Ref, error) {
	return v3Current.DefaultCreate(GroupKind{APIVersion: apiVersion, Kind: kind})
}

// V3SupportedFormRefs returns every exact family FormRef this provider build
// can read, observe, update, and delete, in a stable order.
func V3SupportedFormRefs() []V3Ref { return v3Current.SupportedRefs() }
