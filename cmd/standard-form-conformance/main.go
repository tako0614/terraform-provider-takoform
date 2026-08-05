package main

import (
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "verify" && os.Args[1] != "generate-current-surfaces" && os.Args[1] != "materializability-check" && os.Args[1] != "release-plan" && os.Args[1] != "current-release-plan" && os.Args[1] != "candidate-publication-check" && os.Args[1] != "published-package-check" && os.Args[1] != "retained-ga-core-v1-published-package-check" && os.Args[1] != "legacy-published-package-check" && os.Args[1] != "legacy-admission-evidence-check") {
		fmt.Fprintln(os.Stderr, "usage: standard-form-conformance verify|generate-current-surfaces|release-plan|current-release-plan|materializability-check|candidate-publication-check|published-package-check|retained-ga-core-v1-published-package-check|legacy-published-package-check|legacy-admission-evidence-check")
		os.Exit(2)
	}
	var err error
	if os.Args[1] == "verify" {
		err = standardforms.Verify(".")
	} else if os.Args[1] == "generate-current-surfaces" {
		err = standardforms.GenerateCurrentPublishedSurfaces(".")
	} else if os.Args[1] == "release-plan" {
		var rendered string
		rendered, err = standardforms.RenderReleasePlan(".")
		if err == nil {
			fmt.Print(rendered)
		}
	} else if os.Args[1] == "current-release-plan" {
		var rendered string
		rendered, err = standardforms.RenderCurrentReleasePlan(".")
		if err == nil {
			fmt.Print(rendered)
			return
		}
	} else if os.Args[1] == "materializability-check" {
		err = standardforms.VerifyMaterializableCandidate(".")
	} else if os.Args[1] == "candidate-publication-check" {
		err = standardforms.VerifyCandidatePublication(".")
	} else if os.Args[1] == "published-package-check" {
		err = standardforms.VerifyPublishedPackageSet(".")
	} else if os.Args[1] == "retained-ga-core-v1-published-package-check" {
		err = standardforms.VerifyRetainedGaCoreV1PublishedPackageSet(".")
	} else if os.Args[1] == "legacy-published-package-check" {
		err = standardforms.VerifyLegacyPublishedPackageSet(".")
	} else {
		err = standardforms.VerifyLegacyAdmissionEvidence(".")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "standard-form-conformance:", err)
		os.Exit(1)
	}
	fmt.Println("standard-form-structure:", os.Args[1], "passed")
}
