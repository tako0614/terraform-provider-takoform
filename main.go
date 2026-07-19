// Command terraform-provider-takoform is the thin Takoform OpenTofu/Terraform
// provider plugin entrypoint. It serves the provider over the Terraform plugin
// protocol; all behavior lives in internal/provider.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

// version is overridden at build time with -ldflags "-X main.version=<v>".
var version = "dev"

func main() {
	var debug bool
	var showVersion bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.BoolVar(&showVersion, "version", false, "print the embedded provider version and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(version)
		return
	}

	opts := providerserver.ServeOpts{
		// Canonical plugin handshake identity for the Terraform Registry release.
		// OpenTofu separately distributes the same provider under its own FQN;
		// that FQN remains a distinct state identity and is never a silent alias.
		Address: "registry.terraform.io/tako0614/takoform",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
