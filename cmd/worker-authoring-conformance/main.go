// Command worker-authoring-conformance drives the Edge Platform Family
// authoring surface — the raw v1alpha3 resources and the official `worker-app`
// module — through a real Terraform-compatible CLI against the deterministic
// v1alpha3 reference host.
//
// It is local implementation evidence only and never publication-ready.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/internal/workerauthoring"
)

const usage = "usage: go run ./cmd/worker-authoring-conformance [matrix|render-matrix] " +
	"[--opentofu PATH] [--terraform PATH]"

func main() {
	command := "matrix"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "matrix" || args[0] == "render-matrix") {
		command = args[0]
		args = args[1:]
	}
	openTofuPath := "tofu"
	terraformPath := "terraform"
	for len(args) > 0 {
		if len(args) < 2 {
			fail(fmt.Errorf("%s", usage))
		}
		switch args[0] {
		case "--opentofu":
			openTofuPath = args[1]
		case "--terraform":
			terraformPath = args[1]
		default:
			fail(fmt.Errorf("%s", usage))
		}
		args = args[2:]
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	matrix, err := workerauthoring.RunMatrix(ctx, root, openTofuPath, terraformPath)
	if err != nil {
		fail(err)
	}
	if err := workerauthoring.ValidateMatrix(matrix); err != nil {
		fail(err)
	}
	if command == "render-matrix" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(matrix); err != nil {
			fail(err)
		}
		return
	}
	fmt.Printf(
		"verified non-publishable worker authoring evidence: %d CLIs, %d validated configurations, "+
			"same-name replacement refused at plan, roll-forward serves throughout\n",
		len(matrix.Reports), len(matrix.Reports[0].Configurations),
	)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "worker-authoring-conformance:", err)
	os.Exit(1)
}
