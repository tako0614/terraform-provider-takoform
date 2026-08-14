package provider

import (
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
)

// formCodec couples one exact immutable FormRef with the closed desired-state
// codec for those exact bytes. The pair is looked up as a unit; a familiar
// Kind or SemVer never selects a codec on its own.
type formCodec struct {
	form client.InstalledFormReference
	kind formcatalog.Kind
}

func providerExactFormCodec(form client.InstalledFormReference) (formCodec, error) {
	ref := formregistry.Ref{
		APIVersion:        form.FormRef.APIVersion,
		Kind:              form.FormRef.Kind,
		DefinitionVersion: form.FormRef.DefinitionVersion,
		SchemaDigest:      form.FormRef.SchemaDigest,
		PackageDigest:     form.PackageDigest,
	}
	if _, err := formregistry.ForExact(ref); err != nil {
		return formCodec{}, err
	}
	kind, ok := formcatalog.ByKindVersion(ref.Kind, ref.DefinitionVersion)
	if !ok {
		return formCodec{}, fmt.Errorf(
			"takoform: provider build has no exact codec for %s@%s",
			ref.Kind,
			ref.DefinitionVersion,
		)
	}
	return formCodec{form: form, kind: kind}, nil
}

func (r *formResource) currentFormCodec() (formCodec, error) {
	if r.data == nil {
		return formCodec{}, fmt.Errorf("takoform: provider data is unavailable")
	}
	form, ok := r.data.forms[r.kind.Kind]
	if !ok {
		return formCodec{}, fmt.Errorf("takoform: current %s FormRef is unavailable", r.kind.Kind)
	}
	codec, err := providerExactFormCodec(form)
	if err != nil {
		return formCodec{}, err
	}
	if codec.kind.DefinitionVersion != r.kind.DefinitionVersion {
		return formCodec{}, fmt.Errorf("takoform: current %s codec and resource schema disagree", r.kind.Kind)
	}
	return codec, nil
}
