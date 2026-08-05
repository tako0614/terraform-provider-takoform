package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

const (
	hostAPIWireSchemaID        = "https://forms.takoform.com/schemas/v1alpha1/host-api-wire.schema.json"
	currentHostAPIWireSchemaID = "https://forms.takoform.com/schemas/v1alpha2/host-api-wire.schema.json"
)

func TestEveryNormativeSchemaCompiles(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("schemas")
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	ids := make([]string, 0, len(entries))
	schemaFiles := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		schemaFiles++
		path := "schemas/" + entry.Name()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		id, ok := value.(map[string]any)["$id"].(string)
		if !ok || id == "" {
			t.Fatalf("%s has no schema $id", path)
		}
		if err := compiler.AddResource(id, value); err != nil {
			t.Fatalf("register %s: %v", path, err)
		}
		ids = append(ids, id)
	}
	if len(ids) != schemaFiles {
		t.Fatalf("registered schema IDs = %d, schema files = %d", len(ids), schemaFiles)
	}
	for _, id := range ids {
		if _, err := compiler.Compile(id); err != nil {
			t.Errorf("compile %s: %v", id, err)
		}
	}
}

func TestHostAPIWireSchema(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"",
		"#/$defs/spaceId",
		"#/$defs/resourceRequest",
		"#/$defs/resourceResponse",
		"#/$defs/previewResponse",
		"#/$defs/applyRequest",
		"#/$defs/importRequest",
		"#/$defs/resourceEnvelope",
		"#/$defs/formsResponse",
		"#/$defs/interfaceDeclaration",
		"#/$defs/interfacesResponse",
		"#/$defs/credentialFreeHTTPSURL",
		"#/$defs/errorEnvelope",
	} {
		t.Run("compiles "+fragment, func(t *testing.T) {
			compileHostAPIWire(t, fragment)
		})
	}

	requestSchema := compileHostAPIWire(t, "#/$defs/resourceRequest")
	assertSchemaValid(t, requestSchema, resourceRequest())
	spaceSchema := compileHostAPIWire(t, "#/$defs/spaceId")
	for _, validSpace := range []string{
		"a",
		"Prod",
		"Prod North",
		"prod\u00a0north",
		"prod\u2028north",
		"prod\ufeffnorth",
		strings.Repeat("界", 255),
		strings.Repeat("🐙", 255),
	} {
		t.Run("valid SpaceID "+validSpace, func(t *testing.T) {
			assertSchemaValid(t, spaceSchema, validSpace)
		})
	}
	for _, invalidSpace := range []string{
		"",
		strings.Repeat("界", 256),
		strings.Repeat("🐙", 256),
		" leading",
		"trailing ",
		"\u00a0leading",
		"trailing\u3000",
		"\u0085leading",
		"\u2028leading",
		"trailing\u2029",
		"\ufeffleading",
		"trailing\ufeff",
		"has/slash",
		"has\x00control",
		"has\tcontrol",
	} {
		t.Run("invalid SpaceID "+invalidSpace, func(t *testing.T) {
			assertSchemaInvalid(t, spaceSchema, invalidSpace)
		})
	}
	for _, controlRange := range [][2]rune{{0x00, 0x1f}, {0x7f, 0x9f}} {
		for candidate := controlRange[0]; candidate <= controlRange[1]; candidate++ {
			t.Run(fmt.Sprintf("invalid embedded control U+%04X", candidate), func(t *testing.T) {
				assertSchemaInvalid(t, spaceSchema, "a"+string(candidate)+"b")
			})
		}
	}
	spaceRequest := resourceRequest()
	spaceRequest["metadata"].(map[string]any)["space"] = "Prod North"
	assertSchemaValid(t, requestSchema, spaceRequest)
	canonicalBoundaryName := "a" + strings.Repeat("0", 62)
	boundaryRequest := resourceRequest()
	boundaryRequest["metadata"].(map[string]any)["name"] = canonicalBoundaryName
	assertSchemaValid(t, requestSchema, boundaryRequest)
	for _, invalidName := range []string{
		"",
		"Assets",
		"1assets",
		"asset_name",
		"asset.name",
		"a" + strings.Repeat("0", 63),
	} {
		t.Run("resource metadata rejects non-canonical name "+invalidName, func(t *testing.T) {
			candidate := resourceRequest()
			candidate["metadata"].(map[string]any)["name"] = invalidName
			assertSchemaInvalid(t, requestSchema, candidate)
		})
	}

	responseSchema := compileHostAPIWire(t, "#/$defs/resourceResponse")
	assertSchemaValid(t, responseSchema, resourceResponse("1", true))
	assertSchemaValid(t, responseSchema, resourceResponse("9223372036854775807", true))
	assertSchemaValid(t, responseSchema, resourceResponse("1", false))
	for name, invalid := range map[string]map[string]any{
		"missing observed":      resourceResponseWithoutObserved(),
		"zero generation":       resourceResponse("0", true),
		"leading zero":          resourceResponse("01", true),
		"opaque generation":     resourceResponse("host-token", true),
		"int64 overflow":        resourceResponse("9223372036854775808", true),
		"twenty decimal digits": resourceResponse("10000000000000000000", true),
		"unexpected top level":  resourceResponseWithExtraField(),
	} {
		t.Run(name, func(t *testing.T) {
			assertSchemaInvalid(t, responseSchema, invalid)
		})
	}

	previewSchema := compileHostAPIWire(t, "#/$defs/previewResponse")
	preview := map[string]any{
		"resource": resourceRequest(),
		"review": map[string]any{
			"planDigest": digest("c"),
			"specDigest": digest("d"),
		},
	}
	assertSchemaValid(t, previewSchema, preview)
	preview["selectedTarget"] = "host-private"
	assertSchemaInvalid(t, previewSchema, preview)

	applySchema := compileHostAPIWire(t, "#/$defs/applyRequest")
	apply := resourceRequest()
	apply["review"] = map[string]any{"planDigest": digest("c")}
	assertSchemaValid(t, applySchema, apply)
	apply["review"].(map[string]any)["specDigest"] = digest("d")
	assertSchemaInvalid(t, applySchema, apply)

	errorSchema := compileHostAPIWire(t, "#/$defs/errorEnvelope")
	assertSchemaValid(t, errorSchema, errorEnvelope("resource_busy", true))
	assertSchemaValid(t, errorSchema, errorEnvelope("backend_unavailable", false))
	assertSchemaInvalid(t, errorSchema, errorEnvelope("invalid_argument", true))
	assertSchemaInvalid(t, errorSchema, errorEnvelope("resource_version_conflict", true))

	interfaceSchema := compileHostAPIWire(t, "#/$defs/interfaceDeclaration")
	declaration := interfaceDeclaration()
	assertSchemaValid(t, interfaceSchema, declaration)
	declaration["inputs"] = []any{}
	assertSchemaInvalid(t, interfaceSchema, declaration)
	boundaryDeclaration := interfaceDeclaration()
	boundaryDeclaration["resource"].(map[string]any)["name"] = canonicalBoundaryName
	assertSchemaValid(t, interfaceSchema, boundaryDeclaration)
	for _, invalidName := range []string{
		"",
		"Assets",
		"1assets",
		"asset_name",
		"asset.name",
		"a" + strings.Repeat("0", 63),
	} {
		t.Run("Interface Resource reference rejects non-canonical name "+invalidName, func(t *testing.T) {
			candidate := interfaceDeclaration()
			candidate["resource"].(map[string]any)["name"] = invalidName
			assertSchemaInvalid(t, interfaceSchema, candidate)
		})
	}
	for _, resourceURI := range []string{
		"https://runtime.example.invalid",
		"https://runtime.example.invalid:8443/oauth/resource",
		"https://xn--r8jz45g.xn--zckzah/oauth/resource",
		"https://runtime.example.invalid/%E8%9B%B8/runtime",
		"https://runtime.example.invalid/蛸/runtime",
	} {
		t.Run("Interface resourceUri accepts "+resourceURI, func(t *testing.T) {
			candidate := interfaceDeclaration()
			candidate["resourceUri"] = resourceURI
			assertSchemaValid(t, interfaceSchema, candidate)
		})
	}
	for _, resourceURI := range []string{
		"",
		"http://runtime.example.invalid/oauth/resource",
		"https://user@runtime.example.invalid/oauth/resource",
		"https://runtime.example.invalid/oauth/resource?audience=one",
		"https://runtime.example.invalid/oauth/resource#fragment",
		"https://例え.テスト/oauth/resource",
		"https://localhost/oauth/resource",
		"https://runtime.example.invalid:123456/oauth/resource",
		"https://runtime.example.invalid/has space",
		"https://runtime.example.invalid/has\u00a0space",
		"https://runtime.example.invalid/%ZZ",
		"https://runtime.example.invalid/has\x00control",
	} {
		t.Run("Interface resourceUri rejects "+resourceURI, func(t *testing.T) {
			candidate := interfaceDeclaration()
			candidate["resourceUri"] = resourceURI
			assertSchemaInvalid(t, interfaceSchema, candidate)
		})
	}
	interfacesResponseSchema := compileHostAPIWire(t, "#/$defs/interfacesResponse")
	assertSchemaValid(t, interfacesResponseSchema, map[string]any{"interfaces": []any{}})
	assertSchemaInvalid(t, interfacesResponseSchema, map[string]any{})
	assertSchemaInvalid(t, interfacesResponseSchema, map[string]any{"interfaces": nil})

	uppercaseDigest := resourceRequest()
	uppercaseDigest["form"].(map[string]any)["packageDigest"] = "sha256:" + strings.Repeat("A", 64)
	assertSchemaInvalid(t, requestSchema, uppercaseDigest)

	wire := readJSONMap(t, "schemas/host-api-wire.schema.json")
	defs := wire["$defs"].(map[string]any)
	if got := defs["credentialFreeHTTPSURL"].(map[string]any)["pattern"]; got != formcatalog.PatternCredentialFreeHTTPSURL {
		t.Errorf(
			"credentialFreeHTTPSURL pattern = %#v, want shared grammar %q",
			got,
			formcatalog.PatternCredentialFreeHTTPSURL,
		)
	}
	for _, definition := range []string{"resourceMetadata", "interfaceResourceReference"} {
		properties := defs[definition].(map[string]any)["properties"].(map[string]any)
		if got := properties["name"].(map[string]any)["pattern"]; got != formcatalog.PatternName {
			t.Errorf("%s.name pattern = %#v, want canonical PatternName %q", definition, got, formcatalog.PatternName)
		}
	}
	metadataProperties := defs["resourceMetadata"].(map[string]any)["properties"].(map[string]any)
	if !reflect.DeepEqual(
		metadataProperties["space"],
		map[string]any{"$ref": "#/$defs/spaceId"},
	) {
		t.Errorf(
			"resourceMetadata.space = %#v, want the dedicated SpaceID schema",
			metadataProperties["space"],
		)
	}
	resourceProperties := defs["resourceCore"].(map[string]any)["properties"].(map[string]any)
	if got := resourceProperties["kind"].(map[string]any)["x-takoform-equals"]; got != "/form/formRef/kind" {
		t.Errorf("Resource kind semantic equality annotation = %#v", got)
	}
	if got := resourceProperties["spec"].(map[string]any)["x-takoform-validatesAgainst"]; got != "installed Form Definition desiredSchema" {
		t.Errorf("Resource spec semantic schema annotation = %#v", got)
	}
	statusProperties := defs["resourceStatus"].(map[string]any)["properties"].(map[string]any)
	if got := statusProperties["observed"].(map[string]any)["x-takoform-validatesAgainst"]; got != "installed Form Definition observedSchema" {
		t.Errorf("observed semantic schema annotation = %#v", got)
	}
	output := statusProperties["output"].(map[string]any)
	if got := output["x-takoform-validatesAgainst"]; got != "installed Form Definition outputSchema" {
		t.Errorf("output semantic schema annotation = %#v", got)
	}
	if got := output["x-takoform-requiredWhen"]; got != "installed Form Definition declares outputSchema" {
		t.Errorf("output conditional-presence annotation = %#v", got)
	}
	if got := output["x-takoform-omittedWhen"]; got != "installed Form Definition does not declare outputSchema" {
		t.Errorf("output conditional-omission annotation = %#v", got)
	}
}

