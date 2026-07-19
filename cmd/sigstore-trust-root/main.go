package main

import (
	"fmt"
	"os"
	"path/filepath"

	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const outputPath = "admission/v1/trust/trusted-root.json"

func main() {
	if len(os.Args) != 2 || os.Args[1] != "refresh" {
		fmt.Fprintln(os.Stderr, "usage: sigstore-trust-root refresh")
		os.Exit(2)
	}
	options := tuf.DefaultOptions().WithDisableLocalCache()
	client, err := tuf.New(options)
	if err != nil {
		fail(err)
	}
	raw, err := client.GetTarget("trusted_root.json")
	if err != nil {
		fail(err)
	}
	if _, err := sigstoreroot.NewTrustedRootFromJSON(raw); err != nil {
		fail(fmt.Errorf("verify TUF-authenticated trusted root: %w", err))
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fail(err)
	}
	temporary := outputPath + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o644); err != nil {
		fail(err)
	}
	if err := os.Rename(temporary, outputPath); err != nil {
		fail(err)
	}
	fmt.Printf("%s  %s\n", formpackage.DigestBytes(raw), outputPath)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sigstore-trust-root:", err)
	os.Exit(1)
}
