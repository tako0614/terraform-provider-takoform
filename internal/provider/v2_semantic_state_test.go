package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/tako0614/terraform-provider-takoform/internal/edgev2"
	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

func TestWorkerVersionFormEquivalentSetsAndDefaultsHaveEquivalentState(t *testing.T) {
	ctx := context.Background()
	withoutDefaults := v2DecodedSpec(t, edgev2.WorkerVersionURL, `{
		"worker":{"resourceUid":"worker-1"},
		"bundle":{"resourceUid":"bundle-1"},
		"handlers":["queue","fetch"]
	}`)
	withDefaultsAndDifferentOrder := v2DecodedSpec(t, edgev2.WorkerVersionURL, `{
		"worker":{"resourceUid":"worker-1"},
		"bundle":{"resourceUid":"bundle-1"},
		"handlers":["fetch","queue"],
		"vars":{},
		"requiredSensitiveVars":[],
		"kvBindings":[],"sqliteBindings":[],"bucketBindings":[],
		"queueProducerBindings":[],"serviceBindings":[],"actorBindings":[],"workflowBindings":[]
	}`)

	left := v2StateObject(t, edgev2.WorkerVersionURL, withoutDefaults)
	right := v2StateObject(t, edgev2.WorkerVersionURL, withDefaultsAndDifferentOrder)
	if !left.Equal(right) {
		t.Fatalf("Form-equivalent WorkerVersion specs produced unequal typed state:\nleft:  %#v\nright: %#v", left, right)
	}

	for _, name := range []string{"handlers", "required_sensitive_vars", "kv_bindings"} {
		if _, ok := right.Attributes()[name].(types.Set); !ok {
			t.Errorf("spec.%s state has type %T; want an order-insensitive set", name, right.Attributes()[name])
		}
	}

	leftPlan, leftDef := v2PlanForForm(t, edgev2.WorkerVersionURL, left)
	rightPlan, rightDef := v2PlanForForm(t, edgev2.WorkerVersionURL, right)
	leftWire, err := v2SpecFromPlan(ctx, leftPlan, leftDef)
	if err != nil {
		t.Fatal(err)
	}
	rightWire, err := v2SpecFromPlan(ctx, rightPlan, rightDef)
	if err != nil {
		t.Fatal(err)
	}
	leftJSON, _ := jsonv2.Marshal(leftWire)
	rightJSON, _ := jsonv2.Marshal(rightWire)
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("Form-equivalent set plans encoded differently:\nleft:  %s\nright: %s", leftJSON, rightJSON)
	}
}

func TestWorkerDeploymentVersionsAreAnOrderInsensitiveSet(t *testing.T) {
	ctx := context.Background()
	first := v2DecodedSpec(t, edgev2.WorkerDeploymentURL, `{
		"worker":{"resourceUid":"worker-1"},
		"versions":[
			{"workerVersion":{"resourceUid":"version-a"},"weight":4000},
			{"workerVersion":{"resourceUid":"version-b"},"weight":6000}
		]
	}`)
	second := v2DecodedSpec(t, edgev2.WorkerDeploymentURL, `{
		"worker":{"resourceUid":"worker-1"},
		"versions":[
			{"workerVersion":{"resourceUid":"version-b"},"weight":6000},
			{"workerVersion":{"resourceUid":"version-a"},"weight":4000}
		]
	}`)

	left := v2StateObject(t, edgev2.WorkerDeploymentURL, first)
	right := v2StateObject(t, edgev2.WorkerDeploymentURL, second)
	if !left.Equal(right) {
		t.Fatalf("reordered WorkerDeployment versions produced unequal state:\nleft:  %#v\nright: %#v", left, right)
	}
	if _, ok := left.Attributes()["versions"].(types.Set); !ok {
		t.Fatalf("versions state has type %T; want an order-insensitive set", left.Attributes()["versions"])
	}
	_ = ctx
}

