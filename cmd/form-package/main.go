package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "form-package:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return usageError()
	}
	switch arguments[0] {
	case "verify":
		if len(arguments) != 2 {
			return usageError()
		}
		report, err := formpackage.VerifyDirectory(arguments[1])
		if err != nil {
			return err
		}
		return writeJSON(report)
	case "canonicalize":
		if len(arguments) != 2 {
			return usageError()
		}
		raw, err := os.ReadFile(arguments[1])
		if err != nil {
			return err
		}
		canonical, err := formpackage.Canonicalize(raw)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(canonical, '\n'))
		return err
	case "digest":
		if len(arguments) != 2 {
			return usageError()
		}
		raw, err := os.ReadFile(arguments[1])
		if err != nil {
			return err
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return err
		}
		fmt.Println(digest)
		return nil
	case "validate-revocation":
		if len(arguments) != 2 {
			return usageError()
		}
		raw, err := os.ReadFile(arguments[1])
		if err != nil {
			return err
		}
		statement, err := formpackage.ValidateRevocationStatement(raw)
		if err != nil {
			return err
		}
		return writeJSON(statement)
	case "conformance":
		if len(arguments) > 2 {
			return usageError()
		}
		root := filepath.FromSlash("conformance/form-package-v1")
		if len(arguments) == 2 {
			root = arguments[1]
		}
		report, err := formpackage.VerifyConformance(root)
		if err != nil {
			return err
		}
		return writeJSON(report)
	default:
		return usageError()
	}
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usageError() error {
	return fmt.Errorf("usage: form-package verify DIR | canonicalize FILE | digest FILE | validate-revocation FILE | conformance [CORPUS_DIR]")
}
