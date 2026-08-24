package provider

// v3_codec.go carries the per-exact-FormRef codecs of the Host API v1beta1
// resource lane.
//
// A FormRef recorded in state is a contract identity, and knowing the identity
// is not the same as being able to READ what was written under it: an older
// definition of the same kind may have had a different field set, so decoding
// that state needs the OLD ref's field declarations, not the current schema.
// Each supported exact ref therefore carries its own codec — the field set that
// exact definition declared — and the provider dispatches on the ref recorded
// in state (spec/decisions/0017).
//
// Compatibility rule (spec/versioning.md, "Form versions and provider
// codecs"): an ADDITIVE Form minor may share one codec with the definitions
// before it, because every earlier document stays valid with the same portable
// meaning and the added fields are simply absent from older specs. A BREAKING
// Form major needs its own codec, because a removed, retyped, or re-meant field
// cannot be encoded or decoded by the other definition's declarations.
//
// Where a supported ref names a field set this build does not compile, the
// provider fails closed naming the state ref and the refs it does know. It
// never reinterprets state under a different ref: that would silently rebind a
// resource to a contract its author never wrote.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

// v3FormCodec is one exact FormRef together with the Form declaration that
// decodes state written under it and encodes a spec for it.
type v3FormCodec struct {
	Ref           currentformregistry.V3Ref
	Form          model.Form
	DesiredSchema map[string]any
}

type v3CodecDeclaration struct {
	Form          model.Form
	DesiredSchema map[string]any
}

// v3CodecTable is the provider's exact-FormRef dispatch surface: the build's
// registry plus one codec per supported identity. It is immutable; withCodec
// returns a copy, so a build's own table can never be reshaped at a distance.
type v3CodecTable struct {
	registry *currentformregistry.V3Registry
	codecs   map[currentformregistry.ExactFormKey]v3CodecDeclaration
}

// v3Codecs is the table this provider build carries. Every supported exact ref
// resolves to the catalog Form with the same complete identity. It must not
// use ByKind: that helper intentionally returns the current default and would
// reinterpret a retained Beta ref as a future stable catalog entry when a
// Form line advances.
var v3Codecs = sync.OnceValue(func() *v3CodecTable {
	return newV3CodecTable(currentformregistry.V3Current())
})

func newV3CodecTable(registry *currentformregistry.V3Registry) *v3CodecTable {
	table := &v3CodecTable{
		registry: registry,
		codecs:   map[currentformregistry.ExactFormKey]v3CodecDeclaration{},
	}
	// The supported set spans the eight current families plus the frozen
	// retained v1beta1 generation (decision 0046). Every current family is
	// selected explicitly from the generated current-family projection; the
	// retained catalog is appended separately so the historical lane remains
	// byte/identity independent of current Forms.
	generations := make([]catalogGeneration, 0, len(providerV3CurrentFamilies())+1)
	for _, family := range providerV3CurrentFamilies() {
		generation := catalogGeneration{family: family.family, forms: family.forms}
		generation.rendered, generation.renderErr = family.render()
		generations = append(generations, generation)
	}
	generations = append(generations, catalogGeneration{
		family: retainededgeformcatalog.Family,
		forms:  retainededgeformcatalog.Forms,
	})
	retainedRendered, retainedErr := retainededgeformcatalog.RenderForms()
	retainedGeneration := &generations[len(generations)-1]
	retainedGeneration.rendered = make([]catalogRenderedForm, 0, len(retainedRendered))
	for _, form := range retainedRendered {
		retainedGeneration.rendered = append(retainedGeneration.rendered, catalogRenderedForm{DefinitionJSON: form.DefinitionJSON})
	}
	retainedGeneration.renderErr = retainedErr
	for _, ref := range registry.SupportedRefs() {
		// ObjectBucket is retained only as immutable Provider 2.1.1 source
		// history. Provider 3 deliberately carries no Terraform mapping or
		// state codec for it; legacy state must be disposed with Provider 2.1.1
		// before upgrading to this major line.
		if ref.APIVersion == retainededgeformcatalog.Family.APIVersion() && ref.Kind == "ObjectBucket" {
			continue
		}
		declaration, declared := formForExactRef(ref, generations)
		if !declared {
			// A supported identity whose field set is not compiled into this
			// build has no codec. It stays out of the table, so state bound to
			// it fails closed instead of being decoded by another kind's
			// declarations.
			continue
		}
		table.codecs[ref.ExactKey()] = declaration
	}
	return table
}