func TestWorkerVersionVarsJSONUsesFormSemanticEquality(t *testing.T) {
	ctx := context.Background()
	first := v2DecodedSpec(t, edgev2.WorkerVersionURL, `{
		"worker":{"resourceUid":"worker-1"},"bundle":{"resourceUid":"bundle-1"},"handlers":[],
		"vars":{"A":1.0,"B":1e2,"nested":{"x":true,"y":[1,2]}}
	}`)
	differentArrayOrder := v2DecodedSpec(t, edgev2.WorkerVersionURL, `{
		"worker":{"resourceUid":"worker-1"},"bundle":{"resourceUid":"bundle-1"},"handlers":[],
		"vars":{"nested":{"x":true,"y":[2,1]},"B":100,"A":1}
	}`)

	configuredValue, valueDiags := (v2JSONSemanticStringType{}).ValueFromString(ctx, types.StringValue(`{"nested":{"y":[1,2],"x":true},"A":1,"B":100}`))
	if valueDiags.HasError() {
		t.Fatal(valueDiags)
	}
	equivalentValue := v2StateObject(t, edgev2.WorkerVersionURL, first).Attributes()["vars_json"]
	differentValue := v2StateObject(t, edgev2.WorkerVersionURL, differentArrayOrder).Attributes()["vars_json"]
	semanticValue, ok := configuredValue.(basetypes.StringValuableWithSemanticEquals)
	if !ok {
		t.Fatalf("vars_json value %T does not implement Framework semantic equality", configuredValue)
	}
	equivalent, diags := semanticValue.StringSemanticEquals(ctx, equivalentValue.(basetypes.StringValuable))
	if diags.HasError() || !equivalent {
		t.Fatalf("mathematically equivalent JSON values were not equal: equivalent=%v diagnostics=%v", equivalent, diags)
	}
	unequal, diags := semanticValue.StringSemanticEquals(ctx, differentValue.(basetypes.StringValuable))
	if diags.HasError() || unequal {
		t.Fatalf("array order was not preserved: equivalent=%v diagnostics=%v", unequal, diags)
	}
}

func TestFormDefaultedQueueDelayHasOneStateRepresentation(t *testing.T) {
	omitted := v2DecodedSpec(t, edgev2.AtLeastOnceQueueURL, `{"messageRetentionSeconds":60}`)
	explicit := v2DecodedSpec(t, edgev2.AtLeastOnceQueueURL, `{"deliveryDelaySeconds":0,"messageRetentionSeconds":60}`)
	left := v2StateObject(t, edgev2.AtLeastOnceQueueURL, omitted)
	right := v2StateObject(t, edgev2.AtLeastOnceQueueURL, explicit)
	if !left.Equal(right) {
		t.Fatalf("omitted and explicit Form-defaulted delivery delay differ in typed state: %#v vs %#v", left, right)
	}
}

func v2DecodedSpec(t *testing.T, form string, raw string) map[string]any {
	t.Helper()
	if _, err := edgev2.Decode(form, []byte(raw)); err != nil {
		t.Fatalf("decode exact Form fixture: %v", err)
	}
	value, err := jsonv2.Decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value.(map[string]any)
}

func v2StateObject(t *testing.T, form string, spec map[string]any) types.Object {
	t.Helper()
	value, err := v2StateSpec(context.Background(), v2DefForForm(t, form), spec)
	if err != nil {
		t.Fatalf("project exact spec into state: %v", err)
	}
	object, ok := value.(types.Object)
	if !ok {
		t.Fatalf("state spec has type %T; want an object", value)
	}
	return object
}

func v2DefForForm(t *testing.T, form string) v2FormDef {
	t.Helper()
	for _, def := range v2Forms {
		if def.form == form {
			return def
		}
	}
	t.Fatalf("missing Form definition %q", form)
	return v2FormDef{}
}

func v2PlanForForm(t *testing.T, form string, spec attr.Value) (tfsdk.Plan, v2FormDef) {
	t.Helper()
	def := v2DefForForm(t, form)
	_, schemaResponse := v2TestResource(t, nil, form)
	plan := v2TestPlan(t, schemaResponse, "app")
	if diags := plan.SetAttribute(context.Background(), path.Root("spec"), spec); diags.HasError() {
		t.Fatalf("set typed spec plan: %v", diags)
	}
	return plan, def
}
