// Command terraform-provider-takoform serves the Takoform OpenTofu/Terraform
// provider plugin and exposes explicit local maintenance subcommands.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/tako0614/terraform-provider-takoform/internal/intentstorecli"
	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

// version is overridden at build time with -ldflags "-X main.version=<v>".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "intent-store-init" {
		os.Exit(intentstorecli.Run(os.Args[2:], os.Stdout, os.Stderr))
	}

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
		// Canonical plugin handshake and state identity. Terraform and OpenTofu
		// both install this exact FQN from the Terraform Registry.
		Address: "registry.terraform.io/tako0614/takoform",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