func TestCurrentHostAPIWireCarriesOnlyCurrentFormRefs(t *testing.T) {
	t.Parallel()

	currentSchema := compileCurrentHostAPIWire(t, "#/$defs/resourceRequest")
	current := resourceRequest()
	current["apiVersion"] = "forms.takoform.com/v1alpha2"
	current["form"].(map[string]any)["formRef"].(map[string]any)["apiVersion"] =
		"forms.takoform.com/v1alpha2"
	assertSchemaValid(t, currentSchema, current)
	assertSchemaInvalid(t, currentSchema, resourceRequest())
	assertSchemaInvalid(t, compileHostAPIWire(t, "#/$defs/resourceRequest"), current)
}

func TestCurrentHostAPIFormDefinitionResponseIsExactAndClosed(t *testing.T) {
	t.Parallel()

	schema := compileCurrentHostAPIWire(t, "#/$defs/formDefinitionResponse")
	resource := resourceRequest()
	resource["apiVersion"] = "forms.takoform.com/v1alpha2"
	resource["form"].(map[string]any)["formRef"].(map[string]any)["apiVersion"] =
		"forms.takoform.com/v1alpha2"
	response := map[string]any{
		"identity":    resource["form"],
		"displayName": "Object bucket",
		"description": "Stores opaque objects.",
		"desiredSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		},
	}
	assertSchemaValid(t, schema, response)

	legacy := cloneJSONValue(t, response)
	legacy["identity"].(map[string]any)["formRef"].(map[string]any)["apiVersion"] =
		"forms.takoform.com/v1alpha1"
	assertSchemaInvalid(t, schema, legacy)
	extra := cloneJSONValue(t, response)
	extra["hostImplementation"] = "worker"
	assertSchemaInvalid(t, schema, extra)
}

