package intentstorecli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

func TestRunInitializesBoundStoreAndRefusesBindingChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent-store")
	args := []string{"--path", path, "--store-id", "local-scope-a", "--host-api", "https://host.example/api"}
	var stdout, stderr bytes.Buffer
	if code := Run(args, &stdout, &stderr); code != 0 || stdout.String() != "intent store ready\n" || stderr.Len() != 0 {
		t.Fatalf("Run(init) code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	store, err := intentv2.Open(path, "local-scope-a")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := store.Binding(t.Context())
	if err != nil || binding.HostAPI != "https://host.example/api" {
		t.Fatalf("initialized binding=%#v error=%v", binding, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(exact idempotent init) code=%d stderr=%q", code, stderr.String())
	}
	changed := append([]string(nil), args...)
	changed[len(changed)-1] = "https://other.example/api"
	if code := Run(changed, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(changed binding) code=%d, want refusal", code)
	}
	store, err = intentv2.Open(path, "local-scope-a")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	binding, err = store.Binding(t.Context())
	if err != nil || binding.HostAPI != "https://host.example/api" {
		t.Fatalf("changed init altered existing binding=%#v error=%v", binding, err)
	}
}

func TestRunExplicitlyBindsOnlyEmptyUnboundStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent-store")
	if err := intentv2.Initialize(path, "local-scope-b"); err != nil {
		t.Fatal(err)
	}
	args := []string{"--path", path, "--store-id", "local-scope-b", "--host-api", "https://host.example/api"}
	if code := Run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("Run(bind empty unbound) code=%d", code)
	}
	store, err := intentv2.Open(path, "local-scope-b")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if binding, err := store.Binding(t.Context()); err != nil || binding.HostAPI != "https://host.example/api" {
		t.Fatalf("explicit binding=%#v error=%v", binding, err)
	}
}

func TestRunRequiresAllNonSecretIdentityFlags(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--path", "/tmp/store", "--store-id", "scope"},
	} {
		if code := Run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
			t.Fatalf("Run(%q) code=%d, want usage refusal", args, code)
		}
	}
}
