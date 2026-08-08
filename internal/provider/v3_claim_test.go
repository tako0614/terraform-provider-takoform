package provider

// v3_claim_test.go proves the client half of two rules a host enforces: the
// module media-type set a Worker Bundle may declare (spec/decisions/0012 and
// 0019), and the canonical spelling of a DNS hostname (spec/decisions/0023).
//
// Both are plan-time refusals for the same reason. A configuration that fails
// at apply is a configuration whose plan lied, and a configuration the host
// would REWRITE is a configuration that plans one value and reads back another
// forever.

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// TestWorkerBundleAdmitsExactlyTheBundleMediaTypeSet drives the provider's
// content_type validator over every admitted media type and over the one this
// ABI version does not support.
func TestWorkerBundleAdmitsExactlyTheBundleMediaTypeSet(t *testing.T) {
	t.Parallel()
	check := StringOneOf(workerBundleMediaTypes...)
	validate := func(value string) validator.StringResponse {
		var response validator.StringResponse
		check.ValidateString(context.Background(), validator.StringRequest{
			ConfigValue: types.StringValue(value),
			Path:        path.Root("content_type"),
		}, &response)
		return response
	}
	for _, mediaType := range model.BundleModuleMediaTypes() {
		if response := validate(mediaType); response.Diagnostics.HasError() {
			t.Fatalf("the authoring surface refused the bundle media type %s", mediaType)
		}
	}
	// The manifest admits it nowhere, so the plan must not.
	if response := validate("application/json"); !response.Diagnostics.HasError() {
		t.Fatal("the authoring surface accepted an application/json module; this ABI version loads none")
	}
}

// TestMainModuleMustBeLoadable proves an auxiliary module may sit in the bundle
// and may never be the module the runtime instantiates first.
func TestMainModuleMustBeLoadable(t *testing.T) {
	t.Parallel()
	modules := v3WorkerBundleModulesValue([]v3BundleModule{
		{Name: "index.js", ContentType: "application/javascript+module", ContentFile: "index.js"},
		{Name: "index.js.map", ContentType: "application/source-map+json", ContentFile: "index.js.map"},
	}, nil)

	carried, diags := v3AuthoredBundleModules(modules, "index.js")
	if diags.HasError() {
		t.Fatalf("a bundle carrying a source map beside its code was refused: %v", diags)
	}
	if len(carried) != 2 {
		t.Fatalf("the source map was dropped from the authored bundle: %v", carried)
	}
	_, refused := v3AuthoredBundleModules(modules, "index.js.map")
	if !refused.HasError() {
		t.Fatal("main_module named a source map, which the module graph never imports")
	}
	if summary := refused.Errors()[0].Summary(); !strings.Contains(summary, "not loadable") {
		t.Fatalf("main_module refusal reported %q", summary)
	}
}

// TestHostnameValidatorRequiresTheCanonicalSpelling proves the plan-time half
// of hostname canonicalization: the host stores the canonical spelling, so a
// configuration the host would rewrite is refused with the spelling to write
// instead of drifting on every refresh.
func TestHostnameValidatorRequiresTheCanonicalSpelling(t *testing.T) {
	t.Parallel()
	validate := func(hostname string) validator.StringResponse {
		var response validator.StringResponse
		v3HostnameValidator{}.ValidateString(context.Background(), validator.StringRequest{
			ConfigValue: types.StringValue(hostname),
			Path:        path.Root("hostname"),
		}, &response)
		return response
	}
	// Every offender MATCHES the pattern the Form Definition carries. That is
	// the point: the pattern admits the spellings DNS treats as one name.
	for _, offender := range []string{
		"API.Example.com",
		"api.example.com.",
		"API.EXAMPLE.COM.",
	} {
		response := validate(offender)
		if !response.Diagnostics.HasError() {
			t.Fatalf("the plan accepted %q, which a host would store as %q", offender, model.CanonicalHostname(offender))
		}
		if summary := response.Diagnostics.Errors()[0].Summary(); !strings.Contains(summary, "not canonical") {
			t.Fatalf("%q produced %q", offender, summary)
		}
		if detail := response.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "api.example.com") {
			t.Fatalf("the refusal must name the canonical spelling; got %q", detail)
		}
	}
	if response := validate("api.example.com"); response.Diagnostics.HasError() {
		t.Fatalf("the plan refused a canonical hostname: %v", response.Diagnostics)
	}
	// A U-label never travels: an internationalized name is written as its
	// A-label, and the pattern admits no non-ASCII byte.
	if response := validate("ドメイン.example.com"); !response.Diagnostics.HasError() {
		t.Fatal("the plan accepted a U-label; an internationalized name travels as its A-label")
	}
	if response := validate("xn--eckwd4c7c.example.com"); response.Diagnostics.HasError() {
		t.Fatalf("the plan refused an A-label: %v", response.Diagnostics)
	}
	var unknown validator.StringResponse
	v3HostnameValidator{}.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringUnknown(),
		Path:        path.Root("hostname"),
	}, &unknown)
	if unknown.Diagnostics.HasError() {
		t.Fatal("an unknown hostname must be left to the host, like every other string validator here")
	}
}

// TestCanonicalHostnameIsIdempotent pins the property the store depends on: a
// canonical value canonicalizes to itself, so a re-apply moves nothing.
func TestCanonicalHostnameIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, written := range []string{
		"API.Example.com",
		"api.example.com.",
		"API.EXAMPLE.COM.",
		"api.example.com",
	} {
		once := model.CanonicalHostname(written)
		if twice := model.CanonicalHostname(once); twice != once {
			t.Fatalf("canonicalizing %q twice gave %q then %q", written, once, twice)
		}
		if !model.HostnameIsCanonical(once) {
			t.Fatalf("the canonical form of %q does not report itself canonical", written)
		}
	}
}
