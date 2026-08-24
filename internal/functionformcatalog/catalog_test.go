package functionformcatalog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestCatalogValidates(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIdentityAndRoles(t *testing.T) {
	wantKinds := []string{"Function", "FunctionVersion", "FunctionDeployment", "FunctionEndpoint"}
	if Family.APIVersion() != "function.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q", Family.APIVersion())
	}
	if len(Forms) != len(wantKinds) {
		t.Fatalf("forms = %d, want %d", len(Forms), len(wantKinds))
	}
	wantRoles := []model.Role{model.RoleIdentity, model.RoleRevision, model.RoleDeployment, model.RoleAttachment}
	for index, form := range Forms {
		if form.Kind != wantKinds[index] || form.Role != wantRoles[index] {
			t.Fatalf("form[%d] = %s/%s, want %s/%s", index, form.Kind, form.Role, wantKinds[index], wantRoles[index])
		}
	}
}

func TestFunctionFieldsAndConstraints(t *testing.T) {
	version, ok := ByKind("FunctionVersion")
	if !ok {
		t.Fatal("FunctionVersion not found")
	}
	var names []string
	for _, field := range version.Fields {
		names = append(names, field.Wire)
	}
	want := []string{"function", "artifact", "handler", "vars", "requiredSensitiveVars", "externalServices", "memoryMiB", "timeoutSeconds", "maxConcurrency"}
	if !slices.Equal(names, want) {
		t.Fatalf("FunctionVersion fields = %v, want %v", names, want)
	}
	if target := version.Fields[0].ResourceTarget; target == nil || target.Group != Family.APIVersion() || target.Kind != "Function" || !target.Contract.ExactForm {
		t.Fatalf("FunctionVersion.function target = %#v, want exact same-family ResourceTarget", target)
	}
	deployment, ok := ByKind("FunctionDeployment")
	if !ok {
		t.Fatal("FunctionDeployment not found")
	}
	constraints := deployment.Constraints()
	if len(constraints) != 3 {
		t.Fatalf("FunctionDeployment constraints = %#v, want exclusive, sum, sameResolvedTarget", constraints)
	}
	if constraints[0].Kind != model.ConstraintExclusive || constraints[0].Reference != "/function" {
		t.Fatalf("deployment exclusive constraint = %#v", constraints[0])
	}
	if constraints[1].Kind != model.ConstraintSum || constraints[1].List != "/versions" || constraints[1].Member != "weight" || constraints[1].Total != 10000 {
		t.Fatalf("deployment sum constraint = %#v", constraints[1])
	}
	if constraints[2].Kind != model.ConstraintSameResolvedTarget || constraints[2].Anchor != "/function" || constraints[2].Members != "/versions/*/functionVersion" || constraints[2].Through != "/function" {
		t.Fatalf("deployment same-target constraint = %#v", constraints[2])
	}
}

func TestRenderedExactIdentitiesAndOutputs(t *testing.T) {
	rendered, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != len(Forms) {
		t.Fatalf("rendered = %d, want %d", len(rendered), len(Forms))
	}
	for _, item := range rendered {
		if item.Definition.APIVersion != Family.APIVersion() || item.Definition.DefinitionVersion != definitionVersion {
			t.Fatalf("%s identity = %s@%s", item.Kind, item.Definition.APIVersion, item.Definition.DefinitionVersion)
		}
		if _, ok := item.Fixtures["desired.json"]; !ok {
			t.Fatalf("%s has no canonical desired fixture", item.Kind)
		}
	}
	seenDigests := map[string]string{}
	for _, item := range rendered {
		digest, err := catalogDefinitionDigest(item.DefinitionJSON)
		if err != nil {
			t.Fatal(err)
		}
		if previous, duplicate := seenDigests[digest]; duplicate {
			t.Fatalf("Forms %s and %s share digest %s", previous, item.Kind, digest)
		}
		seenDigests[digest] = item.Kind
	}
	endpoint, ok := ByKind("FunctionEndpoint")
	if !ok {
		t.Fatal("FunctionEndpoint not found")
	}
	if len(endpoint.Outputs) != 2 || !endpoint.Outputs[0].HostAssigned || !endpoint.Outputs[1].HostAssigned {
		t.Fatalf("endpoint outputs = %#v, want two host-assigned outputs", endpoint.Outputs)
	}
	resolver := NewTargetResolver()
	refs, err := resolver.TargetFormRefs("Function")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].APIVersion != Family.APIVersion() || refs[0].Kind != "Function" || refs[0].DefinitionVersion != definitionVersion || len(refs[0].SchemaDigest) != len("sha256:")+64 {
		t.Fatalf("Function exact ref = %#v", refs)
	}
}

