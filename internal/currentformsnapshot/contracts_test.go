package currentformsnapshot

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestCompileClosesExactInterfaceAndBindingArtifacts(t *testing.T) {
	interfaceArtifact, interfaceRef := syntheticInterfaceArtifact(t)
	bindingArtifact, bindingRef := syntheticBindingArtifact(t, interfaceRef)

	snapshot, diagnostics := Compile(Input{
		HostAPI:    "forms.takoform.com/v1",
		Interfaces: []InterfaceArtifact{interfaceArtifact},
		Bindings:   []BindingArtifact{bindingArtifact},
	})
	if len(diagnostics) != 0 || snapshot == nil {
		t.Fatalf("complete contract graph did not compile: snapshot=%v diagnostics=%#v", snapshot != nil, diagnostics)
	}
	if got := snapshot.Interfaces(); !reflect.DeepEqual(got, []Interface{{Ref: interfaceRef}}) {
		t.Fatalf("Interfaces = %#v", got)
	}
	if got := snapshot.Bindings(); !reflect.DeepEqual(got, []Binding{{Ref: bindingRef, TargetInterface: interfaceRef}}) {
		t.Fatalf("Bindings = %#v", got)
	}
	first, ok := snapshot.InterfaceDefinition(interfaceRef)
	if !ok {
		t.Fatal("exact Interface Definition is absent")
	}
	first[0] = '['
	second, ok := snapshot.InterfaceDefinition(interfaceRef)
	if !ok || len(second) == 0 || second[0] != '{' {
		t.Fatal("Interface Definition bytes are not immutable")
	}

	snapshot, diagnostics = Compile(Input{
		HostAPI:  "forms.takoform.com/v1",
		Bindings: []BindingArtifact{bindingArtifact},
	})
	if snapshot != nil || len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticUnresolvedReference {
		t.Fatalf("missing target Interface = snapshot %v diagnostics %#v", snapshot != nil, diagnostics)
	}
}

func TestCompileClosesFormDeclaredInterfacesAndBindings(t *testing.T) {
	interfaceArtifact, interfaceRef := syntheticInterfaceArtifact(t)
	bindingArtifact, bindingRef := syntheticBindingArtifact(t, interfaceRef)
	definition := map[string]any{
		"apiVersion":        "forms.example.com",
		"kind":              "Consumer",
		"definitionVersion": "0.1.0",
		"title":             "Consumer",
		"role":              "revision",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"dependency": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"resource": map[string]any{
							"type":                          "object",
							"additionalProperties":          false,
							"properties":                    map[string]any{},
							"x-takoform-required-interface": interfaceRef,
						},
					},
					"x-takoform-binding": bindingRef.Name,
				},
			},
		},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
		"providedInterfaces":    []formpackage.InterfaceRef{interfaceRef},
		"acceptedBindings":      []formpackage.BindingRef{bindingRef},
	}
	formArtifact, formRef := packageFromDefinition(t, definition)
	base := Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{formArtifact},
		Interfaces:     []InterfaceArtifact{interfaceArtifact},
		Bindings:       []BindingArtifact{bindingArtifact},
		DefaultCreates: []DefaultPin{{Group: formRef.APIVersion, Kind: formRef.Kind, Ref: formRef}},
	}
	if snapshot, diagnostics := Compile(base); snapshot == nil || len(diagnostics) != 0 {
		t.Fatalf("declared exact contracts did not close: snapshot=%v diagnostics=%#v", snapshot != nil, diagnostics)
	}

	missing := base
	missing.Interfaces = nil
	if snapshot, diagnostics := Compile(missing); snapshot != nil || len(diagnostics) == 0 || diagnostics[0].Code != DiagnosticUnresolvedReference {
		t.Fatalf("missing declared Interface = snapshot %v diagnostics %#v", snapshot != nil, diagnostics)
	}
}

func TestCompileRejectsBindingRequiredInterfaceMismatch(t *testing.T) {
	targetArtifact, targetRef := syntheticInterfaceArtifact(t)
	wrongArtifact, wrongRef := syntheticInterfaceArtifactVariant(t, "example.invoke", "read_after_write")
	bindingArtifact, bindingRef := syntheticBindingArtifact(t, targetRef)
	definition := map[string]any{
		"apiVersion":        "forms.example.com",
		"kind":              "Consumer",
		"definitionVersion": "0.1.0",
		"title":             "Consumer",
		"role":              "revision",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"dependency": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"resource": map[string]any{
							"type":                          "object",
							"additionalProperties":          false,
							"properties":                    map[string]any{},
							"x-takoform-required-interface": wrongRef,
						},
					},
					"x-takoform-binding": bindingRef.Name,
				},
			},
		},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
		"providedInterfaces":    []formpackage.InterfaceRef{wrongRef},
		"acceptedBindings":      []formpackage.BindingRef{bindingRef},
	}
	formArtifact, formRef := packageFromDefinition(t, definition)
	snapshot, diagnostics := Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{formArtifact},
		Interfaces:     []InterfaceArtifact{targetArtifact, wrongArtifact},
		Bindings:       []BindingArtifact{bindingArtifact},
		DefaultCreates: []DefaultPin{{Group: formRef.APIVersion, Kind: formRef.Kind, Ref: formRef}},
	})
	if snapshot != nil {
		t.Fatal("Compile returned a Snapshot for a binding annotation with the wrong target Interface")
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticInvalidArtifact {
		t.Fatalf("binding target mismatch diagnostics = %#v, want one %q", diagnostics, DiagnosticInvalidArtifact)
	}
	wantPointer := "/desiredSchema/properties/dependency/properties/resource/x-takoform-required-interface"
	if diagnostics[0].Pointer != wantPointer {
		t.Fatalf("binding target mismatch pointer = %q, want %q", diagnostics[0].Pointer, wantPointer)
	}
}

