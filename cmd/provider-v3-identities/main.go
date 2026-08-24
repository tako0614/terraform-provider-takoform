// Command provider-v3-identities prints the exact Provider 3 release-ledger
// entry derived from the implementation's current registration projection.
// It is a read-only generator: callers review/apply the JSON to the append-only
// release ledger and repository tests enforce byte-semantic equality.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

func main() {
	projection, err := provider.CurrentProviderV3ReleaseIdentityProjection()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(projection); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
