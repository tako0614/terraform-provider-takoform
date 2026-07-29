package main

import (
	"strings"
	"testing"
)

func TestParseMatrixArgsAcceptsExactProviderBinaryForLocalMatrix(t *testing.T) {
	openTofu, terraform, provider, err := parseMatrixArgs("render-matrix", []string{
		"--opentofu", "/tools/tofu",
		"--terraform", "/tools/terraform",
		"--provider-binary", "/release/terraform-provider-takoform",
	})
	if err != nil {
		t.Fatal(err)
	}
	if openTofu != "/tools/tofu" || terraform != "/tools/terraform" ||
		provider != "/release/terraform-provider-takoform" {
		t.Fatalf("parsed matrix args = %q, %q, %q", openTofu, terraform, provider)
	}
}

func TestParseMatrixArgsRejectsProviderBinaryForRegistryMatrix(t *testing.T) {
	_, _, _, err := parseMatrixArgs("render-registry-matrix", []string{
		"--provider-binary", "/release/terraform-provider-takoform",
	})
	if err == nil || !strings.Contains(err.Error(), "only valid for matrix and render-matrix") {
		t.Fatalf("registry provider-binary error = %v", err)
	}
}