func TestCompileResolvesBindingRequiredInterfaceThroughOutOfSubtreeLocalRef(t *testing.T) {
	interfaceArtifact, interfaceRef := syntheticInterfaceArtifact(t)
	bindingArtifact, bindingRef := syntheticBindingArtifact(t, interfaceRef)
	definition := bindingLocalRefDefinition(bindingRef, interfaceRef)
	formArtifact, formRef := packageFromDefinition(t, definition)

	snapshot, diagnostics := Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{formArtifact},
		Interfaces:     []InterfaceArtifact{interfaceArtifact},
		Bindings:       []BindingArtifact{bindingArtifact},
		DefaultCreates: []DefaultPin{{Group: formRef.APIVersion, Kind: formRef.Kind, Ref: formRef}},
	})
	if snapshot == nil || len(diagnostics) != 0 {
		t.Fatalf("binding local $ref did not resolve its matching target Interface: snapshot=%v diagnostics=%#v", snapshot != nil, diagnostics)
	}
}

func TestCompileRejectsBindingRequiredInterfaceMismatchThroughOutOfSubtreeLocalRef(t *testing.T) {
	targetArtifact, targetRef := syntheticInterfaceArtifact(t)
	wrongArtifact, wrongRef := syntheticInterfaceArtifactVariant(t, "example.invoke", "read_after_write")
	bindingArtifact, bindingRef := syntheticBindingArtifact(t, targetRef)
	definition := bindingLocalRefDefinition(bindingRef, wrongRef)
	formArtifact, formRef := packageFromDefinition(t, definition)

	snapshot, diagnostics := Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{formArtifact},
		Interfaces:     []InterfaceArtifact{targetArtifact, wrongArtifact},
		Bindings:       []BindingArtifact{bindingArtifact},
		DefaultCreates: []DefaultPin{{Group: formRef.APIVersion, Kind: formRef.Kind, Ref: formRef}},
	})
	if snapshot != nil {
		t.Fatal("Compile returned a Snapshot for a mismatched Interface reached through a local $ref")
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticInvalidArtifact {
		t.Fatalf("binding local $ref mismatch diagnostics = %#v, want one %q", diagnostics, DiagnosticInvalidArtifact)
	}
	wantPointer := "/desiredSchema/$defs/boundResource/properties/resource/x-takoform-required-interface"
	if diagnostics[0].Pointer != wantPointer {
		t.Fatalf("binding local $ref mismatch pointer = %q, want %q", diagnostics[0].Pointer, wantPointer)
	}
}

func TestCompileRejectsBindingRequiredInterfaceMismatchThroughEscapedLocalRef(t *testing.T) {
	targetArtifact, targetRef := syntheticInterfaceArtifact(t)
	wrongArtifact, wrongRef := syntheticInterfaceArtifactVariant(t, "example.invoke", "read_after_write")
	bindingArtifact, bindingRef := syntheticBindingArtifact(t, targetRef)
	definition := bindingLocalRefDefinition(bindingRef, wrongRef)
	desired := definition["desiredSchema"].(map[string]any)
	dependency := desired["properties"].(map[string]any)["dependency"].(map[string]any)
	dependency["$ref"] = "#/%24defs/a%7E1b%7E0c"
	boundResource := desired["$defs"].(map[string]any)["boundResource"]
	desired["$defs"] = map[string]any{"a/b~c": boundResource}
	formArtifact, formRef := packageFromDefinition(t, definition)

	snapshot, diagnostics := Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{formArtifact},
		Interfaces:     []InterfaceArtifact{targetArtifact, wrongArtifact},
		Bindings:       []BindingArtifact{bindingArtifact},
		DefaultCreates: []DefaultPin{{Group: formRef.APIVersion, Kind: formRef.Kind, Ref: formRef}},
	})
	if snapshot != nil {
		t.Fatal("Compile returned a Snapshot for a mismatched Interface reached through an escaped local $ref")
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticInvalidArtifact {
		t.Fatalf("escaped binding local $ref mismatch diagnostics = %#v, want one %q", diagnostics, DiagnosticInvalidArtifact)
	}
	wantPointer := "/desiredSchema/$defs/a~1b~0c/properties/resource/x-takoform-required-interface"
	if diagnostics[0].Pointer != wantPointer {
		t.Fatalf("escaped binding local $ref mismatch pointer = %q, want %q", diagnostics[0].Pointer, wantPointer)
	}
}

