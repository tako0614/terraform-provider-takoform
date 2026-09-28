// Command provider-container-candidate serves the local, unpublished typed
// Container/Vector/Worker binding schema candidate. It intentionally exposes
// no Host planning or mutation behavior.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

//go:embed container-http-candidate.json
var candidateSource []byte

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("usage: terraform-provider-takoform-container-candidate [-debug]")
	}
	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/tako0614/takoform",
		Debug:   debug,
	}
	if err := providerserver.Serve(context.Background(), provider.NewContainerCandidate(candidateSource), opts); err != nil {
		log.Fatal(fmt.Sprintf("container candidate provider: %v", err))
	}
}
