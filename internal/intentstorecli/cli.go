// Package intentstorecli provides the explicit, shared intent-store initializer
// entrypoint for the provider binary and the standalone source command.
package intentstorecli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

// Run explicitly initializes or verifies one caller-owned v2 intent store.
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("intent-store-init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("path", "", "absolute path for the private intent store")
	storeID := flags.String("store-id", "", "fixed local state-management scope identifier")
	hostAPI := flags.String("host-api", "", "exact HTTPS Host API base URL")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *path == "" || *storeID == "" || *hostAPI == "" {
		fmt.Fprintln(stderr, "usage: intent-store-init --path ABSOLUTE_PATH --store-id ID --host-api HTTPS_BASE_URL")
		return 2
	}
	if err := initialize(*path, *storeID, *hostAPI); err != nil {
		fmt.Fprintln(stderr, "intent store initialization refused")
		return 1
	}
	fmt.Fprintln(stdout, "intent store ready")
	return 0
}

func initialize(path, storeID, hostAPI string) error {
	binding := intentv2.StoreBinding{HostAPI: hostAPI}
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return intentv2.InitializeBound(path, storeID, binding)
	}
	if err != nil {
		return intentv2.ErrStoreUnavailable
	}
	store, err := intentv2.Open(path, storeID)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.Binding(context.Background())
	if err == nil {
		if current == binding {
			return nil
		}
		return intentv2.ErrStoreIdentity
	}
	if !errors.Is(err, intentv2.ErrStoreIdentity) {
		return err
	}
	return store.BindEmpty(context.Background(), binding)
}
