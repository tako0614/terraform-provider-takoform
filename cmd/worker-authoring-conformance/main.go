// Command worker-authoring-conformance drives the Edge Platform Family
// authoring surface — the stable resources and the repository `worker-app`
// module — through a real Terraform-compatible CLI against the deterministic
// stable-v1 reference host.
//
// It is local implementation evidence only and never publication-ready.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/workerauthoring"
)

const usage = "usage: go run ./cmd/worker-authoring-conformance [matrix|render-matrix] " +
	"[--opentofu PATH] [--terraform PATH] [--provider-binary PATH]"

func main() {
	command := "matrix"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "matrix" || args[0] == "render-matrix") {
		command = args[0]
		args = args[1:]
	}
	openTofuPath := "tofu"
	terraformPath := "terraform"
	providerBinary := ""
	for len(args) > 0 {
		if len(args) < 2 {
			fail(fmt.Errorf("%s", usage))
		}
		switch args[0] {
		case "--opentofu":
			openTofuPath = args[1]
		case "--terraform":
			terraformPath = args[1]
		case "--provider-binary":
			providerBinary = args[1]
		default:
			fail(fmt.Errorf("%s", usage))
		}
		args = args[2:]
	}
	root, err := workerauthoring.RepoRoot(".")
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	matrix, err := workerauthoring.RunMatrixWithProvider(ctx, root, openTofuPath, terraformPath, providerBinary)
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
	first := matrix.Reports[0]
	fmt.Printf(
		"verified non-publishable worker authoring evidence: %d CLIs, %d validated configurations, "+
			"same-name replacement refused at plan, roll-forward serves throughout "+
			"(%d Ready samples, %d not ready), two owners of identical output hold %d distinct revisions, "+
			"heterogeneous vars keep their JSON types, destroy removes the %d-resource aggregate "+
			"in dependency order and leaves %d behind\n",
		len(matrix.Reports), len(first.Configurations),
		first.ModuleDeploy.ReadySamples, first.ModuleDeploy.NotReadySamples,
		len(first.TwoOwners.BundleNames)+len(first.TwoOwners.VersionNames),
		len(first.ModuleDestroy.Mutations), len(first.ModuleDestroy.LeftBehind),
	)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "worker-authoring-conformance:", err)
	os.Exit(1)
}
