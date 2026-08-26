package currentformmodel

import (
	"strings"
	"testing"
)

func testForm() Form {
	return Form{
		Family: Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"},
		Kind:   "ExampleRevision", Slug: "example-revision",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleRevision, Title: "Example Revision", Description: "Test form.",
		DefinitionVersion: "0.1.0",
		Fields: []Field{
			{HCL: "main_module", Wire: "mainModule", Kind: KindString, Required: true,
				Pattern: PatternRelativePath, MaxLength: 240, Doc: "Main module path.",
				Example: "worker.mjs", CounterExample: "../worker.mjs"},
			{HCL: "vars", Wire: "vars", Kind: KindJSONMap, Doc: "Non-secret vars.",
				Default: map[string]any{},
				Example: map[string]any{"LOG_LEVEL": "info"}},
			{HCL: "kv_bindings", Wire: "kvBindings", Kind: KindBindingList,
				TargetKind: "EdgeKVNamespace", BindingType: "module-worker.edge-kv", Doc: "KV bindings.",
				Target:  testInterfaceContract(),
				Default: []any{},
				Example: []any{map[string]any{"name": "CACHE", "resource": map[string]any{
					"apiVersion": "edge.forms.takoform.com/v1beta1",
					"kind":       "EdgeKVNamespace",
					"name":       "cache",
				}}}},
			{HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
				Min: I64(60), Max: I64(600), Default: 300, Doc: "Retention bound."},
		},
	}
}

func TestFamilyRendersNamespacedAPIVersion(t *testing.T) {
	t.Parallel()
	family := Family{Group: "edge.forms.takoform.com", Version: "v1beta1"}
	if family.APIVersion() != "edge.forms.takoform.com/v1beta1" {
		t.Fatalf("apiVersion = %q", family.APIVersion())
	}
}

func TestFormValidationAndSchemaNeedNoProviderResourceType(t *testing.T) {
	t.Parallel()
	form := testForm()
	if err := form.Validate(); err != nil {
		t.Fatalf("provider-neutral Form was rejected: %v", err)
	}
	if _, err := form.DesiredSchema(stubResolver{}); err != nil {
		t.Fatalf("alternative client could not consume the provider-neutral Form: %v", err)
	}
}

func TestFormValidationRejectsUnsafeAuthoringIdentity(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*Form){
		"kind": func(form *Form) { form.Kind = "../Example" },
		"slug": func(form *Form) { form.Slug = "../../example" },
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			form := testForm()
			mutate(&form)
			if err := form.Validate(); err == nil {
				t.Fatal("unsafe authoring identity was accepted")
			}
		})
	}
}

// TestLifecycleCapabilitiesFollowMutableFields proves the capability set is
// derived from what the Form can actually represent, not from its role: a
// mutable role with nothing mutable to change declares no update, and no Form
// of any role declares refresh.
func TestLifecycleCapabilitiesFollowMutableFields(t *testing.T) {
	t.Parallel()
	base := []string{"create", "read", "delete", "import", "observe"}
	withUpdate := []string{"create", "read", "update", "delete", "import", "observe"}

	fieldless := Form{
		Kind: "Fieldless", Slug: "fieldless",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleIdentity, Title: "Fieldless", DefinitionVersion: "0.1.0",
	}
	mutable := fieldless
	mutable.Fields = []Field{{
		HCL: "size", Wire: "size", Kind: KindInteger, Min: I64(1), Max: I64(9),
		Default: 1, Doc: "Size.",
	}}
	allImmutable := fieldless
	allImmutable.Fields = []Field{{
		HCL: "host", Wire: "host", Kind: KindString, Required: true, Immutable: true,
		Pattern: PatternHostname, MaxLength: 253, Doc: "Host.", Example: "a.example.invalid",
	}}
	revision := mutable
	revision.Role = RoleRevision

	for _, testCase := range []struct {
		name string
		form Form
		want []string
	}{
		{"identity with no field", fieldless, base},
		{"identity with a mutable field", mutable, withUpdate},
		{"identity whose every field is immutable", allImmutable, base},
		{"revision with an otherwise-mutable field", revision, base},
	} {
		got := testCase.form.LifecycleCapabilities()
		if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
			t.Errorf("%s capabilities = %v, want %v", testCase.name, got, testCase.want)
		}
		for _, capability := range got {
			if capability == "refresh" {
				t.Errorf("%s declares refresh; the v1beta1 channel has no refresh capability", testCase.name)
			}
		}
	}
}