func TestFunctionRuntimeInterfaceIsExactAndResolverRejectsDrift(t *testing.T) {
	t.Parallel()
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != FunctionRuntimeInterfaceName || definitions[0].Version != "1.0.0" {
		t.Fatalf("runtime interface definitions = %#v", definitions)
	}
	rendered, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 || rendered[0].Name != FunctionRuntimeInterfaceName || rendered[0].Version != "1.0.0" {
		t.Fatalf("rendered runtime interfaces = %#v", rendered)
	}
	ref, err := InterfaceRefFor(FunctionRuntimeInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if ref.APIVersion != InterfaceAPIVersion || ref.Name != FunctionRuntimeInterfaceName || ref.Version != "1.0.0" || ref.SchemaDigest != rendered[0].SchemaDigest {
		t.Fatalf("runtime interface ref = %#v, rendered = %#v", ref, rendered[0])
	}
	identity, ok := ByKind("Function")
	if !ok || len(identity.ProvidedInterfaces) != 1 || identity.ProvidedInterfaces[0] != (model.InterfaceRefSource{Name: FunctionRuntimeInterfaceName, Version: "1.0.0"}) {
		t.Fatalf("Function provided interfaces = %#v", identity.ProvidedInterfaces)
	}
	forms, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(forms[0].Definition.ProvidedInterfaces) != 1 || forms[0].Definition.ProvidedInterfaces[0] != ref {
		t.Fatalf("rendered Function provided interfaces = %#v, want %#v", forms[0].Definition.ProvidedInterfaces, ref)
	}
	resolver := NewTargetResolver()
	target := model.ResourceTarget{
		Group: Family.APIVersion(), Kind: "Function",
		Contract: model.TargetContract{Interface: &model.InterfaceRefSource{Name: FunctionRuntimeInterfaceName, Version: "1.0.0"}},
	}
	resolved, err := resolver.ResolveResourceTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RequiredInterface == nil || resolved.RequiredInterface.SchemaDigest != ref.SchemaDigest {
		t.Fatalf("resolved runtime interface = %#v, want digest %s", resolved.RequiredInterface, ref.SchemaDigest)
	}
	for _, wrong := range []model.InterfaceRefSource{
		{Name: "function.other", Version: "1.0.0"},
		{Name: FunctionRuntimeInterfaceName, Version: "1.0.1"},
	} {
		wrong := wrong
		t.Run(wrong.Name+"@"+wrong.Version, func(t *testing.T) {
			_, err := resolver.ResolveResourceTarget(model.ResourceTarget{
				Group: Family.APIVersion(), Kind: "Function",
				Contract: model.TargetContract{Interface: &wrong},
			})
			if err == nil {
				t.Fatalf("wrong runtime interface %s@%s unexpectedly resolved", wrong.Name, wrong.Version)
			}
		})
	}
	if _, err := resolver.RequiredInterface(FunctionRuntimeInterfaceName, "1.0.1"); err == nil {
		t.Fatal("wrong runtime Interface version unexpectedly resolved directly")
	}
}

func TestFunctionRuntimeInterfaceNormativeSchemaAndClosedABI(t *testing.T) {
	t.Parallel()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	normative, err := os.ReadFile(filepath.Join("..", "..", "spec", "schemas", "interface-definition-v1alpha1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	normativeValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(normative))
	if err != nil {
		t.Fatal(err)
	}
	const normativeID = "https://forms.takoform.com/schemas/interfaces/v1alpha1/interface-definition.schema.json"
	if err := compiler.AddResource(normativeID, normativeValue); err != nil {
		t.Fatal(err)
	}
	normativeSchema, err := compiler.Compile(normativeID)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 {
		t.Fatalf("rendered runtime interfaces = %d, want 1", len(rendered))
	}
	definitionValue, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(rendered[0].DefinitionJSON)))
	if err != nil {
		t.Fatal(err)
	}
	if err := normativeSchema.Validate(definitionValue); err != nil {
		t.Fatalf("function.runtime definition violates normative Interface schema: %v", err)
	}

	definition := definitionsByName(t, FunctionRuntimeInterfaceName)
	if len(definition.Operations) != 1 || definition.Operations[0].Name != FunctionRuntimeEntrypoint {
		t.Fatalf("runtime operations = %#v, want one %q entrypoint", definition.Operations, FunctionRuntimeEntrypoint)
	}
	endpoint, ok := ByKind("FunctionEndpoint")
	if !ok || len(endpoint.Fields) != 1 || endpoint.Fields[0].RequiredEntrypoint != FunctionRuntimeEntrypoint {
		t.Fatalf("FunctionEndpoint entrypoint = %#v, want %q", endpoint.Fields, FunctionRuntimeEntrypoint)
	}
	if !strings.Contains(definition.Operations[0].Description, "handler(event, context)") {
		t.Fatalf("runtime operation does not state handler(event, context): %s", definition.Operations[0].Description)
	}

	inputSchema := compileRuntimeInputSchema(t, definition.Operations[0].InputSchema)
	valid := map[string]any{
		"event": map[string]any{
			"kind": "http", "method": "GET", "url": "https://fn.example.invalid/health",
			"headers": map[string]any{}, "body": "",
		},
		"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000},
	}
	if err := inputSchema.Validate(valid); err != nil {
		t.Fatalf("valid http invocation rejected: %v", err)
	}
	invalid := []struct {
		name  string
		value map[string]any
	}{
		{
			name: "unknown event kind",
			value: map[string]any{
				"event": map[string]any{
					"kind": "schedule", "method": "GET", "url": "https://fn.example.invalid/health",
					"headers": map[string]any{}, "body": "",
				},
				"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000},
			},
		},
		{
			name: "unknown event member",
			value: map[string]any{
				"event": map[string]any{
					"kind": "http", "method": "GET", "url": "https://fn.example.invalid/health",
					"headers": map[string]any{}, "body": "", "unexpected": "no",
				},
				"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000},
			},
		},
		{
			name: "unknown context member",
			value: map[string]any{
				"event": map[string]any{
					"kind": "http", "method": "GET", "url": "https://fn.example.invalid/health",
					"headers": map[string]any{}, "body": "",
				},
				"context": map[string]any{"invocationId": "inv-001", "remainingTimeMs": 900000, "unexpected": "no"},
			},
		},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			if err := inputSchema.Validate(testCase.value); err == nil {
				t.Fatal("invalid invocation unexpectedly accepted")
			}
		})
	}
}

func definitionsByName(t *testing.T, name string) InterfaceDefinition {
	t.Helper()
	for _, definition := range InterfaceDefinitions() {
		if definition.Name == name {
			return definition
		}
	}
	t.Fatalf("interface %q not found", name)
	return InterfaceDefinition{}
}

func compileRuntimeInputSchema(t *testing.T, schema map[string]any) *jsonschema.Schema {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	const schemaID = "urn:takoform:function-runtime-input"
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(schemaID, value); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func catalogDefinitionDigest(definition string) (string, error) {
	return formpackage.DigestCanonicalJSON([]byte(definition))
}

func TestOptionalFieldsDeclareMeaning(t *testing.T) {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.Required && field.Default == nil && !field.AbsenceIsSemantic {
				t.Errorf("%s.%s is optional without Default or AbsenceIsSemantic", form.Kind, field.Wire)
			}
		}
	}
}
