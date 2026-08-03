package formcatalog

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestRelationalDatabaseSchemaFieldsAreOptionalAndTyped(t *testing.T) {
	t.Parallel()

	kind, ok := ByKind("RelationalDatabase")
	if !ok {
		t.Fatal("RelationalDatabase is not declared")
	}
	want := map[string]struct {
		wire    string
		grammar Grammar
	}{
		"schema_url":    {wire: "schemaUrl", grammar: GrammarCredentialFreeHTTPSURL},
		"schema_sha256": {wire: "schemaSha256", grammar: GrammarSHA256},
		"schema_format": {wire: "schemaFormat", grammar: GrammarToken},
	}
	for _, field := range kind.Fields {
		expected, tracked := want[field.HCL]
		if !tracked {
			continue
		}
		if field.Required {
			t.Errorf("%s must remain optional", field.HCL)
		}
		if field.Type != TypeString {
			t.Errorf("%s type = %q, want %q", field.HCL, field.Type, TypeString)
		}
		if field.Wire != expected.wire {
			t.Errorf("%s wire = %q, want %q", field.HCL, field.Wire, expected.wire)
		}
		if field.Grammar != expected.grammar {
			t.Errorf("%s grammar = %q, want %q", field.HCL, field.Grammar, expected.grammar)
		}
		delete(want, field.HCL)
	}
	for field := range want {
		t.Errorf("RelationalDatabase is missing %s", field)
	}
}

func TestArtifactFormsRequirePortableDigestBoundHTTPSBytes(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds {
		if !kind.Artifact {
			continue
		}
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			if err := validateDesiredForTest(kind, kind.CanonicalDesired()); err != nil {
				t.Fatalf("canonical desired state is invalid: %v", err)
			}
			for name, source := range map[string]map[string]any{
				"runner-local path": {"artifactPath": "./dist/app.tar"},
				"host-local ref": {
					"artifactRef":    "host/uploads/app.tar",
					"artifactSha256": "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
				},
				"unbound URL": {"artifactUrl": "https://artifacts.portable-conformance.invalid/app.tar"},
				"untyped bytes": {
					"artifactUrl":    "https://artifacts.portable-conformance.invalid/app.tar",
					"artifactSha256": "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
				},
				"URL userinfo": {
					"artifactUrl":       "https://builder@artifacts.portable-conformance.invalid/app.tar",
					"artifactSha256":    "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
					"artifactMediaType": "application/vnd.takoform.test+tar",
				},
				"URL query": {
					"artifactUrl":       "https://artifacts.portable-conformance.invalid/app.tar?download=1",
					"artifactSha256":    "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
					"artifactMediaType": "application/vnd.takoform.test+tar",
				},
				"URL fragment": {
					"artifactUrl":       "https://artifacts.portable-conformance.invalid/app.tar#archive",
					"artifactSha256":    "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
					"artifactMediaType": "application/vnd.takoform.test+tar",
				},
			} {
				desired := cloneValue(kind.CanonicalDesired()).(map[string]any)
				desired["source"] = source
				if err := validateDesiredForTest(kind, desired); err == nil {
					t.Errorf("%s source was accepted", name)
				}
			}
		})
	}
}

func TestArtifactFormsGenerateCredentialFreeURLNegativeCases(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"artifact-url-userinfo": "https://builder@artifacts.portable-conformance.invalid/app.tar",
		"artifact-url-query":    "https://artifacts.portable-conformance.invalid/app.tar?download=1",
		"artifact-url-fragment": "https://artifacts.portable-conformance.invalid/app.tar#archive",
	}
	for _, kind := range Kinds {
		if !kind.Artifact {
			continue
		}
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			cases, err := kind.NegativeCases()
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]string, len(want))
			for _, negative := range cases {
				source, ok := negative.Desired["source"].(map[string]any)
				if !ok {
					continue
				}
				if artifactURL, ok := source["artifactUrl"].(string); ok {
					if _, tracked := want[negative.Name]; tracked {
						got[negative.Name] = artifactURL
					}
				}
			}
			for name, value := range want {
				if got[name] != value {
					t.Errorf("%s = %q, want %q", name, got[name], value)
				}
			}
		})
	}
}

func TestIdentityClientRedirectURIKeepsItsSeparateHTTPSGrammar(t *testing.T) {
	t.Parallel()

	kind, ok := ByKind("IdentityClient")
	if !ok {
		t.Fatal("IdentityClient is not declared")
	}
	desired := cloneValue(kind.CanonicalDesired()).(map[string]any)
	desired["redirectUris"] = []any{"https://app.portable-conformance.invalid/callback?mode=oidc"}
	if err := validateDesiredForTest(kind, desired); err != nil {
		t.Fatalf("legitimate redirect URI query was narrowed with the artifact URL grammar: %v", err)
	}
}

