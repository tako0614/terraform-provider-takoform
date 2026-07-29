package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/admissionmaterial"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "standard-admission-material:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 || (args[0] != "build" && args[0] != "build-current") {
		return usageError()
	}
	commandName := args[0]
	values := map[string]string{}
	for args = args[1:]; len(args) > 0; args = args[2:] {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return usageError()
		}
		key := strings.TrimPrefix(args[0], "--")
		if _, duplicate := values[key]; duplicate {
			return fmt.Errorf("duplicate --%s", key)
		}
		values[key] = args[1]
	}
	required := []string{"host-id", "host-reports", "provider-reports", "output-dir", "admission-version", "host-source-commit", "host-takoform-source-commit", "provider-source-commit", "host-run-id", "provider-run-id"}
	if commandName == "build-current" {
		required = append(required,
			"host-request-id", "host-run-attempt", "host-head-sha",
			"provider-request-id", "provider-run-attempt", "provider-head-sha",
			"registry-readback", "registry-request-id", "registry-run-id",
			"registry-run-attempt", "registry-head-sha",
		)
	}
	for _, key := range required {
		if values[key] == "" {
			return usageError()
		}
	}
	if len(values) != len(required) {
		return usageError()
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		return err
	}
	command := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	raw, err := command.Output()
	if err != nil {
		return fmt.Errorf("resolve source commit: %w", err)
	}
	commit := strings.TrimSpace(string(raw))
	for label, ancestor := range map[string]string{
		"host Takoform source commit": values["host-takoform-source-commit"],
		"provider source commit":      values["provider-source-commit"],
	} {
		if err := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", ancestor, commit).Run(); err != nil {
			return fmt.Errorf("%s is not an ancestor of current source %s", label, commit)
		}
	}
	options := admissionmaterial.BuildOptions{
		Root: root, HostReports: values["host-reports"], ProviderReports: values["provider-reports"],
		OutputDir: values["output-dir"], AdmissionVersion: values["admission-version"], SourceCommit: commit,
		HostSourceCommit: values["host-source-commit"], HostTakoformSourceCommit: values["host-takoform-source-commit"], ProviderSourceCommit: values["provider-source-commit"],
		HostWorkflowRunID: values["host-run-id"], ProviderWorkflowRunID: values["provider-run-id"],
		HostID: values["host-id"],
	}
	if commandName == "build-current" {
		err = admissionmaterial.BuildCurrent(admissionmaterial.CurrentBuildOptions{
			BuildOptions:               options,
			HostRequestID:              values["host-request-id"],
			HostWorkflowRunAttempt:     values["host-run-attempt"],
			HostHeadSHA:                values["host-head-sha"],
			ProviderRequestID:          values["provider-request-id"],
			ProviderWorkflowRunAttempt: values["provider-run-attempt"],
			ProviderHeadSHA:            values["provider-head-sha"],
			RegistryReadback:           values["registry-readback"],
			RegistryRequestID:          values["registry-request-id"],
			RegistryWorkflowRunID:      values["registry-run-id"],
			RegistryWorkflowRunAttempt: values["registry-run-attempt"],
			RegistryHeadSHA:            values["registry-head-sha"],
		})
	} else {
		err = admissionmaterial.Build(options)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "standard-admission-material: built non-publishable %s admission %s material at %s\n", commandName, values["admission-version"], values["output-dir"])
	return err
}

func usageError() error {
	return fmt.Errorf("usage: standard-admission-material build [legacy options] | build-current --host-id ID --host-reports DIR --provider-reports DIR --registry-readback DIR --output-dir DIR --admission-version VERSION --host-source-commit COMMIT --host-takoform-source-commit COMMIT --provider-source-commit COMMIT --host-request-id UUID --host-run-id ID --host-run-attempt ATTEMPT --host-head-sha COMMIT --provider-request-id UUID --provider-run-id ID --provider-run-attempt ATTEMPT --provider-head-sha COMMIT --registry-request-id UUID --registry-run-id ID --registry-run-attempt ATTEMPT --registry-head-sha COMMIT")
}
