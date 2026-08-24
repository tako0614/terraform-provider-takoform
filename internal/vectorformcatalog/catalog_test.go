package vectorformcatalog

import (
	"slices"
	"strings"
	"testing"
)

func TestCatalogValidatesAndUsesVersionlessFamily(t *testing.T) {
	t.Parallel()
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
	if Family.APIVersion() != "vector.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q", Family.APIVersion())
	}
	if len(Forms) != 1 || Forms[0].Kind != "VectorIndex" || Forms[0].DefinitionVersion != "0.1.0" {
		t.Fatalf("forms = %+v", Forms)
	}
}

func TestVectorIdentityFieldsAreImmutableAndClosed(t *testing.T) {
	t.Parallel()
	vector, ok := ByKind("VectorIndex")
	if !ok {
		t.Fatal("VectorIndex is not declared")
	}
	if got := vector.ImmutableFields(); !slices.Equal(got, []string{"/dimension", "/metric"}) {
		t.Fatalf("immutable fields = %v", got)
	}
	if got := vector.LifecycleCapabilities(); !slices.Equal(got, []string{"create", "read", "delete", "import", "observe"}) {
		t.Fatalf("lifecycle capabilities = %v", got)
	}
	for _, field := range vector.Fields {
		if !field.Immutable || !field.Required {
			t.Errorf("%s required=%t immutable=%t, want both true", field.Wire, field.Required, field.Immutable)
		}
	}
	if !slices.Equal(vector.Fields[1].Enum, []string{"cosine", "euclidean", "dotproduct"}) {
		t.Fatalf("metric enum = %v", vector.Fields[1].Enum)
	}
}

func TestRenderedVectorDefinitionPinsInterfaceAndNoProviderName(t *testing.T) {
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
	if len(properties) != 2 {
		t.Fatalf("desired properties = %v", properties)
	}
	if _, ok := properties["dimension"]; !ok {
		t.Fatal("desired schema omits dimension")
	}
	if _, ok := properties["metric"]; !ok {
		t.Fatal("desired schema omits metric")
	}
	if _, ok := properties["name"]; ok {
		t.Fatal("desired schema carries envelope name")
	}
	if len(definition.ProvidedInterfaces) != 1 || definition.ProvidedInterfaces[0].Name != VectorIndexInterfaceName {
		t.Fatalf("provided interfaces = %+v", definition.ProvidedInterfaces)
	}
	if strings.Contains(rendered[0].DefinitionJSON, "takoform_dense_vector_index") || strings.Contains(rendered[0].DefinitionJSON, "takoform_vector_index") {
		t.Fatal("provider resource type leaked into Form Definition")
	}
}

func TestVectorInterfaceSurface(t *testing.T) {
	t.Parallel()
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != VectorIndexInterfaceName {
		t.Fatalf("interfaces = %+v", definitions)
	}
	wantOperations := []string{"upsert", "fetch", "query", "delete"}
	gotOperations := make([]string, 0, len(definitions[0].Operations))
	for _, operation := range definitions[0].Operations {
		gotOperations = append(gotOperations, operation.Name)
	}
	if !slices.Equal(gotOperations, wantOperations) {
		t.Fatalf("operations = %v, want %v", gotOperations, wantOperations)
	}
	for key, want := range map[string]int64{"maxDimension": 1536, "maxMetadataBytes": 40960, "maxTopK": 256} {
		if definitions[0].Limits[key] != want {
			t.Fatalf("limit %s = %d, want %d", key, definitions[0].Limits[key], want)
		}
	}
}
