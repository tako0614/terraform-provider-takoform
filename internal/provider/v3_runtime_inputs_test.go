package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	v3RuntimeInputsTestOrigin = "https://host.example"
	v3RuntimeInputsTestNonce  = "VGVzdFJ1bnRpbWVOb25jZTEyMzQ1Ng"
	v3RuntimeInputsTestSecret = "runtime-secret-sentinel-do-not-leak"
)

func writeV3RuntimeInputsTestFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worker-runtime-inputs.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func unsetV3RuntimeInputsTestEnvironment(t *testing.T) {
	t.Helper()
	prior, existed := os.LookupEnv(v3RuntimeInputsEnvironmentVariable)
	if err := os.Unsetenv(v3RuntimeInputsEnvironmentVariable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(v3RuntimeInputsEnvironmentVariable, prior)
		} else {
			_ = os.Unsetenv(v3RuntimeInputsEnvironmentVariable)
		}
	})
}

func TestLoadV3RuntimeInputsRequiresExactDeclaredBindings(t *testing.T) {
	valid := `{"format":"takoform.worker-runtime-inputs@v1","materialGenerationNonce":"` +
		v3RuntimeInputsTestNonce + `","canonicalPublicOrigin":"` + v3RuntimeInputsTestOrigin +
		`","bindings":{"API_TOKEN":"` + v3RuntimeInputsTestSecret + `","OIDC_SECRET":"second-secret"}}`

	material, err := loadV3RuntimeInputs(
		writeV3RuntimeInputsTestFile(t, valid),
		v3RuntimeInputsTestOrigin,
		[]string{"OIDC_SECRET", "API_TOKEN"},
	)
	if err != nil {
		t.Fatalf("load exact runtime inputs: %v", err)
	}
	defer material.release()
	if material.MaterialGenerationNonce != v3RuntimeInputsTestNonce {
		t.Fatalf("nonce = %q", material.MaterialGenerationNonce)
	}
	if got := material.Bindings["API_TOKEN"]; string(got) != v3RuntimeInputsTestSecret {
		t.Fatalf("API_TOKEN was not loaded exactly")
	}

	for name, declared := range map[string][]string{
		"missing": {"API_TOKEN", "OIDC_SECRET", "MISSING"},
		"extra":   {"API_TOKEN"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadV3RuntimeInputs(
				writeV3RuntimeInputsTestFile(t, valid),
				v3RuntimeInputsTestOrigin,
				declared,
			)
			if err == nil {
				t.Fatal("non-exact binding set was accepted")
			}
			if strings.Contains(err.Error(), v3RuntimeInputsTestSecret) || strings.Contains(err.Error(), "second-secret") {
				t.Fatalf("runtime input value leaked through error: %v", err)
			}
		})
	}
}

