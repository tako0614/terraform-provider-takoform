package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestCronValidatorRejectsWhatTheHostRejects proves the plan-time half of the
// cron grammar: the provider parses the expression exactly as a conforming host
// does, so a value the host would refuse before any mutation is a plan-time
// error rather than a failed apply (spec/decisions/0020).
//
// Every offender below MATCHES the pattern carried in the Form Definition. That
// is the point: validating with the pattern alone would let all of them reach
// the host.
func TestCronValidatorRejectsWhatTheHostRejects(t *testing.T) {
	t.Parallel()
	validate := func(expression string) validator.StringResponse {
		var response validator.StringResponse
		v3CronValidator{}.ValidateString(context.Background(), validator.StringRequest{
			ConfigValue: types.StringValue(expression),
			Path:        path.Root("cron"),
		}, &response)
		return response
	}
	for _, offender := range []string{
		"0 24 * * *",
		"60 0 * * *",
		"5-1 * * * *",
		"*/0 * * * *",
		"0 0 32 * *",
		"0 0 * * 7",
	} {
		response := validate(offender)
		if !response.Diagnostics.HasError() {
			t.Fatalf("the plan accepted the cron expression %q, which a conforming host refuses", offender)
		}
		if summary := response.Diagnostics.Errors()[0].Summary(); !strings.Contains(summary, "Invalid cron expression") {
			t.Fatalf("%q produced %q", offender, summary)
		}
	}
	// The schedules the previous grammar could not express at all.
	for _, accepted := range []string{"* * * * *", "0 * * * *", "*/5 * * * *", "0 9-17 * * 1-5"} {
		if response := validate(accepted); response.Diagnostics.HasError() {
			t.Fatalf("the plan refused %q: %v", accepted, response.Diagnostics)
		}
	}
	// An unknown value is left to the host, exactly like every other string
	// validator in this provider.
	var unknown validator.StringResponse
	v3CronValidator{}.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringUnknown(),
		Path:        path.Root("cron"),
	}, &unknown)
	if unknown.Diagnostics.HasError() {
		t.Fatal("an unknown cron value was rejected during validation")
	}
}
