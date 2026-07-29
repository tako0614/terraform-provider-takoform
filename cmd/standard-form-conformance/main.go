package main

import (
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "generate" && os.Args[1] != "verify" && os.Args[1] != "materializability-check" && os.Args[1] != "release-plan" && os.Args[1] != "candidate-publication-check" && os.Args[1] != "published-package-check" && os.Args[1] != "retained-ga-core-v1-published-package-check" && os.Args[1] != "current-published-package-check" && os.Args[1] != "admission-closure-check" && os.Args[1] != "current-admission-closure-check") {
		fmt.Fprintln(os.Stderr, "usage: standard-form-conformance generate|verify|release-plan|materializability-check|candidate-publication-check|published-package-check|retained-ga-core-v1-published-package-check|current-published-package-check|admission-closure-check|current-admission-closure-check")
		os.Exit(2)
	}
	var err error
	if os.Args[1] == "generate" {
		err = standardforms.Generate(".")
	} else if os.Args[1] == "verify" {
		err = standardforms.Verify(".")
	} else if os.Args[1] == "release-plan" {
		var rendered string
		rendered, err = standardforms.RenderReleasePlan(".")
		if err == nil {
			fmt.Print(rendered)
		}
	} else if os.Args[1] == "materializability-check" {
		err = standardforms.VerifyMaterializableCandidate(".")
	} else if os.Args[1] == "candidate-publication-check" {
		err = standardforms.VerifyCandidatePublication(".")
	} else if os.Args[1] == "published-package-check" {
		err = standardforms.VerifyPublishedPackageSet(".")
	} else if os.Args[1] == "retained-ga-core-v1-published-package-check" {
		err = standardforms.VerifyRetainedGaCoreV1PublishedPackageSet(".")
	} else if os.Args[1] == "current-published-package-check" {
		err = standardforms.VerifyCurrentPublishedPackageSet(".")
	} else if os.Args[1] == "admission-closure-check" {
		err = standardforms.VerifyAdmissionClosure(".")
	} else {
		err = standardforms.VerifyCurrentAdmissionClosure(".")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "standard-form-conformance:", err)
		os.Exit(1)
	}
	fmt.Println("standard-form-structure:", os.Args[1], "passed")
}
