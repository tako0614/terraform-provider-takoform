package provider

// v3_codec.go carries the per-exact-FormRef codecs of the Host API v1alpha3
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

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// v3FormCodec is one exact FormRef together with the Form declaration that
// decodes state written under it and encodes a spec for it.
type v3FormCodec struct {
	Ref  currentformregistry.V3Ref
	Form model.Form
}

// v3CodecTable is the provider's exact-FormRef dispatch surface: the build's
// registry plus one codec per supported identity. It is immutable; withCodec
// returns a copy, so a build's own table can never be reshaped at a distance.
type v3CodecTable struct {
	registry *currentformregistry.V3Registry
	codecs   map[currentformregistry.ExactFormKey]model.Form
}

// v3Codecs is the table this provider build carries. Every supported exact ref
// of the Edge Platform Family resolves to the catalog Form of its kind: today
// the registry holds exactly one definition version per kind, so the current
// declarations ARE that ref's field set.
var v3Codecs = sync.OnceValue(func() *v3CodecTable {
	registry := currentformregistry.V3Current()
	table := &v3CodecTable{
		registry: registry,
		codecs:   map[currentformregistry.ExactFormKey]model.Form{},
	}
	for _, ref := range registry.SupportedRefs() {
		form, declared := edgeformcatalog.ByKind(ref.Kind)
		if !declared {
			// A supported identity whose field set is not compiled into this
			// build has no codec. It stays out of the table, so state bound to
			// it fails closed instead of being decoded by another kind's
			// declarations.
			continue
		}
		table.codecs[ref.ExactKey()] = form
	}
	return table
})

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
		codecs:   make(map[currentformregistry.ExactFormKey]model.Form, len(t.codecs)+1),
	}
	for key, existing := range t.codecs {
		next.codecs[key] = existing
	}
	next.codecs[ref.ExactKey()] = form
	return next, nil
}

// defaultCreate resolves the codec a NEW resource of one kind is created
// under.
func (t *v3CodecTable) defaultCreate(kind string) (v3FormCodec, error) {
	groupKind := currentformregistry.GroupKind{APIVersion: edgeformcatalog.Family.APIVersion(), Kind: kind}
	ref, err := t.registry.DefaultCreate(groupKind)
	if err != nil {
		return v3FormCodec{}, err
	}
	form, known := t.codecs[ref.ExactKey()]
	if !known {
		return v3FormCodec{}, fmt.Errorf("takoform: no codec is compiled for the create target %s", ref.ExactKey())
	}
	return v3FormCodec{Ref: ref, Form: form}, nil
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
	form, known := t.codecs[key]
	if !known {
		return v3FormCodec{}, false
	}
	return v3FormCodec{Ref: ref, Form: form}, true
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
	return diag.NewErrorDiagnostic(
		"State Form identity is not supported by this provider",
		fmt.Sprintf(
			"State is bound to %s/%s@%s schema=%s. This provider build carries codecs for %s. "+
				"The provider will not read, update, or delete this resource under a different exact FormRef, "+
				"because that would reinterpret state written against one contract as another. "+
				"Pin the provider version that wrote the state, or perform an explicit create/import migration "+
				"onto an identity this build supports.",
			got.APIVersion, got.Kind, got.DefinitionVersion, got.SchemaDigest,
			v3RenderRefs(known),
		),
	)
}
