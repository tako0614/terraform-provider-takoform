package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestArtifactSourceRejectsURLsThatCannotEnterNonsensitiveState(t *testing.T) {
	t.Parallel()

	const digest = "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
	source := func(artifactURL string) artifactSourceValues {
		return artifactSourceValues{
			URL:       types.StringValue(artifactURL),
			SHA256:    types.StringValue(digest),
			MediaType: types.StringValue("application/vnd.takoform.test+tar"),
		}
	}
	if _, diags := source("https://artifacts.portable-conformance.invalid/app.tar").toSpec("test"); diags.HasError() {
		t.Fatalf("credential-free artifact URL was rejected: %v", diags)
	}
	for name, artifactURL := range map[string]string{
		"userinfo": "https://builder@artifacts.portable-conformance.invalid/app.tar",
		"query":    "https://artifacts.portable-conformance.invalid/app.tar?download=1",
		"fragment": "https://artifacts.portable-conformance.invalid/app.tar#archive",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, diags := source(artifactURL).toSpec("test"); !diags.HasError() {
				t.Fatalf("artifact URL %q was accepted into nonsensitive state", artifactURL)
			}
		})
	}
}