// catalogGeneration is one compiled declaration set the codec table may
// dispatch into: the family it declares, its Forms, and their rendered bytes.
type catalogGeneration struct {
	family    model.Family
	forms     []model.Form
	rendered  []catalogRenderedForm
	renderErr error
}

type catalogRenderedForm struct {
	DefinitionJSON string
}

// formForExactRef keeps the Form declaration selected by the same exact
// identity as provider state. Two compiled generations may each declare the
// same kind; matching the family, definition version, and rendered schema
// digest binds each exact ref to the one declaration whose published bytes it
// names, so a retained v1beta1 ref can never be decoded by the v1beta2
// declaration of the same kind, nor the reverse.
func formForExactRef(
	ref currentformregistry.V3Ref,
	generations []catalogGeneration,
) (v3CodecDeclaration, bool) {
	for _, generation := range generations {
		if generation.family.APIVersion() != ref.APIVersion {
			continue
		}
		if generation.renderErr != nil || len(generation.rendered) != len(generation.forms) {
			return v3CodecDeclaration{}, false
		}
		for index, candidate := range generation.forms {
			if candidate.Kind == ref.Kind &&
				candidate.DefinitionVersion == ref.DefinitionVersion {
				definitionJSON := []byte(generation.rendered[index].DefinitionJSON)
				// WorkerVersion and WorkerDeployment are the two retained
				// identities whose v1beta1 bytes must remain exactly those
				// shipped by provider 2.1.1. Decode them from the embedded
				// historical artifacts instead of letting current-only model
				// changes alter the old schema digest.
				if generation.family.APIVersion() == retainededgeformcatalog.Family.APIVersion() {
					if frozen, ok := retainedFrozenDefinition(candidate.Kind); ok {
						definitionJSON = frozen
					}
				}
				digest, err := formpackage.DigestCanonicalJSON(definitionJSON)
				if err != nil || digest != ref.SchemaDigest {
					continue
				}
				definition, err := formpackage.ValidateDefinition(definitionJSON)
				if err != nil {
					return v3CodecDeclaration{}, false
				}
				return v3CodecDeclaration{Form: candidate, DesiredSchema: definition.DesiredSchema}, true
			}
		}
	}
	return v3CodecDeclaration{}, false
}

// withCodec returns a copy of the table that also supports ref, decoded and
// encoded by form. asDefaultCreate makes it the create target of its
// group+kind.
func (t *v3CodecTable) withCodec(ref currentformregistry.V3Ref, form model.Form, asDefaultCreate bool) (*v3CodecTable, error) {
	registry, err := t.registry.Register(ref, asDefaultCreate)
	if err != nil {
		return nil, err
	}
	next := &v3CodecTable{
		registry: registry,
		codecs:   make(map[currentformregistry.ExactFormKey]v3CodecDeclaration, len(t.codecs)+1),
	}
	for key, existing := range t.codecs {
		next.codecs[key] = existing
	}
	desiredSchema, err := v3DesiredSchemaForCodec(form)
	if err != nil {
		return nil, fmt.Errorf("takoform: derive exact desired schema for %s: %w", ref.ExactKey(), err)
	}
	next.codecs[ref.ExactKey()] = v3CodecDeclaration{Form: form, DesiredSchema: desiredSchema}
	return next, nil
}

// defaultCreate resolves the codec a NEW resource of one kind is created
// under.
func (t *v3CodecTable) defaultCreate(groupKind currentformregistry.GroupKind) (v3FormCodec, error) {
	ref, err := t.registry.DefaultCreate(groupKind)
	if err != nil {
		return v3FormCodec{}, err
	}
	declaration, known := t.codecs[ref.ExactKey()]
	if !known {
		return v3FormCodec{}, fmt.Errorf("takoform: no codec is compiled for the create target %s", ref.ExactKey())
	}
	return v3FormCodec{Ref: ref, Form: declaration.Form, DesiredSchema: declaration.DesiredSchema}, nil
}

