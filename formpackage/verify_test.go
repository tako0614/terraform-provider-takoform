package formpackage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedSchemasAreDraft202012AndClosed(t *testing.T) {
	t.Parallel()
	schemas, err := loadSchemas()
	if err != nil {
		t.Fatal(err)
	}
	if schemas.formRef == nil || schemas.definition == nil || schemas.index == nil {
		t.Fatal("not all embedded schemas compiled")
	}
}

func TestFormRefSchemaIsExactAndUsesSemVer(t *testing.T) {
	t.Parallel()
	base := map[string]any{
		"apiVersion":        FormAPIVersion,
		"kind":              "ExampleStore",
		"definitionVersion": "1.2.3-rc.1+build.5",
		"schemaDigest":      "sha256:" + strings.Repeat("a", 64),
	}
	if _, err := validateFormRef(canonicalMarshal(t, base)); err != nil {
		t.Fatalf("valid FormRef rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "unknown field", mutate: func(value map[string]any) { value["extension"] = true }},
		{name: "numeric prerelease leading zero", mutate: func(value map[string]any) { value["definitionVersion"] = "1.2.3-01" }},
		{name: "uppercase digest", mutate: func(value map[string]any) { value["schemaDigest"] = "sha256:" + strings.Repeat("A", 64) }},
		{name: "wrong api", mutate: func(value map[string]any) { value["apiVersion"] = "forms.example/v1" }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := make(map[string]any, len(base)+1)
			for key, value := range base {
				candidate[key] = value
			}
			test.mutate(candidate)
			if _, err := validateFormRef(canonicalMarshal(t, candidate)); err == nil {
				t.Fatalf("invalid FormRef unexpectedly accepted: %v", candidate)
			}
		})
	}
}