func TestHostDiscoverySchemaIsClosed(t *testing.T) {
	t.Parallel()

	schema := compileNormativeSchema(t, "schemas/host-discovery.schema.json", "")
	valid := map[string]any{
		"api_versions": []any{"forms.takoform.com/v1alpha1"},
		"features": map[string]any{
			"service_forms":          true,
			"exact_form_ref":         true,
			"optimistic_concurrency": true,
			"idempotent_lifecycle":   true,
			"interface_declarations": true,
		},
		"endpoints": map[string]any{
			"api":         "https://host.example/apis/forms.takoform.com/v1alpha1",
			"forms":       "https://host.example/apis/forms.takoform.com/v1alpha1/forms",
			"interfaces":  "https://host.example/apis/forms.takoform.com/v1alpha1/interfaces",
			"oidc_issuer": "https://identity.example",
		},
	}
	assertSchemaValid(t, schema, valid)

	for name, mutate := range map[string]func(map[string]any){
		"query": func(value map[string]any) {
			value["endpoints"].(map[string]any)["api"] = "https://host.example/api?version=1"
		},
		"fragment": func(value map[string]any) {
			value["endpoints"].(map[string]any)["forms"] = "https://host.example/forms#current"
		},
		"userinfo": func(value map[string]any) {
			value["endpoints"].(map[string]any)["interfaces"] = "https://user@host.example/interfaces"
		},
		"OIDC query": func(value map[string]any) {
			value["endpoints"].(map[string]any)["oidc_issuer"] = "https://identity.example?tenant=one"
		},
		"insecure OIDC": func(value map[string]any) {
			value["endpoints"].(map[string]any)["oidc_issuer"] = "http://identity.example"
		},
		"unknown endpoint": func(value map[string]any) {
			value["endpoints"].(map[string]any)["admin"] = "https://host.example/admin"
		},
		"unknown top level": func(value map[string]any) {
			value["vendor"] = "host"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneJSONValue(t, valid)
			mutate(candidate)
			assertSchemaInvalid(t, schema, candidate)
		})
	}

	raw := readJSONMap(t, "schemas/host-discovery.schema.json")
	endpoints := raw["properties"].(map[string]any)["endpoints"].(map[string]any)["properties"].(map[string]any)
	for _, endpoint := range []string{"forms", "interfaces"} {
		if got := endpoints[endpoint].(map[string]any)["x-takoform-sameOriginWith"]; got != "/endpoints/api" {
			t.Errorf("%s same-origin annotation = %#v", endpoint, got)
		}
	}
}

