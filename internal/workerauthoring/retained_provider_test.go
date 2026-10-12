package workerauthoring

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	providerimpl "github.com/tako0614/terraform-provider-takoform/internal/provider"
)

func TestDefaultWorkerAuthoringPluginAcceptsRetainedV1HCL(t *testing.T) {
	tofu, err := exec.LookPath("tofu")
	if err != nil {
		t.Skip("OpenTofu is unavailable")
	}
	repoRoot, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	h, err := startHarness(ctx, repoRoot, tofu, harnessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := h.write("main.tf", providerBlock(h.Endpoint())+`resource "takoform_module_worker" "app" {
  name = "app"
}
`); err != nil {
		t.Fatal(err)
	}
	output, err := h.run(ctx, "validate", "-no-color")
	if err != nil {
		t.Fatalf("retained v1 HCL was rejected: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Success! The configuration is valid") {
		t.Fatalf("OpenTofu did not validate retained v1 HCL: %s", output)
	}
}

func TestShippingProviderFactoryRemainsV2Only(t *testing.T) {
	shipping := providerimpl.New("5.0.0")()
	var response provider.SchemaResponse
	shipping.Schema(context.Background(), provider.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatal(response.Diagnostics)
	}
	for _, field := range []string{"host_api_root", "store_id"} {
		attribute, ok := response.Schema.Attributes[field].(schema.StringAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("shipping v2 lost required %s", field)
		}
	}
	// Configure, not the schema, enforces exactly one of these two backends.
	for _, field := range []string{"store_path", "intent_store_slot"} {
		attribute, ok := response.Schema.Attributes[field].(schema.StringAttribute)
		if !ok || !attribute.Optional || attribute.Required {
			t.Fatalf("shipping v2 lost optional intent store backend %s", field)
		}
	}
	if got := len(shipping.Resources(context.Background())); got != 13 {
		t.Fatalf("shipping v2 registers %d resources, want 13", got)
	}
	for _, factory := range shipping.Resources(context.Background()) {
		var metadata resource.MetadataResponse
		factory().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		if metadata.TypeName == "" {
			t.Fatal("shipping v2 resource has no type name")
		}
	}
}

func TestExplicitShippingBinaryDoesNotGainRetainedV1Fallback(t *testing.T) {
	tofu, err := exec.LookPath("tofu")
	if err != nil {
		t.Skip("OpenTofu is unavailable")
	}
	repoRoot, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "terraform-provider-takoform")
	if output, err := runCommand(ctx, repoRoot, nil, "go", "build", "-buildvcs=false", "-o", binary, "."); err != nil {
		t.Fatalf("build shipping v2 plugin: %v\n%s", err, output)
	}
	h, err := startHarness(ctx, repoRoot, tofu, harnessOptions{providerBinary: binary})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := h.write("main.tf", providerBlock(h.Endpoint())+`resource "takoform_module_worker" "app" {
  name = "app"
}
`); err != nil {
		t.Fatal(err)
	}
	output, err := h.run(ctx, "validate", "-no-color")
	if err == nil || !strings.Contains(output, `The argument "host_api_root" is required`) ||
		!strings.Contains(output, `The argument "store_id" is required`) {
		t.Fatalf("explicit shipping v2 binary accepted retained v1 HCL: error=%v\n%s", err, output)
	}
}
