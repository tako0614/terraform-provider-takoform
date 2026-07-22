package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "generate" && os.Args[1] != "verify" && os.Args[1] != "materializability-check" && os.Args[1] != "candidate-publication-check" && os.Args[1] != "published-package-check" && os.Args[1] != "admission-closure-check" && os.Args[1] != "release-check") {
		fmt.Fprintln(os.Stderr, "usage: standard-form-conformance generate|verify|materializability-check|candidate-publication-check|published-package-check|admission-closure-check|release-check")
		os.Exit(2)
	}
	var err error
	if os.Args[1] == "generate" {
		err = standardforms.Generate(".")
	} else if os.Args[1] == "verify" {
		err = standardforms.Verify(".")
	} else if os.Args[1] == "materializability-check" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err = standardforms.VerifyMaterializationReadback(ctx, ".", &http.Client{Timeout: 30 * time.Second})
	} else if os.Args[1] == "candidate-publication-check" {
		err = standardforms.VerifyCandidatePublication(".")
	} else if os.Args[1] == "published-package-check" {
		err = standardforms.VerifyPublishedPackageSet(".")
	} else if os.Args[1] == "admission-closure-check" {
		err = standardforms.VerifyAdmissionClosure(".")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err = standardforms.VerifyReleaseReady(ctx, ".", &http.Client{Timeout: 30 * time.Second}, os.Getenv("GITHUB_TOKEN"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "standard-form-conformance:", err)
		os.Exit(1)
	}
	fmt.Println("standard-form-structure:", os.Args[1], "passed")
}
