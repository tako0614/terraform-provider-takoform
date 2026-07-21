// Command composition-manifest verifies a portable Capsule Composition
// manifest and emits its canonical digest for URL pinning.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/composition"
)

func main() {
	if len(os.Args) != 3 || os.Args[1] != "verify" {
		fatal("usage: composition-manifest verify PATH")
	}
	raw, err := os.ReadFile(os.Args[2])
	if err != nil {
		fatal(fmt.Sprintf("read manifest: %v", err))
	}
	manifest, digest, err := composition.Verify(raw)
	if err != nil {
		fatal(fmt.Sprintf("verify manifest: %v", err))
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Digest  string `json:"digest"`
	}{manifest.Metadata.Name, manifest.Metadata.Version, digest}); err != nil {
		fatal(fmt.Sprintf("write result: %v", err))
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "composition-manifest:", message)
	os.Exit(1)
}