func TestRequiredConnectionsHaveExplicitPortableCardinality(t *testing.T) {
	t.Parallel()

	exactlyOne := map[string]bool{
		"BackupPolicy":        true,
		"DnsRecord":           true,
		"HttpRoute":           true,
		"ObjectLifecycleRule": true,
		"RateLimitPolicy":     true,
		"Schedule":            true,
		"WebhookEndpoint":     true,
	}
	for _, kind := range Kinds {
		if kind.Connections != ConnectionsRequired {
			continue
		}
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			empty := cloneValue(kind.CanonicalDesired()).(map[string]any)
			empty["connections"] = map[string]any{}
			if err := validateDesiredForTest(kind, empty); err == nil {
				t.Error("empty required connections were accepted")
			}

			multiple := cloneValue(kind.CanonicalDesired()).(map[string]any)
			connections := multiple["connections"].(map[string]any)
			for _, connection := range connections {
				connections["second"] = cloneValue(connection)
				break
			}
			err := validateDesiredForTest(kind, multiple)
			if exactlyOne[kind.Kind] && err == nil {
				t.Error("a second connection was accepted by an exactly-one Form")
			}
			if !exactlyOne[kind.Kind] && err != nil {
				t.Errorf("multi-backend Form rejected a second connection: %v", err)
			}
		})
	}
}

func TestObjectLifecycleRuleIsOneUnambiguousAction(t *testing.T) {
	t.Parallel()

	kind, ok := ByKind("ObjectLifecycleRule")
	if !ok {
		t.Fatal("ObjectLifecycleRule is not declared")
	}
	desired := kind.CanonicalDesired()
	if desired["action"] != "expire" || desired["afterDays"] != 90 {
		t.Fatalf("canonical lifecycle action is ambiguous: %#v", desired)
	}
	for _, obsolete := range []string{"expireAfterDays", "transitionAfterDays", "transitionStorageClass"} {
		if _, exists := desired[obsolete]; exists {
			t.Errorf("obsolete cross-field lifecycle key %s remains", obsolete)
		}
	}
	for _, required := range []string{"action", "afterDays"} {
		invalid := cloneValue(desired).(map[string]any)
		delete(invalid, required)
		if err := validateDesiredForTest(kind, invalid); err == nil {
			t.Errorf("lifecycle rule without %s was accepted", required)
		}
	}
}

func TestDnsRecordValuesMatchTheirDeclaredType(t *testing.T) {
	t.Parallel()

	kind, _ := ByKind("DnsRecord")
	cases := []struct {
		recordType string
		value      string
		valid      bool
	}{
		{recordType: "A", value: "192.0.2.10", valid: true},
		{recordType: "A", value: "999.999.999.999", valid: false},
		{recordType: "AAAA", value: "2001:db8::10", valid: true},
		{recordType: "AAAA", value: "2001:::10", valid: false},
		{recordType: "CNAME", value: "service.portable-conformance.invalid", valid: true},
		{recordType: "CNAME", value: "not a hostname", valid: false},
		{recordType: "MX", value: "10 mail.portable-conformance.invalid", valid: true},
		{recordType: "MX", value: "mail.portable-conformance.invalid", valid: false},
	}
	for _, test := range cases {
		desired := cloneValue(kind.CanonicalDesired()).(map[string]any)
		desired["recordType"] = test.recordType
		desired["values"] = []any{test.value}
		err := validateDesiredForTest(kind, desired)
		if (err == nil) != test.valid {
			t.Errorf("%s %q validation error = %v, want valid %t", test.recordType, test.value, err, test.valid)
		}
	}
	multipleCNAMEs := cloneValue(kind.CanonicalDesired()).(map[string]any)
	multipleCNAMEs["values"] = []any{"one.portable-conformance.invalid", "two.portable-conformance.invalid"}
	if err := validateDesiredForTest(kind, multipleCNAMEs); err == nil {
		t.Error("CNAME record accepted more than one target")
	}
}

func TestLoadBalancerHealthPathIsOnlyAnHTTPContract(t *testing.T) {
	t.Parallel()

	kind, _ := ByKind("LoadBalancer")
	for _, protocol := range []string{"tcp", "udp"} {
		desired := cloneValue(kind.CanonicalDesired()).(map[string]any)
		desired["protocol"] = protocol
		if err := validateDesiredForTest(kind, desired); err == nil {
			t.Errorf("%s listener accepted an HTTP health_check_path", protocol)
		}
		delete(desired, "healthCheckPath")
		if err := validateDesiredForTest(kind, desired); err != nil {
			t.Errorf("%s listener without HTTP health path is invalid: %v", protocol, err)
		}
	}
}

