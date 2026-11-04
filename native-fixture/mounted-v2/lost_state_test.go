//go:build native

package mountedv2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// Use the actual OpenTofu process and Provider plugin, with a test-only build
// overlay that exits at either durable boundary. The Host is local and the
// original request must not be sent a second time during recovery.
func TestExistingMutationRecoversAfterProviderExit(t *testing.T) {
	tofu, err := exec.LookPath("tofu")
	if err != nil {
		t.Fatalf("OpenTofu 1.12.3 required: %v", err)
	}
	version, err := exec.Command(tofu, "version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "OpenTofu v1.12.3") {
		t.Fatalf("pinned OpenTofu required: %v %s", err, version)
	}
	t.Logf("OpenTofu: %s", strings.TrimSpace(string(version)))
	root := repositoryRoot(t)
	for _, tc := range []struct {
		name, point, action string
	}{
		{"update-after-ack", "ack", "update"},
		{"update-after-completion", "complete", "update"},
		{"delete-after-ack", "ack", "delete"},
		{"delete-after-completion", "complete", "delete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, attempts := newCountingRecoveryHost(t)
			r := setupRunner(t, root, host, false)
			installExistingMutationExitOverlay(t, root, r, tc.point)
			r.retention = 60
			r.runOK("init", "init", "-input=false", "-no-color")
			r.runOK("create", "apply", "-input=false", "-no-color", "-auto-approve")

			t.Setenv("NATIVE_V2_EXISTING_EXIT", r.marker)
			var first result
			if tc.action == "update" {
				r.retention = 120
				first = r.run("update-exit", "apply", "-input=false", "-no-color", "-auto-approve")
			} else {
				first = r.run("delete-exit", "destroy", "-input=false", "-no-color", "-auto-approve")
			}
			t.Setenv("NATIVE_V2_EXISTING_EXIT", "")
			if first.code == 0 || !strings.Contains(first.output, "Plugin did not respond") {
				t.Fatalf("Provider exit was not observed: exit=%d %s", first.code, first.output)
			}
			marker, err := os.ReadFile(r.marker)
			if err != nil {
				t.Fatalf("Provider exit marker absent: %v", err)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(marker)))
			if err != nil || pid <= 0 || pid == first.pid {
				t.Fatalf("exit marker is not a distinct Provider process: %q; CLI PID=%d", marker, first.pid)
			}
			if mutations, _ := host.counts(); mutations != 2 || attempts.Load() != 2 {
				t.Fatalf("create and original %s: accepted=%d attempts=%d", tc.action, mutations, attempts.Load())
			}
			original := r.runOK("old-state", "state", "show", "takoform_at_least_once_queue.x")
			if !strings.Contains(original.output, "message_retention_seconds = 60") {
				t.Fatalf("prior state was not retained after Provider exit: %s", original.output)
			}

			if tc.action == "update" {
				r.runOK("recover-update", "apply", "-input=false", "-no-color", "-auto-approve")
				updated := r.runOK("updated-state", "state", "show", "takoform_at_least_once_queue.x")
				if !strings.Contains(updated.output, "message_retention_seconds = 120") {
					t.Fatalf("recovered update missing from state: %s", updated.output)
				}
			} else {
				r.runOK("recover-delete", "apply", "-refresh-only", "-input=false", "-no-color", "-auto-approve")
				if state := r.runOK("deleted-state", "state", "list"); strings.TrimSpace(state.output) != "" {
					t.Fatalf("recovered delete left a state entry: %s", state.output)
				}
			}
			if mutations, _ := host.counts(); mutations != 2 || attempts.Load() != 2 {
				t.Fatalf("recovery replayed a mutation: accepted=%d attempts=%d", mutations, attempts.Load())
			}
		})
	}
}

func newCountingRecoveryHost(t *testing.T) (*fixtureHost, *atomic.Int32) {
	t.Helper()
	host := &fixtureHost{mode: "update", uid: "uid-queue-1"}
	attempts := &atomic.Int32{}
	host.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost || req.Method == http.MethodPut || req.Method == http.MethodDelete {
			attempts.Add(1)
		}
		host.serve(w, req)
	}))
	t.Cleanup(host.server.Close)
	return host, attempts
}

func installExistingMutationExitOverlay(t *testing.T, root string, r runner, point string) {
	t.Helper()
	file := "v2_lifecycle_existing.go"
	anchor := []byte("\t*state = old\n")
	if point == "complete" {
		file = "v2_resource.go"
		anchor = []byte("\tif intent.Action == intentv2.ActionDelete && op.Status == \"succeeded\" {\n\t\tstate.RemoveResource(ctx)")
	}
	source := filepath.Join(root, "internal", "provider", file)
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(original, anchor) != 1 {
		t.Fatalf("test-only %s exit boundary changed", point)
	}
	insert := []byte("\tif marker := os.Getenv(\"NATIVE_V2_EXISTING_EXIT\"); marker != \"\" { _ = os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0600); os.Exit(73) }\n")
	patched := bytes.Replace(original, anchor, append(insert, anchor...), 1)
	patched = bytes.Replace(patched, []byte("import (\n"), []byte("import (\n\t\"os\"\n"), 1)
	temp := t.TempDir()
	overlaySource := filepath.Join(temp, file)
	if err := os.WriteFile(overlaySource, patched, 0o600); err != nil {
		t.Fatal(err)
	}
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{source: overlaySource}})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(temp, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(r.pluginDir, "registry.terraform.io", "tako0614", "takoform", "5.0.0", runtime.GOOS+"_"+runtime.GOARCH, "terraform-provider-takoform_v5.0.0")
	build := exec.Command("go", "build", "-overlay", overlayPath, "-ldflags=-X main.version=5.0.0", "-o", plugin, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test-only Provider overlay: %v\n%s", err, output)
	}
}