func TestV3RuntimeInputsFailClosedBeforeHostMutation(t *testing.T) {
	tests := map[string]struct {
		declared    []string
		explicitKey bool
		file        func(*testing.T, string) (string, bool)
	}{
		"sensitive declarations without file": {
			declared: []string{"API_TOKEN"},
			file: func(t *testing.T, _ string) (string, bool) {
				return "", false
			},
		},
		"empty file environment with no declaration": {
			file: func(t *testing.T, _ string) (string, bool) {
				return "", true
			},
		},
		"file without declaration": {
			file: func(t *testing.T, origin string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(origin, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret)), true
			},
		},
		"file with caller-configured operation key": {
			declared:    []string{"API_TOKEN"},
			explicitKey: true,
			file: func(t *testing.T, origin string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(origin, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret)), true
			},
		},
		"missing binding": {
			declared: []string{"API_TOKEN", "OIDC_SECRET"},
			file: func(t *testing.T, origin string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(origin, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret)), true
			},
		},
		"extra binding": {
			declared: []string{"OIDC_SECRET"},
			file: func(t *testing.T, origin string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(origin, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret)), true
			},
		},
		"malformed document": {
			declared: []string{"API_TOKEN"},
			file: func(t *testing.T, _ string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, `{"format":`+v3RuntimeInputsTestSecret), true
			},
		},
		"closed document has extra key": {
			declared: []string{"API_TOKEN"},
			file: func(t *testing.T, origin string) (string, bool) {
				body := strings.TrimSuffix(v3RuntimeInputsDocument(origin, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret), "}") + `,"workerUid":"forbidden"}`
				return writeV3RuntimeInputsTestFile(t, body), true
			},
		},
		"bad origin": {
			declared: []string{"API_TOKEN"},
			file: func(t *testing.T, _ string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument("https://different.example", v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret)), true
			},
		},
		"non-base64url nonce": {
			declared: []string{"API_TOKEN"},
			file: func(t *testing.T, origin string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(origin, strings.Repeat("a", 25), v3RuntimeInputsTestSecret)), true
			},
		},
		"oversized value": {
			declared: []string{"API_TOKEN"},
			file: func(t *testing.T, origin string) (string, bool) {
				return writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(origin, v3RuntimeInputsTestNonce, strings.Repeat("s", v3RuntimeInputMaximumValueBytes+1))), true
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			host := newV3RuntimeInputsTLSHost(t)
			resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)
			configured := map[string]attr.Value{
				"worker":                 types.StringValue("module-worker"),
				"bundle":                 types.StringValue("worker-bundle"),
				"handlers":               v3StringSetValue(t, "fetch"),
				v3RevisionOwnerAttribute: types.StringValue(v3TestRevisionOwner),
			}
			if len(test.declared) > 0 {
				configured["required_sensitive_vars"] = v3StringSetValue(t, test.declared...)
			}
			if test.explicitKey {
				configured[v3ApplyIdempotencyKeyAttribute] = types.StringValue(testWorkerVersionApplyKey)
			}
			filePath, set := test.file(t, host.server.URL)
			if set {
				t.Setenv(v3RuntimeInputsEnvironmentVariable, filePath)
			} else {
				unsetV3RuntimeInputsTestEnvironment(t)
			}
			plan := v3PlanWith(t, ctx, schemaResponse, configured)
			modify := frameworkresource.ModifyPlanResponse{Plan: plan}
			resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
				State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
				Plan:   plan,
				Config: v3ConfigWith(t, ctx, schemaResponse, configured),
			}, &modify)
			if !modify.Diagnostics.HasError() {
				t.Fatal("invalid runtime input boundary was accepted")
			}
			for _, diagnostic := range modify.Diagnostics.Errors() {
				if strings.Contains(diagnostic.Detail(), v3RuntimeInputsTestSecret) ||
					strings.Contains(diagnostic.Detail(), strings.Repeat("s", 64)) {
					t.Fatalf("runtime input value leaked through diagnostic: %s", diagnostic.Detail())
				}
			}
			if host.runtimeInputPuts != 0 || len(host.applyHeaders) != 0 {
				t.Fatalf("invalid runtime inputs reached mutation: private/public=%d/%d", host.runtimeInputPuts, len(host.applyHeaders))
			}
		})
	}
}

func newV3RuntimeInputsTLSHost(t *testing.T) *v3FakeHost {
	t.Helper()
	host := newV3FakeHost(t)
	host.server.Close()
	host.server = httptest.NewTLSServer(http.HandlerFunc(host.serve))
	t.Cleanup(host.server.Close)
	return host
}

func v3RuntimeInputsDocument(origin, nonce, secret string) string {
	return `{"format":"takoform.worker-runtime-inputs@v1","materialGenerationNonce":"` + nonce +
		`","canonicalPublicOrigin":"` + origin + `","bindings":{"API_TOKEN":"` + secret + `"}}`
}

