//go:build retained_v1_conformance

// Command retained-v1-provider is an intentionally separate, local-only
// plugin for the frozen stable-v1 worker-authoring conformance fixture. It is
// never the shipping provider entrypoint or a release artifact.
package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

var version = "4.1.0-retained-v1-conformance"

func main() {
	if err := providerserver.Serve(context.Background(), provider.NewRetainedV1ForConformance(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/tako0614/takoform",
	}); err != nil {
		log.Fatal(err)
	}
}
