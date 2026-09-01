package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	v3RuntimeInputsTestNonce  = "VGVzdFJ1bnRpbWVOb25jZTEyMzQ1Ng"
	v3RuntimeInputsTestSecret = "runtime-secret-sentinel-do-not-leak"
)

func newV3RuntimeInputsTLSHost(t *testing.T) *v3FakeHost {
	t.Helper()
	host := newV3FakeHost(t)
	host.server.Close()
	host.server = httptest.NewTLSServer(http.HandlerFunc(host.serve))
	t.Cleanup(host.server.Close)
	return host
}

func v3RuntimeProviderData(t *testing.T, host *v3FakeHost, nonce string, bindings map[string]string) *providerData {
	t.Helper()
	data := newV3TestProviderData(t, host)
	data.runtimeInputNonce = nonce
	data.runtimeInputs = cloneV3RuntimeInputStrings(bindings)
	return data
}

func v3RuntimePlan(t *testing.T, resource *v3FormResource, bundle string) (tfsdk.Plan, string, string) {
	t.Helper()
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue(bundle),
		"handlers":                     v3StringSetValue(t, "fetch"),
		"required_sensitive_vars":      v3StringSetValue(t, "API_TOKEN"),
		v3RevisionOwnerAttribute:       types.StringValue(v3TestRevisionOwner),
		v3ApplyIdempotencyKeyAttribute: types.StringNull(),
	}
	plan := v3PlanWith(t, ctx, schemaResponse, configured)
	response := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		Plan:   plan,
		Config: v3ConfigWith(t, ctx, schemaResponse, configured),
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
	return response.Plan, key.ValueString(), name.ValueString()
}

func TestV3RuntimeInputProviderConfigurationIsClosedAndBounded(t *testing.T) {
	valid := map[string]string{"API_TOKEN": v3RuntimeInputsTestSecret}
	for name, test := range map[string]struct {
		nonce    string
		bindings map[string]string
		valid    bool
	}{
		"absent":               {valid: true},
		"plan nonce":           {nonce: v3RuntimeInputsTestNonce, valid: true},
		"apply map":            {nonce: v3RuntimeInputsTestNonce, bindings: valid, valid: true},
		"map without nonce":    {bindings: valid},
		"invalid nonce":        {nonce: strings.Repeat("a", 25), bindings: valid},
		"invalid binding name": {nonce: v3RuntimeInputsTestNonce, bindings: map[string]string{"bad-name": "value"}},
		"empty value":          {nonce: v3RuntimeInputsTestNonce, bindings: map[string]string{"API_TOKEN": ""}},
		"oversized value":      {nonce: v3RuntimeInputsTestNonce, bindings: map[string]string{"API_TOKEN": strings.Repeat("x", v3RuntimeInputMaximumValueBytes+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateV3RuntimeInputProviderConfiguration(test.nonce, test.bindings)
			if test.valid && err != nil {
				t.Fatalf("valid configuration rejected: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("invalid provider input accepted")
			}
			if err != nil && strings.Contains(err.Error(), v3RuntimeInputsTestSecret) {
				t.Fatalf("provider input value leaked through error: %v", err)
			}
		})
	}
}

func TestV3WorkerVersionPlanUsesNonceWithoutRuntimeValues(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	data := v3RuntimeProviderData(t, host, v3RuntimeInputsTestNonce, nil)
	resource := v3TestFormResource(t, "WorkerVersion", data)
	planA, keyA, nameA := v3RuntimePlan(t, resource, "worker-bundle")
	if strings.Contains(planA.Raw.String(), v3RuntimeInputsTestSecret) {
		t.Fatal("runtime value reached Plan")
	}

	data.runtimeInputNonce = "QW5vdGhlclJhbmRvbU5vbmNlMTIzNDU2Nw"
	_, keyB, nameB := v3RuntimePlan(t, resource, "worker-bundle")
	data.runtimeInputNonce = v3RuntimeInputsTestNonce
	_, keySpec, nameSpec := v3RuntimePlan(t, resource, "worker-bundle-next")
	if keyA == keyB || nameA == nameB {
		t.Fatal("nonce rotation did not change the operation identity")
	}
	if keyA == keySpec || nameA == nameSpec {
		t.Fatal("value-free spec drift did not change the operation identity")
	}
}

func TestV3WorkerVersionPlanRejectsRuntimeValuesBeforeMutation(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	data := v3RuntimeProviderData(t, host, v3RuntimeInputsTestNonce, map[string]string{
		"API_TOKEN": v3RuntimeInputsTestSecret,
	})
	resource := v3TestFormResource(t, "WorkerVersion", data)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                  types.StringValue("module-worker"),
		"bundle":                  types.StringValue("worker-bundle"),
		"handlers":                v3StringSetValue(t, "fetch"),
		"required_sensitive_vars": v3StringSetValue(t, "API_TOKEN"),
		v3RevisionOwnerAttribute:  types.StringValue(v3TestRevisionOwner),
	}
	plan := v3PlanWith(t, ctx, schemaResponse, configured)
	response := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
		Plan:   plan,
		Config: v3ConfigWith(t, ctx, schemaResponse, configured),
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("Plan accepted Apply-only runtime values")
	}
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), v3RuntimeInputsTestSecret) {
			t.Fatalf("runtime value leaked through Plan diagnostic: %s", diagnostic.Detail())
		}
	}
	if host.runtimeInputPuts != 0 || len(host.applyHeaders) != 0 {
		t.Fatal("invalid Plan input reached Host mutation")
	}
}

