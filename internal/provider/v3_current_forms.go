package provider

// v3_current_forms.go is the provider's explicit current-family projection.
//
// Form Definitions are provider-neutral. This file only composes the exact
// current Form declarations the reference provider elects to expose and
// adapts each catalog's renderer to the provider codec table. The generated
// current-form registry remains the source of exact identities; this
// projection never invents a fallback by Kind.

import (
	"github.com/tako0614/terraform-provider-takoform/internal/containerformcatalog"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/functionformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/scheduleformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/tableformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/topicformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/vectorformcatalog"
)

// v3CurrentFamily is one catalog family selected by the reference provider.
// Render is kept next to Forms so a codec always validates the exact rendered
// Definition bytes that correspond to its declaration.
type v3CurrentFamily struct {
	family model.Family
	forms  []model.Form
	render func() ([]catalogRenderedForm, error)
}

// providerV3CurrentFamilies is the eight-family/31-Form current projection
// from forms/candidates/current-family-index.json. It is intentionally an
// explicit Group+Kind source list: adding a family cannot accidentally make a
// same-named Kind resolve through another family's catalog.
func providerV3CurrentFamilies() []v3CurrentFamily {
	return []v3CurrentFamily{
		{
			family: edgeformcatalog.Family,
			forms:  edgeformcatalog.Forms,
			render: adaptV3Rendered(func() ([]edgeformcatalog.RenderedForm, error) { return edgeformcatalog.RenderForms() }, func(form edgeformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: functionformcatalog.Family,
			forms:  functionformcatalog.Forms,
			render: adaptV3Rendered(func() ([]functionformcatalog.RenderedForm, error) { return functionformcatalog.RenderForms() }, func(form functionformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: containerformcatalog.Family,
			forms:  containerformcatalog.Forms,
			render: adaptV3Rendered(func() ([]containerformcatalog.RenderedForm, error) { return containerformcatalog.RenderForms() }, func(form containerformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: tableformcatalog.Family,
			forms:  tableformcatalog.Forms,
			render: adaptV3Rendered(func() ([]tableformcatalog.RenderedForm, error) { return tableformcatalog.RenderForms() }, func(form tableformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: queueformcatalog.Family,
			forms:  queueformcatalog.Forms,
			render: adaptV3Rendered(func() ([]queueformcatalog.RenderedForm, error) { return queueformcatalog.RenderForms() }, func(form queueformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: topicformcatalog.Family,
			forms:  topicformcatalog.Forms,
			render: adaptV3Rendered(func() ([]topicformcatalog.RenderedForm, error) { return topicformcatalog.RenderForms() }, func(form topicformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: scheduleformcatalog.Family,
			forms:  scheduleformcatalog.Forms,
			render: adaptV3Rendered(func() ([]scheduleformcatalog.RenderedForm, error) { return scheduleformcatalog.RenderForms() }, func(form scheduleformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
		{
			family: vectorformcatalog.Family,
			forms:  vectorformcatalog.Forms,
			render: adaptV3Rendered(func() ([]vectorformcatalog.RenderedForm, error) { return vectorformcatalog.RenderForms() }, func(form vectorformcatalog.RenderedForm) string { return form.DefinitionJSON }),
		},
	}
}

// providerV3CurrentForms flattens the exact current-family projection in its
// catalog order for provider registration. The order is not an identity
// lookup; compileV3FormResources resolves each Form through its own GroupKind
// and exact generated registry entry.
func providerV3CurrentForms() []model.Form {
	families := providerV3CurrentFamilies()
	total := 0
	for _, family := range families {
		total += len(family.forms)
	}
	forms := make([]model.Form, 0, total)
	for _, family := range families {
		forms = append(forms, family.forms...)
	}
	return forms
}

// adaptV3Rendered erases catalog-specific RenderedForm types without erasing
// their DefinitionJSON bytes. The renderer itself remains owned by the Form
// family package.
func adaptV3Rendered[T any](render func() ([]T, error), definition func(T) string) func() ([]catalogRenderedForm, error) {
	return func() ([]catalogRenderedForm, error) {
		forms, err := render()
		if err != nil {
			return nil, err
		}
		out := make([]catalogRenderedForm, 0, len(forms))
		for _, form := range forms {
			out = append(out, catalogRenderedForm{DefinitionJSON: definition(form)})
		}
		return out, nil
	}
}