func TestVerifyDirectoryAcceptsClosedDataOnlyPackage(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	report, err := VerifyDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.FormRef.Kind != "ExampleStore" || report.FileCount != 3 || !ValidDigest(report.PackageDigest) {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidateDefinitionAllowsDescriptionsWithoutSubstringFalsePositive(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, func(definition map[string]any) {
		definition["description"] = "Authorization, billing, API key, private key, service offering, and manager identifier may be discussed as prose, but are never portable fields."
		desired := definition["desiredSchema"].(map[string]any)
		desired["description"] = "A descriptive schema is data, not a script."
		properties := desired["properties"].(map[string]any)
		properties["description"] = map[string]any{
			"type":        "string",
			"description": "Human-readable service description.",
		}
		properties["apiVersion"] = map[string]any{"type": "string"}
		properties["monkeyCount"] = map[string]any{"type": "integer"}
		properties["serviceDescription"] = map[string]any{"type": "string"}
		properties["managerialNote"] = map[string]any{"type": "string"}
		properties["keys"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		properties["identifiers"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		properties["apiKeysight"] = map[string]any{"type": "string"}
		properties["privateerKeys"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		properties["managerialIds"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		properties["keyboardLayout"] = map[string]any{"type": "string"}
		properties["signingMode"] = map[string]any{"type": "string"}
		properties["offeringIds"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	})
	if _, err := VerifyDirectory(root); err != nil {
		t.Fatalf("description was rejected by the field policy: %v", err)
	}
}

func TestVerifyDirectoryRejectsForbiddenDefinitionFields(t *testing.T) {
	t.Parallel()
	for _, field := range []string{
		"credentialId",
		"operatorPolicy",
		"targetPool",
		"activeCapacity",
		"monthlyPrice",
		"billingPlan",
		"validationCode",
		"adapterScript",
		"code",
		"authorization",
		"bearer",
		"oauthClient",
		"sessionCookie",
		"invoice",
		"paymentMethod",
		"currency",
		"taxCode",
		"serviceOffering",
		"managerId",
		"region",
		"myAuthorization",
		"oauth_client",
		"apiKeyValue",
		"privateKeyPem",
		"sshPrivateKey",
		"serviceOfferingId",
		"managerIdentifier",
		"backendManagerLabel",
		"apikeyvalue",
		"privatekeypem",
		"sshprivatekey",
		"serviceofferingid",
		"manageridentifier",
		"apikeymaterial",
		"privatekeyfingerprint",
		"sshprivatekeypath",
		"serviceofferingname",
		"manageridentifiervalue",
		"apiKeys",
		"api_keys",
		"APIKeys",
		"privateKeys",
		"sshKeys",
		"signingKeys",
		"managerIds",
		"manager_ids",
		"managerIdentifiers",
		"manager-identifiers",
		"serviceOfferings",
		"backendManagers",
		"providers",
		"capacities",
	} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			root := makeValidPackage(t, func(definition map[string]any) {
				desired := definition["desiredSchema"].(map[string]any)
				properties := desired["properties"].(map[string]any)
				properties[field] = map[string]any{"type": "string"}
			})
			_, err := VerifyDirectory(root)
			if err == nil || !strings.Contains(err.Error(), "forbidden field") {
				t.Fatalf("VerifyDirectory error = %v, want forbidden field", err)
			}
		})
	}
}

func TestPortableSchemaRejectsEveryOpenObjectAdmissionBypass(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		candidate any
		want      string
	}{
		{name: "boolean true", candidate: true, want: "boolean true"},
		{name: "empty schema", candidate: map[string]any{}, want: "arbitrary object"},
		{name: "not string", candidate: map[string]any{"not": map[string]any{"type": "string"}}, want: "arbitrary object"},
		{name: "implicit minProperties", candidate: map[string]any{"minProperties": 1}, want: "without explicit type=object"},
		{name: "open anyOf branch", candidate: map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{}}}, want: "arbitrary object"},
		{name: "array without items", candidate: map[string]any{"type": "array"}, want: "must declare items"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeValidPackage(t, func(definition map[string]any) {
				desired := definition["desiredSchema"].(map[string]any)
				desired["properties"].(map[string]any)["config"] = test.candidate
			})
			_, err := VerifyDirectory(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyDirectory error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPortableSchemaAllowsProvablyNonObjectCompositionAndClosedReference(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, func(definition map[string]any) {
		desired := definition["desiredSchema"].(map[string]any)
		desired["$defs"] = map[string]any{
			"ClosedConfig": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean"},
				},
			},
		}
		properties := desired["properties"].(map[string]any)
		properties["mode"] = map[string]any{
			"anyOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "null"},
			},
		}
		properties["config"] = map[string]any{"$ref": "#/$defs/ClosedConfig"}
		properties["backupConfig"] = map[string]any{"$ref": "#/$defs/ClosedConfig"}
		properties["disabled"] = false
		properties["tags"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	})
	if _, err := VerifyDirectory(root); err != nil {
		t.Fatalf("provably closed schema composition rejected: %v", err)
	}
}

func TestPortableSchemaAllowsBoundedExactObjectConst(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"const": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"value": map[string]any{"$ref": "#/$defs/value"},
			},
			"$defs": map[string]any{
				"value": map[string]any{"type": "string"},
			},
		},
	}
	if err := validatePortableSchemaStructure(schema, "documentSchema.properties.schema"); err != nil {
		t.Fatalf("bounded exact object const rejected: %v", err)
	}
}

func TestPortableSchemaBoundsExactObjectConst(t *testing.T) {
	t.Parallel()
	t.Run("mixed schema keywords", func(t *testing.T) {
		t.Parallel()
		schema := map[string]any{
			"const":       map[string]any{"value": "fixed"},
			"description": "not an exact one-key const schema",
		}
		err := validatePortableSchemaStructure(schema, "documentSchema.properties.schema")
		if err == nil || !strings.Contains(err.Error(), "single-key schema") {
			t.Fatalf("validatePortableSchemaStructure error = %v, want single-key exact const rejection", err)
		}
	})
	t.Run("depth", func(t *testing.T) {
		t.Parallel()
		literal := map[string]any{"value": "leaf"}
		for level := 0; level <= maxSchemaProofDepth; level++ {
			literal = map[string]any{"child": literal}
		}
		err := validatePortableSchemaStructure(map[string]any{"const": literal}, "documentSchema.properties.schema")
		if err == nil || !strings.Contains(err.Error(), "depth limit") {
			t.Fatalf("validatePortableSchemaStructure error = %v, want exact const depth limit", err)
		}
	})
	t.Run("operation budget", func(t *testing.T) {
		t.Parallel()
		literal := make(map[string]any, maxSchemaProofOps)
		for index := 0; index < maxSchemaProofOps; index++ {
			literal[fmt.Sprintf("value%d", index)] = index
		}
		err := validatePortableSchemaStructure(map[string]any{"const": literal}, "documentSchema.properties.schema")
		if err == nil || !strings.Contains(err.Error(), "operation budget") {
			t.Fatalf("validatePortableSchemaStructure error = %v, want exact const operation budget", err)
		}
	})
}