func TestV3WorkerVersionApplyUsesOnlyExactProviderMap(t *testing.T) {
	host := newV3RuntimeInputsTLSHost(t)
	data := v3RuntimeProviderData(t, host, v3RuntimeInputsTestNonce, nil)
	resource := v3TestFormResource(t, "WorkerVersion", data)
	plan, _, _ := v3RuntimePlan(t, resource, "worker-bundle")

	t.Setenv("API_TOKEN", "ambient-env-value-must-not-be-read")
	t.Setenv("TAKOFORM_RUNTIME_INPUTS_FILE", "/tmp/must-not-be-read")
	data.runtimeInputs = map[string]string{"API_TOKEN": v3RuntimeInputsTestSecret}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("apply runtime inputs: %v", created.Diagnostics)
	}
	if host.runtimeInputPuts != 1 || len(host.applyHeaders) != 1 {
		t.Fatalf("private/public PUT count = %d/%d", host.runtimeInputPuts, len(host.applyHeaders))
	}
	if len(host.runtimeInputRaw) != 1 || !strings.Contains(string(host.runtimeInputRaw[0]), v3RuntimeInputsTestSecret) {
		t.Fatal("private preparation did not receive the provider-scoped runtime value")
	}
	if strings.Contains(string(host.runtimeInputRaw[0]), "ambient-env-value-must-not-be-read") {
		t.Fatal("private preparation read a binding from ambient environment")
	}
	for _, publicBody := range host.applyBodies {
		raw, _ := json.Marshal(publicBody)
		if strings.Contains(string(raw), v3RuntimeInputsTestSecret) {
			t.Fatalf("runtime value reached public apply: %s", raw)
		}
	}
	if strings.Contains(created.State.Raw.String(), v3RuntimeInputsTestSecret) {
		t.Fatal("runtime value reached serialized state")
	}
}

func TestV3WorkerVersionApplyRejectsNonceOrBindingDrift(t *testing.T) {
	for name, mutate := range map[string]func(*providerData){
		"nonce drift": func(data *providerData) {
			data.runtimeInputNonce = "QW5vdGhlclJhbmRvbU5vbmNlMTIzNDU2Nw"
			data.runtimeInputs = map[string]string{"API_TOKEN": "rotated-private-value"}
		},
		"missing binding": func(data *providerData) {
			data.runtimeInputs = map[string]string{}
		},
		"extra binding": func(data *providerData) {
			data.runtimeInputs = map[string]string{
				"API_TOKEN": v3RuntimeInputsTestSecret,
				"EXTRA":     "another-private-value",
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			host := newV3RuntimeInputsTLSHost(t)
			data := v3RuntimeProviderData(t, host, v3RuntimeInputsTestNonce, nil)
			resource := v3TestFormResource(t, "WorkerVersion", data)
			plan, _, _ := v3RuntimePlan(t, resource, "worker-bundle")
			mutate(data)
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)
			created := frameworkresource.CreateResponse{
				State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
			}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &created)
			if !created.Diagnostics.HasError() {
				t.Fatal("runtime input drift was accepted")
			}
			for _, diagnostic := range created.Diagnostics.Errors() {
				if strings.Contains(diagnostic.Detail(), v3RuntimeInputsTestSecret) ||
					strings.Contains(diagnostic.Detail(), "rotated-private-value") ||
					strings.Contains(diagnostic.Detail(), "another-private-value") {
					t.Fatalf("runtime value leaked through drift diagnostic: %s", diagnostic.Detail())
				}
			}
			if host.runtimeInputPuts != 0 || len(host.applyHeaders) != 0 {
				t.Fatal("runtime input drift reached Host mutation")
			}
		})
	}
}