func TestCurrentHostDiscoveryRequiresTheCurrentEpoch(t *testing.T) {
	t.Parallel()

	schema := compileNormativeSchema(
		t,
		"schemas/host-discovery-v1alpha2.schema.json",
		"",
	)
	current := map[string]any{
		"api_versions": []any{"forms.takoform.com/v1alpha2"},
		"features": map[string]any{
			"service_forms":          true,
			"exact_form_ref":         true,
			"optimistic_concurrency": true,
			"idempotent_lifecycle":   true,
		},
		"endpoints": map[string]any{
			"api": "https://host.example/apis/forms.takoform.com/v1alpha2",
		},
	}
	assertSchemaValid(t, schema, current)
	mixed := cloneJSONValue(t, current)
	mixed["api_versions"] = []any{
		"forms.takoform.com/v1alpha2",
		"forms.takoform.com/v1alpha1",
	}
	assertSchemaInvalid(t, schema, mixed)
	legacyAPI := cloneJSONValue(t, current)
	legacyAPI["endpoints"].(map[string]any)["api"] =
		"https://host.example/apis/forms.takoform.com/v1alpha1"
	assertSchemaInvalid(t, schema, legacyAPI)
	current["api_versions"] = []any{"forms.takoform.com/v1alpha1"}
	assertSchemaInvalid(t, schema, current)
}