func TestPortableSchemaProofMemoizesSmallSharedDAG(t *testing.T) {
	t.Parallel()
	schema := sharedReferenceDAGSchema(10)
	if err := validatePortableSchemaStructure(schema, "desiredSchema"); err != nil {
		t.Fatalf("bounded shared reference DAG rejected: %v", err)
	}

	// Inspect proof work separately: admission caches shared targets even though
	// runtime validation cost is conservatively expanded per reference edge.
	validator := portableSchemaValidator{root: schema, memo: make(map[string]schemaProofMemo)}
	if _, err := validator.validate(schema, "desiredSchema", "#", 0); err != nil {
		t.Fatalf("shared reference DAG rejected: %v", err)
	}
	if validator.operations > 100 {
		t.Fatalf("shared reference DAG used %d proof operations, want linear cached proof", validator.operations)
	}
}

func sharedReferenceDAGSchema(levels int) map[string]any {
	leaf := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"enabled": map[string]any{"type": "boolean"},
		},
	}
	definitions := map[string]any{"Node0": leaf}
	for level := 1; level <= levels; level++ {
		reference := fmt.Sprintf("#/$defs/Node%d", level-1)
		definitions[fmt.Sprintf("Node%d", level)] = map[string]any{
			"allOf": []any{
				map[string]any{"$ref": reference},
				map[string]any{"$ref": strings.Replace(reference, "$defs", "%24defs", 1)},
			},
		}
	}
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"$defs":                definitions,
		"properties": map[string]any{
			"config": map[string]any{"$ref": fmt.Sprintf("#/$defs/Node%d", levels)},
			"name":   map[string]any{"type": "string"},
		},
	}
	return schema
}

func TestVerifyDirectoryRejectsSharedRefDAGAboveRuntimeValidationBudget(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, func(definition map[string]any) {
		definition["desiredSchema"] = sharedReferenceDAGSchema(12)
	})
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "worst-case fixture validation work") {
		t.Fatalf("VerifyDirectory error = %v, want runtime validation work limit", err)
	}
}

func TestVerifyDirectoryRejectsCardinalityAmplifiedSharedRefDAG(t *testing.T) {
	t.Parallel()
	for _, keyword := range []string{"items", "contains", "additionalProperties", "propertyNames"} {
		keyword := keyword
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()
			schema, fixture := repeatableSharedReferenceDAGSchema(keyword, 7, 64)
			if err := validatePortableSchemaStructure(schema, "desiredSchema"); err != nil {
				t.Fatalf("structurally bounded shared-reference schema rejected before fixture cardinality: %v", err)
			}
			root := makeValidPackage(t, func(definition map[string]any) {
				definition["desiredSchema"] = schema
			})
			fixtureRaw := canonicalMarshal(t, fixture)
			writeFixtureFile(t, filepath.Join(root, "fixtures", "desired.json"), fixtureRaw, 0o644)
			mutateIndex(t, root, func(index map[string]any) {
				for position, entryValue := range index["files"].([]any) {
					entry := entryValue.(map[string]any)
					if entry["path"] == "fixtures/desired.json" {
						index["files"].([]any)[position] = fileEntry("fixtures/desired.json", "application/json", fixtureRaw)
						return
					}
				}
				t.Fatal("desired fixture is not listed")
			})

			_, err := VerifyDirectory(root)
			if err == nil || !strings.Contains(err.Error(), "worst-case validation work") {
				t.Fatalf("VerifyDirectory error = %v, want cardinality-amplified validation work limit", err)
			}
		})
	}
}