func TestV3WorkerVersionRuntimeInputsPlanComputesStableOperationKey(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	attribute := schemaResponse.Schema.Attributes[v3ApplyIdempotencyKeyAttribute]
	if !attribute.IsOptional() || !attribute.IsComputed() || attribute.IsRequired() {
		t.Fatalf("apply_idempotency_key flags = optional=%t computed=%t required=%t", attribute.IsOptional(), attribute.IsComputed(), attribute.IsRequired())
	}

	configured := map[string]attr.Value{
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     v3StringSetValue(t, "fetch"),
		"required_sensitive_vars":      v3StringSetValue(t, "API_TOKEN"),
		v3RevisionOwnerAttribute:       types.StringValue(v3TestRevisionOwner),
		v3ApplyIdempotencyKeyAttribute: types.StringNull(),
	}
	planWith := func(t *testing.T, nonce, secret, bundle string) (string, string) {
		t.Helper()
		planValues := make(map[string]attr.Value, len(configured))
		for name, value := range configured {
			planValues[name] = value
		}
		planValues["bundle"] = types.StringValue(bundle)
		pathToInputs := writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(host.server.URL, nonce, secret))
		t.Setenv(v3RuntimeInputsEnvironmentVariable, pathToInputs)
		plan := v3PlanWith(t, ctx, schemaResponse, planValues)
		response := frameworkresource.ModifyPlanResponse{Plan: plan}
		resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
			State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			Plan:   plan,
			Config: v3ConfigWith(t, ctx, schemaResponse, planValues),
		}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("runtime input plan: %v", response.Diagnostics)
		}
		var key, name types.String
		if diags := response.Plan.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &key); diags.HasError() {
			t.Fatalf("read operation key: %v", diags)
		}
		if diags := response.Plan.GetAttribute(ctx, path.Root("name"), &name); diags.HasError() {
			t.Fatalf("read derived name: %v", diags)
		}
		if key.IsNull() || key.IsUnknown() || name.IsNull() || name.IsUnknown() {
			t.Fatalf("runtime plan did not settle key/name: key=%#v name=%#v", key, name)
		}
		return key.ValueString(), name.ValueString()
	}

	keyA, nameA := planWith(t, v3RuntimeInputsTestNonce, "secret-value-a", "worker-bundle")
	keySameNonce, nameSameNonce := planWith(t, v3RuntimeInputsTestNonce, "rotated-secret-value-b", "worker-bundle")
	keyB, nameB := planWith(t, "QW5vdGhlclJhbmRvbU5vbmNlMTIzNDU2Nw", "secret-value-a", "worker-bundle")
	keySpecDrift, nameSpecDrift := planWith(t, v3RuntimeInputsTestNonce, "secret-value-a", "worker-bundle-next")
	if keyA != keySameNonce || nameA != nameSameNonce {
		t.Fatalf("secret-only rotation changed logical identity: key %q/%q name %q/%q", keyA, keySameNonce, nameA, nameSameNonce)
	}
	if keyA == keyB || nameA == nameB {
		t.Fatalf("nonce rotation did not change operation key and immutable identity: key %q/%q name %q/%q", keyA, keyB, nameA, nameB)
	}
	if keyA == keySpecDrift || nameA == nameSpecDrift {
		t.Fatalf("value-free spec drift did not change operation key and immutable identity: key %q/%q name %q/%q", keyA, keySpecDrift, nameA, nameSpecDrift)
	}
}

