// Package tableformcatalog declares the provider-neutral Table Form Family.
//
// The catalog is data only. It describes the desired identity of a
// key-addressed document table and the exact table.document Interface it
// provides; provider resource names, backend endpoints, and data-plane
// implementations are deliberately outside this package.
package tableformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the versionless Table Form Family group. A Form's own definition
// SemVer and schema digest identify its contract; the family group carries no
// latest or provider version alias.
var Family = model.Family{Group: "table.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	currentHostAPI    = "forms.takoform.com/v1"

	// Attribute names are UTF-8 strings bounded by the proposal's 255-byte
	// portable minimum. JSON Schema's maxLength is a character ceiling; the
	// byte rule remains part of the Interface prose and host validation.
	attributeNamePattern = `^[A-Za-z][A-Za-z0-9._-]{0,254}$`

	keyTypeString = "string"
	keyTypeNumber = "number"
	keyTypeBytes  = "bytes"
)

func attributeField(hcl, wire, doc string, required bool) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindString,
		Required: required, Pattern: attributeNamePattern, MaxLength: 255,
		Doc: doc, Example: "tenantId", CounterExample: "1-not-an-attribute",
	}
}

func keyTypeField(hcl, wire, doc string, example string) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindStringEnum,
		Required: true, Enum: []string{keyTypeString, keyTypeNumber, keyTypeBytes},
		Doc: doc, Example: example, CounterExample: "boolean",
	}
}

func keySchemaField(hcl, wire, doc, name, keyType string) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindObject,
		Required: true, Immutable: true,
		Doc: doc,
		Fields: []model.Field{
			{
				HCL: "name", Wire: "name", Kind: model.KindString,
				Required: true, Pattern: attributeNamePattern, MaxLength: 255,
				Doc:     "Top-level document attribute used by this key.",
				Example: name, CounterExample: "1-not-an-attribute",
			},
			{
				HCL: "type", Wire: "type", Kind: model.KindStringEnum,
				Required: true, Enum: []string{keyTypeString, keyTypeNumber, keyTypeBytes},
				Doc:     "Portable scalar type used to encode and order this key attribute.",
				Example: keyType, CounterExample: "boolean",
			},
		},
		Example: map[string]any{"name": name, "type": keyType},
	}
}

func secondaryIndexField() model.Field {
	return model.Field{
		HCL: "secondary_indexes", Wire: "secondaryIndexes", Kind: model.KindObjectList,
		MaxItems: 20, Default: []any{},
		Doc: "Secondary indexes over top-level document attributes. Adding or removing an entry " +
			"is an in-place table update; an index is redefined only by removing it and adding it later. " +
			"Omitting this list declares no secondary indexes.",
		Fields: []model.Field{
			{
				HCL: "name", Wire: "name", Kind: model.KindString,
				Required: true, Pattern: model.PatternResourceName, MaxLength: model.ResourceNameMaxLength,
				Doc:     "Stable portable name of this secondary index.",
				Example: "by-email", CounterExample: "Not an index name",
			},
			attributeField("partition_key", "partitionKey", "Top-level attribute forming the index partition key.", true),
			{
				HCL: "sort_key", Wire: "sortKey", Kind: model.KindString,
				AbsenceIsSemantic: true, Pattern: attributeNamePattern, MaxLength: 255,
				Doc:     "Optional top-level attribute forming the index sort key. When absent, the index has no sort key.",
				Example: "createdAt", CounterExample: "1-not-an-attribute",
			},
		},
		Example: []any{map[string]any{
			"name": "by-email", "partitionKey": "email", "sortKey": "createdAt",
		}},
	}
}

// Forms is the complete Table Family MVP set, in stable order.
// ResourceType is intentionally empty: provider resource names are not Form
// semantics and are assigned by a provider-owned registry.
var Forms = []model.Form{
	{
		Family: Family, Kind: "Table", Slug: "table", Role: model.RoleIdentity,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Table", Description: "Key-addressed document table with a declared partition key, " +
			"optional sort key, mutable secondary indexes, and optional lazy TTL. The table.document " +
			"Interface fixes document values, conditional writes, consistent single-item reads, and " +
			"key-ordered partition queries; this identity carries only the table's addressing declaration.",
		Fields: []model.Field{
			keySchemaField("partition_key", "partitionKey",
				"Immutable partition key declaration. Every item must carry this top-level attribute; changing it replaces the table.",
				"tenantId", keyTypeString),
			{
				HCL: "sort_key", Wire: "sortKey", Kind: model.KindObject,
				Immutable: true, AbsenceIsSemantic: true,
				Doc: "Optional immutable sort key declaration. When absent, each partition key addresses at most one item; changing it replaces the table.",
				Fields: []model.Field{
					{
						HCL: "name", Wire: "name", Kind: model.KindString,
						Required: true, Pattern: attributeNamePattern, MaxLength: 255,
						Doc:     "Top-level document attribute used by the sort key.",
						Example: "createdAt", CounterExample: "1-not-an-attribute",
					},
					{
						HCL: "type", Wire: "type", Kind: model.KindStringEnum,
						Required: true, Enum: []string{keyTypeString, keyTypeNumber, keyTypeBytes},
						Doc:     "Portable scalar type used to encode and order the sort key attribute.",
						Example: keyTypeNumber, CounterExample: "boolean",
					},
				},
				Example: map[string]any{"name": "createdAt", "type": keyTypeNumber},
			},
			secondaryIndexField(),
			{
				HCL: "ttl_attribute", Wire: "ttlAttribute", Kind: model.KindString,
				AbsenceIsSemantic: true, Pattern: attributeNamePattern, MaxLength: 255,
				Doc:     "Optional top-level number attribute used for lazy epoch-second expiry. When absent, the table has no TTL policy; setting or clearing it is an in-place update.",
				Example: "expiresAt", CounterExample: "1-not-an-attribute",
			},
		},
		StructuralConstraints: []model.Constraint{{
			Kind: model.ConstraintUniqueBy, List: "/secondaryIndexes", Member: "name",
		}},
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: TableDocumentInterfaceName, Version: "1.0.0"}},
	},
}

// Validate proves the source catalog is closed and each Form declaration is
// internally coherent. Interface and rendered Definition checks are repeated
// by RenderForms so callers may validate either surface independently.
func Validate() error {
	if err := model.ValidateNoOpenTokens(Forms); err != nil {
		return err
	}
	seenKinds, seenSlugs := map[string]bool{}, map[string]bool{}
	for _, form := range Forms {
		if err := form.Validate(); err != nil {
			return err
		}
		if form.Family != Family {
			return fmt.Errorf("form %s belongs to family %s, want %s", form.Kind, form.Family.APIVersion(), Family.APIVersion())
		}
		if seenKinds[form.Kind] || seenSlugs[form.Slug] {
			return fmt.Errorf("duplicate Table family identity %s/%s", form.Kind, form.Slug)
		}
		seenKinds[form.Kind], seenSlugs[form.Slug] = true, true
		if form.DefinitionVersion != definitionVersion {
			return fmt.Errorf("form %s definition version %q, want %q", form.Kind, form.DefinitionVersion, definitionVersion)
		}
	}
	if err := ValidateInterfaceDefinitions(InterfaceDefinitions()); err != nil {
		return err
	}
	return nil
}

// ByKind returns one source Form by its exact portable kind.
func ByKind(kind string) (model.Form, bool) {
	for _, form := range Forms {
		if form.Kind == kind {
			return form, true
		}
	}
	return model.Form{}, false
}