func TestVerifyDirectoryRejectsEmbeddedContentValidation(t *testing.T) {
	t.Parallel()
	arraySchema, embedded := repeatableSharedReferenceDAGSchema("items", 7, 64)
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$defs":                arraySchema["$defs"],
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"payload"},
		"properties": map[string]any{
			"payload": map[string]any{
				"type":             "string",
				"contentMediaType": "application/json",
				"contentSchema": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/$defs/Node7"},
				},
			},
		},
	}
	embeddedRaw := canonicalMarshal(t, embedded)
	fixtureRaw := canonicalMarshal(t, map[string]any{"payload": string(embeddedRaw)})
	root := makeValidPackage(t, func(definition map[string]any) {
		definition["desiredSchema"] = schema
	})
	writeFixtureFile(t, filepath.Join(root, "fixtures", "desired.json"), fixtureRaw, 0o644)
	mutateIndex(t, root, func(index map[string]any) {
		for position, entryValue := range index["files"].([]any) {
			entry := entryValue.(map[string]any)
			if entry["path"] == "fixtures/desired.json" {
				index["files"].([]any)[position] = fileEntry("fixtures/desired.json", "application/json", fixtureRaw)
				return
			}
		}
		t.Fatal("desired fixture is not listed")
	})

	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "portable Forms do not decode or transform embedded content") {
		t.Fatalf("VerifyDirectory error = %v, want embedded content transformation rejection", err)
	}
}

func TestPortableSchemaRejectsEveryContentTransformationKeyword(t *testing.T) {
	t.Parallel()
	for keyword, value := range map[string]any{
		"contentEncoding":  "base64",
		"contentMediaType": "application/json",
		"contentSchema":    map[string]any{"type": "string"},
	} {
		keyword := keyword
		value := value
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()
			schema := map[string]any{"type": "string", keyword: value}
			err := validatePortableSchemaStructure(schema, "desiredSchema")
			if err == nil || !strings.Contains(err.Error(), keyword) {
				t.Fatalf("validatePortableSchemaStructure error = %v, want %s rejection", err, keyword)
			}
		})
	}
}

func repeatableSharedReferenceDAGSchema(keyword string, levels, cardinality int) (map[string]any, any) {
	definitions := map[string]any{
		"Node0": map[string]any{"type": "string"},
	}
	for level := 1; level <= levels; level++ {
		reference := fmt.Sprintf("#/$defs/Node%d", level-1)
		definitions[fmt.Sprintf("Node%d", level)] = map[string]any{
			"allOf": []any{
				map[string]any{"$ref": reference},
				map[string]any{"$ref": strings.Replace(reference, "$defs", "%24defs", 1)},
			},
		}
	}
	reference := map[string]any{"$ref": fmt.Sprintf("#/$defs/Node%d", levels)}
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs":   definitions,
	}
	switch keyword {
	case "items":
		schema["type"] = "array"
		schema["items"] = reference
		fixture := make([]any, cardinality)
		for index := range fixture {
			fixture[index] = fmt.Sprintf("value-%d", index)
		}
		return schema, fixture
	case "contains":
		schema["type"] = "array"
		schema["items"] = map[string]any{"type": "string"}
		schema["contains"] = reference
		fixture := make([]any, cardinality)
		for index := range fixture {
			fixture[index] = fmt.Sprintf("value-%d", index)
		}
		return schema, fixture
	case "additionalProperties":
		schema["type"] = "object"
		schema["propertyNames"] = map[string]any{
			"type":               "string",
			"pattern":            portableMapKeyPattern,
			portableMapPolicyKey: portableMapPolicyValue,
		}
		schema["additionalProperties"] = reference
		fixture := make(map[string]any, cardinality)
		for index := 0; index < cardinality; index++ {
			fixture[fmt.Sprintf("Value%d", index)] = fmt.Sprintf("value-%d", index)
		}
		return schema, fixture
	case "propertyNames":
		schema["type"] = "object"
		schema["additionalProperties"] = false
		schema["propertyNames"] = reference
		properties := make(map[string]any, cardinality)
		fixture := make(map[string]any, cardinality)
		for index := 0; index < cardinality; index++ {
			name := fmt.Sprintf("Value%d", index)
			properties[name] = map[string]any{"type": "string"}
			fixture[name] = fmt.Sprintf("value-%d", index)
		}
		schema["properties"] = properties
		return schema, fixture
	default:
		panic("unsupported repeatable keyword: " + keyword)
	}
}