func TestHostOperationContractUsesWireSchemaAndStableErrors(t *testing.T) {
	t.Parallel()

	type operation struct {
		Name                    string     `json:"name"`
		Method                  string     `json:"method"`
		RequestSchema           string     `json:"requestSchema"`
		ResponseSchema          string     `json:"responseSchema"`
		SuccessResourceVersion  string     `json:"successResourceVersion"`
		SuccessStatus           []int      `json:"successStatus"`
		RequiredQueryParameters []string   `json:"requiredQueryParameters"`
		OptionalQueryParameters []string   `json:"optionalQueryParameters"`
		PairedQueryParameters   [][]string `json:"pairedQueryParameters"`
	}
	type operationContract struct {
		WireSchema            string            `json:"wireSchema"`
		QueryParameterSchemas map[string]string `json:"queryParameterSchemas"`
		ResourceVersion       struct {
			Encoding string `json:"encoding"`
			Minimum  string `json:"minimum"`
			Maximum  string `json:"maximum"`
			ETag     string `json:"etag"`
		} `json:"resourceVersion"`
		Operations []operation `json:"operations"`
		Error      struct {
			Schema                 string         `json:"schema"`
			Codes                  []string       `json:"codes"`
			AutomaticallyRetryable []string       `json:"automaticallyRetryable"`
			HTTPStatusByCode       map[string]int `json:"httpStatusByCode"`
		} `json:"errorEnvelope"`
	}
	var contract operationContract
	raw, err := os.ReadFile("host-api/operations.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.WireSchema != currentHostAPIWireSchemaID {
		t.Fatalf("wireSchema = %q, want %q", contract.WireSchema, currentHostAPIWireSchemaID)
	}
	wantSpaceSchema := currentHostAPIWireSchemaID + "#/$defs/spaceId"
	if !reflect.DeepEqual(
		contract.QueryParameterSchemas,
		map[string]string{"space": wantSpaceSchema},
	) {
		t.Fatalf(
			"query parameter schemas = %#v, want only SpaceID %q",
			contract.QueryParameterSchemas,
			wantSpaceSchema,
		)
	}
	compileCurrentHostAPIWire(
		t,
		strings.TrimPrefix(contract.QueryParameterSchemas["space"], currentHostAPIWireSchemaID),
	)
	if contract.ResourceVersion.Encoding != "canonical-decimal-string" ||
		contract.ResourceVersion.Minimum != "1" ||
		contract.ResourceVersion.Maximum != "9223372036854775807" ||
		contract.ResourceVersion.ETag != "exactly-one-strong-quoted-resource-version" {
		t.Fatalf("resourceVersion contract = %#v", contract.ResourceVersion)
	}
	if contract.Error.Schema != currentHostAPIWireSchemaID+"#/$defs/errorEnvelope" {
		t.Fatalf("error envelope schema = %q", contract.Error.Schema)
	}
	compileCurrentHostAPIWire(t, strings.TrimPrefix(contract.Error.Schema, currentHostAPIWireSchemaID))
	for _, operation := range contract.Operations {
		if operation.Name == "observe" || operation.Name == "refresh" {
			if operation.SuccessResourceVersion != "equals-if-match" {
				t.Errorf("%s success generation = %q, want equals-if-match", operation.Name, operation.SuccessResourceVersion)
			}
		} else if operation.SuccessResourceVersion != "" {
			t.Errorf("%s unexpectedly declares success generation semantics %q", operation.Name, operation.SuccessResourceVersion)
		}
		for field, reference := range map[string]string{
			"requestSchema":  operation.RequestSchema,
			"responseSchema": operation.ResponseSchema,
		} {
			if reference == "" {
				continue
			}
			if !strings.HasPrefix(reference, currentHostAPIWireSchemaID+"#/$defs/") {
				t.Errorf("%s %s does not select a wire-schema definition: %q", operation.Name, field, reference)
				continue
			}
			compileCurrentHostAPIWire(t, strings.TrimPrefix(reference, currentHostAPIWireSchemaID))
		}
		if operation.Name == "delete" {
			if !reflect.DeepEqual(operation.SuccessStatus, []int{204}) || operation.ResponseSchema != "" {
				t.Errorf("delete must have one empty 204 success response: %#v", operation)
			}
		} else if operation.ResponseSchema == "" {
			t.Errorf("%s has no success response schema", operation.Name)
		}
	}

	wantStatus := map[string]int{
		"invalid_argument":             400,
		"unauthenticated":              401,
		"permission_denied":            403,
		"form_unknown":                 404,
		"form_not_installed":           409,
		"form_unavailable":             503,
		"form_identity_conflict":       409,
		"resource_not_found":           404,
		"resource_version_conflict":    412,
		"resource_busy":                409,
		"import_conflict":              409,
		"policy_denied":                403,
		"backend_unavailable":          503,
		"interface_identity_ambiguous": 409,
		"interface_instance_ambiguous": 409,
		"internal_error":               500,
	}
	if !reflect.DeepEqual(contract.Error.HTTPStatusByCode, wantStatus) {
		t.Errorf("stable HTTP mapping = %#v, want %#v", contract.Error.HTTPStatusByCode, wantStatus)
	}
	if !reflect.DeepEqual(contract.Error.AutomaticallyRetryable, []string{"resource_busy", "backend_unavailable"}) {
		t.Errorf("automatic retry codes = %v", contract.Error.AutomaticallyRetryable)
	}

	wireErrors := wireErrorCodes(t)
	contractErrors := append([]string(nil), contract.Error.Codes...)
	mappedErrors := make([]string, 0, len(contract.Error.HTTPStatusByCode))
	for code := range contract.Error.HTTPStatusByCode {
		mappedErrors = append(mappedErrors, code)
	}
	sort.Strings(wireErrors)
	sort.Strings(contractErrors)
	sort.Strings(mappedErrors)
	if !reflect.DeepEqual(contractErrors, wireErrors) ||
		!reflect.DeepEqual(contractErrors, mappedErrors) {
		t.Errorf(
			"stable error sets disagree:\noperations=%v\nwire=%v\nhttp=%v",
			contractErrors,
			wireErrors,
			mappedErrors,
		)
	}

	for _, operation := range contract.Operations {
		switch operation.Name {
		case "interfaces":
			if !reflect.DeepEqual(operation.RequiredQueryParameters, []string{"space"}) ||
				len(operation.OptionalQueryParameters) != 0 ||
				len(operation.PairedQueryParameters) != 0 {
				t.Errorf("Interface list query vocabulary is not exact: %#v", operation)
			}
		case "interface":
			if !reflect.DeepEqual(operation.RequiredQueryParameters, []string{"space"}) ||
				!reflect.DeepEqual(operation.OptionalQueryParameters, []string{"version", "resourceKind", "resourceName"}) ||
				!reflect.DeepEqual(operation.PairedQueryParameters, [][]string{{"resourceKind", "resourceName"}}) {
				t.Errorf("Interface get query vocabulary is not exact: %#v", operation)
			}
		default:
			if operation.RequiredQueryParameters != nil ||
				operation.OptionalQueryParameters != nil ||
				operation.PairedQueryParameters != nil {
				t.Errorf("%s unexpectedly declares Interface query vocabulary", operation.Name)
			}
		}
	}
}