// forStateKey resolves the codec of one EXACT recorded identity. Membership —
// not equality with the current create target — decides dispatch, so state
// written before a Form line advanced stays readable. A miss is fatal by
// design: the caller must not query another identity of the same kind and read
// its 404 as deletion.
func (t *v3CodecTable) forStateKey(key currentformregistry.ExactFormKey) (v3FormCodec, bool) {
	ref, supported := t.registry.Lookup(key)
	if !supported {
		return v3FormCodec{}, false
	}
	declaration, known := t.codecs[key]
	if !known {
		return v3FormCodec{}, false
	}
	return v3FormCodec{Ref: ref, Form: declaration.Form, DesiredSchema: declaration.DesiredSchema}, true
}

// v3DesiredSchemaForCodec derives only the desired document shape for
// synthetic/additive test codecs. Production codecs above always retain the
// desiredSchema decoded from their exact rendered Definition. Contract
// annotations do not affect instance validation, so deterministic placeholder
// identities are sufficient here; resolved-UID constraints are deliberately
// removed because their cross-Form proof is installation semantics, not JSON
// shape.
func v3DesiredSchemaForCodec(form model.Form) (map[string]any, error) {
	shape := form
	shape.ResolvedUIDConstraints = nil
	return shape.DesiredSchema(v3SchemaOnlyTargetResolver{})
}

type v3SchemaOnlyTargetResolver struct{}

func (v3SchemaOnlyTargetResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	resolved := model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName}
	switch {
	case target.Contract.ExactForm && target.Contract.Interface == nil:
		resolved.TargetFormRefs = []model.TargetFormRef{{
			APIVersion: target.Group, Kind: target.Kind, DefinitionVersion: "0.0.0", SchemaDigest: digest,
		}}
	case !target.Contract.ExactForm && target.Contract.Interface != nil:
		resolved.RequiredInterface = &model.RequiredInterface{
			APIVersion: "interfaces.takoform.com/v1alpha1",
			Name:       target.Contract.Interface.Name, Version: target.Contract.Interface.Version, SchemaDigest: digest,
		}
	default:
		return model.ResolvedResourceTarget{}, fmt.Errorf("ResourceTarget %s/%s has no single contract", target.Group, target.Kind)
	}
	return resolved, nil
}

// knownRefsForKind is every exact identity of one kind this build can serve —
// exactly what a fail-closed diagnostic must name so the operator can see which
// provider version to pin.
func (t *v3CodecTable) knownRefsForKind(kind string) []currentformregistry.V3Ref {
	all := t.registry.SupportedRefsForKind(kind)
	out := make([]currentformregistry.V3Ref, 0, len(all))
	for _, ref := range all {
		if _, known := t.codecs[ref.ExactKey()]; known {
			out = append(out, ref)
		}
	}
	return out
}

// v3RenderRefs renders a supported-identity list for a diagnostic.
func v3RenderRefs(refs []currentformregistry.V3Ref) string {
	if len(refs) == 0 {
		return "none"
	}
	rendered := make([]string, 0, len(refs))
	for _, ref := range refs {
		rendered = append(rendered, ref.ExactKey().String())
	}
	return strings.Join(rendered, ", ")
}

// v3UnsupportedStateRefError is the one fail-closed diagnostic for state bound
// to an identity this build cannot decode. It names the recorded ref and every
// ref of that kind the build knows, and it never proposes a substitute: reading
// the resource under a different exact FormRef would reinterpret one contract
// as another, and a 404 from that query would then look like deletion.
func v3UnsupportedStateRefError(kind string, got v3FormRefValue, known []currentformregistry.V3Ref) diag.Diagnostic {
	return v3Diagnostic{
		Summary: "State Form identity is not supported by this provider",
		Ref: currentformregistry.V3Ref{
			APIVersion: got.APIVersion, Kind: got.Kind,
			DefinitionVersion: got.DefinitionVersion, SchemaDigest: got.SchemaDigest,
		},
		Pointer: "/form",
		Code:    v3CodeStateRefUnsupported,
		Detail: fmt.Sprintf(
			"This provider build carries %s codecs for %s. "+
				"The provider will not read, update, or delete this resource under a different exact FormRef, "+
				"because that would reinterpret state written against one contract as another.",
			kind, v3RenderRefs(known),
		),
		Repair: "Pin the provider version that wrote the state, or perform an explicit create/import migration " +
			"onto an identity this build supports.",
	}.error()
}