func TestV3WorkerVersionOrdinaryReplacementKeepsOmittedOperationKeyNull(t *testing.T) {
	unsetV3RuntimeInputsTestEnvironment(t)
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                 types.StringValue("module-worker"),
		"bundle":                 types.StringValue("worker-bundle-next"),
		"handlers":               v3StringSetValue(t, "fetch"),
		v3RevisionOwnerAttribute: types.StringValue(v3TestRevisionOwner),
	}
	prior := v3PlanWith(t, ctx, schemaResponse, configured)
	plan := v3PlanWith(t, ctx, schemaResponse, configured)
	if diags := plan.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringUnknown()); diags.HasError() {
		t.Fatalf("set framework-proposed unknown operation key: %v", diags)
	}
	response := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.v3PlanRuntimeInputs(ctx, frameworkresource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: prior.Raw},
		Plan:   plan,
		Config: v3ConfigWith(t, ctx, schemaResponse, configured),
	}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("ordinary no-file operation-key planning: %v", response.Diagnostics)
	}
	var key types.String
	if diags := response.Plan.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &key); diags.HasError() {
		t.Fatalf("read ordinary operation key: %v", diags)
	}
	if !key.IsNull() {
		t.Fatalf("ordinary omitted operation key = %#v, want historical null behavior", key)
	}
	if host.runtimeInputPuts != 0 {
		t.Fatalf("ordinary operation-key planning made %d private calls", host.runtimeInputPuts)
	}
}

func TestV3WorkerVersionRuntimeInputNonceDriftFailsBeforeMutation(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                  types.StringValue("module-worker"),
		"bundle":                  types.StringValue("worker-bundle"),
		"handlers":                v3StringSetValue(t, "fetch"),
		"required_sensitive_vars": v3StringSetValue(t, "API_TOKEN"),
		v3RevisionOwnerAttribute:  types.StringValue(v3TestRevisionOwner),
	}
	pathToInputs := writeV3RuntimeInputsTestFile(
		t,
		v3RuntimeInputsDocument(host.server.URL, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret),
	)
	t.Setenv(v3RuntimeInputsEnvironmentVariable, pathToInputs)
	plan := v3PlanWith(t, ctx, schemaResponse, configured)
	modify := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		Plan:   plan,
		Config: v3ConfigWith(t, ctx, schemaResponse, configured),
	}, &modify)
	if modify.Diagnostics.HasError() {
		t.Fatalf("plan runtime inputs: %v", modify.Diagnostics)
	}
	if err := os.WriteFile(
		pathToInputs,
		[]byte(v3RuntimeInputsDocument(host.server.URL, "QW5vdGhlclJhbmRvbU5vbmNlMTIzNDU2Nw", "rotated-value")),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: modify.Plan}, &created)
	if !created.Diagnostics.HasError() {
		t.Fatal("plan/apply nonce drift was accepted")
	}
	for _, diagnostic := range created.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), v3RuntimeInputsTestSecret) || strings.Contains(diagnostic.Detail(), "rotated-value") {
			t.Fatalf("runtime input value leaked into drift diagnostic: %s", diagnostic.Detail())
		}
	}
	if host.runtimeInputPuts != 0 || len(host.applyHeaders) != 0 {
		t.Fatalf("nonce drift reached private/public mutation: private PUT=%d public PUT=%d", host.runtimeInputPuts, len(host.applyHeaders))
	}
}

