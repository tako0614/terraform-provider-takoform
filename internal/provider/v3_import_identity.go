package provider

// v3_import_identity.go parses the import identity of the Host API v1beta1
// resource lane.
//
// Terraform hands a provider one opaque string, and this lane's identity is
// five values: space, the four-member exact FormRef, and the resource name. A
// delimiter-joined string cannot carry them. A SpaceID is opaque, case-
// sensitive UTF-8 whose only forbidden character is `/`
// (spec/decisions/0003), so every separator a reader could actually type may
// legitimately appear INSIDE the space, and no escape convention survives
// copy-paste through a shell. The canonical form is therefore one JSON object;
// the historical `NAME` and `SPACE/NAME` forms are retained because they are
// unambiguous and they are what almost every import is.
//
// The exact FormRef matters because a resource created under an EARLIER
// definition version is a different contract identity. An import that seeded
// the default create ref would rebind that resource to a definition it was
// never applied under, and the next read would query an identity the host does
// not serve for it (spec/decisions/0017).

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// v3ImportIdentity is one parsed import ID.
type v3ImportIdentity struct {
	// Space is the exact opaque SpaceID, or empty when the ID named none and
	// the provider default applies.
	Space string
	// Name is the portable resource name.
	Name string
	// Ref is the exact FormRef the ID named; valid only when HasFormRef.
	Ref v3FormRefValue
	// HasFormRef reports whether the ID named an exact identity. When false the
	// caller resolves the default create ref.
	HasFormRef bool
}

// v3ImportDocument is the canonical JSON import identity. Every member is
// spelled exactly as the wire spells it, so an operator can copy the values
// straight out of a resource representation or out of `terraform show`.
type v3ImportDocument struct {
	Space             string `json:"space"`
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	Name              string `json:"name"`
}

// V3ImportIDSyntax is the operator-facing description of the accepted forms.
// The docs renderer and the diagnostics share this one statement.
const V3ImportIDSyntax = `NAME, SPACE/NAME, or one JSON object ` +
	`{"space":…,"apiVersion":…,"kind":…,"definitionVersion":…,"schemaDigest":…,"name":…} ` +
	`(space optional; the four FormRef members are all-or-nothing)`

// v3ParseImportID parses one import ID. A value whose first non-space
// character is `{` is the canonical JSON form; anything else is a short form.
// Choosing on that character is unambiguous, because a portable resource name
// matches ^[a-z][a-z0-9-]{0,62}$ and a SpaceID that begins with `{` reaches
// this function only inside the JSON form's own `space` member.
func v3ParseImportID(id string) (v3ImportIdentity, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return v3ImportIdentity{}, errors.New("import ID is empty; expected " + V3ImportIDSyntax)
	}
	if trimmed[0] == '{' {
		return v3ParseImportDocument(trimmed)
	}
	space, name, err := splitImportID(trimmed)
	if err != nil {
		return v3ImportIdentity{}, fmt.Errorf("%w; expected %s", err, V3ImportIDSyntax)
	}
	return v3ImportIdentity{Space: space, Name: name}, nil
}

func v3ParseImportDocument(raw string) (v3ImportIdentity, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document v3ImportDocument
	if err := decoder.Decode(&document); err != nil {
		return v3ImportIdentity{}, fmt.Errorf("import identity is not one JSON object with the members of %s: %w", V3ImportIDSyntax, err)
	}
	if decoder.More() {
		return v3ImportIdentity{}, errors.New("import identity must be exactly one JSON object")
	}
	if err := clientv3.ValidateResourceName(document.Name); err != nil {
		return v3ImportIdentity{}, fmt.Errorf("import identity name is invalid: %w", err)
	}
	if document.Space != "" {
		if err := client.ValidateSpaceID(document.Space); err != nil {
			return v3ImportIdentity{}, fmt.Errorf("import identity SpaceID is invalid: %w", err)
		}
	}
	identity := v3ImportIdentity{Space: document.Space, Name: document.Name}
	members := []string{document.APIVersion, document.Kind, document.DefinitionVersion, document.SchemaDigest}
	present := 0
	for _, member := range members {
		if member != "" {
			present++
		}
	}
	if present == 0 {
		// A JSON identity naming only space and name is the short form spelled
		// safely; it resolves to the default create ref like `SPACE/NAME`.
		return identity, nil
	}
	if present != len(members) {
		return v3ImportIdentity{}, errors.New(
			"import identity must carry apiVersion, kind, definitionVersion, and schemaDigest together, or none of them",
		)
	}
	ref := clientv3.FormRef{
		APIVersion:        document.APIVersion,
		Kind:              document.Kind,
		DefinitionVersion: document.DefinitionVersion,
		SchemaDigest:      document.SchemaDigest,
	}
	if err := clientv3.ValidateFormRef(ref); err != nil {
		return v3ImportIdentity{}, fmt.Errorf("import identity FormRef is invalid: %w", err)
	}
	identity.HasFormRef = true
	identity.Ref = v3FormRefValue{
		APIVersion:        ref.APIVersion,
		Kind:              ref.Kind,
		DefinitionVersion: ref.DefinitionVersion,
		SchemaDigest:      ref.SchemaDigest,
	}
	return identity, nil
}

// v3UnsupportedImportRefError refuses an import that names an exact identity
// this build cannot decode. Adopting it under a different ref would write state
// claiming a contract the resource was never applied under.
func v3UnsupportedImportRefError(got v3FormRefValue, known []currentformregistry.V3Ref) diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"Import Form identity is not supported by this provider",
		fmt.Sprintf(
			"The import identity names %s/%s@%s schema=%s. This provider build carries codecs for %s. "+
				"Import the resource with an identity this build supports, or use the provider version that carries the one you named.",
			got.APIVersion, got.Kind, got.DefinitionVersion, got.SchemaDigest,
			v3RenderRefs(known),
		),
	)
}