func TestConditionalConstraintsHaveMatchingRuntimeValidation(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds {
		if len(kind.Constraints) == 0 {
			continue
		}
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			violations, err := kind.ConditionalViolations(kind.CanonicalDesired())
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 0 {
				t.Fatalf("canonical desired state has runtime violations: %#v", violations)
			}
			for _, constraint := range kind.Constraints {
				desired := cloneValue(kind.CanonicalDesired()).(map[string]any)
				for field, value := range constraint.CounterExample {
					desired[field] = cloneValue(value)
				}
				violations, err := kind.ConditionalViolations(desired)
				if err != nil {
					t.Fatal(err)
				}
				if len(violations) != 1 {
					t.Errorf("%s runtime violations = %#v, want exactly one", constraint.Name, violations)
				}
			}
		})
	}
}

func TestLifecycleSchemasUseCanonicalResourceIdentityGrammar(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			if err := validateSchemaValueForTest(kind.OutputSchema(), kind.CanonicalOutput()); err != nil {
				t.Fatalf("canonical output is invalid: %v", err)
			}
			for name, invalid := range map[string]map[string]any{
				"foreign kind id":   cloneWithPatch(kind.CanonicalOutput(), map[string]any{"id": "OtherKind/" + kind.FixtureName()}),
				"noncanonical name": cloneWithPatch(kind.CanonicalOutput(), map[string]any{"name": "UPPER_CASE"}),
			} {
				if err := validateSchemaValueForTest(kind.OutputSchema(), invalid); err == nil {
					t.Errorf("%s output was accepted", name)
				}
			}

			if err := validateSchemaValueForTest(kind.ObservedSchema(), kind.CanonicalObserved()); err != nil {
				t.Fatalf("canonical observed state is invalid: %v", err)
			}
			for name, invalidObserved := range map[string]map[string]any{
				"opaque id":    cloneWithPatch(kind.CanonicalObserved(), map[string]any{"id": "opaque-host-id"}),
				"foreign kind": kind.ForeignKindObserved(),
			} {
				if err := validateSchemaValueForTest(kind.ObservedSchema(), invalidObserved); err == nil {
					t.Errorf("%s observed resource id was accepted", name)
				}
			}
		})
	}
}

func TestExecutableAndModelFormsCarryTheirOwnDigestBoundBytes(t *testing.T) {
	t.Parallel()

	expectedVersions := map[string]string{
		"ComputeInstance": "3.0.0",
		"ModelEndpoint":   "4.0.0",
		"StatefulEntity":  "4.0.0",
	}
	for kindName, version := range expectedVersions {
		kind, ok := ByKind(kindName)
		if !ok {
			t.Fatalf("%s is not declared", kindName)
		}
		if !kind.Artifact || kind.Version() != version {
			t.Errorf("%s artifact/version = %t/%s, want true/%s", kindName, kind.Artifact, kind.Version(), version)
			continue
		}
		desired := kind.CanonicalDesired()
		if err := validateDesiredForTest(kind, desired); err != nil {
			t.Errorf("%s canonical desired state is invalid: %v", kindName, err)
		}
		for _, hostLocalSelector := range []string{"image", "model"} {
			if _, exists := desired[hostLocalSelector]; exists {
				t.Errorf("%s retains host-local selector %s", kindName, hostLocalSelector)
			}
		}
	}
}

func TestArtifactEntrypointsArePortableRelativeNames(t *testing.T) {
	t.Parallel()

	edge, _ := ByKind("EdgeWorker")
	if edge.CanonicalDesired()["entrypoint"] != "worker.mjs" {
		t.Fatalf("EdgeWorker has no explicit portable entrypoint: %#v", edge.CanonicalDesired())
	}
	for _, invalid := range []string{"/worker.mjs", "../worker.mjs", ".hidden"} {
		desired := cloneValue(edge.CanonicalDesired()).(map[string]any)
		desired["entrypoint"] = invalid
		if err := validateDesiredForTest(edge, desired); err == nil {
			t.Errorf("EdgeWorker accepted non-portable entrypoint %q", invalid)
		}
	}

	site, _ := ByKind("StaticSite")
	invalidSite := cloneValue(site.CanonicalDesired()).(map[string]any)
	invalidSite["indexDocument"] = "../index.html"
	if err := validateDesiredForTest(site, invalidSite); err == nil {
		t.Error("StaticSite accepted an escaping index document")
	}

	workflow, _ := ByKind("Workflow")
	invalidWorkflow := cloneValue(workflow.CanonicalDesired()).(map[string]any)
	invalidWorkflow["entrypoint"] = "not a class"
	if err := validateDesiredForTest(workflow, invalidWorkflow); err == nil {
		t.Error("Workflow accepted an invalid runtime entrypoint")
	}
}