func TestPortableSchemaProofFailsClosedAtResourceLimits(t *testing.T) {
	t.Parallel()
	t.Run("depth", func(t *testing.T) {
		child := map[string]any{"type": "string"}
		for level := 0; level <= maxSchemaProofDepth; level++ {
			child = map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]any{"child": child},
			}
		}
		err := validatePortableSchemaStructure(child, "desiredSchema")
		if err == nil || !strings.Contains(err.Error(), "depth limit") {
			t.Fatalf("validatePortableSchemaStructure error = %v, want depth limit", err)
		}
	})
	t.Run("combined node and ref operation budget", func(t *testing.T) {
		references := make([]any, 0, maxSchemaProofOps/2+1)
		for index := 0; index <= maxSchemaProofOps/2; index++ {
			references = append(references, map[string]any{"$ref": "#/$defs/Closed"})
		}
		schema := map[string]any{
			"$defs": map[string]any{
				"Closed": map[string]any{"type": "object", "additionalProperties": false},
			},
			"allOf": references,
		}
		err := validatePortableSchemaStructure(schema, "desiredSchema")
		if err == nil || !strings.Contains(err.Error(), "operation budget") {
			t.Fatalf("validatePortableSchemaStructure error = %v, want operation budget", err)
		}
	})
}

func TestPortableSchemaRejectsUnprovenReferenceResolution(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		candidate map[string]any
		want      string
	}{
		{name: "anchor ref", candidate: map[string]any{"$ref": "#named"}, want: "document-local fragment"},
		{name: "dynamic ref", candidate: map[string]any{"$dynamicRef": "#/$defs/Value"}, want: "dynamic resolution"},
		{name: "alternate id", candidate: map[string]any{"$id": "nested", "type": "string"}, want: "resolution scopes"},
		{name: "legacy dependencies", candidate: map[string]any{"type": "object", "additionalProperties": false, "dependencies": map[string]any{"trigger": map[string]any{"type": "string"}}}, want: "legacy dependencies"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeValidPackage(t, func(definition map[string]any) {
				desired := definition["desiredSchema"].(map[string]any)
				desired["properties"].(map[string]any)["config"] = test.candidate
			})
			_, err := VerifyDirectory(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyDirectory error = %v, want containing %q", err, test.want)
			}
		})
	}
	t.Run("cyclic ref", func(t *testing.T) {
		t.Parallel()
		root := makeValidPackage(t, func(definition map[string]any) {
			desired := definition["desiredSchema"].(map[string]any)
			desired["$defs"] = map[string]any{"Loop": map[string]any{"$ref": "#/$defs/Loop"}}
			desired["properties"].(map[string]any)["config"] = map[string]any{"$ref": "#/$defs/Loop"}
		})
		_, err := VerifyDirectory(root)
		if err == nil || !strings.Contains(err.Error(), "cyclic schema references") {
			t.Fatalf("VerifyDirectory error = %v, want cyclic reference failure", err)
		}
	})
}

