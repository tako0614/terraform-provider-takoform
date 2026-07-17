package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
)

func main() {
	command := "verify"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "verify" || args[0] == "render" || args[0] == "matrix" || args[0] == "render-matrix") {
		// Commands are deliberately limited below; parsing them before --cli keeps
		// the common `verify --cli /path` invocation terse.
		command = args[0]
		args = args[1:]
	}
	if command == "matrix" || command == "render-matrix" {
		runMatrix(command, args)
		return
	}
	cliPath := os.Getenv("TAKOFORM_TERRAFORM_CLI")
	for len(args) > 0 {
		if args[0] != "--cli" || len(args) < 2 {
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance [verify|render] [--cli PATH]"))
		}
		cliPath = args[1]
		args = args[2:]
	}
	if cliPath == "" {
		cliPath = "tofu"
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	report, err := providerlifecycle.Run(context.Background(), root, cliPath)
	if err != nil {
		fail(err)
	}
	switch command {
	case "verify":
		if err := providerlifecycle.Validate(report); err != nil {
			fail(err)
		}
		fmt.Printf("verified non-publishable generic provider binary lifecycle candidate: %s %s, %d typed resources\n", report.CLI.Product, report.CLI.Version, len(report.Resources))
	case "render":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance [verify|render] [--cli PATH]"))
	}
}

func runMatrix(command string, args []string) {
	openTofuPath := "tofu"
	terraformPath := "terraform"
	for len(args) > 0 {
		if len(args) < 2 {
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance [matrix|render-matrix] [--opentofu PATH] [--terraform PATH]"))
		}
		switch args[0] {
		case "--opentofu":
			openTofuPath = args[1]
		case "--terraform":
			terraformPath = args[1]
		default:
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance [matrix|render-matrix] [--opentofu PATH] [--terraform PATH]"))
		}
		args = args[2:]
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	report, err := providerlifecycle.RunMatrix(context.Background(), root, openTofuPath, terraformPath)
	if err != nil {
		fail(err)
	}
	if command == "render-matrix" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail(err)
		}
		return
	}
	fmt.Printf("verified non-publishable supported CLI/FQN candidate matrix: %d CLIs, %d exact typed resources\n", len(report.Reports), len(report.Reports[0].Resources))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "provider-lifecycle-conformance:", err)
	os.Exit(1)
}
