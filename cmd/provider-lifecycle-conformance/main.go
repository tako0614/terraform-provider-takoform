package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/internal/providerreport"
)

func main() {
	command := "verify"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "verify" || args[0] == "render" || args[0] == "matrix" || args[0] == "render-matrix" || args[0] == "registry-report" || args[0] == "render-registry-report" || args[0] == "registry-matrix" || args[0] == "render-registry-matrix" || args[0] == "provider-reports" || args[0] == "verify-provider-reports") {
		// Commands are deliberately limited below; parsing them before --cli keeps
		// the common `verify --cli /path` invocation terse.
		command = args[0]
		args = args[1:]
	}
	if command == "provider-reports" {
		runProviderReports(args)
		return
	}
	if command == "verify-provider-reports" {
		runVerifyProviderReports(args)
		return
	}
	if command == "registry-report" || command == "render-registry-report" {
		runRegistryReport(command, args)
		return
	}
	if command == "matrix" || command == "render-matrix" || command == "registry-matrix" || command == "render-registry-matrix" {
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

func runRegistryReport(command string, args []string) {
	cliPath := ""
	for len(args) > 0 {
		if len(args) < 2 || args[0] != "--cli" {
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance [registry-report|render-registry-report] --cli PATH"))
		}
		cliPath = args[1]
		args = args[2:]
	}
	if cliPath == "" {
		fail(fmt.Errorf("registry report requires --cli PATH"))
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	report, err := providerlifecycle.RunRegistry(context.Background(), root, cliPath)
	if err != nil {
		fail(err)
	}
	if err := providerlifecycle.ValidateRegistry(report); err != nil {
		fail(err)
	}
	if command == "render-registry-report" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail(err)
		}
		return
	}
	fmt.Printf("verified non-publishable direct Registry lifecycle report: %s %s, %d exact typed resources\n", report.CLI.Product, report.CLI.Version, len(report.Resources))
}

func runProviderReports(args []string) {
	cliPath := os.Getenv("TAKOFORM_TERRAFORM_CLI")
	outputDir := ""
	sourceCommit := ""
	for len(args) > 0 {
		if len(args) < 2 {
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance provider-reports --cli PATH --output-dir DIRECTORY --source-commit COMMIT"))
		}
		switch args[0] {
		case "--cli":
			cliPath = args[1]
		case "--output-dir":
			outputDir = args[1]
		case "--source-commit":
			sourceCommit = args[1]
		default:
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance provider-reports --cli PATH --output-dir DIRECTORY --source-commit COMMIT"))
		}
		args = args[2:]
	}
	if cliPath == "" {
		cliPath = "tofu"
	}
	if outputDir == "" {
		fail(fmt.Errorf("provider-reports requires --output-dir outside admission/"))
	}
	if sourceCommit == "" {
		fail(fmt.Errorf("provider-reports requires --source-commit"))
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	reports, err := providerreport.Generate(context.Background(), root, cliPath)
	if err != nil {
		fail(err)
	}
	if _, err := providerreport.Export(root, outputDir, sourceCommit, reports); err != nil {
		fail(err)
	}
	fmt.Printf("generated %d unsigned RFC 8785 provider reports from exact Legacy compatibility release-source fixtures via %s (%s)\n", len(reports), reports[0].Subject(), providerreport.ProtocolDescription())
	for _, value := range providerreport.SortedReportDigests(reports) {
		fmt.Println(value)
	}
}

func runVerifyProviderReports(args []string) {
	inputDir := ""
	sourceCommit := ""
	for len(args) > 0 {
		if len(args) < 2 {
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance verify-provider-reports --input-dir DIRECTORY --source-commit COMMIT"))
		}
		switch args[0] {
		case "--input-dir":
			inputDir = args[1]
		case "--source-commit":
			sourceCommit = args[1]
		default:
			fail(fmt.Errorf("usage: go run ./cmd/provider-lifecycle-conformance verify-provider-reports --input-dir DIRECTORY --source-commit COMMIT"))
		}
		args = args[2:]
	}
	if inputDir == "" || sourceCommit == "" {
		fail(fmt.Errorf("verify-provider-reports requires --input-dir and --source-commit"))
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	inventory, err := providerreport.VerifyDirectory(root, inputDir, sourceCommit)
	if err != nil {
		fail(err)
	}
	fmt.Printf("verified %d exact unsigned RFC 8785 provider reports for %s at %s\n", len(inventory.Reports), inventory.Subject, inventory.Source.Commit)
}

func runMatrix(command string, args []string) {
	openTofuPath, terraformPath, providerBinaryPath, err := parseMatrixArgs(command, args)
	if err != nil {
		fail(err)
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	registry := command == "registry-matrix" || command == "render-registry-matrix"
	var report providerlifecycle.MatrixReport
	if registry {
		report, err = providerlifecycle.RunRegistryMatrix(context.Background(), root, openTofuPath, terraformPath)
	} else if providerBinaryPath != "" {
		report, err = providerlifecycle.RunMatrixWithProviderBinary(context.Background(), root, openTofuPath, terraformPath, providerBinaryPath)
	} else {
		report, err = providerlifecycle.RunMatrix(context.Background(), root, openTofuPath, terraformPath)
	}
	if err != nil {
		fail(err)
	}
	if command == "render-matrix" || command == "render-registry-matrix" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail(err)
		}
		return
	}
	mode := "local dev-override"
	if registry {
		mode = "direct Registry install/readback"
	}
	fmt.Printf("verified Legacy compatibility CLI/FQN matrix (%s): %d CLIs, %d exact typed resources\n", mode, len(report.Reports), len(report.Reports[0].Resources))
}

func parseMatrixArgs(command string, args []string) (string, string, string, error) {
	const usage = "usage: go run ./cmd/provider-lifecycle-conformance [matrix|render-matrix|registry-matrix|render-registry-matrix] [--opentofu PATH] [--terraform PATH] [--provider-binary PATH]"
	openTofuPath := "tofu"
	terraformPath := "terraform"
	providerBinaryPath := ""
	for len(args) > 0 {
		if len(args) < 2 {
			return "", "", "", fmt.Errorf("%s", usage)
		}
		switch args[0] {
		case "--opentofu":
			openTofuPath = args[1]
		case "--terraform":
			terraformPath = args[1]
		case "--provider-binary":
			providerBinaryPath = args[1]
		default:
			return "", "", "", fmt.Errorf("%s", usage)
		}
		args = args[2:]
	}
	if providerBinaryPath != "" && (command == "registry-matrix" || command == "render-registry-matrix") {
		return "", "", "", fmt.Errorf("--provider-binary is only valid for matrix and render-matrix")
	}
	return openTofuPath, terraformPath, providerBinaryPath, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "provider-lifecycle-conformance:", err)
	os.Exit(1)
}
