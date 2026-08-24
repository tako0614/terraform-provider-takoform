package tableformcatalog

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestCatalogValidatesAndUsesVersionlessFamily(t *testing.T) {
	t.Parallel()
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
	if Family.APIVersion() != "table.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q", Family.APIVersion())
	}
	if len(Forms) != 1 || Forms[0].Kind != "Table" || Forms[0].DefinitionVersion != "0.1.0" {
		t.Fatalf("forms = %+v", Forms)
	}
	if Forms[0].Role != model.RoleIdentity || Forms[0].RequiresHostAPI != "forms.takoform.com/v1" {
		t.Fatalf("table identity metadata = role %q, host %q", Forms[0].Role, Forms[0].RequiresHostAPI)
	}
}

func TestTableDesiredFieldsAndMutability(t *testing.T) {
	t.Parallel()
	table, ok := ByKind("Table")
	if !ok {
		t.Fatal("Table is not declared")
	}
	wantFields := []string{"partitionKey", "sortKey", "secondaryIndexes", "ttlAttribute"}
	gotFields := make([]string, 0, len(table.Fields))
	for _, field := range table.Fields {
		gotFields = append(gotFields, field.Wire)
	}
	if !slices.Equal(gotFields, wantFields) {
		t.Fatalf("table fields = %v, want %v", gotFields, wantFields)
	}
	if got := table.ImmutableFields(); !slices.Equal(got, []string{"/partitionKey", "/sortKey"}) {
		t.Fatalf("immutable fields = %v", got)
	}
	if got := table.LifecycleCapabilities(); !slices.Equal(got, []string{"create", "read", "update", "delete", "import", "observe"}) {
		t.Fatalf("lifecycle capabilities = %v", got)
	}
	if got := table.StructuralConstraints; len(got) != 1 || got[0].Kind != model.ConstraintUniqueBy ||
		got[0].List != "/secondaryIndexes" || got[0].Member != "name" {
		t.Fatalf("Table structural constraints = %#v, want secondaryIndexes unique by name", got)
	}
	for _, field := range table.Fields {
		switch field.Wire {
		case "partitionKey", "sortKey":
			if !field.Immutable {
				t.Errorf("%s is not immutable", field.Wire)
			}
		case "secondaryIndexes", "ttlAttribute":
			if field.Immutable {
				t.Errorf("%s is unexpectedly immutable", field.Wire)
			}
		}
	}
}

func TestTableRejectsDuplicateSecondaryIndexNames(t *testing.T) {
	t.Parallel()
	table, ok := ByKind("Table")
	if !ok {
		t.Fatal("Table is not declared")
	}
	desired := table.CanonicalDesired()
	desired["secondaryIndexes"] = []any{
		map[string]any{"name": "by-email", "partitionKey": "email", "sortKey": "createdAt"},
		map[string]any{"name": "by-email", "partitionKey": "tenantId", "sortKey": "updatedAt"},
	}
	if err := model.ValidateStructuralConstraintValues(table.StructuralConstraints, desired); err == nil || !strings.Contains(err.Error(), "repeat") {
		t.Fatalf("duplicate secondary-index error = %v, want uniqueBy rejection", err)
	}
}

func TestRenderedTableDefinitionPinsInterfaceAndSchema(t *testing.T) {
	t.Parallel()
	rendered, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 {
		t.Fatalf("rendered %d forms, want 1", len(rendered))
	}
	definition := rendered[0].Definition
	properties, ok := definition.DesiredSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("desired schema has no properties")
	}
	for _, name := range []string{"partitionKey", "sortKey", "secondaryIndexes", "ttlAttribute"} {
		if _, ok := properties[name]; !ok {
			t.Errorf("desired schema omits %s", name)
		}
	}
	if _, ok := properties["name"]; ok {
		t.Fatal("desired schema carries envelope name")
	}
	if len(definition.ProvidedInterfaces) != 1 || definition.ProvidedInterfaces[0].Name != TableDocumentInterfaceName {
		t.Fatalf("provided interfaces = %+v", definition.ProvidedInterfaces)
	}
	ref, err := InterfaceRefFor(TableDocumentInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if definition.ProvidedInterfaces[0] != ref {
		t.Fatalf("provided interface = %+v, want %+v", definition.ProvidedInterfaces[0], ref)
	}
	if _, err := json.Marshal(definition); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered[0].DefinitionJSON, "takoform_table") {
		t.Fatal("provider resource type leaked into Form Definition")
	}
}

func TestTableInterfaceSurface(t *testing.T) {
	t.Parallel()
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != TableDocumentInterfaceName {
		t.Fatalf("interfaces = %+v", definitions)
	}
	wantOperations := []string{"get", "put", "delete", "query"}
	gotOperations := make([]string, 0, len(definitions[0].Operations))
	for _, operation := range definitions[0].Operations {
		gotOperations = append(gotOperations, operation.Name)
	}
	if !slices.Equal(gotOperations, wantOperations) {
		t.Fatalf("operations = %v, want %v", gotOperations, wantOperations)
	}
	if definitions[0].Limits["maxSecondaryIndexes"] != 20 || definitions[0].Limits["maxItemBytes"] != 409600 {
		t.Fatalf("portable limits = %+v", definitions[0].Limits)
	}
	for _, operation := range definitions[0].Operations {
		if operation.Name != "get" && operation.Name != "put" {
			continue
		}
		if _, ok := operation.InputSchema["$defs"]; operation.Name == "put" && !ok {
			t.Fatalf("%s input omits recursive document definitions", operation.Name)
		}
	}
}
