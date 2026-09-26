package provider

// v3_host_support_test.go covers the plan-time capability decision at the unit
// level. The end-to-end proof — a real CLI planning against a host that
// implements WorkerVersion and edge.kv while implementing none of bucket,
// SQLite, or queue — lives in cmd/worker-authoring-conformance; what is here is
// the reading of a profile and the shape of the refusals.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

func v3TestProfile(t *testing.T, body string) v3SupportProfile {
	t.Helper()
	var profile map[string]any
	if err := json.Unmarshal([]byte(body), &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	return v3SupportProfile(profile)
}

// TestV3SupportRefusalDistinguishesNoFromCouldNotAsk is the rule the whole
// design rests on: a host that says no is refused, and a host the plan could
// not ask is not.
func TestV3SupportRefusalDistinguishesNoFromCouldNotAsk(t *testing.T) {
	t.Parallel()
	refusals := []*clientv3.APIError{
		{StatusCode: http.StatusNotFound, Code: "form_unknown"},
		{StatusCode: http.StatusNotFound, Code: "resource_not_found"},
	}
	for _, refusal := range refusals {
		if !v3SupportRefusal(refusal) {
			t.Errorf("%s was not read as a refusal", refusal.Code)
		}
	}
	undecided := []error{
		errors.New("dial tcp: connection refused"),
		&clientv3.APIError{StatusCode: http.StatusUnauthorized, Code: "unauthenticated"},
		&clientv3.APIError{StatusCode: http.StatusServiceUnavailable, Code: "backend_unavailable"},
		&clientv3.APIError{StatusCode: http.StatusNotFound, ProtocolInvalid: true},
	}
	for _, err := range undecided {
		if v3SupportRefusal(err) {
			t.Errorf("%v was read as a refusal, which would refuse an apply for failing to ask", err)
		}
	}
}

// TestV3SupportProfileReadsOnlyWhatTheHostPublished proves omission is never
// read as denial: the published profile schema makes every capability member
// optional, so a profile carrying identity and operations alone must decide
// nothing beyond them.
func TestV3SupportProfileReadsOnlyWhatTheHostPublished(t *testing.T) {
	t.Parallel()
	minimal := v3TestProfile(t, `{
		"apiVersion": "support.takoform.com/v1alpha1",
		"kind": "FormSupport",
		"operations": ["create", "read", "update", "delete"]
	}`)
	if values, advertised := minimal.enum("handlers"); advertised {
		t.Errorf("an omitted supportedEnums produced %v", values)
	}
	if _, hasMinimum, _, hasMaximum := minimal.rangeFor("maxRetries"); hasMinimum || hasMaximum {
		t.Error("an omitted supportedRanges produced a bound")
	}
	if value, published := minimal.limit("maximumBundleBytes"); published {
		t.Errorf("an omitted limits produced %d", value)
	}
	if got := minimal.stringSlice("operations"); len(got) != 4 {
		t.Errorf("operations = %v, want the four the host published", got)
	}

	full := v3TestProfile(t, `{
		"apiVersion": "support.takoform.com/v1alpha1",
		"kind": "FormSupport",
		"operations": ["create", "read", "delete"],
		"supportedEnums": {"handlers": ["fetch", "scheduled"]},
		"supportedRanges": {"maxRetries": {"minimum": 0, "maximum": 5}},
		"limits": {"maximumBundleBytes": 10485760, "maximumVersions": 2}
	}`)
	handlers, advertised := full.enum("handlers")
	if !advertised || len(handlers) != 2 || handlers[0] != "fetch" {
		t.Errorf("handlers enum = %v (%v)", handlers, advertised)
	}
	minimum, hasMinimum, maximum, hasMaximum := full.rangeFor("maxRetries")
	if !hasMinimum || !hasMaximum || minimum != 0 || maximum != 5 {
		t.Errorf("maxRetries range = %d..%d (%v/%v)", minimum, maximum, hasMinimum, hasMaximum)
	}
	if ceiling, published := full.limit("maximumBundleBytes"); !published || ceiling != 10485760 {
		t.Errorf("maximumBundleBytes = %d (%v)", ceiling, published)
	}
	if ceiling, published := full.limit("maximumVersions"); !published || ceiling != 2 {
		t.Errorf("maximumVersions = %d (%v)", ceiling, published)
	}
}

// TestV3MaximumLimitNameFollowsTheProfileGrammar pins the convention by which a
// collection field finds its published ceiling. It is a convention rather than
// a list precisely so a host that publishes a ceiling for a field the provider
// never anticipated is honoured without a client change.
func TestV3MaximumLimitNameFollowsTheProfileGrammar(t *testing.T) {
	t.Parallel()
	for wire, want := range map[string]string{
		"handlers":              "maximumHandlers",
		"versions":              "maximumVersions",
		"requiredSensitiveVars": "maximumRequiredSensitiveVars",
		"kvBindings":            "maximumKvBindings",
		"":                      "",
	} {
		if got := v3MaximumLimitName(wire); got != want {
			t.Errorf("v3MaximumLimitName(%q) = %q, want %q", wire, got, want)
		}
	}
}

// TestV3CollectionLimitNameTracksTheProfileVersion pins the one compatibility
// seam between retained beta profiles and stable v1. Beta keeps the historical
// maximum<Field> key; stable reads the desired document's JSON Pointer. A
// legacy-looking extra key in a stable profile must not shadow the pointer the
// host actually published, and an omitted pointer remains an undecided limit.
func TestV3CollectionLimitNameTracksTheProfileVersion(t *testing.T) {
	t.Parallel()
	stable := v3SupportProfile{
		"apiVersion": clientv3.SupportProfileAPIVersion,
		"limits": map[string]any{
			"/requiredSensitiveVars":        int64(0),
			"/maximumRequiredSensitiveVars": int64(99),
		},
	}
	if got := stable.collectionLimitName("requiredSensitiveVars"); got != "/requiredSensitiveVars" {
		t.Fatalf("stable collection limit key = %q, want /requiredSensitiveVars", got)
	}
	if ceiling, published := stable.limit(stable.collectionLimitName("requiredSensitiveVars")); !published || ceiling != 0 {
		t.Fatalf("stable requiredSensitiveVars ceiling = %d (%v), want 0 (true)", ceiling, published)
	}

	// The extra legacy-shaped pointer is not the stable capability key. It must
	// not be consulted as a fallback when the stable pointer is omitted.
	omitted := v3SupportProfile{
		"apiVersion": clientv3.SupportProfileAPIVersion,
		"limits":     map[string]any{"/maximumRequiredSensitiveVars": int64(0)},
	}
	if _, published := omitted.limit(omitted.collectionLimitName("requiredSensitiveVars")); published {
		t.Fatal("stable profile's extra legacy pointer was read as a requiredSensitiveVars ceiling")
	}

	beta := v3SupportProfile{
		"apiVersion": "support.takoform.com/v1alpha1",
		"limits":     map[string]any{"maximumRequiredSensitiveVars": int64(0)},
	}
	if got := beta.collectionLimitName("requiredSensitiveVars"); got != "maximumRequiredSensitiveVars" {
		t.Fatalf("retained beta collection limit key = %q, want maximumRequiredSensitiveVars", got)
	}
	if ceiling, published := beta.limit(beta.collectionLimitName("requiredSensitiveVars")); !published || ceiling != 0 {
		t.Fatalf("retained beta requiredSensitiveVars ceiling = %d (%v), want 0 (true)", ceiling, published)
	}
	if got := stable.collectionLimitName(""); got != "" {
		t.Fatalf("empty stable wire name = %q, want empty", got)
	}
}

// TestV3StableRequiredSensitiveVarsLimitRejectsBeforeMutation proves the
// stable pointer limit is applied to the planned set itself. The check only
// appends a diagnostic; the fake host therefore records no mutation event.
// Empty, missing, and unrelated limit declarations retain the profile's
// omission semantics and do not invent a refusal.
func TestV3StableRequiredSensitiveVarsLimitRejectsBeforeMutation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	host := newV3FakeHost(t)
	r := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	schemaResponse := v3SchemaOf(t, r)
	codec, err := r.codecTable().defaultCreate(v3GroupKind{
		APIVersion: r.form.Family.APIVersion(), Kind: r.form.Kind,
	})
	if err != nil {
		t.Fatalf("WorkerVersion create codec: %v", err)
	}

	cases := []struct {
		name      string
		members   []string
		limits    map[string]any
		wantError bool
	}{
		{
			name:      "stable zero rejects one required secret",
			members:   []string{"SECRET"},
			limits:    map[string]any{"/requiredSensitiveVars": int64(0)},
			wantError: true,
		},
		{
			name:    "stable zero accepts an empty set",
			limits:  map[string]any{"/requiredSensitiveVars": int64(0)},
			members: nil,
		},
		{
			name:    "missing stable limit remains undecided",
			members: []string{"SECRET"},
			limits:  map[string]any{},
		},
		{
			name:    "extra legacy pointer remains unrelated",
			members: []string{"SECRET"},
			limits:  map[string]any{"/maximumRequiredSensitiveVars": int64(0)},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
				"required_sensitive_vars": v3StringSetValue(t, testCase.members...),
			})
			// Seed the support cache so this exercises the same plan-time entrypoint
			// as ModifyPlan without requiring a second HTTP fixture. The support
			// answer is what a stable host's validated FormSupport profile carries.
			r.data.support = newV3SupportCache()
			r.data.support.forms[codec.Ref.ExactKey()] = v3SupportAnswer{Profile: map[string]any{
				"apiVersion": clientv3.SupportProfileAPIVersion,
				"kind":       "FormSupport",
				"formRef": map[string]any{
					"apiVersion":        codec.Ref.APIVersion,
					"kind":              codec.Ref.Kind,
					"definitionVersion": codec.Ref.DefinitionVersion,
					"schemaDigest":      codec.Ref.SchemaDigest,
				},
				"operations": []any{"create", "read", "delete"},
				"limits":     testCase.limits,
			}}
			response := frameworkresource.ModifyPlanResponse{Plan: plan}
			r.v3PlanHostSupport(ctx, &response)
			if got := response.Diagnostics.HasError(); got != testCase.wantError {
				t.Fatalf("diagnostics.HasError() = %v, want %v: %v", got, testCase.wantError, response.Diagnostics)
			}
			if testCase.wantError {
				detail := response.Diagnostics.Errors()[0].Detail()
				for _, want := range []string{"/requiredSensitiveVars", v3CodeLimitExceeded} {
					if !strings.Contains(detail, want) {
						t.Errorf("diagnostic detail does not carry %q:\n%s", want, detail)
					}
				}
			}
			if len(host.events) != 0 || len(host.applySpecs) != 0 {
				t.Fatalf("limit check caused host mutation: events=%v applySpecs=%v", host.events, host.applySpecs)
			}
		})
	}
}

// TestV3SupportUnreadableIsAWarningThatDecidesNothing pins the fail-open half of
// the rule.
func TestV3SupportUnreadableIsAWarningThatDecidesNothing(t *testing.T) {
	t.Parallel()
	rendered := v3SupportUnreadable("takoform_worker_version", "Form WorkerVersion", errors.New("i/o timeout"))
	if rendered.Severity().String() != "Warning" {
		t.Fatalf("severity = %s, want Warning: a provider must not refuse an apply for failing to ask",
			rendered.Severity())
	}
	detail := rendered.Detail()
	for _, want := range []string{
		"i/o timeout",
		"Nothing has been refused",
		"Code: " + v3CodeHostSupportUnknown,
		"capacity, price, region, and SLA are Service Offering facts",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not carry %q:\n%s", want, detail)
		}
	}
}
