package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

var (
	compiledProviderOnce sync.Once
	compiledProviderPath string
	compiledProviderErr  error
)

func distributedProviderBinary(t *testing.T) string {
	t.Helper()
	compiledProviderOnce.Do(func() {
		compiledProviderPath = filepath.Join(t.TempDir(), "terraform-provider-takoform")
		cmd := exec.Command("go", "build", "-o", compiledProviderPath, ".")
		if output, err := cmd.CombinedOutput(); err != nil {
			compiledProviderErr = fmt.Errorf("build distributed provider binary: %w\n%s", err, output)
		}
	})
	if compiledProviderErr != nil {
		t.Fatal(compiledProviderErr)
	}
	return compiledProviderPath
}

func TestDistributedProviderBinaryInitializesPrivateIntentStore(t *testing.T) {
	binary := distributedProviderBinary(t)
	var requests atomic.Int32
	host := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer host.Close()

	path := filepath.Join(t.TempDir(), "private-store")
	storeID := "caller-scope-a"
	hostAPI := host.URL + "/apis/forms.takoform.com/v2"
	args := []string{"intent-store-init", "--path", path, "--store-id", storeID, "--host-api", hostAPI}
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := run(args...)
	if err != nil || output != "intent store ready\n" {
		t.Fatalf("distributed initializer output=%q error=%v", output, err)
	}
	output, err = run(args...)
	if err != nil || output != "intent store ready\n" {
		t.Fatalf("exact idempotent initializer output=%q error=%v", output, err)
	}

	changedHost := append([]string(nil), args...)
	changedHost[len(changedHost)-1] = "https://other.example/apis/forms.takoform.com/v2"
	if output, err := run(changedHost...); err == nil || !strings.Contains(output, "refused") {
		t.Fatalf("changed Host root was not refused: output=%q error=%v", output, err)
	}
	changedID := append([]string(nil), args...)
	changedID[4] = "caller-scope-b"
	if output, err := run(changedID...); err == nil || !strings.Contains(output, "refused") {
		t.Fatalf("changed store ID was not refused: output=%q error=%v", output, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("initializer sent %d Host API requests", requests.Load())
	}

	store, err := intentv2.Open(path, storeID)
	if err != nil {
		t.Fatalf("original store was not preserved: %v", err)
	}
	binding, err := store.Binding(t.Context())
	if err != nil || binding.HostAPI != hostAPI {
		t.Fatalf("original binding after refused changes = %#v, error=%v", binding, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if output, err := run("-version"); err != nil || output != "dev\n" {
		t.Fatalf("normal root -version output=%q error=%v", output, err)
	}
}
