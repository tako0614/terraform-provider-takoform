// standard-form-conformance verifies and regenerates the catalog-derived
// public surfaces of the Edge Platform Family: one reference document and one
// example per Form, plus the Form inventory. The central-epoch subcommands
// that used to live beside these — release plans, candidate publication,
// legacy package and admission checks — were withdrawn with the pre-Beta
// generations they verified (decision 0042).
package main

import (
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: standard-form-conformance verify|generate-current-surfaces")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "verify":
		err = standardforms.Verify(".")
	case "generate-current-surfaces":
		err = standardforms.GenerateCurrentPublishedSurfaces(".")
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q; use verify or generate-current-surfaces\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "standard-form-structure:", err)
		os.Exit(1)
	}
	fmt.Printf("standard-form-structure: %s passed\n", os.Args[1])
}
