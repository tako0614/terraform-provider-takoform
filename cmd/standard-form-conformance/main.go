package main

import (
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "generate" && os.Args[1] != "verify") {
		fmt.Fprintln(os.Stderr, "usage: standard-form-conformance generate|verify")
		os.Exit(2)
	}
	var err error
	if os.Args[1] == "generate" {
		err = standardforms.Generate(".")
	} else {
		err = standardforms.Verify(".")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "standard-form-conformance:", err)
		os.Exit(1)
	}
	fmt.Println("standard-form-structure:", os.Args[1], "passed")
}