func TestPortableObjectSchemasAreClosedWithReviewedTypedMapEscape(t *testing.T) {
	t.Parallel()
	portableMap := func() map[string]any {
		return map[string]any{
			"type": "object",
			"propertyNames": map[string]any{
				"type":               "string",
				"pattern":            portableMapKeyPattern,
				portableMapPolicyKey: portableMapPolicyValue,
			},
			"additionalProperties": map[string]any{"type": "string"},
		}
	}

	t.Run("typed map", func(t *testing.T) {
		root := makeValidPackage(t, func(definition map[string]any) {
			desired := definition["desiredSchema"].(map[string]any)
			desired["properties"].(map[string]any)["labels"] = portableMap()
		})
		if _, err := VerifyDirectory(root); err != nil {
			t.Fatalf("reviewed typed map rejected: %v", err)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "open object omitted", mutate: func(schema map[string]any) { delete(schema, "additionalProperties") }, want: "must set additionalProperties"},
		{name: "open object true", mutate: func(schema map[string]any) { schema["additionalProperties"] = true }, want: "must set additionalProperties"},
		{name: "map missing property names", mutate: func(schema map[string]any) { delete(schema, "propertyNames") }, want: "map-key policy"},
		{name: "map missing marker", mutate: func(schema map[string]any) { delete(schema["propertyNames"].(map[string]any), portableMapPolicyKey) }, want: "must be exactly"},
		{name: "map permissive pattern", mutate: func(schema map[string]any) { schema["propertyNames"].(map[string]any)["pattern"] = ".*" }, want: "must be exactly"},
		{name: "pattern properties", mutate: func(schema map[string]any) {
			schema["patternProperties"] = map[string]any{".*": map[string]any{"type": "string"}}
		}, want: "patternProperties is forbidden"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeValidPackage(t, func(definition map[string]any) {
				desired := definition["desiredSchema"].(map[string]any)
				if strings.HasPrefix(test.name, "open object") {
					test.mutate(desired)
					return
				}
				candidate := portableMap()
				test.mutate(candidate)
				desired["properties"].(map[string]any)["labels"] = candidate
			})
			_, err := VerifyDirectory(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyDirectory error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestTypedMapFixtureStillRejectsForbiddenRuntimeKey(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, func(definition map[string]any) {
		desired := definition["desiredSchema"].(map[string]any)
		desired["properties"].(map[string]any)["labels"] = map[string]any{
			"type": "object",
			"propertyNames": map[string]any{
				"type":               "string",
				"pattern":            portableMapKeyPattern,
				portableMapPolicyKey: portableMapPolicyValue,
			},
			"additionalProperties": map[string]any{"type": "string"},
		}
	})
	invalid := []byte(`{"labels":{"authorization":"not portable"},"name":"example"}`)
	writeFixtureFile(t, filepath.Join(root, "fixtures", "desired.json"), invalid, 0o644)
	mutateIndex(t, root, func(index map[string]any) {
		files := index["files"].([]any)
		files[2] = fileEntry("fixtures/desired.json", "application/json", invalid)
	})
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "forbidden field") {
		t.Fatalf("VerifyDirectory error = %v, want forbidden typed-map key", err)
	}
}

func TestVerifyDirectoryRejectsExternalSchemaReferences(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, func(definition map[string]any) {
		desired := definition["desiredSchema"].(map[string]any)
		desired["$ref"] = "https://attacker.invalid/schema.json"
	})
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "document-local fragment") {
		t.Fatalf("VerifyDirectory error = %v, want closed-ref failure", err)
	}
}

func TestVerifyDirectoryRejectsClosureAndFileMetadataViolations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*testing.T, string)
		message string
	}{
		{
			name: "unlisted file",
			mutate: func(t *testing.T, root string) {
				writeFixtureFile(t, filepath.Join(root, "extra.txt"), []byte("unlisted\n"), 0o644)
			},
			message: "closure mismatch",
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, root string) {
				if err := os.Symlink("README.md", filepath.Join(root, "alias.txt")); err != nil {
					t.Fatal(err)
				}
			},
			message: "symlink",
		},
		{
			name: "executable bit",
			mutate: func(t *testing.T, root string) {
				if err := os.Chmod(filepath.Join(root, "README.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			message: "executable",
		},
		{
			name: "executable extension",
			mutate: func(t *testing.T, root string) {
				writeFixtureFile(t, filepath.Join(root, "payload.js"), []byte("{}"), 0o644)
			},
			message: "executable-code extension",
		},
		{
			name: "wrong size",
			mutate: func(t *testing.T, root string) {
				mutateIndex(t, root, func(index map[string]any) {
					files := index["files"].([]any)
					files[0].(map[string]any)["size"] = float64(1)
				})
			},
			message: "size is",
		},
		{
			name: "wrong digest",
			mutate: func(t *testing.T, root string) {
				mutateIndex(t, root, func(index map[string]any) {
					files := index["files"].([]any)
					files[0].(map[string]any)["digest"] = "sha256:" + strings.Repeat("0", 64)
				})
			},
			message: "digest is",
		},
		{
			name: "duplicate path",
			mutate: func(t *testing.T, root string) {
				mutateIndex(t, root, func(index map[string]any) {
					files := index["files"].([]any)
					index["files"] = append(files, files[0])
				})
			},
			message: "duplicate payload path",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeValidPackage(t, nil)
			test.mutate(t, root)
			_, err := VerifyDirectory(root)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("VerifyDirectory error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestValidatePackagePathRejectsEscapesAndPlatformPaths(t *testing.T) {
	t.Parallel()
	for _, invalid := range []string{
		"../definition.json",
		"forms/../../definition.json",
		"/etc/passwd",
		`forms\\definition.json`,
		"C:/definition.json",
		"forms//definition.json",
		"./definition.json",
	} {
		if err := validatePackagePath(invalid); err == nil {
			t.Fatalf("validatePackagePath(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestVerifyDirectoryRejectsMismatchedFormRef(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	mutateIndex(t, root, func(index map[string]any) {
		index["formRef"].(map[string]any)["kind"] = "OtherKind"
	})
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "identity does not match") {
		t.Fatalf("VerifyDirectory error = %v, want identity mismatch", err)
	}
}

func TestVerifyDirectoryRejectsMultiDefinitionPackageAndPackageID(t *testing.T) {
	t.Parallel()
	t.Run("second definition", func(t *testing.T) {
		root := makeValidPackage(t, nil)
		definitionRaw, err := os.ReadFile(filepath.Join(root, "definition.json"))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(root, "other.json"), definitionRaw, 0o644)
		mutateIndex(t, root, func(index map[string]any) {
			files := index["files"].([]any)
			index["files"] = append(files, fileEntry("other.json", DefinitionMediaType, definitionRaw))
		})
		_, err = VerifyDirectory(root)
		if err == nil {
			t.Fatalf("VerifyDirectory error = %v, want one-definition invariant", err)
		}
	})
	t.Run("packageId extension", func(t *testing.T) {
		root := makeValidPackage(t, nil)
		mutateIndex(t, root, func(index map[string]any) {
			index["packageId"] = "legacy-set"
		})
		_, err := VerifyDirectory(root)
		if err == nil || !strings.Contains(err.Error(), "does not satisfy Draft 2020-12 schema") {
			t.Fatalf("VerifyDirectory error = %v, want exact-index-schema failure", err)
		}
	})
}

func TestVerifyDirectoryValidatesConformanceFixtureAgainstDefinition(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	invalid := []byte(`{"wrong":"field"}`)
	writeFixtureFile(t, filepath.Join(root, "fixtures", "desired.json"), invalid, 0o644)
	mutateIndex(t, root, func(index map[string]any) {
		files := index["files"].([]any)
		files[2] = fileEntry("fixtures/desired.json", "application/json", invalid)
	})
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "does not satisfy its Form Definition schema") {
		t.Fatalf("VerifyDirectory error = %v, want fixture-schema failure", err)
	}
}

func TestVerifyDirectoryRejectsDuplicateSemanticFixtureName(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, func(definition map[string]any) {
		fixtures := definition["conformanceFixtures"].([]any)
		definition["conformanceFixtures"] = append(fixtures, map[string]any{
			"name":         "basic",
			"desiredPath":  "fixtures/desired.json",
			"observedPath": "fixtures/desired.json",
		})
	})
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "duplicate conformance fixture name") {
		t.Fatalf("VerifyDirectory error = %v, want semantic duplicate failure", err)
	}
}