func compileHostAPIWire(t *testing.T, fragment string) *jsonschema.Schema {
	return compileVersionedHostAPIWire(
		t,
		"schemas/form-ref.schema.json",
		"schemas/host-api-wire.schema.json",
		hostAPIWireSchemaID,
		fragment,
	)
}

func compileCurrentHostAPIWire(t *testing.T, fragment string) *jsonschema.Schema {
	return compileVersionedHostAPIWire(
		t,
		"schemas/form-ref-v1alpha2.schema.json",
		"schemas/host-api-wire-v1alpha2.schema.json",
		currentHostAPIWireSchemaID,
		fragment,
	)
}

func compileVersionedHostAPIWire(
	t *testing.T,
	formRefPath string,
	wirePath string,
	wireID string,
	fragment string,
) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	for _, path := range []string{formRefPath, wirePath} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		id := value.(map[string]any)["$id"].(string)
		if err := compiler.AddResource(id, value); err != nil {
			t.Fatalf("register %s: %v", path, err)
		}
	}
	schema, err := compiler.Compile(wireID + fragment)
	if err != nil {
		t.Fatalf("compile wire schema %q: %v", fragment, err)
	}
	return schema
}

func compileNormativeSchema(t *testing.T, path, fragment string) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	id := value.(map[string]any)["$id"].(string)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(id, value); err != nil {
		t.Fatalf("register %s: %v", path, err)
	}
	schema, err := compiler.Compile(id + fragment)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return schema
}

