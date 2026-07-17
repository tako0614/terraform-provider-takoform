package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tako0614/terraform-provider-takoform/internal/characterization"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
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
		portableRoot := filepath.Join(repoRoot, "conformance", "portable-host-v1")
		portable, err := portableconformance.Verify(portableRoot)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("verified non-publishable compatibility candidate: %d kinds, %d evidence files; portable host %s\n", report.KindCount, report.FileCount, portable.APIVersion)
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
	case "write-manifest":
		manifest, err := characterization.RenderManifest(evidenceRoot)
		if err != nil {
			fatal(err)
		}
		raw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			fatal(err)
		}
		raw = append(raw, '\n')
		if err := os.WriteFile(filepath.Join(evidenceRoot, "manifest.json"), raw, 0o644); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("usage: go run ./cmd/conformance [verify|render-manifest|write-manifest]"))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "conformance:", err)
	os.Exit(1)
}
