package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/characterization"
)

func main() {
	command := "verify"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	repoRoot, err := characterization.FindRepoRoot(".")
	if err != nil {
		fatal(err)
	}
	evidenceRoot := characterization.EvidenceRoot(repoRoot)
	switch command {
	case "verify":
		report, err := characterization.Verify(evidenceRoot)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("verified non-publishable compatibility candidate: %d kinds, %d evidence files\n", report.KindCount, report.FileCount)
	case "render-manifest":
		manifest, err := characterization.RenderManifest(evidenceRoot)
		if err != nil {
			fatal(err)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(manifest); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("usage: go run ./cmd/conformance [verify|render-manifest]"))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "conformance:", err)
	os.Exit(1)
}
