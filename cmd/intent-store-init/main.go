package main

import (
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/intentstorecli"
)

func main() {
	os.Exit(intentstorecli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