func assertSchemaValid(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	if err := schema.Validate(value); err != nil {
		t.Fatalf("expected schema-valid value: %v", err)
	}
}

func assertSchemaInvalid(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	if err := schema.Validate(value); err == nil {
		t.Fatal("expected schema-invalid value")
	}
}

func resourceRequest() map[string]any {
	return map[string]any{
		"apiVersion": "forms.takoform.com/v1alpha1",
		"kind":       "ObjectBucket",
		"form": map[string]any{
			"formRef": map[string]any{
				"apiVersion":        "forms.takoform.com/v1alpha1",
				"kind":              "ObjectBucket",
				"definitionVersion": "3.0.0",
				"schemaDigest":      digest("a"),
			},
			"packageDigest": digest("b"),
		},
		"metadata": map[string]any{
			"name":  "assets",
			"space": "production",
		},
		"spec": map[string]any{"name": "assets"},
	}
}

func resourceResponse(version string, includeOutput bool) map[string]any {
	resource := resourceRequest()
	resource["metadata"].(map[string]any)["resourceVersion"] = version
	status := map[string]any{
		"observed": map[string]any{"id": "ObjectBucket/assets"},
	}
	if includeOutput {
		status["output"] = map[string]any{"name": "assets"}
	}
	resource["status"] = status
	return resource
}

func resourceResponseWithoutObserved() map[string]any {
	resource := resourceResponse("1", true)
	delete(resource["status"].(map[string]any), "observed")
	return resource
}

func resourceResponseWithExtraField() map[string]any {
	resource := resourceResponse("1", true)
	resource["target"] = "host-private"
	return resource
}

func interfaceDeclaration() map[string]any {
	return map[string]any{
		"name":    "object.storage",
		"version": "1",
		"resource": map[string]any{
			"kind": "ObjectBucket",
			"name": "assets",
		},
		"document": map[string]any{"operations": []any{"get", "put"}},
		"values":   map[string]any{"bucket": "assets"},
		"form": map[string]any{
			"formRef": map[string]any{
				"apiVersion":        "forms.takoform.com/v1alpha1",
				"kind":              "ObjectBucket",
				"definitionVersion": "3.0.0",
				"schemaDigest":      digest("a"),
			},
			"packageDigest": digest("b"),
		},
	}
}

func errorEnvelope(code string, retryable bool) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   "portable failure",
			"requestId": "request-1",
			"retryable": retryable,
		},
	}
}

func wireErrorCodes(t *testing.T) []string {
	t.Helper()
	var schema struct {
		Defs struct {
			ErrorBody struct {
				Properties struct {
					Code struct {
						Enum []string `json:"enum"`
					} `json:"code"`
				} `json:"properties"`
			} `json:"errorBody"`
		} `json:"$defs"`
	}
	raw, err := os.ReadFile("schemas/host-api-wire.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	return schema.Defs.ErrorBody.Properties.Code.Enum
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func cloneJSONValue(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func digest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
