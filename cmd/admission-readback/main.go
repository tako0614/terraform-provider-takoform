// Command admission-readback renders deterministic unsigned Phase 2 readback
// subjects. It never signs evidence, publishes a release, or changes Form
// admission status.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admission-readback:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "registry" {
		return usageError()
	}
	flags := flag.NewFlagSet("registry", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	matrix := flags.String("matrix", "", "direct Registry lifecycle matrix JSON")
	commit := flags.String("provider-release-commit", "", "exact published provider tag commit")
	output := flags.String("output", "", "create the canonical readback at this path instead of stdout")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *matrix == "" || *commit == "" {
		return usageError()
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		return err
	}
	_, canonical, err := admissionrelease.BuildRegistryReadback(root, *matrix, *commit)
	if err != nil {
		return err
	}
	if *output == "" {
		_, err = os.Stdout.Write(canonical)
		return err
	}
	handle, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := handle.Write(canonical); err != nil {
		_ = handle.Close()
		_ = os.Remove(*output)
		return err
	}
	if err := handle.Close(); err != nil {
		_ = os.Remove(*output)
		return err
	}
	return nil
}

func usageError() error {
	return fmt.Errorf("usage: admission-readback registry --matrix FILE --provider-release-commit COMMIT [--output FILE]")
}