func TestContractAnnotationsLocalRefCycleIsSafe(t *testing.T) {
	_, interfaceRef := syntheticInterfaceArtifact(t)
	_, bindingRef := syntheticBindingArtifact(t, interfaceRef)
	root := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"dependency": map[string]any{
				"$ref":               "#/$defs/cycleA",
				"x-takoform-binding": bindingRef.Name,
			},
		},
		"$defs": map[string]any{
			"cycleA": map[string]any{
				"type":                          "object",
				"additionalProperties":          false,
				"properties":                    map[string]any{},
				"x-takoform-required-interface": interfaceRef,
				"$ref":                          "#/$defs/cycleB",
			},
			"cycleB": map[string]any{
				"$ref": "#/$defs/cycleA",
			},
		},
	}
	annotations := contractAnnotations(root, "/desiredSchema")
	boundInterfaces := 0
	for _, annotation := range annotations.interfaces {
		if annotation.bindingName == bindingRef.Name {
			boundInterfaces++
		}
	}
	if boundInterfaces != 1 {
		t.Fatalf("cycle-safe local $ref traversal found %d bound required-interface annotations, want one: %#v", boundInterfaces, annotations.interfaces)
	}
}

func bindingLocalRefDefinition(bindingRef formpackage.BindingRef, requiredRef formpackage.InterfaceRef) map[string]any {
	return map[string]any{
		"apiVersion":        "forms.example.com",
		"kind":              "Consumer",
		"definitionVersion": "0.1.0",
		"title":             "Consumer",
		"role":              "revision",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"dependency": map[string]any{
					"$ref":               "#/$defs/boundResource",
					"x-takoform-binding": bindingRef.Name,
				},
			},
			"$defs": map[string]any{
				"boundResource": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"resource": map[string]any{
							"type":                          "object",
							"additionalProperties":          false,
							"properties":                    map[string]any{},
							"x-takoform-required-interface": requiredRef,
						},
					},
				},
			},
		},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
		"providedInterfaces":    []formpackage.InterfaceRef{requiredRef},
		"acceptedBindings":      []formpackage.BindingRef{bindingRef},
	}
}

func syntheticInterfaceArtifact(t *testing.T) (InterfaceArtifact, formpackage.InterfaceRef) {
	return syntheticInterfaceArtifactNamed(t, "example.invoke")
}

func syntheticInterfaceArtifactNamed(t *testing.T, name string) (InterfaceArtifact, formpackage.InterfaceRef) {
	return syntheticInterfaceArtifactVariant(t, name, "eventual")
}

func syntheticInterfaceArtifactVariant(t *testing.T, name, consistency string) (InterfaceArtifact, formpackage.InterfaceRef) {
	t.Helper()
	objectSchema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
	}
	document := map[string]any{
		"apiVersion": "interfaces.takoform.com/v1alpha1",
		"kind":       "InterfaceDefinition",
		"name":       name,
		"version":    "1.0.0",
		"operations": []any{map[string]any{
			"name": "invoke", "inputSchema": objectSchema,
			"outputSchema": objectSchema, "errors": []any{},
		}},
		"semantics": map[string]any{"consistency": consistency},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return InterfaceArtifact{Origin: "test://interface/" + name, ExpectedDigest: digest, Definition: raw}, formpackage.InterfaceRef{
		APIVersion: "interfaces.takoform.com/v1alpha1", Name: name, Version: "1.0.0", SchemaDigest: digest,
	}
}

func syntheticBindingArtifact(t *testing.T, target formpackage.InterfaceRef) (BindingArtifact, formpackage.BindingRef) {
	t.Helper()
	document := map[string]any{
		"apiVersion":      "bindings.takoform.com/v1alpha2",
		"kind":            "BindingDefinition",
		"name":            "consumer.invoke",
		"version":         "1.0.0",
		"sourceRole":      "revision",
		"targetInterface": target,
		"allowedTargetForms": []any{map[string]any{
			"apiVersion": "forms.example.com", "kind": "Target",
		}},
		"bindingNameGrammar": "^[A-Za-z_$][A-Za-z0-9_$]*$",
		"runtimeProjection":  map[string]any{"operations": []string{"invoke"}},
		"lifecycle":          map[string]any{"targetDeletion": "refuse_while_bound"},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return BindingArtifact{Origin: "test://binding/consumer.invoke", ExpectedDigest: digest, Definition: raw}, formpackage.BindingRef{
		APIVersion: "bindings.takoform.com/v1alpha2", Name: "consumer.invoke", Version: "1.0.0", SchemaDigest: digest,
	}
}