func TestDesiredSchemaOmitsNameAndStaysClosed(t *testing.T) {
	t.Parallel()
	form := testForm()
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	schema := mustDesiredSchema(t, form)
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["type"] != "object" {
		t.Fatalf("schema envelope = %v", schema)
	}
	if schema["additionalProperties"] != false {
		t.Fatal("desired schema must be a closed object")
	}
	properties := schema["properties"].(map[string]any)
	if _, present := properties["name"]; present {
		t.Fatal("v1beta1 desired schemas must not declare a name property; the envelope owns metadata.name")
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "mainModule" {
		t.Fatalf("required = %v", schema["required"])
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("JSONMap field must emit the bounded $defs value chain")
	}
	if _, present := defs["jsonValueDepth1"]; !present {
		t.Fatalf("missing jsonValueDepth1 in %v", defs)
	}
	leaf := defs["jsonValueDepth8"].(map[string]any)
	leafTypes := leaf["type"].([]any)
	for _, leafType := range leafTypes {
		if leafType == "object" || leafType == "array" {
			t.Fatalf("depth chain leaf admits containers: %v", leafTypes)
		}
	}
	vars := properties["vars"].(map[string]any)
	names := vars["propertyNames"].(map[string]any)
	if names["pattern"] != PortableMapKeyPattern || names[portableMapPolicyKey] != portableMapPolicy {
		t.Fatalf("vars propertyNames = %v", names)
	}
}

func TestDesiredSchemaEmptyFormHasEmptyProperties(t *testing.T) {
	t.Parallel()
	form := Form{
		Kind: "ExampleIdentity", Slug: "example-identity",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleIdentity, Title: "Example Identity", Description: "Identity with no fields.",
		DefinitionVersion: "0.1.0",
	}
	schema := mustDesiredSchema(t, form)
	if len(schema["properties"].(map[string]any)) != 0 {
		t.Fatalf("properties = %v", schema["properties"])
	}
	if _, present := schema["required"]; present {
		t.Fatal("empty form must omit required")
	}
	if _, present := schema["$defs"]; present {
		t.Fatal("empty form must omit $defs")
	}
}

func TestNegativeCasesDerivation(t *testing.T) {
	t.Parallel()
	form := testForm()
	cases, err := form.NegativeCases()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]NegativeCase{}
	for _, item := range cases {
		byName[item.Name] = item
	}
	for _, want := range []string{
		"unexpected-property",
		"missing-main-module",
		"main-module",
		"retention-seconds",
		"vars",
		"kv-bindings-binding-name",
	} {
		if _, present := byName[want]; !present {
			t.Errorf("missing derived negative case %q (have %v)", want, names(cases))
		}
	}
	if _, present := byName["missing-main-module"].Desired["mainModule"]; present {
		t.Fatal("missing-required case still carries the required field")
	}
	if byName["retention-seconds"].Desired["retentionSeconds"] != int64(59) {
		t.Fatalf("integer counter-example = %v", byName["retention-seconds"].Desired["retentionSeconds"])
	}

	empty := Form{
		Kind: "ExampleIdentity", Slug: "example-identity",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleIdentity, Title: "Example Identity", Description: "Identity with no fields.",
		DefinitionVersion: "0.1.0",
	}
	emptyCases, err := empty.NegativeCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(emptyCases) == 0 {
		t.Fatal("every Form must derive at least one desired-stage negative case")
	}
}

func names(cases []NegativeCase) []string {
	out := make([]string, 0, len(cases))
	for _, item := range cases {
		out = append(out, item.Name)
	}
	return out
}

func TestOpenTokenGuard(t *testing.T) {
	t.Parallel()
	form := testForm()
	form.Fields = append(form.Fields, Field{
		HCL: "runtime", Wire: "runtime", Kind: KindString, Pattern: "^[a-z]+$", Doc: "Open token.",
	})
	if err := ValidateNoOpenTokens([]Form{form}); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("open-token guard error = %v", err)
	}

	// Case-insensitive and nested.
	nested := testForm()
	nested.Fields = append(nested.Fields, Field{
		HCL: "options", Wire: "options", Kind: KindObject, Doc: "Nested options.",
		Fields: []Field{{HCL: "delivery_guarantee", Wire: "deliveryGuarantee", Kind: KindString, MaxLength: 16, Doc: "Open token."}},
	})
	if err := ValidateNoOpenTokens([]Form{nested}); err == nil {
		t.Fatal("nested open-token field unexpectedly accepted")
	}

	// A closed enum may reuse a name; only free strings are banned.
	enum := testForm()
	enum.Fields = append(enum.Fields, Field{
		HCL: "ordering", Wire: "ordering", Kind: KindStringEnum, Enum: []string{"none"}, Doc: "Closed enum.",
	})
	if err := ValidateNoOpenTokens([]Form{enum}); err != nil {
		t.Fatalf("closed enum rejected by open-token guard: %v", err)
	}
}

func TestBindingListsRequireRevisionRole(t *testing.T) {
	t.Parallel()
	form := testForm()
	form.Role = RoleIdentity
	if err := form.Validate(); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("identity form with binding list error = %v", err)
	}
}

func TestRevisionFormsFixEveryField(t *testing.T) {
	t.Parallel()
	form := testForm()
	pointers := form.ImmutableFields()
	if len(pointers) != len(form.Fields) {
		t.Fatalf("revision immutable pointers = %v", pointers)
	}
}

// TestValidateRejectsEnvelopeOwnedName proves the "no name property" rule the
// specification states is machine-enforced for every authored Form, not only
// for the schema emitter.
func TestValidateRejectsEnvelopeOwnedName(t *testing.T) {
	form := Form{
		Kind: "ExampleThing", Slug: "example-thing",
		Title: "Example", DefinitionVersion: "0.1.0", Role: RoleIdentity,
		RequiresHostAPI: "forms.takoform.com/v1beta1",
		Fields: []Field{{
			HCL: "name", Wire: "name", Kind: KindString, Doc: "Resource name.",
			Required: true, Example: "example",
		}},
	}
	err := form.Validate()
	if err == nil {
		t.Fatal("a top-level name field must be rejected; the envelope owns metadata.name")
	}
	if !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("validate error = %v, want it to name the envelope rule", err)
	}
}