func TestV3WorkerVersionRuntimeInputFileDriftFailsBeforeMutation(t *testing.T) {
	tests := map[string]func(*testing.T, string, string){
		"missing binding": func(t *testing.T, path, origin string) {
			body := `{"format":"` + v3RuntimeInputsFileFormat + `","materialGenerationNonce":"` +
				v3RuntimeInputsTestNonce + `","canonicalPublicOrigin":"` + origin + `","bindings":{}}`
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"extra binding": func(t *testing.T, path, origin string) {
			body := `{"format":"` + v3RuntimeInputsFileFormat + `","materialGenerationNonce":"` +
				v3RuntimeInputsTestNonce + `","canonicalPublicOrigin":"` + origin +
				`","bindings":{"API_TOKEN":"` + v3RuntimeInputsTestSecret + `","EXTRA":"another-private-value"}}`
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"malformed": func(t *testing.T, path, _ string) {
			if err := os.WriteFile(path, []byte(`{"format":`+v3RuntimeInputsTestSecret), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"bad origin": func(t *testing.T, path, _ string) {
			if err := os.WriteFile(path, []byte(v3RuntimeInputsDocument(
				"https://different.example", v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret,
			)), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"wrong mode": func(t *testing.T, path, _ string) {
			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatal(err)
			}
		},
		"symlink replacement": func(t *testing.T, path, origin string) {
			target := writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(
				origin, v3RuntimeInputsTestNonce, "replacement-private-value",
			))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
		"oversized": func(t *testing.T, path, _ string) {
			if err := os.WriteFile(path, []byte(strings.Repeat("x", v3RuntimeInputsMaximumFileBytes+1)), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			host := newV3RuntimeInputsTLSHost(t)
			resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)
			configured := map[string]attr.Value{
				"worker":                  types.StringValue("module-worker"),
				"bundle":                  types.StringValue("worker-bundle"),
				"handlers":                v3StringSetValue(t, "fetch"),
				"required_sensitive_vars": v3StringSetValue(t, "API_TOKEN"),
				v3RevisionOwnerAttribute:  types.StringValue(v3TestRevisionOwner),
			}
			pathToInputs := writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(
				host.server.URL, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret,
			))
			t.Setenv(v3RuntimeInputsEnvironmentVariable, pathToInputs)
			plan := v3PlanWith(t, ctx, schemaResponse, configured)
			modify := frameworkresource.ModifyPlanResponse{Plan: plan}
			resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
				State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
				Plan:   plan,
				Config: v3ConfigWith(t, ctx, schemaResponse, configured),
			}, &modify)
			if modify.Diagnostics.HasError() {
				t.Fatalf("plan valid runtime inputs: %v", modify.Diagnostics)
			}

			mutate(t, pathToInputs, host.server.URL)
			created := frameworkresource.CreateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: modify.Plan}, &created)
			if !created.Diagnostics.HasError() {
				t.Fatal("runtime input file drift was accepted")
			}
			for _, diagnostic := range created.Diagnostics.Errors() {
				if strings.Contains(diagnostic.Detail(), v3RuntimeInputsTestSecret) ||
					strings.Contains(diagnostic.Detail(), "another-private-value") ||
					strings.Contains(diagnostic.Detail(), "replacement-private-value") {
					t.Fatalf("runtime input value leaked through apply diagnostic: %s", diagnostic.Detail())
				}
			}
			if host.runtimeInputPuts != 0 || len(host.applyHeaders) != 0 {
				t.Fatalf("runtime input drift reached mutation: private/public=%d/%d", host.runtimeInputPuts, len(host.applyHeaders))
			}
		})
	}
}