func TestEmailAndIdentityFormsDoNotEncodeContradictoryFieldPairs(t *testing.T) {
	t.Parallel()

	email, _ := ByKind("EmailSender")
	emailDesired := email.CanonicalDesired()
	if emailDesired["defaultLocalPart"] != "notifications" {
		t.Fatalf("EmailSender does not express the sender relative to its domain: %#v", emailDesired)
	}
	if _, exists := emailDesired["defaultSender"]; exists {
		t.Error("EmailSender still permits a mailbox outside its declared domain")
	}

	identity, _ := ByKind("IdentityClient")
	identityDesired := identity.CanonicalDesired()
	if identityDesired["authMethod"] != "none" {
		t.Fatalf("IdentityClient has no single token authentication contract: %#v", identityDesired)
	}
	for _, obsolete := range []string{"grantTypes"} {
		if _, exists := identityDesired[obsolete]; exists {
			t.Errorf("IdentityClient retains ambiguous field %s", obsolete)
		}
	}
}

func TestFeatureFlagHasOneCompleteEvaluationValue(t *testing.T) {
	t.Parallel()

	kind, _ := ByKind("FeatureFlag")
	desired := kind.CanonicalDesired()
	if desired["enabledPercentage"] != 25 {
		t.Fatalf("FeatureFlag has no complete enabled percentage: %#v", desired)
	}
	for _, obsolete := range []string{"enabled", "rolloutPercentage"} {
		if _, exists := desired[obsolete]; exists {
			t.Errorf("FeatureFlag retains ambiguous field %s", obsolete)
		}
	}
}

func TestEveryDeclaredConstraintShipsARejectableNamedFixture(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()
			cases, err := kind.NegativeCases()
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, negative := range cases {
				if negative.Name == "" || names[negative.Name] {
					t.Errorf("invalid or duplicate negative fixture name %q", negative.Name)
				}
				names[negative.Name] = true
				if err := validateDesiredForTest(kind, negative.Desired); err == nil {
					t.Errorf("negative fixture %s is accepted by the desired schema", negative.Name)
				}
			}
			requiredOmissions := []string{"missing-name"}
			if kind.Artifact {
				requiredOmissions = append(requiredOmissions, "missing-source")
			}
			if kind.Connections == ConnectionsRequired {
				requiredOmissions = append(requiredOmissions, "missing-connections")
			}
			for _, field := range kind.Fields {
				if field.Required {
					requiredOmissions = append(requiredOmissions, "missing-"+field.HCL)
				}
			}
			for _, required := range requiredOmissions {
				if !names[required] {
					t.Errorf("required desired key has no omission fixture %s", required)
				}
			}
			// The generated package adds one observed negative to these desired
			// cases. Keep both fixture classes within the normative independent
			// maximum of 32 without collapsing per-field omission coverage.
			if len(cases)+1 > 32 {
				t.Errorf("%d total negative fixtures exceed the independent class maximum of 32", len(cases)+1)
			}
			if kind.Artifact && !names["artifact-source"] {
				t.Error("artifact contract has no negative fixture")
			}
			if kind.Connections == ConnectionsRequired && !names["connections-required"] {
				t.Error("required connection contract has no empty-set fixture")
			}
			if kind.MaxConnections > 0 && !names["connections-cardinality"] {
				t.Error("bounded connection contract has no overflow fixture")
			}
			for _, constraint := range kind.Constraints {
				if !names[constraint.Name] {
					t.Errorf("conditional constraint %s has no negative fixture", constraint.Name)
				}
			}
		})
	}
}

func TestEveryDeclaredFieldHasAPositiveWitness(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds {
		for _, field := range kind.Fields {
			if field.Example == nil && field.Default == "" {
				t.Errorf("%s.%s has no positive example or default", kind.Kind, field.HCL)
			}
		}
	}
}

func validateDesiredForTest(kind Kind, desired map[string]any) error {
	return validateSchemaValueForTest(kind.DesiredSchema(), desired)
}

func validateSchemaValueForTest(schema map[string]any, value map[string]any) error {
	schemaRaw, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var schemaValue any
	if err := json.Unmarshal(schemaRaw, &schemaValue); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	const schemaID = "https://forms.takoform.invalid/desired.schema.json"
	if err := compiler.AddResource(schemaID, schemaValue); err != nil {
		return err
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		return err
	}
	valueRaw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoded, err := jsonschema.UnmarshalJSON(bytes.NewReader(valueRaw))
	if err != nil {
		return err
	}
	return compiled.Validate(decoded)
}

func cloneWithPatch(original map[string]any, patch map[string]any) map[string]any {
	cloned := cloneValue(original).(map[string]any)
	for key, value := range patch {
		cloned[key] = value
	}
	return cloned
}