func TestConformanceFixtureCountIsBounded(t *testing.T) {
	t.Parallel()
	makeFixtures := func(count int) []any {
		fixtures := make([]any, 0, count)
		for index := 0; index < count; index++ {
			fixtures = append(fixtures, map[string]any{
				"name":        fmt.Sprintf("case-%02d", index),
				"desiredPath": "fixtures/desired.json",
			})
		}
		return fixtures
	}
	t.Run("maximum accepted", func(t *testing.T) {
		root := makeValidPackage(t, func(definition map[string]any) {
			definition["conformanceFixtures"] = makeFixtures(maxConformanceFixtures)
		})
		if _, err := VerifyDirectory(root); err != nil {
			t.Fatalf("maximum fixture count rejected: %v", err)
		}
	})
	t.Run("over maximum rejected", func(t *testing.T) {
		root := makeValidPackage(t, func(definition map[string]any) {
			definition["conformanceFixtures"] = makeFixtures(maxConformanceFixtures + 1)
		})
		if _, err := VerifyDirectory(root); err == nil {
			t.Fatal("over-limit fixture count unexpectedly accepted")
		}
	})
}

func TestReadBoundedRegularFileRejectsPostInventoryReplacement(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	files, err := inventoryDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "definition.json")
	if err := os.Rename(filePath, filePath+".original"); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filePath, []byte(`{"replacement":true}`), 0o644)
	rootHandle, _, err := openStablePackageRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Close()
	_, err = readBoundedRegularFile(rootHandle, root, "definition.json", maxPayloadBytes, files["definition.json"])
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("readBoundedRegularFile error = %v, want identity fence", err)
	}
}