func TestV3WorkerVersionRuntimeInputsReachOnlyPrivatePreparation(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                  types.StringValue("module-worker"),
		"bundle":                  types.StringValue("worker-bundle"),
		"handlers":                v3StringSetValue(t, "fetch"),
		"required_sensitive_vars": v3StringSetValue(t, "API_TOKEN"),
		v3RevisionOwnerAttribute:  types.StringValue(v3TestRevisionOwner),
	}
	pathToInputs := writeV3RuntimeInputsTestFile(
		t,
		v3RuntimeInputsDocument(host.server.URL, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret),
	)
	t.Setenv(v3RuntimeInputsEnvironmentVariable, pathToInputs)
	t.Setenv("API_TOKEN", "ambient-env-value-must-not-be-read")
	plan := v3PlanWith(t, ctx, schemaResponse, configured)
	modify := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		Plan:   plan,
		Config: v3ConfigWith(t, ctx, schemaResponse, configured),
	}, &modify)
	if modify.Diagnostics.HasError() {
		t.Fatalf("plan runtime inputs: %v", modify.Diagnostics)
	}
	if strings.Contains(modify.Plan.Raw.String(), v3RuntimeInputsTestSecret) {
		t.Fatal("runtime input value reached the plan")
	}

	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: modify.Plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("apply runtime inputs: %v", created.Diagnostics)
	}
	if host.runtimeInputPuts != 1 || len(host.applyHeaders) != 1 {
		t.Fatalf("private/public PUT count = %d/%d", host.runtimeInputPuts, len(host.applyHeaders))
	}
	if len(host.runtimeInputRaw) != 1 || !strings.Contains(string(host.runtimeInputRaw[0]), v3RuntimeInputsTestSecret) {
		t.Fatal("private preparation did not receive the runtime value")
	}
	if strings.Contains(string(host.runtimeInputRaw[0]), "ambient-env-value-must-not-be-read") {
		t.Fatal("private preparation read a required_sensitive_vars name from ambient environment")
	}
	for _, publicBody := range host.applyBodies {
		raw, _ := json.Marshal(publicBody)
		if strings.Contains(string(raw), v3RuntimeInputsTestSecret) {
			t.Fatalf("runtime input value reached public apply: %s", raw)
		}
	}
	if strings.Contains(created.State.Raw.String(), v3RuntimeInputsTestSecret) {
		t.Fatal("runtime input value reached serialized state")
	}
	var offenders []string
	if err := tftypes.Walk(created.State.Raw, func(attributePath *tftypes.AttributePath, value tftypes.Value) (bool, error) {
		if !value.IsKnown() || value.IsNull() || !value.Type().Is(tftypes.String) {
			return true, nil
		}
		var text string
		if err := value.As(&text); err != nil {
			return true, nil
		}
		if strings.Contains(text, v3RuntimeInputsTestSecret) {
			offenders = append(offenders, attributePath.String())
		}
		return true, nil
	}); err != nil {
		t.Fatalf("walk runtime-input state: %v", err)
	}
	if len(offenders) != 0 {
		t.Fatalf("runtime input value reached state at %v", offenders)
	}
}

func TestV3WorkerVersionRuntimeInputHostReflectionNeverReachesDiagnosticsOrState(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	host.runtimeInputPutReflection = v3RuntimeInputsTestSecret
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                  types.StringValue("module-worker"),
		"bundle":                  types.StringValue("worker-bundle"),
		"handlers":                v3StringSetValue(t, "fetch"),
		"required_sensitive_vars": v3StringSetValue(t, "API_TOKEN"),
		v3RevisionOwnerAttribute:  types.StringValue(v3TestRevisionOwner),
	}
	t.Setenv(
		v3RuntimeInputsEnvironmentVariable,
		writeV3RuntimeInputsTestFile(
			t, v3RuntimeInputsDocument(host.server.URL, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret),
		),
	)
	plan := v3PlanWith(t, ctx, schemaResponse, configured)
	modified := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		Plan:  plan, Config: v3ConfigWith(t, ctx, schemaResponse, configured),
	}, &modified)
	if modified.Diagnostics.HasError() {
		t.Fatalf("plan reflected-error runtime inputs: %v", modified.Diagnostics)
	}

	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: modified.Plan}, &created)
	if !created.Diagnostics.HasError() {
		t.Fatal("Host-reflected private rejection produced no provider diagnostic")
	}
	sentinels := []string{
		v3RuntimeInputsTestSecret,
		"reflected-message", "reflected-request-id", "reflected-host-code", "reflected-details",
	}
	for _, diagnostic := range created.Diagnostics.Errors() {
		serialized := diagnostic.Summary() + "\n" + diagnostic.Detail()
		for _, sentinel := range sentinels {
			if strings.Contains(serialized, sentinel) {
				t.Fatalf("provider diagnostic exposed Host-reflected data %q: %s", sentinel, serialized)
			}
		}
	}
	for _, sentinel := range sentinels {
		if strings.Contains(created.State.Raw.String(), sentinel) {
			t.Fatalf("provider state exposed Host-reflected data %q", sentinel)
		}
	}
	if host.runtimeInputGets != 2 || host.runtimeInputPuts != 1 || len(host.applyHeaders) != 0 {
		t.Fatalf(
			"Host reflection private GET/PUT/public PUT = %d/%d/%d, want 2/1/0",
			host.runtimeInputGets, host.runtimeInputPuts, len(host.applyHeaders),
		)
	}
}
