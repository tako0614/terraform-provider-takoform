// Command terraform-provider-takoform is the thin Takoform OpenTofu/Terraform
// provider plugin entrypoint. It serves the provider over the Terraform plugin
// protocol; all behavior lives in internal/provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

// version is overridden at build time with -ldflags "-X main.version=<v>".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// Canonical Terraform Registry identity. OpenTofu accepts the same
		// fully-qualified provider address.
		Address: "registry.terraform.io/tako0614/takoform",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
