package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tako0614/terraform-provider-takoform/internal/characterization"
	"github.com/tako0614/terraform-provider-takoform/internal/migrationproof"
)

func main() {
	root, err := characterization.FindRepoRoot(".")
	if err != nil {
		fail(err)
	}
	report, err := migrationproof.Verify(filepath.Join(root, "conformance", "provider-migration-v1"))
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "migration-proof:", err); os.Exit(1) }