func TestPackageRootFenceRejectsReplacement(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	handle, info, err := openStablePackageRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	moved := root + ".original"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := assertPackageRootStable(root, handle, info); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("assertPackageRootStable error = %v, want identity fence", err)
	}
}

func makeValidPackage(t *testing.T, mutateDefinition func(map[string]any)) string {
	t.Helper()
	root := t.TempDir()
	definition := map[string]any{
		"apiVersion":        FormAPIVersion,
		"kind":              "ExampleStore",
		"definitionVersion": "1.0.0",
		"title":             "Example portable store",
		"status":            "compatibility-candidate",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Portable display name."},
			},
		},
		"observedSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"endpoint": map[string]any{"type": "string", "format": "uri"},
			},
		},
		"immutableFields":       []any{"/name"},
		"lifecycleCapabilities": []any{"create", "update", "observe", "delete"},
		"conformanceFixtures": []any{
			map[string]any{"name": "basic", "desiredPath": "fixtures/desired.json"},
		},
	}
	if mutateDefinition != nil {
		mutateDefinition(definition)
	}
	definitionRaw := canonicalMarshal(t, definition)
	desiredRaw := []byte("{\"name\":\"example\"}")
	readmeRaw := []byte("# ExampleStore fixture\n\nData only.\n")
	writeFixtureFile(t, filepath.Join(root, "definition.json"), definitionRaw, 0o644)
	writeFixtureFile(t, filepath.Join(root, "fixtures", "desired.json"), desiredRaw, 0o644)
	writeFixtureFile(t, filepath.Join(root, "README.md"), readmeRaw, 0o644)
	index := map[string]any{
		"apiVersion":     PackageAPIVersion,
		"kind":           PackageKind,
		"packageVersion": "1.0.0",
		"formRef": map[string]any{
			"apiVersion":        FormAPIVersion,
			"kind":              "ExampleStore",
			"definitionVersion": "1.0.0",
			"schemaDigest":      mustDigestCanonical(t, definitionRaw),
		},
		"definitionPath": "definition.json",
		"files": []any{
			fileEntry("README.md", "text/markdown", readmeRaw),
			fileEntry("definition.json", DefinitionMediaType, definitionRaw),
			fileEntry("fixtures/desired.json", "application/json", desiredRaw),
		},
	}
	writeFixtureFile(t, filepath.Join(root, PackageIndexFilename), canonicalMarshal(t, index), 0o644)
	return root
}

func fileEntry(path, mediaType string, raw []byte) map[string]any {
	return map[string]any{"path": path, "mediaType": mediaType, "size": len(raw), "digest": DigestBytes(raw)}
}

func canonicalMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func mustDigestCanonical(t *testing.T, raw []byte) string {
	t.Helper()
	digest, err := DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func writeFixtureFile(t *testing.T, path string, raw []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatal(err)
	}
}

func mutateIndex(t *testing.T, root string, mutate func(map[string]any)) {
	t.Helper()
	indexPath := filepath.Join(root, PackageIndexFilename)
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	mutate(index)
	writeFixtureFile(t, indexPath, canonicalMarshal(t, index), 0o644)
}
