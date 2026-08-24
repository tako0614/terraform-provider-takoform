package currentformmodel

import (
	"testing"
)

func standardServiceTestForm(protocol string) Form {
	return Form{
		Family: Family{Group: "runtime.forms.example"}, Kind: "RuntimeRevision", Slug: "runtime-revision",
		RequiresHostAPI: "forms.takoform.com/v1", Role: RoleRevision,
		Title: "Runtime Revision", Description: "Synthetic standard-service reference coverage.", DefinitionVersion: "0.1.0",
		Fields: []Field{{
			HCL: "external_services", Wire: "externalServices", Kind: KindExternalServiceList,
			Default: []any{}, ProjectsEnvironmentNames: true,
			Doc: "Sealed runtime-native service bindings. Omitting it declares no external service.",
			Example: []any{map[string]any{
				"name": "QUANTUM_CACHE",
				"service": map[string]any{
					"apiVersion": StandardServiceAPIVersion,
					"protocol":   protocol,
				},
				"required": true,
			}},
		}},
	}
}

func TestStandardServiceProtocolIsOpenNamespacedAndOpaque(t *testing.T) {
	t.Parallel()
	// Takoform has no entry for this protocol and needs none: the namespace
	// owner and a Host's exact support profile give the identifier meaning.
	form := standardServiceTestForm("dev.example.quantum-cache")
	if err := form.Validate(); err != nil {
		t.Fatalf("unknown but schema-valid protocol was rejected: %v", err)
	}
	schema := mustDesiredSchema(t, form)
	property := schema["properties"].(map[string]any)["externalServices"].(map[string]any)
	if got := property[StandardServiceAnnotationKey]; got != StandardServiceAPIVersion {
		t.Fatalf("standard-service annotation = %v, want %s", got, StandardServiceAPIVersion)
	}
	protocolSchema := property["items"].(map[string]any)["properties"].(map[string]any)["service"].(map[string]any)["properties"].(map[string]any)["protocol"].(map[string]any)
	requiredSchema := property["items"].(map[string]any)["properties"].(map[string]any)["required"].(map[string]any)
	if requiredSchema["type"] != "boolean" || requiredSchema["default"] != true {
		t.Fatalf("standard-service required schema = %#v, want boolean default true", requiredSchema)
	}
	if _, closed := protocolSchema["enum"]; closed {
		t.Fatalf("current protocol schema grew a central enum: %#v", protocolSchema)
	}
	if protocolSchema["pattern"] != PatternStandardServiceProtocol || protocolSchema["maxLength"] != StandardServiceProtocolMaxLength {
		t.Fatalf("protocol schema = %#v", protocolSchema)
	}
	compiled := compileDesiredSchema(t, schema)
	if err := compiled.Validate(form.CanonicalDesired()); err != nil {
		t.Fatalf("unknown namespaced protocol does not satisfy emitted schema: %v", err)
	}
}

func TestStandardServiceProtocolRejectsNonNamespacedAndUnnormalizedValues(t *testing.T) {
	t.Parallel()
	for _, protocol := range []string{"s3-compatible", "Com.AmazonAWS.S3", "com..s3", "com.amazonaws.-s3"} {
		form := standardServiceTestForm(protocol)
		if err := form.Validate(); err != nil {
			t.Fatalf("form declaration for protocol %q is invalid before fixture validation: %v", protocol, err)
		}
		compiled := compileDesiredSchema(t, mustDesiredSchema(t, form))
		if err := compiled.Validate(form.CanonicalDesired()); err == nil {
			t.Errorf("protocol %q satisfied the emitted namespaced protocol grammar", protocol)
		}
	}
}

func TestStandardServiceClaimsOnlyItsRuntimeBindingName(t *testing.T) {
	t.Parallel()
	form := standardServiceTestForm("com.amazonaws.s3")
	fields := form.EnvironmentNameFields()
	if len(fields) != 1 || fields[0].Source != EnvironmentExternalServiceNames {
		t.Fatalf("environment fields = %#v", fields)
	}
	if got := EnvironmentNamesOfExample(fields[0]); len(got) != 1 || got[0] != "QUANTUM_CACHE" {
		t.Fatalf("standard-service environment names = %v, want only the sealed binding key", got)
	}
}
