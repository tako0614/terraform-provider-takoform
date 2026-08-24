package topicformcatalog

import (
	"slices"
	"strings"
	"testing"
)

func TestTopicPublishInterfaceIsExactAndClosed(t *testing.T) {
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != TopicPublishInterfaceName || definitions[0].Version != "1.0.0" {
		t.Fatalf("definitions = %#v", definitions)
	}
	if got := definitions[0].Semantics; got.Consistency != "eventual" || got.Delivery != "at_least_once" || got.Ordering != "none" {
		t.Fatalf("semantics = %#v", got)
	}
	var operations []string
	for _, operation := range definitions[0].Operations {
		operations = append(operations, operation.Name)
	}
	if !slices.Equal(operations, []string{"publish"}) {
		t.Fatalf("operations = %v", operations)
	}
	rendered, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 || !strings.HasPrefix(rendered[0].SchemaDigest, "sha256:") {
		t.Fatalf("rendered interfaces = %#v", rendered)
	}
	ref, err := InterfaceRefFor(TopicPublishInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if ref.SchemaDigest != rendered[0].SchemaDigest || ref.APIVersion != InterfaceAPIVersion {
		t.Fatalf("interface ref = %#v, rendered = %#v", ref, rendered[0])
	}
	input := definitions[0].Operations[0].InputSchema
	body := input["properties"].(map[string]any)["body"].(map[string]any)
	if len(body["oneOf"].([]any)) != 2 {
		t.Fatalf("body schema = %#v, want closed utf8/base64 tagged union", body)
	}
}
